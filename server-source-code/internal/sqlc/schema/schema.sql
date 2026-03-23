-- PatchMon database schema (mirrors Prisma schema)
-- Used by sqlc for type-safe query generation

-- role_permissions
CREATE TABLE IF NOT EXISTS role_permissions (
    id TEXT PRIMARY KEY,
    role TEXT NOT NULL UNIQUE,
    can_view_dashboard BOOLEAN NOT NULL DEFAULT true,
    can_view_hosts BOOLEAN NOT NULL DEFAULT true,
    can_manage_hosts BOOLEAN NOT NULL DEFAULT false,
    can_view_packages BOOLEAN NOT NULL DEFAULT true,
    can_manage_packages BOOLEAN NOT NULL DEFAULT false,
    can_view_users BOOLEAN NOT NULL DEFAULT false,
    can_manage_users BOOLEAN NOT NULL DEFAULT false,
    can_manage_superusers BOOLEAN NOT NULL DEFAULT false,
    can_view_reports BOOLEAN NOT NULL DEFAULT true,
    can_export_data BOOLEAN NOT NULL DEFAULT false,
    can_manage_settings BOOLEAN NOT NULL DEFAULT false,
    can_manage_notifications BOOLEAN NOT NULL DEFAULT false,
    can_view_notification_logs BOOLEAN NOT NULL DEFAULT false,
    can_manage_patching BOOLEAN NOT NULL DEFAULT false,
    can_manage_compliance BOOLEAN NOT NULL DEFAULT false,
    can_manage_docker BOOLEAN NOT NULL DEFAULT false,
    can_manage_alerts BOOLEAN NOT NULL DEFAULT false,
    can_manage_automation BOOLEAN NOT NULL DEFAULT false,
    can_use_remote_access BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP(3) NOT NULL
);

-- settings
CREATE TABLE IF NOT EXISTS settings (
    id TEXT PRIMARY KEY,
    server_url TEXT NOT NULL DEFAULT 'http://localhost:3001',
    server_protocol TEXT NOT NULL DEFAULT 'http',
    server_host TEXT NOT NULL DEFAULT 'localhost',
    server_port INTEGER NOT NULL DEFAULT 3001,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP(3) NOT NULL,
    update_interval INTEGER NOT NULL DEFAULT 60,
    auto_update BOOLEAN NOT NULL DEFAULT false,
    default_compliance_mode TEXT NOT NULL DEFAULT 'on-demand',
    compliance_scan_interval INTEGER NOT NULL DEFAULT 1440,
    github_repo_url TEXT NOT NULL DEFAULT 'https://github.com/PatchMon/PatchMon.git',
    ssh_key_path TEXT,
    repository_type TEXT NOT NULL DEFAULT 'public',
    last_update_check TIMESTAMP(3),
    latest_version TEXT,
    update_available BOOLEAN NOT NULL DEFAULT false,
    signup_enabled BOOLEAN NOT NULL DEFAULT false,
    default_user_role TEXT NOT NULL DEFAULT 'user',
    ignore_ssl_self_signed BOOLEAN NOT NULL DEFAULT false,
    logo_dark TEXT DEFAULT NULL,
    logo_light TEXT DEFAULT NULL,
    favicon TEXT DEFAULT NULL,
    logo_dark_data BYTEA,
    logo_light_data BYTEA,
    favicon_data BYTEA,
    logo_dark_content_type TEXT,
    logo_light_content_type TEXT,
    favicon_content_type TEXT,
    metrics_enabled BOOLEAN NOT NULL DEFAULT true,
    metrics_anonymous_id TEXT,
    metrics_last_sent TIMESTAMP(3),
    show_github_version_on_login BOOLEAN NOT NULL DEFAULT true,
    ai_enabled BOOLEAN NOT NULL DEFAULT false,
    ai_provider TEXT NOT NULL DEFAULT 'openrouter',
    ai_model TEXT,
    ai_api_key TEXT,
    alerts_enabled BOOLEAN NOT NULL DEFAULT true,
    discord_oauth_enabled BOOLEAN NOT NULL DEFAULT false,
    discord_client_id TEXT,
    discord_client_secret TEXT,
    discord_redirect_uri TEXT,
    discord_button_text TEXT DEFAULT 'Login with Discord',
    oidc_enabled BOOLEAN NOT NULL DEFAULT false,
    oidc_issuer_url TEXT,
    oidc_client_id TEXT,
    oidc_client_secret TEXT,
    oidc_redirect_uri TEXT,
    oidc_scopes TEXT,
    oidc_auto_create_users BOOLEAN NOT NULL DEFAULT false,
    oidc_default_role TEXT DEFAULT 'user',
    oidc_disable_local_auth BOOLEAN NOT NULL DEFAULT false,
    oidc_button_text TEXT DEFAULT 'Login with SSO',
    oidc_sync_roles BOOLEAN NOT NULL DEFAULT false,
    oidc_admin_group TEXT,
    oidc_superadmin_group TEXT,
    oidc_host_manager_group TEXT,
    oidc_readonly_group TEXT,
    oidc_user_group TEXT,
    oidc_enforce_https BOOLEAN NOT NULL DEFAULT true,
    max_login_attempts INTEGER,
    lockout_duration_minutes INTEGER,
    session_inactivity_timeout_minutes INTEGER,
    tfa_max_remember_sessions INTEGER,
    password_min_length INTEGER,
    password_require_uppercase BOOLEAN,
    password_require_lowercase BOOLEAN,
    password_require_number BOOLEAN,
    password_require_special BOOLEAN,
    enable_hsts BOOLEAN,
    json_body_limit TEXT,
    agent_update_body_limit TEXT,
    db_transaction_long_timeout INTEGER,
    cors_origin TEXT,
    enable_logging BOOLEAN,
    log_level TEXT,
    timezone TEXT,
    jwt_expires_in TEXT,
    max_tfa_attempts INTEGER,
    tfa_lockout_duration_minutes INTEGER,
    tfa_remember_me_expires_in TEXT,
    trust_proxy BOOLEAN,
    rate_limit_window_ms INTEGER,
    rate_limit_max INTEGER,
    auth_rate_limit_window_ms INTEGER,
    auth_rate_limit_max INTEGER,
    agent_rate_limit_window_ms INTEGER,
    agent_rate_limit_max INTEGER,
    password_rate_limit_window_ms INTEGER,
    password_rate_limit_max INTEGER,
    auth_browser_session_cookies BOOLEAN
);

-- host_groups
CREATE TABLE IF NOT EXISTS host_groups (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    color TEXT DEFAULT '#3B82F6',
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP(3) NOT NULL
);

-- packages
CREATE TABLE IF NOT EXISTS packages (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    category TEXT,
    latest_version TEXT,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP(3) NOT NULL
);

-- repositories
CREATE TABLE IF NOT EXISTS repositories (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    url TEXT NOT NULL,
    distribution TEXT NOT NULL,
    components TEXT NOT NULL,
    repo_type TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    is_secure BOOLEAN NOT NULL DEFAULT true,
    priority INTEGER,
    description TEXT,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP(3) NOT NULL
);

-- users
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT,
    role TEXT NOT NULL DEFAULT 'admin',
    is_active BOOLEAN NOT NULL DEFAULT true,
    last_login TIMESTAMP(3),
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP(3) NOT NULL,
    tfa_backup_codes TEXT,
    tfa_enabled BOOLEAN NOT NULL DEFAULT false,
    tfa_secret TEXT,
    first_name TEXT,
    last_name TEXT,
    theme_preference TEXT DEFAULT 'dark',
    color_theme TEXT DEFAULT 'cyber_blue',
    ui_preferences JSONB,
    oidc_sub TEXT UNIQUE,
    oidc_provider TEXT,
    avatar_url TEXT,
    discord_id TEXT UNIQUE,
    discord_username TEXT,
    discord_avatar TEXT,
    discord_linked_at TIMESTAMP(3)
);

-- hosts
CREATE TABLE IF NOT EXISTS hosts (
    id TEXT PRIMARY KEY,
    machine_id TEXT,
    friendly_name TEXT NOT NULL,
    ip TEXT,
    os_type TEXT NOT NULL,
    os_version TEXT NOT NULL,
    architecture TEXT,
    last_update TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP(3) NOT NULL,
    api_id TEXT NOT NULL UNIQUE,
    api_key TEXT NOT NULL UNIQUE,
    agent_version TEXT,
    auto_update BOOLEAN NOT NULL DEFAULT true,
    cpu_cores INTEGER,
    cpu_model TEXT,
    disk_details JSONB,
    dns_servers JSONB,
    gateway_ip TEXT,
    hostname TEXT,
    kernel_version TEXT,
    installed_kernel_version TEXT,
    load_average JSONB,
    network_interfaces JSONB,
    ram_installed DOUBLE PRECISION,
    selinux_status TEXT,
    swap_size DOUBLE PRECISION,
    system_uptime TEXT,
    notes TEXT,
    needs_reboot BOOLEAN DEFAULT false,
    reboot_reason TEXT,
    docker_enabled BOOLEAN NOT NULL DEFAULT false,
    compliance_enabled BOOLEAN NOT NULL DEFAULT false,
    compliance_on_demand_only BOOLEAN NOT NULL DEFAULT true,
    compliance_openscap_enabled BOOLEAN NOT NULL DEFAULT true,
    compliance_docker_bench_enabled BOOLEAN NOT NULL DEFAULT false,
    compliance_scanner_status JSONB,
    compliance_scanner_updated_at TIMESTAMP(3),
    compliance_default_profile_id TEXT,
    host_down_alerts_enabled BOOLEAN,
    expected_platform TEXT,
    package_manager TEXT,
    primary_interface TEXT
);

-- dashboard_preferences
CREATE TABLE IF NOT EXISTS dashboard_preferences (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    card_id TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    "order" INTEGER NOT NULL DEFAULT 0,
    col_span INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP(3) NOT NULL,
    UNIQUE(user_id, card_id)
);

-- dashboard_layout
CREATE TABLE IF NOT EXISTS dashboard_layout (
    user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    stats_columns INTEGER NOT NULL DEFAULT 5,
    charts_columns INTEGER NOT NULL DEFAULT 3,
    updated_at TIMESTAMP(3) NOT NULL
);

-- host_pending_config
CREATE TABLE IF NOT EXISTS host_pending_config (
    host_id TEXT PRIMARY KEY REFERENCES hosts(id) ON DELETE CASCADE,
    docker_enabled BOOLEAN,
    compliance_enabled BOOLEAN,
    compliance_on_demand_only BOOLEAN,
    compliance_openscap_enabled BOOLEAN,
    compliance_docker_bench_enabled BOOLEAN,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- host_group_memberships
CREATE TABLE IF NOT EXISTS host_group_memberships (
    id TEXT PRIMARY KEY,
    host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    host_group_id TEXT NOT NULL REFERENCES host_groups(id) ON DELETE CASCADE,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(host_id, host_group_id)
);

-- host_packages
CREATE TABLE IF NOT EXISTS host_packages (
    id TEXT PRIMARY KEY,
    host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    package_id TEXT NOT NULL REFERENCES packages(id) ON DELETE CASCADE,
    current_version TEXT NOT NULL,
    available_version TEXT,
    needs_update BOOLEAN NOT NULL DEFAULT false,
    is_security_update BOOLEAN NOT NULL DEFAULT false,
    last_checked TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- WUA (Windows Update Agent) metadata — populated for Category='Windows Update' entries only
    wua_guid            TEXT,
    wua_kb              TEXT,
    wua_severity        TEXT,
    wua_categories      JSONB,
    wua_description     TEXT,
    wua_support_url     TEXT,
    wua_revision_number INTEGER,
    wua_date_installed  TIMESTAMP(3),
    wua_install_result  TEXT,
    UNIQUE(host_id, package_id)
);

-- host_repositories
CREATE TABLE IF NOT EXISTS host_repositories (
    id TEXT PRIMARY KEY,
    host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    repository_id TEXT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    is_enabled BOOLEAN NOT NULL DEFAULT true,
    last_checked TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(host_id, repository_id)
);

-- update_history
CREATE TABLE IF NOT EXISTS update_history (
    id TEXT PRIMARY KEY,
    host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    packages_count INTEGER NOT NULL,
    security_count INTEGER NOT NULL,
    total_packages INTEGER,
    payload_size_kb DOUBLE PRECISION,
    execution_time DOUBLE PRECISION,
    timestamp TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status TEXT NOT NULL DEFAULT 'success',
    error_message TEXT
);

-- system_statistics
CREATE TABLE IF NOT EXISTS system_statistics (
    id TEXT PRIMARY KEY,
    unique_packages_count INTEGER NOT NULL,
    unique_security_count INTEGER NOT NULL,
    total_packages INTEGER NOT NULL,
    total_hosts INTEGER NOT NULL,
    hosts_needing_updates INTEGER NOT NULL,
    timestamp TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- user_sessions
CREATE TABLE IF NOT EXISTS user_sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    refresh_token TEXT NOT NULL UNIQUE,
    access_token_hash TEXT,
    ip_address TEXT,
    user_agent TEXT,
    device_fingerprint TEXT,
    device_id TEXT,
    last_activity TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP(3) NOT NULL,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    is_revoked BOOLEAN NOT NULL DEFAULT false,
    tfa_remember_me BOOLEAN NOT NULL DEFAULT false,
    tfa_bypass_until TIMESTAMP(3),
    login_count INTEGER NOT NULL DEFAULT 1,
    last_login_ip TEXT
);

-- auto_enrollment_tokens
CREATE TABLE IF NOT EXISTS auto_enrollment_tokens (
    id TEXT PRIMARY KEY,
    token_name TEXT NOT NULL,
    token_key TEXT NOT NULL UNIQUE,
    token_secret TEXT NOT NULL,
    created_by_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    allowed_ip_ranges TEXT[] NOT NULL,
    max_hosts_per_day INTEGER NOT NULL DEFAULT 100,
    hosts_created_today INTEGER NOT NULL DEFAULT 0,
    last_reset_date DATE NOT NULL DEFAULT CURRENT_DATE,
    default_host_group_id TEXT REFERENCES host_groups(id) ON DELETE SET NULL,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP(3) NOT NULL,
    last_used_at TIMESTAMP(3),
    expires_at TIMESTAMP(3),
    metadata JSONB,
    scopes JSONB
);

-- docker_images
CREATE TABLE IF NOT EXISTS docker_images (
    id TEXT PRIMARY KEY,
    repository TEXT NOT NULL,
    tag TEXT NOT NULL DEFAULT 'latest',
    image_id TEXT NOT NULL,
    digest TEXT,
    size_bytes BIGINT,
    source TEXT NOT NULL DEFAULT 'docker-hub',
    created_at TIMESTAMP(3) NOT NULL,
    last_pulled TIMESTAMP(3),
    last_checked TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP(3) NOT NULL,
    UNIQUE(repository, tag, image_id)
);

-- docker_containers
CREATE TABLE IF NOT EXISTS docker_containers (
    id TEXT PRIMARY KEY,
    host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    container_id TEXT NOT NULL,
    name TEXT NOT NULL,
    image_id TEXT REFERENCES docker_images(id) ON DELETE SET NULL,
    image_name TEXT NOT NULL,
    image_tag TEXT NOT NULL DEFAULT 'latest',
    status TEXT NOT NULL,
    state TEXT,
    ports JSONB,
    labels JSONB,
    created_at TIMESTAMP(3) NOT NULL,
    started_at TIMESTAMP(3),
    updated_at TIMESTAMP(3) NOT NULL,
    last_checked TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(host_id, container_id)
);

-- docker_image_updates
CREATE TABLE IF NOT EXISTS docker_image_updates (
    id TEXT PRIMARY KEY,
    image_id TEXT NOT NULL REFERENCES docker_images(id) ON DELETE CASCADE,
    current_tag TEXT NOT NULL,
    available_tag TEXT NOT NULL,
    is_security_update BOOLEAN NOT NULL DEFAULT false,
    severity TEXT,
    changelog_url TEXT,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP(3) NOT NULL,
    UNIQUE(image_id, available_tag)
);

-- docker_volumes
CREATE TABLE IF NOT EXISTS docker_volumes (
    id TEXT PRIMARY KEY,
    host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    volume_id TEXT NOT NULL,
    name TEXT NOT NULL,
    driver TEXT NOT NULL,
    mountpoint TEXT,
    renderer TEXT,
    scope TEXT NOT NULL DEFAULT 'local',
    labels JSONB,
    options JSONB,
    size_bytes BIGINT,
    ref_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP(3) NOT NULL,
    updated_at TIMESTAMP(3) NOT NULL,
    last_checked TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(host_id, volume_id)
);

-- docker_networks
CREATE TABLE IF NOT EXISTS docker_networks (
    id TEXT PRIMARY KEY,
    host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    network_id TEXT NOT NULL,
    name TEXT NOT NULL,
    driver TEXT NOT NULL,
    scope TEXT NOT NULL DEFAULT 'local',
    ipv6_enabled BOOLEAN NOT NULL DEFAULT false,
    internal BOOLEAN NOT NULL DEFAULT false,
    attachable BOOLEAN NOT NULL DEFAULT true,
    ingress BOOLEAN NOT NULL DEFAULT false,
    config_only BOOLEAN NOT NULL DEFAULT false,
    labels JSONB,
    ipam JSONB,
    container_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP(3),
    updated_at TIMESTAMP(3) NOT NULL,
    last_checked TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(host_id, network_id)
);

-- job_history
CREATE TABLE IF NOT EXISTS job_history (
    id TEXT PRIMARY KEY,
    job_id TEXT NOT NULL,
    queue_name TEXT NOT NULL,
    job_name TEXT NOT NULL,
    host_id TEXT REFERENCES hosts(id) ON DELETE SET NULL,
    api_id TEXT,
    status TEXT NOT NULL,
    attempt_number INTEGER NOT NULL DEFAULT 1,
    error_message TEXT,
    output JSONB,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP(3) NOT NULL,
    completed_at TIMESTAMP(3)
);

-- release_notes_acceptances
CREATE TABLE IF NOT EXISTS release_notes_acceptances (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    version TEXT NOT NULL,
    accepted_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, version)
);

-- audit_logs
CREATE TABLE IF NOT EXISTS audit_logs (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    event TEXT NOT NULL,
    user_id TEXT,
    target_user_id TEXT,
    ip_address TEXT,
    user_agent TEXT,
    request_id TEXT,
    details TEXT,
    success BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- compliance_profiles
CREATE TABLE IF NOT EXISTS compliance_profiles (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    name TEXT NOT NULL UNIQUE,
    type TEXT NOT NULL,
    os_family TEXT,
    version TEXT,
    description TEXT,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP(3) NOT NULL
);

-- compliance_scans
CREATE TABLE IF NOT EXISTS compliance_scans (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    profile_id TEXT NOT NULL REFERENCES compliance_profiles(id) ON DELETE CASCADE,
    started_at TIMESTAMP(3) NOT NULL,
    completed_at TIMESTAMP(3),
    status TEXT NOT NULL,
    total_rules INTEGER NOT NULL DEFAULT 0,
    passed INTEGER NOT NULL DEFAULT 0,
    failed INTEGER NOT NULL DEFAULT 0,
    warnings INTEGER NOT NULL DEFAULT 0,
    skipped INTEGER NOT NULL DEFAULT 0,
    not_applicable INTEGER NOT NULL DEFAULT 0,
    score DOUBLE PRECISION,
    error_message TEXT,
    raw_output TEXT,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP(3) NOT NULL
);

-- Only one completed scan per host+profile at a time (newer replaces older)
CREATE UNIQUE INDEX IF NOT EXISTS idx_compliance_scans_host_profile_completed
ON compliance_scans (host_id, profile_id)
WHERE status = 'completed';

-- compliance_rules
CREATE TABLE IF NOT EXISTS compliance_rules (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    profile_id TEXT NOT NULL REFERENCES compliance_profiles(id) ON DELETE CASCADE,
    rule_ref TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT,
    rationale TEXT,
    severity TEXT,
    section TEXT,
    remediation TEXT,
    UNIQUE(profile_id, rule_ref)
);

-- compliance_results
CREATE TABLE IF NOT EXISTS compliance_results (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    scan_id TEXT NOT NULL REFERENCES compliance_scans(id) ON DELETE CASCADE,
    rule_id TEXT NOT NULL REFERENCES compliance_rules(id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    finding TEXT,
    actual TEXT,
    expected TEXT,
    remediation TEXT,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(scan_id, rule_id)
);

-- alerts
CREATE TABLE IF NOT EXISTS alerts (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    type TEXT NOT NULL,
    severity TEXT NOT NULL,
    title TEXT NOT NULL,
    message TEXT NOT NULL,
    metadata JSONB,
    is_active BOOLEAN NOT NULL DEFAULT true,
    assigned_to_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    resolved_at TIMESTAMP(3),
    resolved_by_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP(3) NOT NULL
);

-- alert_history
CREATE TABLE IF NOT EXISTS alert_history (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    alert_id TEXT NOT NULL REFERENCES alerts(id) ON DELETE CASCADE,
    user_id TEXT REFERENCES users(id) ON DELETE CASCADE,
    action TEXT NOT NULL,
    metadata JSONB,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- alert_actions
CREATE TABLE IF NOT EXISTS alert_actions (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    name TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    description TEXT,
    is_state_action BOOLEAN NOT NULL DEFAULT false,
    severity_override TEXT,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP(3) NOT NULL
);

-- patch_policies
CREATE TABLE IF NOT EXISTS patch_policies (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    patch_delay_type TEXT NOT NULL,
    delay_minutes INTEGER,
    fixed_time_utc TEXT,
    timezone TEXT,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP(3) NOT NULL
);

-- patch_policy_assignments
CREATE TABLE IF NOT EXISTS patch_policy_assignments (
    id TEXT PRIMARY KEY,
    patch_policy_id TEXT NOT NULL REFERENCES patch_policies(id) ON DELETE CASCADE,
    target_type TEXT NOT NULL,
    target_id TEXT NOT NULL,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP(3) NOT NULL
);

-- patch_policy_exclusions
CREATE TABLE IF NOT EXISTS patch_policy_exclusions (
    id TEXT PRIMARY KEY,
    patch_policy_id TEXT NOT NULL REFERENCES patch_policies(id) ON DELETE CASCADE,
    host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP(3) NOT NULL
);

-- patch_runs
CREATE TABLE IF NOT EXISTS patch_runs (
    id TEXT PRIMARY KEY,
    host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    job_id TEXT NOT NULL,
    patch_type TEXT NOT NULL,
    package_name TEXT,
    package_names JSONB,
    status TEXT NOT NULL,
    shell_output TEXT NOT NULL DEFAULT '',
    error_message TEXT,
    started_at TIMESTAMP(3),
    completed_at TIMESTAMP(3),
    scheduled_at TIMESTAMP(3),
    triggered_by_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    approved_by_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    dry_run BOOLEAN NOT NULL DEFAULT false,
    packages_affected JSONB,
    policy_id TEXT REFERENCES patch_policies(id) ON DELETE SET NULL,
    policy_name TEXT,
    policy_snapshot JSONB,
    validation_run_id TEXT REFERENCES patch_runs(id) ON DELETE SET NULL,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP(3) NOT NULL
);

-- alert_config
CREATE TABLE IF NOT EXISTS alert_config (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    alert_type TEXT NOT NULL UNIQUE,
    is_enabled BOOLEAN NOT NULL DEFAULT true,
    default_severity TEXT NOT NULL DEFAULT 'informational',
    auto_assign_enabled BOOLEAN NOT NULL DEFAULT false,
    auto_assign_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    auto_assign_rule TEXT,
    auto_assign_conditions JSONB,
    retention_days INTEGER,
    auto_resolve_after_days INTEGER,
    cleanup_resolved_only BOOLEAN NOT NULL DEFAULT true,
    notification_enabled BOOLEAN NOT NULL DEFAULT true,
    escalation_enabled BOOLEAN NOT NULL DEFAULT false,
    escalation_after_hours INTEGER,
    alert_delay_seconds INTEGER,
    metadata JSONB,
    category TEXT NOT NULL DEFAULT 'general',
    check_interval_minutes INTEGER,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP(3) NOT NULL
);

-- notification_destinations
CREATE TABLE IF NOT EXISTS notification_destinations (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    channel_type TEXT NOT NULL,
    display_name TEXT NOT NULL,
    config_encrypted TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- notification_routes
CREATE TABLE IF NOT EXISTS notification_routes (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    destination_id TEXT NOT NULL REFERENCES notification_destinations(id) ON DELETE CASCADE,
    event_types JSONB NOT NULL DEFAULT '["*"]'::jsonb,
    min_severity TEXT NOT NULL DEFAULT 'informational',
    host_group_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    host_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    match_rules JSONB,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- notification_delivery_log
CREATE TABLE IF NOT EXISTS notification_delivery_log (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    event_fingerprint TEXT NOT NULL,
    reference_type TEXT NOT NULL DEFAULT '',
    reference_id TEXT NOT NULL DEFAULT '',
    destination_id TEXT NOT NULL REFERENCES notification_destinations(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    status TEXT NOT NULL,
    error_message TEXT,
    attempt_count INTEGER NOT NULL DEFAULT 1,
    provider_message_id TEXT,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- scheduled_reports
CREATE TABLE IF NOT EXISTS scheduled_reports (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    name TEXT NOT NULL,
    cron_expr TEXT NOT NULL DEFAULT '0 8 * * *',
    enabled BOOLEAN NOT NULL DEFAULT true,
    definition JSONB NOT NULL DEFAULT '{}'::jsonb,
    destination_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    next_run_at TIMESTAMP(3),
    last_run_at TIMESTAMP(3),
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- scheduled_report_runs
CREATE TABLE IF NOT EXISTS scheduled_report_runs (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    scheduled_report_id TEXT NOT NULL REFERENCES scheduled_reports(id) ON DELETE CASCADE,
    run_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status TEXT NOT NULL,
    error_message TEXT,
    summary_hash TEXT,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP
);
