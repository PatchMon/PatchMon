import { useQuery } from "@tanstack/react-query";
import {
	ArrowDown,
	ArrowUp,
	ChevronLeft,
	ChevronRight,
	Filter,
	RefreshCw,
	Search,
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

const SEVERITY_ORDER = ["critical", "high", "medium", "low", "unknown"];

const ScanResultsTab = ({ profileTypeFilter }) => {
	const [search, setSearch] = useState("");
	const [severity_filter, set_severity_filter] = useState("");
	const [status_filter, set_status_filter] = useState("");
	const [page, setPage] = useState(0);
	const [sort_by, set_sort_by] = useState("hosts_failed"); // most fails first, then alphabetical
	const [sort_asc, set_sort_asc] = useState(false); // false = most first for fail count
	const limit = 50;

	const { data, isLoading, isFetching } = useQuery({
		queryKey: [
			"compliance-rules",
			search,
			severity_filter,
			status_filter,
			profileTypeFilter,
			page,
		],
		queryFn: () =>
			complianceAPI.getRules({
				search: search || undefined,
				severity: severity_filter || undefined,
				status: status_filter || undefined,
				profile_type:
					profileTypeFilter && profileTypeFilter !== "all"
						? profileTypeFilter
						: undefined,
				limit,
				offset: page * limit,
			}),
		staleTime: 60 * 1000,
		refetchOnWindowFocus: false,
		keepPreviousData: true,
	});

	const raw_rules = data?.rules || [];
	const total = data?.pagination?.total || 0;
	const total_pages = Math.ceil(total / limit);

	// Status order for sorting: fail=0, warn=1, pass=2, none=3 (lower = more severe)
	const get_status_sort_key = (rule) => {
		if (rule.hosts_failed > 0) return 0;
		if (rule.hosts_warned > 0) return 1;
		if (rule.hosts_passed > 0) return 2;
		return 3;
	};

	const sorted_rules = [...raw_rules].sort((a, b) => {
		let va, vb;
		switch (sort_by) {
			case "status": {
				va = get_status_sort_key(a);
				vb = get_status_sort_key(b);
				if (va !== vb) return sort_asc ? va - vb : vb - va;
				// Secondary: by fail count then warn count, then alphabetical
				const status_secondary = sort_asc
					? a.hosts_failed - b.hosts_failed || a.hosts_warned - b.hosts_warned
					: b.hosts_failed - a.hosts_failed || b.hosts_warned - a.hosts_warned;
				if (status_secondary !== 0) return status_secondary;
				return (a.title || "").localeCompare(b.title || "", undefined, {
					sensitivity: "base",
				});
			}
			case "title":
				va = (a.title || "").toLowerCase();
				vb = (b.title || "").toLowerCase();
				return sort_asc ? va.localeCompare(vb) : vb.localeCompare(va);
			case "severity":
				va = SEVERITY_ORDER.indexOf(a.severity || "unknown");
				vb = SEVERITY_ORDER.indexOf(b.severity || "unknown");
				if (va < 0) va = 99;
				if (vb < 0) vb = 99;
				return sort_asc ? va - vb : vb - va;
			case "profile_type":
				va = a.profile_type || "";
				vb = b.profile_type || "";
				return sort_asc ? va.localeCompare(vb) : vb.localeCompare(va);
			case "hosts_passed":
			case "hosts_failed":
			case "hosts_warned":
			case "total_hosts":
				va = a[sort_by] ?? 0;
				vb = b[sort_by] ?? 0;
				if (va !== vb) return sort_asc ? va - vb : vb - va;
				// Secondary: alphabetical by title
				return (a.title || "").localeCompare(b.title || "", undefined, {
					sensitivity: "base",
				});
			default:
				return 0;
		}
	});

	const handle_sort = (column) => {
		if (sort_by === column) {
			set_sort_asc((prev) => !prev);
		} else {
			set_sort_by(column);
			set_sort_asc(!(column === "status" || column === "hosts_failed"));
		}
	};

	const SortableTh = ({ column, label, className = "" }) => {
		const active = sort_by === column;
		return (
			// biome-ignore lint/a11y/useSemanticElements: th with button role is a valid pattern for sortable table headers
			<th
				role="button"
				tabIndex={0}
				onClick={() => handle_sort(column)}
				onKeyDown={(e) => {
					if (e.key === "Enter" || e.key === " ") {
						e.preventDefault();
						handle_sort(column);
					}
				}}
				className={`px-4 py-2 text-xs font-medium text-secondary-500 dark:text-secondary-300 select-none cursor-pointer hover:bg-secondary-100 dark:hover:bg-secondary-600 ${className}`}
			>
				<span className="inline-flex items-center gap-1">
					{label}
					{active ? (
						sort_asc ? (
							<ArrowUp className="h-3.5 w-3.5 shrink-0" />
						) : (
							<ArrowDown className="h-3.5 w-3.5 shrink-0" />
						)
					) : null}
				</span>
			</th>
		);
	};

	const get_status_icon = (rule) => {
		if (rule.hosts_failed > 0) {
			return <ShieldX className="h-4 w-4 text-red-500" title="Has failures" />;
		}
		if (rule.hosts_warned > 0) {
			return (
				<ShieldAlert className="h-4 w-4 text-yellow-500" title="Has warnings" />
			);
		}
		if (rule.hosts_passed > 0) {
			return (
				<ShieldCheck className="h-4 w-4 text-green-500" title="All passing" />
			);
		}
		return <Shield className="h-4 w-4 text-secondary-400" title="No data" />;
	};

	return (
		<div className="space-y-4">
			{/* Filters */}
			<div className="card p-4">
				<div className="flex flex-wrap items-center gap-3">
					{/* Search */}
					<div className="relative flex-1 min-w-[200px]">
						<Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-secondary-400" />
						<input
							type="text"
							placeholder="Search rules by title, ID, or section..."
							value={search}
							onChange={(e) => {
								setSearch(e.target.value);
								setPage(0);
							}}
							className="w-full pl-10 pr-4 py-2 bg-secondary-50 dark:bg-secondary-700 border border-secondary-200 dark:border-secondary-600 rounded-lg text-sm text-secondary-900 dark:text-white placeholder-secondary-400 focus:outline-none focus:ring-2 focus:ring-primary-500"
						/>
					</div>

					{/* Severity Filter */}
					<div className="flex items-center gap-2">
						<Filter className="h-4 w-4 text-secondary-400" />
						<select
							value={severity_filter}
							onChange={(e) => {
								set_severity_filter(e.target.value);
								setPage(0);
							}}
							className="px-3 py-2 bg-secondary-50 dark:bg-secondary-700 border border-secondary-200 dark:border-secondary-600 rounded-lg text-sm text-secondary-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-primary-500"
						>
							<option value="">All Severities</option>
							{SEVERITY_ORDER.map((s) => (
								<option key={s} value={s}>
									{s.charAt(0).toUpperCase() + s.slice(1)}
								</option>
							))}
						</select>
					</div>

					{/* Status Filter */}
					<select
						value={status_filter}
						onChange={(e) => {
							set_status_filter(e.target.value);
							setPage(0);
						}}
						className="px-3 py-2 bg-secondary-50 dark:bg-secondary-700 border border-secondary-200 dark:border-secondary-600 rounded-lg text-sm text-secondary-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-primary-500"
					>
						<option value="">All Statuses</option>
						<option value="fail">Failing</option>
						<option value="warn">Warnings</option>
						<option value="pass">Passing</option>
					</select>

					{isFetching && !isLoading && (
						<RefreshCw className="h-4 w-4 text-secondary-400 animate-spin" />
					)}
				</div>
			</div>

			{/* Results Table */}
			{isLoading ? (
				<div className="flex items-center justify-center py-12">
					<RefreshCw className="h-6 w-6 animate-spin text-secondary-400" />
				</div>
			) : raw_rules.length === 0 ? (
				<div className="card p-8 text-center">
					<Shield className="h-8 w-8 text-secondary-400 mx-auto mb-3" />
					<p className="text-secondary-500 dark:text-secondary-400">
						{search || severity_filter || status_filter
							? "No rules match your filters"
							: "No compliance rules found. Run a scan to populate rules."}
					</p>
				</div>
			) : (
				<div className="card overflow-hidden">
					<div className="overflow-x-auto">
						<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-600">
							<thead className="bg-secondary-50 dark:bg-secondary-700">
								<tr>
									<SortableTh
										column="status"
										label="Status"
										className="text-left w-12"
									/>
									<SortableTh
										column="title"
										label="Rule"
										className="text-left"
									/>
									<SortableTh
										column="severity"
										label="Severity"
										className="text-left w-24"
									/>
									<SortableTh
										column="profile_type"
										label="Profile"
										className="text-left w-20"
									/>
									<SortableTh
										column="hosts_passed"
										label="Pass"
										className="text-center w-16 text-green-600 dark:text-green-400"
									/>
									<SortableTh
										column="hosts_failed"
										label="Fail"
										className="text-center w-16 text-red-600 dark:text-red-400"
									/>
									<SortableTh
										column="hosts_warned"
										label="Warn"
										className="text-center w-16 text-yellow-600 dark:text-yellow-400"
									/>
									<SortableTh
										column="total_hosts"
										label="Hosts"
										className="text-center w-16"
									/>
								</tr>
							</thead>
							<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-700 text-sm">
								{sorted_rules.map((rule) => (
									<tr
										key={rule.id}
										className="hover:bg-secondary-50 dark:hover:bg-secondary-700/50 transition-colors"
									>
										<td className="px-4 py-2 whitespace-nowrap">
											{get_status_icon(rule)}
										</td>
										<td className="px-4 py-2">
											<Link
												to={`/compliance/rules/${rule.id}`}
												className="text-secondary-900 dark:text-white hover:text-primary-600 dark:hover:text-primary-400 hover:underline font-medium"
											>
												{rule.title}
											</Link>
										</td>
										<td className="px-4 py-2 whitespace-nowrap">
											<span
												className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${
													SEVERITY_COLORS[rule.severity || "unknown"] ||
													"bg-secondary-100 text-secondary-600 dark:bg-secondary-700 dark:text-secondary-400"
												}`}
											>
												{(rule.severity || "unknown").charAt(0).toUpperCase() +
													(rule.severity || "unknown").slice(1)}
											</span>
										</td>
										<td className="px-4 py-2 whitespace-nowrap">
											<span
												className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${
													rule.profile_type === "docker-bench"
														? "bg-blue-900/30 text-blue-400"
														: "bg-green-900/30 text-green-400"
												}`}
											>
												{rule.profile_type === "docker-bench"
													? "Docker"
													: "OpenSCAP"}
											</span>
										</td>
										<td className="px-4 py-2 text-center whitespace-nowrap">
											{rule.hosts_passed > 0 ? (
												<span className="font-medium text-green-600 dark:text-green-400">
													{rule.hosts_passed}
												</span>
											) : (
												<span className="text-secondary-400">—</span>
											)}
										</td>
										<td className="px-4 py-2 text-center whitespace-nowrap">
											{rule.hosts_failed > 0 ? (
												<span className="font-medium text-red-600 dark:text-red-400">
													{rule.hosts_failed}
												</span>
											) : (
												<span className="text-secondary-400">—</span>
											)}
										</td>
										<td className="px-4 py-2 text-center whitespace-nowrap">
											{rule.hosts_warned > 0 ? (
												<span className="font-medium text-yellow-600 dark:text-yellow-400">
													{rule.hosts_warned}
												</span>
											) : (
												<span className="text-secondary-400">—</span>
											)}
										</td>
										<td className="px-4 py-2 text-center whitespace-nowrap text-secondary-500 dark:text-secondary-400">
											{rule.total_hosts}
										</td>
									</tr>
								))}
							</tbody>
						</table>
					</div>

					{/* Pagination */}
					{total_pages > 1 && (
						<div className="flex items-center justify-between px-4 py-3 border-t border-secondary-200 dark:border-secondary-700">
							<p className="text-sm text-secondary-500 dark:text-secondary-400">
								Showing {page * limit + 1}–{Math.min((page + 1) * limit, total)}{" "}
								of {total} rules
							</p>
							<div className="flex items-center gap-2">
								<button
									type="button"
									onClick={() => setPage((p) => Math.max(0, p - 1))}
									disabled={page === 0}
									className="p-1.5 rounded-lg bg-secondary-100 dark:bg-secondary-700 text-secondary-600 dark:text-secondary-300 hover:bg-secondary-200 dark:hover:bg-secondary-600 disabled:opacity-50 disabled:cursor-not-allowed"
								>
									<ChevronLeft className="h-4 w-4" />
								</button>
								<span className="text-sm text-secondary-500 dark:text-secondary-400">
									Page {page + 1} of {total_pages}
								</span>
								<button
									type="button"
									onClick={() =>
										setPage((p) => Math.min(total_pages - 1, p + 1))
									}
									disabled={page >= total_pages - 1}
									className="p-1.5 rounded-lg bg-secondary-100 dark:bg-secondary-700 text-secondary-600 dark:text-secondary-300 hover:bg-secondary-200 dark:hover:bg-secondary-600 disabled:opacity-50 disabled:cursor-not-allowed"
								>
									<ChevronRight className="h-4 w-4" />
								</button>
							</div>
						</div>
					)}
				</div>
			)}
		</div>
	);
};

export default ScanResultsTab;
