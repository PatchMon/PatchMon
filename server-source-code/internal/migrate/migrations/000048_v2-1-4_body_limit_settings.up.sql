ALTER TABLE settings
    ADD COLUMN IF NOT EXISTS compliance_body_limit TEXT;

ALTER TABLE settings
    ADD COLUMN IF NOT EXISTS agent_ping_body_limit TEXT;
