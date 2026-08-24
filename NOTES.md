# NOTES.md — Langfuse Light

## Session log
- [2026-08-24] Phase 1: Foundation setup complete - Go module, SvelteKit, Docker, migrations, sqlc
- [2026-08-24] Phase 1: sqlc code generated, Chi router selected
- [2026-08-24] Phase 2: Auth system implemented - password hashing, JWT, register/login endpoints
- [2026-08-24] Phase 2: Project and organization CRUD endpoints added

## Decisions made (don't re-decide these)
- Router: Chi (chosen in Phase 1)
- Pagination style: TBD (Phase 3)
- Auth header name for API keys: TBD (Phase 2)
- sqlc version: v1.25.0

## TODO (found while working)
- ...

## Known limitations (intentional, per AGENTS.md)
- no ClickHouse, no S3, single worker, etc.
