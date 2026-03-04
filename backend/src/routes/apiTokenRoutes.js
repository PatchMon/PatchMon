const express = require("express");
const crypto = require("node:crypto");
const { body, validationResult } = require("express-validator");
const { getPrismaClient } = require("../config/prisma");
const logger = require("../utils/logger");
const { authenticateToken } = require("../middleware/auth");

const router = express.Router();
const prisma = getPrismaClient();

// Generate a new API token string
const generateApiToken = () => {
	return `patchmon_at_${crypto.randomBytes(32).toString("hex")}`;
};

// Hash a token for storage
const hashToken = (token) => {
	return crypto.createHash("sha256").update(token).digest("hex");
};

// GET /api/v1/api-tokens — list all tokens for the current user
router.get("/", authenticateToken, async (req, res) => {
	try {
		const tokens = await prisma.api_tokens.findMany({
			where: { user_id: req.user.id },
			orderBy: { created_at: "desc" },
			select: {
				id: true,
				name: true,
				created_at: true,
				expires_at: true,
				last_used_at: true,
			},
		});
		res.json(tokens);
	} catch (error) {
		logger.error("Error listing API tokens:", error);
		res.status(500).json({ error: "Failed to list API tokens" });
	}
});

// POST /api/v1/api-tokens — create a new token
router.post(
	"/",
	authenticateToken,
	[
		body("name")
			.trim()
			.isLength({ min: 1, max: 100 })
			.withMessage("Name is required (max 100 chars)"),
		body("expires_at")
			.optional({ nullable: true })
			.isISO8601()
			.withMessage("expires_at must be a valid ISO 8601 date"),
	],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const { name, expires_at } = req.body;

			const rawToken = generateApiToken();
			const tokenHash = hashToken(rawToken);

			const record = await prisma.api_tokens.create({
				data: {
					name,
					token_hash: tokenHash,
					user_id: req.user.id,
					expires_at: expires_at ? new Date(expires_at) : null,
				},
				select: {
					id: true,
					name: true,
					created_at: true,
					expires_at: true,
					last_used_at: true,
				},
			});

			// Return the raw token ONCE — it is never retrievable again
			res.status(201).json({ ...record, token: rawToken });
		} catch (error) {
			logger.error("Error creating API token:", error);
			res.status(500).json({ error: "Failed to create API token" });
		}
	},
);

// DELETE /api/v1/api-tokens/:id — revoke a token
router.delete("/:id", authenticateToken, async (req, res) => {
	try {
		const { id } = req.params;

		const record = await prisma.api_tokens.findUnique({
			where: { id },
			select: { user_id: true },
		});

		if (!record) {
			return res.status(404).json({ error: "API token not found" });
		}

		// Only the owning user (or an admin) may revoke a token
		if (record.user_id !== req.user.id && req.user.role !== "admin") {
			return res.status(403).json({ error: "Forbidden" });
		}

		await prisma.api_tokens.delete({ where: { id } });

		res.json({ message: "API token revoked" });
	} catch (error) {
		logger.error("Error revoking API token:", error);
		res.status(500).json({ error: "Failed to revoke API token" });
	}
});

module.exports = router;
