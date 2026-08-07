import { useQuery } from "@tanstack/react-query";
import { ChevronRight } from "lucide-react";
import { useMemo, useState } from "react";
import { Bar } from "react-chartjs-2";
import { Link, useNavigate } from "react-router-dom";
import { useTheme } from "../../../contexts/ThemeContext";
import { notificationsAPI } from "../../../utils/api";

const DEST_COLORS = [
	"#3b82f6",
	"#8b5cf6",
	"#10b981",
	"#f59e0b",
	"#ef4444",
	"#ec4899",
	"#06b6d4",
	"#f97316",
];

const DeliveryByDestination = () => {
	const { isDark } = useTheme();
	const navigate = useNavigate();
	const [period, setPeriod] = useState("7d");
	const days = period === "7d" ? 7 : 30;

	const { data: logEntries = [] } = useQuery({
		queryKey: ["notifications", "delivery-log", "chart"],
		queryFn: async () => {
			const response = await notificationsAPI.listDeliveryLog({
				limit: 500,
			});
			return response.data?.data || response.data || [];
		},
		staleTime: 60 * 1000,
		refetchInterval: 60 * 1000,
	});

	const { data: destinations = [] } = useQuery({
		queryKey: ["notifications", "destinations"],
		queryFn: async () => {
			const response = await notificationsAPI.listDestinations();
			return response.data?.data || response.data || [];
		},
		staleTime: 5 * 60 * 1000,
	});

	const dest_map = useMemo(() => {
		const m = {};
		for (const d of destinations) {
			m[d.id] = d.display_name || d.channel_type || d.id;
		}
		return m;
	}, [destinations]);

	const { labels, datasets, has_data } = useMemo(() => {
		const now = new Date();
		const cutoff = new Date(now);
		cutoff.setDate(cutoff.getDate() - days);

		const filtered = logEntries.filter((e) => new Date(e.created_at) >= cutoff);

		// Build day labels
		const day_labels = [];
		const day_keys = [];
		for (let i = days - 1; i >= 0; i--) {
			const date = new Date(now);
			date.setDate(date.getDate() - i);
			date.setHours(0, 0, 0, 0);
			day_keys.push(date.toISOString().slice(0, 10));
			day_labels.push(
				days <= 7
					? date.toLocaleDateString("en-US", { weekday: "short" })
					: date.toLocaleDateString("en-US", {
							month: "short",
							day: "numeric",
						}),
			);
		}

		// Group by destination + day
		const dest_ids = [...new Set(filtered.map((e) => e.destination_id))];
		const dest_day_counts = {};
		for (const id of dest_ids) {
			dest_day_counts[id] = {};
			for (const dk of day_keys) {
				dest_day_counts[id][dk] = 0;
			}
		}
		for (const entry of filtered) {
			const dk = new Date(entry.created_at).toISOString().slice(0, 10);
			if (dest_day_counts[entry.destination_id]?.[dk] != null) {
				dest_day_counts[entry.destination_id][dk]++;
			}
		}

		const chart_datasets = dest_ids.map((id, idx) => ({
			label: dest_map[id] || id.slice(0, 8),
			data: day_keys.map((dk) => dest_day_counts[id][dk]),
			backgroundColor: DEST_COLORS[idx % DEST_COLORS.length],
			borderRadius: 3,
		}));

		return {
			labels: day_labels,
			datasets: chart_datasets,
			has_data: filtered.length > 0,
		};
	}, [logEntries, dest_map, days]);

	const chart_data = { labels, datasets };

	const options = {
		responsive: true,
		maintainAspectRatio: false,
		interaction: {
			mode: "index",
			intersect: false,
		},
		onClick: (_event, elements) => {
			if (elements.length > 0) {
				navigate("/reporting?tab=log");
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
			tooltip: {
				mode: "index",
				intersect: false,
			},
		},
		scales: {
			x: {
				stacked: true,
				ticks: {
					color: isDark ? "#ffffff" : "#374151",
					font: { size: 11 },
				},
				grid: { color: isDark ? "#374151" : "#e5e7eb" },
			},
			y: {
				stacked: true,
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
			<div className="flex items-center justify-between mb-4 flex-shrink-0">
				<h3 className="text-lg font-medium text-secondary-900 dark:text-white">
					Delivery by Destination
				</h3>
				<div className="flex gap-1">
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
					<Bar data={chart_data} options={options} />
				) : (
					<div className="flex items-center justify-center h-full text-secondary-500 dark:text-white text-sm">
						No delivery data for this period
					</div>
				)}
			</div>
			<Link
				to="/reporting?tab=log"
				className="mt-3 flex items-center justify-center gap-1.5 w-full py-2 text-sm font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300 hover:underline flex-shrink-0"
			>
				View delivery log
				<ChevronRight className="h-4 w-4" />
			</Link>
		</div>
	);
};

export default DeliveryByDestination;
