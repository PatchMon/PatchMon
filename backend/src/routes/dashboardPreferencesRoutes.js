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

// Helper function to create permission-based dashboard preferences for a new user
async function createDefaultDashboardPreferences(userId, userRole = "user") {
	try {
		// Get user's actual permissions
		const permissions = await getUserPermissions(userRole);

		// Define all possible dashboard cards with their required permissions
		// Order aligned with preferred layout
		const allCards = [
			// Host-related cards
			{ cardId: "totalHosts", requiredPermission: "can_view_hosts", order: 0 },
			{
				cardId: "hostsNeedingUpdates",
				requiredPermission: "can_view_hosts",
				order: 1,
			},

			// Package-related cards
			{
				cardId: "totalOutdatedPackages",
				requiredPermission: "can_view_packages",
				order: 2,
			},
			{
				cardId: "securityUpdates",
				requiredPermission: "can_view_packages",
				order: 3,
			},

			// Host-related cards (continued)
			{
				cardId: "totalHostGroups",
				requiredPermission: "can_view_hosts",
				order: 4,
			},
			{
				cardId: "upToDateHosts",
				requiredPermission: "can_view_hosts",
				order: 5,
			},
			{
				cardId: "hostsNeedingReboot",
				requiredPermission: "can_view_hosts",
				order: 6,
			},

			// Repository-related cards
			{ cardId: "totalRepos", requiredPermission: "can_view_hosts", order: 7 },

			// User management cards (admin only)
			{ cardId: "totalUsers", requiredPermission: "can_view_users", order: 8 },

			// System/Report cards
			{
				cardId: "osDistribution",
				requiredPermission: "can_view_reports",
				order: 9,
			},
			{
				cardId: "osDistributionBar",
				requiredPermission: "can_view_reports",
				order: 10,
			},
			{
				cardId: "osDistributionDoughnut",
				requiredPermission: "can_view_reports",
				order: 11,
			},
			{
				cardId: "recentCollection",
				requiredPermission: "can_view_hosts",
				order: 12,
			},
			{
				cardId: "updateStatus",
				requiredPermission: "can_view_reports",
				order: 13,
			},
			{
				cardId: "packagePriority",
				requiredPermission: "can_view_packages",
				order: 14,
			},
			{
				cardId: "packageTrends",
				requiredPermission: "can_view_packages",
				order: 15,
			},
			{
				cardId: "recentUsers",
				requiredPermission: "can_view_users",
				order: 16,
			},
			{
				cardId: "quickStats",
				requiredPermission: "can_view_dashboard",
				order: 17,
			},
			{
				cardId: "complianceHostStatus",
				requiredPermission: "can_view_hosts",
				order: 18,
			},
			{
				cardId: "complianceOpenSCAPDistribution",
				requiredPermission: "can_view_hosts",
				order: 19,
			},
			{
				cardId: "complianceFailuresBySeverity",
				requiredPermission: "can_view_hosts",
				order: 20,
			},
			{
				cardId: "complianceProfilesInUse",
				requiredPermission: "can_view_hosts",
				order: 21,
			},
			{
				cardId: "complianceLastScanAge",
				requiredPermission: "can_view_hosts",
				order: 22,
			},
			{
				cardId: "complianceTrendLine",
				requiredPermission: "can_view_hosts",
				order: 23,
			},
		];

		// Filter cards based on user's permissions
		const allowedCards = allCards.filter((card) => {
			return permissions[card.requiredPermission] === true;
		});

		// Create preferences data
		const preferencesData = allowedCards.map((card) => ({
			id: uuidv4(),
			user_id: userId,
			card_id: card.cardId,
			enabled: true,
			order: card.order, // Preserve original order from allCards
			created_at: new Date(),
			updated_at: new Date(),
		}));

		await prisma.dashboard_preferences.createMany({
			data: preferencesData,
		});

		logger.info(
			`Permission-based dashboard preferences created for user ${userId} with role ${userRole}: ${allowedCards.length} cards`,
		);
	} catch (error) {
		logger.error("Error creating default dashboard preferences:", error);
		// Don't throw error - this shouldn't break user creation
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

			// Create new preferences
			const newPreferences = preferences.map((pref) => ({
				id: require("uuid").v4(),
				user_id: userId,
				card_id: pref.cardId,
				enabled: pref.enabled,
				order: pref.order,
				updated_at: new Date(),
			}));

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

// Get default dashboard card configuration
router.get("/defaults", authenticateToken, async (_req, res) => {
	try {
		// This provides a comprehensive dashboard view for all new users
		const defaultCards = [
			{
				cardId: "totalHosts",
				title: "Total Hosts",
				icon: "Server",
				enabled: true,
				order: 0,
			},
			{
				cardId: "hostsNeedingUpdates",
				title: "Needs Updating",
				icon: "AlertTriangle",
				enabled: true,
				order: 1,
			},
			{
				cardId: "totalOutdatedPackages",
				title: "Outdated Packages",
				icon: "Package",
				enabled: true,
				order: 2,
			},
			{
				cardId: "securityUpdates",
				title: "Security Updates",
				icon: "Shield",
				enabled: true,
				order: 3,
			},
			{
				cardId: "totalHostGroups",
				title: "Host Groups",
				icon: "Folder",
				enabled: true,
				order: 4,
			},
			{
				cardId: "upToDateHosts",
				title: "Up to date",
				icon: "CheckCircle",
				enabled: true,
				order: 5,
			},
			{
				cardId: "hostsNeedingReboot",
				title: "Needs Reboots",
				icon: "RotateCcw",
				enabled: true,
				order: 6,
			},
			{
				cardId: "totalRepos",
				title: "Repositories",
				icon: "GitBranch",
				enabled: true,
				order: 7,
			},
			{
				cardId: "totalUsers",
				title: "Users",
				icon: "Users",
				enabled: true,
				order: 8,
			},
			{
				cardId: "osDistribution",
				title: "OS Distribution",
				icon: "BarChart3",
				enabled: true,
				order: 9,
			},
			{
				cardId: "osDistributionBar",
				title: "OS Distribution (Bar)",
				icon: "BarChart3",
				enabled: true,
				order: 9,
			},
			{
				cardId: "osDistributionDoughnut",
				title: "OS Distribution (Doughnut)",
				icon: "PieChart",
				enabled: true,
				order: 10,
			},
			{
				cardId: "recentCollection",
				title: "Recent Collection",
				icon: "Server",
				enabled: true,
				order: 11,
			},
			{
				cardId: "updateStatus",
				title: "Update Status",
				icon: "BarChart3",
				enabled: true,
				order: 12,
			},
			{
				cardId: "packagePriority",
				title: "Package Priority",
				icon: "BarChart3",
				enabled: true,
				order: 13,
			},
			{
				cardId: "packageTrends",
				title: "Package Trends",
				icon: "TrendingUp",
				enabled: true,
				order: 14,
			},
			{
				cardId: "recentUsers",
				title: "Recent Users Logged in",
				icon: "Users",
				enabled: true,
				order: 15,
			},
			{
				cardId: "quickStats",
				title: "Quick Stats",
				icon: "TrendingUp",
				enabled: true,
				order: 16,
			},
			{
				cardId: "complianceHostStatus",
				title: "Host Compliance Status",
				icon: "BarChart3",
				enabled: true,
				order: 17,
			},
			{
				cardId: "complianceOpenSCAPDistribution",
				title: "OpenSCAP Distribution",
				icon: "PieChart",
				enabled: true,
				order: 18,
			},
			{
				cardId: "complianceFailuresBySeverity",
				title: "Failures by Severity",
				icon: "PieChart",
				enabled: true,
				order: 19,
			},
			{
				cardId: "complianceProfilesInUse",
				title: "Compliance Profiles in Use",
				icon: "PieChart",
				enabled: true,
				order: 20,
			},
			{
				cardId: "complianceLastScanAge",
				title: "Last Scan Age",
				icon: "BarChart3",
				enabled: true,
				order: 21,
			},
			{
				cardId: "complianceTrendLine",
				title: "Compliance Trend",
				icon: "TrendingUp",
				enabled: true,
				order: 22,
			},
		];

		res.json(defaultCards);
	} catch (error) {
		logger.error("Default dashboard cards error:", error);
		res.status(500).json({ error: "Failed to fetch default dashboard cards" });
	}
});

module.exports = { router, createDefaultDashboardPreferences };
