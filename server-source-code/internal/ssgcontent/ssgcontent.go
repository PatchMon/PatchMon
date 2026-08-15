// Package ssgcontent resolves the SCAP Security Guide content a server holds on
// disk: which datastream files are present, and which SSG release they came
// from. Both the compliance handlers and the ssg-update-check worker read the
// same directory and must agree on the answer.
package ssgcontent

import (
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// FilenameRe validates SSG datastream filenames to prevent path traversal.
var FilenameRe = regexp.MustCompile(`^ssg-[a-z0-9]+-ds\.xml$`)

// versionRe bounds what a derived version may look like, so a malformed
// datastream cannot put arbitrary text into the version agents compare against.
// Digits are counted, not just matched: `\d+` would accept a component of any
// size, and versions are compared numerically downstream.
var versionRe = regexp.MustCompile(`^\d{1,5}(\.\d{1,5}){1,2}$`)

const markerFile = ".ssg-version"

// parseLimit bounds how far into a datastream the benchmark version is looked
// for. It lives in the first component: 208KB in for ssg-rhel9, the largest
// datastream in SSG 0.1.81 at 28MB on disk.
const parseLimit = 4 << 20

// parseAttempts bounds how many datastreams are tried before giving up. Every
// datastream in a release carries the same version, so the first normally
// answers; the rest are there in case it is truncated or unreadable.
const parseAttempts = 3

// maxDepth bounds the decoder's element stack. parseLimit caps the bytes read
// but not what reading them retains: 4MB of `<a><a><a>` holds an auto-close
// stack peaking around 140MB of live heap, against the 256MiB GOMEMLIMIT the
// image sets. Since Version also runs at startup, that is reachable before the
// listener is up, where an OOM becomes a restart loop rather than one failed
// request. Real datastreams nest 11 to 14 deep and put the version at depth 4.
const maxDepth = 64

// maxVersionLen bounds the version string. Nothing legitimate comes close, and
// being short is the only property callers depend on. It is a backstop, not the
// defence against oversized numeric components: that lives in versionRe for the
// derived path, and in the update-check query's numeric casts for every path,
// including the host-reported version this package never sees.
const maxVersionLen = 32

// Files returns the datastream filenames in dir, sorted by name.
func Files(dir string) []string {
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && FilenameRe.MatchString(e.Name()) {
			files = append(files, e.Name())
		}
	}
	return files
}

// Version returns the SSG release held in dir, or "" when dir holds no usable
// content.
//
// The Docker build writes a .ssg-version marker and nothing else does, so the
// marker is the fast path but cannot be the only one. The upstream
// ComplianceAsCode release tarball carries its version solely in the top-level
// directory name, which extraction strips, so any install populating the
// directory from upstream has perfectly good datastreams and no marker. Falling
// back to the content itself keeps those installs working instead of reporting
// the whole directory as absent.
func Version(dir string) string {
	if dir == "" {
		return ""
	}
	if v := readMarker(dir); v != "" {
		return v
	}
	files := Files(dir)
	if len(files) > parseAttempts {
		files = files[:parseAttempts]
	}
	for _, name := range files {
		if v := versionFromDatastream(filepath.Join(dir, name)); v != "" {
			return v
		}
	}
	return ""
}

// readMarker deliberately does not apply versionRe. The marker is written by our
// own build from a build arg, and imposing a format on it now could refuse an
// existing install whose marker the regex happens to reject, for no gain. The
// length bound is a different matter: no legitimate marker is anywhere near it,
// and it is the only property the callers actually depend on. Note the
// consequence, since nothing downstream may assume otherwise: the version this
// package reports is NOT guaranteed to match versionRe, only to be short.
func readMarker(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, markerFile))
	if err != nil {
		return ""
	}
	v := strings.TrimSpace(string(data))
	if len(v) > maxVersionLen {
		return ""
	}
	return v
}

// versionFromDatastream reads the SSG release out of a datastream's XCCDF
// benchmark header:
//
//	<xccdf-1.2:version update="...">0.1.81</xccdf-1.2:version>
func versionFromDatastream(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	dec := xml.NewDecoder(io.LimitReader(f, parseLimit))
	// The benchmark description, notice and front matter all precede the version
	// in XCCDF document order and carry XHTML, so a strict decode that errors on
	// an undefined entity aborts before ever reaching the version. Leniency can
	// only cost a missed match, never a wrong one: the result still has to clear
	// versionRe below. Covered by TestVersionToleratesUndefinedEntitiesBeforeVersion.
	dec.Strict = false

	depth := 0
	for {
		tok, err := dec.Token()
		if err != nil {
			return ""
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			if _, closing := tok.(xml.EndElement); closing {
				depth--
			}
			continue
		}
		depth++
		if depth > maxDepth {
			return ""
		}
		if start.Name.Local != "version" || !strings.Contains(start.Name.Space, "xccdf") {
			continue
		}
		var v string
		if err := dec.DecodeElement(&v, &start); err != nil {
			return ""
		}
		v = strings.TrimSpace(v)
		if len(v) <= maxVersionLen && versionRe.MatchString(v) {
			return v
		}
		// Give up on this file rather than scanning on. XCCDF document order puts
		// Benchmark/version ahead of the Rule and Group version elements, so the
		// first match is the release; a later match would be some rule's own
		// version, which is a wrong answer rather than a second chance.
		//
		// Returning here is also what keeps depth honest: DecodeElement consumed
		// through the matching EndElement without this loop seeing it, so the
		// counter is stale from this point on. Turning this into a continue would
		// silently under-count and weaken maxDepth.
		return ""
	}
}
