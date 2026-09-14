ALTER TABLE settings
    DROP COLUMN IF EXISTS compliance_body_limit;

ALTER TABLE settings
    DROP COLUMN IF EXISTS agent_ping_body_limit;
