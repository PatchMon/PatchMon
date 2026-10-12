import { describe, expect, it } from "vitest";
import { isNavChildActive } from "../../utils/navActive";

const overview = { name: "Overview", href: "/patching?tab=overview" };
const runs = { name: "Runs & History", href: "/patching?tab=runs" };
const patching = { href: "/patching", children: [overview, runs] };

describe("isNavChildActive", () => {
	it("marks the default child active when the tab param is absent", () => {
		const location = { pathname: "/patching", search: "" };
		expect(isNavChildActive(location, patching, overview)).toBe(true);
		expect(isNavChildActive(location, patching, runs)).toBe(false);
	});

	it("keeps the default child active alongside non-tab params", () => {
		const location = { pathname: "/patching", search: "?status=failed" };
		expect(isNavChildActive(location, patching, overview)).toBe(true);
	});

	it("follows an explicit tab param", () => {
		const location = { pathname: "/patching", search: "?tab=runs" };
		expect(isNavChildActive(location, patching, overview)).toBe(false);
		expect(isNavChildActive(location, patching, runs)).toBe(true);
	});

	it("does not mark children of another page", () => {
		const location = { pathname: "/docker", search: "" };
		expect(isNavChildActive(location, patching, overview)).toBe(false);
	});

	it("matches a tab carried alongside filter params", () => {
		const location = {
			pathname: "/patching",
			search: "?tab=runs&status=failed",
		};
		expect(isNavChildActive(location, patching, runs)).toBe(true);
		expect(isNavChildActive(location, patching, overview)).toBe(false);
	});
});
