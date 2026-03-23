import {
	BellRing,
	Bot,
	Boxes,
	Container,
	Crown,
	Download,
	Eye,
	FileBarChart,
	LayoutDashboard,
	MailCheck,
	Package,
	PackageSearch,
	PencilRuler,
	Send,
	Server,
	Settings,
	ShieldCheck,
	Terminal,
	UserCog,
	Users,
	Wrench,
} from "lucide-react";

export const RISK_LEVELS = {
	LOW: "low",
	MEDIUM: "medium",
	MEDIUM_HIGH: "medium-high",
	HIGH: "high",
};

export const PERMISSION_GROUPS = [
	{
		id: "monitoring",
		name: "Monitoring & Visibility",
		description:
			"Read-only access to dashboards, hosts, packages, reports, and logs",
		riskLevel: RISK_LEVELS.LOW,
		icon: Eye,
		permissions: [
			{
				key: "can_view_dashboard",
				label: "View Dashboard",
				description: "View dashboard statistics and overview panels",
				icon: LayoutDashboard,
			},
			{
				key: "can_view_hosts",
				label: "View Hosts",
				description: "View host list, details, and connection status",
				icon: Server,
			},
			{
				key: "can_view_packages",
				label: "View Packages",
				description: "View package inventory across all hosts",
				icon: Package,
			},
			{
				key: "can_view_reports",
				label: "View Reports",
				description: "View compliance scans and alert reports",
				icon: FileBarChart,
			},
			{
				key: "can_view_notification_logs",
				label: "View Notification Logs",
				description: "View notification delivery history and status",
				icon: MailCheck,
			},
		],
	},
	{
		id: "infrastructure",
		name: "Host & Infrastructure",
		description: "Create, modify, and delete hosts, packages, and containers",
		riskLevel: RISK_LEVELS.MEDIUM,
		icon: PencilRuler,
		permissions: [
			{
				key: "can_manage_hosts",
				label: "Manage Hosts",
				description:
					"Create, edit, delete hosts, host groups, repositories, and integrations",
				icon: Boxes,
			},
			{
				key: "can_manage_packages",
				label: "Manage Packages",
				description: "Manage package inventory and metadata",
				icon: PackageSearch,
			},
			{
				key: "can_manage_docker",
				label: "Manage Docker",
				description: "Delete Docker containers, images, volumes, and networks",
				icon: Container,
			},
		],
	},
	{
		id: "operations",
		name: "Operations",
		description:
			"Day-to-day NOC tasks: patching, compliance, alerts, automation, remote access",
		riskLevel: RISK_LEVELS.MEDIUM_HIGH,
		icon: Wrench,
		permissions: [
			{
				key: "can_manage_patching",
				label: "Manage Patching",
				description: "Trigger patches, approve runs, manage patch policies",
				icon: Wrench,
			},
			{
				key: "can_manage_compliance",
				label: "Manage Compliance",
				description:
					"Trigger compliance scans, remediate findings, install scanners",
				icon: ShieldCheck,
			},
			{
				key: "can_manage_alerts",
				label: "Manage Alerts",
				description: "Alert actions, assign, delete, and bulk operations",
				icon: BellRing,
			},
			{
				key: "can_manage_automation",
				label: "Manage Automation",
				description: "Trigger and manage automation jobs",
				icon: Bot,
			},
			{
				key: "can_use_remote_access",
				label: "Remote Access",
				description: "SSH and RDP terminal access to managed hosts",
				icon: Terminal,
			},
		],
	},
	{
		id: "administration",
		name: "Administration",
		description:
			"Organization-level control: users, settings, notifications, data export",
		riskLevel: RISK_LEVELS.HIGH,
		icon: Crown,
		permissions: [
			{
				key: "can_view_users",
				label: "View Users",
				description: "View user list and account details",
				icon: Users,
			},
			{
				key: "can_manage_users",
				label: "Manage Users",
				description: "Create, edit, and delete user accounts",
				icon: UserCog,
			},
			{
				key: "can_manage_superusers",
				label: "Manage Superusers",
				description: "Manage superadmin accounts and elevated privileges",
				icon: Crown,
			},
			{
				key: "can_manage_settings",
				label: "Manage Settings",
				description:
					"System configuration, OIDC, AI, alert config, enrollment tokens",
				icon: Settings,
			},
			{
				key: "can_manage_notifications",
				label: "Manage Notifications",
				description: "Configure notification destinations and routing rules",
				icon: Send,
			},
			{
				key: "can_export_data",
				label: "Export Data",
				description: "Download and export data and reports",
				icon: Download,
			},
		],
	},
];

export const countPermissions = (roleObj) => {
	let enabled = 0;
	let total = 0;
	for (const group of PERMISSION_GROUPS) {
		for (const perm of group.permissions) {
			total++;
			if (roleObj[perm.key]) enabled++;
		}
	}
	return { enabled, total };
};

export const riskBorderColor = (level) =>
	({
		[RISK_LEVELS.LOW]: "border-l-green-500",
		[RISK_LEVELS.MEDIUM]: "border-l-blue-500",
		[RISK_LEVELS.MEDIUM_HIGH]: "border-l-yellow-500",
		[RISK_LEVELS.HIGH]: "border-l-red-500",
	})[level] || "border-l-gray-500";

export const riskBadgeClasses = (level) =>
	({
		[RISK_LEVELS.LOW]:
			"bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400",
		[RISK_LEVELS.MEDIUM]:
			"bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400",
		[RISK_LEVELS.MEDIUM_HIGH]:
			"bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400",
		[RISK_LEVELS.HIGH]:
			"bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400",
	})[level] || "bg-gray-100 text-gray-700";

export const riskGroupBg = (level) =>
	({
		[RISK_LEVELS.LOW]: "bg-green-50/60 dark:bg-green-900/10",
		[RISK_LEVELS.MEDIUM]: "bg-blue-50/60 dark:bg-blue-900/10",
		[RISK_LEVELS.MEDIUM_HIGH]: "bg-yellow-50/60 dark:bg-yellow-900/10",
		[RISK_LEVELS.HIGH]: "bg-red-50/60 dark:bg-red-900/10",
	})[level] || "bg-gray-50";

export const riskLabel = (level) =>
	({
		[RISK_LEVELS.LOW]: "Low Risk",
		[RISK_LEVELS.MEDIUM]: "Medium",
		[RISK_LEVELS.MEDIUM_HIGH]: "Medium-High",
		[RISK_LEVELS.HIGH]: "High Risk",
	})[level] || "Unknown";

const buildPreset = (predicate) => {
	const data = {};
	for (const g of PERMISSION_GROUPS) {
		for (const p of g.permissions) {
			data[p.key] = predicate(g);
		}
	}
	return data;
};

export const ROLE_PRESETS = {
	readonly: buildPreset((g) => g.id === "monitoring"),
	operator: buildPreset((g) => g.id !== "administration"),
	admin: buildPreset(() => true),
	clear: buildPreset(() => false),
};
