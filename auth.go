package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

const sessionCookie = "palomas_sesion"

// sessionMaxAge es la duración de la sesión (7 días, en segundos).
const sessionMaxAge = 7 * 24 * 3600

// cookieSecure activa el atributo Secure en la cookie de sesión.
// Debe ser true cuando la app se sirve por HTTPS (p. ej. tras Caddy/TLS) y
// false en desarrollo local por HTTP plano (con Secure el navegador no la
// enviaría por http://localhost). Se fija con el flag --cookie-secure.
var cookieSecure = false

type Session struct {
	UserID int64
	CSRF   string
}

var (
	sessionsMu sync.RWMutex
	sessions   = make(map[string]*Session)
)

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "0000000000000000"
	}
	return hex.EncodeToString(b)
}

func createSession(w http.ResponseWriter, userID int64) {
	token := randHex(16)
	sessionsMu.Lock()
	sessions[token] = &Session{UserID: userID, CSRF: randHex(16)}
	sessionsMu.Unlock()
	// #nosec G124 -- Secure es condicional a propósito: se activa con
	// --cookie-secure en producción (HTTPS). En desarrollo local por HTTP
	// plano debe ir apagado o el navegador no enviaría la cookie.
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   cookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   sessionMaxAge,
	})
}

func destroySession(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		sessionsMu.Lock()
		delete(sessions, c.Value)
		sessionsMu.Unlock()
	}
	// #nosec G124 -- igual que createSession: Secure lo gobierna
	// --cookie-secure (ver arriba).
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", HttpOnly: true,
		Secure: cookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
}

func getSession(r *http.Request) *Session {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return nil
	}
	sessionsMu.RLock()
	s := sessions[c.Value]
	sessionsMu.RUnlock()
	return s
}

// currentUser devuelve el usuario logueado (o nil).
func currentUser(r *http.Request) *User {
	s := getSession(r)
	if s == nil {
		return nil
	}
	u, err := getUser(s.UserID)
	if err != nil {
		return nil
	}
	return u
}

func csrfFor(r *http.Request) string {
	if s := getSession(r); s != nil {
		return s.CSRF
	}
	return ""
}

func validCSRF(r *http.Request) bool {
	s := getSession(r)
	if s == nil {
		return false
	}
	got := r.FormValue("csrf")
	return got != "" && got == s.CSRF
}

// requireLogin exige sesión activa; si no, redirige a /login.
func requireLogin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if currentUser(r) == nil {
			// #nosec G710 -- el destino Location es la ruta fija /login; solo el
			// valor del query param next deriva del request, ya escapado.
			http.Redirect(w, r, "/login?next="+urlEscape(r.URL.RequestURI()), http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

func urlEscape(s string) string {
	repl := strings.NewReplacer(
		"&", "%26", "?", "%3F", "=", "%3D", "#", "%23", " ", "%20", "+", "%2B")
	return repl.Replace(s)
}

// requireAdmin exige sesión de administrador.
func requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := currentUser(r)
		if !u.isAdmin() {
			// #nosec G710 -- igual que requireLogin: Location fijo a /login.
			http.Redirect(w, r, "/login?next="+urlEscape(r.URL.RequestURI()), http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

// requirePostCSRF valida el token CSRF en POST para las rutas autenticadas.
// Además limita el tamaño del cuerpo antes de leerlo (uploads incluidos).
func requirePostCSRF(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if currentUser(r) == nil {
			http.Error(w, "403 — sesión requerida", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 40<<20)
		if !validCSRF(r) {
			http.Error(w, "403 — token de sesión inválido o caducado", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

var errBadCreds = errors.New("credenciales inválidas")

func registerUser(username, password string) (*User, error) {
	username = strings.TrimSpace(username)
	if len(username) < 3 || len(username) > 24 {
		return nil, errors.New("el nombre de usuario debe tener entre 3 y 24 caracteres")
	}
	if len(password) < 6 {
		return nil, errors.New("la contraseña debe tener al menos 6 caracteres")
	}
	if _, err := getUserByName(username); err == nil {
		return nil, errors.New("ese nombre de usuario ya está ocupado")
	}
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	res, err := db.Exec(`INSERT INTO users (username, password_hash) VALUES (?, ?)`, username, string(h))
	if err != nil {
		return nil, errors.New("ese nombre de usuario ya está ocupado")
	}
	id, _ := res.LastInsertId()
	return getUser(id)
}

func loginUser(username, password string) (*User, error) {
	var id int64
	var hash string
	err := db.QueryRow(`SELECT id, password_hash FROM users WHERE username = ?`, username).
		Scan(&id, &hash)
	if err == sql.ErrNoRows {
		// consume un poco de tiempo para no permitir enumerar usuarios.
		// El resultado se ignora a propósito: solo interesa igualar tiempos.
		// #nosec G104 -- uso intencional como retardo, no como verificación
		_ = bcrypt.CompareHashAndPassword(
			[]byte("$2a$10$7EqJtq98hPqEX7fNZaFWoOhiA0Wkn8kTZ3oP7Z2kX8k9j2j5Wl1uG"),
			[]byte(password))
		return nil, errBadCreds
	}
	if err != nil {
		return nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return nil, errBadCreds
	}
	return getUser(id)
}
