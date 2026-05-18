#ifndef STREAM_MANAGER_H
#define STREAM_MANAGER_H

#include <string>
#include <thread>
#include <atomic>
#include <condition_variable>
#include <mutex>
#include <filesystem>
#include <vector>
#include <chrono>
#include <limits>
#include "crow_all.h"
#include "decoder.h"

/*
 * StreamManager - Maquina de estados para monitorizacion de archivos binarios
 * en rutas SMB compartidas.
 *
 * ESTADOS DE ARCHIVO:
 *   SCANNING   -> Buscando el archivo mas reciente en el directorio
 *   LOCKED_ON  -> Archivo activo encontrado y creciendo; procesando datos
 *   STALLED    -> El archivo activo dejo de crecer; esperando nuevo archivo
 *   SWITCHING  -> Archivo mas nuevo detectado; transicion al nuevo objetivo
 *
 * LOGICA DE BARRIDOS:
 *   Cada chunk (26 bytes) es un punto individual (tiempo, frecuencia, nivel).
 *   La estacion barre de fmin a fmax en pasos y luego vuelve a fmin.
 *   Un nuevo barrido se detecta cuando la frecuencia cae por debajo de
 *   la anterior (reset al inicio del rango).
 *   El primer barrido completo calibra los ejes del frontend (init_frame).
 *   Los siguientes barridos se envian como delta_frame normalizados a bins.
 *
 * ACCESO A ARCHIVOS:
 *   Windows: CreateFile con FILE_SHARE_READ | FILE_SHARE_WRITE (no bloquea al escritor)
 *   Linux:   open(O_RDONLY) no-exclusivo por contrato POSIX
 */

enum class StreamState {
    SCANNING,
    LOCKED_ON,
    STALLED,
    SWITCHING
};

struct FileInfo {
    std::string path;
    std::uintmax_t size;
    std::filesystem::file_time_type write_time;
};

class StreamManager {
public:
    /* conn    -> Puntero a la conexion WebSocket activa de Crow
     * smb_dir -> Ruta al directorio donde residen los archivos binarios
     *            Ejemplo Linux: /mnt/samba/MALAGA_01
     *            Ejemplo Windows: \\servidor\samba\st01 (sin barra final)
     */
    StreamManager(crow::websocket::connection* conn, std::string smb_dir);
    ~StreamManager();

    void start();
    void stop();

    std::string state_str() const;

private:
    void monitor_loop();
    FileInfo get_newest_file() const;
    void process_new_data(std::uintmax_t current_size);
    void send_status(const std::string& event, const std::string& filename = "");
    void flush_sweep(bool force = false);
    void send_init_frame();

    // Dependencias externas
    crow::websocket::connection* conn_;
    std::string smb_dir_;

    // Estado de la maquina de archivos
    StreamState state_;

    // Archivo activo
    std::string    current_file_;
    std::uintmax_t last_offset_;
    std::uintmax_t last_known_size_;
    int            stall_ticks_;

    // Ticks sin crecer antes de considerar el archivo como parado (30 x 200ms = 6s)
    static constexpr int STALL_THRESHOLD = 30;

    // ── Logica de barridos ───────────────────────────────────────────────────
    // Buffer del barrido en curso: pares (frecuencia, nivel)
    struct SweepPoint { double freq; double level; };
    std::vector<SweepPoint> sweep_buffer_;

    // Ultima frecuencia del punto anterior (para detectar reset de barrido)
    double last_freq_;

    // Calibracion de ejes (se descubre en el primer barrido completo)
    double   cal_fmin_;
    double   cal_fmax_;
    int      cal_bins_;          // Numero de bins de frecuencia del eje X
    bool     calibrated_;        // true una vez que el primer init_frame fue enviado
    int      pending_sweeps_;    // Barridos acumulados antes de calibrar

    // Control del hilo
    std::atomic<bool> running_;
    std::thread       monitor_thread_;
    std::mutex              cv_mtx_;
    std::condition_variable cv_;
    std::mutex              send_mutex_; // Mutex para proteger el acceso al socket
};

#endif // STREAM_MANAGER_H
