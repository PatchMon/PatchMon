import { Doughnut } from "react-chartjs-2";
import { useTheme } from "../../../contexts/ThemeContext";
import { getDoughnutOptions } from "./chartOptions";

const SEVERITY_COLORS = {
	critical: "#EF4444",
	high: "#F59E0B",
	medium: "#3B82F6",
	low: "#10B981",
	unknown: "#6B7280",
};

const FailuresBySeverityDoughnut = ({ data }) => {
	const { isDark } = useTheme();

	const severity_breakdown = data?.severity_breakdown || [];
	const labels = severity_breakdown.map((s) =>
		s.severity
			? `${s.severity.charAt(0).toUpperCase()}${s.severity.slice(1)}`
			: "Unknown",
	);
	const values = severity_breakdown.map((s) => Number(s.count) || 0);
	const colors = severity_breakdown.map(
		(s) => SEVERITY_COLORS[s.severity] || SEVERITY_COLORS.unknown,
	);
	const has_data = values.some((v) => v > 0);

	const chart_data = {
		labels: has_data ? labels : ["No failures"],
		datasets: [
			{
				data: has_data ? values : [1],
				backgroundColor: has_data ? colors : ["#374151"],
				borderWidth: 2,
				borderColor: "#ffffff",
			},
		],
	};

	const options = getDoughnutOptions(isDark, false);

	return (
		<div className="card p-4 sm:p-6 w-full">
			<h3 className="text-lg font-medium text-secondary-900 dark:text-white mb-4">
				Failures by Severity
			</h3>
			<div className="h-64 w-full flex items-center justify-center">
				<div className="w-full h-full max-w-sm">
					{has_data ? (
						<Doughnut data={chart_data} options={options} />
					) : (
						<div className="flex items-center justify-center h-full text-secondary-500 dark:text-secondary-400 text-sm">
							No failure data yet
						</div>
					)}
				</div>
			</div>
		</div>
	);
};

export default FailuresBySeverityDoughnut;
