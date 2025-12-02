-- Add docker_enabled field to hosts table
-- This field persists the Docker integration enabled state across container restarts
-- Fixes GitHub issue #352

ALTER TABLE "hosts" ADD COLUMN "docker_enabled" BOOLEAN NOT NULL DEFAULT false;
