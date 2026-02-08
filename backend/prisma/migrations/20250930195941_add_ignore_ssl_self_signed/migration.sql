-- Add ignore_ssl_self_signed column to settings table
-- This allows users to configure whether curl commands should ignore SSL certificate validation

ALTER TABLE "settings" ADD COLUMN "ignore_ssl_self_signed" BOOLEAN NOT NULL DEFAULT false;
