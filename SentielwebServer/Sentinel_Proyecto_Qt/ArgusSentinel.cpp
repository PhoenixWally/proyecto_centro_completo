#include "ArgusSentinel.h"
#include <QApplication>
#include <QMessageBox>
#include <QFileDialog>
#include <QInputDialog>
#include <fstream>
#include <algorithm>
#include <cmath>
#include <QVector3D>
#include <QtDataVisualization/qsurfacedataitem.h>
#include <QtDataVisualization/q3dinputhandler.h>
#include <QtDataVisualization/q3dcamera.h>
#include <ctime>
#include <mutex>
#include <iostream>
#include "ConfiguradorFuentes.h"
#include "VisorCapturas.h"

// Variables compartidas exclusivas para C++ Threading Safety
std::mutex radar_mtx;

// La cabecera nlohmann es instalada vía CMake FetchContent ("nlohmann/json.hpp")
#include <nlohmann/json.hpp>
using json = nlohmann::json;

ArgusSentinel::ArgusSentinel(QWidget* parent) : QMainWindow(parent) {
    this->setWindowTitle("Argus Sentinel (C++ Qt6 Rebuild) V3");
    this->resize(1200, 800);
    this->setStyleSheet(
        "QWidget { background-color: #080808; color: #FFFFFF; font-family: 'Segoe UI', Arial; }"
        "QGroupBox { color: #00FF00; border: 1px solid #333333; margin-top: 1ex; border-radius: 5px; font-weight: bold; }"
        "QGroupBox::title { subcontrol-origin: margin; left: 10px; padding: 0 5px; }"
        "QComboBox { background-color: #111111; color: white; border: 1px solid #444; padding: 2px; }"
        "QLineEdit { background-color: #111111; color: white; border: 1px solid #444; }"
    ); // Forzar Dark Mode total sin importar el sistema host

    // Inicializamos variables críticas (Estado seguro)
    buffer_raw.clear();
    stop_event = false;
    is_beeping = false;

    // Carga de configuración inicial JSON simulada en este source
    fuentes_guardadas = cargar_fuentes();

    // Dibujamos toda la UI dinámica (Reemplaza al setup() de Tkinter)
    setupUI();

    // Creamos un hilo de temporizador repetitivo que reemplaza al .after(500)
    QTimer* timerSync = new QTimer(this);
    connect(timerSync, &QTimer::timeout, this, &ArgusSentinel::sync_ui_values);
    timerSync->start(500); 

    // OBLIGATORIO: Conectar el hilo de lectura binario de disco con la pantalla UI para que se repinten.
    connect(this, &ArgusSentinel::onMallaActualizada, this, &ArgusSentinel::repintar);
}

// ===========================================
// IMPLEMENTACIÓN DE UI Y SLOTS COMPLETOS
// ===========================================
#include <QGridLayout>
#include <QVBoxLayout>
#include <QHBoxLayout>
#include <QGroupBox>
#include <QMenuBar>
#include <QMenu>
#include <QAction>

void ArgusSentinel::setupUI() {
    // 0. --- MENÚ SUPERIOR ---
    QMenuBar* menuBarra = this->menuBar();
    QMenu* menuArchivo = menuBarra->addMenu("Archivo");
    menuArchivo->addAction("⚙️ Configurar Fuentes", this, &ArgusSentinel::abrir_configurador_fuentes);
    menuArchivo->addAction("☀️ Cambiar Tema", this, &ArgusSentinel::toggleTheme);
    menuArchivo->addSeparator();
    menuArchivo->addAction("🚪 Salir", this, &ArgusSentinel::cerrar_app);
    
    QMenu* menuAyuda = menuBarra->addMenu("Ayuda");
    menuAyuda->addAction("📜 Instrucciones"); // placeholder
    menuAyuda->addAction("ℹ️ Versión", this, &ArgusSentinel::mostrar_info_version);

    // 1. --- Layout Principal ---
    QWidget* centralWidget = new QWidget(this);
    this->setCentralWidget(centralWidget);
    QGridLayout* mainLayout = new QGridLayout(centralWidget);
    mainLayout->setContentsMargins(10, 10, 10, 10);
    mainLayout->setSpacing(10);
    
    mainLayout->setColumnMinimumWidth(0, 250); // Panel Izquierdo
    mainLayout->setColumnStretch(1, 1);        // Centro
    mainLayout->setColumnMinimumWidth(2, 250); // Panel Derecho
    
    // === 2. PANEL IZQUIERDO (Fuente y Filtros) ===
    QVBoxLayout* leftPanel = new QVBoxLayout();
    
    QLabel* lblFuente = new QLabel("Fuente de Datos:");
    combo_fuentes = new QComboBox();
    
    btn_start = new QPushButton("▶ INICIAR");
    btn_start->setStyleSheet("background-color: #27AE60; font-weight: bold; color: white; padding: 5px;");
    
    btn_stop = new QPushButton("🛑 DETENER");
    btn_stop->setStyleSheet("background-color: #C0392B; font-weight: bold; color: white; padding: 5px;");
    btn_stop->setEnabled(false);
    
    lbl_status = new QLabel("ESPERANDO");
    lbl_status->setAlignment(Qt::AlignCenter);
    
    leftPanel->addWidget(lblFuente);
    leftPanel->addWidget(combo_fuentes);
    leftPanel->addWidget(btn_start);
    leftPanel->addWidget(btn_stop);
    leftPanel->addWidget(lbl_status);
    
    QGroupBox* gbFiltros = new QGroupBox("Filtros Radar");
    QVBoxLayout* layFiltros = new QVBoxLayout(gbFiltros);
    entry_fmin = new QLineEdit(); entry_fmin->setPlaceholderText("Frec. Min (MHz)");
    entry_fmax = new QLineEdit(); entry_fmax->setPlaceholderText("Frec. Max (MHz)");
    chk_filtro_2d = new QCheckBox("Aplicar Filtro Picos"); chk_filtro_2d->setChecked(true);
    entry_threshold = new QLineEdit("15.0"); entry_threshold->setPlaceholderText("Umbral (dB)");
    chk_alarm = new QCheckBox("🔊 Alarma Sonora"); chk_alarm->setChecked(false);
    layFiltros->addWidget(entry_fmin); layFiltros->addWidget(entry_fmax);
    layFiltros->addWidget(chk_filtro_2d); layFiltros->addWidget(entry_threshold);
    layFiltros->addWidget(chk_alarm);
    leftPanel->addWidget(gbFiltros);
    
    // Bloque Capturas (Faltante)
    QGroupBox* gbSnap = new QGroupBox("Configurar Capturas");
    QVBoxLayout* laySnap = new QVBoxLayout(gbSnap);
    entry_snap_count = new QLineEdit("10"); entry_snap_count->setPlaceholderText("Max Capturas");
    QPushButton* btn_dir_snap = new QPushButton("📂 Carpeta guardar");
    QPushButton* btn_view_snaps = new QPushButton("📷 VER CAPTURAS");
    lbl_out_path = new QLabel("Carpeta: (Ninguna)");
    
    laySnap->addWidget(entry_snap_count);
    laySnap->addWidget(btn_dir_snap);
    lbl_out_path->setWordWrap(true);
    lbl_out_path->setMaximumWidth(280); // Límite estricto de texto 
    laySnap->addWidget(lbl_out_path);
    laySnap->addWidget(btn_view_snaps);
    leftPanel->addWidget(gbSnap);
    leftPanel->addStretch();
    
    // Panel Envolvente rígido para no deformar el 3D con rutas UNC largas
    QWidget* wdgLeft = new QWidget();
    wdgLeft->setFixedWidth(320);
    wdgLeft->setLayout(leftPanel);
    mainLayout->addWidget(wdgLeft, 0, 0); // Fila 0, Columna 0
    
    // === 3. PANEL CENTRAL (Radar 3D y Plot 2D) ===
    QVBoxLayout* centerLayout = new QVBoxLayout();
    
    // Gráfica Superior 3D Render
    graph3D = new Q3DSurface();
    QWidget* container3D = QWidget::createWindowContainer(graph3D);
    container3D->setMinimumSize(QSize(400, 300));
    
    series3D = new QSurface3DSeries();
    series3D->setDrawMode(QSurface3DSeries::DrawSurface);
    
    // Gradiente Térmico 3D (Jet/Plasma) adaptado de Python
    QLinearGradient gr3D;
    gr3D.setColorAt(0.0, QColor("#00008B")); // Azul profundo (Ruido)
    gr3D.setColorAt(0.4, QColor("#00FFFF")); // Cian (Línea base)
    gr3D.setColorAt(0.6, QColor("#00FF00")); // Verde (Normal)
    gr3D.setColorAt(0.8, QColor("#FFFF00")); // Amarillo (Aviso)
    gr3D.setColorAt(1.0, QColor("#FF0000")); // Rojo (Saturación)
    series3D->setBaseGradient(gr3D);
    series3D->setColorStyle(Q3DTheme::ColorStyleRangeGradient);
    
    graph3D->addSeries(series3D);
    
    graph3D->axisY()->setTitle("Nivel (dBµV)");
    graph3D->axisZ()->setTitle("Frecuencia (MHz)"); // Eje Profundidad visual
    graph3D->axisX()->setTitle("Tiempo (H)");     // Eje Ancho visual
    graph3D->axisX()->setTitleVisible(true);
    graph3D->axisY()->setTitleVisible(true);
    graph3D->axisZ()->setTitleVisible(true);
    
    // Evitar que el cubo cambie de proporción y se comprima
    graph3D->setAspectRatio(2.0); // Ratio fijo (doble de ancho que de alto)
    graph3D->setHorizontalAspectRatio(1.0);
    
    graph3D->activeTheme()->setBackgroundEnabled(true);
    graph3D->activeTheme()->setBackgroundColor(QColor(10, 10, 10));
    graph3D->activeTheme()->setLabelBackgroundEnabled(false);
    
    // Forzamos los colores de la caja 3D para que no hereden el color negro de Windows Light Mode
    graph3D->activeTheme()->setLabelTextColor(Qt::white);
    graph3D->activeTheme()->setGridLineColor(QColor(80, 80, 80));
    graph3D->activeTheme()->setWindowColor(QColor(10, 10, 10));
    
    // Fijar cámara y vista (Menor inclinación y extra zoom-out inicial)
    Q3DCamera* camera = graph3D->scene()->activeCamera();
    camera->setCameraPreset(Q3DCamera::CameraPresetIsometricLeft);
    camera->setZoomLevel(70.0f); // Alejamos mucho más para empezar
    graph3D->setMargin(0.12f);   // 12% de margen vacío inquebrantable a los lados del 3D
    
    Q3DInputHandler* handler = static_cast<Q3DInputHandler*>(graph3D->activeInputHandler());
    if (handler) {
        handler->setRotationEnabled(false);
        handler->setZoomEnabled(false); // Zoom apagado con ratón para que no cambie por accidente
    }

    // Panel Flotante o Botones Superiores para los Zoom Manuales
    QHBoxLayout* lay3DControls = new QHBoxLayout();
    lay3DControls->addStretch();
    QPushButton* btn_zoom_out = new QPushButton("🔎 Zoom -");
    QPushButton* btn_zoom_in = new QPushButton("🔎 Zoom +");
    btn_zoom_out->setFixedWidth(80);
    btn_zoom_in->setFixedWidth(80);
    lay3DControls->addWidget(btn_zoom_out);
    lay3DControls->addWidget(btn_zoom_in);
    
    QObject::connect(btn_zoom_in, &QPushButton::clicked, [camera]() {
        camera->setZoomLevel(camera->zoomLevel() + 10.0f);
    });
    QObject::connect(btn_zoom_out, &QPushButton::clicked, [camera]() {
        camera->setZoomLevel(camera->zoomLevel() - 10.0f);
    });

    centerLayout->addLayout(lay3DControls);
    centerLayout->addWidget(container3D, 2); // Peso 2 (66% de la pantalla)

    // Gráfica Inferior 2D
    plot2D = new QCustomPlot(this);
    plot2D->addGraph();
    
    // Gradiente Térmico 2D superpuesto en el borde
    QLinearGradient gr2D;
    gr2D.setCoordinateMode(QGradient::ObjectBoundingMode);
    gr2D.setStart(0.0, 1.0);     // 1.0 (Abajo)
    gr2D.setFinalStop(0.0, 0.0); // 0.0 (Arriba)
    gr2D.setColorAt(0.0, QColor("#00008B")); 
    gr2D.setColorAt(0.4, QColor("#00FFFF"));
    gr2D.setColorAt(0.6, QColor("#00FF00"));
    gr2D.setColorAt(0.8, QColor("#FFFF00"));
    gr2D.setColorAt(1.0, QColor("#FF0000"));
    plot2D->graph(0)->setPen(QPen(QBrush(gr2D), 2.0)); // Pluma gruesa pintada con gradiente
    
    // Gráfico de Referencia: Umbral Fijo (Graph 1)
    plot2D->addGraph();
    QPen penUmbral(Qt::DashLine);
    penUmbral.setColor(QColor("#FFD700")); // Amarillo ORO
    penUmbral.setWidth(1);
    plot2D->graph(1)->setPen(penUmbral);
    
    plot2D->setBackground(QBrush(QColor(0, 0, 0)));
    plot2D->xAxis->setLabel("Frecuencia (MHz)");
    plot2D->yAxis->setLabel("Nivel (dBµV)");
    
    // Forzar textos de etiquetas 2D
    plot2D->xAxis->setLabelColor(Qt::white);
    plot2D->yAxis->setLabelColor(Qt::white);
    
    // Forzar colores de las marcas de los ejes (Ticks)
    plot2D->xAxis->setTickLabelColor(Qt::white);
    plot2D->yAxis->setTickLabelColor(Qt::white);
    plot2D->xAxis->setBasePen(QPen(Qt::white));
    plot2D->yAxis->setBasePen(QPen(Qt::white));
    plot2D->xAxis->setTickPen(QPen(Qt::white));
    plot2D->yAxis->setTickPen(QPen(Qt::white));
    plot2D->xAxis->setSubTickPen(QPen(Qt::white));
    plot2D->yAxis->setSubTickPen(QPen(Qt::white));
    plot2D->yAxis->setRange(-10, 80);
    centerLayout->addWidget(plot2D, 1); // Peso 1 (33% inferior)
    
    mainLayout->addLayout(centerLayout, 0, 1);
    
    // === 4. PANEL DERECHO (Picos de Detección) ===
    QVBoxLayout* rightPanel = new QVBoxLayout();
    QLabel* lblDets = new QLabel("Picos de Detección:");
    txt_detections = new QTextEdit();
    txt_detections->setReadOnly(true);
    txt_detections->setStyleSheet("background-color: #0d0d0d; color: #FFD700; font-family: Consolas;"); // Picos en amarillo
    rightPanel->addWidget(lblDets);
    rightPanel->addWidget(txt_detections);
    mainLayout->addLayout(rightPanel, 0, 2);
    
    // === 5. PANEL INFERIOR HORIZONTAL (Consola Log) ===
    QVBoxLayout* bottomPanel = new QVBoxLayout();
    QLabel* lblLog = new QLabel("Consola del Sistema:");
    lblLog->setMaximumHeight(20); // Fija el alto del título para evitar su expansión
    log_text = new QTextEdit();
    log_text->setReadOnly(true);
    log_text->setMaximumHeight(100); // Límite estricto a la caja
    log_text->setStyleSheet("background-color: #1a1a1a; color: #00FF00; font-family: Consolas; font-size: 11px; border: 1px solid #333;");
    bottomPanel->addWidget(lblLog);
    bottomPanel->addWidget(log_text);
    mainLayout->addLayout(bottomPanel, 1, 0, 1, 3); // Fila 1, inicia en Col 0, ocupa 3 Columnas a lo largo
    
    // Reglas maestras de escalado
    mainLayout->setRowStretch(0, 1); // Fila superior expande con la ventana (Para el 3D y 2D)
    mainLayout->setRowStretch(1, 0); // Fila log queda clavada a su tamaño máximo
    
    // Actualización Inicial del Combo
    fuentes_guardadas["Antena Local Test"] = "C:/ArgusTest";
    actualizar_combo_ui();
    
    // Conexiones Eventos
    connect(btn_start, &QPushButton::clicked, this, &ArgusSentinel::iniciar_monitor);
    connect(btn_stop, &QPushButton::clicked, this, &ArgusSentinel::detener_monitor);
    connect(combo_fuentes, QOverload<int>::of(&QComboBox::currentIndexChanged), this, &ArgusSentinel::al_cambiar_fuente);
    connect(btn_view_snaps, &QPushButton::clicked, this, &ArgusSentinel::abrir_visor);
    connect(btn_dir_snap, &QPushButton::clicked, this, &ArgusSentinel::seleccionar_carpeta_capturas);
}

void ArgusSentinel::iniciar_monitor() {
    if (carpeta_monitor.empty()) {
        QMessageBox::warning(this, "Aviso", "No has seleccionado carpeta monitor.");
        return; 
    }
    stop_event = false;
    buffer_capturas.clear();
    buffer_raw.clear(); 
    last_snap_time = get_current_time_sec(); 
    last_5min_snap = last_snap_time;
    
    btn_start->setEnabled(false);
    btn_stop->setEnabled(true);
    lbl_status->setText("ON-LINE: MONITORIZANDO");
    lbl_status->setStyleSheet("color: #27AE60; font-weight: bold;");

    write_log("Desplegando hilo radar sobre: " + carpeta_monitor);
    // Lanza el vigilante crudo paralelizado (detach no colgará Qt)
    std::thread([this]() {
        this->bucle_monitor();
    }).detach(); 
}

void ArgusSentinel::detener_monitor() {
    stop_event = true;
    btn_start->setEnabled(true);
    btn_stop->setEnabled(false);
    lbl_status->setText("STOP: DETENIDO");
    lbl_status->setStyleSheet("color: #E67E22; font-weight: bold;");
}

void ArgusSentinel::bucle_monitor() {
    // Uso atómico puro para evitar condiciones de carrera (Race Conditions)
    auto ultimo_scan = std::chrono::steady_clock::now() - std::chrono::hours(1);
    std::vector<std::pair<fs::path, fs::file_time_type>> candidatos;

    while (!stop_event.load()) {
        try {
            auto ahora = std::chrono::steady_clock::now();
            
            // OPTIMIZACIÓN RED C++: Solo consultamos el directorio UNC en red cada 5 segundos
            if (candidatos.empty() || std::chrono::duration_cast<std::chrono::seconds>(ahora - ultimo_scan).count() > 5) {
                candidatos.clear();
                auto limite = fs::file_time_type::clock::now() - std::chrono::hours(24);
                for (const auto& entry : fs::directory_iterator(carpeta_monitor)) {
                    if (entry.is_regular_file()) {
                        // Uso de caché Win32 iterador (NO dispara Syscalls al disco como fs::last_write_time)
                        auto mtime = entry.last_write_time(); 
                        if (mtime > limite) candidatos.push_back({entry.path(), mtime});
                    }
                }
                ultimo_scan = std::chrono::steady_clock::now();
            }

            if (candidatos.empty()) {
                std::this_thread::sleep_for(std::chrono::seconds(1));
                continue;
            }

            std::sort(candidatos.begin(), candidatos.end(), [](const auto& a, const auto& b) { return a.second > b.second; });
            std::string archivo_seleccionado = candidatos[0].first.string();

            if (archivo_seleccionado != archivo_activo) {
                archivo_activo = archivo_seleccionado;
                write_log("Conectado a Flujo: " + candidatos[0].first.filename().string());
                
                std::lock_guard<std::mutex> lock(radar_mtx);
                buffer_raw.clear(); // Limpia la lista previa (Amnesia del radar V3)
            }

            auto nuevos = leer_cola_segura(archivo_activo);
            if (!nuevos.empty()) {
                {
                    std::lock_guard<std::mutex> lock(radar_mtx);
                    ultimo_barrido.insert(ultimo_barrido.end(), nuevos.begin(), nuevos.end());
                }
                hora_exacta = nuevos.back().t_str;
                
                // Dispara el aviso de repintar a la pantalla a través del nuevo Signal Qt seguro cruzando hilos
                emit onMallaActualizada();
            }
            std::this_thread::sleep_for(std::chrono::milliseconds(500));
        } catch (const std::exception& e) {
            write_log("CRASH LECTOR -> " + std::string(e.what()));
            std::this_thread::sleep_for(std::chrono::seconds(2));
        } catch (...) {
            write_log("CRASH FATAL IGNORADO. Intentando reiniciar lector...");
            std::this_thread::sleep_for(std::chrono::seconds(2));
        }
    }
}

std::vector<PuntoRadar> ArgusSentinel::leer_cola_segura(const std::string& ruta) {
    std::vector<PuntoRadar> puntos;
    std::ifstream file(ruta, std::ios::binary | std::ios::ate); 
    if (!file.is_open()) return puntos;
    
    std::streamsize sz = file.tellg();
    if (sz < 28) return puntos;
    
    std::streamsize offset = sz - 2;
    if (offset > 200 * 26) offset = 200 * 26;
    std::streamsize start_pos = sz - offset;
    
    std::streamsize residuo = (start_pos - 2) % 26;
    if (residuo != 0) start_pos -= residuo;
    if (start_pos < 2) start_pos = 2;
    
    file.seekg(start_pos, std::ios::beg);
    std::vector<char> buffer(sz - start_pos);
    if(!file.read(buffer.data(), buffer.size())) return puntos;
    
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
        if (std::abs(chunk.lv) < 300.0) {
            uint64_t total_sec = (chunk.tr / 10000000ULL);
            
            // FILTRO DE SEGURIDAD EXTREMA: Evitar underflow (Corrupción Binaria)
            if (total_sec > 11644473600ULL) {
                time_t unix_time = total_sec - 11644473600ULL + 3600ULL; // +1 hr offset
                
                struct tm parts = {0}; // Inicializar a cero por seguridad
                if (gmtime_s(&parts, &unix_time) == 0) { // Si la conversión es legítima
                    
                    // Replicar el filtro Python "ts.year > 2020"
                    if ((parts.tm_year + 1900) > 2020) {
                        PuntoRadar pr;
                        pr.f = chunk.fr / 1000000.0; 
                        
                        // [FILTRO DE BASURA BINARIA] Evita que corrupciones en Memoria (ej. Freq=0.0) destrocen el zoom
                        if (pr.f > 10.0 && pr.f < 50000.0) {
                            pr.l = chunk.lv;
                            
                            uint64_t sub_ms = (chunk.tr / 10000ULL) % 1000ULL;
                            char buf[30];
                            std::strftime(buf, sizeof(buf), "%H:%M:%S", &parts);
                            
                            char final_buf[50];
                            snprintf(final_buf, sizeof(final_buf), "%s.%03llu", buf, sub_ms);
                            
                            pr.t_str = final_buf; 
                            pr.t_obj = std::chrono::system_clock::from_time_t(unix_time);
                            
                            puntos.push_back(pr);
                        }
                    }
                        
                        char final_buf[50];
                        snprintf(final_buf, sizeof(final_buf), "%s.%03llu", buf, sub_ms);
                        
                        pr.t_str = final_buf; 
                        pr.t_obj = std::chrono::system_clock::from_time_t(unix_time);
                        
                        puntos.push_back(pr);
                    }
                }
            }
        }
        block_offset += 26;
    }
    return puntos;
}

void ArgusSentinel::sync_ui_values() {
    try {
        if (!entry_fmin->text().isEmpty()) current_fmin = entry_fmin->text().replace(',', '.').toDouble();
        else current_fmin = std::nullopt;
        
        if (!entry_fmax->text().isEmpty()) current_fmax = entry_fmax->text().replace(',', '.').toDouble();
        else current_fmax = std::nullopt;
        
        if (!entry_threshold->text().isEmpty()) current_umbral = entry_threshold->text().replace(',', '.').toDouble();
        else current_umbral = 15.0;
    } catch (...) {}
}

void ArgusSentinel::al_cambiar_fuente(int index) {
    if (index > 0) {
        QString eleccion = combo_fuentes->currentText();
        std::string name = eleccion.toStdString();
        if (fuentes_guardadas.find(name) != fuentes_guardadas.end()) {
            carpeta_monitor = fuentes_guardadas[name];
            btn_start->setEnabled(true);
            grid_freqs.clear(); // Forzamos reset total del tamaño de la malla al cambiar de radar
            write_log("Origen espectral enganchado: " + name);
            write_log("Ruta de vigia: " + carpeta_monitor);
        }
    } else {
        carpeta_monitor = "";
        btn_start->setEnabled(false);
    }
}

void ArgusSentinel::actualizar_combo_ui() {
    combo_fuentes->clear();
    combo_fuentes->addItem("-- Selecciona Foco Origen --");
    for (const auto& pair : fuentes_guardadas) {
        combo_fuentes->addItem(QString::fromStdString(pair.first));
    }
}

void ArgusSentinel::write_log(const std::string& message) {
    auto now = std::chrono::system_clock::now();
    std::time_t now_c = std::chrono::system_clock::to_time_t(now);
    struct tm parts;
    if(localtime_s(&parts, &now_c) == 0) { 
        char time_str[20];
        std::strftime(time_str, sizeof(time_str), "%H:%M:%S", &parts);
        std::string log_line = "[" + std::string(time_str) + "] " + message;
        
        // Uso de QMetaObject para cruzar llamadas desde un hilo secundario al Principal TextEdit visual sin hacer Crash.
        QMetaObject::invokeMethod(log_text, [this, log_line]() {
            log_text->append(QString::fromStdString(log_line));
        });
    }
}

void ArgusSentinel::repintar() {
    std::vector<PuntoRadar> buffer_descarga;
    {
        std::lock_guard<std::mutex> lock(radar_mtx);
        if (ultimo_barrido.empty()) return;
        buffer_descarga = std::move(ultimo_barrido);
        ultimo_barrido.clear();
    }
    
    try {
        {
            std::lock_guard<std::mutex> lock(radar_mtx);
            buffer_raw.insert(buffer_raw.end(), buffer_descarga.begin(), buffer_descarga.end());
            
            // Poda 3 Segundos
            auto max_t = buffer_raw[0].t_obj;
            for (const auto& d : buffer_raw) if (d.t_obj > max_t) max_t = d.t_obj;
            auto tiempo_corte = max_t - std::chrono::seconds(3);
            buffer_raw.erase(std::remove_if(buffer_raw.begin(), buffer_raw.end(), [&tiempo_corte](const PuntoRadar& d) { return d.t_obj < tiempo_corte; }), buffer_raw.end());
        }

        // Max-Hold
        std::map<double, double> df_agrupado;
        {
            std::lock_guard<std::mutex> lock(radar_mtx);
            for (const auto& pt : buffer_raw) {
                double f_round = std::round(pt.f * 10000.0) / 10000.0;
                if (df_agrupado.find(f_round) == df_agrupado.end() || pt.l > df_agrupado[f_round]) df_agrupado[f_round] = pt.l;
            }
        }

        bool usar_filtro_2d = chk_filtro_2d->isChecked();
        QVector<double> x, y; // Arrays nativos de Qt para Fast-Draw
        for (const auto& pair : df_agrupado) {
            if (usar_filtro_2d) {
                if (current_fmin.has_value() && pair.first < current_fmin.value()) continue;
                if (current_fmax.has_value() && pair.first > current_fmax.value()) continue;
            }
            x.append(pair.first);
            y.append(pair.second);
        }

        if (x.isEmpty()) return;

        double min_f = *std::min_element(x.begin(), x.end());
        double max_f = *std::max_element(x.begin(), x.end());
        
        // Evitar colapso gráfico si el radar hace Fixed Frequency (CW) muy estrecha
        if (max_f - min_f < 0.1) {
            double center = (min_f + max_f) / 2.0;
            min_f = center - 0.5;
            max_f = center + 0.5;
        }
        
        // Expansión Dinámica de Bloqueo: NUNCA se encoge, solo se expande 
        // hasta "descubrir" el borde real del barrido del radar y luego se bloquea permanentemente.
        bool cambio_banda = false;
        if (grid_freqs.empty() || min_f < f_min_actual || max_f > f_max_actual) {
            if (grid_freqs.empty()) {
                f_min_actual = min_f;
                f_max_actual = max_f;
            } else {
                f_min_actual = std::min(f_min_actual, min_f);
                f_max_actual = std::max(f_max_actual, max_f);
            }
            plot2D->xAxis->setRange(f_min_actual, f_max_actual);
            cambio_banda = true;
        }

        // Inyección V3 Directa 2D
        plot2D->graph(0)->setData(x, y);
        
        // Inyectar Línea Umbral Amarilla de referencia visual
        QVector<double> ux(2), uy(2);
        ux[0] = plot2D->xAxis->range().lower; uy[0] = current_umbral;
        ux[1] = plot2D->xAxis->range().upper; uy[1] = current_umbral;
        plot2D->graph(1)->setData(ux, uy);
        
        plot2D->replot(QCustomPlot::rpQueuedReplot); // 'QueuedReplot' evita bloquear el UI
        
        // ===================================
        // TUBERÍA 3D (Interpolación Numérica Continua np.interp)
        // ===================================   
        int NUM_BINS = 300;
        double f_start = f_min_actual; // Usamos el filtro maestro bloqueado
        double f_end = f_max_actual;
        if (f_start >= f_end) f_end = f_start + 1.0;
        
        std::vector<double> current_f_keys;
        double step = (f_end - f_start) / (NUM_BINS - 1);
        for(int i=0; i<NUM_BINS; i++) current_f_keys.push_back(f_start + i * step);

        // Reseteo Completo de Matriz solo cuando cambiamos radicalmente de banda
        if (Z_history.empty() || grid_freqs.empty() || cambio_banda) {
            grid_freqs = current_f_keys;
            Z_history.clear();
            std::vector<double> base_row(NUM_BINS, (double)db_min); // piso de ruido
            for(int i=0; i<hist_length; ++i) Z_history.push_back(base_row);
        }

        // np.interp equivalente en C++
        std::vector<double> row_z(NUM_BINS, (double)db_min);
        for(int i=0; i<NUM_BINS; i++) {
            double qx = grid_freqs[i];
            auto it = std::lower_bound(x.begin(), x.end(), qx);
            if (it == x.end()) row_z[i] = (double)db_min;
            else if (it == x.begin()) row_z[i] = (double)db_min;
            else {
                auto it_prev = it - 1;
                double x0 = *it_prev; double y0 = y[std::distance(x.begin(), it_prev)];
                double x1 = *it; double y1 = y[std::distance(x.begin(), it)];
                if (x1 == x0) row_z[i] = y0;
                else row_z[i] = y0 + (y1 - y0) * (qx - x0) / (x1 - x0);
            }
        }

        // Shift temporal y auto-amnesia en RAM
        Z_history.erase(Z_history.begin());
        Z_history.push_back(row_z);

        // Envío Estricto transpuesto hacia C++. (Python X=Freq, Y=Time. Aquí Z=Freq, X=Time)
        // Las Filas del Array en Q3D definen el eje Z (Frecuencia) y las Columnas definen el Eje X (Tiempo)
        QSurfaceDataArray *dataArray = new QSurfaceDataArray;
        dataArray->reserve(NUM_BINS);
        for (int j = 0; j < NUM_BINS; j++) {
            QSurfaceDataRow *newRow = new QSurfaceDataRow(Z_history.size());
            for (size_t i = 0; i < Z_history.size(); i++) {
                // Posiciones: (X=Tiempo, Y=Nivel, Z=Frecuencia)
                (*newRow)[i].setPosition(QVector3D((double)i, Z_history[i][j], grid_freqs[j]));
            }
            dataArray->append(newRow);
        }
        series3D->dataProxy()->resetArray(dataArray);
        
        // Ajuste fijo y ESTRICTO de los ejes nativos Q3D para prevenir compresión visual
        graph3D->axisY()->setRange(db_min, db_max);
        graph3D->axisZ()->setRange(f_start, f_end); // Eje Profundidad
        graph3D->axisX()->setRange(0, hist_length - 1); // Eje Horizontal time

        // Envío Opcional de detecciones de Pico al Panel Derecha
        auto p_max = std::max_element(y.begin(), y.end());
        if(p_max != y.end() && *p_max >= current_umbral) {
            int idx = std::distance(y.begin(), p_max);
            QString det = QString("T:%1 -> F:%2 L:%3").arg(QString::fromStdString(hora_exacta)).arg(x[idx]).arg(y[idx]);
            QMetaObject::invokeMethod(txt_detections, [this, det]() { txt_detections->append(det); });
            
            if (chk_alarm->isChecked() && !is_beeping.load()) {
                is_beeping = true;
                std::thread([this](){
                    std::cout << "\a" << std::flush;
                    std::this_thread::sleep_for(std::chrono::milliseconds(200));
                    this->is_beeping = false;
                }).detach();
            }
        }
        
        // LÓGICA DE CAPTURAS: Guardar Pantallazo cada 5 Minutos Automáticamente
        if (!carpeta_capturas.empty()) {
            double ahora_sec = std::chrono::duration_cast<std::chrono::seconds>(std::chrono::system_clock::now().time_since_epoch()).count();
            if ((ahora_sec - last_5min_snap) >= 300.0) { // 300s = 5 minutos
                last_5min_snap = ahora_sec;
                guardar_captura();
            }
        }
        
    } catch (...) {}
}

void ArgusSentinel::guardar_captura() {
    if (carpeta_capturas.empty()) return;
    int snaps_limit = 10;
    try { snaps_limit = std::stoi(entry_snap_count->text().toStdString()); } catch(...) {}
    
    // Lista actual de fotos ascendente (las viejas primero)
    std::vector<std::pair<fs::path, fs::file_time_type>> fotos;
    for (const auto& entry : fs::directory_iterator(carpeta_capturas)) {
        if (entry.path().extension() == ".png") {
            fotos.push_back({entry.path(), entry.last_write_time()}); // Usa el caché nativo Win32
        }
    }
    std::sort(fotos.begin(), fotos.end(), [](const auto& a, const auto& b) { return a.second < b.second; });
    
    // Rotación FIFO estricta
    while (fotos.size() >= snaps_limit && !fotos.empty()) {
        std::error_code ec;
        fs::remove(fotos[0].first, ec);
        fotos.erase(fotos.begin());
    }
    
    // Generar formato fecha Radar_YYYYMMDD_HHMMSS.png
    time_t rawtime; struct tm* timeinfo; char buffer[80];
    time(&rawtime); timeinfo = localtime(&rawtime);
    strftime(buffer, sizeof(buffer), "%Y%m%d_%H%M%S", timeinfo);
    std::string filename = carpeta_capturas + "/Radar_" + std::string(buffer) + ".png";
    
    // Pillamos el widget de Qt y lo grabamos nativo en PNG
    QPixmap pantallazo = this->grab();
    if (pantallazo.save(QString::fromStdString(filename), "PNG")) {
        write_log("Instantánea guardada " + fs::path(filename).filename().string());
    }
}

// ===========================================
// BLOQUE DE COMPATIBILIDAD VÍA SLOTS VACÍOS Y FUNCIONES JSON UTILITARIAS
// ===========================================
std::unordered_map<std::string, std::string> ArgusSentinel::cargar_fuentes() {
    std::unordered_map<std::string, std::string> map_f;
    std::ifstream file(archivo_fuentes);
    if (file.is_open()) {
        try {
            json j = json::parse(file);
            for (auto& item : j.items()) map_f[item.key()] = item.value().get<std::string>();
        } catch (...) {}
    }
    return map_f;
}

void ArgusSentinel::guardar_fuentes() {
    try {
        json j(fuentes_guardadas);
        std::ofstream file(archivo_fuentes);
        if (file.is_open()) file << j.dump(4);
    } catch (...) {}
}

void ArgusSentinel::abrir_configurador_fuentes() {
    // Abre el Diálogo especializado como Modulo Bloqueante nativo
    ConfiguradorFuentes dlg(fuentes_guardadas, this);
    dlg.exec();
    
    if (dlg.se_modificaron_fuentes()) {
        guardar_fuentes();       // Pinta los valores RAM sobre JSON
        actualizar_combo_ui();   // Refresca el panel izquierdo de la GUI
        write_log("Libreta de direcciones y rutas C++ sincronizada y salvada.");
    }
}

void ArgusSentinel::seleccionar_carpeta_capturas() {
    QString dir = QFileDialog::getExistingDirectory(this, "Carpeta destino de Alertas", "", QFileDialog::ShowDirsOnly);
    if (!dir.isEmpty()) {
        carpeta_capturas = dir.toStdString();
        lbl_out_path->setText("Carpeta: " + dir);
        write_log("Destino Screenshots -> " + carpeta_capturas);
    }
}

void ArgusSentinel::abrir_visor() {
    if (carpeta_capturas.empty() || !fs::exists(carpeta_capturas)) {
        QMessageBox::warning(this, "Aviso", "Aún no se ha seleccionado ninguna carpeta.");
        return;
    }
    
    // Rastrear todas las capturas (descendentes por fecha)
    std::vector<std::pair<std::string, fs::file_time_type>> archivos;
    for (const auto& entry : fs::directory_iterator(carpeta_capturas)) {
        if (entry.path().extension() == ".png") {
            archivos.push_back({entry.path().filename().string(), entry.last_write_time()});
        }
    }
    std::sort(archivos.begin(), archivos.end(), [](const auto& a, const auto& b) { return a.second > b.second; });
    
    std::vector<std::string> lista_imgs;
    for(const auto& f : archivos) lista_imgs.push_back(f.first);
    
    if (lista_imgs.empty()) {
        QMessageBox::information(this, "Aviso", "La carpeta de capturas está vacía.");
        return;
    }
    
    // Abre cuadro de diálogo bloqueante UI
    VisorCapturas visor(lista_imgs, carpeta_capturas, this);
    visor.exec();
}

void ArgusSentinel::mostrar_info_version() {
    QMessageBox::about(this, "Acerca de Argus Sentinel", "<b>Argus Sentinel (Qt C++)</b><br>Traducción ultra optimizada con renderizado hardware QCustomPlot y amnesia circular.<br><i>V3 - Build Estable</i>");
}

void ArgusSentinel::appendLog(const QString& msg) {}
void ArgusSentinel::guardar_snapshot_circular() {}
void ArgusSentinel::play_alarm_sound() {}
void ArgusSentinel::cerrar_app() { detener_monitor(); QApplication::quit(); }
double ArgusSentinel::get_current_time_sec() { return std::chrono::duration<double>(std::chrono::system_clock::now().time_since_epoch()).count(); }
void ArgusSentinel::closeEvent(QCloseEvent *event) { detener_monitor(); event->accept(); }

// ==========================================
// TEMA CLARO / OSCURO (DINÁMICO)
// ==========================================
void ArgusSentinel::toggleTheme() {
    isLightMode = !isLightMode;
    QString bgColor = isLightMode ? "#F0F0F0" : "#080808";
    QString fgColor = isLightMode ? "#111111" : "#FFFFFF";
    QString gboxColor = isLightMode ? "#008800" : "#00FF00";
    QString lineColor = isLightMode ? "#888888" : "#333333";
    QString inputBg = isLightMode ? "#FFFFFF" : "#111111";
    QString inputBorder = isLightMode ? "#AAAAAA" : "#444444";

    // 1. QMainWindow y Widgets Base
    this->setStyleSheet(
        "QWidget { background-color: " + bgColor + "; color: " + fgColor + "; font-family: 'Segoe UI', Arial; }"
        "QGroupBox { color: " + gboxColor + "; border: 1px solid " + lineColor + "; margin-top: 1ex; border-radius: 5px; font-weight: bold; }"
        "QGroupBox::title { subcontrol-origin: margin; left: 10px; padding: 0 5px; }"
        "QComboBox { background-color: " + inputBg + "; color: " + fgColor + "; border: 1px solid " + inputBorder + "; padding: 2px; }"
        "QLineEdit { background-color: " + inputBg + "; color: " + fgColor + "; border: 1px solid " + inputBorder + "; }"
    );

    // 2. Gráfico 2D
    plot2D->setBackground(QBrush(QColor(isLightMode ? "#FAFAFA" : "#000000")));
    QColor plotColor = QColor(isLightMode ? "#111111" : "#FFFFFF");
    QColor gridColor2D = QColor(isLightMode ? "#CCCCCC" : "#333333");
    
    plot2D->xAxis->setLabelColor(plotColor);
    plot2D->yAxis->setLabelColor(plotColor);
    plot2D->xAxis->setTickLabelColor(plotColor);
    plot2D->yAxis->setTickLabelColor(plotColor);
    plot2D->xAxis->setBasePen(QPen(plotColor));
    plot2D->yAxis->setBasePen(QPen(plotColor));
    plot2D->xAxis->setTickPen(QPen(plotColor));
    plot2D->yAxis->setTickPen(QPen(plotColor));
    plot2D->xAxis->setSubTickPen(QPen(plotColor));
    plot2D->yAxis->setSubTickPen(QPen(plotColor));
    
    plot2D->xAxis->grid()->setPen(QPen(gridColor2D, 1, Qt::DotLine));
    plot2D->yAxis->grid()->setPen(QPen(gridColor2D, 1, Qt::DotLine));
    plot2D->replot();

    // 3. Gráfico 3D
    QColor color3Dbg = QColor(isLightMode ? "#F9F9F9" : "#0A0A0A");
    QColor gridColor3D = QColor(isLightMode ? "#CCCCCC" : "#505050");
    
    graph3D->activeTheme()->setBackgroundColor(color3Dbg);
    graph3D->activeTheme()->setWindowColor(color3Dbg);
    graph3D->activeTheme()->setLabelTextColor(plotColor);
    graph3D->activeTheme()->setGridLineColor(gridColor3D);
}
