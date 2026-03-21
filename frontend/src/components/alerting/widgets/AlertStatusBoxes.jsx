import {
	AlertOctagon,
	AlertTriangle,
	CheckCircle2,
	ShieldAlert,
} from "lucide-react";
import { Link } from "react-router-dom";

const AlertStatusBoxes = ({ stats, alerts }) => {
	const active =
		(stats?.critical ?? 0) +
		(stats?.error ?? 0) +
		(stats?.warning ?? 0) +
		(stats?.informational ?? 0);
	const critical = stats?.critical ?? 0;
	const warnings = stats?.warning ?? 0;

	// Count alerts resolved today
	const today = new Date();
	today.setHours(0, 0, 0, 0);
	const resolved_today = (alerts || []).filter((a) => {
		if (!a.resolved_at) return false;
		const resolved = new Date(a.resolved_at);
		return resolved >= today;
	}).length;

	const boxes = [
		{
			label: "Active Alerts",
			value: active,
			Icon: AlertTriangle,
			icon_class: "text-red-500",
			to: "/reporting?tab=alerts",
		},
		{
			label: "Critical",
			value: critical,
			Icon: AlertOctagon,
			icon_class: "text-red-600",
			to: "/reporting?tab=alerts&severity=critical",
		},
		{
			label: "Warnings",
			value: warnings,
			Icon: ShieldAlert,
			icon_class: "text-amber-500",
			to: "/reporting?tab=alerts&severity=warning",
		},
		{
			label: "Resolved Today",
			value: resolved_today,
			Icon: CheckCircle2,
			icon_class: "text-green-500",
			to: "/reporting?tab=alerts&status=resolved",
		},
	];

	return (
		<div className="card p-4 sm:p-6 w-full h-full flex flex-col">
			<h3 className="text-lg font-medium text-secondary-900 dark:text-white mb-4 flex-shrink-0">
				Alert Summary
			</h3>
			<div className="grid grid-cols-2 gap-3 flex-1 min-h-0">
				{boxes.map((box) => {
					const Icon = box.Icon;
					return (
						<Link
							key={box.label}
							to={box.to}
							className="card p-4 text-left w-full hover:shadow-card-hover dark:hover:shadow-card-hover-dark transition-shadow duration-200"
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
		</div>
	);
};

export default AlertStatusBoxes;
