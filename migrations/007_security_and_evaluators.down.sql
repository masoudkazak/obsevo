-- 007_security_and_evaluators.down.sql

DROP TABLE IF EXISTS audit_logs;

ALTER TABLE evaluator_configs DROP CONSTRAINT evaluator_configs_type_check;
DELETE FROM evaluator_configs WHERE type NOT IN ('CODE', 'LLM_JUDGE');
ALTER TABLE evaluator_configs ADD CONSTRAINT evaluator_configs_type_check
  CHECK (type IN ('CODE', 'LLM_JUDGE'));

DROP INDEX IF EXISTS idx_api_keys_public_key;
ALTER TABLE api_keys
  DROP COLUMN IF EXISTS expires_at,
  DROP COLUMN IF EXISTS last_used_at,
  DROP COLUMN IF EXISTS display_secret_key,
  DROP COLUMN IF EXISTS secret_key_hash,
  DROP COLUMN IF EXISTS public_key;
