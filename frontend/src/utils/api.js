import axios from "axios";

const API_BASE_URL = import.meta.env.VITE_API_URL || "/api/v1";

// Create axios instance with default config
// Uses httpOnly cookies for authentication (credentials: include)
const api = axios.create({
	baseURL: API_BASE_URL,
	timeout: 10000, // 10 seconds
	headers: {
		"Content-Type": "application/json",
	},
	withCredentials: true, // Send cookies with requests for httpOnly token auth
});

// Request interceptor
api.interceptors.request.use(
	(config) => {
		// Authentication is handled via httpOnly cookies (withCredentials: true)
		// No need to add Authorization header - server reads from cookies

		// Add device ID for TFA remember-me functionality
		// This uniquely identifies the browser profile (normal vs incognito)
		let deviceId = localStorage.getItem("device_id");
		if (!deviceId) {
			// Generate a unique device ID and store it
			// Use crypto.randomUUID() if available, otherwise generate a UUID v4 manually
			if (typeof crypto !== "undefined" && crypto.randomUUID) {
				deviceId = crypto.randomUUID();
			} else {
				// Fallback: Generate UUID v4 manually
				deviceId = "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx".replace(
					/[xy]/g,
					(c) => {
						const r = (Math.random() * 16) | 0;
						const v = c === "x" ? r : (r & 0x3) | 0x8;
						return v.toString(16);
					},
				);
			}
			localStorage.setItem("device_id", deviceId);
		}
		config.headers["X-Device-ID"] = deviceId;

		return config;
	},
	(error) => {
		return Promise.reject(error);
	},
);

// Response interceptor
api.interceptors.response.use(
	(response) => response,
	(error) => {
		if (error.response?.status === 401) {
			// Don't redirect if we're on the login page or if it's a TFA-related error
			const currentPath = window.location.pathname;
			const requestUrl = error.config?.url || "";
			const isTfaError =
				requestUrl.includes("/verify-tfa") || requestUrl.includes("/tfa/");

			if (currentPath !== "/login" && !isTfaError) {
				// Dispatch event for AuthContext to handle - avoids race with React updates
				// that could trigger ErrorBoundary "Something went wrong" before redirect
				localStorage.removeItem("user");
				window.dispatchEvent(new CustomEvent("auth:session-expired"));
			}
		}
		return Promise.reject(error);
	},
);

// Dashboard API
export const dashboardAPI = {
	getStats: () => api.get("/dashboard/stats"),
	getHosts: (params = {}) => {
		const queryString = new URLSearchParams(params).toString();
		return api.get(`/dashboard/hosts${queryString ? `?${queryString}` : ""}`);
	},
	getPackages: () => api.get("/dashboard/packages"),
	getHostDetail: (hostId, params = {}) => {
		const queryString = new URLSearchParams(params).toString();
		const url = `/dashboard/hosts/${hostId}${queryString ? `?${queryString}` : ""}`;
		return api.get(url);
	},
	getHostQueue: (hostId, params = {}) => {
		const queryString = new URLSearchParams(params).toString();
		const url = `/dashboard/hosts/${hostId}/queue${queryString ? `?${queryString}` : ""}`;
		return api.get(url);
	},
	getHostWsStatus: (hostId) => api.get(`/dashboard/hosts/${hostId}/ws-status`),
	getWsStatusByApiId: (apiId) => api.get(`/ws/status/${apiId}`),
	getPackageTrends: (params = {}) => {
		const queryString = new URLSearchParams(params).toString();
		const url = `/dashboard/package-trends${queryString ? `?${queryString}` : ""}`;
		return api.get(url);
	},
	getPackageSpikeAnalysis: (params = {}) => {
		const queryString = new URLSearchParams(params).toString();
		const url = `/dashboard/package-spike-analysis${queryString ? `?${queryString}` : ""}`;
		return api.get(url);
	},
	getRecentUsers: () => api.get("/dashboard/recent-users"),
	getRecentCollection: () => api.get("/dashboard/recent-collection"),
	triggerSystemStatistics: () =>
		api.post("/automation/trigger/system-statistics"),
};

// Admin Hosts API (for management interface)
export const adminHostsAPI = {
	create: (data) => api.post("/hosts/create", data),
	list: () => api.get("/hosts/admin/list"),
	delete: (hostId) => api.delete(`/hosts/${hostId}`),
	deleteBulk: (hostIds) => api.delete("/hosts/bulk", { data: { hostIds } }),
	regenerateCredentials: (hostId) =>
		api.post(`/hosts/${hostId}/regenerate-credentials`),
	updateGroup: (hostId, hostGroupId) =>
		api.put(`/hosts/${hostId}/group`, { hostGroupId }),
	updateGroups: (hostId, groupIds) =>
		api.put(`/hosts/${hostId}/groups`, { groupIds }),
	bulkUpdateGroup: (hostIds, hostGroupId) =>
		api.put("/hosts/bulk/group", { hostIds, hostGroupId }),
	bulkUpdateGroups: (hostIds, groupIds) =>
		api.put("/hosts/bulk/groups", { hostIds, groupIds }),
	toggleAutoUpdate: (hostId, autoUpdate) =>
		api.patch(`/hosts/${hostId}/auto-update`, { auto_update: autoUpdate }),
	toggleHostDownAlerts: (hostId, enabled) =>
		api.patch(`/hosts/${hostId}/host-down-alerts`, {
			host_down_alerts_enabled: enabled,
		}),
	forceAgentUpdate: (hostId) => api.post(`/hosts/${hostId}/force-agent-update`),
	refreshIntegrationStatus: (hostId) =>
		api.post(`/hosts/${hostId}/refresh-integration-status`),
	fetchReport: (hostId) => api.post(`/hosts/${hostId}/fetch-report`),
	fetchReportBulk: (hostIds) =>
		api.post("/hosts/bulk/fetch-report", { hostIds }),
	updateFriendlyName: (hostId, friendlyName) =>
		api.patch(`/hosts/${hostId}/friendly-name`, {
			friendly_name: friendlyName,
		}),
	updateConnection: (hostId, connectionInfo) =>
		api.patch(`/hosts/${hostId}/connection`, connectionInfo),
	setPrimaryInterface: (hostId, interfaceName) =>
		api.patch(`/hosts/${hostId}/primary-interface`, {
			interface_name: interfaceName,
		}),
	updateNotes: (hostId, notes) =>
		api.patch(`/hosts/${hostId}/notes`, {
			notes: notes,
		}),
	getIntegrations: (hostId) => api.get(`/hosts/${hostId}/integrations`),
	toggleIntegration: (hostId, integrationName, enabled) =>
		api.post(`/hosts/${hostId}/integrations/${integrationName}/toggle`, {
			enabled,
		}),
	getIntegrationSetupStatus: (hostId, integrationName) =>
		api.get(`/hosts/${hostId}/integrations/${integrationName}/status`),
	requestComplianceStatus: (hostId) =>
		api.post(`/hosts/${hostId}/integrations/compliance/request-status`),
	refreshDocker: (hostId) => api.post(`/hosts/${hostId}/refresh-docker`),
	setComplianceMode: (hostId, mode) =>
		api.post(`/hosts/${hostId}/integrations/compliance/mode`, {
			mode: mode,
		}),
	setComplianceScanners: (hostId, settings) =>
		api.post(`/hosts/${hostId}/integrations/compliance/scanners`, settings),
	applyPendingConfig: (hostId) =>
		api.post(`/hosts/${hostId}/integrations/apply-pending-config`),
	setComplianceOnDemandOnly: (hostId, onDemandOnly) =>
		api.post(`/hosts/${hostId}/compliance/on-demand-only`, {
			on_demand_only: onDemandOnly,
		}),
};

// Host Groups API
export const hostGroupsAPI = {
	list: () => api.get("/host-groups"),
	get: (id) => api.get(`/host-groups/${id}`),
	create: (data) => api.post("/host-groups", data),
	update: (id, data) => api.put(`/host-groups/${id}`, data),
	delete: (id) => api.delete(`/host-groups/${id}`),
	getHosts: (id) => api.get(`/host-groups/${id}/hosts`),
};

// Admin Users API (for user management)
export const adminUsersAPI = {
	list: () => api.get("/auth/admin/users"),
	listForAssignment: () => api.get("/auth/users/for-assignment"), // Public endpoint for assignment dropdowns
	create: (userData) => api.post("/auth/admin/users", userData),
	update: (userId, userData) =>
		api.put(`/auth/admin/users/${userId}`, userData),
	delete: (userId) => api.delete(`/auth/admin/users/${userId}`),
	resetPassword: (userId, newPassword) =>
		api.post(`/auth/admin/users/${userId}/reset-password`, { newPassword }),
};

// Permissions API (for role management)
export const permissionsAPI = {
	getRoles: () => api.get("/permissions/roles"),
	getRole: (role) => api.get(`/permissions/roles/${role}`),
	updateRole: (role, permissions) =>
		api.put(`/permissions/roles/${role}`, permissions),
	deleteRole: (role) => api.delete(`/permissions/roles/${role}`),
	getUserPermissions: () => api.get("/permissions/user-permissions"),
};

// Settings API
export const settingsAPI = {
	get: () => api.get("/settings"),
	getPublic: () => api.get("/settings/public"), // Public endpoint for read-only settings (auto_update, etc.)
	update: (settings) => api.put("/settings", settings),
	getServerUrl: () => api.get("/settings/server-url"),
	getCurrentUrl: () => api.get("/settings/current-url"),
	getEnvConfig: () => api.get("/settings/env-config"),
	getEnvironmentConfig: () => api.get("/settings/environment"),
	updateEnvironmentConfig: (key, value) =>
		api.patch(`/settings/environment/${key}`, { value }),
};

// Community links API (public - used in nav, login, wizard)
export const communityAPI = {
	getLinks: () => api.get("/community/links").then((res) => res.data),
};

// Marketing API (public - used during first-time setup)
export const marketingAPI = {
	subscribe: (data) =>
		fetch("/api/v1/marketing/subscribe", {
			method: "POST",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify(data),
		}).then((res) => {
			if (!res.ok) {
				return res.json().then((d) => {
					throw new Error(d.error || "Subscribe failed");
				});
			}
			return res.json();
		}),
};

// User Preferences API
export const userPreferencesAPI = {
	get: () => api.get("/user/preferences"),
	update: (preferences) => api.patch("/user/preferences", preferences),
};

// Agent File Management API
export const agentFileAPI = {
	getInfo: () => api.get("/hosts/agent/info"),
	download: () => api.get("/hosts/agent/download", { responseType: "blob" }),
};

// Repository API
export const repositoryAPI = {
	list: (params = {}) => api.get("/repositories", { params }),
	getById: (repositoryId) => api.get(`/repositories/${repositoryId}`),
	getByHost: (hostId) => api.get(`/repositories/host/${hostId}`),
	update: (repositoryId, data) =>
		api.put(`/repositories/${repositoryId}`, data),
	delete: (repositoryId) => api.delete(`/repositories/${repositoryId}`),
	toggleHostRepository: (hostId, repositoryId, isEnabled) =>
		api.patch(`/repositories/host/${hostId}/repository/${repositoryId}`, {
			isEnabled,
		}),
	getStats: () => api.get("/repositories/stats/summary"),
	cleanupOrphaned: () => api.delete("/repositories/cleanup/orphaned"),
};

// Dashboard Preferences API
export const dashboardPreferencesAPI = {
	get: () => api.get("/dashboard-preferences"),
	update: (preferences) => api.put("/dashboard-preferences", { preferences }),
	getDefaults: () => api.get("/dashboard-preferences/defaults"),
	getLayout: () => api.get("/dashboard-preferences/layout"),
	updateLayout: (layout) => api.put("/dashboard-preferences/layout", layout),
};

// Hosts API (for agent communication - kept for compatibility)
export const hostsAPI = {
	// Legacy register endpoint (now deprecated)
	register: (data) => api.post("/hosts/register", data),

	// Updated to use API credentials
	update: (apiId, apiKey, data) =>
		api.post("/hosts/update", data, {
			headers: {
				"X-API-ID": apiId,
				"X-API-KEY": apiKey,
			},
		}),
	getInfo: (apiId, apiKey) =>
		api.get("/hosts/info", {
			headers: {
				"X-API-ID": apiId,
				"X-API-KEY": apiKey,
			},
		}),
	ping: (apiId, apiKey) =>
		api.post(
			"/hosts/ping",
			{},
			{
				headers: {
					"X-API-ID": apiId,
					"X-API-KEY": apiKey,
				},
			},
		),
	toggleAutoUpdate: (id, autoUpdate) =>
		api.patch(`/hosts/${id}/auto-update`, { auto_update: autoUpdate }),
};

// Packages API
export const packagesAPI = {
	getAll: (params = {}) => api.get("/packages", { params }),
	getById: (packageId) => api.get(`/packages/${packageId}`),
	getCategories: () => api.get("/packages/categories/list"),
	getHosts: (packageId, params = {}) =>
		api.get(`/packages/${packageId}/hosts`, { params }),
	getActivity: (packageId, params = {}) =>
		api.get(`/packages/${packageId}/activity`, { params }).then((r) => r.data),
	update: (packageId, data) => api.put(`/packages/${packageId}`, data),
};

// Utility functions
export const isCorsError = (error) => {
	// Check for browser-level CORS errors (when request is blocked before reaching server)
	if (error.message?.includes("Failed to fetch") && !error.response) {
		return true;
	}

	// Check for TypeError with Failed to fetch (common CORS error pattern)
	if (
		error.name === "TypeError" &&
		error.message?.includes("Failed to fetch")
	) {
		return true;
	}

	// Check for server CORS errors that get converted to 500 by proxy
	if (error.response?.status === 500) {
		// Check if the error message contains CORS-related text
		if (
			error.message?.includes("Not allowed by CORS") ||
			error.message?.includes("CORS") ||
			error.message?.includes("cors")
		) {
			return true;
		}

		// Check if the response data contains CORS error information
		if (
			error.response?.data?.error?.includes("CORS") ||
			error.response?.data?.error?.includes("cors") ||
			error.response?.data?.message?.includes("CORS") ||
			error.response?.data?.message?.includes("cors") ||
			error.response?.data?.message?.includes("Not allowed by CORS")
		) {
			return true;
		}

		// Check for specific CORS error patterns from server logs
		if (
			error.message?.includes("origin") &&
			error.message?.includes("callback")
		) {
			return true;
		}

		// Check if this is likely a CORS error based on context
		// If we're accessing from localhost but CORS_ORIGIN is set to fabio, this is likely CORS
		const currentOrigin = window.location.origin;
		if (
			currentOrigin === "http://localhost:3000" &&
			error.config?.url?.includes("/api/")
		) {
			// This is likely a CORS error when accessing from localhost
			return true;
		}
	}

	// Check for CORS-related errors
	return (
		error.message?.includes("CORS") ||
		error.message?.includes("cors") ||
		error.message?.includes("Access to fetch") ||
		error.message?.includes("blocked by CORS policy") ||
		error.message?.includes("Cross-Origin Request Blocked") ||
		error.message?.includes("NetworkError when attempting to fetch resource") ||
		error.message?.includes("ERR_BLOCKED_BY_CLIENT") ||
		error.message?.includes("ERR_NETWORK") ||
		error.message?.includes("ERR_CONNECTION_REFUSED")
	);
};

export const formatError = (error) => {
	// Check for CORS-related errors
	if (isCorsError(error)) {
		return "CORS_ORIGIN mismatch - please set your URL in your environment variable";
	}

	if (error.response?.data?.message) {
		return error.response.data.message;
	}
	if (error.response?.data?.error) {
		return error.response.data.error;
	}
	if (error.message) {
		return error.message;
	}
	return "An unexpected error occurred";
};

/**
 * Module-level timezone used by all date formatting helpers.
 * Set once via setGlobalTimezone() when settings load.
 */
let _globalTimezone = null;

/** Set the global IANA timezone (e.g. "Europe/London"). */
export const setGlobalTimezone = (tz) => {
	_globalTimezone = tz;
};

/** Get the current global timezone (or null). */
export const getGlobalTimezone = () => _globalTimezone;

/**
 * Format a date for display. When timezone is provided (e.g. from settings.timezone),
 * formats in that IANA timezone; otherwise falls back to the global timezone,
 * then browser locale.
 * @param {string|Date|number} date - ISO string, Date, or timestamp
 * @param {string} [timezone] - Optional IANA timezone (e.g. America/New_York)
 */
export const formatDate = (date, timezone) => {
	const d = new Date(date);
	if (Number.isNaN(d.getTime())) return " -";
	const tz = timezone || _globalTimezone;
	if (tz) {
		try {
			return new Intl.DateTimeFormat(undefined, {
				timeZone: tz,
				dateStyle: "short",
				timeStyle: "medium",
			}).format(d);
		} catch {
			return d.toLocaleString();
		}
	}
	return d.toLocaleString();
};

/**
 * Format a date for display (date only, no time).
 * Uses the global timezone when no explicit timezone is passed.
 * @param {string|Date|number} date - ISO string, Date, or timestamp
 * @param {string} [timezone] - Optional IANA timezone
 */
export const formatDateOnly = (date, timezone) => {
	const d = new Date(date);
	if (Number.isNaN(d.getTime())) return " -";
	const tz = timezone || _globalTimezone;
	if (tz) {
		try {
			return new Intl.DateTimeFormat(undefined, {
				timeZone: tz,
				dateStyle: "short",
			}).format(d);
		} catch {
			return d.toLocaleDateString();
		}
	}
	return d.toLocaleDateString();
};

// Version API
export const versionAPI = {
	getCurrent: () => api.get("/version/current"),
	checkUpdates: () => api.get("/version/check-updates"),
	testSshKey: (data) => api.post("/version/test-ssh-key", data),
};

// Agent Version API (Settings > Agent Version)
export const agentVersionAPI = {
	getInfo: () => api.get("/agent/version"),
	checkUpdates: () => api.post("/agent/version/check"),
	refresh: () => api.post("/agent/version/refresh"),
	download: (arch, os) =>
		api.get("/agent/download", { params: { arch, os }, responseType: "blob" }),
};

// RDP API (in-browser RDP for Windows hosts via guacd)
export const rdpAPI = {
	createTicket: (data) => api.post("/auth/rdp-ticket", data),
};

// Auth API
export const authAPI = {
	login: (username, password) =>
		api.post("/auth/login", { username, password }),
	verifyTfa: (username, token, remember_me = false) =>
		api.post("/auth/verify-tfa", { username, token, remember_me }),
	signup: (username, email, password, firstName, lastName) =>
		api.post("/auth/signup", {
			username,
			email,
			password,
			firstName,
			lastName,
		}),
};

// TFA API
export const tfaAPI = {
	setup: () => api.get("/tfa/setup"),
	verifySetup: (data) => api.post("/tfa/verify-setup", data),
	disable: (data) => api.post("/tfa/disable", data),
	status: () => api.get("/tfa/status"),
	regenerateBackupCodes: () => api.post("/tfa/regenerate-backup-codes"),
	verify: (data) => api.post("/tfa/verify", data),
};

export const formatRelativeTime = (date) => {
	if (date == null) return " -";
	const now = new Date();
	const diff = now - new Date(date);
	const abs = Math.abs(diff);
	const future = diff < 0;
	const seconds = Math.floor(abs / 1000);
	const minutes = Math.floor(seconds / 60);
	const hours = Math.floor(minutes / 60);
	const days = Math.floor(hours / 24);

	const suffix = future ? "" : " ago";
	const prefix = future ? "in " : "";
	if (days > 0) return `${prefix}${days} day${days > 1 ? "s" : ""}${suffix}`;
	if (hours > 0)
		return `${prefix}${hours} hour${hours > 1 ? "s" : ""}${suffix}`;
	if (minutes > 0) return `${prefix}${minutes} min${suffix}`;
	if (future) return "in a few seconds";
	return "just now";
};

// Search API
export const searchAPI = {
	global: (query, config = {}) =>
		api.get("/search", { params: { q: query }, ...config }),
};

// AI Terminal Assistant API
export const aiAPI = {
	getStatus: () => api.get("/ai/status"), // Available to all authenticated users
	getProviders: () => api.get("/ai/providers"),
	getSettings: () => api.get("/ai/settings"), // Admin only
	updateSettings: (data) => api.put("/ai/settings", data),
	testConnection: () => api.post("/ai/test"),
	assist: (data) => api.post("/ai/assist", data),
	complete: (data) => api.post("/ai/complete", data),
};

// Discord OAuth API
export const discordAPI = {
	getConfig: () => api.get("/auth/discord/config"),
	getSettings: () => api.get("/auth/discord/settings"),
	updateSettings: (data) => api.put("/auth/discord/settings", data),
	link: () => api.post("/auth/discord/link"),
	unlink: () => api.post("/auth/discord/unlink"),
};

// OIDC / SSO API
export const oidcAPI = {
	getSettings: () => api.get("/auth/oidc/settings"),
	updateSettings: (data) => api.put("/auth/oidc/settings", data),
	importFromEnv: () => api.post("/auth/oidc/settings/import-from-env"),
};

// Alerts API
export const alertsAPI = {
	getAlerts: (params = {}) => {
		const queryString = new URLSearchParams(params).toString();
		const url = `/alerts${queryString ? `?${queryString}` : ""}`;
		return api.get(url);
	},
	getAlertStats: () => api.get("/alerts/stats"),
	getAlert: (id) => api.get(`/alerts/${id}`),
	getAlertHistory: (id) => api.get(`/alerts/${id}/history`),
	getAvailableActions: () => api.get("/alerts/actions"),
	performAlertAction: (id, action, metadata = null) =>
		api.post(`/alerts/${id}/action`, { action, metadata }),
	assignAlert: (id, userId) => api.post(`/alerts/${id}/assign`, { userId }),
	unassignAlert: (id) => api.post(`/alerts/${id}/unassign`),
	resolveAlert: (id) => api.put(`/alerts/${id}/resolve`),
	getAlertConfig: () => api.get("/alerts/config"),
	getAlertConfigByType: (alertType) => api.get(`/alerts/config/${alertType}`),
	updateAlertConfig: (alertType, config) =>
		api.put(`/alerts/config/${alertType}`, config),
	bulkUpdateAlertConfig: (configs) =>
		api.post("/alerts/config/bulk-update", { configs }),
	previewCleanup: () => api.get("/alerts/cleanup/preview"),
	triggerCleanup: () => api.post("/alerts/cleanup"),
	deleteAlert: (id) => api.delete(`/alerts/${id}`),
	bulkDeleteAlerts: (alertIds) => api.post("/alerts/bulk-delete", { alertIds }),
	bulkAction: (alertIds, action) =>
		api.post("/alerts/bulk-action", { alertIds, action }),
};

// Notifications (webhooks, email, scheduled reports)
export const notificationsAPI = {
	listDestinations: () => api.get("/notifications/destinations"),
	createDestination: (data) => api.post("/notifications/destinations", data),
	updateDestination: (id, data) =>
		api.put(`/notifications/destinations/${id}`, data),
	getDestinationConfig: (id) =>
		api.get(`/notifications/destinations/${id}/config`),
	deleteDestination: (id) => api.delete(`/notifications/destinations/${id}`),
	listRoutes: () => api.get("/notifications/routes"),
	createRoute: (data) => api.post("/notifications/routes", data),
	updateRoute: (id, data) => api.put(`/notifications/routes/${id}`, data),
	deleteRoute: (id) => api.delete(`/notifications/routes/${id}`),
	listDeliveryLog: (params = {}) =>
		api.get("/notifications/delivery-log", { params }),
	test: (data) => api.post("/notifications/test", data),
	listScheduledReports: () => api.get("/notifications/scheduled-reports"),
	createScheduledReport: (data) =>
		api.post("/notifications/scheduled-reports", data),
	updateScheduledReport: (id, data) =>
		api.put(`/notifications/scheduled-reports/${id}`, data),
	deleteScheduledReport: (id) =>
		api.delete(`/notifications/scheduled-reports/${id}`),
	runScheduledReportNow: (id) =>
		api.post(`/notifications/scheduled-reports/${id}/run-now`),
};

export default api;
