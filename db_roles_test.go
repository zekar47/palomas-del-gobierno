package main

// Tests de la capa de datos agregada: migración de roles, gestión de
// usuarios, edición de hilos y comentarios.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMigracionRoles(t *testing.T) {
	env := setupTestDB(t)
	_ = env

	// Versión actual.
	var v string
	if err := db.QueryRow(`SELECT v FROM meta WHERE k = 'schema_version'`).Scan(&v); err != nil || v != "2" {
		t.Fatalf("schema_version = %q, %v", v, err)
	}
	// Idempotente.
	if err := migrate(); err != nil {
		t.Fatalf("migrate repetido: %v", err)
	}

	// Simular BD vieja (CHECK sin 'member') con datos, migrar y verificar.
	if _, err := db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TABLE users`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE users (
	  id INTEGER PRIMARY KEY AUTOINCREMENT, username TEXT UNIQUE NOT NULL COLLATE NOCASE,
	  password_hash TEXT NOT NULL, role TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('admin','user')),
	  created_at TEXT NOT NULL DEFAULT (datetime('now'))) `); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM meta WHERE k = 'schema_version'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users (username, password_hash, role) VALUES ('viejo_admin','h','admin'),('viejo_user','h','user')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		t.Fatal(err)
	}
	if err := migrate(); err != nil {
		t.Fatalf("migrate sobre BD vieja: %v", err)
	}
	// Datos intactos + rol member ya permitido.
	users, err := listUsers()
	if err != nil || len(users) != 2 {
		t.Fatalf("tras migrar: %+v, %v", users, err)
	}
	if _, err := db.Exec(`INSERT INTO users (username, password_hash, role) VALUES ('banda1','h','member')`); err != nil {
		t.Fatalf("insert member tras migrar: %v", err)
	}
	// Integridad FK sana.
	var bad int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_foreign_key_check`).Scan(&bad); err != nil || bad != 0 {
		t.Fatalf("fk check = %d, %v", bad, err)
	}
	// Nuevos inserts siguen autonumerando bien.
	id, err := createPost(users[0].ID, "post-migración", "c")
	if err != nil || id == 0 {
		t.Fatalf("createPost tras migrar: %d, %v", id, err)
	}
}

func TestRoles(t *testing.T) {
	env := setupTestDB(t)

	if !env.admin.canPostNews() || !env.admin.IsAdmin() {
		t.Fatal("admin debe poder todo")
	}
	if env.user.canPostNews() || env.user.IsAdmin() {
		t.Fatal("user normal no debe poder noticias")
	}
	var nilUser *User
	if nilUser.canPostNews() || nilUser.IsAdmin() || canEdit(nilUser, 1) {
		t.Fatal("nil no debe poder nada")
	}

	m, err := registerUser("banda1", "password1")
	if err != nil {
		t.Fatal(err)
	}
	if err := setUserRole(m.ID, "member"); err != nil {
		t.Fatalf("promover: %v", err)
	}
	m, _ = getUser(m.ID)
	if m.Role != "member" || !m.canPostNews() || m.IsAdmin() {
		t.Fatalf("member: %+v", m)
	}
	if err := setUserRole(m.ID, "user"); err != nil {
		t.Fatalf("degradar: %v", err)
	}
	for _, bad := range []string{"admin", "root", "", "ADMIN"} {
		if err := setUserRole(m.ID, bad); err == nil {
			t.Fatalf("rol %q debió fallar", bad)
		}
	}
	if err := setUserRole(999999, "member"); err == nil {
		t.Fatal("usuario inexistente debió fallar")
	}
	// No se puede degradar ni eliminar al último admin.
	if err := setUserRole(env.admin.ID, "member"); err == nil {
		t.Fatal("degradar último admin debió fallar")
	}
	if _, err := deleteUser(env.admin.ID); err == nil {
		t.Fatal("eliminar último admin debió fallar")
	}
	// Con dos admins sí se puede.
	a2, _ := registerUser("admin2", "password1")
	if _, err := db.Exec(`UPDATE users SET role = 'admin' WHERE id = ?`, a2.ID); err != nil {
		t.Fatal(err)
	}
	if err := setUserRole(env.admin.ID, "member"); err != nil {
		t.Fatalf("degradar con 2 admins: %v", err)
	}

	users, err := listUsers()
	if err != nil || len(users) < 3 {
		t.Fatalf("listUsers: %d, %v", len(users), err)
	}
	if users[0].ID > users[1].ID {
		t.Fatal("listUsers no ordenado por id")
	}
}

func TestRequireMember(t *testing.T) {
	setupTestDB(t)
	next := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) }
	h := requireMember(next)

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/admin/new", nil))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("anónimo = %d", rec.Code)
	}
	ucookie, _ := loginAs(t, "usuario1", "password1")
	req := httptest.NewRequest(http.MethodGet, "/admin/new", nil)
	req.AddCookie(ucookie)
	rec = httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("user = %d, quiero redirect", rec.Code)
	}
	m, _ := registerUser("banda9", "password1")
	_ = setUserRole(m.ID, "member")
	mcookie, _ := loginAs(t, "banda9", "password1")
	mreq := httptest.NewRequest(http.MethodGet, "/admin/new", nil)
	mreq.AddCookie(mcookie)
	rec = httptest.NewRecorder()
	h(rec, mreq)
	if rec.Code != http.StatusTeapot {
		t.Fatalf("member = %d, quiero 418", rec.Code)
	}
}

func TestUpdateUsernamePassword(t *testing.T) {
	env := setupTestDB(t)

	if err := updateUsername(env.user.ID, "ab"); err == nil {
		t.Fatal("nombre corto debió fallar")
	}
	if err := updateUsername(env.user.ID, strings.Repeat("a", 25)); err == nil {
		t.Fatal("nombre largo debió fallar")
	}
	if err := updateUsername(env.user.ID, "ADMIN"); err == nil {
		t.Fatal("duplicado NOCASE debió fallar")
	}
	if err := updateUsername(env.user.ID, "  nuevo-nombre  "); err != nil {
		t.Fatalf("renombrar: %v", err)
	}
	u, _ := getUser(env.user.ID)
	if u.Username != "nuevo-nombre" {
		t.Fatalf("username = %q", u.Username)
	}
	// Cambiar mayúsculas propias sí vale.
	if err := updateUsername(env.user.ID, "NUEVO-NOMBRE"); err != nil {
		t.Fatalf("cambio propio: %v", err)
	}

	if !verifyPassword(env.user.ID, "password1") || verifyPassword(env.user.ID, "mala") {
		t.Fatal("verifyPassword mal")
	}
	if verifyPassword(999999, "x") {
		t.Fatal("verifyPassword inexistente debió ser false")
	}
	if err := setPassword(env.user.ID, "12345"); err == nil {
		t.Fatal("clave corta debió fallar")
	}
	if err := setPassword(env.user.ID, "nueva-clave-larga"); err != nil {
		t.Fatalf("setPassword: %v", err)
	}
	if _, err := loginUser("NUEVO-NOMBRE", "nueva-clave-larga"); err != nil {
		t.Fatalf("login tras cambio: %v", err)
	}
}

func TestDeleteUserCascada(t *testing.T) {
	env := setupTestDB(t)

	// Víctima con de todo: post con imagen, comentario propio y ajeno en su
	// post, hilo con respuesta ajena, reacción y respuesta en hilo ajeno.
	victim, _ := registerUser("victima", "password1")
	pid, _ := createPost(victim.ID, "P", "cuerpo")
	_ = updatePost(pid, "P", "cuerpo", "foto.png", "")
	_ = addComment(pid, victim.ID, "mío")
	_ = addComment(pid, env.user.ID, "ajeno en post ajeno")
	tid, _ := createThread(victim.ID, "H", "cuerpo")
	_ = addReply(tid, env.admin.ID, "respuesta ajena en hilo ajeno")
	_ = toggleReaction(pid, victim.ID, "like")
	tid2, _ := createThread(env.user.ID, "H2", "c")
	_ = addReply(tid2, victim.ID, "respuesta mía en hilo ajeno")

	paths, err := deleteUser(victim.ID)
	if err != nil {
		t.Fatalf("deleteUser: %v", err)
	}
	if len(paths) != 1 || paths[0] != "foto.png" {
		t.Fatalf("paths = %v", paths)
	}
	tablas := map[string]int{"users": 2, "posts": 0, "comments": 0, "threads": 1, "replies": 0, "reactions": 0}
	for tbl, want := range tablas {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + tbl).Scan(&n); err != nil || n != want {
			t.Fatalf("%s = %d (quiero %d), err=%v", tbl, n, want, err)
		}
	}
	if _, err := deleteUser(999999); err == nil {
		t.Fatal("inexistente debió fallar")
	}
}

func TestThreadCommentMutations(t *testing.T) {
	env := setupTestDB(t)

	tid, _ := createThread(env.user.ID, "T", "cuerpo")
	if err := updateThread(tid, "T2", "cuerpo2"); err != nil {
		t.Fatalf("updateThread: %v", err)
	}
	th, _ := getThread(tid)
	if th.Title != "T2" || th.Body != "cuerpo2" {
		t.Fatalf("thread: %+v", th)
	}
	_ = addReply(tid, env.admin.ID, "r")
	if err := deleteThread(tid); err != nil {
		t.Fatalf("deleteThread: %v", err)
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM replies`).Scan(&n)
	if n != 0 {
		t.Fatal("replies huérfanas tras deleteThread")
	}

	pid, _ := createPost(env.admin.ID, "P", "c")
	_ = addComment(pid, env.user.ID, "original")
	cs, _ := listComments(pid)
	c, err := getComment(cs[0].ID)
	if err != nil || c.Body != "original" || c.Username != "usuario1" {
		t.Fatalf("getComment: %+v, %v", c, err)
	}
	if _, err := getComment(999999); err == nil {
		t.Fatal("getComment inexistente debió fallar")
	}
	if err := updateComment(c.ID, "editado"); err != nil {
		t.Fatalf("updateComment: %v", err)
	}
	if err := deleteComment(c.ID); err != nil {
		t.Fatalf("deleteComment: %v", err)
	}
	cs, _ = listComments(pid)
	if len(cs) != 0 {
		t.Fatal("comentario sigue tras borrar")
	}

	if !canEdit(env.admin, env.user.ID) || !canEdit(env.user, env.user.ID) || canEdit(env.user, env.admin.ID) {
		t.Fatal("canEdit mal")
	}
}

func TestReplyMutations(t *testing.T) {
	env := setupTestDB(t)

	tid, _ := createThread(env.user.ID, "T", "c")
	_ = addReply(tid, env.user.ID, "original")
	rs, _ := listReplies(tid)
	rp, err := getReply(rs[0].ID)
	if err != nil || rp.Body != "original" || rp.ThreadID != tid || rp.Username != "usuario1" {
		t.Fatalf("getReply: %+v, %v", rp, err)
	}
	if _, err := getReply(999999); err == nil {
		t.Fatal("getReply inexistente debió fallar")
	}
	if err := updateReply(rp.ID, "editada"); err != nil {
		t.Fatalf("updateReply: %v", err)
	}
	if err := deleteReply(rp.ID); err != nil {
		t.Fatalf("deleteReply: %v", err)
	}
	rs, _ = listReplies(tid)
	if len(rs) != 0 {
		t.Fatal("respuesta sigue tras borrar")
	}
}
