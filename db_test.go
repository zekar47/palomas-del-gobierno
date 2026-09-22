package main

import (
	"database/sql"
	"testing"
)

func TestMigrateIdempotente(t *testing.T) {
	env := setupTestDB(t)
	_ = env
	if err := migrate(); err != nil {
		t.Fatalf("segundo migrate: %v", err)
	}
	if err := migrate(); err != nil {
		t.Fatalf("tercer migrate: %v", err)
	}
}

func TestSeedAdmin(t *testing.T) {
	setupTestDB(t) // ya deja un admin creado

	// Con usuarios existentes es no-op.
	if err := seedAdmin("otra-clave"); err != nil {
		t.Fatalf("seedAdmin no-op: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("usuarios = %d, err = %v; quiero 2 (admin+usuario1)", n, err)
	}
	// La clave del admin sigue siendo la original, no "otra-clave".
	if _, err := loginUser("admin", "admin-test-pass"); err != nil {
		t.Fatalf("login admin con clave original: %v", err)
	}

	// BD vacía + pass vacío => crea admin/admin123.
	if _, err := db.Exec(`DELETE FROM users`); err != nil {
		t.Fatalf("limpiar users: %v", err)
	}
	if err := seedAdmin(""); err != nil {
		t.Fatalf("seedAdmin default: %v", err)
	}
	if _, err := loginUser("admin", "admin123"); err != nil {
		t.Fatalf("login admin/admin123: %v", err)
	}
}

func TestGetUser(t *testing.T) {
	env := setupTestDB(t)

	u, err := getUser(env.admin.ID)
	if err != nil || u.Username != "admin" || u.Role != "admin" {
		t.Fatalf("getUser admin: %+v, %v", u, err)
	}
	if _, err := getUser(999999); err != sql.ErrNoRows {
		t.Fatalf("getUser inexistente: quiero ErrNoRows, tengo %v", err)
	}
	if _, err := getUserByName("ADMIN"); err != nil { // NOCASE
		t.Fatalf("getUserByName case-insensitive: %v", err)
	}
	if _, err := getUserByName("nadie"); err != sql.ErrNoRows {
		t.Fatalf("getUserByName inexistente: quiero ErrNoRows, tengo %v", err)
	}

	var nilUser *User
	if nilUser.isAdmin() {
		t.Fatal("nil.isAdmin() debe ser false")
	}
	if !env.admin.isAdmin() || env.user.isAdmin() {
		t.Fatal("roles admin/user incorrectos")
	}
}

func TestPostsCRUD(t *testing.T) {
	env := setupTestDB(t)

	id, err := createPost(env.admin.ID, "Título 1", "cuerpo **fuerte**")
	if err != nil {
		t.Fatalf("createPost: %v", err)
	}
	p, err := getPost(id)
	if err != nil {
		t.Fatalf("getPost: %v", err)
	}
	if p.Title != "Título 1" || p.Author != "admin" || p.Likes != 0 || p.Comments != 0 {
		t.Fatalf("post inesperado: %+v", p)
	}
	if _, err := getPost(999999); err == nil {
		t.Fatal("getPost inexistente debe fallar")
	}

	if err := updatePost(id, "Título 1 ed", "nuevo cuerpo", "img.png", "vid.mp4"); err != nil {
		t.Fatalf("updatePost: %v", err)
	}
	p, _ = getPost(id)
	if p.Title != "Título 1 ed" || p.ImagePath != "img.png" || p.VideoPath != "vid.mp4" {
		t.Fatalf("update no aplicado: %+v", p)
	}

	id2, _ := createPost(env.user.ID, "Título 2", "otro")
	posts, err := listPosts(0) // sin límite
	if err != nil || len(posts) != 2 {
		t.Fatalf("listPosts sin límite: %d, %v", len(posts), err)
	}
	posts, err = listPosts(1)
	if err != nil || len(posts) != 1 {
		t.Fatalf("listPosts(1): %d, %v", len(posts), err)
	}
	_ = id2

	if err := deletePost(id); err != nil {
		t.Fatalf("deletePost: %v", err)
	}
	if _, err := getPost(id); err == nil {
		t.Fatal("post borrado sigue visible")
	}
}

func TestToggleReaction(t *testing.T) {
	env := setupTestDB(t)
	postID, _ := createPost(env.admin.ID, "R", "cuerpo")

	if got := myReaction(postID, env.user.ID); got != "" {
		t.Fatalf("myReaction inicial = %q, quiero vacío", got)
	}
	// Insert like.
	if err := toggleReaction(postID, env.user.ID, "like"); err != nil {
		t.Fatalf("toggle like: %v", err)
	}
	if got := myReaction(postID, env.user.ID); got != "like" {
		t.Fatalf("myReaction = %q, quiero like", got)
	}
	// Toggle off.
	if err := toggleReaction(postID, env.user.ID, "like"); err != nil {
		t.Fatalf("toggle off: %v", err)
	}
	if got := myReaction(postID, env.user.ID); got != "" {
		t.Fatalf("tras toggle off = %q, quiero vacío", got)
	}
	// Cambio like -> dislike (update).
	if err := toggleReaction(postID, env.user.ID, "like"); err != nil {
		t.Fatal(err)
	}
	if err := toggleReaction(postID, env.user.ID, "dislike"); err != nil {
		t.Fatalf("cambio a dislike: %v", err)
	}
	if got := myReaction(postID, env.user.ID); got != "dislike" {
		t.Fatalf("myReaction = %q, quiero dislike", got)
	}
	p, _ := getPost(postID)
	if p.Likes != 0 || p.Dislikes != 1 {
		t.Fatalf("contadores: likes=%d dislikes=%d", p.Likes, p.Dislikes)
	}
}

func TestComments(t *testing.T) {
	env := setupTestDB(t)
	postID, _ := createPost(env.admin.ID, "C", "cuerpo")

	if err := addComment(postID, env.user.ID, "primer comentario"); err != nil {
		t.Fatalf("addComment: %v", err)
	}
	if err := addComment(postID, env.admin.ID, "segundo"); err != nil {
		t.Fatalf("addComment 2: %v", err)
	}
	cs, err := listComments(postID)
	if err != nil || len(cs) != 2 {
		t.Fatalf("listComments: %d, %v", len(cs), err)
	}
	if cs[0].Username != "usuario1" || cs[0].Body != "primer comentario" {
		t.Fatalf("comentario 1: %+v", cs[0])
	}
	p, _ := getPost(postID)
	if p.Comments != 2 {
		t.Fatalf("contador comentarios = %d, quiero 2", p.Comments)
	}

	// Cascada: borrar el post borra comentarios y reacciones.
	_ = toggleReaction(postID, env.user.ID, "like")
	if err := deletePost(postID); err != nil {
		t.Fatalf("deletePost: %v", err)
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM comments`).Scan(&n)
	if n != 0 {
		t.Fatalf("quedaron %d comentarios huérfanos", n)
	}
	_ = db.QueryRow(`SELECT COUNT(*) FROM reactions`).Scan(&n)
	if n != 0 {
		t.Fatalf("quedaron %d reacciones huérfanas", n)
	}
}

func TestThreadsReplies(t *testing.T) {
	env := setupTestDB(t)

	tid, err := createThread(env.user.ID, "Hilo 1", "contenido del hilo")
	if err != nil {
		t.Fatalf("createThread: %v", err)
	}
	th, err := getThread(tid)
	if err != nil || th.Title != "Hilo 1" || th.Username != "usuario1" || th.Replies != 0 {
		t.Fatalf("getThread: %+v, %v", th, err)
	}
	if _, err := getThread(999999); err == nil {
		t.Fatal("getThread inexistente debe fallar")
	}

	// Fijar updated_at viejo para probar el bump de addReply de forma determinista.
	if _, err := db.Exec(`UPDATE threads SET updated_at = '2000-01-01 00:00:00' WHERE id = ?`, tid); err != nil {
		t.Fatal(err)
	}
	if err := addReply(tid, env.admin.ID, "respuesta del admin"); err != nil {
		t.Fatalf("addReply: %v", err)
	}
	th, _ = getThread(tid)
	if th.UpdatedAt == "2000-01-01 00:00:00" {
		t.Fatal("addReply no actualizó updated_at")
	}
	if th.Replies != 1 {
		t.Fatalf("replies = %d, quiero 1", th.Replies)
	}
	rs, err := listReplies(tid)
	if err != nil || len(rs) != 1 || rs[0].Username != "admin" {
		t.Fatalf("listReplies: %+v, %v", rs, err)
	}

	tid2, _ := createThread(env.admin.ID, "Hilo 2", "otro")
	if _, err := db.Exec(`UPDATE threads SET updated_at = '2001-01-01 00:00:00' WHERE id = ?`, tid2); err != nil {
		t.Fatal(err)
	}
	all, err := listThreads(0)
	if err != nil || len(all) != 2 {
		t.Fatalf("listThreads: %d, %v", len(all), err)
	}
	// El hilo con bump (más reciente) va primero.
	if all[0].ID != tid {
		t.Fatalf("orden por actividad: primero %d, quiero %d", all[0].ID, tid)
	}
	lim, _ := listThreads(1)
	if len(lim) != 1 {
		t.Fatalf("listThreads(1) = %d", len(lim))
	}
}
