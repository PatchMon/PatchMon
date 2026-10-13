import { formatRelativeTime } from "../../utils/api";
import { getUpdateStatusMeta } from "../../utils/docker";
import Tooltip from "../ui/Tooltip";

export const RecreateNeededBadge = ({ count }) =>
	count > 0 ? (
		<span
			className="badge-secondary w-fit whitespace-nowrap"
			title="Containers running an older image than the one now pulled for their tag"
		>
			Recreate needed ({count})
		</span>
	) : null;

const UpdateStatusBadge = ({ status, checkedAt, reason }) => {
	const meta = getUpdateStatusMeta(status, {
		checkedAt,
		reason,
		formatTime: formatRelativeTime,
	});

	if (!meta.tooltip) {
		return (
			<span className={`${meta.className} w-fit whitespace-nowrap`}>
				{meta.label}
			</span>
		);
	}

	return (
		<Tooltip content={meta.tooltip}>
			<button
				type="button"
				className={`${meta.className} w-fit whitespace-nowrap cursor-help focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-1`}
				aria-label={`${meta.label}. ${meta.tooltip}`}
				onClick={(e) => {
					e.preventDefault();
					e.stopPropagation();
				}}
			>
				{meta.label}
			</button>
		</Tooltip>
	);
};

export default UpdateStatusBadge;
