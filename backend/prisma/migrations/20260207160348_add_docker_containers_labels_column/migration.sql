-- AlterTable: Add labels column to docker_containers table
-- This migration is safe to run multiple times and preserves existing data
-- The column may be missing if the database was created before this column was added to the schema

-- Add labels column if it doesn't exist
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_schema = 'public' 
        AND table_name = 'docker_containers' 
        AND column_name = 'labels'
    ) THEN
        ALTER TABLE "docker_containers" ADD COLUMN "labels" JSONB;
    END IF;
END $$;

