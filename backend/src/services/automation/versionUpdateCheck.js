const fs = require("node:fs");
const path = require("node:path");
const { prisma } = require("./shared/prisma");
const logger = require("../../utils/logger");
const { compareVersions, checkVersionFromDNS } = require("./shared/utils");
const { invalidateCache } = require("../settingsService");

/**
 * Version Update Check Automation
 * Checks for new releases using DNS TXT record lookup
 */
class VersionUpdateCheck {
	constructor(queueManager) {
		this.queueManager = queueManager;
		this.queueName = "version-update-check";
	}

	/**
	 * Process version update check job
	 */
	async process(_job) {
		const startTime = Date.now();
		logger.info("🔍 Starting version update check...");

		try {
			// Get settings
			const settings = await prisma.settings.findFirst();

			// Check version from DNS TXT record
			const latestVersion = await checkVersionFromDNS(
				"server.vcheck.patchmon.net",
			);

			if (!latestVersion) {
				throw new Error("Could not determine latest version");
			}

			// Read version from package.json (using fs to avoid require cache)
			let currentVersion = null;
			try {
				const packagePath = path.join(__dirname, "../../../package.json");
				const packageContent = fs.readFileSync(packagePath, "utf8");
				const packageJson = JSON.parse(packageContent);
				if (packageJson?.version) {
					currentVersion = packageJson.version;
				}
			} catch (packageError) {
				logger.error(
					"Could not read version from package.json:",
					packageError.message,
				);
				throw new Error(
					"Could not determine current version from package.json",
				);
			}

			if (!currentVersion) {
				throw new Error("Version not found in package.json");
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

			// Invalidate settings cache so frontend gets fresh data
			invalidateCache();

			const executionTime = Date.now() - startTime;
			logger.info(
				`✅ Version update check completed in ${executionTime}ms - Current: ${currentVersion}, Latest: ${latestVersion}, Update Available: ${isUpdateAvailable}`,
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
			logger.error(
				`❌ Version update check failed after ${executionTime}ms:`,
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
					// Invalidate settings cache
					invalidateCache();
				}
			} catch (updateError) {
				logger.error("❌ Error updating last check time:", updateError.message);
			}

			throw error;
		}
	}

	/**
	 * Schedule recurring version update check (daily at midnight)
	 */
	async schedule() {
		const job = await this.queueManager.queues[this.queueName].add(
			"version-update-check",
			{},
			{
				repeat: { cron: "0 0 * * *" }, // Daily at midnight
				jobId: "version-update-check-recurring",
			},
		);
		logger.info("✅ Version update check scheduled");
		return job;
	}

	/**
	 * Trigger manual version update check
	 */
	async triggerManual() {
		const job = await this.queueManager.queues[this.queueName].add(
			"version-update-check-manual",
			{},
			{ priority: 1 },
		);
		logger.info("✅ Manual version update check triggered");
		return job;
	}
}

module.exports = VersionUpdateCheck;

