-- AlterTable: Add alerts_enabled column to settings table
-- This migration is safe to run multiple times and preserves existing data
-- Master switch to enable/disable the entire alerts system

-- Add alerts_enabled column with default value true (only if it doesn't exist)
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'settings' AND column_name = 'alerts_enabled'
    ) THEN
        ALTER TABLE "settings" ADD COLUMN "alerts_enabled" BOOLEAN NOT NULL DEFAULT true;
    END IF;
END $$;

