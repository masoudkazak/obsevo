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

## Decisions made (don't re-decide these)
- Router: Chi (chosen in Phase 1)
- Pagination style: limit/offset (chosen in Phase 3)
- Auth header name for API keys: TBD (Phase 2)
- sqlc version: v1.25.0
- Frontend styling: Tailwind CSS v4 with @tailwindcss/vite plugin
- Frontend state: Svelte 5 runes mode + writable stores
- API proxy: Vite dev server proxies /api and /health to localhost:8080

## TODO (found while working)
- ...

## Known limitations (intentional, per AGENTS.md)
- no ClickHouse, no S3, single worker, etc.
