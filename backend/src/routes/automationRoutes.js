const express = require("express");
const logger = require("../utils/logger");
const crypto = require("node:crypto");
const { queueManager, QUEUE_NAMES } = require("../services/automation");
const { getConnectedApiIds } = require("../services/agentWs");
const { authenticateToken, requireAdmin } = require("../middleware/auth");
const { redis } = require("../services/automation/shared/redis");

const router = express.Router();

// Bull Board Ticket - One-time use ticket for Bull Board authentication
// SECURITY: Uses Redis-stored tickets instead of tokens in URLs to prevent exposure in logs
const BULL_BOARD_TICKET_PREFIX = "bullboard:ticket:";
const BULL_BOARD_TICKET_TTL = 30; // 30 seconds - very short-lived

router.post("/bullboard-ticket", authenticateToken, requireAdmin, async (req, res) => {
	try {
		// Generate a random ticket
		const ticket = crypto.randomBytes(32).toString("hex");
		const key = `${BULL_BOARD_TICKET_PREFIX}${ticket}`;

		// Store ticket data in Redis with short TTL
		const ticketData = JSON.stringify({
			userId: req.user.id,
			sessionId: req.session_id,
			role: req.user.role,
			createdAt: Date.now(),
		});

		await redis.setex(key, BULL_BOARD_TICKET_TTL, ticketData);

		res.json({
			ticket: ticket,
			expiresIn: BULL_BOARD_TICKET_TTL,
		});
	} catch (error) {
		logger.error("Bull Board ticket generation error:", error);
		res.status(500).json({ error: "Failed to generate Bull Board ticket" });
	}
});

// Validate and consume Bull Board ticket (exported for use in server.js)
async function consumeBullBoardTicket(ticket) {
	const key = `${BULL_BOARD_TICKET_PREFIX}${ticket}`;
	const data = await redis.get(key);

	if (!data) {
		return { valid: false, reason: "Invalid or expired ticket" };
	}

	// Immediately delete the ticket (one-time use)
	await redis.del(key);

	try {
		const ticketData = JSON.parse(data);

		// Verify the ticket hasn't been tampered with
		if (!ticketData.userId || !ticketData.role) {
			return { valid: false, reason: "Malformed ticket data" };
		}

		// Check if admin role
		if (ticketData.role !== "admin") {
			return { valid: false, reason: "Admin access required" };
		}

		return {
			valid: true,
			userId: ticketData.userId,
			sessionId: ticketData.sessionId,
			role: ticketData.role,
		};
	} catch (error) {
		return { valid: false, reason: "Failed to parse ticket data" };
	}
}

// Get all queue statistics
router.get("/stats", authenticateToken, async (_req, res) => {
	try {
		const stats = await queueManager.getAllQueueStats();
		res.json({
			success: true,
			data: stats,
		});
	} catch (error) {
		logger.error("Error fetching queue stats:", error);
		res.status(500).json({
			success: false,
			error: "Failed to fetch queue statistics",
		});
	}
});

// Get specific queue statistics
router.get("/stats/:queueName", authenticateToken, async (req, res) => {
	try {
		const { queueName } = req.params;

		if (!Object.values(QUEUE_NAMES).includes(queueName)) {
			return res.status(400).json({
				success: false,
				error: "Invalid queue name",
			});
		}

		const stats = await queueManager.getQueueStats(queueName);
		res.json({
			success: true,
			data: stats,
		});
	} catch (error) {
		logger.error("Error fetching queue stats:", error);
		res.status(500).json({
			success: false,
			error: "Failed to fetch queue statistics",
		});
	}
});

// Get recent jobs for a queue
router.get("/jobs/:queueName", authenticateToken, async (req, res) => {
	try {
		const { queueName } = req.params;
		const { limit = 10 } = req.query;

		if (!Object.values(QUEUE_NAMES).includes(queueName)) {
			return res.status(400).json({
				success: false,
				error: "Invalid queue name",
			});
		}

		const jobs = await queueManager.getRecentJobs(
			queueName,
			parseInt(limit, 10),
		);

		// Format jobs for frontend
		const formattedJobs = jobs.map((job) => ({
			id: job.id,
			name: job.name,
			status: job.finishedOn
				? job.failedReason
					? "failed"
					: "completed"
				: "active",
			progress: job.progress,
			data: job.data,
			returnvalue: job.returnvalue,
			failedReason: job.failedReason,
			processedOn: job.processedOn,
			finishedOn: job.finishedOn,
			createdAt: new Date(job.timestamp),
			attemptsMade: job.attemptsMade,
			delay: job.delay,
		}));

		res.json({
			success: true,
			data: formattedJobs,
		});
	} catch (error) {
		logger.error("Error fetching recent jobs:", error);
		res.status(500).json({
			success: false,
			error: "Failed to fetch recent jobs",
		});
	}
});

// Trigger manual GitHub update check
router.post("/trigger/github-update", authenticateToken, async (_req, res) => {
	try {
		const job = await queueManager.triggerGitHubUpdateCheck();
		res.json({
			success: true,
			data: {
				jobId: job.id,
				message: "GitHub update check triggered successfully",
			},
		});
	} catch (error) {
		logger.error("Error triggering GitHub update check:", error);
		res.status(500).json({
			success: false,
			error: "Failed to trigger GitHub update check",
		});
	}
});

// Trigger manual session cleanup
router.post(
	"/trigger/session-cleanup",
	authenticateToken,
	async (_req, res) => {
		try {
			const job = await queueManager.triggerSessionCleanup();
			res.json({
				success: true,
				data: {
					jobId: job.id,
					message: "Session cleanup triggered successfully",
				},
			});
		} catch (error) {
			logger.error("Error triggering session cleanup:", error);
			res.status(500).json({
				success: false,
				error: "Failed to trigger session cleanup",
			});
		}
	},
);

// Trigger Agent Collection: enqueue report_now for connected agents only
router.post(
	"/trigger/agent-collection",
	authenticateToken,
	async (_req, res) => {
		try {
			const queue = queueManager.queues[QUEUE_NAMES.AGENT_COMMANDS];
			const apiIds = getConnectedApiIds();
			if (!apiIds || apiIds.length === 0) {
				return res.json({ success: true, data: { enqueued: 0 } });
			}
			const jobs = apiIds.map((apiId) => ({
				name: "report_now",
				data: { api_id: apiId, type: "report_now" },
				opts: { attempts: 3, backoff: { type: "fixed", delay: 2000 } },
			}));
			await queue.addBulk(jobs);
			res.json({ success: true, data: { enqueued: jobs.length } });
		} catch (error) {
			logger.error("Error triggering agent collection:", error);
			res
				.status(500)
				.json({ success: false, error: "Failed to trigger agent collection" });
		}
	},
);

// Trigger manual orphaned repo cleanup
router.post(
	"/trigger/orphaned-repo-cleanup",
	authenticateToken,
	async (_req, res) => {
		try {
			const job = await queueManager.triggerOrphanedRepoCleanup();
			res.json({
				success: true,
				data: {
					jobId: job.id,
					message: "Orphaned repository cleanup triggered successfully",
				},
			});
		} catch (error) {
			logger.error("Error triggering orphaned repository cleanup:", error);
			res.status(500).json({
				success: false,
				error: "Failed to trigger orphaned repository cleanup",
			});
		}
	},
);

// Trigger manual orphaned package cleanup
router.post(
	"/trigger/orphaned-package-cleanup",
	authenticateToken,
	async (_req, res) => {
		try {
			const job = await queueManager.triggerOrphanedPackageCleanup();
			res.json({
				success: true,
				data: {
					jobId: job.id,
					message: "Orphaned package cleanup triggered successfully",
				},
			});
		} catch (error) {
			logger.error("Error triggering orphaned package cleanup:", error);
			res.status(500).json({
				success: false,
				error: "Failed to trigger orphaned package cleanup",
			});
		}
	},
);

// Trigger manual Docker inventory cleanup
router.post(
	"/trigger/docker-inventory-cleanup",
	authenticateToken,
	async (_req, res) => {
		try {
			const job = await queueManager.triggerDockerInventoryCleanup();
			res.json({
				success: true,
				data: {
					jobId: job.id,
					message: "Docker inventory cleanup triggered successfully",
				},
			});
		} catch (error) {
			logger.error("Error triggering Docker inventory cleanup:", error);
			res.status(500).json({
				success: false,
				error: "Failed to trigger Docker inventory cleanup",
			});
		}
	},
);

// Trigger manual system statistics collection
router.post(
	"/trigger/system-statistics",
	authenticateToken,
	async (_req, res) => {
		try {
			const job = await queueManager.triggerSystemStatistics();
			res.json({
				success: true,
				data: {
					jobId: job.id,
					message: "System statistics collection triggered successfully",
				},
			});
		} catch (error) {
			logger.error("Error triggering system statistics collection:", error);
			res.status(500).json({
				success: false,
				error: "Failed to trigger system statistics collection",
			});
		}
	},
);

// Get queue health status
router.get("/health", authenticateToken, async (_req, res) => {
	try {
		const stats = await queueManager.getAllQueueStats();
		const totalJobs = Object.values(stats).reduce((sum, queueStats) => {
			return sum + queueStats.waiting + queueStats.active + queueStats.failed;
		}, 0);

		const health = {
			status: "healthy",
			totalJobs,
			queues: Object.keys(stats).length,
			timestamp: new Date().toISOString(),
		};

		// Check for unhealthy conditions
		if (totalJobs > 1000) {
			health.status = "warning";
			health.message = "High number of queued jobs";
		}

		const failedJobs = Object.values(stats).reduce((sum, queueStats) => {
			return sum + queueStats.failed;
		}, 0);

		if (failedJobs > 10) {
			health.status = "error";
			health.message = "High number of failed jobs";
		}

		res.json({
			success: true,
			data: health,
		});
	} catch (error) {
		logger.error("Error checking queue health:", error);
		res.status(500).json({
			success: false,
			error: "Failed to check queue health",
		});
	}
});

// Get automation overview (for dashboard cards)
router.get("/overview", authenticateToken, async (_req, res) => {
	try {
		const stats = await queueManager.getAllQueueStats();
		const { getSettings } = require("../services/settingsService");
		const settings = await getSettings();

		// Get recent jobs for each queue to show last run times
		const recentJobs = await Promise.all([
			queueManager.getRecentJobs(QUEUE_NAMES.GITHUB_UPDATE_CHECK, 1),
			queueManager.getRecentJobs(QUEUE_NAMES.SESSION_CLEANUP, 1),
			queueManager.getRecentJobs(QUEUE_NAMES.ORPHANED_REPO_CLEANUP, 1),
			queueManager.getRecentJobs(QUEUE_NAMES.ORPHANED_PACKAGE_CLEANUP, 1),
			queueManager.getRecentJobs(QUEUE_NAMES.DOCKER_INVENTORY_CLEANUP, 1),
			queueManager.getRecentJobs(QUEUE_NAMES.AGENT_COMMANDS, 1),
			queueManager.getRecentJobs(QUEUE_NAMES.SYSTEM_STATISTICS, 1),
		]);

		// Calculate overview metrics
		const overview = {
			scheduledTasks:
				stats[QUEUE_NAMES.GITHUB_UPDATE_CHECK].delayed +
				stats[QUEUE_NAMES.SESSION_CLEANUP].delayed +
				stats[QUEUE_NAMES.ORPHANED_REPO_CLEANUP].delayed +
				stats[QUEUE_NAMES.ORPHANED_PACKAGE_CLEANUP].delayed +
				stats[QUEUE_NAMES.DOCKER_INVENTORY_CLEANUP].delayed +
				stats[QUEUE_NAMES.SYSTEM_STATISTICS].delayed,

			runningTasks:
				stats[QUEUE_NAMES.GITHUB_UPDATE_CHECK].active +
				stats[QUEUE_NAMES.SESSION_CLEANUP].active +
				stats[QUEUE_NAMES.ORPHANED_REPO_CLEANUP].active +
				stats[QUEUE_NAMES.ORPHANED_PACKAGE_CLEANUP].active +
				stats[QUEUE_NAMES.DOCKER_INVENTORY_CLEANUP].active +
				stats[QUEUE_NAMES.SYSTEM_STATISTICS].active,

			failedTasks:
				stats[QUEUE_NAMES.GITHUB_UPDATE_CHECK].failed +
				stats[QUEUE_NAMES.SESSION_CLEANUP].failed +
				stats[QUEUE_NAMES.ORPHANED_REPO_CLEANUP].failed +
				stats[QUEUE_NAMES.ORPHANED_PACKAGE_CLEANUP].failed +
				stats[QUEUE_NAMES.DOCKER_INVENTORY_CLEANUP].failed +
				stats[QUEUE_NAMES.SYSTEM_STATISTICS].failed,

			totalAutomations: Object.values(stats).reduce((sum, queueStats) => {
				return (
					sum +
					queueStats.completed +
					queueStats.failed +
					queueStats.active +
					queueStats.waiting +
					queueStats.delayed
				);
			}, 0),

			// Automation details with last run times
			automations: [
				{
					name: "GitHub Update Check",
					queue: QUEUE_NAMES.GITHUB_UPDATE_CHECK,
					description: "Checks for new PatchMon releases",
					schedule: "Daily at midnight",
					lastRun: recentJobs[0][0]?.finishedOn
						? new Date(recentJobs[0][0].finishedOn).toLocaleString()
						: "Never",
					lastRunTimestamp: recentJobs[0][0]?.finishedOn || 0,
					status: recentJobs[0][0]?.failedReason
						? "Failed"
						: recentJobs[0][0]
							? "Success"
							: "Never run",
					stats: stats[QUEUE_NAMES.GITHUB_UPDATE_CHECK],
				},
				{
					name: "Session Cleanup",
					queue: QUEUE_NAMES.SESSION_CLEANUP,
					description: "Cleans up expired user sessions",
					schedule: "Every hour",
					lastRun: recentJobs[1][0]?.finishedOn
						? new Date(recentJobs[1][0].finishedOn).toLocaleString()
						: "Never",
					lastRunTimestamp: recentJobs[1][0]?.finishedOn || 0,
					status: recentJobs[1][0]?.failedReason
						? "Failed"
						: recentJobs[1][0]
							? "Success"
							: "Never run",
					stats: stats[QUEUE_NAMES.SESSION_CLEANUP],
				},
				{
					name: "Orphaned Repo Cleanup",
					queue: QUEUE_NAMES.ORPHANED_REPO_CLEANUP,
					description: "Removes repositories with no associated hosts",
					schedule: "Daily at 2 AM",
					lastRun: recentJobs[2][0]?.finishedOn
						? new Date(recentJobs[2][0].finishedOn).toLocaleString()
						: "Never",
					lastRunTimestamp: recentJobs[2][0]?.finishedOn || 0,
					status: recentJobs[2][0]?.failedReason
						? "Failed"
						: recentJobs[2][0]
							? "Success"
							: "Never run",
					stats: stats[QUEUE_NAMES.ORPHANED_REPO_CLEANUP],
				},
				{
					name: "Orphaned Package Cleanup",
					queue: QUEUE_NAMES.ORPHANED_PACKAGE_CLEANUP,
					description: "Removes packages with no associated hosts",
					schedule: "Daily at 3 AM",
					lastRun: recentJobs[3][0]?.finishedOn
						? new Date(recentJobs[3][0].finishedOn).toLocaleString()
						: "Never",
					lastRunTimestamp: recentJobs[3][0]?.finishedOn || 0,
					status: recentJobs[3][0]?.failedReason
						? "Failed"
						: recentJobs[3][0]
							? "Success"
							: "Never run",
					stats: stats[QUEUE_NAMES.ORPHANED_PACKAGE_CLEANUP],
				},
				{
					name: "Docker Inventory Cleanup",
					queue: QUEUE_NAMES.DOCKER_INVENTORY_CLEANUP,
					description:
						"Removes Docker containers and images for non-existent hosts",
					schedule: "Daily at 4 AM",
					lastRun: recentJobs[4][0]?.finishedOn
						? new Date(recentJobs[4][0].finishedOn).toLocaleString()
						: "Never",
					lastRunTimestamp: recentJobs[4][0]?.finishedOn || 0,
					status: recentJobs[4][0]?.failedReason
						? "Failed"
						: recentJobs[4][0]
							? "Success"
							: "Never run",
					stats: stats[QUEUE_NAMES.DOCKER_INVENTORY_CLEANUP],
				},
				{
					name: "Collect Host Statistics",
					queue: QUEUE_NAMES.AGENT_COMMANDS,
					description: "Collects package statistics from connected agents only",
					schedule: `Every ${settings.update_interval} minutes (Agent-driven)`,
					lastRun: recentJobs[5][0]?.finishedOn
						? new Date(recentJobs[5][0].finishedOn).toLocaleString()
						: "Never",
					lastRunTimestamp: recentJobs[5][0]?.finishedOn || 0,
					status: recentJobs[5][0]?.failedReason
						? "Failed"
						: recentJobs[5][0]
							? "Success"
							: "Never run",
					stats: stats[QUEUE_NAMES.AGENT_COMMANDS],
				},
				{
					name: "System Statistics Collection",
					queue: QUEUE_NAMES.SYSTEM_STATISTICS,
					description: "Collects aggregated system-wide package statistics",
					schedule: "Every 30 minutes",
					lastRun: recentJobs[6][0]?.finishedOn
						? new Date(recentJobs[6][0].finishedOn).toLocaleString()
						: "Never",
					lastRunTimestamp: recentJobs[6][0]?.finishedOn || 0,
					status: recentJobs[6][0]?.failedReason
						? "Failed"
						: recentJobs[6][0]
							? "Success"
							: "Never run",
					stats: stats[QUEUE_NAMES.SYSTEM_STATISTICS],
				},
			].sort((a, b) => {
				// Sort by last run timestamp (most recent first)
				// If both have never run (timestamp 0), maintain original order
				if (a.lastRunTimestamp === 0 && b.lastRunTimestamp === 0) return 0;
				if (a.lastRunTimestamp === 0) return 1; // Never run goes to bottom
				if (b.lastRunTimestamp === 0) return -1; // Never run goes to bottom
				return b.lastRunTimestamp - a.lastRunTimestamp; // Most recent first
			}),
		};

		res.json({
			success: true,
			data: overview,
		});
	} catch (error) {
		logger.error("Error fetching automation overview:", error);
		res.status(500).json({
			success: false,
			error: "Failed to fetch automation overview",
		});
	}
});

module.exports = router;
module.exports.consumeBullBoardTicket = consumeBullBoardTicket;
