# Bitácora de operación — palomas-del-gobierno

Registro cronológico de lo que se hace en este repositorio: cambios, motivos,
fallos y cómo se arreglaron. La entrada más reciente va al final.

Formato de entrada: fecha (UTC), qué, por qué, cómo se verificó. Si algo salió
mal, se documenta en la misma entrada bajo "Fallo:".

---

## 2026-09-22 — Contexto heredado (sesión anterior, resumido)

Se dejó: stack `palomas-entorno` en us-east-1 (EC2 t3.micro, EIP
`44.218.216.230`, bucket S3, SG solo-8080), suite de tests con cobertura 86%,
`gosec`/`govulncheck` en verde (Go 1.26.6), pipeline CI+Deploy en verde y sitio
en vivo en `http://44.218.216.230:8080`.

Fallos de esa sesión y su arreglo (para no repetirlos):

1. `secrets` no existe en `jobs.<id>.if` de GitHub Actions → el workflow moría
   al instante. Arreglo: detectar el token en un step y gatear con
   `steps.<id>.outputs`.
2. `run: echo "texto: con dos puntos"` sin citar rompe el parseo YAML (`: `
   es separador, aunque vaya entre comillas intermedias). Arreglo: citar TODO
   el valor. Detectado con `actionlint`.
3. El binario exige `templates/` y `static/` relativos al CWD; systemd
   arrancaba en `/` y la app moría en loop. Arreglo: `WorkingDirectory` en la
   unit + sincronizar ambas carpetas desde S3 en cada deploy.
4. `case "$s" in InProgress\|Pending\|Delayed)` no alterna (`\|` es literal);
   el deploy local abortaba al primer poll. Arreglo: `A|B|C)`. Además los pipes
   (`| tail`) enmascaran exit codes: el fallo parecía éxito.
5. `govulncheck` encontró 3 CVE reales de stdlib en Go 1.26.5 (uno en
   `html/template`, GO-2026-6091). Arreglo: `go 1.26.6`.
6. El conversor `t-yuki/gocover2sonar` recordado de memoria no existe (404).
   Arreglo: conversor propio `scripts/gocover2sonar.py` sin dependencias.

## 2026-09-23 — Sesión: TLS/HTTPS, contraseña admin, hallazgos Sonar en auth.go

Plan: (a) TLS con Caddy + Let's Encrypt usando nombre `sslip.io` (sin dominio
propio, coste $0, sin ALB); (b) documentar y scriptear la obtención de la
contraseña admin; (c) corregir los 3 hallazgos Sonar en `auth.go` con fixes
reales + commits separados; (d) endurecer SG (8080 solo-vía-Caddy) tras
verificar HTTPS; (e) registrar todo aquí.

## 2026-09-23 — Fix Sonar líneas 43/60: cookies sin Secure (S2092)

Qué: las cookies de sesión/logout no llevaban `Secure` (ni `SameSite` la de
logout). Sonar las marcó como no confiables.
Por qué fix real y no supresión: ahora habrá HTTPS en producción, así que el
atributo debe existir de verdad.
Cambio: nuevo flag `--cookie-secure` (var `cookieSecure`, default `false` para
no romper `http://localhost` en desarrollo); `createSession` y
`destroySession` lo usan; la cookie de logout además ganó `SameSite=Lax`;
`MaxAge` a const `sessionMaxAge`. De paso `absURL` respeta
`X-Forwarded-Proto` (lo necesitarán los feeds tras el terminador TLS; el SG
impedirá falsificar esa cabecera desde fuera).
Verificación: `TestCookieSecureFlag` (Secure on/off + logout), `TestAbsURL`
con cabecera de proxy, `go vet`, `gosec` 0 issues.
