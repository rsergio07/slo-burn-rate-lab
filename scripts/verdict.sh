#!/usr/bin/env bash
# Prints the SLO verdict over the last N seconds. SLO: 99% of checkouts under 500ms.
set -euo pipefail
window="${1:-60}s"
prom="${PROMETHEUS_URL:-http://localhost:30090}"

query() {
  curl -fsS -G "$prom/api/v1/query" --data-urlencode "query=$1" |
    jq -r '.data.result[0].value[1] // "NaN"'
}

inside=$(query "sum(rate(customer_checkout_duration_seconds_bucket{le=\"0.5\"}[$window])) / sum(rate(customer_checkout_duration_seconds_count[$window]))")
p99=$(query "histogram_quantile(0.99, sum by (le) (rate(customer_checkout_duration_seconds_bucket[$window])))")
ok=$(query "sum(rate(customer_checkouts_total{result=\"ok\"}[$window])) / sum(rate(customer_checkouts_total[$window]))")
gave_up=$(query "sum(increase(customer_checkouts_total{result=\"gave_up\"}[$window]))")

if [ "$inside" = "NaN" ]; then
  echo "no traffic data in the last $window; is the customers deployment running?" >&2
  exit 1
fi

awk -v inside="$inside" -v p99="$p99" -v ok="$ok" -v gave_up="$gave_up" -v window="$window" 'BEGIN {
  printf "window            %s\n", window
  printf "inside SLO        %.2f%%\n", inside * 100
  printf "p99 latency       %.0f ms\n", p99 * 1000
  printf "completed         %.2f%%\n", ok * 100
  printf "customers lost    %.0f\n", gave_up
  printf "SLO (99%% < 500ms) %s\n", (inside >= 0.99 ? "PASS" : "FAIL")
}'
