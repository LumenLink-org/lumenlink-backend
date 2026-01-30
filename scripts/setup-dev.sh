#!/bin/bash
# LumenLink Development Environment Setup Script

set -e

echo "🚀 Setting up LumenLink development environment..."

# Check prerequisites
command -v docker >/dev/null 2>&1 || { echo "❌ Docker required. Please install Docker."; exit 1; }
command -v docker-compose >/dev/null 2>&1 || { echo "❌ Docker Compose required. Please install Docker Compose."; exit 1; }
command -v cargo >/dev/null 2>&1 || { echo "❌ Rust required. Install from https://rustup.rs/"; exit 1; }
command -v go >/dev/null 2>&1 || { echo "❌ Go required. Install from https://go.dev/dl/"; exit 1; }
command -v node >/dev/null 2>&1 || { echo "❌ Node.js required. Install from https://nodejs.org/"; exit 1; }

echo "✅ Prerequisites check passed"

# Create .env file if it doesn't exist
if [ ! -f .env ]; then
    echo "Creating .env file..."
    if [ -f env.example ]; then
        cp env.example .env
    elif [ -f .env.example ]; then
        cp .env.example .env
    else
        cat > .env <<EOF
# Database
DB_NAME=lumenlink_dev
DB_USER=lumenlink
DB_PASSWORD=dev_password_change_me
DATABASE_URL=postgres://lumenlink:dev_password_change_me@localhost:5432/lumenlink_dev?sslmode=disable

# Redis
REDIS_URL=redis://localhost:6379

# Development
RUST_LOG=debug
GO_ENV=development
NODE_ENV=development
EOF
    fi
    echo "⚠️  Please edit .env file with your settings"
fi

# Start Docker containers
echo "Starting Docker containers..."
if command -v docker-compose &> /dev/null; then
    docker-compose up -d
    
    # Wait for PostgreSQL
    echo "Waiting for PostgreSQL..."
    timeout=30
    counter=0
    until docker-compose exec -T postgres pg_isready -U lumenlink > /dev/null 2>&1; do
        sleep 1
        counter=$((counter + 1))
        if [ $counter -ge $timeout ]; then
            echo "⚠️  PostgreSQL not ready after $timeout seconds"
            break
        fi
    done
    
    if docker-compose exec -T postgres pg_isready -U lumenlink > /dev/null 2>&1; then
        echo "✅ PostgreSQL is ready"
    fi
else
    echo "⚠️  docker-compose not found, skipping Docker setup"
    echo "   Install Docker Desktop or run: ./scripts/docker-setup.sh"
fi

# Run database migrations (when implemented)
# echo "Running database migrations..."
# cd server/rendezvous
# go run cmd/migrate/main.go up
# cd ../..

# Build Rust core
echo "Building Rust core..."
cd core
cargo build
cd ..

# Install Go dependencies
echo "Installing Go dependencies..."
cd backend/server/rendezvous
go mod download
cd ../../..

# Install web dependencies
echo "Installing web dependencies..."
cd web
npm install
cd ..

echo "✅ Development environment ready!"
echo ""
echo "Next steps:"
echo "  1. Edit .env file with your settings"
echo "  2. Run: docker-compose logs -f"
echo "  3. Start developing!"
echo ""
echo "Useful commands:"
echo "  - Run tests: ./scripts/test-all.sh"
echo "  - Start services: docker-compose up"
echo "  - Stop services: docker-compose down"
