-- Persist last known compliance scanner status on host so UI can show accurate state
-- when Redis cache is empty (e.g. before agent has reported after tab open).
-- No existing data is modified; new columns are nullable.

ALTER TABLE "hosts" ADD COLUMN IF NOT EXISTS "compliance_scanner_status" JSONB;
ALTER TABLE "hosts" ADD COLUMN IF NOT EXISTS "compliance_scanner_updated_at" TIMESTAMP(3);
