const express = require("express");
const logger = require("../utils/logger");
const { getPrismaClient } = require("../config/prisma");

const router = express.Router();
const prisma = getPrismaClient();

// Get all packages with their update status
router.get("/", async (req, res) => {
	try {
		const {
			page = 1,
			limit = 50,
			search = "",
			category = "",
			needsUpdate = "",
			isSecurityUpdate = "",
			host = "",
		} = req.query;

		const skip = (parseInt(page, 10) - 1) * parseInt(limit, 10);
		const take = parseInt(limit, 10);

		// Build where clause
		const where = {
			AND: [
				// Search filter
				search
					? {
							OR: [
								{ name: { contains: search, mode: "insensitive" } },
								{ description: { contains: search, mode: "insensitive" } },
							],
						}
					: {},
				// Category filter
				category ? { category: { equals: category } } : {},
				// Host filter - only return packages installed on the specified host
				// Combined with update status filters if both are present
				host
					? {
							host_packages: {
								some: {
									host_id: host,
									// If needsUpdate or isSecurityUpdate filters are present, apply them here
									...(needsUpdate
										? { needs_update: needsUpdate === "true" }
										: {}),
									...(isSecurityUpdate
										? { is_security_update: isSecurityUpdate === "true" }
										: {}),
								},
							},
						}
					: {},
				// Update status filters (only applied if no host filter)
				// If host filter is present, these are already applied above
				!host && needsUpdate
					? {
							host_packages: {
								some: {
									needs_update: needsUpdate === "true",
								},
							},
						}
					: {},
				!host && isSecurityUpdate
					? {
							host_packages: {
								some: {
									is_security_update: isSecurityUpdate === "true",
								},
							},
						}
					: {},
			],
		};

		// Get packages with counts
		const [packages, totalCount] = await Promise.all([
			prisma.packages.findMany({
				where,
				select: {
					id: true,
					name: true,
					description: true,
					category: true,
					latest_version: true,
					created_at: true,
					_count: {
						select: {
							host_packages: true,
						},
					},
				},
				skip,
				take,
				orderBy: {
					name: "asc",
				},
			}),
			prisma.packages.count({ where }),
		]);

		// OPTIMIZATION: Batch query all stats instead of N individual queries
		const packageIds = packages.map((pkg) => pkg.id);

		// Get all counts and host data in 3 batch queries instead of N*3 queries
		const [allUpdatesCounts, allSecurityCounts, allPackageHostsData] =
			await Promise.all([
				// Batch count all packages that need updates
				prisma.host_packages.groupBy({
					by: ["package_id"],
					where: {
						package_id: { in: packageIds },
						needs_update: true,
						...(host ? { host_id: host } : {}),
					},
					_count: { id: true },
				}),
				// Batch count all packages with security updates
				prisma.host_packages.groupBy({
					by: ["package_id"],
					where: {
						package_id: { in: packageIds },
						needs_update: true,
						is_security_update: true,
						...(host ? { host_id: host } : {}),
					},
					_count: { id: true },
				}),
				// Batch fetch all host data for packages
				prisma.host_packages.findMany({
					where: {
						package_id: { in: packageIds },
						...(host ? { host_id: host } : { needs_update: true }),
					},
					select: {
						package_id: true,
						hosts: {
							select: {
								id: true,
								friendly_name: true,
								hostname: true,
								os_type: true,
								needs_reboot: true,
							},
						},
						current_version: true,
						available_version: true,
						needs_update: true,
						is_security_update: true,
					},
					// Limit to first 10 per package
					take: 100, // Increased from package-based limit
				}),
			]);

		// Create lookup maps for O(1) access
		const updatesCountMap = new Map(
			allUpdatesCounts.map((item) => [item.package_id, item._count.id]),
		);
		const securityCountMap = new Map(
			allSecurityCounts.map((item) => [item.package_id, item._count.id]),
		);
		const packageHostsMap = new Map();

		// Group host data by package_id
		for (const hp of allPackageHostsData) {
			if (!packageHostsMap.has(hp.package_id)) {
				packageHostsMap.set(hp.package_id, []);
			}
			const hosts = packageHostsMap.get(hp.package_id);
			hosts.push({
				hostId: hp.hosts.id,
				friendlyName: hp.hosts.friendly_name,
				osType: hp.hosts.os_type,
				currentVersion: hp.current_version,
				availableVersion: hp.available_version,
				needsUpdate: hp.needs_update,
				isSecurityUpdate: hp.is_security_update,
			});

			// Limit to 10 hosts per package
			if (hosts.length > 10) {
				packageHostsMap.set(hp.package_id, hosts.slice(0, 10));
			}
		}

		// Map packages with stats from lookup maps (no more DB queries!)
		const packagesWithStats = packages.map((pkg) => {
			const updatesCount = updatesCountMap.get(pkg.id) || 0;
			const securityCount = securityCountMap.get(pkg.id) || 0;
			const packageHosts = packageHostsMap.get(pkg.id) || [];

			return {
				...pkg,
				packageHostsCount: pkg._count.host_packages,
				packageHosts,
				stats: {
					totalInstalls: pkg._count.host_packages,
					updatesNeeded: updatesCount,
					securityUpdates: securityCount,
				},
			};
		});

		res.json({
			packages: packagesWithStats,
			pagination: {
				page: parseInt(page, 10),
				limit: parseInt(limit, 10),
				total: totalCount,
				pages: Math.ceil(totalCount / parseInt(limit, 10)),
			},
		});
	} catch (error) {
		logger.error("Error fetching packages:", error);
		res.status(500).json({ error: "Failed to fetch packages" });
	}
});

// Get package details by ID
router.get("/:packageId", async (req, res) => {
	try {
		const { packageId } = req.params;

		const packageData = await prisma.packages.findUnique({
			where: { id: packageId },
			include: {
				host_packages: {
					include: {
						hosts: {
							select: {
								id: true,
								hostname: true,
								ip: true,
								os_type: true,
								os_version: true,
								last_update: true,
								needs_reboot: true,
							},
						},
					},
					orderBy: {
						needs_update: "desc",
					},
				},
			},
		});

		if (!packageData) {
			return res.status(404).json({ error: "Package not found" });
		}

		// Calculate statistics
		const stats = {
			totalInstalls: packageData.host_packages.length,
			updatesNeeded: packageData.host_packages.filter((hp) => hp.needs_update)
				.length,
			securityUpdates: packageData.host_packages.filter(
				(hp) => hp.needs_update && hp.is_security_update,
			).length,
			upToDate: packageData.host_packages.filter((hp) => !hp.needs_update)
				.length,
		};

		// Group by version
		const versionDistribution = packageData.host_packages.reduce((acc, hp) => {
			const version = hp.current_version;
			acc[version] = (acc[version] || 0) + 1;
			return acc;
		}, {});

		// Group by OS type
		const osDistribution = packageData.host_packages.reduce((acc, hp) => {
			const osType = hp.hosts.os_type;
			acc[osType] = (acc[osType] || 0) + 1;
			return acc;
		}, {});

		res.json({
			...packageData,
			stats,
			distributions: {
				versions: Object.entries(versionDistribution).map(
					([version, count]) => ({
						version,
						count,
					}),
				),
				osTypes: Object.entries(osDistribution).map(([osType, count]) => ({
					osType,
					count,
				})),
			},
		});
	} catch (error) {
		logger.error("Error fetching package details:", error);
		res.status(500).json({ error: "Failed to fetch package details" });
	}
});

// Get hosts where a package is installed
router.get("/:packageId/hosts", async (req, res) => {
	try {
		const { packageId } = req.params;
		const {
			page = 1,
			limit = 25,
			search = "",
			sortBy = "friendly_name",
			sortOrder = "asc",
		} = req.query;

		const offset = (parseInt(page, 10) - 1) * parseInt(limit, 10);

		// Build search conditions
		const searchConditions = search
			? {
					OR: [
						{
							hosts: {
								friendly_name: { contains: search, mode: "insensitive" },
							},
						},
						{ hosts: { hostname: { contains: search, mode: "insensitive" } } },
						{ current_version: { contains: search, mode: "insensitive" } },
						{ available_version: { contains: search, mode: "insensitive" } },
					],
				}
			: {};

		// Build sort conditions
		const orderBy = {};
		if (
			sortBy === "friendly_name" ||
			sortBy === "hostname" ||
			sortBy === "os_type"
		) {
			orderBy.hosts = { [sortBy]: sortOrder };
		} else if (sortBy === "needs_update") {
			orderBy[sortBy] = sortOrder;
		} else {
			orderBy[sortBy] = sortOrder;
		}

		// Get total count
		const totalCount = await prisma.host_packages.count({
			where: {
				package_id: packageId,
				...searchConditions,
			},
		});

		// Get paginated results
		const hostPackages = await prisma.host_packages.findMany({
			where: {
				package_id: packageId,
				...searchConditions,
			},
			include: {
				hosts: {
					select: {
						id: true,
						friendly_name: true,
						hostname: true,
						os_type: true,
						os_version: true,
						last_update: true,
						needs_reboot: true,
					},
				},
			},
			orderBy,
			skip: offset,
			take: parseInt(limit, 10),
		});

		// Transform the data for the frontend
		const hosts = hostPackages.map((hp) => ({
			hostId: hp.hosts.id,
			friendlyName: hp.hosts.friendly_name,
			hostname: hp.hosts.hostname,
			osType: hp.hosts.os_type,
			osVersion: hp.hosts.os_version,
			lastUpdate: hp.hosts.last_update,
			currentVersion: hp.current_version,
			availableVersion: hp.available_version,
			needsUpdate: hp.needs_update,
			isSecurityUpdate: hp.is_security_update,
			lastChecked: hp.last_checked,
			needsReboot: hp.hosts.needs_reboot,
		}));

		res.json({
			hosts,
			pagination: {
				page: parseInt(page, 10),
				limit: parseInt(limit, 10),
				total: totalCount,
				pages: Math.ceil(totalCount / parseInt(limit, 10)),
			},
		});
	} catch (error) {
		logger.error("Error fetching package hosts:", error);
		res.status(500).json({ error: "Failed to fetch package hosts" });
	}
});

module.exports = router;
