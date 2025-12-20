const express = require("express");
const { body, validationResult } = require("express-validator");
const { authenticateToken } = require("../middleware/auth");
const { requireManageSettings } = require("../middleware/permissions");
const NotificationRuleService = require("../services/NotificationRuleService");

const router = express.Router();
const ruleService = new NotificationRuleService();

/**
 * GET /api/v1/notifications/rules
 * Get all notification rules
 */
router.get("/", authenticateToken, async (_req, res) => {
	try {
		const rules = await ruleService.getRules();
		res.json(rules);
	} catch (error) {
		console.error("Error fetching notification rules:", error);
		res.status(500).json({ error: "Failed to fetch notification rules" });
	}
});

/**
 * GET /api/v1/notifications/rules/:id
 * Get a specific notification rule by ID
 */
router.get("/:id", authenticateToken, async (req, res) => {
	try {
		const { id } = req.params;
		const rules = await ruleService.getRules();
		const rule = rules.find((r) => r.id === id);

		if (!rule) {
			return res.status(404).json({ error: "Notification rule not found" });
		}

		res.json(rule);
	} catch (error) {
		console.error("Error fetching notification rule:", error);
		res.status(500).json({ error: "Failed to fetch notification rule" });
	}
});

/**
 * POST /api/v1/notifications/rules
 * Create a new notification rule
 */
router.post(
	"/",
	authenticateToken,
	requireManageSettings,
	[
		body("name")
			.trim()
			.isLength({ min: 1 })
			.withMessage("Rule name is required"),
		body("event_type")
			.trim()
			.isLength({ min: 1 })
			.withMessage("Event type is required"),
		body("channel_ids")
			.isArray({ min: 1 })
			.withMessage("At least one channel ID is required"),
		body("priority")
			.optional()
			.isInt({ min: 0, max: 10 })
			.withMessage("Priority must be an integer between 0 and 10"),
		body("message_title").optional().trim(),
		body("message_template").optional().trim(),
		body("description").optional().trim(),
		body("filters")
			.optional()
			.isArray()
			.withMessage("Filters must be an array"),
	],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const {
				name,
				event_type,
				channel_ids,
				priority,
				message_title,
				message_template,
				description,
				filters,
			} = req.body;

			const rule = await ruleService.createRule({
				name,
				event_type,
				channel_ids,
				priority,
				message_title,
				message_template,
				description,
				filters,
				created_by_user_id: req.user.id,
			});

			res.status(201).json(rule);
		} catch (error) {
			console.error("Error creating notification rule:", error);
			res.status(400).json({ error: error.message });
		}
	},
);

/**
 * PUT /api/v1/notifications/rules/:id
 * Update an existing notification rule
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
			.withMessage("Rule name must be non-empty"),
		body("event_type").optional().trim(),
		body("channel_ids")
			.optional()
			.isArray({ min: 1 })
			.withMessage("At least one channel ID is required"),
		body("priority")
			.optional()
			.isInt({ min: 0, max: 10 })
			.withMessage("Priority must be an integer between 0 and 10"),
		body("message_title").optional().trim(),
		body("message_template").optional().trim(),
		body("description").optional().trim(),
		body("filters")
			.optional()
			.isArray()
			.withMessage("Filters must be an array"),
	],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const { id } = req.params;
			const {
				name,
				event_type,
				channel_ids,
				priority,
				message_title,
				message_template,
				description,
				filters,
			} = req.body;

			const rule = await ruleService.updateRule(id, {
				name,
				event_type,
				channel_ids,
				priority,
				message_title,
				message_template,
				description,
				filters,
			});

			res.json(rule);
		} catch (error) {
			console.error("Error updating notification rule:", error);
			if (error.message.includes("not found")) {
				return res.status(404).json({ error: error.message });
			}
			res.status(400).json({ error: error.message });
		}
	},
);

/**
 * DELETE /api/v1/notifications/rules/:id
 * Delete a notification rule
 */
router.delete(
	"/:id",
	authenticateToken,
	requireManageSettings,
	async (req, res) => {
		try {
			const { id } = req.params;

			const rule = await ruleService.deleteRule(id);

			res.json({ message: "Notification rule deleted successfully", rule });
		} catch (error) {
			console.error("Error deleting notification rule:", error);
			if (error.message.includes("not found")) {
				return res.status(404).json({ error: error.message });
			}
			res.status(500).json({ error: "Failed to delete notification rule" });
		}
	},
);

/**
 * PATCH /api/v1/notifications/rules/:id/toggle
 * Toggle a notification rule's enabled/disabled state
 */
router.patch(
	"/:id/toggle",
	authenticateToken,
	requireManageSettings,
	async (req, res) => {
		try {
			const { id } = req.params;

			const rule = await ruleService.toggleRule(id);

			res.json({
				message: "Notification rule toggled successfully",
				rule,
			});
		} catch (error) {
			console.error("Error toggling notification rule:", error);
			if (error.message.includes("not found")) {
				return res.status(404).json({ error: error.message });
			}
			res.status(500).json({ error: "Failed to toggle notification rule" });
		}
	},
);

module.exports = router;
