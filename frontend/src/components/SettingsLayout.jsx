import {
	BarChart3,
	Bot,
	ChevronDown,
	ChevronLeft,
	ChevronRight,
	Code,
	Folder,
	Image,
	Key,
	KeyRound,
	RefreshCw,
	Settings,
	Shield,
	UserCircle,
	Users,
	Variable,
	Wrench,
} from "lucide-react";
import { useState } from "react";
import { Link, Outlet, useLocation, useNavigate } from "react-router-dom";
import { getRequiredTier } from "../constants/tiers";
import { useAuth } from "../contexts/AuthContext";
import { useSettings } from "../contexts/SettingsContext";
import DiscordIcon from "./DiscordIcon";
import TierBadge from "./TierBadge";

const SettingsLayout = ({ children }) => {
	const content = children ?? <Outlet />;
	const location = useLocation();
	const { canManageSettings, canViewUsers, canManageUsers, hasModule } =
		useAuth();
	const { settings: publicSettings } = useSettings();
	const [sidebarCollapsed, setSidebarCollapsed] = useState(false);

	// Build secondary navigation based on permissions
	const buildSecondaryNavigation = () => {
		const nav = [];

		// Users section
		if (canViewUsers() || canManageUsers()) {
			const userItems = [
				{
					name: "Users",
					href: "/settings/users",
					icon: Users,
				},
			];
			// Custom RBAC roles management is a Plus-tier feature (rbac_custom).
			// Built-in admin/viewer roles stay available in every tier, but the
			// dedicated "Roles" editor manages custom role CRUD. Locked items
			// stay visible with a TierBadge for discovery; the route renders
			// an upgrade screen via <ModuleGate>.
			{
				const locked = !hasModule("rbac_custom");
				userItems.push({
					name: "Roles",
					href: "/settings/roles",
					icon: Shield,
					lockedModule: locked ? "rbac_custom" : null,
					lockedTier: locked ? getRequiredTier("rbac_custom") : null,
				});
			}
			userItems.push({
				name: "My Profile",
				href: "/settings/profile",
				icon: UserCircle,
			});
			if (canManageSettings()) {
				userItems.push(
					{
						name: "Discord Auth",
						href: "/settings/discord-auth",
						icon: DiscordIcon,
					},
					{
						name: "OIDC / SSO",
						href: "/settings/oidc-auth",
						icon: KeyRound,
					},
				);
			}
			nav.push({
				section: "User Management",
				items: userItems,
			});
		}

		// Host Groups
		if (canManageSettings()) {
			nav.push({
				section: "Hosts Management",
				items: [
					{
						name: "Host Groups",
						href: "/settings/host-groups",
						icon: Folder,
					},
					{
						name: "Agent Updates",
						href: "/settings/agent-config",
						icon: RefreshCw,
					},
					{
						name: "Agent Version",
						href: "/settings/agent-version",
						icon: Settings,
					},
				],
			});
		}

		// Alert Management moved to Reporting page tabs

		// Patch Management moved to /patching?tab=policies

		// Server Config
		if (canManageSettings()) {
			// Integrations section. AI Terminal is a Max-tier feature (module key "ai").
			const integrationsItems = [
				{
					name: "API integrations",
					href: "/settings/integrations",
					icon: Wrench,
				},
			];
			{
				const locked = !hasModule("ai");
				integrationsItems.push({
					name: "AI Terminal",
					href: "/settings/ai-terminal",
					icon: Bot,
					lockedModule: locked ? "ai" : null,
					lockedTier: locked ? getRequiredTier("ai") : null,
				});
			}
			nav.push({
				section: "Integrations",
				items: integrationsItems,
			});

			const isAdminMode = publicSettings?.admin_mode;
			const serverItems = [];
			// Server URL and Server Version are hidden in managed/multi-context deployments.
			if (!isAdminMode) {
				serverItems.push({
					name: "Server URL",
					href: "/settings/server-url",
					icon: Wrench,
				});
			}
			serverItems.push({
				name: "Environment",
				href: "/settings/environment",
				icon: Variable,
			});
			// Custom branding (logo/favicon upload) is a Plus-tier feature
			// (module key "custom_branding").
			{
				const locked = !hasModule("custom_branding");
				serverItems.push({
					name: "Branding",
					href: "/settings/branding",
					icon: Image,
					lockedModule: locked ? "custom_branding" : null,
					lockedTier: locked ? getRequiredTier("custom_branding") : null,
				});
			}
			if (!isAdminMode) {
				serverItems.push({
					name: "Server Version",
					href: "/settings/server-version",
					icon: Code,
				});
			}
			// Metrics is hidden in managed/multi-context deployments.
			if (!isAdminMode) {
				serverItems.push({
					name: "Metrics",
					href: "/settings/metrics",
					icon: BarChart3,
				});
				serverItems.push({
					name: "API Tokens",
					href: "/settings/api-tokens",
					icon: Key,
				});
			}
			nav.push({
				section: "Server",
				items: serverItems,
			});
		}

		return nav;
	};

	const secondaryNavigation = buildSecondaryNavigation();
	const navigate = useNavigate();

	const isActive = (path) => location.pathname === path;

	// Flatten all navigation items for dropdown
	const getAllNavItems = () => {
		const items = [];
		secondaryNavigation.forEach((section) => {
			section.items.forEach((item) => {
				if (!item.comingSoon) {
					items.push({
						...item,
						section: section.section,
					});
				}
			});
		});
		return items;
	};

	const allNavItems = getAllNavItems();

	const handleDropdownChange = (e) => {
		const selectedHref = e.target.value;
		if (selectedHref) {
			navigate(selectedHref);
		}
	};

	return (
		<div className="bg-transparent">
			{/* Within-page secondary navigation and content */}
			<div className="px-2 sm:px-4 lg:px-6">
				{/* Mobile Dropdown */}
				<div className="md:hidden mb-4">
					<label
						htmlFor="settings-select"
						className="block text-sm font-medium text-secondary-700 dark:text-white mb-2"
					>
						Settings Section
					</label>
					<div className="relative">
						<select
							id="settings-select"
							value={location.pathname}
							onChange={handleDropdownChange}
							className="block w-full pl-3 pr-10 py-2 text-base border border-secondary-300 dark:border-secondary-600 rounded-md bg-white dark:bg-secondary-800 text-secondary-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-primary-500 appearance-none"
						>
							{allNavItems.map((item) => (
								<option key={item.href} value={item.href}>
									{item.section} - {item.name}
									{item.lockedTier ? ` (${item.lockedTier.toUpperCase()})` : ""}
								</option>
							))}
						</select>
						<ChevronDown className="absolute right-3 top-1/2 transform -translate-y-1/2 h-5 w-5 text-secondary-400 pointer-events-none" />
					</div>
				</div>

				<div className="flex gap-4">
					{/* Left secondary nav (within page) - Hidden on mobile */}
					<aside
						className={`hidden md:block ${sidebarCollapsed ? "w-14" : "w-56"} transition-all duration-300 flex-shrink-0`}
					>
						<div className="bg-white dark:bg-secondary-800 border border-secondary-200 dark:border-secondary-600 rounded-lg">
							{/* Collapse button */}
							<div className="flex justify-end p-2 border-b border-secondary-200 dark:border-secondary-600">
								<button
									type="button"
									onClick={() => setSidebarCollapsed(!sidebarCollapsed)}
									className="p-1 text-secondary-400 hover:text-secondary-600 dark:text-white dark:hover:text-secondary-300 rounded transition-colors"
									title={
										sidebarCollapsed ? "Expand sidebar" : "Collapse sidebar"
									}
								>
									{sidebarCollapsed ? (
										<ChevronRight className="h-4 w-4" />
									) : (
										<ChevronLeft className="h-4 w-4" />
									)}
								</button>
							</div>

							<div className={`${sidebarCollapsed ? "p-2" : "p-3"}`}>
								<nav>
									<ul
										className={`${sidebarCollapsed ? "space-y-2" : "space-y-4"}`}
									>
										{secondaryNavigation.map((item) => (
											<li key={item.section}>
												{!sidebarCollapsed && (
													<h4 className="text-xs font-semibold text-secondary-500 dark:text-white uppercase tracking-wider mb-2">
														{item.section}
													</h4>
												)}
												<ul
													className={`${sidebarCollapsed ? "space-y-1" : "space-y-1"}`}
												>
													{item.items.map((subItem) => (
														<li key={subItem.name}>
															<Link
																to={subItem.href}
																className={`group flex items-center rounded-md text-sm leading-5 font-medium transition-colors ${
																	sidebarCollapsed
																		? "justify-center p-2"
																		: "gap-2 p-2"
																} ${
																	isActive(subItem.href)
																		? "bg-primary-50 dark:bg-primary-600 text-primary-700 dark:text-white"
																		: "text-secondary-700 dark:text-secondary-200 hover:text-primary-700 dark:hover:text-primary-300 hover:bg-secondary-50 dark:hover:bg-secondary-700"
																}`}
																title={sidebarCollapsed ? subItem.name : ""}
															>
																<subItem.icon className="h-4 w-4 flex-shrink-0" />
																{!sidebarCollapsed && (
																	<span className="truncate flex items-center gap-2">
																		{subItem.name}
																		{subItem.comingSoon && (
																			<span className="text-xs bg-secondary-100 text-secondary-600 px-1.5 py-0.5 rounded">
																				Soon
																			</span>
																		)}
																		{subItem.lockedTier && (
																			<TierBadge tier={subItem.lockedTier} />
																		)}
																	</span>
																)}
															</Link>

															{!sidebarCollapsed && subItem.subTabs && (
																<ul className="ml-6 mt-1 space-y-1">
																	{subItem.subTabs.map((subTab) => (
																		<li key={subTab.name}>
																			<Link
																				to={subTab.href}
																				className={`block px-3 py-1 text-xs font-medium rounded transition-colors ${
																					isActive(subTab.href)
																						? "bg-primary-100 dark:bg-primary-700 text-primary-700 dark:text-primary-200"
																						: "text-secondary-600 dark:text-white hover:text-primary-700 dark:hover:text-primary-300 hover:bg-secondary-50 dark:hover:bg-secondary-700"
																				}`}
																			>
																				{subTab.name}
																			</Link>
																		</li>
																	))}
																</ul>
															)}
														</li>
													))}
												</ul>
											</li>
										))}
									</ul>
								</nav>
							</div>
						</div>
					</aside>

					{/* Right content */}
					<section className="flex-1 min-w-0">
						<div className="bg-white dark:bg-secondary-800 border border-secondary-200 dark:border-secondary-600 rounded-lg p-4">
							{content}
						</div>
					</section>
				</div>
			</div>
		</div>
	);
};

export default SettingsLayout;
