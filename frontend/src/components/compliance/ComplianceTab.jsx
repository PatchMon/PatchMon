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
	Search,
	Container,
	Box,
} from "lucide-react";
import { complianceAPI } from "../../utils/complianceApi";
import ComplianceScore from "./ComplianceScore";
import { PieChart, Pie, Cell, BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer } from "recharts";

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

const ComplianceTab = ({ hostId, apiId, isConnected }) => {
	const [activeSubtab, setActiveSubtab] = useState("overview");
	const [expandedRules, setExpandedRules] = useState({});
	const [statusFilter, setStatusFilter] = useState("fail");
	const [severityFilter, setSeverityFilter] = useState("all"); // Filter by severity within Failed tab
	const [ruleSearch, setRuleSearch] = useState(""); // Search rules by title/id
	const [selectedProfile, setSelectedProfile] = useState("level1_server");
	const [enableRemediation, setEnableRemediation] = useState(false);
	const [remediatingRule, setRemediatingRule] = useState(null);
	const [scanProgress, setScanProgress] = useState(null); // Real-time progress from SSE
	const [dockerImageName, setDockerImageName] = useState(""); // Docker image name for CVE scan
	const [scanAllDockerImages, setScanAllDockerImages] = useState(true); // Scan all Docker images
	const queryClient = useQueryClient();

	// Persist scan state in sessionStorage to survive tab switches
	const scanStateKey = `compliance-scan-${hostId}`;
	const [scanInProgress, setScanInProgressState] = useState(() => {
		try {
			const saved = sessionStorage.getItem(scanStateKey);
			if (saved) {
				const parsed = JSON.parse(saved);
				// Check if scan started less than 10 minutes ago
				if (parsed.startTime && Date.now() - parsed.startTime < 10 * 60 * 1000) {
					return true;
				}
				// Clear stale scan state
				sessionStorage.removeItem(scanStateKey);
			}
		} catch (e) {
			// Ignore parsing errors
		}
		return false;
	});
	const [scanMessage, setScanMessageState] = useState(() => {
		try {
			const saved = sessionStorage.getItem(scanStateKey);
			if (saved) {
				const parsed = JSON.parse(saved);
				if (parsed.startTime && Date.now() - parsed.startTime < 10 * 60 * 1000) {
					return parsed;
				}
			}
		} catch (e) {
			// Ignore parsing errors
		}
		return null;
	});

	// Wrapper to persist scan state
	const setScanInProgress = (value) => {
		setScanInProgressState(value);
		if (!value) {
			sessionStorage.removeItem(scanStateKey);
		}
	};
	const setScanMessage = (msg, profile = null) => {
		setScanMessageState(msg);
		if (msg && msg.startTime) {
			const toSave = { ...msg };
			if (profile) toSave.profileName = profile;
			sessionStorage.setItem(scanStateKey, JSON.stringify(toSave));
		} else if (!msg) {
			sessionStorage.removeItem(scanStateKey);
		}
	};

	const { data: latestScan, isLoading, refetch: refetchLatest } = useQuery({
		queryKey: ["compliance-latest", hostId],
		queryFn: () => complianceAPI.getLatestScan(hostId).then((res) => res.data),
		enabled: !!hostId,
		staleTime: 30 * 1000, // Consider data fresh for 30 seconds
		refetchOnWindowFocus: false, // Don't refetch on window focus to avoid flicker
		placeholderData: (previousData) => previousData, // Keep previous data during refetch
	});

	const { data: scanHistory, refetch: refetchHistory } = useQuery({
		queryKey: ["compliance-history", hostId],
		queryFn: () => complianceAPI.getHostScans(hostId, { limit: 10 }).then((res) => res.data),
		enabled: !!hostId,
		staleTime: 30 * 1000,
		refetchOnWindowFocus: false,
		placeholderData: (previousData) => previousData,
	});

	// Get integration status (scanner info, components)
	const { data: integrationStatus, refetch: refetchStatus, isFetching: isRefreshingStatus } = useQuery({
		queryKey: ["compliance-status", hostId],
		queryFn: () => complianceAPI.getIntegrationStatus(hostId).then((res) => res.data),
		enabled: !!hostId,
		refetchInterval: 30000, // Refresh every 30 seconds
	});

	// Update selected profile when agent profiles are loaded
	useEffect(() => {
		const agentProfiles = integrationStatus?.status?.scanner_info?.available_profiles;
		if (agentProfiles?.length > 0) {
			// If current selection isn't in the agent's available profiles, select the first one
			const currentInList = agentProfiles.some(p => (p.xccdf_id || p.id) === selectedProfile);
			if (!currentInList) {
				const firstProfile = agentProfiles[0];
				setSelectedProfile(firstProfile.xccdf_id || firstProfile.id);
			}
		}
	}, [integrationStatus?.status?.scanner_info?.available_profiles]);

	// SSG content update mutation
	const [updateMessage, setUpdateMessage] = useState(null);
	const ssgUpdateMutation = useMutation({
		mutationFn: () => {
			if (!hostId) {
				return Promise.reject(new Error("No host ID available"));
			}
			return complianceAPI.upgradeSSG(hostId);
		},
		onSuccess: () => {
			setUpdateMessage({ type: "success", text: "SSG update command sent! Security content will be updated shortly." });
			setTimeout(() => {
				setUpdateMessage(null);
				refetchStatus();
			}, 5000);
		},
		onError: (error) => {
			console.error("SSG update error:", error);
			const errorMsg = error.response?.data?.error
				|| error.message
				|| "Failed to send SSG update command";
			setUpdateMessage({
				type: "error",
				text: errorMsg
			});
			setTimeout(() => setUpdateMessage(null), 8000);
		},
	});

	// SSG upgrade mutation
	const [ssgUpgradeMessage, setSSGUpgradeMessage] = useState(null);
	const ssgUpgradeMutation = useMutation({
		mutationFn: () => complianceAPI.upgradeSSG(hostId),
		onSuccess: () => {
			setSSGUpgradeMessage({ type: "success", text: "Upgrading SSG content from GitHub... This may take 10-15 seconds." });
			// First refresh after 8 seconds (download + extract takes ~6-7s)
			setTimeout(() => {
				refetchStatus();
			}, 8000);
			// Second refresh after 12 seconds to catch any stragglers
			setTimeout(() => {
				setSSGUpgradeMessage(null);
				refetchStatus();
			}, 12000);
		},
		onError: (error) => {
			setSSGUpgradeMessage({
				type: "error",
				text: error.response?.data?.error || "Failed to send SSG upgrade command"
			});
			setTimeout(() => setSSGUpgradeMessage(null), 5000);
		},
	});

	// Single rule remediation mutation with enhanced feedback
	const [remediationStatus, setRemediationStatus] = useState(null); // { phase: 'sending'|'running'|'complete'|'error', rule: string, message: string }
	const remediateRuleMutation = useMutation({
		mutationFn: (ruleId) => complianceAPI.remediateRule(hostId, ruleId),
		onMutate: (ruleId) => {
			setRemediatingRule(ruleId);
			setRemediationStatus({ phase: "sending", rule: ruleId, message: "Sending remediation command to agent..." });
		},
		onSuccess: (_, ruleId) => {
			setRemediationStatus({ phase: "running", rule: ruleId, message: "Agent is applying the fix. This may take a moment..." });
			// Show running status for a few seconds, then complete
			setTimeout(() => {
				setRemediationStatus({ phase: "complete", rule: ruleId, message: "Fix applied! Run a new scan to verify the change." });
				setRemediatingRule(null);
				// Clear the status and refresh after showing success
				setTimeout(() => {
					setRemediationStatus(null);
					refetchLatest();
				}, 4000);
			}, 3000);
		},
		onError: (error) => {
			setRemediationStatus({
				phase: "error",
				rule: remediatingRule,
				message: error.response?.data?.error || "Failed to remediate rule"
			});
			setRemediatingRule(null);
			setTimeout(() => setRemediationStatus(null), 6000);
		},
	});

	// Elapsed time for scan in progress
	const [elapsedTime, setElapsedTime] = useState(0);

	// Update elapsed time every second when scan is in progress
	useEffect(() => {
		let timer;
		if (scanInProgress && scanMessage?.startTime) {
			timer = setInterval(() => {
				setElapsedTime(Math.floor((Date.now() - scanMessage.startTime) / 1000));
			}, 1000);
		} else {
			setElapsedTime(0);
		}
		return () => clearInterval(timer);
	}, [scanInProgress, scanMessage?.startTime]);

	// Poll for scan completion when scan is in progress (less frequent to reduce flicker)
	useEffect(() => {
		let pollInterval;
		if (scanInProgress) {
			pollInterval = setInterval(async () => {
				try {
					const result = await refetchLatest();
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
				} catch (error) {
					// Silently ignore polling errors to prevent flicker
					console.debug("Scan poll error:", error);
				}
			}, 10000); // Poll every 10 seconds instead of 5 to reduce load
		}
		return () => clearInterval(pollInterval);
	}, [scanInProgress, scanMessage?.startTime, refetchLatest, refetchHistory]);

	// SSE connection for real-time compliance scan progress
	useEffect(() => {
		if (!scanInProgress || !apiId) {
			setScanProgress(null);
			return;
		}

		// Connect to SSE - authentication is handled via httpOnly cookies automatically
		console.log("[Compliance SSE] Connecting for api_id:", apiId);
		const eventSource = new EventSource(
			`/api/v1/ws/compliance-progress/${apiId}/stream`,
			{ withCredentials: true }
		);

		eventSource.onmessage = (event) => {
			try {
				const data = JSON.parse(event.data);
				console.log("[Compliance SSE] Progress:", data);
				setScanProgress(data);

				// If scan completed or failed, update UI
				if (data.phase === "completed" || data.phase === "failed") {
					// Give a moment for the message to be displayed, then clear progress
					setTimeout(() => setScanProgress(null), 5000);
				}
			} catch (err) {
				console.warn("[Compliance SSE] Failed to parse event:", err);
			}
		};

		eventSource.onerror = (err) => {
			console.warn("[Compliance SSE] Connection error:", err);
			// EventSource will auto-reconnect, no action needed
		};

		return () => {
			console.log("[Compliance SSE] Disconnecting");
			eventSource.close();
		};
	}, [scanInProgress, apiId]);

	const triggerScan = useMutation({
		mutationFn: (options) => complianceAPI.triggerScan(hostId, options),
		onSuccess: (_, variables) => {
			setScanInProgress(true);
			const remediationText = variables.enableRemediation
				? " Remediation is enabled - failed rules will be automatically fixed."
				: "";
			// Get profile name for display
			const profileName = availableProfiles.find(p =>
				(p.xccdf_id || p.id) === variables.profileId
			)?.name || variables.profileId;
			setScanMessage({
				type: "info",
				text: `Compliance scan started. This may take several minutes...${remediationText}`,
				startTime: Date.now(),
				profileName: profileName,
			}, profileName);
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
			{/* Version Mismatch Warnings */}
			{scannerInfo?.content_mismatch && (
				<div className="p-4 rounded-lg bg-orange-900/30 border border-orange-700 text-orange-200">
					<div className="flex items-start gap-3">
						<AlertTriangle className="h-5 w-5 flex-shrink-0 mt-0.5" />
						<div className="flex-1">
							<p className="font-medium">Content Version Mismatch</p>
							<p className="text-sm text-orange-300/80 mt-1">
								{scannerInfo.mismatch_warning || "SCAP content may not match your OS version. Results may show many N/A rules."}
							</p>
						</div>
						<button
							onClick={() => setActiveSubtab("settings")}
							className="flex items-center gap-1.5 px-3 py-1.5 bg-orange-600/30 hover:bg-orange-600/50 text-orange-200 text-sm rounded-lg transition-colors"
						>
							<Settings className="h-4 w-4" />
							Update
						</button>
					</div>
				</div>
			)}

			{/* Scan In Progress Banner */}
			{scanInProgress && (
				<div className="p-4 rounded-lg bg-primary-900/30 border border-primary-600 text-primary-200">
					<div className="flex items-center justify-between">
						<div className="flex items-center gap-3">
							<RefreshCw className="h-5 w-5 animate-spin text-primary-400" />
							<div>
								<p className="font-medium">Compliance Scan In Progress</p>
								<p className="text-sm text-primary-300/80">
									{scanProgress?.message
										? scanProgress.message
										: scanMessage?.profileName
											? `Running ${scanMessage.profileName}...`
											: "Scan is running, please wait..."}
								</p>
							</div>
						</div>
						<div className="flex items-center gap-4">
							<div className="text-right">
								<p className="text-xl font-mono font-bold text-primary-400">
									{Math.floor(elapsedTime / 60)}:{(elapsedTime % 60).toString().padStart(2, '0')}
								</p>
								<p className="text-xs text-primary-400/60">elapsed</p>
							</div>
							<button
								onClick={() => setActiveSubtab("scan")}
								className="flex items-center gap-1.5 px-3 py-1.5 bg-primary-600/30 hover:bg-primary-600/50 text-primary-200 text-sm rounded-lg transition-colors"
							>
								<Play className="h-4 w-4" />
								View
							</button>
						</div>
					</div>
				</div>
			)}

			{scannerInfo?.ssg_needs_upgrade && !scannerInfo?.content_mismatch && (
				<div className="p-4 rounded-lg bg-yellow-900/30 border border-yellow-700 text-yellow-200">
					<div className="flex items-start gap-3">
						<AlertTriangle className="h-5 w-5 flex-shrink-0 mt-0.5" />
						<div className="flex-1">
							<p className="font-medium">SSG Content Update Available</p>
							<p className="text-sm text-yellow-300/80 mt-1">
								{scannerInfo.ssg_upgrade_message || `Current version ${scannerInfo.ssg_version} is below minimum ${scannerInfo.ssg_min_version}. Update recommended for accurate compliance results.`}
							</p>
						</div>
						<button
							onClick={() => setActiveSubtab("settings")}
							className="flex items-center gap-1.5 px-3 py-1.5 bg-yellow-600/30 hover:bg-yellow-600/50 text-yellow-200 text-sm rounded-lg transition-colors"
						>
							<Download className="h-4 w-4" />
							Update
						</button>
					</div>
				</div>
			)}

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

							{/* Stats Grid - Use actual results for accurate counts */}
							{(() => {
								const results = latestScan.compliance_results || latestScan.results || [];
								const counts = {
									total: results.length,
									pass: results.filter(r => r.status === "pass").length,
									fail: results.filter(r => r.status === "fail").length,
									warn: results.filter(r => r.status === "warn").length,
									skip: results.filter(r => r.status === "skip").length,
									notapplicable: results.filter(r => r.status === "notapplicable").length,
								};
								return (
									<div className="grid grid-cols-5 gap-3">
										<div className="bg-secondary-700/50 rounded-lg p-3 text-center">
											<p className="text-2xl font-bold text-white">{counts.total}</p>
											<p className="text-xs text-secondary-400">Total Rules</p>
										</div>
										<div className="bg-green-900/20 border border-green-800/50 rounded-lg p-3 text-center">
											<p className="text-2xl font-bold text-green-400">{counts.pass}</p>
											<p className="text-xs text-secondary-400">Passed</p>
										</div>
										<div className="bg-red-900/20 border border-red-800/50 rounded-lg p-3 text-center">
											<p className="text-2xl font-bold text-red-400">{counts.fail}</p>
											<p className="text-xs text-secondary-400">Failed</p>
										</div>
										<div className="bg-yellow-900/20 border border-yellow-800/50 rounded-lg p-3 text-center">
											<p className="text-2xl font-bold text-yellow-400">{counts.warn}</p>
											<p className="text-xs text-secondary-400">Warnings</p>
										</div>
										<div className="bg-secondary-700/50 rounded-lg p-3 text-center">
											<p className="text-2xl font-bold text-secondary-400">
												{counts.skip + counts.notapplicable}
											</p>
											<p className="text-xs text-secondary-400">N/A</p>
										</div>
									</div>
								);
							})()}
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

			{/* Charts Section */}
			{latestScan && (() => {
				const results = latestScan.compliance_results || latestScan.results || [];
				const failedResults = results.filter(r => r.status === "fail");

				// Results breakdown data
				const resultsData = [
					{ name: "Passed", value: results.filter(r => r.status === "pass").length, color: "#22c55e" },
					{ name: "Failed", value: results.filter(r => r.status === "fail").length, color: "#ef4444" },
					{ name: "Warnings", value: results.filter(r => r.status === "warn").length, color: "#eab308" },
					{ name: "N/A", value: results.filter(r => r.status === "skip" || r.status === "notapplicable").length, color: "#6b7280" },
				].filter(d => d.value > 0);

				// Severity breakdown for failed rules
				const getSeverity = (r) => r.compliance_rules?.severity || r.rule?.severity || r.severity || "unknown";
				const severityData = [
					{ name: "Critical", value: failedResults.filter(r => getSeverity(r) === "critical").length, color: "#dc2626" },
					{ name: "High", value: failedResults.filter(r => getSeverity(r) === "high").length, color: "#f97316" },
					{ name: "Medium", value: failedResults.filter(r => getSeverity(r) === "medium").length, color: "#eab308" },
					{ name: "Low", value: failedResults.filter(r => getSeverity(r) === "low").length, color: "#3b82f6" },
					{ name: "Unknown", value: failedResults.filter(r => getSeverity(r) === "unknown").length, color: "#6b7280" },
				].filter(d => d.value > 0);

				return (
					<div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
						{/* Results Breakdown Pie Chart */}
						<div className="bg-secondary-800 rounded-lg border border-secondary-700 p-4">
							<h3 className="text-white font-medium mb-4 flex items-center gap-2">
								<BarChart3 className="h-4 w-4 text-primary-400" />
								Results Breakdown
							</h3>
							<div className="h-48">
								<ResponsiveContainer width="100%" height="100%">
									<PieChart>
										<Pie
											data={resultsData}
											cx="50%"
											cy="50%"
											innerRadius={40}
											outerRadius={70}
											dataKey="value"
											label={({ name, value }) => `${name}: ${value}`}
											labelLine={false}
										>
											{resultsData.map((entry, index) => (
												<Cell key={`cell-${index}`} fill={entry.color} />
											))}
										</Pie>
										<Tooltip
											contentStyle={{
												backgroundColor: "#1f2937",
												border: "1px solid #374151",
												borderRadius: "0.5rem",
											}}
											labelStyle={{ color: "#9ca3af" }}
										/>
									</PieChart>
								</ResponsiveContainer>
							</div>
							<div className="flex flex-wrap justify-center gap-4 mt-2">
								{resultsData.map((entry) => (
									<div key={entry.name} className="flex items-center gap-2 text-sm">
										<div className="w-3 h-3 rounded-full" style={{ backgroundColor: entry.color }} />
										<span className="text-secondary-400">{entry.name}</span>
									</div>
								))}
							</div>
						</div>

						{/* Failed Rules by Severity */}
						{failedResults.length > 0 && (
							<div className="bg-secondary-800 rounded-lg border border-secondary-700 p-4">
								<h3 className="text-white font-medium mb-4 flex items-center gap-2">
									<AlertTriangle className="h-4 w-4 text-red-400" />
									Failed Rules by Severity
								</h3>
								<div className="h-48">
									<ResponsiveContainer width="100%" height="100%">
										<BarChart data={severityData} layout="vertical">
											<XAxis type="number" stroke="#6b7280" fontSize={12} />
											<YAxis type="category" dataKey="name" stroke="#6b7280" fontSize={12} width={70} />
											<Tooltip
												contentStyle={{
													backgroundColor: "#1f2937",
													border: "1px solid #374151",
													borderRadius: "0.5rem",
												}}
												labelStyle={{ color: "#9ca3af" }}
											/>
											<Bar dataKey="value" radius={[0, 4, 4, 0]}>
												{severityData.map((entry, index) => (
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
					<div className="flex items-center justify-between mb-4">
						<div className="flex items-center gap-4">
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
						<div className="text-right">
							<p className="text-2xl font-mono font-bold text-primary-400">
								{Math.floor(elapsedTime / 60)}:{(elapsedTime % 60).toString().padStart(2, '0')}
							</p>
							<p className="text-xs text-secondary-500">elapsed</p>
						</div>
					</div>
					{/* Progress bar - use SSE progress if available, otherwise estimate from time */}
					<div className="w-full bg-secondary-700 rounded-full h-2 mb-3 overflow-hidden">
						<div
							className="bg-gradient-to-r from-primary-600 to-primary-400 h-2 rounded-full transition-all duration-1000"
							style={{ width: `${scanProgress?.progress || Math.min(95, (elapsedTime / 300) * 100)}%` }}
						/>
					</div>
					{/* Real-time progress message from SSE */}
					{scanProgress?.message ? (
						<div className="space-y-2">
							<p className="text-sm text-primary-300 flex items-center gap-2">
								<Clock className="h-4 w-4" />
								{scanProgress.message}
							</p>
							{scanProgress.phase && (
								<p className="text-xs text-secondary-500">
									Phase: <span className="capitalize font-medium text-secondary-400">{scanProgress.phase}</span>
								</p>
							)}
						</div>
					) : (
						<p className="text-sm text-secondary-400 flex items-center gap-2">
							<Clock className="h-4 w-4" />
							{elapsedTime < 120
								? "OpenSCAP scans typically take 3-5 minutes..."
								: elapsedTime < 300
									? "Still scanning, please wait..."
									: "This scan is taking longer than usual. Complex systems may require more time."}
						</p>
					)}
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

					{/* Docker Image Options - Only show for oscap-docker profile type */}
					{(() => {
						const selectedProfileData = availableProfiles.find(p => (p.xccdf_id || p.id) === selectedProfile);
						if (selectedProfileData?.type !== "oscap-docker") return null;

						return (
							<div className="bg-secondary-800 rounded-lg border border-secondary-700 p-6">
								<div className="flex items-center gap-3 mb-4">
									<div className="p-2 bg-blue-500/20 rounded-lg">
										<Container className="h-5 w-5 text-blue-400" />
									</div>
									<div>
										<h3 className="text-lg font-medium text-white">Docker Image Selection</h3>
										<p className="text-sm text-secondary-400">Choose which Docker images to scan for CVEs</p>
									</div>
								</div>

								<div className="space-y-4">
									{/* Scan All Images Toggle */}
									<div className="flex items-center justify-between p-4 bg-secondary-900/50 rounded-lg">
										<div className="flex items-center gap-3">
											<Box className="h-5 w-5 text-blue-400" />
											<div>
												<p className="text-white font-medium">Scan All Images</p>
												<p className="text-sm text-secondary-400">Scan all Docker images on this host</p>
											</div>
										</div>
										<button
											onClick={() => setScanAllDockerImages(!scanAllDockerImages)}
											className="focus:outline-none"
										>
											{scanAllDockerImages ? (
												<ToggleRight className="h-8 w-8 text-blue-500" />
											) : (
												<ToggleLeft className="h-8 w-8 text-secondary-500" />
											)}
										</button>
									</div>

									{/* Specific Image Input - Only show when Scan All is off */}
									{!scanAllDockerImages && (
										<div className="space-y-2">
											<label className="block text-sm font-medium text-secondary-300">
												Image Name (e.g., nginx:latest or ubuntu:22.04)
											</label>
											<input
												type="text"
												value={dockerImageName}
												onChange={(e) => setDockerImageName(e.target.value)}
												placeholder="Enter Docker image name..."
												className="w-full px-4 py-2 bg-secondary-900 border border-secondary-700 rounded-lg text-white placeholder-secondary-500 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
											/>
											{!dockerImageName && (
												<p className="text-sm text-yellow-400 flex items-center gap-1">
													<AlertTriangle className="h-3 w-3" />
													Please enter an image name or enable "Scan All Images"
												</p>
											)}
										</div>
									)}

									<div className="p-3 bg-blue-900/20 border border-blue-800/50 rounded-lg">
										<div className="flex items-start gap-2">
											<Info className="h-4 w-4 text-blue-400 mt-0.5 flex-shrink-0" />
											<div className="text-sm text-blue-200">
												<p className="font-medium">About Docker Image CVE Scanning</p>
												<p className="text-blue-300/80 mt-1">
													Uses OpenSCAP to scan Docker images for known vulnerabilities (CVEs).
													The scan downloads the latest OVAL vulnerability data for the image's OS.
												</p>
											</div>
										</div>
									</div>
								</div>
							</div>
						);
					})()}

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
								onClick={() => {
									// Find the selected profile to get its type
									const profile = availableProfiles.find(p => (p.xccdf_id || p.id) === selectedProfile);
									const profileType = profile?.type || "openscap"; // Default to openscap for CIS/STIG profiles

									// Build scan options
									const scanOptions = {
										profileType: profileType,
										profileId: selectedProfile,
										enableRemediation: enableRemediation,
									};

									// Add Docker image options for oscap-docker
									if (profileType === "oscap-docker") {
										scanOptions.scanAllImages = scanAllDockerImages;
										if (!scanAllDockerImages && dockerImageName) {
											scanOptions.imageName = dockerImageName;
										}
									}

									triggerScan.mutate(scanOptions);
								}}
								disabled={!isConnected || triggerScan.isPending || (
									// Disable if oscap-docker profile but no image specified and not scanning all
									availableProfiles.find(p => (p.xccdf_id || p.id) === selectedProfile)?.type === "oscap-docker" &&
									!scanAllDockerImages && !dockerImageName
								)}
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
	const renderResults = () => {
		const results = latestScan?.compliance_results || latestScan?.results || [];
		const failedResults = results.filter(r => r.status === "fail");
		const counts = {
			fail: failedResults.length,
			warn: results.filter(r => r.status === "warn").length,
			pass: results.filter(r => r.status === "pass").length,
			skipped: results.filter(r => r.status === "skip" || r.status === "notapplicable").length,
		};

		// Severity counts for failed rules
		const getSeverity = (result) => {
			return result.compliance_rules?.severity || result.rule?.severity || result.severity || "unknown";
		};
		const severityCounts = {
			critical: failedResults.filter(r => getSeverity(r) === "critical").length,
			high: failedResults.filter(r => getSeverity(r) === "high").length,
			medium: failedResults.filter(r => getSeverity(r) === "medium").length,
			low: failedResults.filter(r => getSeverity(r) === "low").length,
			unknown: failedResults.filter(r => getSeverity(r) === "unknown").length,
		};

		// Severity subtabs for Failed tab
		const severitySubtabs = [
			{ id: "all", label: "All", count: counts.fail },
			{ id: "critical", label: "Critical", count: severityCounts.critical, color: "text-red-500" },
			{ id: "high", label: "High", count: severityCounts.high, color: "text-orange-400" },
			{ id: "medium", label: "Medium", count: severityCounts.medium, color: "text-yellow-400" },
			{ id: "low", label: "Low", count: severityCounts.low, color: "text-blue-400" },
		];

		// Results subtabs configuration
		const resultsSubtabs = [
			{ id: "fail", label: "Failed", count: counts.fail, icon: XCircle, color: "text-red-400", bgColor: "bg-red-900/20", borderColor: "border-red-700" },
			{ id: "warn", label: "Warnings", count: counts.warn, icon: AlertTriangle, color: "text-yellow-400", bgColor: "bg-yellow-900/20", borderColor: "border-yellow-700" },
			{ id: "pass", label: "Passed", count: counts.pass, icon: CheckCircle, color: "text-green-400", bgColor: "bg-green-900/20", borderColor: "border-green-700" },
			{ id: "skipped", label: "Skipped/N/A", count: counts.skipped, icon: MinusCircle, color: "text-secondary-400", bgColor: "bg-secondary-700/50", borderColor: "border-secondary-600" },
		];

		// Get title for search matching
		const getTitle = (result) => {
			return result.compliance_rules?.title || result.rule?.title || result.title || "";
		};
		const getRuleId = (result) => {
			return result.compliance_rules?.rule_ref || result.rule?.rule_ref || result.rule_id || "";
		};

		// Map statusFilter to include both skip and notapplicable for "skipped" tab
		const getFilteredResults = () => {
			let filtered;
			if (statusFilter === "skipped") {
				filtered = results.filter(r => r.status === "skip" || r.status === "notapplicable");
			} else if (statusFilter === "all") {
				filtered = results;
			} else {
				filtered = results.filter(r => r.status === statusFilter);
			}

			// Apply severity filter if on Failed tab and not "all"
			if (statusFilter === "fail" && severityFilter !== "all") {
				filtered = filtered.filter(r => getSeverity(r) === severityFilter);
			}

			// Apply search filter
			if (ruleSearch.trim()) {
				const searchLower = ruleSearch.toLowerCase().trim();
				filtered = filtered.filter(r =>
					getTitle(r).toLowerCase().includes(searchLower) ||
					getRuleId(r).toLowerCase().includes(searchLower)
				);
			}

			return filtered;
		};

		const currentFilteredResults = getFilteredResults();

		return (
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

					{/* Remediation Status Banner */}
					{remediationStatus && (
						<div className={`rounded-lg border p-4 ${
							remediationStatus.phase === "error"
								? "bg-red-900/30 border-red-700"
								: remediationStatus.phase === "complete"
									? "bg-green-900/30 border-green-700"
									: "bg-orange-900/30 border-orange-700"
						}`}>
							<div className="flex items-center gap-3">
								{remediationStatus.phase === "sending" && (
									<div className="h-5 w-5 border-2 border-orange-400 border-t-transparent rounded-full animate-spin" />
								)}
								{remediationStatus.phase === "running" && (
									<Wrench className="h-5 w-5 text-orange-400 animate-pulse" />
								)}
								{remediationStatus.phase === "complete" && (
									<CheckCircle className="h-5 w-5 text-green-400" />
								)}
								{remediationStatus.phase === "error" && (
									<XCircle className="h-5 w-5 text-red-400" />
								)}
								<div className="flex-1">
									<p className={`font-medium ${
										remediationStatus.phase === "error"
											? "text-red-200"
											: remediationStatus.phase === "complete"
												? "text-green-200"
												: "text-orange-200"
									}`}>
										{remediationStatus.phase === "sending" && "Sending Fix Command..."}
										{remediationStatus.phase === "running" && "Applying Fix..."}
										{remediationStatus.phase === "complete" && "Fix Applied!"}
										{remediationStatus.phase === "error" && "Fix Failed"}
									</p>
									<p className={`text-sm ${
										remediationStatus.phase === "error"
											? "text-red-300/80"
											: remediationStatus.phase === "complete"
												? "text-green-300/80"
												: "text-orange-300/80"
									}`}>
										{remediationStatus.message}
									</p>
								</div>
							</div>
						</div>
					)}

					{/* Results Subtabs */}
					<div className="bg-secondary-800 rounded-lg border border-secondary-700">
						<div className="flex border-b border-secondary-700">
							{resultsSubtabs.map((tab) => {
								const Icon = tab.icon;
								const isActive = statusFilter === tab.id;
								return (
									<button
										key={tab.id}
										onClick={() => {
											setStatusFilter(tab.id);
											// Reset severity filter when switching tabs
											if (tab.id !== "fail") {
												setSeverityFilter("all");
											}
										}}
										className={`flex-1 flex items-center justify-center gap-2 px-4 py-3 text-sm font-medium transition-colors border-b-2 ${
											isActive
												? `${tab.color} border-current bg-secondary-700/50`
												: "text-secondary-400 border-transparent hover:text-secondary-200 hover:bg-secondary-700/30"
										}`}
									>
										<Icon className="h-4 w-4" />
										<span>{tab.label}</span>
										<span className={`px-2 py-0.5 rounded-full text-xs ${
											isActive ? tab.bgColor : "bg-secondary-600"
										}`}>
											{tab.count}
										</span>
									</button>
								);
							})}
						</div>

						{/* Severity subtabs - only show for Failed tab */}
						{statusFilter === "fail" && (
							<div className="flex items-center gap-2 px-4 py-2 border-b border-secondary-700 bg-secondary-750">
								<span className="text-xs text-secondary-400 mr-2">Severity:</span>
								{severitySubtabs.map((tab) => {
									const isActive = severityFilter === tab.id;
									return (
										<button
											key={tab.id}
											onClick={() => setSeverityFilter(tab.id)}
											className={`px-3 py-1 text-xs font-medium rounded-full transition-colors ${
												isActive
													? `bg-secondary-600 ${tab.color || "text-white"}`
													: "text-secondary-400 hover:text-secondary-200 hover:bg-secondary-700"
											}`}
										>
											{tab.label}
											{tab.count > 0 && (
												<span className="ml-1 opacity-75">({tab.count})</span>
											)}
										</button>
									);
								})}
							</div>
						)}

						{/* Search bar */}
						<div className="px-4 py-2 border-b border-secondary-700">
							<div className="relative">
								<Search className="absolute left-3 top-1/2 transform -translate-y-1/2 h-4 w-4 text-secondary-400" />
								<input
									type="text"
									placeholder="Search rules by title or ID..."
									value={ruleSearch}
									onChange={(e) => setRuleSearch(e.target.value)}
									className="w-full pl-10 pr-4 py-2 bg-secondary-700 border border-secondary-600 rounded-lg text-sm text-white placeholder-secondary-400 focus:outline-none focus:border-primary-500"
								/>
								{ruleSearch && (
									<button
										onClick={() => setRuleSearch("")}
										className="absolute right-3 top-1/2 transform -translate-y-1/2 text-secondary-400 hover:text-white"
									>
										<XCircle className="h-4 w-4" />
									</button>
								)}
							</div>
						</div>

						{/* Results List */}
						{currentFilteredResults && currentFilteredResults.length > 0 ? (
							<div className="divide-y divide-secondary-700 max-h-[600px] overflow-y-auto">
								{currentFilteredResults.map((result) => (
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
												{result.compliance_rules?.title || result.rule?.title || result.title || "Unknown Rule"}
											</p>
											<p className="text-xs text-secondary-400">
												{(result.compliance_rules?.section || result.rule?.section || result.section) &&
													`${result.compliance_rules?.section || result.rule?.section || result.section} • `}
												{(result.compliance_rules?.severity || result.rule?.severity || result.severity) && (
													<span className={`capitalize ${
														(result.compliance_rules?.severity || result.rule?.severity || result.severity) === "critical" ? "text-red-400" :
														(result.compliance_rules?.severity || result.rule?.severity || result.severity) === "high" ? "text-orange-400" :
														(result.compliance_rules?.severity || result.rule?.severity || result.severity) === "medium" ? "text-yellow-400" :
														"text-secondary-400"
													}`}>
														{result.compliance_rules?.severity || result.rule?.severity || result.severity}
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
											{/* Rule ID reference */}
											{(result.compliance_rules?.rule_ref || result.rule?.rule_ref || result.rule_id) && (
												<div className="flex items-center gap-2 text-xs">
													<span className="text-secondary-500">Rule ID:</span>
													<code className="bg-secondary-700 px-2 py-0.5 rounded text-secondary-300 font-mono">
														{result.compliance_rules?.rule_ref || result.rule?.rule_ref || result.rule_id}
													</code>
												</div>
											)}
											{(result.compliance_rules?.description || result.rule?.description || result.description) && (
												<div>
													<p className="text-secondary-400 font-medium mb-1 flex items-center gap-1">
														<Info className="h-3.5 w-3.5" />
														Description
													</p>
													<p className="text-secondary-300">
														{result.compliance_rules?.description || result.rule?.description || result.description}
													</p>
												</div>
											)}

											{/* WHY THIS FAILED - Clear explanation for failed rules */}
											{result.status === "fail" && (
												<div className="bg-red-900/20 border border-red-800/50 rounded-lg p-3">
													<p className="text-red-400 font-medium mb-2 flex items-center gap-1">
														<XCircle className="h-3.5 w-3.5" />
														Why This Failed
													</p>
													<div className="text-red-200/90 text-sm space-y-2">
														{(() => {
															// Check all possible locations for metadata (nested or direct from agent)
															const title = result.compliance_rules?.title || result.rule?.title || result.title || "";
															const description = result.compliance_rules?.description || result.rule?.description || result.description || "";
															const rationale = result.compliance_rules?.rationale || result.rule?.rationale || result.rationale || "";

															// If we have a specific finding from the scan, show it first
															if (result.finding) {
																return <p>{result.finding}</p>;
															}

															// If we have actual vs expected, show detailed comparison
															if (result.actual) {
																return (
																	<>
																		<p>The check found a non-compliant value:</p>
																		<div className="mt-2 grid grid-cols-1 gap-2">
																			<div className="bg-red-800/30 rounded p-2">
																				<span className="text-red-300 text-xs font-medium">Current setting:</span>
																				<code className="block mt-1 text-red-200 break-all">{result.actual}</code>
																			</div>
																			{result.expected && (
																				<div className="bg-green-800/30 rounded p-2">
																					<span className="text-green-300 text-xs font-medium">Required setting:</span>
																					<code className="block mt-1 text-green-200 break-all">{result.expected}</code>
																				</div>
																			)}
																		</div>
																	</>
																);
															}

															// PRIORITY: Use actual description from benchmark if available
															// This is the real explanation from SSG/CIS, not a generic pattern match
															if (description && description.length > 10) {
																// Clean up the description - remove extra whitespace
																const cleanDesc = description.replace(/\s+/g, ' ').trim();
																return (
																	<>
																		<p className="leading-relaxed">{cleanDesc}</p>
																		{rationale && rationale.length > 10 && (
																			<p className="mt-2 text-red-300/70 text-xs italic">
																				<strong>Security Impact:</strong> {rationale.replace(/\s+/g, ' ').trim().substring(0, 200)}{rationale.length > 200 ? "..." : ""}
																			</p>
																		)}
																	</>
																);
															}

															// Fallback: Generate explanation from title if no description
															// Parse "Ensure X is Y" pattern
															const ensureMatch = title.match(/^Ensure\s+(.+?)\s+(?:is|are)\s+(.+)$/i);
															if (ensureMatch) {
																const [, subject, expectedState] = ensureMatch;
																return (
																	<>
																		<p>This rule requires that <strong>{subject}</strong> is <strong>{expectedState}</strong>.</p>
																		<p className="mt-1 text-red-300/80">The current system configuration does not meet this requirement.</p>
																	</>
																);
															}

															// Parse "Install X" pattern
															const installMatch = title.match(/^Install\s+(.+)$/i);
															if (installMatch) {
																return (
																	<>
																		<p>The package <strong>{installMatch[1]}</strong> must be installed but is not present on this system.</p>
																	</>
																);
															}

															// Final fallback
															return (
																<>
																	<p>The system does not meet the requirement: <strong>{title || "this security check"}</strong></p>
																	<p className="mt-1 text-red-300/80">See the remediation steps below for how to fix this.</p>
																</>
															);
														})()}
													</div>
												</div>
											)}

											{/* Rationale - explains WHY this rule matters */}
											{(result.compliance_rules?.rationale || result.rule?.rationale || result.rationale) && (
												<div>
													<p className="text-secondary-400 font-medium mb-1 flex items-center gap-1">
														<BookOpen className="h-3.5 w-3.5" />
														Why This Matters
													</p>
													<p className="text-secondary-300 text-sm leading-relaxed">
														{result.compliance_rules?.rationale || result.rule?.rationale || result.rationale}
													</p>
												</div>
											)}
											{/* Show actual vs expected for clearer understanding */}
											{(result.actual || result.expected) && (
												<div className="grid grid-cols-1 md:grid-cols-2 gap-3">
													{result.actual && (
														<div className="bg-secondary-700/50 rounded p-2">
															<p className="text-secondary-400 text-xs font-medium mb-1">Current Value</p>
															<code className="text-red-300 text-xs break-all">{result.actual}</code>
														</div>
													)}
													{result.expected && (
														<div className="bg-secondary-700/50 rounded p-2">
															<p className="text-secondary-400 text-xs font-medium mb-1">Required Value</p>
															<code className="text-green-300 text-xs break-all">{result.expected}</code>
														</div>
													)}
												</div>
											)}

											{/* WHAT THE FIX DOES - Explanation before remediation */}
											{result.status === "fail" && (result.compliance_rules?.remediation || result.rule?.remediation || result.remediation) && (
												<div className="bg-orange-900/20 border border-orange-800/50 rounded-lg p-3">
													<p className="text-orange-400 font-medium mb-2 flex items-center gap-1">
														<Wrench className="h-3.5 w-3.5" />
														What the Fix Does
													</p>
													<div className="text-orange-200/90 text-sm space-y-2">
														<p>
															{(() => {
																const remediation = result.compliance_rules?.remediation || result.rule?.remediation || result.remediation || "";
																const title = result.compliance_rules?.title || result.rule?.title || "";
																// Generate a user-friendly description based on the rule
																if (remediation.includes("sysctl") || remediation.includes("/proc/sys")) {
																	return "This fix will modify kernel parameters to enable the required security setting. Changes are applied immediately and persist across reboots.";
																} else if (remediation.includes("chmod") || remediation.includes("chown")) {
																	return "This fix will update file permissions or ownership to meet the required security standard. This restricts unauthorized access to sensitive files.";
																} else if (remediation.includes("apt") || remediation.includes("yum") || remediation.includes("dnf")) {
																	return "This fix will install, update, or remove packages as needed to meet the security requirement.";
																} else if (remediation.includes("systemctl") || remediation.includes("service")) {
																	return "This fix will enable, disable, or configure a system service to meet the security requirement.";
																} else if (remediation.includes("/etc/ssh")) {
																	return "This fix will update SSH daemon configuration to harden remote access security.";
																} else if (remediation.includes("audit") || remediation.includes("auditd")) {
																	return "This fix will configure audit logging to track security-relevant system events.";
																} else if (remediation.includes("pam") || remediation.includes("/etc/pam")) {
																	return "This fix will configure authentication modules to enforce stronger access controls.";
																} else if (title.toLowerCase().includes("password")) {
																	return "This fix will update password policy settings to require stronger passwords or enforce better credential management.";
																} else if (title.toLowerCase().includes("firewall") || remediation.includes("iptables") || remediation.includes("nftables")) {
																	return "This fix will configure firewall rules to restrict network access and improve security.";
																} else {
																	return "This fix will apply the recommended configuration change to bring your system into compliance with the security benchmark.";
																}
															})()}
														</p>
													</div>
												</div>
											)}

											{(result.compliance_rules?.remediation || result.rule?.remediation || result.remediation) && (
												<div>
													<p className="text-secondary-400 font-medium mb-1 flex items-center gap-1">
														<Wrench className="h-3.5 w-3.5" />
														Remediation Steps
													</p>
													<pre className="text-secondary-300 whitespace-pre-wrap bg-secondary-700/30 rounded p-2 text-xs font-mono overflow-x-auto">
														{result.compliance_rules?.remediation || result.rule?.remediation || result.remediation}
													</pre>
												</div>
											)}
											{/* Fix This Rule button - only for failed rules */}
											{result.status === "fail" && isConnected && (
												<div className="mt-4 pt-3 border-t border-secondary-600">
													<button
														onClick={(e) => {
															e.stopPropagation();
															// Use rule_ref (XCCDF rule ID like xccdf_org.ssgproject...) not rule_id (database UUID)
															const ruleRef = result.compliance_rules?.rule_ref || result.rule?.rule_ref || result.rule_ref;
															console.log("[Compliance] Remediate click:", { ruleRef, compliance_rules: result.compliance_rules, result });
															if (ruleRef) {
																remediateRuleMutation.mutate(ruleRef);
															} else {
																console.error("[Compliance] No rule_ref found for remediation");
															}
														}}
														disabled={remediatingRule === (result.compliance_rules?.rule_ref || result.rule?.rule_ref || result.rule_ref || result.rule_id) || remediateRuleMutation.isPending}
														className={`flex items-center gap-2 px-3 py-1.5 rounded-lg text-sm font-medium transition-colors ${
															remediatingRule === (result.compliance_rules?.rule_ref || result.rule?.rule_ref || result.rule_ref || result.rule_id)
																? "bg-orange-600/50 text-orange-200 cursor-wait"
																: "bg-orange-600 hover:bg-orange-500 text-white"
														}`}
													>
														{remediatingRule === (result.compliance_rules?.rule_ref || result.rule?.rule_ref || result.rule_ref || result.rule_id) ? (
															<>
																<RefreshCw className="h-4 w-4 animate-spin" />
																Fixing...
															</>
														) : (
															<>
																<Wrench className="h-4 w-4" />
																Fix This Rule
															</>
														)}
													</button>
													<p className="text-xs text-secondary-500 mt-1">
														Attempts to automatically remediate this specific rule
													</p>
												</div>
											)}
										</div>
									)}
								</div>
							))}
						</div>
					) : (
						<div className="p-8 text-center">
							<p className="text-secondary-400">No {statusFilter !== "all" ? statusFilter : ""} results found</p>
						</div>
					)}
					</div>
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
	};

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
							disabled={isRefreshingStatus || ssgUpgradeMutation.isPending || ssgUpgradeMessage}
							className={`p-2 hover:bg-secondary-700 rounded-lg transition-colors ${(isRefreshingStatus || ssgUpgradeMutation.isPending || ssgUpgradeMessage) ? "cursor-wait" : ""}`}
							title={(isRefreshingStatus || ssgUpgradeMutation.isPending || ssgUpgradeMessage) ? "Refreshing..." : "Refresh status"}
						>
							<RefreshCw className={`h-4 w-4 ${(isRefreshingStatus || ssgUpgradeMutation.isPending || ssgUpgradeMessage) ? "text-primary-400 animate-spin" : "text-secondary-400"}`} />
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
												(components.openscap === "ready" || info?.openscap_available || info?.openscap_version) ? "text-green-400" :
												components.openscap === "installing" ? "text-blue-400" :
												components.openscap === "error" ? "text-red-400" :
												"text-secondary-400"
											}`}>
												{components.openscap || (info?.openscap_available || info?.openscap_version ? "Ready" : "Not installed")}
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
											<p className="text-xs text-secondary-400">CIS Docker Benchmark</p>
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

								{/* oscap-docker Component */}
								<div className="bg-secondary-700/30 rounded-lg p-4 border border-secondary-600">
									<div className="flex items-center gap-3 mb-4">
										<div className="p-2 bg-orange-600/20 rounded-lg">
											<Package className="h-5 w-5 text-orange-400" />
										</div>
										<div>
											<p className="text-white font-medium">oscap-docker</p>
											<p className="text-xs text-secondary-400">Docker Image CVE Scanning</p>
										</div>
									</div>
									<div className="space-y-2 text-sm">
										<div className="flex justify-between">
											<span className="text-secondary-400">Status</span>
											<span className={`capitalize ${
												components["oscap-docker"] === "ready" ? "text-green-400" :
												components["oscap-docker"] === "installing" ? "text-blue-400" :
												components["oscap-docker"] === "unavailable" ? "text-secondary-500" :
												components["oscap-docker"] === "error" ? "text-red-400" :
												"text-secondary-400"
											}`}>
												{components["oscap-docker"] || "Not configured"}
											</span>
										</div>
										<div className="flex justify-between">
											<span className="text-secondary-400">Available</span>
											<span className={info?.oscap_docker_available ? "text-green-400" : "text-secondary-500"}>
												{info?.oscap_docker_available ? "Yes" : "No"}
											</span>
										</div>
										<div className="flex justify-between">
											<span className="text-secondary-400">Requirement</span>
											<span className="text-secondary-300 text-xs">Docker + Compliance enabled</span>
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
															: profile.type === "oscap-docker"
															? "bg-orange-900/30 text-orange-400"
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
							PatchMonEnhanced uses industry-standard compliance scanning tools to evaluate your
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

				{/* SSG Security Content Update */}
				<div className="bg-secondary-800 rounded-lg border border-secondary-700 p-6">
					<h3 className="text-lg font-medium text-white flex items-center gap-2 mb-4">
						<Download className="h-5 w-5 text-primary-400" />
						Security Content Update
					</h3>
					<div className="space-y-4">
						<p className="text-sm text-secondary-300">
							Download the latest SCAP Security Guide (SSG) content from GitHub.
							This updates compliance rules, benchmarks, and remediation scripts.
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
							onClick={() => ssgUpdateMutation.mutate()}
							disabled={!isConnected || ssgUpdateMutation.isPending}
							className={`flex items-center gap-2 px-4 py-2 rounded-lg font-medium transition-colors ${
								!isConnected
									? "bg-secondary-700 text-secondary-500 cursor-not-allowed"
									: ssgUpdateMutation.isPending
									? "bg-primary-600/50 text-white cursor-wait"
									: "bg-primary-600 hover:bg-primary-500 text-white"
							}`}
						>
							{ssgUpdateMutation.isPending ? (
								<RefreshCw className="h-4 w-4 animate-spin" />
							) : (
								<Download className="h-4 w-4" />
							)}
							{ssgUpdateMutation.isPending ? "Updating Security Content..." : "Update SSG Content"}
						</button>
						{!isConnected && (
							<p className="text-xs text-secondary-500">Agent must be connected to update security content</p>
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
