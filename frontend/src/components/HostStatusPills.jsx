import { Activity, Package, RotateCcw, Wifi, WifiOff } from "lucide-react";
import { formatRelativeTime } from "../utils/api";
import { deriveReportingStateByTime } from "../utils/hostStatus";
import Tooltip from "./ui/Tooltip";

/**
 * HostStatusPills renders up to four independent status pills for a host:
 *
 *   1. WS         — WebSocket connection state (green/amber/red)
 *   2. Reporting  — Agent data freshness, cross-coupled with WS state
 *   3. Reboot     — Only visible when host.needs_reboot === true
 *   4. Updates    — Up-to-date / pending / security required
 *
 * Each pill uses semantic design-system colours (success/warning/danger) and
 * is wrapped in a Tooltip with an explainer. Pass `compact` for tight contexts
 * (mobile cards, inline rows) — labels collapse to icons + abbreviated text.
 *
 * Props:
 *   host                       — host record from the API
 *   wsStatus                   — { connected, secure, disconnected_seconds_ago } | undefined
 *   hostDownThresholdSeconds   — number, defaults to 30
 *   compact                    — boolean
 */
const PILL_BASE =
	"inline-flex items-center justify-center gap-1 px-2 py-0.5 rounded text-xs font-medium whitespace-nowrap";
// Interactive pills must hit the 44px WCAG tap target below the `md` breakpoint,
// which is exactly where the mobile card layout is shown. From `md` up the
// desktop table takes over and the pills return to their compact density.
const PILL_TAP_TARGET = "min-h-[44px] min-w-[44px] md:min-h-0 md:min-w-0";
const PILL_VARIANTS = {
	success:
		"bg-success-100 text-success-800 dark:bg-success-900 dark:text-success-200",
	warning:
		"bg-warning-100 text-warning-800 dark:bg-warning-900 dark:text-warning-200",
	danger:
		"bg-danger-100 text-danger-800 dark:bg-danger-900 dark:text-danger-200",
	neutral:
		"bg-secondary-100 text-secondary-700 dark:bg-secondary-700 dark:text-secondary-200",
};

const Pill = ({ variant = "neutral", icon: Icon, label, srLabel, tooltip }) => {
	// Render as a span when there is no tooltip (purely visual badge), and as
	// a button-shaped element when a tooltip is present so keyboard users can
	// focus the pill and surface the explainer. Using <button type="button">
	// satisfies a11y rules and lets <Tooltip> hook into native focus events.
	if (!tooltip) {
		return (
			<span
				className={`${PILL_BASE} ${PILL_VARIANTS[variant] || PILL_VARIANTS.neutral}`}
				title={srLabel || label}
			>
				{Icon ? (
					<Icon className="h-3 w-3 flex-shrink-0" aria-hidden="true" />
				) : null}
				{label ? <span>{label}</span> : null}
			</span>
		);
	}
	const trigger = (
		<button
			type="button"
			className={`${PILL_BASE} ${PILL_TAP_TARGET} ${PILL_VARIANTS[variant] || PILL_VARIANTS.neutral} cursor-help focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-1`}
			aria-label={srLabel || label}
			onClick={(e) => e.preventDefault()}
		>
			{Icon ? (
				<Icon className="h-3 w-3 flex-shrink-0" aria-hidden="true" />
			) : null}
			{label ? <span>{label}</span> : null}
		</button>
	);
	return <Tooltip content={tooltip}>{trigger}</Tooltip>;
};

const formatSecondsAgo = (seconds) => {
	if (!Number.isFinite(seconds) || seconds < 0) return "unknown duration";
	if (seconds < 60) return `${Math.round(seconds)}s`;
	if (seconds < 3600) return `${Math.round(seconds / 60)}m`;
	if (seconds < 86400) return `${Math.round(seconds / 3600)}h`;
	return `${Math.round(seconds / 86400)}d`;
};

const HostStatusPills = ({
	host,
	wsStatus,
	hostDownThresholdSeconds = 30,
	updateIntervalMinutes = 60,
	compact = false,
}) => {
	if (!host) return null;

	const threshold = Number.isFinite(hostDownThresholdSeconds)
		? hostDownThresholdSeconds
		: 30;

	const lastUpdateRel = host.last_update
		? formatRelativeTime(host.last_update)
		: "never";

	// ---------- WS pill ----------
	let wsPill = null;
	if (wsStatus !== undefined && wsStatus !== null) {
		const connected = !!wsStatus.connected;
		const secure = !!wsStatus.secure;
		const disconnectedSeconds = wsStatus.disconnected_seconds_ago;
		// The backend preserves `secure` across disconnects (registry.Unregister
		// only flips Connected, never Secure), so the protocol label is
		// meaningful regardless of connection state. Using `secure` always
		// keeps the label honest. Hardcoding "WS" on disconnect would falsely
		// imply insecure for hosts that actually connect via WSS.
		const protocol = secure ? "WSS" : "WS";

		if (connected) {
			wsPill = (
				<Pill
					key="ws"
					variant="success"
					icon={Wifi}
					label={compact ? null : protocol}
					srLabel="WebSocket connected"
					tooltip={`WebSocket connected${secure ? " (secure)" : ""}. Real-time control channel is active.`}
				/>
			);
		} else {
			const seconds =
				typeof disconnectedSeconds === "number" ? disconnectedSeconds : null;
			// When duration is unknown (server cold-start with the agent already
			// disconnected, so the registry never recorded a DisconnectedAt),
			// treat it as past-threshold rather than within-grace. Operators
			// shouldn't see a permanently-amber pill for an agent that's been
			// silent for an indeterminate amount of time.
			const withinGrace = seconds !== null && seconds <= threshold;
			const variant = withinGrace ? "warning" : "danger";
			let tooltip;
			if (withinGrace) {
				tooltip = `WebSocket disconnected (${formatSecondsAgo(seconds)}). Within the ${threshold}s grace window, the agent may be reconnecting.`;
			} else if (seconds !== null) {
				tooltip = `WebSocket disconnected for ${formatSecondsAgo(seconds)} (threshold: ${threshold}s).`;
			} else {
				tooltip = `WebSocket disconnected, duration unknown (likely past the ${threshold}s threshold). The server may have restarted while the agent was already offline.`;
			}
			// Disconnected state must not be conveyed by colour alone (WCAG
			// 1.4.1). The struck-through WifiOff glyph carries it in compact
			// mode, where there is no room for text, and the label spells the
			// state out everywhere else.
			wsPill = (
				<Pill
					key="ws"
					variant={variant}
					icon={WifiOff}
					label={
						compact
							? null
							: `${protocol} ${withinGrace ? "reconnecting" : "offline"}`
					}
					srLabel={
						withinGrace
							? "WebSocket disconnected, within grace window"
							: "WebSocket disconnected"
					}
					tooltip={tooltip}
				/>
			);
		}
	}

	// ---------- Reporting pill ----------
	// Computed purely from last_update + the configured agent update interval
	// — not from the backend's pre-computed reportingState/isStale. This is
	// the single source of truth so the pill always agrees with the operator's
	// configured threshold regardless of which API path served the host record
	// or how the backend's SQL CASE happens to evaluate.
	//
	// Cross-couples with live WS state for the amber-vs-red distinction in the
	// "overdue" range (×1 to ×2 of the interval). While `wsStatus` is still
	// loading, treat it as "assume connected" so a healthy host doesn't briefly
	// flicker through "Stale" before the WS payload arrives.
	const reportingState = deriveReportingStateByTime(
		host.last_update,
		updateIntervalMinutes,
	);
	const wsLoaded = wsStatus !== undefined && wsStatus !== null;
	const wsConnectedOrUnknown = !wsLoaded || wsStatus.connected === true;

	let reportingVariant = "success";
	let reportingLabel = "Reporting";
	let reportingTooltip = `Agent reported recently. Last update: ${lastUpdateRel}.`;
	if (reportingState !== "reporting") {
		if (wsConnectedOrUnknown) {
			reportingVariant = "warning";
			reportingLabel = "Overdue";
			reportingTooltip = `Agent has not pushed a report yet but the WebSocket is still connected, so this is likely transient. Last update: ${lastUpdateRel}.`;
		} else {
			reportingVariant = "danger";
			reportingLabel = "Stale";
			reportingTooltip = `Agent has not reported and the WebSocket is disconnected. Last update: ${lastUpdateRel}.`;
		}
	}
	const reportingPill = (
		<Pill
			key="reporting"
			variant={reportingVariant}
			icon={Activity}
			label={compact ? null : reportingLabel}
			srLabel={`Reporting status: ${reportingLabel}`}
			tooltip={reportingTooltip}
		/>
	);

	// ---------- Reboot pill (only when needed) ----------
	const rebootPill = host.needs_reboot ? (
		<Pill
			key="reboot"
			variant="warning"
			icon={RotateCcw}
			label={compact ? null : "Reboot pending"}
			srLabel="Reboot pending"
			tooltip={
				host.reboot_reason || "Host requires a reboot to apply pending updates."
			}
		/>
	) : null;

	// ---------- Updates pill ----------
	const securityCount = host.securityUpdatesCount || 0;
	const updatesCount = host.updatesCount || 0;
	const updateState =
		host.updateState ||
		(securityCount > 0
			? "security_required"
			: updatesCount > 0
				? "updates_pending"
				: "up_to_date");

	let updatesVariant = "success";
	let updatesLabel = "Up to date";
	let updatesTooltip = "All packages up to date.";
	if (updateState === "security_required" || securityCount > 0) {
		updatesVariant = "danger";
		updatesLabel = "Security patches required";
		updatesTooltip = `${securityCount} security update${securityCount === 1 ? "" : "s"} available.`;
	} else if (updateState === "updates_pending" || updatesCount > 0) {
		updatesVariant = "warning";
		updatesLabel = "Updates pending";
		updatesTooltip = `${updatesCount} non-security update${updatesCount === 1 ? "" : "s"} available.`;
	}
	const updatesPill = (
		<Pill
			key="updates"
			variant={updatesVariant}
			icon={Package}
			label={compact ? null : updatesLabel}
			srLabel={updatesLabel}
			tooltip={updatesTooltip}
		/>
	);

	return (
		<div className="inline-flex flex-wrap items-center gap-2">
			{wsPill}
			{reportingPill}
			{rebootPill}
			{updatesPill}
		</div>
	);
};

export default HostStatusPills;
export { HostStatusPills };
