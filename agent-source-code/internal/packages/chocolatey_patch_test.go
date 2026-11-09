package packages

import (
	"context"
	"errors"
	"testing"

	"patchmon-agent/pkg/models"
)

type fakeChocolatey struct {
	pkgs      []models.Package
	code      uint32
	err       error
	calls     []string
	unchanged bool
	onUpgrade func(context.Context)
}

func (f *fakeChocolatey) Scan(context.Context) ([]models.Package, error) { return f.pkgs, f.err }
func (f *fakeChocolatey) Upgrade(ctx context.Context, id, version string, _ func(string)) (uint32, error) {
	f.calls = append(f.calls, id)
	if f.onUpgrade != nil {
		f.onUpgrade(ctx)
	}
	if (f.code == 0 || f.code == 3010 || f.code == 1641) && !f.unchanged {
		for i := range f.pkgs {
			if f.pkgs[i].ChocolateyID == id {
				f.pkgs[i].CurrentVersion = version
				f.pkgs[i].NeedsUpdate = false
			}
		}
	}
	return f.code, f.err
}
func chocoFixture(id string, pinned bool) models.Package {
	return models.Package{ChocolateyID: id, ChocolateyPinned: pinned, Name: "chocolatey:" + id, CurrentVersion: "1.0", AvailableVersion: "2.0", NeedsUpdate: true}
}
func runChocoFake(req WindowsPatchRequest, f *fakeChocolatey) WindowsPatchResult {
	r := newWindowsPatchRun(req, WindowsPatchHooks{}, &fakeWUA{}, &fakeWinget{})
	r.chocolatey = f
	return r.run(context.Background(), context.Background())
}
func TestChocolateySelectedPatching(t *testing.T) {
	for _, tc := range []struct {
		name                            string
		dry, pinned, missing, unchanged bool
		onUpgrade                       func(context.Context)
		code                            uint32
		want                            string
		calls                           int
		reboot                          bool
	}{
		{name: "selected only", want: itemInstalled, calls: 1},
		{name: "dry run", dry: true, want: itemWouldInstall},
		{name: "pinned", pinned: true, want: itemSkipped},
		{name: "removed", missing: true, want: itemNotApplicable},
		{name: "failure", code: 1, want: itemFailed, calls: 1},
		{name: "no action", code: 2, want: itemNotApplicable, calls: 1},
		{name: "reboot success", code: 3010, want: itemPendingReboot, calls: 1, reboot: true},
		{name: "reboot exit without upgrade", code: 3010, unchanged: true, want: itemFailed, calls: 1, reboot: true},
		{name: "pending reboot", code: 350, want: itemSkipped, calls: 1, reboot: true},
		{name: "suspended", code: 1604, want: itemFailed, calls: 1, reboot: true},
		{name: "exit zero without upgrade", unchanged: true, want: itemFailed, calls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeChocolatey{pkgs: []models.Package{chocoFixture("one", tc.pinned), chocoFixture("other", false)}, code: tc.code, unchanged: tc.unchanged}
			if tc.missing {
				f.pkgs = f.pkgs[1:]
			}
			res := runChocoFake(WindowsPatchRequest{DryRun: tc.dry, Targets: []WindowsPatchTarget{{Kind: WindowsTargetChocolatey, ID: "one"}}}, f)
			if len(res.Items) != 1 || res.Items[0].Status != tc.want || len(f.calls) != tc.calls || res.RebootRequired != tc.reboot {
				t.Fatalf("result=%+v calls=%v", res, f.calls)
			}
			for _, id := range f.calls {
				if id != "one" {
					t.Fatalf("unselected upgrade %s", id)
				}
			}
		})
	}
}
func TestChocolateyPatchAllScope(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		f := &fakeChocolatey{pkgs: []models.Package{chocoFixture("one", false), chocoFixture("pinned", true)}}
		res := runChocoFake(WindowsPatchRequest{PatchAll: true, IncludeApps: true, IncludeChocolatey: enabled}, f)
		if enabled {
			if len(f.calls) != 1 || f.calls[0] != "one" || len(res.Items) != 2 {
				t.Fatalf("result=%+v calls=%v", res, f.calls)
			}
		} else if len(f.calls) != 0 || len(res.Items) != 0 {
			t.Fatal("older server unexpectedly enabled Chocolatey")
		}
	}
}
func TestChocolateyFailureAndRebootStop(t *testing.T) {
	f := &fakeChocolatey{err: errors.New("feed unavailable")}
	res := runChocoFake(WindowsPatchRequest{PatchAll: true, IncludeApps: true, IncludeChocolatey: true}, f)
	if res.Stage != "failed" || len(f.calls) != 0 {
		t.Fatalf("scan failure accepted: %+v", res)
	}
	f = &fakeChocolatey{pkgs: []models.Package{chocoFixture("one", false), chocoFixture("two", false)}, code: 3010}
	res = runChocoFake(WindowsPatchRequest{Targets: []WindowsPatchTarget{{Kind: WindowsTargetChocolatey, ID: "one"}, {Kind: WindowsTargetChocolatey, ID: "two"}}}, f)
	if len(f.calls) != 1 || res.Items[1].Status != itemSkipped {
		t.Fatalf("continued after reboot: %+v", res)
	}
}
func TestChocolateyTargetValidation(t *testing.T) {
	for _, id := range []string{"all", "ALL", "--force", "a b", "a;calc", "../file", "$(calc)", ""} {
		if ValidWindowsPatchTarget(WindowsPatchTarget{Kind: WindowsTargetChocolatey, ID: id}) {
			t.Fatalf("accepted %q", id)
		}
	}
	for _, id := range []string{"7zip.install", "git", "dotnet-8.0-sdk"} {
		if !ValidWindowsPatchTarget(WindowsPatchTarget{Kind: WindowsTargetChocolatey, ID: id}) {
			t.Fatalf("rejected %q", id)
		}
	}
}

func TestChocolateyStopLetsActiveUpgradeFinish(t *testing.T) {
	stopped := false
	stopCtx, stop := context.WithCancel(context.Background())
	defer stop()
	f := &fakeChocolatey{pkgs: []models.Package{chocoFixture("one", false), chocoFixture("two", false)}}
	f.onUpgrade = func(ctx context.Context) {
		stopped = true
		stop()
		if ctx.Err() != nil {
			t.Fatal("stop cancelled active installer")
		}
	}
	r := newWindowsPatchRun(WindowsPatchRequest{Targets: []WindowsPatchTarget{{Kind: WindowsTargetChocolatey, ID: "one"}, {Kind: WindowsTargetChocolatey, ID: "two"}}}, WindowsPatchHooks{Stopped: func() bool { return stopped }}, &fakeWUA{}, &fakeWinget{})
	r.chocolatey = f
	res := r.run(context.Background(), stopCtx)
	if len(f.calls) != 1 || len(res.Items) != 2 || res.Items[0].Status != itemInstalled || res.Items[1].Status != itemSkipped {
		t.Fatalf("result=%+v calls=%v", res, f.calls)
	}
}
