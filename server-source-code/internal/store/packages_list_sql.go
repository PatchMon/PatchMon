package store

import (
	"context"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// listPackagesRow is the decoded row shape returned by runListPackages.
// Mirrors the previously sqlc-generated db.ListPackagesRow so the store
// call site rendering loop did not need to change. status_rank is used
// by the SQL for ORDER BY only; it is not surfaced to callers.
type listPackagesRow struct {
	ID              string
	Name            string
	Description     *string
	Category        *string
	LatestVersion   *string
	CreatedAt       pgtype.Timestamp
	TotalInstalls   int32
	UpdatesNeeded   int32
	SecurityUpdates int32
}

// packagesListSortColumn maps the public sort_key value (as documented in
// the API and used by the frontend) to the column expression that drives
// the SQL ORDER BY. Whitelisted to make this safe to interpolate into the
// query string — anything not in the map falls back to `name`.
//
// The bare `name` and `id` clauses appended after every sort act as the
// stable tiebreaker, so the keyset stays deterministic across pages even
// when the chosen sort column has duplicates (e.g. many packages share
// status_rank=1).
var packagesListSortColumn = map[string]string{
	"name":          "name",
	"latestVersion": "latest_version",
	"packageHosts":  "total_installs",
	"status":        "status_rank",
}

// nullsTrailingOnSortKey marks which whitelisted sort keys can contain
// NULLs and need an explicit NULLS LAST treatment. `latest_version` is
// the only nullable sort target; `total_installs` and `status_rank` are
// COALESCE-zeroed in the SELECT, and `name` is non-null UNIQUE.
var nullsTrailingOnSortKey = map[string]bool{
	"latestVersion": true,
}

// resolvePackagesListSort returns the safe ORDER BY clause for the
// requested sort key + direction. Both inputs are validated against the
// whitelist so the caller never injects raw user input into the SQL.
// The returned clause already includes the (name, id) tiebreaker.
func resolvePackagesListSort(sortKey, sortDir string) string {
	col, ok := packagesListSortColumn[sortKey]
	if !ok {
		col = "name"
		sortKey = "name"
	}
	dir := "ASC"
	if strings.EqualFold(sortDir, "desc") {
		dir = "DESC"
	}
	var b strings.Builder
	b.WriteString(col)
	b.WriteByte(' ')
	b.WriteString(dir)
	if nullsTrailingOnSortKey[sortKey] {
		b.WriteString(" NULLS LAST")
	}
	// Stable tiebreaker. `name` is UNIQUE so adding it once is enough,
	// but appending `id` as well keeps the contract identical to what
	// the previous CASE-WHEN ORDER BY guaranteed.
	if col != "name" {
		b.WriteString(", name ASC")
	}
	b.WriteString(", id ASC")
	return b.String()
}

// packagesListFilters bundles the optional filter inputs for the Packages
// list page. Mirrors the previous sqlc param shape so the store call
// site only had to swap the function call.
type packagesListFilters struct {
	Search           *string
	Category         *string
	HostID           *string
	NeedsUpdate      *string
	IsSecurityUpdate *string
	RepositoryID     *string
}

// packagesListWhereSQL builds the WHERE predicate plus the matching args
// slice for ListPackages / CountPackages. Filters that are unset emit no
// SQL at all, so the planner sees a tight predicate instead of a wall of
// `$N IS NULL OR ...` guards.
//
// `tableAlias` is the alias of the `packages` table in the caller query
// (e.g. `p` for both ListPackages and CountPackages).
//
// Returns:
//   - where: the body that goes after `WHERE` (always non-empty: starts
//     with `1=1` so the caller can simply `WHERE ` + where).
//   - args: the positional arg slice, indexed from 1.
func packagesListWhereSQL(filters packagesListFilters, tableAlias string) (string, []any) {
	var (
		b    strings.Builder
		args []any
	)
	b.WriteString("1=1")
	addArg := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}

	// Text search across name + description. Both columns have a
	// trigram (gin_trgm_ops) index — packages.name from the v1.5.0 init
	// (idx_packages_name_trgm) and packages.description from
	// migration 000043 (idx_packages_description_trgm) — so the OR
	// predicate is index-served end-to-end.
	if filters.Search != nil && *filters.Search != "" {
		p := addArg("%" + *filters.Search + "%")
		b.WriteString(" AND (")
		b.WriteString(tableAlias)
		b.WriteString(".name ILIKE ")
		b.WriteString(p)
		b.WriteString(" OR ")
		b.WriteString(tableAlias)
		b.WriteString(".description ILIKE ")
		b.WriteString(p)
		b.WriteByte(')')
	}

	if filters.Category != nil && *filters.Category != "" {
		p := addArg(*filters.Category)
		b.WriteString(" AND ")
		b.WriteString(tableAlias)
		b.WriteString(".category = ")
		b.WriteString(p)
	}

	// is_security_update='false' means "exclude packages that have any
	// host with needs_update + is_security_update" — the inverse of the
	// EXISTS branch below. Emitted as a NOT EXISTS so the planner can
	// use idx_host_packages_needs_update_security_package (partial on
	// package_id WHERE needs_update AND is_security_update).
	if filters.IsSecurityUpdate != nil && *filters.IsSecurityUpdate == "false" {
		b.WriteString(" AND NOT EXISTS (SELECT 1 FROM host_packages hps WHERE hps.package_id = ")
		b.WriteString(tableAlias)
		b.WriteString(".id AND hps.needs_update = true AND hps.is_security_update = true)")
	}

	// EXISTS branch: only emit when at least one of the host_packages
	// filters is active. When none are set, the planner sees no
	// host_packages join at all — it just walks `packages` (filtered by
	// the lighter predicates above) and joins mv_package_stats for the
	// per-package counters. This is the fast path for the default
	// Packages page render.
	//
	// For is_security_update='false' the previous SQL required BOTH the
	// NOT EXISTS above (no security-update row) AND an EXISTS row with
	// needs_update=true AND is_security_update=false (i.e. at least one
	// non-security pending update). Preserving the second half here so
	// the result set is identical: a package with zero pending updates
	// is excluded by the 'false' filter, matching the original query.
	hostExistsActive := (filters.HostID != nil && *filters.HostID != "") ||
		(filters.NeedsUpdate != nil && *filters.NeedsUpdate == "true") ||
		(filters.IsSecurityUpdate != nil && (*filters.IsSecurityUpdate == "true" || *filters.IsSecurityUpdate == "false")) ||
		(filters.RepositoryID != nil && *filters.RepositoryID != "")

	if hostExistsActive {
		b.WriteString(" AND EXISTS (SELECT 1 FROM host_packages hp WHERE hp.package_id = ")
		b.WriteString(tableAlias)
		b.WriteString(".id")
		if filters.HostID != nil && *filters.HostID != "" {
			p := addArg(*filters.HostID)
			b.WriteString(" AND hp.host_id = ")
			b.WriteString(p)
		}
		// needs_update='true' is the only meaningful value (matches the
		// previous SQL which only honoured 'true').
		if filters.NeedsUpdate != nil && *filters.NeedsUpdate == "true" {
			b.WriteString(" AND hp.needs_update = true")
		}
		// is_security_update='true' implies the row needs an update *and*
		// is flagged security. is_security_update='false' implies the
		// row needs an update but is NOT security. Same shape as the
		// previous query's inner EXISTS predicate.
		if filters.IsSecurityUpdate != nil && *filters.IsSecurityUpdate == "true" {
			b.WriteString(" AND hp.needs_update = true AND hp.is_security_update = true")
		}
		if filters.IsSecurityUpdate != nil && *filters.IsSecurityUpdate == "false" {
			b.WriteString(" AND hp.needs_update = true AND hp.is_security_update = false")
		}
		if filters.RepositoryID != nil && *filters.RepositoryID != "" {
			p := addArg(*filters.RepositoryID)
			b.WriteString(" AND hp.source_repository_id = ")
			b.WriteString(p)
		}
		b.WriteByte(')')
	}

	return b.String(), args
}

// listPackagesSQL builds the parameterised SELECT for the Packages list
// page. The ORDER BY column / direction is interpolated from a whitelist
// (resolvePackagesListSort) — never raw user input — so the planner can
// drive output from an indexed column scan + LIMIT pushdown for the
// common `name ASC` case rather than a full sort over the filtered set.
func listPackagesSQL(filters packagesListFilters, sortKey, sortDir string, limit, offset int32) (string, []any) {
	where, args := packagesListWhereSQL(filters, "p")
	orderBy := resolvePackagesListSort(sortKey, sortDir)
	limitP := "$" + strconv.Itoa(len(args)+1)
	offsetP := "$" + strconv.Itoa(len(args)+2)
	args = append(args, limit, offset)

	// We deliberately do not wrap this in a CTE. With the per-sort-key
	// whitelisted ORDER BY column, the planner can either:
	//   - For sort=name ASC: walk the implicit btree on packages.name
	//     UNIQUE in order, hash-join mv_package_stats, stop at LIMIT.
	//     No top-level Sort node, no Gather, no parallel workers — the
	//     ones that previously blew Docker's default 64 MB /dev/shm.
	//   - For sort=total_installs / status_rank: an in-memory sort over
	//     the filtered set; with the EXISTS clause only emitted when a
	//     host filter is active, this stays well within work_mem.
	var b strings.Builder
	b.WriteString(`SELECT p.id, p.name, p.description, p.category, p.latest_version, p.created_at,
       COALESCE(s.total_installs, 0)::int   AS total_installs,
       COALESCE(s.updates_needed, 0)::int   AS updates_needed,
       COALESCE(s.security_updates, 0)::int AS security_updates,
       CASE
           WHEN COALESCE(s.security_updates, 0) > 0 THEN 0
           WHEN COALESCE(s.updates_needed, 0) > 0   THEN 1
           ELSE 2
       END AS status_rank
FROM packages p
LEFT JOIN mv_package_stats s ON s.package_id = p.id
WHERE `)
	b.WriteString(where)
	b.WriteString(" ORDER BY ")
	b.WriteString(orderBy)
	b.WriteString(" LIMIT ")
	b.WriteString(limitP)
	b.WriteString(" OFFSET ")
	b.WriteString(offsetP)
	return b.String(), args
}

// countPackagesSQL builds the matching COUNT query. Exactly the same
// WHERE predicate as listPackagesSQL so totals stay consistent with the
// page rows. The SUM aggregates mv_package_stats.total_installs over the
// whole filtered set (not the current page) for the Installations card.
// mv_package_stats has a UNIQUE index on package_id, so the LEFT JOIN
// cannot fan out the COUNT.
func countPackagesSQL(filters packagesListFilters) (string, []any) {
	where, args := packagesListWhereSQL(filters, "p")
	return `SELECT COUNT(*)::int,
       COALESCE(SUM(COALESCE(s.total_installs, 0)), 0)::bigint
FROM packages p
LEFT JOIN mv_package_stats s ON s.package_id = p.id
WHERE ` + where, args
}

// runListPackages executes the dynamic SELECT inside the supplied
// transaction (the same one the work_mem bump applies to). Mirrors the
// contract of the old q.ListPackages: returns rows in store order and
// propagates any pgx error untouched.
func runListPackages(ctx context.Context, tx pgx.Tx, filters packagesListFilters, sortKey, sortDir string, limit, offset int32) ([]listPackagesRow, error) {
	sqlText, args := listPackagesSQL(filters, sortKey, sortDir, limit, offset)
	rows, err := tx.Query(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []listPackagesRow
	for rows.Next() {
		var r listPackagesRow
		// status_rank is computed for ORDER BY only — scan into a
		// throwaway int so the column count matches.
		var statusRank int32
		var createdAt pgtype.Timestamp
		if err := rows.Scan(
			&r.ID,
			&r.Name,
			&r.Description,
			&r.Category,
			&r.LatestVersion,
			&createdAt,
			&r.TotalInstalls,
			&r.UpdatesNeeded,
			&r.SecurityUpdates,
			&statusRank,
		); err != nil {
			return nil, err
		}
		r.CreatedAt = createdAt
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// runCountPackages executes the matching COUNT inside the supplied tx and
// returns the matching package count plus the total installs across all of
// them.
func runCountPackages(ctx context.Context, tx pgx.Tx, filters packagesListFilters) (int32, int64, error) {
	sqlText, args := countPackagesSQL(filters)
	var total int32
	var totalInstalls int64
	if err := tx.QueryRow(ctx, sqlText, args...).Scan(&total, &totalInstalls); err != nil {
		return 0, 0, err
	}
	return total, totalInstalls, nil
}
