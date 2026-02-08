-- Remove dashboard preferences population
-- This migration clears all existing dashboard preferences so they can be recreated
-- with the correct default order by server.js initialization

-- Clear all existing dashboard preferences
-- This ensures users get the correct default order from server.js
DELETE FROM dashboard_preferences;

-- Recreate indexes for better performance
CREATE INDEX IF NOT EXISTS "dashboard_preferences_user_id_idx" ON "dashboard_preferences"("user_id");
CREATE INDEX IF NOT EXISTS "dashboard_preferences_card_id_idx" ON "dashboard_preferences"("card_id");
CREATE INDEX IF NOT EXISTS "dashboard_preferences_user_card_idx" ON "dashboard_preferences"("user_id", "card_id");