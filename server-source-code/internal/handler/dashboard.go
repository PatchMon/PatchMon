package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/cve"
	"github.com/PatchMon/PatchMon/server-source-code/internal/cve/distro"
	"github.com/PatchMon/PatchMon/server-source-code/internal/queue"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/PatchMon/PatchMon/server-source-code/internal/util"
	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
)

// DashboardHandler handles dashboard routes.
type DashboardHandler struct {
	dashboard  *store.DashboardStore
	hosts      *store.HostsStore
	packages   *store.PackagesStore
	users      *store.UsersStore
	docker     *store.DockerStore
	inspector  *asynq.Inspector
	cve        *cve.Service
	distroEval *distro.Evaluator
}

// NewDashboardHandler creates a new dashboard handler.
func NewDashboardHandler(dashboard *store.DashboardStore, hosts *store.HostsStore, packages *store.PackagesStore, users *store.UsersStore, docker *store.DockerStore, inspector *asynq.Inspector, cveService *cve.Service, distroEval *distro.Evaluator) *DashboardHandler {
	return &DashboardHandler{dashboard: dashboard, hosts: hosts, packages: packages, users: users, docker: docker, inspector: inspector, cve: cveService, distroEval: distroEval}
}

// Stats handles GET /dashboard/stats.
func (h *DashboardHandler) Stats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.dashboard.GetStats(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to load dashboard stats")
		return
	}
	JSON(w, http.StatusOK, stats)
}

// Hosts handles GET /dashboard/hosts.
func (h *DashboardHandler) Hosts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	search := q.Get("search")
	if len(search) > 200 {
		search = search[:200]
	}
	params := store.HostsListParams{
		Search:    search,
		Group:     q.Get("group"),
		Status:    q.Get("status"),
		OS:        q.Get("os"),
		OSVersion: q.Get("osVersion"),
	}

	// Optional kernel-version filter. The value is either a version expression
	// (e.g. "<6.18.36", "5.15..6.1", "6.8") or a CVE identifier
	// (e.g. "CVE-2026-46331"), which is resolved to affected kernel ranges.
	if kernel := strings.TrimSpace(q.Get("kernel")); kernel != "" {
		filter, status, msg := h.resolveKernelFilter(r.Context(), kernel)
		if filter == nil {
			Error(w, status, msg)
			return
		}
		params.Kernel = filter
	}

	hosts, err := h.dashboard.GetHostsWithCounts(r.Context(), params)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to load hosts")
		return
	}

	// Distro-aware CVE evaluation: annotate every host with its per-distro
	// status (vulnerable/patched/not_affected/unknown) so nothing is silently
	// hidden. The frontend surfaces the status and lets the user focus on the
	// vulnerable ones.
	if cveID := strings.TrimSpace(q.Get("cve")); cveID != "" {
		if !cve.IsCVEID(cveID) {
			Error(w, http.StatusBadRequest, "Invalid CVE identifier")
			return
		}
		if err := h.annotateCVEStatus(r.Context(), hosts, cveID); err != nil {
			Error(w, http.StatusServiceUnavailable, err.Error())
			return
		}
	}

	JSON(w, http.StatusOK, hosts)
}

// annotateCVEStatus evaluates each host's per-distro status for the CVE and
// annotates it in place with cve_status/cve_fixed_version/cve_release/
// cve_distro/cve_kernel_pkg.
func (h *DashboardHandler) annotateCVEStatus(ctx context.Context, hosts []map[string]interface{}, cveID string) error {
	if h.distroEval == nil {
		return fmt.Errorf("distro CVE evaluation is not available")
	}
	ids := make([]string, 0, len(hosts))
	for _, hm := range hosts {
		if id, ok := hm["id"].(string); ok {
			ids = append(ids, id)
		}
	}
	kpkgs, err := h.dashboard.GetKernelPackagesForHosts(ctx, ids)
	if err != nil {
		return fmt.Errorf("failed to load kernel packages")
	}

	for _, hm := range hosts {
		id, _ := hm["id"].(string)
		var pkgs []distro.KernelPackage
		for _, p := range kpkgs[id] {
			pkgs = append(pkgs, distro.KernelPackage{Name: p.Name, Version: p.Version})
		}
		osType := asString(hm["os_type"])
		uname := asString(hm["kernel_version"])
		sel := distro.SelectRunningKernel(osType, uname, pkgs)
		res := h.distroEval.Evaluate(ctx, cveID, distro.Host{
			OSType:           osType,
			OSVersion:        asString(hm["os_version"]),
			KernelVersion:    uname,
			KernelPkgName:    sel.Name,
			KernelPkgVersion: sel.Version,
			InstalledKernels: pkgs,
		})
		status := string(res.Status)
		if res.Status == distro.StatusUnknown {
			if v := h.nvdVerdict(ctx, cveID, uname); v != "" {
				status = v
				hm["cve_note"] = "resolved via NVD upstream ranges"
			}
		}
		hm["cve_status"] = status
		hm["cve_fixed_version"] = res.FixedVersion
		hm["cve_release"] = res.Release
		hm["cve_distro"] = res.Distro
		hm["cve_kernel_pkg"] = sel.Version
		hm["cve_reboot_required"] = res.RebootRequired
	}
	return nil
}

// nvdVerdict resolves a host whose distribution has no verdict (unknown /
// won't-fix "ignored") using the CVE's upstream NVD version ranges: "vulnerable"
// if the running kernel falls in an affected range, "not_affected" if it sits
// outside every range (below, above, or in a gap — those versions simply aren't
// affected upstream), or "" when NVD has no data. Only applied to unknown hosts,
// so it never overrides an authoritative distro verdict (a real backported fix
// shows up as released/not-affected, not unknown).
func (h *DashboardHandler) nvdVerdict(ctx context.Context, cveID, kernelVersion string) string {
	if h.cve == nil || strings.TrimSpace(kernelVersion) == "" {
		return ""
	}
	res, err := h.cve.Lookup(ctx, cveID)
	if err != nil || res == nil || res.Filter == nil || len(res.Ranges) == 0 {
		return ""
	}
	if res.Filter.Matches(kernelVersion) {
		return "vulnerable"
	}
	return "not_affected"
}

// asString reads a string or *string map value, returning "" for anything else.
func asString(v interface{}) string {
	switch s := v.(type) {
	case string:
		return s
	case *string:
		if s != nil {
			return *s
		}
	}
	return ""
}

// parseCVEList splits a comma/space/semicolon/newline-separated list into
// unique, validated, upper-cased CVE ids (capped to avoid abuse).
func parseCVEList(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	seen := map[string]bool{}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		id := strings.ToUpper(strings.TrimSpace(p))
		if !cve.IsCVEID(id) || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
		if len(out) >= 100 {
			break
		}
	}
	return out
}

// CVEReport handles GET /dashboard/cve-report?cves=CVE-...,CVE-... . For each
// CVE it returns the hosts their distribution reports vulnerable, plus a status
// breakdown, evaluating every host against its distro's fixed kernel version.
func (h *DashboardHandler) CVEReport(w http.ResponseWriter, r *http.Request) {
	if h.distroEval == nil {
		Error(w, http.StatusServiceUnavailable, "Distro CVE evaluation is not available")
		return
	}
	cves := parseCVEList(r.URL.Query().Get("cves"))
	if len(cves) == 0 {
		Error(w, http.StatusBadRequest, "Provide one or more CVE identifiers in the 'cves' parameter")
		return
	}

	ctx := r.Context()
	hosts, err := h.dashboard.GetHostsWithCounts(ctx, store.HostsListParams{})
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to load hosts")
		return
	}
	ids := make([]string, 0, len(hosts))
	for _, hm := range hosts {
		if id, ok := hm["id"].(string); ok {
			ids = append(ids, id)
		}
	}
	kpkgs, err := h.dashboard.GetKernelPackagesForHosts(ctx, ids)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to load kernel packages")
		return
	}

	// Precompute the evaluator input for every host once.
	type hostEntry struct {
		dh      distro.Host
		summary map[string]interface{}
	}
	entries := make([]hostEntry, 0, len(hosts))
	for _, hm := range hosts {
		id, _ := hm["id"].(string)
		var pkgs []distro.KernelPackage
		for _, p := range kpkgs[id] {
			pkgs = append(pkgs, distro.KernelPackage{Name: p.Name, Version: p.Version})
		}
		osType := asString(hm["os_type"])
		uname := asString(hm["kernel_version"])
		sel := distro.SelectRunningKernel(osType, uname, pkgs)
		entries = append(entries, hostEntry{
			dh: distro.Host{
				OSType:           osType,
				OSVersion:        asString(hm["os_version"]),
				KernelVersion:    uname,
				KernelPkgName:    sel.Name,
				KernelPkgVersion: sel.Version,
				InstalledKernels: pkgs,
			},
			summary: hm,
		})
	}

	results := make([]map[string]interface{}, 0, len(cves))
	for _, cveID := range cves {
		counts := map[string]int{"vulnerable": 0, "patched": 0, "not_affected": 0, "unknown": 0}
		vulnHosts := make([]map[string]interface{}, 0)
		for _, e := range entries {
			res := h.distroEval.Evaluate(ctx, cveID, e.dh)
			status := res.Status
			if status == distro.StatusUnknown {
				switch h.nvdVerdict(ctx, cveID, e.dh.KernelVersion) {
				case "vulnerable":
					status = distro.StatusVulnerable
				case "not_affected":
					status = distro.StatusNotAffected
				}
			}
			counts[string(status)]++
			if status == distro.StatusVulnerable {
				vulnHosts = append(vulnHosts, map[string]interface{}{
					"id":              e.summary["id"],
					"friendly_name":   e.summary["friendly_name"],
					"hostname":        e.summary["hostname"],
					"os_type":         e.summary["os_type"],
					"os_version":      e.summary["os_version"],
					"kernel_version":  e.summary["kernel_version"],
					"fixed_version":   res.FixedVersion,
					"distro":          res.Distro,
					"release":         res.Release,
					"reboot_required": res.RebootRequired,
				})
			}
		}
		entry := map[string]interface{}{
			"cve_id": cveID,
			"counts": counts,
			"hosts":  vulnHosts,
		}
		// Enrich with NVD metadata (cached) — severity/score/description/ranges.
		if h.cve != nil {
			if nv, err := h.cve.Lookup(ctx, cveID); err == nil && nv != nil {
				entry["nvd"] = map[string]interface{}{
					"description":   nv.Description,
					"cvss_score":    nv.CVSSScore,
					"cvss_severity": nv.CVSSSeverity,
					"cvss_vector":   nv.CVSSVector,
					"published":     nv.Published,
					"last_modified": nv.LastModified,
					"references":    nv.References,
					"weaknesses":    nv.Weaknesses,
					"labels":        nv.Labels,
					"ranges":        nv.Ranges,
				}
			}
		}
		results = append(results, entry)
	}

	JSON(w, http.StatusOK, map[string]interface{}{"cves": cves, "results": results})
}

// CVEDataSources handles GET /dashboard/cve/sources — freshness of the CVE
// databases: per source, the last update attempt/success, whether it succeeded,
// how many CVEs are held and the newest CVE known.
func (h *DashboardHandler) CVEDataSources(w http.ResponseWriter, r *http.Request) {
	if h.distroEval == nil {
		Error(w, http.StatusServiceUnavailable, "Distro CVE evaluation is not available")
		return
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"sources":      h.distroEval.SourcesStatus(),
		"generated_at": time.Now().UTC().Format(time.RFC3339),
	})
}

// resolveKernelFilter turns a raw kernel filter value (version expression or
// CVE id) into a KernelFilter. On failure it returns a nil filter plus an HTTP
// status and message for the caller to surface.
func (h *DashboardHandler) resolveKernelFilter(ctx context.Context, value string) (*util.KernelFilter, int, string) {
	if cve.IsCVEID(value) {
		if h.cve == nil {
			return nil, http.StatusServiceUnavailable, "CVE lookup is not available"
		}
		res, err := h.cve.Lookup(ctx, value)
		if err != nil {
			return nil, http.StatusBadGateway, err.Error()
		}
		return res.Filter, 0, ""
	}
	filter, err := util.ParseKernelExpr(value)
	if err != nil {
		return nil, http.StatusBadRequest, "Invalid kernel filter: " + err.Error()
	}
	return filter, 0, ""
}

// CVEKernelRanges handles GET /dashboard/cve/{cveId}/kernel-ranges. It resolves
// a CVE identifier to the affected upstream Linux-kernel version ranges so the
// UI can display what a CVE filter expands to.
func (h *DashboardHandler) CVEKernelRanges(w http.ResponseWriter, r *http.Request) {
	cveID := chi.URLParam(r, "cveId")
	if !cve.IsCVEID(cveID) {
		Error(w, http.StatusBadRequest, "Invalid CVE identifier")
		return
	}
	if h.cve == nil {
		Error(w, http.StatusServiceUnavailable, "CVE lookup is not available")
		return
	}
	res, err := h.cve.Lookup(r.Context(), cveID)
	if err != nil {
		Error(w, http.StatusBadGateway, err.Error())
		return
	}
	JSON(w, http.StatusOK, res)
}

// HostDetail handles GET /dashboard/hosts/:hostId.
func (h *DashboardHandler) HostDetail(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "hostId")
	limit := parseIntQuery(r, "limit", 10)
	offset := parseIntQuery(r, "offset", 0)
	include := r.URL.Query().Get("include")

	detail, err := h.dashboard.GetHostDetail(r.Context(), hostID, limit, offset)
	if err != nil || detail == nil {
		Error(w, http.StatusNotFound, "Host not found")
		return
	}

	// Include Docker data when requested (matches Node dashboard API)
	if include == "docker" && h.docker != nil {
		dockerDetail, err := h.docker.GetHostDockerDetail(r.Context(), hostID)
		if err == nil && dockerDetail != nil {
			containers := make([]map[string]interface{}, len(dockerDetail.Containers))
			for i, c := range dockerDetail.Containers {
				containers[i] = map[string]interface{}{
					"id": c.ID, "host_id": c.HostID, "container_id": c.ContainerID,
					"name": c.Name, "image_id": c.ImageID, "image_name": c.ImageName,
					"image_tag": c.ImageTag, "status": c.Status, "state": c.State,
					"ports": c.Ports, "labels": c.Labels,
					"created_at": c.CreatedAt, "started_at": c.StartedAt,
					"updated_at": c.UpdatedAt, "last_checked": c.LastChecked,
				}
				if c.ImageTag != "" {
					containers[i]["image"] = c.ImageName + ":" + c.ImageTag
				} else {
					containers[i]["image"] = c.ImageName
				}
			}

			images := make([]map[string]interface{}, len(dockerDetail.Images))
			for i, img := range dockerDetail.Images {
				images[i] = map[string]interface{}{
					"id": img.ID, "repository": img.Repository, "tag": img.Tag,
					"image_id": img.ImageID, "digest": img.Digest, "size_bytes": img.SizeBytes,
					"source": img.Source, "created_at": img.CreatedAt,
					"last_checked": img.LastChecked, "updated_at": img.UpdatedAt,
				}
				if img.SizeBytes != nil {
					images[i]["size"] = formatBytes(*img.SizeBytes)
				} else {
					images[i]["size"] = nil
				}
			}

			volumes := make([]map[string]interface{}, len(dockerDetail.Volumes))
			for i, v := range dockerDetail.Volumes {
				volumes[i] = map[string]interface{}{
					"id": v.ID, "host_id": v.HostID, "volume_id": v.VolumeID,
					"name": v.Name, "driver": v.Driver, "mountpoint": v.Mountpoint,
					"renderer": v.Renderer, "scope": v.Scope, "labels": v.Labels,
					"options": v.Options, "size_bytes": v.SizeBytes, "ref_count": v.RefCount,
					"created_at": v.CreatedAt, "updated_at": v.UpdatedAt, "last_checked": v.LastChecked,
				}
			}

			networks := make([]map[string]interface{}, len(dockerDetail.Networks))
			for i, n := range dockerDetail.Networks {
				networks[i] = map[string]interface{}{
					"id": n.ID, "host_id": n.HostID, "network_id": n.NetworkID,
					"name": n.Name, "driver": n.Driver, "scope": n.Scope,
					"ipv6_enabled": n.IPv6Enabled, "internal": n.Internal,
					"attachable": n.Attachable, "ingress": n.Ingress,
					"config_only": n.ConfigOnly, "labels": n.Labels, "ipam": n.IPAM,
					"container_count": n.ContainerCount,
					"created_at":      n.CreatedAt, "updated_at": n.UpdatedAt, "last_checked": n.LastChecked,
				}
			}

			running := 0
			if rc, ok := dockerDetail.Stats["runningContainers"]; ok {
				if n, ok := rc.(int); ok {
					running = n
				}
			}
			detail["docker"] = map[string]interface{}{
				"containers": containers,
				"images":     images,
				"volumes":    volumes,
				"networks":   networks,
				"stats": map[string]interface{}{
					"total_containers":   len(dockerDetail.Containers),
					"running_containers": running,
					"total_images":       len(dockerDetail.Images),
					"total_volumes":      len(dockerDetail.Volumes),
					"total_networks":     len(dockerDetail.Networks),
				},
			}
		}
	}

	JSON(w, http.StatusOK, detail)
}

// formatBytes formats bytes as human-readable string (e.g. "1.5 GB").
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// Packages handles GET /dashboard/packages.
func (h *DashboardHandler) Packages(w http.ResponseWriter, r *http.Request) {
	pkgs, err := h.dashboard.GetPackagesWithHosts(r.Context())
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to load packages")
		return
	}
	JSON(w, http.StatusOK, pkgs)
}

// PackageTrends handles GET /dashboard/package-trends.
func (h *DashboardHandler) PackageTrends(w http.ResponseWriter, r *http.Request) {
	days := parseIntQuery(r, "days", 30)
	hostID := r.URL.Query().Get("hostId")
	data, err := h.dashboard.GetPackageTrends(r.Context(), days, hostID)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to load package trends")
		return
	}
	JSON(w, http.StatusOK, data)
}

// RecentUsers handles GET /dashboard/recent-users.
func (h *DashboardHandler) RecentUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.dashboard.GetRecentUsers(r.Context(), 5)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to load recent users")
		return
	}
	JSON(w, http.StatusOK, users)
}

// RecentCollection handles GET /dashboard/recent-collection.
func (h *DashboardHandler) RecentCollection(w http.ResponseWriter, r *http.Request) {
	hosts, err := h.dashboard.GetRecentCollection(r.Context(), 5)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to load recent collection")
		return
	}
	JSON(w, http.StatusOK, hosts)
}

// HostQueue handles GET /dashboard/hosts/:hostId/queue.
// Returns queue stats and job history for the host (matches Node backend shape for frontend).
func (h *DashboardHandler) HostQueue(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "hostId")
	limit := parseIntQuery(r, "limit", 20)

	host, err := h.hosts.GetByID(r.Context(), hostID)
	if err != nil || host == nil {
		Error(w, http.StatusNotFound, "Host not found")
		return
	}

	// Build response matching Node: { success: true, data: { hostId, apiId, friendlyName, waiting, active, delayed, failed, jobHistory } }
	data := map[string]interface{}{
		"hostId":       hostID,
		"apiId":        host.ApiID,
		"friendlyName": host.FriendlyName,
		"waiting":      0,
		"active":       0,
		"delayed":      0,
		"failed":       0,
		"jobHistory":   []queue.HostJobRow{},
	}

	if h.inspector != nil {
		queueData, err := queue.GetHostJobs(r.Context(), h.inspector, host.ApiID, limit)
		if err == nil {
			data["waiting"] = queueData.Waiting
			data["active"] = queueData.Active
			data["delayed"] = queueData.Delayed
			data["failed"] = queueData.Failed
			data["jobHistory"] = queueData.JobHistory
		}
	}

	// Merge with DB job_history (exclude live job IDs already in data)
	liveIDs := make(map[string]bool)
	if hist, ok := data["jobHistory"].([]queue.HostJobRow); ok {
		for _, j := range hist {
			liveIDs[j.JobID] = true
		}
	}
	dbRows, _ := h.dashboard.GetJobHistoryByApiID(r.Context(), host.ApiID, limit)
	for _, dbRow := range dbRows {
		if liveIDs[dbRow.JobID] {
			continue
		}
		liveIDs[dbRow.JobID] = true
		createdAt := dbRow.CreatedAt.Time
		updatedAt := dbRow.UpdatedAt.Time
		var completedAt *time.Time
		if dbRow.CompletedAt.Valid {
			t := dbRow.CompletedAt.Time
			completedAt = &t
		}
		var errMsg *string
		if dbRow.ErrorMessage != nil {
			errMsg = dbRow.ErrorMessage
		}
		var output interface{}
		if len(dbRow.Output) > 0 {
			_ = json.Unmarshal(dbRow.Output, &output)
		}
		jobRow := queue.HostJobRow{
			ID:            dbRow.ID,
			JobID:         dbRow.JobID,
			JobName:       dbRow.JobName,
			QueueName:     &dbRow.QueueName,
			Status:        dbRow.Status,
			AttemptNumber: int(dbRow.AttemptNumber),
			ErrorMessage:  errMsg,
			Output:        output,
			CreatedAt:     &createdAt,
			UpdatedAt:     &updatedAt,
			CompletedAt:   completedAt,
		}
		data["jobHistory"] = append(data["jobHistory"].([]queue.HostJobRow), jobRow)
	}

	// Trim to limit
	if hist, ok := data["jobHistory"].([]queue.HostJobRow); ok && len(hist) > limit {
		data["jobHistory"] = hist[:limit]
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    data,
	})
}
