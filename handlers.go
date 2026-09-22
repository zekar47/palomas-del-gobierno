package main

import (
	"fmt"
	"html"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var uploadsDir = "uploads"

var reStripTags = regexp.MustCompile(`<[^>]+>`)

var funcs = template.FuncMap{
	"markdown": renderMarkdown,
	"excerpt": func(n int, h template.HTML) string {
		s := strings.Join(strings.Fields(strings.TrimSpace(reStripTags.ReplaceAllString(string(h), " "))), " ")
		s = html.UnescapeString(s)
		r := []rune(s)
		if len(r) > n {
			return string(r[:n]) + "…"
		}
		return s
	},
	"timeFmt": func(s string) string {
		t, err := time.Parse("2006-01-02 15:04:05", s)
		if err != nil {
			return s
		}
		return t.UTC().Format("02-01-2006 15:04")
	},
	"eqstr": func(a, b string) bool { return a == b },
}

var pageTemplates = map[string]*template.Template{}

func loadTemplates() error {
	pages := []string{"home", "news", "post", "admin_edit", "forum", "thread", "new_thread", "login", "register", "notfound"}
	for _, p := range pages {
		t, err := template.New("base").Funcs(funcs).ParseFiles("templates/base.html", "templates/"+p+".html")
		if err != nil {
			return err
		}
		pageTemplates[p] = t
	}
	return nil
}

func render(w http.ResponseWriter, page string, data any) {
	t, ok := pageTemplates[page]
	if !ok {
		http.Error(w, "plantilla desconocida: "+page, http.StatusInternalServerError)
		return
	}
	if err := t.ExecuteTemplate(w, "base", data); err != nil {
		log.Printf("ERROR renderizando %s: %v", page, err)
		http.Error(w, "error al renderizar la página", http.StatusInternalServerError)
	}
}

type pageData struct {
	Title string
	User  *User
	CSRF  string
	Flash string
}

func loadPage(r *http.Request, title string) pageData {
	return pageData{Title: title, User: currentUser(r), CSRF: csrfFor(r)}
}

func parseID(s string) int64 {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

func routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", homeHandler)

	mux.HandleFunc("GET /static/", staticHandler)
	mux.HandleFunc("GET /uploads/", uploadsHandler)

	// sesión
	mux.HandleFunc("GET /login", loginGet)
	mux.HandleFunc("POST /login", loginPost)
	mux.HandleFunc("GET /register", registerGet)
	mux.HandleFunc("POST /register", registerPost)
	mux.HandleFunc("POST /logout", requireLogin(requirePostCSRF(logoutPost)))

	// noticias
	mux.HandleFunc("GET /news", newsList)
	mux.HandleFunc("GET /news/feed.xml", newsFeed)
	mux.HandleFunc("GET /news/{id}", newsShow)
	mux.HandleFunc("POST /news/{id}/reaction", requireLogin(requirePostCSRF(newsReaction)))
	mux.HandleFunc("POST /news/{id}/comment", requireLogin(requirePostCSRF(newsComment)))

	// administración
	mux.HandleFunc("GET /admin/new", requireAdmin(adminNewGet))
	mux.HandleFunc("POST /admin/new", requireAdmin(requirePostCSRF(adminNewPost)))
	mux.HandleFunc("GET /admin/edit/{id}", requireAdmin(adminEditGet))
	mux.HandleFunc("POST /admin/edit/{id}", requireAdmin(requirePostCSRF(adminEditPost)))
	mux.HandleFunc("POST /admin/delete/{id}", requireAdmin(requirePostCSRF(adminDelete)))

	// foro
	mux.HandleFunc("GET /forum", forumList)
	mux.HandleFunc("GET /forum/feed.xml", forumFeed)
	mux.HandleFunc("GET /forum/new", requireLogin(forumNewGet))
	mux.HandleFunc("POST /forum/new", requireLogin(requirePostCSRF(forumNewPost)))
	mux.HandleFunc("GET /forum/{id}", forumThread)
	mux.HandleFunc("POST /forum/{id}/reply", requireLogin(requirePostCSRF(forumReply)))

	// vista previa de markdown (para el editor)
	mux.HandleFunc("POST /preview", requireLogin(requirePostCSRF(previewHandler)))

	mux.HandleFunc("/", notFoundHandler)
}

func staticHandler(w http.ResponseWriter, r *http.Request) {
	http.StripPrefix("/static/", http.FileServer(http.Dir("static"))).ServeHTTP(w, r)
}

func uploadsHandler(w http.ResponseWriter, r *http.Request) {
	// solo se sirven archivos de uploads, nunca por debajo
	name := filepath.Base(r.URL.Path)
	if name == "." || name == "/" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, filepath.Join(uploadsDir, name))
}

// ---------------------------------------------------------------------------
// portada
// ---------------------------------------------------------------------------

type member struct {
	Name  string
	Role  string
	Bio   string
	Photo string
}

type homeData struct {
	pageData
	Latest  []*Post
	Threads []*Thread
	Members []member
	Posts   int
	Users   int
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	latest, err := listPosts(3)
	if err != nil {
		latest = nil
	}
	threads, _ := listThreads(4)
	members := []member{
		{Name: "HUGOAT", Role: "guitarra y voz", Bio: "El líder de la banda aunque siempre diga lo contrario.", Photo: "/static/hugo.webp"},
		{Name: "ZEKAR", Role: "bajo y voz", Bio: "Bajo la influencia de alucinógenos piensa que el bajo es un piano.", Photo: "/static/zekar.webp"},
		{Name: "CALEB", Role: "guitarra rítmica", Bio: "No se aprende las rolas... pero le sale chida la carne asada.", Photo: "/static/caleb.webp"},
		{Name: "ELI", Role: "guitarra líder", Bio: "Hizo su propia guitarra porque ninguna era suficiente.", Photo: "/static/eli.png"},
		{Name: "EMMANUEL", Role: "batería", Bio: "Nunca le preguntes qué pasó con la batería del video de N.O.V.A.", Photo: "/static/emmanuel.webp"},
	}
	data := homeData{
		pageData: loadPage(r, "PALOMAS DEL GOBIERNO"),
		Latest:   latest,
		Threads:  threads,
		Members:  members,
	}
	_ = db.QueryRow(`SELECT COUNT(*) FROM posts`).Scan(&data.Posts)
	_ = db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&data.Users)
	render(w, "home", data)
}

// ---------------------------------------------------------------------------
// noticias
// ---------------------------------------------------------------------------

type newsData struct {
	pageData
	Posts []*Post
}

func newsList(w http.ResponseWriter, r *http.Request) {
	posts, err := listPosts(50)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	render(w, "news", newsData{pageData: loadPage(r, "NOTICIAS"), Posts: posts})
}

type postData struct {
	pageData
	Post       *Post
	PostHTML   template.HTML
	Comments   []*Comment
	MyReaction string
}

func newsShow(w http.ResponseWriter, r *http.Request) {
	id := parseID(r.PathValue("id"))
	if id == 0 {
		notFoundHandler(w, r)
		return
	}
	p, err := getPost(id)
	if err != nil {
		notFoundHandler(w, r)
		return
	}
	comments, _ := listComments(id)
	my := ""
	if u := currentUser(r); u != nil {
		my = myReaction(id, u.ID)
	}
	render(w, "post", postData{
		pageData:   loadPage(r, p.Title),
		Post:       p,
		PostHTML:   postHTML(p),
		Comments:   comments,
		MyReaction: my,
	})
}

func newsReaction(w http.ResponseWriter, r *http.Request) {
	id := parseID(r.PathValue("id"))
	kind := r.PostFormValue("reaction")
	if id == 0 || (kind != "like" && kind != "dislike") {
		http.Error(w, "reacción inválida", http.StatusBadRequest)
		return
	}
	if err := toggleReaction(id, currentUser(r).ID, kind); err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/news/%d", id), http.StatusSeeOther)
}

func newsComment(w http.ResponseWriter, r *http.Request) {
	id := parseID(r.PathValue("id"))
	body := strings.TrimSpace(r.PostFormValue("body"))
	if id == 0 {
		http.Error(w, "noticia inválida", http.StatusBadRequest)
		return
	}
	if body == "" {
		http.Error(w, "comentario vacío", http.StatusBadRequest)
		return
	}
	if len([]rune(body)) > 4000 {
		http.Error(w, "comentario demasiado largo (máx 4000 caracteres)", http.StatusBadRequest)
		return
	}
	if err := addComment(id, currentUser(r).ID, body); err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/news/%d", id), http.StatusSeeOther)
}

// ---------------------------------------------------------------------------
// administración
// ---------------------------------------------------------------------------

type adminData struct {
	pageData
	Post   *Post
	IsEdit bool
	Error  string
}

func adminNewGet(w http.ResponseWriter, r *http.Request) {
	render(w, "admin_edit", adminData{
		pageData: loadPage(r, "NUEVA NOTICIA"),
		IsEdit:   false,
	})
}

var imgExts = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true}
var vidExts = map[string]bool{".mp4": true, ".webm": true}

func adminNewPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		render(w, "admin_edit", adminData{pageData: loadPage(r, "NUEVA NOTICIA"), Error: "archivo demasiado grande (máx 40 MB)"})
		return
	}
	title := strings.TrimSpace(r.PostFormValue("title"))
	body := r.PostFormValue("body")
	if title == "" {
		render(w, "admin_edit", adminData{pageData: loadPage(r, "NUEVA NOTICIA"), Error: "la noticia necesita un título"})
		return
	}
	id, err := createPost(currentUser(r).ID, title, body)
	if err != nil {
		http.Error(w, "error al guardar", http.StatusInternalServerError)
		return
	}
	img, err := saveUpload(r, "image", imgExts, 10<<20)
	if err != nil {
		_ = deletePost(id)
		render(w, "admin_edit", adminData{pageData: loadPage(r, "NUEVA NOTICIA"), Error: "imagen: " + err.Error()})
		return
	}
	vid, err := saveUpload(r, "video", vidExts, 25<<20)
	if err != nil {
		removeUploads(img)
		_ = deletePost(id)
		render(w, "admin_edit", adminData{pageData: loadPage(r, "NUEVA NOTICIA"), Error: "video: " + err.Error()})
		return
	}
	_ = updatePost(id, title, body, img, vid)
	http.Redirect(w, r, fmt.Sprintf("/news/%d", id), http.StatusSeeOther)
}

func adminEditGet(w http.ResponseWriter, r *http.Request) {
	id := parseID(r.PathValue("id"))
	p, err := getPost(id)
	if err != nil {
		notFoundHandler(w, r)
		return
	}
	render(w, "admin_edit", adminData{pageData: loadPage(r, "EDITAR NOTICIA"), Post: p, IsEdit: true})
}

func adminEditPost(w http.ResponseWriter, r *http.Request) {
	id := parseID(r.PathValue("id"))
	p, err := getPost(id)
	if err != nil {
		notFoundHandler(w, r)
		return
	}
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		render(w, "admin_edit", adminData{pageData: loadPage(r, "EDITAR NOTICIA"), Post: p, IsEdit: true, Error: "archivo demasiado grande (máx 40 MB)"})
		return
	}
	title := strings.TrimSpace(r.PostFormValue("title"))
	body := r.PostFormValue("body")
	if title == "" {
		render(w, "admin_edit", adminData{pageData: loadPage(r, "EDITAR NOTICIA"), Post: p, IsEdit: true, Error: "la noticia necesita un título"})
		return
	}

	newImage, newVideo := p.ImagePath, p.VideoPath
	if r.PostFormValue("remove_image") == "1" {
		removeUploads(newImage)
		newImage = ""
	}
	if r.PostFormValue("remove_video") == "1" {
		removeUploads(newVideo)
		newVideo = ""
	}
	if img, err := saveUpload(r, "image", imgExts, 10<<20); err != nil {
		render(w, "admin_edit", adminData{pageData: loadPage(r, "EDITAR NOTICIA"), Post: p, IsEdit: true, Error: "imagen: " + err.Error()})
		return
	} else if img != "" {
		removeUploads(newImage)
		newImage = img
	}
	if vid, err := saveUpload(r, "video", vidExts, 25<<20); err != nil {
		removeUploads(newImage)
		render(w, "admin_edit", adminData{pageData: loadPage(r, "EDITAR NOTICIA"), Post: p, IsEdit: true, Error: "video: " + err.Error()})
		return
	} else if vid != "" {
		removeUploads(newVideo)
		newVideo = vid
	}

	if err := updatePost(id, title, body, newImage, newVideo); err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/news/%d", id), http.StatusSeeOther)
}

func adminDelete(w http.ResponseWriter, r *http.Request) {
	id := parseID(r.PathValue("id"))
	p, err := getPost(id)
	if err != nil {
		notFoundHandler(w, r)
		return
	}
	removeUploads(p.ImagePath, p.VideoPath)
	_ = deletePost(id)
	http.Redirect(w, r, "/news", http.StatusSeeOther)
}

func saveUpload(r *http.Request, field string, allowed map[string]bool, maxBytes int64) (string, error) {
	file, header, err := r.FormFile(field)
	if err == http.ErrMissingFile {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer file.Close()
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !allowed[ext] {
		return "", fmt.Errorf("extensión no permitida (%s); usa %s", header.Filename, joinExts(allowed))
	}
	if header.Size > maxBytes {
		return "", fmt.Errorf("archivo demasiado grande (máx %d MB)", maxBytes>>20)
	}
	name := fmt.Sprintf("%s-%s%s", randHex(8), strconv.FormatInt(time.Now().UnixNano(), 36), ext)
	// #nosec G304 -- el nombre es generado por el servidor (rand+timestamp) con
	// extensión de whitelist; el usuario no controla la ruta resultante.
	dst, err := os.Create(filepath.Join(uploadsDir, name))
	if err != nil {
		return "", err
	}
	defer dst.Close()
	if _, err := io.Copy(dst, file); err != nil {
		return "", err
	}
	return name, nil
}

func joinExts(m map[string]bool) string {
	var exts []string
	for e := range m {
		exts = append(exts, e)
	}
	return strings.Join(exts, ", ")
}

func removeUploads(paths ...string) {
	for _, p := range paths {
		if p == "" || strings.Contains(p, "/") || strings.Contains(p, "..") {
			continue
		}
		_ = os.Remove(filepath.Join(uploadsDir, p))
	}
}

// ---------------------------------------------------------------------------
// foro
// ---------------------------------------------------------------------------

type forumData struct {
	pageData
	Threads []*Thread
}

func forumList(w http.ResponseWriter, r *http.Request) {
	threads, err := listThreads(100)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	render(w, "forum", forumData{pageData: loadPage(r, "FORO"), Threads: threads})
}

func forumNewGet(w http.ResponseWriter, r *http.Request) {
	render(w, "new_thread", forumData{pageData: loadPage(r, "NUEVO HILO")})
}

func forumNewPost(w http.ResponseWriter, r *http.Request) {
	title := strings.TrimSpace(r.PostFormValue("title"))
	body := strings.TrimSpace(r.PostFormValue("body"))
	if title == "" {
		pd := loadPage(r, "NUEVO HILO")
		pd.Flash = "el hilo necesita un título"
		render(w, "new_thread", forumData{pageData: pd})
		return
	}
	if body == "" {
		pd := loadPage(r, "NUEVO HILO")
		pd.Flash = "el hilo necesita contenido"
		render(w, "new_thread", forumData{pageData: pd})
		return
	}
	if len([]rune(body)) > 20000 {
		pd := loadPage(r, "NUEVO HILO")
		pd.Flash = "contenido demasiado largo"
		render(w, "new_thread", forumData{pageData: pd})
		return
	}
	id, err := createThread(currentUser(r).ID, title, body)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/forum/%d", id), http.StatusSeeOther)
}

type threadData struct {
	pageData
	Thread  *Thread
	Replies []*Reply
}

func forumThread(w http.ResponseWriter, r *http.Request) {
	id := parseID(r.PathValue("id"))
	t, err := getThread(id)
	if err != nil {
		notFoundHandler(w, r)
		return
	}
	replies, _ := listReplies(id)
	render(w, "thread", threadData{pageData: loadPage(r, t.Title), Thread: t, Replies: replies})
}

func forumReply(w http.ResponseWriter, r *http.Request) {
	id := parseID(r.PathValue("id"))
	body := strings.TrimSpace(r.PostFormValue("body"))
	if id == 0 {
		http.Error(w, "hilo inválido", http.StatusBadRequest)
		return
	}
	if body == "" {
		http.Error(w, "respuesta vacía", http.StatusBadRequest)
		return
	}
	if len([]rune(body)) > 20000 {
		http.Error(w, "respuesta demasiado larga (máx 20000 caracteres)", http.StatusBadRequest)
		return
	}
	if err := addReply(id, currentUser(r).ID, body); err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/forum/%d", id), http.StatusSeeOther)
}

// ---------------------------------------------------------------------------
// sesión
// ---------------------------------------------------------------------------

type authData struct {
	pageData
	Error string
	Next  string
}

func loginGet(w http.ResponseWriter, r *http.Request) {
	render(w, "login", authData{
		pageData: loadPage(r, "ACCESO"),
		Next:     r.URL.Query().Get("next"),
	})
}

func loginPost(w http.ResponseWriter, r *http.Request) {
	u, err := loginUser(r.PostFormValue("username"), r.PostFormValue("password"))
	if err != nil {
		render(w, "login", authData{
			pageData: loadPage(r, "ACCESO"),
			Error:    "usuario o contraseña incorrectos",
			Next:     r.PostFormValue("next"),
		})
		return
	}
	createSession(w, u.ID)
	http.Redirect(w, r, safeNext(r.PostFormValue("next")), http.StatusSeeOther)
}

func registerGet(w http.ResponseWriter, r *http.Request) {
	render(w, "register", authData{pageData: loadPage(r, "ALISTAMIENTO")})
}

func registerPost(w http.ResponseWriter, r *http.Request) {
	u, err := registerUser(r.PostFormValue("username"), r.PostFormValue("password"))
	if err != nil {
		render(w, "register", authData{pageData: loadPage(r, "ALISTAMIENTO"), Error: err.Error()})
		return
	}
	createSession(w, u.ID)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func logoutPost(w http.ResponseWriter, r *http.Request) {
	destroySession(w, r)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func safeNext(s string) string {
	// Solo rutas absolutas internas. Se rechaza "//..." porque los
	// navegadores lo interpretan como URL protocol-relative (open redirect).
	if s == "" || !strings.HasPrefix(s, "/") || strings.HasPrefix(s, "//") {
		return "/"
	}
	return s
}

// ---------------------------------------------------------------------------
// preview de markdown (usa un poco de JS, ver static/app.js)
// ---------------------------------------------------------------------------

func previewHandler(w http.ResponseWriter, r *http.Request) {
	body := r.PostFormValue("body")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if _, err := io.WriteString(w, string(renderMarkdown(body))); err != nil {
		log.Printf("ERROR escribiendo preview: %v", err)
	}
}

func notFoundHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	render(w, "notfound", loadPage(r, "404 — PALOMA EXTRAVIADA"))
}
