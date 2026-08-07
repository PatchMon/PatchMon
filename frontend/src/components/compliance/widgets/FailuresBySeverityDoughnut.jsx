import { ChevronRight } from "lucide-react";
import { useRef } from "react";
import { Doughnut } from "react-chartjs-2";
import { Link, useNavigate } from "react-router-dom";
import { useTheme } from "../../../contexts/ThemeContext";
import { getDoughnutOptions } from "./chartOptions";

const SEVERITY_COLORS = {
	critical: "#EF4444",
	high: "#F59E0B",
	medium: "#3B82F6",
	low: "#10B981",
	unknown: "#6B7280",
};

const FailuresBySeverityDoughnut = ({ data, onTabChange }) => {
	const { isDark } = useTheme();
	const navigate = useNavigate();
	const chart_ref = useRef(null);

	const goToScanResults = (severity) => {
		if (onTabChange) {
			onTabChange("scan-results", severity ? { severity } : undefined);
		} else {
			navigate("/compliance", {
				state: {
					complianceTab: "scan-results",
					scanResultsFilters: severity ? { severity } : undefined,
				},
			});
		}
	};

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
				borderWidth: 0,
			},
		],
	};

	const base_options = getDoughnutOptions(isDark, false);
	const options = {
		...base_options,
		onClick: (_event, elements) => {
			if (elements.length > 0 && has_data) {
				const severity = severity_breakdown[elements[0].index]?.severity;
				if (severity) {
					goToScanResults(severity);
				}
			}
		},
		onHover: (event, elements) => {
			event.native.target.style.cursor =
				has_data && elements.length > 0 ? "pointer" : "default";
		},
	};

	return (
		<div className="card p-4 sm:p-6 w-full h-full flex flex-col">
			<h3 className="text-lg font-medium text-secondary-900 dark:text-white mb-4 flex-shrink-0">
				Failures by Severity
			</h3>
			<div className="h-56 w-full flex items-center justify-center flex-1 min-h-0">
				<div className="w-full h-full max-w-sm">
					{has_data ? (
						<Doughnut ref={chart_ref} data={chart_data} options={options} />
					) : (
						<div className="flex items-center justify-center h-full text-secondary-500 dark:text-white text-sm">
							No failure data yet
						</div>
					)}
				</div>
			</div>
			{onTabChange ? (
				<button
					type="button"
					onClick={() => goToScanResults()}
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

export default FailuresBySeverityDoughnut;
