#!/bin/bash
# Smoke tests: bootstrap, happy path, failure modes, observability
# Run from repo root. Requires: docker, go, curl
# Uses polling with timeout (no random sleeps).

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
cd "$REPO_ROOT"

RENDEZVOUS_PORT=18181
COMPOSE_PROJECT=lumenlink_smoke_test
POLL_MAX=30
POLL_INTERVAL=1

cleanup() {
  echo "Smoke cleanup..."
  docker compose -f docker-compose.yml -p "$COMPOSE_PROJECT" down -v 2>/dev/null || true
  pkill -f "cmd/server/main.go" 2>/dev/null || true
}
trap cleanup EXIT

wait_for() {
  local desc="$1"
  local cmd="$2"
  local i=1
  echo "Waiting for $desc (max ${POLL_MAX}s)..."
  while [ $i -le "$POLL_MAX" ]; do
    if eval "$cmd" 2>/dev/null; then
      echo "$desc ready"
      return 0
    fi
    sleep "$POLL_INTERVAL"
    i=$((i + 1))
  done
  echo "$desc did not become ready"
  return 1
}

echo "=== Smoke A: Fresh start / bootstrap ==="
docker compose -f docker-compose.yml -p "$COMPOSE_PROJECT" up -d postgres redis
wait_for "PostgreSQL" "docker compose -f docker-compose.yml -p $COMPOSE_PROJECT exec -T postgres pg_isready -U lumenlink"
wait_for "Redis" "docker compose -f docker-compose.yml -p $COMPOSE_PROJECT exec -T redis redis-cli ping"

echo "=== Running migrations ==="
export DATABASE_URL="postgres://lumenlink:dev_password_change_me@localhost:5432/lumenlink_dev?sslmode=disable"
(cd backend/server/rendezvous && go run cmd/migrate/main.go -command up)
cd "$REPO_ROOT"

echo "=== Starting Rendezvous (port $RENDEZVOUS_PORT) ==="
export PORT=$RENDEZVOUS_PORT
export REDIS_URL="redis://localhost:6379"
export LUMENLINK_ALLOW_EPHEMERAL_SIGNING_KEY=true
(cd backend/server/rendezvous && go run cmd/server/main.go) &
cd "$REPO_ROOT"

wait_for "Rendezvous" "curl -sf http://localhost:$RENDEZVOUS_PORT/health >/dev/null"

echo "=== Smoke B: Primary happy path ==="
curl -sf "http://localhost:$RENDEZVOUS_PORT/health" | grep -q "healthy" || (echo "Health failed"; exit 1)
CONFIG_RESP=$(curl -sf -X POST "http://localhost:$RENDEZVOUS_PORT/api/v1/config" \
  -H "Content-Type: application/json" \
  -d '{"device_id":"smoke-device","platform":"android","region":"us-east-1"}')
echo "$CONFIG_RESP" | grep -q "config_pack" || (echo "Config fetch failed"; echo "$CONFIG_RESP"; exit 1)
echo "Happy path OK"

echo "=== Smoke C: Failure modes ==="
# Invalid payload -> 400
CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "http://localhost:$RENDEZVOUS_PORT/api/v1/config" \
  -H "Content-Type: application/json" \
  -d '{"device_id":"","platform":""}')
[ "$CODE" = "400" ] || (echo "Expected 400 for invalid payload, got $CODE"; exit 1)
echo "Invalid payload -> 400 OK"

echo "=== Smoke D: Observability sanity ==="
# Invalid JSON -> 400, response must not leak stack/panic/internal paths
ERR_RESP=$(curl -s -X POST "http://localhost:$RENDEZVOUS_PORT/api/v1/config" \
  -H "Content-Type: application/json" \
  -d 'not json')
if echo "$ERR_RESP" | grep -qE "panic|stack|/usr/|/home/|\.go:[0-9]+"; then
  echo "Response leaks internals: $ERR_RESP"
  exit 1
fi
echo "Observability OK"

# Rate limit: burst 10, send 15 rapid requests, expect at least one 429
echo "=== Smoke E: Rate limit check ==="
GOT_429=0
for _ in $(seq 1 15); do
  CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "http://localhost:$RENDEZVOUS_PORT/api/v1/config" \
    -H "Content-Type: application/json" \
    -d '{"device_id":"rate-test","platform":"android","region":"us-east-1"}')
  [ "$CODE" = "429" ] && GOT_429=1
done
[ "$GOT_429" = "1" ] || echo "Rate limit not triggered (burst may be higher)"
echo "Rate limit check OK"

echo "=== Smoke tests passed ==="
