const axios = require("axios");
const logger = require("../../utils/logger");

// In-memory cache for social media statistics
const socialMediaStatsCache = {
	github_stars: null,
	discord_members: null,
	buymeacoffee_supporters: null,
	youtube_subscribers: null,
	linkedin_followers: null,
	last_updated: null,
};

/**
 * Helper function to parse subscriber/follower count from text with K/M/B suffixes
 */
function parseCount(text) {
	if (!text) return null;

	// Remove commas and extract numbers
	const cleanText = text.replace(/,/g, "").trim();

	// Match patterns like "1.2K", "1.2M", "123K", etc.
	const match = cleanText.match(/([\d.]+)\s*([KMB])?/i);
	if (match) {
		let count = parseFloat(match[1]);
		const suffix = match[2]?.toUpperCase();

		if (suffix === "K") {
			count *= 1000;
		} else if (suffix === "M") {
			count *= 1000000;
		} else if (suffix === "B") {
			count *= 1000000000;
		}

		return Math.floor(count);
	}

	// Try to find just numbers
	const numbers = cleanText.match(/\d+/);
	if (numbers) {
		return parseInt(numbers[0], 10);
	}

	return null;
}

/**
 * Scrape Buy Me a Coffee supporter count
 * Extracted from buyMeACoffeeRoutes.js
 */
async function scrapeBuyMeACoffee() {
	try {
		const response = await axios.get("https://buymeacoffee.com/iby___", {
			headers: {
				"User-Agent":
					"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
			},
			timeout: 10000,
		});

		const html = response.data;
		let supporterCount = null;

		// Pattern 1: Look for "X supporters" text
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
					supporterCount = count;
					break;
				}
			}
		}

		// Pattern 2: Look for data attributes
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

		// Pattern 3: Look for JSON-LD structured data
		if (!supporterCount) {
			const jsonLdMatches = html.matchAll(
				/<script[^>]*type=["']application\/ld\+json["'][^>]*>(.*?)<\/script>/gis,
			);
			for (const jsonLdMatch of jsonLdMatches) {
				try {
					const jsonLd = JSON.parse(jsonLdMatch[1]);
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

		// Pattern 4: Look for class names or IDs
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

		// Pattern 5: Look for numbers near supporter-related text
		if (!supporterCount) {
			const numberMatches = html.matchAll(/\b(\d{1,6})\b/g);
			for (const match of numberMatches) {
				const num = parseInt(match[1], 10);
				if (num > 0 && num < 1000000) {
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

		return supporterCount;
	} catch (error) {
		logger.error("Error scraping Buy Me a Coffee:", error.message);
		return null;
	}
}

/**
 * Scrape Discord member count from invitation page
 */
async function scrapeDiscord() {
	try {
		const response = await axios.get("https://patchmon.net/discord", {
			headers: {
				"User-Agent":
					"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36",
			},
			timeout: 10000,
			maxRedirects: 5,
		});

		const html = response.data;
		let memberCount = null;

		// Pattern 1: Look for "X members" or "X online" text
		const textPatterns = [
			/(\d+)\s+members?/i,
			/(\d+)\s+online/i,
			/member[^>]*>.*?(\d+)/i,
			/(\d+)[^<]*member/i,
			/>(\d+)<[^>]*member/i,
		];

		for (const pattern of textPatterns) {
			const match = html.match(pattern);
			if (match?.[1]) {
				const count = parseInt(match[1], 10);
				if (count > 0 && count < 10000000) {
					memberCount = count;
					break;
				}
			}
		}

		// Pattern 2: Look for data attributes
		if (!memberCount) {
			const dataPatterns = [
				/data-members?=["'](\d+)["']/i,
				/data-count=["'](\d+)["']/i,
				/member[^>]*data-[^=]*=["'](\d+)["']/i,
			];

			for (const pattern of dataPatterns) {
				const match = html.match(pattern);
				if (match?.[1]) {
					const count = parseInt(match[1], 10);
					if (count > 0 && count < 10000000) {
						memberCount = count;
						break;
					}
				}
			}
		}

		// Pattern 3: Look for JSON-LD or meta tags
		if (!memberCount) {
			const jsonLdMatches = html.matchAll(
				/<script[^>]*type=["']application\/ld\+json["'][^>]*>(.*?)<\/script>/gis,
			);
			for (const jsonLdMatch of jsonLdMatches) {
				try {
					const jsonLd = JSON.parse(jsonLdMatch[1]);
					const findCount = (obj) => {
						if (typeof obj !== "object" || obj === null) return null;
						if (obj.memberCount || obj.member_count || obj.members) {
							return parseInt(
								obj.memberCount || obj.member_count || obj.members,
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
					if (count && count > 0 && count < 10000000) {
						memberCount = count;
						break;
					}
				} catch (_e) {
					// Ignore JSON parse errors
				}
			}
		}

		// Pattern 4: Look for numbers near member-related text
		if (!memberCount) {
			const numberMatches = html.matchAll(/\b(\d{1,7})\b/g);
			for (const match of numberMatches) {
				const num = parseInt(match[1], 10);
				if (num > 0 && num < 10000000) {
					const start = Math.max(0, match.index - 200);
					const end = Math.min(
						html.length,
						match.index + match[0].length + 200,
					);
					const context = html.substring(start, end).toLowerCase();
					if (
						context.includes("member") ||
						context.includes("discord") ||
						context.includes("online")
					) {
						memberCount = num;
						break;
					}
				}
			}
		}

		return memberCount;
	} catch (error) {
		logger.error("Error scraping Discord:", error.message);
		return null;
	}
}

/**
 * Scrape YouTube subscriber count
 */
async function scrapeYouTube() {
	try {
		const response = await axios.get("https://www.youtube.com/@patchmonTV", {
			headers: {
				"User-Agent":
					"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36",
			},
			timeout: 10000,
		});

		const html = response.data;
		let subscriberCount = null;

		// Pattern 1: Look for ytInitialData JavaScript variable
		const ytInitialDataMatch = html.match(/var ytInitialData = ({.+?});/);
		if (ytInitialDataMatch) {
			try {
				const ytData = JSON.parse(ytInitialDataMatch[1]);
				// Navigate through the nested structure to find subscriber count
				const findSubscriberCount = (obj, depth = 0) => {
					if (depth > 10) return null;
					if (typeof obj !== "object" || obj === null) return null;

					// Check for subscriber count in various possible locations
					if (obj.subscriberCount || obj.subscriber_count || obj.subscribers) {
						const count = parseCount(
							String(
								obj.subscriberCount || obj.subscriber_count || obj.subscribers,
							),
						);
						if (count && count > 0) return count;
					}

					// Check for subscriber text
					if (typeof obj === "string" && obj.includes("subscriber")) {
						const count = parseCount(obj);
						if (count && count > 0) return count;
					}

					// Recursively search
					if (Array.isArray(obj)) {
						for (const item of obj) {
							const found = findSubscriberCount(item, depth + 1);
							if (found) return found;
						}
					} else {
						for (const value of Object.values(obj)) {
							const found = findSubscriberCount(value, depth + 1);
							if (found) return found;
						}
					}

					return null;
				};

				subscriberCount = findSubscriberCount(ytData);
			} catch (_e) {
				// Ignore JSON parse errors
			}
		}

		// Pattern 2: Look for subscriber count in HTML text
		if (!subscriberCount) {
			const subscriberPatterns = [
				/(\d+(?:\.\d+)?[KMB]?)\s+subscribers?/i,
				/subscribers?[^>]*>.*?(\d+(?:\.\d+)?[KMB]?)/i,
				/(\d+(?:\.\d+)?[KMB]?)[^<]*subscriber/i,
			];

			for (const pattern of subscriberPatterns) {
				const match = html.match(pattern);
				if (match?.[1]) {
					const count = parseCount(match[1]);
					if (count && count > 0) {
						subscriberCount = count;
						break;
					}
				}
			}
		}

		// Pattern 3: Look for numbers near subscriber-related text
		if (!subscriberCount) {
			const numberMatches = html.matchAll(/(\d+(?:\.\d+)?[KMB]?)/gi);
			for (const match of numberMatches) {
				const start = Math.max(0, match.index - 100);
				const end = Math.min(html.length, match.index + match[0].length + 100);
				const context = html.substring(start, end).toLowerCase();
				if (context.includes("subscriber")) {
					const count = parseCount(match[1]);
					if (count && count > 0) {
						subscriberCount = count;
						break;
					}
				}
			}
		}

		return subscriberCount;
	} catch (error) {
		logger.error("Error scraping YouTube:", error.message);
		return null;
	}
}

/**
 * Scrape LinkedIn follower count
 */
async function scrapeLinkedIn() {
	try {
		const response = await axios.get("https://linkedin.com/company/patchmon", {
			headers: {
				"User-Agent":
					"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36",
			},
			timeout: 10000,
		});

		const html = response.data;
		let followerCount = null;

		// Pattern 1: Look for follower count in text
		const textPatterns = [
			/(\d+(?:\.\d+)?[KMB]?)\s+followers?/i,
			/followers?[^>]*>.*?(\d+(?:\.\d+)?[KMB]?)/i,
			/(\d+(?:\.\d+)?[KMB]?)[^<]*follower/i,
		];

		for (const pattern of textPatterns) {
			const match = html.match(pattern);
			if (match?.[1]) {
				const count = parseCount(match[1]);
				if (count && count > 0) {
					followerCount = count;
					break;
				}
			}
		}

		// Pattern 2: Look for data attributes
		if (!followerCount) {
			const dataPatterns = [
				/data-followers?=["'](\d+(?:\.\d+)?[KMB]?)["']/i,
				/data-count=["'](\d+(?:\.\d+)?[KMB]?)["']/i,
				/follower[^>]*data-[^=]*=["'](\d+(?:\.\d+)?[KMB]?)["']/i,
			];

			for (const pattern of dataPatterns) {
				const match = html.match(pattern);
				if (match?.[1]) {
					const count = parseCount(match[1]);
					if (count && count > 0) {
						followerCount = count;
						break;
					}
				}
			}
		}

		// Pattern 3: Look for JSON-LD structured data
		if (!followerCount) {
			const jsonLdMatches = html.matchAll(
				/<script[^>]*type=["']application\/ld\+json["'][^>]*>(.*?)<\/script>/gis,
			);
			for (const jsonLdMatch of jsonLdMatches) {
				try {
					const jsonLd = JSON.parse(jsonLdMatch[1]);
					const findCount = (obj) => {
						if (typeof obj !== "object" || obj === null) return null;
						if (obj.followerCount || obj.follower_count || obj.followers) {
							return parseCount(
								String(
									obj.followerCount || obj.follower_count || obj.followers,
								),
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
					if (count && count > 0) {
						followerCount = count;
						break;
					}
				} catch (_e) {
					// Ignore JSON parse errors
				}
			}
		}

		// Pattern 4: Look for numbers near follower-related text
		if (!followerCount) {
			const numberMatches = html.matchAll(/(\d+(?:\.\d+)?[KMB]?)/gi);
			for (const match of numberMatches) {
				const start = Math.max(0, match.index - 100);
				const end = Math.min(html.length, match.index + match[0].length + 100);
				const context = html.substring(start, end).toLowerCase();
				if (context.includes("follower")) {
					const count = parseCount(match[1]);
					if (count && count > 0) {
						followerCount = count;
						break;
					}
				}
			}
		}

		return followerCount;
	} catch (error) {
		logger.error("Error scraping LinkedIn:", error.message);
		return null;
	}
}

/**
 * Social Media Stats Collection Automation
 * Fetches social media statistics and stores them in memory cache
 */
class SocialMediaStats {
	constructor(queueManager) {
		this.queueManager = queueManager;
		this.queueName = "social-media-stats";
	}

	/**
	 * Process social media stats collection job
	 */
	async process(_job) {
		const startTime = Date.now();
		logger.info("📊 Starting social media stats collection...");

		const results = {
			github_stars: null,
			discord_members: null,
			buymeacoffee_supporters: null,
			youtube_subscribers: null,
			linkedin_followers: null,
		};

		try {
			// Fetch GitHub stars
			try {
				const response = await axios.get(
					"https://api.github.com/repos/PatchMon/PatchMon",
					{
						headers: {
							Accept: "application/vnd.github.v3+json",
						},
						timeout: 10000,
					},
				);

				if (response.data?.stargazers_count) {
					results.github_stars = response.data.stargazers_count;
					logger.info(`✅ GitHub stars: ${results.github_stars}`);
				}
			} catch (error) {
				logger.error("Error fetching GitHub stars:", error.message);
			}

			// Scrape Discord members
			try {
				const discordCount = await scrapeDiscord();
				if (discordCount !== null) {
					results.discord_members = discordCount;
					logger.info(`✅ Discord members: ${results.discord_members}`);
				}
			} catch (error) {
				logger.error("Error scraping Discord:", error.message);
			}

			// Scrape Buy Me a Coffee supporters
			try {
				const bmcCount = await scrapeBuyMeACoffee();
				if (bmcCount !== null) {
					results.buymeacoffee_supporters = bmcCount;
					logger.info(
						`✅ Buy Me a Coffee supporters: ${results.buymeacoffee_supporters}`,
					);
				}
			} catch (error) {
				logger.error("Error scraping Buy Me a Coffee:", error.message);
			}

			// Scrape YouTube subscribers
			try {
				const youtubeCount = await scrapeYouTube();
				if (youtubeCount !== null) {
					results.youtube_subscribers = youtubeCount;
					logger.info(`✅ YouTube subscribers: ${results.youtube_subscribers}`);
				}
			} catch (error) {
				logger.error("Error scraping YouTube:", error.message);
			}

			// Scrape LinkedIn followers
			try {
				const linkedinCount = await scrapeLinkedIn();
				if (linkedinCount !== null) {
					results.linkedin_followers = linkedinCount;
					logger.info(`✅ LinkedIn followers: ${results.linkedin_followers}`);
				}
			} catch (error) {
				logger.error("Error scraping LinkedIn:", error.message);
			}

			// Update cache - only update fields that successfully fetched
			// Preserve existing values if fetch failed
			if (results.github_stars !== null) {
				socialMediaStatsCache.github_stars = results.github_stars;
			}
			if (results.discord_members !== null) {
				socialMediaStatsCache.discord_members = results.discord_members;
			}
			if (results.buymeacoffee_supporters !== null) {
				socialMediaStatsCache.buymeacoffee_supporters =
					results.buymeacoffee_supporters;
			}
			if (results.youtube_subscribers !== null) {
				socialMediaStatsCache.youtube_subscribers = results.youtube_subscribers;
			}
			if (results.linkedin_followers !== null) {
				socialMediaStatsCache.linkedin_followers = results.linkedin_followers;
			}

			// Update last_updated timestamp if at least one stat was fetched
			if (
				results.github_stars !== null ||
				results.discord_members !== null ||
				results.buymeacoffee_supporters !== null ||
				results.youtube_subscribers !== null ||
				results.linkedin_followers !== null
			) {
				socialMediaStatsCache.last_updated = new Date();
			}

			const executionTime = Date.now() - startTime;
			logger.info(
				`✅ Social media stats collection completed in ${executionTime}ms`,
			);

			return {
				success: true,
				...results,
				executionTime,
			};
		} catch (error) {
			const executionTime = Date.now() - startTime;
			logger.error(
				`❌ Social media stats collection failed after ${executionTime}ms:`,
				error.message,
			);
			throw error;
		}
	}

	/**
	 * Schedule recurring social media stats collection (daily at midnight)
	 */
	async schedule() {
		const job = await this.queueManager.queues[this.queueName].add(
			"social-media-stats",
			{},
			{
				repeat: { cron: "0 0 * * *" }, // Daily at midnight
				jobId: "social-media-stats-recurring",
			},
		);
		logger.info("✅ Social media stats collection scheduled");
		return job;
	}

	/**
	 * Trigger manual social media stats collection
	 */
	async triggerManual() {
		const job = await this.queueManager.queues[this.queueName].add(
			"social-media-stats-manual",
			{},
			{ priority: 1 },
		);
		logger.info("✅ Manual social media stats collection triggered");
		return job;
	}
}

module.exports = SocialMediaStats;
module.exports.socialMediaStatsCache = socialMediaStatsCache;
