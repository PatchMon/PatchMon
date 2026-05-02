-- Agent Activity feed. Returns the merged time-ordered stream of agent comm
-- rows for a given host: inbound reports (update_history) AND outbound jobs
-- (job_history). The two source tables have different shapes; this query
-- normalises them to one column set and discriminates via the `kind` column
-- ('report' or 'job'). Caller passes the host_id, optional direction / type /
-- status filters, optional search string (ILIKE on error_message and job
-- output), optional time-range / limit / offset for pagination.
--
-- CountAgentActivity returns the filtered total separately so stale/deep-linked
-- pages beyond the end can still report the real total.
--
-- The since_ts arg uses sqlc.narg (nullable) on both branches; pgx encodes
-- pgtype.Timestamp{Valid:false} as SQL NULL, which the IS NULL guard catches.

-- name: CountAgentActivity :one
SELECT COUNT(*)::int FROM (
    SELECT uh.id
    FROM update_history uh
    WHERE uh.host_id = sqlc.arg('host_id')::text
      AND (sqlc.arg('direction')::text = '' OR sqlc.arg('direction')::text = 'in')
      AND (COALESCE(cardinality(sqlc.arg('types')::text[]), 0) = 0 OR uh.report_type = ANY(sqlc.arg('types')::text[]))
      AND (COALESCE(cardinality(sqlc.arg('statuses')::text[]), 0) = 0 OR uh.status = ANY(sqlc.arg('statuses')::text[]))
      AND (sqlc.narg('since_ts')::timestamp IS NULL OR uh.timestamp >= sqlc.narg('since_ts')::timestamp)
      AND (sqlc.arg('search')::text = '' OR (uh.error_message IS NOT NULL AND uh.error_message ILIKE '%' || sqlc.arg('search')::text || '%'))

    UNION ALL

    SELECT jh.id
    FROM job_history jh
    WHERE jh.host_id = sqlc.arg('host_id')::text
      AND (sqlc.arg('direction')::text = '' OR sqlc.arg('direction')::text = 'out')
      AND (COALESCE(cardinality(sqlc.arg('types')::text[]), 0) = 0 OR jh.job_name = ANY(sqlc.arg('types')::text[]))
      AND (COALESCE(cardinality(sqlc.arg('statuses')::text[]), 0) = 0 OR jh.status = ANY(sqlc.arg('statuses')::text[]))
      AND (sqlc.narg('since_ts')::timestamp IS NULL OR jh.created_at >= sqlc.narg('since_ts')::timestamp)
      AND (sqlc.arg('search')::text = ''
           OR (jh.error_message IS NOT NULL AND jh.error_message ILIKE '%' || sqlc.arg('search')::text || '%')
           OR (jh.output IS NOT NULL AND jh.output::text ILIKE '%' || sqlc.arg('search')::text || '%'))
) merged;

-- name: ListAgentActivity :many
SELECT * FROM (
    SELECT
        'report'::text AS kind,
        uh.id AS row_id,
        uh.timestamp AS occurred_at,
        uh.report_type AS type,
        ''::text AS job_id,
        ''::text AS job_name,
        ''::text AS queue_name,
        uh.sections_sent AS sections_sent,
        uh.sections_unchanged AS sections_unchanged,
        uh.payload_size_kb AS payload_size_kb,
        uh.execution_time AS server_processing_ms,
        uh.agent_execution_ms AS agent_execution_ms,
        0::int AS attempt_number,
        uh.status AS status,
        uh.error_message AS error_message,
        uh.packages_count AS packages_count,
        uh.security_count AS security_count,
        NULL::timestamp AS completed_at,
        ''::text AS output
    FROM update_history uh
    WHERE uh.host_id = sqlc.arg('host_id')::text
      AND (sqlc.arg('direction')::text = '' OR sqlc.arg('direction')::text = 'in')
      AND (COALESCE(cardinality(sqlc.arg('types')::text[]), 0) = 0 OR uh.report_type = ANY(sqlc.arg('types')::text[]))
      AND (COALESCE(cardinality(sqlc.arg('statuses')::text[]), 0) = 0 OR uh.status = ANY(sqlc.arg('statuses')::text[]))
      AND (sqlc.narg('since_ts')::timestamp IS NULL OR uh.timestamp >= sqlc.narg('since_ts')::timestamp)
      AND (sqlc.arg('search')::text = '' OR (uh.error_message IS NOT NULL AND uh.error_message ILIKE '%' || sqlc.arg('search')::text || '%'))

    UNION ALL

    SELECT
        'job'::text AS kind,
        jh.id AS row_id,
        jh.created_at AS occurred_at,
        jh.job_name AS type,
        jh.job_id AS job_id,
        jh.job_name AS job_name,
        jh.queue_name AS queue_name,
        ARRAY[]::text[] AS sections_sent,
        ARRAY[]::text[] AS sections_unchanged,
        NULL::double precision AS payload_size_kb,
        CASE
            WHEN jh.completed_at IS NOT NULL
            THEN EXTRACT(EPOCH FROM (jh.completed_at - jh.created_at)) * 1000
            ELSE NULL
        END AS server_processing_ms,
        NULL::int AS agent_execution_ms,
        jh.attempt_number AS attempt_number,
        jh.status AS status,
        jh.error_message AS error_message,
        -- packages_count / security_count are NOT NULL on update_history,
        -- so the report branch dictates a NOT NULL int32 in the generated
        -- row struct. Returning 0 here (instead of NULL) keeps pgx scan
        -- happy; the store layer only surfaces these for kind='report' so
        -- the wire response is unaffected for jobs.
        0::int AS packages_count,
        0::int AS security_count,
        jh.completed_at AS completed_at,
        COALESCE(jh.output::text, '') AS output
    FROM job_history jh
    WHERE jh.host_id = sqlc.arg('host_id')::text
      AND (sqlc.arg('direction')::text = '' OR sqlc.arg('direction')::text = 'out')
      AND (COALESCE(cardinality(sqlc.arg('types')::text[]), 0) = 0 OR jh.job_name = ANY(sqlc.arg('types')::text[]))
      AND (COALESCE(cardinality(sqlc.arg('statuses')::text[]), 0) = 0 OR jh.status = ANY(sqlc.arg('statuses')::text[]))
      AND (sqlc.narg('since_ts')::timestamp IS NULL OR jh.created_at >= sqlc.narg('since_ts')::timestamp)
      AND (sqlc.arg('search')::text = ''
           OR (jh.error_message IS NOT NULL AND jh.error_message ILIKE '%' || sqlc.arg('search')::text || '%')
           OR (jh.output IS NOT NULL AND jh.output::text ILIKE '%' || sqlc.arg('search')::text || '%'))
) merged
ORDER BY occurred_at DESC
LIMIT sqlc.arg('row_limit')::int
OFFSET sqlc.arg('row_offset')::int;

-- name: DeleteOldUpdateHistory :execrows
-- Retention sweep target. Returns the number of rows deleted so the worker
-- can log the volume.
DELETE FROM update_history
WHERE timestamp < (NOW() - (sqlc.arg('retention_days')::int * INTERVAL '1 day'));
