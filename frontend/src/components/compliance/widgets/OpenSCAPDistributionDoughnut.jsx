import { useState } from "react";
import { Bar } from "react-chartjs-2";
import { useTheme } from "../../../contexts/ThemeContext";
import { getBarOptions } from "./chartOptions";

const ROW_LABELS = ["OpenSCAP", "Docker Bench"];
const PASSED_COLOR = "#10B981";
const FAILED_COLOR = "#EF4444";
const WARNINGS_COLOR = "#F59E0B";
const SKIPPED_COLOR = "#6B7280";

function getSegmentValues(stats) {
	const passed = stats?.total_passed ?? 0;
	const failed = stats?.total_failed ?? 0;
	const warnings = stats?.total_warnings ?? 0;
	const total_rules = stats?.total_rules ?? 0;
	const skipped = Math.max(0, total_rules - passed - failed - warnings);
	return { passed, failed, warnings, skipped };
}

const OpenSCAPDistributionDoughnut = ({ data }) => {
	const { isDark } = useTheme();
	const [show_skipped, set_show_skipped] = useState(false);

	const profile_stats = data?.profile_type_stats || [];
	const openscap = profile_stats.find((p) => p.type === "openscap");
	const docker_bench = profile_stats.find((p) => p.type === "docker-bench");

	const osc = getSegmentValues(openscap);
	const db = getSegmentValues(docker_bench);

	const has_data =
		(openscap?.total_rules ?? 0) > 0 || (docker_bench?.total_rules ?? 0) > 0;

	const datasets = [
		{
			label: "Passed",
			data: [osc.passed, db.passed],
			backgroundColor: PASSED_COLOR,
			borderWidth: 0,
		},
		{
			label: "Failed",
			data: [osc.failed, db.failed],
			backgroundColor: FAILED_COLOR,
			borderWidth: 0,
		},
		{
			label: "Warnings",
			data: [osc.warnings, db.warnings],
			backgroundColor: WARNINGS_COLOR,
			borderWidth: 0,
		},
		...(show_skipped
			? [
					{
						label: "Skipped",
						data: [osc.skipped, db.skipped],
						backgroundColor: SKIPPED_COLOR,
						borderWidth: 0,
					},
				]
			: []),
	];

	const chart_data = {
		labels: ROW_LABELS,
		datasets,
	};

	const options = getBarOptions(isDark, "y");

	return (
		<div className="card p-4 sm:p-6 w-full h-full flex flex-col">
			<div className="flex flex-wrap items-center justify-between gap-2 mb-4 flex-shrink-0">
				<h3 className="text-lg font-medium text-secondary-900 dark:text-white">
					Benchmark Distribution
				</h3>
				<label className="flex items-center gap-2 cursor-pointer">
					<input
						type="checkbox"
						checked={show_skipped}
						onChange={(e) => set_show_skipped(e.target.checked)}
						className="rounded border-secondary-300 dark:border-secondary-600 text-primary-600 focus:ring-primary-500"
					/>
					<span className="text-sm text-secondary-600 dark:text-secondary-400">
						Show skipped
					</span>
				</label>
			</div>
			<div className="h-48 sm:h-56 w-full flex-1 min-h-0">
				{has_data ? (
					<Bar data={chart_data} options={options} />
				) : (
					<div className="flex items-center justify-center h-full text-secondary-500 dark:text-secondary-400 text-sm">
						No benchmark data yet
					</div>
				)}
			</div>
		</div>
	);
};

export default OpenSCAPDistributionDoughnut;
