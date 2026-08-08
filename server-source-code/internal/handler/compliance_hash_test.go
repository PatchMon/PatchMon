package handler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// complianceFixtureFile is the shared canonical-hash fixture. The agent's test
// suite loads a byte-identical copy, so any drift on either side fails both.
const complianceFixtureFile = "canonical_fixtures.json"

// loadComplianceFixtureRaw returns the raw JSON of the `compliance` fixture
// block: `{"input": {"scans": [...]}, "expected_hash": "..."}`.
//
// We deliberately keep it as raw JSON rather than reusing the typed `fixture`
// struct, because the whole point of these tests is to decode the SAME bytes
// through the HANDLER's wire struct (complianceScanItem) rather than through
// ComplianceHashInput.
func loadComplianceFixtureRaw(t *testing.T) (input json.RawMessage, expectedHash string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", complianceFixtureFile))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var all struct {
		Compliance struct {
			Input        json.RawMessage `json:"input"`
			ExpectedHash string          `json:"expected_hash"`
		} `json:"compliance"`
	}
	if err := json.Unmarshal(b, &all); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	if len(all.Compliance.Input) == 0 {
		t.Fatalf("fixture has no compliance.input block")
	}
	return all.Compliance.Input, all.Compliance.ExpectedHash
}

// decodeScanItems decodes a `{"scans": [...]}` document through the exact
// struct the compliance endpoint decodes inbound agent payloads into.
func decodeScanItems(t *testing.T, raw []byte) []complianceScanItem {
	t.Helper()
	var wrapper struct {
		Scans []complianceScanItem `json:"scans"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		t.Fatalf("decode scans through complianceScanItem: %v", err)
	}
	if len(wrapper.Scans) == 0 {
		t.Fatalf("decoded zero scans")
	}
	return wrapper.Scans
}

// TestCanonicalComplianceHashFromPayload_MatchesFixture exercises the HANDLER
// BOUNDARY, which is where the real bug lived.
//
// TestCanonicalFixtureHashes hashes ComplianceHashInput directly, so it can
// never catch a field that the inbound wire struct (complianceScanItem) fails
// to decode: the conversion in canonicalComplianceHashFromPayload simply reads
// a Go zero value and the fixture test stays green. This test forces the
// fixture bytes through that conversion.
func TestCanonicalComplianceHashFromPayload_MatchesFixture(t *testing.T) {
	raw, want := loadComplianceFixtureRaw(t)
	if want == "" || want == "TBD" {
		t.Skip("fixture has no baked-in expected compliance hash")
	}

	got, err := canonicalComplianceHashFromPayload(decodeScanItems(t, raw))
	if err != nil {
		t.Fatalf("canonicalComplianceHashFromPayload: %v", err)
	}
	if got != want {
		t.Errorf("handler-boundary compliance hash drift: got %s want %s", got, want)
	}
}

// TestCanonicalComplianceHashFromPayload_CarriesRemediationFields is the direct
// regression guard for the hash-gate wedge.
//
// The agent hashes remediation_applied / remediation_count. The server's
// inbound struct used to omit both, so every scan where remediation had
// actually been applied hashed differently server-side, was rejected with
// "compliance hash mismatch", was never persisted, and left compliance_hash
// stale — which made the next ping re-request compliance, so the agent ran a
// full OpenSCAP scan on every single tick, forever.
//
// Two assertions matter here:
//  1. the remediated payload must NOT hash the same as the non-remediated one
//     (proves the fields reach the canonical struct at all), and
//  2. the handler-boundary hash must equal the hash of the equivalent
//     ComplianceHashInput (proves the server agrees with what the agent hashed).
func TestCanonicalComplianceHashFromPayload_CarriesRemediationFields(t *testing.T) {
	raw, _ := loadComplianceFixtureRaw(t)

	baseline, err := canonicalComplianceHashFromPayload(decodeScanItems(t, raw))
	if err != nil {
		t.Fatalf("baseline hash: %v", err)
	}

	// Flip remediation on in the raw fixture bytes so both decode paths see
	// exactly the same document.
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal fixture input: %v", err)
	}
	scans, ok := doc["scans"].([]any)
	if !ok || len(scans) == 0 {
		t.Fatalf("fixture input has no scans array")
	}
	first, ok := scans[0].(map[string]any)
	if !ok {
		t.Fatalf("fixture scan is not an object")
	}
	first["remediation_applied"] = true
	first["remediation_count"] = 7
	remediatedRaw, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("re-marshal fixture: %v", err)
	}

	items := decodeScanItems(t, remediatedRaw)
	if !items[0].RemediationApplied {
		t.Fatalf("complianceScanItem did not decode remediation_applied — the wire struct is missing the field")
	}
	if items[0].RemediationCount != 7 {
		t.Fatalf("complianceScanItem decoded remediation_count = %d, want 7", items[0].RemediationCount)
	}

	got, err := canonicalComplianceHashFromPayload(items)
	if err != nil {
		t.Fatalf("remediated hash: %v", err)
	}
	if got == baseline {
		t.Fatalf("remediation fields do not affect the handler-boundary hash (%s) — they are being dropped in the conversion to ComplianceWireScan", got)
	}

	// The agent-side shape must agree byte-for-byte.
	var wireInput ComplianceHashInput
	if err := json.Unmarshal(remediatedRaw, &wireInput); err != nil {
		t.Fatalf("decode remediated fixture as ComplianceHashInput: %v", err)
	}
	want, err := CanonicalComplianceHash(wireInput)
	if err != nil {
		t.Fatalf("CanonicalComplianceHash: %v", err)
	}
	if got != want {
		t.Errorf("handler boundary disagrees with the canonical wire hash: got %s want %s", got, want)
	}
}

// TestCanonicalComplianceHashFromPayload_AllWireFieldsRoundTrip walks every
// field of the canonical scan shape and asserts that changing it changes the
// handler-boundary hash. Any future field added to ComplianceWireScan but
// forgotten in complianceScanItem (or in the conversion) reproduces exactly the
// remediation bug, and this test fails the moment it does.
func TestCanonicalComplianceHashFromPayload_AllWireFieldsRoundTrip(t *testing.T) {
	raw, _ := loadComplianceFixtureRaw(t)

	baseline, err := canonicalComplianceHashFromPayload(decodeScanItems(t, raw))
	if err != nil {
		t.Fatalf("baseline hash: %v", err)
	}

	// Values chosen to differ from the fixture for every field.
	mutations := map[string]any{
		"profile_name":        "some other profile",
		"profile_type":        "docker-bench",
		"status":              "failed",
		"score":               11.25,
		"total_rules":         4321,
		"passed":              4320,
		"failed":              1,
		"warnings":            2,
		"skipped":             3,
		"not_applicable":      4,
		"remediation_applied": true,
		"remediation_count":   9,
	}

	for field, value := range mutations {
		t.Run(field, func(t *testing.T) {
			var doc map[string]any
			if err := json.Unmarshal(raw, &doc); err != nil {
				t.Fatalf("unmarshal fixture input: %v", err)
			}
			scans := doc["scans"].([]any)
			scans[0].(map[string]any)[field] = value
			mutated, err := json.Marshal(doc)
			if err != nil {
				t.Fatalf("re-marshal fixture: %v", err)
			}

			got, err := canonicalComplianceHashFromPayload(decodeScanItems(t, mutated))
			if err != nil {
				t.Fatalf("hash mutated payload: %v", err)
			}
			if got == baseline {
				t.Errorf("changing %q did not change the handler-boundary hash — the field is not being carried from complianceScanItem into ComplianceWireScan", field)
			}
		})
	}
}
