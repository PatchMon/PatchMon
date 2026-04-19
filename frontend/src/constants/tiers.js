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
