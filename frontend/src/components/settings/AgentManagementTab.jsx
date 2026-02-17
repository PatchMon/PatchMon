import { useMutation, useQuery } from "@tanstack/react-query";
import {
	AlertCircle,
	CheckCircle,
	ChevronDown,
	Clock,
	Download,
	ExternalLink,
	RefreshCw,
	X,
} from "lucide-react";
import { useEffect, useState } from "react";
import api from "../../utils/api";

const AgentManagementTab = () => {
	const [toasts, setToasts] = useState([]);
	const [showVersionSelector, setShowVersionSelector] = useState(false);
	const [_selectedVersion, setSelectedVersion] = useState(null);
	const [downloadProgress, setDownloadProgress] = useState(null);

	// Auto-hide toasts after 5 seconds
	useEffect(() => {
		if (toasts.length > 0) {
			const timers = toasts.map((toast) => {
				return setTimeout(() => {
					setToasts((prev) => prev.filter((t) => t.id !== toast.id));
				}, 5000);
			});
			return () => {
				timers.forEach((timer) => {
					clearTimeout(timer);
				});
			};
		}
	}, [toasts]);

	// Close version selector when clicking outside
	useEffect(() => {
		if (!showVersionSelector) return;

		const handleClickOutside = (event) => {
			const target = event.target;
			// Check if click is outside the version selector
			// Use a small delay to allow button clicks to register first
			const selectorElement = document.querySelector("[data-version-selector]");
			if (selectorElement && !selectorElement.contains(target)) {
				setTimeout(() => {
					setShowVersionSelector(false);
					setSelectedVersion(null);
				}, 100);
			}
		};

		// Use a slight delay to avoid immediate closure
		const timeoutId = setTimeout(() => {
			document.addEventListener("mousedown", handleClickOutside);
		}, 100);

		return () => {
			clearTimeout(timeoutId);
			document.removeEventListener("mousedown", handleClickOutside);
		};
	}, [showVersionSelector]);

	const showToast = (message, type = "success") => {
		const id = Date.now() + Math.random();
		setToasts((prev) => [...prev, { id, message, type }]);
	};

	const removeToast = (id) => {
		setToasts((prev) => prev.filter((t) => t.id !== id));
	};

	// Agent version queries
	const {
		data: versionInfo,
		isLoading: versionLoading,
		error: versionError,
		refetch: refetchVersion,
	} = useQuery({
		queryKey: ["agentVersion"],
		queryFn: async () => {
			const response = await api.get("/agent/version");
			return response.data;
		},
		refetchInterval: 5 * 60 * 1000, // Refetch every 5 minutes
		enabled: true, // Always enabled
		retry: 3, // Retry failed requests
	});

	const {
		data: releasesData,
		isLoading: releasesLoading,
		error: releasesError,
		refetch: refetchReleases,
	} = useQuery({
		queryKey: ["agentReleases"],
		queryFn: async () => {
			const response = await api.get("/agent/releases");
			return response.data;
		},
		enabled: false, // Only fetch when needed
		retry: 3,
	});

	const checkUpdatesMutation = useMutation({
		mutationFn: async () => {
			// First check GitHub for updates
			await api.post("/agent/version/check");
			// Then refresh current agent version detection
			await api.post("/agent/version/refresh");
		},
		onSuccess: () => {
			refetchVersion();
			showToast("Successfully checked for updates", "success");
		},
		onError: (error) => {
			showToast(`Failed to check for updates: ${error.message}`, "error");
		},
	});

	const downloadUpdateMutation = useMutation({
		mutationFn: async (version) => {
			// Try SSE endpoint first for real-time progress
			try {
				return await new Promise((resolve, reject) => {
					const versionParam = version ? `?version=${version}` : "";
					const eventSource = new EventSource(
						`/api/v1/agent/version/download-stream${versionParam}`,
						{ withCredentials: true },
					);

					let isComplete = false;

					eventSource.onmessage = (event) => {
						try {
							const data = JSON.parse(event.data);

							if (data.status === "starting") {
								setDownloadProgress({
									message: data.message,
									current: 0,
									total: data.total || 6, // Use backend total if available
								});
								showToast(data.message, "info");
							} else if (data.status === "downloading") {
								// Update progress state, only show one "downloading" toast
								setDownloadProgress({
									message: data.message,
									current: data.current,
									total: data.total,
								});
							} else if (data.status === "success") {
								// Individual architecture success - show brief notification
								showToast(`✓ ${data.architecture}`, "success");
							} else if (data.status === "failed") {
								// Individual architecture failure
								showToast(`✗ ${data.architecture}: ${data.reason}`, "error");
							} else if (
								data.status === "complete" ||
								data.status === "finished"
							) {
								// All done
								setDownloadProgress(null);
								showToast(data.message, "success");
								isComplete = true;
								eventSource.close();
								refetchVersion();
								resolve(data.result);
							} else if (data.status === "error") {
								setDownloadProgress(null);
								isComplete = true;
								eventSource.close();
								reject(new Error(data.error || "Download failed"));
							}
						} catch (err) {
							console.error("Failed to parse SSE message:", err);
						}
					};

					eventSource.onerror = (error) => {
						console.error("SSE connection error:", error);
						setDownloadProgress(null);
						if (!isComplete) {
							eventSource.close();
							reject(
								new Error(
									"Connection lost during download. Please check server logs.",
								),
							);
						}
					};

					// Timeout after 10 minutes
					setTimeout(() => {
						if (!isComplete) {
							eventSource.close();
							setDownloadProgress(null);
							reject(
								new Error("Download timeout - taking longer than expected"),
							);
						}
					}, 600000);
				});
			} catch (sseError) {
				// Fallback to legacy POST endpoint if SSE fails
				console.warn("SSE download failed, falling back to POST:", sseError);
				setDownloadProgress(null);
				showToast("Downloading in background...", "info");

				// Increase timeout for binary downloads
				const downloadResult = await api.post(
					"/agent/version/download",
					{
						version: version || null,
					},
					{
						timeout: 600000, // 10 minutes for large binary downloads
					},
				);
				// Refresh current agent version detection after download
				await api.post("/agent/version/refresh");
				// Return the download result for success handling
				return downloadResult;
			}
		},
		onSuccess: () => {
			setShowVersionSelector(false);
			setSelectedVersion(null);
			setDownloadProgress(null);
		},
		onError: (error) => {
			setDownloadProgress(null);
			showToast(`Download failed: ${error.message}`, "error");
		},
	});

	const handleDownloadClick = () => {
		// Fetch releases and show selector
		refetchReleases();
		setShowVersionSelector(true);
	};

	const handleVersionSelect = (version) => {
		setSelectedVersion(version);
		// Close dropdown immediately to prevent click-outside handler from interfering
		setShowVersionSelector(false);
		// Trigger download
		downloadUpdateMutation.mutate(version);
	};

	const getVersionStatus = () => {
		if (versionError) {
			return {
				status: "error",
				message: "Failed to load version info",
				Icon: AlertCircle,
				color: "text-red-600",
			};
		}

		if (!versionInfo || versionLoading) {
			return {
				status: "loading",
				message: "Loading version info...",
				Icon: RefreshCw,
				color: "text-gray-600",
			};
		}

		// Use the backend's updateStatus for proper semver comparison
		switch (versionInfo.updateStatus) {
			case "update-available":
				return {
					status: "update-available",
					message: `Update available: ${versionInfo.latestVersion}`,
					Icon: Clock,
					color: "text-yellow-600",
				};
			case "newer-version":
				return {
					status: "newer-version",
					message: `Newer version running: ${versionInfo.currentVersion}`,
					Icon: CheckCircle,
					color: "text-blue-600",
				};
			case "up-to-date":
				return {
					status: "up-to-date",
					message: `Up to date: ${versionInfo.latestVersion}`,
					Icon: CheckCircle,
					color: "text-green-600",
				};
			case "no-agent":
				return {
					status: "no-agent",
					message: "No agent binary found",
					Icon: AlertCircle,
					color: "text-orange-600",
				};
			case "github-unavailable":
				return {
					status: "github-unavailable",
					message: `Agent running: ${versionInfo.currentVersion} (GitHub API unavailable)`,
					Icon: CheckCircle,
					color: "text-purple-600",
				};
			case "no-data":
				return {
					status: "no-data",
					message: "No version data available",
					Icon: AlertCircle,
					color: "text-gray-600",
				};
			default:
				return {
					status: "unknown",
					message: "Version status unknown",
					Icon: AlertCircle,
					color: "text-gray-600",
				};
		}
	};

	const versionStatus = getVersionStatus();
	const StatusIcon = versionStatus.Icon;

	return (
		<div className="space-y-6">
			{/* Toast Notifications */}
			<div className="fixed top-20 right-4 z-[100] space-y-2 max-w-md">
				{toasts.map((toast) => (
					<div
						key={toast.id}
						className={`rounded-lg shadow-lg border-2 p-4 flex items-start space-x-3 animate-in slide-in-from-top-5 ${
							toast.type === "success"
								? "bg-green-50 dark:bg-green-900/90 border-green-500 dark:border-green-600"
								: toast.type === "info"
									? "bg-blue-50 dark:bg-blue-900/90 border-blue-500 dark:border-blue-600"
									: "bg-red-50 dark:bg-red-900/90 border-red-500 dark:border-red-600"
						}`}
					>
						<div
							className={`flex-shrink-0 rounded-full p-1 ${
								toast.type === "success"
									? "bg-green-100 dark:bg-green-800"
									: toast.type === "info"
										? "bg-blue-100 dark:bg-blue-800"
										: "bg-red-100 dark:bg-red-800"
							}`}
						>
							{toast.type === "success" ? (
								<CheckCircle className="h-5 w-5 text-green-600 dark:text-green-400" />
							) : toast.type === "info" ? (
								<Clock className="h-5 w-5 text-blue-600 dark:text-blue-400" />
							) : (
								<AlertCircle className="h-5 w-5 text-red-600 dark:text-red-400" />
							)}
						</div>
						<div className="flex-1 min-w-0">
							<p
								className={`text-sm font-medium break-words ${
									toast.type === "success"
										? "text-green-800 dark:text-green-100"
										: toast.type === "info"
											? "text-blue-800 dark:text-blue-100"
											: "text-red-800 dark:text-red-100"
								}`}
							>
								{toast.message}
							</p>
						</div>
						<button
							type="button"
							onClick={() => removeToast(toast.id)}
							className={`flex-shrink-0 rounded-lg p-1 transition-colors ${
								toast.type === "success"
									? "hover:bg-green-100 dark:hover:bg-green-800 text-green-600 dark:text-green-400"
									: toast.type === "info"
										? "hover:bg-blue-100 dark:hover:bg-blue-800 text-blue-600 dark:text-blue-400"
										: "hover:bg-red-100 dark:hover:bg-red-800 text-red-600 dark:text-red-400"
							}`}
						>
							<X className="h-4 w-4" />
						</button>
					</div>
				))}
			</div>

			{/* Download Progress Indicator (Persistent) */}
			{downloadProgress && (
				<div className="fixed bottom-4 right-4 z-[100] max-w-md rounded-lg shadow-lg border-2 bg-blue-50 dark:bg-blue-900/90 border-blue-500 dark:border-blue-600 p-4">
					<div className="flex items-center space-x-3">
						<RefreshCw className="h-5 w-5 text-blue-600 dark:text-blue-400 animate-spin" />
						<div className="flex-1">
							<p className="text-sm font-medium text-blue-800 dark:text-blue-100">
								{downloadProgress.message}
							</p>
							<div className="mt-2 w-full bg-blue-200 dark:bg-blue-800 rounded-full h-2">
								<div
									className="bg-blue-600 dark:bg-blue-400 h-2 rounded-full transition-all duration-300"
									style={{
										width: `${(downloadProgress.current / downloadProgress.total) * 100}%`,
									}}
								/>
							</div>
							<p className="text-xs text-blue-600 dark:text-blue-300 mt-1">
								{downloadProgress.current} of {downloadProgress.total}{" "}
								architectures
							</p>
						</div>
					</div>
				</div>
			)}

			{/* Header */}
			<div className="mb-4 md:mb-6">
				<h2 className="text-xl md:text-2xl font-bold text-secondary-900 dark:text-white mb-2">
					Agent Version Management
				</h2>
				<p className="text-sm md:text-base text-secondary-600 dark:text-secondary-400">
					Monitor and manage agent versions across your infrastructure
				</p>
			</div>

			{/* Status Banner */}
			<div
				className={`rounded-xl shadow-sm p-4 md:p-6 border-2 ${
					versionStatus.status === "up-to-date"
						? "bg-green-50 dark:bg-green-900/20 border-green-200 dark:border-green-800"
						: versionStatus.status === "update-available"
							? "bg-yellow-50 dark:bg-yellow-900/20 border-yellow-200 dark:border-yellow-800"
							: versionStatus.status === "no-agent"
								? "bg-orange-50 dark:bg-orange-900/20 border-orange-200 dark:border-orange-800"
								: "bg-white dark:bg-secondary-800 border-secondary-200 dark:border-secondary-600"
				}`}
			>
				<div className="flex flex-col sm:flex-row items-start justify-between gap-4">
					<div className="flex items-start space-x-3 md:space-x-4 flex-1 min-w-0">
						<div
							className={`p-2 md:p-3 rounded-lg flex-shrink-0 ${
								versionStatus.status === "up-to-date"
									? "bg-green-100 dark:bg-green-800"
									: versionStatus.status === "update-available"
										? "bg-yellow-100 dark:bg-yellow-800"
										: versionStatus.status === "no-agent"
											? "bg-orange-100 dark:bg-orange-800"
											: "bg-secondary-100 dark:bg-secondary-700"
							}`}
						>
							{StatusIcon && (
								<StatusIcon
									className={`h-5 w-5 md:h-6 md:w-6 ${versionStatus.color}`}
								/>
							)}
						</div>
						<div className="min-w-0 flex-1">
							<h3 className="text-base md:text-lg font-semibold text-secondary-900 dark:text-white mb-1">
								{versionStatus.message}
							</h3>
							<p className="text-xs md:text-sm text-secondary-600 dark:text-secondary-400">
								{versionStatus.status === "up-to-date" &&
									"All agent binaries are current"}
								{versionStatus.status === "update-available" &&
									"A newer version is available for download"}
								{versionStatus.status === "no-agent" &&
									"Download agent binaries to get started"}
								{versionStatus.status === "github-unavailable" &&
									"Cannot check for updates at this time"}
								{![
									"up-to-date",
									"update-available",
									"no-agent",
									"github-unavailable",
								].includes(versionStatus.status) &&
									"Version information unavailable"}
							</p>
						</div>
					</div>
					<button
						type="button"
						onClick={() => checkUpdatesMutation.mutate()}
						disabled={checkUpdatesMutation.isPending}
						className="flex items-center px-3 md:px-4 py-2 bg-white dark:bg-secondary-700 text-secondary-700 dark:text-secondary-200 rounded-lg hover:bg-secondary-50 dark:hover:bg-secondary-600 border border-secondary-300 dark:border-secondary-600 disabled:opacity-50 disabled:cursor-not-allowed transition-all duration-200 shadow-sm hover:shadow w-full sm:w-auto justify-center sm:justify-start flex-shrink-0"
					>
						<RefreshCw
							className={`h-4 w-4 mr-2 ${checkUpdatesMutation.isPending ? "animate-spin" : ""}`}
						/>
						{checkUpdatesMutation.isPending
							? "Checking..."
							: "Check for Updates"}
					</button>
				</div>
			</div>

			{/* Version Information Grid */}
			<div className="grid grid-cols-2 lg:grid-cols-3 gap-4 md:gap-6">
				{/* Current Version Card */}
				<div className="bg-white dark:bg-secondary-800 rounded-xl shadow-sm p-4 md:p-6 border border-secondary-200 dark:border-secondary-600 hover:shadow-md transition-shadow duration-200">
					<h4 className="text-xs md:text-sm font-medium text-secondary-500 dark:text-secondary-400 mb-2">
						Current Version
					</h4>
					<p className="text-xl md:text-2xl font-bold text-secondary-900 dark:text-white">
						{versionInfo?.currentVersion || (
							<span className="text-base md:text-lg text-secondary-400 dark:text-secondary-500">
								Not detected
							</span>
						)}
					</p>
				</div>

				{/* Latest Version Card */}
				<div className="bg-white dark:bg-secondary-800 rounded-xl shadow-sm p-4 md:p-6 border border-secondary-200 dark:border-secondary-600 hover:shadow-md transition-shadow duration-200">
					<h4 className="text-xs md:text-sm font-medium text-secondary-500 dark:text-secondary-400 mb-2">
						Latest Available
					</h4>
					<p className="text-xl md:text-2xl font-bold text-secondary-900 dark:text-white">
						{versionInfo?.latestVersion || (
							<span className="text-base md:text-lg text-secondary-400 dark:text-secondary-500">
								Unknown
							</span>
						)}
					</p>
				</div>

				{/* Last Checked Card */}
				<div className="bg-white dark:bg-secondary-800 rounded-xl shadow-sm p-4 md:p-6 border border-secondary-200 dark:border-secondary-600 hover:shadow-md transition-shadow duration-200 col-span-2 lg:col-span-1">
					<h4 className="text-xs md:text-sm font-medium text-secondary-500 dark:text-secondary-400 mb-2">
						Last Checked
					</h4>
					<p className="text-base md:text-lg font-semibold text-secondary-900 dark:text-white">
						{versionInfo?.lastChecked
							? new Date(versionInfo.lastChecked).toLocaleString("en-US", {
									month: "short",
									day: "numeric",
									hour: "2-digit",
									minute: "2-digit",
								})
							: "Never"}
					</p>
				</div>
			</div>

			{/* Download Updates Section */}
			<div className="bg-gradient-to-br from-primary-50 to-blue-50 dark:from-secondary-800 dark:to-secondary-800 rounded-xl shadow-sm p-4 md:p-8 border border-primary-200 dark:border-secondary-600">
				<div>
					<div className="flex-1">
						<h3 className="text-lg md:text-xl font-bold text-secondary-900 dark:text-white mb-3">
							{!versionInfo?.currentVersion
								? "Get Started with Agent Binaries"
								: versionStatus.status === "update-available"
									? "New Agent Version Available"
									: "Agent Binaries"}
						</h3>
						<p className="text-sm md:text-base text-secondary-700 dark:text-secondary-300 mb-4">
							{!versionInfo?.currentVersion
								? "No agent binaries detected. Download from GitHub to begin managing your agents."
								: versionStatus.status === "update-available"
									? `A new agent version (${versionInfo.latestVersion}) is available. Download the latest binaries from GitHub.`
									: "Download or redownload agent binaries from GitHub."}
						</p>
						<div className="flex flex-col sm:flex-row items-stretch sm:items-center gap-3 relative">
							<div
								className="relative flex-1 sm:flex-initial"
								data-version-selector
							>
								<button
									type="button"
									onClick={handleDownloadClick}
									disabled={downloadUpdateMutation.isPending || releasesLoading}
									className="w-full sm:w-auto flex items-center justify-center px-4 md:px-6 py-3 bg-primary-600 text-white rounded-lg hover:bg-primary-700 disabled:opacity-50 disabled:cursor-not-allowed transition-all duration-200 shadow-md hover:shadow-lg font-medium"
								>
									{downloadUpdateMutation.isPending ? (
										<>
											<RefreshCw className="h-5 w-5 mr-2 animate-spin" />
											Downloading...
										</>
									) : (
										<>
											<Download className="h-5 w-5 mr-2" />
											{!versionInfo?.currentVersion
												? "Download Binaries from GitHub"
												: versionStatus.status === "update-available"
													? "Download New Agent Version"
													: "Redownload Binaries"}
										</>
									)}
								</button>

								{/* Version Selector Dropdown */}
								{showVersionSelector && (
									<div
										className="absolute top-full left-0 mt-2 w-full sm:w-80 bg-white dark:bg-secondary-800 rounded-lg shadow-xl border border-secondary-200 dark:border-secondary-600 z-50 max-h-96 overflow-y-auto"
										data-version-selector
									>
										<div className="p-3 border-b border-secondary-200 dark:border-secondary-600 flex items-center justify-between">
											<h4 className="text-sm font-semibold text-secondary-900 dark:text-white">
												Select Version to Download
											</h4>
											<button
												type="button"
												onClick={() => {
													setShowVersionSelector(false);
													setSelectedVersion(null);
												}}
												className="text-secondary-500 hover:text-secondary-700 dark:text-secondary-400 dark:hover:text-secondary-200"
											>
												<X className="h-4 w-4" />
											</button>
										</div>
										<div className="p-2">
											{releasesLoading ? (
												<div className="p-4 text-center text-secondary-600 dark:text-secondary-400">
													<RefreshCw className="h-5 w-5 animate-spin mx-auto mb-2" />
													Loading releases...
												</div>
											) : releasesError ? (
												<div className="p-4 text-center text-red-600 dark:text-red-400">
													<AlertCircle className="h-5 w-5 mx-auto mb-2" />
													Failed to load releases
												</div>
											) : releasesData?.releases &&
												releasesData.releases.length > 0 ? (
												<div className="space-y-1">
													{releasesData.releases.map((release) => (
														<button
															key={release.version}
															type="button"
															onClick={(e) => {
																e.preventDefault();
																e.stopPropagation();
																handleVersionSelect(release.version);
															}}
															disabled={downloadUpdateMutation.isPending}
															className="w-full text-left px-3 py-2 rounded-md hover:bg-secondary-100 dark:hover:bg-secondary-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors flex items-center justify-between group"
														>
															<div className="flex-1 min-w-0">
																<div className="flex items-center gap-2">
																	<span className="font-medium text-secondary-900 dark:text-white">
																		{release.tag_name}
																	</span>
																	{release.prerelease && (
																		<span className="text-xs px-2 py-0.5 bg-yellow-100 dark:bg-yellow-900 text-yellow-800 dark:text-yellow-200 rounded">
																			Pre-release
																		</span>
																	)}
																	{release.draft && (
																		<span className="text-xs px-2 py-0.5 bg-gray-100 dark:bg-gray-700 text-gray-800 dark:text-gray-200 rounded">
																			Draft
																		</span>
																	)}
																</div>
																{release.published_at && (
																	<div className="text-xs text-secondary-500 dark:text-secondary-400 mt-1">
																		{new Date(
																			release.published_at,
																		).toLocaleDateString()}
																	</div>
																)}
															</div>
															<ChevronDown className="h-4 w-4 text-secondary-400 group-hover:text-secondary-600 dark:group-hover:text-secondary-300 transform rotate-[-90deg]" />
														</button>
													))}
												</div>
											) : (
												<div className="p-4 text-center text-secondary-600 dark:text-secondary-400">
													No releases available
												</div>
											)}
										</div>
									</div>
								)}
							</div>
							<a
								href="https://github.com/PatchMon/PatchMon-agent/releases"
								target="_blank"
								rel="noopener noreferrer"
								className="flex items-center justify-center px-4 py-3 text-secondary-700 dark:text-secondary-300 hover:text-primary-600 dark:hover:text-primary-400 transition-colors duration-200 font-medium border border-secondary-300 dark:border-secondary-600 rounded-lg hover:bg-secondary-50 dark:hover:bg-secondary-700"
							>
								<ExternalLink className="h-4 w-4 mr-2" />
								View on GitHub
							</a>
						</div>
					</div>
				</div>
			</div>

			{/* Supported Architectures */}
			{versionInfo?.supportedArchitectures &&
				versionInfo.supportedArchitectures.length > 0 && (
					<div className="bg-white dark:bg-secondary-800 rounded-xl shadow-sm p-4 md:p-6 border border-secondary-200 dark:border-secondary-600">
						<h4 className="text-base md:text-lg font-semibold text-secondary-900 dark:text-white mb-4">
							Supported Architectures
						</h4>
						<div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 gap-3">
							{versionInfo.supportedArchitectures.map((arch) => (
								<div
									key={arch}
									className="flex items-center justify-center px-3 md:px-4 py-2 md:py-3 bg-secondary-50 dark:bg-secondary-700 rounded-lg border border-secondary-200 dark:border-secondary-600"
								>
									<code className="text-xs md:text-sm font-mono text-secondary-700 dark:text-secondary-300">
										{arch}
									</code>
								</div>
							))}
						</div>
					</div>
				)}
		</div>
	);
};

export default AgentManagementTab;
