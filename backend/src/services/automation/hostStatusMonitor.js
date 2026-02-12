const { prisma } = require("./shared/prisma");
const logger = require("../../utils/logger");
const { subMinutes, isBefore } = require("date-fns");
const alertService = require("../alertService");
const alertConfigService = require("../alertConfigService");

/**
 * Host Status Monitor Automation
 * Monitors host status and creates alerts when hosts go offline
 * This is a fallback for hosts that don't use websockets
 */
class HostStatusMonitor {
	constructor(queueManager) {
		this.queueManager = queueManager;
		this.queueName = "host-status-monitor";
	}

	/**
	 * Process host status monitoring job
	 */
	async process(_job) {
		const startTime = Date.now();
		logger.info("🔍 Starting host status monitoring...");

		try {
			// Check if alerts system is enabled
			const { isAlertsEnabled } = require("../alertService");
			if (!(await isAlertsEnabled())) {
				logger.info(
					"⚠️ Alerts system is disabled, skipping host status monitoring",
				);
				return;
			}

			// Check if host_down alert type is enabled
			const hostDownConfig =
				await alertConfigService.getAlertConfigByType("host_down");
			if (!hostDownConfig || !hostDownConfig.is_enabled) {
				logger.info("⚠️ Host down alerts are disabled, skipping monitoring");
				return;
			}

			// Get settings to determine threshold
			const settings = await prisma.settings.findFirst();
			const updateIntervalMinutes = settings?.update_interval || 60;
			// Consider host offline if it hasn't reported in 3x the update interval
			const thresholdMinutes = updateIntervalMinutes * 3;

			// Find all hosts
			const hosts = await prisma.hosts.findMany({
				select: {
					id: true,
					friendly_name: true,
					hostname: true,
					api_id: true,
					last_update: true,
					status: true,
					host_down_alerts_enabled: true,
				},
			});

			const now = new Date();
			let alertsCreated = 0;
			let alertsUpdated = 0;

			for (const host of hosts) {
				// Check if host is stale (hasn't reported within threshold)
				const isStale = isBefore(
					new Date(host.last_update),
					subMinutes(now, thresholdMinutes),
				);

				// Only create alert if host is stale and was previously active
				if (isStale && host.status === "active") {
					// Check per-host setting: false = disabled, null = inherit, true = enabled
					let shouldCreateAlert = false;
					if (host.host_down_alerts_enabled === false) {
						// Explicitly disabled for this host
						shouldCreateAlert = false;
					} else if (host.host_down_alerts_enabled === true) {
						// Explicitly enabled for this host (overrides global)
						shouldCreateAlert = true;
					} else {
						// null = inherit from global config
						shouldCreateAlert = hostDownConfig?.is_enabled;
					}

					if (shouldCreateAlert) {
						// Check if alert already exists for this host
						// Fetch all host_down alerts and filter by metadata
						const allHostDownAlerts = await prisma.alerts.findMany({
							where: {
								type: "host_down",
								is_active: true,
							},
						});

						const existingAlert = allHostDownAlerts.find(
							(alert) => alert.metadata?.host_id === host.id,
						);

						if (existingAlert) {
							// Update existing alert
							await alertService.updateAlert(existingAlert.id, {
								updated_at: new Date(),
							});
							alertsUpdated++;
						} else {
							// Create new alert
							const severity = hostDownConfig.default_severity || "warning";
							const hostName =
								host.friendly_name || host.hostname || host.api_id;

							const newAlert = await alertService.createAlert(
								"host_down",
								severity,
								`Host ${hostName} is offline`,
								`Host "${hostName}" has not reported in ${thresholdMinutes} minutes. Last update: ${new Date(host.last_update).toLocaleString()}`,
								{
									host_id: host.id,
									host_name: hostName,
									last_update: host.last_update,
									threshold_minutes: thresholdMinutes,
								},
							);

							// Auto-assign if configured
							if (
								newAlert &&
								hostDownConfig.auto_assign_enabled &&
								hostDownConfig.auto_assign_user_id
							) {
								await alertService.assignAlertToUser(
									newAlert.id,
									hostDownConfig.auto_assign_user_id,
									null, // System assignment
								);
							}

							alertsCreated++;
						}
					}
				} else if (!isStale && host.status === "offline") {
					// Host came back online - resolve any existing alerts
					const allHostDownAlerts = await prisma.alerts.findMany({
						where: {
							type: "host_down",
							is_active: true,
						},
					});

					const existingAlert = allHostDownAlerts.find(
						(alert) => alert.metadata?.host_id === host.id,
					);

					if (existingAlert) {
						// Auto-resolve if configured
						if (hostDownConfig.auto_resolve_after_days === null) {
							// Only auto-resolve if auto_resolve_after_days is not set (immediate resolve)
							await alertService.performAlertAction(
								null, // System action
								existingAlert.id,
								"resolved",
								{
									resolved_reason: "Host came back online",
									system_action: true,
								},
							);
						}
					}
				}
			}

			const duration = Date.now() - startTime;
			logger.info(
				`✅ Host status monitoring completed in ${duration}ms - Created: ${alertsCreated}, Updated: ${alertsUpdated}`,
			);
		} catch (error) {
			logger.error(`❌ Host status monitoring failed:`, error);
			throw error;
		}
	}

	/**
	 * Schedule the host status monitoring job
	 */
	async schedule() {
		try {
			// Run every 5 minutes
			await this.queueManager.queues[this.queueName].add(
				"host-status-monitor",
				{},
				{
					repeat: {
						every: 5 * 60 * 1000, // 5 minutes
					},
					jobId: "host-status-monitor-recurring",
				},
			);

			logger.info(`✅ Scheduled host status monitoring (every 5 minutes)`);
		} catch (error) {
			logger.error(`❌ Failed to schedule host status monitoring:`, error);
			throw error;
		}
	}

	/**
	 * Manually trigger host status monitoring
	 */
	async triggerManual() {
		const job = await this.queueManager.queues[this.queueName].add(
			"host-status-monitor-manual",
			{},
			{ priority: 1 },
		);
		logger.info("✅ Manual host status monitor triggered");
		return job;
	}
}

module.exports = HostStatusMonitor;
