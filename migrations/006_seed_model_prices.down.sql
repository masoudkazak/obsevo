-- 006_seed_model_prices.down.sql
-- Remove the seeded global defaults, leaving any project-scoped prices intact.

DELETE FROM model_prices WHERE project_id IS NULL;
