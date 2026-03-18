import { ChevronRight } from "lucide-react";
import { useRef } from "react";
import { Doughnut } from "react-chartjs-2";
import { Link, useNavigate } from "react-router-dom";
import { useTheme } from "../../../contexts/ThemeContext";
import { getDoughnutOptions } from "../../compliance/widgets/chartOptions";

const STATUS_LINKS = ["completed", "failed", "cancelled"];

const PatchRunOutcomesDoughnut = ({ data }) => {
	const { isDark } = useTheme();
	const navigate = useNavigate();
	const chart_ref = useRef(null);

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
				borderWidth: 0,
			},
		],
	};

	const base_options = getDoughnutOptions(isDark, false);
	const options = {
		...base_options,
		onClick: (_event, elements) => {
			if (elements.length > 0) {
				const status = STATUS_LINKS[elements[0].index];
				if (status) {
					navigate(`/patching?tab=runs&status=${status}`);
				}
			}
		},
		onHover: (event, elements) => {
			event.native.target.style.cursor =
				elements.length > 0 ? "pointer" : "default";
		},
	};

	return (
		<div className="card p-4 sm:p-6 w-full h-full flex flex-col">
			<h3 className="text-lg font-medium text-secondary-900 dark:text-white mb-4 flex-shrink-0">
				Run Outcomes
			</h3>
			<div className="h-56 w-full flex items-center justify-center flex-1 min-h-0">
				<div className="w-full h-full max-w-sm">
					{has_data ? (
						<Doughnut ref={chart_ref} data={chart_data} options={options} />
					) : (
						<div className="flex items-center justify-center h-full text-secondary-500 dark:text-white text-sm">
							No completed runs yet
						</div>
					)}
				</div>
			</div>
			<Link
				to="/patching?tab=runs"
				className="mt-3 flex items-center justify-center gap-1.5 w-full py-2 text-sm font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300 hover:underline flex-shrink-0"
			>
				View all runs
				<ChevronRight className="h-4 w-4" />
			</Link>
		</div>
	);
};

export default PatchRunOutcomesDoughnut;
