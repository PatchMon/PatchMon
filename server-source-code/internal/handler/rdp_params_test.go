package handler

import (
	"encoding/json"
	"os"
	"sort"
	"testing"
)

func TestRDPConnectionParametersFollowBrowserResize(t *testing.T) {
	p := rdpConnectionParameters(40123, "Administrator", "secret", "")
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

func TestRDPConnectionParametersKeyboardLayout(t *testing.T) {
	if got := rdpConnectionParameters(1, "", "", "en-gb-qwerty")["server-layout"]; got != "en-gb-qwerty" {
		t.Fatalf("server-layout = %q, want en-gb-qwerty", got)
	}
	if _, ok := rdpConnectionParameters(1, "", "", "")["server-layout"]; ok {
		t.Fatal("server-layout must be omitted when no layout was requested")
	}
}

func TestValidRDPKeyboardLayout(t *testing.T) {
	for _, layout := range []string{"", "en-gb-qwerty", "en-us-qwerty", "de-de-qwertz", "fr-fr-azerty", "ja-jp-qwerty"} {
		if !validRDPKeyboardLayout(layout) {
			t.Errorf("%q rejected", layout)
		}
	}
	for _, layout := range []string{"failsafe", "EN-GB-QWERTY", "en-gb", "en-gb-qwerty\nsecurity", "../en-us-qwerty"} {
		if validRDPKeyboardLayout(layout) {
			t.Errorf("%q accepted", layout)
		}
	}
}

func TestRDPKeyboardLayoutsMatchFrontend(t *testing.T) {
	raw, err := os.ReadFile("../../../frontend/src/utils/rdpKeyboardLayouts.json")
	if err != nil {
		t.Fatal(err)
	}
	var frontend []string
	if err := json.Unmarshal(raw, &frontend); err != nil {
		t.Fatal(err)
	}
	server := make([]string, 0, len(rdpKeyboardLayouts))
	for layout := range rdpKeyboardLayouts {
		server = append(server, layout)
	}
	sort.Strings(server)
	sort.Strings(frontend)
	if len(server) != len(frontend) {
		t.Fatalf("server allows %v, frontend offers %v", server, frontend)
	}
	for i := range server {
		if server[i] != frontend[i] {
			t.Fatalf("server allows %v, frontend offers %v", server, frontend)
		}
	}
}
