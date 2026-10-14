package handler

import "testing"

func TestRDPConnectionParametersFollowBrowserResize(t *testing.T) {
	p := rdpConnectionParameters(40123, "Administrator", "secret")
	if got := p["resize-method"]; got != "display-update" {
		t.Fatalf("resize-method = %q, want display-update", got)
	}
	if p["hostname"] != "127.0.0.1" || p["port"] != "40123" {
		t.Fatalf("unexpected upstream %s:%s", p["hostname"], p["port"])
	}
	if p["security"] != "any" || p["ignore-cert"] != "true" {
		t.Fatalf("security parameters changed: %v", p)
	}
}
