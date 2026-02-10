import { useState } from "react";
import NotificationChannels from "../../components/NotificationChannels";
import NotificationHistory from "../../components/NotificationHistory";
import NotificationRules from "../../components/NotificationRules";

const Notifications = () => {
	const [activeTab, setActiveTab] = useState("channels");

	return (
		<div className="space-y-6">
			{/* Header */}
			<div className="flex items-center justify-between">
				<div>
					<h1 className="text-2xl font-bold text-secondary-900 dark:text-white">
						Notifications
					</h1>
					<p className="mt-1 text-sm text-secondary-600 dark:text-secondary-400">
						Configure notification preferences and alert rules
					</p>
				</div>
			</div>

			{/* Tabs */}
			<div className="border-b border-secondary-200 dark:border-secondary-600">
				<div className="flex space-x-8">
					<button
						type="button"
						onClick={() => setActiveTab("channels")}
						className={`py-4 px-1 border-b-2 font-medium text-sm ${
							activeTab === "channels"
								? "border-primary-500 text-primary-600 dark:text-primary-400"
								: "border-transparent text-secondary-500 dark:text-secondary-400 hover:text-secondary-700 dark:hover:text-secondary-300 hover:border-secondary-300 dark:hover:border-secondary-600"
						}`}
					>
						Channels
					</button>
					<button
						type="button"
						onClick={() => setActiveTab("rules")}
						className={`py-4 px-1 border-b-2 font-medium text-sm ${
							activeTab === "rules"
								? "border-primary-500 text-primary-600 dark:text-primary-400"
								: "border-transparent text-secondary-500 dark:text-secondary-400 hover:text-secondary-700 dark:hover:text-secondary-300 hover:border-secondary-300 dark:hover:border-secondary-600"
						}`}
					>
						Rules
					</button>
					<button
						type="button"
						onClick={() => setActiveTab("history")}
						className={`py-4 px-1 border-b-2 font-medium text-sm ${
							activeTab === "history"
								? "border-primary-500 text-primary-600 dark:text-primary-400"
								: "border-transparent text-secondary-500 dark:text-secondary-400 hover:text-secondary-700 dark:hover:text-secondary-300 hover:border-secondary-300 dark:hover:border-secondary-600"
						}`}
					>
						History
					</button>
				</div>
			</div>

			{/* Tab Content */}
			<div>
				{activeTab === "channels" && <NotificationChannels />}

				{activeTab === "rules" && <NotificationRules />}

				{activeTab === "history" && <NotificationHistory />}
			</div>
		</div>
	);
};

export default Notifications;
