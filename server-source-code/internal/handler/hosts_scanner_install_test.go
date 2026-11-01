package handler

import (
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
)

func TestOpenscapNewlyEnabled(t *testing.T) {
	tests := []struct {
		name                       string
		wasCompliance, wasOpenscap bool
		compliance, openscap       bool
		want                       bool
	}{
		{"compliance switched on with openscap", false, true, true, true, true},
		{"openscap ticked on an enabled host", true, false, true, true, true},
		{"both switched on", false, false, true, true, true},
		{"already on, unrelated change", true, true, true, true, false},
		{"switched off", true, true, false, true, false},
		{"docker bench only", false, false, true, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := &models.Host{ComplianceEnabled: tt.wasCompliance, ComplianceOpenscapEnabled: tt.wasOpenscap}
			if got := openscapNewlyEnabled(before, tt.compliance, tt.openscap); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOpenscapInstallSupported(t *testing.T) {
	pm := func(s string) *string { return &s }
	tests := []struct {
		name string
		pm   *string
		want bool
	}{
		{name: "not reported yet", want: true},
		{name: "debian family", pm: pm("apt"), want: true},
		{name: "rhel family", pm: pm("dnf"), want: true},
		{name: "older rhel", pm: pm("yum"), want: true},
		{name: "suse", pm: pm("zypper"), want: true},
		{name: "agent older than zypper support on suse", pm: pm("unknown"), want: true},
		{name: "windows", pm: pm("windows"), want: false},
		{name: "freebsd", pm: pm("pkg"), want: false},
		{name: "alpine", pm: pm("apk"), want: false},
		{name: "arch", pm: pm("pacman"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := openscapInstallSupported(&models.Host{PackageManager: tt.pm}); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestScannerInstallNeeded(t *testing.T) {
	ready := []byte(`{"status":"ready","scanner_info":{"openscap_available":true}}`)
	enabled := models.Host{ComplianceEnabled: true, ComplianceOpenscapEnabled: true}
	withStatus := func(raw string) models.Host {
		h := enabled
		h.ComplianceScannerStatus = []byte(raw)
		return h
	}
	tests := []struct {
		name                 string
		before               models.Host
		compliance, openscap bool
		want                 bool
	}{
		{"newly enabled", models.Host{ComplianceOpenscapEnabled: true}, true, true, true},
		{"on-demand to enabled, scanner never reported", enabled, true, true, true},
		{"on-demand to enabled, scanner reported error", withStatus(`{"status":"error","scanner_info":{"openscap_available":false}}`), true, true, true},
		{"partial: docker bench ready, openscap missing", withStatus(`{"status":"partial","components":{"openscap":"failed","docker-bench":"ready"},"scanner_info":{"openscap_available":false}}`), true, true, true},
		{"partial: openscap ready, docker bench missing", withStatus(`{"status":"partial","components":{"openscap":"ready","docker-bench":"failed"},"scanner_info":{"openscap_available":true}}`), true, true, false},
		{"openscap ready per components only", withStatus(`{"status":"partial","components":{"openscap":"ready"}}`), true, true, false},
		{"already enabled and scanner ready", models.Host{ComplianceEnabled: true, ComplianceOpenscapEnabled: true, ComplianceScannerStatus: ready}, true, true, false},
		{"docker bench only: openscap not selected", models.Host{ComplianceEnabled: true, ComplianceDockerBenchEnabled: true}, true, false, false},
		{"compliance disabled", enabled, false, true, false},
		{"malformed status counts as not ready", withStatus(`{`), true, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scannerInstallNeeded(&tt.before, tt.compliance, tt.openscap); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
