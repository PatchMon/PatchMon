ALTER TABLE role_permissions
    ADD COLUMN IF NOT EXISTS can_view_session_recordings BOOLEAN NOT NULL DEFAULT false;

UPDATE role_permissions
SET can_view_session_recordings = true
WHERE role IN ('admin', 'superadmin');

CREATE TABLE IF NOT EXISTS ssh_host_accounts (
    id TEXT PRIMARY KEY,
    host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    linux_username TEXT NOT NULL,
    allow_sudo BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (host_id, linux_username),
    CHECK (linux_username ~ '^[a-z_][a-z0-9_-]{0,31}$'),
    CHECK (linux_username <> 'root')
);

CREATE TABLE IF NOT EXISTS ssh_sessions (
    id TEXT PRIMARY KEY,
    host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    linux_username TEXT NOT NULL,
    transport TEXT NOT NULL,
    status TEXT NOT NULL,
    client_ip TEXT,
    user_agent TEXT,
    started_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ended_at TIMESTAMP(3),
    duration_ms BIGINT,
    failure_reason TEXT,
    recorded BOOLEAN NOT NULL DEFAULT false,
    event_count BIGINT NOT NULL DEFAULT 0,
    recording_bytes BIGINT NOT NULL DEFAULT 0,
    recording_deleted_at TIMESTAMP(3),
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (transport IN ('bastion', 'web', 'tunnel')),
    CHECK (status IN ('opening', 'active', 'completed', 'failed', 'disconnected'))
);

CREATE INDEX IF NOT EXISTS ssh_sessions_host_started_idx
    ON ssh_sessions(host_id, started_at DESC);
CREATE INDEX IF NOT EXISTS ssh_sessions_user_started_idx
    ON ssh_sessions(user_id, started_at DESC);
CREATE INDEX IF NOT EXISTS ssh_sessions_linux_user_started_idx
    ON ssh_sessions(linux_username, started_at DESC);
CREATE INDEX IF NOT EXISTS ssh_sessions_status_started_idx
    ON ssh_sessions(status, started_at DESC);

CREATE TABLE IF NOT EXISTS ssh_recording_access_audit (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES ssh_sessions(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    action TEXT NOT NULL,
    client_ip TEXT,
    created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (action IN ('view', 'search', 'delete', 'purge'))
);

CREATE INDEX IF NOT EXISTS ssh_recording_access_session_idx
    ON ssh_recording_access_audit(session_id, created_at DESC);
