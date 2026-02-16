const express = require("express");
const logger = require("../utils/logger");
const { getPrismaClient } = require("../config/prisma");
const { authenticateToken } = require("../middleware/auth");

const router = express.Router();
const prisma = getPrismaClient();

/**
 * GET /api/v1/user/preferences
 * Get current user's preferences (theme and color theme)
 */
router.get("/", authenticateToken, async (req, res) => {
	try {
		const userId = req.user.id;

		const user = await prisma.users.findUnique({
			where: { id: userId },
			select: {
				theme_preference: true,
				color_theme: true,
				ui_preferences: true,
			},
		});

		if (!user) {
			return res.status(404).json({ error: "User not found" });
		}

		const ui_preferences = user.ui_preferences || {};
		res.json({
			theme_preference: user.theme_preference || "dark",
			color_theme: user.color_theme || "cyber_blue",
			ui_preferences,
			hosts_column_config: ui_preferences.hosts_column_config ?? null,
		});
	} catch (error) {
		logger.error("Error fetching user preferences:", error);
		res.status(500).json({ error: "Failed to fetch user preferences" });
	}
});

/**
 * PATCH /api/v1/user/preferences
 * Update current user's preferences
 */
function is_valid_hosts_column_config(value) {
	if (!Array.isArray(value)) return false;
	const allowed_ids = new Set([
		"select",
		"host",
		"hostname",
		"ip",
		"group",
		"os",
		"os_version",
		"agent_version",
		"auto_update",
		"ws_status",
		"integrations",
		"status",
		"needs_reboot",
		"uptime",
		"updates",
		"security_updates",
		"notes",
		"last_update",
		"actions",
	]);
	for (const col of value) {
		if (!col || typeof col.id !== "string" || !allowed_ids.has(col.id))
			return false;
		if (typeof col.visible !== "boolean") return false;
		if (typeof col.order !== "number" || col.order < 0) return false;
	}
	return true;
}

router.patch("/", authenticateToken, async (req, res) => {
	try {
		const userId = req.user.id;
		const { theme_preference, color_theme, hosts_column_config } = req.body;

		// Validate inputs
		const updateData = {};
		if (theme_preference !== undefined) {
			if (!["light", "dark"].includes(theme_preference)) {
				return res.status(400).json({
					error: "Invalid theme preference. Must be 'light' or 'dark'",
				});
			}
			updateData.theme_preference = theme_preference;
		}

		if (color_theme !== undefined) {
			const validColorThemes = [
				"default",
				"cyber_blue",
				"neon_purple",
				"matrix_green",
				"ocean_blue",
				"sunset_gradient",
			];
			if (!validColorThemes.includes(color_theme)) {
				return res.status(400).json({
					error: `Invalid color theme. Must be one of: ${validColorThemes.join(", ")}`,
				});
			}
			updateData.color_theme = color_theme;
		}

		if (hosts_column_config !== undefined) {
			if (!is_valid_hosts_column_config(hosts_column_config)) {
				return res.status(400).json({
					error:
						"Invalid hosts_column_config: must be an array of { id, visible, order } for allowed column ids",
				});
			}
			const user = await prisma.users.findUnique({
				where: { id: userId },
				select: { ui_preferences: true },
			});
			const current_ui =
				user?.ui_preferences && typeof user.ui_preferences === "object"
					? user.ui_preferences
					: {};
			updateData.ui_preferences = {
				...current_ui,
				hosts_column_config,
			};
		}

		if (Object.keys(updateData).length === 0) {
			return res
				.status(400)
				.json({ error: "No preferences provided to update" });
		}

		updateData.updated_at = new Date();

		const updatedUser = await prisma.users.update({
			where: { id: userId },
			data: updateData,
			select: {
				theme_preference: true,
				color_theme: true,
				ui_preferences: true,
			},
		});

		const ui_preferences = updatedUser.ui_preferences || {};
		res.json({
			message: "Preferences updated successfully",
			preferences: {
				theme_preference: updatedUser.theme_preference,
				color_theme: updatedUser.color_theme,
				ui_preferences,
				hosts_column_config: ui_preferences.hosts_column_config ?? null,
			},
		});
	} catch (error) {
		logger.error("Error updating user preferences:", error);
		res.status(500).json({ error: "Failed to update user preferences" });
	}
});

module.exports = router;
