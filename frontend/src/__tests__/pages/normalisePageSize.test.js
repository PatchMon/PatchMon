/**
 * Unit tests for the Hosts page-size guard. It backs both the `?pageSize=`
 * URL param and the localStorage value, so it has to survive anything.
 */

import { describe, expect, it } from "vitest";
import { normalisePageSize } from "../../pages/Hosts";

const DEFAULT = 50;

describe("normalisePageSize", () => {
	it("accepts every offered page size", () => {
		for (const size of [25, 50, 100, 200, 500]) {
			expect(normalisePageSize(size)).toBe(size);
			expect(normalisePageSize(String(size))).toBe(size);
		}
	});

	it("falls back to the default for values that are not offered", () => {
		expect(normalisePageSize(10)).toBe(DEFAULT);
		expect(normalisePageSize(51)).toBe(DEFAULT);
		expect(normalisePageSize(1000)).toBe(DEFAULT);
		expect(normalisePageSize(0)).toBe(DEFAULT);
		expect(normalisePageSize(-25)).toBe(DEFAULT);
	});

	it("falls back to the default for NaN and non-numeric input", () => {
		expect(normalisePageSize(Number.NaN)).toBe(DEFAULT);
		expect(normalisePageSize("abc")).toBe(DEFAULT);
		expect(normalisePageSize("")).toBe(DEFAULT);
		expect(normalisePageSize(null)).toBe(DEFAULT);
		expect(normalisePageSize(undefined)).toBe(DEFAULT);
		expect(normalisePageSize({})).toBe(DEFAULT);
		expect(normalisePageSize([])).toBe(DEFAULT);
	});

	it("falls back to the default for out-of-range numeric strings", () => {
		expect(normalisePageSize("999999999999")).toBe(DEFAULT);
		expect(normalisePageSize("-1")).toBe(DEFAULT);
		expect(normalisePageSize("Infinity")).toBe(DEFAULT);
	});

	it("uses the integer prefix that parseInt yields, not a rounded value", () => {
		// "100px" and "100.9" both parse to 100, which is an offered size.
		expect(normalisePageSize("100px")).toBe(100);
		expect(normalisePageSize("100.9")).toBe(100);
		// 99.9 parses to 99, which is not offered.
		expect(normalisePageSize("99.9")).toBe(DEFAULT);
	});
});
