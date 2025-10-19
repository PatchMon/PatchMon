const express = require("express");
const { authenticateToken } = require("../middleware/auth");
const { requireManageSettings } = require("../middleware/permissions");
const { getPrismaClient } = require("../config/prisma");

const prisma = getPrismaClient();

// Default GitHub repository URL
const DEFAULT_GITHUB_REPO = "https://github.com/PatchMon/PatchMon.git";

const router = express.Router();

// Helper function to get current version from package.json
function getCurrentVersion() {
	try {
		const packageJson = require("../../package.json");
		return packageJson?.version || "1.3.0";
	} catch (packageError) {
		console.warn(
			"Could not read version from package.json, using fallback:",
			packageError.message,
		);
		return "1.3.0";
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
		console.error("Error fetching latest release:", error.message);
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
		console.error("Error fetching latest commit:", error.message);
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
		console.error("Error getting current version:", error);
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
					console.warn(
						"Failed to fetch fresh GitHub data:",
						githubError.message,
					);

					// Provide fallback data when GitHub API is rate-limited
					if (
						githubError.message.includes("rate limit") ||
						githubError.message.includes("API rate limit")
					) {
						console.log("GitHub API rate limited, providing fallback data");
						latestRelease = {
							tagName: "v1.2.8",
							version: "1.2.8",
							publishedAt: "2025-10-02T17:12:53Z",
							htmlUrl:
								"https://github.com/PatchMon/PatchMon/releases/tag/v1.2.8",
						};
						latestCommit = {
							sha: "cc89df161b8ea5d48ff95b0eb405fe69042052cd",
							message: "Update README.md\n\nAdded Documentation Links",
							author: "9 Technology Group LTD",
							date: "2025-10-04T18:38:09Z",
							htmlUrl:
								"https://github.com/PatchMon/PatchMon/commit/cc89df161b8ea5d48ff95b0eb405fe69042052cd",
						};
						commitDifference = {
							commitsBehind: 0,
							commitsAhead: 3, // Main branch is ahead of release
							totalCommits: 3,
							branchInfo: "main branch vs release",
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
			console.error("Error getting update information:", error);
			res.status(500).json({ error: "Failed to get update information" });
		}
	},
);

module.exports = router;
