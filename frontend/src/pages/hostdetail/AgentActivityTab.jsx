import { useQuery } from "@tanstack/react-query";
import {
	AlertTriangle,
	ChevronLeft,
	ChevronRight,
	RefreshCw,
	Search,
	Server,
	X,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import AgentActivityTable from "../../components/host/AgentActivityTable";
import QueueStatCards from "../../components/host/QueueStatCards";
import { dashboardAPI } from "../../utils/api";

const PAGE_SIZE = 50;

// URL params owned by this tab. Listed here so we know exactly which keys to
// strip when clearing filters or rewriting params on tab change.
const PARAM_KEYS = ["direction", "type", "status", "since", "q", "page"];

const DIRECTION_OPTIONS = [
	{ value: "", label: "All directions" },
	{ value: "in", label: "Inbound (reports)" },
	{ value: "out", label: "Outbound (jobs)" },
];

const TIME_RANGE_OPTIONS = [
	{ value: "24h", label: "Last 24 hours" },
	{ value: "7d", label: "Last 7 days" },
	{ value: "30d", label: "Last 30 days" },
	{ value: "all", label: "All time" },
];

const REPORT_TYPE_OPTIONS = [
	{ value: "ping", label: "Ping" },
	{ value: "full", label: "Full" },
	{ value: "partial", label: "Partial" },
	{ value: "docker", label: "Docker" },
	{ value: "compliance", label: "Compliance" },
];

const COMMON_JOB_OPTIONS = [
	{ value: "report_now", label: "Report Now" },
	{ value: "refresh_integration_status", label: "Refresh Integration Status" },
	{ value: "docker_inventory_refresh", label: "Docker Inventory Refresh" },
	{ value: "update_agent", label: "Agent Update" },
	{ value: "run_scan", label: "Compliance Scan" },
	{ value: "install_compliance_tools", label: "Install Compliance Scanner" },
	{ value: "ssg_upgrade", label: "SSG Content Upgrade" },
	{ value: "run_patch", label: "Run Patch" },
	{ value: "scheduled_reports_dispatch", label: "Scheduled Reports Dispatch" },
	{ value: "scheduled_report_run", label: "Scheduled Report Run" },
	{ value: "update-threshold-monitor", label: "Update Threshold Monitor" },
	{ value: "host-status-monitor", label: "Host Status Monitor" },
	{ value: "metrics-send", label: "Metrics Send" },
	{ value: "agent-reports-cleanup", label: "Agent Reports Cleanup" },
	{ value: "patch-run-cleanup", label: "Patch Run Cleanup" },
	{ value: "compliance-scan-cleanup", label: "Compliance Scan Cleanup" },
	{ value: "ssg-update-check", label: "SSG Update Check" },
	{ value: "version-update-check", label: "Version Update Check" },
	{ value: "system-statistics", label: "System Statistics" },
	{ value: "docker-inventory-cleanup", label: "Docker Inventory Cleanup" },
	{ value: "orphaned-package-cleanup", label: "Orphaned Package Cleanup" },
	{ value: "orphaned-repo-cleanup", label: "Orphaned Repo Cleanup" },
	{ value: "session-cleanup", label: "Session Cleanup" },
];

const STATUS_OPTIONS = [
	{ value: "success", label: "Success" },
	{ value: "completed", label: "Completed" },
	{ value: "active", label: "Active" },
	{ value: "waiting", label: "Waiting" },
	{ value: "delayed", label: "Delayed" },
	{ value: "failed", label: "Failed" },
	{ value: "error", label: "Error" },
];

const DEFAULT_TIME_RANGE = "7d";

// Convert a UI keyword time-range to an RFC3339 timestamp for the server's
// `since` query param. Server-side parser only accepts RFC3339 — sending
// "7d" was previously a silent no-op and the user got every row.
const TIME_RANGE_MS = {
	"24h": 24 * 60 * 60 * 1000,
	"7d": 7 * 24 * 60 * 60 * 1000,
	"30d": 30 * 24 * 60 * 60 * 1000,
};

const sinceFromRange = (range) => {
	const ms = TIME_RANGE_MS[range];
	if (!ms) return "";
	return new Date(Date.now() - ms).toISOString();
};

const parseList = (value) =>
	(value || "")
		.split(",")
		.map((v) => v.trim())
		.filter((v) => v.length > 0);

// Multiselect rendered as toggleable chips. Cheap, accessible, no extra deps.
const ChipMultiSelect = ({ label, options, values, onChange }) => {
	const toggle = (value) => {
		if (values.includes(value)) {
			onChange(values.filter((v) => v !== value));
		} else {
			onChange([...values, value]);
		}
	};
	return (
		<div>
			<span className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1">
				{label}
			</span>
			<div className="flex flex-wrap gap-1.5">
				{options.map((opt) => {
					const active = values.includes(opt.value);
					return (
						<button
							type="button"
							key={opt.value}
							onClick={() => toggle(opt.value)}
							className={`px-2.5 py-1 rounded border text-xs font-medium transition-colors ${
								active
									? "bg-primary-100 border-primary-300 text-primary-800 dark:bg-primary-900 dark:border-primary-700 dark:text-primary-200"
									: "bg-white border-secondary-300 text-secondary-600 hover:border-primary-400 dark:bg-secondary-800 dark:border-secondary-600 dark:text-secondary-200"
							}`}
						>
							{opt.label}
						</button>
					);
				})}
			</div>
		</div>
	);
};

const AgentActivityTab = ({ hostId }) => {
	const [searchParams, setSearchParams] = useSearchParams();

	// Read filter values straight off the URL — single source of truth so deep
	// links and browser back/forward "just work".
	const direction = searchParams.get("direction") || "";
	const typeRaw = searchParams.get("type") || "";
	const statusRaw = searchParams.get("status") || "";
	// Memoise the parsed arrays against their raw URL strings so the array
	// reference is stable across renders. Without this, parseList() would
	// produce a fresh array each render, which propagates into the queryParams
	// useMemo (typeFilter/statusFilter are deps) and forces sinceFromRange()
	// to recompute Date.now() on every render — TanStack Query then sees a
	// new query key every ~10ms and refetches in a tight loop.
	const typeFilter = useMemo(() => parseList(typeRaw), [typeRaw]);
	const statusFilter = useMemo(() => parseList(statusRaw), [statusRaw]);
	const timeRange = searchParams.get("since") || DEFAULT_TIME_RANGE;
	const urlSearch = searchParams.get("q") || "";
	const page = Math.max(
		1,
		Number.parseInt(searchParams.get("page") || "1", 10) || 1,
	);

	const [searchTerm, setSearchTerm] = useState(urlSearch);
	const searchDebounceRef = useRef(null);

	// Mutate only the keys we own; preserves `tab` and any unrelated params.
	const updateParams = useCallback(
		(updates) => {
			setSearchParams(
				(prev) => {
					const next = new URLSearchParams(prev);
					Object.entries(updates).forEach(([key, value]) => {
						if (
							value === undefined ||
							value === null ||
							value === "" ||
							(Array.isArray(value) && value.length === 0)
						) {
							next.delete(key);
						} else if (Array.isArray(value)) {
							next.set(key, value.join(","));
						} else {
							next.set(key, String(value));
						}
					});
					return next;
				},
				{ replace: true },
			);
		},
		[setSearchParams],
	);

	// Debounce the search input -> URL. Reset the page when search changes.
	useEffect(() => {
		if (searchDebounceRef.current) clearTimeout(searchDebounceRef.current);
		searchDebounceRef.current = setTimeout(() => {
			const trimmed = searchTerm.trim();
			if (trimmed === urlSearch) return;
			updateParams({ q: trimmed, page: null });
		}, 400);
		return () => {
			if (searchDebounceRef.current) clearTimeout(searchDebounceRef.current);
		};
	}, [searchTerm, urlSearch, updateParams]);

	// Keep local input in sync if URL changes externally (browser nav, deep
	// links, clear-filters).
	useEffect(() => {
		setSearchTerm(urlSearch);
	}, [urlSearch]);

	const setDirection = (value) =>
		updateParams({ direction: value, page: null });
	const setTypeFilter = (values) => updateParams({ type: values, page: null });
	const setStatusFilter = (values) =>
		updateParams({ status: values, page: null });
	const setTimeRange = (value) =>
		updateParams({
			since: value === DEFAULT_TIME_RANGE ? null : value,
			page: null,
		});

	const [showFilters, setShowFilters] = useState(false);

	const queryParams = useMemo(
		() => ({
			direction,
			type: typeFilter,
			status: statusFilter,
			since: timeRange === "all" ? "" : sinceFromRange(timeRange),
			search: urlSearch,
			limit: PAGE_SIZE,
			offset: (page - 1) * PAGE_SIZE,
		}),
		[direction, typeFilter, statusFilter, timeRange, urlSearch, page],
	);

	const {
		data: response,
		isLoading,
		isFetching,
		error,
		refetch,
	} = useQuery({
		queryKey: ["host-activity", hostId, queryParams],
		queryFn: () =>
			dashboardAPI.getHostActivity(hostId, queryParams).then((res) => res.data),
		enabled: !!hostId,
		staleTime: 30 * 1000,
		refetchInterval: 30 * 1000,
		keepPreviousData: true,
	});

	// HostActivity returns the body at the top level (no {success, data}
	// wrapper). The queryFn already unwraps the axios envelope via .data, so
	// `response` here is the parsed JSON body.
	const data = response || {};
	const stats = data.stats || {};
	const items = Array.isArray(data.items) ? data.items : [];
	const total = typeof data.total === "number" ? data.total : items.length;
	const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));
	const start = total === 0 ? 0 : (page - 1) * PAGE_SIZE + 1;
	const end = total === 0 ? 0 : Math.min(page * PAGE_SIZE, total);

	const typeOptions = useMemo(
		() => [...REPORT_TYPE_OPTIONS, ...COMMON_JOB_OPTIONS],
		[],
	);

	const clearFilters = () => {
		setSearchParams(
			(prev) => {
				const next = new URLSearchParams(prev);
				for (const key of PARAM_KEYS) {
					next.delete(key);
				}
				return next;
			},
			{ replace: true },
		);
		setSearchTerm("");
	};

	const hasActiveFilters =
		direction !== "" ||
		typeFilter.length > 0 ||
		statusFilter.length > 0 ||
		timeRange !== DEFAULT_TIME_RANGE ||
		urlSearch.trim() !== "";

	const setPage = (next) => {
		const clamped = Math.min(Math.max(1, next), totalPages);
		updateParams({ page: clamped === 1 ? null : clamped });
	};

	return (
		<div className="space-y-4">
			<QueueStatCards stats={stats} />

			<div className="card p-4 sm:p-6 space-y-4">
				<div className="flex flex-col sm:flex-row gap-3 sm:items-center sm:justify-between">
					<div className="flex-1">
						<div className="relative">
							<Search className="absolute left-3 top-1/2 transform -translate-y-1/2 h-4 w-4 text-secondary-400 dark:text-white" />
							<input
								type="text"
								placeholder="Search errors and output..."
								value={searchTerm}
								onChange={(e) => setSearchTerm(e.target.value)}
								className="pl-10 pr-4 py-2 w-full border border-secondary-300 dark:border-secondary-600 rounded-lg focus:ring-2 focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-800 text-secondary-900 dark:text-white placeholder-secondary-500 dark:placeholder-secondary-400 min-h-[44px] sm:min-h-0"
							/>
						</div>
					</div>
					<div className="flex items-center gap-2">
						<button
							type="button"
							onClick={() => setShowFilters((prev) => !prev)}
							className={`btn-outline flex items-center gap-1.5 sm:gap-2 px-3 py-2 min-h-[44px] text-xs sm:text-sm ${
								showFilters || hasActiveFilters
									? "bg-primary-50 border-primary-300 dark:bg-primary-900/30 dark:border-primary-700"
									: ""
							}`}
						>
							{showFilters ? <X className="h-4 w-4 flex-shrink-0" /> : null}
							<span>Filters</span>
							{hasActiveFilters && !showFilters && (
								<span className="inline-flex items-center justify-center px-1.5 py-0.5 rounded bg-primary-600 text-white text-[10px] font-semibold">
									{
										[
											direction !== "",
											typeFilter.length > 0,
											statusFilter.length > 0,
											timeRange !== DEFAULT_TIME_RANGE,
											urlSearch.trim() !== "",
										].filter(Boolean).length
									}
								</span>
							)}
						</button>
						<button
							type="button"
							onClick={() => refetch()}
							disabled={isFetching}
							className="btn-outline flex items-center gap-2 px-3 py-2 min-h-[44px] text-xs sm:text-sm"
							title="Refresh activity"
						>
							<RefreshCw
								className={`h-4 w-4 ${isFetching ? "animate-spin" : ""}`}
							/>
							<span className="hidden sm:inline">Refresh</span>
						</button>
					</div>
				</div>

				{showFilters && (
					<div className="bg-secondary-50 dark:bg-secondary-700 p-3 sm:p-4 rounded-lg border dark:border-secondary-600 space-y-4">
						<div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3 sm:gap-4">
							<div>
								<span className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1">
									Direction
								</span>
								<select
									value={direction}
									onChange={(e) => setDirection(e.target.value)}
									className="w-full border border-secondary-300 dark:border-secondary-600 rounded-lg px-3 py-2.5 sm:py-2 focus:ring-2 focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-800 text-secondary-900 dark:text-white min-h-[44px] sm:min-h-0"
								>
									{DIRECTION_OPTIONS.map((opt) => (
										<option key={opt.value} value={opt.value}>
											{opt.label}
										</option>
									))}
								</select>
							</div>
							<div>
								<span className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1">
									Time range
								</span>
								<select
									value={timeRange}
									onChange={(e) => setTimeRange(e.target.value)}
									className="w-full border border-secondary-300 dark:border-secondary-600 rounded-lg px-3 py-2.5 sm:py-2 focus:ring-2 focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-800 text-secondary-900 dark:text-white min-h-[44px] sm:min-h-0"
								>
									{TIME_RANGE_OPTIONS.map((opt) => (
										<option key={opt.value} value={opt.value}>
											{opt.label}
										</option>
									))}
								</select>
							</div>
							<div className="flex items-end">
								<button
									type="button"
									onClick={clearFilters}
									disabled={!hasActiveFilters}
									className="btn-outline w-full min-h-[44px] disabled:opacity-50 disabled:cursor-not-allowed"
								>
									Clear filters
								</button>
							</div>
						</div>
						<ChipMultiSelect
							label="Type"
							options={typeOptions}
							values={typeFilter}
							onChange={setTypeFilter}
						/>
						<ChipMultiSelect
							label="Status"
							options={STATUS_OPTIONS}
							values={statusFilter}
							onChange={setStatusFilter}
						/>
					</div>
				)}

				{error ? (
					<div className="bg-danger-50 dark:bg-danger-900/30 border border-danger-200 dark:border-danger-700 rounded-md p-4">
						<div className="flex">
							<AlertTriangle className="h-5 w-5 text-danger-500 dark:text-danger-400 flex-shrink-0" />
							<div className="ml-3">
								<h3 className="text-sm font-medium text-danger-800 dark:text-danger-200">
									Failed to load agent activity
								</h3>
								<p className="text-sm text-danger-700 dark:text-danger-300 mt-1">
									{error?.response?.data?.error ||
										error?.message ||
										"Unknown error"}
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
				) : isLoading ? (
					<div className="flex items-center justify-center h-32">
						<RefreshCw className="h-8 w-8 animate-spin text-primary-600" />
					</div>
				) : items.length === 0 ? (
					<div className="text-center py-8">
						<Server className="h-12 w-12 text-secondary-400 mx-auto mb-4" />
						<p className="text-secondary-500 dark:text-white">
							No activity to show
						</p>
						<p className="text-sm text-secondary-400 dark:text-white mt-2">
							{hasActiveFilters
								? "Try adjusting your search terms or filters"
								: "The agent has not reported in this time range yet"}
						</p>
					</div>
				) : (
					<>
						<AgentActivityTable items={items} />
						<div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 pt-3 border-t border-secondary-200 dark:border-secondary-600">
							<p className="text-sm text-secondary-700 dark:text-white">
								{total > 0 ? `${start}-${end} of ${total}` : "0 of 0"}
							</p>
							<div className="flex items-center gap-2">
								<button
									type="button"
									onClick={() => setPage(page - 1)}
									disabled={page === 1 || isFetching}
									className="p-2 rounded hover:bg-secondary-100 dark:hover:bg-secondary-600 disabled:opacity-50 disabled:cursor-not-allowed min-h-[44px] min-w-[44px] flex items-center justify-center"
									title="Previous page"
								>
									<ChevronLeft className="h-4 w-4" />
								</button>
								<span className="text-sm text-secondary-700 dark:text-white whitespace-nowrap">
									Page {page} of {totalPages}
								</span>
								<button
									type="button"
									onClick={() => setPage(page + 1)}
									disabled={page >= totalPages || isFetching}
									className="p-2 rounded hover:bg-secondary-100 dark:hover:bg-secondary-600 disabled:opacity-50 disabled:cursor-not-allowed min-h-[44px] min-w-[44px] flex items-center justify-center"
									title="Next page"
								>
									<ChevronRight className="h-4 w-4" />
								</button>
							</div>
						</div>
					</>
				)}
			</div>
		</div>
	);
};

export default AgentActivityTab;
