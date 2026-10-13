import { Info } from "lucide-react";
import Tooltip from "./Tooltip";

const InfoTooltip = ({
	content,
	label = "More information",
	side = "top",
	className = "",
}) => {
	if (!content) return null;
	return (
		<Tooltip content={content} side={side}>
			<button
				type="button"
				aria-label={label}
				className={`relative inline-flex flex-shrink-0 items-center justify-center rounded p-0.5 align-middle text-secondary-400 hover:text-secondary-600 dark:hover:text-secondary-200 cursor-help focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 before:absolute before:-inset-3 md:before:-inset-1 ${className}`}
			>
				<Info className="h-4 w-4" aria-hidden="true" />
			</button>
		</Tooltip>
	);
};

export default InfoTooltip;
export { InfoTooltip };
