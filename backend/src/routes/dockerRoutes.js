const express = require("express");
const { authenticateToken } = require("../middleware/auth");
const { getPrismaClient } = require("../config/prisma");
const { v4: uuidv4 } = require("uuid");
const { get_current_time, parse_date } = require("../utils/timezone");
const { verifyApiKey } = require("../utils/apiKeyUtils");

const prisma = getPrismaClient();
const router = express.Router();

/**
 * Sanitize request body for logging - removes sensitive fields
 * @param {Object} body - The request body to sanitize
 * @returns {Object} Sanitized copy of the body
 */
function sanitizeBodyForLogging(body) {
	if (!body || typeof body !== "object") {
		return body;
	}

	const sensitiveFields = [
		"apiKey",
		"api_key",
		"apiId",
		"api_id",
		"password",
		"token",
		"secret",
		"credential",
		"privateKey",
		"private_key",
		"passphrase",
	];

	const sanitized = { ...body };

	for (const key of Object.keys(sanitized)) {
		const lowerKey = key.toLowerCase();
		if (sensitiveFields.some((field) => lowerKey.includes(field.toLowerCase()))) {
			sanitized[key] = "[REDACTED]";
		} else if (typeof sanitized[key] === "object" && sanitized[key] !== null) {
			sanitized[key] = sanitizeBodyForLogging(sanitized[key]);
		}
	}

	return sanitized;
}

/**
 * Validate and sanitize pagination parameters
 * @param {string|number} page - Page number
 * @param {string|number} limit - Items per page
 * @returns {{page: number, limit: number, skip: number, take: number}}
 */
function validatePagination(page, limit) {
	const parsedPage = Math.max(1, parseInt(page, 10) || 1);
	const parsedLimit = Math.min(100, Math.max(1, parseInt(limit, 10) || 50));
	return {
		page: parsedPage,
		limit: parsedLimit,
		skip: (parsedPage - 1) * parsedLimit,
		take: parsedLimit,
	};
}

// Helper function to convert BigInt fields to strings for JSON serialization
const convertBigIntToString = (obj) => {
	if (obj === null || obj === undefined) return obj;

	if (typeof obj === "bigint") {
		return obj.toString();
	}

	if (Array.isArray(obj)) {
		return obj.map(convertBigIntToString);
	}

	if (typeof obj === "object") {
		const converted = {};
		for (const key in obj) {
			converted[key] = convertBigIntToString(obj[key]);
		}
		return converted;
	}

	return obj;
};

// GET /api/v1/docker/dashboard - Get Docker dashboard statistics
router.get("/dashboard", authenticateToken, async (_req, res) => {
	try {
		// Get total hosts with Docker containers
		const hostsWithDocker = await prisma.docker_containers.groupBy({
			by: ["host_id"],
			_count: true,
		});

		// Get total containers
		const totalContainers = await prisma.docker_containers.count();

		// Get running containers
		const runningContainers = await prisma.docker_containers.count({
			where: { status: "running" },
		});

		// Get total images
		const totalImages = await prisma.docker_images.count();

		// Get available updates
		const availableUpdates = await prisma.docker_image_updates.count();

		// Get containers by status
		const containersByStatus = await prisma.docker_containers.groupBy({
			by: ["status"],
			_count: true,
		});

		// Get images by source
		const imagesBySource = await prisma.docker_images.groupBy({
			by: ["source"],
			_count: true,
		});

		res.json({
			stats: {
				totalHostsWithDocker: hostsWithDocker.length,
				totalContainers,
				runningContainers,
				totalImages,
				availableUpdates,
			},
			containersByStatus,
			imagesBySource,
		});
	} catch (error) {
		console.error("Error fetching Docker dashboard:", error);
		res.status(500).json({ error: "Failed to fetch Docker dashboard" });
	}
});

// GET /api/v1/docker/containers - Get all containers with filters
router.get("/containers", authenticateToken, async (req, res) => {
	try {
		const { status, hostId, imageId, search, page = 1, limit = 50 } = req.query;
		const pagination = validatePagination(page, limit);

		const where = {};
		if (status) where.status = status;
		if (hostId) where.host_id = hostId;
		if (imageId) where.image_id = imageId;
		if (search) {
			where.OR = [
				{ name: { contains: search, mode: "insensitive" } },
				{ image_name: { contains: search, mode: "insensitive" } },
			];
		}

		const { skip, take } = pagination;

		const [containers, total] = await Promise.all([
			prisma.docker_containers.findMany({
				where,
				include: {
					docker_images: true,
				},
				orderBy: { updated_at: "desc" },
				skip,
				take,
			}),
			prisma.docker_containers.count({ where }),
		]);

		// Get host information for each container
		const hostIds = [...new Set(containers.map((c) => c.host_id))];
		const hosts = await prisma.hosts.findMany({
			where: { id: { in: hostIds } },
			select: { id: true, friendly_name: true, hostname: true, ip: true },
		});

		const hostsMap = hosts.reduce((acc, host) => {
			acc[host.id] = host;
			return acc;
		}, {});

		const containersWithHosts = containers.map((container) => ({
			...container,
			host: hostsMap[container.host_id],
		}));

		res.json(
			convertBigIntToString({
				containers: containersWithHosts,
				pagination: {
					page: parseInt(page, 10),
					limit: parseInt(limit, 10),
					total,
					totalPages: Math.ceil(total / parseInt(limit, 10)),
				},
			}),
		);
	} catch (error) {
		console.error("Error fetching containers:", error);
		res.status(500).json({ error: "Failed to fetch containers" });
	}
});

// GET /api/v1/docker/containers/:id - Get container detail
router.get("/containers/:id", authenticateToken, async (req, res) => {
	try {
		const { id } = req.params;

		const container = await prisma.docker_containers.findUnique({
			where: { id },
			include: {
				docker_images: {
					include: {
						docker_image_updates: true,
					},
				},
			},
		});

		if (!container) {
			return res.status(404).json({ error: "Container not found" });
		}

		// Get host information
		const host = await prisma.hosts.findUnique({
			where: { id: container.host_id },
			select: {
				id: true,
				friendly_name: true,
				hostname: true,
				ip: true,
				os_type: true,
				os_version: true,
			},
		});

		// Get other containers using the same image
		const similarContainers = await prisma.docker_containers.findMany({
			where: {
				image_id: container.image_id,
				id: { not: id },
			},
			take: 10,
		});

		res.json(
			convertBigIntToString({
				container: {
					...container,
					host,
				},
				similarContainers,
			}),
		);
	} catch (error) {
		console.error("Error fetching container detail:", error);
		res.status(500).json({ error: "Failed to fetch container detail" });
	}
});

// GET /api/v1/docker/images - Get all images with filters
router.get("/images", authenticateToken, async (req, res) => {
	try {
		const { source, search, page = 1, limit = 50 } = req.query;
		const pagination = validatePagination(page, limit);

		const where = {};
		if (source) where.source = source;
		if (search) {
			where.OR = [
				{ repository: { contains: search, mode: "insensitive" } },
				{ tag: { contains: search, mode: "insensitive" } },
			];
		}

		const { skip, take } = pagination;

		const [images, total] = await Promise.all([
			prisma.docker_images.findMany({
				where,
				include: {
					_count: {
						select: {
							docker_containers: true,
							docker_image_updates: true,
						},
					},
					docker_image_updates: {
						take: 1,
						orderBy: { created_at: "desc" },
					},
				},
				orderBy: { updated_at: "desc" },
				skip,
				take,
			}),
			prisma.docker_images.count({ where }),
		]);

		// Get unique hosts using each image
		const imagesWithHosts = await Promise.all(
			images.map(async (image) => {
				const containers = await prisma.docker_containers.findMany({
					where: { image_id: image.id },
					select: { host_id: true },
					distinct: ["host_id"],
				});
				return {
					...image,
					hostsCount: containers.length,
					hasUpdates: image._count.docker_image_updates > 0,
				};
			}),
		);

		res.json(
			convertBigIntToString({
				images: imagesWithHosts,
				pagination: {
					page: parseInt(page, 10),
					limit: parseInt(limit, 10),
					total,
					totalPages: Math.ceil(total / parseInt(limit, 10)),
				},
			}),
		);
	} catch (error) {
		console.error("Error fetching images:", error);
		res.status(500).json({ error: "Failed to fetch images" });
	}
});

// GET /api/v1/docker/images/:id - Get image detail
router.get("/images/:id", authenticateToken, async (req, res) => {
	try {
		const { id } = req.params;

		const image = await prisma.docker_images.findUnique({
			where: { id },
			include: {
				docker_containers: {
					take: 100,
				},
				docker_image_updates: {
					orderBy: { created_at: "desc" },
				},
			},
		});

		if (!image) {
			return res.status(404).json({ error: "Image not found" });
		}

		// Get unique hosts using this image
		const hostIds = [...new Set(image.docker_containers.map((c) => c.host_id))];
		const hosts = await prisma.hosts.findMany({
			where: { id: { in: hostIds } },
			select: { id: true, friendly_name: true, hostname: true, ip: true },
		});

		res.json(
			convertBigIntToString({
				image,
				hosts,
				totalContainers: image.docker_containers.length,
				totalHosts: hosts.length,
			}),
		);
	} catch (error) {
		console.error("Error fetching image detail:", error);
		res.status(500).json({ error: "Failed to fetch image detail" });
	}
});

// GET /api/v1/docker/hosts - Get all hosts with Docker
router.get("/hosts", authenticateToken, async (req, res) => {
	try {
		const { page = 1, limit = 50 } = req.query;
		const pagination = validatePagination(page, limit);

		// Get hosts that have Docker containers
		const hostsWithContainers = await prisma.docker_containers.groupBy({
			by: ["host_id"],
			_count: true,
		});

		const hostIds = hostsWithContainers.map((h) => h.host_id);

		const { skip, take } = pagination;

		const hosts = await prisma.hosts.findMany({
			where: { id: { in: hostIds } },
			skip,
			take,
			orderBy: { friendly_name: "asc" },
		});

		// Get container counts and statuses for each host
		const hostsWithStats = await Promise.all(
			hosts.map(async (host) => {
				const [totalContainers, runningContainers, totalImages] =
					await Promise.all([
						prisma.docker_containers.count({
							where: { host_id: host.id },
						}),
						prisma.docker_containers.count({
							where: { host_id: host.id, status: "running" },
						}),
						prisma.docker_containers.findMany({
							where: { host_id: host.id },
							select: { image_id: true },
							distinct: ["image_id"],
						}),
					]);

				return {
					...host,
					dockerStats: {
						totalContainers,
						runningContainers,
						totalImages: totalImages.length,
					},
				};
			}),
		);

		res.json(
			convertBigIntToString({
				hosts: hostsWithStats,
				pagination: {
					page: parseInt(page, 10),
					limit: parseInt(limit, 10),
					total: hostIds.length,
					totalPages: Math.ceil(hostIds.length / parseInt(limit, 10)),
				},
			}),
		);
	} catch (error) {
		console.error("Error fetching Docker hosts:", error);
		res.status(500).json({ error: "Failed to fetch Docker hosts" });
	}
});

// GET /api/v1/docker/hosts/:id - Get host Docker detail
router.get("/hosts/:id", authenticateToken, async (req, res) => {
	try {
		const { id } = req.params;

		const host = await prisma.hosts.findUnique({
			where: { id },
		});

		if (!host) {
			return res.status(404).json({ error: "Host not found" });
		}

		// Get containers on this host
		const containers = await prisma.docker_containers.findMany({
			where: { host_id: id },
			include: {
				docker_images: {
					include: {
						docker_image_updates: true,
					},
				},
			},
			orderBy: { name: "asc" },
		});

		// Get unique images on this host
		const imageIds = [...new Set(containers.map((c) => c.image_id))].filter(
			Boolean,
		);
		const images = await prisma.docker_images.findMany({
			where: { id: { in: imageIds } },
		});

		// Get container statistics
		const runningContainers = containers.filter(
			(c) => c.status === "running",
		).length;
		const stoppedContainers = containers.filter(
			(c) => c.status === "exited" || c.status === "stopped",
		).length;

		res.json(
			convertBigIntToString({
				host,
				containers,
				images,
				stats: {
					totalContainers: containers.length,
					runningContainers,
					stoppedContainers,
					totalImages: images.length,
				},
			}),
		);
	} catch (error) {
		console.error("Error fetching host Docker detail:", error);
		res.status(500).json({ error: "Failed to fetch host Docker detail" });
	}
});

// GET /api/v1/docker/updates - Get available updates
router.get("/updates", authenticateToken, async (req, res) => {
	try {
		const { page = 1, limit = 50, securityOnly = false } = req.query;
		const pagination = validatePagination(page, limit);

		const where = {};
		if (securityOnly === "true") {
			where.is_security_update = true;
		}

		const { skip, take } = pagination;

		const [updates, total] = await Promise.all([
			prisma.docker_image_updates.findMany({
				where,
				include: {
					docker_images: {
						include: {
							docker_containers: {
								select: {
									id: true,
									host_id: true,
									name: true,
								},
							},
						},
					},
				},
				orderBy: [{ is_security_update: "desc" }, { created_at: "desc" }],
				skip,
				take,
			}),
			prisma.docker_image_updates.count({ where }),
		]);

		// Get affected hosts for each update
		const updatesWithHosts = await Promise.all(
			updates.map(async (update) => {
				const hostIds = [
					...new Set(
						update.docker_images.docker_containers.map((c) => c.host_id),
					),
				];
				const hosts = await prisma.hosts.findMany({
					where: { id: { in: hostIds } },
					select: { id: true, friendly_name: true, hostname: true },
				});
				return {
					...update,
					affectedHosts: hosts,
					affectedContainersCount:
						update.docker_images.docker_containers.length,
				};
			}),
		);

		res.json(
			convertBigIntToString({
				updates: updatesWithHosts,
				pagination: {
					page: parseInt(page, 10),
					limit: parseInt(limit, 10),
					total,
					totalPages: Math.ceil(total / parseInt(limit, 10)),
				},
			}),
		);
	} catch (error) {
		console.error("Error fetching Docker updates:", error);
		res.status(500).json({ error: "Failed to fetch Docker updates" });
	}
});

// POST /api/v1/docker/collect - Collect Docker data from agent (DEPRECATED - kept for backward compatibility)
// New agents should use POST /api/v1/integrations/docker
router.post("/collect", async (req, res) => {
	try {
		const { apiId, apiKey, containers, images, updates } = req.body;

		// Validate API credentials
		const host = await prisma.hosts.findFirst({
			where: { api_id: apiId },
		});

		if (!host) {
			return res.status(401).json({ error: "Invalid API credentials" });
		}

		// Verify API key (supports bcrypt hashed and legacy plaintext keys)
		const isValidKey = await verifyApiKey(apiKey, host.api_key);
		if (!isValidKey) {
			return res.status(401).json({ error: "Invalid API credentials" });
		}

		const now = get_current_time();

		// Process containers
		if (containers && Array.isArray(containers)) {
			for (const containerData of containers) {
				const containerId = uuidv4();

				// Find or create image
				let imageId = null;
				if (containerData.image_repository && containerData.image_tag) {
					const image = await prisma.docker_images.upsert({
						where: {
							repository_tag_image_id: {
								repository: containerData.image_repository,
								tag: containerData.image_tag,
								image_id: containerData.image_id || "unknown",
							},
						},
						update: {
							last_checked: now,
							updated_at: now,
						},
						create: {
							id: uuidv4(),
							repository: containerData.image_repository,
							tag: containerData.image_tag,
							image_id: containerData.image_id || "unknown",
							source: containerData.image_source || "docker-hub",
							created_at: parse_date(containerData.created_at, now),
							last_checked: now,
							updated_at: now,
						},
					});
					imageId = image.id;
				}

				// Upsert container
				await prisma.docker_containers.upsert({
					where: {
						host_id_container_id: {
							host_id: host.id,
							container_id: containerData.container_id,
						},
					},
					update: {
						name: containerData.name,
						image_id: imageId,
						image_name: containerData.image_name,
						image_tag: containerData.image_tag || "latest",
						status: containerData.status,
						state: containerData.state,
						ports: containerData.ports || null,
						started_at: containerData.started_at
							? parse_date(containerData.started_at, null)
							: null,
						updated_at: now,
						last_checked: now,
					},
					create: {
						id: containerId,
						host_id: host.id,
						container_id: containerData.container_id,
						name: containerData.name,
						image_id: imageId,
						image_name: containerData.image_name,
						image_tag: containerData.image_tag || "latest",
						status: containerData.status,
						state: containerData.state,
						ports: containerData.ports || null,
						created_at: parse_date(containerData.created_at, now),
						started_at: containerData.started_at
							? parse_date(containerData.started_at, null)
							: null,
						updated_at: now,
					},
				});
			}
		}

		// Process standalone images
		if (images && Array.isArray(images)) {
			for (const imageData of images) {
				await prisma.docker_images.upsert({
					where: {
						repository_tag_image_id: {
							repository: imageData.repository,
							tag: imageData.tag,
							image_id: imageData.image_id,
						},
					},
					update: {
						size_bytes: imageData.size_bytes
							? BigInt(imageData.size_bytes)
							: null,
						last_checked: now,
						updated_at: now,
					},
					create: {
						id: uuidv4(),
						repository: imageData.repository,
						tag: imageData.tag,
						image_id: imageData.image_id,
						digest: imageData.digest,
						size_bytes: imageData.size_bytes
							? BigInt(imageData.size_bytes)
							: null,
						source: imageData.source || "docker-hub",
						created_at: parse_date(imageData.created_at, now),
						updated_at: now,
					},
				});
			}
		}

		// Process updates
		// First, get all images for this host to clean up old updates
		const hostImageIds = await prisma.docker_containers
			.findMany({
				where: { host_id: host.id },
				select: { image_id: true },
				distinct: ["image_id"],
			})
			.then((results) => results.map((r) => r.image_id).filter(Boolean));

		// Delete old updates for images on this host that are no longer reported
		if (hostImageIds.length > 0) {
			const reportedImageIds = [];

			// Process new updates
			if (updates && Array.isArray(updates)) {
				for (const updateData of updates) {
					// Find the image by repository, tag, and image_id
					const image = await prisma.docker_images.findFirst({
						where: {
							repository: updateData.repository,
							tag: updateData.current_tag,
							image_id: updateData.image_id,
						},
					});

					if (image) {
						reportedImageIds.push(image.id);

						// Store digest info in changelog_url field as JSON for now
						const digestInfo = JSON.stringify({
							method: "digest_comparison",
							current_digest: updateData.current_digest,
							available_digest: updateData.available_digest,
						});

						// Upsert the update record
						await prisma.docker_image_updates.upsert({
							where: {
								image_id_available_tag: {
									image_id: image.id,
									available_tag: updateData.available_tag,
								},
							},
							update: {
								updated_at: now,
								changelog_url: digestInfo,
								severity: "digest_changed",
							},
							create: {
								id: uuidv4(),
								image_id: image.id,
								current_tag: updateData.current_tag,
								available_tag: updateData.available_tag,
								severity: "digest_changed",
								changelog_url: digestInfo,
								updated_at: now,
							},
						});
					}
				}
			}

			// Remove stale updates for images on this host that are no longer in the updates list
			const imageIdsToCleanup = hostImageIds.filter(
				(id) => !reportedImageIds.includes(id),
			);
			if (imageIdsToCleanup.length > 0) {
				await prisma.docker_image_updates.deleteMany({
					where: {
						image_id: { in: imageIdsToCleanup },
					},
				});
			}
		}

		res.json({ success: true, message: "Docker data collected successfully" });
	} catch (error) {
		console.error("Error collecting Docker data:", error);
		console.error("Error stack:", error.stack);
		// Sanitize request body before logging to prevent credential exposure
		console.error("Request body (sanitized):", JSON.stringify(sanitizeBodyForLogging(req.body), null, 2));
		res.status(500).json({
			error: "Failed to collect Docker data",
			message: error.message,
			details: process.env.NODE_ENV === "development" ? error.stack : undefined,
		});
	}
});

// NOTE: POST /api/v1/integrations/docker is defined in integrationRoutes.js
// The duplicate route that was here has been removed (it had an invalid path)

// DELETE /api/v1/docker/containers/:id - Delete a container
router.delete("/containers/:id", authenticateToken, async (req, res) => {
	try {
		const { id } = req.params;

		// Check if container exists
		const container = await prisma.docker_containers.findUnique({
			where: { id },
		});

		if (!container) {
			return res.status(404).json({ error: "Container not found" });
		}

		// Delete the container
		await prisma.docker_containers.delete({
			where: { id },
		});

		console.log(`🗑️  Deleted container: ${container.name} (${id})`);

		res.json({
			success: true,
			message: `Container ${container.name} deleted successfully`,
		});
	} catch (error) {
		console.error("Error deleting container:", error);
		res.status(500).json({ error: "Failed to delete container" });
	}
});

// DELETE /api/v1/docker/images/:id - Delete an image
router.delete("/images/:id", authenticateToken, async (req, res) => {
	try {
		const { id } = req.params;

		// Check if image exists
		const image = await prisma.docker_images.findUnique({
			where: { id },
			include: {
				_count: {
					select: {
						docker_containers: true,
					},
				},
			},
		});

		if (!image) {
			return res.status(404).json({ error: "Image not found" });
		}

		// Check if image is in use by containers
		if (image._count.docker_containers > 0) {
			return res.status(400).json({
				error: `Cannot delete image: ${image._count.docker_containers} container(s) are using this image`,
				containersCount: image._count.docker_containers,
			});
		}

		// Delete image updates first
		await prisma.docker_image_updates.deleteMany({
			where: { image_id: id },
		});

		// Delete the image
		await prisma.docker_images.delete({
			where: { id },
		});

		console.log(`🗑️  Deleted image: ${image.repository}:${image.tag} (${id})`);

		res.json({
			success: true,
			message: `Image ${image.repository}:${image.tag} deleted successfully`,
		});
	} catch (error) {
		console.error("Error deleting image:", error);
		res.status(500).json({ error: "Failed to delete image" });
	}
});

// GET /api/v1/docker/volumes - Get all volumes with filters
router.get("/volumes", authenticateToken, async (req, res) => {
	try {
		const { driver, search, page = 1, limit = 50 } = req.query;
		const pagination = validatePagination(page, limit);

		const where = {};
		if (driver) where.driver = driver;
		if (search) {
			where.OR = [{ name: { contains: search, mode: "insensitive" } }];
		}

		const { skip, take } = pagination;

		const [volumes, total] = await Promise.all([
			prisma.docker_volumes.findMany({
				where,
				include: {
					hosts: {
						select: {
							id: true,
							friendly_name: true,
							hostname: true,
							ip: true,
						},
					},
				},
				orderBy: { updated_at: "desc" },
				skip,
				take,
			}),
			prisma.docker_volumes.count({ where }),
		]);

		res.json(
			convertBigIntToString({
				volumes,
				pagination: {
					page: parseInt(page, 10),
					limit: parseInt(limit, 10),
					total,
					totalPages: Math.ceil(total / parseInt(limit, 10)),
				},
			}),
		);
	} catch (error) {
		console.error("Error fetching volumes:", error);
		res.status(500).json({ error: "Failed to fetch volumes" });
	}
});

// GET /api/v1/docker/volumes/:id - Get volume detail
router.get("/volumes/:id", authenticateToken, async (req, res) => {
	try {
		const { id } = req.params;

		const volume = await prisma.docker_volumes.findUnique({
			where: { id },
			include: {
				hosts: {
					select: {
						id: true,
						friendly_name: true,
						hostname: true,
						ip: true,
						os_type: true,
						os_version: true,
					},
				},
			},
		});

		if (!volume) {
			return res.status(404).json({ error: "Volume not found" });
		}

		res.json(convertBigIntToString({ volume }));
	} catch (error) {
		console.error("Error fetching volume detail:", error);
		res.status(500).json({ error: "Failed to fetch volume detail" });
	}
});

// GET /api/v1/docker/networks - Get all networks with filters
router.get("/networks", authenticateToken, async (req, res) => {
	try {
		const { driver, search, page = 1, limit = 50 } = req.query;
		const pagination = validatePagination(page, limit);

		const where = {};
		if (driver) where.driver = driver;
		if (search) {
			where.OR = [{ name: { contains: search, mode: "insensitive" } }];
		}

		const { skip, take } = pagination;

		const [networks, total] = await Promise.all([
			prisma.docker_networks.findMany({
				where,
				include: {
					hosts: {
						select: {
							id: true,
							friendly_name: true,
							hostname: true,
							ip: true,
						},
					},
				},
				orderBy: { updated_at: "desc" },
				skip,
				take,
			}),
			prisma.docker_networks.count({ where }),
		]);

		res.json(
			convertBigIntToString({
				networks,
				pagination: {
					page: parseInt(page, 10),
					limit: parseInt(limit, 10),
					total,
					totalPages: Math.ceil(total / parseInt(limit, 10)),
				},
			}),
		);
	} catch (error) {
		console.error("Error fetching networks:", error);
		res.status(500).json({ error: "Failed to fetch networks" });
	}
});

// GET /api/v1/docker/networks/:id - Get network detail
router.get("/networks/:id", authenticateToken, async (req, res) => {
	try {
		const { id } = req.params;

		const network = await prisma.docker_networks.findUnique({
			where: { id },
			include: {
				hosts: {
					select: {
						id: true,
						friendly_name: true,
						hostname: true,
						ip: true,
						os_type: true,
						os_version: true,
					},
				},
			},
		});

		if (!network) {
			return res.status(404).json({ error: "Network not found" });
		}

		res.json(convertBigIntToString({ network }));
	} catch (error) {
		console.error("Error fetching network detail:", error);
		res.status(500).json({ error: "Failed to fetch network detail" });
	}
});

// GET /api/v1/docker/agent - Serve the Docker agent installation script
router.get("/agent", async (_req, res) => {
	try {
		const fs = require("node:fs");
		const path = require("node:path");
		const agentPath = path.join(
			__dirname,
			"../../..",
			"agents",
			"patchmon-docker-agent.sh",
		);

		// Check if file exists
		if (!fs.existsSync(agentPath)) {
			return res.status(404).json({ error: "Docker agent script not found" });
		}

		// Read and serve the file
		const agentScript = fs.readFileSync(agentPath, "utf8");
		res.setHeader("Content-Type", "text/x-shellscript");
		res.setHeader(
			"Content-Disposition",
			'inline; filename="patchmon-docker-agent.sh"',
		);
		res.send(agentScript);
	} catch (error) {
		console.error("Error serving Docker agent:", error);
		res.status(500).json({ error: "Failed to serve Docker agent script" });
	}
});

// DELETE /api/v1/docker/volumes/:id - Delete a volume
router.delete("/volumes/:id", authenticateToken, async (req, res) => {
	try {
		const { id } = req.params;

		// Check if volume exists
		const volume = await prisma.docker_volumes.findUnique({
			where: { id },
		});

		if (!volume) {
			return res.status(404).json({ error: "Volume not found" });
		}

		// Delete the volume
		await prisma.docker_volumes.delete({
			where: { id },
		});

		console.log(`🗑️  Deleted volume: ${volume.name} (${id})`);

		res.json({
			success: true,
			message: `Volume ${volume.name} deleted successfully`,
		});
	} catch (error) {
		console.error("Error deleting volume:", error);
		res.status(500).json({ error: "Failed to delete volume" });
	}
});

// DELETE /api/v1/docker/networks/:id - Delete a network
router.delete("/networks/:id", authenticateToken, async (req, res) => {
	try {
		const { id } = req.params;

		// Check if network exists
		const network = await prisma.docker_networks.findUnique({
			where: { id },
		});

		if (!network) {
			return res.status(404).json({ error: "Network not found" });
		}

		// Delete the network
		await prisma.docker_networks.delete({
			where: { id },
		});

		console.log(`🗑️  Deleted network: ${network.name} (${id})`);

		res.json({
			success: true,
			message: `Network ${network.name} deleted successfully`,
		});
	} catch (error) {
		console.error("Error deleting network:", error);
		res.status(500).json({ error: "Failed to delete network" });
	}
});

module.exports = router;
