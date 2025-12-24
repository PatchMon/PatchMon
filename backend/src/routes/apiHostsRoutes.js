const express = require("express");
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
router.get(
	"/hosts",
	authenticateApiToken("api"),
	requireApiScope("host", "get"),
	async (req, res) => {
		try {
			const { hostgroup } = req.query;

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
						console.warn(`Host groups not found: ${notFoundNames.join(", ")}`);
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

			// Query hosts with groups
			const hosts = await prisma.hosts.findMany({
				where: whereClause,
				select: {
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
				},
				orderBy: {
					friendly_name: "asc",
				},
			});

			// Format response
			const formattedHosts = hosts.map((host) => ({
				id: host.id,
				friendly_name: host.friendly_name,
				hostname: host.hostname,
				ip: host.ip,
				host_groups: host.host_group_memberships.map((membership) => ({
					id: membership.host_groups.id,
					name: membership.host_groups.name,
				})),
			}));

			res.json({
				hosts: formattedHosts,
				total: formattedHosts.length,
				filtered_by_groups: filterValues.length > 0 ? filterValues : undefined,
			});
		} catch (error) {
			console.error("Error fetching hosts:", error);
			res.status(500).json({ error: "Failed to fetch hosts" });
		}
	},
);

// GET /api/v1/api/hosts/:id/stats - Get host statistics
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
			const [totalInstalledPackages, outdatedPackagesCount, securityUpdatesCount, totalRepos] = await Promise.all([
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
			console.error("Error fetching host statistics:", error);
			res.status(500).json({ error: "Failed to fetch host statistics" });
		}
	},
);

// GET /api/v1/api/hosts/:id/info - Get detailed host information
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
			console.error("Error fetching host info:", error);
			res.status(500).json({ error: "Failed to fetch host information" });
		}
	},
);

// GET /api/v1/api/hosts/:id/network - Get host network information
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
			console.error("Error fetching host network info:", error);
			res.status(500).json({ error: "Failed to fetch host network information" });
		}
	},
);

// GET /api/v1/api/hosts/:id/system - Get host system information
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
			console.error("Error fetching host system info:", error);
			res.status(500).json({ error: "Failed to fetch host system information" });
		}
	},
);

// GET /api/v1/api/hosts/:id/package_reports - Get host package update reports
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
			console.error("Error fetching host package reports:", error);
			res.status(500).json({ error: "Failed to fetch host package reports" });
		}
	},
);

// GET /api/v1/api/hosts/:id/agent_queue - Get host agent queue status and jobs
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
				if (queueManager && queueManager.getQueue) {
					const agentQueue = queueManager.getQueue(host.api_id);
					if (agentQueue) {
						const counts = await agentQueue.getJobCounts();
						queueStats = {
							waiting: counts.waiting || 0,
							active: counts.active || 0,
							delayed: counts.delayed || 0,
							failed: counts.failed || 0,
						};
					}
				}
			} catch (queueError) {
				console.warn("Could not fetch live queue stats:", queueError.message);
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
			console.error("Error fetching host agent queue:", error);
			res.status(500).json({ error: "Failed to fetch host agent queue" });
		}
	},
);

// GET /api/v1/api/hosts/:id/notes - Get host notes
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
			console.error("Error fetching host notes:", error);
			res.status(500).json({ error: "Failed to fetch host notes" });
		}
	},
);

// GET /api/v1/api/hosts/:id/integrations - Get host integrations status
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
					description: "Monitor Docker containers, images, volumes, and networks. Collects real-time container status events.",
				};
			}

			res.json({
				host_id: id,
				integrations: {
					docker: dockerDetails || {
						enabled: false,
						description: "Monitor Docker containers, images, volumes, and networks. Collects real-time container status events.",
					},
				},
			});
		} catch (error) {
			console.error("Error fetching host integrations:", error);
			res.status(500).json({ error: "Failed to fetch host integrations" });
		}
	},
);

module.exports = router;
