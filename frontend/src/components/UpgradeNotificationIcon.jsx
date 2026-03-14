import { ArrowUpCircle } from "lucide-react";

const UpgradeNotificationIcon = ({ className = "h-4 w-4", show = true }) => {
	if (!show) return null;

	return (
		<ArrowUpCircle
			className={`${className} text-red-500 animate-pulse`}
			title="Update available"
		/>
	);
};

export default UpgradeNotificationIcon;
