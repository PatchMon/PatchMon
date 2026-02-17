import {
	AlertTriangle,
	ShieldAlert,
	ShieldCheck,
	ShieldOff,
} from "lucide-react";

const HostComplianceStatusBar = ({ data }) => {
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
		},
		{
			label: "Warning",
			value: warning,
			Icon: AlertTriangle,
			icon_class: "text-yellow-600",
		},
		{
			label: "Critical",
			value: critical,
			Icon: ShieldAlert,
			icon_class: "text-red-600",
		},
		{
			label: "Never scanned",
			value: unscanned,
			Icon: ShieldOff,
			icon_class: "text-secondary-600",
		},
	];

	return (
		<div className="card p-4 sm:p-6 w-full">
			<h3 className="text-lg font-medium text-secondary-900 dark:text-white mb-4">
				Host Compliance Status
			</h3>
			<div className="grid grid-cols-2 gap-3">
				{boxes.map((box) => {
					const Icon = box.Icon;
					return (
						<div
							key={box.label}
							className="card p-4 cursor-default text-left w-full"
						>
							<div className="flex items-center">
								<Icon className={`h-5 w-5 ${box.icon_class} mr-2`} />
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

export default HostComplianceStatusBar;
