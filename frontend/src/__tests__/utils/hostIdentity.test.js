import { describe, expect, it } from "vitest";
import {
	CONFIG_VERSION_STATE,
	getConfigVersionState,
	hasConfigVersionMismatch,
	hasHostIdMismatch,
	hasIdentityConflict,
	supportsConfigVersion,
} from "../../utils/hostIdentity";

const { MATCH, MISMATCH, NOT_REPORTED, REQUIRES_UPGRADE, UNKNOWN } =
	CONFIG_VERSION_STATE;

describe("getConfigVersionState", () => {
	it.each([
		["2.1.4", "2.1.4", MATCH],
		["2.1.4-rc.1", "2.1.4-rc.1", MATCH],
		["2.1.4-rc.1.4", "2.1.4-rc.1.4", MATCH],
		["2.1.3-main-93c589a4", "2.1.3-main-93c589a4", MATCH],
		["0.0.0-dev", "0.0.0-dev", MATCH],
		["v2.1.4", "2.1.4", MATCH],
		[" 2.1.4 ", "2.1.4", MATCH],
		["2.1.5", "2.1.4", MISMATCH],
		["2.1.4", "2.1.4-rc.1", MISMATCH],
		["2.1.4-rc.2", "2.1.4-rc.1", MISMATCH],
		["2.1.4-main-93c589a4", "2.1.4-main-0abc1234", MISMATCH],
		["2.1.4-main-93c589a4", "2.1.4", MISMATCH],
		["2.1.4+build.2", "2.1.4+build.1", MISMATCH],
		["2.1.4", null, NOT_REPORTED],
		["2.1.4", "", NOT_REPORTED],
		["2.1.5", undefined, NOT_REPORTED],
		["2.1.4-rc.1", null, NOT_REPORTED],
		["2.1.4-main-93c589a4", null, NOT_REPORTED],
		["2.1.3", null, REQUIRES_UPGRADE],
		["2.1.3-main-93c589a4", null, REQUIRES_UPGRADE],
		["0.0.0-dev", null, REQUIRES_UPGRADE],
		["unknown", null, REQUIRES_UPGRADE],
		[null, null, UNKNOWN],
		["", "2.1.4", UNKNOWN],
		[undefined, undefined, UNKNOWN],
	])("agent %j with config %j is %s", (agent, config, expected) => {
		expect(getConfigVersionState(agent, config)).toBe(expected);
	});
});

describe("supportsConfigVersion", () => {
	it("treats every 2.1.4 pre-release and later as supporting it", () => {
		expect(supportsConfigVersion("2.1.4-rc.1")).toBe(true);
		expect(supportsConfigVersion("2.1.4")).toBe(true);
		expect(supportsConfigVersion("2.2.0")).toBe(true);
		expect(supportsConfigVersion("2.1.3")).toBe(false);
		expect(supportsConfigVersion("")).toBe(false);
	});
});

describe("hasConfigVersionMismatch", () => {
	it("never flags an old agent that sends no config version", () => {
		expect(
			hasConfigVersionMismatch({
				agent_version: "2.1.3",
				config_version: null,
			}),
		).toBe(false);
		expect(hasConfigVersionMismatch({ agent_version: "2.1.3" })).toBe(false);
		expect(hasConfigVersionMismatch({})).toBe(false);
		expect(hasConfigVersionMismatch(undefined)).toBe(false);
	});

	it("flags only a reported value that differs", () => {
		expect(
			hasConfigVersionMismatch({
				agent_version: "2.1.4",
				config_version: "2.1.4-rc.1",
			}),
		).toBe(true);
		expect(
			hasConfigVersionMismatch({
				agent_version: "2.1.4",
				config_version: "2.1.4",
			}),
		).toBe(false);
	});
});

describe("hasHostIdMismatch", () => {
	it("uses the server flag from the host detail response", () => {
		expect(hasHostIdMismatch({ id: "a", host_id_mismatch: true })).toBe(true);
		expect(hasHostIdMismatch({ id: "a", host_id_mismatch: false })).toBe(false);
	});

	it("derives it from reported_host_id on list rows", () => {
		expect(hasHostIdMismatch({ id: "a", reported_host_id: "b" })).toBe(true);
		expect(hasHostIdMismatch({ id: "a", reported_host_id: "a" })).toBe(false);
		expect(hasHostIdMismatch({ id: "a", reported_host_id: null })).toBe(false);
		expect(hasHostIdMismatch({ id: "a", reported_host_id: "" })).toBe(false);
		expect(hasHostIdMismatch(null)).toBe(false);
	});
});

describe("hasIdentityConflict", () => {
	it("is set only by a conflict timestamp", () => {
		expect(
			hasIdentityConflict({ identity_conflict_at: "2026-10-13T10:00:00Z" }),
		).toBe(true);
		expect(hasIdentityConflict({ identity_conflict_at: null })).toBe(false);
		expect(hasIdentityConflict({})).toBe(false);
	});
});
