import { Heart } from "lucide-react";
import BuyMeACoffeeIcon from "./BuyMeACoffeeIcon";

const DONATE_URL = "https://buymeacoffee.com/iby___";

const DonateModal = ({ isOpen, onClose }) => {
	if (!isOpen) return null;

	const handleDonate = () => {
		window.open(DONATE_URL, "_blank", "noopener,noreferrer");
		onClose();
	};

	return (
		<div className="fixed inset-0 z-50 overflow-y-auto">
			<div className="flex items-center justify-center min-h-screen pt-4 px-4 pb-20 text-center sm:block sm:p-0">
				<button
					type="button"
					className="fixed inset-0 bg-secondary-500 bg-opacity-75 transition-opacity cursor-default"
					onClick={onClose}
					aria-label="Close modal"
				/>

				<div className="inline-block align-bottom bg-white dark:bg-secondary-800 rounded-lg text-left overflow-hidden shadow-xl transform transition-all sm:my-8 sm:align-middle sm:max-w-2xl sm:w-full">
					<div className="bg-white dark:bg-secondary-800 px-4 pt-5 pb-4 sm:p-6 sm:pb-4">
						{/* Header */}
						<div className="flex items-center justify-between mb-4">
							<div className="flex items-center gap-2">
								<Heart className="h-5 w-5 text-primary-600 dark:text-primary-400 fill-current" />
								<h3 className="text-lg font-medium text-secondary-900 dark:text-white">
									A Personal Note
								</h3>
							</div>
							<button
								type="button"
								onClick={onClose}
								className="inline-flex items-center px-3 py-1.5 text-sm font-medium text-secondary-700 dark:text-white bg-secondary-100 dark:bg-secondary-700 border border-secondary-300 dark:border-secondary-600 rounded-md hover:bg-secondary-200 dark:hover:bg-secondary-600 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-secondary-500 transition-colors"
								aria-label="Close"
							>
								Close
							</button>
						</div>

						{/* Content - Founder's note */}
						<div className="max-h-[60vh] overflow-y-auto pr-2">
							<div className="space-y-6">
								<div>
									<h2 className="text-lg font-semibold text-secondary-900 dark:text-white mb-4">
										A personal note from the founder Iby
									</h2>
									<div className="space-y-4 text-sm text-secondary-600 dark:text-white leading-relaxed">
										<p>
											PatchMon wouldn't be what it is today without the
											incredible support the community has shown over the last 6
											months.
										</p>
										<p>
											Your feedback and dedication on Discord has been keeping
											me going - I love each and every one of you.
										</p>
										<p className="font-medium text-secondary-800 dark:text-secondary-200">
											PatchMon is entirely self-funded. There are no investors
											or VC backing, just me building this in my spare time.
											Your donations help keep the project Open Source, maintain
											the infrastructure, and develop new features.
										</p>
									</div>
								</div>
							</div>
						</div>
					</div>

					{/* Footer */}
					<div className="bg-secondary-50 dark:bg-secondary-700 px-4 py-3 sm:px-6 sm:flex sm:flex-row-reverse sm:items-center sm:gap-3">
						<button
							type="button"
							onClick={handleDonate}
							className="w-full inline-flex justify-center items-center gap-2 rounded-md border border-transparent shadow-sm px-4 py-2 text-base font-medium text-white bg-secondary-800 dark:bg-secondary-800 hover:bg-secondary-700 dark:hover:bg-secondary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-secondary-500 sm:w-auto sm:text-sm transition-colors"
						>
							<BuyMeACoffeeIcon className="h-5 w-5 text-yellow-500" />
							Donate a coffee
							<Heart className="h-4 w-4 fill-current" />
						</button>
					</div>
				</div>
			</div>
		</div>
	);
};

export default DonateModal;
