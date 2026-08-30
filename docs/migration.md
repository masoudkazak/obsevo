# Migrating from Langfuse

Langfuse Light reads and writes the same API shapes as Langfuse, so a migration
is mostly a copy rather than a transform. This document covers the tool, what it
carries across, and the handful of things it deliberately does not.

## The short version

```bash
# 1. Point your SDK at the new host and keep using it — no code changes.
export LANGFUSE_HOST=http://localhost:3001
export LANGFUSE_PUBLIC_KEY=pk-lf-...      # from Settings → API Keys
export LANGFUSE_SECRET_KEY=sk-lf-...

# 2. Copy history across.
go run ./cmd/migrate-langfuse \
  -source-host https://cloud.langfuse.com \
  -source-public-key pk-lf-OLD -source-secret-key sk-lf-OLD \
  -target-host http://localhost:3001 \
  -target-public-key pk-lf-NEW -target-secret-key sk-lf-NEW \
  -dry-run
```

Drop `-dry-run` when the summary looks right.

## Why your application code does not change

The compatibility surface is deliberate, not incidental:

| Concern | Langfuse | Langfuse Light |
|---|---|---|
| Ingestion endpoint | `POST /api/public/ingestion` | same |
| Ingestion events | `trace-create`, `span-create/update`, `generation-create/update`, `event-create`, `observation-create/update`, `score-create`, `sdk-log` | same |
| Ingestion response | `207` with `successes` / `errors` | same |
| Auth | HTTP Basic, public key as user, secret as password | same (plus an `x-api-key` header for this project's own clients) |
| Key format | `pk-lf-…` / `sk-lf-…` | same |
| Read API | `/api/public/traces`, `/observations`, `/scores`, `/sessions`, `/v2/prompts`, `/datasets`, `/dataset-items`, `/dataset-run-items` | same |
| Response envelope | `{data, meta:{page,limit,totalItems,totalPages}}` | same |
| Field naming | camelCase | same on `/api/public/*` |

So the practical migration for a running application is one environment
variable. The official Langfuse Python and JS SDKs work unmodified against this
server. This repository also ships its own lighter SDKs under `sdk/` if you
would rather drop the dependency.

## What the tool copies

| Resource | Copied | Notes |
|---|---|---|
| Traces | yes | id, name, user, session, tags, release, version, input, output, metadata, timestamp |
| Observations | yes | spans, generations and events, including `parentObservationId` so the tree is preserved |
| Token usage | yes | normalised into the stored shape; every Langfuse usage form is accepted |
| Cost | recomputed | see [Cost](#cost) below |
| Scores | yes | numeric, boolean and categorical, with their source and comment |
| Prompts | yes | every version, with type, labels, tags, config and commit message |
| Datasets | yes | name and description |
| Dataset items | yes | input, expected output, metadata, source trace/observation, status |
| Dataset runs | **no** | see [What is not copied](#what-is-not-copied) |
| Sessions | derived | recreated automatically from the `sessionId` on migrated traces |
| Users | derived | this project has no user entity; `userId` on traces is the same string |

### Cost

Observation cost is **recomputed from the migrated token usage** against this
project's model pricing table, rather than copied. Two reasons: a cost that was
copied would silently disagree with everything ingested afterwards, and prices
change, so a stale number is worse than a consistently derived one.

If the recomputed figures do not match what you expect, the pricing table is the
place to fix it — `GET /api/model-prices` shows what is in effect, and a
project-scoped row overrides the built-in default:

```bash
curl -X POST "$HOST/api/model-prices?project_id=$PROJECT" \
  -H "Authorization: Bearer $JWT" -H 'Content-Type: application/json' \
  -d '{"model_name":"gpt-4o","input_price":0.0000025,"output_price":0.00001}'
```

A cost your client reported explicitly (`cost` or `usage.totalCost` on the
event) is always kept as-is and never overwritten by an estimate.

## What is not copied

- **Dataset runs and run items.** A run's value is its link to the traces that
  produced it, and its cost and latency aggregates are computed from those
  traces. Copying runs whose traces may not have migrated would produce
  confident, wrong numbers. Re-run the experiment against the migrated dataset
  instead — it takes one command and the result is real.
- **Users, organizations and members.** These are per-deployment; create them on
  the new instance.
- **API keys.** Secrets are stored as digests and cannot be exported. Issue new
  keys and update your applications.
- **Langfuse-only features** with no counterpart here: managed evaluators
  running on Langfuse's infrastructure, LLM playground state, annotation queues,
  and the enterprise RBAC model beyond viewer/editor/admin.

## Options

| Flag | Meaning |
|---|---|
| `-what` | comma-separated subset: `traces,scores,prompts,datasets` |
| `-from` / `-to` | RFC 3339 bounds; migrate only traces in a window |
| `-page-size` | records fetched per request (default 50) |
| `-max-pages` | stop after N pages per resource — useful for a trial run |
| `-id-prefix` | prefix every migrated id |
| `-dry-run` | read and report, write nothing |

### Migrating in slices

For a large history, migrate a month at a time and check as you go:

```bash
for month in 01 02 03; do
  go run ./cmd/migrate-langfuse \
    -source-host https://cloud.langfuse.com \
    -source-public-key "$OLD_PK" -source-secret-key "$OLD_SK" \
    -target-host http://localhost:3001 \
    -target-public-key "$NEW_PK" -target-secret-key "$NEW_SK" \
    -what traces \
    -from "2026-${month}-01T00:00:00Z" \
    -to   "2026-${month}-28T00:00:00Z"
done
```

Ingestion is idempotent on the record id, so re-running an overlapping window
merges rather than duplicating.

### When ids could collide

`-id-prefix` namespaces every migrated trace, observation, score and dataset
item id. Use it when merging two source projects into one target, or when
migrating between projects on the same instance — trace ids are unique per
instance, and reusing one from another project is refused rather than allowed to
cross the tenant boundary.

```bash
go run ./cmd/migrate-langfuse ... -id-prefix "legacy-"
```

## Verifying the result

The tool prints what it wrote and notes anything it could not:

```
=== migration summary ===
  traces         1
  observations   2
  scores         2
  prompts        1
  datasets       1
  dataset items  1

notes:
  - dataset mig-ds: runs were not migrated (re-run experiments against the migrated items)
```

Ingestion is asynchronous, so give the worker a moment before counting on the
target:

```bash
curl -u "$NEW_PK:$NEW_SK" "$HOST/api/public/traces?limit=1" | jq .meta.totalItems
curl -u "$NEW_PK:$NEW_SK" "$HOST/api/public/scores?limit=1" | jq .meta.totalItems
```

Spot-check one trace end to end — the tree, the model, the usage and the scores
should all be intact:

```bash
curl -u "$NEW_PK:$NEW_SK" "$HOST/api/public/traces/<traceId>" | jq \
  '{id, name, userId, totalCost, observations: [.observations[] | {id, type, parentObservationId, model, usage}], scores: [.scores[] | {name, value, stringValue, dataType}]}'
```

## Cutting over

1. Stand up Langfuse Light and create a project and API key.
2. Migrate history with the tool.
3. Point one non-critical service at the new host and confirm its traces arrive.
4. Move the rest.
5. Keep the old instance readable for as long as you want a fallback — nothing
   in this process modifies the source.

The SDKs buffer and retry, so a brief overlap where both hosts receive traffic
is harmless; ingestion is idempotent on the event id.
