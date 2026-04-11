import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
	AlertTriangle,
	ArrowDown,
	ArrowUp,
	ArrowUpDown,
	Bell,
	BookOpen,
	Calendar,
	CheckCircle,
	GitBranch,
	Info,
	LayoutDashboard,
	MoreVertical,
	RefreshCw,
	Search,
	Settings,
	Trash2,
	X,
	XCircle,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useLocation, useSearchParams } from "react-router-dom";
import {
	AlertResponderWorkload,
	AlertSeverityDoughnut,
	AlertsByType,
	AlertVolumeTrend,
	DeliveryByDestination,
	RecentAlerts,
} from "../components/alerting/widgets";
import { useAuth } from "../contexts/AuthContext";
import { useToast } from "../contexts/ToastContext";
import { adminUsersAPI, alertsAPI, formatRelativeTime } from "../utils/api";
import { NotificationPanel } from "./settings/AlertChannels";
import { AlertSettings } from "./settings/AlertSettings";

// System-only actions that should not appear in user-facing menus
const SYSTEM_ONLY_ACTIONS = new Set(["created", "updated"]);

const VALID_TABS = new Set([
	"overview",
	"alerts",
	"alert-settings",
	"destinations",
	"rules",
	"reports",
	"log",
]);

const Reporting = () => {
	const { user: _user } = useAuth();
	const queryClient = useQueryClient();
	const toast = useToast();
	const [searchParams] = useSearchParams();
	const location = useLocation();

	const urlTab = searchParams.get("tab");
	const urlSeverity = searchParams.get("severity");
	const urlStatus = searchParams.get("status");
	const urlType = searchParams.get("type");
	const urlAssignment = searchParams.get("assignment");

	const [searchTerm, setSearchTerm] = useState("");
	const [severityFilter, setSeverityFilter] = useState(urlSeverity || "all");
	const [typeFilter, setTypeFilter] = useState(urlType || "all");
	const [statusFilter, setStatusFilter] = useState(urlStatus || "all");
	const [assignmentFilter, setAssignmentFilter] = useState(
		urlAssignment || "all",
	);
	const [sortField, setSortField] = useState("created_at");
	const [sortDirection, setSortDirection] = useState("desc");
	const [selectedAlert, setSelectedAlert] = useState(null);
	const [showAlertModal, setShowAlertModal] = useState(false);
	const [openActionMenu, setOpenActionMenu] = useState(null);
	const [menuPosition, setMenuPosition] = useState({ top: 0, right: 0 });
	const menuButtonRefs = useRef({});
	const [selectedAlerts, setSelectedAlerts] = useState(new Set());
	const [activeTab, setActiveTab] = useState(
		VALID_TABS.has(urlTab) ? urlTab : "overview",
	);

	// Sync tab and filters on actual URL navigation (not in-page tab clicks)
	useEffect(() => {
		const params = new URLSearchParams(location.search);
		const tab = params.get("tab");
		if (tab && VALID_TABS.has(tab)) {
			setActiveTab(tab);
		}
		const severity = params.get("severity");
		if (severity) setSeverityFilter(severity);
		const status = params.get("status");
		if (status) setStatusFilter(status);
		const type = params.get("type");
		if (type) setTypeFilter(type);
		const assignment = params.get("assignment");
		if (assignment) setAssignmentFilter(assignment);
	}, [location.search]);

	const tabs = [
		{ id: "overview", name: "Overview", icon: LayoutDashboard },
		{ id: "alerts", name: "Alerts", icon: AlertTriangle },
		{ id: "alert-settings", name: "Alert Lifecycle", icon: Settings },
		{ id: "destinations", name: "Destinations", icon: Bell },
		{ id: "rules", name: "Event Rules", icon: GitBranch },
		{ id: "reports", name: "Scheduled Reports", icon: Calendar },
		{ id: "log", name: "Delivery Log", icon: BookOpen },
	];

	// Fetch ALL alerts (unfiltered) for overview widgets
	const {
		data: allAlertsData,
		refetch: refetchAlerts,
		isFetching: isFetchingAlerts,
	} = useQuery({
		queryKey: ["alerts"],
		queryFn: async () => {
			const response = await alertsAPI.getAlerts();
			return response.data.data || [];
		},
		refetchInterval: 30000,
		refetchOnWindowFocus: true,
		refetchOnMount: true,
		staleTime: 0,
	});

	// Fetch alerts for the alerts table tab (respects assignment filter)
	const {
		data: alertsData,
		isLoading: alertsLoading,
		error: alertsError,
	} = useQuery({
		queryKey: ["alerts", "filtered", assignmentFilter],
		queryFn: async () => {
			const params = {};
			if (assignmentFilter === "assignedToMe") {
				params.assignedToMe = "true";
			}
			const response = await alertsAPI.getAlerts(params);
			return response.data.data || [];
		},
		refetchInterval: 30000,
		refetchOnWindowFocus: true,
		refetchOnMount: true,
		staleTime: 0,
	});

	// Fetch alert stats - polling for updates
	const { data: statsData, isLoading: statsLoading } = useQuery({
		queryKey: ["alert-stats"],
		queryFn: async () => {
			const response = await alertsAPI.getAlertStats();
			return response.data.data || {};
		},
		refetchInterval: 30000, // Refresh every 30 seconds to reduce API load
		refetchOnWindowFocus: true, // Refetch when user returns to tab
		refetchOnMount: true, // Always refetch on mount
		staleTime: 0, // Data is immediately stale, always refetch
	});

	// Fetch available actions
	const { data: availableActions } = useQuery({
		queryKey: ["alert-actions"],
		queryFn: async () => {
			const response = await alertsAPI.getAvailableActions();
			return response.data.data || [];
		},
	});

	// Fetch users for assignment (use public endpoint that works for all authenticated users)
	const { data: usersData } = useQuery({
		queryKey: ["users", "for-assignment"],
		queryFn: async () => {
			try {
				// Try public assignment endpoint first (available to all authenticated users)
				const response = await adminUsersAPI.listForAssignment();
				return response.data.data || [];
			} catch (error) {
				// Fallback to admin endpoint if user has permissions
				if (error.response?.status === 403 || error.response?.status === 401) {
					try {
						const response = await adminUsersAPI.list();
						return response.data.data || [];
					} catch (_e) {
						// If both fail, return empty array
						return [];
					}
				}
				// For other errors, return empty array
				return [];
			}
		},
	});

	// Fetch alert history when modal is open
	const { data: alertHistory } = useQuery({
		queryKey: ["alert-history", selectedAlert?.id],
		queryFn: async () => {
			if (!selectedAlert?.id) return [];
			const response = await alertsAPI.getAlertHistory(selectedAlert.id);
			return response.data.data || [];
		},
		enabled: !!selectedAlert?.id && showAlertModal,
	});

	// Perform alert action mutation
	const performActionMutation = useMutation({
		mutationFn: async ({ alertId, action, metadata }) => {
			return alertsAPI.performAlertAction(alertId, action, metadata);
		},
		onSuccess: () => {
			// Immediately refetch to show changes
			queryClient.invalidateQueries({ queryKey: ["alerts"] });
			queryClient.invalidateQueries({ queryKey: ["alert-stats"] });
			refetchAlerts();
		},
	});

	// Assign alert mutation
	const assignAlertMutation = useMutation({
		mutationFn: async ({ alertId, userId }) => {
			return alertsAPI.assignAlert(alertId, userId);
		},
		onSuccess: () => {
			// Immediately refetch to show changes
			queryClient.invalidateQueries({ queryKey: ["alerts"] });
			queryClient.invalidateQueries({ queryKey: ["alert-stats"] }); // Invalidate all alert-stats queries (including sidebar)
			refetchAlerts();
		},
	});

	// Unassign alert mutation
	const unassignAlertMutation = useMutation({
		mutationFn: async (alertId) => {
			return alertsAPI.unassignAlert(alertId);
		},
		onSuccess: () => {
			// Immediately refetch to show changes
			queryClient.invalidateQueries({ queryKey: ["alerts"] });
			queryClient.invalidateQueries({ queryKey: ["alert-stats"] }); // Invalidate all alert-stats queries (including sidebar)
			refetchAlerts();
		},
	});

	// Delete alerts mutation
	const deleteAlertsMutation = useMutation({
		mutationFn: async (alertIds) => {
			if (alertIds.length === 1) {
				return alertsAPI.deleteAlert(alertIds[0]);
			} else {
				return alertsAPI.bulkDeleteAlerts(alertIds);
			}
		},
		onSuccess: () => {
			// Immediately refetch to show changes
			queryClient.invalidateQueries({ queryKey: ["alerts"] });
			queryClient.invalidateQueries({ queryKey: ["alert-stats"] });
			refetchAlerts();
			setSelectedAlerts(new Set()); // Clear selection after delete
		},
	});

	// Bulk action mutation (acknowledge, resolve, etc. for multiple alerts)
	const bulkActionMutation = useMutation({
		mutationFn: async ({ alertIds, action }) => {
			return alertsAPI.bulkAction(alertIds, action);
		},
		onSuccess: (_data, variables) => {
			queryClient.invalidateQueries({ queryKey: ["alerts"] });
			queryClient.invalidateQueries({ queryKey: ["alert-stats"] });
			refetchAlerts();
			setSelectedAlerts(new Set());
			toast.success(
				`${variables.alertIds.length} alert(s) updated: ${variables.action}`,
			);
		},
		onError: (err) => {
			toast.error(err.response?.data?.error || "Failed to perform bulk action");
		},
	});

	// Handle bulk action on selected alerts
	const handleBulkAction = async (actionName) => {
		if (selectedAlerts.size === 0) return;
		await bulkActionMutation.mutateAsync({
			alertIds: Array.from(selectedAlerts),
			action: actionName,
		});
	};

	// Track previous stats for trend indicators
	const prevStatsRef = useRef(null);
	useEffect(() => {
		if (statsData && prevStatsRef.current === null) {
			prevStatsRef.current = statsData;
		}
	}, [statsData]);

	const allAlerts = allAlertsData || [];
	const alerts = alertsData || [];
	const stats = statsData || {};

	// Get severity badge
	const getSeverityBadge = (severity) => {
		const severityLower = severity?.toLowerCase() || "informational";
		const colors = {
			informational:
				"bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200",
			warning:
				"bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200",
			error:
				"bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200",
			critical: "bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200",
		};
		return (
			<span
				className={`px-2 py-1 text-xs font-medium rounded ${
					colors[severityLower] || colors.informational
				}`}
			>
				{severity}
			</span>
		);
	};

	// Get user-facing actions (excludes system-only), split into workflow vs resolution
	const workflowActions = useMemo(
		() =>
			(availableActions || []).filter(
				(a) => !a.is_state_action && !SYSTEM_ONLY_ACTIONS.has(a.name),
			),
		[availableActions],
	);
	const resolutionActions = useMemo(
		() => (availableActions || []).filter((a) => a.is_state_action),
		[availableActions],
	);

	// Get status badge from current state
	const getStatusBadge = (alert) => {
		const currentState = alert.current_state;
		if (!currentState?.action) {
			return (
				<span className="px-2 py-1 text-xs font-medium rounded bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200">
					Open
				</span>
			);
		}

		const action = currentState.action.toLowerCase();
		const statusStyles = {
			acknowledged:
				"bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200",
			investigating:
				"bg-indigo-100 text-indigo-800 dark:bg-indigo-900 dark:text-indigo-200",
			escalated:
				"bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200",
			silenced:
				"bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200",
			done: "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200",
			resolved: "bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200",
		};
		const style =
			statusStyles[action] ||
			"bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200";
		const label =
			action.charAt(0).toUpperCase() + action.slice(1).replace("_", " ");

		return (
			<span className={`px-2 py-1 text-xs font-medium rounded ${style}`}>
				{label}
			</span>
		);
	};

	// Get type badge
	const getTypeBadge = (type) => {
		if (type == null || typeof type !== "string") {
			return (
				<span className="px-2 py-1 text-xs font-medium rounded bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200">
					-
				</span>
			);
		}
		const typeColors = {
			server_update:
				"bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200",
			agent_update:
				"bg-indigo-100 text-indigo-800 dark:bg-indigo-900 dark:text-indigo-200",
			host_down: "bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200",
		};
		return (
			<span
				className={`px-2 py-1 text-xs font-medium rounded ${
					typeColors[type] ||
					"bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200"
				}`}
			>
				{type.replace("_", " ")}
			</span>
		);
	};

	// Filter and sort alerts
	const filteredAndSortedAlerts = useMemo(() => {
		let filtered = [...alerts];

		// Search filter
		if (searchTerm) {
			const searchLower = searchTerm.toLowerCase();
			filtered = filtered.filter(
				(alert) =>
					alert.title?.toLowerCase().includes(searchLower) ||
					alert.message?.toLowerCase().includes(searchLower) ||
					alert.type?.toLowerCase().includes(searchLower),
			);
		}

		// Severity filter
		if (severityFilter !== "all") {
			filtered = filtered.filter(
				(alert) =>
					alert.severity?.toLowerCase() === severityFilter.toLowerCase(),
			);
		}

		// Type filter
		if (typeFilter !== "all") {
			filtered = filtered.filter((alert) => alert.type === typeFilter);
		}

		// Status filter
		if (statusFilter !== "all") {
			filtered = filtered.filter((alert) => {
				const currentState = alert.current_state;
				const action = currentState?.action?.toLowerCase() || "";
				const resolvedStates = ["done", "resolved"];
				if (statusFilter === "open")
					return !action || !resolvedStates.includes(action);
				if (statusFilter === "acknowledged") return action === "acknowledged";
				if (statusFilter === "investigating") return action === "investigating";
				if (statusFilter === "escalated") return action === "escalated";
				if (statusFilter === "silenced") return action === "silenced";
				if (statusFilter === "done") return action === "done";
				if (statusFilter === "resolved") return action === "resolved";
				return true;
			});
		}

		// Assignment filter
		if (assignmentFilter === "assigned") {
			filtered = filtered.filter((alert) => alert.assigned_to_user_id !== null);
		} else if (assignmentFilter === "unassigned") {
			filtered = filtered.filter((alert) => alert.assigned_to_user_id === null);
		}

		// Sort
		filtered.sort((a, b) => {
			let aValue, bValue;

			if (sortField === "created_at") {
				aValue = new Date(a.created_at).getTime();
				bValue = new Date(b.created_at).getTime();
			} else if (sortField === "severity") {
				const severityOrder = {
					critical: 4,
					error: 3,
					warning: 2,
					informational: 1,
				};
				aValue = severityOrder[a.severity?.toLowerCase()] || 0;
				bValue = severityOrder[b.severity?.toLowerCase()] || 0;
			} else if (sortField === "type") {
				aValue = a.type || "";
				bValue = b.type || "";
			} else {
				aValue = a[sortField] || "";
				bValue = b[sortField] || "";
			}

			if (sortDirection === "asc") {
				return aValue > bValue ? 1 : aValue < bValue ? -1 : 0;
			} else {
				return aValue < bValue ? 1 : aValue > bValue ? -1 : 0;
			}
		});

		return filtered;
	}, [
		alerts,
		searchTerm,
		severityFilter,
		typeFilter,
		statusFilter,
		assignmentFilter,
		sortField,
		sortDirection,
	]);

	// Get unique alert types for filter
	const alertTypes = useMemo(() => {
		const types = new Set(
			alerts.map((alert) => alert.type).filter((t) => t != null && t !== ""),
		);
		return Array.from(types).sort();
	}, [alerts]);

	// Handle sort
	const handleSort = (field) => {
		if (sortField === field) {
			setSortDirection(sortDirection === "asc" ? "desc" : "asc");
		} else {
			setSortField(field);
			setSortDirection("asc");
		}
	};

	// Get sort icon
	const getSortIcon = (field) => {
		if (sortField !== field) {
			return <ArrowUpDown className="h-4 w-4 text-secondary-400" />;
		}
		return sortDirection === "asc" ? (
			<ArrowUp className="h-4 w-4 text-primary-600" />
		) : (
			<ArrowDown className="h-4 w-4 text-primary-600" />
		);
	};

	// Handle action
	const handleAction = async (alertId, action, e) => {
		if (e) {
			e.stopPropagation();
		}
		try {
			await performActionMutation.mutateAsync({
				alertId,
				action,
			});
		} catch (error) {
			console.error("Failed to perform action:", error);
		}
	};

	// Calculate menu position based on button location
	const calculateMenuPosition = useCallback((alertId) => {
		const buttonRef = menuButtonRefs.current[alertId];
		if (buttonRef) {
			const rect = buttonRef.getBoundingClientRect();
			setMenuPosition({
				top: rect.bottom + window.scrollY + 4,
				right: window.innerWidth - rect.right + window.scrollX,
			});
		}
	}, []);

	// Update menu position when menu opens
	useEffect(() => {
		if (openActionMenu) {
			calculateMenuPosition(openActionMenu);
			const handleScroll = () => {
				if (openActionMenu) {
					calculateMenuPosition(openActionMenu);
				}
			};
			const handleResize = () => {
				if (openActionMenu) {
					calculateMenuPosition(openActionMenu);
				}
			};
			window.addEventListener("scroll", handleScroll, true);
			window.addEventListener("resize", handleResize);
			return () => {
				window.removeEventListener("scroll", handleScroll, true);
				window.removeEventListener("resize", handleResize);
			};
		}
	}, [openActionMenu, calculateMenuPosition]);

	// Close menu when clicking outside
	useEffect(() => {
		const handleClickOutside = (event) => {
			if (openActionMenu) {
				const buttonRef = menuButtonRefs.current[openActionMenu];
				if (buttonRef && !buttonRef.contains(event.target)) {
					// Check if click is outside the menu as well
					const menuElement = document.querySelector(
						`[data-menu-id="${openActionMenu}"]`,
					);
					if (menuElement && !menuElement.contains(event.target)) {
						setOpenActionMenu(null);
					}
				}
			}
		};

		if (openActionMenu) {
			document.addEventListener("mousedown", handleClickOutside);
			return () => {
				document.removeEventListener("mousedown", handleClickOutside);
			};
		}
	}, [openActionMenu]);

	// Handle row click to show details
	const handleRowClick = (alert) => {
		setSelectedAlert(alert);
		setShowAlertModal(true);
	};

	// Handle inline assignment
	const handleInlineAssign = async (alertId, userId, e) => {
		if (e) {
			e.stopPropagation();
		}
		try {
			if (userId) {
				await assignAlertMutation.mutateAsync({ alertId, userId });
			} else {
				await unassignAlertMutation.mutateAsync(alertId);
			}
		} catch (error) {
			console.error("Failed to assign alert:", error);
		}
	};

	// Handle checkbox selection
	const handleSelectAlert = (alertId, checked) => {
		const newSelected = new Set(selectedAlerts);
		if (checked) {
			newSelected.add(alertId);
		} else {
			newSelected.delete(alertId);
		}
		setSelectedAlerts(newSelected);
	};

	// Handle select all
	const handleSelectAll = (checked) => {
		if (checked) {
			setSelectedAlerts(new Set(filteredAndSortedAlerts.map((a) => a.id)));
		} else {
			setSelectedAlerts(new Set());
		}
	};

	// Handle delete selected alerts
	const handleDeleteSelected = async () => {
		if (selectedAlerts.size === 0) return;

		if (
			window.confirm(
				`Are you sure you want to delete ${selectedAlerts.size} alert(s)? This action cannot be undone.`,
			)
		) {
			try {
				await deleteAlertsMutation.mutateAsync(Array.from(selectedAlerts));
			} catch (error) {
				console.error("Failed to delete alerts:", error);
			}
		}
	};

	if (alertsError) {
		return (
			<div className="bg-danger-50 border border-danger-200 rounded-md p-4">
				<div className="flex">
					<AlertTriangle className="h-5 w-5 text-danger-400" />
					<div className="ml-3">
						<h3 className="text-sm font-medium text-danger-800">
							Error loading alerts
						</h3>
						<p className="text-sm text-danger-700 mt-1">
							{alertsError.message || "Failed to load alerts"}
						</p>
						<button
							type="button"
							onClick={() => refetchAlerts()}
							className="mt-2 btn-danger text-xs"
						>
							Try again
						</button>
					</div>
				</div>
			</div>
		);
	}

	return (
		<div className="space-y-6">
			<div className="flex items-center justify-between">
				<div>
					<h1 className="text-2xl font-semibold text-secondary-900 dark:text-white">
						Reporting
					</h1>
					<p className="text-sm text-secondary-600 dark:text-white mt-1">
						View and manage system alerts and notifications
					</p>
				</div>
				<div className="flex items-center gap-3">
					<button
						type="button"
						onClick={() => refetchAlerts()}
						disabled={isFetchingAlerts}
						className="btn-outline flex items-center justify-center p-2"
						title="Refresh alerts"
					>
						<RefreshCw
							className={`h-4 w-4 ${isFetchingAlerts ? "animate-spin" : ""}`}
						/>
					</button>
				</div>
			</div>

			<div className="grid grid-cols-2 lg:grid-cols-5 gap-4">
				{[
					{
						label: "Informational",
						icon: Info,
						color: "text-blue-600",
						value: stats.informational || 0,
						filter: "informational",
					},
					{
						label: "Warning",
						icon: AlertTriangle,
						color: "text-yellow-600",
						value: stats.warning || 0,
						filter: "warning",
					},
					{
						label: "Error",
						icon: XCircle,
						color: "text-orange-600",
						value: stats.error || 0,
						filter: "error",
					},
					{
						label: "Critical",
						icon: AlertTriangle,
						color: "text-red-600",
						value: stats.critical || 0,
						filter: "critical",
					},
					{
						label: "Total Active",
						icon: CheckCircle,
						color: "text-secondary-600",
						value:
							(stats.informational || 0) +
							(stats.warning || 0) +
							(stats.error || 0) +
							(stats.critical || 0),
						filter: "all",
					},
				].map((card) => (
					<button
						key={card.label}
						type="button"
						className={`card card-hover p-4 text-left ${activeTab === "alerts" && severityFilter === card.filter ? "ring-2 ring-primary-500" : ""}`}
						onClick={() => {
							setActiveTab("alerts");
							setSeverityFilter(card.filter);
							setStatusFilter("open");
						}}
					>
						<div className="flex items-center">
							<div className="flex-shrink-0">
								<card.icon className={`h-5 w-5 ${card.color} mr-2`} />
							</div>
							<div className="w-0 flex-1">
								<p className="text-sm text-secondary-500 dark:text-white">
									{card.label}
								</p>
								<p className="text-xl font-semibold text-secondary-900 dark:text-white">
									{statsLoading ? "..." : card.value}
								</p>
							</div>
						</div>
					</button>
				))}
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

			{/* Overview Tab */}
			{activeTab === "overview" && (
				<div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4 sm:gap-6 auto-rows-[320px]">
					<DeliveryByDestination />
					<AlertSeverityDoughnut stats={stats} />
					<AlertVolumeTrend alerts={allAlerts} />
					<AlertsByType alerts={allAlerts} />
					<RecentAlerts alerts={allAlerts} />
					<AlertResponderWorkload alerts={allAlerts} users={usersData} />
				</div>
			)}

			{/* Alerts Tab */}
			{activeTab === "alerts" && (
				<>
					{/* Filters and Search */}
					<div className="card p-4">
						<div className="flex flex-col md:flex-row gap-4">
							{/* Search */}
							<div className="flex-1">
								<div className="relative">
									<Search className="absolute left-3 top-1/2 transform -translate-y-1/2 h-4 w-4 text-secondary-400" />
									<input
										type="text"
										placeholder="Search alerts..."
										value={searchTerm}
										onChange={(e) => setSearchTerm(e.target.value)}
										className="w-full pl-10 pr-4 py-2 border border-secondary-300 dark:border-secondary-600 rounded-md bg-white dark:bg-secondary-800 text-secondary-900 dark:text-white"
									/>
								</div>
							</div>

							{/* Filters */}
							<div className="flex flex-wrap gap-2">
								<select
									value={severityFilter}
									onChange={(e) => setSeverityFilter(e.target.value)}
									className="px-3 py-2 border border-secondary-300 dark:border-secondary-600 rounded-md bg-white dark:bg-secondary-800 text-secondary-900 dark:text-white text-sm"
								>
									<option value="all">All Severities</option>
									<option value="informational">Informational</option>
									<option value="warning">Warning</option>
									<option value="error">Error</option>
									<option value="critical">Critical</option>
								</select>

								<select
									value={typeFilter}
									onChange={(e) => setTypeFilter(e.target.value)}
									className="px-3 py-2 border border-secondary-300 dark:border-secondary-600 rounded-md bg-white dark:bg-secondary-800 text-secondary-900 dark:text-white text-sm"
								>
									<option value="all">All Types</option>
									{alertTypes.map((type) => (
										<option key={type} value={type}>
											{String(type).replace("_", " ")}
										</option>
									))}
								</select>

								<select
									value={statusFilter}
									onChange={(e) => setStatusFilter(e.target.value)}
									className="px-3 py-2 border border-secondary-300 dark:border-secondary-600 rounded-md bg-white dark:bg-secondary-800 text-secondary-900 dark:text-white text-sm"
								>
									<option value="all">All Status</option>
									<option value="open">Open</option>
									<option value="acknowledged">Acknowledged</option>
									<option value="investigating">Investigating</option>
									<option value="escalated">Escalated</option>
									<option value="silenced">Silenced</option>
									<option value="done">Done</option>
									<option value="resolved">Resolved</option>
								</select>

								<select
									value={assignmentFilter}
									onChange={(e) => setAssignmentFilter(e.target.value)}
									className="px-3 py-2 border border-secondary-300 dark:border-secondary-600 rounded-md bg-white dark:bg-secondary-800 text-secondary-900 dark:text-white text-sm"
								>
									<option value="all">All Assignments</option>
									<option value="assignedToMe">Assigned to me</option>
									<option value="assigned">Assigned</option>
									<option value="unassigned">Unassigned</option>
								</select>
							</div>
						</div>
					</div>

					{/* Alerts Table */}
					<div className="card overflow-hidden">
						{/* Bulk Actions Bar */}
						{selectedAlerts.size > 0 && (
							<div className="px-4 py-2 bg-primary-50 dark:bg-primary-900 border-b border-secondary-200 dark:border-secondary-700 flex items-center justify-between gap-2 flex-wrap">
								<div className="text-sm text-secondary-700 dark:text-white">
									{selectedAlerts.size} alert(s) selected
								</div>
								<div className="flex items-center gap-2 flex-wrap">
									{workflowActions.map((action) => (
										<button
											key={action.name}
											type="button"
											onClick={() => handleBulkAction(action.name)}
											disabled={bulkActionMutation.isPending}
											className="px-3 py-1.5 text-sm font-medium text-secondary-700 dark:text-white bg-white dark:bg-secondary-700 border border-secondary-300 dark:border-secondary-600 hover:bg-secondary-50 dark:hover:bg-secondary-600 rounded-md disabled:opacity-50 disabled:cursor-not-allowed"
										>
											{action.display_name}
										</button>
									))}
									{resolutionActions.map((action) => (
										<button
											key={action.name}
											type="button"
											onClick={() => handleBulkAction(action.name)}
											disabled={bulkActionMutation.isPending}
											className="px-3 py-1.5 text-sm font-medium text-green-700 dark:text-green-300 bg-green-50 dark:bg-green-900/30 border border-green-300 dark:border-green-700 hover:bg-green-100 dark:hover:bg-green-900/50 rounded-md disabled:opacity-50 disabled:cursor-not-allowed"
										>
											{action.display_name}
										</button>
									))}
									<button
										type="button"
										onClick={handleDeleteSelected}
										disabled={deleteAlertsMutation.isPending}
										className="flex items-center gap-2 px-3 py-1.5 text-sm font-medium text-white bg-danger-600 hover:bg-danger-700 rounded-md disabled:opacity-50 disabled:cursor-not-allowed"
									>
										<Trash2 className="h-4 w-4" />
										Delete
									</button>
								</div>
							</div>
						)}
						{alertsLoading ? (
							<div className="text-center py-8">
								<div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600 mx-auto"></div>
								<p className="mt-2 text-sm text-secondary-500">
									Loading alerts...
								</p>
							</div>
						) : filteredAndSortedAlerts.length === 0 ? (
							<div className="text-center py-8">
								<AlertTriangle className="h-12 w-12 mx-auto text-secondary-400" />
								<h3 className="mt-2 text-sm font-medium text-secondary-900 dark:text-white">
									No alerts found
								</h3>
								<p className="mt-1 text-sm text-secondary-500">
									{searchTerm ||
									severityFilter !== "all" ||
									typeFilter !== "all"
										? "Try adjusting your search filters"
										: "No active alerts"}
								</p>
							</div>
						) : (
							<div className="overflow-x-auto">
								<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-700">
									<thead className="bg-secondary-50 dark:bg-secondary-800">
										<tr>
											<th
												scope="col"
												className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider"
												onClick={(e) => e.stopPropagation()}
											>
												<input
													type="checkbox"
													checked={
														selectedAlerts.size > 0 &&
														selectedAlerts.size ===
															filteredAndSortedAlerts.length
													}
													onChange={(e) => {
														e.stopPropagation();
														handleSelectAll(e.target.checked);
													}}
													onClick={(e) => e.stopPropagation()}
													className="h-4 w-4 text-primary-600 focus:ring-primary-500 border-secondary-300 rounded cursor-pointer"
												/>
											</th>
											<th
												scope="col"
												className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider cursor-pointer"
												onClick={() => handleSort("severity")}
											>
												<div className="flex items-center gap-2">
													Severity
													{getSortIcon("severity")}
												</div>
											</th>
											<th
												scope="col"
												className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider cursor-pointer"
												onClick={() => handleSort("type")}
											>
												<div className="flex items-center gap-2">
													Type
													{getSortIcon("type")}
												</div>
											</th>
											<th
												scope="col"
												className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider"
											>
												Title
											</th>
											<th
												scope="col"
												className="hidden md:table-cell px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider"
											>
												Message
											</th>
											<th
												scope="col"
												className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider"
											>
												Assigned To
											</th>
											<th
												scope="col"
												className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider"
											>
												Status
											</th>
											<th
												scope="col"
												className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider cursor-pointer"
												onClick={() => handleSort("created_at")}
											>
												<div className="flex items-center gap-2">
													Created
													{getSortIcon("created_at")}
												</div>
											</th>
											<th
												scope="col"
												className="px-4 py-2 text-right text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider"
											>
												Actions
											</th>
										</tr>
									</thead>
									<tbody className="bg-white dark:bg-secondary-900 divide-y divide-secondary-200 dark:divide-secondary-700">
										{filteredAndSortedAlerts.map((alert) => (
											<tr
												key={alert.id}
												className="hover:bg-secondary-50 dark:hover:bg-secondary-800"
											>
												<td
													className="px-4 py-2 whitespace-nowrap"
													onClick={(e) => e.stopPropagation()}
												>
													<input
														type="checkbox"
														checked={selectedAlerts.has(alert.id)}
														onChange={(e) => {
															e.stopPropagation();
															handleSelectAlert(alert.id, e.target.checked);
														}}
														onClick={(e) => e.stopPropagation()}
														className="h-4 w-4 text-primary-600 focus:ring-primary-500 border-secondary-300 rounded cursor-pointer"
													/>
												</td>
												<td
													className="px-4 py-2 whitespace-nowrap cursor-pointer"
													onClick={() => handleRowClick(alert)}
												>
													{getSeverityBadge(alert.severity)}
												</td>
												<td
													className="px-4 py-2 whitespace-nowrap cursor-pointer"
													onClick={() => handleRowClick(alert)}
												>
													{getTypeBadge(alert.type)}
												</td>
												<td
													className="px-4 py-2 cursor-pointer"
													onClick={() => handleRowClick(alert)}
												>
													<div className="text-sm font-medium text-secondary-900 dark:text-white">
														{alert.title}
													</div>
												</td>
												<td
													className="hidden md:table-cell px-4 py-2 cursor-pointer"
													onClick={() => handleRowClick(alert)}
												>
													<div className="text-sm text-secondary-500 dark:text-white max-w-md truncate">
														{alert.message}
													</div>
												</td>
												<td
													className="px-4 py-2 whitespace-nowrap"
													onClick={(e) => e.stopPropagation()}
												>
													<select
														value={alert.assigned_to_user_id || ""}
														onChange={(e) => {
															const newUserId = e.target.value;
															handleInlineAssign(alert.id, newUserId, e);
														}}
														onClick={(e) => e.stopPropagation()}
														className="px-2 py-1 text-sm border border-secondary-300 dark:border-secondary-600 rounded-md bg-white dark:bg-secondary-800 text-secondary-900 dark:text-white hover:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-primary-500 min-w-[120px]"
														disabled={
															assignAlertMutation.isPending ||
															unassignAlertMutation.isPending
														}
													>
														<option value="">Unassigned</option>
														{usersData?.map((u) => (
															<option key={u.id} value={u.id}>
																{u.username || u.email}
															</option>
														))}
													</select>
												</td>
												<td
													className="px-4 py-2 whitespace-nowrap cursor-pointer"
													onClick={() => handleRowClick(alert)}
												>
													{getStatusBadge(alert)}
												</td>
												<td
													className="px-4 py-2 whitespace-nowrap text-sm text-secondary-500 dark:text-white cursor-pointer"
													onClick={() => handleRowClick(alert)}
												>
													{alert.created_at
														? formatRelativeTime(alert.created_at)
														: " -"}
												</td>
												<td
													className="px-4 py-2 whitespace-nowrap text-right text-sm font-medium"
													onClick={(e) => e.stopPropagation()}
												>
													<div className="relative inline-block">
														<button
															ref={(el) => {
																if (el) {
																	menuButtonRefs.current[alert.id] = el;
																} else {
																	delete menuButtonRefs.current[alert.id];
																}
															}}
															type="button"
															onClick={(e) => {
																e.stopPropagation();
																const newMenuId =
																	openActionMenu === alert.id ? null : alert.id;
																setOpenActionMenu(newMenuId);
																if (newMenuId) {
																	// Calculate position after state update
																	setTimeout(
																		() => calculateMenuPosition(newMenuId),
																		0,
																	);
																}
															}}
															className="p-1 text-secondary-500 hover:text-secondary-700 dark:hover:text-secondary-300 rounded-md hover:bg-secondary-100 dark:hover:bg-secondary-700"
															disabled={performActionMutation.isPending}
														>
															<MoreVertical className="h-5 w-5" />
														</button>
													</div>
												</td>
											</tr>
										))}
									</tbody>
								</table>
							</div>
						)}
					</div>

					{/* Fixed dropdown menu (rendered outside table to avoid clipping) */}
					{openActionMenu && (
						<>
							<div
								className="fixed inset-0 z-40"
								onClick={(e) => {
									e.stopPropagation();
									setOpenActionMenu(null);
								}}
							/>
							<div
								data-menu-id={openActionMenu}
								className="fixed z-50 w-48 bg-white dark:bg-secondary-800 rounded-md shadow-lg border border-secondary-200 dark:border-secondary-600"
								style={{
									top: `${menuPosition.top}px`,
									right: `${menuPosition.right}px`,
								}}
								onClick={(e) => e.stopPropagation()}
							>
								<div className="py-1">
									{workflowActions.length > 0 && (
										<>
											<div className="px-4 py-1 text-xs font-semibold text-secondary-400 dark:text-secondary-300 uppercase tracking-wider">
												Workflow
											</div>
											{workflowActions.map((action) => (
												<button
													key={action.name}
													type="button"
													onClick={(e) => {
														e.stopPropagation();
														handleAction(openActionMenu, action.name, e);
														setOpenActionMenu(null);
													}}
													className="w-full text-left px-4 py-2 text-sm text-secondary-700 dark:text-white hover:bg-secondary-100 dark:hover:bg-secondary-700"
													disabled={performActionMutation.isPending}
												>
													{action.display_name}
												</button>
											))}
										</>
									)}
									{workflowActions.length > 0 &&
										resolutionActions.length > 0 && (
											<div className="border-t border-secondary-200 dark:border-secondary-600 my-1" />
										)}
									{resolutionActions.length > 0 && (
										<>
											<div className="px-4 py-1 text-xs font-semibold text-secondary-400 dark:text-secondary-300 uppercase tracking-wider">
												Resolve
											</div>
											{resolutionActions.map((action) => (
												<button
													key={action.name}
													type="button"
													onClick={(e) => {
														e.stopPropagation();
														handleAction(openActionMenu, action.name, e);
														setOpenActionMenu(null);
													}}
													className="w-full text-left px-4 py-2 text-sm text-secondary-700 dark:text-white hover:bg-secondary-100 dark:hover:bg-secondary-700"
													disabled={performActionMutation.isPending}
												>
													{action.display_name}
												</button>
											))}
										</>
									)}
								</div>
							</div>
						</>
					)}
				</>
			)}

			{/* Alert Settings Tab */}
			{activeTab === "alert-settings" && <AlertSettings />}

			{/* Destinations Tab */}
			{activeTab === "destinations" && (
				<NotificationPanel panel="destinations" />
			)}

			{/* Event Rules Tab */}
			{activeTab === "rules" && <NotificationPanel panel="routes" />}

			{/* Scheduled Reports Tab */}
			{activeTab === "reports" && <NotificationPanel panel="reports" />}

			{/* Delivery Log Tab */}
			{activeTab === "log" && <NotificationPanel panel="log" />}

			{/* Alert Details Modal */}
			{showAlertModal && selectedAlert && (
				<div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4">
					<button
						type="button"
						onClick={() => {
							setShowAlertModal(false);
							setSelectedAlert(null);
						}}
						className="fixed inset-0 cursor-default"
						aria-label="Close modal"
					/>
					<div className="bg-white dark:bg-secondary-800 rounded-lg shadow-xl max-w-3xl w-full mx-4 relative z-10 max-h-[90vh] overflow-y-auto">
						<div className="px-6 py-4 border-b border-secondary-200 dark:border-secondary-600 sticky top-0 bg-white dark:bg-secondary-800">
							<div className="flex items-center justify-between">
								<h3 className="text-lg font-medium text-secondary-900 dark:text-white">
									Alert Details
								</h3>
								<button
									type="button"
									onClick={() => {
										setShowAlertModal(false);
										setSelectedAlert(null);
									}}
									className="text-secondary-400 hover:text-secondary-600 dark:text-white dark:hover:text-secondary-300"
								>
									<X className="h-5 w-5" />
								</button>
							</div>
						</div>
						<div className="px-6 py-4 space-y-4">
							{/* Alert Info */}
							<div className="grid grid-cols-2 gap-4">
								<div>
									<label className="text-xs font-medium text-secondary-500 dark:text-white">
										Severity
									</label>
									<div className="mt-1">
										{getSeverityBadge(selectedAlert.severity)}
									</div>
								</div>
								<div>
									<label className="text-xs font-medium text-secondary-500 dark:text-white">
										Type
									</label>
									<div className="mt-1">{getTypeBadge(selectedAlert.type)}</div>
								</div>
								<div>
									<label className="text-xs font-medium text-secondary-500 dark:text-white">
										Status
									</label>
									<div className="mt-1">{getStatusBadge(selectedAlert)}</div>
								</div>
								<div>
									<label className="text-xs font-medium text-secondary-500 dark:text-white">
										Created
									</label>
									<div className="mt-1 text-sm text-secondary-900 dark:text-white">
										{new Date(selectedAlert.created_at).toLocaleString()}
									</div>
								</div>
								<div>
									<label className="text-xs font-medium text-secondary-500 dark:text-white">
										Assigned To
									</label>
									<select
										value={selectedAlert.assigned_to_user_id || ""}
										onChange={(e) => {
											const newUserId = e.target.value;
											handleInlineAssign(selectedAlert.id, newUserId);
										}}
										className="mt-1 w-full px-3 py-2 border border-secondary-300 dark:border-secondary-600 rounded-md bg-white dark:bg-secondary-800 text-secondary-900 dark:text-white text-sm"
										disabled={
											assignAlertMutation.isPending ||
											unassignAlertMutation.isPending
										}
									>
										<option value="">Unassigned</option>
										{usersData?.map((u) => (
											<option key={u.id} value={u.id}>
												{u.username || u.email}
											</option>
										))}
									</select>
								</div>
							</div>

							{/* Title */}
							<div>
								<label className="text-xs font-medium text-secondary-500 dark:text-white">
									Title
								</label>
								<div className="mt-1 text-sm font-medium text-secondary-900 dark:text-white">
									{selectedAlert.title}
								</div>
							</div>

							{/* Message */}
							<div>
								<label className="text-xs font-medium text-secondary-500 dark:text-white">
									Message
								</label>
								<div className="mt-1 text-sm text-secondary-700 dark:text-white whitespace-pre-wrap">
									{selectedAlert.message}
								</div>
							</div>

							{/* Metadata */}
							{selectedAlert.metadata &&
								Object.keys(selectedAlert.metadata).length > 0 && (
									<div>
										<label className="text-xs font-medium text-secondary-500 dark:text-white">
											Metadata
										</label>
										<div className="mt-1 text-sm text-secondary-700 dark:text-white bg-secondary-50 dark:bg-secondary-900 p-3 rounded-md">
											<pre className="whitespace-pre-wrap">
												{JSON.stringify(selectedAlert.metadata, null, 2)}
											</pre>
										</div>
									</div>
								)}

							{/* Actions */}
							<div>
								<label className="text-xs font-medium text-secondary-500 dark:text-white mb-2 block">
									Actions
								</label>
								<div className="flex flex-wrap gap-2">
									{workflowActions.map((action) => (
										<button
											key={action.name}
											type="button"
											onClick={() => {
												handleAction(selectedAlert.id, action.name);
												setShowAlertModal(false);
												setSelectedAlert(null);
											}}
											className="btn-outline text-xs px-3 py-1"
											disabled={performActionMutation.isPending}
										>
											{action.display_name}
										</button>
									))}
									{resolutionActions.map((action) => (
										<button
											key={action.name}
											type="button"
											onClick={() => {
												handleAction(selectedAlert.id, action.name);
												setShowAlertModal(false);
												setSelectedAlert(null);
											}}
											className="btn-outline text-xs px-3 py-1 border-red-300 text-red-700 hover:bg-red-50 dark:border-red-600 dark:text-red-400 dark:hover:bg-red-900"
											disabled={performActionMutation.isPending}
										>
											{action.display_name}
										</button>
									))}
								</div>
							</div>

							{/* History */}
							<div>
								<label className="text-xs font-medium text-secondary-500 dark:text-white mb-2 block">
									History
								</label>
								<div className="space-y-2 max-h-64 overflow-y-auto">
									{alertHistory && alertHistory.length > 0 ? (
										alertHistory.map((historyItem) => (
											<div
												key={historyItem.id}
												className="flex items-start gap-3 p-2 bg-secondary-50 dark:bg-secondary-900 rounded-md"
											>
												<div className="flex-1">
													<div className="text-sm font-medium text-secondary-900 dark:text-white">
														{historyItem.action}
													</div>
													<div className="text-xs text-secondary-500 dark:text-white">
														by{" "}
														{historyItem.users?.username ||
															historyItem.users?.email ||
															"System"}
													</div>
													<div className="text-xs text-secondary-400">
														{new Date(historyItem.created_at).toLocaleString()}
													</div>
												</div>
											</div>
										))
									) : (
										<div className="text-sm text-secondary-500 dark:text-white">
											No history available
										</div>
									)}
								</div>
							</div>
						</div>
						<div className="px-6 py-4 border-t border-secondary-200 dark:border-secondary-600 flex justify-end">
							<button
								type="button"
								onClick={() => {
									setShowAlertModal(false);
									setSelectedAlert(null);
								}}
								className="px-4 py-2 text-sm font-medium text-white bg-primary-600 rounded-md hover:bg-primary-700"
							>
								Close
							</button>
						</div>
					</div>
				</div>
			)}
		</div>
	);
};

export default Reporting;
