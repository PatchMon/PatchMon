package packages

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"patchmon-agent/internal/winexec"
	"patchmon-agent/pkg/models"
)

// Windows patch target kinds, as carried in run_patch targets and patch run items.
const (
	WindowsTargetWUA    = "wua"
	WindowsTargetWinget = "winget"
)

const (
	itemPending             = "pending"
	itemWouldInstall        = "would_install"
	itemNotApplicable       = "not_applicable"
	itemInstalled           = "installed"
	itemAlreadyInstalled    = "already_installed"
	itemPendingReboot       = "installed_pending_reboot"
	itemInstalledWithErrors = "installed_with_errors"
	itemFailed              = "failed"
	itemAborted             = "aborted"
	itemSkipped             = "skipped"
)

const (
	wuaCategorySecurity = "0fa1201d-4330-4fa8-8ae9-b877473b6441"
	wuaCategoryCritical = "e6cf1350-c01b-414d-a61f-263d14d133b4"
	wuaCategoryUpgrades = "3689bdc8-b205-4af4-8d4a-a63924c5e9d5"
	wuaTypeDriver       = 2
)

const maxItemsPerPost = 200

//go:embed wua.ps1
var wuaScript string

// wuaPendingScript is the one "installed, waiting for a restart" rule, shared by the patch
// runner and the inventory so the two never disagree.
//
//go:embed wua_pending.ps1
var wuaPendingScript string

func wuaPowerShellArgs() []string {
	return []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", winexec.Script(wuaPendingScript + wuaScript)}
}

var wuaGUIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// WindowsPatchTarget is one validated update or app the server asked the agent to patch.
type WindowsPatchTarget struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// ValidWindowsPatchTarget reports whether a target is safe to act on.
func ValidWindowsPatchTarget(t WindowsPatchTarget) bool {
	switch t.Kind {
	case WindowsTargetWUA:
		return wuaGUIDPattern.MatchString(t.ID)
	case WindowsTargetWinget:
		return wingetIDPattern.MatchString(t.ID)
	}
	return false
}

// WindowsPatchRequest describes one Windows patch run.
type WindowsPatchRequest struct {
	PatchAll       bool
	DryRun         bool
	IncludeUpdates bool
	IncludeApps    bool
	Targets        []WindowsPatchTarget
}

// WindowsPatchHooks connects the runner to the agent's output stream, item endpoint and stop flag.
type WindowsPatchHooks struct {
	Output         func(string)
	PostItems      func([]models.PatchRunItem)
	Stopped        func() bool
	RebootRequired func() bool
}

// WindowsPatchResult is the outcome of a Windows patch run.
type WindowsPatchResult struct {
	Stage          string
	ErrorMessage   string
	RebootRequired bool
	Items          []models.PatchRunItem
}

type wuaUpdate struct {
	ID                  string      `json:"id"`
	Revision            int32       `json:"revision"`
	Title               string      `json:"title"`
	KBs                 jsonStrings `json:"kbs"`
	Categories          jsonStrings `json:"categories"`
	CategoryIDs         jsonStrings `json:"category_ids"`
	Type                int         `json:"type_code"`
	BrowseOnly          bool        `json:"browse_only"`
	IsDownloaded        bool        `json:"is_downloaded"`
	RebootBehavior      int         `json:"reboot_behavior"`
	CanRequestUserInput bool        `json:"can_request_user_input"`
	PendingReboot       bool        `json:"pending_reboot"`
}

type jsonStrings []string

func (s *jsonStrings) UnmarshalJSON(b []byte) error {
	var one string
	if err := json.Unmarshal(b, &one); err == nil {
		if one != "" {
			*s = jsonStrings{one}
		}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return err
	}
	*s = many
	return nil
}

type wuaScan struct {
	Updates                     []wuaUpdate
	RebootRequiredBeforeInstall bool
}

type wuaOpResult struct {
	ID               string `json:"id"`
	Found            bool   `json:"found"`
	AlreadyInstalled bool   `json:"already_installed"`
	PendingReboot    bool   `json:"pending_reboot"`
	ResultCode       int    `json:"result_code"`
	HResult          string `json:"hresult"`
	RebootRequired   bool   `json:"reboot_required"`
}

type wuaFinal struct {
	wuaOpResult
	Op                          string `json:"op"`
	RebootRequiredBeforeInstall bool   `json:"reboot_required_before_install"`
}

type wuaOutput struct {
	Updates []wuaUpdate
	Results []wuaOpResult
	Final   *wuaFinal
}

type wuaScriptError struct {
	Message string
	HResult string
}

func (e *wuaScriptError) itemMessage() string {
	if hint := wuaHResultHint(e.HResult); hint != "" {
		return e.Message + ": " + hint
	}
	return e.Message
}

func (e *wuaScriptError) Error() string {
	msg := e.Message
	if e.HResult != "" {
		msg += " (" + e.HResult + ")"
	}
	if hint := wuaHResultHint(e.HResult); hint != "" {
		msg += ": " + hint
	}
	return msg
}

var errWUANoResult = errors.New("the Windows Update script ended without a result")

func parseWUAOutput(r io.Reader, log func(string)) (wuaOutput, error) {
	var out wuaOutput
	var scriptErr error
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	first := true
	for sc.Scan() {
		line := sc.Bytes()
		if first {
			line = winexec.TrimBOM(line)
			first = false
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var head struct {
			Type    string `json:"type"`
			Message string `json:"message"`
			HResult string `json:"hresult"`
		}
		if line[0] != '{' || json.Unmarshal(line, &head) != nil {
			log(stripTerminalControls(string(line)) + "\n")
			continue
		}
		switch head.Type {
		case "log":
			log(stripTerminalControls(head.Message) + "\n")
		case "update":
			var u wuaUpdate
			if json.Unmarshal(line, &u) == nil {
				out.Updates = append(out.Updates, u)
			}
		case "update_result":
			var res wuaOpResult
			if json.Unmarshal(line, &res) == nil {
				out.Results = append(out.Results, res)
			}
		case "result":
			var f wuaFinal
			if json.Unmarshal(line, &f) == nil {
				out.Final = &f
			}
		case "error":
			scriptErr = &wuaScriptError{Message: stripTerminalControls(head.Message), HResult: head.HResult}
		default:
			log(stripTerminalControls(string(line)) + "\n")
		}
	}
	if err := sc.Err(); err != nil {
		return out, err
	}
	if scriptErr != nil {
		return out, scriptErr
	}
	if out.Final == nil {
		return out, errWUANoResult
	}
	return out, nil
}

func isFeatureUpgrade(u wuaUpdate) bool {
	for _, id := range u.CategoryIDs {
		if strings.EqualFold(id, wuaCategoryUpgrades) {
			return true
		}
	}
	for _, c := range u.Categories {
		if strings.EqualFold(c, "Upgrades") {
			return true
		}
	}
	return false
}

func isSecurityOrCritical(u wuaUpdate) bool {
	for _, id := range u.CategoryIDs {
		if strings.EqualFold(id, wuaCategorySecurity) || strings.EqualFold(id, wuaCategoryCritical) {
			return true
		}
	}
	return false
}

func patchAllEligible(u wuaUpdate) bool {
	return u.Type != wuaTypeDriver && !u.BrowseOnly && !isFeatureUpgrade(u)
}

func wuaKB(u wuaUpdate) string {
	return strings.Join(u.KBs, ", ")
}

func wuaItem(u wuaUpdate, status, message string) models.PatchRunItem {
	return models.PatchRunItem{
		Kind:     WindowsTargetWUA,
		ID:       strings.ToLower(u.ID),
		Revision: u.Revision,
		Title:    u.Title,
		KB:       wuaKB(u),
		Status:   status,
		Message:  message,
	}
}

const (
	msgNoLongerOffered = "No longer offered by Windows Update: installed, superseded or withdrawn"
	msgPendingReboot   = "Installed earlier; restart the host to finish"
	msgInteractive     = "Needs interactive input"
	msgRestartFirst    = "Restart pending from an earlier install; restart the host and run again"
	msgStopped         = "Stopped"
	msgTimeLimit       = "The run time limit was reached"
)

func selectWUAUpdates(scan wuaScan, patchAll bool, targetIDs []string) (install []wuaUpdate, decided []models.PatchRunItem) {
	var chosen []wuaUpdate
	if patchAll {
		for _, u := range scan.Updates {
			if patchAllEligible(u) && wuaGUIDPattern.MatchString(u.ID) {
				chosen = append(chosen, u)
			}
		}
	} else {
		byID := make(map[string]wuaUpdate, len(scan.Updates))
		for _, u := range scan.Updates {
			byID[strings.ToLower(u.ID)] = u
		}
		for _, id := range targetIDs {
			u, ok := byID[strings.ToLower(id)]
			if !ok {
				decided = append(decided, models.PatchRunItem{Kind: WindowsTargetWUA, ID: strings.ToLower(id), Status: itemNotApplicable, Message: msgNoLongerOffered})
				continue
			}
			chosen = append(chosen, u)
		}
	}
	for _, u := range chosen {
		switch {
		case u.PendingReboot:
			it := wuaItem(u, itemPendingReboot, msgPendingReboot)
			it.RebootRequired = true
			decided = append(decided, it)
		case u.CanRequestUserInput:
			decided = append(decided, wuaItem(u, itemSkipped, msgInteractive))
		case scan.RebootRequiredBeforeInstall:
			decided = append(decided, wuaItem(u, itemSkipped, msgRestartFirst))
		default:
			install = append(install, u)
		}
	}
	return orderForInstall(install), decided
}

func orderForInstall(updates []wuaUpdate) []wuaUpdate {
	out := append([]wuaUpdate(nil), updates...)
	sort.SliceStable(out, func(i, j int) bool {
		si, sj := isSecurityOrCritical(out[i]), isSecurityOrCritical(out[j])
		if si != sj {
			return si
		}
		return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title)
	})
	return out
}

func rebootBehaviourText(b int) string {
	switch b {
	case 1:
		return "Restart required"
	case 2:
		return "May need a restart"
	default:
		return "No restart expected"
	}
}

func wuaResultStatus(rc int) string {
	switch rc {
	case 2:
		return itemInstalled
	case 3:
		return itemInstalledWithErrors
	case 4:
		return itemFailed
	case 5:
		return itemAborted
	default:
		return itemFailed
	}
}

func wuaResultMessage(rc int, hresult string) string {
	var msg string
	switch rc {
	case 2:
		msg = "Installed"
	case 3:
		msg = "Installed with errors"
	case 4:
		msg = "Failed"
	case 5:
		msg = "Aborted"
	default:
		msg = "Did not run"
	}
	if hint := wuaHResultHint(hresult); hint != "" {
		msg += ": " + hint
	}
	return msg
}

func isZeroHResult(h string) bool {
	return strings.Trim(strings.TrimPrefix(strings.ToLower(h), "0x"), "0") == ""
}

var wuaHResultHints = map[string]string{
	"0x80240016": "Another installation is running, or a restart is pending",
	"0x8024000b": "Operation was cancelled",
	"0x80240017": "Update not applicable to this host",
	"0x8024001e": "Update service is stopping (shutdown in progress)",
	"0x80070005": "Access denied: agent not running as LocalSystem",
	"0x80070422": "Windows Update service is disabled",
	"0x8024402c": "Cannot reach the update server or WSUS: check proxy, firewall, WSUS URL",
	"0x8024402f": "Cannot reach the update server or WSUS: check proxy, firewall, WSUS URL",
	"0x80244007": "Cannot reach the update server or WSUS: check proxy, firewall, WSUS URL",
	"0x80240438": "Cannot reach the update server or WSUS: check proxy, firewall, WSUS URL",
	"0x80246007": "Update not downloaded",
	"0x8024200d": "Update needs another download; run again",
	"0x80070bc9": "A restart is required before this can install",
	"0x800f0922": "Servicing failed (often the system reserved partition or .NET); see CBS.log",
	"0x80073701": "Component store corrupt; run DISM /RestoreHealth",
}

func normaliseHResult(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	if h == "" {
		return ""
	}
	if !strings.HasPrefix(h, "0x") {
		h = "0x" + h
	}
	return h
}

func wuaHResultHint(h string) string {
	return wuaHResultHints[normaliseHResult(h)]
}

func isInstallBlockedHResult(h string) bool {
	return normaliseHResult(h) == "0x80240016"
}

const (
	wingetNoPackageFound       uint32 = 0x8A150014
	wingetUpdateNotApplicable  uint32 = 0x8A15002B
	wingetVersionNotNewer      uint32 = 0x8A15004F
	wingetPinned               uint32 = 0x8A150068
	wingetUserScopeFromAdmin   uint32 = 0x8A15007D
	wingetPackageInUse         uint32 = 0x8A150101
	wingetInstallInProgress    uint32 = 0x8A150102
	wingetFileInUse            uint32 = 0x8A150103
	wingetRebootToFinish       uint32 = 0x8A150109
	wingetRebootForInstall     uint32 = 0x8A15010A
	wingetRebootInitiated      uint32 = 0x8A15010B
	wingetInUseByOtherApp      uint32 = 0x8A150111
	wingetSourceAgreementsNeed uint32 = 0x8A150046
)

type wingetOutcome struct {
	Status         string
	RebootRequired bool
	Message        string
}

const msgUserScope = "Not found for the system account: the app is probably installed for a single user, which the agent cannot upgrade"

func classifyWingetUpgradeExit(code uint32, dryRun bool, output, id string) wingetOutcome {
	if dryRun {
		switch code {
		case 0:
			if wingetOutputListsID(output, id) {
				return wingetOutcome{Status: itemWouldInstall, Message: "Upgrade available"}
			}
			return wingetOutcome{Status: itemNotApplicable, Message: "No upgrade available"}
		case wingetUpdateNotApplicable, wingetVersionNotNewer:
			return wingetOutcome{Status: itemNotApplicable, Message: "No upgrade available"}
		case wingetNoPackageFound:
			return wingetOutcome{Status: itemNotApplicable, Message: msgUserScope}
		case wingetPinned:
			return wingetOutcome{Status: itemSkipped, Message: "Pinned with winget pin"}
		}
		return wingetOutcome{Status: itemFailed, Message: wingetCodeMessage(code)}
	}
	switch code {
	case 0:
		return wingetOutcome{Status: itemInstalled, Message: "Upgraded"}
	case wingetRebootToFinish, wingetRebootInitiated:
		return wingetOutcome{Status: itemInstalled, RebootRequired: true, Message: "Upgraded; restart to finish"}
	case wingetUpdateNotApplicable, wingetVersionNotNewer:
		return wingetOutcome{Status: itemNotApplicable, Message: "No applicable upgrade"}
	case wingetNoPackageFound, wingetUserScopeFromAdmin:
		return wingetOutcome{Status: itemNotApplicable, Message: msgUserScope}
	case wingetPinned:
		return wingetOutcome{Status: itemSkipped, Message: "Pinned with winget pin"}
	case wingetPackageInUse, wingetFileInUse, wingetInUseByOtherApp:
		return wingetOutcome{Status: itemFailed, Message: "Application is in use: close the app and retry"}
	case wingetInstallInProgress:
		return wingetOutcome{Status: itemFailed, Message: "Another installation is in progress"}
	case wingetRebootForInstall:
		return wingetOutcome{Status: itemFailed, Message: "Restart the host, then try again"}
	}
	return wingetOutcome{Status: itemFailed, Message: wingetCodeMessage(code)}
}

func hexCode(code uint32) string {
	return fmt.Sprintf("0x%08X", code)
}

var wingetCodeMessages = map[uint32]string{
	wingetSourceAgreementsNeed: "winget source agreements were not accepted",
	0x8A150008:                 "Downloading the installer failed",
	0x8A150011:                 "The installer's hash does not match its manifest",
	0x8A150105:                 "Not enough disk space",
	0x8A150107:                 "The installer needs internet access",
	0x8A15010F:                 "Blocked by organisation policy",
	0x80072EE2:                 "The download timed out",
	0x80072EE7:                 "Could not resolve the download server's name: check DNS and proxy settings",
	0x80072EFD:                 "Could not connect to the download server",
	0x80072F8F:                 "Could not make a secure connection: check the system clock and any TLS inspection",
}

func wingetCodeMessage(code uint32) string {
	if msg, ok := wingetCodeMessages[code]; ok {
		return msg
	}
	return "winget exited with an error"
}

func wingetOutputListsID(output, id string) bool {
	for _, line := range strings.Split(output, "\n") {
		for _, f := range strings.Fields(line) {
			if strings.EqualFold(f, id) {
				return true
			}
		}
	}
	return false
}

func wingetUpgradeArgs(id string) []string {
	return []string{"upgrade", "--id", id, "--exact", "--silent", "--accept-source-agreements", "--accept-package-agreements", "--disable-interactivity"}
}

func wingetListArgs(id string) []string {
	return []string{"list", "--id", id, "--exact", "--upgrade-available", "--accept-source-agreements", "--disable-interactivity"}
}

func finalWindowsStage(items []models.PatchRunItem, runErrors []string, stopped, dryRun, patchPackage bool) (stage, errMsg string) {
	if stopped {
		return "cancelled", "stopped by user"
	}
	var bad []models.PatchRunItem
	for _, it := range items {
		switch it.Status {
		case itemFailed, itemAborted:
			bad = append(bad, it)
		case itemSkipped:
			if patchPackage {
				bad = append(bad, it)
			}
		}
	}
	if len(bad) > 0 || len(runErrors) > 0 {
		var parts []string
		if len(bad) > 0 {
			var names []string
			for i, it := range bad {
				if i == 3 {
					break
				}
				names = append(names, itemLabel(it))
			}
			parts = append(parts, fmt.Sprintf("%d of %d failed: %s", len(bad), len(items), strings.Join(names, ", ")))
		}
		parts = append(parts, runErrors...)
		return "failed", strings.Join(parts, "; ")
	}
	if dryRun {
		return "dry_run_completed", ""
	}
	return "completed", ""
}

func itemLabel(it models.PatchRunItem) string {
	name := it.KB
	if name == "" {
		name = it.Title
	}
	if name == "" {
		name = it.ID
	}
	switch {
	case it.HResult != "" && !isZeroHResult(it.HResult):
		name += " (" + it.HResult + ")"
	case it.ExitCode != nil && *it.ExitCode != 0:
		name += " (" + hexCode(uint32(*it.ExitCode)) + ")"
	case it.Status == itemSkipped && it.Message != "":
		name += " (" + it.Message + ")"
	}
	return name
}

type wuaBackend interface {
	Scan(ctx context.Context, log func(string)) (wuaScan, error)
	Download(ctx context.Context, ids []string, log func(string)) (map[string]wuaOpResult, error)
	Install(ctx context.Context, id string, log func(string)) (wuaOpResult, error)
}

type wingetBackend interface {
	Resolve() error
	Run(ctx context.Context, args []string, log func(string)) (code uint32, output string, err error)
	UpdatableApps() []models.Package
}

type windowsPatchTimeouts struct {
	scan, download, downloadFeature, install, installFeature, wingetUpgrade, wingetList, heartbeat time.Duration
}

var defaultWindowsPatchTimeouts = windowsPatchTimeouts{
	scan:            15 * time.Minute,
	download:        60 * time.Minute,
	downloadFeature: 180 * time.Minute,
	install:         90 * time.Minute,
	installFeature:  240 * time.Minute,
	wingetUpgrade:   30 * time.Minute,
	wingetList:      5 * time.Minute,
	heartbeat:       60 * time.Second,
}

var wuaMu sync.Mutex

var (
	lastWindowsUpdatesMu  sync.Mutex
	lastWindowsUpdates    []models.Package
	lastWindowsUpdatesSet bool
)

// While a patch run holds wuaMu, a report gets the last pending list instead of searching.
// With no list cached yet, it reports the updates as omitted so the server keeps its rows.
func (m *WindowsManager) windowsUpdatesGuarded() (pkgs []models.Package, omitted bool) {
	return guardedWindowsUpdates(m.getWindowsUpdates)
}

func guardedWindowsUpdates(collect func() []models.Package) ([]models.Package, bool) {
	if !wuaMu.TryLock() {
		lastWindowsUpdatesMu.Lock()
		defer lastWindowsUpdatesMu.Unlock()
		if !lastWindowsUpdatesSet {
			return nil, true
		}
		out := make([]models.Package, len(lastWindowsUpdates))
		copy(out, lastWindowsUpdates)
		return out, false
	}
	defer wuaMu.Unlock()
	pkgs := collect()
	if pkgs != nil {
		lastWindowsUpdatesMu.Lock()
		lastWindowsUpdates = append([]models.Package(nil), pkgs...)
		lastWindowsUpdatesSet = true
		lastWindowsUpdatesMu.Unlock()
	}
	return pkgs, false
}

// UpdatableWingetApps returns installed applications without querying Windows Update.
func (m *WindowsManager) UpdatableWingetApps() []models.Package {
	return m.appPackages()
}

type lineLogger struct {
	log      func(string)
	pending  []byte
	captured strings.Builder
	limit    int
}

func newLineLogger(log func(string), limit int) *lineLogger {
	return &lineLogger{log: log, limit: limit}
}

func (l *lineLogger) Write(p []byte) (int, error) {
	l.pending = append(l.pending, p...)
	for {
		i := bytes.IndexByte(l.pending, '\n')
		if i < 0 {
			break
		}
		l.emit(string(l.pending[:i]))
		l.pending = l.pending[i+1:]
	}
	if len(l.pending) > 64*1024 {
		l.emit(string(l.pending))
		l.pending = nil
	}
	return len(p), nil
}

func (l *lineLogger) Flush() {
	if len(l.pending) > 0 {
		l.emit(string(l.pending))
		l.pending = nil
	}
}

func (l *lineLogger) emit(raw string) {
	line := strings.TrimRight(stripTerminalControls(raw), " \t")
	if strings.TrimSpace(line) == "" {
		return
	}
	if l.captured.Len() < l.limit {
		l.captured.WriteString(line)
		l.captured.WriteByte('\n')
	}
	if l.log != nil {
		l.log(line + "\n")
	}
}

func (l *lineLogger) Output() string {
	return l.captured.String()
}

type windowsPatchRun struct {
	req      WindowsPatchRequest
	hooks    WindowsPatchHooks
	wua      wuaBackend
	winget   wingetBackend
	timeouts windowsPatchTimeouts
	now      func() time.Time

	items          []*models.PatchRunItem
	byKey          map[string]*models.PatchRunItem
	runErrors      []string
	timeLimitNoted bool
}

func newWindowsPatchRun(req WindowsPatchRequest, hooks WindowsPatchHooks, wua wuaBackend, winget wingetBackend) *windowsPatchRun {
	return &windowsPatchRun{
		req:      req,
		hooks:    hooks,
		wua:      wua,
		winget:   winget,
		timeouts: defaultWindowsPatchTimeouts,
		now:      time.Now,
		byKey:    map[string]*models.PatchRunItem{},
	}
}

func (r *windowsPatchRun) out(format string, args ...interface{}) {
	if r.hooks.Output != nil {
		r.hooks.Output(fmt.Sprintf(format, args...))
	}
}

func (r *windowsPatchRun) log(s string) {
	if r.hooks.Output != nil {
		r.hooks.Output(s)
	}
}

func (r *windowsPatchRun) stopped() bool {
	return r.hooks.Stopped != nil && r.hooks.Stopped()
}

func (r *windowsPatchRun) ts() string {
	return r.now().UTC().Format(time.RFC3339)
}

func (r *windowsPatchRun) upsert(it models.PatchRunItem) *models.PatchRunItem {
	key := it.Kind + "\x00" + it.ID
	if cur, ok := r.byKey[key]; ok {
		if it.Title == "" {
			it.Title = cur.Title
		}
		if it.KB == "" {
			it.KB = cur.KB
		}
		if it.Revision == 0 {
			it.Revision = cur.Revision
		}
		*cur = it
		return cur
	}
	p := &it
	r.byKey[key] = p
	r.items = append(r.items, p)
	return p
}

func (r *windowsPatchRun) post(items ...*models.PatchRunItem) {
	if r.hooks.PostItems == nil || len(items) == 0 {
		return
	}
	for start := 0; start < len(items); start += maxItemsPerPost {
		end := min(start+maxItemsPerPost, len(items))
		batch := make([]models.PatchRunItem, 0, end-start)
		for _, it := range items[start:end] {
			batch = append(batch, *it)
		}
		r.hooks.PostItems(batch)
	}
}

func (r *windowsPatchRun) finish(it *models.PatchRunItem, status, message string) {
	it.Status = status
	it.Message = message
	it.CompletedAt = r.ts()
	r.post(it)
}

func (r *windowsPatchRun) withHeartbeat(label string, fn func()) {
	done := make(chan struct{})
	start := r.now()
	go func() {
		t := time.NewTicker(r.timeouts.heartbeat)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				r.out("%s: %dm elapsed\n", label, int(r.now().Sub(start).Minutes()))
			}
		}
	}()
	defer close(done)
	fn()
}

func (r *windowsPatchRun) targetIDs(kind string) []string {
	var ids []string
	for _, t := range r.req.Targets {
		if t.Kind == kind {
			ids = append(ids, t.ID)
		}
	}
	return ids
}

func (r *windowsPatchRun) interruptReason(runCtx context.Context) string {
	if r.stopped() {
		return msgStopped
	}
	if runCtx.Err() != nil {
		if !r.timeLimitNoted {
			r.timeLimitNoted = true
			r.runErrors = append(r.runErrors, msgTimeLimit)
		}
		return msgTimeLimit
	}
	return ""
}

// run executes the run. stopCtx is cancelled by a stop request and by runCtx;
// installs use runCtx only, so a stop never kills an install in progress.
func (r *windowsPatchRun) run(runCtx, stopCtx context.Context) WindowsPatchResult {
	if r.req.PatchAll && !r.req.IncludeUpdates && !r.req.IncludeApps {
		return WindowsPatchResult{Stage: "failed", ErrorMessage: "Patch all needs Windows Updates, applications, or both"}
	}
	wuaIDs := r.targetIDs(WindowsTargetWUA)
	wingetIDs := r.targetIDs(WindowsTargetWinget)
	if !r.req.PatchAll && len(wuaIDs)+len(wingetIDs) == 0 {
		return WindowsPatchResult{Stage: "failed", ErrorMessage: "no valid patch targets"}
	}
	if !r.req.PatchAll {
		for _, id := range wuaIDs {
			r.upsert(models.PatchRunItem{Kind: WindowsTargetWUA, ID: strings.ToLower(id), Status: itemPending})
		}
		for _, id := range wingetIDs {
			r.upsert(models.PatchRunItem{Kind: WindowsTargetWinget, ID: id, Status: itemPending})
		}
	}
	if r.req.DryRun {
		r.out("[PatchMon] Dry run: nothing will be downloaded or installed\n")
	}

	if (r.req.PatchAll && r.req.IncludeUpdates) || len(wuaIDs) > 0 {
		r.runWUAPhase(runCtx, stopCtx, wuaIDs)
	} else if r.req.PatchAll {
		r.out("[Windows Update] Skipped: not selected for this run\n")
	}
	if (r.req.PatchAll && r.req.IncludeApps) || len(wingetIDs) > 0 {
		r.runWingetPhase(runCtx, stopCtx, wingetIDs)
	} else if r.req.PatchAll {
		r.out("[winget] Skipped: not selected for this run\n")
	}

	res := WindowsPatchResult{}
	installedUpdate := false
	for _, it := range r.items {
		res.Items = append(res.Items, *it)
		if it.RebootRequired {
			res.RebootRequired = true
		}
		if it.Kind == WindowsTargetWUA && (it.Status == itemInstalled || it.Status == itemInstalledWithErrors) {
			installedUpdate = true
		}
	}
	if !r.req.DryRun && !res.RebootRequired && installedUpdate && r.hooks.RebootRequired != nil {
		res.RebootRequired = r.hooks.RebootRequired()
	}
	if len(res.Items) == 0 && len(r.runErrors) == 0 {
		r.out("Nothing to install\n")
	}
	if res.RebootRequired {
		r.out("\n[Restart required] A restart is required to finish installing updates. PatchMon does not restart the host.\n")
	}
	res.Stage, res.ErrorMessage = finalWindowsStage(res.Items, r.runErrors, r.stopped(), r.req.DryRun, !r.req.PatchAll)
	r.post(r.items...)
	return res
}

func (r *windowsPatchRun) failWUATargets(wuaIDs []string, status, message string) {
	var touched []*models.PatchRunItem
	for _, id := range wuaIDs {
		it := r.upsert(models.PatchRunItem{Kind: WindowsTargetWUA, ID: strings.ToLower(id), Status: status, Message: message})
		touched = append(touched, it)
	}
	r.post(touched...)
}

func (r *windowsPatchRun) runWUAPhase(runCtx, stopCtx context.Context, wuaIDs []string) {
	wuaMu.Lock()
	defer wuaMu.Unlock()

	r.out("[Windows Update] Searching for updates...\n")
	var scan wuaScan
	var err error
	r.withHeartbeat("[Windows Update] Searching for updates", func() {
		ctx, cancel := context.WithTimeout(stopCtx, r.timeouts.scan)
		defer cancel()
		scan, err = r.wua.Scan(ctx, r.log)
	})
	if err != nil {
		if reason := r.interruptReason(runCtx); reason != "" {
			r.failWUATargets(wuaIDs, itemSkipped, reason)
			return
		}
		msg := "Windows Update search failed: " + err.Error()
		r.out("[Windows Update] %s\n", msg)
		if r.req.PatchAll {
			r.runErrors = append(r.runErrors, msg)
		} else {
			r.failWUATargets(wuaIDs, itemFailed, msg)
		}
		return
	}

	install, decided := selectWUAUpdates(scan, r.req.PatchAll, wuaIDs)
	var touched []*models.PatchRunItem
	for _, it := range decided {
		it.CompletedAt = r.ts()
		touched = append(touched, r.upsert(it))
	}
	queued := make([]*models.PatchRunItem, 0, len(install))
	for _, u := range install {
		it := wuaItem(u, itemPending, "")
		if r.req.DryRun {
			it.Status = itemWouldInstall
			it.Message = rebootBehaviourText(u.RebootBehavior)
			if isFeatureUpgrade(u) {
				it.Message += "; feature upgrade"
			}
			it.CompletedAt = r.ts()
		}
		p := r.upsert(it)
		touched = append(touched, p)
		queued = append(queued, p)
	}
	r.post(touched...)
	r.out("[Windows Update] %d update(s) to install, %d not installable now\n", len(install), len(decided))
	for _, it := range decided {
		r.out("  %s: %s\n", displayName(it), it.Message)
	}
	if r.req.DryRun {
		for _, it := range queued {
			r.out("  Would install %s (%s)\n", displayName(*it), it.Message)
		}
		return
	}
	if len(install) == 0 {
		return
	}

	feature := false
	ids := make([]string, 0, len(install))
	for _, u := range install {
		ids = append(ids, u.ID)
		feature = feature || isFeatureUpgrade(u)
	}
	dlTimeout := r.timeouts.download
	if feature {
		dlTimeout = r.timeouts.downloadFeature
	}
	r.out("[Windows Update] Downloading %d update(s)...\n", len(ids))
	var dl map[string]wuaOpResult
	r.withHeartbeat("[Windows Update] Downloading", func() {
		ctx, cancel := context.WithTimeout(stopCtx, dlTimeout)
		defer cancel()
		dl, err = r.wua.Download(ctx, ids, r.log)
	})
	if err != nil {
		reason := r.interruptReason(runCtx)
		status := itemSkipped
		msg := reason
		if reason == "" {
			status = itemFailed
			msg = "Download failed: " + errorItemMessage(err)
		}
		for _, it := range queued {
			r.finish(it, status, msg)
		}
		return
	}

	for i, u := range install {
		it := queued[i]
		if res, ok := dl[strings.ToLower(u.ID)]; ok && res.ResultCode != 2 && res.ResultCode != 3 {
			it.HResult = normaliseHResult(res.HResult)
			rc := int32(res.ResultCode)
			it.ResultCode = &rc
			r.finish(it, itemFailed, "Download "+strings.ToLower(wuaResultMessage(res.ResultCode, res.HResult)))
			queued[i] = nil
		}
	}

	blocked := ""
	for i, u := range install {
		it := queued[i]
		if it == nil {
			continue
		}
		if blocked != "" {
			r.finish(it, itemSkipped, blocked)
			continue
		}
		if reason := r.interruptReason(runCtx); reason != "" {
			r.finish(it, itemSkipped, reason)
			continue
		}
		timeout := r.timeouts.install
		if isFeatureUpgrade(u) {
			timeout = r.timeouts.installFeature
		}
		it.StartedAt = r.ts()
		r.post(it)
		label := "[Windows Update] Installing " + displayName(*it)
		r.out("%s...\n", label)
		var res wuaOpResult
		var ierr error
		r.withHeartbeat(label, func() {
			ctx, cancel := context.WithTimeout(runCtx, timeout)
			defer cancel()
			res, ierr = r.wua.Install(ctx, u.ID, r.log)
		})
		r.applyInstallResult(it, res, ierr)
		r.out("  %s: %s\n", displayName(*it), it.Message)
		if it.Status == itemFailed && isInstallBlockedHResult(it.HResult) {
			blocked = msgRestartFirst
		}
	}
}

func (r *windowsPatchRun) applyInstallResult(it *models.PatchRunItem, res wuaOpResult, err error) {
	if err != nil {
		var se *wuaScriptError
		if errors.As(err, &se) {
			it.HResult = normaliseHResult(se.HResult)
		}
		r.finish(it, itemFailed, "Install failed: "+errorItemMessage(err))
		return
	}
	switch {
	case !res.Found:
		r.finish(it, itemNotApplicable, msgNoLongerOffered)
		return
	case res.AlreadyInstalled:
		r.finish(it, itemAlreadyInstalled, "Already installed")
		return
	case res.PendingReboot:
		it.RebootRequired = true
		r.finish(it, itemPendingReboot, msgPendingReboot)
		return
	}
	rc := int32(res.ResultCode)
	it.ResultCode = &rc
	it.HResult = normaliseHResult(res.HResult)
	it.RebootRequired = res.RebootRequired
	msg := wuaResultMessage(res.ResultCode, it.HResult)
	if res.RebootRequired {
		msg += "; restart required"
	}
	r.finish(it, wuaResultStatus(res.ResultCode), msg)
}

func errorItemMessage(err error) string {
	var se *wuaScriptError
	if errors.As(err, &se) {
		return se.itemMessage()
	}
	return err.Error()
}

func displayName(it models.PatchRunItem) string {
	if it.KB != "" && !strings.Contains(it.Title, it.KB) && it.Title != "" {
		return it.Title + " (" + it.KB + ")"
	}
	if it.Title != "" {
		return it.Title
	}
	return it.ID
}

func (r *windowsPatchRun) runWingetPhase(runCtx, stopCtx context.Context, wingetIDs []string) {
	if err := r.winget.Resolve(); err != nil {
		msg := "winget is not installed on this host"
		r.out("[winget] %s (%s)\n", msg, err.Error())
		var touched []*models.PatchRunItem
		for _, id := range wingetIDs {
			touched = append(touched, r.upsert(models.PatchRunItem{Kind: WindowsTargetWinget, ID: id, Status: itemFailed, Message: msg, CompletedAt: r.ts()}))
		}
		r.post(touched...)
		return
	}

	var queue []*models.PatchRunItem
	if r.req.PatchAll {
		r.out("[winget] Checking applications for upgrades...\n")
		seen := map[string]bool{}
		for _, p := range r.winget.UpdatableApps() {
			if !p.NeedsUpdate || p.WingetID == "" || !wingetIDPattern.MatchString(p.WingetID) || seen[strings.ToLower(p.WingetID)] {
				continue
			}
			seen[strings.ToLower(p.WingetID)] = true
			queue = append(queue, r.upsert(models.PatchRunItem{Kind: WindowsTargetWinget, ID: p.WingetID, Title: p.Name, Status: itemPending}))
		}
		r.post(queue...)
	} else {
		for _, id := range wingetIDs {
			queue = append(queue, r.byKey[WindowsTargetWinget+"\x00"+id])
		}
	}
	r.out("[winget] %d application(s) to check\n", len(queue))

	for _, it := range queue {
		if reason := r.interruptReason(runCtx); reason != "" {
			r.finish(it, itemSkipped, reason)
			continue
		}
		it.StartedAt = r.ts()
		var args []string
		var ctx context.Context
		var cancel context.CancelFunc
		label := "[winget] "
		if r.req.DryRun {
			args = wingetListArgs(it.ID)
			ctx, cancel = context.WithTimeout(stopCtx, r.timeouts.wingetList)
			label += "Checking " + it.ID
		} else {
			args = wingetUpgradeArgs(it.ID)
			ctx, cancel = context.WithTimeout(runCtx, r.timeouts.wingetUpgrade)
			label += "Upgrading " + it.ID
			r.post(it)
		}
		r.out("%s...\n", label)
		var code uint32
		var output string
		var err error
		r.withHeartbeat(label, func() {
			code, output, err = r.winget.Run(ctx, args, r.log)
		})
		cancel()
		if err != nil {
			if reason := r.interruptReason(runCtx); reason != "" && r.req.DryRun {
				r.finish(it, itemSkipped, reason)
				continue
			}
			r.finish(it, itemFailed, "winget did not complete: "+err.Error())
			r.out("  %s: %s\n", it.ID, it.Message)
			continue
		}
		oc := classifyWingetUpgradeExit(code, r.req.DryRun, output, it.ID)
		ec := int64(code)
		it.ExitCode = &ec
		it.RebootRequired = oc.RebootRequired
		r.finish(it, oc.Status, oc.Message)
		r.out("  %s: %s\n", it.ID, it.Message)
	}
}

// WindowsRunLimit is the overall time cap for a run. Named targets may include a feature
// upgrade, which needs its own download and install budget.
func WindowsRunLimit(req WindowsPatchRequest) time.Duration {
	const base = 4 * time.Hour
	if req.PatchAll {
		return base
	}
	for _, t := range req.Targets {
		if t.Kind == WindowsTargetWUA {
			return max(base, defaultWindowsPatchTimeouts.downloadFeature+defaultWindowsPatchTimeouts.installFeature+time.Hour)
		}
	}
	return base
}

// RunWindowsPatch runs a Windows patch run with the host's WUA and winget backends.
func RunWindowsPatch(runCtx, stopCtx context.Context, req WindowsPatchRequest, hooks WindowsPatchHooks, apps func() []models.Package) WindowsPatchResult {
	return newWindowsPatchRun(req, hooks, newWUAScriptBackend(), newWingetExecBackend(apps)).run(runCtx, stopCtx)
}

type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

type cappedBuffer struct {
	buf   *bytes.Buffer
	limit int
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if room := c.limit - c.buf.Len(); room > 0 {
		c.buf.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

// NewWindowsPatchRequest validates a run_patch message for Windows and returns the targets it dropped.
func NewWindowsPatchRequest(patchType string, dryRun bool, targets []WindowsPatchTarget, includeUpdates, includeApps *bool) (WindowsPatchRequest, []WindowsPatchTarget, error) {
	req := WindowsPatchRequest{
		PatchAll:       patchType == "patch_all",
		DryRun:         dryRun,
		IncludeUpdates: includeUpdates == nil || *includeUpdates,
		IncludeApps:    includeApps == nil || *includeApps,
	}
	var rejected []WindowsPatchTarget
	if req.PatchAll {
		if !req.IncludeUpdates && !req.IncludeApps {
			return req, nil, errors.New("patch all needs Windows Updates, applications, or both")
		}
		return req, nil, nil
	}
	seen := map[string]bool{}
	for _, t := range targets {
		if !ValidWindowsPatchTarget(t) {
			rejected = append(rejected, t)
			continue
		}
		if t.Kind == WindowsTargetWUA {
			t.ID = strings.ToLower(t.ID)
		}
		key := t.Kind + "\x00" + strings.ToLower(t.ID)
		if seen[key] {
			continue
		}
		seen[key] = true
		req.Targets = append(req.Targets, t)
	}
	if len(req.Targets) == 0 {
		return req, rejected, errors.New("no valid patch targets")
	}
	return req, rejected, nil
}
