# LumenLink Backend

Backend services for the LumenLink platform. This folder runs the Go `rendezvous` API plus supporting services (PostgreSQL/TimescaleDB, Redis, Prometheus, Grafana) using Docker Compose.

The backend is responsible for:
- Serving the control-plane API used by clients and the web app
- Managing gateway/relay metadata and discovery signals
- Processing operator metrics for monitoring and health reporting
- Signing configuration packs that clients consume

## Services

- **Rendezvous API** (Go): `http://localhost:8080`
- **PostgreSQL/TimescaleDB**: `localhost:5432`
- **Redis**: `localhost:6379`
- **Prometheus**: `http://localhost:9090`
- **Grafana**: `http://localhost:3000`

## Quick Start (Docker)

```bash
cd backend
docker-compose up -d --build
```

Check health:

```bash
curl http://localhost:8080/health
```

## Configuration

Create a `.env` file in `backend/` (not committed to git). The file is ignored by `.gitignore`.

Minimum required:

```
DB_NAME=lumenlink
DB_USER=lumenlink
DB_PASSWORD=your_secure_password
GRAFANA_PASSWORD=your_secure_grafana_password
LUMENLINK_ALLOW_EPHEMERAL_SIGNING_KEY=true
```

Production recommended (use real signing keys):

```
LUMENLINK_CONFIG_SIGNING_PRIVATE_KEY=base64_private_key
LUMENLINK_CONFIG_SIGNING_PUBLIC_KEY=base64_public_key
```

## API Endpoints

### Health

```
GET /health
```

### API v1

```
POST /api/v1/config
POST /api/v1/attest
POST /api/v1/gateway/status
POST /api/v1/discovery/log
GET  /api/v1/gateways
```

## Common Commands

Rebuild only the backend:

```bash
docker-compose up -d --build rendezvous
```

View logs:

```bash
docker logs -f lumenlink-rendezvous
```

## Notes

- Grafana and Prometheus should not be exposed publicly. Prefer localhost access or SSH tunnel.
- The API is typically exposed via Nginx at `https://api.your-domain`.
