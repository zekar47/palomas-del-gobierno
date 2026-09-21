package main

import (
	"database/sql"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

var db *sql.DB

const schema = `
CREATE TABLE IF NOT EXISTS users (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  username      TEXT UNIQUE NOT NULL COLLATE NOCASE,
  password_hash TEXT NOT NULL,
  role          TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('admin','user')),
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
	_, err := db.Exec(schema)
	return err
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
