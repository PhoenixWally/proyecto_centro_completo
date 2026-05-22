#include "stream_manager.h"
#include "obfuscator.h"
#include <algorithm>
#include <chrono>
#include <cmath>
#include <cstring>
#include <iostream>
#include <stdexcept>

// Acceso compartido a archivos: no bloquear al proceso escritor
#ifdef _WIN32
#define WIN32_LEAN_AND_MEAN
#include <windows.h> // CreateFile, FILE_SHARE_READ | FILE_SHARE_WRITE
#else
#include <fcntl.h>  // open(), O_RDONLY
#include <unistd.h> // read(), lseek()
#endif

// ============================================================================
//  StreamManager — Implementación de la máquina de estados
//
//  LÓGICA DE BARRIDOS:
//  Cada registro binario (ArgusChunk, 26 bytes) es un punto independiente:
//    - tr: FILETIME (timestamp 100ns desde 1601-01-01)
//    - fr: frecuencia en Hz
//    - lv: nivel en dBµV
//    - ex: ignorado
//
//  La estación barre desde fmin → fmax en pasos y vuelve a fmin.
//  Un barrido completo se detecta cuando la frecuencia de un nuevo punto
//  es MENOR que la del punto anterior (reset del barrido).
//
//  Al completar el PRIMER barrido se envía un "init_frame" al frontend
//  con fmin, fmax y el número de bins descubierto.  Los siguientes
//  barridos se envían como "delta_frame" con un array de nivel por bin
//  (rellenando con NaN las frecuencias no medidas en ese barrido).
// ============================================================================

StreamManager::StreamManager(crow::websocket::connection *conn,
                             std::string smb_dir)
    : conn_(conn), smb_dir_(std::move(smb_dir)), state_(StreamState::SCANNING),
      current_file_(""), last_offset_(0), last_known_size_(0), stall_ticks_(0),
      last_freq_(-1.0), cal_fmin_(std::numeric_limits<double>::max()),
      cal_fmax_(std::numeric_limits<double>::lowest()), cal_bins_(0),
      calibrated_(false), pending_sweeps_(0), running_(false) {}

StreamManager::~StreamManager() { stop(); }

void StreamManager::start() {
  if (running_)
    return;
  running_ = true;
  state_ = StreamState::SCANNING;
  monitor_thread_ = std::thread(&StreamManager::monitor_loop, this);
}

void StreamManager::stop() {
  running_ = false;
  cv_.notify_all();
  if (monitor_thread_.joinable()) {
    monitor_thread_.join();
  }
}

std::string StreamManager::state_str() const {
  switch (state_) {
  case StreamState::SCANNING:
    return "SCANNING";
  case StreamState::LOCKED_ON:
    return "LOCKED_ON";
  case StreamState::STALLED:
    return "STALLED";
  case StreamState::SWITCHING:
    return "SWITCHING";
  }
  return "UNKNOWN";
}

// ────────────────────────────────────────────────────────────────────────────
//  get_newest_file: devuelve el archivo regular más reciente del directorio
// ────────────────────────────────────────────────────────────────────────────
FileInfo StreamManager::get_newest_file() const {
  FileInfo best{};
  best.write_time = std::filesystem::file_time_type::min();
  best.size = 0;

  // --- THROTTLE ANTI-SATURACIÓN SMB ---
  // No permitir que se escanee un directorio enorme más de una vez cada 3
  // segundos para evitar colgar el Explorador de Windows y la red.
  static auto last_scan =
      std::chrono::steady_clock::now() - std::chrono::seconds(10);
  auto now_sys = std::chrono::steady_clock::now();
  if (std::chrono::duration_cast<std::chrono::milliseconds>(now_sys - last_scan)
          .count() < 3000) {
    return best; // Devuelve vacío, forzando a que espere el cooldown
  }
  last_scan = now_sys;
  // ------------------------------------

  std::error_code ec;
  if (!std::filesystem::exists(smb_dir_, ec) ||
      !std::filesystem::is_directory(smb_dir_, ec)) {
    return best;
  }

  auto now = std::filesystem::file_time_type::clock::now();

  for (const auto &entry : std::filesystem::directory_iterator(smb_dir_, ec)) {
    if (ec)
      break;
    if (!entry.is_regular_file(ec) || ec)
      continue;

    // Usar metadatos cacheados por el iterador (reduce las peticiones de red
    // SMB un 99%)
    auto ftime = entry.last_write_time(ec);
    if (ec)
      continue;

    if (ftime > best.write_time) {
      best.write_time = ftime;
      best.path = entry.path().string();
      best.size = entry.file_size(ec);
      if (ec)
        best.size = 0;
    }
  }

  return best;
}

// ────────────────────────────────────────────────────────────────────────────
//  send_status: Notifica al frontend sobre cambios de estado (informativo)
// ────────────────────────────────────────────────────────────────────────────
void StreamManager::send_status(const std::string &event,
                                const std::string &filename) {
  if (!conn_)
    return;
  try {
    crow::json::wvalue msg;
    msg["type"] = "stream_status";
    msg["event"] = event;
    if (!filename.empty()) {
      msg["file"] = std::filesystem::path(filename).filename().string();
    }

    std::lock_guard<std::mutex> lock(send_mutex_);
    if (conn_) {
      conn_->send_text(msg.dump());
    }
  } catch (const std::exception &e) {
    std::cerr << OBF("[StreamManager] ERROR en send_status: ") << e.what()
              << std::endl;
  }
}

// ────────────────────────────────────────────────────────────────────────────
//  send_init_frame: Envía la calibración de ejes al frontend.
//  Solo se llama UNA VEZ tras el primer barrido completo.
//  Incluye además el contenido de ese primer barrido para render inmediato.
// ────────────────────────────────────────────────────────────────────────────
void StreamManager::send_init_frame() {
  if (!conn_ || sweep_buffer_.empty())
    return;

  std::cout << OBF("[StreamManager] Preparando init_frame...") << std::endl;

  // Calcular fmin/fmax REALES de los datos actuales
  double fmin = std::numeric_limits<double>::max();
  double fmax = std::numeric_limits<double>::lowest();
  for (const auto &p : sweep_buffer_) {
    if (p.freq < fmin)
      fmin = p.freq;
    if (p.freq > fmax)
      fmax = p.freq;
  }

  // Rango mínimo de seguridad si los datos son iguales
  if (fmax <= fmin)
    fmax = fmin + 1e6;

  cal_fmin_ = fmin;
  cal_fmax_ = fmax;
  cal_bins_ = 256; // Forzamos 256 bins para compatibilidad total con la web

  // FORZAR RESOLUCIÓN MÍNIMA: Si hay pocos bins, interpolamos a 256 para que el
  // 3D sea fluido
  if (cal_bins_ < 256) {
    std::cout << OBF("[StreamManager] Bins detectados insuficientes (")
              << cal_bins_
              << OBF("). Forzando interpolacion a 256 bins para suavidad.")
              << std::endl;
    cal_bins_ = 256;
  }

  std::cout << OBF("[StreamManager] Calibrando: fmin=") << cal_fmin_
            << OBF(" fmax=") << cal_fmax_ << OBF(" bins=") << cal_bins_
            << std::endl;

  try {
    // Blindaje anti-NaN para metadatos de cabecera
    if (std::isnan(cal_fmin_) || std::isinf(cal_fmin_))
      cal_fmin_ = 0.0;
    if (std::isnan(cal_fmax_) || std::isinf(cal_fmax_))
      cal_fmax_ = 1.0;

    crow::json::wvalue msg;
    msg["type"] = "init_frame";
    msg["fmin"] = cal_fmin_;
    msg["fmax"] = cal_fmax_;
    msg["bins"] = cal_bins_;

    // Interpolación lineal
    std::map<double, double> df;
    for (const auto &p : sweep_buffer_) {
      if (df.find(p.freq) == df.end() || p.level > df[p.freq])
        df[p.freq] = p.level;
    }
    std::vector<double> rx, ry;
    for (const auto &pair : df) {
      rx.push_back(pair.first);
      ry.push_back(pair.second);
    }

    std::vector<double> levels_binned(cal_bins_, -120.0);
    if (!rx.empty()) {
      double step =
          (cal_bins_ > 1) ? (cal_fmax_ - cal_fmin_) / (cal_bins_ - 1) : 1.0;
      for (int i = 0; i < cal_bins_; i++) {
        double qx = cal_fmin_ + i * step;
        auto it = std::lower_bound(rx.begin(), rx.end(), qx);
        if (it == rx.end() || it == rx.begin()) {
          levels_binned[i] = -120.0;
        } else {
          auto it_prev = it - 1;
          double x0 = *it_prev;
          double y0 = ry[std::distance(rx.begin(), it_prev)];
          double x1 = *it;
          double y1 = ry[std::distance(rx.begin(), it)];
          if (x1 == x0)
            levels_binned[i] = y0;
          else
            levels_binned[i] = y0 + (y1 - y0) * (qx - x0) / (x1 - x0);
        }
      }
    }

    // Serialización SEGURA: Usar vector de wvalue intermedio
    std::vector<crow::json::wvalue> levels_vec;
    levels_vec.reserve(cal_bins_);
    for (int i = 0; i < cal_bins_; i++) {
      if (std::isnan(levels_binned[i]) || std::isinf(levels_binned[i])) {
        levels_vec.emplace_back(-120.0);
      } else {
        levels_vec.emplace_back(levels_binned[i]);
      }
    }
    msg["sweep"] = std::move(levels_vec);

    std::cout << OBF("[StreamManager] Enviando init_frame (JSON dump)...")
              << std::endl;
    std::string out = msg.dump();

    std::lock_guard<std::mutex> lock(send_mutex_);
    if (conn_) {
      conn_->send_text(out);
      std::cout << OBF("[StreamManager] init_frame enviado correctamente.")
                << std::endl;
    }
    calibrated_ = true;

  } catch (const std::exception &e) {
    std::cerr << OBF("[StreamManager] ERROR CRITICO en send_init_frame: ")
              << e.what() << std::endl;
  }
}

// ────────────────────────────────────────────────────────────────────────────
//  flush_sweep: Toma el sweep_buffer_ actual, lo normaliza a bins de
//  frecuencia calibrados y lo envía al frontend como delta_frame.
//  Si force=true se envía aunque el buffer esté vacío (barrido vacío).
// ────────────────────────────────────────────────────────────────────────────
void StreamManager::flush_sweep(bool force) {
  if (!conn_)
    return;
  if (!force && sweep_buffer_.empty())
    return;
  if (!calibrated_)
    return;

  try {
    // Paso de frecuencia calibrado
    double step =
        (cal_bins_ > 1) ? (cal_fmax_ - cal_fmin_) / (cal_bins_ - 1) : 1.0;

    std::map<double, double> df;
    for (const auto &p : sweep_buffer_) {
      if (df.find(p.freq) == df.end() || p.level > df[p.freq])
        df[p.freq] = p.level;
    }
    std::vector<double> rx, ry;
    for (const auto &pair : df) {
      rx.push_back(pair.first);
      ry.push_back(pair.second);
    }

    std::vector<double> levels_binned(cal_bins_, -120.0);
    if (!rx.empty()) {
      for (int i = 0; i < cal_bins_; i++) {
        double qx = cal_fmin_ + i * step;
        auto it = std::lower_bound(rx.begin(), rx.end(), qx);
        if (it == rx.end() || it == rx.begin()) {
          levels_binned[i] = -120.0;
        } else {
          auto it_prev = it - 1;
          double x0 = *it_prev;
          double y0 = ry[std::distance(rx.begin(), it_prev)];
          double x1 = *it;
          double y1 = ry[std::distance(rx.begin(), it)];
          if (x1 == x0)
            levels_binned[i] = y0;
          else
            levels_binned[i] = y0 + (y1 - y0) * (qx - x0) / (x1 - x0);
        }
      }
    }

    crow::json::wvalue msg;
    msg["type"] = "delta_frame";

    std::vector<crow::json::wvalue> levels_vec;
    levels_vec.reserve(cal_bins_);
    for (int i = 0; i < cal_bins_; i++) {
      if (std::isnan(levels_binned[i]) || std::isinf(levels_binned[i])) {
        levels_vec.emplace_back(-120.0);
      } else {
        levels_vec.emplace_back(levels_binned[i]);
      }
    }
    msg["sweep"] = std::move(levels_vec);

    std::string out = msg.dump();
    std::lock_guard<std::mutex> lock(send_mutex_);
    if (conn_) {
      conn_->send_text(out);
      // Log de resumen del barrido enviado
      if (!rx.empty()) {
        std::cout << OBF("[StreamManager] SEND delta_frame: ") << cal_bins_
                  << OBF(" bins, FreqRange: [") << rx.front() << OBF(" - ")
                  << rx.back() << OBF("], SampleLv: ") << ry[0] << std::endl;
      }
    }

  } catch (const std::exception &e) {
    std::cerr << OBF("[StreamManager] ERROR en flush_sweep: ") << e.what()
              << std::endl;
  }
}

// Handle global persistente para evitar aperturas constantes en SMB
static HANDLE g_hActiveFile = INVALID_HANDLE_VALUE;
static std::string g_activePath = "";

void CloseActiveHandle() {
    if (g_hActiveFile != INVALID_HANDLE_VALUE) {
        CloseHandle(g_hActiveFile);
        g_hActiveFile = INVALID_HANDLE_VALUE;
        g_activePath = "";
    }
}

int64_t read_shared(const std::string &path, uint8_t *buffer, uintmax_t offset, uintmax_t count) {
    if (g_activePath != path) {
        CloseActiveHandle();
        std::wstring wpath(path.begin(), path.end());
        g_hActiveFile = CreateFileW(wpath.c_str(), GENERIC_READ, 
            FILE_SHARE_READ | FILE_SHARE_WRITE | FILE_SHARE_DELETE, 
            NULL, OPEN_EXISTING, FILE_ATTRIBUTE_NORMAL, NULL);
        if (g_hActiveFile == INVALID_HANDLE_VALUE) return -1;
        g_activePath = path;
    }

    LARGE_INTEGER li;
    li.QuadPart = offset;
    if (!SetFilePointerEx(g_hActiveFile, li, NULL, FILE_BEGIN)) return -1;

    DWORD bytesRead = 0;
    if (!ReadFile(g_hActiveFile, buffer, (DWORD)count, &bytesRead, NULL)) return -1;
    return (int64_t)bytesRead;
}

// ────────────────────────────────────────────────────────────────────────────
//  process_new_data: Lee bloques nuevos desde current_file_, los acumula
//  en sweep_buffer_ y detecta límites de barrido.
//
//  DETECCIÓN DE LÍMITE DE BARRIDO:
//  Cuando la frecuencia del nuevo punto es MENOR que la del punto anterior,
//  la estación ha reiniciado el barrido → fin del barrido anterior.
//
//  PRIMER BARRIDO (calibración):
//  Al detectar el primer fin de barrido → send_init_frame() (una sola vez).
//
//  BARRIDOS SIGUIENTES:
//  Cada fin de barrido → flush_sweep() → delta_frame al frontend.
// ────────────────────────────────────────────────────────────────────────────
uintmax_t get_size_persistent(const std::string &path) {
    if (g_activePath != path) {
        // Forzamos apertura para obtener tamaño si no está abierto
        uint8_t dummy;
        read_shared(path, &dummy, 0, 0);
    }
    if (g_hActiveFile == INVALID_HANDLE_VALUE) return 0;
    LARGE_INTEGER li;
    if (GetFileSizeEx(g_hActiveFile, &li)) return (uintmax_t)li.QuadPart;
    return 0;
}

void StreamManager::process_new_data(std::uintmax_t current_size) {
  if (current_file_.empty() || !conn_) return;
  if (current_size <= last_offset_) return;

  std::uintmax_t new_bytes = current_size - last_offset_;
  std::uintmax_t full_chunks = new_bytes / sizeof(ArgusChunk);
  if (full_chunks == 0) return;

  std::uintmax_t bytes_to_read = full_chunks * sizeof(ArgusChunk);
  std::vector<uint8_t> raw(bytes_to_read);

  int64_t bytes_read = read_shared(current_file_, raw.data(), last_offset_, bytes_to_read);

  if (bytes_read <= 0) return;

  std::uintmax_t chunks_read =
      static_cast<std::uintmax_t>(bytes_read) / sizeof(ArgusChunk);
  last_offset_ += chunks_read * sizeof(ArgusChunk);

  if (chunks_read == 0)
    return;

  std::vector<ArgusChunk> chunks(chunks_read);
  std::memcpy(chunks.data(), raw.data(), chunks_read * sizeof(ArgusChunk));

  // Procesador de ráfagas Sentinel (26 bytes)
  uintmax_t old_offset = last_offset_;
  size_t p_idx = 0;
  size_t consecutive_garbage = 0;

  while (p_idx + sizeof(ArgusChunk) <= bytes_read) {
    ArgusChunk c;
    std::memcpy(&c, raw.data() + p_idx, sizeof(ArgusChunk));
    
    // Comprobamos si es un punto de radar válido (Datos Limpios)
    if (c.fr > 100e6 && c.fr < 30e9) {
        sweep_buffer_.push_back({c.fr, c.lv});
        last_freq_ = c.fr;
        p_idx += sizeof(ArgusChunk);
        consecutive_garbage = 0;
    } else {
        // No es una frecuencia -> Buscamos sincronismo
        size_t k = 1;
        bool found = false;
        size_t max_search = (bytes_read - p_idx - 8);
        if (max_search > 65536) max_search = 65536; 
        
        for (; k < max_search; ++k) {
            double test_fr;
            std::memcpy(&test_fr, raw.data() + p_idx + k, 8);
            if (test_fr > 1500e6 && test_fr < 1600e6) { 
                found = true;
                break;
            }
        }
        
        if (found) {
            p_idx += k;
            consecutive_garbage = 0;
        } else {
            size_t skip = (max_search > 0 ? max_search : 1);
            p_idx += skip;
            consecutive_garbage += skip;
            
            // Si llevamos más de 10KB de puro texto, saltamos al siguiente archivo
            if (consecutive_garbage > 10240) {
                std::cout << OBF("[StreamManager] Salto de archivo: Detectados 10KB de texto/logs.") << std::endl;
                state_ = StreamState::SCANNING;
                break;
            }
        }
    }
  }

  // Actualizamos el offset final basándonos en lo que realmente hemos consumido
  last_offset_ = old_offset + p_idx;

  if (!sweep_buffer_.empty()) {
      if (!calibrated_) send_init_frame();
      
      const size_t CHUNK_SIZE = 1024;
      std::vector<SweepPoint> original = sweep_buffer_;
      sweep_buffer_.clear();

      for (size_t i = 0; i < original.size(); i += CHUNK_SIZE) {
          size_t end = std::min(i + CHUNK_SIZE, original.size());
          sweep_buffer_.assign(original.begin() + i, original.begin() + end);
          flush_sweep();
          if (original.size() > CHUNK_SIZE) {
              std::this_thread::sleep_for(std::chrono::milliseconds(5)); 
          }
      }
      std::cout << OBF("[WS] Enviado total: ") << original.size() << OBF(" puntos en ráfagas.") << std::endl;
      sweep_buffer_.clear();
  }
}

// ────────────────────────────────────────────────────────────────────────────
//  monitor_loop — Máquina de estados principal
// ────────────────────────────────────────────────────────────────────────────
void StreamManager::monitor_loop() {
  std::cout << OBF("[StreamManager] Iniciando monitorización de: ") << smb_dir_
            << std::endl;

  while (running_) {
    try {
      switch (state_) {

      // ────────────────────────────────────────────────────────────
      case StreamState::SCANNING: {
        FileInfo newest = get_newest_file();
        if (!newest.path.empty() && newest.size > 0) {
          // SOLO reseteamos si es un archivo DIFERENTE al que ya teníamos
          if (newest.path != current_file_) {
            current_file_ = newest.path;
            last_offset_ = 2; // Offset de 2 bytes para saltar la cabecera
            last_known_size_ = newest.size;
            stall_ticks_ = 0;
            last_freq_ = -1.0;
            sweep_buffer_.clear();
            std::cout << OBF("[StreamManager] LOCKED_ON (Nuevo Archivo) → ")
                      << std::filesystem::path(current_file_).filename()
                      << std::endl;
          } else {
            // Es el mismo archivo, simplemente volvemos a vigilarlo
            state_ = StreamState::LOCKED_ON;
            break;
          }

          state_ = StreamState::LOCKED_ON;
          send_status("locked_on", current_file_);
        }
        break;
      }

      // ────────────────────────────────────────────────────────────
      case StreamState::LOCKED_ON: {
        auto cur_size = get_size_persistent(current_file_);
        if (cur_size == 0 && !current_file_.empty()) {
          // Si falla la lectura de tamaño, podría ser un error de red
          break;
        }

        if (cur_size > last_known_size_) {
          std::cout << OBF("[StreamManager] File Growth: ") << last_known_size_
                    << OBF(" -> ") << cur_size << OBF(" bytes.") << std::endl;
          stall_ticks_ = 0;
          last_known_size_ = cur_size;
          process_new_data(cur_size);
        } else {
          stall_ticks_++;
          if (stall_ticks_ >= STALL_THRESHOLD) {
            // El archivo no creció: forzar flush del barrido incompleto
            // para que el frontend no quede esperando datos
            if (!sweep_buffer_.empty()) {
              if (calibrated_)
                flush_sweep(true);
              else
                send_init_frame();
              sweep_buffer_.clear();
              last_freq_ = -1.0;
            }
            state_ = StreamState::STALLED;
            std::cout
                << OBF("[StreamManager] STALLED — archivo sin actividad: ")
                << std::filesystem::path(current_file_).filename() << std::endl;
            send_status("stalled", current_file_);
          }
        }

        // Ya no buscamos archivos nuevos aquí para no saturar el SMB con
        // iteraciones de directorio. Si Argus crea un archivo nuevo, dejará de
        // escribir en este, entrará en STALLED, y allí se buscará el archivo
        // nuevo.
        break;
      }

      // ────────────────────────────────────────────────────────────
      case StreamState::STALLED: {
        auto cur_size = get_size_persistent(current_file_);
        if (cur_size > last_known_size_) {
          last_known_size_ = cur_size;
          stall_ticks_ = 0;
          state_ = StreamState::LOCKED_ON;
          std::cout << OBF("[StreamManager] LOCKED_ON (reactivado) → ")
                    << std::filesystem::path(current_file_).filename()
                    << std::endl;
          send_status("locked_on", current_file_);
          process_new_data(cur_size);
          break;
        }

        FileInfo newest = get_newest_file();
        if (!newest.path.empty() && newest.path != current_file_) {
          std::filesystem::file_time_type cur_time;
          std::error_code ec2;
          cur_time = std::filesystem::last_write_time(current_file_, ec2);

          if (!ec2 && newest.write_time > cur_time && newest.size > 0) {
            state_ = StreamState::SWITCHING;
            current_file_ = newest.path;
            last_offset_ = 2; // Cabecera de 2 bytes
            last_known_size_ = newest.size;
            stall_ticks_ = 0;

            std::cout << OBF("[StreamManager] SWITCHING (desde STALLED) → ")
                      << std::filesystem::path(newest.path).filename()
                      << std::endl;
            send_status("switching", newest.path);
          }
        }
        break;
      }

      // ────────────────────────────────────────────────────────────
      case StreamState::SWITCHING: {
        // Resetear estado de barrido al cambiar de archivo
        sweep_buffer_.clear();
        last_freq_ = -1.0;
        // Nota: calibrated_ se mantiene si ya calibramos —
        // mismo tipo de estación, mismos bins esperados.

        process_new_data(last_known_size_);
        state_ = StreamState::LOCKED_ON;
        std::cout << OBF("[StreamManager] LOCKED_ON (tras switch) → ")
                  << std::filesystem::path(current_file_).filename()
                  << std::endl;
        send_status("locked_on", current_file_);
        break;
      }

      } // end switch

    } catch (const std::exception &e) {
      std::cerr << OBF("[StreamManager] Excepcion en monitor_loop [")
                << state_str() << OBF("]: ") << e.what() << std::endl;
    }

    // Pausa interrumpible al instante usando variables de condición
    // (Anti-cuelgue WebSocket)
    std::unique_lock<std::mutex> lk(cv_mtx_);
    cv_.wait_for(lk, std::chrono::milliseconds(1000),
                 [this] { return !running_.load(); });
  }

  std::cout << OBF("[StreamManager] Hilo de monitorización detenido.")
            << std::endl;
}
