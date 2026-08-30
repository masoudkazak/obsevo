# Gap Analysis — Langfuse Light

Baseline: commit `3f3d069`, reviewed 2026-08-30.

This document records the state of the repository as measured (not as documented),
compares it against the Langfuse feature surface we intend to replace, and
prioritises the remaining work. `NOTES.md` claims Phases 1–10 are complete; the
measurements below show that several of those phases produce runtime errors on
their primary path.

## 1. Current architecture (as built)

```
SvelteKit SPA (web/)  ──HTTP──>  Go binary (cmd/server)
                                   ├─ chi router          internal/api/*
                                   ├─ service layer       internal/services/*
                                   ├─ sqlc queries        internal/db/*  (generated)
                                   ├─ goroutine worker    internal/worker/ingestion.go
                                   └─ redis queue         internal/queue/redis.go
                                          │
                            PostgreSQL 16 ┘ └ Redis 7
```

Containers: `api`, `postgres`, `redis` (web is dev-served by Vite; no `web`
container is defined in `docker-compose.yml`). Migrations are applied by the API
process at startup via `runMigrations` in `cmd/server/main.go`, tracked in
`schema_migrations`.

Fixed decisions carried over from `NOTES.md`: chi router, limit/offset
pagination, `x-api-key` header for SDK auth, sqlc codegen. The sqlc version
recorded in `NOTES.md` (1.25.0) is wrong — the committed generated files are
produced by **1.31.1**, which is what reproduces byte-identical output.

## 2. Verified defects on the core path

Each of these was reproduced against a live Postgres/Redis using the checked-in
code. They are ordered by blast radius.

### 2.1 All trace and observation ingestion fails (P0)

`parseTime` in `internal/services/traces.go` builds a timestamp with
`pgtype.Timestamptz.Scan(s)` and discards the error. pgx's text scan path for
`timestamptz` expects the Postgres wire layout (`2006-01-02 15:04:05.999999999Z07:00`),
not RFC3339 with a `T` separator — which is what every Langfuse SDK emits. The
parse fails, the error is dropped, and a zero (`Valid: false`) value reaches the
`NOT NULL` column:

```
POST /api/traces  {"id":"...","start_time":"2026-08-30T10:00:00Z", ...}
→ {"error":"... null value in column \"start_time\" of relation \"traces\"
   violates not-null constraint (SQLSTATE 23502)"}
```

The identical failure occurs for observations. Consequence: **no trace has ever
been successfully ingested through the HTTP API.** Everything downstream that
reads traces — analytics, evaluators, dataset runs, the whole dashboard — is
operating on an empty table.

### 2.2 All prompt creation fails (P0)

`prompts.prompt` is `JSONB`, but `PromptService.CreatePrompt` writes
`[]byte(req.Prompt)` where `req.Prompt` is a plain Go string. pgx passes those
bytes to Postgres as JSON source text, so any non-JSON template is rejected:

```
POST /api/prompts  {"name":"greet","prompt":"Hello {{name}}, welcome!"}
→ {"error":"creating prompt: ERROR: invalid input syntax for type json (SQLSTATE 22P02)"}
```

Langfuse stores a text prompt as a JSON string and a chat prompt as a JSON array
of `{role, content}` objects. The fix is to encode correctly and accept both
shapes, not to change the column type.

### 2.3 Cost is never persisted (P0)

`CreateTrace` passes `pgtype.Float8{Float64: 0, Valid: req.TotalCost != nil}` —
the value is hardcoded to `0` and only the validity flag varies. `updateTrace`
and `CreateObservation` have the same defect. The handler then patches the
returned struct in memory, so the HTTP response *looks* correct while the row
holds `0`. Every cost analytic reads the stored zeros.

### 2.4 Client-supplied observation IDs are discarded (P0)

`CreateObservation` accepts `req.ID` but the `CreateObservation` sqlc query has
no `id` column in its INSERT, so Postgres generates a fresh UUID. SDKs assign IDs
client-side and reference them in `parentObservationId`. Dropping them breaks
parent/child linking and makes create-then-update (the normal span lifecycle)
impossible.

### 2.5 No project authorization (P0, security)

Every dashboard handler takes `project_id` from a query parameter and uses it
directly. `auth.Middleware` proves *who* the caller is but nothing checks whether
that user's organizations contain the requested project. Any authenticated user
can read or write any other tenant's traces, prompts, datasets, scores and
analytics by changing one query parameter. Detail endpoints (`GET /api/traces/{id}`,
`GET /api/observations/{id}`, `GET /api/scores/{id}`) do not scope by project at
all. The SDK path has the same hole: `GET /api/public/traces/{traceId}` resolves
the trace without comparing it to the API key's project.

### 2.6 Queue retry is unbounded (P1)

`Worker.processBatch` calls `queue.RequeueFailed` on any error, which pushes the
item back with no attempt counter and no dead-letter path. A permanently
poisoned item (for example one that trips 2.1) spins forever at the tick rate.

## 3. Langfuse compatibility gaps

### 3.1 Data model

| Langfuse field | Status | Notes |
|---|---|---|
| `traces.release`, `version`, `public`, `bookmarked` | missing | |
| `traces.environment` | missing | Langfuse v3+ concept |
| `observations.project_id` | missing | forces every scope check through a join |
| `observations.level` (DEBUG/DEFAULT/WARNING/ERROR) | missing | only `status` exists |
| `observations.status_message` | missing | error text has nowhere to go |
| `observations.completion_start_time` | missing | needed for time-to-first-token |
| `observations.prompt_id` / `prompt_version` | missing | breaks prompt↔generation linking |
| `observations.usage_details` / `cost_details` | missing | only flat `token_usage`/`cost` |
| `scores.observation_id` | missing | scores can only attach to a trace |
| `scores.session_id`, `dataset_run_id` | missing | |
| `scores.data_type` (NUMERIC/CATEGORICAL/BOOLEAN) | missing | |
| `scores.string_value` | missing | categorical scores cannot be stored |
| `scores.project_id` | missing | |
| `prompts.labels` | missing | replaced by a single `is_active` boolean |
| `prompts.type` (text/chat) | missing | |
| `prompts.tags`, `commit_message`, `created_by` | missing | |
| `sessions` table | missing | `traces.session_id` is a bare string |
| model pricing table | missing | no cost can be computed server-side |
| `dataset_runs.metadata`, `description` | missing | |
| `dataset_items.status`, `source_observation_id` | missing | |

`is_active` is not equivalent to Langfuse's `labels`. Langfuse resolves a prompt
by label (`production`, `latest`, or any custom label), and one version can carry
several labels. A boolean cannot express that, and the current
`GetActivePromptByName` even tolerates multiple active rows by silently taking
the highest version.

### 3.2 Ingestion API

`POST /api/public/ingestion` handles three event types: `trace-create`,
`trace-update`, `observation-create`. The Langfuse SDKs emit `span-create`,
`span-update`, `generation-create`, `generation-update`, `event-create`,
`observation-create`, `observation-update`, `score-create` and `sdk-log`. An
unrecognised type is reported as a 400 per item, so a stock SDK loses every span
and generation it sends.

Other deviations:

- Auth is `x-api-key`. Langfuse SDKs send HTTP Basic with the public key as
  username and the secret key as password. No SDK can authenticate as-is.
- The batch response is `200`; Langfuse returns `207` with per-item results.
- `GET /api/public/traces/{traceId}` returns `{trace, observations}`; Langfuse
  returns the trace object with an `observations` field and `htmlPath`.
- Missing public endpoints: `/api/public/v2/prompts`, `/api/public/scores`,
  `/api/public/sessions`, `/api/public/observations`, `/api/public/datasets`,
  `/api/public/dataset-items`, `/api/public/dataset-run-items`,
  `/api/public/metrics/daily`, `/api/public/projects`, `/api/public/health`.

### 3.3 Evaluation engine

The brief asks for ten evaluator types. Present: exact-match is absent, keyword,
regex and numeric-range exist, plus an undocumented `length_check`. Missing:
exact match, JSON validity, JSON schema, custom code, semantic similarity,
embedding similarity.

`llmJudgeEvaluator.Evaluate` interpolates the prompt and then returns a
hardcoded `0.5` with the comment `"LLM judge placeholder"` — it never issues an
HTTP request. This is the single largest functional gap against the brief, which
states LLM-as-a-judge must not be a placeholder.

`buildCodeEvaluator` also swallows every `json.Unmarshal` error and falls through
to `noopEvaluator` for an unknown `evaluator` key, so a typo in a config yields
scores of `0` with no diagnostic.

`numericRangeEvaluator` documents `"cost"` or `"tokens"` but only implements
`"cost"`; `"tokens"` returns 0 with `"unsupported field"`.

### 3.4 Experiments

No experiment concept exists. `dataset_runs` can be created and items attached,
but there is no comparison between runs, no per-run aggregate of
score/cost/latency, and no regression detection. The brief's "Model B → +8%
quality, −20% cost, +10% latency" output has no backing data structure.

### 3.5 SDKs and OpenTelemetry

Neither a Python nor a JS/TS SDK exists in the repository. No OTel ingestion
endpoint exists.

### 3.6 Analytics

Implemented: total traces, error rate, average/min/max latency, cost and token
totals, and four over-time series bucketed by day.

Missing: P50 and P95 latency (only avg/min/max), average score in the summary,
and every filter except project and time range — user, session, model, tags and
status are not filterable. The over-time queries hardcode `date_trunc('day', ...)`,
so an hour-scale range collapses to one point.

### 3.7 Security

Beyond 2.5: API keys are stored and compared in plaintext (`GetAPIKeyByKey`
selects on the raw key), there is no rate limiting anywhere, no audit log, no
CORS configuration, and role membership (`VIEWER`/`EDITOR`/`ADMIN`) is written
at registration but never enforced on any route.

## 4. Priority order

The sequencing rule is: nothing above can be demonstrated until the core path
works, so correctness precedes compatibility, which precedes new features.

| # | Work | Rationale |
|---|---|---|
| P0 | §2.1–2.5 — timestamps, prompt encoding, cost persistence, observation IDs, project authorization | The product does not function and leaks across tenants |
| P1 | §3.1 schema alignment + §3.2 ingestion compatibility | Unblocks real SDK traffic, which every later item depends on |
| P2 | §3.3 evaluation engine — ten evaluators, real LLM judge | Largest explicit gap against the brief |
| P3 | model pricing + server-side cost computation | Makes cost analytics meaningful |
| P4 | §3.4 experiments and run comparison | Depends on P1 and P2 |
| P5 | §3.6 analytics — percentiles, filters, adaptive bucketing | |
| P6 | §3.5 Python and JS SDKs, OTel ingestion | Depends on a stable P1 API |
| P7 | §3.7 security hardening — key hashing, rate limits, RBAC, audit log | |
| P8 | benchmark harness, migration tooling, documentation | |

## 5. Constraints held constant

Per `AGENTS.md` and the project brief, the following are not revisited: no
ClickHouse, no second datastore, no S3, no Kubernetes, no Kafka, no separate
worker service, no microservices. Postgres plus Redis plus one Go binary remains
the whole system, and every item above is scoped to fit that.
