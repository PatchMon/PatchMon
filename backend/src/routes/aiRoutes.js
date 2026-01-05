const express = require("express");
const { body, validationResult } = require("express-validator");
const logger = require("../utils/logger");
const { getPrismaClient } = require("../config/prisma");
const { authenticateToken } = require("../middleware/auth");
const { requireManageSettings } = require("../middleware/permissions");
const { encrypt, decrypt, isEncrypted } = require("../utils/encryption");
const { getProviders, getCompletion, getAssistance } = require("../services/aiService");

const router = express.Router();

// Rate limiting for AI endpoints
const requestCounts = new Map();
const RATE_LIMIT_WINDOW = 60 * 1000; // 1 minute
const RATE_LIMIT_MAX = 30; // 30 requests per minute

function checkRateLimit(userId) {
	const now = Date.now();
	const userKey = `ai_${userId}`;
	const userData = requestCounts.get(userKey) || { count: 0, windowStart: now };

	// Reset window if expired
	if (now - userData.windowStart > RATE_LIMIT_WINDOW) {
		userData.count = 0;
		userData.windowStart = now;
	}

	userData.count++;
	requestCounts.set(userKey, userData);

	return userData.count <= RATE_LIMIT_MAX;
}

// Clean up old rate limit entries periodically
setInterval(() => {
	const now = Date.now();
	for (const [key, data] of requestCounts.entries()) {
		if (now - data.windowStart > RATE_LIMIT_WINDOW * 2) {
			requestCounts.delete(key);
		}
	}
}, RATE_LIMIT_WINDOW);

/**
 * Get AI status (available to all authenticated users)
 * GET /api/v1/ai/status
 */
router.get("/status", authenticateToken, async (_req, res) => {
	try {
		const prisma = getPrismaClient();
		const settings = await prisma.settings.findFirst();

		res.json({
			ai_enabled: settings?.ai_enabled || false,
			ai_api_key_set: !!settings?.ai_api_key,
		});
	} catch (error) {
		logger.error("Error fetching AI status:", error);
		res.status(500).json({ error: "Failed to fetch AI status" });
	}
});

/**
 * Get available AI providers and their models
 * GET /api/v1/ai/providers
 */
router.get("/providers", authenticateToken, (_req, res) => {
	try {
		const providers = getProviders();
		res.json({ providers });
	} catch (error) {
		logger.error("Error fetching AI providers:", error);
		res.status(500).json({ error: "Failed to fetch AI providers" });
	}
});

/**
 * Get AI settings (admin only - includes full configuration)
 * GET /api/v1/ai/settings
 */
router.get("/settings", authenticateToken, requireManageSettings, async (_req, res) => {
	try {
		const prisma = getPrismaClient();
		const settings = await prisma.settings.findFirst();

		if (!settings) {
			return res.json({
				ai_enabled: false,
				ai_provider: "openrouter",
				ai_model: null,
				ai_api_key_set: false,
			});
		}

		res.json({
			ai_enabled: settings.ai_enabled || false,
			ai_provider: settings.ai_provider || "openrouter",
			ai_model: settings.ai_model || null,
			ai_api_key_set: !!settings.ai_api_key,
		});
	} catch (error) {
		logger.error("Error fetching AI settings:", error);
		res.status(500).json({ error: "Failed to fetch AI settings" });
	}
});

/**
 * Update AI settings
 * PUT /api/v1/ai/settings
 */
router.put(
	"/settings",
	authenticateToken,
	requireManageSettings,
	[
		body("ai_enabled").optional().isBoolean(),
		body("ai_provider").optional().isIn(["openrouter", "anthropic", "openai", "gemini"]),
		body("ai_model").optional().isString(),
		body("ai_api_key").optional().isString(),
	],
	async (req, res) => {
		const errors = validationResult(req);
		if (!errors.isEmpty()) {
			return res.status(400).json({ errors: errors.array() });
		}

		try {
			const prisma = getPrismaClient();
			const { ai_enabled, ai_provider, ai_model, ai_api_key } = req.body;

			const updateData = {
				updated_at: new Date(),
			};

			if (typeof ai_enabled === "boolean") {
				updateData.ai_enabled = ai_enabled;
			}

			if (ai_provider) {
				updateData.ai_provider = ai_provider;
			}

			if (ai_model !== undefined) {
				updateData.ai_model = ai_model;
			}

			// Encrypt API key if provided
			if (ai_api_key) {
				// Don't re-encrypt if already encrypted
				if (isEncrypted(ai_api_key)) {
					updateData.ai_api_key = ai_api_key;
				} else {
					updateData.ai_api_key = encrypt(ai_api_key);
				}
			}

			// Update or create settings
			let settings = await prisma.settings.findFirst();
			if (settings) {
				settings = await prisma.settings.update({
					where: { id: settings.id },
					data: updateData,
				});
			} else {
				settings = await prisma.settings.create({
					data: {
						id: crypto.randomUUID(),
						...updateData,
					},
				});
			}

			logger.info("AI settings updated");

			res.json({
				message: "AI settings updated successfully",
				ai_enabled: settings.ai_enabled,
				ai_provider: settings.ai_provider,
				ai_model: settings.ai_model,
				ai_api_key_set: !!settings.ai_api_key,
			});
		} catch (error) {
			logger.error("Error updating AI settings:", error);
			res.status(500).json({ error: "Failed to update AI settings" });
		}
	}
);

/**
 * Test AI connection
 * POST /api/v1/ai/test
 */
router.post("/test", authenticateToken, requireManageSettings, async (req, res) => {
	try {
		const prisma = getPrismaClient();
		const settings = await prisma.settings.findFirst();

		if (!settings?.ai_api_key) {
			return res.status(400).json({ error: "AI API key not configured" });
		}

		const response = await getAssistance(
			settings,
			"Respond with exactly: 'Connection successful!' - nothing else.",
			"",
			[]
		);

		if (response.toLowerCase().includes("connection successful")) {
			res.json({ success: true, message: "AI connection test successful" });
		} else {
			res.json({ success: true, message: "AI responded", response: response.substring(0, 100) });
		}
	} catch (error) {
		logger.error("AI connection test failed:", error);
		res.status(400).json({ error: `Connection test failed: ${error.message}` });
	}
});

/**
 * Get AI assistance for terminal
 * POST /api/v1/ai/assist
 */
router.post(
	"/assist",
	authenticateToken,
	[
		body("question").isString().isLength({ min: 1, max: 2000 }),
		body("context").optional().isString().isLength({ max: 10000 }),
		body("history").optional().isArray(),
	],
	async (req, res) => {
		const errors = validationResult(req);
		if (!errors.isEmpty()) {
			return res.status(400).json({ errors: errors.array() });
		}

		// Rate limit check
		if (!checkRateLimit(req.user.id)) {
			return res.status(429).json({ error: "Rate limit exceeded. Please wait a moment." });
		}

		try {
			const prisma = getPrismaClient();
			const settings = await prisma.settings.findFirst();

			if (!settings?.ai_enabled) {
				return res.status(400).json({ error: "AI assistant is not enabled" });
			}

			if (!settings?.ai_api_key) {
				return res.status(400).json({ error: "AI API key not configured" });
			}

			const { question, context, history } = req.body;

			// Sanitize history to prevent injection
			const sanitizedHistory = (history || [])
				.slice(-10) // Keep last 10 messages max
				.filter(m => m.role && m.content)
				.map(m => ({
					role: m.role === "assistant" ? "assistant" : "user",
					content: String(m.content).substring(0, 2000),
				}));

			const response = await getAssistance(settings, question, context, sanitizedHistory);

			res.json({ response });
		} catch (error) {
			logger.error("AI assistance error:", error);
			res.status(500).json({ error: `AI request failed: ${error.message}` });
		}
	}
);

/**
 * Get command completion suggestion
 * POST /api/v1/ai/complete
 */
router.post(
	"/complete",
	authenticateToken,
	[
		body("input").isString().isLength({ min: 2, max: 500 }),
		body("context").optional().isString().isLength({ max: 5000 }),
	],
	async (req, res) => {
		const errors = validationResult(req);
		if (!errors.isEmpty()) {
			return res.status(400).json({ errors: errors.array() });
		}

		// Rate limit check (stricter for completions)
		if (!checkRateLimit(req.user.id)) {
			return res.status(429).json({ error: "Rate limit exceeded" });
		}

		try {
			const prisma = getPrismaClient();
			const settings = await prisma.settings.findFirst();

			if (!settings?.ai_enabled) {
				return res.status(400).json({ error: "AI assistant is not enabled" });
			}

			if (!settings?.ai_api_key) {
				return res.status(400).json({ error: "AI API key not configured" });
			}

			const { input, context } = req.body;

			const completion = await getCompletion(settings, input, context);

			res.json({ completion });
		} catch (error) {
			logger.error("AI completion error:", error);
			res.status(500).json({ error: "Completion request failed" });
		}
	}
);

module.exports = router;
