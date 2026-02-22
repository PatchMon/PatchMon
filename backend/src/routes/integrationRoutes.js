const express = require("express");
const logger = require("../utils/logger");
const { getPrismaClient } = require("../config/prisma");
const { v4: uuidv4 } = require("uuid");
const { verifyApiKey } = require("../utils/apiKeyUtils");

const prisma = getPrismaClient();
const router = express.Router();

// POST /api/v1/integrations/docker - Docker data collection endpoint
router.post("/docker", async (req, res) => {
	try {
		const apiId = req.headers["x-api-id"];
		const apiKey = req.headers["x-api-key"];
		const {
			containers,
			images,
			volumes,
			networks,
			updates,
			daemon_info: _daemon_info,
			hostname,
			machine_id,
			agent_version: _agent_version,
		} = req.body;

		logger.info(
			`[Docker Integration] Received data from ${hostname || machine_id}`,
		);

		// Validate API credentials
		const host = await prisma.hosts.findFirst({
			where: { api_id: apiId },
		});

		if (!host) {
			logger.warn("[Docker Integration] Invalid API credentials");
			return res.status(401).json({ error: "Invalid API credentials" });
		}

		// Verify API key (supports bcrypt hashed and legacy plaintext keys)
		const isValidKey = await verifyApiKey(apiKey, host.api_key);
		if (!isValidKey) {
			logger.warn("[Docker Integration] Invalid API key");
			return res.status(401).json({ error: "Invalid API credentials" });
		}

		logger.info(
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
		let volumesProcessed = 0;
		let networksProcessed = 0;
		let updatesProcessed = 0;

		// Process containers
		if (containers && Array.isArray(containers)) {
			logger.info(
				`[Docker Integration] Processing ${containers.length} containers`,
			);

			// Batch fetch existing images and containers
			const imageKeys = containers
				.filter((c) => c.image_repository && c.image_tag)
				.map((c) => ({
					repository: c.image_repository,
					tag: c.image_tag,
					image_id: c.image_id || "unknown",
				}));

			const existingImages = await prisma.docker_images.findMany({
				where: {
					OR: imageKeys.map((k) => ({
						repository: k.repository,
						tag: k.tag,
						image_id: k.image_id,
					})),
				},
			});

			const existingImagesMap = new Map(
				existingImages.map((img) => [
					`${img.repository}|${img.tag}|${img.image_id}`,
					img,
				]),
			);

			const existingContainers = await prisma.docker_containers.findMany({
				where: {
					host_id: host.id,
					container_id: { in: containers.map((c) => c.container_id) },
				},
			});

			const existingContainersMap = new Map(
				existingContainers.map((c) => [c.container_id, c]),
			);

			// Separate images into create and update batches
			const imagesToCreate = [];
			const imagesToUpdate = [];
			const imageIdMap = new Map();

			for (const containerData of containers) {
				if (containerData.image_repository && containerData.image_tag) {
					const key = `${containerData.image_repository}|${containerData.image_tag}|${containerData.image_id || "unknown"}`;
					const existingImage = existingImagesMap.get(key);

					if (existingImage) {
						imagesToUpdate.push({
							id: existingImage.id,
							last_checked: now,
							updated_at: now,
						});
						imageIdMap.set(key, existingImage.id);
					} else if (!imageIdMap.has(key)) {
						const newImageId = uuidv4();
						imagesToCreate.push({
							id: newImageId,
							repository: containerData.image_repository,
							tag: containerData.image_tag,
							image_id: containerData.image_id || "unknown",
							source: containerData.image_source || "docker-hub",
							created_at: parseDate(containerData.created_at),
							updated_at: now,
						});
						imageIdMap.set(key, newImageId);
					}
				}
			}

			// Batch create new images
			if (imagesToCreate.length > 0) {
				await prisma.docker_images.createMany({
					data: imagesToCreate,
					skipDuplicates: true,
				});
			}

			// Batch update existing images
			for (const update of imagesToUpdate) {
				const { id, ...updateData } = update;
				await prisma.docker_images.update({
					where: { id },
					data: updateData,
				});
			}

			// Separate containers into create and update batches
			const containersToCreate = [];
			const containersToUpdate = [];

			for (const containerData of containers) {
				const imageKey =
					containerData.image_repository && containerData.image_tag
						? `${containerData.image_repository}|${containerData.image_tag}|${containerData.image_id || "unknown"}`
						: null;
				const imageId = imageKey ? imageIdMap.get(imageKey) : null;

				const existingContainer = existingContainersMap.get(
					containerData.container_id,
				);

				if (existingContainer) {
					containersToUpdate.push({
						id: existingContainer.id,
						name: containerData.name,
						image_id: imageId,
						image_name: containerData.image_name,
						image_tag: containerData.image_tag || "latest",
						status: containerData.status,
						state: containerData.state || containerData.status,
						ports: containerData.ports || null,
						labels: containerData.labels || null,
						started_at: containerData.started_at
							? parseDate(containerData.started_at)
							: null,
						updated_at: now,
						last_checked: now,
					});
				} else {
					containersToCreate.push({
						id: uuidv4(),
						host_id: host.id,
						container_id: containerData.container_id,
						name: containerData.name,
						image_id: imageId,
						image_name: containerData.image_name,
						image_tag: containerData.image_tag || "latest",
						status: containerData.status,
						state: containerData.state || containerData.status,
						ports: containerData.ports || null,
						labels: containerData.labels || null,
						created_at: parseDate(containerData.created_at),
						started_at: containerData.started_at
							? parseDate(containerData.started_at)
							: null,
						updated_at: now,
					});
				}
			}

			// Batch create new containers
			if (containersToCreate.length > 0) {
				await prisma.docker_containers.createMany({
					data: containersToCreate,
					skipDuplicates: true,
				});
			}

			// Batch update existing containers
			for (const update of containersToUpdate) {
				const { id, ...updateData } = update;
				await prisma.docker_containers.update({
					where: { id },
					data: updateData,
				});
			}

			containersProcessed = containers.length;
		} // Process standalone images
		if (images && Array.isArray(images)) {
			logger.info(`[Docker Integration] Processing ${images.length} images`);

			// Batch fetch existing standalone images
			const standaloneImageKeys = images.map((img) => ({
				repository: img.repository,
				tag: img.tag,
				image_id: img.image_id,
			}));

			const existingStandaloneImages = await prisma.docker_images.findMany({
				where: {
					OR: standaloneImageKeys.map((k) => ({
						repository: k.repository,
						tag: k.tag,
						image_id: k.image_id,
					})),
				},
			});

			const existingStandaloneImagesMap = new Map(
				existingStandaloneImages.map((img) => [
					`${img.repository}|${img.tag}|${img.image_id}`,
					img,
				]),
			);

			// Separate into create and update batches
			const standaloneImagesToCreate = [];
			const standaloneImagesToUpdate = [];

			for (const imageData of images) {
				const key = `${imageData.repository}|${imageData.tag}|${imageData.image_id}`;
				const existingImage = existingStandaloneImagesMap.get(key);

				if (existingImage) {
					standaloneImagesToUpdate.push({
						id: existingImage.id,
						size_bytes: imageData.size_bytes
							? BigInt(imageData.size_bytes)
							: null,
						digest: imageData.digest || null,
						last_checked: now,
						updated_at: now,
					});
				} else {
					standaloneImagesToCreate.push({
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
					});
				}
			}

			// Batch create new standalone images
			if (standaloneImagesToCreate.length > 0) {
				await prisma.docker_images.createMany({
					data: standaloneImagesToCreate,
					skipDuplicates: true,
				});
			}

			// Batch update existing standalone images
			for (const update of standaloneImagesToUpdate) {
				const { id, ...updateData } = update;
				await prisma.docker_images.update({
					where: { id },
					data: updateData,
				});
			}

			imagesProcessed = images.length;
		} // Process volumes
		if (volumes && Array.isArray(volumes)) {
			logger.info(`[Docker Integration] Processing ${volumes.length} volumes`);

			// Batch fetch existing volumes
			const existingVolumes = await prisma.docker_volumes.findMany({
				where: {
					host_id: host.id,
					volume_id: { in: volumes.map((v) => v.volume_id) },
				},
			});

			const existingVolumesMap = new Map(
				existingVolumes.map((v) => [v.volume_id, v]),
			);

			// Separate into create and update batches
			const volumesToCreate = [];
			const volumesToUpdate = [];

			for (const volumeData of volumes) {
				const existingVolume = existingVolumesMap.get(volumeData.volume_id);

				if (existingVolume) {
					volumesToUpdate.push({
						id: existingVolume.id,
						name: volumeData.name,
						driver: volumeData.driver || "local",
						mountpoint: volumeData.mountpoint || null,
						renderer: volumeData.renderer || null,
						scope: volumeData.scope || "local",
						labels: volumeData.labels || null,
						options: volumeData.options || null,
						size_bytes: volumeData.size_bytes
							? BigInt(volumeData.size_bytes)
							: null,
						ref_count: volumeData.ref_count || 0,
						updated_at: now,
						last_checked: now,
					});
				} else {
					volumesToCreate.push({
						id: uuidv4(),
						host_id: host.id,
						volume_id: volumeData.volume_id,
						name: volumeData.name,
						driver: volumeData.driver || "local",
						mountpoint: volumeData.mountpoint || null,
						renderer: volumeData.renderer || null,
						scope: volumeData.scope || "local",
						labels: volumeData.labels || null,
						options: volumeData.options || null,
						size_bytes: volumeData.size_bytes
							? BigInt(volumeData.size_bytes)
							: null,
						ref_count: volumeData.ref_count || 0,
						created_at: parseDate(volumeData.created_at),
						updated_at: now,
					});
				}
			}

			// Batch create new volumes
			if (volumesToCreate.length > 0) {
				await prisma.docker_volumes.createMany({
					data: volumesToCreate,
					skipDuplicates: true,
				});
			}

			// Batch update existing volumes
			for (const update of volumesToUpdate) {
				const { id, ...updateData } = update;
				await prisma.docker_volumes.update({
					where: { id },
					data: updateData,
				});
			}

			volumesProcessed = volumes.length;
		} // Process networks
		if (networks && Array.isArray(networks)) {
			logger.info(
				`[Docker Integration] Processing ${networks.length} networks`,
			);

			// Batch fetch existing networks
			const existingNetworks = await prisma.docker_networks.findMany({
				where: {
					host_id: host.id,
					network_id: { in: networks.map((n) => n.network_id) },
				},
			});

			const existingNetworksMap = new Map(
				existingNetworks.map((n) => [n.network_id, n]),
			);

			// Separate into create and update batches
			const networksToCreate = [];
			const networksToUpdate = [];

			for (const networkData of networks) {
				const existingNetwork = existingNetworksMap.get(networkData.network_id);

				if (existingNetwork) {
					networksToUpdate.push({
						id: existingNetwork.id,
						name: networkData.name,
						driver: networkData.driver,
						scope: networkData.scope || "local",
						ipv6_enabled: networkData.ipv6_enabled || false,
						internal: networkData.internal || false,
						attachable:
							networkData.attachable !== undefined
								? networkData.attachable
								: true,
						ingress: networkData.ingress || false,
						config_only: networkData.config_only || false,
						labels: networkData.labels || null,
						ipam: networkData.ipam || null,
						container_count: networkData.container_count || 0,
						updated_at: now,
						last_checked: now,
					});
				} else {
					networksToCreate.push({
						id: uuidv4(),
						host_id: host.id,
						network_id: networkData.network_id,
						name: networkData.name,
						driver: networkData.driver,
						scope: networkData.scope || "local",
						ipv6_enabled: networkData.ipv6_enabled || false,
						internal: networkData.internal || false,
						attachable:
							networkData.attachable !== undefined
								? networkData.attachable
								: true,
						ingress: networkData.ingress || false,
						config_only: networkData.config_only || false,
						labels: networkData.labels || null,
						ipam: networkData.ipam || null,
						container_count: networkData.container_count || 0,
						created_at: networkData.created_at
							? parseDate(networkData.created_at)
							: null,
						updated_at: now,
					});
				}
			}

			// Batch create new networks
			if (networksToCreate.length > 0) {
				await prisma.docker_networks.createMany({
					data: networksToCreate,
					skipDuplicates: true,
				});
			}

			// Batch update existing networks
			for (const update of networksToUpdate) {
				const { id, ...updateData } = update;
				await prisma.docker_networks.update({
					where: { id },
					data: updateData,
				});
			}

			networksProcessed = networks.length;
		} // Process updates
		if (updates && Array.isArray(updates)) {
			logger.info(`[Docker Integration] Processing ${updates.length} updates`);

			// Batch fetch images for all updates
			const updateImageKeys = updates.map((u) => ({
				repository: u.repository,
				tag: u.current_tag,
				image_id: u.image_id,
			}));

			const updatesImages = await prisma.docker_images.findMany({
				where: {
					OR: updateImageKeys.map((k) => ({
						repository: k.repository,
						tag: k.tag,
						image_id: k.image_id,
					})),
				},
			});

			const updatesImagesMap = new Map(
				updatesImages.map((img) => [
					`${img.repository}|${img.tag}|${img.image_id}`,
					img,
				]),
			);

			// Get existing update records
			const updateImageIds = updatesImages.map((img) => img.id);
			const existingUpdates = await prisma.docker_image_updates.findMany({
				where: {
					image_id: { in: updateImageIds },
				},
			});

			const existingUpdatesMap = new Map(
				existingUpdates.map((u) => [`${u.image_id}|${u.available_tag}`, u]),
			);

			// Separate into create and update batches
			const imageUpdatesToCreate = [];
			const imageUpdatesToUpdate = [];

			for (const updateData of updates) {
				const imageKey = `${updateData.repository}|${updateData.current_tag}|${updateData.image_id}`;
				const image = updatesImagesMap.get(imageKey);

				if (image) {
					// Store digest info in changelog_url field as JSON
					const digestInfo = JSON.stringify({
						method: "digest_comparison",
						current_digest: updateData.current_digest,
						available_digest: updateData.available_digest,
					});

					const updateKey = `${image.id}|${updateData.available_tag}`;
					const existingUpdate = existingUpdatesMap.get(updateKey);

					if (existingUpdate) {
						imageUpdatesToUpdate.push({
							id: existingUpdate.id,
							updated_at: now,
							changelog_url: digestInfo,
							severity: "digest_changed",
						});
					} else {
						imageUpdatesToCreate.push({
							id: uuidv4(),
							image_id: image.id,
							current_tag: updateData.current_tag,
							available_tag: updateData.available_tag,
							severity: "digest_changed",
							changelog_url: digestInfo,
							created_at: now,
							updated_at: now,
						});
					}
				}
			}

			// Batch create new image updates
			if (imageUpdatesToCreate.length > 0) {
				await prisma.docker_image_updates.createMany({
					data: imageUpdatesToCreate,
					skipDuplicates: true,
				});
			}

			// Batch update existing image updates
			for (const update of imageUpdatesToUpdate) {
				const { id, ...updateData } = update;
				await prisma.docker_image_updates.update({
					where: { id },
					data: updateData,
				});
			}

			updatesProcessed =
				imageUpdatesToCreate.length + imageUpdatesToUpdate.length;
		}
		logger.info(
			`[Docker Integration] Successfully processed: ${containersProcessed} containers, ${imagesProcessed} images, ${volumesProcessed} volumes, ${networksProcessed} networks, ${updatesProcessed} updates`,
		);

		res.json({
			message: "Docker data collected successfully",
			containers_received: containersProcessed,
			images_received: imagesProcessed,
			volumes_received: volumesProcessed,
			networks_received: networksProcessed,
			updates_found: updatesProcessed,
		});
	} catch (error) {
		logger.error("[Docker Integration] Error collecting Docker data:", error);
		logger.error("[Docker Integration] Error stack:", error.stack);
		res.status(500).json({
			error: "Failed to collect Docker data",
			message: error.message,
			details: process.env.NODE_ENV === "development" ? error.stack : undefined,
		});
	}
});

module.exports = router;
