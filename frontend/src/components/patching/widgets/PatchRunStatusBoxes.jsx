import { CheckCircle, Clock, ListChecks, XCircle } from "lucide-react";

const PatchRunStatusBoxes = ({ data }) => {
	const summary = data?.summary || {};
	const total = summary.total_runs ?? 0;
	const active = (summary.queued ?? 0) + (summary.running ?? 0);
	const completed = summary.completed ?? 0;
	const failed = summary.failed ?? 0;

	const boxes = [
		{
			label: "Total Runs",
			value: total,
			Icon: ListChecks,
			icon_class: "text-primary-600",
		},
		{
			label: "Active",
			value: active,
			Icon: Clock,
			icon_class: "text-blue-600",
		},
		{
			label: "Completed",
			value: completed,
			Icon: CheckCircle,
			icon_class: "text-green-600",
		},
		{
			label: "Failed",
			value: failed,
			Icon: XCircle,
			icon_class: "text-red-600",
		},
	];

	return (
		<div className="card p-4 sm:p-6 w-full h-full flex flex-col">
			<h3 className="text-lg font-medium text-secondary-900 dark:text-white mb-4 flex-shrink-0">
				Patch Run Status
			</h3>
			<div className="grid grid-cols-2 gap-3 flex-1 min-h-0">
				{boxes.map((box) => {
					const Icon = box.Icon;
					return (
						<div
							key={box.label}
							className="card p-4 cursor-default text-left w-full"
						>
							<div className="flex items-center">
								<Icon
									className={`h-5 w-5 ${box.icon_class} mr-2 flex-shrink-0`}
								/>
								<div>
									<p className="text-sm text-secondary-500 dark:text-white">
										{box.label}
									</p>
									<p className="text-xl font-semibold text-secondary-900 dark:text-white">
										{box.value}
									</p>
								</div>
							</div>
						</div>
					);
				})}
			</div>
		</div>
	);
};

export default PatchRunStatusBoxes;
