import { formatDistanceToNow } from "date-fns";
import { BellOff, ChevronRight } from "lucide-react";
import { Link } from "react-router-dom";

const DISPLAY_LIMIT = 5;

const SEVERITY_COLORS = {
	critical: "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400",
	error:
		"bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400",
	warning:
		"bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400",
	informational:
		"bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400",
};

const STATUS_INDICATORS = {
	open: { color: "bg-red-500", pulse: true, label: "Open" },
	acknowledged: { color: "bg-blue-500", pulse: false, label: "Acked" },
	investigating: {
		color: "bg-indigo-500",
		pulse: false,
		label: "Investigating",
	},
	escalated: { color: "bg-orange-500", pulse: true, label: "Escalated" },
	silenced: { color: "bg-yellow-500", pulse: false, label: "Silenced" },
	done: { color: "bg-green-500", pulse: false, label: "Done" },
	resolved: { color: "bg-gray-400", pulse: false, label: "Resolved" },
};

const RecentAlerts = ({ alerts }) => {
	const recent = (alerts || [])
		.sort((a, b) => new Date(b.created_at) - new Date(a.created_at))
		.slice(0, DISPLAY_LIMIT);

	const getStatus = (alert) => {
		const action = alert.current_state?.action?.toLowerCase() || "open";
		return STATUS_INDICATORS[action] || STATUS_INDICATORS.open;
	};

	return (
		<div className="card p-4 sm:p-6 w-full h-full flex flex-col">
			<div className="flex items-center justify-between mb-4 flex-shrink-0">
				<h3 className="text-lg font-medium text-secondary-900 dark:text-white">
					Recent Alerts
				</h3>
			</div>

			{recent.length === 0 ? (
				<div className="flex flex-col items-center justify-center py-6 text-secondary-400 dark:text-white flex-1">
					<BellOff className="h-8 w-8 mb-2" />
					<p className="text-sm">No recent alerts</p>
				</div>
			) : (
				<div className="flex flex-col flex-1 min-h-0">
					<div className="overflow-hidden rounded-lg border border-secondary-200 dark:border-secondary-700 flex-1 min-h-0">
						<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-700 text-sm">
							<thead className="bg-secondary-50 dark:bg-secondary-700/50">
								<tr>
									<th className="px-3 py-1.5 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase">
										Severity
									</th>
									<th className="px-3 py-1.5 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase">
										Alert
									</th>
									<th className="px-3 py-1.5 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase">
										Time
									</th>
									<th className="px-3 py-1.5 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase">
										Status
									</th>
								</tr>
							</thead>
							<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-700">
								{recent.map((alert) => {
									const severity_lower =
										alert.severity?.toLowerCase() || "informational";
									const status = getStatus(alert);
									return (
										<tr
											key={alert.id}
											className="hover:bg-secondary-50 dark:hover:bg-secondary-700/50 transition-colors"
										>
											<td className="px-3 py-1.5 whitespace-nowrap">
												<span
													className={`text-xs font-medium px-2 py-0.5 rounded ${
														SEVERITY_COLORS[severity_lower] ||
														SEVERITY_COLORS.informational
													}`}
												>
													{alert.severity}
												</span>
											</td>
											<td className="px-3 py-1.5">
												<p className="font-medium text-secondary-900 dark:text-white truncate max-w-[12rem]">
													{alert.title}
												</p>
											</td>
											<td className="px-3 py-1.5 whitespace-nowrap text-secondary-500 dark:text-white text-xs">
												{formatDistanceToNow(new Date(alert.created_at), {
													addSuffix: true,
												})}
											</td>
											<td className="px-3 py-1.5 whitespace-nowrap">
												<div className="flex items-center gap-1.5">
													<span
														className={`h-2 w-2 rounded-full ${status.color} ${
															status.pulse ? "animate-pulse" : ""
														}`}
													/>
													<span className="text-xs text-secondary-600 dark:text-secondary-300">
														{status.label}
													</span>
												</div>
											</td>
										</tr>
									);
								})}
							</tbody>
						</table>
					</div>
					<Link
						to="/reporting?tab=alerts"
						className="mt-3 flex items-center justify-center gap-1.5 w-full py-2 text-sm font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300 hover:underline flex-shrink-0"
					>
						View all alerts
						<ChevronRight className="h-4 w-4" />
					</Link>
				</div>
			)}
		</div>
	);
};

export default RecentAlerts;
