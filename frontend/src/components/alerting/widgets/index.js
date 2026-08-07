import {
	ArcElement,
	BarElement,
	CategoryScale,
	Chart as ChartJS,
	Filler,
	Legend,
	LinearScale,
	LineElement,
	PointElement,
	Tooltip,
} from "chart.js";

ChartJS.register(
	ArcElement,
	BarElement,
	CategoryScale,
	Filler,
	Legend,
	LineElement,
	LinearScale,
	PointElement,
	Tooltip,
);

export { default as AlertResponderWorkload } from "./AlertResponderWorkload";
export { default as AlertSeverityDoughnut } from "./AlertSeverityDoughnut";
export { default as AlertStatusBoxes } from "./AlertStatusBoxes";
export { default as AlertsByType } from "./AlertsByType";
export { default as AlertsRequiringAttention } from "./AlertsRequiringAttention";
export { default as AlertVolumeTrend } from "./AlertVolumeTrend";
export { default as DeliveryByDestination } from "./DeliveryByDestination";
export { default as RecentAlerts } from "./RecentAlerts";

export const ALERTING_WIDGET_CARD_IDS = [
	"alertStatusBoxes",
	"alertSeverityDoughnut",
	"alertVolumeTrend",
	"alertsByType",
	"recentAlerts",
	"alertResponderWorkload",
	"deliveryByDestination",
];
