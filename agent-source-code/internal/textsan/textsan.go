// Package textsan normalises text collected from the host before the agent
// hashes it, logs it, or ships it to the server.
package textsan

import (
	"strings"
	"unicode/utf8"

	"patchmon-agent/pkg/models"
)

// Clean removes NUL and repairs invalid UTF-8.
//
// PostgreSQL can store neither: the server binds package data as jsonb, which
// rejects the \u0000 escape outright, so one padded registry value (several
// Windows installers pad DisplayVersion with trailing NULs) used to fail the
// host's entire report. The server sanitises defensively as well, for agents
// older than this; cleaning here keeps `patchmon-agent report --json` honest
// and stops the bad bytes entering the canonical hash.
//
// Both branches are scans that allocate nothing for clean input.
func Clean(s string) string {
	if s == "" {
		return s
	}
	if strings.IndexByte(s, 0) >= 0 {
		s = strings.ReplaceAll(s, "\x00", "")
	}
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "�")
	}
	return s
}

// CleanPackages cleans every string field in place and returns the slice for
// convenient chaining at a return statement.
func CleanPackages(pkgs []models.Package) []models.Package {
	for i := range pkgs {
		p := &pkgs[i]
		p.Name = Clean(p.Name)
		p.Description = Clean(p.Description)
		p.Category = Clean(p.Category)
		p.CurrentVersion = Clean(p.CurrentVersion)
		p.AvailableVersion = Clean(p.AvailableVersion)
		p.SourceRepository = Clean(p.SourceRepository)
		p.WUAGuid = Clean(p.WUAGuid)
		p.WUAKb = Clean(p.WUAKb)
		p.WUASeverity = Clean(p.WUASeverity)
		p.WUASupportURL = Clean(p.WUASupportURL)
		for j := range p.WUACategories {
			p.WUACategories[j] = Clean(p.WUACategories[j])
		}
	}
	return pkgs
}

// CleanRepositories cleans every string field in place and returns the slice.
func CleanRepositories(repos []models.Repository) []models.Repository {
	for i := range repos {
		r := &repos[i]
		r.Name = Clean(r.Name)
		r.URL = Clean(r.URL)
		r.Distribution = Clean(r.Distribution)
		r.Components = Clean(r.Components)
		r.RepoType = Clean(r.RepoType)
	}
	return repos
}
