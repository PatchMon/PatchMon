const { prisma } = require("./shared/prisma");
const logger = require("../../utils/logger");

/**
 * Docker Inventory Cleanup Automation
 * Removes Docker containers and images for hosts that no longer exist
 */
class DockerInventoryCleanup {
	constructor(queueManager) {
		this.queueManager = queueManager;
		this.queueName = "docker-inventory-cleanup";
	}

	/**
	 * Process Docker inventory cleanup job
	 */
	async process(_job) {
		const startTime = Date.now();
		logger.info("🧹 Starting Docker inventory cleanup...");

		try {
			// Step 1: Find and delete orphaned containers (containers for non-existent hosts)
			const orphanedContainers = await prisma.docker_containers.findMany({
				where: {
					host_id: {
						// Find containers where the host doesn't exist
						notIn: await prisma.hosts
							.findMany({ select: { id: true } })
							.then((hosts) => hosts.map((h) => h.id)),
					},
				},
			});

			let deletedContainersCount = 0;
			const deletedContainers = [];

			for (const container of orphanedContainers) {
				try {
					await prisma.docker_containers.delete({
						where: { id: container.id },
					});
					deletedContainersCount++;
					deletedContainers.push({
						id: container.id,
						container_id: container.container_id,
						name: container.name,
						image_name: container.image_name,
						host_id: container.host_id,
					});
					logger.info(
						`🗑️ Deleted orphaned container: ${container.name} (host_id: ${container.host_id})`,
					);
				} catch (deleteError) {
					logger.error(
						`❌ Failed to delete container ${container.id}:`,
						deleteError.message,
					);
				}
			}

			// Step 2: Find and delete orphaned images (images with no containers using them)
			const orphanedImages = await prisma.docker_images.findMany({
				where: {
					docker_containers: {
						none: {},
					},
				},
				include: {
					_count: {
						select: {
							docker_containers: true,
							docker_image_updates: true,
						},
					},
				},
			});

			let deletedImagesCount = 0;
			const deletedImages = [];

			for (const image of orphanedImages) {
				try {
					// First delete any image updates associated with this image
					if (image._count.docker_image_updates > 0) {
						await prisma.docker_image_updates.deleteMany({
							where: { image_id: image.id },
						});
					}

					// Then delete the image itself
					await prisma.docker_images.delete({
						where: { id: image.id },
					});
					deletedImagesCount++;
					deletedImages.push({
						id: image.id,
						repository: image.repository,
						tag: image.tag,
						image_id: image.image_id,
					});
					logger.info(
						`🗑️ Deleted orphaned image: ${image.repository}:${image.tag}`,
					);
				} catch (deleteError) {
					logger.error(
						`❌ Failed to delete image ${image.id}:`,
						deleteError.message,
					);
				}
			}

			const executionTime = Date.now() - startTime;
			logger.info(
				`✅ Docker inventory cleanup completed in ${executionTime}ms - Deleted ${deletedContainersCount} containers and ${deletedImagesCount} images`,
			);

			return {
				success: true,
				deletedContainersCount,
				deletedImagesCount,
				deletedContainers,
				deletedImages,
				executionTime,
			};
		} catch (error) {
			const executionTime = Date.now() - startTime;
			logger.error(
				`❌ Docker inventory cleanup failed after ${executionTime}ms:`,
				error.message,
			);
			throw error;
		}
	}

	/**
	 * Schedule recurring Docker inventory cleanup (daily at 4 AM)
	 */
	async schedule() {
		const job = await this.queueManager.queues[this.queueName].add(
			"docker-inventory-cleanup",
			{},
			{
				repeat: { cron: "0 4 * * *" }, // Daily at 4 AM
				jobId: "docker-inventory-cleanup-recurring",
			},
		);
		logger.info("✅ Docker inventory cleanup scheduled");
		return job;
	}

	/**
	 * Trigger manual Docker inventory cleanup
	 */
	async triggerManual() {
		const job = await this.queueManager.queues[this.queueName].add(
			"docker-inventory-cleanup-manual",
			{},
			{ priority: 1 },
		);
		logger.info("✅ Manual Docker inventory cleanup triggered");
		return job;
	}
}

module.exports = DockerInventoryCleanup;
