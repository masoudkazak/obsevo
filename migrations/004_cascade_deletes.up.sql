-- 004_cascade_deletes.up.sql
-- Add ON DELETE CASCADE to FK constraints that block dataset/evaluator deletion

ALTER TABLE dataset_run_items
  DROP CONSTRAINT dataset_run_items_dataset_item_id_fkey,
  ADD CONSTRAINT dataset_run_items_dataset_item_id_fkey
    FOREIGN KEY (dataset_item_id) REFERENCES dataset_items(id) ON DELETE CASCADE;

ALTER TABLE evaluation_runs
  DROP CONSTRAINT evaluation_runs_evaluator_config_id_fkey,
  ADD CONSTRAINT evaluation_runs_evaluator_config_id_fkey
    FOREIGN KEY (evaluator_config_id) REFERENCES evaluator_configs(id) ON DELETE CASCADE;
