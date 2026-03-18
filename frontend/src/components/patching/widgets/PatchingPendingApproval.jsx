import { AlertTriangle, CheckSquare, Clock, Play } from "lucide-react";
import { Link } from "react-router-dom";

const PatchingPendingApproval = ({ data }) => {
	const summary = data?.summary || {};
	const pending_validation = summary.pending_validation ?? 0;
	const validated = summary.validated ?? 0;
	const queued = summary.queued ?? 0;
	const running = summary.running ?? 0;

	const needs_action = pending_validation + validated;

	const boxes = [
		{
			label: "Pending Validation",
			value: pending_validation,
			Icon: Clock,
			icon_class: "text-amber-500",
			highlight: pending_validation > 0,
			to: "/patching?tab=runs&status=pending_validation",
		},
		{
			label: "Awaiting Approval",
			value: validated,
			Icon: AlertTriangle,
			icon_class: "text-amber-600",
			highlight: validated > 0,
			to: "/patching?tab=runs&status=validated",
		},
		{
			label: "Queued",
			value: queued,
			Icon: CheckSquare,
			icon_class: "text-blue-500",
			highlight: false,
			to: "/patching?tab=runs&status=queued",
		},
		{
			label: "Running",
			value: running,
			Icon: Play,
			icon_class: "text-green-500",
			highlight: false,
			to: "/patching?tab=runs&status=running",
		},
	];

	return (
		<div className="card p-4 sm:p-6 w-full h-full flex flex-col">
			<div className="flex items-center justify-between mb-4 flex-shrink-0">
				<h3 className="text-lg font-medium text-secondary-900 dark:text-white">
					Pending Approval
				</h3>
				{needs_action > 0 && (
					<span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300">
						<AlertTriangle className="h-3 w-3" />
						{needs_action} need action
					</span>
				)}
			</div>
			<div className="grid grid-cols-2 gap-3 flex-1 min-h-0">
				{boxes.map((box) => {
					const Icon = box.Icon;
					return (
						<Link
							key={box.label}
							to={box.to}
							className={`card p-4 text-left w-full hover:shadow-card-hover dark:hover:shadow-card-hover-dark transition-shadow duration-200 ${
								box.highlight
									? "border border-amber-400 dark:border-amber-600 bg-amber-50 dark:bg-amber-900/10"
									: ""
							}`}
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
						</Link>
					);
				})}
			</div>
			{needs_action > 0 && (
				<Link
					to="/patching?tab=runs&status=validated"
					className="mt-3 flex items-center justify-center gap-1.5 w-full py-2 text-sm font-medium text-amber-600 hover:text-amber-700 dark:text-amber-400 dark:hover:text-amber-300 hover:underline flex-shrink-0"
				>
					Review {needs_action} run{needs_action !== 1 ? "s" : ""} needing
					action
				</Link>
			)}
		</div>
	);
};

export default PatchingPendingApproval;
