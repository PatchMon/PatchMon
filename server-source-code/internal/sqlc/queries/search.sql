-- name: GlobalSearch :many
WITH host_results AS (
    SELECT
        id,
        COALESCE(friendly_name, hostname, '') AS name,
        'host'::text AS type,
        COALESCE(os_type || ' ' || os_version, '') AS description
    FROM hosts
    WHERE friendly_name ILIKE '%' || @search::text || '%'
       OR hostname ILIKE '%' || @search::text || '%'
       OR ip ILIKE '%' || @search::text || '%'
       OR notes ILIKE '%' || @search::text || '%'
    ORDER BY last_update DESC NULLS LAST
    LIMIT 5
),
package_results AS (
    SELECT
        id,
        name,
        'package'::text AS type,
        COALESCE(description, category, '') AS description
    FROM packages
    WHERE name ILIKE '%' || @search::text || '%'
       OR description ILIKE '%' || @search::text || '%'
    ORDER BY name ASC
    LIMIT 5
),
repository_results AS (
    SELECT
        id,
        name,
        'repository'::text AS type,
        COALESCE(url, '') AS description
    FROM repositories
    WHERE name ILIKE '%' || @search::text || '%'
       OR url ILIKE '%' || @search::text || '%'
       OR description ILIKE '%' || @search::text || '%'
    ORDER BY name ASC
    LIMIT 5
),
host_group_results AS (
    SELECT
        id,
        name,
        'host_group'::text AS type,
        COALESCE(description, '') AS description
    FROM host_groups
    WHERE name ILIKE '%' || @search::text || '%'
       OR description ILIKE '%' || @search::text || '%'
    ORDER BY name ASC
    LIMIT 5
),
user_results AS (
    SELECT
        id,
        username AS name,
        'user'::text AS type,
        COALESCE(email, '') || CASE WHEN first_name IS NOT NULL OR last_name IS NOT NULL
            THEN ' - ' || COALESCE(first_name, '') || ' ' || COALESCE(last_name, '')
            ELSE '' END AS description
    FROM users
    WHERE username ILIKE '%' || @search::text || '%'
       OR email ILIKE '%' || @search::text || '%'
       OR first_name ILIKE '%' || @search::text || '%'
       OR last_name ILIKE '%' || @search::text || '%'
    ORDER BY username ASC
    LIMIT 5
),
docker_container_results AS (
    SELECT
        id,
        name,
        'docker_container'::text AS type,
        image_name || ':' || image_tag || ' (' || status || ')' AS description
    FROM docker_containers
    WHERE name ILIKE '%' || @search::text || '%'
       OR image_name ILIKE '%' || @search::text || '%'
       OR container_id ILIKE '%' || @search::text || '%'
    ORDER BY name ASC
    LIMIT 5
),
docker_image_results AS (
    SELECT
        id,
        repository || ':' || tag AS name,
        'docker_image'::text AS type,
        COALESCE(source, '') AS description
    FROM docker_images
    WHERE repository ILIKE '%' || @search::text || '%'
       OR tag ILIKE '%' || @search::text || '%'
       OR image_id ILIKE '%' || @search::text || '%'
    ORDER BY repository ASC
    LIMIT 5
)
SELECT * FROM host_results
UNION ALL
SELECT * FROM package_results
UNION ALL
SELECT * FROM repository_results
UNION ALL
SELECT * FROM host_group_results
UNION ALL
SELECT * FROM user_results
UNION ALL
SELECT * FROM docker_container_results
UNION ALL
SELECT * FROM docker_image_results
LIMIT 30;
