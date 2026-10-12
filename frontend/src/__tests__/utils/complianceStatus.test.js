import { describe, expect, it } from "vitest";
import { hostComplianceStatus } from "../../utils/complianceStatus";

describe("hostComplianceStatus", () => {
	it("prefers the server-computed status", () => {
		expect(
			hostComplianceStatus({
				compliance_status: "warning",
				score: 10,
				failed: 5,
			}),
		).toBe("warning");
	});

	it.each([
		[{ score: 85, failed: 2 }, "compliant"],
		[{ score: 70, failed: 4 }, "warning"],
		[{ score: 30, failed: 0 }, "warning"],
		[{ score: 30, failed: 3 }, "critical"],
	])("falls back to score and failures for %o", (row, expected) => {
		expect(hostComplianceStatus(row)).toBe(expected);
	});
});
