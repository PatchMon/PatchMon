// Package pgtime provides helpers for converting time.Time values into
// pgx's pgtype.Timestamp, always normalizing to UTC.
//
// # Why this package exists
//
// pgx v5's encoder for pgtype.Timestamp (which targets Postgres
// "TIMESTAMP WITHOUT TIME ZONE" columns) uses the Time's wall-clock
// components verbatim — see discardTimeZone in
// github.com/jackc/pgx/v5/pgtype/timestamp.go. A time.Time in a non-UTC
// location is therefore persisted as local-time-masquerading-as-UTC, then
// read back tagged as UTC, and rendered shifted by the zone offset on the
// frontend. When the server runs with e.g. TZ=Europe/London this produces
// a visible +1h drift on any stored timestamp.
//
// All time.Time -> pgtype.Timestamp conversions in this codebase should
// route through this package so storage is always real UTC regardless of
// time.Local. Retrieval-side formatting (pgTime / pgTimeToISO) already
// treats values as UTC, so storing UTC closes the loop.
package pgtime

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// From returns a valid pgtype.Timestamp with t normalized to UTC.
func From(t time.Time) pgtype.Timestamp {
	return pgtype.Timestamp{Time: t.UTC(), Valid: true}
}

// FromPtr returns an invalid pgtype.Timestamp when t is nil, otherwise a
// valid one with *t normalized to UTC.
func FromPtr(t *time.Time) pgtype.Timestamp {
	if t == nil {
		return pgtype.Timestamp{}
	}
	return pgtype.Timestamp{Time: t.UTC(), Valid: true}
}

// Now returns a valid pgtype.Timestamp for the current UTC instant.
func Now() pgtype.Timestamp {
	return pgtype.Timestamp{Time: time.Now().UTC(), Valid: true}
}
