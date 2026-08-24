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
- [2026-08-24] Phase 6: Datasets & Batch Evaluation implemented
  - Dataset service (services/datasets.go) — CRUD, import JSON/CSV, export JSON/CSV
  - Dataset API handlers (api/datasets.go) — 14 endpoints for datasets, items, runs, run items, import/export
  - Added sqlc queries: DeleteDataset, CountDatasetItemsByDatasetID
  - Batch evaluation: create runs, add run items (link item→observation→score), export run results
  - Export supports both JSON and CSV via `?format=csv` query param
  - Registered dataset routes in api/router.go
  - Wired datasetService in cmd/server/main.go
  - Build checks: gofmt, go vet, go build, go test all pass
- [2026-08-24] Phase 9: SDK Compatibility & Testing implemented
  - Added API key auth middleware (internal/auth/middleware.go) — validates `x-api-key` header against api_keys table, sets project_id in context
  - Added SDK-compatible handler (internal/api/sdk.go) — 5 endpoints under `/api/public/` with API key auth
  - SDK endpoints: POST/GET traces, GET trace by ID, POST observations, POST batch ingestion
  - Batch ingestion supports trace-create, trace-update, observation-create event types
  - Fixed ListOrganizations stub — now uses GetOrganizationsByUserID query
  - Fixed ListProjects stub — now aggregates projects from user's organizations
  - Fixed CreateProject — now uses user's first organization instead of random UUID
  - Added unit tests: services/prompts_test.go (template compilation), auth/jwt_test.go (JWT), auth/password_test.go (bcrypt)
  - Added integration tests: api/handlers_test.go (validation, error handling, middleware)
  - Created OpenAPI 3.0 specification: docs/openapi.yaml — full API reference
  - Removed unused uuid import from projects.go
  - Installed Go 1.22.5 locally, all checks pass: gofmt clean, go vet clean, go build clean, go test 3/3 packages pass

## Decisions made (don't re-decide these)
- Router: Chi (chosen in Phase 1)
- Pagination style: limit/offset (chosen in Phase 3)
- Auth header name for API keys: `x-api-key` (Phase 9)
- sqlc version: v1.25.0
- Frontend styling: Tailwind CSS v4 with @tailwindcss/vite plugin
- Frontend state: Svelte 5 runes mode + writable stores
- API proxy: Vite dev server proxies /api and /health to localhost:8080
- Analytics queries: raw SQL in queries.sql for aggregation (avg/min/max latency, cost, tokens, error rate)
- Score sources: USER, EVALUATOR, SDK (per DB constraint)
- SDK API auth: x-api-key header (validates against api_keys table, extracts project_id)

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

## Phase 6 API Endpoints Added
- POST `/api/datasets` — create dataset (project_id, name, description)
- GET `/api/datasets` — list datasets (project_id)
- GET `/api/datasets/{id}` — get dataset
- DELETE `/api/datasets/{id}` — delete dataset
- POST `/api/datasets/{id}/items` — add item (input, expected_output, metadata, source_trace_id)
- GET `/api/datasets/{id}/items` — list items
- POST `/api/datasets/{id}/import` — import JSON/CSV (Content-Type determines format)
- GET `/api/datasets/{id}/export` — export items (?format=json|csv)
- POST `/api/datasets/{id}/runs` — create run (name)
- GET `/api/datasets/{id}/runs` — list runs
- GET `/api/datasets/{id}/runs/{runId}` — get run
- POST `/api/datasets/{id}/runs/{runId}/items` — add run item (dataset_item_id, observation_id, score_id)
- GET `/api/datasets/{id}/runs/{runId}/items` — list run items
- GET `/api/datasets/{id}/runs/{runId}/export` — export run results (?format=json|csv)

## Phase 9 API Endpoints Added (SDK-compatible, API key auth)
- POST `/api/public/traces` — create trace (x-api-key auth, body uses camelCase: userId, sessionId, startTime, endTime)
- GET `/api/public/traces` — list traces (x-api-key auth, ?limit=&page=&name=)
- GET `/api/public/traces/{traceId}` — get trace with observations
- POST `/api/public/observations` — create observation (x-api-key auth, body uses camelCase: traceId, modelParameters, parentObservationId)
- POST `/api/public/ingestion` — batch ingestion (x-api-key auth, body: {batch: [{id, type, timestamp, body}]})
  - Supported types: trace-create, trace-update, observation-create

## Phase 9 Files Changed
- internal/auth/middleware.go — added APIKeyMiddleware, GetProjectID, GetAPIKeyID, ProjectIDKey, APIKeyIDKey
- internal/api/sdk.go — new file, SDK-compatible endpoints
- internal/api/router.go — added SDK route group with API key middleware
- internal/api/projects.go — fixed ListOrganizations, ListProjects, CreateProject stubs
- tests/services/prompts_test.go — new file, template compilation tests
- tests/auth/jwt_test.go — new file, JWT generation/validation tests
- tests/auth/password_test.go — new file, bcrypt hashing tests
- tests/api/handlers_test.go — new file, API handler validation tests
- docs/openapi.yaml — new file, OpenAPI 3.0 specification

## Known limitations (intentional, per AGENTS.md)
- no ClickHouse, no S3, single worker, etc.
