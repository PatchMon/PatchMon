import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
	Bell,
	Clock,
	Loader2,
	Play,
	Plus,
	RefreshCw,
	Send,
	Trash2,
} from "lucide-react";
import { useMemo, useState } from "react";
import { useAuth } from "../../contexts/AuthContext";
import { useToast } from "../../contexts/ToastContext";
import {
	formatRelativeTime,
	hostGroupsAPI,
	notificationsAPI,
} from "../../utils/api";

const EVENT_TYPES = [
	{ value: "*", label: "All events (wildcard)" },
	{ value: "host_down", label: "Host down" },
	{ value: "host_recovered", label: "Host recovered" },
	{ value: "server_update", label: "Server update" },
	{ value: "agent_update", label: "Agent update" },
	{ value: "patch_run_completed", label: "Patch run completed" },
	{ value: "patch_run_failed", label: "Patch run failed" },
	{ value: "compliance_scan_completed", label: "Compliance scan completed" },
	{ value: "test", label: "Test (route-based; prefer Test button)" },
];

const SEVERITIES = [
	{ value: "informational", label: "Informational" },
	{ value: "warning", label: "Warning" },
	{ value: "error", label: "Error" },
	{ value: "critical", label: "Critical" },
];

const REPORT_SECTIONS = [
	{ id: "executive_summary", label: "Executive summary" },
	{ id: "compliance_summary", label: "Compliance summary" },
	{ id: "recent_patch_runs", label: "Recent patch runs" },
	{ id: "hosts_offline", label: "Hosts / status" },
];

// Human-readable cron description (common patterns)
const describeCron = (expr) => {
	if (!expr) return "";
	const parts = expr.trim().split(/\s+/);
	if (parts.length !== 5) return "";
	const [min, hour, dom, mon, dow] = parts;
	const dayNames = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
	const h = Number.parseInt(hour, 10);
	const m = Number.parseInt(min, 10);
	const time =
		!Number.isNaN(h) && !Number.isNaN(m)
			? `${String(h).padStart(2, "0")}:${String(m).padStart(2, "0")}`
			: null;
	if (!time) return "";
	if (dom === "*" && mon === "*" && dow === "*") return `Daily at ${time}`;
	if (dom === "*" && mon === "*" && dow !== "*") {
		const days = dow
			.split(",")
			.map((d) => dayNames[Number.parseInt(d, 10)] || d)
			.join(", ");
		return `${days} at ${time}`;
	}
	if (dom !== "*" && mon === "*" && dow === "*")
		return `Day ${dom} of each month at ${time}`;
	return "";
};

const defaultWebhookConfig = () =>
	JSON.stringify({ url: "", headers: {}, signing_secret: "" }, null, 2);

const defaultEmailConfig = () =>
	JSON.stringify(
		{
			smtp_host: "",
			smtp_port: 587,
			username: "",
			password: "",
			from: "",
			to: "",
			use_tls: true,
		},
		null,
		2,
	);

const AlertChannels = () => {
	const queryClient = useQueryClient();
	const toast = useToast();
	const { canManageNotifications, canViewNotificationLogs, hasPermission } =
		useAuth();
	const canManage = canManageNotifications();
	const canLog = canViewNotificationLogs();
	const canListHostGroups = hasPermission("can_view_hosts");

	const [destForm, setDestForm] = useState({
		channel_type: "webhook",
		display_name: "",
		enabled: true,
		configText: defaultWebhookConfig(),
	});
	const [editingDestId, setEditingDestId] = useState(null);

	const [routeForm, setRouteForm] = useState({
		destination_id: "",
		event_type: "host_down",
		min_severity: "informational",
		host_group_id: "",
		enabled: true,
	});
	const [editingRouteId, setEditingRouteId] = useState(null);

	const [reportForm, setReportForm] = useState({
		name: "",
		cron_expr: "0 8 * * *",
		timezone: "UTC",
		enabled: true,
		destination_ids: [],
		sections: ["executive_summary", "compliance_summary", "recent_patch_runs"],
		host_group_ids: [],
		top_hosts: 20,
	});
	const [editingReportId, setEditingReportId] = useState(null);

	// Delivery log pagination state
	const [logPage, setLogPage] = useState(0);
	const logPageSize = 50;

	const { data: destinations = [], isLoading: destLoading } = useQuery({
		queryKey: ["notifications", "destinations"],
		queryFn: () => notificationsAPI.listDestinations().then((r) => r.data),
		enabled: canManage,
	});

	const { data: routes = [], isLoading: routesLoading } = useQuery({
		queryKey: ["notifications", "routes"],
		queryFn: () => notificationsAPI.listRoutes().then((r) => r.data),
		enabled: canManage,
	});

	const { data: deliveryLog = [], isLoading: logLoading } = useQuery({
		queryKey: ["notifications", "delivery-log", logPage],
		queryFn: () =>
			notificationsAPI
				.listDeliveryLog({ limit: logPageSize, offset: logPage * logPageSize })
				.then((r) => r.data),
		enabled: canLog,
	});

	const { data: scheduledReports = [], isLoading: reportsLoading } = useQuery({
		queryKey: ["notifications", "scheduled-reports"],
		queryFn: () => notificationsAPI.listScheduledReports().then((r) => r.data),
		enabled: canManage,
	});

	const { data: hostGroups = [] } = useQuery({
		queryKey: ["host-groups"],
		queryFn: () => hostGroupsAPI.list().then((r) => r.data ?? []),
		enabled: canManage && canListHostGroups,
	});

	const hostGroupOptions = useMemo(() => {
		const list = Array.isArray(hostGroups) ? hostGroups : [];
		return list;
	}, [hostGroups]);

	// Build destination ID -> display_name lookup
	const destNameMap = useMemo(() => {
		const map = {};
		for (const d of destinations) {
			map[d.id] = d.display_name;
		}
		return map;
	}, [destinations]);

	// Build host group ID -> name lookup
	const hostGroupNameMap = useMemo(() => {
		const map = {};
		for (const g of hostGroupOptions) {
			map[g.id] = g.name || g.id;
		}
		return map;
	}, [hostGroupOptions]);

	const invalidateNotifications = () => {
		queryClient.invalidateQueries({ queryKey: ["notifications"] });
	};

	const createDest = useMutation({
		mutationFn: (body) => notificationsAPI.createDestination(body),
		onSuccess: () => {
			invalidateNotifications();
			toast.success("Destination created");
		},
		onError: (err) =>
			toast.error(err.response?.data?.error || "Failed to create destination"),
	});

	const updateDest = useMutation({
		mutationFn: ({ id, body }) => notificationsAPI.updateDestination(id, body),
		onSuccess: () => {
			invalidateNotifications();
			toast.success("Destination updated");
		},
		onError: (err) =>
			toast.error(err.response?.data?.error || "Failed to update destination"),
	});

	const deleteDest = useMutation({
		mutationFn: (id) => notificationsAPI.deleteDestination(id),
		onSuccess: () => {
			invalidateNotifications();
			toast.success("Destination deleted");
		},
		onError: (err) =>
			toast.error(err.response?.data?.error || "Failed to delete destination"),
	});

	const createRoute = useMutation({
		mutationFn: (body) => notificationsAPI.createRoute(body),
		onSuccess: () => {
			invalidateNotifications();
			toast.success("Route created");
		},
		onError: (err) =>
			toast.error(err.response?.data?.error || "Failed to create route"),
	});

	const updateRoute = useMutation({
		mutationFn: ({ id, body }) => notificationsAPI.updateRoute(id, body),
		onSuccess: () => {
			invalidateNotifications();
			toast.success("Route updated");
		},
		onError: (err) =>
			toast.error(err.response?.data?.error || "Failed to update route"),
	});

	const deleteRoute = useMutation({
		mutationFn: (id) => notificationsAPI.deleteRoute(id),
		onSuccess: () => {
			invalidateNotifications();
			toast.success("Route deleted");
		},
		onError: (err) =>
			toast.error(err.response?.data?.error || "Failed to delete route"),
	});

	const testNotify = useMutation({
		mutationFn: (destination_id) => notificationsAPI.test({ destination_id }),
	});

	const sendTest = (destinationId) => {
		testNotify.mutate(destinationId, {
			onSuccess: () => {
				toast.info(
					"Test notification enqueued. Check the delivery log below to confirm delivery.",
				);
				// Refresh the delivery log after a short delay so the result appears
				setTimeout(() => {
					queryClient.invalidateQueries({
						queryKey: ["notifications", "delivery-log"],
					});
				}, 3000);
			},
			onError: (err) => {
				toast.error(err.response?.data?.error || err.message || "Test failed");
			},
		});
	};

	const createReport = useMutation({
		mutationFn: (body) => notificationsAPI.createScheduledReport(body),
		onSuccess: () => {
			invalidateNotifications();
			toast.success("Scheduled report created");
		},
		onError: (err) =>
			toast.error(err.response?.data?.error || "Failed to create report"),
	});

	const updateReport = useMutation({
		mutationFn: ({ id, body }) =>
			notificationsAPI.updateScheduledReport(id, body),
		onSuccess: () => {
			invalidateNotifications();
			toast.success("Scheduled report updated");
		},
		onError: (err) =>
			toast.error(err.response?.data?.error || "Failed to update report"),
	});

	const deleteReport = useMutation({
		mutationFn: (id) => notificationsAPI.deleteScheduledReport(id),
		onSuccess: () => {
			invalidateNotifications();
			toast.success("Scheduled report deleted");
		},
		onError: (err) =>
			toast.error(err.response?.data?.error || "Failed to delete report"),
	});

	const runReportNow = useMutation({
		mutationFn: (id) => notificationsAPI.runScheduledReportNow(id),
		onSuccess: () => {
			invalidateNotifications();
			toast.success("Report scheduled for immediate delivery");
		},
		onError: (err) =>
			toast.error(err.response?.data?.error || "Failed to trigger report run"),
	});

	const parseConfig = () => {
		try {
			return JSON.parse(destForm.configText || "{}");
		} catch {
			throw new Error("Config must be valid JSON");
		}
	};

	const submitDestination = async (e) => {
		e.preventDefault();
		const displayName = destForm.display_name.trim();
		if (!displayName) {
			toast.warning("Display name is required");
			return;
		}
		try {
			if (editingDestId) {
				const patch = {
					display_name: displayName,
					enabled: destForm.enabled,
				};
				const raw = destForm.configText.trim();
				// Strip leading comment lines (// ...) to find actual JSON
				const jsonPart = raw
					.split("\n")
					.filter((line) => !line.trimStart().startsWith("//"))
					.join("\n")
					.trim();
				if (jsonPart) {
					try {
						patch.config = JSON.parse(jsonPart);
					} catch {
						toast.warning("Config must be valid JSON");
						return;
					}
				}
				await updateDest.mutateAsync({ id: editingDestId, body: patch });
			} else {
				let config;
				try {
					config = parseConfig();
				} catch (err) {
					toast.warning(err.message);
					return;
				}
				await createDest.mutateAsync({
					channel_type: destForm.channel_type,
					display_name: displayName,
					config,
					enabled: destForm.enabled,
				});
			}
			setEditingDestId(null);
			setDestForm({
				channel_type: "webhook",
				display_name: "",
				enabled: true,
				configText: defaultWebhookConfig(),
			});
		} catch {
			// Error already handled by mutation onError
		}
	};

	const startEditDestination = (d) => {
		setEditingDestId(d.id);
		setDestForm({
			channel_type: d.channel_type,
			display_name: d.display_name,
			enabled: d.enabled,
			configText: d.has_secret
				? "// Credentials are stored securely. Leave this field as-is to keep them,\n// or paste new JSON to replace:\n// " +
					(d.channel_type === "email"
						? '{ "smtp_host": "...", "smtp_port": 587, "username": "...", "password": "...", "from": "...", "to": "...", "use_tls": true }'
						: '{ "url": "https://...", "headers": {}, "signing_secret": "" }')
				: d.channel_type === "email"
					? defaultEmailConfig()
					: defaultWebhookConfig(),
		});
	};

	const submitRoute = async (e) => {
		e.preventDefault();
		if (!routeForm.destination_id) {
			toast.warning("Choose a destination");
			return;
		}
		const body = {
			destination_id: routeForm.destination_id,
			event_type: routeForm.event_type,
			min_severity: routeForm.min_severity,
			enabled: routeForm.enabled,
		};
		if (routeForm.host_group_id) {
			body.host_group_id = routeForm.host_group_id;
		}
		try {
			if (editingRouteId) {
				await updateRoute.mutateAsync({ id: editingRouteId, body });
			} else {
				await createRoute.mutateAsync(body);
			}
			setEditingRouteId(null);
			setRouteForm({
				destination_id: "",
				event_type: "host_down",
				min_severity: "informational",
				host_group_id: "",
				enabled: true,
			});
		} catch {
			// Error already handled by mutation onError
		}
	};

	const startEditRoute = (row) => {
		setEditingRouteId(row.id);
		setRouteForm({
			destination_id: row.destination_id,
			event_type: row.event_type,
			min_severity: row.min_severity || "informational",
			host_group_id: row.host_group_id || "",
			enabled: row.enabled !== false,
		});
	};

	const buildReportDefinition = () => ({
		version: 1,
		sections: reportForm.sections,
		host_group_ids: reportForm.host_group_ids,
		limits: { top_hosts: Number(reportForm.top_hosts) || 20 },
	});

	const submitReport = async (e) => {
		e.preventDefault();
		if (!reportForm.name.trim()) {
			toast.warning("Report name is required");
			return;
		}
		const body = {
			name: reportForm.name.trim(),
			cron_expr: reportForm.cron_expr.trim() || "0 8 * * *",
			timezone: reportForm.timezone.trim() || "UTC",
			enabled: reportForm.enabled,
			definition: buildReportDefinition(),
			destination_ids: reportForm.destination_ids,
		};
		try {
			if (editingReportId) {
				await updateReport.mutateAsync({ id: editingReportId, body });
			} else {
				await createReport.mutateAsync(body);
			}
			setEditingReportId(null);
			setReportForm({
				name: "",
				cron_expr: "0 8 * * *",
				timezone: "UTC",
				enabled: true,
				destination_ids: [],
				sections: [
					"executive_summary",
					"compliance_summary",
					"recent_patch_runs",
				],
				host_group_ids: [],
				top_hosts: 20,
			});
		} catch {
			// Error already handled by mutation onError
		}
	};

	const startEditReport = (row) => {
		const def = row.definition || {};
		setEditingReportId(row.id);
		setReportForm({
			name: row.name,
			cron_expr: row.cron_expr,
			timezone: row.timezone || "UTC",
			enabled: row.enabled !== false,
			destination_ids: Array.isArray(row.destination_ids)
				? row.destination_ids
				: [],
			sections:
				Array.isArray(def.sections) && def.sections.length > 0
					? def.sections
					: ["executive_summary", "compliance_summary", "recent_patch_runs"],
			host_group_ids: Array.isArray(def.host_group_ids)
				? def.host_group_ids
				: [],
			top_hosts: def.limits?.top_hosts ?? 20,
		});
	};

	const toggleSection = (id) => {
		setReportForm((prev) => {
			const has = prev.sections.includes(id);
			return {
				...prev,
				sections: has
					? prev.sections.filter((s) => s !== id)
					: [...prev.sections, id],
			};
		});
	};

	const toggleReportDestination = (id) => {
		setReportForm((prev) => {
			const has = prev.destination_ids.includes(id);
			return {
				...prev,
				destination_ids: has
					? prev.destination_ids.filter((x) => x !== id)
					: [...prev.destination_ids, id],
			};
		});
	};

	const toggleReportHostGroup = (id) => {
		setReportForm((prev) => {
			const has = prev.host_group_ids.includes(id);
			return {
				...prev,
				host_group_ids: has
					? prev.host_group_ids.filter((x) => x !== id)
					: [...prev.host_group_ids, id],
			};
		});
	};

	const cardClass =
		"bg-white dark:bg-secondary-800 border border-secondary-200 dark:border-secondary-600 rounded-lg";

	// Cron preview for the report form
	const cronPreview = describeCron(reportForm.cron_expr);

	return (
		<div className="space-y-8">
			<div className="flex items-center justify-between gap-4 flex-wrap">
				<div>
					<h1 className="text-2xl font-bold text-secondary-900 dark:text-white">
						Notifications
					</h1>
					<p className="mt-1 text-sm text-secondary-600 dark:text-secondary-300">
						Destinations (webhook or email), routes, scheduled reports, and
						delivery history
					</p>
				</div>
				{canLog && (
					<button
						type="button"
						onClick={() =>
							queryClient.invalidateQueries({
								queryKey: ["notifications", "delivery-log"],
							})
						}
						className="inline-flex items-center gap-2 px-3 py-2 text-sm rounded-md border border-secondary-300 dark:border-secondary-600 hover:bg-secondary-50 dark:hover:bg-secondary-700"
					>
						<RefreshCw className="h-4 w-4" />
						Refresh log
					</button>
				)}
			</div>

			{canManage && (
				<section className={`${cardClass} p-6 space-y-4`}>
					<div className="flex items-center justify-between">
						<h2 className="text-lg font-semibold text-secondary-900 dark:text-white">
							Destinations
						</h2>
						{destLoading && <Loader2 className="h-5 w-5 animate-spin" />}
					</div>
					<p className="text-sm text-secondary-600 dark:text-secondary-300">
						Webhook JSON: <code className="text-xs">url</code>, optional{" "}
						<code className="text-xs">headers</code>,{" "}
						<code className="text-xs">signing_secret</code> (leave both empty
						for Slack (
						<code className="text-xs">hooks.slack.com/services/…</code>) or
						Discord (<code className="text-xs">discord.com/api/webhooks/…</code>
						); otherwise the default JSON payload is sent. Email JSON:{" "}
						<code className="text-xs">smtp_host</code>,{" "}
						<code className="text-xs">smtp_port</code>,{" "}
						<code className="text-xs">from</code>,{" "}
						<code className="text-xs">to</code>, etc.
					</p>

					{destinations.length > 0 && (
						<ul className="divide-y divide-secondary-200 dark:divide-secondary-600">
							{destinations.map((d) => (
								<li
									key={d.id}
									className="py-3 flex flex-wrap items-center justify-between gap-2"
								>
									<div>
										<p className="font-medium text-secondary-900 dark:text-white">
											{d.display_name}
										</p>
										<p className="text-xs text-secondary-500">
											{d.channel_type}
											{d.enabled ? "" : " · disabled"}
											{d.has_secret ? " · credentials stored" : ""}
										</p>
									</div>
									<div className="flex items-center gap-2">
										<button
											type="button"
											className="text-sm text-primary-600 hover:underline"
											onClick={() => sendTest(d.id)}
											disabled={testNotify.isPending}
										>
											<span className="inline-flex items-center gap-1">
												<Send className="h-3.5 w-3.5" />
												Test
											</span>
										</button>
										<button
											type="button"
											className="text-sm text-primary-600 hover:underline"
											onClick={() => startEditDestination(d)}
										>
											Edit
										</button>
										<button
											type="button"
											className="text-sm text-red-600 hover:underline"
											onClick={() => {
												if (confirm("Delete this destination?")) {
													deleteDest.mutate(d.id);
												}
											}}
										>
											<Trash2 className="h-4 w-4 inline" />
										</button>
									</div>
								</li>
							))}
						</ul>
					)}

					<form
						onSubmit={submitDestination}
						className="space-y-3 border-t border-secondary-200 dark:border-secondary-600 pt-4"
					>
						<p className="text-sm font-medium text-secondary-800 dark:text-secondary-200">
							{editingDestId ? "Edit destination" : "Add destination"}
						</p>
						<div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
							<label className="block text-sm">
								<span className="text-secondary-700 dark:text-secondary-300">
									Channel
								</span>
								<select
									className="mt-1 w-full rounded-md border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-900 px-2 py-2 text-sm"
									value={destForm.channel_type}
									onChange={(e) => {
										const t = e.target.value;
										setDestForm((prev) => ({
											...prev,
											channel_type: t,
											configText:
												t === "email"
													? defaultEmailConfig()
													: defaultWebhookConfig(),
										}));
									}}
									disabled={!!editingDestId}
								>
									<option value="webhook">Webhook</option>
									<option value="email">Email (SMTP)</option>
								</select>
							</label>
							<label className="block text-sm">
								<span className="text-secondary-700 dark:text-secondary-300">
									Display name
								</span>
								<input
									className="mt-1 w-full rounded-md border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-900 px-2 py-2 text-sm"
									value={destForm.display_name}
									onChange={(e) =>
										setDestForm((p) => ({
											...p,
											display_name: e.target.value,
										}))
									}
								/>
							</label>
						</div>
						<label className="flex items-center gap-2 text-sm">
							<input
								type="checkbox"
								checked={destForm.enabled}
								onChange={(e) =>
									setDestForm((p) => ({ ...p, enabled: e.target.checked }))
								}
							/>
							Enabled
						</label>
						<label className="block text-sm">
							<span className="text-secondary-700 dark:text-secondary-300">
								Config (JSON)
							</span>
							<textarea
								className="mt-1 w-full font-mono text-xs rounded-md border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-900 px-2 py-2 min-h-[140px]"
								value={destForm.configText}
								onChange={(e) =>
									setDestForm((p) => ({ ...p, configText: e.target.value }))
								}
							/>
						</label>
						<div className="flex gap-2">
							<button
								type="submit"
								className="inline-flex items-center gap-2 px-4 py-2 rounded-md bg-primary-600 text-white text-sm hover:bg-primary-700"
								disabled={createDest.isPending || updateDest.isPending}
							>
								<Plus className="h-4 w-4" />
								{editingDestId ? "Save changes" : "Add destination"}
							</button>
							{editingDestId && (
								<button
									type="button"
									className="px-4 py-2 text-sm rounded-md border border-secondary-300 dark:border-secondary-600"
									onClick={() => {
										setEditingDestId(null);
										setDestForm({
											channel_type: "webhook",
											display_name: "",
											enabled: true,
											configText: defaultWebhookConfig(),
										});
									}}
								>
									Cancel
								</button>
							)}
						</div>
					</form>
				</section>
			)}

			{canManage && (
				<section className={`${cardClass} p-6 space-y-4`}>
					<div className="flex items-center justify-between">
						<h2 className="text-lg font-semibold text-secondary-900 dark:text-white">
							Routes
						</h2>
						{routesLoading && <Loader2 className="h-5 w-5 animate-spin" />}
					</div>
					<p className="text-sm text-secondary-600 dark:text-secondary-300">
						Map event types to destinations. Use &quot;All events&quot; to match
						every event type. Optional host group limits routing to hosts in
						that group.
					</p>

					{routes.length === 0 &&
						destinations.length === 0 &&
						!routesLoading && (
							<div className="rounded-md p-4 bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800">
								<p className="text-sm text-blue-800 dark:text-blue-200">
									Create a destination above first, then add routes here to
									start receiving notifications.
								</p>
							</div>
						)}

					{routes.length > 0 && (
						<div className="overflow-x-auto">
							<table className="min-w-full text-sm">
								<thead>
									<tr className="text-left border-b border-secondary-200 dark:border-secondary-600">
										<th className="py-2 pr-4">Destination</th>
										<th className="py-2 pr-4">Event</th>
										<th className="py-2 pr-4">Min severity</th>
										<th className="py-2 pr-4">Host group</th>
										<th className="py-2 pr-4">On</th>
										<th className="py-2"> </th>
									</tr>
								</thead>
								<tbody>
									{routes.map((row) => (
										<tr
											key={row.id}
											className="border-b border-secondary-100 dark:border-secondary-700"
										>
											<td className="py-2 pr-4">
												{row.destination_display_name || row.destination_id}
											</td>
											<td className="py-2 pr-4">
												{row.event_type === "*" ? "All events" : row.event_type}
											</td>
											<td className="py-2 pr-4">{row.min_severity}</td>
											<td className="py-2 pr-4 text-xs">
												{row.host_group_id
													? hostGroupNameMap[row.host_group_id] ||
														row.host_group_id
													: " -"}
											</td>
											<td className="py-2 pr-4">
												{row.enabled ? "yes" : "no"}
											</td>
											<td className="py-2">
												<button
													type="button"
													className="text-primary-600 text-xs mr-2"
													onClick={() => startEditRoute(row)}
												>
													Edit
												</button>
												<button
													type="button"
													className="text-red-600 text-xs"
													onClick={() => {
														if (confirm("Delete this route?")) {
															deleteRoute.mutate(row.id);
														}
													}}
												>
													Delete
												</button>
											</td>
										</tr>
									))}
								</tbody>
							</table>
						</div>
					)}

					<form
						onSubmit={submitRoute}
						className="space-y-3 border-t border-secondary-200 dark:border-secondary-600 pt-4"
					>
						<p className="text-sm font-medium text-secondary-800 dark:text-secondary-200">
							{editingRouteId ? "Edit route" : "Add route"}
						</p>
						<div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
							<label className="block text-sm">
								<span className="text-secondary-700 dark:text-secondary-300">
									Destination
								</span>
								<select
									className="mt-1 w-full rounded-md border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-900 px-2 py-2 text-sm"
									value={routeForm.destination_id}
									onChange={(e) =>
										setRouteForm((p) => ({
											...p,
											destination_id: e.target.value,
										}))
									}
								>
									<option value="">Select…</option>
									{destinations.map((d) => (
										<option key={d.id} value={d.id}>
											{d.display_name} ({d.channel_type})
										</option>
									))}
								</select>
							</label>
							<label className="block text-sm">
								<span className="text-secondary-700 dark:text-secondary-300">
									Event type
								</span>
								<select
									className="mt-1 w-full rounded-md border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-900 px-2 py-2 text-sm"
									value={routeForm.event_type}
									onChange={(e) =>
										setRouteForm((p) => ({
											...p,
											event_type: e.target.value,
										}))
									}
								>
									{EVENT_TYPES.map((o) => (
										<option key={o.value} value={o.value}>
											{o.label}
										</option>
									))}
								</select>
							</label>
							<label className="block text-sm">
								<span className="text-secondary-700 dark:text-secondary-300">
									Minimum severity
								</span>
								<select
									className="mt-1 w-full rounded-md border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-900 px-2 py-2 text-sm"
									value={routeForm.min_severity}
									onChange={(e) =>
										setRouteForm((p) => ({
											...p,
											min_severity: e.target.value,
										}))
									}
								>
									{SEVERITIES.map((o) => (
										<option key={o.value} value={o.value}>
											{o.label}
										</option>
									))}
								</select>
							</label>
							<label className="block text-sm">
								<span className="text-secondary-700 dark:text-secondary-300">
									Host group (optional)
								</span>
								<select
									className="mt-1 w-full rounded-md border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-900 px-2 py-2 text-sm"
									value={routeForm.host_group_id}
									onChange={(e) =>
										setRouteForm((p) => ({
											...p,
											host_group_id: e.target.value,
										}))
									}
									disabled={!canListHostGroups}
								>
									<option value="">All hosts</option>
									{hostGroupOptions.map((g) => (
										<option key={g.id} value={g.id}>
											{g.name || g.id}
										</option>
									))}
								</select>
							</label>
						</div>
						<label className="flex items-center gap-2 text-sm">
							<input
								type="checkbox"
								checked={routeForm.enabled}
								onChange={(e) =>
									setRouteForm((p) => ({ ...p, enabled: e.target.checked }))
								}
							/>
							Enabled
						</label>
						<div className="flex gap-2">
							<button
								type="submit"
								className="inline-flex items-center gap-2 px-4 py-2 rounded-md bg-primary-600 text-white text-sm hover:bg-primary-700"
								disabled={createRoute.isPending || updateRoute.isPending}
							>
								<Plus className="h-4 w-4" />
								{editingRouteId ? "Save route" : "Add route"}
							</button>
							{editingRouteId && (
								<button
									type="button"
									className="px-4 py-2 text-sm rounded-md border border-secondary-300 dark:border-secondary-600"
									onClick={() => {
										setEditingRouteId(null);
										setRouteForm({
											destination_id: "",
											event_type: "host_down",
											min_severity: "informational",
											host_group_id: "",
											enabled: true,
										});
									}}
								>
									Cancel
								</button>
							)}
						</div>
					</form>
				</section>
			)}

			{canManage && (
				<section className={`${cardClass} p-6 space-y-4`}>
					<div className="flex items-center justify-between">
						<h2 className="text-lg font-semibold text-secondary-900 dark:text-white">
							Scheduled reports
						</h2>
						{reportsLoading && <Loader2 className="h-5 w-5 animate-spin" />}
					</div>
					<p className="text-sm text-secondary-600 dark:text-secondary-300">
						Cron in server timezone field (IANA, e.g. UTC, America/New_York).
						Reports are emailed or sent via webhook according to each
						destination&apos;s channel.
					</p>

					{scheduledReports.length > 0 && (
						<ul className="divide-y divide-secondary-200 dark:divide-secondary-600">
							{scheduledReports.map((r) => (
								<li
									key={r.id}
									className="py-3 flex flex-wrap justify-between gap-2"
								>
									<div>
										<p className="font-medium text-secondary-900 dark:text-white">
											{r.name}
										</p>
										<p className="text-xs text-secondary-500">
											{r.cron_expr} · {r.timezone}
											{r.enabled ? "" : " · disabled"}
											{describeCron(r.cron_expr)
												? ` · ${describeCron(r.cron_expr)}`
												: ""}
										</p>
										{r.next_run_at && (
											<p className="text-xs text-secondary-500">
												Next: {formatRelativeTime(r.next_run_at)}
											</p>
										)}
									</div>
									<div className="flex gap-2 items-center">
										<button
											type="button"
											className="text-sm text-primary-600 inline-flex items-center gap-1"
											onClick={() => runReportNow.mutate(r.id)}
											disabled={runReportNow.isPending || !r.enabled}
											title={
												!r.enabled
													? "Enable the report first"
													: "Run this report now"
											}
										>
											<Play className="h-3.5 w-3.5" />
											Run now
										</button>
										<button
											type="button"
											className="text-sm text-primary-600"
											onClick={() => startEditReport(r)}
										>
											Edit
										</button>
										<button
											type="button"
											className="text-sm text-red-600"
											onClick={() => {
												if (confirm("Delete this scheduled report?")) {
													deleteReport.mutate(r.id);
												}
											}}
										>
											Delete
										</button>
									</div>
								</li>
							))}
						</ul>
					)}

					<form
						onSubmit={submitReport}
						className="space-y-3 border-t border-secondary-200 dark:border-secondary-600 pt-4"
					>
						<p className="text-sm font-medium text-secondary-800 dark:text-secondary-200">
							{editingReportId
								? "Edit scheduled report"
								: "New scheduled report"}
						</p>
						<div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
							<label className="block text-sm">
								<span className="text-secondary-700 dark:text-secondary-300">
									Name
								</span>
								<input
									className="mt-1 w-full rounded-md border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-900 px-2 py-2 text-sm"
									value={reportForm.name}
									onChange={(e) =>
										setReportForm((p) => ({ ...p, name: e.target.value }))
									}
								/>
							</label>
							<div className="block text-sm">
								<label>
									<span className="text-secondary-700 dark:text-secondary-300">
										Cron
									</span>
									<input
										className="mt-1 w-full font-mono text-sm rounded-md border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-900 px-2 py-2"
										placeholder="0 8 * * *"
										value={reportForm.cron_expr}
										onChange={(e) =>
											setReportForm((p) => ({
												...p,
												cron_expr: e.target.value,
											}))
										}
									/>
								</label>
								{cronPreview && (
									<p className="mt-1 text-xs text-secondary-500 flex items-center gap-1">
										<Clock className="h-3 w-3" />
										{cronPreview}{" "}
										{reportForm.timezone ? `(${reportForm.timezone})` : ""}
									</p>
								)}
							</div>
							<label className="block text-sm">
								<span className="text-secondary-700 dark:text-secondary-300">
									Timezone
								</span>
								<input
									className="mt-1 w-full rounded-md border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-900 px-2 py-2 text-sm"
									value={reportForm.timezone}
									onChange={(e) =>
										setReportForm((p) => ({
											...p,
											timezone: e.target.value,
										}))
									}
								/>
							</label>
							<label className="block text-sm">
								<span className="text-secondary-700 dark:text-secondary-300">
									Top rows per section
								</span>
								<input
									type="number"
									min={1}
									className="mt-1 w-full rounded-md border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-900 px-2 py-2 text-sm"
									value={reportForm.top_hosts}
									onChange={(e) =>
										setReportForm((p) => ({
											...p,
											top_hosts: e.target.value,
										}))
									}
								/>
							</label>
						</div>
						<label className="flex items-center gap-2 text-sm">
							<input
								type="checkbox"
								checked={reportForm.enabled}
								onChange={(e) =>
									setReportForm((p) => ({
										...p,
										enabled: e.target.checked,
									}))
								}
							/>
							Enabled
						</label>
						<div>
							<p className="text-sm text-secondary-700 dark:text-secondary-300 mb-2">
								Sections
							</p>
							<div className="flex flex-wrap gap-3">
								{REPORT_SECTIONS.map((s) => (
									<label
										key={s.id}
										className="inline-flex items-center gap-2 text-sm"
									>
										<input
											type="checkbox"
											checked={reportForm.sections.includes(s.id)}
											onChange={() => toggleSection(s.id)}
										/>
										{s.label}
									</label>
								))}
							</div>
						</div>
						<div>
							<p className="text-sm text-secondary-700 dark:text-secondary-300 mb-2">
								Deliver to destinations
							</p>
							<div className="flex flex-wrap gap-3">
								{destinations.length === 0 && (
									<p className="text-xs text-secondary-500">
										Add at least one destination above.
									</p>
								)}
								{destinations.map((d) => (
									<label
										key={d.id}
										className="inline-flex items-center gap-2 text-sm"
									>
										<input
											type="checkbox"
											checked={reportForm.destination_ids.includes(d.id)}
											onChange={() => toggleReportDestination(d.id)}
										/>
										{d.display_name}{" "}
										<span className="text-xs text-secondary-400">
											({d.channel_type})
										</span>
									</label>
								))}
							</div>
						</div>
						{canListHostGroups && hostGroupOptions.length > 0 && (
							<div>
								<p className="text-sm text-secondary-700 dark:text-secondary-300 mb-2">
									Scope to host groups (optional)
								</p>
								<div className="flex flex-wrap gap-3">
									{hostGroupOptions.map((g) => (
										<label
											key={g.id}
											className="inline-flex items-center gap-2 text-sm"
										>
											<input
												type="checkbox"
												checked={reportForm.host_group_ids.includes(g.id)}
												onChange={() => toggleReportHostGroup(g.id)}
											/>
											{g.name || g.id}
										</label>
									))}
								</div>
							</div>
						)}
						<div className="flex gap-2">
							<button
								type="submit"
								className="inline-flex items-center gap-2 px-4 py-2 rounded-md bg-primary-600 text-white text-sm hover:bg-primary-700"
								disabled={createReport.isPending || updateReport.isPending}
							>
								<Bell className="h-4 w-4" />
								{editingReportId ? "Save report" : "Create report"}
							</button>
							{editingReportId && (
								<button
									type="button"
									className="px-4 py-2 text-sm rounded-md border border-secondary-300 dark:border-secondary-600"
									onClick={() => {
										setEditingReportId(null);
										setReportForm({
											name: "",
											cron_expr: "0 8 * * *",
											timezone: "UTC",
											enabled: true,
											destination_ids: [],
											sections: [
												"executive_summary",
												"compliance_summary",
												"recent_patch_runs",
											],
											host_group_ids: [],
											top_hosts: 20,
										});
									}}
								>
									Cancel
								</button>
							)}
						</div>
					</form>
				</section>
			)}

			{canLog && (
				<section className={`${cardClass} p-6 space-y-4`}>
					<div className="flex items-center justify-between">
						<h2 className="text-lg font-semibold text-secondary-900 dark:text-white">
							Delivery log
						</h2>
						{logLoading && <Loader2 className="h-5 w-5 animate-spin" />}
					</div>
					{deliveryLog.length === 0 && !logLoading ? (
						<p className="text-sm text-secondary-500">
							No delivery entries yet.
						</p>
					) : (
						<>
							<div className="overflow-x-auto max-h-[480px] overflow-y-auto">
								<table className="min-w-full text-xs">
									<thead className="sticky top-0 bg-white dark:bg-secondary-800">
										<tr className="text-left border-b border-secondary-200 dark:border-secondary-600">
											<th className="py-2 pr-2">Time</th>
											<th className="py-2 pr-2">Status</th>
											<th className="py-2 pr-2">Event</th>
											<th className="py-2 pr-2">Destination</th>
											<th className="py-2 pr-2">Ref</th>
											<th className="py-2">Error</th>
										</tr>
									</thead>
									<tbody>
										{deliveryLog.map((row) => (
											<tr
												key={row.id}
												className="border-b border-secondary-100 dark:border-secondary-700 align-top"
											>
												<td
													className="py-2 pr-2 whitespace-nowrap"
													title={row.created_at || ""}
												>
													{row.created_at
														? formatRelativeTime(row.created_at)
														: " -"}
												</td>
												<td className="py-2 pr-2">
													<span
														className={`inline-block px-1.5 py-0.5 rounded text-xs font-medium ${
															row.status === "sent"
																? "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200"
																: "bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200"
														}`}
													>
														{row.status}
													</span>
												</td>
												<td className="py-2 pr-2">{row.event_type}</td>
												<td className="py-2 pr-2">
													{destNameMap[row.destination_id] ||
														row.destination_id}
												</td>
												<td className="py-2 pr-2">
													{row.reference_type}:
													{typeof row.reference_id === "string"
														? `${row.reference_id.slice(0, 8)}…`
														: " -"}
												</td>
												<td className="py-2 text-red-600 dark:text-red-400 max-w-xs break-words">
													{row.error_message || " -"}
												</td>
											</tr>
										))}
									</tbody>
								</table>
							</div>
							{/* Pagination controls */}
							<div className="flex items-center justify-between pt-2">
								<p className="text-xs text-secondary-500">
									Page {logPage + 1}
									{deliveryLog.length < logPageSize && logPage === 0
										? ` · ${deliveryLog.length} entries`
										: ""}
								</p>
								<div className="flex gap-2">
									<button
										type="button"
										className="px-3 py-1 text-xs rounded-md border border-secondary-300 dark:border-secondary-600 disabled:opacity-50"
										disabled={logPage === 0}
										onClick={() => setLogPage((p) => Math.max(0, p - 1))}
									>
										Previous
									</button>
									<button
										type="button"
										className="px-3 py-1 text-xs rounded-md border border-secondary-300 dark:border-secondary-600 disabled:opacity-50"
										disabled={deliveryLog.length < logPageSize}
										onClick={() => setLogPage((p) => p + 1)}
									>
										Next
									</button>
								</div>
							</div>
						</>
					)}
				</section>
			)}

			{!canManage && !canLog && (
				<div className={`${cardClass} p-8 text-center text-secondary-600`}>
					You don&apos;t have permission to view this page.
				</div>
			)}
		</div>
	);
};

export default AlertChannels;
