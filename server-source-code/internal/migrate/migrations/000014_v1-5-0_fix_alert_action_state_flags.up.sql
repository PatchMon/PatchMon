-- Fix is_state_action flags on alert_actions.
-- Only "done" and "resolved" should be state actions (they deactivate the alert).
-- assigned/unassigned/silenced/unsilenced are workflow transitions that must keep
-- the alert active; they were incorrectly seeded as is_state_action=true.
UPDATE alert_actions SET is_state_action = false WHERE name IN ('assigned', 'unassigned', 'silenced', 'unsilenced');

-- Add NOC workflow actions: escalated, investigating.
-- These are non-state actions (alert stays active).
INSERT INTO alert_actions (id, name, display_name, description, is_state_action, severity_override, created_at, updated_at)
VALUES
    (gen_random_uuid()::TEXT, 'investigating', 'Investigating', 'Alert is being investigated', false, NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (gen_random_uuid()::TEXT, 'escalated', 'Escalated', 'Alert has been escalated', false, 'critical', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT (name) DO NOTHING;
