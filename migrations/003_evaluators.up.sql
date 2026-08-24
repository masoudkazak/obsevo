-- 003_evaluators.up.sql

CREATE TABLE evaluator_configs (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
  project_id TEXT NOT NULL REFERENCES projects(id),
  name TEXT NOT NULL,
  description TEXT,
  type TEXT NOT NULL CHECK (type IN ('CODE', 'LLM_JUDGE')),
  config JSONB NOT NULL DEFAULT '{}',
  is_active BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(project_id, name)
);

CREATE TABLE evaluation_runs (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
  project_id TEXT NOT NULL REFERENCES projects(id),
  evaluator_config_id TEXT NOT NULL REFERENCES evaluator_configs(id),
  name TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'RUNNING', 'COMPLETED', 'FAILED')),
  result_summary JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_evaluator_configs_project ON evaluator_configs(project_id);
CREATE INDEX idx_evaluation_runs_project ON evaluation_runs(project_id);
CREATE INDEX idx_evaluation_runs_evaluator ON evaluation_runs(evaluator_config_id);
