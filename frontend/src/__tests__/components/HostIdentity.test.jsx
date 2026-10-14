import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
	AgentVersionIndicators,
	ConfigVersionValue,
	HostIdentityBanners,
} from "../../components/host/HostIdentity";
import { adminHostsAPI } from "../../utils/api";

const toast = { success: vi.fn(), error: vi.fn() };

vi.mock("../../contexts/ToastContext", () => ({
	useToast: () => toast,
}));

vi.mock("../../utils/api", async (importOriginal) => {
	const actual = await importOriginal();
	return {
		...actual,
		adminHostsAPI: {
			dismissIdentityConflict: vi.fn(() =>
				Promise.resolve({ data: { success: true } }),
			),
		},
	};
});

const baseHost = {
	id: "3b0c2f7e-6a1d-4c55-9a0e-2f1f7d9e8c41",
	friendly_name: "web-01",
	agent_version: "2.1.4",
	config_version: "2.1.4",
	host_id_mismatch: false,
	identity_conflict_at: null,
	credentials_reissued_at: null,
};

let queryClient;
const renderWithClient = (ui) => {
	queryClient = new QueryClient({
		defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
	});
	vi.spyOn(queryClient, "invalidateQueries");
	return render(
		<QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>,
	);
};

beforeEach(() => {
	vi.clearAllMocks();
});

describe("ConfigVersionValue", () => {
	it("shows the plain value when it matches the agent", () => {
		render(<ConfigVersionValue host={baseHost} />);
		expect(screen.getByText("2.1.4")).toBeInTheDocument();
		expect(screen.queryByText("Out of date")).toBeNull();
	});

	it("flags an rc config left behind by a final release", () => {
		render(
			<ConfigVersionValue
				host={{ ...baseHost, config_version: "2.1.4-rc.1" }}
			/>,
		);
		expect(screen.getByText("2.1.4-rc.1")).toBeInTheDocument();
		expect(screen.getByText("Out of date")).toBeInTheDocument();
		expect(
			screen.getByRole("button", { name: "About config version out of date" }),
		).toBeInTheDocument();
	});

	it("says Not reported for a new agent with no value", () => {
		render(<ConfigVersionValue host={{ ...baseHost, config_version: null }} />);
		expect(screen.getByText("Not reported")).toBeInTheDocument();
	});

	it("never shows a mismatch for an old agent that sends nothing", () => {
		render(
			<ConfigVersionValue
				host={{ ...baseHost, agent_version: "2.1.3", config_version: null }}
			/>,
		);
		expect(screen.getByText("Requires agent 2.1.4")).toBeInTheDocument();
		expect(screen.queryByText("Out of date")).toBeNull();
		expect(screen.queryByText("Not reported")).toBeNull();
	});

	it("falls back to the empty label when the agent has never reported", () => {
		render(
			<ConfigVersionValue
				host={{ ...baseHost, agent_version: null, config_version: null }}
				emptyLabel="N/A"
			/>,
		);
		expect(screen.getByText("N/A")).toBeInTheDocument();
	});
});

describe("AgentVersionIndicators", () => {
	it("renders nothing for a healthy host or an old agent", () => {
		const { container, rerender } = render(
			<AgentVersionIndicators host={baseHost} />,
		);
		expect(container).toBeEmptyDOMElement();
		rerender(
			<AgentVersionIndicators
				host={{ ...baseHost, agent_version: "2.1.3", config_version: null }}
			/>,
		);
		expect(container).toBeEmptyDOMElement();
	});

	it("shows a warning icon when the config version differs", () => {
		render(
			<AgentVersionIndicators
				host={{ ...baseHost, config_version: "2.1.3" }}
			/>,
		);
		const icon = screen.getByRole("button", {
			name: "Config version out of date",
		});
		fireEvent.mouseEnter(icon);
		expect(screen.getByRole("tooltip")).toHaveTextContent(
			"since it was upgraded",
		);
	});

	it("shows a danger icon for a host ID mismatch or a possible clone", () => {
		const { rerender } = render(
			<AgentVersionIndicators
				host={{ ...baseHost, reported_host_id: "another-host" }}
			/>,
		);
		expect(
			screen.getByRole("button", { name: "Host ID mismatch in config.yml" }),
		).toBeInTheDocument();
		rerender(
			<AgentVersionIndicators
				host={{ ...baseHost, identity_conflict_at: "2026-10-13T10:00:00Z" }}
			/>,
		);
		expect(
			screen.getByRole("button", { name: "Possible cloned machine" }),
		).toBeInTheDocument();
	});
});

describe("HostIdentityBanners", () => {
	it("renders nothing without an identity problem", () => {
		const { container } = renderWithClient(
			<HostIdentityBanners host={baseHost} canManage />,
		);
		expect(container).toBeEmptyDOMElement();
	});

	it("shows the host ID mismatch remedy", () => {
		renderWithClient(
			<HostIdentityBanners
				host={{ ...baseHost, host_id_mismatch: true }}
				canManage
			/>,
		);
		expect(screen.getByText("Host ID mismatch")).toBeInTheDocument();
		expect(
			screen.getByText("patchmon-agent config sync-host-id"),
		).toBeInTheDocument();
		expect(screen.queryByRole("button", { name: /dismiss/i })).toBeNull();
	});

	it("offers regenerate and dismiss on a possible clone", async () => {
		const onRegenerate = vi.fn();
		renderWithClient(
			<HostIdentityBanners
				host={{ ...baseHost, identity_conflict_at: "2026-10-13T10:00:00Z" }}
				canManage
				onRegenerateCredentials={onRegenerate}
			/>,
		);
		expect(screen.getByText(/possible clone/)).toBeInTheDocument();

		fireEvent.click(
			screen.getByRole("button", { name: /regenerate credentials/i }),
		);
		expect(onRegenerate).toHaveBeenCalledTimes(1);

		fireEvent.click(screen.getByRole("button", { name: /dismiss/i }));
		await waitFor(() =>
			expect(adminHostsAPI.dismissIdentityConflict).toHaveBeenCalledWith(
				baseHost.id,
			),
		);
		await waitFor(() =>
			expect(toast.success).toHaveBeenCalledWith(
				"Possible clone warning dismissed",
			),
		);
		expect(queryClient.invalidateQueries).toHaveBeenCalledWith({
			queryKey: ["host", baseHost.id],
		});
	});

	it("reports a failed dismiss with a toast", async () => {
		adminHostsAPI.dismissIdentityConflict.mockRejectedValueOnce({
			response: { data: { error: "Forbidden" } },
		});
		renderWithClient(
			<HostIdentityBanners
				host={{ ...baseHost, identity_conflict_at: "2026-10-13T10:00:00Z" }}
				canManage
			/>,
		);
		fireEvent.click(screen.getByRole("button", { name: /dismiss/i }));
		await waitFor(() => expect(toast.error).toHaveBeenCalledWith("Forbidden"));
	});

	it("hides the actions from users who cannot manage hosts", () => {
		renderWithClient(
			<HostIdentityBanners
				host={{ ...baseHost, identity_conflict_at: "2026-10-13T10:00:00Z" }}
				canManage={false}
			/>,
		);
		expect(screen.getByText(/possible clone/)).toBeInTheDocument();
		expect(screen.queryByRole("button", { name: /dismiss/i })).toBeNull();
		expect(
			screen.queryByRole("button", { name: /regenerate credentials/i }),
		).toBeNull();
	});
});
