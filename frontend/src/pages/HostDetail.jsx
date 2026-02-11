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
	Container,
	Cpu,
	Database,
	Download,
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
import { useEffect, useRef, useState } from "react";
import { Link, useLocation, useNavigate, useParams } from "react-router-dom";
import ComplianceTab from "../components/compliance/ComplianceTab";
import InlineEdit from "../components/InlineEdit";
import InlineMultiGroupEdit from "../components/InlineMultiGroupEdit";
import SshTerminal from "../components/SshTerminal";
import {
	adminHostsAPI,
	alertsAPI,
	dashboardAPI,
	formatDate,
	formatRelativeTime,
	hostGroupsAPI,
	repositoryAPI,
	settingsAPI,
} from "../utils/api";
import { complianceAPI } from "../utils/complianceApi";
import { OSIcon } from "../utils/osIcons.jsx";
import AgentQueueTab from "./hostdetail/AgentQueueTab";
import CredentialsModal from "./hostdetail/CredentialsModal";
import DeleteConfirmationModal from "./hostdetail/DeleteConfirmationModal";

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
	const [integrationRefreshMessage, setIntegrationRefreshMessage] = useState({
		text: "",
		isError: false,
	});
	const [dockerRefreshMessage, setDockerRefreshMessage] = useState({
		text: "",
		isError: false,
	});
	const [showAllReports, setShowAllReports] = useState(false);

	// State for auto-update confirmation dialog
	const [autoUpdateDialog, setAutoUpdateDialog] = useState(false);

	// Ref to track component mount state for setTimeout cleanup
	const isMountedRef = useRef(true);
	const timeoutRefs = useRef([]);

	// Cleanup timeouts on unmount
	useEffect(() => {
		isMountedRef.current = true;
		return () => {
			isMountedRef.current = false;
			// Clear all pending timeouts
			timeoutRefs.current.forEach((timeoutId) => {
				clearTimeout(timeoutId);
			});
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
			timeoutRefs.current = timeoutRefs.current.filter(
				(id) => id !== timeoutId,
			);
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

	// Fetch global settings to check if auto-update master toggle is enabled
	// Try public endpoint first (works for all users), fallback to full settings if user has permissions
	const { data: settings } = useQuery({
		queryKey: ["settings"],
		queryFn: async () => {
			try {
				// Try public endpoint first (available to all authenticated users)
				return await settingsAPI.getPublic().then((res) => res.data);
			} catch (error) {
				// If public endpoint fails, try full settings (requires can_manage_settings)
				if (error.response?.status === 403 || error.response?.status === 401) {
					try {
						return await settingsAPI.get().then((res) => res.data);
					} catch (_e) {
						// If both fail, return minimal default
						return { auto_update: false };
					}
				}
				// For other errors, return minimal default
				return { auto_update: false };
			}
		},
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
	const { data: complianceLatest, isLoading: _isLoadingCompliance } = useQuery({
		queryKey: ["compliance-latest-quickview", hostId],
		queryFn: () =>
			complianceAPI
				.getLatestScan(hostId)
				.then((res) => res.data)
				.catch(() => null),
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

	// Toggle agent auto-update mutation (updates PatchMon agent script, not system packages)
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

	// Mutation to enable global auto-update setting
	const enableGlobalAutoUpdateMutation = useMutation({
		mutationFn: () =>
			settingsAPI.update({ autoUpdate: true }).then((res) => res.data),
		onSuccess: () => {
			queryClient.invalidateQueries(["settings"]);
		},
	});

	// Handle auto-update toggle with global setting check
	const handleAutoUpdateToggle = () => {
		// If currently enabled, just disable
		if (host?.auto_update) {
			toggleAutoUpdateMutation.mutate(false);
			return;
		}

		// If enabling and global is OFF, show confirmation dialog
		if (!settings?.auto_update) {
			setAutoUpdateDialog(true);
			return;
		}

		// Global is ON, just enable the host
		toggleAutoUpdateMutation.mutate(true);
	};

	// Handle dialog actions
	const handleEnableBoth = () => {
		// Enable global setting first, then host
		enableGlobalAutoUpdateMutation.mutate(undefined, {
			onSuccess: () => {
				toggleAutoUpdateMutation.mutate(true);
				setAutoUpdateDialog(false);
			},
		});
	};

	const handleEnableHostOnly = () => {
		// Just enable the host (user acknowledges it won't work)
		toggleAutoUpdateMutation.mutate(true);
		setAutoUpdateDialog(false);
	};

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
			const errorMsg =
				error.response?.data?.error || "Failed to send update command";
			const details = error.response?.data?.details;
			setUpdateMessage({
				text: details ? `${errorMsg}: ${details}` : errorMsg,
				jobId: "",
				isError: true,
			});
			safeSetTimeout(
				() => setUpdateMessage({ text: "", jobId: "", isError: false }),
				5000,
			);
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

	// Refresh integration status mutation
	const refreshIntegrationStatusMutation = useMutation({
		mutationFn: () =>
			adminHostsAPI.refreshIntegrationStatus(hostId).then((res) => res.data),
		onSuccess: () => {
			setIntegrationRefreshMessage({
				text: "Integration status refresh requested",
				isError: false,
			});
			// Refetch integrations data after a short delay to allow agent to respond
			safeSetTimeout(() => {
				refetchIntegrations();
				queryClient.invalidateQueries(["compliance-setup-status", hostId]);
			}, 2000);
			safeSetTimeout(
				() => setIntegrationRefreshMessage({ text: "", isError: false }),
				5000,
			);
		},
		onError: (error) => {
			setIntegrationRefreshMessage({
				text:
					error.response?.data?.error || "Failed to refresh integration status",
				isError: true,
			});
			safeSetTimeout(
				() => setIntegrationRefreshMessage({ text: "", isError: false }),
				5000,
			);
		},
	});

	// Refresh Docker inventory mutation
	const refreshDockerMutation = useMutation({
		mutationFn: () =>
			adminHostsAPI.refreshDocker(hostId).then((res) => res.data),
		onSuccess: () => {
			setDockerRefreshMessage({
				text: "Docker refresh requested - data will update shortly",
				isError: false,
			});
			// Refetch Docker data after a short delay to allow agent to respond
			safeSetTimeout(() => {
				refetchDocker();
				queryClient.invalidateQueries(["docker", "host", hostId]);
			}, 3000);
			safeSetTimeout(
				() => setDockerRefreshMessage({ text: "", isError: false }),
				5000,
			);
		},
		onError: (error) => {
			setDockerRefreshMessage({
				text: error.response?.data?.error || "Failed to refresh Docker data",
				isError: true,
			});
			safeSetTimeout(
				() => setDockerRefreshMessage({ text: "", isError: false }),
				5000,
			);
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
	const { data: complianceSetupStatus, refetch: refetchComplianceStatus } =
		useQuery({
			queryKey: ["compliance-setup-status", hostId],
			queryFn: () =>
				adminHostsAPI
					.getIntegrationSetupStatus(hostId, "compliance")
					.then((res) => res.data),
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
		refetch: refetchDocker,
	} = useQuery({
		queryKey: ["docker", "host", hostId],
		queryFn: () =>
			dashboardAPI
				.getHostDetail(hostId, { include: "docker" })
				.then((res) => res.data?.docker),
		staleTime: 30 * 1000,
		refetchOnWindowFocus: false,
		enabled:
			!!hostId &&
			(activeTab === "docker" || integrationsData?.data?.integrations?.docker),
	});

	// Fetch global alert config for host_down
	const { data: hostDownAlertConfig } = useQuery({
		queryKey: ["alert-config", "host_down"],
		queryFn: () =>
			alertsAPI.getAlertConfigByType("host_down").then((res) => res.data.data),
		staleTime: 5 * 60 * 1000, // 5 minutes
		refetchOnWindowFocus: false,
	});

	// Mutation to update host down alerts setting
	const toggleHostDownAlertsMutation = useMutation({
		mutationFn: (enabled) =>
			adminHostsAPI.toggleHostDownAlerts(hostId, enabled),
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ["host", hostId] });
			setUpdateMessage({
				text: "Host down alerts setting updated successfully",
				jobId: "",
			});
			safeSetTimeout(() => {
				if (isMountedRef.current) {
					setUpdateMessage({ text: "", jobId: "" });
				}
			}, 3000);
		},
		onError: (error) => {
			setUpdateMessage({
				text: error.response?.data?.error || "Failed to update setting",
				jobId: "",
			});
			safeSetTimeout(() => {
				if (isMountedRef.current) {
					setUpdateMessage({ text: "", jobId: "" });
				}
			}, 5000);
		},
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
				const updatedData = {
					...oldData,
					data: {
						...oldData.data,
						integrations: {
							...oldData.data.integrations,
							[data.data.integration]: data.data.enabled,
						},
					},
				};
				// Update compliance mode if compliance was toggled
				if (data.data.integration === "compliance") {
					updatedData.compliance_mode =
						data.data.mode || (data.data.enabled ? "enabled" : "disabled");
					updatedData.data.compliance_mode = updatedData.compliance_mode;
				}
				return updatedData;
			});
			// Also invalidate to ensure we get fresh data
			queryClient.invalidateQueries(["host-integrations", hostId]);
			// If compliance was just enabled/disabled, poll for setup status
			if (data.data.integration === "compliance" && data.data.enabled) {
				// Poll multiple times to catch status updates (installation takes ~4-10s)
				const pollTimes = [500, 2000, 4000, 6000, 8000, 10000, 15000];
				pollTimes.forEach((delay) => {
					safeSetTimeout(() => refetchComplianceStatus(), delay);
				});
			}
		},
		onError: (error) => {
			// On error, refetch to get the actual state
			refetchIntegrations();
			// Log error for debugging
			console.error(
				"Failed to toggle integration:",
				error.response?.data?.error || error.message,
			);
		},
	});

	// Set compliance mode mutation (three-state: disabled, on-demand, enabled)
	const setComplianceModeMutation = useMutation({
		mutationFn: (mode) =>
			adminHostsAPI.setComplianceMode(hostId, mode).then((res) => res.data),
		onSuccess: (data) => {
			// Update the cache with the new state
			queryClient.setQueryData(["host-integrations", hostId], (oldData) => {
				if (!oldData) return oldData;
				return {
					...oldData,
					compliance_mode: data.data.mode,
					data: {
						...oldData.data,
						compliance_mode: data.data.mode,
						integrations: {
							...oldData.data.integrations,
							compliance: data.data.mode !== "disabled",
						},
					},
				};
			});
			// Also invalidate to ensure we get fresh data
			queryClient.invalidateQueries(["host-integrations", hostId]);
			// If compliance was just enabled, poll for setup status
			if (data.data.mode === "enabled" || data.data.mode === "on-demand") {
				const pollTimes = [500, 2000, 4000, 6000, 8000, 10000, 15000];
				pollTimes.forEach((delay) => {
					safeSetTimeout(() => refetchComplianceStatus(), delay);
				});
			}
		},
		onError: (error) => {
			// On error, refetch to get the actual state
			refetchIntegrations();
			console.error(
				"Failed to set compliance mode:",
				error.response?.data?.error || error.message,
			);
		},
	});

	// Legacy: Toggle compliance on-demand-only mode mutation (kept for backward compatibility)
	const _toggleComplianceOnDemandOnlyMutation = useMutation({
		mutationFn: (onDemandOnly) =>
			adminHostsAPI
				.setComplianceOnDemandOnly(hostId, onDemandOnly)
				.then((res) => res.data),
		onSuccess: (data) => {
			// Update the cache with the new state
			queryClient.setQueryData(["host-integrations", hostId], (oldData) => {
				if (!oldData) return oldData;
				return {
					...oldData,
					compliance_on_demand_only: data.data.on_demand_only,
					compliance_mode:
						data.data.mode ||
						(data.data.on_demand_only ? "on-demand" : "enabled"),
				};
			});
			// Also invalidate to ensure we get fresh data
			queryClient.invalidateQueries(["host-integrations", hostId]);
		},
		onError: (error) => {
			// On error, refetch to get the actual state
			refetchIntegrations();
			console.error(
				"Failed to toggle compliance on-demand-only:",
				error.response?.data?.error || error.message,
			);
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
			{integrationsData?.data?.integrations?.compliance &&
				complianceLatest &&
				complianceLatest.score !== undefined && (
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
											{complianceLatest.compliance_profiles?.name ||
												"Security Profile"}
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
										<span className="text-secondary-700 dark:text-secondary-300">
											{complianceLatest.passed || 0}
										</span>
									</div>
									<div className="flex items-center gap-1.5">
										<X className="h-4 w-4 text-red-500" />
										<span className="text-secondary-700 dark:text-secondary-300">
											{complianceLatest.failed || 0}
										</span>
									</div>
									<div className="flex items-center gap-1.5">
										<AlertTriangle className="h-4 w-4 text-yellow-500" />
										<span className="text-secondary-700 dark:text-secondary-300">
											{complianceLatest.warnings || 0}
										</span>
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
									<div className="flex items-center gap-2">
										<button
											type="button"
											onClick={handleAutoUpdateToggle}
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
										{/* Warning badge when global auto-update is disabled */}
										{!settings?.auto_update && host.auto_update && (
											<span
												className="text-amber-500 dark:text-amber-400"
												title="Global auto-updates disabled in Settings → Agent Updates"
											>
												<AlertTriangle className="h-4 w-4" />
											</span>
										)}
									</div>
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
											<div className="grid grid-cols-1 md:grid-cols-2 gap-4">
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
													<div className="space-y-3 max-h-80 overflow-y-auto pr-2">
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

					{/* Reporting Card */}
					<div className="card p-4">
						<h3 className="text-lg font-semibold text-secondary-900 dark:text-white mb-4 flex items-center gap-2">
							<AlertTriangle className="h-5 w-5 text-primary-600" />
							Reporting
						</h3>
						<div className="space-y-4">
							<p className="text-xs text-secondary-600 dark:text-secondary-300">
								Control whether this host triggers alert entries when it goes
								offline. When disabled, no alerts will be created for this host
								even if the global setting is enabled.
							</p>

							{/* Settings - Side by Side on larger mobile, stacked on small */}
							<div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
								{/* Current Setting */}
								<div>
									<label className="text-xs font-medium text-secondary-500 dark:text-secondary-400 mb-2 block">
										Current Setting
									</label>
									<div className="text-sm text-secondary-900 dark:text-white">
										{host?.host_down_alerts_enabled === null ? (
											<span className="inline-flex items-center px-2 py-1 rounded bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200">
												Inherit from global settings
											</span>
										) : host?.host_down_alerts_enabled === true ? (
											<span className="inline-flex items-center px-2 py-1 rounded bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">
												Enabled
											</span>
										) : (
											<span className="inline-flex items-center px-2 py-1 rounded bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200">
												Disabled
											</span>
										)}
									</div>
								</div>

								{/* Global Setting Reference */}
								{hostDownAlertConfig && (
									<div>
										<label className="text-xs font-medium text-secondary-500 dark:text-secondary-400 mb-2 block">
											Global Setting
										</label>
										<div className="text-sm text-secondary-600 dark:text-secondary-300">
											{hostDownAlertConfig.is_enabled ? (
												<span className="inline-flex items-center px-2 py-1 rounded bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">
													Enabled
												</span>
											) : (
												<span className="inline-flex items-center px-2 py-1 rounded bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200">
													Disabled
												</span>
											)}
											{host?.host_down_alerts_enabled === null && (
												<span className="ml-2 text-xs text-secondary-500 dark:text-secondary-400 block mt-1">
													(currently inherited)
												</span>
											)}
										</div>
									</div>
								)}
							</div>

							{/* Action Buttons */}
							<div className="flex flex-wrap gap-2">
								<button
									type="button"
									onClick={() => toggleHostDownAlertsMutation.mutate(null)}
									disabled={
										toggleHostDownAlertsMutation.isPending ||
										host?.host_down_alerts_enabled === null
									}
									className={`px-3 py-1.5 text-sm font-medium rounded transition-colors ${
										host?.host_down_alerts_enabled === null
											? "bg-primary-600 text-white"
											: "bg-secondary-200 dark:bg-secondary-600 text-secondary-700 dark:text-secondary-300 hover:bg-secondary-300 dark:hover:bg-secondary-500"
									} disabled:opacity-50 disabled:cursor-not-allowed`}
								>
									Inherit
								</button>
								<button
									type="button"
									onClick={() => toggleHostDownAlertsMutation.mutate(true)}
									disabled={
										toggleHostDownAlertsMutation.isPending ||
										host?.host_down_alerts_enabled === true
									}
									className={`px-3 py-1.5 text-sm font-medium rounded transition-colors ${
										host?.host_down_alerts_enabled === true
											? "bg-green-600 text-white"
											: "bg-secondary-200 dark:bg-secondary-600 text-secondary-700 dark:text-secondary-300 hover:bg-secondary-300 dark:hover:bg-secondary-500"
									} disabled:opacity-50 disabled:cursor-not-allowed`}
								>
									Enable
								</button>
								<button
									type="button"
									onClick={() => toggleHostDownAlertsMutation.mutate(false)}
									disabled={
										toggleHostDownAlertsMutation.isPending ||
										host?.host_down_alerts_enabled === false
									}
									className={`px-3 py-1.5 text-sm font-medium rounded transition-colors ${
										host?.host_down_alerts_enabled === false
											? "bg-red-600 text-white"
											: "bg-secondary-200 dark:bg-secondary-600 text-secondary-700 dark:text-secondary-300 hover:bg-secondary-300 dark:hover:bg-secondary-500"
									} disabled:opacity-50 disabled:cursor-not-allowed`}
								>
									Disable
								</button>
							</div>

							{/* Success/Error Message */}
							{updateMessage.text && (
								<div
									className={`text-sm ${
										updateMessage.text.includes("successfully")
											? "text-green-600 dark:text-green-400"
											: "text-red-600 dark:text-red-400"
									}`}
								>
									{updateMessage.text}
								</div>
							)}
						</div>
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
						<button
							type="button"
							onClick={() => handleTabChange("reporting")}
							className={`px-4 py-2 text-sm font-medium ${
								activeTab === "reporting"
									? "text-primary-600 dark:text-primary-400 border-b-2 border-primary-500"
									: "text-secondary-500 dark:text-secondary-400 hover:text-secondary-700 dark:hover:text-secondary-300"
							}`}
						>
							Reporting
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
												if (
													value.trim() &&
													!/^(\d{1,3}\.){3}\d{1,3}$/.test(value.trim())
												) {
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
													updateConnectionMutation.mutate({
														hostname: newHostname.trim(),
													});
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
										<div className="flex items-center gap-2">
											<button
												type="button"
												onClick={handleAutoUpdateToggle}
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
											{/* Warning badge when global auto-update is disabled */}
											{!settings?.auto_update && host.auto_update && (
												<span
													className="text-amber-500 dark:text-amber-400"
													title="Global auto-updates disabled in Settings → Agent Updates"
												>
													<AlertTriangle className="h-4 w-4" />
												</span>
											)}
										</div>
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
												<div className="grid grid-cols-1 md:grid-cols-2 gap-4">
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
													<div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3 max-h-80 overflow-y-auto pr-2">
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
								{/* Header with refresh button */}
								<div className="flex items-center justify-between">
									<div className="flex items-center gap-2">
										{integrationRefreshMessage.text && (
											<span
												className={`text-sm ${integrationRefreshMessage.isError ? "text-red-600 dark:text-red-400" : "text-green-600 dark:text-green-400"}`}
											>
												{integrationRefreshMessage.text}
											</span>
										)}
									</div>
									<button
										type="button"
										onClick={() => refreshIntegrationStatusMutation.mutate()}
										disabled={
											refreshIntegrationStatusMutation.isPending ||
											!wsStatus?.connected
										}
										title={
											wsStatus?.connected
												? "Refresh integration status from agent"
												: "Agent is not connected"
										}
										className="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium text-secondary-700 dark:text-secondary-200 bg-secondary-100 dark:bg-secondary-700 hover:bg-secondary-200 dark:hover:bg-secondary-600 rounded-md transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
									>
										<RefreshCw
											className={`h-4 w-4 ${refreshIntegrationStatusMutation.isPending ? "animate-spin" : ""}`}
										/>
										{refreshIntegrationStatusMutation.isPending
											? "Refreshing..."
											: "Refresh Status"}
									</button>
								</div>
								{isLoadingIntegrations ? (
									<div className="flex items-center justify-center h-32">
										<RefreshCw className="h-6 w-6 animate-spin text-primary-600" />
									</div>
								) : (
									<div className="grid grid-cols-1 gap-4">
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
														{integrationsData?.data?.integrations
															?.compliance ? (
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
														Provides security posture assessment and remediation
														recommendations.
													</p>

													{/* Setup Status Display - hide when status is "disabled" */}
													{((complianceSetupStatus?.status?.status &&
														complianceSetupStatus?.status?.status !==
															"disabled") ||
														(!complianceSetupStatus?.status?.status &&
															integrationsData?.data?.integrations
																?.compliance)) && (
														<div className="mt-3 p-3 rounded-lg border bg-secondary-100 dark:bg-secondary-800 border-secondary-300 dark:border-secondary-600">
															{/* Installing State */}
															{complianceSetupStatus?.status?.status ===
																"installing" && (
																<div className="space-y-2">
																	<div className="flex items-center gap-2">
																		<RefreshCw className="h-4 w-4 animate-spin text-primary-600 dark:text-primary-400" />
																		<span className="text-sm font-medium text-primary-700 dark:text-primary-300">
																			Installing Compliance Tools
																		</span>
																	</div>
																	<div className="w-full bg-secondary-200 dark:bg-secondary-700 rounded-full h-1.5">
																		<div
																			className="bg-primary-600 h-1.5 rounded-full animate-pulse"
																			style={{ width: "60%" }}
																		/>
																	</div>
																	<p className="text-xs text-secondary-600 dark:text-secondary-400">
																		{complianceSetupStatus.status.message ||
																			"Installing OpenSCAP packages and security content..."}
																	</p>
																	{complianceSetupStatus.status.components && (
																		<div className="flex flex-wrap gap-2 mt-2">
																			{Object.entries(
																				complianceSetupStatus.status.components,
																			)
																				.filter(
																					([, status]) =>
																						status !== "unavailable",
																				)
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
																						{status === "ready" && (
																							<CheckCircle2 className="h-3 w-3" />
																						)}
																						{status === "failed" && (
																							<AlertCircle className="h-3 w-3" />
																						)}
																						{status !== "ready" &&
																							status !== "failed" && (
																								<Clock className="h-3 w-3" />
																							)}
																						{name}
																					</span>
																				))}
																		</div>
																	)}
																</div>
															)}

															{/* Removing State */}
															{complianceSetupStatus?.status?.status ===
																"removing" && (
																<div className="space-y-2">
																	<div className="flex items-center gap-2">
																		<RefreshCw className="h-4 w-4 animate-spin text-warning-600 dark:text-warning-400" />
																		<span className="text-sm font-medium text-warning-700 dark:text-warning-300">
																			Removing Compliance Tools
																		</span>
																	</div>
																	<div className="w-full bg-secondary-200 dark:bg-secondary-700 rounded-full h-1.5">
																		<div
																			className="bg-warning-500 h-1.5 rounded-full animate-pulse"
																			style={{ width: "40%" }}
																		/>
																	</div>
																	<p className="text-xs text-secondary-600 dark:text-secondary-400">
																		{complianceSetupStatus.status.message ||
																			"Removing OpenSCAP packages..."}
																	</p>
																</div>
															)}

															{/* Ready State */}
															{complianceSetupStatus?.status?.status ===
																"ready" && (
																<div className="flex items-center gap-2">
																	<CheckCircle2 className="h-4 w-4 text-green-600 dark:text-green-400" />
																	<span className="text-sm font-medium text-green-700 dark:text-green-300">
																		Compliance Tools Ready
																	</span>
																	{complianceSetupStatus.status.components && (
																		<div className="flex gap-1 ml-2">
																			{Object.entries(
																				complianceSetupStatus.status.components,
																			)
																				.filter(
																					([, status]) =>
																						status !== "unavailable",
																				)
																				.map(([name, _status]) => (
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
															{complianceSetupStatus?.status?.status ===
																"partial" && (
																<div className="space-y-2">
																	<div className="flex items-center gap-2">
																		<AlertTriangle className="h-4 w-4 text-warning-600 dark:text-warning-400" />
																		<span className="text-sm font-medium text-warning-700 dark:text-warning-300">
																			Partial Installation
																		</span>
																	</div>
																	<p className="text-xs text-secondary-600 dark:text-secondary-400">
																		{complianceSetupStatus.status.message ||
																			"Some components failed to install"}
																	</p>
																	{complianceSetupStatus.status.components && (
																		<div className="flex flex-wrap gap-2">
																			{Object.entries(
																				complianceSetupStatus.status.components,
																			)
																				.filter(
																					([, status]) =>
																						status !== "unavailable",
																				)
																				.map(([name, status]) => (
																					<span
																						key={name}
																						className={`inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs ${
																							status === "ready"
																								? "bg-green-100 text-green-700 dark:bg-green-900 dark:text-green-300"
																								: "bg-red-100 text-red-700 dark:bg-red-900 dark:text-red-300"
																						}`}
																					>
																						{status === "ready" ? (
																							<CheckCircle2 className="h-3 w-3" />
																						) : (
																							<AlertCircle className="h-3 w-3" />
																						)}
																						{name}
																					</span>
																				))}
																		</div>
																	)}
																</div>
															)}

															{/* Error State */}
															{complianceSetupStatus?.status?.status ===
																"error" && (
																<div className="space-y-2">
																	<div className="flex items-center gap-2">
																		<AlertCircle className="h-4 w-4 text-danger-600 dark:text-danger-400" />
																		<span className="text-sm font-medium text-danger-700 dark:text-danger-300">
																			Installation Failed
																		</span>
																	</div>
																	<p className="text-xs text-danger-600 dark:text-danger-400">
																		{complianceSetupStatus?.status?.message ||
																			"Setup failed - check agent logs"}
																	</p>
																</div>
															)}

															{/* Fallback: Compliance enabled but no status in cache - assume ready */}
															{!complianceSetupStatus?.status?.status &&
																integrationsData?.data?.integrations
																	?.compliance && (
																	<div className="flex items-center gap-2">
																		<CheckCircle2 className="h-4 w-4 text-green-600 dark:text-green-400" />
																		<span className="text-sm font-medium text-green-700 dark:text-green-300">
																			Compliance Tools Ready
																		</span>
																	</div>
																)}
														</div>
													)}
												</div>
												<div className="flex-shrink-0">
													{/* Three-state compliance mode selector - small inline */}
													{(() => {
														const currentMode =
															integrationsData?.data?.compliance_mode ||
															integrationsData?.compliance_mode ||
															(integrationsData?.data?.integrations?.compliance
																? integrationsData?.data
																		?.compliance_on_demand_only ||
																	integrationsData?.compliance_on_demand_only
																	? "on-demand"
																	: "enabled"
																: "disabled");
														const isDisabled =
															setComplianceModeMutation.isPending ||
															!wsStatus?.connected ||
															complianceSetupStatus?.status?.status ===
																"installing" ||
															complianceSetupStatus?.status?.status ===
																"removing";

														return (
															<div className="flex flex-col gap-0.5 bg-secondary-100 dark:bg-secondary-800 rounded-md p-0.5">
																<button
																	type="button"
																	onClick={() =>
																		setComplianceModeMutation.mutate("disabled")
																	}
																	disabled={isDisabled}
																	title={
																		isDisabled
																			? "Agent is not connected or operation in progress"
																			: "Disable compliance scanning"
																	}
																	className={`w-full px-2 py-1 text-xs font-medium rounded transition-colors ${
																		currentMode === "disabled"
																			? "bg-white dark:bg-secondary-700 text-secondary-900 dark:text-secondary-100 shadow-sm"
																			: "text-secondary-600 dark:text-secondary-400 hover:text-secondary-900 dark:hover:text-secondary-100"
																	} ${
																		isDisabled
																			? "opacity-50 cursor-not-allowed"
																			: "cursor-pointer"
																	}`}
																>
																	Disabled
																</button>
																<button
																	type="button"
																	onClick={() =>
																		setComplianceModeMutation.mutate(
																			"on-demand",
																		)
																	}
																	disabled={isDisabled}
																	title={
																		isDisabled
																			? "Agent is not connected or operation in progress"
																			: "Enable compliance scanning (on-demand only - runs when triggered from UI)"
																	}
																	className={`w-full px-2 py-1 text-xs font-medium rounded transition-colors ${
																		currentMode === "on-demand"
																			? "bg-white dark:bg-secondary-700 text-secondary-900 dark:text-secondary-100 shadow-sm"
																			: "text-secondary-600 dark:text-secondary-400 hover:text-secondary-900 dark:hover:text-secondary-100"
																	} ${
																		isDisabled
																			? "opacity-50 cursor-not-allowed"
																			: "cursor-pointer"
																	}`}
																>
																	On-Demand
																</button>
																<button
																	type="button"
																	onClick={() =>
																		setComplianceModeMutation.mutate("enabled")
																	}
																	disabled={isDisabled}
																	title={
																		isDisabled
																			? "Agent is not connected or operation in progress"
																			: "Enable compliance scanning with automatic scheduled scans"
																	}
																	className={`w-full px-2 py-1 text-xs font-medium rounded transition-colors ${
																		currentMode === "enabled"
																			? "bg-white dark:bg-secondary-700 text-secondary-900 dark:text-secondary-100 shadow-sm"
																			: "text-secondary-600 dark:text-secondary-400 hover:text-secondary-900 dark:hover:text-secondary-100"
																	} ${
																		isDisabled
																			? "opacity-50 cursor-not-allowed"
																			: "cursor-pointer"
																	}`}
																>
																	Enabled
																</button>
															</div>
														);
													})()}
												</div>
											</div>
											{!wsStatus?.connected && (
												<p className="text-xs text-warning-600 dark:text-warning-400 mt-2">
													Agent must be connected via WebSocket to change
													compliance settings
												</p>
											)}
											{/* Mode description */}
											{(() => {
												const currentMode =
													integrationsData?.data?.compliance_mode ||
													integrationsData?.compliance_mode ||
													(integrationsData?.data?.integrations?.compliance
														? integrationsData?.data
																?.compliance_on_demand_only ||
															integrationsData?.compliance_on_demand_only
															? "on-demand"
															: "enabled"
														: "disabled");
												const modeDescriptions = {
													disabled:
														"Compliance scanning is disabled. No scans will run.",
													"on-demand":
														"Compliance scans only run when manually triggered from the UI, not during scheduled reports.",
													enabled:
														"Compliance scanning is enabled with automatic scheduled scans during regular reports.",
												};
												return (
													<p className="text-xs text-secondary-500 dark:text-secondary-400 mt-2">
														{modeDescriptions[currentMode] ||
															modeDescriptions.disabled}
													</p>
												);
											})()}
										</div>
									</div>
								)}
							</div>
						)}

						{/* Docker Tab */}
						{activeTab === "docker" && (
							<div className="space-y-4">
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
										{/* Docker Sub-tabs with Refresh Button */}
										{(() => {
											// Calculate stacks for tab count
											const stacksSet = new Set();
											dockerData.containers?.forEach((c) => {
												const project =
													c.labels?.["com.docker.compose.project"];
												if (project) stacksSet.add(project);
											});
											const stackCount = stacksSet.size;

											return (
												<div className="flex items-center justify-between gap-2 border-b border-secondary-200 dark:border-secondary-600 pb-2 flex-wrap">
													<div className="flex gap-2 flex-wrap">
														<button
															type="button"
															onClick={() => setDockerSubTab("stacks")}
															className={`px-3 py-1.5 text-xs font-medium rounded-t flex items-center gap-1.5 ${
																dockerSubTab === "stacks"
																	? "bg-primary-100 dark:bg-primary-900 text-primary-700 dark:text-primary-300"
																	: "text-secondary-500 dark:text-secondary-400 hover:bg-secondary-100 dark:hover:bg-secondary-700"
															}`}
														>
															Stacks
															<span className="px-1.5 py-0.5 text-xs rounded bg-orange-100 dark:bg-orange-900/30 text-orange-700 dark:text-orange-400">
																{stackCount}
															</span>
														</button>
														<button
															type="button"
															onClick={() => setDockerSubTab("containers")}
															className={`px-3 py-1.5 text-xs font-medium rounded-t flex items-center gap-1.5 ${
																dockerSubTab === "containers"
																	? "bg-primary-100 dark:bg-primary-900 text-primary-700 dark:text-primary-300"
																	: "text-secondary-500 dark:text-secondary-400 hover:bg-secondary-100 dark:hover:bg-secondary-700"
															}`}
														>
															Containers
															<span className="px-1.5 py-0.5 text-xs rounded bg-secondary-200 dark:bg-secondary-600">
																{dockerData.containers?.length || 0}
															</span>
														</button>
														<button
															type="button"
															onClick={() => setDockerSubTab("images")}
															className={`px-3 py-1.5 text-xs font-medium rounded-t flex items-center gap-1.5 ${
																dockerSubTab === "images"
																	? "bg-primary-100 dark:bg-primary-900 text-primary-700 dark:text-primary-300"
																	: "text-secondary-500 dark:text-secondary-400 hover:bg-secondary-100 dark:hover:bg-secondary-700"
															}`}
														>
															Images
															<span className="px-1.5 py-0.5 text-xs rounded bg-secondary-200 dark:bg-secondary-600">
																{dockerData.images?.length || 0}
															</span>
														</button>
														<button
															type="button"
															onClick={() => setDockerSubTab("volumes")}
															className={`px-3 py-1.5 text-xs font-medium rounded-t flex items-center gap-1.5 ${
																dockerSubTab === "volumes"
																	? "bg-primary-100 dark:bg-primary-900 text-primary-700 dark:text-primary-300"
																	: "text-secondary-500 dark:text-secondary-400 hover:bg-secondary-100 dark:hover:bg-secondary-700"
															}`}
														>
															Volumes
															<span className="px-1.5 py-0.5 text-xs rounded bg-secondary-200 dark:bg-secondary-600">
																{dockerData.volumes?.length || 0}
															</span>
														</button>
														<button
															type="button"
															onClick={() => setDockerSubTab("networks")}
															className={`px-3 py-1.5 text-xs font-medium rounded-t flex items-center gap-1.5 ${
																dockerSubTab === "networks"
																	? "bg-primary-100 dark:bg-primary-900 text-primary-700 dark:text-primary-300"
																	: "text-secondary-500 dark:text-secondary-400 hover:bg-secondary-100 dark:hover:bg-secondary-700"
															}`}
														>
															Networks
															<span className="px-1.5 py-0.5 text-xs rounded bg-secondary-200 dark:bg-secondary-600">
																{dockerData.networks?.length || 0}
															</span>
														</button>
													</div>
													<div className="flex items-center gap-2">
														{dockerRefreshMessage.text && (
															<span
																className={`text-xs ${dockerRefreshMessage.isError ? "text-red-600" : "text-green-600"}`}
															>
																{dockerRefreshMessage.text}
															</span>
														)}
														<button
															onClick={() => refreshDockerMutation.mutate()}
															disabled={refreshDockerMutation.isPending}
															className="flex items-center gap-1.5 px-2 py-1.5 text-xs font-medium text-primary-600 dark:text-primary-400 bg-primary-50 dark:bg-primary-900/20 hover:bg-primary-100 dark:hover:bg-primary-900/30 rounded border border-primary-200 dark:border-primary-800 transition-colors disabled:opacity-50"
															title="Re-sync Docker data from agent"
														>
															<RefreshCw
																className={`h-3.5 w-3.5 ${refreshDockerMutation.isPending ? "animate-spin" : ""}`}
															/>
															Re-sync
														</button>
													</div>
												</div>
											);
										})()}

										{/* Stacks Sub-tab */}
										{dockerSubTab === "stacks" && (
											<div className="space-y-4">
												{(() => {
													// Group containers by compose project
													const stacksMap = new Map();
													const standaloneContainers = [];

													dockerData.containers?.forEach((container) => {
														const project =
															container.labels?.["com.docker.compose.project"];
														if (project) {
															if (!stacksMap.has(project)) {
																stacksMap.set(project, []);
															}
															stacksMap.get(project).push(container);
														} else {
															standaloneContainers.push(container);
														}
													});

													const stacks = Array.from(stacksMap.entries()).sort(
														(a, b) => a[0].localeCompare(b[0]),
													);

													if (stacks.length === 0) {
														return (
															<div className="text-center py-8">
																<Server className="h-12 w-12 text-secondary-400 mx-auto mb-3" />
																<p className="text-secondary-500 dark:text-secondary-400">
																	No Docker Compose stacks found
																</p>
																<p className="text-xs text-secondary-400 mt-1">
																	Containers started with docker-compose will
																	appear here
																</p>
															</div>
														);
													}

													return (
														<div className="space-y-4">
															{stacks.map(([stackName, containers]) => {
																const runningCount = containers.filter(
																	(c) => c.state === "running",
																).length;
																const totalCount = containers.length;
																const allRunning = runningCount === totalCount;

																return (
																	<div
																		key={stackName}
																		className="bg-secondary-50 dark:bg-secondary-700/30 rounded-lg border border-secondary-200 dark:border-secondary-600 overflow-hidden"
																	>
																		{/* Stack Header */}
																		<div className="px-4 py-3 bg-secondary-100 dark:bg-secondary-700/50 border-b border-secondary-200 dark:border-secondary-600">
																			<div className="flex items-center justify-between">
																				<div className="flex items-center gap-3">
																					<div
																						className={`w-3 h-3 rounded-full ${allRunning ? "bg-green-500" : "bg-yellow-500"}`}
																					/>
																					<h4 className="font-medium text-secondary-900 dark:text-white">
																						{stackName}
																					</h4>
																				</div>
																				<span
																					className={`text-xs px-2 py-1 rounded-full ${
																						allRunning
																							? "bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-400"
																							: "bg-yellow-100 dark:bg-yellow-900/30 text-yellow-700 dark:text-yellow-400"
																					}`}
																				>
																					{runningCount}/{totalCount} running
																				</span>
																			</div>
																		</div>
																		{/* Stack Containers */}
																		<div className="divide-y divide-secondary-200 dark:divide-secondary-600">
																			{containers.map((container) => (
																				<div
																					key={container.id}
																					className="px-4 py-2 flex items-center justify-between hover:bg-secondary-100 dark:hover:bg-secondary-700/50"
																				>
																					<div className="flex items-center gap-3">
																						<span
																							className={`w-2 h-2 rounded-full ${
																								container.state === "running"
																									? "bg-green-500"
																									: container.state === "exited"
																										? "bg-red-500"
																										: "bg-yellow-500"
																							}`}
																						/>
																						<div>
																							<p className="text-sm font-medium">
																								<Link
																									to={`/docker/containers/${container.id}`}
																									className="text-primary-600 dark:text-primary-400 hover:text-primary-900 dark:hover:text-primary-300"
																								>
																									{container.labels?.[
																										"com.docker.compose.service"
																									] || container.name}
																								</Link>
																							</p>
																							<p className="text-xs text-secondary-500 dark:text-secondary-400 font-mono">
																								{container.image}
																							</p>
																						</div>
																					</div>
																					<span
																						className={`text-xs px-2 py-0.5 rounded ${
																							container.state === "running"
																								? "bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-400"
																								: "bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-400"
																						}`}
																					>
																						{container.state}
																					</span>
																				</div>
																			))}
																		</div>
																	</div>
																);
															})}

															{/* Standalone containers section */}
															{standaloneContainers.length > 0 && (
																<div className="mt-4 pt-4 border-t border-secondary-200 dark:border-secondary-600">
																	<h4 className="text-sm font-medium text-secondary-600 dark:text-secondary-400 mb-3">
																		Standalone Containers (
																		{standaloneContainers.length})
																	</h4>
																	<div className="space-y-2">
																		{standaloneContainers.map((container) => (
																			<div
																				key={container.id}
																				className="flex items-center justify-between px-3 py-2 bg-secondary-100 dark:bg-secondary-700/30 rounded-lg"
																			>
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
																					<Link
																						to={`/docker/containers/${container.id}`}
																						className="text-sm text-primary-600 dark:text-primary-400 hover:text-primary-900 dark:hover:text-primary-300"
																					>
																						{container.name}
																					</Link>
																				</div>
																				<span className="text-xs text-secondary-500 dark:text-secondary-400 font-mono">
																					{container.image}
																				</span>
																			</div>
																		))}
																	</div>
																</div>
															)}
														</div>
													);
												})()}
											</div>
										)}

										{/* Containers Sub-tab */}
										{dockerSubTab === "containers" && (
											<div className="space-y-2">
												{dockerData.containers?.length === 0 ? (
													<p className="text-secondary-500 dark:text-secondary-400 text-center py-4">
														No containers found
													</p>
												) : (
													<div className="overflow-x-auto">
														<table className="w-full text-sm">
															<thead>
																<tr className="text-left text-xs text-secondary-500 dark:text-secondary-400 border-b border-secondary-200 dark:border-secondary-600">
																	<th className="pb-2 font-medium">Status</th>
																	<th className="pb-2 font-medium">Name</th>
																	<th className="pb-2 font-medium">Image</th>
																	<th className="pb-2 font-medium">Ports</th>
																	<th className="pb-2 font-medium text-right">
																		Uptime
																	</th>
																</tr>
															</thead>
															<tbody className="divide-y divide-secondary-100 dark:divide-secondary-700">
																{dockerData.containers?.map((container) => (
																	<tr
																		key={container.id}
																		className="hover:bg-secondary-50 dark:hover:bg-secondary-700/50"
																	>
																		<td className="py-2">
																			<span
																				className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-medium ${
																					container.state === "running"
																						? "bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-400"
																						: container.state === "exited"
																							? "bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-400"
																							: "bg-yellow-100 dark:bg-yellow-900/30 text-yellow-700 dark:text-yellow-400"
																				}`}
																			>
																				<span
																					className={`w-1.5 h-1.5 rounded-full ${
																						container.state === "running"
																							? "bg-green-500"
																							: container.state === "exited"
																								? "bg-red-500"
																								: "bg-yellow-500"
																					}`}
																				/>
																				{container.state}
																			</span>
																		</td>
																		<td className="py-2 font-medium">
																			<Link
																				to={`/docker/containers/${container.id}`}
																				className="text-primary-600 dark:text-primary-400 hover:text-primary-900 dark:hover:text-primary-300"
																			>
																				{container.name}
																			</Link>
																		</td>
																		<td
																			className="py-2 font-mono text-xs text-secondary-600 dark:text-secondary-300 max-w-[200px] truncate"
																			title={container.image}
																		>
																			{container.image}
																		</td>
																		<td className="py-2 text-xs text-secondary-500 dark:text-secondary-400">
																			{container.ports &&
																			Object.keys(container.ports).length >
																				0 ? (
																				<div className="flex flex-wrap gap-1">
																					{Object.entries(container.ports)
																						.slice(0, 3)
																						.map(([portKey, portValue]) => {
																							// portKey is like "80/tcp" (private port), portValue is like "0.0.0.0:8080" (public binding)
																							// Format: "0.0.0.0:8080->80/tcp" or just "80/tcp" if no public port
																							const portStr = portValue
																								? `${portValue}->${portKey}`
																								: portKey;
																							return (
																								<span
																									key={portKey}
																									className="px-1.5 py-0.5 bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-400 rounded text-xs font-mono"
																									title={portStr}
																								>
																									{portStr}
																								</span>
																							);
																						})}
																					{Object.keys(container.ports).length >
																						3 && (
																						<span className="text-secondary-400">
																							+
																							{Object.keys(container.ports)
																								.length - 3}
																						</span>
																					)}
																				</div>
																			) : (
																				<span className="text-secondary-400">
																					-
																				</span>
																			)}
																		</td>
																		<td className="py-2 text-xs text-secondary-500 dark:text-secondary-400 text-right">
																			{container.status || "-"}
																		</td>
																	</tr>
																))}
															</tbody>
														</table>
													</div>
												)}
											</div>
										)}

										{/* Images Sub-tab */}
										{dockerSubTab === "images" && (
											<div className="space-y-2">
												{dockerData.images?.length === 0 ? (
													<p className="text-secondary-500 dark:text-secondary-400 text-center py-4">
														No images found
													</p>
												) : (
													<div className="overflow-x-auto">
														<table className="w-full text-sm">
															<thead>
																<tr className="text-left text-xs text-secondary-500 dark:text-secondary-400 border-b border-secondary-200 dark:border-secondary-600">
																	<th className="pb-2 font-medium">
																		Repository
																	</th>
																	<th className="pb-2 font-medium">Tag</th>
																	<th className="pb-2 font-medium">ID</th>
																	<th className="pb-2 font-medium text-right">
																		Size
																	</th>
																</tr>
															</thead>
															<tbody className="divide-y divide-secondary-100 dark:divide-secondary-700">
																{dockerData.images?.map((image) => (
																	<tr
																		key={image.id}
																		className="hover:bg-secondary-50 dark:hover:bg-secondary-700/50"
																	>
																		<td
																			className="py-2 font-mono max-w-[200px] truncate"
																			title={image.repository}
																		>
																			<Link
																				to={`/docker/images/${image.id}`}
																				className="text-primary-600 dark:text-primary-400 hover:text-primary-900 dark:hover:text-primary-300"
																			>
																				{image.repository || "<none>"}
																			</Link>
																		</td>
																		<td className="py-2">
																			<span className="px-2 py-0.5 bg-purple-100 dark:bg-purple-900/30 text-purple-700 dark:text-purple-400 rounded text-xs font-mono">
																				{image.tag || "latest"}
																			</span>
																		</td>
																		<td className="py-2 text-xs font-mono text-secondary-500 dark:text-secondary-400">
																			{image.id?.slice(7, 19) || "-"}
																		</td>
																		<td className="py-2 text-xs text-secondary-500 dark:text-secondary-400 text-right">
																			{image.size || "-"}
																		</td>
																	</tr>
																))}
															</tbody>
														</table>
													</div>
												)}
											</div>
										)}

										{/* Volumes Sub-tab */}
										{dockerSubTab === "volumes" && (
											<div className="space-y-2">
												{dockerData.volumes?.length === 0 ? (
													<p className="text-secondary-500 dark:text-secondary-400 text-center py-4">
														No volumes found
													</p>
												) : (
													<div className="overflow-x-auto">
														<table className="w-full text-sm">
															<thead>
																<tr className="text-left text-xs text-secondary-500 dark:text-secondary-400 border-b border-secondary-200 dark:border-secondary-600">
																	<th className="pb-2 font-medium">Name</th>
																	<th className="pb-2 font-medium">Driver</th>
																	<th className="pb-2 font-medium">
																		Mount Point
																	</th>
																</tr>
															</thead>
															<tbody className="divide-y divide-secondary-100 dark:divide-secondary-700">
																{dockerData.volumes?.map((volume) => (
																	<tr
																		key={volume.name}
																		className="hover:bg-secondary-50 dark:hover:bg-secondary-700/50"
																	>
																		<td
																			className="py-2 font-mono max-w-[200px] truncate"
																			title={volume.name}
																		>
																			<Link
																				to={`/docker/volumes/${volume.name}`}
																				className="text-primary-600 dark:text-primary-400 hover:text-primary-900 dark:hover:text-primary-300"
																			>
																				{volume.name}
																			</Link>
																		</td>
																		<td className="py-2">
																			<span className="px-2 py-0.5 bg-secondary-100 dark:bg-secondary-600 text-secondary-700 dark:text-secondary-300 rounded text-xs">
																				{volume.driver || "local"}
																			</span>
																		</td>
																		<td
																			className="py-2 text-xs font-mono text-secondary-500 dark:text-secondary-400 max-w-[300px] truncate"
																			title={volume.mountpoint}
																		>
																			{volume.mountpoint || "-"}
																		</td>
																	</tr>
																))}
															</tbody>
														</table>
													</div>
												)}
											</div>
										)}

										{/* Networks Sub-tab */}
										{dockerSubTab === "networks" && (
											<div className="space-y-2">
												{dockerData.networks?.length === 0 ? (
													<p className="text-secondary-500 dark:text-secondary-400 text-center py-4">
														No networks found
													</p>
												) : (
													<div className="overflow-x-auto">
														<table className="w-full text-sm">
															<thead>
																<tr className="text-left text-xs text-secondary-500 dark:text-secondary-400 border-b border-secondary-200 dark:border-secondary-600">
																	<th className="pb-2 font-medium">Name</th>
																	<th className="pb-2 font-medium">Driver</th>
																	<th className="pb-2 font-medium">Scope</th>
																	<th className="pb-2 font-medium">Subnet</th>
																</tr>
															</thead>
															<tbody className="divide-y divide-secondary-100 dark:divide-secondary-700">
																{dockerData.networks?.map((network) => (
																	<tr
																		key={network.id || network.name}
																		className="hover:bg-secondary-50 dark:hover:bg-secondary-700/50"
																	>
																		<td
																			className="py-2 font-mono max-w-[200px] truncate"
																			title={network.name}
																		>
																			<Link
																				to={`/docker/networks/${network.id || network.name}`}
																				className="text-primary-600 dark:text-primary-400 hover:text-primary-900 dark:hover:text-primary-300"
																			>
																				{network.name}
																			</Link>
																		</td>
																		<td className="py-2">
																			<span className="px-2 py-0.5 bg-secondary-100 dark:bg-secondary-600 text-secondary-700 dark:text-secondary-300 rounded text-xs">
																				{network.driver || "bridge"}
																			</span>
																		</td>
																		<td className="py-2 text-xs text-secondary-500 dark:text-secondary-400">
																			{network.scope || "-"}
																		</td>
																		<td className="py-2 text-xs font-mono text-secondary-500 dark:text-secondary-400">
																			{network.ipam?.config?.[0]?.subnet || "-"}
																		</td>
																	</tr>
																))}
															</tbody>
														</table>
													</div>
												)}
											</div>
										)}

										{/* Ports Sub-tab */}
										{dockerSubTab === "ports" && (
											<div className="space-y-2">
												{(() => {
													// Collect all ports from all containers
													const allPorts = [];
													dockerData.containers?.forEach((container) => {
														if (container.ports && container.ports.length > 0) {
															container.ports.forEach((port) => {
																allPorts.push({
																	containerName: container.name,
																	containerId: container.id,
																	containerState: container.state,
																	publicPort: port.PublicPort,
																	privatePort: port.PrivatePort,
																	type: port.Type || "tcp",
																	ip: port.IP || "0.0.0.0",
																});
															});
														}
													});

													if (allPorts.length === 0) {
														return (
															<p className="text-secondary-500 dark:text-secondary-400 text-center py-4">
																No ports found
															</p>
														);
													}

													return (
														<div className="overflow-x-auto">
															<table className="w-full text-sm">
																<thead>
																	<tr className="text-left text-xs text-secondary-500 dark:text-secondary-400 border-b border-secondary-200 dark:border-secondary-600">
																		<th className="pb-2 font-medium">
																			Container
																		</th>
																		<th className="pb-2 font-medium">
																			Public Port
																		</th>
																		<th className="pb-2 font-medium">
																			Private Port
																		</th>
																		<th className="pb-2 font-medium">Type</th>
																		<th className="pb-2 font-medium">IP</th>
																		<th className="pb-2 font-medium">Status</th>
																	</tr>
																</thead>
																<tbody className="divide-y divide-secondary-100 dark:divide-secondary-700">
																	{allPorts.map((port, idx) => (
																		<tr
																			key={`${port.containerId}-${idx}`}
																			className="hover:bg-secondary-50 dark:hover:bg-secondary-700/50"
																		>
																			<td className="py-2 font-medium text-secondary-900 dark:text-white">
																				{port.containerName}
																			</td>
																			<td className="py-2">
																				{port.publicPort ? (
																					<span className="px-2 py-0.5 bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-400 rounded text-xs font-mono">
																						{port.publicPort}
																					</span>
																				) : (
																					<span className="text-secondary-400">
																						-
																					</span>
																				)}
																			</td>
																			<td className="py-2">
																				<span className="px-2 py-0.5 bg-secondary-100 dark:bg-secondary-600 text-secondary-700 dark:text-secondary-300 rounded text-xs font-mono">
																					{port.privatePort}
																				</span>
																			</td>
																			<td className="py-2 text-xs text-secondary-500 dark:text-secondary-400 uppercase">
																				{port.type}
																			</td>
																			<td className="py-2 text-xs font-mono text-secondary-500 dark:text-secondary-400">
																				{port.ip}
																			</td>
																			<td className="py-2">
																				<span
																					className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-medium ${
																						port.containerState === "running"
																							? "bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-400"
																							: port.containerState === "exited"
																								? "bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-400"
																								: "bg-yellow-100 dark:bg-yellow-900/30 text-yellow-700 dark:text-yellow-400"
																					}`}
																				>
																					<span
																						className={`w-1.5 h-1.5 rounded-full ${
																							port.containerState === "running"
																								? "bg-green-500"
																								: port.containerState ===
																										"exited"
																									? "bg-red-500"
																									: "bg-yellow-500"
																						}`}
																					/>
																					{port.containerState}
																				</span>
																			</td>
																		</tr>
																	))}
																</tbody>
															</table>
														</div>
													);
												})()}
											</div>
										)}
									</>
								)}
							</div>
						)}

						{/* Compliance */}
						{activeTab === "compliance" && (
							<ComplianceTab
								hostId={hostId}
								apiId={host?.api_id}
								isConnected={wsStatus?.connected}
								complianceEnabled={host?.compliance_enabled}
								dockerEnabled={host?.docker_enabled}
							/>
						)}

						{/* Reporting */}
						{activeTab === "reporting" && (
							<div className="space-y-4">
								<div className="bg-secondary-50 dark:bg-secondary-700 rounded-lg p-4 border border-secondary-200 dark:border-secondary-600">
									<div className="flex items-center gap-3 mb-3">
										<AlertTriangle className="h-5 w-5 text-primary-600 dark:text-primary-400" />
										<h4 className="text-sm font-medium text-secondary-900 dark:text-white">
											Host Down Alerts
										</h4>
									</div>
									<p className="text-xs text-secondary-600 dark:text-secondary-300 mb-4">
										Control whether this host triggers alert entries when it
										goes offline. When disabled, no alerts will be created for
										this host even if the global setting is enabled.
									</p>

									{/* Settings and Buttons - Side by Side */}
									<div className="grid grid-cols-1 md:grid-cols-2 gap-4 mb-4">
										{/* Current Setting */}
										<div>
											<label className="text-xs font-medium text-secondary-500 dark:text-secondary-400 mb-2 block">
												Current Setting
											</label>
											<div className="text-sm text-secondary-900 dark:text-white">
												{host?.host_down_alerts_enabled === null ? (
													<span className="inline-flex items-center px-2 py-1 rounded bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200">
														Inherit from global settings
													</span>
												) : host?.host_down_alerts_enabled === true ? (
													<span className="inline-flex items-center px-2 py-1 rounded bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">
														Enabled
													</span>
												) : (
													<span className="inline-flex items-center px-2 py-1 rounded bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200">
														Disabled
													</span>
												)}
											</div>
										</div>

										{/* Global Setting Reference */}
										{hostDownAlertConfig && (
											<div>
												<label className="text-xs font-medium text-secondary-500 dark:text-secondary-400 mb-2 block">
													Global Setting
												</label>
												<div className="text-sm text-secondary-600 dark:text-secondary-300">
													{hostDownAlertConfig.is_enabled ? (
														<span className="inline-flex items-center px-2 py-1 rounded bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">
															Enabled
														</span>
													) : (
														<span className="inline-flex items-center px-2 py-1 rounded bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200">
															Disabled
														</span>
													)}
													{host?.host_down_alerts_enabled === null && (
														<span className="ml-2 text-xs text-secondary-500 dark:text-secondary-400">
															(currently inherited)
														</span>
													)}
												</div>
											</div>
										)}
									</div>

									{/* Action Buttons */}
									<div className="flex flex-wrap gap-2">
										<button
											type="button"
											onClick={() => toggleHostDownAlertsMutation.mutate(null)}
											disabled={
												toggleHostDownAlertsMutation.isPending ||
												host?.host_down_alerts_enabled === null
											}
											className={`px-3 py-1.5 text-sm font-medium rounded transition-colors ${
												host?.host_down_alerts_enabled === null
													? "bg-primary-600 text-white"
													: "bg-secondary-200 dark:bg-secondary-600 text-secondary-700 dark:text-secondary-300 hover:bg-secondary-300 dark:hover:bg-secondary-500"
											} disabled:opacity-50 disabled:cursor-not-allowed`}
										>
											Inherit
										</button>
										<button
											type="button"
											onClick={() => toggleHostDownAlertsMutation.mutate(true)}
											disabled={
												toggleHostDownAlertsMutation.isPending ||
												host?.host_down_alerts_enabled === true
											}
											className={`px-3 py-1.5 text-sm font-medium rounded transition-colors ${
												host?.host_down_alerts_enabled === true
													? "bg-green-600 text-white"
													: "bg-secondary-200 dark:bg-secondary-600 text-secondary-700 dark:text-secondary-300 hover:bg-secondary-300 dark:hover:bg-secondary-500"
											} disabled:opacity-50 disabled:cursor-not-allowed`}
										>
											Enable
										</button>
										<button
											type="button"
											onClick={() => toggleHostDownAlertsMutation.mutate(false)}
											disabled={
												toggleHostDownAlertsMutation.isPending ||
												host?.host_down_alerts_enabled === false
											}
											className={`px-3 py-1.5 text-sm font-medium rounded transition-colors ${
												host?.host_down_alerts_enabled === false
													? "bg-red-600 text-white"
													: "bg-secondary-200 dark:bg-secondary-600 text-secondary-700 dark:text-secondary-300 hover:bg-secondary-300 dark:hover:bg-secondary-500"
											} disabled:opacity-50 disabled:cursor-not-allowed`}
										>
											Disable
										</button>
									</div>

									{/* Success/Error Message */}
									{updateMessage.text && (
										<div
											className={`mt-3 text-sm ${
												updateMessage.text.includes("successfully")
													? "text-green-600 dark:text-green-400"
													: "text-red-600 dark:text-red-400"
											}`}
										>
											{updateMessage.text}
										</div>
									)}
								</div>
							</div>
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

			{/* Auto-Update Confirmation Dialog */}
			{autoUpdateDialog && (
				<div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
					<div className="bg-white dark:bg-secondary-800 rounded-lg shadow-xl max-w-md w-full mx-4 overflow-hidden">
						<div className="p-6">
							<div className="flex items-start gap-4">
								<div className="flex-shrink-0 w-10 h-10 rounded-full bg-amber-100 dark:bg-amber-900/30 flex items-center justify-center">
									<AlertTriangle className="h-5 w-5 text-amber-600 dark:text-amber-400" />
								</div>
								<div className="flex-1">
									<h3 className="text-lg font-semibold text-secondary-900 dark:text-white">
										Global Auto-Updates Disabled
									</h3>
									<p className="mt-2 text-sm text-secondary-600 dark:text-secondary-300">
										The master auto-update setting is currently{" "}
										<strong>disabled</strong> in Settings → Agent Updates.
									</p>
									<p className="mt-2 text-sm text-secondary-600 dark:text-secondary-300">
										Enabling auto-update for{" "}
										<strong>{host?.friendly_name || host?.hostname}</strong>{" "}
										won't take effect until global auto-updates are enabled.
									</p>
								</div>
							</div>
						</div>
						<div className="bg-secondary-50 dark:bg-secondary-700/50 px-6 py-4 flex flex-col sm:flex-row gap-3 sm:justify-end">
							<button
								type="button"
								onClick={() => setAutoUpdateDialog(false)}
								className="px-4 py-2 text-sm font-medium text-secondary-700 dark:text-secondary-200 bg-white dark:bg-secondary-600 border border-secondary-300 dark:border-secondary-500 rounded-md hover:bg-secondary-50 dark:hover:bg-secondary-500 transition-colors"
							>
								Cancel
							</button>
							<button
								type="button"
								onClick={handleEnableHostOnly}
								className="px-4 py-2 text-sm font-medium text-secondary-700 dark:text-secondary-200 bg-white dark:bg-secondary-600 border border-secondary-300 dark:border-secondary-500 rounded-md hover:bg-secondary-50 dark:hover:bg-secondary-500 transition-colors"
							>
								Enable Host Only
							</button>
							<button
								type="button"
								onClick={handleEnableBoth}
								disabled={enableGlobalAutoUpdateMutation.isPending}
								className="px-4 py-2 text-sm font-medium text-white bg-primary-600 rounded-md hover:bg-primary-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
							>
								{enableGlobalAutoUpdateMutation.isPending
									? "Enabling..."
									: "Enable Both"}
							</button>
						</div>
					</div>
				</div>
			)}
		</div>
	);
};

// Components moved to separate files in ./hostdetail/

export default HostDetail;
