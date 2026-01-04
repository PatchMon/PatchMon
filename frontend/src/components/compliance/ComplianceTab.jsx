import { useState, lazy, Suspense, useEffect } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
	Shield,
	Play,
	RefreshCw,
	CheckCircle,
	XCircle,
	AlertTriangle,
	MinusCircle,
	ChevronDown,
	ChevronRight,
	Settings,
	Clock,
	BarChart3,
	ListChecks,
	History,
	Info,
	Package,
	Server,
	Wrench,
	ToggleLeft,
	ToggleRight,
	Download,
	BookOpen,
} from "lucide-react";
import { complianceAPI } from "../../utils/complianceApi";
import { hostsAPI } from "../../utils/api";
import ComplianceScore from "./ComplianceScore";

// Lazy load ComplianceTrend to avoid recharts bundling issues
const ComplianceTrend = lazy(() => import("./ComplianceTrend"));

// Fallback scan profiles (used if agent doesn't provide any)
const DEFAULT_SCAN_PROFILES = [
	{ id: "level1_server", name: "CIS Level 1 Server", description: "Basic security hardening for servers", type: "openscap" },
	{ id: "level2_server", name: "CIS Level 2 Server", description: "Extended security hardening (more restrictive)", type: "openscap" },
];

// Subtab definitions
const SUBTABS = [
	{ id: "overview", name: "Overview", icon: BarChart3 },
	{ id: "scan", name: "Run Scan", icon: Play },
	{ id: "results", name: "Results", icon: ListChecks },
	{ id: "history", name: "History", icon: History },
	{ id: "settings", name: "Settings", icon: Settings },
];

const ComplianceTab = ({ hostId, isConnected }) => {
	const [activeSubtab, setActiveSubtab] = useState("overview");
	const [expandedRules, setExpandedRules] = useState({});
	const [statusFilter, setStatusFilter] = useState("all");
	const [scanMessage, setScanMessage] = useState(null);
	const [selectedProfile, setSelectedProfile] = useState("openscap");
	const [scanInProgress, setScanInProgress] = useState(false);
	const [enableRemediation, setEnableRemediation] = useState(false);
	const queryClient = useQueryClient();

	const { data: latestScan, isLoading, refetch: refetchLatest } = useQuery({
		queryKey: ["compliance-latest", hostId],
		queryFn: () => complianceAPI.getLatestScan(hostId).then((res) => res.data),
		enabled: !!hostId,
	});

	const { data: scanHistory, refetch: refetchHistory } = useQuery({
		queryKey: ["compliance-history", hostId],
		queryFn: () => complianceAPI.getHostScans(hostId, { limit: 10 }).then((res) => res.data),
		enabled: !!hostId,
	});

	// Get integration status (scanner info, components)
	const { data: integrationStatus, refetch: refetchStatus, isFetching: isRefreshingStatus } = useQuery({
		queryKey: ["compliance-status", hostId],
		queryFn: () => complianceAPI.getIntegrationStatus(hostId).then((res) => res.data),
		enabled: !!hostId,
		refetchInterval: 30000, // Refresh every 30 seconds
	});

	// Agent update mutation
	const [updateMessage, setUpdateMessage] = useState(null);
	const agentUpdateMutation = useMutation({
		mutationFn: () => hostsAPI.forceAgentUpdate(hostId),
		onSuccess: () => {
			setUpdateMessage({ type: "success", text: "Update command sent! Agent will update shortly." });
			setTimeout(() => setUpdateMessage(null), 5000);
		},
		onError: (error) => {
			setUpdateMessage({
				type: "error",
				text: error.response?.data?.error || "Failed to send update command"
			});
			setTimeout(() => setUpdateMessage(null), 5000);
		},
	});

	// SSG upgrade mutation
	const [ssgUpgradeMessage, setSSGUpgradeMessage] = useState(null);
	const ssgUpgradeMutation = useMutation({
		mutationFn: () => complianceAPI.upgradeSSG(hostId),
		onSuccess: () => {
			setSSGUpgradeMessage({ type: "success", text: "SSG upgrade command sent! Packages will be upgraded shortly." });
			setTimeout(() => {
				setSSGUpgradeMessage(null);
				refetchStatus(); // Refresh to get new version
			}, 5000);
		},
		onError: (error) => {
			setSSGUpgradeMessage({
				type: "error",
				text: error.response?.data?.error || "Failed to send SSG upgrade command"
			});
			setTimeout(() => setSSGUpgradeMessage(null), 5000);
		},
	});

	// Poll for scan completion when scan is in progress
	useEffect(() => {
		let pollInterval;
		if (scanInProgress) {
			pollInterval = setInterval(() => {
				refetchLatest().then((result) => {
					if (result.data && scanMessage?.startTime) {
						const scanTime = new Date(result.data.completed_at).getTime();
						if (scanTime > scanMessage.startTime) {
							setScanInProgress(false);
							setScanMessage({
								type: "success",
								text: `Scan completed! Score: ${result.data.score?.toFixed(1) || 0}%`,
							});
							refetchHistory();
							// Auto-switch to results tab
							setActiveSubtab("results");
							setTimeout(() => setScanMessage(null), 10000);
						}
					}
				});
			}, 5000);
		}
		return () => clearInterval(pollInterval);
	}, [scanInProgress, scanMessage?.startTime, refetchLatest, refetchHistory]);

	const triggerScan = useMutation({
		mutationFn: (options) => complianceAPI.triggerScan(hostId, options),
		onSuccess: (_, variables) => {
			setScanInProgress(true);
			const remediationText = variables.enableRemediation
				? " Remediation is enabled - failed rules will be automatically fixed."
				: "";
			setScanMessage({
				type: "info",
				text: `Compliance scan started. This may take several minutes...${remediationText}`,
				startTime: Date.now(),
			});
		},
		onError: (error) => {
			const errorMsg = error.response?.data?.error || error.message || "Failed to trigger scan";
			setScanMessage({ type: "error", text: errorMsg });
			setTimeout(() => setScanMessage(null), 5000);
		},
	});

	const toggleRule = (ruleId) => {
		setExpandedRules((prev) => ({
			...prev,
			[ruleId]: !prev[ruleId],
		}));
	};

	const getStatusIcon = (status) => {
		switch (status) {
			case "pass":
				return <CheckCircle className="h-4 w-4 text-green-400" />;
			case "fail":
				return <XCircle className="h-4 w-4 text-red-400" />;
			case "warn":
				return <AlertTriangle className="h-4 w-4 text-yellow-400" />;
			default:
				return <MinusCircle className="h-4 w-4 text-secondary-400" />;
		}
	};

	const getStatusBadge = (status) => {
		const styles = {
			pass: "bg-green-900/30 text-green-400 border-green-700",
			fail: "bg-red-900/30 text-red-400 border-red-700",
			warn: "bg-yellow-900/30 text-yellow-400 border-yellow-700",
			skip: "bg-secondary-700/50 text-secondary-400 border-secondary-600",
			notapplicable: "bg-secondary-700/50 text-secondary-500 border-secondary-600",
		};
		return styles[status] || styles.skip;
	};

	const filteredResults = latestScan?.compliance_results?.filter((r) =>
		statusFilter === "all" ? true : r.status === statusFilter
	) || latestScan?.results?.filter((r) =>
		statusFilter === "all" ? true : r.status === statusFilter
	);

	if (isLoading) {
		return (
			<div className="flex items-center justify-center h-64">
				<div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-500"></div>
			</div>
		);
	}

	// Render Overview subtab
	const renderOverview = () => (
		<div className="space-y-6">
			{/* Latest Scan Summary */}
			{latestScan ? (
				<div className="bg-secondary-800 rounded-lg border border-secondary-700 p-6">
					<div className="flex flex-col lg:flex-row lg:items-center justify-between gap-6">
						<div className="flex-1">
							<div className="flex items-center gap-3 mb-4">
								<Shield className="h-8 w-8 text-primary-400" />
								<div>
									<h3 className="text-lg font-semibold text-white">
										{latestScan.compliance_profiles?.name || latestScan.profile?.name || "Security Scan"}
									</h3>
									<p className="text-sm text-secondary-400">
										Last scanned {new Date(latestScan.completed_at).toLocaleString()}
									</p>
								</div>
							</div>

							{/* Stats Grid */}
							<div className="grid grid-cols-5 gap-3">
								<div className="bg-secondary-700/50 rounded-lg p-3 text-center">
									<p className="text-2xl font-bold text-white">{latestScan.total_rules || 0}</p>
									<p className="text-xs text-secondary-400">Total Rules</p>
								</div>
								<div className="bg-green-900/20 border border-green-800/50 rounded-lg p-3 text-center">
									<p className="text-2xl font-bold text-green-400">{latestScan.passed || 0}</p>
									<p className="text-xs text-secondary-400">Passed</p>
								</div>
								<div className="bg-red-900/20 border border-red-800/50 rounded-lg p-3 text-center">
									<p className="text-2xl font-bold text-red-400">{latestScan.failed || 0}</p>
									<p className="text-xs text-secondary-400">Failed</p>
								</div>
								<div className="bg-yellow-900/20 border border-yellow-800/50 rounded-lg p-3 text-center">
									<p className="text-2xl font-bold text-yellow-400">{latestScan.warnings || 0}</p>
									<p className="text-xs text-secondary-400">Warnings</p>
								</div>
								<div className="bg-secondary-700/50 rounded-lg p-3 text-center">
									<p className="text-2xl font-bold text-secondary-400">
										{(latestScan.skipped || 0) + (latestScan.not_applicable || 0)}
									</p>
									<p className="text-xs text-secondary-400">N/A</p>
								</div>
							</div>
						</div>

						<div className="flex justify-center lg:justify-end">
							<ComplianceScore score={latestScan.score} size="xl" />
						</div>
					</div>

					{latestScan.error_message && (
						<div className="mt-4 p-3 bg-red-900/20 border border-red-800 rounded-lg text-sm text-red-300">
							<strong>Error:</strong> {latestScan.error_message}
						</div>
					)}

					{/* Quick Actions */}
					<div className="mt-6 pt-4 border-t border-secondary-700 flex flex-wrap gap-3">
						<button
							onClick={() => setActiveSubtab("scan")}
							className="flex items-center gap-2 px-4 py-2 bg-primary-600 hover:bg-primary-700 text-white rounded-lg transition-colors"
						>
							<Play className="h-4 w-4" />
							Run New Scan
						</button>
						<button
							onClick={() => setActiveSubtab("results")}
							className="flex items-center gap-2 px-4 py-2 bg-secondary-700 hover:bg-secondary-600 text-white rounded-lg transition-colors"
						>
							<ListChecks className="h-4 w-4" />
							View Details
						</button>
					</div>
				</div>
			) : (
				<div className="bg-secondary-800 rounded-lg border border-secondary-700 p-12 text-center">
					<Shield className="h-16 w-16 text-secondary-600 mx-auto mb-4" />
					<h3 className="text-lg font-medium text-white mb-2">No Compliance Scans Yet</h3>
					<p className="text-secondary-400 mb-6">
						Run a security compliance scan to check this host against CIS benchmarks
					</p>
					<button
						onClick={() => setActiveSubtab("scan")}
						className="inline-flex items-center gap-2 px-6 py-3 bg-primary-600 hover:bg-primary-700 text-white rounded-lg transition-colors"
					>
						<Play className="h-5 w-5" />
						Run First Scan
					</button>
				</div>
			)}

			{/* Compliance Trend Chart */}
			<Suspense fallback={<div className="h-48 bg-secondary-800 rounded-lg border border-secondary-700 animate-pulse" />}>
				<ComplianceTrend hostId={hostId} />
			</Suspense>
		</div>
	);

	// Get available profiles from agent or use defaults
	const scannerInfo = integrationStatus?.status?.scanner_info;
	const availableProfiles = scannerInfo?.available_profiles?.length > 0
		? scannerInfo.available_profiles
		: DEFAULT_SCAN_PROFILES;

	// Render Run Scan subtab
	const renderScanTab = () => (
		<div className="space-y-6">
			{/* Connection Warning */}
			{!isConnected && (
				<div className="p-4 rounded-lg bg-yellow-900/30 border border-yellow-700 text-yellow-200">
					<div className="flex items-center gap-3">
						<AlertTriangle className="h-5 w-5 flex-shrink-0" />
						<div>
							<p className="font-medium">Agent Not Connected</p>
							<p className="text-sm text-yellow-300/80">
								Scans cannot be triggered until the agent reconnects.
							</p>
						</div>
					</div>
				</div>
			)}

			{/* Content Mismatch Warning */}
			{scannerInfo?.content_mismatch && (
				<div className="p-4 rounded-lg bg-orange-900/30 border border-orange-700 text-orange-200">
					<div className="flex items-center gap-3">
						<AlertTriangle className="h-5 w-5 flex-shrink-0" />
						<div>
							<p className="font-medium">Content Version Mismatch</p>
							<p className="text-sm text-orange-300/80">
								{scannerInfo.mismatch_warning || "SCAP content may not match your OS version. Results may show many N/A rules."}
							</p>
						</div>
					</div>
				</div>
			)}

			{/* Scan Message */}
			{scanMessage && (
				<div className={`p-4 rounded-lg flex items-center gap-3 ${
					scanMessage.type === "success"
						? "bg-green-900/50 border border-green-700 text-green-200"
						: scanMessage.type === "info"
							? "bg-blue-900/50 border border-blue-700 text-blue-200"
							: "bg-red-900/50 border border-red-700 text-red-200"
				}`}>
					{scanMessage.type === "info" && <RefreshCw className="h-5 w-5 animate-spin flex-shrink-0" />}
					{scanMessage.type === "success" && <CheckCircle className="h-5 w-5 flex-shrink-0" />}
					{scanMessage.type === "error" && <XCircle className="h-5 w-5 flex-shrink-0" />}
					<span>{scanMessage.text}</span>
				</div>
			)}

			{/* Scan In Progress */}
			{scanInProgress ? (
				<div className="bg-secondary-800 rounded-lg border border-primary-600 p-6">
					<div className="flex items-center gap-4 mb-4">
						<div className="p-3 bg-primary-600/20 rounded-full">
							<RefreshCw className="h-6 w-6 animate-spin text-primary-400" />
						</div>
						<div>
							<h3 className="text-lg font-medium text-white">Scan In Progress</h3>
							<p className="text-sm text-secondary-400">
								Running {availableProfiles.find(p => p.id === selectedProfile)?.name || selectedProfile}
							</p>
						</div>
					</div>
					<div className="w-full bg-secondary-700 rounded-full h-3 mb-3">
						<div className="bg-primary-600 h-3 rounded-full animate-pulse" style={{ width: "60%" }} />
					</div>
					<p className="text-sm text-secondary-400 flex items-center gap-2">
						<Clock className="h-4 w-4" />
						OpenSCAP scans typically take 2-5 minutes depending on system configuration
					</p>
				</div>
			) : (
				<>
					{/* Profile Selection */}
					<div className="bg-secondary-800 rounded-lg border border-secondary-700 p-6">
						<div className="flex items-center justify-between mb-4">
							<h3 className="text-lg font-medium text-white">Select Scan Profile</h3>
							<span className="text-sm text-secondary-400">
								{availableProfiles.length} profiles available
							</span>
						</div>

						{/* Group profiles by category */}
						{(() => {
							const grouped = availableProfiles.reduce((acc, profile) => {
								const cat = profile.category || "other";
								if (!acc[cat]) acc[cat] = [];
								acc[cat].push(profile);
								return acc;
							}, {});

							const categoryLabels = {
								cis: { name: "CIS Benchmarks", color: "text-green-400", bg: "bg-green-900/30" },
								stig: { name: "DISA STIG", color: "text-orange-400", bg: "bg-orange-900/30" },
								"pci-dss": { name: "PCI-DSS", color: "text-purple-400", bg: "bg-purple-900/30" },
								hipaa: { name: "HIPAA", color: "text-blue-400", bg: "bg-blue-900/30" },
								anssi: { name: "ANSSI", color: "text-cyan-400", bg: "bg-cyan-900/30" },
								standard: { name: "Standard", color: "text-yellow-400", bg: "bg-yellow-900/30" },
								other: { name: "Other Profiles", color: "text-secondary-400", bg: "bg-secondary-700/50" },
							};

							return Object.entries(grouped).map(([category, profiles]) => {
								const catInfo = categoryLabels[category] || categoryLabels.other;
								return (
									<div key={category} className="mb-4 last:mb-0">
										<h4 className={`text-sm font-medium mb-2 ${catInfo.color}`}>
											{catInfo.name} ({profiles.length})
										</h4>
										<div className="grid gap-2">
											{profiles.map((profile) => (
												<button
													key={profile.id}
													onClick={() => setSelectedProfile(profile.xccdf_id || profile.id)}
													className={`flex items-center gap-3 p-3 rounded-lg border transition-all text-left ${
														selectedProfile === (profile.xccdf_id || profile.id)
															? "bg-primary-900/30 border-primary-600"
															: "bg-secondary-700/30 border-secondary-600 hover:border-secondary-500"
													}`}
												>
													<div className={`p-1.5 rounded ${
														selectedProfile === (profile.xccdf_id || profile.id)
															? "bg-primary-600/20"
															: catInfo.bg
													}`}>
														{profile.type === "docker-bench" ? (
															<Package className={`h-4 w-4 ${
																selectedProfile === (profile.xccdf_id || profile.id) ? "text-primary-400" : catInfo.color
															}`} />
														) : (
															<Shield className={`h-4 w-4 ${
																selectedProfile === (profile.xccdf_id || profile.id) ? "text-primary-400" : catInfo.color
															}`} />
														)}
													</div>
													<div className="flex-1 min-w-0">
														<p className={`font-medium text-sm truncate ${
															selectedProfile === (profile.xccdf_id || profile.id) ? "text-primary-300" : "text-white"
														}`}>
															{profile.name}
														</p>
														{profile.description && (
															<p className="text-xs text-secondary-400 truncate">{profile.description}</p>
														)}
													</div>
													<div className="flex items-center gap-2">
														<span className={`px-2 py-0.5 text-xs rounded ${
															profile.type === "docker-bench"
																? "bg-blue-900/30 text-blue-400"
																: "bg-green-900/30 text-green-400"
														}`}>
															{profile.type === "docker-bench" ? "Docker" : "SCAP"}
														</span>
														{selectedProfile === (profile.xccdf_id || profile.id) && (
															<CheckCircle className="h-4 w-4 text-primary-400" />
														)}
													</div>
												</button>
											))}
										</div>
									</div>
								);
							});
						})()}
					</div>

					{/* Scan Options */}
					<div className="bg-secondary-800 rounded-lg border border-secondary-700 p-6">
						<h3 className="text-lg font-medium text-white mb-4 flex items-center gap-2">
							<Wrench className="h-5 w-5 text-primary-400" />
							Scan Options
						</h3>

						{/* Remediation Toggle */}
						<div className="space-y-4">
							<div className="flex items-center justify-between p-4 bg-secondary-700/30 rounded-lg border border-secondary-600">
								<div className="flex items-center gap-3">
									<div className={`p-2 rounded-lg ${enableRemediation ? "bg-orange-600/20" : "bg-secondary-600/50"}`}>
										<Wrench className={`h-5 w-5 ${enableRemediation ? "text-orange-400" : "text-secondary-400"}`} />
									</div>
									<div>
										<p className="text-white font-medium">Auto-Remediation</p>
										<p className="text-sm text-secondary-400">
											Automatically fix failed rules during scan
										</p>
									</div>
								</div>
								<button
									onClick={() => setEnableRemediation(!enableRemediation)}
									className="focus:outline-none"
								>
									{enableRemediation ? (
										<ToggleRight className="h-8 w-8 text-orange-400" />
									) : (
										<ToggleLeft className="h-8 w-8 text-secondary-500" />
									)}
								</button>
							</div>

							{enableRemediation && (
								<div className="p-3 bg-orange-900/20 border border-orange-800/50 rounded-lg">
									<div className="flex items-start gap-2">
										<AlertTriangle className="h-4 w-4 text-orange-400 mt-0.5 flex-shrink-0" />
										<div className="text-sm text-orange-200">
											<p className="font-medium">Remediation Warning</p>
											<p className="text-orange-300/80 mt-1">
												This will automatically modify system configuration to fix failed compliance rules.
												Review the profile requirements before enabling. Changes may affect system behavior.
											</p>
										</div>
									</div>
								</div>
							)}
						</div>
					</div>

					{/* Run Scan Button */}
					<div className="bg-secondary-800 rounded-lg border border-secondary-700 p-6">
						<div className="flex flex-col sm:flex-row items-center justify-between gap-4">
							<div>
								<h3 className="text-lg font-medium text-white">Ready to Scan</h3>
								<p className="text-sm text-secondary-400">
									Selected: {availableProfiles.find(p => (p.xccdf_id || p.id) === selectedProfile)?.name || selectedProfile}
									{enableRemediation && (
										<span className="ml-2 px-2 py-0.5 bg-orange-900/30 text-orange-400 rounded text-xs">
											+ Remediation
										</span>
									)}
								</p>
							</div>
							<button
								onClick={() => triggerScan.mutate({
									profileType: selectedProfile,
									enableRemediation: enableRemediation,
								})}
								disabled={!isConnected || triggerScan.isPending}
								className={`flex items-center gap-2 px-6 py-3 ${
									enableRemediation
										? "bg-orange-600 hover:bg-orange-700"
										: "bg-primary-600 hover:bg-primary-700"
								} disabled:bg-secondary-700 disabled:cursor-not-allowed text-white rounded-lg transition-colors font-medium`}
							>
								{triggerScan.isPending ? (
									<RefreshCw className="h-5 w-5 animate-spin" />
								) : enableRemediation ? (
									<Wrench className="h-5 w-5" />
								) : (
									<Play className="h-5 w-5" />
								)}
								{enableRemediation ? "Scan & Remediate" : "Start Scan"}
							</button>
						</div>
					</div>
				</>
			)}

			{/* Last Scan Info */}
			{latestScan && !scanInProgress && (
				<div className="bg-secondary-800/50 rounded-lg border border-secondary-700 p-4">
					<div className="flex items-center justify-between">
						<div className="flex items-center gap-3">
							<ComplianceScore score={latestScan.score} size="sm" />
							<div>
								<p className="text-sm text-white">Last Scan Result</p>
								<p className="text-xs text-secondary-400">
									{new Date(latestScan.completed_at).toLocaleString()}
								</p>
							</div>
						</div>
						<button
							onClick={() => setActiveSubtab("results")}
							className="text-sm text-primary-400 hover:text-primary-300"
						>
							View Results →
						</button>
					</div>
				</div>
			)}
		</div>
	);

	// Render Results subtab
	const renderResults = () => (
		<div className="space-y-4">
			{latestScan ? (
				<>
					{/* Scan Info Header */}
					<div className="bg-secondary-800 rounded-lg border border-secondary-700 p-4">
						<div className="flex items-center justify-between">
							<div className="flex items-center gap-4">
								<ComplianceScore score={latestScan.score} size="md" />
								<div>
									<h3 className="text-white font-medium">
										{latestScan.compliance_profiles?.name || latestScan.profile?.name || "Scan Results"}
									</h3>
									<p className="text-sm text-secondary-400">
										{new Date(latestScan.completed_at).toLocaleString()} •
										{latestScan.total_rules} rules evaluated
									</p>
								</div>
							</div>
							<button
								onClick={() => {
									refetchLatest();
									refetchHistory();
								}}
								className="p-2 hover:bg-secondary-700 rounded-lg transition-colors"
								title="Refresh results"
							>
								<RefreshCw className="h-4 w-4 text-secondary-400" />
							</button>
						</div>
					</div>

					{/* Results Filter */}
					<div className="flex flex-wrap gap-2">
						{["all", "fail", "warn", "pass", "notapplicable"].map((status) => {
							const count = status === "all"
								? (latestScan.compliance_results?.length || latestScan.results?.length || 0)
								: (latestScan.compliance_results || latestScan.results || []).filter(r => r.status === status).length;
							return (
								<button
									key={status}
									onClick={() => setStatusFilter(status)}
									className={`flex items-center gap-2 px-3 py-1.5 rounded-lg text-sm capitalize transition-colors ${
										statusFilter === status
											? "bg-primary-600 text-white"
											: "bg-secondary-700 text-secondary-300 hover:bg-secondary-600"
									}`}
								>
									{status === "notapplicable" ? "N/A" : status}
									<span className={`px-1.5 py-0.5 rounded text-xs ${
										statusFilter === status
											? "bg-primary-500"
											: "bg-secondary-600"
									}`}>
										{count}
									</span>
								</button>
							);
						})}
					</div>

					{/* Results List */}
					{filteredResults && filteredResults.length > 0 ? (
						<div className="bg-secondary-800 rounded-lg border border-secondary-700 divide-y divide-secondary-700">
							{filteredResults.map((result) => (
								<div key={result.id} className="p-4">
									<button
										onClick={() => toggleRule(result.id)}
										className="w-full flex items-center gap-3 text-left"
									>
										{expandedRules[result.id] ? (
											<ChevronDown className="h-4 w-4 text-secondary-400 flex-shrink-0" />
										) : (
											<ChevronRight className="h-4 w-4 text-secondary-400 flex-shrink-0" />
										)}
										{getStatusIcon(result.status)}
										<div className="flex-1 min-w-0">
											<p className="text-white font-medium">
												{result.compliance_rules?.title || result.rule?.title || "Unknown Rule"}
											</p>
											<p className="text-xs text-secondary-400">
												{(result.compliance_rules?.section || result.rule?.section) &&
													`${result.compliance_rules?.section || result.rule?.section} • `}
												{(result.compliance_rules?.severity || result.rule?.severity) && (
													<span className={`capitalize ${
														(result.compliance_rules?.severity || result.rule?.severity) === "critical" ? "text-red-400" :
														(result.compliance_rules?.severity || result.rule?.severity) === "high" ? "text-orange-400" :
														(result.compliance_rules?.severity || result.rule?.severity) === "medium" ? "text-yellow-400" :
														"text-secondary-400"
													}`}>
														{result.compliance_rules?.severity || result.rule?.severity}
													</span>
												)}
											</p>
										</div>
										<span className={`px-2 py-1 rounded text-xs border ${getStatusBadge(result.status)}`}>
											{result.status}
										</span>
									</button>

									{expandedRules[result.id] && (
										<div className="mt-4 ml-8 space-y-3 text-sm border-l-2 border-secondary-600 pl-4">
											{(result.compliance_rules?.description || result.rule?.description) && (
												<div>
													<p className="text-secondary-400 font-medium mb-1">Description</p>
													<p className="text-secondary-300">
														{result.compliance_rules?.description || result.rule?.description}
													</p>
												</div>
											)}
											{result.finding && (
												<div>
													<p className="text-secondary-400 font-medium mb-1">Finding</p>
													<p className="text-secondary-300">{result.finding}</p>
												</div>
											)}
											{(result.compliance_rules?.remediation || result.rule?.remediation || result.remediation) && (
												<div>
													<p className="text-secondary-400 font-medium mb-1">Remediation</p>
													<p className="text-secondary-300">
														{result.compliance_rules?.remediation || result.rule?.remediation || result.remediation}
													</p>
												</div>
											)}
										</div>
									)}
								</div>
							))}
						</div>
					) : (
						<div className="bg-secondary-800 rounded-lg border border-secondary-700 p-8 text-center">
							<p className="text-secondary-400">No {statusFilter !== "all" ? statusFilter : ""} results found</p>
						</div>
					)}
				</>
			) : (
				<div className="bg-secondary-800 rounded-lg border border-secondary-700 p-12 text-center">
					<ListChecks className="h-12 w-12 text-secondary-600 mx-auto mb-4" />
					<h3 className="text-lg font-medium text-white mb-2">No Results Yet</h3>
					<p className="text-secondary-400 mb-4">Run a compliance scan to see detailed results</p>
					<button
						onClick={() => setActiveSubtab("scan")}
						className="inline-flex items-center gap-2 px-4 py-2 bg-primary-600 hover:bg-primary-700 text-white rounded-lg transition-colors"
					>
						<Play className="h-4 w-4" />
						Run Scan
					</button>
				</div>
			)}
		</div>
	);

	// Render History subtab
	const renderHistory = () => (
		<div className="space-y-4">
			{scanHistory?.scans && scanHistory.scans.length > 0 ? (
				<div className="bg-secondary-800 rounded-lg border border-secondary-700 divide-y divide-secondary-700">
					{scanHistory.scans.map((scan, index) => (
						<div
							key={scan.id}
							className={`p-4 ${index === 0 ? "bg-primary-900/10" : ""}`}
						>
							<div className="flex items-center justify-between">
								<div className="flex items-center gap-4">
									<ComplianceScore score={scan.score} size="sm" />
									<div>
										<div className="flex items-center gap-2">
											<p className="text-white font-medium">
												{scan.compliance_profiles?.name || "Compliance Scan"}
											</p>
											{index === 0 && (
												<span className="px-2 py-0.5 text-xs bg-primary-600/30 text-primary-300 rounded">
													Latest
												</span>
											)}
										</div>
										<p className="text-sm text-secondary-400">
											{new Date(scan.completed_at).toLocaleString()}
										</p>
									</div>
								</div>
								<div className="text-right">
									<div className="flex items-center gap-3 text-sm">
										<span className="text-green-400">{scan.passed} passed</span>
										<span className="text-red-400">{scan.failed} failed</span>
										{scan.warnings > 0 && (
											<span className="text-yellow-400">{scan.warnings} warn</span>
										)}
									</div>
									<p className="text-xs text-secondary-500">
										{scan.total_rules} total rules
									</p>
								</div>
							</div>
							{scan.error_message && (
								<div className="mt-2 p-2 bg-red-900/20 border border-red-800 rounded text-sm text-red-300">
									{scan.error_message}
								</div>
							)}
						</div>
					))}
				</div>
			) : (
				<div className="bg-secondary-800 rounded-lg border border-secondary-700 p-12 text-center">
					<History className="h-12 w-12 text-secondary-600 mx-auto mb-4" />
					<h3 className="text-lg font-medium text-white mb-2">No Scan History</h3>
					<p className="text-secondary-400 mb-4">Previous scans will appear here</p>
					<button
						onClick={() => setActiveSubtab("scan")}
						className="inline-flex items-center gap-2 px-4 py-2 bg-primary-600 hover:bg-primary-700 text-white rounded-lg transition-colors"
					>
						<Play className="h-4 w-4" />
						Run First Scan
					</button>
				</div>
			)}

			{/* Trend Chart */}
			{scanHistory?.scans && scanHistory.scans.length > 1 && (
				<Suspense fallback={<div className="h-48 bg-secondary-800 rounded-lg border border-secondary-700 animate-pulse" />}>
					<ComplianceTrend hostId={hostId} />
				</Suspense>
			)}
		</div>
	);

	// Render Settings subtab
	const renderSettings = () => {
		const status = integrationStatus?.status;
		const components = status?.components || {};
		const info = status?.scanner_info;

		return (
			<div className="space-y-6">
				{/* Scanner Status */}
				<div className="bg-secondary-800 rounded-lg border border-secondary-700 p-6">
					<div className="flex items-center justify-between mb-4">
						<h3 className="text-lg font-medium text-white flex items-center gap-2">
							<Shield className="h-5 w-5 text-primary-400" />
							Scanner Status
						</h3>
						<button
							onClick={() => refetchStatus()}
							disabled={isRefreshingStatus}
							className={`p-2 hover:bg-secondary-700 rounded-lg transition-colors ${isRefreshingStatus ? "cursor-wait" : ""}`}
							title={isRefreshingStatus ? "Refreshing..." : "Refresh status"}
						>
							<RefreshCw className={`h-4 w-4 ${isRefreshingStatus ? "text-primary-400 animate-spin" : "text-secondary-400"}`} />
						</button>
					</div>

					{status ? (
						<div className="space-y-4">
							{/* Overall Status */}
							<div className="flex items-center gap-3 p-3 bg-secondary-700/50 rounded-lg">
								{status.status === "ready" ? (
									<CheckCircle className="h-5 w-5 text-green-400" />
								) : status.status === "installing" ? (
									<RefreshCw className="h-5 w-5 text-blue-400 animate-spin" />
								) : status.status === "error" ? (
									<XCircle className="h-5 w-5 text-red-400" />
								) : (
									<MinusCircle className="h-5 w-5 text-secondary-400" />
								)}
								<div className="flex-1">
									<p className="text-white font-medium capitalize">{status.status || "Unknown"}</p>
									{status.message && (
										<p className="text-sm text-secondary-400">{status.message}</p>
									)}
								</div>
							</div>

							{/* Content Mismatch Warning */}
							{info?.content_mismatch && (
								<div className="p-3 bg-orange-900/30 border border-orange-700 rounded-lg">
									<div className="flex items-center gap-2 text-orange-300">
										<AlertTriangle className="h-4 w-4" />
										<span className="font-medium">Content Version Mismatch</span>
									</div>
									<p className="text-sm text-orange-200/80 mt-1">
										{info.mismatch_warning}
									</p>
								</div>
							)}

							{/* Scanner Details Grid */}
							<div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
								{/* OpenSCAP Details */}
								<div className="bg-secondary-700/30 rounded-lg p-4 border border-secondary-600">
									<div className="flex items-center gap-3 mb-4">
										<div className="p-2 bg-primary-600/20 rounded-lg">
											<Shield className="h-5 w-5 text-primary-400" />
										</div>
										<div>
											<p className="text-white font-medium">OpenSCAP Scanner</p>
											<p className="text-xs text-secondary-400">CIS Benchmark Scanning</p>
										</div>
									</div>
									<div className="space-y-2 text-sm">
										<div className="flex justify-between">
											<span className="text-secondary-400">Status</span>
											<span className={`capitalize ${
												components.openscap === "ready" ? "text-green-400" :
												components.openscap === "installing" ? "text-blue-400" :
												components.openscap === "error" ? "text-red-400" :
												"text-secondary-400"
											}`}>
												{components.openscap || "Not installed"}
											</span>
										</div>
										<div className="flex justify-between">
											<span className="text-secondary-400">Version</span>
											<span className="text-secondary-300 font-mono text-xs">
												{info?.openscap_version || "N/A"}
											</span>
										</div>
										<div className="flex justify-between">
											<span className="text-secondary-400">Content Package</span>
											<span className="text-secondary-300 font-mono text-xs">
												{info?.content_package || "N/A"}
											</span>
										</div>
										<div className="flex justify-between">
											<span className="text-secondary-400">Content File</span>
											<span className="text-secondary-300 font-mono text-xs truncate max-w-[180px]" title={info?.content_file}>
												{info?.content_file || "N/A"}
											</span>
										</div>
										{info?.ssg_version && (
											<div className="flex justify-between">
												<span className="text-secondary-400">SSG Version</span>
												<span className={`font-mono text-xs ${info?.ssg_needs_upgrade ? "text-yellow-400" : "text-secondary-300"}`}>
													{info.ssg_version}{info?.ssg_needs_upgrade && ` (min: ${info.ssg_min_version})`}
												</span>
											</div>
										)}
										{info?.ssg_needs_upgrade && (
											<div className="mt-3 p-2 bg-yellow-600/20 border border-yellow-600/40 rounded-lg">
												<p className="text-yellow-400 text-xs mb-2">
													{info.ssg_upgrade_message || "SSG content upgrade recommended"}
												</p>
												<button
													onClick={() => ssgUpgradeMutation.mutate()}
													disabled={ssgUpgradeMutation.isPending}
													className="w-full flex items-center justify-center gap-2 px-3 py-1.5 bg-yellow-600/30 hover:bg-yellow-600/50 text-yellow-300 text-xs rounded transition-colors disabled:opacity-50"
												>
													{ssgUpgradeMutation.isPending ? (
														<>
															<RefreshCw className="h-3 w-3 animate-spin" />
															Upgrading...
														</>
													) : (
														<>
															<Download className="h-3 w-3" />
															Upgrade SSG Content
														</>
													)}
												</button>
											</div>
										)}
										{ssgUpgradeMessage && (
											<div className={`mt-2 p-2 rounded text-xs ${
												ssgUpgradeMessage.type === "success"
													? "bg-green-600/20 text-green-400 border border-green-600/40"
													: "bg-red-600/20 text-red-400 border border-red-600/40"
											}`}>
												{ssgUpgradeMessage.text}
											</div>
										)}
									</div>
								</div>

								{/* OS Info */}
								<div className="bg-secondary-700/30 rounded-lg p-4 border border-secondary-600">
									<div className="flex items-center gap-3 mb-4">
										<div className="p-2 bg-green-600/20 rounded-lg">
											<Server className="h-5 w-5 text-green-400" />
										</div>
										<div>
											<p className="text-white font-medium">System Information</p>
											<p className="text-xs text-secondary-400">Detected OS Details</p>
										</div>
									</div>
									<div className="space-y-2 text-sm">
										<div className="flex justify-between">
											<span className="text-secondary-400">OS Name</span>
											<span className="text-secondary-300 capitalize">
												{info?.os_name || "N/A"}
											</span>
										</div>
										<div className="flex justify-between">
											<span className="text-secondary-400">Version</span>
											<span className="text-secondary-300">
												{info?.os_version || "N/A"}
											</span>
										</div>
										<div className="flex justify-between">
											<span className="text-secondary-400">Family</span>
											<span className="text-secondary-300 capitalize">
												{info?.os_family || "N/A"}
											</span>
										</div>
									</div>
								</div>

								{/* Docker Bench Component */}
								<div className="bg-secondary-700/30 rounded-lg p-4 border border-secondary-600">
									<div className="flex items-center gap-3 mb-4">
										<div className="p-2 bg-blue-600/20 rounded-lg">
											<Package className="h-5 w-5 text-blue-400" />
										</div>
										<div>
											<p className="text-white font-medium">Docker Bench</p>
											<p className="text-xs text-secondary-400">Docker Security Scanning</p>
										</div>
									</div>
									<div className="space-y-2 text-sm">
										<div className="flex justify-between">
											<span className="text-secondary-400">Status</span>
											<span className={`capitalize ${
												components["docker-bench"] === "ready" ? "text-green-400" :
												components["docker-bench"] === "installing" ? "text-blue-400" :
												components["docker-bench"] === "unavailable" ? "text-secondary-500" :
												components["docker-bench"] === "error" ? "text-red-400" :
												"text-secondary-400"
											}`}>
												{components["docker-bench"] || "Not configured"}
											</span>
										</div>
										<div className="flex justify-between">
											<span className="text-secondary-400">Available</span>
											<span className={info?.docker_bench_available ? "text-green-400" : "text-secondary-500"}>
												{info?.docker_bench_available ? "Yes" : "No"}
											</span>
										</div>
										<div className="flex justify-between">
											<span className="text-secondary-400">Requirement</span>
											<span className="text-secondary-300 text-xs">Docker Integration enabled</span>
										</div>
									</div>
								</div>

								{/* Available Profiles */}
								<div className="bg-secondary-700/30 rounded-lg p-4 border border-secondary-600">
									<div className="flex items-center gap-3 mb-4">
										<div className="p-2 bg-purple-600/20 rounded-lg">
											<ListChecks className="h-5 w-5 text-purple-400" />
										</div>
										<div>
											<p className="text-white font-medium">Available Profiles</p>
											<p className="text-xs text-secondary-400">Scan options from agent</p>
										</div>
									</div>
									<div className="space-y-2">
										{info?.available_profiles?.length > 0 ? (
											info.available_profiles.map((profile, idx) => (
												<div key={idx} className="flex items-center justify-between text-sm">
													<span className="text-secondary-300">{profile.name}</span>
													<span className={`px-2 py-0.5 text-xs rounded ${
														profile.type === "docker-bench"
															? "bg-blue-900/30 text-blue-400"
															: "bg-green-900/30 text-green-400"
													}`}>
														{profile.type}
													</span>
												</div>
											))
										) : (
											<p className="text-secondary-500 text-sm">No profiles available</p>
										)}
									</div>
								</div>
							</div>

							{/* Last Updated */}
							{status.timestamp && (
								<p className="text-xs text-secondary-500 text-right">
									Last updated: {new Date(status.timestamp).toLocaleString()}
								</p>
							)}
						</div>
					) : (
						<div className="text-center py-8">
							<Info className="h-12 w-12 text-secondary-600 mx-auto mb-3" />
							<p className="text-secondary-400">No scanner status available</p>
							<p className="text-sm text-secondary-500 mt-1">
								Enable compliance integration to see scanner details
							</p>
						</div>
					)}
				</div>

				{/* Information Section */}
				<div className="bg-secondary-800 rounded-lg border border-secondary-700 p-6">
					<h3 className="text-lg font-medium text-white flex items-center gap-2 mb-4">
						<Info className="h-5 w-5 text-primary-400" />
						About Compliance Scanning
					</h3>
					<div className="space-y-4 text-sm text-secondary-300">
						<p>
							PatchMon uses industry-standard compliance scanning tools to evaluate your
							systems against security benchmarks.
						</p>
						<div className="grid grid-cols-1 md:grid-cols-2 gap-4">
							<div className="p-3 bg-secondary-700/30 rounded-lg">
								<p className="text-white font-medium mb-1">OpenSCAP (oscap)</p>
								<p className="text-secondary-400 text-xs">
									Scans against CIS Benchmarks for Linux distributions.
									Evaluates system configuration, file permissions, and security settings.
								</p>
							</div>
							<div className="p-3 bg-secondary-700/30 rounded-lg">
								<p className="text-white font-medium mb-1">Docker Bench for Security</p>
								<p className="text-secondary-400 text-xs">
									Checks Docker host and container configurations against
									CIS Docker Benchmark recommendations.
								</p>
							</div>
						</div>
					</div>
				</div>

				{/* Agent Update */}
				<div className="bg-secondary-800 rounded-lg border border-secondary-700 p-6">
					<h3 className="text-lg font-medium text-white flex items-center gap-2 mb-4">
						<Download className="h-5 w-5 text-primary-400" />
						Agent Update
					</h3>
					<div className="space-y-4">
						<p className="text-sm text-secondary-300">
							Force the agent to check for and download the latest version from GitHub.
						</p>
						{updateMessage && (
							<div className={`p-3 rounded-lg ${
								updateMessage.type === "success"
									? "bg-green-900/30 border border-green-700 text-green-300"
									: "bg-red-900/30 border border-red-700 text-red-300"
							}`}>
								{updateMessage.text}
							</div>
						)}
						<button
							onClick={() => agentUpdateMutation.mutate()}
							disabled={!isConnected || agentUpdateMutation.isPending}
							className={`flex items-center gap-2 px-4 py-2 rounded-lg font-medium transition-colors ${
								!isConnected
									? "bg-secondary-700 text-secondary-500 cursor-not-allowed"
									: agentUpdateMutation.isPending
									? "bg-primary-600/50 text-white cursor-wait"
									: "bg-primary-600 hover:bg-primary-500 text-white"
							}`}
						>
							{agentUpdateMutation.isPending ? (
								<RefreshCw className="h-4 w-4 animate-spin" />
							) : (
								<Download className="h-4 w-4" />
							)}
							{agentUpdateMutation.isPending ? "Sending Update Command..." : "Update Agent Now"}
						</button>
						{!isConnected && (
							<p className="text-xs text-secondary-500">Agent must be connected to trigger updates</p>
						)}
					</div>
				</div>

				{/* CIS Level Guide */}
				<div className="bg-secondary-800 rounded-lg border border-secondary-700 p-6">
					<h3 className="text-lg font-medium text-white flex items-center gap-2 mb-4">
						<BookOpen className="h-5 w-5 text-primary-400" />
						CIS Benchmark Levels Guide
					</h3>
					<div className="space-y-4">
						<div className="p-4 bg-green-900/20 border border-green-800/50 rounded-lg">
							<h4 className="text-green-400 font-medium mb-2">Level 1 - Essential Security</h4>
							<ul className="text-sm text-green-200/80 space-y-1 list-disc list-inside">
								<li>Practical security measures with minimal service disruption</li>
								<li>Suitable for most production environments</li>
								<li>Covers essential hardening: password policies, file permissions, network settings</li>
								<li>Recommended as baseline for all systems</li>
							</ul>
						</div>
						<div className="p-4 bg-orange-900/20 border border-orange-800/50 rounded-lg">
							<h4 className="text-orange-400 font-medium mb-2">Level 2 - Defense in Depth</h4>
							<ul className="text-sm text-orange-200/80 space-y-1 list-disc list-inside">
								<li>Extended security for high-security environments</li>
								<li>May impact functionality - test before applying</li>
								<li>Includes stricter controls: audit logging, kernel hardening, additional restrictions</li>
								<li>Recommended for systems handling sensitive data</li>
							</ul>
						</div>
						<div className="p-4 bg-purple-900/20 border border-purple-800/50 rounded-lg">
							<h4 className="text-purple-400 font-medium mb-2">Other Profiles (STIG, PCI-DSS, HIPAA)</h4>
							<ul className="text-sm text-purple-200/80 space-y-1 list-disc list-inside">
								<li><strong>STIG:</strong> DoD Security Technical Implementation Guides</li>
								<li><strong>PCI-DSS:</strong> Payment Card Industry Data Security Standard</li>
								<li><strong>HIPAA:</strong> Health Insurance Portability and Accountability Act</li>
								<li>These profiles target specific compliance requirements</li>
							</ul>
						</div>
					</div>
				</div>

				{/* Troubleshooting */}
				<div className="bg-yellow-900/20 border border-yellow-800/50 rounded-lg p-4">
					<h4 className="text-yellow-300 font-medium flex items-center gap-2 mb-2">
						<AlertTriangle className="h-4 w-4" />
						Troubleshooting
					</h4>
					<ul className="text-sm text-yellow-200/80 space-y-1 list-disc list-inside">
						<li>If scans show all "N/A" results, the SCAP content may not match your OS version</li>
						<li>Try disabling and re-enabling compliance to upgrade packages</li>
						<li>Docker Bench requires Docker integration to be enabled first</li>
					</ul>
				</div>

				{/* Ubuntu 24.04 Notice */}
				<div className="bg-blue-900/20 border border-blue-800/50 rounded-lg p-4">
					<h4 className="text-blue-300 font-medium flex items-center gap-2 mb-2">
						<Info className="h-4 w-4" />
						Ubuntu 24.04 Users
					</h4>
					<div className="text-sm text-blue-200/80 space-y-2">
						<p>
							<strong>CIS/STIG content for Ubuntu 24.04</strong> is available in SCAP Security Guide v0.1.76+.
							If you see "content mismatch" warnings, your ssg-base package needs updating.
						</p>
						<p>Options for Ubuntu 24.04 compliance:</p>
						<ul className="list-disc list-inside ml-2 space-y-1">
							<li><strong>Update ssg-base package</strong> to v0.1.76 or higher</li>
							<li><strong>Canonical's Ubuntu Security Guide (USG)</strong> - Official CIS hardening with Ubuntu Pro</li>
							<li><strong>OVAL Vulnerability Scanning</strong> - Free CVE content from Canonical</li>
						</ul>
						<p className="mt-2 text-xs text-blue-300/70">
							Note: USG provides <code className="bg-blue-800/50 px-1 rounded">usg audit</code> and <code className="bg-blue-800/50 px-1 rounded">usg fix</code> commands for CIS Level 1/2 hardening.
						</p>
					</div>
				</div>
			</div>
		);
	};

	return (
		<div className="space-y-4">
			{/* Header */}
			<div className="flex items-center gap-3">
				<Shield className="h-6 w-6 text-primary-400" />
				<h2 className="text-xl font-semibold text-white">Security Compliance</h2>
			</div>

			{/* Subtab Navigation */}
			<div className="flex gap-1 p-1 bg-secondary-800 rounded-lg border border-secondary-700">
				{SUBTABS.map((tab) => {
					const TabIcon = tab.icon;
					return (
						<button
							key={tab.id}
							onClick={() => setActiveSubtab(tab.id)}
							className={`flex items-center gap-2 px-4 py-2 rounded-md text-sm font-medium transition-colors flex-1 justify-center ${
								activeSubtab === tab.id
									? "bg-primary-600 text-white"
									: "text-secondary-400 hover:text-white hover:bg-secondary-700"
							}`}
						>
							<TabIcon className="h-4 w-4" />
							<span className="hidden sm:inline">{tab.name}</span>
						</button>
					);
				})}
			</div>

			{/* Subtab Content */}
			<div className="min-h-[400px]">
				{activeSubtab === "overview" && renderOverview()}
				{activeSubtab === "scan" && renderScanTab()}
				{activeSubtab === "results" && renderResults()}
				{activeSubtab === "history" && renderHistory()}
				{activeSubtab === "settings" && renderSettings()}
			</div>
		</div>
	);
};

export default ComplianceTab;
