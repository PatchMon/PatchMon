const { prisma } = require("./shared/prisma");
const logger = require("../../utils/logger");

/**
 * Compliance Scan Cleanup Automation
 * Automatically stops and cleans up compliance scans running over 3 hours
 */
class ComplianceScanCleanup {
	constructor(queueManager) {
		this.queueManager = queueManager;
		this.queueName = "compliance-scan-cleanup";
		this.timeoutThresholdMs = 3 * 60 * 60 * 1000; // 3 hours in milliseconds
	}

	/**
	 * Process compliance scan cleanup job
	 */
	async process(_job) {
		const startTime = Date.now();
		logger.info("Starting compliance scan cleanup...");

		try {
			const thresholdTime = new Date(Date.now() - this.timeoutThresholdMs);

			// Find scans that have been running for more than 3 hours
			const stalledScans = await prisma.compliance_scans.findMany({
				where: {
					OR: [
						{ status: "running" },
						{ completed_at: null, status: { not: "failed" } },
					],
					started_at: {
						lt: thresholdTime,
					},
				},
				include: {
					hosts: {
						select: {
							id: true,
							hostname: true,
							friendly_name: true,
						},
					},
					compliance_profiles: {
						select: {
							name: true,
							type: true,
						},
					},
				},
			});

			if (stalledScans.length === 0) {
				const executionTime = Date.now() - startTime;
				logger.info(
					`Compliance scan cleanup completed in ${executionTime}ms - No stalled scans found`,
				);
				return {
					success: true,
					scansTerminated: 0,
					executionTime,
				};
			}

			// Update stalled scans to failed status
			const scanIds = stalledScans.map((scan) => scan.id);
			const result = await prisma.compliance_scans.updateMany({
				where: {
					id: {
						in: scanIds,
					},
				},
				data: {
					status: "failed",
					completed_at: new Date(),
					error_message:
						"Scan terminated automatically after running for more than 3 hours",
				},
			});

			// Log details about cleaned up scans
			for (const scan of stalledScans) {
				const hostName =
					scan.hosts?.friendly_name ||
					scan.hosts?.hostname ||
					scan.host_id ||
					"Unknown";
				const profileName = scan.compliance_profiles?.name || "Unknown profile";
				const runtime = Math.round(
					(Date.now() - new Date(scan.started_at).getTime()) / 1000 / 60,
				); // minutes

				logger.warn(
					`Terminated stalled compliance scan: Host="${hostName}", Profile="${profileName}", Runtime=${runtime}min, ScanID=${scan.id}`,
				);
			}

			const executionTime = Date.now() - startTime;
			logger.info(
				`Compliance scan cleanup completed in ${executionTime}ms - Terminated ${result.count} stalled scans`,
			);

			return {
				success: true,
				scansTerminated: result.count,
				scanDetails: stalledScans.map((s) => ({
					id: s.id,
					hostId: s.host_id,
					hostName: s.hosts?.friendly_name || s.hosts?.hostname || "Unknown",
					profileName: s.compliance_profiles?.name || "Unknown",
					startedAt: s.started_at,
					runtime: Math.round(
						(Date.now() - new Date(s.started_at).getTime()) / 1000 / 60,
					),
				})),
				executionTime,
			};
		} catch (error) {
			const executionTime = Date.now() - startTime;
			logger.error(
				`Compliance scan cleanup failed after ${executionTime}ms:`,
				error.message,
			);
			throw error;
		}
	}

	/**
	 * Schedule recurring compliance scan cleanup (daily at 1 AM)
	 */
	async schedule() {
		const job = await this.queueManager.queues[this.queueName].add(
			"compliance-scan-cleanup",
			{},
			{
				repeat: { cron: "0 1 * * *" }, // Daily at 1 AM
				jobId: "compliance-scan-cleanup-recurring",
			},
		);
		logger.info("Compliance scan cleanup scheduled (daily at 1 AM)");
		return job;
	}

	/**
	 * Trigger manual compliance scan cleanup
	 */
	async triggerManual() {
		const job = await this.queueManager.queues[this.queueName].add(
			"compliance-scan-cleanup-manual",
			{},
			{ priority: 1 },
		);
		logger.info("Manual compliance scan cleanup triggered");
		return job;
	}
}

module.exports = ComplianceScanCleanup;
