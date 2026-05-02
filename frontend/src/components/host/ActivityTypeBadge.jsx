import { ArrowDown, ArrowUp } from "lucide-react";

// Reports (agent -> server, inbound). Each gets its own light/dark palette to
// keep visual signal high without relying on the arrow alone.
const REPORT_STYLE = {
	ping: {
		label: "PING",
		classes:
			"bg-secondary-100 text-secondary-700 dark:bg-secondary-700 dark:text-secondary-100",
	},
	full: {
		label: "FULL",
		classes: "bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200",
	},
	partial: {
		label: "PARTIAL",
		classes: "bg-sky-100 text-sky-800 dark:bg-sky-900 dark:text-sky-200",
	},
	docker: {
		label: "DOCKER",
		classes: "bg-cyan-100 text-cyan-800 dark:bg-cyan-900 dark:text-cyan-200",
	},
	compliance: {
		label: "COMPLIANCE",
		classes:
			"bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200",
	},
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
				className={`${baseChip} bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200`}
				title={`Outbound: server queued ${label} for the agent`}
			>
				<ArrowDown className="h-3 w-3 flex-shrink-0" />
				<span>{label}</span>
			</span>
		);
	}

	const reportKey = (type || "").toString().toLowerCase();
	const meta = REPORT_STYLE[reportKey] || {
		label: (type || "REPORT").toString().toUpperCase(),
		classes:
			"bg-secondary-100 text-secondary-700 dark:bg-secondary-700 dark:text-secondary-100",
	};

	return (
		<span
			className={`${baseChip} ${meta.classes}`}
			title={`Inbound: agent sent a ${meta.label.toLowerCase()} report to the server`}
		>
			<ArrowUp className="h-3 w-3 flex-shrink-0" />
			<span>{meta.label}</span>
		</span>
	);
};

export default ActivityTypeBadge;
