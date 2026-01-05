-- Add AI Terminal Assistant settings to the settings table
ALTER TABLE "settings" ADD COLUMN IF NOT EXISTS "ai_enabled" BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE "settings" ADD COLUMN IF NOT EXISTS "ai_provider" TEXT NOT NULL DEFAULT 'openrouter';
ALTER TABLE "settings" ADD COLUMN IF NOT EXISTS "ai_model" TEXT;
ALTER TABLE "settings" ADD COLUMN IF NOT EXISTS "ai_api_key" TEXT;
