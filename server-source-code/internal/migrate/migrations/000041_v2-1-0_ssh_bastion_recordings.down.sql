DROP TABLE IF EXISTS ssh_recording_access_audit;
DROP TABLE IF EXISTS ssh_sessions;
DROP TABLE IF EXISTS ssh_host_accounts;
ALTER TABLE role_permissions
    DROP COLUMN IF EXISTS can_view_session_recordings;
