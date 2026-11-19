import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import Integrations from "../../pages/settings/Integrations";
import api from "../../utils/api";

vi.mock("../../utils/api", () => ({
	default: { get: vi.fn(), post: vi.fn(), patch: vi.fn() },
	settingsAPI: {
		get: vi.fn().mockResolvedValue({ data: {} }),
		getServerUrl: vi.fn().mockResolvedValue({ data: {} }),
	},
	formatDate: () => "Nov 19, 2026",
}));

vi.mock("../../contexts/ConfirmContext", () => ({
	useConfirm: () => vi.fn(),
}));

vi.mock("../../contexts/ToastContext", () => ({
	useToast: () => ({ error: vi.fn(), success: vi.fn() }),
}));

const credential = {
	id: "credential-1",
	token_name: "Deployment tool",
	token_key: "patchmon_ae_test",
	is_active: true,
	max_hosts_per_day: 100,
	allowed_ip_ranges: [],
	metadata: { integration_type: "api" },
	scopes: { host: ["get", "put", "patch", "update"] },
};

const renderCredentials = async (tokens = []) => {
	api.get.mockImplementation((path) =>
		Promise.resolve({ data: path === "/auto-enrollment/tokens" ? tokens : [] }),
	);
	const client = new QueryClient({
		defaultOptions: { queries: { retry: false } },
	});

	render(
		<QueryClientProvider client={client}>
			<MemoryRouter>
				<Integrations />
			</MemoryRouter>
		</QueryClientProvider>,
	);
	// Both navigation layouts are rendered; use the first scoped credentials tab.
	fireEvent.click(
		screen.getAllByRole("button", { name: "Scoped Credentials" })[0],
	);
	await screen.findByText(
		tokens.length ? "Deployment tool" : "No scoped credentials created yet.",
	);
};

const checkbox = (name) => screen.getByRole("checkbox", { name });

beforeEach(() => {
	vi.clearAllMocks();
	api.post.mockResolvedValue({ data: {} });
	api.patch.mockResolvedValue({ data: {} });
});

describe("Integration credential scopes", () => {
	it("creates credentials with independently selected scopes and no default delete permission", async () => {
		await renderCredentials();
		fireEvent.click(screen.getByRole("button", { name: "New Credential" }));
		expect(screen.getAllByRole("checkbox")).toHaveLength(10);
		for (const input of screen.getAllByRole("checkbox")) {
			expect(input).not.toBeChecked();
		}

		fireEvent.change(screen.getByLabelText("Token Name *"), {
			target: { value: "Deployment tool" },
		});
		for (const name of [
			"Host: get",
			"Host: delete",
			"Host Group: get",
			"Host Group: create",
			"Host Group: update",
			"Enrollment Token: get",
			"Enrollment Token: create",
			"Enrollment Token: update",
		]) {
			fireEvent.click(checkbox(name));
		}
		fireEvent.click(screen.getByRole("button", { name: "Create Token" }));

		await waitFor(() =>
			expect(api.post).toHaveBeenCalledWith(
				"/auto-enrollment/tokens",
				expect.objectContaining({
					metadata: { integration_type: "api" },
					scopes: {
						host: ["get", "delete"],
						host_group: ["get", "create", "update"],
						enrollment_token: ["get", "create", "update"],
					},
				}),
			),
		);
	});

	it("adds and removes new scopes while preserving legacy host permissions on edit", async () => {
		await renderCredentials([credential]);
		fireEvent.click(screen.getByRole("button", { name: "Edit" }));
		expect(checkbox("Host: get")).toBeChecked();
		expect(checkbox("Host Group: delete")).not.toBeChecked();
		expect(checkbox("Enrollment Token: delete")).not.toBeChecked();

		fireEvent.click(checkbox("Host Group: update"));
		fireEvent.click(checkbox("Host Group: create"));
		fireEvent.click(checkbox("Host Group: create"));
		fireEvent.click(checkbox("Host Group: delete"));
		fireEvent.click(checkbox("Enrollment Token: create"));
		fireEvent.click(checkbox("Enrollment Token: delete"));
		fireEvent.click(screen.getByRole("button", { name: "Update Credential" }));

		await waitFor(() =>
			expect(api.patch).toHaveBeenCalledWith(
				"/auto-enrollment/tokens/credential-1",
				expect.objectContaining({
					scopes: {
						host: ["get", "put", "patch", "update"],
						host_group: ["update", "delete"],
						enrollment_token: ["create", "delete"],
					},
				}),
			),
		);
		expect(credential.scopes).toEqual({
			host: ["get", "put", "patch", "update"],
		});
	});

	it("allows an existing credential without scopes to receive new permissions", async () => {
		await renderCredentials([{ ...credential, scopes: null }]);
		fireEvent.click(screen.getByRole("button", { name: "Edit" }));
		fireEvent.click(checkbox("Enrollment Token: update"));
		fireEvent.click(screen.getByRole("button", { name: "Update Credential" }));

		await waitFor(() =>
			expect(api.patch).toHaveBeenCalledWith(
				"/auto-enrollment/tokens/credential-1",
				expect.objectContaining({
					scopes: { host: [], enrollment_token: ["update"] },
				}),
			),
		);
	});

	it("does not carry delete permissions from a canceled edit into a new credential", async () => {
		await renderCredentials([
			{
				...credential,
				scopes: {
					host: ["get", "delete"],
					host_group: ["delete"],
					enrollment_token: ["delete"],
				},
			},
		]);
		fireEvent.click(screen.getByRole("button", { name: "Edit" }));
		expect(checkbox("Host Group: delete")).toBeChecked();
		fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
		fireEvent.click(screen.getByRole("button", { name: "New Credential" }));
		for (const input of screen.getAllByRole("checkbox")) {
			expect(input).not.toBeChecked();
		}
	});
});
