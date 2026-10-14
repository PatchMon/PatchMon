import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
	fireEvent,
	render,
	screen,
	waitFor,
	within,
} from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import Hosts from "../../pages/Hosts";
import { dashboardAPI, userPreferencesAPI } from "../../utils/api";

const { HOSTS } = vi.hoisted(() => {
	const host = (overrides) => ({
		api_id: `api-${overrides.id}`,
		os_type: "Ubuntu",
		os_version: "22.04",
		status: "active",
		host_group_memberships: [],
		updatesCount: 0,
		securityUpdatesCount: 0,
		last_update: new Date().toISOString(),
		config_version: null,
		reported_host_id: null,
		identity_conflict_at: null,
		...overrides,
	});

	const HOSTS = [
		host({
			id: "h-ok",
			friendly_name: "healthy",
			agent_version: "2.1.4",
			config_version: "2.1.4",
		}),
		host({
			id: "h-stale",
			friendly_name: "stale-config",
			agent_version: "2.1.4",
			config_version: "2.1.4-rc.1",
		}),
		host({ id: "h-old", friendly_name: "old-agent", agent_version: "2.1.3" }),
		host({
			id: "h-clone",
			friendly_name: "cloned",
			agent_version: "2.1.4",
			config_version: "2.1.4",
			identity_conflict_at: "2026-10-13T10:00:00Z",
		}),
		host({
			id: "h-wrongid",
			friendly_name: "wrong-id",
			agent_version: "2.1.4",
			config_version: "2.1.4",
			reported_host_id: "h-ok",
		}),
	];
	return { HOSTS };
});

vi.mock("../../utils/api", async (importOriginal) => {
	const actual = await importOriginal();
	const ok = (data) => vi.fn(() => Promise.resolve({ data }));
	return {
		...actual,
		dashboardAPI: {
			getHosts: ok({ items: HOSTS, total: HOSTS.length, limit: 50, offset: 0 }),
			getHostFilterOptions: ok({ osDistribution: [] }),
			getHostCounts: ok({ total: HOSTS.length }),
			getWsStatusSummary: ok({}),
		},
		hostGroupsAPI: { list: ok([]) },
		settingsAPI: { getPublic: ok({}), get: ok({}), update: ok({}) },
		userPreferencesAPI: { get: ok({}), update: ok({}) },
		alertsAPI: { getAlertConfigByType: ok({ data: {} }) },
		adminHostsAPI: {},
	};
});

const renderHosts = () => {
	const queryClient = new QueryClient({
		defaultOptions: { queries: { retry: false } },
	});
	return render(
		<QueryClientProvider client={queryClient}>
			<MemoryRouter initialEntries={["/hosts"]}>
				<Hosts />
			</MemoryRouter>
		</QueryClientProvider>,
	);
};

const desktopRow = (name) =>
	screen
		.getAllByText(name)
		.map((el) => el.closest("tr"))
		.find(Boolean);

beforeEach(() => {
	localStorage.clear();
	userPreferencesAPI.get.mockImplementation(() =>
		Promise.resolve({ data: {} }),
	);
	vi.stubGlobal(
		"fetch",
		vi.fn(() =>
			Promise.resolve({ ok: true, json: () => Promise.resolve({ data: {} }) }),
		),
	);
});

afterEach(() => {
	vi.unstubAllGlobals();
});

describe("Hosts table config version", () => {
	it("hides the Config Version column by default", async () => {
		renderHosts();
		await screen.findAllByText("stale-config");
		expect(
			screen.getAllByRole("columnheader", { name: /agent version/i }).length,
		).toBeGreaterThan(0);
		expect(
			screen.queryByRole("columnheader", { name: /config version/i }),
		).toBeNull();
	});

	it("shows the column when the saved preferences turn it on", async () => {
		userPreferencesAPI.get.mockImplementation(() =>
			Promise.resolve({
				data: {
					hosts_column_config: [
						{ id: "agent_version", visible: true, order: 7 },
						{ id: "config_version", visible: true, order: 8 },
					],
				},
			}),
		);
		renderHosts();
		expect(
			(await screen.findAllByRole("columnheader", { name: /config version/i }))
				.length,
		).toBeGreaterThan(0);
		const row = desktopRow("old-agent");
		expect(within(row).getByText("Requires agent 2.1.4")).toBeInTheDocument();
	});

	it("sorts by config version from its header, like Agent Version", async () => {
		userPreferencesAPI.get.mockImplementation(() =>
			Promise.resolve({
				data: {
					hosts_column_config: [
						{ id: "agent_version", visible: true, order: 7 },
						{ id: "config_version", visible: true, order: 8 },
					],
				},
			}),
		);
		renderHosts();
		const header = (
			await screen.findAllByRole("columnheader", { name: /config version/i })
		)[0];
		const sortButton = within(header).getByRole("button", {
			name: /config version/i,
		});
		fireEvent.click(sortButton);
		await waitFor(() =>
			expect(dashboardAPI.getHosts).toHaveBeenLastCalledWith(
				expect.objectContaining({ sort: "config_version" }),
			),
		);
	});

	it("keeps a saved layout from before the column existed hidden", async () => {
		userPreferencesAPI.get.mockImplementation(() =>
			Promise.resolve({
				data: {
					hosts_column_config: [
						{ id: "host", visible: true, order: 1 },
						{ id: "agent_version", visible: true, order: 7 },
					],
				},
			}),
		);
		renderHosts();
		await screen.findAllByText("stale-config");
		expect(
			screen.queryByRole("columnheader", { name: /config version/i }),
		).toBeNull();
	});

	it("always marks mismatches next to the agent version", async () => {
		renderHosts();
		await screen.findAllByText("stale-config");

		expect(
			within(desktopRow("stale-config")).getByRole("button", {
				name: "Config version out of date",
			}),
		).toBeInTheDocument();
		expect(
			within(desktopRow("cloned")).getByRole("button", {
				name: "Possible cloned machine",
			}),
		).toBeInTheDocument();
		expect(
			within(desktopRow("wrong-id")).getByRole("button", {
				name: "Host ID mismatch in config.yml",
			}),
		).toBeInTheDocument();

		for (const name of ["healthy", "old-agent"]) {
			const row = desktopRow(name);
			expect(
				within(row).queryByRole("button", { name: /config version/i }),
			).toBeNull();
			expect(
				within(row).queryByRole("button", { name: /clone|host id/i }),
			).toBeNull();
		}
	});
});
