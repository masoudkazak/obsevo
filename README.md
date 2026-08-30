# Obsevo

A lightweight, self-hosted, MIT-licensed LLM observability platform.

Built with **Go** (backend) + **SvelteKit** (frontend) + **PostgreSQL** + **Redis**.

## Features

- **LLM Tracing** — Hierarchical traces (Trace → Observation) with metadata, model info, token usage, cost
- **Prompt Management** — Versioning, template compilation with `{{variables}}`, active version control
- **Evaluation & Scoring** — Score CRUD, LLM-as-a-judge evaluators, aggregation, analytics
- **Datasets** — Test sets, batch evaluation runs, CSV/JSON import/export
- **Analytics Dashboard** — Cost, latency, token usage, error rate over time with charts
- **SDK Compatibility** — Python/JS SDK compatible API endpoints (`/api/public/`)
- **Auth** — JWT-based authentication, organization/project management, role-based access (viewer/editor/admin)
- **API Keys** — Project-level API keys for SDK ingestion

## Architecture

```
┌─────────────────────────────────────────────────────┐
│                   Obsevo                             │
├─────────────────────────────────────────────────────┤
│  Frontend:  SvelteKit (Tailwind CSS v4, Chart.js)   │
│  Backend:   Go (Chi router + sqlc)                  │
│  Worker:    Goroutines in same Go process           │
├─────────────────────────────────────────────────────┤
│  Database:  PostgreSQL 16 (single DB)               │
│  Cache:     Redis 7 (queue + cache)                 │
│  Storage:   Local disk (Docker volume)              │
└─────────────────────────────────────────────────────┘
```

**3-4 containers**. ~200-400MB RAM. Self-hosted.

## Quick Start

### Prerequisites

- Docker Engine 20.10+
- Docker Compose v2+

### 1. Clone and configure

```bash
git clone <repo-url> obsevo
cd obsevo
cp .env.example .env
```

### 2. Generate secrets

```bash
openssl rand -base64 48  # Use for JWT_SECRET
openssl rand -base64 48  # Use for APP_SECRET_KEY
openssl rand -base64 16  # Use for POSTGRES_PASSWORD
```

### 3. Start

```bash
docker-compose up -d
```

The API is available at `http://localhost:3001` (default).

### 4. Create an account

```bash
curl -X POST http://localhost:3001/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"you@example.com","password":"yourpassword","name":"Your Name"}'
```

Then log in at `http://localhost:3001` in your browser (or use the API).

## Development

### Prerequisites

- Go 1.22+
- Node.js 18+
- PostgreSQL 16
- Redis 7
- sqlc (`go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`)

### Setup

```bash
# Start database and Redis
docker-compose up -d postgres redis

# Install frontend dependencies
cd web && npm install && cd ..

# Generate sqlc code
make sqlc-generate

# Run the API
make run

# In another terminal, run the frontend dev server
cd web && npm run dev
```

### Make Commands

| Command | Description |
|---|---|
| `make build` | Build the Go binary |
| `make run` | Run the application |
| `make test` | Run all tests |
| `make test-coverage` | Run tests with coverage report |
| `make lint` | Run linter |
| `make fmt` | Format code |
| `make vet` | Run go vet |
| `make sqlc-generate` | Regenerate sqlc code |
| `make docker-up` | Start Docker containers |
| `make docker-down` | Stop Docker containers |

## API Endpoints

### Public (no auth)
- `POST /api/auth/register` — Register user
- `POST /api/auth/login` — Login
- `GET /health` — Health check

### Protected (JWT Bearer token)
- `GET/POST /api/organizations` — Organization CRUD
- `GET/POST /api/projects` — Project CRUD
- `GET/POST /api/traces` — Trace list/create
- `GET /api/traces/{id}` — Trace detail with observations
- `GET/POST /api/observations` — Observation list/create
- `GET/POST /api/prompts` — Prompt list/create
- `PUT /api/prompts/{name}` — Update prompt (new version)
- `GET/POST /api/scores` — Score list/create
- `GET /api/analytics` — Analytics summary
- `GET/POST /api/datasets` — Dataset list/create
- `GET/POST /api/datasets/{id}/items` — Dataset items
- `POST /api/datasets/{id}/import` — Import JSON/CSV
- `GET /api/datasets/{id}/export` — Export JSON/CSV
- `GET/POST /api/datasets/{id}/runs` — Dataset runs
- `GET/POST /api/api-keys` — API key management
- `GET /api/members` — Member management
- `GET/PUT /api/profile` — User profile

### SDK-compatible (API key auth via `x-api-key` header)
- `POST /api/public/traces` — Create trace
- `GET /api/public/traces` — List traces
- `GET /api/public/traces/{traceId}` — Get trace
- `POST /api/public/observations` — Create observation
- `POST /api/public/ingestion` — Batch ingestion

## Project Structure

```
obsevo/
├── cmd/server/main.go           # Entry point
├── internal/
│   ├── config/config.go         # Env-based config
│   ├── db/                      # sqlc generated code
│   ├── auth/                    # JWT, password hashing, middleware
│   ├── api/                     # HTTP handlers + router
│   ├── services/                # Business logic
│   ├── queue/                   # Redis queue
│   └── worker/                  # Background processor
├── migrations/                  # PostgreSQL migrations
├── web/                         # SvelteKit frontend
├── tests/                       # Unit & integration tests
├── docker-compose.yml
├── Dockerfile
├── Makefile
└── .env.example
```

## Environment Variables

See `.env.example` for all available configuration. Key variables:

| Variable | Required | Default | Description |
|---|---|---|---|
| `DATABASE_URL` | Yes | — | PostgreSQL connection string |
| `REDIS_URL` | No | `localhost:6379` | Redis address |
| `JWT_SECRET` | Yes | — | Secret for JWT signing |
| `APP_SECRET_KEY` | Yes | — | Application secret key |
| `APP_PORT` | No | `3001` | API server port |
| `APP_ENV` | No | `development` | Environment (development/production) |

## License

MIT
