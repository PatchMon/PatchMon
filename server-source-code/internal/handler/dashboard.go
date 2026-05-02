package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/queue"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
)

// DashboardHandler handles dashboard routes.
type DashboardHandler struct {
	dashboard *store.DashboardStore
	hosts     *store.HostsStore
	packages  *store.PackagesStore
	users     *store.UsersStore
	docker    *store.DockerStore
	activity  *store.AgentActivityStore
	inspector *asynq.Inspector
}

// NewDashboardHandler creates a new dashboard handler.
func NewDashboardHandler(dashboard *store.DashboardStore, hosts *store.HostsStore, packages *store.PackagesStore, users *store.UsersStore, docker *store.DockerStore, activity *store.AgentActivityStore, inspector *asynq.Inspector) *DashboardHandler {
	return &DashboardHandler{dashboard: dashboard, hosts: hosts, packages: packages, users: users, docker: docker, activity: activity, inspector: inspector}
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
	hosts, err := h.dashboard.GetHostsWithCounts(r.Context(), params)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to load hosts")
		return
	}
	JSON(w, http.StatusOK, hosts)
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
		queueData, err := queue.GetHostJobs(r.Context(), h.inspector, hostID, host.ApiID, limit)
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

// activityItem is the wire shape returned by HostActivity for each merged
// row. Combines Agent Activity (update_history) rows with live asynq jobs
// for a unified per-host timeline.
type activityItem struct {
	Kind               string     `json:"kind"`
	ID                 string     `json:"id"`
	OccurredAt         time.Time  `json:"occurred_at"`
	Type               string     `json:"type"`
	JobID              *string    `json:"job_id,omitempty"`
	JobName            *string    `json:"job_name,omitempty"`
	QueueName          *string    `json:"queue_name,omitempty"`
	SectionsSent       []string   `json:"sections_sent"`
	SectionsUnchanged  []string   `json:"sections_unchanged"`
	PayloadSizeKb      *float64   `json:"payload_size_kb,omitempty"`
	ServerProcessingMs *float64   `json:"server_processing_ms,omitempty"`
	AgentExecutionMs   *int       `json:"agent_execution_ms,omitempty"`
	AttemptNumber      *int       `json:"attempt_number,omitempty"`
	Status             string     `json:"status"`
	ErrorMessage       *string    `json:"error_message,omitempty"`
	PackagesCount      *int       `json:"packages_count,omitempty"`
	SecurityCount      *int       `json:"security_count,omitempty"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
	Output             *string    `json:"output,omitempty"`
}

// HostActivity handles GET /dashboard/hosts/:hostId/activity. Returns the
// unified Agent Activity feed plus the four queue stat counts (waiting,
// active, delayed, failed) above the table.
//
// Query params:
//
//	direction: "" (all) | "in" (reports) | "out" (jobs)
//	type:      comma-separated report-type / job-name filter
//	status:    comma-separated status filter
//	since:     RFC3339 timestamp; rows older than this are excluded
//	search:    case-insensitive ILIKE match on error_message and job output
//	limit:     default 50, max 500
//	offset:    default 0
func (h *DashboardHandler) HostActivity(w http.ResponseWriter, r *http.Request) {
	hostID := chi.URLParam(r, "hostId")
	limit := parseIntQuery(r, "limit", 50)
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	offset := parseIntQuery(r, "offset", 0)
	direction := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("direction")))
	typesFilter := splitCSV(r.URL.Query().Get("type"))
	statusFilter := splitCSV(r.URL.Query().Get("status"))
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	var sinceTs *time.Time
	if v := strings.TrimSpace(r.URL.Query().Get("since")); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			sinceTs = &t
		}
	}

	host, err := h.hosts.GetByID(r.Context(), hostID)
	if err != nil || host == nil {
		Error(w, http.StatusNotFound, "Host not found")
		return
	}

	// Stats block: same data the existing HostQueue endpoint returned, kept
	// verbatim so the migrated UI can re-use the existing stat-card design.
	stats := map[string]int{"waiting": 0, "active": 0, "delayed": 0, "failed": 0}
	var liveJobs []queue.HostJobRow
	if h.inspector != nil {
		queueData, qerr := queue.GetHostJobs(r.Context(), h.inspector, hostID, host.ApiID, limit)
		if qerr == nil && queueData != nil {
			stats["waiting"] = queueData.Waiting
			stats["active"] = queueData.Active
			stats["delayed"] = queueData.Delayed
			stats["failed"] = queueData.Failed
			liveJobs = queueData.JobHistory
		}
	}

	if direction != "in" && direction != "out" {
		direction = ""
	}

	items := make([]activityItem, 0, limit)
	liveByJobID := make(map[string]queue.HostJobRow, len(liveJobs))
	for _, j := range liveJobs {
		if j.JobID != "" {
			liveByJobID[j.JobID] = j
		}
	}

	rows, total, err := h.activity.List(r.Context(), store.ListAgentActivityParams{
		HostID:    hostID,
		Direction: direction,
		Types:     typesFilter,
		Statuses:  statusFilter,
		Search:    search,
		Since:     sinceTs,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		slog.Error("agent activity list failed", "host_id", hostID, "error", err)
		Error(w, http.StatusInternalServerError, "Failed to load agent activity")
		return
	}

	for _, row := range rows {
		item := activityItemFromStore(row)
		if row.Kind == "job" && row.JobID != nil {
			if live, ok := liveByJobID[*row.JobID]; ok {
				applyLiveJobState(&item, live)
			}
		}
		items = append(items, item)
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"hostId":       hostID,
		"apiId":        host.ApiID,
		"friendlyName": host.FriendlyName,
		"stats":        stats,
		"items":        items,
		"total":        total,
	})
}

func splitCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func applyLiveJobState(item *activityItem, live queue.HostJobRow) {
	item.Status = live.Status
	attempt := live.AttemptNumber
	item.AttemptNumber = &attempt
	item.ErrorMessage = live.ErrorMessage
	item.QueueName = live.QueueName
	item.CompletedAt = live.CompletedAt
	if live.Output != nil {
		if s, ok := live.Output.(string); ok {
			item.Output = &s
		}
	}
}

func activityItemFromStore(r store.AgentActivityRow) activityItem {
	return activityItem{
		Kind:               r.Kind,
		ID:                 r.ID,
		OccurredAt:         r.OccurredAt,
		Type:               r.Type,
		JobID:              r.JobID,
		JobName:            r.JobName,
		QueueName:          r.QueueName,
		SectionsSent:       r.SectionsSent,
		SectionsUnchanged:  r.SectionsUnchanged,
		PayloadSizeKb:      r.PayloadSizeKb,
		ServerProcessingMs: r.ServerProcessingMs,
		AgentExecutionMs:   r.AgentExecutionMs,
		AttemptNumber:      r.AttemptNumber,
		Status:             r.Status,
		ErrorMessage:       r.ErrorMessage,
		PackagesCount:      r.PackagesCount,
		SecurityCount:      r.SecurityCount,
		CompletedAt:        r.CompletedAt,
		Output:             r.Output,
	}
}
