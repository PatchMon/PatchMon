-- AlterTable: Add AI Terminal Assistant settings columns to settings table
-- These columns were missing from the database but exist in the Prisma schema
-- This migration is safe to run multiple times and preserves existing data

-- Add ai_enabled column with default value (only if it doesn't exist)
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'settings' AND column_name = 'ai_enabled'
    ) THEN
        ALTER TABLE "settings" ADD COLUMN "ai_enabled" BOOLEAN NOT NULL DEFAULT false;
    END IF;
END $$;

-- Add ai_provider column with default value (only if it doesn't exist)
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'settings' AND column_name = 'ai_provider'
    ) THEN
        ALTER TABLE "settings" ADD COLUMN "ai_provider" TEXT NOT NULL DEFAULT 'openrouter';
    END IF;
END $$;

-- Add ai_model column (nullable, only if it doesn't exist)
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'settings' AND column_name = 'ai_model'
    ) THEN
        ALTER TABLE "settings" ADD COLUMN "ai_model" TEXT;
    END IF;
END $$;

-- Add ai_api_key column (nullable, for encrypted API key, only if it doesn't exist)
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'settings' AND column_name = 'ai_api_key'
    ) THEN
        ALTER TABLE "settings" ADD COLUMN "ai_api_key" TEXT;
    END IF;
END $$;

