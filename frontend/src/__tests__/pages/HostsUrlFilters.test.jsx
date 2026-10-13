import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
	act,
	fireEvent,
	render,
	screen,
	waitFor,
} from "@testing-library/react";
import { MemoryRouter, useLocation, useNavigate } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import Hosts from "../../pages/Hosts";
import { dashboardAPI } from "../../utils/api";

vi.mock("../../utils/api", async (importOriginal) => {
	const actual = await importOriginal();
	const ok = (data) => vi.fn(() => Promise.resolve({ data }));
	return {
		...actual,
		dashboardAPI: {
			getHosts: ok({
				items: [
					{
						id: "h1",
						api_id: "a1",
						hostname: "web-01",
						friendly_name: "web-01",
						os_type: "Ubuntu",
						os_version: "22.04",
						status: "active",
						host_group_memberships: [],
						updatesCount: 0,
						securityUpdatesCount: 0,
						last_update: new Date().toISOString(),
					},
				],
				total: 500,
				limit: 50,
				offset: 0,
			}),
			getHostFilterOptions: ok({
				osDistribution: [
					{ os_type: "Ubuntu", os_version: "22.04" },
					{ os_type: "Ubuntu", os_version: "24.04" },
				],
			}),
			getHostCounts: ok({ total: 500 }),
			getWsStatusSummary: ok({}),
		},
		hostGroupsAPI: {
			list: ok([
				{ id: "g1", name: "Web" },
				{ id: "g2", name: "DB" },
			]),
		},
		settingsAPI: { getPublic: ok({}), get: ok({}), update: ok({}) },
		userPreferencesAPI: { get: ok({}), update: ok({}) },
		alertsAPI: { getAlertConfigByType: ok({ data: {} }) },
		adminHostsAPI: {},
	};
});

let location;
let navigate;
const Probe = () => {
	location = useLocation();
	navigate = useNavigate();
	return null;
};

const renderAt = (entry) => {
	const queryClient = new QueryClient({
		defaultOptions: { queries: { retry: false } },
	});
	return render(
		<QueryClientProvider client={queryClient}>
			<MemoryRouter initialEntries={[entry]}>
				<Hosts />
				<Probe />
			</MemoryRouter>
		</QueryClientProvider>,
	);
};

const params = () => new URLSearchParams(location.search);
const lastHostsQuery = () => dashboardAPI.getHosts.mock.calls.at(-1)[0];
const openFilters = async () => {
	await screen.findByText("Total Hosts");
	if (!screen.queryByLabelText("Reporting")) {
		fireEvent.click(screen.getByRole("button", { name: /filters/i }));
	}
};

beforeEach(() => {
	dashboardAPI.getHosts.mockClear();
	localStorage.clear();
	vi.stubGlobal(
		"fetch",
		vi.fn(() =>
			Promise.resolve({ ok: true, json: () => Promise.resolve({ data: {} }) }),
		),
	);
});

afterEach(() => {
	vi.useRealTimers();
	vi.unstubAllGlobals();
});

describe("Hosts filters are written to the URL", () => {
	it("writes each dropdown filter to the URL and drops it again at its default", async () => {
		renderAt("/hosts");
		await openFilters();

		fireEvent.change(screen.getByLabelText("Reporting"), {
			target: { value: "overdue" },
		});
		fireEvent.change(screen.getByLabelText("Connection"), {
			target: { value: "offline" },
		});
		fireEvent.change(screen.getByLabelText("Host Group"), {
			target: { value: "g2" },
		});
		await screen.findByRole("option", { name: "Ubuntu" });
		fireEvent.change(screen.getByLabelText("Operating System"), {
			target: { value: "ubuntu" },
		});
		fireEvent.change(await screen.findByLabelText("OS Version"), {
			target: { value: "24.04" },
		});

		expect(params().get("status")).toBe("overdue");
		expect(params().get("connection")).toBe("offline");
		expect(params().get("group")).toBe("g2");
		expect(params().get("osFilter")).toBe("ubuntu");
		expect(params().get("osVersionFilter")).toBe("24.04");

		fireEvent.change(screen.getByLabelText("Reporting"), {
			target: { value: "all" },
		});
		fireEvent.change(screen.getByLabelText("Operating System"), {
			target: { value: "all" },
		});
		expect(params().has("status")).toBe(false);
		expect(params().has("osFilter")).toBe(false);
		expect(params().has("osVersionFilter")).toBe(false);
		expect(params().get("connection")).toBe("offline");
	});

	it("writes grouping, hide stale and sort, and leaves defaults out", async () => {
		renderAt("/hosts");
		await screen.findByText("Total Hosts");
		expect(location.search).toBe("");

		fireEvent.change(screen.getByDisplayValue("No Grouping"), {
			target: { value: "os" },
		});
		fireEvent.click(screen.getByRole("button", { name: /hide stale/i }));
		expect(params().get("groupBy")).toBe("os");
		expect(params().get("hideStale")).toBe("true");

		fireEvent.click(
			screen.getAllByRole("button", { name: /system hostname/i })[0],
		);
		expect(params().get("order")).toBe("desc");
		expect(params().has("sort")).toBe(false);
		fireEvent.click(
			screen.getAllByRole("button", { name: /system hostname/i })[0],
		);
		expect(params().has("order")).toBe(false);
	});

	it("writes the search term after the debounce", async () => {
		renderAt("/hosts");
		await screen.findByText("Total Hosts");
		vi.useFakeTimers();
		fireEvent.change(screen.getByPlaceholderText(/search hosts/i), {
			target: { value: "web-01 " },
		});
		expect(params().has("search")).toBe(false);
		act(() => vi.advanceTimersByTime(400));
		expect(params().get("search")).toBe("web-01");
		fireEvent.change(screen.getByPlaceholderText(/search hosts/i), {
			target: { value: "" },
		});
		act(() => vi.advanceTimersByTime(400));
		expect(params().has("search")).toBe(false);
	});

	it("goes back to page 1 when a filter changes", async () => {
		renderAt("/hosts?page=3");
		await openFilters();
		expect(params().get("page")).toBe("3");
		fireEvent.change(screen.getByLabelText("Connection"), {
			target: { value: "connected" },
		});
		expect(params().get("page")).not.toBe("3");
	});
});

describe("Hosts filters are restored from the URL", () => {
	it("restores every filter, the search and the sort on load", async () => {
		renderAt(
			"/hosts?status=overdue&connection=offline&group=g2&osFilter=ubuntu&osVersionFilter=22.04&search=web&groupBy=os&hideStale=true&sort=ip&order=desc",
		);
		await openFilters();
		await screen.findByRole("option", { name: "DB" });
		expect(screen.getByLabelText("Reporting")).toHaveValue("overdue");
		expect(screen.getByLabelText("Connection")).toHaveValue("offline");
		expect(screen.getByLabelText("Host Group")).toHaveValue("g2");
		expect(screen.getByLabelText("Operating System")).toHaveValue("ubuntu");
		expect(screen.getByLabelText("OS Version")).toHaveValue("22.04");
		expect(screen.getByPlaceholderText(/search hosts/i)).toHaveValue("web");
		expect(screen.getByDisplayValue("By OS")).toBeInTheDocument();

		await waitFor(() =>
			expect(lastHostsQuery()).toMatchObject({
				search: "web",
				group: "g2",
				os: "ubuntu",
				osVersion: "22.04",
				hideStale: "true",
				sort: "ip",
				order: "desc",
			}),
		);
	});

	it("follows back and forward navigation", async () => {
		renderAt("/hosts");
		await openFilters();
		act(() => navigate("/hosts?status=stale&search=db"));
		expect(screen.getByLabelText("Reporting")).toHaveValue("stale");
		expect(screen.getByPlaceholderText(/search hosts/i)).toHaveValue("db");
		act(() => navigate(-1));
		expect(screen.getByLabelText("Reporting")).toHaveValue("all");
		expect(screen.getByPlaceholderText(/search hosts/i)).toHaveValue("");
	});

	it("ignores values that are not valid options", async () => {
		renderAt(
			"/hosts?status=bogus&connection=nope&groupBy=x&sort=evil&order=up",
		);
		await openFilters();
		expect(screen.getByLabelText("Reporting")).toHaveValue("all");
		expect(screen.getByLabelText("Connection")).toHaveValue("all");
		expect(screen.getByDisplayValue("No Grouping")).toBeInTheDocument();
		await waitFor(() =>
			expect(lastHostsQuery()).toMatchObject({
				sort: "hostname",
				order: "asc",
			}),
		);
	});
});

describe("existing deep links into /hosts still apply", () => {
	it.each([
		["/hosts?filter=stale", "stale"],
		["/hosts?filter=upToDate", "reporting"],
		["/hosts?filter=needsUpdates", "all"],
		["/hosts?filter=inactive", "all"],
	])("%s opens the filters with Reporting = %s", async (entry, status) => {
		renderAt(entry);
		await screen.findByText("Total Hosts");
		expect(screen.getByLabelText("Reporting")).toHaveValue(status);
		await waitFor(() =>
			expect(lastHostsQuery().filter).toBe(
				new URLSearchParams(entry.split("?")[1]).get("filter"),
			),
		);
	});

	it("rewrites the legacy offline link to stale", async () => {
		renderAt("/hosts?filter=offline");
		await screen.findByText("Total Hosts");
		await waitFor(() => expect(params().get("filter")).toBe("stale"));
		expect(screen.getByLabelText("Reporting")).toHaveValue("stale");
	});

	it("applies the dashboard OS chart link", async () => {
		renderAt("/hosts?osFilter=ubuntu&osVersionFilter=22.04&showFilters=true");
		await screen.findByText("Total Hosts");
		expect(screen.getByLabelText("Operating System")).toHaveValue("ubuntu");
		expect(screen.getByLabelText("OS Version")).toHaveValue("22.04");
		await waitFor(() =>
			expect(lastHostsQuery()).toMatchObject({
				os: "ubuntu",
				osVersion: "22.04",
			}),
		);
	});

	it("applies the host group link from settings", async () => {
		renderAt("/hosts?group=g1");
		await screen.findByRole("option", { name: "Web" });
		expect(screen.getByLabelText("Host Group")).toHaveValue("g1");
		await waitFor(() => expect(lastHostsQuery().group).toBe("g1"));
	});

	it("applies the reboot and selected-hosts links", async () => {
		renderAt("/hosts?reboot=true");
		await screen.findByText("Total Hosts");
		await waitFor(() => expect(lastHostsQuery().reboot).toBe("true"));
	});

	it("applies the packages page selected-hosts link", async () => {
		renderAt("/hosts?selected=a,b&filter=selected");
		await screen.findByText("Total Hosts");
		await waitFor(() =>
			expect(lastHostsQuery()).toMatchObject({
				filter: "selected",
				selected: "a,b",
			}),
		);
	});

	it("a later filter change keeps the deep link and does not re-impose it", async () => {
		renderAt("/hosts?filter=stale");
		await screen.findByText("Total Hosts");
		fireEvent.change(screen.getByLabelText("Reporting"), {
			target: { value: "overdue" },
		});
		expect(screen.getByLabelText("Reporting")).toHaveValue("overdue");
		expect(params().get("filter")).toBe("stale");
		expect(params().get("status")).toBe("overdue");
	});

	it("clicking a stat card replaces the URL filters", async () => {
		renderAt("/hosts?status=overdue&search=web&groupBy=os");
		await screen.findByText("Total Hosts");
		fireEvent.click(screen.getByText("Needs Updates"));
		expect(params().get("filter")).toBe("needsUpdates");
		expect(params().has("status")).toBe(false);
		fireEvent.click(screen.getByText("Total Hosts"));
		expect(location.search).toBe("");
		expect(screen.getByPlaceholderText(/search hosts/i)).toHaveValue("");
	});
});
