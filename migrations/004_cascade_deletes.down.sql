-- 004_cascade_deletes.down.sql
-- Revert ON DELETE CASCADE changes

ALTER TABLE dataset_run_items
  DROP CONSTRAINT dataset_run_items_dataset_item_id_fkey,
  ADD CONSTRAINT dataset_run_items_dataset_item_id_fkey
    FOREIGN KEY (dataset_item_id) REFERENCES dataset_items(id);

ALTER TABLE evaluation_runs
  DROP CONSTRAINT evaluation_runs_evaluator_config_id_fkey,
  ADD CONSTRAINT evaluation_runs_evaluator_config_id_fkey
    FOREIGN KEY (evaluator_config_id) REFERENCES evaluator_configs(id);
