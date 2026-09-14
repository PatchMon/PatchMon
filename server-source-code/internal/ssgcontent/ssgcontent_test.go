package ssgcontent

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// datastream mirrors the shape of a real ComplianceAsCode datastream as far as
// the version scan is concerned: the collection header, a cpe-dictionary
// component ahead of the benchmark, and the version inside the benchmark.
func datastream(version string) string {
	return `<?xml version="1.0" encoding="utf-8"?>
<ds:data-stream-collection
    xmlns:cpe-dict="http://cpe.mitre.org/dictionary/2.0"
    xmlns:dc="http://purl.org/dc/elements/1.1/"
    xmlns:ds="http://scap.nist.gov/schema/scap/source/1.2"
    xmlns:xccdf-1.2="http://checklists.nist.gov/xccdf/1.2"
    id="scap_org.open-scap_collection_from_xccdf_ssg-debian13-xccdf.xml" schematron-version="1.3">
  <ds:data-stream id="scap_org.open-scap_datastream_from_xccdf_ssg-debian13-xccdf.xml" scap-version="1.3" use-case="OTHER">
    <ds:checklists>
      <ds:component-ref id="scap_org.open-scap_cref_ssg-debian13-xccdf.xml"/>
    </ds:checklists>
  </ds:data-stream>
  <ds:component id="scap_org.open-scap_comp_ssg-debian13-cpe-dictionary.xml">
    <cpe-dict:cpe-list>
      <cpe-dict:cpe-item name="cpe:/o:debian:debian_linux:13">
        <cpe-dict:title xml:lang="en-us">Debian 13</cpe-dict:title>
      </cpe-dict:cpe-item>
    </cpe-dict:cpe-list>
  </ds:component>
  <ds:component id="scap_org.open-scap_comp_ssg-debian13-xccdf.xml">
    <xccdf-1.2:Benchmark id="xccdf_org.ssgproject.content_benchmark_DEBIAN-13">
      <xccdf-1.2:status date="2026-06-01">draft</xccdf-1.2:status>
      <xccdf-1.2:title xml:lang="en-US">Guide to the Secure Configuration of Debian 13</xccdf-1.2:title>
      <xccdf-1.2:platform idref="cpe:/o:debian:debian_linux:13"/>
      <xccdf-1.2:version update="https://github.com/ComplianceAsCode/content/releases/latest">` + version + `</xccdf-1.2:version>
      <xccdf-1.2:metadata>
        <dc:publisher>SCAP Security Guide Project</dc:publisher>
      </xccdf-1.2:metadata>
    </xccdf-1.2:Benchmark>
  </ds:component>
</ds:data-stream-collection>`
}

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// The reported case (#1060): the upstream ComplianceAsCode release tarball
// carries its version only in the top-level directory name, which extraction
// strips, so an install populated from upstream has real datastreams and no
// marker. Reporting that as "no content" left the UI claiming nothing was
// configured and, worse, 503'd every agent asking for content.
func TestVersionDerivedFromDatastreamWhenMarkerAbsent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	write(t, dir, "ssg-debian13-ds.xml", datastream("0.1.81"))
	for _, sub := range []string{"ansible", "bash", "guides", "manifests"} {
		if err := os.Mkdir(filepath.Join(dir, sub), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	write(t, dir, "README.md", "# SCAP Security Guide")

	if got := Version(dir); got != "0.1.81" {
		t.Errorf("Version() = %q, want %q", got, "0.1.81")
	}
}

// The Docker build writes the marker, so it stays the fast path and is trusted
// over the content when both are present.
func TestVersionPrefersMarker(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	write(t, dir, markerFile, "0.1.81\n")
	write(t, dir, "ssg-debian13-ds.xml", datastream("0.1.70"))

	if got := Version(dir); got != "0.1.81" {
		t.Errorf("Version() = %q, want the marker value %q", got, "0.1.81")
	}
}

func TestVersionEmptyWithoutContent(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"no directory configured": "",
		"missing directory":       filepath.Join(t.TempDir(), "absent"),
		"empty directory":         t.TempDir(),
	}
	for name, dir := range tests {
		if got := Version(dir); got != "" {
			t.Errorf("%s: Version() = %q, want empty", name, got)
		}
	}
}

// A datastream that cannot be parsed must not stop the scan: the remaining
// files in a release all carry the same version.
func TestVersionSkipsUnparseableDatastream(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	write(t, dir, "ssg-aaa-ds.xml", "not xml at all")
	write(t, dir, "ssg-bbb-ds.xml", datastream("0.1.81"))

	if got := Version(dir); got != "0.1.81" {
		t.Errorf("Version() = %q, want %q", got, "0.1.81")
	}
}

// The scan gives up after parseAttempts files. Pinned because the truncation is
// deliberate, and reads as an off-by-one worth "tidying" if nothing fails when
// it goes.
func TestVersionGivesUpAfterParseAttempts(t *testing.T) {
	t.Parallel()

	// Names are scanned in sorted order, so the good file's position is
	// controlled by its name.
	within := t.TempDir()
	write(t, within, "ssg-a1-ds.xml", "not xml")
	write(t, within, "ssg-a2-ds.xml", "not xml")
	write(t, within, "ssg-a3-ds.xml", datastream("0.1.81"))
	if got := Version(within); got != "0.1.81" {
		t.Errorf("good file at position %d: Version() = %q, want %q", parseAttempts, got, "0.1.81")
	}

	beyond := t.TempDir()
	write(t, beyond, "ssg-a1-ds.xml", "not xml")
	write(t, beyond, "ssg-a2-ds.xml", "not xml")
	write(t, beyond, "ssg-a3-ds.xml", "not xml")
	write(t, beyond, "ssg-a4-ds.xml", datastream("0.1.81"))
	if got := Version(beyond); got != "" {
		t.Errorf("good file at position %d: Version() = %q, want empty", parseAttempts+1, got)
	}
}

// datastreamPadded pushes the version element past a chosen byte offset by
// growing the description that precedes it.
func datastreamPadded(version string, filler int) string {
	return strings.Replace(
		datastream(version),
		`<xccdf-1.2:platform idref="cpe:/o:debian:debian_linux:13"/>`,
		`<xccdf-1.2:description>`+strings.Repeat("x", filler)+`</xccdf-1.2:description>`,
		1,
	)
}

// parseLimit is the bound most likely to bite silently, if a future SSG release
// grows its benchmark front matter. Assert both sides of it so a regression
// shows up as a failure rather than as content that stops being served.
func TestVersionBoundedByParseLimit(t *testing.T) {
	t.Parallel()

	within := t.TempDir()
	write(t, within, "ssg-debian13-ds.xml", datastreamPadded("0.1.81", 64<<10))
	if got := Version(within); got != "0.1.81" {
		t.Errorf("version within parseLimit: Version() = %q, want %q", got, "0.1.81")
	}

	beyond := t.TempDir()
	write(t, beyond, "ssg-debian13-ds.xml", datastreamPadded("0.1.81", parseLimit+(1<<20)))
	if got := Version(beyond); got != "" {
		t.Errorf("version beyond parseLimit: Version() = %q, want empty", got)
	}
}

// Why versionFromDatastream decodes leniently. The benchmark description, notice
// and front matter all precede the version in XCCDF document order and carry
// XHTML, so a strict decode that errors on an undefined entity aborts before it
// ever reaches the version. Set dec.Strict back to true and this fails.
func TestVersionToleratesUndefinedEntitiesBeforeVersion(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	body := strings.Replace(
		datastream("0.1.81"),
		`<xccdf-1.2:platform idref="cpe:/o:debian:debian_linux:13"/>`,
		`<xccdf-1.2:description>hardening&nbsp;guide &amp; notes</xccdf-1.2:description>`,
		1,
	)
	write(t, dir, "ssg-debian13-ds.xml", body)

	if got := Version(dir); got != "0.1.81" {
		t.Errorf("Version() = %q, want %q", got, "0.1.81")
	}
}

// The version reaches agents, which compare it and write it to their own marker,
// so a malformed datastream must not be able to inject arbitrary text into it.
func TestVersionRejectsMalformedVersionText(t *testing.T) {
	t.Parallel()

	bad := []string{
		"nightly",
		"0.1.81; rm -rf /",
		"",
		"v0.1.81",
		"../../etc/passwd",
		// Both of these clear an unbounded `\d+`. The update-check query casts
		// each dot-separated component numerically, and int4 stops at ten digits,
		// so eleven is already enough to matter: the length backstop alone would
		// have let this through, since 13 characters is well under maxVersionLen.
		"99999999999.1.81",
		strings.Repeat("9", 4096) + ".1.81",
	}
	for _, v := range bad {
		dir := t.TempDir()
		write(t, dir, "ssg-debian13-ds.xml", datastream(v))
		if got := Version(dir); got != "" {
			t.Errorf("Version() with version %q = %q, want empty", truncate(v), got)
		}
	}
}

// The marker keeps its format freedom, since our own build writes it and
// tightening the format could refuse an existing install for no gain. It does
// not keep unbounded length, which is the only part callers depend on.
func TestMarkerIsLengthBoundedButNotFormatChecked(t *testing.T) {
	t.Parallel()

	odd := t.TempDir()
	write(t, odd, markerFile, "0.1.81-custom\n")
	if got := Version(odd); got != "0.1.81-custom" {
		t.Errorf("Version() = %q, want the marker verbatim", got)
	}

	oversized := t.TempDir()
	write(t, oversized, markerFile, strings.Repeat("9", maxVersionLen+1))
	write(t, oversized, "ssg-debian13-ds.xml", datastream("0.1.81"))
	if got := Version(oversized); got != "0.1.81" {
		t.Errorf("Version() = %q, want the oversized marker ignored and the content used", got)
	}
}

// nestedDatastream puts the version element at a chosen nesting depth.
func nestedDatastream(depth int) string {
	return `<?xml version="1.0"?>
<ds:data-stream-collection xmlns:ds="http://scap.nist.gov/schema/scap/source/1.2" xmlns:xccdf-1.2="http://checklists.nist.gov/xccdf/1.2">` +
		strings.Repeat("<a>", depth-2) +
		`<xccdf-1.2:version>0.1.81</xccdf-1.2:version>` +
		strings.Repeat("</a>", depth-2) +
		`</ds:data-stream-collection>`
}

// parseLimit caps the bytes read but not the element stack they retain, so depth
// is bounded separately. Real datastreams measure 11 to 14 deep with the version
// at depth 4, so maxDepth has room to spare; assert both sides so shrinking it
// to something that breaks real content fails here.
func TestVersionBoundedByMaxDepth(t *testing.T) {
	t.Parallel()

	within := t.TempDir()
	write(t, within, "ssg-debian13-ds.xml", nestedDatastream(maxDepth))
	if got := Version(within); got != "0.1.81" {
		t.Errorf("version at depth %d: Version() = %q, want %q", maxDepth, got, "0.1.81")
	}

	beyond := t.TempDir()
	write(t, beyond, "ssg-debian13-ds.xml", nestedDatastream(maxDepth+1))
	if got := Version(beyond); got != "" {
		t.Errorf("version at depth %d: Version() = %q, want empty", maxDepth+1, got)
	}
}

func truncate(s string) string {
	if len(s) > 40 {
		return s[:40] + "..."
	}
	return s
}

// Reading datastreams means parsing large third-party XML in the server
// process, so pin the properties that make that safe. Go's encoding/xml does not
// expand DTD-declared entities and does not resolve external ones, and none of
// this changes with dec.Strict = false. Verified empirically, not assumed: swap
// in an XML library that does resolve entities and these fail.
func TestVersionIsNotVulnerableToEntityAttacks(t *testing.T) {
	t.Parallel()

	secretDir := t.TempDir()
	secret := filepath.Join(secretDir, "secret.txt")
	if err := os.WriteFile(secret, []byte("TOPSECRET"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		body string
		want string
	}{
		{
			// The version is a declared entity that never expands, so it fails
			// versionRe rather than blowing up the heap.
			name: "entity expansion does not blow up",
			body: `<?xml version="1.0"?>
<!DOCTYPE d [
 <!ENTITY a "aaaaaaaaaa">
 <!ENTITY b "&a;&a;&a;&a;&a;&a;&a;&a;&a;&a;">
 <!ENTITY c "&b;&b;&b;&b;&b;&b;&b;&b;&b;&b;">
 <!ENTITY d "&c;&c;&c;&c;&c;&c;&c;&c;&c;&c;">
]>
<ds:data-stream-collection xmlns:ds="http://scap.nist.gov/schema/scap/source/1.2" xmlns:xccdf-1.2="http://checklists.nist.gov/xccdf/1.2">
  <xccdf-1.2:description>&d;</xccdf-1.2:description>
  <xccdf-1.2:version>&d;</xccdf-1.2:version>
</ds:data-stream-collection>`,
			want: "",
		},
		{
			name: "external file entity is not resolved",
			body: `<?xml version="1.0"?>
<!DOCTYPE r [ <!ENTITY xxe SYSTEM "file://` + secret + `"> ]>
<ds:data-stream-collection xmlns:ds="http://scap.nist.gov/schema/scap/source/1.2" xmlns:xccdf-1.2="http://checklists.nist.gov/xccdf/1.2">
  <xccdf-1.2:version>&xxe;</xccdf-1.2:version>
</ds:data-stream-collection>`,
			want: "",
		},
		{
			name: "external http entity is not fetched",
			body: `<?xml version="1.0"?>
<!DOCTYPE r [ <!ENTITY xxe SYSTEM "http://127.0.0.1:1/pwn"> ]>
<ds:data-stream-collection xmlns:ds="http://scap.nist.gov/schema/scap/source/1.2" xmlns:xccdf-1.2="http://checklists.nist.gov/xccdf/1.2">
  <xccdf-1.2:version>&xxe;</xccdf-1.2:version>
</ds:data-stream-collection>`,
			want: "",
		},
		{
			// Bounded by maxDepth, so it stops after 64 elements rather than
			// retaining a 4MB auto-close stack. See TestVersionBoundedByMaxDepth.
			name: "deep nesting terminates",
			body: `<?xml version="1.0"?>` + strings.Repeat("<a>", 100000) + strings.Repeat("</a>", 100000),
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			write(t, dir, "ssg-hostile-ds.xml", tt.body)

			got := Version(dir)
			if got != tt.want {
				t.Errorf("Version() = %q, want %q", got, tt.want)
			}
			if strings.Contains(got, "TOPSECRET") {
				t.Errorf("Version() leaked local file contents: %q", got)
			}
		})
	}
}

func TestFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	for _, name := range []string{"ssg-rhel9-ds.xml", "ssg-debian13-ds.xml", "README.md", "ssg-bogus.xml"} {
		write(t, dir, name, "x")
	}
	if err := os.Mkdir(filepath.Join(dir, "ssg-adir-ds.xml"), 0o750); err != nil {
		t.Fatal(err)
	}

	want := []string{"ssg-debian13-ds.xml", "ssg-rhel9-ds.xml"}
	if got := Files(dir); !reflect.DeepEqual(got, want) {
		t.Errorf("Files() = %v, want %v", got, want)
	}
	if got := Files(""); got != nil {
		t.Errorf("Files(\"\") = %v, want nil", got)
	}
}
