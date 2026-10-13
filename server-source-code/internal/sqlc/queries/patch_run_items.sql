-- name: InsertPatchRunItems :exec
INSERT INTO patch_run_items (
    id, patch_run_id, host_id, kind, identifier, revision_number, title, kb, status,
    created_at, updated_at
)
SELECT
    gen_random_uuid()::text, sqlc.arg('patch_run_id'), sqlc.arg('host_id'),
    kind, identifier, NULLIF(revision_number, 0), title, NULLIF(kb, ''), 'pending',
    NOW(), NOW()
FROM jsonb_to_recordset(sqlc.arg('payload')::jsonb)
    AS t(kind text, identifier text, revision_number int, title text, kb text);

-- name: ListPatchRunItems :many
SELECT * FROM patch_run_items
WHERE patch_run_id = $1
ORDER BY kind DESC, title ASC, identifier ASC;

-- name: ListPendingPatchRunItems :many
SELECT * FROM patch_run_items
WHERE patch_run_id = $1 AND status = 'pending'
ORDER BY kind DESC, title ASC, identifier ASC;

-- name: ListPatchRunItemKeys :many
SELECT kind, identifier FROM patch_run_items WHERE patch_run_id = $1;

-- name: UpdatePatchRunItemsFromAgent :execrows
UPDATE patch_run_items i SET
    revision_number = COALESCE(NULLIF(t.revision_number, 0), i.revision_number),
    kb              = COALESCE(NULLIF(t.kb, ''), i.kb),
    status          = t.status,
    result_code     = t.result_code,
    hresult         = NULLIF(t.hresult, ''),
    exit_code       = t.exit_code,
    reboot_required = COALESCE(t.reboot_required, false),
    download_bytes  = t.download_bytes,
    message         = NULLIF(t.message, ''),
    started_at      = COALESCE(t.started_at AT TIME ZONE 'UTC', i.started_at),
    completed_at    = COALESCE(t.completed_at AT TIME ZONE 'UTC', i.completed_at),
    updated_at      = NOW()
FROM jsonb_to_recordset(sqlc.arg('payload')::jsonb)
    AS t(kind text, identifier text, revision_number int, title text, kb text, status text,
         result_code int, hresult text, exit_code bigint, reboot_required boolean,
         download_bytes bigint, message text, started_at timestamptz, completed_at timestamptz)
WHERE i.patch_run_id = sqlc.arg('patch_run_id')
  AND i.kind = t.kind
  AND i.identifier = t.identifier;

-- name: UpsertPatchRunItemsFromAgent :exec
INSERT INTO patch_run_items (
    id, patch_run_id, host_id, kind, identifier, revision_number, title, kb, status,
    result_code, hresult, exit_code, reboot_required, download_bytes, message,
    started_at, completed_at, created_at, updated_at
)
SELECT
    gen_random_uuid()::text, sqlc.arg('patch_run_id'), sqlc.arg('host_id'),
    t.kind, t.identifier, NULLIF(t.revision_number, 0), COALESCE(NULLIF(t.title, ''), t.identifier),
    NULLIF(t.kb, ''), t.status, t.result_code, NULLIF(t.hresult, ''), t.exit_code,
    COALESCE(t.reboot_required, false), t.download_bytes, NULLIF(t.message, ''),
    t.started_at AT TIME ZONE 'UTC', t.completed_at AT TIME ZONE 'UTC', NOW(), NOW()
FROM jsonb_to_recordset(sqlc.arg('payload')::jsonb)
    AS t(kind text, identifier text, revision_number int, title text, kb text, status text,
         result_code int, hresult text, exit_code bigint, reboot_required boolean,
         download_bytes bigint, message text, started_at timestamptz, completed_at timestamptz)
ON CONFLICT (patch_run_id, kind, identifier) DO UPDATE SET
    revision_number = COALESCE(EXCLUDED.revision_number, patch_run_items.revision_number),
    kb              = COALESCE(EXCLUDED.kb, patch_run_items.kb),
    status          = EXCLUDED.status,
    result_code     = EXCLUDED.result_code,
    hresult         = EXCLUDED.hresult,
    exit_code       = EXCLUDED.exit_code,
    reboot_required = EXCLUDED.reboot_required,
    download_bytes  = EXCLUDED.download_bytes,
    message         = EXCLUDED.message,
    started_at      = COALESCE(EXCLUDED.started_at, patch_run_items.started_at),
    completed_at    = COALESCE(EXCLUDED.completed_at, patch_run_items.completed_at),
    updated_at      = NOW();

-- name: ResolveWindowsPatchTargets :many
SELECT p.name,
    hp.needs_update, hp.wua_guid, hp.wua_revision_number, hp.wua_kb, hp.wua_categories, hp.winget_id,
    (hp.needs_update AND (hp.wua_guid IS NOT NULL OR hp.winget_id IS NOT NULL) AND NOT COALESCE(hp.winget_pinned, false) AND NOT COALESCE(hp.winget_store_app, false))::boolean AS patchable,
    (CASE
        WHEN NOT hp.needs_update THEN 'not_pending'
        WHEN COALESCE(hp.winget_pinned, false) THEN 'winget_pinned'
        WHEN COALESCE(hp.winget_store_app, false) THEN 'store_app'
        WHEN hp.wua_guid IS NULL AND hp.winget_id IS NULL THEN 'no_winget_id'
        ELSE ''
    END)::text AS not_patchable_reason
FROM host_packages hp
JOIN packages p ON p.id = hp.package_id
WHERE hp.host_id = sqlc.arg('host_id')
  AND p.name = ANY(sqlc.arg('names')::text[]);

-- name: DeletePatchRunItems :exec
DELETE FROM patch_run_items WHERE patch_run_id = $1;

-- name: LockPatchRunStatus :one
SELECT status,
    (COALESCE(completed_at, updated_at) > NOW() - make_interval(secs => sqlc.arg('grace_seconds')::int))::boolean AS within_grace
FROM patch_runs WHERE id = sqlc.arg('id') FOR UPDATE;

-- name: AbortPendingPatchRunItems :exec
UPDATE patch_run_items
SET status = 'aborted', message = sqlc.arg('message'), completed_at = NOW(), updated_at = NOW()
WHERE id = ANY(sqlc.arg('ids')::text[]) AND status = 'pending';
