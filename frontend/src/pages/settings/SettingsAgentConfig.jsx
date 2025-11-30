import { CheckCircle, Code, Settings } from "lucide-react";
import { useEffect, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import SettingsLayout from "../../components/SettingsLayout";
import AgentManagementTab from "../../components/settings/AgentManagementTab";
import AgentUpdatesTab from "../../components/settings/AgentUpdatesTab";

const SettingsAgentConfig = () => {
	const location = useLocation();
	const navigate = useNavigate();
	const [activeTab, setActiveTab] = useState(() => {
		// Set initial tab based on current route
		if (location.pathname === "/settings/agent-version") return "management";
		return "updates";
	});

	// Update active tab when route changes
	useEffect(() => {
		if (location.pathname === "/settings/agent-version") {
			setActiveTab("management");
		} else if (location.pathname === "/settings/agent-config") {
			setActiveTab("updates");
		}
	}, [location.pathname]);

	const tabs = [
		{
			id: "updates",
			name: "Agent Updates",
			icon: Settings,
			href: "/settings/agent-config",
		},
		{
			id: "management",
			name: "Agent Version",
			icon: Code,
			href: "/settings/agent-version",
		},
	];

	const renderTabContent = () => {
		switch (activeTab) {
			case "updates":
				return <AgentUpdatesTab />;
			case "management":
				return <AgentManagementTab />;
			default:
				return <AgentUpdatesTab />;
		}
	};

	return (
		<SettingsLayout>
			<div className="space-y-6">
				{/* Mobile Button Navigation */}
				<div className="md:hidden space-y-2">
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
								className={`w-full flex items-center justify-between px-4 py-3 rounded-md font-medium text-sm transition-colors ${
									activeTab === tab.id
										? "bg-primary-50 dark:bg-primary-900/20 text-primary-600 dark:text-primary-400 border border-primary-200 dark:border-primary-800"
										: "bg-secondary-50 dark:bg-secondary-700 text-secondary-700 dark:text-secondary-300 border border-secondary-200 dark:border-secondary-600 hover:bg-secondary-100 dark:hover:bg-secondary-600"
								}`}
							>
								<div className="flex items-center space-x-3">
									<Icon className="h-5 w-5" />
									<span>{tab.name}</span>
								</div>
								{activeTab === tab.id && (
									<CheckCircle className="h-5 w-5 text-primary-600 dark:text-primary-400" />
								)}
							</button>
						);
					})}
				</div>

				{/* Desktop Tab Navigation */}
				<div className="hidden md:block border-b border-secondary-200 dark:border-secondary-600">
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
											: "border-transparent text-secondary-500 dark:text-secondary-400 hover:text-secondary-700 dark:hover:text-secondary-300 hover:border-secondary-300 dark:hover:border-secondary-500"
									}`}
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
		</SettingsLayout>
	);
};

export default SettingsAgentConfig;
