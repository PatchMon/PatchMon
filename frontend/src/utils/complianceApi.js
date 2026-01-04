import api from "./api";

export const complianceAPI = {
	// Get all compliance profiles
	getProfiles: () => api.get("/compliance/profiles"),

	// Get dashboard statistics
	getDashboard: () => api.get("/compliance/dashboard"),

	// Get scan history for a host
	getHostScans: (hostId, params = {}) =>
		api.get(`/compliance/scans/${hostId}`, { params }),

	// Get latest scan for a host
	getLatestScan: (hostId) =>
		api.get(`/compliance/scans/${hostId}/latest`),

	// Get detailed results for a scan
	getScanResults: (scanId, params = {}) =>
		api.get(`/compliance/results/${scanId}`, { params }),

	// Trigger a compliance scan with options
	triggerScan: (hostId, options = {}) =>
		api.post(`/compliance/trigger/${hostId}`, {
			profile_type: options.profileType || options.profile_type || "all",
			profile_id: options.profileId || options.profile_id || null,
			enable_remediation: options.enableRemediation || false,
			fetch_remote_resources: options.fetchRemoteResources || false,
		}),

	// Get compliance score trends
	getTrends: (hostId, days = 30) =>
		api.get(`/compliance/trends/${hostId}`, { params: { days } }),

	// Get compliance integration status (scanner info, components)
	getIntegrationStatus: (hostId) =>
		api.get(`/hosts/${hostId}/integrations/compliance/status`),

	// Upgrade SSG content packages on the agent
	upgradeSSG: (hostId) => api.post(`/compliance/upgrade-ssg/${hostId}`),

	// Remediate a single failed rule
	remediateRule: (hostId, ruleId) =>
		api.post(`/compliance/remediate/${hostId}`, { rule_id: ruleId }),
};
