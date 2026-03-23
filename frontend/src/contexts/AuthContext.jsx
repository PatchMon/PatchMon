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
	const [firstTimeWizardActive, setFirstTimeWizardActive] = useState(false);
	// When non-null, setup check failed (backend/DB down or rate limited) - do not show first-time setup
	const [setupCheckError, setSetupCheckError] = useState(null);

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

	// Listen for 401 session-expired from API interceptor - clear auth state so React Router navigates to login
	// (avoids hard redirect race that could trigger ErrorBoundary "Something went wrong")
	useEffect(() => {
		const handleSessionExpired = () => {
			setToken(null);
			setUser(null);
			setPermissions(null);
			localStorage.removeItem("token");
			localStorage.removeItem("user");
		};
		window.addEventListener("auth:session-expired", handleSessionExpired);
		return () =>
			window.removeEventListener("auth:session-expired", handleSessionExpired);
	}, []);

	// Initialize auth state - validate session via API (cookies) or localStorage.
	// Only runs once on mount to prevent re-validation (and loading flash) on every navigation.
	useEffect(() => {
		const abortController = new AbortController();

		const validateSession = async () => {
			// Read pathname at call time (not as a dependency) to avoid re-running on navigation
			const onLoginPage = window.location.pathname === "/login";
			const hasStoredUser = !!localStorage.getItem("user");
			if (onLoginPage && !hasStoredUser) {
				localStorage.removeItem("token");
				setSetupCheckError(null);
				setAuthPhase(AUTH_PHASES.CHECKING_SETUP);
				return;
			}

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
					devLog("Profile validation failed:", response.status, errorData);
				}
			} catch (error) {
				if (error.name === "AbortError") return;
				devLog("Profile validation exception:", error);
				devLog("Cookie-based auth failed:", error.message);
			}

			if (abortController.signal.aborted) return;

			// Clean up any stale token from localStorage (security measure)
			localStorage.removeItem("token");

			// No valid session, check if setup is needed
			setSetupCheckError(null);
			setAuthPhase(AUTH_PHASES.CHECKING_SETUP);
		};

		validateSession();

		return () => abortController.abort();
	}, [fetchPermissions]); // eslint-disable-line react-hooks/exhaustive-deps

	const refetchUser = useCallback(async () => {
		try {
			const response = await fetch("/api/v1/auth/profile", {
				credentials: "include",
			});
			if (response.ok) {
				const data = await response.json();
				setUser(data.user);
				await fetchPermissions();
				return data.user;
			}
		} catch (error) {
			devLog("refetchUser failed:", error);
		}
		return null;
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
				credentials: "include", // Include cookies for httpOnly token
				body: JSON.stringify({ username, password }),
			});

			const data = await response.json();

			if (response.ok) {
				// Check if TFA is required
				if (data.requiresTfa) {
					return { success: true, requiresTfa: true };
				}

				// Regular successful login
				// Note: httpOnly cookies are set by the server for secure auth
				// localStorage is used for backward compatibility and UI state only
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
				const userPermissions = await fetchPermissions(data.token);
				if (userPermissions) {
					setPermissions(userPermissions);
				}

				// Note: User preferences will be automatically fetched by ColorThemeContext
				// when the component mounts, so no need to invalidate here

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

				return { success: false, error: data.error || "Login failed" };
			}
		} catch (error) {
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
			sessionStorage.setItem("explicit_logout", "true");
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
			const fetchOptions = {
				method: "PUT",
				headers: {
					"Content-Type": "application/json",
				},
				body: JSON.stringify(profileData),
				credentials: "include",
			};
			if (token) {
				fetchOptions.headers.Authorization = `Bearer ${token}`;
			}
			const response = await fetch("/api/v1/auth/profile", fetchOptions);

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
			const fetchOptions = {
				method: "PUT",
				headers: {
					"Content-Type": "application/json",
				},
				body: JSON.stringify({ currentPassword, newPassword }),
				credentials: "include",
			};
			if (token) {
				fetchOptions.headers.Authorization = `Bearer ${token}`;
			}
			const response = await fetch(
				"/api/v1/auth/change-password",
				fetchOptions,
			);

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
	const canManageNotifications = () =>
		hasPermission("can_manage_notifications");
	const canViewNotificationLogs = () =>
		hasPermission("can_view_notification_logs");
	const canManagePatching = () => hasPermission("can_manage_patching");
	const canManageCompliance = () => hasPermission("can_manage_compliance");
	const canManageDocker = () => hasPermission("can_manage_docker");
	const canManageAlerts = () => hasPermission("can_manage_alerts");
	const canManageAutomation = () => hasPermission("can_manage_automation");
	const canUseRemoteAccess = () => hasPermission("can_use_remote_access");

	const SETUP_COMPLETE_CACHE_KEY = "patchmon_setup_complete";

	// Check if any admin users exist (for first-time setup)
	// Uses login-settings (includes hasAdminUsers) and caches result to avoid repeated public API calls
	const checkAdminUsersExist = useCallback(async () => {
		setSetupCheckError(null);

		// Skip API call if we've already confirmed setup is complete (cached)
		if (localStorage.getItem(SETUP_COMPLETE_CACHE_KEY) === "1") {
			setNeedsFirstTimeSetup(false);
			setAuthPhase(AUTH_PHASES.READY);
			return;
		}

		try {
			const response = await fetch("/api/v1/settings/login-settings", {
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
					devLog(
						"No admin users, but OIDC can handle first user - bypassing welcome page",
					);
					setNeedsFirstTimeSetup(false);
				} else {
					setNeedsFirstTimeSetup(!data.hasAdminUsers);
					// Cache setup complete to avoid repeated API calls on subsequent visits
					if (data.hasAdminUsers) {
						localStorage.setItem(SETUP_COMPLETE_CACHE_KEY, "1");
					}
				}

				setSetupCheckError(null);
				setAuthPhase(AUTH_PHASES.READY); // Setup check complete, move to ready phase
			} else if (response.status === 429) {
				// Rate limited - do not show first-time setup
				setSetupCheckError("rate_limited");
				setNeedsFirstTimeSetup(false);
				setAuthPhase(AUTH_PHASES.READY);
			} else if (response.status === 403) {
				// Check for CORS_ORIGIN mismatch (access from wrong URL)
				try {
					const data = await response.json();
					if (
						data?.code === "cors_mismatch" ||
						data?.error?.includes("CORS_ORIGIN")
					) {
						setSetupCheckError("cors_mismatch");
					} else {
						setSetupCheckError("server_unavailable");
					}
				} catch {
					setSetupCheckError("server_unavailable");
				}
				setNeedsFirstTimeSetup(false);
				setAuthPhase(AUTH_PHASES.READY);
			} else if (response.status === 502 || response.status === 503) {
				// 502/503 from nginx: backend unreachable. Could be backend down,
				// or nginx converting 403 to 502 (proxy_next_upstream). Show message
				// that mentions CORS so user checks CORS_ORIGIN when using wrong URL.
				setSetupCheckError("server_or_cors");
				setNeedsFirstTimeSetup(false);
				setAuthPhase(AUTH_PHASES.READY);
			} else {
				// 5xx, 4xx (e.g. 500 DB error) - backend/DB not accessible
				setSetupCheckError("server_unavailable");
				setNeedsFirstTimeSetup(false);
				setAuthPhase(AUTH_PHASES.READY);
			}
		} catch (error) {
			console.error("Error checking admin users:", error);
			// Network error or backend unreachable - could be CORS when behind proxy
			setSetupCheckError("server_or_cors");
			setNeedsFirstTimeSetup(false);
			setAuthPhase(AUTH_PHASES.READY);
		}
	}, []);

	// Check for admin users ONLY when in CHECKING_SETUP phase
	useEffect(() => {
		if (isAuthPhase.checkingSetup(authPhase)) {
			checkAdminUsersExist();
		}
	}, [authPhase, checkAdminUsersExist]);

	const retrySetupCheck = useCallback(() => {
		setSetupCheckError(null);
		setAuthPhase(AUTH_PHASES.CHECKING_SETUP);
	}, []);

	const setAuthState = (authToken, authUser, options = {}) => {
		const { keepWizardVisible = false } = options;

		// Use flushSync to ensure all state updates are applied synchronously
		flushSync(() => {
			setToken(authToken);
			setUser({
				...authUser,
				accepted_release_notes_versions:
					authUser.accepted_release_notes_versions || [],
			});
			if (!keepWizardVisible) {
				setNeedsFirstTimeSetup(false);
				setFirstTimeWizardActive(false);
			} else {
				setFirstTimeWizardActive(true);
			}
			setAuthPhase(AUTH_PHASES.READY);
		});

		if (!keepWizardVisible) {
			// Cache setup complete (user just created admin account or logged in)
			localStorage.setItem(SETUP_COMPLETE_CACHE_KEY, "1");
		}

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

	const completeFirstTimeWizard = () => {
		setFirstTimeWizardActive(false);
		setNeedsFirstTimeSetup(false);
		localStorage.setItem(SETUP_COMPLETE_CACHE_KEY, "1");
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
		firstTimeWizardActive,
		setupCheckError,
		retrySetupCheck,
		authPhase,
		login,
		logout,
		updateProfile,
		changePassword,
		refreshPermissions,
		refetchUser,
		setAuthState,
		completeFirstTimeWizard,
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
		canManageNotifications,
		canViewNotificationLogs,
		canManagePatching,
		canManageCompliance,
		canManageDocker,
		canManageAlerts,
		canManageAutomation,
		canUseRemoteAccess,
		acceptReleaseNotes,
	};

	return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
};
