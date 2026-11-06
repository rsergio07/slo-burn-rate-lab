#!/usr/bin/env bash
# Prints the error ratio per window and the checkout alerts that are active.
set -euo pipefail
prom="${PROMETHEUS_URL:-http://localhost:30090}"
q() { curl -fsS -G "$prom/api/v1/query" --data-urlencode "query=$1" | jq -r '.data.result[0].value[1] // "NaN"'; }
awk -v a="$(q slo:error_ratio:rate30s)" -v b="$(q slo:error_ratio:rate2m)" 'BEGIN { printf "error ratio 30s   %.2f%%\nerror ratio 2m    %.2f%%\n", a*100, b*100 }'
active=$(curl -fsS "$prom/api/v1/alerts" | jq -r '.data.alerts[] | select(.labels.slo=="checkout") | "\(.state)  \(.labels.alertname)  severity=\(.labels.severity)"')
echo "alerts:"
echo "${active:-  none}"
