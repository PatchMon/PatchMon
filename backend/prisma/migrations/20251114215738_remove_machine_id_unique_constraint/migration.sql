-- Remove machine_id unique constraint and make it nullable
-- This allows multiple hosts with the same machine_id
-- Duplicate detection now relies on config.yml/credentials.yml checking instead

-- Drop the unique constraint
ALTER TABLE "hosts" DROP CONSTRAINT IF EXISTS "hosts_machine_id_key";

-- Make machine_id nullable
ALTER TABLE "hosts" ALTER COLUMN "machine_id" DROP NOT NULL;

-- Keep the index for query performance (but not unique)
CREATE INDEX IF NOT EXISTS "hosts_machine_id_idx" ON "hosts"("machine_id");

