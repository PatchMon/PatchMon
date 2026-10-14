package agents

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func extractShellFunc(t *testing.T, script []byte, name string) string {
	t.Helper()
	s := string(script)
	start := strings.Index(s, name+"() {")
	if start < 0 {
		t.Fatalf("%s not found", name)
	}
	end := strings.Index(s[start:], "\n}\n")
	if end < 0 {
		t.Fatalf("%s has no closing brace", name)
	}
	return s[start : start+end+3]
}

func shells(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, sh := range []string{"sh", "dash", "bash"} {
		if p, err := exec.LookPath(sh); err == nil {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		t.Skip("no POSIX shell available")
	}
	return out
}

func runShell(t *testing.T, sh, src string, args ...string) string {
	t.Helper()
	out, err := exec.Command(sh, append([]string{"-c", src, "_"}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", sh, err, out)
	}
	return string(out)
}

func TestEnrolHelpersMatchBetweenScripts(t *testing.T) {
	for _, name := range []string{"patchmon_identity", "identity_value", "same_server", "enrol_outcome", "set_aside_identity", "restore_identity", "write_stub_config"} {
		direct := extractShellFunc(t, DirectHostAutoEnrollScript, name)
		proxmox := extractShellFunc(t, ProxmoxAutoEnrollScript, name)
		if direct != proxmox {
			t.Errorf("%s differs between direct_host_auto_enroll.sh and proxmox_auto_enroll.sh", name)
		}
	}
}

func TestPatchmonIdentity(t *testing.T) {
	fn := extractShellFunc(t, DirectHostAutoEnrollScript, "patchmon_identity")
	write := func(t *testing.T, path, body string, age time.Duration) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		ts := time.Now().Add(-age)
		if err := os.Chtimes(path, ts, ts); err != nil {
			t.Fatal(err)
		}
	}
	const hid = "3b0c2f7e-6a1d-4c55-9a0e-2f1f7d9e8c41"

	cases := []struct {
		name  string
		setup func(t *testing.T, dir string)
		want  string
	}{
		{"nothing present", func(*testing.T, string) {}, "api_id=\napi_key=\nhost_id=\nserver=\n"},
		{"live files, quoted values", func(t *testing.T, dir string) {
			write(t, filepath.Join(dir, "credentials.yml"), "# c\napi_id: \"patchmon_ab12\"\napi_key: \"deadbeef\"\n", 0)
			write(t, filepath.Join(dir, "config.yml"), "patchmon_server: \"https://pm.example/\"\nhost_id: \""+hid+"\"\n", 0)
		}, "api_id=patchmon_ab12\napi_key=deadbeef\nhost_id=" + hid + "\nserver=https://pm.example/\n"},
		{"agent-written unquoted values with CRLF", func(t *testing.T, dir string) {
			write(t, filepath.Join(dir, "credentials.yml"), "api_id: patchmon_ab12\r\napi_key: deadbeef\r\n", 0)
			write(t, filepath.Join(dir, "config.yml"), "config_version: 2.1.4\r\nhost_id: "+hid+"\r\npatchmon_server: http://10.0.0.5:3000\r\n", 0)
		}, "api_id=patchmon_ab12\napi_key=deadbeef\nhost_id=" + hid + "\nserver=http://10.0.0.5:3000\n"},
		{"falls back to the newest backups", func(t *testing.T, dir string) {
			write(t, filepath.Join(dir, "credentials.yml.backup.20260101_000000"), "api_id: patchmon_old\napi_key: aaaa\n", 2*time.Hour)
			write(t, filepath.Join(dir, "credentials.yml.backup.20260102_000000"), "api_id: patchmon_new\napi_key: bbbb\n", time.Hour)
			write(t, filepath.Join(dir, "config.yml.backup.20260101_000000"), "patchmon_server: https://old\nhost_id: old-id\n", 2*time.Hour)
			write(t, filepath.Join(dir, "config.yml.backup.20260102_000000"), "patchmon_server: https://pm\nhost_id: "+hid+"\n", time.Hour)
		}, "api_id=patchmon_new\napi_key=bbbb\nhost_id=" + hid + "\nserver=https://pm\n"},
		{"a config without a server is skipped", func(t *testing.T, dir string) {
			write(t, filepath.Join(dir, "config.yml"), "host_id: stray\n", 0)
			write(t, filepath.Join(dir, "config.yml.backup.20260102_000000"), "patchmon_server: https://pm\nhost_id: "+hid+"\n", time.Hour)
		}, "api_id=\napi_key=\nhost_id=" + hid + "\nserver=https://pm\n"},
		{"incomplete live credentials use a complete backup", func(t *testing.T, dir string) {
			write(t, filepath.Join(dir, "credentials.yml"), "api_id: patchmon_ab12\n", 0)
			write(t, filepath.Join(dir, "credentials.yml.backup.20260102_000000"), "api_id: patchmon_cd34\napi_key: cccc\n", time.Hour)
		}, "api_id=patchmon_cd34\napi_key=cccc\nhost_id=\nserver=\n"},
		{"unsafe values are dropped", func(t *testing.T, dir string) {
			write(t, filepath.Join(dir, "credentials.yml"), "api_id: patchmon_ab;rm\napi_key: dead$(id)\n", 0)
			write(t, filepath.Join(dir, "config.yml"), "patchmon_server: https://pm/$(id)\nhost_id: ../../x\n", 0)
		}, "api_id=\napi_key=\nhost_id=\nserver=\n"},
	}
	for _, sh := range shells(t) {
		for _, c := range cases {
			t.Run(filepath.Base(sh)+"/"+c.name, func(t *testing.T) {
				dir := t.TempDir()
				c.setup(t, dir)
				if got := runShell(t, sh, fn+`patchmon_identity "$1"`, dir); got != c.want {
					t.Errorf("got %q, want %q", got, c.want)
				}
			})
		}
	}
}

func TestEnrolOutcome(t *testing.T) {
	fn := extractShellFunc(t, DirectHostAutoEnrollScript, "enrol_outcome")
	cases := []struct{ code, body, want string }{
		{"201", `{"host":{"id":"x"}}`, "created"},
		{"200", `{"reenrolled":true}`, "reenrolled"},
		{"409", `{"code":"host_reporting","error":"e","friendly_name":"web","host_id":"x"}`, "host_reporting"},
		{"409", `{"code":"host_not_reenrollable","error":"e"}`, "host_not_reenrollable"},
		{"409", `{"code":"machine_id_match","error":"Host already enrolled"}`, "already_enrolled"},
		{"409", `{"error":"Host already enrolled","friendly_name":"web"}`, "already_enrolled"},
		{"429", `{"code":"reenrol_cooldown"}`, "retry_later"},
		{"429", `{"error":"Rate limit exceeded"}`, "retry_later"},
		{"500", `{}`, "failed"},
		{"000", ``, "failed"},
	}
	for _, sh := range shells(t) {
		for _, c := range cases {
			if got := strings.TrimSpace(runShell(t, sh, fn+`enrol_outcome "$1" "$2"`, c.code, c.body)); got != c.want {
				t.Errorf("%s: enrol_outcome(%s, %s) = %q, want %q", sh, c.code, c.body, got, c.want)
			}
		}
	}
}

func TestWriteStubConfig(t *testing.T) {
	fn := extractShellFunc(t, DirectHostAutoEnrollScript, "write_stub_config")
	const hid = "3b0c2f7e-6a1d-4c55-9a0e-2f1f7d9e8c41"
	for _, sh := range shells(t) {
		t.Run(filepath.Base(sh), func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "patchmon")
			runShell(t, sh, fn+`write_stub_config "$1" "$2" "$3"`, dir, "https://pm.example", hid)
			path := filepath.Join(dir, "config.yml")
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if want := "patchmon_server: \"https://pm.example\"\nhost_id: \"" + hid + "\"\n"; string(b) != want {
				t.Errorf("stub = %q, want %q", b, want)
			}
			if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
				t.Errorf("stub mode = %v, want 0600", fi.Mode().Perm())
			}

			runShell(t, sh, fn+`write_stub_config "$1" "$2" "$3"`, dir, "https://other", "aaaa")
			if b2, _ := os.ReadFile(path); string(b2) != string(b) {
				t.Error("an existing config.yml was overwritten")
			}

			empty := filepath.Join(t.TempDir(), "patchmon")
			runShell(t, sh, fn+`write_stub_config "$1" "$2" "$3"`, empty, "https://pm.example", "bad id")
			if _, err := os.Stat(filepath.Join(empty, "config.yml")); !os.IsNotExist(err) {
				t.Error("a stub was written for an invalid host_id")
			}
		})
	}
}

func TestInstallScriptHostIDLine(t *testing.T) {
	s := string(PatchmonInstallScript)
	start := strings.Index(s, `HOST_ID_LINE=""`)
	end := strings.Index(s[start:], "esac\n")
	if start < 0 || end < 0 {
		t.Fatal("HOST_ID_LINE block not found in patchmon_install.sh")
	}
	snippet := s[start:start+end+5] + `printf '%s' "$HOST_ID_LINE"`
	if !strings.Contains(s, "${HOST_ID_LINE}\npatchmon_server:") {
		t.Fatal("config heredoc no longer writes HOST_ID_LINE")
	}
	for _, sh := range shells(t) {
		for in, want := range map[string]string{
			"3b0c2f7e-6a1d-4c55-9a0e-2f1f7d9e8c41": `host_id: "3b0c2f7e-6a1d-4c55-9a0e-2f1f7d9e8c41"`,
			"":                                     "",
			`x"; rm -rf /`:                         "",
		} {
			cmd := exec.Command(sh, "-c", snippet)
			cmd.Env = append(os.Environ(), "PATCHMON_HOST_ID="+in)
			out, err := cmd.Output()
			if err != nil {
				t.Fatal(err)
			}
			if string(out) != want {
				t.Errorf("%s: PATCHMON_HOST_ID=%q gives %q, want %q", sh, in, out, want)
			}
		}
	}
}

func TestIdentityValueAndSameServer(t *testing.T) {
	fns := extractShellFunc(t, DirectHostAutoEnrollScript, "identity_value") + extractShellFunc(t, DirectHostAutoEnrollScript, "same_server")
	ident := "api_id=patchmon_ab\napi_id=second\nhost_id=evil\r\nX-Injected: 1\nserver=https://pm.example/\n"
	for _, sh := range shells(t) {
		for key, want := range map[string]string{"api_id": "patchmon_ab", "host_id": "", "server": "https://pm.example/", "missing": ""} {
			if got := runShell(t, sh, fns+`identity_value "$1" "$2"`, ident, key); got != want {
				t.Errorf("%s: identity_value(%s) = %q, want %q", sh, key, got, want)
			}
		}
		for _, c := range []struct {
			a, b string
			want bool
		}{
			{"https://pm.example/", "https://pm.example", true},
			{"https://pm.example", "https://pm.example", true},
			{"https://other", "https://pm.example", false},
			{"", "https://pm.example", false},
		} {
			got := exec.Command(sh, "-c", fns+`same_server "$1" "$2"`, "_", c.a, c.b).Run() == nil
			if got != c.want {
				t.Errorf("%s: same_server(%q, %q) = %v, want %v", sh, c.a, c.b, got, c.want)
			}
		}
	}
}

func TestSetAsideIdentity(t *testing.T) {
	fns := extractShellFunc(t, DirectHostAutoEnrollScript, "set_aside_identity") + extractShellFunc(t, DirectHostAutoEnrollScript, "patchmon_identity")
	for _, sh := range shells(t) {
		t.Run(filepath.Base(sh), func(t *testing.T) {
			dir := t.TempDir()
			for name, body := range map[string]string{
				"config.yml":                         "patchmon_server: https://pm\nhost_id: old\n",
				"credentials.yml":                    "api_id: patchmon_old\napi_key: aaaa\n",
				"config.yml.backup.20260101_000000":  "patchmon_server: https://pm\nhost_id: older\n",
				"credentials.yml.backup.20260101_00": "api_id: patchmon_older\napi_key: bbbb\n",
				"logs.txt":                           "keep",
			} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			moved := strings.TrimSpace(runShell(t, sh, fns+`set_aside_identity "$1"`, dir))
			if !strings.HasPrefix(moved, filepath.Join(dir, "pre-force-new-")) {
				t.Fatalf("set-aside dir = %q", moved)
			}
			entries, _ := os.ReadDir(moved)
			if len(entries) != 4 {
				t.Fatalf("moved %d files, want 4", len(entries))
			}
			if fi, _ := os.Stat(moved); fi.Mode().Perm() != 0o700 {
				t.Errorf("set-aside dir mode = %v, want 0700", fi.Mode().Perm())
			}
			if _, err := os.Stat(filepath.Join(dir, "logs.txt")); err != nil {
				t.Error("an unrelated file was moved")
			}
			if got := runShell(t, sh, fns+`patchmon_identity "$1"`, dir); got != "api_id=\napi_key=\nhost_id=\nserver=\n" {
				t.Errorf("a later rerun would still find the old identity: %q", got)
			}
			if got := strings.TrimSpace(runShell(t, sh, fns+`set_aside_identity "$1"`, t.TempDir())); got != "" {
				t.Errorf("nothing to move should print nothing, got %q", got)
			}
		})
	}
}

func TestProxmoxForceNewHostFor(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}
	fn := extractShellFunc(t, ProxmoxAutoEnrollScript, "force_new_host_for")
	for _, c := range []struct {
		value, vmid string
		want        bool
	}{
		{"true", "101", true},
		{"false", "101", false},
		{"", "101", false},
		{"101", "101", true},
		{"101,102", "102", true},
		{"101 102", "102", true},
		{"1011", "101", false},
		{"101", "1011", false},
	} {
		cmd := exec.Command(bash, "-c", fn+`force_new_host_for "$1"`, "_", c.vmid)
		cmd.Env = append(os.Environ(), "FORCE_NEW_HOST="+c.value)
		if got := cmd.Run() == nil; got != c.want {
			t.Errorf("FORCE_NEW_HOST=%q vmid %s: %v, want %v", c.value, c.vmid, got, c.want)
		}
	}
}

func TestRestoreIdentityUndoesSetAside(t *testing.T) {
	fns := extractShellFunc(t, DirectHostAutoEnrollScript, "set_aside_identity") + extractShellFunc(t, DirectHostAutoEnrollScript, "restore_identity")
	for _, sh := range shells(t) {
		t.Run(filepath.Base(sh), func(t *testing.T) {
			dir := t.TempDir()
			files := map[string]string{
				"config.yml":                        "patchmon_server: https://pm\nhost_id: old\n",
				"credentials.yml":                   "api_id: patchmon_old\napi_key: aaaa\n",
				"config.yml.backup.20260101_000000": "host_id: older\n",
			}
			for name, body := range files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			moved := strings.TrimSpace(runShell(t, sh, fns+`set_aside_identity "$1"`, dir))
			if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte("stub"), 0o600); err != nil {
				t.Fatal(err)
			}
			runShell(t, sh, fns+`restore_identity "$1" "$2"`, dir, moved)
			for name, body := range files {
				got, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil {
					t.Fatalf("%s not restored: %v", name, err)
				}
				if name == "config.yml" {
					if string(got) != "stub" {
						t.Errorf("restore overwrote a newer config.yml")
					}
					continue
				}
				if string(got) != body {
					t.Errorf("%s = %q, want %q", name, got, body)
				}
			}

			outside := t.TempDir()
			if err := os.WriteFile(filepath.Join(outside, "x"), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
			runShell(t, sh, fns+`restore_identity "$1" "$2"`, dir, outside)
			if _, err := os.Stat(filepath.Join(dir, "x")); err == nil {
				t.Error("restore_identity moved files from a directory it did not create")
			}
		})
	}
}
