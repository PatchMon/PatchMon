const express = require("express");
const logger = require("../utils/logger");
const { body, validationResult } = require("express-validator");
const { getPrismaClient } = require("../config/prisma");
const { randomUUID } = require("node:crypto");
const { authenticateToken } = require("../middleware/auth");
const { requireManageHosts } = require("../middleware/permissions");

const router = express.Router();
const prisma = getPrismaClient();

// Get all host groups
router.get("/", authenticateToken, async (_req, res) => {
	try {
		const hostGroups = await prisma.host_groups.findMany({
			include: {
				_count: {
					select: {
						host_group_memberships: true,
					},
				},
			},
			orderBy: {
				name: "asc",
			},
		});

		// Transform the count field to match frontend expectations
		const transformedGroups = hostGroups.map((group) => ({
			...group,
			_count: {
				hosts: group._count.host_group_memberships,
			},
		}));

		res.json(transformedGroups);
	} catch (error) {
		logger.error("Error fetching host groups:", error);
		res.status(500).json({ error: "Failed to fetch host groups" });
	}
});

// Get a specific host group by ID
router.get("/:id", authenticateToken, async (req, res) => {
	try {
		const { id } = req.params;

		const hostGroup = await prisma.host_groups.findUnique({
			where: { id },
			include: {
				host_group_memberships: {
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
				},
			},
		});

		if (!hostGroup) {
			return res.status(404).json({ error: "Host group not found" });
		}

		res.json(hostGroup);
	} catch (error) {
		logger.error("Error fetching host group:", error);
		res.status(500).json({ error: "Failed to fetch host group" });
	}
});

// Create a new host group
router.post(
	"/",
	authenticateToken,
	requireManageHosts,
	[
		body("name").trim().isLength({ min: 1 }).withMessage("Name is required"),
		body("description").optional().trim(),
		body("color")
			.optional()
			.isHexColor()
			.withMessage("Color must be a valid hex color"),
	],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const { name, description, color } = req.body;

			// Check if host group with this name already exists
			const existingGroup = await prisma.host_groups.findUnique({
				where: { name },
			});

			if (existingGroup) {
				return res
					.status(400)
					.json({ error: "A host group with this name already exists" });
			}

			const hostGroup = await prisma.host_groups.create({
				data: {
					id: randomUUID(),
					name,
					description: description || null,
					color: color || "#3B82F6",
					updated_at: new Date(),
				},
			});

			res.status(201).json(hostGroup);
		} catch (error) {
			logger.error("Error creating host group:", error);
			res.status(500).json({ error: "Failed to create host group" });
		}
	},
);

// Update a host group
router.put(
	"/:id",
	authenticateToken,
	requireManageHosts,
	[
		body("name").trim().isLength({ min: 1 }).withMessage("Name is required"),
		body("description").optional().trim(),
		body("color")
			.optional()
			.isHexColor()
			.withMessage("Color must be a valid hex color"),
	],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const { id } = req.params;
			const { name, description, color } = req.body;

			// Check if host group exists
			const existingGroup = await prisma.host_groups.findUnique({
				where: { id },
			});

			if (!existingGroup) {
				return res.status(404).json({ error: "Host group not found" });
			}

			// Check if another host group with this name already exists
			const duplicateGroup = await prisma.host_groups.findFirst({
				where: {
					name,
					id: { not: id },
				},
			});

			if (duplicateGroup) {
				return res
					.status(400)
					.json({ error: "A host group with this name already exists" });
			}

			const hostGroup = await prisma.host_groups.update({
				where: { id },
				data: {
					name,
					description: description || null,
					color: color || "#3B82F6",
					updated_at: new Date(),
				},
			});

			res.json(hostGroup);
		} catch (error) {
			logger.error("Error updating host group:", error);
			res.status(500).json({ error: "Failed to update host group" });
		}
	},
);

// Delete a host group
router.delete(
	"/:id",
	authenticateToken,
	requireManageHosts,
	async (req, res) => {
		try {
			const { id } = req.params;

			// Check if host group exists
			const existingGroup = await prisma.host_groups.findUnique({
				where: { id },
				include: {
					_count: {
						select: {
							host_group_memberships: true,
						},
					},
				},
			});

			if (!existingGroup) {
				return res.status(404).json({ error: "Host group not found" });
			}

			// If host group has memberships, remove them first
			if (existingGroup._count.host_group_memberships > 0) {
				await prisma.host_group_memberships.deleteMany({
					where: { host_group_id: id },
				});
			}

			await prisma.host_groups.delete({
				where: { id },
			});

			res.json({ message: "Host group deleted successfully" });
		} catch (error) {
			logger.error("Error deleting host group:", error);
			res.status(500).json({ error: "Failed to delete host group" });
		}
	},
);

// Get hosts in a specific group
router.get("/:id/hosts", authenticateToken, async (req, res) => {
	try {
		const { id } = req.params;

		const hosts = await prisma.hosts.findMany({
			where: {
				host_group_memberships: {
					some: {
						host_group_id: id,
					},
				},
			},
			select: {
				id: true,
				friendly_name: true,
				ip: true,
				os_type: true,
				os_version: true,
				architecture: true,
				status: true,
				last_update: true,
				created_at: true,
				needs_reboot: true,
			},
			orderBy: {
				friendly_name: "asc",
			},
		});

		res.json(hosts);
	} catch (error) {
		logger.error("Error fetching hosts in group:", error);
		res.status(500).json({ error: "Failed to fetch hosts in group" });
	}
});

module.exports = router;
