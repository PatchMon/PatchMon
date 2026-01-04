import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
	Activity,
	AlertCircle,
	AlertTriangle,
	ArrowLeft,
	Calendar,
	CheckCircle,
	CheckCircle2,
	Clock,
	Clock3,
	Container,
	Copy,
	Cpu,
	Database,
	Download,
	Eye,
	EyeOff,
	HardDrive,
	Key,
	MemoryStick,
	Monitor,
	Package,
	RefreshCw,
	RotateCcw,
	Server,
	Shield,
	Terminal,
	Trash2,
	Wifi,
	X,
} from "lucide-react";
import { useEffect, useId, useRef, useState } from "react";
import { Link, useLocation, useNavigate, useParams } from "react-router-dom";
import ComplianceTab from "../components/compliance/ComplianceTab";
import InlineEdit from "../components/InlineEdit";
import InlineMultiGroupEdit from "../components/InlineMultiGroupEdit";
import SshTerminal from "../components/SshTerminal";
import {
	adminHostsAPI,
	dashboardAPI,
	formatDate,
	formatRelativeTime,
	hostGroupsAPI,
	repositoryAPI,
	settingsAPI,
} from "../utils/api";
import { complianceAPI } from "../utils/complianceApi";
import { OSIcon } from "../utils/osIcons.jsx";
import CredentialsModal from "./hostdetail/CredentialsModal";
import DeleteConfirmationModal from "./hostdetail/DeleteConfirmationModal";
import AgentQueueTab from "./hostdetail/AgentQueueTab";

const HostDetail = () => {
	const { hostId } = useParams();
	const navigate = useNavigate();
	const location = useLocation();
	const queryClient = useQueryClient();
	const [showCredentialsModal, setShowCredentialsModal] = useState(false);

	// Get plaintext API key from navigation state (only available immediately after host creation)
	const plaintextApiKey = location.state?.apiKey;
	const [showDeleteModal, setShowDeleteModal] = useState(false);
	const [activeTab, setActiveTab] = useState("host");
	const [dockerSubTab, setDockerSubTab] = useState("containers");
	const [historyPage, setHistoryPage] = useState(0);
	const [historyLimit] = useState(10);
	const [notes, setNotes] = useState("");
	const [notesMessage, setNotesMessage] = useState({ text: "", type: "" });
	const [updateMessage, setUpdateMessage] = useState({ text: "", jobId: "" });
	const [reportMessage, setReportMessage] = useState({ text: "", jobId: "" });
	const [showAllReports, setShowAllReports] = useState(false);

	// Ref to track component mount state for setTimeout cleanup
	const isMountedRef = useRef(true);
	const timeoutRefs = useRef([]);

	// Cleanup timeouts on unmount
	useEffect(() => {
		isMountedRef.current = true;
		return () => {
			isMountedRef.current = false;
			// Clear all pending timeouts
			timeoutRefs.current.forEach((timeoutId) => clearTimeout(timeoutId));
			timeoutRefs.current = [];
		};
	}, []);

	// Helper function to safely set timeout with cleanup tracking
	const safeSetTimeout = (callback, delay) => {
		const timeoutId = setTimeout(() => {
			if (isMountedRef.current) {
				callback();
			}
			// Remove from tracking array
			timeoutRefs.current = timeoutRefs.current.filter((id) => id !== timeoutId);
		}, delay);
		timeoutRefs.current.push(timeoutId);
		return timeoutId;
	};

	const {
		data: host,
		isLoading,
		error,
		refetch,
		isFetching,
	} = useQuery({
		queryKey: ["host", hostId, historyPage, historyLimit],
		queryFn: () =>
			dashboardAPI
				.getHostDetail(hostId, {
					limit: historyLimit,
					offset: historyPage * historyLimit,
				})
				.then((res) => res.data),
		staleTime: 5 * 60 * 1000, // 5 minutes - data stays fresh longer
		refetchOnWindowFocus: false, // Don't refetch when window regains focus
	});

	// WebSocket connection status using polling (secure - uses httpOnly cookies)
	const [wsStatus, setWsStatus] = useState(null);

	useEffect(() => {
		if (!host?.api_id) return;

		let isMounted = true;

		// Fetch initial status
		const fetchStatus = async () => {
			try {
				const response = await fetch(`/api/v1/ws/status/${host.api_id}`, {
					credentials: "include",
				});
				if (response.ok && isMounted) {
					const result = await response.json();
					setWsStatus(result.data);
				}
			} catch (_err) {
				// Silently handle errors
			}
		};

		fetchStatus();

		// Poll every 5 seconds for status updates
		const pollInterval = setInterval(fetchStatus, 5000);

		// Cleanup on unmount or when api_id changes
		return () => {
			isMounted = false;
			clearInterval(pollInterval);
		};
	}, [host?.api_id]);

	// Fetch repository count for this host
	const { data: repositories, isLoading: isLoadingRepos } = useQuery({
		queryKey: ["host-repositories", hostId],
		queryFn: () => repositoryAPI.getByHost(hostId).then((res) => res.data),
		staleTime: 5 * 60 * 1000, // 5 minutes - data stays fresh longer
		refetchOnWindowFocus: false, // Don't refetch when window regains focus
		enabled: !!hostId,
	});

	// Fetch latest compliance scan for quick view (only if compliance might be enabled)
	const { data: complianceLatest, isLoading: isLoadingCompliance } = useQuery({
		queryKey: ["compliance-latest-quickview", hostId],
		queryFn: () => complianceAPI.getLatestScan(hostId).then((res) => res.data).catch(() => null),
		staleTime: 2 * 60 * 1000, // 2 minutes
		refetchOnWindowFocus: false,
		enabled: !!hostId,
		retry: false, // Don't retry if compliance not enabled
	});

	// Fetch host groups for multi-select
	const { data: hostGroups } = useQuery({
		queryKey: ["host-groups"],
		queryFn: () => hostGroupsAPI.list().then((res) => res.data),
		staleTime: 5 * 60 * 1000, // 5 minutes - data stays fresh longer
		refetchOnWindowFocus: false, // Don't refetch when window regains focus
	});

	// Tab change handler
	const handleTabChange = (tabName) => {
		setActiveTab(tabName);
	};

	// Auto-show credentials modal for new/pending hosts
	useEffect(() => {
		if (host && host.status === "pending") {
			setShowCredentialsModal(true);
		}
	}, [host]);

	// Sync notes state with host data
	useEffect(() => {
		if (host) {
			setNotes(host.notes || "");
		}
	}, [host]);

	const deleteHostMutation = useMutation({
		mutationFn: (hostId) => adminHostsAPI.delete(hostId),
		onSuccess: () => {
			queryClient.invalidateQueries(["hosts"]);
			navigate("/hosts");
		},
	});

	// Toggle agent auto-update mutation (updates PatchMonEnhanced agent script, not system packages)
	const toggleAutoUpdateMutation = useMutation({
		mutationFn: (auto_update) =>
			adminHostsAPI
				.toggleAutoUpdate(hostId, auto_update)
				.then((res) => res.data),
		onSuccess: () => {
			queryClient.invalidateQueries(["host", hostId]);
			queryClient.invalidateQueries(["hosts"]);
		},
	});

	// Force agent update mutation
	const forceAgentUpdateMutation = useMutation({
		mutationFn: () =>
			adminHostsAPI.forceAgentUpdate(hostId).then((res) => res.data),
		onSuccess: (data) => {
			queryClient.invalidateQueries(["host", hostId]);
			queryClient.invalidateQueries(["hosts"]);
			// Show success message with job ID
			if (data?.jobId) {
				setUpdateMessage({
					text: "Update queued successfully",
					jobId: data.jobId,
				});
				// Clear message after 5 seconds
				safeSetTimeout(() => setUpdateMessage({ text: "", jobId: "" }), 5000);
			}
		},
		onError: (error) => {
			setUpdateMessage({
				text: error.response?.data?.error || "Failed to queue update",
				jobId: "",
			});
			safeSetTimeout(() => setUpdateMessage({ text: "", jobId: "" }), 5000);
		},
	});

	// Fetch report mutation
	const fetchReportMutation = useMutation({
		mutationFn: () => adminHostsAPI.fetchReport(hostId).then((res) => res.data),
		onSuccess: (data) => {
			queryClient.invalidateQueries(["host", hostId]);
			queryClient.invalidateQueries(["hosts"]);
			// Show success message with job ID
			if (data?.jobId) {
				setReportMessage({
					text: "Report fetch queued successfully",
					jobId: data.jobId,
				});
				// Clear message after 5 seconds
				safeSetTimeout(() => setReportMessage({ text: "", jobId: "" }), 5000);
			}
		},
		onError: (error) => {
			setReportMessage({
				text: error.response?.data?.error || "Failed to fetch report",
				jobId: "",
			});
			safeSetTimeout(() => setReportMessage({ text: "", jobId: "" }), 5000);
		},
	});

	const updateFriendlyNameMutation = useMutation({
		mutationFn: (friendlyName) =>
			adminHostsAPI
				.updateFriendlyName(hostId, friendlyName)
				.then((res) => res.data),
		onSuccess: () => {
			queryClient.invalidateQueries(["host", hostId]);
		},
	});

	const updateConnectionMutation = useMutation({
		mutationFn: (connectionInfo) =>
			adminHostsAPI
				.updateConnection(hostId, connectionInfo)
				.then((res) => res.data),
		onSuccess: () => {
			queryClient.invalidateQueries(["host", hostId]);
			queryClient.invalidateQueries(["hosts"]);
		},
	});

	const updateHostGroupsMutation = useMutation({
		mutationFn: ({ hostId, groupIds }) =>
			adminHostsAPI.updateGroups(hostId, groupIds).then((res) => res.data),
		onSuccess: () => {
			queryClient.invalidateQueries(["host", hostId]);
			queryClient.invalidateQueries(["hosts"]);
		},
	});

	const updateNotesMutation = useMutation({
		mutationFn: ({ hostId, notes }) =>
			adminHostsAPI.updateNotes(hostId, notes).then((res) => res.data),
		onSuccess: () => {
			queryClient.invalidateQueries(["host", hostId]);
			queryClient.invalidateQueries(["hosts"]);
			setNotesMessage({ text: "Notes saved successfully!", type: "success" });
			// Clear message after 3 seconds
			safeSetTimeout(() => setNotesMessage({ text: "", type: "" }), 3000);
		},
		onError: (error) => {
			setNotesMessage({
				text: error.response?.data?.error || "Failed to save notes",
				type: "error",
			});
			// Clear message after 5 seconds for errors
			safeSetTimeout(() => setNotesMessage({ text: "", type: "" }), 5000);
		},
	});

	// Fetch integration status
	const {
		data: integrationsData,
		isLoading: isLoadingIntegrations,
		refetch: refetchIntegrations,
	} = useQuery({
		queryKey: ["host-integrations", hostId],
		queryFn: () =>
			adminHostsAPI.getIntegrations(hostId).then((res) => res.data),
		staleTime: 30 * 1000, // 30 seconds
		refetchOnWindowFocus: false,
		enabled: !!hostId, // Always fetch to control tab visibility
	});

	// Poll for compliance setup status when compliance is enabled
	const {
		data: complianceSetupStatus,
		refetch: refetchComplianceStatus,
	} = useQuery({
		queryKey: ["compliance-setup-status", hostId],
		queryFn: () =>
			adminHostsAPI.getIntegrationSetupStatus(hostId, "compliance").then((res) => res.data),
		staleTime: 5 * 1000, // 5 seconds
		refetchInterval: (query) => {
			// Poll every 2 seconds while status is "installing" or "removing"
			const status = query.state?.data?.status?.status;
			if (status === "installing" || status === "removing") {
				return 2000; // Poll faster during installation
			}
			return false; // Stop polling when done
		},
		refetchOnWindowFocus: false,
		// Always enable for hosts - we need to catch status updates
		enabled: !!hostId,
	});

	// Fetch Docker data for this host
	const {
		data: dockerData,
		isLoading: isLoadingDocker,
	} = useQuery({
		queryKey: ["docker", "host", hostId],
		queryFn: () =>
			dashboardAPI.getHostDetail(hostId, { include: "docker" }).then((res) => res.data?.docker),
		staleTime: 30 * 1000,
		refetchOnWindowFocus: false,
		enabled: !!hostId && (activeTab === "docker" || integrationsData?.data?.integrations?.docker),
	});

	// Refetch integrations when WebSocket status changes (e.g., after agent restart)
	useEffect(() => {
		if (
			wsStatus?.connected &&
			activeTab === "integrations" &&
			integrationsData?.data?.connected === false
		) {
			// Agent just reconnected, refetch integrations to get updated connection status
			refetchIntegrations();
		}
	}, [
		wsStatus?.connected,
		activeTab,
		integrationsData?.data?.connected,
		refetchIntegrations,
	]);

	// Toggle integration mutation
	const toggleIntegrationMutation = useMutation({
		mutationFn: ({ integrationName, enabled }) =>
			adminHostsAPI
				.toggleIntegration(hostId, integrationName, enabled)
				.then((res) => res.data),
		onSuccess: (data) => {
			// Optimistically update the cache with the new state
			queryClient.setQueryData(["host-integrations", hostId], (oldData) => {
				if (!oldData) return oldData;
				return {
					...oldData,
					data: {
						...oldData.data,
						integrations: {
							...oldData.data.integrations,
							[data.data.integration]: data.data.enabled,
						},
					},
				};
			});
			// Also invalidate to ensure we get fresh data
			queryClient.invalidateQueries(["host-integrations", hostId]);
			// If compliance was just enabled/disabled, poll for setup status
			if (data.data.integration === "compliance") {
				// Poll multiple times to catch status updates (installation takes ~4-10s)
				const pollTimes = [500, 2000, 4000, 6000, 8000, 10000, 15000];
				pollTimes.forEach(delay => {
					safeSetTimeout(() => refetchComplianceStatus(), delay);
				});
			}
		},
		onError: (error) => {
			// On error, refetch to get the actual state
			refetchIntegrations();
			// Log error for debugging
			console.error("Failed to toggle integration:", error.response?.data?.error || error.message);
		},
	});

	const handleDeleteHost = async () => {
		if (
			window.confirm(
				`Are you sure you want to delete host "${host.friendly_name}"? This action cannot be undone.`,
			)
		) {
			try {
				await deleteHostMutation.mutateAsync(hostId);
			} catch (error) {
				console.error("Failed to delete host:", error);
				alert("Failed to delete host");
			}
		}
	};

	if (isLoading) {
		return (
			<div className="flex items-center justify-center h-64">
				<RefreshCw className="h-8 w-8 animate-spin text-primary-600" />
			</div>
		);
	}

	if (error) {
		return (
			<div className="space-y-6">
				<div className="flex items-center justify-between">
					<div className="flex items-center gap-3">
						<Link
							to="/hosts"
							className="text-secondary-500 hover:text-secondary-700"
						>
							<ArrowLeft className="h-5 w-5" />
						</Link>
					</div>
				</div>

				<div className="bg-danger-50 border border-danger-200 rounded-md p-4">
					<div className="flex">
						<AlertTriangle className="h-5 w-5 text-danger-400" />
						<div className="ml-3">
							<h3 className="text-sm font-medium text-danger-800">
								Error loading host
							</h3>
							<p className="text-sm text-danger-700 mt-1">
								{error.message || "Failed to load host details"}
							</p>
							<button
								type="button"
								onClick={() => refetch()}
								className="mt-2 btn-danger text-xs"
							>
								Try again
							</button>
						</div>
					</div>
				</div>
			</div>
		);
	}

	if (!host) {
		return (
			<div className="space-y-6">
				<div className="flex items-center justify-between">
					<div className="flex items-center gap-3">
						<Link
							to="/hosts"
							className="text-secondary-500 hover:text-secondary-700"
						>
							<ArrowLeft className="h-5 w-5" />
						</Link>
					</div>
				</div>

				<div className="card p-8 text-center">
					<Server className="h-12 w-12 text-secondary-400 mx-auto mb-4" />
					<h3 className="text-lg font-medium text-secondary-900 dark:text-white mb-2">
						Host Not Found
					</h3>
					<p className="text-secondary-600 dark:text-secondary-300">
						The requested host could not be found.
					</p>
				</div>
			</div>
		);
	}

	const getStatusColor = (isStale, needsUpdate) => {
		if (isStale) return "text-danger-600";
		if (needsUpdate) return "text-warning-600";
		return "text-success-600";
	};

	const getStatusIcon = (isStale, needsUpdate) => {
		if (isStale) return <AlertTriangle className="h-5 w-5" />;
		if (needsUpdate) return <Clock className="h-5 w-5" />;
		return <CheckCircle className="h-5 w-5" />;
	};

	const getStatusText = (isStale, needsUpdate) => {
		if (isStale) return "Stale";
		if (needsUpdate) return "Needs Updates";
		return "Up to Date";
	};

	const isStale = Date.now() - new Date(host.last_update) > 24 * 60 * 60 * 1000;

	return (
		<div className="min-h-screen flex flex-col">
			{/* Header */}
			<div className="flex flex-col md:flex-row md:items-start md:justify-between gap-4 mb-4 pb-4 border-b border-secondary-200 dark:border-secondary-600">
				<div className="flex items-start gap-3">
					<Link
						to="/hosts"
						className="text-secondary-500 hover:text-secondary-700 dark:text-secondary-400 dark:hover:text-secondary-200 mt-1"
					>
						<ArrowLeft className="h-5 w-5" />
					</Link>
					<div className="flex flex-col gap-2">
						{/* Title row with friendly name, badge, and status */}
						<div className="flex items-center gap-3 flex-wrap">
							<h1 className="text-2xl font-semibold text-secondary-900 dark:text-white">
								{host.friendly_name}
							</h1>
							{wsStatus && (
								<span
									className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-semibold uppercase ${
										wsStatus.connected
											? "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200 animate-pulse"
											: "bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200"
									}`}
									title={
										wsStatus.connected
											? `Agent connected via ${wsStatus.secure ? "WSS (secure)" : "WS"}`
											: "Agent not connected"
									}
								>
									{wsStatus.connected
										? wsStatus.secure
											? "WSS"
											: "WS"
										: "Offline"}
								</span>
							)}
							<div
								className={`flex items-center gap-2 px-2 py-1 rounded-full text-xs font-medium ${getStatusColor(isStale, host.stats.outdated_packages > 0)}`}
							>
								{getStatusIcon(isStale, host.stats.outdated_packages > 0)}
								{getStatusText(isStale, host.stats.outdated_packages > 0)}
							</div>
							{host.needs_reboot && (
								<span
									className="inline-flex items-center gap-1 px-2 py-0.5 rounded-md text-xs font-medium bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200"
									title={host.reboot_reason || "Reboot required"}
								>
									<RotateCcw className="h-3 w-3" />
									Reboot Required
								</span>
							)}
							{/* Integration Badges */}
							{integrationsData?.data?.integrations?.compliance && (
								<span
									className="inline-flex items-center gap-1 px-2 py-0.5 rounded-md text-xs font-medium bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200"
									title="Compliance scanning enabled"
								>
									<Shield className="h-3 w-3" />
									Compliance
								</span>
							)}
							{integrationsData?.data?.integrations?.docker && (
								<span
									className="inline-flex items-center gap-1 px-2 py-0.5 rounded-md text-xs font-medium bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200"
									title="Docker monitoring enabled"
								>
									<Container className="h-3 w-3" />
									Docker
								</span>
							)}
						</div>
						{/* Info row with uptime and last updated */}
						<div className="flex items-center gap-4 text-sm text-secondary-600 dark:text-white">
							{host.system_uptime && (
								<div className="flex items-center gap-1">
									<Clock className="h-3.5 w-3.5" />
									<span className="text-xs font-medium">Uptime:</span>
									<span className="text-xs">{host.system_uptime}</span>
								</div>
							)}
							<div className="flex items-center gap-1">
								<Clock className="h-3.5 w-3.5" />
								<span className="text-xs font-medium">Last updated:</span>
								<span className="text-xs">
									{formatRelativeTime(host.last_update)}
								</span>
							</div>
						</div>
					</div>
				</div>
				<div className="flex items-center gap-2 flex-wrap w-full md:w-auto">
					<div className="flex-1 min-w-0">
						<button
							type="button"
							onClick={() => fetchReportMutation.mutate()}
							disabled={fetchReportMutation.isPending || !wsStatus?.connected}
							className="btn-outline flex items-center gap-2 text-sm whitespace-nowrap w-full"
							title={
								!wsStatus?.connected
									? "Agent is not connected"
									: "Fetch package data from agent"
							}
						>
							<Download
								className={`h-4 w-4 ${
									fetchReportMutation.isPending ? "animate-spin" : ""
								}`}
							/>
							<span className="hidden sm:inline">Fetch Report</span>
							<span className="sm:hidden">Fetch</span>
						</button>
						{reportMessage.text && (
							<p className="text-xs mt-1.5 text-secondary-600 dark:text-secondary-400">
								{reportMessage.text}
								{reportMessage.jobId && (
									<span className="ml-1 font-mono text-secondary-500">
										(Job #{reportMessage.jobId})
									</span>
								)}
							</p>
						)}
					</div>
					<div className="flex items-center gap-2 flex-shrink-0">
						<button
							type="button"
							onClick={() => setShowCredentialsModal(true)}
							className={`btn-outline flex items-center text-sm whitespace-nowrap ${
								host?.machine_id ? "justify-center p-2" : "gap-2"
							}`}
							title="View credentials"
						>
							<Key className="h-4 w-4" />
							{!host?.machine_id && (
								<span className="hidden sm:inline">Deploy Agent</span>
							)}
						</button>
						<button
							type="button"
							onClick={() => refetch()}
							disabled={isFetching}
							className="btn-outline flex items-center justify-center p-2 text-sm"
							title="Refresh dashboard"
						>
							<RefreshCw
								className={`h-4 w-4 ${isFetching ? "animate-spin" : ""}`}
							/>
						</button>
						<button
							type="button"
							onClick={() => setShowDeleteModal(true)}
							className="btn-danger flex items-center justify-center p-2 text-sm"
							title="Delete host"
						>
							<Trash2 className="h-4 w-4" />
						</button>
					</div>
				</div>
			</div>

			{/* Package Statistics Cards */}
			<div className="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
				<button
					type="button"
					onClick={() => navigate(`/packages?host=${hostId}`)}
					className="card p-4 cursor-pointer hover:shadow-card-hover dark:hover:shadow-card-hover-dark transition-shadow duration-200 text-left w-full"
					title="View all packages for this host"
				>
					<div className="flex items-center">
						<Package className="h-5 w-5 text-primary-600 mr-2" />
						<div>
							<p className="text-sm text-secondary-500 dark:text-white">
								Total Installed
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{host.stats.total_packages}
							</p>
						</div>
					</div>
				</button>

				<button
					type="button"
					onClick={() => navigate(`/packages?host=${hostId}&filter=outdated`)}
					className="card p-4 cursor-pointer hover:shadow-card-hover dark:hover:shadow-card-hover-dark transition-shadow duration-200 text-left w-full"
					title="View outdated packages for this host"
				>
					<div className="flex items-center">
						<Clock className="h-5 w-5 text-warning-600 mr-2" />
						<div>
							<p className="text-sm text-secondary-500 dark:text-white">
								Outdated Packages
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{host.stats.outdated_packages}
							</p>
						</div>
					</div>
				</button>

				<button
					type="button"
					onClick={() => navigate(`/packages?host=${hostId}&filter=security`)}
					className="card p-4 cursor-pointer hover:shadow-card-hover dark:hover:shadow-card-hover-dark transition-shadow duration-200 text-left w-full"
					title="View security packages for this host"
				>
					<div className="flex items-center">
						<Shield className="h-5 w-5 text-danger-600 mr-2" />
						<div>
							<p className="text-sm text-secondary-500 dark:text-white">
								Security Updates
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{host.stats.security_updates}
							</p>
						</div>
					</div>
				</button>

				<button
					type="button"
					onClick={() => navigate(`/repositories?host=${hostId}`)}
					className="card p-4 cursor-pointer hover:shadow-card-hover dark:hover:shadow-card-hover-dark transition-shadow duration-200 text-left w-full"
					title="View repositories for this host"
				>
					<div className="flex items-center">
						<Database className="h-5 w-5 text-blue-600 mr-2" />
						<div>
							<p className="text-sm text-secondary-500 dark:text-white">
								Repos
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{isLoadingRepos ? "..." : repositories?.length || 0}
							</p>
						</div>
					</div>
				</button>
			</div>

			{/* Compliance Quick View - Only shows when compliance is enabled and has scan data */}
			{complianceLatest && complianceLatest.score !== undefined && (
				<div className="mb-6">
					<button
						type="button"
						onClick={() => handleTabChange("compliance")}
						className="card p-4 w-full cursor-pointer hover:shadow-card-hover dark:hover:shadow-card-hover-dark transition-shadow duration-200 text-left"
						title="View compliance details"
					>
						<div className="flex items-center justify-between">
							<div className="flex items-center gap-4">
								<div className="flex-shrink-0">
									<div
										className={`w-12 h-12 rounded-full flex items-center justify-center ${
											complianceLatest.score >= 80
												? "bg-green-100 dark:bg-green-900/30"
												: complianceLatest.score >= 60
													? "bg-yellow-100 dark:bg-yellow-900/30"
													: "bg-red-100 dark:bg-red-900/30"
										}`}
									>
										<span
											className={`text-lg font-bold ${
												complianceLatest.score >= 80
													? "text-green-600 dark:text-green-400"
													: complianceLatest.score >= 60
														? "text-yellow-600 dark:text-yellow-400"
														: "text-red-600 dark:text-red-400"
											}`}
										>
											{Math.round(complianceLatest.score)}%
										</span>
									</div>
								</div>
								<div>
									<div className="flex items-center gap-2">
										<Shield className="h-4 w-4 text-primary-600 dark:text-primary-400" />
										<p className="text-sm font-medium text-secondary-900 dark:text-white">
											Compliance Score
										</p>
									</div>
									<p className="text-xs text-secondary-500 dark:text-secondary-400 mt-0.5">
										{complianceLatest.compliance_profiles?.name || "Security Profile"}
										{complianceLatest.completed_at && (
											<span className="ml-2">
												• {formatRelativeTime(complianceLatest.completed_at)}
											</span>
										)}
									</p>
								</div>
							</div>
							<div className="flex items-center gap-4 text-sm">
								<div className="flex items-center gap-1.5">
									<CheckCircle className="h-4 w-4 text-green-500" />
									<span className="text-secondary-700 dark:text-secondary-300">{complianceLatest.passed || 0}</span>
								</div>
								<div className="flex items-center gap-1.5">
									<X className="h-4 w-4 text-red-500" />
									<span className="text-secondary-700 dark:text-secondary-300">{complianceLatest.failed || 0}</span>
								</div>
								<div className="flex items-center gap-1.5">
									<AlertTriangle className="h-4 w-4 text-yellow-500" />
									<span className="text-secondary-700 dark:text-secondary-300">{complianceLatest.warnings || 0}</span>
								</div>
							</div>
						</div>
					</button>
				</div>
			)}

			{/* Main Content - Full Width */}
			<div className="flex-1 md:overflow-hidden">
				{/* Mobile View - All sections as cards stacked vertically */}
				<div className="md:hidden space-y-4 pb-4">
					{/* Host Info Card */}
					<div className="card p-4">
						<h3 className="text-lg font-semibold text-secondary-900 dark:text-white mb-4 flex items-center gap-2">
							<Server className="h-5 w-5 text-primary-600" />
							Host Information
						</h3>
						<div className="space-y-4">
							<div className="space-y-3">
								<div>
									<p className="text-xs text-secondary-500 dark:text-secondary-300 mb-1.5">
										Friendly Name
									</p>
									<InlineEdit
										value={host.friendly_name}
										onSave={(newName) =>
											updateFriendlyNameMutation.mutate(newName)
										}
										placeholder="Enter friendly name..."
										maxLength={100}
										validate={(value) => {
											if (!value.trim()) return "Friendly name is required";
											if (value.trim().length < 1)
												return "Friendly name must be at least 1 character";
											if (value.trim().length > 100)
												return "Friendly name must be less than 100 characters";
											return null;
										}}
										className="w-full text-sm"
									/>
								</div>

								{host.hostname && (
									<div>
										<p className="text-xs text-secondary-500 dark:text-secondary-300 mb-1.5">
											System Hostname
										</p>
										<p className="font-medium text-secondary-900 dark:text-white font-mono text-sm">
											{host.hostname}
										</p>
									</div>
								)}

								{host.machine_id && (
									<div>
										<p className="text-xs text-secondary-500 dark:text-secondary-300 mb-1.5">
											Machine ID
										</p>
										<p className="font-medium text-secondary-900 dark:text-white font-mono text-sm break-all">
											{host.machine_id}
										</p>
									</div>
								)}

								<div>
									<p className="text-xs text-secondary-500 dark:text-secondary-300 mb-1.5">
										Host Groups
									</p>
									{(() => {
										const groupIds =
											host.host_group_memberships?.map(
												(membership) => membership.host_groups.id,
											) || [];
										return (
											<InlineMultiGroupEdit
												key={`${host.id}-${groupIds.join(",")}`}
												value={groupIds}
												onSave={(newGroupIds) =>
													updateHostGroupsMutation.mutate({
														hostId: host.id,
														groupIds: newGroupIds,
													})
												}
												options={hostGroups || []}
												placeholder="Select groups..."
												className="w-full"
											/>
										);
									})()}
								</div>

								<div>
									<p className="text-xs text-secondary-500 dark:text-secondary-300 mb-1.5">
										Operating System
									</p>
									<div className="flex items-center gap-2">
										<OSIcon osType={host.os_type} className="h-4 w-4" />
										<p className="font-medium text-secondary-900 dark:text-white text-sm">
											{host.os_type} {host.os_version}
										</p>
									</div>
								</div>

								<div>
									<p className="text-xs text-secondary-500 dark:text-secondary-300 mb-1.5">
										Agent Version
									</p>
									<p className="font-medium text-secondary-900 dark:text-white text-sm">
										{host.agent_version || "Unknown"}
									</p>
								</div>

								<div>
									<p className="text-xs text-secondary-500 dark:text-secondary-300 mb-1.5">
										Agent Auto-update
									</p>
									<button
										type="button"
										onClick={() =>
											toggleAutoUpdateMutation.mutate(!host.auto_update)
										}
										disabled={toggleAutoUpdateMutation.isPending}
										className={`relative inline-flex h-5 w-9 items-center rounded-md transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 ${
											host.auto_update
												? "bg-primary-600 dark:bg-primary-500"
												: "bg-secondary-200 dark:bg-secondary-600"
										}`}
									>
										<span
											className={`inline-block h-3 w-3 transform rounded-full bg-white transition-transform ${
												host.auto_update ? "translate-x-5" : "translate-x-1"
											}`}
										/>
									</button>
								</div>

								<div>
									<p className="text-xs text-secondary-500 dark:text-secondary-300 mb-1.5">
										Force Agent Version Upgrade
									</p>
									<button
										type="button"
										onClick={() => forceAgentUpdateMutation.mutate()}
										disabled={
											forceAgentUpdateMutation.isPending || !wsStatus?.connected
										}
										title={
											!wsStatus?.connected
												? "Agent is not connected"
												: "Force agent to update now"
										}
										className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-primary-600 dark:text-primary-400 bg-primary-50 dark:bg-primary-900/20 border border-primary-200 dark:border-primary-800 rounded-md hover:bg-primary-100 dark:hover:bg-primary-900/40 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
									>
										<RefreshCw
											className={`h-3 w-3 ${
												forceAgentUpdateMutation.isPending ? "animate-spin" : ""
											}`}
										/>
										{forceAgentUpdateMutation.isPending
											? "Updating..."
											: wsStatus?.connected
												? "Update Now"
												: "Offline"}
									</button>
									{updateMessage.text && (
										<p className="text-xs mt-1.5 text-secondary-600 dark:text-secondary-400">
											{updateMessage.text}
											{updateMessage.jobId && (
												<span className="ml-1 font-mono text-secondary-500">
													(Job #{updateMessage.jobId})
												</span>
											)}
										</p>
									)}
								</div>
							</div>
						</div>
					</div>

					{/* Network Card */}
					{(host.dns_servers || host.network_interfaces) && (
						<div className="card p-4">
							<h3 className="text-lg font-semibold text-secondary-900 dark:text-white mb-4 flex items-center gap-2">
								<Wifi className="h-5 w-5 text-primary-600" />
								Network
							</h3>
							<div className="space-y-4">
								{host.dns_servers &&
									Array.isArray(host.dns_servers) &&
									host.dns_servers.length > 0 && (
										<div>
											<p className="text-xs text-secondary-500 dark:text-secondary-300 mb-2">
												DNS Servers
											</p>
											<div className="space-y-1">
												{host.dns_servers.map((dns) => (
													<div
														key={dns}
														className="bg-secondary-50 dark:bg-secondary-700 p-2 rounded border border-secondary-200 dark:border-secondary-600"
													>
														<p className="font-medium text-secondary-900 dark:text-white font-mono text-sm">
															{dns}
														</p>
													</div>
												))}
											</div>
										</div>
									)}

								{host.network_interfaces &&
									Array.isArray(host.network_interfaces) &&
									host.network_interfaces.length > 0 && (
										<div>
											<p className="text-xs text-secondary-500 dark:text-secondary-300 mb-3">
												Network Interfaces
											</p>
											<div className="space-y-4">
												{host.network_interfaces.map((iface) => (
													<div
														key={iface.name}
														className="border border-secondary-200 dark:border-secondary-700 rounded-lg p-3 bg-secondary-50 dark:bg-secondary-900/50"
													>
														{/* Interface Header */}
														<div className="flex items-center justify-between mb-3">
															<div className="flex items-center gap-2">
																<p className="font-semibold text-secondary-900 dark:text-white text-sm">
																	{iface.name}
																</p>
																{iface.type && (
																	<span className="text-xs text-secondary-500 dark:text-secondary-400 bg-secondary-200 dark:bg-secondary-700 px-2 py-0.5 rounded">
																		{iface.type}
																	</span>
																)}
																{iface.status && (
																	<span
																		className={`text-xs font-medium px-2 py-0.5 rounded ${
																			iface.status === "up"
																				? "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200"
																				: "bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200"
																		}`}
																	>
																		{iface.status === "up" ? "UP" : "DOWN"}
																	</span>
																)}
															</div>
														</div>

														{/* Interface Details */}
														<div className="grid grid-cols-1 md:grid-cols-2 gap-3 text-xs mb-3">
															{iface.macAddress && (
																<div>
																	<p className="text-secondary-500 dark:text-secondary-400 mb-0.5">
																		MAC Address
																	</p>
																	<p className="font-mono text-secondary-900 dark:text-white">
																		{iface.macAddress}
																	</p>
																</div>
															)}
															{iface.mtu && (
																<div>
																	<p className="text-secondary-500 dark:text-secondary-400 mb-0.5">
																		MTU
																	</p>
																	<p className="text-secondary-900 dark:text-white">
																		{iface.mtu}
																	</p>
																</div>
															)}
															{iface.linkSpeed && iface.linkSpeed > 0 && (
																<div>
																	<p className="text-secondary-500 dark:text-secondary-400 mb-0.5">
																		Link Speed
																	</p>
																	<p className="text-secondary-900 dark:text-white">
																		{iface.linkSpeed} Mbps
																		{iface.duplex &&
																			` (${iface.duplex} duplex)`}
																	</p>
																</div>
															)}
														</div>

														{/* Addresses */}
														{iface.addresses &&
															Array.isArray(iface.addresses) &&
															iface.addresses.length > 0 && (
																<div className="space-y-2 pt-2 border-t border-secondary-200 dark:border-secondary-700">
																	<p className="text-xs font-medium text-secondary-500 dark:text-secondary-400 mb-2">
																		IP Addresses
																	</p>
																	<div className="space-y-2">
																		{iface.addresses.map((addr, idx) => (
																			<div
																				key={`${addr.address}-${addr.family}-${idx}`}
																				className="bg-white dark:bg-secondary-800 rounded p-2 border border-secondary-200 dark:border-secondary-700"
																			>
																				<div className="flex items-center gap-2 mb-1">
																					<span
																						className={`inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium ${
																							addr.family === "inet6"
																								? "bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200"
																								: "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200"
																						}`}
																					>
																						{addr.family === "inet6"
																							? "inet6"
																							: "inet"}
																					</span>
																					<span className="font-mono text-sm font-semibold text-secondary-900 dark:text-white">
																						{addr.address}
																						{addr.netmask && (
																							<span className="text-secondary-500 dark:text-secondary-400 ml-1">
																								{addr.netmask}
																							</span>
																						)}
																					</span>
																				</div>
																				{addr.gateway && (
																					<div className="text-xs text-secondary-600 dark:text-secondary-400 ml-1">
																						Gateway:{" "}
																						<span className="font-mono">
																							{addr.gateway}
																						</span>
																					</div>
																				)}
																			</div>
																		))}
																	</div>
																</div>
															)}
													</div>
												))}
											</div>
										</div>
									)}
							</div>
						</div>
					)}

					{/* System Card */}
					<div className="card p-4">
						<h3 className="text-lg font-semibold text-secondary-900 dark:text-white mb-4 flex items-center gap-2">
							<Terminal className="h-5 w-5 text-primary-600" />
							System
						</h3>
						<div className="space-y-4">
							{/* System Information */}
							{(host.kernel_version ||
								host.selinux_status ||
								host.architecture) && (
								<div>
									<h4 className="text-sm font-medium text-secondary-900 dark:text-white mb-3 flex items-center gap-2">
										<Terminal className="h-4 w-4 text-primary-600 dark:text-primary-400" />
										System Information
									</h4>
									<div className="space-y-3">
										{host.architecture && (
											<div>
												<p className="text-xs text-secondary-500 dark:text-secondary-300">
													Architecture
												</p>
												<p className="font-medium text-secondary-900 dark:text-white text-sm">
													{host.architecture}
												</p>
											</div>
										)}

										{host.kernel_version && (
											<div>
												<p className="text-xs text-secondary-500 dark:text-secondary-300">
													Running Kernel
												</p>
												<p className="font-medium text-secondary-900 dark:text-white font-mono text-sm break-all">
													{host.kernel_version}
												</p>
											</div>
										)}

										{host.installed_kernel_version && (
											<div>
												<p className="text-xs text-secondary-500 dark:text-secondary-300">
													Installed Kernel
												</p>
												<p className="font-medium text-secondary-900 dark:text-white font-mono text-sm break-all">
													{host.installed_kernel_version}
												</p>
											</div>
										)}

										{host.selinux_status && (
											<div>
												<p className="text-xs text-secondary-500 dark:text-secondary-300">
													SELinux Status
												</p>
												<span
													className={`inline-flex items-center px-2 py-1 rounded-full text-xs font-medium ${
														host.selinux_status === "enabled"
															? "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200"
															: host.selinux_status === "permissive"
																? "bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200"
																: "bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200"
													}`}
												>
													{host.selinux_status}
												</span>
											</div>
										)}
									</div>
								</div>
							)}

							{/* Resource Information */}
							{(host.system_uptime ||
								host.cpu_model ||
								host.cpu_cores ||
								host.ram_installed ||
								host.swap_size !== undefined ||
								(host.load_average &&
									Array.isArray(host.load_average) &&
									host.load_average.length > 0 &&
									host.load_average.some((load) => load != null)) ||
								(host.disk_details &&
									Array.isArray(host.disk_details) &&
									host.disk_details.length > 0)) && (
								<div className="pt-4 border-t border-secondary-200 dark:border-secondary-600">
									<h4 className="text-sm font-medium text-secondary-900 dark:text-white mb-3 flex items-center gap-2">
										<Monitor className="h-4 w-4 text-primary-600 dark:text-primary-400" />
										Resource Information
									</h4>
									<div className="space-y-3">
										{host.system_uptime && (
											<div>
												<p className="text-xs text-secondary-500 dark:text-secondary-300">
													System Uptime
												</p>
												<p className="font-medium text-secondary-900 dark:text-white text-sm">
													{host.system_uptime}
												</p>
											</div>
										)}

										{host.cpu_model && (
											<div>
												<p className="text-xs text-secondary-500 dark:text-secondary-300">
													CPU Model
												</p>
												<p className="font-medium text-secondary-900 dark:text-white text-sm">
													{host.cpu_model}
												</p>
											</div>
										)}

										{host.cpu_cores && (
											<div>
												<p className="text-xs text-secondary-500 dark:text-secondary-300">
													CPU Cores
												</p>
												<p className="font-medium text-secondary-900 dark:text-white text-sm">
													{host.cpu_cores}
												</p>
											</div>
										)}

										{host.ram_installed && (
											<div>
												<p className="text-xs text-secondary-500 dark:text-secondary-300">
													RAM Installed
												</p>
												<p className="font-medium text-secondary-900 dark:text-white text-sm">
													{host.ram_installed} GB
												</p>
											</div>
										)}

										{host.swap_size !== undefined &&
											host.swap_size !== null && (
												<div>
													<p className="text-xs text-secondary-500 dark:text-secondary-300">
														Swap Size
													</p>
													<p className="font-medium text-secondary-900 dark:text-white text-sm">
														{host.swap_size} GB
													</p>
												</div>
											)}

										{host.load_average &&
											Array.isArray(host.load_average) &&
											host.load_average.length > 0 &&
											host.load_average.some((load) => load != null) && (
												<div>
													<p className="text-xs text-secondary-500 dark:text-secondary-300">
														Load Average
													</p>
													<p className="font-medium text-secondary-900 dark:text-white text-sm">
														{host.load_average
															.filter((load) => load != null)
															.map((load, index) => (
																<span key={`load-${index}-${load}`}>
																	{typeof load === "number"
																		? load.toFixed(2)
																		: String(load)}
																	{index <
																		host.load_average.filter(
																			(load) => load != null,
																		).length -
																			1 && ", "}
																</span>
															))}
													</p>
												</div>
											)}

										{host.disk_details &&
											Array.isArray(host.disk_details) &&
											host.disk_details.length > 0 && (
												<div className="pt-3 border-t border-secondary-200 dark:border-secondary-600">
													<h5 className="text-sm font-medium text-secondary-900 dark:text-white mb-3 flex items-center gap-2">
														<HardDrive className="h-4 w-4 text-primary-600 dark:text-primary-400" />
														Disk Usage
													</h5>
													<div className="space-y-3">
														{host.disk_details.map((disk, index) => (
															<div
																key={disk.name || `disk-${index}`}
																className="bg-secondary-50 dark:bg-secondary-700 p-3 rounded-lg"
															>
																<div className="flex items-center gap-2 mb-2">
																	<HardDrive className="h-4 w-4 text-secondary-500" />
																	<span className="font-medium text-secondary-900 dark:text-white text-sm">
																		{disk.name || `Disk ${index + 1}`}
																	</span>
																</div>
																{disk.size && (
																	<p className="text-xs text-secondary-600 dark:text-secondary-300 mb-1">
																		Size: {disk.size}
																	</p>
																)}
																{disk.mountpoint && (
																	<p className="text-xs text-secondary-600 dark:text-secondary-300 mb-1">
																		Mount: {disk.mountpoint}
																	</p>
																)}
																{disk.usage &&
																	typeof disk.usage === "number" && (
																		<div className="mt-2">
																			<div className="flex justify-between text-xs text-secondary-600 dark:text-secondary-300 mb-1">
																				<span>Usage</span>
																				<span>{disk.usage}%</span>
																			</div>
																			<div className="w-full bg-secondary-200 dark:bg-secondary-600 rounded-full h-2">
																				<div
																					className="bg-primary-600 dark:bg-primary-400 h-2 rounded-full transition-all duration-300"
																					style={{
																						width: `${Math.min(Math.max(disk.usage, 0), 100)}%`,
																					}}
																				></div>
																			</div>
																		</div>
																	)}
															</div>
														))}
													</div>
												</div>
											)}
									</div>
								</div>
							)}

							{/* No Data State */}
							{!host.kernel_version &&
								!host.selinux_status &&
								!host.architecture &&
								!host.system_uptime &&
								!host.cpu_model &&
								!host.cpu_cores &&
								!host.ram_installed &&
								host.swap_size === undefined &&
								(!host.load_average ||
									!Array.isArray(host.load_average) ||
									host.load_average.length === 0 ||
									!host.load_average.some((load) => load != null)) &&
								(!host.disk_details ||
									!Array.isArray(host.disk_details) ||
									host.disk_details.length === 0) && (
									<div className="text-center py-8">
										<Terminal className="h-8 w-8 text-secondary-400 mx-auto mb-2" />
										<p className="text-sm text-secondary-500 dark:text-secondary-300">
											No system information available
										</p>
									</div>
								)}
						</div>
					</div>

					{/* Package Reports Card */}
					<div className="card p-4">
						<h3 className="text-lg font-semibold text-secondary-900 dark:text-white mb-4 flex items-center gap-2">
							<Calendar className="h-5 w-5 text-primary-600" />
							Package Reports
						</h3>
						<div className="space-y-4">
							{host.update_history?.length > 0 ? (
								<>
									<div className="space-y-3">
										{(showAllReports
											? host.update_history
											: host.update_history.slice(0, 1)
										).map((update) => (
											<div
												key={update.id}
												className="p-3 bg-secondary-50 dark:bg-secondary-700 rounded-lg space-y-2"
											>
												<div className="flex items-start justify-between gap-3">
													<div className="flex items-center gap-1.5">
														<div
															className={`w-1.5 h-1.5 rounded-full ${update.status === "success" ? "bg-success-500" : "bg-danger-500"}`}
														/>
														<span
															className={`text-sm font-medium ${
																update.status === "success"
																	? "text-success-700 dark:text-success-300"
																	: "text-danger-700 dark:text-danger-300"
															}`}
														>
															{update.status === "success"
																? "Success"
																: "Failed"}
														</span>
													</div>
													<div className="text-xs text-secondary-500 dark:text-secondary-400">
														{formatDate(update.timestamp)}
													</div>
												</div>

												<div className="flex flex-wrap items-center gap-3 text-sm pt-2 border-t border-secondary-200 dark:border-secondary-600">
													<div className="flex items-center gap-2">
														<Package className="h-4 w-4 text-secondary-400" />
														<span className="text-secondary-700 dark:text-secondary-300">
															Total: {update.total_packages || "-"}
														</span>
													</div>
													<div className="flex items-center gap-2">
														<span className="text-secondary-700 dark:text-secondary-300">
															Outdated: {update.packages_count || "-"}
														</span>
													</div>
													{update.security_count > 0 && (
														<div className="flex items-center gap-1">
															<Shield className="h-4 w-4 text-danger-600" />
															<span className="text-danger-600 font-medium">
																{update.security_count} Security
															</span>
														</div>
													)}
												</div>

												<div className="flex flex-wrap items-center gap-4 text-xs text-secondary-500 dark:text-secondary-400 pt-2 border-t border-secondary-200 dark:border-secondary-600">
													{update.payload_size_kb && (
														<div>
															Payload: {update.payload_size_kb.toFixed(2)} KB
														</div>
													)}
													{update.execution_time && (
														<div>
															Exec Time: {update.execution_time.toFixed(2)}s
														</div>
													)}
												</div>
											</div>
										))}
									</div>
									{host.update_history.length > 1 && (
										<button
											type="button"
											onClick={() => setShowAllReports(!showAllReports)}
											className="w-full btn-outline flex items-center justify-center gap-2 py-2 text-sm"
										>
											{showAllReports ? (
												<>
													Show Less
													<X className="h-4 w-4" />
												</>
											) : (
												<>
													Show More ({host.update_history.length - 1} more)
													<Calendar className="h-4 w-4" />
												</>
											)}
										</button>
									)}
								</>
							) : (
								<div className="text-center py-8">
									<Calendar className="h-8 w-8 text-secondary-400 mx-auto mb-2" />
									<p className="text-sm text-secondary-500 dark:text-secondary-300">
										No update history available
									</p>
								</div>
							)}
						</div>
					</div>

					{/* Notes Card */}
					<div className="card p-4">
						<h3 className="text-lg font-semibold text-secondary-900 dark:text-white mb-4">
							Notes
						</h3>
						<div className="space-y-4">
							{notesMessage.text && (
								<div
									className={`rounded-md p-4 ${
										notesMessage.type === "success"
											? "bg-green-50 dark:bg-green-900 border border-green-200 dark:border-green-700"
											: "bg-red-50 dark:bg-red-900 border border-red-200 dark:border-red-700"
									}`}
								>
									<div className="flex">
										{notesMessage.type === "success" ? (
											<CheckCircle className="h-5 w-5 text-green-400 dark:text-green-300" />
										) : (
											<AlertCircle className="h-5 w-5 text-red-400 dark:text-red-300" />
										)}
										<div className="ml-3">
											<p
												className={`text-sm font-medium ${
													notesMessage.type === "success"
														? "text-green-800 dark:text-green-200"
														: "text-red-800 dark:text-red-200"
												}`}
											>
												{notesMessage.text}
											</p>
										</div>
									</div>
								</div>
							)}

							<div className="bg-secondary-50 dark:bg-secondary-700 rounded-lg p-4">
								<textarea
									value={notes}
									onChange={(e) => setNotes(e.target.value)}
									placeholder="Add notes about this host..."
									className="w-full h-32 p-3 border border-secondary-200 dark:border-secondary-600 rounded-lg bg-white dark:bg-secondary-800 text-secondary-900 dark:text-white placeholder-secondary-500 dark:placeholder-secondary-400 focus:ring-2 focus:ring-primary-500 focus:border-primary-500 resize-none"
									maxLength={1000}
								/>
								<div className="flex justify-between items-center mt-3">
									<p className="text-xs text-secondary-500 dark:text-secondary-400">
										{notes.length}/1000
									</p>
									<button
										type="button"
										onClick={() => {
											updateNotesMutation.mutate({
												hostId: host.id,
												notes: notes,
											});
										}}
										disabled={updateNotesMutation.isPending}
										className="px-3 py-1.5 text-xs font-medium text-white bg-primary-600 hover:bg-primary-700 disabled:bg-primary-400 rounded-md transition-colors"
									>
										{updateNotesMutation.isPending ? "Saving..." : "Save Notes"}
									</button>
								</div>
							</div>
						</div>
					</div>

					{/* Agent Queue Card */}
					<div className="card p-4">
						<h3 className="text-lg font-semibold text-secondary-900 dark:text-white mb-4 flex items-center gap-2">
							<Server className="h-5 w-5 text-primary-600" />
							Agent Queue
						</h3>
						<AgentQueueTab hostId={hostId} />
					</div>

					{/* Integrations Card */}
					<div className="card p-4">
						<h3 className="text-lg font-semibold text-secondary-900 dark:text-white mb-4">
							Integrations
						</h3>
						{isLoadingIntegrations ? (
							<div className="flex items-center justify-center h-32">
								<RefreshCw className="h-6 w-6 animate-spin text-primary-600" />
							</div>
						) : (
							<div className="space-y-4">
								<div className="bg-secondary-50 dark:bg-secondary-700 rounded-lg p-4 border border-secondary-200 dark:border-secondary-600">
									<div className="flex items-start justify-between gap-4">
										<div className="flex-1">
											<div className="flex items-center gap-3 mb-2">
												<Database className="h-5 w-5 text-primary-600 dark:text-primary-400" />
												<h4 className="text-sm font-medium text-secondary-900 dark:text-white">
													Docker
												</h4>
												{integrationsData?.data?.integrations?.docker ? (
													<span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-semibold bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">
														Enabled
													</span>
												) : (
													<span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-semibold bg-gray-200 text-gray-600 dark:bg-gray-600 dark:text-gray-400">
														Disabled
													</span>
												)}
											</div>
											<p className="text-xs text-secondary-600 dark:text-secondary-300">
												Monitor Docker containers, images, volumes, and
												networks.
											</p>
										</div>
										<div className="flex-shrink-0">
											<button
												type="button"
												onClick={() =>
													toggleIntegrationMutation.mutate({
														integrationName: "docker",
														enabled:
															!integrationsData?.data?.integrations?.docker,
													})
												}
												disabled={
													toggleIntegrationMutation.isPending ||
													!wsStatus?.connected
												}
												className={`relative inline-flex h-5 w-9 items-center rounded-md transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 ${
													integrationsData?.data?.integrations?.docker
														? "bg-primary-600 dark:bg-primary-500"
														: "bg-secondary-200 dark:bg-secondary-600"
												} ${
													toggleIntegrationMutation.isPending ||
													!integrationsData?.data?.connected
														? "opacity-50 cursor-not-allowed"
														: ""
												}`}
											>
												<span
													className={`inline-block h-3 w-3 transform rounded-full bg-white transition-transform ${
														integrationsData?.data?.integrations?.docker
															? "translate-x-5"
															: "translate-x-1"
													}`}
												/>
											</button>
										</div>
									</div>
									{!wsStatus?.connected && (
										<p className="text-xs text-warning-600 dark:text-warning-400 mt-2">
											Agent must be connected via WebSocket to toggle
											integrations
										</p>
									)}
								</div>
							</div>
						)}
					</div>
				</div>

				{/* Desktop View - Tab Interface */}
				<div className="hidden md:block card">
					<div className="flex border-b border-secondary-200 dark:border-secondary-600">
						<button
							type="button"
							onClick={() => handleTabChange("host")}
							className={`px-4 py-2 text-sm font-medium ${
								activeTab === "host"
									? "text-primary-600 dark:text-primary-400 border-b-2 border-primary-500"
									: "text-secondary-500 dark:text-secondary-400 hover:text-secondary-700 dark:hover:text-secondary-300"
							}`}
						>
							Host Info
						</button>
						<button
							type="button"
							onClick={() => handleTabChange("network")}
							className={`px-4 py-2 text-sm font-medium ${
								activeTab === "network"
									? "text-primary-600 dark:text-primary-400 border-b-2 border-primary-500"
									: "text-secondary-500 dark:text-secondary-400 hover:text-secondary-700 dark:hover:text-secondary-300"
							}`}
						>
							Network
						</button>
						<button
							type="button"
							onClick={() => handleTabChange("system")}
							className={`px-4 py-2 text-sm font-medium ${
								activeTab === "system"
									? "text-primary-600 dark:text-primary-400 border-b-2 border-primary-500"
									: "text-secondary-500 dark:text-secondary-400 hover:text-secondary-700 dark:hover:text-secondary-300"
							}`}
						>
							System
						</button>
						<button
							type="button"
							onClick={() => handleTabChange("history")}
							className={`px-4 py-2 text-sm font-medium ${
								activeTab === "history"
									? "text-primary-600 dark:text-primary-400 border-b-2 border-primary-500"
									: "text-secondary-500 dark:text-secondary-400 hover:text-secondary-700 dark:hover:text-secondary-300"
							}`}
						>
							Package Reports
						</button>
						<button
							type="button"
							onClick={() => handleTabChange("queue")}
							className={`px-4 py-2 text-sm font-medium ${
								activeTab === "queue"
									? "text-primary-600 dark:text-primary-400 border-b-2 border-primary-500"
									: "text-secondary-500 dark:text-secondary-400 hover:text-secondary-700 dark:hover:text-secondary-300"
							}`}
						>
							Agent Queue
						</button>
						<button
							type="button"
							onClick={() => handleTabChange("notes")}
							className={`px-4 py-2 text-sm font-medium ${
								activeTab === "notes"
									? "text-primary-600 dark:text-primary-400 border-b-2 border-primary-500"
									: "text-secondary-500 dark:text-secondary-400 hover:text-secondary-700 dark:hover:text-secondary-300"
							}`}
						>
							Notes
						</button>
						<button
							type="button"
							onClick={() => handleTabChange("integrations")}
							className={`px-4 py-2 text-sm font-medium ${
								activeTab === "integrations"
									? "text-primary-600 dark:text-primary-400 border-b-2 border-primary-500"
									: "text-secondary-500 dark:text-secondary-400 hover:text-secondary-700 dark:hover:text-secondary-300"
							}`}
						>
							Integrations
						</button>
						{integrationsData?.data?.integrations?.docker && (
							<button
								type="button"
								onClick={() => handleTabChange("docker")}
								className={`px-4 py-2 text-sm font-medium ${
									activeTab === "docker"
										? "text-primary-600 dark:text-primary-400 border-b-2 border-primary-500"
										: "text-secondary-500 dark:text-secondary-400 hover:text-secondary-700 dark:hover:text-secondary-300"
								}`}
							>
								<Database className="h-4 w-4 inline mr-1" />
								Docker
							</button>
						)}
						{integrationsData?.data?.integrations?.compliance && (
							<button
								type="button"
								onClick={() => handleTabChange("compliance")}
								className={`px-4 py-2 text-sm font-medium ${
									activeTab === "compliance"
										? "text-primary-600 dark:text-primary-400 border-b-2 border-primary-500"
										: "text-secondary-500 dark:text-secondary-400 hover:text-secondary-700 dark:hover:text-secondary-300"
								}`}
							>
								<Shield className="h-4 w-4 inline mr-1" />
								Compliance
							</button>
						)}
						<button
							type="button"
							onClick={() => handleTabChange("terminal")}
							className={`px-4 py-2 text-sm font-medium ${
								activeTab === "terminal"
									? "text-primary-600 dark:text-primary-400 border-b-2 border-primary-500"
									: "text-secondary-500 dark:text-secondary-400 hover:text-secondary-700 dark:hover:text-secondary-300"
							}`}
						>
							<Terminal className="h-4 w-4 inline mr-1" />
							Terminal
						</button>
					</div>

					<div className="p-4">
						{/* Host Information */}
						{activeTab === "host" && (
							<div className="space-y-4">
								<div className="grid grid-cols-1 md:grid-cols-2 gap-4">
									<div>
										<p className="text-xs text-secondary-500 dark:text-secondary-300 mb-1.5">
											Friendly Name
										</p>
										<InlineEdit
											value={host.friendly_name}
											onSave={(newName) =>
												updateFriendlyNameMutation.mutate(newName)
											}
											placeholder="Enter friendly name..."
											maxLength={100}
											validate={(value) => {
												if (!value.trim()) return "Friendly name is required";
												if (value.trim().length < 1)
													return "Friendly name must be at least 1 character";
												if (value.trim().length > 100)
													return "Friendly name must be less than 100 characters";
												return null;
											}}
											className="w-full text-sm"
										/>
									</div>

									<div>
										<p className="text-xs text-secondary-500 dark:text-secondary-300 mb-1.5">
											IP Address
										</p>
										<InlineEdit
											value={host.ip || ""}
											onSave={(newIp) => {
												if (!newIp.trim()) {
													updateConnectionMutation.mutate({ ip: null });
												} else {
													updateConnectionMutation.mutate({ ip: newIp.trim() });
												}
											}}
											placeholder="No IP set (click to add)"
											validate={(value) => {
												if (value.trim() && !/^(\d{1,3}\.){3}\d{1,3}$/.test(value.trim())) {
													return "Invalid IP address format";
												}
												return null;
											}}
											className="w-full text-sm font-mono"
										/>
									</div>

									<div>
										<p className="text-xs text-secondary-500 dark:text-secondary-300 mb-1.5">
											Hostname
										</p>
										<InlineEdit
											value={host.hostname || ""}
											onSave={(newHostname) => {
												if (!newHostname.trim()) {
													updateConnectionMutation.mutate({ hostname: null });
												} else {
													updateConnectionMutation.mutate({ hostname: newHostname.trim() });
												}
											}}
											placeholder="No hostname set (click to add)"
											maxLength={255}
											className="w-full text-sm font-mono"
										/>
									</div>

									{host.machine_id && (
										<div>
											<p className="text-xs text-secondary-500 dark:text-secondary-300 mb-1.5">
												Machine ID
											</p>
											<p className="font-medium text-secondary-900 dark:text-white font-mono text-sm break-all">
												{host.machine_id}
											</p>
										</div>
									)}

									<div>
										<p className="text-xs text-secondary-500 dark:text-secondary-300 mb-1.5">
											Host Groups
										</p>
										{/* Extract group IDs from the new many-to-many structure */}
										{(() => {
											const groupIds =
												host.host_group_memberships?.map(
													(membership) => membership.host_groups.id,
												) || [];
											return (
												<InlineMultiGroupEdit
													key={`${host.id}-${groupIds.join(",")}`}
													value={groupIds}
													onSave={(newGroupIds) =>
														updateHostGroupsMutation.mutate({
															hostId: host.id,
															groupIds: newGroupIds,
														})
													}
													options={hostGroups || []}
													placeholder="Select groups..."
													className="w-full"
												/>
											);
										})()}
									</div>

									<div>
										<p className="text-xs text-secondary-500 dark:text-secondary-300 mb-1.5">
											Operating System
										</p>
										<div className="flex items-center gap-2">
											<OSIcon osType={host.os_type} className="h-4 w-4" />
											<p className="font-medium text-secondary-900 dark:text-white text-sm">
												{host.os_type} {host.os_version}
											</p>
										</div>
									</div>

									<div>
										<p className="text-xs text-secondary-500 dark:text-secondary-300 mb-1.5">
											Agent Version
										</p>
										<p className="font-medium text-secondary-900 dark:text-white text-sm">
											{host.agent_version || "Unknown"}
										</p>
									</div>

									<div>
										<p className="text-xs text-secondary-500 dark:text-secondary-300 mb-1.5">
											Agent Auto-update
										</p>
										<button
											type="button"
											onClick={() =>
												toggleAutoUpdateMutation.mutate(!host.auto_update)
											}
											disabled={toggleAutoUpdateMutation.isPending}
											className={`relative inline-flex h-5 w-9 items-center rounded-md transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 ${
												host.auto_update
													? "bg-primary-600 dark:bg-primary-500"
													: "bg-secondary-200 dark:bg-secondary-600"
											}`}
										>
											<span
												className={`inline-block h-3 w-3 transform rounded-full bg-white transition-transform ${
													host.auto_update ? "translate-x-5" : "translate-x-1"
												}`}
											/>
										</button>
									</div>

									<div>
										<p className="text-xs text-secondary-500 dark:text-secondary-300 mb-1.5">
											Force Agent Version Upgrade
										</p>
										<button
											type="button"
											onClick={() => forceAgentUpdateMutation.mutate()}
											disabled={
												forceAgentUpdateMutation.isPending ||
												!wsStatus?.connected
											}
											title={
												!wsStatus?.connected
													? "Agent is not connected"
													: "Force agent to update now"
											}
											className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-primary-600 dark:text-primary-400 bg-primary-50 dark:bg-primary-900/20 border border-primary-200 dark:border-primary-800 rounded-md hover:bg-primary-100 dark:hover:bg-primary-900/40 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
										>
											<RefreshCw
												className={`h-3 w-3 ${
													forceAgentUpdateMutation.isPending
														? "animate-spin"
														: ""
												}`}
											/>
											{forceAgentUpdateMutation.isPending
												? "Updating..."
												: wsStatus?.connected
													? "Update Now"
													: "Offline"}
										</button>
										{updateMessage.text && (
											<p className="text-xs mt-1.5 text-secondary-600 dark:text-secondary-400">
												{updateMessage.text}
												{updateMessage.jobId && (
													<span className="ml-1 font-mono text-secondary-500">
														(Job #{updateMessage.jobId})
													</span>
												)}
											</p>
										)}
									</div>

								</div>
							</div>
						)}

						{/* Network Information */}
						{activeTab === "network" &&
							(host.dns_servers || host.network_interfaces) && (
								<div className="space-y-6">
									{/* DNS Servers */}
									{host.dns_servers &&
										Array.isArray(host.dns_servers) &&
										host.dns_servers.length > 0 && (
											<div>
												<h4 className="text-sm font-medium text-secondary-900 dark:text-white mb-3 flex items-center gap-2">
													<Wifi className="h-4 w-4 text-primary-600 dark:text-primary-400" />
													DNS Servers
												</h4>
												<div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
													{host.dns_servers.map((dns) => (
														<div
															key={dns}
															className="bg-secondary-50 dark:bg-secondary-700 p-3 rounded-lg border border-secondary-200 dark:border-secondary-600"
														>
															<p className="font-mono text-sm font-medium text-secondary-900 dark:text-white">
																{dns}
															</p>
														</div>
													))}
												</div>
											</div>
										)}

									{/* Network Interfaces */}
									{host.network_interfaces &&
										Array.isArray(host.network_interfaces) &&
										host.network_interfaces.length > 0 && (
											<div>
												<h4 className="text-sm font-medium text-secondary-900 dark:text-white mb-4 flex items-center gap-2">
													<Wifi className="h-4 w-4 text-primary-600 dark:text-primary-400" />
													Network Interfaces
												</h4>
												<div className="space-y-4">
													{host.network_interfaces.map((iface) => (
														<div
															key={iface.name}
															className="border border-secondary-200 dark:border-secondary-700 rounded-lg p-4 bg-secondary-50 dark:bg-secondary-900/50"
														>
															{/* Interface Header */}
															<div className="flex items-center justify-between mb-3">
																<div className="flex items-center gap-2">
																	<p className="font-semibold text-secondary-900 dark:text-white text-sm">
																		{iface.name}
																	</p>
																	{iface.type && (
																		<span className="text-xs text-secondary-500 dark:text-secondary-400 bg-secondary-200 dark:bg-secondary-700 px-2 py-0.5 rounded">
																			{iface.type}
																		</span>
																	)}
																	{iface.status && (
																		<span
																			className={`text-xs font-medium px-2 py-0.5 rounded ${
																				iface.status === "up"
																					? "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200"
																					: "bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200"
																			}`}
																		>
																			{iface.status === "up" ? "UP" : "DOWN"}
																		</span>
																	)}
																</div>
															</div>

															{/* Interface Details */}
															<div className="grid grid-cols-1 md:grid-cols-2 gap-3 text-xs mb-3">
																{iface.macAddress && (
																	<div>
																		<p className="text-secondary-500 dark:text-secondary-400 mb-0.5">
																			MAC Address
																		</p>
																		<p className="font-mono text-secondary-900 dark:text-white">
																			{iface.macAddress}
																		</p>
																	</div>
																)}
																{iface.mtu && (
																	<div>
																		<p className="text-secondary-500 dark:text-secondary-400 mb-0.5">
																			MTU
																		</p>
																		<p className="text-secondary-900 dark:text-white">
																			{iface.mtu}
																		</p>
																	</div>
																)}
																{iface.linkSpeed && iface.linkSpeed > 0 && (
																	<div>
																		<p className="text-secondary-500 dark:text-secondary-400 mb-0.5">
																			Link Speed
																		</p>
																		<p className="text-secondary-900 dark:text-white">
																			{iface.linkSpeed} Mbps
																			{iface.duplex &&
																				` (${iface.duplex} duplex)`}
																		</p>
																	</div>
																)}
															</div>

															{/* Addresses */}
															{iface.addresses &&
																Array.isArray(iface.addresses) &&
																iface.addresses.length > 0 && (
																	<div className="space-y-2 pt-3 border-t border-secondary-200 dark:border-secondary-700">
																		<p className="text-xs font-medium text-secondary-500 dark:text-secondary-400 mb-2">
																			IP Addresses
																		</p>
																		<div className="space-y-2">
																			{iface.addresses.map((addr, idx) => (
																				<div
																					key={`${addr.address}-${addr.family}-${idx}`}
																					className="bg-white dark:bg-secondary-800 rounded p-2 border border-secondary-200 dark:border-secondary-700"
																				>
																					<div className="flex items-center gap-2 mb-1">
																						<span
																							className={`inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium ${
																								addr.family === "inet6"
																									? "bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200"
																									: "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200"
																							}`}
																						>
																							{addr.family === "inet6"
																								? "inet6"
																								: "inet"}
																						</span>
																						<span className="font-mono text-sm font-semibold text-secondary-900 dark:text-white">
																							{addr.address}
																							{addr.netmask && (
																								<span className="text-secondary-500 dark:text-secondary-400 ml-1">
																									{addr.netmask}
																								</span>
																							)}
																						</span>
																					</div>
																					{addr.gateway && (
																						<div className="text-xs text-secondary-600 dark:text-secondary-400 ml-1">
																							Gateway:{" "}
																							<span className="font-mono">
																								{addr.gateway}
																							</span>
																						</div>
																					)}
																				</div>
																			))}
																		</div>
																	</div>
																)}
														</div>
													))}
												</div>
											</div>
										)}
								</div>
							)}

						{/* System Information */}
						{activeTab === "system" && (
							<div className="space-y-6">
								{/* Basic System Information */}
								{(host.kernel_version ||
									host.selinux_status ||
									host.architecture) && (
									<div>
										<h4 className="text-sm font-medium text-secondary-900 dark:text-white mb-3 flex items-center gap-2">
											<Terminal className="h-4 w-4 text-primary-600 dark:text-primary-400" />
											System Information
										</h4>
										<div className="grid grid-cols-1 md:grid-cols-3 gap-4">
											{host.architecture && (
												<div>
													<p className="text-xs text-secondary-500 dark:text-secondary-300">
														Architecture
													</p>
													<p className="font-medium text-secondary-900 dark:text-white text-sm">
														{host.architecture}
													</p>
												</div>
											)}

											{host.kernel_version && (
												<div>
													<p className="text-xs text-secondary-500 dark:text-secondary-300">
														Running Kernel
													</p>
													<p className="font-medium text-secondary-900 dark:text-white font-mono text-sm break-all">
														{host.kernel_version}
													</p>
												</div>
											)}

											{host.installed_kernel_version && (
												<div>
													<p className="text-xs text-secondary-500 dark:text-secondary-300">
														Installed Kernel
													</p>
													<p className="font-medium text-secondary-900 dark:text-white font-mono text-sm break-all">
														{host.installed_kernel_version}
													</p>
												</div>
											)}

											{host.selinux_status && (
												<div>
													<p className="text-xs text-secondary-500 dark:text-secondary-300">
														SELinux Status
													</p>
													<span
														className={`inline-flex items-center px-2 py-1 rounded-full text-xs font-medium ${
															host.selinux_status === "enabled"
																? "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200"
																: host.selinux_status === "permissive"
																	? "bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200"
																	: "bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200"
														}`}
													>
														{host.selinux_status}
													</span>
												</div>
											)}
										</div>
									</div>
								)}

								{/* Resource Information */}
								{(host.system_uptime ||
									host.cpu_model ||
									host.cpu_cores ||
									host.ram_installed ||
									host.swap_size !== undefined ||
									(host.load_average &&
										Array.isArray(host.load_average) &&
										host.load_average.length > 0 &&
										host.load_average.some((load) => load != null)) ||
									(host.disk_details &&
										Array.isArray(host.disk_details) &&
										host.disk_details.length > 0)) && (
									<div className="pt-4 border-t border-secondary-200 dark:border-secondary-600">
										<h4 className="text-sm font-medium text-secondary-900 dark:text-white mb-3 flex items-center gap-2">
											<Monitor className="h-4 w-4 text-primary-600 dark:text-primary-400" />
											Resource Information
										</h4>

										{/* System Overview */}
										<div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
											{/* System Uptime */}
											{host.system_uptime && (
												<div className="bg-secondary-50 dark:bg-secondary-700 p-4 rounded-lg">
													<div className="flex items-center gap-2 mb-2">
														<Clock className="h-4 w-4 text-primary-600 dark:text-primary-400" />
														<p className="text-xs text-secondary-500 dark:text-secondary-300">
															System Uptime
														</p>
													</div>
													<p className="font-medium text-secondary-900 dark:text-white text-sm">
														{host.system_uptime}
													</p>
												</div>
											)}

											{/* CPU Model */}
											{host.cpu_model && (
												<div className="bg-secondary-50 dark:bg-secondary-700 p-4 rounded-lg">
													<div className="flex items-center gap-2 mb-2">
														<Cpu className="h-4 w-4 text-primary-600 dark:text-primary-400" />
														<p className="text-xs text-secondary-500 dark:text-secondary-300">
															CPU Model
														</p>
													</div>
													<p className="font-medium text-secondary-900 dark:text-white text-sm">
														{host.cpu_model}
													</p>
												</div>
											)}

											{/* CPU Cores */}
											{host.cpu_cores && (
												<div className="bg-secondary-50 dark:bg-secondary-700 p-4 rounded-lg">
													<div className="flex items-center gap-2 mb-2">
														<Cpu className="h-4 w-4 text-primary-600 dark:text-primary-400" />
														<p className="text-xs text-secondary-500 dark:text-secondary-300">
															CPU Cores
														</p>
													</div>
													<p className="font-medium text-secondary-900 dark:text-white text-sm">
														{host.cpu_cores}
													</p>
												</div>
											)}

											{/* RAM Installed */}
											{host.ram_installed && (
												<div className="bg-secondary-50 dark:bg-secondary-700 p-4 rounded-lg">
													<div className="flex items-center gap-2 mb-2">
														<MemoryStick className="h-4 w-4 text-primary-600 dark:text-primary-400" />
														<p className="text-xs text-secondary-500 dark:text-secondary-300">
															RAM Installed
														</p>
													</div>
													<p className="font-medium text-secondary-900 dark:text-white text-sm">
														{host.ram_installed} GB
													</p>
												</div>
											)}

											{/* Swap Size */}
											{host.swap_size !== undefined &&
												host.swap_size !== null && (
													<div className="bg-secondary-50 dark:bg-secondary-700 p-4 rounded-lg">
														<div className="flex items-center gap-2 mb-2">
															<MemoryStick className="h-4 w-4 text-primary-600 dark:text-primary-400" />
															<p className="text-xs text-secondary-500 dark:text-secondary-300">
																Swap Size
															</p>
														</div>
														<p className="font-medium text-secondary-900 dark:text-white text-sm">
															{host.swap_size} GB
														</p>
													</div>
												)}

											{/* Load Average */}
											{host.load_average &&
												Array.isArray(host.load_average) &&
												host.load_average.length > 0 &&
												host.load_average.some((load) => load != null) && (
													<div className="bg-secondary-50 dark:bg-secondary-700 p-4 rounded-lg">
														<div className="flex items-center gap-2 mb-2">
															<Activity className="h-4 w-4 text-primary-600 dark:text-primary-400" />
															<p className="text-xs text-secondary-500 dark:text-secondary-300">
																Load Average
															</p>
														</div>
														<p className="font-medium text-secondary-900 dark:text-white text-sm">
															{host.load_average
																.filter((load) => load != null)
																.map((load, index) => (
																	<span key={`load-${index}-${load}`}>
																		{typeof load === "number"
																			? load.toFixed(2)
																			: String(load)}
																		{index <
																			host.load_average.filter(
																				(load) => load != null,
																			).length -
																				1 && ", "}
																	</span>
																))}
														</p>
													</div>
												)}
										</div>

										{/* Disk Information */}
										{host.disk_details &&
											Array.isArray(host.disk_details) &&
											host.disk_details.length > 0 && (
												<div className="pt-4 border-t border-secondary-200 dark:border-secondary-600">
													<h5 className="text-sm font-medium text-secondary-900 dark:text-white mb-3 flex items-center gap-2">
														<HardDrive className="h-4 w-4 text-primary-600 dark:text-primary-400" />
														Disk Usage
													</h5>
													<div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
														{host.disk_details.map((disk, index) => (
															<div
																key={disk.name || `disk-${index}`}
																className="bg-secondary-50 dark:bg-secondary-700 p-3 rounded-lg"
															>
																<div className="flex items-center gap-2 mb-2">
																	<HardDrive className="h-4 w-4 text-secondary-500" />
																	<span className="font-medium text-secondary-900 dark:text-white text-sm">
																		{disk.name || `Disk ${index + 1}`}
																	</span>
																</div>
																{disk.size && (
																	<p className="text-xs text-secondary-600 dark:text-secondary-300 mb-1">
																		Size: {disk.size}
																	</p>
																)}
																{disk.mountpoint && (
																	<p className="text-xs text-secondary-600 dark:text-secondary-300 mb-1">
																		Mount: {disk.mountpoint}
																	</p>
																)}
																{disk.usage &&
																	typeof disk.usage === "number" && (
																		<div className="mt-2">
																			<div className="flex justify-between text-xs text-secondary-600 dark:text-secondary-300 mb-1">
																				<span>Usage</span>
																				<span>{disk.usage}%</span>
																			</div>
																			<div className="w-full bg-secondary-200 dark:bg-secondary-600 rounded-full h-2">
																				<div
																					className="bg-primary-600 dark:bg-primary-400 h-2 rounded-full transition-all duration-300"
																					style={{
																						width: `${Math.min(Math.max(disk.usage, 0), 100)}%`,
																					}}
																				></div>
																			</div>
																		</div>
																	)}
															</div>
														))}
													</div>
												</div>
											)}
									</div>
								)}

								{/* No Data State */}
								{!host.kernel_version &&
									!host.selinux_status &&
									!host.architecture &&
									!host.system_uptime &&
									!host.cpu_model &&
									!host.cpu_cores &&
									!host.ram_installed &&
									host.swap_size === undefined &&
									(!host.load_average ||
										!Array.isArray(host.load_average) ||
										host.load_average.length === 0 ||
										!host.load_average.some((load) => load != null)) &&
									(!host.disk_details ||
										!Array.isArray(host.disk_details) ||
										host.disk_details.length === 0) && (
										<div className="text-center py-8">
											<Terminal className="h-8 w-8 text-secondary-400 mx-auto mb-2" />
											<p className="text-sm text-secondary-500 dark:text-secondary-300">
												No system information available
											</p>
											<p className="text-xs text-secondary-400 dark:text-secondary-400 mt-1">
												System information will appear once the agent collects
												data from this host
											</p>
										</div>
									)}
							</div>
						)}

						{activeTab === "network" &&
							!(
								host.ip ||
								host.gateway_ip ||
								host.dns_servers ||
								host.network_interfaces
							) && (
								<div className="text-center py-8">
									<Wifi className="h-8 w-8 text-secondary-400 mx-auto mb-2" />
									<p className="text-sm text-secondary-500 dark:text-secondary-300">
										No network information available
									</p>
								</div>
							)}

						{/* Update History */}
						{activeTab === "history" && (
							<div className="space-y-4">
								{host.update_history?.length > 0 ? (
									<>
										{/* Mobile Card Layout */}
										<div className="md:hidden space-y-3">
											{host.update_history.map((update) => (
												<div
													key={update.id}
													className="p-3 bg-secondary-50 dark:bg-secondary-700 rounded-lg space-y-2"
												>
													<div className="flex items-start justify-between gap-3">
														<div className="flex items-center gap-1.5">
															<div
																className={`w-1.5 h-1.5 rounded-full ${update.status === "success" ? "bg-success-500" : "bg-danger-500"}`}
															/>
															<span
																className={`text-sm font-medium ${
																	update.status === "success"
																		? "text-success-700 dark:text-success-300"
																		: "text-danger-700 dark:text-danger-300"
																}`}
															>
																{update.status === "success"
																	? "Success"
																	: "Failed"}
															</span>
														</div>
														<div className="text-xs text-secondary-500 dark:text-secondary-400">
															{formatDate(update.timestamp)}
														</div>
													</div>

													<div className="flex flex-wrap items-center gap-3 text-sm pt-2 border-t border-secondary-200 dark:border-secondary-600">
														<div className="flex items-center gap-2">
															<Package className="h-4 w-4 text-secondary-400" />
															<span className="text-secondary-700 dark:text-secondary-300">
																Total: {update.total_packages || "-"}
															</span>
														</div>
														<div className="flex items-center gap-2">
															<span className="text-secondary-700 dark:text-secondary-300">
																Outdated: {update.packages_count || "-"}
															</span>
														</div>
														{update.security_count > 0 && (
															<div className="flex items-center gap-1">
																<Shield className="h-4 w-4 text-danger-600" />
																<span className="text-danger-600 font-medium">
																	{update.security_count} Security
																</span>
															</div>
														)}
													</div>

													<div className="flex flex-wrap items-center gap-4 text-xs text-secondary-500 dark:text-secondary-400 pt-2 border-t border-secondary-200 dark:border-secondary-600">
														{update.payload_size_kb && (
															<div>
																Payload: {update.payload_size_kb.toFixed(2)} KB
															</div>
														)}
														{update.execution_time && (
															<div>
																Exec Time: {update.execution_time.toFixed(2)}s
															</div>
														)}
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
															Status
														</th>
														<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
															Date
														</th>
														<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
															Total Packages
														</th>
														<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
															Outdated Packages
														</th>
														<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
															Security
														</th>
														<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
															Payload (KB)
														</th>
														<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
															Exec Time (s)
														</th>
													</tr>
												</thead>
												<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-600">
													{host.update_history.map((update) => (
														<tr
															key={update.id}
															className="hover:bg-secondary-50 dark:hover:bg-secondary-700"
														>
															<td className="px-4 py-2 whitespace-nowrap">
																<div className="flex items-center gap-1.5">
																	<div
																		className={`w-1.5 h-1.5 rounded-full ${update.status === "success" ? "bg-success-500" : "bg-danger-500"}`}
																	/>
																	<span
																		className={`text-xs font-medium ${
																			update.status === "success"
																				? "text-success-700 dark:text-success-300"
																				: "text-danger-700 dark:text-danger-300"
																		}`}
																	>
																		{update.status === "success"
																			? "Success"
																			: "Failed"}
																	</span>
																</div>
															</td>
															<td className="px-4 py-2 whitespace-nowrap text-xs text-secondary-900 dark:text-white">
																{formatDate(update.timestamp)}
															</td>
															<td className="px-4 py-2 whitespace-nowrap text-xs text-secondary-900 dark:text-white">
																{update.total_packages || "-"}
															</td>
															<td className="px-4 py-2 whitespace-nowrap text-xs text-secondary-900 dark:text-white">
																{update.packages_count}
															</td>
															<td className="px-4 py-2 whitespace-nowrap">
																{update.security_count > 0 ? (
																	<div className="flex items-center gap-1">
																		<Shield className="h-3 w-3 text-danger-600" />
																		<span className="text-xs text-danger-600 font-medium">
																			{update.security_count}
																		</span>
																	</div>
																) : (
																	<span className="text-xs text-secondary-500 dark:text-secondary-400">
																		-
																	</span>
																)}
															</td>
															<td className="px-4 py-2 whitespace-nowrap text-xs text-secondary-900 dark:text-white">
																{update.payload_size_kb
																	? `${update.payload_size_kb.toFixed(2)}`
																	: "-"}
															</td>
															<td className="px-4 py-2 whitespace-nowrap text-xs text-secondary-900 dark:text-white">
																{update.execution_time
																	? `${update.execution_time.toFixed(2)}`
																	: "-"}
															</td>
														</tr>
													))}
												</tbody>
											</table>
										</div>

										{/* Pagination Controls */}
										{host.pagination &&
											host.pagination.total > historyLimit && (
												<div className="flex items-center justify-between px-4 py-3 border-t border-secondary-200 dark:border-secondary-600 bg-secondary-50 dark:bg-secondary-700">
													<div className="flex items-center gap-2 text-sm text-secondary-600 dark:text-secondary-300">
														<span>
															Showing {historyPage * historyLimit + 1} to{" "}
															{Math.min(
																(historyPage + 1) * historyLimit,
																host.pagination.total,
															)}{" "}
															of {host.pagination.total} entries
														</span>
													</div>
													<div className="flex items-center gap-2">
														<button
															type="button"
															onClick={() => setHistoryPage(0)}
															disabled={historyPage === 0}
															className="px-3 py-1 text-xs font-medium text-secondary-600 dark:text-secondary-300 hover:text-secondary-800 dark:hover:text-secondary-100 disabled:opacity-50 disabled:cursor-not-allowed"
														>
															First
														</button>
														<button
															type="button"
															onClick={() => setHistoryPage(historyPage - 1)}
															disabled={historyPage === 0}
															className="px-3 py-1 text-xs font-medium text-secondary-600 dark:text-secondary-300 hover:text-secondary-800 dark:hover:text-secondary-100 disabled:opacity-50 disabled:cursor-not-allowed"
														>
															Previous
														</button>
														<span className="px-3 py-1 text-xs font-medium text-secondary-900 dark:text-white">
															Page {historyPage + 1} of{" "}
															{Math.ceil(host.pagination.total / historyLimit)}
														</span>
														<button
															type="button"
															onClick={() => setHistoryPage(historyPage + 1)}
															disabled={!host.pagination.hasMore}
															className="px-3 py-1 text-xs font-medium text-secondary-600 dark:text-secondary-300 hover:text-secondary-800 dark:hover:text-secondary-100 disabled:opacity-50 disabled:cursor-not-allowed"
														>
															Next
														</button>
														<button
															type="button"
															onClick={() =>
																setHistoryPage(
																	Math.ceil(
																		host.pagination.total / historyLimit,
																	) - 1,
																)
															}
															disabled={!host.pagination.hasMore}
															className="px-3 py-1 text-xs font-medium text-secondary-600 dark:text-secondary-300 hover:text-secondary-800 dark:hover:text-secondary-100 disabled:opacity-50 disabled:cursor-not-allowed"
														>
															Last
														</button>
													</div>
												</div>
											)}
									</>
								) : (
									<div className="text-center py-8">
										<Calendar className="h-8 w-8 text-secondary-400 mx-auto mb-2" />
										<p className="text-sm text-secondary-500 dark:text-secondary-300">
											No update history available
										</p>
									</div>
								)}
							</div>
						)}

						{/* Terminal - Always mounted and open to preserve connection, hidden when not active */}
						{host && (
							<div className={activeTab === "terminal" ? "" : "hidden"}>
								<SshTerminal
									host={host}
									isOpen={true}
									onClose={() => handleTabChange("host")}
									embedded={true}
								/>
							</div>
						)}

						{/* Notes */}
						{activeTab === "notes" && (
							<div className="space-y-4">
								<div className="flex items-center justify-between">
									<h3 className="text-lg font-medium text-secondary-900 dark:text-white">
										Host Notes
									</h3>
								</div>

								{/* Success/Error Message */}
								{notesMessage.text && (
									<div
										className={`rounded-md p-4 ${
											notesMessage.type === "success"
												? "bg-green-50 dark:bg-green-900 border border-green-200 dark:border-green-700"
												: "bg-red-50 dark:bg-red-900 border border-red-200 dark:border-red-700"
										}`}
									>
										<div className="flex">
											{notesMessage.type === "success" ? (
												<CheckCircle className="h-5 w-5 text-green-400 dark:text-green-300" />
											) : (
												<AlertCircle className="h-5 w-5 text-red-400 dark:text-red-300" />
											)}
											<div className="ml-3">
												<p
													className={`text-sm font-medium ${
														notesMessage.type === "success"
															? "text-green-800 dark:text-green-200"
															: "text-red-800 dark:text-red-200"
													}`}
												>
													{notesMessage.text}
												</p>
											</div>
										</div>
									</div>
								)}

								<div className="bg-secondary-50 dark:bg-secondary-700 rounded-lg p-4">
									<textarea
										value={notes}
										onChange={(e) => setNotes(e.target.value)}
										placeholder="Add notes about this host... (e.g., purpose, special configurations, maintenance notes)"
										className="w-full h-32 p-3 border border-secondary-200 dark:border-secondary-600 rounded-lg bg-white dark:bg-secondary-800 text-secondary-900 dark:text-white placeholder-secondary-500 dark:placeholder-secondary-400 focus:ring-2 focus:ring-primary-500 focus:border-primary-500 resize-none"
										maxLength={1000}
									/>
									<div className="flex justify-between items-center mt-3">
										<p className="text-xs text-secondary-500 dark:text-secondary-400">
											Use this space to add important information about this
											host for your team
										</p>
										<div className="flex items-center gap-2">
											<span className="text-xs text-secondary-400 dark:text-secondary-500">
												{notes.length}/1000
											</span>
											<button
												type="button"
												onClick={() => {
													updateNotesMutation.mutate({
														hostId: host.id,
														notes: notes,
													});
												}}
												disabled={updateNotesMutation.isPending}
												className="px-3 py-1.5 text-xs font-medium text-white bg-primary-600 hover:bg-primary-700 disabled:bg-primary-400 rounded-md transition-colors"
											>
												{updateNotesMutation.isPending
													? "Saving..."
													: "Save Notes"}
											</button>
										</div>
									</div>
								</div>
							</div>
						)}

						{/* Agent Queue */}
						{activeTab === "queue" && <AgentQueueTab hostId={hostId} />}

						{/* Integrations */}
						{activeTab === "integrations" && (
							<div className="space-y-4">
								{isLoadingIntegrations ? (
									<div className="flex items-center justify-center h-32">
										<RefreshCw className="h-6 w-6 animate-spin text-primary-600" />
									</div>
								) : (
									<div className="grid grid-cols-1 md:grid-cols-2 gap-4">
										{/* Docker Integration */}
										<div className="bg-secondary-50 dark:bg-secondary-700 rounded-lg p-4 border border-secondary-200 dark:border-secondary-600">
											<div className="flex items-start justify-between gap-4">
												<div className="flex-1">
													<div className="flex items-center gap-3 mb-2">
														<Database className="h-5 w-5 text-primary-600 dark:text-primary-400" />
														<h4 className="text-sm font-medium text-secondary-900 dark:text-white">
															Docker
														</h4>
														{integrationsData?.data?.integrations?.docker ? (
															<span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-semibold bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">
																Enabled
															</span>
														) : (
															<span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-semibold bg-gray-200 text-gray-600 dark:bg-gray-600 dark:text-gray-400">
																Disabled
															</span>
														)}
													</div>
													<p className="text-xs text-secondary-600 dark:text-secondary-300">
														Monitor Docker containers, images, volumes, and
														networks. Collects real-time container status
														events.
													</p>
												</div>
												<div className="flex-shrink-0">
													<button
														type="button"
														onClick={() =>
															toggleIntegrationMutation.mutate({
																integrationName: "docker",
																enabled:
																	!integrationsData?.data?.integrations?.docker,
															})
														}
														disabled={
															toggleIntegrationMutation.isPending ||
															!wsStatus?.connected
														}
														title={
															!wsStatus?.connected
																? "Agent is not connected"
																: integrationsData?.data?.integrations?.docker
																	? "Disable Docker integration"
																	: "Enable Docker integration"
														}
														className={`relative inline-flex h-5 w-9 items-center rounded-md transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 ${
															integrationsData?.data?.integrations?.docker
																? "bg-primary-600 dark:bg-primary-500"
																: "bg-secondary-200 dark:bg-secondary-600"
														} ${
															toggleIntegrationMutation.isPending ||
															!integrationsData?.data?.connected
																? "opacity-50 cursor-not-allowed"
																: ""
														}`}
													>
														<span
															className={`inline-block h-3 w-3 transform rounded-full bg-white transition-transform ${
																integrationsData?.data?.integrations?.docker
																	? "translate-x-5"
																	: "translate-x-1"
															}`}
														/>
													</button>
												</div>
											</div>
											{!wsStatus?.connected && (
												<p className="text-xs text-warning-600 dark:text-warning-400 mt-2">
													Agent must be connected via WebSocket to toggle
													integrations
												</p>
											)}
											{toggleIntegrationMutation.isPending && (
												<p className="text-xs text-secondary-600 dark:text-secondary-400 mt-2">
													Updating integration...
												</p>
											)}
										</div>

										{/* Compliance Integration */}
										<div className="bg-secondary-50 dark:bg-secondary-700 rounded-lg p-4 border border-secondary-200 dark:border-secondary-600">
											<div className="flex items-start justify-between gap-4">
												<div className="flex-1">
													<div className="flex items-center gap-3 mb-2">
														<Shield className="h-5 w-5 text-primary-600 dark:text-primary-400" />
														<h4 className="text-sm font-medium text-secondary-900 dark:text-white">
															Compliance Scanning
														</h4>
														{integrationsData?.data?.integrations?.compliance ? (
															<span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-semibold bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">
																Enabled
															</span>
														) : (
															<span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-semibold bg-gray-200 text-gray-600 dark:bg-gray-600 dark:text-gray-400">
																Disabled
															</span>
														)}
													</div>
													<p className="text-xs text-secondary-600 dark:text-secondary-300 mb-2">
														Run CIS benchmark compliance scans using OpenSCAP.
														Provides security posture assessment and remediation recommendations.
													</p>

													{/* Setup Status Display */}
													{complianceSetupStatus?.status?.status && (
														<div className="mt-3 p-3 rounded-lg border bg-secondary-100 dark:bg-secondary-800 border-secondary-300 dark:border-secondary-600">
															{/* Installing State */}
															{complianceSetupStatus.status.status === "installing" && (
																<div className="space-y-2">
																	<div className="flex items-center gap-2">
																		<RefreshCw className="h-4 w-4 animate-spin text-primary-600 dark:text-primary-400" />
																		<span className="text-sm font-medium text-primary-700 dark:text-primary-300">
																			Installing Compliance Tools
																		</span>
																	</div>
																	<div className="w-full bg-secondary-200 dark:bg-secondary-700 rounded-full h-1.5">
																		<div className="bg-primary-600 h-1.5 rounded-full animate-pulse" style={{ width: "60%" }} />
																	</div>
																	<p className="text-xs text-secondary-600 dark:text-secondary-400">
																		{complianceSetupStatus.status.message || "Installing OpenSCAP packages and security content..."}
																	</p>
																	{complianceSetupStatus.status.components && (
																		<div className="flex flex-wrap gap-2 mt-2">
																			{Object.entries(complianceSetupStatus.status.components)
																				.filter(([, status]) => status !== "unavailable")
																				.map(([name, status]) => (
																				<span
																					key={name}
																					className={`inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs ${
																						status === "ready"
																							? "bg-green-100 text-green-700 dark:bg-green-900 dark:text-green-300"
																							: status === "failed"
																								? "bg-red-100 text-red-700 dark:bg-red-900 dark:text-red-300"
																								: "bg-secondary-200 text-secondary-600 dark:bg-secondary-600 dark:text-secondary-300"
																					}`}
																				>
																					{status === "ready" && <CheckCircle2 className="h-3 w-3" />}
																					{status === "failed" && <AlertCircle className="h-3 w-3" />}
																					{status !== "ready" && status !== "failed" && <Clock className="h-3 w-3" />}
																					{name}
																				</span>
																			))}
																		</div>
																	)}
																</div>
															)}

															{/* Removing State */}
															{complianceSetupStatus.status.status === "removing" && (
																<div className="space-y-2">
																	<div className="flex items-center gap-2">
																		<RefreshCw className="h-4 w-4 animate-spin text-warning-600 dark:text-warning-400" />
																		<span className="text-sm font-medium text-warning-700 dark:text-warning-300">
																			Removing Compliance Tools
																		</span>
																	</div>
																	<div className="w-full bg-secondary-200 dark:bg-secondary-700 rounded-full h-1.5">
																		<div className="bg-warning-500 h-1.5 rounded-full animate-pulse" style={{ width: "40%" }} />
																	</div>
																	<p className="text-xs text-secondary-600 dark:text-secondary-400">
																		{complianceSetupStatus.status.message || "Removing OpenSCAP packages..."}
																	</p>
																</div>
															)}

															{/* Ready State */}
															{complianceSetupStatus.status.status === "ready" && (
																<div className="flex items-center gap-2">
																	<CheckCircle2 className="h-4 w-4 text-green-600 dark:text-green-400" />
																	<span className="text-sm font-medium text-green-700 dark:text-green-300">
																		Compliance Tools Ready
																	</span>
																	{complianceSetupStatus.status.components && (
																		<div className="flex gap-1 ml-2">
																			{Object.entries(complianceSetupStatus.status.components)
																				.filter(([, status]) => status !== "unavailable")
																				.map(([name, status]) => (
																				<span
																					key={name}
																					className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs bg-green-100 text-green-700 dark:bg-green-900 dark:text-green-300"
																				>
																					<CheckCircle2 className="h-3 w-3" />
																					{name}
																				</span>
																			))}
																		</div>
																	)}
																</div>
															)}

															{/* Partial State */}
															{complianceSetupStatus.status.status === "partial" && (
																<div className="space-y-2">
																	<div className="flex items-center gap-2">
																		<AlertTriangle className="h-4 w-4 text-warning-600 dark:text-warning-400" />
																		<span className="text-sm font-medium text-warning-700 dark:text-warning-300">
																			Partial Installation
																		</span>
																	</div>
																	<p className="text-xs text-secondary-600 dark:text-secondary-400">
																		{complianceSetupStatus.status.message || "Some components failed to install"}
																	</p>
																	{complianceSetupStatus.status.components && (
																		<div className="flex flex-wrap gap-2">
																			{Object.entries(complianceSetupStatus.status.components)
																				.filter(([, status]) => status !== "unavailable")
																				.map(([name, status]) => (
																				<span
																					key={name}
																					className={`inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs ${
																						status === "ready"
																							? "bg-green-100 text-green-700 dark:bg-green-900 dark:text-green-300"
																							: "bg-red-100 text-red-700 dark:bg-red-900 dark:text-red-300"
																					}`}
																				>
																					{status === "ready" ? <CheckCircle2 className="h-3 w-3" /> : <AlertCircle className="h-3 w-3" />}
																					{name}
																				</span>
																			))}
																		</div>
																	)}
																</div>
															)}

															{/* Error State */}
															{complianceSetupStatus.status.status === "error" && (
																<div className="space-y-2">
																	<div className="flex items-center gap-2">
																		<AlertCircle className="h-4 w-4 text-danger-600 dark:text-danger-400" />
																		<span className="text-sm font-medium text-danger-700 dark:text-danger-300">
																			Installation Failed
																		</span>
																	</div>
																	<p className="text-xs text-danger-600 dark:text-danger-400">
																		{complianceSetupStatus.status.message || "Setup failed - check agent logs"}
																	</p>
																</div>
															)}
														</div>
													)}
												</div>
												<div className="flex-shrink-0">
													<button
														type="button"
														onClick={() =>
															toggleIntegrationMutation.mutate({
																integrationName: "compliance",
																enabled:
																	!integrationsData?.data?.integrations?.compliance,
															})
														}
														disabled={
															toggleIntegrationMutation.isPending ||
															!wsStatus?.connected ||
															complianceSetupStatus?.status?.status === "installing" ||
															complianceSetupStatus?.status?.status === "removing"
														}
														title={
															!wsStatus?.connected
																? "Agent is not connected"
																: complianceSetupStatus?.status?.status === "installing"
																	? "Installation in progress..."
																	: complianceSetupStatus?.status?.status === "removing"
																		? "Removal in progress..."
																		: integrationsData?.data?.integrations?.compliance
																			? "Disable compliance scanning"
																			: "Enable compliance scanning"
														}
														className={`relative inline-flex h-5 w-9 items-center rounded-md transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 ${
															integrationsData?.data?.integrations?.compliance
																? "bg-primary-600 dark:bg-primary-500"
																: "bg-secondary-200 dark:bg-secondary-600"
														} ${
															toggleIntegrationMutation.isPending ||
															!integrationsData?.data?.connected ||
															complianceSetupStatus?.status?.status === "installing" ||
															complianceSetupStatus?.status?.status === "removing"
																? "opacity-50 cursor-not-allowed"
																: ""
														}`}
													>
														<span
															className={`inline-block h-3 w-3 transform rounded-full bg-white transition-transform ${
																integrationsData?.data?.integrations?.compliance
																	? "translate-x-5"
																	: "translate-x-1"
															}`}
														/>
													</button>
												</div>
											</div>
											{!wsStatus?.connected && (
												<p className="text-xs text-warning-600 dark:text-warning-400 mt-2">
													Agent must be connected via WebSocket to toggle
													integrations
												</p>
											)}
										</div>

									</div>
								)}
							</div>
						)}

						{/* Docker Tab */}
						{activeTab === "docker" && (
							<div className="space-y-4">
								{/* Docker Sub-tabs */}
								<div className="flex gap-2 border-b border-secondary-200 dark:border-secondary-600 pb-2">
									<button
										type="button"
										onClick={() => setDockerSubTab("containers")}
										className={`px-3 py-1.5 text-xs font-medium rounded-t ${
											dockerSubTab === "containers"
												? "bg-primary-100 dark:bg-primary-900 text-primary-700 dark:text-primary-300"
												: "text-secondary-500 dark:text-secondary-400 hover:bg-secondary-100 dark:hover:bg-secondary-700"
										}`}
									>
										All Containers
									</button>
									<button
										type="button"
										onClick={() => setDockerSubTab("running")}
										className={`px-3 py-1.5 text-xs font-medium rounded-t ${
											dockerSubTab === "running"
												? "bg-primary-100 dark:bg-primary-900 text-primary-700 dark:text-primary-300"
												: "text-secondary-500 dark:text-secondary-400 hover:bg-secondary-100 dark:hover:bg-secondary-700"
										}`}
									>
										Running
									</button>
									<button
										type="button"
										onClick={() => setDockerSubTab("images")}
										className={`px-3 py-1.5 text-xs font-medium rounded-t ${
											dockerSubTab === "images"
												? "bg-primary-100 dark:bg-primary-900 text-primary-700 dark:text-primary-300"
												: "text-secondary-500 dark:text-secondary-400 hover:bg-secondary-100 dark:hover:bg-secondary-700"
										}`}
									>
										Images
									</button>
									<button
										type="button"
										onClick={() => setDockerSubTab("volumes")}
										className={`px-3 py-1.5 text-xs font-medium rounded-t ${
											dockerSubTab === "volumes"
												? "bg-primary-100 dark:bg-primary-900 text-primary-700 dark:text-primary-300"
												: "text-secondary-500 dark:text-secondary-400 hover:bg-secondary-100 dark:hover:bg-secondary-700"
										}`}
									>
										Volumes
									</button>
								</div>

								{isLoadingDocker ? (
									<div className="flex items-center justify-center h-32">
										<RefreshCw className="h-6 w-6 animate-spin text-primary-600" />
									</div>
								) : !dockerData ? (
									<div className="text-center py-8">
										<Database className="h-12 w-12 text-gray-400 mx-auto mb-4" />
										<p className="text-gray-500 dark:text-gray-400">
											No Docker data available
										</p>
									</div>
								) : (
									<>
										{/* Containers Sub-tab */}
										{dockerSubTab === "containers" && (
											<div className="space-y-3">
												{dockerData.containers?.length === 0 ? (
													<p className="text-secondary-500 dark:text-secondary-400 text-center py-4">
														No containers found
													</p>
												) : (
													dockerData.containers?.map((container) => (
														<div
															key={container.id}
															className="bg-secondary-50 dark:bg-secondary-700 rounded-lg p-3 border border-secondary-200 dark:border-secondary-600"
														>
															<div className="flex items-center justify-between">
																<div className="flex items-center gap-2">
																	<span
																		className={`w-2 h-2 rounded-full ${
																			container.state === "running"
																				? "bg-green-500"
																				: container.state === "exited"
																					? "bg-red-500"
																					: "bg-yellow-500"
																		}`}
																	/>
																	<span className="font-medium text-secondary-900 dark:text-white text-sm">
																		{container.name}
																	</span>
																</div>
																<span className="text-xs text-secondary-500 dark:text-secondary-400">
																	{container.state}
																</span>
															</div>
															<div className="mt-1 text-xs text-secondary-600 dark:text-secondary-300">
																<span className="font-mono">{container.image}</span>
															</div>
														</div>
													))
												)}
											</div>
										)}

										{/* Running Sub-tab */}
										{dockerSubTab === "running" && (
											<div className="space-y-3">
												{dockerData.containers?.filter((c) => c.state === "running").length === 0 ? (
													<p className="text-secondary-500 dark:text-secondary-400 text-center py-4">
														No running containers
													</p>
												) : (
													dockerData.containers
														?.filter((c) => c.state === "running")
														.map((container) => (
															<div
																key={container.id}
																className="bg-secondary-50 dark:bg-secondary-700 rounded-lg p-3 border border-secondary-200 dark:border-secondary-600"
															>
																<div className="flex items-center justify-between">
																	<div className="flex items-center gap-2">
																		<span className="w-2 h-2 rounded-full bg-green-500" />
																		<span className="font-medium text-secondary-900 dark:text-white text-sm">
																			{container.name}
																		</span>
																	</div>
																	<span className="text-xs text-secondary-500 dark:text-secondary-400">
																		{container.status}
																	</span>
																</div>
																<div className="mt-1 text-xs text-secondary-600 dark:text-secondary-300">
																	<span className="font-mono">{container.image}</span>
																</div>
															</div>
														))
												)}
											</div>
										)}

										{/* Images Sub-tab */}
										{dockerSubTab === "images" && (
											<div className="space-y-3">
												{dockerData.images?.length === 0 ? (
													<p className="text-secondary-500 dark:text-secondary-400 text-center py-4">
														No images found
													</p>
												) : (
													dockerData.images?.map((image) => (
														<div
															key={image.id}
															className="bg-secondary-50 dark:bg-secondary-700 rounded-lg p-3 border border-secondary-200 dark:border-secondary-600"
														>
															<div className="flex items-center justify-between">
																<span className="font-medium text-secondary-900 dark:text-white text-sm font-mono">
																	{image.repository}:{image.tag}
																</span>
																<span className="text-xs text-secondary-500 dark:text-secondary-400">
																	{image.size}
																</span>
															</div>
															<div className="mt-1 text-xs text-secondary-600 dark:text-secondary-300">
																ID: {image.id?.slice(0, 12)}
															</div>
														</div>
													))
												)}
											</div>
										)}

										{/* Volumes Sub-tab */}
										{dockerSubTab === "volumes" && (
											<div className="space-y-3">
												{dockerData.volumes?.length === 0 ? (
													<p className="text-secondary-500 dark:text-secondary-400 text-center py-4">
														No volumes found
													</p>
												) : (
													dockerData.volumes?.map((volume) => (
														<div
															key={volume.name}
															className="bg-secondary-50 dark:bg-secondary-700 rounded-lg p-3 border border-secondary-200 dark:border-secondary-600"
														>
															<div className="flex items-center justify-between">
																<span className="font-medium text-secondary-900 dark:text-white text-sm font-mono">
																	{volume.name}
																</span>
																<span className="text-xs text-secondary-500 dark:text-secondary-400">
																	{volume.driver}
																</span>
															</div>
															{volume.mountpoint && (
																<div className="mt-1 text-xs text-secondary-600 dark:text-secondary-300 font-mono truncate">
																	{volume.mountpoint}
																</div>
															)}
														</div>
													))
												)}
											</div>
										)}
									</>
								)}
							</div>
						)}

						{/* Compliance */}
						{activeTab === "compliance" && (
							<ComplianceTab hostId={hostId} apiId={host?.api_id} isConnected={wsStatus?.connected} />
						)}
					</div>
				</div>
			</div>

			{/* Credentials Modal */}
			{showCredentialsModal && (
				<CredentialsModal
					host={host}
					isOpen={showCredentialsModal}
					onClose={() => setShowCredentialsModal(false)}
					plaintextApiKey={plaintextApiKey}
				/>
			)}

			{/* Delete Confirmation Modal */}
			{showDeleteModal && (
				<DeleteConfirmationModal
					host={host}
					isOpen={showDeleteModal}
					onClose={() => setShowDeleteModal(false)}
					onConfirm={handleDeleteHost}
					isLoading={deleteHostMutation.isPending}
				/>
			)}

		</div>
	);
};

// Components moved to separate files in ./hostdetail/

export default HostDetail;
