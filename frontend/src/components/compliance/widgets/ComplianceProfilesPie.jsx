import { ChevronRight } from "lucide-react";
import { Bar } from "react-chartjs-2";
import { Link } from "react-router-dom";
import { useTheme } from "../../../contexts/ThemeContext";
import { getBarOptions } from "./chartOptions";

const PROFILE_COLORS = [
	"#3B82F6",
	"#10B981",
	"#F59E0B",
	"#8B5CF6",
	"#06B6D4",
	"#EF4444",
];

const ComplianceProfilesPie = ({ data, onTabChange }) => {
	const { isDark } = useTheme();

	const profile_distribution = data?.profile_distribution || [];
	const labels = profile_distribution.map((p) => p.name || p.type || "Unknown");
	const values = profile_distribution.map((p) => Number(p.host_count) || 0);
	const colors = profile_distribution.map(
		(_, i) => PROFILE_COLORS[i % PROFILE_COLORS.length],
	);
	const has_data = values.some((v) => v > 0);

	const chart_data = {
		labels: has_data ? labels : ["No profiles"],
		datasets: [
			{
				label: "Hosts",
				data: has_data ? values : [0],
				backgroundColor: has_data ? colors : ["#374151"],
				borderWidth: 0,
				borderRadius: 4,
				borderSkipped: false,
			},
		],
	};

	const options = getBarOptions(isDark, "y");

	return (
		<div className="card p-4 sm:p-6 w-full h-full flex flex-col">
			<h3 className="text-lg font-medium text-secondary-900 dark:text-white mb-4 flex-shrink-0">
				Compliance Profiles in Use
			</h3>
			<div className="h-56 flex-1 min-h-0">
				{has_data ? (
					<Bar data={chart_data} options={options} />
				) : (
					<div className="flex items-center justify-center h-full text-secondary-500 dark:text-white text-sm">
						No profile data yet
					</div>
				)}
			</div>
			{onTabChange ? (
				<button
					type="button"
					onClick={() => onTabChange("scan-results")}
					className="mt-3 flex items-center justify-center gap-1.5 w-full py-2 text-sm font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300 hover:underline flex-shrink-0"
				>
					View scan results
					<ChevronRight className="h-4 w-4" />
				</button>
			) : (
				<Link
					to="/compliance"
					state={{ complianceTab: "scan-results" }}
					className="mt-3 flex items-center justify-center gap-1.5 w-full py-2 text-sm font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300 hover:underline flex-shrink-0"
				>
					View scan results
					<ChevronRight className="h-4 w-4" />
				</Link>
			)}
		</div>
	);
};

export default ComplianceProfilesPie;
