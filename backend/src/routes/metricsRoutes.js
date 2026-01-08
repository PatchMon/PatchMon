const express = require("express");
const logger = require("../utils/logger");
const { body, validationResult } = require("express-validator");
const { v4: uuidv4 } = require("uuid");
const { authenticateToken } = require("../middleware/auth");
const { requireManageSettings } = require("../middleware/permissions");
const { getSettings, updateSettings } = require("../services/settingsService");
const { queueManager, QUEUE_NAMES } = require("../services/automation");

const router = express.Router();

// Get metrics settings
router.get("/", authenticateToken, requireManageSettings, async (_req, res) => {
	try {
		const settings = await getSettings();

		// Generate anonymous ID if it doesn't exist
		if (!settings.metrics_anonymous_id) {
			const anonymousId = uuidv4();
			await updateSettings(settings.id, {
				metrics_anonymous_id: anonymousId,
			});
			settings.metrics_anonymous_id = anonymousId;
		}

		res.json({
			metrics_enabled: settings.metrics_enabled ?? true,
			metrics_anonymous_id: settings.metrics_anonymous_id,
			metrics_last_sent: settings.metrics_last_sent,
		});
	} catch (error) {
		logger.error("Metrics settings fetch error:", error);
		res.status(500).json({ error: "Failed to fetch metrics settings" });
	}
});

// Update metrics settings
router.put(
	"/",
	authenticateToken,
	requireManageSettings,
	[
		body("metrics_enabled")
			.isBoolean()
			.withMessage("Metrics enabled must be a boolean"),
	],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const { metrics_enabled } = req.body;
			const settings = await getSettings();

			await updateSettings(settings.id, {
				metrics_enabled,
			});

			logger.info(
				`Metrics ${metrics_enabled ? "enabled" : "disabled"} by user`,
			);

			res.json({
				message: "Metrics settings updated successfully",
				metrics_enabled,
			});
		} catch (error) {
			logger.error("Metrics settings update error:", error);
			res.status(500).json({ error: "Failed to update metrics settings" });
		}
	},
);

// Regenerate anonymous ID
router.post(
	"/regenerate-id",
	authenticateToken,
	requireManageSettings,
	async (_req, res) => {
		try {
			const settings = await getSettings();
			const newAnonymousId = uuidv4();

			await updateSettings(settings.id, {
				metrics_anonymous_id: newAnonymousId,
			});

			logger.info("Anonymous ID regenerated");

			res.json({
				message: "Anonymous ID regenerated successfully",
				metrics_anonymous_id: newAnonymousId,
			});
		} catch (error) {
			logger.error("Anonymous ID regeneration error:", error);
			res.status(500).json({ error: "Failed to regenerate anonymous ID" });
		}
	},
);

// Manually send metrics now
router.post(
	"/send-now",
	authenticateToken,
	requireManageSettings,
	async (_req, res) => {
		try {
			const settings = await getSettings();

			if (!settings.metrics_enabled) {
				return res.status(400).json({
					error: "Metrics are disabled. Please enable metrics first.",
				});
			}

			// Trigger metrics directly (no queue delay for manual trigger)
			const metricsReporting =
				queueManager.automations[QUEUE_NAMES.METRICS_REPORTING];
			const result = await metricsReporting.process(
				{ name: "manual-send" },
				false,
			);

			if (result.success) {
				logger.info("✅ Manual metrics sent successfully");
				res.json({
					message: "Metrics sent successfully",
					data: result,
				});
			} else {
				logger.error("❌ Failed to send metrics:", result);
				res.status(500).json({
					error: "Failed to send metrics",
					details: result.reason || result.error,
				});
			}
		} catch (error) {
			logger.error("Send metrics error:", error);
			res.status(500).json({
				error: "Failed to send metrics",
				details: error.message,
			});
		}
	},
);

module.exports = router;
