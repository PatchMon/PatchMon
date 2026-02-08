-- AlterTable: Make alert_history.user_id nullable for system actions
-- This migration is safe to run and preserves existing data
-- Allows system actions (like "created", "resolved" by system) to have null user_id

-- First, drop the foreign key constraint
DO $$ 
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.table_constraints 
        WHERE constraint_schema = 'public' 
        AND constraint_name = 'alert_history_user_id_fkey'
    ) THEN
        ALTER TABLE "alert_history" DROP CONSTRAINT "alert_history_user_id_fkey";
    END IF;
END $$;

-- Alter the column to be nullable
ALTER TABLE "alert_history" ALTER COLUMN "user_id" DROP NOT NULL;

-- Re-add the foreign key constraint (now nullable)
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints 
        WHERE constraint_schema = 'public' 
        AND constraint_name = 'alert_history_user_id_fkey'
    ) THEN
        ALTER TABLE "alert_history" 
        ADD CONSTRAINT "alert_history_user_id_fkey" 
        FOREIGN KEY ("user_id") REFERENCES "users"("id") ON DELETE CASCADE;
    END IF;
END $$;

