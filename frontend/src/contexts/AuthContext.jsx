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
			const fetchOptions = {
				credentials: "include", // Include cookies for httpOnly token
			};
			// Add Authorization header if token provided (backward compatibility)
			if (authToken) {
				fetchOptions.headers = {
					Authorization: `Bearer ${authToken}`,
				};
			}
			const response = await fetch("/api/v1/permissions/user-permissions", fetchOptions);

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
		const validateSession = async () => {
			try {
				// First, try to validate via API using httpOnly cookies
				const response = await fetch("/api/v1/auth/profile", {
					credentials: "include",
				});

				if (response.ok) {
					const data = await response.json();
					setUser(data.user);
					// Fetch permissions
					await fetchPermissions();
					setAuthPhase(AUTH_PHASES.READY);
					return;
				}
			} catch (error) {
				console.log("Cookie-based auth not available, checking localStorage");
			}

			// Fall back to localStorage for backward compatibility
			const storedToken = localStorage.getItem("token");
			const storedUser = localStorage.getItem("user");

			if (storedToken && storedUser) {
				try {
					setToken(storedToken);
					const parsedUser = JSON.parse(storedUser);
					setUser({
						...parsedUser,
						accepted_release_notes_versions:
							parsedUser.accepted_release_notes_versions || [],
					});
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
		};

		validateSession();
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
				localStorage.setItem("user", JSON.stringify({
					...data.user,
					accepted_release_notes_versions:
						data.user.accepted_release_notes_versions || [],
				}));
				// Keep token in localStorage for backward compatibility with API clients
				localStorage.setItem("token", data.token);

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

	const acceptReleaseNotes = async (version) => {
		try {
			const response = await fetch("/api/v1/release-notes-acceptance/accept", {
				method: "POST",
				headers: {
					Authorization: `Bearer ${token}`,
					"Content-Type": "application/json",
				},
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

		// Only store token if provided (OIDC uses httpOnly cookies, no token in JS)
		if (authToken) {
			localStorage.setItem("token", authToken);
		} else {
			// Remove stale token for cookie-based auth (OIDC)
			localStorage.removeItem("token");
		}

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
