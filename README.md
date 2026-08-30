<h1 align="center">Obsevo</h1>

<p align="center">
  <strong>Open-source LLM observability. Your data stays yours.</strong>
</p>

<p align="center">
  <a href="#quick-start">Quick Start</a> ·
  <a href="#sdk-usage">SDK Usage</a> ·
  <a href="#api-reference">API</a> ·
  <a href="#migrating-from-langfuse">Migration</a> ·
  <a href="#performance">Performance</a> ·
  <a href="#development">Development</a> ·
  <a href="#license">License</a>
</p>

---

Obsevo is a lightweight alternative to [Langfuse](https://langfuse.com) built for teams that want full control over their LLM data. Trace every prompt, completion, and tool call. Version your prompts. Run evaluations. Ship with confidence.

**3 containers. ~300MB RAM. Under 3 minutes to deploy.**

## Features

| Feature | Description |
|---|---|
| **LLM Tracing** | Hierarchical traces (Trace → Observation) with metadata, model info, token usage, and cost tracking |
| **Prompt Management** | Versioned prompts with `{{variable}}` template compilation and active version control |
| **Evaluation & Scoring** | Score CRUD, LLM-as-a-judge evaluators, aggregation, and analytics |
| **Datasets** | Test sets, batch evaluation runs, CSV/JSON import & export |
| **Analytics Dashboard** | Cost, latency, token usage, error rate over time with interactive charts |
| **SDK Compatibility** | Drop-in Python & JS SDKs with the same API surface as Langfuse |
| **Auth & RBAC** | JWT authentication, organizations, projects, and role-based access (viewer/editor/admin) |
| **API Keys** | Project-level API keys for SDK ingestion |

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
              ┌────────▼──┐  ┌───▼────────┐
              │ PostgreSQL │  │   Redis    │
              │    16      │  │     7      │
              └────────────┘  └────────────┘
```

- **4 containers**: `api`, `web`, `postgres`, `redis`
- **No external dependencies**: no ClickHouse, no S3, no object storage
- **Worker**: goroutines inside the same Go process (no separate queue service)

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
# JWT_SECRET
openssl rand -base64 48

# APP_SECRET_KEY
openssl rand -base64 48

# POSTGRES_PASSWORD
openssl rand -base64 16
```

Paste the values into your `.env` file.

### 3. Start everything

```bash
docker-compose up -d
```

The API is available at **http://localhost:3001**.

### 4. Create your account

```bash
curl -X POST http://localhost:3001/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"you@example.com","password":"yourpassword","name":"Your Name"}'
```

Open **http://localhost:3001** in your browser and log in.

## SDK Usage

Obsevo ships with Python and TypeScript SDKs that are compatible with the Langfuse API.

### Python

```python
from langfuse_light import Langfuse

client = Langfuse(
    public_key="pk-...",
    secret_key="sk-...",
    host="http://localhost:3001"
)

# Create a trace
trace = client.trace(
    name="my-trace",
    user_id="user-123",
    metadata={"env": "production"}
)

# Add an observation (span/generation)
span = trace.span(
    name="llm-call",
    model="gpt-4",
    input={"prompt": "Hello"},
    output={"completion": "Hi there!"},
    usage={"input": 10, "output": 5}
)

# Score the trace
trace.score(name="quality", value=1, comment="Looks good")

client.flush()
```

### TypeScript

```typescript
import { Langfuse } from "langfuse-light";

const client = new Langfuse({
  publicKey: "pk-...",
  secretKey: "sk-...",
  baseUrl: "http://localhost:3001",
});

// Create a trace
const trace = client.trace({
  name: "my-trace",
  userId: "user-123",
  metadata: { env: "production" },
});

// Add an observation
await trace.span({
  name: "llm-call",
  model: "gpt-4",
  input: { prompt: "Hello" },
  output: { completion: "Hi there!" },
  usage: { input: 10, output: 5 },
});

// Score the trace
await trace.score({ name: "quality", value: 1, comment: "Looks good" });

await client.shutdownAsync();
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
| `POST` | `/api/public/observations` | Create observation (SDK) |
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

</details>

<details>
<summary><strong>Datasets</strong></summary>

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/datasets` | List datasets |
| `POST` | `/api/datasets` | Create a dataset |
| `GET` | `/api/datasets/{id}/items` | List items |
| `POST` | `/api/datasets/{id}/import` | Import JSON/CSV |
| `GET` | `/api/datasets/{id}/export` | Export JSON/CSV |
| `GET` | `/api/datasets/{id}/runs` | List runs |
| `POST` | `/api/datasets/{id}/runs` | Create a run |

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

</details>

## Configuration

All configuration is via environment variables. See [`.env.example`](.env.example) for the full list.

| Variable | Required | Default | Description |
|---|---|---|---|
| `DATABASE_URL` | Yes | — | PostgreSQL connection string |
| `REDIS_URL` | No | `redis://localhost:6379` | Redis connection string |
| `JWT_SECRET` | Yes | — | Secret for JWT signing |
| `APP_SECRET_KEY` | Yes | — | Application secret key |
| `APP_PORT` | No | `3001` | API server port |
| `APP_ENV` | No | `development` | `development` / `production` |
| `LOG_LEVEL` | No | `info` | `debug` / `info` / `warn` / `error` |
| `RATE_LIMIT_PER_MINUTE` | No | `0` (disabled) | Per-minute rate limit per client |
| `WORKER_CONCURRENCY` | No | `0` (auto) | Ingestion worker pool size |
| `DB_MAX_CONNS` | No | `20` | PostgreSQL connection pool ceiling |

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

## Project Structure

```
obsevo/
├── cmd/server/main.go            # Entry point
├── internal/
│   ├── config/config.go          # Environment-based configuration
│   ├── db/                       # sqlc generated code (do not edit)
│   ├── auth/                     # JWT, password hashing, middleware
│   ├── api/                      # HTTP handlers + Chi router
│   ├── services/                 # Business logic
│   ├── queue/                    # Redis queue
│   └── worker/                   # Background ingestion processor
├── migrations/                   # PostgreSQL migrations
├── sdk/
│   ├── python/                   # Python SDK (langfuse-light)
│   └── typescript/               # TypeScript SDK
├── web/                          # SvelteKit frontend
├── tests/                        # Unit & integration tests
├── docker-compose.yml
├── Dockerfile
├── Makefile
└── .env.example
```

## Contributing

Contributions are welcome. Please open an issue first to discuss what you'd like to change.

1. Fork the repo
2. Create a feature branch (`git checkout -b feat/my-feature`)
3. Make your changes
4. Run `make lint && make test` to verify
5. Open a pull request

## Migrating from Langfuse

Obsevo is API-compatible with Langfuse. Switching is as simple as pointing your SDK at the new host:

```bash
export LANGFUSE_HOST=http://localhost:3001
export LANGFUSE_PUBLIC_KEY=pk-lf-...
export LANGFUSE_SECRET_KEY=sk-lf-...
```

The official Langfuse Python and JS SDKs work unmodified. To migrate history:

```bash
go run ./cmd/migrate-langfuse \
  -source-host https://cloud.langfuse.com \
  -source-public-key pk-lf-OLD -source-secret-key sk-lf-OLD \
  -target-host http://localhost:3001 \
  -target-public-key pk-lf-NEW -target-secret-key sk-lf-NEW \
  -dry-run
```

Drop `-dry-run` when the summary looks right. See [docs/migration.md](docs/migration.md) for full details.

## Performance

Benchmarked against a 10,000 traces/min workload (4 observations per trace):

| Metric | Value |
|---|---|
| Ingestion latency (p95) | 2.0 ms |
| Worker drain time | 0.2 s |
| Total RAM usage | ~217 MB |
| Storage per event | ~1.1 KB |

The system handles up to 60,000 traces/min with sub-2ms ingestion latency. See [docs/benchmark.md](docs/benchmark.md) for full results and reproduction steps.

## Production Deployment

### Resource Requirements

| Component | Minimum | Recommended |
|---|---|---|
| RAM (total) | 512 MB | 1 GB |
| CPU | 1 vCPU | 2 vCPU |
| Disk | 1 GB | 10 GB |

### Security Checklist

- [ ] Set strong `JWT_SECRET` and `APP_SECRET_KEY` (32+ random characters)
- [ ] Set strong `POSTGRES_PASSWORD`
- [ ] Set `APP_ENV=production`
- [ ] Don't expose PostgreSQL/Redis ports to public network
- [ ] Use TLS termination (nginx, Caddy, or Traefik)

### Backups

```bash
# Backup
docker compose exec postgres pg_dump -U obsevo obsevo > backup_$(date +%Y%m%d).sql

# Restore
cat backup_20260824.sql | docker compose exec -T postgres psql -U obsevo obsevo
```

See [docs/deployment.md](docs/deployment.md) for nginx config and scaling notes.

## API Specification

Full OpenAPI 3.0 spec available at [docs/openapi.yaml](docs/openapi.yaml).

## License

[MIT](LICENSE)
