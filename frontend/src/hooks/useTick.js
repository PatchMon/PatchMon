import { useEffect, useState } from "react";

/**
 * useTick - re-render the consumer on a fixed interval.
 *
 * Returns a fresh `Date.now()` value on every render. The interval triggers
 * a state update so the consumer re-renders periodically. Use this to keep
 * derived values like live uptime ticking forward without coupling the
 * consumer to a wall-clock timer.
 *
 * @param {number} [intervalMs=60000] - Tick interval in milliseconds.
 * @returns {number} Current `Date.now()` snapshot at render time.
 */
export const useTick = (intervalMs = 60000) => {
	const [, setTick] = useState(0);
	useEffect(() => {
		const id = setInterval(() => setTick((t) => t + 1), intervalMs);
		return () => clearInterval(id);
	}, [intervalMs]);
	return Date.now();
};
