-- Convert notification_routes from single-select to multi-select for events, host groups, and hosts.

-- 1. event_type TEXT → event_types JSONB (array of strings)
ALTER TABLE notification_routes ADD COLUMN IF NOT EXISTS event_types JSONB NOT NULL DEFAULT '["*"]'::jsonb;
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'notification_routes' AND column_name = 'event_type') THEN
        UPDATE notification_routes SET event_types = jsonb_build_array(event_type) WHERE event_type IS NOT NULL AND event_type != '';
    END IF;
END $$;
ALTER TABLE notification_routes DROP COLUMN IF EXISTS event_type;

-- 2. host_group_id TEXT → host_group_ids JSONB (array of strings)
ALTER TABLE notification_routes ADD COLUMN IF NOT EXISTS host_group_ids JSONB NOT NULL DEFAULT '[]'::jsonb;
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'notification_routes' AND column_name = 'host_group_id') THEN
        UPDATE notification_routes SET host_group_ids = jsonb_build_array(host_group_id) WHERE host_group_id IS NOT NULL AND host_group_id != '';
    END IF;
END $$;
ALTER TABLE notification_routes DROP CONSTRAINT IF EXISTS notification_routes_host_group_id_fkey;
ALTER TABLE notification_routes DROP COLUMN IF EXISTS host_group_id;

-- 3. Add host_ids for individual host targeting
ALTER TABLE notification_routes ADD COLUMN IF NOT EXISTS host_ids JSONB NOT NULL DEFAULT '[]'::jsonb;

-- 4. Drop old index, create new one for JSONB containment
DROP INDEX IF EXISTS idx_notification_routes_event;
CREATE INDEX IF NOT EXISTS idx_notification_routes_enabled ON notification_routes(enabled) WHERE enabled = true;
