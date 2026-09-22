package main

// Helpers compartidos por todos los tests.
// Cada test parte de una BD SQLite temporal fresca, uploads temporales y
// sesiones reiniciadas. Sin t.Parallel(): los globales (db, sessions,
// uploadsDir) no son seguros para concurrencia entre tests.

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	_ "modernc.org/sqlite"
)

var loadTemplatesOnce sync.Once

type testEnv struct {
	mux   *http.ServeMux
	admin *User
	user  *User
}

// setupTestDB crea el entorno aislado de un test.
func setupTestDB(t *testing.T) *testEnv {
	t.Helper()

	if db != nil {
		_ = db.Close()
		db = nil
	}
	sessionsMu.Lock()
	sessions = make(map[string]*Session)
	sessionsMu.Unlock()

	dir := t.TempDir()
	uploadsDir = filepath.Join(dir, "uploads")
	if err := os.MkdirAll(uploadsDir, 0o755); err != nil {
		t.Fatalf("mkdir uploads: %v", err)
	}

	var err error
	dsn := filepath.Join(dir, "test.db") + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)"
	db, err = sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	if err := migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	loadTemplatesOnce.Do(func() {
		if err := loadTemplates(); err != nil {
			t.Fatalf("loadTemplates: %v", err)
		}
	})

	if err := seedAdmin("admin-test-pass"); err != nil {
		t.Fatalf("seedAdmin: %v", err)
	}
	admin, err := getUserByName("admin")
	if err != nil {
		t.Fatalf("getUserByName admin: %v", err)
	}
	user, err := registerUser("usuario1", "password1")
	if err != nil {
		t.Fatalf("registerUser: %v", err)
	}

	mux := http.NewServeMux()
	routes(mux)
	return &testEnv{mux: mux, admin: admin, user: user}
}

// loginAs autentica y devuelve la cookie de sesión + token CSRF.
func loginAs(t *testing.T, username, password string) (*http.Cookie, string) {
	t.Helper()
	u, err := loginUser(username, password)
	if err != nil {
		t.Fatalf("loginUser(%s): %v", username, err)
	}
	rec := httptest.NewRecorder()
	createSession(rec, u.ID)
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("createSession no emitió cookie")
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookies[0])
	return cookies[0], csrfFor(req)
}

// doReq ejecuta una petición contra el mux con form opcional y cookie opcional.
func doReq(mux http.Handler, method, target string, form url.Values, cookie *http.Cookie) *httptest.ResponseRecorder {
	var body *strings.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	} else {
		body = strings.NewReader("")
	}
	req := httptest.NewRequest(method, target, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// authedForm arma un form con el token CSRF incluido.
func authedForm(csrf string, kv map[string]string) url.Values {
	v := url.Values{}
	v.Set("csrf", csrf)
	for k, val := range kv {
		v.Set(k, val)
	}
	return v
}
