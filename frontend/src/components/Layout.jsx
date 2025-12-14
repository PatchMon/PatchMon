import { useQuery } from "@tanstack/react-query";
import {
	Activity,
	BarChart3,
	BookOpen,
	ChevronLeft,
	ChevronRight,
	Clock,
	Container,
	GitBranch,
	Github,
	Globe,
	Home,
	LogOut,
	Mail,
	Menu,
	Package,
	Plus,
	RefreshCw,
	Route,
	Server,
	Settings,
	Star,
	UserCircle,
	X,
	Zap,
} from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { FaReddit, FaYoutube } from "react-icons/fa";
import { Link, useLocation, useNavigate } from "react-router-dom";
import { useAuth } from "../contexts/AuthContext";
import { useColorTheme } from "../contexts/ColorThemeContext";
import { useUpdateNotification } from "../contexts/UpdateNotificationContext";
import { dashboardAPI, versionAPI } from "../utils/api";
import DiscordIcon from "./DiscordIcon";
import GlobalSearch from "./GlobalSearch";
import Logo from "./Logo";
import UpgradeNotificationIcon from "./UpgradeNotificationIcon";

const Layout = ({ children }) => {
	const [sidebarOpen, setSidebarOpen] = useState(false);
	const [sidebarCollapsed, setSidebarCollapsed] = useState(() => {
		// Load sidebar state from localStorage, default to false
		const saved = localStorage.getItem("sidebarCollapsed");
		return saved ? JSON.parse(saved) : false;
	});
	const [_userMenuOpen, setUserMenuOpen] = useState(false);
	const [githubStars, setGithubStars] = useState(null);
	const [mobileLinksOpen, setMobileLinksOpen] = useState(false);
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
	} = useAuth();
	const { updateAvailable } = useUpdateNotification();
	const { themeConfig } = useColorTheme();
	const userMenuRef = useRef(null);
	const bgCanvasRef = useRef(null);

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

	// Fetch version info
	const { data: versionInfo } = useQuery({
		queryKey: ["versionInfo"],
		queryFn: () => versionAPI.getCurrent().then((res) => res.data),
		staleTime: 300000, // Consider data stale after 5 minutes
	});

	// Build navigation based on permissions
	const buildNavigation = () => {
		const nav = [];

		// Dashboard - only show if user can view dashboard
		if (canViewDashboard()) {
			nav.push({ name: "Dashboard", href: "/", icon: Home });
		}

		// Inventory section - only show if user has any inventory permissions
		if (canViewHosts() || canViewPackages() || canViewReports()) {
			const inventoryItems = [];

			if (canViewHosts()) {
				inventoryItems.push({ name: "Hosts", href: "/hosts", icon: Server });
				inventoryItems.push({
					name: "Repos",
					href: "/repositories",
					icon: GitBranch,
				});
			}

			if (canViewPackages()) {
				inventoryItems.push({
					name: "Packages",
					href: "/packages",
					icon: Package,
				});
			}

			// Add Automation item (available to all users with inventory access)
			inventoryItems.push({
				name: "Automation",
				href: "/automation",
				icon: RefreshCw,
			});

			if (canViewReports()) {
				inventoryItems.push(
					{
						name: "Docker",
						href: "/docker",
						icon: Container,
						beta: true,
					},
					{
						name: "Services",
						href: "/services",
						icon: Activity,
						comingSoon: true,
					},
					{
						name: "Reporting",
						href: "/reporting",
						icon: BarChart3,
						comingSoon: true,
					},
				);
			}

			// Add Pro-Action item (available to all users with inventory access)
			inventoryItems.push({
				name: "Pro-Action",
				href: "/pro-action",
				icon: Zap,
				comingSoon: true,
			});

			if (inventoryItems.length > 0) {
				nav.push({
					section: "Inventory",
					items: inventoryItems,
				});
			}
		}

		return nav;
	};

	// Build settings navigation separately (for bottom placement)
	const buildSettingsNavigation = () => {
		const settingsNav = [];

		// Settings section - consolidated all settings into one page
		if (
			canManageSettings() ||
			canViewUsers() ||
			canManageUsers() ||
			canViewReports() ||
			canExportData()
		) {
			const settingsItems = [];

			settingsItems.push({
				name: "Settings",
				href: "/settings/users",
				icon: Settings,
				showUpgradeIcon: updateAvailable,
			});

			settingsNav.push({
				section: "Settings",
				items: settingsItems,
			});
		}

		return settingsNav;
	};

	const navigation = buildNavigation();
	const settingsNavigation = buildSettingsNavigation();

	const isActive = (path) => location.pathname === path;

	// Get page title based on current route
	const getPageTitle = () => {
		const path = location.pathname;

		if (path === "/") return "Dashboard";
		if (path === "/hosts") return "Hosts";
		if (path === "/packages") return "Packages";
		if (path === "/repositories" || path.startsWith("/repositories/"))
			return "Repositories";
		if (path === "/services") return "Services";
		if (path === "/docker") return "Docker";
		if (path === "/pro-action") return "Pro-Action";
		if (path === "/automation") return "Automation";
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
		setUserMenuOpen(false);
	};

	const handleAddHost = () => {
		// Navigate to hosts page with add modal parameter
		navigate("/hosts?action=add");
	};

	// Generate clean radial gradient background with subtle triangular accents for dark mode
	useEffect(() => {
		const generateBackground = () => {
			if (
				!bgCanvasRef.current ||
				!themeConfig?.login ||
				!document.documentElement.classList.contains("dark")
			) {
				return;
			}

			const canvas = bgCanvasRef.current;
			canvas.width = window.innerWidth;
			canvas.height = window.innerHeight;
			const ctx = canvas.getContext("2d");

			// Get theme colors - pick first color from each palette
			const xColors = themeConfig.login.xColors || [
				"#667eea",
				"#764ba2",
				"#f093fb",
				"#4facfe",
			];
			const yColors = themeConfig.login.yColors || [
				"#667eea",
				"#764ba2",
				"#f093fb",
				"#4facfe",
			];

			// Use date for daily color rotation
			const today = new Date();
			const seed =
				today.getFullYear() * 10000 + today.getMonth() * 100 + today.getDate();
			const random = (s) => {
				const x = Math.sin(s) * 10000;
				return x - Math.floor(x);
			};

			const color1 = xColors[Math.floor(random(seed) * xColors.length)];
			const color2 = yColors[Math.floor(random(seed + 1000) * yColors.length)];

			// Create clean radial gradient from center to bottom-right corner
			const gradient = ctx.createRadialGradient(
				canvas.width * 0.3, // Center slightly left
				canvas.height * 0.3, // Center slightly up
				0,
				canvas.width * 0.5, // Expand to cover screen
				canvas.height * 0.5,
				Math.max(canvas.width, canvas.height) * 1.2,
			);

			// Subtle gradient with darker corners
			gradient.addColorStop(0, color1);
			gradient.addColorStop(0.6, color2);
			gradient.addColorStop(1, "#0a0a0a"); // Very dark edges

			ctx.fillStyle = gradient;
			ctx.fillRect(0, 0, canvas.width, canvas.height);

			// Add subtle triangular shapes as accents across entire background
			const cellSize = 180;
			const cols = Math.ceil(canvas.width / cellSize) + 1;
			const rows = Math.ceil(canvas.height / cellSize) + 1;

			for (let y = 0; y < rows; y++) {
				for (let x = 0; x < cols; x++) {
					const idx = y * cols + x;
					// Draw more triangles (less sparse)
					if (random(seed + idx + 5000) > 0.4) {
						const baseX =
							x * cellSize + random(seed + idx * 3) * cellSize * 0.8;
						const baseY =
							y * cellSize + random(seed + idx * 3 + 100) * cellSize * 0.8;
						const size = 50 + random(seed + idx * 4) * 100;

						ctx.beginPath();
						ctx.moveTo(baseX, baseY);
						ctx.lineTo(baseX + size, baseY);
						ctx.lineTo(baseX + size / 2, baseY - size * 0.866);
						ctx.closePath();

						// More visible white with slightly higher opacity
						ctx.fillStyle = `rgba(255, 255, 255, ${0.05 + random(seed + idx * 5) * 0.08})`;
						ctx.fill();
					}
				}
			}
		};

		generateBackground();

		// Regenerate on window resize or theme change
		const handleResize = () => {
			generateBackground();
		};

		window.addEventListener("resize", handleResize);

		// Watch for dark mode changes
		const observer = new MutationObserver((mutations) => {
			mutations.forEach((mutation) => {
				if (mutation.attributeName === "class") {
					generateBackground();
				}
			});
		});

		observer.observe(document.documentElement, {
			attributes: true,
			attributeFilter: ["class"],
		});

		return () => {
			window.removeEventListener("resize", handleResize);
			observer.disconnect();
		};
	}, [themeConfig]);

	// Fetch GitHub stars count
	const fetchGitHubStars = useCallback(async () => {
		// Try to load cached star count first
		const cachedStars = localStorage.getItem("githubStarsCount");
		if (cachedStars) {
			setGithubStars(parseInt(cachedStars, 10));
		}

		// Skip API call if fetched recently
		const lastFetch = localStorage.getItem("githubStarsFetchTime");
		const now = Date.now();
		if (lastFetch && now - parseInt(lastFetch, 10) < 600000) {
			// 10 minute cache
			return;
		}

		try {
			const response = await fetch(
				"https://api.github.com/repos/9technologygroup/patchmon.net",
				{
					headers: {
						Accept: "application/vnd.github.v3+json",
					},
				},
			);

			if (response.ok) {
				const data = await response.json();
				setGithubStars(data.stargazers_count);
				localStorage.setItem(
					"githubStarsCount",
					data.stargazers_count.toString(),
				);
				localStorage.setItem("githubStarsFetchTime", now.toString());
			} else if (response.status === 403 || response.status === 429) {
				console.warn("GitHub API rate limit exceeded, using cached value");
			}
		} catch (error) {
			console.error("Failed to fetch GitHub stars:", error);
			// Keep using cached value if available
		}
	}, []);

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

	// Save sidebar collapsed state to localStorage
	useEffect(() => {
		localStorage.setItem("sidebarCollapsed", JSON.stringify(sidebarCollapsed));
	}, [sidebarCollapsed]);

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

	// Fetch GitHub stars on component mount
	useEffect(() => {
		fetchGitHubStars();
	}, [fetchGitHubStars]);

	// Set CSS custom properties for glassmorphism and theme colors in dark mode
	useEffect(() => {
		const updateThemeStyles = () => {
			const isDark = document.documentElement.classList.contains("dark");
			const root = document.documentElement;

			if (isDark && themeConfig?.app) {
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
		<div className="min-h-screen bg-secondary-50 dark:bg-black relative overflow-hidden">
			{/* Full-screen Trianglify Background (Dark Mode Only) */}
			<canvas
				ref={bgCanvasRef}
				className="fixed inset-0 w-full h-full hidden dark:block"
				style={{ zIndex: 0 }}
			/>
			<div
				className="fixed inset-0 bg-gradient-to-br from-black/10 to-black/20 hidden dark:block pointer-events-none"
				style={{ zIndex: 1 }}
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
						{navigation.length === 0 && settingsNavigation.length === 0 && (
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
												: "text-secondary-600 dark:text-secondary-300 hover:bg-secondary-50 dark:hover:bg-secondary-700 hover:text-secondary-900 dark:hover:text-white"
										}`}
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
										<h3 className="text-xs font-semibold text-secondary-500 dark:text-secondary-400 uppercase tracking-wider mb-2">
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
																		: "text-secondary-600 dark:text-secondary-300 hover:bg-secondary-50 dark:hover:bg-secondary-700 hover:text-secondary-900 dark:hover:text-white"
																}`}
																onClick={() => setSidebarOpen(false)}
															>
																<subItem.icon className="mr-3 h-5 w-5" />
																<span className="flex items-center gap-2 flex-1">
																	{subItem.name}
																	{subItem.name === "Hosts" &&
																		stats?.cards?.totalHosts !== undefined && (
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
															<Link
																to={subItem.href}
																className={`group flex items-center px-2 py-2 text-sm font-medium rounded-md ${
																	isActive(subItem.href)
																		? "bg-primary-100 dark:bg-primary-600 text-primary-900 dark:text-white"
																		: "text-secondary-600 dark:text-secondary-300 hover:bg-secondary-50 dark:hover:bg-secondary-700 hover:text-secondary-900 dark:hover:text-white"
																} ${subItem.comingSoon ? "opacity-50 cursor-not-allowed" : ""}`}
																onClick={
																	subItem.comingSoon
																		? (e) => e.preventDefault()
																		: () => setSidebarOpen(false)
																}
															>
																<subItem.icon className="mr-3 h-5 w-5" />
																<span className="flex items-center gap-2">
																	{subItem.name}
																	{subItem.name === "Hosts" &&
																		stats?.cards?.totalHosts !== undefined && (
																			<span className="ml-2 inline-flex items-center justify-center px-1.5 py-0.5 text-xs rounded bg-secondary-100 dark:bg-secondary-600 text-secondary-700 dark:text-secondary-200">
																				{stats.cards.totalHosts}
																			</span>
																		)}
																	{subItem.comingSoon && (
																		<span className="text-xs bg-secondary-100 dark:bg-secondary-600 text-secondary-600 dark:text-secondary-200 px-1.5 py-0.5 rounded">
																			Soon
																		</span>
																	)}
																	{subItem.beta && (
																		<span className="text-xs bg-blue-100 dark:bg-blue-900 text-blue-600 dark:text-blue-200 px-1.5 py-0.5 rounded font-medium">
																			Beta
																		</span>
																	)}
																	{subItem.new && (
																		<span className="text-xs bg-green-100 dark:bg-green-900 text-green-600 dark:text-green-200 px-1.5 py-0.5 rounded font-medium">
																			New
																		</span>
																	)}
																</span>
															</Link>
														)}
													</div>
												))}
										</div>
									</div>
								);
							}
							return null;
						})}

						{/* Settings Section - Mobile */}
						{settingsNavigation.map((item) => {
							if (item.section) {
								// Settings section (no heading)
								return (
									<div key={item.section}>
										<div className="space-y-1">
											{item.items
												.filter((subItem) => !subItem.comingSoon)
												.map((subItem) => (
													<Link
														key={subItem.name}
														to={subItem.href}
														className={`group flex items-center px-2 py-2 text-sm font-medium rounded-md ${
															isActive(subItem.href)
																? "bg-primary-100 dark:bg-primary-600 text-primary-900 dark:text-white"
																: "text-secondary-600 dark:text-secondary-300 hover:bg-secondary-50 dark:hover:bg-secondary-700 hover:text-secondary-900 dark:hover:text-white"
														}`}
														onClick={() => setSidebarOpen(false)}
													>
														<subItem.icon className="mr-3 h-5 w-5" />
														<span className="flex items-center gap-2">
															{subItem.name}
															{subItem.showUpgradeIcon && (
																<UpgradeNotificationIcon className="h-3 w-3" />
															)}
														</span>
													</Link>
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
											: "text-secondary-600 dark:text-secondary-300 hover:bg-secondary-50 dark:hover:bg-secondary-700 hover:text-secondary-900 dark:hover:text-white"
									}`}
									onClick={() => setSidebarOpen(false)}
								>
									<UserCircle className="mr-3 h-5 w-5" />
									<span className="flex items-center gap-2">
										{user?.first_name || user?.username}
										{user?.role === "admin" && (
											<span className="text-xs bg-secondary-100 dark:bg-secondary-600 text-secondary-600 dark:text-secondary-200 px-1.5 py-0.5 rounded">
												Admin
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
									className="w-full group flex items-center px-2 py-3 text-sm font-medium rounded-md text-secondary-600 dark:text-secondary-300 hover:bg-secondary-50 dark:hover:bg-secondary-700 hover:text-secondary-900 dark:hover:text-white min-h-[44px]"
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
					sidebarCollapsed ? "lg:w-16" : "lg:w-56"
				} bg-white dark:bg-transparent`}
			>
				{/* Collapse/Expand button on border */}
				<button
					type="button"
					onClick={() => setSidebarCollapsed(!sidebarCollapsed)}
					className="absolute top-5 -right-3 z-[200] flex items-center justify-center w-6 h-6 rounded-full bg-white border border-secondary-300 dark:border-white/20 shadow-md hover:bg-secondary-50 transition-colors"
					style={{
						backgroundColor: "var(--button-bg, white)",
						backdropFilter: "var(--button-blur, none)",
						WebkitBackdropFilter: "var(--button-blur, none)",
					}}
					title={sidebarCollapsed ? "Expand sidebar" : "Collapse sidebar"}
				>
					{sidebarCollapsed ? (
						<ChevronRight className="h-4 w-4 text-secondary-700 dark:text-white" />
					) : (
						<ChevronLeft className="h-4 w-4 text-secondary-700 dark:text-white" />
					)}
				</button>

				<div
					className={`flex grow flex-col gap-y-5 border-r border-secondary-200 dark:border-white/10 bg-white ${
						sidebarCollapsed ? "px-2 shadow-lg" : "px-6"
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
									src="/assets/favicon.svg"
									alt="PatchMon"
									className="h-12 w-12 object-contain"
								/>
							</Link>
						) : (
							<Link to="/" className="flex items-center">
								<Logo className="h-10 w-auto" alt="PatchMon Logo" />
							</Link>
						)}
					</div>
					<nav className="flex flex-1 flex-col">
						<ul className="flex flex-1 flex-col gap-y-6">
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
										<li
											key={item.name}
											className={sidebarCollapsed ? "" : "-mx-2"}
										>
											<Link
												to={item.href}
												className={`group flex gap-x-3 rounded-md text-sm leading-6 font-semibold transition-all duration-200 ${
													isActive(item.href)
														? "bg-primary-50 dark:bg-primary-600 text-primary-700 dark:text-white"
														: "text-secondary-700 dark:text-secondary-200 hover:text-primary-700 dark:hover:text-primary-300 hover:bg-secondary-50 dark:hover:bg-secondary-700"
												} ${sidebarCollapsed ? "justify-center p-2" : "p-2"}`}
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
									// Section with items
									return (
										<li key={item.section}>
											{!sidebarCollapsed && (
												<h3 className="text-xs font-semibold text-secondary-500 dark:text-secondary-300 uppercase tracking-wider mb-2">
													{item.section}
												</h3>
											)}
											<ul
												className={`space-y-1 ${sidebarCollapsed ? "" : "-mx-2"}`}
											>
												{item.items.map((subItem) => (
													<li key={subItem.name}>
														{subItem.name === "Hosts" && canManageHosts() ? (
															// Special handling for Hosts item with integrated + button
															<div className="flex items-center gap-1">
																<Link
																	to={subItem.href}
																	className={`group flex gap-x-3 rounded-md text-sm leading-6 font-medium transition-all duration-200 flex-1 ${
																		isActive(subItem.href)
																			? "bg-primary-50 dark:bg-primary-600 text-primary-700 dark:text-white"
																			: "text-secondary-700 dark:text-secondary-200 hover:text-primary-700 dark:hover:text-primary-300 hover:bg-secondary-50 dark:hover:bg-secondary-700"
																	} ${sidebarCollapsed ? "justify-center p-2" : "p-2"}`}
																	title={sidebarCollapsed ? subItem.name : ""}
																>
																	<subItem.icon
																		className={`h-5 w-5 shrink-0 ${sidebarCollapsed ? "mx-auto" : ""}`}
																	/>
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
															<Link
																to={subItem.href}
																className={`group flex gap-x-3 rounded-md text-sm leading-6 font-medium transition-all duration-200 ${
																	isActive(subItem.href)
																		? "bg-primary-50 dark:bg-primary-600 text-primary-700 dark:text-white"
																		: "text-secondary-700 dark:text-secondary-200 hover:text-primary-700 dark:hover:text-primary-300 hover:bg-secondary-50 dark:hover:bg-secondary-700"
																} ${sidebarCollapsed ? "justify-center p-2 relative" : "p-2"} ${
																	subItem.comingSoon
																		? "opacity-50 cursor-not-allowed"
																		: ""
																}`}
																title={sidebarCollapsed ? subItem.name : ""}
																onClick={
																	subItem.comingSoon
																		? (e) => e.preventDefault()
																		: undefined
																}
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
																	<span className="truncate flex items-center gap-2">
																		{subItem.name}
																		{subItem.name === "Hosts" &&
																			stats?.cards?.totalHosts !==
																				undefined && (
																				<span className="ml-2 inline-flex items-center justify-center px-1.5 py-0.5 text-xs rounded bg-secondary-100 text-secondary-700">
																					{stats.cards.totalHosts}
																				</span>
																			)}
																		{subItem.comingSoon && (
																			<span className="text-xs bg-secondary-100 text-secondary-600 px-1.5 py-0.5 rounded">
																				Soon
																			</span>
																		)}
																		{subItem.beta && (
																			<span className="text-xs bg-blue-100 dark:bg-blue-900 text-blue-600 dark:text-blue-200 px-1.5 py-0.5 rounded font-medium">
																				Beta
																			</span>
																		)}
																		{subItem.new && (
																			<span className="text-xs bg-green-100 dark:bg-green-900 text-green-600 dark:text-green-200 px-1.5 py-0.5 rounded font-medium">
																				New
																			</span>
																		)}
																		{subItem.showUpgradeIcon && (
																			<UpgradeNotificationIcon className="h-3 w-3" />
																		)}
																	</span>
																)}
															</Link>
														)}
													</li>
												))}
											</ul>
										</li>
									);
								}
								return null;
							})}
						</ul>

						{/* Settings Section - Bottom of Navigation */}
						{settingsNavigation.length > 0 && (
							<ul className="gap-y-6">
								{settingsNavigation.map((item) => {
									if (item.section) {
										// Settings section (no heading)
										return (
											<li key={item.section}>
												<ul
													className={`space-y-1 ${sidebarCollapsed ? "" : "-mx-2"}`}
												>
													{item.items.map((subItem) => (
														<li key={subItem.name}>
															<Link
																to={subItem.href}
																className={`group flex gap-x-3 rounded-md text-sm leading-6 font-medium transition-all duration-200 ${
																	isActive(subItem.href)
																		? "bg-primary-50 dark:bg-primary-600 text-primary-700 dark:text-white"
																		: "text-secondary-700 dark:text-secondary-200 hover:text-primary-700 dark:hover:text-primary-300 hover:bg-secondary-50 dark:hover:bg-secondary-700"
																} ${sidebarCollapsed ? "justify-center p-2 relative" : "p-2"}`}
																title={sidebarCollapsed ? subItem.name : ""}
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
																	<span className="truncate flex items-center gap-2">
																		{subItem.name}
																		{subItem.showUpgradeIcon && (
																			<UpgradeNotificationIcon className="h-3 w-3" />
																		)}
																	</span>
																)}
															</Link>
														</li>
													))}
												</ul>
											</li>
										);
									}
									return null;
								})}
							</ul>
						)}
					</nav>

					{/* Profile Section - Bottom of Sidebar */}
					<div className="border-t border-secondary-200 dark:border-secondary-600">
						{!sidebarCollapsed ? (
							<div>
								{/* User Info with Sign Out - Username is clickable */}
								<div className="flex items-center justify-between -mx-2 py-2">
									<Link
										to="/settings/profile"
										className={`flex-1 min-w-0 rounded-md p-2 transition-all duration-200 ${
											isActive("/settings/profile")
												? "bg-primary-50 dark:bg-primary-600"
												: "hover:bg-secondary-50 dark:hover:bg-secondary-700"
										}`}
									>
										<div className="flex items-center gap-x-3">
											<UserCircle
												className={`h-5 w-5 shrink-0 ${
													isActive("/settings/profile")
														? "text-primary-700 dark:text-white"
														: "text-secondary-500 dark:text-secondary-400"
												}`}
											/>
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
												{user?.role === "admin" && (
													<span
														className={`text-xs leading-4 ${
															isActive("/settings/profile")
																? "text-primary-600 dark:text-primary-200"
																: "text-secondary-500 dark:text-secondary-400"
														}`}
													>
														Role: Admin
													</span>
												)}
											</div>
										</div>
									</Link>
									<button
										type="button"
										onClick={handleLogout}
										className="ml-2 p-1.5 text-secondary-400 hover:text-secondary-600 hover:bg-secondary-100 rounded transition-colors"
										title="Sign out"
									>
										<LogOut className="h-4 w-4" />
									</button>
								</div>
								{/* Updated info */}
								{stats && (
									<div className="px-2 py-1 border-t border-secondary-200 dark:border-secondary-700">
										<div className="flex items-center gap-x-1 text-xs text-secondary-500 dark:text-white/70">
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
											{versionInfo && (
												<span className="text-xs text-secondary-400 dark:text-white/60 flex-shrink-0">
													v{versionInfo.version}
												</span>
											)}
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
									<UserCircle className="h-5 w-5" />
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
										{versionInfo && (
											<span className="text-xs text-secondary-400 dark:text-white/60 mt-1">
												v{versionInfo.version}
											</span>
										)}
									</div>
								)}
							</div>
						)}
					</div>
				</div>
			</div>

			{/* Main content */}
			<div
				className={`flex flex-col min-h-screen transition-all duration-300 relative z-10 ${
					sidebarCollapsed ? "lg:pl-16" : "lg:pl-56"
				}`}
			>
				{/* Top bar */}
				<div
					className="sticky top-0 z-[90] flex h-16 shrink-0 items-center gap-x-2 sm:gap-x-4 border-b border-secondary-200 dark:border-white/10 bg-white px-3 sm:px-4 sm:px-6 lg:px-8 shadow-sm"
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
						{/* Page title - hidden on dashboard, hosts, repositories, packages, automation, docker, settings, and host details to give more space to search */}
						{![
							"/",
							"/hosts",
							"/repositories",
							"/packages",
							"/automation",
							"/docker",
						].includes(location.pathname) &&
							!location.pathname.startsWith("/hosts/") &&
							!location.pathname.startsWith("/docker/") &&
							!location.pathname.startsWith("/packages/") &&
							!location.pathname.startsWith("/settings/") && (
								<div className="relative flex items-center flex-shrink-0">
									<h2 className="text-base sm:text-lg font-semibold text-secondary-900 dark:text-secondary-100 whitespace-nowrap">
										{getPageTitle()}
									</h2>
								</div>
							)}

						{/* Global Search Bar */}
						<div
							className={`flex items-center min-w-0 ${["/", "/hosts", "/repositories", "/packages", "/automation", "/docker"].includes(location.pathname) || location.pathname.startsWith("/hosts/") || location.pathname.startsWith("/docker/") || location.pathname.startsWith("/packages/") || location.pathname.startsWith("/settings/") ? "flex-1 max-w-none" : "flex-1 md:flex-none md:max-w-sm"}`}
						>
							<GlobalSearch />
						</div>

						<div className="flex items-center gap-x-2 sm:gap-x-4 lg:gap-x-6 justify-end flex-shrink-0">
							{/* Mobile External Links Menu */}
							<div className="relative md:hidden">
								<button
									type="button"
									onClick={() => setMobileLinksOpen(!mobileLinksOpen)}
									className="flex items-center justify-center w-10 h-10 bg-gray-50 dark:bg-transparent text-secondary-600 dark:text-secondary-300 hover:bg-gray-100 dark:hover:bg-white/10 rounded-lg transition-colors shadow-sm min-w-[44px] min-h-[44px]"
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
												{/* GitHub */}
												<a
													href="https://github.com/PatchMon/PatchMon"
													target="_blank"
													rel="noopener noreferrer"
													className="flex items-center gap-3 px-3 py-3 bg-gray-50 dark:bg-gray-800 text-secondary-600 dark:text-secondary-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg transition-colors min-h-[44px]"
													onClick={() => setMobileLinksOpen(false)}
												>
													<Github className="h-5 w-5 flex-shrink-0" />
													<span className="text-sm font-medium flex-1">
														GitHub
													</span>
													{githubStars !== null && (
														<div className="flex items-center gap-1">
															<Star className="h-4 w-4 fill-current text-yellow-500" />
															<span className="text-sm">{githubStars}</span>
														</div>
													)}
												</a>
												{/* Buy Me a Coffee */}
												<a
													href="https://buymeacoffee.com/iby___"
													target="_blank"
													rel="noopener noreferrer"
													className="flex items-center gap-3 px-3 py-3 bg-gray-50 dark:bg-gray-800 text-secondary-600 dark:text-secondary-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg transition-colors min-h-[44px]"
													onClick={() => setMobileLinksOpen(false)}
												>
													<svg
														className="h-5 w-5 text-yellow-500 flex-shrink-0"
														viewBox="0 0 900 1300"
														fill="currentColor"
													>
														<title>Buy Me a Coffee</title>
														<path d="M879.567 341.849L872.53 306.352C866.215 274.503 851.882 244.409 819.19 232.898C808.711 229.215 796.821 227.633 788.786 220.01C780.751 212.388 778.376 200.55 776.518 189.572C773.076 169.423 769.842 149.257 766.314 129.143C763.269 111.85 760.86 92.4243 752.928 76.56C742.604 55.2584 721.182 42.8009 699.88 34.559C688.965 30.4844 677.826 27.0375 666.517 24.2352C613.297 10.1947 557.342 5.03277 502.591 2.09047C436.875 -1.53577 370.983 -0.443234 305.422 5.35968C256.625 9.79894 205.229 15.1674 158.858 32.0469C141.91 38.224 124.445 45.6399 111.558 58.7341C95.7448 74.8221 90.5829 99.7026 102.128 119.765C110.336 134.012 124.239 144.078 138.985 150.737C158.192 159.317 178.251 165.846 198.829 170.215C256.126 182.879 315.471 187.851 374.007 189.968C438.887 192.586 503.87 190.464 568.44 183.618C584.408 181.863 600.347 179.758 616.257 177.304C634.995 174.43 647.022 149.928 641.499 132.859C634.891 112.453 617.134 104.538 597.055 107.618C594.095 108.082 591.153 108.512 588.193 108.942L586.06 109.252C579.257 110.113 572.455 110.915 565.653 111.661C551.601 113.175 537.515 114.414 523.394 115.378C491.768 117.58 460.057 118.595 428.363 118.647C397.219 118.647 366.058 117.769 334.983 115.722C320.805 114.793 306.661 113.611 292.552 112.177C286.134 111.506 279.733 110.801 273.333 110.009L267.241 109.235L265.917 109.046L259.602 108.134C246.697 106.189 233.792 103.953 221.025 101.251C219.737 100.965 218.584 100.249 217.758 99.2193C216.932 98.1901 216.482 96.9099 216.482 95.5903C216.482 94.2706 216.932 92.9904 217.758 91.9612C218.584 90.9319 219.737 90.2152 221.025 89.9293H221.266C232.33 87.5721 243.479 85.5589 254.663 83.8038C258.392 83.2188 262.131 82.6453 265.882 82.0832H265.985C272.988 81.6186 280.026 80.3625 286.994 79.5366C347.624 73.2301 408.614 71.0801 469.538 73.1014C499.115 73.9618 528.676 75.6996 558.116 78.6935C564.448 79.3474 570.746 80.0357 577.043 80.8099C579.452 81.1025 581.878 81.4465 584.305 81.7391L589.191 82.4445C603.438 84.5667 617.61 87.1419 631.708 90.1703C652.597 94.7128 679.422 96.1925 688.713 119.077C691.673 126.338 693.015 134.408 694.649 142.03L696.732 151.752C696.786 151.926 696.826 152.105 696.852 152.285C701.773 175.227 706.7 198.169 711.632 221.111C711.994 222.806 712.002 224.557 711.657 226.255C711.312 227.954 710.621 229.562 709.626 230.982C708.632 232.401 707.355 233.6 705.877 234.504C704.398 235.408 702.75 235.997 701.033 236.236H700.895L697.884 236.649L694.908 237.044C685.478 238.272 676.038 239.419 666.586 240.486C647.968 242.608 629.322 244.443 610.648 245.992C573.539 249.077 536.356 251.102 499.098 252.066C480.114 252.57 461.135 252.806 442.162 252.771C366.643 252.712 291.189 248.322 216.173 239.625C208.051 238.662 199.93 237.629 191.808 236.58C198.106 237.389 187.231 235.96 185.029 235.651C179.867 234.928 174.705 234.177 169.543 233.397C152.216 230.798 134.993 227.598 117.7 224.793C96.7944 221.352 76.8005 223.073 57.8906 233.397C42.3685 241.891 29.8055 254.916 21.8776 270.735C13.7217 287.597 11.2956 305.956 7.64786 324.075C4.00009 342.193 -1.67805 361.688 0.472751 380.288C5.10128 420.431 33.165 453.054 73.5313 460.35C111.506 467.232 149.687 472.807 187.971 477.556C338.361 495.975 490.294 498.178 641.155 484.129C653.44 482.982 665.708 481.732 677.959 480.378C681.786 479.958 685.658 480.398 689.292 481.668C692.926 482.938 696.23 485.005 698.962 487.717C701.694 490.429 703.784 493.718 705.08 497.342C706.377 500.967 706.846 504.836 706.453 508.665L702.633 545.797C694.936 620.828 687.239 695.854 679.542 770.874C671.513 849.657 663.431 928.434 655.298 1007.2C653.004 1029.39 650.71 1051.57 648.416 1073.74C646.213 1095.58 645.904 1118.1 641.757 1139.68C635.218 1173.61 612.248 1194.45 578.73 1202.07C548.022 1209.06 516.652 1212.73 485.161 1213.01C450.249 1213.2 415.355 1211.65 380.443 1211.84C343.173 1212.05 297.525 1208.61 268.756 1180.87C243.479 1156.51 239.986 1118.36 236.545 1085.37C231.957 1041.7 227.409 998.039 222.9 954.381L197.607 711.615L181.244 554.538C180.968 551.94 180.693 549.376 180.435 546.76C178.473 528.023 165.207 509.681 144.301 510.627C126.407 511.418 106.069 526.629 108.168 546.76L120.298 663.214L145.385 904.104C152.532 972.528 159.661 1040.96 166.773 1109.41C168.15 1122.52 169.44 1135.67 170.885 1148.78C178.749 1220.43 233.465 1259.04 301.224 1269.91C340.799 1276.28 381.337 1277.59 421.497 1278.24C472.979 1279.07 524.977 1281.05 575.615 1271.72C650.653 1257.95 706.952 1207.85 714.987 1130.13C717.282 1107.69 719.576 1085.25 721.87 1062.8C729.498 988.559 737.115 914.313 744.72 840.061L769.601 597.451L781.009 486.263C781.577 480.749 783.905 475.565 787.649 471.478C791.392 467.391 796.352 464.617 801.794 463.567C823.25 459.386 843.761 452.245 859.023 435.916C883.318 409.918 888.153 376.021 879.567 341.849ZM72.4301 365.835C72.757 365.68 72.1548 368.484 71.8967 369.792C71.8451 367.813 71.9483 366.058 72.4301 365.835ZM74.5121 381.94C74.6842 381.819 75.2003 382.508 75.7337 383.334C74.925 382.576 74.4089 382.009 74.4949 381.94H74.5121ZM76.5597 384.641C77.2996 385.897 77.6953 386.689 76.5597 384.641V384.641ZM80.672 387.979H80.7752C80.7752 388.1 80.9645 388.22 81.0333 388.341C80.9192 388.208 80.7925 388.087 80.6548 387.979H80.672ZM800.796 382.989C793.088 390.319 781.473 393.726 769.996 395.43C641.292 414.529 510.713 424.199 380.597 419.932C287.476 416.749 195.336 406.407 103.144 393.382C94.1102 392.109 84.3197 390.457 78.1082 383.798C66.4078 371.237 72.1548 345.944 75.2003 330.768C77.9878 316.865 83.3218 298.334 99.8572 296.355C125.667 293.327 155.64 304.218 181.175 308.09C211.917 312.781 242.774 316.538 273.745 319.36C405.925 331.405 540.325 329.529 671.92 311.91C695.906 308.686 719.805 304.941 743.619 300.674C764.835 296.871 788.356 289.731 801.175 311.703C809.967 326.673 811.137 346.701 809.778 363.615C809.359 370.984 806.139 377.915 800.779 382.989H800.796Z" />
													</svg>
													<span className="text-sm font-medium flex-1">
														Buy Me a Coffee
													</span>
												</a>
												{/* Roadmap */}
												<a
													href="https://github.com/orgs/PatchMon/projects/2/views/1"
													target="_blank"
													rel="noopener noreferrer"
													className="flex items-center gap-3 px-3 py-3 bg-gray-50 dark:bg-gray-800 text-secondary-600 dark:text-secondary-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg transition-colors min-h-[44px]"
													onClick={() => setMobileLinksOpen(false)}
												>
													<Route className="h-5 w-5 flex-shrink-0" />
													<span className="text-sm font-medium flex-1">
														Roadmap
													</span>
												</a>
												{/* Docs */}
												<a
													href="https://docs.patchmon.net"
													target="_blank"
													rel="noopener noreferrer"
													className="flex items-center gap-3 px-3 py-3 bg-gray-50 dark:bg-gray-800 text-secondary-600 dark:text-secondary-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg transition-colors min-h-[44px]"
													onClick={() => setMobileLinksOpen(false)}
												>
													<BookOpen className="h-5 w-5 flex-shrink-0" />
													<span className="text-sm font-medium flex-1">
														Documentation
													</span>
												</a>
												{/* Discord */}
												<a
													href="https://patchmon.net/discord"
													target="_blank"
													rel="noopener noreferrer"
													className="flex items-center gap-3 px-3 py-3 bg-gray-50 dark:bg-gray-800 text-secondary-600 dark:text-secondary-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg transition-colors min-h-[44px]"
													onClick={() => setMobileLinksOpen(false)}
												>
													<DiscordIcon className="h-5 w-5 flex-shrink-0" />
													<span className="text-sm font-medium flex-1">
														Discord
													</span>
												</a>
												{/* Email */}
												<a
													href="mailto:support@patchmon.net"
													className="flex items-center gap-3 px-3 py-3 bg-gray-50 dark:bg-gray-800 text-secondary-600 dark:text-secondary-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg transition-colors min-h-[44px]"
													onClick={() => setMobileLinksOpen(false)}
												>
													<Mail className="h-5 w-5 flex-shrink-0" />
													<span className="text-sm font-medium flex-1">
														Email Support
													</span>
												</a>
												{/* YouTube */}
												<a
													href="https://youtube.com/@patchmonTV"
													target="_blank"
													rel="noopener noreferrer"
													className="flex items-center gap-3 px-3 py-3 bg-gray-50 dark:bg-gray-800 text-secondary-600 dark:text-secondary-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg transition-colors min-h-[44px]"
													onClick={() => setMobileLinksOpen(false)}
												>
													<FaYoutube className="h-5 w-5 flex-shrink-0" />
													<span className="text-sm font-medium flex-1">
														YouTube
													</span>
												</a>
												{/* Reddit */}
												<a
													href="https://www.reddit.com/r/patchmon"
													target="_blank"
													rel="noopener noreferrer"
													className="flex items-center gap-3 px-3 py-3 bg-gray-50 dark:bg-gray-800 text-secondary-600 dark:text-secondary-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg transition-colors min-h-[44px]"
													onClick={() => setMobileLinksOpen(false)}
												>
													<FaReddit className="h-5 w-5 flex-shrink-0" />
													<span className="text-sm font-medium flex-1">
														Reddit
													</span>
												</a>
												{/* Web */}
												<a
													href="https://patchmon.net"
													target="_blank"
													rel="noopener noreferrer"
													className="flex items-center gap-3 px-3 py-3 bg-gray-50 dark:bg-gray-800 text-secondary-600 dark:text-secondary-300 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg transition-colors min-h-[44px]"
													onClick={() => setMobileLinksOpen(false)}
												>
													<Globe className="h-5 w-5 flex-shrink-0" />
													<span className="text-sm font-medium flex-1">
														Website
													</span>
												</a>
											</div>
										</div>
									</>
								)}
							</div>

							{/* Desktop External Links */}
							<div className="hidden md:flex items-center gap-1">
								{/* 1) GitHub */}
								<a
									href="https://github.com/PatchMon/PatchMon"
									target="_blank"
									rel="noopener noreferrer"
									className="flex items-center justify-center gap-1.5 px-3 py-2 bg-gray-50 dark:bg-transparent text-secondary-600 dark:text-secondary-300 hover:bg-gray-100 dark:hover:bg-white/10 rounded-lg transition-colors shadow-sm group relative"
									style={{
										backgroundColor: "var(--button-bg, rgb(249, 250, 251))",
										backdropFilter: "var(--button-blur, none)",
										WebkitBackdropFilter: "var(--button-blur, none)",
									}}
									title="GitHub"
									aria-label="GitHub"
								>
									<Github className="h-5 w-5 flex-shrink-0" />
									{githubStars !== null && (
										<div className="flex items-center gap-1">
											<Star className="h-4 w-4 fill-current text-yellow-500" />
											<span className="text-sm font-medium">{githubStars}</span>
										</div>
									)}
								</a>
								{/* 2) Buy Me a Coffee */}
								<a
									href="https://buymeacoffee.com/iby___"
									target="_blank"
									rel="noopener noreferrer"
									className="flex items-center justify-center w-10 h-10 bg-gray-50 dark:bg-transparent text-secondary-600 dark:text-secondary-300 hover:bg-gray-100 dark:hover:bg-white/10 rounded-lg transition-colors shadow-sm"
									style={{
										backgroundColor: "var(--button-bg, rgb(249, 250, 251))",
										backdropFilter: "var(--button-blur, none)",
										WebkitBackdropFilter: "var(--button-blur, none)",
									}}
									title="Buy Me a Coffee"
									aria-label="Buy Me a Coffee"
								>
									<svg
										className="h-5 w-5 text-yellow-500"
										viewBox="0 0 900 1300"
										fill="currentColor"
									>
										<title>Buy Me a Coffee</title>
										<path d="M879.567 341.849L872.53 306.352C866.215 274.503 851.882 244.409 819.19 232.898C808.711 229.215 796.821 227.633 788.786 220.01C780.751 212.388 778.376 200.55 776.518 189.572C773.076 169.423 769.842 149.257 766.314 129.143C763.269 111.85 760.86 92.4243 752.928 76.56C742.604 55.2584 721.182 42.8009 699.88 34.559C688.965 30.4844 677.826 27.0375 666.517 24.2352C613.297 10.1947 557.342 5.03277 502.591 2.09047C436.875 -1.53577 370.983 -0.443234 305.422 5.35968C256.625 9.79894 205.229 15.1674 158.858 32.0469C141.91 38.224 124.445 45.6399 111.558 58.7341C95.7448 74.8221 90.5829 99.7026 102.128 119.765C110.336 134.012 124.239 144.078 138.985 150.737C158.192 159.317 178.251 165.846 198.829 170.215C256.126 182.879 315.471 187.851 374.007 189.968C438.887 192.586 503.87 190.464 568.44 183.618C584.408 181.863 600.347 179.758 616.257 177.304C634.995 174.43 647.022 149.928 641.499 132.859C634.891 112.453 617.134 104.538 597.055 107.618C594.095 108.082 591.153 108.512 588.193 108.942L586.06 109.252C579.257 110.113 572.455 110.915 565.653 111.661C551.601 113.175 537.515 114.414 523.394 115.378C491.768 117.58 460.057 118.595 428.363 118.647C397.219 118.647 366.058 117.769 334.983 115.722C320.805 114.793 306.661 113.611 292.552 112.177C286.134 111.506 279.733 110.801 273.333 110.009L267.241 109.235L265.917 109.046L259.602 108.134C246.697 106.189 233.792 103.953 221.025 101.251C219.737 100.965 218.584 100.249 217.758 99.2193C216.932 98.1901 216.482 96.9099 216.482 95.5903C216.482 94.2706 216.932 92.9904 217.758 91.9612C218.584 90.9319 219.737 90.2152 221.025 89.9293H221.266C232.33 87.5721 243.479 85.5589 254.663 83.8038C258.392 83.2188 262.131 82.6453 265.882 82.0832H265.985C272.988 81.6186 280.026 80.3625 286.994 79.5366C347.624 73.2301 408.614 71.0801 469.538 73.1014C499.115 73.9618 528.676 75.6996 558.116 78.6935C564.448 79.3474 570.746 80.0357 577.043 80.8099C579.452 81.1025 581.878 81.4465 584.305 81.7391L589.191 82.4445C603.438 84.5667 617.61 87.1419 631.708 90.1703C652.597 94.7128 679.422 96.1925 688.713 119.077C691.673 126.338 693.015 134.408 694.649 142.03L696.732 151.752C696.786 151.926 696.826 152.105 696.852 152.285C701.773 175.227 706.7 198.169 711.632 221.111C711.994 222.806 712.002 224.557 711.657 226.255C711.312 227.954 710.621 229.562 709.626 230.982C708.632 232.401 707.355 233.6 705.877 234.504C704.398 235.408 702.75 235.997 701.033 236.236H700.895L697.884 236.649L694.908 237.044C685.478 238.272 676.038 239.419 666.586 240.486C647.968 242.608 629.322 244.443 610.648 245.992C573.539 249.077 536.356 251.102 499.098 252.066C480.114 252.57 461.135 252.806 442.162 252.771C366.643 252.712 291.189 248.322 216.173 239.625C208.051 238.662 199.93 237.629 191.808 236.58C198.106 237.389 187.231 235.96 185.029 235.651C179.867 234.928 174.705 234.177 169.543 233.397C152.216 230.798 134.993 227.598 117.7 224.793C96.7944 221.352 76.8005 223.073 57.8906 233.397C42.3685 241.891 29.8055 254.916 21.8776 270.735C13.7217 287.597 11.2956 305.956 7.64786 324.075C4.00009 342.193 -1.67805 361.688 0.472751 380.288C5.10128 420.431 33.165 453.054 73.5313 460.35C111.506 467.232 149.687 472.807 187.971 477.556C338.361 495.975 490.294 498.178 641.155 484.129C653.44 482.982 665.708 481.732 677.959 480.378C681.786 479.958 685.658 480.398 689.292 481.668C692.926 482.938 696.23 485.005 698.962 487.717C701.694 490.429 703.784 493.718 705.08 497.342C706.377 500.967 706.846 504.836 706.453 508.665L702.633 545.797C694.936 620.828 687.239 695.854 679.542 770.874C671.513 849.657 663.431 928.434 655.298 1007.2C653.004 1029.39 650.71 1051.57 648.416 1073.74C646.213 1095.58 645.904 1118.1 641.757 1139.68C635.218 1173.61 612.248 1194.45 578.73 1202.07C548.022 1209.06 516.652 1212.73 485.161 1213.01C450.249 1213.2 415.355 1211.65 380.443 1211.84C343.173 1212.05 297.525 1208.61 268.756 1180.87C243.479 1156.51 239.986 1118.36 236.545 1085.37C231.957 1041.7 227.409 998.039 222.9 954.381L197.607 711.615L181.244 554.538C180.968 551.94 180.693 549.376 180.435 546.76C178.473 528.023 165.207 509.681 144.301 510.627C126.407 511.418 106.069 526.629 108.168 546.76L120.298 663.214L145.385 904.104C152.532 972.528 159.661 1040.96 166.773 1109.41C168.15 1122.52 169.44 1135.67 170.885 1148.78C178.749 1220.43 233.465 1259.04 301.224 1269.91C340.799 1276.28 381.337 1277.59 421.497 1278.24C472.979 1279.07 524.977 1281.05 575.615 1271.72C650.653 1257.95 706.952 1207.85 714.987 1130.13C717.282 1107.69 719.576 1085.25 721.87 1062.8C729.498 988.559 737.115 914.313 744.72 840.061L769.601 597.451L781.009 486.263C781.577 480.749 783.905 475.565 787.649 471.478C791.392 467.391 796.352 464.617 801.794 463.567C823.25 459.386 843.761 452.245 859.023 435.916C883.318 409.918 888.153 376.021 879.567 341.849ZM72.4301 365.835C72.757 365.68 72.1548 368.484 71.8967 369.792C71.8451 367.813 71.9483 366.058 72.4301 365.835ZM74.5121 381.94C74.6842 381.819 75.2003 382.508 75.7337 383.334C74.925 382.576 74.4089 382.009 74.4949 381.94H74.5121ZM76.5597 384.641C77.2996 385.897 77.6953 386.689 76.5597 384.641V384.641ZM80.672 387.979H80.7752C80.7752 388.1 80.9645 388.22 81.0333 388.341C80.9192 388.208 80.7925 388.087 80.6548 387.979H80.672ZM800.796 382.989C793.088 390.319 781.473 393.726 769.996 395.43C641.292 414.529 510.713 424.199 380.597 419.932C287.476 416.749 195.336 406.407 103.144 393.382C94.1102 392.109 84.3197 390.457 78.1082 383.798C66.4078 371.237 72.1548 345.944 75.2003 330.768C77.9878 316.865 83.3218 298.334 99.8572 296.355C125.667 293.327 155.64 304.218 181.175 308.09C211.917 312.781 242.774 316.538 273.745 319.36C405.925 331.405 540.325 329.529 671.92 311.91C695.906 308.686 719.805 304.941 743.619 300.674C764.835 296.871 788.356 289.731 801.175 311.703C809.967 326.673 811.137 346.701 809.778 363.615C809.359 370.984 806.139 377.915 800.779 382.989H800.796Z" />
									</svg>
								</a>
								{/* 3) Roadmap */}
								<a
									href="https://github.com/orgs/PatchMon/projects/2/views/1"
									target="_blank"
									rel="noopener noreferrer"
									className="flex items-center justify-center w-10 h-10 bg-gray-50 dark:bg-transparent text-secondary-600 dark:text-secondary-300 hover:bg-gray-100 dark:hover:bg-white/10 rounded-lg transition-colors shadow-sm"
									style={{
										backgroundColor: "var(--button-bg, rgb(249, 250, 251))",
										backdropFilter: "var(--button-blur, none)",
										WebkitBackdropFilter: "var(--button-blur, none)",
									}}
									title="Roadmap"
								>
									<Route className="h-5 w-5" />
								</a>
								{/* 4) Docs */}
								<a
									href="https://docs.patchmon.net"
									target="_blank"
									rel="noopener noreferrer"
									className="flex items-center justify-center w-10 h-10 bg-gray-50 dark:bg-transparent text-secondary-600 dark:text-secondary-300 hover:bg-gray-100 dark:hover:bg-white/10 rounded-lg transition-colors shadow-sm"
									style={{
										backgroundColor: "var(--button-bg, rgb(249, 250, 251))",
										backdropFilter: "var(--button-blur, none)",
										WebkitBackdropFilter: "var(--button-blur, none)",
									}}
									title="Documentation"
								>
									<BookOpen className="h-5 w-5" />
								</a>
								{/* 5) Discord */}
								<a
									href="https://patchmon.net/discord"
									target="_blank"
									rel="noopener noreferrer"
									className="flex items-center justify-center w-10 h-10 bg-gray-50 dark:bg-transparent text-secondary-600 dark:text-secondary-300 hover:bg-gray-100 dark:hover:bg-white/10 rounded-lg transition-colors shadow-sm"
									style={{
										backgroundColor: "var(--button-bg, rgb(249, 250, 251))",
										backdropFilter: "var(--button-blur, none)",
										WebkitBackdropFilter: "var(--button-blur, none)",
									}}
									title="Discord"
								>
									<DiscordIcon className="h-5 w-5" />
								</a>
								{/* 6) Email */}
								<a
									href="mailto:support@patchmon.net"
									className="flex items-center justify-center w-10 h-10 bg-gray-50 dark:bg-transparent text-secondary-600 dark:text-secondary-300 hover:bg-gray-100 dark:hover:bg-white/10 rounded-lg transition-colors shadow-sm"
									style={{
										backgroundColor: "var(--button-bg, rgb(249, 250, 251))",
										backdropFilter: "var(--button-blur, none)",
										WebkitBackdropFilter: "var(--button-blur, none)",
									}}
									title="Email support@patchmon.net"
								>
									<Mail className="h-5 w-5" />
								</a>
								{/* 7) YouTube */}
								<a
									href="https://youtube.com/@patchmonTV"
									target="_blank"
									rel="noopener noreferrer"
									className="flex items-center justify-center w-10 h-10 bg-gray-50 dark:bg-transparent text-secondary-600 dark:text-secondary-300 hover:bg-gray-100 dark:hover:bg-white/10 rounded-lg transition-colors shadow-sm"
									style={{
										backgroundColor: "var(--button-bg, rgb(249, 250, 251))",
										backdropFilter: "var(--button-blur, none)",
										WebkitBackdropFilter: "var(--button-blur, none)",
									}}
									title="YouTube Channel"
								>
									<FaYoutube className="h-5 w-5" />
								</a>
								{/* 8) Reddit */}
								<a
									href="https://www.reddit.com/r/patchmon"
									target="_blank"
									rel="noopener noreferrer"
									className="flex items-center justify-center w-10 h-10 bg-gray-50 dark:bg-transparent text-secondary-600 dark:text-secondary-300 hover:bg-gray-100 dark:hover:bg-white/10 rounded-lg transition-colors shadow-sm"
									style={{
										backgroundColor: "var(--button-bg, rgb(249, 250, 251))",
										backdropFilter: "var(--button-blur, none)",
										WebkitBackdropFilter: "var(--button-blur, none)",
									}}
									title="Reddit Community"
								>
									<FaReddit className="h-5 w-5" />
								</a>
								{/* 9) Web */}
								<a
									href="https://patchmon.net"
									target="_blank"
									rel="noopener noreferrer"
									className="flex items-center justify-center w-10 h-10 bg-gray-50 dark:bg-transparent text-secondary-600 dark:text-secondary-300 hover:bg-gray-100 dark:hover:bg-white/10 rounded-lg transition-colors shadow-sm"
									style={{
										backgroundColor: "var(--button-bg, rgb(249, 250, 251))",
										backdropFilter: "var(--button-blur, none)",
										WebkitBackdropFilter: "var(--button-blur, none)",
									}}
									title="Visit patchmon.net"
								>
									<Globe className="h-5 w-5" />
								</a>
							</div>
						</div>
					</div>
				</div>

				<main className="flex-1 py-6 bg-secondary-50 dark:bg-transparent">
					<div className="px-4 sm:px-6 lg:px-8">{children}</div>
				</main>
			</div>
		</div>
	);
};

export default Layout;
