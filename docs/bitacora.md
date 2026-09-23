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

## 2026-09-23 — Fix Sonar línea 189: secreto hardcodeado (S2068)

Qué: en `loginUser`, para igualar tiempos ante usuario inexistente se comparaba
contra un hash bcrypt literal en el código. Sonar lo marcó (2 issues de
responsabilidad): un secreto en el fuente no es confiable.
Por qué fix real: se eliminó el literal. Ahora se genera un hash desechable
con `bcrypt.GenerateFromPassword` (mismo orden de coste que una comparación,
así que la protección anti-enumeración se mantiene) y el error se maneja
explícito — de paso cayó un `#nosec G104` que ya no hace falta.
Verificación: `TestLoginUser` (inexistente/clave mala siguen dando
`errBadCreds`), `TestSQLiInocua`, `gosec` 0 issues con 7 supresiones (una
menos que antes).

## 2026-09-23 — Contraseña del administrador: dónde está y cómo leerla

Qué: la contraseña real del admin se genera una sola vez en el primer arranque
de la instancia (user-data: 16 bytes hex de `/dev/urandom` → `/etc/palomas.env`,
`chmod 600`) y no existe en ningún otro lado.
Cómo leerla: nuevo `scripts/admin_password.sh` (resuelve el InstanceId desde
los outputs del stack, lee el archivo vía SSM y lo imprime). Verificado:
devuelve `PALOMAS_ADMIN_PASSWORD=…` y un login `POST /login` con esa clave
responde 303 (sesión creada). Nota: al recrear el stack la contraseña cambia
(user-data corre de nuevo).

## 2026-09-23 — TLS (1): SG 80/443 y dos updates fallidos por charset

Qué: para HTTPS con Caddy se abrieron los puertos 80 (redirect + desafío
ACME) y 443 en el SG. El `cloudformation deploy` falló DOS veces con
`UPDATE_ROLLBACK_COMPLETE` antes de pasar.
Fallo 1: la descripción de una regla llevaba tilde ("desafío"/"restringirá").
Fallo 2: la descripción `"HTTPS Caddy (Let's Encrypt)"` lleva apóstrofe.
EC2 solo acepta `[a-zA-Z0-9. _-:/()#,@[]+=&;{}!$*]` en descripciones de reglas
(ni tildes ni apóstrofes). Arreglo: todo ASCII sin apóstrofes; el tercer
deploy pasó y el SG quedó en 80/8080/443.
Lección: validar descripciones de SG contra ese charset antes de desplegar.

## 2026-09-23 — TLS (2): checksums de Caddy son SHA512, no SHA256

Qué: el primer deploy con Caddy falló en la instancia con
`sha256sum: no properly formatted SHA256 checksum lines found`.
Diagnóstico: el `grep` sí matcheaba (repro en SSM: PIPESTATUS=0 1); el archivo
`caddy_*_checksums.txt` trae hashes de 128 hex = SHA512 (42 líneas verificadas
localmente). Supuse SHA256 sin mirar.
Arreglo: `sha512sum -c -` en `scripts/instance_deploy.sh`.
Lección: verificar el largo del hash antes de elegir la herramienta.

## 2026-09-23 — TLS (3): el tarball se descargaba con otro nombre

Qué: tras arreglar lo de SHA512, falló con `sha512sum: caddy_2.11.4...tar.gz:
No such file or directory`. El `curl -o caddy.tgz` renombraba el archivo pero
el checksum referencia el nombre original del release.
Arreglo: descargar conservando el nombre original
(`-o "caddy_${VER}_linux_amd64.tar.gz"`) y limpiar el `caddy.tgz` huérfano de
intentos anteriores.
Lección: si se verifica checksum contra nombre de archivo, no renombrar al
descargar.

## 2026-09-23 — TLS (4): HTTPS vivo, pero sin Secure ni https en feeds

Qué: el deploy pasó y `https://44-218-216-230.sslip.io` dio 200 con
certificado Let's Encrypt válido (CN correcto, 90 días) y redirect 308 de
HTTP a HTTPS. Pero al verificar fino: la cookie NO traía `Secure` y los feeds
seguían generando `http://`.
Diagnóstico: causa única — `systemctl enable --now` NO reinicia un servicio
que ya está activo; la instancia seguía corriendo el binario viejo (sin
`--cookie-secure` ni `X-Forwarded-Proto`).
Arreglo: `enable` + `restart` explícito para palomas (binario nuevo +
plantillas cacheadas lo exigen) y `reload-or-restart` para caddy (no cortar
conexiones). Tras redesplegar: cookie
`HttpOnly; Secure; SameSite=Lax` y feeds con `https://`. Verificado con
`curl -D` (Set-Cookie) y `openssl s_client` (issuer Let's Encrypt).
Lección: `--now` solo arranca si está inactivo; todo deploy debe reiniciar
explícito. Por eso la verificación fina (no solo HTTP 200) es obligatoria.

## 2026-09-23 — TLS (5): 8080 restringido a solo-vía-Caddy

Qué: con HTTPS verificado, el SG deja de exponer el 8080 al mundo: la regla
pasó de `CidrIp 0.0.0.0/0` a `SourceSecurityGroupId` propio (self). Ahora a la
app solo se llega vía Caddy (:80/:443), lo que además hace no-falsificable la
cabecera `X-Forwarded-Proto`.
Verificación: `https://` sigue 200 y `http://<EIP>:8080/` ya no responde desde
fuera. Variable `PALOMAS_APP_URL` actualizada a la URL https.
Fallo intermedio: la primera versión ponía la regla self como `ingress`
inline del propio SG y CloudFormation rechazó con `Circular dependency
between resources: [AppInstance, AppSecurityGroup, AppEIPAssociation]`.
Arreglo: regla 8080 como recurso aparte `AWS::EC2::SecurityGroupIngress`
(`AppSelfIngress8080`), que solo depende del SG.

## 2026-09-23 — SonarCloud: primer análisis OK, pero métricas vacías

Qué: tras desactivar Automatic Analysis, el CI pasó completo (`test` +
`sonar` en verde). Pero el artefacto `metricas-calidad` traía todo `?`:
el script consultó la API segundos después de subir el scan, cuando el
Compute Engine aún no lo procesaba.
Arreglo: `scripts/sonar_metrics.sh` ahora reintenta la API 12×25s hasta ver
medidas, y además consulta `/api/qualitygates/project_status` para incluir el
estado del Quality Gate y sus condiciones en el reporte.

## 2026-09-23 — SonarCloud: conflicto Automatic Analysis vs CI

Qué: el job `sonar` del CI falló con `You are running CI analysis while
Automatic Analysis is enabled` (el job `test` pasó en verde). Buena señal:
el proyecto ya existe en SonarCloud y `SONAR_TOKEN` ya está como secret.
Pendiente del dueño (un click): en SonarCloud → proyecto → Administration →
Analysis Method → desactivar **Automatic Analysis** (el análisis de CI es el
autoritativo: lleva cobertura y gate). Tras eso, re-correr el workflow.
