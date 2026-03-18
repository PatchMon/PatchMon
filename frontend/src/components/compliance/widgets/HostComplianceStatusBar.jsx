import {
	AlertTriangle,
	ShieldAlert,
	ShieldCheck,
	ShieldOff,
} from "lucide-react";
import { Fragment } from "react";
import { Link } from "react-router-dom";

const HostComplianceStatusBar = ({ data, onTabChange }) => {
	const summary = data?.summary || {};
	const compliant = summary.hosts_compliant ?? 0;
	const warning = summary.hosts_warning ?? 0;
	const critical = summary.hosts_critical ?? 0;
	const unscanned = summary.unscanned ?? 0;

	const boxes = [
		{
			label: "Compliant",
			value: compliant,
			Icon: ShieldCheck,
			icon_class: "text-green-600",
			to: "/compliance",
		},
		{
			label: "Warning",
			value: warning,
			Icon: AlertTriangle,
			icon_class: "text-yellow-600",
			to: "/compliance",
		},
		{
			label: "Critical",
			value: critical,
			Icon: ShieldAlert,
			icon_class: "text-red-600",
			to: "/compliance",
		},
		{
			label: "Never scanned",
			value: unscanned,
			Icon: ShieldOff,
			icon_class: "text-secondary-600 dark:text-white",
			to: "/compliance",
		},
	];

	return (
		<div className="card p-4 sm:p-6 w-full h-full flex flex-col">
			<h3 className="text-lg font-medium text-secondary-900 dark:text-white mb-4 flex-shrink-0">
				Host Compliance Status
			</h3>
			<div className="grid grid-cols-2 gap-3 flex-1 min-h-0">
				{boxes.map((box) => {
					const Icon = box.Icon;
					const content = (
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
					);
					return (
						<Fragment key={box.label}>
							{onTabChange ? (
								<button
									type="button"
									onClick={() => onTabChange("hosts")}
									className="card p-4 text-left w-full hover:bg-secondary-50 dark:hover:bg-secondary-700/50 transition-colors"
								>
									{content}
								</button>
							) : (
								<Link
									to={box.to}
									state={{ complianceTab: "hosts" }}
									className="card p-4 text-left w-full hover:bg-secondary-50 dark:hover:bg-secondary-700/50 transition-colors"
								>
									{content}
								</Link>
							)}
						</Fragment>
					);
				})}
			</div>
		</div>
	);
};

export default HostComplianceStatusBar;
