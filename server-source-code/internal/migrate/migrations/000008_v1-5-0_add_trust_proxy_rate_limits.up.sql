-- Add trust_proxy and rate limit columns to settings for in-app configuration
ALTER TABLE settings ADD COLUMN IF NOT EXISTS trust_proxy BOOLEAN;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS rate_limit_window_ms INTEGER;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS rate_limit_max INTEGER;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS auth_rate_limit_window_ms INTEGER;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS auth_rate_limit_max INTEGER;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS agent_rate_limit_window_ms INTEGER;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS agent_rate_limit_max INTEGER;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS password_rate_limit_window_ms INTEGER;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS password_rate_limit_max INTEGER;
