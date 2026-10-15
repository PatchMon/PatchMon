package packages

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
)

type zypperStream struct {
	Messages       []zypperMessage       `xml:"message"`
	Updates        []zypperUpdate        `xml:"update-status>update-list>update"`
	BlockedUpdates []zypperUpdate        `xml:"update-status>blocked-update-list>update"`
	AppletInfo     []zypperAppletInfo    `xml:"update-status>update-list>appletinfo"`
	InstallSummary *ZypperInstallSummary `xml:"install-summary"`
	Repos          []ZypperRepo          `xml:"repo-list>repo"`
	Locks          []ZypperLock          `xml:"locks>lock"`
	Prompts        []zypperPrompt        `xml:"prompt"`
}

type zypperPrompt struct {
	Description string `xml:"description"`
}

type zypperMessage struct {
	Type string `xml:"type,attr"`
	Text string `xml:",chardata"`
}

type zypperAppletInfo struct {
	Status string `xml:"status,attr"`
}

type zypperUpdate struct {
	Kind        string `xml:"kind,attr"`
	Name        string `xml:"name,attr"`
	Edition     string `xml:"edition,attr"`
	EditionOld  string `xml:"edition-old,attr"`
	Arch        string `xml:"arch,attr"`
	Status      string `xml:"status,attr"`
	Category    string `xml:"category,attr"`
	Severity    string `xml:"severity,attr"`
	PkgManager  string `xml:"pkgmanager,attr"`
	Interactive string `xml:"interactive,attr"`
	Summary     string `xml:"summary"`
	Source      struct {
		Alias string `xml:"alias,attr"`
		URL   string `xml:"url,attr"`
	} `xml:"source"`
}

// ZypperSolvable is one package, patch, pattern or product in a zypper transaction summary.
type ZypperSolvable struct {
	Type       string `xml:"type,attr"`
	Name       string `xml:"name,attr"`
	Edition    string `xml:"edition,attr"`
	EditionOld string `xml:"edition-old,attr"`
	Arch       string `xml:"arch,attr"`
	ArchOld    string `xml:"arch-old,attr"`
	Repository string `xml:"repository,attr"`
}

// ZypperInstallSummary is the <install-summary> zypper prints for a transaction.
type ZypperInstallSummary struct {
	PackagesToChange int              `xml:"packages-to-change,attr"`
	NeedRestart      int              `xml:"need-restart,attr"`
	NeedReboot       int              `xml:"need-reboot,attr"`
	Upgrade          []ZypperSolvable `xml:"to-upgrade>solvable"`
	Install          []ZypperSolvable `xml:"to-install>solvable"`
	Remove           []ZypperSolvable `xml:"to-remove>solvable"`
	Downgrade        []ZypperSolvable `xml:"to-downgrade>solvable"`
	Reinstall        []ZypperSolvable `xml:"to-reinstall>solvable"`
	ChangeArch       []ZypperSolvable `xml:"to-change-arch>solvable"`
	ChangeVendor     []ZypperSolvable `xml:"to-change-vendor>solvable"`
}

// ZypperRepo is one <repo> from zypper --xmlout repos.
type ZypperRepo struct {
	Alias   string `xml:"alias,attr"`
	Name    string `xml:"name,attr"`
	Type    string `xml:"type,attr"`
	Enabled string `xml:"enabled,attr"`
	URL     string `xml:"url"`
}

// ZypperLock is one entry from zypper --xmlout locks.
type ZypperLock struct {
	Name string `xml:"name"`
	Type string `xml:"type"`
}

func parseZypperStream(out []byte) (zypperStream, error) {
	var s zypperStream
	start := bytes.Index(out, []byte("<stream"))
	if start < 0 {
		return s, fmt.Errorf("zypper printed no XML stream")
	}
	if err := xml.Unmarshal(out[start:], &s); err != nil {
		return s, fmt.Errorf("parsing zypper XML: %w", err)
	}
	return s, nil
}

func (s zypperStream) messages(kind string) []string {
	var out []string
	for _, m := range s.Messages {
		if m.Type == kind {
			if t := strings.TrimSpace(m.Text); t != "" {
				out = append(out, t)
			}
		}
	}
	return out
}

func (s zypperStream) noUpdateRepositories() bool {
	for _, a := range s.AppletInfo {
		if a.Status == "no-update-repositories" {
			return true
		}
	}
	return false
}
