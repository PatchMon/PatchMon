// Reporting-state derivation shared by the Hosts table, the Reporting filter
// and HostStatusPills. Kept in one module so the pill colours and the filter
// predicate can never drift apart.
//
// Computed purely from `last_update` and the configured agent update_interval.
// Mirrors the legacy effectiveStatus / isStale boundary (x2 of update_interval)
// but adds the intermediate "overdue" amber state between x1 and x2.
//
// Deliberately does NOT depend on `host.status` (active/pending/error), unlike
// the SQL `isStale` flag which only flips for `status='active'` — so a pending
// host that has gone silent for 19 hours reads correctly as stale.

const DEFAULT_UPDATE_INTERVAL_MINUTES = 60;

/**
 * @param {string|null|undefined} lastUpdateIso
 * @param {number} updateIntervalMinutes
 * @param {number} [nowMs] injectable clock, for tests
 * @returns {"reporting"|"overdue"|"stale"}
 */
export const deriveReportingStateByTime = (
	lastUpdateIso,
	updateIntervalMinutes,
	nowMs = Date.now(),
) => {
	if (!lastUpdateIso) return "stale";
	const lastUpdateMs = new Date(lastUpdateIso).getTime();
	if (!Number.isFinite(lastUpdateMs)) return "stale";
	const interval = Number.isFinite(updateIntervalMinutes)
		? Math.max(1, updateIntervalMinutes)
		: DEFAULT_UPDATE_INTERVAL_MINUTES;
	const elapsedMin = Math.max(0, (nowMs - lastUpdateMs) / 60000);
	if (elapsedMin <= interval) return "reporting";
	if (elapsedMin <= interval * 2) return "overdue";
	return "stale";
};

/**
 * Cross-couples the time-based state with live WebSocket state.
 *
 * `wsConnectedOrUnknown` should be true when either the agent's WS is
 * known-connected OR the WS status has not loaded yet. This prevents a brief
 * "Stale" flicker on healthy hosts during the wsStatusMap loading window: only
 * an explicit `connected: false` from the API downgrades the pill to red.
 *
 * @param {object|null|undefined} host
 * @param {boolean} wsConnectedOrUnknown
 * @param {number} updateIntervalMinutes
 * @param {number} [nowMs] injectable clock, for tests
 * @returns {"reporting"|"overdue"|"stale"}
 */
export const deriveReportingState = (
	host,
	wsConnectedOrUnknown,
	updateIntervalMinutes,
	nowMs = Date.now(),
) => {
	const timeState = deriveReportingStateByTime(
		host?.last_update,
		updateIntervalMinutes,
		nowMs,
	);
	if (timeState === "reporting") return "reporting";
	return wsConnectedOrUnknown ? "overdue" : "stale";
};
