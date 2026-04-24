import {
	CategoryScale,
	Chart as ChartJS,
	Filler,
	LinearScale,
	LineElement,
	PointElement,
	Tooltip,
} from "chart.js";
import { ChevronRight } from "lucide-react";
import { useMemo, useState } from "react";
import { Line } from "react-chartjs-2";
import { Link, useNavigate } from "react-router-dom";
import { useTheme } from "../../../contexts/ThemeContext";

ChartJS.register(
	CategoryScale,
	LinearScale,
	PointElement,
	LineElement,
	Filler,
	Tooltip,
);

const SEVERITY_COLORS = {
	critical: { border: "#ef4444", bg: "rgba(239, 68, 68, 0.08)" },
	error: { border: "#f97316", bg: "rgba(249, 115, 22, 0.08)" },
	warning: { border: "#f59e0b", bg: "rgba(245, 158, 11, 0.08)" },
	informational: { border: "#3b82f6", bg: "rgba(59, 130, 246, 0.08)" },
};

const AlertVolumeTrend = ({ alerts }) => {
	const { isDark } = useTheme();
	const navigate = useNavigate();
	const [period, setPeriod] = useState("7d");
	const [view, setView] = useState("volume");
	const days = period === "7d" ? 7 : 30;

	const { labels, new_counts, resolved_counts, severity_counts, has_data } =
		useMemo(() => {
			const now = new Date();
			const day_labels = [];
			const new_by_day = [];
			const resolved_by_day = [];
			const sev = {
				critical: [],
				error: [],
				warning: [],
				informational: [],
			};
			let any_data = false;

			for (let i = days - 1; i >= 0; i--) {
				const date = new Date(now);
				date.setDate(date.getDate() - i);
				date.setHours(0, 0, 0, 0);
				const next_day = new Date(date);
				next_day.setDate(next_day.getDate() + 1);

				const label =
					days <= 7
						? date.toLocaleDateString("en-US", { weekday: "short" })
						: date.toLocaleDateString("en-US", {
								month: "short",
								day: "numeric",
							});
				day_labels.push(label);

				const day_alerts = (alerts || []).filter((a) => {
					const created = new Date(a.created_at);
					return created >= date && created < next_day;
				});

				new_by_day.push(day_alerts.length);

				const resolved_count = (alerts || []).filter((a) => {
					if (!a.resolved_at) return false;
					const resolved = new Date(a.resolved_at);
					return resolved >= date && resolved < next_day;
				}).length;
				resolved_by_day.push(resolved_count);

				for (const key of Object.keys(sev)) {
					sev[key].push(day_alerts.filter((a) => a.severity === key).length);
				}

				if (day_alerts.length > 0 || resolved_count > 0) any_data = true;
			}

			return {
				labels: day_labels,
				new_counts: new_by_day,
				resolved_counts: resolved_by_day,
				severity_counts: sev,
				has_data: any_data,
			};
		}, [alerts, days]);

	const volume_datasets = [
		{
			label: "New Alerts",
			data: new_counts,
			borderColor: "#ef4444",
			backgroundColor: "rgba(239, 68, 68, 0.08)",
			fill: true,
			tension: 0.3,
			pointRadius: 3,
			pointHoverRadius: 5,
		},
		{
			label: "Resolved",
			data: resolved_counts,
			borderColor: "#10b981",
			backgroundColor: "rgba(16, 185, 129, 0.08)",
			fill: true,
			tension: 0.3,
			pointRadius: 3,
			pointHoverRadius: 5,
		},
	];

	const severity_datasets = Object.entries(SEVERITY_COLORS).map(
		([key, colors]) => ({
			label: key.charAt(0).toUpperCase() + key.slice(1),
			data: severity_counts[key],
			borderColor: colors.border,
			backgroundColor: colors.bg,
			fill: true,
			tension: 0.3,
			pointRadius: 3,
			pointHoverRadius: 5,
		}),
	);

	const chart_data = {
		labels,
		datasets: view === "volume" ? volume_datasets : severity_datasets,
	};

	const options = {
		responsive: true,
		maintainAspectRatio: false,
		interaction: {
			mode: "index",
			intersect: false,
		},
		onClick: (_event, elements) => {
			if (elements.length > 0) {
				navigate("/reporting?tab=alerts");
			}
		},
		onHover: (event, elements) => {
			event.native.target.style.cursor =
				elements.length > 0 ? "pointer" : "default";
		},
		plugins: {
			legend: {
				position: "top",
				align: "end",
				labels: {
					color: isDark ? "#ffffff" : "#374151",
					font: { size: 11 },
					usePointStyle: true,
					pointStyle: "circle",
					padding: 12,
				},
			},
		},
		scales: {
			x: {
				ticks: {
					color: isDark ? "#ffffff" : "#374151",
					font: { size: 11 },
				},
				grid: { color: isDark ? "#374151" : "#e5e7eb" },
			},
			y: {
				beginAtZero: true,
				ticks: {
					color: isDark ? "#ffffff" : "#374151",
					font: { size: 11 },
					precision: 0,
				},
				grid: { color: isDark ? "#374151" : "#e5e7eb" },
			},
		},
	};

	const pill_base =
		"text-xs font-medium px-2.5 py-1 rounded-full cursor-pointer transition-colors";
	const pill_active =
		"bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300";
	const pill_inactive =
		"bg-secondary-100 text-secondary-600 dark:bg-secondary-800 dark:text-secondary-300 hover:bg-secondary-200 dark:hover:bg-secondary-700";

	return (
		<div className="card p-4 sm:p-6 w-full h-full flex flex-col">
			<div className="flex items-center justify-between mb-4 flex-shrink-0 gap-2 flex-wrap">
				<h3 className="text-lg font-medium text-secondary-900 dark:text-white">
					Alert Volume Trend
				</h3>
				<div className="flex gap-1">
					<button
						type="button"
						onClick={() => setView("volume")}
						className={`${pill_base} ${view === "volume" ? pill_active : pill_inactive}`}
					>
						Volume
					</button>
					<button
						type="button"
						onClick={() => setView("severity")}
						className={`${pill_base} ${view === "severity" ? pill_active : pill_inactive}`}
					>
						Severity
					</button>
					<span className="mx-1 border-l border-secondary-300 dark:border-secondary-600" />
					<button
						type="button"
						onClick={() => setPeriod("7d")}
						className={`${pill_base} ${period === "7d" ? pill_active : pill_inactive}`}
					>
						7d
					</button>
					<button
						type="button"
						onClick={() => setPeriod("30d")}
						className={`${pill_base} ${period === "30d" ? pill_active : pill_inactive}`}
					>
						30d
					</button>
				</div>
			</div>
			<div className="flex-1 min-h-0">
				{has_data ? (
					<Line data={chart_data} options={options} />
				) : (
					<div className="flex items-center justify-center h-full text-secondary-500 dark:text-white text-sm">
						No alert data for this period
					</div>
				)}
			</div>
			<Link
				to="/reporting?tab=alerts"
				className="mt-3 flex items-center justify-center gap-1.5 w-full py-2 text-sm font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300 hover:underline flex-shrink-0"
			>
				View alert history
				<ChevronRight className="h-4 w-4" />
			</Link>
		</div>
	);
};

export default AlertVolumeTrend;
