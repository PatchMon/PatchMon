const express = require("express");
const logger = require("../utils/logger");
const axios = require("axios");
const { authenticateToken } = require("../middleware/auth");

const router = express.Router();

// Cache for supporter count (5 minute cache)
const supporterCountCache = {
	count: null,
	timestamp: null,
	cacheDuration: 5 * 60 * 1000, // 5 minutes
};

// Get supporter count from Buy Me a Coffee page
router.get("/supporter-count", authenticateToken, async (_req, res) => {
	try {
		// Check cache first
		const now = Date.now();
		if (
			supporterCountCache.count !== null &&
			supporterCountCache.timestamp !== null &&
			now - supporterCountCache.timestamp < supporterCountCache.cacheDuration
		) {
			return res.json({ count: supporterCountCache.count, cached: true });
		}

		// Fetch the Buy Me a Coffee page
		const response = await axios.get("https://buymeacoffee.com/iby___", {
			headers: {
				"User-Agent":
					"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
			},
			timeout: 10000, // 10 second timeout
		});

		const html = response.data;

		// Try multiple patterns to find the supporter count
		let supporterCount = null;

		// Pattern 1: Look for "X supporters" or "X supporter" text (most common)
		// Try various formats: "25 supporters", "1 supporter", "25 people", etc.
		const textPatterns = [
			/(\d+)\s+supporters?/i,
			/(\d+)\s+people\s+(have\s+)?(bought|supported)/i,
			/supporter[^>]*>.*?(\d+)/i,
			/(\d+)[^<]*supporter/i,
			/>(\d+)<[^>]*supporter/i,
			/supporter[^<]*<[^>]*>(\d+)/i,
		];

		for (const pattern of textPatterns) {
			const match = html.match(pattern);
			if (match?.[1]) {
				const count = parseInt(match[1], 10);
				if (count > 0 && count < 1000000) {
					// Reasonable upper limit
					supporterCount = count;
					break;
				}
			}
		}

		// Pattern 2: Look for data attributes or specific class names
		// Buy Me a Coffee might use data attributes like data-count, data-supporters, etc.
		if (!supporterCount) {
			const dataPatterns = [
				/data-supporters?=["'](\d+)["']/i,
				/data-count=["'](\d+)["']/i,
				/supporter[^>]*data-[^=]*=["'](\d+)["']/i,
			];

			for (const pattern of dataPatterns) {
				const match = html.match(pattern);
				if (match?.[1]) {
					const count = parseInt(match[1], 10);
					if (count > 0 && count < 1000000) {
						supporterCount = count;
						break;
					}
				}
			}
		}

		// Pattern 3: Look for JSON-LD structured data or meta tags
		if (!supporterCount) {
			const jsonLdMatches = html.matchAll(
				/<script[^>]*type=["']application\/ld\+json["'][^>]*>(.*?)<\/script>/gis,
			);
			for (const jsonLdMatch of jsonLdMatches) {
				try {
					const jsonLd = JSON.parse(jsonLdMatch[1]);
					// Look for supporter count in structured data (could be nested)
					const findCount = (obj) => {
						if (typeof obj !== "object" || obj === null) return null;
						if (obj.supporterCount || obj.supporter_count || obj.supporters) {
							return parseInt(
								obj.supporterCount || obj.supporter_count || obj.supporters,
								10,
							);
						}
						for (const value of Object.values(obj)) {
							if (typeof value === "object") {
								const found = findCount(value);
								if (found) return found;
							}
						}
						return null;
					};
					const count = findCount(jsonLd);
					if (count && count > 0 && count < 1000000) {
						supporterCount = count;
						break;
					}
				} catch (_e) {
					// Ignore JSON parse errors
				}
			}
		}

		// Pattern 4: Look for specific class names or IDs that Buy Me a Coffee uses
		if (!supporterCount) {
			const classPatterns = [
				/class="[^"]*supporter[^"]*"[^>]*>.*?(\d+)/i,
				/id="[^"]*supporter[^"]*"[^>]*>.*?(\d+)/i,
				/<span[^>]*class="[^"]*count[^"]*"[^>]*>(\d+)<\/span>/i,
			];

			for (const pattern of classPatterns) {
				const match = html.match(pattern);
				if (match?.[1]) {
					const count = parseInt(match[1], 10);
					if (count > 0 && count < 1000000) {
						supporterCount = count;
						break;
					}
				}
			}
		}

		// Pattern 5: Look for numbers near common supporter-related text
		if (!supporterCount) {
			// Find all numbers in the HTML and check context around them
			const numberMatches = html.matchAll(/\b(\d{1,6})\b/g);
			for (const match of numberMatches) {
				const num = parseInt(match[1], 10);
				if (num > 0 && num < 1000000) {
					// Check context around this number (200 chars before and after)
					const start = Math.max(0, match.index - 200);
					const end = Math.min(
						html.length,
						match.index + match[0].length + 200,
					);
					const context = html.substring(start, end).toLowerCase();
					if (
						context.includes("supporter") ||
						context.includes("coffee") ||
						context.includes("donation")
					) {
						supporterCount = num;
						break;
					}
				}
			}
		}

		// Update cache
		if (supporterCount !== null && supporterCount > 0) {
			supporterCountCache.count = supporterCount;
			supporterCountCache.timestamp = now;
		}

		if (supporterCount === null) {
			// If we couldn't parse it, return the cached value if available, or 0
			if (supporterCountCache.count !== null) {
				return res.json({
					count: supporterCountCache.count,
					cached: true,
					error: "Could not parse current count, returning cached value",
				});
			}
			return res.status(500).json({
				error: "Could not retrieve supporter count",
			});
		}

		res.json({ count: supporterCount, cached: false });
	} catch (error) {
		logger.error("Error fetching Buy Me a Coffee supporter count:", error);

		// Return cached value if available
		if (supporterCountCache.count !== null) {
			return res.json({
				count: supporterCountCache.count,
				cached: true,
				error: "Failed to fetch, returning cached value",
			});
		}

		res.status(500).json({
			error: "Failed to fetch supporter count",
		});
	}
});

module.exports = router;
