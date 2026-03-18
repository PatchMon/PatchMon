import { ChevronRight } from "lucide-react";
import { useRef } from "react";
import { Doughnut } from "react-chartjs-2";
import { Link, useNavigate } from "react-router-dom";
import { useTheme } from "../../../contexts/ThemeContext";
import { getDoughnutOptions } from "../../compliance/widgets/chartOptions";

const TYPE_FILTERS = ["patch_all", "patch_package"];

const PatchRunsByType = ({ data }) => {
	const { isDark } = useTheme();
	const navigate = useNavigate();
	const chart_ref = useRef(null);

	const recent_runs = data?.recent_runs || [];

	let patch_all = 0;
	let patch_package = 0;
	for (const run of recent_runs) {
		if (run.patch_type === "patch_all") {
			patch_all++;
		} else if (run.patch_type === "patch_package") {
			patch_package++;
		}
	}

	const has_data = patch_all > 0 || patch_package > 0;

	const chart_data = {
		labels: has_data ? ["Patch All", "Patch Package"] : ["No runs yet"],
		datasets: [
			{
				data: has_data ? [patch_all, patch_package] : [1],
				backgroundColor: has_data ? ["#3B82F6", "#8B5CF6"] : ["#374151"],
				borderWidth: 2,
				borderColor: isDark ? "#1f2937" : "#ffffff",
			},
		],
	};

	const base_options = getDoughnutOptions(isDark, false);
	const options = {
		...base_options,
		onClick: (_event, elements) => {
			if (elements.length > 0) {
				const type = TYPE_FILTERS[elements[0].index];
				if (type) {
					navigate(`/patching?tab=runs`);
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
				Runs by Type
			</h3>
			<div className="h-56 w-full flex items-center justify-center flex-1 min-h-0">
				<div className="w-full h-full max-w-sm">
					{has_data ? (
						<Doughnut ref={chart_ref} data={chart_data} options={options} />
					) : (
						<div className="flex items-center justify-center h-full text-secondary-500 dark:text-white text-sm">
							No run data yet
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

export default PatchRunsByType;
