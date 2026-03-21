import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
	AlertTriangle,
	Check,
	Loader2,
	RefreshCw,
	Save,
	Trash2,
	X,
} from "lucide-react";
import { useEffect, useState } from "react";
import { useToast } from "../../contexts/ToastContext";
import { adminUsersAPI, alertsAPI, settingsAPI } from "../../utils/api";

const TH =
	"px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider";
const TD =
	"px-4 py-2 text-sm text-secondary-900 dark:text-white whitespace-nowrap";

const SEVERITIES = [
	{
		value: "informational",
		label: "Info",
		color: "bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200",
	},
	{
		value: "warning",
		label: "Warning",
		color:
			"bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200",
	},
	{
		value: "error",
		label: "Error",
		color:
			"bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200",
	},
	{
		value: "critical",
		label: "Critical",
		color: "bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200",
	},
];

const SELECT_SM =
	"px-2 py-1 text-sm border border-secondary-300 dark:border-secondary-600 rounded-md bg-white dark:bg-secondary-900 text-secondary-900 dark:text-white disabled:opacity-50";
const INPUT_SM =
	"w-20 px-2 py-1 text-sm border border-secondary-300 dark:border-secondary-600 rounded-md bg-white dark:bg-secondary-900 text-secondary-900 dark:text-white disabled:opacity-50";

const configsEqual = (a, b) => {
	if (!a || !b || a.alert_type !== b.alert_type) return false;
	const fields = [
		"is_enabled",
		"default_severity",
		"auto_assign_enabled",
		"auto_assign_user_id",
		"auto_assign_rule",
		"retention_days",
		"auto_resolve_after_days",
		"cleanup_resolved_only",
		"notification_enabled",
		"escalation_enabled",
		"escalation_after_hours",
		"alert_delay_seconds",
	];
	return fields.every((f) => {
		const va = a[f];
		const vb = b[f];
		if (va == null && vb == null) return true;
		if (va == null || vb == null) return false;
		return String(va) === String(vb);
	});
};

const formatAlertType = (type) =>
	type.replace(/_/g, " ").replace(/\b\w/g, (l) => l.toUpperCase());

const Toggle = ({ checked, onChange, disabled }) => (
	<button
		type="button"
		onClick={() => onChange(!checked)}
		disabled={disabled}
		className={`relative inline-flex h-5 w-9 items-center rounded-md transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 ${
			checked
				? "bg-primary-600 dark:bg-primary-500"
				: "bg-secondary-200 dark:bg-secondary-600"
		} disabled:opacity-50 disabled:cursor-not-allowed`}
	>
		<span
			className={`inline-block h-3 w-3 transform rounded-md bg-white transition-transform ${checked ? "translate-x-5" : "translate-x-1"}`}
		/>
	</button>
);

const AlertSettings = () => {
	const queryClient = useQueryClient();
	const toast = useToast();
	const [localConfigs, setLocalConfigs] = useState(null);

	const { data: settings } = useQuery({
		queryKey: ["settings"],
		queryFn: async () => (await settingsAPI.get()).data,
	});

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

	const { data: usersData } = useQuery({
		queryKey: ["users", "for-assignment"],
		queryFn: async () => {
			try {
				return (await adminUsersAPI.listForAssignment()).data.data || [];
			} catch {
				try {
					return (await adminUsersAPI.list()).data.data || [];
				} catch {
					return [];
				}
			}
		},
	});

	const updateSettingsMutation = useMutation({
		mutationFn: (data) => settingsAPI.update(data),
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ["settings"] });
			toast.success("Settings saved");
		},
		onError: (err) =>
			toast.error(err.response?.data?.error || "Failed to save"),
	});

	const bulkUpdateMutation = useMutation({
		mutationFn: (configs) => alertsAPI.bulkUpdateAlertConfig(configs),
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ["alert-config"] });
			toast.success("Settings applied");
		},
		onError: (err) =>
			toast.error(err.response?.data?.error || "Failed to apply"),
	});

	useEffect(() => {
		if (alertConfigs && Array.isArray(alertConfigs)) {
			setLocalConfigs(alertConfigs.map((c) => ({ ...c })));
		}
	}, [alertConfigs]);

	const isDirty =
		localConfigs &&
		alertConfigs &&
		(localConfigs.length !== alertConfigs.length ||
			localConfigs.some((lc, i) => !configsEqual(lc, alertConfigs[i])));

	useEffect(() => {
		if (!isDirty) return;
		const handler = (e) => e.preventDefault();
		window.addEventListener("beforeunload", handler);
		return () => window.removeEventListener("beforeunload", handler);
	}, [isDirty]);

	const handleFieldChange = (alertType, field, value) => {
		let v = value;
		if (
			value === "" &&
			[
				"auto_assign_user_id",
				"retention_days",
				"auto_resolve_after_days",
				"escalation_after_hours",
				"alert_delay_seconds",
			].includes(field)
		) {
			v = null;
		}
		setLocalConfigs(
			(prev) =>
				prev?.map((c) =>
					c.alert_type === alertType ? { ...c, [field]: v } : c,
				) ?? prev,
		);
	};

	const handleApply = async () => {
		if (!localConfigs?.length || !isDirty) return;
		await bulkUpdateMutation.mutateAsync(
			localConfigs.map((c) => ({
				alert_type: c.alert_type,
				is_enabled: c.is_enabled,
				default_severity: c.default_severity,
				auto_assign_enabled: c.auto_assign_enabled,
				auto_assign_user_id: c.auto_assign_user_id || null,
				auto_assign_rule: c.auto_assign_rule || null,
				auto_assign_conditions: c.auto_assign_conditions || null,
				retention_days: c.retention_days ?? null,
				auto_resolve_after_days: c.auto_resolve_after_days ?? null,
				cleanup_resolved_only: c.cleanup_resolved_only,
				notification_enabled: c.notification_enabled,
				escalation_enabled: c.escalation_enabled,
				escalation_after_hours: c.escalation_after_hours ?? null,
				alert_delay_seconds: c.alert_delay_seconds ?? null,
				metadata: c.metadata || null,
			})),
		);
	};

	const handleDiscard = () => {
		if (alertConfigs && Array.isArray(alertConfigs)) {
			setLocalConfigs(alertConfigs.map((c) => ({ ...c })));
		}
		toast.info("Changes discarded");
	};

	if (isLoading) {
		return (
			<div className="flex justify-center py-12">
				<Loader2 className="h-6 w-6 animate-spin text-secondary-400" />
			</div>
		);
	}

	if (error) {
		return (
			<div className="card p-6">
				<div className="flex items-start gap-3">
					<AlertTriangle className="h-5 w-5 text-danger-500 mt-0.5" />
					<div>
						<p className="text-sm font-medium text-danger-800 dark:text-danger-200">
							Failed to load alert settings
						</p>
						<button
							type="button"
							onClick={() => refetch()}
							className="mt-2 btn-outline text-xs"
						>
							Try again
						</button>
					</div>
				</div>
			</div>
		);
	}

	const alertsEnabled = settings?.alerts_enabled !== false;
	const configs = localConfigs ?? alertConfigs ?? [];

	return (
		<div className="space-y-6">
			{/* Master Switch */}
			<div className="card p-4 md:p-6">
				<div className="flex items-center justify-between">
					<div>
						<h2 className="text-lg font-semibold text-secondary-900 dark:text-white">
							Alerts system
						</h2>
						<p className="text-sm text-secondary-600 dark:text-white mt-1">
							Master switch for the entire alerts system
						</p>
					</div>
					<div className="flex items-center gap-3">
						<Toggle
							checked={alertsEnabled}
							onChange={() =>
								updateSettingsMutation.mutate({
									alerts_enabled: !alertsEnabled,
								})
							}
							disabled={updateSettingsMutation.isPending}
						/>
						<span className="text-sm font-medium text-secondary-700 dark:text-white">
							{alertsEnabled ? "Enabled" : "Disabled"}
						</span>
					</div>
				</div>
				{!alertsEnabled && (
					<div className="mt-4 p-3 bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800 rounded-md flex items-start gap-2">
						<AlertTriangle className="h-4 w-4 text-yellow-600 dark:text-yellow-400 mt-0.5 shrink-0" />
						<p className="text-sm text-yellow-700 dark:text-yellow-300">
							All alert services are disabled. No alerts will be created.
						</p>
					</div>
				)}
			</div>

			{/* Unsaved changes bar */}
			{isDirty && alertsEnabled && (
				<div className="card p-3 flex items-center justify-between bg-amber-50 dark:bg-amber-900/20 border-amber-200 dark:border-amber-800">
					<p className="text-sm text-amber-800 dark:text-amber-200">
						You have unsaved changes
					</p>
					<div className="flex items-center gap-2">
						<button
							type="button"
							onClick={handleDiscard}
							disabled={bulkUpdateMutation.isPending}
							className="btn-outline flex items-center gap-1 text-sm"
						>
							<X className="h-3.5 w-3.5" /> Discard
						</button>
						<button
							type="button"
							onClick={handleApply}
							disabled={bulkUpdateMutation.isPending}
							className="btn-primary flex items-center gap-1 text-sm"
						>
							{bulkUpdateMutation.isPending ? (
								<Loader2 className="h-3.5 w-3.5 animate-spin" />
							) : (
								<Save className="h-3.5 w-3.5" />
							)}
							Apply
						</button>
					</div>
				</div>
			)}

			{/* Alert Type Table */}
			{alertsEnabled && configs.length > 0 && (
				<div className="card p-4 md:p-6 space-y-4">
					<div className="flex items-center justify-between">
						<h2 className="text-lg font-semibold text-secondary-900 dark:text-white">
							Alert type configurations
						</h2>
						<button
							type="button"
							onClick={() => refetch()}
							className="btn-outline flex items-center gap-1 text-sm"
						>
							<RefreshCw className="h-3.5 w-3.5" /> Refresh
						</button>
					</div>

					<div className="overflow-x-auto">
						<table className="min-w-full table-fixed divide-y divide-secondary-200 dark:divide-secondary-600">
							<thead className="bg-secondary-50 dark:bg-secondary-700">
								<tr>
									<th className={TH}>Alert type</th>
									<th className={`${TH} w-20`}>Active</th>
									<th className={`${TH} w-28`}>Severity</th>
									<th className={`${TH} w-24`}>Alert delay</th>
									<th className={TH}>Auto-assign</th>
									<th className={`${TH} w-24`}>Retention</th>
									<th className={`${TH} w-28`}>Auto-resolve</th>
								</tr>
							</thead>
							<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-600">
								{configs.map((c) => {
									const dis = bulkUpdateMutation.isPending;
									const off = !c.is_enabled;
									return (
										<tr
											key={c.alert_type}
											className="hover:bg-secondary-50 dark:hover:bg-secondary-700"
										>
											<td className={TD}>
												<span className="font-medium">
													{formatAlertType(c.alert_type)}
												</span>
											</td>
											<td className={TD}>
												<Toggle
													checked={c.is_enabled}
													onChange={(v) =>
														handleFieldChange(c.alert_type, "is_enabled", v)
													}
													disabled={dis}
												/>
											</td>
											<td className={TD}>
												{off ? (
													<span className="text-secondary-400">-</span>
												) : (
													<select
														className={SELECT_SM}
														value={c.default_severity}
														onChange={(e) =>
															handleFieldChange(
																c.alert_type,
																"default_severity",
																e.target.value,
															)
														}
														disabled={dis}
													>
														{SEVERITIES.map((s) => (
															<option key={s.value} value={s.value}>
																{s.label}
															</option>
														))}
													</select>
												)}
											</td>
											<td className={TD}>
												{off ? (
													<span className="text-secondary-400">-</span>
												) : (
													<div className="flex items-center gap-1">
														<input
															type="number"
															min={0}
															className={INPUT_SM}
															value={c.alert_delay_seconds || ""}
															placeholder="-"
															onChange={(e) =>
																handleFieldChange(
																	c.alert_type,
																	"alert_delay_seconds",
																	e.target.value
																		? Number.parseInt(e.target.value, 10)
																		: null,
																)
															}
															disabled={dis}
														/>
														<span className="text-xs text-secondary-400">
															sec
														</span>
													</div>
												)}
											</td>
											<td className={TD}>
												{off ? (
													<span className="text-secondary-400">-</span>
												) : (
													<div className="flex items-center gap-2">
														<Toggle
															checked={c.auto_assign_enabled}
															onChange={(v) =>
																handleFieldChange(
																	c.alert_type,
																	"auto_assign_enabled",
																	v,
																)
															}
															disabled={dis}
														/>
														{c.auto_assign_enabled && (
															<select
																className={`${SELECT_SM} text-xs min-w-[100px]`}
																value={c.auto_assign_user_id || ""}
																onChange={(e) =>
																	handleFieldChange(
																		c.alert_type,
																		"auto_assign_user_id",
																		e.target.value || null,
																	)
																}
																disabled={dis}
															>
																<option value="">Select...</option>
																{usersData?.map((u) => (
																	<option key={u.id} value={u.id}>
																		{u.username || u.email}
																	</option>
																))}
															</select>
														)}
													</div>
												)}
											</td>
											<td className={TD}>
												{off ? (
													<span className="text-secondary-400">-</span>
												) : (
													<div className="flex items-center gap-1">
														<input
															type="number"
															min={1}
															className={INPUT_SM}
															value={c.retention_days || ""}
															placeholder="-"
															onChange={(e) =>
																handleFieldChange(
																	c.alert_type,
																	"retention_days",
																	e.target.value
																		? Number.parseInt(e.target.value, 10)
																		: null,
																)
															}
															disabled={dis}
														/>
														<span className="text-xs text-secondary-400">
															days
														</span>
													</div>
												)}
											</td>
											<td className={TD}>
												{off ? (
													<span className="text-secondary-400">-</span>
												) : (
													<div className="flex items-center gap-1">
														<input
															type="number"
															min={1}
															className={INPUT_SM}
															value={c.auto_resolve_after_days || ""}
															placeholder="-"
															onChange={(e) =>
																handleFieldChange(
																	c.alert_type,
																	"auto_resolve_after_days",
																	e.target.value
																		? Number.parseInt(e.target.value, 10)
																		: null,
																)
															}
															disabled={dis}
														/>
														<span className="text-xs text-secondary-400">
															days
														</span>
													</div>
												)}
											</td>
										</tr>
									);
								})}
							</tbody>
						</table>
					</div>
				</div>
			)}

			{/* Disabled placeholder */}
			{!alertsEnabled && (
				<div className="card p-8 text-center">
					<AlertTriangle className="h-12 w-12 mx-auto text-secondary-300 dark:text-secondary-600 mb-3" />
					<p className="text-sm text-secondary-500">
						Enable the alerts system above to configure alert types.
					</p>
				</div>
			)}

			{/* Cleanup */}
			{alertsEnabled && (
				<div className="card p-4 md:p-6 space-y-4">
					<h2 className="text-lg font-semibold text-secondary-900 dark:text-white">
						Alert cleanup
					</h2>
					<p className="text-sm text-secondary-600 dark:text-white">
						Preview and delete alerts based on retention policies configured
						above.
					</p>
					<CleanupSection />
				</div>
			)}
		</div>
	);
};

const CleanupSection = () => {
	const toast = useToast();
	const [previewLoading, setPreviewLoading] = useState(false);
	const [previewData, setPreviewData] = useState(null);

	const handlePreview = async () => {
		setPreviewLoading(true);
		try {
			const response = await alertsAPI.previewCleanup();
			const data = response.data.data;
			setPreviewData(Array.isArray(data) ? data : (data?.alerts ?? []));
		} catch {
			toast.error("Failed to preview cleanup");
		} finally {
			setPreviewLoading(false);
		}
	};

	const handleCleanup = async () => {
		if (!window.confirm("Delete these alerts? This cannot be undone.")) return;
		try {
			const response = await alertsAPI.triggerCleanup();
			const count =
				response.data.data.deleted ?? response.data.data.deleted_count ?? 0;
			toast.success(`Cleanup completed: ${count} alert(s) deleted`);
			setPreviewData(null);
		} catch (err) {
			toast.error(
				`Cleanup failed: ${err.response?.data?.error || err.message}`,
			);
		}
	};

	return (
		<div className="space-y-3">
			<div className="flex items-center gap-3">
				<button
					type="button"
					onClick={handlePreview}
					disabled={previewLoading}
					className="btn-outline flex items-center gap-2 text-sm"
				>
					{previewLoading ? (
						<Loader2 className="h-3.5 w-3.5 animate-spin" />
					) : (
						<RefreshCw className="h-3.5 w-3.5" />
					)}
					Preview cleanup
				</button>
				{previewData && previewData.length > 0 && (
					<button
						type="button"
						onClick={handleCleanup}
						className="btn-danger flex items-center gap-2 text-sm"
					>
						<Trash2 className="h-3.5 w-3.5" /> Delete {previewData.length}{" "}
						alerts
					</button>
				)}
			</div>
			{previewData && previewData.length === 0 && (
				<p className="text-sm text-secondary-500 flex items-center gap-1">
					<Check className="h-4 w-4 text-green-500" /> No alerts need cleanup.
				</p>
			)}
			{previewData && previewData.length > 0 && (
				<div className="rounded-md p-3 bg-secondary-50 dark:bg-secondary-700/50">
					<p className="text-sm font-medium text-secondary-900 dark:text-white mb-2">
						{previewData.length} alert(s) would be deleted:
					</p>
					<ul className="list-disc list-inside text-sm text-secondary-600 dark:text-white space-y-0.5">
						{previewData.slice(0, 10).map((a) => (
							<li key={a.id ?? a.ID}>
								{formatAlertType(a.type ?? a.Type)} -{" "}
								{new Date(a.created_at ?? a.CreatedAt).toLocaleDateString()}
							</li>
						))}
						{previewData.length > 10 && (
							<li>... and {previewData.length - 10} more</li>
						)}
					</ul>
				</div>
			)}
		</div>
	);
};

export { AlertSettings };
export default AlertSettings;
