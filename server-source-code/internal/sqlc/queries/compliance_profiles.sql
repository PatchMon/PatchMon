-- name: ListComplianceProfiles :many
SELECT id, name, type, os_family, version, description, created_at, updated_at
FROM compliance_profiles
ORDER BY name ASC;

-- name: GetComplianceProfileByName :one
SELECT id, name, type, os_family, version, description, created_at, updated_at
FROM compliance_profiles
WHERE name = $1;

-- name: GetComplianceProfileByID :one
SELECT id, name, type, os_family, version, description, created_at, updated_at
FROM compliance_profiles
WHERE id = $1;

-- name: GetFirstComplianceProfileByType :one
SELECT id, name, type, os_family, version, description, created_at, updated_at
FROM compliance_profiles
WHERE type = $1
ORDER BY name ASC
LIMIT 1;

-- name: UpsertComplianceProfile :one
-- Single-statement get-or-create against the UNIQUE(name) constraint. The
-- previous SELECT-then-INSERT was both a TOCTOU race and, when called from
-- inside SubmitScan's transaction, a second pool checkout while the first
-- connection was pinned.
--
-- DO UPDATE (rather than DO NOTHING) so the row is always returned; type is
-- only overwritten when a non-empty value is supplied.
INSERT INTO compliance_profiles (id, name, type, os_family, version, description, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
-- The conflict branch deliberately does NOT touch type. An existing profile
-- keeps the type it was created with, and the submitted type is ignored, which
-- is exactly what the SELECT-then-INSERT it replaced did.
--
-- Overwriting it here would be a behaviour change with teeth: callers default
-- an empty submitted type to "openscap" before calling, so a Docker Bench
-- profile scanned by an agent that omits ProfileType would be rewritten to
-- "openscap". That flips which toggle gates it in SubmitScan
-- (openscapEnabled vs dockerBenchEnabled) and corrupts the stored row for
-- every future scan.
ON CONFLICT (name) DO UPDATE SET
    updated_at = NOW()
RETURNING id, name, type, os_family, version, description, created_at, updated_at;

-- name: CreateComplianceProfile :one
INSERT INTO compliance_profiles (id, name, type, os_family, version, description, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
RETURNING id, name, type, os_family, version, description, created_at, updated_at;
