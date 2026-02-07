-- AlterTable: Add compliance columns to hosts table
-- These columns were missing from the database but exist in the Prisma schema
-- This migration is safe to run multiple times and preserves existing data

-- Add compliance_enabled column with default value (only if it doesn't exist)
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'hosts' AND column_name = 'compliance_enabled'
    ) THEN
        ALTER TABLE "hosts" ADD COLUMN "compliance_enabled" BOOLEAN NOT NULL DEFAULT false;
    END IF;
END $$;

-- Add compliance_on_demand_only column with default value (only if it doesn't exist)
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'hosts' AND column_name = 'compliance_on_demand_only'
    ) THEN
        ALTER TABLE "hosts" ADD COLUMN "compliance_on_demand_only" BOOLEAN NOT NULL DEFAULT true;
    END IF;
END $$;

