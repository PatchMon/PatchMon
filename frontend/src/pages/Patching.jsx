import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
	AlertTriangle,
	CheckCircle,
	Clock,
	History,
	LayoutDashboard,
	ListChecks,
	Package,
	PlayCircle,
	RefreshCw,
	Server,
	User,
	XCircle,
} from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";
import { patchingAPI } from "../utils/patchingApi";

const PATCHING_TABS = [
	{ id: "overview", label: "Overview", icon: LayoutDashboard },
	{ id: "runs", label: "Runs & History", icon: History },
];

const statusBadge = (status) => {
	const map = {
		queued: {
			class:
				"bg-secondary-100 text-secondary-800 dark:bg-secondary-700 dark:text-secondary-200",
			label: "Queued",
		},
		pending_validation: {
			class:
				"bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200",
			label: "Pending validation",
		},
		validated: {
			class:
				"bg-emerald-100 text-emerald-800 dark:bg-emerald-900 dark:text-emerald-200",
			label: "Validated",
		},
		scheduled: {
			class:
				"bg-secondary-100 text-secondary-800 dark:bg-secondary-700 dark:text-secondary-200",
			label: "Scheduled",
		},
		running: {
			class: "bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200",
			label: "Running",
		},
		completed: {
			class:
				"bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200",
			label: "Completed",
		},
		failed: {
			class: "bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200",
			label: "Failed",
		},
		cancelled: {
			class:
				"bg-secondary-100 text-secondary-600 dark:bg-secondary-600 dark:text-secondary-200",
			label: "Cancelled",
		},
	};
	const s = map[status] || {
		class: "bg-secondary-100 text-secondary-800",
		label: status,
	};
	return (
		<span
			className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${s.class}`}
		>
			{s.label}
		</span>
	);
};

const ValidatedBadge = () => (
	<span className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs font-medium bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200">
		<AlertTriangle className="h-3 w-3" />
		Extra deps
	</span>
);

// PackageDisplay shows requested packages and dependencies if available
const PackageDisplay = ({ run }) => {
	if (run.patch_type !== "patch_package") {
		return "Patch all";
	}

	const requested =
		Array.isArray(run.package_names) && run.package_names.length > 0
			? run.package_names
			: run.package_name
				? [run.package_name]
				: [];
	const requestedSet = new Set(requested.map((n) => n.toLowerCase()));

	// Find extra dependencies (packages in packages_affected that aren't in requested)
	const extraDeps =
		run.packages_affected?.filter((p) => !requestedSet.has(p.toLowerCase())) ||
		[];

	if (requested.length === 0) {
		return "—";
	}

	return (
		<span className="flex items-center gap-1 flex-wrap">
			<Package className="h-4 w-4 shrink-0" />
			<span>{requested.join(", ")}</span>
			{extraDeps.length > 0 && (
				<span className="text-xs text-amber-600 dark:text-amber-400">
					(+ {extraDeps.length} dep{extraDeps.length !== 1 ? "s" : ""}:{" "}
					{extraDeps.join(", ")})
				</span>
			)}
		</span>
	);
};

const Patching = () => {
	const [activeTab, setActiveTab] = useState("overview");
	const [runsFilterStatus, setRunsFilterStatus] = useState("");
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
		queryKey: ["patching-runs", runsFilterStatus, runsPage],
		queryFn: () =>
			patchingAPI.getRuns({
				...(runsFilterStatus ? { status: runsFilterStatus } : {}),
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
			queryClient.invalidateQueries(["patching-runs"]);
			queryClient.invalidateQueries(["patching-dashboard"]);
		},
	});
	const [approvingId, setApprovingId] = useState(null);

	const handleApprove = async (runId) => {
		setApprovingId(runId);
		try {
			await approveMutation.mutateAsync(runId);
		} finally {
			setApprovingId(null);
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
	const recent_runs = dashboard?.recent_runs || [];
	const active_runs = dashboard?.active_runs || [];

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

			{/* Summary cards - above tabs like Docker page */}
			<div className="grid grid-cols-2 lg:grid-cols-5 gap-4 mb-6">
				<div className="card p-4">
					<div className="flex items-center">
						<ListChecks className="h-5 w-5 text-primary-600 mr-2" />
						<div>
							<p className="text-sm text-secondary-500 dark:text-secondary-400">
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
							<p className="text-sm text-secondary-500 dark:text-secondary-400">
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
							<p className="text-sm text-secondary-500 dark:text-secondary-400">
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
							<p className="text-sm text-secondary-500 dark:text-secondary-400">
								Failed
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{summary.failed ?? 0}
							</p>
						</div>
					</div>
				</div>
				<Link
					to="/settings/patch-management"
					className="card p-4 hover:shadow-card-hover dark:hover:shadow-card-hover-dark transition-shadow"
				>
					<div className="flex items-center">
						<Server className="h-5 w-5 text-secondary-600 mr-2" />
						<div>
							<p className="text-sm text-secondary-500 dark:text-secondary-400">
								Patch policies
							</p>
							<p className="text-sm font-medium text-primary-600 dark:text-primary-400">
								Manage in Settings
							</p>
						</div>
					</div>
				</Link>
			</div>

			{/* Tabs */}
			<div className="card flex-1 flex flex-col overflow-hidden min-h-0">
				<div className="border-b border-secondary-200 dark:border-secondary-600">
					<nav className="-mb-px flex space-x-8 px-4" aria-label="Tabs">
						{PATCHING_TABS.map((tab) => {
							const Icon = tab.icon;
							return (
								<button
									key={tab.id}
									type="button"
									onClick={() => setActiveTab(tab.id)}
									className={`flex items-center gap-2 py-4 px-1 border-b-2 font-medium text-sm transition-colors whitespace-nowrap ${
										activeTab === tab.id
											? "border-primary-500 text-primary-600 dark:text-primary-400"
											: "border-transparent text-secondary-500 hover:text-secondary-700 dark:text-secondary-400 dark:hover:text-secondary-300"
									}`}
								>
									<Icon className="h-4 w-4" />
									{tab.label}
								</button>
							);
						})}
					</nav>
				</div>

				<div className="flex-1 overflow-auto p-4 md:p-6">
					{activeTab === "overview" && (
						<>
							{/* Active runs */}
							{active_runs?.length > 0 && (
								<div className="card p-4 md:p-6">
									<h3 className="text-lg font-medium text-secondary-900 dark:text-white mb-4 flex items-center gap-2">
										<AlertTriangle className="h-5 w-5 text-amber-500" />
										Active runs
									</h3>
									<div className="overflow-x-auto">
										<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-600">
											<thead className="bg-secondary-50 dark:bg-secondary-700">
												<tr>
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
														className="px-4 py-3 text-right text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider"
													>
														Actions
													</th>
												</tr>
											</thead>
											<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-600">
												{active_runs.map((run) => (
													<tr
														key={run.id}
														className="hover:bg-secondary-50 dark:hover:bg-secondary-700 transition-colors"
													>
														<td className="px-4 py-2 whitespace-nowrap text-sm text-secondary-900 dark:text-white">
															{run.hosts?.friendly_name ||
																run.hosts?.hostname ||
																run.host_id}
														</td>
														<td className="px-4 py-2 whitespace-nowrap text-sm text-secondary-900 dark:text-white">
															{run.patch_type === "patch_package" ? (
																<span className="flex items-center gap-1">
																	<Package className="h-4 w-4" />{" "}
																	{Array.isArray(run.package_names) &&
																	run.package_names.length > 0
																		? run.package_names.join(", ")
																		: run.package_name || "—"}
																</span>
															) : (
																"Patch all"
															)}
														</td>
														<td className="px-4 py-2 whitespace-nowrap">
															<div className="flex items-center gap-1 flex-wrap">
																{statusBadge(run.status)}
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
																"—"
															)}
														</td>
														<td className="px-4 py-2 whitespace-nowrap text-sm text-secondary-600 dark:text-secondary-400">
															{run.started_at
																? new Date(run.started_at).toLocaleString()
																: new Date(run.created_at).toLocaleString()}
														</td>
														<td className="px-4 py-2 whitespace-nowrap text-right">
															<Link
																to={`/patching/runs/${run.id}`}
																className="text-primary-600 dark:text-primary-400 hover:underline text-sm"
															>
																View
															</Link>
														</td>
													</tr>
												))}
											</tbody>
										</table>
									</div>
								</div>
							)}

							{/* Recent runs */}
							<div className="card p-4 md:p-6">
								<h3 className="text-lg font-medium text-secondary-900 dark:text-white mb-4">
									Recent runs
								</h3>
								{recent_runs.length === 0 ? (
									<p className="text-secondary-500 dark:text-secondary-400 text-sm">
										No patch runs yet. Trigger a patch from Packages or a host.
									</p>
								) : (
									<div className="overflow-x-auto">
										<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-600">
											<thead className="bg-secondary-50 dark:bg-secondary-700">
												<tr>
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
												{recent_runs.map((run) => (
													<tr
														key={run.id}
														className="hover:bg-secondary-50 dark:hover:bg-secondary-700 transition-colors"
													>
														<td className="px-4 py-2 whitespace-nowrap text-sm text-secondary-900 dark:text-white">
															{run.hosts?.friendly_name ||
																run.hosts?.hostname ||
																run.host_id}
														</td>
														<td className="px-4 py-2 whitespace-nowrap text-sm text-secondary-900 dark:text-white">
															{run.patch_type === "patch_package" ? (
																<span className="flex items-center gap-1">
																	<Package className="h-4 w-4" />{" "}
																	{Array.isArray(run.package_names) &&
																	run.package_names.length > 0
																		? run.package_names.join(", ")
																		: run.package_name || "—"}
																</span>
															) : (
																"Patch all"
															)}
														</td>
														<td className="px-4 py-2 whitespace-nowrap">
															<div className="flex items-center gap-1 flex-wrap">
																{statusBadge(run.status)}
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
																"—"
															)}
														</td>
														<td className="px-4 py-2 whitespace-nowrap text-sm text-secondary-600 dark:text-secondary-400">
															{run.completed_at
																? new Date(run.completed_at).toLocaleString()
																: "—"}
														</td>
														<td className="px-4 py-2 whitespace-nowrap text-right">
															<Link
																to={`/patching/runs/${run.id}`}
																className="text-primary-600 dark:text-primary-400 hover:underline text-sm"
															>
																View output
															</Link>
														</td>
													</tr>
												))}
											</tbody>
										</table>
									</div>
								)}
							</div>
						</>
					)}

					{activeTab === "runs" && (
						<>
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
									<option value="validated">Validated</option>
									<option value="scheduled">Scheduled</option>
									<option value="running">Running</option>
									<option value="completed">Completed</option>
									<option value="failed">Failed</option>
									<option value="cancelled">Cancelled</option>
								</select>
							</div>
							<div className="card p-4 md:p-6">
								<div className="overflow-x-auto">
									<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-600">
										<thead className="bg-secondary-50 dark:bg-secondary-700">
											<tr>
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
													<td className="px-4 py-2 whitespace-nowrap text-sm text-secondary-900 dark:text-white">
														{run.hosts?.friendly_name ||
															run.hosts?.hostname ||
															run.host_id}
													</td>
													<td className="px-4 py-2 whitespace-nowrap text-sm text-secondary-900 dark:text-white">
														<PackageDisplay run={run} />
													</td>
													<td className="px-4 py-2 whitespace-nowrap">
														<div className="flex items-center gap-1 flex-wrap">
															{statusBadge(run.status)}
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
															"—"
														)}
													</td>
													<td className="px-4 py-2 whitespace-nowrap text-sm text-secondary-600 dark:text-secondary-400">
														{new Date(run.created_at).toLocaleString()}
													</td>
													<td className="px-4 py-2 whitespace-nowrap text-sm text-secondary-600 dark:text-secondary-400">
														{run.completed_at
															? new Date(run.completed_at).toLocaleString()
															: "—"}
													</td>
													<td className="px-4 py-2 whitespace-nowrap text-right">
														<div className="flex items-center justify-end gap-2">
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
																View output
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
						</>
					)}
				</div>
			</div>
		</div>
	);
};

export default Patching;
