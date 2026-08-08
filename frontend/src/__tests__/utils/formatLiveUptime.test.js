/**
 * Unit tests for formatLiveUptime, the live "Uptime" cell formatter used by
 * the Hosts table and the host detail page.
 */

import { describe, expect, it } from "vitest";
import { formatLiveUptime } from "../../utils/api";

const NOW = Date.parse("2026-07-27T12:00:00.000Z");
const minutesBefore = (m) => new Date(NOW - m * 60000).toISOString();

describe("formatLiveUptime", () => {
	it("returns an empty string when boot_time is missing", () => {
		// The caller falls back to the legacy system_uptime TEXT on "".
		expect(formatLiveUptime(null, NOW)).toBe("");
		expect(formatLiveUptime(undefined, NOW)).toBe("");
		expect(formatLiveUptime("", NOW)).toBe("");
	});

	it("returns an empty string when boot_time is unparseable", () => {
		expect(formatLiveUptime("not-a-date", NOW)).toBe("");
		expect(formatLiveUptime("2026-13-45T99:99:99Z", NOW)).toBe("");
	});

	it("formats minutes only under an hour", () => {
		expect(formatLiveUptime(minutesBefore(0), NOW)).toBe("0 minutes");
		expect(formatLiveUptime(minutesBefore(1), NOW)).toBe("1 minute");
		expect(formatLiveUptime(minutesBefore(59), NOW)).toBe("59 minutes");
	});

	it("formats hours and minutes under a day", () => {
		expect(formatLiveUptime(minutesBefore(60), NOW)).toBe("1 hour, 0 minutes");
		expect(formatLiveUptime(minutesBefore(61), NOW)).toBe("1 hour, 1 minute");
		expect(formatLiveUptime(minutesBefore(150), NOW)).toBe(
			"2 hours, 30 minutes",
		);
	});

	it("formats days, hours and minutes from a day up", () => {
		expect(formatLiveUptime(minutesBefore(1440), NOW)).toBe(
			"1 day, 0 hours, 0 minutes",
		);
		expect(formatLiveUptime(minutesBefore(1440 + 61), NOW)).toBe(
			"1 day, 1 hour, 1 minute",
		);
		expect(formatLiveUptime(minutesBefore(3 * 1440 + 125), NOW)).toBe(
			"3 days, 2 hours, 5 minutes",
		);
	});

	it("clamps a future boot_time (clock skew) to zero rather than going negative", () => {
		const future = new Date(NOW + 90 * 60000).toISOString();
		expect(formatLiveUptime(future, NOW)).toBe("0 minutes");
	});

	it("truncates rather than rounds partial minutes", () => {
		const thirtySecondsAgo = new Date(NOW - 30 * 1000).toISOString();
		expect(formatLiveUptime(thirtySecondsAgo, NOW)).toBe("0 minutes");
	});
});
