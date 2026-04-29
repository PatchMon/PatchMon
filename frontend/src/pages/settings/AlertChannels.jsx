import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
	Bell,
	Check,
	ChevronLeft,
	ChevronRight,
	Clock,
	Edit2,
	Globe,
	Loader2,
	Mail,
	Play,
	Plus,
	RefreshCw,
	Send,
	Trash2,
	X,
} from "lucide-react";
import { useMemo, useState } from "react";
import { SiDiscord, SiNtfy, SiSlack } from "react-icons/si";
import { Link } from "react-router-dom";
import { useAuth } from "../../contexts/AuthContext";
import { useToast } from "../../contexts/ToastContext";
import {
	adminHostsAPI,
	formatRelativeTime,
	hostGroupsAPI,
	notificationsAPI,
} from "../../utils/api";

/* ───────────────────── Constants ───────────────────── */

const EVENT_TYPES = [
	{ value: "*", label: "All events" },
	{ value: "host_down", label: "Host down" },
	{ value: "host_recovered", label: "Host recovered / up" },
	{ value: "host_enrolled", label: "Host enrolled" },
	{ value: "host_deleted", label: "Host deleted" },
	{ value: "server_update", label: "Server update" },
	{ value: "agent_update", label: "Agent update" },
	{ value: "patch_run_started", label: "Patch run started" },
	{ value: "patch_run_completed", label: "Patch run completed" },
	{ value: "patch_run_failed", label: "Patch run failed" },
	{ value: "patch_run_approved", label: "Patch run approved" },
	{ value: "patch_run_cancelled", label: "Patch run cancelled" },
	{ value: "patch_reboot_required", label: "Reboot required" },
	{ value: "compliance_scan_completed", label: "Compliance scan completed" },
	{ value: "compliance_scan_failed", label: "Compliance scan failed" },
	{ value: "container_stopped", label: "Container stopped" },
	{ value: "container_started", label: "Container started" },
	{
		value: "container_image_update_available",
		label: "Container image update available",
	},
	{ value: "ssh_session_started", label: "SSH session started" },
	{ value: "rdp_session_started", label: "RDP session started" },
	{
		value: "host_security_updates_exceeded",
		label: "Security updates threshold exceeded",
	},
	{
		value: "host_pending_updates_exceeded",
		label: "Pending updates threshold exceeded",
	},
	{ value: "user_login", label: "User login" },
	{ value: "user_login_failed", label: "Failed login attempt" },
	{ value: "account_locked", label: "Account locked" },
	{ value: "user_created", label: "User created" },
	{ value: "user_role_changed", label: "User role changed" },
	{ value: "user_tfa_disabled", label: "2FA disabled" },
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
	{ id: "open_alerts", label: "Open alerts" },
	{ id: "hosts_by_updates", label: "Hosts by outstanding updates" },
	{ id: "top_security_packages", label: "Top outdated security packages" },
];

const CHANNEL_TYPES = [
	{
		value: "webhook",
		label: "Webhook",
		description: "Generic, Discord, or Slack",
		icon: Globe,
		brandIcons: { discord: SiDiscord, slack: SiSlack },
	},
	{
		value: "email",
		label: "Email",
		description: "SMTP delivery",
		icon: Mail,
	},
	{
		value: "ntfy",
		label: "ntfy",
		description: "Push notifications via ntfy.sh",
		icon: SiNtfy,
	},
	{
		value: "internal",
		label: "Internal Alerts",
		description: "Alert records in the Alerts tab",
		icon: Bell,
	},
];

const FREQUENCY_OPTIONS = [
	{ value: "daily", label: "Daily" },
	{ value: "weekdays", label: "Weekdays (Mon-Fri)" },
	{ value: "weekly", label: "Weekly" },
	{ value: "monthly", label: "Monthly" },
];

const MONTH_DAY_PRESETS = [
	{ value: "1", label: "1st" },
	{ value: "15", label: "15th" },
	{ value: "L", label: "Last day" },
];

const DAY_LABELS = [
	{ value: "1", short: "Mon" },
	{ value: "2", short: "Tue" },
	{ value: "3", short: "Wed" },
	{ value: "4", short: "Thu" },
	{ value: "5", short: "Fri" },
	{ value: "6", short: "Sat" },
	{ value: "0", short: "Sun" },
];

const TLS_MODES = [
	{ value: "starttls", label: "STARTTLS (recommended)" },
	{ value: "tls", label: "Implicit TLS / SSL" },
	{ value: "none", label: "None (insecure)" },
	{ value: "auto", label: "Auto" },
];

const TLS_MODE_HELP = {
	starttls:
		"Connect in plaintext on the submission port, then upgrade to TLS via the STARTTLS command. Typical port: 587.",
	tls: "Open a TLS connection from the start (sometimes called SMTPS). Typical port: 465.",
	none: "Send mail in plaintext. Credentials will be rejected by the server. Use only for local relays you trust.",
	auto: "Pick STARTTLS, implicit TLS, or plaintext automatically based on the port and what the server advertises.",
};

const TLS_MODE_DEFAULT_PORTS = {
	starttls: 587,
	tls: 465,
	none: 25,
};

const KNOWN_SMTP_PORTS = new Set([25, 465, 587, 2525]);

const hydrateTLSMode = (cfg) => {
	if (!cfg || typeof cfg !== "object") return "starttls";
	if (
		typeof cfg.tls_mode === "string" &&
		TLS_MODES.some((m) => m.value === cfg.tls_mode)
	) {
		return cfg.tls_mode;
	}
	if (cfg.use_tls === false) return "none";
	return "auto";
};

const buildCron = (frequency, time, days, monthDay) => {
	const [h, m] = (time || "08:00").split(":");
	const hour = Number.parseInt(h, 10) || 0;
	const minute = Number.parseInt(m, 10) || 0;
	switch (frequency) {
		case "weekdays":
			return `${minute} ${hour} * * 1-5`;
		case "weekly":
			return `${minute} ${hour} * * ${days.length > 0 ? days.join(",") : "1"}`;
		case "monthly":
			return `${minute} ${hour} ${monthDay || "1"} * *`;
		default:
			return `${minute} ${hour} * * *`;
	}
};

const describeSchedule = (expr) => {
	if (!expr) return "";
	const parts = expr.trim().split(/\s+/);
	if (parts.length !== 5) return expr;
	const [min, hour, dom, , dow] = parts;
	const h = Number.parseInt(hour, 10);
	const m = Number.parseInt(min, 10);
	const time =
		!Number.isNaN(h) && !Number.isNaN(m)
			? `${String(h).padStart(2, "0")}:${String(m).padStart(2, "0")}`
			: null;
	if (!time) return expr;
	if (dom === "*" && dow === "*") return `Daily at ${time}`;
	if (dom === "*" && dow === "1-5") return `Weekdays at ${time}`;
	if (dom !== "*" && dow === "*") {
		if (dom === "L") return `Last day of month at ${time}`;
		const ordinal =
			dom === "1" || dom === "21" || dom === "31"
				? "st"
				: dom === "2" || dom === "22"
					? "nd"
					: dom === "3" || dom === "23"
						? "rd"
						: "th";
		return `${dom}${ordinal} of month at ${time}`;
	}
	if (dom === "*" && dow && dow !== "*") {
		const dayNames = {
			0: "Sun",
			1: "Mon",
			2: "Tue",
			3: "Wed",
			4: "Thu",
			5: "Fri",
			6: "Sat",
		};
		const days = dow
			.split(",")
			.map((d) => dayNames[d] || d)
			.join(", ");
		return `${days} at ${time}`;
	}
	return expr;
};

const channelIcon = (type) => {
	const ct = CHANNEL_TYPES.find((c) => c.value === type);
	if (!ct) return null;
	const Icon = ct.icon;
	return <Icon className="h-4 w-4" />;
};

const INPUT =
	"w-full px-3 py-2 bg-white dark:bg-secondary-900 border border-secondary-300 dark:border-secondary-600 rounded-md text-sm text-secondary-900 dark:text-white focus:ring-2 focus:ring-primary-500 focus:border-primary-500 placeholder-secondary-400";
const SELECT = `${INPUT} appearance-none`;

const statusBadge = (status) => {
	const ok = status === "sent";
	return (
		<span
			className={`px-2 py-0.5 text-xs font-medium rounded-md ${ok ? "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200" : "bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200"}`}
		>
			{status}
		</span>
	);
};

/* ───────────────── Destination Modal ───────────────── */

const DestinationModal = ({
	isOpen,
	onClose,
	onSave,
	editingDest,
	isPending,
}) => {
	const [step, setStep] = useState(editingDest ? 2 : 1);
	const [channelType, setChannelType] = useState(
		editingDest?.channel_type || "",
	);
	const [displayName, setDisplayName] = useState(
		editingDest?.display_name || "",
	);
	const [enabled, setEnabled] = useState(editingDest?.enabled !== false);
	const [config, setConfig] = useState(editingDest?._loadedConfig || {});
	const [tlsMode, setTlsMode] = useState(
		editingDest ? hydrateTLSMode(editingDest._loadedConfig || {}) : "starttls",
	);
	const [isTestingSMTP, setIsTestingSMTP] = useState(false);
	const toast = useToast();

	if (!isOpen) return null;

	const credentialsSet = Boolean(
		(config.username && String(config.username).length > 0) ||
			(config.password && String(config.password).length > 0),
	);
	const showPlainAuthWarning =
		channelType === "email" && tlsMode === "none" && credentialsSet;

	const handleSave = () => {
		if (!displayName.trim()) {
			toast.warning("Display name is required");
			return;
		}
		if (channelType === "webhook" && !config.url) {
			toast.warning("Webhook URL is required");
			return;
		}
		if (
			channelType === "email" &&
			(!config.smtp_host || !config.from || !config.to)
		) {
			toast.warning("SMTP host, from, and to are required");
			return;
		}
		if (channelType === "email" && showPlainAuthWarning) {
			toast.warning(
				"Clear the credentials or pick STARTTLS / Implicit TLS before saving",
			);
			return;
		}
		if (channelType === "ntfy" && !config.topic) {
			toast.warning("Topic is required");
			return;
		}
		let outConfig = config;
		if (channelType === "email") {
			// Dual-write tls_mode (new) and use_tls (legacy) so the existing backend
			// read path keeps working for one release while the new mailer rolls out.
			outConfig = {
				...config,
				tls_mode: tlsMode,
				use_tls: tlsMode !== "none",
			};
		}
		onSave({
			channel_type: channelType,
			display_name: displayName.trim(),
			config: outConfig,
			enabled,
		});
	};

	const updateConfig = (key, value) =>
		setConfig((p) => ({ ...p, [key]: value }));

	const handleTLSModeChange = (nextMode) => {
		const prevDefault = TLS_MODE_DEFAULT_PORTS[tlsMode];
		const nextDefault = TLS_MODE_DEFAULT_PORTS[nextMode];
		setTlsMode(nextMode);
		const currentPortRaw = config.smtp_port;
		const currentPort =
			typeof currentPortRaw === "number"
				? currentPortRaw
				: Number(currentPortRaw);
		const portIsKnownDefault =
			!Number.isNaN(currentPort) &&
			KNOWN_SMTP_PORTS.has(currentPort) &&
			(prevDefault === undefined || currentPort === prevDefault);
		if (
			nextDefault !== undefined &&
			(currentPortRaw === undefined ||
				currentPortRaw === "" ||
				portIsKnownDefault)
		) {
			setConfig((p) => ({ ...p, smtp_port: nextDefault }));
		}
	};

	const handleSendTestEmail = async () => {
		if (!editingDest?.id) {
			toast.warning("Save the destination first to send a test email");
			return;
		}
		setIsTestingSMTP(true);
		try {
			const resp = await notificationsAPI.testSMTP(editingDest.id);
			const data = resp?.data || {};
			if (data.ok) {
				toast.success("Test email sent successfully");
			} else {
				const stage = data.stage ? `${data.stage} failed` : "Test failed";
				const message = data.message ? `: ${data.message}` : "";
				toast.error(`${stage}${message}`);
			}
		} catch (err) {
			const apiMsg =
				err?.response?.data?.message ||
				err?.response?.data?.error ||
				err?.message ||
				"Failed to send test email";
			toast.error(apiMsg);
		} finally {
			setIsTestingSMTP(false);
		}
	};

	const renderFields = () => {
		switch (channelType) {
			case "webhook":
				return (
					<div className="space-y-4">
						<div>
							<label className="block text-sm font-medium text-secondary-700 dark:text-white mb-1">
								Webhook URL <span className="text-danger-500">*</span>
							</label>
							<input
								className={INPUT}
								placeholder="https://hooks.slack.com/services/... or https://discord.com/api/webhooks/..."
								value={config.url || ""}
								onChange={(e) => updateConfig("url", e.target.value)}
							/>
							<p className="mt-1 text-xs text-secondary-500">
								Discord and Slack URLs are auto-detected for rich formatting
							</p>
						</div>
						<div>
							<label className="block text-sm font-medium text-secondary-700 dark:text-white mb-1">
								Signing secret
							</label>
							<input
								className={INPUT}
								type="password"
								placeholder="Optional HMAC signing secret"
								value={config.signing_secret || ""}
								onChange={(e) => updateConfig("signing_secret", e.target.value)}
							/>
						</div>
					</div>
				);
			case "email":
				return (
					<div className="space-y-4">
						<div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
							<div>
								<label className="block text-sm font-medium text-secondary-700 dark:text-white mb-1">
									SMTP host <span className="text-danger-500">*</span>
								</label>
								<input
									className={INPUT}
									placeholder="smtp.example.com"
									value={config.smtp_host || ""}
									onChange={(e) => updateConfig("smtp_host", e.target.value)}
								/>
							</div>
							<div>
								<label className="block text-sm font-medium text-secondary-700 dark:text-white mb-1">
									SMTP port
								</label>
								<input
									className={INPUT}
									type="number"
									placeholder="587"
									value={config.smtp_port || 587}
									onChange={(e) =>
										updateConfig("smtp_port", Number(e.target.value) || 587)
									}
								/>
							</div>
						</div>
						<div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
							<div>
								<label className="block text-sm font-medium text-secondary-700 dark:text-white mb-1">
									Username
								</label>
								<input
									className={INPUT}
									value={config.username || ""}
									onChange={(e) => updateConfig("username", e.target.value)}
								/>
							</div>
							<div>
								<label className="block text-sm font-medium text-secondary-700 dark:text-white mb-1">
									Password
								</label>
								<input
									className={INPUT}
									type="password"
									value={config.password || ""}
									onChange={(e) => updateConfig("password", e.target.value)}
								/>
							</div>
						</div>
						<div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
							<div>
								<label className="block text-sm font-medium text-secondary-700 dark:text-white mb-1">
									From <span className="text-danger-500">*</span>
								</label>
								<input
									className={INPUT}
									placeholder="noreply@example.com"
									value={config.from || ""}
									onChange={(e) => updateConfig("from", e.target.value)}
								/>
							</div>
							<div>
								<label className="block text-sm font-medium text-secondary-700 dark:text-white mb-1">
									To <span className="text-danger-500">*</span>
								</label>
								<input
									className={INPUT}
									placeholder="team@example.com"
									value={config.to || ""}
									onChange={(e) => updateConfig("to", e.target.value)}
								/>
							</div>
						</div>
						<div>
							<label className="block text-sm font-medium text-secondary-700 dark:text-white mb-1">
								TLS mode
							</label>
							<select
								className={`${SELECT} min-h-[44px]`}
								value={tlsMode}
								onChange={(e) => handleTLSModeChange(e.target.value)}
							>
								{TLS_MODES.map((m) => (
									<option key={m.value} value={m.value}>
										{m.label}
									</option>
								))}
							</select>
							<p className="mt-1 text-xs text-secondary-500">
								{TLS_MODE_HELP[tlsMode]}
							</p>
						</div>
						{showPlainAuthWarning && (
							<div className="bg-danger-50 dark:bg-danger-900/30 border border-danger-200 dark:border-danger-700 rounded-md p-3 text-sm text-danger-700 dark:text-danger-300">
								PLAIN authentication over an unencrypted connection will be
								rejected by the server. Either clear the credentials or choose
								STARTTLS / Implicit TLS.
							</div>
						)}
					</div>
				);
			case "ntfy":
				return (
					<div className="space-y-4">
						<div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
							<div>
								<label className="block text-sm font-medium text-secondary-700 dark:text-white mb-1">
									Server URL
								</label>
								<input
									className={INPUT}
									placeholder="https://ntfy.sh"
									value={config.server_url || ""}
									onChange={(e) => updateConfig("server_url", e.target.value)}
								/>
								<p className="mt-1 text-xs text-secondary-500">
									Leave empty for ntfy.sh
								</p>
							</div>
							<div>
								<label className="block text-sm font-medium text-secondary-700 dark:text-white mb-1">
									Topic <span className="text-danger-500">*</span>
								</label>
								<input
									className={INPUT}
									placeholder="patchmon-alerts"
									value={config.topic || ""}
									onChange={(e) => updateConfig("topic", e.target.value)}
								/>
							</div>
						</div>
						<div>
							<label className="block text-sm font-medium text-secondary-700 dark:text-white mb-1">
								Access token
							</label>
							<input
								className={INPUT}
								type="password"
								placeholder="Optional"
								value={config.token || ""}
								onChange={(e) => updateConfig("token", e.target.value)}
							/>
						</div>
						<div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
							<div>
								<label className="block text-sm font-medium text-secondary-700 dark:text-white mb-1">
									Username
								</label>
								<input
									className={INPUT}
									placeholder="Optional basic auth"
									value={config.username || ""}
									onChange={(e) => updateConfig("username", e.target.value)}
								/>
							</div>
							<div>
								<label className="block text-sm font-medium text-secondary-700 dark:text-white mb-1">
									Password
								</label>
								<input
									className={INPUT}
									type="password"
									value={config.password || ""}
									onChange={(e) => updateConfig("password", e.target.value)}
								/>
							</div>
						</div>
					</div>
				);
			default:
				return null;
		}
	};

	return (
		<div
			className="fixed inset-0 bg-black/50 flex items-center justify-center z-50"
			onClick={onClose}
		>
			<div
				className="bg-white dark:bg-secondary-800 rounded-lg shadow-xl max-w-lg w-full mx-4 relative z-10"
				onClick={(e) => e.stopPropagation()}
			>
				<div className="px-6 py-4 border-b border-secondary-200 dark:border-secondary-600 flex items-center justify-between">
					<h3 className="text-lg font-semibold text-secondary-900 dark:text-white">
						{editingDest
							? "Edit destination"
							: step === 1
								? "Choose channel"
								: "Configure destination"}
					</h3>
					<button
						type="button"
						onClick={onClose}
						className="text-secondary-400 hover:text-secondary-600 dark:hover:text-white"
					>
						<X className="h-5 w-5" />
					</button>
				</div>

				<div className="px-6 py-5">
					{step === 1 && !editingDest && (
						<div className="grid grid-cols-3 gap-4">
							{CHANNEL_TYPES.filter((ct) => ct.value !== "internal").map(
								(ct) => {
									const Icon = ct.icon;
									return (
										<button
											key={ct.value}
											type="button"
											className={`flex flex-col items-center justify-center p-6 rounded-lg border-2 transition-all ${
												channelType === ct.value
													? "border-primary-500 bg-primary-50 dark:bg-primary-900/30"
													: "border-secondary-300 dark:border-secondary-600 hover:border-primary-400"
											}`}
											onClick={() => setChannelType(ct.value)}
										>
											<Icon className="h-10 w-10 text-secondary-700 dark:text-secondary-200 mb-2" />
											<span className="text-sm font-medium text-secondary-900 dark:text-white">
												{ct.label}
											</span>
											<span className="text-xs text-secondary-500 mt-1 text-center">
												{ct.description}
											</span>
										</button>
									);
								},
							)}
						</div>
					)}

					{step === 2 && (
						<div className="space-y-5">
							<div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
								<div>
									<label className="block text-sm font-medium text-secondary-700 dark:text-white mb-1">
										Display name <span className="text-danger-500">*</span>
									</label>
									<input
										className={INPUT}
										placeholder="e.g. Ops Discord"
										value={displayName}
										onChange={(e) => setDisplayName(e.target.value)}
									/>
								</div>
								<div className="flex items-end pb-1">
									<label className="flex items-center gap-2 text-sm text-secondary-700 dark:text-white">
										<div
											className={`relative inline-flex h-5 w-9 items-center rounded-md transition-colors ${enabled ? "bg-primary-600 dark:bg-primary-500" : "bg-secondary-200 dark:bg-secondary-600"}`}
											onClick={() => setEnabled(!enabled)}
											onKeyDown={() => {}}
										>
											<span
												className={`inline-block h-3 w-3 transform rounded-md bg-white transition-transform ${enabled ? "translate-x-5" : "translate-x-1"}`}
											/>
										</div>
										Enabled
									</label>
								</div>
							</div>
							{renderFields()}
						</div>
					)}
				</div>

				<div className="px-6 py-4 border-t border-secondary-200 dark:border-secondary-600 flex justify-between">
					{step === 2 && !editingDest ? (
						<button
							type="button"
							className="btn-outline flex items-center gap-1"
							onClick={() => setStep(1)}
						>
							<ChevronLeft className="h-4 w-4" /> Back
						</button>
					) : (
						<div />
					)}
					<div className="flex flex-wrap gap-2">
						<button type="button" className="btn-outline" onClick={onClose}>
							Cancel
						</button>
						{step === 2 && channelType === "email" && (
							<button
								type="button"
								className="btn-outline flex items-center gap-1 min-h-[44px]"
								disabled={
									!editingDest?.id ||
									isPending ||
									isTestingSMTP ||
									showPlainAuthWarning
								}
								onClick={handleSendTestEmail}
								title={
									!editingDest?.id
										? "Save the destination first to send a test email"
										: "Send a test email using the saved configuration"
								}
							>
								{isTestingSMTP ? (
									<RefreshCw className="h-4 w-4 animate-spin" />
								) : (
									<Send className="h-4 w-4" />
								)}
								{isTestingSMTP ? "Sending..." : "Send test email"}
							</button>
						)}
						{step === 1 && (
							<button
								type="button"
								className="btn-primary"
								disabled={!channelType}
								onClick={() => setStep(2)}
							>
								Next <ChevronRight className="h-4 w-4 inline ml-1" />
							</button>
						)}
						{step === 2 && (
							<button
								type="button"
								className="btn-primary flex items-center gap-1"
								disabled={isPending || isTestingSMTP || showPlainAuthWarning}
								onClick={handleSave}
							>
								{isPending ? (
									<Loader2 className="h-4 w-4 animate-spin" />
								) : (
									<Check className="h-4 w-4" />
								)}
								{editingDest ? "Save" : "Create"}
							</button>
						)}
					</div>
				</div>
			</div>
		</div>
	);
};

/* ───────────────── Route Modal ───────────────── */

const RouteModal = ({
	isOpen,
	onClose,
	onSave,
	editingRoute,
	destinations,
	hostGroups,
	hosts,
	isPending,
}) => {
	const parseArr = (v) => (Array.isArray(v) ? v : []);
	const [form, setForm] = useState({
		destination_id: editingRoute?.destination_id || "",
		event_types:
			parseArr(editingRoute?.event_types).length > 0
				? parseArr(editingRoute.event_types)
				: ["*"],
		min_severity: editingRoute?.min_severity || "informational",
		host_group_ids: parseArr(editingRoute?.host_group_ids),
		host_ids: parseArr(editingRoute?.host_ids),
		enabled: editingRoute?.enabled !== false,
	});
	const toast = useToast();

	if (!isOpen) return null;

	const handleSave = () => {
		if (!form.destination_id) {
			toast.warning("Choose a destination");
			return;
		}
		onSave({
			destination_id: form.destination_id,
			event_types: form.event_types.length > 0 ? form.event_types : ["*"],
			min_severity: form.min_severity,
			host_group_ids: form.host_group_ids,
			host_ids: form.host_ids,
			enabled: form.enabled,
		});
	};

	const upd = (key, value) => setForm((p) => ({ ...p, [key]: value }));
	const toggleArr = (key, id) =>
		setForm((p) => ({
			...p,
			[key]: p[key].includes(id)
				? p[key].filter((x) => x !== id)
				: [...p[key], id],
		}));

	const allEventValues = EVENT_TYPES.filter((e) => e.value !== "*").map(
		(e) => e.value,
	);

	const toggleEvent = (value) => {
		if (value === "*") {
			// Toggle: if all selected, clear all; if not all, select all
			setForm((p) =>
				p.event_types.includes("*")
					? { ...p, event_types: [] }
					: { ...p, event_types: ["*"] },
			);
			return;
		}
		setForm((p) => {
			// If currently "all", expand to individual events then remove the clicked one
			const next = p.event_types.includes("*")
				? allEventValues.filter((v) => v !== value)
				: p.event_types.includes(value)
					? p.event_types.filter((x) => x !== value)
					: [...p.event_types, value];
			// If all individual events are selected, collapse back to wildcard
			if (next.length >= allEventValues.length)
				return { ...p, event_types: ["*"] };
			return { ...p, event_types: next.length > 0 ? next : ["*"] };
		});
	};

	const allEvents = form.event_types.includes("*");

	return (
		<div
			className="fixed inset-0 bg-black/50 flex items-center justify-center z-50"
			onClick={onClose}
		>
			<div
				className="bg-white dark:bg-secondary-800 rounded-lg shadow-xl max-w-lg w-full mx-4 relative z-10 max-h-[90vh] overflow-y-auto"
				onClick={(e) => e.stopPropagation()}
			>
				<div className="px-6 py-4 border-b border-secondary-200 dark:border-secondary-600 flex items-center justify-between sticky top-0 bg-white dark:bg-secondary-800 z-10">
					<h3 className="text-lg font-semibold text-secondary-900 dark:text-white">
						{editingRoute ? "Edit route" : "Add route"}
					</h3>
					<button
						type="button"
						onClick={onClose}
						className="text-secondary-400 hover:text-secondary-600 dark:hover:text-white"
					>
						<X className="h-5 w-5" />
					</button>
				</div>
				<div className="px-6 py-5 space-y-5">
					<div>
						<label className="block text-sm font-medium text-secondary-700 dark:text-white mb-1">
							Destination <span className="text-danger-500">*</span>
						</label>
						<select
							className={SELECT}
							value={form.destination_id}
							onChange={(e) => upd("destination_id", e.target.value)}
						>
							<option value="">Select destination...</option>
							{destinations.map((d) => (
								<option key={d.id} value={d.id}>
									{d.display_name} ({d.channel_type})
								</option>
							))}
						</select>
					</div>

					<div>
						<label className="block text-sm font-medium text-secondary-700 dark:text-white mb-2">
							Events
						</label>
						<div className="space-y-1.5">
							<label className="flex items-center gap-2 text-sm text-secondary-700 dark:text-white font-medium">
								<input
									type="checkbox"
									checked={allEvents}
									onChange={() => toggleEvent("*")}
								/>
								All events
							</label>
							<div className="grid grid-cols-2 gap-1.5 pl-4 pt-1">
								{EVENT_TYPES.filter((e) => e.value !== "*").map((o) => (
									<label
										key={o.value}
										className="flex items-center gap-2 text-sm text-secondary-700 dark:text-white"
									>
										<input
											type="checkbox"
											checked={allEvents || form.event_types.includes(o.value)}
											onChange={() => toggleEvent(o.value)}
										/>
										{o.label}
									</label>
								))}
							</div>
						</div>
					</div>

					<div>
						<label className="block text-sm font-medium text-secondary-700 dark:text-white mb-1">
							Minimum severity
						</label>
						<select
							className={SELECT}
							value={form.min_severity}
							onChange={(e) => upd("min_severity", e.target.value)}
						>
							{SEVERITIES.map((o) => (
								<option key={o.value} value={o.value}>
									{o.label}
								</option>
							))}
						</select>
					</div>

					{hostGroups.length > 0 && (
						<div>
							<label className="block text-sm font-medium text-secondary-700 dark:text-white mb-2">
								Host groups{" "}
								<span className="text-xs font-normal text-secondary-500">
									(optional, leave empty for all)
								</span>
							</label>
							<div className="space-y-1.5 max-h-40 overflow-y-auto">
								{hostGroups.map((g) => (
									<label
										key={g.id}
										className="flex items-center gap-2 text-sm text-secondary-700 dark:text-white"
									>
										<input
											type="checkbox"
											checked={form.host_group_ids.includes(g.id)}
											onChange={() => toggleArr("host_group_ids", g.id)}
										/>
										{g.name || g.id}
									</label>
								))}
							</div>
						</div>
					)}

					{hosts.length > 0 && (
						<div>
							<label className="block text-sm font-medium text-secondary-700 dark:text-white mb-2">
								Individual hosts{" "}
								<span className="text-xs font-normal text-secondary-500">
									(optional, leave empty for all)
								</span>
							</label>
							<div className="space-y-1.5 max-h-40 overflow-y-auto">
								{hosts.map((h) => (
									<label
										key={h.id}
										className="flex items-center gap-2 text-sm text-secondary-700 dark:text-white"
									>
										<input
											type="checkbox"
											checked={form.host_ids.includes(h.id)}
											onChange={() => toggleArr("host_ids", h.id)}
										/>
										{h.friendly_name || h.hostname || h.id}
									</label>
								))}
							</div>
						</div>
					)}

					<label className="flex items-center gap-2 text-sm text-secondary-700 dark:text-white">
						<input
							type="checkbox"
							checked={form.enabled}
							onChange={(e) => upd("enabled", e.target.checked)}
						/>
						Enabled
					</label>
				</div>
				<div className="px-6 py-4 border-t border-secondary-200 dark:border-secondary-600 flex justify-end gap-2 sticky bottom-0 bg-white dark:bg-secondary-800">
					<button type="button" className="btn-outline" onClick={onClose}>
						Cancel
					</button>
					<button
						type="button"
						className="btn-primary flex items-center gap-1"
						disabled={isPending}
						onClick={handleSave}
					>
						{isPending ? (
							<Loader2 className="h-4 w-4 animate-spin" />
						) : (
							<Check className="h-4 w-4" />
						)}
						{editingRoute ? "Save" : "Add"}
					</button>
				</div>
			</div>
		</div>
	);
};

/* ───────────────── Report Modal ───────────────── */

const ReportModal = ({
	isOpen,
	onClose,
	onSave,
	editingReport,
	destinations,
	hostGroups,
	isPending,
}) => {
	const defRow = editingReport?.definition || {};

	// Parse existing cron on init
	const parseCronInit = () => {
		let frequency = "daily";
		let time = "08:00";
		let days = ["1"];
		let monthDay = "1";
		if (editingReport?.cron_expr) {
			const parts = editingReport.cron_expr.trim().split(/\s+/);
			if (parts.length === 5) {
				const [min, hour, dom, , dow] = parts;
				const h = Number.parseInt(hour, 10);
				const m = Number.parseInt(min, 10);
				if (!Number.isNaN(h) && !Number.isNaN(m)) {
					time = `${String(h).padStart(2, "0")}:${String(m).padStart(2, "0")}`;
				}
				if (dow === "1-5") frequency = "weekdays";
				else if (dom !== "*") {
					frequency = "monthly";
					monthDay = dom;
				} else if (dow && dow !== "*") {
					frequency = "weekly";
					days = dow.split(",");
				}
			}
		}
		return { frequency, time, days, monthDay };
	};
	const cronInit = parseCronInit();

	const [form, setForm] = useState({
		name: editingReport?.name || "",
		frequency: cronInit.frequency,
		time: cronInit.time,
		days: cronInit.days,
		monthDay: cronInit.monthDay,
		enabled: editingReport?.enabled !== false,
		destination_ids: Array.isArray(editingReport?.destination_ids)
			? editingReport.destination_ids
			: [],
		sections:
			Array.isArray(defRow.sections) && defRow.sections.length > 0
				? defRow.sections
				: ["executive_summary", "compliance_summary", "recent_patch_runs"],
		host_group_ids: Array.isArray(defRow.host_group_ids)
			? defRow.host_group_ids
			: [],
		top_hosts: defRow.limits?.top_hosts ?? 20,
	});
	const toast = useToast();

	if (!isOpen) return null;

	const upd = (key, value) => setForm((p) => ({ ...p, [key]: value }));
	const toggleArr = (key, id) =>
		setForm((p) => ({
			...p,
			[key]: p[key].includes(id)
				? p[key].filter((x) => x !== id)
				: [...p[key], id],
		}));

	const toggleDay = (d) =>
		setForm((p) => ({
			...p,
			days: p.days.includes(d) ? p.days.filter((x) => x !== d) : [...p.days, d],
		}));

	const handleSave = () => {
		if (!form.name.trim()) {
			toast.warning("Report name is required");
			return;
		}
		if (form.frequency === "weekly" && form.days.length === 0) {
			toast.warning("Select at least one day");
			return;
		}
		const cronExpr = buildCron(
			form.frequency,
			form.time,
			form.days,
			form.monthDay,
		);
		onSave({
			name: form.name.trim(),
			cron_expr: cronExpr,
			enabled: form.enabled,
			definition: {
				version: 1,
				sections: form.sections,
				host_group_ids: form.host_group_ids,
				limits: { top_hosts: Number(form.top_hosts) || 20 },
			},
			destination_ids: form.destination_ids,
		});
	};

	return (
		<div
			className="fixed inset-0 bg-black/50 flex items-center justify-center z-50"
			onClick={onClose}
		>
			<div
				className="bg-white dark:bg-secondary-800 rounded-lg shadow-xl max-w-lg w-full mx-4 relative z-10 max-h-[90vh] overflow-y-auto"
				onClick={(e) => e.stopPropagation()}
			>
				<div className="px-6 py-4 border-b border-secondary-200 dark:border-secondary-600 flex items-center justify-between sticky top-0 bg-white dark:bg-secondary-800 z-10">
					<h3 className="text-lg font-semibold text-secondary-900 dark:text-white">
						{editingReport ? "Edit report" : "New scheduled report"}
					</h3>
					<button
						type="button"
						onClick={onClose}
						className="text-secondary-400 hover:text-secondary-600 dark:hover:text-white"
					>
						<X className="h-5 w-5" />
					</button>
				</div>
				<div className="px-6 py-5 space-y-5">
					<div>
						<label className="block text-sm font-medium text-secondary-700 dark:text-white mb-1">
							Report name <span className="text-danger-500">*</span>
						</label>
						<input
							className={INPUT}
							placeholder="Weekly ops report"
							value={form.name}
							onChange={(e) => upd("name", e.target.value)}
						/>
					</div>

					<div>
						<label className="block text-sm font-medium text-secondary-700 dark:text-white mb-2">
							Schedule
						</label>
						<div className="flex flex-wrap gap-3 items-center">
							<select
								className={`${SELECT} w-auto`}
								value={form.frequency}
								onChange={(e) => upd("frequency", e.target.value)}
							>
								{FREQUENCY_OPTIONS.map((p) => (
									<option key={p.value} value={p.value}>
										{p.label}
									</option>
								))}
							</select>
							<span className="text-sm text-secondary-500">at</span>
							<input
								type="time"
								className={`${INPUT} w-auto`}
								value={form.time}
								onChange={(e) => upd("time", e.target.value)}
							/>
						</div>
						{form.frequency === "weekly" && (
							<div className="flex gap-1.5 mt-3">
								{DAY_LABELS.map((d) => (
									<button
										key={d.value}
										type="button"
										className={`px-3 py-1.5 text-xs font-medium rounded-md border transition-colors ${
											form.days.includes(d.value)
												? "bg-primary-600 text-white border-primary-600"
												: "bg-white dark:bg-secondary-900 text-secondary-700 dark:text-secondary-300 border-secondary-300 dark:border-secondary-600 hover:border-primary-400"
										}`}
										onClick={() => toggleDay(d.value)}
									>
										{d.short}
									</button>
								))}
							</div>
						)}
						{form.frequency === "monthly" && (
							<div className="mt-3 space-y-2">
								<div className="flex gap-1.5 flex-wrap">
									{MONTH_DAY_PRESETS.map((p) => (
										<button
											key={p.value}
											type="button"
											className={`px-3 py-1.5 text-xs font-medium rounded-md border transition-colors ${
												form.monthDay === p.value
													? "bg-primary-600 text-white border-primary-600"
													: "bg-white dark:bg-secondary-900 text-secondary-700 dark:text-secondary-300 border-secondary-300 dark:border-secondary-600 hover:border-primary-400"
											}`}
											onClick={() => upd("monthDay", p.value)}
										>
											{p.label}
										</button>
									))}
									<span className="text-sm text-secondary-500 self-center px-1">
										or
									</span>
									<input
										type="number"
										min={1}
										max={31}
										placeholder="Day"
										className={`${INPUT} w-20 text-center`}
										value={
											!["1", "15", "L"].includes(form.monthDay)
												? form.monthDay
												: ""
										}
										onChange={(e) => {
											const v = e.target.value;
											if (v === "") return;
											const n = Math.max(1, Math.min(31, Number(v) || 1));
											upd("monthDay", String(n));
										}}
										onFocus={() => {
											if (["1", "15", "L"].includes(form.monthDay))
												upd("monthDay", "");
										}}
									/>
								</div>
							</div>
						)}
						<p className="mt-2 text-xs text-secondary-500 flex items-center gap-1">
							<Clock className="h-3 w-3" /> Server timezone
						</p>
					</div>

					<div>
						<label className="block text-sm font-medium text-secondary-700 dark:text-white mb-2">
							Sections
						</label>
						<div className="grid grid-cols-2 gap-2">
							{REPORT_SECTIONS.map((s) => (
								<label
									key={s.id}
									className="flex items-center gap-2 text-sm text-secondary-700 dark:text-white"
								>
									<input
										type="checkbox"
										checked={form.sections.includes(s.id)}
										onChange={() => toggleArr("sections", s.id)}
									/>
									{s.label}
								</label>
							))}
						</div>
					</div>

					<div>
						<label className="block text-sm font-medium text-secondary-700 dark:text-white mb-2">
							Deliver to
						</label>
						{destinations.length === 0 ? (
							<p className="text-xs text-secondary-500">
								Add a destination first.
							</p>
						) : (
							<div className="space-y-1.5">
								{destinations.map((d) => (
									<label
										key={d.id}
										className="flex items-center gap-2 text-sm text-secondary-700 dark:text-white"
									>
										<input
											type="checkbox"
											checked={form.destination_ids.includes(d.id)}
											onChange={() => toggleArr("destination_ids", d.id)}
										/>
										{channelIcon(d.channel_type)}
										{d.display_name}
									</label>
								))}
							</div>
						)}
					</div>

					{hostGroups.length > 0 && (
						<div>
							<label className="block text-sm font-medium text-secondary-700 dark:text-white mb-2">
								Scope to host groups
							</label>
							<div className="space-y-1.5">
								{hostGroups.map((g) => (
									<label
										key={g.id}
										className="flex items-center gap-2 text-sm text-secondary-700 dark:text-white"
									>
										<input
											type="checkbox"
											checked={form.host_group_ids.includes(g.id)}
											onChange={() => toggleArr("host_group_ids", g.id)}
										/>
										{g.name || g.id}
									</label>
								))}
							</div>
						</div>
					)}

					<div className="grid grid-cols-2 gap-4">
						<div>
							<label className="block text-sm font-medium text-secondary-700 dark:text-white mb-1">
								Top rows per section
							</label>
							<input
								className={INPUT}
								type="number"
								min={1}
								value={form.top_hosts}
								onChange={(e) => upd("top_hosts", e.target.value)}
							/>
						</div>
						<div className="flex items-end pb-1">
							<label className="flex items-center gap-2 text-sm text-secondary-700 dark:text-white">
								<input
									type="checkbox"
									checked={form.enabled}
									onChange={(e) => upd("enabled", e.target.checked)}
								/>
								Enabled
							</label>
						</div>
					</div>
				</div>
				<div className="px-6 py-4 border-t border-secondary-200 dark:border-secondary-600 flex justify-end gap-2 sticky bottom-0 bg-white dark:bg-secondary-800">
					<button type="button" className="btn-outline" onClick={onClose}>
						Cancel
					</button>
					<button
						type="button"
						className="btn-primary flex items-center gap-1"
						disabled={isPending}
						onClick={handleSave}
					>
						{isPending ? (
							<Loader2 className="h-4 w-4 animate-spin" />
						) : (
							<Check className="h-4 w-4" />
						)}
						{editingReport ? "Save" : "Create"}
					</button>
				</div>
			</div>
		</div>
	);
};

/* ───────────────── Main Page ───────────────── */

/** Renders notification management for a specific panel. Used by Reporting page tabs. */
export const NotificationPanel = ({ panel }) => {
	const queryClient = useQueryClient();
	const toast = useToast();
	const { canManageNotifications, canViewNotificationLogs, hasPermission } =
		useAuth();
	const canManage = canManageNotifications();
	const canLog = canViewNotificationLogs();
	const canListHostGroups = hasPermission("can_view_hosts");

	// Modal states
	const [destModal, setDestModal] = useState({ open: false, editing: null });
	const [routeModal, setRouteModal] = useState({ open: false, editing: null });
	const [reportModal, setReportModal] = useState({
		open: false,
		editing: null,
	});
	const [logPage, setLogPage] = useState(0);
	const logPageSize = 50;

	// Queries
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
	const { data: hostsList = [] } = useQuery({
		queryKey: ["hosts-list"],
		queryFn: () => adminHostsAPI.list().then((r) => r.data ?? []),
		enabled: canManage && canListHostGroups,
	});

	const hostGroupOptions = useMemo(
		() => (Array.isArray(hostGroups) ? hostGroups : []),
		[hostGroups],
	);
	const destNameMap = useMemo(() => {
		const m = {};
		for (const d of destinations) m[d.id] = d.display_name;
		return m;
	}, [destinations]);
	const hostGroupNameMap = useMemo(() => {
		const m = {};
		for (const g of hostGroupOptions) m[g.id] = g.name || g.id;
		return m;
	}, [hostGroupOptions]);

	const invalidate = () =>
		queryClient.invalidateQueries({ queryKey: ["notifications"] });

	// Mutations
	const createDest = useMutation({
		mutationFn: (body) => notificationsAPI.createDestination(body),
		onSuccess: () => {
			invalidate();
			toast.success("Destination created");
			setDestModal({ open: false, editing: null });
		},
		onError: (err) =>
			toast.error(err.response?.data?.error || "Failed to create"),
	});
	const updateDest = useMutation({
		mutationFn: ({ id, body }) => notificationsAPI.updateDestination(id, body),
		onSuccess: () => {
			invalidate();
			toast.success("Destination updated");
			setDestModal({ open: false, editing: null });
		},
		onError: (err) =>
			toast.error(err.response?.data?.error || "Failed to update"),
	});
	const deleteDest = useMutation({
		mutationFn: (id) => notificationsAPI.deleteDestination(id),
		onSuccess: () => {
			invalidate();
			toast.success("Destination deleted");
		},
		onError: (err) =>
			toast.error(err.response?.data?.error || "Failed to delete"),
	});
	const testNotify = useMutation({
		mutationFn: (destination_id) => notificationsAPI.test({ destination_id }),
	});

	const createRoute = useMutation({
		mutationFn: (body) => notificationsAPI.createRoute(body),
		onSuccess: () => {
			invalidate();
			toast.success("Route created");
			setRouteModal({ open: false, editing: null });
		},
		onError: (err) =>
			toast.error(err.response?.data?.error || "Failed to create"),
	});
	const updateRoute = useMutation({
		mutationFn: ({ id, body }) => notificationsAPI.updateRoute(id, body),
		onSuccess: () => {
			invalidate();
			toast.success("Route updated");
			setRouteModal({ open: false, editing: null });
		},
		onError: (err) =>
			toast.error(err.response?.data?.error || "Failed to update"),
	});
	const deleteRoute = useMutation({
		mutationFn: (id) => notificationsAPI.deleteRoute(id),
		onSuccess: () => {
			invalidate();
			toast.success("Route deleted");
		},
		onError: (err) =>
			toast.error(err.response?.data?.error || "Failed to delete"),
	});

	const createReport = useMutation({
		mutationFn: (body) => notificationsAPI.createScheduledReport(body),
		onSuccess: () => {
			invalidate();
			toast.success("Report created");
			setReportModal({ open: false, editing: null });
		},
		onError: (err) =>
			toast.error(err.response?.data?.error || "Failed to create"),
	});
	const updateReport = useMutation({
		mutationFn: ({ id, body }) =>
			notificationsAPI.updateScheduledReport(id, body),
		onSuccess: () => {
			invalidate();
			toast.success("Report updated");
			setReportModal({ open: false, editing: null });
		},
		onError: (err) =>
			toast.error(err.response?.data?.error || "Failed to update"),
	});
	const deleteReport = useMutation({
		mutationFn: (id) => notificationsAPI.deleteScheduledReport(id),
		onSuccess: () => {
			invalidate();
			toast.success("Report deleted");
		},
		onError: (err) =>
			toast.error(err.response?.data?.error || "Failed to delete"),
	});
	const runReportNow = useMutation({
		mutationFn: (id) => notificationsAPI.runScheduledReportNow(id),
		onSuccess: () => {
			invalidate();
			toast.success("Report scheduled for immediate delivery");
		},
		onError: (err) => toast.error(err.response?.data?.error || "Failed to run"),
	});

	const sendTest = (id) => {
		testNotify.mutate(id, {
			onSuccess: () => {
				toast.info("Test notification enqueued");
				setTimeout(
					() =>
						queryClient.invalidateQueries({
							queryKey: ["notifications", "delivery-log"],
						}),
					3000,
				);
			},
			onError: (err) =>
				toast.error(err.response?.data?.error || err.message || "Test failed"),
		});
	};

	const openEditDest = async (d) => {
		let loadedConfig = {};
		if (d.has_secret) {
			try {
				const resp = await notificationsAPI.getDestinationConfig(d.id);
				loadedConfig = resp.data;
			} catch {
				/* fallback to empty */
			}
		}
		setDestModal({
			open: true,
			editing: { ...d, _loadedConfig: loadedConfig },
		});
	};

	const handleDestSave = (data) => {
		if (destModal.editing) {
			updateDest.mutate({ id: destModal.editing.id, body: data });
		} else {
			createDest.mutate(data);
		}
	};

	const handleRouteSave = (data) => {
		if (routeModal.editing) {
			updateRoute.mutate({ id: routeModal.editing.id, body: data });
		} else {
			createRoute.mutate(data);
		}
	};

	const handleReportSave = (data) => {
		if (reportModal.editing) {
			updateReport.mutate({ id: reportModal.editing.id, body: data });
		} else {
			createReport.mutate(data);
		}
	};

	const TH =
		"px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider";
	const TD =
		"px-4 py-2 text-sm text-secondary-900 dark:text-white whitespace-nowrap";
	const TDW = "px-4 py-2 text-sm text-secondary-900 dark:text-white";
	const W_STATUS = "w-24";
	const W_ACTIONS = "w-36";

	// When used as a standalone page, render all sections. When panel prop is set, render only that section.
	const showAll = !panel;
	const showDest = showAll || panel === "destinations";
	const showRoutes = showAll || panel === "routes";
	const showReports = showAll || panel === "reports";
	const showLog = showAll || panel === "log";

	return (
		<div className="space-y-6">
			{/* Header - only shown on standalone page */}
			{showAll && (
				<div className="flex items-center justify-between">
					<div>
						<h1 className="text-2xl font-semibold text-secondary-900 dark:text-white">
							Notifications
						</h1>
						<p className="text-sm text-secondary-600 dark:text-white mt-1">
							Manage destinations, routing rules, scheduled reports, and
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
							className="btn-outline flex items-center gap-2"
						>
							<RefreshCw className="h-4 w-4" /> Refresh log
						</button>
					)}
				</div>
			)}

			{/* ── Destinations ── */}
			{showDest && canManage && (
				<div className="card p-4 md:p-6 space-y-4">
					<div className="flex items-center justify-between">
						<h2 className="text-lg font-semibold text-secondary-900 dark:text-white">
							Destinations
						</h2>
						<button
							type="button"
							className="btn-primary flex items-center gap-2"
							onClick={() => setDestModal({ open: true, editing: null })}
						>
							<Plus className="h-4 w-4" /> Add destination
						</button>
					</div>

					{destLoading && <Loader2 className="h-5 w-5 animate-spin mx-auto" />}

					{!destLoading && destinations.length === 0 && (
						<div className="rounded-md p-4 bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 text-center">
							<p className="text-sm text-blue-800 dark:text-blue-200">
								No destinations yet. Add one to start receiving notifications.
							</p>
						</div>
					)}

					{destinations.length > 0 && (
						<div className="overflow-x-auto">
							<table className="min-w-full table-fixed divide-y divide-secondary-200 dark:divide-secondary-600">
								<thead className="bg-secondary-50 dark:bg-secondary-700">
									<tr>
										<th className={`${TH} w-28`}>Channel</th>
										<th className={TH}>Name</th>
										<th className={`${TH} w-20`}>Enabled</th>
										<th className={`${TH} ${W_ACTIONS}`}>Actions</th>
									</tr>
								</thead>
								<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-600">
									{destinations.map((d) => {
										const isBuiltIn = d.id === "internal-alerts";
										return (
											<tr
												key={d.id}
												className="hover:bg-secondary-50 dark:hover:bg-secondary-700"
											>
												<td className={TD}>
													<span className="inline-flex items-center gap-2">
														{channelIcon(d.channel_type)}
														{isBuiltIn ? (
															<span className="text-xs text-secondary-500">
																Built-in
															</span>
														) : (
															d.channel_type
														)}
													</span>
												</td>
												<td className={TD}>{d.display_name}</td>
												<td className={TD}>
													<button
														type="button"
														onClick={() =>
															updateDest.mutate({
																id: d.id,
																body: { enabled: !d.enabled },
															})
														}
														disabled={updateDest.isPending}
														className={`relative inline-flex h-5 w-9 items-center rounded-md transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 ${
															d.enabled
																? "bg-primary-600 dark:bg-primary-500"
																: "bg-secondary-200 dark:bg-secondary-600"
														} disabled:opacity-50`}
													>
														<span
															className={`inline-block h-3 w-3 transform rounded-md bg-white transition-transform ${d.enabled ? "translate-x-5" : "translate-x-1"}`}
														/>
													</button>
												</td>
												<td className={`${TD} flex items-center gap-2`}>
													{!isBuiltIn && (
														<button
															type="button"
															className="text-primary-600 hover:text-primary-700 inline-flex items-center gap-1 text-xs"
															onClick={() => sendTest(d.id)}
															disabled={testNotify.isPending}
														>
															<Send className="h-3.5 w-3.5" /> Test
														</button>
													)}
													<button
														type="button"
														className="text-primary-600 hover:text-primary-700 inline-flex items-center gap-1 text-xs"
														onClick={() => openEditDest(d)}
													>
														<Edit2 className="h-3.5 w-3.5" /> Edit
													</button>
													{!isBuiltIn && (
														<button
															type="button"
															className="text-red-600 hover:text-red-700 inline-flex items-center gap-1 text-xs"
															onClick={() => {
																if (confirm("Delete this destination?"))
																	deleteDest.mutate(d.id);
															}}
														>
															<Trash2 className="h-3.5 w-3.5" />
														</button>
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
			)}

			{/* ── Event Rules ── */}
			{showRoutes && canManage && (
				<div className="card p-4 md:p-6 space-y-4">
					<div className="flex items-center justify-between">
						<h2 className="text-lg font-semibold text-secondary-900 dark:text-white">
							Event Rules
						</h2>
						<button
							type="button"
							className="btn-primary flex items-center gap-2"
							onClick={() => setRouteModal({ open: true, editing: null })}
							disabled={destinations.length === 0}
						>
							<Plus className="h-4 w-4" /> Add event rule
						</button>
					</div>

					{routesLoading && (
						<Loader2 className="h-5 w-5 animate-spin mx-auto" />
					)}

					{!routesLoading && routes.length === 0 && destinations.length > 0 && (
						<div className="rounded-md p-4 bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 text-center">
							<p className="text-sm text-blue-800 dark:text-blue-200">
								No event rules yet. Event rules map events to destinations.
							</p>
						</div>
					)}

					{routes.length > 0 && (
						<div className="overflow-x-auto">
							<table className="min-w-full table-fixed divide-y divide-secondary-200 dark:divide-secondary-600">
								<thead className="bg-secondary-50 dark:bg-secondary-700">
									<tr>
										<th className={TH}>Destination</th>
										<th className={TH}>Events</th>
										<th className={`${TH} w-32`}>Min severity</th>
										<th className={TH}>Scope</th>
										<th className={`${TH} ${W_STATUS}`}>Status</th>
										<th className={`${TH} ${W_ACTIONS}`}>Actions</th>
									</tr>
								</thead>
								<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-600">
									{routes.map((row) => (
										<tr
											key={row.id}
											className="hover:bg-secondary-50 dark:hover:bg-secondary-700"
										>
											<td className={TD}>
												{row.destination_display_name || row.destination_id}
											</td>
											<td className={TDW}>
												{Array.isArray(row.event_types) &&
												row.event_types.includes("*")
													? "All events"
													: Array.isArray(row.event_types)
														? row.event_types
																.map((e) => e.replace(/_/g, " "))
																.join(", ")
														: "All events"}
											</td>
											<td className={TD}>
												<span
													className={`px-2 py-0.5 text-xs font-medium rounded-md ${
														row.min_severity === "critical"
															? "bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200"
															: row.min_severity === "error"
																? "bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200"
																: row.min_severity === "warning"
																	? "bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200"
																	: "bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200"
													}`}
												>
													{row.min_severity}
												</span>
											</td>
											<td className={TDW}>
												{Array.isArray(row.host_group_ids) &&
												row.host_group_ids.length > 0
													? row.host_group_ids
															.map((id) => hostGroupNameMap[id] || id)
															.join(", ")
													: "All"}
											</td>
											<td className={TD}>
												<span
													className={`px-2 py-0.5 text-xs font-medium rounded-md ${row.enabled ? "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200" : "bg-secondary-100 text-secondary-600 dark:bg-secondary-700 dark:text-secondary-300"}`}
												>
													{row.enabled ? "Active" : "Disabled"}
												</span>
											</td>
											<td className={`${TD} flex items-center gap-2`}>
												<button
													type="button"
													className="text-primary-600 hover:text-primary-700 inline-flex items-center gap-1 text-xs"
													onClick={() =>
														setRouteModal({ open: true, editing: row })
													}
												>
													<Edit2 className="h-3.5 w-3.5" /> Edit
												</button>
												<button
													type="button"
													className="text-red-600 hover:text-red-700 inline-flex items-center gap-1 text-xs"
													onClick={() => {
														if (confirm("Delete this route?"))
															deleteRoute.mutate(row.id);
													}}
												>
													<Trash2 className="h-3.5 w-3.5" />
												</button>
											</td>
										</tr>
									))}
								</tbody>
							</table>
						</div>
					)}
				</div>
			)}

			{/* ── Scheduled Reports ── */}
			{showReports && canManage && (
				<div className="card p-4 md:p-6 space-y-4">
					<div className="flex items-center justify-between">
						<h2 className="text-lg font-semibold text-secondary-900 dark:text-white">
							Scheduled reports
						</h2>
						<button
							type="button"
							className="btn-primary flex items-center gap-2"
							onClick={() => setReportModal({ open: true, editing: null })}
							disabled={destinations.length === 0}
						>
							<Plus className="h-4 w-4" /> New report
						</button>
					</div>

					{reportsLoading && (
						<Loader2 className="h-5 w-5 animate-spin mx-auto" />
					)}

					{!reportsLoading && scheduledReports.length === 0 && (
						<div className="rounded-md p-4 bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 text-center">
							<p className="text-sm text-blue-800 dark:text-blue-200">
								No scheduled reports yet.
							</p>
						</div>
					)}

					{scheduledReports.length > 0 && (
						<div className="overflow-x-auto">
							<table className="min-w-full table-fixed divide-y divide-secondary-200 dark:divide-secondary-600">
								<thead className="bg-secondary-50 dark:bg-secondary-700">
									<tr>
										<th className={`${TH} w-10`} />
										<th className={TH}>Name</th>
										<th className={TH}>Schedule</th>
										<th className={TH}>Next run</th>
										<th className={`${TH} ${W_STATUS}`}>Status</th>
										<th className={`${TH} ${W_ACTIONS}`}>Actions</th>
									</tr>
								</thead>
								<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-600">
									{scheduledReports.map((r) => (
										<tr
											key={r.id}
											className="hover:bg-secondary-50 dark:hover:bg-secondary-700"
										>
											<td className="px-2 py-2">
												<button
													type="button"
													className="inline-flex items-center justify-center w-6 h-6 rounded border border-transparent text-white bg-green-600 hover:bg-green-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-green-500 disabled:opacity-40"
													onClick={() => runReportNow.mutate(r.id)}
													disabled={runReportNow.isPending || !r.enabled}
													title={
														!r.enabled ? "Enable the report first" : "Run now"
													}
												>
													<Play className="h-3.5 w-3.5" />
												</button>
											</td>
											<td className={TD}>{r.name}</td>
											<td className={TD}>
												<span className="flex items-center gap-1">
													<Clock className="h-3.5 w-3.5 text-secondary-400" />
													{describeSchedule(r.cron_expr)}
												</span>
											</td>
											<td className={TD}>
												{r.next_run_at
													? formatRelativeTime(r.next_run_at)
													: " -"}
											</td>
											<td className={TD}>
												<span
													className={`px-2 py-0.5 text-xs font-medium rounded-md ${r.enabled ? "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200" : "bg-secondary-100 text-secondary-600 dark:bg-secondary-700 dark:text-secondary-300"}`}
												>
													{r.enabled ? "Active" : "Disabled"}
												</span>
											</td>
											<td className={`${TD} flex items-center gap-2`}>
												<button
													type="button"
													className="text-primary-600 hover:text-primary-700 inline-flex items-center gap-1 text-xs"
													onClick={() =>
														setReportModal({ open: true, editing: r })
													}
												>
													<Edit2 className="h-3.5 w-3.5" /> Edit
												</button>
												<button
													type="button"
													className="text-red-600 hover:text-red-700 inline-flex items-center gap-1 text-xs"
													onClick={() => {
														if (confirm("Delete this report?"))
															deleteReport.mutate(r.id);
													}}
												>
													<Trash2 className="h-3.5 w-3.5" />
												</button>
											</td>
										</tr>
									))}
								</tbody>
							</table>
						</div>
					)}
				</div>
			)}

			{/* ── Delivery Log ── */}
			{showLog && canLog && (
				<div className="card p-4 md:p-6 space-y-4">
					<div className="flex items-center justify-between">
						<h2 className="text-lg font-semibold text-secondary-900 dark:text-white">
							Delivery log
						</h2>
						{logLoading && <Loader2 className="h-5 w-5 animate-spin" />}
					</div>

					{!logLoading && deliveryLog.length === 0 ? (
						<p className="text-sm text-secondary-500">
							No delivery entries yet.
						</p>
					) : (
						<>
							<div className="overflow-x-auto">
								<table className="min-w-full table-fixed divide-y divide-secondary-200 dark:divide-secondary-600">
									<thead className="bg-secondary-50 dark:bg-secondary-700">
										<tr>
											<th className={`${TH} w-28`}>Time</th>
											<th className={`${TH} ${W_STATUS}`}>Status</th>
											<th className={TH}>Event</th>
											<th className={TH}>Destination</th>
											<th className={TH}>Reference</th>
											<th className={TH}>Error</th>
										</tr>
									</thead>
									<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-600">
										{deliveryLog.map((row) => (
											<tr
												key={row.id}
												className="hover:bg-secondary-50 dark:hover:bg-secondary-700 align-top"
											>
												<td className={TD} title={row.created_at || ""}>
													{row.created_at
														? formatRelativeTime(row.created_at)
														: " -"}
												</td>
												<td className="px-4 py-2">{statusBadge(row.status)}</td>
												<td className={TD}>{row.event_type}</td>
												<td className={TD}>
													{destNameMap[row.destination_id] ||
														row.destination_id}
												</td>
												<td className={TDW}>
													{(() => {
														const rid =
															typeof row.reference_id === "string"
																? row.reference_id
																: "";
														const rt = row.reference_type;
														let href = null;
														if (rid) {
															if (rt === "patch_run")
																href = `/patching/runs/${rid}`;
															else if (
																rt === "host" &&
																row.event_type === "compliance_scan_completed"
															)
																href = `/compliance/hosts/${rid}`;
															else if (rt === "host") href = `/hosts/${rid}`;
															else if (rt === "alert") href = `/hosts/${rid}`;
														}
														return href ? (
															<Link
																to={href}
																className="text-primary-600 hover:text-primary-700 hover:underline"
															>
																{rt}:{rid}
															</Link>
														) : (
															<span>
																{rt}:{rid || " -"}
															</span>
														);
													})()}
												</td>
												<td className="px-4 py-2 text-sm text-red-600 dark:text-red-400 max-w-xs break-words whitespace-normal">
													{row.error_message || " -"}
												</td>
											</tr>
										))}
									</tbody>
								</table>
							</div>
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
										className="p-1.5 rounded-lg bg-secondary-100 dark:bg-secondary-700 disabled:opacity-50"
										disabled={logPage === 0}
										onClick={() => setLogPage((p) => Math.max(0, p - 1))}
									>
										<ChevronLeft className="h-4 w-4" />
									</button>
									<button
										type="button"
										className="p-1.5 rounded-lg bg-secondary-100 dark:bg-secondary-700 disabled:opacity-50"
										disabled={deliveryLog.length < logPageSize}
										onClick={() => setLogPage((p) => p + 1)}
									>
										<ChevronRight className="h-4 w-4" />
									</button>
								</div>
							</div>
						</>
					)}
				</div>
			)}

			{showAll && !canManage && !canLog && (
				<div className="card p-8 text-center text-secondary-600">
					You don&apos;t have permission to view this page.
				</div>
			)}

			{/* Modals */}
			<DestinationModal
				key={destModal.editing?.id || "new-dest"}
				isOpen={destModal.open}
				onClose={() => setDestModal({ open: false, editing: null })}
				onSave={handleDestSave}
				editingDest={destModal.editing}
				isPending={createDest.isPending || updateDest.isPending}
			/>
			<RouteModal
				key={routeModal.editing?.id || "new-route"}
				isOpen={routeModal.open}
				onClose={() => setRouteModal({ open: false, editing: null })}
				onSave={handleRouteSave}
				editingRoute={routeModal.editing}
				destinations={destinations}
				hostGroups={hostGroupOptions}
				hosts={Array.isArray(hostsList) ? hostsList : []}
				isPending={createRoute.isPending || updateRoute.isPending}
			/>
			<ReportModal
				key={reportModal.editing?.id || "new-report"}
				isOpen={reportModal.open}
				onClose={() => setReportModal({ open: false, editing: null })}
				onSave={handleReportSave}
				editingReport={reportModal.editing}
				destinations={destinations}
				hostGroups={hostGroupOptions}
				isPending={createReport.isPending || updateReport.isPending}
			/>
		</div>
	);
};

// Default export for standalone settings page (renders all panels)
const AlertChannels = () => <NotificationPanel />;
export default AlertChannels;
