#!/usr/bin/env bash
# Gate de cobertura: falla si el total es menor al umbral (default 80%).
set -euo pipefail
cd "$(dirname "$0")/.."

THRESHOLD="${COVERAGE_THRESHOLD:-80}"

rm -f coverage.out coverage.html
go test -race -count=1 -covermode=atomic -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html

TOTAL=$(go tool cover -func=coverage.out | tail -1 | awk '{print $3}' | tr -d '%')
echo "cobertura total: ${TOTAL}% (umbral: ${THRESHOLD}%)"

if awk -v t="$TOTAL" -v th="$THRESHOLD" 'BEGIN { exit !(t + 0 >= th + 0) }'; then
	echo "OK: cobertura >= ${THRESHOLD}%"
else
	echo "FALLO: cobertura ${TOTAL}% < ${THRESHOLD}%"
	exit 1
fi
