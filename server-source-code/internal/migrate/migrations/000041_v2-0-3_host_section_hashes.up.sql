-- 000041: per-section content hashes for hash-gated agent check-in.
--
-- The agent ships SHA-256 hashes of canonical packages/repos/interfaces/
-- hostname/docker/compliance content on every ping. Server compares each
-- against the stored value and asks the agent for full content only when
-- a hash differs (or is NULL). This collapses the steady-state hourly
-- check-in from a ~2 MB POST to a ~1 KB ping + ~200 B response.
--
-- Hash columns are 64-char lowercase hex (sha256). NULL means "no stored
-- hash, force the agent to send a full report so we can populate it" —
-- this is also the natural state for hosts that existed before this
-- migration ran. last_full_report_at is informational, used by the
-- operator UI / staleness alerts.

ALTER TABLE hosts
    ADD COLUMN IF NOT EXISTS packages_hash       TEXT,
    ADD COLUMN IF NOT EXISTS repos_hash          TEXT,
    ADD COLUMN IF NOT EXISTS interfaces_hash     TEXT,
    ADD COLUMN IF NOT EXISTS hostname_hash       TEXT,
    ADD COLUMN IF NOT EXISTS docker_hash         TEXT,
    ADD COLUMN IF NOT EXISTS compliance_hash     TEXT,
    ADD COLUMN IF NOT EXISTS last_full_report_at TIMESTAMP(3);

-- Length checks. Idempotent guard so the migration is safe to rerun on a
-- partially-applied database.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'hosts_packages_hash_len') THEN
        ALTER TABLE hosts ADD CONSTRAINT hosts_packages_hash_len
            CHECK (packages_hash IS NULL OR length(packages_hash) = 64);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'hosts_repos_hash_len') THEN
        ALTER TABLE hosts ADD CONSTRAINT hosts_repos_hash_len
            CHECK (repos_hash IS NULL OR length(repos_hash) = 64);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'hosts_interfaces_hash_len') THEN
        ALTER TABLE hosts ADD CONSTRAINT hosts_interfaces_hash_len
            CHECK (interfaces_hash IS NULL OR length(interfaces_hash) = 64);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'hosts_hostname_hash_len') THEN
        ALTER TABLE hosts ADD CONSTRAINT hosts_hostname_hash_len
            CHECK (hostname_hash IS NULL OR length(hostname_hash) = 64);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'hosts_docker_hash_len') THEN
        ALTER TABLE hosts ADD CONSTRAINT hosts_docker_hash_len
            CHECK (docker_hash IS NULL OR length(docker_hash) = 64);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'hosts_compliance_hash_len') THEN
        ALTER TABLE hosts ADD CONSTRAINT hosts_compliance_hash_len
            CHECK (compliance_hash IS NULL OR length(compliance_hash) = 64);
    END IF;
END $$;
