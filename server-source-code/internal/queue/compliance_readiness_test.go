package queue

import "testing"

func TestValidateComplianceScanReadinessBlocksFailedOpenSCAP(t *testing.T) {
	status := []byte(`{"components":{"openscap":"failed","docker-bench":"ready"}}`)

	err := ValidateComplianceScanReadiness(status, "openscap", nil, true, true)

	if err == nil {
		t.Fatal("expected failed OpenSCAP scanner to block the scan")
	}
	if err.Error() != "OpenSCAP scanner is failed" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateComplianceScanReadinessAllowsReadyDockerBenchWhenOpenSCAPFailed(t *testing.T) {
	status := []byte(`{"components":{"openscap":"failed","docker-bench":"ready"}}`)

	err := ValidateComplianceScanReadiness(status, "docker-bench", nil, true, true)

	if err != nil {
		t.Fatalf("expected Docker Bench scan to be allowed: %v", err)
	}
}

func TestValidateComplianceScanReadinessBlocksDefaultScanWhenEnabledScannerFailed(t *testing.T) {
	status := []byte(`{"components":{"openscap":"failed","docker-bench":"unavailable"}}`)

	err := ValidateComplianceScanReadiness(status, "all", nil, true, false)

	if err == nil {
		t.Fatal("expected failed enabled scanner to block default scan")
	}
	if err.Error() != "OpenSCAP scanner is failed" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateComplianceScanReadinessAllowsUnknownStatusForOlderAgents(t *testing.T) {
	err := ValidateComplianceScanReadiness(nil, "openscap", nil, true, false)

	if err != nil {
		t.Fatalf("expected missing scanner status to be allowed: %v", err)
	}
}

func TestValidateComplianceScanReadinessBlocksDisabledExplicitScanner(t *testing.T) {
	err := ValidateComplianceScanReadiness(nil, "docker-bench", nil, true, false)

	if err == nil {
		t.Fatal("expected disabled Docker Bench scanner to block explicit scan")
	}
	if err.Error() != "docker-bench scanner is disabled for this host" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateComplianceScanReadinessUsesScannerInfoWhenComponentsMissing(t *testing.T) {
	status := []byte(`{"scanner_info":{"openscap_available":false}}`)

	err := ValidateComplianceScanReadiness(status, "openscap", nil, true, false)

	if err == nil {
		t.Fatal("expected unavailable OpenSCAP scanner to block the scan")
	}
	if err.Error() != "OpenSCAP scanner is unavailable" {
		t.Fatalf("unexpected error: %v", err)
	}
}
