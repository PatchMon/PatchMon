package main

import (
	"strings"
	"testing"
)

func stringPointer(s string) *string { return &s }

func TestNormalizeServer(t *testing.T) {
	tests := []struct {
		input    string
		insecure bool
		want     string
		wantErr  bool
	}{
		{"patchmon.example.com/", false, "https://patchmon.example.com", false},
		{"https://patchmon.example.com/base/", false, "https://patchmon.example.com/base", false},
		{"http://localhost:3000", false, "", true},
		{"http://localhost:3000", true, "http://localhost:3000", false},
	}
	for _, tt := range tests {
		got, err := normalizeServer(tt.input, tt.insecure)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Fatalf("normalizeServer(%q) = %q, %v; want %q, error=%v", tt.input, got, err, tt.want, tt.wantErr)
		}
	}
}

func TestResolveHost(t *testing.T) {
	hosts := []host{
		{ID: "id-1", APIID: "api-1", FriendlyName: "Web", Hostname: stringPointer("web-01")},
		{ID: "id-2", APIID: "api-2", FriendlyName: "DB", Hostname: stringPointer("db-01")},
	}
	for _, query := range []string{"id-1", "api-1", "web", "WEB-01"} {
		got, err := resolveHost(hosts, query)
		if err != nil || got.ID != "id-1" {
			t.Fatalf("resolveHost(%q) = %#v, %v", query, got, err)
		}
	}
	if _, err := resolveHost(hosts, "missing"); err == nil {
		t.Fatal("expected missing host error")
	}
}

func TestResolveHostAmbiguous(t *testing.T) {
	hosts := []host{{ID: "1", FriendlyName: "same"}, {ID: "2", FriendlyName: "SAME"}}
	_, err := resolveHost(hosts, "same")
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected ambiguous error, got %v", err)
	}
}

func TestWebsocketURL(t *testing.T) {
	got, err := websocketURL("https://patchmon.example.com", "host-id", "secret+/=")
	if err != nil {
		t.Fatal(err)
	}
	want := "wss://patchmon.example.com/api/v1/ssh-terminal/host-id?ticket=secret%2B%2F%3D"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
