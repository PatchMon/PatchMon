import { GripVertical, List, Settings as SettingsIcon, X } from "lucide-react";

/**
 * Small modal shown when the user clicks the dashboard settings icon.
 * Offers two options: change order through list, or drag cards on the dashboard.
 */
const DashboardSettingsChoiceModal = ({
	is_open,
	on_close,
	on_choose_list,
	on_choose_drag,
}) => {
	if (!is_open) return null;

	return (
		<div className="fixed inset-0 z-50 overflow-y-auto">
			<div className="flex items-center justify-center min-h-screen pt-4 px-4 pb-20 text-center sm:block sm:p-0">
				<button
					type="button"
					className="fixed inset-0 bg-secondary-500 bg-opacity-75 transition-opacity cursor-default"
					onClick={on_close}
					aria-label="Close"
				/>

				<div className="inline-block align-bottom bg-white dark:bg-secondary-800 rounded-lg text-left overflow-hidden shadow-xl transform transition-all sm:my-8 sm:align-middle sm:max-w-md sm:w-full">
					<div className="bg-white dark:bg-secondary-800 px-4 pt-5 pb-4 sm:p-6">
						<div className="flex items-center justify-between mb-4">
							<div className="flex items-center gap-2">
								<SettingsIcon className="h-5 w-5 text-primary-600" />
								<h3 className="text-lg font-medium text-secondary-900 dark:text-white">
									Customize dashboard
								</h3>
							</div>
							<button
								type="button"
								onClick={on_close}
								className="text-secondary-400 hover:text-secondary-600 dark:text-secondary-500 dark:hover:text-secondary-300"
								aria-label="Close"
							>
								<X className="h-5 w-5" />
							</button>
						</div>

						<p className="text-sm text-secondary-600 dark:text-secondary-400 mb-4">
							Choose how you want to reorder your dashboard cards:
						</p>

						<div className="space-y-2">
							<button
								type="button"
								onClick={() => {
									on_close();
									on_choose_list();
								}}
								className="w-full flex items-center gap-3 p-3 rounded-lg border border-secondary-200 dark:border-secondary-600 bg-white dark:bg-secondary-800 text-left hover:bg-secondary-50 dark:hover:bg-secondary-700 transition-colors"
							>
								<List className="h-5 w-5 text-primary-600 shrink-0" />
								<div>
									<span className="font-medium text-secondary-900 dark:text-white block">
										Change order through list
									</span>
									<span className="text-sm text-secondary-500 dark:text-secondary-400">
										Open a list of all cards to reorder and show/hide them
									</span>
								</div>
							</button>

							<button
								type="button"
								onClick={() => {
									on_close();
									on_choose_drag();
								}}
								className="w-full flex items-center gap-3 p-3 rounded-lg border border-secondary-200 dark:border-secondary-600 bg-white dark:bg-secondary-800 text-left hover:bg-secondary-50 dark:hover:bg-secondary-700 transition-colors"
							>
								<GripVertical className="h-5 w-5 text-primary-600 shrink-0" />
								<div>
									<span className="font-medium text-secondary-900 dark:text-white block">
										Drag cards on dashboard
									</span>
									<span className="text-sm text-secondary-500 dark:text-secondary-400">
										Drag and drop cards directly on the dashboard to reorder
									</span>
								</div>
							</button>
						</div>
					</div>
				</div>
			</div>
		</div>
	);
};

export default DashboardSettingsChoiceModal;
