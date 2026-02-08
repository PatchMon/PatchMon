-- Fix dashboard preferences unique constraint
-- This migration fixes the unique constraint on dashboard_preferences table

-- Drop existing indexes if they exist
DROP INDEX IF EXISTS "dashboard_preferences_card_id_key";
DROP INDEX IF EXISTS "dashboard_preferences_user_id_card_id_key";
DROP INDEX IF EXISTS "dashboard_preferences_user_id_key";

-- Add the correct unique constraint
ALTER TABLE "dashboard_preferences" ADD CONSTRAINT "dashboard_preferences_user_id_card_id_key" UNIQUE ("user_id", "card_id");
