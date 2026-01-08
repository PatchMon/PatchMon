const express = require("express");
const logger = require("../utils/logger");
const {
	socialMediaStatsCache,
} = require("../services/automation/socialMediaStats");

const router = express.Router();

// Get social media statistics from cache
router.get("/", async (_req, res) => {
	try {
		res.json({
			github_stars: socialMediaStatsCache.github_stars,
			discord_members: socialMediaStatsCache.discord_members,
			buymeacoffee_supporters: socialMediaStatsCache.buymeacoffee_supporters,
			youtube_subscribers: socialMediaStatsCache.youtube_subscribers,
			linkedin_followers: socialMediaStatsCache.linkedin_followers,
			last_updated: socialMediaStatsCache.last_updated,
		});
	} catch (error) {
		logger.error("Error fetching social media stats:", error);
		res.status(500).json({
			error: "Failed to fetch social media statistics",
		});
	}
});

module.exports = router;
