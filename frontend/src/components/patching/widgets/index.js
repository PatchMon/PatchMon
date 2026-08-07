import { ArcElement, Chart as ChartJS, Legend, Tooltip } from "chart.js";

ChartJS.register(ArcElement, Tooltip, Legend);

export { default as PatchingActivePolicies } from "./PatchingActivePolicies";
export { default as PatchingPendingApproval } from "./PatchingPendingApproval";
export { default as PatchingRecentRuns } from "./PatchingRecentRuns";
export { default as PatchRunOutcomesDoughnut } from "./PatchRunOutcomesDoughnut";
export { default as PatchRunStatusBoxes } from "./PatchRunStatusBoxes";
export { default as PatchRunsByType } from "./PatchRunsByType";

export const PATCHING_WIDGET_CARD_IDS = [
	"patchingRunStatus",
	"patchingRunOutcomesDoughnut",
	"patchingPendingApproval",
	"patchingRunsByType",
	"patchingActivePolicies",
	"patchingRecentRuns",
];
