package main

import (
	"bytes"
	"html/template"

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
	return template.HTML(buf.String())
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
	return template.HTML(b.String())
}

func htmlEscape(s string) string {
	return template.HTMLEscapeString(s)
}
