#ifndef ARGUSSENTINEL_H
#define ARGUSSENTINEL_H

#include <QMainWindow>
#include <QCloseEvent>
#include <QScrollArea>
#include <QComboBox>
#include <QPushButton>
#include <QGroupBox>
#include <QCheckBox>
#include <QLineEdit>
#include <QTextEdit>
#include <QLabel>
#include <QTimer>
#include "qcustomplot.h"
#include <QtDataVisualization/q3dsurface.h>
#include <QtDataVisualization/qsurfacedataproxy.h>
#include <QtDataVisualization/qsurface3dseries.h>
#include <QtDataVisualization/qvalue3daxis.h>
#include <QtDataVisualization/q3dtheme.h>

#include <vector>
#include <string>
#include <optional>
#include <unordered_map>
#include <atomic>
#include <chrono>
#include <filesystem>
#include <thread>

// Espacio de nombres para facilitar legibilidad de manejo de disco
namespace fs = std::filesystem;

// Estructura de bloque dinámico procesado (Similar al diccionario Python de V3)
struct PuntoRadar {
    std::string t_str;
    double f;
    double l;
    std::chrono::system_clock::time_point t_obj;
};

class ArgusSentinel : public QMainWindow {
    Q_OBJECT

public:
    explicit ArgusSentinel(QWidget* parent = nullptr);
    ~ArgusSentinel() = default;

protected:
    void closeEvent(QCloseEvent *event) override;

private slots:
    // Slots puros (equivalentes a los command=self.xxx que reaccionan a clicks y acciones)
    void iniciar_monitor();
    void detener_monitor();
    void abrir_visor();
    void al_cambiar_fuente(int index);
    void abrir_configurador_fuentes();
    void seleccionar_carpeta_capturas();
    void sync_ui_values();
    void mostrar_info_version();
    void toggleTheme();
    
    // Función de inyección segura desde hilo secundario al TextEdit (Log Hacker) cruzado por QMetaObject
    void appendLog(const QString& msg);

signals:
    // Slot activado por el Thread secundario para refrescar pantallas 2D/3D
    void onMallaActualizada();

private:
    void setupUI();
    
    // --- Lógica del Motor ---
    void bucle_monitor();
    std::vector<PuntoRadar> leer_cola_segura(const std::string& ruta);
    void repintar();
    void guardar_snapshot_circular();
    void play_alarm_sound();
    
    // --- Herramientas Helpers y JSON ---
    double get_current_time_sec();
    std::unordered_map<std::string, std::string> cargar_fuentes();
    void guardar_fuentes();
    void actualizar_combo_ui();
    void write_log(const std::string& message);
    void cerrar_app();
    void guardar_captura();

    // ==========================================
    // Variables de Estado y Hilos
    // ==========================================
    std::string carpeta_monitor = "";
    std::string archivo_activo = "";             
    std::atomic<bool> stop_event{false};         
    std::vector<PuntoRadar> ultimo_barrido;      
    std::string hora_exacta = "--:--:--.---";
    int db_min = -10;
    int db_max = 80;
    bool isLightMode = false;

    std::optional<double> current_fmin = std::nullopt;
    std::optional<double> current_fmax = std::nullopt;
    double current_umbral = 15.0;
    std::atomic<bool> is_beeping{false};

    std::vector<PuntoRadar> buffer_raw;
    double f_min_actual = 0.0;
    double f_max_actual = 0.0;

    int hist_length = 25;
    std::vector<std::vector<double>> Z_history;  
    std::vector<double> grid_freqs;              
    std::optional<double> _last_fmin_hash = std::nullopt;
    std::optional<double> _last_fmax_hash = std::nullopt;

    std::string carpeta_capturas = "";
    std::vector<std::string> buffer_capturas;
    int max_capturas = 10;
    double last_snap_time = 0.0;
    double last_5min_snap = 0.0;

    std::string archivo_fuentes = "sentinel_fuentes.json";
    std::unordered_map<std::string, std::string> fuentes_guardadas;

    // ==========================================
    // Componentes Dinámicos de la Interfaz UI
    // ==========================================
    QComboBox* combo_fuentes;
    QPushButton* btn_start;
    QPushButton* btn_stop;
    QLabel* lbl_status;
    QLineEdit* entry_fmin;
    QLineEdit* entry_fmax;
    QCheckBox* chk_filtro_2d;
    QLineEdit* entry_threshold;
    QCheckBox* chk_alarm;
    QLineEdit* entry_snap_count;
    QLabel* lbl_out_path;
    QTextEdit* txt_detections;
    QTextEdit* log_text;

    // Punteros al motor gráfico 2D (y 3D opcional)
    QCustomPlot* plot2D;
    Q3DSurface* graph3D;
    QSurface3DSeries* series3D;
};

#endif // ARGUSSENTINEL_H
