const express = require("express");
const { PrismaClient } = require("@prisma/client");
const { body, validationResult } = require("express-validator");
const { v4: uuidv4 } = require("uuid");
const crypto = require("node:crypto");
const _path = require("node:path");
const _fs = require("node:fs");
const { authenticateToken, _requireAdmin } = require("../middleware/auth");
const {
	requireManageHosts,
	requireManageSettings,
} = require("../middleware/permissions");

const router = express.Router();
const prisma = new PrismaClient();

// Secure endpoint to download the agent script (requires API authentication)
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

		if (!host || host.api_key !== apiKey) {
			return res.status(401).json({ error: "Invalid API credentials" });
		}

		// Serve agent script directly from file system
		const fs = require("node:fs");
		const path = require("node:path");

		const agentPath = path.join(__dirname, "../../../agents/patchmon-agent.sh");

		if (!fs.existsSync(agentPath)) {
			return res.status(404).json({ error: "Agent script not found" });
		}

		// Read file and convert line endings
		let scriptContent = fs
			.readFileSync(agentPath, "utf8")
			.replace(/\r\n/g, "\n")
			.replace(/\r/g, "\n");

		// Determine curl flags dynamically from settings for consistency
		let curlFlags = "-s";
		try {
			const settings = await prisma.settings.findFirst();
			if (settings && settings.ignore_ssl_self_signed === true) {
				curlFlags = "-sk";
			}
		} catch (_) {}

		// Inject the curl flags into the script
		scriptContent = scriptContent.replace(
			'CURL_FLAGS=""',
			`CURL_FLAGS="${curlFlags}"`,
		);

		res.setHeader("Content-Type", "application/x-shellscript");
		res.setHeader(
			"Content-Disposition",
			'attachment; filename="patchmon-agent.sh"',
		);
		res.send(scriptContent);
	} catch (error) {
		console.error("Agent download error:", error);
		res.status(500).json({ error: "Failed to download agent script" });
	}
});

// Version check endpoint for agents
router.get("/agent/version", async (_req, res) => {
	try {
		const fs = require("node:fs");
		const path = require("node:path");

		// Read version directly from agent script file
		const agentPath = path.join(__dirname, "../../../agents/patchmon-agent.sh");

		if (!fs.existsSync(agentPath)) {
			return res.status(404).json({ error: "Agent script not found" });
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
			downloadUrl: `/api/v1/hosts/agent/download`,
			releaseNotes: `PatchMon Agent v${currentVersion}`,
			minServerVersion: null,
		});
	} catch (error) {
		console.error("Version check error:", error);
		res.status(500).json({ error: "Failed to get agent version" });
	}
});

// Generate API credentials
const generateApiCredentials = () => {
	const apiId = `patchmon_${crypto.randomBytes(8).toString("hex")}`;
	const apiKey = crypto.randomBytes(32).toString("hex");
	return { apiId, apiKey };
};

// Middleware to validate API credentials
const validateApiCredentials = async (req, res, next) => {
	try {
		const apiId = req.headers["x-api-id"] || req.body.apiId;
		const apiKey = req.headers["x-api-key"] || req.body.apiKey;

		if (!apiId || !apiKey) {
			return res.status(401).json({ error: "API ID and Key required" });
		}

		const host = await prisma.hosts.findFirst({
			where: {
				api_id: apiId,
				api_key: apiKey,
			},
		});

		if (!host) {
			return res.status(401).json({ error: "Invalid API credentials" });
		}

		req.hostRecord = host;
		next();
	} catch (error) {
		console.error("API credential validation error:", error);
		res.status(500).json({ error: "API credential validation failed" });
	}
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
		body("hostGroupId").optional(),
	],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const { friendly_name, hostGroupId } = req.body;

			// Generate unique API credentials for this host
			const { apiId, apiKey } = generateApiCredentials();

			// If hostGroupId is provided, verify the group exists
			if (hostGroupId) {
				const hostGroup = await prisma.host_groups.findUnique({
					where: { id: hostGroupId },
				});

				if (!hostGroup) {
					return res.status(400).json({ error: "Host group not found" });
				}
			}

			// Create new host with API credentials - system info will be populated when agent connects
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
					api_key: apiKey,
					host_group_id: hostGroupId || null,
					status: "pending", // Will change to 'active' when agent connects
					updated_at: new Date(),
				},
				include: {
					host_groups: {
						select: {
							id: true,
							name: true,
							color: true,
						},
					},
				},
			});

			res.status(201).json({
				message: "Host created successfully",
				hostId: host.id,
				friendlyName: host.friendly_name,
				apiId: host.api_id,
				apiKey: host.api_key,
				hostGroup: host.host_groups,
				instructions:
					"Use these credentials in your patchmon agent configuration. System information will be automatically detected when the agent connects.",
			});
		} catch (error) {
			console.error("Host creation error:", error);
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

// Update host information and packages (now uses API credentials)
router.post(
	"/update",
	validateApiCredentials,
	[
		body("packages").isArray().withMessage("Packages must be an array"),
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
			.optional()
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

			// Update machine_id if provided and current one is a placeholder
			if (req.body.machineId && host.machine_id.startsWith("pending-")) {
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
			if (req.body.gatewayIp) updateData.gateway_ip = req.body.gatewayIp;
			if (req.body.dnsServers) updateData.dns_servers = req.body.dnsServers;
			if (req.body.networkInterfaces)
				updateData.network_interfaces = req.body.networkInterfaces;

			// System Information
			if (req.body.kernelVersion)
				updateData.kernel_version = req.body.kernelVersion;
			if (req.body.selinuxStatus)
				updateData.selinux_status = req.body.selinuxStatus;
			if (req.body.systemUptime)
				updateData.system_uptime = req.body.systemUptime;
			if (req.body.loadAverage) updateData.load_average = req.body.loadAverage;

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
			await prisma.$transaction(async (tx) => {
				// Update host data
				await tx.hosts.update({
					where: { id: host.id },
					data: updateData,
				});

				// Clear existing host packages to avoid duplicates
				await tx.host_packages.deleteMany({
					where: { host_id: host.id },
				});

				// Process each package
				for (const packageData of packages) {
					// Find or create package
					let pkg = await tx.packages.findUnique({
						where: { name: packageData.name },
					});

					if (!pkg) {
						pkg = await tx.packages.create({
							data: {
								id: uuidv4(),
								name: packageData.name,
								description: packageData.description || null,
								category: packageData.category || null,
								latest_version:
									packageData.availableVersion || packageData.currentVersion,
								updated_at: new Date(),
							},
						});
					} else {
						// Update package latest version if newer
						if (
							packageData.availableVersion &&
							packageData.availableVersion !== pkg.latest_version
						) {
							await tx.packages.update({
								where: { id: pkg.id },
								data: {
									latest_version: packageData.availableVersion,
									updated_at: new Date(),
								},
							});
						}
					}

					// Create host package relationship
					// Use upsert to handle potential duplicates gracefully
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
			}, {
				maxWait: 5000, // default: 2000
				timeout: 10000 // default: 5000
			});

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
			console.error("Host update error:", error);

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
				console.error("Failed to log update error:", logError);
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
		console.error("Get host info error:", error);
		res.status(500).json({ error: "Failed to fetch host information" });
	}
});

// Ping endpoint for health checks (now uses API credentials)
router.post("/ping", validateApiCredentials, async (req, res) => {
	try {
		// Update last update timestamp
		await prisma.hosts.update({
			where: { id: req.hostRecord.id },
			data: {
				last_update: new Date(),
				updated_at: new Date(),
			},
		});

		const response = {
			message: "Ping successful",
			timestamp: new Date().toISOString(),
			friendlyName: req.hostRecord.friendly_name,
		};

		// Check if this is a crontab update trigger
		if (req.body.triggerCrontabUpdate && req.hostRecord.auto_update) {
			console.log(
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
		console.error("Ping error:", error);
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

			const host = await prisma.hosts.findUnique({
				where: { id: hostId },
			});

			if (!host) {
				return res.status(404).json({ error: "Host not found" });
			}

			// Generate new API credentials
			const { apiId, apiKey } = generateApiCredentials();

			// Update host with new credentials
			const updatedHost = await prisma.hosts.update({
				where: { id: hostId },
				data: {
					api_id: apiId,
					api_key: apiKey,
					updated_at: new Date(),
				},
			});

			res.json({
				message: "API credentials regenerated successfully",
				hostname: updatedHost.hostname,
				apiId: updatedHost.api_id,
				apiKey: updatedHost.api_key,
				warning:
					"Previous credentials are now invalid. Update your agent configuration.",
			});
		} catch (error) {
			console.error("Credential regeneration error:", error);
			res.status(500).json({ error: "Failed to regenerate credentials" });
		}
	},
);

// Admin endpoint to bulk update host groups
router.put(
	"/bulk/group",
	authenticateToken,
	requireManageHosts,
	[
		body("hostIds").isArray().withMessage("Host IDs must be an array"),
		body("hostIds.*")
			.isLength({ min: 1 })
			.withMessage("Each host ID must be provided"),
		body("hostGroupId").optional(),
	],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const { hostIds, hostGroupId } = req.body;

			// If hostGroupId is provided, verify the group exists
			if (hostGroupId) {
				const hostGroup = await prisma.host_groups.findUnique({
					where: { id: hostGroupId },
				});

				if (!hostGroup) {
					return res.status(400).json({ error: "Host group not found" });
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

			// Bulk update host groups
			const updateResult = await prisma.hosts.updateMany({
				where: { id: { in: hostIds } },
				data: {
					host_group_id: hostGroupId || null,
					updated_at: new Date(),
				},
			});

			// Get updated hosts with group information
			const updatedHosts = await prisma.hosts.findMany({
				where: { id: { in: hostIds } },
				select: {
					id: true,
					friendly_name: true,
					host_groups: {
						select: {
							id: true,
							name: true,
							color: true,
						},
					},
				},
			});

			res.json({
				message: `Successfully updated ${updateResult.count} host${updateResult.count !== 1 ? "s" : ""}`,
				updatedCount: updateResult.count,
				hosts: updatedHosts,
			});
		} catch (error) {
			console.error("Bulk host group update error:", error);
			res.status(500).json({ error: "Failed to update host groups" });
		}
	},
);

// Admin endpoint to update host group
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

			// Check if host exists
			const host = await prisma.hosts.findUnique({
				where: { id: hostId },
			});

			if (!host) {
				return res.status(404).json({ error: "Host not found" });
			}

			// If hostGroupId is provided, verify the group exists
			if (hostGroupId) {
				const hostGroup = await prisma.host_groups.findUnique({
					where: { id: hostGroupId },
				});

				if (!hostGroup) {
					return res.status(400).json({ error: "Host group not found" });
				}
			}

			// Update host group
			const updatedHost = await prisma.hosts.update({
				where: { id: hostId },
				data: {
					host_group_id: hostGroupId || null,
					updated_at: new Date(),
				},
				include: {
					host_groups: {
						select: {
							id: true,
							name: true,
							color: true,
						},
					},
				},
			});

			res.json({
				message: "Host group updated successfully",
				host: updatedHost,
			});
		} catch (error) {
			console.error("Host group update error:", error);
			res.status(500).json({ error: "Failed to update host group" });
		}
	},
);

// Admin endpoint to list all hosts
router.get(
	"/admin/list",
	authenticateToken,
	requireManageHosts,
	async (_req, res) => {
		try {
			const hosts = await prisma.hosts.findMany({
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
					host_group_id: true,
					notes: true,
					host_groups: {
						select: {
							id: true,
							name: true,
							color: true,
						},
					},
				},
				orderBy: { created_at: "desc" },
			});

			res.json(hosts);
		} catch (error) {
			console.error("List hosts error:", error);
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
				console.warn(
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
			console.error("Bulk host deletion error:", error);

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
			console.error("Host deletion error:", error);

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

			const host = await prisma.hosts.update({
				where: { id: hostId },
				data: {
					auto_update: auto_update,
					updated_at: new Date(),
				},
			});

			res.json({
				message: `Agent auto-update ${auto_update ? "enabled" : "disabled"} successfully`,
				host: {
					id: host.id,
					friendlyName: host.friendly_name,
					autoUpdate: host.auto_update,
				},
			});
		} catch (error) {
			console.error("Agent auto-update toggle error:", error);
			res.status(500).json({ error: "Failed to toggle agent auto-update" });
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

		if (!host || host.api_key !== apiKey) {
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
			console.warn(
				"Could not fetch settings, using default server URL:",
				settingsError.message,
			);
		}

		// Determine curl flags dynamically from settings (ignore self-signed)
		let curlFlags = "-s";
		try {
			const settings = await prisma.settings.findFirst();
			if (settings && settings.ignore_ssl_self_signed === true) {
				curlFlags = "-sk";
			}
		} catch (_) {}

		// Check for --force parameter
		const forceInstall = req.query.force === "true" || req.query.force === "1";

		// Inject the API credentials, server URL, curl flags, and force flag into the script
		const envVars = `#!/bin/bash
export PATCHMON_URL="${serverUrl}"
export API_ID="${host.api_id}"
export API_KEY="${host.api_key}"
export CURL_FLAGS="${curlFlags}"
export FORCE_INSTALL="${forceInstall ? "true" : "false"}"

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
		console.error("Installation script error:", error);
		res.status(500).json({ error: "Failed to serve installation script" });
	}
});

// Check if machine_id already exists (requires auth)
router.post("/check-machine-id", validateApiCredentials, async (req, res) => {
	try {
		const { machine_id } = req.body;

		if (!machine_id) {
			return res.status(400).json({
				error: "machine_id is required",
			});
		}

		// Check if a host with this machine_id exists
		const existing_host = await prisma.hosts.findUnique({
			where: { machine_id },
			select: {
				id: true,
				friendly_name: true,
				machine_id: true,
				api_id: true,
				status: true,
				created_at: true,
			},
		});

		if (existing_host) {
			return res.status(200).json({
				exists: true,
				host: existing_host,
				message: "This machine is already enrolled",
			});
		}

		return res.status(200).json({
			exists: false,
			message: "Machine not yet enrolled",
		});
	} catch (error) {
		console.error("Error checking machine_id:", error);
		res.status(500).json({ error: "Failed to check machine_id" });
	}
});

// Serve the removal script (public endpoint - no authentication required)
router.get("/remove", async (_req, res) => {
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
		} catch (_) {}

		// Prepend environment for CURL_FLAGS so script can use it if needed
		const envPrefix = `#!/bin/bash\nexport CURL_FLAGS="${curlFlags}"\n\n`;
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
		console.error("Removal script error:", error);
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
			console.error("Get agent info error:", error);
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
				console.log(`Created backup: ${backupPath}`);
			} catch (error) {
				// Ignore if original doesn't exist
				if (error.code !== "ENOENT") {
					console.warn("Failed to create backup:", error.message);
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
			console.error("Upload agent error:", error);
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
		const host = await prisma.hosts.findFirst({
			where: {
				api_id: apiId,
				api_key: apiKey,
			},
		});

		if (!host) {
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
		console.error("Get agent timestamp error:", error);
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
		const host = await prisma.hosts.findFirst({
			where: {
				api_id: apiId,
				api_key: apiKey,
			},
		});

		if (!host) {
			return res.status(401).json({ error: "Invalid API credentials" });
		}

		const settings = await prisma.settings.findFirst();

		// Return both global and host-specific auto-update settings
		res.json({
			auto_update: settings?.auto_update || false,
			host_auto_update: host.auto_update || false,
		});
	} catch (error) {
		console.error("Get settings error:", error);
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
					host_group_id: true,
					agent_version: true,
					auto_update: true,
					created_at: true,
					updated_at: true,
					host_groups: {
						select: {
							id: true,
							name: true,
							color: true,
						},
					},
				},
			});

			res.json({
				message: "Friendly name updated successfully",
				host: updatedHost,
			});
		} catch (error) {
			console.error("Update friendly name error:", error);
			res.status(500).json({ error: "Failed to update friendly name" });
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
					host_group_id: true,
					agent_version: true,
					auto_update: true,
					created_at: true,
					updated_at: true,
					notes: true,
					host_groups: {
						select: {
							id: true,
							name: true,
							color: true,
						},
					},
				},
			});

			res.json({
				message: "Notes updated successfully",
				host: updatedHost,
			});
		} catch (error) {
			console.error("Update notes error:", error);
			res.status(500).json({ error: "Failed to update notes" });
		}
	},
);

module.exports = router;
