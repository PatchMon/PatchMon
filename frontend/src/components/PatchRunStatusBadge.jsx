import { QueuedStatusBadge } from "./QueuedStatusBadge";

const STATUS_MAP = {
	queued: {
		class:
			"bg-secondary-100 text-secondary-800 dark:bg-secondary-700 dark:text-secondary-200",
		label: "Queued",
	},
	pending_validation: {
		class: "bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200",
		label: "Pending validation",
	},
	validated: {
		class:
			"bg-emerald-100 text-emerald-800 dark:bg-emerald-900 dark:text-emerald-200",
		label: "Validated",
	},
	scheduled: {
		class:
			"bg-secondary-100 text-secondary-800 dark:bg-secondary-700 dark:text-secondary-200",
		label: "Scheduled",
	},
	running: {
		class: "bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200",
		label: "Running",
	},
	completed: {
		class: "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200",
		label: "Completed",
	},
	failed: {
		class: "bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200",
		label: "Failed",
	},
	cancelled: {
		class:
			"bg-secondary-100 text-secondary-600 dark:bg-secondary-600 dark:text-secondary-200",
		label: "Cancelled",
	},
};

/**
 * Status badge for patch runs. For queued runs with scheduled_at, shows a countdown timer.
 */
export function PatchRunStatusBadge({ run }) {
	const status = run?.status ?? "";
	const scheduledAt = run?.scheduled_at ?? null;

	if (status === "queued" && scheduledAt) {
		return <QueuedStatusBadge scheduledAt={scheduledAt} />;
	}

	const s = STATUS_MAP[status] || {
		class: "bg-secondary-100 text-secondary-800",
		label: status,
	};
	return (
		<span
			className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${s.class}`}
		>
			{s.label}
		</span>
	);
}
