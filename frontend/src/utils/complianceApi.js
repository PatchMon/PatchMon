import api from "./api";

export const complianceAPI = {
	// Get all compliance profiles
	getProfiles: () => api.get("/compliance/profiles"),

	// Get dashboard statistics
	getDashboard: () => api.get("/compliance/dashboard"),

	// Get scan history for a host
	getHostScans: (hostId, params = {}) =>
		api.get(`/compliance/scans/${hostId}`, { params }),

	// Get latest scan for a host (optionally filter by profile type)
	getLatestScan: (hostId, profileType = null) =>
		api.get(`/compliance/scans/${hostId}/latest`, {
			params: profileType ? { profile_type: profileType } : {},
		}),

	// Get latest scan summary for each profile type (openscap, docker-bench)
	getLatestScansByType: (hostId) =>
		api.get(`/compliance/scans/${hostId}/latest-by-type`),

	// Get detailed results for a scan (paginated: limit, offset, status, severity)
	getScanResults: (scanId, params = {}) =>
		api
			.get(`/compliance/results/${scanId}`, { params })
			.then((res) => res.data),

	// Trigger a compliance scan with options
	triggerScan: (hostId, options = {}) =>
		api.post(`/compliance/trigger/${hostId}`, {
			profile_type: options.profileType || options.profile_type || "all",
			profile_id: options.profileId || options.profile_id || null,
			enable_remediation: options.enableRemediation || false,
			fetch_remote_resources: options.fetchRemoteResources || false,
			// Docker image CVE scan options
			image_name: options.imageName || null,
			scan_all_images: options.scanAllImages || false,
		}),

	// Request the agent to cancel the currently running compliance scan (runs in background; cancel is optional)
	cancelScan: (hostId) => api.post(`/compliance/cancel/${hostId}`),

	// Get compliance score trends
	getTrends: (hostId, days = 30) =>
		api.get(`/compliance/trends/${hostId}`, { params: { days } }),

	// Get compliance integration status (scanner info, components)
	getIntegrationStatus: (hostId) =>
		api.get(`/hosts/${hostId}/integrations/compliance/status`),

	// Upgrade SSG content packages on the agent
	upgradeSSG: (hostId) => api.post(`/compliance/upgrade-ssg/${hostId}`),

	// Install scanner (OpenSCAP + SSG) on the agent via BullMQ job
	installScanner: (hostId) => api.post(`/compliance/install-scanner/${hostId}`),

	// Get install job status for progress polling
	getInstallJobStatus: (hostId) =>
		api.get(`/compliance/install-job/${hostId}`).then((res) => res.data),

	// Cancel the current install job for a host
	cancelInstallScanner: (hostId) =>
		api.post(`/compliance/install-scanner/${hostId}/cancel`),

	// Remediate a single failed rule
	remediateRule: (hostId, ruleId) =>
		api.post(`/compliance/remediate/${hostId}`, { rule_id: ruleId }),

	// Get currently running/active scans
	getActiveScans: () => api.get("/compliance/scans/active"),

	// Trigger bulk compliance scans on multiple hosts
	triggerBulkScan: (hostIds, options = {}) =>
		api.post("/compliance/trigger/bulk", {
			hostIds,
			profile_type: options.profileType || "all",
			profile_id: options.profileId || null,
			enable_remediation: options.enableRemediation || false,
			fetch_remote_resources: options.fetchRemoteResources || false,
		}),

	// Get global scan history (paginated, filterable)
	getScanHistory: (params = {}) =>
		api.get("/compliance/scans/history", { params }).then((res) => res.data),

	// Get rules with aggregated cross-host pass/fail/warn counts
	getRules: (params = {}) =>
		api.get("/compliance/rules", { params }).then((res) => res.data),

	// Get detailed rule info plus affected hosts
	getRuleDetail: (ruleId) =>
		api.get(`/compliance/rules/${ruleId}`).then((res) => res.data),
};
