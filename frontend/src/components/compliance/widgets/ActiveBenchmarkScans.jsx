import { useQuery } from "@tanstack/react-query";
import { formatDistanceToNow } from "date-fns";
import { RefreshCw, Shield, ShieldCheck } from "lucide-react";
import { Link } from "react-router-dom";
import { complianceAPI } from "../../../utils/complianceApi";

const ActiveBenchmarkScans = () => {
	const { data, isLoading } = useQuery({
		queryKey: ["compliance-active-scans"],
		queryFn: () => complianceAPI.getActiveScans().then((res) => res.data),
		staleTime: 15 * 1000,
		refetchInterval: 20 * 1000,
	});

	const scans = data?.activeScans || [];

	return (
		<div className="card p-4 sm:p-6 w-full h-full flex flex-col">
			<div className="flex items-center justify-between mb-4 flex-shrink-0">
				<h3 className="text-lg font-medium text-secondary-900 dark:text-white">
					Active Benchmark Scans
				</h3>
				{scans.length > 0 && (
					<span className="inline-flex items-center gap-1.5 text-xs font-medium px-2.5 py-1 rounded-full bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300">
						<RefreshCw className="h-3 w-3 animate-spin" />
						{scans.length} running
					</span>
				)}
			</div>

			{isLoading ? (
				<div className="flex items-center justify-center h-24 flex-1 min-h-0">
					<RefreshCw className="h-5 w-5 animate-spin text-secondary-400" />
				</div>
			) : scans.length === 0 ? (
				<div className="flex flex-col items-center justify-center py-6 text-secondary-400 dark:text-secondary-500 flex-1">
					<ShieldCheck className="h-8 w-8 mb-2" />
					<p className="text-sm">No scans currently running</p>
				</div>
			) : (
				<div className="overflow-hidden rounded-lg border border-secondary-200 dark:border-secondary-700 flex-1 min-h-0 flex flex-col">
					<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-700 text-sm">
						<thead className="bg-secondary-50 dark:bg-secondary-700/50">
							<tr>
								<th className="px-3 py-1.5 text-left text-xs font-medium text-secondary-500 dark:text-secondary-400 uppercase">
									Host
								</th>
								<th className="px-3 py-1.5 text-left text-xs font-medium text-secondary-500 dark:text-secondary-400 uppercase">
									Profile
								</th>
								<th className="px-3 py-1.5 text-left text-xs font-medium text-secondary-500 dark:text-secondary-400 uppercase">
									Started
								</th>
							</tr>
						</thead>
						<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-700">
							{scans.map((scan) => (
								<tr
									key={scan.id}
									className="hover:bg-secondary-50 dark:hover:bg-secondary-700/50 transition-colors"
								>
									<td className="px-3 py-1.5 whitespace-nowrap">
										<Link
											to={`/compliance/hosts/${scan.hostId}`}
											className="font-medium text-primary-600 hover:text-primary-900 dark:text-primary-400 dark:hover:text-primary-300 hover:underline"
										>
											{scan.hostName || "Unknown"}
										</Link>
									</td>
									<td className="px-3 py-1.5 whitespace-nowrap">
										<span className="inline-flex items-center gap-1.5">
											<RefreshCw className="h-3 w-3 animate-spin text-blue-600 dark:text-blue-400" />
											<span
												className={`inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs font-medium ${
													scan.profileType === "docker-bench"
														? "bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300"
														: "bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300"
												}`}
											>
												<Shield className="h-3 w-3" />
												{scan.profileType === "docker-bench"
													? "Docker Bench"
													: scan.profileType === "openscap"
														? "OpenSCAP"
														: scan.profileName || "Scanning"}
											</span>
										</span>
									</td>
									<td className="px-3 py-1.5 whitespace-nowrap text-secondary-500 dark:text-secondary-400 text-xs">
										{scan.startedAt
											? formatDistanceToNow(new Date(scan.startedAt), {
													addSuffix: true,
												})
											: "—"}
									</td>
								</tr>
							))}
						</tbody>
					</table>
				</div>
			)}
		</div>
	);
};

export default ActiveBenchmarkScans;
