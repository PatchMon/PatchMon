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
    packages_hash = COALESCE(sqlc.narg('packages_hash')::text, packages_hash),
    repos_hash = COALESCE(sqlc.narg('repos_hash')::text, repos_hash),
    interfaces_hash = COALESCE(sqlc.narg('interfaces_hash')::text, interfaces_hash),
    hostname_hash = COALESCE(sqlc.narg('hostname_hash')::text, hostname_hash),
    last_full_report_at = COALESCE(sqlc.narg('last_full_report_at')::timestamp, last_full_report_at),
    awaiting_post_patch_report_run_id = NULL
WHERE id = sqlc.arg('id');

-- name: DeleteHostPackagesByHostID :exec
DELETE FROM host_packages WHERE host_id = $1;

-- name: GetPackageByName :one
SELECT id, name, description, category, latest_version, created_at, updated_at
FROM packages WHERE name = $1;

-- name: BulkUpsertPackages :many
-- Bulk upsert N packages in a single statement, eliminating per-row round-trips.
--
-- Caller MUST pre-sort the input rows by name AND deduplicate by name
-- (deterministic lock acquisition; ON CONFLICT cannot affect the same row
-- twice in one statement — cardinality_violation 21000). The inner ORDER BY
-- name in the `input` CTE is belt-and-braces: PostgreSQL's executor processes
-- INSERT...SELECT tuples in plan order, and the explicit Sort node above
-- jsonb_to_recordset guarantees that order even if the planner introduces
-- parallel workers (unlikely at this row count). Both layers must remain —
-- do not strip the ORDER BY thinking the Go-side sort is sufficient. Sorted
-- lock acquisition prevents the cross-host 40P01 deadlocks reported in
-- production.
--
-- Input shape: a single jsonb array of objects, each with keys
--   { id, name, description, category, latest_version }
-- We use jsonb_to_recordset rather than parallel text[]+unnest because sqlc
-- v1.x's static analyzer cannot resolve the multi-array unnest() overload
-- ("function unnest(unknown, ...) does not exist"). jsonb_to_recordset gives
-- the same per-row shape with one well-typed parameter and zero overload
-- ambiguity.
--
-- Description / category / latest_version are JSON nulls when the agent
-- omits them, which jsonb_to_recordset emits as SQL NULL directly.
--
-- The DO UPDATE WHERE clause skips no-op updates: when the agent reports the
-- same description/category/latest_version that's already stored, no row
-- update happens, no dead tuple is created, and no FOR NO KEY UPDATE row
-- lock is taken. Steady-state production workloads with mostly-stable package
-- catalogues see ~95% of upsert calls become no-ops, drastically reducing
-- WAL volume, vacuum pressure, AND the lock-conflict surface that produced
-- the deadlocks.
--
-- The UNION ALL fallback returns (id, name) for input rows that did NOT fire
-- DO UPDATE (i.e. the values are unchanged), so the caller's name → id map
-- is complete regardless of whether each row was newly inserted, updated, or
-- left unchanged. This is required: BulkInsertHostPackages depends on
-- knowing the package_id for every input package.
--
-- The DO UPDATE intentionally touches only NON-KEY columns
-- (description / category / latest_version / updated_at). The row lock taken
-- by ON CONFLICT is therefore FOR NO KEY UPDATE, which does NOT conflict
-- with FOR KEY SHARE locks held by concurrent BulkInsertHostPackages FK
-- checks on the same packages.id rows. Do not extend this DO UPDATE to
-- touch id or name without re-evaluating the lock-compatibility analysis.
WITH upserted AS (
    INSERT INTO packages (id, name, description, category, latest_version, created_at, updated_at)
    SELECT t.id, t.name, t.description, t.category, t.latest_version, NOW(), NOW()
    FROM jsonb_to_recordset(sqlc.arg('payload')::jsonb)
        AS t(id text, name text, description text, category text, latest_version text)
    ORDER BY t.name
    ON CONFLICT (name) DO UPDATE SET
        latest_version = COALESCE(EXCLUDED.latest_version, packages.latest_version),
        description    = COALESCE(EXCLUDED.description,    packages.description),
        category       = COALESCE(EXCLUDED.category,       packages.category),
        updated_at     = NOW()
    WHERE
           packages.latest_version IS DISTINCT FROM COALESCE(EXCLUDED.latest_version, packages.latest_version)
        OR packages.description    IS DISTINCT FROM COALESCE(EXCLUDED.description,    packages.description)
        OR packages.category       IS DISTINCT FROM COALESCE(EXCLUDED.category,       packages.category)
    RETURNING id, name
)
SELECT id, name FROM upserted
UNION ALL
-- Fallback: input rows whose values matched the existing row exactly (so the
-- skip-no-op WHERE made the DO UPDATE a no-op and RETURNING omitted them).
-- Re-parsing the JSON here is cheap (kilobytes, in-memory) and avoids a
-- per-row round-trip — the alternative was a SELECT loop in Go.
SELECT p.id, p.name
FROM packages p
JOIN jsonb_to_recordset(sqlc.arg('payload')::jsonb)
    AS i(name text) USING (name)
WHERE NOT EXISTS (SELECT 1 FROM upserted u WHERE u.name = i.name);

-- name: BulkInsertHostPackages :exec
-- Bulk insert N host_packages rows in a single statement. Replaces the legacy
-- InsertHostPackage and InsertHostPackageWithWUA per-row inserts — one path
-- now covers Linux/FreeBSD (all wua_* columns NULL) and Windows (wua_* set).
--
-- DeleteHostPackagesByHostID has already cleared this host's rows, so no
-- ON CONFLICT clause is needed: the (host_id, package_id) UNIQUE constraint
-- cannot fire.
--
-- Caller pre-sorts by package_id (defensive — host_packages rows for one host
-- cannot cross-host-deadlock because of host-partitioning, but consistent
-- ordering is cheap insurance and aids index locality on (host_id, package_id)).
--
-- Input shape: a single jsonb array of objects with one key per column.
--
-- We use `jsonb_to_recordset` with a typed column list rather than
-- `jsonb_array_elements + elem->>'col'` per column. The recordset variant
-- parses each input element ONCE into a typed tuple; the array_elements
-- variant re-enters the JSON parser for each `->>` operator (15 columns ×
-- per-row = 15 lookups/row). Benchmarks at 10k packages show roughly a
-- 3x decode speedup, which matters for the report-heavy workload at 100+
-- hosts. JSON nulls become SQL NULLs directly so callers do not need
-- empty-string sentinels — but the Go side still emits empty strings for
-- text fields, which the NULLIF wrappers below collapse to NULL. Both
-- conventions remain valid; do not change one without the other.
--
-- wua_categories is stored as jsonb and is declared as a jsonb column in
-- the recordset spec, so it round-trips as a real JSON value (array or
-- null) without going through text serialisation.
INSERT INTO host_packages (
    id, host_id, package_id, current_version, available_version,
    needs_update, is_security_update, source_repository_id,
    wua_guid, wua_kb, wua_severity, wua_categories,
    wua_description, wua_support_url, wua_revision_number,
    last_checked
)
SELECT
    id, host_id, package_id, current_version,
    NULLIF(available_version, '')        AS available_version,
    needs_update, is_security_update,
    NULLIF(source_repository_id, '')     AS source_repository_id,
    NULLIF(wua_guid, '')                 AS wua_guid,
    NULLIF(wua_kb, '')                   AS wua_kb,
    NULLIF(wua_severity, '')             AS wua_severity,
    wua_categories,
    NULLIF(wua_description, '')          AS wua_description,
    NULLIF(wua_support_url, '')          AS wua_support_url,
    NULLIF(wua_revision_number, 0)       AS wua_revision_number,
    NOW()
FROM jsonb_to_recordset(sqlc.arg('payload')::jsonb)
    AS t(
        id text, host_id text, package_id text, current_version text,
        available_version text,
        needs_update boolean, is_security_update boolean,
        source_repository_id text,
        wua_guid text, wua_kb text, wua_severity text,
        wua_categories jsonb,
        wua_description text, wua_support_url text,
        wua_revision_number int
    )
ORDER BY package_id;

-- name: InsertUpdateHistory :exec
INSERT INTO update_history (id, host_id, packages_count, security_count, total_packages, payload_size_kb, execution_time, timestamp, status, error_message)
VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), $8, $9);
