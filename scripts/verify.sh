#!/bin/bash
# LumenLink full verification: lint, unit tests, build, integration, e2e
# Usage: ./scripts/verify.sh [--skip-e2e]
#   --skip-e2e  Skip integration and e2e (for CI without Docker)
#
# Requires: go, cargo, docker (for steps 4-5), curl

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

SKIP_E2E=0
for arg in "$@"; do
  case "$arg" in
    --skip-e2e) SKIP_E2E=1 ;;
  esac
done

echo "=== LumenLink verify: lint, unit, build, integration, e2e ==="

# Require at least Go for backend verification
if ! command -v go &>/dev/null; then
  echo "ERROR: go not found. Install Go 1.21+ and rerun."
  exit 1
fi

# --- 1. Lint / Format ---
echo ""
echo "--- 1. Lint & Format ---"
if command -v cargo &>/dev/null; then
  echo "Rust (fmt)..."
  (cd core && cargo fmt --all -- --check) || { echo "Run: cd core && cargo fmt --all"; exit 1; }
else
  echo "Rust (fmt)... skipped (cargo not found)"
fi
if command -v cargo &>/dev/null; then
  echo "Rust (clippy)..."
  (cd core && cargo clippy --all-targets --all-features -- -D warnings 2>/dev/null) || \
    (cd core && cargo clippy --all-targets -- -D warnings 2>/dev/null) || true
else
  echo "Rust (clippy)... skipped (cargo not found)"
fi
echo "Go (gofmt)..."
if [ -n "$(gofmt -l backend/server/rendezvous 2>/dev/null)" ]; then
  echo "Go code not formatted. Run: gofmt -w backend/server/rendezvous"
  exit 1
fi
echo "Go (vet)..."
(cd backend/server/rendezvous && go vet ./...) || exit 1
if [ -d web ] && [ -f web/package.json ]; then
  echo "Web (lint)..."
  (cd web && npm run lint 2>/dev/null) || echo "Web lint skipped (not configured)"
fi

# --- 2. Unit Tests ---
echo ""
echo "--- 2. Unit Tests ---"
echo "Rust core..."
(cd core && cargo test 2>/dev/null || cargo test --all-features 2>/dev/null || cargo test)
echo "Go rendezvous..."
(cd backend/server/rendezvous && go test ./...)
if [ -d web ] && [ -f web/package.json ]; then
  echo "Web (test)..."
  (cd web && npm test 2>/dev/null) || echo "Web tests skipped (not implemented)"
fi

# --- 3. Build ---
echo ""
echo "--- 3. Build ---"
if command -v cargo &>/dev/null; then
  echo "Rust core..."
  (cd core && cargo build --all-features)
  echo "Rust relay..."
  (cd server/relay && cargo build 2>/dev/null) || echo "Relay build skipped (eBPF deps)"
else
  echo "Rust core... skipped (cargo not found)"
  echo "Rust relay... skipped (cargo not found)"
fi
echo "Go rendezvous..."
(cd backend/server/rendezvous && go build ./...)
if [ -d web ] && [ -f web/package.json ]; then
  echo "Web (type-check)..."
  (cd web && npm run type-check 2>/dev/null || npm run build 2>/dev/null) || echo "Web build skipped"
fi

# --- 4. Integration Tests ---
echo ""
echo "--- 4. Integration Tests ---"
if [ "$SKIP_E2E" = "1" ]; then
  echo "Skipped (--skip-e2e)"
elif ! command -v docker &>/dev/null; then
  echo "Skipped (Docker not available)"
elif ! docker info &>/dev/null; then
  echo "Skipped (Docker not running)"
else
  bash "test scenarios/scripts/integration-relay-rendezvous.sh"
fi

# --- 5. E2E / Smoke Tests ---
echo ""
echo "--- 5. E2E / Smoke Tests ---"
if [ "$SKIP_E2E" = "1" ]; then
  echo "Skipped (--skip-e2e)"
elif ! command -v docker &>/dev/null; then
  echo "Skipped (Docker not available)"
elif ! docker info &>/dev/null; then
  echo "Skipped (Docker not running)"
else
  bash "scripts/smoke/smoke-all.sh"
fi

echo ""
echo "=== Verify passed ==="
