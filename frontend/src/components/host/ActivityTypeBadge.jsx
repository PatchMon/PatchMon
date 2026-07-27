import { ArrowDown, ArrowUp } from "lucide-react";

// Direction is conveyed by the arrow icon (up = inbound, down = outbound),
// not by colour. Pills use a single neutral palette to reduce table noise.
const NEUTRAL_PILL_CLASSES =
	"bg-secondary-100 text-secondary-700 dark:bg-secondary-700 dark:text-secondary-100";

const REPORT_LABELS = {
	ping: "PING",
	full: "FULL",
	partial: "PARTIAL",
	docker: "DOCKER",
	compliance: "COMPLIANCE",
};

// Jobs (server -> agent, outbound). Single amber/orange palette so the
// "direction" reads instantly even before scanning the arrow.
const JOB_LABELS = {
	report_now: "Report Now",
	"report-now": "Report Now",
	refresh_integration_status: "Refresh Integration Status",
	"refresh-integration-status": "Refresh Integration Status",
	docker_inventory_refresh: "Docker Inventory Refresh",
	"docker-inventory-refresh": "Docker Inventory Refresh",
	update_agent: "Agent Update",
	"update-agent": "Agent Update",
	run_scan: "Compliance Scan",
	"run-scan": "Compliance Scan",
	install_compliance_tools: "Install Compliance Scanner",
	"install-compliance-tools": "Install Compliance Scanner",
	ssg_upgrade: "SSG Content Upgrade",
	"ssg-upgrade": "SSG Content Upgrade",
	run_patch: "Run Patch",
	"run-patch": "Run Patch",
	scheduled_reports_dispatch: "Scheduled Reports Dispatch",
	"scheduled-reports-dispatch": "Scheduled Reports Dispatch",
	scheduled_report_run: "Scheduled Report Run",
	"scheduled-report-run": "Scheduled Report Run",
	"update-threshold-monitor": "Update Threshold Monitor",
	"host-status-monitor": "Host Status Monitor",
	"metrics-send": "Metrics Send",
	"agent-reports-cleanup": "Agent Reports Cleanup",
	"patch-run-cleanup": "Patch Run Cleanup",
	"compliance-scan-cleanup": "Compliance Scan Cleanup",
	"ssg-update-check": "SSG Update Check",
	"version-update-check": "Version Update Check",
	"system-statistics": "System Statistics",
	"docker-inventory-cleanup": "Docker Inventory Cleanup",
	"orphaned-package-cleanup": "Orphaned Package Cleanup",
	"orphaned-repo-cleanup": "Orphaned Repo Cleanup",
	"session-cleanup": "Session Cleanup",
};

const formatJobLabel = (jobName) => {
	if (!jobName) return "Job";
	const lower = jobName.toString().toLowerCase();
	if (JOB_LABELS[lower]) return JOB_LABELS[lower];
	return jobName
		.toString()
		.replace(/[_-]+/g, " ")
		.replace(/\s+/g, " ")
		.trim()
		.split(" ")
		.map((word) =>
			word.length === 0 ? word : word[0].toUpperCase() + word.slice(1),
		)
		.join(" ");
};

const baseChip =
	"inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs font-medium whitespace-nowrap";

const ActivityTypeBadge = ({ kind, type, jobName }) => {
	if (kind === "job") {
		const label = formatJobLabel(jobName || type);
		return (
			<span
				className={`${baseChip} ${NEUTRAL_PILL_CLASSES}`}
				title={`Outbound: server queued ${label} for the agent`}
			>
				<ArrowDown className="h-3 w-3 flex-shrink-0" />
				<span>{label}</span>
			</span>
		);
	}

	const reportKey = (type || "").toString().toLowerCase();
	const label =
		REPORT_LABELS[reportKey] || (type || "REPORT").toString().toUpperCase();

	return (
		<span
			className={`${baseChip} ${NEUTRAL_PILL_CLASSES}`}
			title={`Inbound: agent sent a ${label.toLowerCase()} report to the server`}
		>
			<ArrowUp className="h-3 w-3 flex-shrink-0" />
			<span>{label}</span>
		</span>
	);
};

export default ActivityTypeBadge;
