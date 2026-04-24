import { AlertTriangle, ChevronRight, Clock, UserX } from "lucide-react";
import { useMemo } from "react";
import { Link } from "react-router-dom";

const AlertsRequiringAttention = ({ alerts }) => {
	const { unacked, unassigned, escalated, stale_alerts } = useMemo(() => {
		const now = new Date();
		const stale_threshold = 24 * 60 * 60 * 1000; // 24 hours

		let _unacked = 0;
		let _unassigned = 0;
		let _escalated = 0;
		const _stale = [];

		for (const alert of alerts || []) {
			const action = alert.current_state?.action?.toLowerCase() || "";
			const is_terminal = ["done", "resolved"].includes(action);
			if (is_terminal) continue;

			// No action taken yet = unacknowledged
			if (!action || action === "created" || action === "updated") {
				_unacked++;
			}

			if (!alert.assigned_to_user_id) {
				_unassigned++;
			}

			if (action === "escalated") {
				_escalated++;
			}

			// Stale: no state change in 24h and still active
			const last_update = new Date(alert.updated_at || alert.created_at);
			if (now - last_update > stale_threshold) {
				_stale.push(alert);
			}
		}

		return {
			unacked: _unacked,
			unassigned: _unassigned,
			escalated: _escalated,
			stale_alerts: _stale
				.sort(
					(a, b) =>
						new Date(a.updated_at || a.created_at) -
						new Date(b.updated_at || b.created_at),
				)
				.slice(0, 3),
		};
	}, [alerts]);

	const items = [
		{
			label: "Unacknowledged",
			value: unacked,
			Icon: AlertTriangle,
			color: "text-red-500",
			bg: "bg-red-50 dark:bg-red-900/20",
		},
		{
			label: "Unassigned",
			value: unassigned,
			Icon: UserX,
			color: "text-amber-500",
			bg: "bg-amber-50 dark:bg-amber-900/20",
		},
		{
			label: "Escalated",
			value: escalated,
			Icon: AlertTriangle,
			color: "text-orange-500",
			bg: "bg-orange-50 dark:bg-orange-900/20",
		},
	];

	return (
		<div className="card p-4 sm:p-6 w-full h-full flex flex-col">
			<h3 className="text-lg font-medium text-secondary-900 dark:text-white mb-4 flex-shrink-0">
				Requiring Attention
			</h3>

			<div className="flex-1 min-h-0 flex flex-col gap-3">
				{/* Metric rows */}
				<div className="flex flex-col gap-2">
					{items.map((item) => {
						const Icon = item.Icon;
						return (
							<div
								key={item.label}
								className={`flex items-center justify-between px-3 py-2.5 rounded-lg ${item.bg}`}
							>
								<div className="flex items-center gap-2">
									<Icon className={`h-4 w-4 ${item.color}`} />
									<span className="text-sm text-secondary-700 dark:text-secondary-200">
										{item.label}
									</span>
								</div>
								<span
									className={`text-lg font-bold ${item.value > 0 ? item.color : "text-secondary-400 dark:text-secondary-300"}`}
								>
									{item.value}
								</span>
							</div>
						);
					})}
				</div>

				{/* Stale alerts */}
				{stale_alerts.length > 0 && (
					<div className="mt-1">
						<p className="text-xs font-medium text-secondary-500 dark:text-secondary-400 uppercase mb-1.5 flex items-center gap-1">
							<Clock className="h-3 w-3" />
							Stale ({">"}24h no update)
						</p>
						<div className="flex flex-col gap-1">
							{stale_alerts.map((alert) => (
								<div
									key={alert.id}
									className="text-xs text-secondary-600 dark:text-secondary-300 truncate px-2 py-1 bg-secondary-50 dark:bg-secondary-700/50 rounded"
								>
									{alert.title}
								</div>
							))}
						</div>
					</div>
				)}
			</div>

			<Link
				to="/reporting?tab=alerts&status=open"
				className="mt-3 flex items-center justify-center gap-1.5 w-full py-2 text-sm font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300 hover:underline flex-shrink-0"
			>
				View open alerts
				<ChevronRight className="h-4 w-4" />
			</Link>
		</div>
	);
};

export default AlertsRequiringAttention;
