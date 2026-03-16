import { useEffect, useState } from "react";

/**
 * Formats remaining time until a target date as human-readable text.
 * @param {Date} target - Target date
 * @returns {string} e.g. "in 5 min", "in 2 h", "in 30 s", "soon"
 */
function formatTimeUntil(target) {
	const now = Date.now();
	const ms = target - now;
	if (ms <= 0) return "soon";

	const sec = Math.floor(ms / 1000);
	const min = Math.floor(sec / 60);
	const h = Math.floor(min / 60);

	if (sec < 60) return `in ${sec} s`;
	if (min < 60) return `in ${min} min`;
	if (h < 24) return `in ${h} h`;
	return `in ${Math.floor(h / 24)} d`;
}

/**
 * Badge for queued patch runs. Shows "Queued" and a countdown timer when scheduled_at is set.
 */
export function QueuedStatusBadge({ scheduledAt, className = "" }) {
	const [countdown, setCountdown] = useState(null);

	useEffect(() => {
		if (!scheduledAt) {
			setCountdown(null);
			return;
		}
		const target = new Date(scheduledAt).getTime();
		const tick = () => {
			const now = Date.now();
			if (target <= now) {
				setCountdown("soon");
				return;
			}
			setCountdown(formatTimeUntil(new Date(target)));
		};
		tick();
		const id = setInterval(tick, 1000);
		return () => clearInterval(id);
	}, [scheduledAt]);

	const baseClass =
		"inline-flex items-center gap-1.5 px-2 py-0.5 rounded text-xs font-medium bg-secondary-100 text-secondary-800 dark:bg-secondary-700 dark:text-secondary-200";

	return (
		<span
			className={`${baseClass} ${className}`}
			title={
				scheduledAt
					? `Runs at ${new Date(scheduledAt).toLocaleString()}`
					: "Queued"
			}
		>
			Queued
			{scheduledAt && countdown && (
				<span className="opacity-90 font-normal">({countdown})</span>
			)}
		</span>
	);
}
