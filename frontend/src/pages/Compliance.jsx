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

	const { summary, recent_scans, worst_hosts, top_failing_rules, profile_distribution, severity_breakdown, profile_type_stats } = dashboard || {};
	const activeScans = activeScansData?.activeScans || [];

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

			{/* Summary Cards - Row 1: Host Stats */}
			<div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-4">
				<div className="bg-secondary-800 rounded-lg p-4 border border-secondary-700">
					<div className="flex items-center gap-2 text-secondary-400 mb-2">
						<Server className="h-4 w-4" />
						<span className="text-sm">Scanned Hosts</span>
					</div>
					<p className="text-2xl font-bold text-white">{summary?.total_hosts || 0}</p>
				</div>

				<div className="bg-secondary-800 rounded-lg p-4 border border-secondary-700">
					<div className="flex items-center gap-2 text-secondary-400 mb-2">
						<TrendingUp className="h-4 w-4" />
						<span className="text-sm">Average Score</span>
					</div>
					<p className="text-2xl font-bold text-white">{summary?.average_score?.toFixed(1) || 0}%</p>
				</div>

				<div className="bg-secondary-800 rounded-lg p-4 border border-green-700/50">
					<div className="flex items-center gap-2 text-green-400 mb-2">
						<ShieldCheck className="h-4 w-4" />
						<span className="text-sm">Compliant</span>
					</div>
					<p className="text-2xl font-bold text-green-400">{summary?.compliant || 0}</p>
				</div>

				<div className="bg-secondary-800 rounded-lg p-4 border border-yellow-700/50">
					<div className="flex items-center gap-2 text-yellow-400 mb-2">
						<ShieldAlert className="h-4 w-4" />
						<span className="text-sm">Warning</span>
					</div>
					<p className="text-2xl font-bold text-yellow-400">{summary?.warning || 0}</p>
				</div>

				<div className="bg-secondary-800 rounded-lg p-4 border border-red-700/50">
					<div className="flex items-center gap-2 text-red-400 mb-2">
						<ShieldX className="h-4 w-4" />
						<span className="text-sm">Critical</span>
					</div>
					<p className="text-2xl font-bold text-red-400">{summary?.critical || 0}</p>
				</div>

				<div className="bg-secondary-800 rounded-lg p-4 border border-secondary-600">
					<div className="flex items-center gap-2 text-secondary-400 mb-2">
						<ShieldOff className="h-4 w-4" />
						<span className="text-sm">Not Scanned</span>
					</div>
					<p className="text-2xl font-bold text-secondary-400">{summary?.unscanned || 0}</p>
				</div>
			</div>

			{/* Summary Cards - Row 2: Rule Stats */}
			{summary?.total_rules > 0 && (
				<div className="grid grid-cols-2 md:grid-cols-4 gap-4">
					<div className="bg-secondary-800 rounded-lg p-4 border border-secondary-700">
						<div className="flex items-center gap-2 text-secondary-400 mb-2">
							<List className="h-4 w-4" />
							<span className="text-sm">Total Rules Evaluated</span>
						</div>
						<p className="text-2xl font-bold text-white">{summary?.total_rules?.toLocaleString() || 0}</p>
					</div>

					<div className="bg-secondary-800 rounded-lg p-4 border border-green-700/50">
						<div className="flex items-center gap-2 text-green-400 mb-2">
							<CheckCircle className="h-4 w-4" />
							<span className="text-sm">Rules Passed</span>
						</div>
						<p className="text-2xl font-bold text-green-400">{summary?.total_passed_rules?.toLocaleString() || 0}</p>
					</div>

					<div className="bg-secondary-800 rounded-lg p-4 border border-red-700/50">
						<div className="flex items-center gap-2 text-red-400 mb-2">
							<XCircle className="h-4 w-4" />
							<span className="text-sm">Rules Failed</span>
						</div>
						<p className="text-2xl font-bold text-red-400">{summary?.total_failed_rules?.toLocaleString() || 0}</p>
					</div>

					<div className="bg-secondary-800 rounded-lg p-4 border border-secondary-700">
						<div className="flex items-center gap-2 text-secondary-400 mb-2">
							<TrendingUp className="h-4 w-4" />
							<span className="text-sm">Pass Rate</span>
						</div>
						<p className="text-2xl font-bold text-white">
							{summary?.total_rules > 0
								? ((summary?.total_passed_rules / summary?.total_rules) * 100).toFixed(1)
								: 0}%
						</p>
					</div>
				</div>
			)}

			{/* Profile Type Stats - OpenSCAP vs Docker Bench */}
			{profile_type_stats && profile_type_stats.length > 0 && (
				<div className="grid grid-cols-1 md:grid-cols-2 gap-4">
					{/* OpenSCAP Card */}
					{(() => {
						const openscap = profile_type_stats.find(p => p.type === "openscap");
						return (
							<div className={`bg-secondary-800 rounded-lg p-4 border-2 ${openscap ? "border-blue-700/50" : "border-secondary-700 opacity-50"}`}>
								<div className="flex items-center justify-between mb-3">
									<div className="flex items-center gap-2">
										<Server className="h-5 w-5 text-blue-400" />
										<h3 className="text-white font-medium">OpenSCAP</h3>
									</div>
									{openscap?.average_score != null && (
										<span className={`text-xl font-bold ${
											openscap.average_score >= 80 ? "text-green-400" :
											openscap.average_score >= 60 ? "text-yellow-400" : "text-red-400"
										}`}>
											{Math.round(openscap.average_score)}%
										</span>
									)}
								</div>
								{openscap ? (
									<div className="grid grid-cols-2 gap-3 text-sm">
										<div>
											<p className="text-secondary-400">Hosts Scanned</p>
											<p className="text-white font-medium">{openscap.hosts_scanned}</p>
										</div>
										<div>
											<p className="text-secondary-400">Total Rules</p>
											<p className="text-white font-medium">{openscap.total_rules?.toLocaleString()}</p>
										</div>
										<div>
											<p className="text-green-400">Passed</p>
											<p className="text-white font-medium">{openscap.total_passed?.toLocaleString()}</p>
										</div>
										<div>
											<p className="text-red-400">Failed</p>
											<p className="text-white font-medium">{openscap.total_failed?.toLocaleString()}</p>
										</div>
									</div>
								) : (
									<p className="text-secondary-500 text-sm">No OpenSCAP scans yet</p>
								)}
							</div>
						);
					})()}

					{/* Docker Bench Card */}
					{(() => {
						const dockerBench = profile_type_stats.find(p => p.type === "docker-bench");
						return (
							<div className={`bg-secondary-800 rounded-lg p-4 border-2 ${dockerBench ? "border-cyan-700/50" : "border-secondary-700 opacity-50"}`}>
								<div className="flex items-center justify-between mb-3">
									<div className="flex items-center gap-2">
										<Container className="h-5 w-5 text-cyan-400" />
										<h3 className="text-white font-medium">Docker Bench</h3>
									</div>
									{dockerBench?.average_score != null && (
										<span className={`text-xl font-bold ${
											dockerBench.average_score >= 80 ? "text-green-400" :
											dockerBench.average_score >= 60 ? "text-yellow-400" : "text-red-400"
										}`}>
											{Math.round(dockerBench.average_score)}%
										</span>
									)}
								</div>
								{dockerBench ? (
									<div className="grid grid-cols-2 gap-3 text-sm">
										<div>
											<p className="text-secondary-400">Hosts Scanned</p>
											<p className="text-white font-medium">{dockerBench.hosts_scanned}</p>
										</div>
										<div>
											<p className="text-secondary-400">Total Rules</p>
											<p className="text-white font-medium">{dockerBench.total_rules?.toLocaleString()}</p>
										</div>
										<div>
											<p className="text-green-400">Passed</p>
											<p className="text-white font-medium">{dockerBench.total_passed?.toLocaleString()}</p>
										</div>
										<div>
											<p className="text-yellow-400">Warnings</p>
											<p className="text-white font-medium">{dockerBench.total_warnings?.toLocaleString()}</p>
										</div>
									</div>
								) : (
									<p className="text-secondary-500 text-sm">No Docker Bench scans yet</p>
								)}
							</div>
						);
					})()}
				</div>
			)}

			{/* Charts Section */}
			{summary && (summary.total_hosts > 0 || summary.unscanned > 0) && (() => {
				const hostDistribution = [
					{ name: "Compliant (≥80%)", value: summary.compliant || 0, color: "#22c55e" },
					{ name: "Warning (60-80%)", value: summary.warning || 0, color: "#eab308" },
					{ name: "Critical (<60%)", value: summary.critical || 0, color: "#ef4444" },
					{ name: "Not Scanned", value: summary.unscanned || 0, color: "#6b7280" },
				].filter(d => d.value > 0);

				// Score ranges for bar chart
				const scoreRanges = [
					{ range: "90-100%", count: filteredScans?.filter(s => s.score >= 90).length || 0, color: "#22c55e" },
					{ range: "80-89%", count: filteredScans?.filter(s => s.score >= 80 && s.score < 90).length || 0, color: "#84cc16" },
					{ range: "70-79%", count: filteredScans?.filter(s => s.score >= 70 && s.score < 80).length || 0, color: "#eab308" },
					{ range: "60-69%", count: filteredScans?.filter(s => s.score >= 60 && s.score < 70).length || 0, color: "#f97316" },
					{ range: "<60%", count: filteredScans?.filter(s => s.score < 60).length || 0, color: "#ef4444" },
				];

				return (
					<div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
						{/* Host Compliance Distribution */}
						<div className="bg-secondary-800 rounded-lg border border-secondary-700 p-4">
							<h3 className="text-white font-medium mb-4 flex items-center gap-2">
								<PieChartIcon className="h-4 w-4 text-primary-400" />
								Host Compliance Status
							</h3>
							<div className="h-48">
								<ResponsiveContainer width="100%" height="100%">
									<PieChart>
										<Pie
											data={hostDistribution}
											cx="50%"
											cy="50%"
											innerRadius={40}
											outerRadius={70}
											dataKey="value"
											label={({ name, value }) => `${value}`}
											labelLine={false}
										>
											{hostDistribution.map((entry, index) => (
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
								{hostDistribution.map((entry) => (
									<div key={entry.name} className="flex items-center gap-2 text-sm">
										<div className="w-3 h-3 rounded-full" style={{ backgroundColor: entry.color }} />
										<span className="text-secondary-400">{entry.name}: {entry.value}</span>
									</div>
								))}
							</div>
						</div>

						{/* Recent Scan Score Distribution */}
						{filteredScans && filteredScans.length > 0 && (
							<div className="bg-secondary-800 rounded-lg border border-secondary-700 p-4">
								<h3 className="text-white font-medium mb-4 flex items-center gap-2">
									<BarChart3 className="h-4 w-4 text-primary-400" />
									Recent Scans by Score
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
						)}
					</div>
				);
			})()}

			{/* Additional Charts - Severity & Profile Distribution */}
			{((severity_breakdown && severity_breakdown.length > 0) || (profile_distribution && profile_distribution.length > 0)) && (
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

						return (
							<div className="bg-secondary-800 rounded-lg border border-secondary-700 p-4">
								<h3 className="text-white font-medium mb-4 flex items-center gap-2">
									<AlertTriangle className="h-4 w-4 text-primary-400" />
									Failed Rules by Severity
								</h3>
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
							<h3 className="text-white font-medium mb-4 flex items-center gap-2">
								<Shield className="h-4 w-4 text-primary-400" />
								Profiles in Use
							</h3>
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

			{/* Top Failing Rules */}
			{filteredTopFailingRules && filteredTopFailingRules.length > 0 && (
				<div className="bg-secondary-800 rounded-lg border border-secondary-700">
					<div className="px-4 py-3 border-b border-secondary-700">
						<h2 className="text-lg font-semibold text-white flex items-center gap-2">
							<XCircle className="h-5 w-5 text-red-400" />
							Top Failing Rules
							{profileTypeFilter !== "all" && (
								<span className="text-sm font-normal text-secondary-400 ml-2">
									({profileTypeFilter === "openscap" ? "OpenSCAP" : "Docker Bench"})
								</span>
							)}
						</h2>
					</div>
					<div className="divide-y divide-secondary-700">
						{filteredTopFailingRules.map((rule) => {
							const severityColors = {
								critical: "bg-red-500/20 text-red-400 border-red-500/30",
								high: "bg-orange-500/20 text-orange-400 border-orange-500/30",
								medium: "bg-yellow-500/20 text-yellow-400 border-yellow-500/30",
								low: "bg-green-500/20 text-green-400 border-green-500/30",
							};
							const severityClass = severityColors[rule.severity] || "bg-secondary-700 text-secondary-400 border-secondary-600";

							return (
								<div key={rule.rule_id} className="flex items-center justify-between px-4 py-3">
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

			<div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
				{/* Recent Scans */}
				<div className="bg-secondary-800 rounded-lg border border-secondary-700">
					<div className="px-4 py-3 border-b border-secondary-700">
						<h2 className="text-lg font-semibold text-white flex items-center gap-2">
							<Clock className="h-5 w-5 text-secondary-400" />
							Recent Scans
							{profileTypeFilter !== "all" && (
								<span className="text-sm font-normal text-secondary-400 ml-2">
									({profileTypeFilter === "openscap" ? "OpenSCAP" : "Docker Bench"})
								</span>
							)}
						</h2>
					</div>
					<div className="divide-y divide-secondary-700">
						{filteredScans?.map((scan) => (
							<Link
								key={scan.id}
								to={`/hosts/${scan.host?.id}`}
								className="flex items-center justify-between px-4 py-3 hover:bg-secondary-700/50 transition-colors"
							>
								<div className="flex items-center gap-3">
									{scan.compliance_profiles?.type === "docker-bench" ? (
										<Container className="h-4 w-4 text-blue-400 flex-shrink-0" />
									) : (
										<Server className="h-4 w-4 text-green-400 flex-shrink-0" />
									)}
									<div>
										<p className="text-white font-medium">
											{scan.host?.friendly_name || scan.host?.hostname}
										</p>
										<p className="text-sm text-secondary-400">{scan.profile?.name}</p>
									</div>
								</div>
								<div className="flex items-center gap-3">
									<ComplianceScore score={scan.score} size="sm" />
									<span className="text-xs text-secondary-500">
										{new Date(scan.completed_at).toLocaleDateString()}
									</span>
								</div>
							</Link>
						))}
						{(!filteredScans || filteredScans.length === 0) && (
							<div className="px-4 py-8 text-center text-secondary-400">
								No compliance scans yet
							</div>
						)}
					</div>
				</div>

				{/* Worst Performing Hosts */}
				<div className="bg-secondary-800 rounded-lg border border-secondary-700">
					<div className="px-4 py-3 border-b border-secondary-700">
						<h2 className="text-lg font-semibold text-white flex items-center gap-2">
							<TrendingDown className="h-5 w-5 text-red-400" />
							Needs Attention
							{profileTypeFilter !== "all" && (
								<span className="text-sm font-normal text-secondary-400 ml-2">
									({profileTypeFilter === "openscap" ? "OpenSCAP" : "Docker Bench"})
								</span>
							)}
						</h2>
					</div>
					<div className="divide-y divide-secondary-700">
						{filteredWorstHosts?.map((scan) => (
							<Link
								key={scan.id}
								to={`/hosts/${scan.host?.id}`}
								className="flex items-center justify-between px-4 py-3 hover:bg-secondary-700/50 transition-colors"
							>
								<div className="flex items-center gap-3">
									{scan.compliance_profiles?.type === "docker-bench" ? (
										<Container className="h-4 w-4 text-blue-400 flex-shrink-0" />
									) : (
										<Server className="h-4 w-4 text-green-400 flex-shrink-0" />
									)}
									<div>
										<p className="text-white font-medium">
											{scan.host?.friendly_name || scan.host?.hostname}
										</p>
										<p className="text-sm text-secondary-400">{scan.profile?.name}</p>
									</div>
								</div>
								<ComplianceScore score={scan.score} size="sm" />
							</Link>
						))}
						{(!filteredWorstHosts || filteredWorstHosts.length === 0) && (
							<div className="px-4 py-8 text-center text-secondary-400">
								No hosts with low compliance scores
							</div>
						)}
					</div>
				</div>
			</div>
		</div>
	);
};

export default Compliance;
