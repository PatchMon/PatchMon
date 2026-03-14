-- name: ListComplianceResultsByScan :many
SELECT cr.id, cr.scan_id, cr.rule_id, cr.status, cr.finding, cr.actual, cr.expected, cr.remediation, cr.created_at,
       crules.rule_ref, crules.title, crules.description, crules.rationale, crules.severity, crules.section, crules.remediation as rule_remediation
FROM compliance_results cr
JOIN compliance_rules crules ON crules.id = cr.rule_id
WHERE cr.scan_id = $1
  AND (sqlc.narg('status_filter')::text IS NULL OR cr.status = sqlc.narg('status_filter'))
  AND (sqlc.narg('severity_filter')::text IS NULL OR crules.severity = sqlc.narg('severity_filter'))
ORDER BY
  CASE cr.status
    WHEN 'fail' THEN 1 WHEN 'failed' THEN 1 WHEN 'failure' THEN 1
    WHEN 'warn' THEN 2 WHEN 'warning' THEN 2 WHEN 'warned' THEN 2
    WHEN 'pass' THEN 3 WHEN 'passed' THEN 3
    ELSE 4
  END,
  CASE COALESCE(crules.severity, 'unknown')
    WHEN 'critical' THEN 1 WHEN 'high' THEN 2 WHEN 'medium' THEN 3 WHEN 'low' THEN 4
    ELSE 5
  END;

-- name: CountComplianceResultsByScan :one
SELECT COUNT(*)
FROM compliance_results cr
JOIN compliance_rules crules ON crules.id = cr.rule_id
WHERE cr.scan_id = $1
  AND (sqlc.narg('status_filter')::text IS NULL OR cr.status = sqlc.narg('status_filter'))
  AND (sqlc.narg('severity_filter')::text IS NULL OR crules.severity = sqlc.narg('severity_filter'));

-- name: GetComplianceResultSeverityBreakdown :many
SELECT crules.severity, COUNT(*)::int as count
FROM compliance_results cr
JOIN compliance_rules crules ON crules.id = cr.rule_id
WHERE cr.scan_id = $1 AND cr.status IN ('fail', 'failed', 'failure')
GROUP BY crules.severity;

-- name: GetComplianceResultSeverityBreakdownForScans :many
SELECT COALESCE(crules.severity, 'unknown')::text as severity, COUNT(*)::int as count
FROM compliance_results cr
JOIN compliance_rules crules ON crules.id = cr.rule_id
WHERE cr.scan_id = ANY($1::text[]) AND cr.status IN ('fail', 'failed', 'failure')
GROUP BY crules.severity;

-- name: GetComplianceResultStatusBreakdown :many
SELECT cr.status, COUNT(*)::int as count
FROM compliance_results cr
WHERE cr.scan_id = $1
GROUP BY cr.status;

-- name: CreateComplianceResult :one
INSERT INTO compliance_results (id, scan_id, rule_id, status, finding, actual, expected, remediation, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
RETURNING id, scan_id, rule_id, status, finding, actual, expected, remediation, created_at;

-- name: DeleteComplianceResultsByScan :exec
DELETE FROM compliance_results WHERE scan_id = $1;

-- name: GetComplianceResultsForRuleFromScans :many
SELECT cr.status, cr.finding, cr.actual, cr.expected, cr.remediation,
       cs.host_id, cs.completed_at,
       h.hostname, h.friendly_name, h.ip
FROM compliance_results cr
JOIN compliance_scans cs ON cs.id = cr.scan_id
JOIN hosts h ON h.id = cs.host_id
WHERE cr.rule_id = $1 AND cs.id = ANY($2::text[]);
