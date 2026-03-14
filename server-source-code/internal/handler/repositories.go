package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/go-chi/chi/v5"
)

// optionalInt unmarshals from JSON number or empty string.
type optionalInt struct{ val *int }

func (o *optionalInt) UnmarshalJSON(data []byte) error {
	s := string(data)
	if s == "null" || s == `""` || s == "" {
		o.val = nil
		return nil
	}
	var i int
	if err := json.Unmarshal(data, &i); err != nil {
		// Try string for form values like ""
		var str string
		if err2 := json.Unmarshal(data, &str); err2 != nil {
			return err
		}
		if str == "" {
			o.val = nil
			return nil
		}
		i, err = strconv.Atoi(str)
		if err != nil {
			return err
		}
	}
	o.val = &i
	return nil
}

// RepositoriesHandler handles repository routes.
type RepositoriesHandler struct {
	repos *store.RepositoriesStore
}

// NewRepositoriesHandler creates a new repositories handler.
func NewRepositoriesHandler(repos *store.RepositoriesStore) *RepositoriesHandler {
	return &RepositoriesHandler{repos: repos}
}

// List handles GET /repositories.
func (h *RepositoriesHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	search := q.Get("search")
	if len(search) > 200 {
		search = search[:200]
	}
	params := store.RepoListParams{
		HostID: q.Get("host"),
		Search: search,
		Status: q.Get("status"),
		Type:   q.Get("type"),
	}
	repos, err := h.repos.List(r.Context(), params)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to fetch repositories")
		return
	}
	JSON(w, http.StatusOK, repos)
}

// GetByHost handles GET /repositories/host/:hostId.
func (h *RepositoriesHandler) GetByHost(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "hostId")
	if hostID == "" {
		Error(w, http.StatusBadRequest, "hostId is required")
		return
	}
	repos, err := h.repos.GetByHost(r.Context(), hostID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to fetch host repositories")
		return
	}
	JSON(w, http.StatusOK, repos)
}

// GetByID handles GET /repositories/:repositoryId.
func (h *RepositoriesHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repositoryId")
	if repoID == "" {
		Error(w, http.StatusBadRequest, "repositoryId is required")
		return
	}
	repo, err := h.repos.GetByID(r.Context(), repoID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to fetch repository details")
		return
	}
	if repo == nil {
		Error(w, http.StatusNotFound, "Repository not found")
		return
	}
	JSON(w, http.StatusOK, repo)
}

// Update handles PUT /repositories/:repositoryId.
func (h *RepositoriesHandler) Update(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repositoryId")
	if repoID == "" {
		Error(w, http.StatusBadRequest, "repositoryId is required")
		return
	}
	var body struct {
		Name          *string     `json:"name"`
		Description   *string     `json:"description"`
		IsActive      *bool       `json:"isActive"`
		IsActiveSnake *bool       `json:"is_active"`
		Priority      optionalInt `json:"priority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if body.Name != nil && *body.Name == "" {
		Error(w, http.StatusBadRequest, "Name is required")
		return
	}
	isActive := body.IsActive
	if isActive == nil {
		isActive = body.IsActiveSnake
	}
	repo, err := h.repos.Update(r.Context(), repoID, body.Name, body.Description, isActive, body.Priority.val)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to update repository")
		return
	}
	JSON(w, http.StatusOK, repo)
}

// ToggleHostRepository handles PATCH /repositories/host/:hostId/repository/:repositoryId.
func (h *RepositoriesHandler) ToggleHostRepository(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "hostId")
	repoID := chi.URLParam(r, "repositoryId")
	if hostID == "" || repoID == "" {
		Error(w, http.StatusBadRequest, "hostId and repositoryId are required")
		return
	}
	var body struct {
		IsEnabled bool `json:"isEnabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	hr, err := h.repos.ToggleHostRepository(r.Context(), hostID, repoID, body.IsEnabled)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to toggle repository status")
		return
	}
	if hr == nil {
		Error(w, http.StatusNotFound, "Host repository not found")
		return
	}
	status := "disabled"
	if body.IsEnabled {
		status = "enabled"
	}
	hostName := hr.Hosts.FriendlyName
	if hostName == "" {
		hostName = "host"
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"message":        "Repository " + status + " for host " + hostName,
		"hostRepository": hr,
	})
}

// GetStats handles GET /repositories/stats/summary.
func (h *RepositoriesHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.repos.GetStats(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to fetch repository statistics")
		return
	}
	JSON(w, http.StatusOK, stats)
}

// Delete handles DELETE /repositories/:repositoryId.
func (h *RepositoriesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	repoID := chi.URLParam(r, "repositoryId")
	if repoID == "" {
		Error(w, http.StatusBadRequest, "repositoryId is required")
		return
	}
	deleted, err := h.repos.Delete(r.Context(), repoID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to delete repository")
		return
	}
	if deleted == nil {
		Error(w, http.StatusNotFound, "Repository not found")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"message":           "Repository deleted successfully",
		"deletedRepository": deleted,
	})
}

// CleanupOrphaned handles DELETE /repositories/cleanup/orphaned.
func (h *RepositoriesHandler) CleanupOrphaned(w http.ResponseWriter, r *http.Request) {
	deleted, count, err := h.repos.CleanupOrphaned(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to cleanup orphaned repositories")
		return
	}
	if count == 0 {
		JSON(w, http.StatusOK, map[string]interface{}{
			"message":             "No orphaned repositories found",
			"deletedCount":        0,
			"deletedRepositories": []interface{}{},
		})
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"message":             fmt.Sprintf("Successfully deleted %d orphaned repositories", count),
		"deletedCount":        count,
		"deletedRepositories": deleted,
	})
}
