-- CreateTable: Add alerts system tables
-- This migration is safe to run multiple times and preserves existing data
-- The tables may be missing if the database was created before these tables were added to the schema

-- Create alerts table if it doesn't exist
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.tables 
        WHERE table_schema = 'public' AND table_name = 'alerts'
    ) THEN
        CREATE TABLE "alerts" (
            "id" TEXT NOT NULL,
            "type" TEXT NOT NULL,
            "severity" TEXT NOT NULL,
            "title" TEXT NOT NULL,
            "message" TEXT NOT NULL,
            "metadata" JSONB,
            "is_active" BOOLEAN NOT NULL DEFAULT true,
            "assigned_to_user_id" TEXT,
            "resolved_at" TIMESTAMP(3),
            "resolved_by_user_id" TEXT,
            "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
            "updated_at" TIMESTAMP(3) NOT NULL,

            CONSTRAINT "alerts_pkey" PRIMARY KEY ("id")
        );

        -- Create indexes if they don't exist
        IF NOT EXISTS (
            SELECT 1 FROM pg_indexes 
            WHERE tablename = 'alerts' AND indexname = 'alerts_type_idx'
        ) THEN
            CREATE INDEX "alerts_type_idx" ON "alerts"("type");
        END IF;

        IF NOT EXISTS (
            SELECT 1 FROM pg_indexes 
            WHERE tablename = 'alerts' AND indexname = 'alerts_severity_idx'
        ) THEN
            CREATE INDEX "alerts_severity_idx" ON "alerts"("severity");
        END IF;

        IF NOT EXISTS (
            SELECT 1 FROM pg_indexes 
            WHERE tablename = 'alerts' AND indexname = 'alerts_is_active_idx'
        ) THEN
            CREATE INDEX "alerts_is_active_idx" ON "alerts"("is_active");
        END IF;

        IF NOT EXISTS (
            SELECT 1 FROM pg_indexes 
            WHERE tablename = 'alerts' AND indexname = 'alerts_assigned_to_user_id_idx'
        ) THEN
            CREATE INDEX "alerts_assigned_to_user_id_idx" ON "alerts"("assigned_to_user_id");
        END IF;

        IF NOT EXISTS (
            SELECT 1 FROM pg_indexes 
            WHERE tablename = 'alerts' AND indexname = 'alerts_created_at_idx'
        ) THEN
            CREATE INDEX "alerts_created_at_idx" ON "alerts"("created_at");
        END IF;

        -- Add foreign key constraints if they don't exist
        IF NOT EXISTS (
            SELECT 1 FROM information_schema.table_constraints 
            WHERE constraint_name = 'alerts_assigned_to_user_id_fkey'
        ) THEN
            ALTER TABLE "alerts" ADD CONSTRAINT "alerts_assigned_to_user_id_fkey" 
            FOREIGN KEY ("assigned_to_user_id") REFERENCES "users"("id") ON DELETE SET NULL ON UPDATE CASCADE;
        END IF;

        IF NOT EXISTS (
            SELECT 1 FROM information_schema.table_constraints 
            WHERE constraint_name = 'alerts_resolved_by_user_id_fkey'
        ) THEN
            ALTER TABLE "alerts" ADD CONSTRAINT "alerts_resolved_by_user_id_fkey" 
            FOREIGN KEY ("resolved_by_user_id") REFERENCES "users"("id") ON DELETE SET NULL ON UPDATE CASCADE;
        END IF;
    END IF;
END $$;

-- Create alert_history table if it doesn't exist
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.tables 
        WHERE table_schema = 'public' AND table_name = 'alert_history'
    ) THEN
        CREATE TABLE "alert_history" (
            "id" TEXT NOT NULL,
            "alert_id" TEXT NOT NULL,
            "user_id" TEXT NOT NULL,
            "action" TEXT NOT NULL,
            "metadata" JSONB,
            "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

            CONSTRAINT "alert_history_pkey" PRIMARY KEY ("id")
        );

        -- Create indexes if they don't exist
        IF NOT EXISTS (
            SELECT 1 FROM pg_indexes 
            WHERE tablename = 'alert_history' AND indexname = 'alert_history_alert_id_idx'
        ) THEN
            CREATE INDEX "alert_history_alert_id_idx" ON "alert_history"("alert_id");
        END IF;

        IF NOT EXISTS (
            SELECT 1 FROM pg_indexes 
            WHERE tablename = 'alert_history' AND indexname = 'alert_history_user_id_idx'
        ) THEN
            CREATE INDEX "alert_history_user_id_idx" ON "alert_history"("user_id");
        END IF;

        IF NOT EXISTS (
            SELECT 1 FROM pg_indexes 
            WHERE tablename = 'alert_history' AND indexname = 'alert_history_action_idx'
        ) THEN
            CREATE INDEX "alert_history_action_idx" ON "alert_history"("action");
        END IF;

        IF NOT EXISTS (
            SELECT 1 FROM pg_indexes 
            WHERE tablename = 'alert_history' AND indexname = 'alert_history_created_at_idx'
        ) THEN
            CREATE INDEX "alert_history_created_at_idx" ON "alert_history"("created_at");
        END IF;

        IF NOT EXISTS (
            SELECT 1 FROM pg_indexes 
            WHERE tablename = 'alert_history' AND indexname = 'alert_history_alert_id_created_at_idx'
        ) THEN
            CREATE INDEX "alert_history_alert_id_created_at_idx" ON "alert_history"("alert_id", "created_at");
        END IF;

        -- Add foreign key constraints if they don't exist
        IF NOT EXISTS (
            SELECT 1 FROM information_schema.table_constraints 
            WHERE constraint_name = 'alert_history_alert_id_fkey'
        ) THEN
            ALTER TABLE "alert_history" ADD CONSTRAINT "alert_history_alert_id_fkey" 
            FOREIGN KEY ("alert_id") REFERENCES "alerts"("id") ON DELETE CASCADE ON UPDATE CASCADE;
        END IF;

        IF NOT EXISTS (
            SELECT 1 FROM information_schema.table_constraints 
            WHERE constraint_name = 'alert_history_user_id_fkey'
        ) THEN
            ALTER TABLE "alert_history" ADD CONSTRAINT "alert_history_user_id_fkey" 
            FOREIGN KEY ("user_id") REFERENCES "users"("id") ON DELETE CASCADE ON UPDATE CASCADE;
        END IF;
    END IF;
END $$;

-- Create alert_actions table if it doesn't exist
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.tables 
        WHERE table_schema = 'public' AND table_name = 'alert_actions'
    ) THEN
        CREATE TABLE "alert_actions" (
            "id" TEXT NOT NULL,
            "name" TEXT NOT NULL,
            "display_name" TEXT NOT NULL,
            "description" TEXT,
            "is_state_action" BOOLEAN NOT NULL DEFAULT false,
            "severity_override" TEXT,
            "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
            "updated_at" TIMESTAMP(3) NOT NULL,

            CONSTRAINT "alert_actions_pkey" PRIMARY KEY ("id")
        );

        -- Create unique constraint on name
        IF NOT EXISTS (
            SELECT 1 FROM information_schema.table_constraints 
            WHERE constraint_name = 'alert_actions_name_key'
        ) THEN
            ALTER TABLE "alert_actions" ADD CONSTRAINT "alert_actions_name_key" UNIQUE ("name");
        END IF;

        -- Create indexes if they don't exist
        IF NOT EXISTS (
            SELECT 1 FROM pg_indexes 
            WHERE tablename = 'alert_actions' AND indexname = 'alert_actions_name_idx'
        ) THEN
            CREATE INDEX "alert_actions_name_idx" ON "alert_actions"("name");
        END IF;

        IF NOT EXISTS (
            SELECT 1 FROM pg_indexes 
            WHERE tablename = 'alert_actions' AND indexname = 'alert_actions_is_state_action_idx'
        ) THEN
            CREATE INDEX "alert_actions_is_state_action_idx" ON "alert_actions"("is_state_action");
        END IF;

        -- Insert default actions
        INSERT INTO "alert_actions" ("id", "name", "display_name", "description", "is_state_action", "created_at", "updated_at")
        VALUES 
            (gen_random_uuid()::text, 'created', 'Created', 'Alert was created (system action)', false, NOW(), NOW()),
            (gen_random_uuid()::text, 'acknowledged', 'Acknowledged', 'User acknowledged the alert', false, NOW(), NOW()),
            (gen_random_uuid()::text, 'assigned', 'Assigned', 'Alert was assigned/delegated to a user', true, NOW(), NOW()),
            (gen_random_uuid()::text, 'unassigned', 'Unassigned', 'Alert assignment was removed', true, NOW(), NOW()),
            (gen_random_uuid()::text, 'silenced', 'Silenced', 'Alert was silenced', true, NOW(), NOW()),
            (gen_random_uuid()::text, 'unsilenced', 'Unsilenced', 'Alert was unsilenced', true, NOW(), NOW()),
            (gen_random_uuid()::text, 'done', 'Mark as Done', 'Alert was marked as done', true, NOW(), NOW()),
            (gen_random_uuid()::text, 'resolved', 'Resolved', 'Alert was resolved', true, NOW(), NOW()),
            (gen_random_uuid()::text, 'updated', 'Updated', 'Alert was updated (system action)', false, NOW(), NOW())
        ON CONFLICT ("name") DO NOTHING;
    END IF;
END $$;

-- Create alert_config table if it doesn't exist
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.tables 
        WHERE table_schema = 'public' AND table_name = 'alert_config'
    ) THEN
        CREATE TABLE "alert_config" (
            "id" TEXT NOT NULL,
            "alert_type" TEXT NOT NULL,
            "is_enabled" BOOLEAN NOT NULL DEFAULT true,
            "default_severity" TEXT NOT NULL DEFAULT 'informational',
            "auto_assign_enabled" BOOLEAN NOT NULL DEFAULT false,
            "auto_assign_user_id" TEXT,
            "auto_assign_rule" TEXT,
            "auto_assign_conditions" JSONB,
            "retention_days" INTEGER,
            "auto_resolve_after_days" INTEGER,
            "cleanup_resolved_only" BOOLEAN NOT NULL DEFAULT true,
            "notification_enabled" BOOLEAN NOT NULL DEFAULT true,
            "escalation_enabled" BOOLEAN NOT NULL DEFAULT false,
            "escalation_after_hours" INTEGER,
            "metadata" JSONB,
            "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
            "updated_at" TIMESTAMP(3) NOT NULL,

            CONSTRAINT "alert_config_pkey" PRIMARY KEY ("id")
        );

        -- Create unique constraint on alert_type
        IF NOT EXISTS (
            SELECT 1 FROM information_schema.table_constraints 
            WHERE constraint_name = 'alert_config_alert_type_key'
        ) THEN
            ALTER TABLE "alert_config" ADD CONSTRAINT "alert_config_alert_type_key" UNIQUE ("alert_type");
        END IF;

        -- Create indexes if they don't exist
        IF NOT EXISTS (
            SELECT 1 FROM pg_indexes 
            WHERE tablename = 'alert_config' AND indexname = 'alert_config_alert_type_idx'
        ) THEN
            CREATE INDEX "alert_config_alert_type_idx" ON "alert_config"("alert_type");
        END IF;

        IF NOT EXISTS (
            SELECT 1 FROM pg_indexes 
            WHERE tablename = 'alert_config' AND indexname = 'alert_config_is_enabled_idx'
        ) THEN
            CREATE INDEX "alert_config_is_enabled_idx" ON "alert_config"("is_enabled");
        END IF;

        IF NOT EXISTS (
            SELECT 1 FROM pg_indexes 
            WHERE tablename = 'alert_config' AND indexname = 'alert_config_auto_assign_enabled_idx'
        ) THEN
            CREATE INDEX "alert_config_auto_assign_enabled_idx" ON "alert_config"("auto_assign_enabled");
        END IF;

        -- Add foreign key constraint if it doesn't exist
        IF NOT EXISTS (
            SELECT 1 FROM information_schema.table_constraints 
            WHERE constraint_name = 'alert_config_auto_assign_user_id_fkey'
        ) THEN
            ALTER TABLE "alert_config" ADD CONSTRAINT "alert_config_auto_assign_user_id_fkey" 
            FOREIGN KEY ("auto_assign_user_id") REFERENCES "users"("id") ON DELETE SET NULL ON UPDATE CASCADE;
        END IF;

        -- Insert default configurations for known alert types
        INSERT INTO "alert_config" ("id", "alert_type", "is_enabled", "default_severity", "created_at", "updated_at")
        VALUES 
            (gen_random_uuid()::text, 'server_update', true, 'informational', NOW(), NOW()),
            (gen_random_uuid()::text, 'agent_update', true, 'informational', NOW(), NOW()),
            (gen_random_uuid()::text, 'host_down', true, 'warning', NOW(), NOW())
        ON CONFLICT ("alert_type") DO NOTHING;
    END IF;
END $$;

