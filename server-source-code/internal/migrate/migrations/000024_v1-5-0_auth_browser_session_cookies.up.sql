ALTER TABLE settings
    ADD COLUMN IF NOT EXISTS auth_browser_session_cookies BOOLEAN;
