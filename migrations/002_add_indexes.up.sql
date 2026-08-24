-- 002_add_indexes.up.sql
-- Performance indexes for hot query paths

-- Prompts: queries filter by project_id + name, and order by version DESC
CREATE INDEX idx_prompts_project_name ON prompts(project_id, name, version DESC);

-- Scores: queries filter by trace_id + name
CREATE INDEX idx_scores_trace_name ON scores(trace_id, name);

-- Dataset items: list by dataset_id
CREATE INDEX idx_dataset_items_dataset ON dataset_items(dataset_id);

-- Dataset runs: list by dataset_id
CREATE INDEX idx_dataset_runs_dataset ON dataset_runs(dataset_id);

-- Dataset run items: list by run_id
CREATE INDEX idx_dataset_run_items_run ON dataset_run_items(dataset_run_id);

-- Observations: filter by trace_id + type (for error rate queries)
CREATE INDEX idx_observations_trace_status ON observations(trace_id, status);

-- Traces: cost and token analytics filter on total_cost IS NOT NULL
CREATE INDEX idx_traces_project_cost ON traces(project_id) WHERE total_cost IS NOT NULL;
