import { afterEach, describe, expect, it, vi } from "vitest";
import api, { packagesAPI, permissionsAPI } from "../../utils/api";

afterEach(() => vi.restoreAllMocks());

describe("path params are encoded", () => {
	it("encodes role names", async () => {
		const get = vi.spyOn(api, "get").mockResolvedValue({});
		const put = vi.spyOn(api, "put").mockResolvedValue({});
		const del = vi.spyOn(api, "delete").mockResolvedValue({});
		await permissionsAPI.getRole("Ops Team (EU)");
		await permissionsAPI.updateRole("Ops Team (EU)", {});
		await permissionsAPI.deleteRole("qa/team#1");
		expect(get).toHaveBeenCalledWith("/permissions/roles/Ops%20Team%20(EU)");
		expect(put).toHaveBeenCalledWith(
			"/permissions/roles/Ops%20Team%20(EU)",
			{},
		);
		expect(del).toHaveBeenCalledWith("/permissions/roles/qa%2Fteam%231");
	});

	it("encodes package IDs and names", async () => {
		const get = vi.spyOn(api, "get").mockResolvedValue({ data: {} });
		await packagesAPI.getById("a#b");
		await packagesAPI.getHosts("q?x", { page: 1 });
		await packagesAPI.getActivity("dir/name");
		expect(get.mock.calls.map((c) => c[0])).toEqual([
			"/packages/a%23b",
			"/packages/q%3Fx/hosts",
			"/packages/dir%2Fname/activity",
		]);
	});
});
