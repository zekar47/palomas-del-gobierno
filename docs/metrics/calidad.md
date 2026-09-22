# Métricas de calidad — palomas-del-gobierno

- **Fecha (UTC):** 2026-09-22T22:56:19Z
- **SonarCloud:** no (proyecto `zekar47_palomas-del-gobierno`)
- **Cobertura local (go cover):** 86.0% (umbral CI: 80%)

## SonarCloud

| Métrica | Valor |
|---|---|
| Bugs | ? |
| Vulnerabilidades | ? |
| Security hotspots | ? |
| **Code smells** | **?** |
| **Deuda técnica (SQALE)** | **?** |
| Cobertura | ?% |
| Duplicación | ?% |
| Líneas de código (ncloc) | ? |
| Rating fiabilidad | ? |
| Rating seguridad | ? |
| Rating mantenibilidad | ? |

## SAST local (referencia)

- **gosec:** 0 issues (8 supresiones `#nosec` justificadas en código: cookies sin
  `Secure` por HTTP plano, Goldmark sin unsafe, nombre de upload generado por
  servidor, redirects a ruta fija `/login`, retardo anti-enumeración).
- **govulncheck:** 0 vulnerabilidades alcanzables.
- **Tests de seguridad en Go:** XSS (payloads servidos escapados/omitidos) y
  SQLi (payloads inocuos, tablas intactas) en `security_test.go`.

## Quality Gate

Objetivo: **Sonar Way + cobertura ≥ 80%**. El workflow de CI falla bajo el umbral
(`scripts/check_coverage.sh`) y publica este reporte como artefacto + job summary.
