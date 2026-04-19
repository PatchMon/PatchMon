import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
	AlertTriangle,
	CheckCircle,
	Clock,
	Edit,
	History,
	LayoutDashboard,
	ListChecks,
	PlayCircle,
	Plus,
	RefreshCw,
	Server,
	Shield,
	Trash2,
	User,
	Users,
	X,
	XCircle,
} from "lucide-react";
import { Fragment, useEffect, useState } from "react";
import {
	Link,
	useLocation,
	useNavigate,
	useSearchParams,
} from "react-router-dom";
import { PackageListDisplay } from "../components/PackageListDisplay";
import { PatchRunStatusBadge } from "../components/PatchRunStatusBadge";
import {
	PatchingActivePolicies,
	PatchingPendingApproval,
	PatchingRecentRuns,
	PatchRunOutcomesDoughnut,
	PatchRunStatusBoxes,
	PatchRunsByType,
} from "../components/patching/widgets";
import { TimezoneSelect } from "../components/TimezoneSelect";
import { useToast } from "../contexts/ToastContext";
import { adminHostsAPI, formatDate, hostGroupsAPI } from "../utils/api";
import { patchingAPI } from "../utils/patchingApi";

const PATCHING_TABS = [
	{ id: "overview", label: "Overview", icon: LayoutDashboard },
	{ id: "runs", label: "Runs & History", icon: History },
	{ id: "policies", label: "Policies", icon: Shield },
];

const ValidatedBadge = () => (
	<span className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs font-medium bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200">
		<AlertTriangle className="h-3 w-3" />
		Extra deps
	</span>
);

const Patching = () => {
	const [searchParams] = useSearchParams();
	const location = useLocation();
	const navigate = useNavigate();
	const urlTab = searchParams.get("tab");
	const initialTab = ["runs", "policies"].includes(urlTab)
		? urlTab
		: "overview";
	const initialStatus = searchParams.get("status") || "";
	const initialType = searchParams.get("type") || "";
	const [activeTab, setActiveTab] = useState(initialTab);

	// Sync tab and filter state on actual URL navigation (not in-page tab clicks)
	useEffect(() => {
		const params = new URLSearchParams(location.search);
		const tab = params.get("tab");
		if (tab && ["overview", "runs", "policies"].includes(tab)) {
			setActiveTab(tab);
		}
		const status = params.get("status") || "";
		setRunsFilterStatus(status);
		const type = params.get("type") || "";
		setRunsFilterType(type);
		setRunsPage(1);
	}, [location.search]);

	const [runsFilterStatus, setRunsFilterStatus] = useState(initialStatus);
	const [runsFilterType, setRunsFilterType] = useState(initialType);
	const [runsPage, setRunsPage] = useState(1);
	const runsLimit = 25;
	const queryClient = useQueryClient();

	const {
		data: dashboard,
		isLoading,
		error,
	} = useQuery({
		queryKey: ["patching-dashboard"],
		queryFn: () => patchingAPI.getDashboard(),
		staleTime: 30 * 1000,
		refetchInterval: 30 * 1000,
	});

	const { data: runsData } = useQuery({
		queryKey: ["patching-runs", runsFilterStatus, runsFilterType, runsPage],
		queryFn: () =>
			patchingAPI.getRuns({
				...(runsFilterStatus ? { status: runsFilterStatus } : {}),
				...(runsFilterType ? { patch_type: runsFilterType } : {}),
				limit: runsLimit,
				offset: (runsPage - 1) * runsLimit,
			}),
		staleTime: 15 * 1000,
		enabled: activeTab === "runs",
	});

	const runs = runsData?.runs || [];
	const runsPagination = runsData?.pagination || { total: 0, pages: 0 };

	const approveMutation = useMutation({
		mutationFn: (runId) => patchingAPI.approveRun(runId),
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ["patching-runs"] });
			queryClient.invalidateQueries({ queryKey: ["patching-dashboard"] });
		},
	});
	const retryValidationMutation = useMutation({
		mutationFn: (runId) => patchingAPI.retryValidation(runId),
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ["patching-runs"] });
			queryClient.invalidateQueries({ queryKey: ["patching-dashboard"] });
		},
	});
	const [approvingId, setApprovingId] = useState(null);
	const [retryingId, setRetryingId] = useState(null);
	const [selectedRunIds, setSelectedRunIds] = useState(new Set());
	const [selectedApproveIds, setSelectedApproveIds] = useState(new Set());
	const [bulkApproving, setBulkApproving] = useState(false);
	const [bulkApproveResult, setBulkApproveResult] = useState(null);

	const deletableStatuses = new Set([
		"queued",
		"pending_validation",
		"validated",
		"approved",
		"scheduled",
	]);
	const deletableRuns = runs.filter((r) => deletableStatuses.has(r.status));
	const allDeletableSelected =
		deletableRuns.length > 0 &&
		deletableRuns.every((r) => selectedRunIds.has(r.id));

	const approvableStatuses = new Set(["validated", "pending_validation"]);
	const approvableRuns = runs.filter((r) => approvableStatuses.has(r.status));
	const allApprovableSelected =
		approvableRuns.length > 0 &&
		approvableRuns.every((r) => selectedApproveIds.has(r.id));

	const deleteRunMutation = useMutation({
		mutationFn: (runId) => patchingAPI.deleteRun(runId),
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ["patching-runs"] });
			queryClient.invalidateQueries({ queryKey: ["patching-dashboard"] });
		},
	});
	const [deletingIds, setDeletingIds] = useState(new Set());

	const handleToggleSelect = (runId) => {
		setSelectedRunIds((prev) => {
			const next = new Set(prev);
			if (next.has(runId)) next.delete(runId);
			else next.add(runId);
			return next;
		});
	};

	const handleToggleSelectAll = () => {
		if (allDeletableSelected) {
			setSelectedRunIds(new Set());
		} else {
			setSelectedRunIds(new Set(deletableRuns.map((r) => r.id)));
		}
	};

	const handleDeleteSelected = async () => {
		const ids = [...selectedRunIds];
		setDeletingIds(new Set(ids));
		try {
			await Promise.all(ids.map((id) => deleteRunMutation.mutateAsync(id)));
			setSelectedRunIds(new Set());
		} catch (_err) {
			// Mutation error - queries will refetch; user sees updated list
		} finally {
			setDeletingIds(new Set());
		}
	};

	const handleToggleApproveSelect = (runId) => {
		setSelectedApproveIds((prev) => {
			const next = new Set(prev);
			if (next.has(runId)) next.delete(runId);
			else next.add(runId);
			return next;
		});
	};

	const handleToggleApproveSelectAll = () => {
		if (allApprovableSelected) {
			setSelectedApproveIds(new Set());
		} else {
			setSelectedApproveIds(new Set(approvableRuns.map((r) => r.id)));
		}
	};

	const handleApproveSelected = async () => {
		const ids = [...selectedApproveIds];
		if (ids.length === 0) return;
		setBulkApproving(true);
		setBulkApproveResult(null);
		// Use allSettled — approvals are independent writes, so a single
		// failing row should not cancel the remaining approvals.
		const results = await Promise.allSettled(
			ids.map((id) => approveMutation.mutateAsync(id)),
		);
		const failed = results.filter((r) => r.status === "rejected").length;
		setBulkApproveResult({ ok: ids.length - failed, failed });
		setSelectedApproveIds(new Set());
		setBulkApproving(false);
	};

	const handleApprove = async (runId) => {
		setApprovingId(runId);
		try {
			// Approval creates a NEW execution run linked to this validation
			// run. When that new run is going to start immediately we deep-link
			// the user into its detail page so they can watch the live
			// terminal output instead of having to hunt for it in the list.
			const res = await approveMutation.mutateAsync(runId);
			const newRunId = res?.patch_run_id;
			const runAt = res?.run_at ? Date.parse(res.run_at) : NaN;
			const isImmediate = Number.isFinite(runAt) && runAt - Date.now() < 5_000;
			if (newRunId && isImmediate) {
				navigate(`/patching/runs/${newRunId}`);
			}
		} finally {
			setApprovingId(null);
		}
	};

	const handleRetryValidation = async (runId) => {
		setRetryingId(runId);
		try {
			await retryValidationMutation.mutateAsync(runId);
		} finally {
			setRetryingId(null);
		}
	};

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
				<p className="text-red-200">Failed to load patching dashboard</p>
			</div>
		);
	}

	const summary = dashboard?.summary || {};

	return (
		<div className="space-y-6">
			<div>
				<h1 className="text-2xl font-semibold text-secondary-900 dark:text-white">
					Patching
				</h1>
				<p className="text-sm text-secondary-600 dark:text-white mt-1">
					View and manage patch runs across hosts
				</p>
			</div>

			<div className="grid grid-cols-2 lg:grid-cols-5 gap-4">
				<div className="card p-4">
					<div className="flex items-center">
						<ListChecks className="h-5 w-5 text-primary-600 mr-2" />
						<div>
							<p className="text-sm text-secondary-500 dark:text-white">
								Total runs
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{summary.total_runs ?? 0}
							</p>
						</div>
					</div>
				</div>
				<div className="card p-4">
					<div className="flex items-center">
						<Clock className="h-5 w-5 text-blue-600 mr-2" />
						<div>
							<p className="text-sm text-secondary-500 dark:text-white">
								Queued / Running
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{(summary.queued ?? 0) + (summary.running ?? 0)}
							</p>
						</div>
					</div>
				</div>
				<div className="card p-4">
					<div className="flex items-center">
						<CheckCircle className="h-5 w-5 text-green-600 mr-2" />
						<div>
							<p className="text-sm text-secondary-500 dark:text-white">
								Completed
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{summary.completed ?? 0}
							</p>
						</div>
					</div>
				</div>
				<div className="card p-4">
					<div className="flex items-center">
						<XCircle className="h-5 w-5 text-red-600 mr-2" />
						<div>
							<p className="text-sm text-secondary-500 dark:text-white">
								Failed
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{summary.failed ?? 0}
							</p>
						</div>
					</div>
				</div>
				<button
					type="button"
					onClick={() => setActiveTab("policies")}
					className="card p-4 hover:shadow-card-hover dark:hover:shadow-card-hover-dark transition-shadow text-left"
				>
					<div className="flex items-center">
						<Shield className="h-5 w-5 text-secondary-600 mr-2" />
						<div>
							<p className="text-sm text-secondary-500 dark:text-white">
								Patch policies
							</p>
							<p className="text-sm font-medium text-primary-600 dark:text-primary-400">
								Manage policies
							</p>
						</div>
					</div>
				</button>
			</div>

			{/* Tabs */}
			<div className="border-b border-secondary-200 dark:border-secondary-600 overflow-x-auto scrollbar-hide">
				<nav
					className="-mb-px flex space-x-4 sm:space-x-8 px-4"
					aria-label="Tabs"
				>
					{PATCHING_TABS.map((tab) => {
						const Icon = tab.icon;
						return (
							<button
								key={tab.id}
								type="button"
								onClick={() => setActiveTab(tab.id)}
								className={`${
									activeTab === tab.id
										? "border-primary-500 text-primary-600 dark:text-primary-400"
										: "border-transparent text-secondary-500 hover:text-secondary-700 hover:border-secondary-300 dark:text-white dark:hover:text-primary-400"
								} whitespace-nowrap py-4 px-1 border-b-2 font-medium text-sm flex items-center`}
							>
								<Icon className="h-4 w-4 mr-2" />
								{tab.label}
							</button>
						);
					})}
				</nav>
			</div>

			{activeTab === "overview" && (
				<div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4 sm:gap-6 mt-6">
					<PatchRunStatusBoxes data={dashboard} />
					<PatchRunOutcomesDoughnut data={dashboard} />
					<PatchingRecentRuns data={dashboard} />
					<PatchingPendingApproval data={dashboard} />
					<PatchRunsByType data={dashboard} />
					<PatchingActivePolicies data={dashboard} />
				</div>
			)}

			{activeTab === "runs" && (
				<div className="mt-4">
					<div className="flex flex-wrap items-center gap-3 mb-4">
						<span className="text-sm text-secondary-600 dark:text-secondary-400">
							Filter by status:
						</span>
						<select
							value={runsFilterStatus}
							onChange={(e) => {
								setRunsFilterStatus(e.target.value);
								setRunsPage(1);
							}}
							className="rounded-md border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-800 text-secondary-900 dark:text-white text-sm"
						>
							<option value="">All</option>
							<option value="queued">Queued</option>
							<option value="pending_validation">Pending validation</option>
							<option value="validated">Validated (awaiting approval)</option>
							<option value="approved">Approved</option>
							<option value="scheduled">Scheduled</option>
							<option value="running">Running</option>
							<option value="completed">Completed</option>
							<option value="failed">Failed</option>
							<option value="cancelled">Cancelled</option>
						</select>
						<span className="text-sm text-secondary-600 dark:text-secondary-400">
							Type:
						</span>
						<select
							value={runsFilterType}
							onChange={(e) => {
								setRunsFilterType(e.target.value);
								setRunsPage(1);
							}}
							className="rounded-md border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-800 text-secondary-900 dark:text-white text-sm"
						>
							<option value="">All</option>
							<option value="patch_all">Patch All</option>
							<option value="patch_package">Patch Package</option>
						</select>
						{selectedRunIds.size > 0 && (
							<>
								<button
									type="button"
									onClick={handleDeleteSelected}
									disabled={deletingIds.size > 0}
									className="btn-danger flex items-center gap-1.5 sm:gap-2 px-3 sm:px-4 py-2 min-h-[44px] text-xs sm:text-sm"
								>
									<Trash2 className="h-4 w-4 flex-shrink-0" />
									<span>Delete {selectedRunIds.size} selected</span>
								</button>
								<button
									type="button"
									onClick={() => setSelectedRunIds(new Set())}
									className="text-xs sm:text-sm text-secondary-500 dark:text-white/70 hover:text-secondary-700 dark:hover:text-white/90 min-h-[44px] px-2"
								>
									<span className="hidden sm:inline">Clear selection</span>
									<span className="sm:hidden">Clear</span>
								</button>
							</>
						)}
						{selectedApproveIds.size > 0 && (
							<>
								<button
									type="button"
									onClick={handleApproveSelected}
									disabled={bulkApproving}
									className="btn-primary flex items-center gap-1.5 sm:gap-2 px-3 sm:px-4 py-2 min-h-[44px] text-xs sm:text-sm"
								>
									{bulkApproving ? (
										<RefreshCw className="h-4 w-4 flex-shrink-0 animate-spin" />
									) : (
										<CheckCircle className="h-4 w-4 flex-shrink-0" />
									)}
									<span>
										{bulkApproving ? "Approving…" : "Approve"}{" "}
										{selectedApproveIds.size} selected
									</span>
								</button>
								<button
									type="button"
									onClick={() => setSelectedApproveIds(new Set())}
									disabled={bulkApproving}
									className="text-xs sm:text-sm text-secondary-500 dark:text-white/70 hover:text-secondary-700 dark:hover:text-white/90 min-h-[44px] px-2"
								>
									<span className="hidden sm:inline">Clear approve</span>
									<span className="sm:hidden">Clear</span>
								</button>
							</>
						)}
					</div>
					{bulkApproveResult && (
						<div className="flex items-center justify-between gap-2 px-3 py-2 rounded-lg border border-secondary-200 dark:border-secondary-600 bg-secondary-50 dark:bg-secondary-800 text-sm">
							<div className="flex items-center gap-2 flex-wrap">
								{bulkApproveResult.failed === 0 ? (
									<CheckCircle className="h-4 w-4 text-green-600 flex-shrink-0" />
								) : (
									<AlertTriangle className="h-4 w-4 text-amber-600 flex-shrink-0" />
								)}
								<span className="text-secondary-800 dark:text-secondary-200">
									Approved {bulkApproveResult.ok}
									{bulkApproveResult.failed > 0
										? `, ${bulkApproveResult.failed} failed`
										: ""}
								</span>
							</div>
							<button
								type="button"
								onClick={() => setBulkApproveResult(null)}
								className="text-secondary-500 hover:text-secondary-700 dark:text-white/70 dark:hover:text-white/90"
								aria-label="Dismiss"
							>
								<X className="h-4 w-4" />
							</button>
						</div>
					)}
					<div className="card p-4 md:p-6">
						<div className="overflow-x-auto">
							<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-600">
								<thead className="bg-secondary-50 dark:bg-secondary-700">
									<tr>
										<th
											scope="col"
											className="px-4 py-3 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider w-10"
											title="Select for delete"
										>
											{deletableRuns.length > 0 ? (
												<input
													type="checkbox"
													checked={allDeletableSelected}
													onChange={handleToggleSelectAll}
													className="h-4 w-4 text-primary-600 focus:ring-primary-500 border-secondary-300 dark:border-secondary-600 rounded cursor-pointer"
													title="Select all deletable runs"
													aria-label="Select all deletable runs"
												/>
											) : null}
										</th>
										<th
											scope="col"
											className="px-4 py-3 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider w-10"
											title="Select for approve"
										>
											{approvableRuns.length > 0 ? (
												<input
													type="checkbox"
													checked={allApprovableSelected}
													onChange={handleToggleApproveSelectAll}
													className="h-4 w-4 text-primary-600 focus:ring-primary-500 border-secondary-300 dark:border-secondary-600 rounded cursor-pointer"
													title="Select all approvable runs"
													aria-label="Select all approvable runs"
												/>
											) : null}
										</th>
										<th
											scope="col"
											className="px-4 py-3 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider"
										>
											Host
										</th>
										<th
											scope="col"
											className="px-4 py-3 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider"
										>
											Type
										</th>
										<th
											scope="col"
											className="px-4 py-3 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider"
										>
											Status
										</th>
										<th
											scope="col"
											className="px-4 py-3 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider"
										>
											Initiated by
										</th>
										<th
											scope="col"
											className="px-4 py-3 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider"
										>
											Started
										</th>
										<th
											scope="col"
											className="px-4 py-3 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider"
										>
											Completed
										</th>
										<th
											scope="col"
											className="px-4 py-3 text-right text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider"
										>
											Actions
										</th>
									</tr>
								</thead>
								<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-600">
									{runs.map((run) => (
										<tr
											key={run.id}
											className="hover:bg-secondary-50 dark:hover:bg-secondary-700 transition-colors"
										>
											<td className="px-4 py-2 whitespace-nowrap w-10">
												{deletableStatuses.has(run.status) ? (
													<input
														type="checkbox"
														checked={selectedRunIds.has(run.id)}
														onChange={() => handleToggleSelect(run.id)}
														className="h-4 w-4 text-primary-600 focus:ring-primary-500 border-secondary-300 dark:border-secondary-600 rounded cursor-pointer"
														title="Select for delete"
														aria-label="Select for delete"
													/>
												) : null}
											</td>
											<td className="px-4 py-2 whitespace-nowrap w-10">
												{approvableStatuses.has(run.status) ? (
													<input
														type="checkbox"
														checked={selectedApproveIds.has(run.id)}
														onChange={() => handleToggleApproveSelect(run.id)}
														className="h-4 w-4 text-primary-600 focus:ring-primary-500 border-secondary-300 dark:border-secondary-600 rounded cursor-pointer"
														title="Select for approve"
														aria-label="Select for approve"
													/>
												) : null}
											</td>
											<td className="px-4 py-2 whitespace-nowrap text-sm text-secondary-900 dark:text-white">
												{run.hosts?.friendly_name ||
													run.hosts?.hostname ||
													run.host_id}
											</td>
											<td className="px-4 py-2 text-sm text-secondary-900 dark:text-white">
												<PackageListDisplay run={run} />
											</td>
											<td className="px-4 py-2 whitespace-nowrap">
												<div className="flex items-center gap-1 flex-wrap">
													<PatchRunStatusBadge run={run} />
													{run.status === "validated" &&
														run.packages_affected?.length >
															(run.package_names?.length || 1) && (
															<ValidatedBadge />
														)}
												</div>
											</td>
											<td className="px-4 py-2 whitespace-nowrap text-sm text-secondary-600 dark:text-secondary-400">
												{run.triggered_by_username ? (
													<span className="flex items-center gap-1">
														<User className="h-4 w-4" />
														{run.triggered_by_username}
													</span>
												) : (
													" -"
												)}
											</td>
											<td className="px-4 py-2 whitespace-nowrap text-sm text-secondary-600 dark:text-secondary-400">
												{formatDate(run.created_at)}
											</td>
											<td className="px-4 py-2 whitespace-nowrap text-sm text-secondary-600 dark:text-secondary-400">
												{run.completed_at ? formatDate(run.completed_at) : " -"}
											</td>
											<td className="px-4 py-2 whitespace-nowrap text-right">
												<div className="flex items-center justify-end gap-2">
													{run.status === "pending_validation" && (
														<>
															<button
																type="button"
																onClick={() => handleRetryValidation(run.id)}
																disabled={retryingId === run.id}
																className="inline-flex items-center gap-1 text-xs px-2 py-1 rounded border border-secondary-300 dark:border-secondary-600 text-secondary-700 dark:text-secondary-300 hover:bg-secondary-100 dark:hover:bg-secondary-700 disabled:opacity-50"
																title="Re-queue validation (host may have been offline)"
															>
																{retryingId === run.id ? (
																	<RefreshCw className="h-3 w-3 animate-spin" />
																) : (
																	<RefreshCw className="h-3 w-3" />
																)}
																Retry
															</button>
															<button
																type="button"
																onClick={() => handleApprove(run.id)}
																disabled={approvingId === run.id}
																className="inline-flex items-center gap-1 text-xs px-2 py-1 rounded bg-amber-600 text-white hover:bg-amber-700 disabled:opacity-50"
																title="Skip validation and patch immediately"
															>
																{approvingId === run.id ? (
																	<RefreshCw className="h-3 w-3 animate-spin" />
																) : (
																	<PlayCircle className="h-3 w-3" />
																)}
																Skip & Patch
															</button>
														</>
													)}
													{run.status === "validated" && (
														<button
															type="button"
															onClick={() => handleApprove(run.id)}
															disabled={approvingId === run.id}
															className="inline-flex items-center gap-1 text-xs px-2 py-1 rounded bg-primary-600 text-white hover:bg-primary-700 disabled:opacity-50"
															title="Approve this validated run to proceed with patching"
														>
															{approvingId === run.id ? (
																<RefreshCw className="h-3 w-3 animate-spin" />
															) : (
																<PlayCircle className="h-3 w-3" />
															)}
															Approve
														</button>
													)}
													<Link
														to={`/patching/runs/${run.id}`}
														className="text-primary-600 dark:text-primary-400 hover:underline text-sm"
													>
														View
													</Link>
												</div>
											</td>
										</tr>
									))}
								</tbody>
							</table>
						</div>
						{runsPagination.pages > 1 && (
							<div className="flex items-center justify-between mt-4 pt-4 border-t border-secondary-200 dark:border-secondary-600">
								<p className="text-sm text-secondary-500">
									Total {runsPagination.total} run(s)
								</p>
								<div className="flex gap-2">
									<button
										type="button"
										disabled={runsPage <= 1}
										onClick={() => setRunsPage((p) => Math.max(1, p - 1))}
										className="px-3 py-1 rounded border border-secondary-300 dark:border-secondary-600 disabled:opacity-50 text-sm"
									>
										Previous
									</button>
									<button
										type="button"
										disabled={runsPage >= runsPagination.pages}
										onClick={() => setRunsPage((p) => p + 1)}
										className="px-3 py-1 rounded border border-secondary-300 dark:border-secondary-600 disabled:opacity-50 text-sm"
									>
										Next
									</button>
								</div>
							</div>
						)}
					</div>
				</div>
			)}
			{activeTab === "policies" && <PoliciesTab />}
		</div>
	);
};

/* ───────────────────── Policies Tab ───────────────────── */

const delay_type_labels = {
	immediate: "Immediate",
	delayed: "Delayed",
	fixed_time: "Fixed time",
};

function PoliciesTab() {
	const queryClient = useQueryClient();
	const toast = useToast();
	const [showModal, setShowModal] = useState(false);
	const [editingPolicy, setEditingPolicy] = useState(null);
	const [form, setForm] = useState({
		name: "",
		description: "",
		patch_delay_type: "immediate",
		delay_minutes: 60,
		fixed_time_utc: "03:00",
		timezone: "UTC",
	});
	const [expandedPolicyId, setExpandedPolicyId] = useState(null);

	const { data: policies = [], isLoading } = useQuery({
		queryKey: ["patching-policies"],
		queryFn: () => patchingAPI.getPolicies(),
	});

	const { data: hostGroups = [] } = useQuery({
		queryKey: ["hostGroups"],
		queryFn: () => hostGroupsAPI.list().then((res) => res.data),
	});

	const { data: hostsData } = useQuery({
		queryKey: ["hosts-list"],
		queryFn: () => adminHostsAPI.list().then((res) => res.data),
	});
	const hosts = hostsData?.data || [];

	const createMutation = useMutation({
		mutationFn: (data) => patchingAPI.createPolicy(data),
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ["patching-policies"] });
			setShowModal(false);
			resetForm();
			toast.success("Policy created");
		},
		onError: (err) => toast.error(err.response?.data?.error || err.message),
	});

	const updateMutation = useMutation({
		mutationFn: ({ id, data }) => patchingAPI.updatePolicy(id, data),
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ["patching-policies"] });
			setShowModal(false);
			setEditingPolicy(null);
			toast.success("Policy updated");
		},
		onError: (err) => toast.error(err.response?.data?.error || err.message),
	});

	const deleteMutation = useMutation({
		mutationFn: (id) => patchingAPI.deletePolicy(id),
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ["patching-policies"] });
			toast.success("Policy deleted");
		},
		onError: (err) => toast.error(err.response?.data?.error || err.message),
	});

	const resetForm = () =>
		setForm({
			name: "",
			description: "",
			patch_delay_type: "immediate",
			delay_minutes: 60,
			fixed_time_utc: "03:00",
			timezone: "UTC",
		});

	const openCreate = () => {
		setEditingPolicy(null);
		resetForm();
		setShowModal(true);
	};

	const openEdit = (policy) => {
		setEditingPolicy(policy);
		setForm({
			name: policy.name,
			description: policy.description || "",
			patch_delay_type: policy.patch_delay_type || "immediate",
			delay_minutes: policy.delay_minutes ?? 60,
			fixed_time_utc: policy.fixed_time_utc || "03:00",
			timezone: policy.timezone || "UTC",
		});
		setShowModal(true);
	};

	const handleSubmit = (e) => {
		e.preventDefault();
		const payload = {
			name: form.name.trim(),
			description: form.description.trim() || null,
			patch_delay_type: form.patch_delay_type,
			delay_minutes:
				form.patch_delay_type === "delayed" ? Number(form.delay_minutes) : null,
			fixed_time_utc:
				form.patch_delay_type === "fixed_time" ? form.fixed_time_utc : null,
			timezone: form.timezone?.trim() || null,
		};
		if (editingPolicy) {
			updateMutation.mutate({ id: editingPolicy.id, data: payload });
		} else {
			createMutation.mutate(payload);
		}
	};

	const formatSchedule = (policy) => {
		const label =
			delay_type_labels[policy.patch_delay_type] || policy.patch_delay_type;
		if (policy.patch_delay_type === "delayed" && policy.delay_minutes != null) {
			return `${label} (${policy.delay_minutes} min)`;
		}
		if (policy.patch_delay_type === "fixed_time" && policy.fixed_time_utc) {
			return `${label} at ${policy.fixed_time_utc} ${policy.timezone || "UTC"}`;
		}
		return label;
	};

	return (
		<div className="mt-4 space-y-4">
			<div className="flex items-center justify-between">
				<p className="text-sm text-secondary-600 dark:text-secondary-400">
					Patch policies control when patches run. Assign policies to hosts or
					host groups.
				</p>
				<button
					type="button"
					onClick={openCreate}
					className="btn-primary flex items-center gap-2"
				>
					<Plus className="h-4 w-4" />
					Create policy
				</button>
			</div>

			<div className="card p-4 md:p-6">
				{isLoading ? (
					<div className="p-8 text-center text-secondary-500">Loading...</div>
				) : policies.length === 0 ? (
					<div className="p-8 text-center text-secondary-500">
						No policies yet. Create one to control when patches run.
					</div>
				) : (
					<div className="overflow-x-auto">
						<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-600">
							<thead className="bg-secondary-50 dark:bg-secondary-700">
								<tr>
									<th
										scope="col"
										className="px-4 py-3 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider"
									>
										Name
									</th>
									<th
										scope="col"
										className="px-4 py-3 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider"
									>
										Description
									</th>
									<th
										scope="col"
										className="px-4 py-3 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider"
									>
										Schedule
									</th>
									<th
										scope="col"
										className="px-4 py-3 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider"
									>
										Assignments
									</th>
									<th
										scope="col"
										className="px-4 py-3 text-right text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider"
									>
										Actions
									</th>
								</tr>
							</thead>
							<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-600">
								{policies.map((policy) => (
									<Fragment key={policy.id}>
										<tr className="hover:bg-secondary-50 dark:hover:bg-secondary-700 transition-colors">
											<td className="px-4 py-3 whitespace-nowrap text-sm font-medium text-secondary-900 dark:text-white">
												{policy.name}
											</td>
											<td className="px-4 py-3 text-sm text-secondary-600 dark:text-secondary-400 max-w-xs truncate">
												{policy.description || "-"}
											</td>
											<td className="px-4 py-3 whitespace-nowrap text-sm text-secondary-600 dark:text-secondary-400">
												<span className="inline-flex items-center gap-1">
													<Clock className="h-3.5 w-3.5" />
													{formatSchedule(policy)}
												</span>
											</td>
											<td className="px-4 py-3 whitespace-nowrap text-sm">
												<button
													type="button"
													onClick={() =>
														setExpandedPolicyId(
															expandedPolicyId === policy.id ? null : policy.id,
														)
													}
													className="text-primary-600 dark:text-primary-400 hover:underline"
												>
													{expandedPolicyId === policy.id
														? "Hide"
														: `${policy._count?.assignments ?? 0} assignment(s)`}
												</button>
											</td>
											<td className="px-4 py-3 whitespace-nowrap text-right">
												<div className="flex items-center justify-end gap-2">
													<button
														type="button"
														onClick={() => openEdit(policy)}
														className="p-1.5 rounded hover:bg-secondary-100 dark:hover:bg-secondary-700 text-secondary-600 dark:text-secondary-300"
														title="Edit"
													>
														<Edit className="h-4 w-4" />
													</button>
													<button
														type="button"
														onClick={() => {
															if (
																window.confirm(
																	`Delete policy "${policy.name}"?`,
																)
															)
																deleteMutation.mutate(policy.id);
														}}
														className="p-1.5 rounded hover:bg-red-100 dark:hover:bg-red-900/30 text-red-600"
														title="Delete"
													>
														<Trash2 className="h-4 w-4" />
													</button>
												</div>
											</td>
										</tr>
										{expandedPolicyId === policy.id && (
											<tr key={`${policy.id}-assignments`}>
												<td colSpan={5} className="p-0">
													<PolicyAssignments
														policy={policy}
														hosts={hosts}
														hostGroups={hostGroups}
														onUpdate={() =>
															queryClient.invalidateQueries({
																queryKey: ["patching-policies"],
															})
														}
													/>
												</td>
											</tr>
										)}
									</Fragment>
								))}
							</tbody>
						</table>
					</div>
				)}
			</div>

			{/* Create/Edit Modal */}
			{showModal && (
				<div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50">
					<div className="bg-white dark:bg-secondary-800 rounded-lg shadow-xl max-w-md w-full max-h-[90vh] overflow-y-auto">
						<div className="flex items-center justify-between p-4 border-b border-secondary-200 dark:border-secondary-600">
							<h3 className="text-lg font-semibold text-secondary-900 dark:text-white">
								{editingPolicy ? "Edit policy" : "Create policy"}
							</h3>
							<button
								type="button"
								onClick={() => setShowModal(false)}
								className="p-1 rounded hover:bg-secondary-100 dark:hover:bg-secondary-700"
							>
								<X className="h-5 w-5" />
							</button>
						</div>
						<form onSubmit={handleSubmit} className="p-4 space-y-4">
							<div>
								<label className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-1">
									Name
								</label>
								<input
									type="text"
									value={form.name}
									onChange={(e) =>
										setForm((f) => ({ ...f, name: e.target.value }))
									}
									className="w-full rounded border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white px-3 py-2"
									required
								/>
							</div>
							<div>
								<label className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-1">
									Description (optional)
								</label>
								<input
									type="text"
									value={form.description}
									onChange={(e) =>
										setForm((f) => ({
											...f,
											description: e.target.value,
										}))
									}
									className="w-full rounded border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white px-3 py-2"
								/>
							</div>
							<div>
								<label className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-1">
									Patch delay
								</label>
								<select
									value={form.patch_delay_type}
									onChange={(e) =>
										setForm((f) => ({
											...f,
											patch_delay_type: e.target.value,
										}))
									}
									className="w-full rounded border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white px-3 py-2"
								>
									<option value="immediate">Immediate</option>
									<option value="delayed">Delayed (run after N minutes)</option>
									<option value="fixed_time">Fixed time (e.g. 3:00 AM)</option>
								</select>
							</div>
							{form.patch_delay_type === "delayed" && (
								<div>
									<label className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-1">
										Delay (minutes)
									</label>
									<input
										type="number"
										min={1}
										value={form.delay_minutes}
										onChange={(e) =>
											setForm((f) => ({
												...f,
												delay_minutes: e.target.value,
											}))
										}
										className="w-full rounded border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white px-3 py-2"
									/>
								</div>
							)}
							{form.patch_delay_type === "fixed_time" && (
								<>
									<div>
										<label className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-1">
											Time (HH:MM)
										</label>
										<input
											type="text"
											placeholder="03:00"
											value={form.fixed_time_utc}
											onChange={(e) =>
												setForm((f) => ({
													...f,
													fixed_time_utc: e.target.value,
												}))
											}
											className="w-full rounded border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white px-3 py-2"
										/>
									</div>
									<div>
										<label className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-1">
											Timezone
										</label>
										<TimezoneSelect
											value={form.timezone}
											onChange={(e) =>
												setForm((f) => ({
													...f,
													timezone: e.target.value,
												}))
											}
										/>
									</div>
								</>
							)}
							<div className="flex justify-end gap-2 pt-2">
								<button
									type="button"
									onClick={() => setShowModal(false)}
									className="btn-outline"
								>
									Cancel
								</button>
								<button
									type="submit"
									className="btn-primary"
									disabled={
										createMutation.isPending || updateMutation.isPending
									}
								>
									{editingPolicy ? "Update" : "Create"}
								</button>
							</div>
						</form>
					</div>
				</div>
			)}
		</div>
	);
}

/* ───────────────── Policy Assignments (inline) ───────────────── */

function PolicyAssignments({ policy, hosts, hostGroups, onUpdate }) {
	const queryClient = useQueryClient();
	const toast = useToast();
	const [addTargetType, setAddTargetType] = useState("host");
	const [addTargetId, setAddTargetId] = useState("");
	const [addExclusionHostId, setAddExclusionHostId] = useState("");

	const { data: fullPolicy } = useQuery({
		queryKey: ["patching-policy", policy.id],
		queryFn: () => patchingAPI.getPolicyById(policy.id),
		enabled: !!policy.id,
	});

	const assignments = fullPolicy?.assignments || [];
	const exclusions = fullPolicy?.exclusions || [];

	const addAssignmentMutation = useMutation({
		mutationFn: () =>
			patchingAPI.addPolicyAssignment(policy.id, addTargetType, addTargetId),
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ["patching-policies"] });
			queryClient.invalidateQueries({
				queryKey: ["patching-policy", policy.id],
			});
			setAddTargetId("");
			onUpdate?.();
			toast.success("Assignment added");
		},
		onError: (err) => toast.error(err.response?.data?.error || err.message),
	});

	const removeAssignmentMutation = useMutation({
		mutationFn: (assignmentId) =>
			patchingAPI.removePolicyAssignment(policy.id, assignmentId),
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ["patching-policies"] });
			queryClient.invalidateQueries({
				queryKey: ["patching-policy", policy.id],
			});
			onUpdate?.();
			toast.success("Assignment removed");
		},
		onError: (err) => toast.error(err.response?.data?.error || err.message),
	});

	const addExclusionMutation = useMutation({
		mutationFn: () =>
			patchingAPI.addPolicyExclusion(policy.id, addExclusionHostId),
		onSuccess: () => {
			queryClient.invalidateQueries({
				queryKey: ["patching-policy", policy.id],
			});
			setAddExclusionHostId("");
			onUpdate?.();
			toast.success("Exclusion added");
		},
		onError: (err) => toast.error(err.response?.data?.error || err.message),
	});

	const removeExclusionMutation = useMutation({
		mutationFn: (hostId) =>
			patchingAPI.removePolicyExclusion(policy.id, hostId),
		onSuccess: () => {
			queryClient.invalidateQueries({
				queryKey: ["patching-policy", policy.id],
			});
			onUpdate?.();
			toast.success("Exclusion removed");
		},
		onError: (err) => toast.error(err.response?.data?.error || err.message),
	});

	const getTargetLabel = (a) => {
		if (a.target_type === "host") {
			const h = hosts.find((x) => x.id === a.target_id);
			return h?.friendly_name || h?.hostname || a.target_id;
		}
		const g = hostGroups.find((x) => x.id === a.target_id);
		return g?.name || a.target_id;
	};

	return (
		<div className="px-4 pb-4 pt-3 bg-secondary-50 dark:bg-secondary-900/50 border-t border-secondary-200 dark:border-secondary-600">
			<div className="space-y-3">
				<div className="flex items-center gap-2 text-sm font-medium text-secondary-700 dark:text-secondary-300">
					<Users className="h-4 w-4" />
					Applied to
				</div>
				{assignments.length === 0 ? (
					<p className="text-sm text-secondary-500">
						No assignments. Add a host or host group.
					</p>
				) : (
					<ul className="flex flex-wrap gap-2">
						{assignments.map((a) => (
							<li
								key={a.id}
								className="inline-flex items-center gap-1 px-2 py-1 rounded bg-secondary-200 dark:bg-secondary-700 text-sm"
							>
								{a.target_type === "host" ? (
									<Server className="h-3 w-3" />
								) : (
									<Users className="h-3 w-3" />
								)}
								{getTargetLabel(a)}
								<button
									type="button"
									onClick={() => removeAssignmentMutation.mutate(a.id)}
									className="ml-1 text-secondary-500 hover:text-red-600"
								>
									<X className="h-3 w-3" />
								</button>
							</li>
						))}
					</ul>
				)}
				<div className="flex flex-wrap items-center gap-2">
					<select
						value={addTargetType}
						onChange={(e) => {
							setAddTargetType(e.target.value);
							setAddTargetId("");
						}}
						className="rounded border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white text-sm px-2 py-1"
					>
						<option value="host">Host</option>
						<option value="host_group">Host group</option>
					</select>
					<select
						value={addTargetId}
						onChange={(e) => setAddTargetId(e.target.value)}
						className="rounded border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white text-sm px-2 py-1 min-w-[160px]"
					>
						<option value="">
							Select {addTargetType === "host" ? "host" : "group"}...
						</option>
						{addTargetType === "host"
							? hosts.map((h) => (
									<option key={h.id} value={h.id}>
										{h.friendly_name || h.hostname || h.id}
									</option>
								))
							: hostGroups.map((g) => (
									<option key={g.id} value={g.id}>
										{g.name}
									</option>
								))}
					</select>
					<button
						type="button"
						onClick={() => addTargetId && addAssignmentMutation.mutate()}
						disabled={!addTargetId || addAssignmentMutation.isPending}
						className="btn-outline text-sm py-1"
					>
						Add
					</button>
				</div>

				<div className="flex items-center gap-2 text-sm font-medium text-secondary-700 dark:text-secondary-300 pt-2 border-t border-secondary-200 dark:border-secondary-600">
					<Clock className="h-4 w-4" />
					Exclusions (hosts excluded from this policy when applied via group)
				</div>
				{exclusions.length === 0 ? (
					<p className="text-sm text-secondary-500">No exclusions.</p>
				) : (
					<ul className="flex flex-wrap gap-2">
						{exclusions.map((exc) => (
							<li
								key={exc.id}
								className="inline-flex items-center gap-1 px-2 py-1 rounded bg-amber-100 dark:bg-amber-900/30 text-sm"
							>
								{exc.hosts?.friendly_name || exc.hosts?.hostname || exc.host_id}
								<button
									type="button"
									onClick={() => removeExclusionMutation.mutate(exc.host_id)}
									className="ml-1 text-amber-700 dark:text-amber-400 hover:text-red-600"
								>
									<X className="h-3 w-3" />
								</button>
							</li>
						))}
					</ul>
				)}
				<div className="flex flex-wrap items-center gap-2">
					<select
						value={addExclusionHostId}
						onChange={(e) => setAddExclusionHostId(e.target.value)}
						className="rounded border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white text-sm px-2 py-1 min-w-[160px]"
					>
						<option value="">Select host to exclude...</option>
						{hosts.map((h) => (
							<option key={h.id} value={h.id}>
								{h.friendly_name || h.hostname || h.id}
							</option>
						))}
					</select>
					<button
						type="button"
						onClick={() => addExclusionHostId && addExclusionMutation.mutate()}
						disabled={!addExclusionHostId || addExclusionMutation.isPending}
						className="btn-outline text-sm py-1"
					>
						Exclude host
					</button>
				</div>
			</div>
		</div>
	);
}

export default Patching;
