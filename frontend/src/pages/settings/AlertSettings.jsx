import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, RefreshCw } from "lucide-react";
import { useEffect, useState } from "react";
import { adminUsersAPI, alertsAPI, settingsAPI } from "../../utils/api";

const AlertSettings = () => {
	const queryClient = useQueryClient();
	const [saveMessage, setSaveMessage] = useState(null);

	// Fetch settings for master switch
	const { data: settings } = useQuery({
		queryKey: ["settings"],
		queryFn: async () => {
			const response = await settingsAPI.get();
			return response.data;
		},
	});

	// Fetch alert configuration
	const {
		data: alertConfigs,
		isLoading,
		error,
		refetch,
	} = useQuery({
		queryKey: ["alert-config"],
		queryFn: async () => {
			const response = await alertsAPI.getAlertConfig();
			return response.data.data || [];
		},
	});

	// Fetch users for auto-assignment dropdown (use public endpoint that works for all authenticated users)
	const { data: usersData } = useQuery({
		queryKey: ["users", "for-assignment"],
		queryFn: async () => {
			try {
				// Try public assignment endpoint first (available to all authenticated users)
				const response = await adminUsersAPI.listForAssignment();
				return response.data.data || [];
			} catch (error) {
				// Fallback to admin endpoint if user has permissions
				if (error.response?.status === 403 || error.response?.status === 401) {
					try {
						const response = await adminUsersAPI.list();
						return response.data.data || [];
					} catch (_e) {
						// If both fail, return empty array
						return [];
					}
				}
				// For other errors, return empty array
				return [];
			}
		},
	});

	// Update settings mutation (for master switch)
	const updateSettingsMutation = useMutation({
		mutationFn: async (data) => {
			return settingsAPI.update(data);
		},
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ["settings"] });
			setSaveMessage({ type: "success", text: "Settings saved successfully" });
			setTimeout(() => setSaveMessage(null), 3000);
		},
		onError: (error) => {
			setSaveMessage({
				type: "error",
				text: error.response?.data?.error || "Failed to save settings",
			});
			setTimeout(() => setSaveMessage(null), 5000);
		},
	});

	// Update mutation
	const updateMutation = useMutation({
		mutationFn: async ({ alertType, config }) => {
			return alertsAPI.updateAlertConfig(alertType, config);
		},
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ["alert-config"] });
			setSaveMessage({ type: "success", text: "Settings saved successfully" });
			setTimeout(() => setSaveMessage(null), 3000);
		},
		onError: (error) => {
			setSaveMessage({
				type: "error",
				text: error.response?.data?.error || "Failed to save settings",
			});
			setTimeout(() => setSaveMessage(null), 5000);
		},
	});

	// Bulk update mutation
	const _bulkUpdateMutation = useMutation({
		mutationFn: async (configs) => {
			return alertsAPI.bulkUpdateAlertConfig(configs);
		},
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ["alert-config"] });
			setSaveMessage({ type: "success", text: "Settings saved successfully" });
			setTimeout(() => setSaveMessage(null), 3000);
		},
		onError: (error) => {
			setSaveMessage({
				type: "error",
				text: error.response?.data?.error || "Failed to save settings",
			});
			setTimeout(() => setSaveMessage(null), 5000);
		},
	});

	// Handle individual config update
	const handleConfigUpdate = async (alertType, field, value) => {
		const config = alertConfigs.find((c) => c.alert_type === alertType);
		if (!config) return;

		// Convert empty strings to null for optional fields
		let cleanedValue = value;
		if (
			value === "" &&
			(field === "auto_assign_user_id" ||
				field === "retention_days" ||
				field === "auto_resolve_after_days" ||
				field === "escalation_after_hours")
		) {
			cleanedValue = null;
		}

		// Build clean config object with only updatable fields
		const updatedConfig = {
			is_enabled: config.is_enabled,
			default_severity: config.default_severity,
			auto_assign_enabled: config.auto_assign_enabled,
			auto_assign_user_id: config.auto_assign_user_id || null,
			auto_assign_rule: config.auto_assign_rule || null,
			auto_assign_conditions: config.auto_assign_conditions || null,
			retention_days: config.retention_days || null,
			auto_resolve_after_days: config.auto_resolve_after_days || null,
			cleanup_resolved_only: config.cleanup_resolved_only,
			notification_enabled: config.notification_enabled,
			escalation_enabled: config.escalation_enabled,
			escalation_after_hours: config.escalation_after_hours || null,
			metadata: config.metadata || null,
		};

		// Update the specific field that was changed
		updatedConfig[field] = cleanedValue;

		await updateMutation.mutateAsync({
			alertType,
			config: updatedConfig,
		});
	};

	if (isLoading) {
		return (
			<div className="space-y-6">
				<div className="text-center py-8">
					<div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600 mx-auto"></div>
					<p className="mt-2 text-sm text-secondary-500">
						Loading alert settings...
					</p>
				</div>
			</div>
		);
	}

	if (error) {
		return (
			<div className="space-y-6">
				<div className="bg-danger-50 border border-danger-200 rounded-md p-4">
					<div className="flex">
						<AlertTriangle className="h-5 w-5 text-danger-400" />
						<div className="ml-3">
							<h3 className="text-sm font-medium text-danger-800">
								Error loading alert settings
							</h3>
							<p className="text-sm text-danger-700 mt-1">
								{error.message || "Failed to load alert settings"}
							</p>
							<button
								type="button"
								onClick={() => refetch()}
								className="mt-2 btn-danger text-xs"
							>
								Try again
							</button>
						</div>
					</div>
				</div>
			</div>
		);
	}

	return (
		<div className="space-y-6">
			{/* Header */}
			<div className="flex items-center justify-between">
				<div>
					<h1 className="text-2xl font-bold text-secondary-900 dark:text-white">
						Alert Settings
					</h1>
					<p className="mt-1 text-sm text-secondary-600 dark:text-secondary-400">
						Configure alert types, severities, auto-assignment, and retention
						policies
					</p>
				</div>
				<div className="flex items-center gap-3">
					<button
						type="button"
						onClick={() => refetch()}
						className="btn-outline flex items-center gap-2"
					>
						<RefreshCw className="h-4 w-4" />
						Refresh
					</button>
				</div>
			</div>

			{/* Save Message */}
			{saveMessage && (
				<div
					className={`rounded-md p-4 ${
						saveMessage.type === "success"
							? "bg-green-50 border border-green-200 text-green-800"
							: "bg-danger-50 border border-danger-200 text-danger-800"
					}`}
				>
					{saveMessage.text}
				</div>
			)}

			{/* Master Switch */}
			<div className="card p-6">
				<div className="flex items-center justify-between mb-4">
					<div>
						<h2 className="text-lg font-semibold text-secondary-900 dark:text-white">
							Alerts System Master Switch
						</h2>
						<p className="text-sm text-secondary-600 dark:text-secondary-400 mt-1">
							Enable or disable the entire alerts system. When disabled, no
							alerts will be created, alert queues will be skipped, and the
							Reporting page will be hidden.
						</p>
					</div>
					<div className="flex items-center gap-3">
						<button
							type="button"
							onClick={() => {
								updateSettingsMutation.mutate({
									alerts_enabled: !(settings?.alerts_enabled !== false),
								});
							}}
							disabled={updateSettingsMutation.isPending}
							className={`relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 ${
								settings?.alerts_enabled !== false
									? "bg-primary-600 dark:bg-primary-500"
									: "bg-secondary-300 dark:bg-secondary-600"
							} disabled:opacity-50 disabled:cursor-not-allowed`}
						>
							<span
								className={`pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out ${
									settings?.alerts_enabled !== false
										? "translate-x-4"
										: "translate-x-0"
								}`}
							/>
						</button>
						<span className="text-sm font-medium text-secondary-700 dark:text-secondary-300">
							{settings?.alerts_enabled !== false ? "Enabled" : "Disabled"}
						</span>
					</div>
				</div>
				{settings?.alerts_enabled === false && (
					<div className="mt-4 p-4 bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800 rounded-md">
						<div className="flex">
							<AlertTriangle className="h-5 w-5 text-yellow-600 dark:text-yellow-400" />
							<div className="ml-3">
								<h3 className="text-sm font-medium text-yellow-800 dark:text-yellow-200">
									Alerts System Disabled
								</h3>
								<p className="text-sm text-yellow-700 dark:text-yellow-300 mt-1">
									All alert-related services are currently disabled. No alerts
									will be created, and alert cleanup jobs will be skipped.
								</p>
							</div>
						</div>
					</div>
				)}
			</div>

			{/* Alert Type Configurations Table */}
			{settings?.alerts_enabled !== false && (
				<div className="card overflow-hidden">
					<div className="px-6 py-4 border-b border-secondary-200 dark:border-secondary-600">
						<h2 className="text-lg font-semibold text-secondary-900 dark:text-white">
							Alert Type Configurations
						</h2>
						<p className="text-sm text-secondary-600 dark:text-secondary-400 mt-1">
							Configure settings for each alert type inline
						</p>
					</div>
					<div className="overflow-x-auto">
						<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-700">
							<thead className="bg-secondary-50 dark:bg-secondary-800">
								<tr>
									<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
										Alert Type
									</th>
									<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
										Enabled
									</th>
									<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
										Default Severity
									</th>
									<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
										Auto-Assign
									</th>
									<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
										Retention Days
									</th>
									<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
										Auto-Resolve Days
									</th>
									<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
										Notifications
									</th>
								</tr>
							</thead>
							<tbody className="bg-white dark:bg-secondary-900 divide-y divide-secondary-200 dark:divide-secondary-700">
								{alertConfigs.map((config) => (
									<AlertTypeTableRow
										key={config.alert_type}
										config={config}
										onUpdate={handleConfigUpdate}
										isSaving={updateMutation.isPending}
										usersData={usersData}
									/>
								))}
							</tbody>
						</table>
					</div>
				</div>
			)}

			{settings?.alerts_enabled === false && (
				<div className="card p-6 text-center">
					<AlertTriangle className="h-12 w-12 mx-auto text-secondary-400" />
					<h3 className="mt-2 text-sm font-medium text-secondary-900 dark:text-white">
						Alert configurations are hidden
					</h3>
					<p className="mt-1 text-sm text-secondary-500">
						Enable the alerts system above to configure individual alert types.
					</p>
				</div>
			)}

			{/* Cleanup Section */}
			{settings?.alerts_enabled !== false && (
				<div className="card p-6">
					<h2 className="text-lg font-semibold text-secondary-900 dark:text-white mb-4">
						Alert Cleanup
					</h2>
					<p className="text-sm text-secondary-600 dark:text-secondary-400 mb-4">
						Manage alert retention and cleanup policies
					</p>
					<CleanupSection />
				</div>
			)}
		</div>
	);
};

// Alert Type Table Row Component
const AlertTypeTableRow = ({ config, onUpdate, isSaving, usersData }) => {
	const [localConfig, setLocalConfig] = useState(config);

	// Update local state when config prop changes
	useEffect(() => {
		setLocalConfig(config);
	}, [config]);

	const handleFieldChange = (field, value) => {
		const updated = { ...localConfig, [field]: value };
		setLocalConfig(updated);
		onUpdate(config.alert_type, field, value);
	};

	const formatAlertType = (type) => {
		return type.replace("_", " ").replace(/\b\w/g, (l) => l.toUpperCase());
	};

	return (
		<tr className="hover:bg-secondary-50 dark:hover:bg-secondary-800">
			{/* Alert Type */}
			<td className="px-6 py-4 whitespace-nowrap">
				<div className="text-sm font-medium text-secondary-900 dark:text-white">
					{formatAlertType(config.alert_type)}
				</div>
			</td>

			{/* Enabled */}
			<td className="px-6 py-4 whitespace-nowrap">
				<button
					type="button"
					onClick={() =>
						handleFieldChange("is_enabled", !localConfig.is_enabled)
					}
					disabled={isSaving}
					className={`relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 ${
						localConfig.is_enabled
							? "bg-primary-600 dark:bg-primary-500"
							: "bg-secondary-300 dark:bg-secondary-600"
					} disabled:opacity-50 disabled:cursor-not-allowed`}
				>
					<span
						className={`pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out ${
							localConfig.is_enabled ? "translate-x-4" : "translate-x-0"
						}`}
					/>
				</button>
			</td>

			{/* Default Severity */}
			<td className="px-6 py-4 whitespace-nowrap">
				<select
					value={localConfig.default_severity}
					onChange={(e) =>
						handleFieldChange("default_severity", e.target.value)
					}
					disabled={isSaving || !localConfig.is_enabled}
					className="px-2 py-1 text-sm border border-secondary-300 dark:border-secondary-600 rounded-md bg-white dark:bg-secondary-800 text-secondary-900 dark:text-white disabled:opacity-50 disabled:cursor-not-allowed"
				>
					<option value="informational">Informational</option>
					<option value="warning">Warning</option>
					<option value="error">Error</option>
					<option value="critical">Critical</option>
				</select>
			</td>

			{/* Auto-Assign */}
			<td className="px-6 py-4 whitespace-nowrap">
				<div className="flex flex-col gap-2">
					<button
						type="button"
						onClick={() =>
							handleFieldChange(
								"auto_assign_enabled",
								!localConfig.auto_assign_enabled,
							)
						}
						disabled={isSaving || !localConfig.is_enabled}
						className={`relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 ${
							localConfig.auto_assign_enabled
								? "bg-primary-600 dark:bg-primary-500"
								: "bg-secondary-300 dark:bg-secondary-600"
						} disabled:opacity-50 disabled:cursor-not-allowed`}
					>
						<span
							className={`pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out ${
								localConfig.auto_assign_enabled
									? "translate-x-4"
									: "translate-x-0"
							}`}
						/>
					</button>
					{localConfig.auto_assign_enabled && localConfig.is_enabled && (
						<select
							value={localConfig.auto_assign_user_id || ""}
							onChange={(e) =>
								handleFieldChange("auto_assign_user_id", e.target.value || null)
							}
							disabled={isSaving}
							className="px-2 py-1 text-xs border border-secondary-300 dark:border-secondary-600 rounded-md bg-white dark:bg-secondary-800 text-secondary-900 dark:text-white disabled:opacity-50 disabled:cursor-not-allowed min-w-[120px]"
						>
							<option value="">Select user...</option>
							{usersData?.map((u) => (
								<option key={u.id} value={u.id}>
									{u.username || u.email}
								</option>
							))}
						</select>
					)}
				</div>
			</td>

			{/* Retention Days */}
			<td className="px-6 py-4 whitespace-nowrap">
				<input
					type="number"
					value={localConfig.retention_days || ""}
					onChange={(e) =>
						handleFieldChange(
							"retention_days",
							e.target.value ? parseInt(e.target.value, 10) : null,
						)
					}
					disabled={isSaving || !localConfig.is_enabled}
					placeholder="—"
					className="w-20 px-2 py-1 text-sm border border-secondary-300 dark:border-secondary-600 rounded-md bg-white dark:bg-secondary-800 text-secondary-900 dark:text-white disabled:opacity-50 disabled:cursor-not-allowed"
				/>
			</td>

			{/* Auto-Resolve After Days */}
			<td className="px-6 py-4 whitespace-nowrap">
				<input
					type="number"
					value={localConfig.auto_resolve_after_days || ""}
					onChange={(e) =>
						handleFieldChange(
							"auto_resolve_after_days",
							e.target.value ? parseInt(e.target.value, 10) : null,
						)
					}
					disabled={isSaving || !localConfig.is_enabled}
					placeholder="—"
					className="w-20 px-2 py-1 text-sm border border-secondary-300 dark:border-secondary-600 rounded-md bg-white dark:bg-secondary-800 text-secondary-900 dark:text-white disabled:opacity-50 disabled:cursor-not-allowed"
				/>
			</td>

			{/* Notifications */}
			<td className="px-6 py-4 whitespace-nowrap">
				<button
					type="button"
					onClick={() =>
						handleFieldChange(
							"notification_enabled",
							!localConfig.notification_enabled,
						)
					}
					disabled={isSaving || !localConfig.is_enabled}
					className={`relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 ${
						localConfig.notification_enabled
							? "bg-primary-600 dark:bg-primary-500"
							: "bg-secondary-300 dark:bg-secondary-600"
					} disabled:opacity-50 disabled:cursor-not-allowed`}
				>
					<span
						className={`pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out ${
							localConfig.notification_enabled
								? "translate-x-4"
								: "translate-x-0"
						}`}
					/>
				</button>
			</td>
		</tr>
	);
};

// Cleanup Section Component
const CleanupSection = () => {
	const [previewLoading, setPreviewLoading] = useState(false);
	const [previewData, setPreviewData] = useState(null);

	const handlePreview = async () => {
		setPreviewLoading(true);
		try {
			const response = await alertsAPI.previewCleanup();
			setPreviewData(response.data.data);
		} catch (error) {
			console.error("Failed to preview cleanup:", error);
		} finally {
			setPreviewLoading(false);
		}
	};

	const handleCleanup = async () => {
		if (
			!window.confirm(
				"Are you sure you want to delete these alerts? This action cannot be undone.",
			)
		) {
			return;
		}

		try {
			const response = await alertsAPI.triggerCleanup();
			alert(
				`Cleanup completed: ${response.data.data.deleted_count} alerts deleted`,
			);
			setPreviewData(null);
		} catch (error) {
			alert(
				"Failed to trigger cleanup: " +
					(error.response?.data?.error || error.message),
			);
		}
	};

	return (
		<div className="space-y-4">
			<div className="flex items-center gap-3">
				<button
					type="button"
					onClick={handlePreview}
					disabled={previewLoading}
					className="btn-outline flex items-center gap-2"
				>
					<RefreshCw
						className={`h-4 w-4 ${previewLoading ? "animate-spin" : ""}`}
					/>
					Preview Cleanup
				</button>
				{previewData && previewData.length > 0 && (
					<button
						type="button"
						onClick={handleCleanup}
						className="btn-danger flex items-center gap-2"
					>
						Delete {previewData.length} Alerts
					</button>
				)}
			</div>

			{previewData && (
				<div className="mt-4">
					{previewData.length === 0 ? (
						<p className="text-sm text-secondary-600 dark:text-secondary-400">
							No alerts need to be cleaned up based on current retention
							policies.
						</p>
					) : (
						<div className="bg-secondary-50 dark:bg-secondary-800 rounded-md p-4">
							<p className="text-sm font-medium text-secondary-900 dark:text-white mb-2">
								{previewData.length} alert(s) would be deleted:
							</p>
							<ul className="list-disc list-inside text-sm text-secondary-600 dark:text-secondary-400 space-y-1">
								{previewData.slice(0, 10).map((alert) => (
									<li key={alert.id}>
										{alert.type} - Created{" "}
										{new Date(alert.created_at).toLocaleDateString()}
									</li>
								))}
								{previewData.length > 10 && (
									<li>... and {previewData.length - 10} more</li>
								)}
							</ul>
						</div>
					)}
				</div>
			)}
		</div>
	);
};

export default AlertSettings;
