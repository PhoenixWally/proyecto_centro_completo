package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func main() {
	log.Println("==================================================")
	log.Println("       INICIANDO LAUNCHER PHOENIX SENTINEL        ")
	log.Println("==================================================")

	// 1. Obtener la ruta base del bundle
	exePath, err := os.Executable()
	if err != nil {
		log.Fatalf("Error crítico al obtener ruta del ejecutable: %v", err)
	}
	bundleDir := filepath.Dir(exePath)
	log.Printf("[Launcher] Directorio base del BUNDLE detectado: %s", bundleDir)

	// Definir rutas relativas absolutas
	nginxDir := filepath.Join(bundleDir, "nginx")
	nginxExe := filepath.Join(nginxDir, "nginx.exe")
	nginxConfTemplateBundle := filepath.Join(nginxDir, "conf", "nginx.conf.template")

	// Crear directorio de configuración de nginx en ProgramData
	programDataPath := os.Getenv("PROGRAMDATA")
	if programDataPath == "" {
		programDataPath = "C:\\ProgramData"
	}
	nginxDataRoot := filepath.Join(programDataPath, "centro_cter", "nginx")
	nginxConfDir := filepath.Join(nginxDataRoot, "conf")
	nginxConfReal := filepath.Join(nginxConfDir, "nginx.conf")

	// Crear el directorio si no existe
	if err := os.MkdirAll(nginxConfDir, 0755); err != nil {
		log.Fatalf("Error crítico al crear directorio de nginx en %s: %v", nginxConfDir, err)
	}

	// Copiar todos los archivos de configuración del bundle (mime.types, etc.) a ProgramData
	nginxConfBundleDir := filepath.Join(nginxDir, "conf")
	log.Printf("[Launcher] Copiando archivos de configuración de nginx de %s a %s...", nginxConfBundleDir, nginxConfDir)
	if err := copyDir(nginxConfBundleDir, nginxConfDir); err != nil {
		log.Fatalf("Error al copiar configuración de nginx al directorio de datos: %v", err)
	}
	log.Println("[Launcher] Archivos de configuración de nginx copiados con éxito.")

	ssoDir := filepath.Join(bundleDir, "sso_portal")
	ssoExe := filepath.Join(ssoDir, "sso_portal.exe")

	caDir := filepath.Join(bundleDir, "centro_analitico")
	caExe := filepath.Join(caDir, "canalitico.exe")

	pythonExe := filepath.Join(bundleDir, "python_embed", "python.exe")

	pgDir := filepath.Join(bundleDir, "postgresql")
	pgCtlExe := filepath.Join(pgDir, "bin", "pg_ctl.exe")
	pgDataDir := filepath.Join(programDataPath, "centro_cter", "postgresql", "data")

	log.Printf("[Launcher] Nginx: %s", nginxExe)
	log.Printf("[Launcher] Nginx Conf: %s", nginxConfReal)
	log.Printf("[Launcher] SSO Portal: %s", ssoExe)
	log.Printf("[Launcher] Centro Analítico: %s", caExe)
	log.Printf("[Launcher] Python Embebido: %s", pythonExe)
	log.Printf("[Launcher] PostgreSQL pg_ctl: %s", pgCtlExe)
	log.Printf("[Launcher] PostgreSQL Data: %s", pgDataDir)

	// 2. Generar nginx.conf real a partir de la plantilla
	log.Println("[Launcher] Generando nginx.conf con rutas dinámicas...")
	if _, err := os.Stat(nginxConfTemplateBundle); os.IsNotExist(err) {
		log.Fatalf("Error crítico: No existe la plantilla nginx.conf.template en %s", nginxConfTemplateBundle)
	}

	templateBytes, err := os.ReadFile(nginxConfTemplateBundle)
	if err != nil {
		log.Fatalf("Error al leer plantilla nginx.conf.template: %v", err)
	}
	templateStr := string(templateBytes)
	// Normalizar finales de línea CRLF (\r\n) a LF (\n) para asegurar que los reemplazos multilínea coincidan en Windows
	templateStr = strings.ReplaceAll(templateStr, "\r\n", "\n")

	// Normalizar las rutas de nginx usando barras normales '/' que Nginx requiere en Windows
	nginxDirForward := filepath.ToSlash(nginxDir)

	// Reemplazar la ruta C:/nginx de la plantilla por la real del bundle
	replacedConf := strings.ReplaceAll(templateStr, "C:/nginx", nginxDirForward)

	// Reemplazar el proxy_pass del Centro Analítico por un alias estático
	// El HTML está en nginx/html/analitico/, no lo sirve el backend Go
	oldAnalitico := "proxy_pass http://127.0.0.1:8082/;\n            proxy_set_header Cookie $http_cookie;"
	newAnalitico := fmt.Sprintf("alias \"%s/html/analitico/\";\n            index index.html;\n            try_files $uri $uri/ /analitico/index.html;", nginxDirForward)
	replacedConf = strings.ReplaceAll(replacedConf, oldAnalitico, newAnalitico)

	err = os.WriteFile(nginxConfReal, []byte(replacedConf), 0644)
	if err != nil {
		log.Fatalf("Error al escribir nginx.conf final: %v", err)
	}
	log.Println("[Launcher] nginx.conf generado con éxito.")

	// Listado de comandos activos para controlarlos
	var cmds []*exec.Cmd

	// Función para añadir proceso al control
	registerCmd := func(cmd *exec.Cmd) {
		cmds = append(cmds, cmd)
	}

	// 3. Arrancar PostgreSQL Embebido (Debe ser el PRIMERO para que los backends puedan conectarse)
	log.Println("[Launcher] Arrancando PostgreSQL Embebido...")
	if _, err := os.Stat(pgDataDir); os.IsNotExist(err) {
		log.Fatalf("Error crítico: La carpeta de datos de PostgreSQL no existe en %s. Por favor, ejecuta primero db_init\\init_db.bat.", pgDataDir)
	}

	cmdPG := exec.Command(pgCtlExe, "start", "-D", pgDataDir, "-w")
	cmdPG.Dir = pgDir
	cmdPG.Stdout = os.Stdout
	cmdPG.Stderr = os.Stderr
	if err := cmdPG.Run(); err != nil {
		log.Fatalf("Error al arrancar PostgreSQL Embebido: %v", err)
	}
	log.Println("[Launcher] PostgreSQL Embebido arrancado y listo para recibir conexiones.")

	// 4. Lanzar SSO Portal (Puerto 8080)
	log.Println("[Launcher] Arrancando SSO Portal...")
	cmdSSO := exec.Command(ssoExe)
	cmdSSO.Dir = ssoDir
	cmdSSO.Stdout = os.Stdout
	cmdSSO.Stderr = os.Stderr
	// Propagar variables de entorno y JWT
	cmdSSO.Env = os.Environ()
	if err := cmdSSO.Start(); err != nil {
		// Parar PostgreSQL antes de morir
		exec.Command(pgCtlExe, "stop", "-D", pgDataDir, "-m", "fast").Run()
		log.Fatalf("Error al iniciar SSO Portal: %v", err)
	}
	registerCmd(cmdSSO)
	log.Println("[Launcher] SSO Portal iniciado en segundo plano (PID:", cmdSSO.Process.Pid, ")")

	// 5. Lanzar Centro Analítico (Puerto 8082)
	log.Println("[Launcher] Arrancando Centro Analítico...")
	cmdCA := exec.Command(caExe)
	cmdCA.Dir = caDir
	cmdCA.Stdout = os.Stdout
	cmdCA.Stderr = os.Stderr
	// Propagar PYTHON_EXE para que lo lea el Go del Centro Analítico
	cmdCA.Env = append(os.Environ(), fmt.Sprintf("PYTHON_EXE=%s", pythonExe))
	if err := cmdCA.Start(); err != nil {
		// Parar SSO y PostgreSQL antes de morir
		cmdSSO.Process.Kill()
		exec.Command(pgCtlExe, "stop", "-D", pgDataDir, "-m", "fast").Run()
		log.Fatalf("Error al iniciar Centro Analítico: %v", err)
	}
	registerCmd(cmdCA)
	log.Println("[Launcher] Centro Analítico iniciado en segundo plano (PID:", cmdCA.Process.Pid, ")")

	// Esperar un momento corto antes de arrancar Nginx para asegurar que los backends escuchan
	time.Sleep(1 * time.Second)

	// 6. Lanzar Nginx (Puerto 80)
	log.Println("[Launcher] Arrancando Nginx...")
	cmdNginx := exec.Command(nginxExe, "-p", nginxDir+"/", "-c", nginxConfReal)
	cmdNginx.Dir = nginxDir
	cmdNginx.Stdout = os.Stdout
	cmdNginx.Stderr = os.Stderr
	if err := cmdNginx.Start(); err != nil {
		// Parar todos los procesos activos antes de morir
		cmdSSO.Process.Kill()
		cmdCA.Process.Kill()
		exec.Command(pgCtlExe, "stop", "-D", pgDataDir, "-m", "fast").Run()
		log.Fatalf("Error al iniciar Nginx: %v", err)
	}
	registerCmd(cmdNginx)
	log.Println("[Launcher] Nginx iniciado en segundo plano (PID:", cmdNginx.Process.Pid, ")")

	log.Println("==================================================")
	log.Println(" ✅ SISTEMA COMPLETAMENTE OPERATIVO ONLINE/OFFLINE ")
	log.Println(" 👉 URL: http://localhost/sso/                     ")
	log.Println(" Presiona Ctrl+C o cierra la ventana para detener  ")
	log.Println("==================================================")

	// 7. Capturar señales de parada para apagar todo a la vez
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// Monitorear si algún proceso hijo se cae de forma imprevista
	doneChan := make(chan error, 3)
	go func() { doneChan <- cmdSSO.Wait() }()
	go func() { doneChan <- cmdCA.Wait() }()
	go func() { doneChan <- cmdNginx.Wait() }()

	select {
	case sig := <-sigs:
		log.Printf("[Launcher] Señal de parada recibida (%v). Apagando procesos...", sig)
	case err := <-doneChan:
		log.Printf("[Launcher] Un proceso hijo ha terminado inesperadamente (%v). Apagando sistema...", err)
	}

	// 8. Apagar procesos hijos
	// Primero detener Nginx
	log.Println("[Launcher] Deteniendo Nginx...")
	stopNginx := exec.Command(nginxExe, "-s", "stop", "-p", nginxDir+"/", "-c", nginxConfReal)
	stopNginx.Dir = nginxDir
	stopNginx.Run()
	cmdNginx.Process.Kill()

	// Detener los backends
	log.Println("[Launcher] Deteniendo SSO Portal...")
	cmdSSO.Process.Kill()

	log.Println("[Launcher] Deteniendo Centro Analítico...")
	cmdCA.Process.Kill()

	// Detener PostgreSQL Embebido
	log.Println("[Launcher] Deteniendo PostgreSQL Embebido...")
	stopPG := exec.Command(pgCtlExe, "stop", "-D", pgDataDir, "-m", "fast")
	stopPG.Dir = pgDir
	stopPG.Run()

	log.Println("[Launcher] Sistema Phoenix Sentinel apagado con éxito.")
	log.Println("==================================================")
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	return out.Sync()
}

// copyDir copia recursivamente el contenido de srcDir a dstDir.
// Los directorios se crean si no existen. Los archivos existentes se sobreescriben.
func copyDir(srcDir, dstDir string) error {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return fmt.Errorf("error leyendo directorio origen %s: %w", srcDir, err)
	}

	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return fmt.Errorf("error creando directorio destino %s: %w", dstDir, err)
	}

	for _, entry := range entries {
		srcPath := filepath.Join(srcDir, entry.Name())
		dstPath := filepath.Join(dstDir, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return fmt.Errorf("error copiando %s a %s: %w", srcPath, dstPath, err)
			}
		}
	}
	return nil
}
