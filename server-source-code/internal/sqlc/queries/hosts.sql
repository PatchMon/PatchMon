-- name: ListHosts :many
SELECT * FROM hosts ORDER BY friendly_name;

-- name: ListHostsPaginated :many
SELECT id, friendly_name, hostname, ip, os_type, os_version, architecture, last_update, status, api_id, agent_version, auto_update, created_at, notes, system_uptime, needs_reboot, docker_enabled, compliance_enabled
FROM hosts
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountHosts :one
SELECT COUNT(*) FROM hosts;

-- name: GetHostByID :one
SELECT * FROM hosts WHERE id = $1;

-- name: GetHostsByIDs :many
SELECT * FROM hosts WHERE id = ANY($1::text[]);

-- name: DeleteHostsByIDs :exec
DELETE FROM hosts WHERE id = ANY($1::text[]);

-- name: GetHostByApiID :one
SELECT * FROM hosts WHERE api_id = $1;

-- name: CreateHost :exec
INSERT INTO hosts (
    id, machine_id, friendly_name, ip, os_type, os_version, architecture, last_update, status,
    api_id, api_key, agent_version, auto_update, created_at, updated_at,
    docker_enabled, compliance_enabled, compliance_on_demand_only, expected_platform
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9,
    $10, $11, $12, $13, $14, $15,
    $16, $17, $18, $19
);

-- name: UpdateHostFriendlyName :exec
UPDATE hosts SET friendly_name = $1, updated_at = NOW() WHERE id = $2;

-- name: UpdateHostNotes :exec
UPDATE hosts SET notes = $1, updated_at = NOW() WHERE id = $2;

-- name: UpdateHostConnection :exec
UPDATE hosts SET ip = $1, hostname = $2, updated_at = NOW() WHERE id = $3;

-- name: UpdateHostPrimaryInterface :exec
UPDATE hosts SET primary_interface = $1, updated_at = NOW() WHERE id = $2;

-- name: UpdateHostAutoUpdate :exec
UPDATE hosts SET auto_update = $1, updated_at = NOW() WHERE id = $2;

-- name: UpdateHostDownAlerts :exec
UPDATE hosts SET host_down_alerts_enabled = $1, updated_at = NOW() WHERE id = $2;

-- name: UpdateHostDockerEnabled :exec
UPDATE hosts SET docker_enabled = $1, updated_at = NOW() WHERE id = $2;

-- name: UpdateHostComplianceEnabled :exec
UPDATE hosts SET compliance_enabled = $1, updated_at = NOW() WHERE id = $2;

-- name: UpdateHostComplianceMode :exec
UPDATE hosts SET compliance_enabled = $1, compliance_on_demand_only = $2, updated_at = NOW() WHERE id = $3;

-- name: UpdateHostComplianceScanners :exec
UPDATE hosts SET compliance_openscap_enabled = $1, compliance_docker_bench_enabled = $2, updated_at = NOW() WHERE id = $3;

-- name: UpdateHostComplianceDefaultProfile :exec
UPDATE hosts SET compliance_default_profile_id = $1, updated_at = NOW() WHERE id = $2;

-- name: UpdateHostComplianceScannerStatus :exec
UPDATE hosts SET compliance_scanner_status = $1, compliance_scanner_updated_at = $2, updated_at = NOW() WHERE id = $3;

-- name: UpdateHostApiCredentials :exec
UPDATE hosts SET api_id = $1, api_key = $2, updated_at = NOW() WHERE id = $3;

-- name: UpdateHostRebootStatus :exec
UPDATE hosts SET needs_reboot = $2, reboot_reason = $3, updated_at = NOW() WHERE id = $1;

-- name: UpdateHostPing :exec
UPDATE hosts SET last_update = NOW(), updated_at = NOW(), status = 'active' WHERE id = $1;

-- name: DeleteHost :exec
DELETE FROM hosts WHERE id = $1;

-- name: ListHostsForComplianceDashboard :many
SELECT id, hostname, friendly_name, compliance_enabled, compliance_on_demand_only, docker_enabled
FROM hosts;

-- name: CountUnscannedHosts :one
SELECT COUNT(*) FROM hosts h
WHERE NOT EXISTS (SELECT 1 FROM compliance_scans cs WHERE cs.host_id = h.id);
