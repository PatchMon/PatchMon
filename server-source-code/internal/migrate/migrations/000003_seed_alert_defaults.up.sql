-- Seed default alert_actions and alert_config (from legacy Node app v1.4.0)
-- Uses ON CONFLICT to preserve existing data on Prisma-migrated or already-seeded DBs.
-- Fresh installs get defaults; existing installs keep user customizations.

-- =============================================================================
-- Seed default alert_actions
-- These are the action types used by the Reporting/Alerts system (acknowledge,
-- assign, resolve, etc.). The Reporting UI fetches these for the action dropdown.
-- =============================================================================

INSERT INTO alert_actions (id, name, display_name, description, is_state_action, severity_override, created_at, updated_at)
VALUES
    (gen_random_uuid()::TEXT, 'created', 'Created', 'Alert was created (system action)', false, NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (gen_random_uuid()::TEXT, 'acknowledged', 'Acknowledged', 'User acknowledged the alert', false, NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (gen_random_uuid()::TEXT, 'assigned', 'Assigned', 'Alert was assigned/delegated to a user', true, NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (gen_random_uuid()::TEXT, 'unassigned', 'Unassigned', 'Alert assignment was removed', true, NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (gen_random_uuid()::TEXT, 'silenced', 'Silenced', 'Alert was silenced', true, NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (gen_random_uuid()::TEXT, 'unsilenced', 'Unsilenced', 'Alert was unsilenced', true, NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (gen_random_uuid()::TEXT, 'done', 'Mark as Done', 'Alert was marked as done', true, NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (gen_random_uuid()::TEXT, 'resolved', 'Resolved', 'Alert was resolved', true, NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (gen_random_uuid()::TEXT, 'updated', 'Updated', 'Alert was updated (system action)', false, NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT (name) DO NOTHING;

-- =============================================================================
-- Seed default alert_config
-- These are the alert type configurations shown in Alert Settings. Without these,
-- host_down, server_update, and agent_update alerts will not fire (GetConfigForType
-- returns nil and the alert logic exits early).
-- =============================================================================

INSERT INTO alert_config (id, alert_type, is_enabled, default_severity, auto_assign_enabled, notification_enabled, cleanup_resolved_only, created_at, updated_at)
VALUES
    (gen_random_uuid()::TEXT, 'host_down', true, 'warning', false, true, true, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (gen_random_uuid()::TEXT, 'server_update', true, 'informational', false, true, true, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (gen_random_uuid()::TEXT, 'agent_update', true, 'informational', false, true, true, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT (alert_type) DO NOTHING;
