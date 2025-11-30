import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertCircle, CheckCircle, Save, Shield, X } from "lucide-react";
import { useEffect, useId, useState } from "react";
import { permissionsAPI, settingsAPI } from "../../utils/api";

const AgentUpdatesTab = () => {
	const updateIntervalId = useId();
	const autoUpdateId = useId();
	const signupEnabledId = useId();
	const defaultRoleId = useId();
	const ignoreSslId = useId();
	const [formData, setFormData] = useState({
		updateInterval: 60,
		autoUpdate: false,
		signupEnabled: false,
		defaultUserRole: "user",
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

	// Fetch available roles for default user role dropdown
	const { data: roles, isLoading: rolesLoading } = useQuery({
		queryKey: ["rolePermissions"],
		queryFn: () => permissionsAPI.getRoles().then((res) => res.data),
	});

	// Update form data when settings are loaded
	useEffect(() => {
		if (settings) {
			const newFormData = {
				updateInterval: settings.update_interval || 60,
				autoUpdate: settings.auto_update || false,
				signupEnabled: settings.signup_enabled === true,
				defaultUserRole: settings.default_user_role || "user",
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
					className={`fixed top-4 right-4 z-50 max-w-md rounded-lg shadow-lg border-2 p-4 flex items-start space-x-3 animate-in slide-in-from-top-5 ${
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

				{/* Auto-Update Setting */}
				<div>
					<label className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-2">
						<div className="flex items-center gap-2">
							<input
								id={autoUpdateId}
								type="checkbox"
								checked={formData.autoUpdate}
								onChange={(e) =>
									handleInputChange("autoUpdate", e.target.checked)
								}
								className="rounded border-secondary-300 text-primary-600 shadow-sm focus:border-primary-300 focus:ring focus:ring-primary-200 focus:ring-opacity-50"
							/>
							<label htmlFor={autoUpdateId}>
								Enable Automatic Agent Updates
							</label>
						</div>
					</label>
					<p className="mt-1 text-sm text-secondary-500 dark:text-secondary-400">
						When enabled, agents will automatically update themselves when a
						newer version is available during their regular update cycle.
					</p>
				</div>

				{/* SSL Certificate Setting */}
				<div>
					<label className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-2">
						<div className="flex items-center gap-2">
							<input
								id={ignoreSslId}
								type="checkbox"
								checked={formData.ignoreSslSelfSigned}
								onChange={(e) =>
									handleInputChange("ignoreSslSelfSigned", e.target.checked)
								}
								className="rounded border-secondary-300 text-primary-600 shadow-sm focus:border-primary-300 focus:ring focus:ring-primary-200 focus:ring-opacity-50"
							/>
							<label htmlFor={ignoreSslId}>
								Ignore SSL Self-Signed Certificates
							</label>
						</div>
					</label>
					<p className="mt-1 text-sm text-secondary-500 dark:text-secondary-400">
						When enabled, curl commands in agent scripts will use the -k flag to
						ignore SSL certificate validation errors. Use with caution on
						production systems as this reduces security.
					</p>
				</div>

				{/* User Signup Setting */}
				<div>
					<label className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-2">
						<div className="flex items-center gap-2">
							<input
								id={signupEnabledId}
								type="checkbox"
								checked={formData.signupEnabled}
								onChange={(e) =>
									handleInputChange("signupEnabled", e.target.checked)
								}
								className="rounded border-secondary-300 text-primary-600 shadow-sm focus:border-primary-300 focus:ring focus:ring-primary-200 focus:ring-opacity-50"
							/>
							<label htmlFor={signupEnabledId}>
								Enable User Self-Registration
							</label>
						</div>
					</label>

					{/* Default User Role Dropdown */}
					{formData.signupEnabled && (
						<div className="mt-3 ml-6">
							<label
								htmlFor={defaultRoleId}
								className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-2"
							>
								Default Role for New Users
							</label>
							<select
								id={defaultRoleId}
								value={formData.defaultUserRole}
								onChange={(e) =>
									handleInputChange("defaultUserRole", e.target.value)
								}
								className="w-full max-w-xs border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
								disabled={rolesLoading}
							>
								{rolesLoading ? (
									<option>Loading roles...</option>
								) : roles && Array.isArray(roles) ? (
									roles.map((role) => (
										<option key={role.role} value={role.role}>
											{role.role.charAt(0).toUpperCase() + role.role.slice(1)}
										</option>
									))
								) : (
									<option value="user">User</option>
								)}
							</select>
							<p className="mt-1 text-xs text-secondary-500 dark:text-secondary-400">
								New users will be assigned this role when they register.
							</p>
						</div>
					)}

					<p className="mt-1 text-sm text-secondary-500 dark:text-secondary-400">
						When enabled, users can create their own accounts through the signup
						page. When disabled, only administrators can create user accounts.
					</p>
				</div>

				{/* Security Notice */}
				<div className="bg-blue-50 dark:bg-blue-900 border border-blue-200 dark:border-blue-700 rounded-md p-4">
					<div className="flex">
						<Shield className="h-5 w-5 text-blue-400 dark:text-blue-300" />
						<div className="ml-3">
							<h3 className="text-sm font-medium text-blue-800 dark:text-blue-200">
								Security Notice
							</h3>
							<p className="mt-1 text-sm text-blue-700 dark:text-blue-300">
								When enabling user self-registration, exercise caution on
								internal networks. Consider restricting access to trusted
								networks only and ensure proper role assignments to prevent
								unauthorized access to sensitive systems.
							</p>
						</div>
					</div>
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
							<p className="mb-3">To completely remove PatchMon from a host:</p>

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
