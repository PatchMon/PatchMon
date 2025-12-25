// Common utilities for automation jobs
const path = require("node:path");
const fs = require("node:fs");

/**
 * Compare two semantic versions
 * @param {string} version1 - First version
 * @param {string} version2 - Second version
 * @returns {number} - 1 if version1 > version2, -1 if version1 < version2, 0 if equal
 */
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

/**
 * Check public GitHub repository for latest release
 * @param {string} owner - Repository owner
 * @param {string} repo - Repository name
 * @returns {Promise<string|null>} - Latest version or null
 */
async function checkPublicRepo(owner, repo) {
	try {
		const httpsRepoUrl = `https://api.github.com/repos/${owner}/${repo}/releases/latest`;

		// Get current version for User-Agent (or use generic if unavailable)
		let currentVersion = "unknown";
		try {
			// Use __dirname to construct absolute path to package.json
			const packageJsonPath = path.join(__dirname, "../../../package.json");
			const packageJsonContent = fs.readFileSync(packageJsonPath, "utf8");
			const packageJson = JSON.parse(packageJsonContent);
			if (packageJson?.version) {
				currentVersion = packageJson.version;
			}
		} catch (packageError) {
			console.warn(
				"Could not read version from package.json for User-Agent:",
				packageError.message,
			);
		}

		const response = await fetch(httpsRepoUrl, {
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
				console.log("⚠️ GitHub API rate limit exceeded, skipping update check");
				return null;
			}
			throw new Error(
				`GitHub API error: ${response.status} ${response.statusText}`,
			);
		}

		const releaseData = await response.json();
		return releaseData.tag_name.replace("v", "");
	} catch (error) {
		console.error("GitHub API error:", error.message);
		throw error;
	}
}

module.exports = {
	compareVersions,
	checkPublicRepo,
};
