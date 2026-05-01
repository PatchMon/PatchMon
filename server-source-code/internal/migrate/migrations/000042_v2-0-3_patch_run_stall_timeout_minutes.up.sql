-- Add patch_run_stall_timeout_minutes setting for in-app configuration
-- (env -> DB -> default). NULL means "use env or built-in default".
ALTER TABLE settings ADD COLUMN IF NOT EXISTS patch_run_stall_timeout_minutes INTEGER;
