package main

// Pruebas de seguridad: demuestran que los vectores XSS se sirven escapados
// y que los intentos de inyección SQL son inocuos (queries parametrizadas).

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

var xssPayloads = []string{
	`<script>alert(1)</script>`,
	`"><img src=x onerror=alert(1)>`,
	`<svg onload=alert(document.domain)>`,
	`';alert(1);//`,
	`<iframe src="https://evil.test"></iframe>`,
}

func TestXSSComentarioEscapado(t *testing.T) {
	env := setupTestDB(t)
	pid, _ := createPost(env.admin.ID, "XSS", "cuerpo")
	cookie, csrf := loginAs(t, "usuario1", "password1")

	for _, p := range xssPayloads {
		rec := doReq(env.mux, http.MethodPost, fmt.Sprintf("/news/%d/comment", pid),
			authedForm(csrf, map[string]string{"body": p}), cookie)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("comment %q = %d", p, rec.Code)
		}
	}
	rec := doReq(env.mux, http.MethodGet, fmt.Sprintf("/news/%d", pid), nil, nil)
	body := rec.Body.String()
	for _, p := range []string{"<script>alert(1)</script>", "<svg onload", "<iframe src="} {
		if strings.Contains(body, p) {
			t.Fatalf("XSS crudo en respuesta: %q", p)
		}
	}
	if !strings.Contains(body, "&lt;script&gt;") && !strings.Contains(body, "raw HTML omitted") {
		t.Fatal("esperaba el payload neutralizado (escapado u omitido) en la respuesta")
	}
}

func TestXSSTituloHiloUsername(t *testing.T) {
	env := setupTestDB(t)
	cookie, csrf := loginAs(t, "usuario1", "password1")

	// Hilo con payload en título y cuerpo.
	rec := doReq(env.mux, http.MethodPost, "/forum/new",
		authedForm(csrf, map[string]string{"title": xssPayloads[0], "body": xssPayloads[1]}), cookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("crear hilo = %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	rec = doReq(env.mux, http.MethodGet, loc, nil, nil)
	if strings.Contains(rec.Body.String(), "<script>alert(1)</script>") {
		t.Fatal("XSS crudo en hilo")
	}

	// Noticia admin con payload en título.
	acookie, acsrf := loginAs(t, "admin", "admin-test-pass")
	mrec, mreq := multipartPost(t, "/admin/new",
		map[string]string{"title": xssPayloads[0], "body": "b", "csrf": acsrf}, "", "", nil, acookie)
	doMultipart(env.mux, mrec, mreq)
	if mrec.Code != http.StatusSeeOther {
		t.Fatalf("crear noticia = %d", mrec.Code)
	}
	rec = doReq(env.mux, http.MethodGet, mrec.Header().Get("Location"), nil, nil)
	if strings.Contains(rec.Body.String(), "<script>alert(1)</script>") {
		t.Fatal("XSS crudo en título de noticia")
	}

	// Username con payload: el registro lo acepta (charset libre) pero la UI lo escapa.
	xssUser := `<svg onload=alert(1)>` // 22 runas, dentro del límite 3-24
	if _, err := registerUser(xssUser, "password9"); err != nil {
		t.Fatalf("registro username raro: %v", err)
	}
	xcookie, xcsrf := loginAs(t, xssUser, "password9")
	pid, _ := createPost(env.admin.ID, "U", "c")
	rec = doReq(env.mux, http.MethodPost, fmt.Sprintf("/news/%d/comment", pid),
		authedForm(xcsrf, map[string]string{"body": "hola"}), xcookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("comment = %d", rec.Code)
	}
	rec = doReq(env.mux, http.MethodGet, fmt.Sprintf("/news/%d", pid), nil, nil)
	if strings.Contains(rec.Body.String(), "<svg onload=alert(1)>") {
		t.Fatal("XSS crudo vía username")
	}
}

func TestSQLiInocua(t *testing.T) {
	env := setupTestDB(t)
	payloads := []string{
		`' OR 1=1--`,
		`' OR '1'='1`,
		`"; DROP TABLE users;--`,
		`admin'--`,
		`' UNION SELECT password_hash FROM users--`,
	}

	// Login con payloads: siempre errBadCreds, nunca bypass.
	for _, p := range payloads {
		if _, err := loginUser(p, p); err != errBadCreds {
			t.Fatalf("loginUser(%q) = %v, quiero errBadCreds", p, err)
		}
		if _, err := loginUser("usuario1", p); err != errBadCreds {
			t.Fatalf("loginUser clave %q = %v", p, err)
		}
	}

	// Comentario e hilo con payload SQL: se guardan como texto literal.
	cookie, csrf := loginAs(t, "usuario1", "password1")
	pid, _ := createPost(env.admin.ID, "S", "c")
	for _, p := range payloads {
		rec := doReq(env.mux, http.MethodPost, fmt.Sprintf("/news/%d/comment", pid),
			authedForm(csrf, map[string]string{"body": p}), cookie)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("comment %q = %d", p, rec.Code)
		}
		rec = doReq(env.mux, http.MethodPost, "/forum/new",
			authedForm(csrf, map[string]string{"title": p, "body": p}), cookie)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("thread %q = %d", p, rec.Code)
		}
	}

	// Las tablas siguen intactas y con sus filas.
	for _, tbl := range []string{"users", "posts", "comments", "threads"} {
		var n int
		if err := db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM %s`, tbl)).Scan(&n); err != nil {
			t.Fatalf("tabla %s dañada: %v", tbl, err)
		}
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	if n != 2 {
		t.Fatalf("users = %d, quiero 2", n)
	}
	// El login legítimo sigue funcionando.
	if _, err := loginUser("usuario1", "password1"); err != nil {
		t.Fatalf("login legítimo tras SQLi: %v", err)
	}
	// getUserByName con payload no confunde a otro usuario.
	if _, err := getUserByName(payloads[0]); err != sql.ErrNoRows {
		t.Fatalf("getUserByName(payload) = %v", err)
	}
}
