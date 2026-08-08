package store

import (
	"bytes"
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// nulEscape is the JSON encoding of U+0000. Because all four hex digits are
// zero there is no case variance to match on.
const nulEscape = `\u0000`

// sanitizeText strips NUL and repairs invalid UTF-8 in an agent-supplied
// string.
//
// PostgreSQL cannot store U+0000 in either target type this package writes to:
// `jsonb` rejects the \u0000 escape with 22P05 (unsupported Unicode escape
// sequence) and `text` rejects the raw byte with 22021. Windows hosts do send
// it — several installers pad registry values such as DisplayVersion with
// trailing NULs, which survive ConvertTo-Json, the agent, and json.Marshal
// here, and then abort the whole report transaction.
//
// Both checks are scans with no allocation on the overwhelmingly common clean
// input, so this is safe to run over every string in every report.
func sanitizeText(s string) string {
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

func sanitizeTextPtr(p *string) {
	if p != nil {
		*p = sanitizeText(*p)
	}
}

func sanitizeTextSlice(ss []string) {
	for i := range ss {
		ss[i] = sanitizeText(ss[i])
	}
}

// sanitizeRawJSON cleans a pre-encoded JSON blob that is bound straight to a
// jsonb parameter. A raw 0x00 byte would already have failed json.Valid, so
// the only reachable form is the \u0000 escape. Re-encoding is confined to
// blobs that actually contain one: everything else is returned untouched.
func sanitizeRawJSON(raw json.RawMessage) json.RawMessage {
	if !bytes.Contains(raw, []byte(nulEscape)) {
		return raw
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return raw
	}
	cleaned, err := json.Marshal(scrubJSONValue(v))
	if err != nil {
		return raw
	}
	return cleaned
}

func scrubJSONValue(v any) any {
	switch t := v.(type) {
	case string:
		return sanitizeText(t)
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[sanitizeText(k)] = scrubJSONValue(val)
		}
		return out
	case []any:
		for i := range t {
			t[i] = scrubJSONValue(t[i])
		}
		return t
	default:
		return v
	}
}

// sanitizeReportPayload cleans every agent-supplied string in the payload,
// in place.
//
// This must run AFTER the handler's canonical-hash drift check and BEFORE
// sortReportInputs. The agent hashes the values it collected, so the server
// has to hash the same bytes or every report from an affected host would be
// rejected as a hash mismatch instead of stored. Running before the sort and
// dedup matters too: stripping NUL can make two package names collide, and
// dedupe-by-name downstream is what keeps that from reaching ON CONFLICT.
func sanitizeReportPayload(p *ReportPayload) {
	for i := range p.Packages {
		pkg := &p.Packages[i]
		pkg.Name = sanitizeText(pkg.Name)
		pkg.Description = sanitizeText(pkg.Description)
		pkg.Category = sanitizeText(pkg.Category)
		pkg.CurrentVersion = sanitizeText(pkg.CurrentVersion)
		sanitizeTextPtr(pkg.AvailableVersion)
		pkg.SourceRepository = sanitizeText(pkg.SourceRepository)
		pkg.WUAGuid = sanitizeText(pkg.WUAGuid)
		pkg.WUAKb = sanitizeText(pkg.WUAKb)
		pkg.WUASeverity = sanitizeText(pkg.WUASeverity)
		pkg.WUASupportURL = sanitizeText(pkg.WUASupportURL)
		sanitizeTextSlice(pkg.WUACategories)
	}

	for i := range p.Repositories {
		repo := &p.Repositories[i]
		repo.Name = sanitizeText(repo.Name)
		repo.URL = sanitizeText(repo.URL)
		repo.Distribution = sanitizeText(repo.Distribution)
		repo.Components = sanitizeText(repo.Components)
		repo.RepoType = sanitizeText(repo.RepoType)
	}

	p.OSType = sanitizeText(p.OSType)
	p.OSVersion = sanitizeText(p.OSVersion)
	p.Hostname = sanitizeText(p.Hostname)
	p.IP = sanitizeText(p.IP)
	p.Architecture = sanitizeText(p.Architecture)
	p.AgentVersion = sanitizeText(p.AgentVersion)
	p.MachineID = sanitizeText(p.MachineID)
	p.KernelVersion = sanitizeText(p.KernelVersion)
	p.InstalledKernelVersion = sanitizeText(p.InstalledKernelVersion)
	p.SELinuxStatus = sanitizeText(p.SELinuxStatus)
	p.SystemUptime = sanitizeText(p.SystemUptime)
	p.CPUModel = sanitizeText(p.CPUModel)
	p.GatewayIP = sanitizeText(p.GatewayIP)
	p.RebootReason = sanitizeText(p.RebootReason)
	p.PackageManager = sanitizeText(p.PackageManager)
	sanitizeTextSlice(p.DNSServers)

	p.DiskDetails = sanitizeRawJSON(p.DiskDetails)
	p.NetworkInterfaces = sanitizeRawJSON(p.NetworkInterfaces)
}
