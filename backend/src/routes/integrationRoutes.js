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
						labels: containerData.labels || null,
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
						labels: containerData.labels || null,
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
			logger.info(`[Docker Integration] Processing ${images.length} images`);
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

		// Process volumes
		if (volumes && Array.isArray(volumes)) {
			logger.info(`[Docker Integration] Processing ${volumes.length} volumes`);
			for (const volumeData of volumes) {
				await prisma.docker_volumes.upsert({
					where: {
						host_id_volume_id: {
							host_id: host.id,
							volume_id: volumeData.volume_id,
						},
					},
					update: {
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
					},
					create: {
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
					},
				});
				volumesProcessed++;
			}
		}

		// Process networks
		if (networks && Array.isArray(networks)) {
			logger.info(
				`[Docker Integration] Processing ${networks.length} networks`,
			);
			for (const networkData of networks) {
				await prisma.docker_networks.upsert({
					where: {
						host_id_network_id: {
							host_id: host.id,
							network_id: networkData.network_id,
						},
					},
					update: {
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
					},
					create: {
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
					},
				});
				networksProcessed++;
			}
		}

		// Process updates
		if (updates && Array.isArray(updates)) {
			logger.info(`[Docker Integration] Processing ${updates.length} updates`);
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
