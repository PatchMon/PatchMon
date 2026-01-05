const express = require("express");
const { getPrismaClient } = require("../config/prisma");
const { body, validationResult } = require("express-validator");
const { v4: uuidv4 } = require("uuid");
const crypto = require("node:crypto");
const bcrypt = require("bcryptjs");
const logger = require("../utils/logger");
const { authenticateToken } = require("../middleware/auth");
const {
	requireManageHosts,
	requireManageSettings,
} = require("../middleware/permissions");
const { queueManager, QUEUE_NAMES } = require("../services/automation");
const { pushIntegrationToggle, isConnected } = require("../services/agentWs");
const { compareVersions } = require("../services/automation/shared/utils");
const { redis } = require("../services/automation/shared/redis");
const { verifyApiKey } = require("../utils/apiKeyUtils");

const router = express.Router();
const prisma = getPrismaClient();

// Bootstrap token configuration
const BOOTSTRAP_TOKEN_PREFIX = "bootstrap:";
const BOOTSTRAP_TOKEN_TTL = 300; // 5 minutes in seconds

/**
 * Generate a secure bootstrap token for agent installation
 * @param {string} apiId - The host's API ID
 * @param {string} apiKey - The host's API key (will be stored encrypted)
 * @returns {Promise<string>} The bootstrap token
 */
async function generateBootstrapToken(apiId, apiKey) {
	const token = crypto.randomBytes(32).toString("hex");
	const key = `${BOOTSTRAP_TOKEN_PREFIX}${token}`;

	// Store the credentials encrypted in Redis with short TTL
	const data = JSON.stringify({ apiId, apiKey, createdAt: Date.now() });
	await redis.setex(key, BOOTSTRAP_TOKEN_TTL, data);

	return token;
}

/**
 * Retrieve and delete bootstrap token data (one-time use)
 * @param {string} token - The bootstrap token
 * @returns {Promise<{apiId: string, apiKey: string}|null>} The credentials or null
 */
async function consumeBootstrapToken(token) {
	const key = `${BOOTSTRAP_TOKEN_PREFIX}${token}`;
	const data = await redis.get(key);

	if (!data) {
		return null;
	}

	// Delete immediately (one-time use)
	await redis.del(key);

	try {
		return JSON.parse(data);
	} catch (e) {
		return null;
	}
}

// In-memory cache for integration states (api_id -> { integration_name -> enabled })
// This stores the last known state from successful toggles
const integrationStateCache = new Map();

// Middleware to validate API credentials
const validateApiCredentials = async (req, res, next) => {
	try {
		const apiId = req.headers["x-api-id"] || req.body.apiId;
		const apiKey = req.headers["x-api-key"] || req.body.apiKey;

		if (!apiId || !apiKey) {
			return res.status(401).json({ error: "API ID and Key required" });
		}

		// Find host by API ID only (we'll verify the key separately)
		const host = await prisma.hosts.findUnique({
			where: { api_id: apiId },
		});

		if (!host) {
			return res.status(401).json({ error: "Invalid API credentials" });
		}

		// Verify the API key
		const isValidKey = await verifyApiKey(apiKey, host.api_key);
		if (!isValidKey) {
			return res.status(401).json({ error: "Invalid API credentials" });
		}

		req.hostRecord = host;
		next();
	} catch (error) {
		logger.error("API credential validation error:", error);
		res.status(500).json({ error: "API credential validation failed" });
	}
};

// Secure endpoint to download the agent script/binary (requires API authentication)
router.get("/agent/download", async (req, res) => {
	try {
		// Verify API credentials
		const apiId = req.headers["x-api-id"];
		const apiKey = req.headers["x-api-key"];

		if (!apiId || !apiKey) {
			return res.status(401).json({ error: "API credentials required" });
		}

		// Validate API credentials
		const host = await prisma.hosts.findUnique({
			where: { api_id: apiId },
		});

		if (!host) {
			return res.status(401).json({ error: "Invalid API credentials" });
		}

		// Verify API key (supports both hashed and legacy plaintext keys)
		const isValidKey = await verifyApiKey(apiKey, host.api_key);
		if (!isValidKey) {
			return res.status(401).json({ error: "Invalid API credentials" });
		}

		const fs = require("node:fs");
		const path = require("node:path");

		// Check if this is a legacy agent (bash script) requesting update
		// Legacy agents will have agent_version < 1.2.9 (excluding 1.2.9 itself)
		// But allow forcing binary download for fresh installations
		const forceBinary = req.query.force === "binary";
		const isLegacyAgent =
			!forceBinary &&
			host.agent_version &&
			((host.agent_version.startsWith("1.2.") &&
				host.agent_version !== "1.2.9") ||
				host.agent_version.startsWith("1.1.") ||
				host.agent_version.startsWith("1.0."));

		if (isLegacyAgent) {
			// Serve migration script for legacy agents
			const migrationScriptPath = path.join(
				__dirname,
				"../../../agents/patchmon-agent.sh",
			);

			if (!fs.existsSync(migrationScriptPath)) {
				return res.status(404).json({ error: "Migration script not found" });
			}

			// Set appropriate headers for script download
			res.setHeader("Content-Type", "text/plain");
			res.setHeader(
				"Content-Disposition",
				'attachment; filename="patchmon-agent.sh"',
			);

			// Stream the migration script
			const fileStream = fs.createReadStream(migrationScriptPath);
			fileStream.pipe(res);

			fileStream.on("error", (error) => {
				logger.error("Migration script stream error:", error);
				if (!res.headersSent) {
					res.status(500).json({ error: "Failed to stream migration script" });
				}
			});
		} else {
			// Serve Go binary for new agents
			const architecture = req.query.arch || "amd64";

			// Validate architecture
			const validArchitectures = ["amd64", "386", "arm64", "arm"];
			if (!validArchitectures.includes(architecture)) {
				return res.status(400).json({
					error: "Invalid architecture. Must be one of: amd64, 386, arm64, arm",
				});
			}

			const binaryName = `patchmonenhanced-agent-linux-${architecture}`;
			const binaryPath = path.join(__dirname, "../../../agents", binaryName);

			if (!fs.existsSync(binaryPath)) {
				return res.status(404).json({
					error: `Agent binary not found for architecture: ${architecture}`,
				});
			}

			// Set appropriate headers for binary download
			res.setHeader("Content-Type", "application/octet-stream");
			res.setHeader(
				"Content-Disposition",
				`attachment; filename="${binaryName}"`,
			);

			// Stream the binary file
			const fileStream = fs.createReadStream(binaryPath);
			fileStream.pipe(res);

			fileStream.on("error", (error) => {
				logger.error("Binary stream error:", error);
				if (!res.headersSent) {
					res.status(500).json({ error: "Failed to stream agent binary" });
				}
			});
		}
	} catch (error) {
		logger.error("Agent download error:", error);
		res.status(500).json({ error: "Failed to serve agent" });
	}
});

// Version check endpoint for agents
router.get("/agent/version", validateApiCredentials, async (req, res) => {
	try {
		const fs = require("node:fs");
		const path = require("node:path");

		// Check general server auto_update setting
		const settings = await prisma.settings.findFirst();
		const serverAutoUpdateEnabled = settings?.auto_update;

		// Check per-host auto_update setting (req.hostRecord is set by validateApiCredentials middleware)
		const host = req.hostRecord;
		const hostAutoUpdateEnabled = host?.auto_update;

		// Determine if auto-update is disabled
		const autoUpdateDisabled =
			!serverAutoUpdateEnabled || !hostAutoUpdateEnabled;
		let autoUpdateDisabledReason = null;
		if (!serverAutoUpdateEnabled && !hostAutoUpdateEnabled) {
			autoUpdateDisabledReason =
				"Auto-update is disabled in server settings and for this host";
		} else if (!serverAutoUpdateEnabled) {
			autoUpdateDisabledReason = "Auto-update is disabled in server settings";
		} else if (!hostAutoUpdateEnabled) {
			autoUpdateDisabledReason = "Auto-update is disabled for this host";
		}

		// Get architecture parameter (default to amd64 for Go agents)
		const architecture = req.query.arch || "amd64";
		const agentType = req.query.type || "go"; // "go" or "legacy"

		if (agentType === "legacy") {
			// Legacy agent version check (bash script)
			const agentPath = path.join(
				__dirname,
				"../../../agents/patchmon-agent.sh",
			);

			if (!fs.existsSync(agentPath)) {
				return res.status(404).json({ error: "Legacy agent script not found" });
			}

			const scriptContent = fs.readFileSync(agentPath, "utf8");
			const versionMatch = scriptContent.match(/AGENT_VERSION="([^"]+)"/);

			if (!versionMatch) {
				return res
					.status(500)
					.json({ error: "Could not extract version from agent script" });
			}

			const currentVersion = versionMatch[1];

			res.json({
				currentVersion: currentVersion,
				latestVersion: currentVersion,
				hasUpdate: false,
				autoUpdateDisabled: autoUpdateDisabled,
				autoUpdateDisabledReason: autoUpdateDisabled
					? autoUpdateDisabledReason
					: null,
				downloadUrl: `/api/v1/hosts/agent/download`,
				releaseNotes: `PatchMon Agent v${currentVersion}`,
				minServerVersion: null,
				agentType: "legacy",
			});
		} else {
			// Go agent version check
			// Always check the server's local binary for the requested architecture
			// The server's agents folder is the source of truth, not GitHub
			const { execFile } = require("node:child_process");
			const { promisify } = require("node:util");
			const execFileAsync = promisify(execFile);

			const binaryName = `patchmonenhanced-agent-linux-${architecture}`;
			const binaryPath = path.join(__dirname, "../../../agents", binaryName);

			if (fs.existsSync(binaryPath)) {
				// Binary exists in server's agents folder - use its version
				let serverVersion = null;

				// Try method 1: Execute binary directly (works for same architecture)
				// Using execFile instead of exec to prevent shell injection
				try {
					const { stdout } = await execFileAsync(binaryPath, ["--help"], {
						timeout: 10000,
					});

					// Parse version from help output (e.g., "PatchMon Agent v1.3.1")
					const versionMatch = stdout.match(
						/PatchMon Agent v([0-9]+\.[0-9]+\.[0-9]+)/i,
					);

					if (versionMatch) {
						serverVersion = versionMatch[1];
					}
				} catch (execError) {
					// Execution failed (likely cross-architecture) - try alternative method
					logger.warn(
						`Failed to execute binary ${binaryName} to get version (may be cross-architecture): ${execError.message}`,
					);

					// Try method 2: Extract version using strings command (works for cross-architecture)
					// Using execFile with strings command to avoid shell injection
					try {
						const { stdout: stringsOutput } = await execFileAsync(
							"strings",
							[binaryPath],
							{ timeout: 10000, maxBuffer: 10 * 1024 * 1024 },
						);

						// Filter in Node.js instead of piping through grep
						const versionMatch = stringsOutput.match(
							/PatchMon Agent v([0-9]+\.[0-9]+\.[0-9]+)/i,
						);

						if (versionMatch) {
							serverVersion = versionMatch[1];
							logger.info(
								`✅ Extracted version ${serverVersion} from binary using strings command`,
							);
						}
					} catch (stringsError) {
						logger.warn(
							`Failed to extract version using strings command: ${stringsError.message}`,
						);
					}
				}

				// If we successfully got the version, return it
				if (serverVersion) {
					const agentVersion = req.query.currentVersion || serverVersion;

					// Proper semantic version comparison: only update if server version is NEWER
					const hasUpdate = compareVersions(serverVersion, agentVersion) > 0;

					// Calculate SHA256 hash of the binary for integrity verification
					// This allows agents to verify the downloaded binary matches the expected hash
					let binaryHash = null;
					try {
						const binaryContent = fs.readFileSync(binaryPath);
						binaryHash = crypto.createHash("sha256").update(binaryContent).digest("hex");
					} catch (hashErr) {
						logger.warn(`Failed to calculate hash for binary ${binaryName}: ${hashErr.message}`);
					}

					// Return update info, but indicate if auto-update is disabled
					return res.json({
						currentVersion: agentVersion,
						latestVersion: serverVersion,
						hasUpdate: hasUpdate && !autoUpdateDisabled, // Only true if update available AND auto-update enabled
						autoUpdateDisabled: autoUpdateDisabled,
						autoUpdateDisabledReason: autoUpdateDisabled
							? autoUpdateDisabledReason
							: null,
						downloadUrl: `/api/v1/hosts/agent/download?arch=${architecture}`,
						releaseNotes: `PatchMon Agent v${serverVersion}`,
						minServerVersion: null,
						architecture: architecture,
						agentType: "go",
						hash: binaryHash, // SHA256 hash for integrity verification
					});
				}

				// If we couldn't get version, fall through to error response
				logger.warn(
					`Could not determine version for binary ${binaryName} using any method`,
				);
			}

			// Binary doesn't exist or couldn't get version - return error
			// Don't fall back to GitHub - the server's agents folder is the source of truth
			const agentVersion = req.query.currentVersion || "unknown";
			return res.status(404).json({
				error: `Agent binary not found for architecture: ${architecture}. Please ensure the binary is in the server's agents folder.`,
				currentVersion: agentVersion,
				latestVersion: null,
				hasUpdate: false,
				autoUpdateDisabled: autoUpdateDisabled,
				autoUpdateDisabledReason: autoUpdateDisabled
					? autoUpdateDisabledReason
					: null,
				architecture: architecture,
				agentType: "go",
			});
		}
	} catch (error) {
		logger.error("Version check error:", error);
		res.status(500).json({ error: "Failed to get agent version" });
	}
});

// Generate API credentials with hashed key for secure storage
const generateApiCredentials = async () => {
	const apiId = `patchmon_${crypto.randomBytes(8).toString("hex")}`;
	const apiKey = crypto.randomBytes(32).toString("hex");
	// Hash the API key for secure storage (bcrypt with cost factor 10)
	const apiKeyHash = await bcrypt.hash(apiKey, 10);
	return { apiId, apiKey, apiKeyHash };
};

// Admin endpoint to create a new host manually (replaces auto-registration)
router.post(
	"/create",
	authenticateToken,
	requireManageHosts,
	[
		body("friendly_name")
			.isLength({ min: 1 })
			.withMessage("Friendly name is required"),
		body("hostGroupIds")
			.optional()
			.isArray()
			.withMessage("Host group IDs must be an array"),
		body("hostGroupIds.*")
			.optional()
			.isUUID()
			.withMessage("Each host group ID must be a valid UUID"),
		body("docker_enabled")
			.optional()
			.isBoolean()
			.withMessage("Docker enabled must be a boolean"),
		body("compliance_enabled")
			.optional()
			.isBoolean()
			.withMessage("Compliance enabled must be a boolean"),
	],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const { friendly_name, hostGroupIds, docker_enabled, compliance_enabled } = req.body;

			// Generate unique API credentials for this host
			// apiKey is plaintext (shown to admin once), apiKeyHash is stored in DB
			const { apiId, apiKey, apiKeyHash } = await generateApiCredentials();

			// If hostGroupIds is provided, verify all groups exist
			if (hostGroupIds && hostGroupIds.length > 0) {
				const hostGroups = await prisma.host_groups.findMany({
					where: { id: { in: hostGroupIds } },
				});

				if (hostGroups.length !== hostGroupIds.length) {
					return res
						.status(400)
						.json({ error: "One or more host groups not found" });
				}
			}

			// Create new host with API credentials - system info will be populated when agent connects
			// Store the hashed API key for security (plaintext is shown to admin only once)
			const host = await prisma.hosts.create({
				data: {
					id: uuidv4(),
					machine_id: `pending-${uuidv4()}`, // Temporary placeholder until agent connects with real machine_id
					friendly_name: friendly_name,
					os_type: "unknown", // Will be updated when agent connects
					os_version: "unknown", // Will be updated when agent connects
					ip: null, // Will be updated when agent connects
					architecture: null, // Will be updated when agent connects
					api_id: apiId,
					api_key: apiKeyHash, // Store hash, not plaintext
					status: "pending", // Will change to 'active' when agent connects
					docker_enabled: docker_enabled ?? false, // Set integration state if provided
					compliance_enabled: compliance_enabled ?? false, // Set compliance integration state if provided
					updated_at: new Date(),
					// Create host group memberships if hostGroupIds are provided
					host_group_memberships:
						hostGroupIds && hostGroupIds.length > 0
							? {
									create: hostGroupIds.map((groupId) => ({
										id: uuidv4(),
										host_groups: {
											connect: { id: groupId },
										},
									})),
								}
							: undefined,
				},
				include: {
					host_group_memberships: {
						include: {
							host_groups: {
								select: {
									id: true,
									name: true,
									color: true,
								},
							},
						},
					},
				},
			});

			res.status(201).json({
				message: "Host created successfully",
				hostId: host.id,
				friendlyName: host.friendly_name,
				apiId: host.api_id,
				apiKey: apiKey, // Return plaintext key (shown only once, not stored)
				hostGroups:
					host.host_group_memberships?.map(
						(membership) => membership.host_groups,
					) || [],
				instructions:
					"Use these credentials in your patchmon agent configuration. System information will be automatically detected when the agent connects.",
			});
		} catch (error) {
			logger.error("Host creation error:", error);

			// Check if error is related to connection pool exhaustion
			if (
				error.message &&
				(error.message.includes("connection pool") ||
					error.message.includes("Timed out fetching") ||
					error.message.includes("pool timeout"))
			) {
				logger.error("⚠️  DATABASE CONNECTION POOL EXHAUSTED!");
				logger.error(
					`⚠️  Current limit: DB_CONNECTION_LIMIT=${process.env.DB_CONNECTION_LIMIT || "30"}`,
				);
				logger.error(
					`⚠️  Pool timeout: DB_POOL_TIMEOUT=${process.env.DB_POOL_TIMEOUT || "20"}s`,
				);
				logger.error(
					"⚠️  Suggestion: Increase DB_CONNECTION_LIMIT in your .env file",
				);
			}

			res.status(500).json({ error: "Failed to create host" });
		}
	},
);

// Legacy register endpoint (deprecated - returns error message)
router.post("/register", async (_req, res) => {
	res.status(400).json({
		error:
			"Host registration has been disabled. Please contact your administrator to add this host to PatchMon.",
		deprecated: true,
		message:
			"Hosts must now be pre-created by administrators with specific API credentials.",
	});
});

// Request size limit middleware for /update endpoint
// Smaller limit than global to prevent DoS while still allowing large package lists
const updateBodyLimit = express.json({
	limit: process.env.AGENT_UPDATE_BODY_LIMIT || "2mb",
});

// Update host information and packages (now uses API credentials)
router.post(
	"/update",
	updateBodyLimit,
	validateApiCredentials,
	[
		body("packages")
			.isArray({ max: 10000 })
			.withMessage("Packages must be an array with max 10000 items"),
		body("packages.*.name")
			.isLength({ min: 1 })
			.withMessage("Package name is required"),
		body("packages.*.currentVersion")
			.isLength({ min: 1 })
			.withMessage("Current version is required"),
		body("packages.*.availableVersion").optional().isLength({ min: 1 }),
		body("packages.*.needsUpdate")
			.isBoolean()
			.withMessage("needsUpdate must be boolean"),
		body("packages.*.isSecurityUpdate")
			.optional()
			.isBoolean()
			.withMessage("isSecurityUpdate must be boolean"),
		body("agentVersion")
			.optional()
			.isLength({ min: 1 })
			.withMessage("Agent version must be a non-empty string"),
		// Hardware Information
		body("cpuModel")
			.optional()
			.isString()
			.withMessage("CPU model must be a string"),
		body("cpuCores")
			.optional()
			.isInt({ min: 1 })
			.withMessage("CPU cores must be a positive integer"),
		body("ramInstalled")
			.optional()
			.isFloat({ min: 0.01 })
			.withMessage("RAM installed must be a positive number"),
		body("swapSize")
			.optional()
			.isFloat({ min: 0 })
			.withMessage("Swap size must be a non-negative number"),
		body("diskDetails")
			.optional()
			.isArray()
			.withMessage("Disk details must be an array"),
		// Network Information
		body("gatewayIp")
			.optional({ checkFalsy: true })
			.isIP()
			.withMessage("Gateway IP must be a valid IP address"),
		body("dnsServers")
			.optional()
			.isArray()
			.withMessage("DNS servers must be an array"),
		body("networkInterfaces")
			.optional()
			.isArray()
			.withMessage("Network interfaces must be an array"),
		// System Information
		body("kernelVersion")
			.optional()
			.isString()
			.withMessage("Kernel version must be a string"),
		body("installedKernelVersion")
			.optional()
			.isString()
			.withMessage("Installed kernel version must be a string"),
		body("selinuxStatus")
			.optional()
			.isIn(["enabled", "disabled", "permissive"])
			.withMessage("SELinux status must be enabled, disabled, or permissive"),
		body("systemUptime")
			.optional()
			.isString()
			.withMessage("System uptime must be a string"),
		body("loadAverage")
			.optional()
			.isArray()
			.withMessage("Load average must be an array"),
		body("machineId")
			.optional()
			.isString()
			.withMessage("Machine ID must be a string"),
		body("needsReboot")
			.optional()
			.isBoolean()
			.withMessage("Needs reboot must be a boolean"),
		body("rebootReason")
			.optional()
			.isString()
			.withMessage("Reboot reason must be a string"),
	],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const { packages, repositories, executionTime } = req.body;
			const host = req.hostRecord;

			// Calculate payload size in KB
			const payloadSizeBytes = JSON.stringify(req.body).length;
			const payloadSizeKb = payloadSizeBytes / 1024;

			// Update host last update timestamp and system info if provided
			const updateData = {
				last_update: new Date(),
				updated_at: new Date(),
			};

			// Update machine_id if provided and current one is a placeholder or null
			if (
				req.body.machineId &&
				(host.machine_id === null || host.machine_id.startsWith("pending-"))
			) {
				updateData.machine_id = req.body.machineId;
			}

			// Basic system info
			if (req.body.osType) updateData.os_type = req.body.osType;
			if (req.body.osVersion) updateData.os_version = req.body.osVersion;
			if (req.body.hostname) updateData.hostname = req.body.hostname;
			if (req.body.ip) updateData.ip = req.body.ip;
			if (req.body.architecture)
				updateData.architecture = req.body.architecture;
			if (req.body.agentVersion)
				updateData.agent_version = req.body.agentVersion;

			// Hardware Information
			if (req.body.cpuModel) updateData.cpu_model = req.body.cpuModel;
			if (req.body.cpuCores) updateData.cpu_cores = req.body.cpuCores;
			if (req.body.ramInstalled)
				updateData.ram_installed = req.body.ramInstalled;
			if (req.body.swapSize !== undefined)
				updateData.swap_size = req.body.swapSize;
			if (req.body.diskDetails) updateData.disk_details = req.body.diskDetails;

			// Network Information
			if (req.body.gatewayIp) {
				updateData.gateway_ip = req.body.gatewayIp;
			} else if (Object.hasOwn(req.body, "gatewayIp")) {
				// Log warning if gateway field was sent but empty (isolated network)
				logger.warn(
					`Host ${host.hostname} reported with no default gateway configured`,
				);
			}
			if (req.body.dnsServers) updateData.dns_servers = req.body.dnsServers;
			if (req.body.networkInterfaces)
				updateData.network_interfaces = req.body.networkInterfaces;

			// System Information
			if (req.body.kernelVersion)
				updateData.kernel_version = req.body.kernelVersion;
			if (req.body.installedKernelVersion)
				updateData.installed_kernel_version = req.body.installedKernelVersion;
			if (req.body.selinuxStatus)
				updateData.selinux_status = req.body.selinuxStatus;
			if (req.body.systemUptime)
				updateData.system_uptime = req.body.systemUptime;
			if (req.body.loadAverage) updateData.load_average = req.body.loadAverage;

			// Reboot Status
			if (req.body.needsReboot !== undefined)
				updateData.needs_reboot = req.body.needsReboot;
			if (req.body.rebootReason !== undefined)
				updateData.reboot_reason = req.body.rebootReason;

			// If this is the first update (status is 'pending'), change to 'active'
			if (host.status === "pending") {
				updateData.status = "active";
			}

			// Calculate package counts before transaction
			const securityCount = packages.filter(
				(pkg) => pkg.isSecurityUpdate,
			).length;
			const updatesCount = packages.filter((pkg) => pkg.needsUpdate).length;
			const totalPackages = packages.length;

			// Process everything in a single transaction to avoid race conditions
			await prisma.$transaction(
				async (tx) => {
					// Update host data
					await tx.hosts.update({
						where: { id: host.id },
						data: updateData,
					});

					// Clear existing host packages to avoid duplicates
					await tx.host_packages.deleteMany({
						where: { host_id: host.id },
					});

					// Process packages in batches using createMany/updateMany
					const packagesToCreate = [];
					const packagesToUpdate = [];

					// First pass: identify what needs to be created/updated
					const existingPackages = await tx.packages.findMany({
						where: {
							name: { in: packages.map((p) => p.name) },
						},
					});

					const existingPackageMap = new Map(
						existingPackages.map((p) => [p.name, p]),
					);

					for (const packageData of packages) {
						const existingPkg = existingPackageMap.get(packageData.name);

						if (!existingPkg) {
							// Package doesn't exist, create it
							const newPkg = {
								id: uuidv4(),
								name: packageData.name,
								description: packageData.description || null,
								category: packageData.category || null,
								latest_version:
									packageData.availableVersion || packageData.currentVersion,
								created_at: new Date(),
								updated_at: new Date(),
							};
							packagesToCreate.push(newPkg);
							existingPackageMap.set(packageData.name, newPkg);
						} else {
							// Package exists - check if we need to update version or metadata
							if (
								(packageData.availableVersion &&
									packageData.availableVersion !==
										existingPkg.latest_version) ||
								(packageData.description &&
									packageData.description !== existingPkg.description) ||
								(packageData.category &&
									packageData.category !== existingPkg.category)
							) {
								packagesToUpdate.push({
									id: existingPkg.id,
									latest_version:
										packageData.availableVersion || existingPkg.latest_version,
									description:
										packageData.description || existingPkg.description,
									category: packageData.category || existingPkg.category,
								});
							}
						}
					}

					// Batch create new packages
					if (packagesToCreate.length > 0) {
						await tx.packages.createMany({
							data: packagesToCreate,
							skipDuplicates: true,
						});
					}

					// Batch update existing packages
					for (const update of packagesToUpdate) {
						await tx.packages.update({
							where: { id: update.id },
							data: {
								latest_version: update.latest_version,
								description: update.description,
								category: update.category,
								updated_at: new Date(),
							},
						});
					}

					// Now process host_packages
					for (const packageData of packages) {
						const pkg = existingPackageMap.get(packageData.name);

						await tx.host_packages.upsert({
							where: {
								host_id_package_id: {
									host_id: host.id,
									package_id: pkg.id,
								},
							},
							update: {
								current_version: packageData.currentVersion,
								available_version: packageData.availableVersion || null,
								needs_update: packageData.needsUpdate,
								is_security_update: packageData.isSecurityUpdate || false,
								last_checked: new Date(),
							},
							create: {
								id: uuidv4(),
								host_id: host.id,
								package_id: pkg.id,
								current_version: packageData.currentVersion,
								available_version: packageData.availableVersion || null,
								needs_update: packageData.needsUpdate,
								is_security_update: packageData.isSecurityUpdate || false,
								last_checked: new Date(),
							},
						});
					}

					// Process repositories if provided
					if (repositories && Array.isArray(repositories)) {
						// Clear existing host repositories
						await tx.host_repositories.deleteMany({
							where: { host_id: host.id },
						});

						// Deduplicate repositories by URL+distribution+components to avoid constraint violations
						const uniqueRepos = new Map();
						for (const repoData of repositories) {
							const key = `${repoData.url}|${repoData.distribution}|${repoData.components}`;
							if (!uniqueRepos.has(key)) {
								uniqueRepos.set(key, repoData);
							}
						}

						// Process each unique repository
						for (const repoData of uniqueRepos.values()) {
							// Find or create repository
							let repo = await tx.repositories.findFirst({
								where: {
									url: repoData.url,
									distribution: repoData.distribution,
									components: repoData.components,
								},
							});

							if (!repo) {
								repo = await tx.repositories.create({
									data: {
										id: uuidv4(),
										name: repoData.name,
										url: repoData.url,
										distribution: repoData.distribution,
										components: repoData.components,
										repo_type: repoData.repoType,
										is_active: true,
										is_secure: repoData.isSecure || false,
										description: `${repoData.repoType} repository for ${repoData.distribution}`,
										updated_at: new Date(),
									},
								});
							}

							// Create host repository relationship
							await tx.host_repositories.create({
								data: {
									id: uuidv4(),
									host_id: host.id,
									repository_id: repo.id,
									is_enabled: repoData.isEnabled !== false, // Default to enabled
									last_checked: new Date(),
								},
							});
						}
					}

					// Create update history record
					await tx.update_history.create({
						data: {
							id: uuidv4(),
							host_id: host.id,
							packages_count: updatesCount,
							security_count: securityCount,
							total_packages: totalPackages,
							payload_size_kb: payloadSizeKb,
							execution_time: executionTime ? parseFloat(executionTime) : null,
							status: "success",
						},
					});
				},
				{
					maxWait: 30000, // Wait up to 30s for a transaction slot
					timeout: 60000, // Allow transaction to run for up to 60s
				},
			);

			// Agent auto-update is now handled client-side by the agent itself

			const response = {
				message: "Host updated successfully",
				packagesProcessed: packages.length,
				updatesAvailable: updatesCount,
				securityUpdates: securityCount,
			};

			// Check if crontab update is needed (when update interval changes)
			// This is a simple check - if the host has auto-update enabled, we'll suggest crontab update
			if (host.auto_update) {
				// For now, we'll always suggest crontab update to ensure it's current
				// In a more sophisticated implementation, we could track when the interval last changed
				response.crontabUpdate = {
					shouldUpdate: true,
					message:
						"Please ensure your crontab is up to date with current interval settings",
					command: "update-crontab",
				};
			}

			res.json(response);
		} catch (error) {
			logger.error("Host update error:", error);

			// Log error in update history
			try {
				await prisma.update_history.create({
					data: {
						id: uuidv4(),
						host_id: req.hostRecord.id,
						packages_count: 0,
						security_count: 0,
						status: "error",
						error_message: error.message,
					},
				});
			} catch (logError) {
				logger.error("Failed to log update error:", logError);
			}

			res.status(500).json({ error: "Failed to update host" });
		}
	},
);

// Get host information (now uses API credentials)
router.get("/info", validateApiCredentials, async (req, res) => {
	try {
		const host = await prisma.hosts.findUnique({
			where: { id: req.hostRecord.id },
			select: {
				id: true,
				friendly_name: true,
				hostname: true,
				ip: true,
				os_type: true,
				os_version: true,
				architecture: true,
				last_update: true,
				status: true,
				created_at: true,
				api_id: true, // Include API ID for reference
			},
		});

		res.json(host);
	} catch (error) {
		logger.error("Get host info error:", error);
		res.status(500).json({ error: "Failed to fetch host information" });
	}
});

// Get integration status for agent (uses API credentials)
router.get("/integrations", validateApiCredentials, async (req, res) => {
	try {
		const host = await prisma.hosts.findUnique({
			where: { id: req.hostRecord.id },
			select: {
				id: true,
				docker_enabled: true,
				compliance_enabled: true,
			},
		});

		if (!host) {
			return res.status(404).json({ error: "Host not found" });
		}

		// Return integration states from database (source of truth)
		const integrations = {
			docker: host.docker_enabled ?? false,
			compliance: host.compliance_enabled ?? false,
		};

		res.json({
			success: true,
			integrations: integrations,
		});
	} catch (error) {
		logger.error("Get integration status error:", error);
		res.status(500).json({ error: "Failed to get integration status" });
	}
});

// Receive integration setup status from agent
router.post("/integration-status", validateApiCredentials, async (req, res) => {
	try {
		const { integration, enabled, status, message, components, scanner_info } = req.body;
		const hostId = req.hostRecord.id;
		const apiId = req.hostRecord.api_id;

		logger.info(`📊 Integration status update from ${apiId}:`, {
			integration,
			enabled,
			status,
			message,
			components,
			scanner_info: scanner_info ? "present" : "not provided",
		});

		// Store the status update in Redis for real-time UI updates
		const statusKey = `integration_status:${apiId}:${integration}`;
		const statusData = {
			integration,
			enabled,
			status,
			message,
			components: components || {},
			scanner_info: scanner_info || null,
			timestamp: new Date().toISOString(),
		};

		// Store with 1 hour expiry
		await redis.setex(statusKey, 3600, JSON.stringify(statusData));

		// Also broadcast via WebSocket if available
		try {
			const { broadcastToHost } = require("../services/agentWs");
			if (broadcastToHost) {
				broadcastToHost(apiId, {
					type: "integration_status",
					data: statusData,
				});
			}
		} catch (wsError) {
			logger.info("WebSocket broadcast not available:", wsError.message);
		}

		// Update host record with compliance setup status
		if (integration === "compliance" && status === "ready") {
			await prisma.hosts.update({
				where: { id: hostId },
				data: {
					compliance_enabled: enabled,
					updated_at: new Date(),
				},
			});
		}

		res.json({
			success: true,
			message: "Integration status received",
		});
	} catch (error) {
		logger.error("Integration status update error:", error);
		res.status(500).json({ error: "Failed to process integration status" });
	}
});

// Ping endpoint for health checks (now uses API credentials)
router.post("/ping", validateApiCredentials, async (req, res) => {
	try {
		const now = new Date();
		const lastUpdate = req.hostRecord.last_update;

		// Detect if this is an agent startup (first ping or after long absence)
		const timeSinceLastUpdate = lastUpdate ? now - lastUpdate : null;
		const isStartup =
			!timeSinceLastUpdate || timeSinceLastUpdate > 5 * 60 * 1000; // 5 minutes

		// Log agent startup
		if (isStartup) {
			logger.info(
				`🚀 Agent startup detected: ${req.hostRecord.friendly_name} (${req.hostRecord.hostname || req.hostRecord.api_id})`,
			);

			// Check if status was previously offline
			if (req.hostRecord.status === "offline") {
				logger.info(`✅ Agent back online: ${req.hostRecord.friendly_name}`);
			}
		}

		// Update last update timestamp and set status to active
		await prisma.hosts.update({
			where: { id: req.hostRecord.id },
			data: {
				last_update: now,
				updated_at: now,
				status: "active",
			},
		});

		const response = {
			message: "Ping successful",
			timestamp: now.toISOString(),
			friendlyName: req.hostRecord.friendly_name,
			agentStartup: isStartup,
		};

		// Include integration states in ping response for initial agent configuration
		// This allows agent to sync config.yml with database state during setup
		response.integrations = {
			docker: req.hostRecord.docker_enabled ?? false,
			compliance: req.hostRecord.compliance_enabled ?? false,
		};

		// Check if this is a crontab update trigger
		if (req.body.triggerCrontabUpdate && req.hostRecord.auto_update) {
			logger.info(
				`Triggering crontab update for host: ${req.hostRecord.friendly_name}`,
			);
			response.crontabUpdate = {
				shouldUpdate: true,
				message:
					"Update interval changed, please run: /usr/local/bin/patchmon-agent.sh update-crontab",
				command: "update-crontab",
			};
		}

		res.json(response);
	} catch (error) {
		logger.error("Ping error:", error);
		res.status(500).json({ error: "Ping failed" });
	}
});

// Admin endpoint to regenerate API credentials for a host
router.post(
	"/:hostId/regenerate-credentials",
	authenticateToken,
	requireManageHosts,
	async (req, res) => {
		try {
			const { hostId } = req.params;

			// Generate new API credentials before transaction (CPU intensive, don't hold lock)
			// apiKey is plaintext (shown to admin once), apiKeyHash is stored in DB
			const { apiId, apiKey, apiKeyHash } = await generateApiCredentials();

			// Use transaction to ensure atomicity of check and update
			const updatedHost = await prisma.$transaction(async (tx) => {
				const host = await tx.hosts.findUnique({
					where: { id: hostId },
				});

				if (!host) {
					throw new Error("HOST_NOT_FOUND");
				}

				// Update host with new credentials (store hash, not plaintext)
				return tx.hosts.update({
					where: { id: hostId },
					data: {
						api_id: apiId,
						api_key: apiKeyHash, // Store hash, not plaintext
						updated_at: new Date(),
					},
				});
			});

			res.json({
				message: "API credentials regenerated successfully",
				hostname: updatedHost.hostname,
				apiId: updatedHost.api_id,
				apiKey: apiKey, // Return plaintext key (shown only once, not stored)
				warning:
					"Previous credentials are now invalid. Update your agent configuration.",
			});
		} catch (error) {
			if (error.message === "HOST_NOT_FOUND") {
				return res.status(404).json({ error: "Host not found" });
			}
			logger.error("Credential regeneration error:", error);
			res.status(500).json({ error: "Failed to regenerate credentials" });
		}
	},
);

router.put(
	"/bulk/groups",
	authenticateToken,
	requireManageHosts,
	[
		body("hostIds").isArray().withMessage("Host IDs must be an array"),
		body("hostIds.*")
			.isLength({ min: 1 })
			.withMessage("Each host ID must be provided"),
		body("groupIds").isArray().optional(),
		body("groupIds.*")
			.optional()
			.isUUID()
			.withMessage("Each group ID must be a valid UUID"),
	],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const { hostIds, groupIds = [] } = req.body;

			// Verify all groups exist if provided
			if (groupIds.length > 0) {
				const existingGroups = await prisma.host_groups.findMany({
					where: { id: { in: groupIds } },
					select: { id: true },
				});

				if (existingGroups.length !== groupIds.length) {
					return res.status(400).json({
						error: "One or more host groups not found",
						provided: groupIds,
						found: existingGroups.map((g) => g.id),
					});
				}
			}

			// Check if all hosts exist
			const existingHosts = await prisma.hosts.findMany({
				where: { id: { in: hostIds } },
				select: { id: true, friendly_name: true },
			});

			if (existingHosts.length !== hostIds.length) {
				const foundIds = existingHosts.map((h) => h.id);
				const missingIds = hostIds.filter((id) => !foundIds.includes(id));
				return res.status(400).json({
					error: "Some hosts not found",
					missingHostIds: missingIds,
				});
			}

			// Use transaction to update group memberships for all hosts
			const updatedHosts = await prisma.$transaction(async (tx) => {
				const results = [];

				for (const hostId of hostIds) {
					// Remove existing memberships for this host
					await tx.host_group_memberships.deleteMany({
						where: { host_id: hostId },
					});

					// Add new memberships for this host
					if (groupIds.length > 0) {
						await tx.host_group_memberships.createMany({
							data: groupIds.map((groupId) => ({
								id: crypto.randomUUID(),
								host_id: hostId,
								host_group_id: groupId,
							})),
						});
					}

					// Get updated host with groups
					const updatedHost = await tx.hosts.findUnique({
						where: { id: hostId },
						include: {
							host_group_memberships: {
								include: {
									host_groups: {
										select: {
											id: true,
											name: true,
											color: true,
										},
									},
								},
							},
						},
					});

					results.push(updatedHost);
				}

				return results;
			});

			res.json({
				message: `Successfully updated ${updatedHosts.length} host${updatedHosts.length !== 1 ? "s" : ""}`,
				updatedCount: updatedHosts.length,
				hosts: updatedHosts,
			});
		} catch (error) {
			logger.error("Bulk host groups update error:", error);
			res.status(500).json({ error: "Failed to update host groups" });
		}
	},
);

// Admin endpoint to update host groups (many-to-many)
router.put(
	"/:hostId/groups",
	authenticateToken,
	requireManageHosts,
	[body("groupIds").isArray().optional()],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const { hostId } = req.params;
			const { groupIds = [] } = req.body;

			// Check if host exists
			const host = await prisma.hosts.findUnique({
				where: { id: hostId },
			});

			if (!host) {
				return res.status(404).json({ error: "Host not found" });
			}

			// Verify all groups exist
			if (groupIds.length > 0) {
				const existingGroups = await prisma.host_groups.findMany({
					where: { id: { in: groupIds } },
					select: { id: true },
				});

				if (existingGroups.length !== groupIds.length) {
					return res.status(400).json({
						error: "One or more host groups not found",
						provided: groupIds,
						found: existingGroups.map((g) => g.id),
					});
				}
			}

			// Use transaction to update group memberships
			const updatedHost = await prisma.$transaction(async (tx) => {
				// Remove existing memberships
				await tx.host_group_memberships.deleteMany({
					where: { host_id: hostId },
				});

				// Add new memberships
				if (groupIds.length > 0) {
					await tx.host_group_memberships.createMany({
						data: groupIds.map((groupId) => ({
							id: crypto.randomUUID(),
							host_id: hostId,
							host_group_id: groupId,
						})),
					});
				}

				// Return updated host with groups
				return await tx.hosts.findUnique({
					where: { id: hostId },
					include: {
						host_group_memberships: {
							include: {
								host_groups: {
									select: {
										id: true,
										name: true,
										color: true,
									},
								},
							},
						},
					},
				});
			});

			res.json({
				message: "Host groups updated successfully",
				host: updatedHost,
			});
		} catch (error) {
			logger.error("Host groups update error:", error);
			res.status(500).json({ error: "Failed to update host groups" });
		}
	},
);

// Legacy endpoint to update single host group (for backward compatibility)
router.put(
	"/:hostId/group",
	authenticateToken,
	requireManageHosts,
	[body("hostGroupId").optional()],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const { hostId } = req.params;
			const { hostGroupId } = req.body;

			// Convert single group to array and use the new endpoint logic
			const _groupIds = hostGroupId ? [hostGroupId] : [];

			// Check if host exists
			const host = await prisma.hosts.findUnique({
				where: { id: hostId },
			});

			if (!host) {
				return res.status(404).json({ error: "Host not found" });
			}

			// Verify group exists if provided
			if (hostGroupId) {
				const hostGroup = await prisma.host_groups.findUnique({
					where: { id: hostGroupId },
				});

				if (!hostGroup) {
					return res.status(400).json({ error: "Host group not found" });
				}
			}

			// Use transaction to update group memberships
			const updatedHost = await prisma.$transaction(async (tx) => {
				// Remove existing memberships
				await tx.host_group_memberships.deleteMany({
					where: { host_id: hostId },
				});

				// Add new membership if group provided
				if (hostGroupId) {
					await tx.host_group_memberships.create({
						data: {
							id: crypto.randomUUID(),
							host_id: hostId,
							host_group_id: hostGroupId,
						},
					});
				}

				// Return updated host with groups
				return await tx.hosts.findUnique({
					where: { id: hostId },
					include: {
						host_group_memberships: {
							include: {
								host_groups: {
									select: {
										id: true,
										name: true,
										color: true,
									},
								},
							},
						},
					},
				});
			});

			res.json({
				message: "Host group updated successfully",
				host: updatedHost,
			});
		} catch (error) {
			logger.error("Host group update error:", error);
			res.status(500).json({ error: "Failed to update host group" });
		}
	},
);

// Admin endpoint to list all hosts with optional pagination
// Query params: page (default: 1), pageSize (default: 100, max: 500), all (skip pagination)
router.get(
	"/admin/list",
	authenticateToken,
	requireManageHosts,
	async (req, res) => {
		try {
			// Check if client wants all results (for backward compatibility)
			const returnAll = req.query.all === "true";

			// Pagination parameters with defaults and limits
			const page = Math.max(1, parseInt(req.query.page) || 1);
			const pageSize = returnAll
				? undefined
				: Math.min(Math.max(1, parseInt(req.query.pageSize) || 100), 500);

			const queryOptions = {
				select: {
					id: true,
					friendly_name: true,
					hostname: true,
					ip: true,
					os_type: true,
					os_version: true,
					architecture: true,
					last_update: true,
					status: true,
					api_id: true,
					agent_version: true,
					auto_update: true,
					created_at: true,
					notes: true,
					host_group_memberships: {
						include: {
							host_groups: {
								select: {
									id: true,
									name: true,
									color: true,
								},
							},
						},
					},
				},
				orderBy: { created_at: "desc" },
			};

			// Add pagination if not requesting all
			if (!returnAll) {
				queryOptions.skip = (page - 1) * pageSize;
				queryOptions.take = pageSize;
			}

			const [hosts, totalCount] = await Promise.all([
				prisma.hosts.findMany(queryOptions),
				prisma.hosts.count(),
			]);

			// Return paginated response with metadata
			res.json({
				data: hosts,
				pagination: returnAll
					? { total: totalCount, page: 1, pageSize: totalCount, totalPages: 1 }
					: {
							total: totalCount,
							page,
							pageSize,
							totalPages: Math.ceil(totalCount / pageSize),
						},
			});
		} catch (error) {
			logger.error("List hosts error:", error);
			res.status(500).json({ error: "Failed to fetch hosts" });
		}
	},
);

// Admin endpoint to delete multiple hosts
router.delete(
	"/bulk",
	authenticateToken,
	requireManageHosts,
	[
		body("hostIds")
			.isArray({ min: 1 })
			.withMessage("At least one host ID is required"),
		body("hostIds.*")
			.isLength({ min: 1 })
			.withMessage("Each host ID must be provided"),
	],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const { hostIds } = req.body;

			// Verify all hosts exist before deletion
			const existingHosts = await prisma.hosts.findMany({
				where: { id: { in: hostIds } },
				select: { id: true, friendly_name: true },
			});

			if (existingHosts.length !== hostIds.length) {
				const foundIds = existingHosts.map((h) => h.id);
				const missingIds = hostIds.filter((id) => !foundIds.includes(id));
				return res.status(404).json({
					error: "Some hosts not found",
					missingIds,
				});
			}

			// Delete all hosts (cascade will handle related data)
			const deleteResult = await prisma.hosts.deleteMany({
				where: { id: { in: hostIds } },
			});

			// Check if all hosts were actually deleted
			if (deleteResult.count !== hostIds.length) {
				logger.warn(
					`Expected to delete ${hostIds.length} hosts, but only deleted ${deleteResult.count}`,
				);
			}

			res.json({
				message: `${deleteResult.count} host${deleteResult.count !== 1 ? "s" : ""} deleted successfully`,
				deletedCount: deleteResult.count,
				requestedCount: hostIds.length,
				deletedHosts: existingHosts.map((h) => ({
					id: h.id,
					friendly_name: h.friendly_name,
				})),
			});
		} catch (error) {
			logger.error("Bulk host deletion error:", error);

			// Handle specific Prisma errors
			if (error.code === "P2025") {
				return res.status(404).json({
					error: "Some hosts were not found or already deleted",
					details:
						"The hosts may have been deleted by another process or do not exist",
				});
			}

			if (error.code === "P2003") {
				return res.status(400).json({
					error: "Cannot delete hosts due to foreign key constraints",
					details: "Some hosts have related data that prevents deletion",
				});
			}

			res.status(500).json({
				error: "Failed to delete hosts",
				details: error.message || "An unexpected error occurred",
			});
		}
	},
);

// Admin endpoint to delete host
router.delete(
	"/:hostId",
	authenticateToken,
	requireManageHosts,
	async (req, res) => {
		try {
			const { hostId } = req.params;

			// Check if host exists first
			const existingHost = await prisma.hosts.findUnique({
				where: { id: hostId },
				select: { id: true, friendly_name: true },
			});

			if (!existingHost) {
				return res.status(404).json({
					error: "Host not found",
					details: "The host may have been deleted or does not exist",
				});
			}

			// Delete host and all related data (cascade)
			await prisma.hosts.delete({
				where: { id: hostId },
			});

			res.json({
				message: "Host deleted successfully",
				deletedHost: {
					id: existingHost.id,
					friendly_name: existingHost.friendly_name,
				},
			});
		} catch (error) {
			logger.error("Host deletion error:", error);

			// Handle specific Prisma errors
			if (error.code === "P2025") {
				return res.status(404).json({
					error: "Host not found",
					details: "The host may have been deleted or does not exist",
				});
			}

			if (error.code === "P2003") {
				return res.status(400).json({
					error: "Cannot delete host due to foreign key constraints",
					details: "The host has related data that prevents deletion",
				});
			}

			res.status(500).json({
				error: "Failed to delete host",
				details: error.message || "An unexpected error occurred",
			});
		}
	},
);

// Force immediate report from agent
router.post(
	"/:hostId/fetch-report",
	authenticateToken,
	requireManageHosts,
	async (req, res) => {
		try {
			const { hostId } = req.params;

			// Get host to verify it exists
			const host = await prisma.hosts.findUnique({
				where: { id: hostId },
			});

			if (!host) {
				return res.status(404).json({ error: "Host not found" });
			}

			// Get the agent-commands queue
			const queue = queueManager.queues[QUEUE_NAMES.AGENT_COMMANDS];

			if (!queue) {
				return res.status(500).json({
					error: "Queue not available",
				});
			}

			// Add job to queue
			const job = await queue.add(
				"report_now",
				{
					api_id: host.api_id,
					type: "report_now",
				},
				{
					attempts: 3,
					backoff: {
						type: "exponential",
						delay: 2000,
					},
				},
			);

			res.json({
				success: true,
				message: "Report fetch queued successfully",
				jobId: job.id,
				host: {
					id: host.id,
					friendlyName: host.friendly_name,
					apiId: host.api_id,
				},
			});
		} catch (error) {
			logger.error("Force fetch report error:", error);
			res.status(500).json({ error: "Failed to fetch report" });
		}
	},
);

// Toggle agent auto-update setting
router.patch(
	"/:hostId/auto-update",
	authenticateToken,
	requireManageHosts,
	[
		body("auto_update")
			.isBoolean()
			.withMessage("Agent auto-update setting must be a boolean"),
	],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const { hostId } = req.params;
			const { auto_update } = req.body;

			// If enabling auto-update on a host, also enable the global setting
			// This makes the per-host toggle the primary control
			let globalEnabled = false;
			if (auto_update) {
				const settings = await prisma.settings.findFirst();
				if (settings && !settings.auto_update) {
					await prisma.settings.update({
						where: { id: settings.id },
						data: { auto_update: true },
					});
					globalEnabled = true;
					logger.info("📊 Global auto-update enabled (triggered by host toggle)");
				}
			}

			const host = await prisma.hosts.update({
				where: { id: hostId },
				data: {
					auto_update: auto_update,
					updated_at: new Date(),
				},
			});

			res.json({
				message: `Agent auto-update ${auto_update ? "enabled" : "disabled"} successfully${globalEnabled ? " (global setting also enabled)" : ""}`,
				host: {
					id: host.id,
					friendlyName: host.friendly_name,
					autoUpdate: host.auto_update,
				},
				globalEnabled: globalEnabled,
			});
		} catch (error) {
			logger.error("Agent auto-update toggle error:", error);
			res.status(500).json({ error: "Failed to toggle agent auto-update" });
		}
	},
);

// Force agent update for specific host
router.post(
	"/:hostId/force-agent-update",
	authenticateToken,
	requireManageHosts,
	async (req, res) => {
		try {
			const { hostId } = req.params;

			// Get host to verify it exists
			const host = await prisma.hosts.findUnique({
				where: { id: hostId },
			});

			if (!host) {
				return res.status(404).json({ error: "Host not found" });
			}

			// Get the agent-commands queue
			const queue = queueManager.queues[QUEUE_NAMES.AGENT_COMMANDS];

			if (!queue) {
				return res.status(500).json({
					error: "Queue not available",
				});
			}

			// Add job to queue with bypass_settings flag for true force updates
			// This allows the force endpoint to bypass auto_update settings
			const job = await queue.add(
				"update_agent",
				{
					api_id: host.api_id,
					type: "update_agent",
					bypass_settings: true, // Force endpoint bypasses settings
				},
				{
					attempts: 3,
					backoff: {
						type: "exponential",
						delay: 2000,
					},
				},
			);

			res.json({
				success: true,
				message: "Agent update queued successfully",
				jobId: job.id,
				host: {
					id: host.id,
					friendlyName: host.friendly_name,
					apiId: host.api_id,
				},
			});
		} catch (error) {
			logger.error("Force agent update error:", error);
			res.status(500).json({
				error: "Failed to queue agent update",
				details: error.message || "Unknown error occurred"
			});
		}
	},
);

// Refresh integration status for specific host
// This triggers the agent to re-scan and report its integration capabilities
router.post(
	"/:hostId/refresh-integration-status",
	authenticateToken,
	requireManageHosts,
	async (req, res) => {
		try {
			const { hostId } = req.params;

			// Get host to verify it exists
			const host = await prisma.hosts.findUnique({
				where: { id: hostId },
			});

			if (!host) {
				return res.status(404).json({ error: "Host not found" });
			}

			// Get the agent-commands queue
			const queue = queueManager.queues[QUEUE_NAMES.AGENT_COMMANDS];

			if (!queue) {
				return res.status(500).json({
					error: "Queue not available",
				});
			}

			// Add job to queue to refresh integration status
			const job = await queue.add(
				"refresh_integration_status",
				{
					api_id: host.api_id,
					type: "refresh_integration_status",
				},
				{
					attempts: 2,
					backoff: {
						type: "exponential",
						delay: 1000,
					},
				},
			);

			res.json({
				success: true,
				message: "Integration status refresh queued",
				jobId: job.id,
				host: {
					id: host.id,
					friendlyName: host.friendly_name,
					apiId: host.api_id,
				},
			});
		} catch (error) {
			logger.error("Refresh integration status error:", error);
			res.status(500).json({
				error: "Failed to refresh integration status",
				details: error.message || "Unknown error occurred"
			});
		}
	},
);

// Serve the installation script (requires API authentication)
router.get("/install", async (req, res) => {
	try {
		// Verify API credentials
		const apiId = req.headers["x-api-id"];
		const apiKey = req.headers["x-api-key"];

		if (!apiId || !apiKey) {
			return res.status(401).json({ error: "API credentials required" });
		}

		// Validate API credentials
		const host = await prisma.hosts.findUnique({
			where: { api_id: apiId },
		});

		if (!host) {
			return res.status(401).json({ error: "Invalid API credentials" });
		}

		// Verify API key (supports both hashed and legacy plaintext keys)
		const isValidKey = await verifyApiKey(apiKey, host.api_key);
		if (!isValidKey) {
			return res.status(401).json({ error: "Invalid API credentials" });
		}

		const fs = require("node:fs");
		const path = require("node:path");

		const scriptPath = path.join(
			__dirname,
			"../../../agents/patchmon_install.sh",
		);

		if (!fs.existsSync(scriptPath)) {
			return res.status(404).json({ error: "Installation script not found" });
		}

		let script = fs.readFileSync(scriptPath, "utf8");

		// Convert Windows line endings to Unix line endings
		script = script.replace(/\r\n/g, "\n").replace(/\r/g, "\n");

		// Get the configured server URL from settings
		let serverUrl = "http://localhost:3001";
		try {
			const settings = await prisma.settings.findFirst();
			if (settings?.server_url) {
				serverUrl = settings.server_url;
			}
		} catch (settingsError) {
			logger.warn(
				"Could not fetch settings, using default server URL:",
				settingsError.message,
			);
		}

		// Determine curl flags dynamically from settings (ignore self-signed)
		let curlFlags = "-s";
		let skipSSLVerify = "false";
		try {
			const settings = await prisma.settings.findFirst();
			if (settings && settings.ignore_ssl_self_signed === true) {
				curlFlags = "-sk";
				skipSSLVerify = "true";
			}
		} catch (sslSettingsError) {
			logger.warn("Could not fetch SSL settings:", sslSettingsError.message);
		}

		// Check for --force parameter
		const forceInstall = req.query.force === "true" || req.query.force === "1";

		// Get architecture parameter (only set if explicitly provided, otherwise let script auto-detect)
		const architecture = req.query.arch;

		// Generate a secure bootstrap token instead of embedding the API key directly
		// The agent will exchange this token for actual credentials via a secure API call
		// IMPORTANT: Use the plaintext apiKey from the request headers, NOT host.api_key (which is the hash)
		const bootstrapToken = await generateBootstrapToken(host.api_id, apiKey);

		// Inject bootstrap token and server URL into the script
		// The actual API credentials are NOT embedded - they will be fetched securely
		const archExport = architecture
			? `export ARCHITECTURE="${architecture}"\n`
			: "";
		const envVars = `#!/bin/sh
export PATCHMON_URL="${serverUrl}"
export BOOTSTRAP_TOKEN="${bootstrapToken}"
export CURL_FLAGS="${curlFlags}"
export SKIP_SSL_VERIFY="${skipSSLVerify}"
export FORCE_INSTALL="${forceInstall ? "true" : "false"}"
${archExport}
# Fetch actual credentials using bootstrap token (one-time use, expires in 5 minutes)
fetch_credentials() {
    CREDS=$(curl \${CURL_FLAGS} -X POST "\${PATCHMON_URL}/api/v1/hosts/bootstrap/exchange" \\
        -H "Content-Type: application/json" \\
        -d "{\\"token\\": \\"\${BOOTSTRAP_TOKEN}\\"}" 2>/dev/null)

    if [ -z "\$CREDS" ] || echo "\$CREDS" | grep -q '"error"'; then
        echo "ERROR: Failed to fetch credentials. Bootstrap token may have expired."
        echo "Please request a new installation script."
        exit 1
    fi

    export API_ID=$(echo "\$CREDS" | grep -o '"apiId":"[^"]*"' | cut -d'"' -f4)
    export API_KEY=$(echo "\$CREDS" | grep -o '"apiKey":"[^"]*"' | cut -d'"' -f4)

    if [ -z "\$API_ID" ] || [ -z "\$API_KEY" ]; then
        echo "ERROR: Invalid credentials received from server."
        exit 1
    fi
}
fetch_credentials
`;

		// Remove the shebang from the original script and prepend our env vars
		script = script.replace(/^#!/, "#");
		script = envVars + script;

		res.setHeader("Content-Type", "text/plain");
		res.setHeader(
			"Content-Disposition",
			'inline; filename="patchmon_install.sh"',
		);
		res.send(script);
	} catch (error) {
		logger.error("Installation script error:", error);
		res.status(500).json({ error: "Failed to serve installation script" });
	}
});

// Note: /check-machine-id endpoint removed - using config.yml checking method instead

// Exchange bootstrap token for actual API credentials (one-time use)
router.post("/bootstrap/exchange", async (req, res) => {
	try {
		const { token } = req.body;

		if (!token) {
			return res.status(400).json({ error: "Bootstrap token required" });
		}

		// Consume the bootstrap token (one-time use)
		const credentials = await consumeBootstrapToken(token);

		if (!credentials) {
			return res.status(401).json({
				error: "Invalid or expired bootstrap token",
			});
		}

		// Return the actual credentials
		res.json({
			apiId: credentials.apiId,
			apiKey: credentials.apiKey,
		});
	} catch (error) {
		logger.error("Bootstrap token exchange error:", error.message);
		res.status(500).json({ error: "Failed to exchange bootstrap token" });
	}
});

// Serve the removal script (public - no authentication required)
// The script is static and only removes PatchMon files from the system
router.get("/remove", async (req, res) => {
	try {
		const fs = require("node:fs");
		const path = require("node:path");

		const scriptPath = path.join(
			__dirname,
			"../../../agents/patchmon_remove.sh",
		);

		if (!fs.existsSync(scriptPath)) {
			return res.status(404).json({ error: "Removal script not found" });
		}

		// Read the script content
		let script = fs.readFileSync(scriptPath, "utf8");

		// Convert line endings
		script = script.replace(/\r\n/g, "\n").replace(/\r/g, "\n");

		// Determine curl flags dynamically from settings for consistency
		let curlFlags = "-s";
		try {
			const settings = await prisma.settings.findFirst();
			if (settings && settings.ignore_ssl_self_signed === true) {
				curlFlags = "-sk";
			}
		} catch (settingsError) {
			logger.warn("Could not fetch settings:", settingsError.message);
		}

		// Prepend environment for CURL_FLAGS so script can use it if needed
		const envPrefix = `#!/bin/sh\nexport CURL_FLAGS="${curlFlags}"\n\n`;
		script = script.replace(/^#!/, "#");
		script = envPrefix + script;

		// Set appropriate headers for script download
		res.setHeader("Content-Type", "text/plain");
		res.setHeader(
			"Content-Disposition",
			'inline; filename="patchmon_remove.sh"',
		);
		res.send(script);
	} catch (error) {
		logger.error("Removal script error:", error.message);
		res.status(500).json({ error: "Failed to serve removal script" });
	}
});

// ==================== AGENT FILE MANAGEMENT ====================

// Get agent file information (admin only)
router.get(
	"/agent/info",
	authenticateToken,
	requireManageSettings,
	async (_req, res) => {
		try {
			const fs = require("node:fs").promises;
			const path = require("node:path");

			const agentPath = path.join(
				__dirname,
				"../../../agents/patchmon-agent.sh",
			);

			try {
				const stats = await fs.stat(agentPath);
				const content = await fs.readFile(agentPath, "utf8");

				// Extract version from agent script (look for AGENT_VERSION= line)
				const versionMatch = content.match(/^AGENT_VERSION="([^"]+)"/m);
				const version = versionMatch ? versionMatch[1] : "unknown";

				res.json({
					exists: true,
					version,
					lastModified: stats.mtime,
					size: stats.size,
					sizeFormatted: `${(stats.size / 1024).toFixed(1)} KB`,
				});
			} catch (error) {
				if (error.code === "ENOENT") {
					res.json({
						exists: false,
						version: null,
						lastModified: null,
						size: 0,
						sizeFormatted: "0 KB",
					});
				} else {
					throw error;
				}
			}
		} catch (error) {
			logger.error("Get agent info error:", error);
			res.status(500).json({ error: "Failed to get agent information" });
		}
	},
);

// Update agent file (admin only)
router.post(
	"/agent/upload",
	authenticateToken,
	requireManageSettings,
	async (req, res) => {
		try {
			const { scriptContent } = req.body;

			if (!scriptContent || typeof scriptContent !== "string") {
				return res.status(400).json({ error: "Script content is required" });
			}

			// Basic validation - check if it looks like a shell script
			if (!scriptContent.trim().startsWith("#!/")) {
				return res.status(400).json({
					error: "Invalid script format - must start with shebang (#!/...)",
				});
			}

			const fs = require("node:fs").promises;
			const path = require("node:path");

			const agentPath = path.join(
				__dirname,
				"../../../agents/patchmon-agent.sh",
			);

			// Create backup of existing file
			try {
				const backupPath = `${agentPath}.backup.${Date.now()}`;
				await fs.copyFile(agentPath, backupPath);
				logger.info(`Created backup: ${backupPath}`);
			} catch (error) {
				// Ignore if original doesn't exist
				if (error.code !== "ENOENT") {
					logger.warn("Failed to create backup:", error.message);
				}
			}

			// Write new agent script
			await fs.writeFile(agentPath, scriptContent, { mode: 0o755 });

			// Get updated file info
			const stats = await fs.stat(agentPath);
			const versionMatch = scriptContent.match(/^AGENT_VERSION="([^"]+)"/m);
			const version = versionMatch ? versionMatch[1] : "unknown";

			res.json({
				message: "Agent script updated successfully",
				version,
				lastModified: stats.mtime,
				size: stats.size,
				sizeFormatted: `${(stats.size / 1024).toFixed(1)} KB`,
			});
		} catch (error) {
			logger.error("Upload agent error:", error);
			res.status(500).json({ error: "Failed to update agent script" });
		}
	},
);

// Get agent file timestamp for update checking (requires API credentials)
router.get("/agent/timestamp", async (req, res) => {
	try {
		// Check for API credentials
		const apiId = req.headers["x-api-id"];
		const apiKey = req.headers["x-api-key"];

		if (!apiId || !apiKey) {
			return res.status(401).json({ error: "API credentials required" });
		}

		// Verify API credentials
		const host = await prisma.hosts.findUnique({
			where: { api_id: apiId },
		});

		if (!host) {
			return res.status(401).json({ error: "Invalid API credentials" });
		}

		// Verify API key using bcrypt (or timing-safe comparison for legacy keys)
		const isValidKey = await verifyApiKey(apiKey, host.api_key);
		if (!isValidKey) {
			return res.status(401).json({ error: "Invalid API credentials" });
		}

		const fs = require("node:fs").promises;
		const path = require("node:path");

		const agentPath = path.join(__dirname, "../../../agents/patchmon-agent.sh");

		try {
			const stats = await fs.stat(agentPath);
			const content = await fs.readFile(agentPath, "utf8");

			// Extract version from agent script
			const versionMatch = content.match(/^AGENT_VERSION="([^"]+)"/m);
			const version = versionMatch ? versionMatch[1] : "unknown";

			res.json({
				version,
				lastModified: stats.mtime,
				timestamp: Math.floor(stats.mtime.getTime() / 1000), // Unix timestamp
				exists: true,
			});
		} catch (error) {
			if (error.code === "ENOENT") {
				res.json({
					version: null,
					lastModified: null,
					timestamp: 0,
					exists: false,
				});
			} else {
				throw error;
			}
		}
	} catch (error) {
		logger.error("Get agent timestamp error:", error);
		res.status(500).json({ error: "Failed to get agent timestamp" });
	}
});

// Get settings for agent (requires API credentials)
router.get("/settings", async (req, res) => {
	try {
		// Check for API credentials
		const apiId = req.headers["x-api-id"];
		const apiKey = req.headers["x-api-key"];

		if (!apiId || !apiKey) {
			return res.status(401).json({ error: "API credentials required" });
		}

		// Verify API credentials
		const host = await prisma.hosts.findUnique({
			where: { api_id: apiId },
		});

		if (!host) {
			return res.status(401).json({ error: "Invalid API credentials" });
		}

		// Verify API key using bcrypt (or timing-safe comparison for legacy keys)
		const isValidKey = await verifyApiKey(apiKey, host.api_key);
		if (!isValidKey) {
			return res.status(401).json({ error: "Invalid API credentials" });
		}

		const settings = await prisma.settings.findFirst();

		// Return both global and host-specific auto-update settings
		res.json({
			auto_update: settings?.auto_update || false,
			host_auto_update: host.auto_update || false,
		});
	} catch (error) {
		logger.error("Get settings error:", error);
		res.status(500).json({ error: "Failed to get settings" });
	}
});

// Update host friendly name (admin only)
router.patch(
	"/:hostId/friendly-name",
	authenticateToken,
	requireManageHosts,
	[
		body("friendly_name")
			.isLength({ min: 1, max: 100 })
			.withMessage("Friendly name must be between 1 and 100 characters"),
	],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const { hostId } = req.params;
			const { friendly_name } = req.body;

			// Check if host exists
			const host = await prisma.hosts.findUnique({
				where: { id: hostId },
			});

			if (!host) {
				return res.status(404).json({ error: "Host not found" });
			}

			// Check if friendly name is already taken by another host
			const existingHost = await prisma.hosts.findFirst({
				where: {
					friendly_name: friendly_name,
					id: { not: hostId },
				},
			});

			if (existingHost) {
				return res
					.status(400)
					.json({ error: "Friendly name is already taken by another host" });
			}

			// Update the friendly name
			const updatedHost = await prisma.hosts.update({
				where: { id: hostId },
				data: { friendly_name: friendly_name },
				select: {
					id: true,
					friendly_name: true,
					hostname: true,
					ip: true,
					os_type: true,
					os_version: true,
					architecture: true,
					last_update: true,
					status: true,
					updated_at: true,
					host_group_memberships: {
						include: {
							host_groups: {
								select: {
									id: true,
									name: true,
									color: true,
								},
							},
						},
					},
				},
			});

			res.json({
				message: "Friendly name updated successfully",
				host: updatedHost,
			});
		} catch (error) {
			logger.error("Update friendly name error:", error);
			res.status(500).json({ error: "Failed to update friendly name" });
		}
	},
);

// Update host IP and hostname (admin only)
router.patch(
	"/:hostId/connection",
	authenticateToken,
	requireManageHosts,
	[
		body("ip")
			.optional()
			.isIP()
			.withMessage("IP must be a valid IP address"),
		body("hostname")
			.optional()
			.isLength({ max: 255 })
			.withMessage("Hostname must be less than 255 characters"),
	],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const { hostId } = req.params;
			const { ip, hostname } = req.body;

			// Check if host exists
			const host = await prisma.hosts.findUnique({
				where: { id: hostId },
			});

			if (!host) {
				return res.status(404).json({ error: "Host not found" });
			}

			// Build update data
			const updateData = {};
			if (ip !== undefined) updateData.ip = ip;
			if (hostname !== undefined) updateData.hostname = hostname;
			updateData.updated_at = new Date();

			// Update the host
			const updatedHost = await prisma.hosts.update({
				where: { id: hostId },
				data: updateData,
				select: {
					id: true,
					friendly_name: true,
					hostname: true,
					ip: true,
					os_type: true,
					os_version: true,
					architecture: true,
					last_update: true,
					status: true,
					updated_at: true,
					host_group_memberships: {
						include: {
							host_groups: {
								select: {
									id: true,
									name: true,
									color: true,
								},
							},
						},
					},
				},
			});

			res.json({
				message: "Host connection information updated successfully",
				host: updatedHost,
			});
		} catch (error) {
			logger.error("Update host connection error:", error);
			res.status(500).json({ error: "Failed to update host connection information" });
		}
	},
);

// Update host notes (admin only)
router.patch(
	"/:hostId/notes",
	authenticateToken,
	requireManageHosts,
	[
		body("notes")
			.optional()
			.isLength({ max: 1000 })
			.withMessage("Notes must be less than 1000 characters"),
	],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const { hostId } = req.params;
			const { notes } = req.body;

			// Check if host exists
			const existingHost = await prisma.hosts.findUnique({
				where: { id: hostId },
			});

			if (!existingHost) {
				return res.status(404).json({ error: "Host not found" });
			}

			// Update the notes
			const updatedHost = await prisma.hosts.update({
				where: { id: hostId },
				data: {
					notes: notes || null,
					updated_at: new Date(),
				},
				select: {
					id: true,
					friendly_name: true,
					hostname: true,
					ip: true,
					os_type: true,
					os_version: true,
					architecture: true,
					last_update: true,
					status: true,
					notes: true,
					host_group_memberships: {
						include: {
							host_groups: {
								select: {
									id: true,
									name: true,
									color: true,
								},
							},
						},
					},
				},
			});

			res.json({
				message: "Notes updated successfully",
				host: updatedHost,
			});
		} catch (error) {
			logger.error("Update notes error:", error);
			res.status(500).json({ error: "Failed to update notes" });
		}
	},
);

// Get integration status for a host
router.get(
	"/:hostId/integrations",
	authenticateToken,
	requireManageHosts,
	async (req, res) => {
		try {
			const { hostId } = req.params;

			// Get host to verify it exists
			const host = await prisma.hosts.findUnique({
				where: { id: hostId },
				select: {
					id: true,
					api_id: true,
					friendly_name: true,
					docker_enabled: true,
					compliance_enabled: true,
				},
			});

			if (!host) {
				return res.status(404).json({ error: "Host not found" });
			}

			// Check if agent is connected
			const connected = isConnected(host.api_id);

			// Get integration states from database (persisted) with cache fallback
			// Database is source of truth, cache is used for quick WebSocket lookups
			const cachedState = integrationStateCache.get(host.api_id) || {};
			const integrations = {
				docker: host.docker_enabled ?? cachedState.docker ?? false,
				compliance: host.compliance_enabled ?? cachedState.compliance ?? false,
			};

			res.json({
				success: true,
				data: {
					integrations,
					connected,
					host: {
						id: host.id,
						friendlyName: host.friendly_name,
						apiId: host.api_id,
					},
				},
			});
		} catch (error) {
			logger.error("Get integration status error:", error);
			res.status(500).json({ error: "Failed to get integration status" });
		}
	},
);

// Get integration setup status for a host (frontend-facing)
router.get(
	"/:hostId/integrations/:integrationName/status",
	authenticateToken,
	async (req, res) => {
		try {
			const { hostId, integrationName } = req.params;

			// Get host to get api_id
			const host = await prisma.hosts.findUnique({
				where: { id: hostId },
				select: { api_id: true },
			});

			if (!host) {
				return res.status(404).json({ error: "Host not found" });
			}

			// Get status from Redis
			const statusKey = `integration_status:${host.api_id}:${integrationName}`;
			const statusData = await redis.get(statusKey);

			if (!statusData) {
				return res.json({
					success: true,
					status: null,
					message: "No status available",
				});
			}

			res.json({
				success: true,
				status: JSON.parse(statusData),
			});
		} catch (error) {
			logger.error("Get integration setup status error:", error);
			res.status(500).json({ error: "Failed to get integration setup status" });
		}
	},
);

// Toggle integration status for a host
router.post(
	"/:hostId/integrations/:integrationName/toggle",
	authenticateToken,
	requireManageHosts,
	[body("enabled").isBoolean().withMessage("Enabled status must be a boolean")],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const { hostId, integrationName } = req.params;
			const { enabled } = req.body;

			// Validate integration name
			const validIntegrations = ["docker", "compliance"];
			if (!validIntegrations.includes(integrationName)) {
				return res.status(400).json({
					error: "Invalid integration name",
					validIntegrations,
				});
			}

			// Get host to verify it exists
			const host = await prisma.hosts.findUnique({
				where: { id: hostId },
				select: {
					id: true,
					api_id: true,
					friendly_name: true,
					docker_enabled: true,
				},
			});

			if (!host) {
				return res.status(404).json({ error: "Host not found" });
			}

			// Check if agent is connected
			if (!isConnected(host.api_id)) {
				return res.status(503).json({
					error: "Agent is not connected",
					message:
						"The agent must be connected via WebSocket to toggle integrations",
				});
			}

			// Send WebSocket message to agent
			const success = pushIntegrationToggle(
				host.api_id,
				integrationName,
				enabled,
			);

			if (!success) {
				return res.status(503).json({
					error: "Failed to send integration toggle",
					message: "Agent connection may have been lost",
				});
			}

			// Persist integration state to database
			if (integrationName === "docker") {
				await prisma.hosts.update({
					where: { id: hostId },
					data: { docker_enabled: enabled },
				});
			} else if (integrationName === "compliance") {
				await prisma.hosts.update({
					where: { id: hostId },
					data: { compliance_enabled: enabled },
				});
			}

			// Update cache with new state (for quick WebSocket lookups)
			if (!integrationStateCache.has(host.api_id)) {
				integrationStateCache.set(host.api_id, {});
			}
			integrationStateCache.get(host.api_id)[integrationName] = enabled;

			res.json({
				success: true,
				message: `Integration ${integrationName} ${enabled ? "enabled" : "disabled"} successfully`,
				data: {
					integration: integrationName,
					enabled,
					host: {
						id: host.id,
						friendlyName: host.friendly_name,
						apiId: host.api_id,
					},
				},
			});
		} catch (error) {
			logger.error("Toggle integration error:", error);
			res.status(500).json({ error: "Failed to toggle integration" });
		}
	},
);

module.exports = router;
