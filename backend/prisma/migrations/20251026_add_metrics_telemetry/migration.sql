-- AddMetricsTelemetry
-- Add anonymous metrics and telemetry fields to settings table

-- Add metrics fields to settings table
ALTER TABLE "settings" ADD COLUMN "metrics_enabled" BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE "settings" ADD COLUMN "metrics_anonymous_id" TEXT;
ALTER TABLE "settings" ADD COLUMN "metrics_last_sent" TIMESTAMP(3);

-- Generate UUID for existing records (if any exist)
-- This will use PostgreSQL's gen_random_uuid() function
UPDATE "settings" 
SET "metrics_anonymous_id" = gen_random_uuid()::text 
WHERE "metrics_anonymous_id" IS NULL;

