package handler

import (
	_ "embed"
	"net/http"

	"github.com/go-chi/chi/v5"
)

//go:embed release_notes_data/RELEASE_NOTES_1.4.5.md
var releaseNotes1_4_5 string

//go:embed release_notes_data/RELEASE_NOTES_1.4.2.md
var releaseNotes1_4_2 string

//go:embed release_notes_data/RELEASE_NOTES_1.4.1.md
var releaseNotes1_4_1 string

//go:embed release_notes_data/RELEASE_NOTES_1.4.0.md
var releaseNotes1_4_0 string

//go:embed release_notes_data/RELEASE_NOTES_1.3.7.md
var releaseNotes1_3_7 string

// releaseNotesContent maps version to markdown content (embedded at build time).
var releaseNotesContent = map[string]string{
	"1.3.7": releaseNotes1_3_7,
	"1.4.0": releaseNotes1_4_0,
	"1.4.1": releaseNotes1_4_1,
	"1.4.2": releaseNotes1_4_2,
	"1.4.5": releaseNotes1_4_5,
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
