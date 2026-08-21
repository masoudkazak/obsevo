# Langfuse Light — Architecture Plan

## Goal

Build a lightweight, open-source version of Langfuse with maximum functional similarity but simpler architecture and stack — suitable for small teams with minimal infrastructure overhead (easy self-hosting, no heavy side services). Code will NOT be a copy of Langfuse; only behavior and capabilities will be similar.

License: **MIT** (recommended — maximum adoption, compatible with Langfuse's own MIT license).

---

## Phase 1: Analysis of Langfuse

### Core Features (Prioritized)

#### Must-have

| Feature | Description |
|---|---|
| **LLM Tracing/Observability** | Hierarchical traces (Trace → Generation → Span → Event) with metadata, model info, token usage, cost |
| **Prompt Management** | Versioning, template compilation with variables, SDK-side caching |
| **Evaluation/Scoring** | LLM-as-a-judge, code evaluators, user feedback, manual labeling, score metadata |
| **Datasets** | Test sets, batch evaluation runs on datasets |
| **Dashboard & Analytics** | Average latency, cost, token usage, error rate, throughput over time ranges |
| **Web Server + API** | REST API with OpenAPI spec, Python and JS/TS SDKs |
| **Simple Auth** | Email/password + optional SSO (SAML/OIDC in enterprise) |
| **Projects & Organizations** | Multi-project per org, API keys at project level |

#### Nice-to-have

| Feature | Description |
|---|---|
| **Playground** | Test prompts with different models directly from UI |
| **Batch Export** | Export traces to CSV/JSON |
| **OpenTelemetry Integration** | Receive spans from OTel collector |
| **Framework Integrations** | LangChain, LlamaIndex, Haystack, Vercel AI SDK, etc. |

#### Removable in Lightweight Version

| Feature | Reason |
|---|---|
| **Complex Multi-tenancy** | Simple org/role is sufficient |
| **Advanced RBAC** | Only viewer/editor/admin roles |
| **Kubernetes/Helm mandatory** | docker-compose is enough |
| **S3 mandatory** | Local filesystem is enough for small teams |
| **Background Migrations** | Simple Prisma migrations |
| **Azure Blob / OCI support** | Only S3-compatible or local |
| **Transactional Emails** | Only simple invite flow |
| **In-app AI Agent** | New, non-essential feature |

### Current Langfuse Stack

```
┌─────────────────────────────────────────────────┐
│                  Langfuse v4                     │
├─────────────────────────────────────────────────┤
│  Frontend:  Next.js (React) — langfuse-web       │
│  Backend:   Next.js API routes + tRPC            │
│  Worker:    BullMQ (Node.js) — langfuse-worker   │
│  ORM:       Prisma                               │
├─────────────────────────────────────────────────┤
│  Postgres    → transactional (users, orgs,       │
│                projects, prompts, API keys)       │
│  ClickHouse  → OLAP (traces, observations,       │
│                scores — read-heavy analytics)     │
│  Redis       → queue (BullMQ) + cache (API keys, │
│                prompts)                           │
│  S3/MinIO    → raw event storage + media +       │
│                batch export                       │
└─────────────────────────────────────────────────┘
```

**Why 4 services?**
- **Postgres**: CRUD operations (users, orgs, prompts, configs) — OLTP workload
- **ClickHouse**: analytics queries on millions of traces — OLAP workload (much faster than Postgres for aggregate queries)
- **Redis**: queue for async ingestion + caching API keys and prompts
- **S3**: initial event persistence (recoverability) + media storage

---

## Phase 2: Proposed Lightweight Architecture

### Key Decisions

#### Can we work without ClickHouse?

> **Yes, for small teams (up to ~100k traces/day) Postgres alone is sufficient.** ClickHouse exists purely for performance on heavy analytics queries. Postgres with proper indexing and simple partitioning works well at small scale. Trade-off: analytics queries on large datasets are ~10x slower — acceptable for small teams.

#### Is S3 mandatory?

> **No.** Files can be stored directly on disk. S3 is for recoverability and multi-modal storage. For the lightweight version, a Docker volume is sufficient.

#### Is Redis mandatory?

> **No, but recommended.** Without Redis, queue can use Postgres (pg-boss or similar) and cache can use in-memory Node.js. But Redis makes implementation simpler and faster, and keeps main app memory free. **Recommendation: keep Redis — it's just one container.**

### Proposed Lightweight Stack

```
┌─────────────────────────────────────────────────────┐
│              Langfuse Light                          │
├─────────────────────────────────────────────────────┤
│  Frontend:  SvelteKit (lightweight SPA)              │
│  Backend:   Go (Fiber/Chi router + sqlc ORM)         │
│  Worker:    In same Go process (goroutines)          │
├─────────────────────────────────────────────────────┤
│  Database:  PostgreSQL 16 (single DB for everything) │
│  Cache:     Redis 7 (queue + cache)                  │
│  Storage:   Local disk (volume) or optional S3       │
└─────────────────────────────────────────────────────┘
```

**Why Go + SvelteKit?**
- AI coding tools (Claude Code, OpenCode) write both Go and Svelte equally fast — language choice doesn't affect development speed anymore
- Go: ~30-50MB RAM (vs ~300MB for Node.js), single binary deployment, fast concurrent ingestion via goroutines
- SvelteKit: ~50% smaller bundle than Next.js, faster runtime, simpler mental model
- sqlc: type-safe SQL directly from queries — no ORM overhead, full control

**Container count: 3 (vs 6 in original Langfuse)**
- `langfuse-api` — Go binary (API server + background worker)
- `langfuse-web` — SvelteKit static build (served by Go or nginx)
- `postgres` — PostgreSQL
- `redis` — Redis

### Comparison

| | Original Langfuse | Langfuse Light |
|---|---|---|
| Containers | 6 (web, worker, postgres, clickhouse, redis, minio) | 3-4 (api, web, postgres, redis) |
| RAM Required | ~8GB+ | ~200-400MB |
| Startup Time | 10-15 min | 2-3 min |
| ClickHouse | Required | Removed |
| S3/MinIO | Required | Removed (local disk) |
| Multi-modal traces | From S3 | From disk |
| Analytics performance | High (ClickHouse) | Medium (Postgres + indexes) |
| Worker separation | Required | Optional (goroutines in same process) |
| Horizontal scaling | Supported | Single binary (sufficient for small teams) |
| Language | TypeScript (Node.js) | Go (backend) + Svelte (frontend) |

### Explicit Trade-offs

| What's Lost | Replacement |
|---|---|
| ClickHouse analytics speed | Postgres with indexes and materialized views |
| S3 recoverability | Postgres WAL backup + local volume |
| Multi-modal large files | Size limit on local disk |
| Horizontal worker scaling | Single worker (sufficient for small team) |
| ClickHouse cluster mode | Not needed |

---

## Phase 3: Implementation Plan

### Project Structure

```
langfuse-light/
├── docker-compose.yml
├── Dockerfile
├── .env.example
├── Makefile
├── go.mod
├── go.sum
├── cmd/
│   └── server/
│       └── main.go               # Entry point
├── internal/
│   ├── config/
│   │   └── config.go             # Env-based config
│   ├── db/
│   │   ├── queries.sql            # sqlc raw queries
│   │   ├── db.go                  # sqlc generated
│   │   ├── models.go              # sqlc generated
│   │   └── queries.sql.go         # sqlc generated
│   ├── auth/
│   │   ├── jwt.go
│   │   ├── middleware.go
│   │   └── password.go
│   ├── api/
│   │   ├── router.go              # Chi/Fiber routes
│   │   ├── middleware.go
│   │   ├── traces.go
│   │   ├── prompts.go
│   │   ├── evaluations.go
│   │   ├── datasets.go
│   │   ├── projects.go
│   │   └── auth.go
│   ├── services/
│   │   ├── traces.go
│   │   ├── prompts.go
│   │   ├── evaluations.go
│   │   ├── datasets.go
│   │   └── projects.go
│   ├── queue/
│   │   └── redis.go               # Simple Redis queue
│   └── worker/
│       └── ingestion.go           # Background processor
├── migrations/
│   ├── 001_init.up.sql
│   └── 001_init.down.sql
├── web/                            # SvelteKit frontend
│   ├── package.json
│   ├── svelte.config.js
│   ├── src/
│   │   ├── routes/
│   │   │   ├── +page.svelte
│   │   │   ├── traces/
│   │   │   ├── prompts/
│   │   │   ├── datasets/
│   │   │   └── settings/
│   │   ├── lib/
│   │   │   ├── api.ts
│   │   │   └── stores.ts
│   │   └── app.html
│   └── static/
├── tests/
│   ├── api/
│   └── services/
└── README.md
```

### Implementation Order

1. **Database migrations** (SQL) — users, orgs, projects, API keys, traces, observations, prompts, scores, datasets
2. **sqlc queries** — type-safe SQL queries from raw SQL
3. **Auth** — email/password registration/login with JWT
4. **Core API** — trace ingestion, prompt CRUD, evaluation scores (Go + Chi/Fiber)
5. **Worker** — background trace processing (goroutines + Redis queue)
6. **Dashboard UI** — traces list, trace detail, prompt management, analytics (SvelteKit)
7. **SDK compatibility** — Python/JS SDK endpoint compatibility
8. **Datasets** — dataset CRUD + evaluation runs
9. **Tests** — API tests, service tests
10. **Docker setup** — docker-compose.yml, Dockerfile, README

### Database Schema (SQL Migrations)

```sql
-- 001_init.up.sql

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE organizations (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
  name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE users (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
  email TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  name TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE members (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
  user_id TEXT NOT NULL REFERENCES users(id),
  org_id TEXT NOT NULL REFERENCES organizations(id),
  role TEXT NOT NULL DEFAULT 'VIEWER' CHECK (role IN ('VIEWER', 'EDITOR', 'ADMIN')),
  UNIQUE(user_id, org_id)
);

CREATE TABLE projects (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
  name TEXT NOT NULL,
  org_id TEXT NOT NULL REFERENCES organizations(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE api_keys (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
  project_id TEXT NOT NULL REFERENCES projects(id),
  key TEXT NOT NULL UNIQUE,
  name TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE traces (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
  project_id TEXT NOT NULL REFERENCES projects(id),
  name TEXT,
  input JSONB,
  output JSONB,
  metadata JSONB,
  user_id TEXT,
  session_id TEXT,
  tags TEXT[] DEFAULT '{}',
  start_time TIMESTAMPTZ NOT NULL,
  end_time TIMESTAMPTZ,
  total_cost DOUBLE PRECISION,
  token_usage JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_traces_project_start ON traces(project_id, start_time DESC);
CREATE INDEX idx_traces_project_name ON traces(project_id, name);

CREATE TABLE observations (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
  trace_id TEXT NOT NULL REFERENCES traces(id) ON DELETE CASCADE,
  type TEXT NOT NULL CHECK (type IN ('SPAN', 'GENERATION', 'EVENT')),
  name TEXT,
  input JSONB,
  output JSONB,
  metadata JSONB,
  model TEXT,
  model_parameters JSONB,
  start_time TIMESTAMPTZ NOT NULL,
  end_time TIMESTAMPTZ,
  token_usage JSONB,
  cost DOUBLE PRECISION,
  status TEXT NOT NULL DEFAULT 'DEFAULT' CHECK (status IN ('DEFAULT', 'OK', 'ERROR')),
  parent_observation_id TEXT REFERENCES observations(id)
);

CREATE INDEX idx_observations_trace ON observations(trace_id);

CREATE TABLE prompts (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
  project_id TEXT NOT NULL REFERENCES projects(id),
  name TEXT NOT NULL,
  version INT NOT NULL DEFAULT 1,
  prompt JSONB NOT NULL,
  config JSONB,
  is_active BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(project_id, name, version)
);

CREATE TABLE scores (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
  trace_id TEXT NOT NULL REFERENCES traces(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  value DOUBLE PRECISION,
  comment TEXT,
  source TEXT NOT NULL CHECK (source IN ('USER', 'EVALUATOR', 'SDK')),
  user_id TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_scores_trace ON scores(trace_id);

CREATE TABLE datasets (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
  project_id TEXT NOT NULL REFERENCES projects(id),
  name TEXT NOT NULL,
  description TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(project_id, name)
);

CREATE TABLE dataset_items (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
  dataset_id TEXT NOT NULL REFERENCES datasets(id) ON DELETE CASCADE,
  input JSONB NOT NULL,
  expected_output JSONB,
  metadata JSONB,
  source_trace_id TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE dataset_runs (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
  dataset_id TEXT NOT NULL REFERENCES datasets(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE dataset_run_items (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
  dataset_run_id TEXT NOT NULL REFERENCES dataset_runs(id) ON DELETE CASCADE,
  dataset_item_id TEXT NOT NULL REFERENCES dataset_items(id),
  observation_id TEXT,
  score_id TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

---

## Summary

Langfuse Light delivers ~80% of Langfuse's value with ~20% of the infrastructure complexity:

- **3 containers** instead of 6
- **200-400MB RAM** instead of 8GB+
- **2-3 min startup** instead of 10-15 min
- **Single Postgres** instead of Postgres + ClickHouse + S3
- **Go backend** — single binary, fast goroutines, ~30MB footprint
- **SvelteKit frontend** — smaller bundle, faster runtime
- **MIT license** — fully open, community-friendly
