ALTER TABLE hosts ADD COLUMN IF NOT EXISTS config_version TEXT;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS reported_host_id TEXT;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS enrolment_token_id TEXT REFERENCES auto_enrollment_tokens(id) ON DELETE SET NULL;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS credentials_reissued_at TIMESTAMP(3);
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS identity_fingerprint TEXT;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS identity_prev_fingerprint TEXT;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS identity_fingerprint_changed_at TIMESTAMP(3);
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS identity_conflict_at TIMESTAMP(3);
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS enrolment_machine_id TEXT;

CREATE INDEX IF NOT EXISTS idx_hosts_enrolment_token_id ON hosts (enrolment_token_id) WHERE enrolment_token_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_hosts_enrolment_machine_id_norm ON hosts (lower(replace(enrolment_machine_id, '-', ''))) WHERE enrolment_machine_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_hosts_machine_id_norm ON hosts (lower(replace(machine_id, '-', '')));
