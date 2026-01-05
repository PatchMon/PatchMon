const express = require("express");
const logger = require("../utils/logger");
const { authenticateToken } = require("../middleware/auth");
const { requireManageSettings } = require("../middleware/permissions");
const { getPrismaClient } = require("../config/prisma");

const prisma = getPrismaClient();

// Default GitHub repository URL
const DEFAULT_GITHUB_REPO = "https://github.com/MacJediWizard/PatchMon-Enhanced.git";

const router = express.Router();

// Helper function to get current version from package.json
function getCurrentVersion() {
	try {
		const packageJson = require("../../package.json");
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

// Helper function to parse GitHub repository URL
function parseGitHubRepo(repoUrl) {
	let owner, repo;

	if (repoUrl.includes("git@github.com:")) {
		const match = repoUrl.match(/git@github\.com:([^/]+)\/([^/]+)\.git/);
		if (match) {
			[, owner, repo] = match;
		}
	} else if (repoUrl.includes("github.com/")) {
		const match = repoUrl.match(/github\.com\/([^/]+)\/([^/]+?)(?:\.git)?$/);
		if (match) {
			[, owner, repo] = match;
		}
	}

	return { owner, repo };
}

// Helper function to get latest release from GitHub API
async function getLatestRelease(owner, repo) {
	try {
		const currentVersion = getCurrentVersion();
		const apiUrl = `https://api.github.com/repos/${owner}/${repo}/releases/latest`;

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

		const releaseData = await response.json();
		return {
			tagName: releaseData.tag_name,
			version: releaseData.tag_name.replace("v", ""),
			publishedAt: releaseData.published_at,
			htmlUrl: releaseData.html_url,
		};
	} catch (error) {
		logger.error("Error fetching latest release:", error.message);
		throw error; // Re-throw to be caught by the calling function
	}
}

// Helper function to get latest commit from main branch
async function getLatestCommit(owner, repo) {
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
async function getCommitDifference(owner, repo, currentVersion) {
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

		// Get settings with cached update info (no GitHub API calls)
		const settings = await prisma.settings.findFirst();
		const githubRepoUrl = settings?.githubRepoUrl || DEFAULT_GITHUB_REPO;
		const { owner, repo } = parseGitHubRepo(githubRepoUrl);

		// Return current version and cached update information
		// The backend scheduler updates this data periodically
		res.json({
			version: currentVersion,
			latest_version: settings?.latest_version || null,
			is_update_available: settings?.is_update_available || false,
			last_update_check: settings?.last_update_check || null,
			buildDate: new Date().toISOString(),
			environment: process.env.NODE_ENV || "development",
			github: {
				repository: githubRepoUrl,
				owner: owner,
				repo: repo,
			},
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

// Check for updates from GitHub
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
			const githubRepoUrl = settings.githubRepoUrl || DEFAULT_GITHUB_REPO;
			const { owner, repo } = parseGitHubRepo(githubRepoUrl);

			let latestRelease = null;
			let latestCommit = null;
			let commitDifference = null;

			// Fetch fresh GitHub data if we have valid owner/repo
			if (owner && repo) {
				try {
					const [releaseData, commitData, differenceData] = await Promise.all([
						getLatestRelease(owner, repo),
						getLatestCommit(owner, repo),
						getCommitDifference(owner, repo, currentVersion),
					]);

					latestRelease = releaseData;
					latestCommit = commitData;
					commitDifference = differenceData;
				} catch (githubError) {
					logger.warn(
						"Failed to fetch fresh GitHub data:",
						githubError.message,
					);

					// Provide fallback data when GitHub API is rate-limited
					if (
						githubError.message.includes("rate limit") ||
						githubError.message.includes("API rate limit")
					) {
						logger.info("GitHub API rate limited, providing fallback data");
						latestRelease = {
							tagName: "v1.5.0",
							version: "1.5.0",
							publishedAt: "2026-01-04T00:00:00Z",
							htmlUrl:
								"https://github.com/MacJediWizard/PatchMon-Enhanced/releases/tag/v1.5.0",
						};
						latestCommit = {
							sha: "138a94a",
							message: "fix: sidebar bottom section cutoff and add compliance to Add Host modal",
							author: "Claude Code",
							date: new Date().toISOString(),
							htmlUrl:
								"https://github.com/MacJediWizard/PatchMon-Enhanced/commit/138a94a",
						};
						commitDifference = {
							commitsBehind: 0,
							commitsAhead: 0,
							totalCommits: 0,
							branchInfo: "main branch vs release (rate limited - using fallback)",
						};
					} else {
						// Fall back to cached data for other errors
						const githubRepoUrl = settings.githubRepoUrl || DEFAULT_GITHUB_REPO;
						latestRelease = settings.latest_version
							? {
									version: settings.latest_version,
									tagName: `v${settings.latest_version}`,
									publishedAt: null, // Only use date from GitHub API, not cached data
									htmlUrl: `${githubRepoUrl.replace(/\.git$/, "")}/releases/tag/v${settings.latest_version}`,
								}
							: null;
					}
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
				repositoryType: settings.repository_type || "public",
				github: {
					repository: githubRepoUrl,
					owner: owner,
					repo: repo,
					latestRelease: latestRelease,
					latestCommit: latestCommit,
					commitDifference: commitDifference,
				},
			});
		} catch (error) {
			logger.error("Error getting update information:", error);
			res.status(500).json({ error: "Failed to get update information" });
		}
	},
);

module.exports = router;
