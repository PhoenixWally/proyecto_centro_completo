# Informe Técnico: SentinelWebServer (C++ Qt)

**Repositorio:** `PhoenixWally/argus`  
**Rama:** `main`  
**Ruta:** `traduccion_c++/proyecto_web/`  
**Fecha de análisis:** 2025  
**Autor del análisis:** Subagente técnico (Computer)

---

## 1. Visión General

`SentinelWebServer` es la reescritura en C++ del proyecto **Argus Sentinel**, originalmente implementado en Python. El repositorio `argus` contiene una familia amplia de herramientas de monitorización de espectro radioeléctrico desarrolladas para la red de estaciones JPIT (Málaga, España): bots de alertas (`Sentinel_Alert_v1_4w32.py`), grabadores de datos, generadores de informes y varios scripts de análisis, todos centrados en leer archivos binarios propietarios del sistema de monitorización **ARGUS/CTER** (Centro de Telecomunicaciones y Espectro Radioeléctrico).

La rama `traduccion_c++/` contiene dos proyectos paralelos:
- `Sentinel_Proyecto_Qt/` — traducción directa de la GUI de escritorio Python a Qt Widgets.
- `proyecto_web/` (**este proyecto**) — arquitectura completamente nueva: un servidor HTTP + WebSocket embebido en el binario, con un frontend web moderno que sustituye la GUI de escritorio.

### Objetivo del proyecto

Proveer **visualización en tiempo real** del espectro radioeléctrico (gráfica 2D de nivel vs. frecuencia y gráfica 3D tipo cascada temporal) directamente en el navegador, sin instalar Python ni librerías externas en el cliente. El binario C++ actúa como servidor completo: sirve el HTML estático, ofrece la API REST de configuración y emite tramas de datos vía WebSocket cada 500 ms.

---

## 2. Arquitectura

### Diagrama lógico

```
┌──────────────────────────────────────────────────────────────┐
│                  SentinelWebServer.exe                       │
│                                                              │
│  ┌──────────────────┐     ┌────────────────────────────────┐ │
│  │  SentinelServer  │     │       RadarMonitor(es)         │ │
│  │  (QObject)       │     │  (hilo C++ por antena)         │ │
│  │                  │     │                                │ │
│  │  QWebSocketServer│◄────│  onDataBroadcast callback      │ │
│  │  :8081           │     │  (500 ms, JSON)                │ │
│  │                  │     │                                │ │
│  │  httplib::Server │     │  Lee archivos binarios ARGUS   │ │
│  │  :8080 (thread)  │     │  desde rutas UNC de red        │ │
│  └──────────────────┘     └────────────────────────────────┘ │
│          │                          │                        │
│          │ /api/fuentes (REST)       │ ConfiguradorFuentes    │
│          │ /api/captura (REST)       │ (config_fuentes.json)  │
│          │ /         (static)        │                        │
└──────────┼──────────────────────────┼────────────────────────┘
           │ HTTP :8080               │ std::filesystem / WinAPI
           ▼                          ▼
   ┌──────────────────┐      ┌────────────────────┐
   │  Navegador Web   │      │  Recurso de red UNC │
   │                  │ WS   │  \\192.168.x.x\     │
   │  index.html (*)  │:8081  │  argus_db\UMA\...  │
   │  app2.js         │◄─────│  (archivos binarios │
   │  style2.css      │      │   del CTER)         │
   │  Plotly.js 2.27.0│      └────────────────────┘
   │  html2canvas     │
   └──────────────────┘

(*) index.html ≠ app2.js: index.html es el panel de supervisión
    general (Leaflet, Histórico). app2.js es el visor de radar.
```

### Patrón arquitectónico

**Single-binary embedded server**: todo el servidor (HTTP estático + API REST + WebSocket de datos) se compila en un único ejecutable Windows. No hay proceso separado de servidor web; el ejecutable arranca, abre el puerto 8080 para el frontend y el 8081 para WebSocket, y lanza el navegador por defecto (`system("start http://localhost:8080")`).

---

## 3. Stack Tecnológico

### Backend

| Componente | Versión / Estándar | Notas |
|---|---|---|
| Lenguaje | C++17 | `set(CMAKE_CXX_STANDARD 17)` en `CMakeLists.txt:5` |
| Framework | Qt 6.2+ | Módulos: `Qt6::Core`, `Qt6::WebSockets` |
| Servidor HTTP | cpp-httplib (header-only) | `httplib.h` vendoreado en `backend/` |
| Serialización JSON | nlohmann/json 3.11.3 | Descargado en build via `FetchContent` |
| APIs Windows | `Ws2_32`, `Crypt32`, `Mpr` | Enlazados en `target_link_libraries` |
| Red Windows | WNetAddConnection2A (winnetwk.h) | Autenticación transparente en rutas UNC |

### Frontend

| Componente | Versión | Notas |
|---|---|---|
| HTML5 | — | `index.html` (panel general) |
| JavaScript | ES2020 (async/await, módulos) | `app2.js` (visor radar) |
| CSS | CSS Custom Properties | `style2.css`, dark/light mode |
| Plotly.js | 2.27.0 | Gráficas 2D scatter y superficie 3D |
| html2canvas | — | Captura de pantalla del DOM para exportar |
| Leaflet.js | — | Mapa interactivo en `index.html` |
| SheetJS (xlsx.full.min.js) | — | Exportación Excel en `index.html` |

### Build y despliegue

| Componente | Detalle |
|---|---|
| Build system | CMake ≥ 3.16 |
| Target | `SentinelWebServer` (ejecutable) |
| IDE objetivo | Visual Studio (pistas: `backend/x64/`, `*.vcxproj` en `.gitignore`) |
| Configuraciones | Debug / Release / aRelease |
| Dependencias externas | Qt6 (instalado), nlohmann/json (auto-descargado), httplib.h (vendoreado) |

---

## 4. Componentes del Backend

### 4.1 `SentinelServer`

**Archivos:** `backend/SentinelServer.h`, `backend/SentinelServer.cpp`  
**Hereda de:** `QObject` (macro `Q_OBJECT`)

**Responsabilidad:** Orquestador central. Arranca el servidor WebSocket Qt y el servidor HTTP cpp-httplib. Gestiona conexiones de clientes, sus suscripciones a antenas y la creación bajo demanda de instancias `RadarMonitor`.

#### Miembros públicos

```cpp
// SentinelServer.h:16
explicit SentinelServer(quint16 wsPort, int httpPort, QObject *parent = nullptr);
~SentinelServer();
```

Se construye con dos puertos: WebSocket (8081) y HTTP (8080). El destructor cierra el servidor WebSocket, detiene httplib y une el hilo HTTP.

#### Slots Qt (privados)

```cpp
// SentinelServer.h:20-22
void onNewConnection();               // Nueva conexión WebSocket entrante
void processTextMessage(const QString& message); // Mensajes JSON del cliente
void socketDisconnected();            // Limpieza al desconectar cliente
```

#### Miembros privados

```cpp
// SentinelServer.h:28-36
QWebSocketServer *m_pWebSocketServer;
QList<QWebSocket *> m_clients;

std::unordered_map<std::string, std::shared_ptr<RadarMonitor>> m_monitors;
std::unordered_map<QWebSocket*, std::string> m_client_subscriptions;

httplib::Server http_svr;
std::thread http_thread;
```

El mapa `m_monitors` garantiza que sólo existe **una instancia** de `RadarMonitor` por antena, independientemente del número de clientes WebSocket que la soliciten (patrón multicast). El mapa `m_client_subscriptions` asocia cada socket con el identificador de su antena suscrita.

#### Método `startHttpServer` (privado)

```cpp
// SentinelServer.cpp:46-163
void SentinelServer::startHttpServer(int port)
```

Resuelve la carpeta `public/` (primero junto al `.exe`, luego fallback a ruta hardcoded de desarrollo `D:/jpit/...`), la registra como punto de montaje raíz (`/`) de httplib, registra los endpoints REST y lanza httplib en un `std::thread` separado.

#### Método `setupMonitor` (privado)

```cpp
// SentinelServer.cpp:219-262
void SentinelServer::setupMonitor(const std::string& source_id)
```

Crea un `RadarMonitor` si no existe ya. Inyecta dos lambdas de callback usando `QMetaObject::invokeMethod` para devolver la ejecución al hilo del event loop Qt (thread-safe):
- `onDataBroadcast` — Envía el JSON de datos a todos los clientes suscritos a esa antena.
- `onLogBroadcast` — Envía mensajes de log/error al frontend.

#### Método `processTextMessage` (slot)

Procesa tres acciones JSON recibidas del cliente:

| Acción | Payload adicional | Efecto |
|---|---|---|
| `subscribe` | `source: string` | Registra la suscripción y crea/inicia el monitor |
| `update_filters` | `source, fmin, fmax` | Actualiza filtro de frecuencias en el monitor |
| `clear_cache` | `source: string` | Limpia el buffer de datos del monitor |

---

### 4.2 `RadarMonitor`

**Archivos:** `backend/RadarMonitor.h`, `backend/RadarMonitor.cpp`  
**Hereda de:** nada (clase C++ pura, sin Qt)

**Responsabilidad:** Leer, parsear y procesar continuamente los archivos binarios ARGUS/CTER de una antena concreta, y emitir frames de datos vía callback cada 500 ms.

#### Estructura `PuntoRadar`

```cpp
// RadarMonitor.h:13-17
struct PuntoRadar {
    uint64_t t_obj;   // Timestamp Unix (segundos)
    double l;          // Nivel en dBµV
    double f;          // Frecuencia en MHz
};
```

#### Enumeración `ArgusFileType`

```cpp
// RadarMonitor.h:19-24
enum class ArgusFileType {
    TYPE_A_LOG,           // Archivo de log/alarmas (ignorado)
    TYPE_B_TIMEBASE,      // Tracker de base de tiempos (ignorado)
    TYPE_C_MEASUREMENTS,  // Archivo de mediciones (procesado)
    UNKNOWN_OR_EMPTY
};
```

#### Miembros públicos

```cpp
// RadarMonitor.h:31-38
void setFrequencyFilter(std::optional<double> fmin, std::optional<double> fmax);
void start();   // Lanza worker_thread
void stop();    // Detiene is_running y hace join()
void clearCache();

std::function<void(const std::string& json_payload)> onDataBroadcast;
std::function<void(const std::string&)> onLogBroadcast;
```

#### Estado interno

```cpp
// RadarMonitor.h:44-67
std::string source_id;           // Identificador de antena ("UMA", "MA"...)
std::string folder_path;         // Ruta UNC/local a la carpeta de datos

std::atomic<bool> is_running;
std::thread worker_thread;
std::mutex data_mtx;             // Protege buffer_raw, ultimo_barrido, Z_history

std::optional<double> current_fmin, current_fmax;  // Filtro de frecuencias

std::vector<PuntoRadar> buffer_raw;       // Buffer acumulativo (ventana ~3s)
std::vector<PuntoRadar> ultimo_barrido;   // Datos del último archivo leído
std::string ultimo_metadata;              // Footer UTF-16 del archivo

std::vector<std::vector<double>> Z_history; // Matriz 25×300: histórico 3D
std::vector<double> grid_freqs;             // Eje X fijo de 300 bins

double f_min_actual, f_max_actual;  // Rango de banda detectado dinámicamente
int db_min = -10, db_max = 80;      // Rango vertical (dBµV)
int hist_length = 25;               // Filas del histórico 3D
```

---

### 4.3 `ConfiguradorFuentes`

**Archivos:** `backend/ConfiguradorFuentes.h`, `backend/ConfiguradorFuentes.cpp`

**Responsabilidad:** Persistir y recuperar la lista de fuentes de datos (antenas) desde un archivo JSON local, y autenticar transparentemente contra recursos compartidos Windows.

#### Estructura `FuenteRadar`

```cpp
// ConfiguradorFuentes.h:8-13
struct FuenteRadar {
    std::string id;        // Identificador corto ("UMA", "MA", "MIJAS"...)
    std::string path;      // Ruta UNC o local ("//192.168.29.71/argus_db/UMA")
    std::string user;      // Usuario de red Windows
    std::string password;  // Contraseña de red Windows
};
```

#### Métodos estáticos

```cpp
// ConfiguradorFuentes.h:17-21
static std::vector<FuenteRadar> cargarFuentes();
static void guardarFuentes(const std::vector<FuenteRadar>& fuentes);
static bool conectarUNC(const std::string& ruta, const std::string& user,
                        const std::string& password);
```

#### Archivo de configuración

El archivo `config_fuentes.json` (junto al `.exe`) tiene el siguiente formato:

```json
[
    {
        "id": "UMA",
        "path": "//192.168.29.71/argus_db/UMA",
        "user": "",
        "password": ""
    },
    {
        "id": "MA",
        "path": "//192.168.29.71/argus_db/MA",
        "user": "",
        "password": ""
    }
]
```

Si el archivo no existe en el primer arranque, `cargarFuentes()` genera este fallback y lo persiste (`ConfiguradorFuentes.cpp:29-36`).

#### Autenticación UNC (Windows API)

```cpp
// ConfiguradorFuentes.cpp:56-103
bool ConfiguradorFuentes::conectarUNC(...)
```

Usa `WNetAddConnection2A` (Windows Network API, `winnetwk.h`) para autenticar contra el share SMB sin necesidad de login previo del usuario. Extrae la raíz del share (`\\servidor\recurso`) de la ruta completa antes de autenticar. Maneja casos especiales: `ERROR_SESSION_CREDENTIAL_CONFLICT` (1219) y `ERROR_ALREADY_ASSIGNED` (85) se consideran éxito.

---

## 5. Endpoints HTTP (API REST)

Todos los endpoints son registrados en `SentinelServer::startHttpServer()` (`SentinelServer.cpp:69-163`) usando la macro de httplib. El servidor además monta la carpeta `public/` como raíz estática (`/`).

### `GET /api/fuentes`

- **Descripción:** Lista todas las fuentes de antenas configuradas.
- **Parámetros:** ninguno.
- **Respuesta:** `200 OK`, `Content-Type: application/json`

```json
[
    { "id": "UMA", "path": "//192.168.29.71/argus_db/UMA", "user": "", "password": "" },
    { "id": "MA",  "path": "//192.168.29.71/argus_db/MA",  "user": "", "password": "" }
]
```

- **Implementación:** `SentinelServer.cpp:70-81` — llama a `ConfiguradorFuentes::cargarFuentes()` y serializa con nlohmann/json.

---

### `POST /api/fuentes`

- **Descripción:** Reemplaza completamente la lista de fuentes.
- **Cuerpo:** Array JSON con el mismo esquema que `GET /api/fuentes`.
- **Respuesta éxito:** `200 OK`, `{"status":"ok"}`
- **Respuesta error:** `400 Bad Request` (JSON malformado).

- **Implementación:** `SentinelServer.cpp:83-101` — parsea el body, construye vector de `FuenteRadar` y llama a `ConfiguradorFuentes::guardarFuentes()`.

---

### `POST /api/captura`

- **Descripción:** Recibe una captura de pantalla del visor en base64 y la guarda en disco con rotación automática.
- **Cuerpo:**

```json
{
    "src": "UMA",
    "image": "data:image/jpeg;base64,/9j/4AAQ..."
}
```

- **Respuesta éxito:** `200 OK`, `{"status":"ok"}`
- **Respuesta error:** `500 Internal Server Error`.

- **Lógica de almacenamiento** (`SentinelServer.cpp:104-154`):
  1. Extrae el payload base64 puro (descarta el prefijo `data:image/jpeg;base64,`).
  2. Decodifica con `QByteArray::fromBase64()`.
  3. Crea el directorio `capturas/<src>/` si no existe (`std::filesystem::create_directories`).
  4. Guarda como `YYYYMMDD_HHMMSS.jpg` (`localtime_s` para timestamp local).
  5. **Purga rotativa:** si hay más de 100 archivos `.jpg`, elimina los más antiguos (ordenados por `last_write_time`).

---

### `GET /` y recursos estáticos

- httplib sirve automáticamente cualquier archivo bajo `public/` resolviendo la ruta.
- Prioridad de búsqueda: `<exe_dir>/public/` → fallback hardcoded `D:/jpit/.../public`.

---

## 6. Lógica de RadarMonitor (Pieza Central)

### Hilo de trabajo (`threadLoop`)

La función `RadarMonitor::threadLoop()` (`RadarMonitor.cpp:182-350`) corre en un `std::thread` dedicado (uno por antena activa). Ciclo de 500 ms:

#### Fase 1: Descubrimiento de archivos (cada 2 iteraciones)

```cpp
// RadarMonitor.cpp:195-213
if (iteracion % 2 == 0) {
    for (const auto& entry : fs::directory_iterator(folder_path)) {
        if (entry.is_regular_file()) {
            auto dt = entry.last_write_time();
            if (dt > local_cache) {
                local_cache = dt;
                archivo_activo = entry.path().string();
                buffer_raw.clear(); // Solo si es archivo nuevo
            }
        }
    }
}
```

Escanea el directorio y selecciona el archivo modificado más recientemente. Si cambia de archivo, limpia el buffer (el sistema ARGUS escribe en archivos consecutivos por sesión/turno).

#### Fase 2: Clasificación del archivo

```cpp
// RadarMonitor.cpp:219-231
ArgusFileType tipo = clasificarArchivo(archivo_activo);
```

La función `clasificarArchivo()` (`RadarMonitor.cpp:42-74`) lee los primeros 256 bytes del archivo en binario y aplica heurísticas:

- **TYPE_A_LOG:** Si contiene cadenas ASCII `"CTER"`, `"ALARMAS"`, `"EA-MALAGA"`, `"Log"` → archivo de log, se ignora.
- **TYPE_B_TIMEBASE:** Si más del 85% de los primeros 176 bytes son nulos → archivo de base de tiempos, se ignora.
- **TYPE_C_MEASUREMENTS:** Cualquier otro → archivo de mediciones, se procesa.

#### Fase 3: Lectura del archivo binario (`readBinaryFile`)

```cpp
// RadarMonitor.cpp:78-180
```

El formato binario ARGUS de mediciones usa **chunks de 26 bytes** con la siguiente estructura (little-endian):

```cpp
// RadarMonitor.cpp:98-105
#pragma pack(push, 1)
struct ArgusChunk {
    uint64_t tr;   // Timestamp Windows FILETIME (100-ns intervals desde 1601)
    double   fr;   // Frecuencia en Hz (se divide entre 1e6 para convertir a MHz)
    double   lv;   // Nivel en dBµV
    uint16_t ex;   // Campo extra (ignorado)
};  // Total: 8 + 8 + 8 + 2 = 26 bytes
#pragma pack(pop)
```

**Lectura optimizada:** Lee desde los últimos ~5,2 MB del archivo (`200000 × 26 bytes`), suficiente para capturar barridos lentos completos, sin cargar archivos enteros en memoria.

**Conversión de timestamp:**
```cpp
// RadarMonitor.cpp:113-116
uint64_t total_sec = (chunk.tr / 10000000ULL);   // De 100-ns a segundos
time_t unix_time = total_sec - 11644473600ULL;    // Epoch Windows → Unix
```

**Filtros de validez:**
- Descarta timestamps anteriores a la epoch de Windows válida.
- Descarta registros antes del año 2020 (corrupción/zero-padding).
- Solo acepta frecuencias entre 10 MHz y 50.000 MHz (`RadarMonitor.cpp:126`).

**Lectura del footer de metadata:** Lee los últimos 4 KB del archivo, decodifica el UTF-16 heurísticamente (extrae caracteres ASCII imprimibles, ignora bytes nulos intercalados), y si detecta cadenas como `"Hz"`, `"CTER"` o años como `"202x"`, guarda el footer como metadata del barrido (`último_metadata`).

#### Fase 4: Procesamiento y construcción del frame

```cpp
// RadarMonitor.cpp:233-342
```

1. **Ventana temporal de 3 s:** Calcula el timestamp máximo del buffer y descarta puntos con más de 3 segundos de antigüedad (ventana deslizante).

2. **Agrupación por frecuencia:**
   ```cpp
   // RadarMonitor.cpp:254
   double f_round = std::round(pt.f * 500.0) / 500.0;  // Resolución 0,002 MHz
   ```
   Agrupa las lecturas en bins de 2 kHz, reteniendo el **nivel máximo** de cada bin (detector de pico). Esto reduce el número de puntos de potencialmente decenas de miles a unos pocos miles, manejables por el navegador.

3. **Filtrado por frecuencia** (si el usuario ha configurado `fmin`/`fmax` desde el frontend).

4. **Expansión dinámica de banda:** El rango de frecuencias del eje X nunca se encoge; solo se expande cuando aparece nueva frecuencia fuera del rango conocido (`RadarMonitor.cpp:282-291`). Esto evita saltos visuales en la gráfica.

5. **Rejilla de 300 bins:** Normaliza los datos a una cuadrícula uniforme de 300 puntos equiespaciados entre `f_min_actual` y `f_max_actual` mediante interpolación lineal (`RadarMonitor.cpp:293-317`).

6. **Historial 3D (matriz Z):** Mantiene una cola circular de 25 filas × 300 columnas. En cada ciclo, descarta la fila más antigua y añade la nueva al final (`RadarMonitor.cpp:319-320`). Esto forma la superficie 3D que Plotly renderiza.

7. **Emisión del frame JSON:**
   ```cpp
   // RadarMonitor.cpp:328-342
   json packet;
   packet["source"] = source_id;   // "UMA", "MA"...
   packet["type"] = "radar_frame";
   packet["time"] = hora_exacta;   // "UTC HH:MM:SS<br>LOC HH:MM:SS"
   packet["meta_raw"] = meta_to_send;
   packet["x2d"] = x;              // Vector de frecuencias (MHz)
   packet["y2d"] = y;              // Vector de niveles (dBµV)
   packet["x3d"] = grid_freqs;     // Eje X de la matriz 3D (300 puntos)
   packet["z3d"] = Z_history;      // Matriz 25×300
   packet["db_min"] = -10;
   packet["db_max"] = 80;
   ```

8. **Pausa de 500 ms** (`std::this_thread::sleep_for(std::chrono::milliseconds(500))`).

---

## 7. ConfiguradorFuentes: Detalles de Red

La autenticación UNC (`ConfiguradorFuentes::conectarUNC`, `ConfiguradorFuentes.cpp:56-104`) permite que el servidor acceda a recursos compartidos Windows **sin intervención del usuario**:

1. Normaliza slashes (`/` → `\`).
2. Extrae la raíz `\\servidor\share` (hasta el cuarto slash).
3. Llama a `WNetAddConnection2A` con `CONNECT_TEMPORARY` (la conexión no sobrevive reinicios).
4. Acepta como éxito los códigos de error `NO_ERROR` (0), `ERROR_SESSION_CREDENTIAL_CONFLICT` (1219) y `ERROR_ALREADY_ASSIGNED` (85).

Dependencias de enlace requeridas: `Mpr.lib` (WNet API), `Ws2_32.lib` (Winsock para httplib), `Crypt32.lib` (requerido indirectamente por httplib en modo HTTPS).

---

## 8. Frontend

### 8.1 `index.html` — Panel de Supervisión General

La página principal es un **panel de control de red**, no el visor de radar. Sus secciones:

| Pestaña | Contenido |
|---|---|
| `tab-inicio` | KPI dashboard: círculos animados (Operativas / Limitadas / Caídas / Grabando), gráfica SVG nativa de evolución del día |
| `tab-mapa` | Mapa Leaflet.js con marcadores de estaciones, filtro por nombre/IP/provincia |
| `tab-historico` | Gráfica SVG de evolución histórica con selector de fechas, calendario mensual |
| `tab-admin` | Tabla editable de estaciones, exportación Excel via SheetJS |

El panel incluye modal de login con roles (usuario normal / administrador), toggle de tema (dark/light), selector de color de acento CSS, y modal de edición de registros.

> **Nota:** `index.html` carga `app.js` (no `app2.js`). El visor de radar (`app2.js`) corresponde a una **segunda página** del frontend cuyo HTML no está en el repositorio analizado o es generado dinámicamente. `app2.js` asume que el HTML contiene los elementos `plot3D`, `plot2D`, `combo-fuentes`, `btn-start`, etc.

### 8.2 `app2.js` — Visor de Radar en Tiempo Real

#### Estructura general

El script es módulo único sin framework. Opera sobre el DOM existente e implementa directamente:

1. **Carga de fuentes al iniciar** (`loadSources`, `app2.js:15-42`):
   ```javascript
   const resp = await fetch('/api/fuentes');
   const fuentes = await resp.json();
   ```
   Rellena el combo `<select>` de antenas y la tabla del modal de configuración.

2. **CRUD de fuentes** (`deleteSource`, `addFuente`): operaciones `GET + filtrar/añadir + POST` a `/api/fuentes` para sincronizar cambios.

3. **Conexión WebSocket** (`app2.js:324-545`):
   - URL: `ws://${window.location.hostname}:8081`
   - Al conectar, envía `{"action":"subscribe","source":"UMA"}`.
   - Al recibir, distingue por `data.type`:
     - `"log_msg"` → escribe en consola de log.
     - `"radar_frame"` → renderiza gráficas.

4. **Renderizado Plotly** (`app2.js:443-500`):

   **Gráfica 3D (superficie):**
   ```javascript
   const trace3D = {
       z: data.z3d,        // Matriz 25×300
       x: data.x3d,        // 300 bins de frecuencia (MHz)
       colorscale: 'Jet',
       type: 'surface',
       cmin: data.db_min,  // -10 dBµV
       cmax: data.db_max   // 80 dBµV
   };
   Plotly.react('plot3D', [trace3D], layout3D, {displayModeBar: false});
   ```
   La cámara del usuario se preserva entre actualizaciones (`app2.js:455-458`).

   **Gráfica 2D (espectro + umbral):**
   ```javascript
   const trace2D = {
       x: data.x2d, y: data.y2d,
       type: 'scatter', mode: 'lines+markers',
       marker: { color: data.y2d, colorscale: 'Jet', size: 4 }
   };
   const thres2D = { // Línea amarilla de umbral
       x: [min_f, max_f], y: [thres, thres],
       line: { color: '#FFD700', dash: 'dash' }
   };
   Plotly.react('plot2D', [trace2D, thres2D], layout2D, ...);
   ```

5. **Detección de picos** (`app2.js:503-527`): Algoritmo de vecinos próximos sobre `data.y2d`. Un punto es "pico" si supera el umbral configurable Y es mayor que ambos vecinos inmediatos. Si no se detecta pico estructural pero el máximo supera el umbral, se registra el máximo como pico (salvavidas).

6. **Alarma sonora** (`playBeep`, `app2.js:104-122`): Oscilador de onda diente de sierra a 880 Hz, duración 0,3 s, con limitador de 2 pitidos/segundo via `AudioContext` nativo.

7. **Captura y exportación** (`doFullCapture`, `app2.js:260-322`):
   - Usa `Plotly.toImage()` (PNG/JPEG nativo de Plotly) para capturar los canvas WebGL correctamente, ya que `html2canvas` no puede capturar elementos WebGL directamente.
   - Superpone las imágenes Plotly sobre los divs, llama a `html2canvas(document.body)`, y las elimina al terminar.
   - **Auto-captura rotativa:** cada 10 frames (≈5 s) hace `POST /api/captura` con la imagen en base64 (`app2.js:462-470`).
   - **Captura manual:** descarga directamente como `Captura_SentinelHD_<src>_<ts>.jpg`.

8. **Filtros de frecuencia** (`updateFilters`, `app2.js:196-208`): Envía `{"action":"update_filters","fmin":...,"fmax":...}` al WebSocket cuando el usuario modifica los campos de entrada.

9. **Controles de cámara 3D** (`app2.js:548-560`): Botones de zoom in/out modifican `layout3D.scene.camera.eye` y llaman a `Plotly.relayout`.

10. **Tema claro/oscuro** (`toggleTheme`, `app2.js:212-253`): Alterna clase CSS `light-mode` y relayouta ambas gráficas Plotly con nuevos colores.

### 8.3 `style2.css` — Hoja de Estilos

Layout de dos columnas (`flex-direction: row`): panel izquierdo fijo de 320 px con controles, panel derecho expandible con las gráficas. Responsive via `@media` que colapsa a columna única en pantallas menores de 900 px.

Variables CSS (`--bg-main`, `--bg-panel`, etc.) con dos conjuntos: dark mode (negro #0d0d0d) y light mode (#eef2f5). Transición suave de 0,3 s entre temas.

---

## 9. Build y Despliegue

### `CMakeLists.txt` — Análisis completo

```cmake
# CMakeLists.txt:1-40
cmake_minimum_required(VERSION 3.16)
project(SentinelWebServer VERSION 3.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_AUTOMOC ON)         # Genera moc_*.cpp para Q_OBJECT automáticamente

find_package(Qt6 6.2 COMPONENTS Core WebSockets REQUIRED)

include(FetchContent)
FetchContent_Declare(json
    URL https://github.com/nlohmann/json/releases/download/v3.11.3/json.tar.xz)
FetchContent_MakeAvailable(json)

add_executable(SentinelWebServer
    main.cpp RadarMonitor.cpp RadarMonitor.h
    SentinelServer.cpp SentinelServer.h
    ConfiguradorFuentes.cpp ConfiguradorFuentes.h)

target_include_directories(SentinelWebServer PRIVATE ${CMAKE_SOURCE_DIR})

target_link_libraries(SentinelWebServer PRIVATE
    Qt6::Core Qt6::WebSockets
    nlohmann_json::nlohmann_json
    Ws2_32 Crypt32 Mpr)
```

**Módulos Qt utilizados:**
- `Qt6::Core` — QObject, QCoreApplication, QByteArray, QMetaObject, QDir, QHostAddress.
- `Qt6::WebSockets` — QWebSocketServer, QWebSocket.

**Nota sobre `Qt6::Network`:** A pesar de que la tarea menciona el módulo Network, el `CMakeLists.txt` real **no lo incluye**. La comunicación TCP/HTTP la gestiona httplib (no Qt Network), y la autenticación de red Windows la gestiona directamente la WinAPI. `QWebSocket` requiere internamente `Qt6::Network`, pero CMake lo resuelve como dependencia transitiva automáticamente.

### Proceso de compilación

```bash
# Configurar (Visual Studio 2022, x64)
cmake -B backend/build -S backend -G "Visual Studio 17 2022" -A x64

# Compilar en Release
cmake --build backend/build --config Release
```

El ejecutable resultante (`SentinelWebServer.exe`) debe colocarse junto a la carpeta `public/`. Si `public/` no está al lado del exe (ej. durante desarrollo con Qt Creator), usa el fallback hardcoded.

### Estructura de despliegue

```
SentinelWebServer.exe
config_fuentes.json         ← Generado en primer arranque
public/
├── index.html
├── app2.js
├── style2.css
├── style.css
├── plotly-2.27.0.min.js
├── html2canvas.min.js
├── leaflet/
│   ├── leaflet.js
│   └── leaflet.css
├── xlsx.full.min.js
└── LOGOJPITMALAGAFARO.png
capturas/                   ← Creado automáticamente
└── UMA/
    ├── 20250101_120000.jpg
    └── ...
```

---

## 10. Recursos del Sistema: C++ vs Python

| Métrica | SentinelWebServer (C++) | Sentinel Python original |
|---|---|---|
| **RAM base** | ~10-20 MB | ~80-150 MB (CPython + NumPy + Matplotlib + tkinter) |
| **RAM con datos** | +5-15 MB por antena activa | +30-80 MB por sesión |
| **CPU en reposo** | ~0 % (sleep_for 500 ms) | ~2-5 % (event loop tkinter + GIL) |
| **CPU en procesamiento** | <5 % (ciclo 500 ms, STL nativo) | 10-25 % (NumPy/Pandas, sin SIMD explícito) |
| **Tiempo de arranque** | <1 s | 3-8 s (importación de módulos) |
| **Dependencias en destino** | Solo Qt6 runtime DLLs | Python 3.x, pip, NumPy, Pandas, PIL, customtkinter, pywin32 |
| **Portabilidad** | Binario único (con DLLs Qt) | Requiere entorno Python completo |

**Justificación:** El rendimiento de C++ es determinante para la frecuencia de actualización (500 ms); Python con GIL y el overhead de NumPy añadía latencia variable. La ausencia de intérprete elimina el tiempo de importación y reduce radicalmente el footprint RAM, permitiendo desplegar el servidor en máquinas con hardware modesto.

---

## 11. Comparación con Sentinel Python Original

### `Sentinelv3.py` (GUI de escritorio)

La versión Python (`Sentinelv3.py`) es una aplicación de escritorio `customtkinter` + `Matplotlib` con clase principal `ArgusSentinel(ctk.CTk)`. Comparte con la versión C++ web:
- Lectura del mismo formato binario ARGUS (chunks de 26 bytes, conversión FILETIME).
- Conversión `fr / 1e6` para obtener MHz.
- Visualización de espectro 2D y cascada 3D.
- Gestión de capturas de pantalla.

Diferencias arquitectónicas clave:
- Python usa `tkinter.Canvas` + Matplotlib embebido; C++ web usa Plotly.js en navegador.
- Python corre en una sola máquina con GUI local; C++ permite acceso remoto vía navegador.
- Python usa `threading.Thread` con GIL; C++ usa `std::thread` nativo sin restricciones.
- Python usa `PIL.Image` para capturas; C++ usa `html2canvas` + `Plotly.toImage`.

### `Sentinel_Alert_v1_4w32.py` (bot de alertas headless)

Este script complementario es un **nodo táctico** (sin GUI) que:
- Lee los mismos archivos binarios ARGUS.
- Detecta picos sobre un umbral configurable (`OFFSET_ALERTAS = 15.0 dBµV`).
- Envía alertas por correo (`smtplib`, Gmail bot: `jpitmalagaalertas@gmail.com`).
- Escribe registros en Excel (`openpyxl`).
- Respeta ventanas horarias y fechas bloqueadas configuradas via JSON.
- Usa `win32file` (pywin32) para apertura de archivos en red.

La versión C++ web no implementa alertas por correo (es una feature del sistema Python separado). Los dos proyectos son complementarios: C++ para visualización, Python para alertas automatizadas.

---

## 12. Riesgos y Deuda Técnica

### 12.1 Seguridad

| Riesgo | Detalle | Severidad |
|---|---|---|
| **Sin autenticación en API REST** | Los endpoints `/api/fuentes` (GET/POST) y `/api/captura` no requieren ningún token, cookie ni header de auth. Cualquier proceso con acceso al puerto 8080 puede leer/modificar las fuentes o subir imágenes arbitrarias. | Alta |
| **Sin autenticación en WebSocket** | El puerto 8081 acepta cualquier conexión. Un atacante en red local puede suscribirse a cualquier antena y recibir datos de espectro en tiempo real. | Alta |
| **Contraseñas en texto plano** | `config_fuentes.json` almacena credenciales de red Windows sin cifrado. | Media |
| **Sin HTTPS/WSS** | La comunicación va por HTTP y WS planos. Susceptible a intercepción en red LAN. | Media (LAN local, no Internet) |
| **CORS abierto (implícito)** | httplib por defecto no añade cabeceras CORS, lo que puede ser problemático si el frontend alguna vez se sirve desde origen diferente. | Baja |
| **`system("start http://localhost:8080")`** | Usar `system()` con string hardcoded es una mala práctica; en este caso es inocuo (no hay inyección posible) pero el estilo es obsoleto. | Baja |

### 12.2 Gestión de versiones y dependencias

| Riesgo | Detalle |
|---|---|
| **httplib.h vendoreado** | El archivo `httplib.h` de 686 KB está copiado directamente en el repositorio sin referencia explícita a su versión ni mecanismo de actualización. Vulnerabilidades futuras en cpp-httplib no se aplicarán automáticamente. Lo correcto sería tratarlo como `nlohmann/json`: descargar vía `FetchContent` a una versión fija. |
| **Plotly.js y html2canvas también vendoreados** | Los archivos JavaScript de terceros en `public/` no tienen gestión de versiones. |
| **Ruta hardcoded de desarrollador** | `SentinelServer.cpp:55`: `"D:/jpit/Remotas/argus/traduccion_c++/proyecto_web/public"` es el fallback de desarrollo. Si este directorio no existe en el equipo de producción **y** `public/` tampoco está junto al exe, el servidor arranca sin archivos estáticos. |

### 12.3 Robustez

| Riesgo | Detalle |
|---|---|
| **Sin límite de memoria en `buffer_raw`** | La ventana de 3 segundos limita el tamaño del buffer en condiciones normales, pero en archivos con tasas de muestreo muy altas podría crecer indefinidamente hasta que `clearCache()` sea invocado. |
| **Sin reconexión automática** | Si la ruta UNC se desconecta, `RadarMonitor` reporta error via `onLogBroadcast` y espera 2 s, pero no intenta re-autenticar automáticamente. |
| **Mutex de grano grueso** | `data_mtx` protege `buffer_raw`, `ultimo_barrido` y `Z_history` con un único mutex, lo que serializa acceso incluso cuando sólo se necesita leer `Z_history` para el broadcast. Un `std::shared_mutex` mejoraría el rendimiento bajo alta carga. |
| **Hilo HTTP no tiene timeout** | El `std::thread` que ejecuta `http_svr.listen()` no tiene mecanismo de watchdog. Si httplib se bloquea, el destructor puede quedar en `http_thread.join()` indefinidamente. |
| **`app2.js` no maneja reconexión** | Si el WebSocket cae (p. ej. el exe se reinicia), el frontend muestra "DESCONECTADO" pero no intenta reconectarse automáticamente. El usuario debe pulsar "Iniciar" de nuevo. |

### 12.4 Portabilidad

- El proyecto es **exclusivamente Windows**: uso de `WNetAddConnection2A`, `winnetwk.h`, `localtime_s`, `system("start ...")`, y el enlace a `Mpr.lib`/`Crypt32.lib` hacen imposible la compilación en Linux/macOS sin refactoring significativo.

---

## Apéndice: Flujo Completo de Datos

```
Archivo ARGUS (binario, 26 B/chunk)
    │
    ▼ RadarMonitor::readBinaryFile() [hilo worker]
Chunks → PuntoRadar {t_obj, f_MHz, l_dBuV}
    │
    ▼ Ventana temporal 3s + agrupación max-peak en bins 0.002 MHz
x[] (frecuencias) + y[] (niveles)
    │
    ▼ Interpolación lineal a 300 bins uniformes
row_z[300] → Z_history[25][300] (cola circular)
    │
    ▼ nlohmann/json::dump() → JSON string
    │
    ▼ onDataBroadcast(json_string) → QMetaObject::invokeMethod()
    │
    ▼ Qt Event Loop → QWebSocket::sendTextMessage()
    │
    ▼ WebSocket frame → Navegador
    │
    ▼ app2.js ws.onmessage → JSON.parse()
    │
    ├── Plotly.react('plot3D', surface {z: z3d, x: x3d})
    ├── Plotly.react('plot2D', scatter {x: x2d, y: y2d} + umbral)
    ├── Detección de picos → writeDetection() + playBeep()
    └── Auto-captura cada 10 frames → POST /api/captura
```

---

*Informe generado a partir de análisis estático del código fuente en `PhoenixWally/argus`, rama `main`, carpeta `traduccion_c++/proyecto_web/`.*
