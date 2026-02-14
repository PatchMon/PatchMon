import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
	AlertTriangle,
	ArrowDown,
	ArrowUp,
	ArrowUpDown,
	Info,
	MoreVertical,
	RefreshCw,
	Search,
	Trash2,
	X,
	XCircle,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useAuth } from "../contexts/AuthContext";
import { adminUsersAPI, alertsAPI, formatRelativeTime } from "../utils/api";

const Reporting = () => {
	const { user: _user } = useAuth();
	const queryClient = useQueryClient();
	const [searchTerm, setSearchTerm] = useState("");
	const [severityFilter, setSeverityFilter] = useState("all");
	const [typeFilter, setTypeFilter] = useState("all");
	const [statusFilter, setStatusFilter] = useState("all");
	const [assignmentFilter, setAssignmentFilter] = useState("all");
	const [sortField, setSortField] = useState("created_at");
	const [sortDirection, setSortDirection] = useState("desc");
	const [selectedAlert, setSelectedAlert] = useState(null);
	const [showAlertModal, setShowAlertModal] = useState(false);
	const [openActionMenu, setOpenActionMenu] = useState(null); // Track which alert's menu is open
	const [menuPosition, setMenuPosition] = useState({ top: 0, right: 0 }); // Position for fixed dropdown
	const menuButtonRefs = useRef({}); // Store refs for each menu button
	const [selectedAlerts, setSelectedAlerts] = useState(new Set()); // Track selected alerts for bulk operations

	// Fetch alerts - with aggressive polling for real-time updates
	const {
		data: alertsData,
		isLoading: alertsLoading,
		error: alertsError,
		refetch: refetchAlerts,
		isFetching: isFetchingAlerts,
	} = useQuery({
		queryKey: ["alerts", assignmentFilter],
		queryFn: async () => {
			const params = {};
			if (assignmentFilter === "assignedToMe") {
				params.assignedToMe = "true";
			}
			const response = await alertsAPI.getAlerts(params);
			return response.data.data || [];
		},
		refetchInterval: 30000, // Refresh every 30 seconds to reduce API load
		refetchOnWindowFocus: true, // Refetch when user returns to tab
		refetchOnMount: true, // Always refetch on mount
		staleTime: 0, // Data is immediately stale, always refetch
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

	// Get status badge from current state
	const getStatusBadge = (alert) => {
		const currentState = alert.current_state;
		if (!currentState || !currentState.action) {
			return (
				<span className="px-2 py-1 text-xs font-medium rounded bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200">
					Active
				</span>
			);
		}

		const action = currentState.action.toLowerCase();
		if (action === "done") {
			return (
				<span className="px-2 py-1 text-xs font-medium rounded bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">
					Done
				</span>
			);
		}
		if (action === "silenced") {
			return (
				<span className="px-2 py-1 text-xs font-medium rounded bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200">
					Silenced
				</span>
			);
		}
		if (action === "resolved") {
			return (
				<span className="px-2 py-1 text-xs font-medium rounded bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200">
					Resolved
				</span>
			);
		}

		return (
			<span className="px-2 py-1 text-xs font-medium rounded bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200">
				Active
			</span>
		);
	};

	// Get type badge
	const getTypeBadge = (type) => {
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
				if (!currentState || !currentState.action) {
					return statusFilter === "active";
				}
				const action = currentState.action.toLowerCase();
				if (statusFilter === "done") return action === "done";
				if (statusFilter === "silenced") return action === "silenced";
				if (statusFilter === "resolved") return action === "resolved";
				return (
					statusFilter === "active" &&
					!["done", "silenced", "resolved"].includes(action)
				);
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
		const types = new Set(alerts.map((alert) => alert.type));
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
			{/* Page Header */}
			<div className="flex items-center justify-between">
				<div>
					<h1 className="text-2xl font-semibold text-secondary-900 dark:text-white">
						Reporting
					</h1>
					<p className="text-sm text-secondary-600 dark:text-secondary-400 mt-1">
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

			{/* Stats Cards */}
			<div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
				{/* Informational Card */}
				<div className="card p-4">
					<div className="flex items-center">
						<div className="flex-shrink-0">
							<Info className="h-5 w-5 text-blue-600 mr-2" />
						</div>
						<div className="w-0 flex-1">
							<p className="text-sm text-secondary-500 dark:text-white">
								Informational
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{statsLoading ? "..." : stats.informational || 0}
							</p>
						</div>
					</div>
				</div>

				{/* Warning Card */}
				<div className="card p-4">
					<div className="flex items-center">
						<div className="flex-shrink-0">
							<AlertTriangle className="h-5 w-5 text-yellow-600 mr-2" />
						</div>
						<div className="w-0 flex-1">
							<p className="text-sm text-secondary-500 dark:text-white">
								Warning
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{statsLoading ? "..." : stats.warning || 0}
							</p>
						</div>
					</div>
				</div>

				{/* Error Card */}
				<div className="card p-4">
					<div className="flex items-center">
						<div className="flex-shrink-0">
							<XCircle className="h-5 w-5 text-orange-600 mr-2" />
						</div>
						<div className="w-0 flex-1">
							<p className="text-sm text-secondary-500 dark:text-white">
								Error
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{statsLoading ? "..." : stats.error || 0}
							</p>
						</div>
					</div>
				</div>

				{/* Critical Card */}
				<div className="card p-4">
					<div className="flex items-center">
						<div className="flex-shrink-0">
							<AlertTriangle className="h-5 w-5 text-red-600 mr-2" />
						</div>
						<div className="w-0 flex-1">
							<p className="text-sm text-secondary-500 dark:text-white">
								Critical
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{statsLoading ? "..." : stats.critical || 0}
							</p>
						</div>
					</div>
				</div>
			</div>

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
									{type.replace("_", " ")}
								</option>
							))}
						</select>

						<select
							value={statusFilter}
							onChange={(e) => setStatusFilter(e.target.value)}
							className="px-3 py-2 border border-secondary-300 dark:border-secondary-600 rounded-md bg-white dark:bg-secondary-800 text-secondary-900 dark:text-white text-sm"
						>
							<option value="all">All Status</option>
							<option value="active">Active</option>
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
					<div className="px-4 py-2 bg-primary-50 dark:bg-primary-900 border-b border-secondary-200 dark:border-secondary-700 flex items-center justify-between">
						<div className="text-sm text-secondary-700 dark:text-secondary-300">
							{selectedAlerts.size} alert(s) selected
						</div>
						<button
							type="button"
							onClick={handleDeleteSelected}
							disabled={deleteAlertsMutation.isPending}
							className="flex items-center gap-2 px-3 py-1.5 text-sm font-medium text-white bg-danger-600 hover:bg-danger-700 rounded-md disabled:opacity-50 disabled:cursor-not-allowed"
						>
							<Trash2 className="h-4 w-4" />
							Delete Selected
						</button>
					</div>
				)}
				{alertsLoading ? (
					<div className="text-center py-8">
						<div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600 mx-auto"></div>
						<p className="mt-2 text-sm text-secondary-500">Loading alerts...</p>
					</div>
				) : filteredAndSortedAlerts.length === 0 ? (
					<div className="text-center py-8">
						<AlertTriangle className="h-12 w-12 mx-auto text-secondary-400" />
						<h3 className="mt-2 text-sm font-medium text-secondary-900 dark:text-white">
							No alerts found
						</h3>
						<p className="mt-1 text-sm text-secondary-500">
							{searchTerm || severityFilter !== "all" || typeFilter !== "all"
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
										className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider"
										onClick={(e) => e.stopPropagation()}
									>
										<input
											type="checkbox"
											checked={
												selectedAlerts.size > 0 &&
												selectedAlerts.size === filteredAndSortedAlerts.length
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
										className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider cursor-pointer"
										onClick={() => handleSort("severity")}
									>
										<div className="flex items-center gap-2">
											Severity
											{getSortIcon("severity")}
										</div>
									</th>
									<th
										scope="col"
										className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider cursor-pointer"
										onClick={() => handleSort("type")}
									>
										<div className="flex items-center gap-2">
											Type
											{getSortIcon("type")}
										</div>
									</th>
									<th
										scope="col"
										className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider"
									>
										Title
									</th>
									<th
										scope="col"
										className="hidden md:table-cell px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider"
									>
										Message
									</th>
									<th
										scope="col"
										className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider"
									>
										Assigned To
									</th>
									<th
										scope="col"
										className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider"
									>
										Status
									</th>
									<th
										scope="col"
										className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider cursor-pointer"
										onClick={() => handleSort("created_at")}
									>
										<div className="flex items-center gap-2">
											Created
											{getSortIcon("created_at")}
										</div>
									</th>
									<th
										scope="col"
										className="px-4 py-2 text-right text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider"
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
											<div className="text-sm text-secondary-500 dark:text-secondary-400 max-w-md truncate">
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
											className="px-4 py-2 whitespace-nowrap text-sm text-secondary-500 dark:text-secondary-400 cursor-pointer"
											onClick={() => handleRowClick(alert)}
										>
											{formatRelativeTime(alert.created_at)}
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
							{availableActions
								?.filter((action) => action.is_state_action)
								.map((action) => (
									<button
										key={action.name}
										type="button"
										onClick={(e) => {
											e.stopPropagation();
											handleAction(openActionMenu, action.name, e);
											setOpenActionMenu(null);
										}}
										className="w-full text-left px-4 py-2 text-sm text-secondary-700 dark:text-secondary-300 hover:bg-secondary-100 dark:hover:bg-secondary-700"
										disabled={performActionMutation.isPending}
									>
										{action.display_name}
									</button>
								))}
						</div>
					</div>
				</>
			)}

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
									className="text-secondary-400 hover:text-secondary-600 dark:text-secondary-500 dark:hover:text-secondary-300"
								>
									<X className="h-5 w-5" />
								</button>
							</div>
						</div>
						<div className="px-6 py-4 space-y-4">
							{/* Alert Info */}
							<div className="grid grid-cols-2 gap-4">
								<div>
									<label className="text-xs font-medium text-secondary-500 dark:text-secondary-400">
										Severity
									</label>
									<div className="mt-1">
										{getSeverityBadge(selectedAlert.severity)}
									</div>
								</div>
								<div>
									<label className="text-xs font-medium text-secondary-500 dark:text-secondary-400">
										Type
									</label>
									<div className="mt-1">{getTypeBadge(selectedAlert.type)}</div>
								</div>
								<div>
									<label className="text-xs font-medium text-secondary-500 dark:text-secondary-400">
										Status
									</label>
									<div className="mt-1">{getStatusBadge(selectedAlert)}</div>
								</div>
								<div>
									<label className="text-xs font-medium text-secondary-500 dark:text-secondary-400">
										Created
									</label>
									<div className="mt-1 text-sm text-secondary-900 dark:text-white">
										{new Date(selectedAlert.created_at).toLocaleString()}
									</div>
								</div>
								<div>
									<label className="text-xs font-medium text-secondary-500 dark:text-secondary-400">
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
								<label className="text-xs font-medium text-secondary-500 dark:text-secondary-400">
									Title
								</label>
								<div className="mt-1 text-sm font-medium text-secondary-900 dark:text-white">
									{selectedAlert.title}
								</div>
							</div>

							{/* Message */}
							<div>
								<label className="text-xs font-medium text-secondary-500 dark:text-secondary-400">
									Message
								</label>
								<div className="mt-1 text-sm text-secondary-700 dark:text-secondary-300 whitespace-pre-wrap">
									{selectedAlert.message}
								</div>
							</div>

							{/* Metadata */}
							{selectedAlert.metadata &&
								Object.keys(selectedAlert.metadata).length > 0 && (
									<div>
										<label className="text-xs font-medium text-secondary-500 dark:text-secondary-400">
											Metadata
										</label>
										<div className="mt-1 text-sm text-secondary-700 dark:text-secondary-300 bg-secondary-50 dark:bg-secondary-900 p-3 rounded-md">
											<pre className="whitespace-pre-wrap">
												{JSON.stringify(selectedAlert.metadata, null, 2)}
											</pre>
										</div>
									</div>
								)}

							{/* Actions */}
							<div>
								<label className="text-xs font-medium text-secondary-500 dark:text-secondary-400 mb-2 block">
									Actions
								</label>
								<div className="flex flex-wrap gap-2">
									{availableActions
										?.filter((action) => action.is_state_action)
										.map((action) => (
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
								</div>
							</div>

							{/* History */}
							<div>
								<label className="text-xs font-medium text-secondary-500 dark:text-secondary-400 mb-2 block">
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
													<div className="text-xs text-secondary-500 dark:text-secondary-400">
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
										<div className="text-sm text-secondary-500 dark:text-secondary-400">
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
