import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import ServerUrlCorsWarning from "../../components/ServerUrlCorsWarning";
import ProtocolUrlTab from "../../components/settings/ProtocolUrlTab";
import { settingsAPI } from "../../utils/api";

vi.mock("../../utils/api", () => ({
	settingsAPI: {
		get: vi.fn(),
		update: vi.fn(),
		checkServerUrlCors: vi.fn(),
	},
}));

const renderWithClient = (ui) => {
	const queryClient = new QueryClient({
		defaultOptions: { queries: { retry: false } },
	});
	return render(
		<QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>,
	);
};

const corsResult = (data) => Promise.resolve({ data });

describe("ServerUrlCorsWarning", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it("shows the CORS_ORIGIN warning when the host is not allowed", async () => {
		settingsAPI.checkServerUrlCors.mockReturnValue(
			corsResult({
				server_url: "https://patchmon.example.com",
				allowed: false,
				reason: "host_not_in_cors_origin",
			}),
		);

		renderWithClient(<ServerUrlCorsWarning />);

		expect(await screen.findByRole("alert")).toHaveTextContent(
			"not listed in CORS_ORIGIN",
		);
	});

	it("shows the invalid URL variant when the Server URL cannot be parsed", async () => {
		settingsAPI.checkServerUrlCors.mockReturnValue(
			corsResult({
				server_url: "",
				allowed: false,
				reason: "server_url_invalid",
			}),
		);

		renderWithClient(<ServerUrlCorsWarning />);

		expect(await screen.findByRole("alert")).toHaveTextContent(
			"not a valid URL",
		);
	});

	it("renders nothing when the host is allowed", async () => {
		settingsAPI.checkServerUrlCors.mockReturnValue(
			corsResult({
				server_url: "https://patchmon.example.com",
				allowed: true,
				reason: "",
			}),
		);

		renderWithClient(<ServerUrlCorsWarning />);

		await waitFor(() =>
			expect(settingsAPI.checkServerUrlCors).toHaveBeenCalled(),
		);
		expect(screen.queryByRole("alert")).not.toBeInTheDocument();
	});

	it("renders nothing when the check is forbidden", async () => {
		settingsAPI.checkServerUrlCors.mockRejectedValue({
			response: { status: 403 },
		});

		const { container } = renderWithClient(<ServerUrlCorsWarning />);

		await waitFor(() =>
			expect(settingsAPI.checkServerUrlCors).toHaveBeenCalledTimes(1),
		);
		expect(screen.queryByRole("alert")).not.toBeInTheDocument();
		expect(container).toBeEmptyDOMElement();
	});

	it("does not run the check when disabled", () => {
		renderWithClient(<ServerUrlCorsWarning enabled={false} />);

		expect(settingsAPI.checkServerUrlCors).not.toHaveBeenCalled();
	});
});

describe("ProtocolUrlTab CORS warning", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		settingsAPI.get.mockReturnValue(
			Promise.resolve({
				data: {
					server_protocol: "https",
					server_host: "patchmon.example.com",
					server_port: 443,
				},
			}),
		);
	});

	it("shows the warning under the Server URL fields when not allowed", async () => {
		settingsAPI.checkServerUrlCors.mockReturnValue(
			corsResult({
				server_url: "https://patchmon.example.com",
				allowed: false,
				reason: "host_not_in_cors_origin",
			}),
		);

		renderWithClient(<ProtocolUrlTab />);

		expect(await screen.findByRole("alert")).toHaveTextContent(
			"not listed in CORS_ORIGIN",
		);
	});

	it("hides the warning when allowed", async () => {
		settingsAPI.checkServerUrlCors.mockReturnValue(
			corsResult({
				server_url: "https://patchmon.example.com",
				allowed: true,
				reason: "",
			}),
		);

		renderWithClient(<ProtocolUrlTab />);

		expect(await screen.findByText("Server Configuration")).toBeInTheDocument();
		await waitFor(() =>
			expect(settingsAPI.checkServerUrlCors).toHaveBeenCalled(),
		);
		expect(screen.queryByRole("alert")).not.toBeInTheDocument();
	});

	it("hides the warning on a 403", async () => {
		settingsAPI.checkServerUrlCors.mockRejectedValue({
			response: { status: 403 },
		});

		renderWithClient(<ProtocolUrlTab />);

		expect(await screen.findByText("Server Configuration")).toBeInTheDocument();
		await waitFor(() =>
			expect(settingsAPI.checkServerUrlCors).toHaveBeenCalled(),
		);
		expect(screen.queryByRole("alert")).not.toBeInTheDocument();
	});
});
