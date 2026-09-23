# Métricas de calidad — palomas-del-gobierno

- **Fecha (UTC):** 2026-09-23T02:59:45Z
- **SonarCloud:** sí (proyecto `zekar47_palomas-del-gobierno`)
- **Cobertura local (go cover):** 85.2% (umbral CI: 80%)

## SonarCloud

| Métrica | Valor |
|---|---|
| Bugs | 0 |
| Vulnerabilidades | 5 |
| Security hotspots | 0 |
| **Code smells** | **0** |
| **Deuda técnica (SQALE)** | **0d 0h 0m** |
| Cobertura | 86.8% |
| Duplicación | 0.0% |
| Líneas de código (ncloc) | 2764 |
| Rating fiabilidad | A |
| Rating seguridad | C |
| Rating mantenibilidad | A |

## Hallazgos abiertos de seguridad

| Tipo | Severidad | Ubicación | Mensaje |
|---|---|---|---|
| VULNERABILITY | HIGH | .github/workflows/deploy.yml:19 | Make sure that no untrusted code is executed from a fork. |
| VULNERABILITY | LOW | auth.go:52 | Make sure that setting the "Secure" flag to "false" is safe here. |
| VULNERABILITY | LOW | auth.go:71 | Make sure that setting the "Secure" flag to "false" is safe here. |
| VULNERABILITY | LOW | db.go:220 | Make sure using a dynamically formatted SQL query is safe here. |
| VULNERABILITY | LOW | db.go:342 | Make sure using a dynamically formatted SQL query is safe here. |

## SAST local (referencia)

- **gosec:** 0 issues (7 supresiones `#nosec` justificadas en código: `Secure`
  condicional por flag, Goldmark sin unsafe, nombre de upload generado por el
  servidor, redirects a ruta fija `/login`).
- **govulncheck:** 0 vulnerabilidades alcanzables (Go 1.26.6).
- **Tests de seguridad en Go:** XSS (payloads servidos escapados/omitidos) y
  SQLi (payloads inocuos, tablas intactas) en `security_test.go`.

## Quality Gate

Objetivo: **Sonar Way + cobertura ≥ 80%**. El workflow de CI falla bajo el umbral
(`scripts/check_coverage.sh`) y publica este reporte como artefacto + job summary.

### Estado del Quality Gate en SonarCloud: OK

- new_reliability_rating: OK (1 GT 1)
- new_security_rating: OK (1 GT 1)
- new_maintainability_rating: OK (1 GT 1)
- new_coverage: OK (85.0 LT 80)
- new_duplicated_lines_density: OK (0.0 GT 3)
- new_security_hotspots_reviewed: OK (100.0 LT 100)
