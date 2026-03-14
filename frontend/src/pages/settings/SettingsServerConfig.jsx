import { Code, Image, Server } from "lucide-react";
import { useEffect, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import BrandingTab from "../../components/settings/BrandingTab";
import ProtocolUrlTab from "../../components/settings/ProtocolUrlTab";
import VersionUpdateTab from "../../components/settings/VersionUpdateTab";

const SettingsServerConfig = () => {
	const location = useLocation();
	const navigate = useNavigate();
	const [activeTab, setActiveTab] = useState(() => {
		// Set initial tab based on current route
		if (location.pathname === "/settings/server-version") return "version";
		if (location.pathname === "/settings/server-url") return "protocol";
		if (location.pathname === "/settings/branding") return "branding";
		if (location.pathname === "/settings/server-config/version")
			return "version";
		return "protocol";
	});

	// Update active tab when route changes
	useEffect(() => {
		if (location.pathname === "/settings/server-version") {
			setActiveTab("version");
		} else if (location.pathname === "/settings/server-url") {
			setActiveTab("protocol");
		} else if (location.pathname === "/settings/branding") {
			setActiveTab("branding");
		} else if (location.pathname === "/settings/server-config/version") {
			setActiveTab("version");
		} else if (location.pathname === "/settings/server-config") {
			setActiveTab("protocol");
		}
	}, [location.pathname]);

	const tabs = [
		{
			id: "protocol",
			name: "Server URL",
			icon: Server,
			href: "/settings/server-url",
		},
		{
			id: "branding",
			name: "Branding",
			icon: Image,
			href: "/settings/branding",
		},
		{
			id: "version",
			name: "Server Version",
			icon: Code,
			href: "/settings/server-version",
		},
	];

	const renderTabContent = () => {
		switch (activeTab) {
			case "protocol":
				return <ProtocolUrlTab />;
			case "branding":
				return <BrandingTab />;
			case "version":
				return <VersionUpdateTab />;
			default:
				return <ProtocolUrlTab />;
		}
	};

	return (
		<div className="space-y-6">
			{/* Tab Navigation */}
			<div className="border-b border-secondary-200 dark:border-secondary-600">
				<nav className="-mb-px flex space-x-8">
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
										: "border-transparent text-secondary-500 dark:text-white hover:text-secondary-700 dark:hover:text-secondary-300 hover:border-secondary-300 dark:hover:border-secondary-500"
								}`}
								title={tab.name}
							>
								<Icon className="h-4 w-4" />
								{tab.name}
							</button>
						);
					})}
				</nav>
			</div>

			{/* Tab Content */}
			<div className="mt-6">{renderTabContent()}</div>
		</div>
	);
};

export default SettingsServerConfig;
