const express = require("express");
const logger = require("../utils/logger");
const { body, validationResult } = require("express-validator");
const { getPrismaClient } = require("../config/prisma");
const { authenticateToken } = require("../middleware/auth");
const {
	requireViewHosts,
	requireManageHosts,
} = require("../middleware/permissions");

const router = express.Router();
const prisma = getPrismaClient();

// Get all repositories with host count
router.get("/", authenticateToken, requireViewHosts, async (_req, res) => {
	try {
		const repositories = await prisma.repositories.findMany({
			include: {
				host_repositories: {
					include: {
						hosts: {
							select: {
								id: true,
								friendly_name: true,
								status: true,
							},
						},
					},
				},
				_count: {
					select: {
						host_repositories: true,
					},
				},
			},
			orderBy: [{ name: "asc" }, { url: "asc" }],
		});

		// Transform data to include host counts and status
		const transformedRepos = repositories.map((repo) => ({
			...repo,
			hostCount: repo._count.host_repositories,
			enabledHostCount: repo.host_repositories.filter((hr) => hr.is_enabled)
				.length,
			activeHostCount: repo.host_repositories.filter(
				(hr) => hr.hosts.status === "active",
			).length,
			hosts: repo.host_repositories.map((hr) => ({
				id: hr.hosts.id,
				friendlyName: hr.hosts.friendly_name,
				status: hr.hosts.status,
				isEnabled: hr.is_enabled,
				lastChecked: hr.last_checked,
			})),
		}));

		res.json(transformedRepos);
	} catch (error) {
		logger.error("Repository list error:", error);
		res.status(500).json({ error: "Failed to fetch repositories" });
	}
});

// Get repositories for a specific host
router.get(
	"/host/:hostId",
	authenticateToken,
	requireViewHosts,
	async (req, res) => {
		try {
			const { hostId } = req.params;

			const hostRepositories = await prisma.host_repositories.findMany({
				where: { host_id: hostId },
				include: {
					repositories: true,
					hosts: {
						select: {
							id: true,
							friendly_name: true,
						},
					},
				},
				orderBy: {
					repositories: {
						name: "asc",
					},
				},
			});

			res.json(hostRepositories);
		} catch (error) {
			logger.error("Host repositories error:", error);
			res.status(500).json({ error: "Failed to fetch host repositories" });
		}
	},
);

// Get repository details with all hosts
router.get(
	"/:repositoryId",
	authenticateToken,
	requireViewHosts,
	async (req, res) => {
		try {
			const { repositoryId } = req.params;

			const repository = await prisma.repositories.findUnique({
				where: { id: repositoryId },
				include: {
					host_repositories: {
						include: {
							hosts: {
								select: {
									id: true,
									friendly_name: true,
									hostname: true,
									ip: true,
									os_type: true,
									os_version: true,
									status: true,
									last_update: true,
									needs_reboot: true,
								},
							},
						},
						orderBy: {
							hosts: {
								friendly_name: "asc",
							},
						},
					},
				},
			});

			if (!repository) {
				return res.status(404).json({ error: "Repository not found" });
			}

			res.json(repository);
		} catch (error) {
			logger.error("Repository detail error:", error);
			res.status(500).json({ error: "Failed to fetch repository details" });
		}
	},
);

// Update repository information (admin only)
router.put(
	"/:repositoryId",
	authenticateToken,
	requireManageHosts,
	[
		body("name")
			.optional()
			.isLength({ min: 1 })
			.withMessage("Name is required"),
		body("description").optional(),
		body("isActive")
			.optional()
			.isBoolean()
			.withMessage("isActive must be a boolean"),
		body("priority")
			.optional()
			.isInt({ min: 0 })
			.withMessage("Priority must be a positive integer"),
	],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const { repositoryId } = req.params;
			const { name, description, isActive, priority } = req.body;

			const repository = await prisma.repositories.update({
				where: { id: repositoryId },
				data: {
					...(name && { name }),
					...(description !== undefined && { description }),
					...(isActive !== undefined && { is_active: isActive }),
					...(priority !== undefined && { priority }),
				},
				include: {
					_count: {
						select: {
							host_repositories: true,
						},
					},
				},
			});

			res.json(repository);
		} catch (error) {
			logger.error("Repository update error:", error);
			res.status(500).json({ error: "Failed to update repository" });
		}
	},
);

// Toggle repository status for a specific host
router.patch(
	"/host/:hostId/repository/:repositoryId",
	authenticateToken,
	requireManageHosts,
	[body("isEnabled").isBoolean().withMessage("isEnabled must be a boolean")],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const { hostId, repositoryId } = req.params;
			const { isEnabled } = req.body;

			const hostRepository = await prisma.host_repositories.update({
				where: {
					host_id_repository_id: {
						host_id: hostId,
						repository_id: repositoryId,
					},
				},
				data: {
					is_enabled: isEnabled,
					last_checked: new Date(),
				},
				include: {
					repositories: true,
					hosts: {
						select: {
							friendly_name: true,
						},
					},
				},
			});

			res.json({
				message: `Repository ${isEnabled ? "enabled" : "disabled"} for host ${hostRepository.hosts.friendly_name}`,
				hostRepository,
			});
		} catch (error) {
			logger.error("Host repository toggle error:", error);
			res.status(500).json({ error: "Failed to toggle repository status" });
		}
	},
);

// Get repository statistics
router.get(
	"/stats/summary",
	authenticateToken,
	requireViewHosts,
	async (_req, res) => {
		try {
			const stats = await prisma.repositories.aggregate({
				_count: true,
			});

			const hostRepoStats = await prisma.host_repositories.aggregate({
				_count: {
					is_enabled: true,
				},
				where: {
					is_enabled: true,
				},
			});

			const secureRepos = await prisma.repositories.count({
				where: { is_secure: true },
			});

			const activeRepos = await prisma.repositories.count({
				where: { is_active: true },
			});

			res.json({
				totalRepositories: stats._count,
				activeRepositories: activeRepos,
				secureRepositories: secureRepos,
				enabledHostRepositories: hostRepoStats._count.isEnabled,
				securityPercentage:
					stats._count > 0 ? Math.round((secureRepos / stats._count) * 100) : 0,
			});
		} catch (error) {
			logger.error("Repository stats error:", error);
			res.status(500).json({ error: "Failed to fetch repository statistics" });
		}
	},
);

// Delete a specific repository (admin only)
router.delete(
	"/:repositoryId",
	authenticateToken,
	requireManageHosts,
	async (req, res) => {
		try {
			const { repositoryId } = req.params;

			// Check if repository exists first
			const existingRepository = await prisma.repositories.findUnique({
				where: { id: repositoryId },
				select: {
					id: true,
					name: true,
					url: true,
					_count: {
						select: {
							host_repositories: true,
						},
					},
				},
			});

			if (!existingRepository) {
				return res.status(404).json({
					error: "Repository not found",
					details: "The repository may have been deleted or does not exist",
				});
			}

			// Delete repository and all related data (cascade will handle host_repositories)
			await prisma.repositories.delete({
				where: { id: repositoryId },
			});

			res.json({
				message: "Repository deleted successfully",
				deletedRepository: {
					id: existingRepository.id,
					name: existingRepository.name,
					url: existingRepository.url,
					hostCount: existingRepository._count.host_repositories,
				},
			});
		} catch (error) {
			logger.error("Repository deletion error:", error);

			// Handle specific Prisma errors
			if (error.code === "P2025") {
				return res.status(404).json({
					error: "Repository not found",
					details: "The repository may have been deleted or does not exist",
				});
			}

			if (error.code === "P2003") {
				return res.status(400).json({
					error: "Cannot delete repository due to foreign key constraints",
					details: "The repository has related data that prevents deletion",
				});
			}

			res.status(500).json({
				error: "Failed to delete repository",
				details: error.message || "An unexpected error occurred",
			});
		}
	},
);

// Cleanup orphaned repositories (admin only)
router.delete(
	"/cleanup/orphaned",
	authenticateToken,
	requireManageHosts,
	async (_req, res) => {
		try {
			logger.info("Cleaning up orphaned repositories...");

			// Find repositories with no host relationships
			const orphanedRepos = await prisma.repositories.findMany({
				where: {
					host_repositories: {
						none: {},
					},
				},
			});

			if (orphanedRepos.length === 0) {
				return res.json({
					message: "No orphaned repositories found",
					deletedCount: 0,
					deletedRepositories: [],
				});
			}

			// Delete orphaned repositories
			const deleteResult = await prisma.repositories.deleteMany({
				where: {
					hostRepositories: {
						none: {},
					},
				},
			});

			logger.info(`Deleted ${deleteResult.count} orphaned repositories`);

			res.json({
				message: `Successfully deleted ${deleteResult.count} orphaned repositories`,
				deletedCount: deleteResult.count,
				deletedRepositories: orphanedRepos.map((repo) => ({
					id: repo.id,
					name: repo.name,
					url: repo.url,
				})),
			});
		} catch (error) {
			logger.error("Repository cleanup error:", error);
			res
				.status(500)
				.json({ error: "Failed to cleanup orphaned repositories" });
		}
	},
);

module.exports = router;
