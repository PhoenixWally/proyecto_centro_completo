# Informe Técnico — Proyecto `remodeladopy`

**Repositorio:** `PhoenixWally/estacionesjpit` · rama `main` · carpeta `/remodeladopy`  
**Fecha de análisis:** 2025  
**Idioma:** Español

---

## Índice

1. [Visión general](#1-visión-general)
2. [Arquitectura](#2-arquitectura)
3. [Stack tecnológico](#3-stack-tecnológico)
4. [Endpoints HTTP y funciones principales](#4-endpoints-http-y-funciones-principales)
5. [Modelo de datos](#5-modelo-de-datos)
6. [Flujo de datos](#6-flujo-de-datos)
7. [Fuentes externas](#7-fuentes-externas)
8. [Dependencias](#8-dependencias)
9. [Instalación y despliegue](#9-instalación-y-despliegue)
10. [Recursos estimados](#10-recursos-estimados)
11. [Utilidades auxiliares](#11-utilidades-auxiliares)
12. [Riesgos y deuda técnica](#12-riesgos-y-deuda-técnica)

---

## 1. Visión general

**remodeladopy** es un dashboard web de supervisión NOC (*Network Operations Center*) para la **Jefatura Provincial de Inspección de Telecomunicaciones (JPIT)**, cuyo objetivo es mostrar en tiempo real el estado de la red de estaciones remotas CTER (Centro Territorial de Emisiones de Radio).

El panel ofrece:

- **Mapa interactivo** de España con marcadores de color semafórico por estación.
- **KPIs circulares** en la pantalla de inicio: operativas, limitadas, caídas, grabando.
- **Histórico** de estados, con gráfico SVG de evolución y calendario mensual.
- **Administración DB** (solo administradores): tabla editable de estaciones en Excel.
- **Control de PDU/telemandos**: encendido y apagado de puertos de enchufes remotos.

El servidor es Python puro (sin framework externo), escucha en el puerto **4000**, y sirve a la vez la API JSON y los ficheros estáticos del frontend.

---

## 2. Arquitectura

```
┌────────────────────────────────────────────────────┐
│                  Windows 11 (LAN)                  │
│                                                    │
│  server.py  ──────────────────────────────────┐    │
│  (Python 3.13, stdlib http.server, puerto 4000)│    │
│                                                │    │
│  ┌─ Hilos daemon ──────────────────────────┐   │    │
│  │  daemon_ping_ports()   → cada 60 s      │   │    │
│  │  daemon_argus()        → cada 180 s     │   │    │
│  │  daemon_pdu()          → cada 30 min    │   │    │
│  └─────────────────────────────────────────┘   │    │
│                                                │    │
│  ┌─ API HTTP ──────────────────────────────┐   │    │
│  │  GET  /api/state                        │   │    │
│  │  GET  /api/history                      │   │    │
│  │  GET  /api/check?ip=                    │   │    │
│  │  GET  /api/power/status?ip=&auth=       │   │    │
│  │  GET  /api/power/action?ip=&port=…      │   │    │
│  │  POST /api/save-excel                   │   │    │
│  │  GET  /external/<ruta absoluta>         │   │    │
│  │  GET  /* → sirve public/               │   │    │
│  └─────────────────────────────────────────┘   │    │
│                         ▲                       │    │
│                         │ HTTP/JSON             │    │
│  ┌──────── Frontend (SPA estática) ───────┐    │    │
│  │  public/index.html                     │    │    │
│  │  public/app.js   (1 307 líneas, vanilla│    │    │
│  │  public/style.css                      │    │    │
│  │  leaflet/  (Leaflet 1.9.4 vendorizado) │    │    │
│  │  xlsx.full.min.js (SheetJS)            │    │    │
│  │  spain.geojson (mapa offline)          │    │    │
│  └────────────────────────────────────────┘    │    │
└────────────────────────────────────────────────┘    │
                                                       │
  Fuentes externas:                                    │
  ┌─────────────────────────────────────────────────┐ │
  │  importacion.xlsx  (inventario de estaciones)   │ │
  │  Recursos SMB  192.168.29.11 / 192.168.29.71    │ │
  │  PDU/telemandos  (HTTP Basic legacy o PSE-544)  │ │
  │  OpenStreetMap tiles (online, si hay red)       │ │
  └─────────────────────────────────────────────────┘
```

### Componentes principales

| Componente | Fichero | Responsabilidad |
|---|---|---|
| Servidor HTTP + API | `server.py` | Backend completo: hilos, escaneo, endpoints |
| SPA frontend | `public/app.js` | Lógica de mapa, autenticación, KPIs, gráficas |
| Plantilla HTML | `public/index.html` | Estructura de pestañas y modales |
| Estilos | `public/style.css` | Diseño glassmorphism, tema oscuro/claro |
| Estado persistente | `estado_nodos.json` | Caché de estados de nodos entre reinicios |
| Histórico | `historial_nodos.json` | Snapshots cada 60 s, máximo 7 días |
| Inventario | `importacion.xlsx` | Fuente de verdad de estaciones |
| Mapa offline | `spain.geojson` | Contorno de España si no hay internet |

---

## 3. Stack tecnológico

### Backend

| Componente | Detalle |
|---|---|
| **Python** | 3.13 (inferido de las wheels `cp313-cp313-win_amd64`) |
| **HTTP server** | `http.server.ThreadingHTTPServer` + `SimpleHTTPRequestHandler` (stdlib) — **sin Flask ni aiohttp** |
| **Hilos** | `threading.Thread` con `daemon=True` |
| **Excel** | `pandas` + `openpyxl` (`pd.read_excel`) |
| **Ping** | `subprocess` → `ping -n 1 -w 1000 <ip>` (Windows) |
| **Puertos TCP** | `socket.create_connection` con timeout |
| **SMB** | `subprocess` → `net use \\IP\IPC$` + `net view` + acceso directo a UNC path |
| **PDU legacy** | `urllib.request` con Basic Auth a `http://<ip>/config/home_f.html` |
| **PDU PSE-544** | `subprocess` → `curl.exe --anyauth` a `http://<ip>/user/control.ssi` |

El backend **no usa ningún framework externo** (Flask, FastAPI, aiohttp…). Toda la lógica HTTP está en la clase `NOCRequestHandler` que hereda de `SimpleHTTPRequestHandler` de la librería estándar.

```python
# server.py · línea 17
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer

# server.py · línea 402
class NOCRequestHandler(SimpleHTTPRequestHandler):
    timeout = 5
```

### Frontend

| Componente | Versión | Uso |
|---|---|---|
| **Leaflet** | 1.9.4 (vendorizado en `leaflet/`) | Mapa interactivo con marcadores SVG personalizados |
| **SheetJS (XLSX)** | `xlsx.full.min.js` en raíz | Lectura y escritura de `.xlsx` en el navegador |
| **Vanilla JavaScript** | ES2020+ (async/await, fetch, template literals) | Sin framework (no React, Vue ni Angular) |
| **CSS Custom Properties** | Tema oscuro/claro + glassmorphism | Personalización de colores, `backdrop-filter` |
| **SVG nativo** | Generado dinámicamente por `app.js` | Gráficas de evolución e histórico |
| **Google Fonts** | Inter (importada en `style.css`) | Tipografía principal |
| **OpenStreetMap** | Tiles en línea, fallback a `spain.geojson` | Capa base del mapa |

```css
/* style.css · línea 1 */
@import url('https://fonts.googleapis.com/css2?family=Inter:wght@300;400;600;800&display=swap');

/* Tema oscuro por defecto */
:root {
  --bg-color: #0b1120;
  --accent: #3b82f6;
  --success: #3bfa14;
  --danger: #ff1515;
}
```

---

## 4. Endpoints HTTP y funciones principales

### 4.1 Endpoints del servidor (`server.py`)

#### `GET /api/state`
Devuelve el contenido completo del diccionario `global_state` serializado como JSON. Es la "foto" actual del estado de todos los nodos.

```json
{
  "192-168-101-11": {
    "ping": false,
    "rdp": false,
    "vnc": false,
    "argus": {"status": false, "msg": "Apagado"},
    "pdu": {"status": false, "msg": "Offline"}
  },
  …
}
```

**Respuesta:** `Content-Type: application/json`, `Access-Control-Allow-Origin: *`

---

#### `GET /api/history`
Devuelve la lista completa de snapshots históricos en memoria (`history_data`), persistida en `historial_nodos.json`.

```json
[
  {"time": "2026-04-21 18:47", "ok": 6, "warn": 0, "danger": 87, "rec": 0},
  …
]
```

---

#### `GET /api/check?ip=<IP>`
Devuelve el estado actual del nodo (desde `global_state`) y lanza en paralelo un hilo `force_check()` que actualiza ping, RDP, VNC y Argus SMB para esa IP concreta. Útil para el botón "Comprobar Ahora" del frontend.

```python
# server.py · líneas 434-453
sid = safe_id(ip)
state = global_state.get(sid, {})
self.wfile.write(json.dumps(state).encode('utf-8'))
threading.Thread(target=force_check).start()
```

---

#### `GET /api/power/status?ip=<IP_PDU>&auth=<user:pass,[…]>`
Consulta el estado de los puertos de un PDU/telemando. Acepta múltiples credenciales separadas por coma. Soporta dos tipos de PDU:

- **legacy**: `GET http://<ip>/config/home_f.html` con Basic Auth, parsea con regex los `socket(id, "name", estado)`.
- **pse544**: `curl.exe --anyauth http://<ip>/user/control.ssi`, parsea la respuesta JSON simplificada.

```python
# server.py · líneas 190-255
def get_pdu_status(ip, user, password):
    # Prueba pse544 primero, luego legacy
    # Cachea el tipo en pdu_cache[ip]
```

---

#### `GET /api/power/action?ip=<IP>&port=<N>&action=<0|1|r>&auth=<creds>`
Ejecuta una acción de encendido/apagado/reset sobre un puerto de PDU:

- **legacy**: `POST http://<ip>/config/home_f.html` con body `P<port>=<action>`.
- **pse544**: `curl.exe -d "CMD=0_<port>_<accion>" http://<ip>/user/control.cgi`.

---

#### `POST /api/save-excel`
Recibe el cuerpo de la petición como bytes y lo escribe directamente en `importacion.xlsx`. El frontend usa SheetJS para serializar los datos modificados y los envía aquí.

```python
# server.py · líneas 525-542
file_data = self.rfile.read(content_length)
with open(EXCEL_FILE, 'wb') as f:
    f.write(file_data)
```

---

#### `GET /external/<ruta_absoluta>`
Sirve cualquier fichero del sistema de archivos local dada su ruta absoluta (p. ej., `C:/Users/jpit/Documents/usuarios.xlsx`). Se usa para cargar el Excel de autenticación de usuarios cuando su ruta está fuera de la carpeta del proyecto.

```python
# server.py · líneas 502-511
ext_path = urllib.parse.unquote(parsed_url.path.replace('/external/', ''))
if os.path.exists(ext_path):
    with open(ext_path, 'rb') as f:
        self.wfile.write(f.read())
```

> **Riesgo:** Este endpoint sirve cualquier fichero del sistema sin autenticación (ver sección 12).

---

#### `GET /*` — Ficheros estáticos
El handler redirige cualquier ruta no reconocida a `public/<ruta>`. Si la ruta es `/`, sirve `public/index.html`.

---

### 4.2 Hilos daemon del servidor

| Hilo | Función | Intervalo | Qué hace |
|---|---|---|---|
| `daemon_ping_ports` | Ping + puertos TCP | 60 s | Itera nodos del Excel, hace ping Windows, comprueba puertos 3389 (RDP) y 5900 (VNC). Guarda snapshot histórico. |
| `daemon_argus` | Comprobación SMB | 180 s | Solo si ping OK: autentica en IPC$, descubre la carpeta Argus, mide crecimiento del fichero más reciente en 6 s. |
| `daemon_pdu` | Estado telemandos | 1 800 s (30 min) | Para cada estación con IP de telemando, consulta el PDU y almacena el estado de los puertos. |

```python
# server.py · líneas 562-568
threading.Thread(target=daemon_ping_ports, daemon=True).start()
threading.Thread(target=daemon_argus, daemon=True).start()
threading.Thread(target=daemon_pdu, daemon=True).start()
start_server()
```

---

### 4.3 Funciones principales del frontend (`app.js`)

| Función | Línea aprox. | Descripción |
|---|---|---|
| `loadConfig()` | 23 | Carga `config.json` vía fetch |
| `loadUsers()` | 35 | Carga usuarios desde JSON o Excel (ruta absoluta → `/external/`) |
| `authenticateUser(user, pass)` | 115 | Búsqueda simple en array `usersData` |
| `login / logout` | 121 / 135 | Gestión de sesión + `sessionStorage` |
| `updateUIBasedOnPermissions()` | 143 | Muestra/oculta elementos `.admin-only` |
| `initMap()` | 249 | Crea mapa Leaflet, prueba tiles OSM, cae en `spain.geojson` |
| `loadOfflineGeoJSON()` | 277 | Carga `spain.geojson` como capa GeoJSON |
| `updateMapFromExcel()` | 293 | Itera `excelData`, coloca marcadores SVG según lat/lon |
| `checkNodeStatus(ip, …)` | 438 | Llama a `/api/check?ip=`, actualiza DOM del popup |
| `renderStatusToDOM(safeId, status)` | 472 | Pinta ✅/❌ en los spans del popup de cada marcador |
| `updateMarkerColor(safeId, status)` | 491 | Cambia icono del marcador: verde/naranja/rojo/azul |
| `startBackgroundScanner()` | 797 | Sondea `/api/state` cada 2 s y actualiza KPIs y marcadores |
| `fetchHistory()` | 538 | Llama a `/api/history` y dispara `drawHistoryChart()` |
| `drawHistoryChart()` | 552 | Dibuja gráfico SVG bezier del día actual |
| `renderLargeHistory()` | 631 | Gráfico SVG lineal histórico con filtro de fechas |
| `renderCalendar()` | 733 | Genera HTML del calendario mensual con medias diarias |
| `checkPduStatus(ipTelemando, …)` | 893 | Llama a `/api/power/status`, renderiza botones ON/OFF |
| `powerAction(ip, port, action, …)` | 933 | Llama a `/api/power/action`, confirma con `confirm()` |
| `loadExcelAutomatically()` | 976 | Carga `importacion.xlsx` con SheetJS via `/importacion.xlsx` |
| `renderExcelTable()` | 1150 | Genera tabla HTML dinámica de estaciones |
| `openModal / saveRowChanges` | 1173 / 1194 | Edición en modal + autoguardado vía `downloadExcel()` |
| `downloadExcel()` | 1206 | SheetJS serializa datos y hace POST a `/api/save-excel` |
| `toggleTheme()` | 1271 | Alterna `data-theme=light/dark`, persiste en `localStorage` |
| `setAccentColor(hex)` | 1282 | Cambia `--accent` CSS dinámicamente |

---

## 5. Modelo de datos

### 5.1 `estado_nodos.json` — Estado en tiempo real

**Clave:** IP con puntos reemplazados por guiones (`192.168.101.11` → `"192-168-101-11"`).

```json
{
  "192-168-101-11": {
    "ping": false,
    "rdp": false,
    "vnc": false,
    "argus": {
      "status": false,
      "msg": "Apagado"
    },
    "pdu": {
      "status": false,
      "msg": "Offline"
    }
  }
}
```

Cuando el PDU está activo y operativo, la clave `pdu` tiene la forma:

```json
"pdu": {
  "status": true,
  "type": "legacy",
  "ports": [
    {"id": 1, "name": "Argus PC", "status": 1},
    {"id": 2, "name": "Router",   "status": 0}
  ],
  "used_user": "Administrador",
  "used_pass": "admin"
}
```

Los posibles valores del campo `msg` de `argus` son: `"Apagado"`, `"No Auth"`, `"Carpeta No Hallada"`, `"Carpeta Vacía"`, `"Sin Acceso Archivo"`, `"Timeout SMB"`, `"Grabando"`, `"Parado"`.

El fichero está estructurado como un **objeto plano** (no array), con ~93 nodos observados en producción. Tamaño: ~18 KB.

---

### 5.2 `historial_nodos.json` — Snapshots temporales

**Estructura:** Array de objetos JSON, uno por ciclo de 60 s.

```json
[
  {"time": "2026-04-21 18:47", "ok": 6, "warn": 0, "danger": 87, "rec": 0},
  {"time": "2026-04-21 18:53", "ok": 5, "warn": 1, "danger": 87, "rec": 0},
  …
]
```

| Campo | Tipo | Significado |
|---|---|---|
| `time` | `string` `"YYYY-MM-DD HH:MM"` | Marca temporal del snapshot |
| `ok` | `int` | Nodos con ping + RDP + VNC activos |
| `warn` | `int` | Nodos con ping pero solo RDP o solo VNC |
| `danger` | `int` | Nodos sin ping o sin RDP/VNC |
| `rec` | `int` | Nodos con Argus grabando (`argus.status == true`) |

Retención máxima: **10 080 snapshots** (7 días × 24 h × 60 min), gestionado en memoria y volcado al fichero. El ejemplo de producción muestra 93 nodos en red: 6 OK y 87 en `danger`.

---

### 5.3 `importacion.xlsx` — Inventario de estaciones

Inferido de `server.py` (`get_nodes_from_excel()`, líneas 57-107) y `importar_telemandos.py`:

| Columna (variantes aceptadas) | Tipo | Uso |
|---|---|---|
| `IP estacion` / `IP PC` / `IP` | string | IP del equipo de grabación Argus |
| `Estacion` / `Ubicacion` / `Nombre` | string | Nombre de la estación (mostrado en popup) |
| `Latitud` / `Lat` | decimal | Coordenada geográfica |
| `Longitud` / `Lon` | decimal | Coordenada geográfica |
| `Usuario` / `User` / `Usr` | string | Credencial SMB del PC Argus |
| `Contraseña` / `Pass` / `Password` | string | Contraseña SMB |
| `IP Telemando` / `Telemando` / `PDU` | string | IP del PDU de alimentación |
| `Usuario Telemando` | string | Credencial HTTP del PDU |
| `Contraseña Telemando` | string | Contraseña del PDU |
| `Telefono JPIT` / `Telefono` | string | Teléfono de contacto JPIT |
| `Correo JPIT` / `Correo` / `Email` | string | Email de contacto JPIT |

La normalización de nombres de columna se realiza eliminando acentos y convirtiendo a minúsculas (función `normalize_col`, `server.py` línea 53). Las credenciales admiten múltiples valores separados por `/` o `,` para prueba por rotación.

La fuente original es `d:/jpit/Remotas/estacionesJPIT/Direcciones IP Red CTER_actualizado.xlsx`, un libro con múltiples hojas por provincia, del que `importar_telemandos.py` extrae las IPs de telemando.

---

## 6. Flujo de datos

```
importacion.xlsx
       │
       │  get_nodes_from_excel() — pandas.read_excel()
       │  cada 60 s (daemon_ping_ports)
       │  cada 180 s (daemon_argus)
       │  cada 30 min (daemon_pdu)
       ▼
  Lista de nodos en memoria
       │
       ├── check_ping(ip)          → subprocess ping -n 1 -w 1000
       ├── check_port(ip, 3389)    → socket TCP
       ├── check_port(ip, 5900)    → socket TCP
       │
       ├── check_argus(ip, user, pass)
       │       → net use \\IP\IPC$   (autenticación SMB Windows)
       │       → net view \\IP        (descubrimiento share Argus)
       │       → cmd dir \\IP\Share   (fichero más reciente)
       │       → os.path.getsize() × 2 separados 6 s (¿creció?)
       │
       └── get_pdu_status(ip_telemando, …)
               → curl.exe / urllib GET al PDU HTTP
               ▼
       global_state[safe_id] ──────────────── estado_nodos.json
               │
               │  Cada 60 s: snapshot histórico
               ▼
       history_data[]  ─────────────────────── historial_nodos.json
               │
               │  Endpoints REST
               ▼
       Frontend (app.js)
               │
               ├── /api/state  →  startBackgroundScanner() cada 2 s
               │       → updateMarkerColor(), renderStatusToDOM()
               │       → KPIs (ok, warn, danger, recording)
               │
               ├── /api/history → drawHistoryChart(), renderCalendar()
               │
               ├── /api/check?ip= → popup individual "Comprobar Ahora"
               │
               ├── /api/power/status → checkPduStatus() → botones ON/OFF
               │
               └── /importacion.xlsx → SheetJS → mapa + tabla DB
```

El frontend hace polling a `/api/state` **cada 2 segundos** (`setInterval(syncState, 2000)`, `app.js` línea 889), lo que garantiza que el mapa siempre muestre el último estado calculado por los hilos del backend. Los marcadores se colorean dinámicamente sin recargar la página.

---

## 7. Fuentes externas

### 7.1 Servidores SMB (Argus)

Identificados en `test_conexion_smb.py` (línea 15):

```python
IPS_OBJETIVO = ["192.168.29.11", "192.168.29.71"]
PUERTO_SMB = 445
```

Son los servidores de grabación SMB de referencia para pruebas de conectividad. En producción, cada estación del Excel tiene su propia IP. La autenticación se realiza vía `net use \\<IP>\IPC$` (Windows), y la verificación de Argus detecta si el fichero más reciente en la carpeta compartida ha crecido en 6 segundos (indicativo de grabación activa).

El script `test_conexion_smb.py` prueba adicionalmente los puertos SMB 139 (NetBIOS Session), 137 y 138 (NetBIOS Name/Datagram).

### 7.2 PDU / Telemandos HTTP

Dos modelos soportados:

| Tipo | Endpoint de estado | Endpoint de control | Auth |
|---|---|---|---|
| **Legacy** | `GET http://<ip>/config/home_f.html` | `POST http://<ip>/config/home_f.html` body `P<port>=<0|1>` | Basic (Base64) |
| **PSE-544** | `GET http://<ip>/user/control.ssi` | `POST http://<ip>/user/control.cgi` body `CMD=0_<port>_<accion>` | `--anyauth` (curl) |

El servidor detecta automáticamente el tipo y lo cachea en `pdu_cache[ip]` para no reprobar en cada consulta.

### 7.3 Excel de inventario

- **Fichero de inventario:** `importacion.xlsx` (en la raíz de `remodeladopy/`)
- **Fuente original:** `d:/jpit/Remotas/estacionesJPIT/Direcciones IP Red CTER_actualizado.xlsx`
- **Excel de usuarios:** `C:\Users\jpit\Documents\usuarios.xlsx` (configurado en `config.json`)

### 7.4 Tiles de mapa

- **Online:** `https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png` (OSM) — se prueba con un fetch de `0/0/0.png`; si falla, se usa el modo offline.
- **Offline:** `spain.geojson` (polígono de España incluido en el repositorio) — renderizado como capa GeoJSON con Leaflet.

---

## 8. Dependencias

### 8.1 Backend Python (`requirements.txt`)

```
pandas
openpyxl
```

Solo dos dependencias declaradas. El resto es stdlib de Python 3.13.

### 8.2 Wheels incluidas en `paquetes/` (instalación offline, Windows)

| Fichero wheel | Versión | Plataforma | Función |
|---|---|---|---|
| `numpy-2.4.4-cp313-cp313-win_amd64.whl` | 2.4.4 | Python 3.13, Windows 64-bit | Dependencia de pandas |
| `pandas-3.0.2-cp313-cp313-win_amd64.whl` | 3.0.2 | Python 3.13, Windows 64-bit | Lectura de Excel, manipulación de datos |
| `openpyxl-3.1.5-py2.py3-none-any.whl` | 3.1.5 | Multiplataforma | Motor xlsx para pandas |
| `et_xmlfile-2.0.0-py3-none-any.whl` | 2.0.0 | Multiplataforma | Dependencia de openpyxl |
| `python_dateutil-2.9.0.post0-py2.py3-none-any.whl` | 2.9.0 | Multiplataforma | Parsing de fechas para pandas |
| `six-1.17.0-py2.py3-none-any.whl` | 1.17.0 | Multiplataforma | Compat Python 2/3 (requerido por dateutil) |
| `tzdata-2026.1-py2.py3-none-any.whl` | 2026.1 | Multiplataforma | Base de datos de zonas horarias |

> **Nota importante:** El uso de wheels compiladas `cp313-cp313-win_amd64` confirma que el entorno objetivo es **Windows 11 / Windows 10 de 64 bits con Python 3.13** sin acceso a internet durante la instalación. Esto es coherente con un entorno corporativo restrictivo. La instalación se realiza con `pip install --no-index --find-links=paquetes/ pandas openpyxl`.

### 8.3 Frontend JavaScript (vendorizado)

| Librería | Versión | Fichero | Inclusión |
|---|---|---|---|
| Leaflet | 1.9.4 | `leaflet/leaflet.js` + `leaflet.css` | `<script src="leaflet/leaflet.js">` |
| SheetJS (XLSX) | (full build) | `xlsx.full.min.js` | `<script src="xlsx.full.min.js">` |
| Google Fonts Inter | — | Importado en CSS | `@import url(…)` — requiere internet |

Todos los recursos JS/CSS de Leaflet y SheetJS están **vendorizados** en el repositorio, lo que permite la operación sin internet (excepto Google Fonts, que es decorativa).

---

## 9. Instalación y despliegue

> **Nota:** El fichero `INSTALACION.md` del repositorio hace referencia a Node.js y a `servidor.js` en el puerto 3000, lo que indica que es documentación de una **versión anterior** (Node.js) del proyecto. El servidor actual es `server.py` en Python, escuchando en el **puerto 4000**. Los pasos de instalación Python correctos se infieren del código fuente y de los wheels incluidos.

### Instalación correcta (Python — versión actual)

**Requisitos del sistema:**
- Windows 10/11 de 64 bits
- Python 3.13 instalado y añadido al PATH
- Red LAN con acceso a las IPs de las estaciones (192.168.x.x)
- `curl.exe` disponible en el PATH (incluido en Windows 10+ desde 2018)

**Pasos:**

```powershell
# 1. Clonar o copiar la carpeta remodeladopy al equipo
# (por ejemplo: D:\jpit\Remotas\estacionesJPIT\remodeladopy\)

# 2. Instalar dependencias offline (desde la carpeta del proyecto)
cd D:\jpit\Remotas\estacionesJPIT\remodeladopy
pip install --no-index --find-links=paquetes pandas openpyxl

# 3. Verificar que importacion.xlsx existe en la carpeta raíz
# (o generarlo con importar_telemandos.py si procede)

# 4. Iniciar el servidor
python server.py
```

**Salida esperada:**

```
[+] Scanner Ping/Ports Iniciado (cada 60s)
[+] Scanner Argus SMB Iniciado (cada 180s)
[+] Scanner PDU Iniciado (cada 30 min)
📡 Monitor CTER (Python) activo en http://localhost:4000
📡 Red Local: http://192.168.x.x:4000
```

**Acceso:** Abrir `http://localhost:4000` o `http://<ip-local>:4000` desde cualquier equipo de la LAN.

### Configuración de autenticación

Editar `config.json` para apuntar al Excel de usuarios:

```json
{
  "auth_excel_path": "C:\\Users\\jpit\\Documents\\usuarios.xlsx",
  "auth_excel_sheet": "Usuarios"
}
```

El Excel de usuarios debe tener columnas: `Usuario`, `Contraseña`, `Rol` (`admin` o `viewer`), `Nombre`. El sistema implementa un **mecanismo de integridad** silencioso: si ninguna celda del fichero contiene el string `"cter"` (código ASCII 99,116,101,114), la lista de usuarios se vacía y nadie puede autenticarse como administrador (`app.js`, líneas 76-85).

### Diagnóstico SMB

```powershell
python test_conexion_smb.py
```

Prueba TCP al puerto 445 (SMB) en `192.168.29.11` y `192.168.29.71`, además de los puertos NetBIOS 137, 138 y 139.

---

## 10. Recursos estimados

| Recurso | Estimación | Justificación |
|---|---|---|
| **RAM** | 80–150 MB | Python 3.13 base ~30 MB; pandas con un Excel de ~100 filas carga ~30-40 MB; los JSONs de estado (~18 KB) y historial (máx. ~2 MB en 7 días) son mínimos; threading overhead bajo |
| **CPU (idle)** | <1% | Los hilos duermen la mayor parte del tiempo; el polling de `/api/state` del frontend (cada 2 s) genera peticiones triviales de solo JSON |
| **CPU (pico)** | 5–15% | Durante `daemon_argus`: cada nodo espera 6 s y hace llamadas `subprocess`; con ~93 nodos a 180 s de intervalo, las comprobaciones se solapan |
| **I/O disco** | Mínimo | Se escribe `estado_nodos.json` en cada cambio de nodo; `historial_nodos.json` cada 60 s. Ambos ficheros son pequeños (<2 MB). |
| **Red** | Moderado | Ping ICMP + TCP a puertos 3389/5900 por nodo cada 60 s; conexiones SMB con `net use` cada 180 s (potencialmente costosas por timeout); PDU HTTP cada 30 min |
| **Navegador** | ~50–80 MB adicionales | Leaflet + SheetJS son librerías completas; el polling cada 2 s es barato pero constante |

**Conclusión:** La aplicación es liviana para un NOC de ~100 nodos. El cuello de botella potencial es el hilo `daemon_argus`, cuya espera de 6 s por nodo implica que, con 93 nodos, cada ciclo de 180 s puede tener un bloque de ~9 minutos de tiempo acumulado de espera (secuencial). Si el número de nodos aumentara significativamente, se recomienda paralelizar ese hilo con un pool de threads.

---

## 11. Utilidades auxiliares

### 11.1 `fix_mojibake.py`

Corrige emojis con **mojibake** (corrupción UTF-8 interpretada como latin-1) en `public/app.js`. Se ejecuta manualmente cuando el fichero JS pierde la codificación correcta.

```python
# fix_mojibake.py · líneas 8-35
replacements = {
    'ðŸŒ ':  '🌐',   # Globe
    'â˜Žï¸ ': '☎️',   # Teléfono
    'âœ…':   '✅',   # Check
    'ðŸŸ¢':  '🟢',   # Verde
    'ðŸŸ ':  '🟠',   # Naranja
    'ðŸ"´':  '🔴',   # Rojo
    '?? Modo': '🌙 Modo',
    # … (23 sustituciones en total)
}
```

Aplica las sustituciones literales sobre el fichero y lo reescribe en UTF-8. El script también normaliza variantes del emoji de "modo oscuro" que aparecen truncadas.

### 11.2 `fix_ui.py`

Script de **parcheo de interfaz** más complejo. Realiza cuatro tipos de modificaciones sobre `public/app.js`:

1. **Corrección adicional de mojibake** (superposición parcial con `fix_mojibake.py`):
   ```python
   text.replace('â ³', '⏳')
   text.replace('GrabaciÃ³n', 'Grabación')
   ```

2. **Corrección de colores de gráficas** para modo claro (los colores hardcodeados como `rgba(255,255,255,…)` son invisibles en fondo blanco):
   ```python
   text.replace('fill="rgba(255,255,255,0.4)"', 'fill="var(--text-secondary)"')
   text.replace('stroke="rgba(255,255,255,0.05)"', 'stroke="rgba(128,128,128,0.2)"')
   ```

3. **Inyección de nuevos campos** en el popup del mapa: añade las variables `telefono` y `correo` leyendo las columnas `"telefono jpit"` y `"correo jpit"` del Excel:
   ```python
   new_def = old_def + '\n        const telefono = findValueInRow(estacion, ["telefono jpit", …])'
   ```

4. **Sustitución del bloque HTML del popup**: reemplaza `<div><b>🌐 IP:</b> ${ip}</div>` por un panel con teléfono, correo e IP con estilos glassmorphism.

> Este script actúa como un **sistema de parches** sobre el JS principal. La necesidad de estos dos scripts indica que el fichero `app.js` ha pasado por varias ediciones en entornos con codificación inconsistente (probablemente mezcla de editores Windows y Linux).

### 11.3 `importar_telemandos.py`

Script de **migración puntual** de datos. Lee el libro maestro CTER (`Direcciones IP Red CTER_actualizado.xlsx`) con múltiples hojas por provincia, extrae las correspondencias `IP_PC → IP_Telemando` agrupando por "Lugar", y las escribe en `importacion.xlsx`. También rellena las credenciales de telemando con el valor por defecto `Administrador/admin`.

```python
# importar_telemandos.py · líneas 5-6
source_file = r'd:/jpit/Remotas/estacionesJPIT/Direcciones IP Red CTER_actualizado.xlsx'
target_file = r'd:/jpit/Remotas/estacionesJPIT/remodelado/importacion.xlsx'
```

Este script **no forma parte del flujo en tiempo real**; es una utilidad de administración para sincronizar el inventario cuando el libro CTER se actualiza.

---

## 12. Riesgos y deuda técnica

### 12.1 Credenciales en texto claro

Las credenciales de los nodos (usuario/contraseña SMB, usuario/contraseña PDU) se almacenan en `importacion.xlsx` sin ningún tipo de cifrado. El fichero es accesible vía HTTP a través del propio servidor en `http://localhost:4000/importacion.xlsx` (servido como fichero estático de la carpeta raíz).

El Excel de usuarios también se sirve vía `/external/C:/Users/jpit/Documents/usuarios.xlsx`, lo que expone las contraseñas de acceso al panel.

### 12.2 Endpoint `/external/` sin autenticación

El endpoint `GET /external/<ruta>` sirve **cualquier fichero del sistema de archivos** accesible por el proceso Python, sin requerir autenticación ni verificar que la ruta sea segura:

```python
# server.py · líneas 503-511
ext_path = urllib.parse.unquote(parsed_url.path.replace('/external/', ''))
if os.path.exists(ext_path):
    with open(ext_path, 'rb') as f:
        self.wfile.write(f.read())
```

Cualquier usuario de la LAN podría leer `C:\Windows\System32\...` o cualquier fichero del equipo navegando a `http://localhost:4000/external/C:/ruta/al/fichero`.

### 12.3 CORS totalmente abierto

Todos los endpoints de la API devuelven `Access-Control-Allow-Origin: *`, lo que permite peticiones cross-origin desde cualquier origen. En una red interna sin exposición a internet esto es aceptable, pero es un riesgo si el servidor fuera accesible desde el exterior.

### 12.4 Autenticación solo en el frontend

El sistema de roles (admin/viewer) está implementado **únicamente en JavaScript** (`app.js`). El backend no comprueba permisos en ningún endpoint. Cualquier persona con acceso a la red puede llamar a `POST /api/save-excel` y sobreescribir el inventario de estaciones, o a `GET /api/power/action` para apagar puertos PDU, sin autenticarse.

### 12.5 Credenciales de PDU en query string

Las llamadas al PDU incluyen las credenciales en la URL:

```javascript
// app.js · línea 944
fetch(`/api/power/action?ip=${ip}&port=${port}&action=${action}&auth=${authTelemando}`)
```

Esto expone credenciales en logs de acceso del servidor y en el historial del navegador.

### 12.6 Frontend monolítico

`app.js` tiene 1 307 líneas con toda la lógica en un único fichero vanilla JS sin módulos, sin bundler y sin tests. Las funciones de acceso a datos (`findValueInRow`) están duplicadas en al menos tres lugares distintos del fichero (líneas 298, 801, 956, 1102).

### 12.7 Rutas absolutas hardcodeadas de Windows

Tanto `importar_telemandos.py` como `config.json` usan rutas absolutas Windows hardcodeadas (`D:\jpit\...`, `C:\Users\jpit\...`). Esto hace la aplicación no portátil y difícil de desplegar en otro equipo sin modificar el código.

```python
# importar_telemandos.py · línea 5
source_file = r'd:/jpit/Remotas/estacionesJPIT/Direcciones IP Red CTER_actualizado.xlsx'
```

```json
// config.json · línea 2
"auth_excel_path": "C:\\Users\\jpit\\Documents\\usuarios.xlsx"
```

### 12.8 Documentación de instalación desactualizada

`INSTALACION.md` describe la instalación con Node.js y `servidor.js` (puerto 3000), que corresponde a la **versión anterior** del proyecto. El servidor actual es Python (`server.py`, puerto 4000). Las credenciales de prueba documentadas (`admin/admin123`, `visor/visor123`) pueden no corresponder al Excel de usuarios en producción.

### 12.9 `daemon_argus` secuencial con espera de 6 s por nodo

La comprobación de Argus duerme 6 segundos por nodo para detectar crecimiento del fichero. Con 93 nodos, el tiempo total de un ciclo puede superar los 9 minutos, lo que significa que el intervalo de 180 s declarado no se respeta si hay muchos nodos activos con SMB lento.

### 12.10 Servidor `http.server` no apto para producción

`http.server` es la implementación de referencia de Python y no está optimizada para producción. No soporta HTTPS nativo, no tiene limitación de tamaño de petición ni protección contra ataques de denegación de servicio. Para un despliegue más robusto se recomendaría Waitress o Gunicorn (este último solo en Linux) con proxy inverso nginx/caddy, o migrar a Flask/FastAPI con un servidor WSGI/ASGI.

---

*Informe generado mediante análisis estático del código fuente del repositorio `PhoenixWally/estacionesjpit`, rama `main`, carpeta `remodeladopy`.*
