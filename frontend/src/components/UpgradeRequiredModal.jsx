import { X } from "lucide-react";
import { useEffect } from "react";
import UpgradeRequiredContent from "./UpgradeRequiredContent";

// Modal variant of the upgrade screen. Used for in-page locked actions where
// we don't want to navigate away (e.g. host-detail tabs).
const UpgradeRequiredModal = ({ module: moduleKey, open, onClose }) => {
	useEffect(() => {
		if (!open) return;
		const onKey = (e) => {
			if (e.key === "Escape") onClose?.();
		};
		window.addEventListener("keydown", onKey);
		return () => window.removeEventListener("keydown", onKey);
	}, [open, onClose]);

	if (!open) return null;

	return (
		<div
			className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50"
			role="dialog"
			aria-modal="true"
			onClick={onClose}
			onKeyDown={(e) => {
				if (e.key === "Enter" || e.key === " ") onClose?.();
			}}
		>
			<div
				className="relative w-full max-w-2xl max-h-[90vh] overflow-y-auto"
				onClick={(e) => e.stopPropagation()}
				onKeyDown={(e) => e.stopPropagation()}
			>
				<button
					type="button"
					onClick={onClose}
					aria-label="Close"
					className="absolute top-3 right-3 z-10 p-2 rounded-md bg-white/80 dark:bg-secondary-800/80 text-secondary-600 dark:text-secondary-300 hover:bg-white dark:hover:bg-secondary-800 min-h-[44px] min-w-[44px] flex items-center justify-center"
				>
					<X className="h-5 w-5" />
				</button>
				<UpgradeRequiredContent module={moduleKey} variant="modal" />
			</div>
		</div>
	);
};

export default UpgradeRequiredModal;
