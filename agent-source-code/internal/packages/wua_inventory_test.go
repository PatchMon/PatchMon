package packages

import (
	"encoding/json"
	"testing"
)

const (
	classSecurity   = "0fa1201d-4330-4fa8-8ae9-b877473b6441"
	classCritical   = "e6cf1350-c01b-414d-a61f-263d14d133b4"
	classDefinition = "e0789628-ce08-4437-be74-2495b842f43b"
	classUpgrades   = "3689bdc8-b205-4af4-8d4a-a63924c5e9d5"
)

// Shapes taken from the Windows 11 ARM64 VM scan in the design doc (section 13.5):
// MsrcSeverity empty on every pending update, and .NET titles that already carry their KB.
const wuaScanFixture = `[
 {"Title":"2026-09 .NET Framework Security Update (KB5126052)","KBIDs":["5126052"],"CategoryIDs":["` + classSecurity + `"],"WUACategories":["Mises à jour de la sécurité"],"WUAGuid":"11111111-1111-1111-1111-111111111111","WUAKb":"KB5126052","WUASeverity":"","NeedsUpdate":true,"CurrentVersion":"pending","TypeCode":1},
 {"Title":"2026-10 Cumulative Update for Windows 11","KBIDs":["5129195"],"CategoryIDs":["` + classSecurity + `"],"WUACategories":["Security Updates"],"WUAGuid":"22222222-2222-2222-2222-222222222222","WUAKb":"KB5129195","WUASeverity":"","NeedsUpdate":true,"CurrentVersion":"pending","TypeCode":1},
 {"Title":"Security Intelligence Update for Microsoft Defender Antivirus - KB2267602 (Version 1.419.1)","KBIDs":["2267602"],"CategoryIDs":["` + classDefinition + `"],"WUAGuid":"33333333-3333-3333-3333-333333333333","WUASeverity":"","NeedsUpdate":true,"CurrentVersion":"pending","TypeCode":1},
 {"Title":"Critical fix","KBIDs":["5000001"],"CategoryIDs":["` + classCritical + `"],"WUAGuid":"44444444-4444-4444-4444-444444444444","NeedsUpdate":true,"CurrentVersion":"pending","TypeCode":1},
 {"Title":"Contoso - Display - 31.0.101.5592","KBIDs":[],"CategoryIDs":[],"WUAGuid":"55555555-5555-5555-5555-555555555555","NeedsUpdate":true,"CurrentVersion":"pending","TypeCode":2,"BrowseOnly":true},
 {"Title":"Windows 11, version 25H2","KBIDs":[],"CategoryIDs":["` + classUpgrades + `"],"WUAGuid":"66666666-6666-6666-6666-666666666666","NeedsUpdate":true,"CurrentVersion":"pending","TypeCode":1},
 {"Title":"2026-09 .NET Framework Security Update (KB5126053)","KBIDs":["5126053"],"CategoryIDs":["` + classSecurity + `"],"WUAGuid":"77777777-7777-7777-7777-777777777777","NeedsUpdate":true,"CurrentVersion":"pending","TypeCode":1,"PendingReboot":true},
 {"Name":"KB5043080","CurrentVersion":"installed","NeedsUpdate":false,"IsSecurityUpdate":true,"WUAKb":"KB5043080"}
]`

func TestWUARawToPackageFromScan(t *testing.T) {
	var raw []wuaRawUpdate
	if err := json.Unmarshal([]byte(wuaScanFixture), &raw); err != nil {
		t.Fatal(err)
	}
	type want struct {
		name     string
		security bool
		needs    bool
		current  string
		wuaType  string
		browse   bool
	}
	wants := []want{
		{"2026-09 .NET Framework Security Update (KB5126052)", true, true, "pending", "Software", false},
		{"2026-10 Cumulative Update for Windows 11 (KB5129195)", true, true, "pending", "Software", false},
		{"Security Intelligence Update for Microsoft Defender Antivirus - KB2267602 (Version 1.419.1)", false, true, "pending", "Software", false},
		{"Critical fix (KB5000001)", false, true, "pending", "Software", false},
		{"Contoso - Display - 31.0.101.5592", false, true, "pending", "Driver", true},
		{"Windows 11, version 25H2", false, true, "pending", "FeatureUpgrade", false},
		{"2026-09 .NET Framework Security Update (KB5126053)", true, false, "pending restart", "Software", false},
		{"KB5043080", true, false, "installed", "", false},
	}
	if len(raw) != len(wants) {
		t.Fatalf("fixture has %d rows, want %d", len(raw), len(wants))
	}
	for i, u := range raw {
		pkg, ok := wuaRawToPackage(u)
		if !ok {
			t.Fatalf("row %d dropped", i)
		}
		w := wants[i]
		if pkg.Name != w.name || pkg.IsSecurityUpdate != w.security || pkg.NeedsUpdate != w.needs || pkg.CurrentVersion != w.current || pkg.WUAType != w.wuaType || pkg.WUABrowseOnly != w.browse {
			t.Errorf("row %d = {%q sec=%v needs=%v cur=%q type=%q browse=%v}, want %+v", i, pkg.Name, pkg.IsSecurityUpdate, pkg.NeedsUpdate, pkg.CurrentVersion, pkg.WUAType, pkg.WUABrowseOnly, w)
		}
		if pkg.Category != "Windows Update" {
			t.Errorf("row %d category %q", i, pkg.Category)
		}
	}
}

func TestWUAIsSecurityMatchesIDCaseInsensitively(t *testing.T) {
	if !wuaIsSecurity([]string{"0FA1201D-4330-4FA8-8AE9-B877473B6441"}, "") {
		t.Error("upper-case Security Updates ID not recognised")
	}
	if !wuaIsSecurity(nil, "Important") {
		t.Error("MSRC severity Important no longer treated as security")
	}
}

func TestWUARawToPackageDropsNameless(t *testing.T) {
	if _, ok := wuaRawToPackage(wuaRawUpdate{}); ok {
		t.Error("nameless row kept")
	}
}
