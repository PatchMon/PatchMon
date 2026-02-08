const fs = require("node:fs");
const path = require("node:path");
const { prisma } = require("./shared/prisma");
const logger = require("../../utils/logger");
const { compareVersions, checkVersionFromDNS } = require("./shared/utils");
const { invalidateCache } = require("../settingsService");
const alertService = require("../alertService");
const alertConfigService = require("../alertConfigService");
const agentVersionService = require("../agentVersionService");

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
	 * Check and create alert for a specific update type (server or agent)
	 */
	async checkAndCreateUpdateAlert(
		alertType,
		currentVersion,
		latestVersion,
		updateTitle,
		updateMessage,
	) {
		try {
			logger.info(
				`🔍 Processing ${alertType} alert check - current: ${currentVersion}, latest: ${latestVersion}`,
			);

			const alertsEnabled = await alertService.isAlertsEnabled();
			if (!alertsEnabled) {
				logger.info(
					`⚠️ Alerts system is disabled, skipping ${alertType} update alert`,
				);
				return;
			}

			const isEnabled = await alertConfigService.isAlertTypeEnabled(alertType);
			if (!isEnabled) {
				logger.info(`⚠️ ${alertType} alerts are disabled, skipping`);
				logger.info(
					`💡 Tip: Enable ${alertType} alerts in Alert Configuration settings to receive update notifications.`,
				);
				return;
			}

			const defaultSeverity =
				await alertConfigService.getDefaultSeverity(alertType);

			const isUpdateAvailable =
				compareVersions(latestVersion, currentVersion) > 0;

			if (isUpdateAvailable) {
				// Check if alert already exists for this specific update version
				const existingAlert = await prisma.alerts.findFirst({
					where: {
						type: alertType,
						is_active: true,
						metadata: {
							path: ["latest_version"],
							equals: latestVersion,
						},
					},
				});

				if (!existingAlert) {
					// Create new alert
					logger.info(`📝 Creating ${alertType} alert...`);
					const alert = await alertService.createAlert(
						alertType,
						defaultSeverity,
						updateTitle,
						updateMessage,
						{
							current_version: currentVersion,
							latest_version: latestVersion,
						},
					);

					if (!alert) {
						logger.warn(
							`⚠️ Alert creation returned null for ${alertType} - alerts may be disabled`,
						);
						return;
					}

					// Check auto-assignment
					if (
						await alertConfigService.shouldAutoAssign(alertType, {
							severity: defaultSeverity,
						})
					) {
						const assignUserId =
							await alertConfigService.getAutoAssignUser(alertType);
						if (assignUserId) {
							await alertService.assignAlertToUser(
								alert.id,
								assignUserId,
								null,
							);
							logger.info(
								`✅ Auto-assigned ${alertType} alert ${alert.id} to user ${assignUserId}`,
							);
						}
					}

					logger.info(`✅ Created ${alertType} alert: ${alert.id}`);
				} else {
					logger.info(
						`ℹ️ ${alertType} update alert already exists: ${existingAlert.id}`,
					);
				}
			} else {
				// System is up to date - resolve ALL active alerts of this type
				const activeAlerts = await prisma.alerts.findMany({
					where: {
						type: alertType,
						is_active: true,
					},
				});

				if (activeAlerts.length > 0) {
					logger.info(
						`✅ Resolving ${activeAlerts.length} active ${alertType} alert(s) - system is up to date`,
					);
					for (const alert of activeAlerts) {
						await alertService.performAlertAction(null, alert.id, "resolved", {
							reason: "update_no_longer_available",
							current_version: currentVersion,
							latest_version: latestVersion,
						});
						logger.info(`✅ Resolved ${alertType} alert: ${alert.id}`);
					}
				} else {
					logger.info(
						`ℹ️ ${alertType} is up to date, no active alerts to resolve`,
					);
				}
			}
		} catch (alertError) {
			// Don't fail the version check if alert creation fails
			logger.error(`Failed to create/update ${alertType} alert:`, alertError);
		}
	}

	/**
	 * Process version update check job
	 * Checks both server and agent versions
	 */
	async process(_job) {
		const startTime = Date.now();
		logger.info("🔍 Starting version update check (server and agent)...");

		try {
			// Get settings
			const settings = await prisma.settings.findFirst();

			// ===== SERVER VERSION CHECK =====
			let serverCurrentVersion = null;
			let serverLatestVersion = null;
			let serverUpdateAvailable = false;

			try {
				// Check server version from DNS TXT record
				serverLatestVersion = await checkVersionFromDNS(
					"server.vcheck.patchmon.net",
				);

				if (!serverLatestVersion) {
					logger.warn("⚠️ Could not determine latest server version from DNS");
				} else {
					// Read version from package.json (using fs to avoid require cache)
					try {
						const packagePath = path.join(__dirname, "../../../package.json");
						const packageContent = fs.readFileSync(packagePath, "utf8");
						const packageJson = JSON.parse(packageContent);
						if (packageJson?.version) {
							serverCurrentVersion = packageJson.version;
						}
					} catch (packageError) {
						logger.error(
							"Could not read version from package.json:",
							packageError.message,
						);
					}

					if (serverCurrentVersion && serverLatestVersion) {
						serverUpdateAvailable =
							compareVersions(serverLatestVersion, serverCurrentVersion) > 0;
					}
				}
			} catch (serverError) {
				logger.error("❌ Server version check failed:", serverError.message);
			}

			// ===== AGENT VERSION CHECK =====
			let agentCurrentVersion = null;
			let agentLatestVersion = null;
			let agentUpdateAvailable = false;

			try {
				// Use the existing agentVersionService to get version info (same as used in /settings/agent-version)
				logger.info(
					"🔍 Getting agent version info from agentVersionService...",
				);
				const agentVersionInfo = await agentVersionService.getVersionInfo();

				agentCurrentVersion = agentVersionInfo.currentVersion || null;
				agentLatestVersion = agentVersionInfo.latestVersion || null;

				if (agentCurrentVersion) {
					logger.info(`✅ Current agent version: ${agentCurrentVersion}`);
				} else {
					logger.warn(
						"⚠️ Could not determine current agent version (binary not found or version parsing failed)",
					);
				}

				if (agentLatestVersion) {
					logger.info(`✅ Latest agent version: ${agentLatestVersion}`);
				} else {
					logger.warn("⚠️ Could not determine latest agent version from DNS");
				}

				if (agentCurrentVersion && agentLatestVersion) {
					agentUpdateAvailable =
						compareVersions(agentLatestVersion, agentCurrentVersion) > 0;
					logger.info(
						`🔍 Agent version comparison: ${agentCurrentVersion} vs ${agentLatestVersion} = ${agentUpdateAvailable ? "UPDATE AVAILABLE" : "UP TO DATE"}`,
					);
				} else {
					logger.warn(
						`⚠️ Cannot compare agent versions - current: ${agentCurrentVersion || "null"}, latest: ${agentLatestVersion || "null"}`,
					);
				}
			} catch (agentError) {
				logger.error("❌ Agent version check failed:", agentError.message);
				logger.error("❌ Agent version check error stack:", agentError.stack);
			}

			// Update settings with server check results (for backward compatibility)
			if (serverCurrentVersion && serverLatestVersion) {
				await prisma.settings.update({
					where: { id: settings.id },
					data: {
						last_update_check: new Date(),
						update_available: serverUpdateAvailable,
						latest_version: serverLatestVersion,
					},
				});
			} else {
				// Still update last check time even if version check failed
				await prisma.settings.update({
					where: { id: settings.id },
					data: {
						last_update_check: new Date(),
					},
				});
			}

			// Invalidate settings cache so frontend gets fresh data
			invalidateCache();

			// ===== CREATE ALERTS =====
			// Check server update alert
			if (serverCurrentVersion && serverLatestVersion) {
				logger.info("🔍 Checking server update alert...");
				await this.checkAndCreateUpdateAlert(
					"server_update",
					serverCurrentVersion,
					serverLatestVersion,
					"Server Update Available",
					`A new server version (${serverLatestVersion}) is available. Current version: ${serverCurrentVersion}`,
				);
			} else {
				logger.warn(
					`⚠️ Skipping server update alert - current: ${serverCurrentVersion || "null"}, latest: ${serverLatestVersion || "null"}`,
				);
			}

			// Check agent update alert
			if (agentCurrentVersion && agentLatestVersion) {
				logger.info("🔍 Checking agent update alert...");

				// Ensure agent_update config exists (create default if missing)
				let agentUpdateConfig =
					await alertConfigService.getAlertConfigByType("agent_update");
				if (!agentUpdateConfig) {
					logger.info(
						`[version-update-check] Agent update config not found, creating default...`,
					);
					try {
						agentUpdateConfig = await alertConfigService.updateAlertConfig(
							"agent_update",
							{
								is_enabled: true,
								default_severity: "informational",
								auto_assign_enabled: false,
								notification_enabled: true,
								cleanup_resolved_only: true,
							},
						);
						logger.info(
							`[version-update-check] Created default agent_update config`,
						);
					} catch (createError) {
						logger.error(
							`[version-update-check] Failed to create agent_update config:`,
							createError,
						);
					}
				}

				await this.checkAndCreateUpdateAlert(
					"agent_update",
					agentCurrentVersion,
					agentLatestVersion,
					"Agent Files Update Available",
					`A new agent version (${agentLatestVersion}) is available. Current version: ${agentCurrentVersion}`,
				);
			} else {
				logger.warn(
					`⚠️ Skipping agent update alert - current: ${agentCurrentVersion || "null"}, latest: ${agentLatestVersion || "null"}`,
				);
				if (!agentCurrentVersion) {
					logger.info(
						"💡 Tip: Agent binary not found. Download agent binaries from the Automation page to enable agent update alerts.",
					);
				}
			}

			const executionTime = Date.now() - startTime;
			logger.info(
				`✅ Version update check completed in ${executionTime}ms - Server: ${serverCurrentVersion || "unknown"} -> ${serverLatestVersion || "unknown"} (${serverUpdateAvailable ? "update available" : "up to date"}), Agent: ${agentCurrentVersion || "unknown"} -> ${agentLatestVersion || "unknown"} (${agentUpdateAvailable ? "update available" : "up to date"})`,
			);

			return {
				success: true,
				server: {
					currentVersion: serverCurrentVersion,
					latestVersion: serverLatestVersion,
					isUpdateAvailable: serverUpdateAvailable,
				},
				agent: {
					currentVersion: agentCurrentVersion,
					latestVersion: agentLatestVersion,
					isUpdateAvailable: agentUpdateAvailable,
				},
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
