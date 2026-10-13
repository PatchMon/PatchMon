package registry

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDefaultDockerConfigPathHonoursEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", dir)
	if got := DefaultDockerConfigPath(); got != filepath.Join(dir, "config.json") {
		t.Errorf("path = %q", got)
	}
}

func TestLoadDockerConfigAuths(t *testing.T) {
	enc := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	path := writeConfig(t, fmt.Sprintf(`{
		"auths": {
			"https://index.docker.io/v1/": {"auth": %q},
			"ghcr.io": {"auth": %q},
			"https://registry.example.com:5000/v2/": {"identitytoken": "idtok"},
			"broken.example.com": {"auth": "!!!"},
			"empty.example.com": {}
		}
	}`, enc("hubuser:hubpass"), enc("gh:p:with:colons")))

	cfg, err := LoadDockerConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	tests := []struct {
		host string
		want Credential
		ok   bool
	}{
		{DockerHubHost, Credential{Username: "hubuser", Password: "hubpass"}, true},
		{"ghcr.io", Credential{Username: "gh", Password: "p:with:colons"}, true},
		{"registry.example.com:5000", Credential{IdentityToken: "idtok"}, true},
		{"broken.example.com", Credential{}, false},
		{"empty.example.com", Credential{}, false},
		{"quay.io", Credential{}, false},
	}
	for _, tt := range tests {
		got, ok := cfg.Lookup(ctx, tt.host)
		if ok != tt.ok || got != tt.want {
			t.Errorf("Lookup(%q) = %v, %v; want ok=%v", tt.host, got.Username, ok, tt.ok)
		}
	}
}

func TestLoadDockerConfigMissingAndInvalid(t *testing.T) {
	cfg, err := LoadDockerConfig(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatalf("missing file err = %v", err)
	}
	if _, ok := cfg.Lookup(context.Background(), "ghcr.io"); ok {
		t.Error("empty config returned a credential")
	}

	_, err = LoadDockerConfig(writeConfig(t, `{"auths": {"x": {"auth": "c2VjcmV0OnNlY3JldA=="}`))
	if err == nil {
		t.Fatal("expected a parse error")
	}
	if strings.Contains(err.Error(), "c2VjcmV0") {
		t.Error("parse error echoes file contents")
	}
}

func TestCredentialNeverFormats(t *testing.T) {
	c := Credential{Username: "u", Password: "hunter2", IdentityToken: "tok"}
	for _, s := range []string{fmt.Sprint(c), fmt.Sprintf("%v", c), fmt.Sprintf("%+v", c), fmt.Sprintf("%#v", c)} {
		if strings.Contains(s, "hunter2") || strings.Contains(s, "tok") {
			t.Errorf("credential formatted as %q", s)
		}
	}
}

func TestCredentialHelpers(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("credential helper fake is a shell script")
	}
	binDir := t.TempDir()
	logPath := filepath.Join(binDir, "calls.log")
	script := `#!/bin/sh
read -r server
echo "$1 $server" >> ` + logPath + `
case "$server" in
  ghcr.io) echo '{"ServerURL":"ghcr.io","Username":"helperuser","Secret":"helpersecret"}' ;;
  https://index.docker.io/v1/) echo '{"ServerURL":"https://index.docker.io/v1/","Username":"<token>","Secret":"hubtoken"}' ;;
  *) echo "credentials not found in native keychain" >&2; exit 1 ;;
esac
`
	if err := os.WriteFile(filepath.Join(binDir, "docker-credential-fake"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	path := writeConfig(t, `{
		"credsStore": "fake",
		"credHelpers": {"evil.example.com": "../../bin/sh", "quay.io": "fake"},
		"auths": {"ghcr.io": {}}
	}`)
	cfg, err := LoadDockerConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	got, ok := cfg.Lookup(ctx, "ghcr.io")
	if !ok || got.Username != "helperuser" || got.Password != "helpersecret" {
		t.Errorf("ghcr.io via credsStore = %q ok=%v", got.Username, ok)
	}
	got, ok = cfg.Lookup(ctx, DockerHubHost)
	if !ok || got.IdentityToken != "hubtoken" || got.Username != "" {
		t.Errorf("hub identity token via helper: ok=%v", ok)
	}
	if _, ok := cfg.Lookup(ctx, "quay.io"); ok {
		t.Error("helper reporting not found should give no credential")
	}
	if _, ok := cfg.Lookup(ctx, "evil.example.com"); ok {
		t.Error("invalid helper name must not run")
	}
	_, _ = cfg.Lookup(ctx, "ghcr.io")

	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(log), "get ghcr.io"); n != 1 {
		t.Errorf("helper called %d times for ghcr.io, want 1 (cached)", n)
	}
	if strings.Contains(string(log), "evil") {
		t.Error("helper ran for a rejected helper name")
	}
}
