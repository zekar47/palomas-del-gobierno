package main

import (
	"crypto/tls"
	"encoding/xml"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRfc3339(t *testing.T) {
	if got := rfc3339("2026-09-22 10:30:00"); got != "2026-09-22T10:30:00Z" {
		t.Fatalf("rfc3339 = %q", got)
	}
	// Entrada inválida => fallback a "ahora" en RFC3339 válido.
	got := rfc3339("basura")
	if len(got) < 20 || !strings.Contains(got, "T") {
		t.Fatalf("rfc3339 fallback = %q", got)
	}
}

func testReq(host string, tlsOn bool) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Host = host
	if tlsOn {
		r.TLS = &tls.ConnectionState{}
	}
	return r
}

func TestAbsURL(t *testing.T) {
	if got := absURL(testReq("ejemplo.test", false), "/news/1"); got != "http://ejemplo.test/news/1" {
		t.Fatalf("absURL http = %q", got)
	}
	if got := absURL(testReq("ejemplo.test", true), "/x"); got != "https://ejemplo.test/x" {
		t.Fatalf("absURL https = %q", got)
	}
	// Tras el terminador TLS se respeta X-Forwarded-Proto.
	proxied := testReq("tienda.ejemplo", false)
	proxied.Header.Set("X-Forwarded-Proto", "https")
	if got := absURL(proxied, "/news/feed.xml"); got != "https://tienda.ejemplo/news/feed.xml" {
		t.Fatalf("absURL proxy = %q", got)
	}
}

func TestTagURI(t *testing.T) {
	if got := tagURI(testReq("localhost:8080", false), "post", 3); got != "tag:palomasdelgobierno.local,2026:post/3" {
		t.Fatalf("tagURI localhost = %q", got)
	}
	if got := tagURI(testReq("44.218.216.230:8080", false), "thread", 7); got != "tag:44.218.216.230:8080,2026:thread/7" {
		t.Fatalf("tagURI ip = %q", got)
	}
}

func TestFeedHead(t *testing.T) {
	f := feedHead(testReq("ejemplo.test", false), "/news/feed.xml", "Título")
	if f.Xmlns == "" || len(f.Link) != 2 || f.Link[0].Href != "http://ejemplo.test/news/feed.xml" {
		t.Fatalf("feedHead: %+v", f)
	}
}

func TestNewsFeed(t *testing.T) {
	env := setupTestDB(t)
	pid, _ := createPost(env.admin.ID, "Noticia feed", "cuerpo **x**")
	_ = updatePost(pid, "Noticia feed", "cuerpo **x**", "img.png", "vid.mp4")

	rec := httptest.NewRecorder()
	newsFeed(rec, testReq("ejemplo.test", false))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/atom+xml; charset=utf-8" {
		t.Fatalf("content-type = %q", ct)
	}
	var f atomFeed
	if err := xml.Unmarshal(rec.Body.Bytes(), &f); err != nil {
		t.Fatalf("XML inválido: %v", err)
	}
	if len(f.Entries) != 1 || f.Entries[0].Title != "Noticia feed" {
		t.Fatalf("entries: %+v", f.Entries)
	}
	e := f.Entries[0]
	if e.Content.Type != "html" || !strings.Contains(e.Content.Body, "<strong>x</strong>") {
		t.Fatalf("contenido: %+v", e.Content)
	}
	if !strings.Contains(e.Content.Body, "/uploads/img.png") || !strings.Contains(e.Content.Body, "/uploads/vid.mp4") {
		t.Fatalf("media ausente: %s", e.Content.Body)
	}
}

func TestNewsFeedVacio(t *testing.T) {
	setupTestDB(t)
	rec := httptest.NewRecorder()
	newsFeed(rec, testReq("ejemplo.test", false))
	var f atomFeed
	if err := xml.Unmarshal(rec.Body.Bytes(), &f); err != nil {
		t.Fatalf("XML inválido: %v", err)
	}
	if len(f.Entries) != 0 {
		t.Fatalf("entries = %d, quiero 0", len(f.Entries))
	}
}

func TestForumFeed(t *testing.T) {
	env := setupTestDB(t)
	tid, _ := createThread(env.user.ID, "Hilo feed", "pregunta **importante**")
	_ = addReply(tid, env.admin.ID, "respuesta *útil*")

	rec := httptest.NewRecorder()
	forumFeed(rec, testReq("ejemplo.test", false))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	var f atomFeed
	if err := xml.Unmarshal(rec.Body.Bytes(), &f); err != nil {
		t.Fatalf("XML inválido: %v", err)
	}
	if len(f.Entries) != 1 {
		t.Fatalf("entries = %d", len(f.Entries))
	}
	body := f.Entries[0].Content.Body
	if !strings.Contains(body, "<strong>importante</strong>") || !strings.Contains(body, "<em>útil</em>") {
		t.Fatalf("cuerpo incompleto: %s", body)
	}
	if !strings.Contains(body, "usuario1") || !strings.Contains(body, "admin") {
		t.Fatalf("autores ausentes: %s", body)
	}
}

func TestWriteAtomError(t *testing.T) {
	w := &failingWriter{header: http.Header{}}
	writeAtom(w, feedHead(testReq("x", false), "/", "t")) // no debe tronar
}

type failingWriter struct{ header http.Header }

func (f *failingWriter) Header() http.Header       { return f.header }
func (f *failingWriter) Write([]byte) (int, error) { return 0, errTestBoom }
func (f *failingWriter) WriteHeader(int)           {}

var errTestBoom = errors.New("boom")
