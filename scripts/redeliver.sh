#!/usr/bin/env bash
# Replay every deposits.allocated event to bill-scheduler, to prove that
# redelivery pays nothing twice.  Usage: scripts/redeliver.sh compose|k8s
set -euo pipefail

GROUP=bill-scheduler
TOPIC=deposits.allocated

cd "$(dirname "$0")/.." # docker compose needs to run from the repo root

# Each mode defines three functions; everything below them is shared:
#   kafka_groups     run kafka-consumer-groups.sh against the broker, for $GROUP
#   scheduler_stop   stop every scheduler instance, so the group has no members
#   scheduler_start  bring the scheduler back (must be safe to call twice)
mode=${1:-}
case "$mode" in
compose)
  TIMEOUT=30 # seconds to wait for the scheduler to catch up again

  kafka_groups() {
    docker compose exec -T kafka /opt/kafka/bin/kafka-consumer-groups.sh \
      --bootstrap-server localhost:9092 --group "$GROUP" "$@"
  }
  scheduler_stop() { docker compose stop bill-scheduler; }
  scheduler_start() { docker compose start bill-scheduler; }
  ;;
k8s)
  TIMEOUT=90 # pods have to be scheduled, start and join the group again
  NS=mos
  APP=bill-scheduler # Argo CD Application, Deployment and pod label all use this name
  replicas=

  kafka_groups() {
    kubectl -n "$NS" exec mos-kafka-dual-role-0 -c kafka -- \
      /opt/kafka/bin/kafka-consumer-groups.sh \
      --bootstrap-server localhost:9092 --group "$GROUP" "$@"
  }
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
  ;;
*)
  echo "usage: $0 compose|k8s" >&2
  exit 2
  ;;
esac

# Whatever goes wrong after this point, do not leave the stack without its scheduler
# (in k8s that also means auto-sync back on). Both start functions are safe to repeat.
trap scheduler_start EXIT

# Kafka refuses to reset a group's offsets while it has active members.
# Stopping the scheduler first makes it leave the group.
scheduler_stop

# If the group has not fully left yet, the reset fails; retry for a few seconds.
reset_ok=false
for _ in $(seq 10); do
  if kafka_groups --reset-offsets --to-earliest --execute --topic "$TOPIC"; then
    reset_ok=true
    break
  fi
  sleep 1
done
if [ "$reset_ok" != true ]; then
  echo "could not reset offsets for $GROUP" >&2
  exit 1 # the EXIT trap starts the scheduler again
fi

trap - EXIT
scheduler_start

# Wait until the scheduler has re-read everything: total LAG (column 6) is 0.
# "-" means no committed offset yet, which is not caught up, so it counts as non-zero.
deadline=$((SECONDS + TIMEOUT))
while ((SECONDS < deadline)); do
  lag=$(kafka_groups --describe 2>/dev/null |
    awk -v t="$TOPIC" '$2 == t { rows++; sum += ($6 == "-" ? 1 : $6) } END { if (rows) print sum }')
  if [ "${lag:-}" = "0" ]; then
    echo "redelivery done: lag is 0"
    exit 0
  fi
  sleep 1
done

echo "timed out after ${TIMEOUT}s waiting for $GROUP lag to reach 0 (last: ${lag:-no data})" >&2
exit 1
