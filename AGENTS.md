# AGENTS.md — Langfuse Light

You are an autonomous coding agent working on **Langfuse Light**, a lightweight
open-source clone of Langfuse (LLM observability platform). This file is the
single source of truth for how you must work in this repo. Read it fully
before touching any file. If something here conflicts with what you "know"
from training, THIS FILE WINS.

---

## 0. Golden Rules (read these twice)

1. **Do exactly one task per turn.** Never bundle "also fix X while I'm here."
   If you notice something else broken, write it down in `NOTES.md` under
   `## TODO (found while working)` and move on.
2. **Never invent APIs, table names, column names, or file paths.** If a name
   isn't in this file or already in the codebase, STOP and re-read this file
   and the relevant existing code before guessing.
3. **Always read a file with the view/read tool immediately before editing
   it.** Never edit from memory of an earlier turn — the file may have
   changed.
4. **Small diffs only.** Prefer changing 1 file / 1 function over rewriting
   whole files. If a change touches more than ~150 lines, stop and split it
   into smaller steps.
5. **Never delete or rewrite a file just to "clean it up"** unless the task
   explicitly asks for that file.
6. **After every code change, you MUST run the build/lint/test commands**
   listed in section 6, and you MUST show the output. A task is not done
   until the build is green.
7. **If a command fails, do not "work around" it by disabling checks,
   deleting tests, or commenting out failing code.** Fix the root cause, or
   stop and explain in `NOTES.md` why you are stuck.
8. **Never install a new dependency, package, or tool that isn't already in
   `go.mod` / `web/package.json`** without explicit permission in the task
   description.
9. When unsure between two possible implementations, **choose the simplest
   one that matches the schema and structure already defined in this file.**
   Do not add abstraction layers, generic frameworks, or "flexibility" that
   wasn't asked for.
10. Work **one Phase at a time**, in the order defined in section 8. Do not
    jump ahead to a later phase's feature even if it seems easy.

---

## 1. What this project is

A self-hosted, MIT-licensed, lightweight alternative to Langfuse with:
LLM trace/observation logging, prompt versioning, evaluation/scoring,
datasets + batch evaluation runs, and a small analytics dashboard.

It is **not** a copy of Langfuse's code — only behavior/API shape is similar.
Complex multi-tenancy, ClickHouse, S3, Kubernetes, and BullMQ are
intentionally **removed**. Do not re-add them. See "Explicitly out of scope"
below.

### Explicitly out of scope — never implement these unless asked
- ClickHouse, or any second database engine
- S3 / MinIO / Azure Blob / OCI object storage
- Kubernetes manifests or Helm charts
- BullMQ / Node.js queue systems
- SAML/OIDC SSO, advanced RBAC beyond viewer/editor/admin
- Transactional email sending (invite flow is just a generated link/token)
- Any "AI agent" feature inside the product itself

---

## 2. Tech stack (do not substitute any of this)

| Layer      | Choice                                   |
|------------|-------------------------------------------|
| Backend    | Go (standard toolchain), router: Chi or Fiber (pick ONE at Phase 1 and stay consistent) |
| DB access  | sqlc (type-safe generated Go from raw SQL) — **never hand-write code in `*_sql.go` generated files** |
| Database   | PostgreSQL 16 (single database, no ClickHouse) |
| Queue/Cache| Redis 7 |
| Worker     | goroutines inside the same Go binary — **not** a separate service |
| Frontend   | SvelteKit (SPA/static build) |
| Storage    | Local disk volume by default; S3-compatible client is optional/pluggable later, not required now |
| Container  | Docker + docker-compose |
| License    | MIT |

Target footprint: 3–4 containers (`api`, `web`, `postgres`, `redis`),
~200–400MB RAM, < 3 min startup. Keep this in mind — if a change adds a new
service/container, stop and ask.

---

## 3. Directory structure (create/use exactly this layout)

```
langfuse-light/
├── docker-compose.yml
├── Dockerfile
├── .env.example
├── Makefile
├── go.mod
├── go.sum
├── cmd/server/main.go
├── internal/
│   ├── config/config.go
│   ├── db/                # sqlc output lives here — generated, do not hand-edit
│   │   ├── queries.sql
│   │   ├── db.go
│   │   ├── models.go
│   │   └── queries.sql.go
│   ├── auth/{jwt.go,middleware.go,password.go}
│   ├── api/
│   │   ├── router.go
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
│   ├── queue/redis.go
│   └── worker/ingestion.go
├── migrations/
│   ├── 001_init.up.sql
│   └── 001_init.down.sql
├── web/                    # SvelteKit app
│   └── src/routes/{traces,prompts,datasets,settings}/...
├── tests/{api,services}/
├── NOTES.md                # your running log — see section 9
└── README.md
```

**Rule:** before creating a new file, check if this tree already has a place
for it. If it doesn't fit anywhere above, stop and ask instead of inventing a
new top-level folder.

---

## 4. Database schema (do not rename/redesign without being asked)

Core tables, already fixed by design — implement migrations to match this
exactly unless a task explicitly changes the schema:

`organizations`, `users`, `members` (role: VIEWER/EDITOR/ADMIN),
`projects`, `api_keys`, `traces`, `observations` (type: SPAN/GENERATION/EVENT),
`prompts` (versioned, unique on project_id+name+version), `scores`
(source: USER/EVALUATOR/SDK), `datasets`, `dataset_items`, `dataset_runs`,
`dataset_run_items`.

Conventions:
- All primary keys: `TEXT` using `gen_random_uuid()::TEXT`.
- All timestamps: `TIMESTAMPTZ`, default `now()`.
- Use `JSONB` for input/output/metadata/config fields.
- Foreign keys use `ON DELETE CASCADE` only where the original plan shows it
  (traces→observations, traces→scores, datasets→dataset_items, etc).
- Every new query needs a matching index if it filters/sorts by a non-PK
  column used in a hot path (e.g. `project_id, start_time`).

If you need a new column or table, **add a new migration file**
(`00N_description.up.sql` / `.down.sql`). Never edit an already-applied
migration file.

---

## 5. Coding conventions

### Go
- Package layout: `api/` = HTTP handlers only (parse request, call service,
  write response). `services/` = business logic. `db/` = generated only.
- Every exported function needs a one-line doc comment.
- Errors: wrap with `fmt.Errorf("doing X: %w", err)`, never swallow errors.
- Use `context.Context` as first parameter for anything touching DB/Redis.
- No global mutable state except config loaded once at startup.
- Run `gofmt -w .` before finishing any Go change.

### SQL / sqlc
- Write queries in `internal/db/queries.sql` using sqlc annotation comments
  (`-- name: GetTraceByID :one`, etc).
- After adding/editing a query, run `sqlc generate` (see Makefile) — never
  hand-write the generated `.go` files.

### SvelteKit
- File-based routing under `web/src/routes/`.
- Keep API calls in `web/src/lib/api.ts`, shared state in
  `web/src/lib/stores.ts`. Don't fetch directly from inside `.svelte` files
  for anything reused more than once.
- Match the route folders already listed in section 3 — don't invent new
  top-level routes without being asked.

### API design
- REST, JSON in/out, plural nouns: `/api/traces`, `/api/prompts`,
  `/api/datasets`, etc. (matches section 3's `api/*.go` files 1:1).
- Auth: JWT bearer token for user sessions, API key header
  (`x-api-key` or similar — check `auth/middleware.go` before assuming) for
  SDK ingestion endpoints.
- Every list endpoint must support pagination (`limit`, `cursor` or
  `page`/`page_size` — pick one convention in Phase 3 and reuse everywhere).

---

## 6. Required checks after every change

Run these and paste the output before declaring a task done:

```bash
# Go backend
gofmt -l .            # must print nothing
go vet ./...
go build ./...
go test ./...

# Frontend (only if you touched web/)
cd web && npm run check && npm run build
```

If `Makefile` targets exist for these (e.g. `make check`, `make test`), use
those instead — check `Makefile` first.

Never mark a task complete if any of these fail.

---

## 7. Docker / local dev

- `docker-compose.yml` must define exactly: `api`, `web` (only if served
  separately), `postgres`, `redis`. Do not add more services.
- Config via environment variables only, documented in `.env.example`.
  Never hardcode secrets, ports, or connection strings in Go/Svelte code.
- Add healthchecks to `postgres` and `redis` in compose; `api` should wait
  on both being healthy.

---

## 8. Build order — work in this sequence, one phase per session

1. **Foundation**: go.mod, SvelteKit scaffold, Makefile, `.env.example`,
   docker-compose (postgres+redis), first migration, sqlc config.
2. **Auth & orgs**: password hashing, JWT, register/login, org/project/API
   key CRUD, role middleware.
3. **Traces & observations**: ingestion endpoints, list/detail endpoints,
   Redis queue + goroutine worker, cost/token aggregation.
4. **Prompts**: CRUD, versioning, template variable substitution.
5. **Evaluation & scoring**: score CRUD, evaluator hooks, basic analytics
   (latency/cost/error-rate aggregation queries).
6. **Datasets**: CRUD, dataset items, dataset runs, run items, CSV/JSON
   export.
7. **Frontend — dashboard**: nav, trace list/detail, prompt UI.
8. **Frontend — analytics & settings**: charts, project settings, API key
   management, member/invite UI.
9. **SDK compatibility & tests**: confirm Python/JS Langfuse SDK request
   shapes are accepted, OpenAPI spec, test coverage.
10. **Deployment polish**: multi-stage Dockerfile, healthchecks, docs.

**Do not start phase N+1 until phase N's build/lint/test checks all pass.**

---

## 9. NOTES.md — your working memory

Because you (the model) don't reliably remember earlier sessions, keep a
`NOTES.md` at repo root and update it every session with:

```
## Session log
- [date] Phase X, task Y: what was done, what file changed, test status

## Decisions made (don't re-decide these)
- Router: chosen in Phase 1 = ...
- Pagination style: ...
- Auth header name for API keys: ...

## TODO (found while working)
- ...

## Known limitations (intentional, per AGENTS.md)
- no ClickHouse, no S3, single worker, etc.
```

At the start of every new session/task, **read `NOTES.md` first**, before
reading any other file.

---

## 10. Definition of Done (checklist for every task)

- [ ] Change matches the directory/file it belongs to per section 3
- [ ] No out-of-scope feature (section 1) was added
- [ ] Schema changes are a new migration file, not an edit to an old one
- [ ] `gofmt`, `go vet`, `go build`, `go test` all pass (or frontend
      equivalents if `web/` was touched)
- [ ] `NOTES.md` updated with what changed
- [ ] Diff is small and focused on the single requested task