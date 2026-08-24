-- 002_add_indexes.down.sql
-- Remove performance indexes

DROP INDEX IF EXISTS idx_prompts_project_name;
DROP INDEX IF EXISTS idx_scores_trace_name;
DROP INDEX IF EXISTS idx_dataset_items_dataset;
DROP INDEX IF EXISTS idx_dataset_runs_dataset;
DROP INDEX IF EXISTS idx_dataset_run_items_run;
DROP INDEX IF EXISTS idx_observations_trace_status;
DROP INDEX IF EXISTS idx_traces_project_cost;
