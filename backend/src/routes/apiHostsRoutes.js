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

module.exports = router;
