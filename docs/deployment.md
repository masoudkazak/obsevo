# Deployment Guide

## Quick Start (Docker Compose)

### Prerequisites
- Docker Engine 20.10+
- Docker Compose v2+
- 512MB+ RAM available

### 1. Clone and configure

```bash
git clone <repo-url> obsevo
cd obsevo
cp .env.example .env
```

### 2. Edit `.env` with production values

**Required** — the API will refuse to start without these:

```bash
JWT_SECRET=<random-64-char-string>
APP_SECRET_KEY=<random-64-char-string>
POSTGRES_PASSWORD=<strong-password>
```

Generate secrets:
```bash
openssl rand -base64 48
```

### 3. Start services

```bash
docker compose up -d
```

Services will start in order: postgres → redis → api.

### 4. Verify

```bash
curl http://localhost:3001/health
# {"status":"ok","service":"obsevo"}
```

### 5. Apply database migrations

```bash
docker compose exec api ./obsevo migrate up
# Or use the migrate tool directly:
migrate -path migrations -database "postgres://obsevo:<password>@localhost:5432/obsevo?sslmode=disable" up
```

---

## Environment Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `APP_ENV` | No | `production` | Environment: `development`, `staging`, `production` |
| `APP_PORT` | No | `3001` | API server port |
| `APP_SECRET_KEY` | **Yes** | — | Application secret key (min 32 chars) |
| `DATABASE_URL` | No | `postgres://obsevo:obsevo@localhost:5432/obsevo?sslmode=disable` | PostgreSQL connection string |
| `POSTGRES_USER` | No | `obsevo` | PostgreSQL username |
| `POSTGRES_PASSWORD` | No | `obsevo` | PostgreSQL password |
| `POSTGRES_DB` | No | `obsevo` | PostgreSQL database name |
| `POSTGRES_PORT` | No | `5432` | PostgreSQL exposed port |
| `REDIS_URL` | No | `redis://localhost:6379` | Redis connection string |
| `REDIS_PORT` | No | `6379` | Redis exposed port |
| `JWT_SECRET` | **Yes** | — | JWT signing secret (min 32 chars) |
| `JWT_EXPIRY` | No | `24h` | JWT token lifetime |
| `UPLOAD_DIR` | No | `./data/uploads` | File upload directory |
| `MAX_UPLOAD_SIZE` | No | `10485760` | Max upload size in bytes (10MB) |
| `LOG_LEVEL` | No | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `LOG_FORMAT` | No | `json` | Log format: `json`, `text` |

---

## Service Architecture

```
┌─────────────┐     ┌──────────────┐     ┌──────────────┐
│   Web UI    │────▶│   API :3001  │────▶│  PostgreSQL  │
│  (port 80)  │     │   (Go)       │     │  :5432       │
└─────────────┘     └──────┬───────┘     └──────────────┘
                           │
                           ▼
                    ┌──────────────┐
                    │    Redis     │
                    │   :6379      │
                    └──────────────┘
```

| Container | Image | RAM Limit | Purpose |
|---|---|---|---|
| `postgres` | `postgres:16-alpine` | 256MB | Primary database |
| `redis` | `redis:7-alpine` | 128MB | Queue and cache |
| `api` | Built from `Dockerfile` | 256MB | REST API server |

---

## Production Checklist

### Security
- [ ] Set strong `JWT_SECRET` (32+ random characters)
- [ ] Set strong `APP_SECRET_KEY` (32+ random characters)
- [ ] Set strong `POSTGRES_PASSWORD`
- [ ] Set `APP_ENV=production`
- [ ] Don't expose PostgreSQL/Redis ports to public network
- [ ] Use TLS termination (reverse proxy: nginx, Caddy, Traefik)

### Reverse Proxy (nginx example)

```nginx
server {
    listen 443 ssl http2;
    server_name obsevo.example.com;

    ssl_certificate     /etc/ssl/certs/obsevo.pem;
    ssl_certificate_key /etc/ssl/private/obsevo.key;

    location / {
        proxy_pass http://127.0.0.1:3001;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

### Backups

```bash
# Database backup
docker compose exec postgres pg_dump -U obsevo obsevo > backup_$(date +%Y%m%d).sql

# Restore
cat backup_20260824.sql | docker compose exec -T postgres psql -U obsevo obsevo
```

---

## Resource Requirements

| Component | Minimum | Recommended |
|---|---|---|
| RAM (total) | 512MB | 1GB |
| CPU | 1 vCPU | 2 vCPU |
| Disk | 1GB | 10GB |
| Docker | 20.10+ | 24+ |

---

## Scaling Notes

This is a lightweight, single-instance deployment. For larger workloads:

- Increase PostgreSQL `shared_buffers` and `work_mem`
- Add a Redis password and TLS
- Use a managed PostgreSQL service
- Place behind a load balancer with multiple API instances

---

## Updating

```bash
docker compose pull
docker compose build api
docker compose up -d
```

Database migrations run automatically on startup.
