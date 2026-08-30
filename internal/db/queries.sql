-- queries.sql
-- Type-safe SQL queries for sqlc

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1;

-- name: CreateUser :one
INSERT INTO users (email, password_hash, name)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetOrganizationByID :one
SELECT * FROM organizations WHERE id = $1;

-- name: CreateOrganization :one
INSERT INTO organizations (name)
VALUES ($1)
RETURNING *;

-- name: GetMemberByUserAndOrg :one
SELECT * FROM members WHERE user_id = $1 AND org_id = $2;

-- name: CreateMember :one
INSERT INTO members (user_id, org_id, role)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetOrganizationsByUserID :many
SELECT o.* FROM organizations o
JOIN members m ON m.org_id = o.id
WHERE m.user_id = $1
ORDER BY o.created_at DESC;

-- name: GetMembersByOrgID :many
SELECT m.*, u.email, u.name as user_name FROM members m
JOIN users u ON u.id = m.user_id
WHERE m.org_id = $1
ORDER BY m.id;

-- name: GetProjectByID :one
SELECT * FROM projects WHERE id = $1;

-- name: GetProjectsByOrgID :many
SELECT * FROM projects WHERE org_id = $1 ORDER BY created_at DESC;

-- name: CreateProject :one
INSERT INTO projects (name, org_id)
VALUES ($1, $2)
RETURNING *;

-- name: GetAPIKeyByKey :one
SELECT * FROM api_keys WHERE key = $1;

-- name: GetAPIKeysByProjectID :many
SELECT * FROM api_keys WHERE project_id = $1 ORDER BY created_at DESC;

-- name: CreateAPIKey :one
INSERT INTO api_keys (project_id, key, name)
VALUES ($1, $2, $3)
RETURNING *;

-- name: DeleteAPIKey :exec
DELETE FROM api_keys WHERE id = $1 AND project_id = $2;

-- name: GetTraceByID :one
SELECT * FROM traces WHERE id = $1;

-- name: GetTracesByProjectID :many
SELECT * FROM traces 
WHERE project_id = $1 
ORDER BY start_time DESC
LIMIT $2 OFFSET $3;

-- name: GetTracesByProjectIDAndName :many
SELECT * FROM traces 
WHERE project_id = $1 AND name = $2
ORDER BY start_time DESC
LIMIT $3 OFFSET $4;

-- name: CountTracesByProjectID :one
SELECT COUNT(*) FROM traces WHERE project_id = $1;

-- name: GetTraceWithProject :one
SELECT t.*, p.name as project_name FROM traces t
JOIN projects p ON p.id = t.project_id
WHERE t.id = $1;

-- name: CreateTrace :one
INSERT INTO traces (id, project_id, name, input, output, metadata, user_id, session_id, tags, start_time, end_time, total_cost, token_usage)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING *;

-- name: UpdateTrace :one
UPDATE traces
SET name = COALESCE($2, name),
    input = COALESCE($3, input),
    output = COALESCE($4, output),
    metadata = COALESCE($5, metadata),
    end_time = COALESCE($6, end_time),
    total_cost = COALESCE($7, total_cost),
    token_usage = COALESCE($8, token_usage)
WHERE id = $1
RETURNING *;

-- name: GetObservationByID :one
SELECT * FROM observations WHERE id = $1;

-- name: GetObservationsByTraceID :many
SELECT * FROM observations 
WHERE trace_id = $1 
ORDER BY start_time ASC;

-- name: CreateObservation :one
INSERT INTO observations (trace_id, type, name, input, output, metadata, model, model_parameters, start_time, end_time, token_usage, cost, status, parent_observation_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
RETURNING *;

-- name: GetPromptByID :one
SELECT * FROM prompts WHERE id = $1;

-- name: GetPromptByNameAndVersion :one
SELECT * FROM prompts WHERE project_id = $1 AND name = $2 AND version = $3;

-- name: GetActivePromptByName :one
SELECT * FROM prompts WHERE project_id = $1 AND name = $2 AND is_active = true ORDER BY version DESC LIMIT 1;

-- name: GetPromptsByProjectID :many
SELECT * FROM prompts 
WHERE project_id = $1 
ORDER BY name, version DESC;

-- name: CreatePrompt :one
INSERT INTO prompts (project_id, name, version, prompt, config, is_active)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: UpdatePromptActive :exec
UPDATE prompts SET is_active = $3 WHERE project_id = $1 AND name = $2;

-- name: UpdatePromptActiveByVersion :exec
UPDATE prompts SET is_active = $3 WHERE project_id = $1 AND name = $2 AND version = $4;

-- name: GetScoreByID :one
SELECT * FROM scores WHERE id = $1;

-- name: GetScoresByTraceID :many
SELECT * FROM scores WHERE trace_id = @trace_id::TEXT ORDER BY created_at DESC;

-- name: CreateScore :one
INSERT INTO scores (
  id, project_id, trace_id, observation_id, session_id, dataset_run_id,
  name, value, string_value, data_type, comment, source, user_id, config_id, metadata
)
VALUES (
  COALESCE(NULLIF(@id::TEXT, ''), gen_random_uuid()::TEXT),
  @project_id::TEXT,
  NULLIF(@trace_id::TEXT, ''),
  NULLIF(@observation_id::TEXT, ''),
  NULLIF(@session_id::TEXT, ''),
  NULLIF(@dataset_run_id::TEXT, ''),
  @name::TEXT,
  sqlc.narg('value')::DOUBLE PRECISION,
  NULLIF(@string_value::TEXT, ''),
  @data_type::TEXT,
  NULLIF(@comment::TEXT, ''),
  @source::TEXT,
  NULLIF(@user_id::TEXT, ''),
  NULLIF(@config_id::TEXT, ''),
  @metadata
)
ON CONFLICT (id) DO UPDATE SET
  value        = EXCLUDED.value,
  string_value = EXCLUDED.string_value,
  comment      = EXCLUDED.comment,
  metadata     = EXCLUDED.metadata
RETURNING *;

-- name: GetScoresByTraceIDAndName :many
SELECT * FROM scores
WHERE trace_id = @trace_id::TEXT AND name = @name::TEXT
ORDER BY created_at DESC;

-- name: GetScoreByIDAndProject :one
SELECT * FROM scores WHERE id = $1 AND project_id = $2;

-- name: ListScores :many
SELECT * FROM scores
WHERE project_id = @project_id::TEXT
  AND (NULLIF(@name::TEXT, '')           IS NULL OR name           = @name::TEXT)
  AND (NULLIF(@source::TEXT, '')         IS NULL OR source         = @source::TEXT)
  AND (NULLIF(@trace_id::TEXT, '')       IS NULL OR trace_id       = @trace_id::TEXT)
  AND (NULLIF(@observation_id::TEXT, '') IS NULL OR observation_id = @observation_id::TEXT)
  AND (NULLIF(@data_type::TEXT, '')      IS NULL OR data_type      = @data_type::TEXT)
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountScores :one
SELECT COUNT(*) FROM scores
WHERE project_id = @project_id::TEXT
  AND (NULLIF(@name::TEXT, '')   IS NULL OR name   = @name::TEXT)
  AND (NULLIF(@source::TEXT, '') IS NULL OR source = @source::TEXT);

-- name: DeleteScore :exec
DELETE FROM scores WHERE id = $1 AND project_id = $2;

-- name: GetScoreAggregationByProjectID :many
SELECT s.name, COUNT(*) as count, AVG(s.value) as avg_value, MIN(s.value) as min_value, MAX(s.value) as max_value
FROM scores s
WHERE s.project_id = $1
GROUP BY s.name
ORDER BY s.name;

-- name: GetDatasetByID :one
SELECT * FROM datasets WHERE id = $1;

-- name: GetDatasetsByProjectID :many
SELECT * FROM datasets WHERE project_id = $1 ORDER BY created_at DESC;

-- name: CreateDataset :one
INSERT INTO datasets (project_id, name, description)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetDatasetItemByID :one
SELECT * FROM dataset_items WHERE id = $1;

-- name: GetDatasetItemsByDatasetID :many
SELECT * FROM dataset_items WHERE dataset_id = $1 ORDER BY created_at;

-- name: CreateDatasetItem :one
INSERT INTO dataset_items (dataset_id, input, expected_output, metadata, source_trace_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetDatasetRunByID :one
SELECT * FROM dataset_runs WHERE id = $1;

-- name: GetDatasetRunsByDatasetID :many
SELECT * FROM dataset_runs WHERE dataset_id = $1 ORDER BY created_at DESC;

-- name: CreateDatasetRun :one
INSERT INTO dataset_runs (dataset_id, name)
VALUES ($1, $2)
RETURNING *;

-- name: GetTraceLatencyStatsByProjectID :one
SELECT
  COUNT(*) as total_traces,
  COALESCE(AVG(EXTRACT(EPOCH FROM (end_time - start_time))), 0) as avg_latency_seconds,
  COALESCE(MIN(EXTRACT(EPOCH FROM (end_time - start_time))), 0) as min_latency_seconds,
  COALESCE(MAX(EXTRACT(EPOCH FROM (end_time - start_time))), 0) as max_latency_seconds
FROM traces
WHERE project_id = $1 AND end_time IS NOT NULL AND start_time IS NOT NULL;

-- name: GetTraceCostStatsByProjectID :one
SELECT
  COUNT(*) as total_traces,
  COALESCE(SUM(total_cost), 0) as total_cost,
  COALESCE(AVG(total_cost), 0) as avg_cost,
  COALESCE(MIN(total_cost), 0) as min_cost,
  COALESCE(MAX(total_cost), 0) as max_cost
FROM traces
WHERE project_id = $1 AND total_cost IS NOT NULL;

-- name: GetTraceTokenUsageStatsByProjectID :one
SELECT
  COUNT(*) as total_traces,
  COALESCE(SUM((token_usage->>'total_tokens')::bigint), 0) as total_tokens,
  COALESCE(AVG((token_usage->>'total_tokens')::bigint), 0) as avg_tokens,
  COALESCE(SUM((token_usage->>'input_tokens')::bigint), 0) as total_input_tokens,
  COALESCE(SUM((token_usage->>'output_tokens')::bigint), 0) as total_output_tokens
FROM traces
WHERE project_id = $1 AND token_usage IS NOT NULL;

-- name: GetTraceErrorRateByProjectID :one
SELECT
  COUNT(*) as total_traces,
  COUNT(*) FILTER (WHERE EXISTS (
    SELECT 1 FROM observations o WHERE o.trace_id = traces.id AND o.status = 'ERROR'
  )) as error_traces
FROM traces
WHERE traces.project_id = $1;

-- name: GetTraceCountByProjectIDAndTimeRange :one
SELECT COUNT(*) as count
FROM traces
WHERE project_id = $1 AND start_time >= $2 AND start_time < $3;

-- name: GetDatasetRunItemByID :one
SELECT * FROM dataset_run_items WHERE id = $1;

-- name: GetDatasetRunItemsByRunID :many
SELECT * FROM dataset_run_items WHERE dataset_run_id = $1 ORDER BY created_at;

-- name: CreateDatasetRunItem :one
INSERT INTO dataset_run_items (dataset_run_id, dataset_item_id, observation_id, score_id)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: DeleteDataset :exec
DELETE FROM datasets WHERE id = $1;

-- name: CountDatasetItemsByDatasetID :one
SELECT COUNT(*) FROM dataset_items WHERE dataset_id = $1;

-- name: GetTraceCostOverTimeByProjectID :many
SELECT
  date_trunc('day', start_time)::timestamptz as time_bucket,
  COUNT(*) as trace_count,
  COALESCE(SUM(total_cost), 0) as total_cost,
  COALESCE(AVG(total_cost), 0) as avg_cost
FROM traces
WHERE project_id = $1 AND start_time >= $2 AND start_time < $3 AND total_cost IS NOT NULL
GROUP BY date_trunc('day', start_time)
ORDER BY time_bucket;

-- name: GetTraceLatencyOverTimeByProjectID :many
SELECT
  date_trunc('day', start_time)::timestamptz as time_bucket,
  COUNT(*) as trace_count,
  COALESCE(AVG(EXTRACT(EPOCH FROM (end_time - start_time))), 0) as avg_latency_seconds,
  COALESCE(MIN(EXTRACT(EPOCH FROM (end_time - start_time))), 0) as min_latency_seconds,
  COALESCE(MAX(EXTRACT(EPOCH FROM (end_time - start_time))), 0) as max_latency_seconds
FROM traces
WHERE project_id = $1 AND start_time >= $2 AND start_time < $3 AND end_time IS NOT NULL AND start_time IS NOT NULL
GROUP BY date_trunc('day', start_time)
ORDER BY time_bucket;

-- name: GetTraceTokenUsageOverTimeByProjectID :many
SELECT
  date_trunc('day', start_time)::timestamptz as time_bucket,
  COUNT(*) as trace_count,
  COALESCE(SUM((token_usage->>'total_tokens')::bigint), 0) as total_tokens,
  COALESCE(SUM((token_usage->>'input_tokens')::bigint), 0) as total_input_tokens,
  COALESCE(SUM((token_usage->>'output_tokens')::bigint), 0) as total_output_tokens
FROM traces
WHERE project_id = $1 AND start_time >= $2 AND start_time < $3 AND token_usage IS NOT NULL
GROUP BY date_trunc('day', start_time)
ORDER BY time_bucket;

-- name: GetTraceCountOverTimeByProjectID :many
SELECT
  date_trunc('day', t.start_time)::timestamptz as time_bucket,
  COUNT(*) as trace_count,
  COUNT(*) FILTER (WHERE EXISTS (
    SELECT 1 FROM observations o WHERE o.trace_id = t.id AND o.status = 'ERROR'
  )) as error_count
FROM traces t
WHERE t.project_id = $1 AND t.start_time >= $2 AND t.start_time < $3
GROUP BY date_trunc('day', t.start_time)
ORDER BY time_bucket;

-- name: UpdateMemberRole :exec
UPDATE members SET role = $3 WHERE user_id = $1 AND org_id = $2;

-- name: DeleteMember :exec
DELETE FROM members WHERE user_id = $1 AND org_id = $2;

-- name: UpdateUserName :exec
UPDATE users SET name = $2 WHERE id = $1;

-- name: GetEvaluatorConfigByID :one
SELECT * FROM evaluator_configs WHERE id = $1;

-- name: GetEvaluatorConfigsByProjectID :many
SELECT * FROM evaluator_configs WHERE project_id = $1 ORDER BY created_at DESC;

-- name: CreateEvaluatorConfig :one
INSERT INTO evaluator_configs (project_id, name, description, type, config, is_active)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: UpdateEvaluatorConfig :exec
UPDATE evaluator_configs
SET name = COALESCE($2, name),
    description = COALESCE($3, description),
    config = COALESCE($4, config),
    is_active = COALESCE($5, is_active)
WHERE id = $1;

-- name: DeleteEvaluatorConfig :exec
DELETE FROM evaluator_configs WHERE id = $1 AND project_id = $2;

-- name: GetEvaluatorConfigByIDAndProject :one
SELECT * FROM evaluator_configs WHERE id = $1 AND project_id = $2;

-- name: GetEvaluationRunByID :one
SELECT * FROM evaluation_runs WHERE id = $1;

-- name: GetEvaluationRunByIDAndProject :one
SELECT * FROM evaluation_runs WHERE id = $1 AND project_id = $2;

-- name: GetEvaluationRunsByProjectID :many
SELECT * FROM evaluation_runs WHERE project_id = $1 ORDER BY created_at DESC;

-- name: CreateEvaluationRun :one
INSERT INTO evaluation_runs (project_id, evaluator_config_id, name, status, result_summary)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdateEvaluationRunStatus :exec
UPDATE evaluation_runs SET status = $2, result_summary = $3 WHERE id = $1;

-- ===========================================================================
-- Authorization
-- ===========================================================================

-- name: GetProjectForUser :one
-- Returns the project only if the user belongs to the organization owning it.
SELECT p.* FROM projects p
JOIN members m ON m.org_id = p.org_id
WHERE p.id = $1 AND m.user_id = $2;

-- name: GetMemberRoleForProject :one
-- Returns the caller's role in the organization owning the project.
SELECT m.role FROM projects p
JOIN members m ON m.org_id = p.org_id
WHERE p.id = $1 AND m.user_id = $2;

-- ===========================================================================
-- Traces — upsert and project-scoped reads
-- ===========================================================================

-- name: UpsertTrace :one
-- Creates a trace, or merges non-empty fields into an existing one. Ingestion
-- is retried and reordered by SDKs, so this must be idempotent.
INSERT INTO traces (
  id, project_id, name, input, output, metadata, user_id, session_id, tags,
  start_time, end_time, total_cost, token_usage, release, version, public,
  bookmarked, environment
)
VALUES (
  @id::TEXT, @project_id::TEXT,
  NULLIF(@name::TEXT, ''), @input, @output, @metadata,
  NULLIF(@user_id::TEXT, ''), NULLIF(@session_id::TEXT, ''), @tags::TEXT[],
  @start_time, sqlc.narg('end_time')::TIMESTAMPTZ,
  sqlc.narg('total_cost')::DOUBLE PRECISION, @token_usage,
  NULLIF(@release::TEXT, ''), NULLIF(@version::TEXT, ''),
  @public::BOOLEAN, @bookmarked::BOOLEAN, @environment::TEXT
)
ON CONFLICT (id) DO UPDATE SET
  name        = COALESCE(EXCLUDED.name, traces.name),
  input       = COALESCE(EXCLUDED.input, traces.input),
  output      = COALESCE(EXCLUDED.output, traces.output),
  metadata    = COALESCE(EXCLUDED.metadata, traces.metadata),
  user_id     = COALESCE(EXCLUDED.user_id, traces.user_id),
  session_id  = COALESCE(EXCLUDED.session_id, traces.session_id),
  tags        = CASE WHEN cardinality(EXCLUDED.tags) > 0 THEN EXCLUDED.tags ELSE traces.tags END,
  end_time    = COALESCE(EXCLUDED.end_time, traces.end_time),
  total_cost  = COALESCE(EXCLUDED.total_cost, traces.total_cost),
  token_usage = COALESCE(EXCLUDED.token_usage, traces.token_usage),
  release     = COALESCE(EXCLUDED.release, traces.release),
  version     = COALESCE(EXCLUDED.version, traces.version),
  updated_at  = now()
WHERE traces.project_id = EXCLUDED.project_id
RETURNING *;

-- name: GetTraceByIDAndProject :one
SELECT * FROM traces WHERE id = $1 AND project_id = $2;

-- name: DeleteTrace :exec
DELETE FROM traces WHERE id = $1 AND project_id = $2;

-- name: ListTracesFiltered :many
SELECT * FROM traces
WHERE project_id = @project_id::TEXT
  AND (NULLIF(@name::TEXT, '')        IS NULL OR name        = @name::TEXT)
  AND (NULLIF(@user_id::TEXT, '')     IS NULL OR user_id     = @user_id::TEXT)
  AND (NULLIF(@session_id::TEXT, '')  IS NULL OR session_id  = @session_id::TEXT)
  AND (NULLIF(@release::TEXT, '')     IS NULL OR release     = @release::TEXT)
  AND (NULLIF(@version::TEXT, '')     IS NULL OR version     = @version::TEXT)
  AND (NULLIF(@environment::TEXT, '') IS NULL OR environment = @environment::TEXT)
  AND (cardinality(@tags::TEXT[]) = 0 OR tags @> @tags::TEXT[])
  AND (sqlc.narg('from_time')::TIMESTAMPTZ IS NULL OR start_time >= sqlc.narg('from_time')::TIMESTAMPTZ)
  AND (sqlc.narg('to_time')::TIMESTAMPTZ   IS NULL OR start_time <  sqlc.narg('to_time')::TIMESTAMPTZ)
ORDER BY start_time DESC
LIMIT $1 OFFSET $2;

-- name: CountTracesFiltered :one
SELECT COUNT(*) FROM traces
WHERE project_id = @project_id::TEXT
  AND (NULLIF(@name::TEXT, '')        IS NULL OR name        = @name::TEXT)
  AND (NULLIF(@user_id::TEXT, '')     IS NULL OR user_id     = @user_id::TEXT)
  AND (NULLIF(@session_id::TEXT, '')  IS NULL OR session_id  = @session_id::TEXT)
  AND (NULLIF(@release::TEXT, '')     IS NULL OR release     = @release::TEXT)
  AND (NULLIF(@version::TEXT, '')     IS NULL OR version     = @version::TEXT)
  AND (NULLIF(@environment::TEXT, '') IS NULL OR environment = @environment::TEXT)
  AND (cardinality(@tags::TEXT[]) = 0 OR tags @> @tags::TEXT[])
  AND (sqlc.narg('from_time')::TIMESTAMPTZ IS NULL OR start_time >= sqlc.narg('from_time')::TIMESTAMPTZ)
  AND (sqlc.narg('to_time')::TIMESTAMPTZ   IS NULL OR start_time <  sqlc.narg('to_time')::TIMESTAMPTZ);

-- ===========================================================================
-- Observations — upsert and project-scoped reads
-- ===========================================================================

-- name: UpsertObservation :one
-- Honours a client-supplied id so that parentObservationId links resolve and
-- the create-then-update span lifecycle works.
INSERT INTO observations (
  id, trace_id, project_id, type, name, input, output, metadata, model,
  model_parameters, start_time, end_time, completion_start_time, token_usage,
  usage_details, cost_details, cost, status, level, status_message,
  parent_observation_id, prompt_id, prompt_name, prompt_version, version, environment
)
VALUES (
  COALESCE(NULLIF(@id::TEXT, ''), gen_random_uuid()::TEXT),
  @trace_id::TEXT, @project_id::TEXT, @type::TEXT,
  NULLIF(@name::TEXT, ''), @input, @output, @metadata,
  NULLIF(@model::TEXT, ''), @model_parameters,
  @start_time,
  sqlc.narg('end_time')::TIMESTAMPTZ,
  sqlc.narg('completion_start_time')::TIMESTAMPTZ,
  @token_usage, @usage_details, @cost_details,
  sqlc.narg('cost')::DOUBLE PRECISION,
  @status::TEXT, @level::TEXT, NULLIF(@status_message::TEXT, ''),
  NULLIF(@parent_observation_id::TEXT, ''),
  NULLIF(@prompt_id::TEXT, ''), NULLIF(@prompt_name::TEXT, ''),
  sqlc.narg('prompt_version')::INT,
  NULLIF(@version::TEXT, ''), @environment::TEXT
)
ON CONFLICT (id) DO UPDATE SET
  name                  = COALESCE(EXCLUDED.name, observations.name),
  type                  = EXCLUDED.type,
  input                 = COALESCE(EXCLUDED.input, observations.input),
  output                = COALESCE(EXCLUDED.output, observations.output),
  metadata              = COALESCE(EXCLUDED.metadata, observations.metadata),
  model                 = COALESCE(EXCLUDED.model, observations.model),
  model_parameters      = COALESCE(EXCLUDED.model_parameters, observations.model_parameters),
  end_time              = COALESCE(EXCLUDED.end_time, observations.end_time),
  completion_start_time = COALESCE(EXCLUDED.completion_start_time, observations.completion_start_time),
  token_usage           = COALESCE(EXCLUDED.token_usage, observations.token_usage),
  usage_details         = COALESCE(EXCLUDED.usage_details, observations.usage_details),
  cost_details          = COALESCE(EXCLUDED.cost_details, observations.cost_details),
  cost                  = COALESCE(EXCLUDED.cost, observations.cost),
  status                = EXCLUDED.status,
  level                 = EXCLUDED.level,
  status_message        = COALESCE(EXCLUDED.status_message, observations.status_message),
  parent_observation_id = COALESCE(EXCLUDED.parent_observation_id, observations.parent_observation_id),
  prompt_id             = COALESCE(EXCLUDED.prompt_id, observations.prompt_id),
  prompt_name           = COALESCE(EXCLUDED.prompt_name, observations.prompt_name),
  prompt_version        = COALESCE(EXCLUDED.prompt_version, observations.prompt_version)
WHERE observations.project_id = EXCLUDED.project_id
RETURNING *;

-- name: GetObservationByIDAndProject :one
SELECT * FROM observations WHERE id = $1 AND project_id = $2;

-- name: GetObservationsByTraceIDAndProject :many
SELECT * FROM observations
WHERE trace_id = $1 AND project_id = $2
ORDER BY start_time ASC;

-- name: ListObservationsFiltered :many
SELECT * FROM observations
WHERE project_id = @project_id::TEXT
  AND (NULLIF(@trace_id::TEXT, '') IS NULL OR trace_id = @trace_id::TEXT)
  AND (NULLIF(@type::TEXT, '')     IS NULL OR type     = @type::TEXT)
  AND (NULLIF(@name::TEXT, '')     IS NULL OR name     = @name::TEXT)
  AND (NULLIF(@model::TEXT, '')    IS NULL OR model    = @model::TEXT)
  AND (NULLIF(@level::TEXT, '')    IS NULL OR level    = @level::TEXT)
  AND (sqlc.narg('from_time')::TIMESTAMPTZ IS NULL OR start_time >= sqlc.narg('from_time')::TIMESTAMPTZ)
  AND (sqlc.narg('to_time')::TIMESTAMPTZ   IS NULL OR start_time <  sqlc.narg('to_time')::TIMESTAMPTZ)
ORDER BY start_time DESC
LIMIT $1 OFFSET $2;

-- name: CountObservationsFiltered :one
SELECT COUNT(*) FROM observations
WHERE project_id = @project_id::TEXT
  AND (NULLIF(@trace_id::TEXT, '') IS NULL OR trace_id = @trace_id::TEXT)
  AND (NULLIF(@type::TEXT, '')     IS NULL OR type     = @type::TEXT)
  AND (NULLIF(@name::TEXT, '')     IS NULL OR name     = @name::TEXT)
  AND (NULLIF(@model::TEXT, '')    IS NULL OR model    = @model::TEXT)
  AND (NULLIF(@level::TEXT, '')    IS NULL OR level    = @level::TEXT);

-- name: RecalculateTraceAggregates :exec
-- Rolls observation cost and token usage up onto the parent trace. Called by
-- the ingestion worker so the write path stays cheap.
UPDATE traces t SET
  total_cost = agg.total_cost,
  token_usage = jsonb_build_object(
    'input_tokens',  agg.input_tokens,
    'output_tokens', agg.output_tokens,
    'total_tokens',  agg.total_tokens
  ),
  updated_at = now()
FROM (
  SELECT
    COALESCE(SUM(o.cost), 0)::DOUBLE PRECISION AS total_cost,
    COALESCE(SUM((o.token_usage->>'input_tokens')::BIGINT), 0)  AS input_tokens,
    COALESCE(SUM((o.token_usage->>'output_tokens')::BIGINT), 0) AS output_tokens,
    COALESCE(SUM((o.token_usage->>'total_tokens')::BIGINT), 0)  AS total_tokens
  FROM observations o
  WHERE o.trace_id = @trace_id::TEXT
) agg
WHERE t.id = @trace_id::TEXT AND t.project_id = @project_id::TEXT;

-- ===========================================================================
-- Sessions
-- ===========================================================================

-- name: UpsertSession :exec
INSERT INTO sessions (id, project_id, environment)
VALUES (@id::TEXT, @project_id::TEXT, @environment::TEXT)
ON CONFLICT (project_id, id) DO NOTHING;

-- name: ListSessions :many
SELECT
  s.id,
  s.project_id,
  s.bookmarked,
  s.public,
  s.environment,
  s.created_at,
  COUNT(t.id)                                       AS trace_count,
  COALESCE(SUM(t.total_cost), 0)::DOUBLE PRECISION  AS total_cost,
  MIN(t.start_time)                                 AS first_trace_at,
  MAX(t.start_time)                                 AS last_trace_at
FROM sessions s
LEFT JOIN traces t ON t.session_id = s.id AND t.project_id = s.project_id
WHERE s.project_id = @project_id::TEXT
GROUP BY s.id, s.project_id, s.bookmarked, s.public, s.environment, s.created_at
ORDER BY s.created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountSessions :one
SELECT COUNT(*) FROM sessions WHERE project_id = $1;

-- name: GetSession :one
SELECT * FROM sessions WHERE id = $1 AND project_id = $2;

-- name: GetTracesBySessionID :many
SELECT * FROM traces
WHERE project_id = $1 AND session_id = $2
ORDER BY start_time ASC;

-- ===========================================================================
-- Model prices
-- ===========================================================================

-- name: ListModelPricesForProject :many
-- Project-scoped rows first, so they take precedence over the global defaults.
SELECT * FROM model_prices
WHERE project_id = @project_id::TEXT OR project_id IS NULL
ORDER BY (project_id IS NULL), length(match_pattern) DESC, model_name;

-- name: ListModelPrices :many
SELECT * FROM model_prices
WHERE (NULLIF(@project_id::TEXT, '') IS NULL OR project_id = @project_id::TEXT)
ORDER BY model_name;

-- name: GetModelPriceByID :one
SELECT * FROM model_prices WHERE id = $1;

-- name: CreateModelPrice :one
INSERT INTO model_prices (
  project_id, model_name, match_pattern, unit, input_price, output_price, total_price, currency
)
VALUES (
  NULLIF(@project_id::TEXT, ''), @model_name::TEXT, @match_pattern::TEXT, @unit::TEXT,
  sqlc.narg('input_price')::DOUBLE PRECISION,
  sqlc.narg('output_price')::DOUBLE PRECISION,
  sqlc.narg('total_price')::DOUBLE PRECISION,
  @currency::TEXT
)
RETURNING *;

-- name: DeleteModelPrice :exec
DELETE FROM model_prices
WHERE id = $1 AND (project_id = @project_id::TEXT OR (project_id IS NULL AND @project_id::TEXT = ''));

-- ===========================================================================
-- API keys (Langfuse-style public/secret pairs)
-- ===========================================================================

-- name: CreateAPIKeyPair :one
INSERT INTO api_keys (project_id, key, name, public_key, secret_key_hash, display_secret_key)
VALUES (
  @project_id::TEXT, @key::TEXT, NULLIF(@name::TEXT, ''),
  @public_key::TEXT, @secret_key_hash::TEXT, @display_secret_key::TEXT
)
RETURNING *;

-- name: GetAPIKeyByPublicKey :one
SELECT * FROM api_keys WHERE public_key = $1;

-- name: TouchAPIKey :exec
UPDATE api_keys SET last_used_at = now() WHERE id = $1;

-- ===========================================================================
-- Audit log
-- ===========================================================================

-- name: CreateAuditLog :exec
INSERT INTO audit_logs (org_id, project_id, user_id, api_key_id, action, resource, resource_id, ip_address, detail)
VALUES (
  NULLIF(@org_id::TEXT, ''), NULLIF(@project_id::TEXT, ''), NULLIF(@user_id::TEXT, ''),
  NULLIF(@api_key_id::TEXT, ''), @action::TEXT, @resource::TEXT,
  NULLIF(@resource_id::TEXT, ''), NULLIF(@ip_address::TEXT, ''), @detail
);

-- name: ListAuditLogs :many
SELECT * FROM audit_logs
WHERE project_id = @project_id::TEXT
  AND (NULLIF(@action::TEXT, '')   IS NULL OR action   = @action::TEXT)
  AND (NULLIF(@resource::TEXT, '') IS NULL OR resource = @resource::TEXT)
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- ===========================================================================
-- Prompts — label-based resolution
-- ===========================================================================

-- name: CreatePromptVersion :one
INSERT INTO prompts (project_id, name, version, prompt, config, is_active, type, labels, tags, commit_message, created_by)
VALUES (
  @project_id::TEXT, @name::TEXT, @version::INT, @prompt, @config,
  @is_active::BOOLEAN, @type::TEXT, @labels::TEXT[], @tags::TEXT[],
  NULLIF(@commit_message::TEXT, ''), NULLIF(@created_by::TEXT, '')
)
RETURNING *;

-- name: GetMaxPromptVersion :one
SELECT COALESCE(MAX(version), 0)::INT AS max_version
FROM prompts WHERE project_id = $1 AND name = $2;

-- name: GetPromptByLabel :one
SELECT * FROM prompts
WHERE project_id = $1 AND name = $2 AND labels @> ARRAY[@label::TEXT]
ORDER BY version DESC
LIMIT 1;

-- name: GetLatestPrompt :one
SELECT * FROM prompts
WHERE project_id = $1 AND name = $2
ORDER BY version DESC
LIMIT 1;

-- name: GetPromptVersions :many
SELECT * FROM prompts
WHERE project_id = $1 AND name = $2
ORDER BY version DESC;

-- name: ListPromptNames :many
-- One row per prompt name, with every label in use across its versions.
SELECT
  name,
  COUNT(*)::BIGINT              AS version_count,
  MAX(version)::INT             AS latest_version,
  MAX(created_at)::TIMESTAMPTZ  AS updated_at,
  COALESCE((
    SELECT ARRAY_AGG(DISTINCT l)
    FROM prompts p2, unnest(p2.labels) AS l
    WHERE p2.project_id = prompts.project_id AND p2.name = prompts.name
  ), ARRAY[]::TEXT[])::TEXT[]   AS labels
FROM prompts
WHERE prompts.project_id = $1
GROUP BY prompts.project_id, prompts.name
ORDER BY prompts.name;

-- name: RemoveLabelFromPrompts :exec
UPDATE prompts
SET labels = array_remove(labels, @label::TEXT)
WHERE project_id = $1 AND name = $2;

-- name: AddLabelToPromptVersion :exec
UPDATE prompts
SET labels = array_append(labels, @label::TEXT)
WHERE project_id = $1 AND name = $2 AND version = @version::INT
  AND NOT (labels @> ARRAY[@label::TEXT]);

-- name: SyncPromptActiveFromLabels :exec
-- Keeps the original is_active boolean consistent with the production label,
-- so clients written against the pre-label API keep working.
UPDATE prompts
SET is_active = ('production' = ANY(labels))
WHERE project_id = $1 AND name = $2;

-- name: DeletePromptVersion :exec
DELETE FROM prompts WHERE project_id = $1 AND name = $2 AND version = @version::INT;

-- name: DeletePromptByName :exec
DELETE FROM prompts WHERE project_id = $1 AND name = $2;

-- ===========================================================================
-- Datasets — project-scoped access
-- ===========================================================================

-- name: GetDatasetByIDAndProject :one
SELECT * FROM datasets WHERE id = $1 AND project_id = $2;

-- name: GetDatasetByNameAndProject :one
SELECT * FROM datasets WHERE name = $1 AND project_id = $2;

-- name: DeleteDatasetInProject :exec
DELETE FROM datasets WHERE id = $1 AND project_id = $2;

-- name: CreateDatasetItemFull :one
INSERT INTO dataset_items (id, dataset_id, input, expected_output, metadata, source_trace_id, source_observation_id, status)
VALUES (
  COALESCE(NULLIF(@id::TEXT, ''), gen_random_uuid()::TEXT),
  @dataset_id::TEXT, @input, @expected_output, @metadata,
  NULLIF(@source_trace_id::TEXT, ''), NULLIF(@source_observation_id::TEXT, ''),
  @status::TEXT
)
ON CONFLICT (id) DO UPDATE SET
  input           = EXCLUDED.input,
  expected_output = EXCLUDED.expected_output,
  metadata        = EXCLUDED.metadata,
  status          = EXCLUDED.status
-- Without this guard, supplying an id that already exists in a different
-- dataset would silently rewrite that other dataset's item.
WHERE dataset_items.dataset_id = EXCLUDED.dataset_id
RETURNING *;

-- name: UpdateDatasetItemStatus :exec
UPDATE dataset_items SET status = @status::TEXT WHERE id = $1;

-- name: DeleteDatasetItem :exec
DELETE FROM dataset_items WHERE id = $1 AND dataset_id = $2;

-- name: CreateDatasetRunFull :one
INSERT INTO dataset_runs (dataset_id, name, description, metadata)
VALUES (@dataset_id::TEXT, @name::TEXT, NULLIF(@description::TEXT, ''), @metadata)
RETURNING *;

-- name: GetDatasetRunByIDAndDataset :one
SELECT * FROM dataset_runs WHERE id = $1 AND dataset_id = $2;

-- name: GetDatasetRunByNameAndDataset :one
SELECT * FROM dataset_runs WHERE name = $1 AND dataset_id = $2;

-- name: DeleteDatasetRun :exec
DELETE FROM dataset_runs WHERE id = $1 AND dataset_id = $2;

-- name: CreateDatasetRunItemFull :one
INSERT INTO dataset_run_items (dataset_run_id, dataset_item_id, trace_id, observation_id, score_id)
VALUES (
  @dataset_run_id::TEXT, @dataset_item_id::TEXT,
  NULLIF(@trace_id::TEXT, ''), NULLIF(@observation_id::TEXT, ''), NULLIF(@score_id::TEXT, '')
)
RETURNING *;

-- name: GetDatasetRunItemsWithDetail :many
-- Run items joined to their dataset item and the trace that produced them, so
-- a run can be reported without an N+1 query per item.
SELECT
  ri.id             AS run_item_id,
  ri.dataset_item_id,
  ri.trace_id,
  ri.observation_id,
  ri.score_id,
  ri.created_at,
  di.input          AS item_input,
  di.expected_output,
  di.metadata       AS item_metadata,
  t.output          AS trace_output,
  t.total_cost,
  t.token_usage,
  EXTRACT(EPOCH FROM (t.end_time - t.start_time))::DOUBLE PRECISION AS latency_seconds
FROM dataset_run_items ri
JOIN dataset_items di ON di.id = ri.dataset_item_id
LEFT JOIN traces t    ON t.id = ri.trace_id
WHERE ri.dataset_run_id = $1
ORDER BY ri.created_at;

-- name: GetDatasetRunStats :one
-- Aggregate cost and latency for one run. Percentiles come from Postgres
-- directly so the whole run never has to be loaded into the process.
SELECT
  COUNT(*)::BIGINT                                  AS item_count,
  COUNT(t.id)::BIGINT                               AS traced_count,
  COALESCE(SUM(t.total_cost), 0)::DOUBLE PRECISION  AS total_cost,
  COALESCE(AVG(t.total_cost), 0)::DOUBLE PRECISION  AS avg_cost,
  COALESCE(SUM((t.token_usage->>'total_tokens')::BIGINT), 0)::BIGINT AS total_tokens,
  COALESCE(AVG(EXTRACT(EPOCH FROM (t.end_time - t.start_time))), 0)::DOUBLE PRECISION AS avg_latency_seconds,
  COALESCE(PERCENTILE_CONT(0.5) WITHIN GROUP (
    ORDER BY EXTRACT(EPOCH FROM (t.end_time - t.start_time))), 0)::DOUBLE PRECISION   AS p50_latency_seconds,
  COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP (
    ORDER BY EXTRACT(EPOCH FROM (t.end_time - t.start_time))), 0)::DOUBLE PRECISION   AS p95_latency_seconds
FROM dataset_run_items ri
LEFT JOIN traces t ON t.id = ri.trace_id
WHERE ri.dataset_run_id = $1;

-- name: GetDatasetRunScoreStats :many
-- Per-score-name aggregates for one run. A score counts when it is attached to
-- the run item directly, to the run, or to the trace the run item produced.
SELECT
  s.name,
  COUNT(*)::BIGINT                     AS count,
  AVG(s.value)::DOUBLE PRECISION       AS avg_value,
  MIN(s.value)::DOUBLE PRECISION       AS min_value,
  MAX(s.value)::DOUBLE PRECISION       AS max_value,
  STDDEV_POP(s.value)::DOUBLE PRECISION AS stddev_value
FROM dataset_run_items ri
JOIN scores s
  ON s.id = ri.score_id
  OR s.dataset_run_id = ri.dataset_run_id
  OR (ri.trace_id IS NOT NULL AND s.trace_id = ri.trace_id)
WHERE ri.dataset_run_id = $1 AND s.value IS NOT NULL
GROUP BY s.name
ORDER BY s.name;

-- ===========================================================================
-- Analytics — percentiles and filtered aggregates
-- ===========================================================================

-- name: GetTraceLatencyPercentiles :one
SELECT
  COUNT(*)::BIGINT AS total_traces,
  COALESCE(AVG(EXTRACT(EPOCH FROM (end_time - start_time))), 0)::DOUBLE PRECISION AS avg_latency_seconds,
  COALESCE(MIN(EXTRACT(EPOCH FROM (end_time - start_time))), 0)::DOUBLE PRECISION AS min_latency_seconds,
  COALESCE(MAX(EXTRACT(EPOCH FROM (end_time - start_time))), 0)::DOUBLE PRECISION AS max_latency_seconds,
  COALESCE(PERCENTILE_CONT(0.50) WITHIN GROUP (
    ORDER BY EXTRACT(EPOCH FROM (end_time - start_time))), 0)::DOUBLE PRECISION AS p50_latency_seconds,
  COALESCE(PERCENTILE_CONT(0.90) WITHIN GROUP (
    ORDER BY EXTRACT(EPOCH FROM (end_time - start_time))), 0)::DOUBLE PRECISION AS p90_latency_seconds,
  COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP (
    ORDER BY EXTRACT(EPOCH FROM (end_time - start_time))), 0)::DOUBLE PRECISION AS p95_latency_seconds,
  COALESCE(PERCENTILE_CONT(0.99) WITHIN GROUP (
    ORDER BY EXTRACT(EPOCH FROM (end_time - start_time))), 0)::DOUBLE PRECISION AS p99_latency_seconds
FROM traces
WHERE project_id = @project_id::TEXT
  AND end_time IS NOT NULL
  AND (NULLIF(@user_id::TEXT, '')     IS NULL OR user_id     = @user_id::TEXT)
  AND (NULLIF(@session_id::TEXT, '')  IS NULL OR session_id  = @session_id::TEXT)
  AND (NULLIF(@environment::TEXT, '') IS NULL OR environment = @environment::TEXT)
  AND (cardinality(@tags::TEXT[]) = 0 OR tags @> @tags::TEXT[])
  AND (sqlc.narg('from_time')::TIMESTAMPTZ IS NULL OR start_time >= sqlc.narg('from_time')::TIMESTAMPTZ)
  AND (sqlc.narg('to_time')::TIMESTAMPTZ   IS NULL OR start_time <  sqlc.narg('to_time')::TIMESTAMPTZ);

-- name: GetModelUsageStats :many
-- Cost and token usage broken down by model, for the cost-analysis view.
SELECT
  COALESCE(o.model, 'unknown')                      AS model,
  COUNT(*)::BIGINT                                  AS generation_count,
  COALESCE(SUM(o.cost), 0)::DOUBLE PRECISION        AS total_cost,
  COALESCE(AVG(o.cost), 0)::DOUBLE PRECISION        AS avg_cost,
  COALESCE(SUM((o.token_usage->>'input_tokens')::BIGINT), 0)::BIGINT  AS input_tokens,
  COALESCE(SUM((o.token_usage->>'output_tokens')::BIGINT), 0)::BIGINT AS output_tokens,
  COALESCE(SUM((o.token_usage->>'total_tokens')::BIGINT), 0)::BIGINT  AS total_tokens,
  COALESCE(AVG(EXTRACT(EPOCH FROM (o.end_time - o.start_time))), 0)::DOUBLE PRECISION AS avg_latency_seconds
FROM observations o
WHERE o.project_id = @project_id::TEXT
  AND o.type = 'GENERATION'
  AND (sqlc.narg('from_time')::TIMESTAMPTZ IS NULL OR o.start_time >= sqlc.narg('from_time')::TIMESTAMPTZ)
  AND (sqlc.narg('to_time')::TIMESTAMPTZ   IS NULL OR o.start_time <  sqlc.narg('to_time')::TIMESTAMPTZ)
GROUP BY COALESCE(o.model, 'unknown')
ORDER BY total_cost DESC;

-- name: GetCostByUser :many
SELECT
  COALESCE(user_id, 'anonymous')                   AS user_id,
  COUNT(*)::BIGINT                                 AS trace_count,
  COALESCE(SUM(total_cost), 0)::DOUBLE PRECISION   AS total_cost,
  COALESCE(SUM((token_usage->>'total_tokens')::BIGINT), 0)::BIGINT AS total_tokens
FROM traces
WHERE project_id = @project_id::TEXT
  AND (sqlc.narg('from_time')::TIMESTAMPTZ IS NULL OR start_time >= sqlc.narg('from_time')::TIMESTAMPTZ)
  AND (sqlc.narg('to_time')::TIMESTAMPTZ   IS NULL OR start_time <  sqlc.narg('to_time')::TIMESTAMPTZ)
GROUP BY COALESCE(user_id, 'anonymous')
ORDER BY total_cost DESC
LIMIT $1;

-- name: GetTraceMetricsOverTime :many
-- One row per time bucket. The bucket width is a parameter so an hour-scale
-- range does not collapse into a single daily point.
SELECT
  date_bin(@bucket::INTERVAL, start_time, @origin::TIMESTAMPTZ)::TIMESTAMPTZ AS time_bucket,
  COUNT(*)::BIGINT                                 AS trace_count,
  COUNT(*) FILTER (WHERE EXISTS (
    SELECT 1 FROM observations o WHERE o.trace_id = traces.id AND o.level = 'ERROR'
  ))::BIGINT                                       AS error_count,
  COALESCE(SUM(total_cost), 0)::DOUBLE PRECISION   AS total_cost,
  COALESCE(SUM((token_usage->>'input_tokens')::BIGINT), 0)::BIGINT  AS input_tokens,
  COALESCE(SUM((token_usage->>'output_tokens')::BIGINT), 0)::BIGINT AS output_tokens,
  COALESCE(SUM((token_usage->>'total_tokens')::BIGINT), 0)::BIGINT  AS total_tokens,
  COALESCE(AVG(EXTRACT(EPOCH FROM (end_time - start_time))), 0)::DOUBLE PRECISION AS avg_latency_seconds,
  COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP (
    ORDER BY EXTRACT(EPOCH FROM (end_time - start_time))), 0)::DOUBLE PRECISION   AS p95_latency_seconds
FROM traces
WHERE project_id = @project_id::TEXT
  AND start_time >= @from_time::TIMESTAMPTZ
  AND start_time <  @to_time::TIMESTAMPTZ
  AND (NULLIF(@user_id::TEXT, '')     IS NULL OR user_id     = @user_id::TEXT)
  AND (NULLIF(@session_id::TEXT, '')  IS NULL OR session_id  = @session_id::TEXT)
  AND (NULLIF(@environment::TEXT, '') IS NULL OR environment = @environment::TEXT)
  AND (cardinality(@tags::TEXT[]) = 0 OR tags @> @tags::TEXT[])
GROUP BY time_bucket
ORDER BY time_bucket;

-- name: UpdateObservationCost :exec
UPDATE observations
SET cost = sqlc.narg('cost')::DOUBLE PRECISION,
    cost_details = @cost_details
WHERE id = $1 AND project_id = $2;
