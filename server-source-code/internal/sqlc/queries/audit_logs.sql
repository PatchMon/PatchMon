-- name: InsertAuditLog :exec
INSERT INTO audit_logs (
    id, event, user_id, target_user_id, ip_address, user_agent, request_id, details, success, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, CURRENT_TIMESTAMP
);
