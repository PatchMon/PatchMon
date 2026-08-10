package agents

import (
	"regexp"
	"strings"
	"testing"

	yaml "go.yaml.in/yaml/v3"
)

// The installer builds config.yml from a PowerShell here-string. This lifts that
// block out of the script, substitutes what PowerShell would interpolate, and
// parses the result with the same YAML library the agent uses.
//
// Guards issue #884: the block used to double-quote the Windows paths, and a
// double-quoted YAML scalar processes backslash escapes. "\c" in
// \credentials.yml is not a valid escape, so config.yml never parsed, the agent
// fell back to defaults, and the first save wrote those defaults back over it,
// blanking patchmon_server.
func TestWindowsInstallScriptGeneratesParseableConfig(t *testing.T) {
	const (
		serverURL  = "https://patchmon.example.net"
		configPath = `C:\ProgramData\PatchMon`
	)

	block := extractConfigHereString(t, string(PatchmonWindowsInstallScript))

	// What PowerShell resolves at run time. $ConfigPath and $ServerURL are piped
	// through a -replace that doubles any single quote.
	subs := strings.NewReplacer(
		`$($ServerURL -replace "'", "''")`, serverURL,
		`$($ConfigPath -replace "'", "''")`, configPath,
		`$($SkipSslVerify.ToString().ToLower())`, "false",
	)
	rendered := subs.Replace(block)

	// Anything left means the script changed shape and the substitutions above
	// are stale. Without this the leftovers would parse as ordinary scalars and
	// the test would pass while checking nothing.
	if strings.Contains(rendered, "$") {
		t.Fatalf("unsubstituted PowerShell in the config block, update this test:\n%s", rendered)
	}

	var got struct {
		PatchmonServer  string `yaml:"patchmon_server"`
		APIVersion      string `yaml:"api_version"`
		CredentialsFile string `yaml:"credentials_file"`
		LogFile         string `yaml:"log_file"`
		LogLevel        string `yaml:"log_level"`
		SkipSSLVerify   bool   `yaml:"skip_ssl_verify"`
	}
	if err := yaml.Unmarshal([]byte(rendered), &got); err != nil {
		t.Fatalf("generated config.yml does not parse: %v\n%s", err, rendered)
	}

	// Exact matches, because the failure mode this guards is silent corruption
	// as much as it is a parse error: "\P" is a valid escape and decodes to
	// U+2029, so a double-quoted path can survive parsing and still be wrong.
	for _, c := range []struct{ name, got, want string }{
		{"patchmon_server", got.PatchmonServer, serverURL},
		{"api_version", got.APIVersion, "v1"},
		{"credentials_file", got.CredentialsFile, configPath + `\credentials.yml`},
		{"log_file", got.LogFile, configPath + `\patchmon-agent.log`},
		{"log_level", got.LogLevel, "info"},
	} {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
	if got.SkipSSLVerify {
		t.Error("skip_ssl_verify = true, want false")
	}
}

// The repair branch rewrites a config an older installer left double-quoted, so
// a host that is already broken recovers on re-run. No test executes PowerShell,
// but the pattern itself is RE2-compatible and is lifted from the script rather
// than restated, so a change there is a change here.
func TestWindowsInstallScriptRepairPattern(t *testing.T) {
	re := regexp.MustCompile(extractRepairPattern(t, string(PatchmonWindowsInstallScript)))
	const repl = "${1}${2}: '${3}'${4}${5}"

	for name, c := range map[string]struct{ in, want string }{
		"repairs a broken path": {
			"credentials_file: \"C:\\ProgramData\\PatchMon\\credentials.yml\"\n",
			"credentials_file: 'C:\\ProgramData\\PatchMon\\credentials.yml'\n",
		},
		"preserves CRLF": {
			"log_file: \"C:\\a\\b.log\"\r\n",
			"log_file: 'C:\\a\\b.log'\r\n",
		},
		"preserves indentation and a trailing comment": {
			"  log_file: \"C:\\a\\b.log\"  # note\n",
			"  log_file: 'C:\\a\\b.log'  # note\n",
		},
		"leaves an already single-quoted path alone": {
			"log_file: 'C:\\a\\b.log'\n",
			"log_file: 'C:\\a\\b.log'\n",
		},
		"leaves a double-quoted value with no backslash alone": {
			"patchmon_server: \"https://patchmon.example.net\"\n",
			"patchmon_server: \"https://patchmon.example.net\"\n",
		},
		"leaves an unrelated key alone": {
			"some_other_path: \"C:\\a\\b\"\n",
			"some_other_path: \"C:\\a\\b\"\n",
		},
		"does not touch skip_ssl_verify": {
			"skip_ssl_verify: false\n",
			"skip_ssl_verify: false\n",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := re.ReplaceAllString(c.in, repl); got != c.want {
				t.Errorf("got  %q\nwant %q", got, c.want)
			}
		})
	}

	// The whole point is that the repaired document parses.
	broken := "patchmon_server: \"https://patchmon.example.net\"\n" +
		"credentials_file: \"C:\\ProgramData\\PatchMon\\credentials.yml\"\n" +
		"log_file: \"C:\\ProgramData\\PatchMon\\patchmon-agent.log\"\n"
	var out struct {
		CredentialsFile string `yaml:"credentials_file"`
	}
	if err := yaml.Unmarshal([]byte(re.ReplaceAllString(broken, repl)), &out); err != nil {
		t.Fatalf("repaired config does not parse: %v", err)
	}
	if want := `C:\ProgramData\PatchMon\credentials.yml`; out.CredentialsFile != want {
		t.Errorf("credentials_file = %q, want %q", out.CredentialsFile, want)
	}
}

func extractRepairPattern(t *testing.T, script string) string {
	t.Helper()
	re := regexp.MustCompile(`(?m)^\s*\$doubleQuotedPath = '(.*)'\s*$`)
	m := re.FindStringSubmatch(script)
	if m == nil {
		t.Fatal("could not find the $doubleQuotedPath assignment in patchmon_install_windows.ps1")
	}
	return m[1]
}

func extractConfigHereString(t *testing.T, script string) string {
	t.Helper()
	re := regexp.MustCompile(`(?s)\$configContent = @"\r?\n(.*?)\r?\n"@`)
	m := re.FindStringSubmatch(script)
	if m == nil {
		t.Fatal(`could not find the '$configContent = @"' here-string in patchmon_install_windows.ps1`)
	}
	return m[1]
}
