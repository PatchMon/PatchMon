import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
	AlertTriangle,
	ArrowDown,
	ArrowUp,
	ArrowUpDown,
	CheckCircle,
	CheckSquare,
	ChevronDown,
	Clock,
	Columns,
	Container,
	Download,
	ExternalLink,
	Eye as EyeIcon,
	EyeOff as EyeOffIcon,
	Filter,
	FolderPlus,
	GripVertical,
	Plus,
	RefreshCw,
	RotateCcw,
	Search,
	Server,
	Shield,
	Square,
	Trash2,
	Wifi,
	X,
} from "lucide-react";
import { useEffect, useId, useMemo, useRef, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import AddHostWizard from "../components/AddHostWizard";
import InlineEdit from "../components/InlineEdit";
import InlineMultiGroupEdit from "../components/InlineMultiGroupEdit";
import InlineToggle from "../components/InlineToggle";
import {
	adminHostsAPI,
	dashboardAPI,
	formatRelativeTime,
	hostGroupsAPI,
	settingsAPI,
	userPreferencesAPI,
} from "../utils/api";
import { getOSDisplayName, OSIcon } from "../utils/osIcons.jsx";

const Hosts = () => {
	const hostGroupFilterId = useId();
	const statusFilterId = useId();
	const osFilterId = useId();
	const [showAddModal, setShowAddModal] = useState(false);
	const [selectedHosts, setSelectedHosts] = useState([]);
	const [showBulkAssignModal, setShowBulkAssignModal] = useState(false);
	const [showBulkDeleteModal, setShowBulkDeleteModal] = useState(false);
	const [bulkFetchReportMessage, setBulkFetchReportMessage] = useState({
		text: "",
		type: "success", // "success" or "error"
	});
	const [searchParams] = useSearchParams();
	const navigate = useNavigate();

	// Table state
	const [searchTerm, setSearchTerm] = useState("");
	const [sortField, setSortField] = useState("hostname");
	const [sortDirection, setSortDirection] = useState("asc");
	const [groupFilter, setGroupFilter] = useState("all");
	const [statusFilter, setStatusFilter] = useState("all");
	const [osFilter, setOsFilter] = useState("all");
	const [showFilters, setShowFilters] = useState(false);
	const [groupBy, setGroupBy] = useState("none");
	const [showColumnSettings, setShowColumnSettings] = useState(false);
	const [hideStale, setHideStale] = useState(false);

	// Handle URL filter parameters
	useEffect(() => {
		const filter = searchParams.get("filter");
		const showFiltersParam = searchParams.get("showFilters");
		const osFilterParam = searchParams.get("osFilter");
		const groupParam = searchParams.get("group");

		if (filter === "needsUpdates") {
			setShowFilters(true);
			setStatusFilter("all");
			// We'll filter hosts with updates > 0 in the filtering logic
		} else if (filter === "inactive") {
			setShowFilters(true);
			setStatusFilter("inactive");
			// We'll filter hosts with inactive status in the filtering logic
		} else if (filter === "upToDate") {
			setShowFilters(true);
			setStatusFilter("active");
			// We'll filter hosts that are up to date in the filtering logic
		} else if (filter === "stale") {
			setShowFilters(true);
			setStatusFilter("all");
			// We'll filter hosts that are stale in the filtering logic
		} else if (filter === "selected") {
			setShowFilters(true);
			setStatusFilter("all");
			// We'll filter hosts by selected hosts in the filtering logic
		} else if (showFiltersParam === "true") {
			setShowFilters(true);
		}

		// Handle OS filter parameter
		if (osFilterParam) {
			setShowFilters(true);
			setOsFilter(osFilterParam);
		}

		// Handle group filter parameter
		if (groupParam) {
			setShowFilters(true);
			setGroupFilter(groupParam);
		}

		// Handle add host action from navigation
		const action = searchParams.get("action");
		if (action === "add") {
			setShowAddModal(true);
			// Remove the action parameter from URL without triggering a page reload
			const newSearchParams = new URLSearchParams(searchParams);
			newSearchParams.delete("action");
			navigate(
				`/hosts${newSearchParams.toString() ? `?${newSearchParams.toString()}` : ""}`,
				{
					replace: true,
				},
			);
		}

		// Handle selected hosts from packages page
		const selected = searchParams.get("selected");
		if (selected) {
			const hostIds = selected.split(",").filter(Boolean);
			setSelectedHosts(hostIds);
			// Remove the selected parameter from URL without triggering a page reload
			const newSearchParams = new URLSearchParams(searchParams);
			newSearchParams.delete("selected");
			navigate(
				`/hosts${newSearchParams.toString() ? `?${newSearchParams.toString()}` : ""}`,
				{
					replace: true,
				},
			);
		}
	}, [searchParams, navigate]);

	// Default column config (shared for initial state and reset)
	const default_column_config = useMemo(
		() => [
			{ id: "select", label: "Select", visible: true, order: 0 },
			{ id: "host", label: "Friendly Name", visible: true, order: 1 },
			{ id: "hostname", label: "System Hostname", visible: true, order: 2 },
			{ id: "ip", label: "IP Address", visible: false, order: 3 },
			{ id: "group", label: "Group", visible: true, order: 4 },
			{ id: "os", label: "OS", visible: true, order: 5 },
			{ id: "os_version", label: "OS Version", visible: false, order: 6 },
			{ id: "agent_version", label: "Agent Version", visible: true, order: 7 },
			{
				id: "auto_update",
				label: "Agent Auto-Update",
				visible: true,
				order: 8,
			},
			{ id: "ws_status", label: "Connection", visible: true, order: 9 },
			{ id: "integrations", label: "Integrations", visible: true, order: 10 },
			{ id: "status", label: "Status", visible: true, order: 11 },
			{ id: "needs_reboot", label: "Reboot", visible: true, order: 12 },
			{ id: "uptime", label: "Uptime", visible: true, order: 13 },
			{ id: "updates", label: "Updates", visible: true, order: 14 },
			{
				id: "security_updates",
				label: "Security Updates",
				visible: true,
				order: 15,
			},
			{ id: "notes", label: "Notes", visible: false, order: 16 },
			{ id: "last_update", label: "Last Update", visible: true, order: 17 },
			{ id: "actions", label: "Actions", visible: true, order: 18 },
		],
		[],
	);

	// Column configuration: server-backed so it persists across browsers
	const [columnConfig, setColumnConfig] = useState(default_column_config);

	const queryClient = useQueryClient();

	// Fetch user preferences (includes hosts_column_config from server)
	const { data: user_preferences, isFetched: user_preferences_fetched } =
		useQuery({
			queryKey: ["userPreferences"],
			queryFn: () => userPreferencesAPI.get().then((res) => res.data),
			staleTime: 2 * 60 * 1000,
		});

	// Apply server or localStorage config once preferences fetch has settled (so server wins)
	const column_config_initialized = useRef(false);
	useEffect(() => {
		if (column_config_initialized.current) return;
		if (!user_preferences_fetched) return;
		// Prefer server config; if request failed or no config on server, fall back to localStorage
		const server_config = user_preferences?.hosts_column_config;
		const defaultConfig = default_column_config;
		let saved_config = null;
		if (server_config?.length) {
			saved_config = server_config;
		} else {
			const from_storage = localStorage.getItem("hosts-column-config");
			if (from_storage) {
				try {
					const parsed = JSON.parse(from_storage);
					const has_old_columns = parsed.some(
						(col) =>
							col.id === "agentVersion" ||
							col.id === "autoUpdate" ||
							col.id === "osVersion" ||
							col.id === "lastUpdate",
					);
					if (!has_old_columns) saved_config = parsed;
					else localStorage.removeItem("hosts-column-config");
				} catch {
					localStorage.removeItem("hosts-column-config");
				}
			}
		}
		column_config_initialized.current = true;
		if (!saved_config) return;
		const merged = defaultConfig.map((defaultCol) => {
			const savedCol = saved_config.find((col) => col.id === defaultCol.id);
			if (savedCol) {
				const order =
					typeof savedCol.order === "number"
						? savedCol.order
						: defaultCol.order;
				return { ...defaultCol, visible: savedCol.visible, order };
			}
			return defaultCol;
		});
		const sorted = merged
			.slice()
			.sort((a, b) => a.order - b.order)
			.map((col, index) => ({ ...col, order: index }));
		const updated = sorted.map((col) =>
			col.id === "ws_status" ? { ...col, visible: true } : col,
		);
		setColumnConfig(updated);
		// If we had only localStorage and no server config, persist to server for cross-browser sync
		if (!server_config?.length) {
			const payload = updated.map((col) => ({
				id: col.id,
				visible: col.visible,
				order: col.order,
			}));
			userPreferencesAPI
				.update({ hosts_column_config: payload })
				.catch(() => {})
				.then(() =>
					queryClient.invalidateQueries({ queryKey: ["userPreferences"] }),
				);
		}
	}, [
		user_preferences,
		user_preferences_fetched,
		default_column_config,
		queryClient,
	]);

	const {
		data: hosts,
		isLoading,
		error,
		refetch,
		isFetching,
	} = useQuery({
		queryKey: ["hosts"],
		queryFn: () => dashboardAPI.getHosts().then((res) => res.data),
		staleTime: 5 * 60 * 1000, // Data stays fresh for 5 minutes
		refetchOnWindowFocus: false, // Don't refetch when window regains focus
	});

	const { data: hostGroups } = useQuery({
		queryKey: ["hostGroups"],
		queryFn: () => hostGroupsAPI.list().then((res) => res.data),
	});

	// Fetch global settings to check if auto-update master toggle is enabled
	// Fetch settings to check global auto-update status
	// Try public endpoint first (works for all users), fallback to full settings if user has permissions
	const { data: settings } = useQuery({
		queryKey: ["settings", "public"],
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

	// State for auto-update confirmation dialog
	const [autoUpdateDialog, setAutoUpdateDialog] = useState({
		show: false,
		hostId: null,
		hostName: null,
	});

	// Track WebSocket status for all hosts
	const [wsStatusMap, setWsStatusMap] = useState({});

	// Fetch initial WebSocket status for all hosts
	useEffect(() => {
		if (!hosts || hosts.length === 0) return;

		// Fetch initial WebSocket status for all hosts
		const fetchInitialStatus = async () => {
			const apiIds = hosts
				.filter((host) => host.api_id)
				.map((host) => host.api_id);

			if (apiIds.length === 0) return;

			try {
				const response = await fetch(
					`/api/v1/ws/status?apiIds=${apiIds.join(",")}`,
					{
						credentials: "include",
					},
				);
				if (response.ok) {
					const result = await response.json();
					setWsStatusMap(result.data);
				}
			} catch (_error) {
				// Silently handle errors
			}
		};

		fetchInitialStatus();
	}, [hosts]);

	// Subscribe to WebSocket status changes for all hosts via polling (lightweight alternative to SSE)
	useEffect(() => {
		if (!hosts || hosts.length === 0) return;

		// Use polling instead of SSE to avoid connection pool issues
		// Poll every 10 seconds instead of 19 persistent connections
		const pollInterval = setInterval(() => {
			const apiIds = hosts
				.filter((host) => host.api_id)
				.map((host) => host.api_id);

			if (apiIds.length === 0) return;

			fetch(`/api/v1/ws/status?apiIds=${apiIds.join(",")}`, {
				credentials: "include",
			})
				.then((response) => response.json())
				.then((result) => {
					if (result.success && result.data) {
						setWsStatusMap(result.data);
					}
				})
				.catch(() => {
					// Silently handle errors
				});
		}, 10000); // Poll every 10 seconds

		// Cleanup function
		return () => {
			clearInterval(pollInterval);
		};
	}, [hosts]);

	const bulkUpdateGroupMutation = useMutation({
		mutationFn: ({ hostIds, groupIds }) =>
			adminHostsAPI.bulkUpdateGroups(hostIds, groupIds),
		onSuccess: (data) => {
			// Update the cache with the new host data
			if (data?.hosts) {
				queryClient.setQueryData(["hosts"], (oldData) => {
					if (!oldData) return oldData;
					return oldData.map((host) => {
						const updatedHost = data.hosts.find((h) => h.id === host.id);
						if (updatedHost) {
							return updatedHost;
						}
						return host;
					});
				});
			}

			// Also invalidate to ensure consistency
			queryClient.invalidateQueries(["hosts"]);
			setSelectedHosts([]);
			setShowBulkAssignModal(false);
		},
	});

	const updateFriendlyNameMutation = useMutation({
		mutationFn: ({ hostId, friendlyName }) =>
			adminHostsAPI
				.updateFriendlyName(hostId, friendlyName)
				.then((res) => res.data),
		onSuccess: () => {
			queryClient.invalidateQueries(["hosts"]);
		},
	});

	const updateHostGroupsMutation = useMutation({
		mutationFn: ({ hostId, groupIds }) =>
			adminHostsAPI.updateGroups(hostId, groupIds).then((res) => res.data),
		onSuccess: (data) => {
			// Update the cache with the new host data
			queryClient.setQueryData(["hosts"], (oldData) => {
				if (!oldData) return oldData;
				return oldData.map((host) =>
					host.id === data.host.id ? data.host : host,
				);
			});
			// Also invalidate to ensure consistency
			queryClient.invalidateQueries(["hosts"]);
		},
	});

	const toggleAutoUpdateMutation = useMutation({
		mutationFn: ({ hostId, autoUpdate }) =>
			adminHostsAPI
				.toggleAutoUpdate(hostId, autoUpdate)
				.then((res) => res.data),
		onSuccess: () => {
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
	const handleAutoUpdateToggle = (host, newValue) => {
		// If disabling, just do it
		if (!newValue) {
			toggleAutoUpdateMutation.mutate({
				hostId: host.id,
				autoUpdate: false,
			});
			return;
		}

		// If enabling and global is OFF, show confirmation dialog
		if (!settings?.auto_update) {
			setAutoUpdateDialog({
				show: true,
				hostId: host.id,
				hostName: host.friendly_name || host.hostname,
			});
			return;
		}

		// Global is ON, just enable the host
		toggleAutoUpdateMutation.mutate({
			hostId: host.id,
			autoUpdate: true,
		});
	};

	// Handle dialog actions
	const handleEnableBoth = () => {
		// Enable global setting first, then host
		enableGlobalAutoUpdateMutation.mutate(undefined, {
			onSuccess: () => {
				toggleAutoUpdateMutation.mutate({
					hostId: autoUpdateDialog.hostId,
					autoUpdate: true,
				});
				setAutoUpdateDialog({ show: false, hostId: null, hostName: null });
			},
		});
	};

	const handleEnableHostOnly = () => {
		// Just enable the host (user acknowledges it won't work)
		toggleAutoUpdateMutation.mutate({
			hostId: autoUpdateDialog.hostId,
			autoUpdate: true,
		});
		setAutoUpdateDialog({ show: false, hostId: null, hostName: null });
	};

	const bulkDeleteMutation = useMutation({
		mutationFn: (hostIds) => adminHostsAPI.deleteBulk(hostIds),
		onSuccess: () => {
			queryClient.invalidateQueries(["hosts"]);
			setSelectedHosts([]);
			setShowBulkDeleteModal(false);
		},
	});

	const bulkFetchReportMutation = useMutation({
		mutationFn: (hostIds) =>
			adminHostsAPI.fetchReportBulk(hostIds).then((res) => res.data),
		onSuccess: (data) => {
			queryClient.invalidateQueries(["hosts"]);
			// Show success message
			if (data?.successCount !== undefined) {
				const message = `Report fetch queued for ${data.successCount} of ${data.totalRequested} host${data.totalRequested !== 1 ? "s" : ""}`;
				setBulkFetchReportMessage({ text: message, type: "success" });
				// Clear message after 5 seconds
				setTimeout(
					() => setBulkFetchReportMessage({ text: "", type: "success" }),
					5000,
				);
			} else if (data?.message) {
				setBulkFetchReportMessage({ text: data.message, type: "success" });
				setTimeout(
					() => setBulkFetchReportMessage({ text: "", type: "success" }),
					5000,
				);
			}
		},
		onError: (error) => {
			const errorMsg =
				error.response?.data?.error ||
				error.response?.data?.details ||
				"Failed to fetch reports";
			setBulkFetchReportMessage({ text: errorMsg, type: "error" });
			setTimeout(
				() => setBulkFetchReportMessage({ text: "", type: "error" }),
				5000,
			);
		},
	});

	// Helper functions for bulk selection
	const handleSelectHost = (hostId) => {
		setSelectedHosts((prev) =>
			prev.includes(hostId)
				? prev.filter((id) => id !== hostId)
				: [...prev, hostId],
		);
	};

	const handleSelectAll = (hostsToSelect) => {
		const hostIdsToSelect = hostsToSelect.map((host) => host.id);
		const allSelected = hostIdsToSelect.every((id) =>
			selectedHosts.includes(id),
		);
		if (allSelected) {
			// Deselect all hosts in this group
			setSelectedHosts((prev) =>
				prev.filter((id) => !hostIdsToSelect.includes(id)),
			);
		} else {
			// Select all hosts in this group (merge with existing selections)
			setSelectedHosts((prev) => {
				const newSelection = [...prev];
				hostIdsToSelect.forEach((id) => {
					if (!newSelection.includes(id)) {
						newSelection.push(id);
					}
				});
				return newSelection;
			});
		}
	};

	const handleBulkAssign = (groupIds) => {
		bulkUpdateGroupMutation.mutate({ hostIds: selectedHosts, groupIds });
	};

	const handleBulkDelete = () => {
		bulkDeleteMutation.mutate(selectedHosts);
	};

	const handleBulkFetchReport = () => {
		bulkFetchReportMutation.mutate(selectedHosts);
	};

	// Table filtering and sorting logic
	const filteredAndSortedHosts = useMemo(() => {
		if (!hosts || !Array.isArray(hosts)) return [];

		const filtered = hosts.filter((host) => {
			// Search filter
			const matchesSearch =
				searchTerm === "" ||
				host.friendly_name.toLowerCase().includes(searchTerm.toLowerCase()) ||
				host.ip?.toLowerCase().includes(searchTerm.toLowerCase()) ||
				host.os_type?.toLowerCase().includes(searchTerm.toLowerCase()) ||
				host.notes?.toLowerCase().includes(searchTerm.toLowerCase());

			// Group filter - handle multiple groups per host
			const memberships = host.host_group_memberships || [];
			const matchesGroup =
				groupFilter === "all" ||
				(groupFilter === "ungrouped" && memberships.length === 0) ||
				(groupFilter !== "ungrouped" &&
					memberships.some(
						(membership) => membership.host_groups?.id === groupFilter,
					));

			// Status filter
			const matchesStatus =
				statusFilter === "all" ||
				(host.effectiveStatus || host.status) === statusFilter;

			// OS filter
			const matchesOs =
				osFilter === "all" ||
				host.os_type?.toLowerCase() === osFilter.toLowerCase();

			// URL filter for hosts needing updates, inactive hosts, up-to-date hosts, stale hosts, offline hosts, reboot required, or selected hosts
			const filter = searchParams.get("filter");
			const rebootParam = searchParams.get("reboot");
			const matchesUrlFilter =
				(filter !== "needsUpdates" ||
					(host.updatesCount && host.updatesCount > 0)) &&
				(filter !== "inactive" ||
					(host.effectiveStatus || host.status) === "inactive") &&
				(filter !== "upToDate" || (!host.isStale && host.updatesCount === 0)) &&
				(filter !== "stale" || host.isStale) &&
				(filter !== "offline" ||
					wsStatusMap[host.api_id]?.connected !== true) &&
				(filter !== "selected" || selectedHosts.includes(host.id)) &&
				(!rebootParam || host.needs_reboot === true);

			// Hide stale filter
			const matchesHideStale = !hideStale || !host.isStale;

			return (
				matchesSearch &&
				matchesGroup &&
				matchesStatus &&
				matchesOs &&
				matchesUrlFilter &&
				matchesHideStale
			);
		});

		// Sorting
		filtered.sort((a, b) => {
			let aValue, bValue;

			switch (sortField) {
				case "friendlyName":
					aValue = a.friendly_name.toLowerCase();
					bValue = b.friendly_name.toLowerCase();
					break;
				case "hostname":
					aValue = a.hostname?.toLowerCase() || "zzz_no_hostname";
					bValue = b.hostname?.toLowerCase() || "zzz_no_hostname";
					break;
				case "ip":
					aValue = a.ip?.toLowerCase() || "zzz_no_ip";
					bValue = b.ip?.toLowerCase() || "zzz_no_ip";
					break;
				case "group": {
					// Handle multiple groups per host - use first group alphabetically for sorting
					const aGroups = a.host_group_memberships || [];
					const bGroups = b.host_group_memberships || [];
					if (aGroups.length === 0) {
						aValue = "zzz_ungrouped";
					} else {
						const aGroupNames = aGroups
							.map((m) => m.host_groups?.name || "")
							.filter((name) => name)
							.sort();
						aValue = aGroupNames[0] || "zzz_ungrouped";
					}
					if (bGroups.length === 0) {
						bValue = "zzz_ungrouped";
					} else {
						const bGroupNames = bGroups
							.map((m) => m.host_groups?.name || "")
							.filter((name) => name)
							.sort();
						bValue = bGroupNames[0] || "zzz_ungrouped";
					}
					break;
				}
				case "os":
					aValue = a.os_type?.toLowerCase() || "zzz_unknown";
					bValue = b.os_type?.toLowerCase() || "zzz_unknown";
					break;
				case "os_version":
					aValue = a.os_version?.toLowerCase() || "zzz_unknown";
					bValue = b.os_version?.toLowerCase() || "zzz_unknown";
					break;
				case "agent_version":
					aValue = a.agent_version?.toLowerCase() || "zzz_no_version";
					bValue = b.agent_version?.toLowerCase() || "zzz_no_version";
					break;
				case "status":
					aValue = a.effectiveStatus || a.status;
					bValue = b.effectiveStatus || b.status;
					break;
				case "updates":
					aValue = a.updatesCount || 0;
					bValue = b.updatesCount || 0;
					break;
				case "security_updates":
					aValue = a.securityUpdatesCount || 0;
					bValue = b.securityUpdatesCount || 0;
					break;
				case "needs_reboot":
					// Sort by boolean: false (0) comes before true (1)
					aValue = a.needs_reboot ? 1 : 0;
					bValue = b.needs_reboot ? 1 : 0;
					break;
				case "uptime": {
					// Parse uptime strings like "X days, Y hours, Z minutes" into total minutes for numeric sorting
					const parseUptimeToMinutes = (uptimeStr) => {
						// Handle null, undefined, empty string, or "Unknown"
						if (
							!uptimeStr ||
							uptimeStr.trim() === "" ||
							uptimeStr.toLowerCase() === "unknown"
						) {
							return -1; // Sort invalid/missing uptime to the end
						}

						let total = 0;
						// Match patterns: "X days", "X day", "X hours", "X hour", "X minutes", "X minute"
						// Case insensitive, flexible whitespace
						const daysMatch = uptimeStr.match(/(\d+)\s+days?/i);
						const hoursMatch = uptimeStr.match(/(\d+)\s+hours?/i);
						const minutesMatch = uptimeStr.match(/(\d+)\s+minutes?/i);

						if (daysMatch) total += parseInt(daysMatch[1], 10) * 1440;
						if (hoursMatch) total += parseInt(hoursMatch[1], 10) * 60;
						if (minutesMatch) total += parseInt(minutesMatch[1], 10);

						// If no matches found, return -1 to sort to the end
						return total > 0 ? total : -1;
					};
					aValue = parseUptimeToMinutes(a.system_uptime);
					bValue = parseUptimeToMinutes(b.system_uptime);
					break;
				}
				case "last_update":
					aValue = new Date(a.last_update);
					bValue = new Date(b.last_update);
					break;
				case "notes":
					aValue = (a.notes || "").toLowerCase();
					bValue = (b.notes || "").toLowerCase();
					break;
				case "integrations": {
					// Sort by integration count: both=2, one=1, none=0
					const aScore =
						(a.docker_enabled ? 1 : 0) + (a.compliance_enabled ? 1 : 0);
					const bScore =
						(b.docker_enabled ? 1 : 0) + (b.compliance_enabled ? 1 : 0);
					aValue = aScore;
					bValue = bScore;
					break;
				}
				default:
					aValue = a[sortField];
					bValue = b[sortField];
			}

			if (aValue < bValue) return sortDirection === "asc" ? -1 : 1;
			if (aValue > bValue) return sortDirection === "asc" ? 1 : -1;
			return 0;
		});

		return filtered;
	}, [
		hosts,
		searchTerm,
		groupFilter,
		statusFilter,
		osFilter,
		sortField,
		sortDirection,
		searchParams,
		hideStale,
		wsStatusMap,
		selectedHosts,
	]);

	// Get unique OS types from hosts for dynamic dropdown
	const uniqueOsTypes = useMemo(() => {
		if (!hosts) return [];
		const osTypes = new Set();
		hosts.forEach((host) => {
			if (host.os_type) {
				osTypes.add(host.os_type);
			}
		});
		return Array.from(osTypes).sort();
	}, [hosts]);

	// Group hosts by selected field
	const groupedHosts = useMemo(() => {
		if (groupBy === "none") {
			return { "All Hosts": filteredAndSortedHosts };
		}

		const groups = {};
		filteredAndSortedHosts.forEach((host) => {
			if (groupBy === "group") {
				// Handle multiple groups per host
				const memberships = host.host_group_memberships || [];
				if (memberships.length === 0) {
					// Host has no groups, add to "Ungrouped"
					if (!groups.Ungrouped) {
						groups.Ungrouped = [];
					}
					groups.Ungrouped.push(host);
				} else {
					// Host has one or more groups, add to each group
					memberships.forEach((membership) => {
						const groupName = membership.host_groups?.name || "Unknown";
						if (!groups[groupName]) {
							groups[groupName] = [];
						}
						groups[groupName].push(host);
					});
				}
			} else {
				// Other grouping types (status, os, etc.)
				let groupKey;
				switch (groupBy) {
					case "status":
						groupKey =
							(host.effectiveStatus || host.status).charAt(0).toUpperCase() +
							(host.effectiveStatus || host.status).slice(1);
						break;
					case "os":
						groupKey = host.os_type || "Unknown";
						break;
					default:
						groupKey = "All Hosts";
				}

				if (!groups[groupKey]) {
					groups[groupKey] = [];
				}
				groups[groupKey].push(host);
			}
		});

		return groups;
	}, [filteredAndSortedHosts, groupBy]);

	const handleSort = (field) => {
		if (sortField === field) {
			setSortDirection(sortDirection === "asc" ? "desc" : "asc");
		} else {
			setSortField(field);
			setSortDirection("asc");
		}
	};

	const getSortIcon = (field) => {
		if (sortField !== field) return <ArrowUpDown className="h-4 w-4" />;
		return sortDirection === "asc" ? (
			<ArrowUp className="h-4 w-4" />
		) : (
			<ArrowDown className="h-4 w-4" />
		);
	};

	// Column management functions (persist to server so config is shared across browsers)
	const updateColumnConfig = (newConfig) => {
		setColumnConfig(newConfig);
		localStorage.setItem("hosts-column-config", JSON.stringify(newConfig));
		const payload = newConfig.map((col) => ({
			id: col.id,
			visible: col.visible,
			order: col.order,
		}));
		userPreferencesAPI
			.update({ hosts_column_config: payload })
			.catch(() => {})
			.then(() =>
				queryClient.invalidateQueries({ queryKey: ["userPreferences"] }),
			);
	};

	const toggleColumnVisibility = (columnId) => {
		const newConfig = columnConfig.map((col) =>
			col.id === columnId ? { ...col, visible: !col.visible } : col,
		);
		updateColumnConfig(newConfig);
	};

	const reorderColumns = (fromIndex, toIndex) => {
		const newConfig = [...columnConfig];
		const [movedColumn] = newConfig.splice(fromIndex, 1);
		newConfig.splice(toIndex, 0, movedColumn);

		// Update order values
		const updatedConfig = newConfig.map((col, index) => ({
			...col,
			order: index,
		}));
		updateColumnConfig(updatedConfig);
	};

	const resetColumns = () => {
		updateColumnConfig([...default_column_config]);
	};

	// Get visible columns in order
	const visibleColumns = columnConfig
		.filter((col) => col.visible)
		.sort((a, b) => a.order - b.order);

	const renderOsWithVersionTooltip = (host, textClassName) => {
		const osVersion =
			typeof host.os_version === "string" && host.os_version.trim()
				? host.os_version.trim()
				: "OS version unavailable";

		return (
			<div
				className={`group relative inline-flex items-center gap-2 text-sm cursor-help ${textClassName}`}
			>
				<OSIcon osType={host.os_type} className="h-4 w-4" />
				<span>{getOSDisplayName(host.os_type)}</span>

				<div
					role="tooltip"
					className="pointer-events-none absolute left-1/2 top-full z-30 mt-2 w-max max-w-sm -translate-x-1/2 rounded-lg border border-secondary-200 bg-secondary-50 px-3 py-2 text-left text-xs text-secondary-700 shadow-lg opacity-0 transition-opacity duration-150 group-hover:opacity-100 dark:border-secondary-400 dark:bg-secondary-700 dark:text-secondary-200"
				>
					<div className="absolute -top-1 left-1/2 h-2 w-2 -translate-x-1/2 rotate-45 border-l border-t border-secondary-200 bg-secondary-50 dark:border-secondary-400 dark:bg-secondary-700" />
					<p className="font-medium text-secondary-900 dark:text-white">
						OS Version
					</p>
					<p className="mt-0.5 break-words">{osVersion}</p>
				</div>
			</div>
		);
	};

	// Helper function to render table cell content
	const renderCellContent = (column, host) => {
		switch (column.id) {
			case "select":
				return (
					<button
						type="button"
						onClick={() => handleSelectHost(host.id)}
						className="flex items-center gap-2 hover:text-secondary-700"
					>
						{selectedHosts.includes(host.id) ? (
							<CheckSquare className="h-4 w-4 text-primary-600" />
						) : (
							<Square className="h-4 w-4 text-secondary-400" />
						)}
					</button>
				);
			case "host":
				return (
					<InlineEdit
						value={host.friendly_name}
						onSave={(newName) =>
							updateFriendlyNameMutation.mutate({
								hostId: host.id,
								friendlyName: newName,
							})
						}
						placeholder="Enter friendly name..."
						maxLength={100}
						linkTo={`/hosts/${host.id}`}
						validate={(value) => {
							if (!value.trim()) return "Friendly name is required";
							if (value.trim().length < 1)
								return "Friendly name must be at least 1 character";
							if (value.trim().length > 100)
								return "Friendly name must be less than 100 characters";
							return null;
						}}
						className="w-full"
					/>
				);
			case "hostname":
				return (
					<div className="text-sm text-secondary-900 dark:text-white font-mono">
						{host.hostname || "N/A"}
					</div>
				);
			case "ip":
				return (
					<div className="text-sm text-secondary-900 dark:text-white">
						{host.ip || "N/A"}
					</div>
				);
			case "group": {
				// Extract group IDs from the new many-to-many structure
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
			}
			case "os":
				return renderOsWithVersionTooltip(
					host,
					"text-secondary-900 dark:text-white",
				);
			case "os_version":
				return (
					<div className="text-sm text-secondary-900 dark:text-white">
						{host.os_version || "N/A"}
					</div>
				);
			case "agent_version":
				return (
					<div className="text-sm text-secondary-900 dark:text-white">
						{host.agent_version || "N/A"}
					</div>
				);
			case "auto_update":
				return (
					<div className="flex items-center gap-1">
						<InlineToggle
							value={host.auto_update}
							onSave={(autoUpdate) => handleAutoUpdateToggle(host, autoUpdate)}
							trueLabel="Yes"
							falseLabel="No"
						/>
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
				);
			case "ws_status": {
				const wsStatus = wsStatusMap[host.api_id];
				if (!wsStatus) {
					return (
						<span className="inline-flex items-center px-2 py-1 rounded-md text-xs font-medium bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400">
							<div className="w-2 h-2 bg-gray-400 rounded-full mr-1.5"></div>
							Unknown
						</span>
					);
				}
				return (
					<span
						className={`inline-flex items-center px-2 py-1 rounded-md text-xs font-medium ${
							wsStatus.connected
								? "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200"
								: "bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200"
						}`}
						title={
							wsStatus.connected
								? `Agent connected via ${wsStatus.secure ? "WSS (secure)" : "WS (insecure)"}`
								: "Agent not connected"
						}
					>
						{wsStatus.connected && (
							<div className="w-2 h-2 rounded-full mr-1.5 bg-green-500 animate-pulse"></div>
						)}
						<span>
							{wsStatus.connected
								? wsStatus.secure
									? "WSS"
									: "WS"
								: "Offline"}
						</span>
					</span>
				);
			}
			case "integrations":
				return (
					<div className="flex items-center gap-1">
						{host.docker_enabled && (
							<span
								className="inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200"
								title="Docker monitoring enabled"
							>
								<Container className="h-3 w-3" />
							</span>
						)}
						{host.compliance_enabled && (
							<span
								className="inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200"
								title="Compliance scanning enabled"
							>
								<Shield className="h-3 w-3" />
							</span>
						)}
						{!host.docker_enabled && !host.compliance_enabled && (
							<span className="text-xs text-secondary-400 dark:text-secondary-500">
								—
							</span>
						)}
					</div>
				);
			case "status":
				return (
					<div className="text-sm text-secondary-900 dark:text-white">
						{(host.effectiveStatus || host.status).charAt(0).toUpperCase() +
							(host.effectiveStatus || host.status).slice(1)}
					</div>
				);
			case "needs_reboot":
				return (
					<div className="flex justify-center">
						{host.needs_reboot ? (
							<span className="inline-flex items-center gap-1 px-2 py-1 rounded-md text-xs font-medium bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200">
								<RotateCcw className="h-3 w-3" />
								Required
							</span>
						) : (
							<span className="inline-flex items-center gap-1 px-2 py-1 rounded-md text-xs font-medium bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">
								<CheckCircle className="h-3 w-3" />
								No
							</span>
						)}
					</div>
				);
			case "uptime":
				return (
					<div className="text-sm text-secondary-900 dark:text-white">
						{host.system_uptime || "N/A"}
					</div>
				);
			case "updates":
				return (
					<button
						type="button"
						onClick={() =>
							navigate(`/packages?host=${host.id}&filter=outdated`)
						}
						className="text-sm text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300 font-medium hover:underline"
						title="View outdated packages for this host"
					>
						{host.updatesCount || 0}
					</button>
				);
			case "security_updates":
				return (
					<button
						type="button"
						onClick={() =>
							navigate(`/packages?host=${host.id}&filter=security-updates`)
						}
						className="text-sm text-red-600 hover:text-red-900 dark:text-red-400 dark:hover:text-red-300 font-medium hover:underline"
						title="View security updates for this host"
					>
						{host.securityUpdatesCount || 0}
					</button>
				);
			case "last_update":
				return (
					<div className="text-sm text-secondary-500 dark:text-secondary-300">
						{formatRelativeTime(host.last_update)}
					</div>
				);
			case "notes":
				return (
					<div className="text-sm text-secondary-900 dark:text-white max-w-xs">
						{host.notes ? (
							<div className="truncate" title={host.notes}>
								{host.notes}
							</div>
						) : (
							<span className="text-secondary-400 dark:text-secondary-500 italic">
								No notes
							</span>
						)}
					</div>
				);
			case "actions":
				return (
					<Link
						to={`/hosts/${host.id}`}
						className="text-primary-600 hover:text-primary-900 flex items-center gap-1"
					>
						View
						<ExternalLink className="h-3 w-3" />
					</Link>
				);
			default:
				return null;
		}
	};

	// Stats card click handlers
	const handleTotalHostsClick = () => {
		// Clear all filters to show all hosts
		setSearchTerm("");
		setGroupFilter("all");
		setStatusFilter("all");
		setOsFilter("all");
		setGroupBy("none");
		setHideStale(false);
		setShowFilters(false);
		// Clear URL parameters to ensure no filters are applied
		navigate("/hosts", { replace: true });
	};

	const handleNeedsUpdatesClick = () => {
		// Filter to show hosts needing updates (regardless of status)
		setStatusFilter("all");
		setShowFilters(true);
		// Clear conflicting filters and set needsUpdates filter
		const newSearchParams = new URLSearchParams(window.location.search);
		newSearchParams.set("filter", "needsUpdates");
		newSearchParams.delete("reboot"); // Clear reboot filter when switching to needsUpdates
		navigate(`/hosts?${newSearchParams.toString()}`, { replace: true });
	};

	const handleConnectionStatusClick = () => {
		// Filter to show offline hosts (not connected via WebSocket)
		setStatusFilter("all");
		setShowFilters(true);
		// Clear conflicting filters and set offline filter
		const newSearchParams = new URLSearchParams(window.location.search);
		newSearchParams.set("filter", "offline");
		newSearchParams.delete("reboot"); // Clear reboot filter when switching to offline
		navigate(`/hosts?${newSearchParams.toString()}`, { replace: true });
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
			<div className="bg-danger-50 border border-danger-200 rounded-md p-4">
				<div className="flex">
					<AlertTriangle className="h-5 w-5 text-danger-400" />
					<div className="ml-3">
						<h3 className="text-sm font-medium text-danger-800">
							Error loading hosts
						</h3>
						<p className="text-sm text-danger-700 mt-1">
							{error.message || "Failed to load hosts"}
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
		);
	}

	return (
		<div className="min-h-0 flex flex-col md:h-[calc(100vh-7rem)] md:overflow-hidden">
			{/* Page Header */}
			<div className="flex items-center justify-between mb-6">
				<div>
					<h1 className="text-2xl font-semibold text-secondary-900 dark:text-white">
						Hosts
					</h1>
					<p className="text-sm text-secondary-600 dark:text-white/80 mt-1">
						Manage and monitor your connected hosts
					</p>
				</div>
				<div className="flex items-center gap-3">
					<button
						type="button"
						onClick={() => refetch()}
						disabled={isFetching}
						className="btn-outline flex items-center justify-center p-2"
						title="Refresh hosts data"
					>
						<RefreshCw
							className={`h-4 w-4 ${isFetching ? "animate-spin" : ""}`}
						/>
					</button>
					<button
						type="button"
						onClick={() => setShowAddModal(true)}
						className="btn-primary flex items-center gap-2"
					>
						<Plus className="h-4 w-4" />
						Add Host
					</button>
				</div>
			</div>

			{/* Stats Summary */}
			<div className="grid grid-cols-2 sm:grid-cols-4 gap-3 sm:gap-4 mb-6">
				<button
					type="button"
					className="card p-4 cursor-pointer hover:shadow-card-hover dark:hover:shadow-card-hover-dark transition-shadow duration-200 text-left w-full"
					onClick={handleTotalHostsClick}
				>
					<div className="flex items-center">
						<Server className="h-5 w-5 text-primary-600 mr-2" />
						<div>
							<p className="text-sm text-secondary-500 dark:text-white">
								Total Hosts
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{hosts?.length || 0}
							</p>
						</div>
					</div>
				</button>
				<button
					type="button"
					className="card p-4 cursor-pointer hover:shadow-card-hover dark:hover:shadow-card-hover-dark transition-shadow duration-200 text-left w-full"
					onClick={handleNeedsUpdatesClick}
				>
					<div className="flex items-center">
						<Clock className="h-5 w-5 text-warning-600 mr-2" />
						<div>
							<p className="text-sm text-secondary-500 dark:text-white">
								Needs Updates
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{hosts?.filter((h) => h.updatesCount > 0).length || 0}
							</p>
						</div>
					</div>
				</button>
				<button
					type="button"
					className="card p-4 cursor-pointer hover:shadow-card-hover dark:hover:shadow-card-hover-dark transition-shadow duration-200 text-left w-full"
					onClick={() => {
						const newSearchParams = new URLSearchParams();
						newSearchParams.set("reboot", "true");
						// Clear filter parameter when setting reboot filter
						navigate(`/hosts?${newSearchParams.toString()}`, { replace: true });
					}}
				>
					<div className="flex items-center">
						<RotateCcw className="h-5 w-5 text-orange-600 mr-2" />
						<div>
							<p className="text-sm text-secondary-500 dark:text-white">
								Needs Reboots
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{hosts?.filter((h) => h.needs_reboot === true).length || 0}
							</p>
						</div>
					</div>
				</button>
				<button
					type="button"
					className="card p-4 cursor-pointer hover:shadow-card-hover dark:hover:shadow-card-hover-dark transition-shadow duration-200 text-left w-full"
					onClick={handleConnectionStatusClick}
				>
					<div className="flex items-center">
						<Wifi className="h-5 w-5 text-primary-600 mr-2" />
						<div className="flex-1">
							<p className="text-sm text-secondary-500 dark:text-white mb-1">
								Connection Status
							</p>
							{(() => {
								const connectedCount =
									hosts?.filter(
										(h) => wsStatusMap[h.api_id]?.connected === true,
									).length || 0;
								const offlineCount =
									hosts?.filter(
										(h) => wsStatusMap[h.api_id]?.connected !== true,
									).length || 0;
								return (
									<div className="flex gap-4">
										<div className="flex items-center gap-1">
											<div className="w-2 h-2 bg-green-500 rounded-full"></div>
											<span className="text-sm font-medium text-secondary-900 dark:text-white">
												{connectedCount}
											</span>
											<span className="text-xs text-secondary-500 dark:text-secondary-400 hidden sm:inline">
												Connected
											</span>
										</div>
										<div className="flex items-center gap-1">
											<div className="w-2 h-2 bg-red-500 rounded-full"></div>
											<span className="text-sm font-medium text-secondary-900 dark:text-white">
												{offlineCount}
											</span>
											<span className="text-xs text-secondary-500 dark:text-secondary-400 hidden sm:inline">
												Offline
											</span>
										</div>
									</div>
								);
							})()}
						</div>
					</div>
				</button>
			</div>

			{/* Hosts List */}
			<div className="card flex-1 flex flex-col md:overflow-hidden min-h-0">
				<div className="px-4 py-4 sm:p-4 flex-1 flex flex-col md:overflow-hidden min-h-0">
					<div className="flex flex-col sm:flex-row sm:items-center sm:justify-end gap-3 mb-4">
						{bulkFetchReportMessage.text && (
							<div
								className={`flex items-center gap-2 px-4 py-2 rounded-lg text-sm ${
									bulkFetchReportMessage.type === "success"
										? "bg-green-50 dark:bg-green-900/30 border border-green-200 dark:border-green-700 text-green-800 dark:text-green-200"
										: "bg-red-50 dark:bg-red-900/30 border border-red-200 dark:border-red-700 text-red-800 dark:text-red-200"
								}`}
							>
								{bulkFetchReportMessage.type === "success" ? (
									<CheckCircle className="h-4 w-4" />
								) : (
									<AlertTriangle className="h-4 w-4" />
								)}
								<span>{bulkFetchReportMessage.text}</span>
							</div>
						)}
						{selectedHosts.length > 0 && (
							<div className="flex flex-wrap items-center gap-2 sm:gap-3">
								<span className="text-sm text-secondary-600 dark:text-white/80 flex-shrink-0">
									{selectedHosts.length} host
									{selectedHosts.length !== 1 ? "s" : ""} selected
								</span>
								<button
									type="button"
									onClick={handleBulkFetchReport}
									disabled={bulkFetchReportMutation.isPending}
									className="btn-outline flex items-center gap-1.5 sm:gap-2 px-3 sm:px-4 py-2 min-h-[44px] text-xs sm:text-sm"
									title="Fetch reports from selected hosts"
								>
									<Download
										className={`h-4 w-4 flex-shrink-0 ${
											bulkFetchReportMutation.isPending ? "animate-spin" : ""
										}`}
									/>
									<span className="hidden sm:inline">Fetch Reports</span>
									<span className="sm:hidden">Fetch</span>
								</button>
								<button
									type="button"
									onClick={() => setShowBulkAssignModal(true)}
									className="btn-outline flex items-center gap-1.5 sm:gap-2 px-3 sm:px-4 py-2 min-h-[44px] text-xs sm:text-sm"
								>
									<FolderPlus className="h-4 w-4 flex-shrink-0" />
									<span className="hidden sm:inline">Assign to Group</span>
									<span className="sm:hidden">Assign</span>
								</button>
								<button
									type="button"
									onClick={() => setShowBulkDeleteModal(true)}
									className="btn-danger flex items-center gap-1.5 sm:gap-2 px-3 sm:px-4 py-2 min-h-[44px] text-xs sm:text-sm"
								>
									<Trash2 className="h-4 w-4 flex-shrink-0" />
									<span>Delete</span>
								</button>
								<button
									type="button"
									onClick={() => setSelectedHosts([])}
									className="text-xs sm:text-sm text-secondary-500 dark:text-white/70 hover:text-secondary-700 dark:hover:text-white/90 min-h-[44px] px-2"
								>
									<span className="hidden sm:inline">Clear Selection</span>
									<span className="sm:hidden">Clear</span>
								</button>
							</div>
						)}
					</div>

					{/* Table Controls */}
					<div className="mb-4 space-y-4">
						{/* Search and Filter Bar */}
						<div className="flex flex-col sm:flex-row gap-4">
							<div className="flex-1">
								<div className="relative">
									<Search className="absolute left-3 top-1/2 transform -translate-y-1/2 h-4 w-4 text-secondary-400 dark:text-secondary-500" />
									<input
										type="text"
										placeholder="Search hosts, IP addresses, or OS..."
										value={searchTerm}
										onChange={(e) => setSearchTerm(e.target.value)}
										className="pl-10 pr-4 py-2 w-full border border-secondary-300 dark:border-secondary-600 rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-800 text-secondary-900 dark:text-white placeholder-secondary-500 dark:placeholder-secondary-400"
									/>
								</div>
							</div>
							<div className="flex flex-wrap gap-2">
								<button
									type="button"
									onClick={() => setShowFilters(!showFilters)}
									className={`btn-outline flex items-center gap-1.5 sm:gap-2 px-2 sm:px-4 py-2 min-h-[44px] text-xs sm:text-sm ${showFilters ? "bg-primary-50 border-primary-300" : ""}`}
								>
									<Filter className="h-4 w-4 flex-shrink-0" />
									<span className="hidden sm:inline">Filters</span>
								</button>
								<button
									type="button"
									onClick={() => setShowColumnSettings(true)}
									className="btn-outline flex items-center gap-1.5 sm:gap-2 px-2 sm:px-4 py-2 min-h-[44px] text-xs sm:text-sm"
								>
									<Columns className="h-4 w-4 flex-shrink-0" />
									<span className="hidden sm:inline">Columns</span>
								</button>
								<div className="relative">
									<select
										value={groupBy}
										onChange={(e) => setGroupBy(e.target.value)}
										className="appearance-none bg-white dark:bg-secondary-800 border-2 border-secondary-300 dark:border-secondary-600 rounded-lg px-2 py-2 pr-6 text-xs sm:text-sm focus:ring-2 focus:ring-primary-500 focus:border-primary-500 text-secondary-900 dark:text-white hover:border-secondary-400 dark:hover:border-secondary-500 transition-colors min-w-[100px] sm:min-w-[120px] min-h-[44px]"
									>
										<option value="none">No Grouping</option>
										<option value="group">By Group</option>
										<option value="status">By Status</option>
										<option value="os">By OS</option>
									</select>
									<ChevronDown className="absolute right-1 top-1/2 transform -translate-y-1/2 h-4 w-4 text-secondary-400 dark:text-secondary-500 pointer-events-none" />
								</div>
								<button
									type="button"
									onClick={() => setHideStale(!hideStale)}
									className={`btn-outline flex items-center gap-1.5 sm:gap-2 px-2 sm:px-4 py-2 min-h-[44px] text-xs sm:text-sm ${hideStale ? "bg-primary-50 border-primary-300" : ""}`}
								>
									<AlertTriangle className="h-4 w-4 flex-shrink-0" />
									<span className="hidden sm:inline">Hide Stale</span>
								</button>
							</div>
						</div>

						{/* Advanced Filters */}
						{showFilters && (
							<div className="bg-secondary-50 dark:bg-secondary-700 p-3 sm:p-4 rounded-lg border dark:border-secondary-600">
								<div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3 sm:gap-4">
									<div>
										<label
											htmlFor={hostGroupFilterId}
											className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
										>
											Host Group
										</label>
										<select
											id={hostGroupFilterId}
											value={groupFilter}
											onChange={(e) => setGroupFilter(e.target.value)}
											className="w-full border border-secondary-300 dark:border-secondary-600 rounded-lg px-3 py-2.5 sm:py-2 focus:ring-2 focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-800 text-secondary-900 dark:text-white min-h-[44px]"
										>
											<option value="all">All Groups</option>
											<option value="ungrouped">Ungrouped</option>
											{hostGroups?.map((group) => (
												<option key={group.id} value={group.id}>
													{group.name}
												</option>
											))}
										</select>
									</div>
									<div>
										<label
											htmlFor={statusFilterId}
											className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
										>
											Status
										</label>
										<select
											id={statusFilterId}
											value={statusFilter}
											onChange={(e) => setStatusFilter(e.target.value)}
											className="w-full border border-secondary-300 dark:border-secondary-600 rounded-lg px-3 py-2.5 sm:py-2 focus:ring-2 focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-800 text-secondary-900 dark:text-white min-h-[44px]"
										>
											<option value="all">All Status</option>
											<option value="active">Active</option>
											<option value="pending">Pending</option>
											<option value="inactive">Inactive</option>
											<option value="error">Error</option>
										</select>
									</div>
									<div>
										<label
											htmlFor={osFilterId}
											className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
										>
											Operating System
										</label>
										<select
											id={osFilterId}
											value={osFilter}
											onChange={(e) => setOsFilter(e.target.value)}
											className="w-full border border-secondary-300 dark:border-secondary-600 rounded-lg px-3 py-2.5 sm:py-2 focus:ring-2 focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-800 text-secondary-900 dark:text-white min-h-[44px]"
										>
											<option value="all">All OS</option>
											{uniqueOsTypes.map((osType) => (
												<option key={osType} value={osType.toLowerCase()}>
													{osType}
												</option>
											))}
										</select>
									</div>
									<div className="flex items-end">
										<button
											type="button"
											onClick={() => {
												setSearchTerm("");
												setGroupFilter("all");
												setStatusFilter("all");
												setOsFilter("all");
												setGroupBy("none");
												setHideStale(false);
											}}
											className="btn-outline w-full min-h-[44px]"
										>
											Clear Filters
										</button>
									</div>
								</div>
							</div>
						)}
					</div>

					<div className="flex-1 md:overflow-hidden">
						{!hosts || hosts.length === 0 ? (
							<div className="text-center py-8">
								<Server className="h-12 w-12 text-secondary-400 mx-auto mb-4" />
								<p className="text-secondary-500">No hosts registered yet</p>
								<p className="text-sm text-secondary-400 mt-2">
									Click "Add Host" to manually register a new host and get API
									credentials
								</p>
							</div>
						) : filteredAndSortedHosts.length === 0 ? (
							<div className="text-center py-8">
								<Search className="h-12 w-12 text-secondary-400 mx-auto mb-4" />
								<p className="text-secondary-500">
									No hosts match your current filters
								</p>
								<p className="text-sm text-secondary-400 mt-2">
									Try adjusting your search terms or filters to see more results
								</p>
							</div>
						) : (
							<div className="md:h-full overflow-auto">
								<div className="space-y-6">
									{Object.entries(groupedHosts).map(
										([groupName, groupHosts]) => (
											<div key={groupName} className="space-y-3">
												{/* Group Header */}
												{groupBy !== "none" && (
													<div className="flex items-center justify-between bg-secondary-100 dark:bg-secondary-700 px-4 py-2 rounded-lg">
														<h3 className="text-sm font-medium text-secondary-900 dark:text-white">
															{groupName} ({groupHosts.length})
														</h3>
													</div>
												)}

												{/* Mobile Card Layout */}
												<div className="md:hidden space-y-3">
													{groupHosts.map((host) => {
														const isInactive =
															(host.effectiveStatus || host.status) ===
															"inactive";
														const isSelected = selectedHosts.includes(host.id);
														const wsStatus = wsStatusMap[host.api_id];
														const groupIds =
															host.host_group_memberships?.map(
																(membership) => membership.host_groups.id,
															) || [];
														const groups =
															hostGroups?.filter((g) =>
																groupIds.includes(g.id),
															) || [];

														return (
															<div
																key={host.id}
																className={`card p-4 space-y-3 ${
																	isSelected
																		? "ring-2 ring-primary-500 bg-primary-50 dark:bg-primary-900/20"
																		: isInactive
																			? "bg-red-50 dark:bg-red-900/20"
																			: ""
																}`}
															>
																{/* Header with select and main info */}
																<div className="flex items-start justify-between gap-3">
																	<div className="flex items-center gap-2 flex-1 min-w-0">
																		{visibleColumns.some(
																			(col) => col.id === "select",
																		) && (
																			<button
																				type="button"
																				onClick={() =>
																					handleSelectHost(host.id)
																				}
																				className="flex-shrink-0 min-w-[44px] min-h-[44px] flex items-center justify-center"
																			>
																				{isSelected ? (
																					<CheckSquare className="h-5 w-5 text-primary-600" />
																				) : (
																					<Square className="h-5 w-5 text-secondary-400" />
																				)}
																			</button>
																		)}
																		<div className="flex-1 min-w-0">
																			{visibleColumns.some(
																				(col) => col.id === "host",
																			) && (
																				<Link
																					to={`/hosts/${host.id}`}
																					className="text-base font-semibold text-secondary-900 dark:text-white hover:text-primary-600 dark:hover:text-primary-400 block truncate"
																				>
																					{host.friendly_name || "Unnamed Host"}
																				</Link>
																			)}
																			{visibleColumns.some(
																				(col) => col.id === "hostname",
																			) &&
																				host.hostname && (
																					<div className="text-sm text-secondary-500 dark:text-secondary-400 font-mono truncate">
																						{host.hostname}
																					</div>
																				)}
																		</div>
																	</div>
																	{visibleColumns.some(
																		(col) => col.id === "actions",
																	) && (
																		<Link
																			to={`/hosts/${host.id}`}
																			className="btn-primary text-sm px-3 py-2 min-h-[44px] flex items-center gap-1 flex-shrink-0"
																		>
																			View
																			<ExternalLink className="h-4 w-4" />
																		</Link>
																	)}
																</div>

																{/* OS, Status and connection info */}
																<div className="flex items-center justify-between gap-2">
																	{visibleColumns.some(
																		(col) => col.id === "os",
																	) &&
																		renderOsWithVersionTooltip(
																			host,
																			"text-secondary-700 dark:text-secondary-300",
																		)}
																	<div className="flex flex-wrap items-center gap-2">
																		{visibleColumns.some(
																			(col) => col.id === "status",
																		) && (
																			<span className="inline-flex items-center px-2 py-1 rounded-md text-xs font-medium bg-secondary-100 text-secondary-700 dark:bg-secondary-700 dark:text-secondary-300">
																				{(host.effectiveStatus || host.status)
																					.charAt(0)
																					.toUpperCase() +
																					(
																						host.effectiveStatus || host.status
																					).slice(1)}
																			</span>
																		)}
																		{visibleColumns.some(
																			(col) => col.id === "ws_status",
																		) &&
																			wsStatus && (
																				<span
																					className={`inline-flex items-center px-2 py-1 rounded-md text-xs font-medium ${
																						wsStatus.connected
																							? "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200"
																							: "bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200"
																					}`}
																				>
																					{wsStatus.connected && (
																						<div className="w-2 h-2 rounded-full mr-1.5 bg-green-500 animate-pulse"></div>
																					)}
																					<span>
																						{wsStatus.connected
																							? wsStatus.secure
																								? "WSS"
																								: "WS"
																							: "Offline"}
																					</span>
																				</span>
																			)}
																	</div>
																</div>

																{/* Reboot Required */}
																{visibleColumns.some(
																	(col) => col.id === "needs_reboot",
																) &&
																	host.needs_reboot && (
																		<div>
																			<span
																				className="inline-flex items-center gap-1 px-2 py-1 rounded-md text-xs font-medium bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200"
																				title={
																					host.reboot_reason ||
																					"Reboot required"
																				}
																			>
																				<RotateCcw className="h-3 w-3" />
																				Reboot Required
																			</span>
																		</div>
																	)}

																{/* Group info */}
																<div className="flex flex-wrap items-center gap-3 text-sm">
																	{visibleColumns.some(
																		(col) => col.id === "group",
																	) &&
																		groups.length > 0 && (
																			<div className="flex items-center gap-1 flex-wrap">
																				<span className="text-secondary-500 dark:text-secondary-400">
																					Groups:
																				</span>
																				{groups.map((g, idx) => (
																					<span
																						key={g.id}
																						className="text-secondary-700 dark:text-secondary-300"
																					>
																						{g.name}
																						{idx < groups.length - 1 ? "," : ""}
																					</span>
																				))}
																			</div>
																		)}
																</div>

																{/* Updates info */}
																<div className="flex items-center gap-4 pt-2 border-t border-secondary-200 dark:border-secondary-600">
																	{visibleColumns.some(
																		(col) => col.id === "updates",
																	) && (
																		<button
																			type="button"
																			onClick={() =>
																				navigate(
																					`/packages?host=${host.id}&filter=outdated`,
																				)
																			}
																			className="text-sm text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300 font-medium min-h-[44px] flex items-center"
																		>
																			{host.updatesCount || 0} Updates
																		</button>
																	)}
																	{visibleColumns.some(
																		(col) => col.id === "security_updates",
																	) && (
																		<button
																			type="button"
																			onClick={() =>
																				navigate(
																					`/packages?host=${host.id}&filter=security-updates`,
																				)
																			}
																			className="text-sm text-red-600 hover:text-red-900 dark:text-red-400 dark:hover:text-red-300 font-medium min-h-[44px] flex items-center"
																		>
																			{host.securityUpdatesCount || 0} Security
																		</button>
																	)}
																	{visibleColumns.some(
																		(col) => col.id === "last_update",
																	) && (
																		<div className="text-xs text-secondary-500 dark:text-secondary-400 ml-auto">
																			Updated{" "}
																			{formatRelativeTime(host.last_update)}
																		</div>
																	)}
																</div>
															</div>
														);
													})}
												</div>

												{/* Desktop Table Layout */}
												<div className="hidden md:block">
													<table className="w-full divide-y divide-secondary-200 dark:divide-secondary-600">
														<thead className="bg-secondary-50 dark:bg-secondary-700">
															<tr>
																{visibleColumns.map((column) => (
																	<th
																		key={column.id}
																		className="px-3 sm:px-4 py-2 text-center text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider whitespace-nowrap"
																	>
																		{column.id === "select" ? (
																			<button
																				type="button"
																				onClick={() =>
																					handleSelectAll(groupHosts)
																				}
																				className="flex items-center gap-2 hover:text-secondary-700"
																			>
																				{groupHosts.every((host) =>
																					selectedHosts.includes(host.id),
																				) ? (
																					<CheckSquare className="h-4 w-4" />
																				) : (
																					<Square className="h-4 w-4" />
																				)}
																			</button>
																		) : column.id === "host" ? (
																			<button
																				type="button"
																				onClick={() =>
																					handleSort("friendlyName")
																				}
																				className="flex items-center gap-2 hover:text-secondary-700"
																			>
																				{column.label}
																				{getSortIcon("friendlyName")}
																			</button>
																		) : column.id === "hostname" ? (
																			<button
																				type="button"
																				onClick={() => handleSort("hostname")}
																				className="flex items-center gap-2 hover:text-secondary-700"
																			>
																				{column.label}
																				{getSortIcon("hostname")}
																			</button>
																		) : column.id === "ip" ? (
																			<button
																				type="button"
																				onClick={() => handleSort("ip")}
																				className="flex items-center gap-2 hover:text-secondary-700"
																			>
																				{column.label}
																				{getSortIcon("ip")}
																			</button>
																		) : column.id === "group" ? (
																			<button
																				type="button"
																				onClick={() => handleSort("group")}
																				className="flex items-center gap-2 hover:text-secondary-700"
																			>
																				{column.label}
																				{getSortIcon("group")}
																			</button>
																		) : column.id === "os" ? (
																			<button
																				type="button"
																				onClick={() => handleSort("os")}
																				className="flex items-center gap-2 hover:text-secondary-700"
																			>
																				{column.label}
																				{getSortIcon("os")}
																			</button>
																		) : column.id === "os_version" ? (
																			<button
																				type="button"
																				onClick={() => handleSort("os_version")}
																				className="flex items-center gap-2 hover:text-secondary-700"
																			>
																				{column.label}
																				{getSortIcon("os_version")}
																			</button>
																		) : column.id === "agent_version" ? (
																			<button
																				type="button"
																				onClick={() =>
																					handleSort("agent_version")
																				}
																				className="flex items-center gap-2 hover:text-secondary-700"
																			>
																				{column.label}
																				{getSortIcon("agent_version")}
																			</button>
																		) : column.id === "auto_update" ? (
																			<div className="flex items-center gap-2 font-normal text-xs text-secondary-500 dark:text-secondary-300 normal-case tracking-wider">
																				{column.label}
																			</div>
																		) : column.id === "ws_status" ? (
																			<div className="flex items-center gap-2 font-normal text-xs text-secondary-500 dark:text-secondary-300 normal-case tracking-wider">
																				<Wifi className="h-3 w-3" />
																				{column.label}
																			</div>
																		) : column.id === "integrations" ? (
																			<button
																				type="button"
																				onClick={() =>
																					handleSort("integrations")
																				}
																				className="flex items-center gap-2 hover:text-secondary-700 font-normal text-xs text-secondary-500 dark:text-secondary-300 normal-case tracking-wider"
																			>
																				{column.label}
																				{getSortIcon("integrations")}
																			</button>
																		) : column.id === "status" ? (
																			<button
																				type="button"
																				onClick={() => handleSort("status")}
																				className="flex items-center gap-2 hover:text-secondary-700"
																			>
																				{column.label}
																				{getSortIcon("status")}
																			</button>
																		) : column.id === "updates" ? (
																			<button
																				type="button"
																				onClick={() => handleSort("updates")}
																				className="flex items-center gap-2 hover:text-secondary-700"
																			>
																				{column.label}
																				{getSortIcon("updates")}
																			</button>
																		) : column.id === "security_updates" ? (
																			<button
																				type="button"
																				onClick={() =>
																					handleSort("security_updates")
																				}
																				className="flex items-center gap-2 hover:text-secondary-700"
																			>
																				{column.label}
																				{getSortIcon("security_updates")}
																			</button>
																		) : column.id === "needs_reboot" ? (
																			<button
																				type="button"
																				onClick={() =>
																					handleSort("needs_reboot")
																				}
																				className="flex items-center gap-2 hover:text-secondary-700"
																			>
																				{column.label}
																				{getSortIcon("needs_reboot")}
																			</button>
																		) : column.id === "uptime" ? (
																			<button
																				type="button"
																				onClick={() => handleSort("uptime")}
																				className="flex items-center gap-2 hover:text-secondary-700 normal-case"
																			>
																				{column.label}
																				{getSortIcon("uptime")}
																			</button>
																		) : column.id === "last_update" ? (
																			<button
																				type="button"
																				onClick={() =>
																					handleSort("last_update")
																				}
																				className="flex items-center gap-2 hover:text-secondary-700"
																			>
																				{column.label}
																				{getSortIcon("last_update")}
																			</button>
																		) : (
																			column.label
																		)}
																	</th>
																))}
															</tr>
														</thead>
														<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-600">
															{groupHosts.map((host) => {
																const isInactive =
																	(host.effectiveStatus || host.status) ===
																	"inactive";
																const isSelected = selectedHosts.includes(
																	host.id,
																);

																let rowClasses =
																	"hover:bg-secondary-50 dark:hover:bg-secondary-700";

																if (isSelected) {
																	rowClasses +=
																		" bg-primary-50 dark:bg-primary-600";
																} else if (isInactive) {
																	rowClasses += " bg-red-50 dark:bg-red-900/20";
																}

																return (
																	<tr key={host.id} className={rowClasses}>
																		{visibleColumns.map((column) => (
																			<td
																				key={column.id}
																				className="px-3 sm:px-4 py-2 whitespace-nowrap text-center"
																			>
																				{renderCellContent(column, host)}
																			</td>
																		))}
																	</tr>
																);
															})}
														</tbody>
													</table>
												</div>
											</div>
										),
									)}
								</div>
							</div>
						)}
					</div>
				</div>
			</div>

			{/* Modals */}
			<AddHostWizard
				isOpen={showAddModal}
				onClose={() => setShowAddModal(false)}
				onSuccess={() => queryClient.invalidateQueries(["hosts"])}
			/>

			{/* Bulk Assign Modal */}
			{showBulkAssignModal && (
				<BulkAssignModal
					selectedHosts={selectedHosts}
					hosts={hosts}
					onClose={() => setShowBulkAssignModal(false)}
					onAssign={handleBulkAssign}
					isLoading={bulkUpdateGroupMutation.isPending}
				/>
			)}

			{/* Bulk Delete Modal */}
			{showBulkDeleteModal && (
				<BulkDeleteModal
					selectedHosts={selectedHosts}
					hosts={hosts}
					onClose={() => setShowBulkDeleteModal(false)}
					onDelete={handleBulkDelete}
					isLoading={bulkDeleteMutation.isPending}
				/>
			)}

			{/* Column Settings Modal */}
			{showColumnSettings && (
				<ColumnSettingsModal
					columnConfig={columnConfig}
					onClose={() => setShowColumnSettings(false)}
					onToggleVisibility={toggleColumnVisibility}
					onReorder={reorderColumns}
					onReset={resetColumns}
				/>
			)}

			{/* Auto-Update Confirmation Dialog */}
			{autoUpdateDialog.show && (
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
										<strong>{autoUpdateDialog.hostName}</strong> won't take
										effect until global auto-updates are enabled.
									</p>
								</div>
							</div>
						</div>
						<div className="bg-secondary-50 dark:bg-secondary-700/50 px-6 py-4 flex flex-col sm:flex-row gap-3 sm:justify-end">
							<button
								type="button"
								onClick={() =>
									setAutoUpdateDialog({
										show: false,
										hostId: null,
										hostName: null,
									})
								}
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

// Bulk Assign Modal Component
const BulkAssignModal = ({
	selectedHosts,
	hosts,
	onClose,
	onAssign,
	isLoading,
}) => {
	const [selectedGroupIds, setSelectedGroupIds] = useState([]);

	// Fetch host groups for selection
	const { data: hostGroups } = useQuery({
		queryKey: ["hostGroups"],
		queryFn: () => hostGroupsAPI.list().then((res) => res.data),
	});

	const selectedHostNames = hosts
		.filter((host) => selectedHosts.includes(host.id))
		.map((host) => host.friendly_name);

	const handleSubmit = (e) => {
		e.preventDefault();
		onAssign(selectedGroupIds);
	};

	const toggleGroup = (groupId) => {
		setSelectedGroupIds((prev) => {
			if (prev.includes(groupId)) {
				return prev.filter((id) => id !== groupId);
			} else {
				return [...prev, groupId];
			}
		});
	};

	return (
		<div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
			<div className="bg-white dark:bg-secondary-800 rounded-lg p-6 w-full max-w-md">
				<div className="flex justify-between items-center mb-4">
					<h3 className="text-lg font-semibold text-secondary-900 dark:text-white">
						Assign to Host Groups
					</h3>
					<button
						type="button"
						onClick={onClose}
						className="text-secondary-400 hover:text-secondary-600 dark:text-secondary-300 dark:hover:text-secondary-100"
					>
						<X className="h-5 w-5" />
					</button>
				</div>

				<div className="mb-4">
					<p className="text-sm text-secondary-600 dark:text-secondary-400 mb-2">
						Assigning {selectedHosts.length} host
						{selectedHosts.length !== 1 ? "s" : ""}:
					</p>
					<div className="max-h-32 overflow-y-auto bg-secondary-50 dark:bg-secondary-700 rounded-md p-3">
						{selectedHostNames.map((friendlyName) => (
							<div
								key={friendlyName}
								className="text-sm text-secondary-700 dark:text-secondary-300"
							>
								• {friendlyName}
							</div>
						))}
					</div>
				</div>

				<form onSubmit={handleSubmit} className="space-y-4">
					<div>
						<span className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-3">
							Host Groups
						</span>
						<div className="space-y-2 max-h-48 overflow-y-auto">
							{/* Host Group Options */}
							{hostGroups?.map((group) => (
								<label
									key={group.id}
									className={`flex items-center gap-3 p-3 border-2 rounded-lg transition-all duration-200 cursor-pointer ${
										selectedGroupIds.includes(group.id)
											? "border-primary-500 bg-primary-50 dark:bg-primary-900/30"
											: "border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-700 hover:border-secondary-400 dark:hover:border-secondary-500"
									}`}
								>
									<input
										type="checkbox"
										checked={selectedGroupIds.includes(group.id)}
										onChange={() => toggleGroup(group.id)}
										className="w-4 h-4 text-primary-600 bg-gray-100 border-gray-300 rounded focus:ring-primary-500 dark:focus:ring-primary-600 dark:ring-offset-gray-800 focus:ring-2 dark:bg-gray-700 dark:border-gray-600"
									/>
									<div className="flex items-center gap-2 flex-1">
										{group.color && (
											<div
												className="w-3 h-3 rounded-full border border-secondary-300 dark:border-secondary-500 flex-shrink-0"
												style={{ backgroundColor: group.color }}
											></div>
										)}
										<div className="text-sm font-medium text-secondary-700 dark:text-secondary-200">
											{group.name}
										</div>
									</div>
								</label>
							))}
						</div>
						<p className="mt-2 text-sm text-secondary-500 dark:text-secondary-400">
							Select one or more groups to assign these hosts to, or leave
							ungrouped.
						</p>
					</div>

					<div className="flex justify-end gap-3 pt-4">
						<button
							type="button"
							onClick={onClose}
							className="btn-outline"
							disabled={isLoading}
						>
							Cancel
						</button>
						<button type="submit" className="btn-primary" disabled={isLoading}>
							{isLoading ? "Assigning..." : "Assign to Groups"}
						</button>
					</div>
				</form>
			</div>
		</div>
	);
};

// Bulk Delete Modal Component
const BulkDeleteModal = ({
	selectedHosts,
	hosts,
	onClose,
	onDelete,
	isLoading,
}) => {
	const selectedHostNames = hosts
		.filter((host) => selectedHosts.includes(host.id))
		.map((host) => host.friendly_name || host.hostname || host.id);

	const handleSubmit = (e) => {
		e.preventDefault();
		onDelete();
	};

	return (
		<div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
			<div className="bg-white dark:bg-secondary-800 rounded-lg shadow-xl max-w-md w-full mx-4">
				<div className="px-6 py-4 border-b border-secondary-200 dark:border-secondary-600">
					<div className="flex items-center justify-between">
						<h3 className="text-lg font-medium text-secondary-900 dark:text-white">
							Delete Hosts
						</h3>
						<button
							type="button"
							onClick={onClose}
							className="text-secondary-400 hover:text-secondary-600 dark:text-secondary-500 dark:hover:text-secondary-300"
							disabled={isLoading}
						>
							<X className="h-5 w-5" />
						</button>
					</div>
				</div>

				<div className="px-6 py-4">
					<div className="mb-4">
						<div className="flex items-center gap-2 mb-3">
							<AlertTriangle className="h-5 w-5 text-danger-600" />
							<h4 className="text-sm font-medium text-danger-800 dark:text-danger-200">
								Warning: This action cannot be undone
							</h4>
						</div>
						<p className="text-sm text-secondary-600 dark:text-secondary-400 mb-4">
							You are about to permanently delete {selectedHosts.length} host
							{selectedHosts.length !== 1 ? "s" : ""}. This will remove all host
							data, including package information, update history, and API
							credentials.
						</p>
					</div>

					<div className="mb-4">
						<p className="text-sm text-secondary-600 dark:text-secondary-400 mb-2">
							Hosts to be deleted:
						</p>
						<div className="max-h-32 overflow-y-auto bg-secondary-50 dark:bg-secondary-700 rounded-md p-3">
							{selectedHostNames.map((friendlyName) => (
								<div
									key={friendlyName}
									className="text-sm text-secondary-700 dark:text-secondary-300"
								>
									• {friendlyName}
								</div>
							))}
						</div>
					</div>

					<form onSubmit={handleSubmit} className="space-y-4">
						<div className="flex justify-end gap-3 pt-4">
							<button
								type="button"
								onClick={onClose}
								className="btn-outline"
								disabled={isLoading}
							>
								Cancel
							</button>
							<button type="submit" className="btn-danger" disabled={isLoading}>
								{isLoading
									? "Deleting..."
									: `Delete ${selectedHosts.length} Host${selectedHosts.length !== 1 ? "s" : ""}`}
							</button>
						</div>
					</form>
				</div>
			</div>
		</div>
	);
};

// Column Settings Modal Component
const ColumnSettingsModal = ({
	columnConfig,
	onClose,
	onToggleVisibility,
	onReorder,
	onReset,
}) => {
	const [draggedIndex, setDraggedIndex] = useState(null);

	const handleDragStart = (e, index) => {
		setDraggedIndex(index);
		e.dataTransfer.effectAllowed = "move";
	};

	const handleDragOver = (e) => {
		e.preventDefault();
		e.dataTransfer.dropEffect = "move";
	};

	const handleDrop = (e, dropIndex) => {
		e.preventDefault();
		if (draggedIndex !== null && draggedIndex !== dropIndex) {
			onReorder(draggedIndex, dropIndex);
		}
		setDraggedIndex(null);
	};

	return (
		<div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4">
			<div className="bg-white dark:bg-secondary-800 rounded-lg shadow-xl max-w-lg w-full max-h-[85vh] flex flex-col">
				{/* Header */}
				<div className="px-6 py-4 border-b border-secondary-200 dark:border-secondary-600 flex-shrink-0">
					<div className="flex items-center justify-between">
						<h3 className="text-lg font-medium text-secondary-900 dark:text-white">
							Column Settings
						</h3>
						<button
							type="button"
							onClick={onClose}
							className="text-secondary-400 hover:text-secondary-600 dark:text-secondary-500 dark:hover:text-secondary-300"
						>
							<X className="h-5 w-5" />
						</button>
					</div>
					<p className="text-sm text-secondary-600 dark:text-secondary-300 mt-2">
						Drag to reorder columns or toggle visibility
					</p>
				</div>

				{/* Scrollable content */}
				<div className="px-6 py-4 flex-1 overflow-y-auto">
					<div className="space-y-1">
						{columnConfig.map((column, index) => (
							<button
								key={column.id}
								type="button"
								draggable
								aria-label={`Drag to reorder ${column.label} column`}
								onDragStart={(e) => handleDragStart(e, index)}
								onDragOver={handleDragOver}
								onDrop={(e) => handleDrop(e, index)}
								onKeyDown={(e) => {
									if (e.key === "Enter" || e.key === " ") {
										e.preventDefault();
										// Focus handling for keyboard users
									}
								}}
								className={`flex items-center justify-between p-2.5 border rounded-lg cursor-move w-full transition-colors ${
									draggedIndex === index
										? "opacity-50"
										: "hover:bg-secondary-50 dark:hover:bg-secondary-700"
								} border-secondary-200 dark:border-secondary-600`}
							>
								<div className="flex items-center gap-2.5">
									<GripVertical className="h-4 w-4 text-secondary-400 dark:text-secondary-500 flex-shrink-0" />
									<span className="text-sm font-medium text-secondary-900 dark:text-white truncate">
										{column.label}
									</span>
								</div>
								<button
									type="button"
									onClick={(e) => {
										e.stopPropagation();
										onToggleVisibility(column.id);
									}}
									className={`p-1 rounded transition-colors flex-shrink-0 min-w-[44px] min-h-[44px] flex items-center justify-center ${
										column.visible
											? "text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
											: "text-secondary-400 hover:text-secondary-600 dark:text-secondary-500 dark:hover:text-secondary-300"
									}`}
									aria-label={
										column.visible
											? `Hide ${column.label} column`
											: `Show ${column.label} column`
									}
								>
									{column.visible ? (
										<EyeIcon className="h-4 w-4" />
									) : (
										<EyeOffIcon className="h-4 w-4" />
									)}
								</button>
							</button>
						))}
					</div>
				</div>

				{/* Footer */}
				<div className="px-6 py-4 border-t border-secondary-200 dark:border-secondary-600 flex-shrink-0">
					<div className="flex justify-between">
						<button type="button" onClick={onReset} className="btn-outline">
							Reset to Default
						</button>
						<button type="button" onClick={onClose} className="btn-primary">
							Done
						</button>
					</div>
				</div>
			</div>
		</div>
	);
};

export default Hosts;
