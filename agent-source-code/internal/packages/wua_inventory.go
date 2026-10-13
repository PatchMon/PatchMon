package packages

import (
	"strings"

	"patchmon-agent/pkg/models"
)

const (
	wuaUpdateTypeSoftware = "Software"
	wuaUpdateTypeDriver   = "Driver"
	wuaUpdateTypeFeature  = "FeatureUpgrade"

	wuaPendingRestartVersion = "pending restart"
)

type wuaRawUpdate struct {
	Name              string   `json:"Name"`
	CurrentVersion    string   `json:"CurrentVersion"`
	AvailableVersion  string   `json:"AvailableVersion"`
	NeedsUpdate       bool     `json:"NeedsUpdate"`
	IsSecurityUpdate  bool     `json:"IsSecurityUpdate"`
	WUAGuid           string   `json:"WUAGuid"`
	WUAKb             string   `json:"WUAKb"`
	WUASeverity       string   `json:"WUASeverity"`
	WUACategories     []string `json:"WUACategories"`
	WUASupportURL     string   `json:"WUASupportURL"`
	WUARevisionNumber int32    `json:"WUARevisionNumber"`
	Title             string   `json:"Title"`
	KBIDs             []string `json:"KBIDs"`
	CategoryIDs       []string `json:"CategoryIDs"`
	BrowseOnly        bool     `json:"BrowseOnly"`
	TypeCode          int      `json:"TypeCode"`
	PendingReboot     bool     `json:"PendingReboot"`
}

func wuaRawToPackage(u wuaRawUpdate) (models.Package, bool) {
	pkg := models.Package{
		Name:              u.Name,
		Category:          "Windows Update",
		CurrentVersion:    u.CurrentVersion,
		AvailableVersion:  u.AvailableVersion,
		NeedsUpdate:       u.NeedsUpdate,
		IsSecurityUpdate:  u.IsSecurityUpdate,
		WUAGuid:           u.WUAGuid,
		WUAKb:             u.WUAKb,
		WUASeverity:       u.WUASeverity,
		WUACategories:     u.WUACategories,
		WUASupportURL:     u.WUASupportURL,
		WUARevisionNumber: u.WUARevisionNumber,
	}
	if u.Title != "" {
		pkg.Name = wuaDisplayName(u.Title, u.KBIDs)
		pkg.IsSecurityUpdate = wuaIsSecurity(u.CategoryIDs, u.WUASeverity)
		pkg.WUABrowseOnly = u.BrowseOnly
		pkg.WUAType = wuaUpdateType(u.TypeCode, u.CategoryIDs)
		if u.PendingReboot {
			pkg.NeedsUpdate = false
			pkg.CurrentVersion = wuaPendingRestartVersion
		}
	}
	if pkg.Name == "" {
		return models.Package{}, false
	}
	if pkg.CurrentVersion == "" {
		pkg.CurrentVersion = "pending"
	}
	return pkg, true
}

func wuaDisplayName(title string, kbs []string) string {
	title = strings.TrimSpace(title)
	var missing []string
	for _, kb := range kbs {
		kb = strings.TrimSpace(kb)
		if kb == "" {
			continue
		}
		if !strings.HasPrefix(strings.ToUpper(kb), "KB") {
			kb = "KB" + kb
		}
		if !strings.Contains(strings.ToUpper(title), strings.ToUpper(kb)) {
			missing = append(missing, kb)
		}
	}
	if len(missing) == 0 {
		return title
	}
	return title + " (" + strings.Join(missing, ", ") + ")"
}

func wuaHasClass(classIDs []string, id string) bool {
	for _, c := range classIDs {
		if strings.EqualFold(strings.TrimSpace(c), id) {
			return true
		}
	}
	return false
}

func wuaIsSecurity(classIDs []string, severity string) bool {
	return wuaHasClass(classIDs, wuaCategorySecurity) || severity == "Critical" || severity == "Important"
}

func wuaUpdateType(typeCode int, classIDs []string) string {
	switch {
	case wuaHasClass(classIDs, wuaCategoryUpgrades):
		return wuaUpdateTypeFeature
	case typeCode == wuaTypeDriver:
		return wuaUpdateTypeDriver
	}
	return wuaUpdateTypeSoftware
}
