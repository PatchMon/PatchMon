DELETE FROM alert_config WHERE alert_type IN (
    'container_stopped', 'container_started', 'container_image_update_available',
    'patch_run_started', 'patch_run_approved', 'patch_run_cancelled', 'patch_reboot_required',
    'host_enrolled', 'host_deleted',
    'compliance_scan_failed',
    'ssh_session_started', 'rdp_session_started',
    'user_login', 'user_login_failed', 'account_locked',
    'user_created', 'user_role_changed', 'user_tfa_disabled'
);
