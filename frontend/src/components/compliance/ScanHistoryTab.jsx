import { useQuery } from "@tanstack/react-query";
import { format, formatDistanceToNow } from "date-fns";
import {
	CheckCircle,
	ChevronLeft,
	ChevronRight,
	Clock,
	RefreshCw,
	Search,
	Shield,
	XCircle,
} from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";
import { complianceAPI } from "../../utils/complianceApi";

const STATUS_CONFIG = {
	completed: {
		icon: CheckCircle,
		label: "Completed",
		colour: "text-green-600 dark:text-green-400",
		bg: "bg-green-100 dark:bg-green-900/30",
	},
	failed: {
		icon: XCircle,
		label: "Failed",
		colour: "text-red-600 dark:text-red-400",
		bg: "bg-red-100 dark:bg-red-900/30",
	},
	running: {
		icon: RefreshCw,
		label: "Running",
		colour: "text-blue-600 dark:text-blue-400",
		bg: "bg-blue-100 dark:bg-blue-900/30",
	},
};

const PROFILE_TYPE_LABELS = {
	openscap: "OpenSCAP",
	"docker-bench": "Docker Bench",
};

const format_duration = (ms) => {
	if (ms == null) return "—";
	if (ms < 1000) return `${ms}ms`;
	const seconds = ms / 1000;
	if (seconds < 60) return `${seconds.toFixed(1)}s`;
	const minutes = Math.floor(seconds / 60);
	const remaining = Math.round(seconds % 60);
	return `${minutes}m ${remaining}s`;
};

const ScanHistoryTab = ({ scanned_hosts }) => {
	const [page, set_page] = useState(0);
	const [status_filter, set_status_filter] = useState("");
	const [type_filter, set_type_filter] = useState("");
	const [host_filter, set_host_filter] = useState("");
	const [search, set_search] = useState("");
	const limit = 25;

	const { data, isLoading } = useQuery({
		queryKey: [
			"compliance-scan-history",
			page,
			status_filter,
			type_filter,
			host_filter,
		],
		queryFn: () => {
			const params = { limit, offset: page * limit };
			if (status_filter) params.status = status_filter;
			if (type_filter) params.profile_type = type_filter;
			if (host_filter) params.host_id = host_filter;
			return complianceAPI.getScanHistory(params);
		},
		staleTime: 30 * 1000,
		keepPreviousData: true,
	});

	const scans = data?.scans || [];
	const { total = 0, total_pages = 1 } = data?.pagination || {};

	const filtered_scans = search.trim()
		? scans.filter(
				(s) =>
					s.host_name.toLowerCase().includes(search.toLowerCase()) ||
					s.profile_name.toLowerCase().includes(search.toLowerCase()),
			)
		: scans;

	const host_options = scanned_hosts || [];

	return (
		<div className="space-y-4">
			{/* Filters */}
			<div className="card p-4">
				<div className="flex flex-wrap items-center gap-3">
					<div className="relative flex-1 min-w-[200px] max-w-sm">
						<Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-secondary-400" />
						<input
							type="text"
							placeholder="Search host or profile..."
							value={search}
							onChange={(e) => set_search(e.target.value)}
							className="w-full pl-9 pr-3 py-2 bg-secondary-50 dark:bg-secondary-700 border border-secondary-200 dark:border-secondary-600 rounded-lg text-sm text-secondary-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-primary-500"
						/>
					</div>

					<select
						value={status_filter}
						onChange={(e) => {
							set_status_filter(e.target.value);
							set_page(0);
						}}
						className="px-3 py-2 bg-secondary-50 dark:bg-secondary-700 border border-secondary-200 dark:border-secondary-600 rounded-lg text-sm text-secondary-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-primary-500"
					>
						<option value="">All Statuses</option>
						<option value="completed">Completed</option>
						<option value="failed">Failed</option>
						<option value="running">Running</option>
					</select>

					<select
						value={type_filter}
						onChange={(e) => {
							set_type_filter(e.target.value);
							set_page(0);
						}}
						className="px-3 py-2 bg-secondary-50 dark:bg-secondary-700 border border-secondary-200 dark:border-secondary-600 rounded-lg text-sm text-secondary-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-primary-500"
					>
						<option value="">All Types</option>
						<option value="openscap">OpenSCAP</option>
						<option value="docker-bench">Docker Bench</option>
					</select>

					{host_options.length > 0 && (
						<select
							value={host_filter}
							onChange={(e) => {
								set_host_filter(e.target.value);
								set_page(0);
							}}
							className="px-3 py-2 bg-secondary-50 dark:bg-secondary-700 border border-secondary-200 dark:border-secondary-600 rounded-lg text-sm text-secondary-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-primary-500 max-w-[220px] truncate"
						>
							<option value="">All Hosts</option>
							{host_options.map((h) => (
								<option key={h.host_id} value={h.host_id}>
									{h.friendly_name || h.hostname || h.host_id}
								</option>
							))}
						</select>
					)}

					{(status_filter || type_filter || host_filter || search) && (
						<button
							type="button"
							onClick={() => {
								set_status_filter("");
								set_type_filter("");
								set_host_filter("");
								set_search("");
								set_page(0);
							}}
							className="text-xs text-primary-600 hover:text-primary-800 dark:text-primary-400 dark:hover:text-primary-300"
						>
							Clear filters
						</button>
					)}

					<div className="ml-auto flex items-center gap-3">
						{total > 0 && (
							<span className="text-sm text-secondary-500 dark:text-secondary-400 whitespace-nowrap">
								{total} scan{total !== 1 ? "s" : ""} found
							</span>
						)}
						{total_pages > 1 && (
							<Pagination
								page={page}
								total_pages={total_pages}
								set_page={set_page}
							/>
						)}
					</div>
				</div>
			</div>

			{/* Table */}
			<div className="card overflow-hidden">
				{isLoading ? (
					<div className="flex items-center justify-center py-16">
						<RefreshCw className="h-6 w-6 animate-spin text-secondary-400" />
					</div>
				) : filtered_scans.length === 0 ? (
					<div className="text-center py-16 text-secondary-400 dark:text-secondary-500">
						<Shield className="h-10 w-10 mx-auto mb-3" />
						<p className="font-medium text-secondary-700 dark:text-secondary-300">
							No scan history found
						</p>
						<p className="text-sm mt-1">
							Completed and failed scans will appear here.
						</p>
					</div>
				) : (
					<div className="overflow-x-auto">
						<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-600">
							<thead className="bg-secondary-50 dark:bg-secondary-700">
								<tr>
									<th className="px-3 sm:px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider whitespace-nowrap">
										Host
									</th>
									<th className="px-3 sm:px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider whitespace-nowrap">
										Profile
									</th>
									<th className="px-3 sm:px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider whitespace-nowrap">
										Status
									</th>
									<th className="px-3 sm:px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider whitespace-nowrap">
										Outcome
									</th>
									<th className="px-3 sm:px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider whitespace-nowrap">
										Duration
									</th>
									<th className="px-3 sm:px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider whitespace-nowrap">
										Rules
									</th>
									<th className="px-3 sm:px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider whitespace-nowrap">
										Started
									</th>
								</tr>
							</thead>
							<tbody className="divide-y divide-secondary-200 dark:divide-secondary-700">
								{filtered_scans.map((scan) => {
									const cfg =
										STATUS_CONFIG[scan.status] || STATUS_CONFIG.completed;
									const StatusIcon = cfg.icon;
									const score_pct =
										scan.score != null ? `${Math.round(scan.score)}%` : "—";

									return (
										<tr
											key={scan.id}
											className="hover:bg-secondary-50 dark:hover:bg-secondary-700/50 transition-colors"
										>
											<td className="px-3 sm:px-4 py-2 whitespace-nowrap">
												<Link
													to={`/compliance/hosts/${scan.host_id}`}
													className="text-sm font-medium text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300 hover:underline"
												>
													{scan.host_name}
												</Link>
											</td>

											<td className="px-3 sm:px-4 py-2 whitespace-nowrap">
												<div className="flex items-center gap-2">
													<span
														className={`inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs font-medium ${
															scan.profile_type === "docker-bench"
																? "bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300"
																: "bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300"
														}`}
													>
														{PROFILE_TYPE_LABELS[scan.profile_type] ||
															scan.profile_type}
													</span>
													<span className="text-sm text-secondary-500 dark:text-secondary-400 truncate max-w-[180px]">
														{scan.profile_name}
													</span>
												</div>
											</td>

											<td className="px-3 sm:px-4 py-2 whitespace-nowrap">
												<span
													className={`inline-flex items-center gap-1.5 text-sm font-medium ${cfg.colour}`}
												>
													<StatusIcon
														className={`h-4 w-4 ${scan.status === "running" ? "animate-spin" : ""}`}
													/>
													{cfg.label}
												</span>
											</td>

											<td className="px-3 sm:px-4 py-2 whitespace-nowrap">
												{scan.status === "completed" ? (
													<div className="flex items-center gap-3 text-sm">
														<span className="text-green-600 dark:text-green-400 font-medium">
															{scan.passed} pass
														</span>
														{scan.failed > 0 && (
															<span className="text-red-600 dark:text-red-400 font-medium">
																{scan.failed} fail
															</span>
														)}
														{scan.warnings > 0 && (
															<span className="text-yellow-600 dark:text-yellow-400 font-medium">
																{scan.warnings} warn
															</span>
														)}
														<span className="text-secondary-400">
															{score_pct}
														</span>
													</div>
												) : scan.status === "failed" ? (
													<span
														className="text-sm text-red-500 dark:text-red-400 max-w-[200px] truncate block"
														title={scan.error_message || ""}
													>
														{scan.error_message || "Scan failed"}
													</span>
												) : (
													<span className="text-sm text-secondary-400">
														In progress…
													</span>
												)}
											</td>

											<td className="px-3 sm:px-4 py-2 whitespace-nowrap text-sm text-secondary-500 dark:text-secondary-400">
												<div className="flex items-center gap-1.5">
													<Clock className="h-3.5 w-3.5" />
													{scan.status === "running"
														? formatDistanceToNow(new Date(scan.started_at), {
																addSuffix: false,
															})
														: format_duration(scan.duration_ms)}
												</div>
											</td>

											<td className="px-3 sm:px-4 py-2 whitespace-nowrap text-sm text-secondary-500 dark:text-secondary-400">
												{scan.results_stored} / {scan.total_rules}
											</td>

											<td
												className="px-3 sm:px-4 py-2 whitespace-nowrap text-sm text-secondary-500 dark:text-secondary-400"
												title={
													scan.started_at
														? format(
																new Date(scan.started_at),
																"dd MMM yyyy HH:mm",
															)
														: ""
												}
											>
												{scan.started_at
													? formatDistanceToNow(new Date(scan.started_at), {
															addSuffix: true,
														})
													: "—"}
											</td>
										</tr>
									);
								})}
							</tbody>
						</table>
					</div>
				)}
			</div>

			{/* Bottom pagination */}
			{total_pages > 1 && (
				<div className="flex justify-center">
					<Pagination
						page={page}
						total_pages={total_pages}
						set_page={set_page}
					/>
				</div>
			)}
		</div>
	);
};

const Pagination = ({ page, total_pages, set_page }) => (
	<div className="flex items-center gap-2">
		<button
			type="button"
			disabled={page === 0}
			onClick={() => set_page(page - 1)}
			className="p-1.5 rounded-lg border border-secondary-200 dark:border-secondary-600 disabled:opacity-40 hover:bg-secondary-100 dark:hover:bg-secondary-700 transition-colors"
		>
			<ChevronLeft className="h-4 w-4" />
		</button>
		<span className="text-sm text-secondary-600 dark:text-secondary-400 tabular-nums">
			{page + 1} / {total_pages}
		</span>
		<button
			type="button"
			disabled={page + 1 >= total_pages}
			onClick={() => set_page(page + 1)}
			className="p-1.5 rounded-lg border border-secondary-200 dark:border-secondary-600 disabled:opacity-40 hover:bg-secondary-100 dark:hover:bg-secondary-700 transition-colors"
		>
			<ChevronRight className="h-4 w-4" />
		</button>
	</div>
);

export default ScanHistoryTab;
