import { useEffect } from "react";
import { authAPI } from "../utils/api";
import { hasPendingInteraction } from "../utils/userActivity";

// Worst case, an interaction is reported this long after it happened, so the
// interval has to leave room under the smallest inactivity window an operator
// might set. One minute is a legitimate setting; 30s keeps it safe, and is still
// a third of what the sidebar's own status poll already costs.
const HEARTBEAT_INTERVAL_MS = 30 * 1000;

// Keeps an in-use session alive without relying on whatever the current page
// happens to poll. Pages differ in that respect and some poll nothing at all, so
// without a beat of its own a user reading one of those screens would be logged
// out mid-read. It only fires when there has been a real interaction since the
// last beat, so an unattended tab reports nothing and ages out as configured.
export function useSessionHeartbeat(enabled) {
	useEffect(() => {
		if (!enabled) return undefined;
		const id = setInterval(() => {
			if (document.visibilityState === "hidden") return;
			if (!hasPendingInteraction()) return;
			authAPI.heartbeat().catch(() => {
				// A failed beat is not actionable: the response interceptor already
				// handles an expired session, and the next beat retries.
			});
		}, HEARTBEAT_INTERVAL_MS);
		return () => clearInterval(id);
	}, [enabled]);
}
