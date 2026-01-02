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

	// Trigger a compliance scan
	triggerScan: (hostId, profileType = "all") =>
		api.post(`/compliance/trigger/${hostId}`, { profile_type: profileType }),

	// Get compliance score trends
	getTrends: (hostId, days = 30) =>
		api.get(`/compliance/trends/${hostId}`, { params: { days } }),
};
