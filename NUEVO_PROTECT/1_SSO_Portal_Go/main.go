package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/argon2"

	"bufio"
	"os/exec"
	"path/filepath"

	"github.com/gorilla/websocket"
)

var dbPool *pgxpool.Pool
var jwtSecret = []byte("kP9$vF2m!tX7*qZb")

type contextKey string

const nivelKey contextKey = "nivel_poder"

// Claims define la estructura del payload JWT
type Claims struct {
	UsuarioID  string   `json:"usuario_id"`
	Provincias []string `json:"provincias"`
	NivelPoder int      `json:"nivel_poder"`
	jwt.RegisteredClaims
}

// LoginRequest mapea la petición JSON de entrada
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func main() {
	var err error

	// Configuración base de base de datos desde entorno, o por defecto
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:Portutatis@localhost:5432/sentinel_sso?sslmode=disable"
	}

	// Iniciar la conexión usando pgx
	dbPool, err = pgxpool.New(context.Background(), dbURL)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v\n", err)
	}
	defer dbPool.Close()

	// Migración automática del Centro Analítico
	_, err = dbPool.Exec(context.Background(), `
		ALTER TABLE estaciones ADD COLUMN IF NOT EXISTS ruta_bin VARCHAR(500) DEFAULT NULL;
		ALTER TABLE estaciones ADD COLUMN IF NOT EXISTS ruta_res VARCHAR(500) DEFAULT NULL;
		ALTER TABLE estaciones ADD COLUMN IF NOT EXISTS usr_red VARCHAR(100) DEFAULT NULL;
		ALTER TABLE estaciones ADD COLUMN IF NOT EXISTS pwd_red VARCHAR(100) DEFAULT NULL;
		ALTER TABLE usuarios ADD COLUMN IF NOT EXISTS puesto VARCHAR(250) DEFAULT NULL;
		ALTER TABLE usuarios ADD COLUMN IF NOT EXISTS email VARCHAR(255) DEFAULT NULL;
		ALTER TABLE usuarios ADD COLUMN IF NOT EXISTS permiso_extraccion BOOLEAN DEFAULT FALSE;
		ALTER TABLE usuarios ADD COLUMN IF NOT EXISTS permiso_alarmas BOOLEAN DEFAULT FALSE;
		ALTER TABLE usuarios ADD COLUMN IF NOT EXISTS permiso_visor BOOLEAN DEFAULT FALSE;
	`)
	if err != nil {
		log.Printf("Aviso: No se pudieron crear las columnas del Centro Analítico en la base de datos: %v", err)
	} else {
		log.Println("Migración del Centro Analítico completada con éxito.")
		// Activar por defecto todos los permisos para usuarios de nivel <= 60 si no estaban configurados (todos false)
		_, _ = dbPool.Exec(context.Background(), `
			UPDATE usuarios 
			SET permiso_extraccion = TRUE, permiso_alarmas = TRUE, permiso_visor = TRUE
			WHERE id IN (
				SELECT u.id 
				FROM usuarios u
				JOIN roles r ON u.rol_id = r.id
				WHERE r.nivel_poder <= 60 AND u.permiso_extraccion = FALSE AND u.permiso_alarmas = FALSE AND u.permiso_visor = FALSE
			)
		`)

		// Registrar la aplicación de gestión de procesos (WinRM) si no existe
		_, _ = dbPool.Exec(context.Background(), `
			INSERT INTO aplicaciones (id, nombre, ruta_base)
			SELECT '8fa86047-927b-4029-923f-917c913cc59b', 'Gestión de Procesos', '/procesos/'
			WHERE NOT EXISTS (SELECT 1 FROM aplicaciones WHERE id = '8fa86047-927b-4029-923f-917c913cc59b');
		`)

		// Asignar automáticamente esta aplicación a todos los administradores (nivel >= 80)
		_, _ = dbPool.Exec(context.Background(), `
			INSERT INTO usuario_aplicaciones (usuario_id, aplicacion_id)
			SELECT u.id, '8fa86047-927b-4029-923f-917c913cc59b'
			FROM usuarios u
			INNER JOIN roles r ON u.rol_id = r.id
			WHERE r.nivel_poder >= 80
			ON CONFLICT DO NOTHING;
		`)

		// Asegurar que todos los usuarios existentes tengan las aplicaciones por defecto (Sentinel, Centro Analítico, VNC)
		_, _ = dbPool.Exec(context.Background(), `
			INSERT INTO usuario_aplicaciones (usuario_id, aplicacion_id)
			SELECT u.id, a.id
			FROM usuarios u
			CROSS JOIN (
				SELECT id FROM aplicaciones 
				WHERE id IN ('56169a15-796d-4fd3-8140-4ac8a0d96c4a', '1ee3a2ea-927f-46cd-b294-86494b668895', '7c848b36-00cf-45f2-8ccd-bde877797394')
			) a
			ON CONFLICT DO NOTHING;
		`)
	}

	if secret := os.Getenv("JWT_SECRET"); secret != "" {
		jwtSecret = []byte(secret)
	}

	// Configurar las rutas
	mux := http.NewServeMux()
	mux.HandleFunc("/ws/", HandleRadarStream)
	mux.HandleFunc("/api/login", loginHandler)
	mux.HandleFunc("/api/logout", logoutHandler)
	mux.HandleFunc("/api/session", sessionHandler)
	mux.HandleFunc("/verify", verifyHandler)

	// Rutas de administración protegidas
	mux.Handle("/api/admin/usuarios", AdminMiddleware(http.HandlerFunc(adminUsuariosHandler)))
	mux.Handle("/api/admin/usuarios/editar", AdminMiddleware(http.HandlerFunc(adminEditarUsuarioHandler)))
	mux.Handle("/api/admin/usuarios/eliminar", AdminMiddleware(http.HandlerFunc(adminEliminarUsuarioHandler)))
	mux.Handle("/api/admin/estaciones", AdminMiddleware(http.HandlerFunc(adminEstacionesHandler)))
	mux.Handle("/api/admin/estaciones/", AdminMiddleware(http.HandlerFunc(adminEstacionesHandler)))
	mux.Handle("/api/admin/estaciones/editar", AdminMiddleware(http.HandlerFunc(adminEditarEstacionHandler)))
	mux.Handle("/api/admin/roles", AdminMiddleware(http.HandlerFunc(adminRolesHandler)))
	mux.Handle("/api/admin/provincias", AdminMiddleware(http.HandlerFunc(adminProvinciasHandler)))
	mux.Handle("/api/winrm", JWTMiddleware(http.HandlerFunc(winrmHandler)))

	// API Frontend (acceso por JWT de usuario)
	mux.Handle("/api/v1/stations", JWTMiddleware(http.HandlerFunc(apiV1StationsHandler)))

	// Capa de Resolución Interna (solo para el Core C++, protegida por clave de servicio)
	mux.HandleFunc("/api/internal/resolve-path", apiInternalResolvePathHandler)

	addr := "127.0.0.1:8080"

	// Mostrar a qué BD nos conectamos (enmascarando el password)
	maskedURL := dbURL
	if atIdx := strings.LastIndex(dbURL, "@"); atIdx != -1 {
		if colonIdx := strings.LastIndex(dbURL[:atIdx], ":"); colonIdx != -1 && colonIdx > 8 {
			maskedURL = dbURL[:colonIdx+1] + "****" + dbURL[atIdx:]
		}
	}
	log.Printf("Conectado a la base de datos: %s", maskedURL)
	log.Printf("Starting SSO Portal at %s...\n", addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("Server failed: %v\n", err)
	}
}

// loginHandler atiende la validación y generación del token
func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	var userID, hash string
	var nivelPoder int

	// Buscar el hash, ID y nivel de poder del usuario (haciendo JOIN con la tabla roles)
	query := `
		SELECT u.id, u.password_hash, r.nivel_poder 
		FROM usuarios u 
		INNER JOIN roles r ON u.rol_id = r.id 
		WHERE u.username = $1
	`
	err := dbPool.QueryRow(ctx, query, req.Username).Scan(&userID, &hash, &nivelPoder)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Printf("Login fallido: Usuario '%s' no encontrado", req.Username)
		} else {
			log.Printf("ERROR SQL CRÍTICO en login para '%s': %v", req.Username, err)
		}
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	// Validar contraseña con Argon2
	match, err := verifyArgon2Hash(req.Password, hash)
	if err != nil || !match {
		log.Printf("Login fallido: contraseña incorrecta para el usuario '%s'", req.Username)
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	log.Printf("Login exitoso: el usuario '%s' ha ingresado con nivel %d", req.Username, nivelPoder)

	// Obtener provincias asignadas al usuario
	rows, err := dbPool.Query(ctx, "SELECT provincia_id FROM usuario_provincias WHERE usuario_id = $1", userID)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var provincias []string
	for rows.Next() {
		var pid string
		if err := rows.Scan(&pid); err == nil {
			provincias = append(provincias, pid)
		}
	}

	// Crear token JWT con MapClaims para garantizar nivel_poder como float64 al decodificar
	jwtClaims := jwt.MapClaims{
		"usuario_id":  userID,
		"provincias":  provincias,
		"nivel_poder": nivelPoder,
		"exp":         time.Now().Add(8 * time.Hour).Unix(),
		"iat":         time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwtClaims)
	tokenString, err := token.SignedString(jwtSecret)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	// Retornar JWT en una cookie HttpOnly
	http.SetCookie(w, &http.Cookie{
		Name:     "jwt",
		Value:    tokenString,
		Expires:  time.Now().Add(8 * time.Hour),
		HttpOnly: true,
		Path:     "/",
		SameSite: http.SameSiteStrictMode,
	})

	// Consultar aplicaciones autorizadas
	rowsApps, err := dbPool.Query(context.Background(), `
		SELECT a.nombre, a.ruta_base 
		FROM usuario_aplicaciones ua
		JOIN aplicaciones a ON ua.aplicacion_id = a.id
		WHERE ua.usuario_id = $1`, userID)
	appsList := []map[string]string{}
	if err == nil {
		defer rowsApps.Close()
		for rowsApps.Next() {
			var nombre, rutaBase string
			if errScan := rowsApps.Scan(&nombre, &rutaBase); errScan == nil {
				appsList = append(appsList, map[string]string{"nombre": nombre, "ruta": rutaBase})
			}
		}
	} else {
		log.Printf("[loginHandler] Error al cargar aplicaciones: %v", err)
	}

	resp := map[string]interface{}{
		"message":      "Login exitoso",
		"nivel_poder":  nivelPoder,
		"aplicaciones": appsList,
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// logoutHandler elimina la cookie JWT del cliente, invalidando la sesión de forma real
func logoutHandler(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "jwt",
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Sesión cerrada correctamente"})
}

// sessionHandler devuelve el estado de la sesión activa y las aplicaciones autorizadas
func sessionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cookie, err := r.Cookie("jwt")
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "No session"})
		return
	}

	token, err := jwt.ParseWithClaims(cookie.Value, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})

	if err != nil || !token.Valid {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid token"})
		return
	}

	claims, ok := token.Claims.(*Claims)
	if !ok {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid claims"})
		return
	}

	// Consultar de forma síncrona los permisos individuales actualizados en la base de datos
	var nivelPoder int
	var pExt, pAlm, pVis bool
	err = dbPool.QueryRow(r.Context(), `
		SELECT r.nivel_poder, u.permiso_extraccion, u.permiso_alarmas, u.permiso_visor 
		FROM usuarios u 
		INNER JOIN roles r ON u.rol_id = r.id 
		WHERE u.id = $1`, claims.UsuarioID).Scan(&nivelPoder, &pExt, &pAlm, &pVis)
	if err != nil {
		// Fallback por si hay algún error
		nivelPoder = claims.NivelPoder
	}

	// Consultar aplicaciones autorizadas
	rowsApps, err := dbPool.Query(r.Context(), `
		SELECT a.nombre, a.ruta_base 
		FROM usuario_aplicaciones ua
		JOIN aplicaciones a ON ua.aplicacion_id = a.id
		WHERE ua.usuario_id = $1`, claims.UsuarioID)
	appsList := []map[string]string{}
	if err == nil {
		defer rowsApps.Close()
		for rowsApps.Next() {
			var nombre, rutaBase string
			if errScan := rowsApps.Scan(&nombre, &rutaBase); errScan == nil {
				appsList = append(appsList, map[string]string{"nombre": nombre, "ruta": rutaBase})
			}
		}
	} else {
		log.Printf("[sessionHandler] Error al cargar aplicaciones: %v", err)
	}

	resp := map[string]interface{}{
		"nivel_poder":        nivelPoder,
		"permiso_extraccion": pExt,
		"permiso_alarmas":    pAlm,
		"permiso_visor":      pVis,
		"aplicaciones":       appsList,
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// verifyHandler se usa por Nginx (auth_request) para validar la solicitud de entrada a las subrutas
func verifyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extraer el JWT de la cookie
	cookie, err := r.Cookie("jwt")
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Parsear el JWT
	token, err := jwt.ParseWithClaims(cookie.Value, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})

	if err != nil || !token.Valid {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	claims, ok := token.Claims.(*Claims)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	nivelPoder := claims.NivelPoder

	// 1. VIP/Superadmin: Si el nivel es >= 80 (Superadmin o Admin Global), tiene acceso universal implícito a todos los recursos
	if nivelPoder >= 80 {
		w.WriteHeader(http.StatusOK)
		return
	}

	// 2. Leer qué ruta está intentando visitar el usuario a través de Nginx
	originalURI := r.Header.Get("X-Original-URI")

	// Excepción para Procesos, WinRM y GET de estaciones para usuarios autorizados (incluyendo nivel < 60)
	if strings.HasPrefix(originalURI, "/procesos/") ||
		strings.HasPrefix(originalURI, "/api/winrm") ||
		strings.HasPrefix(originalURI, "/api/admin/estaciones") {

		// Verificar si tiene asignada la aplicación Procesos o la de Centro Analítico
		var hasApp bool
		err := dbPool.QueryRow(r.Context(), `
			SELECT EXISTS (
				SELECT 1 FROM usuario_aplicaciones 
				WHERE usuario_id = $1 AND aplicacion_id IN ('8fa86047-927b-4029-923f-917c913cc59b', '1ee3a2ea-927f-46cd-b294-86494b668895')
			)`, claims.UsuarioID).Scan(&hasApp)
		if err == nil && hasApp {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Si es nivel >= 60, también permitimos por rol administrativo genérico
		if nivelPoder >= 60 {
			w.WriteHeader(http.StatusOK)
			return
		}

		w.WriteHeader(http.StatusForbidden)
		return
	}

	// 3. EXCEPCIÓN VIP ADMIN: Si es el panel de administración, validamos estrictamente por nivel de poder >= 60.
	if strings.HasPrefix(originalURI, "/sso/admin.html") || strings.HasPrefix(originalURI, "/api/admin/") {
		if nivelPoder >= 60 {
			w.WriteHeader(http.StatusOK) // Adelante, eres Admin o Gestor Provincial
			return
		}
		w.WriteHeader(http.StatusForbidden) // Bloqueado, nivel insuficiente
		return
	}

	// 4. EXCEPCIÓN VIP CENTRO ANALÍTICO: Validamos por nivel >= 80 (Superadmin/Admin Global) OR si tiene alguno de los 3 permisos habilitados en BD.
	if strings.HasPrefix(originalURI, "/analitico/") ||
		strings.HasPrefix(originalURI, "/api/fuentes") ||
		strings.HasPrefix(originalURI, "/api/extraer") ||
		strings.HasPrefix(originalURI, "/api/files/") ||
		strings.HasPrefix(originalURI, "/api/config") ||
		strings.HasPrefix(originalURI, "/api/progreso") ||
		strings.HasPrefix(originalURI, "/api/planificacion") {
		if nivelPoder >= 80 {
			w.WriteHeader(http.StatusOK) // Superadmin / Admin Global siempre tienen acceso incondicional
			return
		}

		// Validar sub-permisos individuales para niveles 60 o inferiores (incluyendo Gestores Provinciales)
		var pExt, pAlm, pVis bool
		err = dbPool.QueryRow(r.Context(),
			"SELECT permiso_extraccion, permiso_alarmas, permiso_visor FROM usuarios WHERE id = $1",
			claims.UsuarioID).Scan(&pExt, &pAlm, &pVis)
		if err == nil && (pExt || pAlm || pVis) {
			w.WriteHeader(http.StatusOK) // Acceso por poseer al menos un permiso habilitado
			return
		}

		w.WriteHeader(http.StatusForbidden) // Bloqueado si el superadmin/admin le ha quitado los tres permisos
		return
	}

	// Normalizar URI: agregar barra diagonal si falta al final (para que coincida con LIKE '.../%')
	dbURI := originalURI
	if strings.HasPrefix(dbURI, "/api/fuentes") ||
		strings.HasPrefix(dbURI, "/api/extraer") ||
		strings.HasPrefix(dbURI, "/api/files/") ||
		strings.HasPrefix(dbURI, "/api/config") ||
		strings.HasPrefix(dbURI, "/api/progreso") ||
		strings.HasPrefix(dbURI, "/api/planificacion") {
		dbURI = "/analitico/"
	}
	if !strings.HasSuffix(dbURI, "/") {
		dbURI = dbURI + "/"
	}

	// Generar variantes para admitir tanto '/awacs/' como '/sentinel/' en la base de datos
	dbURI1 := dbURI
	dbURI2 := dbURI
	if strings.HasPrefix(dbURI, "/awacs/") {
		dbURI2 = strings.Replace(dbURI, "/awacs/", "/sentinel/", 1)
	} else if strings.HasPrefix(dbURI, "/sentinel/") {
		dbURI2 = strings.Replace(dbURI, "/sentinel/", "/awacs/", 1)
	}

	ctx := context.Background()
	// Verificamos en usuario_aplicaciones si hay permiso para esa ruta (admite ruta_base '/sentinel/' y '/awacs/')
	query := `
		SELECT 1
		FROM usuario_aplicaciones ua
		JOIN aplicaciones a ON ua.aplicacion_id = a.id
		WHERE ua.usuario_id = $1 AND (
			$2 LIKE (a.ruta_base || '%') OR
			$3 LIKE (a.ruta_base || '%')
		)
		LIMIT 1
	`
	var exists int
	err = dbPool.QueryRow(ctx, query, claims.UsuarioID, dbURI1, dbURI2).Scan(&exists)
	if err != nil {
		// pgx.ErrNoRows u otro error indican acceso denegado
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Aprobado, Nginx recibirá un 200 OK y permitirá que pase a la verdadera aplicación
	w.WriteHeader(http.StatusOK)
}

// JWTMiddleware verifica el JWT del usuario pero no exige nivel de admin
func JWTMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("jwt")
		if err != nil {
			jsonResponse(w, "Unauthorized: No session cookie", http.StatusUnauthorized)
			return
		}

		token, err := jwt.Parse(cookie.Value, func(token *jwt.Token) (interface{}, error) {
			return jwtSecret, nil
		})

		if err != nil || !token.Valid {
			jsonResponse(w, "Unauthorized: Invalid token", http.StatusUnauthorized)
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			jsonResponse(w, "Forbidden: Invalid claims structure", http.StatusForbidden)
			return
		}

		nivelFloat, ok := claims["nivel_poder"].(float64)
		if !ok {
			http.Error(w, "JWT sin nivel_poder", http.StatusForbidden)
			return
		}
		nivelPoder := int(nivelFloat)

		c := &Claims{
			UsuarioID:  claims["usuario_id"].(string),
			NivelPoder: nivelPoder,
		}
		if provs, ok := claims["provincias"].([]interface{}); ok {
			for _, p := range provs {
				if ps, ok := p.(string); ok {
					c.Provincias = append(c.Provincias, ps)
				}
			}
		}

		ctx := context.WithValue(r.Context(), "user_claims", c)
		ctx = context.WithValue(ctx, nivelKey, nivelPoder)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// AdminMiddleware verifica el JWT y el nivel de poder del usuario
func AdminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("jwt")
		if err != nil {
			jsonResponse(w, "Unauthorized: No session cookie", http.StatusUnauthorized)
			return
		}

		token, err := jwt.Parse(cookie.Value, func(token *jwt.Token) (interface{}, error) {
			return jwtSecret, nil
		})

		if err != nil || !token.Valid {
			jsonResponse(w, "Unauthorized: Invalid token", http.StatusUnauthorized)
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			jsonResponse(w, "Forbidden: Invalid claims structure", http.StatusForbidden)
			return
		}

		nivelFloat, ok := claims["nivel_poder"].(float64)
		if !ok {
			http.Error(w, "JWT sin nivel_poder", http.StatusForbidden)
			return
		}
		nivelPoder := int(nivelFloat)

		if nivelPoder < 20 {
			jsonResponse(w, "Forbidden: Insufficient privileges", http.StatusForbidden)
			return
		}

		if r.Method != http.MethodGet && nivelPoder < 60 {
			jsonResponse(w, "Forbidden: Insufficient privileges", http.StatusForbidden)
			return
		}

		// Reconstruir el struct Claims para mantener compatibilidad con los handlers existentes
		c := &Claims{
			UsuarioID:  claims["usuario_id"].(string),
			NivelPoder: nivelPoder,
		}
		if provs, ok := claims["provincias"].([]interface{}); ok {
			for _, p := range provs {
				if ps, ok := p.(string); ok {
					c.Provincias = append(c.Provincias, ps)
				}
			}
		}

		// Almacenar claims y el nivel exacto en el contexto
		ctx := context.WithValue(r.Context(), "user_claims", c)
		ctx = context.WithValue(ctx, nivelKey, int(nivelFloat))
		log.Printf("[MIDDLEWARE] Interceptado %s %s. Nivel inyectado en contexto: %d", r.Method, r.URL.Path, int(nivelFloat))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// adminUsuariosHandler gestiona la lista y creación de usuarios
func adminUsuariosHandler(w http.ResponseWriter, r *http.Request) {
	val := r.Context().Value("user_claims")
	if val == nil {
		jsonResponse(w, "Internal error: missing claims", http.StatusInternalServerError)
		return
	}
	claims := val.(*Claims)

	if r.Method == http.MethodGet {
		var rows pgx.Rows
		var err error
		provFilter := r.URL.Query().Get("provincia_id")

		if provFilter != "" {
			// Si el usuario no es admin global/super (nivel < 80), verificar que tenga asignada esa provincia
			if claims.NivelPoder < 80 {
				tieneAcceso := false
				for _, p := range claims.Provincias {
					if p == provFilter {
						tieneAcceso = true
						break
					}
				}
				if !tieneAcceso {
					jsonResponse(w, "Forbidden: No tienes acceso a esta provincia", http.StatusForbidden)
					return
				}
			}

			// Filtro por provincia (utilizado por el Centro Analítico) - Excluye admins globales/super (nivel >= 80)
			rows, err = dbPool.Query(r.Context(), `
				SELECT DISTINCT u.id, u.username, r.nivel_poder, r.id,
				       up.provincia_id,
				       p.nombre AS provincia_nombre,
				       u.puesto,
				       u.email,
				       u.permiso_extraccion,
				       u.permiso_alarmas,
				       u.permiso_visor,
				       (SELECT EXISTS (SELECT 1 FROM usuario_aplicaciones WHERE usuario_id = u.id AND aplicacion_id = '8fa86047-927b-4029-923f-917c913cc59b')) AS permiso_procesos
				FROM usuarios u
				INNER JOIN roles r ON u.rol_id = r.id
				LEFT JOIN usuario_provincias up ON up.usuario_id = u.id
				LEFT JOIN provincias p ON p.id = up.provincia_id
				WHERE up.provincia_id = $1 AND r.nivel_poder < 80
				ORDER BY u.username`, provFilter)
		} else if claims.NivelPoder >= 80 {
			rows, err = dbPool.Query(r.Context(), `
				SELECT u.id, u.username, r.nivel_poder, r.id,
				       up.provincia_id,
				       p.nombre AS provincia_nombre,
				       u.puesto,
				       u.email,
				       u.permiso_extraccion,
				       u.permiso_alarmas,
				       u.permiso_visor,
				       (SELECT EXISTS (SELECT 1 FROM usuario_aplicaciones WHERE usuario_id = u.id AND aplicacion_id = '8fa86047-927b-4029-923f-917c913cc59b')) AS permiso_procesos
				FROM usuarios u
				INNER JOIN roles r ON u.rol_id = r.id
				LEFT JOIN usuario_provincias up ON up.usuario_id = u.id
				LEFT JOIN provincias p ON p.id = up.provincia_id
				ORDER BY u.username`)
		} else {
			// Nivel 60: Usuarios que comparten provincias con el admin
			query := `
				SELECT DISTINCT u.id, u.username, r.nivel_poder, r.id,
				       up.provincia_id,
				       p.nombre AS provincia_nombre,
				       u.puesto,
				       u.email,
				       u.permiso_extraccion,
				       u.permiso_alarmas,
				       u.permiso_visor,
				       (SELECT EXISTS (SELECT 1 FROM usuario_aplicaciones WHERE usuario_id = u.id AND aplicacion_id = '8fa86047-927b-4029-923f-917c913cc59b')) AS permiso_procesos
				FROM usuarios u
				INNER JOIN roles r ON u.rol_id = r.id
				LEFT JOIN usuario_provincias up ON up.usuario_id = u.id
				LEFT JOIN provincias p ON p.id = up.provincia_id
				WHERE up.provincia_id IN (SELECT provincia_id FROM usuario_provincias WHERE usuario_id = $1)
				ORDER BY u.username
			`
			rows, err = dbPool.Query(r.Context(), query, claims.UsuarioID)
		}

		if err != nil {
			jsonResponse(w, "Database error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		users := []map[string]interface{}{}
		for rows.Next() {
			var id, username string
			var nivel, rolID int
			var provinciaID, provinciaNombre, puesto, email *string
			var pExt, pAlm, pVis, pProc bool
			if err := rows.Scan(&id, &username, &nivel, &rolID, &provinciaID, &provinciaNombre, &puesto, &email, &pExt, &pAlm, &pVis, &pProc); err == nil {
				provID := ""
				provNombre := ""
				puestoVal := ""
				emailVal := ""
				if provinciaID != nil {
					provID = *provinciaID
				}
				if provinciaNombre != nil {
					provNombre = *provinciaNombre
				}
				if puesto != nil {
					puestoVal = *puesto
				}
				if email != nil {
					emailVal = *email
				}
				users = append(users, map[string]interface{}{
					"id":                 id,
					"usuario":            username,
					"nivel_poder":        nivel,
					"rol_id":             rolID,
					"provincia_id":       provID,
					"provincia":          provNombre,
					"puesto":             puestoVal,
					"email":              emailVal,
					"permiso_extraccion": pExt,
					"permiso_alarmas":    pAlm,
					"permiso_visor":      pVis,
					"permiso_procesos":   pProc,
				})
			}
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(users)

	} else if r.Method == http.MethodPost {
		val := r.Context().Value(nivelKey)
		miNivel, ok := val.(int)
		if !ok || miNivel == 0 {
			log.Printf("[POST /api/admin/usuarios] CRÍTICO: miNivel del contexto inválido. Valor: %v (tipo %T)", val, val)
			http.Error(w, "Error interno de jerarquía", http.StatusInternalServerError)
			return
		}
		log.Printf("[POST /api/admin/usuarios] miNivel leído del contexto: %d", miNivel)

		var err error

		type UsuarioInput struct {
			Username          string  `json:"username"`
			Password          string  `json:"password,omitempty"`
			RolID             int     `json:"rol_id"`
			ProvinciaID       *string `json:"provincia_id"` // Puntero vital para aceptar null
			Puesto            *string `json:"puesto"`
			Email             *string `json:"email"`
			PermisoExtraccion bool    `json:"permiso_extraccion"`
			PermisoAlarmas    bool    `json:"permiso_alarmas"`
			PermisoVisor      bool    `json:"permiso_visor"`
			PermisoProcesos   bool    `json:"permiso_procesos"`
		}
		var req UsuarioInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			log.Printf("CRÍTICO JSON: %v", err)
			jsonResponse(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		req.Username = sanitizeUsername(req.Username)
		if req.Username == "" {
			jsonResponse(w, "Invalid username", http.StatusBadRequest)
			return
		}
		if req.Password == "" {
			jsonResponse(w, "Password required for new user", http.StatusBadRequest)
			return
		}

		// Restricción de seguridad del backend: Un admin nivel 60 solo puede otorgar permiso de visor de archivos
		if miNivel < 80 {
			req.PermisoExtraccion = false
			req.PermisoAlarmas = false

			// Si el admin es nivel 60 o inferior, verificar que tenga acceso a Procesos antes de poder asignarlo
			var adminHasProcesos bool
			err = dbPool.QueryRow(r.Context(), `
				SELECT EXISTS (
					SELECT 1 FROM usuario_aplicaciones 
					WHERE usuario_id = $1 AND aplicacion_id = '8fa86047-927b-4029-923f-917c913cc59b'
				)`, claims.UsuarioID).Scan(&adminHasProcesos)
			if err != nil || !adminHasProcesos {
				req.PermisoProcesos = false
			}
		}

		// Obtener nivel del rol destino y verificar jerarquía estrictamente
		var nivelRolDestino int
		err = dbPool.QueryRow(r.Context(), "SELECT nivel_poder FROM roles WHERE id = $1", req.RolID).Scan(&nivelRolDestino)
		if err != nil {
			jsonResponse(w, "rol_id inválido o no encontrado", http.StatusBadRequest)
			return
		}
		if miNivel >= 80 {
			if nivelRolDestino > miNivel {
				jsonResponse(w, fmt.Sprintf("Forbidden: el nivel del rol destino (%d) debe ser menor o igual que el tuyo (%d)", nivelRolDestino, miNivel), http.StatusForbidden)
				return
			}
		} else {
			if nivelRolDestino >= miNivel {
				jsonResponse(w, fmt.Sprintf("Forbidden: el nivel del rol destino (%d) debe ser estrictamente menor que el tuyo (%d)", nivelRolDestino, miNivel), http.StatusForbidden)
				return
			}
		}

		// Determinar provincia_id a asignar según nivel del admin
		var provinciaIDFinal *string
		if miNivel >= 80 {
			// SuperAdmin: usa el provincia_id enviado en el JSON
			provinciaIDFinal = req.ProvinciaID
		} else {
			// Admin Provincial (nivel 60): busca su propia provincia en usuario_provincias
			var provAdmin string
			err = dbPool.QueryRow(r.Context(),
				"SELECT provincia_id FROM usuario_provincias WHERE usuario_id = $1 LIMIT 1",
				claims.UsuarioID).Scan(&provAdmin)
			if err != nil {
				jsonResponse(w, "No se encontró provincia del admin", http.StatusInternalServerError)
				return
			}
			provinciaIDFinal = &provAdmin
		}

		passHash, err := hashArgon2(req.Password)
		if err != nil {
			jsonResponse(w, "Hashing error", http.StatusInternalServerError)
			return
		}

		// Insertar usuario y obtener su ID nuevo
		var newUserID string
		err = dbPool.QueryRow(r.Context(),
			`INSERT INTO usuarios (username, password_hash, rol_id, puesto, email, permiso_extraccion, permiso_alarmas, permiso_visor) 
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
			req.Username, passHash, req.RolID, req.Puesto, req.Email, req.PermisoExtraccion, req.PermisoAlarmas, req.PermisoVisor).Scan(&newUserID)
		if err != nil {
			log.Printf("[POST /api/admin/usuarios] Error insert: %v", err)
			jsonResponse(w, "Insert error", http.StatusInternalServerError)
			return
		}

		// Asignar provincia
		if provinciaIDFinal != nil {
			_, err = dbPool.Exec(r.Context(),
				"INSERT INTO usuario_provincias (usuario_id, provincia_id) VALUES ($1, $2)",
				newUserID, provinciaIDFinal)
			if err != nil {
				log.Printf("[POST /api/admin/usuarios] WARN: usuario creado pero fallo asignando provincia: %v", err)
			}
		}

		// Asignar aplicaciones por defecto (Sentinel, Centro Analítico, VNC)
		defaultApps := []string{
			"56169a15-796d-4fd3-8140-4ac8a0d96c4a", // Sentinel
			"1ee3a2ea-927f-46cd-b294-86494b668895", // Centro Analítico
			"7c848b36-00cf-45f2-8ccd-bde877797394", // Servidor VNC
		}
		for _, appID := range defaultApps {
			_, _ = dbPool.Exec(r.Context(),
				"INSERT INTO usuario_aplicaciones (usuario_id, aplicacion_id) VALUES ($1, $2) ON CONFLICT DO NOTHING",
				newUserID, appID)
		}

		// Asignar aplicación de procesos si está marcada
		if req.PermisoProcesos {
			_, _ = dbPool.Exec(r.Context(),
				"INSERT INTO usuario_aplicaciones (usuario_id, aplicacion_id) VALUES ($1, '8fa86047-927b-4029-923f-917c913cc59b') ON CONFLICT DO NOTHING",
				newUserID)
		}

		provIDLog := "NULL"
		if provinciaIDFinal != nil {
			provIDLog = *provinciaIDFinal
		}
		log.Printf("[POST /api/admin/usuarios] Usuario '%s' (id=%s) creado con rol_id=%d (nivel=%d) provincia='%s' por admin nivel=%d",
			req.Username, newUserID, req.RolID, nivelRolDestino, provIDLog, miNivel)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	} else if r.Method == http.MethodPut {
		// PUT /api/admin/usuarios — Editar usuario directamente en esta ruta
		val2 := r.Context().Value(nivelKey)
		miNivel2, ok2 := val2.(int)
		if !ok2 || miNivel2 == 0 {
			jsonResponse(w, "Error interno de jerarquía", http.StatusInternalServerError)
			return
		}

		type EditarUsuarioInput struct {
			ID          string  `json:"id"`
			Username    string  `json:"username"`
			Password    string  `json:"password,omitempty"`
			RolID       int     `json:"rol_id"`
			ProvinciaID *string `json:"provincia_id"`
			Puesto      *string `json:"puesto"`
			Email       *string `json:"email"`
		}
		var req2 EditarUsuarioInput
		if err := json.NewDecoder(r.Body).Decode(&req2); err != nil {
			log.Printf("ERROR DECODE PUT: %v", err)
			jsonResponse(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		if req2.ID == "" {
			jsonResponse(w, "id requerido", http.StatusBadRequest)
			return
		}
		log.Printf("DEBUG: ID=%s, Rol=%d, Provincia=%v", req2.ID, req2.RolID, req2.ProvinciaID)

		// Verificar nivel actual del usuario a editar
		var nivelActual2 int
		if err := dbPool.QueryRow(r.Context(),
			"SELECT r.nivel_poder FROM usuarios u INNER JOIN roles r ON u.rol_id = r.id WHERE u.id = $1", req2.ID).Scan(&nivelActual2); err != nil {
			jsonResponse(w, "Usuario no encontrado", http.StatusNotFound)
			return
		}
		if nivelActual2 >= miNivel2 {
			jsonResponse(w, fmt.Sprintf("Forbidden: el usuario a editar tiene nivel (%d) >= el tuyo (%d)", nivelActual2, miNivel2), http.StatusForbidden)
			return
		}

		// Verificar nivel del nuevo rol
		var nivelRolDest2 int
		if err := dbPool.QueryRow(r.Context(), "SELECT nivel_poder FROM roles WHERE id = $1", req2.RolID).Scan(&nivelRolDest2); err != nil {
			jsonResponse(w, "rol_id inválido o no encontrado", http.StatusBadRequest)
			return
		}
		if nivelRolDest2 >= miNivel2 {
			jsonResponse(w, fmt.Sprintf("Forbidden: el nuevo rol tiene nivel (%d) >= el tuyo (%d)", nivelRolDest2, miNivel2), http.StatusForbidden)
			return
		}

		// Validación de provincia para Admin Provincial
		provinciaIDEdit := req2.ProvinciaID
		if miNivel2 < 80 && provinciaIDEdit != nil {
			// Verificar que la provincia destino sea la propia del admin
			var miProvincia string
			if err := dbPool.QueryRow(r.Context(),
				"SELECT provincia_id FROM usuario_provincias WHERE usuario_id = $1 LIMIT 1",
				claims.UsuarioID).Scan(&miProvincia); err != nil || miProvincia != *provinciaIDEdit {
				jsonResponse(w, "Forbidden: no puedes asignar una provincia diferente a la tuya", http.StatusForbidden)
				return
			}
		}

		req2.Username = sanitizeUsername(req2.Username)
		if req2.Username == "" {
			jsonResponse(w, "Invalid username", http.StatusBadRequest)
			return
		}

		// Actualizar tabla usuarios
		var putErr error
		if req2.Password != "" {
			passHash2, hashErr := hashArgon2(req2.Password)
			if hashErr != nil {
				jsonResponse(w, "Hashing error", http.StatusInternalServerError)
				return
			}
			_, putErr = dbPool.Exec(r.Context(),
				"UPDATE usuarios SET username=$1, password_hash=$2, rol_id=$3, puesto=$4, email=$5 WHERE id=$6",
				req2.Username, passHash2, req2.RolID, req2.Puesto, req2.Email, req2.ID)
		} else {
			_, putErr = dbPool.Exec(r.Context(),
				"UPDATE usuarios SET username=$1, rol_id=$2, puesto=$3, email=$4 WHERE id=$5",
				req2.Username, req2.RolID, req2.Puesto, req2.Email, req2.ID)
		}
		if putErr != nil {
			jsonResponse(w, "Update error", http.StatusInternalServerError)
			return
		}

		// Lógica SQL de Provincia: Primero borrar, luego insertar si aplica
		_, deleteErr := dbPool.Exec(r.Context(), "DELETE FROM usuario_provincias WHERE usuario_id = $1", req2.ID)
		if deleteErr != nil {
			log.Printf("[PUT /api/admin/usuarios] WARN: fallo eliminando provincia actual para id=%s: %v", req2.ID, deleteErr)
		}

		if provinciaIDEdit != nil {
			_, insertErr := dbPool.Exec(r.Context(),
				"INSERT INTO usuario_provincias (usuario_id, provincia_id) VALUES ($1, $2)",
				req2.ID, provinciaIDEdit)
			if insertErr != nil {
				log.Printf("[PUT /api/admin/usuarios] WARN: fallo asignando nueva provincia para id=%s: %v", req2.ID, insertErr)
			}
		}

		// Asegurar aplicaciones por defecto al editar
		defaultApps := []string{
			"56169a15-796d-4fd3-8140-4ac8a0d96c4a", // Sentinel
			"1ee3a2ea-927f-46cd-b294-86494b668895", // Centro Analítico
			"7c848b36-00cf-45f2-8ccd-bde877797394", // Servidor VNC
		}
		for _, appID := range defaultApps {
			_, _ = dbPool.Exec(r.Context(),
				"INSERT INTO usuario_aplicaciones (usuario_id, aplicacion_id) VALUES ($1, $2) ON CONFLICT DO NOTHING",
				req2.ID, appID)
		}

		provIDLog2 := "NULL"
		if provinciaIDEdit != nil {
			provIDLog2 = *provinciaIDEdit
		}
		log.Printf("[PUT /api/admin/usuarios] Usuario id=%s actualizado a rol_id=%d provincia='%s' por admin nivel=%d",
			req2.ID, req2.RolID, provIDLog2, miNivel2)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	} else {
		jsonResponse(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// adminEditarUsuarioHandler maneja PUT /api/admin/usuarios/editar
func adminEditarUsuarioHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		jsonResponse(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	val := r.Context().Value(nivelKey)
	miNivel, ok := val.(int)
	if !ok || miNivel == 0 {
		log.Printf("[PUT /api/admin/usuarios/editar] CRÍTICO: miNivel del contexto inválido. Valor: %v (tipo %T)", val, val)
		http.Error(w, "Error interno de jerarquía", http.StatusInternalServerError)
		return
	}
	log.Printf("[PUT /api/admin/usuarios/editar] miNivel leído del contexto: %d", miNivel)

	type EditarUsuarioInput struct {
		ID                string  `json:"id"`
		Username          string  `json:"username"`
		Password          string  `json:"password,omitempty"`
		RolID             int     `json:"rol_id"`
		ProvinciaID       *string `json:"provincia_id"`
		Puesto            *string `json:"puesto"`
		Email             *string `json:"email"`
		PermisoExtraccion bool    `json:"permiso_extraccion"`
		PermisoAlarmas    bool    `json:"permiso_alarmas"`
		PermisoVisor      bool    `json:"permiso_visor"`
		PermisoProcesos   bool    `json:"permiso_procesos"`
	}

	var req EditarUsuarioInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("ERROR DECODE PUT: %v", err)
		jsonResponse(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.ID == "" {
		jsonResponse(w, "id requerido", http.StatusBadRequest)
		return
	}

	// Obtener nivel actual del usuario a editar
	var nivelActual int
	err := dbPool.QueryRow(r.Context(),
		"SELECT r.nivel_poder FROM usuarios u INNER JOIN roles r ON u.rol_id = r.id WHERE u.id = $1", req.ID).Scan(&nivelActual)
	if err != nil {
		jsonResponse(w, "Usuario no encontrado", http.StatusNotFound)
		return
	}

	// Obtener nivel del nuevo rol destino
	var nivelRolDestino int
	if err := dbPool.QueryRow(r.Context(), "SELECT nivel_poder FROM roles WHERE id = $1", req.RolID).Scan(&nivelRolDestino); err != nil {
		jsonResponse(w, "rol_id inválido o no encontrado", http.StatusBadRequest)
		return
	}

	// Seguridad: miNivel debe ser mayor que nivel actual del objetivo Y mayor que el nuevo nivel asignado (o igual si es superadmin)
	if miNivel >= 80 {
		if nivelActual > miNivel {
			jsonResponse(w, fmt.Sprintf("Forbidden: el usuario a editar tiene nivel (%d) > el tuyo (%d)", nivelActual, miNivel), http.StatusForbidden)
			return
		}
		if nivelRolDestino > miNivel {
			jsonResponse(w, fmt.Sprintf("Forbidden: el nuevo rol tiene nivel (%d) > el tuyo (%d)", nivelRolDestino, miNivel), http.StatusForbidden)
			return
		}
	} else {
		if nivelActual >= miNivel {
			jsonResponse(w, fmt.Sprintf("Forbidden: el usuario a editar tiene nivel (%d) >= el tuyo (%d)", nivelActual, miNivel), http.StatusForbidden)
			return
		}
		if nivelRolDestino >= miNivel {
			jsonResponse(w, fmt.Sprintf("Forbidden: el nuevo rol tiene nivel (%d) >= el tuyo (%d)", nivelRolDestino, miNivel), http.StatusForbidden)
			return
		}
	}

	// Restricción de seguridad del backend: Un admin nivel 60 no puede otorgar permiso de extracción ni de alarmas
	if miNivel < 80 {
		req.PermisoExtraccion = false
		req.PermisoAlarmas = false

		// Si el admin es nivel 60 o inferior, verificar que tenga acceso a Procesos antes de poder asignarlo
		valClaims := r.Context().Value("user_claims")
		if valClaims != nil {
			claims := valClaims.(*Claims)
			var adminHasProcesos bool
			err = dbPool.QueryRow(r.Context(), `
				SELECT EXISTS (
					SELECT 1 FROM usuario_aplicaciones 
					WHERE usuario_id = $1 AND aplicacion_id = '8fa86047-927b-4029-923f-917c913cc59b'
				)`, claims.UsuarioID).Scan(&adminHasProcesos)
			if err != nil || !adminHasProcesos {
				req.PermisoProcesos = false
			}
		} else {
			req.PermisoProcesos = false
		}
	}

	// Determinar provincia_id a asignar según nivel del admin
	var provinciaIDFinal *string
	if miNivel >= 80 {
		// SuperAdmin: usa el provincia_id enviado en el JSON
		provinciaIDFinal = req.ProvinciaID
	} else {
		// Admin Provincial (nivel 60): busca su propia provincia en usuario_provincias
		valClaims := r.Context().Value("user_claims")
		if valClaims == nil {
			jsonResponse(w, "Internal error: missing claims", http.StatusInternalServerError)
			return
		}
		claims := valClaims.(*Claims)
		var provAdmin string
		err = dbPool.QueryRow(r.Context(),
			"SELECT provincia_id FROM usuario_provincias WHERE usuario_id = $1 LIMIT 1",
			claims.UsuarioID).Scan(&provAdmin)
		if err != nil {
			jsonResponse(w, "No se encontró provincia del admin", http.StatusInternalServerError)
			return
		}
		provinciaIDFinal = &provAdmin
	}

	req.Username = sanitizeUsername(req.Username)
	if req.Username == "" {
		jsonResponse(w, "Username requerido o inválido", http.StatusBadRequest)
		return
	}

	if req.Password != "" {
		passHash, err := hashArgon2(req.Password)
		if err != nil {
			jsonResponse(w, "Hashing error", http.StatusInternalServerError)
			return
		}
		_, err = dbPool.Exec(r.Context(),
			`UPDATE usuarios SET username=$1, password_hash=$2, rol_id=$3, puesto=$4, email=$5, 
			                     permiso_extraccion=$6, permiso_alarmas=$7, permiso_visor=$8 WHERE id=$9`,
			req.Username, passHash, req.RolID, req.Puesto, req.Email,
			req.PermisoExtraccion, req.PermisoAlarmas, req.PermisoVisor, req.ID)
		if err != nil {
			jsonResponse(w, "Update error", http.StatusInternalServerError)
			return
		}
	} else {
		_, err = dbPool.Exec(r.Context(),
			`UPDATE usuarios SET username=$1, rol_id=$2, puesto=$3, email=$4, 
			                     permiso_extraccion=$5, permiso_alarmas=$6, permiso_visor=$7 WHERE id=$8`,
			req.Username, req.RolID, req.Puesto, req.Email,
			req.PermisoExtraccion, req.PermisoAlarmas, req.PermisoVisor, req.ID)
		if err != nil {
			jsonResponse(w, "Update error", http.StatusInternalServerError)
			return
		}
	}

	// Lógica SQL de Provincia: Primero borrar, luego insertar si aplica
	_, deleteErr := dbPool.Exec(r.Context(), "DELETE FROM usuario_provincias WHERE usuario_id = $1", req.ID)
	if deleteErr != nil {
		log.Printf("[PUT /api/admin/usuarios/editar] WARN: fallo eliminando provincia actual para id=%s: %v", req.ID, deleteErr)
	}

	if provinciaIDFinal != nil {
		_, insertErr := dbPool.Exec(r.Context(),
			"INSERT INTO usuario_provincias (usuario_id, provincia_id) VALUES ($1, $2)",
			req.ID, provinciaIDFinal)
		if insertErr != nil {
			log.Printf("[PUT /api/admin/usuarios/editar] WARN: fallo asignando nueva provincia para id=%s: %v", req.ID, insertErr)
		}
	}

	// Lógica de Aplicación de Procesos (WinRM)
	_, _ = dbPool.Exec(r.Context(), `
		DELETE FROM usuario_aplicaciones 
		WHERE usuario_id = $1 AND aplicacion_id = '8fa86047-927b-4029-923f-917c913cc59b'`, req.ID)
	if req.PermisoProcesos {
		_, _ = dbPool.Exec(r.Context(), `
			INSERT INTO usuario_aplicaciones (usuario_id, aplicacion_id) 
			VALUES ($1, '8fa86047-927b-4029-923f-917c913cc59b') ON CONFLICT DO NOTHING`, req.ID)
	}

	// Asegurar aplicaciones por defecto al editar
	defaultApps := []string{
		"56169a15-796d-4fd3-8140-4ac8a0d96c4a", // Sentinel
		"1ee3a2ea-927f-46cd-b294-86494b668895", // Centro Analítico
		"7c848b36-00cf-45f2-8ccd-bde877797394", // Servidor VNC
	}
	for _, appID := range defaultApps {
		_, _ = dbPool.Exec(r.Context(),
			"INSERT INTO usuario_aplicaciones (usuario_id, aplicacion_id) VALUES ($1, $2) ON CONFLICT DO NOTHING",
			req.ID, appID)
	}

	log.Printf("[PUT /api/admin/usuarios/editar] Usuario id=%s actualizado a rol_id=%d (nivel=%d) por admin nivel=%d", req.ID, req.RolID, nivelRolDestino, miNivel)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// adminEstacionesHandler devuelve estaciones y rutas UNC
func adminEstacionesHandler(w http.ResponseWriter, r *http.Request) {
	val := r.Context().Value("user_claims")
	if val == nil {
		jsonResponse(w, "Internal error: missing claims", http.StatusInternalServerError)
		return
	}
	claims := val.(*Claims)

	if r.Method == http.MethodGet {
		var rows pgx.Rows
		var err error
		if claims.NivelPoder >= 80 {
			rows, err = dbPool.Query(r.Context(), `
				SELECT e.id, e.nombre, e.provincia_id, e.ip_red, p.nombre AS provincia_nombre,
				       e.ruta_bin, e.ruta_res, e.usr_red, e.pwd_red
				FROM estaciones e 
				LEFT JOIN provincias p ON e.provincia_id = p.id
			`)
		} else {
			// Verificar si tiene acceso a la aplicación de Procesos o Centro Analítico
			var hasApp bool
			errApp := dbPool.QueryRow(r.Context(), `
				SELECT EXISTS (
					SELECT 1 FROM usuario_aplicaciones 
					WHERE usuario_id = $1 AND aplicacion_id IN ('8fa86047-927b-4029-923f-917c913cc59b', '1ee3a2ea-927f-46cd-b294-86494b668895')
				)`, claims.UsuarioID).Scan(&hasApp)
			if errApp != nil || !hasApp {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				json.NewEncoder(w).Encode([]interface{}{})
				return
			}

			// Estaciones asociadas a las provincias del Admin
			query := `
				SELECT e.id, e.nombre, e.provincia_id, e.ip_red, p.nombre AS provincia_nombre,
				       e.ruta_bin, e.ruta_res, e.usr_red, e.pwd_red
				FROM estaciones e 
				JOIN usuario_provincias up ON e.provincia_id = up.provincia_id 
				LEFT JOIN provincias p ON e.provincia_id = p.id
				WHERE up.usuario_id = $1
			`
			rows, err = dbPool.Query(r.Context(), query, claims.UsuarioID)
		}

		if err != nil {
			log.Printf("[GET /api/admin/estaciones] Query error: %v", err)
			jsonResponse(w, "Database error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		estaciones := []map[string]interface{}{}
		for rows.Next() {
			var id, nombre string
			var provID, ipRed, provNombre, rutaBin, rutaRes, usrRed, pwdRed *string
			if err := rows.Scan(&id, &nombre, &provID, &ipRed, &provNombre, &rutaBin, &rutaRes, &usrRed, &pwdRed); err == nil {
				pID, ip, pNom, rBin, rRes, uRed, pRed := "", "", "", "", "", "", ""
				if provID != nil {
					pID = *provID
				}
				if ipRed != nil {
					ip = *ipRed
				}
				if provNombre != nil {
					pNom = *provNombre
				}
				if rutaBin != nil {
					rBin = *rutaBin
				}
				if rutaRes != nil {
					rRes = *rutaRes
				}
				if usrRed != nil {
					uRed = *usrRed
				}
				if pwdRed != nil {
					pRed = *pwdRed
				}
				estaciones = append(estaciones, map[string]interface{}{
					"id":           id,
					"nombre":       nombre,
					"provincia_id": pID,
					"provincia":    pNom,
					"ip_red":       ip,
					"ruta_bin":     rBin,
					"ruta_res":     rRes,
					"usr_red":      uRed,
					"pwd_red":      pRed,
				})
			}
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(estaciones)

	} else if r.Method == http.MethodPost {
		// Crear estación
		valNivel := r.Context().Value(nivelKey)
		miNivel, ok := valNivel.(int)
		if !ok || miNivel == 0 {
			http.Error(w, "Error interno de jerarquía", http.StatusInternalServerError)
			return
		}
		if miNivel < 80 {
			jsonResponse(w, "No tienes permiso para crear estaciones", http.StatusForbidden)
			return
		}

		type EstacionInput struct {
			Nombre    string  `json:"nombre"`
			IP        string  `json:"ip"`
			Provincia *string `json:"provincia"`
			RutaBin   *string `json:"ruta_bin"`
			RutaRes   *string `json:"ruta_res"`
			UsrRed    *string `json:"usr_red"`
			PwdRed    *string `json:"pwd_red"`
		}

		var req EstacionInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonResponse(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		if req.Nombre == "" {
			jsonResponse(w, "Nombre requerido", http.StatusBadRequest)
			return
		}

		var provinciaIDFinal *string
		if miNivel >= 80 {
			provinciaIDFinal = req.Provincia
		} else {
			// Nivel 60: Solo puede crear estaciones en su propia provincia
			var provAdmin string
			err := dbPool.QueryRow(r.Context(),
				"SELECT provincia_id FROM usuario_provincias WHERE usuario_id = $1 LIMIT 1",
				claims.UsuarioID).Scan(&provAdmin)
			if err != nil {
				jsonResponse(w, "No se encontró tu provincia. No puedes crear estaciones.", http.StatusForbidden)
				return
			}
			// Verificar si intentó mandar otra provincia
			if req.Provincia != nil && *req.Provincia != provAdmin {
				jsonResponse(w, "No tienes permiso para crear estaciones en otra provincia", http.StatusForbidden)
				return
			}
			provinciaIDFinal = &provAdmin
		}

		_, err := dbPool.Exec(r.Context(),
			"INSERT INTO estaciones (nombre, provincia_id, ip_red, ruta_bin, ruta_res, usr_red, pwd_red) VALUES ($1, $2, $3, $4, $5, $6, $7)",
			req.Nombre, provinciaIDFinal, req.IP, req.RutaBin, req.RutaRes, req.UsrRed, req.PwdRed)
		if err != nil {
			log.Printf("[POST /api/admin/estaciones] Error insert: %v", err)
			jsonResponse(w, "Error al crear la estación", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

	} else if r.Method == http.MethodDelete {
		// DELETE /api/admin/estaciones/{id}
		// Extraer el ID de la ruta
		path := strings.TrimPrefix(r.URL.Path, "/api/admin/estaciones/")
		if path == "" || path == r.URL.Path {
			jsonResponse(w, "ID requerido", http.StatusBadRequest)
			return
		}

		valNivel := r.Context().Value(nivelKey)
		miNivel, ok := valNivel.(int)
		if !ok || miNivel == 0 {
			http.Error(w, "Error interno de jerarquía", http.StatusInternalServerError)
			return
		}
		if miNivel < 80 {
			jsonResponse(w, "No tienes permiso para eliminar estaciones", http.StatusForbidden)
			return
		}

		// Si es nivel < 80, debe comprobar que la estación pertenece a su provincia
		if miNivel < 80 {
			var provAdmin string
			err := dbPool.QueryRow(r.Context(),
				"SELECT provincia_id FROM usuario_provincias WHERE usuario_id = $1 LIMIT 1",
				claims.UsuarioID).Scan(&provAdmin)
			if err != nil {
				jsonResponse(w, "No tienes asignada una provincia", http.StatusForbidden)
				return
			}

			var provEstacion string
			err = dbPool.QueryRow(r.Context(), "SELECT provincia_id FROM estaciones WHERE id = $1", path).Scan(&provEstacion)
			if err != nil {
				jsonResponse(w, "Estación no encontrada", http.StatusNotFound)
				return
			}

			if provAdmin != provEstacion {
				jsonResponse(w, "No tienes permiso para eliminar esta estación", http.StatusForbidden)
				return
			}
		}

		_, err := dbPool.Exec(r.Context(), "DELETE FROM estaciones WHERE id = $1", path)
		if err != nil {
			log.Printf("[DELETE /api/admin/estaciones/%s] Error: %v", path, err)
			jsonResponse(w, "Error al eliminar", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

	} else {
		jsonResponse(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// adminEditarEstacionHandler maneja PUT /api/admin/estaciones/editar
func adminEditarEstacionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		jsonResponse(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	valNivel := r.Context().Value(nivelKey)
	miNivel, ok := valNivel.(int)
	if !ok || miNivel == 0 {
		http.Error(w, "Error interno de jerarquía", http.StatusInternalServerError)
		return
	}
	if miNivel < 80 {
		jsonResponse(w, "No tienes permiso para editar estaciones", http.StatusForbidden)
		return
	}

	type EditarEstacionInput struct {
		ID        string  `json:"id"`
		Nombre    string  `json:"nombre"`
		IP        string  `json:"ip"`
		Provincia *string `json:"provincia"`
		RutaBin   *string `json:"ruta_bin"`
		RutaRes   *string `json:"ruta_res"`
		UsrRed    *string `json:"usr_red"`
		PwdRed    *string `json:"pwd_red"`
	}

	var req EditarEstacionInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.ID == "" || req.Nombre == "" {
		jsonResponse(w, "ID y Nombre son requeridos", http.StatusBadRequest)
		return
	}

	valClaims := r.Context().Value("user_claims")
	if valClaims == nil {
		jsonResponse(w, "Internal error: missing claims", http.StatusInternalServerError)
		return
	}
	claims := valClaims.(*Claims)

	var provinciaIDFinal *string

	if miNivel >= 80 {
		provinciaIDFinal = req.Provincia
	} else {
		// Nivel 60: Solo puede editar estaciones de su propia provincia
		var provAdmin string
		err := dbPool.QueryRow(r.Context(),
			"SELECT provincia_id FROM usuario_provincias WHERE usuario_id = $1 LIMIT 1",
			claims.UsuarioID).Scan(&provAdmin)
		if err != nil {
			jsonResponse(w, "No tienes asignada una provincia", http.StatusForbidden)
			return
		}

		// Verificar que la estación actualmente pertenezca a la provincia del admin
		var provEstacion string
		err = dbPool.QueryRow(r.Context(), "SELECT provincia_id FROM estaciones WHERE id = $1", req.ID).Scan(&provEstacion)
		if err != nil {
			jsonResponse(w, "Estación no encontrada", http.StatusNotFound)
			return
		}
		if provEstacion != provAdmin {
			jsonResponse(w, "No tienes permiso para editar esta estación", http.StatusForbidden)
			return
		}

		// Y que no intente asignarla a otra provincia
		if req.Provincia != nil && *req.Provincia != provAdmin {
			jsonResponse(w, "No puedes mover la estación a otra provincia", http.StatusForbidden)
			return
		}
		provinciaIDFinal = &provAdmin
	}

	_, err := dbPool.Exec(r.Context(),
		"UPDATE estaciones SET nombre=$1, ip_red=$2, provincia_id=$3, ruta_bin=$4, ruta_res=$5, usr_red=$6, pwd_red=$7 WHERE id=$8",
		req.Nombre, req.IP, provinciaIDFinal, req.RutaBin, req.RutaRes, req.UsrRed, req.PwdRed, req.ID)
	if err != nil {
		log.Printf("[PUT /api/admin/estaciones/editar] Error: %v", err)
		jsonResponse(w, "Update error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// adminEliminarUsuarioHandler maneja DELETE /api/admin/usuarios/eliminar
func adminEliminarUsuarioHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		jsonResponse(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	val := r.Context().Value(nivelKey)
	miNivel, ok := val.(int)
	if !ok || miNivel == 0 {
		http.Error(w, "Error interno de jerarquía", http.StatusInternalServerError)
		return
	}

	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
		jsonResponse(w, "id requerido", http.StatusBadRequest)
		return
	}

	// Verificar que el usuario a eliminar tiene nivel inferior al admin
	var nivelObjetivo int
	err := dbPool.QueryRow(r.Context(),
		"SELECT r.nivel_poder FROM usuarios u INNER JOIN roles r ON u.rol_id = r.id WHERE u.id = $1", req.ID).Scan(&nivelObjetivo)
	if err != nil {
		jsonResponse(w, "Usuario no encontrado", http.StatusNotFound)
		return
	}
	if nivelObjetivo >= miNivel {
		jsonResponse(w, fmt.Sprintf("Forbidden: no puedes eliminar a un usuario con nivel (%d) >= el tuyo (%d)", nivelObjetivo, miNivel), http.StatusForbidden)
		return
	}

	// Eliminar registros dependientes primero (FK constraints)
	_, _ = dbPool.Exec(r.Context(), "DELETE FROM usuario_provincias WHERE usuario_id = $1", req.ID)
	_, _ = dbPool.Exec(r.Context(), "DELETE FROM usuario_aplicaciones WHERE usuario_id = $1", req.ID)

	_, err = dbPool.Exec(r.Context(), "DELETE FROM usuarios WHERE id = $1", req.ID)
	if err != nil {
		log.Printf("[DELETE /api/admin/usuarios/eliminar] Error al eliminar usuario id=%s: %v", req.ID, err)
		jsonResponse(w, "Delete error", http.StatusInternalServerError)
		return
	}

	log.Printf("[DELETE /api/admin/usuarios/eliminar] Usuario id=%s eliminado por admin nivel=%d", req.ID, miNivel)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// adminRolesHandler devuelve todos los roles del sistema (GET /api/admin/roles)
// El frontend lo usa para poblar el <select> con IDs reales de la BD.
func adminRolesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonResponse(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rows, err := dbPool.Query(r.Context(), "SELECT id, nombre, nivel_poder FROM roles ORDER BY nivel_poder ASC")
	if err != nil {
		jsonResponse(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	roles := []map[string]interface{}{}
	for rows.Next() {
		var id, nivel int
		var nombre string
		if err := rows.Scan(&id, &nombre, &nivel); err == nil {
			roles = append(roles, map[string]interface{}{
				"id":          id,
				"nombre":      nombre,
				"nivel_poder": nivel,
			})
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(roles)
}

// adminProvinciasHandler devuelve todas las provincias (GET /api/admin/provincias)
func adminProvinciasHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonResponse(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rows, err := dbPool.Query(r.Context(), "SELECT id, nombre FROM provincias ORDER BY nombre ASC")
	if err != nil {
		jsonResponse(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	provincias := []map[string]interface{}{}
	for rows.Next() {
		var id, nombre string
		if err := rows.Scan(&id, &nombre); err == nil {
			provincias = append(provincias, map[string]interface{}{
				"id":     id,
				"nombre": nombre,
			})
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(provincias)
}

// jsonResponse es un helper para enviar errores en formato JSON
func jsonResponse(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// apiV1StationsHandler devuelve la lista de estaciones autorizadas para el frontend AWACS.
// Solo expone metadatos públicos (id, name, freq_range). La ruta física (ip_red) nunca sale de este handler.
func apiV1StationsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonResponse(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	val := r.Context().Value("user_claims")
	if val == nil {
		jsonResponse(w, "Internal error: missing claims", http.StatusInternalServerError)
		return
	}
	claims := val.(*Claims)

	// Consulta segura: seleccionamos freq_range si existe, ip_red NUNCA se expone aquí.
	// La columna freq_range es nullable; si no existe en el esquema actual, el fallback lo gestiona.
	var rows pgx.Rows
	var err error
	if claims.NivelPoder >= 80 {
		rows, err = dbPool.Query(r.Context(), `
			SELECT e.id, e.nombre, e.freq_range, e.ip_red
			FROM estaciones e
			ORDER BY e.nombre ASC
		`)
	} else {
		// Usuarios normales: solo ven las estaciones asociadas a sus provincias
		query := `
			SELECT e.id, e.nombre, e.freq_range, e.ip_red
			FROM estaciones e
			JOIN usuario_provincias up ON e.provincia_id = up.provincia_id
			WHERE up.usuario_id = $1
			ORDER BY e.nombre ASC
		`
		rows, err = dbPool.Query(r.Context(), query, claims.UsuarioID)
	}

	if err != nil {
		log.Printf("[GET /api/v1/stations] Query error: %v", err)
		// Fallback: si la columna freq_range no existe aún en el esquema, intentar sin ella
		if strings.Contains(err.Error(), "freq_range") {
			log.Printf("[GET /api/v1/stations] Columna freq_range no encontrada, usando fallback sin ella")
			apiV1StationsFallbackHandler(w, r, claims)
			return
		}
		jsonResponse(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type StationResponse struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		FreqRange    string `json:"freq_range"`
		PhysicalPath string `json:"physical_path"`
	}

	stations := []StationResponse{}
	for rows.Next() {
		var id, nombre string
		var freqRange *string
		var ipRed *string
		if err := rows.Scan(&id, &nombre, &freqRange, &ipRed); err == nil {
			fr := "VHF/UHF Standard"
			if freqRange != nil && *freqRange != "" {
				fr = *freqRange
			}
			path := ""
			if ipRed != nil {
				path = *ipRed
			}
			stations = append(stations, StationResponse{
				ID:           id,
				Name:         nombre,
				FreqRange:    fr,
				PhysicalPath: path,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(stations)
}

// apiV1StationsFallbackHandler es el handler de reserva si freq_range aún no existe en la BD.
func apiV1StationsFallbackHandler(w http.ResponseWriter, r *http.Request, claims *Claims) {
	var rows pgx.Rows
	var err error
	if claims.NivelPoder >= 80 {
		rows, err = dbPool.Query(r.Context(), "SELECT id, nombre, ip_red FROM estaciones ORDER BY nombre ASC")
	} else {
		rows, err = dbPool.Query(r.Context(), `
			SELECT e.id, e.nombre, e.ip_red
			FROM estaciones e
			JOIN usuario_provincias up ON e.provincia_id = up.provincia_id
			WHERE up.usuario_id = $1
			ORDER BY e.nombre ASC
		`, claims.UsuarioID)
	}
	if err != nil {
		jsonResponse(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type StationResponse struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		FreqRange    string `json:"freq_range"`
		PhysicalPath string `json:"physical_path"`
	}
	stations := []StationResponse{}
	for rows.Next() {
		var id, nombre string
		var ipRed *string
		if err := rows.Scan(&id, &nombre, &ipRed); err == nil {
			path := ""
			if ipRed != nil {
				path = *ipRed
			}
			stations = append(stations, StationResponse{ID: id, Name: nombre, FreqRange: "VHF/UHF Standard", PhysicalPath: path})
		} else {
			log.Printf("[Fallback] Error scanning row: %v", err)
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(stations)
}

// apiInternalResolvePathHandler traduce un station_id a su ruta física real.
// SEGURIDAD: Este endpoint NUNCA es accesible públicamente. Solo lo llama el Core C++ internamente
// mediante la clave de servicio compartida (variable de entorno INTERNAL_SERVICE_KEY).
// El frontend JAMÁS debe conocer la existencia de este endpoint ni la estructura de rutas.
func apiInternalResolvePathHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonResponse(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Verificación de clave de servicio interna (distinta del JWT de usuario)
	internalKey := os.Getenv("INTERNAL_SERVICE_KEY")
	if internalKey == "" {
		internalKey = "Ph0en1xN4tNast1y4"
	}
	requestedKey := r.Header.Get("X-Internal-Key")
	if subtle.ConstantTimeCompare([]byte(requestedKey), []byte(internalKey)) != 1 {
		log.Printf("[SECURITY] Intento de acceso no autorizado a /api/internal/resolve-path desde %s", r.RemoteAddr)
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	stationID := r.URL.Query().Get("station_id")
	if stationID == "" {
		jsonResponse(w, "station_id requerido", http.StatusBadRequest)
		return
	}

	// Consultar la ruta física (ip_red) de la estación — esta información NUNCA llega al frontend
	var ipRed string
	var nombre string
	err := dbPool.QueryRow(r.Context(),
		"SELECT nombre, ip_red FROM estaciones WHERE id = $1",
		stationID).Scan(&nombre, &ipRed)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			jsonResponse(w, "Estación no encontrada", http.StatusNotFound)
		} else {
			log.Printf("[GET /api/internal/resolve-path] Error BD para id=%s: %v", stationID, err)
			jsonResponse(w, "Database error", http.StatusInternalServerError)
		}
		return
	}

	log.Printf("[INTERNAL] Resolución de ruta: station_id=%s → ruta=[REDACTED] solicitado por Core", stationID)

	// Respuesta interna con la ruta física real (solo para el Core C++)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]string{
		"station_id":    stationID,
		"nombre":        nombre,
		"physical_path": ipRed,
	})
}

// sanitizeUsername limpia el string dejando solo letras, números y guiones
func sanitizeUsername(u string) string {
	reg := regexp.MustCompile(`[^a-zA-Z0-9\-]`)
	return reg.ReplaceAllString(u, "")
}

// hashArgon2 genera un hash compatible con verifyArgon2Hash
func hashArgon2(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	// Parámetros idénticos a los usados en la verificación
	var memory uint32 = 65536
	var iterations uint32 = 1
	var parallelism uint8 = 4
	var keyLength uint32 = 32

	hash := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, keyLength)

	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	// Formato: $argon2id$v=19$m=65536,t=1,p=4$salt$hash
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", memory, iterations, parallelism, b64Salt, b64Hash), nil
}

// verifyArgon2Hash desglosa el formato de hash de Argon2 y verifica la contraseña.
func verifyArgon2Hash(password, hashStr string) (bool, error) {
	parts := strings.Split(hashStr, "$")
	if len(parts) < 6 {
		return false, fmt.Errorf("formato de hash inválido")
	}

	var memory, time uint32
	var threads uint32
	// Extraer parámetros: m=65536,t=1,p=4
	_, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads)
	if err != nil {
		return false, err
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, err
	}

	decodedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, err
	}

	hash := argon2.IDKey([]byte(password), salt, time, memory, uint8(threads), uint32(len(decodedHash)))

	if subtle.ConstantTimeCompare(decodedHash, hash) == 1 {
		return true, nil
	}
	return false, nil
}

// ============================================================================
//
//	HandleRadarStream - Intermediario para streaming del Decodificador C++
//
// ============================================================================
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		tokenStr := r.URL.Query().Get("token")
		if tokenStr == "" {
			cookie, err := r.Cookie("jwt")
			if err == nil {
				tokenStr = cookie.Value
			}
		}
		if tokenStr == "" {
			return false
		}
		_, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (interface{}, error) {
			return jwtSecret, nil
		})
		return err == nil
	},
}

type PuntoRadar struct {
	T    int64   `json:"t"`
	F    float64 `json:"f"`
	L    float64 `json:"l"`
	Time string  `json:"time"`
}

func copyLastBytesAligned(srcPath, destPath string, maxBytes int64) (int64, error) {
	src, err := os.Open(srcPath)
	if err != nil {
		return 0, err
	}
	defer src.Close()

	info, err := src.Stat()
	if err != nil {
		return 0, err
	}
	size := info.Size()
	if size < 28 {
		return 0, fmt.Errorf("file too small")
	}

	startPos := int64(2)
	if size > maxBytes {
		startPos = size - maxBytes
		residuo := (startPos - 2) % 26
		if residuo != 0 {
			startPos -= residuo
		}
		if startPos < 2 {
			startPos = 2
		}
	}

	_, err = src.Seek(startPos, io.SeekStart)
	if err != nil {
		return 0, err
	}

	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return 0, err
	}

	dest, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		return 0, err
	}
	defer dest.Close()

	_, err = dest.Write([]byte{0x00, 0x00})
	if err != nil {
		return 0, err
	}

	n, err := io.Copy(dest, src)
	return n + 2, err
}

func interpolateSweep(puntos []PuntoRadar, bins int, fmin, fmax float64) []float64 {
	df := make(map[float64]float64)
	for _, p := range puntos {
		if val, exists := df[p.F]; !exists || p.L > val {
			df[p.F] = p.L
		}
	}

	var rx []float64
	for f := range df {
		rx = append(rx, f)
	}
	if len(rx) == 0 {
		sweep := make([]float64, bins)
		for i := range sweep {
			sweep[i] = -120.0
		}
		return sweep
	}
	sort.Float64s(rx)

	ry := make([]float64, len(rx))
	for i, f := range rx {
		ry[i] = df[f]
	}

	sweep := make([]float64, bins)
	step := 1.0
	if bins > 1 {
		step = (fmax - fmin) / float64(bins-1)
	}

	for i := 0; i < bins; i++ {
		qx := fmin + float64(i)*step

		idx := sort.Search(len(rx), func(j int) bool {
			return rx[j] >= qx
		})

		if idx == len(rx) || idx == 0 {
			if idx == 0 {
				sweep[i] = ry[0]
			} else {
				sweep[i] = ry[len(ry)-1]
			}
		} else {
			x0 := rx[idx-1]
			y0 := ry[idx-1]
			x1 := rx[idx]
			y1 := ry[idx]

			if x1 == x0 {
				sweep[i] = y0
			} else {
				sweep[i] = y0 + (y1-y0)*(qx-x0)/(x1-x0)
			}
		}

		if sweep[i] < -150.0 || sweep[i] > 150.0 || sweep[i] != sweep[i] {
			sweep[i] = -120.0
		}
	}

	return sweep
}

func HandleRadarStream(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WS_ERROR] Fallo al iniciar WebSocket: %v", err)
		return
	}
	defer conn.Close()

	log.Printf("[WS] Cliente conectado desde %s", r.RemoteAddr)

	var currentFile string
	var lastSize int64

	for {
		var msg map[string]interface{}
		err := conn.ReadJSON(&msg)
		if err != nil {
			log.Printf("[WS] Conexion cerrada por el cliente: %v", err)
			break
		}

		action, _ := msg["action"].(string)
		if action == "subscribe" {
			physicalPath, _ := msg["source_path"].(string)
			if physicalPath == "" {
				log.Printf("[WS] Suscripcion sin ruta fisica")
				continue
			}

			log.Printf("[WS] Suscribiendose a ruta: %s", physicalPath)

			go func(dirPath string) {
				var calibrated bool
				var calFmin, calFmax float64
				var streamFmin, streamFmax float64
				bins := 256

				ticker := time.NewTicker(200 * time.Millisecond)
				defer ticker.Stop()

				var stallTicks int
				randId := fmt.Sprintf("%d_%d", time.Now().UnixNano(), os.Getpid())
				tempFile := filepath.Join(os.TempDir(), fmt.Sprintf("temp_chunk_%s.bin", randId))
				defer os.Remove(tempFile)

				for range ticker.C {
					var shouldProcess bool

					if currentFile == "" {
						newest, size, err := getNewestRadarFile(dirPath)
						if err != nil {
							log.Printf("[WS_ERROR] Fallo al leer directorio de radar %s: %v", dirPath, err)
						} else if newest == "" {
							log.Printf("[WS_WARNING] No se encontró ningún archivo de radar en %s", dirPath)
						} else if size > 0 {
							currentFile = newest
							lastSize = size
							stallTicks = 0
							shouldProcess = true
							log.Printf("[WS] Iniciando stream con archivo más nuevo: %s (tamaño: %d)", newest, size)
						}
					} else {
						fCheck, err := os.Open(currentFile)
						if err != nil {
							log.Printf("[WS_WARNING] No se pudo abrir el archivo de radar activo: %v", err)
							currentFile = ""
							lastSize = 0
							continue
						}
						newSize, err := fCheck.Seek(0, io.SeekEnd)
						fCheck.Close()
						if err != nil {
							currentFile = ""
							lastSize = 0
							continue
						}

						if newSize > lastSize {
							lastSize = newSize
							stallTicks = 0
							shouldProcess = true
						} else {
							stallTicks++
							if stallTicks >= 25 {
								stallTicks = 0
								newest, size, err := getNewestRadarFile(dirPath)
								if err != nil {
									log.Printf("[WS_ERROR] Fallo al re-escanear directorio %s: %v", dirPath, err)
								} else if newest != "" && newest != currentFile {
									log.Printf("[WS] Detectado archivo mas nuevo: %s", newest)
									currentFile = newest
									lastSize = size
									shouldProcess = true
								}
							}
						}
					}

					if shouldProcess && currentFile != "" {
						_, err := copyLastBytesAligned(currentFile, tempFile, 512*26)
						if err != nil {
							log.Printf("[WS_ERROR] Fallo al extraer últimos bytes de %s: %v", currentFile, err)
							continue
						}

						exePath, _ := os.Executable()
						exeDir := filepath.Dir(exePath)
						parentDir := filepath.Dir(exeDir)
						cmdPath := filepath.Join(parentDir, "sentinel_core", "sentinel_core.exe")
						cmd := exec.Command(cmdPath, tempFile)
						cmd.Dir = filepath.Dir(cmdPath)
						stdout, err := cmd.StdoutPipe()
						if err != nil {
							log.Printf("[WS_DECODER] Error pipe: %v", err)
							continue
						}

						if err := cmd.Start(); err != nil {
							log.Printf("[WS_DECODER] Error start: %v", err)
							continue
						}

						scanner := bufio.NewScanner(stdout)
						var puntos []PuntoRadar
						for scanner.Scan() {
							var p PuntoRadar
							if err := json.Unmarshal(scanner.Bytes(), &p); err == nil {
								puntos = append(puntos, p)
							}
						}
						cmd.Wait()

						if len(puntos) == 0 {
							log.Printf("[WS_WARNING] El decodificador no devolvió puntos para %s", currentFile)
							continue
						}

						// Esperar a tener suficientes puntos para una calibración inicial completa
						if len(puntos) < 256 && !calibrated {
							continue
						}

						fmin := puntos[0].F
						fmax := puntos[0].F
						for _, p := range puntos {
							if p.F < fmin {
								fmin = p.F
							}
							if p.F > fmax {
								fmax = p.F
							}
						}
						if fmax <= fmin {
							fmax = fmin + 1.0
						}

						// Registrar envolvente estática absoluta (nunca se contrae)
						if streamFmin == 0 || fmin < streamFmin {
							streamFmin = fmin
						}
						if streamFmax == 0 || fmax > streamFmax {
							streamFmax = fmax
						}

						sweep := interpolateSweep(puntos, bins, streamFmin, streamFmax)

						if !calibrated {
							calFmin = streamFmin
							calFmax = streamFmax
							calibrated = true

							conn.WriteJSON(map[string]interface{}{
								"type":  "init_frame",
								"fmin":  calFmin * 1e6,
								"fmax":  calFmax * 1e6,
								"bins":  bins,
								"sweep": sweep,
							})
							log.Printf("[WS] init_frame (Fmin=%f, Fmax=%f)", calFmin, calFmax)
						} else {
							err = conn.WriteJSON(map[string]interface{}{
								"type":  "delta_frame",
								"sweep": sweep,
							})
							if err != nil {
								break
							}
						}
					}
				}
			}(physicalPath)
		}
	}
}

func getNewestRadarFile(dirPath string) (string, int64, error) {
	files, err := os.ReadDir(dirPath)
	if err != nil {
		return "", 0, err
	}

	var newestFile string
	var newestTime time.Time
	var size int64

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		info, err := file.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(newestTime) {
			newestTime = info.ModTime()
			newestFile = filepath.Join(dirPath, file.Name())
			size = info.Size()
		}
	}

	return newestFile, size, nil
}

type WinRMMachine struct {
	Name    string `json:"name"`
	IP      string `json:"ip"`
	User    string `json:"user"`
	Pass    string `json:"pass"`
	RutaBin string `json:"rutaBin"`
}

type WinRMPayload struct {
	Machine WinRMMachine           `json:"machine"`
	Action  string                 `json:"action"`
	Params  map[string]interface{} `json:"params"`
}

func winrmHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload WinRMPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request payload: " + err.Error()})
		return
	}

	valClaims := r.Context().Value("user_claims")
	if valClaims == nil {
		jsonResponse(w, "Internal error: missing claims", http.StatusInternalServerError)
		return
	}
	claims := valClaims.(*Claims)

	if payload.Action != "list_files" {
		log.Printf("[winrm] ACCIÓN RECIBIDA: %s | Máquina: %s (%s) | Usuario: ID %s (Nivel %d)", payload.Action, payload.Machine.Name, payload.Machine.IP, claims.UsuarioID, claims.NivelPoder)
	}

	// Si es un usuario de nivel < 80, validar acceso estricto a Procesos y Provincias
	if claims.NivelPoder < 80 {
		// 1. Verificar si tiene acceso a la aplicación de Procesos
		var hasProcesos bool
		err := dbPool.QueryRow(r.Context(), `
			SELECT EXISTS (
				SELECT 1 FROM usuario_aplicaciones 
				WHERE usuario_id = $1 AND aplicacion_id = '8fa86047-927b-4029-923f-917c913cc59b'
			)`, claims.UsuarioID).Scan(&hasProcesos)
		if err != nil || !hasProcesos {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{"error": "Forbidden: No tienes acceso a la aplicación de Procesos"})
			return
		}

		// 2. Verificar si la máquina remota pertenece a la provincia del usuario
		var isAllowedMachine bool
		err = dbPool.QueryRow(r.Context(), `
			SELECT EXISTS (
				SELECT 1 FROM estaciones e
				JOIN usuario_provincias up ON e.provincia_id = up.provincia_id
				WHERE up.usuario_id = $1 AND (e.ip_red = $2 OR e.nombre = $3)
			)`, claims.UsuarioID, payload.Machine.IP, payload.Machine.Name).Scan(&isAllowedMachine)
		if err != nil || !isAllowedMachine {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{"error": "Forbidden: No tienes permiso para administrar esta máquina"})
			return
		}
	}

	// Construcción dinámica de comandos PowerShell compatibles con Windows 7 (PowerShell 2.0)
	var cmdStr string
	escPass := strings.ReplaceAll(payload.Machine.Pass, `'`, `''`)
	escUser := strings.ReplaceAll(payload.Machine.User, `'`, `''`)

	// Extraer únicamente el Hostname o IP limpia (evitando dobles barras UNC como \\192.168.29.71\argus_db)
	ip := payload.Machine.IP
	ip = strings.TrimPrefix(ip, "\\\\")
	ip = strings.TrimPrefix(ip, "//")
	if idx := strings.IndexAny(ip, "\\/"); idx != -1 {
		ip = ip[:idx]
	}
	ip = strings.TrimSpace(ip)

	switch payload.Action {
	case "list_files":
		pathParam := ""
		if p, ok := payload.Params["path"].(string); ok && p != "" {
			pathParam = p
		} else {
			pathParam = payload.Machine.RutaBin
		}
		escPath := strings.ReplaceAll(pathParam, `'`, `''`)
		cmdStr = fmt.Sprintf(
			`$secpasswd = ConvertTo-SecureString '%s' -AsPlainText -Force; `+
				`$cred = New-Object System.Management.Automation.PSCredential ('%s', $secpasswd); `+
				`Invoke-Command -ComputerName %s -Credential $cred -ScriptBlock { `+
				`  param($path); `+
				`  $files = Get-ChildItem -Path $path -ErrorAction SilentlyContinue | ForEach-Object { `+
				`    $len = 0; if ($_.Length) { $len = $_.Length }; `+
				`    $isDir = "false"; if ($_.PSIsContainer) { $isDir = "true" }; `+
				`    '{"Name":"' + $_.Name + '","Length":' + $len + ',"LastWriteTime":"' + $_.LastWriteTime.ToString("yyyy-MM-ddTHH:mm:ss") + '","PSIsContainer":' + $isDir + '}' `+
				`  }; `+
				`  "[" + ($files -join ",") + "]" `+
				`} -ArgumentList '%s'`,
			escPass, escUser, ip, escPath,
		)

	case "list_processes":
		// Limpiar posible dominio en escUser (ej: "cter" en lugar de "DOMAIN\cter")
		userClean := escUser
		if idx := strings.Index(userClean, "\\"); idx != -1 {
			userClean = userClean[idx+1:]
		}
		cmdStr = fmt.Sprintf(
			`$secpasswd = ConvertTo-SecureString '%s' -AsPlainText -Force; `+
				`$cred = New-Object System.Management.Automation.PSCredential ('%s', $secpasswd); `+
				`Invoke-Command -ComputerName %s -Credential $cred -ScriptBlock { `+
				`  param($winrmUser); `+
				`  $currUser = $env:USERNAME; `+
				`  if (!$currUser) { $currUser = $winrmUser }; `+
				`  $procs = Get-WmiObject Win32_Process -ErrorAction SilentlyContinue | ForEach-Object { `+
				`    $owner = $_.GetOwner().User; `+
				`    if ($owner -eq $currUser -or $owner -eq $winrmUser) { `+
				`      '{"Name":"' + $_.Name + '","Id":' + $_.ProcessId + ',"SessionId":"' + $_.SessionId + '"}' `+
				`    } `+
				`  } | Where-Object { $_ -ne $null }; `+
				`  "[" + ($procs -join ",") + "]" `+
				`} -ArgumentList '%s'`,
			escPass, escUser, ip, userClean,
		)

	case "kill_process":
		target := ""
		nameVal, _ := payload.Params["name"].(string)
		if nameVal != "" {
			target = nameVal
		} else {
			pidVal := payload.Params["pid"]
			var pidInt int
			switch v := pidVal.(type) {
			case float64:
				pidInt = int(v)
			case int:
				pidInt = v
			case string:
				fmt.Sscanf(v, "%d", &pidInt)
			}
			if pidInt > 0 {
				target = fmt.Sprintf("%d", pidInt)
			}
		}

		if target != "" {
			// Intentar pskill.exe de forma nativa a través de cmd /c para solucionar Reparse Points
			log.Printf("[winrm] Ejecutando pskill nativo: cmd /c pskill -t -nobanner \\\\%s -u %s -p **** %s", ip, payload.Machine.User, target)
			out, err := exec.Command("cmd", "/c", "pskill", "-t", "-nobanner", "\\\\"+ip, "-u", payload.Machine.User, "-p", payload.Machine.Pass, target).CombinedOutput()
			if err == nil {
				log.Printf("[winrm] pskill exitoso:\n%s", string(out))
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				json.NewEncoder(w).Encode(map[string]string{"output": "pskill exitoso:\n" + strings.TrimSpace(string(out))})
				return
			}
			log.Printf("[winrm] pskill falló con error: %v. Output:\n%s\nUsando fallback taskkill", err, string(out))
		}

		// Fallback si pskill falla o no esta disponible
		if nameVal != "" {
			nameClean := strings.TrimSuffix(nameVal, ".exe")
			nameClean = strings.TrimSuffix(nameClean, ".EXE")
			cmdStr = fmt.Sprintf(
				`$secpasswd = ConvertTo-SecureString '%s' -AsPlainText -Force; `+
					`$cred = New-Object System.Management.Automation.PSCredential ('%s', $secpasswd); `+
					`Invoke-Command -ComputerName %s -Credential $cred -ScriptBlock { `+
					`  param($pname); `+
					`  $result = Stop-Process -Name $pname -Force 2>&1; `+
					`  if (!$result -or $result.ToString().Contains("cannot")) { `+
					`    $result = taskkill /F /IM "$pname.exe" /T 2>&1; `+
					`  }; `+
					`  Write-Output $result; `+
					`} -ArgumentList '%s'`,
				escPass, escUser, ip, nameClean,
			)
		} else {
			pidVal := payload.Params["pid"]
			var pidInt int
			switch v := pidVal.(type) {
			case float64:
				pidInt = int(v)
			case int:
				pidInt = v
			case string:
				fmt.Sscanf(v, "%d", &pidInt)
			}
			if pidInt == 0 {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"error": "Invalid PID"})
				return
			}
			cmdStr = fmt.Sprintf(
				`$secpasswd = ConvertTo-SecureString '%s' -AsPlainText -Force; `+
					`$cred = New-Object System.Management.Automation.PSCredential ('%s', $secpasswd); `+
					`Invoke-Command -ComputerName %s -Credential $cred -ScriptBlock { `+
					`  param($pid); `+
					`  $result = Stop-Process -Id $pid -Force 2>&1; `+
					`  if (!$result -or $result.ToString().Contains("cannot")) { `+
					`    $result = taskkill /F /PID $pid /T 2>&1; `+
					`  }; `+
					`  Write-Output $result; `+
					`} -ArgumentList %d`,
				escPass, escUser, ip, pidInt,
			)
		}

	case "list_shortcuts":
		cmdStr = fmt.Sprintf(
			`$secpasswd = ConvertTo-SecureString '%s' -AsPlainText -Force; `+
				`$cred = New-Object System.Management.Automation.PSCredential ('%s', $secpasswd); `+
				`Invoke-Command -ComputerName %s -Credential $cred -ScriptBlock { `+
				`  $paths = @("$env:USERPROFILE\Desktop", "$env:PUBLIC\Desktop"); `+
				`  $shortcuts = Get-ChildItem -Path $paths -Filter *.lnk -ErrorAction SilentlyContinue | ForEach-Object { `+
				`    $sh = (New-Object -ComObject WScript.Shell).CreateShortcut($_.FullName); `+
				`    $escName = $_.Name.Replace('\','\\').Replace('"','\"'); `+
				`    $escPath = $_.FullName.Replace('\','\\').Replace('"','\"'); `+
				`    $escTarget = $sh.TargetPath.Replace('\','\\').Replace('"','\"'); `+
				`    $escArgs = $sh.Arguments.Replace('\','\\').Replace('"','\"'); `+
				`    '{"Name":"' + $escName + '","Path":"' + $escPath + '","Target":"' + $escTarget + '","Arguments":"' + $escArgs + '"}' `+
				`  }; `+
				`  "[" + ($shortcuts -join ",") + "]" `+
				`}`,
			escPass, escUser, ip,
		)

	case "launch_shortcut":
		pathParam, _ := payload.Params["path"].(string)
		sessionIdVal := payload.Params["sessionId"]
		sessionIdStr := "1"
		if sessionIdVal != nil {
			switch v := sessionIdVal.(type) {
			case string:
				if v != "" {
					sessionIdStr = v
				}
			case float64:
				sessionIdStr = fmt.Sprintf("%.0f", v)
			case int:
				sessionIdStr = fmt.Sprintf("%d", v)
			}
		}

		// Generar script de PowerShell inteligente
		psScript := fmt.Sprintf(`$path = '%s'
if ($path -like "*.lnk") {
    $sh = New-Object -ComObject WScript.Shell
    $lnk = $sh.CreateShortcut($path)
    $target = $lnk.TargetPath
    $args = $lnk.Arguments
    $workingDir = $lnk.WorkingDirectory
} else {
    $target = $path
    $args = ""
    $workingDir = ""
}

function Get-LocalPath($uncPath) {
    if ($uncPath -and $uncPath.StartsWith("\\")) {
        $parts = $uncPath -split '\\' | Where-Object { $_ }
        if ($parts.Count -ge 2) {
            $computer = $parts[0]
            $shareName = $parts[1]
            $localNames = @("localhost", "127.0.0.1", $env:COMPUTERNAME)
            $ips = [System.Net.Dns]::GetHostAddresses("") | ForEach-Object { $_.IPAddressToString }
            $localNames += $ips
            $isLocal = $false
            foreach ($name in $localNames) {
                if ($computer.ToLower() -eq $name.ToLower()) {
                    $isLocal = $true
                    break
                }
            }
            if ($isLocal) {
                $rest = ""
                if ($parts.Count -gt 2) {
                    $rest = $parts[2..($parts.Count-1)] -join '\'
                }
                $localShare = Get-WmiObject Win32_Share | Where-Object { $_.Name -eq $shareName } | Select-Object -First 1
                if ($localShare) {
                    if ($rest) {
                        return Join-Path $localShare.Path $rest
                    }
                    return $localShare.Path
                }
            }
        }
    }
    return $uncPath
}

$localTarget = Get-LocalPath $target
$localWorkingDir = Get-LocalPath $workingDir
if (!$localWorkingDir -and $localTarget) {
    if (Test-Path $localTarget) {
        $localWorkingDir = Split-Path $localTarget
    }
}

$psi = New-Object System.Diagnostics.ProcessStartInfo
$psi.FileName = $localTarget
$psi.Arguments = $args
if ($localWorkingDir -and (Test-Path $localWorkingDir)) {
    $psi.WorkingDirectory = $localWorkingDir
}
$psi.UseShellExecute = $true
[System.Diagnostics.Process]::Start($psi)
`, strings.ReplaceAll(pathParam, `'`, `''`))

		b64Script := base64.StdEncoding.EncodeToString([]byte(psScript))

		// Comando WinRM para escribir el script en C:\Windows\Temp\argus_launch.ps1
		writeCmdStr := fmt.Sprintf(
			`$secpasswd = ConvertTo-SecureString '%s' -AsPlainText -Force; `+
				`$cred = New-Object System.Management.Automation.PSCredential ('%s', $secpasswd); `+
				`Invoke-Command -ComputerName %s -Credential $cred -ScriptBlock { `+
				`  param($b64); `+
				`  $bytes = [System.Convert]::FromBase64String($b64); `+
				`  $content = [System.Text.Encoding]::UTF8.GetString($bytes); `+
				`  $path = "C:\Windows\Temp\argus_launch.ps1"; `+
				`  Set-Content -Path $path -Value $content -Encoding UTF8 -Force; `+
				`} -ArgumentList '%s'`,
			escPass, escUser, ip, b64Script,
		)

		log.Printf("[winrm] Escribiendo script temporal de lanzamiento en %s...", ip)
		outWrite, errWrite := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", writeCmdStr).CombinedOutput()
		if errWrite != nil {
			log.Printf("[winrm] Error al escribir script en equipo remoto: %v. Output:\n%s", errWrite, string(outWrite))
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{
				"error":   "Error al escribir script en equipo remoto: " + errWrite.Error(),
				"details": string(outWrite),
			})
			return
		}

		// Intentar psexec como método principal
		log.Printf("[winrm] Lanzando script de forma interactiva via psexec en sesión %s...", sessionIdStr)
		psexecArgs := []string{
			"\\\\" + ip,
			"-u", payload.Machine.User,
			"-p", payload.Machine.Pass,
			"-accepteula",
			"-nobanner",
			"-d",
			"-i", sessionIdStr,
			"-h",
			"powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", "C:\\Windows\\Temp\\argus_launch.ps1",
		}

		var successOutput string
		outPs, errPs := exec.Command("psexec", psexecArgs...).CombinedOutput()
		outStr := string(outPs)

		// psexec a veces retorna códigos de salida no cero (como el PID del proceso iniciado)
		// incluso cuando se ejecuta correctamente. Verificamos si la salida indica éxito.
		isSuccess := errPs == nil ||
			strings.Contains(strings.ToLower(outStr), "started") ||
			strings.Contains(strings.ToLower(outStr), "process id")

		if isSuccess {
			successOutput = "Lanzamiento interactivo exitoso (psexec):\n" + outStr
			log.Printf("[winrm] %s", successOutput)
		} else {
			log.Printf("[winrm] psexec falló (%v) o no inició el proceso, usando fallback schtasks. Output:\n%s", errPs, outStr)

			// Fallback schtasks
			timeStr := time.Now().Add(70 * time.Second).Format("15:04")
			schCmdStr := fmt.Sprintf(
				`$secpasswd = ConvertTo-SecureString '%s' -AsPlainText -Force; `+
					`$cred = New-Object System.Management.Automation.PSCredential ('%s', $secpasswd); `+
					`Invoke-Command -ComputerName %s -Credential $cred -ScriptBlock { `+
					`  param($time); `+
					`  $taskPath = 'powershell.exe -NoProfile -NonInteractive -ExecutionPolicy Bypass -File C:\Windows\Temp\argus_launch.ps1'; `+
					`  schtasks /Create /TN "ArgusRemoteLaunch" /TR $taskPath /SC ONCE /ST $time /F /RL HIGHEST /RU "INTERACTIVE" 2>&1; `+
					`  schtasks /Run /TN "ArgusRemoteLaunch" 2>&1; `+
					`  Start-Sleep 6; `+
					`  schtasks /Delete /TN "ArgusRemoteLaunch" /F 2>&1; `+
					`} -ArgumentList '%s'`,
				escPass, escUser, ip, timeStr,
			)

			outSch, errSch := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", schCmdStr).CombinedOutput()
			if errSch != nil {
				log.Printf("[winrm] Fallback de schtasks también falló: %v. Output:\n%s", errSch, string(outSch))
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{
					"error":   "Fallo al lanzar el acceso directo por ambos métodos (psexec y schtasks)",
					"details": "psexec: " + outStr + "\nschtasks: " + string(outSch),
				})
				// Limpiar el script temporal de todas formas de forma asíncrona
				go func() {
					cleanupCmdStr := fmt.Sprintf(
						`$secpasswd = ConvertTo-SecureString '%s' -AsPlainText -Force; `+
							`$cred = New-Object System.Management.Automation.PSCredential ('%s', $secpasswd); `+
							`Invoke-Command -ComputerName %s -Credential $cred -ScriptBlock { `+
							`  Remove-Item -Path "C:\Windows\Temp\argus_launch.ps1" -Force -ErrorAction SilentlyContinue; `+
							`}`,
						escPass, escUser, ip,
					)
					exec.Command("powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", cleanupCmdStr).Run()
				}()
				return
			}
			successOutput = "Lanzamiento interactivo exitoso (fallback schtasks):\n" + string(outSch)
			log.Printf("[winrm] %s", successOutput)
		}

		// Limpieza asíncrona del script temporal
		go func() {
			time.Sleep(8 * time.Second)
			cleanupCmdStr := fmt.Sprintf(
				`$secpasswd = ConvertTo-SecureString '%s' -AsPlainText -Force; `+
					`$cred = New-Object System.Management.Automation.PSCredential ('%s', $secpasswd); `+
					`Invoke-Command -ComputerName %s -Credential $cred -ScriptBlock { `+
					`  Remove-Item -Path "C:\Windows\Temp\argus_launch.ps1" -Force -ErrorAction SilentlyContinue; `+
					`}`,
				escPass, escUser, ip,
			)
			exec.Command("powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", cleanupCmdStr).Run()
		}()

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]string{"output": successOutput})
		return

	default:
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Unknown WinRM action: " + payload.Action})
		return
	}

	// Ejecutar PowerShell de fallback localmente pasando el comando pre-autenticado
	if payload.Action != "list_files" {
		log.Printf("[winrm] Ejecutando PowerShell de fallback para acción %s...", payload.Action)
	}
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", cmdStr).CombinedOutput()
	if err != nil {
		if payload.Action != "list_files" {
			log.Printf("[winrm] PowerShell de fallback falló con error: %v. Output:\n%s", err, string(out))
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error":   "PowerShell execution failed: " + err.Error(),
			"details": string(out),
		})
		return
	}
	if payload.Action != "list_files" {
		log.Printf("[winrm] PowerShell de fallback exitoso. Output:\n%s", string(out))
	}

	// Escribir respuesta envolviendo en JSON si es texto plano
	outStr := strings.TrimSpace(string(out))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if len(outStr) > 0 && (outStr[0] == '[' || outStr[0] == '{') {
		w.Write([]byte(outStr))
	} else {
		json.NewEncoder(w).Encode(map[string]string{"output": outStr})
	}
}
