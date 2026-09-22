package main

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseID(t *testing.T) {
	casos := map[string]int64{"5": 5, "1": 1, "0": 0, "-3": 0, "abc": 0, "": 0, "12x": 0, "9999999999999999999999": 0}
	for in, want := range casos {
		if got := parseID(in); got != want {
			t.Fatalf("parseID(%q) = %d, quiero %d", in, got, want)
		}
	}
}

func TestSafeNext(t *testing.T) {
	if safeNext("") != "/" || safeNext("/news/1") != "/news/1" {
		t.Fatal("safeNext rutas válidas")
	}
	for _, bad := range []string{"http://evil.test", "https://evil.test/x", "//evil.test", "//evil.test/x", "///", "javascript:alert(1)", "evil"} {
		if safeNext(bad) != "/" {
			t.Fatalf("safeNext(%q) debió devolver /", bad)
		}
	}
}

func TestHomeHandler(t *testing.T) {
	env := setupTestDB(t)
	rec := doReq(env.mux, http.MethodGet, "/", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "PALOMAS") {
		t.Fatal("portada sin marca PALOMAS")
	}
}

func TestHomeHandlerDBCaida(t *testing.T) {
	env := setupTestDB(t)
	_ = db.Close() // simula BD inaccesible: la portada debe seguir respondiendo 200
	rec := doReq(env.mux, http.MethodGet, "/", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, quiero 200 aun sin BD", rec.Code)
	}
	for _, ruta := range []string{"/news", "/forum", "/news/feed.xml", "/forum/feed.xml"} {
		rec := doReq(env.mux, http.MethodGet, ruta, nil, nil)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("GET %s = %d, quiero 500 sin BD", ruta, rec.Code)
		}
	}
}

func TestStaticHandler(t *testing.T) {
	env := setupTestDB(t)
	rec := doReq(env.mux, http.MethodGet, "/static/style.css", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("style.css = %d", rec.Code)
	}
	rec = doReq(env.mux, http.MethodGet, "/static/no-existe.css", nil, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("inexistente = %d, quiero 404", rec.Code)
	}
}

func TestUploadsHandler(t *testing.T) {
	env := setupTestDB(t)
	contenido := []byte("datos-falsos-de-imagen")
	if err := os.WriteFile(filepath.Join(uploadsDir, "foto.png"), contenido, 0o644); err != nil {
		t.Fatal(err)
	}
	rec := doReq(env.mux, http.MethodGet, "/uploads/foto.png", nil, nil)
	if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), contenido) {
		t.Fatalf("serve foto: code=%d", rec.Code)
	}
	// Traversal: el mux redirige (307) ante ".."; al seguirlo se cae al 404
	// sin servir nada fuera de uploads.
	rec = doReq(env.mux, http.MethodGet, "/uploads/../main.go", nil, nil)
	if rec.Code == http.StatusTemporaryRedirect || rec.Code == http.StatusMovedPermanently {
		rec = doReq(env.mux, http.MethodGet, rec.Header().Get("Location"), nil, nil)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("traversal = %d, quiero 404", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "package main") {
		t.Fatal("fuga de código fuente vía traversal")
	}
	rec = doReq(env.mux, http.MethodGet, "/uploads/noexiste.png", nil, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("inexistente = %d", rec.Code)
	}
}

func TestLoginFlow(t *testing.T) {
	env := setupTestDB(t)

	rec := doReq(env.mux, http.MethodGet, "/login", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET login = %d", rec.Code)
	}
	// Credenciales malas: re-render con mensaje genérico (sin enumerar).
	rec = doReq(env.mux, http.MethodPost, "/login", url.Values{"username": {"usuario1"}, "password": {"mala"}}, nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "incorrectos") {
		t.Fatalf("login malo: code=%d", rec.Code)
	}
	// Login bueno con next interno.
	rec = doReq(env.mux, http.MethodPost, "/login",
		url.Values{"username": {"usuario1"}, "password": {"password1"}, "next": {"/forum"}}, nil)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/forum" {
		t.Fatalf("login: code=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("login no emitió cookie")
	}
	// next malicioso se neutraliza a /.
	rec = doReq(env.mux, http.MethodPost, "/login",
		url.Values{"username": {"usuario1"}, "password": {"password1"}, "next": {"//evil.test"}}, nil)
	if rec.Header().Get("Location") != "/" {
		t.Fatalf("next malicioso: loc=%q", rec.Header().Get("Location"))
	}
}

func TestRegisterFlow(t *testing.T) {
	env := setupTestDB(t)

	rec := doReq(env.mux, http.MethodGet, "/register", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET register = %d", rec.Code)
	}
	rec = doReq(env.mux, http.MethodPost, "/register",
		url.Values{"username": {"usuario1"}, "password": {"password1"}}, nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "ocupado") {
		t.Fatalf("duplicado: code=%d", rec.Code)
	}
	rec = doReq(env.mux, http.MethodPost, "/register",
		url.Values{"username": {"nuevohttp"}, "password": {"password2"}}, nil)
	if rec.Code != http.StatusSeeOther || len(rec.Result().Cookies()) != 1 {
		t.Fatalf("registro: code=%d", rec.Code)
	}
}

func TestLogout(t *testing.T) {
	env := setupTestDB(t)
	cookie, csrf := loginAs(t, "usuario1", "password1")

	// Anónimo no puede hacer logout (requireLogin redirige).
	rec := doReq(env.mux, http.MethodPost, "/logout", url.Values{"csrf": {"x"}}, nil)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("logout anónimo = %d", rec.Code)
	}
	rec = doReq(env.mux, http.MethodPost, "/logout", authedForm(csrf, nil), cookie)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Fatalf("logout = %d loc=%q", rec.Code, rec.Header().Get("Location"))
	}
	// La sesión murió: ruta protegida redirige a login.
	rec = doReq(env.mux, http.MethodGet, "/forum/new", nil, cookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("tras logout: code=%d, quiero redirect", rec.Code)
	}
}

func TestNewsListShow(t *testing.T) {
	env := setupTestDB(t)
	rec := doReq(env.mux, http.MethodGet, "/news", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("lista vacía = %d", rec.Code)
	}
	pid, _ := createPost(env.admin.ID, "Noticia HTTP", "cuerpo")
	rec = doReq(env.mux, http.MethodGet, "/news", nil, nil)
	if !strings.Contains(rec.Body.String(), "Noticia HTTP") {
		t.Fatal("lista no muestra el post")
	}
	rec = doReq(env.mux, http.MethodGet, fmt.Sprintf("/news/%d", pid), nil, nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Noticia HTTP") {
		t.Fatalf("detalle = %d", rec.Code)
	}
	for _, ruta := range []string{"/news/abc", "/news/0", "/news/-5", "/news/999999"} {
		if rec := doReq(env.mux, http.MethodGet, ruta, nil, nil); rec.Code != http.StatusNotFound {
			t.Fatalf("GET %s = %d, quiero 404", ruta, rec.Code)
		}
	}
}

func TestNewsReaction(t *testing.T) {
	env := setupTestDB(t)
	pid, _ := createPost(env.admin.ID, "R", "c")
	ruta := fmt.Sprintf("/news/%d/reaction", pid)

	// Anónimo => login.
	if rec := doReq(env.mux, http.MethodPost, ruta, url.Values{"reaction": {"like"}}, nil); rec.Code != http.StatusSeeOther {
		t.Fatalf("anónimo = %d", rec.Code)
	}
	cookie, csrf := loginAs(t, "usuario1", "password1")
	// CSRF malo => 403.
	if rec := doReq(env.mux, http.MethodPost, ruta, url.Values{"csrf": {"x"}, "reaction": {"like"}}, cookie); rec.Code != http.StatusForbidden {
		t.Fatalf("CSRF malo = %d", rec.Code)
	}
	// like => 303 y contador.
	rec := doReq(env.mux, http.MethodPost, ruta, authedForm(csrf, map[string]string{"reaction": "like"}), cookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("like = %d", rec.Code)
	}
	p, _ := getPost(pid)
	if p.Likes != 1 {
		t.Fatalf("likes = %d", p.Likes)
	}
	// Valor inválido => 400. ID inválido => 400.
	if rec := doReq(env.mux, http.MethodPost, ruta, authedForm(csrf, map[string]string{"reaction": "meh"}), cookie); rec.Code != http.StatusBadRequest {
		t.Fatalf("kind malo = %d", rec.Code)
	}
	if rec := doReq(env.mux, http.MethodPost, "/news/0/reaction", authedForm(csrf, map[string]string{"reaction": "like"}), cookie); rec.Code != http.StatusBadRequest {
		t.Fatalf("id malo = %d", rec.Code)
	}
}

func TestNewsComment(t *testing.T) {
	env := setupTestDB(t)
	pid, _ := createPost(env.admin.ID, "C", "c")
	ruta := fmt.Sprintf("/news/%d/comment", pid)
	cookie, csrf := loginAs(t, "usuario1", "password1")

	rec := doReq(env.mux, http.MethodPost, ruta, authedForm(csrf, map[string]string{"body": "buen post"}), cookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("comment = %d", rec.Code)
	}
	if rec := doReq(env.mux, http.MethodGet, fmt.Sprintf("/news/%d", pid), nil, nil); !strings.Contains(rec.Body.String(), "buen post") {
		t.Fatal("comentario no visible")
	}
	for name, body := range map[string]string{"vacío": "   ", "largo": strings.Repeat("x", 4001)} {
		if rec := doReq(env.mux, http.MethodPost, ruta, authedForm(csrf, map[string]string{"body": body}), cookie); rec.Code != http.StatusBadRequest {
			t.Fatalf("comment %s = %d, quiero 400", name, rec.Code)
		}
	}
	if rec := doReq(env.mux, http.MethodPost, "/news/0/comment", authedForm(csrf, map[string]string{"body": "x"}), cookie); rec.Code != http.StatusBadRequest {
		t.Fatalf("id malo = %d", rec.Code)
	}
}

// multipartPost arma una petición multipart (para uploads de admin).
func multipartPost(t *testing.T, target string, fields map[string]string, fileField, fileName string, fileBytes []byte, cookie *http.Cookie) (*httptest.ResponseRecorder, *http.Request) {
	t.Helper()
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if fileField != "" {
		fw, err := w.CreateFormFile(fileField, fileName)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write(fileBytes); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, target, &b)
	req.Header.Set("Content-Type", w.FormDataContentType())
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	return rec, req
}

func doMultipart(mux http.Handler, rec *httptest.ResponseRecorder, req *http.Request) *httptest.ResponseRecorder {
	mux.ServeHTTP(rec, req)
	return rec
}

func postIDFromLoc(t *testing.T, loc string) int64 {
	t.Helper()
	var id int64
	if _, err := fmt.Sscanf(loc, "/news/%d", &id); err != nil || id == 0 {
		t.Fatalf("Location sin id: %q", loc)
	}
	return id
}

func TestAdminNewGet(t *testing.T) {
	env := setupTestDB(t)
	if rec := doReq(env.mux, http.MethodGet, "/admin/new", nil, nil); rec.Code != http.StatusSeeOther {
		t.Fatalf("anónimo = %d", rec.Code)
	}
	ucookie, _ := loginAs(t, "usuario1", "password1")
	if rec := doReq(env.mux, http.MethodGet, "/admin/new", nil, ucookie); rec.Code != http.StatusSeeOther {
		t.Fatalf("no-admin = %d", rec.Code)
	}
	acookie, _ := loginAs(t, "admin", "admin-test-pass")
	if rec := doReq(env.mux, http.MethodGet, "/admin/new", nil, acookie); rec.Code != http.StatusOK {
		t.Fatalf("admin = %d", rec.Code)
	}
}

func TestAdminNewPost(t *testing.T) {
	env := setupTestDB(t)
	acookie, acsrf := loginAs(t, "admin", "admin-test-pass")

	// Crear solo texto.
	rec, req := multipartPost(t, "/admin/new",
		map[string]string{"title": "N1", "body": "cuerpo", "csrf": acsrf}, "", "", nil, acookie)
	doMultipart(env.mux, rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("crear = %d", rec.Code)
	}
	_ = postIDFromLoc(t, rec.Header().Get("Location"))

	// Sin título => re-render con error.
	rec, req = multipartPost(t, "/admin/new",
		map[string]string{"title": "  ", "body": "x", "csrf": acsrf}, "", "", nil, acookie)
	doMultipart(env.mux, rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "título") {
		t.Fatalf("sin título = %d", rec.Code)
	}

	// Imagen con extensión prohibida => rollback (el post no queda).
	var n0 int
	_ = db.QueryRow(`SELECT COUNT(*) FROM posts`).Scan(&n0)
	rec, req = multipartPost(t, "/admin/new",
		map[string]string{"title": "Mala", "body": "x", "csrf": acsrf}, "image", "evil.exe", []byte("MZ"), acookie)
	doMultipart(env.mux, rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "imagen:") {
		t.Fatalf("ext mala = %d", rec.Code)
	}
	var n1 int
	_ = db.QueryRow(`SELECT COUNT(*) FROM posts`).Scan(&n1)
	if n0 != n1 {
		t.Fatalf("rollback falló: posts %d -> %d", n0, n1)
	}
}

func TestAdminNewPostConMedia(t *testing.T) {
	env := setupTestDB(t)
	acookie, acsrf := loginAs(t, "admin", "admin-test-pass")

	// Imagen válida.
	rec, req := multipartPost(t, "/admin/new",
		map[string]string{"title": "ConImg", "body": "cuerpo", "csrf": acsrf}, "image", "foto.png", []byte("fakepng"), acookie)
	doMultipart(env.mux, rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("crear con img = %d: %s", rec.Code, rec.Body.String())
	}
	pid := postIDFromLoc(t, rec.Header().Get("Location"))
	p, _ := getPost(pid)
	if p.ImagePath == "" {
		t.Fatal("ImagePath vacío")
	}
	if _, err := os.Stat(filepath.Join(uploadsDir, p.ImagePath)); err != nil {
		t.Fatalf("archivo no está en disco: %v", err)
	}
	// La página del post referencia el upload.
	if rec := doReq(env.mux, http.MethodGet, fmt.Sprintf("/news/%d", pid), nil, nil); !strings.Contains(rec.Body.String(), "/uploads/") {
		t.Fatal("post sin referencia a /uploads/")
	}

	// Video con extensión mala tras imagen buena => rollback (post e imagen fuera).
	var n0 int
	_ = db.QueryRow(`SELECT COUNT(*) FROM posts`).Scan(&n0)
	entries0, _ := os.ReadDir(uploadsDir)
	rec, req = multipartPost(t, "/admin/new",
		map[string]string{"title": "VidMalo", "body": "x", "csrf": acsrf}, "video", "v.avi", []byte("avi"), acookie)
	doMultipart(env.mux, rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "video:") {
		t.Fatalf("video malo = %d", rec.Code)
	}
	var n1 int
	_ = db.QueryRow(`SELECT COUNT(*) FROM posts`).Scan(&n1)
	entries1, _ := os.ReadDir(uploadsDir)
	if n0 != n1 || len(entries0) != len(entries1) {
		t.Fatalf("rollback video: posts %d->%d archivos %d->%d", n0, n1, len(entries0), len(entries1))
	}
}

func TestAdminEditDelete(t *testing.T) {
	env := setupTestDB(t)
	acookie, acsrf := loginAs(t, "admin", "admin-test-pass")

	// Inexistente => 404.
	if rec := doReq(env.mux, http.MethodGet, "/admin/edit/999999", nil, acookie); rec.Code != http.StatusNotFound {
		t.Fatalf("edit 404 = %d", rec.Code)
	}
	rec, req := multipartPost(t, "/admin/new",
		map[string]string{"title": "E1", "body": "b", "csrf": acsrf}, "image", "i.png", []byte("png"), acookie)
	doMultipart(env.mux, rec, req)
	pid := postIDFromLoc(t, rec.Header().Get("Location"))
	p, _ := getPost(pid)
	imgVieja := p.ImagePath

	if rec := doReq(env.mux, http.MethodGet, fmt.Sprintf("/admin/edit/%d", pid), nil, acookie); rec.Code != http.StatusOK {
		t.Fatalf("edit get = %d", rec.Code)
	}
	// Editar título + quitar imagen.
	rec, req = multipartPost(t, "/admin/edit/"+fmt.Sprint(pid),
		map[string]string{"title": "E1-edit", "body": "b2", "remove_image": "1", "csrf": acsrf}, "", "", nil, acookie)
	doMultipart(env.mux, rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("edit post = %d", rec.Code)
	}
	p, _ = getPost(pid)
	if p.Title != "E1-edit" || p.ImagePath != "" {
		t.Fatalf("edit no aplicado: %+v", p)
	}
	if _, err := os.Stat(filepath.Join(uploadsDir, imgVieja)); !os.IsNotExist(err) {
		t.Fatal("imagen vieja sigue en disco")
	}
	// Editar inexistente por POST => 404.
	rec, req = multipartPost(t, "/admin/edit/999999",
		map[string]string{"title": "x", "body": "y", "csrf": acsrf}, "", "", nil, acookie)
	doMultipart(env.mux, rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("edit post 404 = %d", rec.Code)
	}
	// Borrar.
	rec = doReq(env.mux, http.MethodPost, fmt.Sprintf("/admin/delete/%d", pid),
		authedForm(acsrf, nil), acookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("delete = %d", rec.Code)
	}
	if _, err := getPost(pid); err == nil {
		t.Fatal("post borrado sigue")
	}
	// Borrar inexistente => 404.
	if rec := doReq(env.mux, http.MethodPost, "/admin/delete/999999", authedForm(acsrf, nil), acookie); rec.Code != http.StatusNotFound {
		t.Fatalf("delete 404 = %d", rec.Code)
	}
}

func TestSaveUploadDirecto(t *testing.T) {
	env := setupTestDB(t)
	_ = env

	mkReq := func(field, fname string, data []byte) *http.Request {
		var b bytes.Buffer
		w := multipart.NewWriter(&b)
		fw, _ := w.CreateFormFile(field, fname)
		_, _ = fw.Write(data)
		_ = w.Close()
		req := httptest.NewRequest(http.MethodPost, "/admin/new", &b)
		req.Header.Set("Content-Type", w.FormDataContentType())
		return req
	}
	// Campo ausente => "", nil.
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	_ = w.Close()
	req := httptest.NewRequest(http.MethodPost, "/admin/new", &b)
	req.Header.Set("Content-Type", w.FormDataContentType())
	if name, err := saveUpload(req, "image", imgExts, 10<<20); name != "" || err != nil {
		t.Fatalf("ausente = %q, %v", name, err)
	}
	// Ext mala.
	if _, err := saveUpload(mkReq("image", "x.bmp", []byte("d")), "image", imgExts, 10<<20); err == nil {
		t.Fatal("ext mala debió fallar")
	}
	// Muy grande (límite artificial de 10 bytes).
	if _, err := saveUpload(mkReq("image", "x.png", bytes.Repeat([]byte("a"), 100)), "image", imgExts, 10); err == nil {
		t.Fatal("oversize debió fallar")
	}
	// OK escribe el archivo.
	name, err := saveUpload(mkReq("image", "Mi Foto.PNG", []byte("datos")), "image", imgExts, 10<<20)
	if err != nil || !strings.HasSuffix(name, ".png") {
		t.Fatalf("ok = %q, %v", name, err)
	}
	if _, err := os.Stat(filepath.Join(uploadsDir, name)); err != nil {
		t.Fatalf("no está en disco: %v", err)
	}
	// Error de escritura (directorio inexistente).
	prev := uploadsDir
	uploadsDir = filepath.Join(t.TempDir(), "nodir")
	defer func() { uploadsDir = prev }()
	if _, err := saveUpload(mkReq("image", "x.png", []byte("d")), "image", imgExts, 10<<20); err == nil {
		t.Fatal("sin directorio debió fallar")
	}
}

func TestJoinExtsRemoveUploads(t *testing.T) {
	env := setupTestDB(t)
	_ = env
	if joinExts(imgExts) == "" {
		t.Fatal("joinExts vacío")
	}
	if err := os.WriteFile(filepath.Join(uploadsDir, "borrame.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	removeUploads("borrame.png", "", "con/slash", "..", "noexiste.png") // no debe tronar
	if _, err := os.Stat(filepath.Join(uploadsDir, "borrame.png")); !os.IsNotExist(err) {
		t.Fatal("removeUploads no borró")
	}
}

func TestForumFlow(t *testing.T) {
	env := setupTestDB(t)
	cookie, csrf := loginAs(t, "usuario1", "password1")

	if rec := doReq(env.mux, http.MethodGet, "/forum", nil, nil); rec.Code != http.StatusOK {
		t.Fatalf("lista = %d", rec.Code)
	}
	if rec := doReq(env.mux, http.MethodGet, "/forum/new", nil, nil); rec.Code != http.StatusSeeOther {
		t.Fatalf("new anónimo = %d", rec.Code)
	}
	if rec := doReq(env.mux, http.MethodGet, "/forum/new", nil, cookie); rec.Code != http.StatusOK {
		t.Fatalf("new = %d", rec.Code)
	}
	// Validaciones.
	for name, f := range map[string]url.Values{
		"sin título":   authedForm(csrf, map[string]string{"title": "", "body": "b"}),
		"sin cuerpo":   authedForm(csrf, map[string]string{"title": "t", "body": "  "}),
		"cuerpo largo": authedForm(csrf, map[string]string{"title": "t", "body": strings.Repeat("x", 20001)}),
	} {
		if rec := doReq(env.mux, http.MethodPost, "/forum/new", f, cookie); rec.Code != http.StatusOK {
			t.Fatalf("forum new %s = %d, quiero 200 con flash", name, rec.Code)
		}
	}
	rec := doReq(env.mux, http.MethodPost, "/forum/new", authedForm(csrf, map[string]string{"title": "Hilo HTTP", "body": "contenido"}), cookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("crear hilo = %d", rec.Code)
	}
	var tid int64
	if _, err := fmt.Sscanf(rec.Header().Get("Location"), "/forum/%d", &tid); err != nil {
		t.Fatalf("Location: %q", rec.Header().Get("Location"))
	}
	if rec := doReq(env.mux, http.MethodGet, fmt.Sprintf("/forum/%d", tid), nil, nil); rec.Code != http.StatusOK {
		t.Fatalf("hilo = %d", rec.Code)
	}
	if rec := doReq(env.mux, http.MethodGet, "/forum/999999", nil, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("hilo 404 = %d", rec.Code)
	}
	// Respuestas.
	ruta := fmt.Sprintf("/forum/%d/reply", tid)
	if rec := doReq(env.mux, http.MethodPost, ruta, authedForm(csrf, map[string]string{"body": "respuesta"}), cookie); rec.Code != http.StatusSeeOther {
		t.Fatalf("reply = %d", rec.Code)
	}
	if rec := doReq(env.mux, http.MethodGet, fmt.Sprintf("/forum/%d", tid), nil, nil); !strings.Contains(rec.Body.String(), "respuesta") {
		t.Fatal("respuesta no visible")
	}
	for name, body := range map[string]string{"vacía": "  ", "larga": strings.Repeat("x", 20001)} {
		if rec := doReq(env.mux, http.MethodPost, ruta, authedForm(csrf, map[string]string{"body": body}), cookie); rec.Code != http.StatusBadRequest {
			t.Fatalf("reply %s = %d", name, rec.Code)
		}
	}
	if rec := doReq(env.mux, http.MethodPost, "/forum/0/reply", authedForm(csrf, map[string]string{"body": "x"}), cookie); rec.Code != http.StatusBadRequest {
		t.Fatalf("reply id malo = %d", rec.Code)
	}
}

func TestPreview(t *testing.T) {
	env := setupTestDB(t)
	// Anónimo => redirect a login.
	if rec := doReq(env.mux, http.MethodPost, "/preview", url.Values{"body": {"x"}}, nil); rec.Code != http.StatusSeeOther {
		t.Fatalf("preview anónimo = %d", rec.Code)
	}
	cookie, csrf := loginAs(t, "usuario1", "password1")
	rec := doReq(env.mux, http.MethodPost, "/preview", authedForm(csrf, map[string]string{"body": "# T"}), cookie)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<h1") {
		t.Fatalf("preview = %d %q", rec.Code, rec.Body.String())
	}
	// Error de escritura cubre la rama de log sin tronar.
	cookie2, csrf2 := loginAs(t, "usuario1", "password1")
	bad := httptest.NewRequest(http.MethodPost, "/preview", strings.NewReader(url.Values{"body": {"x"}, "csrf": {csrf2}}.Encode()))
	bad.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	bad.AddCookie(cookie2)
	previewHandler(&failingWriter{header: http.Header{}}, bad)
}

func TestRenderDesconocido(t *testing.T) {
	setupTestDB(t)
	rec := httptest.NewRecorder()
	render(rec, "pagina-que-no-existe", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d, quiero 500", rec.Code)
	}
}

func TestLoadTemplatesError(t *testing.T) {
	setupTestDB(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatalf("chdir back: %v", err)
		}
	}()
	if err := loadTemplates(); err == nil {
		t.Fatal("loadTemplates sin templates/ debió fallar")
	}
}

func TestNotFound(t *testing.T) {
	env := setupTestDB(t)
	rec := doReq(env.mux, http.MethodGet, "/ruta-que-no-existe", nil, nil)
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "PALOMA") {
		t.Fatalf("404 = %d", rec.Code)
	}
}
