package main

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

var db *sql.DB

const schema = `
CREATE TABLE IF NOT EXISTS users (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  username      TEXT UNIQUE NOT NULL COLLATE NOCASE,
  password_hash TEXT NOT NULL,
  role          TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('admin','member','user')),
  created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS posts (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  author_id  INTEGER NOT NULL REFERENCES users(id),
  title      TEXT NOT NULL,
  body       TEXT NOT NULL DEFAULT '',
  image_path TEXT NOT NULL DEFAULT '',
  video_path TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS comments (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  post_id    INTEGER NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
  user_id    INTEGER NOT NULL REFERENCES users(id),
  body       TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS reactions (
  post_id INTEGER NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
  user_id INTEGER NOT NULL REFERENCES users(id),
  kind    TEXT NOT NULL CHECK (kind IN ('like','dislike')),
  PRIMARY KEY (post_id, user_id)
);

CREATE TABLE IF NOT EXISTS threads (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id),
  title      TEXT NOT NULL,
  body       TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS replies (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  thread_id  INTEGER NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
  user_id    INTEGER NOT NULL REFERENCES users(id),
  body       TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_posts_created   ON posts(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_comments_post   ON comments(post_id);
CREATE INDEX IF NOT EXISTS idx_threads_updated ON threads(updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_replies_thread  ON replies(thread_id);
`

func migrate() error {
	// Conexión dedicada: las PRAGMA de claves foráneas son por conexión y la
	// migración de roles necesita apagarlas durante la reconstrucción.
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, schema); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS meta(k TEXT PRIMARY KEY, v TEXT NOT NULL)`); err != nil {
		return err
	}
	version := 0
	var vstr string
	err = conn.QueryRowContext(ctx, `SELECT v FROM meta WHERE k = 'schema_version'`).Scan(&vstr)
	switch {
	case err == nil:
		fmt.Sscanf(vstr, "%d", &version)
	case err == sql.ErrNoRows:
		version = 0
	default:
		return err
	}
	if version < 2 {
		if err := migrateTo2(ctx, conn); err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO meta(k, v) VALUES ('schema_version', '2') ON CONFLICT(k) DO UPDATE SET v = '2'`); err != nil {
			return err
		}
	}
	return nil
}

// migrateTo2 amplía el CHECK de users.role a ('admin','member','user').
// SQLite no permite ALTER CHECK: se reconstruye la tabla en una transacción
// con las FK apagadas (solo en esta conexión) y se verifica integridad al
// final. Es idempotente vía meta.schema_version.
func migrateTo2(ctx context.Context, conn *sql.Conn) error {
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys=OFF`); err != nil {
		return err
	}
	defer conn.ExecContext(ctx, `PRAGMA foreign_keys=ON`)

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmts := []string{
		`CREATE TABLE users_new (
		   id            INTEGER PRIMARY KEY AUTOINCREMENT,
		   username      TEXT UNIQUE NOT NULL COLLATE NOCASE,
		   password_hash TEXT NOT NULL,
		   role          TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('admin','member','user')),
		   created_at    TEXT NOT NULL DEFAULT (datetime('now'))
		 )`,
		`INSERT INTO users_new (id, username, password_hash, role, created_at)
		   SELECT id, username, password_hash, role, created_at FROM users`,
		`DROP TABLE users`,
		`ALTER TABLE users_new RENAME TO users`,
	}
	for _, s := range stmts {
		if _, err := tx.ExecContext(ctx, s); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	var bad int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_foreign_key_check`).Scan(&bad); err != nil {
		return err
	}
	if bad > 0 {
		return fmt.Errorf("integridad rota tras migración: %d filas huérfanas", bad)
	}
	return nil
}

// seedAdmin crea el usuario administrador inicial si no existe ninguno.
// Usa la contraseña dada o, si viene vacía, "admin123".
func seedAdmin(pass string) error {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	if pass == "" {
		pass = "admin123"
		fmt.Println("  ⚠  no hay usuarios: creando administrador 'admin' / 'admin123'")
		fmt.Println("  ⚠  ¡cámbiala pronto!  usa: PALOMAS_ADMIN_PASSWORD=... ./palomas")
	}
	h, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO users (username, password_hash, role) VALUES (?, ?, 'admin')`, "admin", string(h))
	return err
}

// ---------------------------------------------------------------------------
// tipos de datos
// ---------------------------------------------------------------------------

type User struct {
	ID        int64
	Username  string
	Role      string
	CreatedAt string
}

type Post struct {
	ID         int64
	AuthorID   int64
	Author     string
	Title      string
	Body       string
	ImagePath  string
	VideoPath  string
	CreatedAt  string
	UpdatedAt  string
	Likes      int
	Dislikes   int
	Comments   int
	MyReaction string
}

type Comment struct {
	ID        int64
	PostID    int64
	UserID    int64
	Username  string
	Body      string
	CreatedAt string
}

type Thread struct {
	ID        int64
	UserID    int64
	Username  string
	Title     string
	Body      string
	CreatedAt string
	UpdatedAt string
	Replies   int
}

type Reply struct {
	ID        int64
	ThreadID  int64
	UserID    int64
	Username  string
	Body      string
	CreatedAt string
}

func getUser(id int64) (*User, error) {
	u := &User{}
	err := db.QueryRow(
		`SELECT id, username, role, created_at FROM users WHERE id = ?`, id).
		Scan(&u.ID, &u.Username, &u.Role, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func getUserByName(name string) (*User, error) {
	u := &User{}
	err := db.QueryRow(
		`SELECT id, username, role, created_at FROM users WHERE username = ?`, name).
		Scan(&u.ID, &u.Username, &u.Role, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (u *User) isAdmin() bool { return u != nil && u.Role == "admin" }

// IsAdmin es la versión exportada para plantillas.
func (u *User) IsAdmin() bool { return u.isAdmin() }

// canPostNews indica si puede gestionar noticias (admin o miembro de la banda).
func (u *User) canPostNews() bool {
	return u != nil && (u.Role == "admin" || u.Role == "member")
}

// CanPostNews es la versión exportada para plantillas.
func (u *User) CanPostNews() bool { return u.canPostNews() }

// canEdit indica si puede editar/borrar contenido ajeno-propio: autor o admin.
func canEdit(u *User, authorID int64) bool {
	return u != nil && (u.isAdmin() || u.ID == authorID)
}

func listUsers() ([]*User, error) {
	rows, err := db.Query(`SELECT id, username, role, created_at FROM users ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*User
	for rows.Next() {
		u := &User{}
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func countAdmins() (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE role = 'admin'`).Scan(&n)
	return n, err
}

// setUserRole cambia entre 'user' y 'member'. Promover a admin no está
// soportado por UI (evita escaladas accidentales) y nunca deja cero admins.
func setUserRole(id int64, role string) error {
	if role != "user" && role != "member" {
		return fmt.Errorf("rol inválido: %s", role)
	}
	u, err := getUser(id)
	if err != nil {
		return err
	}
	if u.Role == "admin" {
		n, err := countAdmins()
		if err != nil {
			return err
		}
		if n <= 1 {
			return fmt.Errorf("no se puede degradar al último admin")
		}
	}
	_, err = db.Exec(`UPDATE users SET role = ? WHERE id = ?`, role, id)
	return err
}

func updateUsername(id int64, name string) error {
	name = strings.TrimSpace(name)
	if len(name) < 3 || len(name) > 24 {
		return fmt.Errorf("el nombre de usuario debe tener entre 3 y 24 caracteres")
	}
	if existing, err := getUserByName(name); err == nil && existing.ID != id {
		return fmt.Errorf("ese nombre de usuario ya está ocupado")
	}
	_, err := db.Exec(`UPDATE users SET username = ? WHERE id = ?`, name, id)
	return err
}

func verifyPassword(id int64, password string) bool {
	var hash string
	if err := db.QueryRow(`SELECT password_hash FROM users WHERE id = ?`, id).Scan(&hash); err != nil {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func setPassword(id int64, password string) error {
	if len(password) < 6 {
		return fmt.Errorf("la contraseña debe tener al menos 6 caracteres")
	}
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = db.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, string(h), id)
	return err
}

// deleteUser borra la cuenta y TODO su contenido (posts con sus archivos,
// comentarios, hilos, respuestas, reacciones). Devuelve las rutas de archivos
// para que el llamador las borre del disco. Nunca deja cero admins.
func deleteUser(id int64) ([]string, error) {
	u, err := getUser(id)
	if err != nil {
		return nil, err
	}
	if u.Role == "admin" {
		n, err := countAdmins()
		if err != nil {
			return nil, err
		}
		if n <= 1 {
			return nil, fmt.Errorf("no se puede eliminar al último admin")
		}
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var paths []string
	rows, err := tx.Query(`SELECT image_path, video_path FROM posts WHERE author_id = ?`, id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var img, vid string
		if err := rows.Scan(&img, &vid); err != nil {
			rows.Close()
			return nil, err
		}
		paths = append(paths, img, vid)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, q := range []string{
		`DELETE FROM reactions WHERE user_id = ?`,
		`DELETE FROM comments WHERE user_id = ?`,
		`DELETE FROM replies WHERE user_id = ?`,
		`DELETE FROM threads WHERE user_id = ?`,
		`DELETE FROM posts WHERE author_id = ?`,
		`DELETE FROM users WHERE id = ?`,
	} {
		if _, err := tx.Exec(q, id); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	var out []string
	for _, p := range paths {
		if p != "" {
			out = append(out, p)
		}
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// noticias
// ---------------------------------------------------------------------------

const postSelect = `
SELECT p.id, p.author_id, COALESCE(u.username,''), p.title, p.body,
       p.image_path, p.video_path, p.created_at, p.updated_at,
       (SELECT COUNT(*) FROM reactions r WHERE r.post_id = p.id AND r.kind='like'),
       (SELECT COUNT(*) FROM reactions r WHERE r.post_id = p.id AND r.kind='dislike'),
       (SELECT COUNT(*) FROM comments c WHERE c.post_id = p.id)
FROM posts p JOIN users u ON u.id = p.author_id`

func scanPost(rows interface{ Scan(...any) error }) (*Post, error) {
	p := &Post{}
	err := rows.Scan(&p.ID, &p.AuthorID, &p.Author, &p.Title, &p.Body,
		&p.ImagePath, &p.VideoPath, &p.CreatedAt, &p.UpdatedAt,
		&p.Likes, &p.Dislikes, &p.Comments)
	return p, err
}

func listPosts(limit int) ([]*Post, error) {
	q := postSelect + ` ORDER BY p.created_at DESC`
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Post
	for rows.Next() {
		p, err := scanPost(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func getPost(id int64) (*Post, error) {
	return scanPost(db.QueryRow(postSelect+` WHERE p.id = ?`, id))
}

func getComment(id int64) (*Comment, error) {
	c := &Comment{}
	err := db.QueryRow(`
SELECT c.id, c.post_id, c.user_id, COALESCE(u.username,''), c.body, c.created_at
FROM comments c JOIN users u ON u.id = c.user_id
WHERE c.id = ?`, id).
		Scan(&c.ID, &c.PostID, &c.UserID, &c.Username, &c.Body, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func updateComment(id int64, body string) error {
	_, err := db.Exec(`UPDATE comments SET body = ? WHERE id = ?`, body, id)
	return err
}

func deleteComment(id int64) error {
	_, err := db.Exec(`DELETE FROM comments WHERE id = ?`, id)
	return err
}

func createPost(authorID int64, title, body string) (int64, error) {
	res, err := db.Exec(
		`INSERT INTO posts (author_id, title, body) VALUES (?, ?, ?)`,
		authorID, title, body)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func updatePost(id int64, title, body, imagePath, videoPath string) error {
	_, err := db.Exec(
		`UPDATE posts SET title = ?, body = ?, image_path = ?, video_path = ?,
		        updated_at = datetime('now') WHERE id = ?`,
		title, body, imagePath, videoPath, id)
	return err
}

func deletePost(id int64) error {
	_, err := db.Exec(`DELETE FROM posts WHERE id = ?`, id)
	return err
}

// ---------------------------------------------------------------------------
// reacciones y comentarios
// ---------------------------------------------------------------------------

func myReaction(postID, userID int64) string {
	var kind string
	err := db.QueryRow(
		`SELECT kind FROM reactions WHERE post_id = ? AND user_id = ?`,
		postID, userID).Scan(&kind)
	if err != nil {
		return ""
	}
	return kind
}

func toggleReaction(postID, userID int64, kind string) error {
	var cur string
	err := db.QueryRow(
		`SELECT kind FROM reactions WHERE post_id = ? AND user_id = ?`,
		postID, userID).Scan(&cur)
	switch {
	case err == sql.ErrNoRows:
		_, err = db.Exec(
			`INSERT INTO reactions (post_id, user_id, kind) VALUES (?, ?, ?)`,
			postID, userID, kind)
	case cur == kind:
		_, err = db.Exec(
			`DELETE FROM reactions WHERE post_id = ? AND user_id = ?`, postID, userID)
	default:
		_, err = db.Exec(
			`UPDATE reactions SET kind = ? WHERE post_id = ? AND user_id = ?`,
			kind, postID, userID)
	}
	return err
}

func addComment(postID, userID int64, body string) error {
	_, err := db.Exec(
		`INSERT INTO comments (post_id, user_id, body) VALUES (?, ?, ?)`,
		postID, userID, body)
	return err
}

func listComments(postID int64) ([]*Comment, error) {
	rows, err := db.Query(`
SELECT c.id, c.post_id, c.user_id, COALESCE(u.username,''), c.body, c.created_at
FROM comments c JOIN users u ON u.id = c.user_id
WHERE c.post_id = ? ORDER BY c.created_at ASC`, postID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Comment
	for rows.Next() {
		c := &Comment{}
		if err := rows.Scan(&c.ID, &c.PostID, &c.UserID, &c.Username, &c.Body, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// foro
// ---------------------------------------------------------------------------

const threadSelect = `
SELECT t.id, t.user_id, COALESCE(u.username,''), t.title, t.body, t.created_at, t.updated_at,
       (SELECT COUNT(*) FROM replies r WHERE r.thread_id = t.id)
FROM threads t JOIN users u ON u.id = t.user_id`

func listThreads(limit int) ([]*Thread, error) {
	q := threadSelect + ` ORDER BY t.updated_at DESC`
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Thread
	for rows.Next() {
		t := &Thread{}
		if err := rows.Scan(&t.ID, &t.UserID, &t.Username, &t.Title, &t.Body,
			&t.CreatedAt, &t.UpdatedAt, &t.Replies); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func getThread(id int64) (*Thread, error) {
	t := &Thread{}
	err := db.QueryRow(threadSelect+` WHERE t.id = ?`, id).
		Scan(&t.ID, &t.UserID, &t.Username, &t.Title, &t.Body,
			&t.CreatedAt, &t.UpdatedAt, &t.Replies)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func updateThread(id int64, title, body string) error {
	_, err := db.Exec(
		`UPDATE threads SET title = ?, body = ?, updated_at = datetime('now') WHERE id = ?`,
		title, body, id)
	return err
}

func deleteThread(id int64) error {
	_, err := db.Exec(`DELETE FROM threads WHERE id = ?`, id)
	return err
}

func createThread(userID int64, title, body string) (int64, error) {
	res, err := db.Exec(
		`INSERT INTO threads (user_id, title, body) VALUES (?, ?, ?)`,
		userID, title, body)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func addReply(threadID, userID int64, body string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`INSERT INTO replies (thread_id, user_id, body) VALUES (?, ?, ?)`,
		threadID, userID, body); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`UPDATE threads SET updated_at = datetime('now') WHERE id = ?`, threadID); err != nil {
		return err
	}
	return tx.Commit()
}

func listReplies(threadID int64) ([]*Reply, error) {
	rows, err := db.Query(`
SELECT r.id, r.thread_id, r.user_id, COALESCE(u.username,''), r.body, r.created_at
FROM replies r JOIN users u ON u.id = r.user_id
WHERE r.thread_id = ? ORDER BY r.created_at ASC`, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Reply
	for rows.Next() {
		r := &Reply{}
		if err := rows.Scan(&r.ID, &r.ThreadID, &r.UserID, &r.Username, &r.Body, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
