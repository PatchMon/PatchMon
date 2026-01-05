const { PrismaClient } = require("@prisma/client");
const { v4: uuidv4 } = require("uuid");

const prisma = new PrismaClient();

async function seedRolePermissions() {
	console.log("Seeding role permissions...");

	const roles = [
		{
			role: "superadmin",
			can_view_dashboard: true,
			can_view_hosts: true,
			can_manage_hosts: true,
			can_view_packages: true,
			can_manage_packages: true,
			can_view_users: true,
			can_manage_users: true,
			can_manage_superusers: true,
			can_view_reports: true,
			can_export_data: true,
			can_manage_settings: true,
		},
		{
			role: "admin",
			can_view_dashboard: true,
			can_view_hosts: true,
			can_manage_hosts: true,
			can_view_packages: true,
			can_manage_packages: true,
			can_view_users: true,
			can_manage_users: true,
			can_manage_superusers: false,
			can_view_reports: true,
			can_export_data: true,
			can_manage_settings: true,
		},
		{
			role: "host_manager",
			can_view_dashboard: true,
			can_view_hosts: true,
			can_manage_hosts: true,
			can_view_packages: true,
			can_manage_packages: true,
			can_view_users: false,
			can_manage_users: false,
			can_manage_superusers: false,
			can_view_reports: true,
			can_export_data: true,
			can_manage_settings: false,
		},
		{
			role: "readonly",
			can_view_dashboard: true,
			can_view_hosts: true,
			can_manage_hosts: false,
			can_view_packages: true,
			can_manage_packages: false,
			can_view_users: false,
			can_manage_users: false,
			can_manage_superusers: false,
			can_view_reports: true,
			can_export_data: false,
			can_manage_settings: false,
		},
		{
			role: "user",
			can_view_dashboard: true,
			can_view_hosts: true,
			can_manage_hosts: false,
			can_view_packages: true,
			can_manage_packages: false,
			can_view_users: false,
			can_manage_users: false,
			can_manage_superusers: false,
			can_view_reports: true,
			can_export_data: false,
			can_manage_settings: false,
		},
	];

	for (const roleData of roles) {
		await prisma.role_permissions.upsert({
			where: { role: roleData.role },
			update: {
				...roleData,
				updated_at: new Date(),
			},
			create: {
				id: uuidv4(),
				...roleData,
				updated_at: new Date(),
			},
		});
		console.log(`  ✓ Role "${roleData.role}" permissions configured`);
	}

	console.log("Role permissions seeded successfully!");
}

module.exports = { seedRolePermissions };
