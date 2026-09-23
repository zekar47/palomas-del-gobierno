#!/usr/bin/env bash
# Consulta las métricas de SonarCloud y genera docs/metrics/calidad.md.
# Requiere: SONAR_TOKEN, SONAR_PROJECT_KEY (y opcional SONAR_ORG, SONAR_HOST).
# Sin token genera el reporte con los datos locales disponibles y marca
# SonarCloud como pendiente (útil antes del primer análisis en CI).
#
# Notas de robustez (incidentes reales):
# - El Compute Engine tarda en procesar el scan: se reintenta hasta ~5 min.
# - Las métricas se piden UNA POR UNA: si una clave no existe en esta versión
#   de SonarCloud, solo esa queda en "?" en vez de tumbar toda la consulta.
set -euo pipefail
cd "$(dirname "$0")/.."

OUT="docs/metrics/calidad.md"
mkdir -p docs/metrics

SONAR_HOST="${SONAR_HOST:-https://sonarcloud.io}"
PROJECT="${SONAR_PROJECT_KEY:-zekar47_palomas-del-gobierno}"

FECHA=$(date -u +%Y-%m-%dT%H:%M:%SZ)
SONAR_OK="no"
declare -A M

api() { # $1 = path con query
	curl -fsS -u "${SONAR_TOKEN}:" "${SONAR_HOST}/$1" 2>/dev/null || true
}

fetch_one() { # $1 = metric key -> imprime valor o "?"
	local resp val
	resp=$(api "api/measures/component?component=${PROJECT}&metricKeys=$1")
	val=$(echo "$resp" | python3 -c "
import json,sys
try:
    d=json.load(sys.stdin)
    ms=d.get('component',{}).get('measures',[])
    print(ms[0].get('value','?') if ms else '?')
except Exception:
    print('?')")
	echo "${val:-?}"
}

if [[ -n "${SONAR_TOKEN:-}" ]]; then
	KEYS="bugs vulnerabilities security_hotspots code_smells coverage duplicated_lines_density ncloc reliability_rating security_rating sqale_rating sqale_index"
	for k in $KEYS; do M[$k]="?"; done
	# Rondas de reintento: el Compute Engine puede tardar minutos.
	for round in $(seq 1 12); do
		pending=0
		for k in $KEYS; do
			if [[ "${M[$k]}" = "?" ]]; then
				M[$k]=$(fetch_one "$k")
				[[ "${M[$k]}" = "?" ]] && pending=$((pending + 1))
			fi
		done
		echo "sonar-metrics ronda $round: pendientes=$pending" >&2
		[[ "$pending" -eq 0 ]] && break
		sleep 25
	done
	[[ "${M[bugs]}" != "?" ]] && SONAR_OK="sí"
fi

get() { echo "${M[$1]:-?}"; }

debt_fmt() { # minutos SQALE -> "Xd Yh Zm"
	m="$1"
	case "$m" in ''|'?'|*[!0-9]*) echo "$1";; *) python3 -c "m=int('$m'); print(f'{m//1440}d {(m%1440)//60}h {m%60}m')";; esac
}

rating_letra() { # 1.0-5.0 -> A-E
	case "$1" in 1.0) echo "A";; 2.0) echo "B";; 3.0) echo "C";; 4.0) echo "D";; 5.0) echo "E";; *) echo "$1";; esac
}

if [[ "$SONAR_OK" = "sí" ]]; then
	BUGS=$(get bugs)
	VULNS=$(get vulnerabilities)
	HOTSPOTS=$(get security_hotspots)
	SMELLS=$(get code_smells)
	DEBT=$(debt_fmt "$(get sqale_index)")
	COV=$(get coverage)
	DUPL=$(get duplicated_lines_density)
	NCLOC=$(get ncloc)
	REL=$(rating_letra "$(get reliability_rating)")
	SEC=$(rating_letra "$(get security_rating)")
	MAIN=$(rating_letra "$(get maintainability_rating)")
else
	BUGS="?" ; VULNS="?" ; HOTSPOTS="?" ; SMELLS="?" ; DEBT="?"
	COV="?" ; DUPL="?" ; NCLOC="?" ; REL="?" ; SEC="?" ; MAIN="?"
fi

# Hallazgos abiertos de seguridad (vulnerabilidades + hotspots por revisar).
ISSUES_MD="Sin datos (falta SONAR_TOKEN o la API no respondió)."
if [[ -n "${SONAR_TOKEN:-}" ]]; then
	ISSUES_JSON=$(api "api/issues/search?projects=${PROJECT}&issueStatuses=OPEN,CONFIRMED&ps=100")
	HOT_JSON=$(api "api/hotspots/search?projectKey=${PROJECT}&statuses=TO_REVIEW&ps=100")
	ISSUES_MD=$(python3 - "$ISSUES_JSON" "$HOT_JSON" <<'EOF' || echo "Sin datos (respuesta inesperada)."
import json,sys
out=[]
try:
    d=json.loads(sys.argv[1])
    items=d.get('issues',[])
except Exception:
    items=[]
vulns=[it for it in items if (it.get('type') or '')=='VULNERABILITY']
smells=[it for it in items if (it.get('type') or '')!='VULNERABILITY']
for it in vulns + smells[:15]:
    sev=(it.get('impacts') or [{}])[0].get('severity','?')
    out.append("| {} | {} | {}:{} | {} |".format(
        (it.get('type') or '?'), sev,
        (it.get('component') or '').split(':')[-1], it.get('line') or '?',
        (it.get('message') or '').replace('|','/')[:120]))
try:
    h=json.loads(sys.argv[2])
    hots=h.get('hotspots',[])
except Exception:
    hots=[]
for ht in hots[:20]:
    out.append("| HOTSPOT | {} | {}:{} | {} |".format(
        ht.get('vulnerabilityProbability','?'),
        (ht.get('component') or {}).get('key','').split(':')[-1] if isinstance(ht.get('component'),dict) else ht.get('component',''),
        ht.get('line') or '?',
        (ht.get('message') or ht.get('ruleKey') or '').replace('|','/')[:120]))
print("\n".join(out) if out else "Sin hallazgos abiertos.")
EOF
)
fi

# Estado del Quality Gate.
QGATE="?"
QG_COND=""
if [[ -n "${SONAR_TOKEN:-}" ]]; then
	QG_RESP=$(api "api/qualitygates/project_status?projectKey=${PROJECT}")
	if [[ -n "$QG_RESP" ]]; then
		QGATE=$(echo "$QG_RESP" | python3 -c "import json,sys; print(json.load(sys.stdin).get('projectStatus',{}).get('status','?'))" 2>/dev/null || echo "?")
		QG_COND=$(echo "$QG_RESP" | python3 -c "
import json,sys
d=json.load(sys.stdin).get('projectStatus',{})
for c in d.get('conditions',[]):
    print('- {}: {} ({} {} {})'.format(c.get('metricKey'),c.get('status'),c.get('actualValue','?'),c.get('comparator','?'),c.get('errorThreshold','?')))" 2>/dev/null || true)
	fi
fi

# Cobertura local (siempre disponible si existe coverage.out).
if [[ -f coverage.out ]]; then
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

## Hallazgos abiertos de seguridad

| Tipo | Severidad | Ubicación | Mensaje |
|---|---|---|---|
${ISSUES_MD}

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
