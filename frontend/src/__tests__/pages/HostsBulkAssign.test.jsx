import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import Hosts from "../../pages/Hosts";
import { adminHostsAPI } from "../../utils/api";

const { host } = vi.hoisted(() => ({
	host: (id, name, groupIds) => ({
		id,
		api_id: `api-${id}`,
		hostname: name,
		friendly_name: name,
		os_type: "Ubuntu",
		os_version: "22.04",
		status: "active",
		host_group_memberships: groupIds.map((g) => ({
			host_groups: { id: g, name: g },
		})),
		updatesCount: 0,
		securityUpdatesCount: 0,
		last_update: new Date().toISOString(),
	}),
}));

vi.mock("../../utils/api", async (importOriginal) => {
	const actual = await importOriginal();
	const ok = (data) => vi.fn(() => Promise.resolve({ data }));
	return {
		...actual,
		dashboardAPI: {
			getHosts: ok({
				// Issue #1245: host A in Prod, host B in Web.
				items: [host("a", "host-a", ["prod"]), host("b", "host-b", ["web"])],
				total: 2,
				limit: 50,
				offset: 0,
			}),
			getHostFilterOptions: ok({ osDistribution: [] }),
			getHostCounts: ok({ total: 2 }),
			getWsStatusSummary: ok({}),
		},
		hostGroupsAPI: {
			list: ok([
				{ id: "prod", name: "Prod" },
				{ id: "web", name: "Web" },
				{ id: "linux", name: "Linux" },
			]),
		},
		settingsAPI: { getPublic: ok({}), get: ok({}), update: ok({}) },
		userPreferencesAPI: { get: ok({}), update: ok({}) },
		alertsAPI: { getAlertConfigByType: ok({ data: {} }) },
		adminHostsAPI: {
			bulkUpdateGroups: ok({}),
			bulkChangeGroups: ok({}),
		},
	};
});

const selectHost = async (name) => {
	const cells = await screen.findAllByText(name);
	const row = cells.map((c) => c.closest("tr")).find(Boolean);
	fireEvent.click(row.querySelector("button"));
};

const groupCheckbox = (name) =>
	screen
		.getByText(name, { selector: "div" })
		.closest("label")
		.querySelector("input");

beforeEach(() => {
	localStorage.clear();
	vi.stubGlobal(
		"fetch",
		vi.fn(() =>
			Promise.resolve({ ok: true, json: () => Promise.resolve({ data: {} }) }),
		),
	);
});

describe("bulk Assign to Group", () => {
	it("shows current groups and only adds the checked one", async () => {
		const queryClient = new QueryClient({
			defaultOptions: { queries: { retry: false } },
		});
		render(
			<QueryClientProvider client={queryClient}>
				<MemoryRouter initialEntries={["/hosts"]}>
					<Hosts />
				</MemoryRouter>
			</QueryClientProvider>,
		);

		await selectHost("host-a");
		await selectHost("host-b");
		fireEvent.click(screen.getByRole("button", { name: /assign to group/i }));
		await screen.findByText("Linux", { selector: "div" });

		// Each group is in one of the two hosts: shown as partly checked.
		for (const name of ["Prod", "Web"]) {
			expect(groupCheckbox(name).checked).toBe(false);
			expect(groupCheckbox(name).indeterminate).toBe(true);
		}
		expect(groupCheckbox("Linux").indeterminate).toBe(false);

		fireEvent.click(groupCheckbox("Linux"));
		fireEvent.click(screen.getByRole("button", { name: "Assign to Groups" }));

		await waitFor(() =>
			expect(adminHostsAPI.bulkChangeGroups).toHaveBeenCalledWith(
				["a", "b"],
				["linux"],
				[],
			),
		);
		expect(adminHostsAPI.bulkUpdateGroups).not.toHaveBeenCalled();
	});
});
