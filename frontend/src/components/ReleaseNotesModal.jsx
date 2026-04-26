import { useQuery } from "@tanstack/react-query";
import {
	ArrowRight,
	Bell,
	ExternalLink,
	Heart,
	RefreshCw,
	Sparkles,
} from "lucide-react";
import { useCallback, useEffect, useId, useState } from "react";
import ReactMarkdown from "react-markdown";
import { useAuth } from "../contexts/AuthContext";
import { useSettings } from "../contexts/SettingsContext";
import { authAPI, versionAPI } from "../utils/api";

// Closing the modal or finishing the last step calls `acceptReleaseNotes` so
// the version is marked read even if the user skips later steps.
const ReleaseNotesModal = ({ isOpen, onAccept }) => {
	const { acceptReleaseNotes, user } = useAuth();
	const { settings: publicSettings } = useSettings();

	// Steps are captured when the modal opens so mid-session context updates
	// don't reshape `steps` under the current `stepIndex`. Newsletter signup
	// is deferred until the user finishes the flow (close / coffee) so we never
	// refetch profile mid-wizard. The subscribe step is self-hosted only and
	// omitted when `admin_mode` or already subscribed at open.
	const buildSteps = () => {
		const showSubscribeStep =
			!publicSettings?.admin_mode && !user?.newsletter_subscribed;
		return [
			"whatsNew",
			...(showSubscribeStep ? ["subscribe"] : []),
			"personalNote",
		];
	};

	const [steps, setSteps] = useState(buildSteps);
	const [stepIndex, setStepIndex] = useState(0);
	const [isAccepting, setIsAccepting] = useState(false);
	const [optIn, setOptIn] = useState(false);
	const stayUpdatedId = useId();
	const stepKey = steps[stepIndex] ?? "whatsNew";

	const { data: versionInfo } = useQuery({
		queryKey: ["versionInfo"],
		queryFn: () => versionAPI.getCurrent().then((res) => res.data),
		staleTime: 300000,
	});

	const { data: releaseNotes, isLoading } = useQuery({
		queryKey: ["releaseNotes", versionInfo?.version],
		queryFn: async () => {
			if (!versionInfo?.version) return null;
			const response = await fetch(
				`/api/v1/release-notes/${versionInfo.version}`,
				{ credentials: "include" },
			);
			return response.json();
		},
		enabled: isOpen && !!versionInfo?.version,
	});

	const isCloudVersion = window.location.hostname.endsWith(".patchmon.cloud");

	const handleAccept = useCallback(async () => {
		const maybeSubscribe = () => {
			if (!optIn || user?.newsletter_subscribed) return;
			void authAPI.subscribeNewsletter().catch(() => {});
		};

		const currentVersion = versionInfo?.version;
		if (!currentVersion) {
			maybeSubscribe();
			onAccept();
			return;
		}
		setIsAccepting(true);
		try {
			const result = await acceptReleaseNotes(currentVersion);
			if (!result.success) {
				console.error("Failed to accept release notes:", result.error);
			}
			maybeSubscribe();
			onAccept();
		} catch (error) {
			console.error("Error accepting release notes:", error);
			maybeSubscribe();
			onAccept();
		} finally {
			setIsAccepting(false);
		}
	}, [
		versionInfo?.version,
		acceptReleaseNotes,
		onAccept,
		optIn,
		user?.newsletter_subscribed,
	]);

	const handleClose = useCallback(async () => {
		await handleAccept();
	}, [handleAccept]);

	const handleNext = () => {
		setStepIndex((idx) => Math.min(idx + 1, steps.length - 1));
	};

	const handleBack = () => {
		setStepIndex((idx) => Math.max(idx - 1, 0));
	};

	const handleSubscribeNext = () => {
		handleNext();
	};

	const handleDonateNow = async () => {
		window.open(
			"https://buymeacoffee.com/iby___",
			"_blank",
			"noopener,noreferrer",
		);
		await handleAccept();
	};

	const handleMembershipNow = async () => {
		window.open(
			"https://buymeacoffee.com/iby___/membership",
			"_blank",
			"noopener,noreferrer",
		);
		await handleAccept();
	};

	// Reset state and capture the step list whenever the modal opens. Deps
	// are intentionally limited to `isOpen` so the running flow is stable.
	// biome-ignore lint/correctness/useExhaustiveDependencies: capture steps once on open
	useEffect(() => {
		if (isOpen) {
			setSteps(buildSteps());
			setStepIndex(0);
			setOptIn(false);
		}
	}, [isOpen]);

	useEffect(() => {
		if (!isOpen) return;
		const onKeyDown = (e) => {
			if (e.key === "Escape") {
				handleClose();
			}
		};
		window.addEventListener("keydown", onKeyDown);
		return () => window.removeEventListener("keydown", onKeyDown);
	}, [isOpen, handleClose]);

	if (!isOpen) return null;

	const currentVersion = versionInfo?.version || "unknown";
	const hasReleaseNotes = releaseNotes?.exists && releaseNotes?.content;

	const BuyMeACoffeeIcon = () => (
		<svg
			className="h-5 w-5 text-yellow-500 flex-shrink-0"
			viewBox="0 0 900 1300"
			fill="currentColor"
		>
			<title>Buy Me a Coffee</title>
			<path d="M879.567 341.849L872.53 306.352C866.215 274.503 851.882 244.409 819.19 232.898C808.711 229.215 796.821 227.633 788.786 220.01C780.751 212.388 778.376 200.55 776.518 189.572C773.076 169.423 769.842 149.257 766.314 129.143C763.269 111.85 760.86 92.4243 752.928 76.56C742.604 55.2584 721.182 42.8009 699.88 34.559C688.965 30.4844 677.826 27.0375 666.517 24.2352C613.297 10.1947 557.342 5.03277 502.591 2.09047C436.875 -1.53577 370.983 -0.443234 305.422 5.35968C256.625 9.79894 205.229 15.1674 158.858 32.0469C141.91 38.224 124.445 45.6399 111.558 58.7341C95.7448 74.8221 90.5829 99.7026 102.128 119.765C110.336 134.012 124.239 144.078 138.985 150.737C158.192 159.317 178.251 165.846 198.829 170.215C256.126 182.879 315.471 187.851 374.007 189.968C438.887 192.586 503.87 190.464 568.44 183.618C584.408 181.863 600.347 179.758 616.257 177.304C634.995 174.43 647.022 149.928 641.499 132.859C634.891 112.453 617.134 104.538 597.055 107.618C594.095 108.082 591.153 108.512 588.193 108.942L586.06 109.252C579.257 110.113 572.455 110.915 565.653 111.661C551.601 113.175 537.515 114.414 523.394 115.378C491.768 117.58 460.057 118.595 428.363 118.647C397.219 118.647 366.058 117.769 334.983 115.722C320.805 114.793 306.661 113.611 292.552 112.177C286.134 111.506 279.733 110.801 273.333 110.009L267.241 109.235L265.917 109.046L259.602 108.134C246.697 106.189 233.792 103.953 221.025 101.251C219.737 100.965 218.584 100.249 217.758 99.2193C216.932 98.1901 216.482 96.9099 216.482 95.5903C216.482 94.2706 216.932 92.9904 217.758 91.9612C218.584 90.9319 219.737 90.2152 221.025 89.9293H221.266C232.33 87.5721 243.479 85.5589 254.663 83.8038C258.392 83.2188 262.131 82.6453 265.882 82.0832H265.985C272.988 81.6186 280.026 80.3625 286.994 79.5366C347.624 73.2301 408.614 71.0801 469.538 73.1014C499.115 73.9618 528.676 75.6996 558.116 78.6935C564.448 79.3474 570.746 80.0357 577.043 80.8099C579.452 81.1025 581.878 81.4465 584.305 81.7391L589.191 82.4445C603.438 84.5667 617.61 87.1419 631.708 90.1703C652.597 94.7128 679.422 96.1925 688.713 119.077C691.673 126.338 693.015 134.408 694.649 142.03L696.732 151.752C696.786 151.926 696.826 152.105 696.852 152.285C701.773 175.227 706.7 198.169 711.632 221.111C711.994 222.806 712.002 224.557 711.657 226.255C711.312 227.954 710.621 229.562 709.626 230.982C708.632 232.401 707.355 233.6 705.877 234.504C704.398 235.408 702.75 235.997 701.033 236.236H700.895L697.884 236.649L694.908 237.044C685.478 238.272 676.038 239.419 666.586 240.486C647.968 242.608 629.322 244.443 610.648 245.992C573.539 249.077 536.356 251.102 499.098 252.066C480.114 252.57 461.135 252.806 442.162 252.771C366.643 252.712 291.189 248.322 216.173 239.625C208.051 238.662 199.93 237.629 191.808 236.58C198.106 237.389 187.231 235.96 185.029 235.651C179.867 234.928 174.705 234.177 169.543 233.397C152.216 230.798 134.993 227.598 117.7 224.793C96.7944 221.352 76.8005 223.073 57.8906 233.397C42.3685 241.891 29.8055 254.916 21.8776 270.735C13.7217 287.597 11.2956 305.956 7.64786 324.075C4.00009 342.193 -1.67805 361.688 0.472751 380.288C5.10128 420.431 33.165 453.054 73.5313 460.35C111.506 467.232 149.687 472.807 187.971 477.556C338.361 495.975 490.294 498.178 641.155 484.129C653.44 482.982 665.708 481.732 677.959 480.378C681.786 479.958 685.658 480.398 689.292 481.668C692.926 482.938 696.23 485.005 698.962 487.717C701.694 490.429 703.784 493.718 705.08 497.342C706.377 500.967 706.846 504.836 706.453 508.665L702.633 545.797C694.936 620.828 687.239 695.854 679.542 770.874C671.513 849.657 663.431 928.434 655.298 1007.2C653.004 1029.39 650.71 1051.57 648.416 1073.74C646.213 1095.58 645.904 1118.1 641.757 1139.68C635.218 1173.61 612.248 1194.45 578.73 1202.07C548.022 1209.06 516.652 1212.73 485.161 1213.01C450.249 1213.2 415.355 1211.65 380.443 1211.84C343.173 1212.05 297.525 1208.61 268.756 1180.87C243.479 1156.51 239.986 1118.36 236.545 1085.37C231.957 1041.7 227.409 998.039 222.9 954.381L197.607 711.615L181.244 554.538C180.968 551.94 180.693 549.376 180.435 546.76C178.473 528.023 165.207 509.681 144.301 510.627C126.407 511.418 106.069 526.629 108.168 546.76L120.298 663.214L145.385 904.104C152.532 972.528 159.661 1040.96 166.773 1109.41C168.15 1122.52 169.44 1135.67 170.885 1148.78C178.749 1220.43 233.465 1259.04 301.224 1269.91C340.799 1276.28 381.337 1277.59 421.497 1278.24C472.979 1279.07 524.977 1281.05 575.615 1271.72C650.653 1257.95 706.952 1207.85 714.987 1130.13C717.282 1107.69 719.576 1085.25 721.87 1062.8C729.498 988.559 737.115 914.313 744.72 840.061L769.601 597.451L781.009 486.263C781.577 480.749 783.905 475.565 787.649 471.478C791.392 467.391 796.352 464.617 801.794 463.567C823.25 459.386 843.761 452.245 859.023 435.916C883.318 409.918 888.153 376.021 879.567 341.849ZM72.4301 365.835C72.757 365.68 72.1548 368.484 71.8967 369.792C71.8451 367.813 71.9483 366.058 72.4301 365.835ZM74.5121 381.94C74.6842 381.819 75.2003 382.508 75.7337 383.334C74.925 382.576 74.4089 382.009 74.4949 381.94H74.5121ZM76.5597 384.641C77.2996 385.897 77.6953 386.689 76.5597 384.641V384.641ZM80.672 387.979H80.7752C80.7752 388.1 80.9645 388.22 81.0333 388.341C80.9192 388.208 80.7925 388.087 80.6548 387.979H80.672ZM800.796 382.989C793.088 390.319 781.473 393.726 769.996 395.43C641.292 414.529 510.713 424.199 380.597 419.932C287.476 416.749 195.336 406.407 103.144 393.382C94.1102 392.109 84.3197 390.457 78.1082 383.798C66.4078 371.237 72.1548 345.944 75.2003 330.768C77.9878 316.865 83.3218 298.334 99.8572 296.355C125.667 293.327 155.64 304.218 181.175 308.09C211.917 312.781 242.774 316.538 273.745 319.36C405.925 331.405 540.325 329.529 671.92 311.91C695.906 308.686 719.805 304.941 743.619 300.674C764.835 296.871 788.356 289.731 801.175 311.703C809.967 326.673 811.137 346.701 809.778 363.615C809.359 370.984 806.139 377.915 800.779 382.989H800.796Z" />
		</svg>
	);

	return (
		<div className="fixed inset-0 z-50 overflow-y-auto">
			<div className="flex items-center justify-center min-h-screen pt-4 px-4 pb-20 text-center sm:block sm:p-0">
				<button
					type="button"
					className="fixed inset-0 bg-secondary-500 bg-opacity-75 transition-opacity cursor-default"
					onClick={handleClose}
					aria-label="Close modal"
				/>

				<div className="inline-block align-bottom bg-white dark:bg-secondary-800 rounded-lg text-left overflow-hidden shadow-xl transform transition-all sm:my-8 sm:align-middle sm:max-w-2xl sm:w-full">
					<div className="bg-white dark:bg-secondary-800 px-4 pt-5 pb-4 sm:p-6 sm:pb-4">
						{/* Header */}
						<div className="flex items-center justify-between mb-4">
							<div className="flex items-center gap-2">
								{stepKey === "whatsNew" && (
									<Sparkles className="h-5 w-5 text-primary-600 dark:text-primary-400" />
								)}
								{stepKey === "subscribe" && (
									<Bell className="h-5 w-5 text-primary-600 dark:text-primary-400" />
								)}
								{stepKey === "personalNote" && (
									<Heart className="h-5 w-5 text-primary-600 dark:text-primary-400 fill-current" />
								)}
								<h3 className="text-lg font-medium text-secondary-900 dark:text-white">
									{stepKey === "whatsNew" && "What's New"}
									{stepKey === "subscribe" && "Stay Updated"}
									{stepKey === "personalNote" && "A Personal Note"}
								</h3>
								{stepKey === "whatsNew" && (
									<span className="text-xs bg-primary-100 dark:bg-primary-900 text-primary-800 dark:text-primary-200 px-2 py-1 rounded font-medium">
										v{currentVersion}
									</span>
								)}
							</div>
							<button
								type="button"
								onClick={handleClose}
								disabled={isAccepting}
								className="inline-flex items-center px-3 py-1.5 text-sm font-medium text-secondary-700 dark:text-white bg-secondary-100 dark:bg-secondary-700 border border-secondary-300 dark:border-secondary-600 rounded-md hover:bg-secondary-200 dark:hover:bg-secondary-600 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-secondary-500 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
								aria-label="Close"
							>
								Close
							</button>
						</div>

						{/* Step: What's New */}
						{stepKey === "whatsNew" && (
							<div className="max-h-[60vh] overflow-y-auto pr-2">
								{isLoading ? (
									<div className="flex items-center justify-center py-12">
										<RefreshCw className="h-8 w-8 animate-spin text-primary-600" />
									</div>
								) : hasReleaseNotes ? (
									<div className="prose prose-sm dark:prose-invert max-w-none">
										<ReactMarkdown
											components={{
												h1: ({ node, ...props }) => (
													<h1
														className="text-xl font-bold text-secondary-900 dark:text-white mb-3 mt-2"
														{...props}
													/>
												),
												h2: ({ node, ...props }) => (
													<h2
														className="text-lg font-semibold text-secondary-900 dark:text-white mt-5 mb-2"
														{...props}
													/>
												),
												h3: ({ node, ...props }) => (
													<h3
														className="text-base font-medium text-secondary-900 dark:text-white mt-4 mb-2"
														{...props}
													/>
												),
												p: ({ node, ...props }) => (
													<p
														className="text-sm text-secondary-600 dark:text-white mb-2 leading-relaxed"
														{...props}
													/>
												),
												ul: ({ node, ...props }) => (
													<ul
														className="list-disc list-inside text-sm text-secondary-600 dark:text-white mb-3 space-y-1.5 ml-2"
														{...props}
													/>
												),
												li: ({ node, ...props }) => (
													<li className="ml-2" {...props} />
												),
												strong: ({ node, ...props }) => (
													<strong
														className="font-semibold text-secondary-900 dark:text-white"
														{...props}
													/>
												),
												code: ({ node, ...props }) => (
													<code
														className="bg-secondary-100 dark:bg-secondary-700 px-1.5 py-0.5 rounded text-xs font-mono"
														{...props}
													/>
												),
												hr: ({ node, ...props }) => (
													<hr
														className="my-4 border-secondary-200 dark:border-secondary-600"
														{...props}
													/>
												),
											}}
										>
											{releaseNotes.content}
										</ReactMarkdown>
									</div>
								) : (
									<div className="text-center py-12">
										<p className="text-sm text-secondary-600 dark:text-white">
											No release notes available for this version.
										</p>
									</div>
								)}
							</div>
						)}

						{/* Step: Subscribe (self-hosted, not yet subscribed) */}
						{stepKey === "subscribe" && (
							<div className="max-h-[60vh] overflow-y-auto pr-2">
								<div className="space-y-5">
									<p className="text-sm text-secondary-600 dark:text-white leading-relaxed">
										Get security advisories and heads-up emails when important
										updates or releases affect how you run PatchMon. Optional —
										you can turn this on below.
									</p>

									<div className="flex items-start justify-between gap-4 p-4 bg-secondary-50 dark:bg-secondary-800/50 rounded-lg border border-secondary-200 dark:border-secondary-600">
										<div className="flex-1">
											<span
												id={stayUpdatedId}
												className="text-sm font-medium text-secondary-900 dark:text-secondary-100"
											>
												Email me security and release updates for PatchMon
											</span>
											{optIn && user?.email && (
												<div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-secondary-600 dark:text-secondary-400">
													<span>Using:</span>
													<span>{user.email}</span>
												</div>
											)}
										</div>
										<button
											type="button"
											role="switch"
											aria-checked={optIn}
											aria-labelledby={stayUpdatedId}
											onClick={() => setOptIn((v) => !v)}
											className={`relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-md border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 ${
												optIn
													? "bg-primary-600 dark:bg-primary-500"
													: "bg-secondary-200 dark:bg-secondary-600"
											}`}
										>
											<span
												className={`pointer-events-none inline-block h-5 w-5 transform rounded-md bg-white shadow ring-0 transition duration-200 ease-in-out ${
													optIn ? "translate-x-5" : "translate-x-0"
												}`}
											/>
										</button>
									</div>
								</div>
							</div>
						)}

						{/* Step: Personal Note */}
						{stepKey === "personalNote" && (
							<div className="max-h-[60vh] overflow-y-auto pr-2">
								<div className="space-y-6">
									<div>
										<h2 className="text-lg font-semibold text-secondary-900 dark:text-white mb-4">
											A personal note from the founder Iby
										</h2>
										<div className="space-y-4 text-sm text-secondary-600 dark:text-white leading-relaxed">
											<p>
												PatchMon wouldn't be what it is today without the
												incredible support the community has shown over the last
												6 months.
											</p>
											<p>
												Your feedback and dedication on Discord has been keeping
												me going - I love each and every one of you.
											</p>
										</div>
									</div>

									<div className="border-t border-secondary-200 dark:border-secondary-600 pt-6">
										<div className="space-y-3">
											<h3 className="text-base font-medium text-secondary-900 dark:text-white">
												{isCloudVersion
													? "Upgrade Your PatchMon Cloud Experience"
													: "Do you find PatchMon useful?"}
											</h3>
											<p className="text-sm text-secondary-600 dark:text-white leading-relaxed">
												{isCloudVersion
													? "Join as a member to unlock premium features like isolated resources, custom domains, priority support, and more. Help support the project while getting enhanced capabilities for your PatchMon Cloud instance."
													: "Please consider supporting the project to keep the project OpenSource to help me maintain the infrastructure and develop new features."}
											</p>
										</div>
									</div>
								</div>
							</div>
						)}
					</div>

					{/* Footer: What's New */}
					{stepKey === "whatsNew" && (
						<div className="bg-secondary-50 dark:bg-secondary-700 px-4 py-3 sm:px-6 sm:flex sm:flex-row-reverse">
							<button
								type="button"
								onClick={handleNext}
								className="w-full inline-flex justify-center items-center gap-2 rounded-md border border-transparent shadow-sm px-4 py-2 text-base font-medium text-white bg-primary-600 hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500 sm:ml-3 sm:w-auto sm:text-sm transition-colors min-h-[44px]"
							>
								Next
								<ArrowRight className="h-4 w-4" />
							</button>
						</div>
					)}

					{/* Footer: Subscribe */}
					{stepKey === "subscribe" && (
						<div className="bg-secondary-50 dark:bg-secondary-700 px-4 py-3 sm:px-6 sm:flex sm:flex-row-reverse sm:items-center sm:gap-3">
							<button
								type="button"
								onClick={handleSubscribeNext}
								className="w-full inline-flex justify-center items-center gap-2 rounded-md border border-transparent shadow-sm px-4 py-2 text-base font-medium text-white bg-primary-600 hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500 sm:ml-3 sm:w-auto sm:text-sm transition-colors min-h-[44px]"
							>
								Next
								<ArrowRight className="h-4 w-4" />
							</button>
							<button
								type="button"
								onClick={handleBack}
								className="mt-3 sm:mt-0 w-full sm:w-auto inline-flex justify-center items-center px-4 py-2 text-sm font-medium text-secondary-700 dark:text-white bg-white dark:bg-secondary-800 border border-secondary-300 dark:border-secondary-600 rounded-md hover:bg-secondary-50 dark:hover:bg-secondary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-secondary-500 transition-colors min-h-[44px]"
							>
								Back
							</button>
						</div>
					)}

					{/* Footer: Personal Note (terminal — fires acceptance) */}
					{stepKey === "personalNote" && (
						<div className="bg-secondary-50 dark:bg-secondary-700 px-4 py-3 sm:px-6 sm:flex sm:flex-row-reverse sm:items-center sm:gap-3">
							<button
								type="button"
								onClick={isCloudVersion ? handleMembershipNow : handleDonateNow}
								disabled={isAccepting}
								className={`w-full inline-flex justify-center items-center gap-2 rounded-md border border-transparent shadow-sm px-4 py-2 text-base font-medium text-white sm:w-auto sm:text-sm transition-colors min-h-[44px] ${
									isAccepting
										? "bg-secondary-400 cursor-not-allowed"
										: "bg-secondary-800 dark:bg-secondary-800 hover:bg-secondary-700 dark:hover:bg-secondary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-secondary-500"
								}`}
							>
								{isAccepting ? (
									<>
										<RefreshCw className="h-4 w-4 animate-spin" />
										Saving...
									</>
								) : (
									<>
										{isCloudVersion ? (
											<>
												<Heart className="h-4 w-4" />
												Join Membership
											</>
										) : (
											<>
												<BuyMeACoffeeIcon />
												Buy me a Coffee
											</>
										)}
										<ExternalLink className="h-4 w-4" />
									</>
								)}
							</button>
						</div>
					)}
				</div>
			</div>
		</div>
	);
};

export default ReleaseNotesModal;
