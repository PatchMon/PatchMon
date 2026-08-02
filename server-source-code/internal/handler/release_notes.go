package handler

import (
	"embed"
	"net/http"
	"path"
	"regexp"

	"github.com/go-chi/chi/v5"
)

// Every RELEASE_NOTES_<semver>.md in release_notes_data is embedded at build
// time, so the "What's New" modal works without network access. The files are
// generated from the GitHub draft release by .github/workflows/release.yml;
// shipping a release adds one file and needs no Go change.
//
//go:embed release_notes_data/*.md
var releaseNotesFS embed.FS

const releaseNotesDir = "release_notes_data"

var releaseNotesFileRe = regexp.MustCompile(`^RELEASE_NOTES_(\d+\.\d+\.\d+)\.md$`)

// releaseNotesContent maps version to markdown content.
var releaseNotesContent = loadReleaseNotes()

func loadReleaseNotes() map[string]string {
	entries, err := releaseNotesFS.ReadDir(releaseNotesDir)
	if err != nil {
		return map[string]string{}
	}
	notes := make(map[string]string, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := releaseNotesFileRe.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		content, err := releaseNotesFS.ReadFile(path.Join(releaseNotesDir, entry.Name()))
		if err != nil {
			continue
		}
		notes[match[1]] = string(content)
	}
	return notes
}

// ReleaseNotesHandler serves release notes embedded in the binary.
type ReleaseNotesHandler struct{}

// NewReleaseNotesHandler creates a handler for release notes.
func NewReleaseNotesHandler() *ReleaseNotesHandler {
	return &ReleaseNotesHandler{}
}

// GetByVersion handles GET /api/v1/release-notes/{version}.
// Returns JSON: { version, content, exists }.
func (h *ReleaseNotesHandler) GetByVersion(w http.ResponseWriter, r *http.Request) {
	version := chi.URLParam(r, "version")
	if version == "" {
		JSON(w, http.StatusBadRequest, map[string]interface{}{
			"version": "",
			"content": nil,
			"exists":  false,
		})
		return
	}

	content, exists := releaseNotesContent[version]
	JSON(w, http.StatusOK, map[string]interface{}{
		"version": version,
		"content": content,
		"exists":  exists,
	})
}
