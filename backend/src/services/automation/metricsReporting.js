const axios = require("axios");
const logger = require("../../utils/logger");
const { prisma } = require("./shared/prisma");
const { updateSettings } = require("../../services/settingsService");

const METRICS_API_URL =
	process.env.METRICS_API_URL || "https://metrics.patchmon.cloud";

/**
 * Metrics Reporting Automation
 * Sends anonymous usage metrics every 24 hours
 */
class MetricsReporting {
	constructor(queueManager) {
		this.queueManager = queueManager;
		this.queueName = "metrics-reporting";
	}

	/**
	 * Process metrics reporting job
	 */
	async process(_job, silent = false) {
		const startTime = Date.now();
		if (!silent) logger.info("📊 Starting metrics reporting...");

		try {
			// Fetch fresh settings directly from database (bypass cache)
			const settings = await prisma.settings.findFirst({
				orderBy: { updated_at: "desc" },
			});

			// Check if metrics are enabled
			if (settings.metrics_enabled !== true) {
				if (!silent) logger.info("📊 Metrics reporting is disabled");
				return { success: false, reason: "disabled" };
			}

			// Check if we have an anonymous ID
			if (!settings.metrics_anonymous_id) {
				if (!silent) logger.info("📊 No anonymous ID found, skipping metrics");
				return { success: false, reason: "no_id" };
			}

			// Get host count
			const hostCount = await prisma.hosts.count();

			// Get version
			const packageJson = require("../../../package.json");
			const version = packageJson.version;

			// Prepare metrics data
			const metricsData = {
				anonymous_id: settings.metrics_anonymous_id,
				host_count: hostCount,
				version,
			};

			if (!silent)
				logger.info(
					`📊 Sending metrics: ${hostCount} hosts, version ${version}`,
				);

			// Send to metrics API
			try {
				const response = await axios.post(
					`${METRICS_API_URL}/metrics/submit`,
					metricsData,
					{
						timeout: 10000,
						headers: {
							"Content-Type": "application/json",
						},
					},
				);

				// Update last sent timestamp
				await updateSettings(settings.id, {
					metrics_last_sent: new Date(),
				});

				const executionTime = Date.now() - startTime;
				if (!silent)
					logger.info(
						`✅ Metrics sent successfully in ${executionTime}ms:`,
						response.data,
					);

				return {
					success: true,
					data: response.data,
					hostCount,
					version,
					executionTime,
				};
			} catch (apiError) {
				const executionTime = Date.now() - startTime;
				if (!silent)
					logger.error(
						`❌ Failed to send metrics to API after ${executionTime}ms:`,
						apiError.message,
					);
				return {
					success: false,
					reason: "api_error",
					error: apiError.message,
					executionTime,
				};
			}
		} catch (error) {
			const executionTime = Date.now() - startTime;
			if (!silent)
				logger.error(
					`❌ Error in metrics reporting after ${executionTime}ms:`,
					error.message,
				);
			// Don't throw on silent mode, just return failure
			if (silent) {
				return {
					success: false,
					reason: "error",
					error: error.message,
					executionTime,
				};
			}
			throw error;
		}
	}

	/**
	 * Schedule recurring metrics reporting (daily at 2 AM)
	 */
	async schedule() {
		const job = await this.queueManager.queues[this.queueName].add(
			"metrics-reporting",
			{},
			{
				repeat: { cron: "0 2 * * *" }, // Daily at 2 AM
				jobId: "metrics-reporting-recurring",
			},
		);
		logger.info("✅ Metrics reporting scheduled (daily at 2 AM)");
		return job;
	}

	/**
	 * Trigger manual metrics reporting
	 */
	async triggerManual() {
		const job = await this.queueManager.queues[this.queueName].add(
			"metrics-reporting-manual",
			{},
			{ priority: 1 },
		);
		logger.info("✅ Manual metrics reporting triggered");
		return job;
	}

	/**
	 * Send metrics immediately (silent mode)
	 * Used for automatic sending on server startup
	 */
	async sendSilent() {
		try {
			const result = await this.process({ name: "startup-silent" }, true);
			return result;
		} catch (error) {
			// Silent failure on startup
			return { success: false, reason: "error", error: error.message };
		}
	}
}

module.exports = MetricsReporting;
