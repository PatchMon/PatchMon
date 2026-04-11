import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import DiscordSettings from "../../../pages/settings/DiscordSettings";
import { discordAPI } from "../../../utils/api";

vi.mock("../../../components/SettingsLayout", () => ({
	default: ({ children }) => <div>{children}</div>,
}));

vi.mock("../../../components/DiscordIcon", () => ({
	default: (props) => <div {...props} />,
}));

vi.mock("../../../utils/api", () => ({
	discordAPI: {
		getSettings: vi.fn(),
		updateSettings: vi.fn(),
	},
}));

const baseSettings = {
	discord_oauth_enabled: false,
	discord_client_id: "existing-client-id",
	discord_client_secret_set: false,
	discord_redirect_uri: "https://example.com/api/v1/auth/discord/callback",
	discord_button_text: "Login with Discord",
	discord_allow_registration: false,
	discord_required_guild_id: null,
};

describe("DiscordSettings", () => {
	let queryClient;

	beforeEach(() => {
		queryClient = new QueryClient({
			defaultOptions: {
				queries: { retry: false },
				mutations: { retry: false },
			},
		});

		discordAPI.getSettings.mockResolvedValue({ data: baseSettings });
		discordAPI.updateSettings.mockImplementation(async (payload) => ({
			data: {
				...baseSettings,
				...payload,
			},
		}));
	});

	const renderComponent = () =>
		render(
			<QueryClientProvider client={queryClient}>
				<DiscordSettings />
			</QueryClientProvider>,
		);

	it("keeps focus while typing and only saves after blur", async () => {
		renderComponent();

		const clientIdInput = await screen.findByLabelText(/client id/i);

		clientIdInput.focus();
		expect(clientIdInput).toHaveFocus();

		fireEvent.change(clientIdInput, { target: { value: "u" } });
		fireEvent.change(clientIdInput, { target: { value: "up" } });
		fireEvent.change(clientIdInput, { target: { value: "updated-client-id" } });

		expect(clientIdInput).toHaveFocus();
		expect(clientIdInput).toHaveValue("updated-client-id");
		expect(discordAPI.updateSettings).not.toHaveBeenCalled();

		fireEvent.blur(clientIdInput);

		await waitFor(() => {
			expect(discordAPI.updateSettings).toHaveBeenCalledTimes(1);
			expect(discordAPI.updateSettings).toHaveBeenCalledWith({
				discord_client_id: "updated-client-id",
			});
		});
	});
});
