const express = require("express");
const { authenticateToken } = require("../middleware/auth");
const { getPrismaClient } = require("../config/prisma");

const prisma = getPrismaClient();
const router = express.Router();

// Mark release notes as accepted for current user
router.post("/accept", authenticateToken, async (req, res) => {
	try {
		const userId = req.user.id;
		const { version } = req.body;

		if (!version) {
			return res.status(400).json({ error: "Version is required" });
		}

		// Check if the model exists (Prisma client might not be regenerated yet)
		if (!prisma.release_notes_acceptances) {
			console.warn(
				"release_notes_acceptances model not available - Prisma client may need regeneration",
			);
			return res.status(503).json({
				error:
					"Release notes acceptance feature not available. Please regenerate Prisma client.",
			});
		}

		await prisma.release_notes_acceptances.upsert({
			where: {
				user_id_version: {
					user_id: userId,
					version: version,
				},
			},
			update: {
				accepted_at: new Date(),
			},
			create: {
				user_id: userId,
				version: version,
				accepted_at: new Date(),
			},
		});

		res.json({ success: true });
	} catch (error) {
		console.error("Error accepting release notes:", error);
		res.status(500).json({ error: "Failed to accept release notes" });
	}
});

module.exports = router;
