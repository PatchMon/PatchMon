import { ChevronRight, User, Users } from "lucide-react";
import { useMemo } from "react";
import { Link, useNavigate } from "react-router-dom";

const AlertResponderWorkload = ({ alerts, users }) => {
	const navigate = useNavigate();
	const { responders, unassigned_count, avg_load } = useMemo(() => {
		const user_map = {};
		let _unassigned = 0;

		for (const alert of alerts || []) {
			const action = alert.current_state?.action?.toLowerCase() || "";
			const is_terminal = ["done", "resolved"].includes(action);
			if (is_terminal) continue;

			if (alert.assigned_to_user_id) {
				if (!user_map[alert.assigned_to_user_id]) {
					// Find user details
					const user = (users || []).find(
						(u) => u.id === alert.assigned_to_user_id,
					);
					user_map[alert.assigned_to_user_id] = {
						id: alert.assigned_to_user_id,
						name: user
							? `${user.first_name || ""} ${user.last_name || ""}`.trim() ||
								user.username ||
								user.email
							: alert.assigned_to_user_id.slice(0, 8),
						count: 0,
					};
				}
				user_map[alert.assigned_to_user_id].count++;
			} else {
				_unassigned++;
			}
		}

		const sorted = Object.values(user_map).sort((a, b) => b.count - a.count);
		const total_assigned = sorted.reduce((sum, r) => sum + r.count, 0);
		const _avg =
			sorted.length > 0 ? Math.round(total_assigned / sorted.length) : 0;

		return {
			responders: sorted.slice(0, 5),
			unassigned_count: _unassigned,
			avg_load: _avg,
		};
	}, [alerts, users]);

	const max_count = responders.length > 0 ? responders[0].count : 1;

	return (
		<div className="card p-4 sm:p-6 w-full h-full flex flex-col">
			<div className="flex items-center justify-between mb-4 flex-shrink-0">
				<h3 className="text-lg font-medium text-secondary-900 dark:text-white">
					Responder Workload
				</h3>
				{avg_load > 0 && (
					<span className="text-xs text-secondary-500 dark:text-secondary-400">
						Avg: {avg_load}
					</span>
				)}
			</div>

			<div className="flex-1 min-h-0 flex flex-col">
				{responders.length === 0 && unassigned_count === 0 ? (
					<div className="flex flex-col items-center justify-center py-6 text-secondary-400 dark:text-white flex-1">
						<Users className="h-8 w-8 mb-2" />
						<p className="text-sm">No active assignments</p>
					</div>
				) : (
					<div className="flex flex-col gap-2 flex-1">
						{responders.map((responder) => {
							const is_overloaded =
								avg_load > 0 && responder.count > avg_load * 1.5;
							const bar_pct = Math.round((responder.count / max_count) * 100);
							return (
								<button
									type="button"
									key={responder.id}
									className="flex flex-col gap-0.5 cursor-pointer rounded px-1 -mx-1 hover:bg-secondary-50 dark:hover:bg-secondary-700/50 transition-colors text-left w-full"
									onClick={() =>
										navigate(`/reporting?tab=alerts&assignment=${responder.id}`)
									}
								>
									<div className="flex items-center justify-between">
										<div className="flex items-center gap-1.5">
											<User className="h-3.5 w-3.5 text-secondary-400" />
											<span className="text-sm text-secondary-700 dark:text-secondary-200 truncate max-w-[10rem]">
												{responder.name}
											</span>
										</div>
										<span
											className={`text-sm font-semibold ${
												is_overloaded
													? "text-red-500"
													: "text-secondary-900 dark:text-white"
											}`}
										>
											{responder.count}
											{is_overloaded && (
												<span className="ml-1 text-xs text-red-400">!</span>
											)}
										</span>
									</div>
									<div className="h-1.5 bg-secondary-100 dark:bg-secondary-700 rounded-full overflow-hidden">
										<div
											className={`h-full rounded-full transition-all ${
												is_overloaded ? "bg-red-400" : "bg-primary-500"
											}`}
											style={{ width: `${bar_pct}%` }}
										/>
									</div>
								</button>
							);
						})}

						{/* Unassigned row */}
						{unassigned_count > 0 && (
							<button
								type="button"
								className="mt-1 pt-2 border-t border-secondary-200 dark:border-secondary-700 cursor-pointer rounded px-1 -mx-1 hover:bg-secondary-50 dark:hover:bg-secondary-700/50 transition-colors text-left w-full"
								onClick={() =>
									navigate("/reporting?tab=alerts&assignment=unassigned")
								}
							>
								<div className="flex items-center justify-between">
									<span className="text-sm text-amber-600 dark:text-amber-400 font-medium">
										Unassigned
									</span>
									<span className="text-sm font-semibold text-amber-600 dark:text-amber-400">
										{unassigned_count}
									</span>
								</div>
							</button>
						)}
					</div>
				)}
			</div>

			<Link
				to="/reporting?tab=alerts&assignment=unassigned"
				className="mt-3 flex items-center justify-center gap-1.5 w-full py-2 text-sm font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300 hover:underline flex-shrink-0"
			>
				View assignments
				<ChevronRight className="h-4 w-4" />
			</Link>
		</div>
	);
};

export default AlertResponderWorkload;
