import { Doughnut } from "react-chartjs-2";
import { useTheme } from "../../../contexts/ThemeContext";
import { getDoughnutOptions } from "../../compliance/widgets/chartOptions";

const PatchRunOutcomesDoughnut = ({ data }) => {
	const { isDark } = useTheme();

	const summary = data?.summary || {};
	const completed = summary.completed ?? 0;
	const failed = summary.failed ?? 0;
	const cancelled = summary.cancelled ?? 0;

	const has_data = completed > 0 || failed > 0 || cancelled > 0;

	const chart_data = {
		labels: has_data ? ["Completed", "Failed", "Cancelled"] : ["No runs yet"],
		datasets: [
			{
				data: has_data ? [completed, failed, cancelled] : [1],
				backgroundColor: has_data
					? ["#10B981", "#EF4444", "#6B7280"]
					: ["#374151"],
				borderWidth: 2,
				borderColor: isDark ? "#1f2937" : "#ffffff",
			},
		],
	};

	const options = getDoughnutOptions(isDark, false);

	return (
		<div className="card p-4 sm:p-6 w-full h-full flex flex-col">
			<h3 className="text-lg font-medium text-secondary-900 dark:text-white mb-4 flex-shrink-0">
				Run Outcomes
			</h3>
			<div className="h-64 w-full flex items-center justify-center flex-1 min-h-0">
				<div className="w-full h-full max-w-sm">
					{has_data ? (
						<Doughnut data={chart_data} options={options} />
					) : (
						<div className="flex items-center justify-center h-full text-secondary-500 dark:text-white text-sm">
							No completed runs yet
						</div>
					)}
				</div>
			</div>
		</div>
	);
};

export default PatchRunOutcomesDoughnut;
