# Informe Técnico: dashboard2.py — Sistema ARGUS de Monitorización de Remotas

**Repositorio:** `PhoenixWally/bot_remotas` (rama `master`)  
**Fecha de análisis:** junio 2025  
**Archivo analizado:** `dashboard2.py` (1 408 líneas, ~105 KB)

---

## 1. Visión General

`dashboard2.py` es el **centro de control web unificado** del sistema denominado **ARGUS**, un conjunto de herramientas desarrollado por el equipo JPIT (Junta de Perímetro de Inspección Técnica) de Málaga para la monitorización de **estaciones remotas de medición de espectro radioeléctrico**. El sistema recibe el nombre informal de "remotas" en el código y en los comentarios.

El propósito del proyecto es doble:

1. **Extracción a demanda**: Un operador selecciona una estación, un rango de fechas y horas, y el sistema extrae datos de medición (binarios propietarios o CSVs intermedios), los analiza, genera gráficas y exporta informes (PNG, Excel, CSV, Word).
2. **Gestión de alertas automáticas**: El operador configura, a través del propio dashboard, las reglas de envío de alertas (horario, días bloqueados, destinatarios) que rigen el comportamiento de los bots autónomos (`botnuevo.py`, `Bot_Analisis_MA.py`, etc.).

El ecosistema completo del proyecto consta de:

| Componente | Función |
|---|---|
| `dashboard2.py` | Frontend web + backend Flask; extracción interactiva y configuración |
| `dashboard.py` | Versión anterior (prototipo sin autenticación ni visor de archivos) |
| `botnuevo.py` | Bot autónomo de análisis para la estación UMA |
| `Bot_Analisis_MA.py` | Bot autónomo para la estación MA (Málaga Aeropuerto) |
| `Bot_Analisis_UMA.py` / `UMA2.py` | Variantes del bot para otras estaciones UMA |
| `renderizador_bot.py` | Renderizador CLI invocable para procesar CSVs producidos por el extractor C++ |
| `web1.py` | Versión intermedia del dashboard (sin autenticación, sin visor de archivos) |
| `src/main.cpp` | Extractor binario en C++17, lee los mismos archivos que los bots Python |
| `compile_all.py` | Script de empaquetado con PyInstaller |

---

## 2. Arquitectura y Componentes

### 2.1 Patrón arquitectónico

`dashboard2.py` implementa un **monolito Python de servidor local** con el siguiente patrón:

```
[Arranque main]
    └── threading.Thread(target=abrir_navegador, daemon=True)  # Abre el browser
    └── app.run(host='0.0.0.0', port=8082)                    # Flask monohilo (modo desarrollo)

[Flask Routes]
    GET  /                  → home()           → render HTML_TEMPLATE (inline)
    GET  /login             → login()
    POST /login             → valida credenciales en usuarios.json
    GET  /logout            → logout()
    GET  /api/fuentes       → lee fuentes_globales.json
    POST /api/fuentes       → persiste fuentes_globales.json
    POST /api/config        → lee el JSON de configuración de una estación concreta
    POST /api/config/save   → persiste el JSON de config de la estación
    POST /api/extraer       → MOTOR PRINCIPAL: lee binarios/CSVs, procesa, genera salidas
    POST /api/files/list    → lista directorio (local o red SMB)
    POST /api/files/get_folders  → carpetas para el modal "mover a histórico"
    POST /api/files/move_history → mueve carpeta a /historico (solo admin)
    GET  /api/files/serve   → sirve archivo por ruta absoluta
    GET  /api/files/download → fuerza descarga de archivo
    POST /api/files/preview → vista previa de CSV/Excel/Word/texto
    GET  /api/usuarios      → CRUD usuarios (solo admin)
    POST /api/usuarios      → CRUD usuarios (solo admin)
    GET  /api/browse_folder → abre diálogo Tkinter en el servidor para seleccionar carpeta
    GET  /api/browse_file   → abre diálogo Tkinter en el servidor para seleccionar JSON
    GET  /favicon.ico       → sirve el icono .ico
```

### 2.2 Diagrama lógico de módulos

```
dashboard2.py
├── CONFIGURACIÓN GLOBAL
│   ├── ARCHIVO_FUENTES_GLOBAL = "fuentes_globales.json"
│   ├── ARCHIVO_USUARIOS       = "usuarios.json"
│   ├── PUERTO                 = 8082
│   ├── OFFSET_ALERTAS         = 15.0  (dB sobre piso de ruido)
│   └── MAX_PICOS              = 30
│
├── GESTORES AUXILIARES
│   ├── conectar_red_windows(ruta, usr, pwd)   → net use via subprocess
│   ├── cargar_usuarios()                       → lee/crea usuarios.json
│   └── enviar_correo_reporte(...)              → SMTP Gmail + adjuntos .docx
│
├── MIDDLEWARE DE SEGURIDAD
│   ├── @before_request → requerir_login()
│   ├── /login  → login()
│   └── /logout → logout()
│
├── MOTOR MATEMÁTICO Y GRÁFICO
│   ├── calcular_metricas(df, offset_db, max_picos)
│   │   ├── p5_ruido = np.percentile(df['L'], 5)
│   │   ├── umbral   = p5_ruido + offset_db
│   │   ├── df_maxhold → max hold por frecuencia (groupby + max)
│   │   └── detección de picos cronológica (ventana ±60s, ±0.5 MHz)
│   └── dibujar_panel_lateral(ax_side, umbral, df_picos, offset_db)
│
├── FRONTEND (HTML_TEMPLATE — string inline ~550 líneas)
│   ├── CSS custom dark-mode (variables CSS, grid layouts)
│   ├── DataTables.js v1.13.6 (CDN)
│   ├── jQuery 3.7.0 (CDN)
│   └── JS vanilla (tabs, modales, AJAX fetch, calendario, visor de archivos)
│
└── BACKEND ROUTES (Flask)
    ├── home() / login() / logout()
    ├── api_fuentes()          → CRUD fuentes_globales.json
    ├── api_get_config()       → lee config_local.json de la estación
    ├── api_save_config()      → persiste config_local.json
    ├── api_extraer()          → MOTOR PRINCIPAL (≈420 líneas, l.978–1394)
    │   ├── Extracción binaria → struct.unpack('<QddH', b) × N bloques de 26 bytes
    │   ├── Extracción CSV     → pd.read_csv + filtrado por fecha + concat
    │   ├── Generación gráficas → matplotlib (2D tiempo, 2D frecuencia, 3D superficie)
    │   ├── Exportación Excel/CSV
    │   ├── Reporte Word       → python-docx (solo si hay alertas)
    │   ├── Envío email        → enviar_correo_reporte()
    │   └── Copia a red SMB    → shutil.copytree con reintentos
    ├── api_files_list()       → os.listdir con autenticación SMB previa
    ├── api_files_preview()    → pandas/mammoth/python-docx según extensión
    └── browse_folder/file()   → Tkinter filedialog en el servidor
```

---

## 3. Tecnologías y Stack

### 3.1 Lenguaje

Python 3.x. La versión mínima requerida es **Python 3.9** (uso de `datetime.timezone`, f-strings con `=`, `pd.ExcelWriter` con `engine='openpyxl'`). El script `compile_all.py` usa el Windows Python Launcher (`py --list-paths`), lo que confirma entorno **Windows**.

### 3.2 Web (servidor)

| Librería | Uso |
|---|---|
| `flask` | Servidor web. Se usa `render_template_string` con el HTML embebido, `session` (cookies firmadas), `send_file`, `jsonify`, `redirect`, `url_for`. Puerto 8082. |
| `werkzeug` (implícita) | Subyace a Flask; su logger se silencia (`log.setLevel(logging.ERROR)`). |

El servidor arranca en modo **desarrollo** (`app.run(host='0.0.0.0', port=8082)`) — no usa Gunicorn ni ningún servidor WSGI de producción.

### 3.3 Análisis de datos

| Librería | Uso |
|---|---|
| `pandas` | Lectura de CSV (`read_csv`), concatenación, filtrado temporal, groupby, pivot_table, ExcelWriter |
| `numpy` | `np.percentile`, `np.meshgrid`, validación `math.isnan` |
| `openpyxl` | Motor de escritura Excel (vía pandas ExcelWriter) |

### 3.4 Visualización

| Librería | Uso |
|---|---|
| `matplotlib` (modo `Agg`) | Gráficas 2D tiempo, 2D frecuencia, 3D superficie. `Figure`, `GridSpec`, `mdates`, `ticker`, `Axes3D` |
| `mpl_toolkits.mplot3d` | Gráfica de superficie 3D (waterfall) con `plot_surface` |

### 3.5 Documentos

| Librería | Uso | Importación |
|---|---|---|
| `python-docx` | Generación de reportes Word con imágenes embebidas | Condicional (`HAS_DOCX`) |
| `mammoth` | Conversión DOCX → HTML para vista previa web | Condicional (`HAS_MAMMOTH`) |

### 3.6 Red y sistema

| Módulo | Uso |
|---|---|
| `subprocess` | `net use` para montar recursos SMB de Windows |
| `shutil` | `copytree` (copia de carpetas a red con reintentos), `move` (archivar a histórico) |
| `struct` | Desempaquetado de registros binarios de 26 bytes: `struct.unpack('<QddH', b)` |
| `smtplib` / `email` | Envío de correos con adjuntos vía SMTP Gmail (puerto 587, STARTTLS) |
| `webbrowser` | Apertura automática del navegador al arrancar |
| `threading` | Hilo daemon para abrir el navegador con retardo de 1.5 s |
| `tkinter` | Sólo para los endpoints `/api/browse_folder` y `/api/browse_file` (diálogos de selección de ruta en la máquina servidor) |

### 3.7 Inferencia de requirements.txt

```
flask>=2.0
pandas>=1.5
numpy>=1.24
matplotlib>=3.7
openpyxl>=3.1
python-docx>=0.8.11       # opcional
mammoth>=1.6              # opcional
```

---

## 4. Funciones Principales

### 4.1 `conectar_red_windows(ruta, usr, pwd)` — líneas 59–74

```python
def conectar_red_windows(ruta, usr, pwd):
    if not usr or not pwd or not str(ruta).startswith('\\\\'): return
    if os.path.exists(ruta): return
    ruta_base = os.path.dirname(ruta)  # extrae carpeta base, no archivo
    comando = f'net use "{ruta_base}" {pwd} /user:{usr}'
    try:
        subprocess.run(comando, shell=True, stdout=subprocess.DEVNULL,
                       stderr=subprocess.DEVNULL, timeout=5)
    except: pass
```

**Qué hace:** Monta un recurso compartido SMB de Windows usando `net use` para que el servidor Python pueda acceder a rutas UNC (`\\servidor\recurso`). **Cómo:** Llama al comando nativo de Windows con un timeout de 5 segundos. **Por qué así:** Es el mecanismo nativo más fiable en entornos Windows sin instalar librerías adicionales (como `smbprotocol`). El comentario en el código indica que es el "formato exacto original del Bot_Analisis_UMA".

### 4.2 `cargar_usuarios()` — líneas 76–83

```python
def cargar_usuarios():
    if os.path.exists(ARCHIVO_USUARIOS):
        with open(ARCHIVO_USUARIOS, 'r', encoding='utf-8') as f:
            return json.load(f)
    default_users = {"admin": {"password": "Argus2026", "role": "admin"}}
    with open(ARCHIVO_USUARIOS, 'w', ...) as f:
        json.dump(default_users, f, indent=4)
    return default_users
```

**Qué hace:** Lee `usuarios.json` o lo crea con credenciales por defecto. **Riesgo:** El fichero real contiene cinco contraseñas en texto plano (véase sección 11).

### 4.3 `enviar_correo_reporte(destinatarios_str, ruta_salida, prefijo, titulo, hay_alertas)` — líneas 85–146

Envía un correo SMTP a una lista de destinatarios (separados por coma) adjuntando **únicamente los archivos `.docx`** del directorio de salida, respetando un límite de 20 MB. Usa STARTTLS contra `smtp.gmail.com:587` con una cuenta Google fija hardcodeada.

### 4.4 `requerir_login()` — líneas 151–155 (middleware `@before_request`)

```python
@app.before_request
def requerir_login():
    if request.endpoint not in ['login', 'static'] and not session.get('autenticado'):
        if request.path.startswith('/api/'):
            return jsonify({"error": "Acceso denegado."}), 401
        return redirect(url_for('login'))
```

Protege todas las rutas excepto `/login` y recursos estáticos. Devuelve 401 JSON para endpoints de API y redirección 302 para páginas.

### 4.5 `calcular_metricas(df, offset_db, max_picos=30)` — líneas 184–211

```python
def calcular_metricas(df, offset_db, max_picos=30):
    p5_ruido = np.percentile(df['L'], 5)       # piso de ruido: percentil 5
    umbral = p5_ruido + offset_db              # umbral = piso + 15 dB
    df_maxhold = df.groupby(df['F'].round(4))['L'].max().reset_index()

    df_over = df[df['L'] >= umbral].sort_values(by='T', ascending=True)
    picos_detectados = []
    for _, row in df_over.iterrows():
        es_nuevo = True
        for p in picos_detectados:
            # Ventana temporal: ±60 s; ventana frecuencial: ±0.5 MHz
            if abs((row['T'] - p['T']).total_seconds()) < 60 and abs(row['F'] - p['F']) < 0.5:
                es_nuevo = False
                if row['L'] > p['L']:  # actualiza al APEX del evento
                    p['T'] = row['T']; p['L'] = row['L']; p['F'] = row['F']
                break
        if es_nuevo and len(picos_detectados) < max_picos:
            picos_detectados.append({'T': row['T'], 'F': row['F'], 'L': row['L']})
```

**Qué hace:** Detecta hasta 30 picos de señal en el espacio (tiempo × frecuencia) por encima del umbral de ruido +15 dB. **Algoritmo:** Recorre las mediciones en orden cronológico; agrupa eventos que ocurren dentro de una ventana de 60 segundos y 0.5 MHz, conservando únicamente el máximo (apex). **Por qué:** Evita contar el mismo pico físico múltiples veces al tener varias mediciones consecutivas. Diferencia clave respecto a los bots: en los bots (`botnuevo.py`, `Bot_Analisis_MA.py`) el recorrido es por nivel descendente (`sort_values by='L'`), mientras que en `dashboard2.py` es cronológico para garantizar la "primera aparición temporal" del evento.

### 4.6 `dibujar_panel_lateral(ax_side, umbral, df_picos, offset_db)` — líneas 213–230

Dibuja el panel derecho de cada gráfica: leyenda y tabla monospace de hasta 30 picos (`Nº | HH:MM:SS | Frec(MHz) | Nivel`). Usa `ax.axis('off')` para ocultar los ejes cartesianos y emplea coordenadas normalizadas (0–1) para el posicionamiento del texto.

### 4.7 `api_extraer()` — líneas 978–1394 (≈420 líneas, ruta POST `/api/extraer`)

Es la función más crítica del sistema. Su flujo se detalla en la sección 5.

### 4.8 `api_files_preview()` — líneas 911–933

```python
@app.route('/api/files/preview', methods=['POST'])
def api_files_preview():
    path = request.json.get('path')
    ext = os.path.splitext(path)[1].lower()
    if ext == '.csv':
        df = pd.read_csv(path, sep=';', decimal=',', on_bad_lines='skip',
                         nrows=500, encoding='utf-8', comment='#')
        return jsonify({"html": df.to_html(table_id="tabla-preview", ...)})
    elif ext == '.xlsx':
        df = pd.read_excel(path, nrows=500)
        ...
    elif ext == '.docx':
        if HAS_MAMMOTH:
            with open(path, "rb") as docx_file:
                result = mammoth.convert_to_html(docx_file)
            return jsonify({"html": result.value})
```

Genera previsualización HTML de hasta 500 filas para CSV/Excel o convierte DOCX a HTML con `mammoth`. Activa DataTables en el frontend mediante la clase CSS `display nowrap`.

---

## 5. Flujo de Datos y Ejecución

### 5.1 Arranque

```
python dashboard2.py
    │
    ├─ app = Flask(__name__)                          # instancia Flask
    ├─ app.secret_key = "phoenix_argus_super_secret_key_2026"
    ├─ threading.Thread(target=abrir_navegador, daemon=True).start()
    │       └─ time.sleep(1.5) → webbrowser.open("http://127.0.0.1:8082")
    └─ logging.getLogger('werkzeug').setLevel(ERROR)  # silencia logs HTTP
    └─ app.run(host='0.0.0.0', port=8082)             # BLOQUEA el proceso principal
```

El proceso ocupa el proceso principal con el servidor Flask y lanza un hilo daemon únicamente para abrir el navegador. **No hay hilos adicionales permanentes**: el servidor Flask en modo desarrollo es **monohilo y síncrono**, lo que significa que cada petición bloquea el servidor hasta completarse. Una extracción larga (varios días de datos binarios) bloquea cualquier otra petición concurrente.

### 5.2 Autenticación

1. El navegador carga `/` → redirigido a `/login` (sin sesión).
2. `POST /login`: compara usuario/contraseña contra `usuarios.json` (comparación directa en texto plano, sin hashing).
3. En éxito: `session['autenticado'] = True`, `session['usuario_actual']`, `session['rol']`.
4. La `secret_key` hardcodeada firma las cookies de sesión.

### 5.3 Extracción de datos — `api_extraer()` (POST `/api/extraer`)

```
Frontend JS → fetch('/api/extraer', {payload})
    │
    └─ Conectar red SMB (net use) si es ruta UNC
    └─ Parsear dt_inicio y dt_fin
    └─ Determinar carpeta de salida
    │       ├─ Si ruta UNC → carpeta temporal LOCAL primero
    │       └─ Si local   → carpeta en ruta_res directamente
    │
    ├─ MODO "binario":
    │   └─ os.listdir(ruta_bin) → filtrar por mtime/ctime ±1 día
    │   └─ Por cada archivo binario:
    │       ├─ Leer 512 bytes cabecera (sniffer heurístico)
    │       │   ├─ Descartar si head decodifica a UTF-16LE con palabras clave de log
    │       │   └─ Descartar si comienza con 32 bytes nulos (índice de tiempo)
    │       └─ Leer en bloques de 260 000 bytes (26 × 10 000):
    │           └─ struct.unpack('<QddH', b) → (timestamp, freq_Hz, level_dB, extra)
    │               ├─ Filtrar: timestamp fuera de rango FILETIME válido
    │               ├─ Filtrar: |level| > 300 o NaN en freq/level
    │               └─ Convertir FILETIME UTC → datetime local España
    │
    ├─ MODO "csv":
    │   └─ os.walk(ruta_res) → buscar .csv recursivamente
    │   └─ Filtro rápido: nombre del fichero/ruta contiene fecha en formatos YYMMDD/YYYYMMDD/DD_MM_YYYY
    │   └─ Filtro profundo: leer primera línea no-comentario, parsear fecha
    │   └─ pd.read_csv(sep=';', decimal=',', comment='#')
    │   └─ Renormalizar columnas → ['T', 'F', 'L']
    │   └─ pd.concat + dropna + sort_values('T')
    │
    └─ GENERACIÓN DE ENTREGABLES (por tramos temporales):
        └─ Segmentar en tramos de N horas (1/2/3/4/6/8/12/24)
        └─ Por cada tramo:
            ├─ calcular_metricas(df_tramo, 15.0, 30)
            ├─ Exportar CSV (formato europeo: sep=';', decimal=',')
            ├─ Exportar Excel (.xlsx, máx 1 048 575 filas)
            ├─ Gráfica 2D tiempo (15×7 pulgadas, 150 dpi, PNG)
            ├─ Gráfica 2D frecuencia (15×7 pulgadas, 150 dpi, PNG)
            └─ Gráfica 3D superficie (16×9 pulgadas, 150 dpi, PNG)
                └─ pivot_table(freq='10s') → np.meshgrid → plot_surface(cmap='turbo')
        └─ Reporte Word (.docx) por día, con imágenes de tramos con alertas
        └─ plt.close('all')
        └─ Envío email (si solicitado)
        └─ Si era UNC: shutil.copytree(local_temp → UNC_final) con 3 reintentos
```

### 5.4 Visor de archivos

El operador navega la estructura de carpetas de la estación seleccionada. Cada click llama a `POST /api/files/list`, que monta el recurso SMB si es necesario y devuelve listado de carpetas y archivos. El frontend renderiza iconos por extensión y permite previsualización (imágenes con flechas de navegación, PDF en `<iframe>`, CSV/Excel con DataTables, DOCX con mammoth).

---

## 6. Dependencias

| Módulo | Externo | Función |
|---|---|---|
| `flask` | Sí | Servidor web, routing, sesiones, JSON responses |
| `pandas` | Sí | Lectura CSV/Excel, manipulación DataFrames, pivot_table, GroupBy |
| `numpy` | Sí | Percentil, meshgrid, validación NaN |
| `matplotlib` | Sí | Generación de gráficas PNG en modo headless (Agg) |
| `openpyxl` | Sí | Motor de escritura Excel (usado vía `pandas.ExcelWriter`) |
| `python-docx` | Sí (opcional) | Creación y escritura de reportes Word con imágenes |
| `mammoth` | Sí (opcional) | Conversión DOCX a HTML para previsualización web |
| `struct` | Stdlib | Desempaquetado de binarios de 26 bytes |
| `smtplib` / `email` | Stdlib | Envío de correos SMTP con adjuntos MIME |
| `subprocess` | Stdlib | Ejecución de `net use` para montar SMB |
| `shutil` | Stdlib | Copia y movimiento de directorios |
| `threading` | Stdlib | Hilo daemon para apertura del navegador |
| `webbrowser` | Stdlib | Apertura del navegador del sistema |
| `tkinter` | Stdlib | Diálogos de selección de ruta (solo en `/api/browse_*`) |

---

## 7. Fuentes Externas y Configuración

### 7.1 `fuentes_globales.json`

Contiene la lista de estaciones remotas configuradas. Cada entrada define:

```json
{
    "nombre": "UMA",
    "ruta_res": "\\\\192.168.29.12\\jpit_malaga_central\\RESULTADOS_BOT\\UMA\\ANALISIS_UMA",
    "ruta_bin": "\\\\192.168.29.12\\jpit_malaga_central\\RESULTADOS_BOT\\UMA\\DATOS",
    "ruta_json": "\\\\192.168.29.12\\jpit_malaga_central\\RESULTADOS_BOT\\UMA\\config_local.json",
    "usr_red": "jpit",
    "pwd_red": "malaga"
}
```

Los tres campos de ruta corresponden a:
- `ruta_res`: carpeta de resultados procesados (CSVs, PNGs, XLSXs, DOCXs). El visor de archivos parte de aquí.
- `ruta_bin`: carpeta de datos binarios crudos generados por la estación remota (archivos sin extensión con el formato propietario ARGUS).
- `ruta_json`: ruta al JSON de configuración de alertas de esa estación (`config_local.json`).

Actualmente hay tres estaciones configuradas: **UMA**, **MA**, **UMA2**, todas en `\\192.168.29.12`.

### 7.2 `usuarios.json`

Mapa de usuario → `{password, role}`. Roles: `admin` y `user` (técnico). Actualmente cinco usuarios, todos con rol `admin`. **Las contraseñas se almacenan en texto plano.**

### 7.3 `web_sources.json`

Usado por `web1.py` (versión anterior). Contiene rutas de `ruta_bin` y `ruta_res` de UMA, apuntando a `\\192.168.29.71\argus_db` como origen binario (la propia estación remota).

### 7.4 `web_contacts.json`

Lista de contactos en formato alternativo (probablemente resto de una versión anterior de web1). Contiene un único contacto técnico con email `@digital.gob.es`.

### 7.5 `config_local.json` (por estación, en red)

No está en el repo (se genera/edita vía el dashboard). Estructura deducida del código:

```json
{
    "alertas_activas": true,
    "hora_inicio": "08:00",
    "hora_fin": "20:00",
    "fechas_bloqueadas": ["2025-12-25", "2026-01-01"],
    "contactos": [
        {"nombre": "Técnico", "email": "tecnico@dsic.es", "grupo": "Tecnicos", "activo": true}
    ]
}
```

### 7.6 Infraestructura de red detectada

| IP | Rol |
|---|---|
| `192.168.29.71` | Estación remota UMA (origen binario `argus_db`) |
| `192.168.29.11` | Servidor intermedio (DESTINO_LOCAL de botnuevo.py) |
| `192.168.29.12` | Servidor central JPIT Málaga (`jpit_malaga_central`) |
| `adswdfs-02.dsic.es` | Servidor DSIC (red de consulta de Bot_Analisis_MA) |
| `smtp.gmail.com:587` | SMTP de alertas |

### 7.7 Formato binario ARGUS

Los archivos binarios de la estación no tienen extensión. Cada registro válido ocupa **26 bytes** con la estructura (little-endian):

```
Offset  Tipo        Descripción
 0      uint16      padding (0x0000, 0x0012 o 0x1200)
 2      uint64      Windows FILETIME (100 ns desde 1601-01-01 UTC)
10      double      Frecuencia en Hz
18      double      Nivel en dBµV (o dBm)
```

La cabecera se comprueba heurísticamente: se descartan archivos que decodifican como UTF-16LE con palabras clave de log ("CTER", "EA-MALAGA", "ALARMAS") o que comienzan con 32 bytes nulos (índices de tiempo). El footer del archivo contiene metadata en texto ASCII ("Tipo de med", parámetros de medición).

Esta misma estructura está documentada en `src/main.cpp` (struct `ArgusMeasure`, `#pragma pack(push,1)`).

---

## 8. Recursos (Memoria y CPU)

### 8.1 Memoria RAM

| Escenario | Estimación | Justificación |
|---|---|---|
| Servidor en reposo | 80–120 MB | Flask + pandas + matplotlib importados; el HTML template (~550 líneas) es una string estática en memoria |
| Extracción típica (1 día, modo CSV) | 200–400 MB | Un día de medición puede tener del orden de 500 000–2 000 000 filas × 3 columnas (T, F, L). Con dtypes `datetime64`, `float64`, `float64`, cada fila ocupa ~40 bytes → 80–320 MB de DataFrame puro. La concatenación con `pd.concat` genera una copia temporal adicional. |
| Extracción de binarios (1 semana) | 400–800 MB | Los bots autónomos procesan tramos de 1 hora; el dashboard puede solicitarse semanas completas en una sola petición. El buffer de lectura en bloques de `26 × 10 000 = 260 000 bytes` es pequeño, pero la lista acumulada `data` crece proporcional al número de puntos. |
| Gráfica 3D (pivot_table) | +100–300 MB adicionales | El `pivot_table(freq='10s')` genera una matriz densa (tiempo × frecuencia). Con 1 hora a 10 s = 360 filas y, por ejemplo, 2000 bins de frecuencia → 720 000 celdas float64 ≈ 5,5 MB por tramo. Multiplicado por tramos paralelos en memoria es manejable, pero el `np.meshgrid` duplica el almacenamiento. |

**Rango total estimado en producción:** 200–600 MB, con picos de hasta 1 GB en extracciones largas de binarios.

### 8.2 CPU

| Operación | Estimación |
|---|---|
| Reposo (servidor Flask en espera) | < 1 % (proceso inactivo) |
| Parseo binario (modo binario, 1 semana) | 50–90 % durante varios minutos (bucle Python puro sobre millones de registros de 26 bytes). El bucle `for i in range(0, len(c), 26)` con `struct.unpack` en Python puro es la operación más costosa del sistema. |
| Renderizado matplotlib 150 dpi (15×7 pulgadas) | Pico de 80–100 % durante 2–5 s por figura |
| Generación de 3D surface | Pico de 100 % durante 5–15 s; `plot_surface` es la operación más pesada de matplotlib |

**Núcleos efectivos:** 1 (Flask en modo desarrollo es monohilo; matplotlib `Agg` también es monohilo). No hay paralelismo.

### 8.3 I/O de red

El montaje SMB (`net use`) es síncrono y bloquea el hilo Flask. Copiar un directorio de resultados por red (`shutil.copytree`) con 3 reintentos puede tardar decenas de segundos en redes lentas; durante ese tiempo el servidor no responde a otras peticiones.

---

## 9. Comparativa dashboard.py vs dashboard2.py

| Característica | `dashboard.py` | `dashboard2.py` |
|---|---|---|
| Puerto | 8083 | 8082 |
| Nombre interno | "PHOENIX" | "ARGUS" |
| Autenticación | **No** (sin login) | **Sí** (login + sesión Flask + roles admin/user) |
| Gestión de usuarios | No | Sí (CRUD via `/api/usuarios`, solo admin) |
| Visor de archivos | No | **Sí** (pestaña "Visor de Archivos": navegador de directorios, preview inline de PNG/PDF/CSV/Excel/Word) |
| Archivo histórico | No | Sí (modal "Mover a Histórico", solo admin) |
| Motor de extracción real | **No** (la `api_extraer` hace `time.sleep(3)` y retorna OK falso) | **Sí** (implementación completa, ~420 líneas) |
| Cálculo de métricas | No (el motor matemático no está en `dashboard.py`) | Sí (`calcular_metricas`, `dibujar_panel_lateral`) |
| Gráficas 3D | Checkbox presente pero no implementado | Implementado |
| Exportación Word | Checkbox presente pero no implementado | Implementado (python-docx) |
| Envío email desde dashboard | Checkbox presente pero no implementado | Implementado (SMTP Gmail) |
| Modo CSV vs binario | Ambos (radio buttons) pero sin implementación | Ambos implementados |
| Segmentación por tramos | Checkbox + selector de horas | Implementado |
| Previsualización DOCX | No | Sí (mammoth) |
| DataTables.js | No | Sí (CDN, para previsualizar CSV/Excel) |
| Brower de rutas en servidor | No | Sí (`/api/browse_folder`, `/api/browse_file` con Tkinter) |
| Conexión SMB robusta | Parcial (en código de bot pero no integrada) | Integrada en `conectar_red_windows()` usada en múltiples rutas |
| Copia local→red con reintentos | No | Sí (3 reintentos con `time.sleep(2)`) |
| `send_file` / download | No | Sí (`/api/files/serve`, `/api/files/download`) |
| Longitud | 752 líneas | 1 408 líneas |

En resumen: `dashboard.py` es un prototipo de interfaz gráfica sin backend funcional. `dashboard2.py` es la versión completa de producción.

---

## 10. Empaquetado y Despliegue

### 10.1 `compile_all.py` — PyInstaller

El script `compile_all.py` empaqueta `dashboard2.py` en un único ejecutable Windows (`.exe`) mediante **PyInstaller**:

```python
cmd = [
    target_python, "-m", "PyInstaller",
    "--noconfirm",
    "--onefile",           # ejecutable único autoextraíble
    "--clean",
    f"--icon={icon_path}", # icono analisis-de-datos.ico
    f"--add-data={icon_path};.",  # incluye el .ico en el bundle
    f"--name={out_name_no_ext}",  # Ej: "dashboard2v1.3"
    basename
]
```

El nombre de salida tiene el sufijo `v1.3` hardcodeado (línea 77). El script primero detecta las versiones de Python instaladas usando el Windows Python Launcher (`py --list-paths`) y permite al operador elegir cuál usar. Instala PyInstaller automáticamente si no está presente.

El `.gitignore` excluye `build/`, `dist/`, `*.spec`, lo que confirma que los binarios compilados no se versionan.

### 10.2 Nuitka (mención en el repo)

El repo contiene referencias a `nuitka-crash-report.xml` en el `.gitignore` implícitamente (no está en los archivos listados pero el enunciado lo menciona). Nuitka fue considerado como alternativa a PyInstaller para compilar a C nativo, pero aparentemente se abandonó en favor de PyInstaller dado el `compile_all.py` actual.

### 10.3 `CMakeLists.txt` + `src/main.cpp`

El extractor C++ (`main.cpp`) se compila con CMake (C++17). No se integra directamente en `dashboard2.py` — es una herramienta independiente que produce CSVs que luego pueden consumirse con `renderizador_bot.py` o directamente en el dashboard (modo CSV).

```cmake
project(BotRemotas VERSION 1.0)
set(CMAKE_CXX_STANDARD 17)
add_executable(BotRemotas src/main.cpp)
```

La estructura `ArgusMeasure` en `main.cpp` (líneas 20–27) es la referencia canónica del formato binario propietario:

```cpp
#pragma pack(push, 1)
struct ArgusMeasure {
    uint16_t pad;        // 2 bytes padding
    uint64_t timestamp;  // 8 bytes FILETIME
    double freq;         // 8 bytes Hz
    double level;        // 8 bytes dBµV
};                       // = 26 bytes total
#pragma pack(pop)
```

### 10.4 `click_enter.vbs`

Script VBScript de automatización de Windows Shell. Espera hasta 10 segundos a que aparezca una ventana con un título concreto (hardcodeado como `"Nombre"` — parece un placeholder no finalizado) y le envía la tecla `{ENTER}`. Su propósito probable es automatizar el confirm de algún diálogo (posiblemente el que aparece al ejecutar el `.exe` compilado con PyInstaller cuando hay alertas de seguridad de Windows).

### 10.5 Entorno de despliegue inferido

- Máquina Windows (servidor local, red corporativa `192.168.x.x`)
- El dashboard se ejecuta directamente con `python dashboard2.py` o como `.exe` compilado
- El navegador se abre automáticamente en `http://127.0.0.1:8082`
- Sin servidor proxy inverso ni HTTPS (acceso local puro)
- Los bots de análisis (`botnuevo.py`, etc.) se ejecutan de forma periódica por el Programador de Tareas de Windows

---

## 11. Riesgos y Deuda Técnica

### 11.1 Credenciales hardcodeadas en el código fuente

**Crítico.** Tanto `dashboard2.py` como todos los bots contienen credenciales reales en el código fuente, visibles en el repositorio:

```python
# dashboard2.py, líneas 88–92
SMTP_USER = "jpitmalagaalertas@gmail.com"
SMTP_PASS = "kwejengkvwuahmim"             # App Password de Google en texto claro
```

```python
# botnuevo.py / Bot_Analisis_MA.py, líneas 47–48 y 50–51
USUARIO_RED = "CTER"
PASS_RED = "123456"
CORREO_BOT = "jpitmalagaalertas@gmail.com"
PASSWORD_BOT = "kwejengkvwuahmim"
```

Los ficheros de configuración JSON (`fuentes_globales.json`, `usuarios.json`) también contienen credenciales de red (`pwd_red: "malaga"`) y contraseñas de usuario en texto plano.

**Riesgo:** Cualquiera con acceso al repositorio (o al ejecutable descompilado) dispone de credenciales de red SMB y del correo de alertas.

### 11.2 Ausencia de HTTPS y tokens CSRF

El servidor Flask usa `secret_key` hardcodeada (`"phoenix_argus_super_secret_key_2026"`). No hay protección CSRF. El endpoint `/api/extraer` no verifica el origen. Al estar en red local esto tiene menor impacto, pero es un riesgo si el servidor es accesible desde otras redes.

### 11.3 Servidor Flask en modo desarrollo en producción

`app.run(host='0.0.0.0', port=8082)` usa el servidor de desarrollo de Werkzeug (monohilo, sin manejo de errores robusto). Una extracción larga bloquea completamente el servidor. Se debería usar Waitress o Gunicorn.

### 11.4 Path traversal en el visor de archivos

Los endpoints `/api/files/serve` y `/api/files/download` sirven cualquier archivo dado por ruta absoluta sin validar que esté bajo las rutas configuradas:

```python
@app.route('/api/files/serve')
def api_files_serve():
    return send_file(request.args.get('path'))  # sin saneamiento
```

Un usuario autenticado (cualquier rol) podría servirse archivos arbitrarios del sistema de archivos del servidor.

### 11.5 Comparación de contraseñas en texto plano sin hashing

```python
# dashboard2.py, línea 164
if u in db and db[u]['password'] == p:  # comparación directa
```

Las contraseñas de `usuarios.json` no se hashean (ni con bcrypt ni con PBKDF2). Si el archivo es accesible a un atacante, las contraseñas están expuestas directamente.

### 11.6 Excepciones silenciosas masivas

El código abusa del patrón `except: pass` en múltiples lugares críticos:

```python
# botnuevo.py / Bot_Analisis_MA.py
except Exception: pass  # en el bucle de desempaquetado binario
except: pass            # en ejecutar_robocopy()
```

```python
# dashboard2.py
try: wb.save(log_path)
except: pass
```

Esto hace que los fallos se silencien sin registro. En `dashboard2.py` hay un intento de mejora: errores en `api_extraer` se registran con `traceback.format_exc()` y se imprimen por stdout, pero no hay ningún sistema de logging persistente en el dashboard.

### 11.7 Falta de tests funcionales

Los cuatro archivos de test (`test_json.py`, `test_long_path.py`, `test_tk.py`, `test_tk_flask.py`) son **herramientas de diagnóstico manuales**, no tests automatizados (sin `unittest`, `pytest` ni assertions). `test_json.py` es un Flask aislado para depurar la lectura de JSON en red; `test_tk.py` verifica que Tkinter abre una ventana; `test_long_path.py` prueba que Python puede crear rutas largas en Windows.

### 11.8 Inconsistencias en el algoritmo de detección de picos

Los bots autónomos ordenan por nivel descendente (`sort_values(by='L', ascending=False)`) para capturar primero los picos más intensos, mientras que `dashboard2.py` ordena cronológicamente (`sort_values(by='T', ascending=True)`) para capturar la "primera ocurrencia". Esto puede producir resultados diferentes para el mismo conjunto de datos.

### 11.9 Zona horaria hardcodeada (bot antiguo vs bots nuevos)

`botnuevo.py` suma 1 hora fija UTC+1 (línea 351):
```python
ts = datetime.datetime(1601, 1, 1) + datetime.timedelta(microseconds=tr/10) + datetime.timedelta(hours=1)
```

Los bots más nuevos (`Bot_Analisis_MA.py`) y `dashboard2.py` usan conversión automática a la zona horaria local del sistema:
```python
ts_utc = datetime.datetime(1601, 1, 1) + datetime.timedelta(microseconds=tr/10)
ts = ts_utc.replace(tzinfo=datetime.timezone.utc).astimezone().replace(tzinfo=None)
```

Esto produce diferencias de una hora en verano (cuando España está en UTC+2) si se comparan resultados del bot antiguo con el dashboard.

### 11.10 Acoplamiento fuerte: un único archivo monolítico

Los 1 408 líneas de `dashboard2.py` mezclan: configuración global, autenticación, lógica de negocio, motor matemático, motor gráfico, plantilla HTML con CSS y JavaScript, y todas las rutas Flask. No hay separación en módulos. Cualquier modificación requiere entender el archivo completo.

### 11.11 Sin límite de tamaño en la petición de extracción

El endpoint `/api/extraer` puede recibir un rango de fechas arbitrariamente largo (p.ej. 1 año de datos binarios). No hay ningún límite de tiempo (timeout de Flask) ni de tamaño de datos que impida una solicitud que consuma toda la RAM disponible.

---

## Apéndice: Inventario de archivos del repositorio analizado

| Archivo | Tamaño | Líneas | Descripción |
|---|---|---|---|
| `dashboard2.py` | 105 KB | 1 408 | Dashboard principal (este informe) |
| `dashboard.py` | 43 KB | 752 | Versión previa sin backend funcional |
| `web1.py` | 49 KB | 651 | Versión intermedia (sin auth, sin visor) |
| `botnuevo.py` | 24 KB | 483 | Bot autónomo UMA |
| `Bot_Analisis_MA.py` | 25 KB | 509 | Bot autónomo MA (incluye sincronización a red DSIC) |
| `Bot_Analisis_UMA.py` | 25 KB | 508 | Bot autónomo UMA variante |
| `Bot_Analisis_UMA2.py` | 25 KB | 507 | Bot autónomo UMA2 |
| `renderizador_bot.py` | — | 109 | CLI para procesar CSVs de C++ |
| `compile_all.py` | — | 104 | Empaquetado con PyInstaller |
| `src/main.cpp` | — | 158 | Extractor C++17 del formato binario ARGUS |
| `CMakeLists.txt` | — | 19 | Proyecto CMake para main.cpp |
| `fuentes_globales.json` | 1.1 KB | 26 | 3 estaciones con rutas y credenciales SMB |
| `usuarios.json` | 419 B | 22 | 5 usuarios admin con contraseñas en texto plano |
| `web_contacts.json` | 144 B | 8 | 1 contacto técnico (herencia de web1) |
| `web_sources.json` | 173 B | 7 | 1 fuente (herencia de web1) |
| `click_enter.vbs` | — | 16 | Automatización de teclado Windows |
| `test_json.py` | — | 94 | Herramienta diagnóstico lectura JSON en red |
| `test_long_path.py` | — | 22 | Herramienta diagnóstico rutas largas Windows |
| `test_tk.py` | — | 9 | Herramienta diagnóstico Tkinter |
| `test_tk_flask.py` | — | 29 | Herramienta diagnóstico Flask+Tkinter |
| `.gitignore` | — | 7 | Excluye build/, dist/, __pycache__, *.spec |
