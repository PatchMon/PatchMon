-- Host report/update flow (agent sends package and system info)

-- name: UpdateHostFromReport :exec
UPDATE hosts SET
    last_update = NOW(),
    updated_at = NOW(),
    status = CASE WHEN status = 'pending' THEN 'active' ELSE status END,
    machine_id = COALESCE(sqlc.narg('machine_id')::text, machine_id),
    os_type = COALESCE(sqlc.narg('os_type')::text, os_type),
    os_version = COALESCE(sqlc.narg('os_version')::text, os_version),
    hostname = COALESCE(sqlc.narg('hostname')::text, hostname),
    ip = COALESCE(sqlc.narg('ip')::text, ip),
    architecture = COALESCE(sqlc.narg('architecture')::text, architecture),
    agent_version = COALESCE(sqlc.narg('agent_version')::text, agent_version),
    cpu_model = COALESCE(sqlc.narg('cpu_model')::text, cpu_model),
    cpu_cores = COALESCE(sqlc.narg('cpu_cores')::int, cpu_cores),
    ram_installed = COALESCE(sqlc.narg('ram_installed')::double precision, ram_installed),
    swap_size = COALESCE(sqlc.narg('swap_size')::double precision, swap_size),
    disk_details = COALESCE(sqlc.narg('disk_details')::jsonb, disk_details),
    gateway_ip = sqlc.narg('gateway_ip'),
    dns_servers = COALESCE(sqlc.narg('dns_servers')::jsonb, dns_servers),
    network_interfaces = COALESCE(sqlc.narg('network_interfaces')::jsonb, network_interfaces),
    kernel_version = COALESCE(sqlc.narg('kernel_version')::text, kernel_version),
    installed_kernel_version = COALESCE(sqlc.narg('installed_kernel_version')::text, installed_kernel_version),
    selinux_status = COALESCE(sqlc.narg('selinux_status')::text, selinux_status),
    system_uptime = COALESCE(sqlc.narg('system_uptime')::text, system_uptime),
    load_average = COALESCE(sqlc.narg('load_average')::jsonb, load_average),
    needs_reboot = COALESCE(sqlc.narg('needs_reboot')::boolean, needs_reboot),
    reboot_reason = sqlc.narg('reboot_reason'),
    package_manager = COALESCE(sqlc.narg('package_manager')::text, package_manager),
    awaiting_post_patch_report_run_id = NULL
WHERE id = sqlc.arg('id');

-- name: DeleteHostPackagesByHostID :exec
DELETE FROM host_packages WHERE host_id = $1;

-- name: GetPackageByName :one
SELECT id, name, description, category, latest_version, created_at, updated_at
FROM packages WHERE name = $1;

-- name: InsertPackage :one
INSERT INTO packages (id, name, description, category, latest_version, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
ON CONFLICT (name) DO UPDATE SET
    latest_version = COALESCE(EXCLUDED.latest_version, packages.latest_version),
    description = COALESCE(EXCLUDED.description, packages.description),
    category = COALESCE(EXCLUDED.category, packages.category),
    updated_at = NOW()
RETURNING id;

-- name: InsertHostPackage :exec
INSERT INTO host_packages (id, host_id, package_id, current_version, available_version, needs_update, is_security_update, source_repository_id, last_checked)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
ON CONFLICT (host_id, package_id) DO UPDATE SET
    current_version = EXCLUDED.current_version,
    available_version = EXCLUDED.available_version,
    needs_update = EXCLUDED.needs_update,
    is_security_update = EXCLUDED.is_security_update,
    source_repository_id = EXCLUDED.source_repository_id,
    last_checked = NOW();

-- name: InsertHostPackageWithWUA :exec
INSERT INTO host_packages (id, host_id, package_id, current_version, available_version, needs_update, is_security_update,
    wua_guid, wua_kb, wua_severity, wua_categories, wua_description, wua_support_url, wua_revision_number, source_repository_id, last_checked)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, NOW())
ON CONFLICT (host_id, package_id) DO UPDATE SET
    current_version = EXCLUDED.current_version,
    available_version = EXCLUDED.available_version,
    needs_update = EXCLUDED.needs_update,
    is_security_update = EXCLUDED.is_security_update,
    wua_guid = EXCLUDED.wua_guid,
    wua_kb = EXCLUDED.wua_kb,
    wua_severity = EXCLUDED.wua_severity,
    wua_categories = EXCLUDED.wua_categories,
    wua_description = EXCLUDED.wua_description,
    wua_support_url = EXCLUDED.wua_support_url,
    wua_revision_number = EXCLUDED.wua_revision_number,
    source_repository_id = EXCLUDED.source_repository_id,
    last_checked = NOW();

-- name: InsertUpdateHistory :exec
INSERT INTO update_history (id, host_id, packages_count, security_count, total_packages, payload_size_kb, execution_time, timestamp, status, error_message)
VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), $8, $9);
