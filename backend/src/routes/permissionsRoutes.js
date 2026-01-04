const express = require("express");
const { getPrismaClient } = require("../config/prisma");
const { authenticateToken } = require("../middleware/auth");
const {
	requireManageSettings,
	requireManageUsers,
} = require("../middleware/permissions");

const router = express.Router();
const prisma = getPrismaClient();

// Get all role permissions (allow users who can manage users to view roles)
router.get(
	"/roles",
	authenticateToken,
	requireManageUsers,
	async (_req, res) => {
		try {
			const permissions = await prisma.role_permissions.findMany();

			// Sort roles: superadmin first, then admin, then user, then others alphabetically
			const roleOrder = { superadmin: 0, admin: 1, user: 2 };
			permissions.sort((a, b) => {
				const aOrder = roleOrder[a.role] ?? 999;
				const bOrder = roleOrder[b.role] ?? 999;
				if (aOrder !== bOrder) return aOrder - bOrder;
				return a.role.localeCompare(b.role);
			});

			res.json(permissions);
		} catch (error) {
			console.error("Get role permissions error:", error);
			res.status(500).json({ error: "Failed to fetch role permissions" });
		}
	},
);

// Get permissions for a specific role
router.get(
	"/roles/:role",
	authenticateToken,
	requireManageSettings,
	async (req, res) => {
		try {
			const { role } = req.params;

			const permissions = await prisma.role_permissions.findUnique({
				where: { role },
			});

			if (!permissions) {
				return res.status(404).json({ error: "Role not found" });
			}

			res.json(permissions);
		} catch (error) {
			console.error("Get role permission error:", error);
			res.status(500).json({ error: "Failed to fetch role permission" });
		}
	},
);

// Create or update role permissions
router.put(
	"/roles/:role",
	authenticateToken,
	requireManageSettings,
	async (req, res) => {
		try {
			const { role } = req.params;
			const {
				can_view_dashboard,
				can_view_hosts,
				can_manage_hosts,
				can_view_packages,
				can_manage_packages,
				can_view_users,
				can_manage_users,
				can_manage_superusers,
				can_view_reports,
				can_export_data,
				can_manage_settings,
			} = req.body;

			// Prevent modifying built-in role permissions (superadmin, admin, user)
			if (role === "superadmin" || role === "admin" || role === "user") {
				return res.status(400).json({
					error: `Cannot modify ${role} role permissions - this is a built-in role`,
				});
			}

			const permissions = await prisma.role_permissions.upsert({
				where: { role },
				update: {
					can_view_dashboard: can_view_dashboard,
					can_view_hosts: can_view_hosts,
					can_manage_hosts: can_manage_hosts,
					can_view_packages: can_view_packages,
					can_manage_packages: can_manage_packages,
					can_view_users: can_view_users,
					can_manage_users: can_manage_users,
					can_manage_superusers: can_manage_superusers,
					can_view_reports: can_view_reports,
					can_export_data: can_export_data,
					can_manage_settings: can_manage_settings,
					updated_at: new Date(),
				},
				create: {
					id: require("uuid").v4(),
					role,
					can_view_dashboard: can_view_dashboard,
					can_view_hosts: can_view_hosts,
					can_manage_hosts: can_manage_hosts,
					can_view_packages: can_view_packages,
					can_manage_packages: can_manage_packages,
					can_view_users: can_view_users,
					can_manage_users: can_manage_users,
					can_manage_superusers: can_manage_superusers,
					can_view_reports: can_view_reports,
					can_export_data: can_export_data,
					can_manage_settings: can_manage_settings,
					updated_at: new Date(),
				},
			});

			res.json({
				message: "Role permissions updated successfully",
				permissions,
			});
		} catch (error) {
			console.error("Update role permissions error:", error);
			res.status(500).json({ error: "Failed to update role permissions" });
		}
	},
);

// Delete a role (and its permissions)
router.delete(
	"/roles/:role",
	authenticateToken,
	requireManageSettings,
	async (req, res) => {
		try {
			const { role } = req.params;

			// Prevent deleting built-in roles (superadmin, admin, user)
			if (role === "superadmin" || role === "admin" || role === "user") {
				return res.status(400).json({
					error: `Cannot delete ${role} role - this is a built-in role`,
				});
			}

			// Check if any users are using this role
			const usersWithRole = await prisma.users.count({
				where: { role },
			});

			if (usersWithRole > 0) {
				return res.status(400).json({
					error: `Cannot delete role "${role}" because ${usersWithRole} user(s) are currently using it`,
				});
			}

			await prisma.role_permissions.delete({
				where: { role },
			});

			res.json({
				message: `Role "${role}" deleted successfully`,
			});
		} catch (error) {
			console.error("Delete role error:", error);
			res.status(500).json({ error: "Failed to delete role" });
		}
	},
);

// Get user's permissions based on their role
router.get("/user-permissions", authenticateToken, async (req, res) => {
	try {
		const userRole = req.user.role;

		const permissions = await prisma.role_permissions.findUnique({
			where: { role: userRole },
		});

		if (!permissions) {
			// If no specific permissions found, return default admin permissions
			return res.json({
				role: userRole,
				can_view_dashboard: true,
				can_view_hosts: true,
				can_manage_hosts: true,
				can_view_packages: true,
				can_manage_packages: true,
				can_view_users: true,
				can_manage_users: true,
				can_view_reports: true,
				can_export_data: true,
				can_manage_settings: true,
			});
		}

		res.json(permissions);
	} catch (error) {
		console.error("Get user permissions error:", error);
		res.status(500).json({ error: "Failed to fetch user permissions" });
	}
});

module.exports = router;
