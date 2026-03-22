import { useQuery } from "@tanstack/react-query";
import {
	ArrowDown,
	ArrowUp,
	ArrowUpDown,
	Clock,
	Play,
	Settings,
	XCircle,
	Zap,
} from "lucide-react";
import { useState } from "react";
import { useToast } from "../contexts/ToastContext";
import api from "../utils/api";

const Automation = () => {
	const toast = useToast();
	const [activeTab, setActiveTab] = useState("overview");
	const [triggeringJob, setTriggeringJob] = useState(null);
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
					.get("/automation/jobs/version-update-check?limit=5")
					.then((r) => r.data.data || []),
				api
					.get("/automation/jobs/session-cleanup?limit=5")
					.then((r) => r.data.data || []),
			]);
			return {
				versionUpdate: jobs[0],
				sessionCleanup: jobs[1],
			};
		},
		refetchInterval: 30000,
	});

	const getJobTypeForQueue = (queue) => {
		if (queue?.includes("version-update-check")) return "github";
		if (queue?.includes("session")) return "sessions";
		if (queue?.includes("orphaned-repo")) return "orphaned-repos";
		if (queue?.includes("orphaned-package")) return "orphaned-packages";
		if (queue?.includes("docker-inventory")) return "docker-inventory";
		if (queue?.includes("agent-commands")) return "agent-collection";
		if (queue?.includes("system-statistics")) return "system-statistics";
		if (queue?.includes("alert-cleanup")) return "alert-cleanup";
		if (queue?.includes("host-status-monitor")) return "host-status-monitor";
		if (queue?.includes("compliance-scan-cleanup"))
			return "compliance-scan-cleanup";
		if (queue?.includes("ssg-update-check")) return "ssg-update-check";
		return null;
	};

	const getStatusBadge = (status) => {
		switch (status) {
			case "Success":
				return (
					<span className="px-2 py-1 text-xs font-medium rounded-md bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">
						Success
					</span>
				);
			case "Failed":
				return (
					<span className="px-2 py-1 text-xs font-medium rounded-md bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200">
						Failed
					</span>
				);
			case "Running":
				return (
					<span className="px-2 py-1 text-xs font-medium rounded-md bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200">
						Running
					</span>
				);
			case "Retrying":
				return (
					<span className="px-2 py-1 text-xs font-medium rounded-md bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200">
						Retrying
					</span>
				);
			case "Archived":
				return (
					<span className="px-2 py-1 text-xs font-medium rounded-md bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200">
						Archived
					</span>
				);
			case "Skipped (Disabled)":
				return (
					<span className="px-2 py-1 text-xs font-medium rounded-md bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200">
						Skipped (Disabled)
					</span>
				);
			case "Never run":
				return (
					<span className="px-2 py-1 text-xs font-medium rounded-md bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200">
						Never run
					</span>
				);
			default:
				return (
					<span className="px-2 py-1 text-xs font-medium rounded-md bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200">
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
		if (schedule === "Daily at 1 AM") {
			const now = new Date();
			const tomorrow = new Date(now);
			tomorrow.setDate(tomorrow.getDate() + 1);
			tomorrow.setHours(1, 0, 0, 0);
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
		if (schedule === "Daily at 5 AM") {
			const now = new Date();
			const tomorrow = new Date(now);
			tomorrow.setDate(tomorrow.getDate() + 1);
			tomorrow.setHours(5, 0, 0, 0);
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
		if (schedule === "Every 5 minutes") {
			const now = new Date();
			const nextRun = new Date(now);
			// Round up to the next 5-minute mark
			const minutes = now.getMinutes();
			const nextFive = Math.ceil((minutes + 1) / 5) * 5;
			if (nextFive >= 60) {
				nextRun.setHours(nextRun.getHours() + 1, nextFive - 60, 0, 0);
			} else {
				nextRun.setMinutes(nextFive, 0, 0);
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
		if (schedule === "Daily at 1 AM") {
			const now = new Date();
			const tomorrow = new Date(now);
			tomorrow.setDate(tomorrow.getDate() + 1);
			tomorrow.setHours(1, 0, 0, 0);
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
		if (schedule === "Daily at 5 AM") {
			const now = new Date();
			const tomorrow = new Date(now);
			tomorrow.setDate(tomorrow.getDate() + 1);
			tomorrow.setHours(5, 0, 0, 0);
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
		if (schedule === "Every 5 minutes") {
			const now = new Date();
			const nextRun = new Date(now);
			const minutes = now.getMinutes();
			const nextFive = Math.ceil((minutes + 1) / 5) * 5;
			if (nextFive >= 60) {
				nextRun.setHours(nextRun.getHours() + 1, nextFive - 60, 0, 0);
			} else {
				nextRun.setMinutes(nextFive, 0, 0);
			}
			return nextRun.getTime();
		}
		return Number.MAX_SAFE_INTEGER; // Unknown schedules go to bottom
	};

	const triggerManualJob = async (jobType, data = {}) => {
		setTriggeringJob(jobType);
		try {
			let endpoint;

			if (jobType === "github") {
				endpoint = "/automation/trigger/version-update";
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
			} else if (jobType === "alert-cleanup") {
				endpoint = "/automation/trigger/alert-cleanup";
			} else if (jobType === "host-status-monitor") {
				endpoint = "/automation/trigger/host-status-monitor";
			} else if (jobType === "compliance-scan-cleanup") {
				endpoint = "/compliance/scans/cleanup";
			} else if (jobType === "ssg-update-check") {
				endpoint = "/automation/trigger/ssg-update-check";
			}

			const response = await api.post(endpoint, data);
			const dataPayload = response.data?.data || {};

			// Show success feedback with job ID or enqueued count
			// (Overview auto-refreshes every 30s via refetchInterval)
			const msg = dataPayload.message || "Job triggered successfully";
			if (dataPayload.jobId) {
				toast.success(`${msg} - Job ID: ${dataPayload.jobId}`);
			} else if (typeof dataPayload.enqueued === "number") {
				toast.success(`${msg} - ${dataPayload.enqueued} job(s) queued`);
			} else {
				toast.success(msg);
			}
		} catch (error) {
			console.error("Error triggering job:", error);
			const errorMsg =
				error.response?.data?.error || error.message || "Unknown error";
			toast.error(`Failed to trigger job: ${errorMsg}`);
		} finally {
			setTriggeringJob(null);
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
					<p className="text-sm text-secondary-600 dark:text-white mt-1">
						Monitor and manage automated server operations, agent
						communications, and patch deployments
					</p>
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
			<div className="border-b border-secondary-200 dark:border-secondary-600 overflow-x-auto scrollbar-hide">
				<nav
					className="-mb-px flex space-x-4 sm:space-x-8 px-4"
					aria-label="Tabs"
				>
					{tabs.map((tab) => (
						<button
							type="button"
							key={tab.id}
							onClick={() => setActiveTab(tab.id)}
							className={`py-2 px-1 border-b-2 font-medium text-sm flex items-center gap-2 ${
								activeTab === tab.id
									? "border-blue-500 text-blue-600 dark:text-blue-400"
									: "border-transparent text-secondary-500 hover:text-secondary-700 hover:border-secondary-300 dark:text-white dark:hover:text-primary-400"
							}`}
						>
							<tab.icon className="h-4 w-4" />
							{tab.name}
						</button>
					))}
				</nav>
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
													<div className="text-sm text-secondary-500 dark:text-white mt-1">
														{automation.description}
													</div>
												)}
											</div>
											{automation.schedule !== "Manual only" ? (
												<button
													type="button"
													onClick={() => {
														const jobType = getJobTypeForQueue(
															automation.queue,
														);
														if (jobType) triggerManualJob(jobType);
													}}
													disabled={
														triggeringJob ===
														getJobTypeForQueue(automation.queue)
													}
													className="inline-flex items-center justify-center w-8 h-8 border border-transparent rounded text-white bg-green-600 hover:bg-green-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-green-500 transition-colors duration-200 flex-shrink-0 disabled:opacity-60 disabled:cursor-not-allowed"
													title="Run Now"
												>
													{triggeringJob ===
													getJobTypeForQueue(automation.queue) ? (
														<span className="animate-spin h-4 w-4 border-2 border-white border-t-transparent rounded-full" />
													) : (
														<Play className="h-4 w-4" />
													)}
												</button>
											) : (
												<span className="text-xs text-secondary-400 dark:text-white flex-shrink-0">
													Manual
												</span>
											)}
										</div>

										{/* Status */}
										<div>{getStatusBadge(automation.status)}</div>

										{/* Schedule and Run Times */}
										<div className="space-y-2 pt-2 border-t border-secondary-200 dark:border-secondary-600">
											<div className="flex items-center justify-between text-sm">
												<span className="text-secondary-500 dark:text-white">
													Frequency:
												</span>
												<span className="text-secondary-900 dark:text-white font-medium">
													{automation.schedule}
												</span>
											</div>
											<div className="flex items-center justify-between text-sm">
												<span className="text-secondary-500 dark:text-white">
													Last Run:
												</span>
												<span className="text-secondary-900 dark:text-white">
													{automation.lastRun}
												</span>
											</div>
											<div className="flex items-center justify-between text-sm">
												<span className="text-secondary-500 dark:text-white">
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
											<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider">
												Run
											</th>
											<th
												className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider cursor-pointer hover:bg-secondary-100 dark:hover:bg-secondary-600"
												onClick={() => handleSort("name")}
											>
												<div className="flex items-center gap-1">
													Task
													{getSortIcon("name")}
												</div>
											</th>
											<th
												className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider cursor-pointer hover:bg-secondary-100 dark:hover:bg-secondary-600"
												onClick={() => handleSort("schedule")}
											>
												<div className="flex items-center gap-1">
													Frequency
													{getSortIcon("schedule")}
												</div>
											</th>
											<th
												className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider cursor-pointer hover:bg-secondary-100 dark:hover:bg-secondary-600"
												onClick={() => handleSort("lastRunTimestamp")}
											>
												<div className="flex items-center gap-1">
													Last Run
													{getSortIcon("lastRunTimestamp")}
												</div>
											</th>
											<th
												className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider cursor-pointer hover:bg-secondary-100 dark:hover:bg-secondary-600"
												onClick={() => handleSort("nextRunTimestamp")}
											>
												<div className="flex items-center gap-1">
													Next Run
													{getSortIcon("nextRunTimestamp")}
												</div>
											</th>
											<th
												className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider cursor-pointer hover:bg-secondary-100 dark:hover:bg-secondary-600"
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
																const jobType = getJobTypeForQueue(
																	automation.queue,
																);
																if (jobType) triggerManualJob(jobType);
															}}
															disabled={
																triggeringJob ===
																getJobTypeForQueue(automation.queue)
															}
															className="inline-flex items-center justify-center w-6 h-6 border border-transparent rounded text-white bg-green-600 hover:bg-green-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-green-500 transition-colors duration-200 disabled:opacity-60 disabled:cursor-not-allowed"
															title="Run Now"
														>
															{triggeringJob ===
															getJobTypeForQueue(automation.queue) ? (
																<span className="animate-spin h-3 w-3 border-2 border-white border-t-transparent rounded-full" />
															) : (
																<Play className="h-3 w-3" />
															)}
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
														<div className="text-xs text-secondary-500 dark:text-white">
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
