package store

import (
	"strconv"
	"strings"
)

// AlertListParams bundles the filter, sort and pagination inputs for the
// Alerts list page. A zero Limit means "no LIMIT" — the unbounded path the
// overview widgets and the scheduled report renderer still rely on.
type AlertListParams struct {
	Page      int
	Limit     int
	Search    string
	Severity  string
	Type      string
	Status    string
	SortBy    string
	SortOrder string
	// Assignment is either "assigned", "unassigned", or a user id. Empty
	// means no assignment filter.
	Assignment string
}

// Assignment filter values that select a group rather than a single user.
const (
	assignmentAssigned   = "assigned"
	assignmentUnassigned = "unassigned"
)

// alertsListSortColumn maps the public sort key (as sent by the frontend) to
// the expression the ORDER BY uses. Whitelisted so it is safe to interpolate
// into the query string — anything unknown falls back to created_at.
//
// severity_rank is an output column of the SELECT below; Postgres allows an
// output alias in ORDER BY, which keeps the CASE expression in one place.
var alertsListSortColumn = map[string]string{
	"created_at": "a.created_at",
	"severity":   "severity_rank",
	"type":       "a.type",
	"title":      "a.title",
}

// severityRankSQL ranks severities so "sort by severity" orders by how bad
// the alert is rather than alphabetically. Mirrors the ordering the frontend
// previously applied client-side.
const severityRankSQL = `CASE LOWER(a.severity)
        WHEN 'critical' THEN 4
        WHEN 'error' THEN 3
        WHEN 'warning' THEN 2
        WHEN 'informational' THEN 1
        ELSE 0
    END`

// resolveAlertsListSort returns the ORDER BY clause for the requested sort
// key and direction, with a stable id tiebreaker so rows do not shuffle
// between pages when the sort column has duplicates.
func resolveAlertsListSort(sortBy, sortOrder string) string {
	col, ok := alertsListSortColumn[sortBy]
	if !ok {
		col = "a.created_at"
	}
	dir := "DESC"
	if strings.EqualFold(sortOrder, "asc") {
		dir = "ASC"
	}
	return col + " " + dir + ", a.id ASC"
}

// alertsListWhereSQL builds the WHERE body plus its positional args. Filters
// that are unset emit no SQL at all, so the planner sees only the active
// predicates. The returned needsHistory flag tells the caller whether the
// predicate references the latest-history LATERAL (only the status filter
// does), so the COUNT query can skip that join when it is not needed.
func alertsListWhereSQL(p AlertListParams) (where string, args []any, needsHistory bool) {
	var b strings.Builder
	b.WriteString("1=1")
	addArg := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}

	if p.Search != "" {
		// title and message both have gin_trgm_ops indexes (migration
		// 000045) so the leading-wildcard ILIKE stays index-served. `type`
		// is matched too because the old client-side search did.
		pat := addArg("%" + p.Search + "%")
		b.WriteString(" AND (a.title ILIKE ")
		b.WriteString(pat)
		b.WriteString(" OR a.message ILIKE ")
		b.WriteString(pat)
		b.WriteString(" OR a.type ILIKE ")
		b.WriteString(pat)
		b.WriteByte(')')
	}

	if p.Severity != "" {
		sev := addArg(strings.ToLower(p.Severity))
		b.WriteString(" AND LOWER(a.severity) = ")
		b.WriteString(sev)
	}

	if p.Type != "" {
		t := addArg(p.Type)
		b.WriteString(" AND a.type = ")
		b.WriteString(t)
	}

	switch p.Assignment {
	case "":
		// No assignment filter.
	case assignmentAssigned:
		b.WriteString(" AND a.assigned_to_user_id IS NOT NULL")
	case assignmentUnassigned:
		b.WriteString(" AND a.assigned_to_user_id IS NULL")
	default:
		u := addArg(p.Assignment)
		b.WriteString(" AND a.assigned_to_user_id = ")
		b.WriteString(u)
	}

	if p.Status != "" {
		needsHistory = true
		if p.Status == "open" {
			// Open means "no terminal action recorded yet" — an alert with
			// no history at all is open, matching the UI's Open badge.
			b.WriteString(" AND (h.action IS NULL OR LOWER(h.action) NOT IN ('done', 'resolved'))")
		} else {
			st := addArg(strings.ToLower(p.Status))
			b.WriteString(" AND LOWER(h.action) = ")
			b.WriteString(st)
		}
	}

	return b.String(), args, needsHistory
}

// latestHistoryJoinSQL resolves the most recent alert_history row per alert.
// Equivalent to the DISTINCT ON query it replaces, but as a LATERAL it can
// stop at the first row per alert using
// idx_alert_history_alert_id_created_at instead of sorting the whole table.
const latestHistoryJoinSQL = `
LEFT JOIN LATERAL (
    SELECT ah.action, ah.created_at, ah.user_id
    FROM alert_history ah
    WHERE ah.alert_id = a.id
    ORDER BY ah.created_at DESC
    LIMIT 1
) h ON TRUE`

// listAlertsSQL builds the parameterised SELECT for the Alerts list. The
// ORDER BY is interpolated from a whitelist (resolveAlertsListSort), never
// from raw user input. A zero limit omits LIMIT/OFFSET entirely for the
// unbounded callers.
func listAlertsSQL(p AlertListParams) (string, []any) {
	where, args, _ := alertsListWhereSQL(p)

	var b strings.Builder
	b.WriteString(`SELECT a.id, a.type, a.severity, a.title, a.message, a.metadata, a.is_active,
       a.assigned_to_user_id, a.resolved_at, a.resolved_by_user_id, a.created_at, a.updated_at,
       u.id, u.username, u.email, u.first_name, u.last_name,
       h.action, h.created_at, hu.id, hu.username, hu.email, hu.first_name, hu.last_name,
       `)
	b.WriteString(severityRankSQL)
	b.WriteString(` AS severity_rank
FROM alerts a
LEFT JOIN users u ON u.id = a.assigned_to_user_id`)
	b.WriteString(latestHistoryJoinSQL)
	b.WriteString(`
LEFT JOIN users hu ON hu.id = h.user_id
WHERE `)
	b.WriteString(where)
	b.WriteString(" ORDER BY ")
	b.WriteString(resolveAlertsListSort(p.SortBy, p.SortOrder))

	if p.Limit > 0 {
		offset := 0
		if p.Page > 1 {
			offset = (p.Page - 1) * p.Limit
		}
		limitP := "$" + strconv.Itoa(len(args)+1)
		offsetP := "$" + strconv.Itoa(len(args)+2)
		args = append(args, p.Limit, offset)
		b.WriteString(" LIMIT ")
		b.WriteString(limitP)
		b.WriteString(" OFFSET ")
		b.WriteString(offsetP)
	}

	return b.String(), args
}

// countAlertsSQL builds the matching COUNT for listAlertsSQL. Same WHERE
// predicate so the total stays consistent with the rows on the page. The
// latest-history LATERAL is only joined when the status filter needs it.
func countAlertsSQL(p AlertListParams) (string, []any) {
	where, args, needsHistory := alertsListWhereSQL(p)

	var b strings.Builder
	b.WriteString("SELECT COUNT(*)::int FROM alerts a")
	if needsHistory {
		b.WriteString(latestHistoryJoinSQL)
	}
	b.WriteString(" WHERE ")
	b.WriteString(where)
	return b.String(), args
}
