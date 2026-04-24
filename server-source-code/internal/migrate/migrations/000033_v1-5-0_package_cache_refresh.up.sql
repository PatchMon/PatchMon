ALTER TABLE settings ADD COLUMN IF NOT EXISTS package_cache_refresh_mode TEXT NOT NULL DEFAULT 'always';
ALTER TABLE settings ADD COLUMN IF NOT EXISTS package_cache_refresh_max_age INTEGER NOT NULL DEFAULT 60;
