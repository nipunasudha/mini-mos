#!/usr/bin/env bash
# Steady load for the dashboards: one deposit every 2s until Ctrl-C. Needs curl only.
# Usage: BASE_URL=http://localhost:8080 scripts/demo-traffic.sh
set -euo pipefail

BASE_URL=${BASE_URL:-http://localhost:8080}
CLIENT="traffic-$(date +%s)" # fresh client per run, same idea as demo.sh
AMOUNT=250000
sent=0
failed=0

# Same admin call as demo.sh step 1; without a client there is nothing to deposit to.
code=$(curl -sS -o /dev/null -w '%{http_code}' -X POST "$BASE_URL/v1/admin/clients" \
  -H 'Content-Type: application/json' -d "{\"client_id\":\"$CLIENT\",\"name\":\"Traffic Client\"}")
case "$code" in
2??) echo "client $CLIENT created" ;;
*)
  echo "could not create client $CLIENT (HTTP $code)" >&2
  exit 1
  ;;
esac

trap 'printf "\ntotal: %d deposits, %d non-2xx\n" "$sent" "$failed"; exit 0' INT TERM

while :; do
  sent=$((sent + 1))
  # A refused connection gives "000" and a non-zero curl exit; keep going, steady load is the point.
  code=$(curl -sS -o /dev/null -w '%{http_code}' -X POST "$BASE_URL/v1/clients/$CLIENT/deposits" \
    -H "Idempotency-Key: $CLIENT-$sent" -H 'Content-Type: application/json' \
    -d "{\"amount_cents\":$AMOUNT,\"source\":\"employer-payroll\"}" 2>/dev/null) || true
  case "$code" in 2??) ;; *) failed=$((failed + 1)) ;; esac
  printf '%s %s %s\n' "$(date +%T)" "$code" "$AMOUNT"
  # sleep in the background so the trap fires at once instead of after the sleep.
  sleep 2 &
  wait $! || true
done
