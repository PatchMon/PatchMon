import {
	ArcElement,
	BarElement,
	CategoryScale,
	Chart as ChartJS,
	Legend,
	LinearScale,
	Tooltip,
} from "chart.js";

ChartJS.register(
	ArcElement,
	BarElement,
	CategoryScale,
	LinearScale,
	Tooltip,
	Legend,
);

export { default as ComplianceProfilesPie } from "./ComplianceProfilesPie";
export { default as ComplianceTrendLinePlaceholder } from "./ComplianceTrendLinePlaceholder";
export { default as FailuresBySeverityDoughnut } from "./FailuresBySeverityDoughnut";
export { default as HostComplianceStatusBar } from "./HostComplianceStatusBar";
export { default as LastScanAgeBar } from "./LastScanAgeBar";
export { default as OpenSCAPDistributionDoughnut } from "./OpenSCAPDistributionDoughnut";

export const COMPLIANCE_WIDGET_CARD_IDS = [
	"complianceHostStatus",
	"complianceOpenSCAPDistribution",
	"complianceFailuresBySeverity",
	"complianceProfilesInUse",
	"complianceLastScanAge",
	"complianceTrendLine",
];
