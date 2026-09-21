package main

import (
	"encoding/xml"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Atom 1.0 — feeds con el contenido COMPLETO de todos los posts.
// ---------------------------------------------------------------------------

type atomFeed struct {
	XMLName  xml.Name    `xml:"feed"`
	Xmlns    string      `xml:"xmlns,attr"`
	ID       string      `xml:"id"`
	Title    string      `xml:"title"`
	Subtitle string      `xml:"subtitle,omitempty"`
	Updated  string      `xml:"updated"`
	Link     []atomLink  `xml:"link"`
	Entries  []atomEntry `xml:"entry"`
}

type atomLink struct {
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr,omitempty"`
	Href string `xml:"href,attr"`
}

type atomEntry struct {
	ID        string      `xml:"id"`
	Title     string      `xml:"title"`
	Updated   string      `xml:"updated"`
	Published string      `xml:"published"`
	Link      []atomLink  `xml:"link"`
	Author    atomAuthor  `xml:"author"`
	Content   atomContent `xml:"content"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

type atomContent struct {
	Type string `xml:"type,attr"`
	Body string `xml:",chardata"`
}

func rfc3339(sqliteTime string) string {
	t, err := time.Parse("2006-01-02 15:04:05", sqliteTime)
	if err != nil {
		return time.Now().UTC().Format(time.RFC3339)
	}
	return t.UTC().Format(time.RFC3339)
}

func absURL(r *http.Request, path string) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host + path
}

func tagURI(r *http.Request, kind string, id int64) string {
	host := r.Host
	if strings.HasPrefix(host, "localhost") {
		host = "palomasdelgobierno.local"
	}
	return fmt.Sprintf("tag:%s,2026:%s/%d", host, kind, id)
}

func feedHead(r *http.Request, selfPath, title string) atomFeed {
	return atomFeed{
		Xmlns:    "http://www.w3.org/2005/Atom",
		ID:       tagURI(r, "feed", 0),
		Title:    title,
		Subtitle: "Palomas del Gobierno — canal de difusión subterránea",
		Updated:  time.Now().UTC().Format(time.RFC3339),
		Link: []atomLink{
			{Rel: "self", Type: "application/atom+xml", Href: absURL(r, selfPath)},
			{Rel: "alternate", Type: "text/html", Href: absURL(r, "/")},
		},
	}
}

func writeAtom(w http.ResponseWriter, f atomFeed) {
	w.Header().Set("Content-Type", "application/atom+xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if err := enc.Encode(f); err != nil {
		return
	}
}

// newsFeed genera el feed atom de noticias con el contenido íntegro.
func newsFeed(w http.ResponseWriter, r *http.Request) {
	posts, err := listPosts(100)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	f := feedHead(r, "/news/feed.xml", "Palomas del Gobierno — noticias")
	for _, p := range posts {
		f.Entries = append(f.Entries, atomEntry{
			ID:        tagURI(r, "post", p.ID),
			Title:     p.Title,
			Updated:   rfc3339(p.UpdatedAt),
			Published: rfc3339(p.CreatedAt),
			Link: []atomLink{
				{Rel: "alternate", Type: "text/html", Href: absURL(r, fmt.Sprintf("/news/%d", p.ID))},
			},
			Author: atomAuthor{Name: p.Author},
			Content: atomContent{
				Type: "html",
				Body: html.UnescapeString(string(postHTML(p))),
			},
		})
	}
	writeAtom(w, f)
}

// forumFeed genera el feed atom del foro con el contenido completo de los
// hilos (incluye todas las respuestas de cada hilo).
func forumFeed(w http.ResponseWriter, r *http.Request) {
	threads, err := listThreads(100)
	if err != nil {
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	}
	f := feedHead(r, "/forum/feed.xml", "Palomas del Gobierno — foro")
	for _, t := range threads {
		replies, _ := listReplies(t.ID)
		var body strings.Builder
		body.WriteString(`<div class="forum-post"><p>por <strong>`)
		body.WriteString(html.EscapeString(t.Username))
		body.WriteString("</strong></p>")
		body.WriteString(renderMarkdownString(t.Body))
		body.WriteString("</div>")
		for _, rp := range replies {
			body.WriteString(`<div class="forum-reply"><p>respuesta de <strong>`)
			body.WriteString(html.EscapeString(rp.Username))
			body.WriteString("</strong>:</p>")
			body.WriteString(renderMarkdownString(rp.Body))
			body.WriteString("</div>")
		}
		f.Entries = append(f.Entries, atomEntry{
			ID:        tagURI(r, "thread", t.ID),
			Title:     t.Title,
			Updated:   rfc3339(t.UpdatedAt),
			Published: rfc3339(t.CreatedAt),
			Link: []atomLink{
				{Rel: "alternate", Type: "text/html", Href: absURL(r, fmt.Sprintf("/forum/%d", t.ID))},
			},
			Author: atomAuthor{Name: t.Username},
			Content: atomContent{
				Type: "html",
				Body: body.String(),
			},
		})
	}
	writeAtom(w, f)
}
