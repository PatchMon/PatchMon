package handler

import (
	"net/http"
	"strconv"

	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/go-chi/chi/v5"
)

// PackagesHandler handles packages routes.
type PackagesHandler struct {
	packages *store.PackagesStore
}

// NewPackagesHandler creates a new packages handler.
func NewPackagesHandler(packages *store.PackagesStore) *PackagesHandler {
	return &PackagesHandler{packages: packages}
}

// List handles GET /packages.
func (h *PackagesHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page <= 0 {
		page = 1
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	if limit > 10000 {
		limit = 10000
	}
	search := q.Get("search")
	if len(search) > 200 {
		search = search[:200]
	}
	params := store.ListParams{
		Page:             page,
		Limit:            limit,
		Search:           search,
		Category:         q.Get("category"),
		NeedsUpdate:      q.Get("needsUpdate"),
		IsSecurityUpdate: q.Get("isSecurityUpdate"),
		Host:             q.Get("host"),
		Repository:       q.Get("repository"),
	}
	pkgs, total, err := h.packages.List(r.Context(), params)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to load packages")
		return
	}
	pages := (total + params.Limit - 1) / params.Limit
	if pages < 1 {
		pages = 1
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"packages": pkgs,
		"pagination": map[string]interface{}{
			"page":  params.Page,
			"limit": params.Limit,
			"total": total,
			"pages": pages,
		},
	})
}

// GetByID handles GET /packages/:packageId.
func (h *PackagesHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	packageID := chi.URLParam(r, "packageId")
	if packageID == "" {
		Error(w, http.StatusBadRequest, "packageId is required")
		return
	}
	pkg, err := h.packages.GetByID(r.Context(), packageID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to fetch package details")
		return
	}
	if pkg == nil {
		Error(w, http.StatusNotFound, "Package not found")
		return
	}
	JSON(w, http.StatusOK, pkg)
}

// GetHosts handles GET /packages/:packageId/hosts.
func (h *PackagesHandler) GetHosts(w http.ResponseWriter, r *http.Request) {
	packageID := chi.URLParam(r, "packageId")
	if packageID == "" {
		Error(w, http.StatusBadRequest, "packageId is required")
		return
	}
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page <= 0 {
		page = 1
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 25
	}
	if limit > 500 {
		limit = 500
	}
	search := q.Get("search")
	if len(search) > 200 {
		search = search[:200]
	}
	params := store.GetHostsParams{
		Page:   page,
		Limit:  limit,
		Search: search,
	}
	if v := q.Get("needsUpdate"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			params.NeedsUpdate = &b
		}
	}
	hosts, total, err := h.packages.GetHosts(r.Context(), packageID, params)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to fetch package hosts")
		return
	}
	pages := (total + params.Limit - 1) / params.Limit
	if pages < 1 {
		pages = 1
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"hosts": hosts,
		"pagination": map[string]interface{}{
			"page":  params.Page,
			"limit": params.Limit,
			"total": total,
			"pages": pages,
		},
	})
}

// GetActivity handles GET /packages/:packageId/activity.
func (h *PackagesHandler) GetActivity(w http.ResponseWriter, r *http.Request) {
	packageID := chi.URLParam(r, "packageId")
	if packageID == "" {
		Error(w, http.StatusBadRequest, "packageId is required")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}
	activities, err := h.packages.GetActivity(r.Context(), packageID, limit, offset)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to fetch package activity")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{"activities": activities})
}

// GetCategories handles GET /packages/categories/list.
func (h *PackagesHandler) GetCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := h.packages.GetCategories(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to fetch categories")
		return
	}
	JSON(w, http.StatusOK, cats)
}
