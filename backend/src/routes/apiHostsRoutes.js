const express = require("express");
const logger = require("../utils/logger");
const { getPrismaClient } = require("../config/prisma");
const { authenticateApiToken } = require("../middleware/apiAuth");
const { requireApiScope } = require("../middleware/apiScope");

const router = express.Router();
const prisma = getPrismaClient();

// Helper function to check if a string is a valid UUID
const isUUID = (str) => {
	const uuidRegex =
		/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
	return uuidRegex.test(str);
};

// GET /api/v1/api/hosts - List hosts with IP and groups
/* #swagger.tags = ['Scoped API - Hosts'] */
/* #swagger.summary = 'List hosts with IP, groups, and optional stats' */
/* #swagger.description = 'Retrieve a list of all hosts with their IP addresses and associated host groups. Use include=stats to add package update counts (updates_count, security_updates_count, total_packages) to each host. Requires Basic Auth with scoped credentials (host:get permission).' */
/* #swagger.security = [{ "basicAuth": [] }] */
/* #swagger.parameters['hostgroup'] = {
    in: 'query',
    type: 'string',
    description: 'Filter by host group name(s) or UUID(s). Comma-separated for multiple groups.',
    required: false
} */
/* #swagger.parameters['include'] = {
    in: 'query',
    type: 'string',
    description: 'Comma-separated list of additional data to include. Supported: "stats" (adds updates_count, security_updates_count, total_packages, needs_reboot, os_type, os_version, last_update, status to each host).',
    required: false
} */
router.get(
	"/hosts",
	authenticateApiToken("api"),
	requireApiScope("host", "get"),
	async (req, res) => {
		try {
			const { hostgroup, include } = req.query;
			const includeStats = include
				?.split(",")
				.map((s) => s.trim().toLowerCase())
				.includes("stats");

			let whereClause = {};
			let filterValues = [];

			// Parse hostgroup filter (comma-separated names or UUIDs)
			if (hostgroup) {
				filterValues = hostgroup.split(",").map((g) => g.trim());

				// Separate UUIDs from names
				const uuidFilters = [];
				const nameFilters = [];

				for (const value of filterValues) {
					if (isUUID(value)) {
						uuidFilters.push(value);
					} else {
						nameFilters.push(value);
					}
				}

				// Find host group IDs from names
				const groupIds = [...uuidFilters];

				if (nameFilters.length > 0) {
					const groups = await prisma.host_groups.findMany({
						where: {
							name: {
								in: nameFilters,
							},
						},
						select: {
							id: true,
							name: true,
						},
					});

					// Add found group IDs
					groupIds.push(...groups.map((g) => g.id));

					// Check if any name filters didn't match
					const foundNames = groups.map((g) => g.name);
					const notFoundNames = nameFilters.filter(
						(name) => !foundNames.includes(name),
					);

					if (notFoundNames.length > 0) {
						logger.warn(`Host groups not found: ${notFoundNames.join(", ")}`);
					}
				}

				// Filter hosts by group memberships
				if (groupIds.length > 0) {
					whereClause = {
						host_group_memberships: {
							some: {
								host_group_id: {
									in: groupIds,
								},
							},
						},
					};
				} else {
					// No valid groups found, return empty result
					return res.json({
						hosts: [],
						total: 0,
						filtered_by_groups: filterValues,
					});
				}
			}

			// Build select based on whether stats are requested
			const hostSelect = {
				id: true,
				friendly_name: true,
				hostname: true,
				ip: true,
				host_group_memberships: {
					include: {
						host_groups: {
							select: {
								id: true,
								name: true,
							},
						},
					},
				},
			};

			// Include additional fields when stats are requested
			if (includeStats) {
				hostSelect.os_type = true;
				hostSelect.os_version = true;
				hostSelect.last_update = true;
				hostSelect.status = true;
				hostSelect.needs_reboot = true;
			}

			// Query hosts with groups
			const hosts = await prisma.hosts.findMany({
				where: whereClause,
				select: hostSelect,
				orderBy: {
					friendly_name: "asc",
				},
			});

			// Batch-fetch update counts when stats are requested (efficient: 3 queries total)
			let updateCountMap = new Map();
			let securityUpdateCountMap = new Map();
			let totalCountMap = new Map();

			if (includeStats && hosts.length > 0) {
				const hostIds = hosts.map((h) => h.id);

				const [updateCounts, securityUpdateCounts, totalCounts] =
					await Promise.all([
						prisma.host_packages.groupBy({
							by: ["host_id"],
							where: {
								host_id: { in: hostIds },
								needs_update: true,
							},
							_count: { id: true },
						}),
						prisma.host_packages.groupBy({
							by: ["host_id"],
							where: {
								host_id: { in: hostIds },
								needs_update: true,
								is_security_update: true,
							},
							_count: { id: true },
						}),
						prisma.host_packages.groupBy({
							by: ["host_id"],
							where: {
								host_id: { in: hostIds },
							},
							_count: { id: true },
						}),
					]);

				updateCountMap = new Map(
					updateCounts.map((item) => [item.host_id, item._count.id]),
				);
				securityUpdateCountMap = new Map(
					securityUpdateCounts.map((item) => [item.host_id, item._count.id]),
				);
				totalCountMap = new Map(
					totalCounts.map((item) => [item.host_id, item._count.id]),
				);
			}

			// Format response
			const formattedHosts = hosts.map((host) => {
				const base = {
					id: host.id,
					friendly_name: host.friendly_name,
					hostname: host.hostname,
					ip: host.ip,
					host_groups: host.host_group_memberships.map((membership) => ({
						id: membership.host_groups.id,
						name: membership.host_groups.name,
					})),
				};

				if (includeStats) {
					base.os_type = host.os_type;
					base.os_version = host.os_version;
					base.last_update = host.last_update;
					base.status = host.status;
					base.needs_reboot = host.needs_reboot;
					base.updates_count = updateCountMap.get(host.id) || 0;
					base.security_updates_count =
						securityUpdateCountMap.get(host.id) || 0;
					base.total_packages = totalCountMap.get(host.id) || 0;
				}

				return base;
			});

			res.json({
				hosts: formattedHosts,
				total: formattedHosts.length,
				filtered_by_groups: filterValues.length > 0 ? filterValues : undefined,
			});
		} catch (error) {
			logger.error("Error fetching hosts:", error);
			res.status(500).json({ error: "Failed to fetch hosts" });
		}
	},
);

// GET /api/v1/api/hosts/:id/stats - Get host statistics
/* #swagger.tags = ['Scoped API - Hosts'] */
/* #swagger.summary = 'Get host statistics' */
/* #swagger.description = 'Retrieve package and repository statistics for a specific host. Requires Basic Auth with scoped credentials (host:get permission).' */
/* #swagger.security = [{ "basicAuth": [] }] */
router.get(
	"/hosts/:id/stats",
	authenticateApiToken("api"),
	requireApiScope("host", "get"),
	async (req, res) => {
		try {
			const { id } = req.params;

			// Verify host exists
			const host = await prisma.hosts.findUnique({
				where: { id },
				select: { id: true },
			});

			if (!host) {
				return res.status(404).json({ error: "Host not found" });
			}

			// Calculate statistics for this specific host
			const [
				totalInstalledPackages,
				outdatedPackagesCount,
				securityUpdatesCount,
				totalRepos,
			] = await Promise.all([
				// Total packages installed on this host
				prisma.host_packages.count({
					where: {
						host_id: id,
					},
				}),
				// Total packages that need updates on this host
				prisma.host_packages.count({
					where: {
						host_id: id,
						needs_update: true,
					},
				}),
				// Total packages with security updates on this host
				prisma.host_packages.count({
					where: {
						host_id: id,
						needs_update: true,
						is_security_update: true,
					},
				}),
				// Total repositories associated with this host
				prisma.host_repositories.count({
					where: {
						host_id: id,
					},
				}),
			]);

			res.json({
				host_id: id,
				total_installed_packages: totalInstalledPackages,
				outdated_packages: outdatedPackagesCount,
				security_updates: securityUpdatesCount,
				total_repos: totalRepos,
			});
		} catch (error) {
			logger.error("Error fetching host statistics:", error);
			res.status(500).json({ error: "Failed to fetch host statistics" });
		}
	},
);

// GET /api/v1/api/hosts/:id/info - Get detailed host information
/* #swagger.tags = ['Scoped API - Hosts'] */
/* #swagger.summary = 'Get detailed host information' */
/* #swagger.description = 'Retrieve detailed information about a specific host including OS details, hostname, IP, and host groups. Requires Basic Auth with scoped credentials (host:get permission).' */
/* #swagger.security = [{ "basicAuth": [] }] */
router.get(
	"/hosts/:id/info",
	authenticateApiToken("api"),
	requireApiScope("host", "get"),
	async (req, res) => {
		try {
			const { id } = req.params;

			const host = await prisma.hosts.findUnique({
				where: { id },
				select: {
					id: true,
					machine_id: true,
					friendly_name: true,
					hostname: true,
					ip: true,
					os_type: true,
					os_version: true,
					agent_version: true,
					host_group_memberships: {
						include: {
							host_groups: {
								select: {
									id: true,
									name: true,
								},
							},
						},
					},
				},
			});

			if (!host) {
				return res.status(404).json({ error: "Host not found" });
			}

			res.json({
				id: host.id,
				machine_id: host.machine_id,
				friendly_name: host.friendly_name,
				hostname: host.hostname,
				ip: host.ip,
				os_type: host.os_type,
				os_version: host.os_version,
				agent_version: host.agent_version,
				host_groups: host.host_group_memberships.map((membership) => ({
					id: membership.host_groups.id,
					name: membership.host_groups.name,
				})),
			});
		} catch (error) {
			logger.error("Error fetching host info:", error);
			res.status(500).json({ error: "Failed to fetch host information" });
		}
	},
);

// GET /api/v1/api/hosts/:id/network - Get host network information
/* #swagger.tags = ['Scoped API - Hosts'] */
/* #swagger.summary = 'Get host network information' */
/* #swagger.description = 'Retrieve network configuration details for a specific host including IP address, gateway, DNS servers, and network interfaces. Requires Basic Auth with scoped credentials (host:get permission).' */
/* #swagger.security = [{ "basicAuth": [] }] */
router.get(
	"/hosts/:id/network",
	authenticateApiToken("api"),
	requireApiScope("host", "get"),
	async (req, res) => {
		try {
			const { id } = req.params;

			const host = await prisma.hosts.findUnique({
				where: { id },
				select: {
					id: true,
					ip: true,
					gateway_ip: true,
					dns_servers: true,
					network_interfaces: true,
				},
			});

			if (!host) {
				return res.status(404).json({ error: "Host not found" });
			}

			res.json({
				id: host.id,
				ip: host.ip,
				gateway_ip: host.gateway_ip,
				dns_servers: host.dns_servers || [],
				network_interfaces: host.network_interfaces || [],
			});
		} catch (error) {
			logger.error("Error fetching host network info:", error);
			res
				.status(500)
				.json({ error: "Failed to fetch host network information" });
		}
	},
);

// GET /api/v1/api/hosts/:id/system - Get host system information
/* #swagger.tags = ['Scoped API - Hosts'] */
/* #swagger.summary = 'Get host system information' */
/* #swagger.description = 'Retrieve system-level information for a specific host including architecture, kernel version, CPU, RAM, disk details, and reboot status. Requires Basic Auth with scoped credentials (host:get permission).' */
/* #swagger.security = [{ "basicAuth": [] }] */
router.get(
	"/hosts/:id/system",
	authenticateApiToken("api"),
	requireApiScope("host", "get"),
	async (req, res) => {
		try {
			const { id } = req.params;

			const host = await prisma.hosts.findUnique({
				where: { id },
				select: {
					id: true,
					architecture: true,
					kernel_version: true,
					installed_kernel_version: true,
					selinux_status: true,
					system_uptime: true,
					cpu_model: true,
					cpu_cores: true,
					ram_installed: true,
					swap_size: true,
					load_average: true,
					disk_details: true,
					needs_reboot: true,
					reboot_reason: true,
				},
			});

			if (!host) {
				return res.status(404).json({ error: "Host not found" });
			}

			res.json({
				id: host.id,
				architecture: host.architecture,
				kernel_version: host.kernel_version,
				installed_kernel_version: host.installed_kernel_version,
				selinux_status: host.selinux_status,
				system_uptime: host.system_uptime,
				cpu_model: host.cpu_model,
				cpu_cores: host.cpu_cores,
				ram_installed: host.ram_installed,
				swap_size: host.swap_size,
				load_average: host.load_average || {},
				disk_details: host.disk_details || [],
				needs_reboot: host.needs_reboot,
				reboot_reason: host.reboot_reason,
			});
		} catch (error) {
			logger.error("Error fetching host system info:", error);
			res
				.status(500)
				.json({ error: "Failed to fetch host system information" });
		}
	},
);

// GET /api/v1/api/hosts/:id/package_reports - Get host package update reports
/* #swagger.tags = ['Scoped API - Hosts'] */
/* #swagger.summary = 'Get host package update reports' */
/* #swagger.description = 'Retrieve package update history reports for a specific host. Requires Basic Auth with scoped credentials (host:get permission).' */
/* #swagger.security = [{ "basicAuth": [] }] */
/* #swagger.parameters['limit'] = {
    in: 'query',
    type: 'integer',
    description: 'Maximum number of reports to return (default: 10)',
    required: false
} */
router.get(
	"/hosts/:id/package_reports",
	authenticateApiToken("api"),
	requireApiScope("host", "get"),
	async (req, res) => {
		try {
			const { id } = req.params;
			const { limit = 10 } = req.query;

			// Verify host exists
			const host = await prisma.hosts.findUnique({
				where: { id },
				select: { id: true },
			});

			if (!host) {
				return res.status(404).json({ error: "Host not found" });
			}

			// Get update history
			const reports = await prisma.update_history.findMany({
				where: { host_id: id },
				orderBy: { timestamp: "desc" },
				take: Number.parseInt(limit, 10),
				select: {
					id: true,
					status: true,
					timestamp: true,
					total_packages: true,
					packages_count: true,
					security_count: true,
					payload_size_kb: true,
					execution_time: true,
					error_message: true,
				},
			});

			res.json({
				host_id: id,
				reports: reports.map((report) => ({
					id: report.id,
					status: report.status,
					date: report.timestamp,
					total_packages: report.total_packages,
					outdated_packages: report.packages_count,
					security_updates: report.security_count,
					payload_kb: report.payload_size_kb,
					execution_time_seconds: report.execution_time,
					error_message: report.error_message,
				})),
				total: reports.length,
			});
		} catch (error) {
			logger.error("Error fetching host package reports:", error);
			res.status(500).json({ error: "Failed to fetch host package reports" });
		}
	},
);

// GET /api/v1/api/hosts/:id/agent_queue - Get host agent queue status and jobs
/* #swagger.tags = ['Scoped API - Hosts'] */
/* #swagger.summary = 'Get host agent queue status and jobs' */
/* #swagger.description = 'Retrieve agent queue status and job history for a specific host. Includes queue statistics (waiting, active, delayed, failed) and recent job history. Requires Basic Auth with scoped credentials (host:get permission).' */
/* #swagger.security = [{ "basicAuth": [] }] */
/* #swagger.parameters['limit'] = {
    in: 'query',
    type: 'integer',
    description: 'Maximum number of jobs to return (default: 10)',
    required: false
} */
router.get(
	"/hosts/:id/agent_queue",
	authenticateApiToken("api"),
	requireApiScope("host", "get"),
	async (req, res) => {
		try {
			const { id } = req.params;
			const { limit = 10 } = req.query;

			// Verify host exists
			const host = await prisma.hosts.findUnique({
				where: { id },
				select: { id: true, api_id: true },
			});

			if (!host) {
				return res.status(404).json({ error: "Host not found" });
			}

			// Get job history for this host
			const jobs = await prisma.job_history.findMany({
				where: { host_id: id },
				orderBy: { created_at: "desc" },
				take: Number.parseInt(limit, 10),
				select: {
					id: true,
					job_id: true,
					job_name: true,
					status: true,
					attempt_number: true,
					created_at: true,
					completed_at: true,
					error_message: true,
					output: true,
				},
			});

			// Get queue statistics (if queue manager is available)
			let queueStats = {
				waiting: 0,
				active: 0,
				delayed: 0,
				failed: 0,
			};

			try {
				// Try to get live queue stats from Bull/BullMQ if available
				const { queueManager } = require("../services/automation");
				if (queueManager?.getHostJobs) {
					const hostQueueData = await queueManager.getHostJobs(
						host.api_id,
						Number.parseInt(limit, 10),
					);
					queueStats = {
						waiting: hostQueueData.waiting || 0,
						active: hostQueueData.active || 0,
						delayed: hostQueueData.delayed || 0,
						failed: hostQueueData.failed || 0,
					};
				}
			} catch (queueError) {
				logger.warn("Could not fetch live queue stats:", queueError.message);
			}

			res.json({
				host_id: id,
				queue_status: queueStats,
				job_history: jobs.map((job) => ({
					id: job.id,
					job_id: job.job_id,
					job_name: job.job_name,
					status: job.status,
					attempt: job.attempt_number,
					created_at: job.created_at,
					completed_at: job.completed_at,
					error_message: job.error_message,
					output: job.output,
				})),
				total_jobs: jobs.length,
			});
		} catch (error) {
			logger.error("Error fetching host agent queue:", error);
			res.status(500).json({ error: "Failed to fetch host agent queue" });
		}
	},
);

// GET /api/v1/api/hosts/:id/notes - Get host notes
/* #swagger.tags = ['Scoped API - Hosts'] */
/* #swagger.summary = 'Get host notes' */
/* #swagger.description = 'Retrieve notes associated with a specific host. Requires Basic Auth with scoped credentials (host:get permission).' */
/* #swagger.security = [{ "basicAuth": [] }] */
router.get(
	"/hosts/:id/notes",
	authenticateApiToken("api"),
	requireApiScope("host", "get"),
	async (req, res) => {
		try {
			const { id } = req.params;

			const host = await prisma.hosts.findUnique({
				where: { id },
				select: {
					id: true,
					notes: true,
				},
			});

			if (!host) {
				return res.status(404).json({ error: "Host not found" });
			}

			res.json({
				host_id: id,
				notes: host.notes || "",
			});
		} catch (error) {
			logger.error("Error fetching host notes:", error);
			res.status(500).json({ error: "Failed to fetch host notes" });
		}
	},
);

// GET /api/v1/api/hosts/:id/integrations - Get host integrations status
/* #swagger.tags = ['Scoped API - Hosts'] */
/* #swagger.summary = 'Get host integrations status' */
/* #swagger.description = 'Retrieve integration status and details for a specific host (e.g., Docker containers, volumes, networks). Requires Basic Auth with scoped credentials (host:get permission).' */
/* #swagger.security = [{ "basicAuth": [] }] */
router.get(
	"/hosts/:id/integrations",
	authenticateApiToken("api"),
	requireApiScope("host", "get"),
	async (req, res) => {
		try {
			const { id } = req.params;

			const host = await prisma.hosts.findUnique({
				where: { id },
				select: {
					id: true,
					docker_enabled: true,
				},
			});

			if (!host) {
				return res.status(404).json({ error: "Host not found" });
			}

			// Get Docker integration details if enabled
			let dockerDetails = null;
			if (host.docker_enabled) {
				const [containers, volumes, networks] = await Promise.all([
					prisma.docker_containers.count({
						where: { host_id: id },
					}),
					prisma.docker_volumes.count({
						where: { host_id: id },
					}),
					prisma.docker_networks.count({
						where: { host_id: id },
					}),
				]);

				dockerDetails = {
					enabled: true,
					containers_count: containers,
					volumes_count: volumes,
					networks_count: networks,
					description:
						"Monitor Docker containers, images, volumes, and networks. Collects real-time container status events.",
				};
			}

			res.json({
				host_id: id,
				integrations: {
					docker: dockerDetails || {
						enabled: false,
						description:
							"Monitor Docker containers, images, volumes, and networks. Collects real-time container status events.",
					},
				},
			});
		} catch (error) {
			logger.error("Error fetching host integrations:", error);
			res.status(500).json({ error: "Failed to fetch host integrations" });
		}
	},
);

// GET /api/v1/api/hosts/:id/packages - Paketliste fuer einen Host
// Optional: ?updates_only=true - nur Pakete mit verfuegbaren Updates
/* #swagger.tags = ['Hosts'] */
/* #swagger.summary = 'Get host packages' */
/* #swagger.description = 'Retrieve the list of packages installed on a specific host. Use the optional query parameter ?updates_only=true to return only packages with available updates. Requires Basic Auth with scoped credentials (host:get permission).' */
/* #swagger.security = [{ "basicAuth": [] }] */
router.get(
	"/hosts/:id/packages",
	authenticateApiToken("api"),
	requireApiScope("host", "get"),
	async (req, res) => {
		try {
			const { id } = req.params;
			const { updates_only } = req.query;

			// Host pruefen
			const host = await prisma.hosts.findUnique({
				where: { id },
				select: { id: true, hostname: true, friendly_name: true },
			});

			if (!host) {
				return res.status(404).json({ error: "Host not found" });
			}

			// Paketfilter
			const whereClause = { host_id: id };
			if (updates_only === "true") {
				whereClause.needs_update = true;
			}

			const packages = await prisma.host_packages.findMany({
				where: whereClause,
				select: {
					id: true,
					current_version: true,
					available_version: true,
					needs_update: true,
					is_security_update: true,
					last_checked: true,
					packages: {
						select: {
							name: true,
							description: true,
							category: true,
						},
					},
				},
				orderBy: [{ is_security_update: "desc" }, { needs_update: "desc" }],
			});

			const formattedPackages = packages.map((hp) => ({
				id: hp.id,
				name: hp.packages.name,
				description: hp.packages.description,
				category: hp.packages.category,
				current_version: hp.current_version,
				available_version: hp.available_version,
				needs_update: hp.needs_update,
				is_security_update: hp.is_security_update,
				last_checked: hp.last_checked,
			}));

			res.json({
				host: {
					id: host.id,
					hostname: host.hostname,
					friendly_name: host.friendly_name,
				},
				packages: formattedPackages,
				total: formattedPackages.length,
			});
		} catch (error) {
			logger.error("Error fetching host packages:", error);
			res.status(500).json({ error: "Failed to fetch host packages" });
		}
	},
);

// DELETE /api/v1/api/hosts/:id - Delete a host
/* #swagger.tags = ['Scoped API - Hosts'] */
/* #swagger.summary = 'Delete a host' */
/* #swagger.description = 'Delete a specific host and all related data (cascade). Requires Basic Auth with scoped credentials (host:delete permission).' */
/* #swagger.security = [{ "basicAuth": [] }] */
router.delete(
	"/hosts/:id",
	authenticateApiToken("api"),
	requireApiScope("host", "delete"),
	async (req, res) => {
		try {
			const { id } = req.params;

			// Validate UUID format
			if (!isUUID(id)) {
				return res.status(400).json({ error: "Invalid host ID format" });
			}

			// Check if host exists first
			const host = await prisma.hosts.findUnique({
				where: { id },
				select: { id: true, friendly_name: true, hostname: true },
			});

			if (!host) {
				return res.status(404).json({ error: "Host not found" });
			}

			// Delete host and all related data (cascade)
			await prisma.hosts.delete({ where: { id } });

			res.json({
				message: "Host deleted successfully",
				deleted: {
					id: host.id,
					friendly_name: host.friendly_name,
					hostname: host.hostname,
				},
			});
		} catch (error) {
			logger.error("Error deleting host via scoped API:", error);

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

			res.status(500).json({ error: "Failed to delete host" });
		}
	},
);

module.exports = router;
