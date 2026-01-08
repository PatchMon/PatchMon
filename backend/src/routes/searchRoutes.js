const express = require("express");
const logger = require("../utils/logger");
const router = express.Router();
const { getPrismaClient } = require("../config/prisma");
const { authenticateToken } = require("../middleware/auth");

const prisma = getPrismaClient();

/**
 * Global search endpoint
 * Searches across hosts, packages, repositories, and users
 * Returns categorized results
 */
router.get("/", authenticateToken, async (req, res) => {
	try {
		const { q } = req.query;

		if (!q || q.trim().length === 0) {
			return res.json({
				hosts: [],
				packages: [],
				repositories: [],
				users: [],
			});
		}

		const searchTerm = q.trim();

		// Prepare results object
		const results = {
			hosts: [],
			packages: [],
			repositories: [],
			users: [],
		};

		// Get user permissions from database
		let userPermissions = null;
		try {
			userPermissions = await prisma.role_permissions.findUnique({
				where: { role: req.user.role },
			});

			// If no specific permissions found, default to admin permissions
			if (!userPermissions) {
				logger.warn(
					`No permissions found for role: ${req.user.role}, defaulting to admin access`,
				);
				userPermissions = {
					can_view_hosts: true,
					can_view_packages: true,
					can_view_users: true,
				};
			}
		} catch (permError) {
			logger.error("Error fetching permissions:", permError);
			// Default to restrictive permissions on error
			userPermissions = {
				can_view_hosts: false,
				can_view_packages: false,
				can_view_users: false,
			};
		}

		// Search hosts if user has permission
		if (userPermissions.can_view_hosts) {
			try {
				const hosts = await prisma.hosts.findMany({
					where: {
						OR: [
							{ hostname: { contains: searchTerm, mode: "insensitive" } },
							{ friendly_name: { contains: searchTerm, mode: "insensitive" } },
							{ ip: { contains: searchTerm, mode: "insensitive" } },
							{ machine_id: { contains: searchTerm, mode: "insensitive" } },
						],
					},
					select: {
						id: true,
						machine_id: true,
						hostname: true,
						friendly_name: true,
						ip: true,
						os_type: true,
						os_version: true,
						status: true,
						last_update: true,
					},
					take: 10, // Limit results
					orderBy: {
						last_update: "desc",
					},
				});

				results.hosts = hosts.map((host) => ({
					id: host.id,
					hostname: host.hostname,
					friendly_name: host.friendly_name,
					ip: host.ip,
					os_type: host.os_type,
					os_version: host.os_version,
					status: host.status,
					last_update: host.last_update,
					type: "host",
				}));
			} catch (error) {
				logger.error("Error searching hosts:", error);
			}
		}

		// Search packages if user has permission
		if (userPermissions.can_view_packages) {
			try {
				const packages = await prisma.packages.findMany({
					where: {
						name: { contains: searchTerm, mode: "insensitive" },
					},
					select: {
						id: true,
						name: true,
						description: true,
						category: true,
						latest_version: true,
						_count: {
							select: {
								host_packages: true,
							},
						},
					},
					take: 10,
					orderBy: {
						name: "asc",
					},
				});

				results.packages = packages.map((pkg) => ({
					id: pkg.id,
					name: pkg.name,
					description: pkg.description,
					category: pkg.category,
					latest_version: pkg.latest_version,
					host_count: pkg._count.host_packages,
					type: "package",
				}));
			} catch (error) {
				logger.error("Error searching packages:", error);
			}
		}

		// Search repositories if user has permission (usually same as hosts)
		if (userPermissions.can_view_hosts) {
			try {
				const repositories = await prisma.repositories.findMany({
					where: {
						OR: [
							{ name: { contains: searchTerm, mode: "insensitive" } },
							{ url: { contains: searchTerm, mode: "insensitive" } },
							{ description: { contains: searchTerm, mode: "insensitive" } },
						],
					},
					select: {
						id: true,
						name: true,
						url: true,
						distribution: true,
						repo_type: true,
						is_active: true,
						description: true,
						_count: {
							select: {
								host_repositories: true,
							},
						},
					},
					take: 10,
					orderBy: {
						name: "asc",
					},
				});

				results.repositories = repositories.map((repo) => ({
					id: repo.id,
					name: repo.name,
					url: repo.url,
					distribution: repo.distribution,
					repo_type: repo.repo_type,
					is_active: repo.is_active,
					description: repo.description,
					host_count: repo._count.host_repositories,
					type: "repository",
				}));
			} catch (error) {
				logger.error("Error searching repositories:", error);
			}
		}

		// Search users if user has permission
		if (userPermissions.can_view_users) {
			try {
				const users = await prisma.users.findMany({
					where: {
						OR: [
							{ username: { contains: searchTerm, mode: "insensitive" } },
							{ email: { contains: searchTerm, mode: "insensitive" } },
							{ first_name: { contains: searchTerm, mode: "insensitive" } },
							{ last_name: { contains: searchTerm, mode: "insensitive" } },
						],
					},
					select: {
						id: true,
						username: true,
						email: true,
						first_name: true,
						last_name: true,
						role: true,
						is_active: true,
						last_login: true,
					},
					take: 10,
					orderBy: {
						username: "asc",
					},
				});

				results.users = users.map((user) => ({
					id: user.id,
					username: user.username,
					email: user.email,
					first_name: user.first_name,
					last_name: user.last_name,
					role: user.role,
					is_active: user.is_active,
					last_login: user.last_login,
					type: "user",
				}));
			} catch (error) {
				logger.error("Error searching users:", error);
			}
		}

		res.json(results);
	} catch (error) {
		logger.error("Global search error:", error);
		res.status(500).json({
			error: "Failed to perform search",
			message: error.message,
		});
	}
});

module.exports = router;
