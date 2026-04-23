// Tier feature matrix — hardcoded from
// docs/research/per-host-pricing/10-tiers-v2-starter-plus-max.md §1.
//
// Kept in lockstep with `patchmon.net-new-website/src/data/tiers.ts` and
// the module catalog in `multi-tenancy/go-manager/internal/modules/catalog.go`.
// If the marketing tier table changes, update this file too.

export const TIER_ORDER = ["starter", "plus", "max"];

export const TIERS = {
	starter: {
		id: "starter",
		name: "Starter",
		tagline: "Monitor your patches. Patch elsewhere.",
		userLimit: 3,
		unitAmountCents: 100, // per host / month (psychological parity across USD/GBP/EUR)
		// Tailwind badge classes — match Packages list in the manager
		badgeClass: "bg-sky-100 text-sky-800 dark:bg-sky-900 dark:text-sky-200",
		accentClass: "border-sky-500 ring-sky-500",
	},
	plus: {
		id: "plus",
		name: "Plus",
		tagline: "Patch, monitor, report. The everyday workhorse.",
		userLimit: null, // unlimited
		unitAmountCents: 200,
		badgeClass:
			"bg-indigo-100 text-indigo-800 dark:bg-indigo-900 dark:text-indigo-200",
		accentClass: "border-indigo-500 ring-indigo-500",
	},
	max: {
		id: "max",
		name: "Max",
		tagline: "Everything. SSH, RDP, AI, compliance.",
		userLimit: null,
		unitAmountCents: 300,
		badgeClass:
			"bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200",
		accentClass: "border-purple-500 ring-purple-500",
	},
};

// Feature matrix rows. `value` entries are either boolean or a display string.
export const TIER_FEATURES = [
	{
		label: "Core monitoring (inventory, detection, repos)",
		starter: true,
		plus: true,
		max: true,
	},
	{
		label: "Linux + FreeBSD + Windows agents",
		starter: true,
		plus: true,
		max: true,
	},
	{
		label: "Host groups, dashboards, search, Gethomepage",
		starter: true,
		plus: true,
		max: true,
	},
	{
		label: "Basic alerts (host-down, threshold)",
		starter: true,
		plus: true,
		max: true,
	},
	{
		label: "Scheduled reports (email/PDF)",
		starter: true,
		plus: true,
		max: true,
	},
	{
		label: "Notification destinations + routes",
		starter: true,
		plus: true,
		max: true,
	},
	{ label: "2FA / TOTP", starter: true, plus: true, max: true },
	{ label: "Trusted devices", starter: true, plus: true, max: true },
	{ label: "OIDC / SSO", starter: true, plus: true, max: true },
	{ label: "Discord OAuth login", starter: true, plus: true, max: true },
	{
		label: "Built-in roles (admin/viewer)",
		starter: true,
		plus: true,
		max: true,
	},
	{
		label: "Scoped REST API + auto-enrollment tokens",
		starter: true,
		plus: true,
		max: true,
	},
	{
		label: "Automation / job management",
		starter: true,
		plus: true,
		max: true,
	},
	{
		label: "User limit",
		starter: "3",
		plus: "Unlimited",
		max: "Unlimited",
	},
	{ label: "Manual patch runs", starter: false, plus: true, max: true },
	{
		label: "Patch scheduling policies + approval workflow",
		starter: false,
		plus: true,
		max: true,
	},
	{
		label: "Docker container monitoring",
		starter: false,
		plus: true,
		max: true,
	},
	{
		label: "Advanced alert config + custom rules",
		starter: false,
		plus: true,
		max: true,
	},
	{ label: "Custom RBAC roles", starter: false, plus: true, max: true },
	{ label: "Audit log export", starter: false, plus: true, max: true },
	{
		label: "Custom domain (email to request)",
		starter: false,
		plus: true,
		max: true,
	},
	{
		label: "Custom branding (logo/favicon)",
		starter: false,
		plus: true,
		max: true,
	},
	{ label: "Browser SSH terminal", starter: false, plus: false, max: true },
	{
		label: "Browser RDP (Guacamole)",
		starter: false,
		plus: false,
		max: true,
	},
	{
		label: "BYO-AI terminal assistant",
		starter: false,
		plus: false,
		max: true,
	},
	{
		label: "Compliance (OpenSCAP + CIS + Docker Bench)",
		starter: false,
		plus: false,
		max: true,
	},
	{
		label: "Direct support channels",
		starter: "Email, Discord",
		plus: "Email, Slack, Discord",
		max: "Email, Slack, Discord, Phone",
	},
];

export const getTier = (tierId) => TIERS[tierId] || null;

export const getNextTier = (tierId) => {
	const idx = TIER_ORDER.indexOf(tierId);
	if (idx < 0 || idx >= TIER_ORDER.length - 1) return null;
	return TIERS[TIER_ORDER[idx + 1]];
};

// Module → required tier mapping for feature-gating UI.
// MUST stay in sync with RequireModule(...) calls in
// server-source-code/internal/server/router.go. When a new gated module is
// added server-side, add it here and to MODULE_LABELS below.
export const MODULE_TIER_MAP = {
	patching: "plus",
	patching_policies: "plus",
	docker: "plus",
	alerts_advanced: "plus",
	rbac_custom: "plus",
	custom_branding: "plus",
	compliance: "max",
	ssh_terminal: "max",
	rdp: "max",
	ai: "max",
};

// Human-readable feature names for upgrade screens.
export const MODULE_LABELS = {
	patching: "Patching",
	patching_policies: "Patching Policies",
	docker: "Docker Monitoring",
	alerts_advanced: "Advanced Alerts",
	rbac_custom: "Custom RBAC Roles",
	custom_branding: "Custom Branding",
	compliance: "Compliance Scanning",
	ssh_terminal: "Browser SSH Terminal",
	rdp: "Browser RDP",
	ai: "AI Terminal Assistant",
};

// Rows from TIER_FEATURES that a given tier unlocks compared to the previous
// tier. Used by the upgrade screen to show "what you get" when upgrading to
// Plus or Max. Starter-exclusive rows are never shown as an upgrade benefit.
const TIER_UNLOCK_LABELS = {
	plus: [
		"Manual patch runs",
		"Patch scheduling policies + approval workflow",
		"Docker container monitoring",
		"Advanced alert config + custom rules",
		"Custom RBAC roles",
		"Audit log export",
		"Custom branding (logo/favicon)",
	],
	max: [
		"Browser SSH terminal",
		"Browser RDP (Guacamole)",
		"BYO-AI terminal assistant",
		"Compliance (OpenSCAP + CIS + Docker Bench)",
	],
};

export const getRequiredTier = (moduleKey) =>
	MODULE_TIER_MAP[moduleKey] ?? null;

export const getModuleLabel = (moduleKey) =>
	MODULE_LABELS[moduleKey] ?? moduleKey;

export const getTierUnlocks = (tierId) => TIER_UNLOCK_LABELS[tierId] ?? [];
