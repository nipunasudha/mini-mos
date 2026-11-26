#!/usr/bin/env bash
# A readable walk through the money flow, for showing on screen. Needs curl and jq.
# The e2e tests are the real proof; this only exits non-zero on an unexpected HTTP status.
set -euo pipefail

BASE_URL=${BASE_URL:-http://localhost:8080}
CLIENT="demo-$(date +%s)" # unique per run, so the demo is repeatable
KEY="demo-dep-$CLIENT"
WAIT=30 # seconds to wait for the scheduler

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

step() { printf '\n== %s\n' "$*"; }

# req <expected-status> <curl args...>: body goes to $tmp/body, headers to $tmp/hdr.
req() {
  local want=$1 code
  shift
  code=$(curl -sS -o "$tmp/body" -D "$tmp/hdr" -w '%{http_code}' "$@")
  if [ "$code" != "$want" ]; then
    echo "unexpected status $code (wanted $want) for: $*" >&2
    cat "$tmp/body" >&2
    exit 1
  fi
}

deposit() { # deposit <idempotency-key>
  req "$2" -X POST "$BASE_URL/v1/clients/$CLIENT/deposits" \
    -H "Idempotency-Key: $1" -H 'Content-Type: application/json' \
    -d '{"amount_cents":250000,"source":"employer-payroll"}'
}

show_bills() {
  req 200 "$BASE_URL/v1/clients/$CLIENT/bills"
  jq -r '.[] | "  \(.pot)\t\(.amount_cents)\tdue \(.due_date)\t\(.status)"' "$tmp/body"
}

# wait_paid <n>: poll until n bills are paid. The scheduler works asynchronously.
wait_paid() {
  local deadline=$((SECONDS + WAIT))
  until req 200 "$BASE_URL/v1/clients/$CLIENT/bills?status=paid" && [ "$(jq length "$tmp/body")" -ge "$1" ]; do
    if ((SECONDS >= deadline)); then
      echo "timed out waiting for $1 paid bill(s)" >&2
      exit 1
    fi
    sleep 1
  done
}

step "1. Create client $CLIENT"
req 201 -X POST "$BASE_URL/v1/admin/clients" -H 'Content-Type: application/json' \
  -d "{\"client_id\":\"$CLIENT\",\"name\":\"Demo Client\"}"
jq -c . "$tmp/body"

step "2. Deposit 250000 twice with the same Idempotency-Key"
deposit "$KEY" 201
echo "first call:  201"
jq -c . "$tmp/body"
deposit "$KEY" 200
echo "second call: 200, $(grep -i '^idempotent-replayed' "$tmp/hdr" | tr -d '\r')"

step "3. Pots (the deposit was applied once)"
req 200 "$BASE_URL/v1/clients/$CLIENT/pots"
jq -r '.[] | "  \(.pot)\t\(.balance_cents)"' "$tmp/body"

step "4. Wait for the scheduler, then show bills: phone paid, rent and power short"
wait_paid 1
show_bills

step "5. Second deposit of 250000 with a new key"
deposit "$KEY-2" 201
jq -c . "$tmp/body"

step "6. Wait, then show bills: all three due bills paid"
wait_paid 3
show_bills
