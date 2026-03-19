-- Revert: restore original is_state_action flags (matches 000003 seed).
UPDATE alert_actions SET is_state_action = true WHERE name IN ('assigned', 'unassigned', 'silenced', 'unsilenced');
DELETE FROM alert_actions WHERE name IN ('investigating', 'escalated');
