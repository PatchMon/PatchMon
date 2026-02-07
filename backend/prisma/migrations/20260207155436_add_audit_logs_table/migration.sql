-- CreateTable: Add audit_logs table
-- This migration is safe to run multiple times and preserves existing data
-- The table may be missing if the database was created before this table was added to the schema

-- Create audit_logs table if it doesn't exist
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

