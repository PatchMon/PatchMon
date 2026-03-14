-- name: InsertReleaseNotesAcceptance :one
INSERT INTO release_notes_acceptances (user_id, version)
VALUES ($1, $2)
RETURNING id;

-- name: GetAcceptedVersionsByUserID :many
SELECT version FROM release_notes_acceptances WHERE user_id = $1;
