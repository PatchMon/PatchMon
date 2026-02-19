import { useQuery } from "@tanstack/react-query";
import { formatDistanceToNow } from "date-fns";
import {
	Calendar,
	ChevronLeft,
	ChevronRight,
	RefreshCw,
	Shield,
	ShieldAlert,
	ShieldCheck,
	ShieldX,
} from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";
import { complianceAPI } from "../../utils/complianceApi";

const SEVERITY_COLORS = {
	critical: "bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300",
	high: "bg-orange-100 text-orange-800 dark:bg-orange-900/30 dark:text-orange-300",
	medium:
		"bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-300",
	low: "bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300",
	unknown:
		"bg-secondary-100 text-secondary-600 dark:bg-secondary-700 dark:text-secondary-400",
};

const LIMIT = 25;

const normalize_status_display = (status) => {
	if (!status) return "—";
	const s = String(status).toLowerCase();
	if (["fail", "failed", "failure"].includes(s)) return "Fail";
	if (["pass", "passed"].includes(s)) return "Pass";
	if (["warn", "warning", "warned"].includes(s)) return "Warn";
	if (["skip", "skipped", "notapplicable", "not_applicable"].includes(s))
		return "N/A";
	return status;
};

const get_status_icon = (status) => {
	const s = String(status).toLowerCase();
	if (["fail", "failed", "failure"].includes(s))
		return <ShieldX className="h-4 w-4 text-red-500" title="Failed" />;
	if (["warn", "warning", "warned"].includes(s))
		return <ShieldAlert className="h-4 w-4 text-yellow-500" title="Warning" />;
	if (["pass", "passed"].includes(s))
		return <ShieldCheck className="h-4 w-4 text-green-500" title="Passed" />;
	return <Shield className="h-4 w-4 text-secondary-400" title="N/A" />;
};

const HostComplianceDetail = ({ hostId }) => {
	const [status_filter, set_status_filter] = useState("");
	const [severity_filter, set_severity_filter] = useState("");
	const [page, set_page] = useState(0);

	const { data: latest_scan, isLoading: scan_loading } = useQuery({
		queryKey: ["compliance-latest", hostId],
		queryFn: async () => {
			try {
				const res = await complianceAPI.getLatestScan(hostId);
				return res.data;
			} catch (e) {
				if (e.response?.status === 404) return null;
				throw e;
			}
		},
		enabled: !!hostId,
		staleTime: 30 * 1000,
		refetchOnWindowFocus: false,
		retry: false,
	});

	const scan_id = latest_scan?.id;
	const { data: scan_results_data, isLoading: results_loading } = useQuery({
		queryKey: [
			"compliance-scan-results",
			scan_id,
			page,
			status_filter,
			severity_filter,
		],
		queryFn: () =>
			complianceAPI.getScanResults(scan_id, {
				limit: LIMIT,
				offset: page * LIMIT,
				...(status_filter ? { status: status_filter } : {}),
				...(severity_filter ? { severity: severity_filter } : {}),
			}),
		enabled: !!scan_id,
		staleTime: 60 * 1000,
		refetchOnWindowFocus: false,
	});

	const results = scan_results_data?.results ?? [];
	const pagination = scan_results_data?.pagination ?? {};
	const total = pagination.total ?? 0;
	const total_pages = Math.ceil(total / LIMIT);

	const passed = latest_scan?.passed ?? 0;
	const failed = latest_scan?.failed ?? 0;
	const warnings_count = latest_scan?.warnings ?? 0;
	const na_count =
		(latest_scan?.skipped ?? 0) + (latest_scan?.not_applicable ?? 0);
	const completed_at = latest_scan?.completed_at
		? new Date(latest_scan.completed_at)
		: null;
	const profile_type = latest_scan?.compliance_profiles?.type;
	const profile_label =
		profile_type === "docker-bench" ? "Docker Bench" : "OpenSCAP";

	if (scan_loading) {
		return (
			<div className="flex items-center justify-center py-12">
				<RefreshCw className="h-8 w-8 animate-spin text-secondary-400" />
			</div>
		);
	}

	return (
		<div className="space-y-6">
			{/* 5 summary cards — click to filter the rule results table below */}
			<div className="grid grid-cols-2 lg:grid-cols-5 gap-4 mb-6">
				<button
					type="button"
					onClick={() => {
						set_status_filter(status_filter === "pass" ? "" : "pass");
						set_page(0);
					}}
					className={`card p-4 text-left transition-shadow ${status_filter === "pass" ? "ring-2 ring-green-500" : "hover:ring-2 hover:ring-green-500/40"}`}
					title="Filter to passing rules"
				>
					<div className="flex items-center">
						<div className="flex-shrink-0">
							<ShieldCheck className="h-5 w-5 text-green-600 mr-2" />
						</div>
						<div className="w-0 flex-1">
							<p className="text-sm text-secondary-500 dark:text-white">
								Passed rules
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{passed}
							</p>
						</div>
					</div>
				</button>
				<button
					type="button"
					onClick={() => {
						set_status_filter(status_filter === "fail" ? "" : "fail");
						set_page(0);
					}}
					className={`card p-4 text-left transition-shadow ${status_filter === "fail" ? "ring-2 ring-red-500" : "hover:ring-2 hover:ring-red-500/40"}`}
					title="Filter to failing rules"
				>
					<div className="flex items-center">
						<div className="flex-shrink-0">
							<ShieldX className="h-5 w-5 text-red-600 mr-2" />
						</div>
						<div className="w-0 flex-1">
							<p className="text-sm text-secondary-500 dark:text-white">
								Failed
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{failed}
							</p>
						</div>
					</div>
				</button>
				<button
					type="button"
					onClick={() => {
						set_status_filter(status_filter === "warn" ? "" : "warn");
						set_page(0);
					}}
					className={`card p-4 text-left transition-shadow ${status_filter === "warn" ? "ring-2 ring-yellow-500" : "hover:ring-2 hover:ring-yellow-500/40"}`}
					title="Filter to warning rules"
				>
					<div className="flex items-center">
						<div className="flex-shrink-0">
							<ShieldAlert className="h-5 w-5 text-yellow-600 mr-2" />
						</div>
						<div className="w-0 flex-1">
							<p className="text-sm text-secondary-500 dark:text-white">
								Warning
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{warnings_count}
							</p>
						</div>
					</div>
				</button>
				<button
					type="button"
					onClick={() => {
						set_status_filter(status_filter === "skipped" ? "" : "skipped");
						set_page(0);
					}}
					className={`card p-4 text-left transition-shadow ${status_filter === "skipped" ? "ring-2 ring-secondary-500" : "hover:ring-2 hover:ring-secondary-500/40"}`}
					title="Filter to skipped/N/A rules"
				>
					<div className="flex items-center">
						<div className="flex-shrink-0">
							<Shield className="h-5 w-5 text-secondary-500 dark:text-secondary-400 mr-2" />
						</div>
						<div className="w-0 flex-1">
							<p className="text-sm text-secondary-500 dark:text-white">N/A</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{na_count}
							</p>
						</div>
					</div>
				</button>
				<div className="card p-4">
					<div className="flex items-center">
						<div className="flex-shrink-0">
							<Calendar className="h-5 w-5 text-primary-600 mr-2" />
						</div>
						<div className="w-0 flex-1">
							<p className="text-sm text-secondary-500 dark:text-white">
								Last scan
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{completed_at
									? formatDistanceToNow(completed_at, { addSuffix: true })
									: "No scan yet"}
							</p>
							{completed_at && (
								<p className="mt-0.5 text-xs text-secondary-500 dark:text-secondary-400 font-normal">
									{completed_at.toLocaleString()}
								</p>
							)}
						</div>
					</div>
				</div>
			</div>

			{/* Rule results - card and table match /hosts and /docker */}
			<div className="card flex-1 flex flex-col overflow-hidden min-h-0">
				<div className="border-b border-secondary-200 dark:border-secondary-600 px-4 py-3">
					<h2 className="text-lg font-semibold text-secondary-900 dark:text-white">
						Rule results
					</h2>
					<p className="text-sm text-secondary-500 dark:text-secondary-400 mt-0.5">
						Results for this host from the latest scan
						{profile_label ? ` (${profile_label})` : ""}
					</p>
				</div>

				{!scan_id ? (
					<div className="p-12 text-center">
						<Shield className="h-12 w-12 text-secondary-400 mx-auto mb-3" />
						<p className="text-secondary-600 dark:text-secondary-400">
							No scan yet. Run a compliance scan from the main Compliance page
							to see results here.
						</p>
						<Link
							to="/compliance"
							className="inline-flex items-center gap-2 mt-4 text-sm font-medium text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300"
						>
							Go to Compliance
						</Link>
					</div>
				) : (
					<>
						{/* Filters - match Docker filter row */}
						<div className="p-4 border-b border-secondary-200 dark:border-secondary-600 flex flex-wrap items-center gap-4">
							<select
								value={status_filter}
								onChange={(e) => {
									set_status_filter(e.target.value);
									set_page(0);
								}}
								className="block w-full sm:w-48 pl-3 pr-10 py-2 text-base border border-secondary-300 dark:border-secondary-600 rounded-md bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-primary-500 sm:text-sm"
							>
								<option value="">All statuses</option>
								<option value="fail">Failed</option>
								<option value="warn">Warning</option>
								<option value="pass">Passed</option>
								<option value="skipped">N/A</option>
							</select>
							<select
								value={severity_filter}
								onChange={(e) => {
									set_severity_filter(e.target.value);
									set_page(0);
								}}
								className="block w-full sm:w-48 pl-3 pr-10 py-2 text-base border border-secondary-300 dark:border-secondary-600 rounded-md bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-primary-500 sm:text-sm"
							>
								<option value="">All severities</option>
								<option value="critical">Critical</option>
								<option value="high">High</option>
								<option value="medium">Medium</option>
								<option value="low">Low</option>
								<option value="unknown">Unknown</option>
							</select>
							{results_loading && (
								<RefreshCw className="h-4 w-4 text-secondary-400 animate-spin" />
							)}
						</div>

						{results_loading && results.length === 0 ? (
							<div className="flex items-center justify-center py-12">
								<RefreshCw className="h-6 w-6 animate-spin text-secondary-400" />
							</div>
						) : results.length === 0 ? (
							<div className="p-8 text-center text-secondary-500 dark:text-secondary-400">
								{total === 0 && !status_filter && !severity_filter ? (
									<>
										<p className="font-medium text-secondary-700 dark:text-secondary-300">
											This scan has no rule results.
										</p>
										<p className="mt-2 text-sm">
											Either the agent did not send a{" "}
											<code className="rounded bg-secondary-200 dark:bg-secondary-600 px-1">
												results
											</code>{" "}
											array in its scan payload, or the backend could not store
											them. Check agent and server logs when the scan was
											submitted for the cause.
										</p>
									</>
								) : (
									"No rules match the current filters."
								)}
							</div>
						) : (
							<>
								<div className="overflow-x-auto">
									<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-600">
										<thead className="bg-secondary-50 dark:bg-secondary-700">
											<tr>
												<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider w-28">
													Status
												</th>
												<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
													Rule
												</th>
												<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider w-24">
													Severity
												</th>
												<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider w-28">
													Profile
												</th>
											</tr>
										</thead>
										<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-600">
											{results.map((row) => {
												const rule = row.compliance_rules;
												const rule_id = rule?.id;
												const title =
													rule?.title || row.rule?.title || row.title || "—";
												const severity =
													rule?.severity || row.severity || "unknown";
												return (
													<tr
														key={row.id}
														className="hover:bg-secondary-50 dark:hover:bg-secondary-700 transition-colors"
													>
														<td className="px-4 py-2 whitespace-nowrap text-sm">
															<div className="flex items-center gap-2">
																{get_status_icon(row.status)}
																<span className="text-secondary-700 dark:text-secondary-300">
																	{normalize_status_display(row.status)}
																</span>
															</div>
														</td>
														<td className="px-4 py-2 text-sm">
															{rule_id ? (
																<Link
																	to={`/compliance/rules/${rule_id}`}
																	className="font-medium text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300"
																>
																	{title}
																</Link>
															) : (
																<span className="text-secondary-900 dark:text-white">
																	{title}
																</span>
															)}
														</td>
														<td className="px-4 py-2 whitespace-nowrap">
															<span
																className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${
																	SEVERITY_COLORS[severity] ||
																	SEVERITY_COLORS.unknown
																}`}
															>
																{(severity || "unknown")
																	.charAt(0)
																	.toUpperCase() +
																	(severity || "unknown").slice(1)}
															</span>
														</td>
														<td className="px-4 py-2 whitespace-nowrap text-sm text-secondary-500 dark:text-secondary-400">
															{profile_type === "docker-bench"
																? "Docker Bench"
																: "OpenSCAP"}
														</td>
													</tr>
												);
											})}
										</tbody>
									</table>
								</div>

								{total_pages > 1 && (
									<div className="flex items-center justify-between px-4 py-3 border-t border-secondary-200 dark:border-secondary-600">
										<p className="text-sm text-secondary-500 dark:text-secondary-400">
											Showing {page * LIMIT + 1}–
											{Math.min((page + 1) * LIMIT, total)} of {total} rules
										</p>
										<div className="flex items-center gap-2">
											<button
												type="button"
												onClick={() => set_page((p) => Math.max(0, p - 1))}
												disabled={page === 0}
												className="p-1.5 rounded-lg bg-secondary-100 dark:bg-secondary-700 text-secondary-600 dark:text-secondary-300 hover:bg-secondary-200 dark:hover:bg-secondary-600 disabled:opacity-50 disabled:cursor-not-allowed inline-flex items-center"
											>
												<ChevronLeft className="h-4 w-4" />
											</button>
											<span className="text-sm text-secondary-500 dark:text-secondary-400">
												Page {page + 1} of {total_pages}
											</span>
											<button
												type="button"
												onClick={() =>
													set_page((p) => Math.min(total_pages - 1, p + 1))
												}
												disabled={page >= total_pages - 1}
												className="p-1.5 rounded-lg bg-secondary-100 dark:bg-secondary-700 text-secondary-600 dark:text-secondary-300 hover:bg-secondary-200 dark:hover:bg-secondary-600 disabled:opacity-50 disabled:cursor-not-allowed inline-flex items-center"
											>
												<ChevronRight className="h-4 w-4" />
											</button>
										</div>
									</div>
								)}
							</>
						)}
					</>
				)}
			</div>
		</div>
	);
};

export default HostComplianceDetail;
