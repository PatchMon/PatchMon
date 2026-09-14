-- name: GetComplianceRuleByProfileAndRef :one
SELECT id, profile_id, rule_ref, title, description, rationale, severity, section, remediation
FROM compliance_rules
WHERE profile_id = $1 AND rule_ref = $2;

-- name: ListComplianceRulesByProfile :many
SELECT id, profile_id, rule_ref, title, description, rationale, severity, section, remediation
FROM compliance_rules
WHERE profile_id = $1;

-- name: GetComplianceRuleByID :one
SELECT cr.id, cr.profile_id, cr.rule_ref, cr.title, cr.description, cr.rationale, cr.severity, cr.section, cr.remediation,
       cp.id as profile_id_val, cp.type as profile_type, cp.name as profile_name
FROM compliance_rules cr
JOIN compliance_profiles cp ON cp.id = cr.profile_id
WHERE cr.id = $1;

-- name: CreateComplianceRule :one
INSERT INTO compliance_rules (id, profile_id, rule_ref, title, description, rationale, severity, section, remediation)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id, profile_id, rule_ref, title, description, rationale, severity, section, remediation;

-- name: UpsertComplianceRule :one
-- Metadata columns are COALESCE-guarded so a submission omitting a field does
-- not blank a stored value.
--
-- title reads the raw parameter, not EXCLUDED: the column is NOT NULL so the
-- INSERT arm supplies a rule_ref fallback, which EXCLUDED would already hold.
INSERT INTO compliance_rules (id, profile_id, rule_ref, title, description, rationale, severity, section, remediation)
VALUES (
    sqlc.arg('id'),
    sqlc.arg('profile_id'),
    sqlc.arg('rule_ref'),
    COALESCE(sqlc.narg('title')::text, sqlc.arg('rule_ref')),
    sqlc.narg('description')::text,
    sqlc.narg('rationale')::text,
    sqlc.narg('severity')::text,
    sqlc.narg('section')::text,
    sqlc.narg('remediation')::text
)
ON CONFLICT (profile_id, rule_ref) DO UPDATE SET
    title = COALESCE(sqlc.narg('title')::text, compliance_rules.title),
    description = COALESCE(EXCLUDED.description, compliance_rules.description),
    severity = COALESCE(EXCLUDED.severity, compliance_rules.severity),
    section = COALESCE(EXCLUDED.section, compliance_rules.section),
    remediation = COALESCE(EXCLUDED.remediation, compliance_rules.remediation)
RETURNING id;

-- name: UpdateComplianceRule :exec
UPDATE compliance_rules
SET title = COALESCE(sqlc.narg('title')::text, title),
    description = COALESCE(sqlc.narg('description')::text, description),
    severity = COALESCE(sqlc.narg('severity')::text, severity),
    section = COALESCE(sqlc.narg('section')::text, section),
    remediation = COALESCE(sqlc.narg('remediation')::text, remediation)
WHERE id = sqlc.arg('id');
