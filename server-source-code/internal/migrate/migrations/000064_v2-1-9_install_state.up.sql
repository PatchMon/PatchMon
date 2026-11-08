CREATE TABLE IF NOT EXISTS install_state (
    id INTEGER PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    setup_completed_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO install_state (id, setup_completed_at)
SELECT 1, MIN(created_at) FROM users HAVING COUNT(*) > 0
ON CONFLICT (id) DO NOTHING;
