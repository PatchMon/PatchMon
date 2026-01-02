const { getPrismaClient } = require("../config/prisma");
const prisma = getPrismaClient();

// Permission middleware factory
const requirePermission = (permission) => {
	return async (req, res, next) => {
		try {
			// Get user's role permissions
			const rolePermissions = await prisma.role_permissions.findUnique({
				where: { role: req.user.role },
			});

			// If no specific permissions found, deny access (secure by default)
			// Only 'admin' role is allowed without explicit permissions for backward compatibility
			if (!rolePermissions) {
				if (req.user.role === "admin") {
					console.warn(
						`No permissions found for admin role, allowing access for backward compatibility`,
					);
					return next();
				}
				console.error(
					`Access denied: No permissions found for role: ${req.user.role}`,
				);
				return res.status(403).json({
					error: "Access denied",
					message: "Your role does not have the required permissions",
				});
			}

			// Check if user has the required permission
			if (!rolePermissions[permission]) {
				return res.status(403).json({
					error: "Insufficient permissions",
					message: `You don't have permission to ${permission.replace("can_", "").replace("_", " ")}`,
				});
			}

			next();
		} catch (error) {
			console.error("Permission check error:", error);
			res.status(500).json({ error: "Permission check failed" });
		}
	};
};

// Specific permission middlewares - using snake_case field names
const requireViewDashboard = requirePermission("can_view_dashboard");
const requireViewHosts = requirePermission("can_view_hosts");
const requireManageHosts = requirePermission("can_manage_hosts");
const requireViewPackages = requirePermission("can_view_packages");
const requireManagePackages = requirePermission("can_manage_packages");
const requireViewUsers = requirePermission("can_view_users");
const requireManageUsers = requirePermission("can_manage_users");
const requireViewReports = requirePermission("can_view_reports");
const requireExportData = requirePermission("can_export_data");
const requireManageSettings = requirePermission("can_manage_settings");

module.exports = {
	requirePermission,
	requireViewDashboard,
	requireViewHosts,
	requireManageHosts,
	requireViewPackages,
	requireManagePackages,
	requireViewUsers,
	requireManageUsers,
	requireViewReports,
	requireExportData,
	requireManageSettings,
};
