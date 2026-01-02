import { lazy, Suspense } from "react";
import { Route, Routes } from "react-router-dom";
import ErrorBoundary from "./components/ErrorBoundary";
import FirstTimeAdminSetup from "./components/FirstTimeAdminSetup";
import Layout from "./components/Layout";
import LogoProvider from "./components/LogoProvider";
import ProtectedRoute from "./components/ProtectedRoute";
import SettingsLayout from "./components/SettingsLayout";
import { isAuthPhase } from "./constants/authPhases";
import { AuthProvider, useAuth } from "./contexts/AuthContext";
import { ColorThemeProvider } from "./contexts/ColorThemeContext";
import { SettingsProvider } from "./contexts/SettingsContext";
import { ThemeProvider } from "./contexts/ThemeContext";
import { UpdateNotificationProvider } from "./contexts/UpdateNotificationContext";

// Lazy load pages
const Dashboard = lazy(() => import("./pages/Dashboard"));
const HostDetail = lazy(() => import("./pages/HostDetail"));
const Hosts = lazy(() => import("./pages/Hosts"));
const Login = lazy(() => import("./pages/Login"));
const PackageDetail = lazy(() => import("./pages/PackageDetail"));
const Packages = lazy(() => import("./pages/Packages"));
const Profile = lazy(() => import("./pages/Profile"));
const Automation = lazy(() => import("./pages/Automation"));
const Repositories = lazy(() => import("./pages/Repositories"));
const RepositoryDetail = lazy(() => import("./pages/RepositoryDetail"));
const Docker = lazy(() => import("./pages/Docker"));
const Compliance = lazy(() => import("./pages/Compliance"));
const DockerContainerDetail = lazy(
	() => import("./pages/docker/ContainerDetail"),
);
const DockerImageDetail = lazy(() => import("./pages/docker/ImageDetail"));
const DockerHostDetail = lazy(() => import("./pages/docker/HostDetail"));
const DockerVolumeDetail = lazy(() => import("./pages/docker/VolumeDetail"));
const DockerNetworkDetail = lazy(() => import("./pages/docker/NetworkDetail"));
const AlertChannels = lazy(() => import("./pages/settings/AlertChannels"));
const Integrations = lazy(() => import("./pages/settings/Integrations"));
const Notifications = lazy(() => import("./pages/settings/Notifications"));
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
const SettingsUsers = lazy(() => import("./pages/settings/SettingsUsers"));
const SettingsMetrics = lazy(() => import("./pages/settings/SettingsMetrics"));

// Loading fallback component
const LoadingFallback = () => (
	<div className="min-h-screen bg-gradient-to-br from-primary-50 to-secondary-50 dark:from-secondary-900 dark:to-secondary-800 flex items-center justify-center">
		<div className="text-center">
			<div className="animate-spin rounded-full h-12 w-12 border-b-2 border-primary-600 mx-auto mb-4"></div>
			<p className="text-secondary-600 dark:text-secondary-300">Loading...</p>
		</div>
	</div>
);

function AppRoutes() {
	const { needsFirstTimeSetup, authPhase, isAuthenticated } = useAuth();
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
					<p className="text-secondary-600 dark:text-secondary-300">
						Checking system status...
					</p>
				</div>
			</div>
		);
	}

	// Show first-time setup if no admin users exist
	if (needsFirstTimeSetup && !isAuth) {
		return <FirstTimeAdminSetup />;
	}

	return (
		<Suspense fallback={<LoadingFallback />}>
			<Routes>
				<Route path="/login" element={<Login />} />
				<Route
					path="/"
					element={
						<ProtectedRoute requirePermission="can_view_dashboard">
							<Layout>
								<Dashboard />
							</Layout>
						</ProtectedRoute>
					}
				/>
				<Route
					path="/hosts"
					element={
						<ProtectedRoute requirePermission="can_view_hosts">
							<Layout>
								<Hosts />
							</Layout>
						</ProtectedRoute>
					}
				/>
				<Route
					path="/hosts/:hostId"
					element={
						<ProtectedRoute requirePermission="can_view_hosts">
							<Layout>
								<HostDetail />
							</Layout>
						</ProtectedRoute>
					}
				/>
				<Route
					path="/packages"
					element={
						<ProtectedRoute requirePermission="can_view_packages">
							<Layout>
								<Packages />
							</Layout>
						</ProtectedRoute>
					}
				/>
				<Route
					path="/repositories"
					element={
						<ProtectedRoute requirePermission="can_view_hosts">
							<Layout>
								<Repositories />
							</Layout>
						</ProtectedRoute>
					}
				/>
				<Route
					path="/repositories/:repositoryId"
					element={
						<ProtectedRoute requirePermission="can_view_hosts">
							<Layout>
								<RepositoryDetail />
							</Layout>
						</ProtectedRoute>
					}
				/>
				<Route
					path="/automation"
					element={
						<ProtectedRoute requirePermission="can_view_hosts">
							<Layout>
								<Automation />
							</Layout>
						</ProtectedRoute>
					}
				/>
				<Route
					path="/compliance"
					element={
						<ProtectedRoute requirePermission="can_view_hosts">
							<Layout>
								<Compliance />
							</Layout>
						</ProtectedRoute>
					}
				/>
				<Route
					path="/docker"
					element={
						<ProtectedRoute requirePermission="can_view_reports">
							<Layout>
								<Docker />
							</Layout>
						</ProtectedRoute>
					}
				/>
				<Route
					path="/docker/containers/:id"
					element={
						<ProtectedRoute requirePermission="can_view_reports">
							<Layout>
								<DockerContainerDetail />
							</Layout>
						</ProtectedRoute>
					}
				/>
				<Route
					path="/docker/images/:id"
					element={
						<ProtectedRoute requirePermission="can_view_reports">
							<Layout>
								<DockerImageDetail />
							</Layout>
						</ProtectedRoute>
					}
				/>
				<Route
					path="/docker/hosts/:id"
					element={
						<ProtectedRoute requirePermission="can_view_reports">
							<Layout>
								<DockerHostDetail />
							</Layout>
						</ProtectedRoute>
					}
				/>
				<Route
					path="/docker/volumes/:id"
					element={
						<ProtectedRoute requirePermission="can_view_reports">
							<Layout>
								<DockerVolumeDetail />
							</Layout>
						</ProtectedRoute>
					}
				/>
				<Route
					path="/docker/networks/:id"
					element={
						<ProtectedRoute requirePermission="can_view_reports">
							<Layout>
								<DockerNetworkDetail />
							</Layout>
						</ProtectedRoute>
					}
				/>
				<Route
					path="/users"
					element={
						<ProtectedRoute requirePermission="can_view_users">
							<Layout>
								<SettingsUsers />
							</Layout>
						</ProtectedRoute>
					}
				/>
				<Route
					path="/permissions"
					element={
						<ProtectedRoute requirePermission="can_manage_settings">
							<Layout>
								<SettingsUsers />
							</Layout>
						</ProtectedRoute>
					}
				/>
				<Route
					path="/settings"
					element={
						<ProtectedRoute requirePermission="can_manage_settings">
							<Layout>
								<SettingsServerConfig />
							</Layout>
						</ProtectedRoute>
					}
				/>
				<Route
					path="/settings/users"
					element={
						<ProtectedRoute requirePermission="can_view_users">
							<Layout>
								<SettingsUsers />
							</Layout>
						</ProtectedRoute>
					}
				/>
				<Route
					path="/settings/roles"
					element={
						<ProtectedRoute requirePermission="can_manage_settings">
							<Layout>
								<SettingsUsers />
							</Layout>
						</ProtectedRoute>
					}
				/>
				<Route
					path="/settings/profile"
					element={
						<ProtectedRoute>
							<Layout>
								<SettingsLayout>
									<Profile />
								</SettingsLayout>
							</Layout>
						</ProtectedRoute>
					}
				/>
				<Route
					path="/settings/host-groups"
					element={
						<ProtectedRoute requirePermission="can_manage_settings">
							<Layout>
								<SettingsHostGroups />
							</Layout>
						</ProtectedRoute>
					}
				/>
				<Route
					path="/settings/notifications"
					element={
						<ProtectedRoute requirePermission="can_manage_settings">
							<Layout>
								<SettingsLayout>
									<Notifications />
								</SettingsLayout>
							</Layout>
						</ProtectedRoute>
					}
				/>
				<Route
					path="/settings/agent-config"
					element={
						<ProtectedRoute requirePermission="can_manage_settings">
							<Layout>
								<SettingsAgentConfig />
							</Layout>
						</ProtectedRoute>
					}
				/>
				<Route
					path="/settings/agent-config/management"
					element={
						<ProtectedRoute requirePermission="can_manage_settings">
							<Layout>
								<SettingsAgentConfig />
							</Layout>
						</ProtectedRoute>
					}
				/>
				<Route
					path="/settings/server-config"
					element={
						<ProtectedRoute requirePermission="can_manage_settings">
							<Layout>
								<SettingsServerConfig />
							</Layout>
						</ProtectedRoute>
					}
				/>
				<Route
					path="/settings/server-config/version"
					element={
						<ProtectedRoute requirePermission="can_manage_settings">
							<Layout>
								<SettingsServerConfig />
							</Layout>
						</ProtectedRoute>
					}
				/>
				<Route
					path="/settings/alert-channels"
					element={
						<ProtectedRoute requirePermission="can_manage_settings">
							<Layout>
								<SettingsLayout>
									<AlertChannels />
								</SettingsLayout>
							</Layout>
						</ProtectedRoute>
					}
				/>
				<Route
					path="/settings/integrations"
					element={
						<ProtectedRoute requirePermission="can_manage_settings">
							<Layout>
								<Integrations />
							</Layout>
						</ProtectedRoute>
					}
				/>
				<Route
					path="/settings/patch-management"
					element={
						<ProtectedRoute requirePermission="can_manage_settings">
							<Layout>
								<PatchManagement />
							</Layout>
						</ProtectedRoute>
					}
				/>
				<Route
					path="/settings/server-url"
					element={
						<ProtectedRoute requirePermission="can_manage_settings">
							<Layout>
								<SettingsServerConfig />
							</Layout>
						</ProtectedRoute>
					}
				/>
				<Route
					path="/settings/server-version"
					element={
						<ProtectedRoute requirePermission="can_manage_settings">
							<Layout>
								<SettingsServerConfig />
							</Layout>
						</ProtectedRoute>
					}
				/>
				<Route
					path="/settings/branding"
					element={
						<ProtectedRoute requirePermission="can_manage_settings">
							<Layout>
								<SettingsServerConfig />
							</Layout>
						</ProtectedRoute>
					}
				/>
				<Route
					path="/settings/agent-version"
					element={
						<ProtectedRoute requirePermission="can_manage_settings">
							<Layout>
								<SettingsAgentConfig />
							</Layout>
						</ProtectedRoute>
					}
				/>
				<Route
					path="/settings/metrics"
					element={
						<ProtectedRoute requirePermission="can_manage_settings">
							<Layout>
								<SettingsMetrics />
							</Layout>
						</ProtectedRoute>
					}
				/>
				<Route
					path="/options"
					element={
						<ProtectedRoute requirePermission="can_manage_hosts">
							<Layout>
								<SettingsHostGroups />
							</Layout>
						</ProtectedRoute>
					}
				/>
				<Route
					path="/packages/:packageId"
					element={
						<ProtectedRoute requirePermission="can_view_packages">
							<Layout>
								<PackageDetail />
							</Layout>
						</ProtectedRoute>
					}
				/>
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
							<UpdateNotificationProvider>
								<LogoProvider>
									<AppRoutes />
								</LogoProvider>
							</UpdateNotificationProvider>
						</ColorThemeProvider>
					</SettingsProvider>
				</ThemeProvider>
			</AuthProvider>
		</ErrorBoundary>
	);
}

export default App;
