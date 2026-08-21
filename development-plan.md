# Langfuse Light — Development Plan

## Overview

This plan breaks the development of Langfuse Light into manageable phases that can be implemented across multiple sessions. Each phase has clear goals, deliverables, and can be worked on independently.

---

## Phase 1: Foundation & Infrastructure (Session 1)

### Goals
- Set up project structure and development environment
- Configure database and basic tooling
- Establish development workflow

### Tasks
1. **Project initialization**
   - Initialize Go module (`go mod init`)
   - Set up SvelteKit project in `web/` directory
   - Create `Makefile` with common commands
   - Set up `.env.example` with required environment variables

2. **Database setup**
   - Create PostgreSQL migration files (`migrations/001_init.up.sql`)
   - Set up sqlc configuration (`sqlc.yaml`)
   - Generate sqlc code from queries
   - Test database connection

3. **Docker configuration**
   - Create `Dockerfile` for Go backend
   - Create `docker-compose.yml` with PostgreSQL and Redis
   - Test container startup

4. **Development tools**
   - Set up Go linter (golangci-lint)
   - Configure SvelteKit development server
   - Create basic CI script

### Deliverables
- Working Go module with basic structure
- SvelteKit project scaffold
- Docker Compose with PostgreSQL and Redis
- Migration files ready to apply

---

## Phase 2: Authentication & User Management (Session 2)

### Goals
- Implement user registration and login
- Set up JWT authentication
- Create organization and project management

### Tasks
1. **Auth system**
   - Implement password hashing (bcrypt)
   - Create JWT token generation/validation
   - Build login/register endpoints
   - Create auth middleware

2. **User management**
   - User CRUD operations
   - Organization creation and management
   - Project creation and API key generation
   - Role-based access control (viewer/editor/admin)

3. **Database queries**
   - Write sqlc queries for users, organizations, projects
   - Implement API key validation
   - Create membership management queries

### Deliverables
- Working auth endpoints (register, login, refresh)
- JWT middleware for protected routes
- Organization and project CRUD
- API key generation and validation

---

## Phase 3: Core API — Traces & Observations (Session 3)

### Goals
- Implement trace ingestion API
- Create observation (span/generation/event) handling
- Build basic query endpoints

### Tasks
1. **Trace ingestion**
   - POST `/api/traces` — create/update traces
   - POST `/api/observations` — create observations
   - Batch ingestion endpoint
   - Input validation and sanitization

2. **Query endpoints**
   - GET `/api/traces` — list traces with filters
   - GET `/api/traces/:id` — get trace with observations
   - GET `/api/observations/:id` — get observation detail
   - Pagination and filtering

3. **Data processing**
   - Redis queue for async ingestion
   - Background worker for trace processing
   - Cost calculation logic
   - Token usage aggregation

### Deliverables
- Trace and observation ingestion API
- Query endpoints with filtering
- Background worker for async processing
- Basic error handling

---

## Phase 4: Prompt Management (Session 4)

### Goals
- Implement prompt versioning system
- Create prompt template compilation
- Build prompt CRUD API

### Tasks
1. **Prompt CRUD**
   - POST `/api/prompts` — create prompt
   - GET `/api/prompts` — list prompts
   - GET `/api/prompts/:name` — get prompt by name
   - PUT `/api/prompts/:name` — update prompt (create new version)

2. **Prompt versioning**
   - Automatic version increment
   - Active version management
   - Prompt history tracking

3. **Template compilation**
   - Variable substitution in prompts
   - Template validation
   - SDK-side caching support

### Deliverables
- Prompt CRUD API
- Version management system
- Template compilation engine

---

## Phase 5: Evaluation & Scoring (Session 5)

### Goals
- Implement scoring system
- Create evaluation endpoints
- Build LLM-as-a-judge framework

### Tasks
1. **Score management**
   - POST `/api/scores` — create score
   - GET `/api/traces/:id/scores` — get trace scores
   - Score aggregation and statistics

2. **Evaluation framework**
   - Code evaluators
   - LLM-as-a-judge integration
   - User feedback collection
   - Manual labeling support

3. **Analytics endpoints**
   - Average latency calculations
   - Cost analytics
   - Token usage statistics
   - Error rate tracking

### Deliverables
- Scoring API
- Evaluation framework
- Basic analytics endpoints

---

## Phase 6: Datasets & Batch Evaluation (Session 6)

### Goals
- Implement dataset management
- Create batch evaluation runs
- Build dataset import/export

### Tasks
1. **Dataset CRUD**
   - POST `/api/datasets` — create dataset
   - GET `/api/datasets` — list datasets
   - POST `/api/datasets/:id/items` — add items
   - Import from JSON/CSV

2. **Batch evaluation**
   - Create dataset runs
   - Execute evaluations on dataset items
   - Track run results and scores
   - Compare run results

3. **Export functionality**
   - Export dataset items
   - Export evaluation results
   - CSV/JSON export formats

### Deliverables
- Dataset management API
- Batch evaluation system
- Import/export functionality

---

## Phase 7: Frontend — Dashboard (Session 7)

### Goals
- Build main dashboard UI
- Create trace visualization
- Implement prompt management interface

### Tasks
1. **Dashboard layout**
   - Navigation and routing
   - Project selection
   - Responsive design

2. **Trace views**
   - Trace list with filters
   - Trace detail with observation tree
   - Timeline visualization
   - Cost and token display

3. **Prompt management UI**
   - Prompt list and search
   - Prompt editor with versioning
   - Template preview

### Deliverables
- Working dashboard with navigation
- Trace list and detail views
- Prompt management interface

---

## Phase 8: Frontend — Analytics & Settings (Session 8)

### Goals
- Build analytics dashboard
- Create settings pages
- Implement user management UI

### Tasks
1. **Analytics dashboard**
   - Cost over time chart
   - Latency distribution
   - Token usage trends
   - Error rate monitoring

2. **Settings pages**
   - Project settings
   - API key management
   - User profile

3. **Organization management**
   - Member list and roles
   - Invite users
   - Organization settings

### Deliverables
- Analytics dashboard with charts
- Settings and configuration pages
- User and organization management

---

## Phase 9: SDK Compatibility & Testing (Session 9)

### Goals
- Ensure SDK compatibility
- Write comprehensive tests
- Create API documentation

### Tasks
1. **SDK compatibility**
   - Python SDK endpoint compatibility
   - JavaScript/TypeScript SDK compatibility
   - OpenAPI specification

2. **Testing**
   - Unit tests for services
   - Integration tests for API
   - End-to-end tests

3. **Documentation**
   - API documentation (OpenAPI/Swagger)
   - Getting started guide
   - Configuration reference

### Deliverables
- SDK-compatible API endpoints
- Comprehensive test suite
- Complete API documentation

---

## Phase 10: Deployment & Finalization (Session 10)

### Goals
- Finalize Docker setup
- Create deployment documentation
- Performance optimization

### Tasks
1. **Docker optimization**
   - Multi-stage builds
   - Image size optimization
   - Health checks

2. **Deployment documentation**
   - Docker Compose deployment guide
   - Environment variable reference
   - Troubleshooting guide

3. **Performance optimization**
   - Database query optimization
   - Caching strategies
   - Memory usage optimization

### Deliverables
- Optimized Docker images
- Complete deployment documentation
- Performance benchmarks

---

## Session Planning Guidelines

### Recommended Session Order
1. **Foundation first** — Always start with Phase 1
2. **Backend before frontend** — Complete Phases 2-6 before Phase 7-8
3. **Core before features** — Traces and auth before advanced features
4. **Testing throughout** — Write tests as you develop each phase

### Session Length Estimates
- **Phase 1**: 2-3 hours (setup heavy)
- **Phase 2-6**: 3-4 hours each (core development)
- **Phase 7-8**: 4-5 hours each (frontend development)
- **Phase 9**: 3-4 hours (testing and docs)
- **Phase 10**: 2-3 hours (finalization)

### Dependencies Between Phases
```
Phase 1 → Phase 2 → Phase 3 → Phase 4 → Phase 5 → Phase 6
                                          ↓
                                    Phase 7 → Phase 8
                                          ↓
                                    Phase 9 → Phase 10
```

### Can Be Parallelized
- Phase 7-8 (frontend) can start after Phase 3 (basic API)
- Phase 9 (testing) can start after Phase 5 (core features)
- Documentation can be written alongside development

---

## Technical Notes

### Database Migrations
- Use PostgreSQL-native features (UUID, JSONB, arrays)
- Create indexes for common query patterns
- Consider partitioning for traces table if needed

### Go Backend Structure
- Follow standard Go project layout
- Use interfaces for testability
- Implement proper error handling
- Use context for request-scoped values

### SvelteKit Frontend
- Use SvelteKit's file-based routing
- Implement proper loading states
- Use stores for state management
- Follow accessibility guidelines

### Docker Setup
- Use multi-stage builds for smaller images
- Implement health checks
- Use volumes for data persistence
- Configure proper logging

---

## Success Metrics

### Functional
- [ ] User registration and login works
- [ ] Traces can be ingested via API
- [ ] Observations can be created and queried
- [ ] Prompts can be versioned and managed
- [ ] Scores can be created and aggregated
- [ ] Datasets can be created and evaluated
- [ ] Dashboard displays all data correctly

### Technical
- [ ] API response time < 100ms for common operations
- [ ] System uses < 400MB RAM
- [ ] Docker startup < 3 minutes
- [ ] 80%+ test coverage for core logic
- [ ] Clean, maintainable code structure

### User Experience
- [ ] Intuitive dashboard navigation
- [ ] Clear error messages
- [ ] Responsive design
- [ ] Good performance on small teams

---

## Notes

- Each phase can be developed independently
- Phases can be paused and resumed across sessions
- Focus on core functionality first
- Keep the codebase simple and maintainable
- Document decisions and trade-offs
