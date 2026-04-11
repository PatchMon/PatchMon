import { useMutation, useQuery } from "@tanstack/react-query";
import {
	AlertCircle,
	CheckCircle,
	Clock,
	Download,
	ExternalLink,
	RefreshCw,
	X,
} from "lucide-react";
import { useEffect, useState } from "react";
import { agentVersionAPI, getGlobalTimezone } from "../../utils/api";

const AgentManagementTab = () => {
	const [toasts, setToasts] = useState([]);

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
			const response = await agentVersionAPI.getInfo();
			return response.data;
		},
		refetchInterval: 5 * 60 * 1000, // Refetch every 5 minutes
		enabled: true, // Always enabled
		retry: 3, // Retry failed requests
	});

	const checkUpdatesMutation = useMutation({
		mutationFn: async () => {
			await agentVersionAPI.checkUpdates();
			await agentVersionAPI.refresh();
		},
		onSuccess: () => {
			refetchVersion();
			showToast("Successfully checked for updates", "success");
		},
		onError: (error) => {
			showToast(`Failed to check for updates: ${error.message}`, "error");
		},
	});

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

	const handleDownload = async (arch) => {
		const [os, archOnly] = arch.split("-");
		const osParam = os || "linux";
		const archParam = archOnly || "amd64";
		try {
			const response = await agentVersionAPI.download(archParam, osParam);
			const blob = new Blob([response.data]);
			const url = window.URL.createObjectURL(blob);
			const a = document.createElement("a");
			a.href = url;
			const filename =
				osParam === "windows"
					? `patchmon-agent-${osParam}-${archParam}.exe`
					: `patchmon-agent-${osParam}-${archParam}`;
			a.download = filename;
			document.body.appendChild(a);
			a.click();
			window.URL.revokeObjectURL(url);
			a.remove();
			showToast(`Downloaded ${filename}`, "success");
		} catch (err) {
			showToast(`Download failed: ${err.message}`, "error");
		}
	};

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

			{/* Header */}
			<div className="mb-4 md:mb-6">
				<h2 className="text-xl md:text-2xl font-bold text-secondary-900 dark:text-white mb-2">
					Agent Version Management
				</h2>
				<p className="text-sm md:text-base text-secondary-600 dark:text-white">
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
							<p className="text-xs md:text-sm text-secondary-600 dark:text-white">
								{versionStatus.status === "up-to-date" &&
									"All agent binaries are current"}
								{versionStatus.status === "update-available" &&
									"A newer version is available. Deploy a new server image to update."}
								{versionStatus.status === "no-agent" &&
									"Agent binaries are included in the server image."}
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
					<h4 className="text-xs md:text-sm font-medium text-secondary-500 dark:text-white mb-2">
						Current Version
					</h4>
					<p className="text-xl md:text-2xl font-bold text-secondary-900 dark:text-white">
						{versionInfo?.currentVersion || (
							<span className="text-base md:text-lg text-secondary-400 dark:text-white">
								Not detected
							</span>
						)}
					</p>
				</div>

				{/* Latest Version Card */}
				<div className="bg-white dark:bg-secondary-800 rounded-xl shadow-sm p-4 md:p-6 border border-secondary-200 dark:border-secondary-600 hover:shadow-md transition-shadow duration-200">
					<h4 className="text-xs md:text-sm font-medium text-secondary-500 dark:text-white mb-2">
						Latest Available
					</h4>
					<p className="text-xl md:text-2xl font-bold text-secondary-900 dark:text-white">
						{versionInfo?.latestVersion || (
							<span className="text-base md:text-lg text-secondary-400 dark:text-white">
								Unknown
							</span>
						)}
					</p>
				</div>

				{/* Last Checked Card */}
				<div className="bg-white dark:bg-secondary-800 rounded-xl shadow-sm p-4 md:p-6 border border-secondary-200 dark:border-secondary-600 hover:shadow-md transition-shadow duration-200 col-span-2 lg:col-span-1">
					<h4 className="text-xs md:text-sm font-medium text-secondary-500 dark:text-white mb-2">
						Last Checked
					</h4>
					<p className="text-base md:text-lg font-semibold text-secondary-900 dark:text-white">
						{versionInfo?.lastChecked
							? new Date(versionInfo.lastChecked).toLocaleString("en-US", {
									month: "short",
									day: "numeric",
									hour: "2-digit",
									minute: "2-digit",
									timeZone: getGlobalTimezone() || undefined,
								})
							: "Never"}
					</p>
				</div>
			</div>

			{/* Agent Binaries Info */}
			<div className="bg-gradient-to-br from-primary-50 to-blue-50 dark:from-secondary-800 dark:to-secondary-800 rounded-xl shadow-sm p-4 md:p-8 border border-primary-200 dark:border-secondary-600">
				<h3 className="text-lg md:text-xl font-bold text-secondary-900 dark:text-white mb-3">
					Agent Binaries
				</h3>
				<p className="text-sm md:text-base text-secondary-700 dark:text-white mb-4">
					Agent binaries are included in the server image. Deploy or pull a new
					image to update.
				</p>
				<a
					href="https://github.com/PatchMon/PatchMon"
					target="_blank"
					rel="noopener noreferrer"
					className="inline-flex items-center justify-center px-4 py-3 text-secondary-700 dark:text-white hover:text-primary-600 dark:hover:text-primary-400 transition-colors duration-200 font-medium border border-secondary-300 dark:border-secondary-600 rounded-lg hover:bg-secondary-50 dark:hover:bg-secondary-700"
				>
					<ExternalLink className="h-4 w-4 mr-2" />
					View on GitHub
				</a>
			</div>

			{/* Supported Architectures */}
			{versionInfo?.supportedArchitectures &&
				versionInfo.supportedArchitectures.length > 0 && (
					<div className="bg-white dark:bg-secondary-800 rounded-xl shadow-sm p-4 md:p-6 border border-secondary-200 dark:border-secondary-600">
						<h4 className="text-base md:text-lg font-semibold text-secondary-900 dark:text-white mb-4">
							Supported Architectures
						</h4>
						<p className="text-sm text-secondary-600 dark:text-secondary-300 mb-4">
							Download agent binaries for your platform (authenticated users
							only).
						</p>
						<div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 gap-3">
							{versionInfo.supportedArchitectures.map((arch) => (
								<div
									key={arch}
									className="flex items-center justify-between gap-2 px-3 md:px-4 py-2 md:py-3 bg-secondary-50 dark:bg-secondary-700 rounded-lg border border-secondary-200 dark:border-secondary-600"
								>
									<code className="text-xs md:text-sm font-mono text-secondary-700 dark:text-white truncate">
										{arch}
									</code>
									<button
										type="button"
										onClick={() => handleDownload(arch)}
										className="flex-shrink-0 p-1.5 rounded-lg text-primary-600 dark:text-primary-400 hover:bg-primary-50 dark:hover:bg-primary-900/30 transition-colors"
										title={`Download ${arch} binary`}
									>
										<Download className="h-4 w-4" />
									</button>
								</div>
							))}
						</div>
					</div>
				)}
		</div>
	);
};

export default AgentManagementTab;
