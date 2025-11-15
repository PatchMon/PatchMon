const express = require("express");
const { getPrismaClient } = require("../config/prisma");
const { authenticateApiToken } = require("../middleware/apiAuth");

const router = express.Router();
const prisma = getPrismaClient();

// Get homepage widget statistics
router.get("/stats", authenticateApiToken("gethomepage"), async (_req, res) => {
	try {
		// Get total hosts count
		const totalHosts = await prisma.hosts.count({
			where: { status: "active" },
		});

		// Get total unique packages that need updates (consistent with dashboard)
		const totalOutdatedPackages = await prisma.packages.count({
			where: {
				host_packages: {
					some: {
						needs_update: true,
					},
				},
			},
		});

		// Get total repositories count
		const totalRepos = await prisma.repositories.count({
			where: { is_active: true },
		});

		// Get hosts that need updates (have outdated packages)
		const hostsNeedingUpdates = await prisma.hosts.count({
			where: {
				status: "active",
				host_packages: {
					some: {
						needs_update: true,
					},
				},
			},
		});

		// Get security updates count (unique packages - consistent with dashboard)
		const securityUpdates = await prisma.packages.count({
			where: {
				host_packages: {
					some: {
						needs_update: true,
						is_security_update: true,
					},
				},
			},
		});

		// Get hosts with security updates
		const hostsWithSecurityUpdates = await prisma.hosts.count({
			where: {
				status: "active",
				host_packages: {
					some: {
						needs_update: true,
						is_security_update: true,
					},
				},
			},
		});

		// Get up-to-date hosts count
		const upToDateHosts = totalHosts - hostsNeedingUpdates;

		// Get recent update activity (last 24 hours)
		const oneDayAgo = new Date(Date.now() - 24 * 60 * 60 * 1000);
		const recentUpdates = await prisma.update_history.count({
			where: {
				timestamp: {
					gte: oneDayAgo,
				},
				status: "success",
			},
		});

		// Get OS distribution
		const osDistribution = await prisma.hosts.groupBy({
			by: ["os_type"],
			where: { status: "active" },
			_count: {
				id: true,
			},
			orderBy: {
				_count: {
					id: "desc",
				},
			},
		});

		// Format OS distribution data
		const osDistributionFormatted = osDistribution.map((os) => ({
			name: os.os_type,
			count: os._count.id,
		}));

		// Extract top 3 OS types for flat display in widgets
		const top_os_1 = osDistributionFormatted[0] || { name: "None", count: 0 };
		const top_os_2 = osDistributionFormatted[1] || { name: "None", count: 0 };
		const top_os_3 = osDistributionFormatted[2] || { name: "None", count: 0 };

		// Prepare response data
		const stats = {
			total_hosts: totalHosts,
			total_outdated_packages: totalOutdatedPackages,
			total_repos: totalRepos,
			hosts_needing_updates: hostsNeedingUpdates,
			up_to_date_hosts: upToDateHosts,
			security_updates: securityUpdates,
			hosts_with_security_updates: hostsWithSecurityUpdates,
			recent_updates_24h: recentUpdates,
			os_distribution: osDistributionFormatted,
			// Flattened OS data for easy widget display
			top_os_1_name: top_os_1.name,
			top_os_1_count: top_os_1.count,
			top_os_2_name: top_os_2.name,
			top_os_2_count: top_os_2.count,
			top_os_3_name: top_os_3.name,
			top_os_3_count: top_os_3.count,
			last_updated: new Date().toISOString(),
		};

		res.json(stats);
	} catch (error) {
		console.error("Error fetching homepage stats:", error);
		res.status(500).json({ error: "Failed to fetch statistics" });
	}
});

// Health check endpoint for the API
router.get("/health", authenticateApiToken("gethomepage"), async (req, res) => {
	res.json({
		status: "ok",
		timestamp: new Date().toISOString(),
		api_key: req.apiToken.token_name,
	});
});

module.exports = router;
