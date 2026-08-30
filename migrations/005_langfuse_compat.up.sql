-- 005_langfuse_compat.up.sql
-- Align the data model with Langfuse semantics.
-- Every column added here is nullable or carries a default, so existing rows and
-- existing API clients keep working unchanged.

-- ---------------------------------------------------------------------------
-- traces
-- ---------------------------------------------------------------------------
ALTER TABLE traces
  ADD COLUMN release     TEXT,
  ADD COLUMN version     TEXT,
  ADD COLUMN public      BOOLEAN     NOT NULL DEFAULT false,
  ADD COLUMN bookmarked  BOOLEAN     NOT NULL DEFAULT false,
  ADD COLUMN environment TEXT        NOT NULL DEFAULT 'default',
  ADD COLUMN updated_at  TIMESTAMPTZ NOT NULL DEFAULT now();

-- ---------------------------------------------------------------------------
-- observations
-- project_id is denormalised from the parent trace so that authorization and
-- analytics can filter observations without joining traces on every query.
-- ---------------------------------------------------------------------------
ALTER TABLE observations
  ADD COLUMN project_id            TEXT,
  ADD COLUMN level                 TEXT NOT NULL DEFAULT 'DEFAULT'
    CHECK (level IN ('DEBUG', 'DEFAULT', 'WARNING', 'ERROR')),
  ADD COLUMN status_message        TEXT,
  ADD COLUMN completion_start_time TIMESTAMPTZ,
  ADD COLUMN prompt_id             TEXT,
  ADD COLUMN prompt_name           TEXT,
  ADD COLUMN prompt_version        INT,
  ADD COLUMN usage_details         JSONB,
  ADD COLUMN cost_details          JSONB,
  ADD COLUMN version               TEXT,
  ADD COLUMN environment           TEXT NOT NULL DEFAULT 'default',
  ADD COLUMN created_at            TIMESTAMPTZ NOT NULL DEFAULT now();

UPDATE observations o
SET project_id = t.project_id
FROM traces t
WHERE o.trace_id = t.id AND o.project_id IS NULL;

-- Observations whose trace vanished cannot be attributed; none should exist
-- because trace_id is NOT NULL with ON DELETE CASCADE.
DELETE FROM observations WHERE project_id IS NULL;

ALTER TABLE observations
  ALTER COLUMN project_id SET NOT NULL,
  ADD CONSTRAINT observations_project_id_fkey
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;

-- Existing rows recorded failures only in `status`; mirror that into `level` so
-- the two stay consistent for data written before this migration.
UPDATE observations SET level = 'ERROR' WHERE status = 'ERROR';

-- ---------------------------------------------------------------------------
-- scores
-- ---------------------------------------------------------------------------
ALTER TABLE scores
  ADD COLUMN project_id     TEXT,
  ADD COLUMN observation_id TEXT,
  ADD COLUMN session_id     TEXT,
  ADD COLUMN dataset_run_id TEXT,
  ADD COLUMN data_type      TEXT NOT NULL DEFAULT 'NUMERIC'
    CHECK (data_type IN ('NUMERIC', 'CATEGORICAL', 'BOOLEAN')),
  ADD COLUMN string_value   TEXT,
  ADD COLUMN config_id      TEXT,
  ADD COLUMN metadata       JSONB,
  ADD COLUMN environment    TEXT NOT NULL DEFAULT 'default';

UPDATE scores s
SET project_id = t.project_id
FROM traces t
WHERE s.trace_id = t.id AND s.project_id IS NULL;

DELETE FROM scores WHERE project_id IS NULL;

ALTER TABLE scores
  ALTER COLUMN project_id SET NOT NULL,
  ADD CONSTRAINT scores_project_id_fkey
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE;

-- Langfuse source values are API / EVAL / ANNOTATION. The original
-- USER / EVALUATOR / SDK values stay accepted so existing clients keep working.
ALTER TABLE scores DROP CONSTRAINT scores_source_check;
ALTER TABLE scores ADD CONSTRAINT scores_source_check
  CHECK (source IN ('USER', 'EVALUATOR', 'SDK', 'API', 'EVAL', 'ANNOTATION'));

-- trace_id becomes optional: Langfuse allows scores attached only to a session
-- or a dataset run.
ALTER TABLE scores ALTER COLUMN trace_id DROP NOT NULL;

-- ---------------------------------------------------------------------------
-- prompts
-- Langfuse resolves a prompt by label, and one version may carry several
-- labels. `is_active` is retained and kept in sync so older clients still work.
-- ---------------------------------------------------------------------------
ALTER TABLE prompts
  ADD COLUMN type           TEXT   NOT NULL DEFAULT 'text'
    CHECK (type IN ('text', 'chat')),
  ADD COLUMN labels         TEXT[] NOT NULL DEFAULT '{}',
  ADD COLUMN tags           TEXT[] NOT NULL DEFAULT '{}',
  ADD COLUMN commit_message TEXT,
  ADD COLUMN created_by     TEXT;

UPDATE prompts SET labels = ARRAY['production', 'latest'] WHERE is_active = true;

-- ---------------------------------------------------------------------------
-- sessions
-- Langfuse groups traces into sessions. Rows are upserted during ingestion.
-- ---------------------------------------------------------------------------
CREATE TABLE sessions (
  id          TEXT        NOT NULL,
  project_id  TEXT        NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  bookmarked  BOOLEAN     NOT NULL DEFAULT false,
  public      BOOLEAN     NOT NULL DEFAULT false,
  environment TEXT        NOT NULL DEFAULT 'default',
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (project_id, id)
);

INSERT INTO sessions (id, project_id, created_at)
SELECT session_id, project_id, MIN(start_time)
FROM traces
WHERE session_id IS NOT NULL AND session_id <> ''
GROUP BY project_id, session_id
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- model_prices
-- Cost is computed server-side from token usage. A NULL project_id row is a
-- built-in default; a project-scoped row overrides it for that project only.
-- ---------------------------------------------------------------------------
CREATE TABLE model_prices (
  id            TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
  project_id    TEXT REFERENCES projects(id) ON DELETE CASCADE,
  model_name    TEXT NOT NULL,
  match_pattern TEXT NOT NULL,
  unit          TEXT NOT NULL DEFAULT 'TOKENS',
  input_price   DOUBLE PRECISION,
  output_price  DOUBLE PRECISION,
  total_price   DOUBLE PRECISION,
  currency      TEXT NOT NULL DEFAULT 'USD',
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- datasets
-- ---------------------------------------------------------------------------
ALTER TABLE dataset_items
  ADD COLUMN status                TEXT NOT NULL DEFAULT 'ACTIVE'
    CHECK (status IN ('ACTIVE', 'ARCHIVED')),
  ADD COLUMN source_observation_id TEXT;

ALTER TABLE dataset_runs
  ADD COLUMN description TEXT,
  ADD COLUMN metadata    JSONB;

ALTER TABLE dataset_run_items
  ADD COLUMN trace_id TEXT;

-- ---------------------------------------------------------------------------
-- indexes for the paths added above
-- ---------------------------------------------------------------------------
CREATE INDEX idx_observations_project_start ON observations(project_id, start_time DESC);
CREATE INDEX idx_observations_parent        ON observations(parent_observation_id);
CREATE INDEX idx_observations_project_model ON observations(project_id, model);
CREATE INDEX idx_scores_project_name        ON scores(project_id, name);
CREATE INDEX idx_scores_observation         ON scores(observation_id);
CREATE INDEX idx_traces_project_session     ON traces(project_id, session_id);
CREATE INDEX idx_traces_project_user        ON traces(project_id, user_id);
CREATE INDEX idx_traces_tags                ON traces USING GIN (tags);
CREATE INDEX idx_prompts_labels             ON prompts USING GIN (labels);
CREATE INDEX idx_model_prices_lookup        ON model_prices(project_id, model_name);
