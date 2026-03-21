import { lazy, Suspense } from "react";
import { Outlet, Route, Routes } from "react-router-dom";
import ErrorBoundary from "./components/ErrorBoundary";
import FirstTimeWizard from "./components/FirstTimeWizard";
import Layout from "./components/Layout";
import LogoProvider from "./components/LogoProvider";
import PageTransition from "./components/PageTransition";
import ProtectedRoute from "./components/ProtectedRoute";
import SettingsLayout from "./components/SettingsLayout";
import SetupCheckError from "./components/SetupCheckError";
import { isAuthPhase } from "./constants/authPhases";
import { AuthProvider, useAuth } from "./contexts/AuthContext";
import { ColorThemeProvider } from "./contexts/ColorThemeContext";
import { SettingsProvider } from "./contexts/SettingsContext";
import { ThemeProvider } from "./contexts/ThemeContext";
import { ToastProvider } from "./contexts/ToastContext";
import { UpdateNotificationProvider } from "./contexts/UpdateNotificationContext";
import Automation from "./pages/Automation";
import Compliance from "./pages/Compliance";
// Eager load main nav pages for instant navigation (no loading flash)
import Dashboard from "./pages/Dashboard";
import Docker from "./pages/Docker";
import Hosts from "./pages/Hosts";
import Login from "./pages/Login";
import Packages from "./pages/Packages";
import Patching from "./pages/Patching";
import Reporting from "./pages/Reporting";
import Repositories from "./pages/Repositories";
import SettingsUsers from "./pages/settings/SettingsUsers";

// Lazy load detail/settings pages (less frequently navigated)
const HostDetail = lazy(() => import("./pages/HostDetail"));
const PackageDetail = lazy(() => import("./pages/PackageDetail"));
const PatchingRunDetail = lazy(() => import("./pages/patching/RunDetail"));
const Profile = lazy(() => import("./pages/Profile"));
const RepositoryDetail = lazy(() => import("./pages/RepositoryDetail"));
const ComplianceHostDetail = lazy(
	() => import("./pages/compliance/HostDetail"),
);
const ComplianceRuleDetail = lazy(
	() => import("./pages/compliance/RuleDetail"),
);
const DockerContainerDetail = lazy(
	() => import("./pages/docker/ContainerDetail"),
);
const DockerImageDetail = lazy(() => import("./pages/docker/ImageDetail"));
const DockerHostDetail = lazy(() => import("./pages/docker/HostDetail"));
const DockerVolumeDetail = lazy(() => import("./pages/docker/VolumeDetail"));
const DockerNetworkDetail = lazy(() => import("./pages/docker/NetworkDetail"));
const SettingsHomeRedirect = lazy(
	() => import("./pages/settings/SettingsHomeRedirect"),
);
const Integrations = lazy(() => import("./pages/settings/Integrations"));
const PatchManagement = lazy(() => import("./pages/settings/PatchManagement"));
const SettingsAgentConfig = lazy(
	() => import("./pages/settings/SettingsAgentConfig"),
);
const SettingsHostGroups = lazy(
	() => import("./pages/settings/SettingsHostGroups"),
);
const SettingsServerConfig = lazy(
	() => import("./pages/settings/SettingsServerConfig"),
);
const SettingsMetrics = lazy(() => import("./pages/settings/SettingsMetrics"));
const EnvironmentSettings = lazy(
	() => import("./pages/settings/EnvironmentSettings"),
);
const AiSettings = lazy(() => import("./pages/settings/AiSettings"));
const DiscordSettings = lazy(() => import("./pages/settings/DiscordSettings"));
const OidcSettings = lazy(() => import("./pages/settings/OidcSettings"));

// Full-screen loading fallback (for initial app load / auth check)
const LoadingFallback = () => (
	<div className="min-h-screen bg-gradient-to-br from-primary-50 to-secondary-50 dark:from-secondary-900 dark:to-secondary-800 flex items-center justify-center">
		<div className="text-center">
			<div className="animate-spin rounded-full h-12 w-12 border-b-2 border-primary-600 mx-auto mb-4"></div>
			<p className="text-secondary-600 dark:text-white">Loading...</p>
		</div>
	</div>
);

// Minimal in-content loading fallback (keeps sidebar visible during page transitions)
const PageLoadingFallback = () => (
	<div className="flex items-center justify-center py-24 min-h-[60vh]">
		<div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600" />
	</div>
);

function AppRoutes() {
	const {
		needsFirstTimeSetup,
		setupCheckError,
		authPhase,
		isAuthenticated,
		firstTimeWizardActive,
	} = useAuth();
	const isAuth = isAuthenticated(); // Call the function to get boolean value

	// Show loading while checking setup or initialising
	if (
		isAuthPhase.initialising(authPhase) ||
		isAuthPhase.checkingSetup(authPhase)
	) {
		return (
			<div className="min-h-screen bg-gradient-to-br from-primary-50 to-secondary-50 dark:from-secondary-900 dark:to-secondary-800 flex items-center justify-center">
				<div className="text-center">
					<div className="animate-spin rounded-full h-12 w-12 border-b-2 border-primary-600 mx-auto mb-4"></div>
					<p className="text-secondary-600 dark:text-white">
						Checking system status...
					</p>
				</div>
			</div>
		);
	}

	// Backend/DB unreachable or rate limited - show error, not first-time setup
	if (setupCheckError) {
		return <SetupCheckError />;
	}

	// Show first-time setup when no admin users exist, or when wizard is in progress (e.g. MFA setup)
	if ((needsFirstTimeSetup && !isAuth) || firstTimeWizardActive) {
		return <FirstTimeWizard />;
	}

	return (
		<Suspense fallback={<LoadingFallback />}>
			<Routes>
				<Route path="/login" element={<Login />} />
				<Route
					element={
						<ProtectedRoute>
							<Layout>
								<PageTransition>
									<Suspense fallback={<PageLoadingFallback />}>
										<Outlet />
									</Suspense>
								</PageTransition>
							</Layout>
						</ProtectedRoute>
					}
				>
					<Route
						path="/"
						element={
							<ProtectedRoute requirePermission="can_view_dashboard">
								<Dashboard />
							</ProtectedRoute>
						}
					/>
					<Route
						path="/hosts"
						element={
							<ProtectedRoute requirePermission="can_view_hosts">
								<Hosts />
							</ProtectedRoute>
						}
					/>
					<Route
						path="/hosts/:hostId"
						element={
							<ProtectedRoute requirePermission="can_view_hosts">
								<HostDetail />
							</ProtectedRoute>
						}
					/>
					<Route
						path="/packages"
						element={
							<ProtectedRoute requirePermission="can_view_packages">
								<Packages />
							</ProtectedRoute>
						}
					/>
					<Route
						path="/reporting"
						element={
							<ProtectedRoute requirePermission="can_view_reports">
								<Reporting />
							</ProtectedRoute>
						}
					/>
					<Route
						path="/repositories"
						element={
							<ProtectedRoute requirePermission="can_view_hosts">
								<Repositories />
							</ProtectedRoute>
						}
					/>
					<Route
						path="/repositories/:repositoryId"
						element={
							<ProtectedRoute requirePermission="can_view_hosts">
								<RepositoryDetail />
							</ProtectedRoute>
						}
					/>
					<Route
						path="/automation"
						element={
							<ProtectedRoute requirePermission="can_view_hosts">
								<Automation />
							</ProtectedRoute>
						}
					/>
					<Route
						path="/patching"
						element={
							<ProtectedRoute requirePermission="can_view_hosts">
								<Patching />
							</ProtectedRoute>
						}
					/>
					<Route
						path="/patching/runs/:id"
						element={
							<ProtectedRoute requirePermission="can_view_hosts">
								<PatchingRunDetail />
							</ProtectedRoute>
						}
					/>
					<Route
						path="/compliance"
						element={
							<ProtectedRoute requirePermission="can_view_hosts">
								<Compliance />
							</ProtectedRoute>
						}
					/>
					<Route
						path="/compliance/hosts/:id"
						element={
							<ProtectedRoute requirePermission="can_view_hosts">
								<ComplianceHostDetail />
							</ProtectedRoute>
						}
					/>
					<Route
						path="/compliance/rules/:id"
						element={
							<ProtectedRoute requirePermission="can_view_hosts">
								<ComplianceRuleDetail />
							</ProtectedRoute>
						}
					/>
					<Route
						path="/docker"
						element={
							<ProtectedRoute requirePermission="can_view_reports">
								<Docker />
							</ProtectedRoute>
						}
					/>
					<Route
						path="/docker/containers/:id"
						element={
							<ProtectedRoute requirePermission="can_view_reports">
								<DockerContainerDetail />
							</ProtectedRoute>
						}
					/>
					<Route
						path="/docker/images/:id"
						element={
							<ProtectedRoute requirePermission="can_view_reports">
								<DockerImageDetail />
							</ProtectedRoute>
						}
					/>
					<Route
						path="/docker/hosts/:id"
						element={
							<ProtectedRoute requirePermission="can_view_reports">
								<DockerHostDetail />
							</ProtectedRoute>
						}
					/>
					<Route
						path="/docker/volumes/:id"
						element={
							<ProtectedRoute requirePermission="can_view_reports">
								<DockerVolumeDetail />
							</ProtectedRoute>
						}
					/>
					<Route
						path="/docker/networks/:id"
						element={
							<ProtectedRoute requirePermission="can_view_reports">
								<DockerNetworkDetail />
							</ProtectedRoute>
						}
					/>
					<Route
						path="/users"
						element={
							<ProtectedRoute requirePermission="can_view_users">
								<SettingsUsers />
							</ProtectedRoute>
						}
					/>
					<Route
						path="/permissions"
						element={
							<ProtectedRoute requirePermission="can_manage_settings">
								<SettingsUsers />
							</ProtectedRoute>
						}
					/>
					{/* Settings routes share SettingsLayout so sidebar stays mounted */}
					<Route
						path="/settings"
						element={
							<ProtectedRoute
								requireAnyPermissions={[
									"can_view_users",
									"can_manage_notifications",
									"can_view_notification_logs",
								]}
							>
								<SettingsLayout>
									<Suspense fallback={<PageLoadingFallback />}>
										<Outlet />
									</Suspense>
								</SettingsLayout>
							</ProtectedRoute>
						}
					>
						<Route
							index
							element={
								<Suspense fallback={<PageLoadingFallback />}>
									<SettingsHomeRedirect />
								</Suspense>
							}
						/>
						<Route
							path="users"
							element={
								<ProtectedRoute requirePermission="can_view_users">
									<SettingsUsers />
								</ProtectedRoute>
							}
						/>
						<Route
							path="roles"
							element={
								<ProtectedRoute requirePermission="can_manage_settings">
									<SettingsUsers />
								</ProtectedRoute>
							}
						/>
						<Route
							path="profile"
							element={
								<ProtectedRoute>
									<Profile />
								</ProtectedRoute>
							}
						/>
						<Route
							path="host-groups"
							element={
								<ProtectedRoute requirePermission="can_manage_settings">
									<SettingsHostGroups />
								</ProtectedRoute>
							}
						/>
						<Route
							path="agent-config"
							element={
								<ProtectedRoute requirePermission="can_manage_settings">
									<SettingsAgentConfig />
								</ProtectedRoute>
							}
						/>
						<Route
							path="agent-config/management"
							element={
								<ProtectedRoute requirePermission="can_manage_settings">
									<SettingsAgentConfig />
								</ProtectedRoute>
							}
						/>
						<Route
							path="server-config"
							element={
								<ProtectedRoute requirePermission="can_manage_settings">
									<SettingsServerConfig />
								</ProtectedRoute>
							}
						/>
						<Route
							path="server-config/version"
							element={
								<ProtectedRoute requirePermission="can_manage_settings">
									<SettingsServerConfig />
								</ProtectedRoute>
							}
						/>
						<Route
							path="integrations"
							element={
								<ProtectedRoute requirePermission="can_manage_settings">
									<Integrations />
								</ProtectedRoute>
							}
						/>
						<Route
							path="patch-management"
							element={
								<ProtectedRoute requirePermission="can_manage_settings">
									<PatchManagement />
								</ProtectedRoute>
							}
						/>
						<Route
							path="server-url"
							element={
								<ProtectedRoute requirePermission="can_manage_settings">
									<SettingsServerConfig />
								</ProtectedRoute>
							}
						/>
						<Route
							path="environment"
							element={
								<ProtectedRoute requirePermission="can_manage_settings">
									<EnvironmentSettings />
								</ProtectedRoute>
							}
						/>
						<Route
							path="server-version"
							element={
								<ProtectedRoute requirePermission="can_manage_settings">
									<SettingsServerConfig />
								</ProtectedRoute>
							}
						/>
						<Route
							path="branding"
							element={
								<ProtectedRoute requirePermission="can_manage_settings">
									<SettingsServerConfig />
								</ProtectedRoute>
							}
						/>
						<Route
							path="agent-version"
							element={
								<ProtectedRoute requirePermission="can_manage_settings">
									<SettingsAgentConfig />
								</ProtectedRoute>
							}
						/>
						<Route
							path="metrics"
							element={
								<ProtectedRoute requirePermission="can_manage_settings">
									<SettingsMetrics />
								</ProtectedRoute>
							}
						/>
						<Route
							path="ai-terminal"
							element={
								<ProtectedRoute requirePermission="can_manage_settings">
									<AiSettings />
								</ProtectedRoute>
							}
						/>
						<Route
							path="discord-auth"
							element={
								<ProtectedRoute requirePermission="can_manage_settings">
									<DiscordSettings />
								</ProtectedRoute>
							}
						/>
						<Route
							path="oidc-auth"
							element={
								<ProtectedRoute requirePermission="can_manage_settings">
									<OidcSettings />
								</ProtectedRoute>
							}
						/>
					</Route>
					<Route
						path="/options"
						element={
							<ProtectedRoute requirePermission="can_manage_hosts">
								<SettingsHostGroups />
							</ProtectedRoute>
						}
					/>
					<Route
						path="/packages/:packageId"
						element={
							<ProtectedRoute requirePermission="can_view_packages">
								<PackageDetail />
							</ProtectedRoute>
						}
					/>
				</Route>
			</Routes>
		</Suspense>
	);
}

function App() {
	return (
		<ErrorBoundary>
			<AuthProvider>
				<ThemeProvider>
					<SettingsProvider>
						<ColorThemeProvider>
							<ToastProvider>
								<UpdateNotificationProvider>
									<LogoProvider>
										<AppRoutes />
									</LogoProvider>
								</UpdateNotificationProvider>
							</ToastProvider>
						</ColorThemeProvider>
					</SettingsProvider>
				</ThemeProvider>
			</AuthProvider>
		</ErrorBoundary>
	);
}

export default App;
