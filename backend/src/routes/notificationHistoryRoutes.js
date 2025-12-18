const express = require("express");
const { query, validationResult } = require("express-validator");
const { authenticateToken } = require("../middleware/auth");
const NotificationService = require("../services/NotificationService");

const router = express.Router();
const notificationService = new NotificationService();

/**
 * GET /api/v1/notifications/history
 * Get notification delivery history with optional filtering
 *
 * Query parameters:
 * - start_date: ISO date string for start of date range
 * - end_date: ISO date string for end of date range
 * - event_type: Filter by event type (package_update, security_update, host_status_change, agent_update)
 * - channel_id: Filter by notification channel ID
 * - status: Filter by delivery status (sent, failed)
 * - limit: Maximum number of results (default: 100, max: 1000)
 * - offset: Offset for pagination (default: 0)
 */
router.get(
	"/",
	authenticateToken,
	[
		query("start_date")
			.optional()
			.isISO8601()
			.withMessage("start_date must be a valid ISO 8601 date"),
		query("end_date")
			.optional()
			.isISO8601()
			.withMessage("end_date must be a valid ISO 8601 date"),
		query("event_type")
			.optional()
			.trim()
			.isIn([
				"package_update",
				"security_update",
				"host_status_change",
				"agent_update",
			])
			.withMessage(
				"event_type must be one of: package_update, security_update, host_status_change, agent_update",
			),
		query("channel_id")
			.optional()
			.trim()
			.isLength({ min: 1 })
			.withMessage("channel_id must be a non-empty string"),
		query("status")
			.optional()
			.trim()
			.isIn(["sent", "failed"])
			.withMessage("status must be either 'sent' or 'failed'"),
		query("limit")
			.optional()
			.isInt({ min: 1, max: 1000 })
			.withMessage("limit must be an integer between 1 and 1000"),
		query("offset")
			.optional()
			.isInt({ min: 0 })
			.withMessage("offset must be a non-negative integer"),
	],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const {
				start_date,
				end_date,
				event_type,
				channel_id,
				status,
				limit = 100,
				offset = 0,
			} = req.query;

			// Build filters object
			const filters = {};

			if (start_date) {
				filters.start_date = start_date;
			}

			if (end_date) {
				filters.end_date = end_date;
			}

			if (event_type) {
				filters.event_type = event_type;
			}

			if (channel_id) {
				filters.channel_id = channel_id;
			}

			if (status) {
				filters.status = status;
			}

			filters.limit = Number.parseInt(limit, 10);
			filters.offset = Number.parseInt(offset, 10);

			// Get history entries
			const history = await notificationService.getHistory(filters);

			// Get total count for pagination
			const totalCount = await notificationService.getHistoryCount({
				start_date,
				end_date,
				event_type,
				channel_id,
				status,
			});

			res.json({
				data: history,
				pagination: {
					total: totalCount,
					limit: filters.limit,
					offset: filters.offset,
					hasMore: filters.offset + filters.limit < totalCount,
				},
			});
		} catch (error) {
			console.error("Error fetching notification history:", error);
			res.status(500).json({ error: "Failed to fetch notification history" });
		}
	},
);

module.exports = router;
