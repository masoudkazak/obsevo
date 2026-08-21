-- 001_init.down.sql
-- Rollback initial database schema

DROP TABLE IF EXISTS dataset_run_items;
DROP TABLE IF EXISTS dataset_runs;
DROP TABLE IF EXISTS dataset_items;
DROP TABLE IF EXISTS datasets;
DROP TABLE IF EXISTS scores;
DROP TABLE IF EXISTS prompts;
DROP TABLE IF EXISTS observations;
DROP TABLE IF EXISTS traces;
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS members;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS organizations;

DROP EXTENSION IF EXISTS "pgcrypto";
