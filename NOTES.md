# NOTES.md — Langfuse Light

## Session log
- [2026-08-24] Phase 1: Foundation setup complete - Go module, SvelteKit, Docker, migrations, sqlc
- [2026-08-24] Phase 1: sqlc code generated, Chi router selected
- [2026-08-24] Phase 2: Auth system implemented - password hashing, JWT, register/login endpoints
- [2026-08-24] Phase 2: Project and organization CRUD endpoints added
- [2026-08-24] Phase 3: Core API — Traces & Observations implemented
  - Trace service (services/traces.go) with create/update, list with filters, get with observations
  - Observation service with create, get by ID
  - Trace API handlers (api/traces.go) — POST/GET traces, POST/GET observations, batch ingestion
  - Redis queue (queue/redis.go) for async ingestion
  - Background worker (worker/ingestion.go) processing queue items
  - Router extracted to api/router.go
  - Added sqlc queries: GetOrganizationsByUserID, GetMembersByOrgID, GetTracesByProjectIDAndName, CountTracesByProjectID, GetTraceWithProject
  - Pagination: limit/offset style
- [2026-08-24] Phase 5: Evaluation & Scoring implemented
  - Added sqlc queries: GetScoresByTraceIDAndName, GetScoreAggregationByProjectID, GetTraceLatencyStatsByProjectID, GetTraceCostStatsByProjectID, GetTraceTokenUsageStatsByProjectID, GetTraceErrorRateByProjectID, GetTraceCountByProjectIDAndTimeRange
  - Evaluation service (services/evaluations.go) — score CRUD, aggregation, latency/cost/token/error analytics
  - Evaluation API handlers (api/evaluations.go) — POST/GET scores, GET trace scores, score aggregation, analytics endpoints
  - Registered evaluation routes in api/router.go
  - Wired evalService in cmd/server/main.go
  - Build checks: gofmt, go vet, go build, go test all pass
- [2026-08-24] Phase 7: Frontend — Dashboard implemented
  - Installed Tailwind CSS v4 with @tailwindcss/vite plugin
  - Created API client (web/src/lib/api.ts) — typed client for all backend endpoints
  - Created stores (web/src/lib/stores.ts) — auth, projects, notifications
  - Created global CSS (web/src/app.css) with Tailwind import
  - Added Vite dev proxy for /api and /health to Go backend (port 8080)
  - Layout: collapsible sidebar with navigation, project selector, user info, logout
  - Login page with email/password form
  - Register page with name/email/password form
  - Trace list page: table with name/time/duration/cost/user/tags, pagination, name filter
  - Trace detail page: header with metadata, observation tree with expandable detail panels, I/O display
  - Prompt list page: grouped by name, search, create modal
  - Prompt detail page: editor tab with live template variable detection, versions tab, new version modal, set active version
  - All labels properly associated with controls (a11y)
  - Build check: 0 errors, 0 warnings; build successful
- [2026-08-24] Phase 7 cont. — Prompt management UI completed
  - Fixed critical bug: SetPromptActive only deactivated versions but never activated target version
  - Added sqlc query UpdatePromptActiveByVersion (queries.sql:143-144)
  - Fixed services/prompts.go SetPromptActive to call UpdatePromptActiveByVersion after deactivation
  - Enhanced template preview with interactive variable substitution — input fields for each {{var}} and live compiled output
  - Fixed pre-existing build error: cmd/server/main.go missing EvaluationHandler initialization
  - All checks pass: gofmt, go vet, go build, go test, svelte-check, npm build

## Decisions made (don't re-decide these)
- Router: Chi (chosen in Phase 1)
- Pagination style: limit/offset (chosen in Phase 3)
- Auth header name for API keys: TBD (Phase 2)
- sqlc version: v1.25.0
- Frontend styling: Tailwind CSS v4 with @tailwindcss/vite plugin
- Frontend state: Svelte 5 runes mode + writable stores
- API proxy: Vite dev server proxies /api and /health to localhost:8080
- Analytics queries: raw SQL in queries.sql for aggregation (avg/min/max latency, cost, tokens, error rate)
- Score sources: USER, EVALUATOR, SDK (per DB constraint)

## TODO (found while working)
- ...

## Phase 5 API Endpoints Added
- POST `/api/scores` — create score (trace_id, name, value, comment, source, user_id)
- GET `/api/scores/:id` — get score by ID
- GET `/api/scores/aggregation?project_id=` — aggregated score stats by name
- GET `/api/traces/:id/scores?name=` — list scores for a trace (optional name filter)
- GET `/api/analytics?project_id=` — full analytics summary (latency, cost, tokens, errors, scores)
- GET `/api/analytics/latency?project_id=` — latency stats (avg/min/max seconds)
- GET `/api/analytics/cost?project_id=` — cost stats (total/avg/min/max)
- GET `/api/analytics/tokens?project_id=` — token usage stats (total/avg/input/output)
- GET `/api/analytics/errors?project_id=` — error rate stats (total/error count/rate)

## Known limitations (intentional, per AGENTS.md)
- no ClickHouse, no S3, single worker, etc.
