/**
 * Unit tests for the shared reporting-state derivation used by the Hosts
 * table, the Reporting filter and HostStatusPills.
 */

import { describe, expect, it } from "vitest";
import {
	deriveReportingState,
	deriveReportingStateByTime,
} from "../../utils/hostStatus";

// Fixed clock so the tests never depend on wall time.
const NOW = Date.parse("2026-07-27T12:00:00.000Z");
const minutesAgo = (m) => new Date(NOW - m * 60000).toISOString();

describe("deriveReportingStateByTime", () => {
	const interval = 60;

	it("returns stale when last_update is missing", () => {
		expect(deriveReportingStateByTime(null, interval, NOW)).toBe("stale");
		expect(deriveReportingStateByTime(undefined, interval, NOW)).toBe("stale");
		expect(deriveReportingStateByTime("", interval, NOW)).toBe("stale");
	});

	it("returns stale when last_update is unparseable", () => {
		expect(deriveReportingStateByTime("not-a-date", interval, NOW)).toBe(
			"stale",
		);
	});

	it("returns reporting inside one interval", () => {
		expect(deriveReportingStateByTime(minutesAgo(0), interval, NOW)).toBe(
			"reporting",
		);
		expect(deriveReportingStateByTime(minutesAgo(59), interval, NOW)).toBe(
			"reporting",
		);
	});

	it("treats exactly one interval as still reporting", () => {
		expect(deriveReportingStateByTime(minutesAgo(60), interval, NOW)).toBe(
			"reporting",
		);
	});

	it("returns overdue between one and two intervals", () => {
		expect(deriveReportingStateByTime(minutesAgo(61), interval, NOW)).toBe(
			"overdue",
		);
		expect(deriveReportingStateByTime(minutesAgo(120), interval, NOW)).toBe(
			"overdue",
		);
	});

	it("returns stale past two intervals", () => {
		expect(deriveReportingStateByTime(minutesAgo(121), interval, NOW)).toBe(
			"stale",
		);
		expect(deriveReportingStateByTime(minutesAgo(19 * 60), interval, NOW)).toBe(
			"stale",
		);
	});

	it("falls back to a 60 minute interval when the interval is not finite", () => {
		expect(deriveReportingStateByTime(minutesAgo(30), undefined, NOW)).toBe(
			"reporting",
		);
		expect(deriveReportingStateByTime(minutesAgo(90), Number.NaN, NOW)).toBe(
			"overdue",
		);
		expect(deriveReportingStateByTime(minutesAgo(200), null, NOW)).toBe(
			"stale",
		);
	});

	it("clamps a sub-minute interval to one minute rather than dividing by zero", () => {
		expect(deriveReportingStateByTime(minutesAgo(0), 0, NOW)).toBe("reporting");
		expect(deriveReportingStateByTime(minutesAgo(5), 0, NOW)).toBe("stale");
	});

	it("clamps future timestamps (clock skew) to reporting", () => {
		const future = new Date(NOW + 10 * 60000).toISOString();
		expect(deriveReportingStateByTime(future, interval, NOW)).toBe("reporting");
	});
});

describe("deriveReportingState", () => {
	const interval = 60;

	// Every WS-by-time combination. WS state only ever decides the amber vs red
	// distinction outside the reporting window; it can never make a host that
	// reported recently look unhealthy.
	const cases = [
		// last_update, wsConnectedOrUnknown, expected
		[minutesAgo(10), true, "reporting"],
		[minutesAgo(10), false, "reporting"],
		[minutesAgo(90), true, "overdue"],
		[minutesAgo(90), false, "stale"],
		[minutesAgo(600), true, "overdue"],
		[minutesAgo(600), false, "stale"],
		[null, true, "overdue"],
		[null, false, "stale"],
		["not-a-date", true, "overdue"],
		["not-a-date", false, "stale"],
	];

	it.each(
		cases,
	)("last_update=%s wsConnectedOrUnknown=%s -> %s", (lastUpdate, wsConnectedOrUnknown, expected) => {
		expect(
			deriveReportingState(
				{ last_update: lastUpdate },
				wsConnectedOrUnknown,
				interval,
				NOW,
			),
		).toBe(expected);
	});

	it("does not depend on host.status", () => {
		// A pending host silent for 19 hours must read as stale, not Reporting,
		// even though the SQL isStale flag only flips for status='active'.
		const pending = { status: "pending", last_update: minutesAgo(19 * 60) };
		expect(deriveReportingState(pending, false, interval, NOW)).toBe("stale");
		expect(deriveReportingState(pending, true, interval, NOW)).toBe("overdue");
	});

	it("treats a missing host as stale rather than throwing", () => {
		expect(deriveReportingState(null, false, interval, NOW)).toBe("stale");
		expect(deriveReportingState(undefined, true, interval, NOW)).toBe(
			"overdue",
		);
	});
});
