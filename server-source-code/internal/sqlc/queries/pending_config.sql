-- name: GetPendingConfig :one
SELECT * FROM host_pending_config WHERE host_id = $1;

-- name: UpsertPendingConfig :exec
INSERT INTO host_pending_config (
    host_id,
    docker_enabled,
    compliance_enabled,
    compliance_on_demand_only,
    compliance_openscap_enabled,
    compliance_docker_bench_enabled,
    created_at,
    updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, COALESCE((SELECT created_at FROM host_pending_config WHERE host_id = $1), NOW()), NOW()
)
ON CONFLICT (host_id) DO UPDATE SET
    docker_enabled = COALESCE(EXCLUDED.docker_enabled, host_pending_config.docker_enabled),
    compliance_enabled = COALESCE(EXCLUDED.compliance_enabled, host_pending_config.compliance_enabled),
    compliance_on_demand_only = COALESCE(EXCLUDED.compliance_on_demand_only, host_pending_config.compliance_on_demand_only),
    compliance_openscap_enabled = COALESCE(EXCLUDED.compliance_openscap_enabled, host_pending_config.compliance_openscap_enabled),
    compliance_docker_bench_enabled = COALESCE(EXCLUDED.compliance_docker_bench_enabled, host_pending_config.compliance_docker_bench_enabled),
    updated_at = NOW();

-- name: DeletePendingConfig :exec
DELETE FROM host_pending_config WHERE host_id = $1;
