import { describe, expect, it } from "vitest";
import { compareVersions, isUpdateAvailable } from "../../utils/version";

// These cases mirror version_test.go in the server. If one side changes, change
// both: the login screen and the server's update alert must agree.
describe("compareVersions", () => {
	it.each([
		["equal", "2.0.2", "2.0.2", 0],
		["patch newer", "2.0.3", "2.0.2", 1],
		["patch older", "2.0.2", "2.0.3", -1],
		["minor beats patch", "2.1.0", "2.0.9", 1],
		["major beats minor", "3.0.0", "2.9.9", 1],
		["v prefix ignored", "v2.0.3", "2.0.3", 0],
		["whitespace ignored", " 2.0.3 ", "2.0.3", 0],
		["missing parts are zero", "2.0", "2.0.0", 0],
		["extra part outranks", "2.0.3.1", "2.0.3", 1],
	])("%s", (_name, a, b, want) => {
		expect(compareVersions(a, b)).toBe(want);
	});

	// The edge-release relationship the whole scheme depends on.
	it.each([
		["rc ranks below its release", "2.0.3-rc.4", "2.0.3", -1],
		["release outranks its rc", "2.0.3", "2.0.3-rc.4", 1],
		["rc still outranks previous release", "2.0.3-rc.1", "2.0.2", 1],
		["previous release below rc", "2.0.2", "2.0.3-rc.1", -1],
		["rc numbers compare numerically", "2.0.3-rc.9", "2.0.3-rc.10", -1],
		["rc numbers are not lexical", "2.0.3-rc.10", "2.0.3-rc.9", 1],
		["identical rc", "2.0.3-rc.4", "2.0.3-rc.4", 0],
		["rc below next release", "2.0.3-rc.99", "2.0.4", -1],
	])("%s", (_name, a, b, want) => {
		expect(compareVersions(a, b)).toBe(want);
	});

	it.each([
		["numeric ranks below alphanumeric", "1.0.0-1", "1.0.0-alpha", -1],
		["alpha before beta", "1.0.0-alpha", "1.0.0-beta", -1],
		["longer identifier list wins", "1.0.0-alpha.1", "1.0.0-alpha", 1],
		["shorter identifier list loses", "1.0.0-alpha", "1.0.0-alpha.1", -1],
		["build metadata ignored", "2.0.3+abc", "2.0.3", 0],
		["build metadata on rc ignored", "2.0.3-rc.4+abc", "2.0.3-rc.4", 0],
	])("%s", (_name, a, b, want) => {
		expect(compareVersions(a, b)).toBe(want);
	});

	it.each([
		["git describe output", "2.0.2-60-gABC", "2.0.2", -1],
		["empty is lowest", "", "2.0.2", -1],
		["null is lowest", null, "2.0.2", -1],
		["both empty", "", "", 0],
	])("handles malformed input: %s", (_name, a, b, want) => {
		expect(compareVersions(a, b)).toBe(want);
	});

	it("is antisymmetric across an ascending fixture", () => {
		const versions = [
			"2.0.2",
			"2.0.3-rc.1",
			"2.0.3-rc.2",
			"2.0.3-rc.10",
			"2.0.3",
			"2.0.4-rc.1",
			"2.1.0",
			"3.0.0",
		];
		versions.forEach((a, i) => {
			versions.forEach((b, j) => {
				// Asserting both directions against the fixture order implies
				// antisymmetry. `|| 0` normalises -0, which toBe treats as
				// distinct from 0.
				expect(compareVersions(a, b)).toBe(Math.sign(i - j) || 0);
				expect(compareVersions(b, a)).toBe(Math.sign(j - i) || 0);
			});
		});
	});
});

describe("isUpdateAvailable", () => {
	it("flags a newer release", () => {
		expect(isUpdateAvailable("2.0.2", "2.0.3")).toBe(true);
	});

	it("flags the release as an update for an edge build", () => {
		expect(isUpdateAvailable("2.0.3-rc.61", "2.0.3")).toBe(true);
	});

	it("does not flag an edge build as an update for the release", () => {
		expect(isUpdateAvailable("2.0.3", "2.0.3-rc.61")).toBe(false);
	});

	it("does not flag equal versions", () => {
		expect(isUpdateAvailable("2.0.3", "2.0.3")).toBe(false);
	});

	it("does not flag when either side is missing", () => {
		expect(isUpdateAvailable("", "2.0.3")).toBe(false);
		expect(isUpdateAvailable("2.0.3", "")).toBe(false);
		expect(isUpdateAvailable(null, undefined)).toBe(false);
	});
});
