const express = require("express");
const { body, validationResult } = require("express-validator");
const logger = require("../utils/logger");
const { getPrismaClient } = require("../config/prisma");
const { authenticateToken } = require("../middleware/auth");
const { requireManageSettings } = require("../middleware/permissions");
const { encrypt, decrypt, isEncrypted, getEncryptionStatus } = require("../utils/encryption");
const { getProviders, getCompletion, getAssistance } = require("../services/aiService");
const { redis } = require("../services/automation/shared/redis");

const router = express.Router();

// Rate limiting for AI endpoints using Redis (works across multiple instances)
const RATE_LIMIT_WINDOW = 60; // 1 minute in seconds
const RATE_LIMIT_MAX = 30; // 30 requests per minute

async function checkRateLimit(userId) {
	try {
		const key = `ratelimit:ai:${userId}`;
		const count = await redis.incr(key);

		// Set expiry only on first request in window
		if (count === 1) {
			await redis.expire(key, RATE_LIMIT_WINDOW);
		}

		return count <= RATE_LIMIT_MAX;
	} catch (error) {
		// If Redis is unavailable, allow the request (fail open)
		logger.warn("Rate limit check failed, allowing request:", error.message);
		return true;
	}
}

/**
 * Get AI status (available to all authenticated users)
 * GET /api/v1/ai/status
 */
router.get("/status", authenticateToken, async (_req, res) => {
	try {
		const prisma = getPrismaClient();
		const settings = await prisma.settings.findFirst();

		// Check if API key exists and can be decrypted
		let apiKeyValid = false;
		if (settings?.ai_api_key) {
			const decrypted = decrypt(settings.ai_api_key);
			apiKeyValid = !!decrypted;
			if (!decrypted && settings.ai_api_key) {
				logger.warn("AI API key exists but cannot be decrypted - encryption key may have changed");
			}
		}

		res.json({
			ai_enabled: settings?.ai_enabled || false,
			ai_api_key_set: apiKeyValid,
		});
	} catch (error) {
		logger.error("Error fetching AI status:", error);
		res.status(500).json({ error: "Failed to fetch AI status" });
	}
});

/**
 * Get encryption debug info (admin only)
 * GET /api/v1/ai/debug
 */
router.get("/debug", authenticateToken, requireManageSettings, async (_req, res) => {
	try {
		const prisma = getPrismaClient();
		const settings = await prisma.settings.findFirst();
		const encryptionStatus = getEncryptionStatus();

		// Test encryption round-trip
		const testValue = "test-" + Date.now();
		const encrypted = encrypt(testValue);
		const decrypted = decrypt(encrypted);
		const roundTripOk = decrypted === testValue;

		// Check if existing AI key can be decrypted
		let existingKeyStatus = "not_set";
		if (settings?.ai_api_key) {
			const decryptedKey = decrypt(settings.ai_api_key);
			existingKeyStatus = decryptedKey ? "valid" : "invalid_cannot_decrypt";
		}

		res.json({
			encryption: encryptionStatus,
			roundTripTest: roundTripOk ? "passed" : "failed",
			existingApiKey: existingKeyStatus,
			recommendation: encryptionStatus.source === "ephemeral"
				? "CRITICAL: Using ephemeral key - encrypted data will be lost on restart. Set SESSION_SECRET environment variable."
				: encryptionStatus.source === "file"
				? "Set SESSION_SECRET environment variable for consistent encryption across restarts"
				: "Configuration looks good",
		});
	} catch (error) {
		logger.error("Error fetching AI debug info:", error);
		res.status(500).json({ error: "Failed to fetch debug info" });
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
				ai_api_key_invalid: false,
			});
		}

		// Check if API key exists and can be decrypted
		let apiKeyValid = false;
		let apiKeyInvalid = false;
		if (settings.ai_api_key) {
			const decrypted = decrypt(settings.ai_api_key);
			apiKeyValid = !!decrypted;
			apiKeyInvalid = !decrypted; // Key exists but can't be decrypted
			if (apiKeyInvalid) {
				logger.warn("AI API key cannot be decrypted - SESSION_SECRET or AI_ENCRYPTION_KEY may have changed. Please re-enter the API key.");
			}
		}

		res.json({
			ai_enabled: settings.ai_enabled || false,
			ai_provider: settings.ai_provider || "openrouter",
			ai_model: settings.ai_model || null,
			ai_api_key_set: apiKeyValid,
			ai_api_key_invalid: apiKeyInvalid, // True if key exists but can't be decrypted
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
		if (!(await checkRateLimit(req.user.id))) {
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
		if (!(await checkRateLimit(req.user.id))) {
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
