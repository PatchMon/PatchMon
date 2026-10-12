-- Not unique: installs that pre-date the enrol duplicate guard may already hold repeated machine_ids.
CREATE INDEX IF NOT EXISTS idx_hosts_machine_id ON hosts (machine_id);
