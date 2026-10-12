import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import DiscordSettings from "../../pages/settings/DiscordSettings";

vi.mock("../../utils/api", () => ({
	discordAPI: {
		getSettings: vi.fn(),
		updateSettings: vi.fn(),
	},
}));

import { discordAPI } from "../../utils/api";

const renderSettings = () => {
	const queryClient = new QueryClient({
		defaultOptions: {
			queries: { retry: false },
			mutations: { retry: false },
		},
	});

	return render(
		<QueryClientProvider client={queryClient}>
			<DiscordSettings />
		</QueryClientProvider>,
	);
};

describe("DiscordSettings", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		discordAPI.getSettings.mockResolvedValue({
			data: {
				discord_oauth_enabled: true,
				discord_client_id: "client-1",
				discord_client_secret_set: true,
				discord_redirect_uri: "https://patchmon.test/discord/callback",
				discord_button_text: "Continue with Discord",
				discord_allow_registration: true,
				discord_required_guild_id: "guild-9",
			},
		});
		discordAPI.updateSettings.mockResolvedValue({ data: {} });
	});

	it("applies registration settings and clears the guild restriction", async () => {
		const user = userEvent.setup();
		renderSettings();

		const guildInput = await screen.findByLabelText(
			"Required Discord Server ID",
		);
		const registrationSwitch = screen.getByRole("switch", {
			name: "Allow Discord Registration",
		});
		expect(guildInput).toHaveValue("guild-9");
		expect(registrationSwitch).toHaveAttribute("aria-checked", "true");
		await user.click(
			screen.getByRole("button", { name: "Setup Instructions" }),
		);
		expect(screen.getByText("identify email guilds")).toBeInTheDocument();

		await user.clear(guildInput);
		expect(screen.getByText("identify email")).toBeInTheDocument();
		expect(screen.queryByText("identify email guilds")).not.toBeInTheDocument();
		await user.click(screen.getByRole("button", { name: "Apply" }));

		await waitFor(() =>
			expect(discordAPI.updateSettings).toHaveBeenCalledWith(
				expect.objectContaining({
					discord_allow_registration: true,
					discord_required_guild_id: "",
				}),
			),
		);
	});
});
