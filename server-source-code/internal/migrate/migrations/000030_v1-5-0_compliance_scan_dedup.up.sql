-- Clean up existing duplicate completed scans: keep only the most recent completed
-- scan per (host_id, profile_id) and delete older duplicates.
DELETE FROM compliance_scans
WHERE id IN (
    SELECT id FROM (
        SELECT id,
               ROW_NUMBER() OVER (
                   PARTITION BY host_id, profile_id
                   ORDER BY completed_at DESC NULLS LAST, created_at DESC
               ) AS rn
        FROM compliance_scans
        WHERE status = 'completed'
    ) ranked
    WHERE rn > 1
);

-- Prevent duplicate completed compliance scans for the same host+profile.
-- Only one completed scan per (host_id, profile_id) should exist at a time;
-- older completed scans are deleted before new ones are inserted (handled in Go code).
-- This index serves as a safety net for any race conditions.
CREATE UNIQUE INDEX IF NOT EXISTS idx_compliance_scans_host_profile_completed
ON compliance_scans (host_id, profile_id)
WHERE status = 'completed';
