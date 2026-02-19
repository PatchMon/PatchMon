import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { formatDistanceToNow } from "date-fns";
import {
	AlertTriangle,
	BarChart3,
	CheckCircle,
	Clock,
	Container,
	History,
	ListChecks,
	PieChart as PieChartIcon,
	Play,
	RefreshCw,
	Server,
	ShieldAlert,
	ShieldCheck,
	ShieldOff,
	StopCircle,
	Users,
	Wifi,
	WifiOff,
	X,
} from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Link, useLocation } from "react-router-dom";
import {
	Bar,
	BarChart,
	Cell,
	Pie,
	PieChart,
	ResponsiveContainer,
	Tooltip,
	XAxis,
	YAxis,
} from "recharts";
import ScanHistoryTab from "../components/compliance/ScanHistoryTab";
import ScanResultsTab from "../components/compliance/ScanResultsTab";
import {
	ComplianceProfilesPie,
	ComplianceTrendLinePlaceholder,
	FailuresBySeverityDoughnut,
	HostComplianceStatusBar,
	LastScanAgeBar,
	OpenSCAPDistributionDoughnut,
} from "../components/compliance/widgets";
import { useToast } from "../contexts/ToastContext";
import { adminHostsAPI } from "../utils/api";
import { complianceAPI } from "../utils/complianceApi";

// Custom tooltip component for consistent styling across all charts
const CustomTooltip = ({ active, payload, label, type }) => {
	if (!active || !payload || payload.length === 0) return null;

	const getTitle = () => {
		if (type === "hostStatus") return `${label} Hosts`;
		if (type === "severity") return `${label} Severity`;
		if (type === "scoreRange") return `Score: ${label}`;
		if (type === "profile") return label;
		if (type === "scanAge") return `${label}`;
		return label;
	};

	return (
		<div className="bg-gray-900 border border-gray-700 rounded-lg px-3 py-2 shadow-lg">
			<p className="text-white font-medium text-sm mb-1">{getTitle()}</p>
			<div className="space-y-1">
				{payload.map((entry, index) => {
					let name = entry.name;
					let color = entry.color;

					// Format the name for better readability
					if (name === "openscap") {
						name = "OpenSCAP";
						color = "#22c55e";
					} else if (name === "dockerBench") {
						name = "Docker Bench";
						color = "#3b82f6";
					} else if (name === "count") {
						name = "Scans";
					} else if (name === "host_count") {
						name = "Hosts";
					}

					return (
						<div
							key={`entry-${index}-${entry.name || entry.dataKey || ""}`}
							className="flex items-center justify-between gap-4 text-sm"
						>
							<div className="flex items-center gap-2">
								<div
									className="w-2.5 h-2.5 rounded-sm"
									style={{ backgroundColor: color }}
								/>
								<span className="text-gray-300">{name}</span>
							</div>
							<span className="text-white font-medium">
								{entry.value?.toLocaleString()}
							</span>
						</div>
					);
				})}
			</div>
		</div>
	);
};

const COMPLIANCE_TABS = [
	{ id: "overview", label: "Overview", icon: BarChart3 },
	{ id: "hosts", label: "Hosts", icon: Users },
	{ id: "scan-results", label: "Scan Results", icon: ListChecks },
	{ id: "history", label: "History", icon: History },
];

const Compliance = () => {
	const queryClient = useQueryClient();
	const toast = useToast();
	const location = useLocation();
	const [activeTab, setActiveTab] = useState(() => {
		const requested = location.state?.complianceTab;
		if (requested && COMPLIANCE_TABS.some((t) => t.id === requested))
			return requested;
		return "overview";
	});

	const [scanResultsFilters, setScanResultsFilters] = useState(null);

	// Handle tab navigation and filters from external links (e.g. host compliance cards)
	useEffect(() => {
		const requested = location.state?.complianceTab;
		if (requested && COMPLIANCE_TABS.some((t) => t.id === requested)) {
			setActiveTab(requested);
		}
		if (location.state?.scanResultsFilters) {
			setScanResultsFilters(location.state.scanResultsFilters);
		}
	}, [location.state?.complianceTab, location.state?.scanResultsFilters]);
	const [showBulkScanModal, setShowBulkScanModal] = useState(false);
	const [selectedHosts, setSelectedHosts] = useState([]);
	const [bulkScanOptions, setBulkScanOptions] = useState({
		profileType: "all",
		enableRemediation: false,
	});
	const [bulkScanResult, setBulkScanResult] = useState(null);
	const [pendingScans, setPendingScans] = useState([]); // Hosts where scan was triggered but not yet in database
	const prevActiveScanIds = useRef(new Set());
	const [profileTypeFilter, _setProfileTypeFilter] = useState("all"); // "all", "openscap", "docker-bench"
	const [tableFilter, setTableFilter] = useState(null); // null = compliance-enabled only, 'never-scanned' = only never-scanned hosts

	// Fetch active/running scans first so we can use it for dashboard refetch rate
	const { data: activeScansData } = useQuery({
		queryKey: ["compliance-active-scans"],
		queryFn: () => complianceAPI.getActiveScans().then((res) => res.data),
		staleTime: 30 * 1000,
		refetchInterval: (query) => {
			const active = query.state?.data?.activeScans?.length > 0;
			return active ? 30000 : 120000; // 30s when scans running, 2 min when idle
		},
	});

	const hasActiveScans = (activeScansData?.activeScans?.length ?? 0) > 0;

	const {
		data: dashboard,
		isLoading,
		error,
	} = useQuery({
		queryKey: ["compliance-dashboard"],
		queryFn: () => complianceAPI.getDashboard().then((res) => res.data),
		staleTime: 60 * 1000, // 60s - avoid refetch on tab switch
		refetchInterval: hasActiveScans ? 30000 : 120000, // 30s when scans running, 2 min when idle
	});

	// Fetch all hosts for bulk scan selection
	const { data: hostsData } = useQuery({
		queryKey: ["hosts-list"],
		queryFn: () => adminHostsAPI.list().then((res) => res.data),
		staleTime: 60000,
		select: (data) => ({ hosts: data.data || [] }),
	});

	// Track active scans and notify when they complete
	useEffect(() => {
		const activeScans = activeScansData?.activeScans || [];
		const currentIds = new Set(activeScans.map((s) => s.id));
		const activeHostIds = new Set(activeScans.map((s) => s.hostId));

		// Find scans that were active before but are now gone (completed)
		for (const prevId of prevActiveScanIds.current) {
			if (!currentIds.has(prevId)) {
				// A scan completed - refresh dashboard data
				queryClient.invalidateQueries(["compliance-dashboard"]);
				toast.success("Compliance scan completed");
				break; // Only show one notification per batch
			}
		}

		// Remove pending scans that now appear as real active scans
		setPendingScans((prev) => prev.filter((p) => !activeHostIds.has(p.hostId)));

		prevActiveScanIds.current = currentIds;
	}, [activeScansData, queryClient, toast]);

	// Check if pending scans have completed (scan records appear in dashboard)
	useEffect(() => {
		if (pendingScans.length === 0 || !dashboard?.recent_scans) return;

		const recentScans = dashboard.recent_scans || [];
		const completedPending = [];

		for (const pending of pendingScans) {
			const pendingStart = new Date(pending.startedAt).getTime();
			const hasCompletedScan = recentScans.some((scan) => {
				const scanComplete = new Date(scan.completed_at).getTime();
				return scan.host_id === pending.hostId && scanComplete > pendingStart;
			});
			if (hasCompletedScan) {
				completedPending.push(pending.hostId);
			}
		}

		if (completedPending.length > 0) {
			setPendingScans((prev) =>
				prev.filter((p) => !completedPending.includes(p.hostId)),
			);
			if (completedPending.length === 1) {
				const completed = pendingScans.find(
					(p) => p.hostId === completedPending[0],
				);
				toast.success(`Scan completed for ${completed?.hostName || "host"}`);
			} else {
				toast.success(`${completedPending.length} scans completed`);
			}
		}
	}, [dashboard?.recent_scans, pendingScans, toast]);

	// Clear stale pending scans after 60 seconds
	useEffect(() => {
		if (pendingScans.length === 0) return;

		const interval = setInterval(() => {
			const now = Date.now();
			setPendingScans((prev) =>
				prev.filter((p) => {
					const age = now - new Date(p.startedAt).getTime();
					return age < 60000; // Remove after 60 seconds
				}),
			);
		}, 5000); // Check every 5 seconds

		return () => clearInterval(interval);
	}, [pendingScans.length]);

	// Bulk scan mutation
	const bulkScanMutation = useMutation({
		mutationFn: (data) =>
			complianceAPI.triggerBulkScan(data.hostIds, data.options),
		onSuccess: (response) => {
			setBulkScanResult(response.data);
			queryClient.invalidateQueries(["compliance-active-scans"]);
			const { success, failed } = response.data.summary || {};

			// Track triggered hosts as pending scans for immediate UI feedback
			if (response.data.triggered?.length > 0) {
				const newPending = response.data.triggered.map((t) => ({
					id: `pending-${t.hostId}`,
					hostId: t.hostId,
					hostName: t.hostName,
					profileType: response.data.profile_type,
					startedAt: new Date().toISOString(),
					isPending: true,
					connected: true,
				}));
				setPendingScans((prev) => [...prev, ...newPending]);
			}

			if (failed === 0) {
				toast.success(`Started ${success} compliance scan(s)`);
				// Auto-close modal after 3 seconds if all succeeded
				setTimeout(() => {
					setShowBulkScanModal(false);
					setBulkScanResult(null);
					setSelectedHosts([]);
				}, 3000);
			} else {
				toast.warning(`Started ${success} scan(s), ${failed} failed`);
			}
		},
		onError: (error) => {
			const errorMsg = error.response?.data?.error || error.message;
			toast.error(`Bulk scan failed: ${errorMsg}`);
		},
	});

	// Single-host scan trigger (for RUN column in hosts table)
	const triggerSingleScanMutation = useMutation({
		mutationFn: ({ hostId }) =>
			complianceAPI.triggerScan(hostId, { profile_type: "all" }),
		onSuccess: (_, { hostId, hostName }) => {
			queryClient.invalidateQueries(["compliance-active-scans"]);
			setPendingScans((prev) => [
				...prev,
				{
					id: `pending-${hostId}`,
					hostId,
					hostName: hostName || "Host",
					profileType: "all",
					startedAt: new Date().toISOString(),
					isPending: true,
					connected: true,
				},
			]);
			toast.success(`Scan started for ${hostName || "host"}`);
		},
		onError: (error, { hostName }) => {
			const errorMsg = error.response?.data?.error || error.message;
			toast.error(`Scan failed for ${hostName || "host"}: ${errorMsg}`);
		},
	});

	const cancelScanMutation = useMutation({
		mutationFn: ({ hostId }) => complianceAPI.cancelScan(hostId),
		onSuccess: (_, { hostName }) => {
			toast.success(`Cancel request sent for ${hostName || "host"}`);
			queryClient.invalidateQueries(["compliance-active-scans"]);
		},
		onError: (error, { hostName }) => {
			const errorMsg = error.response?.data?.error || error.message;
			toast.error(
				`Failed to cancel scan for ${hostName || "host"}: ${errorMsg}`,
			);
		},
	});

	if (isLoading) {
		return (
			<div className="flex items-center justify-center h-64">
				<div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-500"></div>
			</div>
		);
	}

	if (error) {
		return (
			<div className="p-4 bg-red-900/50 border border-red-700 rounded-lg">
				<p className="text-red-200">Failed to load compliance dashboard</p>
			</div>
		);
	}

	const {
		summary,
		recent_scans,
		hosts_with_latest_scan,
		worst_hosts,
		top_failing_rules,
		top_warning_rules,
		_profile_distribution,
		_severity_breakdown,
		severity_by_profile_type,
		docker_bench_by_section,
		scan_age_distribution,
		profile_type_stats,
	} = dashboard || {};

	const allHostsTableRows = hosts_with_latest_scan || [];
	const hostsTableRows =
		tableFilter === "never-scanned"
			? allHostsTableRows.filter((row) => row.last_scan_date == null)
			: allHostsTableRows.filter((row) => row.compliance_enabled);

	// Combine real active scans with pending scans for display
	const realActiveScans = activeScansData?.activeScans || [];
	const activeScans = [...pendingScans, ...realActiveScans];

	// Get stats for the selected profile type
	const openscapStats = profile_type_stats?.find((p) => p.type === "openscap");
	const dockerBenchStats = profile_type_stats?.find(
		(p) => p.type === "docker-bench",
	);

	// Calculate filtered summary based on selected tab
	const getFilteredSummary = () => {
		if (profileTypeFilter === "all") {
			return summary;
		}
		const stats =
			profileTypeFilter === "openscap" ? openscapStats : dockerBenchStats;
		if (!stats) return null;

		// Calculate pass rate
		const passRate =
			stats.total_rules > 0
				? (stats.total_passed / stats.total_rules) * 100
				: 0;

		return {
			total_hosts: stats.hosts_scanned || 0,
			average_score: stats.average_score || 0,
			total_rules: stats.total_rules || 0,
			total_passed_rules: stats.total_passed || 0,
			total_failed_rules: stats.total_failed || 0,
			total_warnings: stats.total_warnings || 0,
			pass_rate: passRate,
		};
	};
	const filteredSummary = getFilteredSummary();

	// Filter data by profile type
	const filteredScans =
		recent_scans?.filter((scan) => {
			if (profileTypeFilter === "all") return true;
			return scan.compliance_profiles?.type === profileTypeFilter;
		}) || [];

	const _filteredWorstHosts =
		worst_hosts?.filter((host) => {
			if (profileTypeFilter === "all") return true;
			return host.compliance_profiles?.type === profileTypeFilter;
		}) || [];

	const _filteredTopFailingRules =
		top_failing_rules?.filter((rule) => {
			if (profileTypeFilter === "all") return true;
			return rule.profile_type === profileTypeFilter;
		}) || [];

	const _filteredTopWarningRules =
		top_warning_rules?.filter((rule) => {
			if (profileTypeFilter === "all") return true;
			return rule.profile_type === profileTypeFilter;
		}) || [];

	// Get display name for current filter
	const getFilterDisplayName = () => {
		if (profileTypeFilter === "openscap") return "OpenSCAP";
		if (profileTypeFilter === "docker-bench") return "Docker Bench";
		return "All Scans";
	};

	const allHosts = hostsData?.hosts || [];

	const handleToggleHost = (hostId) => {
		setSelectedHosts((prev) =>
			prev.includes(hostId)
				? prev.filter((id) => id !== hostId)
				: [...prev, hostId],
		);
	};

	const handleSelectAll = () => {
		if (selectedHosts.length === allHosts.length) {
			setSelectedHosts([]);
		} else {
			setSelectedHosts(allHosts.map((h) => h.id));
		}
	};

	const handleBulkScan = () => {
		if (selectedHosts.length === 0) return;
		bulkScanMutation.mutate({
			hostIds: selectedHosts,
			options: bulkScanOptions,
		});
	};

	const _openBulkScanModal = () => {
		setBulkScanResult(null);
		setShowBulkScanModal(true);
	};

	return (
		<div className="space-y-6">
			{/* Page Header */}
			<div className="mb-6">
				<h1 className="text-2xl font-semibold text-secondary-900 dark:text-white">
					Security Compliance
				</h1>
				<p className="text-sm text-secondary-600 dark:text-secondary-400 mt-1">
					Monitor and manage compliance across your hosts
				</p>
			</div>

			{/* Top: 5 host status cards - same layout/style as Hosts page */}
			<div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-3 sm:gap-4 mb-6">
				<div className="card p-4 cursor-default text-left w-full">
					<div className="flex items-center">
						<Server className="h-5 w-5 text-primary-600 mr-2" />
						<div>
							<p className="text-sm text-secondary-500 dark:text-white">
								Total hosts
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{(summary?.total_hosts ?? 0) + (summary?.unscanned ?? 0) || 0}
							</p>
						</div>
					</div>
				</div>
				<div className="card p-4 cursor-default text-left w-full">
					<div className="flex items-center">
						<ShieldCheck className="h-5 w-5 text-green-600 mr-2" />
						<div>
							<p className="text-sm text-secondary-500 dark:text-white">
								Compliant
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{summary?.hosts_compliant ?? 0}
							</p>
						</div>
					</div>
				</div>
				<div className="card p-4 cursor-default text-left w-full">
					<div className="flex items-center">
						<AlertTriangle className="h-5 w-5 text-yellow-600 mr-2" />
						<div>
							<p className="text-sm text-secondary-500 dark:text-white">
								Warning
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{summary?.hosts_warning ?? 0}
							</p>
						</div>
					</div>
				</div>
				<div className="card p-4 cursor-default text-left w-full">
					<div className="flex items-center">
						<ShieldAlert className="h-5 w-5 text-red-600 mr-2" />
						<div>
							<p className="text-sm text-secondary-500 dark:text-white">
								Critical
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{summary?.hosts_critical ?? 0}
							</p>
						</div>
					</div>
				</div>
				<button
					type="button"
					onClick={() => {
						setTableFilter((prev) =>
							prev === "never-scanned" ? null : "never-scanned",
						);
						setActiveTab("hosts");
					}}
					className={`card p-4 text-left w-full transition-shadow duration-200 ${
						tableFilter === "never-scanned"
							? "ring-2 ring-primary-500 dark:ring-primary-400 bg-primary-50 dark:bg-primary-900/20"
							: "cursor-pointer hover:shadow-card-hover dark:hover:shadow-card-hover-dark"
					}`}
				>
					<div className="flex items-center">
						<ShieldOff className="h-5 w-5 text-secondary-600 mr-2" />
						<div>
							<p className="text-sm text-secondary-500 dark:text-white">
								Never scanned
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{summary?.unscanned ?? 0}
							</p>
							{tableFilter === "never-scanned" && (
								<p className="text-xs text-primary-600 dark:text-primary-400 mt-1">
									Filtered in Hosts tab
								</p>
							)}
						</div>
					</div>
				</button>
			</div>

			{/* Tab Navigation - same pattern as Docker page */}
			<div className="border-b border-secondary-200 dark:border-secondary-600">
				<nav className="-mb-px flex space-x-8 px-4" aria-label="Tabs">
					{COMPLIANCE_TABS.map((tab) => {
						const Icon = tab.icon;
						return (
							<button
								key={tab.id}
								type="button"
								onClick={() => setActiveTab(tab.id)}
								className={`${
									activeTab === tab.id
										? "border-primary-500 text-primary-600 dark:text-primary-400"
										: "border-transparent text-secondary-500 hover:text-secondary-700 hover:border-secondary-300 dark:text-secondary-400 dark:hover:text-secondary-300"
								} whitespace-nowrap py-4 px-1 border-b-2 font-medium text-sm flex items-center`}
							>
								<Icon className="h-4 w-4 mr-2" />
								{tab.label}
							</button>
						);
					})}
				</nav>
			</div>

			{/* ==================== HOSTS TAB ==================== */}
			{activeTab === "hosts" && (
				<>
					{/* Hosts table - full width; last activity per row */}
					<div className="card p-4 md:p-6">
						{tableFilter === "never-scanned" && (
							<p className="text-sm text-primary-600 dark:text-primary-400 mb-4">
								Showing never-scanned hosts only. Click the Never scanned card
								again to clear.
							</p>
						)}
						<div className="overflow-x-auto">
							<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-600">
								<thead className="bg-secondary-50 dark:bg-secondary-700">
									<tr>
										<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 whitespace-nowrap w-16">
											Run
										</th>
										<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300">
											Host name
										</th>
										<th
											className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 w-12"
											title="Compliance status"
										>
											Status
										</th>
										<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300">
											Last activity
										</th>
										<th className="px-4 py-2 text-right text-xs font-medium text-secondary-500 dark:text-secondary-300">
											Passed
										</th>
										<th className="px-4 py-2 text-right text-xs font-medium text-secondary-500 dark:text-secondary-300">
											Failed
										</th>
										<th className="px-4 py-2 text-right text-xs font-medium text-secondary-500 dark:text-secondary-300">
											Skipped
										</th>
										<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300">
											Scanner status
										</th>
										<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300">
											Mode
										</th>
										<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300">
											Scanners
										</th>
									</tr>
								</thead>
								<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-600 text-sm">
									{hostsTableRows.length === 0 ? (
										<tr>
											<td
												colSpan={10}
												className="px-4 py-8 text-center text-secondary-500 dark:text-secondary-400"
											>
												No hosts
											</td>
										</tr>
									) : (
										hostsTableRows.map((row) => {
											const active_scan = activeScans.find(
												(s) => s.hostId === row.host_id,
											);
											const is_scanning =
												!!active_scan ||
												(triggerSingleScanMutation.isPending &&
													triggerSingleScanMutation.variables?.hostId ===
														row.host_id);
											const is_cancelling =
												cancelScanMutation.isPending &&
												cancelScanMutation.variables?.hostId === row.host_id;
											return (
												<tr
													key={row.host_id}
													className={
														is_scanning
															? "bg-blue-950/30 dark:bg-blue-950/30 hover:bg-blue-950/40 dark:hover:bg-blue-950/40"
															: "hover:bg-secondary-50 dark:hover:bg-secondary-700"
													}
												>
													<td className="px-4 py-2 whitespace-nowrap">
														{is_scanning ? (
															<button
																type="button"
																onClick={() =>
																	cancelScanMutation.mutate({
																		hostId: row.host_id,
																		hostName:
																			row.friendly_name ||
																			row.hostname ||
																			"Host",
																	})
																}
																disabled={is_cancelling}
																className="inline-flex items-center justify-center w-6 h-6 border border-transparent rounded text-white bg-red-600 hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-500 transition-colors duration-200 disabled:opacity-50 disabled:cursor-not-allowed"
																title={
																	is_cancelling
																		? "Cancelling..."
																		: "Cancel running scan"
																}
															>
																{is_cancelling ? (
																	<RefreshCw className="h-3 w-3 animate-spin" />
																) : (
																	<StopCircle className="h-3.5 w-3.5" />
																)}
															</button>
														) : (
															<button
																type="button"
																onClick={() =>
																	triggerSingleScanMutation.mutate({
																		hostId: row.host_id,
																		hostName:
																			row.friendly_name ||
																			row.hostname ||
																			"Host",
																	})
																}
																disabled={triggerSingleScanMutation.isPending}
																className="inline-flex items-center justify-center w-6 h-6 border border-transparent rounded text-white bg-green-600 hover:bg-green-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-green-500 transition-colors duration-200 disabled:opacity-50 disabled:cursor-not-allowed"
																title="Run compliance scan"
															>
																<Play className="h-3 w-3" />
															</button>
														)}
													</td>
													<td className="px-4 py-2 whitespace-nowrap">
														<Link
															to={`/compliance/hosts/${row.host_id}`}
															className="text-secondary-900 dark:text-white hover:text-primary-600 dark:hover:text-primary-400 hover:underline font-medium"
														>
															{row.friendly_name || row.hostname || "—"}
														</Link>
													</td>
													<td className="px-4 py-2 whitespace-nowrap">
														{row.score != null ? (
															Number(row.score) >= 80 ? (
																<ShieldCheck
																	className="h-5 w-5 text-green-600 dark:text-green-400"
																	title="Compliant"
																/>
															) : Number(row.score) >= 60 ? (
																<AlertTriangle
																	className="h-5 w-5 text-yellow-600 dark:text-yellow-400"
																	title="Warning"
																/>
															) : (
																<ShieldAlert
																	className="h-5 w-5 text-red-600 dark:text-red-400"
																	title="Critical"
																/>
															)
														) : (
															<ShieldOff
																className="h-5 w-5 text-secondary-400"
																title="Not scanned"
															/>
														)}
													</td>
													<td
														className="px-4 py-2 whitespace-nowrap text-secondary-700 dark:text-secondary-300"
														title={
															is_scanning
																? `Scan running${active_scan?.startedAt ? ` · started ${formatDistanceToNow(new Date(active_scan.startedAt), { addSuffix: true })}` : ""}`
																: row.last_scan_date
																	? `${row.last_activity_title || "Scan"} · ${formatDistanceToNow(new Date(row.last_scan_date), { addSuffix: true })}`
																	: undefined
														}
													>
														{is_scanning ? (
															<span className="inline-flex items-center gap-1.5 text-blue-400">
																<RefreshCw className="h-3 w-3 animate-spin" />
																<span className="text-xs font-medium">
																	{active_scan?.isPending
																		? "Starting..."
																		: active_scan?.profileType ===
																				"docker-bench"
																			? "Docker Bench"
																			: active_scan?.profileType === "openscap"
																				? "OpenSCAP"
																				: "Scanning..."}
																</span>
															</span>
														) : (
															row.last_activity_title ||
															(row.last_scan_date ? "Scan" : "—")
														)}
													</td>
													<td className="px-4 py-2 text-right whitespace-nowrap">
														{row.passed != null ? (
															<button
																type="button"
																onClick={() => {
																	setScanResultsFilters({
																		status: "pass",
																		host_id: row.host_id,
																	});
																	setActiveTab("scan-results");
																}}
																className="text-green-600 dark:text-green-400 hover:underline font-medium tabular-nums"
																title="View passing rules for this host"
															>
																{row.passed}
															</button>
														) : (
															"—"
														)}
													</td>
													<td className="px-4 py-2 text-right whitespace-nowrap">
														{row.failed != null ? (
															<button
																type="button"
																onClick={() => {
																	setScanResultsFilters({
																		status: "fail",
																		host_id: row.host_id,
																	});
																	setActiveTab("scan-results");
																}}
																className="text-red-600 dark:text-red-400 hover:underline font-medium tabular-nums"
																title="View failing rules for this host"
															>
																{row.failed}
															</button>
														) : (
															"—"
														)}
													</td>
													<td className="px-4 py-2 text-right whitespace-nowrap">
														{row.skipped != null ? (
															<button
																type="button"
																onClick={() => {
																	setScanResultsFilters({
																		status: "skipped",
																		host_id: row.host_id,
																	});
																	setActiveTab("scan-results");
																}}
																className="text-secondary-600 dark:text-secondary-400 hover:underline font-medium tabular-nums"
																title="View skipped/N/A rules for this host"
															>
																{row.skipped}
															</button>
														) : (
															"—"
														)}
													</td>
													<td className="px-4 py-2 whitespace-nowrap">
														<span
															className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${
																row.scanner_status === "Scanned"
																	? "bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400"
																	: row.scanner_status === "Enabled"
																		? "bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400"
																		: "bg-secondary-100 text-secondary-700 dark:bg-secondary-700 dark:text-secondary-400"
															}`}
														>
															{row.scanner_status}
														</span>
													</td>
													<td className="px-4 py-2 whitespace-nowrap">
														{row.compliance_mode === "disabled" ? (
															<span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-secondary-100 text-secondary-600 dark:bg-secondary-700 dark:text-secondary-400">
																Disabled
															</span>
														) : (
															<span
																className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${
																	row.compliance_mode === "on-demand"
																		? "bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-400"
																		: "bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400"
																}`}
															>
																{row.compliance_mode === "on-demand"
																	? "On-demand"
																	: "Scheduled"}
															</span>
														)}
													</td>
													<td className="px-4 py-2 whitespace-nowrap text-secondary-700 dark:text-secondary-300">
														{row.compliance_enabled && row.docker_enabled
															? "OpenSCAP, Docker"
															: row.compliance_enabled
																? "OpenSCAP"
																: row.docker_enabled
																	? "Docker"
																	: "—"}
													</td>
												</tr>
											);
										})
									)}
								</tbody>
							</table>
						</div>
					</div>
				</>
			)}

			{/* Bulk Scan Modal */}
			{showBulkScanModal && (
				<div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
					<div className="bg-secondary-800 rounded-lg border border-secondary-700 w-full max-w-2xl max-h-[80vh] overflow-hidden">
						{/* Modal Header */}
						<div className="flex items-center justify-between p-4 border-b border-secondary-700">
							<h2 className="text-lg font-semibold text-white">
								Bulk Compliance Scan
							</h2>
							<button
								onClick={() => {
									setShowBulkScanModal(false);
									setBulkScanResult(null);
								}}
								className="text-secondary-400 hover:text-white"
							>
								<X className="h-5 w-5" />
							</button>
						</div>

						{/* Modal Body */}
						<div className="p-4 space-y-4 max-h-[60vh] overflow-y-auto">
							{/* Scan Options */}
							<div className="space-y-3">
								<h3 className="text-sm font-medium text-secondary-300">
									Scan Options
								</h3>
								<div className="flex flex-wrap gap-4">
									<div className="flex-1 min-w-[200px]">
										<label className="block text-xs text-secondary-400 mb-1">
											Profile Type
										</label>
										<select
											value={bulkScanOptions.profileType}
											onChange={(e) =>
												setBulkScanOptions((prev) => ({
													...prev,
													profileType: e.target.value,
												}))
											}
											className="w-full px-3 py-2 bg-secondary-700 border border-secondary-600 rounded-lg text-white text-sm"
										>
											<option value="all">All Profiles</option>
											<option value="openscap">OpenSCAP Only</option>
											<option value="docker-bench">Docker Bench Only</option>
										</select>
									</div>
									<div className="flex items-center gap-2">
										<input
											type="checkbox"
											id="enableRemediation"
											checked={bulkScanOptions.enableRemediation}
											onChange={(e) =>
												setBulkScanOptions((prev) => ({
													...prev,
													enableRemediation: e.target.checked,
												}))
											}
											className="w-4 h-4 rounded bg-secondary-700 border-secondary-600"
										/>
										<label
											htmlFor="enableRemediation"
											className="text-sm text-secondary-300"
										>
											Enable Remediation
										</label>
									</div>
								</div>
							</div>

							{/* Host Selection */}
							<div className="space-y-3">
								<div className="flex items-center justify-between">
									<h3 className="text-sm font-medium text-secondary-300">
										Select Hosts ({selectedHosts.length} of {allHosts.length})
									</h3>
									<button
										onClick={handleSelectAll}
										className="text-xs text-primary-400 hover:text-primary-300"
									>
										{selectedHosts.length === allHosts.length
											? "Deselect All"
											: "Select All"}
									</button>
								</div>
								<div className="grid grid-cols-1 md:grid-cols-2 gap-2 max-h-[200px] overflow-y-auto">
									{allHosts.map((host) => (
										<label
											key={host.id}
											className={`flex items-center gap-3 p-2 rounded-lg cursor-pointer transition-colors ${
												selectedHosts.includes(host.id)
													? "bg-primary-900/30 border border-primary-700"
													: "bg-secondary-700/50 border border-secondary-600 hover:border-secondary-500"
											}`}
										>
											<input
												type="checkbox"
												checked={selectedHosts.includes(host.id)}
												onChange={() => handleToggleHost(host.id)}
												className="w-4 h-4 rounded bg-secondary-700 border-secondary-600"
											/>
											<div className="flex-1 min-w-0">
												<p className="text-sm font-medium text-white truncate">
													{host.friendly_name || host.hostname}
												</p>
												<p className="text-xs text-secondary-400 truncate">
													{host.hostname}
												</p>
											</div>
											<div
												className={`w-2 h-2 rounded-full ${host.status === "online" ? "bg-green-500" : "bg-red-500"}`}
											/>
										</label>
									))}
								</div>
							</div>

							{/* Results */}
							{bulkScanResult && (
								<div
									className={`p-3 rounded-lg ${bulkScanResult.summary?.failed > 0 ? "bg-yellow-900/30 border border-yellow-700" : "bg-green-900/30 border border-green-700"}`}
								>
									<p
										className={`text-sm font-medium ${bulkScanResult.summary?.failed > 0 ? "text-yellow-300" : "text-green-300"}`}
									>
										{bulkScanResult.message}
									</p>
									{bulkScanResult.failed?.length > 0 && (
										<div className="mt-2 text-xs text-yellow-400">
											<p>Failed hosts:</p>
											<ul className="list-disc list-inside">
												{bulkScanResult.failed.map((f, i) => (
													<li key={`failed-${i}-${f.hostName || ""}`}>
														{f.hostName}: {f.error}
													</li>
												))}
											</ul>
										</div>
									)}
								</div>
							)}
						</div>

						{/* Modal Footer */}
						<div className="flex items-center justify-end gap-3 p-4 border-t border-secondary-700">
							<button
								onClick={() => {
									setShowBulkScanModal(false);
									setBulkScanResult(null);
								}}
								className="px-4 py-2 text-secondary-300 hover:text-white transition-colors"
							>
								Cancel
							</button>
							<button
								onClick={handleBulkScan}
								disabled={
									selectedHosts.length === 0 || bulkScanMutation.isPending
								}
								className="flex items-center gap-2 px-4 py-2 bg-primary-600 hover:bg-primary-700 disabled:bg-secondary-600 disabled:cursor-not-allowed text-white rounded-lg font-medium transition-colors"
							>
								{bulkScanMutation.isPending ? (
									<>
										<RefreshCw className="h-4 w-4 animate-spin" />
										Triggering...
									</>
								) : (
									<>
										<Play className="h-4 w-4" />
										Scan {selectedHosts.length} Host
										{selectedHosts.length !== 1 ? "s" : ""}
									</>
								)}
							</button>
						</div>
					</div>
				</div>
			)}

			{/* ==================== OVERVIEW TAB ==================== */}
			{activeTab === "overview" && (
				<>
					{/* Compliance dashboard widgets - same 6 cards as main Dashboard */}
					<div className="grid grid-cols-1 lg:grid-cols-3 gap-4 sm:gap-6 mb-6">
						<FailuresBySeverityDoughnut data={dashboard} />
						<OpenSCAPDistributionDoughnut data={dashboard} />
						<ComplianceProfilesPie data={dashboard} />
						<LastScanAgeBar data={dashboard} />
						<ComplianceTrendLinePlaceholder />
						<HostComplianceStatusBar data={dashboard} />
					</div>

					{/* Active Scans Section - Only show if there are running scans */}
					{activeScans.length > 0 && (
						<div className="card p-4 border-blue-700/50 bg-blue-900/20">
							<div className="flex items-center gap-2 mb-3">
								<RefreshCw className="h-5 w-5 text-blue-400 animate-spin" />
								<h2 className="text-lg font-semibold text-blue-300">
									Scans in Progress ({activeScans.length})
								</h2>
							</div>
							<div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
								{activeScans.map((scan) => (
									<Link
										key={scan.id}
										to={`/compliance/hosts/${scan.hostId}`}
										className={`rounded-lg p-3 border transition-colors ${
											scan.isPending
												? "bg-yellow-900/30 border-yellow-700/50 hover:border-yellow-500"
												: scan.profileType === "docker-bench"
													? "bg-blue-900/20 border-blue-700/50 hover:border-blue-500"
													: "bg-green-900/20 border-green-700/50 hover:border-green-500"
										}`}
									>
										<div className="flex items-center justify-between mb-2">
											<span className="font-medium text-white truncate">
												{scan.hostName}
											</span>
											{scan.isPending ? (
												<Clock
													className="h-4 w-4 text-yellow-400 animate-pulse"
													title="Triggering..."
												/>
											) : scan.connected ? (
												<Wifi
													className="h-4 w-4 text-green-400"
													title="Connected"
												/>
											) : (
												<WifiOff
													className="h-4 w-4 text-red-400"
													title="Disconnected"
												/>
											)}
										</div>
										<div className="flex items-center gap-2 text-sm text-secondary-400">
											<span
												className={`px-2 py-0.5 rounded text-xs ${
													scan.isPending
														? "bg-yellow-900/50 text-yellow-300"
														: scan.profileType === "docker-bench"
															? "bg-blue-900/50 text-blue-300"
															: "bg-green-900/50 text-green-300"
												}`}
											>
												{scan.isPending
													? "Triggering..."
													: scan.profileType === "docker-bench"
														? "Docker Bench"
														: scan.profileType === "openscap"
															? "OpenSCAP"
															: scan.profileType || "Scanning..."}
											</span>
											<span className="text-xs">
												Started{" "}
												{formatDistanceToNow(new Date(scan.startedAt), {
													addSuffix: true,
												})}
											</span>
										</div>
									</Link>
								))}
							</div>
						</div>
					)}

					{/* ==================== SPECIFIC SCAN TYPE STATS (OpenSCAP or Docker Bench tabs) ==================== */}
					{profileTypeFilter !== "all" && filteredSummary && (
						<>
							<div className="flex items-center gap-3 pt-2">
								<div className="flex items-center gap-2">
									{profileTypeFilter === "openscap" ? (
										<Server className="h-5 w-5 text-green-400" />
									) : (
										<Container className="h-5 w-5 text-blue-400" />
									)}
									<h2 className="text-lg font-semibold text-white">
										{getFilterDisplayName()} Statistics
									</h2>
								</div>
								<div className="flex-1 h-px bg-secondary-700" />
							</div>

							{/* Compact Stats Card for specific scan type */}
							<div
								className={`card p-4 sm:p-5 border-2 ${
									profileTypeFilter === "openscap"
										? "border-green-700/50"
										: "border-blue-700/50"
								}`}
							>
								<div className="grid grid-cols-2 md:grid-cols-4 lg:grid-cols-6 gap-6">
									<div className="text-center">
										<p className="text-xs text-secondary-400 uppercase tracking-wide mb-1">
											Hosts
										</p>
										<p className="text-3xl font-bold text-white">
											{filteredSummary.total_hosts || 0}
										</p>
									</div>
									<div className="text-center">
										<p className="text-xs text-secondary-400 uppercase tracking-wide mb-1">
											Avg Score
										</p>
										<p
											className={`text-3xl font-bold ${
												filteredSummary.average_score >= 80
													? "text-green-400"
													: filteredSummary.average_score >= 60
														? "text-yellow-400"
														: "text-red-400"
											}`}
										>
											{filteredSummary.average_score?.toFixed(1) || 0}%
										</p>
									</div>
									<div className="text-center">
										<p className="text-xs text-green-400 uppercase tracking-wide mb-1">
											Passed
										</p>
										<p className="text-3xl font-bold text-green-400">
											{filteredSummary.total_passed_rules?.toLocaleString() ||
												0}
										</p>
									</div>
									{profileTypeFilter === "docker-bench" ? (
										<div className="text-center">
											<p className="text-xs text-yellow-400 uppercase tracking-wide mb-1">
												Warnings
											</p>
											<p className="text-3xl font-bold text-yellow-400">
												{filteredSummary.total_warnings?.toLocaleString() || 0}
											</p>
										</div>
									) : (
										<div className="text-center">
											<p className="text-xs text-red-400 uppercase tracking-wide mb-1">
												Failed
											</p>
											<p className="text-3xl font-bold text-red-400">
												{filteredSummary.total_failed_rules?.toLocaleString() ||
													0}
											</p>
										</div>
									)}
									<div className="text-center">
										<p className="text-xs text-secondary-400 uppercase tracking-wide mb-1">
											Total Rules
										</p>
										<p className="text-3xl font-bold text-white">
											{filteredSummary.total_rules?.toLocaleString() || 0}
										</p>
									</div>
									<div className="text-center">
										<p className="text-xs text-secondary-400 uppercase tracking-wide mb-1">
											Pass Rate
										</p>
										<p className="text-3xl font-bold text-white">
											{filteredSummary.total_rules > 0
												? (
														(filteredSummary.total_passed_rules /
															filteredSummary.total_rules) *
														100
													).toFixed(1)
												: 0}
											%
										</p>
									</div>
								</div>
								{profileTypeFilter === "docker-bench" &&
									filteredSummary.total_rules > 0 && (
										<div className="mt-4 pt-4 border-t border-secondary-700 text-center">
											<span className="text-sm text-secondary-400">
												Warning Rate:{" "}
											</span>
											<span className="text-sm font-bold text-yellow-400">
												{(
													(filteredSummary.total_warnings /
														filteredSummary.total_rules) *
													100
												).toFixed(1)}
												%
											</span>
										</div>
									)}
							</div>
						</>
					)}

					{/* No data message */}
					{!filteredSummary && (
						<div className="card p-8 text-center">
							<p className="text-secondary-400">
								No {getFilterDisplayName()} scan data available
							</p>
						</div>
					)}

					{/* ==================== OpenSCAP Tab Analysis ==================== */}
					{profileTypeFilter === "openscap" && filteredSummary && (
						<>
							<div className="flex items-center gap-3 pt-4">
								<div className="flex items-center gap-2">
									<PieChartIcon className="h-5 w-5 text-green-400" />
									<h2 className="text-lg font-semibold text-white">
										OpenSCAP Analysis
									</h2>
								</div>
								<div className="flex-1 h-px bg-green-700/50" />
							</div>

							<div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
								{/* Rule Pass/Fail Breakdown */}
								{openscapStats && (
									<div className="card p-4 border border-green-700/50">
										<h3 className="text-white font-medium mb-1 flex items-center gap-2">
											<CheckCircle className="h-4 w-4 text-green-400" />
											Rule Results
										</h3>
										<p className="text-xs text-secondary-500 mb-3">
											{(
												openscapStats.total_passed + openscapStats.total_failed
											).toLocaleString()}{" "}
											rules evaluated
										</p>
										<div className="h-40">
											<ResponsiveContainer width="100%" height="100%">
												<PieChart>
													<Pie
														data={[
															{
																name: "Passed",
																value: openscapStats.total_passed || 0,
																color: "#22c55e",
															},
															{
																name: "Failed",
																value: openscapStats.total_failed || 0,
																color: "#ef4444",
															},
														].filter((d) => d.value > 0)}
														cx="50%"
														cy="50%"
														innerRadius={35}
														outerRadius={60}
														dataKey="value"
														label={({ value }) => `${value.toLocaleString()}`}
														labelLine={false}
													>
														<Cell fill="#22c55e" />
														<Cell fill="#ef4444" />
													</Pie>
													<Tooltip
														content={<CustomTooltip type="ruleStatus" />}
													/>
												</PieChart>
											</ResponsiveContainer>
										</div>
										<div className="flex justify-center gap-6 mt-2 text-sm">
											<div className="flex items-center gap-2">
												<div className="w-3 h-3 rounded-full bg-green-500" />
												<span className="text-green-400">
													Passed: {openscapStats.total_passed?.toLocaleString()}
												</span>
											</div>
											<div className="flex items-center gap-2">
												<div className="w-3 h-3 rounded-full bg-red-500" />
												<span className="text-red-400">
													Failed: {openscapStats.total_failed?.toLocaleString()}
												</span>
											</div>
										</div>
									</div>
								)}

								{/* Failures by Severity */}
								{severity_by_profile_type &&
									severity_by_profile_type.filter(
										(s) => s.profile_type === "openscap",
									).length > 0 &&
									(() => {
										const openscapSeverity = severity_by_profile_type.filter(
											(s) => s.profile_type === "openscap",
										);
										const totalFailures = openscapSeverity.reduce(
											(sum, s) => sum + s.count,
											0,
										);
										const severityOrder = ["critical", "high", "medium", "low"];
										const severityColors = {
											critical: "#ef4444",
											high: "#f97316",
											medium: "#eab308",
											low: "#22c55e",
										};
										const chartData = severityOrder
											.map((sev) => {
												const count =
													openscapSeverity.find((s) => s.severity === sev)
														?.count || 0;
												if (count === 0) return null;
												return {
													name: sev.charAt(0).toUpperCase() + sev.slice(1),
													count,
													color: severityColors[sev],
												};
											})
											.filter(Boolean);

										return (
											<div className="card p-4 border border-green-700/50">
												<h3 className="text-white font-medium mb-1 flex items-center gap-2">
													<AlertTriangle className="h-4 w-4 text-red-400" />
													Failures by Severity
												</h3>
												<p className="text-xs text-secondary-500 mb-3">
													{totalFailures.toLocaleString()} total failures
												</p>
												<div className="h-40">
													<ResponsiveContainer width="100%" height="100%">
														<BarChart data={chartData} layout="vertical">
															<XAxis
																type="number"
																stroke="#6b7280"
																fontSize={12}
															/>
															<YAxis
																type="category"
																dataKey="name"
																stroke="#6b7280"
																fontSize={12}
																width={70}
															/>
															<Tooltip
																content={<CustomTooltip type="severity" />}
															/>
															<Bar dataKey="count" radius={[0, 4, 4, 0]}>
																{chartData.map((entry, index) => (
																	<Cell
																		key={`cell-${index}-${entry.color || entry.dataKey || ""}`}
																		fill={entry.color}
																	/>
																))}
															</Bar>
														</BarChart>
													</ResponsiveContainer>
												</div>
											</div>
										);
									})()}

								{/* Score Distribution */}
								{filteredScans &&
									filteredScans.length > 0 &&
									(() => {
										const scoreRanges = [
											{
												range: "90-100%",
												count: filteredScans.filter((s) => s.score >= 90)
													.length,
												color: "#22c55e",
											},
											{
												range: "80-89%",
												count: filteredScans.filter(
													(s) => s.score >= 80 && s.score < 90,
												).length,
												color: "#84cc16",
											},
											{
												range: "70-79%",
												count: filteredScans.filter(
													(s) => s.score >= 70 && s.score < 80,
												).length,
												color: "#eab308",
											},
											{
												range: "60-69%",
												count: filteredScans.filter(
													(s) => s.score >= 60 && s.score < 70,
												).length,
												color: "#f97316",
											},
											{
												range: "<60%",
												count: filteredScans.filter((s) => s.score < 60).length,
												color: "#ef4444",
											},
										];
										return (
											<div className="card p-4 border border-green-700/50">
												<h3 className="text-white font-medium mb-1 flex items-center gap-2">
													<BarChart3 className="h-4 w-4 text-green-400" />
													Score Distribution
												</h3>
												<p className="text-xs text-secondary-500 mb-3">
													{filteredScans.length} scans
												</p>
												<div className="h-40">
													<ResponsiveContainer width="100%" height="100%">
														<BarChart data={scoreRanges} layout="vertical">
															<XAxis
																type="number"
																stroke="#6b7280"
																fontSize={12}
															/>
															<YAxis
																type="category"
																dataKey="range"
																stroke="#6b7280"
																fontSize={12}
																width={60}
															/>
															<Tooltip
																content={<CustomTooltip type="scoreRange" />}
															/>
															<Bar dataKey="count" radius={[0, 4, 4, 0]}>
																{scoreRanges.map((entry, index) => (
																	<Cell
																		key={`cell-${index}-${entry.color || entry.dataKey || ""}`}
																		fill={entry.color}
																	/>
																))}
															</Bar>
														</BarChart>
													</ResponsiveContainer>
												</div>
											</div>
										);
									})()}

								{/* Last Scan Age - OpenSCAP only */}
								{scan_age_distribution &&
									(() => {
										const chartData = [
											{
												name: "Today",
												count: scan_age_distribution.today?.openscap || 0,
												color: "#22c55e",
											},
											{
												name: "This Week",
												count: scan_age_distribution.this_week?.openscap || 0,
												color: "#84cc16",
											},
											{
												name: "This Month",
												count: scan_age_distribution.this_month?.openscap || 0,
												color: "#eab308",
											},
											{
												name: "Older",
												count: scan_age_distribution.older?.openscap || 0,
												color: "#ef4444",
											},
										].filter((d) => d.count > 0);
										const totalScans = chartData.reduce(
											(sum, d) => sum + d.count,
											0,
										);

										if (totalScans === 0) return null;

										return (
											<div className="card p-4 border border-green-700/50">
												<h3 className="text-white font-medium mb-1 flex items-center gap-2">
													<Clock className="h-4 w-4 text-green-400" />
													Scan Freshness
												</h3>
												<p className="text-xs text-secondary-500 mb-3">
													{totalScans} OpenSCAP scans
												</p>
												<div className="h-40">
													<ResponsiveContainer width="100%" height="100%">
														<BarChart data={chartData} layout="vertical">
															<XAxis
																type="number"
																stroke="#6b7280"
																fontSize={12}
															/>
															<YAxis
																type="category"
																dataKey="name"
																stroke="#6b7280"
																fontSize={11}
																width={80}
															/>
															<Tooltip
																content={<CustomTooltip type="scanAge" />}
															/>
															<Bar dataKey="count" radius={[0, 4, 4, 0]}>
																{chartData.map((entry, index) => (
																	<Cell
																		key={`cell-${index}-${entry.color || entry.dataKey || ""}`}
																		fill={entry.color}
																	/>
																))}
															</Bar>
														</BarChart>
													</ResponsiveContainer>
												</div>
											</div>
										);
									})()}
							</div>
						</>
					)}

					{/* ==================== Docker Bench Tab Analysis ==================== */}
					{profileTypeFilter === "docker-bench" && filteredSummary && (
						<>
							<div className="flex items-center gap-3 pt-4">
								<div className="flex items-center gap-2">
									<PieChartIcon className="h-5 w-5 text-blue-400" />
									<h2 className="text-lg font-semibold text-white">
										Docker Bench Analysis
									</h2>
								</div>
								<div className="flex-1 h-px bg-blue-700/50" />
							</div>

							<div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
								{/* Rule Pass/Warn Breakdown */}
								{dockerBenchStats && (
									<div className="card p-4 border border-blue-700/50">
										<h3 className="text-white font-medium mb-1 flex items-center gap-2">
											<CheckCircle className="h-4 w-4 text-blue-400" />
											Rule Results
										</h3>
										<p className="text-xs text-secondary-500 mb-3">
											{(
												dockerBenchStats.total_passed +
												dockerBenchStats.total_warnings
											).toLocaleString()}{" "}
											rules evaluated
										</p>
										<div className="h-40">
											<ResponsiveContainer width="100%" height="100%">
												<PieChart>
													<Pie
														data={[
															{
																name: "Passed",
																value: dockerBenchStats.total_passed || 0,
																color: "#22c55e",
															},
															{
																name: "Warnings",
																value: dockerBenchStats.total_warnings || 0,
																color: "#eab308",
															},
														].filter((d) => d.value > 0)}
														cx="50%"
														cy="50%"
														innerRadius={35}
														outerRadius={60}
														dataKey="value"
														label={({ value }) => `${value.toLocaleString()}`}
														labelLine={false}
													>
														<Cell fill="#22c55e" />
														<Cell fill="#eab308" />
													</Pie>
													<Tooltip
														content={<CustomTooltip type="ruleStatus" />}
													/>
												</PieChart>
											</ResponsiveContainer>
										</div>
										<div className="flex justify-center gap-6 mt-2 text-sm">
											<div className="flex items-center gap-2">
												<div className="w-3 h-3 rounded-full bg-green-500" />
												<span className="text-green-400">
													Passed:{" "}
													{dockerBenchStats.total_passed?.toLocaleString()}
												</span>
											</div>
											<div className="flex items-center gap-2">
												<div className="w-3 h-3 rounded-full bg-yellow-500" />
												<span className="text-yellow-400">
													Warnings:{" "}
													{dockerBenchStats.total_warnings?.toLocaleString()}
												</span>
											</div>
										</div>
									</div>
								)}

								{/* Warnings by Section */}
								{docker_bench_by_section &&
									docker_bench_by_section.length > 0 &&
									(() => {
										const totalWarnings = docker_bench_by_section.reduce(
											(sum, s) => sum + s.count,
											0,
										);
										const sectionColors = {
											"Host Configuration": "#ef4444",
											"Docker Daemon Configuration": "#f97316",
											"Docker Daemon Configuration Files": "#eab308",
											"Container Images and Build File": "#84cc16",
											"Container Runtime": "#22c55e",
											"Docker Security Operations": "#3b82f6",
											"Docker Swarm Configuration": "#8b5cf6",
										};
										const chartData = docker_bench_by_section.map((s) => ({
											name: s.section,
											shortName:
												s.section.length > 20
													? `${s.section.slice(0, 17)}...`
													: s.section,
											count: s.count,
											color: sectionColors[s.section] || "#6b7280",
										}));

										return (
											<div className="card p-4 border border-blue-700/50">
												<h3 className="text-white font-medium mb-1 flex items-center gap-2">
													<Container className="h-4 w-4 text-yellow-400" />
													Warnings by Section
												</h3>
												<p className="text-xs text-secondary-500 mb-3">
													{totalWarnings.toLocaleString()} total warnings
												</p>
												<div className="h-40">
													<ResponsiveContainer width="100%" height="100%">
														<BarChart data={chartData} layout="vertical">
															<XAxis
																type="number"
																stroke="#6b7280"
																fontSize={12}
															/>
															<YAxis
																type="category"
																dataKey="shortName"
																stroke="#6b7280"
																fontSize={10}
																width={85}
															/>
															<Tooltip
																content={({ active, payload }) => {
																	if (
																		!active ||
																		!payload ||
																		payload.length === 0
																	)
																		return null;
																	const data = payload[0].payload;
																	return (
																		<div className="bg-gray-900 border border-gray-700 rounded-lg px-3 py-2 shadow-lg max-w-xs">
																			<p className="text-white font-medium text-sm mb-1">
																				{data.name}
																			</p>
																			<div className="flex items-center gap-2 text-sm">
																				<div
																					className="w-2.5 h-2.5 rounded"
																					style={{
																						backgroundColor: data.color,
																					}}
																				/>
																				<span className="text-gray-300">
																					Warnings:
																				</span>
																				<span className="text-white font-medium">
																					{data.count.toLocaleString()}
																				</span>
																			</div>
																		</div>
																	);
																}}
															/>
															<Bar dataKey="count" radius={[0, 4, 4, 0]}>
																{chartData.map((entry, index) => (
																	<Cell
																		key={`cell-${index}-${entry.color || entry.dataKey || ""}`}
																		fill={entry.color}
																	/>
																))}
															</Bar>
														</BarChart>
													</ResponsiveContainer>
												</div>
											</div>
										);
									})()}

								{/* Score Distribution */}
								{filteredScans &&
									filteredScans.length > 0 &&
									(() => {
										const scoreRanges = [
											{
												range: "90-100%",
												count: filteredScans.filter((s) => s.score >= 90)
													.length,
												color: "#22c55e",
											},
											{
												range: "80-89%",
												count: filteredScans.filter(
													(s) => s.score >= 80 && s.score < 90,
												).length,
												color: "#84cc16",
											},
											{
												range: "70-79%",
												count: filteredScans.filter(
													(s) => s.score >= 70 && s.score < 80,
												).length,
												color: "#eab308",
											},
											{
												range: "60-69%",
												count: filteredScans.filter(
													(s) => s.score >= 60 && s.score < 70,
												).length,
												color: "#f97316",
											},
											{
												range: "<60%",
												count: filteredScans.filter((s) => s.score < 60).length,
												color: "#ef4444",
											},
										];
										return (
											<div className="card p-4 border border-blue-700/50">
												<h3 className="text-white font-medium mb-1 flex items-center gap-2">
													<BarChart3 className="h-4 w-4 text-blue-400" />
													Score Distribution
												</h3>
												<p className="text-xs text-secondary-500 mb-3">
													{filteredScans.length} scans
												</p>
												<div className="h-40">
													<ResponsiveContainer width="100%" height="100%">
														<BarChart data={scoreRanges} layout="vertical">
															<XAxis
																type="number"
																stroke="#6b7280"
																fontSize={12}
															/>
															<YAxis
																type="category"
																dataKey="range"
																stroke="#6b7280"
																fontSize={12}
																width={60}
															/>
															<Tooltip
																content={<CustomTooltip type="scoreRange" />}
															/>
															<Bar dataKey="count" radius={[0, 4, 4, 0]}>
																{scoreRanges.map((entry, index) => (
																	<Cell
																		key={`cell-${index}-${entry.color || entry.dataKey || ""}`}
																		fill={entry.color}
																	/>
																))}
															</Bar>
														</BarChart>
													</ResponsiveContainer>
												</div>
											</div>
										);
									})()}

								{/* Last Scan Age - Docker Bench only */}
								{scan_age_distribution &&
									(() => {
										const chartData = [
											{
												name: "Today",
												count:
													scan_age_distribution.today?.["docker-bench"] || 0,
												color: "#22c55e",
											},
											{
												name: "This Week",
												count:
													scan_age_distribution.this_week?.["docker-bench"] ||
													0,
												color: "#84cc16",
											},
											{
												name: "This Month",
												count:
													scan_age_distribution.this_month?.["docker-bench"] ||
													0,
												color: "#eab308",
											},
											{
												name: "Older",
												count:
													scan_age_distribution.older?.["docker-bench"] || 0,
												color: "#ef4444",
											},
										].filter((d) => d.count > 0);
										const totalScans = chartData.reduce(
											(sum, d) => sum + d.count,
											0,
										);

										if (totalScans === 0) return null;

										return (
											<div className="card p-4 border border-blue-700/50">
												<h3 className="text-white font-medium mb-1 flex items-center gap-2">
													<Clock className="h-4 w-4 text-blue-400" />
													Scan Freshness
												</h3>
												<p className="text-xs text-secondary-500 mb-3">
													{totalScans} Docker Bench scans
												</p>
												<div className="h-40">
													<ResponsiveContainer width="100%" height="100%">
														<BarChart data={chartData} layout="vertical">
															<XAxis
																type="number"
																stroke="#6b7280"
																fontSize={12}
															/>
															<YAxis
																type="category"
																dataKey="name"
																stroke="#6b7280"
																fontSize={11}
																width={80}
															/>
															<Tooltip
																content={<CustomTooltip type="scanAge" />}
															/>
															<Bar dataKey="count" radius={[0, 4, 4, 0]}>
																{chartData.map((entry, index) => (
																	<Cell
																		key={`cell-${index}-${entry.color || entry.dataKey || ""}`}
																		fill={entry.color}
																	/>
																))}
															</Bar>
														</BarChart>
													</ResponsiveContainer>
												</div>
											</div>
										);
									})()}
							</div>
						</>
					)}
				</>
			)}

			{/* ==================== SCAN RESULTS TAB ==================== */}
			{activeTab === "scan-results" && (
				<ScanResultsTab
					profileTypeFilter={profileTypeFilter}
					scannedHosts={hosts_with_latest_scan}
					initialFilters={scanResultsFilters}
				/>
			)}

			{/* ==================== HISTORY TAB ==================== */}
			{activeTab === "history" && (
				<ScanHistoryTab scanned_hosts={hosts_with_latest_scan} />
			)}
		</div>
	);
};

export default Compliance;
