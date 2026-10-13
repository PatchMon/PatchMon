ALTER TABLE docker_containers DROP COLUMN IF EXISTS image_digest;
ALTER TABLE docker_containers DROP COLUMN IF EXISTS restart_count;
ALTER TABLE docker_containers DROP COLUMN IF EXISTS image_stale;
DROP TABLE IF EXISTS docker_host_images;
DROP TABLE IF EXISTS docker_image_checks;
