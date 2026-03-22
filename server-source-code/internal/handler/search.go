package handler

import (
	"net/http"

	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
)

// SearchHandler handles global search routes.
type SearchHandler struct {
	search *store.SearchStore
}

// NewSearchHandler creates a new search handler.
func NewSearchHandler(search *store.SearchStore) *SearchHandler {
	return &SearchHandler{search: search}
}

// HandleGlobalSearch handles GET /search?q=...
func (h *SearchHandler) HandleGlobalSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		Error(w, http.StatusBadRequest, "Search query parameter 'q' is required")
		return
	}
	if len(q) > 200 {
		q = q[:200]
	}

	limit := parseIntQuery(r, "limit", 20)
	if limit > 100 {
		limit = 100
	}

	results, err := h.search.GlobalSearch(r.Context(), q, limit)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to perform search")
		return
	}

	JSON(w, http.StatusOK, results)
}
