-- 001_init.up.sql
-- Initial database schema for Langfuse Light

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Organizations table
CREATE TABLE organizations (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
  name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Users table
CREATE TABLE users (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
  email TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  name TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Members table (organization membership with roles)
CREATE TABLE members (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
  user_id TEXT NOT NULL REFERENCES users(id),
  org_id TEXT NOT NULL REFERENCES organizations(id),
  role TEXT NOT NULL DEFAULT 'VIEWER' CHECK (role IN ('VIEWER', 'EDITOR', 'ADMIN')),
  UNIQUE(user_id, org_id)
);

-- Projects table
CREATE TABLE projects (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
  name TEXT NOT NULL,
  org_id TEXT NOT NULL REFERENCES organizations(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- API Keys table
CREATE TABLE api_keys (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
  project_id TEXT NOT NULL REFERENCES projects(id),
  key TEXT NOT NULL UNIQUE,
  name TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Traces table
CREATE TABLE traces (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
  project_id TEXT NOT NULL REFERENCES projects(id),
  name TEXT,
  input JSONB,
  output JSONB,
  metadata JSONB,
  user_id TEXT,
  session_id TEXT,
  tags TEXT[] DEFAULT '{}',
  start_time TIMESTAMPTZ NOT NULL,
  end_time TIMESTAMPTZ,
  total_cost DOUBLE PRECISION,
  token_usage JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Indexes for traces
CREATE INDEX idx_traces_project_start ON traces(project_id, start_time DESC);
CREATE INDEX idx_traces_project_name ON traces(project_id, name);

-- Observations table (spans, generations, events)
CREATE TABLE observations (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
  trace_id TEXT NOT NULL REFERENCES traces(id) ON DELETE CASCADE,
  type TEXT NOT NULL CHECK (type IN ('SPAN', 'GENERATION', 'EVENT')),
  name TEXT,
  input JSONB,
  output JSONB,
  metadata JSONB,
  model TEXT,
  model_parameters JSONB,
  start_time TIMESTAMPTZ NOT NULL,
  end_time TIMESTAMPTZ,
  token_usage JSONB,
  cost DOUBLE PRECISION,
  status TEXT NOT NULL DEFAULT 'DEFAULT' CHECK (status IN ('DEFAULT', 'OK', 'ERROR')),
  parent_observation_id TEXT REFERENCES observations(id)
);

-- Index for observations
CREATE INDEX idx_observations_trace ON observations(trace_id);

-- Prompts table (versioned)
CREATE TABLE prompts (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
  project_id TEXT NOT NULL REFERENCES projects(id),
  name TEXT NOT NULL,
  version INT NOT NULL DEFAULT 1,
  prompt JSONB NOT NULL,
  config JSONB,
  is_active BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(project_id, name, version)
);

-- Scores table
CREATE TABLE scores (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
  trace_id TEXT NOT NULL REFERENCES traces(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  value DOUBLE PRECISION,
  comment TEXT,
  source TEXT NOT NULL CHECK (source IN ('USER', 'EVALUATOR', 'SDK')),
  user_id TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Index for scores
CREATE INDEX idx_scores_trace ON scores(trace_id);

-- Datasets table
CREATE TABLE datasets (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
  project_id TEXT NOT NULL REFERENCES projects(id),
  name TEXT NOT NULL,
  description TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(project_id, name)
);

-- Dataset items table
CREATE TABLE dataset_items (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
  dataset_id TEXT NOT NULL REFERENCES datasets(id) ON DELETE CASCADE,
  input JSONB NOT NULL,
  expected_output JSONB,
  metadata JSONB,
  source_trace_id TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Dataset runs table
CREATE TABLE dataset_runs (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
  dataset_id TEXT NOT NULL REFERENCES datasets(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Dataset run items table
CREATE TABLE dataset_run_items (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
  dataset_run_id TEXT NOT NULL REFERENCES dataset_runs(id) ON DELETE CASCADE,
  dataset_item_id TEXT NOT NULL REFERENCES dataset_items(id),
  observation_id TEXT,
  score_id TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
