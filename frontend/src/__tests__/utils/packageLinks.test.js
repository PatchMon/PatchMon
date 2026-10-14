import { matchRoutes } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { packageDetailPath } from "../../utils/packageLinks";

const NAMES = [
	"libstdc++6",
	"g++-14",
	"python3.12",
	"50%",
	"c%25weird",
	"a#b",
	"q?x",
	"dir/name",
	"名前-ünïcode",
	"2026-10 Security Update (KB5127648) (26300.9711)",
];

const routeParam = (path) =>
	matchRoutes([{ path: "/packages/:packageId" }], path)?.[0]?.params.packageId;

describe("packageDetailPath", () => {
	it("links by ID when the name is in the inventory", () => {
		expect(
			packageDetailPath("Mozilla Firefox (x64 en-GB)", {
				"Mozilla Firefox (x64 en-GB)": "3f2b6c1e-0000-4000-8000-000000000001",
			}),
		).toBe("/packages/3f2b6c1e-0000-4000-8000-000000000001");
	});

	it("returns null when the server resolved IDs and the name is missing", () => {
		expect(
			packageDetailPath("2026-10 Security Update (KB5127648) (26300.9711)", {}),
		).toBeNull();
	});

	it("falls back to the name when the server sent no IDs", () => {
		expect(packageDetailPath("libstdc++6")).toBe("/packages/libstdc%2B%2B6");
		expect(packageDetailPath("")).toBeNull();
	});

	it.each(NAMES)("round-trips %s through the router param", (name) => {
		const path = packageDetailPath(name);
		expect(path.slice("/packages/".length)).not.toMatch(/[/?#]/);
		expect(routeParam(path)).toBe(name);
	});
});
