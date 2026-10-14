import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import AddHostWizard from "../../components/AddHostWizard";
import { settingsAPI } from "../../utils/api";

vi.mock("../../utils/api", () => ({
	adminHostsAPI: {
		create: vi.fn(() =>
			Promise.resolve({
				data: {
					hostId: "host-1",
					apiId: "api-1",
					friendlyName: "web-01",
					apiKey: "key-1",
				},
			}),
		),
	},
	dashboardAPI: { getHostDetail: vi.fn() },
	hostGroupsAPI: { list: vi.fn(() => Promise.resolve({ data: [] })) },
	settingsAPI: {
		get: vi.fn(() => Promise.resolve({ data: {} })),
		getServerUrl: vi.fn(() =>
			Promise.resolve({ data: { server_url: "https://patchmon.example.com" } }),
		),
		checkServerUrlCors: vi.fn(),
	},
}));

const renderWizardAtCommandStep = async () => {
	const queryClient = new QueryClient({
		defaultOptions: { queries: { retry: false } },
	});
	render(
		<QueryClientProvider client={queryClient}>
			<MemoryRouter>
				<AddHostWizard isOpen onClose={vi.fn()} />
			</MemoryRouter>
		</QueryClientProvider>,
	);
	fireEvent.click(screen.getByRole("button", { name: "Next" }));
	fireEvent.change(screen.getByPlaceholderText("server.example.com"), {
		target: { value: "web-01" },
	});
	fireEvent.click(screen.getByRole("button", { name: "Next" }));
	await screen.findByRole("button", { name: /Copy command/ });
};

describe("AddHostWizard CORS warning", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it("shows the warning above the install command when not allowed", async () => {
		settingsAPI.checkServerUrlCors.mockResolvedValue({
			data: {
				server_url: "https://patchmon.example.com",
				allowed: false,
				reason: "host_not_in_cors_origin",
			},
		});

		await renderWizardAtCommandStep();

		expect(await screen.findByRole("alert")).toHaveTextContent(
			"not listed in CORS_ORIGIN",
		);
		expect(
			screen.getByRole("button", { name: /Copy command/ }),
		).not.toBeDisabled();
	});

	it("hides the warning when allowed", async () => {
		settingsAPI.checkServerUrlCors.mockResolvedValue({
			data: {
				server_url: "https://patchmon.example.com",
				allowed: true,
				reason: "",
			},
		});

		await renderWizardAtCommandStep();

		await waitFor(() =>
			expect(settingsAPI.checkServerUrlCors).toHaveBeenCalled(),
		);
		expect(screen.queryByRole("alert")).not.toBeInTheDocument();
	});

	it("hides the warning on a 403", async () => {
		settingsAPI.checkServerUrlCors.mockRejectedValue({
			response: { status: 403 },
		});

		await renderWizardAtCommandStep();

		await waitFor(() =>
			expect(settingsAPI.checkServerUrlCors).toHaveBeenCalled(),
		);
		expect(screen.queryByRole("alert")).not.toBeInTheDocument();
	});
});
