import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
	AlertCircle,
	CheckCircle,
	Clock,
	Code,
	Download,
	Image,
	Plus,
	Save,
	Server,
	Settings as SettingsIcon,
	Shield,
	Upload,
	X,
} from "lucide-react";

import { useEffect, useId, useState } from "react";
import UpgradeNotificationIcon from "../components/UpgradeNotificationIcon";
import { useUpdateNotification } from "../contexts/UpdateNotificationContext";
import {
	agentFileAPI,
	permissionsAPI,
	settingsAPI,
	versionAPI,
} from "../utils/api";

const Settings = () => {
	const repoPublicId = useId();
	const repoPrivateId = useId();
	const useCustomSshKeyId = useId();
	const protocolId = useId();
	const hostId = useId();
	const portId = useId();
	const updateIntervalId = useId();
	const defaultRoleId = useId();
	const githubRepoUrlId = useId();
	const sshKeyPathId = useId();
	const _scriptFileId = useId();
	const _scriptContentId = useId();
	const [formData, setFormData] = useState({
		serverProtocol: "http",
		serverHost: "localhost",
		serverPort: 3001,
		updateInterval: 60,
		autoUpdate: false,
		signupEnabled: false,
		defaultUserRole: "user",
		githubRepoUrl: "git@github.com:9technologygroup/patchmon.net.git",
		repositoryType: "public",
		sshKeyPath: "",
		useCustomSshKey: false,
	});
	const [errors, setErrors] = useState({});
	const [isDirty, setIsDirty] = useState(false);
	const [toast, setToast] = useState(null);

	// Tab management
	const [activeTab, setActiveTab] = useState("server");

	// Get update notification state
	const { updateAvailable } = useUpdateNotification();

	// Auto-hide toast after 3 seconds
	useEffect(() => {
		if (toast) {
			const timer = setTimeout(() => {
				setToast(null);
			}, 3000);
			return () => clearTimeout(timer);
		}
	}, [toast]);

	const showToast = (message, type = "success") => {
		setToast({ message, type });
	};

	// Fallback clipboard copy function for HTTP and older browsers
	const copyToClipboard = async (text) => {
		// Try modern clipboard API first
		if (navigator.clipboard?.writeText) {
			try {
				await navigator.clipboard.writeText(text);
				return true;
			} catch (err) {
				console.warn("Clipboard API failed, using fallback:", err);
			}
		}

		// Fallback for HTTP or unsupported browsers
		try {
			const textArea = document.createElement("textarea");
			textArea.value = text;
			textArea.style.position = "fixed";
			textArea.style.left = "-999999px";
			textArea.style.top = "-999999px";
			document.body.appendChild(textArea);
			textArea.focus();
			textArea.select();

			const successful = document.execCommand("copy");
			document.body.removeChild(textArea);

			if (successful) {
				return true;
			}
			throw new Error("execCommand failed");
		} catch (err) {
			console.error("Fallback copy failed:", err);
			throw err;
		}
	};

	// Tab configuration
	const tabs = [
		{ id: "server", name: "Server Configuration", icon: Server },
		{ id: "agent", name: "Agent Management", icon: SettingsIcon },
		{
			id: "version",
			name: "Server Version",
			icon: Code,
			showUpgradeIcon: updateAvailable,
		},
	];

	// Agent management state
	const [_agentInfo, _setAgentInfo] = useState({
		version: null,
		lastModified: null,
		size: null,
		loading: true,
		error: null,
	});
	const [showUploadModal, setShowUploadModal] = useState(false);

	// Logo management state
	const [logoUploadState, setLogoUploadState] = useState({
		dark: { uploading: false, error: null },
		light: { uploading: false, error: null },
		favicon: { uploading: false, error: null },
	});
	const [showLogoUploadModal, setShowLogoUploadModal] = useState(false);
	const [selectedLogoType, setSelectedLogoType] = useState("dark");

	// Version checking state
	const [versionInfo, setVersionInfo] = useState({
		currentVersion: null, // Will be loaded from API
		latestVersion: null,
		isUpdateAvailable: false,
		checking: false,
		error: null,
	});

	const [sshTestResult, setSshTestResult] = useState({
		testing: false,
		success: null,
		message: null,
		error: null,
	});

	const queryClient = useQueryClient();

	// Fetch current settings
	const {
		data: settings,
		isLoading,
		error,
	} = useQuery({
		queryKey: ["settings"],
		queryFn: () => settingsAPI.get().then((res) => res.data),
	});

	// Helper function to get curl flags based on settings
	const getCurlFlags = () => {
		return settings?.ignore_ssl_self_signed ? "-sk" : "-s";
	};

	// Fetch available roles for default user role dropdown
	const { data: roles, isLoading: rolesLoading } = useQuery({
		queryKey: ["rolePermissions"],
		queryFn: () => permissionsAPI.getRoles().then((res) => res.data),
	});

	// Update form data when settings are loaded
	useEffect(() => {
		if (settings) {
			const newFormData = {
				serverProtocol: settings.server_protocol || "http",
				serverHost: settings.server_host || "localhost",
				serverPort: settings.server_port || 3001,
				updateInterval: settings.update_interval || 60,
				autoUpdate: settings.auto_update || false,
				// biome-ignore lint/complexity/noUselessTernary: Seems to be desired given the comment
				signupEnabled: settings.signup_enabled === true ? true : false, // Explicit boolean conversion
				defaultUserRole: settings.default_user_role || "user",
				githubRepoUrl:
					settings.github_repo_url ||
					"https://github.com/PatchMon/PatchMon.git",
				repositoryType: settings.repository_type || "public",
				sshKeyPath: settings.ssh_key_path || "",
				useCustomSshKey: !!settings.ssh_key_path,
			};
			setFormData(newFormData);
			setIsDirty(false);
		}
	}, [settings]);

	// Update settings mutation
	const updateSettingsMutation = useMutation({
		mutationFn: (data) => {
			return settingsAPI.update(data).then((res) => res.data);
		},
		onSuccess: () => {
			queryClient.invalidateQueries(["settings"]);
			setIsDirty(false);
			setErrors({});
		},
		onError: (error) => {
			if (error.response?.data?.errors) {
				setErrors(
					error.response.data.errors.reduce((acc, err) => {
						acc[err.path] = err.msg;
						return acc;
					}, {}),
				);
			} else {
				setErrors({
					general: error.response?.data?.error || "Failed to update settings",
				});
			}
		},
	});

	// Agent file queries and mutations
	const {
		data: agentFileInfo,
		isLoading: agentFileLoading,
		error: agentFileError,
		refetch: refetchAgentFile,
	} = useQuery({
		queryKey: ["agentFile"],
		queryFn: () => agentFileAPI.getInfo().then((res) => res.data),
	});

	const uploadAgentMutation = useMutation({
		mutationFn: (scriptContent) =>
			agentFileAPI.upload(scriptContent).then((res) => res.data),
		onSuccess: () => {
			refetchAgentFile();
			setShowUploadModal(false);
		},
		onError: (error) => {
			console.error("Upload agent error:", error);
		},
	});

	// Logo upload mutation
	const uploadLogoMutation = useMutation({
		mutationFn: ({ logoType, fileContent, fileName }) =>
			fetch("/api/v1/settings/logos/upload", {
				method: "POST",
				headers: {
					"Content-Type": "application/json",
					Authorization: `Bearer ${localStorage.getItem("token")}`,
				},
				body: JSON.stringify({ logoType, fileContent, fileName }),
			}).then((res) => res.json()),
		onSuccess: (_data, variables) => {
			queryClient.invalidateQueries(["settings"]);
			setLogoUploadState((prev) => ({
				...prev,
				[variables.logoType]: { uploading: false, error: null },
			}));
			setShowLogoUploadModal(false);
		},
		onError: (error, variables) => {
			console.error("Upload logo error:", error);
			setLogoUploadState((prev) => ({
				...prev,
				[variables.logoType]: {
					uploading: false,
					error: error.message || "Failed to upload logo",
				},
			}));
		},
	});

	// Load current version on component mount
	useEffect(() => {
		const loadCurrentVersion = async () => {
			try {
				const response = await versionAPI.getCurrent();
				const data = response.data;
				setVersionInfo((prev) => ({
					...prev,
					currentVersion: data.version,
				}));
			} catch (error) {
				console.error("Error loading current version:", error);
			}
		};

		loadCurrentVersion();
	}, []);

	// Version checking functions
	const checkForUpdates = async () => {
		setVersionInfo((prev) => ({ ...prev, checking: true, error: null }));

		try {
			const response = await versionAPI.checkUpdates();
			const data = response.data;

			setVersionInfo({
				currentVersion: data.currentVersion,
				latestVersion: data.latestVersion,
				isUpdateAvailable: data.isUpdateAvailable,
				last_update_check: data.last_update_check,
				checking: false,
				error: null,
			});
		} catch (error) {
			console.error("Version check error:", error);
			setVersionInfo((prev) => ({
				...prev,
				checking: false,
				error: error.response?.data?.error || "Failed to check for updates",
			}));
		}
	};

	const testSshKey = async () => {
		if (!formData.sshKeyPath || !formData.githubRepoUrl) {
			setSshTestResult({
				testing: false,
				success: false,
				message: null,
				error: "Please enter both SSH key path and GitHub repository URL",
			});
			return;
		}

		setSshTestResult({
			testing: true,
			success: null,
			message: null,
			error: null,
		});

		try {
			const response = await versionAPI.testSshKey({
				sshKeyPath: formData.sshKeyPath,
				githubRepoUrl: formData.githubRepoUrl,
			});

			setSshTestResult({
				testing: false,
				success: true,
				message: response.data.message,
				error: null,
			});
		} catch (error) {
			console.error("SSH key test error:", error);
			setSshTestResult({
				testing: false,
				success: false,
				message: null,
				error: error.response?.data?.error || "Failed to test SSH key",
			});
		}
	};

	// Normalize update interval to safe presets
	const normalizeInterval = (minutes) => {
		let m = parseInt(minutes, 10);
		if (Number.isNaN(m)) return 60;
		if (m < 5) m = 5;
		if (m > 1440) m = 1440;
		// If less than 60 minutes, keep within 5-59 and step of 5
		if (m < 60) {
			return Math.min(59, Math.max(5, Math.round(m / 5) * 5));
		}
		// 60 or more: only allow exact hour multiples (60, 120, 180, 360, 720, 1440)
		const allowed = [60, 120, 180, 360, 720, 1440];
		// Snap to nearest allowed value
		let nearest = allowed[0];
		let bestDiff = Math.abs(m - nearest);
		for (const a of allowed) {
			const d = Math.abs(m - a);
			if (d < bestDiff) {
				bestDiff = d;
				nearest = a;
			}
		}
		return nearest;
	};

	const handleInputChange = (field, value) => {
		setFormData((prev) => {
			const newData = {
				...prev,
				[field]: field === "updateInterval" ? normalizeInterval(value) : value,
			};
			return newData;
		});
		setIsDirty(true);
		if (errors[field]) {
			setErrors((prev) => ({ ...prev, [field]: null }));
		}
	};

	const handleSubmit = (e) => {
		e.preventDefault();

		// Only include sshKeyPath if the toggle is enabled
		const dataToSubmit = { ...formData };
		if (!dataToSubmit.useCustomSshKey) {
			dataToSubmit.sshKeyPath = "";
		}
		// Remove the frontend-only field
		delete dataToSubmit.useCustomSshKey;

		updateSettingsMutation.mutate(dataToSubmit);
	};

	const validateForm = () => {
		const newErrors = {};

		if (!formData.serverHost.trim()) {
			newErrors.serverHost = "Server host is required";
		}

		if (
			!formData.serverPort ||
			formData.serverPort < 1 ||
			formData.serverPort > 65535
		) {
			newErrors.serverPort = "Port must be between 1 and 65535";
		}

		if (
			!formData.updateInterval ||
			formData.updateInterval < 5 ||
			formData.updateInterval > 1440
		) {
			newErrors.updateInterval =
				"Update interval must be between 5 and 1440 minutes";
		}

		setErrors(newErrors);
		return Object.keys(newErrors).length === 0;
	};

	const handleSave = () => {
		if (validateForm()) {
			// Prepare data for submission
			const dataToSubmit = { ...formData };
			if (!dataToSubmit.useCustomSshKey) {
				dataToSubmit.sshKeyPath = "";
			}
			// Remove the frontend-only field
			delete dataToSubmit.useCustomSshKey;

			updateSettingsMutation.mutate(dataToSubmit);
		}
	};

	if (isLoading) {
		return (
			<div className="flex items-center justify-center h-64">
				<div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600"></div>
			</div>
		);
	}

	if (error) {
		return (
			<div className="bg-red-50 dark:bg-red-900 border border-red-200 dark:border-red-700 rounded-md p-4">
				<div className="flex">
					<AlertCircle className="h-5 w-5 text-red-400 dark:text-red-300" />
					<div className="ml-3">
						<h3 className="text-sm font-medium text-red-800 dark:text-red-200">
							Error loading settings
						</h3>
						<p className="mt-1 text-sm text-red-700 dark:text-red-300">
							{error.response?.data?.error || "Failed to load settings"}
						</p>
					</div>
				</div>
			</div>
		);
	}

	return (
		<div className="max-w-4xl mx-auto p-6">
			{/* Toast Notification */}
			{toast && (
				<div
					className={`fixed top-4 right-4 z-50 max-w-md rounded-lg shadow-lg border-2 p-4 flex items-start space-x-3 animate-in slide-in-from-top-5 ${
						toast.type === "success"
							? "bg-green-50 dark:bg-green-900/90 border-green-500 dark:border-green-600"
							: "bg-red-50 dark:bg-red-900/90 border-red-500 dark:border-red-600"
					}`}
				>
					<div
						className={`flex-shrink-0 rounded-full p-1 ${
							toast.type === "success"
								? "bg-green-100 dark:bg-green-800"
								: "bg-red-100 dark:bg-red-800"
						}`}
					>
						{toast.type === "success" ? (
							<CheckCircle className="h-5 w-5 text-green-600 dark:text-green-400" />
						) : (
							<AlertCircle className="h-5 w-5 text-red-600 dark:text-red-400" />
						)}
					</div>
					<div className="flex-1">
						<p
							className={`text-sm font-medium ${
								toast.type === "success"
									? "text-green-800 dark:text-green-100"
									: "text-red-800 dark:text-red-100"
							}`}
						>
							{toast.message}
						</p>
					</div>
					<button
						type="button"
						onClick={() => setToast(null)}
						className={`flex-shrink-0 rounded-lg p-1 transition-colors ${
							toast.type === "success"
								? "hover:bg-green-100 dark:hover:bg-green-800 text-green-600 dark:text-green-400"
								: "hover:bg-red-100 dark:hover:bg-red-800 text-red-600 dark:text-red-400"
						}`}
					>
						<X className="h-4 w-4" />
					</button>
				</div>
			)}

			<div className="mb-8">
				<p className="text-secondary-600 dark:text-secondary-300">
					Configure your PatchMon server settings. These settings will be used
					in installation scripts and agent communications.
				</p>
			</div>

			{errors.general && (
				<div className="mb-6 bg-red-50 dark:bg-red-900 border border-red-200 dark:border-red-700 rounded-md p-4">
					<div className="flex">
						<AlertCircle className="h-5 w-5 text-red-400 dark:text-red-300" />
						<div className="ml-3">
							<p className="text-sm text-red-700 dark:text-red-300">
								{errors.general}
							</p>
						</div>
					</div>
				</div>
			)}

			{/* Tab Navigation */}
			<div className="bg-white dark:bg-secondary-800 shadow rounded-lg">
				<div className="border-b border-secondary-200 dark:border-secondary-600">
					<nav className="-mb-px flex space-x-8 px-6">
						{tabs.map((tab) => {
							const Icon = tab.icon;
							return (
								<button
									type="button"
									key={tab.id}
									onClick={() => setActiveTab(tab.id)}
									className={`py-4 px-1 border-b-2 font-medium text-sm flex items-center gap-2 ${
										activeTab === tab.id
											? "border-primary-500 text-primary-600 dark:text-primary-400"
											: "border-transparent text-secondary-500 dark:text-secondary-400 hover:text-secondary-700 dark:hover:text-secondary-300 hover:border-secondary-300 dark:hover:border-secondary-500"
									}`}
								>
									<Icon className="h-4 w-4" />
									{tab.name}
									{tab.showUpgradeIcon && (
										<UpgradeNotificationIcon className="h-3 w-3" />
									)}
								</button>
							);
						})}
					</nav>
				</div>

				{/* Tab Content */}
				<div className="p-6">
					{/* Server Configuration Tab */}
					{activeTab === "server" && (
						<form onSubmit={handleSubmit} className="space-y-6">
							<div className="flex items-center mb-6">
								<Server className="h-6 w-6 text-primary-600 mr-3" />
								<h2 className="text-xl font-semibold text-secondary-900 dark:text-white">
									Server Configuration
								</h2>
							</div>

							<div className="grid grid-cols-1 md:grid-cols-3 gap-6">
								<div>
									<label
										htmlFor={protocolId}
										className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-2"
									>
										Protocol
									</label>
									<select
										id={protocolId}
										value={formData.serverProtocol}
										onChange={(e) =>
											handleInputChange("serverProtocol", e.target.value)
										}
										className="w-full border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
									>
										<option value="http">HTTP</option>
										<option value="https">HTTPS</option>
									</select>
								</div>

								<div>
									<label
										htmlFor={hostId}
										className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-2"
									>
										Host *
									</label>
									<input
										id={hostId}
										type="text"
										value={formData.serverHost}
										onChange={(e) =>
											handleInputChange("serverHost", e.target.value)
										}
										className={`w-full border rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white ${
											errors.serverHost
												? "border-red-300 dark:border-red-500"
												: "border-secondary-300 dark:border-secondary-600"
										}`}
										placeholder="example.com"
									/>
									{errors.serverHost && (
										<p className="mt-1 text-sm text-red-600 dark:text-red-400">
											{errors.serverHost}
										</p>
									)}
								</div>

								<div>
									<label
										htmlFor={portId}
										className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-2"
									>
										Port *
									</label>
									<input
										id={portId}
										type="number"
										value={formData.serverPort}
										onChange={(e) =>
											handleInputChange(
												"serverPort",
												parseInt(e.target.value, 10),
											)
										}
										className={`w-full border rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white ${
											errors.serverPort
												? "border-red-300 dark:border-red-500"
												: "border-secondary-300 dark:border-secondary-600"
										}`}
										min="1"
										max="65535"
									/>
									{errors.serverPort && (
										<p className="mt-1 text-sm text-red-600 dark:text-red-400">
											{errors.serverPort}
										</p>
									)}
								</div>
							</div>

							<div className="mt-4 p-4 bg-secondary-50 dark:bg-secondary-700 rounded-md">
								<h4 className="text-sm font-medium text-secondary-900 dark:text-white mb-2">
									Server URL
								</h4>
								<p className="text-sm text-secondary-600 dark:text-secondary-300 font-mono">
									{formData.serverProtocol}://{formData.serverHost}:
									{formData.serverPort}
								</p>
								<p className="text-xs text-secondary-500 dark:text-secondary-400 mt-1">
									This URL will be used in installation scripts and agent
									communications.
								</p>
							</div>

							{/* Logo Management Section */}
							<div className="mt-6 pt-6 border-t border-secondary-200 dark:border-secondary-600">
								<div className="flex items-center mb-4">
									<Image className="h-5 w-5 text-primary-600 mr-2" />
									<h3 className="text-lg font-semibold text-secondary-900 dark:text-white">
										Logo & Branding
									</h3>
								</div>
								<p className="text-sm text-secondary-500 dark:text-secondary-300 mb-4">
									Customize your PatchMon installation with custom logos and
									favicon.
								</p>

								<div className="grid grid-cols-1 md:grid-cols-3 gap-4">
									{/* Dark Logo */}
									<div className="bg-white dark:bg-secondary-800 rounded-lg p-4 border border-secondary-200 dark:border-secondary-600">
										<h4 className="text-sm font-medium text-secondary-900 dark:text-white mb-3">
											Dark Logo
										</h4>
										{settings?.logo_dark && (
											<div className="flex items-center justify-center p-3 bg-secondary-50 dark:bg-secondary-700 rounded-lg mb-3">
												<img
													src={settings.logo_dark}
													alt="Dark Logo"
													className="max-h-12 max-w-full object-contain"
													onError={(e) => {
														e.target.style.display = "none";
													}}
												/>
											</div>
										)}
										<p className="text-xs text-secondary-600 dark:text-secondary-400 mb-3 truncate">
											{settings?.logo_dark
												? settings.logo_dark.split("/").pop()
												: "Default"}
										</p>
										<button
											type="button"
											onClick={() => {
												setSelectedLogoType("dark");
												setShowLogoUploadModal(true);
											}}
											disabled={logoUploadState.dark.uploading}
											className="w-full btn-outline flex items-center justify-center gap-2 text-sm py-2"
										>
											{logoUploadState.dark.uploading ? (
												<>
													<div className="animate-spin rounded-full h-3 w-3 border-b-2 border-current"></div>
													Uploading...
												</>
											) : (
												<>
													<Upload className="h-3 w-3" />
													Upload
												</>
											)}
										</button>
										{logoUploadState.dark.error && (
											<p className="text-xs text-red-600 dark:text-red-400 mt-2">
												{logoUploadState.dark.error}
											</p>
										)}
									</div>

									{/* Light Logo */}
									<div className="bg-white dark:bg-secondary-800 rounded-lg p-4 border border-secondary-200 dark:border-secondary-600">
										<h4 className="text-sm font-medium text-secondary-900 dark:text-white mb-3">
											Light Logo
										</h4>
										{settings?.logo_light && (
											<div className="flex items-center justify-center p-3 bg-secondary-50 dark:bg-secondary-700 rounded-lg mb-3">
												<img
													src={settings.logo_light}
													alt="Light Logo"
													className="max-h-12 max-w-full object-contain"
													onError={(e) => {
														e.target.style.display = "none";
													}}
												/>
											</div>
										)}
										<p className="text-xs text-secondary-600 dark:text-secondary-400 mb-3 truncate">
											{settings?.logo_light
												? settings.logo_light.split("/").pop()
												: "Default"}
										</p>
										<button
											type="button"
											onClick={() => {
												setSelectedLogoType("light");
												setShowLogoUploadModal(true);
											}}
											disabled={logoUploadState.light.uploading}
											className="w-full btn-outline flex items-center justify-center gap-2 text-sm py-2"
										>
											{logoUploadState.light.uploading ? (
												<>
													<div className="animate-spin rounded-full h-3 w-3 border-b-2 border-current"></div>
													Uploading...
												</>
											) : (
												<>
													<Upload className="h-3 w-3" />
													Upload
												</>
											)}
										</button>
										{logoUploadState.light.error && (
											<p className="text-xs text-red-600 dark:text-red-400 mt-2">
												{logoUploadState.light.error}
											</p>
										)}
									</div>

									{/* Favicon */}
									<div className="bg-white dark:bg-secondary-800 rounded-lg p-4 border border-secondary-200 dark:border-secondary-600">
										<h4 className="text-sm font-medium text-secondary-900 dark:text-white mb-3">
											Favicon
										</h4>
										{settings?.favicon && (
											<div className="flex items-center justify-center p-3 bg-secondary-50 dark:bg-secondary-700 rounded-lg mb-3">
												<img
													src={settings.favicon}
													alt="Favicon"
													className="h-8 w-8 object-contain"
													onError={(e) => {
														e.target.style.display = "none";
													}}
												/>
											</div>
										)}
										<p className="text-xs text-secondary-600 dark:text-secondary-400 mb-3 truncate">
											{settings?.favicon
												? settings.favicon.split("/").pop()
												: "Default"}
										</p>
										<button
											type="button"
											onClick={() => {
												setSelectedLogoType("favicon");
												setShowLogoUploadModal(true);
											}}
											disabled={logoUploadState.favicon.uploading}
											className="w-full btn-outline flex items-center justify-center gap-2 text-sm py-2"
										>
											{logoUploadState.favicon.uploading ? (
												<>
													<div className="animate-spin rounded-full h-3 w-3 border-b-2 border-current"></div>
													Uploading...
												</>
											) : (
												<>
													<Upload className="h-3 w-3" />
													Upload
												</>
											)}
										</button>
										{logoUploadState.favicon.error && (
											<p className="text-xs text-red-600 dark:text-red-400 mt-2">
												{logoUploadState.favicon.error}
											</p>
										)}
									</div>
								</div>

								<div className="mt-4 p-3 bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-700 rounded-md">
									<p className="text-xs text-blue-700 dark:text-blue-300">
										<strong>Supported formats:</strong> PNG, JPG, SVG.{" "}
										<strong>Max size:</strong> 5MB.
										<strong> Recommended sizes:</strong> 200x60px for logos,
										32x32px for favicon.
									</p>
								</div>
							</div>

							{/* Update Interval */}
							<div>
								<label
									htmlFor={updateIntervalId}
									className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-2"
								>
									Agent Update Interval (minutes)
								</label>

								{/* Numeric input (concise width) */}
								<div className="flex items-center gap-2">
									<input
										id={updateIntervalId}
										type="number"
										min="5"
										max="1440"
										step="5"
										value={formData.updateInterval}
										onChange={(e) => {
											const val = parseInt(e.target.value, 10);
											if (!Number.isNaN(val)) {
												handleInputChange(
													"updateInterval",
													Math.min(1440, Math.max(5, val)),
												);
											} else {
												handleInputChange("updateInterval", 60);
											}
										}}
										className={`w-28 border rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white ${
											errors.updateInterval
												? "border-red-300 dark:border-red-500"
												: "border-secondary-300 dark:border-secondary-600"
										}`}
										placeholder="60"
									/>
								</div>

								{/* Quick presets */}
								<div className="mt-3 flex flex-wrap items-center gap-2">
									{[5, 10, 15, 30, 45, 60, 120, 180, 360, 720, 1440].map(
										(m) => (
											<button
												key={m}
												type="button"
												onClick={() => handleInputChange("updateInterval", m)}
												className={`px-3 py-1.5 rounded-md text-xs font-medium border ${
													formData.updateInterval === m
														? "bg-primary-600 text-white border-primary-600"
														: "bg-white dark:bg-secondary-700 text-secondary-700 dark:text-secondary-200 border-secondary-300 dark:border-secondary-600 hover:bg-secondary-50 dark:hover:bg-secondary-600"
												}`}
												aria-label={`Set ${m} minutes`}
											>
												{m % 60 === 0 ? `${m / 60}h` : `${m}m`}
											</button>
										),
									)}
								</div>

								{/* Range slider */}
								<div className="mt-4">
									<input
										type="range"
										min="5"
										max="1440"
										step="5"
										value={formData.updateInterval}
										onChange={(e) => {
											const raw = parseInt(e.target.value, 10);
											handleInputChange(
												"updateInterval",
												normalizeInterval(raw),
											);
										}}
										className="w-full accent-primary-600"
										aria-label="Update interval slider"
									/>
								</div>

								{errors.updateInterval && (
									<p className="mt-1 text-sm text-red-600 dark:text-red-400">
										{errors.updateInterval}
									</p>
								)}

								{/* Helper text */}
								<div className="mt-2 text-sm text-secondary-600 dark:text-secondary-300">
									<span className="font-medium">Effective cadence:</span>{" "}
									{(() => {
										const mins = parseInt(formData.updateInterval, 10) || 60;
										if (mins < 60)
											return `${mins} minute${mins === 1 ? "" : "s"}`;
										const hrs = Math.floor(mins / 60);
										const rem = mins % 60;
										return `${hrs} hour${hrs === 1 ? "" : "s"}${rem ? ` ${rem} min` : ""}`;
									})()}
								</div>

								<p className="mt-1 text-xs text-secondary-500 dark:text-secondary-400">
									This affects new installations and will update existing ones
									when they next reach out.
								</p>
							</div>

							{/* Auto-Update Setting */}
							<div>
								<label className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-2">
									<div className="flex items-center gap-2">
										<input
											type="checkbox"
											checked={formData.autoUpdate}
											onChange={(e) =>
												handleInputChange("autoUpdate", e.target.checked)
											}
											className="rounded border-secondary-300 text-primary-600 shadow-sm focus:border-primary-300 focus:ring focus:ring-primary-200 focus:ring-opacity-50"
										/>
										Enable Automatic Agent Updates
									</div>
								</label>
								<p className="mt-1 text-sm text-secondary-500 dark:text-secondary-400">
									When enabled, agents will automatically update themselves when
									a newer version is available during their regular update
									cycle.
								</p>
							</div>

							{/* User Signup Setting */}
							<div>
								<label className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-2">
									<div className="flex items-center gap-2">
										<input
											type="checkbox"
											checked={formData.signupEnabled}
											onChange={(e) =>
												handleInputChange("signupEnabled", e.target.checked)
											}
											className="rounded border-secondary-300 text-primary-600 shadow-sm focus:border-primary-300 focus:ring focus:ring-primary-200 focus:ring-opacity-50"
										/>
										Enable User Self-Registration
									</div>
								</label>

								{/* Default User Role Dropdown */}
								{formData.signupEnabled && (
									<div className="mt-3 ml-6">
										<label
											htmlFor={defaultRoleId}
											className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-2"
										>
											Default Role for New Users
										</label>
										<select
											id={defaultRoleId}
											value={formData.defaultUserRole}
											onChange={(e) =>
												handleInputChange("defaultUserRole", e.target.value)
											}
											className="w-full max-w-xs border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
											disabled={rolesLoading}
										>
											{rolesLoading ? (
												<option>Loading roles...</option>
											) : roles && Array.isArray(roles) ? (
												roles.map((role) => (
													<option key={role.role} value={role.role}>
														{role.role.charAt(0).toUpperCase() +
															role.role.slice(1)}
													</option>
												))
											) : (
												<option value="user">User</option>
											)}
										</select>
										<p className="mt-1 text-xs text-secondary-500 dark:text-secondary-400">
											New users will be assigned this role when they register.
										</p>
									</div>
								)}

								<p className="mt-1 text-sm text-secondary-500 dark:text-secondary-400">
									When enabled, users can create their own accounts through the
									signup page. When disabled, only administrators can create
									user accounts.
								</p>
							</div>

							{/* Security Notice */}
							<div className="bg-blue-50 dark:bg-blue-900 border border-blue-200 dark:border-blue-700 rounded-md p-4">
								<div className="flex">
									<Shield className="h-5 w-5 text-blue-400 dark:text-blue-300" />
									<div className="ml-3">
										<h3 className="text-sm font-medium text-blue-800 dark:text-blue-200">
											Security Notice
										</h3>
										<p className="mt-1 text-sm text-blue-700 dark:text-blue-300">
											Changing these settings will affect all installation
											scripts and agent communications. Make sure the server URL
											is accessible from your client networks.
										</p>
									</div>
								</div>
							</div>

							{/* Save Button */}
							<div className="flex justify-end">
								<button
									type="button"
									onClick={handleSave}
									disabled={!isDirty || updateSettingsMutation.isPending}
									className={`inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md shadow-sm text-white ${
										!isDirty || updateSettingsMutation.isPending
											? "bg-secondary-400 cursor-not-allowed"
											: "bg-primary-600 hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500"
									}`}
								>
									{updateSettingsMutation.isPending ? (
										<>
											<div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white mr-2"></div>
											Saving...
										</>
									) : (
										<>
											<Save className="h-4 w-4 mr-2" />
											Save Settings
										</>
									)}
								</button>
							</div>

							{updateSettingsMutation.isSuccess && (
								<div className="bg-green-50 dark:bg-green-900 border border-green-200 dark:border-green-700 rounded-md p-4">
									<div className="flex">
										<CheckCircle className="h-5 w-5 text-green-400 dark:text-green-300" />
										<div className="ml-3">
											<p className="text-sm text-green-700 dark:text-green-300">
												Settings saved successfully!
											</p>
										</div>
									</div>
								</div>
							)}
						</form>
					)}

					{/* Agent Management Tab */}
					{activeTab === "agent" && (
						<div className="space-y-6">
							<div className="flex items-center justify-between mb-6">
								<div>
									<div className="flex items-center mb-2">
										<SettingsIcon className="h-6 w-6 text-primary-600 mr-3" />
										<h2 className="text-xl font-semibold text-secondary-900 dark:text-white">
											Agent File Management
										</h2>
									</div>
									<p className="text-sm text-secondary-500 dark:text-secondary-300">
										Manage the PatchMon agent script file used for installations
										and updates
									</p>
								</div>
								<div className="flex items-center gap-2">
									<button
										type="button"
										onClick={() => {
											const url = "/api/v1/hosts/agent/download";
											const link = document.createElement("a");
											link.href = url;
											link.download = "patchmon-agent.sh";
											document.body.appendChild(link);
											link.click();
											document.body.removeChild(link);
										}}
										className="btn-outline flex items-center gap-2"
									>
										<Download className="h-4 w-4" />
										Download
									</button>
									<button
										type="button"
										onClick={() => setShowUploadModal(true)}
										className="btn-primary flex items-center gap-2"
									>
										<Plus className="h-4 w-4" />
										Replace Script
									</button>
								</div>
							</div>

							{agentFileLoading ? (
								<div className="flex items-center justify-center py-8">
									<div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600"></div>
								</div>
							) : agentFileError ? (
								<div className="text-center py-8">
									<p className="text-red-600 dark:text-red-400">
										Error loading agent file: {agentFileError.message}
									</p>
								</div>
							) : !agentFileInfo?.exists ? (
								<div className="text-center py-8">
									<Code className="h-12 w-12 text-secondary-400 mx-auto mb-4" />
									<p className="text-secondary-500 dark:text-secondary-300">
										No agent script found
									</p>
									<p className="text-sm text-secondary-400 dark:text-secondary-400 mt-2">
										Upload an agent script to get started
									</p>
								</div>
							) : (
								<div className="space-y-6">
									{/* Agent File Info */}
									<div className="bg-secondary-50 dark:bg-secondary-700 rounded-lg p-6">
										<h3 className="text-lg font-medium text-secondary-900 dark:text-white mb-4">
											Current Agent Script
										</h3>
										<div className="grid grid-cols-1 md:grid-cols-3 gap-4">
											<div className="flex items-center gap-2">
												<Code className="h-4 w-4 text-blue-600 dark:text-blue-400" />
												<span className="text-sm font-medium text-secondary-700 dark:text-secondary-300">
													Version:
												</span>
												<span className="text-sm text-secondary-900 dark:text-white font-mono">
													{agentFileInfo.version}
												</span>
											</div>
											<div className="flex items-center gap-2">
												<Download className="h-4 w-4 text-green-600 dark:text-green-400" />
												<span className="text-sm font-medium text-secondary-700 dark:text-secondary-300">
													Size:
												</span>
												<span className="text-sm text-secondary-900 dark:text-white">
													{agentFileInfo.sizeFormatted}
												</span>
											</div>
											<div className="flex items-center gap-2">
												<Clock className="h-4 w-4 text-yellow-600 dark:text-yellow-400" />
												<span className="text-sm font-medium text-secondary-700 dark:text-secondary-300">
													Modified:
												</span>
												<span className="text-sm text-secondary-900 dark:text-white">
													{new Date(
														agentFileInfo.lastModified,
													).toLocaleDateString()}
												</span>
											</div>
										</div>
									</div>

									{/* Usage Instructions */}
									<div className="bg-blue-50 dark:bg-blue-900 border border-blue-200 dark:border-blue-700 rounded-md p-4">
										<div className="flex">
											<Shield className="h-5 w-5 text-blue-400 dark:text-blue-300" />
											<div className="ml-3">
												<h3 className="text-sm font-medium text-blue-800 dark:text-blue-200">
													Agent Script Usage
												</h3>
												<div className="mt-2 text-sm text-blue-700 dark:text-blue-300">
													<p className="mb-2">This script is used for:</p>
													<ul className="list-disc list-inside space-y-1">
														<li>
															New agent installations via the install script
														</li>
														<li>
															Agent downloads from the
															/api/v1/hosts/agent/download endpoint
														</li>
														<li>Manual agent deployments and updates</li>
													</ul>
												</div>
											</div>
										</div>
									</div>

									{/* Uninstall Instructions */}
									<div className="bg-red-50 dark:bg-red-900 border border-red-200 dark:border-red-700 rounded-md p-4">
										<div className="flex">
											<Shield className="h-5 w-5 text-red-400 dark:text-red-300" />
											<div className="ml-3">
												<h3 className="text-sm font-medium text-red-800 dark:text-red-200">
													Agent Uninstall Command
												</h3>
												<div className="mt-2 text-sm text-red-700 dark:text-red-300">
													<p className="mb-3">
														To completely remove PatchMon from a host:
													</p>

													{/* Agent Removal Script - Standard */}
													<div className="mb-3">
														<div className="space-y-2">
															<div className="text-xs font-semibold text-red-700 dark:text-red-300 mb-1">
																Standard Removal (preserves backups):
															</div>
															<div className="flex items-center gap-2">
																<div className="bg-red-100 dark:bg-red-800 rounded p-2 font-mono text-xs flex-1">
																	curl {getCurlFlags()} {window.location.origin}
																	/api/v1/hosts/remove | sudo sh
																</div>
																<button
																	type="button"
																	onClick={async () => {
																		try {
																			await copyToClipboard(
																				`curl ${getCurlFlags()} ${window.location.origin}/api/v1/hosts/remove | sudo sh`,
																			);
																			showToast(
																				"Standard removal command copied!",
																				"success",
																			);
																		} catch (err) {
																			console.error("Failed to copy:", err);
																			showToast(
																				"Failed to copy to clipboard",
																				"error",
																			);
																		}
																	}}
																	className="px-2 py-1 bg-red-200 dark:bg-red-700 text-red-800 dark:text-red-200 rounded text-xs hover:bg-red-300 dark:hover:bg-red-600 transition-colors"
																>
																	Copy
																</button>
															</div>
														</div>
													</div>

													{/* Agent Removal Script - Complete */}
													<div className="mb-3">
														<div className="space-y-2">
															<div className="text-xs font-semibold text-red-700 dark:text-red-300 mb-1">
																Complete Removal (includes backups):
															</div>
															<div className="flex items-center gap-2">
																<div className="bg-red-100 dark:bg-red-800 rounded p-2 font-mono text-xs flex-1">
																	curl {getCurlFlags()} {window.location.origin}
																	/api/v1/hosts/remove | sudo REMOVE_BACKUPS=1
																	sh
																</div>
																<button
																	type="button"
																	onClick={async () => {
																		try {
																			await copyToClipboard(
																				`curl ${getCurlFlags()} ${window.location.origin}/api/v1/hosts/remove | sudo REMOVE_BACKUPS=1 sh`,
																			);
																			showToast(
																				"Complete removal command copied!",
																				"success",
																			);
																		} catch (err) {
																			console.error("Failed to copy:", err);
																			showToast(
																				"Failed to copy to clipboard",
																				"error",
																			);
																		}
																	}}
																	className="px-2 py-1 bg-red-200 dark:bg-red-700 text-red-800 dark:text-red-200 rounded text-xs hover:bg-red-300 dark:hover:bg-red-600 transition-colors"
																>
																	Copy
																</button>
															</div>
															<div className="text-xs text-red-600 dark:text-red-400">
																This removes: binaries, systemd/OpenRC services,
																configuration files, logs, crontab entries, and
																backup files
															</div>
														</div>
													</div>

													<p className="mt-2 text-xs text-red-700 dark:text-red-400">
														⚠️ Standard removal preserves backup files for
														safety. Use complete removal to delete everything.
													</p>
												</div>
											</div>
										</div>
									</div>
								</div>
							)}
						</div>
					)}

					{/* Server Version Tab */}
					{activeTab === "version" && (
						<div className="space-y-6">
							<div className="flex items-center mb-6">
								<Code className="h-6 w-6 text-primary-600 mr-3" />
								<h2 className="text-xl font-semibold text-secondary-900 dark:text-white">
									Server Version Management
								</h2>
							</div>

							<div className="bg-secondary-50 dark:bg-secondary-700 rounded-lg p-6">
								<h3 className="text-lg font-medium text-secondary-900 dark:text-white mb-4">
									Version Check Configuration
								</h3>
								<p className="text-sm text-secondary-600 dark:text-secondary-300 mb-6">
									Configure automatic version checking against your GitHub
									repository to notify users of available updates.
								</p>

								<div className="space-y-4">
									<fieldset>
										<legend className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-2">
											Repository Type
										</legend>
										<div className="space-y-2">
											<div className="flex items-center">
												<input
													type="radio"
													id={repoPublicId}
													name="repositoryType"
													value="public"
													checked={formData.repositoryType === "public"}
													onChange={(e) =>
														handleInputChange("repositoryType", e.target.value)
													}
													className="h-4 w-4 text-primary-600 focus:ring-primary-500 border-gray-300"
												/>
												<label
													htmlFor={repoPublicId}
													className="ml-2 text-sm text-secondary-700 dark:text-secondary-200"
												>
													Public Repository (uses GitHub API - no authentication
													required)
												</label>
											</div>
											<div className="flex items-center">
												<input
													type="radio"
													id={repoPrivateId}
													name="repositoryType"
													value="private"
													checked={formData.repositoryType === "private"}
													onChange={(e) =>
														handleInputChange("repositoryType", e.target.value)
													}
													className="h-4 w-4 text-primary-600 focus:ring-primary-500 border-gray-300"
												/>
												<label
													htmlFor={repoPrivateId}
													className="ml-2 text-sm text-secondary-700 dark:text-secondary-200"
												>
													Private Repository (uses SSH with deploy key)
												</label>
											</div>
										</div>
										<p className="mt-1 text-xs text-secondary-500 dark:text-secondary-400">
											Choose whether your repository is public or private to
											determine the appropriate access method.
										</p>
									</fieldset>

									<div>
										<label
											htmlFor={githubRepoUrlId}
											className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-2"
										>
											GitHub Repository URL
										</label>
										<input
											id={githubRepoUrlId}
											type="text"
											value={formData.githubRepoUrl || ""}
											onChange={(e) =>
												handleInputChange("githubRepoUrl", e.target.value)
											}
											className="w-full border border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white font-mono text-sm"
											placeholder="git@github.com:username/repository.git"
										/>
										<p className="mt-1 text-xs text-secondary-500 dark:text-secondary-400">
											SSH or HTTPS URL to your GitHub repository
										</p>
									</div>

									{formData.repositoryType === "private" && (
										<div>
											<div className="flex items-center gap-3 mb-3">
												<input
													type="checkbox"
													id={useCustomSshKeyId}
													checked={formData.useCustomSshKey}
													onChange={(e) => {
														const checked = e.target.checked;
														handleInputChange("useCustomSshKey", checked);
														if (!checked) {
															handleInputChange("sshKeyPath", "");
														}
													}}
													className="h-4 w-4 text-primary-600 focus:ring-primary-500 border-gray-300 rounded"
												/>
												<label
													htmlFor={useCustomSshKeyId}
													className="text-sm font-medium text-secondary-700 dark:text-secondary-200"
												>
													Set custom SSH key path
												</label>
											</div>

											{formData.useCustomSshKey && (
												<div>
													<label
														htmlFor={sshKeyPathId}
														className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-2"
													>
														SSH Key Path
													</label>
													<input
														id={sshKeyPathId}
														type="text"
														value={formData.sshKeyPath || ""}
														onChange={(e) =>
															handleInputChange("sshKeyPath", e.target.value)
														}
														className="w-full border border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white font-mono text-sm"
														placeholder="/root/.ssh/id_ed25519"
													/>
													<p className="mt-1 text-xs text-secondary-500 dark:text-secondary-400">
														Path to your SSH deploy key. If not set, will
														auto-detect from common locations.
													</p>

													<div className="mt-3">
														<button
															type="button"
															onClick={testSshKey}
															disabled={
																sshTestResult.testing ||
																!formData.sshKeyPath ||
																!formData.githubRepoUrl
															}
															className="px-4 py-2 text-sm font-medium text-white bg-blue-600 border border-transparent rounded-md shadow-sm hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500 disabled:opacity-50 disabled:cursor-not-allowed"
														>
															{sshTestResult.testing
																? "Testing..."
																: "Test SSH Key"}
														</button>

														{sshTestResult.success && (
															<div className="mt-2 p-3 bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800 rounded-md">
																<div className="flex items-center">
																	<CheckCircle className="h-4 w-4 text-green-600 dark:text-green-400 mr-2" />
																	<p className="text-sm text-green-800 dark:text-green-200">
																		{sshTestResult.message}
																	</p>
																</div>
															</div>
														)}

														{sshTestResult.error && (
															<div className="mt-2 p-3 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md">
																<div className="flex items-center">
																	<AlertCircle className="h-4 w-4 text-red-600 dark:text-red-400 mr-2" />
																	<p className="text-sm text-red-800 dark:text-red-200">
																		{sshTestResult.error}
																	</p>
																</div>
															</div>
														)}
													</div>
												</div>
											)}

											{!formData.useCustomSshKey && (
												<p className="text-xs text-secondary-500 dark:text-secondary-400">
													Using auto-detection for SSH key location
												</p>
											)}
										</div>
									)}

									<div className="grid grid-cols-1 md:grid-cols-2 gap-4">
										<div className="bg-white dark:bg-secondary-800 rounded-lg p-4 border border-secondary-200 dark:border-secondary-600">
											<div className="flex items-center gap-2 mb-2">
												<CheckCircle className="h-4 w-4 text-green-600 dark:text-green-400" />
												<span className="text-sm font-medium text-secondary-700 dark:text-secondary-300">
													Current Version
												</span>
											</div>
											<span className="text-lg font-mono text-secondary-900 dark:text-white">
												{versionInfo.currentVersion}
											</span>
										</div>

										<div className="bg-white dark:bg-secondary-800 rounded-lg p-4 border border-secondary-200 dark:border-secondary-600">
											<div className="flex items-center gap-2 mb-2">
												<Download className="h-4 w-4 text-blue-600 dark:text-blue-400" />
												<span className="text-sm font-medium text-secondary-700 dark:text-secondary-300">
													Latest Version
												</span>
											</div>
											<span className="text-lg font-mono text-secondary-900 dark:text-white">
												{versionInfo.checking ? (
													<span className="text-blue-600 dark:text-blue-400">
														Checking...
													</span>
												) : versionInfo.latestVersion ? (
													<span
														className={
															versionInfo.isUpdateAvailable
																? "text-orange-600 dark:text-orange-400"
																: "text-green-600 dark:text-green-400"
														}
													>
														{versionInfo.latestVersion}
														{versionInfo.isUpdateAvailable &&
															" (Update Available!)"}
													</span>
												) : (
													<span className="text-secondary-500 dark:text-secondary-400">
														Not checked
													</span>
												)}
											</span>
										</div>
									</div>

									{/* Last Checked Time */}
									{versionInfo.last_update_check && (
										<div className="bg-white dark:bg-secondary-800 rounded-lg p-4 border border-secondary-200 dark:border-secondary-600">
											<div className="flex items-center gap-2 mb-2">
												<Clock className="h-4 w-4 text-blue-600 dark:text-blue-400" />
												<span className="text-sm font-medium text-secondary-700 dark:text-secondary-300">
													Last Checked
												</span>
											</div>
											<span className="text-sm text-secondary-600 dark:text-secondary-400">
												{new Date(
													versionInfo.last_update_check,
												).toLocaleString()}
											</span>
											<p className="text-xs text-secondary-500 dark:text-secondary-400 mt-1">
												Updates are checked automatically every 24 hours
											</p>
										</div>
									)}

									<div className="flex items-center justify-between">
										<div className="flex items-center gap-3">
											<button
												type="button"
												onClick={checkForUpdates}
												disabled={versionInfo.checking}
												className="btn-primary flex items-center gap-2"
											>
												<Download className="h-4 w-4" />
												{versionInfo.checking
													? "Checking..."
													: "Check for Updates"}
											</button>
										</div>

										{/* Save Button for Version Settings */}
										<button
											type="button"
											onClick={handleSave}
											disabled={!isDirty || updateSettingsMutation.isPending}
											className={`inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md shadow-sm text-white ${
												!isDirty || updateSettingsMutation.isPending
													? "bg-secondary-400 cursor-not-allowed"
													: "bg-primary-600 hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500"
											}`}
										>
											{updateSettingsMutation.isPending ? (
												<>
													<div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white mr-2"></div>
													Saving...
												</>
											) : (
												<>
													<Save className="h-4 w-4 mr-2" />
													Save Settings
												</>
											)}
										</button>
									</div>

									{versionInfo.error && (
										<div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-700 rounded-lg p-4">
											<div className="flex">
												<AlertCircle className="h-5 w-5 text-red-400 dark:text-red-300" />
												<div className="ml-3">
													<h3 className="text-sm font-medium text-red-800 dark:text-red-200">
														Version Check Failed
													</h3>
													<p className="mt-1 text-sm text-red-700 dark:text-red-300">
														{versionInfo.error}
													</p>
													{versionInfo.error.includes("private") && (
														<p className="mt-2 text-xs text-red-600 dark:text-red-400">
															For private repositories, you may need to
															configure GitHub authentication or make the
															repository public.
														</p>
													)}
												</div>
											</div>
										</div>
									)}

									{/* Success Message for Version Settings */}
									{updateSettingsMutation.isSuccess && (
										<div className="bg-green-50 dark:bg-green-900 border border-green-200 dark:border-green-700 rounded-md p-4">
											<div className="flex">
												<CheckCircle className="h-5 w-5 text-green-400 dark:text-green-300" />
												<div className="ml-3">
													<p className="text-sm text-green-700 dark:text-green-300">
														Settings saved successfully!
													</p>
												</div>
											</div>
										</div>
									)}
								</div>
							</div>
						</div>
					)}
				</div>
			</div>

			{/* Agent Upload Modal */}
			{showUploadModal && (
				<AgentUploadModal
					isOpen={showUploadModal}
					onClose={() => setShowUploadModal(false)}
					onSubmit={uploadAgentMutation.mutate}
					isLoading={uploadAgentMutation.isPending}
					error={uploadAgentMutation.error}
				/>
			)}

			{/* Logo Upload Modal */}
			{showLogoUploadModal && (
				<LogoUploadModal
					isOpen={showLogoUploadModal}
					onClose={() => setShowLogoUploadModal(false)}
					onSubmit={uploadLogoMutation.mutate}
					isLoading={uploadLogoMutation.isPending}
					error={uploadLogoMutation.error}
					logoType={selectedLogoType}
				/>
			)}
		</div>
	);
};

// Agent Upload Modal Component
const AgentUploadModal = ({ isOpen, onClose, onSubmit, isLoading, error }) => {
	const [scriptContent, setScriptContent] = useState("");
	const [uploadError, setUploadError] = useState("");

	const handleSubmit = (e) => {
		e.preventDefault();
		setUploadError("");

		if (!scriptContent.trim()) {
			setUploadError("Script content is required");
			return;
		}

		if (!scriptContent.trim().startsWith("#!/")) {
			setUploadError(
				"Script must start with a shebang (#!/bin/bash or #!/bin/sh)",
			);
			return;
		}

		onSubmit(scriptContent);
	};

	const handleFileUpload = (e) => {
		const file = e.target.files[0];
		if (file) {
			const reader = new FileReader();
			reader.onload = (event) => {
				setScriptContent(event.target.result);
				setUploadError("");
			};
			reader.readAsText(file);
		}
	};

	if (!isOpen) return null;

	return (
		<div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
			<div className="bg-white dark:bg-secondary-800 rounded-lg shadow-xl max-w-4xl w-full mx-4 max-h-[90vh] overflow-y-auto">
				<div className="px-6 py-4 border-b border-secondary-200 dark:border-secondary-600">
					<div className="flex items-center justify-between">
						<h3 className="text-lg font-medium text-secondary-900 dark:text-white">
							Replace Agent Script
						</h3>
						<button
							type="button"
							onClick={onClose}
							className="text-secondary-400 hover:text-secondary-600 dark:text-secondary-500 dark:hover:text-secondary-300"
						>
							<X className="h-5 w-5" />
						</button>
					</div>
				</div>

				<form onSubmit={handleSubmit} className="px-6 py-4">
					<div className="space-y-4">
						<div>
							<label
								htmlFor={scriptFileId}
								className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-2"
							>
								Upload Script File
							</label>
							<input
								id={scriptFileId}
								type="file"
								accept=".sh"
								onChange={handleFileUpload}
								className="block w-full text-sm text-secondary-500 dark:text-secondary-400 file:mr-4 file:py-2 file:px-4 file:rounded-md file:border-0 file:text-sm file:font-medium file:bg-primary-50 file:text-primary-700 hover:file:bg-primary-100 dark:file:bg-primary-900 dark:file:text-primary-200"
							/>
							<p className="mt-1 text-xs text-secondary-500 dark:text-secondary-400">
								Select a .sh file to upload, or paste the script content below
							</p>
						</div>

						<div>
							<label
								htmlFor={scriptContentId}
								className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-2"
							>
								Script Content *
							</label>
							<textarea
								id={scriptContentId}
								value={scriptContent}
								onChange={(e) => {
									setScriptContent(e.target.value);
									setUploadError("");
								}}
								rows={15}
								className="block w-full border border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white font-mono text-sm"
								placeholder="#!/bin/bash&#10;&#10;# PatchMon Agent Script&#10;VERSION=&quot;1.0.0&quot;&#10;&#10;# Your script content here..."
							/>
						</div>

						{(uploadError || error) && (
							<div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md p-3">
								<p className="text-sm text-red-800 dark:text-red-200">
									{uploadError ||
										error?.response?.data?.error ||
										error?.message}
								</p>
							</div>
						)}

						<div className="bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800 rounded-md p-3">
							<div className="flex">
								<AlertCircle className="h-4 w-4 text-yellow-600 dark:text-yellow-400 mr-2 mt-0.5" />
								<div className="text-sm text-yellow-800 dark:text-yellow-200">
									<p className="font-medium">Important:</p>
									<ul className="mt-1 list-disc list-inside space-y-1">
										<li>This will replace the current agent script file</li>
										<li>A backup will be created automatically</li>
										<li>All new installations will use this script</li>
										<li>
											Existing agents will download this version on their next
											update
										</li>
									</ul>
								</div>
							</div>
						</div>
					</div>

					<div className="flex justify-end gap-3 mt-6">
						<button type="button" onClick={onClose} className="btn-outline">
							Cancel
						</button>
						<button
							type="submit"
							disabled={isLoading || !scriptContent.trim()}
							className="btn-primary"
						>
							{isLoading ? "Uploading..." : "Replace Script"}
						</button>
					</div>
				</form>
			</div>
		</div>
	);
};

// Logo Upload Modal Component
const LogoUploadModal = ({
	isOpen,
	onClose,
	onSubmit,
	isLoading,
	error,
	logoType,
}) => {
	const [selectedFile, setSelectedFile] = useState(null);
	const [previewUrl, setPreviewUrl] = useState(null);
	const [uploadError, setUploadError] = useState("");

	const handleFileSelect = (e) => {
		const file = e.target.files[0];
		if (file) {
			// Validate file type
			const allowedTypes = [
				"image/png",
				"image/jpeg",
				"image/jpg",
				"image/svg+xml",
			];
			if (!allowedTypes.includes(file.type)) {
				setUploadError("Please select a PNG, JPG, or SVG file");
				return;
			}

			// Validate file size (5MB limit)
			if (file.size > 5 * 1024 * 1024) {
				setUploadError("File size must be less than 5MB");
				return;
			}

			setSelectedFile(file);
			setUploadError("");

			// Create preview URL
			const url = URL.createObjectURL(file);
			setPreviewUrl(url);
		}
	};

	const handleSubmit = (e) => {
		e.preventDefault();
		setUploadError("");

		if (!selectedFile) {
			setUploadError("Please select a file");
			return;
		}

		// Convert file to base64
		const reader = new FileReader();
		reader.onload = (event) => {
			const base64 = event.target.result;
			onSubmit({
				logoType,
				fileContent: base64,
				fileName: selectedFile.name,
			});
		};
		reader.readAsDataURL(selectedFile);
	};

	const handleClose = () => {
		setSelectedFile(null);
		setPreviewUrl(null);
		setUploadError("");
		onClose();
	};

	if (!isOpen) return null;

	return (
		<div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
			<div className="bg-white dark:bg-secondary-800 rounded-lg shadow-xl max-w-2xl w-full mx-4 max-h-[90vh] overflow-y-auto">
				<div className="px-6 py-4 border-b border-secondary-200 dark:border-secondary-600">
					<div className="flex items-center justify-between">
						<h3 className="text-lg font-medium text-secondary-900 dark:text-white">
							Upload{" "}
							{logoType === "favicon"
								? "Favicon"
								: `${logoType.charAt(0).toUpperCase() + logoType.slice(1)} Logo`}
						</h3>
						<button
							type="button"
							onClick={handleClose}
							className="text-secondary-400 hover:text-secondary-600 dark:text-secondary-500 dark:hover:text-secondary-300"
						>
							<X className="h-5 w-5" />
						</button>
					</div>
				</div>

				<form onSubmit={handleSubmit} className="px-6 py-4">
					<div className="space-y-4">
						<div>
							<label className="block">
								<span className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-2">
									Select File
								</span>
								<input
									type="file"
									accept="image/png,image/jpeg,image/jpg,image/svg+xml"
									onChange={handleFileSelect}
									className="block w-full text-sm text-secondary-500 dark:text-secondary-400 file:mr-4 file:py-2 file:px-4 file:rounded-md file:border-0 file:text-sm file:font-medium file:bg-primary-50 file:text-primary-700 hover:file:bg-primary-100 dark:file:bg-primary-900 dark:file:text-primary-200"
								/>
							</label>
							<p className="mt-1 text-xs text-secondary-500 dark:text-secondary-400">
								Supported formats: PNG, JPG, SVG. Max size: 5MB.
								{logoType === "favicon"
									? " Recommended: 32x32px SVG."
									: " Recommended: 200x60px."}
							</p>
						</div>

						{previewUrl && (
							<div>
								<div className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-2">
									Preview
								</div>
								<div className="flex items-center justify-center p-4 bg-white dark:bg-secondary-800 rounded-lg border border-secondary-200 dark:border-secondary-600">
									<img
										src={previewUrl}
										alt="Preview"
										className={`object-contain ${
											logoType === "favicon" ? "h-8 w-8" : "max-h-16 max-w-full"
										}`}
									/>
								</div>
							</div>
						)}

						{(uploadError || error) && (
							<div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md p-3">
								<p className="text-sm text-red-800 dark:text-red-200">
									{uploadError ||
										error?.response?.data?.error ||
										error?.message}
								</p>
							</div>
						)}

						<div className="bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800 rounded-md p-3">
							<div className="flex">
								<AlertCircle className="h-4 w-4 text-yellow-600 dark:text-yellow-400 mr-2 mt-0.5" />
								<div className="text-sm text-yellow-800 dark:text-yellow-200">
									<p className="font-medium">Important:</p>
									<ul className="mt-1 list-disc list-inside space-y-1">
										<li>This will replace the current {logoType} logo</li>
										<li>A backup will be created automatically</li>
										<li>The change will be applied immediately</li>
									</ul>
								</div>
							</div>
						</div>
					</div>

					<div className="flex justify-end gap-3 mt-6">
						<button type="button" onClick={handleClose} className="btn-outline">
							Cancel
						</button>
						<button
							type="submit"
							disabled={isLoading || !selectedFile}
							className="btn-primary"
						>
							{isLoading ? "Uploading..." : "Upload Logo"}
						</button>
					</div>
				</form>
			</div>
		</div>
	);
};

export default Settings;
