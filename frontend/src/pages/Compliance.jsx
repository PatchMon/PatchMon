import { useState, useEffect, useRef } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import {
	Shield,
	ShieldCheck,
	ShieldAlert,
	ShieldX,
	ShieldOff,
	TrendingUp,
	TrendingDown,
	Clock,
	Server,
	BarChart3,
	PieChart as PieChartIcon,
	CheckCircle,
	XCircle,
	AlertTriangle,
	List,
	RefreshCw,
	Wifi,
	WifiOff,
	Play,
	X,
	Check,
	Container,
} from "lucide-react";
import { PieChart, Pie, Cell, BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, Legend } from "recharts";
import { complianceAPI } from "../utils/complianceApi";
import { adminHostsAPI } from "../utils/api";
import ComplianceScore from "../components/compliance/ComplianceScore";
import { formatDistanceToNow } from "date-fns";
import { useToast } from "../contexts/ToastContext";

const Compliance = () => {
	const queryClient = useQueryClient();
	const toast = useToast();
	const [showBulkScanModal, setShowBulkScanModal] = useState(false);
	const [selectedHosts, setSelectedHosts] = useState([]);
	const [bulkScanOptions, setBulkScanOptions] = useState({
		profileType: "all",
		enableRemediation: false,
	});
	const [bulkScanResult, setBulkScanResult] = useState(null);
	const prevActiveScanIds = useRef(new Set());
	const [profileTypeFilter, setProfileTypeFilter] = useState("all"); // "all", "openscap", "docker-bench"

	const { data: dashboard, isLoading, error } = useQuery({
		queryKey: ["compliance-dashboard"],
		queryFn: () => complianceAPI.getDashboard().then((res) => res.data),
		refetchInterval: 30000,
	});

	// Fetch active/running scans
	const { data: activeScansData } = useQuery({
		queryKey: ["compliance-active-scans"],
		queryFn: () => complianceAPI.getActiveScans().then((res) => res.data),
		refetchInterval: 5000, // Refresh every 5 seconds for active scans
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

		// Find scans that were active before but are now gone (completed)
		for (const prevId of prevActiveScanIds.current) {
			if (!currentIds.has(prevId)) {
				// A scan completed - refresh dashboard data
				queryClient.invalidateQueries(["compliance-dashboard"]);
				toast.success("Compliance scan completed");
				break; // Only show one notification per batch
			}
		}

		prevActiveScanIds.current = currentIds;
	}, [activeScansData, queryClient, toast]);

	// Bulk scan mutation
	const bulkScanMutation = useMutation({
		mutationFn: (data) => complianceAPI.triggerBulkScan(data.hostIds, data.options),
		onSuccess: (response) => {
			setBulkScanResult(response.data);
			queryClient.invalidateQueries(["compliance-active-scans"]);
			const { success, failed } = response.data.summary || {};
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
			toast.error(`Bulk scan failed: ${error.message}`);
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

	const { summary, recent_scans, worst_hosts, top_failing_rules, top_warning_rules, profile_distribution, severity_breakdown, profile_type_stats } = dashboard || {};
	const activeScans = activeScansData?.activeScans || [];

	// Get stats for the selected profile type
	const openscapStats = profile_type_stats?.find(p => p.type === "openscap");
	const dockerBenchStats = profile_type_stats?.find(p => p.type === "docker-bench");

	// Calculate filtered summary based on selected tab
	const getFilteredSummary = () => {
		if (profileTypeFilter === "all") {
			return summary;
		}
		const stats = profileTypeFilter === "openscap" ? openscapStats : dockerBenchStats;
		if (!stats) return null;

		// Calculate pass rate
		const passRate = stats.total_rules > 0
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
	const filteredScans = recent_scans?.filter(scan => {
		if (profileTypeFilter === "all") return true;
		return scan.compliance_profiles?.type === profileTypeFilter;
	}) || [];

	const filteredWorstHosts = worst_hosts?.filter(host => {
		if (profileTypeFilter === "all") return true;
		return host.compliance_profiles?.type === profileTypeFilter;
	}) || [];

	const filteredTopFailingRules = top_failing_rules?.filter(rule => {
		if (profileTypeFilter === "all") return true;
		return rule.profile_type === profileTypeFilter;
	}) || [];

	const filteredTopWarningRules = top_warning_rules?.filter(rule => {
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
			prev.includes(hostId) ? prev.filter((id) => id !== hostId) : [...prev, hostId]
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

	const openBulkScanModal = () => {
		setBulkScanResult(null);
		setShowBulkScanModal(true);
	};

	return (
		<div className="space-y-6">
			{/* Header */}
			<div className="flex items-center justify-between">
				<div className="flex items-center gap-3">
					<Shield className="h-8 w-8 text-primary-400" />
					<h1 className="text-2xl font-bold text-white">Security Compliance</h1>
				</div>
				<button
					onClick={openBulkScanModal}
					className="flex items-center gap-2 px-4 py-2 bg-primary-600 hover:bg-primary-700 text-white rounded-lg font-medium transition-colors"
				>
					<Play className="h-4 w-4" />
					Bulk Scan
				</button>
			</div>

			{/* Profile Type Filter Tabs */}
			<div className="flex items-center gap-2 bg-secondary-800 p-1 rounded-lg border border-secondary-700 w-fit">
				<button
					onClick={() => setProfileTypeFilter("all")}
					className={`flex items-center gap-2 px-4 py-2 rounded-md transition-colors ${
						profileTypeFilter === "all"
							? "bg-primary-600 text-white"
							: "text-secondary-400 hover:text-white hover:bg-secondary-700"
					}`}
				>
					<Shield className="h-4 w-4" />
					All Scans
				</button>
				<button
					onClick={() => setProfileTypeFilter("openscap")}
					className={`flex items-center gap-2 px-4 py-2 rounded-md transition-colors ${
						profileTypeFilter === "openscap"
							? "bg-green-600 text-white"
							: "text-secondary-400 hover:text-white hover:bg-secondary-700"
					}`}
				>
					<Server className="h-4 w-4" />
					OpenSCAP
				</button>
				<button
					onClick={() => setProfileTypeFilter("docker-bench")}
					className={`flex items-center gap-2 px-4 py-2 rounded-md transition-colors ${
						profileTypeFilter === "docker-bench"
							? "bg-blue-600 text-white"
							: "text-secondary-400 hover:text-white hover:bg-secondary-700"
					}`}
				>
					<Container className="h-4 w-4" />
					Docker Bench
				</button>
			</div>

			{/* Bulk Scan Modal */}
			{showBulkScanModal && (
				<div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
					<div className="bg-secondary-800 rounded-lg border border-secondary-700 w-full max-w-2xl max-h-[80vh] overflow-hidden">
						{/* Modal Header */}
						<div className="flex items-center justify-between p-4 border-b border-secondary-700">
							<h2 className="text-lg font-semibold text-white">Bulk Compliance Scan</h2>
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
								<h3 className="text-sm font-medium text-secondary-300">Scan Options</h3>
								<div className="flex flex-wrap gap-4">
									<div className="flex-1 min-w-[200px]">
										<label className="block text-xs text-secondary-400 mb-1">Profile Type</label>
										<select
											value={bulkScanOptions.profileType}
											onChange={(e) => setBulkScanOptions((prev) => ({ ...prev, profileType: e.target.value }))}
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
											onChange={(e) => setBulkScanOptions((prev) => ({ ...prev, enableRemediation: e.target.checked }))}
											className="w-4 h-4 rounded bg-secondary-700 border-secondary-600"
										/>
										<label htmlFor="enableRemediation" className="text-sm text-secondary-300">
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
										{selectedHosts.length === allHosts.length ? "Deselect All" : "Select All"}
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
												<p className="text-xs text-secondary-400 truncate">{host.hostname}</p>
											</div>
											<div className={`w-2 h-2 rounded-full ${host.status === "online" ? "bg-green-500" : "bg-red-500"}`} />
										</label>
									))}
								</div>
							</div>

							{/* Results */}
							{bulkScanResult && (
								<div className={`p-3 rounded-lg ${bulkScanResult.summary?.failed > 0 ? "bg-yellow-900/30 border border-yellow-700" : "bg-green-900/30 border border-green-700"}`}>
									<p className={`text-sm font-medium ${bulkScanResult.summary?.failed > 0 ? "text-yellow-300" : "text-green-300"}`}>
										{bulkScanResult.message}
									</p>
									{bulkScanResult.failed?.length > 0 && (
										<div className="mt-2 text-xs text-yellow-400">
											<p>Failed hosts:</p>
											<ul className="list-disc list-inside">
												{bulkScanResult.failed.map((f, i) => (
													<li key={i}>{f.hostName}: {f.error}</li>
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
								disabled={selectedHosts.length === 0 || bulkScanMutation.isPending}
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
										Scan {selectedHosts.length} Host{selectedHosts.length !== 1 ? "s" : ""}
									</>
								)}
							</button>
						</div>
					</div>
				</div>
			)}

			{/* Active Scans Section - Only show if there are running scans */}
			{activeScans.length > 0 && (
				<div className="bg-blue-900/30 border border-blue-700/50 rounded-lg p-4">
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
								to={`/hosts/${scan.hostId}`}
								className="bg-secondary-800/50 rounded-lg p-3 border border-secondary-700 hover:border-blue-600 transition-colors"
							>
								<div className="flex items-center justify-between mb-2">
									<span className="font-medium text-white truncate">
										{scan.hostName}
									</span>
									{scan.connected ? (
										<Wifi className="h-4 w-4 text-green-400" title="Connected" />
									) : (
										<WifiOff className="h-4 w-4 text-red-400" title="Disconnected" />
									)}
								</div>
								<div className="flex items-center gap-2 text-sm text-secondary-400">
									<span className="px-2 py-0.5 bg-blue-900/50 text-blue-300 rounded text-xs">
										{scan.profileType || "Scanning..."}
									</span>
									<span className="text-xs">
										Started {formatDistanceToNow(new Date(scan.startedAt), { addSuffix: true })}
									</span>
								</div>
							</Link>
						))}
					</div>
				</div>
			)}

			{/* ==================== OVERVIEW SECTION ==================== */}
			{profileTypeFilter === "all" && filteredSummary && (
				<>
					<div className="flex items-center gap-3 pt-2">
						<div className="flex items-center gap-2">
							<BarChart3 className="h-5 w-5 text-primary-400" />
							<h2 className="text-lg font-semibold text-white">Overview</h2>
						</div>
						<div className="flex-1 h-px bg-secondary-700" />
					</div>

					{/* HOST STATUS - Shows unique hosts by their worst score */}
					<div className="bg-secondary-800 rounded-lg border border-secondary-700 p-5">
						<div className="flex items-center gap-2 mb-4">
							<Server className="h-4 w-4 text-primary-400" />
							<h3 className="text-white font-medium">Host Status</h3>
							<span className="text-xs text-secondary-500">(based on worst score per host)</span>
						</div>
						<div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-5 gap-6">
							<div className="text-center">
								<div className="flex items-center justify-center gap-2 mb-2">
									<Server className="h-4 w-4 text-secondary-400" />
									<span className="text-xs text-secondary-400 uppercase tracking-wide">Total</span>
								</div>
								<p className="text-3xl font-bold text-white">{summary?.total_hosts || 0}</p>
								<p className="text-xs text-secondary-500 mt-1">hosts scanned</p>
							</div>
							<div className="text-center">
								<div className="flex items-center justify-center gap-2 mb-2">
									<ShieldCheck className="h-4 w-4 text-green-400" />
									<span className="text-xs text-green-400 uppercase tracking-wide">Compliant</span>
								</div>
								<p className="text-3xl font-bold text-green-400">{summary?.hosts_compliant || 0}</p>
								<p className="text-xs text-secondary-500 mt-1">all scans ≥80%</p>
							</div>
							<div className="text-center">
								<div className="flex items-center justify-center gap-2 mb-2">
									<ShieldAlert className="h-4 w-4 text-yellow-400" />
									<span className="text-xs text-yellow-400 uppercase tracking-wide">Warning</span>
								</div>
								<p className="text-3xl font-bold text-yellow-400">{summary?.hosts_warning || 0}</p>
								<p className="text-xs text-secondary-500 mt-1">worst 60-80%</p>
							</div>
							<div className="text-center">
								<div className="flex items-center justify-center gap-2 mb-2">
									<ShieldX className="h-4 w-4 text-red-400" />
									<span className="text-xs text-red-400 uppercase tracking-wide">Critical</span>
								</div>
								<p className="text-3xl font-bold text-red-400">{summary?.hosts_critical || 0}</p>
								<p className="text-xs text-secondary-500 mt-1">any scan &lt;60%</p>
							</div>
							<div className="text-center">
								<div className="flex items-center justify-center gap-2 mb-2">
									<ShieldOff className="h-4 w-4 text-secondary-500" />
									<span className="text-xs text-secondary-500 uppercase tracking-wide">Unscanned</span>
								</div>
								<p className="text-3xl font-bold text-secondary-500">{summary?.unscanned || 0}</p>
								<p className="text-xs text-secondary-500 mt-1">no data</p>
							</div>
						</div>
					</div>

					{/* SCAN TYPE BREAKDOWN - OpenSCAP vs Docker Bench */}
					<div className="grid grid-cols-1 md:grid-cols-2 gap-4">
						{/* OpenSCAP Summary */}
						<div className="bg-secondary-800 rounded-lg border-2 border-green-700/50 overflow-hidden">
							<div className="px-4 py-3 bg-green-900/20 border-b border-green-700/30 flex items-center justify-between">
								<div className="flex items-center gap-2">
									<Server className="h-4 w-4 text-green-400" />
									<span className="text-green-400 font-bold">OpenSCAP</span>
									<span className="text-secondary-400 text-sm">CIS Benchmark</span>
								</div>
								{openscapStats?.average_score != null && (
									<span className={`text-lg font-bold ${
										openscapStats.average_score >= 80 ? "text-green-400" :
										openscapStats.average_score >= 60 ? "text-yellow-400" : "text-red-400"
									}`}>
										{Math.round(openscapStats.average_score)}%
									</span>
								)}
							</div>
							<div className="p-4">
								{openscapStats ? (
									<>
										<div className="grid grid-cols-4 gap-2 text-center text-sm">
											<div>
												<p className="text-xl font-bold text-white">{openscapStats.hosts_scanned}</p>
												<p className="text-xs text-secondary-400">Hosts</p>
											</div>
											<div>
												<p className="text-xl font-bold text-green-400">{openscapStats.total_passed?.toLocaleString()}</p>
												<p className="text-xs text-green-400">Passed</p>
											</div>
											<div>
												<p className="text-xl font-bold text-red-400">{openscapStats.total_failed?.toLocaleString()}</p>
												<p className="text-xs text-red-400">Failed</p>
											</div>
											<div>
												<p className="text-xl font-bold text-white">{openscapStats.total_rules?.toLocaleString()}</p>
												<p className="text-xs text-secondary-400">Rules</p>
											</div>
										</div>
										<div className="mt-3 h-2 bg-secondary-700 rounded-full overflow-hidden flex">
											<div className="h-full bg-green-500" style={{ width: `${(openscapStats.total_passed / openscapStats.total_rules) * 100}%` }} />
											<div className="h-full bg-red-500" style={{ width: `${(openscapStats.total_failed / openscapStats.total_rules) * 100}%` }} />
										</div>
										<p className="text-xs text-secondary-500 mt-2 text-center">
											{openscapStats.total_rules - openscapStats.total_passed - openscapStats.total_failed} skipped/N/A
										</p>
									</>
								) : (
									<p className="text-secondary-500 text-sm text-center py-4">No OpenSCAP scans yet</p>
								)}
							</div>
						</div>

						{/* Docker Bench Summary */}
						<div className="bg-secondary-800 rounded-lg border-2 border-blue-700/50 overflow-hidden">
							<div className="px-4 py-3 bg-blue-900/20 border-b border-blue-700/30 flex items-center justify-between">
								<div className="flex items-center gap-2">
									<Container className="h-4 w-4 text-blue-400" />
									<span className="text-blue-400 font-bold">Docker Bench</span>
									<span className="text-secondary-400 text-sm">Container Security</span>
								</div>
								{dockerBenchStats?.average_score != null && (
									<span className={`text-lg font-bold ${
										dockerBenchStats.average_score >= 80 ? "text-green-400" :
										dockerBenchStats.average_score >= 60 ? "text-yellow-400" : "text-red-400"
									}`}>
										{Math.round(dockerBenchStats.average_score)}%
									</span>
								)}
							</div>
							<div className="p-4">
								{dockerBenchStats ? (
									<>
										<div className="grid grid-cols-4 gap-2 text-center text-sm">
											<div>
												<p className="text-xl font-bold text-white">{dockerBenchStats.hosts_scanned}</p>
												<p className="text-xs text-secondary-400">Hosts</p>
											</div>
											<div>
												<p className="text-xl font-bold text-green-400">{dockerBenchStats.total_passed?.toLocaleString()}</p>
												<p className="text-xs text-green-400">Passed</p>
											</div>
											<div>
												<p className="text-xl font-bold text-yellow-400">{dockerBenchStats.total_warnings?.toLocaleString()}</p>
												<p className="text-xs text-yellow-400">Warnings</p>
											</div>
											<div>
												<p className="text-xl font-bold text-white">{dockerBenchStats.total_rules?.toLocaleString()}</p>
												<p className="text-xs text-secondary-400">Rules</p>
											</div>
										</div>
										<div className="mt-3 h-2 bg-secondary-700 rounded-full overflow-hidden flex">
											<div className="h-full bg-green-500" style={{ width: `${(dockerBenchStats.total_passed / dockerBenchStats.total_rules) * 100}%` }} />
											<div className="h-full bg-yellow-500" style={{ width: `${(dockerBenchStats.total_warnings / dockerBenchStats.total_rules) * 100}%` }} />
										</div>
										<p className="text-xs text-secondary-500 mt-2 text-center">
											{dockerBenchStats.total_rules - dockerBenchStats.total_passed - dockerBenchStats.total_warnings} skipped/N/A
										</p>
									</>
								) : (
									<p className="text-secondary-500 text-sm text-center py-4">No Docker Bench scans yet</p>
								)}
							</div>
						</div>
					</div>
				</>
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
							<h2 className="text-lg font-semibold text-white">{getFilterDisplayName()} Statistics</h2>
						</div>
						<div className="flex-1 h-px bg-secondary-700" />
					</div>

					{/* Compact Stats Card for specific scan type */}
					<div className={`bg-secondary-800 rounded-lg border-2 p-5 ${
						profileTypeFilter === "openscap" ? "border-green-700/50" : "border-blue-700/50"
					}`}>
						<div className="grid grid-cols-2 md:grid-cols-4 lg:grid-cols-6 gap-6">
							<div className="text-center">
								<p className="text-xs text-secondary-400 uppercase tracking-wide mb-1">Hosts</p>
								<p className="text-3xl font-bold text-white">{filteredSummary.total_hosts || 0}</p>
							</div>
							<div className="text-center">
								<p className="text-xs text-secondary-400 uppercase tracking-wide mb-1">Avg Score</p>
								<p className={`text-3xl font-bold ${
									filteredSummary.average_score >= 80 ? "text-green-400" :
									filteredSummary.average_score >= 60 ? "text-yellow-400" : "text-red-400"
								}`}>{filteredSummary.average_score?.toFixed(1) || 0}%</p>
							</div>
							<div className="text-center">
								<p className="text-xs text-green-400 uppercase tracking-wide mb-1">Passed</p>
								<p className="text-3xl font-bold text-green-400">{filteredSummary.total_passed_rules?.toLocaleString() || 0}</p>
							</div>
							{profileTypeFilter === "docker-bench" ? (
								<div className="text-center">
									<p className="text-xs text-yellow-400 uppercase tracking-wide mb-1">Warnings</p>
									<p className="text-3xl font-bold text-yellow-400">{filteredSummary.total_warnings?.toLocaleString() || 0}</p>
								</div>
							) : (
								<div className="text-center">
									<p className="text-xs text-red-400 uppercase tracking-wide mb-1">Failed</p>
									<p className="text-3xl font-bold text-red-400">{filteredSummary.total_failed_rules?.toLocaleString() || 0}</p>
								</div>
							)}
							<div className="text-center">
								<p className="text-xs text-secondary-400 uppercase tracking-wide mb-1">Total Rules</p>
								<p className="text-3xl font-bold text-white">{filteredSummary.total_rules?.toLocaleString() || 0}</p>
							</div>
							<div className="text-center">
								<p className="text-xs text-secondary-400 uppercase tracking-wide mb-1">Pass Rate</p>
								<p className="text-3xl font-bold text-white">
									{filteredSummary.total_rules > 0
										? ((filteredSummary.total_passed_rules / filteredSummary.total_rules) * 100).toFixed(1)
										: 0}%
								</p>
							</div>
						</div>
						{profileTypeFilter === "docker-bench" && filteredSummary.total_rules > 0 && (
							<div className="mt-4 pt-4 border-t border-secondary-700 text-center">
								<span className="text-sm text-secondary-400">Warning Rate: </span>
								<span className="text-sm font-bold text-yellow-400">
									{((filteredSummary.total_warnings / filteredSummary.total_rules) * 100).toFixed(1)}%
								</span>
							</div>
						)}
					</div>
				</>
			)}

			{/* No data message */}
			{!filteredSummary && (
				<div className="bg-secondary-800 rounded-lg p-8 border border-secondary-700 text-center">
					<p className="text-secondary-400">No {getFilterDisplayName()} scan data available</p>
				</div>
			)}

			{/* ==================== ANALYSIS SECTION ==================== */}
			{profileTypeFilter === "all" && summary && (summary.total_hosts > 0 || summary.unscanned > 0) && (() => {
				// Use HOST-LEVEL counts (based on worst score per host)
				const hostDistribution = [
					{ name: "Compliant Hosts", value: summary.hosts_compliant || 0, color: "#22c55e" },
					{ name: "Warning Hosts", value: summary.hosts_warning || 0, color: "#eab308" },
					{ name: "Critical Hosts", value: summary.hosts_critical || 0, color: "#ef4444" },
					{ name: "Unscanned Hosts", value: summary.unscanned || 0, color: "#6b7280" },
				].filter(d => d.value > 0);

				// OpenSCAP scans for score distribution
				const openscapScans = recent_scans?.filter(s => s.compliance_profiles?.type === "openscap") || [];
				const openscapScoreRanges = [
					{ range: "90-100%", count: openscapScans.filter(s => s.score >= 90).length, color: "#22c55e" },
					{ range: "80-89%", count: openscapScans.filter(s => s.score >= 80 && s.score < 90).length, color: "#84cc16" },
					{ range: "70-79%", count: openscapScans.filter(s => s.score >= 70 && s.score < 80).length, color: "#eab308" },
					{ range: "60-69%", count: openscapScans.filter(s => s.score >= 60 && s.score < 70).length, color: "#f97316" },
					{ range: "<60%", count: openscapScans.filter(s => s.score < 60).length, color: "#ef4444" },
				];

				// Docker Bench scans for score distribution
				const dockerScans = recent_scans?.filter(s => s.compliance_profiles?.type === "docker-bench") || [];
				const dockerScoreRanges = [
					{ range: "90-100%", count: dockerScans.filter(s => s.score >= 90).length, color: "#22c55e" },
					{ range: "80-89%", count: dockerScans.filter(s => s.score >= 80 && s.score < 90).length, color: "#84cc16" },
					{ range: "70-79%", count: dockerScans.filter(s => s.score >= 70 && s.score < 80).length, color: "#eab308" },
					{ range: "60-69%", count: dockerScans.filter(s => s.score >= 60 && s.score < 70).length, color: "#f97316" },
					{ range: "<60%", count: dockerScans.filter(s => s.score < 60).length, color: "#ef4444" },
				];

				// OpenSCAP rule breakdown (pass/fail)
				const openscapRuleData = openscapStats ? [
					{ name: "Passed", value: openscapStats.total_passed || 0, color: "#22c55e" },
					{ name: "Failed", value: openscapStats.total_failed || 0, color: "#ef4444" },
				].filter(d => d.value > 0) : [];

				// Docker Bench rule breakdown (pass/warn)
				const dockerRuleData = dockerBenchStats ? [
					{ name: "Passed", value: dockerBenchStats.total_passed || 0, color: "#22c55e" },
					{ name: "Warnings", value: dockerBenchStats.total_warnings || 0, color: "#eab308" },
				].filter(d => d.value > 0) : [];

				return (
					<>
					<div className="flex items-center gap-3 pt-4">
						<div className="flex items-center gap-2">
							<PieChartIcon className="h-5 w-5 text-primary-400" />
							<h2 className="text-lg font-semibold text-white">Analysis</h2>
						</div>
						<div className="flex-1 h-px bg-secondary-700" />
					</div>

					{/* Host Compliance Distribution with Scan Type Breakdown */}
					<div className="bg-secondary-800 rounded-lg border border-secondary-700 p-4">
						<h3 className="text-white font-medium mb-1 flex items-center gap-2">
							<PieChartIcon className="h-4 w-4 text-primary-400" />
							Host Compliance Status
						</h3>
						<p className="text-xs text-secondary-500 mb-3">Unique hosts by worst score - showing which scan type caused the status</p>

						{/* Stacked Bar Chart showing scan type breakdown */}
						{(() => {
							const statusData = summary.host_status_by_scan_type || {};
							const chartData = [
								{
									name: "Compliant",
									openscap: statusData.compliant?.openscap || 0,
									dockerBench: statusData.compliant?.["docker-bench"] || 0,
									total: summary.hosts_compliant || 0,
								},
								{
									name: "Warning",
									openscap: statusData.warning?.openscap || 0,
									dockerBench: statusData.warning?.["docker-bench"] || 0,
									total: summary.hosts_warning || 0,
								},
								{
									name: "Critical",
									openscap: statusData.critical?.openscap || 0,
									dockerBench: statusData.critical?.["docker-bench"] || 0,
									total: summary.hosts_critical || 0,
								},
							].filter(d => d.total > 0);

							return chartData.length > 0 ? (
								<>
									<div className="h-48">
										<ResponsiveContainer width="100%" height="100%">
											<BarChart data={chartData} layout="vertical">
												<XAxis type="number" stroke="#6b7280" fontSize={12} />
												<YAxis type="category" dataKey="name" stroke="#6b7280" fontSize={12} width={70} />
												<Tooltip
													contentStyle={{
														backgroundColor: "#1f2937",
														border: "1px solid #374151",
														borderRadius: "0.5rem",
													}}
													formatter={(value, name) => [
														value,
														name === "openscap" ? "OpenSCAP" : "Docker Bench"
													]}
												/>
												<Bar dataKey="openscap" stackId="a" fill="#22c55e" name="openscap" radius={[0, 0, 0, 0]} />
												<Bar dataKey="dockerBench" stackId="a" fill="#3b82f6" name="dockerBench" radius={[0, 4, 4, 0]} />
											</BarChart>
										</ResponsiveContainer>
									</div>
									<div className="flex justify-center gap-6 mt-2">
										<div className="flex items-center gap-2 text-sm">
											<div className="w-3 h-3 rounded bg-green-500" />
											<span className="text-green-400">OpenSCAP</span>
										</div>
										<div className="flex items-center gap-2 text-sm">
											<div className="w-3 h-3 rounded bg-blue-500" />
											<span className="text-blue-400">Docker Bench</span>
										</div>
									</div>
									{/* Detailed breakdown */}
									<div className="mt-4 pt-3 border-t border-secondary-700 grid grid-cols-3 gap-2 text-center text-xs">
										{chartData.map((status) => (
											<div key={status.name} className="space-y-1">
												<p className={`font-medium ${
													status.name === "Compliant" ? "text-green-400" :
													status.name === "Warning" ? "text-yellow-400" : "text-red-400"
												}`}>{status.name}</p>
												<p className="text-secondary-400">
													{status.openscap > 0 && <span className="text-green-400">{status.openscap} OpenSCAP</span>}
													{status.openscap > 0 && status.dockerBench > 0 && " + "}
													{status.dockerBench > 0 && <span className="text-blue-400">{status.dockerBench} Docker</span>}
												</p>
											</div>
										))}
									</div>
								</>
							) : (
								<div className="h-48 flex items-center justify-center text-secondary-500">
									No host status data available
								</div>
							);
						})()}
					</div>

					{/* Scan Type Analysis - Side by Side */}
					<div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
						{/* OpenSCAP Analysis */}
						<div className="bg-secondary-800 rounded-lg border border-green-700/50 overflow-hidden">
							<div className="px-4 py-3 bg-green-900/20 border-b border-green-700/30 flex items-center gap-2">
								<span className="px-2 py-0.5 bg-green-500/20 text-green-400 text-xs font-bold rounded-full border border-green-500/30">
									OpenSCAP
								</span>
								<span className="text-white font-medium text-sm">CIS Benchmark Analysis</span>
							</div>
							<div className="p-4">
								{openscapStats && openscapRuleData.length > 0 ? (
									<>
										{/* Rule Distribution Pie */}
										<div className="h-40">
											<ResponsiveContainer width="100%" height="100%">
												<PieChart>
													<Pie
														data={openscapRuleData}
														cx="50%"
														cy="50%"
														innerRadius={30}
														outerRadius={55}
														dataKey="value"
														label={({ value }) => value.toLocaleString()}
														labelLine={false}
													>
														{openscapRuleData.map((entry, index) => (
															<Cell key={`cell-${index}`} fill={entry.color} />
														))}
													</Pie>
													<Tooltip
														contentStyle={{
															backgroundColor: "#1f2937",
															border: "1px solid #374151",
															borderRadius: "0.5rem",
														}}
														formatter={(value, name) => [value.toLocaleString(), name]}
													/>
												</PieChart>
											</ResponsiveContainer>
										</div>
										<div className="flex justify-center gap-6 mt-2 text-sm">
											<div className="flex items-center gap-2">
												<div className="w-3 h-3 rounded-full bg-green-500" />
												<span className="text-green-400">Passed: {openscapStats.total_passed?.toLocaleString()}</span>
											</div>
											<div className="flex items-center gap-2">
												<div className="w-3 h-3 rounded-full bg-red-500" />
												<span className="text-red-400">Failed: {openscapStats.total_failed?.toLocaleString()}</span>
											</div>
										</div>
										{/* Score Distribution */}
										{openscapScans.length > 0 && (
											<div className="mt-4 pt-4 border-t border-secondary-700">
												<p className="text-xs text-secondary-400 mb-2">Score Distribution</p>
												<div className="h-24">
													<ResponsiveContainer width="100%" height="100%">
														<BarChart data={openscapScoreRanges} layout="vertical">
															<XAxis type="number" stroke="#6b7280" fontSize={10} />
															<YAxis type="category" dataKey="range" stroke="#6b7280" fontSize={10} width={50} />
															<Bar dataKey="count" radius={[0, 4, 4, 0]}>
																{openscapScoreRanges.map((entry, index) => (
																	<Cell key={`cell-${index}`} fill={entry.color} />
																))}
															</Bar>
														</BarChart>
													</ResponsiveContainer>
												</div>
											</div>
										)}
									</>
								) : (
									<div className="h-40 flex items-center justify-center text-secondary-500">
										No OpenSCAP data available
									</div>
								)}
							</div>
						</div>

						{/* Docker Bench Analysis */}
						<div className="bg-secondary-800 rounded-lg border border-blue-700/50 overflow-hidden">
							<div className="px-4 py-3 bg-blue-900/20 border-b border-blue-700/30 flex items-center gap-2">
								<span className="px-2 py-0.5 bg-blue-500/20 text-blue-400 text-xs font-bold rounded-full border border-blue-500/30">
									Docker Bench
								</span>
								<span className="text-white font-medium text-sm">Container Security Analysis</span>
							</div>
							<div className="p-4">
								{dockerBenchStats && dockerRuleData.length > 0 ? (
									<>
										{/* Rule Distribution Pie */}
										<div className="h-40">
											<ResponsiveContainer width="100%" height="100%">
												<PieChart>
													<Pie
														data={dockerRuleData}
														cx="50%"
														cy="50%"
														innerRadius={30}
														outerRadius={55}
														dataKey="value"
														label={({ value }) => value.toLocaleString()}
														labelLine={false}
													>
														{dockerRuleData.map((entry, index) => (
															<Cell key={`cell-${index}`} fill={entry.color} />
														))}
													</Pie>
													<Tooltip
														contentStyle={{
															backgroundColor: "#1f2937",
															border: "1px solid #374151",
															borderRadius: "0.5rem",
														}}
														formatter={(value, name) => [value.toLocaleString(), name]}
													/>
												</PieChart>
											</ResponsiveContainer>
										</div>
										<div className="flex justify-center gap-6 mt-2 text-sm">
											<div className="flex items-center gap-2">
												<div className="w-3 h-3 rounded-full bg-green-500" />
												<span className="text-green-400">Passed: {dockerBenchStats.total_passed?.toLocaleString()}</span>
											</div>
											<div className="flex items-center gap-2">
												<div className="w-3 h-3 rounded-full bg-yellow-500" />
												<span className="text-yellow-400">Warnings: {dockerBenchStats.total_warnings?.toLocaleString()}</span>
											</div>
										</div>
										{/* Score Distribution */}
										{dockerScans.length > 0 && (
											<div className="mt-4 pt-4 border-t border-secondary-700">
												<p className="text-xs text-secondary-400 mb-2">Score Distribution</p>
												<div className="h-24">
													<ResponsiveContainer width="100%" height="100%">
														<BarChart data={dockerScoreRanges} layout="vertical">
															<XAxis type="number" stroke="#6b7280" fontSize={10} />
															<YAxis type="category" dataKey="range" stroke="#6b7280" fontSize={10} width={50} />
															<Bar dataKey="count" radius={[0, 4, 4, 0]}>
																{dockerScoreRanges.map((entry, index) => (
																	<Cell key={`cell-${index}`} fill={entry.color} />
																))}
															</Bar>
														</BarChart>
													</ResponsiveContainer>
												</div>
											</div>
										)}
									</>
								) : (
									<div className="h-40 flex items-center justify-center text-secondary-500">
										No Docker Bench data available
									</div>
								)}
							</div>
						</div>
					</div>
					</>
				);
			})()}

			{/* Charts Section - For specific scan types */}
			{profileTypeFilter !== "all" && filteredScans && filteredScans.length > 0 && (() => {
				// Score ranges for bar chart - filtered scans
				const scoreRanges = [
					{ range: "90-100%", count: filteredScans.filter(s => s.score >= 90).length, color: "#22c55e" },
					{ range: "80-89%", count: filteredScans.filter(s => s.score >= 80 && s.score < 90).length, color: "#84cc16" },
					{ range: "70-79%", count: filteredScans.filter(s => s.score >= 70 && s.score < 80).length, color: "#eab308" },
					{ range: "60-69%", count: filteredScans.filter(s => s.score >= 60 && s.score < 70).length, color: "#f97316" },
					{ range: "<60%", count: filteredScans.filter(s => s.score < 60).length, color: "#ef4444" },
				];

				return (
					<div className="bg-secondary-800 rounded-lg border border-secondary-700 p-4">
						<h3 className="text-white font-medium mb-4 flex items-center gap-2">
							{profileTypeFilter === "openscap" ? (
								<Server className="h-4 w-4 text-green-400" />
							) : (
								<Container className="h-4 w-4 text-blue-400" />
							)}
							{getFilterDisplayName()} - Score Distribution
						</h3>
						<div className="h-48">
							<ResponsiveContainer width="100%" height="100%">
								<BarChart data={scoreRanges} layout="vertical">
									<XAxis type="number" stroke="#6b7280" fontSize={12} />
									<YAxis type="category" dataKey="range" stroke="#6b7280" fontSize={12} width={60} />
									<Tooltip
										contentStyle={{
											backgroundColor: "#1f2937",
											border: "1px solid #374151",
											borderRadius: "0.5rem",
										}}
										formatter={(value) => [value, "Scans"]}
									/>
									<Bar dataKey="count" radius={[0, 4, 4, 0]}>
										{scoreRanges.map((entry, index) => (
											<Cell key={`cell-${index}`} fill={entry.color} />
										))}
									</Bar>
								</BarChart>
							</ResponsiveContainer>
						</div>
					</div>
				);
			})()}

			{/* Additional Charts - Severity & Profile Distribution - Only for "All Scans" tab */}
			{profileTypeFilter === "all" && ((severity_breakdown && severity_breakdown.length > 0) || (profile_distribution && profile_distribution.length > 0)) && (
				<div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
					{/* Severity Breakdown */}
					{severity_breakdown && severity_breakdown.length > 0 && (() => {
						const severityColors = {
							critical: "#ef4444",
							high: "#f97316",
							medium: "#eab308",
							low: "#22c55e",
							unknown: "#6b7280",
						};
						const severityData = severity_breakdown.map(s => ({
							name: s.severity.charAt(0).toUpperCase() + s.severity.slice(1),
							value: s.count,
							color: severityColors[s.severity] || severityColors.unknown,
						}));
						const totalFailures = severityData.reduce((sum, s) => sum + s.value, 0);

						return (
							<div className="bg-secondary-800 rounded-lg border border-secondary-700 p-4">
								<h3 className="text-white font-medium mb-1 flex items-center gap-2">
									<AlertTriangle className="h-4 w-4 text-primary-400" />
									Rule Failures by Severity
								</h3>
								<p className="text-xs text-secondary-500 mb-3">{totalFailures.toLocaleString()} total rule failures across all hosts</p>
								<div className="h-48">
									<ResponsiveContainer width="100%" height="100%">
										<PieChart>
											<Pie
												data={severityData}
												cx="50%"
												cy="50%"
												innerRadius={40}
												outerRadius={70}
												dataKey="value"
												label={({ name, value }) => `${value}`}
												labelLine={false}
											>
												{severityData.map((entry, index) => (
													<Cell key={`cell-${index}`} fill={entry.color} />
												))}
											</Pie>
											<Tooltip
												contentStyle={{
													backgroundColor: "#1f2937",
													border: "1px solid #374151",
													borderRadius: "0.5rem",
												}}
												formatter={(value, name) => [value, name]}
											/>
										</PieChart>
									</ResponsiveContainer>
								</div>
								<div className="flex flex-wrap justify-center gap-4 mt-2">
									{severityData.map((entry) => (
										<div key={entry.name} className="flex items-center gap-2 text-sm">
											<div className="w-3 h-3 rounded-full" style={{ backgroundColor: entry.color }} />
											<span className="text-secondary-400">{entry.name}: {entry.value}</span>
										</div>
									))}
								</div>
							</div>
						);
					})()}

					{/* Profile Distribution */}
					{profile_distribution && profile_distribution.length > 0 && (
						<div className="bg-secondary-800 rounded-lg border border-secondary-700 p-4">
							<h3 className="text-white font-medium mb-1 flex items-center gap-2">
								<Shield className="h-4 w-4 text-primary-400" />
								Compliance Profiles in Use
							</h3>
							<p className="text-xs text-secondary-500 mb-3">Number of hosts scanned with each profile</p>
							<div className="h-48">
								<ResponsiveContainer width="100%" height="100%">
									<BarChart data={profile_distribution} layout="vertical">
										<XAxis type="number" stroke="#6b7280" fontSize={12} />
										<YAxis
											type="category"
											dataKey="name"
											stroke="#6b7280"
											fontSize={11}
											width={140}
											tickFormatter={(value) => value.length > 20 ? value.slice(0, 20) + "..." : value}
										/>
										<Tooltip
											contentStyle={{
												backgroundColor: "#1f2937",
												border: "1px solid #374151",
												borderRadius: "0.5rem",
											}}
											formatter={(value) => [value, "Hosts"]}
										/>
										<Bar dataKey="host_count" fill="#6366f1" radius={[0, 4, 4, 0]} />
									</BarChart>
								</ResponsiveContainer>
							</div>
						</div>
					)}
				</div>
			)}

			{/* ==================== ISSUES SECTION ==================== */}
			{((profileTypeFilter === "all" || profileTypeFilter === "openscap") && filteredTopFailingRules?.length > 0) ||
			 ((profileTypeFilter === "all" || profileTypeFilter === "docker-bench") && filteredTopWarningRules?.length > 0) ? (
				<div className="flex items-center gap-3 pt-4">
					<div className="flex items-center gap-2">
						<AlertTriangle className="h-5 w-5 text-primary-400" />
						<h2 className="text-lg font-semibold text-white">Issues</h2>
					</div>
					<div className="flex-1 h-px bg-secondary-700" />
				</div>
			) : null}

			{/* Top Failing Rules - OpenSCAP (show on All Scans and OpenSCAP tabs) */}
			{(profileTypeFilter === "all" || profileTypeFilter === "openscap") && filteredTopFailingRules && filteredTopFailingRules.length > 0 && (
				<div className="bg-secondary-800 rounded-lg border border-green-700/50 overflow-hidden">
					<div className="px-4 py-3 border-b border-secondary-700 bg-green-900/20 flex items-center justify-between">
						<div className="flex items-center gap-3">
							<span className="px-2 py-1 bg-green-500/20 text-green-400 text-xs font-bold rounded-full border border-green-500/30">
								OpenSCAP
							</span>
							<div>
								<h2 className="text-white font-semibold">Top Failing Rules</h2>
								<p className="text-xs text-secondary-400">CIS Benchmark compliance failures</p>
							</div>
						</div>
						<span className="px-3 py-1 bg-red-500/20 text-red-400 text-sm font-bold rounded-full border border-red-500/30">
							{filteredTopFailingRules.filter(r => r.profile_type === "openscap" || profileTypeFilter === "openscap").length} rules
						</span>
					</div>
					<div className="divide-y divide-secondary-700">
						{filteredTopFailingRules.filter(r => r.profile_type === "openscap" || profileTypeFilter === "openscap").map((rule) => {
							const severityColors = {
								critical: "bg-red-500/20 text-red-400 border-red-500/30",
								high: "bg-orange-500/20 text-orange-400 border-orange-500/30",
								medium: "bg-yellow-500/20 text-yellow-400 border-yellow-500/30",
								low: "bg-green-500/20 text-green-400 border-green-500/30",
							};
							const severityClass = severityColors[rule.severity] || "bg-secondary-700 text-secondary-400 border-secondary-600";

							return (
								<div key={rule.rule_id} className="flex items-center justify-between px-4 py-3 hover:bg-secondary-700/30 transition-colors">
									<div className="flex-1 min-w-0">
										<p className="text-white font-medium truncate">{rule.title}</p>
										<p className="text-sm text-secondary-400 truncate">{rule.rule_id}</p>
									</div>
									<div className="flex items-center gap-3 ml-4">
										<span className={`px-2 py-0.5 rounded text-xs font-medium border ${severityClass}`}>
											{rule.severity}
										</span>
										<span className="text-red-400 font-bold whitespace-nowrap">
											{rule.fail_count} {rule.fail_count === 1 ? "host" : "hosts"}
										</span>
									</div>
								</div>
							);
						})}
					</div>
				</div>
			)}

			{/* Top Warning Rules - Docker Bench (show on All Scans and Docker Bench tabs) */}
			{(profileTypeFilter === "all" || profileTypeFilter === "docker-bench") && filteredTopWarningRules && filteredTopWarningRules.length > 0 && (
				<div className="bg-secondary-800 rounded-lg border border-blue-700/50 overflow-hidden">
					<div className="px-4 py-3 border-b border-secondary-700 bg-blue-900/20 flex items-center justify-between">
						<div className="flex items-center gap-3">
							<span className="px-2 py-1 bg-blue-500/20 text-blue-400 text-xs font-bold rounded-full border border-blue-500/30">
								Docker Bench
							</span>
							<div>
								<h2 className="text-white font-semibold">Top Warnings</h2>
								<p className="text-xs text-secondary-400">Container security issues</p>
							</div>
						</div>
						<span className="px-3 py-1 bg-yellow-500/20 text-yellow-400 text-sm font-bold rounded-full border border-yellow-500/30">
							{filteredTopWarningRules.filter(r => r.profile_type === "docker-bench" || profileTypeFilter === "docker-bench").length} rules
						</span>
					</div>
					<div className="divide-y divide-secondary-700">
						{filteredTopWarningRules.filter(r => r.profile_type === "docker-bench" || profileTypeFilter === "docker-bench").map((rule) => {
							return (
								<div key={rule.rule_id} className="flex items-center justify-between px-4 py-3 hover:bg-secondary-700/30 transition-colors">
									<div className="flex-1 min-w-0">
										<p className="text-white font-medium truncate">{rule.title}</p>
										<p className="text-sm text-secondary-400 truncate">{rule.rule_id}</p>
									</div>
									<div className="flex items-center gap-3 ml-4">
										<span className="px-2 py-0.5 rounded text-xs font-medium border bg-yellow-500/20 text-yellow-400 border-yellow-500/30">
											warning
										</span>
										<span className="text-yellow-400 font-bold whitespace-nowrap">
											{rule.warn_count} {rule.warn_count === 1 ? "host" : "hosts"}
										</span>
									</div>
								</div>
							);
						})}
					</div>
				</div>
			)}

			{/* ==================== ACTIVITY SECTION ==================== */}
			<div className="flex items-center gap-3 pt-4">
				<div className="flex items-center gap-2">
					<Clock className="h-5 w-5 text-primary-400" />
					<h2 className="text-lg font-semibold text-white">Activity</h2>
				</div>
				<div className="flex-1 h-px bg-secondary-700" />
			</div>

			<div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
				{/* Recent Scans */}
				<div className="bg-secondary-800 rounded-lg border border-secondary-700">
					<div className="px-4 py-3 border-b border-secondary-700">
						<h2 className="text-lg font-semibold text-white flex items-center gap-2">
							{profileTypeFilter === "openscap" ? (
								<Server className="h-5 w-5 text-green-400" />
							) : profileTypeFilter === "docker-bench" ? (
								<Container className="h-5 w-5 text-blue-400" />
							) : (
								<Clock className="h-5 w-5 text-secondary-400" />
							)}
							Recent Scans - {getFilterDisplayName()}
						</h2>
					</div>
					<div className="divide-y divide-secondary-700">
						{filteredScans?.map((scan) => {
							const isDockerBench = scan.compliance_profiles?.type === "docker-bench";
							return (
								<Link
									key={scan.id}
									to={`/hosts/${scan.host?.id}`}
									className="flex items-center justify-between px-4 py-3 hover:bg-secondary-700/50 transition-colors"
								>
									<div className="flex items-center gap-3">
										<div className="flex flex-col items-center">
											{isDockerBench ? (
												<Container className="h-4 w-4 text-blue-400 flex-shrink-0" />
											) : (
												<Server className="h-4 w-4 text-green-400 flex-shrink-0" />
											)}
										</div>
										<div>
											<p className="text-white font-medium">
												{scan.host?.friendly_name || scan.host?.hostname}
											</p>
											<div className="flex items-center gap-2">
												<span className={`px-1.5 py-0.5 text-xs rounded ${
													isDockerBench
														? "bg-blue-900/30 text-blue-400"
														: "bg-green-900/30 text-green-400"
												}`}>
													{isDockerBench ? "Docker" : "OpenSCAP"}
												</span>
												<span className="text-sm text-secondary-400">{scan.profile?.name}</span>
											</div>
										</div>
									</div>
									<div className="flex items-center gap-3">
										<ComplianceScore score={scan.score} size="sm" />
										<span className="text-xs text-secondary-500">
											{new Date(scan.completed_at).toLocaleDateString()}
										</span>
									</div>
								</Link>
							);
						})}
						{(!filteredScans || filteredScans.length === 0) && (
							<div className="px-4 py-8 text-center text-secondary-400">
								No {getFilterDisplayName()} scans found
							</div>
						)}
					</div>
				</div>

				{/* Worst Performing Hosts */}
				<div className="bg-secondary-800 rounded-lg border border-secondary-700">
					<div className="px-4 py-3 border-b border-secondary-700">
						<h2 className="text-lg font-semibold text-white flex items-center gap-2">
							{profileTypeFilter === "openscap" ? (
								<Server className="h-5 w-5 text-green-400" />
							) : profileTypeFilter === "docker-bench" ? (
								<Container className="h-5 w-5 text-blue-400" />
							) : (
								<TrendingDown className="h-5 w-5 text-red-400" />
							)}
							Needs Attention - {getFilterDisplayName()}
						</h2>
					</div>
					<div className="divide-y divide-secondary-700">
						{filteredWorstHosts?.map((scan) => {
							const isDockerBench = scan.compliance_profiles?.type === "docker-bench";
							return (
								<Link
									key={scan.id}
									to={`/hosts/${scan.host?.id}`}
									className="flex items-center justify-between px-4 py-3 hover:bg-secondary-700/50 transition-colors"
								>
									<div className="flex items-center gap-3">
										{isDockerBench ? (
											<Container className="h-4 w-4 text-blue-400 flex-shrink-0" />
										) : (
											<Server className="h-4 w-4 text-green-400 flex-shrink-0" />
										)}
										<div>
											<p className="text-white font-medium">
												{scan.host?.friendly_name || scan.host?.hostname}
											</p>
											<div className="flex items-center gap-2">
												<span className={`px-1.5 py-0.5 text-xs rounded ${
													isDockerBench
														? "bg-blue-900/30 text-blue-400"
														: "bg-green-900/30 text-green-400"
												}`}>
													{isDockerBench ? "Docker" : "OpenSCAP"}
												</span>
												<span className="text-sm text-secondary-400">{scan.profile?.name}</span>
											</div>
										</div>
									</div>
									<ComplianceScore score={scan.score} size="sm" />
								</Link>
							);
						})}
						{(!filteredWorstHosts || filteredWorstHosts.length === 0) && (
							<div className="px-4 py-8 text-center text-secondary-400">
								No {getFilterDisplayName()} hosts with low scores
							</div>
						)}
					</div>
				</div>
			</div>
		</div>
	);
};

export default Compliance;
