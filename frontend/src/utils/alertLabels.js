/**
 * Display label overrides for alert types. The internal alert_type IDs (the
 * keys here) MUST stay unchanged across the codebase — they are used by the
 * DB, the API, and external integrations. This map renames the user-visible
 * label only.
 */
export const ALERT_LABEL_OVERRIDES = {
	host_down: "Host Agent Down",
	host_recovered: "Host Agent Recovered",
	// Add future overrides here.
};

/**
 * Resolve a human-readable label for an alert type. Falls back to a Title Case
 * conversion of the snake_case identifier when no override exists.
 */
export const formatAlertType = (type) => {
	if (!type) return "";
	if (ALERT_LABEL_OVERRIDES[type]) return ALERT_LABEL_OVERRIDES[type];
	return type.replace(/_/g, " ").replace(/\b\w/g, (l) => l.toUpperCase());
};
