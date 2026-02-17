/**
 * Shared Chart.js options for compliance widgets (theme-aware).
 * Use from widgets that use useTheme() and pass isDark.
 */
export function getDoughnutOptions(is_dark, is_mobile = false) {
	return {
		responsive: true,
		maintainAspectRatio: false,
		plugins: {
			legend: {
				position: is_mobile ? "bottom" : "right",
				labels: {
					color: is_dark ? "#ffffff" : "#374151",
					font: { size: is_mobile ? 10 : 12 },
					padding: is_mobile ? 10 : 15,
					usePointStyle: true,
					pointStyle: "circle",
				},
			},
		},
		layout: {
			padding: { right: is_mobile ? 10 : 20 },
		},
	};
}

export function getBarOptions(is_dark, index_axis = "x") {
	return {
		responsive: true,
		maintainAspectRatio: false,
		indexAxis: index_axis,
		plugins: {
			legend: {
				display: index_axis === "y",
				position: "top",
				labels: {
					color: is_dark ? "#ffffff" : "#374151",
					font: { size: 12 },
					padding: 15,
					usePointStyle: true,
				},
			},
		},
		scales: {
			x: {
				stacked: index_axis === "y",
				ticks: { color: is_dark ? "#ffffff" : "#374151", font: { size: 12 } },
				grid: { color: is_dark ? "#374151" : "#e5e7eb" },
			},
			y: {
				stacked: index_axis === "y",
				ticks: { color: is_dark ? "#ffffff" : "#374151", font: { size: 12 } },
				grid: { color: is_dark ? "#374151" : "#e5e7eb" },
			},
		},
	};
}

export function getPieOptions(is_dark) {
	return {
		responsive: true,
		maintainAspectRatio: false,
		plugins: {
			legend: {
				position: "right",
				labels: {
					color: is_dark ? "#ffffff" : "#374151",
					font: { size: 12 },
					padding: 15,
					usePointStyle: true,
					pointStyle: "circle",
				},
			},
		},
		layout: { padding: { right: 20 } },
	};
}
