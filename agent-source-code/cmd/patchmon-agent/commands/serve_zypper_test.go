package commands

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"patchmon-agent/internal/packages"
)

func zypperExit(t *testing.T, code int) error {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("zypper runs on Linux only")
	}
	return exec.Command("sh", "-c", fmt.Sprintf("exit %d", code)).Run()
}

type zypperResponse struct {
	out  string
	code int
}

type fakeZypper struct {
	t             *testing.T
	plans         []zypperResponse
	runs          map[string][]zypperResponse
	locks         []packages.ZypperLock
	calls         []string
	notes         []string
	rolling       bool
	transactional bool
}

func (f *fakeZypper) next(queue []zypperResponse) (zypperResponse, []zypperResponse) {
	if len(queue) == 0 {
		return zypperResponse{}, nil
	}
	if len(queue) == 1 {
		return queue[0], queue
	}
	return queue[0], queue[1:]
}

func (f *fakeZypper) err(code int) error {
	if code == 0 {
		return nil
	}
	return zypperExit(f.t, code)
}

func (f *fakeZypper) io() zypperPatchIO {
	return zypperPatchIO{
		run: func(args ...string) (string, error) {
			key := strings.Join(args, " ")
			f.calls = append(f.calls, key)
			for prefix, queue := range f.runs {
				if strings.Contains(key, prefix) {
					var r zypperResponse
					r, f.runs[prefix] = f.next(queue)
					return r.out, f.err(r.code)
				}
			}
			return "", nil
		},
		plan: func(args ...string) ([]byte, error) {
			f.calls = append(f.calls, strings.Join(args, " "))
			var r zypperResponse
			r, f.plans = f.next(f.plans)
			return []byte(r.out), f.err(r.code)
		},
		note:          func(msg string) { f.notes = append(f.notes, msg) },
		locks:         func() ([]packages.ZypperLock, error) { return f.locks, nil },
		rolling:       f.rolling,
		transactional: f.transactional,
	}
}

func (f *fakeZypper) count(substr string) int {
	n := 0
	for _, c := range f.calls {
		if strings.Contains(c, substr) {
			n++
		}
	}
	return n
}

func (f *fakeZypper) noted(substr string) bool {
	for _, n := range f.notes {
		if strings.Contains(n, substr) {
			return true
		}
	}
	return false
}

func zypperFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("../../../internal/packages/testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

const zypperNothingXML = `<?xml version='1.0'?>
<stream><install-summary packages-to-change="0"></install-summary><message type="info">Nothing to do.</message></stream>`

func TestZypperPatchAllUpdatesTheStackThenTheRest(t *testing.T) {
	stack := zypperFixture(t, "zypper-leap155-patch-dry-stack.xml")
	rest := zypperFixture(t, "zypper-leap155-patch-security-dry.xml")
	f := &fakeZypper{
		t:     t,
		plans: []zypperResponse{{stack, 103}, {rest, 0}},
		runs:  map[string][]zypperResponse{" patch ": {{"", 103}, {"", 0}}},
		locks: []packages.ZypperLock{{Name: "bash", Type: "package"}},
	}
	if err := runZypperPatch(f.io(), "patch_all", nil, false); err != nil {
		t.Fatal(err)
	}
	if f.count("--non-interactive refresh") != 1 || f.count("--dry-run") != 2 || f.count(" patch --with-update --auto-agree-with-licenses") != 4 {
		t.Errorf("calls %q", f.calls)
	}
	if !f.noted("zypper updated itself") || !f.noted("left alone: bash") || !f.noted("upgrade curl") {
		t.Errorf("notes %q", f.notes)
	}
}

func TestZypperDryRunWithStackUpdate(t *testing.T) {
	f := &fakeZypper{t: t, plans: []zypperResponse{{zypperFixture(t, "zypper-leap155-patch-dry-stack.xml"), 103}}}
	if err := runZypperPatch(f.io(), "patch_all", nil, true); err != nil {
		t.Fatal(err)
	}
	if f.count("--dry-run") != 1 || len(f.calls) != 2 {
		t.Errorf("a dry run must not change anything: %q", f.calls)
	}
	if !f.noted("has to update itself first") {
		t.Errorf("notes %q", f.notes)
	}
}

func TestZypperRollingUsesDistUpgrade(t *testing.T) {
	f := &fakeZypper{t: t, rolling: true, plans: []zypperResponse{{zypperFixture(t, "zypper-tumbleweed-dup-dry.xml"), 0}}}
	if err := runZypperPatch(f.io(), "patch_all", nil, false); err != nil {
		t.Fatal(err)
	}
	if f.count("dist-upgrade --auto-agree-with-licenses --dry-run") != 1 || f.count("dist-upgrade --auto-agree-with-licenses") != 2 {
		t.Errorf("calls %q", f.calls)
	}
	if !f.noted("Removing libcurl-mini4") {
		t.Errorf("notes %q", f.notes)
	}
}

func TestZypperSelectedPackages(t *testing.T) {
	f := &fakeZypper{
		t:       t,
		rolling: true,
		plans:   []zypperResponse{{zypperFixture(t, "zypper-leap155-patch-security-dry.xml"), 0}},
		locks:   []packages.ZypperLock{{Name: "libcurl4", Type: "package"}},
	}
	if err := runZypperPatch(f.io(), "patch_package", []string{"curl", "libcurl4"}, false); err != nil {
		t.Fatal(err)
	}
	if f.count("update --auto-agree-with-licenses -- curl") != 1 || f.count("libcurl4") != 0 {
		t.Errorf("calls %q", f.calls)
	}
	if !f.noted("left alone: libcurl4") || !f.noted("partial update") {
		t.Errorf("notes %q", f.notes)
	}

	f = &fakeZypper{t: t, locks: []packages.ZypperLock{{Name: "curl*", Type: "package"}}}
	if err := runZypperPatch(f.io(), "patch_package", []string{"curl"}, false); err == nil || !strings.Contains(err.Error(), "locked") {
		t.Errorf("err %v", err)
	}
	if f.count("update") != 0 {
		t.Errorf("calls %q", f.calls)
	}
}

func TestZypperRefusesUnsafePlansAndHosts(t *testing.T) {
	downgrade := `<?xml version='1.0'?>
<stream><install-summary packages-to-change="1"><to-downgrade><solvable type="package" name="vim" edition="9.0-1.1" edition-old="9.1-2.1"/></to-downgrade></install-summary></stream>`
	f := &fakeZypper{t: t, plans: []zypperResponse{{downgrade, 0}}}
	if err := runZypperPatch(f.io(), "patch_all", nil, false); err == nil || !strings.Contains(err.Error(), "downgrade vim") {
		t.Errorf("err %v", err)
	}
	if f.count("--dry-run") != len(f.calls)-1 {
		t.Errorf("a refused plan must not be applied: %q", f.calls)
	}

	f = &fakeZypper{t: t, transactional: true}
	if err := runZypperPatch(f.io(), "patch_all", nil, true); err == nil || !strings.Contains(err.Error(), "transactional-update") || len(f.calls) != 0 {
		t.Errorf("err %v calls %q", err, f.calls)
	}
}

func TestZypperExitCodes(t *testing.T) {
	plan := zypperFixture(t, "zypper-leap155-patch-security-dry.xml")
	cases := []struct {
		name    string
		planOut string
		plan    int
		run     int
		wantErr string
		note    string
	}{
		{"reboot needed", plan, 0, 102, "", "needs a reboot"},
		{"repository skipped", plan, 0, 106, "", "skipped a repository"},
		{"rpm script failed", plan, 0, 107, "RPM script failed", ""},
		{"locked during the run", plan, 0, 7, "zypp lock", ""},
		{"locked while planning", zypperFixture(t, "zypper-leap155-locked.xml"), 7, 0, "zypp lock", ""},
		{"unknown package", zypperFixture(t, "zypper-leap155-update-notfound.xml"), 104, 0, "not available", ""},
		{"commit error", plan, 0, 8, "could not commit", ""},
		{"stack never settles", plan, 0, 103, "after 3 passes", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeZypper{
				t:     t,
				plans: []zypperResponse{{tc.planOut, tc.plan}},
				runs:  map[string][]zypperResponse{" patch ": {{"", tc.run}}},
			}
			err := runZypperPatch(f.io(), "patch_all", nil, false)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("unexpected error %v", err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Fatalf("err %v, want %q", err, tc.wantErr)
			}
			if tc.note != "" && !f.noted(tc.note) {
				t.Errorf("notes %q", f.notes)
			}
		})
	}
}

func TestZypperUnresolvedProblemAfterStackUpdate(t *testing.T) {
	f := &fakeZypper{
		t: t,
		plans: []zypperResponse{
			{zypperFixture(t, "zypper-leap155-patch-dry-stack.xml"), 103},
			{zypperFixture(t, "zypper-leap160-patch-dry-problem.xml"), 4},
		},
		runs: map[string][]zypperResponse{" patch ": {{"", 103}}},
	}
	err := runZypperPatch(f.io(), "patch_all", nil, false)
	if err == nil || !strings.Contains(err.Error(), "cannot resolve the dependencies") || !strings.Contains(err.Error(), "systemd-sysvcompat") {
		t.Fatalf("err %v", err)
	}
	if !f.noted("own update was installed") || f.count(" patch --with-update --auto-agree-with-licenses") != 3 {
		t.Errorf("calls %q notes %q", f.calls, f.notes)
	}
}

func TestZypperRefreshFailure(t *testing.T) {
	plan := zypperFixture(t, "zypper-leap155-patch-security-dry.xml")
	f := &fakeZypper{
		t:     t,
		plans: []zypperResponse{{plan, 0}},
		runs:  map[string][]zypperResponse{"--non-interactive refresh": {{"", 4}}},
	}
	if err := runZypperPatch(f.io(), "patch_all", nil, false); err != nil {
		t.Fatalf("an unreachable repository must not stop the run: %v", err)
	}
	if !f.noted("continuing with cached metadata") || f.count(" patch --with-update") != 2 {
		t.Errorf("calls %q notes %q", f.calls, f.notes)
	}

	f = &fakeZypper{t: t, runs: map[string][]zypperResponse{"--non-interactive refresh": {{"", 7}}}}
	if err := runZypperPatch(f.io(), "patch_all", nil, false); err == nil || !strings.Contains(err.Error(), "zypp lock") {
		t.Errorf("a held zypp lock must fail the run: %v", err)
	}
	if len(f.calls) != 1 {
		t.Errorf("calls %q", f.calls)
	}
}

func TestZypperNothingToDo(t *testing.T) {
	f := &fakeZypper{t: t, plans: []zypperResponse{{zypperNothingXML, 0}}}
	if err := runZypperPatch(f.io(), "patch_all", nil, false); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 2 || !f.noted("nothing to do") {
		t.Errorf("calls %q notes %q", f.calls, f.notes)
	}
}
