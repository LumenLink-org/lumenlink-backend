#!/bin/bash
# Docker Setup Script for LumenLink
# Sets up and starts the development environment

set -e

echo "🚀 Setting up LumenLink Docker environment..."

# Check if docker-compose is available
if ! command -v docker-compose &> /dev/null; then
    echo "❌ docker-compose not found. Please install Docker Compose."
    exit 1
fi

# Check if Docker is running
if ! docker info > /dev/null 2>&1; then
    echo "❌ Docker is not running. Please start Docker Desktop."
    exit 1
fi

# Create .env file if it doesn't exist
if [ ! -f .env ]; then
    echo "Creating .env file from env.example..."
    if [ -f env.example ]; then
        cp env.example .env
        echo "⚠️  Please edit .env file with your settings"
    else
        echo "⚠️  env.example not found, creating basic .env..."
        cat > .env <<EOF
DB_NAME=lumenlink_dev
DB_USER=lumenlink
DB_PASSWORD=dev_password_change_me
DATABASE_URL=postgres://lumenlink:dev_password_change_me@localhost:5432/lumenlink_dev?sslmode=disable
REDIS_URL=redis://localhost:6379
RENDEZVOUS_PORT=8080
GRAFANA_PASSWORD=admin
RUST_LOG=debug
GO_ENV=development
NODE_ENV=development
EOF
    fi
fi

# Create Prometheus config if it doesn't exist
if [ ! -f infra/prometheus/prometheus.yml ]; then
    echo "Creating Prometheus configuration..."
    mkdir -p infra/prometheus
    # The prometheus.yml should already exist, but create a backup if needed
fi

# Pull latest images
echo "📦 Pulling Docker images..."
docker-compose pull

# Build services
echo "🔨 Building services..."
docker-compose build

# Start services
echo "🚀 Starting services..."
docker-compose up -d

# Wait for PostgreSQL to be ready
echo "⏳ Waiting for PostgreSQL to be ready..."
timeout=30
counter=0
until docker-compose exec -T postgres pg_isready -U lumenlink > /dev/null 2>&1; do
    sleep 1
    counter=$((counter + 1))
    if [ $counter -ge $timeout ]; then
        echo "❌ PostgreSQL failed to start within $timeout seconds"
        docker-compose logs postgres
        exit 1
    fi
done
echo "✅ PostgreSQL is ready"

# Wait for Redis to be ready
echo "⏳ Waiting for Redis to be ready..."
timeout=30
counter=0
until docker-compose exec -T redis redis-cli ping > /dev/null 2>&1; do
    sleep 1
    counter=$((counter + 1))
    if [ $counter -ge $timeout ]; then
        echo "❌ Redis failed to start within $timeout seconds"
        docker-compose logs redis
        exit 1
    fi
done
echo "✅ Redis is ready"

# Run database migrations
echo "🔄 Running database migrations..."
cd backend/server/rendezvous
if command -v go &> /dev/null; then
    export DATABASE_URL="postgres://lumenlink:dev_password_change_me@localhost:5432/lumenlink_dev?sslmode=disable"
    go run cmd/migrate/main.go -command up || echo "⚠️  Migration failed (may already be up to date)"
else
    echo "⚠️  Go not found, skipping migrations. Run manually:"
    echo "   cd backend/server/rendezvous && go run cmd/migrate/main.go -command up"
fi
cd ../../..

# Wait a bit for services to fully start
echo "⏳ Waiting for services to start..."
sleep 5

# Check service health
echo ""
echo "🏥 Checking service health..."
./scripts/docker-health-check.sh || true

echo ""
echo "✅ Docker environment setup complete!"
echo ""
echo "Services are running:"
echo "  - PostgreSQL: localhost:5432"
echo "  - Redis: localhost:6379"
echo "  - Rendezvous API: http://localhost:8080"
echo "  - Prometheus: http://localhost:9090"
echo "  - Grafana: http://localhost:3000 (admin/admin)"
echo ""
echo "Useful commands:"
echo "  - View logs: docker-compose logs -f"
echo "  - Stop services: docker-compose down"
echo "  - Restart services: docker-compose restart"
echo "  - Check health: ./scripts/docker-health-check.sh"
