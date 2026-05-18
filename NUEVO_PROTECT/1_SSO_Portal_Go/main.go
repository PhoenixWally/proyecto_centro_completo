package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
	"io"
	"sort"

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
var jwtSecret = []byte("super_secret_key_cambiar_en_produccion")

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
		dbURL = "postgres://postgres:yllaw@localhost:5432/sentinel_sso?sslmode=disable"
	}

	// Iniciar la conexión usando pgx
	dbPool, err = pgxpool.New(context.Background(), dbURL)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v\n", err)
	}
	defer dbPool.Close()

	if secret := os.Getenv("JWT_SECRET"); secret != "" {
		jwtSecret = []byte(secret)
	}

	// Configurar las rutas
	mux := http.NewServeMux()
		mux.HandleFunc("/ws/", HandleRadarStream)
	mux.HandleFunc("/api/login", loginHandler)
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

	resp := map[string]interface{}{
		"message":     "Login exitoso",
		"nivel_poder": nivelPoder,
		"aplicaciones": []map[string]string{
			{"nombre": "Sentinel Radar", "ruta": "/awacs/"},
			{"nombre": "Centro Analítico", "ruta": "/analitico/"},
			{"nombre": "Servidor VNC", "ruta": "/vnc/"},
		},
	}

	w.Header().Set("Content-Type", "application/json")
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

	// 1. Leer qué ruta está intentando visitar el usuario a través de Nginx
	originalURI := r.Header.Get("X-Original-URI")

	// 2. EXCEPCIÓN VIP: Si es el panel de administración, validamos por nivel de poder, no por base de datos.
	if strings.HasPrefix(originalURI, "/sso/admin.html") || strings.HasPrefix(originalURI, "/api/admin/") {
		if nivelPoder >= 60 {
			w.WriteHeader(http.StatusOK) // Adelante, eres Admin
			return
		}
		w.WriteHeader(http.StatusForbidden) // Bloqueado, eres usuario normal
		return
	}

	ctx := context.Background()
	// Verificamos en usuario_aplicaciones si hay permiso para esa ruta (Ej. la ruta original /sentinel/ subcoincide con la ruta_base de la tabla aplicaciones)
	query := `
		SELECT 1
		FROM usuario_aplicaciones ua
		JOIN aplicaciones a ON ua.aplicacion_id = a.id
		WHERE ua.usuario_id = $1 AND $2 LIKE (a.ruta_base || '%')
		LIMIT 1
	`
	var exists int
	err = dbPool.QueryRow(ctx, query, claims.UsuarioID, originalURI).Scan(&exists)
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

		if nivelPoder < 60 {
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
		if claims.NivelPoder >= 80 {
			rows, err = dbPool.Query(r.Context(), `
				SELECT u.id, u.username, r.nivel_poder, r.id,
				       up.provincia_id,
				       p.nombre AS provincia_nombre
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
				       p.nombre AS provincia_nombre
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
			var provinciaID, provinciaNombre *string
			if err := rows.Scan(&id, &username, &nivel, &rolID, &provinciaID, &provinciaNombre); err == nil {
				provID := ""
				provNombre := ""
				if provinciaID != nil {
					provID = *provinciaID
				}
				if provinciaNombre != nil {
					provNombre = *provinciaNombre
				}
				users = append(users, map[string]interface{}{
					"id":           id,
					"usuario":      username,
					"nivel_poder":  nivel,
					"rol_id":       rolID,
					"provincia_id": provID,
					"provincia":    provNombre,
				})
			}
		}
		w.Header().Set("Content-Type", "application/json")
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

		type UsuarioInput struct {
			Username    string  `json:"username"`
			Password    string  `json:"password,omitempty"`
			RolID       int     `json:"rol_id"`
			ProvinciaID *string `json:"provincia_id"` // Puntero vital para aceptar null
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

		// Obtener nivel del rol destino y verificar jerarquía estrictamente
		var nivelRolDestino int
		err := dbPool.QueryRow(r.Context(), "SELECT nivel_poder FROM roles WHERE id = $1", req.RolID).Scan(&nivelRolDestino)
		if err != nil {
			jsonResponse(w, "rol_id inválido o no encontrado", http.StatusBadRequest)
			return
		}
		if nivelRolDestino >= miNivel {
			jsonResponse(w, fmt.Sprintf("Forbidden: el nivel del rol destino (%d) debe ser estrictamente menor que el tuyo (%d)", nivelRolDestino, miNivel), http.StatusForbidden)
			return
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
			"INSERT INTO usuarios (username, password_hash, rol_id) VALUES ($1, $2, $3) RETURNING id",
			req.Username, passHash, req.RolID).Scan(&newUserID)
		if err != nil {
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

		provIDLog := "NULL"
		if provinciaIDFinal != nil {
			provIDLog = *provinciaIDFinal
		}
		log.Printf("[POST /api/admin/usuarios] Usuario '%s' (id=%s) creado con rol_id=%d (nivel=%d) provincia='%s' por admin nivel=%d",
			req.Username, newUserID, req.RolID, nivelRolDestino, provIDLog, miNivel)
		w.Header().Set("Content-Type", "application/json")
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
				"UPDATE usuarios SET username=$1, password_hash=$2, rol_id=$3 WHERE id=$4",
				req2.Username, passHash2, req2.RolID, req2.ID)
		} else {
			_, putErr = dbPool.Exec(r.Context(),
				"UPDATE usuarios SET username=$1, rol_id=$2 WHERE id=$3",
				req2.Username, req2.RolID, req2.ID)
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

		provIDLog2 := "NULL"
		if provinciaIDEdit != nil {
			provIDLog2 = *provinciaIDEdit
		}
		log.Printf("[PUT /api/admin/usuarios] Usuario id=%s actualizado a rol_id=%d provincia='%s' por admin nivel=%d",
			req2.ID, req2.RolID, provIDLog2, miNivel2)
		w.Header().Set("Content-Type", "application/json")
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
		ID          string  `json:"id"`
		Username    string  `json:"username"`
		Password    string  `json:"password,omitempty"`
		RolID       int     `json:"rol_id"`
		ProvinciaID *string `json:"provincia_id"`
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
	log.Printf("DEBUG: ID=%s, Rol=%d, Provincia=%v", req.ID, req.RolID, req.ProvinciaID)

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

	// Seguridad: miNivel debe ser mayor que nivel actual del objetivo Y mayor que el nuevo nivel asignado
	if nivelActual >= miNivel {
		jsonResponse(w, fmt.Sprintf("Forbidden: el usuario a editar tiene nivel (%d) >= el tuyo (%d)", nivelActual, miNivel), http.StatusForbidden)
		return
	}
	if nivelRolDestino >= miNivel {
		jsonResponse(w, fmt.Sprintf("Forbidden: el nuevo rol tiene nivel (%d) >= el tuyo (%d)", nivelRolDestino, miNivel), http.StatusForbidden)
		return
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
			"UPDATE usuarios SET username=$1, password_hash=$2, rol_id=$3 WHERE id=$4",
			req.Username, passHash, req.RolID, req.ID)
		if err != nil {
			jsonResponse(w, "Update error", http.StatusInternalServerError)
			return
		}
	} else {
		_, err = dbPool.Exec(r.Context(),
			"UPDATE usuarios SET username=$1, rol_id=$2 WHERE id=$3",
			req.Username, req.RolID, req.ID)
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

	log.Printf("[PUT /api/admin/usuarios/editar] Usuario id=%s actualizado a rol_id=%d (nivel=%d) por admin nivel=%d", req.ID, req.RolID, nivelRolDestino, miNivel)
	w.Header().Set("Content-Type", "application/json")
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
				SELECT e.id, e.nombre, e.provincia_id, e.ip_red, p.nombre AS provincia_nombre 
				FROM estaciones e 
				LEFT JOIN provincias p ON e.provincia_id = p.id
			`)
		} else {
			// Nivel 60: Estaciones asociadas a las provincias del Admin
			query := `
				SELECT e.id, e.nombre, e.provincia_id, e.ip_red, p.nombre AS provincia_nombre 
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
			var provID, ipRed, provNombre *string
			if err := rows.Scan(&id, &nombre, &provID, &ipRed, &provNombre); err == nil {
				pID, ip, pNom := "", "", ""
				if provID != nil {
					pID = *provID
				}
				if ipRed != nil {
					ip = *ipRed
				}
				if provNombre != nil {
					pNom = *provNombre
				}
				estaciones = append(estaciones, map[string]interface{}{
					"id":           id,
					"nombre":       nombre,
					"provincia_id": pID,
					"provincia":    pNom,
					"ip_red":       ip,
				})
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(estaciones)

	} else if r.Method == http.MethodPost {
		// Crear estación
		valNivel := r.Context().Value(nivelKey)
		miNivel, ok := valNivel.(int)
		if !ok || miNivel == 0 {
			http.Error(w, "Error interno de jerarquía", http.StatusInternalServerError)
			return
		}

		type EstacionInput struct {
			Nombre    string  `json:"nombre"`
			IP        string  `json:"ip"`
			Provincia *string `json:"provincia"`
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
			"INSERT INTO estaciones (nombre, provincia_id, ip_red) VALUES ($1, $2, $3)",
			req.Nombre, provinciaIDFinal, req.IP)
		if err != nil {
			log.Printf("[POST /api/admin/estaciones] Error insert: %v", err)
			jsonResponse(w, "Error al crear la estación", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
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

		w.Header().Set("Content-Type", "application/json")
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

	type EditarEstacionInput struct {
		ID        string  `json:"id"`
		Nombre    string  `json:"nombre"`
		IP        string  `json:"ip"`
		Provincia *string `json:"provincia"`
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
		"UPDATE estaciones SET nombre=$1, ip_red=$2, provincia_id=$3 WHERE id=$4",
		req.Nombre, req.IP, provinciaIDFinal, req.ID)
	if err != nil {
		log.Printf("[PUT /api/admin/estaciones/editar] Error: %v", err)
		jsonResponse(w, "Update error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
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
	w.Header().Set("Content-Type", "application/json")
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
	w.Header().Set("Content-Type", "application/json")
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
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(provincias)
}

// jsonResponse es un helper para enviar errores en formato JSON
func jsonResponse(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
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

	w.Header().Set("Content-Type", "application/json")
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
	w.Header().Set("Content-Type", "application/json")
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
		internalKey = "sentinel_internal_key_cambiar_en_produccion"
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
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"station_id":   stationID,
		"nombre":       nombre,
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
//  HandleRadarStream - Intermediario para streaming del Decodificador C++
// ============================================================================
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
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
						if err == nil && newest != "" && size > 0 {
							currentFile = newest
							lastSize = size
							stallTicks = 0
							shouldProcess = true
						}
					} else {
						info, err := os.Stat(currentFile)
						if err != nil {
							currentFile = ""
							lastSize = 0
							continue
						}

						newSize := info.Size()
						if newSize > lastSize {
							lastSize = newSize
							stallTicks = 0
							shouldProcess = true
						} else {
							stallTicks++
							if stallTicks >= 25 {
								stallTicks = 0
								newest, size, err := getNewestRadarFile(dirPath)
								if err == nil && newest != "" && newest != currentFile {
									log.Printf("[WS] Detectado archivo mas nuevo: %s", newest)
									currentFile = newest
									lastSize = size
									shouldProcess = true
								}
							}
						}
					}

					if shouldProcess && currentFile != "" {
						_, err := copyLastBytesAligned(currentFile, tempFile, 4096*26)
						if err != nil {
							continue
						}

						cmdPath := `C:\nginx\html\sentinel_core.exe`
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
							continue
						}

						fmin := puntos[0].F
						fmax := puntos[0].F
						for _, p := range puntos {
							if p.F < fmin { fmin = p.F }
							if p.F > fmax { fmax = p.F }
						}
						if fmax <= fmin {
							fmax = fmin + 1.0
						}

						sweep := interpolateSweep(puntos, bins, fmin, fmax)

						if !calibrated {
							calFmin = fmin
							calFmax = fmax
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
