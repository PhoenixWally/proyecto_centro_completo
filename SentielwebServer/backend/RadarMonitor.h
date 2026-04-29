#ifndef RADAR_MONITOR_H
#define RADAR_MONITOR_H

#include <string>
#include <vector>
#include <atomic>
#include <mutex>
#include <thread>
#include <functional>
#include <optional>
#include <filesystem>

struct PuntoRadar {
    uint64_t t_obj;
    double l;
    double f;
};

enum class ArgusFileType {
    TYPE_A_LOG,
    TYPE_B_TIMEBASE,
    TYPE_C_MEASUREMENTS,
    UNKNOWN_OR_EMPTY
};

class RadarMonitor {
public:
    RadarMonitor(const std::string& id, const std::string& path);
    ~RadarMonitor();

    void setFrequencyFilter(std::optional<double> fmin, std::optional<double> fmax);
    void start();
    void stop();
    void clearCache();

    // Callback de alto rendimiento ejecutado cada 500ms para mandar vía WebSockets
    std::function<void(const std::string& json_payload)> onDataBroadcast;
    std::function<void(const std::string&)> onLogBroadcast;

private:
    void threadLoop();
    void readBinaryFile(const std::string& filePath);
    
    std::string source_id;
    std::string folder_path;
    
    std::atomic<bool> is_running{false};
    std::thread worker_thread;
    std::mutex data_mtx;

    std::optional<double> current_fmin = std::nullopt;
    std::optional<double> current_fmax = std::nullopt;

    std::vector<PuntoRadar> buffer_raw;
    std::vector<PuntoRadar> ultimo_barrido;
    std::string ultimo_metadata = "";

    std::vector<std::vector<double>> Z_history;
    std::vector<double> grid_freqs;

    double f_min_actual = 0.0;
    double f_max_actual = 0.0;
    std::string hora_exacta = "--:--:--";
    std::string archivo_activo = "";
    int db_min = -10;
    int db_max = 80;
    int hist_length = 25;
    
    // Tools
    std::filesystem::file_time_type cache_last_file_time;

    // Adaptative Amnesia
    double last_f_for_sweep = 0.0;
    uint64_t last_sweep_start_t = 0;
    uint64_t dynamic_amnesia_sec = 3;
};

#endif
