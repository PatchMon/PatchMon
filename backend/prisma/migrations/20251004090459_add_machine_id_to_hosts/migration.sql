-- Add machine_id column as nullable first
ALTER TABLE "hosts" ADD COLUMN "machine_id" TEXT;

-- Generate machine_ids for existing hosts using their API ID as a fallback
UPDATE "hosts" SET "machine_id" = 'migrated-' || "api_id" WHERE "machine_id" IS NULL;

-- Remove the unique constraint from friendly_name
ALTER TABLE "hosts" DROP CONSTRAINT IF EXISTS "hosts_friendly_name_key";

-- Also drop the unique index if it exists (constraint and index can exist separately)
DROP INDEX IF EXISTS "hosts_friendly_name_key";

-- Now make machine_id NOT NULL and add unique constraint
ALTER TABLE "hosts" ALTER COLUMN "machine_id" SET NOT NULL;
ALTER TABLE "hosts" ADD CONSTRAINT "hosts_machine_id_key" UNIQUE ("machine_id");

-- Create indexes for better query performance
CREATE INDEX "hosts_machine_id_idx" ON "hosts"("machine_id");
CREATE INDEX "hosts_friendly_name_idx" ON "hosts"("friendly_name");

