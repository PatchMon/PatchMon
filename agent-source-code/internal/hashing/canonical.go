// Package hashing — canonical content hashes for hash-gated check-in.
//
// MUST produce byte-identical output to the server-side helper at
// internal/handler/hash_canonical.go. Cross-impl parity is enforced by a
// shared fixture file (testdata/canonical_fixtures.json) that both server and
// agent unit tests load and assert known hashes against.
//
// If you change a struct shape, sort key, NFC rule, or rounding precision
// here you MUST mirror the change in the server's hash_canonical.go and
// regenerate the fixture file.
package hashing

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"sort"
	"strings"

	"golang.org/x/text/unicode/norm"
	"patchmon-agent/pkg/models"
)

func nfc(s string) string {
	return strings.TrimSpace(norm.NFC.String(s))
}

func canonicalEncode(v any) ([]byte, error) {
	return json.Marshal(v)
}

func hashHex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// PackageHashRow is the canonical per-package row. Field order MUST match the
// server's PackageHashRow exactly.
type PackageHashRow struct {
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	Category          string   `json:"category"`
	CurrentVersion    string   `json:"currentVersion"`
	AvailableVersion  string   `json:"availableVersion"`
	NeedsUpdate       bool     `json:"needsUpdate"`
	IsSecurityUpdate  bool     `json:"isSecurityUpdate"`
	SourceRepository  string   `json:"sourceRepository"`
	WUAGuid           string   `json:"wuaGuid"`
	WUAKb             string   `json:"wuaKb"`
	WUASeverity       string   `json:"wuaSeverity"`
	WUACategories     []string `json:"wuaCategories"`
	WUASupportURL     string   `json:"wuaSupportUrl"`
	WUARevisionNumber int32    `json:"wuaRevisionNumber"`
}

// PackagesHash hashes the agent's package list. Pre-sorts by name and dedupes
// by name (last occurrence wins).
func PackagesHash(pkgs []models.Package) (string, error) {
	rows := make([]PackageHashRow, 0, len(pkgs))
	for i := range pkgs {
		p := &pkgs[i]
		cats := append([]string(nil), p.WUACategories...)
		sort.Strings(cats)
		for j, c := range cats {
			cats[j] = nfc(c)
		}
		rows = append(rows, PackageHashRow{
			Name:              nfc(p.Name),
			Description:       nfc(p.Description),
			Category:          nfc(p.Category),
			CurrentVersion:    nfc(p.CurrentVersion),
			AvailableVersion:  nfc(p.AvailableVersion),
			NeedsUpdate:       p.NeedsUpdate,
			IsSecurityUpdate:  p.IsSecurityUpdate,
			SourceRepository:  nfc(p.SourceRepository),
			WUAGuid:           nfc(p.WUAGuid),
			WUAKb:             nfc(p.WUAKb),
			WUASeverity:       nfc(p.WUASeverity),
			WUACategories:     cats,
			WUASupportURL:     nfc(p.WUASupportURL),
			WUARevisionNumber: p.WUARevisionNumber,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	if len(rows) > 1 {
		idx := make(map[string]int, len(rows))
		for i, r := range rows {
			idx[r.Name] = i
		}
		if len(idx) != len(rows) {
			deduped := make([]PackageHashRow, 0, len(idx))
			for i, r := range rows {
				if idx[r.Name] == i {
					deduped = append(deduped, r)
				}
			}
			rows = deduped
		}
	}
	b, err := canonicalEncode(rows)
	if err != nil {
		return "", err
	}
	return hashHex(b), nil
}

// RepoHashRow mirrors the server's RepoHashRow exactly.
type RepoHashRow struct {
	Name         string `json:"name"`
	URL          string `json:"url"`
	Distribution string `json:"distribution"`
	Components   string `json:"components"`
	RepoType     string `json:"repoType"`
	IsEnabled    bool   `json:"isEnabled"`
	IsSecure     bool   `json:"isSecure"`
}

// ReposHash hashes the repository list.
func ReposHash(repos []models.Repository) (string, error) {
	rows := make([]RepoHashRow, 0, len(repos))
	for i := range repos {
		r := &repos[i]
		rows = append(rows, RepoHashRow{
			Name:         nfc(r.Name),
			URL:          nfc(r.URL),
			Distribution: nfc(r.Distribution),
			Components:   nfc(r.Components),
			RepoType:     nfc(r.RepoType),
			IsEnabled:    r.IsEnabled,
			IsSecure:     r.IsSecure,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].URL != rows[j].URL {
			return rows[i].URL < rows[j].URL
		}
		if rows[i].Distribution != rows[j].Distribution {
			return rows[i].Distribution < rows[j].Distribution
		}
		return rows[i].Components < rows[j].Components
	})
	if len(rows) > 1 {
		key := func(r RepoHashRow) string {
			return r.URL + "|" + r.Distribution + "|" + r.Components
		}
		seen := make(map[string]int, len(rows))
		for i, r := range rows {
			seen[key(r)] = i
		}
		if len(seen) != len(rows) {
			deduped := make([]RepoHashRow, 0, len(seen))
			for i, r := range rows {
				if seen[key(r)] == i {
					deduped = append(deduped, r)
				}
			}
			rows = deduped
		}
	}
	b, err := canonicalEncode(rows)
	if err != nil {
		return "", err
	}
	return hashHex(b), nil
}

// InterfaceAddressHashRow mirrors the server's row.
type InterfaceAddressHashRow struct {
	Address string `json:"address"`
	Family  string `json:"family"`
	Netmask string `json:"netmask"`
	Gateway string `json:"gateway"`
}

// InterfaceHashRow mirrors the server's row. Addresses included; volatile
// link-layer fields (mtu, linkSpeed, duplex, status) excluded.
type InterfaceHashRow struct {
	Name       string                    `json:"name"`
	Type       string                    `json:"type"`
	MACAddress string                    `json:"macAddress"`
	Addresses  []InterfaceAddressHashRow `json:"addresses"`
}

// InterfacesHash hashes the network interface list.
func InterfacesHash(ifaces []models.NetworkInterface) (string, error) {
	rows := make([]InterfaceHashRow, 0, len(ifaces))
	for i := range ifaces {
		iface := &ifaces[i]
		addrs := make([]InterfaceAddressHashRow, 0, len(iface.Addresses))
		for j := range iface.Addresses {
			a := &iface.Addresses[j]
			addrs = append(addrs, InterfaceAddressHashRow{
				Address: nfc(a.Address),
				Family:  nfc(a.Family),
				Netmask: nfc(a.Netmask),
				Gateway: nfc(a.Gateway),
			})
		}
		sort.SliceStable(addrs, func(i, j int) bool {
			if addrs[i].Family != addrs[j].Family {
				return addrs[i].Family < addrs[j].Family
			}
			return addrs[i].Address < addrs[j].Address
		})
		rows = append(rows, InterfaceHashRow{
			Name:       nfc(iface.Name),
			Type:       nfc(iface.Type),
			MACAddress: strings.ToLower(nfc(iface.MACAddress)),
			Addresses:  addrs,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	b, err := canonicalEncode(rows)
	if err != nil {
		return "", err
	}
	return hashHex(b), nil
}

// HostnameHash hashes the hostname after NFC + trim. No JSON wrapper.
func HostnameHash(hostname string) string {
	return hashHex([]byte(nfc(hostname)))
}

// ----- docker --------------------------------------------------------------

// DockerContainerHashRow mirrors the server's row.
type DockerContainerHashRow struct {
	ContainerID     string `json:"containerId"`
	Name            string `json:"name"`
	ImageRepository string `json:"imageRepository"`
	ImageTag        string `json:"imageTag"`
	ImageID         string `json:"imageId"`
	Status          string `json:"status"`
	State           string `json:"state"`
}

// DockerImageHashRow mirrors the server's row.
type DockerImageHashRow struct {
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	ImageID    string `json:"imageId"`
	Digest     string `json:"digest"`
	SizeBytes  int64  `json:"sizeBytes"`
}

// DockerVolumeHashRow mirrors the server's row.
type DockerVolumeHashRow struct {
	VolumeID string `json:"volumeId"`
	Name     string `json:"name"`
	Driver   string `json:"driver"`
	Scope    string `json:"scope"`
}

// DockerNetworkHashRow mirrors the server's row.
type DockerNetworkHashRow struct {
	NetworkID string `json:"networkId"`
	Name      string `json:"name"`
	Driver    string `json:"driver"`
	Scope     string `json:"scope"`
}

// DockerImageUpdateHashRow mirrors the server's row.
type DockerImageUpdateHashRow struct {
	Repository      string `json:"repository"`
	CurrentTag      string `json:"currentTag"`
	AvailableTag    string `json:"availableTag"`
	CurrentDigest   string `json:"currentDigest"`
	AvailableDigest string `json:"availableDigest"`
	ImageID         string `json:"imageId"`
}

type dockerHashEnvelope struct {
	Containers []DockerContainerHashRow   `json:"containers"`
	Images     []DockerImageHashRow       `json:"images"`
	Volumes    []DockerVolumeHashRow      `json:"volumes"`
	Networks   []DockerNetworkHashRow     `json:"networks"`
	Updates    []DockerImageUpdateHashRow `json:"updates"`
}

// DockerHash hashes the agent's docker payload.
func DockerHash(d *models.DockerData) (string, error) {
	if d == nil {
		return hashHex([]byte("null")), nil
	}
	containers := make([]DockerContainerHashRow, 0, len(d.Containers))
	for i := range d.Containers {
		c := &d.Containers[i]
		containers = append(containers, DockerContainerHashRow{
			ContainerID:     nfc(c.ContainerID),
			Name:            nfc(c.Name),
			ImageRepository: nfc(c.ImageRepository),
			ImageTag:        nfc(c.ImageTag),
			ImageID:         nfc(c.ImageID),
			Status:          nfc(c.Status),
			State:           nfc(c.State),
		})
	}
	sort.SliceStable(containers, func(i, j int) bool {
		return containers[i].ContainerID < containers[j].ContainerID
	})

	images := make([]DockerImageHashRow, 0, len(d.Images))
	for i := range d.Images {
		im := &d.Images[i]
		images = append(images, DockerImageHashRow{
			Repository: nfc(im.Repository),
			Tag:        nfc(im.Tag),
			ImageID:    nfc(im.ImageID),
			Digest:     nfc(im.Digest),
			SizeBytes:  im.SizeBytes,
		})
	}
	sort.SliceStable(images, func(i, j int) bool {
		if images[i].Repository != images[j].Repository {
			return images[i].Repository < images[j].Repository
		}
		if images[i].Tag != images[j].Tag {
			return images[i].Tag < images[j].Tag
		}
		return images[i].ImageID < images[j].ImageID
	})

	volumes := make([]DockerVolumeHashRow, 0, len(d.Volumes))
	for i := range d.Volumes {
		v := &d.Volumes[i]
		volumes = append(volumes, DockerVolumeHashRow{
			VolumeID: nfc(v.VolumeID),
			Name:     nfc(v.Name),
			Driver:   nfc(v.Driver),
			Scope:    nfc(v.Scope),
		})
	}
	sort.SliceStable(volumes, func(i, j int) bool {
		return volumes[i].VolumeID < volumes[j].VolumeID
	})

	networks := make([]DockerNetworkHashRow, 0, len(d.Networks))
	for i := range d.Networks {
		n := &d.Networks[i]
		networks = append(networks, DockerNetworkHashRow{
			NetworkID: nfc(n.NetworkID),
			Name:      nfc(n.Name),
			Driver:    nfc(n.Driver),
			Scope:     nfc(n.Scope),
		})
	}
	sort.SliceStable(networks, func(i, j int) bool {
		return networks[i].NetworkID < networks[j].NetworkID
	})

	updates := make([]DockerImageUpdateHashRow, 0, len(d.Updates))
	for i := range d.Updates {
		u := &d.Updates[i]
		updates = append(updates, DockerImageUpdateHashRow{
			Repository:      nfc(u.Repository),
			CurrentTag:      nfc(u.CurrentTag),
			AvailableTag:    nfc(u.AvailableTag),
			CurrentDigest:   nfc(u.CurrentDigest),
			AvailableDigest: nfc(u.AvailableDigest),
			ImageID:         nfc(u.ImageID),
		})
	}
	sort.SliceStable(updates, func(i, j int) bool {
		if updates[i].Repository != updates[j].Repository {
			return updates[i].Repository < updates[j].Repository
		}
		if updates[i].CurrentTag != updates[j].CurrentTag {
			return updates[i].CurrentTag < updates[j].CurrentTag
		}
		return updates[i].ImageID < updates[j].ImageID
	})

	env := dockerHashEnvelope{
		Containers: containers,
		Images:     images,
		Volumes:    volumes,
		Networks:   networks,
		Updates:    updates,
	}
	b, err := canonicalEncode(env)
	if err != nil {
		return "", err
	}
	return hashHex(b), nil
}

// ----- compliance ----------------------------------------------------------

// ComplianceResultHashRow is one rule evaluation result. Mirrors the server's row.
type ComplianceResultHashRow struct {
	RuleRef     string `json:"ruleRef"`
	Status      string `json:"status"`
	Severity    string `json:"severity"`
	Section     string `json:"section"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Finding     string `json:"finding"`
	Actual      string `json:"actual"`
	Expected    string `json:"expected"`
	Remediation string `json:"remediation"`
}

// ComplianceScanHashRow is one scan, with results sorted by ruleRef.
// Mirrors the server's row.
type ComplianceScanHashRow struct {
	ProfileName        string                    `json:"profileName"`
	ProfileType        string                    `json:"profileType"`
	Status             string                    `json:"status"`
	Score              float64                   `json:"score"`
	TotalRules         int                       `json:"totalRules"`
	Passed             int                       `json:"passed"`
	Failed             int                       `json:"failed"`
	Warnings           int                       `json:"warnings"`
	Skipped            int                       `json:"skipped"`
	NotApplicable      int                       `json:"notApplicable"`
	RemediationApplied bool                      `json:"remediationApplied"`
	RemediationCount   int                       `json:"remediationCount"`
	Results            []ComplianceResultHashRow `json:"results"`
}

type complianceHashEnvelope struct {
	Scans []ComplianceScanHashRow `json:"scans"`
}

func roundFloat(v float64, digits int) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	mul := math.Pow10(digits)
	return math.Round(v*mul) / mul
}

// ComplianceHash hashes the agent's compliance scan data. Timestamps are
// excluded so identical findings re-hash the same regardless of when the scan
// ran.
func ComplianceHash(c *models.ComplianceData) (string, error) {
	if c == nil {
		return hashHex([]byte("null")), nil
	}
	scans := make([]ComplianceScanHashRow, 0, len(c.Scans))
	for i := range c.Scans {
		s := &c.Scans[i]
		results := make([]ComplianceResultHashRow, 0, len(s.Results))
		for j := range s.Results {
			r := &s.Results[j]
			results = append(results, ComplianceResultHashRow{
				RuleRef:     nfc(r.RuleID),
				Status:      nfc(r.Status),
				Severity:    nfc(r.Severity),
				Section:     nfc(r.Section),
				Title:       nfc(r.Title),
				Description: nfc(r.Description),
				Finding:     nfc(r.Finding),
				Actual:      nfc(r.Actual),
				Expected:    nfc(r.Expected),
				Remediation: nfc(r.Remediation),
			})
		}
		sort.SliceStable(results, func(i, j int) bool {
			if results[i].RuleRef != results[j].RuleRef {
				return results[i].RuleRef < results[j].RuleRef
			}
			return results[i].Status < results[j].Status
		})
		scans = append(scans, ComplianceScanHashRow{
			ProfileName:        nfc(s.ProfileName),
			ProfileType:        nfc(s.ProfileType),
			Status:             nfc(s.Status),
			Score:              roundFloat(s.Score, 4),
			TotalRules:         s.TotalRules,
			Passed:             s.Passed,
			Failed:             s.Failed,
			Warnings:           s.Warnings,
			Skipped:            s.Skipped,
			NotApplicable:      s.NotApplicable,
			RemediationApplied: s.RemediationApplied,
			RemediationCount:   s.RemediationCount,
			Results:            results,
		})
	}
	sort.SliceStable(scans, func(i, j int) bool {
		if scans[i].ProfileName != scans[j].ProfileName {
			return scans[i].ProfileName < scans[j].ProfileName
		}
		return scans[i].ProfileType < scans[j].ProfileType
	})
	env := complianceHashEnvelope{Scans: scans}
	b, err := canonicalEncode(env)
	if err != nil {
		return "", err
	}
	return hashHex(b), nil
}
