-- AlterTable: Add can_manage_superusers column to role_permissions table
-- This column was missing from the database but exists in the Prisma schema
-- This migration is safe to run multiple times and preserves existing data

-- Add can_manage_superusers column with default value (only if it doesn't exist)
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'role_permissions' AND column_name = 'can_manage_superusers'
    ) THEN
        ALTER TABLE "role_permissions" ADD COLUMN "can_manage_superusers" BOOLEAN NOT NULL DEFAULT false;
    END IF;
END $$;

