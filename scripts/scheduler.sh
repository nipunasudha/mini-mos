#!/usr/bin/env bash
# Stop or start bill-scheduler on the cluster, for the "watch consumer lag climb" demo.
# Usage: scripts/scheduler.sh stop|start
# Also sourced by redeliver.sh, so the Argo CD dance lives in this one place.
set -euo pipefail

NS=mos
APP=bill-scheduler # Argo CD Application, Deployment and pod label all use this name
replicas=          # remembered by scheduler_stop; scheduler_start falls back to the chart default

scheduler_stop() {
  # Self-heal would put the replicas back within seconds, so pause auto-sync
  # on this one Application first (the ApplicationSet ignores syncPolicy drift).
  kubectl -n argocd patch application "$APP" --type merge -p '{"spec":{"syncPolicy":{"automated":null}}}'
  replicas=$(kubectl -n "$NS" get deploy "$APP" -o jsonpath='{.spec.replicas}')
  [ "${replicas:-0}" -gt 0 ] || replicas=2 # already 0 (a killed earlier run): use the chart default
  kubectl -n "$NS" scale deploy/"$APP" --replicas=0
  # Scaling returns at once; the pods must be gone so they have left the group.
  for _ in $(seq 60); do
    [ -z "$(kubectl -n "$NS" get pods -l app.kubernetes.io/name="$APP" -o name)" ] && return 0
    sleep 1
  done
  echo "$APP pods did not terminate in 60s" >&2
  return 1
}

scheduler_start() {
  # Scale first, then hand control back to Argo CD (same policy the ApplicationSet sets).
  kubectl -n "$NS" scale deploy/"$APP" --replicas="${replicas:-2}"
  kubectl -n argocd patch application "$APP" --type merge \
    -p '{"spec":{"syncPolicy":{"automated":{"prune":true,"selfHeal":true}}}}'
}

# Run the command only when executed, not when redeliver.sh sources the functions.
if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  case "${1:-}" in
  stop) scheduler_stop ;;
  start) scheduler_start ;;
  *)
    echo "usage: $0 stop|start" >&2
    exit 2
    ;;
  esac
fi
