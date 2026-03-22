package store

import (
	"context"
	"fmt"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
)

// SearchResult represents a single global search result.
type SearchResult struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

// SearchStore provides global search across hosts and packages.
type SearchStore struct {
	db database.DBProvider
}

// NewSearchStore creates a new search store.
func NewSearchStore(db database.DBProvider) *SearchStore {
	return &SearchStore{db: db}
}

// GlobalSearch searches across hosts, packages, repositories, host groups, users, and docker entities.
func (s *SearchStore) GlobalSearch(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	d := s.db.DB(ctx)
	if limit <= 0 {
		limit = 30
	}
	pattern := fmt.Sprintf("%%%s%%", query)

	rows, err := d.Raw(ctx, `
		WITH host_results AS (
			SELECT id, COALESCE(friendly_name, hostname, '') AS name,
				'host'::text AS type,
				COALESCE(os_type || ' ' || os_version, '') AS description
			FROM hosts
			WHERE friendly_name ILIKE $1
				OR hostname ILIKE $1
				OR ip ILIKE $1
				OR notes ILIKE $1
			ORDER BY last_update DESC NULLS LAST
			LIMIT 5
		),
		package_results AS (
			SELECT id, name, 'package'::text AS type,
				COALESCE(description, category, '') AS description
			FROM packages
			WHERE name ILIKE $1
				OR description ILIKE $1
			ORDER BY name ASC
			LIMIT 5
		),
		repository_results AS (
			SELECT id, name, 'repository'::text AS type,
				COALESCE(url, '') AS description
			FROM repositories
			WHERE name ILIKE $1
				OR url ILIKE $1
				OR description ILIKE $1
			ORDER BY name ASC
			LIMIT 5
		),
		host_group_results AS (
			SELECT id, name, 'host_group'::text AS type,
				COALESCE(description, '') AS description
			FROM host_groups
			WHERE name ILIKE $1
				OR description ILIKE $1
			ORDER BY name ASC
			LIMIT 5
		),
		user_results AS (
			SELECT id, username AS name,
				'user'::text AS type,
				COALESCE(email, '') || CASE WHEN first_name IS NOT NULL OR last_name IS NOT NULL
					THEN ' - ' || COALESCE(first_name, '') || ' ' || COALESCE(last_name, '')
					ELSE '' END AS description
			FROM users
			WHERE username ILIKE $1
				OR email ILIKE $1
				OR first_name ILIKE $1
				OR last_name ILIKE $1
			ORDER BY username ASC
			LIMIT 5
		),
		docker_container_results AS (
			SELECT id, name,
				'docker_container'::text AS type,
				image_name || ':' || image_tag || ' (' || status || ')' AS description
			FROM docker_containers
			WHERE name ILIKE $1
				OR image_name ILIKE $1
				OR container_id ILIKE $1
			ORDER BY name ASC
			LIMIT 5
		),
		docker_image_results AS (
			SELECT id, repository || ':' || tag AS name,
				'docker_image'::text AS type,
				COALESCE(source, '') AS description
			FROM docker_images
			WHERE repository ILIKE $1
				OR tag ILIKE $1
				OR image_id ILIKE $1
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
		LIMIT $2
	`, pattern, limit)
	if err != nil {
		return nil, fmt.Errorf("global search: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.ID, &r.Name, &r.Type, &r.Description); err != nil {
			return nil, fmt.Errorf("scan search result: %w", err)
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate search results: %w", err)
	}

	if results == nil {
		results = []SearchResult{}
	}
	return results, nil
}
