-- Add labels column to docker_containers for storing container labels (including compose project)
ALTER TABLE "docker_containers" ADD COLUMN "labels" JSONB;
