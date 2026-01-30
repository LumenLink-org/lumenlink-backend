#!/bin/bash
# Smoke test: Server fails gracefully when DB is down
# Run from repo root. Requires: go
# This does NOT start Docker - expects DB to be unavailable.

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
cd "$REPO_ROOT"

echo "=== Smoke: DB down - server fails gracefully ==="
# Start server with invalid DATABASE_URL, capture stderr, expect exit within 60s
export DATABASE_URL="postgres://invalid:invalid@127.0.0.1:19999/nonexistent?sslmode=disable"
export REDIS_URL="redis://127.0.0.1:19998"
export LUMENLINK_ALLOW_EPHEMERAL_SIGNING_KEY=true
export PORT=18282

# Run server in background, capture output
LOG=$(mktemp)
(cd backend/server/rendezvous && timeout 45 go run cmd/server/main.go 2>"$LOG") || true
OUTPUT=$(cat "$LOG")
rm -f "$LOG"

# Should fail (not hang)
# Output should NOT contain panic or stack trace
if echo "$OUTPUT" | grep -qE "panic|runtime error|\.go:[0-9]+"; then
  echo "Server crashed with panic/stack - not graceful"
  echo "$OUTPUT"
  exit 1
fi
# Should contain clear error about DB
if ! echo "$OUTPUT" | grep -qiE "database|connection|failed|ping"; then
  echo "Expected DB-related error message, got:"
  echo "$OUTPUT"
  exit 1
fi
echo "DB-down: server failed gracefully with clear error"
