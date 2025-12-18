const express = require("express");
const { body, validationResult } = require("express-validator");
const { authenticateToken } = require("../middleware/auth");
const { requireManageSettings } = require("../middleware/permissions");
const NotificationChannelService = require("../services/NotificationChannelService");

const router = express.Router();
const channelService = new NotificationChannelService();

/**
 * GET /api/v1/notifications/channels
 * Get all notification channels
 */
router.get("/", authenticateToken, async (_req, res) => {
	try {
		const channels = await channelService.getChannels();
		res.json(channels);
	} catch (error) {
		console.error("Error fetching notification channels:", error);
		res.status(500).json({ error: "Failed to fetch notification channels" });
	}
});

/**
 * GET /api/v1/notifications/channels/:id
 * Get a specific notification channel by ID
 */
router.get("/:id", authenticateToken, async (req, res) => {
	try {
		const { id } = req.params;
		const channels = await channelService.getChannels();
		const channel = channels.find((c) => c.id === id);

		if (!channel) {
			return res.status(404).json({ error: "Notification channel not found" });
		}

		res.json(channel);
	} catch (error) {
		console.error("Error fetching notification channel:", error);
		res.status(500).json({ error: "Failed to fetch notification channel" });
	}
});

/**
 * POST /api/v1/notifications/channels
 * Create a new notification channel
 */
router.post(
	"/",
	authenticateToken,
	requireManageSettings,
	[
		body("name")
			.trim()
			.isLength({ min: 1 })
			.withMessage("Channel name is required"),
		body("server_url")
			.trim()
			.isURL()
			.withMessage("Server URL must be a valid URL"),
		body("token").trim().isLength({ min: 1 }).withMessage("Token is required"),
		body("priority")
			.optional()
			.isInt({ min: 0, max: 10 })
			.withMessage("Priority must be an integer between 0 and 10"),
	],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const { name, server_url, token, priority } = req.body;

			const channel = await channelService.createChannel({
				name,
				server_url,
				token,
				priority,
				created_by_user_id: req.user.id,
			});

			res.status(201).json(channel);
		} catch (error) {
			console.error("Error creating notification channel:", error);
			res.status(400).json({ error: error.message });
		}
	},
);

/**
 * PUT /api/v1/notifications/channels/:id
 * Update an existing notification channel
 */
router.put(
	"/:id",
	authenticateToken,
	requireManageSettings,
	[
		body("name")
			.optional()
			.trim()
			.isLength({ min: 1 })
			.withMessage("Channel name must be non-empty"),
		body("server_url")
			.optional()
			.trim()
			.isURL()
			.withMessage("Server URL must be a valid URL"),
		body("token").optional().trim(),
		body("priority")
			.optional()
			.isInt({ min: 0, max: 10 })
			.withMessage("Priority must be an integer between 0 and 10"),
	],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const { id } = req.params;
			const { name, server_url, token, priority } = req.body;

			const channel = await channelService.updateChannel(id, {
				name,
				server_url,
				token,
				priority,
			});

			res.json(channel);
		} catch (error) {
			console.error("Error updating notification channel:", error);
			if (error.message.includes("not found")) {
				return res.status(404).json({ error: error.message });
			}
			res.status(400).json({ error: error.message });
		}
	},
);

/**
 * DELETE /api/v1/notifications/channels/:id
 * Delete a notification channel
 */
router.delete(
	"/:id",
	authenticateToken,
	requireManageSettings,
	async (req, res) => {
		try {
			const { id } = req.params;

			const channel = await channelService.deleteChannel(id);

			res.json({
				message: "Notification channel deleted successfully",
				channel,
			});
		} catch (error) {
			console.error("Error deleting notification channel:", error);
			if (error.message.includes("not found")) {
				return res.status(404).json({ error: error.message });
			}
			res.status(500).json({ error: "Failed to delete notification channel" });
		}
	},
);

/**
 * POST /api/v1/notifications/channels/:id/test
 * Test a notification channel connection
 */
router.post(
	"/:id/test",
	authenticateToken,
	requireManageSettings,
	async (req, res) => {
		try {
			const { id } = req.params;

			const result = await channelService.testChannel(id);

			if (result.success) {
				res.json(result);
			} else {
				res.status(400).json(result);
			}
		} catch (error) {
			console.error("Error testing notification channel:", error);
			if (error.message.includes("not found")) {
				return res.status(404).json({ error: error.message });
			}
			res.status(500).json({ error: error.message });
		}
	},
);

module.exports = router;
