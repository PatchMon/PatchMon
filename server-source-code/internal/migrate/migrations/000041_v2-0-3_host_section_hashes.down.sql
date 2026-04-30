-- Reverse 000041: drop hash columns and length constraints.

ALTER TABLE hosts
    DROP CONSTRAINT IF EXISTS hosts_packages_hash_len,
    DROP CONSTRAINT IF EXISTS hosts_repos_hash_len,
    DROP CONSTRAINT IF EXISTS hosts_interfaces_hash_len,
    DROP CONSTRAINT IF EXISTS hosts_hostname_hash_len,
    DROP CONSTRAINT IF EXISTS hosts_docker_hash_len,
    DROP CONSTRAINT IF EXISTS hosts_compliance_hash_len;

ALTER TABLE hosts
    DROP COLUMN IF EXISTS packages_hash,
    DROP COLUMN IF EXISTS repos_hash,
    DROP COLUMN IF EXISTS interfaces_hash,
    DROP COLUMN IF EXISTS hostname_hash,
    DROP COLUMN IF EXISTS docker_hash,
    DROP COLUMN IF EXISTS compliance_hash,
    DROP COLUMN IF EXISTS last_full_report_at;
