package store

import (
	"os"
	"strings"
	"testing"
)

// UpsertComplianceProfile replaced a SELECT-then-INSERT. The replacement must
// preserve one specific behaviour: an existing profile keeps the type it was
// created with, and the submitted type is ignored.
//
// This matters because callers default an empty submitted type to "openscap"
// before calling. If the conflict branch wrote EXCLUDED.type, a Docker Bench
// profile scanned by an agent that omits ProfileType would be rewritten to
// "openscap", which flips which toggle gates it in SubmitScan
// (openscapEnabled vs dockerBenchEnabled) and corrupts the stored row for
// every subsequent scan.
//
// There is no database available in unit tests, so this asserts on the query
// text, which is the thing that actually encodes the semantic.
func TestUpsertComplianceProfile_DoesNotOverwriteStoredType(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("../sqlc/queries/compliance_profiles.sql")
	if err != nil {
		t.Fatalf("reading query file: %v", err)
	}
	sql := string(raw)

	start := strings.Index(sql, "-- name: UpsertComplianceProfile")
	if start < 0 {
		t.Fatal("UpsertComplianceProfile query not found")
	}
	stmt := sql[start:]
	if next := strings.Index(stmt[1:], "\n-- name: "); next >= 0 {
		stmt = stmt[:next+1]
	}

	conflictIdx := strings.Index(stmt, "ON CONFLICT")
	if conflictIdx < 0 {
		t.Fatal("UpsertComplianceProfile must carry an ON CONFLICT clause; without it two " +
			"concurrent callers race on UNIQUE(name) and the loser fails with 23505")
	}
	conflict := stmt[conflictIdx:]

	// The conflict branch must not assign type at all.
	for _, forbidden := range []string{
		"type = EXCLUDED.type",
		"type = COALESCE",
		"type =",
	} {
		if strings.Contains(conflict, forbidden) {
			t.Errorf("the ON CONFLICT branch must not write type (found %q).\n"+
				"An existing profile keeps its stored type; the submitted type is ignored.\n"+
				"Overwriting it flips the scanner toggle that gates the profile in SubmitScan.",
				forbidden)
		}
	}

	// It must still return the row, so callers can read the stored type back.
	if !strings.Contains(conflict, "RETURNING") {
		t.Error("the upsert must RETURN the row so callers observe the stored type")
	}
	if !strings.Contains(conflict, "DO UPDATE") {
		t.Error("must be DO UPDATE, not DO NOTHING: DO NOTHING returns no row on conflict, " +
			"so an existing profile would resolve to nothing")
	}
}

// TestUpsertComplianceRule_PreservesMetadataOnConflict covers the sibling
// upsert, where the opposite is true: rule metadata SHOULD be refreshed by a
// later scan, but a submission that omits a field must not blank a value an
// earlier scan supplied.
func TestUpsertComplianceRule_PreservesMetadataOnConflict(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("../sqlc/queries/compliance_rules.sql")
	if err != nil {
		t.Fatalf("reading query file: %v", err)
	}
	sql := string(raw)

	start := strings.Index(sql, "-- name: UpsertComplianceRule")
	if start < 0 {
		t.Fatal("UpsertComplianceRule query not found")
	}
	stmt := sql[start:]
	if next := strings.Index(stmt[1:], "\n-- name: "); next >= 0 {
		stmt = stmt[:next+1]
	}

	if !strings.Contains(stmt, "ON CONFLICT (profile_id, rule_ref)") {
		t.Fatal("must upsert on the (profile_id, rule_ref) unique constraint")
	}
	// Every updated metadata column must be COALESCE-guarded so an omitted
	// field does not blank a stored one.
	for _, col := range []string{"title", "description", "severity", "section", "remediation"} {
		needle := col + " = COALESCE(EXCLUDED." + col
		if !strings.Contains(stmt, needle) {
			t.Errorf("column %q must be COALESCE-guarded in the conflict branch so a "+
				"submission omitting it does not blank an earlier value", col)
		}
	}
}
