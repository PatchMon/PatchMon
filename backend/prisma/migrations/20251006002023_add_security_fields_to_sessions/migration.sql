-- Add security fields to user_sessions table for production-ready remember me
ALTER TABLE "user_sessions" ADD COLUMN "device_fingerprint" TEXT;
ALTER TABLE "user_sessions" ADD COLUMN "login_count" INTEGER NOT NULL DEFAULT 1;
ALTER TABLE "user_sessions" ADD COLUMN "last_login_ip" TEXT;

-- Create index for device fingerprint for efficient querying
CREATE INDEX "user_sessions_device_fingerprint_idx" ON "user_sessions"("device_fingerprint");
