-- Add ui_preferences JSON column to users for per-user UI state (e.g. hosts table column config).
-- Nullable so existing users retain data and no backfill is required.
ALTER TABLE "users" ADD COLUMN IF NOT EXISTS "ui_preferences" JSONB;
