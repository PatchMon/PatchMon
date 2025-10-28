const express = require("express");
const { authenticateToken } = require("../middleware/auth");
const { getPrismaClient } = require("../config/prisma");
const { v4: uuidv4 } = require("uuid");

const prisma = getPrismaClient();
const router = express.Router();

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

		const skip = (parseInt(page, 10) - 1) * parseInt(limit, 10);
		const take = parseInt(limit, 10);

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

		const where = {};
		if (source) where.source = source;
		if (search) {
			where.OR = [
				{ repository: { contains: search, mode: "insensitive" } },
				{ tag: { contains: search, mode: "insensitive" } },
			];
		}

		const skip = (parseInt(page, 10) - 1) * parseInt(limit, 10);
		const take = parseInt(limit, 10);

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

		// Get hosts that have Docker containers
		const hostsWithContainers = await prisma.docker_containers.groupBy({
			by: ["host_id"],
			_count: true,
		});

		const hostIds = hostsWithContainers.map((h) => h.host_id);

		const skip = (parseInt(page, 10) - 1) * parseInt(limit, 10);
		const take = parseInt(limit, 10);

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

		const where = {};
		if (securityOnly === "true") {
			where.is_security_update = true;
		}

		const skip = (parseInt(page, 10) - 1) * parseInt(limit, 10);
		const take = parseInt(limit, 10);

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
			where: { api_id: apiId, api_key: apiKey },
		});

		if (!host) {
			return res.status(401).json({ error: "Invalid API credentials" });
		}

		const now = new Date();

		// Helper function to validate and parse dates
		const parseDate = (dateString) => {
			if (!dateString) return now;
			const date = new Date(dateString);
			return Number.isNaN(date.getTime()) ? now : date;
		};

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
							created_at: parseDate(containerData.created_at),
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
							? parseDate(containerData.started_at)
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
						created_at: parseDate(containerData.created_at),
						started_at: containerData.started_at
							? parseDate(containerData.started_at)
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
						created_at: parseDate(imageData.created_at),
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
		console.error("Request body:", JSON.stringify(req.body, null, 2));
		res.status(500).json({
			error: "Failed to collect Docker data",
			message: error.message,
			details: process.env.NODE_ENV === "development" ? error.stack : undefined,
		});
	}
});

// POST /api/v1/integrations/docker - New integration endpoint for Docker data collection
router.post("/../integrations/docker", async (req, res) => {
	try {
		const apiId = req.headers["x-api-id"];
		const apiKey = req.headers["x-api-key"];
		const {
			containers,
			images,
			updates,
			daemon_info: _daemon_info,
			hostname,
			machine_id,
			agent_version: _agent_version,
		} = req.body;

		console.log(
			`[Docker Integration] Received data from ${hostname || machine_id}`,
		);

		// Validate API credentials
		const host = await prisma.hosts.findFirst({
			where: { api_id: apiId, api_key: apiKey },
		});

		if (!host) {
			console.warn("[Docker Integration] Invalid API credentials");
			return res.status(401).json({ error: "Invalid API credentials" });
		}

		console.log(
			`[Docker Integration] Processing for host: ${host.friendly_name}`,
		);

		const now = new Date();

		// Helper function to validate and parse dates
		const parseDate = (dateString) => {
			if (!dateString) return now;
			const date = new Date(dateString);
			return Number.isNaN(date.getTime()) ? now : date;
		};

		let containersProcessed = 0;
		let imagesProcessed = 0;
		let updatesProcessed = 0;

		// Process containers
		if (containers && Array.isArray(containers)) {
			console.log(
				`[Docker Integration] Processing ${containers.length} containers`,
			);
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
							created_at: parseDate(containerData.created_at),
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
						state: containerData.state || containerData.status,
						ports: containerData.ports || null,
						started_at: containerData.started_at
							? parseDate(containerData.started_at)
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
						state: containerData.state || containerData.status,
						ports: containerData.ports || null,
						created_at: parseDate(containerData.created_at),
						started_at: containerData.started_at
							? parseDate(containerData.started_at)
							: null,
						updated_at: now,
					},
				});
				containersProcessed++;
			}
		}

		// Process standalone images
		if (images && Array.isArray(images)) {
			console.log(`[Docker Integration] Processing ${images.length} images`);
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
						digest: imageData.digest || null,
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
						created_at: parseDate(imageData.created_at),
						updated_at: now,
					},
				});
				imagesProcessed++;
			}
		}

		// Process updates
		if (updates && Array.isArray(updates)) {
			console.log(`[Docker Integration] Processing ${updates.length} updates`);
			for (const updateData of updates) {
				// Find the image by repository and image_id
				const image = await prisma.docker_images.findFirst({
					where: {
						repository: updateData.repository,
						tag: updateData.current_tag,
						image_id: updateData.image_id,
					},
				});

				if (image) {
					// Store digest info in changelog_url field as JSON
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
					updatesProcessed++;
				}
			}
		}

		console.log(
			`[Docker Integration] Successfully processed: ${containersProcessed} containers, ${imagesProcessed} images, ${updatesProcessed} updates`,
		);

		res.json({
			message: "Docker data collected successfully",
			containers_received: containersProcessed,
			images_received: imagesProcessed,
			updates_found: updatesProcessed,
		});
	} catch (error) {
		console.error("[Docker Integration] Error collecting Docker data:", error);
		console.error("[Docker Integration] Error stack:", error.stack);
		res.status(500).json({
			error: "Failed to collect Docker data",
			message: error.message,
			details: process.env.NODE_ENV === "development" ? error.stack : undefined,
		});
	}
});

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

module.exports = router;
