-- 007_security_and_evaluators.up.sql
-- Hashed API keys, Langfuse-style key pairs, an audit log, and support for
-- named evaluators.

-- ---------------------------------------------------------------------------
-- api_keys
--
-- Langfuse SDKs authenticate with HTTP Basic using a public key as the username
-- and a secret key as the password. The secret is stored as a SHA-256 digest
-- rather than in plaintext. SHA-256 (not bcrypt) is deliberate: these are
-- high-entropy generated tokens, not user-chosen passwords, so a slow KDF buys
-- nothing while costing latency on every ingestion request.
--
-- The original `key` column stays for the existing x-api-key header.
-- ---------------------------------------------------------------------------
ALTER TABLE api_keys
  ADD COLUMN public_key         TEXT,
  ADD COLUMN secret_key_hash    TEXT,
  ADD COLUMN display_secret_key TEXT,
  ADD COLUMN last_used_at       TIMESTAMPTZ,
  ADD COLUMN expires_at         TIMESTAMPTZ;

CREATE UNIQUE INDEX idx_api_keys_public_key ON api_keys(public_key) WHERE public_key IS NOT NULL;

-- ---------------------------------------------------------------------------
-- evaluator_configs
--
-- `type` used to be restricted to CODE and LLM_JUDGE. It now holds the name of
-- a registered evaluator; the two original values remain valid so existing
-- configurations are untouched.
-- ---------------------------------------------------------------------------
ALTER TABLE evaluator_configs DROP CONSTRAINT evaluator_configs_type_check;
ALTER TABLE evaluator_configs ADD CONSTRAINT evaluator_configs_type_check
  CHECK (type ~ '^[A-Za-z][A-Za-z0-9_]*$');

-- ---------------------------------------------------------------------------
-- audit_logs
--
-- Records security-relevant mutations. Deliberately narrow: who, what, when,
-- and the target, with no request bodies, so the table stays small.
-- ---------------------------------------------------------------------------
CREATE TABLE audit_logs (
  id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
  org_id      TEXT,
  project_id  TEXT,
  user_id     TEXT,
  api_key_id  TEXT,
  action      TEXT NOT NULL,
  resource    TEXT NOT NULL,
  resource_id TEXT,
  ip_address  TEXT,
  detail      JSONB,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_audit_logs_project_time ON audit_logs(project_id, created_at DESC);
CREATE INDEX idx_audit_logs_user_time    ON audit_logs(user_id, created_at DESC);
