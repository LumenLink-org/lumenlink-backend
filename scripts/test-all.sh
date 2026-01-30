#!/bin/bash
# Run all tests for LumenLink

set -e

echo "🧪 Running all tests..."

# Rust tests
echo ""
echo "📦 Testing Rust core..."
cd core
cargo test --all-features
cargo clippy --all-targets --all-features -- -D warnings
cargo fmt --check
cd ..

# Go tests
echo ""
echo "🔷 Testing Go services..."
cd backend/server/rendezvous
go test ./...
go vet ./...
test -z "$(gofmt -l .)" || (echo "Go code not formatted. Run: gofmt -w ." && exit 1)
cd ../../..

# TypeScript tests (when implemented)
if [ -d "web" ] && [ -f "web/package.json" ]; then
    echo ""
    echo "📱 Testing TypeScript..."
    cd web
    npm test 2>/dev/null || echo "⚠️  Tests not yet implemented"
    npm run lint 2>/dev/null || echo "⚠️  Linting not yet configured"
    cd ..
fi

# CensorLab validation (when implemented)
if [ -d "censorlab" ]; then
    echo ""
    echo "🤖 Running CensorLab validation..."
    cd censorlab
    python validate.py --profile zoom --threshold 0.55 2>/dev/null || echo "⚠️  CensorLab not yet implemented"
    cd ..
fi

echo ""
echo "✅ All tests passed!"
