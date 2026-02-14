const express = require("express");
const logger = require("../utils/logger");

const router = express.Router();

// Get social media statistics (hard-coded values)
router.get("/", async (_req, res) => {
	try {
		res.json({
			github_stars: 2100, // 2.1K
			discord_members: 500,
			buymeacoffee_supporters: 60,
			youtube_subscribers: 100,
			linkedin_followers: 250,
			last_updated: new Date(),
		});
	} catch (error) {
		logger.error("Error fetching social media stats:", error);
		res.status(500).json({
			error: "Failed to fetch social media statistics",
		});
	}
});

module.exports = router;
