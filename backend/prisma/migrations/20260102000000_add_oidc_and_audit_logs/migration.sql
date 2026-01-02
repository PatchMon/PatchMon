-- Add OIDC fields to users table
ALTER TABLE "users" ADD COLUMN IF NOT EXISTS "oidc_sub" TEXT;
ALTER TABLE "users" ADD COLUMN IF NOT EXISTS "oidc_provider" TEXT;

-- Make password_hash nullable for OIDC-only users
ALTER TABLE "users" ALTER COLUMN "password_hash" DROP NOT NULL;

-- Create unique index on oidc_sub
CREATE UNIQUE INDEX IF NOT EXISTS "users_oidc_sub_key" ON "users"("oidc_sub");

-- Create audit_logs table
CREATE TABLE IF NOT EXISTS "audit_logs" (
    "id" TEXT NOT NULL,
    "event" TEXT NOT NULL,
    "user_id" TEXT,
    "target_user_id" TEXT,
    "ip_address" TEXT,
    "user_agent" TEXT,
    "request_id" TEXT,
    "details" JSONB,
    "success" BOOLEAN NOT NULL DEFAULT true,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "audit_logs_pkey" PRIMARY KEY ("id")
);

-- Create indexes for audit_logs
CREATE INDEX IF NOT EXISTS "audit_logs_event_idx" ON "audit_logs"("event");
CREATE INDEX IF NOT EXISTS "audit_logs_user_id_idx" ON "audit_logs"("user_id");
CREATE INDEX IF NOT EXISTS "audit_logs_created_at_idx" ON "audit_logs"("created_at");
