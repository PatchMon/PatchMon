CREATE TABLE IF NOT EXISTS docker_image_checks (
    id TEXT PRIMARY KEY,
    host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    image_id TEXT NOT NULL REFERENCES docker_images(id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    local_digest TEXT,
    remote_digest TEXT,
    registry TEXT,
    reason TEXT,
    checked_at TIMESTAMP(3) NOT NULL,
    first_seen_update_at TIMESTAMP(3),
    updated_at TIMESTAMP(3) NOT NULL,
    UNIQUE(host_id, image_id)
);
CREATE INDEX IF NOT EXISTS docker_image_checks_image_id_idx ON docker_image_checks(image_id);
CREATE INDEX IF NOT EXISTS docker_image_checks_status_idx ON docker_image_checks(status);

CREATE TABLE IF NOT EXISTS docker_host_images (
    host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    image_id TEXT NOT NULL REFERENCES docker_images(id) ON DELETE CASCADE,
    last_seen TIMESTAMP(3) NOT NULL,
    PRIMARY KEY(host_id, image_id)
);
CREATE INDEX IF NOT EXISTS docker_host_images_image_id_idx ON docker_host_images(image_id);

ALTER TABLE docker_containers ADD COLUMN IF NOT EXISTS image_stale BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE docker_containers ADD COLUMN IF NOT EXISTS restart_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE docker_containers ADD COLUMN IF NOT EXISTS image_digest TEXT;

DELETE FROM docker_image_updates;
