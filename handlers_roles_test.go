package main

// Tests HTTP de edición propia (hilos/comentarios), administración de
// usuarios, cuenta propia y permisos del rol member.

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestForumEditDelete(t *testing.T) {
	env := setupTestDB(t)
	ucookie, ucsrf := loginAs(t, "usuario1", "password1")
	acookie, acsrf := loginAs(t, "admin", "admin-test-pass")

	tid, _ := createThread(env.user.ID, "Hilo", "cuerpo")
	ruta := fmt.Sprintf("/forum/%d/edit", tid)
	del := fmt.Sprintf("/forum/%d/delete", tid)

	// Anónimo va a login.
	if rec := doReq(env.mux, http.MethodGet, ruta, nil, nil); rec.Code != http.StatusSeeOther {
		t.Fatalf("anónimo = %d", rec.Code)
	}
	// Dueño ve el form y edita.
	if rec := doReq(env.mux, http.MethodGet, ruta, nil, ucookie); rec.Code != http.StatusOK {
		t.Fatalf("dueño get = %d", rec.Code)
	}
	rec := doReq(env.mux, http.MethodPost, ruta, authedForm(ucsrf, map[string]string{"title": "Hilo v2", "body": "nuevo"}), ucookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("dueño post = %d", rec.Code)
	}
	th, _ := getThread(tid)
	if th.Title != "Hilo v2" {
		t.Fatalf("no se guardó: %+v", th)
	}
	// Validaciones re-renderizan con flash.
	for name, f := range map[string]url.Values{
		"sin título":   authedForm(ucsrf, map[string]string{"title": "", "body": "b"}),
		"sin cuerpo":   authedForm(ucsrf, map[string]string{"title": "t", "body": "  "}),
		"cuerpo largo": authedForm(ucsrf, map[string]string{"title": "t", "body": strings.Repeat("x", 20001)}),
	} {
		if rec := doReq(env.mux, http.MethodPost, ruta, f, ucookie); rec.Code != http.StatusOK {
			t.Fatalf("validación %s = %d", name, rec.Code)
		}
	}
	// Otro usuario: 403 en GET, POST y DELETE.
	if _, err := registerUser("otro1", "password1"); err != nil {
		t.Fatal(err)
	}
	ocookie, ocsrf := loginAs(t, "otro1", "password1")
	if rec := doReq(env.mux, http.MethodGet, ruta, nil, ocookie); rec.Code != http.StatusForbidden {
		t.Fatalf("ajeno get = %d", rec.Code)
	}
	if rec := doReq(env.mux, http.MethodPost, ruta, authedForm(ocsrf, map[string]string{"title": "x", "body": "y"}), ocookie); rec.Code != http.StatusForbidden {
		t.Fatalf("ajeno post = %d", rec.Code)
	}
	if rec := doReq(env.mux, http.MethodPost, del, authedForm(ocsrf, nil), ocookie); rec.Code != http.StatusForbidden {
		t.Fatalf("ajeno delete = %d", rec.Code)
	}
	// Admin sí puede editar lo ajeno.
	rec = doReq(env.mux, http.MethodPost, ruta, authedForm(acsrf, map[string]string{"title": "Por admin", "body": "b"}), acookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("admin post = %d", rec.Code)
	}
	// Inexistente: 404.
	if rec := doReq(env.mux, http.MethodGet, "/forum/999999/edit", nil, ucookie); rec.Code != http.StatusNotFound {
		t.Fatalf("404 = %d", rec.Code)
	}
	if rec := doReq(env.mux, http.MethodPost, "/forum/999999/delete", authedForm(ucsrf, nil), ucookie); rec.Code != http.StatusNotFound {
		t.Fatalf("delete 404 = %d", rec.Code)
	}
	// Dueño borra: hilo + respuestas fuera.
	_ = addReply(tid, env.admin.ID, "r")
	if rec := doReq(env.mux, http.MethodPost, del, authedForm(ucsrf, nil), ucookie); rec.Code != http.StatusSeeOther {
		t.Fatalf("dueño delete = %d", rec.Code)
	}
	if _, err := getThread(tid); err == nil {
		t.Fatal("hilo sigue tras borrar")
	}
}

func TestCommentEditDelete(t *testing.T) {
	env := setupTestDB(t)
	ucookie, ucsrf := loginAs(t, "usuario1", "password1")
	acookie, acsrf := loginAs(t, "admin", "admin-test-pass")

	pid, _ := createPost(env.admin.ID, "P", "c")
	_ = addComment(pid, env.user.ID, "original")
	cs, _ := listComments(pid)
	cid := cs[0].ID
	ruta := fmt.Sprintf("/comment/%d/edit", cid)
	del := fmt.Sprintf("/comment/%d/delete", cid)

	if rec := doReq(env.mux, http.MethodGet, ruta, nil, nil); rec.Code != http.StatusSeeOther {
		t.Fatalf("anónimo = %d", rec.Code)
	}
	if rec := doReq(env.mux, http.MethodGet, ruta, nil, ucookie); rec.Code != http.StatusOK {
		t.Fatalf("dueño get = %d", rec.Code)
	}
	rec := doReq(env.mux, http.MethodPost, ruta, authedForm(ucsrf, map[string]string{"body": "editado"}), ucookie)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != fmt.Sprintf("/news/%d", pid) {
		t.Fatalf("dueño post = %d loc=%q", rec.Code, rec.Header().Get("Location"))
	}
	c, _ := getComment(cid)
	if c.Body != "editado" {
		t.Fatalf("no se guardó: %+v", c)
	}
	for name, body := range map[string]string{"vacío": "  ", "largo": strings.Repeat("x", 4001)} {
		if rec := doReq(env.mux, http.MethodPost, ruta, authedForm(ucsrf, map[string]string{"body": body}), ucookie); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s = %d", name, rec.Code)
		}
	}
	// Ajeno: 403. Admin: ok.
	_, _ = registerUser("otro2", "password1")
	ocookie, ocsrf := loginAs(t, "otro2", "password1")
	if rec := doReq(env.mux, http.MethodPost, del, authedForm(ocsrf, nil), ocookie); rec.Code != http.StatusForbidden {
		t.Fatalf("ajeno delete = %d", rec.Code)
	}
	if rec := doReq(env.mux, http.MethodGet, ruta, nil, ocookie); rec.Code != http.StatusForbidden {
		t.Fatalf("ajeno get = %d", rec.Code)
	}
	rec = doReq(env.mux, http.MethodPost, ruta, authedForm(acsrf, map[string]string{"body": "por admin"}), acookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("admin post = %d", rec.Code)
	}
	if rec := doReq(env.mux, http.MethodGet, "/comment/999999/edit", nil, ucookie); rec.Code != http.StatusNotFound {
		t.Fatalf("404 = %d", rec.Code)
	}
	if rec := doReq(env.mux, http.MethodPost, del, authedForm(ucsrf, nil), ucookie); rec.Code != http.StatusSeeOther {
		t.Fatalf("dueño delete = %d", rec.Code)
	}
	cs, _ = listComments(pid)
	if len(cs) != 0 {
		t.Fatal("comentario sigue")
	}
}

func TestAdminUsers(t *testing.T) {
	env := setupTestDB(t)
	acookie, acsrf := loginAs(t, "admin", "admin-test-pass")
	ucookie, _ := loginAs(t, "usuario1", "password1")
	_ = env

	if rec := doReq(env.mux, http.MethodGet, "/admin/users", nil, nil); rec.Code != http.StatusSeeOther {
		t.Fatalf("anónimo = %d", rec.Code)
	}
	if rec := doReq(env.mux, http.MethodGet, "/admin/users", nil, ucookie); rec.Code != http.StatusSeeOther {
		t.Fatalf("user = %d", rec.Code)
	}
	rec := doReq(env.mux, http.MethodGet, "/admin/users", nil, acookie)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "usuario1") {
		t.Fatalf("admin = %d", rec.Code)
	}
	// Cambiar rol a member y de vuelta.
	uid := env.user.ID
	ruta := fmt.Sprintf("/admin/users/%d/role", uid)
	rec = doReq(env.mux, http.MethodPost, ruta, authedForm(acsrf, map[string]string{"role": "member"}), acookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("role member = %d", rec.Code)
	}
	u, _ := getUser(uid)
	if u.Role != "member" {
		t.Fatalf("rol = %q", u.Role)
	}
	rec = doReq(env.mux, http.MethodPost, ruta, authedForm(acsrf, map[string]string{"role": "user"}), acookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("role user = %d", rec.Code)
	}
	// Rol inválido e inexistente.
	rec = doReq(env.mux, http.MethodPost, ruta, authedForm(acsrf, map[string]string{"role": "root"}), acookie)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "inválido") {
		t.Fatalf("rol malo = %d", rec.Code)
	}
	if rec := doReq(env.mux, http.MethodPost, "/admin/users/999999/role", authedForm(acsrf, map[string]string{"role": "member"}), acookie); rec.Code != http.StatusNotFound {
		t.Fatalf("inexistente = %d", rec.Code)
	}
	if rec := doReq(env.mux, http.MethodPost, "/admin/users/0/role", authedForm(acsrf, map[string]string{"role": "member"}), acookie); rec.Code != http.StatusNotFound {
		t.Fatalf("id 0 = %d", rec.Code)
	}
	// Degradar al último admin falla con mensaje.
	aid := env.admin.ID
	rec = doReq(env.mux, http.MethodPost, fmt.Sprintf("/admin/users/%d/role", aid), authedForm(acsrf, map[string]string{"role": "user"}), acookie)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "ltimo admin") {
		t.Fatalf("degradar último = %d", rec.Code)
	}
	// Borrar usuario borra su contenido y archivos.
	pid, _ := createPost(uid, "Borrable", "c")
	_ = updatePost(pid, "Borrable", "c", "borrame.png", "")
	_ = addComment(pid, uid, "x")
	drec := doReq(env.mux, http.MethodPost, fmt.Sprintf("/admin/users/%d/delete", uid), authedForm(acsrf, nil), acookie)
	if drec.Code != http.StatusSeeOther {
		t.Fatalf("delete = %d", drec.Code)
	}
	if _, err := getUser(uid); err == nil {
		t.Fatal("usuario sigue")
	}
	if _, err := getPost(pid); err == nil {
		t.Fatal("post sigue")
	}
	// Borrar último admin falla; borrar inexistente 404.
	if rec := doReq(env.mux, http.MethodPost, fmt.Sprintf("/admin/users/%d/delete", aid), authedForm(acsrf, nil), acookie); rec.Code != http.StatusOK {
		t.Fatalf("delete último admin = %d", rec.Code)
	}
	if rec := doReq(env.mux, http.MethodPost, "/admin/users/999999/delete", authedForm(acsrf, nil), acookie); rec.Code != http.StatusNotFound {
		t.Fatalf("delete 404 = %d", rec.Code)
	}
	if rec := doReq(env.mux, http.MethodPost, "/admin/users/0/delete", authedForm(acsrf, nil), acookie); rec.Code != http.StatusNotFound {
		t.Fatalf("delete id 0 = %d", rec.Code)
	}
}

func TestAccount(t *testing.T) {
	env := setupTestDB(t)
	cookie, csrf := loginAs(t, "usuario1", "password1")

	if rec := doReq(env.mux, http.MethodGet, "/account", nil, nil); rec.Code != http.StatusSeeOther {
		t.Fatalf("anónimo = %d", rec.Code)
	}
	if rec := doReq(env.mux, http.MethodGet, "/account", nil, cookie); rec.Code != http.StatusOK {
		t.Fatalf("get = %d", rec.Code)
	}
	// Renombrar.
	rec := doReq(env.mux, http.MethodPost, "/account/username", authedForm(csrf, map[string]string{"username": "nuevo-yo"}), cookie)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "nombre actualizado") {
		t.Fatalf("rename = %d", rec.Code)
	}
	rec = doReq(env.mux, http.MethodPost, "/account/username", authedForm(csrf, map[string]string{"username": "admin"}), cookie)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "ocupado") {
		t.Fatalf("rename dup = %d", rec.Code)
	}
	rec = doReq(env.mux, http.MethodPost, "/account/username", authedForm(csrf, map[string]string{"username": "ab"}), cookie)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "3 y 24") {
		t.Fatalf("rename corto = %d", rec.Code)
	}
	// Contraseña.
	rec = doReq(env.mux, http.MethodPost, "/account/password", authedForm(csrf, map[string]string{"current": "mala", "new": "nueva-clave-1"}), cookie)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "no coincide") {
		t.Fatalf("pass mala = %d", rec.Code)
	}
	rec = doReq(env.mux, http.MethodPost, "/account/password", authedForm(csrf, map[string]string{"current": "password1", "new": "123"}), cookie)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "6 caracteres") {
		t.Fatalf("pass corta = %d", rec.Code)
	}
	rec = doReq(env.mux, http.MethodPost, "/account/password", authedForm(csrf, map[string]string{"current": "password1", "new": "nueva-clave-1"}), cookie)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "actualizada") {
		t.Fatalf("pass ok = %d", rec.Code)
	}
	if _, err := loginUser("nuevo-yo", "nueva-clave-1"); err != nil {
		t.Fatalf("login nuevo: %v", err)
	}
	// Eliminar cuenta propia: sesión muerta y datos fuera.
	ncookie, ncsrf := loginAs(t, "nuevo-yo", "nueva-clave-1")
	uid := env.user.ID
	rec = doReq(env.mux, http.MethodPost, "/account/delete", authedForm(ncsrf, nil), ncookie)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Fatalf("self delete = %d", rec.Code)
	}
	if _, err := getUser(uid); err == nil {
		t.Fatal("cuenta sigue")
	}
	if rec := doReq(env.mux, http.MethodGet, "/account", nil, ncookie); rec.Code != http.StatusSeeOther {
		t.Fatalf("tras delete = %d", rec.Code)
	}
	// El último admin no puede auto-eliminarse.
	acookie, acsrf := loginAs(t, "admin", "admin-test-pass")
	rec = doReq(env.mux, http.MethodPost, "/account/delete", authedForm(acsrf, nil), acookie)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "ltimo admin") {
		t.Fatalf("self delete último admin = %d", rec.Code)
	}
}

func TestMemberNews(t *testing.T) {
	env := setupTestDB(t)
	m, _ := registerUser("banda2", "password1")
	_ = setUserRole(m.ID, "member")
	mcookie, mcsrf := loginAs(t, "banda2", "password1")
	ucookie, _ := loginAs(t, "usuario1", "password1")
	_ = env

	// Miembro crea noticia.
	mrec, mreq := multipartPost(t, "/admin/new",
		map[string]string{"title": "De banda", "body": "cuerpo", "csrf": mcsrf}, "", "", nil, mcookie)
	doMultipart(env.mux, mrec, mreq)
	if mrec.Code != http.StatusSeeOther {
		t.Fatalf("member crear = %d", mrec.Code)
	}
	mid := postIDFromLoc(t, mrec.Header().Get("Location"))

	// Usuario normal: requireMember lo redirige a login antes de validar CSRF.
	if rec := doReq(env.mux, http.MethodGet, "/admin/new", nil, ucookie); rec.Code != http.StatusSeeOther {
		t.Fatalf("user get = %d", rec.Code)
	}
	rec := doReq(env.mux, http.MethodPost, "/admin/new", url.Values{}, ucookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("user post = %d", rec.Code)
	}

	// Noticia del admin para probar límites del miembro.
	apid, _ := createPost(env.admin.ID, "Del admin", "c")
	// Miembro edita la suya: ok. La del admin: 403.
	er, ereq := multipartPost(t, fmt.Sprintf("/admin/edit/%d", mid),
		map[string]string{"title": "De banda v2", "body": "c", "csrf": mcsrf}, "", "", nil, mcookie)
	doMultipart(env.mux, er, ereq)
	if er.Code != http.StatusSeeOther {
		t.Fatalf("member edit propia = %d", er.Code)
	}
	er, ereq = multipartPost(t, fmt.Sprintf("/admin/edit/%d", apid),
		map[string]string{"title": "hack", "body": "c", "csrf": mcsrf}, "", "", nil, mcookie)
	doMultipart(env.mux, er, ereq)
	if er.Code != http.StatusForbidden {
		t.Fatalf("member edit ajena = %d", er.Code)
	}
	if rec := doReq(env.mux, http.MethodGet, fmt.Sprintf("/admin/edit/%d", apid), nil, mcookie); rec.Code != http.StatusForbidden {
		t.Fatalf("member get ajena = %d", rec.Code)
	}
	if rec := doReq(env.mux, http.MethodPost, fmt.Sprintf("/admin/delete/%d", apid), authedForm(mcsrf, nil), mcookie); rec.Code != http.StatusForbidden {
		t.Fatalf("member delete ajena = %d", rec.Code)
	}
	// Miembro borra la suya.
	if rec := doReq(env.mux, http.MethodPost, fmt.Sprintf("/admin/delete/%d", mid), authedForm(mcsrf, nil), mcookie); rec.Code != http.StatusSeeOther {
		t.Fatalf("member delete propia = %d", rec.Code)
	}
	// Miembro no entra a /admin/users.
	if rec := doReq(env.mux, http.MethodGet, "/admin/users", nil, mcookie); rec.Code != http.StatusSeeOther {
		t.Fatalf("member users = %d", rec.Code)
	}
}
