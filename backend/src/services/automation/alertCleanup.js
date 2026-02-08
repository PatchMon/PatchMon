const { prisma } = require("./shared/prisma");
const logger = require("../../utils/logger");
const alertConfigService = require("../alertConfigService");

/**
 * Alert Cleanup Automation
 * Runs periodically to clean up old alerts based on retention policies
 */
class AlertCleanup {
	constructor(queueManager) {
		this.queueManager = queueManager;
		this.queueName = "alert-cleanup";
	}

	/**
	 * Process alert cleanup job
	 */
	async process(_job) {
		const startTime = Date.now();
		logger.info("🧹 Starting alert cleanup...");

		try {
			// Check if alerts system is enabled
			const alertService = require("../alertService");
			const alertsEnabled = await alertService.isAlertsEnabled();
			if (!alertsEnabled) {
				logger.info("⚠️ Alerts system is disabled, skipping alert cleanup");
				return {
					success: true,
					deletedCount: 0,
					autoResolvedCount: 0,
					executionTime: Date.now() - startTime,
					skipped: true,
				};
			}

			// Get alerts that should be cleaned up
			const alertsToCleanup = await alertConfigService.getAlertsToCleanup();
			let deletedCount = 0;
			let autoResolvedCount = 0;

			// Delete alerts that exceed retention
			for (const alert of alertsToCleanup) {
				try {
					await prisma.alerts.delete({
						where: { id: alert.id },
					});
					deletedCount++;
				} catch (error) {
					logger.error(`Failed to delete alert ${alert.id}:`, error);
				}
			}

			// Check for alerts that should be auto-resolved
			const configs = await prisma.alert_config.findMany({
				where: {
					auto_resolve_after_days: { not: null },
				},
			});

			for (const config of configs) {
				const cutoffDate = new Date();
				cutoffDate.setDate(cutoffDate.getDate() - config.auto_resolve_after_days);

				const alertsToResolve = await prisma.alerts.findMany({
					where: {
						type: config.alert_type,
						is_active: true,
						created_at: { lte: cutoffDate },
					},
				});

				for (const alert of alertsToResolve) {
					try {
						await prisma.alerts.update({
							where: { id: alert.id },
							data: {
								is_active: false,
								resolved_at: new Date(),
								updated_at: new Date(),
							},
						});

						// Record resolved action in history
						const alertService = require("../alertService");
						await alertService.performAlertAction(null, alert.id, "resolved", {
							reason: "auto_resolved",
							auto_resolve_after_days: config.auto_resolve_after_days,
						});

						autoResolvedCount++;
					} catch (error) {
						logger.error(`Failed to auto-resolve alert ${alert.id}:`, error);
					}
				}
			}

			const executionTime = Date.now() - startTime;
			logger.info(
				`✅ Alert cleanup completed in ${executionTime}ms - Deleted: ${deletedCount}, Auto-resolved: ${autoResolvedCount}`,
			);

			return {
				success: true,
				deletedCount,
				autoResolvedCount,
				executionTime,
			};
		} catch (error) {
			const executionTime = Date.now() - startTime;
			logger.error(
				`❌ Alert cleanup failed after ${executionTime}ms:`,
				error.message,
			);
			throw error;
		}
	}

	/**
	 * Schedule recurring alert cleanup (daily at 3 AM)
	 */
	async schedule() {
		const job = await this.queueManager.queues[this.queueName].add(
			"alert-cleanup",
			{},
			{
				repeat: { cron: "0 3 * * *" }, // Daily at 3 AM
				jobId: "alert-cleanup-recurring",
			},
		);
		logger.info("✅ Alert cleanup scheduled");
		return job;
	}

	/**
	 * Trigger manual alert cleanup
	 */
	async triggerManual() {
		const job = await this.queueManager.queues[this.queueName].add(
			"alert-cleanup-manual",
			{},
			{ priority: 1 },
		);
		logger.info("✅ Manual alert cleanup triggered");
		return job;
	}
}

module.exports = AlertCleanup;

