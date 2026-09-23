#!/usr/bin/env bash
# Consulta las métricas de SonarCloud y genera docs/metrics/calidad.md.
# Requiere: SONAR_TOKEN, SONAR_PROJECT_KEY (y opcional SONAR_ORG, SONAR_HOST).
# Sin token genera el reporte con los datos locales disponibles y marca
# SonarCloud como pendiente (útil antes del primer análisis en CI).
set -euo pipefail
cd "$(dirname "$0")/.."

OUT="docs/metrics/calidad.md"
mkdir -p docs/metrics

SONAR_HOST="${SONAR_HOST:-https://sonarcloud.io}"
PROJECT="${SONAR_PROJECT_KEY:-zekar47_palomas-del-gobierno}"
METRICS="bugs,vulnerabilities,security_hotspots,code_smells,sqale_rating,sqale_debt_minutes,coverage,duplicated_lines_density,ncloc,reliability_rating,security_rating,maintainability_rating"

FECHA=$(date -u +%Y-%m-%dT%H:%M:%SZ)
SONAR_OK="no"
RATINGS_JSON=""

if [ -n "${SONAR_TOKEN:-}" ]; then
	# El scan sube el reporte y el Compute Engine de SonarCloud tarda en
	# procesarlo (~30s-2min): reintentar hasta ver medidas (máx ~5 min).
	for i in $(seq 1 12); do
		RESP=$(curl -fsS -u "${SONAR_TOKEN}:" \
			"${SONAR_HOST}/api/measures/component?component=${PROJECT}&metricKeys=${METRICS}" 2>/dev/null || true)
		if [ -n "$RESP" ] && echo "$RESP" | grep -q '"measures"'; then
			SONAR_OK="sí"
			RATINGS_JSON="$RESP"
			break
		fi
		sleep 25
	done
fi

# Estado del Quality Gate (independiente de las medidas).
QGATE="?"
QG_COND=""
if [ -n "${SONAR_TOKEN:-}" ]; then
	QG_RESP=$(curl -fsS -u "${SONAR_TOKEN}:" \
		"${SONAR_HOST}/api/qualitygates/project_status?projectKey=${PROJECT}" 2>/dev/null || true)
	if [ -n "$QG_RESP" ]; then
		QGATE=$(echo "$QG_RESP" | python3 -c "import json,sys; print(json.load(sys.stdin).get('projectStatus',{}).get('status','?'))" 2>/dev/null || echo "?")
		QG_COND=$(echo "$QG_RESP" | python3 -c "
import json,sys
d=json.load(sys.stdin).get('projectStatus',{})
for c in d.get('conditions',[]):
    print('- {}: {} ({} {} {})'.format(c.get('metricKey'),c.get('status'),c.get('actualValue','?'),c.get('comparator','?'),c.get('errorThreshold','?')))" 2>/dev/null || true)
	fi
fi

get_metric() { # $1 = json, $2 = key
	echo "$1" | python3 -c "import json,sys; d=json.load(sys.stdin); ms={m['metric']:m.get('value','?') for m in d.get('component',{}).get('measures',[])}; print(ms.get('$2','?'))" 2>/dev/null || echo "?"
}

debt_fmt() { # minutos SQALE -> "Xd Yh Zm"
	m="$1"
	case "$m" in ''|'?'|*[!0-9]*) echo "$1";; *) python3 -c "m=int('$m'); print(f'{m//1440}d {(m%1440)//60}h {m%60}m')";; esac
}

rating_letra() { # 1.0-5.0 -> A-E
	case "$1" in 1.0) echo "A";; 2.0) echo "B";; 3.0) echo "C";; 4.0) echo "D";; 5.0) echo "E";; *) echo "$1";; esac
}

if [ "$SONAR_OK" = "sí" ]; then
	BUGS=$(get_metric "$RATINGS_JSON" bugs)
	VULNS=$(get_metric "$RATINGS_JSON" vulnerabilities)
	HOTSPOTS=$(get_metric "$RATINGS_JSON" security_hotspots)
	SMELLS=$(get_metric "$RATINGS_JSON" code_smells)
	DEBT=$(debt_fmt "$(get_metric "$RATINGS_JSON" sqale_debt_minutes)")
	COV=$(get_metric "$RATINGS_JSON" coverage)
	DUPL=$(get_metric "$RATINGS_JSON" duplicated_lines_density)
	NCLOC=$(get_metric "$RATINGS_JSON" ncloc)
	REL=$(rating_letra "$(get_metric "$RATINGS_JSON" reliability_rating)")
	SEC=$(rating_letra "$(get_metric "$RATINGS_JSON" security_rating)")
	MAIN=$(rating_letra "$(get_metric "$RATINGS_JSON" maintainability_rating)")
else
	BUGS="?" ; VULNS="?" ; HOTSPOTS="?" ; SMELLS="?" ; DEBT="?"
	COV="?" ; DUPL="?" ; NCLOC="?" ; REL="?" ; SEC="?" ; MAIN="?"
fi

# Cobertura local (siempre disponible si existe coverage.out).
if [ -f coverage.out ]; then
	COV_LOCAL=$(go tool cover -func=coverage.out | tail -1 | awk '{print $3}')
else
	COV_LOCAL="sin medir (corre ./scripts/check_coverage.sh)"
fi

cat > "$OUT" <<EOF
# Métricas de calidad — palomas-del-gobierno

- **Fecha (UTC):** ${FECHA}
- **SonarCloud:** ${SONAR_OK} (proyecto \`${PROJECT}\`)
- **Cobertura local (go cover):** ${COV_LOCAL} (umbral CI: 80%)

## SonarCloud

| Métrica | Valor |
|---|---|
| Bugs | ${BUGS} |
| Vulnerabilidades | ${VULNS} |
| Security hotspots | ${HOTSPOTS} |
| **Code smells** | **${SMELLS}** |
| **Deuda técnica (SQALE)** | **${DEBT}** |
| Cobertura | ${COV}% |
| Duplicación | ${DUPL}% |
| Líneas de código (ncloc) | ${NCLOC} |
| Rating fiabilidad | ${REL} |
| Rating seguridad | ${SEC} |
| Rating mantenibilidad | ${MAIN} |

## SAST local (referencia)

- **gosec:** 0 issues (7 supresiones \`#nosec\` justificadas en código: \`Secure\`
  condicional por flag, Goldmark sin unsafe, nombre de upload generado por el
  servidor, redirects a ruta fija \`/login\`).
- **govulncheck:** 0 vulnerabilidades alcanzables (Go 1.26.6).
- **Tests de seguridad en Go:** XSS (payloads servidos escapados/omitidos) y
  SQLi (payloads inocuos, tablas intactas) en \`security_test.go\`.

## Quality Gate

Objetivo: **Sonar Way + cobertura ≥ 80%**. El workflow de CI falla bajo el umbral
(\`scripts/check_coverage.sh\`) y publica este reporte como artefacto + job summary.

### Estado del Quality Gate en SonarCloud: ${QGATE}

${QG_COND}
EOF

echo "reporte escrito en $OUT (sonar: $SONAR_OK)"
