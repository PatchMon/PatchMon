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
	for _, name := range []string{"patchmon_identity", "enrol_outcome", "write_stub_config"} {
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
		{"nothing present", func(*testing.T, string) {}, "api_id=\napi_key=\nhost_id=\n"},
		{"live files, quoted values", func(t *testing.T, dir string) {
			write(t, filepath.Join(dir, "credentials.yml"), "# c\napi_id: \"patchmon_ab12\"\napi_key: \"deadbeef\"\n", 0)
			write(t, filepath.Join(dir, "config.yml"), "patchmon_server: x\nhost_id: \""+hid+"\"\n", 0)
		}, "api_id=patchmon_ab12\napi_key=deadbeef\nhost_id=" + hid + "\n"},
		{"agent-written unquoted values with CRLF", func(t *testing.T, dir string) {
			write(t, filepath.Join(dir, "credentials.yml"), "api_id: patchmon_ab12\r\napi_key: deadbeef\r\n", 0)
			write(t, filepath.Join(dir, "config.yml"), "config_version: 2.1.5\r\nhost_id: "+hid+"\r\n", 0)
		}, "api_id=patchmon_ab12\napi_key=deadbeef\nhost_id=" + hid + "\n"},
		{"falls back to the newest backups", func(t *testing.T, dir string) {
			write(t, filepath.Join(dir, "credentials.yml.backup.20260101_000000"), "api_id: patchmon_old\napi_key: aaaa\n", 2*time.Hour)
			write(t, filepath.Join(dir, "credentials.yml.backup.20260102_000000"), "api_id: patchmon_new\napi_key: bbbb\n", time.Hour)
			write(t, filepath.Join(dir, "config.yml"), "patchmon_server: x\n", 0)
			write(t, filepath.Join(dir, "config.yml.backup.20260102_000000"), "host_id: "+hid+"\n", time.Hour)
		}, "api_id=patchmon_new\napi_key=bbbb\nhost_id=" + hid + "\n"},
		{"incomplete live credentials use a complete backup", func(t *testing.T, dir string) {
			write(t, filepath.Join(dir, "credentials.yml"), "api_id: patchmon_ab12\n", 0)
			write(t, filepath.Join(dir, "credentials.yml.backup.20260102_000000"), "api_id: patchmon_cd34\napi_key: cccc\n", time.Hour)
		}, "api_id=patchmon_cd34\napi_key=cccc\nhost_id=\n"},
		{"unsafe values are dropped", func(t *testing.T, dir string) {
			write(t, filepath.Join(dir, "credentials.yml"), "api_id: patchmon_ab;rm\napi_key: dead$(id)\n", 0)
			write(t, filepath.Join(dir, "config.yml"), "host_id: ../../x\n", 0)
		}, "api_id=\napi_key=\nhost_id=\n"},
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
