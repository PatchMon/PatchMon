-- Add agent_reports_retention_days setting for in-app configuration of the
-- Agent Activity (update_history) retention sweep. NULL means "use env or
-- built-in default" (env -> DB -> 30 days).
ALTER TABLE settings ADD COLUMN IF NOT EXISTS agent_reports_retention_days INTEGER;
