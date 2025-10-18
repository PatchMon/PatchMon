const { prisma } = require("./shared/prisma");
const { compareVersions, checkPublicRepo } = require("./shared/utils");

/**
 * GitHub Update Check Automation
 * Checks for new releases on GitHub using HTTPS API
 */
class GitHubUpdateCheck {
	constructor(queueManager) {
		this.queueManager = queueManager;
		this.queueName = "github-update-check";
	}

	/**
	 * Process GitHub update check job
	 */
	async process(_job) {
		const startTime = Date.now();
		console.log("🔍 Starting GitHub update check...");

		try {
			// Get settings
			const settings = await prisma.settings.findFirst();
			const DEFAULT_GITHUB_REPO = "https://github.com/PatchMon/PatchMon.git";
			const repoUrl = settings?.githubRepoUrl || DEFAULT_GITHUB_REPO;
			let owner, repo;

			// Parse GitHub repository URL (supports both HTTPS and SSH formats)
			if (repoUrl.includes("git@github.com:")) {
				const match = repoUrl.match(/git@github\.com:([^/]+)\/([^/]+)\.git/);
				if (match) {
					[, owner, repo] = match;
				}
			} else if (repoUrl.includes("github.com/")) {
				const match = repoUrl.match(
					/github\.com\/([^/]+)\/([^/]+?)(?:\.git)?$/,
				);
				if (match) {
					[, owner, repo] = match;
				}
			}

			if (!owner || !repo) {
				throw new Error("Could not parse GitHub repository URL");
			}

			// Always use HTTPS GitHub API (simpler and more reliable)
			const latestVersion = await checkPublicRepo(owner, repo);

			if (!latestVersion) {
				throw new Error("Could not determine latest version");
			}

			// Read version from package.json
			let currentVersion = "1.2.7"; // fallback
			try {
				const packageJson = require("../../../package.json");
				if (packageJson?.version) {
					currentVersion = packageJson.version;
				}
			} catch (packageError) {
				console.warn(
					"Could not read version from package.json:",
					packageError.message,
				);
			}

			const isUpdateAvailable =
				compareVersions(latestVersion, currentVersion) > 0;

			// Update settings with check results
			await prisma.settings.update({
				where: { id: settings.id },
				data: {
					last_update_check: new Date(),
					update_available: isUpdateAvailable,
					latest_version: latestVersion,
				},
			});

			const executionTime = Date.now() - startTime;
			console.log(
				`✅ GitHub update check completed in ${executionTime}ms - Current: ${currentVersion}, Latest: ${latestVersion}, Update Available: ${isUpdateAvailable}`,
			);

			return {
				success: true,
				currentVersion,
				latestVersion,
				isUpdateAvailable,
				executionTime,
			};
		} catch (error) {
			const executionTime = Date.now() - startTime;
			console.error(
				`❌ GitHub update check failed after ${executionTime}ms:`,
				error.message,
			);

			// Update last check time even on error
			try {
				const settings = await prisma.settings.findFirst();
				if (settings) {
					await prisma.settings.update({
						where: { id: settings.id },
						data: {
							last_update_check: new Date(),
							update_available: false,
						},
					});
				}
			} catch (updateError) {
				console.error(
					"❌ Error updating last check time:",
					updateError.message,
				);
			}

			throw error;
		}
	}

	/**
	 * Schedule recurring GitHub update check (daily at midnight)
	 */
	async schedule() {
		const job = await this.queueManager.queues[this.queueName].add(
			"github-update-check",
			{},
			{
				repeat: { cron: "0 0 * * *" }, // Daily at midnight
				jobId: "github-update-check-recurring",
			},
		);
		console.log("✅ GitHub update check scheduled");
		return job;
	}

	/**
	 * Trigger manual GitHub update check
	 */
	async triggerManual() {
		const job = await this.queueManager.queues[this.queueName].add(
			"github-update-check-manual",
			{},
			{ priority: 1 },
		);
		console.log("✅ Manual GitHub update check triggered");
		return job;
	}
}

module.exports = GitHubUpdateCheck;
