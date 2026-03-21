import {
	BarElement,
	CategoryScale,
	Chart as ChartJS,
	LinearScale,
	Tooltip,
} from "chart.js";
import { ChevronRight } from "lucide-react";
import { useMemo, useRef } from "react";
import { Bar } from "react-chartjs-2";
import { Link, useNavigate } from "react-router-dom";
import { useTheme } from "../../../contexts/ThemeContext";

ChartJS.register(CategoryScale, LinearScale, BarElement, Tooltip);

const TYPE_LABELS = {
	host_down: "Host Down",
	server_update: "Server Update",
	agent_update: "Agent Update",
};

const TYPE_COLORS = {
	host_down: "#ef4444",
	server_update: "#8b5cf6",
	agent_update: "#3b82f6",
};

const AlertsByType = ({ alerts }) => {
	const { isDark } = useTheme();
	const navigate = useNavigate();
	const chart_ref = useRef(null);

	const { labels, counts, colors, type_keys, has_data } = useMemo(() => {
		const type_map = {};
		for (const alert of alerts || []) {
			const type = alert.type || "unknown";
			type_map[type] = (type_map[type] || 0) + 1;
		}

		// Sort by count descending
		const sorted = Object.entries(type_map).sort((a, b) => b[1] - a[1]);
		const _labels = sorted.map(
			([type]) => TYPE_LABELS[type] || type.replace(/_/g, " "),
		);
		const _counts = sorted.map(([, count]) => count);
		const _colors = sorted.map(([type]) => TYPE_COLORS[type] || "#6b7280");
		const _keys = sorted.map(([type]) => type);

		return {
			labels: _labels,
			counts: _counts,
			colors: _colors,
			type_keys: _keys,
			has_data: sorted.length > 0,
		};
	}, [alerts]);

	const chart_data = {
		labels: has_data ? labels : ["No alerts"],
		datasets: [
			{
				data: has_data ? counts : [0],
				backgroundColor: has_data ? colors : ["#374151"],
				borderRadius: 4,
				barThickness: 28,
			},
		],
	};

	const options = {
		responsive: true,
		maintainAspectRatio: false,
		indexAxis: "y",
		plugins: {
			legend: { display: false },
		},
		scales: {
			x: {
				beginAtZero: true,
				ticks: {
					color: isDark ? "#ffffff" : "#374151",
					font: { size: 12 },
					precision: 0,
				},
				grid: { color: isDark ? "#374151" : "#e5e7eb" },
			},
			y: {
				ticks: {
					color: isDark ? "#ffffff" : "#374151",
					font: { size: 12 },
				},
				grid: { display: false },
			},
		},
		onClick: (_event, elements) => {
			if (elements.length > 0) {
				const type = type_keys[elements[0].index];
				if (type) {
					navigate(`/reporting?tab=alerts&type=${type}`);
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
				Alerts by Type
			</h3>
			<div className="flex-1 min-h-0">
				{has_data ? (
					<Bar ref={chart_ref} data={chart_data} options={options} />
				) : (
					<div className="flex items-center justify-center h-full text-secondary-500 dark:text-white text-sm">
						No active alerts
					</div>
				)}
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

export default AlertsByType;
