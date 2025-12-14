import { useQuery } from "@tanstack/react-query";
import {
	Activity,
	ArrowDown,
	ArrowUp,
	ArrowUpDown,
	CheckCircle,
	Clock,
	Play,
	Settings,
	XCircle,
	Zap,
} from "lucide-react";
import { useState } from "react";
import api from "../utils/api";

const Automation = () => {
	const [activeTab, setActiveTab] = useState("overview");
	const [sortField, setSortField] = useState("nextRunTimestamp");
	const [sortDirection, setSortDirection] = useState("asc");

	// Fetch automation overview data
	const { data: overview, isLoading: overviewLoading } = useQuery({
		queryKey: ["automation-overview"],
		queryFn: async () => {
			const response = await api.get("/automation/overview");
			return response.data.data;
		},
		refetchInterval: 30000, // Refresh every 30 seconds
	});

	// Fetch queue statistics
	useQuery({
		queryKey: ["automation-stats"],
		queryFn: async () => {
			const response = await api.get("/automation/stats");
			return response.data.data;
		},
		refetchInterval: 30000,
	});

	// Fetch recent jobs
	useQuery({
		queryKey: ["automation-jobs"],
		queryFn: async () => {
			const jobs = await Promise.all([
				api
					.get("/automation/jobs/github-update-check?limit=5")
					.then((r) => r.data.data || []),
				api
					.get("/automation/jobs/session-cleanup?limit=5")
					.then((r) => r.data.data || []),
			]);
			return {
				githubUpdate: jobs[0],
				sessionCleanup: jobs[1],
			};
		},
		refetchInterval: 30000,
	});

	const _getStatusIcon = (status) => {
		switch (status) {
			case "completed":
				return <CheckCircle className="h-4 w-4 text-green-500" />;
			case "failed":
				return <XCircle className="h-4 w-4 text-red-500" />;
			case "active":
				return <Activity className="h-4 w-4 text-blue-500 animate-pulse" />;
			default:
				return <Clock className="h-4 w-4 text-gray-500" />;
		}
	};

	const _getStatusColor = (status) => {
		switch (status) {
			case "completed":
				return "bg-green-100 text-green-800";
			case "failed":
				return "bg-red-100 text-red-800";
			case "active":
				return "bg-blue-100 text-blue-800";
			default:
				return "bg-gray-100 text-gray-800";
		}
	};

	const _formatDate = (dateString) => {
		if (!dateString) return "N/A";
		return new Date(dateString).toLocaleString();
	};

	const _formatDuration = (ms) => {
		if (!ms) return "N/A";
		return `${ms}ms`;
	};

	const getStatusBadge = (status) => {
		switch (status) {
			case "Success":
				return (
					<span className="px-2 py-1 text-xs font-medium rounded-full bg-green-100 text-green-800">
						Success
					</span>
				);
			case "Failed":
				return (
					<span className="px-2 py-1 text-xs font-medium rounded-full bg-red-100 text-red-800">
						Failed
					</span>
				);
			case "Never run":
				return (
					<span className="px-2 py-1 text-xs font-medium rounded-full bg-gray-100 text-gray-800">
						Never run
					</span>
				);
			default:
				return (
					<span className="px-2 py-1 text-xs font-medium rounded-full bg-gray-100 text-gray-800">
						{status}
					</span>
				);
		}
	};

	const getNextRunTime = (schedule, _lastRun) => {
		if (schedule === "Manual only") return "Manual trigger only";
		if (schedule.includes("Agent-driven")) return "Agent-driven (automatic)";
		if (schedule === "Daily at midnight") {
			const now = new Date();
			const tomorrow = new Date(now);
			tomorrow.setDate(tomorrow.getDate() + 1);
			tomorrow.setHours(0, 0, 0, 0);
			return tomorrow.toLocaleString([], {
				hour12: true,
				hour: "numeric",
				minute: "2-digit",
				day: "numeric",
				month: "numeric",
				year: "numeric",
			});
		}
		if (schedule === "Daily at 2 AM") {
			const now = new Date();
			const tomorrow = new Date(now);
			tomorrow.setDate(tomorrow.getDate() + 1);
			tomorrow.setHours(2, 0, 0, 0);
			return tomorrow.toLocaleString([], {
				hour12: true,
				hour: "numeric",
				minute: "2-digit",
				day: "numeric",
				month: "numeric",
				year: "numeric",
			});
		}
		if (schedule === "Daily at 3 AM") {
			const now = new Date();
			const tomorrow = new Date(now);
			tomorrow.setDate(tomorrow.getDate() + 1);
			tomorrow.setHours(3, 0, 0, 0);
			return tomorrow.toLocaleString([], {
				hour12: true,
				hour: "numeric",
				minute: "2-digit",
				day: "numeric",
				month: "numeric",
				year: "numeric",
			});
		}
		if (schedule === "Daily at 4 AM") {
			const now = new Date();
			const tomorrow = new Date(now);
			tomorrow.setDate(tomorrow.getDate() + 1);
			tomorrow.setHours(4, 0, 0, 0);
			return tomorrow.toLocaleString([], {
				hour12: true,
				hour: "numeric",
				minute: "2-digit",
				day: "numeric",
				month: "numeric",
				year: "numeric",
			});
		}
		if (schedule === "Every hour") {
			const now = new Date();
			const nextHour = new Date(now);
			nextHour.setHours(nextHour.getHours() + 1, 0, 0, 0);
			return nextHour.toLocaleString([], {
				hour12: true,
				hour: "numeric",
				minute: "2-digit",
				day: "numeric",
				month: "numeric",
				year: "numeric",
			});
		}
		if (schedule === "Every 30 minutes") {
			const now = new Date();
			const nextRun = new Date(now);
			// Round up to the next 30-minute mark
			const minutes = now.getMinutes();
			if (minutes < 30) {
				nextRun.setMinutes(30, 0, 0);
			} else {
				nextRun.setHours(nextRun.getHours() + 1, 0, 0, 0);
			}
			return nextRun.toLocaleString([], {
				hour12: true,
				hour: "numeric",
				minute: "2-digit",
				day: "numeric",
				month: "numeric",
				year: "numeric",
			});
		}
		return "Unknown";
	};

	const getNextRunTimestamp = (schedule) => {
		if (schedule === "Manual only") return Number.MAX_SAFE_INTEGER; // Manual tasks go to bottom
		if (schedule.includes("Agent-driven")) return Number.MAX_SAFE_INTEGER - 1; // Agent-driven tasks near bottom but above manual
		if (schedule === "Daily at midnight") {
			const now = new Date();
			const tomorrow = new Date(now);
			tomorrow.setDate(tomorrow.getDate() + 1);
			tomorrow.setHours(0, 0, 0, 0);
			return tomorrow.getTime();
		}
		if (schedule === "Daily at 2 AM") {
			const now = new Date();
			const tomorrow = new Date(now);
			tomorrow.setDate(tomorrow.getDate() + 1);
			tomorrow.setHours(2, 0, 0, 0);
			return tomorrow.getTime();
		}
		if (schedule === "Daily at 3 AM") {
			const now = new Date();
			const tomorrow = new Date(now);
			tomorrow.setDate(tomorrow.getDate() + 1);
			tomorrow.setHours(3, 0, 0, 0);
			return tomorrow.getTime();
		}
		if (schedule === "Daily at 4 AM") {
			const now = new Date();
			const tomorrow = new Date(now);
			tomorrow.setDate(tomorrow.getDate() + 1);
			tomorrow.setHours(4, 0, 0, 0);
			return tomorrow.getTime();
		}
		if (schedule === "Every hour") {
			const now = new Date();
			const nextHour = new Date(now);
			nextHour.setHours(nextHour.getHours() + 1, 0, 0, 0);
			return nextHour.getTime();
		}
		if (schedule === "Every 30 minutes") {
			const now = new Date();
			const nextRun = new Date(now);
			// Round up to the next 30-minute mark
			const minutes = now.getMinutes();
			if (minutes < 30) {
				nextRun.setMinutes(30, 0, 0);
			} else {
				nextRun.setHours(nextRun.getHours() + 1, 0, 0, 0);
			}
			return nextRun.getTime();
		}
		return Number.MAX_SAFE_INTEGER; // Unknown schedules go to bottom
	};

	const openBullBoard = () => {
		const token = localStorage.getItem("token");
		if (!token) {
			alert("Please log in to access the Queue Monitor");
			return;
		}

		// Use the proxied URL through the frontend (port 3000)
		// This avoids CORS issues as everything goes through the same origin
		const url = `/bullboard?token=${encodeURIComponent(token)}`;
		// Open in a new tab instead of a new window
		const bullBoardWindow = window.open(url, "_blank");

		// Add a message listener to handle authentication failures
		if (bullBoardWindow) {
			// Listen for authentication failures and refresh with token
			const checkAuth = () => {
				try {
					// Check if the Bull Board window is still open
					if (bullBoardWindow.closed) return;

					// Inject a script to handle authentication failures
					bullBoardWindow.postMessage(
						{
							type: "BULL_BOARD_TOKEN",
							token: token,
						},
						window.location.origin,
					);
				} catch (e) {
					console.log("Could not communicate with Bull Board window:", e);
				}
			};

			// Send token after a short delay to ensure Bull Board is loaded
			setTimeout(checkAuth, 1000);
		}
	};

	const triggerManualJob = async (jobType, data = {}) => {
		try {
			let endpoint;

			if (jobType === "github") {
				endpoint = "/automation/trigger/github-update";
			} else if (jobType === "sessions") {
				endpoint = "/automation/trigger/session-cleanup";
			} else if (jobType === "orphaned-repos") {
				endpoint = "/automation/trigger/orphaned-repo-cleanup";
			} else if (jobType === "orphaned-packages") {
				endpoint = "/automation/trigger/orphaned-package-cleanup";
			} else if (jobType === "docker-inventory") {
				endpoint = "/automation/trigger/docker-inventory-cleanup";
			} else if (jobType === "agent-collection") {
				endpoint = "/automation/trigger/agent-collection";
			} else if (jobType === "system-statistics") {
				endpoint = "/automation/trigger/system-statistics";
			}

			const _response = await api.post(endpoint, data);

			// Refresh data
			window.location.reload();
		} catch (error) {
			console.error("Error triggering job:", error);
			alert(
				"Failed to trigger job: " +
					(error.response?.data?.error || error.message),
			);
		}
	};

	const handleSort = (field) => {
		if (sortField === field) {
			setSortDirection(sortDirection === "asc" ? "desc" : "asc");
		} else {
			setSortField(field);
			setSortDirection("asc");
		}
	};

	const getSortIcon = (field) => {
		if (sortField !== field) return <ArrowUpDown className="h-4 w-4" />;
		return sortDirection === "asc" ? (
			<ArrowUp className="h-4 w-4" />
		) : (
			<ArrowDown className="h-4 w-4" />
		);
	};

	// Sort automations based on current sort settings
	const sortedAutomations = overview?.automations
		? [...overview.automations].sort((a, b) => {
				let aValue, bValue;

				switch (sortField) {
					case "name":
						aValue = a.name.toLowerCase();
						bValue = b.name.toLowerCase();
						break;
					case "schedule":
						aValue = a.schedule.toLowerCase();
						bValue = b.schedule.toLowerCase();
						break;
					case "lastRun":
						// Convert "Never" to empty string for proper sorting
						aValue = a.lastRun === "Never" ? "" : a.lastRun;
						bValue = b.lastRun === "Never" ? "" : b.lastRun;
						break;
					case "lastRunTimestamp":
						aValue = a.lastRunTimestamp || 0;
						bValue = b.lastRunTimestamp || 0;
						break;
					case "nextRunTimestamp":
						aValue = getNextRunTimestamp(a.schedule);
						bValue = getNextRunTimestamp(b.schedule);
						break;
					case "status":
						aValue = a.status.toLowerCase();
						bValue = b.status.toLowerCase();
						break;
					default:
						aValue = a[sortField];
						bValue = b[sortField];
				}

				if (aValue < bValue) return sortDirection === "asc" ? -1 : 1;
				if (aValue > bValue) return sortDirection === "asc" ? 1 : -1;
				return 0;
			})
		: [];

	const tabs = [{ id: "overview", name: "Overview", icon: Settings }];

	return (
		<div className="space-y-6">
			{/* Page Header */}
			<div className="flex items-center justify-between">
				<div>
					<h1 className="text-2xl font-semibold text-secondary-900 dark:text-white">
						Automation Management
					</h1>
					<p className="text-sm text-secondary-600 dark:text-secondary-400 mt-1">
						Monitor and manage automated server operations, agent
						communications, and patch deployments
					</p>
				</div>
				<div className="flex items-center gap-3">
					<button
						type="button"
						onClick={openBullBoard}
						className="btn-outline flex items-center gap-2"
						title="Open Bull Board Queue Monitor"
					>
						<svg
							className="h-4 w-4"
							xmlns="http://www.w3.org/2000/svg"
							viewBox="0 0 36 36"
							role="img"
							aria-label="Bull Board"
						>
							<circle fill="#DD2E44" cx="18" cy="18" r="18" />
							<circle fill="#FFF" cx="18" cy="18" r="13.5" />
							<circle fill="#DD2E44" cx="18" cy="18" r="10" />
							<circle fill="#FFF" cx="18" cy="18" r="6" />
							<circle fill="#DD2E44" cx="18" cy="18" r="3" />
							<path
								opacity=".2"
								d="M18.24 18.282l13.144 11.754s-2.647 3.376-7.89 5.109L17.579 18.42l.661-.138z"
							/>
							<path
								fill="#FFAC33"
								d="M18.294 19a.994.994 0 01-.704-1.699l.563-.563a.995.995 0 011.408 1.407l-.564.563a.987.987 0 01-.703.292z"
							/>
							<path
								fill="#55ACEE"
								d="M24.016 6.981c-.403 2.079 0 4.691 0 4.691l7.054-7.388c.291-1.454-.528-3.932-1.718-4.238-1.19-.306-4.079.803-5.336 6.935zm5.003 5.003c-2.079.403-4.691 0-4.691 0l7.388-7.054c1.454-.291 3.932.528 4.238 1.718.306 1.19-.803 4.079-6.935 5.336z"
							/>
							<path
								fill="#3A87C2"
								d="M32.798 4.485L21.176 17.587c-.362.362-1.673.882-2.51.046-.836-.836-.419-2.08-.057-2.443L31.815 3.501s.676-.635 1.159-.152-.176 1.136-.176 1.136z"
							/>
						</svg>
						Queue Monitor
					</button>
				</div>
			</div>

			{/* Stats Cards */}
			<div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
				{/* Scheduled Tasks Card */}
				<div className="card p-4">
					<div className="flex items-center">
						<div className="flex-shrink-0">
							<Clock className="h-5 w-5 text-warning-600 mr-2" />
						</div>
						<div className="w-0 flex-1">
							<p className="text-sm text-secondary-500 dark:text-white">
								Scheduled Tasks
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{overviewLoading ? "..." : overview?.scheduledTasks || 0}
							</p>
						</div>
					</div>
				</div>

				{/* Running Tasks Card */}
				<div className="card p-4">
					<div className="flex items-center">
						<div className="flex-shrink-0">
							<Play className="h-5 w-5 text-success-600 mr-2" />
						</div>
						<div className="w-0 flex-1">
							<p className="text-sm text-secondary-500 dark:text-white">
								Running Tasks
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{overviewLoading ? "..." : overview?.runningTasks || 0}
							</p>
						</div>
					</div>
				</div>

				{/* Failed Tasks Card */}
				<div className="card p-4">
					<div className="flex items-center">
						<div className="flex-shrink-0">
							<XCircle className="h-5 w-5 text-red-600 mr-2" />
						</div>
						<div className="w-0 flex-1">
							<p className="text-sm text-secondary-500 dark:text-white">
								Failed Tasks
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{overviewLoading ? "..." : overview?.failedTasks || 0}
							</p>
						</div>
					</div>
				</div>

				{/* Total Task Runs Card */}
				<div className="card p-4">
					<div className="flex items-center">
						<div className="flex-shrink-0">
							<Zap className="h-5 w-5 text-secondary-600 mr-2" />
						</div>
						<div className="w-0 flex-1">
							<p className="text-sm text-secondary-500 dark:text-white">
								Total Task Runs
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{overviewLoading ? "..." : overview?.totalAutomations || 0}
							</p>
						</div>
					</div>
				</div>
			</div>

			{/* Tabs */}
			<div className="mb-6">
				<div className="border-b border-gray-200 dark:border-gray-700">
					<nav className="-mb-px flex space-x-8">
						{tabs.map((tab) => (
							<button
								type="button"
								key={tab.id}
								onClick={() => setActiveTab(tab.id)}
								className={`py-2 px-1 border-b-2 font-medium text-sm flex items-center gap-2 ${
									activeTab === tab.id
										? "border-blue-500 text-blue-600 dark:text-blue-400"
										: "border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300 dark:text-gray-400 dark:hover:text-gray-300"
								}`}
							>
								<tab.icon className="h-4 w-4" />
								{tab.name}
							</button>
						))}
					</nav>
				</div>
			</div>

			{/* Tab Content */}
			{activeTab === "overview" && (
				<div className="card p-4 md:p-6">
					{overviewLoading ? (
						<div className="text-center py-8">
							<div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600 mx-auto"></div>
							<p className="mt-2 text-sm text-secondary-500">
								Loading automations...
							</p>
						</div>
					) : (
						<>
							{/* Mobile Card Layout */}
							<div className="md:hidden space-y-3">
								{sortedAutomations.map((automation) => (
									<div key={automation.queue} className="card p-4 space-y-3">
										{/* Task Name and Run Button */}
										<div className="flex items-start justify-between gap-3">
											<div className="flex-1 min-w-0">
												<div className="text-base font-semibold text-secondary-900 dark:text-white">
													{automation.name}
												</div>
												{automation.description && (
													<div className="text-sm text-secondary-500 dark:text-secondary-400 mt-1">
														{automation.description}
													</div>
												)}
											</div>
											{automation.schedule !== "Manual only" ? (
												<button
													type="button"
													onClick={() => {
														if (automation.queue.includes("github")) {
															triggerManualJob("github");
														} else if (automation.queue.includes("session")) {
															triggerManualJob("sessions");
														} else if (
															automation.queue.includes("orphaned-repo")
														) {
															triggerManualJob("orphaned-repos");
														} else if (
															automation.queue.includes("orphaned-package")
														) {
															triggerManualJob("orphaned-packages");
														} else if (
															automation.queue.includes("docker-inventory")
														) {
															triggerManualJob("docker-inventory");
														} else if (
															automation.queue.includes("agent-commands")
														) {
															triggerManualJob("agent-collection");
														} else if (
															automation.queue.includes("system-statistics")
														) {
															triggerManualJob("system-statistics");
														}
													}}
													className="inline-flex items-center justify-center w-8 h-8 border border-transparent rounded text-white bg-green-600 hover:bg-green-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-green-500 transition-colors duration-200 flex-shrink-0"
													title="Run Now"
												>
													<Play className="h-4 w-4" />
												</button>
											) : (
												<span className="text-xs text-secondary-400 dark:text-secondary-500 flex-shrink-0">
													Manual
												</span>
											)}
										</div>

										{/* Status */}
										<div>{getStatusBadge(automation.status)}</div>

										{/* Schedule and Run Times */}
										<div className="space-y-2 pt-2 border-t border-secondary-200 dark:border-secondary-600">
											<div className="flex items-center justify-between text-sm">
												<span className="text-secondary-500 dark:text-secondary-400">
													Frequency:
												</span>
												<span className="text-secondary-900 dark:text-white font-medium">
													{automation.schedule}
												</span>
											</div>
											<div className="flex items-center justify-between text-sm">
												<span className="text-secondary-500 dark:text-secondary-400">
													Last Run:
												</span>
												<span className="text-secondary-900 dark:text-white">
													{automation.lastRun}
												</span>
											</div>
											<div className="flex items-center justify-between text-sm">
												<span className="text-secondary-500 dark:text-secondary-400">
													Next Run:
												</span>
												<span className="text-secondary-900 dark:text-white">
													{getNextRunTime(
														automation.schedule,
														automation.lastRun,
													)}
												</span>
											</div>
										</div>
									</div>
								))}
							</div>

							{/* Desktop Table Layout */}
							<div className="hidden md:block overflow-x-auto">
								<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-600">
									<thead className="bg-secondary-50 dark:bg-secondary-700">
										<tr>
											<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
												Run
											</th>
											<th
												className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider cursor-pointer hover:bg-secondary-100 dark:hover:bg-secondary-600"
												onClick={() => handleSort("name")}
											>
												<div className="flex items-center gap-1">
													Task
													{getSortIcon("name")}
												</div>
											</th>
											<th
												className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider cursor-pointer hover:bg-secondary-100 dark:hover:bg-secondary-600"
												onClick={() => handleSort("schedule")}
											>
												<div className="flex items-center gap-1">
													Frequency
													{getSortIcon("schedule")}
												</div>
											</th>
											<th
												className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider cursor-pointer hover:bg-secondary-100 dark:hover:bg-secondary-600"
												onClick={() => handleSort("lastRunTimestamp")}
											>
												<div className="flex items-center gap-1">
													Last Run
													{getSortIcon("lastRunTimestamp")}
												</div>
											</th>
											<th
												className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider cursor-pointer hover:bg-secondary-100 dark:hover:bg-secondary-600"
												onClick={() => handleSort("nextRunTimestamp")}
											>
												<div className="flex items-center gap-1">
													Next Run
													{getSortIcon("nextRunTimestamp")}
												</div>
											</th>
											<th
												className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider cursor-pointer hover:bg-secondary-100 dark:hover:bg-secondary-600"
												onClick={() => handleSort("status")}
											>
												<div className="flex items-center gap-1">
													Status
													{getSortIcon("status")}
												</div>
											</th>
										</tr>
									</thead>
									<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-600">
										{sortedAutomations.map((automation) => (
											<tr
												key={automation.queue}
												className="hover:bg-secondary-50 dark:hover:bg-secondary-700"
											>
												<td className="px-4 py-2 whitespace-nowrap">
													{automation.schedule !== "Manual only" ? (
														<button
															type="button"
															onClick={() => {
																if (automation.queue.includes("github")) {
																	triggerManualJob("github");
																} else if (
																	automation.queue.includes("session")
																) {
																	triggerManualJob("sessions");
																} else if (
																	automation.queue.includes("orphaned-repo")
																) {
																	triggerManualJob("orphaned-repos");
																} else if (
																	automation.queue.includes("orphaned-package")
																) {
																	triggerManualJob("orphaned-packages");
																} else if (
																	automation.queue.includes("docker-inventory")
																) {
																	triggerManualJob("docker-inventory");
																} else if (
																	automation.queue.includes("agent-commands")
																) {
																	triggerManualJob("agent-collection");
																} else if (
																	automation.queue.includes("system-statistics")
																) {
																	triggerManualJob("system-statistics");
																}
															}}
															className="inline-flex items-center justify-center w-6 h-6 border border-transparent rounded text-white bg-green-600 hover:bg-green-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-green-500 transition-colors duration-200"
															title="Run Now"
														>
															<Play className="h-3 w-3" />
														</button>
													) : (
														<span className="text-gray-400 text-xs">
															Manual
														</span>
													)}
												</td>
												<td className="px-4 py-2 whitespace-nowrap">
													<div>
														<div className="text-sm font-medium text-secondary-900 dark:text-white">
															{automation.name}
														</div>
														<div className="text-xs text-secondary-500 dark:text-secondary-400">
															{automation.description}
														</div>
													</div>
												</td>
												<td className="px-4 py-2 whitespace-nowrap text-sm text-secondary-900 dark:text-white">
													{automation.schedule}
												</td>
												<td className="px-4 py-2 whitespace-nowrap text-sm text-secondary-900 dark:text-white">
													{automation.lastRun}
												</td>
												<td className="px-4 py-2 whitespace-nowrap text-sm text-secondary-900 dark:text-white">
													{getNextRunTime(
														automation.schedule,
														automation.lastRun,
													)}
												</td>
												<td className="px-4 py-2 whitespace-nowrap">
													{getStatusBadge(automation.status)}
												</td>
											</tr>
										))}
									</tbody>
								</table>
							</div>
						</>
					)}
				</div>
			)}
		</div>
	);
};

export default Automation;
