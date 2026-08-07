import { formatDistanceToNow } from "date-fns";
import { ChevronRight, History } from "lucide-react";
import { Link } from "react-router-dom";
import { PatchRunStatusBadge } from "../../PatchRunStatusBadge";

const DISPLAY_LIMIT = 5;

const PatchingRecentRuns = ({ data }) => {
	const recent_runs = (data?.recent_runs || []).slice(0, DISPLAY_LIMIT);

	return (
		<div className="card p-4 sm:p-6 w-full h-full flex flex-col">
			<div className="flex items-center justify-between mb-4 flex-shrink-0">
				<h3 className="text-lg font-medium text-secondary-900 dark:text-white">
					Recent Runs
				</h3>
			</div>

			{recent_runs.length === 0 ? (
				<div className="flex flex-col items-center justify-center py-6 text-secondary-400 dark:text-white flex-1">
					<History className="h-8 w-8 mb-2" />
					<p className="text-sm">No patch runs yet</p>
				</div>
			) : (
				<div className="flex flex-col flex-1 min-h-0">
					<div className="overflow-hidden rounded-lg border border-secondary-200 dark:border-secondary-700 flex-1 min-h-0">
						<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-700 text-sm">
							<thead className="bg-secondary-50 dark:bg-secondary-700/50">
								<tr>
									<th className="px-3 py-1.5 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase">
										Host
									</th>
									<th className="px-3 py-1.5 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase">
										Status
									</th>
									<th className="px-3 py-1.5 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase">
										When
									</th>
								</tr>
							</thead>
							<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-700">
								{recent_runs.map((run) => (
									<tr
										key={run.id}
										className="hover:bg-secondary-50 dark:hover:bg-secondary-700/50 transition-colors"
									>
										<td className="px-3 py-1.5 whitespace-nowrap">
											<Link
												to={`/patching/runs/${run.id}`}
												className="font-medium text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300 hover:underline truncate max-w-[8rem] block"
											>
												{run.hosts?.friendly_name ||
													run.hosts?.hostname ||
													run.host_id}
											</Link>
										</td>
										<td className="px-3 py-1.5 whitespace-nowrap">
											<PatchRunStatusBadge run={run} />
										</td>
										<td className="px-3 py-1.5 whitespace-nowrap text-secondary-500 dark:text-white text-xs">
											{formatDistanceToNow(new Date(run.created_at), {
												addSuffix: true,
											})}
										</td>
									</tr>
								))}
							</tbody>
						</table>
					</div>
					<Link
						to="/patching?tab=runs"
						className="mt-3 flex items-center justify-center gap-1.5 w-full py-2 text-sm font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300 hover:underline flex-shrink-0"
					>
						View all runs
						<ChevronRight className="h-4 w-4" />
					</Link>
				</div>
			)}
		</div>
	);
};

export default PatchingRecentRuns;
