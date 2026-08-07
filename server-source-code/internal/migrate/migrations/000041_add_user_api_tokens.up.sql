-- User-owned long-lived API tokens for automation/scripting
CREATE TABLE user_api_tokens (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    token_hash  TEXT NOT NULL UNIQUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at  TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ
);

CREATE INDEX user_api_tokens_user_id_idx    ON user_api_tokens(user_id);
CREATE INDEX user_api_tokens_token_hash_idx ON user_api_tokens(token_hash);
