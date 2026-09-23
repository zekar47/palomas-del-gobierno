# PALOMAS DEL GOBIERNO

> `ruido subversivo · frecuencia municipal · 9.000 MHz`

Sitio web completo de la banda ficticia **Palomas del Gobierno**: boletín de noticias con reacciones y comentarios, foro con hilos y respuestas, cuentas de usuario con sesión y CSRF, redacción en Markdown con vista previa en vivo, subida de imagen/video por noticia, feeds Atom de contenido completo, y una estética ciberpunk criolla CRT (oscuro + rojo, glitch, scanlines, marquesina) que sigue siendo legible en navegadores de texto (`w3m`, `lynx`).

Stack: **Go estándar (`net/http`, `html/template`) + SQLite (`modernc.org/sqlite`, puro Go, sin CGO) + Goldmark (Markdown GFM) + bcrypt (`golang.org/x/crypto`)**. Sin frameworks JS, sin ORM, sin build de frontend. Un solo binario + un archivo `.db` + carpetas `templates/`, `static/`, `uploads/`.

---

## Tabla de contenidos

1. [Inicio rápido](#1-inicio-rápido)
2. [Requisitos e instalación](#2-requisitos-e-instalación)
3. [Configuración y ejecución](#3-configuración-y-ejecución)
4. [Mapa del repositorio](#4-mapa-del-repositorio)
5. [Arquitectura general](#5-arquitectura-general)
6. [Ciclo de vida de una petición](#6-ciclo-de-vida-de-una-petición)
7. [Modelo de datos (SQLite)](#7-modelo-de-datos-sqlite)
8. [Autenticación, sesiones y seguridad](#8-autenticación-sesiones-y-seguridad)
9. [Sistema de plantillas y frontend](#9-sistema-de-plantillas-y-frontend)
10. [Markdown y render de contenido](#10-markdown-y-render-de-contenido)
11. [Rutas y handlers (referencia completa)](#11-rutas-y-handlers-referencia-completa)
12. [Noticias + comentarios + reacciones](#12-noticias--comentarios--reacciones)
13. [Administración y subidas (uploads)](#13-administración-y-subidas-uploads)
14. [Foro](#14-foro)
15. [Feeds Atom](#15-feeds-atom)
16. [Archivos estáticos y JS](#16-archivos-estáticos-y-js)
17. [Base de datos en operación](#17-base-de-datos-en-operación)
18. [Desarrollo con Nix](#18-desarrollo-con-nix)
19. [Limitaciones conocidas y trabajo futuro](#19-limitaciones-conocidas-y-trabajo-futuro)
20. [Historial narrativo / contenido](#20-historial-narrativo--contenido)

---

## 1. Inicio rápido

```bash
# 1. Clonar / entrar
cd palomas-del-gobierno

# 2. Correr en desarrollo (puerto 8080 por defecto)
go run . 

# 3. Abrir
# http://localhost:8080

# Con contraseña de admin propia (recomendado):
PALOMAS_ADMIN_PASSWORD='cambia-esto' go run . --addr :8080 --db palomas.db --uploads uploads
```

Primer arranque sin usuarios existentes crea automáticamente:

- usuario `admin` / contraseña `admin123` (o lo que pases en `--admin-pass` / `PALOMAS_ADMIN_PASSWORD`), con rol `admin`, hash bcrypt.

Compilar binario:

```bash
go build -o palomas .
./palomas --addr :8080 --db palomas.db --uploads uploads
```

Probar legibilidad en modo texto (filosofía del proyecto: debe funcionar sin CSS/JS):

```bash
w3m http://localhost:8080
lynx http://localhost:8080
curl http://localhost:8080/news/feed.xml
```

---

## 2. Requisitos e instalación

| Requisito | Versión | Notas |
|---|---|---|
| Go | `1.26.5` (ver `go.mod`) | Sin CGO necesario. `modernc.org/sqlite v1.56.0` es SQLite puro en Go. |
| SQLite CLI (opcional) | cualquiera | Solo para inspeccionar `palomas.db`. |
| Nix (opcional) | cualquiera | `flake.nix` provee `devShell` + `package`. |
| `w3m` / `lynx` / `curl` (opcional) | — | Incluidos en el devShell Nix. |

Dependencias directas (`go.mod`):

- `github.com/yuin/goldmark v1.8.5` — parser Markdown (con extensión GFM).
- `golang.org/x/crypto v0.55.0` — `bcrypt` para hash de contraseñas.
- `modernc.org/sqlite v1.56.0` — driver SQLite + tooling puro Go (`modernc.org/libc`, `mathutil`, `memory`, etc. como indirectas).

No hay `package.json`, no hay bundler, no hay migraciones externas. `go mod download` basta.

---

## 3. Configuración y ejecución

El binario se configura solo con flags (ver `main.go:15-20`):

| Flag | Env equivalente | Default | Descripción |
|---|---|---|---|
| `--addr` | — | `:8080` | Dirección de escucha de `http.Server`. |
| `--db` | — | `palomas.db` | Ruta al archivo SQLite. Se crea si no existe. |
| `--uploads` | — | `uploads` | Directorio de imágenes/videos. Se crea con `0755` si falta. |
| `--admin-pass` | `PALOMAS_ADMIN_PASSWORD` | `""` → `admin123` | Contraseña del admin inicial. Solo se usa si la tabla `users` está vacía. |

Secuencia de arranque (`main.go:22-47`):

1. `os.MkdirAll(uploadsDir)` — garantiza carpeta de subidas.
2. `sql.Open("sqlite", dsn)` con DSN `?_pragma=foreign_keys(1)&_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)` — activa FKs, espera 10 s ante lock, y modo WAL.
3. `db.SetMaxOpenConns(8)` — limita concurrencia SQLite.
4. `migrate()` — ejecuta el `schema` idempotente (`CREATE TABLE/INDEX IF NOT EXISTS`).
5. `seedAdmin(pass)` — inserta `admin` si `COUNT(users)==0`, con `bcrypt.DefaultCost`. Si `pass==""` usa `admin123` e imprime advertencia.
6. `loadTemplates()` — pre-parsea las 10 páginas (`base.html` + cada página) una vez al arrancar. Si falla una plantilla, el proceso muere (`log.Fatalf`).
7. `routes(mux)` + `http.Server{ReadHeaderTimeout: 10s}` + `ListenAndServe()` — sirve. Imprime banner ASCII `PALOMAS`.

> El arranque imprime `sistema en línea: http://localhost:8080`, `base de datos: ...`, `uploads: ...`.

---

## 4. Mapa del repositorio

```
.
├── main.go           # arranque: flags, SQLite, migraciones, seed, plantillas, http.Server
├── db.go             # schema SQL + tipos User/Post/Comment/Thread/Reply + queries
├── auth.go           # sesiones en memoria, cookies, CSRF, middlewares, register/login, bcrypt
├── handlers.go       # router, render, todos los handlers HTTP, uploads, preview, 404
├── markdown.go       # Goldmark GFM (sin HTML unsafe) + postHTML (imagen+body+video)
├── atom.go           # feeds Atom 1.0 de noticias y foro (contenido completo)
├── templates/        # 11 plantillas html/template (base + 10 páginas)
│   ├── base.html         # layout: header glitch, nav, sessionbar, footer, feeds
│   ├── home.html         # portada: hero ASCII, ticker, manifiesto, miembros, bitácora, stats
│   ├── news.html         # lista de noticias (excerpt 280 runas)
│   ├── post.html         # noticia sola + reacciones + comentarios
│   ├── admin_edit.html   # formulario crear/editar noticia (multipart + preview)
│   ├── forum.html        # lista de hilos
│   ├── thread.html       # hilo + respuestas
│   ├── new_thread.html   # formulario nuevo hilo
│   ├── login.html        # acceso
│   ├── register.html     # alistamiento
│   └── notfound.html     # 404 paloma extraviada
├── static/
│   ├── style.css     # ~500 líneas: CRT, glitch, ticker, cards, formularios, responsive
│   ├── app.js        # ~50 líneas: único JS, vista previa Markdown vía POST /preview
│   ├── favicon.svg   # icono
│   └── logo.jpeg     # logo
├── uploads/          # archivos subidos (gitignorados). Nombres aleatorios, servidos en /uploads/
├── palomas.db*       # SQLite + WAL/SHM (gitignorados)
├── go.mod / go.sum  # módulo `palomas`, Go 1.26.5
├── flake.nix / flake.lock  # devShell (go, sqlite, w3m, lynx, curl) + buildGoModule
└── .gitignore        # palomas, *.db*, uploads/, result
```

Sin tests, sin CI, sin Dockerfile. El proyecto es deliberadamente monolítico y legible.

---

## 5. Arquitectura general

```
                ┌────────────┐
                │  Browser   │  (HTML puro + 1 JS opcional + CSS CRT)
                │ w3m OK     │
                └─────┬──────┘
                      │ HTTP (formularios POST + GET)
                      ▼
              ┌───────────────┐
              │ net/http mux  │  Go 1.22+ route patterns: "GET /news/{id}"
              │ handlers.go   │  middlewares: requireLogin / requireAdmin / requirePostCSRF
              └──┬───┬───┬────┘
                 │   │   │
        ┌────────┘   │   └────────┐
        ▼            ▼            ▼
   ┌─────────┐  ┌────────┐  ┌──────────┐
   │auth.go  │  │ db.go  │  │markdown/ │
   │sesión   │  │SQLite  │  │atom.go   │
   │memoria  │  │WAL+FK  │  │goldmark  │
   └─────────┘  └────────┘  └──────────┘
        │            │             │
        │     ┌──────┴──────┐      │
        │     │ palomas.db  │      │
        │     │ users/posts │      │
        │     │ comments/   │      │
        │     │ reactions/  │      │
        │     │ threads/    │      │
        │     │ replies     │      │
        └─────┴─────────────┴──────┘
                 templates/ + static/ + uploads/
```

Principios arquitectónicos:

1. **Server-side rendering total.** Cada página es `html/template` ejecutada en servidor. No hay API JSON (salvo `POST /preview` que devuelve fragmento HTML). Todo funciona con formularios y redirects `303 See Other` (patrón PRG).
2. **Sin ORM.** SQL crudo con `database/sql`. Queries agregadas con subselects (`COUNT likes/dislikes/comments/replies`) para evitar N+1.
3. **Seguridad por capas explícitas.** `bcrypt` + cookie `HttpOnly/SameSiteLax` + token CSRF por sesión + `MaxBytesReader(40MB)` + validación de extensiones/tamaños + escape HTML por defecto.
4. **Progresive enhancement.** El único JS (`static/app.js`) mejora el editor con preview; sin JS todo sigue publicando.
5. **Texto primero.** HTML semántico + CSS decorativo ignorable. `w3m`/`lynx` son ciudadanos de primera clase (el devShell Nix los instala a propósito).

Variable global compartida: `var db *sql.DB` (`db.go:10`) y `var uploadsDir = "uploads"` (`handlers.go:18`), seteadas en `main()` antes de servir. Plantillas precompiladas en `map[string]*template.Template` (`handlers.go:43`).

---

## 6. Ciclo de vida de una petición

Ejemplo: `POST /news/3/comment` (comentar noticia).

```
1. Go mux matchea "POST /news/{id}/comment" → cadena requireLogin(requirePostCSRF(newsComment))
2. requireLogin: currentUser(r) → getSession(r) lee cookie `palomas_sesion` → map en memoria → getUser(id) en SQLite.
   Si nil → 303 a /login?next=/news/3/comment
3. requirePostCSRF: currentUser!=nil? r.Body = MaxBytesReader(40MB). validCSRF compara FormValue("csrf") vs Session.CSRF.
   Si falla → 403.
4. newsComment: parseID(PathValue("id")), TrimSpace(PostFormValue("body")), valida no vacío y ≤4000 runas.
   addComment(postID, userID, body) → INSERT INTO comments.
5. 303 redirect a /news/3 → GET newsShow → getPost + listComments + myReaction → render(w,"post",postData{...})
6. render ejecuta template "base" (base.html + post.html) con FuncMap (markdown, timeFmt, etc.) → HTML + Content-Type text/html.
```

Lecturas públicas (`GET /`, `/news`, `/forum`, feeds) no pasan middlewares. Mutaciones siempre exigen `POST + sesión + CSRF`. Noticias exigen `role IN ('admin','member')` (`requireMember`); crear es de ambos, pero editar/borrar noticia ajena solo admin (`canEdit`). Administración de usuarios exige `role=='admin'`. Hilos y comentarios los edita/borra su autor o un admin (`canEdit`, ajeno → 403).

---

## 7. Modelo de datos (SQLite)

Definido en `db.go:12-68` (`const schema`, aplicado por `migrate()` con `db.Exec(schema)`). Todo `IF NOT EXISTS`, timestamps como `TEXT datetime('now')` (`YYYY-MM-DD HH:MM:SS`).

```sql
users(id PK, username UNIQUE COLLATE NOCASE, password_hash, role CHECK('admin','member','user') DEFAULT 'user', created_at)
meta(k PK, v) — versionado de esquema (`schema_version=2`; la v2 amplió el rol a `member` reconstruyendo `users`).
posts(id PK, author_id→users, title, body DEFAULT '', image_path DEFAULT '', video_path DEFAULT '', created_at, updated_at)
comments(id PK, post_id→posts ON DELETE CASCADE, user_id→users, body, created_at)
reactions(post_id→posts ON DELETE CASCADE, user_id→users, kind CHECK('like','dislike'), PK(post_id,user_id))
threads(id PK, user_id→users, title, body, created_at, updated_at)
replies(id PK, thread_id→threads ON DELETE CASCADE, user_id→users, body, created_at)

INDEX idx_posts_created   ON posts(created_at DESC)
INDEX idx_comments_post   ON comments(post_id)
INDEX idx_threads_updated ON threads(updated_at DESC)
INDEX idx_replies_thread  ON replies(thread_id)
```

Diagrama ER (texto):

```
users 1──* posts 1──* comments (CASCADE post→comments)
              │└──* reactions PK(post,user) (CASCADE)
      1──* threads 1──* replies (CASCADE thread→replies)
      1──* comments / replies / reactions (sin cascade hacia user: borrar user violaría FK)
```

Tipos Go espejo (`db.go:102-152`): `User{ID,Username,Role,CreatedAt}`, `Post{...+Author(l join),Likes,Dislikes,Comments,MyReaction}`, `Comment{...+Username}`, `Thread{...+Username,Replies}`, `Reply{...+Username}`.

Queries clave:

- `postSelect` (`db.go:182-188`): `JOIN users` + 3 subselects `COUNT(like/dislike/comments)`. Reusado por `listPosts(limit)` (`ORDER BY created_at DESC [LIMIT n]`, límite interpolado con `Sprintf` — seguro porque `limit` es `int` interno) y `getPost(id)`.
- `threadSelect` (`db.go:313-316`): `JOIN users` + subselect `COUNT(replies)`. Reusado por `listThreads` (`ORDER BY updated_at DESC`) y `getThread`.
- `toggleReaction` (`db.go:261-280`): lee reacción actual; si `ErrNoRows` → `INSERT`; si `cur==kind` → `DELETE` (toggle off); si distinta → `UPDATE` (cambio like↔dislike).
- `addReply` (`db.go:361-377`): transacción `BEGIN; INSERT reply; UPDATE threads.updated_at=now; COMMIT` — el bump de `updated_at` ordena el foro por actividad reciente. `defer tx.Rollback()` como guarda.
- `seedAdmin` (`db.go:77-96`): `COUNT(users)`; si `0`, `bcrypt.GenerateFromPassword` e `INSERT admin`.

Pragmas de conexión (`main.go:28`): `foreign_keys(1)` (respeta cascadas), `busy_timeout(10000)` (reintenta 10 s ante `SQLITE_BUSY`), `journal_mode(WAL)` (lecturas concurrentes + escrituras sin bloquear tanto). `SetMaxOpenConns(8)` acota pool.

---

## 8. Autenticación, sesiones y seguridad

Archivo: `auth.go` (191 líneas).

### 8.1 Sesiones en memoria

- `type Session{UserID int64, CSRF string}` + `map[string]*Session` protegido por `sync.RWMutex` (`auth.go:17-25`).
- `createSession(w,userID)` (`auth.go:35-48`): `token=randHex(16)` (16 bytes → 32 hex), `CSRF=randHex(16)`, guarda en map, `Set-Cookie: palomas_sesion=token; Path=/; HttpOnly; SameSite=Lax; MaxAge=7d`. `randHex` usa `crypto/rand`; fallback `"0000..."` si falla (prácticamente imposible).
- `destroySession` (`auth.go:50-59`): borra del map + cookie expirada `MaxAge=-1`.
- `getSession/currentUser/csrfFor` (`auth.go:61-90`): lee cookie, lookup map, `getUser(UserID)` fresco desde DB (así cambios de rol aplican al instante). Retorna `nil` si no hay sesión.
- Cookie **sin `Secure`** (pensado para `http://localhost`); `HttpOnly` impide robo vía JS, `SameSite=Lax` mitiga CSRF cross-site en GET/POST top-level.

> Implicación operativa: reiniciar el proceso invalida todas las sesiones (map volátil). Para multi-instancia habría que externalizar a SQLite/Redis.

### 8.2 CSRF

- Token aleatorio por sesión, expuesto a plantillas vía `loadPage(r,title).CSRF = csrfFor(r)` (`handlers.go:76-78`) e inyectado como `<input type=hidden name=csrf value="{{.CSRF}}">` en todo form mutante.
- `requirePostCSRF` (`auth.go:132-145`): exige usuario + `r.Body=MaxBytesReader(40MB)` **antes** de leer + `validCSRF` (`FormValue("csrf")==Session.CSRF`). Si falla → `403`. Usado en logout, reacciones, comentarios, admin, foro, preview.
- Login/register (`POST /login`, `POST /register`) deliberadamente **sin** CSRF (no hay sesión previa que fijar; son endpoints de creación de sesión).

### 8.3 Registro y login

- `registerUser(username,password)` (`auth.go:149-170`): `TrimSpace`, valida `3≤len(username)≤24`, `len(password)≥6` (bytes, no runas), `getUserByName` para mensaje amable "ya ocupado", `bcrypt.GenerateFromPassword(DefaultCost)` e `INSERT`. Si hay carrera UNIQUE, el error SQL se mapea al mismo mensaje. Retorna `getUser(LastInsertId)`.
- `loginUser(username,password)` (`auth.go:172-191`): `SELECT id,password_hash WHERE username=?` (case-insensitive por `COLLATE NOCASE` en schema). Si `ErrNoRows`, hace un `CompareHashAndPassword` contra hash dummy para **igualar tiempo** y no permitir enumerar usuarios por timing, retorna `errBadCreds`. Si hay hash real, compara; si falla → mismo error genérico. Mensaje UI siempre "usuario o contraseña incorrectos" (`handlers.go:558`).
- `username` es `UNIQUE COLLATE NOCASE`: `Admin` y `admin` colisionan.

### 8.4 Middlewares

- `requireLogin(next)` (`auth.go:102-110`): si `currentUser==nil` → `303 /login?next=<RequestURI escapado>`. `urlEscape` (`auth.go:112-116`) escapa `&?=# +` manualmente. `safeNext` (`handlers.go:585-590`) al volver solo acepta paths que empiezan con `/` (anti open-redirect).
- `requireAdmin(next)` (`auth.go:119-128`): `u.isAdmin()` (`u!=nil && Role=="admin"`). Si no, mismo redirect a login (no distingue 403 para no filtrar existencia de rutas admin).
- Composición típica: `requireAdmin(requirePostCSRF(adminNewPost))` — primero rol, luego límite cuerpo + CSRF.

### 8.5 Otras defensas

- `http.Server{ReadHeaderTimeout:10s}` anti Slowloris en headers.
- `MaxBytesReader(40MB)` global en POSTs autenticados; `ParseMultipartForm(8MB)` en admin (resto a disco temporal) + chequeos por archivo (10 MB imagen, 25 MB video).
- Uploads: whitelist extensiones (`imgExts`, `vidExts`), `filepath.Base` + `filepath.Join(uploadsDir,name)` + rechazo de `"/"` y `".."` en borrado/servido, nombres `randHex(8)-base36(nano)+ext` no predecibles ni colisionables por usuario.
- XSS: `html/template` escapa por defecto + Goldmark **sin `html.WithUnsafe`** (`markdown.go:17` comentado explícito) — HTML crudo en Markdown se escapa, no se inyecta. `postHTML` escapa paths con `HTMLEscapeString`.
- Longitudes: comentario noticia ≤4000 runas, hilo/respuesta ≤20000 runas (`handlers.go:263,482,524`).

---

## 9. Sistema de plantillas y frontend

Archivo: `handlers.go:22-67` + `templates/*.html`.

- `loadTemplates()` (`handlers.go:45-55`): para cada página en `[home news post admin_edit forum thread new_thread login register notfound]` hace `template.New("base").Funcs(funcs).ParseFiles("templates/base.html","templates/<p>.html")` y guarda en `pageTemplates[p]`. Cada template resultante contiene definición `base` (layout) + `content` (página). Fallo → error fatal al arrancar (fail-fast, no 500 en caliente).
- `render(w,page,data)` (`handlers.go:57-67`): `ExecuteTemplate(w,"base",data)`; si falta plantilla o falla ejecución → `500` + log.
- `pageData{Title,User,CSRF,Flash}` (`handlers.go:69-74`): base embebida en todos los `*Data` (`homeData`, `newsData`, `postData`, `adminData`, `forumData`, `threadData`, `authData`). `loadPage(r,title)` la rellena con usuario + CSRF frescos.
- `FuncMap` (`handlers.go:22-41`):
  - `markdown string→template.HTML` — `renderMarkdown`, usado como `{{.Body | markdown}}`. Retorna HTML ya sanitizado por Goldmark (sin unsafe).
  - `excerpt n html→string` — quita tags con regex `<[^>]+>`, colapsa espacios, `UnescapeString`, corta por runas + `…`. Usado en `news.html` con `280`.
  - `timeFmt "2006-01-02 15:04:05"→"02-01-2006 15:04" UTC` — formatea timestamps SQLite.
  - `eqstr a b→bool` — compara rol (`{{if eqstr .User.Role "admin"}}`).
- `base.html`: `<!DOCTYPE html lang=es>`, meta viewport/description, `style.css`, 2 `<link rel=alternate atom>`, favicon SVG, `.scanlines` overlay, skip-link accesible, `header.site-header` (statusbar SISTEMA EN LINEA/SUBLEVADO/INCAUTADO + `h1.glitch` + slogan + `nav.mainnav` INICIO/NOTICIAS/FORO/FEED/+NOTICIA si admin + `sessionbar` whoami/logout o INGRESAR/ALISTARSE), `main#contenido` (`Flash` + `template "content"`), `footer` (footnav, canales Atom, arte ASCII paloma, nota "Go+SQLite · sin cookies de rastreo"), `<script app.js defer>`.
- Páginas: ver sección 11 para contenido de cada una. Todas usan clases `.panel/.card/.btn/.meta/.dim` del CSS.

---

## 10. Markdown y render de contenido

Archivo: `markdown.go` (55 líneas).

- `mdRenderer = goldmark.New(WithExtensions(GFM), WithParserOptions(AutoHeadingID))` — **sin** `html.WithUnsafe`, así que `<script>`, `<iframe>`, etc. escritos en Markdown se escapan como texto. GFM aporta tablas, tachado, autolinks, code fences, etc.
- `renderMarkdown(src)→template.HTML` (`markdown.go:21-27`): convierte a `bytes.Buffer`; en error retorna párrafo de error. `renderMarkdownString` variante `string` para feeds.
- `postHTML(p)→template.HTML` (`markdown.go:35-51`): compone noticia completa: si `ImagePath` → `<figure><img src="/uploads/...">`, luego `renderMarkdown(Body)`, si `VideoPath` → `<figure><video controls preload=metadata src=...> + <figcaption><a ...>descargar video</a>`. Paths escapados con `HTMLEscapeString`.
- Preview (`handlers.go:596-601` + `app.js`): `POST /preview` (login+CSRF) devuelve `renderMarkdown(body)` como `text/html`. El editor lo inyecta en `#preview`.
- `admin_edit.html` incluye ayuda Markdown (`#`, `**`, `*`, `~~`, `` ` ``, `![...](/uploads/...)`, `[texto](url)`, `>`, `-`, `|`).

---

## 11. Rutas y handlers (referencia completa)

Definidas en `routes()` (`handlers.go:88-127`) con sintaxis Go 1.22 `METHOD pattern`. `GET /{$}` es raíz exacta; `GET /` final es catch-all 404.

| Método | Ruta | Handler | Auth | Descripción |
|---|---|---|---|---|
| GET | `/` (`/{$}`) | `homeHandler` | pública | Portada: últimas 3 noticias, 4 hilos, 5 miembros hardcodeados, `COUNT(posts/users)`. Template `home`. |
| GET | `/static/` | `staticHandler` | pública | `StripPrefix + FileServer(Dir static)`. CSS/JS/favicon/logo. |
| GET | `/uploads/` | `uploadsHandler` | pública | `ServeFile(Join(uploadsDir, Base(path)))`. Solo basename, anti traversal. |
| GET | `/login` | `loginGet` | pública | Form acceso (+ `?next=`). |
| POST | `/login` | `loginPost` | — | `loginUser` → `createSession` → `303 safeNext(next)`. Error → re-render con mensaje. |
| GET | `/register` | `registerGet` | pública | Form alistamiento. |
| POST | `/register` | `registerPost` | — | `registerUser` → `createSession` → `303 /`. Error → re-render. |
| POST | `/logout` | `logoutPost` | login+CSRF | `destroySession` → `303 /`. |
| GET | `/news` | `newsList` | pública | `listPosts(50)` → `news.html` (cards + excerpt). |
| GET | `/news/feed.xml` | `newsFeed` | pública | Atom noticias (ver §15). |
| GET | `/news/{id}` | `newsShow` | pública | `getPost+listComments+myReaction` → `post.html`. `id≤0` o no existe → 404. |
| POST | `/news/{id}/reaction` | `newsReaction` | login+CSRF | `reaction=like\|dislike` → `toggleReaction` → `303 /news/{id}`. Otro valor → 400. |
| POST | `/news/{id}/comment` | `newsComment` | login+CSRF | `body` trim, no vacío, ≤4000 runas → `addComment` → `303`. |
| GET | `/admin/new` | `adminNewGet` | member (admin o banda) | Form vacío `admin_edit.html` (`IsEdit=false`). |
| POST | `/admin/new` | `adminNewPost` | admin+CSRF | Multipart (ver §13) → crea post + subidas → `303 /news/{id}`. |
| GET | `/admin/edit/{id}` | `adminEditGet` | admin | Form precargado (`IsEdit=true`). No existe → 404. |
| POST | `/admin/edit/{id}` | `adminEditPost` | admin+CSRF | Actualiza título/cuerpo/imagen/video (ver §13). |
| POST | `/admin/delete/{id}` | `adminDelete` | admin+CSRF | Borra archivos + `deletePost` (cascada comentarios/reacciones) → `303 /news`. |
| GET | `/forum` | `forumList` | pública | `listThreads(100)` → `forum.html`. |
| GET | `/forum/feed.xml` | `forumFeed` | pública | Atom foro con hilos+respuestas (ver §15). |
| GET | `/forum/new` | `forumNewGet` | login | Form `new_thread.html`. |
| POST | `/forum/new` | `forumNewPost` | login+CSRF | Valida título/body no vacíos, body ≤20000 → `createThread` → `303 /forum/{id}`. |
| GET | `/forum/{id}` | `forumThread` | pública | `getThread+listReplies` → `thread.html`. No existe → 404. |
| POST | `/forum/{id}/reply` | `forumReply` | login+CSRF | Valida body → `addReply` (tx + bump) → `303`. |
| POST | `/preview` | `previewHandler` | login+CSRF | Devuelve HTML del Markdown `body`. Usado por `app.js`. |
| GET | `/forum/{id}/edit` | `forumEditGet` | login (autor/admin) | Form editar hilo. Ajeno → 403. |
| POST | `/forum/{id}/edit` | `forumEditPost` | login+CSRF (autor/admin) | Valida título/cuerpo → `updateThread` → `303 /forum/{id}`. |
| POST | `/forum/{id}/delete` | `forumDelete` | login+CSRF (autor/admin) | `deleteThread` (cascada replies) → `303 /forum`. |
| GET | `/comment/{id}/edit` | `commentEditGet` | login (autor/admin) | Form editar comentario. Ajeno → 403. |
| POST | `/comment/{id}/edit` | `commentEditPost` | login+CSRF (autor/admin) | Valida body ≤4000 → `updateComment` → `303 /news/{post}`. |
| POST | `/comment/{id}/delete` | `commentDelete` | login+CSRF (autor/admin) | `deleteComment` → `303 /news/{post}`. |
| GET | `/admin/users` | `adminUsers` | admin | Tabla de usuarios (rol, alta, acciones). |
| POST | `/admin/users/{id}/role` | `adminUserRole` | admin+CSRF | `setUserRole` user↔member (nunca deja cero admins). |
| POST | `/admin/users/{id}/delete` | `adminUserDelete` | admin+CSRF | `deleteUser` (cascada total + archivos) → `303 /admin/users`. |
| GET | `/account` | `accountGet` | login | Cuenta propia (renombrar, clave, eliminar). |
| POST | `/account/username` | `accountUsername` | login+CSRF | `updateUsername` (único, 3-24). |
| POST | `/account/password` | `accountPassword` | login+CSRF | Verifica actual + `setPassword` (≥6). |
| POST | `/account/delete` | `accountDelete` | login+CSRF | `deleteUser` propio + logout → `303 /`. |
| * | `/` catch-all | `notFoundHandler` | pública | `404 + notfound.html` ("PALOMA EXTRAVIADA"). También usado manualmente por `newsShow/forumThread/admin*` cuando `parseID==0` o `get*` falla. |

Helpers: `parseID(s)→int64` (`handlers.go:80-86`, `0` si inválido/≤0), `staticHandler`, `uploadsHandler`, `safeNext`, `saveUpload/joinExts/removeUploads`.

---

## 12. Noticias + comentarios + reacciones

- **Lista** (`newsList`, `news.html`): hasta 50 posts `ORDER BY created_at DESC`. Cada card: título linkeado, meta `[fecha] · por autor · ▲likes · ▼dislikes · N comentarios`, excerpt Markdown→texto 280 runas, botón `LEER TRANSMISIÓN →`. Admin ve `+ NUEVA NOTICIA`.
- **Detalle** (`newsShow`, `post.html`): título, meta (incluye "editada [fecha]" si `created!=updated`), `PostHTML` (imagen+markdown+video), dos forms de reacción (botón `.on` si `MyReaction` coincide), toolbar admin EDITAR/BORRAR (borrar con `confirm()`), lista comentarios (`@user · [fecha]` + body Markdown) y form comentar (login requerido, si no link a login).
- **Reacción** (`newsReaction`): toggle — click like sin reacción → inserta like; otra vez → borra; luego dislike → update. Siempre redirect PRG, sin AJAX.
- **Comentario** (`newsComment`): límites estrictos, sin edición/borrado (diseño simple).

---

## 13. Administración y subidas (uploads)

Solo `role=admin` (ver `requireAdmin`). Flujo crear (`adminNewPost`, `handlers.go:295-326`):

1. `ParseMultipartForm(8MB)` — si excede 40 MB totales (límite `MaxBytesReader` del middleware) → re-render con "archivo demasiado grande (máx 40 MB)".
2. `title=TrimSpace(...)` obligatorio; `body` crudo (puede ser vacío en creación, editable después).
3. `createPost(userID,title,body)` → `id`.
4. `saveUpload(r,"image",imgExts{.png,.jpg,.jpeg,.gif,.webp},10MB)` — si error, `deletePost(id)` rollback y re-render "imagen: ...".
5. `saveUpload(r,"video",vidExts{.mp4,.webm},25MB)` — si error, borra imagen ya guardada + `deletePost` y re-render "video: ...".
6. `updatePost(id,title,body,img,vid)` con nombres finales → `303 /news/{id}`.

Flujo editar (`adminEditPost`, `handlers.go:338-386`): carga post, valida título, procesa checkboxes `remove_image/remove_video==1` (borra archivo y vacía campo), luego `saveUpload` opcionales (si retorna `""` no había archivo; si retorna nombre, reemplaza y borra anterior), finalmente `updatePost`. Errores de imagen/video re-renderizan con mensaje sin perder contexto.

`saveUpload` (`handlers.go:400-426`): `FormFile(field)` → si `ErrMissingFile` retorna `"",nil` (campo opcional). Valida extensión lowercased contra whitelist (`joinExts` para mensaje), valida `header.Size>maxBytes`, genera `randHex(8)-base36(nano)+ext`, `os.Create(Join(uploadsDir,name))` + `io.Copy`. Retorna nombre solo-basename (lo que se guarda en DB).

`removeUploads(paths...)` (`handlers.go:436-443`): ignora `""` o con `"/"`/`".."` (anti traversal/borrado fuera), `os.Remove(Join(...))` ignorando error.

`uploadsHandler` (`handlers.go:133-141`): `Base(URL.Path)` + `ServeFile` — nunca sirve subdirectorios.

> Nota: `header.Size` viene del cliente; el límite real anti-DoS es `MaxBytesReader(40MB)` + `ParseMultipartForm`. Archivos quedan en `uploads/` (gitignorado) y se referencian como `/uploads/<nombre>`.

---

## 14. Foro

- **Lista** (`forumList`, `forum.html`): hasta 100 hilos `ORDER BY updated_at DESC` (actividad reciente primero gracias al bump en `addReply`). Cada item: título, `por @user · [creada] · N respuestas · última actividad [...]`. Botón `+ NUEVO HILO` si login, si no prompt de ingreso.
- **Nuevo hilo** (`forumNewGet/forumNewPost`, `new_thread.html`): form título+body Markdown + preview JS. Validaciones con `Flash`: título y contenido obligatorios, body ≤20000 runas. Éxito → `303 /forum/{id}`.
- **Hilo** (`forumThread`, `thread.html`): título, meta autor/fecha, body Markdown, `// RESPUESTAS (N)` lista cronológica ASC, form responder (login) o prompt. Sin edición/borrado ni moderación — simplicidad deliberada.
- **Responder** (`forumReply`): valida hilo válido, body no vacío y ≤20000, `addReply` transaccional (inserta + bump `updated_at`).

---

## 15. Feeds Atom

Archivo: `atom.go` (167 líneas). Dos feeds de **contenido completo** (no solo titulares), anunciados en `<head>` de `base.html` como `rel=alternate`.

- Tipos: `atomFeed{xmlns,id,title,subtitle,updated,link[],entries[]}`, `atomLink{rel,type,href}`, `atomEntry{id,title,updated,published,link[],author,content}`, `atomContent{type,body}`. Serializado con `encoding/xml` indentado, `Content-Type: application/atom+xml; charset=utf-8`.
- Helpers: `rfc3339(sqliteTime)` parsea `2006-01-02 15:04:05` → RFC3339 UTC (fallback `now`); `absURL(r,path)` respeta `http/https` según `r.TLS`; `tagURI(r,kind,id)` → `tag:<host>,2026:<kind>/<id>` (normaliza `localhost*` a `palomasdelgobierno.local` para IDs estables); `feedHead(r,selfPath,title)` con `id tag:...feed/0`, `subtitle "canal de difusión subterránea"`, `updated now`, links `self` + `alternate /`.
- `newsFeed` (`atom.go:101-125`): `listPosts(100)`, cada entry con `tag:post/<id>`, `published/updated`, link `/news/<id>`, autor, `content type=html` = `html.UnescapeString(postHTML(p))` (imagen+markdown+video íntegros).
- `forumFeed` (`atom.go:129-167`): `listThreads(100)`, cada entry con `tag:thread/<id>`, body = `<div forum-post>por <strong>user</strong> + markdown(hilo)</div>` + por cada reply `<div forum-reply>respuesta de <strong>user</strong>: + markdown(reply)</div>` (usernames escapados, cuerpos ya seguros por Goldmark).

Verificación rápida: `curl localhost:8080/news/feed.xml`, `curl localhost:8080/forum/feed.xml`.

---

## 16. Archivos estáticos y JS

- `static/style.css` (~507 líneas): variables `--bg --red --ash --mono`, fondo radial + scanlines fijas + `mix-blend multiply`, `.glitch` con `::before/::after` croma (cyan/blanco) y keyframes `glitch-a/b`, header/nav/sessionbar, `.panel/.card/.grid2` (responsive 1 col <720px), `.ticker` marquesina CSS (`translateX` 28 s loop, pausa en hover), `.members` grid auto-fit, `.post/.reactions/.comments`, formularios negros con foco rojo, `.flash`, footer, `.skip` accesible, `@media prefers-reduced-motion: reduce` (desactiva glitch/ticker/scanlines). Todo decorativo: sin CSS el HTML sigue legible (w3m).
- `static/app.js` (~50 líneas, único JS): `wirePreview()` — busca `#preview-btn/#body/#preview-wrap/#preview`; click alterna `hidden` + `fetch POST /preview {body,csrf} same-origin` → `innerHTML`; `input` con debounce 400 ms refresca si visible. Sin JS, `<noscript>` avisa y el form publica igual. Sin dependencias.
- `static/favicon.svg`, `static/logo.jpeg`: icono y logo referenciados desde `base.html`/contenido.
- Servido por `staticHandler` (`StripPrefix /static/ + FileServer`). Sin cache headers custom, sin minificación.

---

## 17. Base de datos en operación

- Archivo: `palomas.db` (+ `-wal`/`-shm` en caliente). Gitignorado (`.gitignore: *.db*`). Respaldo = copiar el `.db` (ideal con `sqlite3 palomas.db ".backup main backup.db"` en caliente por WAL).
- Inspección:
  ```bash
  sqlite3 palomas.db ".tables"
  sqlite3 palomas.db "SELECT id,username,role,created_at FROM users;"
  sqlite3 palomas.db "SELECT id,title,created_at FROM posts ORDER BY created_at DESC LIMIT 5;"
  ```
- `flake.nix` devShell incluye `sqlite` CLI.
- Concurrencia: WAL + `busy_timeout 10s` + pool 8. Suficiente para escala pequeña/mono-nodo. Escrituras pesadas concurrentes pueden dar `database is locked` tras 10 s — aceptable para este uso.
- Borrados en cascada: borrar post/hilo borra comentarios/reacciones/replies automáticamente (FK `ON DELETE CASCADE`), pero **no** borra archivos en `uploads/` salvo que `adminDelete` los borre explícitamente antes (`removeUploads`). Hilos no tienen borrado UI.

---

## 18. Desarrollo con Nix

`flake.nix`: `description "Palomas del Gobierno — sitio web de la banda (Go + SQLite)"`, inputs `nixpkgs/nixos-unstable`, `systems [x86_64-linux aarch64-linux]`.

- `nix develop` → `mkShell{go, sqlite, w3m, lynx, curl}` + `shellHook` que imprime banner y `go run . [-addr :8080]`.
- `nix build` → `buildGoModule{pname=palomas, version=0.1.0, src=./., vendorHash=null, mainProgram=palomas}` → `result/bin/palomas`.
- `.gitignore` excluye `result` (symlink de `nix build`) además de binario `palomas`.

Sin Nix: `go run .` / `go build -o palomas .` funciona igual (Go puro, sin CGO).

---

## 19. Limitaciones conocidas y trabajo futuro

Lectura honesta del código actual:

1. **Sesiones volátiles.** `map` en memoria → reinicio invalida todo, sin expiración activa (solo `MaxAge` cookie), sin límite por usuario. Mejora: tabla `sessions` en SQLite con expiración y limpieza.
2. **Sin paginación/búsqueda.** `LIMIT 50/100` fijos, sin `OFFSET`/FTS. Con cientos de posts/hilos la lista crece sin control.
3. **Sin roles intermedios ni moderación foro.** Cualquiera logueado abre hilos/responde sin rate-limit; no hay editar/borrar/reportar. Spam/DoS de contenido posible.
4. **Validación mínima de contenido.** Solo longitudes y título no vacío. Sin rate-limit por IP/usuario, sin captcha, sin sanitización extra más allá de Goldmark-safe.
5. **Uploads sin thumbnails ni antivirus.** Nombres aleatorios pero servidos con `ServeFile` (sniffing MIME de Go). Sin límite por usuario/cuota, sin borrado huérfano (si falla entre `createPost` y `updatePost` hay rollback explícito, pero ediciones parciales pueden dejar huérfanos si el proceso muere a mitad).
6. **Timestamps como TEXT sin zona.** `datetime('now')` UTC implícito, formateado como UTC en UI/feeds. Sin localización por usuario.
7. **Seguridad cookie.** Falta `Secure` (necesario tras TLS) y rotación de CSRF. `HttpOnly+Lax` es buen mínimo pero no completo.
8. **Sin tests.** `go vet` pasa, pero no hay `_test.go`. Añadir tests a `toggleReaction`, `saveUpload`, `safeNext`, `excerpt`, `rfc3339` sería de alto valor.
9. **Accesibilidad parcial.** Skip-link y `prefers-reduced-motion` existen, pero faltan labels ARIA completos y contraste auditado.

---

## 20. Historial narrativo / contenido

La portada (`homeHandler`, `handlers.go:163-184`) hardcodea la mitología (no viene de DB):

- **KIRA VÁSQUEZ** — voz. Ex-funcionaria del Ministerio de Palomas y Mensajería. *Activa.*
- **EL CHÓFER** — guitarra. Repartía pan en moto municipal, ahora riffs. *Activo.*
- **MAGDALENA 'MAGO' RUIZ** — bajo. Cuatro cuerdas, cero permiso municipal. *Activa.*
- **TOÑO 'LA MUELA' REYES** — batería. Percusión procesal con kit decomisado. *Activo.*
- **PALOMA FANTASMA** — samplers/ruido. Transmite desde radiotransmisor abandonado. *Desaparecida (¿?).*

Manifiesto (`home.html`): "fuimos un chiste burocrático y nos volvimos un problema de estado... guitarras prestadas, bajo decomisado, ritmo que no sale en el reglamento". Ticker: "se escucha fuerte, se escucha claro, no se puede apagar / la paloma conoce tus horarios". Footer: `© 2026 — el contenido vuela libre / ninguna paloma fue herida`.

---

### Comandos útiles (resumen)

```bash
go run . --addr :8080 --db palomas.db --uploads uploads
PALOMAS_ADMIN_PASSWORD='secreta' go run .
go build -o palomas . && ./palomas --addr :8080
go vet ./...
sqlite3 palomas.db "SELECT * FROM users;"
nix develop   # shell con go+sqlite+w3m+lynx+curl
nix build && ./result/bin/palomas
w3m http://localhost:8080
curl http://localhost:8080/news/feed.xml | head -n 40
```
