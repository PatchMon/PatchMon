package packages

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestChocolateyCollection(t *testing.T) {
	for _, tc := range []struct {
		name, version, list, outdated string
		listCode, outdatedCode        int
		wantErr                       bool
	}{
		{name: "normal", version: "2.6.0", list: "App|1.0\r\npinned|2.0\r\ncurrent|3.0", outdated: "app|1.0|2.0|false\npinned|2.0|3.0|true"},
		{name: "enhanced updates", version: "2.6.0", list: "app|1.0", outdated: "app|1.0|2.0|false", outdatedCode: 2},
		{name: "CLI 1 local list", version: "1.4.0", list: "app|1.0", outdated: "app|1.0|2.0|false"},
		{name: "no updates", version: "2.6.0", list: "app|1.0"},
		{name: "empty", version: "2.6.0"},
		{name: "enhanced empty", version: "2.6.0", listCode: 2},
		{name: "packaging revision", version: "2.6.0", list: "app|1.0.20261001", outdated: "app|1.0.20261001|1.0.20261101|false"},
		{name: "prerelease", version: "2.6.0", list: "app|2.0-beta.1", outdated: "app|2.0-beta.1|2.0|false"},
		{name: "list error", version: "2.6.0", listCode: 1, wantErr: true},
		{name: "outdated error", version: "2.6.0", list: "app|1.0", outdatedCode: 1, wantErr: true},
		{name: "partial error", version: "2.6.0", list: "app|1.0", outdated: "app|1.0|2.0|false", outdatedCode: 1, wantErr: true},
		{name: "malformed list", version: "2.6.0", list: "app", wantErr: true},
		{name: "malformed outdated", version: "2.6.0", list: "app|1.0", outdated: "app|1.0|2.0", wantErr: true},
		{name: "bad pin", version: "2.6.0", list: "app|1.0", outdated: "app|1.0|2.0|maybe", wantErr: true},
		{name: "new package mid scan", version: "2.6.0", list: "app|1.0", outdated: "other|1.0|2.0|false", wantErr: true},
		{name: "version changed mid scan", version: "2.6.0", list: "app|1.0", outdated: "app|1.1|2.0|false", wantErr: true},
		{name: "duplicate ID", version: "2.6.0", list: "app|1.0\nAPP|1.0", wantErr: true},
		{name: "duplicate update", version: "2.6.0", list: "app|1.0", outdated: "app|1.0|2.0|false\napp|1.0|2.0|false", wantErr: true},
		{name: "unknown version", version: "not a version", wantErr: true},
		{name: "inconsistent no results", version: "2.6.0", list: "app|1.0", listCode: 2, wantErr: true},
		{name: "missing enhanced updates", version: "2.6.0", list: "app|1.0", outdatedCode: 2, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var commands [][]string
			pkgs, err := chocolateyPackages(func(timeout time.Duration, args ...string) (string, int, error) {
				commands = append(commands, args)
				switch args[0] {
				case "--version":
					return tc.version, 0, nil
				case "list":
					want := []string{"list", "--limit-output", "--no-color"}
					if strings.HasPrefix(tc.version, "1.") {
						want = append(want, "--local-only")
					}
					if !reflect.DeepEqual(args, want) {
						t.Fatalf("list args=%v", args)
					}
					return tc.list, tc.listCode, nil
				case "outdated":
					if timeout != networkCollectorTimeout || !reflect.DeepEqual(args, []string{"outdated", "--limit-output", "--no-color"}) {
						t.Fatalf("outdated invocation=%v %v", timeout, args)
					}
					return tc.outdated, tc.outdatedCode, nil
				default:
					t.Fatalf("unexpected command: %v", args)
					return "", 0, nil
				}
			})
			if tc.wantErr {
				if err == nil || pkgs != nil {
					t.Fatalf("incomplete inventory accepted: %+v, %v", pkgs, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.list == "" && len(commands) != 2 {
				t.Fatalf("empty inventory queried remote sources: %v", commands)
			}
			for _, p := range pkgs {
				if !strings.HasPrefix(p.Name, "chocolatey:") || p.SourceRepository != "chocolatey" || p.WingetID != "" || p.WUAGuid != "" {
					t.Fatalf("unsafe identity/routing: %+v", p)
				}
				id := strings.TrimPrefix(p.Name, "chocolatey:")
				wantUpdate := strings.Contains(strings.ToLower(tc.outdated), id+"|")
				if p.NeedsUpdate != wantUpdate || (p.AvailableVersion != "") != wantUpdate {
					t.Fatalf("wrong availability: %+v", p)
				}
				if id == "pinned" && !strings.Contains(p.Description, "pinned") {
					t.Fatalf("missing pin: %+v", p)
				}
			}
		})
	}
}

func TestChocolateyCommandFailure(t *testing.T) {
	failure := errors.New("deadline exceeded")
	for _, command := range []string{"--version", "list", "outdated"} {
		t.Run(command, func(t *testing.T) {
			pkgs, err := chocolateyPackages(func(_ time.Duration, args ...string) (string, int, error) {
				if args[0] == command {
					return "", 0, failure
				}
				if args[0] == "--version" {
					return "2.6.0", 0, nil
				}
				return "app|1.0", 0, nil
			})
			if !errors.Is(err, failure) || pkgs != nil {
				t.Fatalf("failure swallowed: %+v %v", pkgs, err)
			}
		})
	}
}

func TestChocolateyVersionFailure(t *testing.T) {
	pkgs, err := chocolateyPackages(func(time.Duration, ...string) (string, int, error) { return "", 1, nil })
	if err == nil || pkgs != nil {
		t.Fatalf("failed version query accepted: %+v %v", pkgs, err)
	}
}

func TestFindChocolatey(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ChocolateyInstall", root)
	t.Setenv("ProgramData", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	if exe, err := findChocolatey(); err != nil || exe != "" {
		t.Fatalf("absent Chocolatey: %q %v", exe, err)
	}
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(bin, "choco.exe")
	if err := os.WriteFile(want, []byte("fixture"), 0700); err != nil {
		t.Fatal(err)
	}
	if exe, err := findChocolatey(); err != nil || exe != want {
		t.Fatalf("custom root: %q %v", exe, err)
	}
	t.Setenv("ChocolateyInstall", "")
	t.Setenv("ProgramData", filepath.Dir(root))
	t.Setenv("PATH", bin)
	if exe, err := findChocolatey(); err != nil || exe != want {
		t.Fatalf("PATH fallback: %q %v", exe, err)
	}
	t.Setenv("PATH", t.TempDir())
	programData := t.TempDir()
	t.Setenv("ProgramData", programData)
	standard := filepath.Join(programData, "chocolatey", "bin", "choco.exe")
	if err := os.MkdirAll(filepath.Dir(standard), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(standard, []byte("fixture"), 0700); err != nil {
		t.Fatal(err)
	}
	if exe, err := findChocolatey(); err != nil || exe != standard {
		t.Fatalf("default root without refreshed PATH: %q %v", exe, err)
	}
}
