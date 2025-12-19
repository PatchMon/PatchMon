const { Queue, Worker } = require("bullmq");
const { redis, redisConnection } = require("./shared/redis");
const { prisma } = require("./shared/prisma");
const agentWs = require("../agentWs");
const { v4: uuidv4 } = require("uuid");
const { get_current_time } = require("../../utils/timezone");

// Import automation classes
const GitHubUpdateCheck = require("./githubUpdateCheck");
const SessionCleanup = require("./sessionCleanup");
const OrphanedRepoCleanup = require("./orphanedRepoCleanup");
const OrphanedPackageCleanup = require("./orphanedPackageCleanup");
const DockerInventoryCleanup = require("./dockerInventoryCleanup");
const DockerImageUpdateCheck = require("./dockerImageUpdateCheck");
const MetricsReporting = require("./metricsReporting");
const SystemStatistics = require("./systemStatistics");

// Queue names
const QUEUE_NAMES = {
	GITHUB_UPDATE_CHECK: "github-update-check",
	SESSION_CLEANUP: "session-cleanup",
	ORPHANED_REPO_CLEANUP: "orphaned-repo-cleanup",
	ORPHANED_PACKAGE_CLEANUP: "orphaned-package-cleanup",
	DOCKER_INVENTORY_CLEANUP: "docker-inventory-cleanup",
	DOCKER_IMAGE_UPDATE_CHECK: "docker-image-update-check",
	METRICS_REPORTING: "metrics-reporting",
	SYSTEM_STATISTICS: "system-statistics",
	AGENT_COMMANDS: "agent-commands",
};

/**
 * Main Queue Manager
 * Manages all BullMQ queues and workers
 */
class QueueManager {
	constructor() {
		this.queues = {};
		this.workers = {};
		this.automations = {};
		this.isInitialized = false;
	}

	/**
	 * Initialize all queues, workers, and automations
	 */
	async initialize() {
		try {
			console.log("✅ Redis connection successful");

			// Initialize queues
			await this.initializeQueues();

			// Initialize automation classes
			await this.initializeAutomations();

			// Initialize workers
			await this.initializeWorkers();

			// Setup event listeners
			this.setupEventListeners();

			this.isInitialized = true;
			console.log("✅ Queue manager initialized successfully");
		} catch (error) {
			console.error("❌ Failed to initialize queue manager:", error.message);
			throw error;
		}
	}

	/**
	 * Initialize all queues
	 */
	async initializeQueues() {
		for (const [_key, queueName] of Object.entries(QUEUE_NAMES)) {
			this.queues[queueName] = new Queue(queueName, {
				connection: redisConnection,
				defaultJobOptions: {
					removeOnComplete: 50, // Keep last 50 completed jobs
					removeOnFail: 20, // Keep last 20 failed jobs
					attempts: 3, // Retry failed jobs 3 times
					backoff: {
						type: "exponential",
						delay: 2000,
					},
				},
			});

			console.log(`✅ Queue '${queueName}' initialized`);
		}
	}

	/**
	 * Initialize automation classes
	 */
	async initializeAutomations() {
		this.automations[QUEUE_NAMES.GITHUB_UPDATE_CHECK] = new GitHubUpdateCheck(
			this,
		);
		this.automations[QUEUE_NAMES.SESSION_CLEANUP] = new SessionCleanup(this);
		this.automations[QUEUE_NAMES.ORPHANED_REPO_CLEANUP] =
			new OrphanedRepoCleanup(this);
		this.automations[QUEUE_NAMES.ORPHANED_PACKAGE_CLEANUP] =
			new OrphanedPackageCleanup(this);
		this.automations[QUEUE_NAMES.DOCKER_INVENTORY_CLEANUP] =
			new DockerInventoryCleanup(this);
		this.automations[QUEUE_NAMES.DOCKER_IMAGE_UPDATE_CHECK] =
			new DockerImageUpdateCheck(this);
		this.automations[QUEUE_NAMES.METRICS_REPORTING] = new MetricsReporting(
			this,
		);
		this.automations[QUEUE_NAMES.SYSTEM_STATISTICS] = new SystemStatistics(
			this,
		);

		console.log("✅ All automation classes initialized");
	}

	/**
	 * Initialize all workers
	 */
	async initializeWorkers() {
		// Optimized worker options to reduce Redis connections
		const workerOptions = {
			connection: redisConnection,
			concurrency: 1, // Keep concurrency low to reduce connections
			// Connection optimization
			maxStalledCount: 1,
			stalledInterval: 30000,
			// Reduce connection churn
			settings: {
				stalledInterval: 30000,
				maxStalledCount: 1,
			},
		};

		// GitHub Update Check Worker
		this.workers[QUEUE_NAMES.GITHUB_UPDATE_CHECK] = new Worker(
			QUEUE_NAMES.GITHUB_UPDATE_CHECK,
			this.automations[QUEUE_NAMES.GITHUB_UPDATE_CHECK].process.bind(
				this.automations[QUEUE_NAMES.GITHUB_UPDATE_CHECK],
			),
			workerOptions,
		);

		// Session Cleanup Worker
		this.workers[QUEUE_NAMES.SESSION_CLEANUP] = new Worker(
			QUEUE_NAMES.SESSION_CLEANUP,
			this.automations[QUEUE_NAMES.SESSION_CLEANUP].process.bind(
				this.automations[QUEUE_NAMES.SESSION_CLEANUP],
			),
			workerOptions,
		);

		// Orphaned Repo Cleanup Worker
		this.workers[QUEUE_NAMES.ORPHANED_REPO_CLEANUP] = new Worker(
			QUEUE_NAMES.ORPHANED_REPO_CLEANUP,
			this.automations[QUEUE_NAMES.ORPHANED_REPO_CLEANUP].process.bind(
				this.automations[QUEUE_NAMES.ORPHANED_REPO_CLEANUP],
			),
			workerOptions,
		);

		// Orphaned Package Cleanup Worker
		this.workers[QUEUE_NAMES.ORPHANED_PACKAGE_CLEANUP] = new Worker(
			QUEUE_NAMES.ORPHANED_PACKAGE_CLEANUP,
			this.automations[QUEUE_NAMES.ORPHANED_PACKAGE_CLEANUP].process.bind(
				this.automations[QUEUE_NAMES.ORPHANED_PACKAGE_CLEANUP],
			),
			workerOptions,
		);

		// Docker Inventory Cleanup Worker
		this.workers[QUEUE_NAMES.DOCKER_INVENTORY_CLEANUP] = new Worker(
			QUEUE_NAMES.DOCKER_INVENTORY_CLEANUP,
			this.automations[QUEUE_NAMES.DOCKER_INVENTORY_CLEANUP].process.bind(
				this.automations[QUEUE_NAMES.DOCKER_INVENTORY_CLEANUP],
			),
			workerOptions,
		);

		// Docker Image Update Check Worker
		this.workers[QUEUE_NAMES.DOCKER_IMAGE_UPDATE_CHECK] = new Worker(
			QUEUE_NAMES.DOCKER_IMAGE_UPDATE_CHECK,
			this.automations[QUEUE_NAMES.DOCKER_IMAGE_UPDATE_CHECK].process.bind(
				this.automations[QUEUE_NAMES.DOCKER_IMAGE_UPDATE_CHECK],
			),
			workerOptions,
		);

		// Metrics Reporting Worker
		this.workers[QUEUE_NAMES.METRICS_REPORTING] = new Worker(
			QUEUE_NAMES.METRICS_REPORTING,
			this.automations[QUEUE_NAMES.METRICS_REPORTING].process.bind(
				this.automations[QUEUE_NAMES.METRICS_REPORTING],
			),
			workerOptions,
		);

		// System Statistics Worker
		this.workers[QUEUE_NAMES.SYSTEM_STATISTICS] = new Worker(
			QUEUE_NAMES.SYSTEM_STATISTICS,
			this.automations[QUEUE_NAMES.SYSTEM_STATISTICS].process.bind(
				this.automations[QUEUE_NAMES.SYSTEM_STATISTICS],
			),
			workerOptions,
		);

		// Agent Commands Worker
		this.workers[QUEUE_NAMES.AGENT_COMMANDS] = new Worker(
			QUEUE_NAMES.AGENT_COMMANDS,
			async (job) => {
				const { api_id, type } = job.data;
				console.log(`Processing agent command: ${type} for ${api_id}`);

				// Log job to job_history
				let historyRecord = null;
				try {
					const host = await prisma.hosts.findUnique({
						where: { api_id },
						select: { id: true },
					});

					if (host) {
						historyRecord = await prisma.job_history.create({
							data: {
								id: uuidv4(),
								job_id: job.id,
								queue_name: QUEUE_NAMES.AGENT_COMMANDS,
								job_name: type,
								host_id: host.id,
								api_id: api_id,
								status: "active",
								attempt_number: job.attemptsMade + 1,
								created_at: get_current_time(),
								updated_at: get_current_time(),
							},
						});
						console.log(`📝 Logged job to job_history: ${job.id} (${type})`);
					}
				} catch (error) {
					console.error("Failed to log job to job_history:", error);
				}

				try {
					// Send command via WebSocket based on type
					if (type === "report_now") {
						agentWs.pushReportNow(api_id);
					} else if (type === "settings_update") {
						// For settings update, we need additional data
						const { update_interval } = job.data;
						agentWs.pushSettingsUpdate(api_id, update_interval);
					} else if (type === "update_agent") {
						// Check if bypass_settings flag is set (for true force updates)
						const bypassSettings = job.data.bypass_settings === true;

						if (!bypassSettings) {
							// Check general server auto_update setting
							const settings = await prisma.settings.findFirst();
							if (!settings || !settings.auto_update) {
								console.log(
									`⚠️ Auto-update is disabled in server settings, skipping update_agent command for agent ${api_id}`,
								);
								throw new Error("Auto-update is disabled in server settings");
							}

							// Check per-host auto_update setting
							const host = await prisma.hosts.findUnique({
								where: { api_id: api_id },
								select: { auto_update: true },
							});

							if (!host) {
								console.log(
									`⚠️ Host not found for agent ${api_id}, skipping update_agent command`,
								);
								throw new Error("Host not found");
							}

							if (!host.auto_update) {
								console.log(
									`⚠️ Auto-update is disabled for host ${api_id}, skipping update_agent command`,
								);
								throw new Error("Auto-update is disabled for this host");
							}
						}

						// Force agent to update by sending WebSocket command
						const ws = agentWs.getConnectionByApiId(api_id);
						if (ws && ws.readyState === 1) {
							// WebSocket.OPEN
							agentWs.pushUpdateAgent(api_id);
							console.log(`✅ Update command sent to agent ${api_id}`);
						} else {
							console.error(`❌ Agent ${api_id} is not connected`);
							throw new Error(
								`Agent ${api_id} is not connected. Cannot send update command.`,
							);
						}
					} else {
						console.error(`Unknown agent command type: ${type}`);
					}

					// Update job history to completed
					if (historyRecord) {
						await prisma.job_history.updateMany({
							where: { job_id: job.id },
							data: {
								status: "completed",
								completed_at: get_current_time(),
								updated_at: get_current_time(),
							},
						});
						console.log(`✅ Marked job as completed in job_history: ${job.id}`);
					}
				} catch (error) {
					// Update job history to failed
					if (historyRecord) {
						await prisma.job_history.updateMany({
							where: { job_id: job.id },
							data: {
								status: "failed",
								error_message: error.message,
								completed_at: get_current_time(),
								updated_at: get_current_time(),
							},
						});
						console.log(`❌ Marked job as failed in job_history: ${job.id}`);
					}
					throw error;
				}
			},
			workerOptions,
		);

		console.log(
			"✅ All workers initialized with optimized connection settings",
		);
	}

	/**
	 * Setup event listeners for all queues
	 */
	setupEventListeners() {
		for (const queueName of Object.values(QUEUE_NAMES)) {
			const queue = this.queues[queueName];
			queue.on("error", (error) => {
				console.error(`❌ Queue '${queueName}' experienced an error:`, error);
			});
			queue.on("failed", (job, err) => {
				console.error(
					`❌ Job '${job.id}' in queue '${queueName}' failed:`,
					err,
				);
			});
			queue.on("completed", (job) => {
				console.log(`✅ Job '${job.id}' in queue '${queueName}' completed.`);
			});
		}

		console.log("✅ Queue events initialized");
	}

	/**
	 * Schedule all recurring jobs
	 */
	async scheduleAllJobs() {
		await this.automations[QUEUE_NAMES.GITHUB_UPDATE_CHECK].schedule();
		await this.automations[QUEUE_NAMES.SESSION_CLEANUP].schedule();
		await this.automations[QUEUE_NAMES.ORPHANED_REPO_CLEANUP].schedule();
		await this.automations[QUEUE_NAMES.ORPHANED_PACKAGE_CLEANUP].schedule();
		await this.automations[QUEUE_NAMES.DOCKER_INVENTORY_CLEANUP].schedule();
		await this.automations[QUEUE_NAMES.DOCKER_IMAGE_UPDATE_CHECK].schedule();
		await this.automations[QUEUE_NAMES.METRICS_REPORTING].schedule();
		await this.automations[QUEUE_NAMES.SYSTEM_STATISTICS].schedule();
	}

	/**
	 * Manual job triggers
	 */
	async triggerGitHubUpdateCheck() {
		return this.automations[QUEUE_NAMES.GITHUB_UPDATE_CHECK].triggerManual();
	}

	async triggerSessionCleanup() {
		return this.automations[QUEUE_NAMES.SESSION_CLEANUP].triggerManual();
	}

	async triggerOrphanedRepoCleanup() {
		return this.automations[QUEUE_NAMES.ORPHANED_REPO_CLEANUP].triggerManual();
	}

	async triggerOrphanedPackageCleanup() {
		return this.automations[
			QUEUE_NAMES.ORPHANED_PACKAGE_CLEANUP
		].triggerManual();
	}

	async triggerDockerInventoryCleanup() {
		return this.automations[
			QUEUE_NAMES.DOCKER_INVENTORY_CLEANUP
		].triggerManual();
	}

	async triggerDockerImageUpdateCheck() {
		return this.automations[
			QUEUE_NAMES.DOCKER_IMAGE_UPDATE_CHECK
		].triggerManual();
	}

	async triggerSystemStatistics() {
		return this.automations[QUEUE_NAMES.SYSTEM_STATISTICS].triggerManual();
	}

	async triggerMetricsReporting() {
		return this.automations[QUEUE_NAMES.METRICS_REPORTING].triggerManual();
	}

	/**
	 * Get queue statistics
	 */
	async getQueueStats(queueName) {
		const queue = this.queues[queueName];
		if (!queue) {
			throw new Error(`Queue ${queueName} not found`);
		}

		const [waiting, active, completed, failed, delayed] = await Promise.all([
			queue.getWaiting(),
			queue.getActive(),
			queue.getCompleted(),
			queue.getFailed(),
			queue.getDelayed(),
		]);

		return {
			waiting: waiting.length,
			active: active.length,
			completed: completed.length,
			failed: failed.length,
			delayed: delayed.length,
		};
	}

	/**
	 * Get all queue statistics
	 */
	async getAllQueueStats() {
		const stats = {};
		for (const queueName of Object.values(QUEUE_NAMES)) {
			stats[queueName] = await this.getQueueStats(queueName);
		}
		return stats;
	}

	/**
	 * Get recent jobs for a queue
	 */
	async getRecentJobs(queueName, limit = 10) {
		const queue = this.queues[queueName];
		if (!queue) {
			throw new Error(`Queue ${queueName} not found`);
		}

		const [completed, failed] = await Promise.all([
			queue.getCompleted(0, limit - 1),
			queue.getFailed(0, limit - 1),
		]);

		return [...completed, ...failed]
			.sort((a, b) => new Date(b.finishedOn) - new Date(a.finishedOn))
			.slice(0, limit);
	}

	/**
	 * Get jobs for a specific host (by API ID)
	 */
	async getHostJobs(apiId, limit = 20) {
		const queue = this.queues[QUEUE_NAMES.AGENT_COMMANDS];
		if (!queue) {
			throw new Error(`Queue ${QUEUE_NAMES.AGENT_COMMANDS} not found`);
		}

		console.log(`[getHostJobs] Looking for jobs with api_id: ${apiId}`);

		// Get active queue status (waiting, active, delayed, failed)
		const [waiting, active, delayed, failed] = await Promise.all([
			queue.getWaiting(),
			queue.getActive(),
			queue.getDelayed(),
			queue.getFailed(),
		]);

		// Filter by API ID
		const filterByApiId = (jobs) =>
			jobs.filter((job) => job.data && job.data.api_id === apiId);

		const waitingCount = filterByApiId(waiting).length;
		const activeCount = filterByApiId(active).length;
		const delayedCount = filterByApiId(delayed).length;
		const failedCount = filterByApiId(failed).length;

		console.log(
			`[getHostJobs] Queue status - Waiting: ${waitingCount}, Active: ${activeCount}, Delayed: ${delayedCount}, Failed: ${failedCount}`,
		);

		// Get job history from database (shows all attempts and status changes)
		const jobHistory = await prisma.job_history.findMany({
			where: {
				api_id: apiId,
			},
			orderBy: {
				created_at: "desc",
			},
			take: limit,
		});

		console.log(
			`[getHostJobs] Found ${jobHistory.length} job history records for api_id: ${apiId}`,
		);

		return {
			waiting: waitingCount,
			active: activeCount,
			delayed: delayedCount,
			failed: failedCount,
			jobHistory: jobHistory.map((job) => ({
				id: job.id,
				job_id: job.job_id,
				job_name: job.job_name,
				status: job.status,
				attempt_number: job.attempt_number,
				error_message: job.error_message,
				output: job.output,
				created_at: job.created_at,
				updated_at: job.updated_at,
				completed_at: job.completed_at,
			})),
		};
	}

	/**
	 * Graceful shutdown
	 */
	async shutdown() {
		console.log("🛑 Shutting down queue manager...");

		for (const queueName of Object.keys(this.queues)) {
			try {
				await this.queues[queueName].close();
			} catch (e) {
				console.warn(
					`⚠️ Failed to close queue '${queueName}':`,
					e?.message || e,
				);
			}
			if (this.workers?.[queueName]) {
				try {
					await this.workers[queueName].close();
				} catch (e) {
					console.warn(
						`⚠️ Failed to close worker for '${queueName}':`,
						e?.message || e,
					);
				}
			}
		}

		await redis.quit();
		console.log("✅ Queue manager shutdown complete");
	}
}

const queueManager = new QueueManager();

module.exports = { queueManager, QUEUE_NAMES };
