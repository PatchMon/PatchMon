-- Add TFA remember me fields to user_sessions table
ALTER TABLE "user_sessions" ADD COLUMN "tfa_remember_me" BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE "user_sessions" ADD COLUMN "tfa_bypass_until" TIMESTAMP(3);

-- Create index for TFA bypass until field for efficient querying
CREATE INDEX "user_sessions_tfa_bypass_until_idx" ON "user_sessions"("tfa_bypass_until");
