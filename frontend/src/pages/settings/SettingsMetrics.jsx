import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
	AlertCircle,
	BarChart3,
	BookOpen,
	CheckCircle,
	Eye,
	EyeOff,
	Globe,
	Info,
	RefreshCw,
	Send,
	Shield,
} from "lucide-react";
import { useState } from "react";
import SettingsLayout from "../../components/SettingsLayout";

// API functions - use httpOnly cookies for auth
const metricsAPI = {
	getSettings: () =>
		fetch("/api/v1/metrics", {
			credentials: "include",
		}).then((res) => res.json()),
	updateSettings: (data) =>
		fetch("/api/v1/metrics", {
			method: "PUT",
			headers: {
				"Content-Type": "application/json",
			},
			credentials: "include",
			body: JSON.stringify(data),
		}).then((res) => res.json()),
	regenerateId: () =>
		fetch("/api/v1/metrics/regenerate-id", {
			method: "POST",
			credentials: "include",
		}).then((res) => res.json()),
	sendNow: () =>
		fetch("/api/v1/metrics/send-now", {
			method: "POST",
			credentials: "include",
		}).then((res) => res.json()),
};

const SettingsMetrics = () => {
	const queryClient = useQueryClient();
	const [showFullId, setShowFullId] = useState(false);

	// Fetch metrics settings
	const {
		data: metricsSettings,
		isLoading,
		error,
	} = useQuery({
		queryKey: ["metrics-settings"],
		queryFn: () => metricsAPI.getSettings(),
	});

	// Toggle metrics mutation
	const toggleMetricsMutation = useMutation({
		mutationFn: (enabled) =>
			metricsAPI.updateSettings({ metrics_enabled: enabled }),
		onSuccess: () => {
			queryClient.invalidateQueries(["metrics-settings"]);
		},
	});

	// Regenerate ID mutation
	const regenerateIdMutation = useMutation({
		mutationFn: () => metricsAPI.regenerateId(),
		onSuccess: () => {
			queryClient.invalidateQueries(["metrics-settings"]);
		},
	});

	// Send now mutation
	const sendNowMutation = useMutation({
		mutationFn: () => metricsAPI.sendNow(),
		onSuccess: () => {
			queryClient.invalidateQueries(["metrics-settings"]);
		},
	});

	if (isLoading) {
		return (
			<SettingsLayout>
				<div className="flex items-center justify-center h-64">
					<div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600"></div>
				</div>
			</SettingsLayout>
		);
	}

	if (error) {
		return (
			<SettingsLayout>
				<div className="bg-red-50 dark:bg-red-900 border border-red-200 dark:border-red-700 rounded-md p-4">
					<div className="flex">
						<AlertCircle className="h-5 w-5 text-red-400 dark:text-red-300" />
						<div className="ml-3">
							<h3 className="text-sm font-medium text-red-800 dark:text-red-200">
								Error loading metrics settings
							</h3>
							<p className="mt-1 text-sm text-red-700 dark:text-red-300">
								{error.message || "Failed to load settings"}
							</p>
						</div>
					</div>
				</div>
			</SettingsLayout>
		);
	}

	const maskId = (id) => {
		if (!id) return "";
		if (showFullId) return id;
		return `${id.substring(0, 8)}...${id.substring(id.length - 8)}`;
	};

	return (
		<SettingsLayout>
			<div className="space-y-6">
				{/* Header */}
				<div className="flex items-center mb-6">
					<BarChart3 className="h-6 w-6 text-primary-600 mr-3" />
					<div>
						<h2 className="text-xl font-semibold text-secondary-900 dark:text-white">
							Anonymous Metrics & Telemetry
						</h2>
						<p className="text-sm text-secondary-600 dark:text-secondary-400 mt-1">
							Help us understand PatchMonEnhanced's global usage (100% anonymous)
						</p>
					</div>
				</div>

				{/* Privacy Information */}
				<div className="bg-blue-50 dark:bg-blue-900/30 border border-blue-200 dark:border-blue-700 rounded-lg p-6">
					<div className="flex">
						<Shield className="h-6 w-6 text-blue-600 dark:text-blue-400 flex-shrink-0" />
						<div className="ml-4 flex-1">
							<h3 className="text-base font-semibold text-blue-900 dark:text-blue-100 mb-3">
								Your Privacy Matters
							</h3>
							<div className="text-sm text-blue-800 dark:text-blue-200 space-y-2">
								<p className="flex items-start">
									<CheckCircle className="h-4 w-4 mr-2 mt-0.5 flex-shrink-0" />
									<span>
										<strong>We do NOT collect:</strong> IP addresses, hostnames,
										system details, or any personally identifiable information
									</span>
								</p>
								<p className="flex items-start">
									<CheckCircle className="h-4 w-4 mr-2 mt-0.5 flex-shrink-0" />
									<span>
										<strong>We ONLY collect:</strong> An anonymous UUID (for
										deduplication) and the number of hosts you're monitoring
									</span>
								</p>
								<p className="flex items-start">
									<CheckCircle className="h-4 w-4 mr-2 mt-0.5 flex-shrink-0" />
									<span>
										<strong>Purpose:</strong> Display a live counter on our
										website showing global PatchMonEnhanced adoption
									</span>
								</p>
								<p className="flex items-start">
									<Globe className="h-4 w-4 mr-2 mt-0.5 flex-shrink-0" />
									<span>
										<strong>Open Source:</strong> All code is public and
										auditable on GitHub
									</span>
								</p>
							</div>
						</div>
					</div>

					{/* More Information Button */}
					<div className="mt-4 pt-4 border-t border-blue-200 dark:border-blue-700">
						<a
							href="https://docs.patchmon.net/books/patchmon-application-documentation/page/metrics-collection-information"
							target="_blank"
							rel="noopener noreferrer"
							className="inline-flex items-center px-4 py-2 text-sm font-medium text-blue-700 dark:text-blue-300 bg-blue-100 dark:bg-blue-900/50 rounded-md hover:bg-blue-200 dark:hover:bg-blue-900/70 transition-colors"
						>
							<BookOpen className="h-4 w-4 mr-2" />
							More Information
						</a>
					</div>
				</div>

				{/* Metrics Toggle */}
				<div className="bg-white dark:bg-secondary-800 rounded-lg border border-secondary-200 dark:border-secondary-700 p-6">
					<div className="flex items-start justify-between">
						<div className="flex-1">
							<h3 className="text-lg font-medium text-secondary-900 dark:text-white mb-2">
								Enable Anonymous Metrics
							</h3>
							<p className="text-sm text-secondary-600 dark:text-secondary-400">
								Share anonymous usage statistics to help us showcase PatchMonEnhanced's
								global adoption. Data is sent automatically every 24 hours.
							</p>
						</div>
						<button
							type="button"
							onClick={() =>
								toggleMetricsMutation.mutate(!metricsSettings?.metrics_enabled)
							}
							disabled={toggleMetricsMutation.isPending}
							className={`ml-4 relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-md border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 ${
								metricsSettings?.metrics_enabled
									? "bg-primary-600"
									: "bg-secondary-200 dark:bg-secondary-700"
							} ${toggleMetricsMutation.isPending ? "opacity-50" : ""}`}
						>
							<span
								className={`inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out ${
									metricsSettings?.metrics_enabled
										? "translate-x-5"
										: "translate-x-0"
								}`}
							/>
						</button>
					</div>

					{/* Status */}
					<div className="mt-4 pt-4 border-t border-secondary-200 dark:border-secondary-700">
						<div className="flex items-center text-sm">
							{metricsSettings?.metrics_enabled ? (
								<>
									<CheckCircle className="h-4 w-4 text-green-500 mr-2" />
									<span className="text-green-700 dark:text-green-400">
										Metrics enabled - Thank you for supporting PatchMonEnhanced!
									</span>
								</>
							) : (
								<>
									<EyeOff className="h-4 w-4 text-secondary-500 mr-2" />
									<span className="text-secondary-600 dark:text-secondary-400">
										Metrics disabled - No data is being sent
									</span>
								</>
							)}
						</div>
					</div>
				</div>

				{/* Anonymous ID Section */}
				<div className="bg-white dark:bg-secondary-800 rounded-lg border border-secondary-200 dark:border-secondary-700 p-6">
					<div className="flex items-start justify-between mb-4">
						<div>
							<h3 className="text-lg font-medium text-secondary-900 dark:text-white mb-2">
								Your Anonymous Instance ID
							</h3>
							<p className="text-sm text-secondary-600 dark:text-secondary-400">
								This UUID identifies your instance without revealing any
								personal information
							</p>
						</div>
					</div>

					<div className="mt-4 space-y-4">
						<div className="flex items-center gap-3">
							<div className="flex-1 bg-secondary-50 dark:bg-secondary-700 rounded-md p-3 font-mono text-sm break-all">
								{maskId(metricsSettings?.metrics_anonymous_id)}
							</div>
							<button
								type="button"
								onClick={() => setShowFullId(!showFullId)}
								className="p-2 text-secondary-600 dark:text-secondary-400 hover:text-secondary-900 dark:hover:text-white"
								title={showFullId ? "Hide ID" : "Show full ID"}
							>
								{showFullId ? (
									<EyeOff className="h-5 w-5" />
								) : (
									<Eye className="h-5 w-5" />
								)}
							</button>
						</div>

						<div className="flex gap-3">
							<button
								type="button"
								onClick={() => regenerateIdMutation.mutate()}
								disabled={regenerateIdMutation.isPending}
								className="inline-flex items-center px-4 py-2 border border-secondary-300 dark:border-secondary-600 text-sm font-medium rounded-md text-secondary-700 dark:text-secondary-200 bg-white dark:bg-secondary-700 hover:bg-secondary-50 dark:hover:bg-secondary-600 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500 disabled:opacity-50"
							>
								{regenerateIdMutation.isPending ? (
									<>
										<div className="animate-spin rounded-full h-4 w-4 border-b-2 border-secondary-700 dark:border-secondary-200 mr-2"></div>
										Regenerating...
									</>
								) : (
									<>
										<RefreshCw className="h-4 w-4 mr-2" />
										Regenerate ID
									</>
								)}
							</button>

							<button
								type="button"
								onClick={() => sendNowMutation.mutate()}
								disabled={
									!metricsSettings?.metrics_enabled || sendNowMutation.isPending
								}
								className="inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md text-white bg-primary-600 hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500 disabled:opacity-50 disabled:cursor-not-allowed"
							>
								{sendNowMutation.isPending ? (
									<>
										<div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white mr-2"></div>
										Sending...
									</>
								) : (
									<>
										<Send className="h-4 w-4 mr-2" />
										Send Metrics Now
									</>
								)}
							</button>
						</div>

						{metricsSettings?.metrics_last_sent && (
							<p className="text-xs text-secondary-500 dark:text-secondary-400">
								Last sent:{" "}
								{new Date(metricsSettings.metrics_last_sent).toLocaleString()}
							</p>
						)}
					</div>

					{/* Success/Error Messages */}
					{regenerateIdMutation.isSuccess && (
						<div className="mt-4 bg-green-50 dark:bg-green-900/30 border border-green-200 dark:border-green-700 rounded-md p-3">
							<div className="flex">
								<CheckCircle className="h-4 w-4 text-green-400 dark:text-green-300 mt-0.5" />
								<p className="ml-2 text-sm text-green-700 dark:text-green-300">
									Anonymous ID regenerated successfully
								</p>
							</div>
						</div>
					)}

					{sendNowMutation.isSuccess && (
						<div className="mt-4 bg-green-50 dark:bg-green-900/30 border border-green-200 dark:border-green-700 rounded-md p-3">
							<div className="flex">
								<CheckCircle className="h-4 w-4 text-green-400 dark:text-green-300 mt-0.5" />
								<div className="ml-2 text-sm text-green-700 dark:text-green-300">
									<p className="font-medium">Metrics sent successfully!</p>
									{sendNowMutation.data?.data && (
										<p className="mt-1">
											Sent: {sendNowMutation.data.data.hostCount} hosts, version{" "}
											{sendNowMutation.data.data.version}
										</p>
									)}
								</div>
							</div>
						</div>
					)}

					{sendNowMutation.isError && (
						<div className="mt-4 bg-red-50 dark:bg-red-900/30 border border-red-200 dark:border-red-700 rounded-md p-3">
							<div className="flex">
								<AlertCircle className="h-4 w-4 text-red-400 dark:text-red-300 mt-0.5" />
								<div className="ml-2 text-sm text-red-700 dark:text-red-300">
									{sendNowMutation.error?.message || "Failed to send metrics"}
								</div>
							</div>
						</div>
					)}
				</div>

				{/* Information Panel */}
				<div className="bg-secondary-50 dark:bg-secondary-800/50 border border-secondary-200 dark:border-secondary-700 rounded-lg p-6">
					<div className="flex">
						<Info className="h-5 w-5 text-secondary-500 dark:text-secondary-400 flex-shrink-0 mt-0.5" />
						<div className="ml-3 text-sm text-secondary-700 dark:text-secondary-300">
							<h4 className="font-medium mb-2">How it works:</h4>
							<ul className="space-y-1 list-disc list-inside">
								<li>
									Metrics are sent automatically every 24 hours when enabled
								</li>
								<li>
									Only host count and version number are transmitted (no
									sensitive data)
								</li>
								<li>The anonymous UUID prevents duplicate counting</li>
								<li>You can regenerate your ID or opt-out at any time</li>
								<li>
									All collected data is displayed publicly on{" "}
									<a
										href="https://patchmon.net"
										target="_blank"
										rel="noopener noreferrer"
										className="text-primary-600 dark:text-primary-400 hover:underline"
									>
										patchmon.net
									</a>
								</li>
							</ul>
						</div>
					</div>
				</div>
			</div>
		</SettingsLayout>
	);
};

export default SettingsMetrics;
