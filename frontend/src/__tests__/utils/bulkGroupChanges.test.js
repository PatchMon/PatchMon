import { describe, expect, it } from "vitest";
import {
	bulkGroupChanges,
	countGroupMembers,
	groupCheckState,
} from "../../utils/bulkGroupChanges";

const host = (id, ...groupIds) => ({
	id,
	host_group_memberships: groupIds.map((g) => ({ host_groups: { id: g } })),
});

// Host A in Prod, host B in Web, as in issue #1245.
const hosts = [host("a", "prod"), host("b", "web"), host("c", "prod", "web")];

describe("countGroupMembers", () => {
	it("counts only the selected hosts", () => {
		expect(countGroupMembers(hosts, ["a", "b"])).toEqual({ prod: 1, web: 1 });
		expect(countGroupMembers(hosts, ["a", "c"])).toEqual({ prod: 2, web: 1 });
	});

	it("handles hosts without memberships", () => {
		expect(countGroupMembers([{ id: "x" }], ["x"])).toEqual({});
	});
});

describe("groupCheckState", () => {
	const counts = { prod: 2, web: 1 };

	it("shows the current memberships", () => {
		expect(groupCheckState("prod", counts, 2, {})).toBe(true);
		expect(groupCheckState("web", counts, 2, {})).toBe("mixed");
		expect(groupCheckState("linux", counts, 2, {})).toBe(false);
	});

	it("uses the user's choice once made", () => {
		expect(groupCheckState("web", counts, 2, { web: true })).toBe(true);
		expect(groupCheckState("prod", counts, 2, { prod: false })).toBe(false);
	});
});

describe("bulkGroupChanges", () => {
	it("adding a group keeps the other memberships", () => {
		const counts = countGroupMembers(hosts, ["a", "b"]);
		expect(bulkGroupChanges(counts, 2, { linux: true })).toEqual({
			addGroupIds: ["linux"],
			removeGroupIds: [],
		});
	});

	it("removes a group only when it is unchecked", () => {
		expect(bulkGroupChanges({ prod: 1 }, 2, { prod: false })).toEqual({
			addGroupIds: [],
			removeGroupIds: ["prod"],
		});
	});

	it("checking a mixed group adds it to the hosts without it", () => {
		expect(bulkGroupChanges({ web: 1 }, 2, { web: true })).toEqual({
			addGroupIds: ["web"],
			removeGroupIds: [],
		});
	});

	it("skips choices that change nothing", () => {
		expect(
			bulkGroupChanges({ prod: 2 }, 2, { prod: true, linux: false }),
		).toEqual({ addGroupIds: [], removeGroupIds: [] });
	});
});
