const express = require("express");
const logger = require("../utils/logger");
const { body, validationResult } = require("express-validator");
const { getPrismaClient } = require("../config/prisma");
const { authenticateToken } = require("../middleware/auth");
const { v4: uuidv4 } = require("uuid");

const router = express.Router();
const prisma = getPrismaClient();

// Helper function to get user permissions based on role
async function getUserPermissions(userRole) {
	try {
		const permissions = await prisma.role_permissions.findUnique({
			where: { role: userRole },
		});

		// If no specific permissions found, return default admin permissions (for backward compatibility)
		if (!permissions) {
			logger.warn(
				`No permissions found for role: ${userRole}, defaulting to admin access`,
			);
			return {
				can_view_dashboard: true,
				can_view_hosts: true,
				can_manage_hosts: true,
				can_view_packages: true,
				can_manage_packages: true,
				can_view_users: true,
				can_manage_users: true,
				can_view_reports: true,
				can_export_data: true,
				can_manage_settings: true,
			};
		}

		return permissions;
	} catch (error) {
		logger.error("Error fetching user permissions:", error);
		// Return admin permissions as fallback
		return {
			can_view_dashboard: true,
			can_view_hosts: true,
			can_manage_hosts: true,
			can_view_packages: true,
			can_manage_packages: true,
			can_view_users: true,
			can_manage_users: true,
			can_view_reports: true,
			can_export_data: true,
			can_manage_settings: true,
		};
	}
}

// Curated default dashboard layout — defines the out-of-the-box experience for
// new users and users upgrading to 1.4.2+.  Each entry carries the card's
// permission gate, default order, whether it is enabled, and its column span.
// Layout mirrors the reference admin dashboard for a clean first-load experience.
const DEFAULT_CARD_LAYOUT = [
	// ── Stats row (6 columns) ────────────────────────────────────────────
	{
		cardId: "totalHosts",
		requiredPermission: "can_view_hosts",
		order: 0,
		enabled: true,
		col_span: 1,
	},
	{
		cardId: "upToDateHosts",
		requiredPermission: "can_view_hosts",
		order: 1,
		enabled: true,
		col_span: 1,
	},
	{
		cardId: "quickStats",
		requiredPermission: "can_view_dashboard",
		order: 2,
		enabled: true,
		col_span: 2,
	},
	{
		cardId: "hostsNeedingUpdates",
		requiredPermission: "can_view_hosts",
		order: 3,
		enabled: true,
		col_span: 1,
	},
	{
		cardId: "hostsNeedingReboot",
		requiredPermission: "can_view_hosts",
		order: 4,
		enabled: true,
		col_span: 1,
	},
	{
		cardId: "totalOutdatedPackages",
		requiredPermission: "can_view_packages",
		order: 5,
		enabled: true,
		col_span: 1,
	},
	{
		cardId: "securityUpdates",
		requiredPermission: "can_view_packages",
		order: 6,
		enabled: true,
		col_span: 1,
	},
	{
		cardId: "totalUsers",
		requiredPermission: "can_view_users",
		order: 7,
		enabled: true,
		col_span: 1,
	},
	{
		cardId: "totalHostGroups",
		requiredPermission: "can_view_hosts",
		order: 8,
		enabled: true,
		col_span: 1,
	},
	{
		cardId: "complianceStats",
		requiredPermission: "can_view_hosts",
		order: 9,
		enabled: true,
		col_span: 1,
	},
	{
		cardId: "totalRepos",
		requiredPermission: "can_view_hosts",
		order: 10,
		enabled: true,
		col_span: 1,
	},

	// ── Charts row (4 columns) ──────────────────────────────────────────
	{
		cardId: "updateStatus",
		requiredPermission: "can_view_reports",
		order: 11,
		enabled: true,
		col_span: 1,
	},
	{
		cardId: "osDistributionBar",
		requiredPermission: "can_view_reports",
		order: 12,
		enabled: true,
		col_span: 1,
	},
	{
		cardId: "packageTrends",
		requiredPermission: "can_view_packages",
		order: 13,
		enabled: true,
		col_span: 2,
	},
	{
		cardId: "complianceHostStatus",
		requiredPermission: "can_view_hosts",
		order: 14,
		enabled: true,
		col_span: 1,
	},
	{
		cardId: "complianceActiveBenchmarkScans",
		requiredPermission: "can_view_hosts",
		order: 15,
		enabled: true,
		col_span: 1,
	},
	{
		cardId: "recentCollection",
		requiredPermission: "can_view_hosts",
		order: 16,
		enabled: true,
		col_span: 1,
	},
	{
		cardId: "recentUsers",
		requiredPermission: "can_view_users",
		order: 17,
		enabled: true,
		col_span: 1,
	},
	{
		cardId: "complianceFailuresBySeverity",
		requiredPermission: "can_view_hosts",
		order: 18,
		enabled: true,
		col_span: 1,
	},
	{
		cardId: "osDistributionDoughnut",
		requiredPermission: "can_view_reports",
		order: 19,
		enabled: true,
		col_span: 1,
	},
	{
		cardId: "complianceOpenSCAPDistribution",
		requiredPermission: "can_view_hosts",
		order: 20,
		enabled: true,
		col_span: 2,
	},
	{
		cardId: "packagePriority",
		requiredPermission: "can_view_packages",
		order: 21,
		enabled: true,
		col_span: 1,
	},
	{
		cardId: "complianceProfilesInUse",
		requiredPermission: "can_view_hosts",
		order: 22,
		enabled: true,
		col_span: 1,
	},
	{
		cardId: "complianceLastScanAge",
		requiredPermission: "can_view_hosts",
		order: 23,
		enabled: true,
		col_span: 1,
	},
	{
		cardId: "osDistribution",
		requiredPermission: "can_view_reports",
		order: 24,
		enabled: true,
		col_span: 1,
	},

	// ── Disabled by default (available to enable) ────────────────────────
	{
		cardId: "complianceTrendLine",
		requiredPermission: "can_view_hosts",
		order: 25,
		enabled: false,
		col_span: 1,
	},
];

// Default grid column counts for the dashboard layout record
const DEFAULT_GRID_LAYOUT = {
	stats_columns: 6,
	charts_columns: 4,
};

// Helper function to create permission-based dashboard preferences for a new user
async function createDefaultDashboardPreferences(userId, userRole = "user") {
	try {
		const permissions = await getUserPermissions(userRole);

		// Filter cards based on user's permissions
		const allowedCards = DEFAULT_CARD_LAYOUT.filter((card) => {
			return permissions[card.requiredPermission] === true;
		});

		const now = new Date();

		// Create card preferences
		const preferencesData = allowedCards.map((card) => ({
			id: uuidv4(),
			user_id: userId,
			card_id: card.cardId,
			enabled: card.enabled,
			order: card.order,
			col_span: card.col_span,
			created_at: now,
			updated_at: now,
		}));

		await prisma.dashboard_preferences.createMany({
			data: preferencesData,
		});

		// Create default dashboard_layout record so new users start with the
		// curated grid column counts rather than relying on frontend fallbacks
		await prisma.dashboard_layout.upsert({
			where: { user_id: userId },
			create: {
				user_id: userId,
				stats_columns: DEFAULT_GRID_LAYOUT.stats_columns,
				charts_columns: DEFAULT_GRID_LAYOUT.charts_columns,
				updated_at: now,
			},
			update: {},
		});

		logger.info(
			`Dashboard defaults created for user ${userId} (role: ${userRole}): ${allowedCards.length} cards`,
		);
	} catch (error) {
		logger.error("Error creating default dashboard preferences:", error);
	}
}

// Get user's dashboard preferences
router.get("/", authenticateToken, async (req, res) => {
	try {
		const preferences = await prisma.dashboard_preferences.findMany({
			where: { user_id: req.user.id },
			orderBy: { order: "asc" },
		});

		res.json(preferences);
	} catch (error) {
		logger.error("Dashboard preferences fetch error:", error);
		res.status(500).json({ error: "Failed to fetch dashboard preferences" });
	}
});

// Update dashboard preferences (bulk update)
router.put(
	"/",
	authenticateToken,
	[
		body("preferences").isArray().withMessage("Preferences must be an array"),
		body("preferences.*.cardId").isString().withMessage("Card ID is required"),
		body("preferences.*.enabled")
			.isBoolean()
			.withMessage("Enabled must be boolean"),
		body("preferences.*.order").isInt().withMessage("Order must be integer"),
		body("preferences.*.col_span")
			.optional()
			.isInt({ min: 1, max: 3 })
			.withMessage("col_span must be 1, 2, or 3"),
	],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const { preferences } = req.body;
			const userId = req.user.id;

			// Delete existing preferences for this user
			await prisma.dashboard_preferences.deleteMany({
				where: { user_id: userId },
			});

			// Create new preferences (ensure order and col_span for persistence)
			const newPreferences = preferences.map((pref, index) => {
				const colSpan = pref.col_span ?? pref.colSpan;
				const span = Number(colSpan);
				const col_span =
					Number.isInteger(span) && span >= 1 && span <= 3 ? span : 1;
				return {
					id: uuidv4(),
					user_id: userId,
					card_id: pref.cardId,
					enabled: pref.enabled,
					order: Number.isInteger(Number(pref.order))
						? Number(pref.order)
						: index,
					col_span,
					updated_at: new Date(),
				};
			});

			await prisma.dashboard_preferences.createMany({
				data: newPreferences,
			});

			res.json({
				message: "Dashboard preferences updated successfully",
				preferences: newPreferences,
			});
		} catch (error) {
			logger.error("Dashboard preferences update error:", error);
			res.status(500).json({ error: "Failed to update dashboard preferences" });
		}
	},
);

// Alias for route-level usage (kept for backward compat references)
const DEFAULT_LAYOUT = DEFAULT_GRID_LAYOUT;

// Get user's dashboard row layout (column counts per row type)
router.get("/layout", authenticateToken, async (req, res) => {
	try {
		const layout = await prisma.dashboard_layout.findUnique({
			where: { user_id: req.user.id },
		});
		if (!layout) {
			return res.json({
				stats_columns: DEFAULT_LAYOUT.stats_columns,
				charts_columns: DEFAULT_LAYOUT.charts_columns,
			});
		}
		res.json({
			stats_columns: layout.stats_columns,
			charts_columns: layout.charts_columns,
		});
	} catch (error) {
		logger.error("Dashboard layout fetch error:", error);
		res.status(500).json({ error: "Failed to fetch dashboard layout" });
	}
});

// Update user's dashboard row layout
router.put(
	"/layout",
	authenticateToken,
	[
		body("stats_columns")
			.optional()
			.isInt({ min: 2, max: 6 })
			.withMessage("stats_columns must be between 2 and 6"),
		body("charts_columns")
			.optional()
			.isInt({ min: 2, max: 4 })
			.withMessage("charts_columns must be between 2 and 4"),
	],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}
			const userId = req.user.id;
			const stats_columns = Number(
				req.body.stats_columns ?? DEFAULT_LAYOUT.stats_columns,
			);
			const charts_columns = Number(
				req.body.charts_columns ?? DEFAULT_LAYOUT.charts_columns,
			);
			const now = new Date();
			await prisma.dashboard_layout.upsert({
				where: { user_id: userId },
				create: {
					user_id: userId,
					stats_columns,
					charts_columns,
					updated_at: now,
				},
				update: {
					stats_columns,
					charts_columns,
					updated_at: now,
				},
			});
			res.json({
				message: "Dashboard layout updated successfully",
				stats_columns,
				charts_columns,
			});
		} catch (error) {
			logger.error("Dashboard layout update error:", error);
			res.status(500).json({ error: "Failed to update dashboard layout" });
		}
	},
);

// Display metadata for the defaults endpoint (title + icon per card)
const CARD_METADATA = {
	totalHosts: { title: "Total Hosts", icon: "Server" },
	hostsNeedingUpdates: { title: "Needs Updating", icon: "AlertTriangle" },
	totalOutdatedPackages: { title: "Outdated Packages", icon: "Package" },
	securityUpdates: { title: "Security Updates", icon: "Shield" },
	upToDateHosts: { title: "Up to date", icon: "CheckCircle" },
	hostsNeedingReboot: { title: "Needs Reboots", icon: "RotateCcw" },
	totalHostGroups: { title: "Host Groups", icon: "Folder" },
	totalRepos: { title: "Repositories", icon: "GitBranch" },
	totalUsers: { title: "Users", icon: "Users" },
	complianceStats: { title: "Compliance", icon: "Shield" },
	quickStats: { title: "Quick Stats", icon: "TrendingUp" },
	osDistribution: { title: "OS Distribution", icon: "BarChart3" },
	osDistributionBar: { title: "OS Distribution (Bar)", icon: "BarChart3" },
	osDistributionDoughnut: {
		title: "OS Distribution (Doughnut)",
		icon: "PieChart",
	},
	recentCollection: { title: "Recent Collection", icon: "Server" },
	updateStatus: { title: "Update Status", icon: "BarChart3" },
	packagePriority: { title: "Package Priority", icon: "BarChart3" },
	packageTrends: { title: "Package Trends", icon: "TrendingUp" },
	recentUsers: { title: "Recent Users Logged in", icon: "Users" },
	complianceHostStatus: { title: "Host Compliance Status", icon: "BarChart3" },
	complianceOpenSCAPDistribution: {
		title: "OpenSCAP Distribution",
		icon: "PieChart",
	},
	complianceFailuresBySeverity: {
		title: "Failures by Severity",
		icon: "PieChart",
	},
	complianceProfilesInUse: {
		title: "Compliance Profiles in Use",
		icon: "PieChart",
	},
	complianceLastScanAge: { title: "Last Scan Age", icon: "BarChart3" },
	complianceTrendLine: { title: "Compliance Trend", icon: "TrendingUp" },
	complianceActiveBenchmarkScans: {
		title: "Active Benchmark Scans",
		icon: "Shield",
	},
};

// Get default dashboard card configuration (derived from DEFAULT_CARD_LAYOUT)
router.get("/defaults", authenticateToken, async (_req, res) => {
	try {
		const defaultCards = DEFAULT_CARD_LAYOUT.map((card) => {
			const meta = CARD_METADATA[card.cardId] || {
				title: card.cardId,
				icon: "BarChart3",
			};
			return {
				cardId: card.cardId,
				title: meta.title,
				icon: meta.icon,
				enabled: card.enabled,
				order: card.order,
				col_span: card.col_span,
			};
		});

		res.json(defaultCards);
	} catch (error) {
		logger.error("Default dashboard cards error:", error);
		res.status(500).json({ error: "Failed to fetch default dashboard cards" });
	}
});

module.exports = {
	router,
	createDefaultDashboardPreferences,
	DEFAULT_CARD_LAYOUT,
	DEFAULT_GRID_LAYOUT,
};
