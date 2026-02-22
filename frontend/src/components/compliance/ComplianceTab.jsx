import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
	AlertTriangle,
	BarChart3,
	BookOpen,
	Box,
	CheckCircle,
	CheckCircle2,
	ChevronDown,
	ChevronLeft,
	ChevronRight,
	Clock,
	Container,
	Download,
	Folder,
	FolderOpen,
	History,
	Info,
	Layers,
	ListChecks,
	Loader2,
	MinusCircle,
	Package,
	Play,
	RefreshCw,
	Search,
	Server,
	Settings,
	Shield,
	SkipForward,
	Square,
	ToggleLeft,
	ToggleRight,
	Wrench,
	XCircle,
} from "lucide-react";
import { lazy, Suspense, useEffect, useRef, useState } from "react";
import {
	Bar,
	BarChart,
	Cell,
	ResponsiveContainer,
	Tooltip,
	XAxis,
	YAxis,
} from "recharts";
import { complianceAPI } from "../../utils/complianceApi";
import ComplianceScore from "./ComplianceScore";

// Lazy load ComplianceTrend to avoid recharts bundling issues
const ComplianceTrend = lazy(() => import("./ComplianceTrend"));

// Fallback scan profiles (used if agent doesn't provide any)
const DEFAULT_SCAN_PROFILES = [
	{
		id: "level1_server",
		name: "CIS Level 1 Server",
		description: "Basic security hardening for servers",
		type: "openscap",
	},
	{
		id: "level2_server",
		name: "CIS Level 2 Server",
		description: "Extended security hardening (more restrictive)",
		type: "openscap",
	},
];

// Subtab definitions
const SUBTABS = [
	{ id: "overview", name: "Overview", icon: BarChart3 },
	{ id: "scan", name: "Run Scan", icon: Play },
	{ id: "results", name: "Results", icon: ListChecks },
	{ id: "history", name: "History", icon: History },
	{ id: "settings", name: "Settings", icon: Settings },
];

// Ordered steps for compliance scanner installation (match agent step ids)
const INSTALL_CHECKLIST_STEPS = [
	{ id: "detect_os", label: "Detect operating system" },
	{ id: "install_openscap", label: "Install OpenSCAP packages" },
	{ id: "verify_openscap", label: "Verify installation and SSG content" },
	{ id: "docker_bench", label: "Docker Bench (optional)" },
	{ id: "complete", label: "Complete" },
];

const ComplianceTab = ({
	hostId,
	apiId,
	isConnected,
	complianceEnabled = false,
	dockerEnabled = false,
}) => {
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
	const [currentPage, setCurrentPage] = useState(1); // Pagination for results
	const [groupBySection, setGroupBySection] = useState(false); // Group results by CIS section
	const [expandedSections, setExpandedSections] = useState({}); // Track which sections are expanded
	const [profileTypeFilter, setProfileTypeFilter] = useState(null); // Filter by profile type (null = show latest overall)
	const resultsPerPage = 25;
	const _queryClient = useQueryClient();

	// Scanner type toggles (OpenSCAP / Docker Bench individually)
	const [scannerToggles, setScannerToggles] = useState({
		openscap: true,
		docker_bench: false,
	});

	const {
		data: integrationsForToggles,
		refetch: refetchIntegrationsForToggles,
	} = useQuery({
		queryKey: ["host-integrations-scanners", hostId],
		queryFn: () =>
			complianceAPI.getHostIntegrations(hostId).then((res) => res.data),
		enabled: !!hostId,
		staleTime: 60 * 1000,
	});

	useEffect(() => {
		if (integrationsForToggles) {
			setScannerToggles({
				openscap:
					integrationsForToggles.data?.compliance_openscap_enabled ??
					integrationsForToggles.compliance_openscap_enabled ??
					true,
				docker_bench:
					integrationsForToggles.data?.compliance_docker_bench_enabled ??
					integrationsForToggles.compliance_docker_bench_enabled ??
					false,
			});
		}
	}, [integrationsForToggles]);

	const scannerToggleMutation = useMutation({
		mutationFn: (settings) => complianceAPI.setScannerToggles(hostId, settings),
		onSuccess: () => {
			refetchIntegrationsForToggles();
		},
	});

	// Persist scan state in sessionStorage to survive tab switches
	const scanStateKey = `compliance-scan-${hostId}`;
	const [scanInProgress, setScanInProgressState] = useState(() => {
		try {
			const saved = sessionStorage.getItem(scanStateKey);
			if (saved) {
				const parsed = JSON.parse(saved);
				// Check if scan started less than 10 minutes ago
				if (
					parsed.startTime &&
					Date.now() - parsed.startTime < 10 * 60 * 1000
				) {
					return true;
				}
				// Clear stale scan state
				sessionStorage.removeItem(scanStateKey);
			}
		} catch (_e) {
			// Ignore parsing errors
		}
		return false;
	});
	const [scanMessage, setScanMessageState] = useState(() => {
		try {
			const saved = sessionStorage.getItem(scanStateKey);
			if (saved) {
				const parsed = JSON.parse(saved);
				if (
					parsed.startTime &&
					Date.now() - parsed.startTime < 10 * 60 * 1000
				) {
					return parsed;
				}
			}
		} catch (_e) {
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
		if (msg?.startTime) {
			const toSave = { ...msg };
			if (profile) toSave.profileName = profile;
			sessionStorage.setItem(scanStateKey, JSON.stringify(toSave));
		} else if (!msg) {
			sessionStorage.removeItem(scanStateKey);
		}
	};

	// Get latest scan summary for each profile type (for tab display)
	const { data: scansByType } = useQuery({
		queryKey: ["compliance-scans-by-type", hostId],
		queryFn: () =>
			complianceAPI.getLatestScansByType(hostId).then((res) => res.data),
		enabled: !!hostId,
		staleTime: 30 * 1000,
		refetchOnWindowFocus: false,
	});

	const {
		data: latestScan,
		isLoading,
		isFetching,
		refetch: refetchLatest,
	} = useQuery({
		queryKey: ["compliance-latest", hostId, profileTypeFilter],
		queryFn: () =>
			complianceAPI
				.getLatestScan(hostId, profileTypeFilter ?? undefined)
				.then((res) => res.data),
		// Run as soon as we have hostId so Results tab has a scan to show; when profileTypeFilter is null we get latest of any type
		enabled: !!hostId,
		staleTime: 30 * 1000, // Consider data fresh for 30 seconds
		refetchOnWindowFocus: false, // Don't refetch on window focus to avoid flicker
	});

	const { data: scanHistory, refetch: refetchHistory } = useQuery({
		queryKey: ["compliance-history", hostId],
		queryFn: () =>
			complianceAPI.getHostScans(hostId, { limit: 10 }).then((res) => res.data),
		enabled: !!hostId,
		staleTime: 30 * 1000,
		refetchOnWindowFocus: false,
		placeholderData: (previousData) => previousData,
	});

	// Refs for SSE handler so effect only depends on scanInProgress/apiId (avoids connect/disconnect storm)
	const refetchLatestRef = useRef(refetchLatest);
	const refetchHistoryRef = useRef(refetchHistory);
	refetchLatestRef.current = refetchLatest;
	refetchHistoryRef.current = refetchHistory;

	// Paginated scan results (used on Results tab to avoid loading all rules at once)
	const scanIdForResults = latestScan?.id;
	const {
		data: scanResultsData,
		isLoading: scanResultsLoading,
		isFetching: scanResultsFetching,
		isError: scanResultsError,
		refetch: refetchScanResults,
	} = useQuery({
		queryKey: [
			"compliance-scan-results",
			scanIdForResults,
			currentPage,
			statusFilter,
			severityFilter,
		],
		queryFn: () =>
			complianceAPI.getScanResults(scanIdForResults, {
				limit: resultsPerPage,
				offset: (currentPage - 1) * resultsPerPage,
				...(statusFilter !== "all" ? { status: statusFilter } : {}),
				...(severityFilter !== "all" ? { severity: severityFilter } : {}),
			}),
		enabled: !!scanIdForResults && activeSubtab === "results",
		staleTime: 60 * 1000,
		refetchOnWindowFocus: false,
	});

	const paginatedResults = scanResultsData?.results ?? [];
	const paginationTotal = scanResultsData?.pagination?.total ?? 0;
	const severityBreakdown = scanResultsData?.severity_breakdown;

	// Get integration status (scanner info, components)
	const {
		data: integrationStatus,
		refetch: refetchStatus,
		isFetching: isRefreshingStatus,
	} = useQuery({
		queryKey: ["compliance-status", hostId],
		queryFn: () =>
			complianceAPI.getIntegrationStatus(hostId).then((res) => res.data),
		enabled: !!hostId,
		staleTime: 5 * 60 * 1000,
		refetchInterval: (query) => {
			const st = query.state?.data?.status?.status;
			if (st === "installing" || st === "removing") return 2000;
			return 5 * 60 * 1000;
		},
		refetchOnWindowFocus: false,
	});

	// Update selected profile when agent profiles are loaded
	useEffect(() => {
		const agentProfiles =
			integrationStatus?.status?.scanner_info?.available_profiles;
		if (agentProfiles?.length > 0) {
			// If current selection isn't in the agent's available profiles, select the first one
			const currentInList = agentProfiles.some(
				(p) => (p.xccdf_id || p.id) === selectedProfile,
			);
			if (!currentInList) {
				const firstProfile = agentProfiles[0];
				setSelectedProfile(firstProfile.xccdf_id || firstProfile.id);
			}
		}
	}, [
		integrationStatus?.status?.scanner_info?.available_profiles,
		selectedProfile,
	]);

	// Auto-switch status filter based on profile type when viewing results
	// Docker Bench uses "warn" for issues, OpenSCAP uses "fail"
	useEffect(() => {
		const profileType = latestScan?.compliance_profiles?.type;
		if (profileType === "docker-bench" && statusFilter === "fail") {
			setStatusFilter("warn");
		}
	}, [latestScan?.compliance_profiles?.type, statusFilter]);

	// Auto-select the first available profile type when scan data loads
	useEffect(() => {
		if (scansByType && profileTypeFilter === null) {
			// Set to the type of the latest scan, or first available
			const latestType = latestScan?.compliance_profiles?.type;
			if (latestType && scansByType[latestType]) {
				setProfileTypeFilter(latestType);
				if (latestType === "docker-bench") {
					setStatusFilter("warn");
				}
			} else if (scansByType.openscap) {
				setProfileTypeFilter("openscap");
			} else if (scansByType["docker-bench"]) {
				setProfileTypeFilter("docker-bench");
				setStatusFilter("warn");
			}
		}
	}, [scansByType, latestScan?.compliance_profiles?.type, profileTypeFilter]);

	// Reset to page 1 when status or severity filter changes (paginated results refetch with new params)
	useEffect(() => {
		setCurrentPage(1);
	}, []);

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
			setUpdateMessage({
				type: "success",
				text: "SSG update command sent! Security content will be updated shortly.",
			});
			setTimeout(() => {
				setUpdateMessage(null);
				refetchStatus();
			}, 5000);
		},
		onError: (error) => {
			console.error("SSG update error:", error);
			const errorMsg =
				error.response?.data?.error ||
				error.message ||
				"Failed to send SSG update command";
			setUpdateMessage({
				type: "error",
				text: errorMsg,
			});
			setTimeout(() => setUpdateMessage(null), 8000);
		},
	});

	// SSG upgrade mutation
	const [ssgUpgradeMessage, setSSGUpgradeMessage] = useState(null);
	const ssgUpgradeMutation = useMutation({
		mutationFn: () => complianceAPI.upgradeSSG(hostId),
		onSuccess: () => {
			setSSGUpgradeMessage({
				type: "success",
				text: "Upgrading SSG content from GitHub... This may take 10-15 seconds.",
			});
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
				text:
					error.response?.data?.error || "Failed to send SSG upgrade command",
			});
			setTimeout(() => setSSGUpgradeMessage(null), 5000);
		},
	});

	// Install scanner mutation (apt install openscap, update SSG)
	const installScannerMutation = useMutation({
		mutationFn: () => complianceAPI.installScanner(hostId),
		onSuccess: () => {
			refetchStatus();
			refetchInstallJob();
			// Poll status while agent is installing (agent sends "installing" then "ready")
			const interval = setInterval(() => {
				refetchStatus();
				refetchInstallJob();
			}, 2500);
			setTimeout(() => clearInterval(interval), 90000);
		},
		onError: (error) => {
			const msg =
				error.response?.data?.error || "Failed to send install command";
			setSSGUpgradeMessage({ type: "error", text: msg });
			setTimeout(() => setSSGUpgradeMessage(null), 6000);
		},
	});

	// Install job status (progress + install_events) — keep polling while job is active so checklist stays visible
	const integration_status = integrationStatus?.status?.status;
	const { data: installJobData, refetch: refetchInstallJob } = useQuery({
		queryKey: ["compliance-install-job", hostId],
		queryFn: () => complianceAPI.getInstallJobStatus(hostId),
		enabled: !!hostId,
		staleTime: 0,
		refetchInterval: (query) => {
			const job = query.state?.data;
			const job_in_progress =
				job?.status === "active" || job?.status === "waiting";
			if (integration_status === "installing" || job_in_progress) return 2000;
			return false;
		},
	});

	// Single rule remediation mutation with enhanced feedback
	const [remediationStatus, setRemediationStatus] = useState(null); // { phase: 'sending'|'running'|'complete'|'error', rule: string, message: string }
	const remediateRuleMutation = useMutation({
		mutationFn: (ruleId) => complianceAPI.remediateRule(hostId, ruleId),
		onMutate: (ruleId) => {
			setRemediatingRule(ruleId);
			setRemediationStatus({
				phase: "sending",
				rule: ruleId,
				message: "Sending remediation command to agent...",
			});
		},
		onSuccess: (_, ruleId) => {
			setRemediationStatus({
				phase: "running",
				rule: ruleId,
				message: "Agent is applying the fix. This may take a moment...",
			});
			// Show running status for a few seconds, then complete
			setTimeout(() => {
				setRemediationStatus({
					phase: "complete",
					rule: ruleId,
					message: "Fix applied! Run a new scan to verify the change.",
				});
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
				message: error.response?.data?.error || "Failed to remediate rule",
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
	}, [
		scanInProgress,
		scanMessage?.startTime,
		refetchLatest,
		refetchHistory,
		// biome-ignore lint/correctness/useExhaustiveDependencies: React useState setters are stable
		setScanInProgress,
		// biome-ignore lint/correctness/useExhaustiveDependencies: React useState setters are stable
		setScanMessage,
	]);

	// SSE connection for real-time compliance scan progress.
	// Effect MUST depend only on scanInProgress and apiId. Including setScanInProgress/setScanMessage
	// (wrapper functions recreated each render) caused the effect to re-run every render → new
	// EventSource and cleanup of the previous one → connect/disconnect storm on the server.
	// biome-ignore lint/correctness/useExhaustiveDependencies: wrapper setters are recreated each render and would cause SSE connect/disconnect storms
	useEffect(() => {
		if (!scanInProgress || !apiId) {
			setScanProgress(null);
			return;
		}

		const eventSource = new EventSource(
			`/api/v1/ws/compliance-progress/${apiId}/stream`,
			{ withCredentials: true },
		);

		eventSource.onmessage = (event) => {
			try {
				const data = JSON.parse(event.data);
				setScanProgress(data);

				if (
					data.phase === "completed" ||
					data.phase === "failed" ||
					data.phase === "cancelled"
				) {
					setScanInProgress(false);
					if (data.phase === "completed") {
						setScanMessage({
							type: "success",
							text: data.message || "Scan completed!",
						});
					} else if (data.phase === "cancelled") {
						setScanMessage({
							type: "info",
							text: data.message || "Scan cancelled",
						});
					} else {
						setScanMessage({
							type: "error",
							text: data.error || data.message || "Scan failed",
						});
					}
					refetchLatestRef.current?.();
					refetchHistoryRef.current?.();
					if (data.phase === "completed") {
						setActiveSubtab("results");
					}
					setTimeout(() => {
						setScanProgress(null);
						setScanMessage(null);
					}, 10000);
				}
			} catch (err) {
				console.warn("[Compliance SSE] Failed to parse event:", err);
			}
		};

		eventSource.onerror = () => {
			// EventSource auto-reconnects; only log if needed for debugging
			// console.debug("[Compliance SSE] Connection error");
		};

		return () => {
			eventSource.close();
		};
	}, [scanInProgress, apiId]);

	const triggerScan = useMutation({
		mutationFn: (options) => complianceAPI.triggerScan(hostId, options),
		onSuccess: (response, variables) => {
			const body = response?.data;
			const job_id = body?.job_id || "";
			const job_label = job_id ? ` — Job ID: ${job_id}` : "";

			if (!isConnected) {
				// Agent offline: queued for later
				setScanMessage({
					type: "success",
					text: `${body?.message || "Scan queued; will run when agent is online"}${job_label}`,
				});
				setTimeout(() => setScanMessage(null), 10000);
				return;
			}

			// Agent connected: scan will run now via the worker
			setScanInProgress(true);
			const remediationText = variables.enableRemediation
				? " Remediation is enabled - failed rules will be automatically fixed."
				: "";
			const profileName =
				availableProfiles.find(
					(p) => (p.xccdf_id || p.id) === variables.profileId,
				)?.name || variables.profileId;
			setScanMessage(
				{
					type: "info",
					text: `Compliance scan started. This may take several minutes...${remediationText}${job_label}`,
					startTime: Date.now(),
					profileName: profileName,
				},
				profileName,
			);
		},
		onError: (error) => {
			const errorMsg =
				error.response?.data?.error ||
				error.message ||
				"Failed to trigger scan";
			setScanMessage({ type: "error", text: errorMsg });
			setTimeout(() => setScanMessage(null), 5000);
		},
	});

	const cancelScanMutation = useMutation({
		mutationFn: () => complianceAPI.cancelScan(hostId),
		onSuccess: () => {
			setScanMessage({
				type: "info",
				text: "Cancel requested. The scan will stop shortly.",
			});
			setTimeout(() => setScanMessage(null), 5000);
		},
		onError: (error) => {
			const errorMsg =
				error.response?.data?.error ||
				error.message ||
				"Failed to send cancel request";
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
			notapplicable:
				"bg-secondary-700/50 text-secondary-500 border-secondary-600",
		};
		return styles[status] || styles.skip;
	};

	const _filteredResults =
		latestScan?.compliance_results?.filter((r) =>
			statusFilter === "all" ? true : r.status === statusFilter,
		) ||
		latestScan?.results?.filter((r) =>
			statusFilter === "all" ? true : r.status === statusFilter,
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
								{scannerInfo.mismatch_warning ||
									"SCAP content may not match your OS version. Results may show many N/A rules."}
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
					<p className="text-xs text-primary-400/80 mb-2">
						The scan runs on the host in the background. You can leave this tab
						and come back; use Cancel to stop it.
					</p>
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
						<div className="flex items-center gap-3">
							<div className="text-right">
								<p className="text-xl font-mono font-bold text-primary-400">
									{Math.floor(elapsedTime / 60)}:
									{(elapsedTime % 60).toString().padStart(2, "0")}
								</p>
								<p className="text-xs text-primary-400/60">elapsed</p>
							</div>
							<button
								type="button"
								onClick={() => cancelScanMutation.mutate()}
								disabled={cancelScanMutation.isPending || !isConnected}
								className="flex items-center gap-1.5 px-3 py-1.5 bg-red-600/30 hover:bg-red-600/50 text-red-200 text-sm rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
								title={
									!isConnected
										? "Host must be connected to cancel"
										: "Stop the running scan"
								}
							>
								<Square className="h-4 w-4" />
								Cancel
							</button>
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
								{scannerInfo.ssg_upgrade_message ||
									`Current version ${scannerInfo.ssg_version} is below minimum ${scannerInfo.ssg_min_version}. Update recommended for accurate compliance results.`}
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

			{/* Scan Results by Type - Show both OpenSCAP and Docker Bench */}
			{scansByType?.openscap || scansByType?.["docker-bench"] ? (
				<>
					<div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
						{/* OpenSCAP Card */}
						<div
							className={`bg-secondary-800 rounded-lg border ${scansByType?.openscap ? "border-green-700/50" : "border-secondary-700"} p-6`}
						>
							<div className="flex items-center gap-3 mb-4">
								<div className="p-2 rounded-lg bg-green-900/30">
									<Shield className="h-6 w-6 text-green-400" />
								</div>
								<div className="flex-1">
									<h3 className="text-lg font-semibold text-white">OpenSCAP</h3>
									<p className="text-xs text-secondary-400">
										CIS Benchmark Scanning
									</p>
								</div>
								{scansByType?.openscap && (
									<ComplianceScore
										score={scansByType.openscap.score}
										size="md"
									/>
								)}
							</div>

							{scansByType?.openscap ? (
								<>
									<div className="grid grid-cols-4 gap-2 mb-4">
										<div className="bg-secondary-700/50 rounded p-2 text-center">
											<p className="text-lg font-bold text-white">
												{scansByType.openscap.total_rules}
											</p>
											<p className="text-xs text-secondary-400">Total</p>
										</div>
										<div className="bg-green-900/20 rounded p-2 text-center">
											<p className="text-lg font-bold text-green-400">
												{scansByType.openscap.passed}
											</p>
											<p className="text-xs text-secondary-400">Passed</p>
										</div>
										<div className="bg-red-900/20 rounded p-2 text-center">
											<p className="text-lg font-bold text-red-400">
												{scansByType.openscap.failed}
											</p>
											<p className="text-xs text-secondary-400">Failed</p>
										</div>
										<div className="bg-secondary-700/50 rounded p-2 text-center">
											<p className="text-lg font-bold text-secondary-400">
												{scansByType.openscap.skipped || 0}
											</p>
											<p className="text-xs text-secondary-400">N/A</p>
										</div>
									</div>

									{/* Severity Breakdown Chart */}
									{scansByType.openscap.severity_breakdown?.length > 0 && (
										<div className="mb-4 p-3 bg-secondary-700/30 rounded-lg">
											<p className="text-xs text-secondary-400 mb-2">
												Failures by Severity
											</p>
											<div className="h-24">
												<ResponsiveContainer width="100%" height="100%">
													<BarChart
														data={scansByType.openscap.severity_breakdown
															.filter((s) => s.severity !== "unknown")
															.map((s) => ({
																name:
																	s.severity.charAt(0).toUpperCase() +
																	s.severity.slice(1),
																count: s.count,
																color:
																	s.severity === "critical"
																		? "#ef4444"
																		: s.severity === "high"
																			? "#f97316"
																			: s.severity === "medium"
																				? "#eab308"
																				: "#22c55e",
															}))}
														layout="vertical"
													>
														<XAxis type="number" hide />
														<YAxis
															type="category"
															dataKey="name"
															width={55}
															tick={{ fontSize: 10, fill: "#9ca3af" }}
														/>
														<Tooltip
															content={({ active, payload }) => {
																if (!active || !payload?.[0]) return null;
																return (
																	<div className="bg-gray-900 border border-gray-700 rounded px-2 py-1 text-xs">
																		<span className="text-white">
																			{payload[0].payload.name}:{" "}
																			{payload[0].value}
																		</span>
																	</div>
																);
															}}
														/>
														<Bar dataKey="count" radius={[0, 4, 4, 0]}>
															{scansByType.openscap.severity_breakdown
																.filter((s) => s.severity !== "unknown")
																.map((entry, index) => (
																	<Cell
																		key={`cell-${entry.severity || index}`}
																		fill={
																			entry.severity === "critical"
																				? "#ef4444"
																				: entry.severity === "high"
																					? "#f97316"
																					: entry.severity === "medium"
																						? "#eab308"
																						: "#22c55e"
																		}
																	/>
																))}
														</Bar>
													</BarChart>
												</ResponsiveContainer>
											</div>
										</div>
									)}

									<p className="text-xs text-secondary-500 mb-3">
										Last scan:{" "}
										{new Date(
											scansByType.openscap.completed_at,
										).toLocaleString()}
									</p>
									<button
										onClick={() => {
											setProfileTypeFilter("openscap");
											setStatusFilter("fail");
											setActiveSubtab("results");
										}}
										className="w-full flex items-center justify-center gap-2 px-3 py-2 bg-green-600/20 hover:bg-green-600/30 text-green-400 rounded-lg transition-colors text-sm"
									>
										<ListChecks className="h-4 w-4" />
										View Results
									</button>
								</>
							) : (
								<div className="text-center py-6">
									<p className="text-secondary-500 text-sm mb-3">
										No OpenSCAP scan data
									</p>
									<button
										onClick={() => setActiveSubtab("scan")}
										className="text-xs text-primary-400 hover:text-primary-300"
									>
										Run a scan →
									</button>
								</div>
							)}
						</div>

						{/* Docker Bench Card */}
						<div
							className={`bg-secondary-800 rounded-lg border ${scansByType?.["docker-bench"] ? "border-blue-700/50" : "border-secondary-700"} p-6`}
						>
							<div className="flex items-center gap-3 mb-4">
								<div className="p-2 rounded-lg bg-blue-900/30">
									<Container className="h-6 w-6 text-blue-400" />
								</div>
								<div className="flex-1">
									<h3 className="text-lg font-semibold text-white">
										Docker Bench
									</h3>
									<p className="text-xs text-secondary-400">
										CIS Docker Benchmark
									</p>
								</div>
								{scansByType?.["docker-bench"] && (
									<ComplianceScore
										score={scansByType["docker-bench"].score}
										size="md"
									/>
								)}
							</div>

							{scansByType?.["docker-bench"] ? (
								<>
									<div className="grid grid-cols-4 gap-2 mb-4">
										<div className="bg-secondary-700/50 rounded p-2 text-center">
											<p className="text-lg font-bold text-white">
												{scansByType["docker-bench"].total_rules}
											</p>
											<p className="text-xs text-secondary-400">Total</p>
										</div>
										<div className="bg-green-900/20 rounded p-2 text-center">
											<p className="text-lg font-bold text-green-400">
												{scansByType["docker-bench"].passed}
											</p>
											<p className="text-xs text-secondary-400">Passed</p>
										</div>
										<div className="bg-yellow-900/20 rounded p-2 text-center">
											<p className="text-lg font-bold text-yellow-400">
												{scansByType["docker-bench"].warnings}
											</p>
											<p className="text-xs text-secondary-400">Warnings</p>
										</div>
										<div className="bg-secondary-700/50 rounded p-2 text-center">
											<p className="text-lg font-bold text-secondary-400">
												{scansByType["docker-bench"].skipped || 0}
											</p>
											<p className="text-xs text-secondary-400">Info</p>
										</div>
									</div>

									{/* Section Breakdown Chart */}
									{scansByType["docker-bench"].section_breakdown?.length >
										0 && (
										<div className="mb-4 p-3 bg-secondary-700/30 rounded-lg">
											<p className="text-xs text-secondary-400 mb-2">
												Warnings by Section
											</p>
											<div className="h-24">
												<ResponsiveContainer width="100%" height="100%">
													<BarChart
														data={scansByType["docker-bench"].section_breakdown
															.slice(0, 4)
															.map((s, i) => ({
																name:
																	s.section.length > 15
																		? `${s.section.slice(0, 12)}...`
																		: s.section,
																fullName: s.section,
																count: s.count,
																color:
																	[
																		"#ef4444",
																		"#f97316",
																		"#eab308",
																		"#84cc16",
																		"#22c55e",
																		"#3b82f6",
																		"#8b5cf6",
																	][i] || "#6b7280",
															}))}
														layout="vertical"
													>
														<XAxis type="number" hide />
														<YAxis
															type="category"
															dataKey="name"
															width={70}
															tick={{ fontSize: 9, fill: "#9ca3af" }}
														/>
														<Tooltip
															content={({ active, payload }) => {
																if (!active || !payload?.[0]) return null;
																return (
																	<div className="bg-gray-900 border border-gray-700 rounded px-2 py-1 text-xs max-w-[200px]">
																		<p className="text-white font-medium">
																			{payload[0].payload.fullName}
																		</p>
																		<p className="text-gray-300">
																			Warnings: {payload[0].value}
																		</p>
																	</div>
																);
															}}
														/>
														<Bar dataKey="count" radius={[0, 4, 4, 0]}>
															{scansByType["docker-bench"].section_breakdown
																.slice(0, 4)
																.map((entry, index) => (
																	<Cell
																		key={`cell-${entry?.section || index}`}
																		fill={
																			[
																				"#ef4444",
																				"#f97316",
																				"#eab308",
																				"#84cc16",
																				"#22c55e",
																				"#3b82f6",
																				"#8b5cf6",
																			][index] || "#6b7280"
																		}
																	/>
																))}
														</Bar>
													</BarChart>
												</ResponsiveContainer>
											</div>
										</div>
									)}

									<p className="text-xs text-secondary-500 mb-3">
										Last scan:{" "}
										{new Date(
											scansByType["docker-bench"].completed_at,
										).toLocaleString()}
									</p>
									<button
										onClick={() => {
											setProfileTypeFilter("docker-bench");
											setStatusFilter("warn");
											setActiveSubtab("results");
										}}
										className="w-full flex items-center justify-center gap-2 px-3 py-2 bg-blue-600/20 hover:bg-blue-600/30 text-blue-400 rounded-lg transition-colors text-sm"
									>
										<ListChecks className="h-4 w-4" />
										View Results
									</button>
								</>
							) : (
								<div className="text-center py-6">
									<p className="text-secondary-500 text-sm mb-3">
										No Docker Bench scan data
									</p>
									<button
										onClick={() => setActiveSubtab("scan")}
										className="text-xs text-primary-400 hover:text-primary-300"
									>
										Run a scan →
									</button>
								</div>
							)}
						</div>
					</div>

					{/* Quick Actions */}
					<div className="flex justify-center gap-3 mt-4">
						<button
							onClick={() => setActiveSubtab("scan")}
							className="flex items-center gap-2 px-4 py-2 bg-primary-600 hover:bg-primary-700 text-white rounded-lg transition-colors"
						>
							<Play className="h-4 w-4" />
							Run New Scan
						</button>
					</div>
				</>
			) : (
				<div className="card p-12 text-center">
					<Shield className="h-16 w-16 text-secondary-600 mx-auto mb-4" />
					<h3 className="text-lg font-medium text-white mb-2">
						No Compliance Scans Yet
					</h3>
					<p className="text-secondary-400 mb-2">
						Run a security compliance scan to check this host against CIS
						benchmarks.
					</p>
					<p className="text-sm text-secondary-500 mb-6">
						Ensure OpenSCAP (and Docker if using Docker Bench) is installed on
						this host.
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

			{/* Compliance Score Trend */}
			{(scansByType?.openscap || scansByType?.["docker-bench"]) && (
				<Suspense
					fallback={
						<div className="h-48 bg-secondary-800 rounded-lg border border-secondary-700 animate-pulse" />
					}
				>
					<ComplianceTrend hostId={hostId} />
				</Suspense>
			)}
		</div>
	);

	// Get available profiles from agent or use defaults
	const scannerInfo = integrationStatus?.status?.scanner_info;
	const availableProfiles =
		scannerInfo?.available_profiles?.length > 0
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
								{scannerInfo.mismatch_warning ||
									"SCAP content may not match your OS version. Results may show many N/A rules."}
							</p>
						</div>
					</div>
				</div>
			)}

			{/* Scan Message */}
			{scanMessage && (
				<div
					className={`p-4 rounded-lg flex items-center gap-3 ${
						scanMessage.type === "success"
							? "bg-green-900/50 border border-green-700 text-green-200"
							: scanMessage.type === "info"
								? "bg-blue-900/50 border border-blue-700 text-blue-200"
								: "bg-red-900/50 border border-red-700 text-red-200"
					}`}
				>
					{scanMessage.type === "info" && (
						<RefreshCw className="h-5 w-5 animate-spin flex-shrink-0" />
					)}
					{scanMessage.type === "success" && (
						<CheckCircle className="h-5 w-5 flex-shrink-0" />
					)}
					{scanMessage.type === "error" && (
						<XCircle className="h-5 w-5 flex-shrink-0" />
					)}
					<span>{scanMessage.text}</span>
				</div>
			)}

			{/* Scan In Progress */}
			{scanInProgress ? (
				<div className="bg-secondary-800 rounded-lg border border-primary-600 p-6">
					<p className="text-xs text-secondary-400 mb-3">
						The scan runs on the host in the background. You can leave this tab
						and come back; use Cancel to stop it.
					</p>
					<div className="flex items-center justify-between mb-4">
						<div className="flex items-center gap-4">
							<div className="p-3 bg-primary-600/20 rounded-full">
								<RefreshCw className="h-6 w-6 animate-spin text-primary-400" />
							</div>
							<div>
								<h3 className="text-lg font-medium text-white">
									Scan In Progress
								</h3>
								<p className="text-sm text-secondary-400">
									Running{" "}
									{availableProfiles.find((p) => p.id === selectedProfile)
										?.name || selectedProfile}
								</p>
							</div>
						</div>
						<div className="flex items-center gap-4">
							<div className="text-right">
								<p className="text-2xl font-mono font-bold text-primary-400">
									{Math.floor(elapsedTime / 60)}:
									{(elapsedTime % 60).toString().padStart(2, "0")}
								</p>
								<p className="text-xs text-secondary-500">elapsed</p>
							</div>
							<button
								type="button"
								onClick={() => cancelScanMutation.mutate()}
								disabled={cancelScanMutation.isPending || !isConnected}
								className="flex items-center gap-1.5 px-3 py-2 bg-red-600/30 hover:bg-red-600/50 text-red-200 text-sm rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
								title={
									!isConnected
										? "Host must be connected to cancel"
										: "Stop the running scan"
								}
							>
								<Square className="h-4 w-4" />
								Cancel
							</button>
						</div>
					</div>
					{/* Progress bar - use SSE progress if available, otherwise estimate from time */}
					<div className="w-full bg-secondary-700 rounded-full h-2 mb-3 overflow-hidden">
						<div
							className="bg-gradient-to-r from-primary-600 to-primary-400 h-2 rounded-full transition-all duration-1000"
							style={{
								width: `${scanProgress?.progress || Math.min(95, (elapsedTime / 300) * 100)}%`,
							}}
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
									Phase:{" "}
									<span className="capitalize font-medium text-secondary-400">
										{scanProgress.phase}
									</span>
								</p>
							)}
						</div>
					) : (
						<p className="text-sm text-secondary-400 flex items-center gap-2">
							<Clock className="h-4 w-4" />
							{(() => {
								const currentProfile = availableProfiles.find(
									(p) => (p.xccdf_id || p.id) === selectedProfile,
								);
								const isDockerBench = currentProfile?.type === "docker-bench";
								if (isDockerBench) {
									return elapsedTime < 60
										? "Docker Bench scans typically complete in 1-2 minutes..."
										: elapsedTime < 180
											? "Still scanning Docker configuration..."
											: "This scan is taking longer than usual.";
								}
								return elapsedTime < 120
									? "OpenSCAP scans typically take 3-5 minutes..."
									: elapsedTime < 300
										? "Still scanning, please wait..."
										: "This scan is taking longer than usual. Complex systems may require more time.";
							})()}
						</p>
					)}
				</div>
			) : (
				<>
					{/* Profile Selection */}
					<div className="bg-secondary-800 rounded-lg border border-secondary-700 p-6">
						<div className="flex items-center justify-between mb-4">
							<h3 className="text-lg font-medium text-white">
								Select Scan Profile
							</h3>
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
								cis: {
									name: "CIS Benchmarks",
									color: "text-green-400",
									bg: "bg-green-900/30",
								},
								stig: {
									name: "DISA STIG",
									color: "text-orange-400",
									bg: "bg-orange-900/30",
								},
								"pci-dss": {
									name: "PCI-DSS",
									color: "text-purple-400",
									bg: "bg-purple-900/30",
								},
								hipaa: {
									name: "HIPAA",
									color: "text-blue-400",
									bg: "bg-blue-900/30",
								},
								anssi: {
									name: "ANSSI",
									color: "text-cyan-400",
									bg: "bg-cyan-900/30",
								},
								standard: {
									name: "Standard",
									color: "text-yellow-400",
									bg: "bg-yellow-900/30",
								},
								other: {
									name: "Other Profiles",
									color: "text-secondary-400",
									bg: "bg-secondary-700/50",
								},
							};

							return Object.entries(grouped).map(([category, profiles]) => {
								const catInfo =
									categoryLabels[category] || categoryLabels.other;
								return (
									<div key={category} className="mb-4 last:mb-0">
										<h4 className={`text-sm font-medium mb-2 ${catInfo.color}`}>
											{catInfo.name} ({profiles.length})
										</h4>
										<div className="grid gap-2">
											{profiles.map((profile) => (
												<button
													key={profile.id}
													onClick={() =>
														setSelectedProfile(profile.xccdf_id || profile.id)
													}
													className={`flex items-center gap-3 p-3 rounded-lg border transition-all text-left ${
														selectedProfile === (profile.xccdf_id || profile.id)
															? "bg-primary-900/30 border-primary-600"
															: "bg-secondary-700/30 border-secondary-600 hover:border-secondary-500"
													}`}
												>
													<div
														className={`p-1.5 rounded ${
															selectedProfile ===
															(profile.xccdf_id || profile.id)
																? "bg-primary-600/20"
																: catInfo.bg
														}`}
													>
														{profile.type === "docker-bench" ? (
															<Package
																className={`h-4 w-4 ${
																	selectedProfile ===
																	(profile.xccdf_id || profile.id)
																		? "text-primary-400"
																		: catInfo.color
																}`}
															/>
														) : (
															<Shield
																className={`h-4 w-4 ${
																	selectedProfile ===
																	(profile.xccdf_id || profile.id)
																		? "text-primary-400"
																		: catInfo.color
																}`}
															/>
														)}
													</div>
													<div className="flex-1 min-w-0">
														<p
															className={`font-medium text-sm truncate ${
																selectedProfile ===
																(profile.xccdf_id || profile.id)
																	? "text-primary-300"
																	: "text-white"
															}`}
														>
															{profile.name}
														</p>
														{profile.description && (
															<p className="text-xs text-secondary-400 truncate">
																{profile.description}
															</p>
														)}
													</div>
													<div className="flex items-center gap-2">
														<span
															className={`px-2 py-0.5 text-xs rounded ${
																profile.type === "docker-bench"
																	? "bg-blue-900/30 text-blue-400"
																	: "bg-green-900/30 text-green-400"
															}`}
														>
															{profile.type === "docker-bench"
																? "Docker"
																: "SCAP"}
														</span>
														{selectedProfile ===
															(profile.xccdf_id || profile.id) && (
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
									<div
										className={`p-2 rounded-lg ${enableRemediation ? "bg-orange-600/20" : "bg-secondary-600/50"}`}
									>
										<Wrench
											className={`h-5 w-5 ${enableRemediation ? "text-orange-400" : "text-secondary-400"}`}
										/>
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
												This will automatically modify system configuration to
												fix failed compliance rules. Review the profile
												requirements before enabling. Changes may affect system
												behavior.
											</p>
										</div>
									</div>
								</div>
							)}
						</div>
					</div>

					{/* Docker Image Options - Only show for oscap-docker profile type */}
					{(() => {
						const selectedProfileData = availableProfiles.find(
							(p) => (p.xccdf_id || p.id) === selectedProfile,
						);
						if (selectedProfileData?.type !== "oscap-docker") return null;

						return (
							<div className="bg-secondary-800 rounded-lg border border-secondary-700 p-6">
								<div className="flex items-center gap-3 mb-4">
									<div className="p-2 bg-blue-500/20 rounded-lg">
										<Container className="h-5 w-5 text-blue-400" />
									</div>
									<div>
										<h3 className="text-lg font-medium text-white">
											Docker Image Selection
										</h3>
										<p className="text-sm text-secondary-400">
											Choose which Docker images to scan for CVEs
										</p>
									</div>
								</div>

								<div className="space-y-4">
									{/* Scan All Images Toggle */}
									<div className="flex items-center justify-between p-4 bg-secondary-900/50 rounded-lg">
										<div className="flex items-center gap-3">
											<Box className="h-5 w-5 text-blue-400" />
											<div>
												<p className="text-white font-medium">
													Scan All Images
												</p>
												<p className="text-sm text-secondary-400">
													Scan all Docker images on this host
												</p>
											</div>
										</div>
										<button
											onClick={() =>
												setScanAllDockerImages(!scanAllDockerImages)
											}
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
												<p className="font-medium">
													About Docker Image CVE Scanning
												</p>
												<p className="text-blue-300/80 mt-1">
													Uses OpenSCAP to scan Docker images for known
													vulnerabilities (CVEs). The scan downloads the latest
													OVAL vulnerability data for the image's OS.
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
								<h3 className="text-lg font-medium text-white">
									Ready to Scan
								</h3>
								<p className="text-sm text-secondary-400">
									Selected:{" "}
									{availableProfiles.find(
										(p) => (p.xccdf_id || p.id) === selectedProfile,
									)?.name || selectedProfile}
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
									const profile = availableProfiles.find(
										(p) => (p.xccdf_id || p.id) === selectedProfile,
									);
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
								disabled={
									triggerScan.isPending ||
									// When offline, oscap-docker cannot be queued
									(!isConnected &&
										availableProfiles.find(
											(p) => (p.xccdf_id || p.id) === selectedProfile,
										)?.type === "oscap-docker") ||
									// Disable if oscap-docker profile but no image specified and not scanning all
									(availableProfiles.find(
										(p) => (p.xccdf_id || p.id) === selectedProfile,
									)?.type === "oscap-docker" &&
										!scanAllDockerImages &&
										!dockerImageName)
								}
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
								{triggerScan.isPending
									? "Starting…"
									: enableRemediation
										? "Scan & Remediate"
										: isConnected
											? "Start Scan"
											: "Queue scan for when agent is online"}
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

	// Render Results subtab (uses paginated results and optional severity_breakdown from API)
	const renderResults = () => {
		const byStatus = severityBreakdown?.by_status || {};
		const bySeverity = severityBreakdown?.by_severity || {};
		const counts = {
			fail: byStatus.fail ?? latestScan?.failed ?? 0,
			warn: byStatus.warn ?? latestScan?.warnings ?? 0,
			pass: byStatus.pass ?? latestScan?.passed ?? 0,
			skipped:
				(byStatus.skip ?? 0) + (byStatus.notapplicable ?? 0) ||
				(latestScan?.skipped ?? 0) + (latestScan?.not_applicable ?? 0),
		};

		// Determine if this is a Docker Bench scan
		const currentProfileType =
			latestScan?.compliance_profiles?.type || "openscap";
		const isDockerBenchResults = currentProfileType === "docker-bench";

		// Severity counts for failed rules (from first-page breakdown or zero)
		const severityCounts = {
			critical: bySeverity.critical ?? 0,
			high: bySeverity.high ?? 0,
			medium: bySeverity.medium ?? 0,
			low: bySeverity.low ?? 0,
			unknown: bySeverity.unknown ?? 0,
		};

		// Severity subtabs for Failed tab
		const severitySubtabs = [
			{ id: "all", label: "All", count: counts.fail },
			{
				id: "critical",
				label: "Critical",
				count: severityCounts.critical,
				color: "text-red-500",
			},
			{
				id: "high",
				label: "High",
				count: severityCounts.high,
				color: "text-orange-400",
			},
			{
				id: "medium",
				label: "Medium",
				count: severityCounts.medium,
				color: "text-yellow-400",
			},
			{
				id: "low",
				label: "Low",
				count: severityCounts.low,
				color: "text-blue-400",
			},
		];

		// Profile type tabs configuration - always show both if compliance is enabled
		const hasOpenSCAP = !!scansByType?.openscap;
		const hasDockerBench = !!scansByType?.["docker-bench"];
		const openscapData = scansByType?.openscap;
		const dockerBenchData = scansByType?.["docker-bench"];

		// Build profile type tabs with metrics
		const profileTypeTabs = [
			{
				id: "openscap",
				label: "OpenSCAP",
				icon: Server,
				available: hasOpenSCAP,
				score: openscapData?.score,
				passed: openscapData?.passed,
				failed: openscapData?.failed,
				total: openscapData?.total_rules,
				date: openscapData?.completed_at,
			},
			{
				id: "docker-bench",
				label: "Docker Bench",
				icon: Container,
				available: hasDockerBench,
				score: dockerBenchData?.score,
				passed: dockerBenchData?.passed,
				warnings: dockerBenchData?.warnings,
				total: dockerBenchData?.total_rules,
				date: dockerBenchData?.completed_at,
			},
		];

		// Results subtabs configuration - labels adapt based on profile type
		const resultsSubtabs = isDockerBenchResults
			? [
					// Docker Bench uses WARN for issues that need attention (not FAIL)
					{
						id: "warn",
						label: "Issues (WARN)",
						count: counts.warn,
						icon: AlertTriangle,
						color: "text-yellow-400",
						bgColor: "bg-yellow-900/20",
						borderColor: "border-yellow-700",
					},
					{
						id: "pass",
						label: "Passed",
						count: counts.pass,
						icon: CheckCircle,
						color: "text-green-400",
						bgColor: "bg-green-900/20",
						borderColor: "border-green-700",
					},
					{
						id: "skipped",
						label: "Info/Note",
						count: counts.skipped,
						icon: MinusCircle,
						color: "text-secondary-400",
						bgColor: "bg-secondary-700/50",
						borderColor: "border-secondary-600",
					},
				]
			: [
					// OpenSCAP uses standard terminology
					{
						id: "fail",
						label: "Failed",
						count: counts.fail,
						icon: XCircle,
						color: "text-red-400",
						bgColor: "bg-red-900/20",
						borderColor: "border-red-700",
					},
					{
						id: "warn",
						label: "Warnings",
						count: counts.warn,
						icon: AlertTriangle,
						color: "text-yellow-400",
						bgColor: "bg-yellow-900/20",
						borderColor: "border-yellow-700",
					},
					{
						id: "pass",
						label: "Passed",
						count: counts.pass,
						icon: CheckCircle,
						color: "text-green-400",
						bgColor: "bg-green-900/20",
						borderColor: "border-green-700",
					},
					{
						id: "skipped",
						label: "Skipped/N/A",
						count: counts.skipped,
						icon: MinusCircle,
						color: "text-secondary-400",
						bgColor: "bg-secondary-700/50",
						borderColor: "border-secondary-600",
					},
				];

		// Get title for search matching
		const getTitle = (result) => {
			return (
				result.compliance_rules?.title ||
				result.rule?.title ||
				result.title ||
				""
			);
		};
		const getRuleId = (result) => {
			return (
				result.compliance_rules?.rule_ref ||
				result.rule?.rule_ref ||
				result.rule_id ||
				""
			);
		};

		// API returns one page; apply only client-side search on current page
		const displayResults =
			ruleSearch.trim() === ""
				? paginatedResults
				: paginatedResults.filter((r) => {
						const searchLower = ruleSearch.toLowerCase().trim();
						return (
							getTitle(r).toLowerCase().includes(searchLower) ||
							getRuleId(r).toLowerCase().includes(searchLower)
						);
					});

		// Pagination from API
		const totalResults = paginationTotal;
		const totalPages = Math.max(1, Math.ceil(totalResults / resultsPerPage));
		const startIndex = (currentPage - 1) * resultsPerPage;
		const endIndex = Math.min(startIndex + resultsPerPage, totalResults);

		// Group results by CIS section (e.g., "1.1", "5.2")
		const getParentSection = (result) => {
			const section =
				result.compliance_rules?.section ||
				result.rule?.section ||
				result.section ||
				"";
			// Extract parent section (e.g., "1.1.1.1" -> "1.1", "5.2.3" -> "5.2")
			const parts = section.split(".");
			return parts.length >= 2 ? `${parts[0]}.${parts[1]}` : section || "Other";
		};

		const groupedResults = groupBySection
			? displayResults.reduce((groups, result) => {
					const section = getParentSection(result);
					if (!groups[section]) {
						groups[section] = [];
					}
					groups[section].push(result);
					return groups;
				}, {})
			: null;

		const sortedSections = groupedResults
			? Object.keys(groupedResults).sort((a, b) => {
					// Sort numerically by section
					const aParts = a.split(".").map(Number);
					const bParts = b.split(".").map(Number);
					for (let i = 0; i < Math.max(aParts.length, bParts.length); i++) {
						const aVal = aParts[i] || 0;
						const bVal = bParts[i] || 0;
						if (aVal !== bVal) return aVal - bVal;
					}
					return 0;
				})
			: [];

		const toggleSection = (section) => {
			setExpandedSections((prev) => ({
				...prev,
				[section]: !prev[section],
			}));
		};

		return (
			<div className="space-y-4">
				{latestScan ? (
					<>
						{/* Remediation Status Banner */}
						{remediationStatus && (
							<div
								className={`rounded-lg border p-4 ${
									remediationStatus.phase === "error"
										? "bg-red-900/30 border-red-700"
										: remediationStatus.phase === "complete"
											? "bg-green-900/30 border-green-700"
											: "bg-orange-900/30 border-orange-700"
								}`}
							>
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
										<p
											className={`font-medium ${
												remediationStatus.phase === "error"
													? "text-red-200"
													: remediationStatus.phase === "complete"
														? "text-green-200"
														: "text-orange-200"
											}`}
										>
											{remediationStatus.phase === "sending" &&
												"Sending Fix Command..."}
											{remediationStatus.phase === "running" &&
												"Applying Fix..."}
											{remediationStatus.phase === "complete" && "Fix Applied!"}
											{remediationStatus.phase === "error" && "Fix Failed"}
										</p>
										<p
											className={`text-sm ${
												remediationStatus.phase === "error"
													? "text-red-300/80"
													: remediationStatus.phase === "complete"
														? "text-green-300/80"
														: "text-orange-300/80"
											}`}
										>
											{remediationStatus.message}
										</p>
									</div>
								</div>
							</div>
						)}

						{/* Profile Type Selection - Click to switch between scan types */}
						<div className="mb-2">
							<h3 className="text-sm font-medium text-secondary-400 mb-2">
								Select Scan Type to View
							</h3>
						</div>
						<div className="grid grid-cols-1 md:grid-cols-2 gap-4">
							{profileTypeTabs.map((tab) => {
								const Icon = tab.icon;
								const isActive = profileTypeFilter === tab.id;
								const isDisabled = !tab.available;
								const isLoadingThis = isActive && isFetching;

								return (
									<button
										key={tab.id}
										onClick={() => {
											if (!isDisabled) {
												setProfileTypeFilter(tab.id);
												setCurrentPage(1);
												if (tab.id === "docker-bench") {
													setStatusFilter("warn");
												} else {
													setStatusFilter("fail");
												}
											}
										}}
										disabled={isDisabled}
										className={`text-left p-4 rounded-lg border-2 transition-all ${
											isActive
												? "bg-primary-900/30 border-primary-500"
												: isDisabled
													? "bg-secondary-800/50 border-secondary-700 opacity-50 cursor-not-allowed"
													: "bg-secondary-800 border-secondary-700 hover:border-secondary-500"
										}`}
									>
										<div className="flex items-center justify-between mb-2">
											<div className="flex items-center gap-2">
												{isLoadingThis ? (
													<RefreshCw className="h-5 w-5 text-primary-400 animate-spin" />
												) : (
													<Icon
														className={`h-5 w-5 ${isActive ? "text-primary-400" : "text-secondary-400"}`}
													/>
												)}
												<span
													className={`font-medium ${isActive ? "text-white" : "text-secondary-200"}`}
												>
													{tab.label}
												</span>
												{isActive && (
													<span className="px-2 py-0.5 text-xs bg-primary-600 text-white rounded">
														Selected
													</span>
												)}
											</div>
											{tab.available && tab.score != null && (
												<span
													className={`text-lg font-bold ${
														tab.score >= 80
															? "text-green-400"
															: tab.score >= 60
																? "text-yellow-400"
																: "text-red-400"
													}`}
												>
													{Math.round(tab.score)}%
												</span>
											)}
										</div>
										{tab.available ? (
											<div className="flex items-center gap-4 text-xs text-secondary-400">
												{tab.id === "docker-bench" ? (
													<>
														<span className="text-green-400">
															{tab.passed} passed
														</span>
														<span className="text-yellow-400">
															{tab.warnings} warnings
														</span>
														<span>{tab.total} rules</span>
													</>
												) : (
													<>
														<span className="text-green-400">
															{tab.passed} passed
														</span>
														<span className="text-red-400">
															{tab.failed} failed
														</span>
														<span>{tab.total} rules</span>
													</>
												)}
											</div>
										) : (
											<p className="text-xs text-secondary-500">
												No scan data available
											</p>
										)}
										{tab.date && (
											<p className="text-xs text-secondary-500 mt-1">
												{new Date(tab.date).toLocaleDateString()}
											</p>
										)}
									</button>
								);
							})}
						</div>

						{/* Current Scan Header - Shows which scan's results are displayed */}
						{latestScan && (
							<>
								<div className="flex items-center justify-between px-4 py-3 bg-secondary-800 rounded-lg border border-secondary-700">
									<div className="flex items-center gap-4">
										<div
											className={`p-2 rounded-lg ${isDockerBenchResults ? "bg-blue-900/30" : "bg-green-900/30"}`}
										>
											{isDockerBenchResults ? (
												<Container className="h-5 w-5 text-blue-400" />
											) : (
												<Shield className="h-5 w-5 text-green-400" />
											)}
										</div>
										<div>
											<p className="text-white font-medium">
												{latestScan.compliance_profiles?.name ||
													(isDockerBenchResults ? "Docker Bench" : "OpenSCAP")}
											</p>
											<p className="text-xs text-secondary-400">
												{new Date(latestScan.completed_at).toLocaleString()} •{" "}
												{latestScan.total_rules} rules evaluated
											</p>
										</div>
									</div>
									<div className="flex items-center gap-3">
										<ComplianceScore score={latestScan.score} size="md" />
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

								{/* Display error message if scan has one (e.g., CPE mismatch) */}
								{latestScan.error_message && (
									<div className="p-4 rounded-lg bg-orange-900/30 border border-orange-700 text-orange-200">
										<div className="flex items-start gap-3">
											<AlertTriangle className="h-5 w-5 text-orange-400 flex-shrink-0 mt-0.5" />
											<div>
												<p className="font-medium text-orange-300">
													Scan Warning
												</p>
												<p className="text-sm text-orange-200/80 mt-1">
													{latestScan.error_message}
												</p>
											</div>
										</div>
									</div>
								)}
							</>
						)}

						{/* Loading State */}
						{isFetching && (
							<div className="flex items-center justify-center py-8 bg-secondary-800 rounded-lg border border-secondary-700 mb-4">
								<RefreshCw className="h-6 w-6 text-primary-400 animate-spin mr-3" />
								<span className="text-secondary-400">
									Loading{" "}
									{profileTypeFilter === "docker-bench"
										? "Docker Bench"
										: "OpenSCAP"}{" "}
									results...
								</span>
							</div>
						)}

						{/* Results Subtabs */}
						{!isFetching && latestScan && (
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
													setCurrentPage(1); // Reset to first page when changing filter
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
												<span
													className={`px-2 py-0.5 rounded-full text-xs ${
														isActive ? tab.bgColor : "bg-secondary-600"
													}`}
												>
													{tab.count}
												</span>
											</button>
										);
									})}
								</div>

								{/* Severity subtabs - only show for Failed tab in OpenSCAP results (Docker Bench doesn't have severity) */}
								{statusFilter === "fail" && !isDockerBenchResults && (
									<div className="flex items-center gap-2 px-4 py-2 border-b border-secondary-700 bg-secondary-750">
										<span className="text-xs text-secondary-400 mr-2">
											Severity:
										</span>
										{severitySubtabs.map((tab) => {
											const isActive = severityFilter === tab.id;
											return (
												<button
													key={tab.id}
													onClick={() => {
														setSeverityFilter(tab.id);
														setCurrentPage(1);
													}}
													className={`px-3 py-1 text-xs font-medium rounded-full transition-colors ${
														isActive
															? `bg-secondary-600 ${tab.color || "text-white"}`
															: "text-secondary-400 hover:text-secondary-200 hover:bg-secondary-700"
													}`}
												>
													{tab.label}
													{tab.count > 0 && (
														<span className="ml-1 opacity-75">
															({tab.count})
														</span>
													)}
												</button>
											);
										})}
									</div>
								)}

								{/* Search bar and grouping toggle */}
								<div className="px-4 py-2 border-b border-secondary-700">
									<div className="flex items-center gap-3">
										<div className="relative flex-1">
											<Search className="absolute left-3 top-1/2 transform -translate-y-1/2 h-4 w-4 text-secondary-400" />
											<input
												type="text"
												placeholder="Search rules by title or ID..."
												value={ruleSearch}
												onChange={(e) => {
													setRuleSearch(e.target.value);
													setCurrentPage(1);
												}}
												className="w-full pl-10 pr-4 py-2 bg-secondary-700 border border-secondary-600 rounded-lg text-sm text-white placeholder-secondary-400 focus:outline-none focus:border-primary-500"
											/>
											{ruleSearch && (
												<button
													onClick={() => {
														setRuleSearch("");
														setCurrentPage(1);
													}}
													className="absolute right-3 top-1/2 transform -translate-y-1/2 text-secondary-400 hover:text-white"
												>
													<XCircle className="h-4 w-4" />
												</button>
											)}
										</div>
										<button
											onClick={() => {
												setGroupBySection(!groupBySection);
												setCurrentPage(1);
												setExpandedSections({});
											}}
											className={`flex items-center gap-2 px-3 py-2 rounded-lg text-sm font-medium transition-colors ${
												groupBySection
													? "bg-primary-600 text-white"
													: "bg-secondary-700 text-secondary-300 hover:bg-secondary-600"
											}`}
											title={
												groupBySection
													? "Show flat list"
													: "Group by CIS section"
											}
										>
											<Layers className="h-4 w-4" />
											<span className="hidden sm:inline">Group</span>
										</button>
									</div>
								</div>

								{/* Loading state for paginated scan results (table data) */}
								{(scanResultsLoading ||
									(scanResultsFetching && !scanResultsData)) && (
									<div className="flex items-center justify-center py-12 border-b border-secondary-700">
										<RefreshCw className="h-8 w-8 text-primary-400 animate-spin mr-3" />
										<span className="text-secondary-400">
											Loading scan results...
										</span>
									</div>
								)}
								{/* Error state for scan results query */}
								{!scanResultsLoading &&
									!scanResultsFetching &&
									scanResultsError && (
										<div className="p-6 border-b border-secondary-700 text-center">
											<p className="text-red-400 mb-3">
												Failed to load scan results
											</p>
											<button
												type="button"
												onClick={() => refetchScanResults()}
												className="inline-flex items-center gap-2 px-4 py-2 bg-primary-600 hover:bg-primary-700 text-white rounded-lg text-sm"
											>
												<RefreshCw className="h-4 w-4" />
												Retry
											</button>
										</div>
									)}
								{/* Results List with Pagination Info */}
								{!scanResultsLoading &&
									!scanResultsFetching &&
									!scanResultsError &&
									displayResults &&
									displayResults.length > 0 && (
										<div className="px-4 py-2 border-b border-secondary-700 flex items-center justify-between text-xs text-secondary-400">
											<span>
												Showing {startIndex + 1}-
												{Math.min(endIndex, totalResults)} of {totalResults}{" "}
												results
											</span>
											{totalPages > 1 && (
												<span>
													Page {currentPage} of {totalPages}
												</span>
											)}
										</div>
									)}
								{!scanResultsLoading &&
								!scanResultsFetching &&
								!scanResultsError &&
								displayResults &&
								displayResults.length > 0 ? (
									<div className="divide-y divide-secondary-700">
										{/* Grouped View */}
										{groupBySection &&
											sortedSections.map((section) => (
												<div key={section} className="bg-secondary-800/50">
													<button
														onClick={() => toggleSection(section)}
														className="w-full flex items-center gap-3 px-4 py-3 hover:bg-secondary-700/50 transition-colors"
													>
														{expandedSections[section] ? (
															<FolderOpen className="h-5 w-5 text-primary-400" />
														) : (
															<Folder className="h-5 w-5 text-secondary-400" />
														)}
														<span className="text-white font-medium">
															Section {section}
														</span>
														<span className="px-2 py-0.5 rounded-full text-xs bg-secondary-600 text-secondary-300">
															{groupedResults[section].length} rules
														</span>
														{expandedSections[section] ? (
															<ChevronDown className="h-4 w-4 text-secondary-400 ml-auto" />
														) : (
															<ChevronRight className="h-4 w-4 text-secondary-400 ml-auto" />
														)}
													</button>
													{expandedSections[section] && (
														<div className="divide-y divide-secondary-700 border-t border-secondary-700">
															{groupedResults[section].map((result) => (
																<div key={result.id} className="p-4 pl-8">
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
																				{result.compliance_rules?.title ||
																					result.rule?.title ||
																					result.title ||
																					"Unknown Rule"}
																			</p>
																			<p className="text-xs text-secondary-400">
																				{(result.compliance_rules?.severity ||
																					result.rule?.severity ||
																					result.severity) && (
																					<span
																						className={`capitalize ${
																							(
																								result.compliance_rules
																									?.severity ||
																									result.rule?.severity ||
																									result.severity
																							) === "critical"
																								? "text-red-400"
																								: (result.compliance_rules
																											?.severity ||
																											result.rule?.severity ||
																											result.severity) ===
																										"high"
																									? "text-orange-400"
																									: (result.compliance_rules
																												?.severity ||
																												result.rule?.severity ||
																												result.severity) ===
																											"medium"
																										? "text-yellow-400"
																										: "text-secondary-400"
																						}`}
																					>
																						{result.compliance_rules
																							?.severity ||
																							result.rule?.severity ||
																							result.severity}
																					</span>
																				)}
																			</p>
																		</div>
																		<span
																			className={`px-2 py-1 rounded text-xs border ${getStatusBadge(result.status)}`}
																		>
																			{result.status}
																		</span>
																	</button>

																	{expandedRules[result.id] && (
																		<div className="mt-4 ml-8 space-y-3 text-sm border-l-2 border-secondary-600 pl-4">
																			{(result.compliance_rules?.rule_ref ||
																				result.rule?.rule_ref ||
																				result.rule_id) && (
																				<div className="flex items-center gap-2 text-xs">
																					<span className="text-secondary-500">
																						Rule ID:
																					</span>
																					<code className="bg-secondary-700 px-2 py-0.5 rounded text-secondary-300 font-mono">
																						{result.compliance_rules
																							?.rule_ref ||
																							result.rule?.rule_ref ||
																							result.rule_id}
																					</code>
																				</div>
																			)}
																			{(result.compliance_rules?.description ||
																				result.rule?.description ||
																				result.description) && (
																				<div>
																					<p className="text-secondary-400 font-medium mb-1 flex items-center gap-1">
																						<Info className="h-3.5 w-3.5" />
																						Description
																					</p>
																					<p className="text-secondary-300">
																						{result.compliance_rules
																							?.description ||
																							result.rule?.description ||
																							result.description}
																					</p>
																				</div>
																			)}
																			{/* Why this failed / Why this warning - DB only: finding, actual, expected */}
																			{(result.status === "fail" ||
																				result.status === "warn") && (
																				<div
																					className={`${result.status === "warn" ? "bg-yellow-900/20 border-yellow-800/50" : "bg-red-900/20 border-red-800/50"} border rounded-lg p-3`}
																				>
																					<p
																						className={`${result.status === "warn" ? "text-yellow-400" : "text-red-400"} font-medium mb-2 flex items-center gap-1`}
																					>
																						<XCircle className="h-3.5 w-3.5" />
																						{result.status === "warn"
																							? "Why This Warning (this host)"
																							: "Why This Failed (this host)"}
																					</p>
																					<div
																						className={`${result.status === "warn" ? "text-yellow-200/90" : "text-red-200/90"} text-sm space-y-2`}
																					>
																						{result.finding ? (
																							<p>{result.finding}</p>
																						) : result.actual ||
																							result.expected ? (
																							<>
																								<p>
																									The check found a
																									non-compliant value:
																								</p>
																								<div className="mt-2 grid grid-cols-1 gap-2">
																									{result.actual && (
																										<div className="bg-red-800/30 rounded p-2">
																											<span className="text-red-300 text-xs font-medium">
																												Current setting:
																											</span>
																											<code className="block mt-1 text-red-200 break-all">
																												{result.actual}
																											</code>
																										</div>
																									)}
																									{result.expected && (
																										<div className="bg-green-800/30 rounded p-2">
																											<span className="text-green-300 text-xs font-medium">
																												Required setting:
																											</span>
																											<code className="block mt-1 text-green-200 break-all">
																												{result.expected}
																											</code>
																										</div>
																									)}
																								</div>
																							</>
																						) : (
																							<p className="text-secondary-400 text-xs italic">
																								No failure details in the
																								database (finding, actual,
																								expected are empty). The agent
																								may not be sending these fields,
																								or the backend may not be
																								storing them — check agent
																								payload and backend logs when
																								the scan was submitted.
																							</p>
																						)}
																					</div>
																				</div>
																			)}
																			{(result.compliance_rules?.rationale ||
																				result.rule?.rationale ||
																				result.rationale) && (
																				<div>
																					<p className="text-secondary-400 font-medium mb-1 flex items-center gap-1">
																						<BookOpen className="h-3.5 w-3.5" />
																						Why This Matters
																					</p>
																					<p className="text-secondary-300 text-sm leading-relaxed">
																						{result.compliance_rules
																							?.rationale ||
																							result.rule?.rationale ||
																							result.rationale}
																					</p>
																				</div>
																			)}
																			{(result.actual || result.expected) && (
																				<div className="grid grid-cols-1 md:grid-cols-2 gap-3">
																					{result.actual && (
																						<div className="bg-secondary-700/50 rounded p-2">
																							<p className="text-secondary-400 text-xs font-medium mb-1">
																								Current Value
																							</p>
																							<code className="text-red-300 text-xs break-all">
																								{result.actual}
																							</code>
																						</div>
																					)}
																					{result.expected && (
																						<div className="bg-secondary-700/50 rounded p-2">
																							<p className="text-secondary-400 text-xs font-medium mb-1">
																								Required Value
																							</p>
																							<code className="text-green-300 text-xs break-all">
																								{result.expected}
																							</code>
																						</div>
																					)}
																				</div>
																			)}
																			{/* What the fix does - for fail/warn with remediation */}
																			{(result.status === "fail" ||
																				result.status === "warn") &&
																				(result.compliance_rules?.remediation ||
																					result.rule?.remediation ||
																					result.remediation) && (
																					<div className="bg-orange-900/20 border border-orange-800/50 rounded-lg p-3">
																						<p className="text-orange-400 font-medium mb-2 flex items-center gap-1">
																							<Wrench className="h-3.5 w-3.5" />
																							What the Fix Does
																						</p>
																						<p className="text-orange-200/90 text-sm">
																							{(() => {
																								const remediation =
																									result.compliance_rules
																										?.remediation ||
																									result.rule?.remediation ||
																									result.remediation ||
																									"";
																								const title =
																									result.compliance_rules
																										?.title ||
																									result.rule?.title ||
																									result.title ||
																									"";
																								if (
																									remediation.includes(
																										"sysctl",
																									) ||
																									remediation.includes(
																										"/proc/sys",
																									)
																								)
																									return "This fix will modify kernel parameters to enable the required security setting.";
																								if (
																									remediation.includes(
																										"chmod",
																									) ||
																									remediation.includes("chown")
																								)
																									return "This fix will update file permissions or ownership to meet the required security standard.";
																								if (
																									remediation.includes("apt") ||
																									remediation.includes("yum") ||
																									remediation.includes("dnf")
																								)
																									return "This fix will install, update, or remove packages as needed to meet the security requirement.";
																								if (
																									remediation.includes(
																										"systemctl",
																									) ||
																									remediation.includes(
																										"service",
																									)
																								)
																									return "This fix will enable, disable, or configure a system service to meet the security requirement.";
																								if (
																									remediation.includes(
																										"/etc/ssh",
																									)
																								)
																									return "This fix will update SSH daemon configuration to harden remote access security.";
																								if (
																									remediation.includes(
																										"audit",
																									) ||
																									remediation.includes("auditd")
																								)
																									return "This fix will configure audit logging to track security-relevant system events.";
																								if (
																									remediation.includes("pam") ||
																									remediation.includes(
																										"/etc/pam",
																									)
																								)
																									return "This fix will configure authentication modules to enforce stronger access controls.";
																								if (
																									title
																										.toLowerCase()
																										.includes("password")
																								)
																									return "This fix will update password policy settings to require stronger passwords or enforce better credential management.";
																								if (
																									remediation.includes(
																										"iptables",
																									) ||
																									remediation.includes(
																										"nftables",
																									) ||
																									title
																										.toLowerCase()
																										.includes("firewall")
																								)
																									return "This fix will configure firewall rules to restrict network access and improve security.";
																								return "This fix will apply the recommended configuration change to bring your system into compliance with the security benchmark.";
																							})()}
																						</p>
																					</div>
																				)}
																			{(result.compliance_rules?.remediation ||
																				result.rule?.remediation ||
																				result.remediation) && (
																				<div>
																					<p className="text-secondary-400 font-medium mb-1 flex items-center gap-1">
																						<Wrench className="h-3.5 w-3.5" />
																						Remediation
																					</p>
																					<pre className="text-secondary-300 whitespace-pre-wrap bg-secondary-700/30 rounded p-2 text-xs font-mono overflow-x-auto">
																						{result.compliance_rules
																							?.remediation ||
																							result.rule?.remediation ||
																							result.remediation}
																					</pre>
																				</div>
																			)}
																		</div>
																	)}
																</div>
															))}
														</div>
													)}
												</div>
											))}
										{/* Flat View */}
										{!groupBySection &&
											paginatedResults.map((result) => (
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
																{result.compliance_rules?.title ||
																	result.rule?.title ||
																	result.title ||
																	"Unknown Rule"}
															</p>
															<p className="text-xs text-secondary-400">
																{(result.compliance_rules?.section ||
																	result.rule?.section ||
																	result.section) &&
																	`${result.compliance_rules?.section || result.rule?.section || result.section} • `}
																{(result.compliance_rules?.severity ||
																	result.rule?.severity ||
																	result.severity) && (
																	<span
																		className={`capitalize ${
																			(
																				result.compliance_rules?.severity ||
																					result.rule?.severity ||
																					result.severity
																			) === "critical"
																				? "text-red-400"
																				: (result.compliance_rules?.severity ||
																							result.rule?.severity ||
																							result.severity) === "high"
																					? "text-orange-400"
																					: (result.compliance_rules
																								?.severity ||
																								result.rule?.severity ||
																								result.severity) === "medium"
																						? "text-yellow-400"
																						: "text-secondary-400"
																		}`}
																	>
																		{result.compliance_rules?.severity ||
																			result.rule?.severity ||
																			result.severity}
																	</span>
																)}
															</p>
														</div>
														<span
															className={`px-2 py-1 rounded text-xs border ${getStatusBadge(result.status)}`}
														>
															{result.status}
														</span>
													</button>

													{expandedRules[result.id] && (
														<div className="mt-4 ml-8 space-y-3 text-sm border-l-2 border-secondary-600 pl-4">
															{/* Rule ID reference */}
															{(result.compliance_rules?.rule_ref ||
																result.rule?.rule_ref ||
																result.rule_id) && (
																<div className="flex items-center gap-2 text-xs">
																	<span className="text-secondary-500">
																		Rule ID:
																	</span>
																	<code className="bg-secondary-700 px-2 py-0.5 rounded text-secondary-300 font-mono">
																		{result.compliance_rules?.rule_ref ||
																			result.rule?.rule_ref ||
																			result.rule_id}
																	</code>
																</div>
															)}
															{(result.compliance_rules?.description ||
																result.rule?.description ||
																result.description) && (
																<div>
																	<p className="text-secondary-400 font-medium mb-1 flex items-center gap-1">
																		<Info className="h-3.5 w-3.5" />
																		Description
																	</p>
																	<p className="text-secondary-300">
																		{result.compliance_rules?.description ||
																			result.rule?.description ||
																			result.description}
																	</p>
																</div>
															)}

															{/* WHY THIS FAILED/WARNED - DB only: finding, actual, expected (no fallback to description) */}
															{(result.status === "fail" ||
																result.status === "warn") && (
																<div
																	className={`${result.status === "warn" ? "bg-yellow-900/20 border-yellow-800/50" : "bg-red-900/20 border-red-800/50"} border rounded-lg p-3`}
																>
																	<p
																		className={`${result.status === "warn" ? "text-yellow-400" : "text-red-400"} font-medium mb-2 flex items-center gap-1`}
																	>
																		<XCircle className="h-3.5 w-3.5" />
																		{result.status === "warn"
																			? "Why This Warning (this host)"
																			: "Why This Failed (this host)"}
																	</p>
																	<div
																		className={`${result.status === "warn" ? "text-yellow-200/90" : "text-red-200/90"} text-sm space-y-2`}
																	>
																		{result.finding ? (
																			<p>{result.finding}</p>
																		) : result.actual || result.expected ? (
																			<>
																				<p>
																					The check found a non-compliant value:
																				</p>
																				<div className="mt-2 grid grid-cols-1 gap-2">
																					{result.actual && (
																						<div className="bg-red-800/30 rounded p-2">
																							<span className="text-red-300 text-xs font-medium">
																								Current setting:
																							</span>
																							<code className="block mt-1 text-red-200 break-all">
																								{result.actual}
																							</code>
																						</div>
																					)}
																					{result.expected && (
																						<div className="bg-green-800/30 rounded p-2">
																							<span className="text-green-300 text-xs font-medium">
																								Required setting:
																							</span>
																							<code className="block mt-1 text-green-200 break-all">
																								{result.expected}
																							</code>
																						</div>
																					)}
																				</div>
																			</>
																		) : (
																			<p className="text-secondary-400 text-xs italic">
																				No failure details in the database
																				(finding, actual, expected are empty).
																				The agent may not be sending these
																				fields, or the backend may not be
																				storing them — check agent payload and
																				backend logs when the scan was
																				submitted.
																			</p>
																		)}
																	</div>
																</div>
															)}

															{/* Rationale - explains WHY this rule matters */}
															{(result.compliance_rules?.rationale ||
																result.rule?.rationale ||
																result.rationale) && (
																<div>
																	<p className="text-secondary-400 font-medium mb-1 flex items-center gap-1">
																		<BookOpen className="h-3.5 w-3.5" />
																		Why This Matters
																	</p>
																	<p className="text-secondary-300 text-sm leading-relaxed">
																		{result.compliance_rules?.rationale ||
																			result.rule?.rationale ||
																			result.rationale}
																	</p>
																</div>
															)}
															{/* Show actual vs expected for clearer understanding */}
															{(result.actual || result.expected) && (
																<div className="grid grid-cols-1 md:grid-cols-2 gap-3">
																	{result.actual && (
																		<div className="bg-secondary-700/50 rounded p-2">
																			<p className="text-secondary-400 text-xs font-medium mb-1">
																				Current Value
																			</p>
																			<code className="text-red-300 text-xs break-all">
																				{result.actual}
																			</code>
																		</div>
																	)}
																	{result.expected && (
																		<div className="bg-secondary-700/50 rounded p-2">
																			<p className="text-secondary-400 text-xs font-medium mb-1">
																				Required Value
																			</p>
																			<code className="text-green-300 text-xs break-all">
																				{result.expected}
																			</code>
																		</div>
																	)}
																</div>
															)}

															{/* WHAT THE FIX DOES - Explanation before remediation */}
															{(result.status === "fail" ||
																result.status === "warn") &&
																(result.compliance_rules?.remediation ||
																	result.rule?.remediation ||
																	result.remediation) && (
																	<div className="bg-orange-900/20 border border-orange-800/50 rounded-lg p-3">
																		<p className="text-orange-400 font-medium mb-2 flex items-center gap-1">
																			<Wrench className="h-3.5 w-3.5" />
																			What the Fix Does
																		</p>
																		<div className="text-orange-200/90 text-sm space-y-2">
																			<p>
																				{(() => {
																					const remediation =
																						result.compliance_rules
																							?.remediation ||
																						result.rule?.remediation ||
																						result.remediation ||
																						"";
																					const title =
																						result.compliance_rules?.title ||
																						result.rule?.title ||
																						"";
																					// Generate a user-friendly description based on the rule
																					if (
																						remediation.includes("sysctl") ||
																						remediation.includes("/proc/sys")
																					) {
																						return "This fix will modify kernel parameters to enable the required security setting. Changes are applied immediately and persist across reboots.";
																					} else if (
																						remediation.includes("chmod") ||
																						remediation.includes("chown")
																					) {
																						return "This fix will update file permissions or ownership to meet the required security standard. This restricts unauthorized access to sensitive files.";
																					} else if (
																						remediation.includes("apt") ||
																						remediation.includes("yum") ||
																						remediation.includes("dnf")
																					) {
																						return "This fix will install, update, or remove packages as needed to meet the security requirement.";
																					} else if (
																						remediation.includes("systemctl") ||
																						remediation.includes("service")
																					) {
																						return "This fix will enable, disable, or configure a system service to meet the security requirement.";
																					} else if (
																						remediation.includes("/etc/ssh")
																					) {
																						return "This fix will update SSH daemon configuration to harden remote access security.";
																					} else if (
																						remediation.includes("audit") ||
																						remediation.includes("auditd")
																					) {
																						return "This fix will configure audit logging to track security-relevant system events.";
																					} else if (
																						remediation.includes("pam") ||
																						remediation.includes("/etc/pam")
																					) {
																						return "This fix will configure authentication modules to enforce stronger access controls.";
																					} else if (
																						title
																							.toLowerCase()
																							.includes("password")
																					) {
																						return "This fix will update password policy settings to require stronger passwords or enforce better credential management.";
																					} else if (
																						title
																							.toLowerCase()
																							.includes("firewall") ||
																						remediation.includes("iptables") ||
																						remediation.includes("nftables")
																					) {
																						return "This fix will configure firewall rules to restrict network access and improve security.";
																					} else {
																						return "This fix will apply the recommended configuration change to bring your system into compliance with the security benchmark.";
																					}
																				})()}
																			</p>
																		</div>
																	</div>
																)}

															{(result.compliance_rules?.remediation ||
																result.rule?.remediation ||
																result.remediation) && (
																<div>
																	<p className="text-secondary-400 font-medium mb-1 flex items-center gap-1">
																		<Wrench className="h-3.5 w-3.5" />
																		Remediation Steps
																	</p>
																	<pre className="text-secondary-300 whitespace-pre-wrap bg-secondary-700/30 rounded p-2 text-xs font-mono overflow-x-auto">
																		{result.compliance_rules?.remediation ||
																			result.rule?.remediation ||
																			result.remediation}
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
																			const ruleRef =
																				result.compliance_rules?.rule_ref ||
																				result.rule?.rule_ref ||
																				result.rule_ref;
																			console.log(
																				"[Compliance] Remediate click:",
																				{
																					ruleRef,
																					compliance_rules:
																						result.compliance_rules,
																					result,
																				},
																			);
																			if (ruleRef) {
																				remediateRuleMutation.mutate(ruleRef);
																			} else {
																				console.error(
																					"[Compliance] No rule_ref found for remediation",
																				);
																			}
																		}}
																		disabled={
																			remediatingRule ===
																				(result.compliance_rules?.rule_ref ||
																					result.rule?.rule_ref ||
																					result.rule_ref ||
																					result.rule_id) ||
																			remediateRuleMutation.isPending
																		}
																		className={`flex items-center gap-2 px-3 py-1.5 rounded-lg text-sm font-medium transition-colors ${
																			remediatingRule ===
																			(
																				result.compliance_rules?.rule_ref ||
																					result.rule?.rule_ref ||
																					result.rule_ref ||
																					result.rule_id
																			)
																				? "bg-orange-600/50 text-orange-200 cursor-wait"
																				: "bg-orange-600 hover:bg-orange-500 text-white"
																		}`}
																	>
																		{remediatingRule ===
																		(result.compliance_rules?.rule_ref ||
																			result.rule?.rule_ref ||
																			result.rule_ref ||
																			result.rule_id) ? (
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
																		Attempts to automatically remediate this
																		specific rule
																	</p>
																</div>
															)}
														</div>
													)}
												</div>
											))}
										{/* Pagination Controls */}
										{totalPages > 1 && (
											<div className="px-4 py-3 border-t border-secondary-700 flex items-center justify-between">
												<div className="flex items-center gap-2">
													<button
														onClick={() => setCurrentPage(1)}
														disabled={currentPage === 1}
														className="px-2 py-1 text-xs rounded bg-secondary-700 text-secondary-300 hover:bg-secondary-600 disabled:opacity-50 disabled:cursor-not-allowed"
													>
														First
													</button>
													<button
														onClick={() =>
															setCurrentPage((p) => Math.max(1, p - 1))
														}
														disabled={currentPage === 1}
														className="p-1.5 rounded bg-secondary-700 text-secondary-300 hover:bg-secondary-600 disabled:opacity-50 disabled:cursor-not-allowed"
													>
														<ChevronLeft className="h-4 w-4" />
													</button>
												</div>
												<div className="flex items-center gap-1">
													{/* Show page numbers with ellipsis for large page counts */}
													{Array.from({ length: totalPages }, (_, i) => i + 1)
														.filter((page) => {
															// Show first, last, current, and pages near current
															if (page === 1 || page === totalPages)
																return true;
															if (Math.abs(page - currentPage) <= 1)
																return true;
															return false;
														})
														.map((page, index, filteredPages) => (
															<>
																{index > 0 &&
																	filteredPages[index - 1] !== page - 1 && (
																		<span
																			key={`ellipsis-${page}`}
																			className="px-1 text-secondary-500"
																		>
																			...
																		</span>
																	)}
																<button
																	key={page}
																	onClick={() => setCurrentPage(page)}
																	className={`px-3 py-1 text-sm rounded ${
																		currentPage === page
																			? "bg-primary-600 text-white"
																			: "bg-secondary-700 text-secondary-300 hover:bg-secondary-600"
																	}`}
																>
																	{page}
																</button>
															</>
														))}
												</div>
												<div className="flex items-center gap-2">
													<button
														onClick={() =>
															setCurrentPage((p) => Math.min(totalPages, p + 1))
														}
														disabled={currentPage === totalPages}
														className="p-1.5 rounded bg-secondary-700 text-secondary-300 hover:bg-secondary-600 disabled:opacity-50 disabled:cursor-not-allowed"
													>
														<ChevronRight className="h-4 w-4" />
													</button>
													<button
														onClick={() => setCurrentPage(totalPages)}
														disabled={currentPage === totalPages}
														className="px-2 py-1 text-xs rounded bg-secondary-700 text-secondary-300 hover:bg-secondary-600 disabled:opacity-50 disabled:cursor-not-allowed"
													>
														Last
													</button>
												</div>
											</div>
										)}
									</div>
								) : !scanResultsLoading &&
									!scanResultsFetching &&
									!scanResultsError ? (
									<div className="p-8 text-center">
										<p className="text-secondary-400">
											No {statusFilter !== "all" ? statusFilter : ""} results
											found for this filter. Try another tab (e.g. Passed or
											Warnings).
										</p>
									</div>
								) : null}
							</div>
						)}
					</>
				) : (
					<div className="bg-secondary-800 rounded-lg border border-secondary-700 p-12 text-center">
						<ListChecks className="h-12 w-12 text-secondary-600 mx-auto mb-4" />
						<h3 className="text-lg font-medium text-white mb-2">
							No Results Yet
						</h3>
						<p className="text-secondary-400 mb-4">
							Run a compliance scan to see detailed results
						</p>
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
	const renderHistory = () => {
		// Group scans by profile type for better display
		const groupedByType =
			scanHistory?.scans?.reduce((acc, scan) => {
				const type = scan.compliance_profiles?.type || "unknown";
				if (!acc[type]) acc[type] = [];
				acc[type].push(scan);
				return acc;
			}, {}) || {};

		const typeLabels = {
			openscap: { name: "OpenSCAP", color: "bg-green-600", icon: Shield },
			"docker-bench": {
				name: "Docker Bench",
				color: "bg-blue-600",
				icon: Container,
			},
			unknown: { name: "Other", color: "bg-secondary-600", icon: Shield },
		};

		return (
			<div className="space-y-4">
				{/* Summary Cards by Type */}
				{Object.keys(groupedByType).length > 0 && (
					<div className="grid grid-cols-1 md:grid-cols-2 gap-4">
						{Object.entries(groupedByType).map(([type, scans]) => {
							const typeInfo = typeLabels[type] || typeLabels.unknown;
							const latestScan = scans[0];
							const Icon = typeInfo.icon;
							return (
								<div
									key={type}
									className="bg-secondary-800 rounded-lg border border-secondary-700 p-4"
								>
									<div className="flex items-center gap-3 mb-3">
										<div className={`p-2 rounded-lg ${typeInfo.color}`}>
											<Icon className="h-5 w-5 text-white" />
										</div>
										<div>
											<h4 className="text-white font-medium">
												{typeInfo.name}
											</h4>
											<p className="text-xs text-secondary-400">
												{scans.length} scan{scans.length !== 1 ? "s" : ""}
											</p>
										</div>
									</div>
									{latestScan && (
										<div className="flex items-center justify-between">
											<ComplianceScore score={latestScan.score} size="sm" />
											<div className="text-right text-sm">
												<div className="flex items-center gap-2">
													<span className="text-green-400">
														{latestScan.passed} passed
													</span>
													{type === "docker-bench" ? (
														<span className="text-yellow-400">
															{latestScan.warnings} warn
														</span>
													) : (
														<span className="text-red-400">
															{latestScan.failed} failed
														</span>
													)}
												</div>
												<p className="text-xs text-secondary-500">
													{new Date(
														latestScan.completed_at,
													).toLocaleDateString()}
												</p>
											</div>
										</div>
									)}
								</div>
							);
						})}
					</div>
				)}

				{/* Full History List */}
				{scanHistory?.scans && scanHistory.scans.length > 0 ? (
					<div className="bg-secondary-800 rounded-lg border border-secondary-700 divide-y divide-secondary-700">
						{scanHistory.scans.map((scan, _index) => {
							const type = scan.compliance_profiles?.type || "unknown";
							const typeInfo = typeLabels[type] || typeLabels.unknown;
							const isDockerBench = type === "docker-bench";
							// Find if this is the latest scan for its type
							const isLatestOfType = groupedByType[type]?.[0]?.id === scan.id;

							return (
								<div
									key={scan.id}
									className={`p-4 ${isLatestOfType ? "bg-primary-900/10" : ""}`}
								>
									<div className="flex items-center justify-between">
										<div className="flex items-center gap-4">
											<ComplianceScore score={scan.score} size="sm" />
											<div>
												<div className="flex items-center gap-2 flex-wrap">
													<span
														className={`px-2 py-0.5 text-xs ${typeInfo.color} text-white rounded`}
													>
														{typeInfo.name}
													</span>
													<p className="text-white font-medium">
														{scan.compliance_profiles?.name ||
															"Compliance Scan"}
													</p>
													{isLatestOfType && (
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
												<span className="text-green-400">
													{scan.passed} passed
												</span>
												{isDockerBench ? (
													<span className="text-yellow-400">
														{scan.warnings} warn
													</span>
												) : (
													<span className="text-red-400">
														{scan.failed} failed
													</span>
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
							);
						})}
					</div>
				) : (
					<div className="bg-secondary-800 rounded-lg border border-secondary-700 p-12 text-center">
						<History className="h-12 w-12 text-secondary-600 mx-auto mb-4" />
						<h3 className="text-lg font-medium text-white mb-2">
							No Scan History
						</h3>
						<p className="text-secondary-400 mb-4">
							Previous scans will appear here
						</p>
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
					<Suspense
						fallback={
							<div className="h-48 bg-secondary-800 rounded-lg border border-secondary-700 animate-pulse" />
						}
					>
						<ComplianceTrend hostId={hostId} />
					</Suspense>
				)}
			</div>
		);
	};

	// Render Settings subtab
	const renderSettings = () => {
		const status = integrationStatus?.status;
		const components = status?.components || {};
		const info = status?.scanner_info;

		// Detect mismatches between enabled integrations and available services
		const openscapAvailable =
			components.openscap === "ready" ||
			info?.openscap_available ||
			info?.openscap_version;
		const dockerBenchAvailable =
			components["docker-bench"] === "ready" || info?.docker_bench_available;
		const _oscapDockerAvailable =
			components["oscap-docker"] === "ready" || info?.oscap_docker_available;

		// Mismatch: compliance enabled but scanner not available
		const complianceMismatch =
			complianceEnabled && status && !openscapAvailable;
		// Mismatch: docker enabled but docker-bench not available (only if compliance is also enabled)
		const dockerMismatch =
			dockerEnabled &&
			complianceEnabled &&
			status &&
			!dockerBenchAvailable &&
			components["docker-bench"] !== "unavailable";

		return (
			<div className="space-y-6">
				{/* Integration Mismatch Warnings */}
				{complianceMismatch && (
					<div className="p-4 bg-red-900/30 border border-red-700 rounded-lg">
						<div className="flex items-start gap-3">
							<AlertTriangle className="h-5 w-5 text-red-400 flex-shrink-0 mt-0.5" />
							<div>
								<h4 className="font-medium text-red-300">
									Compliance Enabled but Scanner Unavailable
								</h4>
								<p className="text-sm text-red-200/80 mt-1">
									Compliance integration is enabled for this host, but OpenSCAP
									is not installed or available on the agent. The agent will
									attempt to install it automatically, or you can manually
									install it:
								</p>
								<code className="block mt-2 p-2 bg-red-900/50 rounded text-xs text-red-200 font-mono">
									# Debian/Ubuntu: apt install openscap-scanner
									scap-security-guide
									<br /># RHEL/CentOS: yum install openscap-scanner
									scap-security-guide
								</code>
							</div>
						</div>
					</div>
				)}

				{dockerMismatch && (
					<div className="p-4 bg-yellow-900/30 border border-yellow-700 rounded-lg">
						<div className="flex items-start gap-3">
							<AlertTriangle className="h-5 w-5 text-yellow-400 flex-shrink-0 mt-0.5" />
							<div>
								<h4 className="font-medium text-yellow-300">
									Docker Integration Enabled but Docker Bench Unavailable
								</h4>
								<p className="text-sm text-yellow-200/80 mt-1">
									Docker integration is enabled, but Docker Bench for Security
									is not available. Ensure Docker is installed and running on
									the agent host.
								</p>
							</div>
						</div>
					</div>
				)}

				{/* No Status Available Warning */}
				{complianceEnabled && !status && (
					<div className="p-4 bg-blue-900/30 border border-blue-700 rounded-lg">
						<div className="flex items-start gap-3">
							<Info className="h-5 w-5 text-blue-400 flex-shrink-0 mt-0.5" />
							<div>
								<h4 className="font-medium text-blue-300">
									Waiting for Agent Status Report
								</h4>
								<p className="text-sm text-blue-200/80 mt-1">
									Compliance is enabled but the agent hasn't reported its
									scanner status yet. The agent reports status on startup and
									periodically (default every 30 minutes). Try refreshing or
									wait for the next check-in.
								</p>
							</div>
						</div>
					</div>
				)}

				{/* Scanner Type Toggles */}
				<div className="bg-secondary-800 rounded-lg border border-secondary-700 p-6">
					<h3 className="text-lg font-medium text-white flex items-center gap-2 mb-4">
						<Settings className="h-5 w-5 text-primary-400" />
						Scanner Types
					</h3>
					<div className="space-y-3">
						<label className="flex items-center justify-between gap-3 p-3 rounded-lg bg-secondary-700/40 cursor-pointer">
							<div>
								<span className="text-sm font-medium text-white">OpenSCAP</span>
								<p className="text-xs text-secondary-400 mt-0.5">
									CIS Benchmarks, STIG, and other SCAP-based compliance checks
								</p>
							</div>
							<input
								type="checkbox"
								checked={scannerToggles.openscap}
								onChange={(e) => {
									const val = e.target.checked;
									setScannerToggles((prev) => ({ ...prev, openscap: val }));
									scannerToggleMutation.mutate({ openscap_enabled: val });
								}}
								className="h-5 w-5 rounded border-secondary-500 text-primary-600 focus:ring-primary-500 bg-secondary-700"
							/>
						</label>
						<label
							className={`flex items-center justify-between gap-3 p-3 rounded-lg bg-secondary-700/40 ${!dockerEnabled ? "opacity-60" : "cursor-pointer"}`}
						>
							<div>
								<span className="text-sm font-medium text-white">
									Docker Bench
								</span>
								<p className="text-xs text-secondary-400 mt-0.5">
									CIS Docker Benchmark security checks
									{!dockerEnabled &&
										" — requires Docker integration to be enabled"}
								</p>
							</div>
							<input
								type="checkbox"
								checked={scannerToggles.docker_bench}
								disabled={!dockerEnabled}
								onChange={(e) => {
									const val = e.target.checked;
									setScannerToggles((prev) => ({ ...prev, docker_bench: val }));
									scannerToggleMutation.mutate({ docker_bench_enabled: val });
								}}
								className="h-5 w-5 rounded border-secondary-500 text-primary-600 focus:ring-primary-500 bg-secondary-700 disabled:opacity-40"
							/>
						</label>
					</div>
				</div>

				{/* Scanner Status */}
				<div className="bg-secondary-800 rounded-lg border border-secondary-700 p-6">
					<div className="flex items-center justify-between mb-4">
						<h3 className="text-lg font-medium text-white flex items-center gap-2">
							<Shield className="h-5 w-5 text-primary-400" />
							Scanner Status
						</h3>
						<button
							onClick={() => refetchStatus()}
							disabled={
								isRefreshingStatus ||
								ssgUpgradeMutation.isPending ||
								ssgUpgradeMessage
							}
							className={`p-2 hover:bg-secondary-700 rounded-lg transition-colors ${isRefreshingStatus || ssgUpgradeMutation.isPending || ssgUpgradeMessage ? "cursor-wait" : ""}`}
							title={
								isRefreshingStatus ||
								ssgUpgradeMutation.isPending ||
								ssgUpgradeMessage
									? "Refreshing..."
									: "Refresh status"
							}
						>
							<RefreshCw
								className={`h-4 w-4 ${isRefreshingStatus || ssgUpgradeMutation.isPending || ssgUpgradeMessage ? "text-primary-400 animate-spin" : "text-secondary-400"}`}
							/>
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
									<p className="text-white font-medium capitalize">
										{status.status || "Unknown"}
									</p>
									{status.message && (
										<p className="text-sm text-secondary-400">
											{status.message}
										</p>
									)}
								</div>
							</div>

							{/* Content Mismatch Warning */}
							{info?.content_mismatch && (
								<div className="p-3 bg-orange-900/30 border border-orange-700 rounded-lg">
									<div className="flex items-center gap-2 text-orange-300">
										<AlertTriangle className="h-4 w-4" />
										<span className="font-medium">
											Content Version Mismatch
										</span>
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
											<p className="text-xs text-secondary-400">
												CIS Benchmark Scanning
											</p>
										</div>
									</div>
									<div className="space-y-2 text-sm">
										<div className="flex justify-between">
											<span className="text-secondary-400">Status</span>
											<span
												className={`capitalize ${
													components.openscap === "ready" ||
													info?.openscap_available ||
													info?.openscap_version
														? "text-green-400"
														: components.openscap === "installing"
															? "text-blue-400"
															: components.openscap === "error"
																? "text-red-400"
																: "text-secondary-400"
												}`}
											>
												{components.openscap ||
													(info?.openscap_available || info?.openscap_version
														? "Ready"
														: "Not installed")}
											</span>
										</div>
										<div className="flex justify-between">
											<span className="text-secondary-400">Version</span>
											<span className="text-secondary-300 font-mono text-xs">
												{info?.openscap_version || "N/A"}
											</span>
										</div>
										<div className="flex justify-between">
											<span className="text-secondary-400">
												Content Package
											</span>
											<span className="text-secondary-300 font-mono text-xs">
												{info?.content_package || "N/A"}
											</span>
										</div>
										<div className="flex justify-between">
											<span className="text-secondary-400">Content File</span>
											<span
												className="text-secondary-300 font-mono text-xs truncate max-w-[180px]"
												title={info?.content_file}
											>
												{info?.content_file || "N/A"}
											</span>
										</div>
										{info?.ssg_version && (
											<div className="flex justify-between">
												<span className="text-secondary-400">SSG Version</span>
												<span
													className={`font-mono text-xs ${info?.ssg_needs_upgrade ? "text-yellow-400" : "text-secondary-300"}`}
												>
													{info.ssg_version}
													{info?.ssg_needs_upgrade &&
														` (min: ${info.ssg_min_version})`}
												</span>
											</div>
										)}
										{info?.ssg_needs_upgrade && (
											<div className="mt-3 p-2 bg-yellow-600/20 border border-yellow-600/40 rounded-lg">
												<p className="text-yellow-400 text-xs mb-2">
													{info.ssg_upgrade_message ||
														"SSG content upgrade recommended"}
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
											<div
												className={`mt-2 p-2 rounded text-xs ${
													ssgUpgradeMessage.type === "success"
														? "bg-green-600/20 text-green-400 border border-green-600/40"
														: "bg-red-600/20 text-red-400 border border-red-600/40"
												}`}
											>
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
											<p className="text-white font-medium">
												System Information
											</p>
											<p className="text-xs text-secondary-400">
												Detected OS Details
											</p>
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
											<p className="text-xs text-secondary-400">
												CIS Docker Benchmark
											</p>
										</div>
									</div>
									<div className="space-y-2 text-sm">
										<div className="flex justify-between">
											<span className="text-secondary-400">Status</span>
											<span
												className={`capitalize ${
													components["docker-bench"] === "ready"
														? "text-green-400"
														: components["docker-bench"] === "installing"
															? "text-blue-400"
															: components["docker-bench"] === "unavailable"
																? "text-secondary-500"
																: components["docker-bench"] === "error"
																	? "text-red-400"
																	: "text-secondary-400"
												}`}
											>
												{components["docker-bench"] || "Not configured"}
											</span>
										</div>
										<div className="flex justify-between">
											<span className="text-secondary-400">Available</span>
											<span
												className={
													info?.docker_bench_available
														? "text-green-400"
														: "text-secondary-500"
												}
											>
												{info?.docker_bench_available ? "Yes" : "No"}
											</span>
										</div>
										<div className="flex justify-between">
											<span className="text-secondary-400">Requirement</span>
											<span className="text-secondary-300 text-xs">
												Docker Integration enabled
											</span>
										</div>
									</div>
								</div>

								{/* oscap-docker Component - Only show when available or potentially available (not on Ubuntu/Debian) */}
								{components["oscap-docker"] !== "unavailable" && (
									<div className="bg-secondary-700/30 rounded-lg p-4 border border-secondary-600">
										<div className="flex items-center gap-3 mb-4">
											<div className="p-2 bg-orange-600/20 rounded-lg">
												<Package className="h-5 w-5 text-orange-400" />
											</div>
											<div>
												<p className="text-white font-medium">oscap-docker</p>
												<p className="text-xs text-secondary-400">
													Docker Image CVE Scanning
												</p>
											</div>
										</div>
										<div className="space-y-2 text-sm">
											<div className="flex justify-between">
												<span className="text-secondary-400">Status</span>
												<span
													className={`capitalize ${
														components["oscap-docker"] === "ready"
															? "text-green-400"
															: components["oscap-docker"] === "installing"
																? "text-blue-400"
																: components["oscap-docker"] === "error"
																	? "text-red-400"
																	: "text-secondary-400"
													}`}
												>
													{components["oscap-docker"] || "Not configured"}
												</span>
											</div>
											<div className="flex justify-between">
												<span className="text-secondary-400">Available</span>
												<span
													className={
														info?.oscap_docker_available
															? "text-green-400"
															: "text-secondary-500"
													}
												>
													{info?.oscap_docker_available ? "Yes" : "No"}
												</span>
											</div>
											<div className="flex justify-between">
												<span className="text-secondary-400">Requirement</span>
												<span className="text-secondary-300 text-xs">
													Docker + Compliance enabled
												</span>
											</div>
										</div>
									</div>
								)}

								{/* Available Profiles */}
								<div className="bg-secondary-700/30 rounded-lg p-4 border border-secondary-600">
									<div className="flex items-center gap-3 mb-4">
										<div className="p-2 bg-purple-600/20 rounded-lg">
											<ListChecks className="h-5 w-5 text-purple-400" />
										</div>
										<div>
											<p className="text-white font-medium">
												Available Profiles
											</p>
											<p className="text-xs text-secondary-400">
												Scan options from agent
											</p>
										</div>
									</div>
									<div className="space-y-2">
										{info?.available_profiles?.length > 0 ? (
											info.available_profiles.map((profile, idx) => (
												<div
													key={`profile-${idx}-${profile || ""}`}
													className="flex items-center justify-between text-sm"
												>
													<span className="text-secondary-300">
														{profile.name}
													</span>
													<span
														className={`px-2 py-0.5 text-xs rounded ${
															profile.type === "docker-bench"
																? "bg-blue-900/30 text-blue-400"
																: profile.type === "oscap-docker"
																	? "bg-orange-900/30 text-orange-400"
																	: "bg-green-900/30 text-green-400"
														}`}
													>
														{profile.type}
													</span>
												</div>
											))
										) : (
											<p className="text-secondary-500 text-sm">
												No profiles available
											</p>
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
							PatchMon uses industry-standard compliance scanning tools to
							evaluate your systems against security benchmarks.
						</p>
						<div className="grid grid-cols-1 md:grid-cols-2 gap-4">
							<div className="p-3 bg-secondary-700/30 rounded-lg">
								<p className="text-white font-medium mb-1">OpenSCAP (oscap)</p>
								<p className="text-secondary-400 text-xs">
									Scans against CIS Benchmarks for Linux distributions.
									Evaluates system configuration, file permissions, and security
									settings.
								</p>
							</div>
							<div className="p-3 bg-secondary-700/30 rounded-lg">
								<p className="text-white font-medium mb-1">
									Docker Bench for Security
								</p>
								<p className="text-secondary-400 text-xs">
									Checks Docker host and container configurations against CIS
									Docker Benchmark recommendations.
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
							This updates compliance rules, benchmarks, and remediation
							scripts.
						</p>
						{updateMessage && (
							<div
								className={`p-3 rounded-lg ${
									updateMessage.type === "success"
										? "bg-green-900/30 border border-green-700 text-green-300"
										: "bg-red-900/30 border border-red-700 text-red-300"
								}`}
							>
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
							{ssgUpdateMutation.isPending
								? "Updating Security Content..."
								: "Update SSG Content"}
						</button>
						{!isConnected && (
							<p className="text-xs text-secondary-500">
								Agent must be connected to update security content
							</p>
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
							<h4 className="text-green-400 font-medium mb-2">
								Level 1 - Essential Security
							</h4>
							<ul className="text-sm text-green-200/80 space-y-1 list-disc list-inside">
								<li>
									Practical security measures with minimal service disruption
								</li>
								<li>Suitable for most production environments</li>
								<li>
									Covers essential hardening: password policies, file
									permissions, network settings
								</li>
								<li>Recommended as baseline for all systems</li>
							</ul>
						</div>
						<div className="p-4 bg-orange-900/20 border border-orange-800/50 rounded-lg">
							<h4 className="text-orange-400 font-medium mb-2">
								Level 2 - Defense in Depth
							</h4>
							<ul className="text-sm text-orange-200/80 space-y-1 list-disc list-inside">
								<li>Extended security for high-security environments</li>
								<li>May impact functionality - test before applying</li>
								<li>
									Includes stricter controls: audit logging, kernel hardening,
									additional restrictions
								</li>
								<li>Recommended for systems handling sensitive data</li>
							</ul>
						</div>
						<div className="p-4 bg-purple-900/20 border border-purple-800/50 rounded-lg">
							<h4 className="text-purple-400 font-medium mb-2">
								Other Profiles (STIG, PCI-DSS, HIPAA)
							</h4>
							<ul className="text-sm text-purple-200/80 space-y-1 list-disc list-inside">
								<li>
									<strong>STIG:</strong> DoD Security Technical Implementation
									Guides
								</li>
								<li>
									<strong>PCI-DSS:</strong> Payment Card Industry Data Security
									Standard
								</li>
								<li>
									<strong>HIPAA:</strong> Health Insurance Portability and
									Accountability Act
								</li>
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
						<li>
							If scans show all "N/A" results, the SCAP content may not match
							your OS version
						</li>
						<li>
							Try disabling and re-enabling compliance to upgrade packages
						</li>
						<li>
							Docker Bench requires Docker integration to be enabled first
						</li>
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
							<strong>CIS/STIG content for Ubuntu 24.04</strong> is available in
							SCAP Security Guide v0.1.76+. If you see "content mismatch"
							warnings, your ssg-base package needs updating.
						</p>
						<p>Options for Ubuntu 24.04 compliance:</p>
						<ul className="list-disc list-inside ml-2 space-y-1">
							<li>
								<strong>Update ssg-base package</strong> to v0.1.76 or higher
							</li>
							<li>
								<strong>Canonical's Ubuntu Security Guide (USG)</strong> -
								Official CIS hardening with Ubuntu Pro
							</li>
							<li>
								<strong>OVAL Vulnerability Scanning</strong> - Free CVE content
								from Canonical
							</li>
						</ul>
						<p className="mt-2 text-xs text-blue-300/70">
							Note: USG provides{" "}
							<code className="bg-blue-800/50 px-1 rounded">usg audit</code> and{" "}
							<code className="bg-blue-800/50 px-1 rounded">usg fix</code>{" "}
							commands for CIS Level 1/2 hardening.
						</p>
					</div>
				</div>
			</div>
		);
	};

	const status = integrationStatus?.status;
	const scanner_info = status?.scanner_info;
	const components = status?.components || {};
	const openscap_ready =
		components.openscap === "ready" ||
		scanner_info?.openscap_available ||
		scanner_info?.openscap_version;
	const status_installing = status?.status === "installing";
	const status_ready = status?.status === "ready";
	const install_job_in_progress =
		installJobData?.status === "active" || installJobData?.status === "waiting";
	const show_install_progress = status_installing || install_job_in_progress;
	const show_install_button =
		!openscap_ready &&
		isConnected &&
		!installScannerMutation.isPending &&
		!status_installing &&
		!install_job_in_progress;

	// Prefer install-job events when in progress (worker merges from Redis); fallback to status.install_events
	const install_events =
		(show_install_progress &&
			((installJobData?.install_events?.length > 0 &&
				installJobData.install_events) ||
				status?.install_events)) ||
		[];
	// Build checklist: for each predefined step, find latest event by step id and derive display state
	const install_checklist_steps = INSTALL_CHECKLIST_STEPS.map(
		({ id, label }) => {
			const evt = install_events.filter((e) => e.step === id).pop();
			const step_status = evt?.status || "pending";
			return {
				id,
				label,
				status: step_status,
				message:
					evt?.message ?? (step_status === "pending" ? "Waiting…" : null),
			};
		},
	);

	return (
		<div className="space-y-4">
			{/* Scanner status bar - above Security Compliance (always show on host compliance tab) */}
			<div className="rounded-lg border border-secondary-700 bg-secondary-800/80 p-4">
				<div className="flex flex-wrap items-center justify-between gap-3">
					<div className="flex flex-wrap items-center gap-4 text-sm">
						<div className="flex items-center gap-2">
							<Shield className="h-4 w-4 text-primary-400" />
							<span className="text-secondary-300">Scanner</span>
							<span
								className={`capitalize ${
									status_ready || openscap_ready
										? "text-green-400"
										: show_install_progress
											? "text-blue-400"
											: status?.status === "error"
												? "text-red-400"
												: "text-secondary-400"
								}`}
							>
								{show_install_progress
									? "Installing…"
									: status_ready || openscap_ready
										? "Ready"
										: (status?.status ?? "Not installed")}
							</span>
						</div>
						{scanner_info?.openscap_version && (
							<span className="text-secondary-500">
								OpenSCAP {scanner_info.openscap_version}
							</span>
						)}
						{scanner_info?.content_package && (
							<span className="text-secondary-500">
								SSG {scanner_info.content_package}
							</span>
						)}
					</div>
					{show_install_button && (
						<button
							type="button"
							onClick={() => installScannerMutation.mutate()}
							disabled={!isConnected || installScannerMutation.isPending}
							className="inline-flex items-center gap-2 rounded-lg bg-primary-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-primary-500 disabled:opacity-50 disabled:cursor-not-allowed"
						>
							<Package className="h-4 w-4" />
							Install scanner
						</button>
					)}
					{show_install_progress && install_events.length === 0 && (
						<span className="text-sm text-blue-400">
							Starting installation…
						</span>
					)}
				</div>
				{/* Installation progress checklist — visible for full install (status or job in progress) */}
				{show_install_progress && (
					<div className="mt-4 pt-4 border-t border-secondary-700">
						<p className="text-sm font-medium text-secondary-200 mb-3">
							Installation progress
						</p>
						<ul className="space-y-2">
							{install_checklist_steps.map((step) => (
								<li key={step.id} className="flex items-center gap-3 text-sm">
									{step.status === "done" && (
										<CheckCircle2 className="h-4 w-4 text-green-400 flex-shrink-0" />
									)}
									{step.status === "in_progress" && (
										<Loader2 className="h-4 w-4 text-blue-400 animate-spin flex-shrink-0" />
									)}
									{step.status === "failed" && (
										<XCircle className="h-4 w-4 text-red-400 flex-shrink-0" />
									)}
									{step.status === "skipped" && (
										<SkipForward className="h-4 w-4 text-secondary-400 flex-shrink-0" />
									)}
									{step.status === "pending" && (
										<div className="h-4 w-4 rounded-full border-2 border-secondary-500 flex-shrink-0" />
									)}
									<div className="flex-1 min-w-0">
										<span
											className={
												step.status === "done"
													? "text-green-400"
													: step.status === "in_progress"
														? "text-blue-400"
														: step.status === "failed"
															? "text-red-400"
															: step.status === "skipped"
																? "text-secondary-400"
																: "text-secondary-500"
											}
										>
											{step.label}
										</span>
										{step.message && step.status !== "pending" && (
											<span className="text-secondary-500 ml-1.5 text-xs">
												— {step.message}
											</span>
										)}
										{step.status === "pending" && (
											<span className="text-secondary-500 ml-1.5 text-xs">
												{step.message}
											</span>
										)}
									</div>
								</li>
							))}
						</ul>
					</div>
				)}
			</div>

			{/* Header */}
			<div className="flex items-center gap-3">
				<Shield className="h-6 w-6 text-primary-400" />
				<h2 className="text-xl font-semibold text-white">
					Security Compliance
				</h2>
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
