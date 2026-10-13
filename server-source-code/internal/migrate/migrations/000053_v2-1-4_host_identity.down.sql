DROP INDEX IF EXISTS idx_hosts_machine_id_norm;
DROP INDEX IF EXISTS idx_hosts_enrolment_machine_id_norm;
DROP INDEX IF EXISTS idx_hosts_enrolment_token_id;

ALTER TABLE hosts DROP COLUMN IF EXISTS enrolment_machine_id;
ALTER TABLE hosts DROP COLUMN IF EXISTS identity_conflict_at;
ALTER TABLE hosts DROP COLUMN IF EXISTS identity_fingerprint_changed_at;
ALTER TABLE hosts DROP COLUMN IF EXISTS identity_prev_fingerprint;
ALTER TABLE hosts DROP COLUMN IF EXISTS identity_fingerprint;
ALTER TABLE hosts DROP COLUMN IF EXISTS credentials_reissued_at;
ALTER TABLE hosts DROP COLUMN IF EXISTS enrolment_token_id;
ALTER TABLE hosts DROP COLUMN IF EXISTS reported_host_id;
ALTER TABLE hosts DROP COLUMN IF EXISTS config_version;
