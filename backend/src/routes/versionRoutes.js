const express = require("express");
const fs = require("node:fs");
const path = require("node:path");
const logger = require("../utils/logger");
const { authenticateToken } = require("../middleware/auth");
const { requireManageSettings } = require("../middleware/permissions");
const { getPrismaClient } = require("../config/prisma");
const _dns = require("node:dns").promises;
const { checkVersionFromDNS } = require("../services/automation/shared/utils");

const prisma = getPrismaClient();

// DNS domain for server version check
const SERVER_VERSION_DNS = "server.vcheck.patchmon.net";

const router = express.Router();

// Helper function to get current version from package.json
// Uses fs.readFileSync to get fresh version on each call (avoids require cache)
function getCurrentVersion() {
	try {
		const packagePath = path.join(__dirname, "../../package.json");
		const packageContent = fs.readFileSync(packagePath, "utf8");
		const packageJson = JSON.parse(packageContent);
		if (!packageJson?.version) {
			throw new Error("Version not found in package.json");
		}
		return packageJson.version;
	} catch (packageError) {
		logger.error(
			"Could not read version from package.json:",
			packageError.message,
		);
		return "unknown";
	}
}

// Add endpoint to get latest version (for frontend - public endpoint)
router.get("/latest", async (_req, res) => {
	try {
		const latestRelease = await getLatestRelease();
		res.json({
			version: latestRelease.version,
			tagName: latestRelease.tagName,
			htmlUrl: latestRelease.htmlUrl,
		});
	} catch (error) {
		logger.error("Error getting latest version:", error);
		// Return cached version from settings as fallback
		const settings = await prisma.settings.findFirst();
		if (settings?.latest_version) {
			res.json({
				version: settings.latest_version,
				tagName: `v${settings.latest_version}`,
				htmlUrl: `https://github.com/PatchMon/PatchMon/releases/tag/v${settings.latest_version}`,
			});
		} else {
			res.status(500).json({ error: "Failed to get latest version" });
		}
	}
});

// Helper function to get latest release from DNS
async function getLatestRelease() {
	try {
		const version = await checkVersionFromDNS(SERVER_VERSION_DNS);
		return {
			tagName: `v${version}`,
			version: version,
			publishedAt: null, // DNS doesn't provide publish date
			htmlUrl: `https://github.com/PatchMon/PatchMon/releases/tag/v${version}`,
		};
	} catch (error) {
		logger.error("Error fetching latest release from DNS:", error.message);
		throw error; // Re-throw to be caught by the calling function
	}
}

// Helper function to get latest commit from main branch
async function _getLatestCommit(owner, repo) {
	try {
		const currentVersion = getCurrentVersion();
		const apiUrl = `https://api.github.com/repos/${owner}/${repo}/commits/main`;

		const response = await fetch(apiUrl, {
			method: "GET",
			headers: {
				Accept: "application/vnd.github.v3+json",
				"User-Agent": `PatchMon-Server/${currentVersion}`,
			},
		});

		if (!response.ok) {
			const errorText = await response.text();
			if (
				errorText.includes("rate limit") ||
				errorText.includes("API rate limit")
			) {
				throw new Error("GitHub API rate limit exceeded");
			}
			throw new Error(
				`GitHub API error: ${response.status} ${response.statusText}`,
			);
		}

		const commitData = await response.json();
		return {
			sha: commitData.sha,
			message: commitData.commit.message,
			author: commitData.commit.author.name,
			date: commitData.commit.author.date,
			htmlUrl: commitData.html_url,
		};
	} catch (error) {
		logger.error("Error fetching latest commit:", error.message);
		throw error; // Re-throw to be caught by the calling function
	}
}

// Helper function to get commit count difference
async function _getCommitDifference(owner, repo, currentVersion) {
	// Try both with and without 'v' prefix for compatibility
	const versionTags = [
		currentVersion, // Try without 'v' first (new format)
		`v${currentVersion}`, // Try with 'v' prefix (old format)
	];

	for (const versionTag of versionTags) {
		try {
			// Compare main branch with the released version tag
			const apiUrl = `https://api.github.com/repos/${owner}/${repo}/compare/${versionTag}...main`;

			const response = await fetch(apiUrl, {
				method: "GET",
				headers: {
					Accept: "application/vnd.github.v3+json",
					"User-Agent": `PatchMon-Server/${getCurrentVersion()}`,
				},
			});

			if (!response.ok) {
				const errorText = await response.text();
				if (
					errorText.includes("rate limit") ||
					errorText.includes("API rate limit")
				) {
					throw new Error("GitHub API rate limit exceeded");
				}
				// If 404, try next tag format
				if (response.status === 404) {
					continue;
				}
				throw new Error(
					`GitHub API error: ${response.status} ${response.statusText}`,
				);
			}

			const compareData = await response.json();
			return {
				commitsBehind: compareData.behind_by || 0, // How many commits main is behind release
				commitsAhead: compareData.ahead_by || 0, // How many commits main is ahead of release
				totalCommits: compareData.total_commits || 0,
				branchInfo: "main branch vs release",
			};
		} catch (error) {
			// If rate limit, throw immediately
			if (error.message.includes("rate limit")) {
				throw error;
			}
		}
	}

	// If all attempts failed, throw error
	throw new Error(
		`Could not find tag '${currentVersion}' or 'v${currentVersion}' in repository`,
	);
}

// Helper function to compare version strings (semantic versioning)
function compareVersions(version1, version2) {
	const v1parts = version1.split(".").map(Number);
	const v2parts = version2.split(".").map(Number);

	const maxLength = Math.max(v1parts.length, v2parts.length);

	for (let i = 0; i < maxLength; i++) {
		const v1part = v1parts[i] || 0;
		const v2part = v2parts[i] || 0;

		if (v1part > v2part) return 1;
		if (v1part < v2part) return -1;
	}

	return 0;
}

// Get current version info
router.get("/current", authenticateToken, async (_req, res) => {
	try {
		const currentVersion = getCurrentVersion();

		// Get settings with cached update info (no DNS calls)
		const settings = await prisma.settings.findFirst();

		// Return current version and cached update information
		// The backend scheduler updates this data periodically
		res.json({
			version: currentVersion,
			latest_version: settings?.latest_version || null,
			is_update_available: settings?.is_update_available || false,
			last_update_check: settings?.last_update_check || null,
			buildDate: new Date().toISOString(),
			environment: process.env.NODE_ENV || "development",
		});
	} catch (error) {
		logger.error("Error getting current version:", error);
		res.status(500).json({ error: "Failed to get current version" });
	}
});

// Test SSH key permissions and GitHub access
router.post(
	"/test-ssh-key",
	authenticateToken,
	requireManageSettings,
	async (_req, res) => {
		res.status(410).json({
			error:
				"SSH key testing has been removed. Using default public repository.",
		});
	},
);

// Check for updates from DNS
router.get(
	"/check-updates",
	authenticateToken,
	requireManageSettings,
	async (_req, res) => {
		try {
			// Get cached update information from settings
			const settings = await prisma.settings.findFirst();

			if (!settings) {
				return res.status(400).json({ error: "Settings not found" });
			}

			const currentVersion = getCurrentVersion();

			let latestRelease = null;

			// Fetch fresh version from DNS
			try {
				latestRelease = await getLatestRelease();
			} catch (dnsError) {
				logger.warn("Failed to fetch version from DNS:", dnsError.message);

				// Fall back to cached data
				if (settings.latest_version) {
					latestRelease = {
						version: settings.latest_version,
						tagName: `v${settings.latest_version}`,
						publishedAt: null,
						htmlUrl: `https://github.com/PatchMon/PatchMon/releases/tag/v${settings.latest_version}`,
					};
				}
			}

			const latestVersion =
				latestRelease?.version || settings.latest_version || currentVersion;
			const isUpdateAvailable = latestRelease
				? compareVersions(latestVersion, currentVersion) > 0
				: settings.update_available || false;

			res.json({
				currentVersion,
				latestVersion,
				isUpdateAvailable,
				lastUpdateCheck: settings.last_update_check || null,
				latestRelease: latestRelease,
			});
		} catch (error) {
			logger.error("Error getting update information:", error);
			res.status(500).json({ error: "Failed to get update information" });
		}
	},
);

module.exports = router;
