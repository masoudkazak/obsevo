-- 006_seed_model_prices.up.sql
-- Seed default model prices so cost tracking works out of the box.
--
-- These are list prices in USD *per token* (published per-million rates divided
-- by 1,000,000) captured when this migration was written. Provider pricing
-- changes, so treat them as a starting point: a project can override any model
-- through POST /api/model-prices, and a project-scoped row always wins over the
-- global default seeded here. Rows with project_id IS NULL are the defaults.
--
-- match_pattern is a Go regular expression evaluated against the observation's
-- model name. Patterns are anchored and allow a trailing date/version suffix so
-- that dated model snapshots price the same as their base model.

INSERT INTO model_prices (project_id, model_name, match_pattern, unit, input_price, output_price, currency) VALUES
  -- OpenAI
  (NULL, 'gpt-4o',            '(?i)^gpt-4o(-\d{4}-\d{2}-\d{2})?$',            'TOKENS', 0.0000025,   0.00001,    'USD'),
  (NULL, 'gpt-4o-mini',       '(?i)^gpt-4o-mini(-\d{4}-\d{2}-\d{2})?$',       'TOKENS', 0.00000015,  0.0000006,  'USD'),
  (NULL, 'gpt-4-turbo',       '(?i)^gpt-4-turbo.*$',                          'TOKENS', 0.00001,     0.00003,    'USD'),
  (NULL, 'gpt-4',             '(?i)^gpt-4(-\d{4})?$',                         'TOKENS', 0.00003,     0.00006,    'USD'),
  (NULL, 'gpt-3.5-turbo',     '(?i)^gpt-3\.5-turbo.*$',                       'TOKENS', 0.0000005,   0.0000015,  'USD'),
  (NULL, 'o1',                '(?i)^o1(-\d{4}-\d{2}-\d{2})?$',                'TOKENS', 0.000015,    0.00006,    'USD'),
  (NULL, 'o1-mini',           '(?i)^o1-mini(-\d{4}-\d{2}-\d{2})?$',           'TOKENS', 0.0000011,   0.0000044,  'USD'),

  -- Anthropic
  (NULL, 'claude-3-opus',     '(?i)^claude-3-opus.*$',                        'TOKENS', 0.000015,    0.000075,   'USD'),
  (NULL, 'claude-3-5-sonnet', '(?i)^claude-3[-.]5-sonnet.*$',                 'TOKENS', 0.000003,    0.000015,   'USD'),
  (NULL, 'claude-3-haiku',    '(?i)^claude-3-haiku.*$',                       'TOKENS', 0.00000025,  0.00000125, 'USD'),
  (NULL, 'claude-sonnet-4',   '(?i)^claude-sonnet-4.*$',                      'TOKENS', 0.000003,    0.000015,   'USD'),
  (NULL, 'claude-opus-4',     '(?i)^claude-opus-4.*$',                        'TOKENS', 0.000015,    0.000075,   'USD'),
  (NULL, 'claude-haiku-4-5',  '(?i)^claude-haiku-4[-.]5.*$',                  'TOKENS', 0.000001,    0.000005,   'USD'),

  -- Google
  (NULL, 'gemini-1.5-pro',    '(?i)^gemini-1\.5-pro.*$',                      'TOKENS', 0.00000125,  0.000005,   'USD'),
  (NULL, 'gemini-1.5-flash',  '(?i)^gemini-1\.5-flash.*$',                    'TOKENS', 0.000000075, 0.0000003,  'USD');

-- Embedding models bill a single rate over total tokens.
INSERT INTO model_prices (project_id, model_name, match_pattern, unit, total_price, currency) VALUES
  (NULL, 'text-embedding-3-small', '(?i)^text-embedding-3-small$', 'TOKENS', 0.00000002, 'USD'),
  (NULL, 'text-embedding-3-large', '(?i)^text-embedding-3-large$', 'TOKENS', 0.00000013, 'USD');
