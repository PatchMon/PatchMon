-- Remove the newly seeded event type configs (only if they haven't been customized).
DELETE FROM alert_config WHERE alert_type IN ('host_recovered', 'patch_run_completed', 'patch_run_failed', 'compliance_scan_completed');
