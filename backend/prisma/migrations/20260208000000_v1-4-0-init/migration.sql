-- Migration: v1.4.0 Init
-- Consolidates all migrations after v1.3.7
-- This migration is safe to run on databases that already have some of these changes applied

-- ============================================================================
-- Add AI settings columns to settings table
-- ============================================================================
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'settings' AND column_name = 'ai_enabled'
    ) THEN
        ALTER TABLE "settings" ADD COLUMN "ai_enabled" BOOLEAN NOT NULL DEFAULT false;
    END IF;
END $$;

DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'settings' AND column_name = 'ai_provider'
    ) THEN
        ALTER TABLE "settings" ADD COLUMN "ai_provider" TEXT NOT NULL DEFAULT 'openrouter';
    END IF;
END $$;

DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'settings' AND column_name = 'ai_model'
    ) THEN
        ALTER TABLE "settings" ADD COLUMN "ai_model" TEXT;
    END IF;
END $$;

DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'settings' AND column_name = 'ai_api_key'
    ) THEN
        ALTER TABLE "settings" ADD COLUMN "ai_api_key" TEXT;
    END IF;
END $$;

-- ============================================================================
-- Add can_manage_superusers to role_permissions
-- ============================================================================
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'role_permissions' AND column_name = 'can_manage_superusers'
    ) THEN
        ALTER TABLE "role_permissions" ADD COLUMN "can_manage_superusers" BOOLEAN NOT NULL DEFAULT false;
    END IF;
END $$;

-- ============================================================================
-- Add compliance columns to hosts table
-- ============================================================================
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'hosts' AND column_name = 'compliance_enabled'
    ) THEN
        ALTER TABLE "hosts" ADD COLUMN "compliance_enabled" BOOLEAN NOT NULL DEFAULT false;
    END IF;
END $$;

DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'hosts' AND column_name = 'compliance_on_demand_only'
    ) THEN
        ALTER TABLE "hosts" ADD COLUMN "compliance_on_demand_only" BOOLEAN NOT NULL DEFAULT true;
    END IF;
END $$;

-- ============================================================================
-- Add OIDC and avatar columns to users table
-- ============================================================================
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'users' AND column_name = 'oidc_sub'
    ) THEN
        ALTER TABLE "users" ADD COLUMN "oidc_sub" TEXT;
        IF NOT EXISTS (
            SELECT 1 FROM pg_indexes 
            WHERE tablename = 'users' AND indexname = 'users_oidc_sub_key'
        ) THEN
            CREATE UNIQUE INDEX "users_oidc_sub_key" ON "users"("oidc_sub");
        END IF;
    END IF;
END $$;

DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'users' AND column_name = 'oidc_provider'
    ) THEN
        ALTER TABLE "users" ADD COLUMN "oidc_provider" TEXT;
    END IF;
END $$;

DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'users' AND column_name = 'avatar_url'
    ) THEN
        ALTER TABLE "users" ADD COLUMN "avatar_url" TEXT;
    END IF;
END $$;

-- ============================================================================
-- Add default compliance mode to settings
-- ============================================================================
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_schema = 'public' 
        AND table_name = 'settings' 
        AND column_name = 'default_compliance_mode'
    ) THEN
        ALTER TABLE "settings" ADD COLUMN "default_compliance_mode" TEXT NOT NULL DEFAULT 'on-demand';
    END IF;
END $$;

-- ============================================================================
-- Create audit_logs table
-- ============================================================================
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.tables 
        WHERE table_schema = 'public' AND table_name = 'audit_logs'
    ) THEN
        CREATE TABLE "audit_logs" (
            "id" TEXT NOT NULL,
            "event" TEXT NOT NULL,
            "user_id" TEXT,
            "target_user_id" TEXT,
            "ip_address" TEXT,
            "user_agent" TEXT,
            "request_id" TEXT,
            "details" TEXT,
            "success" BOOLEAN NOT NULL DEFAULT true,
            "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

            CONSTRAINT "audit_logs_pkey" PRIMARY KEY ("id")
        );

        -- Create indexes if they don't exist
        IF NOT EXISTS (
            SELECT 1 FROM pg_indexes 
            WHERE tablename = 'audit_logs' AND indexname = 'audit_logs_event_idx'
        ) THEN
            CREATE INDEX "audit_logs_event_idx" ON "audit_logs"("event");
        END IF;

        IF NOT EXISTS (
            SELECT 1 FROM pg_indexes 
            WHERE tablename = 'audit_logs' AND indexname = 'audit_logs_user_id_idx'
        ) THEN
            CREATE INDEX "audit_logs_user_id_idx" ON "audit_logs"("user_id");
        END IF;

        IF NOT EXISTS (
            SELECT 1 FROM pg_indexes 
            WHERE tablename = 'audit_logs' AND indexname = 'audit_logs_target_user_id_idx'
        ) THEN
            CREATE INDEX "audit_logs_target_user_id_idx" ON "audit_logs"("target_user_id");
        END IF;

        IF NOT EXISTS (
            SELECT 1 FROM pg_indexes 
            WHERE tablename = 'audit_logs' AND indexname = 'audit_logs_created_at_idx'
        ) THEN
            CREATE INDEX "audit_logs_created_at_idx" ON "audit_logs"("created_at");
        END IF;

        IF NOT EXISTS (
            SELECT 1 FROM pg_indexes 
            WHERE tablename = 'audit_logs' AND indexname = 'audit_logs_success_idx'
        ) THEN
            CREATE INDEX "audit_logs_success_idx" ON "audit_logs"("success");
        END IF;
    END IF;
END $$;

-- ============================================================================
-- Add labels column to docker_containers
-- ============================================================================
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

-- ============================================================================
-- Create compliance tables if they don't exist
-- ============================================================================
-- Create compliance_profiles table if it doesn't exist (must be created first as other tables depend on it)
CREATE TABLE IF NOT EXISTS "compliance_profiles" (
    "id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "type" TEXT NOT NULL,
    "os_family" TEXT,
    "version" TEXT,
    "description" TEXT,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "compliance_profiles_pkey" PRIMARY KEY ("id")
);

-- Create compliance_scans table if it doesn't exist
CREATE TABLE IF NOT EXISTS "compliance_scans" (
    "id" TEXT NOT NULL,
    "host_id" TEXT NOT NULL,
    "profile_id" TEXT NOT NULL,
    "started_at" TIMESTAMP(3) NOT NULL,
    "completed_at" TIMESTAMP(3),
    "status" TEXT NOT NULL,
    "total_rules" INTEGER NOT NULL DEFAULT 0,
    "passed" INTEGER NOT NULL DEFAULT 0,
    "failed" INTEGER NOT NULL DEFAULT 0,
    "warnings" INTEGER NOT NULL DEFAULT 0,
    "skipped" INTEGER NOT NULL DEFAULT 0,
    "not_applicable" INTEGER NOT NULL DEFAULT 0,
    "score" DOUBLE PRECISION,
    "error_message" TEXT,
    "raw_output" TEXT,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "compliance_scans_pkey" PRIMARY KEY ("id")
);

-- Create compliance_rules table if it doesn't exist
CREATE TABLE IF NOT EXISTS "compliance_rules" (
    "id" TEXT NOT NULL,
    "profile_id" TEXT NOT NULL,
    "rule_ref" TEXT NOT NULL,
    "title" TEXT NOT NULL,
    "description" TEXT,
    "rationale" TEXT,
    "severity" TEXT,
    "section" TEXT,
    "remediation" TEXT,

    CONSTRAINT "compliance_rules_pkey" PRIMARY KEY ("id")
);

-- Create compliance_results table if it doesn't exist
CREATE TABLE IF NOT EXISTS "compliance_results" (
    "id" TEXT NOT NULL,
    "scan_id" TEXT NOT NULL,
    "rule_id" TEXT NOT NULL,
    "status" TEXT NOT NULL,
    "finding" TEXT,
    "actual" TEXT,
    "expected" TEXT,
    "remediation" TEXT,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "compliance_results_pkey" PRIMARY KEY ("id")
);

-- Create indexes if they don't exist (using DO block for conditional creation)
DO $$
BEGIN
    -- compliance_profiles indexes
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'compliance_profiles_name_key') THEN
        CREATE UNIQUE INDEX "compliance_profiles_name_key" ON "compliance_profiles"("name");
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'compliance_profiles_type_idx') THEN
        CREATE INDEX "compliance_profiles_type_idx" ON "compliance_profiles"("type");
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'compliance_profiles_os_family_idx') THEN
        CREATE INDEX "compliance_profiles_os_family_idx" ON "compliance_profiles"("os_family");
    END IF;
    
    -- compliance_scans indexes
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'compliance_scans_host_id_idx') THEN
        CREATE INDEX "compliance_scans_host_id_idx" ON "compliance_scans"("host_id");
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'compliance_scans_profile_id_idx') THEN
        CREATE INDEX "compliance_scans_profile_id_idx" ON "compliance_scans"("profile_id");
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'compliance_scans_status_idx') THEN
        CREATE INDEX "compliance_scans_status_idx" ON "compliance_scans"("status");
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'compliance_scans_started_at_idx') THEN
        CREATE INDEX "compliance_scans_started_at_idx" ON "compliance_scans"("started_at");
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'compliance_scans_completed_at_idx') THEN
        CREATE INDEX "compliance_scans_completed_at_idx" ON "compliance_scans"("completed_at");
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'compliance_scans_host_id_started_at_idx') THEN
        CREATE INDEX "compliance_scans_host_id_started_at_idx" ON "compliance_scans"("host_id", "started_at");
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'compliance_scans_host_id_profile_id_idx') THEN
        CREATE INDEX "compliance_scans_host_id_profile_id_idx" ON "compliance_scans"("host_id", "profile_id");
    END IF;
    
    -- compliance_rules indexes
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'compliance_rules_profile_id_idx') THEN
        CREATE INDEX "compliance_rules_profile_id_idx" ON "compliance_rules"("profile_id");
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'compliance_rules_severity_idx') THEN
        CREATE INDEX "compliance_rules_severity_idx" ON "compliance_rules"("severity");
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'compliance_rules_section_idx') THEN
        CREATE INDEX "compliance_rules_section_idx" ON "compliance_rules"("section");
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'compliance_rules_profile_id_rule_ref_key') THEN
        CREATE UNIQUE INDEX "compliance_rules_profile_id_rule_ref_key" ON "compliance_rules"("profile_id", "rule_ref");
    END IF;
    
    -- compliance_results indexes
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'compliance_results_scan_id_idx') THEN
        CREATE INDEX "compliance_results_scan_id_idx" ON "compliance_results"("scan_id");
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'compliance_results_rule_id_idx') THEN
        CREATE INDEX "compliance_results_rule_id_idx" ON "compliance_results"("rule_id");
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'compliance_results_status_idx') THEN
        CREATE INDEX "compliance_results_status_idx" ON "compliance_results"("status");
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'compliance_results_scan_id_status_idx') THEN
        CREATE INDEX "compliance_results_scan_id_status_idx" ON "compliance_results"("scan_id", "status");
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'compliance_results_scan_id_rule_id_key') THEN
        CREATE UNIQUE INDEX "compliance_results_scan_id_rule_id_key" ON "compliance_results"("scan_id", "rule_id");
    END IF;
END $$;

-- Add foreign keys for compliance tables if they don't exist
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints 
        WHERE constraint_name = 'compliance_scans_host_id_fkey'
    ) THEN
        ALTER TABLE "compliance_scans" ADD CONSTRAINT "compliance_scans_host_id_fkey" FOREIGN KEY ("host_id") REFERENCES "hosts"("id") ON DELETE CASCADE ON UPDATE CASCADE;
    END IF;
    
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints 
        WHERE constraint_name = 'compliance_scans_profile_id_fkey'
    ) THEN
        ALTER TABLE "compliance_scans" ADD CONSTRAINT "compliance_scans_profile_id_fkey" FOREIGN KEY ("profile_id") REFERENCES "compliance_profiles"("id") ON DELETE CASCADE ON UPDATE CASCADE;
    END IF;
    
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints 
        WHERE constraint_name = 'compliance_rules_profile_id_fkey'
    ) THEN
        ALTER TABLE "compliance_rules" ADD CONSTRAINT "compliance_rules_profile_id_fkey" FOREIGN KEY ("profile_id") REFERENCES "compliance_profiles"("id") ON DELETE CASCADE ON UPDATE CASCADE;
    END IF;
    
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints 
        WHERE constraint_name = 'compliance_results_scan_id_fkey'
    ) THEN
        ALTER TABLE "compliance_results" ADD CONSTRAINT "compliance_results_scan_id_fkey" FOREIGN KEY ("scan_id") REFERENCES "compliance_scans"("id") ON DELETE CASCADE ON UPDATE CASCADE;
    END IF;
    
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints 
        WHERE constraint_name = 'compliance_results_rule_id_fkey'
    ) THEN
        ALTER TABLE "compliance_results" ADD CONSTRAINT "compliance_results_rule_id_fkey" FOREIGN KEY ("rule_id") REFERENCES "compliance_rules"("id") ON DELETE CASCADE ON UPDATE CASCADE;
    END IF;
END $$;

-- ============================================================================
-- Add alerts system
-- ============================================================================
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
    END IF;
END $$;

-- Create alert_history table
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.tables 
        WHERE table_schema = 'public' AND table_name = 'alert_history'
    ) THEN
        CREATE TABLE "alert_history" (
            "id" TEXT NOT NULL,
            "alert_id" TEXT NOT NULL,
            "user_id" TEXT,
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
            WHERE tablename = 'alert_history' AND indexname = 'alert_history_created_at_idx'
        ) THEN
            CREATE INDEX "alert_history_created_at_idx" ON "alert_history"("created_at");
        END IF;
    END IF;
END $$;

-- Add foreign keys for alerts if they don't exist
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints 
        WHERE constraint_name = 'alerts_assigned_to_user_id_fkey'
    ) THEN
        ALTER TABLE "alerts" ADD CONSTRAINT "alerts_assigned_to_user_id_fkey" FOREIGN KEY ("assigned_to_user_id") REFERENCES "users"("id") ON DELETE SET NULL ON UPDATE CASCADE;
    END IF;
    
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints 
        WHERE constraint_name = 'alerts_resolved_by_user_id_fkey'
    ) THEN
        ALTER TABLE "alerts" ADD CONSTRAINT "alerts_resolved_by_user_id_fkey" FOREIGN KEY ("resolved_by_user_id") REFERENCES "users"("id") ON DELETE SET NULL ON UPDATE CASCADE;
    END IF;
    
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints 
        WHERE constraint_name = 'alert_history_alert_id_fkey'
    ) THEN
        ALTER TABLE "alert_history" ADD CONSTRAINT "alert_history_alert_id_fkey" FOREIGN KEY ("alert_id") REFERENCES "alerts"("id") ON DELETE CASCADE ON UPDATE CASCADE;
    END IF;
    
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints 
        WHERE constraint_name = 'alert_history_user_id_fkey'
    ) THEN
        ALTER TABLE "alert_history" ADD CONSTRAINT "alert_history_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users"("id") ON DELETE SET NULL ON UPDATE CASCADE;
    END IF;
END $$;

-- ============================================================================
-- Add alerts_enabled setting
-- ============================================================================
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_schema = 'public' 
        AND table_name = 'settings' 
        AND column_name = 'alerts_enabled'
    ) THEN
        ALTER TABLE "settings" ADD COLUMN "alerts_enabled" BOOLEAN NOT NULL DEFAULT true;
    END IF;
END $$;

-- ============================================================================
-- Make alert_history user_id nullable
-- ============================================================================
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_schema = 'public' 
        AND table_name = 'alert_history' 
        AND column_name = 'user_id'
        AND is_nullable = 'NO'
    ) THEN
        ALTER TABLE "alert_history" ALTER COLUMN "user_id" DROP NOT NULL;
    END IF;
END $$;

-- ============================================================================
-- Add host_down_alerts_enabled to hosts
-- ============================================================================
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_schema = 'public' 
        AND table_name = 'hosts' 
        AND column_name = 'host_down_alerts_enabled'
    ) THEN
        ALTER TABLE "hosts" ADD COLUMN "host_down_alerts_enabled" BOOLEAN NOT NULL DEFAULT true;
    END IF;
END $$;

-- ============================================================================
-- Make password_hash nullable in users table
-- ============================================================================
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_schema = 'public' 
        AND table_name = 'users' 
        AND column_name = 'password_hash'
        AND is_nullable = 'NO'
    ) THEN
        ALTER TABLE "users" ALTER COLUMN "password_hash" DROP NOT NULL;
    END IF;
END $$;

-- ============================================================================
-- Fix alert_history: rename details to metadata if it exists, or add metadata if missing
-- ============================================================================
DO $$
BEGIN
    -- Check if details column exists (old name)
    IF EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_schema = 'public' 
        AND table_name = 'alert_history' 
        AND column_name = 'details'
    ) THEN
        -- Rename details to metadata
        ALTER TABLE "alert_history" RENAME COLUMN "details" TO "metadata";
    -- Check if metadata column doesn't exist at all
    ELSIF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_schema = 'public' 
        AND table_name = 'alert_history' 
        AND column_name = 'metadata'
    ) THEN
        -- Add metadata column
        ALTER TABLE "alert_history" ADD COLUMN "metadata" JSONB;
    END IF;
END $$;

-- ============================================================================
-- Create alert_actions table
-- ============================================================================
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
            SELECT 1 FROM pg_indexes 
            WHERE tablename = 'alert_actions' AND indexname = 'alert_actions_name_key'
        ) THEN
            CREATE UNIQUE INDEX "alert_actions_name_key" ON "alert_actions"("name");
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
    END IF;
END $$;

-- ============================================================================
-- Create alert_config table
-- ============================================================================
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
            SELECT 1 FROM pg_indexes 
            WHERE tablename = 'alert_config' AND indexname = 'alert_config_alert_type_key'
        ) THEN
            CREATE UNIQUE INDEX "alert_config_alert_type_key" ON "alert_config"("alert_type");
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
    END IF;
END $$;

-- Add foreign key for alert_config.auto_assign_user_id if it doesn't exist
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints 
        WHERE constraint_name = 'alert_config_auto_assign_user_id_fkey'
    ) THEN
        ALTER TABLE "alert_config" ADD CONSTRAINT "alert_config_auto_assign_user_id_fkey" FOREIGN KEY ("auto_assign_user_id") REFERENCES "users"("id") ON DELETE SET NULL ON UPDATE CASCADE;
    END IF;
END $$;

-- ============================================================================
-- Seed default alert_actions data
-- These are the action types used by the reporting/alerts system.
-- Uses ON CONFLICT to avoid duplicates on existing installs.
-- ============================================================================

INSERT INTO "alert_actions" ("id", "name", "display_name", "description", "is_state_action", "severity_override", "created_at", "updated_at")
VALUES
    (gen_random_uuid(), 'created', 'Created', 'Alert was created (system action)', false, NULL, NOW(), NOW()),
    (gen_random_uuid(), 'acknowledged', 'Acknowledged', 'User acknowledged the alert', false, NULL, NOW(), NOW()),
    (gen_random_uuid(), 'assigned', 'Assigned', 'Alert was assigned/delegated to a user', true, NULL, NOW(), NOW()),
    (gen_random_uuid(), 'unassigned', 'Unassigned', 'Alert assignment was removed', true, NULL, NOW(), NOW()),
    (gen_random_uuid(), 'silenced', 'Silenced', 'Alert was silenced', true, NULL, NOW(), NOW()),
    (gen_random_uuid(), 'unsilenced', 'Unsilenced', 'Alert was unsilenced', true, NULL, NOW(), NOW()),
    (gen_random_uuid(), 'done', 'Mark as Done', 'Alert was marked as done', true, NULL, NOW(), NOW()),
    (gen_random_uuid(), 'resolved', 'Resolved', 'Alert was resolved', true, NULL, NOW(), NOW()),
    (gen_random_uuid(), 'updated', 'Updated', 'Alert was updated (system action)', false, NULL, NOW(), NOW())
ON CONFLICT ("name") DO NOTHING;

-- ============================================================================
-- Seed default alert_config data
-- These are the alert type configurations shown in Alert Settings.
-- Uses ON CONFLICT to preserve any existing user-customised settings.
-- ============================================================================

INSERT INTO "alert_config" ("id", "alert_type", "is_enabled", "default_severity", "auto_assign_enabled", "notification_enabled", "cleanup_resolved_only", "created_at", "updated_at")
VALUES
    (gen_random_uuid(), 'host_down', true, 'warning', false, true, true, NOW(), NOW()),
    (gen_random_uuid(), 'server_update', true, 'informational', false, true, true, NOW(), NOW()),
    (gen_random_uuid(), 'agent_update', true, 'informational', false, true, true, NOW(), NOW())
ON CONFLICT ("alert_type") DO NOTHING;

-- ============================================================================
-- Seed default role_permissions data
-- These 5 roles align with OIDC role mapping (superadmin, admin, host_manager, readonly, user).
-- Uses ON CONFLICT to preserve any existing customised permissions.
-- ============================================================================

INSERT INTO "role_permissions" (
    "id", "role",
    "can_view_dashboard", "can_view_hosts", "can_manage_hosts",
    "can_view_packages", "can_manage_packages",
    "can_view_users", "can_manage_users", "can_manage_superusers",
    "can_view_reports", "can_export_data", "can_manage_settings",
    "created_at", "updated_at"
)
VALUES
    (gen_random_uuid(), 'superadmin', true, true, true, true, true, true, true, true, true, true, true, NOW(), NOW()),
    (gen_random_uuid(), 'admin',      true, true, true, true, true, true, true, false, true, true, true, NOW(), NOW()),
    (gen_random_uuid(), 'host_manager', true, true, true, true, true, false, false, false, true, true, false, NOW(), NOW()),
    (gen_random_uuid(), 'readonly',   true, true, false, true, false, false, false, false, true, false, false, NOW(), NOW()),
    (gen_random_uuid(), 'user',       true, true, false, true, false, false, false, false, true, true, false, NOW(), NOW())
ON CONFLICT ("role") DO NOTHING;

-- ============================================================================
-- Seed default compliance_profiles data
-- These are the CIS benchmark profiles used by the compliance scanning system.
-- Uses ON CONFLICT to preserve any existing profiles.
-- ============================================================================

INSERT INTO "compliance_profiles" ("id", "name", "type", "os_family", "version", "description", "created_at", "updated_at")
VALUES
    (gen_random_uuid(), 'CIS Ubuntu 22.04 L1', 'openscap', 'ubuntu', '1.0.0', 'CIS Benchmark for Ubuntu 22.04 LTS - Level 1 Server', NOW(), NOW()),
    (gen_random_uuid(), 'CIS Ubuntu 22.04 L2', 'openscap', 'ubuntu', '1.0.0', 'CIS Benchmark for Ubuntu 22.04 LTS - Level 2 Server', NOW(), NOW()),
    (gen_random_uuid(), 'CIS Ubuntu 20.04 L1', 'openscap', 'ubuntu', '1.1.0', 'CIS Benchmark for Ubuntu 20.04 LTS - Level 1 Server', NOW(), NOW()),
    (gen_random_uuid(), 'CIS RHEL 8 L1', 'openscap', 'rhel', '2.0.0', 'CIS Benchmark for Red Hat Enterprise Linux 8 - Level 1 Server', NOW(), NOW()),
    (gen_random_uuid(), 'CIS RHEL 8 L2', 'openscap', 'rhel', '2.0.0', 'CIS Benchmark for Red Hat Enterprise Linux 8 - Level 2 Server', NOW(), NOW()),
    (gen_random_uuid(), 'CIS Debian 11 L1', 'openscap', 'debian', '1.0.0', 'CIS Benchmark for Debian 11 - Level 1 Server', NOW(), NOW()),
    (gen_random_uuid(), 'CIS Docker', 'docker-bench', NULL, '1.5.0', 'CIS Docker Benchmark v1.5.0', NOW(), NOW())
ON CONFLICT ("name") DO NOTHING;

