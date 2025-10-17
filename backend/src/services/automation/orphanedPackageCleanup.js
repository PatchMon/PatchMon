const { prisma } = require("./shared/prisma");

/**
 * Orphaned Package Cleanup Automation
 * Removes packages with no associated hosts
 */
class OrphanedPackageCleanup {
	constructor(queueManager) {
		this.queueManager = queueManager;
		this.queueName = "orphaned-package-cleanup";
	}

	/**
	 * Process orphaned package cleanup job
	 */
	async process(_job) {
		const startTime = Date.now();
		console.log("🧹 Starting orphaned package cleanup...");

		try {
			// Find packages with 0 hosts
			const orphanedPackages = await prisma.packages.findMany({
				where: {
					host_packages: {
						none: {},
					},
				},
				include: {
					_count: {
						select: {
							host_packages: true,
						},
					},
				},
			});

			let deletedCount = 0;
			const deletedPackages = [];

			// Delete orphaned packages
			for (const pkg of orphanedPackages) {
				try {
					await prisma.packages.delete({
						where: { id: pkg.id },
					});
					deletedCount++;
					deletedPackages.push({
						id: pkg.id,
						name: pkg.name,
						description: pkg.description,
						category: pkg.category,
						latest_version: pkg.latest_version,
					});
					console.log(
						`🗑️ Deleted orphaned package: ${pkg.name} (${pkg.latest_version})`,
					);
				} catch (deleteError) {
					console.error(
						`❌ Failed to delete package ${pkg.id}:`,
						deleteError.message,
					);
				}
			}

			const executionTime = Date.now() - startTime;
			console.log(
				`✅ Orphaned package cleanup completed in ${executionTime}ms - Deleted ${deletedCount} packages`,
			);

			return {
				success: true,
				deletedCount,
				deletedPackages,
				executionTime,
			};
		} catch (error) {
			const executionTime = Date.now() - startTime;
			console.error(
				`❌ Orphaned package cleanup failed after ${executionTime}ms:`,
				error.message,
			);
			throw error;
		}
	}

	/**
	 * Schedule recurring orphaned package cleanup (daily at 3 AM)
	 */
	async schedule() {
		const job = await this.queueManager.queues[this.queueName].add(
			"orphaned-package-cleanup",
			{},
			{
				repeat: { cron: "0 3 * * *" }, // Daily at 3 AM
				jobId: "orphaned-package-cleanup-recurring",
			},
		);
		console.log("✅ Orphaned package cleanup scheduled");
		return job;
	}

	/**
	 * Trigger manual orphaned package cleanup
	 */
	async triggerManual() {
		const job = await this.queueManager.queues[this.queueName].add(
			"orphaned-package-cleanup-manual",
			{},
			{ priority: 1 },
		);
		console.log("✅ Manual orphaned package cleanup triggered");
		return job;
	}
}

module.exports = OrphanedPackageCleanup;
