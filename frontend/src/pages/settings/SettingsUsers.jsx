import { useQuery } from "@tanstack/react-query";
import { Plus, Shield, Users } from "lucide-react";
import { useEffect, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import SettingsLayout from "../../components/SettingsLayout";
import RolesTab from "../../components/settings/RolesTab";
import UsersTab from "../../components/settings/UsersTab";

const SettingsUsers = () => {
	const location = useLocation();
	const navigate = useNavigate();
	const [activeTab, setActiveTab] = useState(() => {
		// Set initial tab based on current route
		if (location.pathname === "/settings/roles") return "roles";
		return "users";
	});

	// Fetch OIDC config to determine if OIDC is enabled
	const { data: oidcConfig } = useQuery({
		queryKey: ["oidcConfig"],
		queryFn: async () => {
			const response = await fetch("/api/v1/auth/oidc/config");
			if (response.ok) {
				return response.json();
			}
			return { enabled: false };
		},
	});

	const isOIDCEnabled = oidcConfig?.enabled || false;

	const tabs = [
		{ id: "users", name: "Users", icon: Users, href: "/settings/users" },
		{ id: "roles", name: "Roles", icon: Shield, href: "/settings/roles" },
	];

	// Update active tab when route changes
	useEffect(() => {
		if (location.pathname === "/settings/roles") {
			setActiveTab("roles");
		} else if (location.pathname === "/settings/users") {
			setActiveTab("users");
		}
	}, [location.pathname]);

	const renderTabContent = () => {
		switch (activeTab) {
			case "users":
				return <UsersTab />;
			case "roles":
				return <RolesTab />;
			default:
				return <UsersTab />;
		}
	};

	return (
		<SettingsLayout>
			<div className="space-y-6">
				{/* Tab Navigation */}
				<div className="border-b border-secondary-200 dark:border-secondary-600">
					<nav className="-mb-px flex items-center justify-between">
						<div className="flex space-x-8">
							{tabs.map((tab) => {
								const Icon = tab.icon;
								return (
									<button
										type="button"
										key={tab.id}
										onClick={() => {
											setActiveTab(tab.id);
											navigate(tab.href);
										}}
										className={`py-4 px-1 border-b-2 font-medium text-sm flex items-center gap-2 ${
											activeTab === tab.id
												? "border-primary-500 text-primary-600 dark:text-primary-400"
												: "border-transparent text-secondary-500 dark:text-secondary-400 hover:text-secondary-700 dark:hover:text-secondary-300 hover:border-secondary-300 dark:hover:border-secondary-500"
										}`}
									>
										<Icon className="h-4 w-4" />
										{tab.name}
									</button>
								);
							})}
						</div>
						{activeTab === "users" && !isOIDCEnabled && (
							<button
								type="button"
								onClick={() =>
									window.dispatchEvent(new Event("openAddUserModal"))
								}
								className="btn-primary flex items-center gap-2"
								title="Add user"
							>
								<Plus className="h-4 w-4" />
								Add User
							</button>
						)}
						{activeTab === "roles" && !isOIDCEnabled && (
							<button
								type="button"
								onClick={() =>
									window.dispatchEvent(new Event("openAddRoleModal"))
								}
								className="btn-primary flex items-center gap-2"
								title="Add role"
							>
								<Plus className="h-4 w-4" />
								Add Role
							</button>
						)}
					</nav>
				</div>

				{/* Tab Content */}
				<div className="mt-6">{renderTabContent()}</div>
			</div>
		</SettingsLayout>
	);
};

export default SettingsUsers;
