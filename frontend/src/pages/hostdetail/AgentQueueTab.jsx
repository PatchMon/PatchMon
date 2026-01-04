import { useQuery } from "@tanstack/react-query";
import {
	AlertCircle,
	CheckCircle2,
	Clock,
	Clock3,
	RefreshCw,
	Server,
} from "lucide-react";
import { dashboardAPI } from "../../utils/api";

const AgentQueueTab = ({ hostId }) => {
	const {
		data: queueData,
		isLoading,
		error,
		refetch,
	} = useQuery({
		queryKey: ["host-queue", hostId],
		queryFn: () => dashboardAPI.getHostQueue(hostId).then((res) => res.data),
		staleTime: 30 * 1000, // 30 seconds
		refetchInterval: 30 * 1000, // Auto-refresh every 30 seconds
	});

	if (isLoading) {
		return (
			<div className="flex items-center justify-center h-32">
				<RefreshCw className="h-6 w-6 animate-spin text-primary-600" />
			</div>
		);
	}

	if (error) {
		return (
			<div className="text-center py-8">
				<AlertCircle className="h-12 w-12 text-red-500 mx-auto mb-4" />
				<p className="text-red-600 dark:text-red-400">
					Failed to load queue data
				</p>
				<button
					type="button"
					onClick={() => refetch()}
					className="mt-2 px-4 py-2 text-sm bg-primary-600 text-white rounded-md hover:bg-primary-700"
				>
					Retry
				</button>
			</div>
		);
	}

	const { waiting, active, delayed, failed, jobHistory } = queueData.data;

	const getStatusIcon = (status) => {
		switch (status) {
			case "completed":
				return <CheckCircle2 className="h-4 w-4 text-green-500" />;
			case "failed":
				return <AlertCircle className="h-4 w-4 text-red-500" />;
			case "active":
				return <Clock3 className="h-4 w-4 text-blue-500" />;
			default:
				return <Clock className="h-4 w-4 text-gray-500" />;
		}
	};

	const getStatusColor = (status) => {
		switch (status) {
			case "completed":
				return "text-green-600 dark:text-green-400";
			case "failed":
				return "text-red-600 dark:text-red-400";
			case "active":
				return "text-blue-600 dark:text-blue-400";
			default:
				return "text-gray-600 dark:text-gray-400";
		}
	};

	const formatJobType = (type) => {
		switch (type) {
			case "settings_update":
				return "Settings Update";
			case "report_now":
				return "Report Now";
			case "update_agent":
				return "Agent Update";
			default:
				return type;
		}
	};

	return (
		<div className="space-y-6">
			<div className="flex items-center justify-between">
				<h3 className="text-lg font-medium text-secondary-900 dark:text-white">
					Live Agent Queue Status
				</h3>
				<button
					type="button"
					onClick={() => refetch()}
					className="btn-outline flex items-center gap-2"
					title="Refresh queue data"
				>
					<RefreshCw className="h-4 w-4" />
				</button>
			</div>

			{/* Queue Summary */}
			<div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
				<div className="card p-4">
					<div className="flex items-center">
						<Server className="h-5 w-5 text-blue-600 mr-2" />
						<div>
							<p className="text-sm text-secondary-500 dark:text-white">
								Waiting
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{waiting}
							</p>
						</div>
					</div>
				</div>

				<div className="card p-4">
					<div className="flex items-center">
						<Clock3 className="h-5 w-5 text-warning-600 mr-2" />
						<div>
							<p className="text-sm text-secondary-500 dark:text-white">
								Active
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{active}
							</p>
						</div>
					</div>
				</div>

				<div className="card p-4">
					<div className="flex items-center">
						<Clock className="h-5 w-5 text-primary-600 mr-2" />
						<div>
							<p className="text-sm text-secondary-500 dark:text-white">
								Delayed
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{delayed}
							</p>
						</div>
					</div>
				</div>

				<div className="card p-4">
					<div className="flex items-center">
						<AlertCircle className="h-5 w-5 text-danger-600 mr-2" />
						<div>
							<p className="text-sm text-secondary-500 dark:text-white">
								Failed
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{failed}
							</p>
						</div>
					</div>
				</div>
			</div>

			{/* Job History */}
			<div>
				{jobHistory.length === 0 ? (
					<div className="text-center py-8">
						<Server className="h-12 w-12 text-gray-400 mx-auto mb-4" />
						<p className="text-gray-500 dark:text-gray-400">
							No job history found
						</p>
					</div>
				) : (
					<>
						{/* Mobile Card Layout */}
						<div className="md:hidden space-y-2">
							{jobHistory.map((job) => (
								<div key={job.id} className="card p-3">
									{/* First Line: Job Name, Job ID + Attempt (centered), Status (right) */}
									<div className="flex items-center justify-between gap-2 mb-2 flex-wrap">
										<span className="text-sm font-semibold text-secondary-900 dark:text-white truncate">
											{formatJobType(job.job_name)}
										</span>
										<div className="flex items-center gap-1.5 text-xs flex-1 justify-center min-w-0">
											<div className="flex items-center gap-1 px-1.5 py-0.5 bg-secondary-50 dark:bg-secondary-700/50 rounded border border-secondary-200 dark:border-secondary-600">
												<span className="text-secondary-500 dark:text-secondary-400 whitespace-nowrap">
													Job:
												</span>
												<span className="font-mono text-secondary-600 dark:text-secondary-300 truncate">
													{job.job_id}
												</span>
											</div>
											<div className="flex items-center gap-1 px-1.5 py-0.5 bg-secondary-50 dark:bg-secondary-700/50 rounded border border-secondary-200 dark:border-secondary-600">
												<span className="text-secondary-500 dark:text-secondary-400 whitespace-nowrap">
													Attempt:
												</span>
												<span className="text-secondary-600 dark:text-secondary-300">
													{job.attempt_number}
												</span>
											</div>
										</div>
										<div className="flex items-center gap-1.5 flex-shrink-0">
											{getStatusIcon(job.status)}
											<span
												className={`text-xs font-medium ${getStatusColor(job.status)} whitespace-nowrap`}
											>
												{job.status.charAt(0).toUpperCase() +
													job.status.slice(1)}
											</span>
										</div>
									</div>

									{/* Second Line: Date/Time with Clock Icon */}
									<div className="space-y-0.5">
										<div className="flex items-center gap-1.5 text-xs text-secondary-600 dark:text-secondary-300">
											<Clock className="h-3.5 w-3.5 text-secondary-500 dark:text-secondary-400" />
											{new Date(job.created_at).toLocaleString()}
										</div>
										{(job.error_message || job.output) && (
											<div className="text-xs mt-1 pt-1 border-t border-secondary-200 dark:border-secondary-600">
												{job.error_message ? (
													<span className="text-red-600 dark:text-red-400 break-words">
														{job.error_message}
													</span>
												) : (
													<span className="text-green-600 dark:text-green-400 break-words">
														{JSON.stringify(job.output)}
													</span>
												)}
											</div>
										)}
									</div>
								</div>
							))}
						</div>

						{/* Desktop Table Layout */}
						<div className="hidden md:block overflow-x-auto">
							<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-600">
								<thead className="bg-secondary-50 dark:bg-secondary-700">
									<tr>
										<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
											Job ID
										</th>
										<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
											Job Name
										</th>
										<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
											Status
										</th>
										<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
											Attempt
										</th>
										<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
											Date/Time
										</th>
										<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
											Error/Output
										</th>
									</tr>
								</thead>
								<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-600">
									{jobHistory.map((job) => (
										<tr
											key={job.id}
											className="hover:bg-secondary-50 dark:hover:bg-secondary-700"
										>
											<td className="px-4 py-2 whitespace-nowrap text-xs font-mono text-secondary-900 dark:text-white">
												{job.job_id}
											</td>
											<td className="px-4 py-2 whitespace-nowrap text-xs text-secondary-900 dark:text-white">
												{formatJobType(job.job_name)}
											</td>
											<td className="px-4 py-2 whitespace-nowrap">
												<div className="flex items-center gap-2">
													{getStatusIcon(job.status)}
													<span
														className={`text-xs font-medium ${getStatusColor(job.status)}`}
													>
														{job.status.charAt(0).toUpperCase() +
															job.status.slice(1)}
													</span>
												</div>
											</td>
											<td className="px-4 py-2 whitespace-nowrap text-xs text-secondary-900 dark:text-white">
												{job.attempt_number}
											</td>
											<td className="px-4 py-2 whitespace-nowrap text-xs text-secondary-900 dark:text-white">
												{new Date(job.created_at).toLocaleString()}
											</td>
											<td className="px-4 py-2 text-xs">
												{job.error_message ? (
													<span className="text-red-600 dark:text-red-400">
														{job.error_message}
													</span>
												) : job.output ? (
													<span className="text-green-600 dark:text-green-400">
														{JSON.stringify(job.output)}
													</span>
												) : (
													<span className="text-secondary-500 dark:text-secondary-400">
														-
													</span>
												)}
											</td>
										</tr>
									))}
								</tbody>
							</table>
						</div>
					</>
				)}
			</div>
		</div>
	);
};

export default AgentQueueTab;
