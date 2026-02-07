import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertCircle, CheckCircle, Save, Shield, X } from "lucide-react";
import { useEffect, useId, useState } from "react";
import { settingsAPI } from "../../utils/api";

const AgentUpdatesTab = () => {
	const updateIntervalId = useId();
	const autoUpdateId = useId();
	const ignoreSslId = useId();
	const [formData, setFormData] = useState({
		updateInterval: 60,
		autoUpdate: false,
		ignoreSslSelfSigned: false,
	});
	const [errors, setErrors] = useState({});
	const [isDirty, setIsDirty] = useState(false);
	const [toast, setToast] = useState(null);

	const queryClient = useQueryClient();

	// Auto-hide toast after 3 seconds
	useEffect(() => {
		if (toast) {
			const timer = setTimeout(() => {
				setToast(null);
			}, 3000);
			return () => clearTimeout(timer);
		}
	}, [toast]);

	const showToast = (message, type = "success") => {
		setToast({ message, type });
	};

	// Fallback clipboard copy function for HTTP and older browsers
	const copyToClipboard = async (text) => {
		// Try modern clipboard API first
		if (navigator.clipboard?.writeText) {
			try {
				await navigator.clipboard.writeText(text);
				return true;
			} catch (err) {
				console.warn("Clipboard API failed, using fallback:", err);
			}
		}

		// Fallback for HTTP or unsupported browsers
		try {
			const textArea = document.createElement("textarea");
			textArea.value = text;
			textArea.style.position = "fixed";
			textArea.style.left = "-999999px";
			textArea.style.top = "-999999px";
			document.body.appendChild(textArea);
			textArea.focus();
			textArea.select();

			const successful = document.execCommand("copy");
			document.body.removeChild(textArea);

			if (successful) {
				return true;
			}
			throw new Error("execCommand failed");
		} catch (err) {
			console.error("Fallback copy failed:", err);
			throw err;
		}
	};

	// Fetch current settings
	const {
		data: settings,
		isLoading,
		error,
	} = useQuery({
		queryKey: ["settings"],
		queryFn: () => settingsAPI.get().then((res) => res.data),
	});

	// Update form data when settings are loaded
	useEffect(() => {
		if (settings) {
			const newFormData = {
				updateInterval: settings.update_interval || 60,
				autoUpdate: settings.auto_update || false,
				ignoreSslSelfSigned: settings.ignore_ssl_self_signed === true,
			};
			setFormData(newFormData);
			setIsDirty(false);
		}
	}, [settings]);

	// Update settings mutation
	const updateSettingsMutation = useMutation({
		mutationFn: (data) => {
			return settingsAPI.update(data).then((res) => res.data);
		},
		onSuccess: () => {
			queryClient.invalidateQueries(["settings"]);
			setIsDirty(false);
			setErrors({});
		},
		onError: (error) => {
			if (error.response?.data?.errors) {
				setErrors(
					error.response.data.errors.reduce((acc, err) => {
						acc[err.path] = err.msg;
						return acc;
					}, {}),
				);
			} else {
				setErrors({
					general: error.response?.data?.error || "Failed to update settings",
				});
			}
		},
	});

	// Normalize update interval to safe presets
	const normalizeInterval = (minutes) => {
		let m = parseInt(minutes, 10);
		if (Number.isNaN(m)) return 60;
		if (m < 5) m = 5;
		if (m > 1440) m = 1440;
		// If less than 60 minutes, keep within 5-59 and step of 5
		if (m < 60) {
			return Math.min(59, Math.max(5, Math.round(m / 5) * 5));
		}
		// 60 or more: only allow exact hour multiples (60, 120, 180, 360, 720, 1440)
		const allowed = [60, 120, 180, 360, 720, 1440];
		// Snap to nearest allowed value
		let nearest = allowed[0];
		let bestDiff = Math.abs(m - nearest);
		for (const a of allowed) {
			const d = Math.abs(m - a);
			if (d < bestDiff) {
				bestDiff = d;
				nearest = a;
			}
		}
		return nearest;
	};

	const handleInputChange = (field, value) => {
		setFormData((prev) => {
			const newData = {
				...prev,
				[field]: field === "updateInterval" ? normalizeInterval(value) : value,
			};
			return newData;
		});
		setIsDirty(true);
		if (errors[field]) {
			setErrors((prev) => ({ ...prev, [field]: null }));
		}
	};

	const validateForm = () => {
		const newErrors = {};

		if (
			!formData.updateInterval ||
			formData.updateInterval < 5 ||
			formData.updateInterval > 1440
		) {
			newErrors.updateInterval =
				"Update interval must be between 5 and 1440 minutes";
		}

		setErrors(newErrors);
		return Object.keys(newErrors).length === 0;
	};

	const handleSave = () => {
		if (validateForm()) {
			updateSettingsMutation.mutate(formData);
		}
	};

	if (isLoading) {
		return (
			<div className="flex items-center justify-center h-64">
				<div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600"></div>
			</div>
		);
	}

	if (error) {
		return (
			<div className="bg-red-50 dark:bg-red-900 border border-red-200 dark:border-red-700 rounded-md p-4">
				<div className="flex">
					<AlertCircle className="h-5 w-5 text-red-400 dark:text-red-300" />
					<div className="ml-3">
						<h3 className="text-sm font-medium text-red-800 dark:text-red-200">
							Error loading settings
						</h3>
						<p className="mt-1 text-sm text-red-700 dark:text-red-300">
							{error.response?.data?.error || "Failed to load settings"}
						</p>
					</div>
				</div>
			</div>
		);
	}

	return (
		<div className="space-y-6">
			{/* Toast Notification */}
			{toast && (
				<div
					className={`fixed top-20 right-4 z-[100] max-w-md rounded-lg shadow-lg border-2 p-4 flex items-start space-x-3 animate-in slide-in-from-top-5 ${
						toast.type === "success"
							? "bg-green-50 dark:bg-green-900/90 border-green-500 dark:border-green-600"
							: "bg-red-50 dark:bg-red-900/90 border-red-500 dark:border-red-600"
					}`}
				>
					<div
						className={`flex-shrink-0 rounded-full p-1 ${
							toast.type === "success"
								? "bg-green-100 dark:bg-green-800"
								: "bg-red-100 dark:bg-red-800"
						}`}
					>
						{toast.type === "success" ? (
							<CheckCircle className="h-5 w-5 text-green-600 dark:text-green-400" />
						) : (
							<AlertCircle className="h-5 w-5 text-red-600 dark:text-red-400" />
						)}
					</div>
					<div className="flex-1">
						<p
							className={`text-sm font-medium ${
								toast.type === "success"
									? "text-green-800 dark:text-green-100"
									: "text-red-800 dark:text-red-100"
							}`}
						>
							{toast.message}
						</p>
					</div>
					<button
						type="button"
						onClick={() => setToast(null)}
						className={`flex-shrink-0 rounded-lg p-1 transition-colors ${
							toast.type === "success"
								? "hover:bg-green-100 dark:hover:bg-green-800 text-green-600 dark:text-green-400"
								: "hover:bg-red-100 dark:hover:bg-red-800 text-red-600 dark:text-red-400"
						}`}
					>
						<X className="h-4 w-4" />
					</button>
				</div>
			)}

			{errors.general && (
				<div className="bg-red-50 dark:bg-red-900 border border-red-200 dark:border-red-700 rounded-md p-4">
					<div className="flex">
						<AlertCircle className="h-5 w-5 text-red-400 dark:text-red-300" />
						<div className="ml-3">
							<p className="text-sm text-red-700 dark:text-red-300">
								{errors.general}
							</p>
						</div>
					</div>
				</div>
			)}

			<form className="space-y-6">
				{/* Update Interval */}
				<div>
					<label
						htmlFor={updateIntervalId}
						className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-2"
					>
						Agent Update Interval (minutes)
					</label>

					{/* Numeric input (concise width) */}
					<div className="flex items-center gap-2">
						<input
							id={updateIntervalId}
							type="number"
							min="5"
							max="1440"
							step="5"
							value={formData.updateInterval}
							onChange={(e) => {
								const val = parseInt(e.target.value, 10);
								if (!Number.isNaN(val)) {
									handleInputChange(
										"updateInterval",
										Math.min(1440, Math.max(5, val)),
									);
								} else {
									handleInputChange("updateInterval", 60);
								}
							}}
							className={`w-28 border rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white ${
								errors.updateInterval
									? "border-red-300 dark:border-red-500"
									: "border-secondary-300 dark:border-secondary-600"
							}`}
							placeholder="60"
						/>
					</div>

					{/* Quick presets */}
					<div className="mt-3 flex flex-wrap items-center gap-2">
						{[5, 10, 15, 30, 45, 60, 120, 180, 360, 720, 1440].map((m) => (
							<button
								key={m}
								type="button"
								onClick={() => handleInputChange("updateInterval", m)}
								className={`px-2 md:px-3 py-1 md:py-1.5 rounded-md text-xs font-medium border ${
									formData.updateInterval === m
										? "bg-primary-600 text-white border-primary-600"
										: "bg-white dark:bg-secondary-700 text-secondary-700 dark:text-secondary-200 border-secondary-300 dark:border-secondary-600 hover:bg-secondary-50 dark:hover:bg-secondary-600"
								}`}
								aria-label={`Set ${m} minutes`}
							>
								{m % 60 === 0 ? `${m / 60}h` : `${m}m`}
							</button>
						))}
					</div>

					{/* Range slider */}
					<div className="mt-4">
						<input
							type="range"
							min="5"
							max="1440"
							step="5"
							value={formData.updateInterval}
							onChange={(e) => {
								const raw = parseInt(e.target.value, 10);
								handleInputChange("updateInterval", normalizeInterval(raw));
							}}
							className="w-full accent-primary-600"
							aria-label="Update interval slider"
						/>
					</div>

					{errors.updateInterval && (
						<p className="mt-1 text-sm text-red-600 dark:text-red-400">
							{errors.updateInterval}
						</p>
					)}

					{/* Helper text */}
					<div className="mt-2 text-sm text-secondary-600 dark:text-secondary-300">
						<span className="font-medium">Effective cadence:</span> {(() => {
							const mins = parseInt(formData.updateInterval, 10) || 60;
							if (mins < 60) return `${mins} minute${mins === 1 ? "" : "s"}`;
							const hrs = Math.floor(mins / 60);
							const rem = mins % 60;
							return `${hrs} hour${hrs === 1 ? "" : "s"}${rem ? ` ${rem} min` : ""}`;
						})()}
					</div>

					<p className="mt-1 text-xs text-secondary-500 dark:text-secondary-400">
						This affects new installations and will update existing ones when
						they next reach out.
					</p>
				</div>

				{/* Auto-Update Setting (Master Toggle) */}
				<div className="flex items-start justify-between gap-4 p-4 bg-secondary-50 dark:bg-secondary-800/50 rounded-lg border border-secondary-200 dark:border-secondary-700">
					<div className="flex-1">
						<div className="flex items-center gap-2">
							<label
								htmlFor={autoUpdateId}
								className="text-sm font-medium text-secondary-900 dark:text-secondary-100"
							>
								Enable Automatic Agent Updates
							</label>
							<span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-primary-100 text-primary-800 dark:bg-primary-900/50 dark:text-primary-300">
								Master
							</span>
						</div>
						<p className="mt-1 text-sm text-secondary-500 dark:text-secondary-400">
							Master switch for all agent auto-updates. Per-host toggles in the
							dashboard control individual agents. Enabling a host toggle will
							automatically enable this master switch.
						</p>
					</div>
					<button
						type="button"
						id={autoUpdateId}
						onClick={() =>
							handleInputChange("autoUpdate", !formData.autoUpdate)
						}
						className={`relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 ${
							formData.autoUpdate
								? "bg-primary-600 dark:bg-primary-500"
								: "bg-secondary-200 dark:bg-secondary-600"
						}`}
					>
						<span
							className={`pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out ${
								formData.autoUpdate ? "translate-x-5" : "translate-x-0"
							}`}
						/>
					</button>
				</div>

				{/* SSL Certificate Setting */}
				<div className="flex items-start justify-between gap-4 p-4 bg-warning-50 dark:bg-warning-900/20 rounded-lg border border-warning-200 dark:border-warning-700">
					<div className="flex-1">
						<div className="flex items-center gap-2">
							<label
								htmlFor={ignoreSslId}
								className="text-sm font-medium text-secondary-900 dark:text-secondary-100"
							>
								Ignore SSL Self-Signed Certificates
							</label>
							<span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-warning-100 text-warning-800 dark:bg-warning-900/50 dark:text-warning-300">
								Security
							</span>
						</div>
						<p className="mt-1 text-sm text-secondary-500 dark:text-secondary-400">
							When enabled, curl commands in agent scripts will use the -k flag
							to ignore SSL certificate validation errors. Use with caution on
							production systems as this reduces security.
						</p>
					</div>
					<button
						type="button"
						id={ignoreSslId}
						onClick={() =>
							handleInputChange(
								"ignoreSslSelfSigned",
								!formData.ignoreSslSelfSigned,
							)
						}
						className={`relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 ${
							formData.ignoreSslSelfSigned
								? "bg-warning-500 dark:bg-warning-600"
								: "bg-secondary-200 dark:bg-secondary-600"
						}`}
					>
						<span
							className={`pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out ${
								formData.ignoreSslSelfSigned ? "translate-x-5" : "translate-x-0"
							}`}
						/>
					</button>
				</div>

				{/* Save Button */}
				<div className="flex justify-end">
					<button
						type="button"
						onClick={handleSave}
						disabled={!isDirty || updateSettingsMutation.isPending}
						className={`inline-flex items-center justify-center px-4 py-2 border border-transparent text-sm font-medium rounded-md shadow-sm text-white w-full sm:w-auto ${
							!isDirty || updateSettingsMutation.isPending
								? "bg-secondary-400 cursor-not-allowed"
								: "bg-primary-600 hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500"
						}`}
					>
						{updateSettingsMutation.isPending ? (
							<>
								<div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white mr-2"></div>
								Saving...
							</>
						) : (
							<>
								<Save className="h-4 w-4 mr-2" />
								Save Settings
							</>
						)}
					</button>
				</div>

				{updateSettingsMutation.isSuccess && (
					<div className="bg-green-50 dark:bg-green-900 border border-green-200 dark:border-green-700 rounded-md p-4">
						<div className="flex">
							<CheckCircle className="h-5 w-5 text-green-400 dark:text-green-300" />
							<div className="ml-3">
								<p className="text-sm text-green-700 dark:text-green-300">
									Settings saved successfully!
								</p>
							</div>
						</div>
					</div>
				)}
			</form>

			{/* Uninstall Instructions */}
			<div className="bg-red-50 dark:bg-red-900 border border-red-200 dark:border-red-700 rounded-md p-4">
				<div className="flex">
					<Shield className="h-5 w-5 text-red-400 dark:text-red-300" />
					<div className="ml-3">
						<h3 className="text-sm font-medium text-red-800 dark:text-red-200">
							Agent Uninstall Command
						</h3>
						<div className="mt-2 text-sm text-red-700 dark:text-red-300">
							<p className="mb-3">
								To completely remove PatchMonEnhanced from a host:
							</p>

							{/* Agent Removal Script - Standard */}
							<div className="mb-3">
								<div className="space-y-2">
									<div className="text-xs font-semibold text-red-700 dark:text-red-300 mb-1">
										Standard Removal (preserves backups):
									</div>
									<div className="flex flex-col sm:flex-row items-stretch sm:items-center gap-2">
										<div className="bg-red-100 dark:bg-red-800 rounded p-2 font-mono text-xs flex-1 break-all overflow-x-auto">
											curl {formData.ignoreSslSelfSigned ? "-sk" : "-s"}{" "}
											{window.location.origin}/api/v1/hosts/remove | sudo sh
										</div>
										<button
											type="button"
											onClick={async () => {
												try {
													const curlFlags = formData.ignoreSslSelfSigned
														? "-sk"
														: "-s";
													await copyToClipboard(
														`curl ${curlFlags} ${window.location.origin}/api/v1/hosts/remove | sudo sh`,
													);
													showToast(
														"Standard removal command copied!",
														"success",
													);
												} catch (err) {
													console.error("Failed to copy:", err);
													showToast("Failed to copy to clipboard", "error");
												}
											}}
											className="px-3 py-2 bg-red-200 dark:bg-red-700 text-red-800 dark:text-red-200 rounded text-xs hover:bg-red-300 dark:hover:bg-red-600 transition-colors flex-shrink-0 whitespace-nowrap"
										>
											Copy
										</button>
									</div>
								</div>
							</div>

							{/* Agent Removal Script - Complete */}
							<div className="mb-3">
								<div className="space-y-2">
									<div className="text-xs font-semibold text-red-700 dark:text-red-300 mb-1">
										Complete Removal (includes backups):
									</div>
									<div className="flex flex-col sm:flex-row items-stretch sm:items-center gap-2">
										<div className="bg-red-100 dark:bg-red-800 rounded p-2 font-mono text-xs flex-1 break-all overflow-x-auto">
											curl {formData.ignoreSslSelfSigned ? "-sk" : "-s"}{" "}
											{window.location.origin}/api/v1/hosts/remove | sudo
											REMOVE_BACKUPS=1 sh
										</div>
										<button
											type="button"
											onClick={async () => {
												try {
													const curlFlags = formData.ignoreSslSelfSigned
														? "-sk"
														: "-s";
													await copyToClipboard(
														`curl ${curlFlags} ${window.location.origin}/api/v1/hosts/remove | sudo REMOVE_BACKUPS=1 sh`,
													);
													showToast(
														"Complete removal command copied!",
														"success",
													);
												} catch (err) {
													console.error("Failed to copy:", err);
													showToast("Failed to copy to clipboard", "error");
												}
											}}
											className="px-3 py-2 bg-red-200 dark:bg-red-700 text-red-800 dark:text-red-200 rounded text-xs hover:bg-red-300 dark:hover:bg-red-600 transition-colors flex-shrink-0 whitespace-nowrap"
										>
											Copy
										</button>
									</div>
									<div className="text-xs text-red-600 dark:text-red-400">
										This removes: binaries, systemd/OpenRC services,
										configuration files, logs, crontab entries, and backup files
									</div>
								</div>
							</div>

							<p className="mt-2 text-xs text-red-700 dark:text-red-400">
								⚠️ Standard removal preserves backup files for safety. Use
								complete removal to delete everything.
							</p>
						</div>
					</div>
				</div>
			</div>
		</div>
	);
};

export default AgentUpdatesTab;
