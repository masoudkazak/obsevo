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

## Decisions made (don't re-decide these)
- Router: Chi (chosen in Phase 1)
- Pagination style: limit/offset (chosen in Phase 3)
- Auth header name for API keys: TBD (Phase 2)
- sqlc version: v1.25.0

## TODO (found while working)
- ...

## Known limitations (intentional, per AGENTS.md)
- no ClickHouse, no S3, single worker, etc.
