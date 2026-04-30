// Package handler — canonical content hashes for hash-gated check-in.
//
// The agent and server independently canonicalise the same logical content
// (packages, repos, interfaces, hostname, docker, compliance) and hash it
// with SHA-256. If the agent's hash matches what the server already has, the
// server skips the full payload. If they differ, the server asks the agent
// for that section.
//
// Both sides MUST produce byte-identical output. Cross-impl parity is
// enforced by a shared fixture file (testdata/canonical_fixtures.json) that
// both server and agent unit tests load and assert known hashes against.
//
// Rules that protect parity:
//   - Sort lists by stable identity tuple before encoding.
//   - Dedupe by the same identity tuple (last occurrence wins).
//   - JSON encode through a typed struct (no maps), all fields explicit
//     (no omitempty), so a missing field and a zero-valued field produce
//     the same bytes.
//   - Strings are NFC-normalised UTF-8 (golang.org/x/text/unicode/norm) and
//     trimmed of leading/trailing whitespace.
//   - Floats are rounded to fixed precision before encoding (compliance
//     scores at 4 dp, no other floats are hashed today).
//   - Output is sha256.Sum256 → lowercase hex, 64 chars.
package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"sort"
	"strings"

	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"golang.org/x/text/unicode/norm"
)

// nfc collapses the input to NFC and strips leading/trailing whitespace. The
// agent applies the exact same transform so hostnames written as "café" in
// either NFC or NFD produce the same hash.
func nfc(s string) string {
	return strings.TrimSpace(norm.NFC.String(s))
}

// canonicalEncode returns the deterministic JSON encoding of v. Uses
// json.Marshal which has stable key ordering for struct fields (in source
// order) — we only ever pass it slices of typed structs, never maps, so the
// output is fully determined by the input slice.
func canonicalEncode(v any) ([]byte, error) {
	return json.Marshal(v)
}

// hashHex hashes bytes and returns lowercase hex.
func hashHex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// ----- packages --------------------------------------------------------------

// PackageHashRow is the canonical per-package row. Field order matches the
// JSON tag order; do not reorder without bumping every stored hash.
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

// CanonicalPackagesHash hashes the agent's package list. Pre-sorts by name and
// dedupes by name (last occurrence wins, matching the upsert WHERE clause).
func CanonicalPackagesHash(pkgs []store.ReportPackage) (string, error) {
	rows := make([]PackageHashRow, 0, len(pkgs))
	for i := range pkgs {
		p := &pkgs[i]
		avail := ""
		if p.AvailableVersion != nil {
			avail = nfc(*p.AvailableVersion)
		}
		cats := append([]string(nil), p.WUACategories...)
		// WUA categories are sorted so reordering by Windows Update API does
		// not alter the hash.
		sort.Strings(cats)
		for j, c := range cats {
			cats[j] = nfc(c)
		}
		rows = append(rows, PackageHashRow{
			Name:              nfc(p.Name),
			Description:       nfc(p.Description),
			Category:          nfc(p.Category),
			CurrentVersion:    nfc(p.CurrentVersion),
			AvailableVersion:  avail,
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
	// Sort by name. Stable so duplicates (same name) preserve agent order
	// before dedup.
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	// Dedupe by name, last wins. Matches dedupePackagesByName in the report
	// store and the COALESCE semantics in BulkUpsertPackages.
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

// ----- repositories ----------------------------------------------------------

// RepoHashRow is the canonical per-repository row.
type RepoHashRow struct {
	Name         string `json:"name"`
	URL          string `json:"url"`
	Distribution string `json:"distribution"`
	Components   string `json:"components"`
	RepoType     string `json:"repoType"`
	IsEnabled    bool   `json:"isEnabled"`
	IsSecure     bool   `json:"isSecure"`
}

// CanonicalReposHash hashes the agent's repository list. Sorts and dedupes by
// the (URL, Distribution, Components) natural key (matches the migration-040
// UNIQUE constraint).
func CanonicalReposHash(repos []store.ReportRepository) (string, error) {
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

// ----- network interfaces ----------------------------------------------------

// InterfaceAddressHashRow is a single IP address bound to an interface.
type InterfaceAddressHashRow struct {
	Address string `json:"address"`
	Family  string `json:"family"`
	Netmask string `json:"netmask"`
	Gateway string `json:"gateway"`
}

// InterfaceHashRow is the canonical per-interface row.
//
// Per user decision (overriding the original §13.7 plan): addresses ARE
// included so DHCP / VPN / k8s-induced IP changes trigger a full report and
// the dashboard's "current IP" view stays current. Volatile link-layer
// fields (mtu, linkSpeed, duplex, status) are still excluded — those flap
// on autoneg / cable disconnect and would generate spurious churn.
type InterfaceHashRow struct {
	Name       string                    `json:"name"`
	Type       string                    `json:"type"`
	MACAddress string                    `json:"macAddress"`
	Addresses  []InterfaceAddressHashRow `json:"addresses"`
}

// decodeNetworkInterfaces parses an agent-supplied networkInterfaces JSONB
// blob (json.RawMessage on the wire) into typed structs the canonical hash
// helper can consume. Returns nil + nil for empty input — caller should
// treat that as "no interfaces in this report" rather than an error.
func decodeNetworkInterfaces(raw []byte) ([]models.NetworkInterface, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var out []models.NetworkInterface
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CanonicalInterfacesHash hashes the agent's NetworkInterface list. Each
// interface's addresses are also sorted by (family, address) to keep the
// hash stable across address-discovery ordering.
func CanonicalInterfacesHash(ifaces []models.NetworkInterface) (string, error) {
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

// ----- hostname --------------------------------------------------------------

// CanonicalHostnameHash hashes a single hostname string after NFC + trim. A
// scalar string does not need a JSON wrapper — feeding the raw NFC bytes to
// SHA-256 gives a stable, simpler hash.
func CanonicalHostnameHash(hostname string) string {
	return hashHex([]byte(nfc(hostname)))
}

// ----- docker ----------------------------------------------------------------

// DockerContainerHashRow is the stable identity of a docker container.
type DockerContainerHashRow struct {
	ContainerID     string `json:"containerId"`
	Name            string `json:"name"`
	ImageRepository string `json:"imageRepository"`
	ImageTag        string `json:"imageTag"`
	ImageID         string `json:"imageId"`
	Status          string `json:"status"`
	State           string `json:"state"`
}

// DockerImageHashRow is the stable identity of a docker image.
type DockerImageHashRow struct {
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	ImageID    string `json:"imageId"`
	Digest     string `json:"digest"`
	SizeBytes  int64  `json:"sizeBytes"`
}

// DockerVolumeHashRow is the stable identity of a docker volume.
type DockerVolumeHashRow struct {
	VolumeID string `json:"volumeId"`
	Name     string `json:"name"`
	Driver   string `json:"driver"`
	Scope    string `json:"scope"`
}

// DockerNetworkHashRow is the stable identity of a docker network.
type DockerNetworkHashRow struct {
	NetworkID string `json:"networkId"`
	Name      string `json:"name"`
	Driver    string `json:"driver"`
	Scope     string `json:"scope"`
}

// DockerImageUpdateHashRow is the stable identity of a docker image update.
type DockerImageUpdateHashRow struct {
	Repository      string `json:"repository"`
	CurrentTag      string `json:"currentTag"`
	AvailableTag    string `json:"availableTag"`
	CurrentDigest   string `json:"currentDigest"`
	AvailableDigest string `json:"availableDigest"`
	ImageID         string `json:"imageId"`
}

// DockerHashEnvelope wraps the four sorted slices into one stable struct.
type DockerHashEnvelope struct {
	Containers []DockerContainerHashRow   `json:"containers"`
	Images     []DockerImageHashRow       `json:"images"`
	Volumes    []DockerVolumeHashRow      `json:"volumes"`
	Networks   []DockerNetworkHashRow     `json:"networks"`
	Updates    []DockerImageUpdateHashRow `json:"updates"`
}

// DockerWireContainer mirrors the agent's models.DockerContainer wire shape
// (snake_case JSON tags). The handler decodes the agent payload into this
// shape before hashing. Field names match the agent's; JSON tags MUST match
// agent-source-code/pkg/models/integrations.go exactly.
type DockerWireContainer struct {
	ContainerID     string `json:"container_id"`
	Name            string `json:"name"`
	ImageRepository string `json:"image_repository"`
	ImageTag        string `json:"image_tag"`
	ImageID         string `json:"image_id"`
	Status          string `json:"status"`
	State           string `json:"state"`
}

// DockerWireImage mirrors the agent's models.DockerImage wire shape.
type DockerWireImage struct {
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	ImageID    string `json:"image_id"`
	Digest     string `json:"digest"`
	SizeBytes  int64  `json:"size_bytes"`
}

// DockerWireVolume mirrors the agent's models.DockerVolume wire shape.
type DockerWireVolume struct {
	VolumeID string `json:"volume_id"`
	Name     string `json:"name"`
	Driver   string `json:"driver"`
	Scope    string `json:"scope"`
}

// DockerWireNetwork mirrors the agent's models.DockerNetwork wire shape.
type DockerWireNetwork struct {
	NetworkID string `json:"network_id"`
	Name      string `json:"name"`
	Driver    string `json:"driver"`
	Scope     string `json:"scope"`
}

// DockerWireImageUpdate mirrors the agent's models.DockerImageUpdate wire
// shape.
type DockerWireImageUpdate struct {
	Repository      string `json:"repository"`
	CurrentTag      string `json:"current_tag"`
	AvailableTag    string `json:"available_tag"`
	CurrentDigest   string `json:"current_digest"`
	AvailableDigest string `json:"available_digest"`
	ImageID         string `json:"image_id"`
}

// DockerHashInput is the wire-shape input both server and agent feed into the
// hash function. JSON tags match the agent's outbound payload so the same
// fixture file decodes identically on both sides.
type DockerHashInput struct {
	Containers []DockerWireContainer   `json:"containers"`
	Images     []DockerWireImage       `json:"images"`
	Volumes    []DockerWireVolume      `json:"volumes"`
	Networks   []DockerWireNetwork     `json:"networks"`
	Updates    []DockerWireImageUpdate `json:"updates"`
}

// CanonicalDockerHash hashes the docker payload. Each list is sorted by its
// stable id; volatile metadata (timestamps, port maps, labels, restart
// counts, container counts) is excluded so transient state does not force
// re-uploads. The next docker upload will refresh the metadata anyway.
func CanonicalDockerHash(in DockerHashInput) (string, error) {
	containers := make([]DockerContainerHashRow, 0, len(in.Containers))
	for i := range in.Containers {
		c := &in.Containers[i]
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

	images := make([]DockerImageHashRow, 0, len(in.Images))
	for i := range in.Images {
		im := &in.Images[i]
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

	volumes := make([]DockerVolumeHashRow, 0, len(in.Volumes))
	for i := range in.Volumes {
		v := &in.Volumes[i]
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

	networks := make([]DockerNetworkHashRow, 0, len(in.Networks))
	for i := range in.Networks {
		n := &in.Networks[i]
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

	updates := make([]DockerImageUpdateHashRow, 0, len(in.Updates))
	for i := range in.Updates {
		u := &in.Updates[i]
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

	env := DockerHashEnvelope{
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

// ----- compliance ------------------------------------------------------------

// ComplianceResultHashRow is one rule evaluation result.
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
//
// Timestamps are EXCLUDED. Two scans with identical findings produce the same
// hash so we don't force a re-upload after every on-demand scan when the
// underlying compliance posture has not changed.
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

// ComplianceHashEnvelope wraps the scans + minimal scanner info.
type ComplianceHashEnvelope struct {
	Scans []ComplianceScanHashRow `json:"scans"`
}

// ComplianceWireResult mirrors the agent's models.ComplianceResult wire shape.
// JSON tags MUST match agent-source-code/pkg/models/compliance.go exactly.
// Note: agent uses "rule_ref" (not "rule_id") because the original wire
// contract treats rule_id and rule_ref differently.
type ComplianceWireResult struct {
	RuleRef     string `json:"rule_ref"`
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

// ComplianceWireScan mirrors the agent's models.ComplianceScan wire shape.
type ComplianceWireScan struct {
	ProfileName        string                 `json:"profile_name"`
	ProfileType        string                 `json:"profile_type"`
	Status             string                 `json:"status"`
	Score              float64                `json:"score"`
	TotalRules         int                    `json:"total_rules"`
	Passed             int                    `json:"passed"`
	Failed             int                    `json:"failed"`
	Warnings           int                    `json:"warnings"`
	Skipped            int                    `json:"skipped"`
	NotApplicable      int                    `json:"not_applicable"`
	RemediationApplied bool                   `json:"remediation_applied"`
	RemediationCount   int                    `json:"remediation_count"`
	Results            []ComplianceWireResult `json:"results"`
}

// ComplianceHashInput is the canonical inbound shape for hashing. JSON tags
// match the agent's outbound compliance payload so the same fixture decodes
// on both sides.
type ComplianceHashInput struct {
	Scans []ComplianceWireScan `json:"scans"`
}

// roundFloat rounds a float to `digits` decimal places. We use 4 dp for
// compliance scores: enough precision to distinguish meaningful scoring
// changes (e.g. 92.4567 vs 92.4571) while collapsing FP-renorm noise that
// would otherwise differ across platforms / Go versions.
func roundFloat(v float64, digits int) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	mul := math.Pow10(digits)
	return math.Round(v*mul) / mul
}

// CanonicalComplianceHash hashes the compliance scan results, excluding
// timestamps and run-specific metadata so identical findings re-hash the
// same way regardless of when the scan ran.
func CanonicalComplianceHash(in ComplianceHashInput) (string, error) {
	scans := make([]ComplianceScanHashRow, 0, len(in.Scans))
	for i := range in.Scans {
		s := &in.Scans[i]
		results := make([]ComplianceResultHashRow, 0, len(s.Results))
		for j := range s.Results {
			r := &s.Results[j]
			results = append(results, ComplianceResultHashRow{
				RuleRef:     nfc(r.RuleRef),
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
	env := ComplianceHashEnvelope{Scans: scans}
	b, err := canonicalEncode(env)
	if err != nil {
		return "", err
	}
	return hashHex(b), nil
}
