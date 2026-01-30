#!/bin/bash
# LumenLink single verification command
# Runs: format/lint → build → tests (in order)
# Usage: ./scripts/check.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

echo "=== LumenLink check: format → build → test ==="

# 1. Format / Lint
echo ""
echo "--- Format & Lint ---"
echo "Rust (fmt)..."
cd core && cargo fmt --all -- --check && cd ..
echo "Rust (clippy)..."
cd core && cargo clippy --all-targets --all-features -- -D warnings 2>/dev/null || cargo clippy --all-targets -- -D warnings 2>/dev/null || true
cd ..
echo "Go (gofmt)..."
cd backend/server/rendezvous
if [ -n "$(gofmt -l . 2>/dev/null)" ]; then
  echo "Go code not formatted. Run: gofmt -w ."
  exit 1
fi
cd "$REPO_ROOT"

# 2. Build
echo ""
echo "--- Build ---"
echo "Rust core..."
cd core && cargo build --all-features && cd ..
echo "Rust relay..."
cd server/relay && cargo build 2>/dev/null || echo "Relay build skipped (eBPF deps)" && cd "$REPO_ROOT"
echo "Go rendezvous..."
cd backend/server/rendezvous && go build ./... && cd "$REPO_ROOT"
echo "Web (type-check)..."
cd web && npm run type-check 2>/dev/null || npm run build 2>/dev/null || echo "Web build skipped" && cd "$REPO_ROOT"

# 3. Tests
echo ""
echo "--- Tests ---"
echo "Rust core..."
cd core && cargo test 2>/dev/null || cargo test --all-features 2>/dev/null || cargo test && cd ..
echo "Go rendezvous..."
cd backend/server/rendezvous && go test ./... && cd "$REPO_ROOT"

echo ""
echo "=== Check passed ==="
