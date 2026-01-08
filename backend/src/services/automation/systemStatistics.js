const { prisma } = require("./shared/prisma");
const logger = require("../../utils/logger");
const { v4: uuidv4 } = require("uuid");

/**
 * System Statistics Collection Automation
 * Collects aggregated system-wide statistics every 30 minutes
 * for use in package trends charts
 */
class SystemStatistics {
	constructor(queueManager) {
		this.queueManager = queueManager;
		this.queueName = "system-statistics";
	}

	/**
	 * Process system statistics collection job
	 */
	async process(_job) {
		const startTime = Date.now();
		logger.info("📊 Starting system statistics collection...");

		try {
			// Calculate unique package counts across all hosts
			const uniquePackagesCount = await prisma.packages.count({
				where: {
					host_packages: {
						some: {
							needs_update: true,
						},
					},
				},
			});

			const uniqueSecurityCount = await prisma.packages.count({
				where: {
					host_packages: {
						some: {
							needs_update: true,
							is_security_update: true,
						},
					},
				},
			});

			// Calculate total unique packages installed on at least one host
			const totalPackages = await prisma.packages.count({
				where: {
					host_packages: {
						some: {}, // At least one host has this package
					},
				},
			});

			// Calculate total hosts
			const totalHosts = await prisma.hosts.count({
				where: {
					status: "active",
				},
			});

			// Calculate hosts needing updates (distinct hosts with packages needing updates)
			const hostsNeedingUpdates = await prisma.hosts.count({
				where: {
					status: "active",
					host_packages: {
						some: {
							needs_update: true,
						},
					},
				},
			});

			// Store statistics in database
			await prisma.system_statistics.create({
				data: {
					id: uuidv4(),
					unique_packages_count: uniquePackagesCount,
					unique_security_count: uniqueSecurityCount,
					total_packages: totalPackages,
					total_hosts: totalHosts,
					hosts_needing_updates: hostsNeedingUpdates,
					timestamp: new Date(),
				},
			});

			const executionTime = Date.now() - startTime;
			logger.info(
				`✅ System statistics collection completed in ${executionTime}ms - Unique packages: ${uniquePackagesCount}, Security: ${uniqueSecurityCount}, Total hosts: ${totalHosts}`,
			);

			return {
				success: true,
				uniquePackagesCount,
				uniqueSecurityCount,
				totalPackages,
				totalHosts,
				hostsNeedingUpdates,
				executionTime,
			};
		} catch (error) {
			const executionTime = Date.now() - startTime;
			logger.error(
				`❌ System statistics collection failed after ${executionTime}ms:`,
				error.message,
			);
			throw error;
		}
	}

	/**
	 * Schedule recurring system statistics collection (every 30 minutes)
	 */
	async schedule() {
		const job = await this.queueManager.queues[this.queueName].add(
			"system-statistics",
			{},
			{
				repeat: { pattern: "*/30 * * * *" }, // Every 30 minutes
				jobId: "system-statistics-recurring",
			},
		);
		logger.info("✅ System statistics collection scheduled (every 30 minutes)");
		return job;
	}

	/**
	 * Trigger manual system statistics collection
	 */
	async triggerManual() {
		const job = await this.queueManager.queues[this.queueName].add(
			"system-statistics-manual",
			{},
			{ priority: 1 },
		);
		logger.info("✅ Manual system statistics collection triggered");
		return job;
	}
}

module.exports = SystemStatistics;
