import { AlertCircle, Clock, Clock3, Server } from "lucide-react";

// Verbatim port of the four stat tiles previously rendered at the top of
// AgentQueueTab. Counts describe in-flight server -> agent work only; report
// rows on the activity timeline are unrelated to these.
const QueueStatCards = ({ stats }) => {
	const waiting = stats?.waiting ?? 0;
	const active = stats?.active ?? 0;
	const delayed = stats?.delayed ?? 0;
	const failed = stats?.failed ?? 0;

	return (
		<div className="grid grid-cols-2 sm:grid-cols-4 gap-3 sm:gap-4 mb-6">
			<div className="card p-3 sm:p-4">
				<div className="flex items-center">
					<Server className="h-5 w-5 text-blue-600 mr-2 flex-shrink-0" />
					<div className="w-0 flex-1">
						<p className="text-sm text-secondary-500 dark:text-white">
							Waiting
						</p>
						<p className="text-xl font-semibold text-secondary-900 dark:text-white">
							{waiting}
						</p>
					</div>
				</div>
			</div>

			<div className="card p-3 sm:p-4">
				<div className="flex items-center">
					<Clock3 className="h-5 w-5 text-warning-600 mr-2 flex-shrink-0" />
					<div className="w-0 flex-1">
						<p className="text-sm text-secondary-500 dark:text-white">Active</p>
						<p className="text-xl font-semibold text-secondary-900 dark:text-white">
							{active}
						</p>
					</div>
				</div>
			</div>

			<div className="card p-3 sm:p-4">
				<div className="flex items-center">
					<Clock className="h-5 w-5 text-primary-600 mr-2 flex-shrink-0" />
					<div className="w-0 flex-1">
						<p className="text-sm text-secondary-500 dark:text-white">
							Delayed
						</p>
						<p className="text-xl font-semibold text-secondary-900 dark:text-white">
							{delayed}
						</p>
					</div>
				</div>
			</div>

			<div className="card p-3 sm:p-4">
				<div className="flex items-center">
					<AlertCircle className="h-5 w-5 text-danger-600 mr-2 flex-shrink-0" />
					<div className="w-0 flex-1">
						<p className="text-sm text-secondary-500 dark:text-white">Failed</p>
						<p className="text-xl font-semibold text-secondary-900 dark:text-white">
							{failed}
						</p>
					</div>
				</div>
			</div>
		</div>
	);
};

export default QueueStatCards;
