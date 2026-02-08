-- CreateTable
CREATE TABLE "dashboard_preferences" (
    "id" TEXT NOT NULL,
    "user_id" TEXT NOT NULL,
    "card_id" TEXT NOT NULL,
    "enabled" BOOLEAN NOT NULL DEFAULT true,
    "order" INTEGER NOT NULL DEFAULT 0,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "dashboard_preferences_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "host_groups" (
    "id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "description" TEXT,
    "color" TEXT DEFAULT '#3B82F6',
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "host_groups_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "host_group_memberships" (
    "id" TEXT NOT NULL,
    "host_id" TEXT NOT NULL,
    "host_group_id" TEXT NOT NULL,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "host_group_memberships_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "host_packages" (
    "id" TEXT NOT NULL,
    "host_id" TEXT NOT NULL,
    "package_id" TEXT NOT NULL,
    "current_version" TEXT NOT NULL,
    "available_version" TEXT,
    "needs_update" BOOLEAN NOT NULL DEFAULT false,
    "is_security_update" BOOLEAN NOT NULL DEFAULT false,
    "last_checked" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "host_packages_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "host_repositories" (
    "id" TEXT NOT NULL,
    "host_id" TEXT NOT NULL,
    "repository_id" TEXT NOT NULL,
    "is_enabled" BOOLEAN NOT NULL DEFAULT true,
    "last_checked" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "host_repositories_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "hosts" (
    "id" TEXT NOT NULL,
    "machine_id" TEXT,
    "friendly_name" TEXT NOT NULL,
    "ip" TEXT,
    "os_type" TEXT NOT NULL,
    "os_version" TEXT NOT NULL,
    "architecture" TEXT,
    "last_update" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "status" TEXT NOT NULL DEFAULT 'active',
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,
    "api_id" TEXT NOT NULL,
    "api_key" TEXT NOT NULL,
    "agent_version" TEXT,
    "auto_update" BOOLEAN NOT NULL DEFAULT true,
    "cpu_cores" INTEGER,
    "cpu_model" TEXT,
    "disk_details" JSONB,
    "dns_servers" JSONB,
    "gateway_ip" TEXT,
    "hostname" TEXT,
    "kernel_version" TEXT,
    "installed_kernel_version" TEXT,
    "load_average" JSONB,
    "network_interfaces" JSONB,
    "ram_installed" INTEGER,
    "selinux_status" TEXT,
    "swap_size" INTEGER,
    "system_uptime" TEXT,
    "notes" TEXT,
    "needs_reboot" BOOLEAN DEFAULT false,
    "reboot_reason" TEXT,
    "docker_enabled" BOOLEAN NOT NULL DEFAULT false,
    "compliance_enabled" BOOLEAN NOT NULL DEFAULT false,
    "compliance_on_demand_only" BOOLEAN NOT NULL DEFAULT true,

    CONSTRAINT "hosts_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "packages" (
    "id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "description" TEXT,
    "category" TEXT,
    "latest_version" TEXT,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "packages_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "repositories" (
    "id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "url" TEXT NOT NULL,
    "distribution" TEXT NOT NULL,
    "components" TEXT NOT NULL,
    "repo_type" TEXT NOT NULL,
    "is_active" BOOLEAN NOT NULL DEFAULT true,
    "is_secure" BOOLEAN NOT NULL DEFAULT true,
    "priority" INTEGER,
    "description" TEXT,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "repositories_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "role_permissions" (
    "id" TEXT NOT NULL,
    "role" TEXT NOT NULL,
    "can_view_dashboard" BOOLEAN NOT NULL DEFAULT true,
    "can_view_hosts" BOOLEAN NOT NULL DEFAULT true,
    "can_manage_hosts" BOOLEAN NOT NULL DEFAULT false,
    "can_view_packages" BOOLEAN NOT NULL DEFAULT true,
    "can_manage_packages" BOOLEAN NOT NULL DEFAULT false,
    "can_view_users" BOOLEAN NOT NULL DEFAULT false,
    "can_manage_users" BOOLEAN NOT NULL DEFAULT false,
    "can_manage_superusers" BOOLEAN NOT NULL DEFAULT false,
    "can_view_reports" BOOLEAN NOT NULL DEFAULT true,
    "can_export_data" BOOLEAN NOT NULL DEFAULT false,
    "can_manage_settings" BOOLEAN NOT NULL DEFAULT false,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "role_permissions_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "settings" (
    "id" TEXT NOT NULL,
    "server_url" TEXT NOT NULL DEFAULT 'http://localhost:3001',
    "server_protocol" TEXT NOT NULL DEFAULT 'http',
    "server_host" TEXT NOT NULL DEFAULT 'localhost',
    "server_port" INTEGER NOT NULL DEFAULT 3001,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,
    "update_interval" INTEGER NOT NULL DEFAULT 60,
    "auto_update" BOOLEAN NOT NULL DEFAULT false,
    "github_repo_url" TEXT NOT NULL DEFAULT 'https://github.com/PatchMon/PatchMon.git',
    "ssh_key_path" TEXT,
    "repository_type" TEXT NOT NULL DEFAULT 'public',
    "last_update_check" TIMESTAMP(3),
    "latest_version" TEXT,
    "update_available" BOOLEAN NOT NULL DEFAULT false,
    "signup_enabled" BOOLEAN NOT NULL DEFAULT false,
    "default_user_role" TEXT NOT NULL DEFAULT 'user',
    "ignore_ssl_self_signed" BOOLEAN NOT NULL DEFAULT false,
    "logo_dark" TEXT DEFAULT '/assets/logo_dark.png',
    "logo_light" TEXT DEFAULT '/assets/logo_light.png',
    "favicon" TEXT DEFAULT '/assets/logo_square.svg',
    "metrics_enabled" BOOLEAN NOT NULL DEFAULT true,
    "metrics_anonymous_id" TEXT,
    "metrics_last_sent" TIMESTAMP(3),
    "show_github_version_on_login" BOOLEAN NOT NULL DEFAULT true,
    "ai_enabled" BOOLEAN NOT NULL DEFAULT false,
    "ai_provider" TEXT NOT NULL DEFAULT 'openrouter',
    "ai_model" TEXT,
    "ai_api_key" TEXT,

    CONSTRAINT "settings_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "update_history" (
    "id" TEXT NOT NULL,
    "host_id" TEXT NOT NULL,
    "packages_count" INTEGER NOT NULL,
    "security_count" INTEGER NOT NULL,
    "total_packages" INTEGER,
    "payload_size_kb" DOUBLE PRECISION,
    "execution_time" DOUBLE PRECISION,
    "timestamp" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "status" TEXT NOT NULL DEFAULT 'success',
    "error_message" TEXT,

    CONSTRAINT "update_history_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "system_statistics" (
    "id" TEXT NOT NULL,
    "unique_packages_count" INTEGER NOT NULL,
    "unique_security_count" INTEGER NOT NULL,
    "total_packages" INTEGER NOT NULL,
    "total_hosts" INTEGER NOT NULL,
    "hosts_needing_updates" INTEGER NOT NULL,
    "timestamp" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "system_statistics_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "users" (
    "id" TEXT NOT NULL,
    "username" TEXT NOT NULL,
    "email" TEXT NOT NULL,
    "password_hash" TEXT,
    "role" TEXT NOT NULL DEFAULT 'admin',
    "is_active" BOOLEAN NOT NULL DEFAULT true,
    "last_login" TIMESTAMP(3),
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,
    "tfa_backup_codes" TEXT,
    "tfa_enabled" BOOLEAN NOT NULL DEFAULT false,
    "tfa_secret" TEXT,
    "first_name" TEXT,
    "last_name" TEXT,
    "theme_preference" TEXT DEFAULT 'dark',
    "color_theme" TEXT DEFAULT 'cyber_blue',
    "oidc_sub" TEXT,
    "oidc_provider" TEXT,
    "avatar_url" TEXT,

    CONSTRAINT "users_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "user_sessions" (
    "id" TEXT NOT NULL,
    "user_id" TEXT NOT NULL,
    "refresh_token" TEXT NOT NULL,
    "access_token_hash" TEXT,
    "ip_address" TEXT,
    "user_agent" TEXT,
    "device_fingerprint" TEXT,
    "last_activity" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "expires_at" TIMESTAMP(3) NOT NULL,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "is_revoked" BOOLEAN NOT NULL DEFAULT false,
    "tfa_remember_me" BOOLEAN NOT NULL DEFAULT false,
    "tfa_bypass_until" TIMESTAMP(3),
    "login_count" INTEGER NOT NULL DEFAULT 1,
    "last_login_ip" TEXT,

    CONSTRAINT "user_sessions_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "auto_enrollment_tokens" (
    "id" TEXT NOT NULL,
    "token_name" TEXT NOT NULL,
    "token_key" TEXT NOT NULL,
    "token_secret" TEXT NOT NULL,
    "created_by_user_id" TEXT,
    "is_active" BOOLEAN NOT NULL DEFAULT true,
    "allowed_ip_ranges" TEXT[],
    "max_hosts_per_day" INTEGER NOT NULL DEFAULT 100,
    "hosts_created_today" INTEGER NOT NULL DEFAULT 0,
    "last_reset_date" DATE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "default_host_group_id" TEXT,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,
    "last_used_at" TIMESTAMP(3),
    "expires_at" TIMESTAMP(3),
    "metadata" JSONB,
    "scopes" JSONB,

    CONSTRAINT "auto_enrollment_tokens_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "docker_containers" (
    "id" TEXT NOT NULL,
    "host_id" TEXT NOT NULL,
    "container_id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "image_id" TEXT,
    "image_name" TEXT NOT NULL,
    "image_tag" TEXT NOT NULL DEFAULT 'latest',
    "status" TEXT NOT NULL,
    "state" TEXT,
    "ports" JSONB,
    "labels" JSONB,
    "created_at" TIMESTAMP(3) NOT NULL,
    "started_at" TIMESTAMP(3),
    "updated_at" TIMESTAMP(3) NOT NULL,
    "last_checked" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "docker_containers_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "docker_images" (
    "id" TEXT NOT NULL,
    "repository" TEXT NOT NULL,
    "tag" TEXT NOT NULL DEFAULT 'latest',
    "image_id" TEXT NOT NULL,
    "digest" TEXT,
    "size_bytes" BIGINT,
    "source" TEXT NOT NULL DEFAULT 'docker-hub',
    "created_at" TIMESTAMP(3) NOT NULL,
    "last_pulled" TIMESTAMP(3),
    "last_checked" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "docker_images_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "docker_image_updates" (
    "id" TEXT NOT NULL,
    "image_id" TEXT NOT NULL,
    "current_tag" TEXT NOT NULL,
    "available_tag" TEXT NOT NULL,
    "is_security_update" BOOLEAN NOT NULL DEFAULT false,
    "severity" TEXT,
    "changelog_url" TEXT,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "docker_image_updates_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "docker_volumes" (
    "id" TEXT NOT NULL,
    "host_id" TEXT NOT NULL,
    "volume_id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "driver" TEXT NOT NULL,
    "mountpoint" TEXT,
    "renderer" TEXT,
    "scope" TEXT NOT NULL DEFAULT 'local',
    "labels" JSONB,
    "options" JSONB,
    "size_bytes" BIGINT,
    "ref_count" INTEGER NOT NULL DEFAULT 0,
    "created_at" TIMESTAMP(3) NOT NULL,
    "updated_at" TIMESTAMP(3) NOT NULL,
    "last_checked" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "docker_volumes_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "docker_networks" (
    "id" TEXT NOT NULL,
    "host_id" TEXT NOT NULL,
    "network_id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "driver" TEXT NOT NULL,
    "scope" TEXT NOT NULL DEFAULT 'local',
    "ipv6_enabled" BOOLEAN NOT NULL DEFAULT false,
    "internal" BOOLEAN NOT NULL DEFAULT false,
    "attachable" BOOLEAN NOT NULL DEFAULT true,
    "ingress" BOOLEAN NOT NULL DEFAULT false,
    "config_only" BOOLEAN NOT NULL DEFAULT false,
    "labels" JSONB,
    "ipam" JSONB,
    "container_count" INTEGER NOT NULL DEFAULT 0,
    "created_at" TIMESTAMP(3),
    "updated_at" TIMESTAMP(3) NOT NULL,
    "last_checked" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "docker_networks_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "job_history" (
    "id" TEXT NOT NULL,
    "job_id" TEXT NOT NULL,
    "queue_name" TEXT NOT NULL,
    "job_name" TEXT NOT NULL,
    "host_id" TEXT,
    "api_id" TEXT,
    "status" TEXT NOT NULL,
    "attempt_number" INTEGER NOT NULL DEFAULT 1,
    "error_message" TEXT,
    "output" JSONB,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,
    "completed_at" TIMESTAMP(3),

    CONSTRAINT "job_history_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "release_notes_acceptances" (
    "id" TEXT NOT NULL,
    "user_id" TEXT NOT NULL,
    "version" TEXT NOT NULL,
    "accepted_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "release_notes_acceptances_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "audit_logs" (
    "id" TEXT NOT NULL,
    "event" TEXT NOT NULL,
    "user_id" TEXT,
    "target_user_id" TEXT,
    "ip_address" TEXT,
    "user_agent" TEXT,
    "request_id" TEXT,
    "details" TEXT,
    "success" BOOLEAN NOT NULL DEFAULT true,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "audit_logs_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "compliance_profiles" (
    "id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "type" TEXT NOT NULL,
    "os_family" TEXT,
    "version" TEXT,
    "description" TEXT,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "compliance_profiles_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "compliance_scans" (
    "id" TEXT NOT NULL,
    "host_id" TEXT NOT NULL,
    "profile_id" TEXT NOT NULL,
    "started_at" TIMESTAMP(3) NOT NULL,
    "completed_at" TIMESTAMP(3),
    "status" TEXT NOT NULL,
    "total_rules" INTEGER NOT NULL DEFAULT 0,
    "passed" INTEGER NOT NULL DEFAULT 0,
    "failed" INTEGER NOT NULL DEFAULT 0,
    "warnings" INTEGER NOT NULL DEFAULT 0,
    "skipped" INTEGER NOT NULL DEFAULT 0,
    "not_applicable" INTEGER NOT NULL DEFAULT 0,
    "score" DOUBLE PRECISION,
    "error_message" TEXT,
    "raw_output" TEXT,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "compliance_scans_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "compliance_rules" (
    "id" TEXT NOT NULL,
    "profile_id" TEXT NOT NULL,
    "rule_ref" TEXT NOT NULL,
    "title" TEXT NOT NULL,
    "description" TEXT,
    "rationale" TEXT,
    "severity" TEXT,
    "section" TEXT,
    "remediation" TEXT,

    CONSTRAINT "compliance_rules_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "compliance_results" (
    "id" TEXT NOT NULL,
    "scan_id" TEXT NOT NULL,
    "rule_id" TEXT NOT NULL,
    "status" TEXT NOT NULL,
    "finding" TEXT,
    "actual" TEXT,
    "expected" TEXT,
    "remediation" TEXT,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "compliance_results_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE UNIQUE INDEX "dashboard_preferences_user_id_card_id_key" ON "dashboard_preferences"("user_id", "card_id");

-- CreateIndex
CREATE UNIQUE INDEX "host_groups_name_key" ON "host_groups"("name");

-- CreateIndex
CREATE INDEX "host_group_memberships_host_id_idx" ON "host_group_memberships"("host_id");

-- CreateIndex
CREATE INDEX "host_group_memberships_host_group_id_idx" ON "host_group_memberships"("host_group_id");

-- CreateIndex
CREATE UNIQUE INDEX "host_group_memberships_host_id_host_group_id_key" ON "host_group_memberships"("host_id", "host_group_id");

-- CreateIndex
CREATE INDEX "host_packages_host_id_idx" ON "host_packages"("host_id");

-- CreateIndex
CREATE INDEX "host_packages_package_id_idx" ON "host_packages"("package_id");

-- CreateIndex
CREATE INDEX "host_packages_needs_update_idx" ON "host_packages"("needs_update");

-- CreateIndex
CREATE INDEX "host_packages_is_security_update_idx" ON "host_packages"("is_security_update");

-- CreateIndex
CREATE INDEX "host_packages_host_id_needs_update_idx" ON "host_packages"("host_id", "needs_update");

-- CreateIndex
CREATE INDEX "host_packages_host_id_needs_update_is_security_update_idx" ON "host_packages"("host_id", "needs_update", "is_security_update");

-- CreateIndex
CREATE INDEX "host_packages_package_id_needs_update_idx" ON "host_packages"("package_id", "needs_update");

-- CreateIndex
CREATE INDEX "host_packages_last_checked_idx" ON "host_packages"("last_checked");

-- CreateIndex
CREATE UNIQUE INDEX "host_packages_host_id_package_id_key" ON "host_packages"("host_id", "package_id");

-- CreateIndex
CREATE INDEX "host_repositories_host_id_idx" ON "host_repositories"("host_id");

-- CreateIndex
CREATE INDEX "host_repositories_repository_id_idx" ON "host_repositories"("repository_id");

-- CreateIndex
CREATE INDEX "host_repositories_is_enabled_idx" ON "host_repositories"("is_enabled");

-- CreateIndex
CREATE UNIQUE INDEX "host_repositories_host_id_repository_id_key" ON "host_repositories"("host_id", "repository_id");

-- CreateIndex
CREATE UNIQUE INDEX "hosts_api_id_key" ON "hosts"("api_id");

-- CreateIndex
CREATE UNIQUE INDEX "hosts_api_key_key" ON "hosts"("api_key");

-- CreateIndex
CREATE INDEX "hosts_machine_id_idx" ON "hosts"("machine_id");

-- CreateIndex
CREATE INDEX "hosts_friendly_name_idx" ON "hosts"("friendly_name");

-- CreateIndex
CREATE INDEX "hosts_hostname_idx" ON "hosts"("hostname");

-- CreateIndex
CREATE INDEX "hosts_needs_reboot_idx" ON "hosts"("needs_reboot");

-- CreateIndex
CREATE INDEX "hosts_status_idx" ON "hosts"("status");

-- CreateIndex
CREATE INDEX "hosts_status_api_id_idx" ON "hosts"("status", "api_id");

-- CreateIndex
CREATE INDEX "hosts_last_update_idx" ON "hosts"("last_update");

-- CreateIndex
CREATE UNIQUE INDEX "packages_name_key" ON "packages"("name");

-- CreateIndex
CREATE INDEX "packages_name_idx" ON "packages"("name");

-- CreateIndex
CREATE INDEX "packages_category_idx" ON "packages"("category");

-- CreateIndex
CREATE INDEX "repositories_is_active_idx" ON "repositories"("is_active");

-- CreateIndex
CREATE INDEX "repositories_name_idx" ON "repositories"("name");

-- CreateIndex
CREATE UNIQUE INDEX "repositories_url_distribution_components_key" ON "repositories"("url", "distribution", "components");

-- CreateIndex
CREATE UNIQUE INDEX "role_permissions_role_key" ON "role_permissions"("role");

-- CreateIndex
CREATE INDEX "update_history_host_id_idx" ON "update_history"("host_id");

-- CreateIndex
CREATE INDEX "update_history_timestamp_idx" ON "update_history"("timestamp");

-- CreateIndex
CREATE INDEX "update_history_status_idx" ON "update_history"("status");

-- CreateIndex
CREATE INDEX "update_history_host_id_timestamp_idx" ON "update_history"("host_id", "timestamp");

-- CreateIndex
CREATE INDEX "system_statistics_timestamp_idx" ON "system_statistics"("timestamp");

-- CreateIndex
CREATE UNIQUE INDEX "users_username_key" ON "users"("username");

-- CreateIndex
CREATE UNIQUE INDEX "users_email_key" ON "users"("email");

-- CreateIndex
CREATE UNIQUE INDEX "users_oidc_sub_key" ON "users"("oidc_sub");

-- CreateIndex
CREATE INDEX "users_role_idx" ON "users"("role");

-- CreateIndex
CREATE UNIQUE INDEX "user_sessions_refresh_token_key" ON "user_sessions"("refresh_token");

-- CreateIndex
CREATE INDEX "user_sessions_user_id_idx" ON "user_sessions"("user_id");

-- CreateIndex
CREATE INDEX "user_sessions_refresh_token_idx" ON "user_sessions"("refresh_token");

-- CreateIndex
CREATE INDEX "user_sessions_expires_at_idx" ON "user_sessions"("expires_at");

-- CreateIndex
CREATE INDEX "user_sessions_tfa_bypass_until_idx" ON "user_sessions"("tfa_bypass_until");

-- CreateIndex
CREATE INDEX "user_sessions_device_fingerprint_idx" ON "user_sessions"("device_fingerprint");

-- CreateIndex
CREATE UNIQUE INDEX "auto_enrollment_tokens_token_key_key" ON "auto_enrollment_tokens"("token_key");

-- CreateIndex
CREATE INDEX "auto_enrollment_tokens_token_key_idx" ON "auto_enrollment_tokens"("token_key");

-- CreateIndex
CREATE INDEX "auto_enrollment_tokens_is_active_idx" ON "auto_enrollment_tokens"("is_active");

-- CreateIndex
CREATE INDEX "docker_containers_host_id_idx" ON "docker_containers"("host_id");

-- CreateIndex
CREATE INDEX "docker_containers_image_id_idx" ON "docker_containers"("image_id");

-- CreateIndex
CREATE INDEX "docker_containers_status_idx" ON "docker_containers"("status");

-- CreateIndex
CREATE INDEX "docker_containers_name_idx" ON "docker_containers"("name");

-- CreateIndex
CREATE UNIQUE INDEX "docker_containers_host_id_container_id_key" ON "docker_containers"("host_id", "container_id");

-- CreateIndex
CREATE INDEX "docker_images_repository_idx" ON "docker_images"("repository");

-- CreateIndex
CREATE INDEX "docker_images_source_idx" ON "docker_images"("source");

-- CreateIndex
CREATE INDEX "docker_images_repository_tag_idx" ON "docker_images"("repository", "tag");

-- CreateIndex
CREATE UNIQUE INDEX "docker_images_repository_tag_image_id_key" ON "docker_images"("repository", "tag", "image_id");

-- CreateIndex
CREATE INDEX "docker_image_updates_image_id_idx" ON "docker_image_updates"("image_id");

-- CreateIndex
CREATE INDEX "docker_image_updates_is_security_update_idx" ON "docker_image_updates"("is_security_update");

-- CreateIndex
CREATE UNIQUE INDEX "docker_image_updates_image_id_available_tag_key" ON "docker_image_updates"("image_id", "available_tag");

-- CreateIndex
CREATE INDEX "docker_volumes_host_id_idx" ON "docker_volumes"("host_id");

-- CreateIndex
CREATE INDEX "docker_volumes_name_idx" ON "docker_volumes"("name");

-- CreateIndex
CREATE INDEX "docker_volumes_driver_idx" ON "docker_volumes"("driver");

-- CreateIndex
CREATE UNIQUE INDEX "docker_volumes_host_id_volume_id_key" ON "docker_volumes"("host_id", "volume_id");

-- CreateIndex
CREATE INDEX "docker_networks_host_id_idx" ON "docker_networks"("host_id");

-- CreateIndex
CREATE INDEX "docker_networks_name_idx" ON "docker_networks"("name");

-- CreateIndex
CREATE INDEX "docker_networks_driver_idx" ON "docker_networks"("driver");

-- CreateIndex
CREATE UNIQUE INDEX "docker_networks_host_id_network_id_key" ON "docker_networks"("host_id", "network_id");

-- CreateIndex
CREATE INDEX "job_history_job_id_idx" ON "job_history"("job_id");

-- CreateIndex
CREATE INDEX "job_history_queue_name_idx" ON "job_history"("queue_name");

-- CreateIndex
CREATE INDEX "job_history_host_id_idx" ON "job_history"("host_id");

-- CreateIndex
CREATE INDEX "job_history_api_id_idx" ON "job_history"("api_id");

-- CreateIndex
CREATE INDEX "job_history_status_idx" ON "job_history"("status");

-- CreateIndex
CREATE INDEX "job_history_created_at_idx" ON "job_history"("created_at");

-- CreateIndex
CREATE INDEX "release_notes_acceptances_user_id_idx" ON "release_notes_acceptances"("user_id");

-- CreateIndex
CREATE INDEX "release_notes_acceptances_version_idx" ON "release_notes_acceptances"("version");

-- CreateIndex
CREATE UNIQUE INDEX "release_notes_acceptances_user_id_version_key" ON "release_notes_acceptances"("user_id", "version");

-- CreateIndex
CREATE INDEX "audit_logs_event_idx" ON "audit_logs"("event");

-- CreateIndex
CREATE INDEX "audit_logs_user_id_idx" ON "audit_logs"("user_id");

-- CreateIndex
CREATE INDEX "audit_logs_target_user_id_idx" ON "audit_logs"("target_user_id");

-- CreateIndex
CREATE INDEX "audit_logs_created_at_idx" ON "audit_logs"("created_at");

-- CreateIndex
CREATE INDEX "audit_logs_success_idx" ON "audit_logs"("success");

-- CreateIndex
CREATE UNIQUE INDEX "compliance_profiles_name_key" ON "compliance_profiles"("name");

-- CreateIndex
CREATE INDEX "compliance_profiles_type_idx" ON "compliance_profiles"("type");

-- CreateIndex
CREATE INDEX "compliance_profiles_os_family_idx" ON "compliance_profiles"("os_family");

-- CreateIndex
CREATE INDEX "compliance_scans_host_id_idx" ON "compliance_scans"("host_id");

-- CreateIndex
CREATE INDEX "compliance_scans_profile_id_idx" ON "compliance_scans"("profile_id");

-- CreateIndex
CREATE INDEX "compliance_scans_status_idx" ON "compliance_scans"("status");

-- CreateIndex
CREATE INDEX "compliance_scans_started_at_idx" ON "compliance_scans"("started_at");

-- CreateIndex
CREATE INDEX "compliance_scans_completed_at_idx" ON "compliance_scans"("completed_at");

-- CreateIndex
CREATE INDEX "compliance_scans_host_id_started_at_idx" ON "compliance_scans"("host_id", "started_at");

-- CreateIndex
CREATE INDEX "compliance_scans_host_id_profile_id_idx" ON "compliance_scans"("host_id", "profile_id");

-- CreateIndex
CREATE INDEX "compliance_rules_profile_id_idx" ON "compliance_rules"("profile_id");

-- CreateIndex
CREATE INDEX "compliance_rules_severity_idx" ON "compliance_rules"("severity");

-- CreateIndex
CREATE INDEX "compliance_rules_section_idx" ON "compliance_rules"("section");

-- CreateIndex
CREATE UNIQUE INDEX "compliance_rules_profile_id_rule_ref_key" ON "compliance_rules"("profile_id", "rule_ref");

-- CreateIndex
CREATE INDEX "compliance_results_scan_id_idx" ON "compliance_results"("scan_id");

-- CreateIndex
CREATE INDEX "compliance_results_rule_id_idx" ON "compliance_results"("rule_id");

-- CreateIndex
CREATE INDEX "compliance_results_status_idx" ON "compliance_results"("status");

-- CreateIndex
CREATE INDEX "compliance_results_scan_id_status_idx" ON "compliance_results"("scan_id", "status");

-- CreateIndex
CREATE UNIQUE INDEX "compliance_results_scan_id_rule_id_key" ON "compliance_results"("scan_id", "rule_id");

-- AddForeignKey
ALTER TABLE "dashboard_preferences" ADD CONSTRAINT "dashboard_preferences_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "host_group_memberships" ADD CONSTRAINT "host_group_memberships_host_id_fkey" FOREIGN KEY ("host_id") REFERENCES "hosts"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "host_group_memberships" ADD CONSTRAINT "host_group_memberships_host_group_id_fkey" FOREIGN KEY ("host_group_id") REFERENCES "host_groups"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "host_packages" ADD CONSTRAINT "host_packages_host_id_fkey" FOREIGN KEY ("host_id") REFERENCES "hosts"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "host_packages" ADD CONSTRAINT "host_packages_package_id_fkey" FOREIGN KEY ("package_id") REFERENCES "packages"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "host_repositories" ADD CONSTRAINT "host_repositories_host_id_fkey" FOREIGN KEY ("host_id") REFERENCES "hosts"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "host_repositories" ADD CONSTRAINT "host_repositories_repository_id_fkey" FOREIGN KEY ("repository_id") REFERENCES "repositories"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "update_history" ADD CONSTRAINT "update_history_host_id_fkey" FOREIGN KEY ("host_id") REFERENCES "hosts"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "user_sessions" ADD CONSTRAINT "user_sessions_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "auto_enrollment_tokens" ADD CONSTRAINT "auto_enrollment_tokens_created_by_user_id_fkey" FOREIGN KEY ("created_by_user_id") REFERENCES "users"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "auto_enrollment_tokens" ADD CONSTRAINT "auto_enrollment_tokens_default_host_group_id_fkey" FOREIGN KEY ("default_host_group_id") REFERENCES "host_groups"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "docker_containers" ADD CONSTRAINT "docker_containers_image_id_fkey" FOREIGN KEY ("image_id") REFERENCES "docker_images"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "docker_image_updates" ADD CONSTRAINT "docker_image_updates_image_id_fkey" FOREIGN KEY ("image_id") REFERENCES "docker_images"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "docker_volumes" ADD CONSTRAINT "docker_volumes_host_id_fkey" FOREIGN KEY ("host_id") REFERENCES "hosts"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "docker_networks" ADD CONSTRAINT "docker_networks_host_id_fkey" FOREIGN KEY ("host_id") REFERENCES "hosts"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "job_history" ADD CONSTRAINT "job_history_host_id_fkey" FOREIGN KEY ("host_id") REFERENCES "hosts"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "release_notes_acceptances" ADD CONSTRAINT "release_notes_acceptances_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "compliance_scans" ADD CONSTRAINT "compliance_scans_host_id_fkey" FOREIGN KEY ("host_id") REFERENCES "hosts"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "compliance_scans" ADD CONSTRAINT "compliance_scans_profile_id_fkey" FOREIGN KEY ("profile_id") REFERENCES "compliance_profiles"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "compliance_rules" ADD CONSTRAINT "compliance_rules_profile_id_fkey" FOREIGN KEY ("profile_id") REFERENCES "compliance_profiles"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "compliance_results" ADD CONSTRAINT "compliance_results_scan_id_fkey" FOREIGN KEY ("scan_id") REFERENCES "compliance_scans"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "compliance_results" ADD CONSTRAINT "compliance_results_rule_id_fkey" FOREIGN KEY ("rule_id") REFERENCES "compliance_rules"("id") ON DELETE CASCADE ON UPDATE CASCADE;
