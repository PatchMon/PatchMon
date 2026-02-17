import { Doughnut } from "react-chartjs-2";
import { useTheme } from "../../../contexts/ThemeContext";
import { getDoughnutOptions } from "./chartOptions";

const LABELS = ["Today", "This week", "This month", "Older"];
const COLORS = ["#10B981", "#3B82F6", "#F59E0B", "#6B7280"];

const LastScanAgeBar = ({ data }) => {
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
				borderWidth: 2,
				borderColor: "#ffffff",
			},
		],
	};

	const options = getDoughnutOptions(isDark, false);

	return (
		<div className="card p-4 sm:p-6 w-full">
			<h3 className="text-lg font-medium text-secondary-900 dark:text-white mb-4">
				Last Scan Age
			</h3>
			<div className="h-64 w-full flex items-center justify-center">
				<div className="w-full h-full max-w-sm">
					{has_data ? (
						<Doughnut data={chart_data} options={options} />
					) : (
						<div className="flex items-center justify-center h-full text-secondary-500 dark:text-secondary-400 text-sm">
							No scan data yet
						</div>
					)}
				</div>
			</div>
		</div>
	);
};

export default LastScanAgeBar;
