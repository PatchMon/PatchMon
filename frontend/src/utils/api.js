import axios from "axios";

const API_BASE_URL = import.meta.env.VITE_API_URL || "/api/v1";

// Create axios instance with default config
const api = axios.create({
	baseURL: API_BASE_URL,
	timeout: 10000, // 10 seconds
	headers: {
		"Content-Type": "application/json",
	},
});

// Request interceptor
api.interceptors.request.use(
	(config) => {
		// Add auth token if available
		const token = localStorage.getItem("token");
		if (token) {
			config.headers.Authorization = `Bearer ${token}`;
		}

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
			// Don't redirect if we're on the login page or if it's a TFA verification error
			const currentPath = window.location.pathname;
			const isTfaError = error.config?.url?.includes("/verify-tfa");

			if (currentPath !== "/login" && !isTfaError) {
				// Handle unauthorized
				localStorage.removeItem("token");
				localStorage.removeItem("user");
				window.location.href = "/login";
			}
		}
		return Promise.reject(error);
	},
);

// Dashboard API
export const dashboardAPI = {
	getStats: () => api.get("/dashboard/stats"),
	getHosts: () => api.get("/dashboard/hosts"),
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
	forceAgentUpdate: (hostId) => api.post(`/hosts/${hostId}/force-agent-update`),
	fetchReport: (hostId) => api.post(`/hosts/${hostId}/fetch-report`),
	updateFriendlyName: (hostId, friendlyName) =>
		api.patch(`/hosts/${hostId}/friendly-name`, {
			friendly_name: friendlyName,
		}),
	updateNotes: (hostId, notes) =>
		api.patch(`/hosts/${hostId}/notes`, {
			notes: notes,
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
	update: (settings) => api.put("/settings", settings),
	getServerUrl: () => api.get("/settings/server-url"),
};

// User Preferences API
export const userPreferencesAPI = {
	get: () => api.get("/user/preferences"),
	update: (preferences) => api.patch("/user/preferences", preferences),
};

// Agent File Management API
export const agentFileAPI = {
	getInfo: () => api.get("/hosts/agent/info"),
	upload: (scriptContent) => api.post("/hosts/agent/upload", { scriptContent }),
	download: () => api.get("/hosts/agent/download", { responseType: "blob" }),
};

// Repository API
export const repositoryAPI = {
	list: () => api.get("/repositories"),
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
	update: (packageId, data) => api.put(`/packages/${packageId}`, data),
	search: (query, params = {}) =>
		api.get(`/packages/search/${query}`, { params }),
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

	// Check for backend CORS errors that get converted to 500 by proxy
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

		// Check for specific CORS error patterns from backend logs
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

export const formatDate = (date) => {
	return new Date(date).toLocaleString();
};

// Version API
export const versionAPI = {
	getCurrent: () => api.get("/version/current"),
	checkUpdates: () => api.get("/version/check-updates"),
	testSshKey: (data) => api.post("/version/test-ssh-key", data),
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
	const now = new Date();
	const diff = now - new Date(date);
	const seconds = Math.floor(diff / 1000);
	const minutes = Math.floor(seconds / 60);
	const hours = Math.floor(minutes / 60);
	const days = Math.floor(hours / 24);

	if (days > 0) return `${days} day${days > 1 ? "s" : ""} ago`;
	if (hours > 0) return `${hours} hour${hours > 1 ? "s" : ""} ago`;
	if (minutes > 0) return `${minutes} minute${minutes > 1 ? "s" : ""} ago`;
	return `${seconds} second${seconds > 1 ? "s" : ""} ago`;
};

// Search API
export const searchAPI = {
	global: (query) => api.get("/search", { params: { q: query } }),
};

export default api;
