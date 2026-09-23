package main

import (
	"bytes"
	"html"
	"html/template"
	"net/url"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
)

var mdRenderer = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithParserOptions(
		parser.WithAutoHeadingID(),
	),
	// sin html.WithUnsafe(): el HTML crudo del markdown se escapa (seguro)
)

// renderMarkdown convierte texto markdown a HTML.
func renderMarkdown(src string) template.HTML {
	var buf bytes.Buffer
	if err := mdRenderer.Convert([]byte(src), &buf); err != nil {
		return template.HTML("<p>[error al renderizar markdown]</p>")
	}
	// #nosec G203 -- el renderer Goldmark se configuró SIN html.WithUnsafe,
	// así que el HTML crudo del input se escapa en vez de inyectarse.
	// embedYouTubeLinks solo genera iframes hacia youtube-nocookie con IDs
	// validados (11 chars alfanuméricos), nunca con input crudo.
	return template.HTML(embedYouTubeLinks(buf.String()))
}

// reAnchor localiza enlaces ya renderizados para evaluar embeds.
var reAnchor = regexp.MustCompile(`<a href="(https?://[^"]+)"[^>]*>.*?</a>`)

// ytHosts son los únicos hosts que pueden terminar en iframe.
var ytHosts = map[string]bool{
	"youtube.com": true, "www.youtube.com": true,
	"m.youtube.com": true, "music.youtube.com": true,
	"youtu.be": true, "www.youtube-nocookie.com": true,
}

// reYTID valida IDs de video (siempre 11 chars [A-Za-z0-9_-]).
var reYTID = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

// youtubeID extrae el ID de video o "" si no es una URL válida de YouTube.
func youtubeID(rawurl string) string {
	u, err := url.Parse(html.UnescapeString(rawurl))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	if !ytHosts[strings.ToLower(u.Hostname())] {
		return ""
	}
	var id string
	switch {
	case strings.HasSuffix(strings.ToLower(u.Hostname()), "youtu.be"):
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) == 1 {
			id = parts[0]
		}
	case strings.HasPrefix(u.Path, "/embed/"),
		strings.HasPrefix(u.Path, "/shorts/"),
		strings.HasPrefix(u.Path, "/live/"):
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) >= 2 {
			id = parts[1]
		}
	case strings.HasPrefix(u.Path, "/watch"):
		id = u.Query().Get("v")
	}
	if !reYTID.MatchString(id) {
		return ""
	}
	return id
}

// youtubeEmbed arma el reproductor. El href original se conserva como
// fallback para navegadores de texto y noscript.
func youtubeEmbed(id, origHref string) string {
	return `<div class="video-embed"><iframe src="https://www.youtube-nocookie.com/embed/` +
		id + `" title="video de YouTube" loading="lazy" allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture; web-share" allowfullscreen></iframe>` +
		`<p class="dim"><a href="` + origHref + `">ver en YouTube</a></p></div>`
}

// embedYouTubeLinks sustituye enlaces a YouTube por su reproductor.
func embedYouTubeLinks(h string) string {
	return reAnchor.ReplaceAllStringFunc(h, func(a string) string {
		m := reAnchor.FindStringSubmatch(a)
		if len(m) < 2 {
			return a
		}
		if id := youtubeID(m[1]); id != "" {
			return youtubeEmbed(id, m[1])
		}
		return a
	})
}

// renderMarkdownString igual, pero devuelve string (para los feeds).
func renderMarkdownString(src string) string {
	return string(renderMarkdown(src))
}

// postHTML arma el contenido completo de una noticia (imagen + cuerpo + video).
func postHTML(p *Post) template.HTML {
	var b bytes.Buffer
	if p.ImagePath != "" {
		b.WriteString(`<figure class="post-media"><img src="/uploads/`)
		b.WriteString(htmlEscape(p.ImagePath))
		b.WriteString(`" alt="imagen de la noticia"></figure>`)
	}
	b.WriteString(string(renderMarkdown(p.Body)))
	if p.VideoPath != "" {
		b.WriteString(`<figure class="post-media"><video controls preload="metadata" src="/uploads/`)
		b.WriteString(htmlEscape(p.VideoPath))
		b.WriteString(`"></video><figcaption><a href="/uploads/`)
		b.WriteString(htmlEscape(p.VideoPath))
		b.WriteString(`">descargar video</a></figcaption></figure>`)
	}
	// #nosec G203 -- paths escapados con htmlEscape; cuerpo vía renderMarkdown
	// (Goldmark sin unsafe). El HTML compuesto es seguro por construcción.
	return template.HTML(b.String())
}

func htmlEscape(s string) string {
	return template.HTMLEscapeString(s)
}
