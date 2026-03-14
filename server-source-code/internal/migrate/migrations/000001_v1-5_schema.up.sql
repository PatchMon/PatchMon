-- PatchMon database schema (golang-migrate)
-- Idempotent: uses CREATE TABLE IF NOT EXISTS for seamless migration from Prisma.
-- Existing Prisma databases: all tables exist, so IF NOT EXISTS = no-op.
-- Only host_pending_config is new (not in Prisma) and will be created.
-- Fresh installs: all tables are created.

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
    oidc_enforce_https BOOLEAN NOT NULL DEFAULT true
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
    host_down_alerts_enabled BOOLEAN,
    expected_platform TEXT
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

-- host_pending_config (Go-only: not in Prisma; added for pending config flow)
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
    metadata JSONB,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP(3) NOT NULL
);

-- =============================================================================
-- Legacy Prisma compatibility: ADD COLUMN IF NOT EXISTS for every column that
-- may be missing in databases created by earlier Prisma migrations.
-- Safe: no-op on fresh installs where CREATE TABLE already has all columns.
--
-- Migration strategy when applying on top of Prisma (PatchMon-Server-node_archive):
-- - CREATE TABLE IF NOT EXISTS: no-op for existing tables, creates host_pending_config (Go-only)
-- - ADD COLUMN IF NOT EXISTS: adds any column Prisma never added (OIDC in settings, env config, etc.)
-- - Patch tables (patch_policies, patch_runs, etc.) are created by 000002_patch_tables
-- - ALTER COLUMN/constraint changes use DO blocks to avoid errors when already applied
-- =============================================================================

-- settings: columns added across many Prisma migrations
ALTER TABLE settings ADD COLUMN IF NOT EXISTS update_interval INTEGER NOT NULL DEFAULT 60;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS auto_update BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS default_compliance_mode TEXT NOT NULL DEFAULT 'on-demand';
ALTER TABLE settings ADD COLUMN IF NOT EXISTS github_repo_url TEXT NOT NULL DEFAULT 'https://github.com/PatchMon/PatchMon.git';
ALTER TABLE settings ADD COLUMN IF NOT EXISTS ssh_key_path TEXT;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS repository_type TEXT NOT NULL DEFAULT 'public';
ALTER TABLE settings ADD COLUMN IF NOT EXISTS last_update_check TIMESTAMP(3);
ALTER TABLE settings ADD COLUMN IF NOT EXISTS latest_version TEXT;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS update_available BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS signup_enabled BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS default_user_role TEXT NOT NULL DEFAULT 'user';
ALTER TABLE settings ADD COLUMN IF NOT EXISTS ignore_ssl_self_signed BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS logo_dark TEXT DEFAULT NULL;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS logo_light TEXT DEFAULT NULL;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS favicon TEXT DEFAULT NULL;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS logo_dark_data BYTEA;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS logo_light_data BYTEA;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS favicon_data BYTEA;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS logo_dark_content_type TEXT;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS logo_light_content_type TEXT;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS favicon_content_type TEXT;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS metrics_enabled BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS metrics_anonymous_id TEXT;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS metrics_last_sent TIMESTAMP(3);
ALTER TABLE settings ADD COLUMN IF NOT EXISTS show_github_version_on_login BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS ai_enabled BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS ai_provider TEXT NOT NULL DEFAULT 'openrouter';
ALTER TABLE settings ADD COLUMN IF NOT EXISTS ai_model TEXT;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS ai_api_key TEXT;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS alerts_enabled BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS discord_oauth_enabled BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS discord_client_id TEXT;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS discord_client_secret TEXT;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS discord_redirect_uri TEXT;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS discord_button_text TEXT DEFAULT 'Login with Discord';
ALTER TABLE settings ADD COLUMN IF NOT EXISTS oidc_enabled BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS oidc_issuer_url TEXT;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS oidc_client_id TEXT;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS oidc_client_secret TEXT;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS oidc_redirect_uri TEXT;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS oidc_scopes TEXT;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS oidc_auto_create_users BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS oidc_default_role TEXT DEFAULT 'user';
ALTER TABLE settings ADD COLUMN IF NOT EXISTS oidc_disable_local_auth BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS oidc_button_text TEXT DEFAULT 'Login with SSO';
ALTER TABLE settings ADD COLUMN IF NOT EXISTS oidc_sync_roles BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS oidc_admin_group TEXT;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS oidc_superadmin_group TEXT;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS oidc_host_manager_group TEXT;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS oidc_readonly_group TEXT;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS oidc_user_group TEXT;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS max_login_attempts INTEGER;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS lockout_duration_minutes INTEGER;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS session_inactivity_timeout_minutes INTEGER;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS tfa_max_remember_sessions INTEGER;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS password_min_length INTEGER;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS password_require_uppercase BOOLEAN;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS password_require_lowercase BOOLEAN;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS password_require_number BOOLEAN;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS password_require_special BOOLEAN;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS enable_hsts BOOLEAN;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS json_body_limit TEXT;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS agent_update_body_limit TEXT;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS db_transaction_long_timeout INTEGER;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS cors_origin TEXT;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS enable_logging BOOLEAN;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS log_level TEXT;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS timezone TEXT;
ALTER TABLE settings ADD COLUMN IF NOT EXISTS oidc_enforce_https BOOLEAN NOT NULL DEFAULT true;

-- users: columns added across Prisma migrations
ALTER TABLE users ADD COLUMN IF NOT EXISTS tfa_backup_codes TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS tfa_enabled BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE users ADD COLUMN IF NOT EXISTS tfa_secret TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS first_name TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS last_name TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS theme_preference TEXT DEFAULT 'dark';
ALTER TABLE users ADD COLUMN IF NOT EXISTS color_theme TEXT DEFAULT 'cyber_blue';
ALTER TABLE users ADD COLUMN IF NOT EXISTS ui_preferences JSONB;
ALTER TABLE users ADD COLUMN IF NOT EXISTS oidc_sub TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS oidc_provider TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS avatar_url TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS discord_id TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS discord_username TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS discord_avatar TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS discord_linked_at TIMESTAMP(3);
-- users.password_hash: make nullable (Prisma v1.4.0 changed this for OIDC-only users)
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='users' AND column_name='password_hash' AND is_nullable='NO') THEN
        ALTER TABLE users ALTER COLUMN password_hash DROP NOT NULL;
    END IF;
END $$;

-- hosts: columns added across many Prisma migrations
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS machine_id TEXT;
-- machine_id was initially NOT NULL + UNIQUE in Prisma, later made nullable and unique dropped
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='hosts' AND column_name='machine_id' AND is_nullable='NO') THEN
        ALTER TABLE hosts ALTER COLUMN machine_id DROP NOT NULL;
    END IF;
END $$;
ALTER TABLE hosts DROP CONSTRAINT IF EXISTS hosts_machine_id_key;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS agent_version TEXT;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS auto_update BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS cpu_cores INTEGER;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS cpu_model TEXT;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS disk_details JSONB;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS dns_servers JSONB;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS gateway_ip TEXT;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS hostname TEXT;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS kernel_version TEXT;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS installed_kernel_version TEXT;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS load_average JSONB;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS network_interfaces JSONB;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS ram_installed DOUBLE PRECISION;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS selinux_status TEXT;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS swap_size DOUBLE PRECISION;
-- Pre-v1.4.2 Prisma databases have ram_installed/swap_size as INTEGER; Go expects DOUBLE PRECISION
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='hosts' AND column_name='ram_installed') THEN
        ALTER TABLE hosts ALTER COLUMN ram_installed TYPE DOUBLE PRECISION USING ram_installed::DOUBLE PRECISION;
    END IF;
END $$;
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='hosts' AND column_name='swap_size') THEN
        ALTER TABLE hosts ALTER COLUMN swap_size TYPE DOUBLE PRECISION USING swap_size::DOUBLE PRECISION;
    END IF;
END $$;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS system_uptime TEXT;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS notes TEXT;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS needs_reboot BOOLEAN DEFAULT false;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS reboot_reason TEXT;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS docker_enabled BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS compliance_enabled BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS compliance_on_demand_only BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS compliance_openscap_enabled BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS compliance_docker_bench_enabled BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS compliance_scanner_status JSONB;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS compliance_scanner_updated_at TIMESTAMP(3);
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS host_down_alerts_enabled BOOLEAN;
-- Prisma v1.4.0 added host_down_alerts_enabled as NOT NULL DEFAULT true; make nullable for Go consistency
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='hosts' AND column_name='host_down_alerts_enabled' AND is_nullable='NO') THEN
        ALTER TABLE hosts ALTER COLUMN host_down_alerts_enabled DROP NOT NULL;
    END IF;
END $$;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS expected_platform TEXT;

-- role_permissions: column added in v1.4.0
ALTER TABLE role_permissions ADD COLUMN IF NOT EXISTS can_manage_superusers BOOLEAN NOT NULL DEFAULT false;

-- user_sessions: columns added in later Prisma migrations
ALTER TABLE user_sessions ADD COLUMN IF NOT EXISTS access_token_hash TEXT;
ALTER TABLE user_sessions ADD COLUMN IF NOT EXISTS device_fingerprint TEXT;
ALTER TABLE user_sessions ADD COLUMN IF NOT EXISTS device_id TEXT;
ALTER TABLE user_sessions ADD COLUMN IF NOT EXISTS tfa_remember_me BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE user_sessions ADD COLUMN IF NOT EXISTS tfa_bypass_until TIMESTAMP(3);
ALTER TABLE user_sessions ADD COLUMN IF NOT EXISTS login_count INTEGER NOT NULL DEFAULT 1;
ALTER TABLE user_sessions ADD COLUMN IF NOT EXISTS last_login_ip TEXT;

-- auto_enrollment_tokens: scopes added later
ALTER TABLE auto_enrollment_tokens ADD COLUMN IF NOT EXISTS scopes JSONB;

-- update_history: columns added in later Prisma migrations
ALTER TABLE update_history ADD COLUMN IF NOT EXISTS total_packages INTEGER;
ALTER TABLE update_history ADD COLUMN IF NOT EXISTS payload_size_kb DOUBLE PRECISION;
ALTER TABLE update_history ADD COLUMN IF NOT EXISTS execution_time DOUBLE PRECISION;

-- dashboard_preferences: col_span added in v1.4.2
ALTER TABLE dashboard_preferences ADD COLUMN IF NOT EXISTS col_span INTEGER NOT NULL DEFAULT 1;

-- docker_containers: labels added in v1.4.0
ALTER TABLE docker_containers ADD COLUMN IF NOT EXISTS labels JSONB;

-- alert_history.user_id: made nullable in v1.4.0 (for system-generated actions)
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='alert_history' AND column_name='user_id' AND is_nullable='NO') THEN
        ALTER TABLE alert_history ALTER COLUMN user_id DROP NOT NULL;
    END IF;
END $$;

-- release_notes_acceptances: ensure id has DEFAULT (Prisma created without it)
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name='release_notes_acceptances') THEN
        ALTER TABLE release_notes_acceptances ALTER COLUMN id SET DEFAULT gen_random_uuid()::TEXT;
    END IF;
END $$;

-- release_notes_acceptances: add FK if missing
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint c
        JOIN pg_class t ON c.conrelid = t.oid
        JOIN pg_namespace n ON t.relnamespace = n.oid
        WHERE c.conname = 'release_notes_acceptances_user_id_fkey'
        AND t.relname = 'release_notes_acceptances'
        AND n.nspname = 'public'
    ) THEN
        ALTER TABLE release_notes_acceptances
        ADD CONSTRAINT release_notes_acceptances_user_id_fkey
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
    END IF;
END $$;

-- Default settings row (idempotent: only when table is empty)
INSERT INTO settings (id, server_url, server_protocol, server_host, server_port, created_at, updated_at)
SELECT 'default', 'http://localhost:3000', 'http', 'localhost', 3000, NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM settings LIMIT 1);

-- Seed default role_permissions (superadmin, admin, host_manager, readonly, user).
-- Uses ON CONFLICT to preserve any existing customized permissions.
INSERT INTO role_permissions (
    id, role,
    can_view_dashboard, can_view_hosts, can_manage_hosts,
    can_view_packages, can_manage_packages,
    can_view_users, can_manage_users, can_manage_superusers,
    can_view_reports, can_export_data, can_manage_settings,
    created_at, updated_at
)
VALUES
    (gen_random_uuid()::TEXT, 'superadmin', true, true, true, true, true, true, true, true, true, true, true, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (gen_random_uuid()::TEXT, 'admin',      true, true, true, true, true, true, true, false, true, true, true, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (gen_random_uuid()::TEXT, 'host_manager', true, true, true, true, true, false, false, false, true, true, false, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (gen_random_uuid()::TEXT, 'readonly',   true, true, false, true, false, false, false, false, true, false, false, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (gen_random_uuid()::TEXT, 'user',       true, true, false, true, false, false, false, false, true, true, false, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT (role) DO NOTHING;

-- =============================================================================
-- Performance indexes
-- =============================================================================
CREATE INDEX IF NOT EXISTS idx_hosts_status ON hosts(status);
CREATE INDEX IF NOT EXISTS idx_hosts_last_update ON hosts(last_update);
CREATE INDEX IF NOT EXISTS idx_hosts_needs_reboot ON hosts(needs_reboot) WHERE needs_reboot = true;
CREATE INDEX IF NOT EXISTS idx_host_packages_host_id ON host_packages(host_id);
CREATE INDEX IF NOT EXISTS idx_host_packages_package_id ON host_packages(package_id);
CREATE INDEX IF NOT EXISTS idx_host_packages_needs_update ON host_packages(host_id) WHERE needs_update = true;
CREATE INDEX IF NOT EXISTS idx_host_group_memberships_host_id ON host_group_memberships(host_id);
CREATE INDEX IF NOT EXISTS idx_host_group_memberships_host_group_id ON host_group_memberships(host_group_id);
CREATE INDEX IF NOT EXISTS idx_docker_containers_host_id ON docker_containers(host_id);
CREATE INDEX IF NOT EXISTS idx_docker_containers_status ON docker_containers(status);
CREATE INDEX IF NOT EXISTS idx_docker_containers_image_id ON docker_containers(image_id);
CREATE INDEX IF NOT EXISTS idx_compliance_scans_host_id ON compliance_scans(host_id);
CREATE INDEX IF NOT EXISTS idx_compliance_scans_status ON compliance_scans(status);
CREATE INDEX IF NOT EXISTS idx_compliance_scans_started_at ON compliance_scans(started_at);
CREATE INDEX IF NOT EXISTS idx_job_history_api_id ON job_history(api_id);
CREATE INDEX IF NOT EXISTS idx_user_sessions_user_id ON user_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_user_sessions_expires_at ON user_sessions(expires_at);
CREATE INDEX IF NOT EXISTS idx_user_sessions_user_device ON user_sessions(user_id, device_id) WHERE device_id IS NOT NULL AND is_revoked = false;
CREATE INDEX IF NOT EXISTS idx_update_history_host_id ON update_history(host_id);
CREATE INDEX IF NOT EXISTS idx_update_history_timestamp ON update_history(timestamp);
CREATE INDEX IF NOT EXISTS idx_system_statistics_timestamp ON system_statistics(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_users_username_lower ON users (LOWER(username));
CREATE INDEX IF NOT EXISTS idx_users_email_lower ON users (LOWER(email));
CREATE INDEX IF NOT EXISTS release_notes_acceptances_user_id_idx ON release_notes_acceptances(user_id);
CREATE INDEX IF NOT EXISTS release_notes_acceptances_version_idx ON release_notes_acceptances(version);

-- Unique constraints for columns added via ALTER TABLE (may not have UNIQUE yet)
CREATE UNIQUE INDEX IF NOT EXISTS users_oidc_sub_key ON users(oidc_sub) WHERE oidc_sub IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS users_discord_id_key ON users(discord_id) WHERE discord_id IS NOT NULL;

CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX IF NOT EXISTS idx_hosts_friendly_name_trgm ON hosts USING gin (friendly_name gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_packages_name_trgm ON packages USING gin (name gin_trgm_ops);
