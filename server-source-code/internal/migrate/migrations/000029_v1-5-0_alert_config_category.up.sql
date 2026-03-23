ALTER TABLE alert_config ADD COLUMN IF NOT EXISTS category TEXT NOT NULL DEFAULT 'general';

-- Categorize existing alert types
UPDATE alert_config SET category = 'host' WHERE alert_type IN ('host_down', 'host_recovered', 'host_enrolled', 'host_deleted', 'host_security_updates_exceeded', 'host_pending_updates_exceeded', 'host_security_updates_resolved', 'host_pending_updates_resolved');
UPDATE alert_config SET category = 'patching' WHERE alert_type IN ('patch_run_started', 'patch_run_completed', 'patch_run_failed', 'patch_run_approved', 'patch_run_cancelled', 'patch_reboot_required');
UPDATE alert_config SET category = 'docker' WHERE alert_type IN ('container_stopped', 'container_started', 'container_image_update_available');
UPDATE alert_config SET category = 'compliance' WHERE alert_type IN ('compliance_scan_completed', 'compliance_scan_failed');
UPDATE alert_config SET category = 'security' WHERE alert_type IN ('user_login', 'user_login_failed', 'account_locked', 'user_created', 'user_role_changed', 'user_tfa_disabled');
UPDATE alert_config SET category = 'remote_access' WHERE alert_type IN ('ssh_session_started', 'rdp_session_started');
UPDATE alert_config SET category = 'system' WHERE alert_type IN ('server_update', 'agent_update');
