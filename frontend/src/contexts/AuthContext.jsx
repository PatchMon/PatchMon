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

// Development-only logging to prevent error details exposure in production
const isDev = import.meta.env.DEV;
const devLog = (...args) => isDev && console.log(...args);
const _devError = (...args) => isDev && console.error(...args);

export const AuthContext = createContext();

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
			const fetchOptions = {
				credentials: "include", // Include cookies for httpOnly token
			};
			// Add Authorization header if token provided (backward compatibility)
			if (authToken) {
				fetchOptions.headers = {
					Authorization: `Bearer ${authToken}`,
				};
			}
			const response = await fetch(
				"/api/v1/permissions/user-permissions",
				fetchOptions,
			);

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
		// Use token from state or rely on cookies
		const updatedPermissions = await fetchPermissions(token);
		return updatedPermissions;
	}, [token, fetchPermissions]);

	// Initialize auth state - validate session via API (cookies) or localStorage
	useEffect(() => {
		const abortController = new AbortController();

		const validateSession = async () => {
			try {
				// First, try to validate via API using httpOnly cookies
				const response = await fetch("/api/v1/auth/profile", {
					credentials: "include",
					signal: abortController.signal,
				});

				if (response.ok) {
					const data = await response.json();
					if (!abortController.signal.aborted) {
						setUser(data.user);
						// Fetch permissions
						await fetchPermissions();
						setAuthPhase(AUTH_PHASES.READY);
					}
					return;
				} else {
					const errorData = await response.json().catch(() => ({}));
					console.error(
						"🔐 [AUTH] Profile validation failed:",
						response.status,
						errorData,
					);
				}
			} catch (error) {
				if (error.name === "AbortError") return;
				console.error("🔐 [AUTH] Profile validation exception:", error);
				devLog("Cookie-based auth failed:", error.message);
			}

			if (abortController.signal.aborted) return;

			// Clean up any stale token from localStorage (security measure)
			localStorage.removeItem("token");

			// No valid session, check if setup is needed
			setAuthPhase(AUTH_PHASES.CHECKING_SETUP);
		};

		validateSession();

		return () => abortController.abort();
	}, [fetchPermissions]);

	const login = async (username, password) => {
		try {
			console.log("🔐 [AUTH] Login attempt started for:", username);

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

			console.log("🔐 [AUTH] Sending login request to /api/v1/auth/login");
			const response = await fetch("/api/v1/auth/login", {
				method: "POST",
				headers: {
					"Content-Type": "application/json",
					"X-Device-ID": deviceId,
				},
				credentials: "include", // Include cookies for httpOnly token
				body: JSON.stringify({ username, password }),
			});

			console.log("🔐 [AUTH] Login response status:", response.status);
			console.log(
				"🔐 [AUTH] Login response headers:",
				Object.fromEntries(response.headers.entries()),
			);

			const data = await response.json();
			console.log("🔐 [AUTH] Login response data:", {
				message: data.message,
				hasToken: !!data.token,
				hasUser: !!data.user,
				requiresTfa: data.requiresTfa,
			});

			if (response.ok) {
				console.log("🔐 [AUTH] Login successful!");

				// Check if TFA is required
				if (data.requiresTfa) {
					console.log("🔐 [AUTH] TFA required");
					return { success: true, requiresTfa: true };
				}

				// Regular successful login
				// Note: httpOnly cookies are set by the server for secure auth
				// localStorage is used for backward compatibility and UI state only
				console.log("🔐 [AUTH] Setting user state and token");
				setToken(data.token);
				setUser({
					...data.user,
					accepted_release_notes_versions:
						data.user.accepted_release_notes_versions || [],
				});
				// Store user info for session recovery (token stored in httpOnly cookie by server)
				// Note: Token is NOT stored in localStorage to prevent XSS attacks
				// The httpOnly cookie set by the server is used for authentication
				localStorage.setItem(
					"user",
					JSON.stringify({
						...data.user,
						accepted_release_notes_versions:
							data.user.accepted_release_notes_versions || [],
					}),
				);

				// Fetch user permissions after successful login
				console.log("🔐 [AUTH] Fetching user permissions...");
				const userPermissions = await fetchPermissions(data.token);
				if (userPermissions) {
					console.log("🔐 [AUTH] Permissions fetched:", userPermissions);
					setPermissions(userPermissions);
				} else {
					console.warn("🔐 [AUTH] No permissions returned");
				}

				// Note: User preferences will be automatically fetched by ColorThemeContext
				// when the component mounts, so no need to invalidate here

				console.log("🔐 [AUTH] Login complete, returning success");
				return { success: true };
			} else {
				console.error("🔐 [AUTH] Login failed with status:", response.status);
				// Handle HTTP error responses (like 500 CORS errors)
				devLog("HTTP error response:", response.status, data);

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
			console.error("🔐 [AUTH] Login exception:", error);
			devLog("Login error:", error);
			devLog("Error response:", error.response);
			devLog("Error message:", error.message);

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
			// Logout via API - server will clear httpOnly cookies
			await fetch("/api/v1/auth/logout", {
				method: "POST",
				credentials: "include", // Include cookies for auth
				headers: {
					...(token ? { Authorization: `Bearer ${token}` } : {}),
					"Content-Type": "application/json",
				},
			});
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
				// Validate that we received user data with expected fields
				if (!data.user || !data.user.id) {
					console.error("Invalid user data in response:", data);
					return {
						success: false,
						error: "Invalid response from server",
					};
				}

				// Update both state and localStorage atomically
				setUser({
					...data.user,
					accepted_release_notes_versions:
						data.user.accepted_release_notes_versions || [],
				});
				localStorage.setItem(
					"user",
					JSON.stringify({
						...data.user,
						accepted_release_notes_versions:
							data.user.accepted_release_notes_versions || [],
					}),
				);

				return { success: true, user: data.user };
			} else {
				// Handle HTTP error responses (like 500 CORS errors)
				devLog("HTTP error response:", response.status, data);

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
				devLog("HTTP error response:", response.status, data);

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

	const acceptReleaseNotes = async (version) => {
		try {
			const headers = {
				"Content-Type": "application/json",
			};
			// Only add Authorization header if token exists (not OIDC)
			if (token) {
				headers.Authorization = `Bearer ${token}`;
			}
			const response = await fetch("/api/v1/release-notes-acceptance/accept", {
				method: "POST",
				headers,
				credentials: "include", // Include cookies for OIDC users
				body: JSON.stringify({ version }),
			});

			const data = await response.json();

			if (response.ok) {
				// Update user state immediately with new accepted version
				const updatedAcceptedVersions = [
					...(user?.accepted_release_notes_versions || []),
					version,
				];

				const updatedUser = {
					...user,
					accepted_release_notes_versions: updatedAcceptedVersions,
				};

				// Update both state and localStorage atomically
				setUser(updatedUser);
				localStorage.setItem("user", JSON.stringify(updatedUser));

				return { success: true };
			} else {
				return {
					success: false,
					error: data.error || "Failed to accept release notes",
				};
			}
		} catch (error) {
			console.error("Error accepting release notes:", error);
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
	// Also checks if OIDC is configured to bypass the welcome page
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

				// If OIDC is enabled with auto-create users, bypass the welcome page
				// The first user will be created via OIDC JIT provisioning as admin
				if (!data.hasAdminUsers && data.oidc?.canBypassWelcome) {
					console.log(
						"No admin users, but OIDC can handle first user - bypassing welcome page",
					);
					setNeedsFirstTimeSetup(false);
				} else {
					setNeedsFirstTimeSetup(!data.hasAdminUsers);
				}

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
			setUser({
				...authUser,
				accepted_release_notes_versions:
					authUser.accepted_release_notes_versions || [],
			});
			setNeedsFirstTimeSetup(false);
			setAuthPhase(AUTH_PHASES.READY);
		});

		// Store user in localStorage (for session recovery)
		localStorage.setItem(
			"user",
			JSON.stringify({
				...authUser,
				accepted_release_notes_versions:
					authUser.accepted_release_notes_versions || [],
			}),
		);

		// Token is NOT stored in localStorage to prevent XSS attacks
		// All auth now uses httpOnly cookies set by the server
		// Remove any stale token from localStorage
		localStorage.removeItem("token");

		// Fetch permissions - works with cookies if token is null
		fetchPermissions(authToken);
	};

	// Computed loading state based on phase and permissions state
	const isLoading = !isAuthPhase.ready(authPhase) || permissionsLoading;

	// Function to check authentication status
	// With httpOnly cookie auth, we check for user presence (server validates via cookies)
	const isAuthenticated = () => {
		// User presence indicates valid session (token is in httpOnly cookie)
		return !!(user && isAuthPhase.ready(authPhase));
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
		acceptReleaseNotes,
	};

	return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
};
