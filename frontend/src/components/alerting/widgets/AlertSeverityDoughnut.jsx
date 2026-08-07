import { ChevronRight } from "lucide-react";
import { useRef } from "react";
import { Doughnut } from "react-chartjs-2";
import { Link, useNavigate } from "react-router-dom";
import { useTheme } from "../../../contexts/ThemeContext";
import { getDoughnutOptions } from "../../compliance/widgets/chartOptions";

const SEVERITY_FILTERS = ["critical", "error", "warning", "informational"];

const AlertSeverityDoughnut = ({ stats }) => {
	const { isDark } = useTheme();
	const navigate = useNavigate();
	const chart_ref = useRef(null);

	const critical = stats?.critical ?? 0;
	const error = stats?.error ?? 0;
	const warning = stats?.warning ?? 0;
	const informational = stats?.informational ?? 0;

	const has_data =
		critical > 0 || error > 0 || warning > 0 || informational > 0;

	const chart_data = {
		labels: has_data
			? ["Critical", "Error", "Warning", "Informational"]
			: ["No alerts"],
		datasets: [
			{
				data: has_data ? [critical, error, warning, informational] : [1],
				backgroundColor: has_data
					? ["#ef4444", "#f97316", "#f59e0b", "#3b82f6"]
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
				const severity = SEVERITY_FILTERS[elements[0].index];
				if (severity) {
					navigate(`/reporting?tab=alerts&severity=${severity}`);
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
				Alerts by Severity
			</h3>
			<div className="h-56 w-full flex items-center justify-center flex-1 min-h-0">
				<div className="w-full h-full max-w-sm">
					{has_data ? (
						<Doughnut ref={chart_ref} data={chart_data} options={options} />
					) : (
						<div className="flex items-center justify-center h-full text-secondary-500 dark:text-white text-sm">
							No active alerts
						</div>
					)}
				</div>
			</div>
			<Link
				to="/reporting?tab=alerts"
				className="mt-3 flex items-center justify-center gap-1.5 w-full py-2 text-sm font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300 hover:underline flex-shrink-0"
			>
				View all alerts
				<ChevronRight className="h-4 w-4" />
			</Link>
		</div>
	);
};

export default AlertSeverityDoughnut;
