package packages

import (
	"context"
	"errors"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"patchmon-agent/internal/winexec"
	"patchmon-agent/pkg/models"
)

func TestValidWindowsPatchTarget(t *testing.T) {
	cases := []struct {
		kind, id string
		want     bool
	}{
		{WindowsTargetWUA, "7f1b0c5e-1234-4abc-9def-0123456789ab", true},
		{WindowsTargetWUA, "7F1B0C5E-1234-4ABC-9DEF-0123456789AB", true},
		{WindowsTargetWUA, "KB5044284", false},
		{WindowsTargetWUA, "7f1b0c5e-1234-4abc-9def-0123456789ab' OR 1=1", false},
		{WindowsTargetWinget, "Notepad++.Notepad++", true},
		{WindowsTargetWinget, "Microsoft.VCRedist.2015+.x64", true},
		{WindowsTargetWinget, "9NBLGGH4NNS1", true},
		{WindowsTargetWinget, "7zip.7zip", true},
		{WindowsTargetWinget, "x';calc;'", false},
		{WindowsTargetWinget, `a"b`, false},
		{WindowsTargetWinget, "a b", false},
		{WindowsTargetWinget, "a;b", false},
		{WindowsTargetWinget, "$(calc)", false},
		{WindowsTargetWinget, "a`b", false},
		{WindowsTargetWinget, "--force", false},
		{WindowsTargetWinget, "-h", false},
		{WindowsTargetWinget, `..\evil`, false},
		{WindowsTargetWinget, "a" + strings.Repeat("b", 128), false},
		{WindowsTargetWinget, "a" + strings.Repeat("b", 127), true},
		{"apt", "vim", false},
		{"", "", false},
	}
	for _, c := range cases {
		if got := ValidWindowsPatchTarget(WindowsPatchTarget{Kind: c.kind, ID: c.id}); got != c.want {
			t.Errorf("ValidWindowsPatchTarget(%q, %q) = %v, want %v", c.kind, c.id, got, c.want)
		}
	}
}

func boolPtr(b bool) *bool { return &b }

func TestNewWindowsPatchRequest(t *testing.T) {
	req, _, err := NewWindowsPatchRequest("patch_all", false, nil, nil, nil)
	if err != nil || !req.PatchAll || !req.IncludeUpdates || !req.IncludeApps {
		t.Fatalf("missing scope flags must default on: %+v %v", req, err)
	}
	req, _, err = NewWindowsPatchRequest("patch_all", true, nil, boolPtr(false), nil)
	if err != nil || req.IncludeUpdates || !req.IncludeApps || !req.DryRun {
		t.Fatalf("include_updates=false not honoured: %+v %v", req, err)
	}
	if _, _, err = NewWindowsPatchRequest("patch_all", false, nil, boolPtr(false), boolPtr(false)); err == nil {
		t.Fatal("patch_all with both scopes off must be rejected")
	}

	targets := []WindowsPatchTarget{
		{Kind: WindowsTargetWUA, ID: "7F1B0C5E-1234-4ABC-9DEF-0123456789AB"},
		{Kind: WindowsTargetWUA, ID: "7f1b0c5e-1234-4abc-9def-0123456789ab"},
		{Kind: WindowsTargetWinget, ID: "x';calc;'"},
		{Kind: WindowsTargetWinget, ID: "7zip.7zip"},
	}
	req, rejected, err := NewWindowsPatchRequest("patch_package", false, targets, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []WindowsPatchTarget{
		{Kind: WindowsTargetWUA, ID: "7f1b0c5e-1234-4abc-9def-0123456789ab"},
		{Kind: WindowsTargetWinget, ID: "7zip.7zip"},
	}
	if !reflect.DeepEqual(req.Targets, want) {
		t.Errorf("targets = %+v, want %+v", req.Targets, want)
	}
	if len(rejected) != 1 || rejected[0].ID != "x';calc;'" {
		t.Errorf("rejected = %+v", rejected)
	}
	if _, _, err = NewWindowsPatchRequest("patch_package", false, []WindowsPatchTarget{{Kind: "winget", ID: "-x"}}, nil, nil); err == nil {
		t.Fatal("patch_package with no valid targets must be rejected")
	}
}

const (
	guidA = "aaaaaaaa-0000-0000-0000-000000000001"
	guidB = "bbbbbbbb-0000-0000-0000-000000000002"
	guidC = "cccccccc-0000-0000-0000-000000000003"
	guidD = "dddddddd-0000-0000-0000-000000000004"
	guidE = "eeeeeeee-0000-0000-0000-000000000005"
	guidF = "ffffffff-0000-0000-0000-000000000006"
)

func sampleScan() wuaScan {
	return wuaScan{Updates: []wuaUpdate{
		{ID: guidA, Title: "Zeta app update", KBs: jsonStrings{"KB1"}, Type: 1},
		{ID: guidB, Title: "Cumulative Update", KBs: jsonStrings{"KB2"}, Type: 1, CategoryIDs: jsonStrings{wuaCategorySecurity}},
		{ID: guidC, Title: "Driver", Type: 2},
		{ID: guidD, Title: "Optional preview", Type: 1, BrowseOnly: true},
		{ID: guidE, Title: "Windows 11, version 24H2", Type: 1, CategoryIDs: jsonStrings{wuaCategoryUpgrades}},
		{ID: guidF, Title: "Feature by name", Type: 1, Categories: jsonStrings{"Upgrades"}},
	}}
}

func ids(us []wuaUpdate) []string {
	var out []string
	for _, u := range us {
		out = append(out, u.ID)
	}
	return out
}

func TestSelectWUAUpdatesPatchAll(t *testing.T) {
	install, decided := selectWUAUpdates(sampleScan(), true, nil)
	if got, want := ids(install), []string{guidB, guidA}; !reflect.DeepEqual(got, want) {
		t.Errorf("patch_all install = %v, want %v (security first, then title; no driver, optional or feature)", got, want)
	}
	if len(decided) != 0 {
		t.Errorf("patch_all must not report items for excluded classes: %+v", decided)
	}
}

func TestSelectWUAUpdatesPatchPackage(t *testing.T) {
	missing := "12345678-0000-0000-0000-000000000000"
	install, decided := selectWUAUpdates(sampleScan(), false, []string{guidE, guidC, strings.ToUpper(missing)})
	if got, want := ids(install), []string{guidC, guidE}; !reflect.DeepEqual(got, want) {
		t.Errorf("explicit targets = %v, want %v (driver and feature upgrade allowed when named)", got, want)
	}
	if len(decided) != 1 || decided[0].ID != missing || decided[0].Status != itemNotApplicable {
		t.Errorf("missing target must be not_applicable: %+v", decided)
	}
}

func TestSelectWUAUpdatesBlockers(t *testing.T) {
	scan := wuaScan{Updates: []wuaUpdate{
		{ID: guidA, Title: "Pending restart", Type: 1, PendingReboot: true},
		{ID: guidB, Title: "Interactive", Type: 1, CanRequestUserInput: true},
		{ID: guidC, Title: "Normal", Type: 1},
	}}
	install, decided := selectWUAUpdates(scan, true, nil)
	if len(install) != 1 || install[0].ID != guidC {
		t.Fatalf("install = %v", ids(install))
	}
	byID := map[string]models.PatchRunItem{}
	for _, it := range decided {
		byID[it.ID] = it
	}
	if it := byID[guidA]; it.Status != itemPendingReboot || !it.RebootRequired {
		t.Errorf("installed-pending-reboot must never reinstall: %+v", it)
	}
	if it := byID[guidB]; it.Status != itemSkipped {
		t.Errorf("interactive update must be skipped: %+v", it)
	}

	scan.RebootRequiredBeforeInstall = true
	install, decided = selectWUAUpdates(scan, true, nil)
	if len(install) != 0 {
		t.Fatalf("restart required before install must block every update: %v", ids(install))
	}
	for _, it := range decided {
		if it.ID == guidC && (it.Status != itemSkipped || it.Message != msgRestartFirst) {
			t.Errorf("blocked update = %+v", it)
		}
	}
}

func TestWUAResultMapping(t *testing.T) {
	cases := map[int]string{2: itemInstalled, 3: itemInstalledWithErrors, 4: itemFailed, 5: itemAborted, 0: itemFailed, 1: itemFailed}
	for rc, want := range cases {
		if got := wuaResultStatus(rc); got != want {
			t.Errorf("rc %d = %q, want %q", rc, got, want)
		}
	}
	if msg := wuaResultMessage(0, ""); msg != "Did not run" {
		t.Errorf("rc 0 message = %q", msg)
	}
	if msg := wuaResultMessage(4, "0x800F0922"); strings.Contains(msg, "0x") || !strings.Contains(msg, "CBS.log") {
		t.Errorf("failure message must carry the hint and leave the code to hresult: %q", msg)
	}
	if msg := wuaResultMessage(2, "0x00000000"); msg != "Installed" {
		t.Errorf("zero HRESULT must not be shown: %q", msg)
	}
	if wuaHResultHint("80240016") == "" || wuaHResultHint("0X8024402F") == "" {
		t.Error("HRESULT hints must match regardless of case or prefix")
	}
	if !isInstallBlockedHResult("0x80240016") || isInstallBlockedHResult("0x80240017") {
		t.Error("install-blocked detection is wrong")
	}
}

func TestClassifyWingetUpgradeExit(t *testing.T) {
	negative := int32(-1978335189)
	cases := []struct {
		code   uint32
		dry    bool
		output string
		status string
		reboot bool
	}{
		{0, false, "", itemInstalled, false},
		{uint32(negative), false, "", itemNotApplicable, false},
		{wingetNoPackageFound, false, "", itemNotApplicable, false},
		{wingetUserScopeFromAdmin, false, "", itemNotApplicable, false},
		{wingetRebootToFinish, false, "", itemInstalled, true},
		{wingetPackageInUse, false, "", itemFailed, false},
		{wingetInstallInProgress, false, "", itemFailed, false},
		{wingetPinned, false, "", itemSkipped, false},
		{0x8A150008, false, "", itemFailed, false},
		{1, false, "", itemFailed, false},
		{0, true, "Name   Id         Version Available\n7-Zip  7zip.7zip  23.01   24.08\n", itemWouldInstall, false},
		{0, true, "No installed package found matching input criteria.\n", itemNotApplicable, false},
		{wingetUpdateNotApplicable, true, "", itemNotApplicable, false},
		{wingetPinned, true, "", itemSkipped, false},
		{0x8A150008, true, "", itemFailed, false},
	}
	for _, c := range cases {
		got := classifyWingetUpgradeExit(c.code, c.dry, c.output, "7zip.7zip")
		if got.Status != c.status || got.RebootRequired != c.reboot {
			t.Errorf("code %s dry=%v = %+v, want %s reboot=%v", hexCode(c.code), c.dry, got, c.status, c.reboot)
		}
	}
	if !strings.Contains(classifyWingetUpgradeExit(wingetNoPackageFound, false, "", "x").Message, "single user") {
		t.Error("user-scope message missing")
	}
	for _, code := range []uint32{0x8A150008, 0x80072EE7, 0x12345678, wingetPackageInUse, wingetInstallInProgress, wingetRebootForInstall} {
		for _, dry := range []bool{false, true} {
			if msg := classifyWingetUpgradeExit(code, dry, "", "x").Message; strings.Contains(msg, "0x") || msg == "" {
				t.Errorf("code %s: message %q must be plain text; the code is in exit_code", hexCode(code), msg)
			}
		}
	}
	if msg := classifyWingetUpgradeExit(0x80072EE7, false, "", "x").Message; !strings.Contains(msg, "DNS") {
		t.Errorf("0x80072EE7 message = %q", msg)
	}
}

func TestWingetArgs(t *testing.T) {
	if got, want := wingetUpgradeArgs("Notepad++.Notepad++"), []string{"upgrade", "--id", "Notepad++.Notepad++", "--exact", "--source", "winget", "--silent", "--accept-source-agreements", "--accept-package-agreements", "--disable-interactivity"}; !reflect.DeepEqual(got, want) {
		t.Errorf("upgrade args = %q", got)
	}
	if got, want := wingetListArgs("7zip.7zip"), []string{"list", "--id", "7zip.7zip", "--exact", "--source", "winget", "--upgrade-available", "--accept-source-agreements", "--disable-interactivity"}; !reflect.DeepEqual(got, want) {
		t.Errorf("list args = %q", got)
	}
	if got, want := wingetInstalledArgs("7zip.7zip"), []string{"list", "--id", "7zip.7zip", "--exact", "--source", "winget", "--accept-source-agreements", "--disable-interactivity"}; !reflect.DeepEqual(got, want) {
		t.Errorf("installed args = %q", got)
	}
	for _, a := range wingetListArgs("x") {
		if a == "upgrade" {
			t.Fatal("dry run must never call winget upgrade")
		}
	}
}

func TestLineLogger(t *testing.T) {
	var got []string
	l := newLineLogger(func(s string) { got = append(got, s) }, 1<<20)
	_, _ = l.Write([]byte("  -\r  \\\r"))
	_, _ = l.Write([]byte("Successfully \x1b[1minstalled\x1b[0m\r\nNo trailing"))
	l.Flush()
	if want := []string{"Successfully installed\n", "No trailing\n"}; !reflect.DeepEqual(got, want) {
		t.Errorf("lines = %q, want %q", got, want)
	}
	if l.Output() != "Successfully installed\nNo trailing\n" {
		t.Errorf("captured = %q", l.Output())
	}
}

func TestParseWUAOutput(t *testing.T) {
	in := "\xEF\xBB\xBF" + `{"type":"log","message":"hello"}` + "\n" +
		"WARNING: not json\r\n" +
		`{"type":"update","id":"` + guidA + `","title":"T","kbs":"KB1","categories":["Security Updates"],"category_ids":["` + wuaCategorySecurity + `"],"type_code":2,"browse_only":true,"pending_reboot":true}` + "\n" +
		`{"type":"update_result","id":"` + guidB + `","result_code":4,"hresult":"0x80246007"}` + "\n" +
		`{"type":"result","op":"scan","count":1,"reboot_required_before_install":true}`
	var logs []string
	out, err := parseWUAOutput(strings.NewReader(in), func(s string) { logs = append(logs, s) })
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Updates) != 1 {
		t.Fatalf("updates = %+v", out.Updates)
	}
	u := out.Updates[0]
	if u.ID != guidA || !reflect.DeepEqual([]string(u.KBs), []string{"KB1"}) || u.Type != 2 || !u.BrowseOnly || !u.PendingReboot || !isSecurityOrCritical(u) {
		t.Errorf("update = %+v", u)
	}
	if len(out.Results) != 1 || out.Results[0].ResultCode != 4 {
		t.Errorf("results = %+v", out.Results)
	}
	if out.Final == nil || !out.Final.RebootRequiredBeforeInstall {
		t.Errorf("final = %+v", out.Final)
	}
	if !reflect.DeepEqual(logs, []string{"hello\n", "WARNING: not json\n"}) {
		t.Errorf("logs = %q", logs)
	}

	if _, err := parseWUAOutput(strings.NewReader(`{"type":"log","message":"x"}`), func(string) {}); !errors.Is(err, errWUANoResult) {
		t.Errorf("missing terminal line must fail, got %v", err)
	}
	if _, err := parseWUAOutput(strings.NewReader(""), func(string) {}); !errors.Is(err, errWUANoResult) {
		t.Errorf("empty output must fail, got %v", err)
	}
	_, err = parseWUAOutput(strings.NewReader(`{"type":"error","message":"boom","hresult":"0x80070422"}`), func(string) {})
	var se *wuaScriptError
	if !errors.As(err, &se) || !strings.Contains(err.Error(), "disabled") {
		t.Errorf("error line = %v", err)
	}
	install := `{"type":"result","op":"install","id":"` + guidA + `","found":true,"result_code":3,"hresult":"0x00000000","reboot_required":true}`
	out, err = parseWUAOutput(strings.NewReader(install), func(string) {})
	if err != nil || out.Final.ResultCode != 3 || !out.Final.RebootRequired || !out.Final.Found {
		t.Errorf("install result = %+v %v", out.Final, err)
	}
}

func TestWUAScriptIsStatic(t *testing.T) {
	if wuaScript == "" {
		t.Fatal("wua.ps1 is not embedded")
	}
	if regexp.MustCompile(`%[-+# 0-9.]*[sdvqxXtTwf]`).MatchString(wuaScript) {
		t.Error("wua.ps1 contains a Go format verb; it must never be built with fmt")
	}
	args := wuaPowerShellArgs()
	if args[len(args)-2] != "-Command" || args[len(args)-1] != winexec.Script(wuaPendingScript+wuaScript) {
		t.Error("the executed script must be exactly the embedded file behind the UTF-8 preamble")
	}
	for _, want := range []string{"[Console]::In.ReadToEnd()", "IsInstalled=0 AND IsHidden=0", "Get-InProgressIds $session", `{12}\z'`, "$guidPattern", "-notmatch $guidPattern"} {
		if !strings.Contains(wuaScript, want) {
			t.Errorf("wua.ps1 lacks %q", want)
		}
	}
}

func TestFinalWindowsStage(t *testing.T) {
	ok := models.PatchRunItem{ID: "a", Status: itemInstalled}
	failed := models.PatchRunItem{ID: "b", KB: "KB5044284", Status: itemFailed, HResult: "0x800f0922"}
	skipped := models.PatchRunItem{ID: "7zip.7zip", Status: itemSkipped, Message: msgPinned}
	code := int64(wingetPackageInUse)
	inUse := models.PatchRunItem{ID: "d", Title: "Notepad++", Status: itemFailed, ExitCode: &code}
	cases := []struct {
		name    string
		items   []models.PatchRunItem
		errs    []string
		stopped bool
		dry     bool
		stage   string
		msg     string
	}{
		{"stop wins", []models.PatchRunItem{failed}, nil, true, false, "cancelled", "stopped by user"},
		{"failure", []models.PatchRunItem{ok, failed, inUse}, nil, false, false, "failed", "2 of 3 failed: KB5044284 (0x800f0922), Notepad++ (0x8A150101)"},
		{"skip is not a failure in patch_all", []models.PatchRunItem{ok, skipped}, nil, false, false, "completed", "1 skipped: 7zip.7zip (pinned in winget)"},
		{"skip is not a failure in patch_package", []models.PatchRunItem{skipped}, nil, false, false, "completed", "1 skipped: 7zip.7zip (pinned in winget)"},
		{"failure and skip", []models.PatchRunItem{failed, skipped}, nil, false, false, "failed", "1 of 2 failed: KB5044284 (0x800f0922); 1 skipped: 7zip.7zip (pinned in winget)"},
		{"dry run", []models.PatchRunItem{{Status: itemWouldInstall}}, nil, false, true, "dry_run_completed", ""},
		{"already done is success", []models.PatchRunItem{{Status: itemAlreadyInstalled}, {Status: itemPendingReboot}}, nil, false, false, "completed", ""},
		{"empty", nil, nil, false, false, "completed", ""},
		{"run error", nil, []string{"Windows Update search failed: x"}, false, false, "failed", "Windows Update search failed: x"},
	}
	for _, c := range cases {
		stage, msg := finalWindowsStage(c.items, c.errs, c.stopped, c.dry)
		if stage != c.stage || msg != c.msg {
			t.Errorf("%s: got (%q, %q), want (%q, %q)", c.name, stage, msg, c.stage, c.msg)
		}
	}
}

func resetWindowsUpdatesCache(t *testing.T) {
	t.Helper()
	reset := func() {
		lastWindowsUpdatesMu.Lock()
		lastWindowsUpdates, lastWindowsUpdatesSet = nil, false
		lastWindowsUpdatesMu.Unlock()
	}
	reset()
	t.Cleanup(reset)
}

func TestGuardedWindowsUpdatesUsesCacheWhileLocked(t *testing.T) {
	resetWindowsUpdatesCache(t)
	first := []models.Package{{Name: "KB1", NeedsUpdate: true}}
	if got, omitted := guardedWindowsUpdates(func() ([]models.Package, wuSearch) { return first, wuSearch{} }, func(string) {}); omitted || len(got) != 1 {
		t.Fatalf("got %+v, omitted %v", got, omitted)
	}
	wuaMu.Lock()
	called := false
	got, omitted := guardedWindowsUpdates(func() ([]models.Package, wuSearch) { called = true; return nil, wuSearch{} }, func(string) {})
	wuaMu.Unlock()
	if called {
		t.Error("collector must not search while a patch run holds wuaMu")
	}
	if omitted || len(got) != 1 || got[0].Name != "KB1" {
		t.Errorf("locked result = %+v, omitted %v, want the cached list", got, omitted)
	}
}

func TestGuardedWindowsUpdatesWithoutCacheIsOmitted(t *testing.T) {
	resetWindowsUpdatesCache(t)
	wuaMu.Lock()
	got, omitted := guardedWindowsUpdates(func() ([]models.Package, wuSearch) { return nil, wuSearch{} }, func(string) {})
	wuaMu.Unlock()
	if !omitted || got != nil {
		t.Fatalf("got %+v, omitted %v; an empty list without the flag would wipe the server's update rows", got, omitted)
	}
	if _, omitted := guardedWindowsUpdates(func() ([]models.Package, wuSearch) { return []models.Package{}, wuSearch{} }, func(string) {}); omitted {
		t.Fatal("a fresh scan is never omitted")
	}
	wuaMu.Lock()
	got, omitted = guardedWindowsUpdates(func() ([]models.Package, wuSearch) { return nil, wuSearch{} }, func(string) {})
	wuaMu.Unlock()
	if omitted || got == nil || len(got) != 0 {
		t.Errorf("a cached empty list is a real answer: %#v, omitted %v", got, omitted)
	}
}

type fakeWUA struct {
	statuses     map[string][]wuaOpResult
	statusCalls  int
	hang         map[string]bool
	mu           sync.Mutex
	scan         wuaScan
	scanErr      error
	downloads    [][]string
	downloadRes  map[string]wuaOpResult
	downloadErr  error
	installs     []string
	installRes   map[string]wuaOpResult
	installErr   map[string]error
	onInstall    func(ctx context.Context, id string)
	installDelay time.Duration
}

func (f *fakeWUA) Scan(ctx context.Context, log func(string)) (wuaScan, error) {
	log("scanning\n")
	if f.hang["scan"] {
		<-ctx.Done()
		return wuaScan{}, ctx.Err()
	}
	return f.scan, f.scanErr
}

func (f *fakeWUA) Download(ctx context.Context, ids []string, _ func(string)) (map[string]wuaOpResult, error) {
	f.mu.Lock()
	f.downloads = append(f.downloads, ids)
	f.mu.Unlock()
	if f.hang["download"] {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return f.downloadRes, f.downloadErr
}

func (f *fakeWUA) Status(_ context.Context, id string, _ func(string)) (wuaOpResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statusCalls++
	seq := f.statuses[id]
	if len(seq) == 0 {
		return wuaOpResult{ID: id, Found: true}, nil
	}
	res := seq[0]
	if len(seq) > 1 {
		f.statuses[id] = seq[1:]
	}
	return res, nil
}

func (f *fakeWUA) Install(ctx context.Context, id string, _ func(string)) (wuaOpResult, error) {
	f.mu.Lock()
	f.installs = append(f.installs, id)
	f.mu.Unlock()
	if f.onInstall != nil {
		f.onInstall(ctx, id)
	}
	if f.hang["install:"+id] {
		<-ctx.Done()
		return wuaOpResult{}, ctx.Err()
	}
	if f.installDelay > 0 {
		time.Sleep(f.installDelay)
	}
	if err := f.installErr[id]; err != nil {
		return wuaOpResult{}, err
	}
	if res, ok := f.installRes[id]; ok {
		return res, nil
	}
	return wuaOpResult{ID: id, Found: true, ResultCode: 2}, nil
}

type fakeWinget struct {
	store      map[string]bool
	checkErr   error
	states     map[string]wingetUpgradeState
	checked    []string
	renames    [][]string
	resolveErr error
	apps       []models.Package
	calls      [][]string
	codes      map[string]uint32
	output     string
}

func (f *fakeWinget) Resolve() error { return f.resolveErr }

func (f *fakeWinget) Run(_ context.Context, args []string, log func(string)) (uint32, string, error) {
	f.calls = append(f.calls, args)
	log("winget output\n")
	return f.codes[args[2]], f.output, nil
}

func (f *fakeWinget) UpdatableApps() []models.Package { return f.apps }

func (f *fakeWinget) StoreApps(apps []models.Package) map[string]bool {
	out := map[string]bool{}
	for _, p := range apps {
		if f.store[p.WingetID] {
			out[strings.ToLower(p.WingetID)] = true
		}
	}
	return out
}

func (f *fakeWinget) UpgradeState(_ context.Context, id string, _ func(string)) (wingetUpgradeState, error) {
	f.checked = append(f.checked, id)
	if f.checkErr != nil {
		return wingetStateNone, f.checkErr
	}
	return f.states[id], nil
}

func (f *fakeWinget) PendingFileRenames() []string {
	if len(f.renames) == 0 {
		return nil
	}
	r := f.renames[0]
	f.renames = f.renames[1:]
	return r
}

type recorder struct {
	mu      sync.Mutex
	out     strings.Builder
	posts   [][]models.PatchRunItem
	stopped bool
	reboot  bool
}

func (r *recorder) hooks() WindowsPatchHooks {
	return WindowsPatchHooks{
		Output: func(s string) { r.mu.Lock(); r.out.WriteString(s); r.mu.Unlock() },
		PostItems: func(items []models.PatchRunItem) {
			r.mu.Lock()
			r.posts = append(r.posts, items)
			r.mu.Unlock()
		},
		Stopped:        func() bool { r.mu.Lock(); defer r.mu.Unlock(); return r.stopped },
		RebootRequired: func() bool { return r.reboot },
	}
}

func (r *recorder) output() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.out.String()
}

func itemByID(res WindowsPatchResult, id string) models.PatchRunItem {
	for _, it := range res.Items {
		if it.ID == id {
			return it
		}
	}
	return models.PatchRunItem{}
}

func runFake(req WindowsPatchRequest, rec *recorder, w *fakeWUA, g *fakeWinget) WindowsPatchResult {
	return newWindowsPatchRun(req, rec.hooks(), w, g).run(context.Background(), context.Background())
}

func TestRunPatchAllDryRunInstallsNothing(t *testing.T) {
	w := &fakeWUA{scan: sampleScan()}
	g := &fakeWinget{
		apps: []models.Package{
			{Name: "7-Zip", WingetID: "7zip.7zip", NeedsUpdate: true},
			{Name: "Up to date", WingetID: "Foo.Bar", NeedsUpdate: false},
			{Name: "Registry only", NeedsUpdate: true},
		},
		output: "7-Zip 7zip.7zip 23.01 24.08\n",
	}
	rec := &recorder{}
	res := runFake(WindowsPatchRequest{PatchAll: true, DryRun: true, IncludeUpdates: true, IncludeApps: true}, rec, w, g)
	if len(w.downloads) != 0 || len(w.installs) != 0 {
		t.Fatalf("dry run downloaded %v or installed %v", w.downloads, w.installs)
	}
	if len(g.calls) != 0 || !reflect.DeepEqual(g.checked, []string{"7zip.7zip"}) {
		t.Fatalf("dry run must only check apps: calls %q, checked %q", g.calls, g.checked)
	}
	if res.Stage != "dry_run_completed" {
		t.Errorf("stage = %q (%s)", res.Stage, res.ErrorMessage)
	}
	for _, id := range []string{guidA, guidB, "7zip.7zip"} {
		if it := itemByID(res, id); it.Status != itemWouldInstall {
			t.Errorf("%s = %+v", id, it)
		}
	}
	if len(res.Items) != 3 {
		t.Errorf("items = %+v", res.Items)
	}
}

func TestRunPatchAllScopes(t *testing.T) {
	w := &fakeWUA{scan: sampleScan()}
	g := &fakeWinget{apps: []models.Package{{Name: "7-Zip", WingetID: "7zip.7zip", NeedsUpdate: true}}}
	runFake(WindowsPatchRequest{PatchAll: true, IncludeUpdates: false, IncludeApps: true}, &recorder{}, w, g)
	if len(w.installs) != 0 || len(g.calls) != 1 {
		t.Errorf("updates off: wua installs %v, winget calls %v", w.installs, g.calls)
	}
	w = &fakeWUA{scan: sampleScan()}
	g = &fakeWinget{apps: []models.Package{{Name: "7-Zip", WingetID: "7zip.7zip", NeedsUpdate: true}}}
	res := runFake(WindowsPatchRequest{PatchAll: true, IncludeUpdates: true, IncludeApps: false}, &recorder{}, w, g)
	if len(g.calls) != 0 || !reflect.DeepEqual(w.installs, []string{guidB, guidA}) {
		t.Errorf("apps off: wua installs %v, winget calls %v", w.installs, g.calls)
	}
	if res.Stage != "completed" {
		t.Errorf("stage = %q %q", res.Stage, res.ErrorMessage)
	}
	res = runFake(WindowsPatchRequest{PatchAll: true}, &recorder{}, &fakeWUA{}, &fakeWinget{})
	if res.Stage != "failed" {
		t.Errorf("both scopes off = %q", res.Stage)
	}
}

func TestRunPatchPackageResults(t *testing.T) {
	missing := "12345678-0000-0000-0000-000000000000"
	w := &fakeWUA{
		scan: sampleScan(),
		installRes: map[string]wuaOpResult{
			guidA: {Found: true, ResultCode: 3, HResult: "0x00000000", RebootRequired: true},
			guidB: {Found: true, ResultCode: 4, HResult: "0x800F0922"},
		},
	}
	g := &fakeWinget{codes: map[string]uint32{"7zip.7zip": 0, "Notepad++.Notepad++": wingetPackageInUse}}
	rec := &recorder{}
	res := runFake(WindowsPatchRequest{Targets: []WindowsPatchTarget{
		{Kind: WindowsTargetWUA, ID: guidA},
		{Kind: WindowsTargetWUA, ID: guidB},
		{Kind: WindowsTargetWUA, ID: missing},
		{Kind: WindowsTargetWinget, ID: "7zip.7zip"},
		{Kind: WindowsTargetWinget, ID: "Notepad++.Notepad++"},
	}}, rec, w, g)

	if res.Stage != "failed" || !strings.HasPrefix(res.ErrorMessage, "2 of 5 failed: KB2 (0x800f0922)") {
		t.Errorf("stage = %q, message = %q", res.Stage, res.ErrorMessage)
	}
	if it := itemByID(res, guidA); it.Status != itemInstalledWithErrors || !it.RebootRequired || it.ResultCode == nil || *it.ResultCode != 3 {
		t.Errorf("rc 3 item = %+v", it)
	}
	if it := itemByID(res, missing); it.Status != itemNotApplicable {
		t.Errorf("missing item = %+v", it)
	}
	if it := itemByID(res, "Notepad++.Notepad++"); it.Status != itemFailed || it.ExitCode == nil || uint32(*it.ExitCode) != wingetPackageInUse {
		t.Errorf("in-use item = %+v", it)
	}
	if !res.RebootRequired {
		t.Error("per-update restart flag must reach the run")
	}
	if !reflect.DeepEqual(g.calls[0], wingetUpgradeArgs("7zip.7zip")) {
		t.Errorf("winget args = %q", g.calls[0])
	}
	for _, batch := range rec.posts {
		if len(batch) > maxItemsPerPost {
			t.Errorf("posted %d items in one call", len(batch))
		}
	}
	if n := strings.Count(rec.output(), "scanning"); n != 1 {
		t.Errorf("output must be sent as deltas; scan log appeared %d times", n)
	}
}

func TestRunStopBetweenItemsNeverInterruptsInstall(t *testing.T) {
	rec := &recorder{}
	runCtx := context.Background()
	stopCtx, stop := context.WithCancel(runCtx)
	var installCtxErr error
	w := &fakeWUA{scan: wuaScan{Updates: []wuaUpdate{
		{ID: guidA, Title: "A", Type: 1},
		{ID: guidB, Title: "B", Type: 1},
	}}}
	w.onInstall = func(ctx context.Context, _ string) {
		rec.mu.Lock()
		rec.stopped = true
		rec.mu.Unlock()
		stop()
		installCtxErr = ctx.Err()
	}
	res := newWindowsPatchRun(WindowsPatchRequest{PatchAll: true, IncludeUpdates: true, IncludeApps: true}, rec.hooks(), w, &fakeWinget{apps: []models.Package{{Name: "7-Zip", WingetID: "7zip.7zip", NeedsUpdate: true}}}).run(runCtx, stopCtx)
	if installCtxErr != nil {
		t.Fatalf("a stop cancelled the install in progress: %v", installCtxErr)
	}
	if len(w.installs) != 1 {
		t.Fatalf("installs after stop = %v", w.installs)
	}
	if res.Stage != "cancelled" {
		t.Errorf("stage = %q", res.Stage)
	}
	if it := itemByID(res, guidA); it.Status != itemInstalled {
		t.Errorf("current item must finish: %+v", it)
	}
	if it := itemByID(res, guidB); it.Status != itemSkipped || it.Message != msgStopped {
		t.Errorf("remaining item = %+v", it)
	}
	if it := itemByID(res, "7zip.7zip"); it.Status != itemSkipped {
		t.Errorf("remaining app = %+v", it)
	}
}

func TestRunInstallBlockedSkipsRest(t *testing.T) {
	w := &fakeWUA{
		scan:       wuaScan{Updates: []wuaUpdate{{ID: guidA, Title: "A", Type: 1}, {ID: guidB, Title: "B", Type: 1}}},
		installErr: map[string]error{guidA: &wuaScriptError{Message: "install not allowed", HResult: "0x80240016"}},
	}
	res := runFake(WindowsPatchRequest{PatchAll: true, IncludeUpdates: true}, &recorder{}, w, &fakeWinget{})
	if len(w.installs) != 1 {
		t.Errorf("installs = %v", w.installs)
	}
	if it := itemByID(res, guidA); it.Status != itemFailed || it.HResult != "0x80240016" {
		t.Errorf("blocked item = %+v", it)
	}
	if it := itemByID(res, guidB); it.Status != itemSkipped || it.Message != msgRestartFirst {
		t.Errorf("next item = %+v", it)
	}
	if res.Stage != "failed" {
		t.Errorf("stage = %q", res.Stage)
	}
}

func TestRunAlreadyInstalledIsNotReinstalled(t *testing.T) {
	w := &fakeWUA{
		scan: wuaScan{Updates: []wuaUpdate{{ID: guidA, Title: "A", Type: 1}, {ID: guidB, Title: "B", Type: 1}, {ID: guidC, Title: "C", Type: 1}}},
		installRes: map[string]wuaOpResult{
			guidA: {Found: true, AlreadyInstalled: true},
			guidB: {Found: true, PendingReboot: true},
			guidC: {Found: false},
		},
	}
	res := runFake(WindowsPatchRequest{PatchAll: true, IncludeUpdates: true}, &recorder{}, w, &fakeWinget{})
	for id, want := range map[string]string{guidA: itemAlreadyInstalled, guidB: itemPendingReboot, guidC: itemNotApplicable} {
		if it := itemByID(res, id); it.Status != want {
			t.Errorf("%s = %+v, want %s", id, it, want)
		}
	}
	if !itemByID(res, guidB).RebootRequired || !res.RebootRequired {
		t.Error("an install pending a restart must report the restart")
	}
	if res.Stage != "completed" {
		t.Errorf("stage = %q %q", res.Stage, res.ErrorMessage)
	}
}

func TestRunDownloadFailure(t *testing.T) {
	w := &fakeWUA{
		scan:        wuaScan{Updates: []wuaUpdate{{ID: guidA, Title: "A", Type: 1}, {ID: guidB, Title: "B", Type: 1}}},
		downloadRes: map[string]wuaOpResult{guidA: {ID: guidA, ResultCode: 4, HResult: "0x8024402F"}},
	}
	res := runFake(WindowsPatchRequest{PatchAll: true, IncludeUpdates: true}, &recorder{}, w, &fakeWinget{})
	if !reflect.DeepEqual(w.installs, []string{guidB}) {
		t.Errorf("installs = %v", w.installs)
	}
	if it := itemByID(res, guidA); it.Status != itemFailed || !strings.Contains(it.Message, "proxy") {
		t.Errorf("download-failed item = %+v", it)
	}

	w = &fakeWUA{scan: wuaScan{Updates: []wuaUpdate{{ID: guidA, Title: "A", Type: 1}}}, downloadErr: errors.New("boom")}
	res = runFake(WindowsPatchRequest{PatchAll: true, IncludeUpdates: true}, &recorder{}, w, &fakeWinget{})
	if len(w.installs) != 0 || itemByID(res, guidA).Status != itemFailed || res.Stage != "failed" {
		t.Errorf("download error: installs %v, result %+v", w.installs, res)
	}
}

func TestRunScanFailure(t *testing.T) {
	w := &fakeWUA{scanErr: &wuaScriptError{Message: "x", HResult: "0x80070422"}}
	res := runFake(WindowsPatchRequest{PatchAll: true, IncludeUpdates: true}, &recorder{}, w, &fakeWinget{})
	if res.Stage != "failed" || !strings.Contains(res.ErrorMessage, "disabled") {
		t.Errorf("patch_all scan failure = %+v", res)
	}
	res = runFake(WindowsPatchRequest{Targets: []WindowsPatchTarget{{Kind: WindowsTargetWUA, ID: guidA}}}, &recorder{}, w, &fakeWinget{})
	if it := itemByID(res, guidA); it.Status != itemFailed || res.Stage != "failed" {
		t.Errorf("patch_package scan failure = %+v", res)
	}
}

func TestRunWingetMissing(t *testing.T) {
	g := &fakeWinget{resolveErr: errors.New("not found")}
	res := runFake(WindowsPatchRequest{Targets: []WindowsPatchTarget{{Kind: WindowsTargetWinget, ID: "7zip.7zip"}}}, &recorder{}, &fakeWUA{}, g)
	if it := itemByID(res, "7zip.7zip"); it.Status != itemFailed || it.Message != "winget is not installed on this host" || res.Stage != "failed" {
		t.Errorf("patch_package without winget = %+v", res)
	}
	rec := &recorder{}
	res = runFake(WindowsPatchRequest{PatchAll: true, IncludeUpdates: true, IncludeApps: true}, rec, &fakeWUA{}, g)
	if res.Stage != "completed" || !strings.Contains(rec.output(), "winget is not installed") {
		t.Errorf("patch_all without winget = %+v", res)
	}
}

func TestRunHeartbeatAndRebootHook(t *testing.T) {
	w := &fakeWUA{scan: wuaScan{Updates: []wuaUpdate{{ID: guidA, Title: "A", KBs: jsonStrings{"KB9"}, Type: 1}}}, installDelay: 60 * time.Millisecond}
	rec := &recorder{reboot: true}
	r := newWindowsPatchRun(WindowsPatchRequest{PatchAll: true, IncludeUpdates: true}, rec.hooks(), w, &fakeWinget{})
	r.timeouts.heartbeat = 10 * time.Millisecond
	res := r.run(context.Background(), context.Background())
	if !strings.Contains(rec.output(), "[Windows Update] Installing A (KB9): 0m elapsed") {
		t.Errorf("no heartbeat in output:\n%s", rec.output())
	}
	if !res.RebootRequired {
		t.Error("the system restart check must be consulted")
	}
}

func TestRunTimeLimitFailsRun(t *testing.T) {
	runCtx, cancel := context.WithCancel(context.Background())
	cancel()
	w := &fakeWUA{scan: wuaScan{Updates: []wuaUpdate{{ID: guidA, Title: "A", Type: 1}}}}
	res := newWindowsPatchRun(WindowsPatchRequest{PatchAll: true, IncludeUpdates: true}, (&recorder{}).hooks(), w, &fakeWinget{}).run(runCtx, context.Background())
	if len(w.installs) != 0 || res.Stage != "failed" || !strings.Contains(res.ErrorMessage, msgTimeLimit) {
		t.Errorf("time limit = %+v, installs %v", res, w.installs)
	}
}

func TestRunRepostsEveryItemLast(t *testing.T) {
	w := &fakeWUA{scan: wuaScan{Updates: []wuaUpdate{{ID: guidA, Title: "A", Type: 1}, {ID: guidB, Title: "B", Type: 1}}}}
	rec := &recorder{}
	res := runFake(WindowsPatchRequest{PatchAll: true, IncludeUpdates: true}, rec, w, &fakeWinget{})
	last := rec.posts[len(rec.posts)-1]
	if !reflect.DeepEqual(last, res.Items) {
		t.Errorf("last post = %+v, want every final item %+v", last, res.Items)
	}
}

func TestRunDryRunSkipsSystemRestartCheck(t *testing.T) {
	rec := &recorder{reboot: true}
	res := runFake(WindowsPatchRequest{PatchAll: true, DryRun: true, IncludeUpdates: true}, rec, &fakeWUA{scan: sampleScan()}, &fakeWinget{})
	if res.RebootRequired {
		t.Error("a dry run must not report the host's restart state")
	}
}

func TestWindowsRunLimit(t *testing.T) {
	long := 8 * time.Hour
	cases := []struct {
		name string
		req  WindowsPatchRequest
		want time.Duration
	}{
		{"patch all, apps only", WindowsPatchRequest{PatchAll: true, IncludeApps: true}, 4 * time.Hour},
		{"patch all with updates", WindowsPatchRequest{PatchAll: true, IncludeUpdates: true, IncludeApps: true}, long},
		{"named apps", WindowsPatchRequest{Targets: []WindowsPatchTarget{{Kind: WindowsTargetWinget, ID: "a.b"}}}, 4 * time.Hour},
		{"named updates", WindowsPatchRequest{Targets: []WindowsPatchTarget{{Kind: WindowsTargetWinget, ID: "a.b"}, {Kind: WindowsTargetWUA, ID: guidA}}}, long},
	}
	for _, c := range cases {
		if got := WindowsRunLimit(c.req); got != c.want {
			t.Errorf("%s = %v, want %v", c.name, got, c.want)
		}
	}
	d := defaultWindowsPatchTimeouts
	if d.install != 180*time.Minute || d.installFeature < d.install {
		t.Errorf("install budgets = %v / %v", d.install, d.installFeature)
	}
	if long < d.downloadFeature+d.installFeature {
		t.Error("the cap must cover a feature upgrade's download and install")
	}
}

func TestPendingRestartRuleIsShared(t *testing.T) {
	for _, want := range []string{"function Get-InProgressIds", "function Get-SystemRebootRequired", "$e.Operation -ne 1", "$latest[$k] -eq 1", "[Math]::Min($n, 500)"} {
		if !strings.Contains(wuaPendingScript, want) {
			t.Errorf("wua_pending.ps1 lacks %q", want)
		}
	}
	if strings.Contains(wuaScript, "function Get-InProgressIds") {
		t.Error("wua.ps1 must use the shared rule, not its own copy")
	}
	if regexp.MustCompile(`%[-+# 0-9.]*[sdvqxXtTwf]`).MatchString(wuaPendingScript) {
		t.Error("wua_pending.ps1 contains a Go format verb")
	}
}

func TestRunRestartReflectsThisRunOnly(t *testing.T) {
	g := &fakeWinget{apps: []models.Package{{Name: "7-Zip", WingetID: "7zip.7zip", NeedsUpdate: true}}}
	res := runFake(WindowsPatchRequest{PatchAll: true, IncludeApps: true}, &recorder{reboot: true}, &fakeWUA{}, g)
	if res.RebootRequired {
		t.Error("an apps-only run must not report the host's pending restart")
	}
	w := &fakeWUA{scan: wuaScan{Updates: []wuaUpdate{{ID: guidA, Title: "A", Type: 1}}}, installRes: map[string]wuaOpResult{guidA: {Found: true, AlreadyInstalled: true}}}
	if res := runFake(WindowsPatchRequest{PatchAll: true, IncludeUpdates: true}, &recorder{reboot: true}, w, &fakeWinget{}); res.RebootRequired {
		t.Error("a run that installed no update must not report the host's pending restart")
	}
	w = &fakeWUA{scan: wuaScan{Updates: []wuaUpdate{{ID: guidA, Title: "A", Type: 1}}}}
	if res := runFake(WindowsPatchRequest{PatchAll: true, IncludeUpdates: true}, &recorder{reboot: true}, w, &fakeWinget{}); !res.RebootRequired {
		t.Error("an installed update followed by a pending restart must report it")
	}
}

func TestNewPendingRenames(t *testing.T) {
	before := []string{`\??\C:\a.tmp`, "", `\??\C:\b.tmp`, ""}
	cases := []struct {
		after []string
		want  bool
	}{
		{before, false},
		{nil, false},
		{[]string{`\??\C:\A.TMP`, ""}, false},
		{append(append([]string{}, before...), `\??\C:\Program Files\7-Zip\7-zip.dll`, `\??\C:\Program Files\7-Zip\7-zip.dll.new`), true},
		{append(append([]string{}, before...), `\??\C:\a.tmp`), true},
	}
	for i, c := range cases {
		if got := newPendingRenames(before, c.after); got != c.want {
			t.Errorf("case %d: got %v, want %v", i, got, c.want)
		}
	}
}

func TestRunFlagsAppUpgradeThatDefersFileReplacement(t *testing.T) {
	g := &fakeWinget{
		codes:   map[string]uint32{"7zip.7zip": 0, "Notepad++.Notepad++": 0},
		renames: [][]string{{"old"}, {"old", `\??\C:\Program Files\7-Zip\7-zip.dll`}, {"old"}, {"old"}},
	}
	res := runFake(WindowsPatchRequest{Targets: []WindowsPatchTarget{
		{Kind: WindowsTargetWinget, ID: "7zip.7zip"},
		{Kind: WindowsTargetWinget, ID: "Notepad++.Notepad++"},
	}}, &recorder{}, &fakeWUA{}, g)
	if it := itemByID(res, "7zip.7zip"); it.Status != itemInstalled || !it.RebootRequired || it.Message != msgDeferredReplace {
		t.Errorf("deferred upgrade = %+v", it)
	}
	if it := itemByID(res, "Notepad++.Notepad++"); it.RebootRequired {
		t.Errorf("clean upgrade = %+v", it)
	}
	if !res.RebootRequired {
		t.Error("the deferred replacement must reach the run")
	}
}

func TestRunNeverUpgradesPinnedApp(t *testing.T) {
	g := &fakeWinget{
		states: map[string]wingetUpgradeState{"7zip.7zip": wingetStatePinned, "Gone.App": wingetStateNotInstalled, "Same.App": wingetStateNone},
		codes:  map[string]uint32{},
	}
	res := runFake(WindowsPatchRequest{Targets: []WindowsPatchTarget{
		{Kind: WindowsTargetWinget, ID: "7zip.7zip"},
		{Kind: WindowsTargetWinget, ID: "Gone.App"},
		{Kind: WindowsTargetWinget, ID: "Same.App"},
	}}, &recorder{}, &fakeWUA{}, g)
	if len(g.calls) != 0 {
		t.Fatalf("winget upgrade ran: %q", g.calls)
	}
	if it := itemByID(res, "7zip.7zip"); it.Status != itemSkipped || it.Message != msgPinned {
		t.Errorf("pinned = %+v", it)
	}
	if it := itemByID(res, "Gone.App"); it.Status != itemNotApplicable || it.Message != msgUserScope {
		t.Errorf("not installed = %+v", it)
	}
	if it := itemByID(res, "Same.App"); it.Status != itemNotApplicable {
		t.Errorf("no upgrade = %+v", it)
	}
	if res.Stage != "completed" || res.ErrorMessage != "1 skipped: 7zip.7zip (pinned in winget)" {
		t.Errorf("stage = %q, summary = %q", res.Stage, res.ErrorMessage)
	}
}

func TestInstalledUpgradeState(t *testing.T) {
	entries := []wingetEntry{{Name: "7-Zip", ID: "7zip.7zip", Version: "26.02", Available: "26.04"}, {Name: "Same", ID: "Same.App", Version: "2.0"}}
	cases := map[string]wingetUpgradeState{"7zip.7zip": wingetStatePinned, "7ZIP.7ZIP": wingetStatePinned, "Same.App": wingetStateNone, "Missing.App": wingetStateNotInstalled}
	for id, want := range cases {
		if got := installedUpgradeState(entries, id); got != want {
			t.Errorf("%s = %v, want %v", id, got, want)
		}
	}
}

func TestInventoryFlagsPinnedApps(t *testing.T) {
	entries := []wingetEntry{
		{Name: "7-Zip", ID: "7zip.7zip", Version: "26.02", Available: "26.04"},
		{Name: "Notepad++", ID: "Notepad++.Notepad++", Version: "8.6", Available: "8.7"},
		{Name: "Long", ID: "Some.VeryLongPublisher.App", Version: "1.0", Available: "2.0"},
		{Name: "Current", ID: "Current.App", Version: "3.0"},
	}
	upgrades := map[string]string{"Notepad++.Notepad++": "8.7", "Some.VeryLong": "2.0"}
	byID := map[string]models.Package{}
	for _, r := range wingetEntriesToRows(entries, upgrades) {
		byID[r.pkg.WingetID] = r.pkg
	}
	if p := byID["7zip.7zip"]; !p.WingetPinned || !p.NeedsUpdate || p.AvailableVersion != "26.04" {
		t.Errorf("pinned app = %+v", p)
	}
	if p := byID["Notepad++.Notepad++"]; p.WingetPinned || !p.NeedsUpdate {
		t.Errorf("upgradable app = %+v", p)
	}
	if p := byID["Some.VeryLongPublisher.App"]; p.WingetPinned {
		t.Errorf("an Id truncated in --upgrade-available is not a pin: %+v", p)
	}
	if p := byID["Current.App"]; p.WingetPinned || p.NeedsUpdate {
		t.Errorf("current app = %+v", p)
	}
	for _, r := range wingetEntriesToRows(entries, nil) {
		if r.pkg.WingetPinned {
			t.Errorf("without --upgrade-available output nothing can be called pinned: %+v", r.pkg)
		}
	}
}

func TestWingetListCheckFailed(t *testing.T) {
	for code, want := range map[uint32]bool{0: false, wingetNoPackageFound: false, wingetUpdateNotApplicable: false, wingetVersionNotNewer: false, 0x8A15005E: true, 0x80072EE7: true} {
		if got := wingetListCheckFailed(code); got != want {
			t.Errorf("%s = %v, want %v", hexCode(code), got, want)
		}
	}
	if msg := (&wingetCheckError{code: 0x8A15005E}).Error(); !strings.Contains(msg, "certificate") {
		t.Errorf("0x8A15005E message = %q", msg)
	}
}

func TestRunCheckErrorFailsItemWithCode(t *testing.T) {
	g := &fakeWinget{checkErr: &wingetCheckError{code: 0x8A15005E}}
	res := runFake(WindowsPatchRequest{Targets: []WindowsPatchTarget{{Kind: WindowsTargetWinget, ID: "7zip.7zip"}}}, &recorder{}, &fakeWUA{}, g)
	it := itemByID(res, "7zip.7zip")
	if it.Status != itemFailed || it.ExitCode == nil || uint32(*it.ExitCode) != 0x8A15005E || strings.Contains(it.Message, "0x") {
		t.Errorf("item = %+v", it)
	}
	if len(g.calls) != 0 {
		t.Errorf("upgrade ran after a failed check: %q", g.calls)
	}
}

func TestRunSkipsStoreAppsWithoutRunningWinget(t *testing.T) {
	g := &fakeWinget{store: map[string]bool{"Microsoft.WindowsTerminal": true}, codes: map[string]uint32{"7zip.7zip": 0}}
	res := runFake(WindowsPatchRequest{Targets: []WindowsPatchTarget{
		{Kind: WindowsTargetWinget, ID: "Microsoft.WindowsTerminal"},
		{Kind: WindowsTargetWinget, ID: "7zip.7zip"},
	}}, &recorder{}, &fakeWUA{}, g)
	if it := itemByID(res, "Microsoft.WindowsTerminal"); it.Status != itemSkipped || it.Message != msgStoreApp {
		t.Errorf("store app = %+v", it)
	}
	if !reflect.DeepEqual(g.checked, []string{"7zip.7zip"}) || len(g.calls) != 1 {
		t.Errorf("winget touched the store app: checked %q, calls %q", g.checked, g.calls)
	}
	if res.Stage != "completed" || res.ErrorMessage != "1 skipped: Microsoft.WindowsTerminal (Microsoft Store app)" {
		t.Errorf("stage %q summary %q", res.Stage, res.ErrorMessage)
	}
}

func TestSignificantRenames(t *testing.T) {
	temps := []string{`C:\Windows\Temp`, `C:\Windows\SystemTemp`, `C:\ProgramData\PatchMon\winget-tmp`}
	nsis := []string{
		`\??\C:\Windows\SystemTemp\nsr6A69.tmp\System.dll`, "",
		`\??\C:\Windows\SystemTemp\nsr6A69.tmp\`, "",
	}
	sevenZip := []string{`\??\C:\Program Files\7-Zip\7zFM.exe.tmp`, `!\??\C:\Program Files\7-Zip\7zFM.exe`}
	intoTemp := []string{`\??\C:\x.dll`, `!\??\C:\Windows\Temp\y.dll`, `\??\C:\a`, `\??\C:\ProgramData\PatchMon\winget-tmp\b`}
	cases := []struct {
		name   string
		values [][]string
		want   int
	}{
		{"NSIS deletes its temp plugin dir", [][]string{nsis}, 0},
		{"7-Zip replaces an in-use file", [][]string{sevenZip}, 1},
		{"rename into a temp dir", [][]string{intoTemp}, 0},
		{"both values, mixed", [][]string{nsis, sevenZip}, 1},
		{"odd trailing entry is ignored", [][]string{{`\??\C:\lonely`}}, 0},
	}
	for _, c := range cases {
		if got := significantRenames(c.values, temps); len(got) != c.want {
			t.Errorf("%s: got %q, want %d", c.name, got, c.want)
		}
	}
	before := significantRenames([][]string{nsis}, temps)
	after := significantRenames([][]string{nsis, nsis, sevenZip}, temps)
	if !newPendingRenames(before, after) {
		t.Error("a new app-file replacement must be noticed")
	}
	if newPendingRenames(before, significantRenames([][]string{nsis, nsis}, temps)) {
		t.Error("new NSIS temp deletes must not be noticed")
	}
}

func resetRestartPending(t *testing.T) {
	t.Helper()
	reset := func() {
		restartPendingMu.Lock()
		restartPendingIDs = map[string]bool{}
		restartPendingMu.Unlock()
	}
	reset()
	t.Cleanup(reset)
}

func TestReportAfterRunSearchesAfreshAndShowsRestartPending(t *testing.T) {
	resetWindowsUpdatesCache(t)
	resetRestartPending(t)
	stale := []models.Package{
		{Name: "KB5127690", WUAGuid: guidA, NeedsUpdate: true, CurrentVersion: "pending"},
		{Name: "KB2", WUAGuid: guidB, NeedsUpdate: true, CurrentVersion: "pending"},
	}
	if _, omitted := guardedWindowsUpdates(func() ([]models.Package, wuSearch) { return stale, wuSearch{} }, func(string) {}); omitted {
		t.Fatal("setup scan omitted")
	}

	w := &fakeWUA{
		scan:       wuaScan{Updates: []wuaUpdate{{ID: guidA, Title: ".NET", Type: 1}}},
		installRes: map[string]wuaOpResult{guidA: {Found: true, ResultCode: 2, RebootRequired: true}},
	}
	runFake(WindowsPatchRequest{Targets: []WindowsPatchTarget{{Kind: WindowsTargetWUA, ID: guidA}}}, &recorder{}, w, &fakeWinget{})

	searched := false
	wuaStillOffers := func() ([]models.Package, wuSearch) {
		searched = true
		return []models.Package{
			{Name: "KB5127690", WUAGuid: strings.ToUpper(guidA), NeedsUpdate: true, CurrentVersion: "pending"},
			{Name: "KB2", WUAGuid: guidB, NeedsUpdate: true, CurrentVersion: "pending"},
		}, wuSearch{}
	}
	got, omitted := guardedWindowsUpdates(wuaStillOffers, func(string) {})
	if !searched || omitted {
		t.Fatalf("the report after a run must search Windows Update again (searched %v, omitted %v)", searched, omitted)
	}
	if got[0].NeedsUpdate || got[0].CurrentVersion != wuaPendingRestartVersion {
		t.Errorf("installed update = %+v, want pending restart", got[0])
	}
	if !got[1].NeedsUpdate {
		t.Errorf("an untouched update must stay pending: %+v", got[1])
	}

	wuaMu.Lock()
	cached, omitted := guardedWindowsUpdates(func() ([]models.Package, wuSearch) { return nil, wuSearch{} }, func(string) {})
	wuaMu.Unlock()
	if omitted || cached[0].NeedsUpdate {
		t.Errorf("a report during a later run must also show the restart: %+v", cached)
	}
}

func TestRunInvalidatesTheUpdateCacheAfterInstalling(t *testing.T) {
	resetWindowsUpdatesCache(t)
	resetRestartPending(t)
	guardedWindowsUpdates(func() ([]models.Package, wuSearch) { return []models.Package{{Name: "KB1"}}, wuSearch{} }, func(string) {})
	w := &fakeWUA{scan: wuaScan{Updates: []wuaUpdate{{ID: guidA, Title: "A", Type: 1}}}, installRes: map[string]wuaOpResult{guidA: {Found: true, ResultCode: 4}}}
	runFake(WindowsPatchRequest{PatchAll: true, IncludeUpdates: true}, &recorder{}, w, &fakeWinget{})
	wuaMu.Lock()
	_, omitted := guardedWindowsUpdates(func() ([]models.Package, wuSearch) { return nil, wuSearch{} }, func(string) {})
	wuaMu.Unlock()
	if !omitted {
		t.Error("the pre-run update list must not be reused after an install attempt")
	}
	restartPendingMu.Lock()
	n := len(restartPendingIDs)
	restartPendingMu.Unlock()
	if n != 0 {
		t.Error("a failed install must not be recorded as waiting for a restart")
	}
}

func runWithTimeouts(req WindowsPatchRequest, w *fakeWUA, set func(*windowsPatchTimeouts)) (WindowsPatchResult, *recorder) {
	rec := &recorder{}
	r := newWindowsPatchRun(req, rec.hooks(), w, &fakeWinget{})
	set(&r.timeouts)
	return r.run(context.Background(), context.Background()), rec
}

func TestWUATimeoutsHaveReadableMessages(t *testing.T) {
	two := wuaScan{Updates: []wuaUpdate{{ID: guidA, Title: "A", Type: 1}, {ID: guidB, Title: "B", Type: 1}}}
	short := 20 * time.Millisecond

	res, _ := runWithTimeouts(WindowsPatchRequest{Targets: []WindowsPatchTarget{{Kind: WindowsTargetWUA, ID: guidA}}}, &fakeWUA{hang: map[string]bool{"scan": true}}, func(to *windowsPatchTimeouts) { to.scan = short })
	if it := itemByID(res, guidA); it.Status != itemFailed || it.Message != "Windows Update search did not finish within 0 minutes" {
		t.Errorf("scan timeout = %+v", it)
	}

	res, _ = runWithTimeouts(WindowsPatchRequest{PatchAll: true, IncludeUpdates: true}, &fakeWUA{scan: two, hang: map[string]bool{"download": true}}, func(to *windowsPatchTimeouts) { to.download = short })
	if it := itemByID(res, guidA); it.Status != itemFailed || !strings.HasPrefix(it.Message, "Download did not finish within") {
		t.Errorf("download timeout = %+v", it)
	}

	res, _ = runWithTimeouts(WindowsPatchRequest{PatchAll: true, IncludeUpdates: true}, &fakeWUA{scan: two, hang: map[string]bool{"install:" + guidA: true}}, func(to *windowsPatchTimeouts) {
		to.install, to.recheck, to.recheckEvery = short, short, time.Millisecond
	})
	if it := itemByID(res, guidA); it.Status != itemFailed || it.RebootRequired || !strings.HasPrefix(it.Message, "Install did not finish within") || !strings.Contains(it.Message, "check after a restart") {
		t.Errorf("install timeout = %+v", it)
	}

	w := &fakeWUA{scan: two, installErr: map[string]error{guidA: errors.New("exec: \"powershell.exe\": file does not exist")}}
	res, rec := runWithTimeouts(WindowsPatchRequest{PatchAll: true, IncludeUpdates: true}, w, func(*windowsPatchTimeouts) {})
	it := itemByID(res, guidA)
	if strings.Contains(it.Message, "exec:") || it.Message != "Install failed: Windows Update could not be run; see the run output" {
		t.Errorf("raw error reached the item: %+v", it)
	}
	if !strings.Contains(rec.output(), "file does not exist") {
		t.Error("the raw error must still reach the run output")
	}
}

func TestInstallTimeoutStopsTheRestOfWindowsUpdate(t *testing.T) {
	scan := wuaScan{Updates: []wuaUpdate{{ID: guidA, Title: "A", KBs: jsonStrings{"KB1"}, Type: 1}, {ID: guidB, Title: "B", KBs: jsonStrings{"KB2"}, Type: 1}}}
	w := &fakeWUA{scan: scan, hang: map[string]bool{"install:" + guidA: true}}
	rec := &recorder{}
	g := &fakeWinget{apps: []models.Package{{Name: "7-Zip", WingetID: "7zip.7zip", NeedsUpdate: true}}}
	r := newWindowsPatchRun(WindowsPatchRequest{PatchAll: true, IncludeUpdates: true, IncludeApps: true}, rec.hooks(), w, g)
	r.timeouts.install, r.timeouts.recheck, r.timeouts.recheckEvery = 20*time.Millisecond, 20*time.Millisecond, time.Millisecond
	res := r.run(context.Background(), context.Background())
	if !reflect.DeepEqual(w.installs, []string{guidA}) {
		t.Fatalf("installs = %v; nothing may start while Windows is still servicing", w.installs)
	}
	if it := itemByID(res, guidB); it.Status != itemSkipped || it.Message != msgStillInstalling {
		t.Errorf("next update = %+v", it)
	}
	if it := itemByID(res, "7zip.7zip"); it.Status != itemInstalled {
		t.Errorf("the winget phase must still run: %+v", it)
	}
	if res.Stage != "failed" || !strings.Contains(res.ErrorMessage, "1 skipped: KB2 (not started, Windows Update was still installing)") {
		t.Errorf("stage %q summary %q", res.Stage, res.ErrorMessage)
	}

	w = &fakeWUA{scan: scan, installRes: map[string]wuaOpResult{guidA: {Found: true, ResultCode: 4}}}
	runFake(WindowsPatchRequest{PatchAll: true, IncludeUpdates: true}, &recorder{}, w, &fakeWinget{})
	if len(w.installs) != 2 {
		t.Errorf("an ordinary failure must not stop the next update: %v", w.installs)
	}
}

func TestInstallTimeoutRechecksWindows(t *testing.T) {
	scan := wuaScan{Updates: []wuaUpdate{{ID: guidA, Title: "A cumulative update", KBs: jsonStrings{"KB5122871"}, Type: 1}, {ID: guidB, Title: "B .NET", KBs: jsonStrings{"KB5127690"}, Type: 1}}}
	set := func(to *windowsPatchTimeouts) {
		to.install, to.recheck, to.recheckEvery = 20*time.Millisecond, time.Second, 5*time.Millisecond
	}
	cases := []struct {
		name      string
		statuses  []wuaOpResult
		status    string
		reboot    bool
		nextRan   bool
		msgPrefix string
	}{
		{"finished after the limit", []wuaOpResult{{Found: true}, {Found: true, AlreadyInstalled: true}}, itemInstalled, false, true, "Installed; Windows finished after"},
		{"installed, restart pending", []wuaOpResult{{Found: true, PendingReboot: true}}, itemPendingReboot, true, true, "Installed; Windows finished after"},
		{"still servicing", nil, itemFailed, false, false, "Install did not finish within"},
	}
	for _, c := range cases {
		w := &fakeWUA{scan: scan, hang: map[string]bool{"install:" + guidA: true}, statuses: map[string][]wuaOpResult{guidA: c.statuses}}
		res, _ := runWithTimeouts(WindowsPatchRequest{PatchAll: true, IncludeUpdates: true}, w, set)
		it := itemByID(res, guidA)
		if it.Status != c.status || it.RebootRequired != c.reboot || !strings.HasPrefix(it.Message, c.msgPrefix) {
			t.Errorf("%s: item = %+v", c.name, it)
		}
		ranNext := len(w.installs) == 2
		if ranNext != c.nextRan {
			t.Errorf("%s: installs = %v", c.name, w.installs)
		}
		if !c.nextRan {
			if next := itemByID(res, guidB); next.Status != itemSkipped || next.Message != msgStillInstalling {
				t.Errorf("%s: next = %+v", c.name, next)
			}
			if w.statusCalls < 2 {
				t.Errorf("%s: polled %d times", c.name, w.statusCalls)
			}
		}
	}
}

func TestFailedWindowsUpdateQueryIsOmittedNotEmpty(t *testing.T) {
	resetWindowsUpdatesCache(t)
	if _, omitted := guardedWindowsUpdates(func() ([]models.Package, wuSearch) {
		return []models.Package{{Name: "KB1", WUAGuid: guidA, NeedsUpdate: true}}, wuSearch{}
	}, func(string) {}); omitted {
		t.Fatal("setup")
	}
	got, omitted := guardedWindowsUpdates(func() ([]models.Package, wuSearch) { return nil, wuSearch{status: wuSearchFailed} }, func(string) {})
	if !omitted || len(got) != 0 {
		t.Fatalf("a failed query must send no Windows Update rows and mark them omitted: %+v, %v", got, omitted)
	}
	got, omitted = guardedWindowsUpdates(func() ([]models.Package, wuSearch) { return []models.Package{}, wuSearch{} }, func(string) {})
	if omitted || got == nil || len(got) != 0 {
		t.Fatalf("a successful empty search is a real empty list: %#v, %v", got, omitted)
	}
	wuaMu.Lock()
	cached, omitted := guardedWindowsUpdates(func() ([]models.Package, wuSearch) { return nil, wuSearch{status: wuSearchFailed} }, func(string) {})
	wuaMu.Unlock()
	if omitted || len(cached) != 0 {
		t.Errorf("the cache must hold the last successful search, not the failed one: %+v, %v", cached, omitted)
	}
}

func TestPersistentCOMFailureReportsInstalledUpdates(t *testing.T) {
	resetWindowsUpdatesCache(t)
	comFailureMu.Lock()
	comFailureStreak = 0
	comFailureMu.Unlock()
	t.Cleanup(func() { comFailureMu.Lock(); comFailureStreak = 0; comFailureMu.Unlock() })

	hotfixes := []models.Package{{Name: "KB5000001", CurrentVersion: "installed", Category: "Windows Update"}}
	comFail := func() ([]models.Package, wuSearch) {
		return append([]models.Package(nil), hotfixes...), wuSearch{status: wuSearchCOMFailed, detail: "0x80070005"}
	}
	var warnings []string
	warn := func(s string) { warnings = append(warnings, s) }

	for i := 1; i <= 2; i++ {
		if got, omitted := guardedWindowsUpdates(comFail, warn); !omitted || len(got) != 0 {
			t.Fatalf("failure %d: got %+v omitted %v, want the server's rows kept", i, got, omitted)
		}
	}
	for i := 3; i <= 4; i++ {
		got, omitted := guardedWindowsUpdates(comFail, warn)
		if omitted || len(got) != 1 || got[0].Name != "KB5000001" {
			t.Fatalf("failure %d: got %+v omitted %v, want installed updates", i, got, omitted)
		}
	}
	streak := 0
	for _, w := range warnings {
		if strings.Contains(w, "keeps failing") && strings.Contains(w, "0x80070005") {
			streak++
		}
	}
	if streak != 1 {
		t.Errorf("the persistent-failure warning must appear once per streak: %q", warnings)
	}

	if _, omitted := guardedWindowsUpdates(func() ([]models.Package, wuSearch) { return []models.Package{}, wuSearch{} }, warn); omitted {
		t.Fatal("a successful search")
	}
	if _, omitted := guardedWindowsUpdates(comFail, warn); !omitted {
		t.Error("a success must reset the streak")
	}
	if _, omitted := guardedWindowsUpdates(func() ([]models.Package, wuSearch) { return nil, wuSearch{status: wuSearchFailed} }, warn); !omitted {
		t.Error("an exit or parse failure is always omitted")
	}
}

func TestStopDuringRecheckSkipsTheRestAsStopped(t *testing.T) {
	scan := wuaScan{Updates: []wuaUpdate{{ID: guidA, Title: "A", Type: 1}, {ID: guidB, Title: "B", Type: 1}}}
	rec := &recorder{}
	stopCtx, stop := context.WithCancel(context.Background())
	w := &fakeWUA{scan: scan, hang: map[string]bool{"install:" + guidA: true}}
	r := newWindowsPatchRun(WindowsPatchRequest{PatchAll: true, IncludeUpdates: true}, rec.hooks(), w, &fakeWinget{})
	r.timeouts.install, r.timeouts.recheck, r.timeouts.recheckEvery = 20*time.Millisecond, time.Minute, 10*time.Millisecond
	go func() {
		for {
			w.mu.Lock()
			n := w.statusCalls
			w.mu.Unlock()
			if n > 0 {
				rec.mu.Lock()
				rec.stopped = true
				rec.mu.Unlock()
				stop()
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()
	res := r.run(context.Background(), stopCtx)
	if it := itemByID(res, guidB); it.Status != itemSkipped || it.Message != msgStopped {
		t.Errorf("remaining update = %+v, want Stopped", it)
	}
	if res.Stage != "cancelled" {
		t.Errorf("stage = %q", res.Stage)
	}
}
