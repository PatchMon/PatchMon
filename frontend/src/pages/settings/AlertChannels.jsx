import { Bell } from "lucide-react";

const AlertChannels = () => {
	return (
		<div className="space-y-6">
			{/* Header */}
			<div className="flex items-center justify-between">
				<div>
					<h1 className="text-2xl font-bold text-secondary-900 dark:text-white">
						Alert Channels
					</h1>
					<p className="mt-1 text-sm text-secondary-600 dark:text-secondary-400">
						Configure how PatchMon sends notifications and alerts
					</p>
				</div>
			</div>

			{/* Coming Soon Card */}
			<div className="bg-white dark:bg-secondary-800 border border-secondary-200 dark:border-secondary-600 rounded-lg p-6">
				<div className="flex items-center gap-4">
					<div className="flex-shrink-0">
						<div className="w-12 h-12 bg-primary-100 dark:bg-primary-900 rounded-lg flex items-center justify-center">
							<Bell className="h-6 w-6 text-primary-600 dark:text-primary-400" />
						</div>
					</div>
					<div className="flex-1">
						<h3 className="text-lg font-semibold text-secondary-900 dark:text-white">
							Alert Channels Coming Soon
						</h3>
						<p className="mt-1 text-sm text-secondary-600 dark:text-secondary-400">
							We're working on adding support for multiple alert channels
							including email, Slack, Discord, and webhooks.
						</p>
						<div className="mt-3">
							<span className="inline-flex items-center px-2.5 py-0.5 rounded-md text-xs font-medium bg-primary-100 text-primary-800 dark:bg-primary-900 dark:text-primary-200">
								In Development
							</span>
						</div>
					</div>
				</div>
			</div>

			{/* Placeholder for future channels */}
			<div className="bg-white dark:bg-secondary-800 border border-secondary-200 dark:border-secondary-600 rounded-lg">
				<div className="px-6 py-4 border-b border-secondary-200 dark:border-secondary-600">
					<h3 className="text-lg font-medium text-secondary-900 dark:text-white">
						Alert Channels
					</h3>
					<p className="mt-1 text-sm text-secondary-600 dark:text-secondary-400">
						No alert channels configured yet
					</p>
				</div>
				<div className="px-6 py-8 text-center">
					<Bell className="mx-auto h-12 w-12 text-secondary-400" />
					<h3 className="mt-2 text-sm font-medium text-secondary-900 dark:text-white">
						No channels
					</h3>
					<p className="mt-1 text-sm text-secondary-500 dark:text-secondary-400">
						Get started by adding your first alert channel.
					</p>
				</div>
			</div>
		</div>
	);
};

export default AlertChannels;
