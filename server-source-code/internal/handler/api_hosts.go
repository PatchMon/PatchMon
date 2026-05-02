package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/queue"
	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
)

// ApiHostsHandler handles GET/DELETE /api/v1/api/hosts (scoped API, Basic Auth).
type ApiHostsHandler struct {
	hosts      *store.HostsStore
	hostGroups *store.HostGroupsStore
	db         database.DBProvider
	dashboard  *store.DashboardStore
	inspector  *asynq.Inspector
}

// NewApiHostsHandler creates a new scoped API hosts handler.
func NewApiHostsHandler(hosts *store.HostsStore, hostGroups *store.HostGroupsStore, db database.DBProvider, dashboard *store.DashboardStore, inspector *asynq.Inspector) *ApiHostsHandler {
	return &ApiHostsHandler{hosts: hosts, hostGroups: hostGroups, db: db, dashboard: dashboard, inspector: inspector}
}

func isUUID(s string) bool {
	_, err := uuid.Parse(strings.TrimSpace(s))
	return err == nil
}

func formatTimestamp(t pgtype.Timestamp) string {
	if t.Valid {
		return t.Time.Format(time.RFC3339)
	}
	return ""
}

// resolveHostGroupParam returns group IDs from comma-separated names or UUIDs.
func (h *ApiHostsHandler) resolveHostGroupParam(ctx context.Context, hostgroup string) ([]string, []string) {
	if hostgroup == "" {
		return nil, nil
	}
	values := strings.Split(hostgroup, ",")
	var ids []string
	var filteredBy []string
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		filteredBy = append(filteredBy, v)
		if isUUID(v) {
			ids = append(ids, v)
		} else {
			groups, _ := h.hostGroups.List(ctx)
			for _, g := range groups {
				if g.Name == v {
					ids = append(ids, g.ID)
					break
				}
			}
		}
	}
	return ids, filteredBy
}

// ListHosts handles GET /api/v1/api/hosts.
func (h *ApiHostsHandler) ListHosts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	hostgroup := r.URL.Query().Get("hostgroup")
	include := r.URL.Query().Get("include")
	includeStats := false
	if include != "" {
		for _, s := range strings.Split(include, ",") {
			if strings.TrimSpace(strings.ToLower(s)) == "stats" {
				includeStats = true
				break
			}
		}
	}

	groupIDs, filteredBy := h.resolveHostGroupParam(ctx, hostgroup)
	if len(filteredBy) > 0 && len(groupIDs) == 0 {
		// hostgroup was provided but no valid groups found (Node parity)
		resp := map[string]interface{}{"hosts": []interface{}{}, "total": 0, "filtered_by_groups": filteredBy}
		JSON(w, http.StatusOK, resp)
		return
	}
	hosts, groupsMap, statsMap, err := h.hosts.ListForScopedApi(ctx, groupIDs, includeStats)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to fetch hosts")
		return
	}

	out := make([]map[string]interface{}, len(hosts))
	for i, host := range hosts {
		groups := groupsMap[host.ID]
		if groups == nil {
			groups = []models.HostGroup{}
		}
		hg := make([]map[string]string, len(groups))
		for j, g := range groups {
			hg[j] = map[string]string{"id": g.ID, "name": g.Name}
		}
		ip := ""
		if host.IP != nil {
			ip = *host.IP
		}
		hostname := ""
		if host.Hostname != nil {
			hostname = *host.Hostname
		}
		item := map[string]interface{}{
			"id":            host.ID,
			"friendly_name": host.FriendlyName,
			"hostname":      hostname,
			"ip":            ip,
			"host_groups":   hg,
		}
		if includeStats {
			item["os_type"] = host.OSType
			item["os_version"] = host.OSVersion
			item["last_update"] = host.LastUpdate.Format(time.RFC3339)
			item["status"] = host.Status
			item["needs_reboot"] = host.NeedsReboot != nil && *host.NeedsReboot
			st := statsMap[host.ID]
			item["updates_count"] = st.Outdated
			item["security_updates_count"] = st.Security
			item["total_packages"] = st.Total
		}
		out[i] = item
	}

	resp := map[string]interface{}{
		"hosts": out,
		"total": len(out),
	}
	if len(filteredBy) > 0 {
		resp["filtered_by_groups"] = filteredBy
	}
	JSON(w, http.StatusOK, resp)
}

// GetHostStats handles GET /api/v1/api/hosts/:id/stats.
func (h *ApiHostsHandler) GetHostStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d := h.db.DB(ctx)
	id := chi.URLParam(r, "id")
	if _, err := h.hosts.GetByID(ctx, id); err != nil {
		Error(w, http.StatusNotFound, "Host not found")
		return
	}
	stats, err := d.Queries.GetHostPackageStats(ctx, id)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to fetch host statistics")
		return
	}
	repoRows, _ := d.Queries.GetHostRepositoryCountByHostIDs(ctx, []string{id})
	totalRepos := 0
	for _, row := range repoRows {
		if row.HostID == id {
			totalRepos = int(row.Cnt)
			break
		}
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"host_id":                  id,
		"total_installed_packages": stats.Column1,
		"outdated_packages":        stats.Column2,
		"security_updates":         stats.Column3,
		"total_repos":              totalRepos,
	})
}

// GetHostInfo handles GET /api/v1/api/hosts/:id/info.
func (h *ApiHostsHandler) GetHostInfo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	host, err := h.hosts.GetByID(ctx, id)
	if err != nil || host == nil {
		Error(w, http.StatusNotFound, "Host not found")
		return
	}
	groups, _ := h.hosts.GetHostGroups(ctx, id)
	hg := make([]map[string]string, len(groups))
	for i, g := range groups {
		hg[i] = map[string]string{"id": g.ID, "name": g.Name}
	}
	ip := ""
	if host.IP != nil {
		ip = *host.IP
	}
	hostname := ""
	if host.Hostname != nil {
		hostname = *host.Hostname
	}
	agentVersion := ""
	if host.AgentVersion != nil {
		agentVersion = *host.AgentVersion
	}
	machineID := ""
	if host.MachineID != nil {
		machineID = *host.MachineID
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"id":            host.ID,
		"machine_id":    machineID,
		"friendly_name": host.FriendlyName,
		"hostname":      hostname,
		"ip":            ip,
		"os_type":       host.OSType,
		"os_version":    host.OSVersion,
		"agent_version": agentVersion,
		"host_groups":   hg,
	})
}

// GetHostNetwork handles GET /api/v1/api/hosts/:id/network.
func (h *ApiHostsHandler) GetHostNetwork(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	host, err := h.hosts.GetByID(ctx, id)
	if err != nil || host == nil {
		Error(w, http.StatusNotFound, "Host not found")
		return
	}
	ip := ""
	if host.IP != nil {
		ip = *host.IP
	}
	gateway := ""
	if host.GatewayIP != nil {
		gateway = *host.GatewayIP
	}
	dnsServers := []interface{}{}
	if len(host.DNSServers) > 0 {
		_ = json.Unmarshal(host.DNSServers, &dnsServers)
	}
	networkInterfaces := []interface{}{}
	if len(host.NetworkInterfaces) > 0 {
		_ = json.Unmarshal(host.NetworkInterfaces, &networkInterfaces)
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"id":                 host.ID,
		"ip":                 ip,
		"gateway_ip":         gateway,
		"dns_servers":        dnsServers,
		"network_interfaces": networkInterfaces,
	})
}

// GetHostSystem handles GET /api/v1/api/hosts/:id/system.
func (h *ApiHostsHandler) GetHostSystem(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	host, err := h.hosts.GetByID(ctx, id)
	if err != nil || host == nil {
		Error(w, http.StatusNotFound, "Host not found")
		return
	}
	arch := ""
	if host.Architecture != nil {
		arch = *host.Architecture
	}
	kernelVersion := ""
	if host.KernelVersion != nil {
		kernelVersion = *host.KernelVersion
	}
	installedKernel := ""
	if host.InstalledKernelVersion != nil {
		installedKernel = *host.InstalledKernelVersion
	}
	selinux := ""
	if host.SelinuxStatus != nil {
		selinux = *host.SelinuxStatus
	}
	uptime := ""
	if host.SystemUptime != nil {
		uptime = *host.SystemUptime
	}
	cpuModel := ""
	if host.CPUModel != nil {
		cpuModel = *host.CPUModel
	}
	cpuCores := 0
	if host.CPUCores != nil {
		cpuCores = *host.CPUCores
	}
	ram := float64(0)
	if host.RamInstalled != nil {
		ram = *host.RamInstalled
	}
	swap := float64(0)
	if host.SwapSize != nil {
		swap = *host.SwapSize
	}
	loadAvg := map[string]interface{}{}
	if len(host.LoadAverage) > 0 {
		_ = json.Unmarshal(host.LoadAverage, &loadAvg)
	}
	diskDetails := []interface{}{}
	if len(host.DiskDetails) > 0 {
		_ = json.Unmarshal(host.DiskDetails, &diskDetails)
	}
	needsReboot := false
	if host.NeedsReboot != nil {
		needsReboot = *host.NeedsReboot
	}
	rebootReason := ""
	if host.RebootReason != nil {
		rebootReason = *host.RebootReason
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"id":                       host.ID,
		"architecture":             arch,
		"kernel_version":           kernelVersion,
		"installed_kernel_version": installedKernel,
		"selinux_status":           selinux,
		"system_uptime":            uptime,
		"cpu_model":                cpuModel,
		"cpu_cores":                cpuCores,
		"ram_installed":            ram,
		"swap_size":                swap,
		"load_average":             loadAvg,
		"disk_details":             diskDetails,
		"needs_reboot":             needsReboot,
		"reboot_reason":            rebootReason,
	})
}

// GetHostPackages handles GET /api/v1/api/hosts/:id/packages.
func (h *ApiHostsHandler) GetHostPackages(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d := h.db.DB(ctx)
	id := chi.URLParam(r, "id")
	updatesOnly := r.URL.Query().Get("updates_only") == "true"

	host, err := h.hosts.GetByID(ctx, id)
	if err != nil || host == nil {
		Error(w, http.StatusNotFound, "Host not found")
		return
	}
	rows, err := d.Queries.GetHostPackagesForScopedApi(ctx, id)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to fetch packages")
		return
	}
	hostname := ""
	if host.Hostname != nil {
		hostname = *host.Hostname
	}
	packages := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		if updatesOnly && !row.NeedsUpdate {
			continue
		}
		lastChecked := ""
		if row.LastChecked.Valid {
			lastChecked = row.LastChecked.Time.Format(time.RFC3339)
		}
		desc := ""
		if row.PkgDescription != nil {
			desc = *row.PkgDescription
		}
		category := ""
		if row.PkgCategory != nil {
			category = *row.PkgCategory
		}
		packages = append(packages, map[string]interface{}{
			"id":                 row.ID,
			"name":               row.PkgName,
			"description":        desc,
			"category":           category,
			"current_version":    row.CurrentVersion,
			"available_version":  row.AvailableVersion,
			"needs_update":       row.NeedsUpdate,
			"is_security_update": row.IsSecurityUpdate,
			"last_checked":       lastChecked,
		})
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"host": map[string]interface{}{
			"id":            host.ID,
			"hostname":      hostname,
			"friendly_name": host.FriendlyName,
		},
		"packages": packages,
		"total":    len(packages),
	})
}

// GetHostPackageReports handles GET /api/v1/api/hosts/:id/package_reports.
func (h *ApiHostsHandler) GetHostPackageReports(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d := h.db.DB(ctx)
	id := chi.URLParam(r, "id")
	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	if _, err := h.hosts.GetByID(ctx, id); err != nil {
		Error(w, http.StatusNotFound, "Host not found")
		return
	}
	rows, err := d.Queries.GetUpdateHistory(ctx, db.GetUpdateHistoryParams{
		HostID: id,
		Limit:  int32(limit),
		Offset: 0,
	})
	if err != nil {
		Error(w, http.StatusInternalServerError, "Failed to fetch package reports")
		return
	}
	reports := make([]map[string]interface{}, len(rows))
	for i, row := range rows {
		totalPkg := 0
		if row.TotalPackages != nil {
			totalPkg = int(*row.TotalPackages)
		}
		execTime := 0.0
		if row.ExecutionTime != nil {
			execTime = *row.ExecutionTime
		}
		payloadKb := 0.0
		if row.PayloadSizeKb != nil {
			payloadKb = *row.PayloadSizeKb
		}
		reports[i] = map[string]interface{}{
			"id":                     row.ID,
			"status":                 row.Status,
			"date":                   formatTimestamp(row.Timestamp),
			"total_packages":         totalPkg,
			"outdated_packages":      row.PackagesCount,
			"security_updates":       row.SecurityCount,
			"payload_kb":             payloadKb,
			"execution_time_seconds": execTime,
			"error_message":          row.ErrorMessage,
		}
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"host_id": id,
		"reports": reports,
		"total":   len(reports),
	})
}

// GetHostAgentQueue handles GET /api/v1/api/hosts/:id/agent_queue.
func (h *ApiHostsHandler) GetHostAgentQueue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	host, err := h.hosts.GetByID(ctx, id)
	if err != nil || host == nil {
		Error(w, http.StatusNotFound, "Host not found")
		return
	}
	queueStatus := map[string]int{"waiting": 0, "active": 0, "delayed": 0, "failed": 0}
	if h.inspector != nil {
		data, err := queue.GetHostJobs(ctx, h.inspector, id, host.ApiID, limit)
		if err == nil {
			queueStatus["waiting"] = data.Waiting
			queueStatus["active"] = data.Active
			queueStatus["delayed"] = data.Delayed
			queueStatus["failed"] = data.Failed
		}
	}
	dbRows, _ := h.dashboard.GetJobHistoryByApiID(ctx, host.ApiID, limit)
	jobHistory := make([]map[string]interface{}, len(dbRows))
	for i, row := range dbRows {
		createdAt := ""
		if row.CreatedAt.Valid {
			createdAt = row.CreatedAt.Time.Format(time.RFC3339)
		}
		completedAt := interface{}(nil)
		if row.CompletedAt.Valid {
			completedAt = row.CompletedAt.Time.Format(time.RFC3339)
		}
		jobHistory[i] = map[string]interface{}{
			"id":            row.ID,
			"job_id":        row.JobID,
			"job_name":      row.JobName,
			"status":        row.Status,
			"attempt":       row.AttemptNumber,
			"created_at":    createdAt,
			"completed_at":  completedAt,
			"error_message": row.ErrorMessage,
			"output":        nil,
		}
		if len(row.Output) > 0 {
			var out interface{}
			_ = json.Unmarshal(row.Output, &out)
			jobHistory[i]["output"] = out
		}
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"host_id":      id,
		"queue_status": queueStatus,
		"job_history":  jobHistory,
		"total_jobs":   len(jobHistory),
	})
}

// GetHostNotes handles GET /api/v1/api/hosts/:id/notes.
func (h *ApiHostsHandler) GetHostNotes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	host, err := h.hosts.GetByID(ctx, id)
	if err != nil || host == nil {
		Error(w, http.StatusNotFound, "Host not found")
		return
	}
	notes := ""
	if host.Notes != nil {
		notes = *host.Notes
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"host_id": id,
		"notes":   notes,
	})
}

// GetHostIntegrations handles GET /api/v1/api/hosts/:id/integrations.
func (h *ApiHostsHandler) GetHostIntegrations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	host, err := h.hosts.GetByID(ctx, id)
	if err != nil || host == nil {
		Error(w, http.StatusNotFound, "Host not found")
		return
	}
	dockerDesc := "Monitor Docker containers, images, volumes, and networks. Collects real-time container status events."
	dockerInfo := map[string]interface{}{
		"enabled":     host.DockerEnabled,
		"description": dockerDesc,
	}
	if host.DockerEnabled {
		d := h.db.DB(ctx)
		containers, _ := d.Queries.CountContainers(ctx, db.CountContainersParams{HostID: &id})
		volumes, _ := d.Queries.CountVolumesByHostID(ctx, id)
		networks, _ := d.Queries.CountNetworksByHostID(ctx, id)
		dockerInfo["containers_count"] = containers
		dockerInfo["volumes_count"] = volumes
		dockerInfo["networks_count"] = networks
	}
	JSON(w, http.StatusOK, map[string]interface{}{
		"host_id":      id,
		"integrations": map[string]interface{}{"docker": dockerInfo},
	})
}

// DeleteHost handles DELETE /api/v1/api/hosts/:id.
func (h *ApiHostsHandler) DeleteHost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	if !isUUID(id) {
		JSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid host ID format"})
		return
	}
	host, err := h.hosts.GetByID(ctx, id)
	if err != nil || host == nil {
		Error(w, http.StatusNotFound, "Host not found")
		return
	}
	hostname := ""
	if host.Hostname != nil {
		hostname = *host.Hostname
	}
	if err := h.hosts.Delete(ctx, id); err != nil {
		Error(w, http.StatusInternalServerError, "Failed to delete host")
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"message": "Host deleted successfully",
		"deleted": map[string]interface{}{
			"id":            host.ID,
			"friendly_name": host.FriendlyName,
			"hostname":      hostname,
		},
	})
}
