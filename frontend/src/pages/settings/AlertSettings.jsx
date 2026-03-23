import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
	AlertTriangle,
	Check,
	ChevronDown,
	ChevronRight,
	Loader2,
	RefreshCw,
	Save,
	Trash2,
	X,
} from "lucide-react";
import { useEffect, useMemo, useState } from "react";
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

const THRESHOLD_ALERT_TYPES = [
	"host_security_updates_exceeded",
	"host_pending_updates_exceeded",
];

const PERIODIC_ALERT_TYPES = [
	"host_down",
	"host_security_updates_exceeded",
	"host_pending_updates_exceeded",
];

const getThreshold = (c) => {
	if (!c?.metadata) return "";
	const m =
		typeof c.metadata === "string" ? JSON.parse(c.metadata) : c.metadata;
	return m?.threshold ?? "";
};

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
		"check_interval_minutes",
	];
	if (
		!fields.every((f) => {
			const va = a[f];
			const vb = b[f];
			if (va == null && vb == null) return true;
			if (va == null || vb == null) return false;
			return String(va) === String(vb);
		})
	)
		return false;
	// Compare metadata threshold for threshold alert types.
	if (THRESHOLD_ALERT_TYPES.includes(a.alert_type)) {
		return String(getThreshold(a)) === String(getThreshold(b));
	}
	return true;
};

const formatAlertType = (type) =>
	type.replace(/_/g, " ").replace(/\b\w/g, (l) => l.toUpperCase());

const CATEGORY_ORDER = [
	"host",
	"patching",
	"compliance",
	"docker",
	"security",
	"remote_access",
	"system",
];

const formatCategory = (cat) =>
	cat.replace(/_/g, " ").replace(/\b\w/g, (l) => l.toUpperCase());

const groupConfigsByCategory = (configs) => {
	const groups = {};
	for (const c of configs) {
		const cat = c.category || "general";
		if (!groups[cat]) groups[cat] = [];
		groups[cat].push(c);
	}
	const ordered = [];
	for (const cat of CATEGORY_ORDER) {
		if (groups[cat]) {
			ordered.push({ category: cat, configs: groups[cat] });
			delete groups[cat];
		}
	}
	const remaining = Object.keys(groups).sort();
	for (const cat of remaining) {
		ordered.push({ category: cat, configs: groups[cat] });
	}
	return ordered;
};

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
	const [collapsedCategories, setCollapsedCategories] = useState({});

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

	const { data: usersData, isLoading: usersLoading } = useQuery({
		queryKey: ["users", "for-assignment"],
		queryFn: async () => {
			const extractUsers = (response) => {
				const d = response.data;
				if (Array.isArray(d)) return d;
				if (Array.isArray(d?.data)) return d.data;
				if (Array.isArray(d?.users)) return d.users;
				return [];
			};
			try {
				const users = extractUsers(await adminUsersAPI.listForAssignment());
				if (users.length > 0) return users;
			} catch (_e) {
				// listForAssignment unavailable, try admin list
			}
			try {
				return extractUsers(await adminUsersAPI.list());
			} catch (_e) {
				return [];
			}
		},
		staleTime: 5 * 60 * 1000,
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
				"check_interval_minutes",
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
				check_interval_minutes: c.check_interval_minutes ?? null,
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

	const alertsEnabled = settings?.alerts_enabled !== false;
	const configs = localConfigs ?? alertConfigs ?? [];
	const groupedConfigs = useMemo(
		() => groupConfigsByCategory(configs),
		[configs],
	);

	const toggleCategory = (cat) => {
		setCollapsedCategories((prev) => ({ ...prev, [cat]: !prev[cat] }));
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

	return (
		<div className="space-y-6">
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

			{/* Alert Type Table — grouped by category */}
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

					{groupedConfigs.map(({ category, configs: catConfigs }) => {
						const isCollapsed = !!collapsedCategories[category];
						return (
							<div key={category}>
								<button
									type="button"
									onClick={() => toggleCategory(category)}
									className="w-full flex items-center gap-2 py-2 px-3 rounded-md bg-secondary-100 dark:bg-secondary-700 hover:bg-secondary-200 dark:hover:bg-secondary-600 transition-colors"
								>
									{isCollapsed ? (
										<ChevronRight className="h-4 w-4 text-secondary-500 dark:text-secondary-400 shrink-0" />
									) : (
										<ChevronDown className="h-4 w-4 text-secondary-500 dark:text-secondary-400 shrink-0" />
									)}
									<span className="text-sm font-semibold text-secondary-800 dark:text-white">
										{formatCategory(category)}
									</span>
									<span className="text-xs text-secondary-500 dark:text-secondary-400">
										({catConfigs.length})
									</span>
								</button>

								{!isCollapsed && (
									<div className="overflow-x-auto mt-1">
										<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-600">
											<thead className="bg-secondary-50 dark:bg-secondary-700">
												<tr>
													<th className={`${TH} min-w-[160px]`}>Alert type</th>
													<th className={`${TH} w-16`}>Active</th>
													<th className={`${TH} w-28`}>Severity</th>
													<th className={`${TH} w-28`}>Alert delay</th>
													<th className={`${TH} w-28`}>Frequency</th>
													<th className={`${TH} w-24`}>Threshold</th>
													<th className={`${TH} min-w-[180px]`}>Auto-assign</th>
													<th className={`${TH} w-28`}>Retention</th>
													<th className={`${TH} w-28`}>Auto-resolve</th>
												</tr>
											</thead>
											<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-600">
												{catConfigs.map((c) => {
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
																		handleFieldChange(
																			c.alert_type,
																			"is_enabled",
																			v,
																		)
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
																						? Number.parseInt(
																								e.target.value,
																								10,
																							)
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
																{off ||
																!PERIODIC_ALERT_TYPES.includes(c.alert_type) ? (
																	<span className="text-secondary-400">-</span>
																) : (
																	<div className="flex items-center gap-1">
																		<input
																			type="number"
																			min={1}
																			className={INPUT_SM}
																			value={c.check_interval_minutes ?? ""}
																			placeholder="-"
																			onChange={(e) =>
																				handleFieldChange(
																					c.alert_type,
																					"check_interval_minutes",
																					e.target.value
																						? Number.parseInt(
																								e.target.value,
																								10,
																							)
																						: null,
																				)
																			}
																			disabled={dis}
																		/>
																		<span className="text-xs text-secondary-400">
																			min
																		</span>
																	</div>
																)}
															</td>
															<td className={TD}>
																{off ||
																!THRESHOLD_ALERT_TYPES.includes(
																	c.alert_type,
																) ? (
																	<span className="text-secondary-400">-</span>
																) : (
																	<input
																		type="number"
																		min={0}
																		className={INPUT_SM}
																		value={getThreshold(c)}
																		placeholder="-"
																		onChange={(e) => {
																			const val = e.target.value
																				? Number.parseInt(e.target.value, 10)
																				: null;
																			const prev =
																				typeof c.metadata === "string"
																					? JSON.parse(c.metadata || "{}")
																					: c.metadata || {};
																			handleFieldChange(
																				c.alert_type,
																				"metadata",
																				{
																					...prev,
																					threshold: val,
																				},
																			);
																		}}
																		disabled={dis}
																	/>
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
																				className={`${SELECT_SM} text-xs min-w-[120px]`}
																				value={c.auto_assign_user_id || ""}
																				onChange={(e) =>
																					handleFieldChange(
																						c.alert_type,
																						"auto_assign_user_id",
																						e.target.value || null,
																					)
																				}
																				disabled={
																					dis ||
																					usersLoading ||
																					!usersData?.length
																				}
																			>
																				<option value="">
																					{usersLoading
																						? "Loading..."
																						: !usersData?.length
																							? "No users found"
																							: "Select user..."}
																				</option>
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
																						? Number.parseInt(
																								e.target.value,
																								10,
																							)
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
																						? Number.parseInt(
																								e.target.value,
																								10,
																							)
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
								)}
							</div>
						);
					})}
				</div>
			)}

			{/* Disabled placeholder */}
			{!alertsEnabled && (
				<div className="card p-8 text-center">
					<AlertTriangle className="h-12 w-12 mx-auto text-secondary-300 dark:text-secondary-600 mb-3" />
					<p className="text-sm text-secondary-500">
						Enable the alerts system using the master switch below to configure
						alert types.
					</p>
				</div>
			)}

			{/* Bottom section: Cleanup (left) + Master Switch (right) */}
			<div className="grid grid-cols-1 md:grid-cols-2 gap-6">
				{/* Cleanup - Left side */}
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

				{/* Master Switch - Right side */}
				<div
					className={`card p-4 md:p-6 ${!alertsEnabled ? "md:col-start-2" : ""}`}
				>
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
			</div>
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
