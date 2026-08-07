package store

import (
	"os"
	"strings"
	"testing"
)

// UpsertComplianceProfile replaced a SELECT-then-INSERT.
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
	for _, col := range []string{"description", "severity", "section", "remediation"} {
		needle := col + " = COALESCE(EXCLUDED." + col
		if !strings.Contains(stmt, needle) {
			t.Errorf("column %q must be COALESCE-guarded in the conflict branch so a "+
				"submission omitting it does not blank an earlier value", col)
		}
	}

	// title is the exception, and asserting on the SQL alone is not enough --
	// that is how the original defect survived review. The column is TEXT NOT
	// NULL, so the INSERT arm has to supply a fallback; if the conflict branch
	// read EXCLUDED.title it would see that fallback rather than what the caller
	// passed, and a scan omitting the title would overwrite a real one with the
	// rule_ref. It must read the raw parameter instead.
	if !strings.Contains(stmt, "title = COALESCE(sqlc.narg('title')") {
		t.Error("the title conflict branch must read the raw parameter, not EXCLUDED.title, " +
			"which is the post-VALUES row and therefore already holds the rule_ref fallback")
	}
}

// TestUpsertComplianceRule_CallSitePassesNullableTitle closes the gap that
// let the previous defect through: the SQL said COALESCE, but the Go call
// site passed a value that could never be nil, so the guard could never fire.
func TestUpsertComplianceRule_CallSitePassesNullableTitle(t *testing.T) {
	t.Parallel()

	src, err := os.ReadFile("compliance.go")
	if err != nil {
		t.Fatalf("reading source: %v", err)
	}
	body := string(src)

	start := strings.Index(body, "UpsertComplianceRuleParams{")
	if start < 0 {
		t.Fatal("UpsertComplianceRuleParams call site not found")
	}
	call := body[start:]
	if end := strings.Index(call, "})"); end > 0 {
		call = call[:end]
	}

	if strings.Contains(call, "Title:       orEmpty(") || strings.Contains(call, "Title: orEmpty(") {
		t.Error("Title must not be passed through orEmpty: it never returns empty, so the " +
			"COALESCE in the conflict branch can never take the stored value and a scan " +
			"omitting the title overwrites a real one with the rule_ref")
	}
	if !strings.Contains(call, "complianceStrPtr(r.Title)") {
		t.Error("Title must be passed as a nullable via complianceStrPtr so an omitted " +
			"title reaches the query as NULL")
	}
}
