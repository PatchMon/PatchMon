const { Queue, Worker, DelayedError } = require("bullmq");
const logger = require("../../utils/logger");
const { redis, redisConnection } = require("./shared/redis");
const { prisma } = require("./shared/prisma");
const agentWs = require("../agentWs");
const { v4: uuidv4 } = require("uuid");
const { get_current_time } = require("../../utils/timezone");

const COMPLIANCE_INSTALL_JOB_PREFIX = "compliance_install_job:";
const COMPLIANCE_INSTALL_CANCEL_PREFIX = "compliance_install_cancel:";
const _COMPLIANCE_INSTALL_JOB_TTL = 3600; // 1 hour
const COMPLIANCE_POLL_INTERVAL_MS = 2500;
const COMPLIANCE_INSTALL_TIMEOUT_MS = 5 * 60 * 1000; // 5 min

// Import automation classes
const VersionUpdateCheck = require("./versionUpdateCheck");
const SessionCleanup = require("./sessionCleanup");
const OrphanedRepoCleanup = require("./orphanedRepoCleanup");
const OrphanedPackageCleanup = require("./orphanedPackageCleanup");
const DockerInventoryCleanup = require("./dockerInventoryCleanup");
const DockerImageUpdateCheck = require("./dockerImageUpdateCheck");
const MetricsReporting = require("./metricsReporting");
const SystemStatistics = require("./systemStatistics");
const AlertCleanup = require("./alertCleanup");
const HostStatusMonitor = require("./hostStatusMonitor");

// Queue names
const QUEUE_NAMES = {
	VERSION_UPDATE_CHECK: "version-update-check",
	SESSION_CLEANUP: "session-cleanup",
	ORPHANED_REPO_CLEANUP: "orphaned-repo-cleanup",
	ORPHANED_PACKAGE_CLEANUP: "orphaned-package-cleanup",
	DOCKER_INVENTORY_CLEANUP: "docker-inventory-cleanup",
	DOCKER_IMAGE_UPDATE_CHECK: "docker-image-update-check",
	METRICS_REPORTING: "metrics-reporting",
	SYSTEM_STATISTICS: "system-statistics",
	AGENT_COMMANDS: "agent-commands",
	ALERT_CLEANUP: "alert-cleanup",
	HOST_STATUS_MONITOR: "host-status-monitor",
	COMPLIANCE: "compliance",
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
			logger.info("✅ Redis connection successful");

			// Initialize queues
			await this.initializeQueues();

			// Initialize automation classes
			await this.initializeAutomations();

			// Initialize workers
			await this.initializeWorkers();

			// Setup event listeners
			this.setupEventListeners();

			this.isInitialized = true;
			logger.info("✅ Queue manager initialized successfully");
		} catch (error) {
			logger.error("❌ Failed to initialize queue manager:", error.message);
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

			logger.info(`✅ Queue '${queueName}' initialized`);
		}
	}

	/**
	 * Initialize automation classes
	 */
	async initializeAutomations() {
		this.automations[QUEUE_NAMES.VERSION_UPDATE_CHECK] = new VersionUpdateCheck(
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
		this.automations[QUEUE_NAMES.ALERT_CLEANUP] = new AlertCleanup(this);
		this.automations[QUEUE_NAMES.HOST_STATUS_MONITOR] = new HostStatusMonitor(
			this,
		);

		logger.info("✅ All automation classes initialized");
	}

	/**
	 * Initialize all workers
	 */
	async initializeWorkers() {
		// Lock duration settings (in milliseconds)
		// Default: 120 seconds - increased to handle network latency and slow Redis operations
		// Must be significantly longer than lockRenewTime to allow for timeouts
		const lockDuration =
			parseInt(process.env.BULLMQ_LOCK_DURATION_MS, 10) || 120000;
		// Lock renewal time (in milliseconds) - should be less than lockDuration and commandTimeout
		// Default: 20 seconds - renews lock well before it expires, giving time for slow Redis operations
		const lockRenewTime =
			parseInt(process.env.BULLMQ_LOCK_RENEW_TIME_MS, 10) || 20000;

		// Optimized worker options to reduce Redis connections
		const workerOptions = {
			connection: redisConnection,
			concurrency: 1, // Keep concurrency low to reduce connections
			// Lock settings - critical for preventing "Missing lock" errors
			lockDuration: lockDuration, // How long a job can run before lock expires
			lockRenewTime: lockRenewTime, // How often to renew the lock
			// Connection optimization
			maxStalledCount: 1,
			stalledInterval: 30000,
			// Reduce connection churn
			settings: {
				stalledInterval: 30000,
				maxStalledCount: 1,
			},
		};

		logger.info(
			`Worker lock configuration: lockDuration=${lockDuration}ms, lockRenewTime=${lockRenewTime}ms`,
		);

		// Version Update Check Worker
		this.workers[QUEUE_NAMES.VERSION_UPDATE_CHECK] = new Worker(
			QUEUE_NAMES.VERSION_UPDATE_CHECK,
			this.automations[QUEUE_NAMES.VERSION_UPDATE_CHECK].process.bind(
				this.automations[QUEUE_NAMES.VERSION_UPDATE_CHECK],
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

		// Alert Cleanup Worker
		this.workers[QUEUE_NAMES.ALERT_CLEANUP] = new Worker(
			QUEUE_NAMES.ALERT_CLEANUP,
			this.automations[QUEUE_NAMES.ALERT_CLEANUP].process.bind(
				this.automations[QUEUE_NAMES.ALERT_CLEANUP],
			),
			workerOptions,
		);

		// Host Status Monitor Worker
		this.workers[QUEUE_NAMES.HOST_STATUS_MONITOR] = new Worker(
			QUEUE_NAMES.HOST_STATUS_MONITOR,
			this.automations[QUEUE_NAMES.HOST_STATUS_MONITOR].process.bind(
				this.automations[QUEUE_NAMES.HOST_STATUS_MONITOR],
			),
			workerOptions,
		);

		// Agent Commands Worker
		this.workers[QUEUE_NAMES.AGENT_COMMANDS] = new Worker(
			QUEUE_NAMES.AGENT_COMMANDS,
			async (job) => {
				const { api_id, type } = job.data;
				logger.info(`Processing agent command: ${type} for ${api_id}`);

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
						logger.info(`📝 Logged job to job_history: ${job.id} (${type})`);
					}
				} catch (error) {
					logger.error("Failed to log job to job_history:", error);
				}

				try {
					// Send command via WebSocket based on type
					if (type === "report_now") {
						agentWs.pushReportNow(api_id);
						logger.info(
							`Collect host statistics: report_now sent to agent ${api_id}`,
						);
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
								logger.info(
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
								logger.info(
									`⚠️ Host not found for agent ${api_id}, skipping update_agent command`,
								);
								throw new Error("Host not found");
							}

							if (!host.auto_update) {
								logger.info(
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
							logger.info(`✅ Update command sent to agent ${api_id}`);
						} else {
							logger.error(`❌ Agent ${api_id} is not connected`);
							throw new Error(
								`Agent ${api_id} is not connected. Cannot send update command.`,
							);
						}
					} else if (type === "refresh_integration_status") {
						// Request agent to refresh and report integration status
						const ws = agentWs.getConnectionByApiId(api_id);
						if (ws && ws.readyState === 1) {
							// WebSocket.OPEN
							agentWs.pushRefreshIntegrationStatus(api_id);
							logger.info(
								`✅ Refresh integration status command sent to agent ${api_id}`,
							);
						} else {
							logger.error(`❌ Agent ${api_id} is not connected`);
							throw new Error(
								`Agent ${api_id} is not connected. Cannot refresh integration status.`,
							);
						}
					} else if (type === "docker_inventory_refresh") {
						// Request agent to refresh and report Docker inventory
						const ws = agentWs.getConnectionByApiId(api_id);
						if (ws && ws.readyState === 1) {
							// WebSocket.OPEN
							agentWs.pushDockerInventoryRefresh(api_id);
							logger.info(
								`✅ Docker inventory refresh command sent to agent ${api_id}`,
							);
						} else {
							logger.error(`❌ Agent ${api_id} is not connected`);
							throw new Error(
								`Agent ${api_id} is not connected. Cannot refresh Docker inventory.`,
							);
						}
					} else {
						logger.error(`Unknown agent command type: ${type}`);
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
						logger.info(`✅ Marked job as completed in job_history: ${job.id}`);
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
						logger.info(`❌ Marked job as failed in job_history: ${job.id}`);
					}
					throw error;
				}
			},
			workerOptions,
		);

		// Compliance Worker (install compliance tools + run_scan when agent offline)
		const COMPLIANCE_SCAN_RETRY_DELAY_MS = 60 * 1000; // 1 min
		this.workers[QUEUE_NAMES.COMPLIANCE] = new Worker(
			QUEUE_NAMES.COMPLIANCE,
			async (job, token) => {
				const { hostId, api_id, type } = job.data;

				if (type === "run_scan") {
					const {
						profile_type = "all",
						profile_id,
						enable_remediation,
						fetch_remote_resources,
					} = job.data;

					// Create job_history record so the Agent Queue tab can track this job
					let historyRecord = null;
					try {
						historyRecord = await prisma.job_history.create({
							data: {
								id: uuidv4(),
								job_id: job.id,
								queue_name: QUEUE_NAMES.COMPLIANCE,
								job_name: "run_scan",
								host_id: hostId,
								api_id: api_id,
								status: "active",
								attempt_number: job.attemptsMade + 1,
								created_at: get_current_time(),
								updated_at: get_current_time(),
							},
						});
					} catch (err) {
						logger.error(
							"[Compliance] run_scan: failed to create job_history:",
							err,
						);
					}

					if (!agentWs.isConnected(api_id)) {
						logger.info(
							`[Compliance] run_scan: agent ${api_id} offline, re-queuing in ${COMPLIANCE_SCAN_RETRY_DELAY_MS / 1000}s`,
						);
						if (historyRecord) {
							await prisma.job_history
								.updateMany({
									where: { job_id: job.id },
									data: { status: "delayed", updated_at: get_current_time() },
								})
								.catch(() => {});
						}
						await job.moveToDelayed(
							Date.now() + COMPLIANCE_SCAN_RETRY_DELAY_MS,
							token,
						);
						throw new DelayedError();
					}
					const scanOptions = {
						profileId: profile_id || null,
						enableRemediation: Boolean(enable_remediation),
						fetchRemoteResources: Boolean(fetch_remote_resources),
					};
					const sent = agentWs.pushComplianceScan(
						api_id,
						profile_type,
						scanOptions,
					);
					if (!sent) {
						if (historyRecord) {
							await prisma.job_history
								.updateMany({
									where: { job_id: job.id },
									data: { status: "delayed", updated_at: get_current_time() },
								})
								.catch(() => {});
						}
						await job.moveToDelayed(
							Date.now() + COMPLIANCE_SCAN_RETRY_DELAY_MS,
							token,
						);
						throw new DelayedError();
					}
					// Create "running" scan records for UI
					const profilesToUse = [];
					if (profile_type === "all" || profile_type === "openscap") {
						let oscapProfile = await prisma.compliance_profiles.findFirst({
							where: { type: "openscap" },
							orderBy: { name: "asc" },
						});
						if (!oscapProfile) {
							oscapProfile = await prisma.compliance_profiles.create({
								data: {
									id: uuidv4(),
									name: "OpenSCAP Scan",
									type: "openscap",
								},
							});
						}
						profilesToUse.push(oscapProfile);
					}
					if (profile_type === "all" || profile_type === "docker-bench") {
						let dockerProfile = await prisma.compliance_profiles.findFirst({
							where: { type: "docker-bench" },
							orderBy: { name: "asc" },
						});
						if (!dockerProfile) {
							dockerProfile = await prisma.compliance_profiles.create({
								data: {
									id: uuidv4(),
									name: "Docker Bench Security",
									type: "docker-bench",
								},
							});
						}
						profilesToUse.push(dockerProfile);
					}
					for (const profile of profilesToUse) {
						try {
							await prisma.compliance_scans.create({
								data: {
									id: uuidv4(),
									host_id: hostId,
									profile_id: profile.id,
									started_at: new Date(),
									completed_at: null,
									status: "running",
									total_rules: 0,
									passed: 0,
									failed: 0,
									warnings: 0,
									skipped: 0,
									not_applicable: 0,
									score: null,
								},
							});
						} catch (err) {
							logger.warn(
								`[Compliance] run_scan: could not create running record: ${err.message}`,
							);
						}
					}

					// Mark job_history as completed
					if (historyRecord) {
						await prisma.job_history
							.updateMany({
								where: { job_id: job.id },
								data: {
									status: "completed",
									completed_at: get_current_time(),
									updated_at: get_current_time(),
								},
							})
							.catch(() => {});
					}

					logger.info(
						`[Compliance] run_scan: triggered for host ${hostId} (agent online)`,
					);
					return;
				}

				if (type !== "install_compliance_tools") {
					logger.warn(`[Compliance] Unknown job type: ${type}`);
					return;
				}
				logger.info(
					`[Compliance] Processing install_compliance_tools for host ${hostId} (${api_id})`,
				);

				let historyRecord = null;
				try {
					const host = await prisma.hosts.findUnique({
						where: { id: hostId },
						select: { id: true, api_id: true },
					});
					if (!host) {
						throw new Error(`Host not found: ${hostId}`);
					}
					historyRecord = await prisma.job_history.create({
						data: {
							id: uuidv4(),
							job_id: job.id,
							queue_name: QUEUE_NAMES.COMPLIANCE,
							job_name: "install_compliance_tools",
							host_id: host.id,
							api_id: host.api_id,
							status: "active",
							attempt_number: job.attemptsMade + 1,
							created_at: get_current_time(),
							updated_at: get_current_time(),
						},
					});
				} catch (err) {
					logger.error("[Compliance] Failed to create job_history:", err);
				}

				const jobKey = `${COMPLIANCE_INSTALL_JOB_PREFIX}${hostId}`;
				const cancelKey = `${COMPLIANCE_INSTALL_CANCEL_PREFIX}${job.id}`;

				try {
					const connected = agentWs.isConnected(api_id);
					logger.info(
						`[Compliance] Agent connection check for ${api_id}: ${connected ? "connected" : "not connected"}`,
					);
					if (!connected) {
						throw new Error("Agent is not connected. Cannot run install.");
					}

					await job.updateData({
						...job.data,
						message: "Sending install command to agent...",
					});
					await job.updateProgress(10);

					const sent = agentWs.pushInstallScanner(api_id);
					if (!sent) {
						throw new Error(
							"Failed to send install_scanner command to agent (WebSocket not ready or send failed).",
						);
					}

					await job.updateData({
						...job.data,
						message: "Installing OpenSCAP and SSG content on agent...",
					});
					await job.updateProgress(30);

					const statusKey = `integration_status:${api_id}:compliance`;
					const deadline = Date.now() + COMPLIANCE_INSTALL_TIMEOUT_MS;
					let lastProgress = 30;

					while (Date.now() < deadline) {
						const cancelled = await redis.get(cancelKey);
						if (cancelled) {
							await redis.del(cancelKey);
							throw new Error("Cancelled by user");
						}

						const raw = await redis.get(statusKey);
						if (raw) {
							const data = JSON.parse(raw);
							const status = data.status;
							const message = data.message || status;
							await job.updateData({ ...job.data, message });

							if (status === "installing") {
								if (lastProgress < 70) {
									lastProgress = 70;
									await job.updateProgress(lastProgress);
								}
							} else if (status === "ready") {
								await job.updateProgress(100);
								break;
							} else if (status === "error") {
								throw new Error(data.message || "Agent reported error");
							} else if (status === "partial") {
								await job.updateProgress(90);
								break;
							}
						}

						await new Promise((r) =>
							setTimeout(r, COMPLIANCE_POLL_INTERVAL_MS),
						);
					}

					if (Date.now() >= deadline) {
						throw new Error("Install timed out after 5 minutes");
					}

					await redis.del(jobKey);
					if (historyRecord) {
						await prisma.job_history.updateMany({
							where: { job_id: job.id },
							data: {
								status: "completed",
								completed_at: get_current_time(),
								updated_at: get_current_time(),
							},
						});
					}
					logger.info(`[Compliance] Install completed for host ${hostId}`);
				} catch (error) {
					await redis.del(jobKey);
					await redis.del(cancelKey).catch(() => {});
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
					}
					const errMsg =
						error?.message ??
						(typeof error === "string" ? error : String(error));
					logger.warn(
						`[Compliance] Install failed for host ${hostId}: ${errMsg}`,
					);
					throw error;
				}
			},
			workerOptions,
		);

		logger.info(
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
				logger.error(`❌ Queue '${queueName}' experienced an error:`, error);
			});
			queue.on("failed", (job, err) => {
				logger.error(`❌ Job '${job.id}' in queue '${queueName}' failed:`, err);
			});
			queue.on("completed", (job) => {
				logger.info(`✅ Job '${job.id}' in queue '${queueName}' completed.`);
			});
		}

		logger.info("✅ Queue events initialized");
	}

	/**
	 * Schedule all recurring jobs
	 */
	async scheduleAllJobs() {
		await this.automations[QUEUE_NAMES.VERSION_UPDATE_CHECK].schedule();
		await this.automations[QUEUE_NAMES.SESSION_CLEANUP].schedule();
		await this.automations[QUEUE_NAMES.ORPHANED_REPO_CLEANUP].schedule();
		await this.automations[QUEUE_NAMES.ORPHANED_PACKAGE_CLEANUP].schedule();
		await this.automations[QUEUE_NAMES.DOCKER_INVENTORY_CLEANUP].schedule();
		await this.automations[QUEUE_NAMES.DOCKER_IMAGE_UPDATE_CHECK].schedule();
		await this.automations[QUEUE_NAMES.METRICS_REPORTING].schedule();
		await this.automations[QUEUE_NAMES.SYSTEM_STATISTICS].schedule();
		await this.automations[QUEUE_NAMES.ALERT_CLEANUP].schedule();
		await this.automations[QUEUE_NAMES.HOST_STATUS_MONITOR].schedule();
	}

	/**
	 * Manual job triggers
	 */
	async triggerVersionUpdateCheck() {
		return this.automations[QUEUE_NAMES.VERSION_UPDATE_CHECK].triggerManual();
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

	async triggerAlertCleanup() {
		return this.automations[QUEUE_NAMES.ALERT_CLEANUP].triggerManual();
	}

	async triggerHostStatusMonitor() {
		return this.automations[QUEUE_NAMES.HOST_STATUS_MONITOR].triggerManual();
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
		// Collect jobs from all queues that carry host-specific work
		const queue_names_to_check = [
			QUEUE_NAMES.AGENT_COMMANDS,
			QUEUE_NAMES.COMPLIANCE,
		];

		const filterByApiId = (jobs) =>
			jobs.filter((job) => job.data && job.data.api_id === apiId);

		let waitingCount = 0;
		let activeCount = 0;
		let delayedCount = 0;
		let failedCount = 0;

		// Collect live BullMQ jobs for this host (across all queues)
		const liveJobs = [];

		for (const qn of queue_names_to_check) {
			const q = this.queues[qn];
			if (!q) continue;
			const [waiting, active, delayed, failed, completed] = await Promise.all([
				q.getWaiting(),
				q.getActive(),
				q.getDelayed(),
				q.getFailed(),
				q.getCompleted(0, limit - 1),
			]);
			const hostWaiting = filterByApiId(waiting);
			const hostActive = filterByApiId(active);
			const hostDelayed = filterByApiId(delayed);
			const hostFailed = filterByApiId(failed);
			const hostCompleted = filterByApiId(completed);
			waitingCount += hostWaiting.length;
			activeCount += hostActive.length;
			delayedCount += hostDelayed.length;
			failedCount += hostFailed.length;

			for (const j of hostWaiting)
				liveJobs.push({ job: j, state: "waiting", queue: qn });
			for (const j of hostActive)
				liveJobs.push({ job: j, state: "active", queue: qn });
			for (const j of hostDelayed)
				liveJobs.push({ job: j, state: "delayed", queue: qn });
			for (const j of hostFailed)
				liveJobs.push({ job: j, state: "failed", queue: qn });
			for (const j of hostCompleted)
				liveJobs.push({ job: j, state: "completed", queue: qn });
		}

		// Get job history from database (shows completed attempts and status changes)
		const jobHistory = await prisma.job_history.findMany({
			where: {
				api_id: apiId,
			},
			orderBy: {
				created_at: "desc",
			},
			take: limit,
		});

		// Merge live BullMQ jobs with DB history; live jobs first, then history.
		// Avoid duplicates: if a live job's id matches a history record's job_id, skip the history one.
		const liveJobIds = new Set(liveJobs.map((l) => l.job.id));
		const historyRows = jobHistory
			.filter((h) => !liveJobIds.has(h.job_id))
			.map((job) => ({
				id: job.id,
				job_id: job.job_id,
				job_name: job.job_name,
				queue_name: job.queue_name || null,
				status: job.status,
				attempt_number: job.attempt_number,
				error_message: job.error_message,
				output: job.output,
				created_at: job.created_at,
				updated_at: job.updated_at,
				completed_at: job.completed_at,
			}));

		const liveRows = liveJobs.map((l) => ({
			id: l.job.id,
			job_id: l.job.id,
			job_name: l.job.name || l.job.data?.type || "unknown",
			queue_name: l.queue,
			status: l.state,
			attempt_number: (l.job.attemptsMade || 0) + 1,
			error_message: l.job.failedReason || null,
			output: null,
			created_at: l.job.timestamp ? new Date(l.job.timestamp) : null,
			updated_at: null,
			completed_at: l.job.finishedOn ? new Date(l.job.finishedOn) : null,
		}));

		const merged = [...liveRows, ...historyRows].slice(0, limit);

		return {
			waiting: waitingCount,
			active: activeCount,
			delayed: delayedCount,
			failed: failedCount,
			jobHistory: merged,
		};
	}

	/**
	 * Graceful shutdown
	 */
	async shutdown() {
		logger.info("🛑 Shutting down queue manager...");

		for (const queueName of Object.keys(this.queues)) {
			try {
				await this.queues[queueName].close();
			} catch (e) {
				logger.warn(`⚠️ Failed to close queue '${queueName}':`, e?.message || e);
			}
			if (this.workers?.[queueName]) {
				try {
					await this.workers[queueName].close();
				} catch (e) {
					logger.warn(
						`⚠️ Failed to close worker for '${queueName}':`,
						e?.message || e,
					);
				}
			}
		}

		await redis.quit();
		logger.info("✅ Queue manager shutdown complete");
	}
}

const queueManager = new QueueManager();

module.exports = { queueManager, QUEUE_NAMES };
