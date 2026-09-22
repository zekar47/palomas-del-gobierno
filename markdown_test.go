package main

import (
	"html/template"
	"strings"
	"testing"
)

func TestRenderMarkdown(t *testing.T) {
	html := string(renderMarkdown("# Hola\n\n**fuerte** y `código`"))
	if !strings.Contains(html, "<h1") || !strings.Contains(html, "<strong>fuerte</strong>") {
		t.Fatalf("markdown básico: %s", html)
	}
	// GFM: tablas y tachado.
	gfm := string(renderMarkdown("| a | b |\n|---|---|\n| 1 | 2 |\n\n~~tacha~~"))
	if !strings.Contains(gfm, "<table") || !strings.Contains(gfm, "<del>tacha</del>") {
		t.Fatalf("GFM: %s", gfm)
	}
	// AutoHeadingID.
	if !strings.Contains(html, `id="hola"`) {
		t.Fatalf("sin heading id: %s", html)
	}
}

func TestRenderMarkdownEscapaHTML(t *testing.T) {
	payloads := []string{
		`<script>alert(1)</script>`,
		`"><img src=x onerror=alert(1)>`,
		`<iframe src="https://evil.test"></iframe>`,
		`<svg onload=alert(1)>`,
	}
	for _, p := range payloads {
		out := string(renderMarkdown(p))
		if strings.Contains(out, "<script>alert(1)</script>") ||
			strings.Contains(out, "<iframe") ||
			strings.Contains(out, "<svg onload") ||
			strings.Contains(out, "onerror=alert(1)>") {
			t.Fatalf("HTML crudo no escapado para %q: %s", p, out)
		}
	}
}

func TestRenderMarkdownString(t *testing.T) {
	if s := renderMarkdownString("hola"); !strings.Contains(s, "hola") {
		t.Fatalf("renderMarkdownString: %q", s)
	}
}

func TestPostHTML(t *testing.T) {
	// Solo cuerpo.
	p := &Post{Body: "cuerpo *énfasis*"}
	out := string(postHTML(p))
	if !strings.Contains(out, "<em>énfasis</em>") {
		t.Fatalf("postHTML cuerpo: %s", out)
	}
	// Con imagen y video.
	p = &Post{Body: "x", ImagePath: "a.png", VideoPath: "b.mp4"}
	out = string(postHTML(p))
	if !strings.Contains(out, `<img src="/uploads/a.png"`) ||
		!strings.Contains(out, `<video controls preload="metadata" src="/uploads/b.mp4"`) {
		t.Fatalf("postHTML media: %s", out)
	}
	// Path malicioso queda escapado, nunca rompe el atributo.
	p = &Post{Body: "x", ImagePath: `"><script>alert(1)</script>`, VideoPath: `" onerror="alert(1)`}
	out = string(postHTML(p))
	if strings.Contains(out, "<script>alert(1)</script>") || strings.Contains(out, `" onerror="`) {
		t.Fatalf("path no escapado: %s", out)
	}
}

func TestHtmlEscape(t *testing.T) {
	if htmlEscape(`<a href="x">&`) != `&lt;a href=&#34;x&#34;&gt;&amp;` {
		t.Fatalf("htmlEscape: %q", htmlEscape(`<a href="x">&`))
	}
}

func TestExcerpt(t *testing.T) {
	fn := funcs["excerpt"].(func(int, template.HTML) string)
	if got := fn(280, template.HTML("<p>Hola <b>mundo</b></p>")); got != "Hola mundo" {
		t.Fatalf("excerpt simple = %q", got)
	}
	largo := template.HTML("<p>" + strings.Repeat("a", 300) + "</p>")
	if got := fn(280, largo); len([]rune(got)) != 281 || !strings.HasSuffix(got, "…") {
		t.Fatalf("excerpt truncado len=%d sufijo=%q", len([]rune(got)), got)
	}
	// Entidades HTML se decodifican.
	if got := fn(280, template.HTML("<p>a &amp; b</p>")); got != "a & b" {
		t.Fatalf("excerpt entidades = %q", got)
	}
}

func TestTimeFmt(t *testing.T) {
	fn := funcs["timeFmt"].(func(string) string)
	if got := fn("2026-09-22 10:30:00"); got != "22-09-2026 10:30" {
		t.Fatalf("timeFmt = %q", got)
	}
	if got := fn("basura"); got != "basura" {
		t.Fatalf("timeFmt inválido = %q", got)
	}
}

func TestEqstr(t *testing.T) {
	fn := funcs["eqstr"].(func(string, string) bool)
	if !fn("admin", "admin") || fn("user", "admin") {
		t.Fatal("eqstr mal")
	}
}
