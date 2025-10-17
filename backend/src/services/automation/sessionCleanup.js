const { prisma } = require("./shared/prisma");

/**
 * Session Cleanup Automation
 * Cleans up expired user sessions
 */
class SessionCleanup {
	constructor(queueManager) {
		this.queueManager = queueManager;
		this.queueName = "session-cleanup";
	}

	/**
	 * Process session cleanup job
	 */
	async process(_job) {
		const startTime = Date.now();
		console.log("🧹 Starting session cleanup...");

		try {
			const result = await prisma.user_sessions.deleteMany({
				where: {
					OR: [{ expires_at: { lt: new Date() } }, { is_revoked: true }],
				},
			});

			const executionTime = Date.now() - startTime;
			console.log(
				`✅ Session cleanup completed in ${executionTime}ms - Cleaned up ${result.count} expired sessions`,
			);

			return {
				success: true,
				sessionsCleaned: result.count,
				executionTime,
			};
		} catch (error) {
			const executionTime = Date.now() - startTime;
			console.error(
				`❌ Session cleanup failed after ${executionTime}ms:`,
				error.message,
			);
			throw error;
		}
	}

	/**
	 * Schedule recurring session cleanup (every hour)
	 */
	async schedule() {
		const job = await this.queueManager.queues[this.queueName].add(
			"session-cleanup",
			{},
			{
				repeat: { cron: "0 * * * *" }, // Every hour
				jobId: "session-cleanup-recurring",
			},
		);
		console.log("✅ Session cleanup scheduled");
		return job;
	}

	/**
	 * Trigger manual session cleanup
	 */
	async triggerManual() {
		const job = await this.queueManager.queues[this.queueName].add(
			"session-cleanup-manual",
			{},
			{ priority: 1 },
		);
		console.log("✅ Manual session cleanup triggered");
		return job;
	}
}

module.exports = SessionCleanup;
