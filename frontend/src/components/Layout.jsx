import { useQuery } from "@tanstack/react-query";
import {
	AlertTriangle,
	BookOpen,
	ChevronDown,
	ChevronLeft,
	ChevronRight,
	Clock,
	Container,
	CreditCard,
	GitBranch,
	Github,
	Globe,
	LayoutDashboard,
	LogOut,
	Menu,
	Package,
	Plus,
	RefreshCw,
	Server,
	Settings,
	Shield,
	Star,
	UserCircle,
	Wrench,
	X,
} from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { FaLinkedin, FaYoutube } from "react-icons/fa";
import { Link, Outlet, useLocation, useNavigate } from "react-router-dom";
import { getRequiredTier } from "../constants/tiers";
import { useAuth } from "../contexts/AuthContext";
import { useColorTheme } from "../contexts/ColorThemeContext";
import { useSettings } from "../contexts/SettingsContext";
import SidebarContext from "../contexts/SidebarContext";
import { useUpdateNotification } from "../contexts/UpdateNotificationContext";
import { alertsAPI, dashboardAPI, settingsAPI, versionAPI } from "../utils/api";
import { isRenderableAvatarSrc } from "../utils/avatar";
import { resolveLogoPath } from "../utils/logoPaths";
import { prefetchRoute } from "../utils/routePrefetch";
import BuyMeACoffeeIcon from "./BuyMeACoffeeIcon";
import { useCommunityLinks } from "./CommunityLinks";
import DiscordIcon from "./DiscordIcon";
import DonateModal from "./DonateModal";
import GlobalSearch from "./GlobalSearch";
import Logo from "./Logo";
import ReleaseNotesModal from "./ReleaseNotesModal";
import TierBadge from "./TierBadge";
import UpgradeNotificationIcon from "./UpgradeNotificationIcon";

const Layout = ({ children }) => {
	// When used as a layout route, render Outlet; otherwise render children (backwards compat)
	const content = children ?? <Outlet />;
	const [sidebarOpen, setSidebarOpen] = useState(false);
	// Pinned collapsed state — the user's explicit choice via the toggle button.
	// Persisted to localStorage. Hover behavior only applies when pinned-collapsed.
	const [pinnedCollapsed, setPinnedCollapsed] = useState(() => {
		const saved = localStorage.getItem("sidebarCollapsed");
		return saved ? JSON.parse(saved) : false;
	});
	// Ephemeral hover state that temporarily expands the sidebar when pinned-collapsed.
	const [isSidebarHovered, setIsSidebarHovered] = useState(false);
	// Effective collapsed state: only collapsed when pinned AND not currently hovered.
	const sidebarCollapsed = pinnedCollapsed && !isSidebarHovered;
	// Keep the external API stable for context consumers (SshTerminal, etc.): the
	// setter always mutates the pinned state, not the ephemeral hover state.
	const setSidebarCollapsed = setPinnedCollapsed;
	const { links: communityLinks } = useCommunityLinks();
	const [_userMenuOpen, setUserMenuOpen] = useState(false);
	const [mobileLinksOpen, setMobileLinksOpen] = useState(false);
	const [showReleaseNotes, setShowReleaseNotes] = useState(false);
	const [showDonateModal, setShowDonateModal] = useState(false);
	const [expandedNav, setExpandedNav] = useState(null);
	const location = useLocation();
	const navigate = useNavigate();
	const {
		user,
		logout,
		canViewDashboard,
		canViewHosts,
		canManageHosts,
		canViewPackages,
		canViewUsers,
		canManageUsers,
		canViewReports,
		canExportData,
		canManageSettings,
		hasModule,
		hasPermission,
	} = useAuth();
	const { settings: publicSettings } = useSettings();
	const canManageBilling =
		publicSettings?.admin_mode === true && hasPermission("can_manage_billing");
	const { updateAvailable } = useUpdateNotification();
	const { themeConfig } = useColorTheme();
	const userMenuRef = useRef(null);

	// Fetch dashboard stats for the "Last updated" info
	const {
		data: stats,
		refetch,
		isFetching,
	} = useQuery({
		queryKey: ["dashboardStats"],
		queryFn: () => dashboardAPI.getStats().then((res) => res.data),
		staleTime: 5 * 60 * 1000, // Data stays fresh for 5 minutes
		refetchOnWindowFocus: false, // Don't refetch when window regains focus
	});

	// Fetch settings for favicon, logos, and alerts_enabled (public endpoint works for all users)
	const { data: settings } = useQuery({
		queryKey: ["settings", "public"],
		queryFn: () => settingsAPI.getPublic().then((res) => res.data),
	});

	// Fetch version info
	const { data: versionInfo } = useQuery({
		queryKey: ["versionInfo"],
		queryFn: () => versionAPI.getCurrent().then((res) => res.data),
		staleTime: 300000, // Consider data stale after 5 minutes
	});

	// Fetch hosts for connection status (only if user can view hosts)
	// Use dashboardAPI.getHosts() to match Hosts.jsx page and ensure api_id is included
	const { data: hosts } = useQuery({
		queryKey: ["hosts", "sidebar"],
		queryFn: () => dashboardAPI.getHosts().then((res) => res.data),
		enabled: canViewHosts(),
		staleTime: 5 * 60 * 1000, // Data stays fresh for 5 minutes
		refetchOnWindowFocus: false,
	});

	// Fetch alert stats for Reporting badge (only if user can view reports and alerts are enabled)
	const { data: alertStats } = useQuery({
		queryKey: ["alert-stats", "sidebar"],
		queryFn: () => alertsAPI.getAlertStats().then((res) => res.data.data || {}),
		enabled: canViewReports() && settings?.alerts_enabled !== false,
		refetchInterval: 30000, // Refresh every 30 seconds to reduce API load
		staleTime: 0, // Always consider stale
	});

	// Track WebSocket status for hosts
	const [wsStatusMap, setWsStatusMap] = useState({});

	// Fetch WebSocket status for hosts
	useEffect(() => {
		if (!hosts || !Array.isArray(hosts) || hosts.length === 0) return;

		// Fetch initial WebSocket status for all hosts
		const fetchInitialStatus = async () => {
			const apiIds = hosts
				.filter((host) => host.api_id)
				.map((host) => host.api_id);

			if (apiIds.length === 0) return;

			try {
				const response = await fetch(
					`/api/v1/ws/status?apiIds=${apiIds.join(",")}`,
					{
						credentials: "include",
					},
				);
				if (response.ok) {
					const result = await response.json();
					setWsStatusMap(result.data);
				}
			} catch (_error) {
				// Silently handle errors
			}
		};

		fetchInitialStatus();

		// Poll every 10 seconds for status updates
		const pollInterval = setInterval(() => {
			const apiIds = hosts
				.filter((host) => host.api_id)
				.map((host) => host.api_id);

			if (apiIds.length === 0) return;

			fetch(`/api/v1/ws/status?apiIds=${apiIds.join(",")}`, {
				credentials: "include",
			})
				.then((response) => response.json())
				.then((result) => {
					if (result.success && result.data) {
						setWsStatusMap(result.data);
					} else if (result.data) {
						setWsStatusMap(result.data);
					}
				})
				.catch(() => {
					// Silently handle errors
				});
		}, 10000);

		return () => {
			clearInterval(pollInterval);
		};
	}, [hosts]);

	// Check for new release notes when user or version changes
	useEffect(() => {
		if (!user || !versionInfo?.version) return;

		// Get accepted versions from user object (loaded on login)
		const acceptedVersions = user.accepted_release_notes_versions || [];
		const currentVersion = versionInfo.version;

		// If already accepted, don't show
		if (acceptedVersions.includes(currentVersion)) {
			return;
		}

		// Check if release notes exist for this version
		fetch(`/api/v1/release-notes/${currentVersion}`, {
			credentials: "include",
		})
			.then((res) => res.json())
			.then((data) => {
				// Only show if release notes exist and version not accepted
				if (data.exists && !acceptedVersions.includes(currentVersion)) {
					setShowReleaseNotes(true);
				}
			})
			.catch((error) => {
				console.error("Error checking release notes:", error);
			});
	}, [user, versionInfo]);

	// Build navigation based on permissions
	const buildNavigation = () => {
		const nav = [];

		// Dashboard - only show if user can view dashboard
		if (canViewDashboard()) {
			nav.push({ name: "Dashboard", href: "/", icon: LayoutDashboard });
		}

		// Assets section
		if (canViewHosts() || canViewPackages()) {
			const assetItems = [];

			if (canViewHosts()) {
				assetItems.push({ name: "Hosts", href: "/hosts", icon: Server });
				assetItems.push({
					name: "Repos",
					href: "/repositories",
					icon: GitBranch,
				});
			}

			if (canViewPackages()) {
				assetItems.push({
					name: "Packages",
					href: "/packages",
					icon: Package,
				});
			}

			if (assetItems.length > 0) {
				nav.push({
					section: "ASSETS",
					items: assetItems,
				});
			}
		}

		// Operations section
		if (canViewHosts() || canViewReports()) {
			const opsItems = [];

			// Patching is a Plus-tier feature (module key: "patching"). Sub-tab
			// "Policies" additionally requires the patching_policies module.
			// Locked items stay visible with a TierBadge so users can discover
			// and upgrade; the route renders an upgrade screen via <ModuleGate>.
			if (canViewHosts()) {
				const patchingLocked = !hasModule("patching");
				const policiesLocked = !hasModule("patching_policies");
				const patchingChildren = [
					{ name: "Overview", href: "/patching?tab=overview" },
					{ name: "Runs & History", href: "/patching?tab=runs" },
					{
						name: "Policies",
						href: "/patching?tab=policies",
						lockedModule: policiesLocked ? "patching_policies" : null,
						lockedTier: policiesLocked
							? getRequiredTier("patching_policies")
							: null,
					},
				];
				opsItems.push({
					name: "Patching",
					href: "/patching",
					icon: Wrench,
					new: !patchingLocked,
					lockedModule: patchingLocked ? "patching" : null,
					lockedTier: patchingLocked ? getRequiredTier("patching") : null,
					children: patchingChildren,
				});
			}

			// Compliance is a Max-tier feature (module key: "compliance").
			if (canViewReports()) {
				const complianceLocked = !hasModule("compliance");
				opsItems.push({
					name: "Compliance",
					href: "/compliance",
					icon: Shield,
					lockedModule: complianceLocked ? "compliance" : null,
					lockedTier: complianceLocked ? getRequiredTier("compliance") : null,
					children: [
						{ name: "Overview", href: "/compliance?tab=overview" },
						{ name: "Hosts", href: "/compliance?tab=hosts" },
						{ name: "Scan Results", href: "/compliance?tab=scan-results" },
						{ name: "History", href: "/compliance?tab=history" },
						{ name: "Settings", href: "/compliance?tab=settings" },
					],
				});
			}

			if (canViewReports() && settings?.alerts_enabled !== false) {
				// "Alert Lifecycle" uses /alerts/config endpoints (advanced alert tuning),
				// which require the alerts_advanced module (Plus tier).
				const alertLifecycleLocked = !hasModule("alerts_advanced");
				const reportingChildren = [
					{ name: "Overview", href: "/reporting?tab=overview" },
					{ name: "Alerts", href: "/reporting?tab=alerts" },
					{
						name: "Alert Lifecycle",
						href: "/reporting?tab=alert-settings",
						lockedModule: alertLifecycleLocked ? "alerts_advanced" : null,
						lockedTier: alertLifecycleLocked
							? getRequiredTier("alerts_advanced")
							: null,
					},
					{ name: "Destinations", href: "/reporting?tab=destinations" },
					{ name: "Event Rules", href: "/reporting?tab=rules" },
					{ name: "Scheduled Reports", href: "/reporting?tab=reports" },
					{ name: "Delivery Log", href: "/reporting?tab=log" },
				];
				opsItems.push({
					name: "Reporting",
					href: "/reporting",
					icon: AlertTriangle,
					new: true,
					children: reportingChildren,
				});
			}

			// Docker container monitoring is a Plus-tier feature (module key: "docker").
			if (canViewReports()) {
				const dockerLocked = !hasModule("docker");
				opsItems.push({
					name: "Docker",
					href: "/docker",
					icon: Container,
					beta: !dockerLocked,
					lockedModule: dockerLocked ? "docker" : null,
					lockedTier: dockerLocked ? getRequiredTier("docker") : null,
					children: [
						{ name: "Stacks", href: "/docker?tab=stacks" },
						{ name: "Containers", href: "/docker?tab=containers" },
						{ name: "Images", href: "/docker?tab=images" },
						{ name: "Volumes", href: "/docker?tab=volumes" },
						{ name: "Networks", href: "/docker?tab=networks" },
						{ name: "Hosts", href: "/docker?tab=hosts" },
					],
				});
			}

			if (opsItems.length > 0) {
				nav.push({
					section: "OPERATIONS",
					items: opsItems,
				});
			}
		}

		// System section
		if (canViewHosts() || canViewPackages() || canViewReports()) {
			const systemItems = [];

			systemItems.push({
				name: "Automation",
				href: "/automation",
				icon: RefreshCw,
			});

			// Billing — double-gated: only on cloud installs (admin_mode === true)
			// AND only for users with can_manage_billing permission. On self-hosted
			// installs (admin_mode === false) this item stays hidden entirely.
			if (canManageBilling) {
				systemItems.push({
					name: "Billing",
					href: "/billing",
					icon: CreditCard,
				});
			}

			if (
				canManageSettings() ||
				canViewUsers() ||
				canManageUsers() ||
				canViewReports() ||
				canExportData()
			) {
				systemItems.push({
					name: "Settings",
					href: "/settings/users",
					icon: Settings,
					showUpgradeIcon: updateAvailable,
				});
			}

			const sidebarLinkIds = ["roadmap", "docs", "email", "website", "billing"];
			const linkChildren = communityLinks
				.filter((l) => sidebarLinkIds.includes(l.id))
				.map((l) => ({
					name: l.label,
					href: l.url,
					external: true,
				}));
			systemItems.push({
				name: "Links",
				href: "#links",
				icon: BookOpen,
				children: linkChildren,
			});

			if (systemItems.length > 0) {
				nav.push({
					section: "SYSTEM",
					items: systemItems,
				});
			}
		}

		return nav;
	};

	const navigation = buildNavigation();
	// Settings sub-nav is in SettingsLayout; main Layout sidebar has no settings sub-nav
	const settingsNavigation = [];

	const isActive = (path) =>
		location.pathname === path ||
		(path !== "/" && location.pathname.startsWith(`${path}/`));

	// Auto-expand the nav item matching the current path on navigation
	const prevPathnameRef = useRef(location.pathname);
	useEffect(() => {
		const path = location.pathname;
		const prevPath = prevPathnameRef.current;
		prevPathnameRef.current = path;

		// Only auto-expand when navigating to a different base path
		if (path === prevPath) return;

		// Find which expandable item matches the new path
		let matched = null;
		for (const item of navigation) {
			if (item.items) {
				for (const sub of item.items) {
					if (
						sub.children &&
						(path === sub.href ||
							(sub.href !== "/" && path.startsWith(`${sub.href}/`)))
					) {
						matched = sub.name;
						break;
					}
				}
			}
			if (matched) break;
		}

		// Expand the matched item, or collapse if navigating to an unrelated page
		setExpandedNav(matched);
	}, [location.pathname, navigation]);

	// Get page title based on current route
	const getPageTitle = () => {
		const path = location.pathname;

		if (path === "/") return "Dashboard";
		if (path === "/hosts") return "Hosts";
		if (path === "/packages") return "Packages";
		if (path === "/reporting") return "Reporting";
		if (path === "/repositories" || path.startsWith("/repositories/"))
			return "Repositories";
		if (path === "/services") return "Services";
		if (path === "/docker") return "Docker";
		if (path === "/pro-action") return "Pro-Action";
		if (path === "/automation") return "Automation";
		if (path === "/patching" || path.startsWith("/patching/"))
			return "Patching";
		if (path === "/compliance" || path.startsWith("/compliance/"))
			return "Compliance";
		if (path === "/users") return "Users";
		if (path === "/permissions") return "Permissions";
		if (path === "/settings") return "Settings";
		if (path === "/options") return "PatchMon Options";
		if (path === "/audit-log") return "Audit Log";
		if (path === "/settings/profile") return "My Profile";
		if (path.startsWith("/hosts/")) return "Host Details";
		if (path.startsWith("/packages/")) return "Package Details";
		if (path.startsWith("/settings/")) return "Settings";

		return "PatchMon";
	};

	const handleLogout = async () => {
		await logout();
	};

	const handleAddHost = () => {
		// Navigate to hosts page with add modal parameter
		navigate("/hosts?action=add");
	};

	// Short format for navigation area
	const formatRelativeTimeShort = (date) => {
		if (!date) return "Never";

		const now = new Date();
		const dateObj = new Date(date);

		// Check if date is valid
		if (Number.isNaN(dateObj.getTime())) return "Invalid date";

		const diff = now - dateObj;
		const seconds = Math.floor(diff / 1000);
		const minutes = Math.floor(seconds / 60);
		const hours = Math.floor(minutes / 60);
		const days = Math.floor(hours / 24);

		if (days > 0) return `${days}d ago`;
		if (hours > 0) return `${hours}h ago`;
		if (minutes > 0) return `${minutes}m ago`;
		return `${seconds}s ago`;
	};

	// Auto-collapse main sidebar on settings pages, restore when leaving
	const sidebarStateBeforeSettings = useRef(null);
	const isSettingsPage = location.pathname.startsWith("/settings");
	const prevIsSettingsPage = useRef(isSettingsPage);

	useEffect(() => {
		const wasSettings = prevIsSettingsPage.current;
		prevIsSettingsPage.current = isSettingsPage;

		if (isSettingsPage && !wasSettings) {
			// Entering settings — remember current state and collapse
			sidebarStateBeforeSettings.current = pinnedCollapsed;
			setPinnedCollapsed(true);
		} else if (
			!isSettingsPage &&
			wasSettings &&
			sidebarStateBeforeSettings.current !== null
		) {
			// Leaving settings — restore previous state
			setPinnedCollapsed(sidebarStateBeforeSettings.current);
			sidebarStateBeforeSettings.current = null;
		}
	}, [isSettingsPage, pinnedCollapsed]);

	// Persist only the pinned state (not the ephemeral hover state) to localStorage,
	// and skip while auto-collapsed for settings.
	useEffect(() => {
		if (!isSettingsPage) {
			localStorage.setItem("sidebarCollapsed", JSON.stringify(pinnedCollapsed));
		}
	}, [pinnedCollapsed, isSettingsPage]);

	// Close user menu when clicking outside
	useEffect(() => {
		const handleClickOutside = (event) => {
			if (userMenuRef.current && !userMenuRef.current.contains(event.target)) {
				setUserMenuOpen(false);
			}
		};

		document.addEventListener("mousedown", handleClickOutside);
		return () => {
			document.removeEventListener("mousedown", handleClickOutside);
		};
	}, []);

	// Set CSS custom properties for glassmorphism and theme colors in dark mode
	useEffect(() => {
		const updateThemeStyles = () => {
			const isDark = document.documentElement.classList.contains("dark");
			const root = document.documentElement;

			if (isDark && themeConfig?.app) {
				// App background tracks the active dark theme preset.
				root.style.setProperty("--app-bg-primary", themeConfig.app.bgPrimary);

				// Glass navigation bars - very light for pattern visibility
				root.style.setProperty("--sidebar-bg", "rgba(0, 0, 0, 0.15)");
				root.style.setProperty("--sidebar-blur", "blur(12px)");
				root.style.setProperty("--topbar-bg", "rgba(0, 0, 0, 0.15)");
				root.style.setProperty("--topbar-blur", "blur(12px)");
				root.style.setProperty("--button-bg", "rgba(255, 255, 255, 0.15)");
				root.style.setProperty("--button-blur", "blur(8px)");

				// Theme-colored cards and buttons - darker to stand out
				root.style.setProperty("--card-bg", themeConfig.app.cardBg);
				root.style.setProperty("--card-border", themeConfig.app.cardBorder);
				root.style.setProperty("--card-bg-hover", themeConfig.app.bgTertiary);
				root.style.setProperty("--theme-button-bg", themeConfig.app.buttonBg);
				root.style.setProperty(
					"--theme-button-hover",
					themeConfig.app.buttonHover,
				);
			} else {
				// Light mode - standard colors
				root.style.setProperty("--app-bg-primary", "#f8fafc");
				root.style.setProperty("--sidebar-bg", "white");
				root.style.setProperty("--sidebar-blur", "none");
				root.style.setProperty("--topbar-bg", "white");
				root.style.setProperty("--topbar-blur", "none");
				root.style.setProperty("--button-bg", "white");
				root.style.setProperty("--button-blur", "none");
				root.style.setProperty("--card-bg", "white");
				root.style.setProperty("--card-border", "#e5e7eb");
				root.style.setProperty("--card-bg-hover", "#f9fafb");
				root.style.setProperty("--theme-button-bg", "#f3f4f6");
				root.style.setProperty("--theme-button-hover", "#e5e7eb");
			}
		};

		updateThemeStyles();

		// Watch for dark mode changes
		const observer = new MutationObserver(() => {
			updateThemeStyles();
		});

		observer.observe(document.documentElement, {
			attributes: true,
			attributeFilter: ["class"],
		});

		return () => observer.disconnect();
	}, [themeConfig]);

	return (
		<SidebarContext.Provider
			value={{
				setSidebarCollapsed,
				sidebarCollapsed,
			}}
		>
			<div
				className="min-h-screen relative overflow-hidden"
				style={{ backgroundColor: "var(--app-bg-primary)" }}
			>
				{/* Static triangle mesh background. Dark mode only on the authenticated app. */}
				<div
					aria-hidden="true"
					className="patchmon-mesh-bg fixed inset-0 w-full h-full pointer-events-none hidden dark:block"
					style={{ zIndex: 0 }}
				/>
				{/* Subtle dark vignette — only in dark mode, where the base bg is black. */}
				<div
					aria-hidden="true"
					className="fixed inset-0 bg-gradient-to-br from-black/10 to-black/20 hidden dark:block pointer-events-none"
					style={{ zIndex: 2 }}
				/>
				{/* Mobile sidebar */}
				<div
					className={`fixed inset-0 z-[60] lg:hidden ${sidebarOpen ? "block" : "hidden"}`}
				>
					<button
						type="button"
						className="fixed inset-0 bg-secondary-600 bg-opacity-75 cursor-default"
						onClick={() => setSidebarOpen(false)}
						aria-label="Close sidebar"
					/>
					<div
						className="relative flex w-full max-w-[280px] flex-col bg-white dark:border-r dark:border-white/10 pb-4 pt-5 shadow-xl"
						style={{
							backgroundColor: "var(--sidebar-bg, white)",
							backdropFilter: "var(--sidebar-blur, none)",
							WebkitBackdropFilter: "var(--sidebar-blur, none)",
						}}
					>
						<div className="absolute right-0 top-0 -mr-12 pt-2">
							<button
								type="button"
								className="ml-1 flex h-11 w-11 min-w-[44px] min-h-[44px] items-center justify-center rounded-full bg-secondary-600/90 hover:bg-secondary-600 focus:outline-none focus:ring-2 focus:ring-inset focus:ring-white transition-colors"
								onClick={() => setSidebarOpen(false)}
								aria-label="Close sidebar"
							>
								<X className="h-6 w-6 text-white" />
							</button>
						</div>
						<div className="flex flex-shrink-0 items-center justify-center px-4">
							<Link to="/" className="flex items-center">
								<Logo className="h-10 w-auto" alt="PatchMon Logo" />
							</Link>
						</div>
						<nav className="mt-8 flex-1 space-y-6 px-2">
							{/* Show message for users with very limited permissions */}
							{navigation.length === 0 && (
								<div className="px-2 py-4 text-center">
									<div className="text-sm text-secondary-500 dark:text-white/70">
										<p className="mb-2">Limited access</p>
										<p className="text-xs">
											Contact your administrator for additional permissions
										</p>
									</div>
								</div>
							)}
							{navigation.map((item) => {
								if (item.name) {
									// Single item (Dashboard)
									return (
										<Link
											key={item.name}
											to={item.href}
											className={`group flex items-center px-2 py-3 text-sm font-medium rounded-md min-h-[44px] ${
												isActive(item.href)
													? "bg-primary-100 dark:bg-primary-600 text-primary-900 dark:text-white"
													: "text-secondary-600 dark:text-white hover:bg-secondary-50 dark:hover:bg-secondary-700 hover:text-secondary-900 dark:hover:text-white"
											}`}
											onMouseEnter={() => prefetchRoute(item.href)}
											onClick={() => setSidebarOpen(false)}
										>
											<item.icon className="mr-3 h-5 w-5" />
											{item.name}
										</Link>
									);
								} else if (item.section) {
									// Section with items
									return (
										<div key={item.section}>
											<h3 className="text-xs font-semibold text-secondary-500 dark:text-white uppercase tracking-wider mb-2">
												{item.section}
											</h3>
											<div className="space-y-1">
												{item.items
													.filter((subItem) => !subItem.comingSoon)
													.map((subItem) => (
														<div key={subItem.name}>
															{subItem.name === "Hosts" && canManageHosts() ? (
																// Special handling for Hosts item with integrated + button (mobile)
																<Link
																	to={subItem.href}
																	className={`group flex items-center px-2 py-3 text-sm font-medium rounded-md min-h-[44px] ${
																		isActive(subItem.href)
																			? "bg-primary-100 dark:bg-primary-600 text-primary-900 dark:text-white"
																			: "text-secondary-600 dark:text-white hover:bg-secondary-50 dark:hover:bg-secondary-700 hover:text-secondary-900 dark:hover:text-white"
																	}`}
																	onMouseEnter={() =>
																		prefetchRoute(subItem.href)
																	}
																	onClick={() => setSidebarOpen(false)}
																>
																	<subItem.icon className="mr-3 h-5 w-5" />
																	<span className="flex items-center gap-2 flex-1">
																		{subItem.name}
																		{subItem.name === "Hosts" &&
																			stats?.cards?.totalHosts !==
																				undefined && (
																				<span className="ml-2 inline-flex items-center justify-center px-1.5 py-0.5 text-xs rounded bg-secondary-100 dark:bg-secondary-600 text-secondary-700 dark:text-secondary-200">
																					{stats.cards.totalHosts}
																				</span>
																			)}
																	</span>
																	<button
																		type="button"
																		onClick={(e) => {
																			e.preventDefault();
																			setSidebarOpen(false);
																			handleAddHost();
																		}}
																		className="ml-auto flex items-center justify-center w-5 h-5 rounded-full border-2 border-current opacity-60 hover:opacity-100 transition-all duration-200 self-center"
																		title="Add Host"
																	>
																		<Plus className="h-3 w-3" />
																	</button>
																</Link>
															) : (
																// Standard navigation item (mobile)
																<>
																	<Link
																		to={subItem.href}
																		className={`group flex items-center px-2 py-2 text-sm font-medium rounded-md ${
																			isActive(subItem.href)
																				? "bg-primary-100 dark:bg-primary-600 text-primary-900 dark:text-white"
																				: "text-secondary-600 dark:text-white hover:bg-secondary-50 dark:hover:bg-secondary-700 hover:text-secondary-900 dark:hover:text-white"
																		} ${subItem.comingSoon ? "opacity-50 cursor-not-allowed" : ""}`}
																		onMouseEnter={() =>
																			!subItem.comingSoon &&
																			prefetchRoute(subItem.href)
																		}
																		onClick={(e) => {
																			if (subItem.comingSoon) {
																				e.preventDefault();
																				return;
																			}
																			if (subItem.children) {
																				if (subItem.href.startsWith("#")) {
																					e.preventDefault();
																				}
																				setExpandedNav(
																					expandedNav === subItem.name
																						? null
																						: subItem.name,
																				);
																			} else {
																				setSidebarOpen(false);
																			}
																		}}
																	>
																		<subItem.icon className="mr-3 h-5 w-5" />
																		<span className="flex items-center gap-2 flex-1">
																			{subItem.name}
																			{subItem.name === "Hosts" &&
																				stats?.cards?.totalHosts !==
																					undefined && (
																					<span className="ml-2 inline-flex items-center justify-center px-1.5 py-0.5 text-xs rounded bg-secondary-100 dark:bg-secondary-600 text-secondary-700 dark:text-secondary-200">
																						{stats.cards.totalHosts}
																					</span>
																				)}
																			{subItem.name === "Reporting" &&
																				alertStats && (
																					<div className="ml-2 flex items-center gap-0.5">
																						{(alertStats.informational || 0) >
																							0 && (
																							<span className="inline-flex items-center justify-center px-1.5 py-0.5 text-xs rounded bg-blue-100 text-blue-700 dark:bg-blue-900 dark:text-blue-200">
																								{alertStats.informational}
																							</span>
																						)}
																						{(alertStats.warning || 0) > 0 && (
																							<span className="inline-flex items-center justify-center px-1.5 py-0.5 text-xs rounded bg-yellow-100 text-yellow-700 dark:bg-yellow-900 dark:text-yellow-200">
																								{alertStats.warning}
																							</span>
																						)}
																						{(alertStats.error || 0) > 0 && (
																							<span className="inline-flex items-center justify-center px-1.5 py-0.5 text-xs rounded bg-orange-100 text-orange-700 dark:bg-orange-900 dark:text-orange-200">
																								{alertStats.error}
																							</span>
																						)}
																						{(alertStats.critical || 0) > 0 && (
																							<span className="inline-flex items-center justify-center px-1.5 py-0.5 text-xs rounded bg-red-100 text-red-700 dark:bg-red-900 dark:text-red-200">
																								{alertStats.critical}
																							</span>
																						)}
																					</div>
																				)}
																			{subItem.comingSoon && (
																				<span className="text-xs bg-secondary-100 dark:bg-secondary-600 text-secondary-600 dark:text-secondary-200 px-1.5 py-0.5 rounded">
																					Soon
																				</span>
																			)}
																			{subItem.alpha && (
																				<span className="text-[10px] bg-purple-50 dark:bg-purple-900/50 text-purple-600 dark:text-purple-300 px-1 py-px rounded font-medium leading-tight">
																					Alpha
																				</span>
																			)}
																			{subItem.beta && (
																				<span className="text-[10px] bg-blue-50 dark:bg-blue-900/50 text-blue-600 dark:text-blue-300 px-1 py-px rounded font-medium leading-tight">
																					Beta
																				</span>
																			)}
																			{subItem.new && (
																				<span className="text-[10px] bg-green-50 dark:bg-green-900/50 text-green-600 dark:text-green-300 px-1 py-px rounded font-medium leading-tight">
																					New
																				</span>
																			)}
																			{subItem.lockedTier && (
																				<TierBadge tier={subItem.lockedTier} />
																			)}
																			{subItem.children && (
																				<ChevronDown
																					className={`ml-auto h-4 w-4 shrink-0 text-secondary-400 transition-transform duration-200 ${
																						expandedNav === subItem.name
																							? "rotate-180"
																							: ""
																					}`}
																				/>
																			)}
																		</span>
																	</Link>
																	{/* Expandable children (mobile) */}
																	{subItem.children &&
																		expandedNav === subItem.name && (
																			<ul className="ml-8 mt-0.5 space-y-0.5 border-l border-secondary-200 dark:border-secondary-700 pl-2">
																				{subItem.children.map((child) => (
																					<li key={child.name}>
																						{child.external ? (
																							<a
																								href={child.href}
																								target={
																									child.href.startsWith(
																										"mailto:",
																									)
																										? undefined
																										: "_blank"
																								}
																								rel="noopener noreferrer"
																								className="block text-sm py-2 px-2 rounded transition-colors min-h-[44px] flex items-center text-secondary-500 dark:text-white hover:text-secondary-900 dark:hover:text-primary-400"
																								onClick={() =>
																									setSidebarOpen(false)
																								}
																							>
																								{child.name}
																							</a>
																						) : (
																							<Link
																								to={child.href}
																								className={`text-sm py-2 px-2 rounded transition-colors min-h-[44px] flex items-center gap-2 ${
																									location.pathname +
																										location.search ===
																									child.href
																										? "text-primary-600 dark:text-primary-400 font-medium"
																										: "text-secondary-500 dark:text-white hover:text-secondary-900 dark:hover:text-primary-400"
																								}`}
																								onClick={() =>
																									setSidebarOpen(false)
																								}
																							>
																								<span>{child.name}</span>
																								{child.lockedTier && (
																									<TierBadge
																										tier={child.lockedTier}
																									/>
																								)}
																							</Link>
																						)}
																					</li>
																				))}
																			</ul>
																		)}
																</>
															)}
														</div>
													))}
											</div>
										</div>
									);
								}
								return null;
							})}

							{/* Mobile Logout Section */}
							<div className="mt-8 pt-4 border-t border-secondary-200 dark:border-secondary-700">
								<div className="px-2 space-y-1">
									<Link
										to="/settings/profile"
										className={`group flex items-center px-2 py-2 text-sm font-medium rounded-md ${
											isActive("/settings/profile")
												? "bg-primary-100 dark:bg-primary-600 text-primary-900 dark:text-white"
												: "text-secondary-600 dark:text-white hover:bg-secondary-50 dark:hover:bg-secondary-700 hover:text-secondary-900 dark:hover:text-white"
										}`}
										onClick={() => setSidebarOpen(false)}
									>
										{isRenderableAvatarSrc(user?.avatar_url) ? (
											<img
												src={user.avatar_url}
												alt={user.username}
												className="mr-3 h-5 w-5 rounded-full object-cover"
											/>
										) : (
											<UserCircle className="mr-3 h-5 w-5" />
										)}
										<span className="flex items-center gap-2">
											{user?.first_name || user?.username}
											{(user?.role === "admin" ||
												user?.role === "superadmin") && (
												<span
													className={`inline-flex items-center text-xs px-1.5 py-0.5 rounded ${
														user?.role === "superadmin"
															? "bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200"
															: "bg-primary-100 text-primary-800 dark:bg-primary-900 dark:text-primary-200"
													}`}
												>
													<Shield className="h-3 w-3 mr-1" />
													{user?.role === "superadmin"
														? "Super Admin"
														: "Admin"}
												</span>
											)}
										</span>
									</Link>
									<button
										type="button"
										onClick={() => {
											handleLogout();
											setSidebarOpen(false);
										}}
										className="w-full group flex items-center px-2 py-3 text-sm font-medium rounded-md text-secondary-600 dark:text-white hover:bg-secondary-50 dark:hover:bg-secondary-700 hover:text-secondary-900 dark:hover:text-white min-h-[44px]"
									>
										<LogOut className="mr-3 h-5 w-5" />
										Sign out
									</button>
								</div>
							</div>
						</nav>
					</div>
				</div>

				{/* Desktop sidebar */}
				<div
					className={`hidden lg:fixed lg:inset-y-0 z-[100] lg:flex lg:flex-col transition-all duration-300 relative ${
						sidebarCollapsed ? "lg:w-16" : "lg:w-64"
					} bg-white dark:bg-transparent`}
					onMouseEnter={() => setIsSidebarHovered(true)}
					onMouseLeave={() => setIsSidebarHovered(false)}
				>
					{/* Pin/unpin button: toggles the persisted pinned state. When pinned-expanded
					    the sidebar stays static; when pinned-collapsed, hover temporarily expands it. */}
					<button
						type="button"
						onClick={() => setPinnedCollapsed(!pinnedCollapsed)}
						className="absolute top-5 -right-3 z-[200] flex items-center justify-center w-6 h-6 rounded-full bg-white border border-secondary-300 dark:border-white/20 shadow-md hover:bg-secondary-50 transition-colors"
						style={{
							backgroundColor: "var(--button-bg, white)",
							backdropFilter: "var(--button-blur, none)",
							WebkitBackdropFilter: "var(--button-blur, none)",
						}}
						title={
							pinnedCollapsed
								? "Pin sidebar expanded"
								: "Collapse sidebar (hover to peek)"
						}
					>
						{pinnedCollapsed ? (
							<ChevronRight className="h-4 w-4 text-secondary-700 dark:text-white" />
						) : (
							<ChevronLeft className="h-4 w-4 text-secondary-700 dark:text-white" />
						)}
					</button>

					<div
						className={`flex grow flex-col gap-y-5 border-r border-secondary-200 dark:border-white/10 bg-white ${
							sidebarCollapsed ? "px-2 shadow-lg" : "px-2"
						}`}
						style={{
							backgroundColor: "var(--sidebar-bg, white)",
							backdropFilter: "var(--sidebar-blur, none)",
							WebkitBackdropFilter: "var(--sidebar-blur, none)",
							overflowY: "auto",
							overflowX: "visible",
						}}
					>
						<div
							className={`flex h-16 shrink-0 items-center border-b border-secondary-200 dark:border-white/10 ${
								sidebarCollapsed ? "justify-center" : "justify-center"
							}`}
						>
							{sidebarCollapsed ? (
								<Link to="/" className="flex items-center">
									<img
										src={`${resolveLogoPath(settings?.favicon, "favicon")}?v=${
											settings?.updated_at
												? new Date(settings.updated_at).getTime()
												: Date.now()
										}`}
										alt="PatchMon"
										className="h-12 w-12 object-contain"
										onError={(e) => {
											e.target.src = `/assets/logo_square_default.svg?v=${Date.now()}`;
										}}
									/>
								</Link>
							) : (
								<Link to="/" className="flex items-center">
									<Logo className="h-10 w-auto" alt="PatchMon Logo" />
								</Link>
							)}
						</div>
						<nav className="flex-1 flex flex-col overflow-y-auto overflow-x-hidden min-h-0">
							<ul className="flex flex-col space-y-1">
								{/* Show message for users with very limited permissions */}
								{navigation.length === 0 && settingsNavigation.length === 0 && (
									<li className="px-2 py-4 text-center">
										<div className="text-sm text-secondary-500 dark:text-white/70">
											<p className="mb-2">Limited access</p>
											<p className="text-xs">
												Contact your administrator for additional permissions
											</p>
										</div>
									</li>
								)}
								{navigation.map((item) => {
									if (item.name) {
										// Single item (Dashboard)
										return (
											<li key={item.name} className="mb-1">
												<Link
													to={item.href}
													className={`group flex items-center gap-x-2.5 rounded-lg text-sm leading-6 font-medium transition-all duration-200 min-h-[36px] ${
														isActive(item.href)
															? "bg-primary-100 dark:bg-primary-600 text-primary-900 dark:text-white"
															: "text-secondary-700 dark:text-secondary-200 hover:text-primary-700 dark:hover:text-primary-300 hover:bg-secondary-50 dark:hover:bg-secondary-700"
													} ${sidebarCollapsed ? "justify-center px-2 py-1.5" : "px-2 py-2"}`}
													onMouseEnter={() => prefetchRoute(item.href)}
													title={sidebarCollapsed ? item.name : ""}
												>
													<item.icon
														className={`h-5 w-5 shrink-0 ${sidebarCollapsed ? "mx-auto" : ""}`}
													/>
													{!sidebarCollapsed && (
														<span className="truncate">{item.name}</span>
													)}
												</Link>
											</li>
										);
									} else if (item.section) {
										// Special handling for LINKS section
										if (item.section === "LINKS") {
											return (
												<li key={item.section} className="mt-4">
													{!sidebarCollapsed && (
														<h3 className="text-xs font-semibold text-secondary-500 dark:text-white uppercase tracking-wider mb-2">
															{item.section}
														</h3>
													)}
													{!sidebarCollapsed ? (
														<div className="flex items-center justify-center gap-2">
															{item.items.map((linkItem) => (
																<a
																	key={linkItem.name}
																	href={linkItem.href}
																	target={
																		linkItem.href.startsWith("http")
																			? "_blank"
																			: undefined
																	}
																	rel={
																		linkItem.href.startsWith("http")
																			? "noopener noreferrer"
																			: undefined
																	}
																	className="flex items-center justify-center w-10 h-10 bg-secondary-50 dark:bg-secondary-800 text-secondary-600 dark:text-white hover:bg-secondary-100 dark:hover:bg-secondary-700 rounded-lg transition-colors"
																	title={linkItem.name}
																>
																	<linkItem.icon className="h-5 w-5" />
																</a>
															))}
														</div>
													) : (
														<div className="flex flex-col items-center gap-1">
															{item.items.map((linkItem) => (
																<a
																	key={linkItem.name}
																	href={linkItem.href}
																	target={
																		linkItem.href.startsWith("http")
																			? "_blank"
																			: undefined
																	}
																	rel={
																		linkItem.href.startsWith("http")
																			? "noopener noreferrer"
																			: undefined
																	}
																	className="flex items-center justify-center w-10 h-10 bg-secondary-50 dark:bg-secondary-800 text-secondary-600 dark:text-white hover:bg-secondary-100 dark:hover:bg-secondary-700 rounded-lg transition-colors"
																	title={linkItem.name}
																>
																	<linkItem.icon className="h-5 w-5" />
																</a>
															))}
														</div>
													)}
												</li>
											);
										}
										// Section with items
										return (
											<li
												key={item.section}
												className="mt-2 pt-2 border-t border-secondary-100 dark:border-white/5 first:mt-0 first:pt-0 first:border-t-0"
											>
												{!sidebarCollapsed && (
													<h3 className="text-[11px] font-medium text-secondary-400 dark:text-white/50 uppercase tracking-widest mb-1.5 px-2">
														{item.section}
													</h3>
												)}
												<ul className="space-y-0.5">
													{item.items.map((subItem) => {
														return (
															<li key={subItem.name}>
																{subItem.name === "Hosts" &&
																canManageHosts() ? (
																	// Special handling for Hosts item with integrated + button
																	<div className="flex items-center gap-1">
																		<Link
																			to={subItem.href}
																			className={`group flex items-center gap-x-2.5 rounded-lg text-sm leading-6 font-medium transition-all duration-200 flex-1 min-h-[36px] ${
																				isActive(subItem.href)
																					? "bg-primary-100 dark:bg-primary-600 text-primary-900 dark:text-white"
																					: "text-secondary-700 dark:text-secondary-200 hover:text-primary-700 dark:hover:text-primary-300 hover:bg-secondary-50 dark:hover:bg-secondary-700"
																			} ${sidebarCollapsed ? "justify-center px-2 py-1.5" : "px-2 py-2"}`}
																			onMouseEnter={() =>
																				prefetchRoute(subItem.href)
																			}
																			title={
																				sidebarCollapsed ? subItem.name : ""
																			}
																		>
																			<subItem.icon
																				className={`h-5 w-5 shrink-0 ${sidebarCollapsed ? "mx-auto" : ""}`}
																			/>
																			{!sidebarCollapsed && (
																				<span className="truncate flex items-center gap-2 flex-1">
																					{subItem.name}
																					{subItem.name === "Hosts" &&
																						hosts &&
																						Array.isArray(hosts) &&
																						hosts.length > 0 && (
																							<div className="ml-2 flex items-center gap-1">
																								{(() => {
																									// Use the exact same logic as Hosts.jsx page
																									const connectedCount =
																										hosts?.filter(
																											(h) =>
																												wsStatusMap[h.api_id]
																													?.connected === true,
																										).length || 0;
																									const offlineCount =
																										hosts?.filter(
																											(h) =>
																												wsStatusMap[h.api_id]
																													?.connected !== true,
																										).length || 0;

																									// If we have WebSocket data, show connected/disconnected badges
																									// Otherwise show total count as fallback
																									if (
																										Object.keys(wsStatusMap)
																											.length > 0
																									) {
																										return (
																											<>
																												{connectedCount > 0 && (
																													<span className="inline-flex items-center justify-center px-1.5 py-0.5 text-xs rounded bg-green-100 text-green-700 dark:bg-green-900 dark:text-green-200">
																														{connectedCount}
																													</span>
																												)}
																												{offlineCount > 0 && (
																													<span className="inline-flex items-center justify-center px-1.5 py-0.5 text-xs rounded bg-red-100 text-red-700 dark:bg-red-900 dark:text-red-200">
																														{offlineCount}
																													</span>
																												)}
																											</>
																										);
																									}

																									// Fallback: show total count if WebSocket status not available yet
																									return (
																										<span className="inline-flex items-center justify-center px-1.5 py-0.5 text-xs rounded bg-secondary-100 text-secondary-700 dark:bg-secondary-600 dark:text-secondary-200">
																											{hosts?.length || 0}
																										</span>
																									);
																								})()}
																							</div>
																						)}
																					{/* {subItem.name === "Packages" &&
																				stats?.cards?.totalOutdatedPackages !==
																					undefined && (
																					<span className="ml-2 inline-flex items-center justify-center px-1.5 py-0.5 text-xs rounded bg-secondary-100 text-secondary-700">
																						{stats.cards.totalOutdatedPackages}
																					</span>
																				)} */}
																					{/* {subItem.name === "Repos" &&
																				stats?.cards?.totalRepos !==
																					undefined && (
																					<span className="ml-2 inline-flex items-center justify-center px-1.5 py-0.5 text-xs rounded bg-secondary-100 text-secondary-700">
																						{stats.cards.totalRepos}
																					</span>
																				)} */}
																				</span>
																			)}
																			{!sidebarCollapsed && (
																				<button
																					type="button"
																					onClick={(e) => {
																						e.preventDefault();
																						handleAddHost();
																					}}
																					className="ml-auto flex items-center justify-center w-5 h-5 rounded-full border-2 border-current opacity-60 hover:opacity-100 transition-all duration-200 self-center"
																					title="Add Host"
																				>
																					<Plus className="h-3 w-3" />
																				</button>
																			)}
																		</Link>
																	</div>
																) : (
																	// Standard navigation item
																	<>
																		<Link
																			to={subItem.href}
																			className={`group flex items-center gap-x-2.5 rounded-lg text-sm leading-6 font-medium transition-all duration-200 min-h-[36px] ${
																				isActive(subItem.href)
																					? "bg-primary-100 dark:bg-primary-600 text-primary-900 dark:text-white"
																					: "text-secondary-700 dark:text-secondary-200 hover:text-primary-700 dark:hover:text-primary-300 hover:bg-secondary-50 dark:hover:bg-secondary-700"
																			} ${sidebarCollapsed ? "justify-center px-2 py-1.5 relative" : "px-2 py-2"} ${
																				subItem.comingSoon
																					? "opacity-50 cursor-not-allowed"
																					: ""
																			}`}
																			title={
																				sidebarCollapsed ? subItem.name : ""
																			}
																			onMouseEnter={() =>
																				!subItem.comingSoon &&
																				prefetchRoute(subItem.href)
																			}
																			onClick={(e) => {
																				if (subItem.comingSoon) {
																					e.preventDefault();
																					return;
																				}
																				if (subItem.children) {
																					if (subItem.href.startsWith("#")) {
																						e.preventDefault();
																					}
																					setExpandedNav(
																						expandedNav === subItem.name
																							? null
																							: subItem.name,
																					);
																				}
																			}}
																		>
																			<div
																				className={`flex items-center ${sidebarCollapsed ? "justify-center" : ""}`}
																			>
																				<subItem.icon
																					className={`h-5 w-5 shrink-0 ${sidebarCollapsed ? "mx-auto" : ""}`}
																				/>
																				{sidebarCollapsed &&
																					subItem.showUpgradeIcon && (
																						<UpgradeNotificationIcon className="h-3 w-3 absolute -top-1 -right-1" />
																					)}
																			</div>
																			{!sidebarCollapsed && (
																				<span className="truncate flex items-center gap-2 flex-1">
																					{subItem.name}
																					{subItem.name === "Hosts" &&
																						stats?.cards?.totalHosts !==
																							undefined && (
																							<span className="ml-2 inline-flex items-center justify-center px-1.5 py-0.5 text-xs rounded bg-secondary-100 text-secondary-700">
																								{stats.cards.totalHosts}
																							</span>
																						)}
																					{subItem.name === "Reporting" &&
																						alertStats && (
																							<div className="ml-2 flex items-center gap-0.5">
																								{(alertStats.informational ||
																									0) > 0 && (
																									<span className="inline-flex items-center justify-center px-1.5 py-0.5 text-xs rounded bg-blue-100 text-blue-700 dark:bg-blue-900 dark:text-blue-200">
																										{alertStats.informational}
																									</span>
																								)}
																								{(alertStats.warning || 0) >
																									0 && (
																									<span className="inline-flex items-center justify-center px-1.5 py-0.5 text-xs rounded bg-yellow-100 text-yellow-700 dark:bg-yellow-900 dark:text-yellow-200">
																										{alertStats.warning}
																									</span>
																								)}
																								{(alertStats.error || 0) >
																									0 && (
																									<span className="inline-flex items-center justify-center px-1.5 py-0.5 text-xs rounded bg-orange-100 text-orange-700 dark:bg-orange-900 dark:text-orange-200">
																										{alertStats.error}
																									</span>
																								)}
																								{(alertStats.critical || 0) >
																									0 && (
																									<span className="inline-flex items-center justify-center px-1.5 py-0.5 text-xs rounded bg-red-100 text-red-700 dark:bg-red-900 dark:text-red-200">
																										{alertStats.critical}
																									</span>
																								)}
																							</div>
																						)}
																					{subItem.comingSoon && (
																						<span className="text-xs bg-secondary-100 text-secondary-600 px-1.5 py-0.5 rounded">
																							Soon
																						</span>
																					)}
																					{subItem.alpha && (
																						<span className="text-[10px] bg-purple-50 dark:bg-purple-900/50 text-purple-600 dark:text-purple-300 px-1 py-px rounded font-medium leading-tight">
																							Alpha
																						</span>
																					)}
																					{subItem.beta && (
																						<span className="text-[10px] bg-blue-50 dark:bg-blue-900/50 text-blue-600 dark:text-blue-300 px-1 py-px rounded font-medium leading-tight">
																							Beta
																						</span>
																					)}
																					{subItem.new && (
																						<span className="text-[10px] bg-green-50 dark:bg-green-900/50 text-green-600 dark:text-green-300 px-1 py-px rounded font-medium leading-tight">
																							New
																						</span>
																					)}
																					{subItem.lockedTier && (
																						<TierBadge
																							tier={subItem.lockedTier}
																						/>
																					)}
																					{subItem.showUpgradeIcon && (
																						<UpgradeNotificationIcon className="h-3 w-3" />
																					)}
																					{subItem.children &&
																						!sidebarCollapsed && (
																							<ChevronDown
																								className={`ml-auto h-4 w-4 shrink-0 text-secondary-400 transition-transform duration-200 ${
																									expandedNav === subItem.name
																										? "rotate-180"
																										: ""
																								}`}
																							/>
																						)}
																				</span>
																			)}
																		</Link>
																		{/* Expandable children */}
																		{subItem.children &&
																			!sidebarCollapsed &&
																			expandedNav === subItem.name && (
																				<ul className="ml-7 mt-0.5 space-y-0.5 border-l border-secondary-200 dark:border-secondary-700 pl-2">
																					{subItem.children.map((child) => (
																						<li key={child.name}>
																							{child.external ? (
																								<a
																									href={child.href}
																									target={
																										child.href.startsWith(
																											"mailto:",
																										)
																											? undefined
																											: "_blank"
																									}
																									rel="noopener noreferrer"
																									className="block text-[13px] py-1 px-2 rounded transition-colors text-secondary-500 dark:text-white hover:text-secondary-900 dark:hover:text-primary-400"
																								>
																									{child.name}
																								</a>
																							) : (
																								<Link
																									to={child.href}
																									className={`text-[13px] py-1 px-2 rounded transition-colors flex items-center gap-2 ${
																										location.pathname +
																											location.search ===
																										child.href
																											? "text-primary-600 dark:text-primary-400 font-medium"
																											: "text-secondary-500 dark:text-white hover:text-secondary-900 dark:hover:text-primary-400"
																									}`}
																								>
																									<span>{child.name}</span>
																									{child.lockedTier && (
																										<TierBadge
																											tier={child.lockedTier}
																										/>
																									)}
																								</Link>
																							)}
																						</li>
																					))}
																				</ul>
																			)}
																	</>
																)}
															</li>
														);
													})}
												</ul>
											</li>
										);
									}
									return null;
								})}
							</ul>
						</nav>

						{/* Profile - Bottom of Sidebar */}
						<div className="flex-shrink-0 px-2 pb-1">
							{!sidebarCollapsed ? (
								<div>
									<div className="flex items-center justify-between">
										<Link
											to="/settings/profile"
											className={`flex-1 min-w-0 rounded-md px-2 py-1.5 transition-all duration-200 ${
												isActive("/settings/profile")
													? "bg-primary-50 dark:bg-primary-600"
													: "hover:bg-secondary-50 dark:hover:bg-secondary-700"
											}`}
										>
											<div className="flex items-center gap-x-3">
												{isRenderableAvatarSrc(user?.avatar_url) ? (
													<img
														src={user.avatar_url}
														alt={user.username}
														className="h-5 w-5 shrink-0 rounded-full object-cover"
													/>
												) : (
													<UserCircle
														className={`h-5 w-5 shrink-0 ${
															isActive("/settings/profile")
																? "text-primary-700 dark:text-white"
																: "text-secondary-500 dark:text-white"
														}`}
													/>
												)}
												<div className="flex flex-col min-w-0">
													<span
														className={`text-sm leading-6 font-semibold truncate ${
															isActive("/settings/profile")
																? "text-primary-700 dark:text-white"
																: "text-secondary-700 dark:text-secondary-200"
														}`}
													>
														{user?.first_name || user?.username}
													</span>
													{(user?.role === "admin" ||
														user?.role === "superadmin") && (
														<span
															className={`inline-flex items-center text-xs leading-4 px-1.5 py-0.5 rounded ${
																user?.role === "superadmin"
																	? "bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200"
																	: "bg-primary-100 text-primary-800 dark:bg-primary-900 dark:text-primary-200"
															}`}
														>
															<Shield className="h-3 w-3 mr-1" />
															{user?.role === "superadmin"
																? "Super Admin"
																: "Admin"}
														</span>
													)}
												</div>
											</div>
										</Link>
										<button
											type="button"
											onClick={handleLogout}
											className="ml-2 p-2 text-secondary-400 hover:text-secondary-600 dark:hover:text-secondary-300 hover:bg-secondary-100 dark:hover:bg-secondary-700 rounded-md transition-colors"
											title="Sign out"
										>
											<LogOut className="h-4 w-4" />
										</button>
									</div>
									{stats && (
										<div className="px-2">
											<div className="flex items-center gap-x-1 text-[11px] text-secondary-400 dark:text-white/50">
												<Clock className="h-3 w-3 flex-shrink-0" />
												<span className="truncate">
													Updated: {formatRelativeTimeShort(stats.lastUpdated)}
												</span>
												<button
													type="button"
													onClick={() => refetch()}
													disabled={isFetching}
													className="p-1 hover:bg-secondary-100 dark:hover:bg-secondary-700 rounded flex-shrink-0 disabled:opacity-50"
													title="Refresh data"
												>
													<RefreshCw
														className={`h-3 w-3 ${isFetching ? "animate-spin" : ""}`}
													/>
												</button>
											</div>
										</div>
									)}
								</div>
							) : (
								<div className="space-y-1">
									<Link
										to="/settings/profile"
										className={`flex items-center justify-center p-2 rounded-md transition-colors ${
											isActive("/settings/profile")
												? "bg-primary-50 dark:bg-primary-600 text-primary-700 dark:text-white"
												: "text-secondary-700 dark:text-secondary-200 hover:bg-secondary-50 dark:hover:bg-secondary-700"
										}`}
										title={`My Profile (${user?.username})`}
									>
										{isRenderableAvatarSrc(user?.avatar_url) ? (
											<img
												src={user.avatar_url}
												alt={user.username}
												className="h-5 w-5 rounded-full object-cover"
											/>
										) : (
											<UserCircle className="h-5 w-5" />
										)}
									</Link>
									<button
										type="button"
										onClick={handleLogout}
										className="flex items-center justify-center w-full p-2 text-secondary-400 hover:text-secondary-600 hover:bg-secondary-100 dark:hover:bg-secondary-700 rounded-md transition-colors"
										title="Sign out"
									>
										<LogOut className="h-4 w-4" />
									</button>
									{/* Updated info for collapsed sidebar */}
									{stats && (
										<div className="flex flex-col items-center py-1 border-t border-secondary-200 dark:border-secondary-700">
											<button
												type="button"
												onClick={() => refetch()}
												disabled={isFetching}
												className="p-1 hover:bg-secondary-100 dark:hover:bg-secondary-700 rounded disabled:opacity-50"
												title={`Refresh data - Updated: ${formatRelativeTimeShort(stats.lastUpdated)}`}
											>
												<RefreshCw
													className={`h-3 w-3 ${isFetching ? "animate-spin" : ""}`}
												/>
											</button>
										</div>
									)}
								</div>
							)}
						</div>
					</div>
				</div>

				{/* Main content — tracks the effective sidebar state (including hover
				    expansion) so the page reflows instead of being overlapped by the sidebar. */}
				<div
					className={`flex flex-col min-h-screen transition-all duration-300 relative z-10 ${
						sidebarCollapsed ? "lg:pl-16" : "lg:pl-64"
					}`}
				>
					{/* Top bar */}
					<div
						className={`fixed top-0 z-[90] flex h-16 shrink-0 items-center gap-x-2 sm:gap-x-4 border-b border-secondary-200 dark:border-white/10 bg-white px-3 sm:px-4 sm:px-6 lg:px-8 shadow-sm transition-all duration-300 ${
							sidebarCollapsed
								? "lg:left-16 lg:right-0"
								: "lg:left-64 lg:right-0"
						} left-0 right-0`}
						style={{
							backgroundColor: "var(--topbar-bg, white)",
							backdropFilter: "var(--topbar-blur, none)",
							WebkitBackdropFilter: "var(--topbar-blur, none)",
						}}
					>
						<button
							type="button"
							className="-m-2.5 p-2.5 text-secondary-700 dark:text-white lg:hidden min-w-[44px] min-h-[44px] flex items-center justify-center"
							onClick={() => setSidebarOpen(true)}
							aria-label="Open menu"
						>
							<Menu className="h-6 w-6" />
						</button>

						{/* Separator */}
						<div className="h-6 w-px bg-secondary-200 dark:bg-secondary-600 lg:hidden" />

						<div className="flex flex-1 gap-x-2 sm:gap-x-4 self-stretch lg:gap-x-6 min-w-0">
							{/* Page title - hidden on dashboard, hosts, repositories, packages, automation, compliance, docker, settings, and host details to give more space to search */}
							{![
								"/",
								"/hosts",
								"/repositories",
								"/packages",
								"/reporting",
								"/automation",
								"/compliance",
								"/docker",
								"/patching",
							].includes(location.pathname) &&
								!location.pathname.startsWith("/hosts/") &&
								!location.pathname.startsWith("/compliance/") &&
								!location.pathname.startsWith("/docker/") &&
								!location.pathname.startsWith("/packages/") &&
								!location.pathname.startsWith("/patching/") &&
								!location.pathname.startsWith("/settings/") && (
									<div className="relative flex items-center flex-shrink-0">
										<h2 className="text-base sm:text-lg font-semibold text-secondary-900 dark:text-secondary-100 whitespace-nowrap">
											{getPageTitle()}
										</h2>
									</div>
								)}

							{/* Global Search Bar */}
							<div
								className={`flex items-center min-w-0 ${["/", "/hosts", "/repositories", "/packages", "/reporting", "/automation", "/compliance", "/docker", "/patching"].includes(location.pathname) || location.pathname.startsWith("/hosts/") || location.pathname.startsWith("/compliance/") || location.pathname.startsWith("/docker/") || location.pathname.startsWith("/packages/") || location.pathname.startsWith("/patching/") || location.pathname.startsWith("/settings/") ? "flex-1 max-w-none" : "flex-1 md:flex-none md:max-w-sm"}`}
							>
								<GlobalSearch />
							</div>

							<div className="flex items-center gap-x-2 sm:gap-x-4 lg:gap-x-6 justify-end flex-shrink-0">
								{/* Mobile External Links Menu */}
								<div className="relative md:hidden">
									<button
										type="button"
										onClick={() => setMobileLinksOpen(!mobileLinksOpen)}
										className="flex items-center justify-center w-10 h-10 bg-gray-50 dark:bg-transparent text-secondary-600 dark:text-white hover:bg-gray-100 dark:hover:bg-white/10 rounded-lg transition-colors shadow-sm min-w-[44px] min-h-[44px]"
										style={{
											backgroundColor: "var(--button-bg, rgb(249, 250, 251))",
											backdropFilter: "var(--button-blur, none)",
											WebkitBackdropFilter: "var(--button-blur, none)",
										}}
										aria-label="External links"
										aria-expanded={mobileLinksOpen}
									>
										<Globe className="h-5 w-5" />
									</button>
									{mobileLinksOpen && (
										<>
											<button
												type="button"
												className="fixed inset-0 z-40 bg-transparent border-0 p-0 cursor-default"
												onClick={() => setMobileLinksOpen(false)}
												onKeyDown={(e) => {
													if (e.key === "Enter" || e.key === " ") {
														e.preventDefault();
														setMobileLinksOpen(false);
													}
												}}
												aria-label="Close mobile menu"
											/>
											<div className="absolute right-0 mt-2 w-64 rounded-lg border border-secondary-200 dark:border-secondary-600 bg-white dark:bg-secondary-800 shadow-lg z-50 max-h-[80vh] overflow-y-auto">
												<div className="p-2 space-y-1">
													{communityLinks
														.filter((l) =>
															[
																"github",
																"discord",
																"linkedin",
																"youtube",
															].includes(l.id),
														)
														.map((link) => {
															const Icon =
																link.id === "discord"
																	? DiscordIcon
																	: link.id === "github"
																		? Github
																		: link.id === "linkedin"
																			? FaLinkedin
																			: FaYoutube;
															return (
																<a
																	key={link.id}
																	href={link.url}
																	target="_blank"
																	rel="noopener noreferrer"
																	className="flex items-center gap-3 px-3 py-3 bg-gray-50 dark:bg-gray-800 text-secondary-600 dark:text-white hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg transition-colors min-h-[44px]"
																	onClick={() => setMobileLinksOpen(false)}
																>
																	<Icon
																		className={`h-5 w-5 flex-shrink-0 ${
																			link.id === "discord"
																				? "text-[#5865F2]"
																				: link.id === "linkedin"
																					? "text-[#0077B5]"
																					: link.id === "youtube"
																						? "text-[#FF0000]"
																						: ""
																		}`}
																	/>
																	<span className="text-sm font-medium flex-1">
																		{link.label}
																	</span>
																	{link.stat && (
																		<div className="flex items-center gap-1">
																			{link.statLabel === "stars" && (
																				<Star className="h-4 w-4 fill-current text-yellow-500" />
																			)}
																			<span className="text-sm">
																				{link.stat}
																			</span>
																		</div>
																	)}
																</a>
															);
														})}
												</div>
											</div>
										</>
									)}
								</div>

								{/* Donate Button — hidden when buymeacoffee link is absent (e.g. AdminMode) */}
								{communityLinks.some((l) => l.id === "buymeacoffee") && (
									<button
										type="button"
										onClick={() => setShowDonateModal(true)}
										className="hidden md:flex items-center justify-center gap-1.5 px-2.5 h-10 bg-gray-50 dark:bg-transparent text-secondary-600 dark:text-white hover:bg-gray-100 dark:hover:bg-white/10 rounded-lg transition-colors shadow-sm"
										style={{
											backgroundColor: "var(--button-bg, rgb(249, 250, 251))",
											backdropFilter: "var(--button-blur, none)",
											WebkitBackdropFilter: "var(--button-blur, none)",
										}}
										title="Donate a coffee"
									>
										<BuyMeACoffeeIcon className="h-5 w-5 text-yellow-500 flex-shrink-0" />
									</button>
								)}

								{/* Desktop External Links */}
								<div className="hidden md:flex items-center gap-1">
									{communityLinks
										.filter((l) =>
											["github", "discord", "linkedin", "youtube"].includes(
												l.id,
											),
										)
										.map((link) => {
											const Icon =
												link.id === "discord"
													? DiscordIcon
													: link.id === "github"
														? Github
														: link.id === "linkedin"
															? FaLinkedin
															: FaYoutube;
											return (
												<a
													key={link.id}
													href={link.url}
													target="_blank"
													rel="noopener noreferrer"
													className="flex items-center justify-center gap-1.5 w-auto px-2.5 h-10 bg-gray-50 dark:bg-transparent text-secondary-600 dark:text-white hover:bg-gray-100 dark:hover:bg-white/10 rounded-lg transition-colors shadow-sm"
													style={{
														backgroundColor:
															"var(--button-bg, rgb(249, 250, 251))",
														backdropFilter: "var(--button-blur, none)",
														WebkitBackdropFilter: "var(--button-blur, none)",
													}}
													title={link.label}
													aria-label={link.label}
												>
													<Icon
														className={`h-5 w-5 flex-shrink-0 ${
															link.id === "discord"
																? "text-[#5865F2]"
																: link.id === "linkedin"
																	? "text-[#0077B5]"
																	: link.id === "youtube"
																		? "text-[#FF0000]"
																		: ""
														}`}
													/>
													{link.stat && (
														<div className="flex items-center gap-1">
															{link.statLabel === "stars" && (
																<Star className="h-4 w-4 fill-current text-yellow-500" />
															)}
															<span className="text-sm font-medium">
																{link.stat}
															</span>
														</div>
													)}
												</a>
											);
										})}
								</div>
							</div>
						</div>
					</div>

					<main className="flex-1 py-6 bg-secondary-50 dark:bg-transparent pt-24">
						<div className="px-4 sm:px-6 lg:px-8">{content}</div>
					</main>
				</div>

				{/* Release Notes Modal */}
				<ReleaseNotesModal
					isOpen={showReleaseNotes}
					onAccept={() => setShowReleaseNotes(false)}
				/>

				{/* Donate Modal */}
				<DonateModal
					isOpen={showDonateModal}
					onClose={() => setShowDonateModal(false)}
				/>
			</div>
		</SidebarContext.Provider>
	);
};

export default Layout;
