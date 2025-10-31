import {
	createContext,
	useCallback,
	useContext,
	useEffect,
	useState,
} from "react";
import { flushSync } from "react-dom";
import { AUTH_PHASES, isAuthPhase } from "../constants/authPhases";
import { isCorsError } from "../utils/api";

const AuthContext = createContext();

export const useAuth = () => {
	const context = useContext(AuthContext);
	if (!context) {
		throw new Error("useAuth must be used within an AuthProvider");
	}
	return context;
};

export const AuthProvider = ({ children }) => {
	const [user, setUser] = useState(null);
	const [token, setToken] = useState(null);
	const [permissions, setPermissions] = useState(null);
	const [needsFirstTimeSetup, setNeedsFirstTimeSetup] = useState(false);

	// Authentication state machine phases
	const [authPhase, setAuthPhase] = useState(AUTH_PHASES.INITIALISING);
	const [permissionsLoading, setPermissionsLoading] = useState(false);

	// Define functions first
	const fetchPermissions = useCallback(async (authToken) => {
		try {
			setPermissionsLoading(true);
			const response = await fetch("/api/v1/permissions/user-permissions", {
				headers: {
					Authorization: `Bearer ${authToken}`,
				},
			});

			if (response.ok) {
				const data = await response.json();
				setPermissions(data);
				return data;
			} else {
				console.error("Failed to fetch permissions");
				return null;
			}
		} catch (error) {
			console.error("Error fetching permissions:", error);
			return null;
		} finally {
			setPermissionsLoading(false);
		}
	}, []);

	const refreshPermissions = useCallback(async () => {
		if (token) {
			const updatedPermissions = await fetchPermissions(token);
			return updatedPermissions;
		}
		return null;
	}, [token, fetchPermissions]);

	// Initialize auth state from localStorage
	useEffect(() => {
		const storedToken = localStorage.getItem("token");
		const storedUser = localStorage.getItem("user");

		if (storedToken && storedUser) {
			try {
				setToken(storedToken);
				setUser(JSON.parse(storedUser));
				// Fetch permissions from backend
				fetchPermissions(storedToken);
				// User is authenticated, skip setup check
				setAuthPhase(AUTH_PHASES.READY);
			} catch (error) {
				console.error("Error parsing stored user data:", error);
				localStorage.removeItem("token");
				localStorage.removeItem("user");
				// Move to setup check phase
				setAuthPhase(AUTH_PHASES.CHECKING_SETUP);
			}
		} else {
			// No stored auth, check if setup is needed
			setAuthPhase(AUTH_PHASES.CHECKING_SETUP);
		}
	}, [fetchPermissions]);

	const login = async (username, password) => {
		try {
			// Get or generate device ID for TFA remember-me
			let deviceId = localStorage.getItem("device_id");
			if (!deviceId) {
				if (typeof crypto !== "undefined" && crypto.randomUUID) {
					deviceId = crypto.randomUUID();
				} else {
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

			const response = await fetch("/api/v1/auth/login", {
				method: "POST",
				headers: {
					"Content-Type": "application/json",
					"X-Device-ID": deviceId,
				},
				body: JSON.stringify({ username, password }),
			});

			const data = await response.json();

			if (response.ok) {
				// Check if TFA is required
				if (data.requiresTfa) {
					return { success: true, requiresTfa: true };
				}

				// Regular successful login
				setToken(data.token);
				setUser(data.user);
				localStorage.setItem("token", data.token);
				localStorage.setItem("user", JSON.stringify(data.user));

				// Fetch user permissions after successful login
				const userPermissions = await fetchPermissions(data.token);
				if (userPermissions) {
					setPermissions(userPermissions);
				}

				return { success: true };
			} else {
				// Handle HTTP error responses (like 500 CORS errors)
				console.log("HTTP error response:", response.status, data);

				// Check if this is a CORS error based on the response data
				if (
					data.message?.includes("Not allowed by CORS") ||
					data.message?.includes("CORS") ||
					data.error?.includes("CORS")
				) {
					return {
						success: false,
						error:
							"CORS_ORIGIN mismatch - please set your URL in your environment variable",
					};
				}

				return { success: false, error: data.error || "Login failed" };
			}
		} catch (error) {
			console.log("Login error:", error);
			console.log("Error response:", error.response);
			console.log("Error message:", error.message);

			// Check for CORS/network errors first
			if (isCorsError(error)) {
				return {
					success: false,
					error:
						"CORS_ORIGIN mismatch - please set your URL in your environment variable",
				};
			}

			// Check for other network errors
			if (
				error.name === "TypeError" &&
				error.message?.includes("Failed to fetch")
			) {
				return {
					success: false,
					error:
						"CORS_ORIGIN mismatch - please set your URL in your environment variable",
				};
			}

			return { success: false, error: "Network error occurred" };
		}
	};

	const logout = async () => {
		try {
			if (token) {
				await fetch("/api/v1/auth/logout", {
					method: "POST",
					headers: {
						Authorization: `Bearer ${token}`,
						"Content-Type": "application/json",
					},
				});
			}
		} catch (error) {
			console.error("Logout error:", error);
		} finally {
			setToken(null);
			setUser(null);
			setPermissions(null);
			localStorage.removeItem("token");
			localStorage.removeItem("user");
		}
	};

	const updateProfile = async (profileData) => {
		try {
			const response = await fetch("/api/v1/auth/profile", {
				method: "PUT",
				headers: {
					Authorization: `Bearer ${token}`,
					"Content-Type": "application/json",
				},
				body: JSON.stringify(profileData),
			});

			const data = await response.json();

			if (response.ok) {
				setUser(data.user);
				localStorage.setItem("user", JSON.stringify(data.user));
				return { success: true, user: data.user };
			} else {
				// Handle HTTP error responses (like 500 CORS errors)
				console.log("HTTP error response:", response.status, data);

				// Check if this is a CORS error based on the response data
				if (
					data.message?.includes("Not allowed by CORS") ||
					data.message?.includes("CORS") ||
					data.error?.includes("CORS")
				) {
					return {
						success: false,
						error:
							"CORS_ORIGIN mismatch - please set your URL in your environment variable",
					};
				}

				return { success: false, error: data.error || "Update failed" };
			}
		} catch (error) {
			// Check for CORS/network errors first
			if (isCorsError(error)) {
				return {
					success: false,
					error:
						"CORS_ORIGIN mismatch - please set your URL in your environment variable",
				};
			}

			// Check for other network errors
			if (
				error.name === "TypeError" &&
				error.message?.includes("Failed to fetch")
			) {
				return {
					success: false,
					error:
						"CORS_ORIGIN mismatch - please set your URL in your environment variable",
				};
			}

			return { success: false, error: "Network error occurred" };
		}
	};

	const changePassword = async (currentPassword, newPassword) => {
		try {
			const response = await fetch("/api/v1/auth/change-password", {
				method: "PUT",
				headers: {
					Authorization: `Bearer ${token}`,
					"Content-Type": "application/json",
				},
				body: JSON.stringify({ currentPassword, newPassword }),
			});

			const data = await response.json();

			if (response.ok) {
				return { success: true };
			} else {
				// Handle HTTP error responses (like 500 CORS errors)
				console.log("HTTP error response:", response.status, data);

				// Check if this is a CORS error based on the response data
				if (
					data.message?.includes("Not allowed by CORS") ||
					data.message?.includes("CORS") ||
					data.error?.includes("CORS")
				) {
					return {
						success: false,
						error:
							"CORS_ORIGIN mismatch - please set your URL in your environment variable",
					};
				}

				return {
					success: false,
					error: data.error || "Password change failed",
				};
			}
		} catch (error) {
			// Check for CORS/network errors first
			if (isCorsError(error)) {
				return {
					success: false,
					error:
						"CORS_ORIGIN mismatch - please set your URL in your environment variable",
				};
			}

			// Check for other network errors
			if (
				error.name === "TypeError" &&
				error.message?.includes("Failed to fetch")
			) {
				return {
					success: false,
					error:
						"CORS_ORIGIN mismatch - please set your URL in your environment variable",
				};
			}

			return { success: false, error: "Network error occurred" };
		}
	};

	const isAdmin = () => {
		return user?.role === "admin";
	};

	// Permission checking functions
	const hasPermission = (permission) => {
		// If permissions are still loading, return false to show loading state
		if (permissionsLoading) {
			return false;
		}
		return permissions?.[permission] === true;
	};

	const canViewDashboard = () => hasPermission("can_view_dashboard");
	const canViewHosts = () => hasPermission("can_view_hosts");
	const canManageHosts = () => hasPermission("can_manage_hosts");
	const canViewPackages = () => hasPermission("can_view_packages");
	const canManagePackages = () => hasPermission("can_manage_packages");
	const canViewUsers = () => hasPermission("can_view_users");
	const canManageUsers = () => hasPermission("can_manage_users");
	const canViewReports = () => hasPermission("can_view_reports");
	const canExportData = () => hasPermission("can_export_data");
	const canManageSettings = () => hasPermission("can_manage_settings");

	// Check if any admin users exist (for first-time setup)
	const checkAdminUsersExist = useCallback(async () => {
		try {
			const response = await fetch("/api/v1/auth/check-admin-users", {
				method: "GET",
				headers: {
					"Content-Type": "application/json",
				},
			});

			if (response.ok) {
				const data = await response.json();
				setNeedsFirstTimeSetup(!data.hasAdminUsers);
				setAuthPhase(AUTH_PHASES.READY); // Setup check complete, move to ready phase
			} else {
				// If endpoint doesn't exist or fails, assume setup is needed
				setNeedsFirstTimeSetup(true);
				setAuthPhase(AUTH_PHASES.READY);
			}
		} catch (error) {
			console.error("Error checking admin users:", error);
			// If there's an error, assume setup is needed
			setNeedsFirstTimeSetup(true);
			setAuthPhase(AUTH_PHASES.READY);
		}
	}, []);

	// Check for admin users ONLY when in CHECKING_SETUP phase
	useEffect(() => {
		if (isAuthPhase.checkingSetup(authPhase)) {
			checkAdminUsersExist();
		}
	}, [authPhase, checkAdminUsersExist]);

	const setAuthState = (authToken, authUser) => {
		// Use flushSync to ensure all state updates are applied synchronously
		flushSync(() => {
			setToken(authToken);
			setUser(authUser);
			setNeedsFirstTimeSetup(false);
			setAuthPhase(AUTH_PHASES.READY);
		});

		// Store in localStorage after state is updated
		localStorage.setItem("token", authToken);
		localStorage.setItem("user", JSON.stringify(authUser));

		// Fetch permissions immediately for the new authenticated user
		fetchPermissions(authToken);
	};

	// Computed loading state based on phase and permissions state
	const isLoading = !isAuthPhase.ready(authPhase) || permissionsLoading;

	// Function to check authentication status (maintains API compatibility)
	const isAuthenticated = () => {
		return !!(user && token && isAuthPhase.ready(authPhase));
	};

	const value = {
		user,
		token,
		permissions,
		isLoading,
		needsFirstTimeSetup,
		authPhase,
		login,
		logout,
		updateProfile,
		changePassword,
		refreshPermissions,
		setAuthState,
		isAuthenticated,
		isAdmin,
		hasPermission,
		canViewDashboard,
		canViewHosts,
		canManageHosts,
		canViewPackages,
		canManagePackages,
		canViewUsers,
		canManageUsers,
		canViewReports,
		canExportData,
		canManageSettings,
	};

	return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
};
