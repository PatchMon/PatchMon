const { prisma } = require("./shared/prisma");

/**
 * Orphaned Repository Cleanup Automation
 * Removes repositories with no associated hosts
 */
class OrphanedRepoCleanup {
	constructor(queueManager) {
		this.queueManager = queueManager;
		this.queueName = "orphaned-repo-cleanup";
	}

	/**
	 * Process orphaned repository cleanup job
	 */
	async process(_job) {
		const startTime = Date.now();
		console.log("🧹 Starting orphaned repository cleanup...");

		try {
			// Find repositories with 0 hosts
			const orphanedRepos = await prisma.repositories.findMany({
				where: {
					host_repositories: {
						none: {},
					},
				},
				include: {
					_count: {
						select: {
							host_repositories: true,
						},
					},
				},
			});

			let deletedCount = 0;
			const deletedRepos = [];

			// Delete orphaned repositories
			for (const repo of orphanedRepos) {
				try {
					await prisma.repositories.delete({
						where: { id: repo.id },
					});
					deletedCount++;
					deletedRepos.push({
						id: repo.id,
						name: repo.name,
						url: repo.url,
					});
					console.log(
						`🗑️ Deleted orphaned repository: ${repo.name} (${repo.url})`,
					);
				} catch (deleteError) {
					console.error(
						`❌ Failed to delete repository ${repo.id}:`,
						deleteError.message,
					);
				}
			}

			const executionTime = Date.now() - startTime;
			console.log(
				`✅ Orphaned repository cleanup completed in ${executionTime}ms - Deleted ${deletedCount} repositories`,
			);

			return {
				success: true,
				deletedCount,
				deletedRepos,
				executionTime,
			};
		} catch (error) {
			const executionTime = Date.now() - startTime;
			console.error(
				`❌ Orphaned repository cleanup failed after ${executionTime}ms:`,
				error.message,
			);
			throw error;
		}
	}

	/**
	 * Schedule recurring orphaned repository cleanup (daily at 2 AM)
	 */
	async schedule() {
		const job = await this.queueManager.queues[this.queueName].add(
			"orphaned-repo-cleanup",
			{},
			{
				repeat: { cron: "0 2 * * *" }, // Daily at 2 AM
				jobId: "orphaned-repo-cleanup-recurring",
			},
		);
		console.log("✅ Orphaned repository cleanup scheduled");
		return job;
	}

	/**
	 * Trigger manual orphaned repository cleanup
	 */
	async triggerManual() {
		const job = await this.queueManager.queues[this.queueName].add(
			"orphaned-repo-cleanup-manual",
			{},
			{ priority: 1 },
		);
		console.log("✅ Manual orphaned repository cleanup triggered");
		return job;
	}
}

module.exports = OrphanedRepoCleanup;
