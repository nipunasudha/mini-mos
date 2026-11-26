#!/usr/bin/env bash
# Replay every deposits.allocated event to bill-scheduler, to prove that
# redelivery pays nothing twice.  Usage: scripts/redeliver.sh compose|k8s
set -euo pipefail

GROUP=bill-scheduler
TOPIC=deposits.allocated
TIMEOUT=30 # seconds to wait for the scheduler to catch up again

cd "$(dirname "$0")/.." # docker compose needs to run from the repo root

mode=${1:-}
case "$mode" in
compose) ;;
k8s)
  echo "k8s: not implemented until M2" >&2
  exit 1
  ;;
*)
  echo "usage: $0 compose|k8s" >&2
  exit 2
  ;;
esac

kafka_groups() {
  docker compose exec -T kafka /opt/kafka/bin/kafka-consumer-groups.sh \
    --bootstrap-server localhost:9092 --group "$GROUP" "$@"
}

# Kafka refuses to reset a group's offsets while it has active members.
# Stopping the scheduler first makes it leave the group.
docker compose stop bill-scheduler

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
  docker compose start bill-scheduler # do not leave the stack without its scheduler
  exit 1
fi

docker compose start bill-scheduler

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
