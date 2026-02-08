-- AlterTable: Add OIDC and avatar columns to users table
-- These columns were missing from the database but exist in the Prisma schema
-- This migration is safe to run multiple times and preserves existing data

-- Add oidc_sub column (nullable, unique) - only if it doesn't exist
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'users' AND column_name = 'oidc_sub'
    ) THEN
        ALTER TABLE "users" ADD COLUMN "oidc_sub" TEXT;
        -- Create unique index if it doesn't exist
        IF NOT EXISTS (
            SELECT 1 FROM pg_indexes 
            WHERE tablename = 'users' AND indexname = 'users_oidc_sub_key'
        ) THEN
            CREATE UNIQUE INDEX "users_oidc_sub_key" ON "users"("oidc_sub");
        END IF;
    END IF;
END $$;

-- Add oidc_provider column (nullable) - only if it doesn't exist
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'users' AND column_name = 'oidc_provider'
    ) THEN
        ALTER TABLE "users" ADD COLUMN "oidc_provider" TEXT;
    END IF;
END $$;

-- Add avatar_url column (nullable) - only if it doesn't exist
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'users' AND column_name = 'avatar_url'
    ) THEN
        ALTER TABLE "users" ADD COLUMN "avatar_url" TEXT;
    END IF;
END $$;

