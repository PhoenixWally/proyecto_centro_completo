package main

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// --- ESTRUCTURAS DE DATOS ---

type PuntoRadar struct {
	T    int64   `json:"t"`
	F    float64 `json:"f"`
	L    float64 `json:"l"`
	Time string  `json:"time"`
}

type PeticionExtraccion struct {
	Fuente struct {
		Nombre  string `json:"nombre"`
		RutaBin string `json:"ruta_bin"`
		RutaRes string `json:"ruta_res"`
		UsrRed  string `json:"usr_red"`
		PwdRed  string `json:"pwd_red"`
	} `json:"fuente"`
	OrigenDatos  string `json:"origen_datos"`
	FIni         string `json:"f_ini"`
	HIni         string `json:"h_ini"`
	FFin         string `json:"f_fin"`
	HFin         string `json:"h_fin"`
	SalidaManual string `json:"salida_manual"`
	Entregables  struct {
		Dividir  bool `json:"dividir"`
		HorasDiv int  `json:"horas_div"`
	} `json:"entregables"`
}

type Tramo struct {
	Inicio time.Time
	Fin    time.Time
	File   *os.File
	Writer *csv.Writer
	Path   string
	Puntos int
}

// Rutas dinámicas — se calculan en init() relativas al directorio del ejecutable o directorios estándar
var (
	BaseDir              string // directorio del ejecutable, para resolver rutas absolutas
	ConfigDir            string // directorio en ProgramData para configuraciones y cachés con permisos de escritura
	EjecutableSentinel   string
	ScriptPintor         string
	ArchivoFuentesGlobal string
	PythonExe            string // ruta al python embebido (inyectada por el lanzador via PYTHON_EXE)
)

func init() {
	exePath, err := os.Executable()
	if err != nil {
		// Fallback: usar directorio actual
		exePath, _ = filepath.Abs(os.Args[0])
	}
	BaseDir = filepath.Dir(exePath)

	// ConfigDir en ProgramData para evitar problemas de permisos de escritura en C:\Program Files
	programData := os.Getenv("PROGRAMDATA")
	if programData == "" {
		programData = `C:\ProgramData`
	}
	ConfigDir = filepath.Join(programData, "centro_cter", "centro_analitico")
	os.MkdirAll(ConfigDir, 0755)

	// sentinel_core.exe está en ../sentinel_core/ relativo al ejecutable del centro analítico
	EjecutableSentinel = filepath.Join(BaseDir, "..", "sentinel_core", "sentinel_core.exe")
	ScriptPintor = filepath.Join(BaseDir, "pintor.py")
	ArchivoFuentesGlobal = filepath.Join(BaseDir, "fuentes_globales.json")

	// Python embebido: el lanzador establece PYTHON_EXE, si no usamos 'python' del PATH
	PythonExe = os.Getenv("PYTHON_EXE")
	if PythonExe == "" {
		PythonExe = "python"
	}

	log.Printf("[Init] BaseDir: %s", BaseDir)
	log.Printf("[Init] ConfigDir: %s", ConfigDir)
	log.Printf("[Init] Sentinel: %s", EjecutableSentinel)
	log.Printf("[Init] Pintor: %s", ScriptPintor)
	log.Printf("[Init] Python: %s", PythonExe)
}

func conectarRedWindows(ruta, usr, pwd string) {
	if usr == "" || pwd == "" || !strings.HasPrefix(ruta, "\\\\") {
		return
	}

	// Comprobar si ya es accesible para evitar colgarse
	if _, err := os.Stat(ruta); err == nil {
		return
	}

	// Extraer carpeta base para que net use funcione
	rutaBase := ruta
	nombreArchivo := filepath.Base(ruta)
	if strings.Contains(nombreArchivo, ".") && len(filepath.Ext(nombreArchivo)) <= 5 {
		rutaBase = filepath.Dir(ruta)
	}

	log.Printf("[Red] Conectando ruta UNC: %s con usuario %s", rutaBase, usr)
	cmd := exec.Command("net", "use", rutaBase, pwd, "/user:"+usr)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Run(); err != nil {
		log.Printf("[Red] Error al conectar %s: %v", rutaBase, err)
	} else {
		log.Printf("[Red] Conexión UNC exitosa a %s", rutaBase)
	}
}

var (
	estadoProgreso     string
	progresoMutex      sync.Mutex
	isExecuting        bool
	ultimoError        string
	rutaSalida         string
	tareasAutomaticas  = make(map[string]bool)
	tareasAutoMutex    sync.RWMutex
	tareasAutoFilePath string
)

func cargarTareasAutomaticas() {
	tareasAutoMutex.Lock()
	defer tareasAutoMutex.Unlock()
	tareasAutoFilePath = filepath.Join(ConfigDir, "tareas_automaticas.json")
	data, err := os.ReadFile(tareasAutoFilePath)
	if err == nil {
		json.Unmarshal(data, &tareasAutomaticas)
	}
}

func guardarTareasAutomaticas() {
	tareasAutoMutex.Lock()
	defer tareasAutoMutex.Unlock()
	data, _ := json.MarshalIndent(tareasAutomaticas, "", "    ")
	os.WriteFile(tareasAutoFilePath, data, 0644)
}


// --- MOTOR DE EXTRACCIÓN (FLUJO POR BLOQUES) ---

func actualizarProgreso(mensaje string) {
	progresoMutex.Lock()
	estadoProgreso = mensaje
	progresoMutex.Unlock()
}

func apiProgresoHandler(w http.ResponseWriter, r *http.Request) {
	progresoMutex.Lock()
	msg := estadoProgreso
	executing := isExecuting
	errStr := ultimoError
	outPath := rutaSalida
	progresoMutex.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"mensaje":   msg,
		"executing": executing,
		"error":     errStr,
		"salida":    outPath,
	})
}

func MotorExtraccionBinaria(req PeticionExtraccion, carpetaDestino string, inicio, fin time.Time) error {
	if req.Fuente.UsrRed != "" && req.Fuente.PwdRed != "" {
		conectarRedWindows(req.Fuente.RutaBin, req.Fuente.UsrRed, req.Fuente.PwdRed)
		conectarRedWindows(req.Fuente.RutaRes, req.Fuente.UsrRed, req.Fuente.PwdRed)
		if req.SalidaManual != "" {
			conectarRedWindows(req.SalidaManual, req.Fuente.UsrRed, req.Fuente.PwdRed)
		}
	}

	actualizarProgreso("⏳ Extrayendo y filtrando datos Sherlock buscando...")
	log.Printf("🔍 Buscando binarios en: %s", req.Fuente.RutaBin)

	entries, err := os.ReadDir(req.Fuente.RutaBin)
	if err != nil {
		return fmt.Errorf("no se puede abrir la ruta de binarios: %v", err)
	}

	var archivosAProcesar []string
	tsMin := inicio.Add(-24 * time.Hour).Unix()
	tsMax := fin.Add(24 * time.Hour).Unix()

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != "" {
			continue
		}
		info, err := entry.Info()
		if err == nil && info.ModTime().Unix() >= tsMin && info.ModTime().Unix() <= tsMax {
			archivosAProcesar = append(archivosAProcesar, filepath.Join(req.Fuente.RutaBin, entry.Name()))
		}
	}

	if len(archivosAProcesar) == 0 {
		return fmt.Errorf("no hay binarios con fechas cercanas a las solicitadas")
	}

	os.MkdirAll(carpetaDestino, os.ModePerm)

	// 1. PREPARAR BLOQUES (TRAMOS)
	horasDiv := 24
	if req.Entregables.Dividir && req.Entregables.HorasDiv > 0 {
		horasDiv = req.Entregables.HorasDiv
	}

	var tramos []*Tramo
	tCurr := inicio
	for tCurr.Before(fin) {
		tNext := tCurr.Add(time.Duration(horasDiv) * time.Hour)
		if tNext.After(fin) {
			tNext = fin
		}
		tramos = append(tramos, &Tramo{Inicio: tCurr, Fin: tNext})
		tCurr = tNext
	}

	// 2. FLUJO DE EXTRACCIÓN Y ENRUTAMIENTO
	for _, rutaArchivo := range archivosAProcesar {
		log.Printf("⚙️ Procesando binario: %s", filepath.Base(rutaArchivo))

		cmdSentinel := exec.Command(EjecutableSentinel, rutaArchivo)
		cmdSentinel.Dir = filepath.Dir(EjecutableSentinel)

		stdout, err := cmdSentinel.StdoutPipe()
		if err != nil {
			continue
		}
		if err := cmdSentinel.Start(); err != nil {
			continue
		}

		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			linea := scanner.Bytes()
			if len(linea) > 0 && linea[0] == '{' {
				var punto PuntoRadar
				if err := json.Unmarshal(linea, &punto); err == nil {
					dt := time.Unix(punto.T, 0)

					if (dt.After(inicio) || dt.Equal(inicio)) && (dt.Before(fin) || dt.Equal(fin)) {
						for _, tr := range tramos {
							if (dt.After(tr.Inicio) || dt.Equal(tr.Inicio)) && dt.Before(tr.Fin) || (dt.Equal(fin) && tr.Fin.Equal(fin)) {
								if tr.Writer == nil {
									tr.Path = filepath.Join(carpetaDestino, fmt.Sprintf("temp_tramo_%d.csv", tr.Inicio.UnixNano()))
									f, _ := os.Create(tr.Path)
									tr.File = f
									tr.Writer = csv.NewWriter(f)
									tr.Writer.Write([]string{"T", "F", "L"})
								}
								tr.Writer.Write([]string{
									dt.Format("2006-01-02 15:04:05"),
									strconv.FormatFloat(punto.F, 'f', -1, 64),
									strconv.FormatFloat(punto.L, 'f', -1, 64),
								})
								tr.Puntos++
								break
							}
						}
					}
				}
			}
		}
		cmdSentinel.Wait()
	}

	// 3. DIBUJADO SECUENCIAL DE CADA BLOQUE POR PYTHON
	puntosTotales := 0
	for _, tr := range tramos {
		if tr.Writer != nil {
			tr.Writer.Flush()
			tr.File.Close()
			puntosTotales += tr.Puntos

			tIniStr := tr.Inicio.Format("2006-01-02 15:04:05")
			tFinStr := tr.Fin.Format("2006-01-02 15:04:05")
			mensajeActual := fmt.Sprintf("🎨 Dibujando bloque: %s -> %s (%d puntos)", tIniStr, tFinStr, tr.Puntos)
			log.Println(mensajeActual)
			actualizarProgreso(mensajeActual)
			//log.Printf("🎨 Invocando Pintor para el bloque: %s -> %s (%d puntos)", tIniStr, tFinStr, tr.Puntos)

			cmdPintor := exec.Command(PythonExe, ScriptPintor, tr.Path, carpetaDestino, req.Fuente.Nombre, tIniStr, tFinStr)
			cmdPintor.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
			out, err := cmdPintor.CombinedOutput()
			if err != nil {
				log.Printf("❌ Error al ejecutar Pintor para el tramo %s -> %s: %v. Output:\n%s", tIniStr, tFinStr, err, string(out))
			} else {
				log.Printf("🎨 Pintor finalizado con éxito para el tramo %s -> %s.", tIniStr, tFinStr)
			}

			os.Remove(tr.Path) // Destruimos el CSV temporal para no llenar el disco
		}
	}

	if puntosTotales == 0 {
		return fmt.Errorf("los datos están vacíos tras aplicar los filtros de hora")
	}
	return nil
}

// --- ENDPOINTS (HANDLERS) DE LA API ---

type SessionInfo struct {
	NivelPoder        int  `json:"nivel_poder"`
	PermisoExtraccion bool `json:"permiso_extraccion"`
	PermisoAlarmas    bool `json:"permiso_alarmas"`
	PermisoVisor      bool `json:"permiso_visor"`
}

func obtenerSessionUsuario(r *http.Request) (SessionInfo, error) {
	var s SessionInfo
	cookie, err := r.Cookie("jwt")
	if err != nil {
		return s, err
	}

	client := &http.Client{Timeout: 3 * time.Second}
	req, err := http.NewRequest("GET", "http://127.0.0.1:8080/api/session", nil)
	if err != nil {
		return s, err
	}
	req.AddCookie(cookie)

	resp, err := client.Do(req)
	if err != nil {
		return s, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return s, fmt.Errorf("status not ok: %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return s, err
	}

	return s, nil
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, filepath.Join(BaseDir, "index.html"))
}

func apiFuentesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodGet {
		sess, err := obtenerSessionUsuario(r)
		if err != nil || (sess.NivelPoder < 80 && sess.NivelPoder < 60 && !sess.PermisoVisor && !sess.PermisoExtraccion && !sess.PermisoAlarmas) {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`[]`))
			return
		}

		client := &http.Client{}
		reqSSO, err := http.NewRequest("GET", "http://127.0.0.1:8080/api/admin/estaciones", nil)
		if err != nil {
			w.Write([]byte(`[]`))
			return
		}

		// Propagar la cookie JWT de la sesión del usuario
		if cookie, err := r.Cookie("jwt"); err == nil {
			reqSSO.AddCookie(cookie)
		}

		respSSO, err := client.Do(reqSSO)
		if err != nil || respSSO.StatusCode != http.StatusOK {
			log.Printf("[Argus apiFuentes] Error query SSO: %v", err)
			w.Write([]byte(`[]`))
			return
		}
		defer respSSO.Body.Close()

		var ssoEstaciones []map[string]interface{}
		if err := json.NewDecoder(respSSO.Body).Decode(&ssoEstaciones); err != nil {
			log.Printf("[Argus apiFuentes] Decode error: %v", err)
			w.Write([]byte(`[]`))
			return
		}

		// Mapear estaciones de la BD a fuentes de Argus
		fuentes := []map[string]interface{}{}
		for _, st := range ssoEstaciones {
			rBin, _ := st["ruta_bin"].(string)
			rRes, _ := st["ruta_res"].(string)
			if rBin == "" || rRes == "" {
				continue // Ignorar estaciones sin configuración del Centro Analítico
			}

			uRed, _ := st["usr_red"].(string)
			pRed, _ := st["pwd_red"].(string)
			nombre, _ := st["nombre"].(string)

			// Generar ruta JSON unificada en la carpeta del SSO
			rutaJson := fmt.Sprintf(`C:\nginx\html\sso\config_estacion_%s.json`, st["id"])

			fuentes = append(fuentes, map[string]interface{}{
				"id":           st["id"],
				"nombre":       nombre,
				"provincia_id": st["provincia_id"],
				"ruta_bin":     rBin,
				"ruta_res":     rRes,
				"ruta_json":    rutaJson,
				"usr_red":      uRed,
				"pwd_red":      pRed,
			})
		}

		// Guardar en caché local fuentes_cache.json
		fuentesBytes, errCache := json.MarshalIndent(fuentes, "", "    ")
		if errCache == nil {
			os.WriteFile(filepath.Join(ConfigDir, "fuentes_cache.json"), fuentesBytes, 0644)
		}

		json.NewEncoder(w).Encode(fuentes)
	} else if r.Method == http.MethodPost {
		// La edición de estaciones se centraliza en el Panel de SSO
		w.Write([]byte(`{"status":"ok"}`))
	}
}

func apiFilesListHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	sess, err := obtenerSessionUsuario(r)
	if err != nil || (sess.NivelPoder < 80 && sess.NivelPoder < 60 && !sess.PermisoVisor) {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"dirs": []string{}, "files": []string{}, "error": "Forbidden: Insufficient privileges",
		})
		return
	}

	var req struct {
		Path   string `json:"path"`
		Fuente struct {
			UsrRed string `json:"usr_red"`
			PwdRed string `json:"pwd_red"`
		} `json:"fuente"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"dirs": []string{}, "files": []string{}, "error": "JSON inválido",
		})
		return
	}

	if req.Fuente.UsrRed != "" && req.Fuente.PwdRed != "" {
		conectarRedWindows(req.Path, req.Fuente.UsrRed, req.Fuente.PwdRed)
	}

	// filepath.Clean limpia la ruta y convierte barras '/' en '\' automáticamente en Windows
	rutaLimpia := filepath.Clean(req.Path)

	entries, err := os.ReadDir(rutaLimpia)
	if err != nil {
		// CRÍTICO: Devolvemos arrays vacíos en lugar de un objeto corrupto
		// para que el bucle .forEach() de JavaScript no lance una excepción en el navegador
		json.NewEncoder(w).Encode(map[string]interface{}{
			"dirs":  []string{},
			"files": []string{},
			"error": err.Error(),
		})
		return
	}

	type FileItem struct {
		Name     string `json:"name"`
		FullPath string `json:"full_path"`
	}
	dirs := []FileItem{}
	files := []FileItem{}

	for _, entry := range entries {
		fullPath := filepath.Join(rutaLimpia, entry.Name())
		item := FileItem{Name: entry.Name(), FullPath: fullPath}

		if entry.IsDir() {
			dirs = append(dirs, item)
		} else {
			files = append(files, item)
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"dirs":  dirs,
		"files": files,
	})
}

func apiSelectFolderHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	sess, err := obtenerSessionUsuario(r)
	if err != nil || (sess.NivelPoder < 80 && sess.NivelPoder < 60 && !sess.PermisoExtraccion) {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "Forbidden: Insufficient privileges"})
		return
	}

	// Script PowerShell para abrir la ventana nativa de selección de carpeta de Windows.
	// El diálogo se ejecuta dentro de un hilo STA dedicado con su propio bucle de mensajes
	// (Application.Run / Application.DoEvents) para evitar que se congele cuando el
	// explorador intenta resolver rutas UNC lentas de red (\\192.168.29.xx).
	psCommand := `
		Add-Type -AssemblyName System.Windows.Forms;
		Add-Type -AssemblyName System.Drawing;

		$resultado = "";

		# Creamos un formulario invisible que será el "dueño" del diálogo,
		# aportando una ventana padre y un bucle de mensajes completo.
		$form = New-Object System.Windows.Forms.Form;
		$form.Opacity = 0;
		$form.ShowInTaskbar = $false;
		$form.StartPosition = "CenterScreen";
		$form.Size = New-Object System.Drawing.Size(1, 1);

		$form.Add_Shown({
			$f = New-Object System.Windows.Forms.OpenFileDialog;
			$f.ValidateNames = $false;
			$f.CheckFileExists = $false;
			$f.CheckPathExists = $false;
			$f.FileName = "Seleccionar carpeta";
			$f.Title = "Selecciona la carpeta de salida -- Navega y pulsa Abrir";
			if ($f.ShowDialog($form) -eq "OK") {
				$script:resultado = [System.IO.Path]::GetDirectoryName($f.FileName);
			}
			$form.Close();
		});

		[System.Windows.Forms.Application]::Run($form);
		Write-Output $script:resultado;
	`
	// Timeout de 3 minutos: si el usuario deja el diálogo abierto mucho tiempo
	// el servidor no se queda bloqueado indefinidamente.
	ctx, cancel := context.WithTimeout(r.Context(), 3*60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-STA", "-Command", psCommand)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err := cmd.Output()
	if err != nil {
		log.Printf("[Argus Select Folder] Error executing PowerShell: %v", err)
		json.NewEncoder(w).Encode(map[string]string{"error": "No se pudo abrir el diálogo: " + err.Error()})
		return
	}

	selectedPath := strings.TrimSpace(string(output))
	json.NewEncoder(w).Encode(map[string]string{"path": selectedPath})
}

func apiFilesServeHandler(w http.ResponseWriter, r *http.Request) {
	sess, err := obtenerSessionUsuario(r)
	if err != nil || (sess.NivelPoder < 80 && sess.NivelPoder < 60 && !sess.PermisoVisor) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	filePath := r.URL.Query().Get("path")
	http.ServeFile(w, r, filePath)
}

func apiFilesDownloadHandler(w http.ResponseWriter, r *http.Request) {
	sess, err := obtenerSessionUsuario(r)
	if err != nil || (sess.NivelPoder < 80 && sess.NivelPoder < 60 && !sess.PermisoVisor) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	filePath := r.URL.Query().Get("path")
	w.Header().Set("Content-Disposition", "attachment; filename="+filepath.Base(filePath))
	http.ServeFile(w, r, filePath)
}

func apiFilesZipHandler(w http.ResponseWriter, r *http.Request) {
	sess, err := obtenerSessionUsuario(r)
	if err != nil || (sess.NivelPoder < 80 && sess.NivelPoder < 60 && !sess.PermisoVisor && !sess.PermisoExtraccion) {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	dirPath := r.URL.Query().Get("path")
	if dirPath == "" {
		http.Error(w, "Se requiere la ruta de la carpeta", http.StatusBadRequest)
		return
	}

	info, err := os.Stat(dirPath)
	if err != nil || !info.IsDir() {
		http.Error(w, "La ruta provista no es un directorio válido", http.StatusBadRequest)
		return
	}

	tempZip, err := os.CreateTemp("", "download_*.zip")
	if err != nil {
		http.Error(w, "Error al crear archivo temporal: "+err.Error(), http.StatusInternalServerError)
		return
	}
	tempZipPath := tempZip.Name()
	tempZip.Close()
	defer os.Remove(tempZipPath)

	err = zipDirectory(dirPath, tempZipPath)
	if err != nil {
		http.Error(w, "Error al empaquetar la carpeta: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, filepath.Base(dirPath)))
	http.ServeFile(w, r, tempZipPath)
}

type FuenteConfig struct {
	ID          interface{} `json:"id"`
	Nombre      string      `json:"nombre"`
	ProvinciaID string      `json:"provincia_id"`
	RutaBin     string      `json:"ruta_bin"`
	RutaRes     string      `json:"ruta_res"`
	RutaJson    string      `json:"ruta_json"`
	UsrRed      string      `json:"usr_red"`
	PwdRed      string      `json:"pwd_red"`
}

func apiConfigHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	sess, err := obtenerSessionUsuario(r)
	if err != nil || (sess.NivelPoder < 80 && sess.NivelPoder < 60 && !sess.PermisoAlarmas) {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "Forbidden: Insufficient privileges"})
		return
	}

	var req struct {
		RutaJson string       `json:"ruta_json"`
		Fuente   FuenteConfig `json:"fuente"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Write([]byte(`{"alertas_activas": false, "hora_inicio": "00:00", "hora_fin": "23:59", "fechas_bloqueadas": [], "contactos": []}`))
		return
	}

	if req.Fuente.UsrRed != "" && req.Fuente.PwdRed != "" {
		conectarRedWindows(req.RutaJson, req.Fuente.UsrRed, req.Fuente.PwdRed)
	}

	var configData map[string]interface{}
	data, err := os.ReadFile(req.RutaJson)
	if err != nil {
		configData = map[string]interface{}{
			"alertas_activas":   false,
			"hora_inicio":       "00:00",
			"hora_fin":          "23:59",
			"fechas_bloqueadas": []interface{}{},
			"contactos":         []interface{}{},
		}
	} else {
		if err := json.Unmarshal(data, &configData); err != nil {
			configData = make(map[string]interface{})
		}
	}

	// 1. Obtener la lista de usuarios del SSO para la provincia asignada
	var ssoUsers []map[string]interface{}
	if req.Fuente.ProvinciaID != "" {
		jwtCookie, _ := r.Cookie("jwt")
		client := &http.Client{Timeout: 5 * time.Second}
		reqURL := fmt.Sprintf("http://127.0.0.1:8080/api/admin/usuarios?provincia_id=%s", req.Fuente.ProvinciaID)
		log.Printf("[Argus apiConfig] Solicitando usuarios a SSO: %s (Cookie jwt presente: %t)", reqURL, jwtCookie != nil)

		reqSSO, err := http.NewRequest("GET", reqURL, nil)
		if err == nil {
			if jwtCookie != nil {
				reqSSO.AddCookie(jwtCookie)
			} else {
				log.Printf("[Argus apiConfig] Advertencia: Cookie 'jwt' no encontrada en la petición entrante")
			}
			respSSO, err := client.Do(reqSSO)
			if err != nil {
				log.Printf("[Argus apiConfig] Error haciendo petición a SSO: %v", err)
			} else {
				log.Printf("[Argus apiConfig] SSO respondió con código: %d", respSSO.StatusCode)
				if respSSO.StatusCode == http.StatusOK {
					if err := json.NewDecoder(respSSO.Body).Decode(&ssoUsers); err != nil {
						log.Printf("[Argus apiConfig] Error decodificando respuesta de SSO: %v", err)
					} else {
						log.Printf("[Argus apiConfig] Se obtuvieron %d usuarios del SSO", len(ssoUsers))
					}
				} else {
					bodyBytes, _ := io.ReadAll(respSSO.Body)
					log.Printf("[Argus apiConfig] Respuesta de error de SSO: %s", string(bodyBytes))
				}
				respSSO.Body.Close()
			}
		} else {
			log.Printf("[Argus apiConfig] Error al crear petición HTTP a SSO: %v", err)
		}
	} else {
		log.Printf("[Argus apiConfig] No se solicitan usuarios a SSO porque req.Fuente.ProvinciaID está vacío")
	}

	// 2. Extraer los estados 'activo' anteriores de los contactos en el JSON
	activeStatus := make(map[string]bool)
	if prevConts, ok := configData["contactos"].([]interface{}); ok {
		for _, pcVal := range prevConts {
			if pc, ok := pcVal.(map[string]interface{}); ok {
				if email, ok := pc["email"].(string); ok {
					if active, ok := pc["activo"].(bool); ok {
						activeStatus[email] = active
					}
				}
			}
		}
	}

	// 3. Crear la nueva lista de contactos basada en los usuarios de la provincia
	var contacts []map[string]interface{}
	for _, u := range ssoUsers {
		nombre, _ := u["usuario"].(string)
		if nombre == "" {
			continue
		}
		puesto, _ := u["puesto"].(string)
		if puesto == "" {
			puesto = "General"
		}
		email, _ := u["email"].(string)
		if email == "" {
			email = nombre + "@digital.gob.es"
		}

		activo := true
		if act, exists := activeStatus[email]; exists {
			activo = act
		}

		contacts = append(contacts, map[string]interface{}{
			"nombre": nombre,
			"email":  email,
			"grupo":  puesto,
			"activo": activo,
		})
	}

	// Si pudimos realizar la consulta correctamente, actualizamos el array en el config
	if len(ssoUsers) > 0 || req.Fuente.ProvinciaID != "" {
		configData["contactos"] = contacts
	}

	respData, _ := json.Marshal(configData)
	w.Write(respData)
}

func apiConfigSaveHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	sess, err := obtenerSessionUsuario(r)
	if err != nil || (sess.NivelPoder < 80 && sess.NivelPoder < 60 && !sess.PermisoAlarmas) {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "Forbidden: Insufficient privileges"})
		return
	}

	var req struct {
		RutaJson string       `json:"ruta_json"`
		Fuente   FuenteConfig `json:"fuente"`
		Datos    interface{}  `json:"datos"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	if req.Fuente.UsrRed != "" && req.Fuente.PwdRed != "" {
		conectarRedWindows(req.RutaJson, req.Fuente.UsrRed, req.Fuente.PwdRed)
	}

	os.MkdirAll(filepath.Dir(req.RutaJson), os.ModePerm)
	jsonData, _ := json.MarshalIndent(req.Datos, "", "    ")
	err = os.WriteFile(req.RutaJson, jsonData, 0644)

	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func apiExtraerHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		http.Error(w, `{"status":"error","error":"Método no permitido"}`, http.StatusMethodNotAllowed)
		return
	}

	sess, err := obtenerSessionUsuario(r)
	if err != nil || (sess.NivelPoder < 80 && sess.NivelPoder < 60 && !sess.PermisoExtraccion) {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "error": "Forbidden: Insufficient privileges"})
		return
	}

	var req PeticionExtraccion
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "error": "JSON corrupto"})
		return
	}

	progresoMutex.Lock()
	if isExecuting {
		progresoMutex.Unlock()
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "error": "Ya hay una extracción en curso en este momento."})
		return
	}
	estadoProgreso = "⏳ Extrayendo y filtrando datos..."
	isExecuting = true
	ultimoError = ""
	rutaSalida = ""
	progresoMutex.Unlock()

	layout := "2006-01-02 15:04"
	inicio, _ := time.ParseInLocation(layout, req.FIni+" "+req.HIni, time.Local)
	fin, _ := time.ParseInLocation(layout, req.FFin+" "+req.HFin, time.Local)

	carpetaDestino := req.SalidaManual
	if carpetaDestino == "" {
		carpetaDestino = filepath.Join(req.Fuente.RutaRes, fmt.Sprintf("ejecucion_a_peticion_%s", time.Now().Format("20060102_150405")))
	}

	go func() {
		err := MotorExtraccionBinaria(req, carpetaDestino, inicio, fin)

		progresoMutex.Lock()
		isExecuting = false
		if err != nil {
			ultimoError = err.Error()
			estadoProgreso = "❌ Error: " + err.Error()
		} else {
			rutaSalida = carpetaDestino
			estadoProgreso = "✅ EXTRACCIÓN COMPLETADA."
		}
		progresoMutex.Unlock()
	}()

	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "Proceso de extracción iniciado en segundo plano."})
}

func apiExtraerCustomHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"status":"error","error":"Método no permitido"}`, http.StatusMethodNotAllowed)
		return
	}

	sess, err := obtenerSessionUsuario(r)
	if err != nil || (sess.NivelPoder < 80 && sess.NivelPoder < 60 && !sess.PermisoExtraccion) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "error": "Forbidden: Insufficient privileges"})
		return
	}

	err = r.ParseMultipartForm(100 * 1024 * 1024) // 100MB max file
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "error": "Archivos demasiado grandes o corruptos"})
		return
	}

	form := r.MultipartForm
	files := form.File["archivo"]
	if len(files) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "error": "Falta el archivo o los archivos en la solicitud"})
		return
	}

	prefijo := r.FormValue("prefijo")
	if prefijo == "" {
		prefijo = "REPORTE_PERSONALIZADO"
	}
	incluir3D := r.FormValue("incluir_3d") == "true"
	incluir3DStr := "false"
	if incluir3D {
		incluir3DStr = "true"
	}

	// Crear espacio de trabajo temporal
	runID := fmt.Sprintf("custom_%d", time.Now().UnixNano())
	tempDir := filepath.Join(os.TempDir(), runID)
	os.MkdirAll(tempDir, os.ModePerm)
	defer os.RemoveAll(tempDir)

	outputDir := filepath.Join(tempDir, "entregables")
	os.MkdirAll(outputDir, os.ModePerm)

	for _, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"status": "error", "error": "No se pudo abrir el archivo: " + fileHeader.Filename})
			return
		}

		// Guardar archivo cargado
		uploadedFilePath := filepath.Join(tempDir, fileHeader.Filename)
		outUploadedFile, err := os.Create(uploadedFilePath)
		if err != nil {
			file.Close()
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"status": "error", "error": "No se pudo crear el archivo temporal para: " + fileHeader.Filename})
			return
		}
		_, err = io.Copy(outUploadedFile, file)
		outUploadedFile.Close()
		file.Close()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"status": "error", "error": "Error al guardar el archivo temporal para: " + fileHeader.Filename})
			return
		}

		// Generar un prefijo único por archivo para evitar sobreescritura de entregables
		fileBase := strings.TrimSuffix(fileHeader.Filename, filepath.Ext(fileHeader.Filename))
		filePrefijo := fmt.Sprintf("%s_%s", prefijo, fileBase)

		cmd := exec.Command(PythonExe, "procesar_usuario.py", uploadedFilePath, outputDir, filePrefijo, incluir3DStr)
		cmd.Dir = "."

		out, err := cmd.CombinedOutput()
		if err != nil {
			log.Printf("[Custom Extract] Python Error on %s: %v\nOutput: %s", fileHeader.Filename, err, string(out))
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{
				"status": "error",
				"error":  fmt.Sprintf("Error al procesar el archivo %s en Python: %s", fileHeader.Filename, string(out)),
			})
			return
		}
	}

	zipFilePath := filepath.Join(tempDir, "resultados.zip")
	err = zipDirectory(outputDir, zipFilePath)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "error": "No se pudo empaquetar el resultado: " + err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s_procesado.zip"`, prefijo))
	http.ServeFile(w, r, zipFilePath)
}

func zipDirectory(sourceDir, zipFilePath string) error {
	zipFile, err := os.Create(zipFilePath)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	archive := zip.NewWriter(zipFile)
	defer archive.Close()

	err = filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relPath)
		header.Method = zip.Deflate

		writer, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(writer, file)
		return err
	})

	return err
}

func apiFilesPreviewHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	sess, err := obtenerSessionUsuario(r)
	if err != nil || (sess.NivelPoder < 80 && sess.NivelPoder < 60 && !sess.PermisoVisor) {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "Forbidden: Insufficient privileges"})
		return
	}

	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "JSON corrupto"})
		return
	}

	rutaLimpia := filepath.Clean(req.Path)

	scriptHelper := filepath.Join(BaseDir, "preview_helper.py")
	cmd := exec.Command(PythonExe, scriptHelper, rutaLimpia)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8")

	output, err := cmd.CombinedOutput()
	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{
			"html": fmt.Sprintf("<span style='color:red;'>Error al procesar vista previa: %v. Detalle: %s</span>", err, string(output)),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{
		"html": string(output),
	})
}

// --- PLANIFICADOR AUTOMÁTICO (PERIÓDICO) ---

func apiPlanificacionStatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	sess, err := obtenerSessionUsuario(r)
	if err != nil || (sess.NivelPoder < 80 && sess.NivelPoder < 60 && !sess.PermisoVisor && !sess.PermisoExtraccion && !sess.PermisoAlarmas) {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "error": "Forbidden: Insufficient privileges"})
		return
	}

	nombreEstacion := r.URL.Query().Get("estacion")
	tareasAutoMutex.RLock()
	activo := tareasAutomaticas[nombreEstacion]
	tareasAutoMutex.RUnlock()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"estacion": nombreEstacion,
		"activo":   activo,
	})
}

func apiPlanificacionToggleHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		http.Error(w, `{"status":"error"}`, http.StatusMethodNotAllowed)
		return
	}
	sess, err := obtenerSessionUsuario(r)
	if err != nil || (sess.NivelPoder < 80 && sess.NivelPoder < 60 && !sess.PermisoExtraccion) {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "error": "Forbidden: Insufficient privileges"})
		return
	}

	var req struct {
		Estacion string `json:"estacion"`
		Activo   bool   `json:"activo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "error": "JSON inválido"})
		return
	}
	
	tareasAutoMutex.Lock()
	tareasAutomaticas[req.Estacion] = req.Activo
	tareasAutoMutex.Unlock()
	guardarTareasAutomaticas()
	
	log.Printf("[Planificador] Planificación automática para '%s' establecida a: %t", req.Estacion, req.Activo)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func MotorExtraccionBinariaPlanificada(req PeticionExtraccion, carpetaDestino string, inicio, fin time.Time) error {
	if req.Fuente.UsrRed != "" && req.Fuente.PwdRed != "" {
		conectarRedWindows(req.Fuente.RutaBin, req.Fuente.UsrRed, req.Fuente.PwdRed)
		conectarRedWindows(req.Fuente.RutaRes, req.Fuente.UsrRed, req.Fuente.PwdRed)
	}

	log.Printf("[Planificador] Buscando binarios en: %s", req.Fuente.RutaBin)

	entries, err := os.ReadDir(req.Fuente.RutaBin)
	if err != nil {
		return fmt.Errorf("no se puede abrir la ruta de binarios: %v", err)
	}

	var archivosAProcesar []string
	tsMin := inicio.Add(-24 * time.Hour).Unix()
	tsMax := fin.Add(24 * time.Hour).Unix()

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != "" {
			continue
		}
		info, err := entry.Info()
		if err == nil && info.ModTime().Unix() >= tsMin && info.ModTime().Unix() <= tsMax {
			archivosAProcesar = append(archivosAProcesar, filepath.Join(req.Fuente.RutaBin, entry.Name()))
		}
	}

	if len(archivosAProcesar) == 0 {
		return fmt.Errorf("no hay binarios con fechas cercanas a las solicitadas")
	}

	os.MkdirAll(carpetaDestino, os.ModePerm)

	var tramos []*Tramo
	tramos = append(tramos, &Tramo{Inicio: inicio, Fin: fin})

	// Extracción y enrutamiento
	for _, rutaArchivo := range archivosAProcesar {
		cmdSentinel := exec.Command(EjecutableSentinel, rutaArchivo)
		cmdSentinel.Dir = filepath.Dir(EjecutableSentinel)

		stdout, err := cmdSentinel.StdoutPipe()
		if err != nil {
			continue
		}
		if err := cmdSentinel.Start(); err != nil {
			continue
		}

		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			linea := scanner.Bytes()
			if len(linea) > 0 && linea[0] == '{' {
				var punto PuntoRadar
				if err := json.Unmarshal(linea, &punto); err == nil {
					dt := time.Unix(punto.T, 0)

					if (dt.After(inicio) || dt.Equal(inicio)) && (dt.Before(fin) || dt.Equal(fin)) {
						for _, tr := range tramos {
							if (dt.After(tr.Inicio) || dt.Equal(tr.Inicio)) && dt.Before(tr.Fin) || (dt.Equal(fin) && tr.Fin.Equal(fin)) {
								if tr.Writer == nil {
									tr.Path = filepath.Join(carpetaDestino, fmt.Sprintf("temp_tramo_%d.csv", tr.Inicio.UnixNano()))
									f, _ := os.Create(tr.Path)
									tr.File = f
									tr.Writer = csv.NewWriter(f)
									tr.Writer.Write([]string{"T", "F", "L"})
								}
								tr.Writer.Write([]string{
									dt.Format("2006-01-02 15:04:05"),
									strconv.FormatFloat(punto.F, 'f', -1, 64),
									strconv.FormatFloat(punto.L, 'f', -1, 64),
								})
								tr.Puntos++
								break
							}
						}
					}
				}
			}
		}
		cmdSentinel.Wait()
	}

	puntosTotales := 0
	for _, tr := range tramos {
		if tr.Writer != nil {
			tr.Writer.Flush()
			tr.File.Close()
			puntosTotales += tr.Puntos

			tIniStr := tr.Inicio.Format("2006-01-02 15:04:05")
			tFinStr := tr.Fin.Format("2006-01-02 15:04:05")

			cmdPintor := exec.Command(PythonExe, ScriptPintor, tr.Path, carpetaDestino, req.Fuente.Nombre, tIniStr, tFinStr)
			cmdPintor.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
			out, err := cmdPintor.CombinedOutput()
			if err != nil {
				log.Printf("[Planificador] Error al ejecutar Pintor: %v. Output:\n%s", err, string(out))
			}
			os.Remove(tr.Path)
		}
	}

	if puntosTotales == 0 {
		return fmt.Errorf("los datos están vacíos tras aplicar los filtros de hora")
	}
	return nil
}

func obtenerSemanaFolder(t time.Time) string {
	_, week := t.ISOWeek()
	weekday := int(t.Weekday())
	offset := weekday - 1
	if weekday == 0 { // Sunday
		offset = 6
	}
	monday := t.AddDate(0, 0, -offset)
	sunday := monday.AddDate(0, 0, 6)
	return fmt.Sprintf("%d Semana %s al %s", week, monday.Format("02_01_2006"), sunday.Format("02_01_2006"))
}

func registrarResultadoEnExcel(fuente FuenteConfig, tEjecucion time.Time, errExt error, carpetaDestino string) {
	rutaExcel := filepath.Join(fuente.RutaRes, fmt.Sprintf("Registro_Extracciones_%s.xlsx", fuente.Nombre))
	
	if fuente.UsrRed != "" && fuente.PwdRed != "" {
		conectarRedWindows(fuente.RutaRes, fuente.UsrRed, fuente.PwdRed)
	}

	fecha := tEjecucion.Format("02/01/2006")
	hora := tEjecucion.Format("15:04:05")
	
	estado := "Finalizado"
	resultado := "Correcto"
	detallesError := ""
	alertas := "No"

	if errExt != nil {
		estado = "Error"
		resultado = "Fallo"
		detallesError = errExt.Error()
	} else {
		diasEs := []string{"Lunes", "Martes", "Miercoles", "Jueves", "Viernes", "Sabado", "Domingo"}
		weekday := int(tEjecucion.Weekday()) - 1
		if weekday < 0 {
			weekday = 6
		}
		nombreDia := diasEs[weekday]
		carpetaDia := fmt.Sprintf("%s_%s", tEjecucion.Format("060102"), nombreDia)
		rutaDia := filepath.Join(carpetaDestino, carpetaDia)

		filepath.Walk(rutaDia, func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && filepath.Ext(path) == ".docx" && strings.Contains(info.Name(), "Reporte_Alertas_") {
				alertas = "Sí"
			}
			return nil
		})
	}

	scriptAuditoria := filepath.Join(BaseDir, "registrar_auditoria.py")
	cmd := exec.Command(PythonExe, scriptAuditoria, rutaExcel, fecha, hora, estado, resultado, detallesError, alertas)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[Planificador] Error al registrar auditoría en Excel: %v. Output: %s", err, string(out))
	} else {
		log.Printf("[Planificador] Auditoría registrada con éxito para '%s' en Excel: %s", fuente.Nombre, string(out))
	}
}

func ejecutarExtraccionesAutomaticas() {
	log.Println("[Planificador] Iniciando ciclo de extracciones automáticas...")

	cachePath := filepath.Join(ConfigDir, "fuentes_cache.json")
	data, err := os.ReadFile(cachePath)
	if err != nil {
		log.Printf("[Planificador] No se pudo leer la caché de fuentes (aún no se han cargado fuentes en la UI): %v", err)
		return
	}

	var fuentes []FuenteConfig
	if err := json.Unmarshal(data, &fuentes); err != nil {
		log.Printf("[Planificador] Error al decodificar la caché de fuentes: %v", err)
		return
	}

	tareasAutoMutex.RLock()
	tareasActivas := make(map[string]bool)
	for k, v := range tareasAutomaticas {
		tareasActivas[k] = v
	}
	tareasAutoMutex.RUnlock()

	for _, fuente := range fuentes {
		if !tareasActivas[fuente.Nombre] {
			continue
		}

		log.Printf("[Planificador] Ejecutando extracción automática para: %s", fuente.Nombre)
		
		fin := time.Now()
		inicio := fin.Add(-1 * time.Hour)

		fIni := inicio.Format("2006-01-02")
		hIni := inicio.Format("15:04")
		fFin := fin.Format("2006-01-02")
		hFin := fin.Format("15:04")

		var req PeticionExtraccion
		req.Fuente.Nombre = fuente.Nombre
		req.Fuente.RutaBin = fuente.RutaBin
		req.Fuente.RutaRes = fuente.RutaRes
		req.Fuente.UsrRed = fuente.UsrRed
		req.Fuente.PwdRed = fuente.PwdRed
		req.OrigenDatos = "binario"
		req.FIni = fIni
		req.HIni = hIni
		req.FFin = fFin
		req.HFin = hFin
		
		req.Entregables.Dividir = false
		req.Entregables.HorasDiv = 1

		semanaFolder := obtenerSemanaFolder(fin)
		carpetaDestino := filepath.Join(fuente.RutaRes, semanaFolder)

		errExt := MotorExtraccionBinariaPlanificada(req, carpetaDestino, inicio, fin)

		registrarResultadoEnExcel(fuente, fin, errExt, carpetaDestino)
	}
}

func iniciarPlanificadorAutomatico() {
	ticker := time.NewTicker(1 * time.Hour)
	go func() {
		time.Sleep(10 * time.Second)
		ejecutarExtraccionesAutomaticas()
		for range ticker.C {
			ejecutarExtraccionesAutomaticas()
		}
	}()
}

// --- FUNCIÓN PRINCIPAL ---

func main() {
	cargarTareasAutomaticas()
	iniciarPlanificadorAutomatico()

	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/api/fuentes", apiFuentesHandler)
	http.HandleFunc("/api/extraer", apiExtraerHandler)
	http.HandleFunc("/api/files/list", apiFilesListHandler)
	http.HandleFunc("/api/files/select_folder", apiSelectFolderHandler)
	http.HandleFunc("/api/files/serve", apiFilesServeHandler)
	http.HandleFunc("/api/files/download", apiFilesDownloadHandler)
	http.HandleFunc("/api/files/zip", apiFilesZipHandler)
	http.HandleFunc("/api/files/preview", apiFilesPreviewHandler)
	http.HandleFunc("/api/config", apiConfigHandler)
	http.HandleFunc("/api/config/save", apiConfigSaveHandler)
	http.HandleFunc("/api/progreso", apiProgresoHandler)
	http.HandleFunc("/api/extraer/custom", apiExtraerCustomHandler)
	http.HandleFunc("/api/planificacion/toggle", apiPlanificacionToggleHandler)
	http.HandleFunc("/api/planificacion/status", apiPlanificacionStatusHandler)
	http.HandleFunc("/dragonite.gif", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(BaseDir, "dragonite.gif"))
	})

	fmt.Println("🚀 Servidor Web ARGUS escuchando en http://127.0.0.1:8082")
	if err := http.ListenAndServe(":8082", nil); err != nil {
		log.Fatalf("Error al levantar servidor: %v", err)
	}
}
