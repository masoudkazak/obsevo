-- 005_langfuse_compat.down.sql

DROP INDEX IF EXISTS idx_model_prices_lookup;
DROP INDEX IF EXISTS idx_prompts_labels;
DROP INDEX IF EXISTS idx_traces_tags;
DROP INDEX IF EXISTS idx_traces_project_user;
DROP INDEX IF EXISTS idx_traces_project_session;
DROP INDEX IF EXISTS idx_scores_observation;
DROP INDEX IF EXISTS idx_scores_project_name;
DROP INDEX IF EXISTS idx_observations_project_model;
DROP INDEX IF EXISTS idx_observations_parent;
DROP INDEX IF EXISTS idx_observations_project_start;

ALTER TABLE dataset_run_items DROP COLUMN IF EXISTS trace_id;

ALTER TABLE dataset_runs
  DROP COLUMN IF EXISTS metadata,
  DROP COLUMN IF EXISTS description;

ALTER TABLE dataset_items
  DROP COLUMN IF EXISTS source_observation_id,
  DROP COLUMN IF EXISTS status;

DROP TABLE IF EXISTS model_prices;
DROP TABLE IF EXISTS sessions;

ALTER TABLE prompts
  DROP COLUMN IF EXISTS created_by,
  DROP COLUMN IF EXISTS commit_message,
  DROP COLUMN IF EXISTS tags,
  DROP COLUMN IF EXISTS labels,
  DROP COLUMN IF EXISTS type;

-- Rows added after 005 may have a NULL trace_id, which the restored NOT NULL
-- constraint cannot represent.
DELETE FROM scores WHERE trace_id IS NULL;
ALTER TABLE scores ALTER COLUMN trace_id SET NOT NULL;

ALTER TABLE scores DROP CONSTRAINT scores_source_check;
DELETE FROM scores WHERE source NOT IN ('USER', 'EVALUATOR', 'SDK');
ALTER TABLE scores ADD CONSTRAINT scores_source_check
  CHECK (source IN ('USER', 'EVALUATOR', 'SDK'));

ALTER TABLE scores
  DROP CONSTRAINT IF EXISTS scores_project_id_fkey;
ALTER TABLE scores
  DROP COLUMN IF EXISTS environment,
  DROP COLUMN IF EXISTS metadata,
  DROP COLUMN IF EXISTS config_id,
  DROP COLUMN IF EXISTS string_value,
  DROP COLUMN IF EXISTS data_type,
  DROP COLUMN IF EXISTS dataset_run_id,
  DROP COLUMN IF EXISTS session_id,
  DROP COLUMN IF EXISTS observation_id,
  DROP COLUMN IF EXISTS project_id;

ALTER TABLE observations
  DROP CONSTRAINT IF EXISTS observations_project_id_fkey;
ALTER TABLE observations
  DROP COLUMN IF EXISTS created_at,
  DROP COLUMN IF EXISTS environment,
  DROP COLUMN IF EXISTS version,
  DROP COLUMN IF EXISTS cost_details,
  DROP COLUMN IF EXISTS usage_details,
  DROP COLUMN IF EXISTS prompt_version,
  DROP COLUMN IF EXISTS prompt_name,
  DROP COLUMN IF EXISTS prompt_id,
  DROP COLUMN IF EXISTS completion_start_time,
  DROP COLUMN IF EXISTS status_message,
  DROP COLUMN IF EXISTS level,
  DROP COLUMN IF EXISTS project_id;

ALTER TABLE traces
  DROP COLUMN IF EXISTS updated_at,
  DROP COLUMN IF EXISTS environment,
  DROP COLUMN IF EXISTS bookmarked,
  DROP COLUMN IF EXISTS public,
  DROP COLUMN IF EXISTS version,
  DROP COLUMN IF EXISTS release;
