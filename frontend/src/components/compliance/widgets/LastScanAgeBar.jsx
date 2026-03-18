import { ChevronRight } from "lucide-react";
import { Doughnut } from "react-chartjs-2";
import { Link } from "react-router-dom";
import { useTheme } from "../../../contexts/ThemeContext";
import { getDoughnutOptions } from "./chartOptions";

const LABELS = ["Today", "This week", "This month", "Older"];
const COLORS = ["#10B981", "#3B82F6", "#F59E0B", "#6B7280"];

const LastScanAgeBar = ({ data, onTabChange }) => {
	const { isDark } = useTheme();

	const scan_age = data?.scan_age_distribution || {};
	const values = [
		(scan_age.today?.openscap ?? 0) + (scan_age.today?.["docker-bench"] ?? 0),
		(scan_age.this_week?.openscap ?? 0) +
			(scan_age.this_week?.["docker-bench"] ?? 0),
		(scan_age.this_month?.openscap ?? 0) +
			(scan_age.this_month?.["docker-bench"] ?? 0),
		(scan_age.older?.openscap ?? 0) + (scan_age.older?.["docker-bench"] ?? 0),
	];
	const has_data = values.some((v) => v > 0);

	const chart_data = {
		labels: LABELS,
		datasets: [
			{
				data: has_data ? values : [1],
				backgroundColor: has_data ? COLORS : ["#374151"],
				borderWidth: 0,
			},
		],
	};

	const options = getDoughnutOptions(isDark, false);

	return (
		<div className="card p-4 sm:p-6 w-full h-full flex flex-col">
			<h3 className="text-lg font-medium text-secondary-900 dark:text-white mb-4 flex-shrink-0">
				Last Scan Age
			</h3>
			<div className="h-56 w-full flex items-center justify-center flex-1 min-h-0">
				<div className="w-full h-full max-w-sm">
					{has_data ? (
						<Doughnut data={chart_data} options={options} />
					) : (
						<div className="flex items-center justify-center h-full text-secondary-500 dark:text-white text-sm">
							No scan data yet
						</div>
					)}
				</div>
			</div>
			{onTabChange ? (
				<button
					type="button"
					onClick={() => onTabChange("hosts")}
					className="mt-3 flex items-center justify-center gap-1.5 w-full py-2 text-sm font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300 hover:underline flex-shrink-0"
				>
					View compliance hosts
					<ChevronRight className="h-4 w-4" />
				</button>
			) : (
				<Link
					to="/compliance"
					state={{ complianceTab: "hosts" }}
					className="mt-3 flex items-center justify-center gap-1.5 w-full py-2 text-sm font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300 hover:underline flex-shrink-0"
				>
					View compliance hosts
					<ChevronRight className="h-4 w-4" />
				</Link>
			)}
		</div>
	);
};

export default LastScanAgeBar;
