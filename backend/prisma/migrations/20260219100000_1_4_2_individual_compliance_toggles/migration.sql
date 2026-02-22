-- Add individual scanner enable/disable toggles for compliance scanning.
-- OpenSCAP defaults to enabled (true) when compliance is on.
-- Docker Bench defaults to disabled (false) since not all hosts have Docker.
-- Existing data is preserved; new columns have safe defaults.

ALTER TABLE "hosts" ADD COLUMN IF NOT EXISTS "compliance_openscap_enabled" BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE "hosts" ADD COLUMN IF NOT EXISTS "compliance_docker_bench_enabled" BOOLEAN NOT NULL DEFAULT false;
