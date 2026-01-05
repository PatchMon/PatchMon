-- Add integration_status_interval setting
-- Controls how often agents report their integration/scanner status (default 30 minutes)

ALTER TABLE "settings" ADD COLUMN "integration_status_interval" INTEGER NOT NULL DEFAULT 30;
