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

func TestYouTubeEmbed(t *testing.T) {
	validos := map[string]string{
		"watch":        "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		"watch params": "https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=30s&list=PL1234567890a",
		"youtu.be":     "https://youtu.be/dQw4w9WgXcQ",
		"youtu.be t":   "https://youtu.be/dQw4w9WgXcQ?t=42",
		"shorts":       "https://www.youtube.com/shorts/dQw4w9WgXcQ",
		"embed":        "https://www.youtube.com/embed/dQw4w9WgXcQ",
		"live":         "https://www.youtube.com/live/dQw4w9WgXcQ",
		"music":        "https://music.youtube.com/watch?v=dQw4w9WgXcQ",
		"link manual":  "[mira esto](https://youtu.be/dQw4w9WgXcQ)",
	}
	for name, in := range validos {
		out := string(renderMarkdown(in))
		if !strings.Contains(out, "youtube-nocookie.com/embed/dQw4w9WgXcQ") {
			t.Fatalf("%s: sin iframe: %s", name, out)
		}
		if !strings.Contains(out, "ver en YouTube") {
			t.Fatalf("%s: sin fallback: %s", name, out)
		}
	}
	invalidos := map[string]string{
		"id corto":  "https://www.youtube.com/watch?v=corto123",
		"id sucio":  "https://www.youtube.com/watch?v=dQw4w9WgXc!",
		"sin id":    "https://www.youtube.com/watch?list=PL123",
		"vimeo":     "https://vimeo.com/123456789",
		"falso":     "https://www.youtube.com.evil.test/watch?v=dQw4w9WgXcQ",
		"evevil":    "https://evilyoutube.com/watch?v=dQw4w9WgXcQ",
		"código":    "`https://www.youtube.com/watch?v=dQw4w9WgXcQ`",
		"ruta rara": "https://youtu.be/",
		"doble seg": "https://youtu.be/dQw4w9WgXcQ/extra",
		// Goldmark no auto-enlaza hosts en mayúsculas: queda texto plano.
		"mayusculas": "https://M.YOUTUBE.COM/watch?v=dQw4w9WgXcQ",
	}
	for name, in := range invalidos {
		out := string(renderMarkdown(in))
		if strings.Contains(out, "<iframe") {
			t.Fatalf("%s: iframe indebido: %s", name, out)
		}
	}
	// Múltiples en un doc + texto alrededor intacto.
	out := string(renderMarkdown("hola https://youtu.be/dQw4w9WgXcQ y https://vimeo.com/1 adiós"))
	if strings.Count(out, "<iframe") != 1 || !strings.Contains(out, "vimeo.com/1") {
		t.Fatalf("múltiple: %s", out)
	}
	// Intento de inyección vía URL: no hay iframe y nada crudo.
	out = string(renderMarkdown("https://www.youtube.com/watch?v=dQw4w9WgXcQ%22%3E%3Cscript%3E"))
	if strings.Contains(out, "<script>") || strings.Contains(out, "<iframe") {
		t.Fatalf("inyección: %s", out)
	}
}

func TestYoutubeIDUnit(t *testing.T) {
	if youtubeID("https://www.youtube.com/watch?v=dQw4w9WgXcQ") != "dQw4w9WgXcQ" {
		t.Fatal("watch")
	}
	for _, bad := range []string{"", "basura", "ftp://youtu.be/dQw4w9WgXcQ", "https://youtu.be/dQw4w9WgXcQ\ninyección", "javascript:alert(1)"} {
		if youtubeID(bad) != "" {
			t.Fatalf("youtubeID(%q) debió ser vacío", bad)
		}
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
