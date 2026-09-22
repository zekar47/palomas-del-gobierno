package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRandHex(t *testing.T) {
	a, b := randHex(16), randHex(16)
	if len(a) != 32 || len(b) != 32 {
		t.Fatalf("longitudes: %d %d, quiero 32", len(a), len(b))
	}
	if a == b {
		t.Fatal("dos tokens seguidos no deben coincidir")
	}
	if randHex(8) == "" {
		t.Fatal("randHex(8) vacío")
	}
}

func TestSesiones(t *testing.T) {
	env := setupTestDB(t)

	// Sin cookie no hay sesión.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if getSession(req) != nil {
		t.Fatal("getSession sin cookie debe ser nil")
	}
	if currentUser(req) != nil {
		t.Fatal("currentUser sin sesión debe ser nil")
	}
	if csrfFor(req) != "" || validCSRF(req) {
		t.Fatal("CSRF sin sesión debe ser vacío/falso")
	}

	// Crear sesión emite cookie HttpOnly.
	rec := httptest.NewRecorder()
	createSession(rec, env.user.ID)
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != sessionCookie || !cookies[0].HttpOnly {
		t.Fatalf("cookie de sesión: %+v", cookies)
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookies[0])
	s := getSession(req)
	if s == nil || s.UserID != env.user.ID || s.CSRF == "" {
		t.Fatalf("sesión: %+v", s)
	}
	u := currentUser(req)
	if u == nil || u.Username != "usuario1" {
		t.Fatalf("currentUser: %+v", u)
	}
	if csrfFor(req) != s.CSRF {
		t.Fatal("csrfFor no devuelve el token de la sesión")
	}

	// CSRF válido / inválido.
	formReq := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("csrf="+s.CSRF))
	formReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	formReq.AddCookie(cookies[0])
	if !validCSRF(formReq) {
		t.Fatal("CSRF correcto rechazado")
	}
	badReq := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("csrf=muerto"))
	badReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	badReq.AddCookie(cookies[0])
	if validCSRF(badReq) {
		t.Fatal("CSRF incorrecto aceptado")
	}

	// Sesión de usuario borrado => currentUser nil.
	if _, err := db.Exec(`DELETE FROM users WHERE id = ?`, env.user.ID); err != nil {
		t.Fatal(err)
	}
	if currentUser(req) != nil {
		t.Fatal("usuario borrado debe dar currentUser nil")
	}

	// Destruir sesión.
	rec2 := httptest.NewRecorder()
	destroySession(rec2, req)
	if getSession(req) != nil {
		t.Fatal("sesión destruida sigue viva")
	}
	// Destruir sin cookie no debe tronar.
	destroySession(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}

func TestRequireLogin(t *testing.T) {
	env := setupTestDB(t)
	next := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) }
	h := requireLogin(next)

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/forum/new", nil))
	if rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/login?next=") {
		t.Fatalf("anónimo: code=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}

	cookie, _ := loginAs(t, "usuario1", "password1")
	req := httptest.NewRequest(http.MethodGet, "/forum/new", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusTeapot {
		t.Fatalf("autenticado: code=%d, quiero 418", rec.Code)
	}
	_ = env
}

func TestRequireAdmin(t *testing.T) {
	setupTestDB(t)
	next := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) }
	h := requireAdmin(next)

	// Anónimo y no-admin van a login.
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/admin/new", nil))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("anónimo: code=%d", rec.Code)
	}
	ucookie, _ := loginAs(t, "usuario1", "password1")
	req := httptest.NewRequest(http.MethodGet, "/admin/new", nil)
	req.AddCookie(ucookie)
	rec = httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("no-admin: code=%d, quiero redirect", rec.Code)
	}
	// Admin pasa.
	acookie, _ := loginAs(t, "admin", "admin-test-pass")
	areq := httptest.NewRequest(http.MethodGet, "/admin/new", nil)
	areq.AddCookie(acookie)
	rec = httptest.NewRecorder()
	h(rec, areq)
	if rec.Code != http.StatusTeapot {
		t.Fatalf("admin: code=%d, quiero 418", rec.Code)
	}
}

func TestRequirePostCSRF(t *testing.T) {
	setupTestDB(t)
	next := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) }
	h := requirePostCSRF(next)

	// Sin sesión => 403.
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodPost, "/x", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("sin sesión: code=%d, quiero 403", rec.Code)
	}
	// Con sesión pero CSRF malo => 403.
	cookie, _ := loginAs(t, "usuario1", "password1")
	bad := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("csrf=muerto"))
	bad.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	bad.AddCookie(cookie)
	rec = httptest.NewRecorder()
	h(rec, bad)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("CSRF malo: code=%d, quiero 403", rec.Code)
	}
	// CSRF bueno pasa.
	cookie2, csrf2 := loginAs(t, "usuario1", "password1")
	good := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("csrf="+csrf2))
	good.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	good.AddCookie(cookie2)
	rec = httptest.NewRecorder()
	h(rec, good)
	if rec.Code != http.StatusTeapot {
		t.Fatalf("CSRF bueno: code=%d, quiero 418", rec.Code)
	}
}

func TestUrlEscape(t *testing.T) {
	if got := urlEscape("/news/1?a=b&c=d#e f+g"); got != "/news/1%3Fa%3Db%26c%3Dd%23e%20f%2Bg" {
		t.Fatalf("urlEscape = %q", got)
	}
	if urlEscape("/simple") != "/simple" {
		t.Fatal("urlEscape alteró ruta simple")
	}
}

func TestRegisterUser(t *testing.T) {
	setupTestDB(t)

	casos := []struct {
		name, user, pass string
	}{
		{"corto", "ab", "password1"},
		{"largo", strings.Repeat("a", 25), "password1"},
		{"clave corta", "nuevo1", "12345"},
		{"duplicado", "usuario1", "password1"},
		{"duplicado NOCASE", "USUARIO1", "password1"},
		{"duplicado admin", "Admin", "password1"},
		{"espacios", "  ", "password1"},
	}
	for _, c := range casos {
		if _, err := registerUser(c.user, c.pass); err == nil {
			t.Fatalf("%s: registro %q debió fallar", c.name, c.user)
		}
	}
	u, err := registerUser("  nuevo2  ", "password2")
	if err != nil {
		t.Fatalf("registro válido: %v", err)
	}
	if u.Username != "nuevo2" { // TrimSpace aplicado
		t.Fatalf("username = %q, quiero nuevo2", u.Username)
	}
	if _, err := loginUser("nuevo2", "password2"); err != nil {
		t.Fatalf("login del nuevo: %v", err)
	}
}

func TestLoginUser(t *testing.T) {
	setupTestDB(t)

	u, err := loginUser("usuario1", "password1")
	if err != nil || u.Username != "usuario1" {
		t.Fatalf("login válido: %+v, %v", u, err)
	}
	if _, err := loginUser("USUARIO1", "password1"); err != nil { // NOCASE
		t.Fatalf("login NOCASE: %v", err)
	}
	if _, err := loginUser("usuario1", "mala-clave"); err != errBadCreds {
		t.Fatalf("clave mala: %v, quiero errBadCreds", err)
	}
	if _, err := loginUser("nadie-existe", "cualquiera"); err != errBadCreds {
		t.Fatalf("usuario inexistente: %v, quiero errBadCreds", err)
	}
}
