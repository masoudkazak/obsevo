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
INSERT INTO traces (project_id, name, input, output, metadata, user_id, session_id, tags, start_time, end_time, total_cost, token_usage)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
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

-- name: GetScoreByID :one
SELECT * FROM scores WHERE id = $1;

-- name: GetScoresByTraceID :many
SELECT * FROM scores WHERE trace_id = $1 ORDER BY created_at DESC;

-- name: CreateScore :one
INSERT INTO scores (trace_id, name, value, comment, source, user_id)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

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

-- name: GetDatasetRunItemByID :one
SELECT * FROM dataset_run_items WHERE id = $1;

-- name: GetDatasetRunItemsByRunID :many
SELECT * FROM dataset_run_items WHERE dataset_run_id = $1 ORDER BY created_at;

-- name: CreateDatasetRunItem :one
INSERT INTO dataset_run_items (dataset_run_id, dataset_item_id, observation_id, score_id)
VALUES ($1, $2, $3, $4)
RETURNING *;
