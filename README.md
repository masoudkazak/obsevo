# Obsevo

**Lightweight, self-hosted LLM observability for small teams.**

[![CI](https://github.com/obsevo/obsevo/actions/workflows/ci.yml/badge.svg)](https://github.com/obsevo/obsevo/actions)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](#license)
[![Go 1.22](https://img.shields.io/badge/go-1.22-00ADD8.svg)](https://go.dev/)
[![SvelteKit](https://img.shields.io/badge/SvelteKit-5-FF3E00.svg)](https://kit.svelte.dev/)

Your AI application may be small. Your observability stack doesn't have to be huge.

Obsevo gives you LLM tracing, token and cost tracking, prompt management, evaluations, datasets, and analytics — in a single Go binary with PostgreSQL and Redis. No ClickHouse. No object storage. No separate queue service. Just the workflow you need, running on a $5 VPS.

---

## Why Obsevo?

Teams building AI applications need to trace prompts and completions, track token usage and costs, version prompts, run evaluations, and analyze performance. Most existing solutions require operating a large observability infrastructure just to monitor a relatively small AI application.

Obsevo is designed for teams that want:

- **Full control** over their observability data (self-hosted, your database)
- **Minimal infrastructure** — 3 containers, ~200-400MB RAM
- **The important LLM observability workflow** without the operational overhead
- **Langfuse API compatibility** — migrate with an environment variable change

## Features

| Category | Feature | Description |
|----------|---------|-------------|
| **Observability** | LLM Tracing | Hierarchical traces (Trace → Observation) with metadata, tags, sessions, user tracking |
| | Observations | Spans, generations, and events with model info, input/output, and status |
| | Token Usage | Normalized token tracking (input/output/total) across any LLM provider |
| | Cost Tracking | Server-side cost computation from token usage with built-in model pricing |
| **Prompts** | Prompt Management | Versioned prompts with `{{variable}}` template compilation |
| | Labels & Tags | Organize prompt versions with labels and tags |
| **Evaluation** | Scoring | Score CRUD with numeric, categorical, and boolean data types |
| | LLM-as-a-Judge | Built-in evaluators: exact match, keyword, regex, length, JSON schema, similarity, and LLM judge |
| | Analytics | Cost, latency, token usage, error rate, and trace count over time |
| **Datasets** | Dataset Management | Create, import (JSON/CSV), export, and manage test datasets |
| | Batch Evaluation | Run evaluations across datasets, compare runs, detect regressions |
| **Platform** | Auth & RBAC | JWT authentication, organizations, projects, viewer/editor/admin roles |
| | API Keys | Project-level API keys for SDK ingestion |
| | Rate Limiting | Redis-backed, configurable per-minute limits |
| | Security | CORS, security headers, body size limits, audit logging |
| **Integration** | Langfuse Compatible | Works with official Langfuse Python and JS SDKs |
| | Migration Tool | `cmd/migrate-langfuse` to copy history from Langfuse |

## Architecture

```
┌──────────────────┐
│    SvelteKit     │
│   (Tailwind v4)  │
└────────┬─────────┘
         │
┌────────▼─────────┐
│   Go API Server  │
│  (Chi + sqlc)    │
└──┬──────────┬────┘
   │          │
┌──▼───┐  ┌──▼───────┐
│Postgres│  │  Redis  │
│  16    │  │    7    │
└───────┘  └─────────┘
```

- **3 containers in production**: `api`, `postgres`, `redis`
- **Single Go binary** — API server + background ingestion worker in one process
- **No external dependencies** — no ClickHouse, no S3, no separate queue service
- **Sharded worker** — goroutines process ingestion events concurrently, maintaining per-trace ordering

## Quick Start

### Prerequisites

- Docker Engine 20.10+
- Docker Compose v2+

### 1. Clone & configure

```bash
git clone https://github.com/obsevo/obsevo.git
cd obsevo
cp .env.example .env
```

### 2. Generate secrets

```bash
# Generate each with:
openssl rand -base64 48
```

Set `JWT_SECRET`, `APP_SECRET_KEY`, and `POSTGRES_PASSWORD` in your `.env` file.

### 3. Start

```bash
docker-compose up -d
```

Migrations run automatically on startup.

### 4. Verify

```bash
curl http://localhost:3001/health
# {"status":"ok","service":"obsevo"}
```

### 5. Create your account

```bash
curl -X POST http://localhost:3001/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"you@example.com","password":"yourpassword","name":"Your Name"}'
```

Open **http://localhost:3001** in your browser and log in.

### 6. Create an API key

Navigate to **Settings → API Keys** in the dashboard and create a key. You'll receive a `public_key` (`pk-lf-...`) and `secret_key` (`sk-lf-...`).

## SDK Usage

Obsevo ships lightweight, zero-dependency Python and TypeScript SDKs under `sdk/`. You can also use the **official Langfuse SDKs** directly — the API is compatible.

### Using the official Langfuse SDKs

The official Langfuse Python and JS SDKs work unmodified. Just point them at your Obsevo instance:

```python
# Official Langfuse SDK — works as-is
from langfuse import Langfuse

client = Langfuse(
    public_key="pk-lf-...",
    secret_key="sk-lf-...",
    host="http://localhost:3001"
)

trace = client.trace(name="my-trace", user_id="user-123")
span = trace.span(name="llm-call")
span.end(output={"result": "Hello"})
client.flush()
```

```typescript
// Official Langfuse SDK — works as-is
import { Langfuse } from "langfuse";

const client = new Langfuse({
  publicKey: "pk-lf-...",
  secretKey: "sk-lf-...",
  host: "http://localhost:3001",
});

const trace = client.trace({ name: "my-trace", userId: "user-123" });
const span = trace.span({ name: "llm-call" });
span.end({ output: { result: "Hello" } });
await client.shutdown();
```

### Using Obsevo's lightweight SDKs

If you prefer zero dependencies, Obsevo ships its own SDKs with the same API surface:

```python
# Obsevo SDK (zero dependencies)
from langfuse_light import Langfuse

client = Langfuse(
    public_key="pk-lf-...",
    secret_key="sk-lf-...",
    host="http://localhost:3001"
)

trace = client.trace(name="my-trace", user_id="user-123")
span = trace.span(name="llm-call")
span.end(output={"result": "Hello"})
client.flush()
```

```typescript
// Obsevo SDK (zero dependencies, uses global fetch)
import { Langfuse } from "obsevo";

const client = new Langfuse({
  publicKey: "pk-lf-...",
  secretKey: "sk-lf-...",
  host: "http://localhost:3001",
});

const trace = client.trace({ name: "my-trace", userId: "user-123" });
const span = trace.span({ name: "llm-call" });
span.end({ output: { result: "Hello" } });
await client.shutdown();
```

Both SDKs read from environment variables by default:

```bash
export LANGFUSE_HOST=http://localhost:3001
export LANGFUSE_PUBLIC_KEY=pk-lf-...
export LANGFUSE_SECRET_KEY=sk-lf-...
```

## Migrating from Langfuse

Switching from Langfuse is mostly an environment variable change:

```bash
export LANGFUSE_HOST=http://localhost:3001
export LANGFUSE_PUBLIC_KEY=pk-lf-...
export LANGFUSE_SECRET_KEY=sk-lf-...
```

To copy historical data, use the included migration tool:

```bash
go run ./cmd/migrate-langfuse \
  -source-host https://cloud.langfuse.com \
  -source-public-key pk-lf-OLD -source-secret-key sk-lf-OLD \
  -target-host http://localhost:3001 \
  -target-public-key pk-lf-NEW -target-secret-key sk-lf-NEW \
  -dry-run
```

Drop `-dry-run` when the summary looks right. The tool copies traces, observations, scores, prompts, datasets, and dataset items. See [docs/migration.md](docs/migration.md) for details.

## Configuration

All configuration is via environment variables. See [`.env.example`](.env.example) for the full list.

| Variable | Required | Default | Description |
|---|---|---|---|
| `DATABASE_URL` | No | `postgres://obsevo:obsevo@localhost:5432/obsevo?sslmode=disable` | PostgreSQL connection string |
| `REDIS_URL` | No | `redis://localhost:6379` | Redis connection string |
| `JWT_SECRET` | **Yes** | — | Secret for JWT signing (min 32 chars) |
| `APP_SECRET_KEY` | **Yes** | — | Application secret key (min 32 chars) |
| `APP_PORT` | No | `3001` | API server port |
| `APP_ENV` | No | `development` | `development` / `production` |
| `LOG_LEVEL` | No | `info` | `debug` / `info` / `warn` / `error` |
| `RATE_LIMIT_PER_MINUTE` | No | `0` (disabled) | Per-minute rate limit per client |
| `WORKER_CONCURRENCY` | No | `0` (auto) | Ingestion worker pool size |
| `DB_MAX_CONNS` | No | `20` | PostgreSQL connection pool ceiling |
| `REDIS_PASSWORD` | No | — | Redis auth password (set in production) |

## Self-Hosting

### Resource Requirements

| Component | Minimum | Recommended |
|---|---|---|
| RAM (total) | 512 MB | 1 GB |
| CPU | 1 vCPU | 2 vCPU |
| Disk | 1 GB | 10 GB |

### Production Checklist

```bash
# 1. Set strong secrets
JWT_SECRET=$(openssl rand -base64 48)
APP_SECRET_KEY=$(openssl rand -base64 48)
POSTGRES_PASSWORD=$(openssl rand -base64 16)

# 2. Set APP_ENV=production in .env

# 3. Don't expose PostgreSQL/Redis ports to public network

# 4. Use TLS termination (nginx, Caddy, or Traefik)
```

### Backups

```bash
# Backup
docker compose exec postgres pg_dump -U obsevo obsevo > backup_$(date +%Y%m%d).sql

# Restore
cat backup_YYYYMMDD.sql | docker compose exec -T postgres psql -U obsevo obsevo
```

See [docs/deployment.md](docs/deployment.md) for nginx config and scaling notes.

## Performance

Benchmarked against a 10,000 traces/min workload (4 observations per trace):

| Metric | Value |
|---|---|
| Ingestion latency (p95) | 1.7 ms |
| Worker drain time | 0.2 s |
| Total RAM usage | ~217 MB |
| Storage per event | ~1.1 KB |

The system handles up to 60,000 traces/min with sub-2ms ingestion latency. See [docs/benchmark.md](docs/benchmark.md) for full results and reproduction steps.

## Development

### Prerequisites

- Go 1.22+
- Node.js 18+
- PostgreSQL 16
- Redis 7
- [sqlc](https://sqlc.dev) (`go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`)

### Setup

```bash
# Start infrastructure
docker-compose up -d postgres redis

# Install frontend dependencies
cd web && npm install && cd ..

# Generate sqlc code
make sqlc-generate

# Run the API server
make run

# In another terminal, run the frontend dev server
cd web && npm run dev
```

### Make Commands

| Command | Description |
|---|---|
| `make build` | Build the Go binary |
| `make run` | Run the API server |
| `make test` | Run all tests |
| `make test-coverage` | Run tests with coverage report |
| `make lint` | Run golangci-lint |
| `make fmt` | Format code with gofmt |
| `make vet` | Run go vet |
| `make sqlc-generate` | Regenerate sqlc code |
| `make docker-up` | Start all Docker containers |
| `make docker-down` | Stop all Docker containers |
| `make install-tools` | Install dev dependencies |
| `make benchmark` | Run ingestion benchmarks |

### Project Structure

```
obsevo/
├── cmd/server/main.go            # Entry point
├── cmd/migrate-langfuse/         # Langfuse migration tool
├── cmd/benchmark/                # Load testing tool
├── internal/
│   ├── config/config.go          # Environment-based configuration
│   ├── db/                       # sqlc generated code (do not edit)
│   ├── auth/                     # JWT, password hashing, middleware
│   ├── api/                      # HTTP handlers + Chi router
│   ├── services/                 # Business logic
│   ├── evaluator/                # Pluggable evaluator framework
│   ├── queue/                    # Redis queue
│   ├── worker/                   # Background ingestion processor
│   └── log/                      # Structured logging
├── migrations/                   # PostgreSQL migrations
├── sdk/
│   ├── python/                   # Python SDK (langfuse_light)
│   └── typescript/               # TypeScript SDK
├── web/                          # SvelteKit frontend
├── tests/                        # Unit & integration tests
├── docs/                         # Deployment, migration, benchmark docs
├── docker-compose.yml
├── Dockerfile
├── Makefile
└── .env.example
```

## API Reference

### Authentication

| Method | Header | Description |
|---|---|---|
| User session | `Authorization: Bearer <jwt>` | For dashboard & management API |
| SDK ingestion | `x-api-key: <key>` | For programmatic trace ingestion |

### Endpoints

<details>
<summary><strong>Auth</strong></summary>

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/auth/register` | Register a new user |
| `POST` | `/api/auth/login` | Login and receive JWT |
| `GET` | `/health` | Health check |

</details>

<details>
<summary><strong>Traces & Observations</strong></summary>

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/traces` | List traces |
| `POST` | `/api/traces` | Create a trace |
| `GET` | `/api/traces/{id}` | Get trace with observations |
| `POST` | `/api/observations` | Create an observation |
| `POST` | `/api/public/traces` | Create trace (SDK) |
| `GET` | `/api/public/traces` | List traces (SDK) |
| `GET` | `/api/public/traces/{traceId}` | Get trace (SDK) |
| `DELETE` | `/api/public/traces/{traceId}` | Delete trace (SDK) |
| `POST` | `/api/public/observations` | Create observation (SDK) |
| `GET` | `/api/public/observations` | List observations (SDK) |
| `GET` | `/api/public/observations/{observationId}` | Get observation (SDK) |
| `POST` | `/api/public/ingestion` | Batch ingestion (SDK) |

</details>

<details>
<summary><strong>Prompts</strong></summary>

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/prompts` | List prompts |
| `POST` | `/api/prompts` | Create prompt |
| `PUT` | `/api/prompts/{name}` | Update prompt (new version) |

</details>

<details>
<summary><strong>Evaluation & Scoring</strong></summary>

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/scores` | List scores |
| `POST` | `/api/scores` | Create a score |
| `GET` | `/api/analytics` | Analytics summary |
| `POST` | `/api/public/scores` | Create score (SDK) |
| `GET` | `/api/public/scores` | List scores (SDK) |

</details>

<details>
<summary><strong>Evaluators</strong></summary>

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/evaluators` | List evaluator configs |
| `POST` | `/api/evaluators` | Create evaluator config |
| `DELETE` | `/api/evaluators/{id}` | Delete evaluator config |
| `POST` | `/api/evaluators/{id}/run` | Run evaluator on traces |
| `GET` | `/api/evaluators/runs` | List evaluator runs |

</details>

<details>
<summary><strong>Datasets</strong></summary>

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/datasets` | List datasets |
| `POST` | `/api/datasets` | Create a dataset |
| `GET` | `/api/datasets/{id}` | Get dataset |
| `DELETE` | `/api/datasets/{id}` | Delete dataset |
| `GET` | `/api/datasets/{id}/items` | List items |
| `POST` | `/api/datasets/{id}/items` | Add item |
| `POST` | `/api/datasets/{id}/import` | Import JSON/CSV |
| `GET` | `/api/datasets/{id}/export` | Export JSON/CSV |
| `GET` | `/api/datasets/{id}/runs` | List runs |
| `POST` | `/api/datasets/{id}/runs` | Create a run |
| `GET` | `/api/datasets/{id}/runs/{runId}` | Get run |
| `POST` | `/api/datasets/{id}/runs/{runId}/items` | Add run item |
| `GET` | `/api/datasets/{id}/runs/{runId}/items` | List run items |
| `GET` | `/api/datasets/{id}/runs/{runId}/export` | Export run results |

</details>

<details>
<summary><strong>Management</strong></summary>

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/organizations` | List organizations |
| `POST` | `/api/organizations` | Create organization |
| `GET` | `/api/projects` | List projects |
| `POST` | `/api/projects` | Create project |
| `GET` | `/api/api-keys` | List API keys |
| `POST` | `/api/api-keys` | Create API key |
| `GET` | `/api/members` | List members |
| `GET` | `/api/profile` | Get profile |
| `PUT` | `/api/profile` | Update profile |
| `GET` | `/api/model-prices` | List model prices |
| `POST` | `/api/model-prices` | Create model price |

</details>

Full OpenAPI 3.0 spec: [docs/openapi.yaml](docs/openapi.yaml)

## Contributing

Contributions are welcome. Please open an issue first to discuss what you'd like to change.

1. Fork the repo
2. Create a feature branch (`git checkout -b feat/my-feature`)
3. Make your changes
4. Run `make lint && make test` to verify
5. Open a pull request

## License

MIT
