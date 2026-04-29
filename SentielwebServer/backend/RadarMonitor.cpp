#include "RadarMonitor.h"
#include <fstream>
#include <iostream>
#include <cmath>
#include <algorithm>
#include <chrono>
#include <map>
#include <cstring>
#include <nlohmann/json.hpp>

using json = nlohmann::json;
namespace fs = std::filesystem;

RadarMonitor::RadarMonitor(const std::string& id, const std::string& path)
    : source_id(id), folder_path(path) {
    cache_last_file_time = std::filesystem::file_time_type::min();
}

RadarMonitor::~RadarMonitor() {
    stop();
}

void RadarMonitor::setFrequencyFilter(std::optional<double> fmin, std::optional<double> fmax) {
    std::lock_guard<std::mutex> lock(data_mtx);
    current_fmin = fmin;
    current_fmax = fmax;
}

void RadarMonitor::start() {
    if (is_running) return;
    is_running = true;
    worker_thread = std::thread(&RadarMonitor::threadLoop, this);
}

void RadarMonitor::stop() {
    is_running = false;
    if (worker_thread.joinable()) {
        worker_thread.join();
    }
}

ArgusFileType clasificarArchivo(const std::string& ruta) {
    std::ifstream file(ruta, std::ios::binary);
    if (!file.is_open()) return ArgusFileType::UNKNOWN_OR_EMPTY;

    std::vector<char> head(256, 0);
    file.read(head.data(), head.size());
    std::streamsize bytes_read = file.gcount();
    if (bytes_read == 0) return ArgusFileType::UNKNOWN_OR_EMPTY;

    std::string ascii_equiv;
    for(int i=0; i<bytes_read; i++) {
        if(head[i] != 0x00 && head[i] >= 32 && head[i] <= 126) ascii_equiv += head[i];
    }
    
    if (ascii_equiv.find("CTER") != std::string::npos || 
        ascii_equiv.find("ALARMAS") != std::string::npos ||
        ascii_equiv.find("EA-MALAGA") != std::string::npos ||
        ascii_equiv.find("Log") != std::string::npos) {
        return ArgusFileType::TYPE_A_LOG;
    }

    int null_count = 0;
    int check_len = std::min((int)bytes_read, 176); // Hasta offset 0xB0 approx
    for (int i = 0; i < check_len; i++) {
        if (head[i] == 0x00) null_count++;
    }
    // Si más del 85% de la cabecera es nula, es el tracker de la base de tiempos
    if (check_len >= 170 && null_count > 150) {
        return ArgusFileType::TYPE_B_TIMEBASE;
    }

    return ArgusFileType::TYPE_C_MEASUREMENTS;
}

#include <regex>

void RadarMonitor::readBinaryFile(const std::string& ruta) {
    std::vector<PuntoRadar> puntos;
    std::ifstream file(ruta, std::ios::binary | std::ios::ate); 
    if (!file.is_open()) return;
    
    std::streamsize sz = file.tellg();
    if (sz < 28) return;
    
    std::streamsize offset = sz - 2;
    if (offset > 200000 * 26) offset = 200000 * 26; // Leer aprox 5.2 MB para cazar barridos lentos enteros
    std::streamsize start_pos = sz - offset;
    
    std::streamsize residuo = (start_pos - 2) % 26;
    if (residuo != 0) start_pos -= residuo;
    if (start_pos < 2) start_pos = 2;
    
    file.seekg(start_pos, std::ios::beg);
    std::vector<char> buffer(sz - start_pos);
    if(!file.read(buffer.data(), buffer.size())) return;
    
#pragma pack(push, 1)
    struct ArgusChunk {
        uint64_t tr;
        double fr;
        double lv;
        uint16_t ex;
    };
#pragma pack(pop)

    size_t block_offset = 0;
    while (block_offset + 26 <= buffer.size()) {
        ArgusChunk chunk;
        std::memcpy(&chunk, buffer.data() + block_offset, 26);
        block_offset += 26;
        
        uint64_t total_sec = (chunk.tr / 10000000ULL);
        if (total_sec < 11644473600ULL) continue; // Descartar si el archivo no tiene timestamp válido
        
        time_t unix_time = total_sec - 11644473600ULL;
        struct tm parts_utc = {0}, parts_loc = {0};
        
        if (gmtime_s(&parts_utc, &unix_time) != 0 || localtime_s(&parts_loc, &unix_time) != 0) continue;
        if ((parts_utc.tm_year + 1900) < 2020) continue; // Descartar registros arqueológicos (corrupciones)
        
        PuntoRadar pr;
        pr.f = chunk.fr / 1000000.0; 
        
        // Protección sensata contra 'Zero-Padding' (cuando escribe NULOS en lugar de frecuencias)
        if (pr.f > 10.0 && pr.f < 50000.0) {
            pr.l = chunk.lv;
            pr.t_obj = static_cast<uint64_t>(unix_time);
            
            char bUtc[32], bLoc[32];
            std::strftime(bUtc, sizeof(bUtc), "UTC %H:%M:%S", &parts_utc);
            std::strftime(bLoc, sizeof(bLoc), "LOC %H:%M:%S", &parts_loc);
            hora_exacta = std::string(bUtc) + "<br>" + bLoc;
            
            puntos.push_back(pr);
        }
    }
    
    // ============================================
    // LECTURA DEL FOOTER METADATA (V3 Seguro - Sin Regex Masivo)
    // ============================================
    std::streamsize tail_offset = std::max<std::streamsize>(0, sz - 4096);
    file.seekg(tail_offset, std::ios::beg);
    std::vector<char> tail_buf(sz - tail_offset);
    file.read(tail_buf.data(), tail_buf.size());

    // Decodificación UTF-16 Segura: Capturamos rachas de caracteres puramente alfanuméricos legibles
    std::string tail_ascii = "";
    int valid_chars = 0;
    for(size_t i=0; i<tail_buf.size(); i++) {
        char c = tail_buf[i];
        if (c >= 32 && c <= 126) {
            tail_ascii += c;
            valid_chars++;
        } else if (c == 0x00) {
            // Ignorar el null del UTF-16
        } else {
            // Si es un byte puramente binario (ruido float64), ponemos un espacio
            if (!tail_ascii.empty() && tail_ascii.back() != ' ') tail_ascii += ' ';
        }
    }
    
    // Extractor Heurístico del Footer UTF-16
    if (valid_chars > 20 && (tail_ascii.find("202") != std::string::npos || tail_ascii.find("Hz") != std::string::npos || tail_ascii.find("CTER") != std::string::npos || tail_ascii.find("Modo") != std::string::npos)) {
        std::string footer_clean = "";
        for (char c : tail_ascii) {
            if (c == ' ' && !footer_clean.empty() && footer_clean.back() == ' ') continue;
            footer_clean += c;
        }
        if (footer_clean.length() > 5) {
            std::lock_guard<std::mutex> lock(data_mtx);
            ultimo_metadata = footer_clean;
        }
    }

    if (!puntos.empty()) {
        std::lock_guard<std::mutex> lock(data_mtx);
        ultimo_barrido.insert(ultimo_barrido.end(), puntos.begin(), puntos.end());
    }
}

void RadarMonitor::threadLoop() {
    int iteracion = 0;
    fs::file_time_type local_cache = cache_last_file_time;
    
    while (is_running) {
        try {
            if (!fs::exists(folder_path)) {
                if (onLogBroadcast) onLogBroadcast("[Error] La ruta no existe o no tiene permisos: " + folder_path);
                std::this_thread::sleep_for(std::chrono::seconds(2));
                continue;
            }

            // Polling Optimizado (Cached directory scan)
            if (iteracion % 2 == 0) {
                for (const auto& entry : fs::directory_iterator(folder_path)) {
                    if (entry.is_regular_file()) {
                        auto dt = entry.last_write_time();
                        if (dt > local_cache) {
                            local_cache = dt;
                            if (archivo_activo != entry.path().string()) {
                                archivo_activo = entry.path().string();
                                
                                if (onLogBroadcast) {
                                    onLogBroadcast("[Lector] Nuevo archivo de trazas: " + entry.path().filename().string());
                                }
                                
                                std::lock_guard<std::mutex> lock(data_mtx);
                                buffer_raw.clear(); // Limpiar memoria solo si es archivo virgen
                            }
                        }
                    }
                }
            }
            iteracion++;

            if (!archivo_activo.empty()) {
                // Heurística en tiempo real: Ignorar archivos inútiles instantáneamente
                ArgusFileType tipo = clasificarArchivo(archivo_activo);
                if (tipo == ArgusFileType::TYPE_A_LOG || tipo == ArgusFileType::TYPE_B_TIMEBASE) {
                    if(iteracion % 20 == 0 && onLogBroadcast) {
                        onLogBroadcast("[Salto] Ignorando archivo de sistema (" + archivo_activo + ")");
                    }
                } else if (tipo == ArgusFileType::TYPE_C_MEASUREMENTS) {
                    readBinaryFile(archivo_activo);
                }
            } else {
                if(iteracion % 20 == 0 && onLogBroadcast) {
                    onLogBroadcast("[Aviso] No se han encontrado archivos binarios en " + folder_path);
                }
            }

            // ================= PROCESAMIENTO =================
            std::vector<PuntoRadar> copiabuf;
            {
                std::lock_guard<std::mutex> lock(data_mtx);
                buffer_raw.insert(buffer_raw.end(), ultimo_barrido.begin(), ultimo_barrido.end());
                ultimo_barrido.clear();
                
                if(!buffer_raw.empty()) {
                    uint64_t max_t = buffer_raw.front().t_obj;
                    for (const auto& d : buffer_raw) if (d.t_obj > max_t) max_t = d.t_obj;
                    uint64_t corte = max_t - 3; // 3 seconds amnesia
                    buffer_raw.erase(std::remove_if(buffer_raw.begin(), buffer_raw.end(), [corte](const PuntoRadar& d) { return d.t_obj < corte; }), buffer_raw.end());
                }
                copiabuf = buffer_raw;
            }

            std::map<double, double> df_agrupado;
            for(const auto& pt : copiabuf) {
                // [Optimizador WEB] Agrupamos con recortes de 2 decimales y medio (factor 500)
                // En C++ nativo se podía subir al factor 10000 (HD), pero un navegador se asfixia
                // de ram si le mandamos más de 10.000 SVG paths de Plotly cada 500 milisegundos.
                double f_round = std::round(pt.f * 500.0) / 500.0;
                if (df_agrupado.find(f_round) == df_agrupado.end() || pt.l > df_agrupado[f_round]) df_agrupado[f_round] = pt.l;
            }

            std::vector<double> x, y;
            for (const auto& pair : df_agrupado) {
                if (current_fmin.has_value() && pair.first < current_fmin.value()) continue;
                if (current_fmax.has_value() && pair.first > current_fmax.value()) continue;
                x.push_back(pair.first);
                y.push_back(pair.second);
            }
            if (x.empty() && iteracion % 20 == 0 && onLogBroadcast) {
                onLogBroadcast("[Aviso] Archivo leído pero sin tramas válidas recientes.");
            }

            if (!x.empty()) {
                double min_f = *std::min_element(x.begin(), x.end());
                double max_f = *std::max_element(x.begin(), x.end());
                
                // Evitamos que la gráfica colapse si el radar se fija a una sola frecuencia (CW)
                if (max_f - min_f < 0.1) {
                    double center = (min_f + max_f) / 2.0;
                    min_f = center - 0.5; max_f = center + 0.5;
                }

                bool cambio_banda = false;
                // Expansión Dinámica de Bloqueo: NUNCA se encoge, solo se expande 
                // hasta "descubrir" el borde real del barrido del radar y luego se bloquea permanentemente.
                if (grid_freqs.empty() || min_f < f_min_actual || max_f > f_max_actual) {
                    if (grid_freqs.empty()) {
                        f_min_actual = min_f;
                        f_max_actual = max_f;
                    } else {
                        f_min_actual = std::min(f_min_actual, min_f);
                        f_max_actual = std::max(f_max_actual, max_f);
                    }
                    cambio_banda = true;
                }

                int NUM_BINS = 300;
                double step = (f_max_actual - f_min_actual) / (NUM_BINS - 1);
                std::vector<double> current_f_keys;
                for(int i=0; i<NUM_BINS; i++) current_f_keys.push_back(f_min_actual + i * step);

                if (Z_history.empty() || grid_freqs.empty() || cambio_banda) {
                    grid_freqs = current_f_keys;
                    Z_history.clear();
                    std::vector<double> base_row(NUM_BINS, (double)db_min);
                    for(int i=0; i<hist_length; ++i) Z_history.push_back(base_row);
                }

                std::vector<double> row_z(NUM_BINS, (double)db_min);
                for(int i=0; i<NUM_BINS; i++) {
                    double qx = grid_freqs[i];
                    auto it = std::lower_bound(x.begin(), x.end(), qx);
                    if (it == x.end() || it == x.begin()) row_z[i] = (double)db_min;
                    else {
                        auto it_prev = it - 1;
                        double x0 = *it_prev; double y0 = y[std::distance(x.begin(), it_prev)];
                        double x1 = *it; double y1 = y[std::distance(x.begin(), it)];
                        if (x1 == x0) row_z[i] = y0;
                        else row_z[i] = y0 + (y1 - y0) * (qx - x0) / (x1 - x0);
                    }
                }
                
                Z_history.erase(Z_history.begin());
                Z_history.push_back(row_z);

                std::string meta_to_send;
                {
                    std::lock_guard<std::mutex> lock(data_mtx);
                    meta_to_send = ultimo_metadata;
                }

                json packet;
                packet["source"] = source_id;
                packet["type"] = "radar_frame";
                packet["time"] = hora_exacta;
                packet["meta_raw"] = meta_to_send;
                packet["x2d"] = x;
                packet["y2d"] = y;
                packet["x3d"] = grid_freqs;
                packet["z3d"] = Z_history; // Matriz 2D
                packet["db_min"] = db_min;
                packet["db_max"] = db_max;

                if (onDataBroadcast) {
                    onDataBroadcast(packet.dump());
                }
            }

            std::this_thread::sleep_for(std::chrono::milliseconds(500));
        } catch (...) {
            std::this_thread::sleep_for(std::chrono::seconds(2));
        }
    }
}

void RadarMonitor::clearCache() {
    std::lock_guard<std::mutex> lock(data_mtx);
    buffer_raw.clear();
    ultimo_barrido.clear();
    Z_history.clear();
    Z_history = std::vector<std::vector<double>>(hist_length, std::vector<double>(grid_freqs.size(), db_min));
}
