import { Code, Image, Server } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import BrandingTab from "../../components/settings/BrandingTab";
import ProtocolUrlTab from "../../components/settings/ProtocolUrlTab";
import VersionUpdateTab from "../../components/settings/VersionUpdateTab";
import { useSettings } from "../../contexts/SettingsContext";

const SettingsServerConfig = () => {
	const location = useLocation();
	const navigate = useNavigate();
	const { settings: publicSettings } = useSettings();
	const isAdminMode = publicSettings?.admin_mode;

	const allTabs = useMemo(() => {
		const tabs = [];
		if (!isAdminMode) {
			tabs.push({
				id: "protocol",
				name: "Server URL",
				icon: Server,
				href: "/settings/server-url",
			});
		}
		tabs.push({
			id: "branding",
			name: "Branding",
			icon: Image,
			href: "/settings/branding",
		});
		if (!isAdminMode) {
			tabs.push({
				id: "version",
				name: "Server Version",
				icon: Code,
				href: "/settings/server-version",
			});
		}
		return tabs;
	}, [isAdminMode]);

	// Determine initial tab from route, falling back to first available tab.
	const resolveTab = useCallback(
		(pathname) => {
			if (!isAdminMode) {
				if (
					pathname === "/settings/server-version" ||
					pathname === "/settings/server-config/version"
				)
					return "version";
				if (
					pathname === "/settings/server-url" ||
					pathname === "/settings/server-config"
				)
					return "protocol";
			}
			if (pathname === "/settings/branding") return "branding";
			return allTabs[0]?.id ?? "branding";
		},
		[isAdminMode, allTabs],
	);

	const [activeTab, setActiveTab] = useState(() =>
		resolveTab(location.pathname),
	);

	// Update active tab when route changes
	useEffect(() => {
		setActiveTab(resolveTab(location.pathname));
	}, [location.pathname, resolveTab]);

	const renderTabContent = () => {
		switch (activeTab) {
			case "protocol":
				return <ProtocolUrlTab />;
			case "branding":
				return <BrandingTab />;
			case "version":
				return <VersionUpdateTab />;
			default:
				return <BrandingTab />;
		}
	};

	return (
		<div className="space-y-6">
			{/* Tab Navigation */}
			<div className="border-b border-secondary-200 dark:border-secondary-600">
				<nav className="-mb-px flex space-x-8">
					{allTabs.map((tab) => {
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
										: "border-transparent text-secondary-500 hover:text-secondary-700 hover:border-secondary-300 dark:text-white dark:hover:text-primary-400"
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
