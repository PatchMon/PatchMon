import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
	fireEvent,
	render,
	screen,
	waitFor,
	within,
} from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ToastProvider } from "../../contexts/ToastContext";
import EnvironmentSettings from "../../pages/settings/EnvironmentSettings";
import { settingsAPI } from "../../utils/api";

vi.mock("../../utils/api", () => ({
	settingsAPI: {
		getEnvironmentConfig: vi.fn(),
		updateEnvironmentConfig: vi.fn(),
	},
}));

const variable = (key, overrides) => ({
	category: "Server",
	key,
	effectiveValue: "",
	effectiveSource: "default",
	defaultValue: "",
	editable: true,
	conflict: false,
	description: "",
	...overrides,
});

const CASES = [
	{
		kind: "log level",
		key: "LOG_LEVEL",
		fields: { effectiveValue: "info", defaultValue: "info" },
		next: "debug",
	},
	{
		kind: "body limit",
		key: "JSON_BODY_LIMIT",
		fields: { effectiveValue: "5mb", defaultValue: "5mb" },
		next: "10mb",
	},
	{
		kind: "fixed options",
		key: "JWT_EXPIRES_IN",
		fields: { effectiveValue: "1h", defaultValue: "1h" },
		next: "2h",
	},
	{
		kind: "timezone",
		key: "TIMEZONE",
		fields: { effectiveValue: "UTC", defaultValue: "UTC" },
		next: "Europe/London",
	},
	{
		kind: "boolean",
		key: "ENABLE_HSTS",
		fields: { effectiveValue: "false", defaultValue: "false" },
		next: "true",
	},
	{
		kind: "free text",
		key: "CORS_ORIGIN",
		fields: {
			effectiveValue: "http://localhost:3000",
			defaultValue: "http://localhost:3000",
		},
		next: "https://patchmon.example.com",
	},
];

const renderPage = () => {
	const queryClient = new QueryClient({
		defaultOptions: { queries: { retry: false } },
	});
	return render(
		<QueryClientProvider client={queryClient}>
			<ToastProvider>
				<EnvironmentSettings />
			</ToastProvider>
		</QueryClientProvider>,
	);
};

const rowFor = async (key) =>
	(await screen.findAllByText(key)).map((el) => el.closest("tr")).find(Boolean);

const cardFor = async (key) =>
	(await screen.findAllByText(key))
		.map((el) => el.closest("[data-env-key]"))
		.find(Boolean);

beforeEach(() => {
	vi.clearAllMocks();
});

describe("Environment settings for a value set in .env", () => {
	it.each(CASES)("does not offer to edit a $kind variable", async (c) => {
		settingsAPI.getEnvironmentConfig.mockResolvedValue({
			data: {
				variables: [
					variable(c.key, {
						...c.fields,
						effectiveSource: "env",
						envValue: c.fields.effectiveValue,
					}),
				],
			},
		});
		renderPage();
		const row = await rowFor(c.key);
		expect(
			within(row).queryByRole("button", { name: /override|edit/i }),
		).toBeNull();
		expect(within(row).queryByRole("combobox")).toBeNull();
		expect(within(row).queryByRole("textbox")).toBeNull();
		expect(within(row).getByText(/set in \.env/i)).toBeInTheDocument();
		expect(settingsAPI.updateEnvironmentConfig).not.toHaveBeenCalled();
	});

	it("says the database value is ignored when both are set", async () => {
		settingsAPI.getEnvironmentConfig.mockResolvedValue({
			data: {
				variables: [
					variable("LOG_LEVEL", {
						effectiveValue: "warn",
						effectiveSource: "env",
						envValue: "warn",
						dbValue: "debug",
						conflict: true,
					}),
				],
			},
		});
		renderPage();
		const row = await rowFor("LOG_LEVEL");
		expect(within(row).getByText(/debug/)).toBeInTheDocument();
		expect(within(row).getByText(/ignored/i)).toBeInTheDocument();
	});
});

describe("Environment settings for a value held in the database", () => {
	it.each(
		CASES,
	)("saves a $kind variable and shows the server's message", async (c) => {
		const message =
			c.key === "LOG_LEVEL"
				? "Saved. Restart the application for this change to take effect."
				: "Saved. The new value is in effect.";
		settingsAPI.getEnvironmentConfig
			.mockResolvedValueOnce({
				data: {
					variables: [variable(c.key, { ...c.fields, effectiveSource: "db" })],
				},
			})
			.mockResolvedValue({
				data: {
					variables: [
						variable(c.key, {
							...c.fields,
							effectiveValue: c.next,
							effectiveSource: "db",
						}),
					],
				},
			});
		settingsAPI.updateEnvironmentConfig.mockResolvedValue({
			data: { message },
		});
		renderPage();
		const row = await rowFor(c.key);
		fireEvent.click(within(row).getByRole("button", { name: /edit/i }));
		const input =
			within(row).queryByRole("combobox") || within(row).getByRole("textbox");
		fireEvent.change(input, { target: { value: c.next } });
		fireEvent.click(within(row).getByRole("button", { name: "Save" }));

		await screen.findByText(message);
		expect(settingsAPI.updateEnvironmentConfig).toHaveBeenCalledWith(
			c.key,
			c.next,
		);
		await waitFor(() =>
			expect(within(row).getByText(c.next)).toBeInTheDocument(),
		);
	});

	it("shows the value the server resolved once the save completes", async () => {
		let resolveRefetch;
		settingsAPI.getEnvironmentConfig
			.mockResolvedValueOnce({
				data: {
					variables: [
						variable("MAX_LOGIN_ATTEMPTS", {
							effectiveValue: "5",
							defaultValue: "5",
						}),
					],
				},
			})
			.mockImplementationOnce(
				() =>
					new Promise((resolve) => {
						resolveRefetch = resolve;
					}),
			);
		settingsAPI.updateEnvironmentConfig.mockResolvedValue({
			data: { message: "Saved. The new value is in effect." },
		});
		renderPage();
		const row = await rowFor("MAX_LOGIN_ATTEMPTS");
		fireEvent.click(within(row).getByRole("button", { name: /edit/i }));
		fireEvent.change(within(row).getByRole("textbox"), {
			target: { value: "007" },
		});
		fireEvent.click(within(row).getByRole("button", { name: "Save" }));
		await waitFor(() => expect(resolveRefetch).toBeTypeOf("function"));
		expect(within(row).queryByText("007")).toBeNull();

		resolveRefetch({
			data: {
				variables: [
					variable("MAX_LOGIN_ATTEMPTS", {
						effectiveValue: "7",
						defaultValue: "5",
						effectiveSource: "db",
					}),
				],
			},
		});
		await screen.findByText("Saved. The new value is in effect.");
		await waitFor(() => expect(within(row).getByText("7")).toBeInTheDocument());
	});

	it("shows the server's error and keeps the editor open", async () => {
		settingsAPI.getEnvironmentConfig.mockResolvedValue({
			data: {
				variables: [
					variable("CORS_ORIGIN", {
						effectiveValue: "http://localhost:3000",
						effectiveSource: "db",
					}),
				],
			},
		});
		settingsAPI.updateEnvironmentConfig.mockRejectedValue({
			response: { data: { error: 'invalid origin "nope"' } },
		});
		renderPage();
		const row = await rowFor("CORS_ORIGIN");
		fireEvent.click(within(row).getByRole("button", { name: /edit/i }));
		fireEvent.change(within(row).getByRole("textbox"), {
			target: { value: "nope" },
		});
		fireEvent.click(within(row).getByRole("button", { name: "Save" }));
		await screen.findByText('invalid origin "nope"');
		expect(within(row).getByRole("textbox")).toHaveValue("nope");
	});
});

describe("Environment settings layout", () => {
	it("moves the description into an info tooltip next to the variable", async () => {
		settingsAPI.getEnvironmentConfig.mockResolvedValue({
			data: {
				variables: [
					variable("CORS_ORIGIN", {
						effectiveValue: "https://a.example.com",
						defaultValue: "http://localhost:3000",
						description: "Allowed browser origins",
					}),
				],
			},
		});
		renderPage();
		const row = await rowFor("CORS_ORIGIN");
		expect(within(row).queryByText("Allowed browser origins")).toBeNull();
		const info = within(row).getByRole("button", {
			name: "About CORS_ORIGIN",
		});
		fireEvent.mouseEnter(info);
		expect(screen.getByRole("tooltip")).toHaveTextContent(
			"Allowed browser origins",
		);
		expect(
			within(row).getByText("Default: http://localhost:3000"),
		).toBeInTheDocument();
	});

	it("does not repeat the default when it matches the value", async () => {
		settingsAPI.getEnvironmentConfig.mockResolvedValue({
			data: {
				variables: [
					variable("LOG_LEVEL", {
						effectiveValue: "info",
						defaultValue: "info",
					}),
				],
			},
		});
		renderPage();
		const row = await rowFor("LOG_LEVEL");
		expect(within(row).queryByText(/^Default:/)).toBeNull();
	});

	it("renders a mobile card with the same value, source and edit action", async () => {
		settingsAPI.getEnvironmentConfig.mockResolvedValue({
			data: {
				variables: [
					variable("PORT", {
						effectiveValue: "3001",
						defaultValue: "3001",
						effectiveSource: "db",
					}),
				],
			},
		});
		renderPage();
		const card = await cardFor("PORT");
		expect(within(card).getByText("3001")).toBeInTheDocument();
		expect(within(card).getByText("Database")).toBeInTheDocument();
		fireEvent.click(within(card).getByRole("button", { name: /edit/i }));
		expect(within(card).getByRole("textbox")).toHaveValue("3001");
	});

	it("keeps the default visible while a value is being edited", async () => {
		settingsAPI.getEnvironmentConfig.mockResolvedValue({
			data: {
				variables: [
					variable("JSON_BODY_LIMIT", {
						effectiveValue: "5mb",
						defaultValue: "5mb",
						effectiveSource: "db",
					}),
				],
			},
		});
		renderPage();
		const row = await rowFor("JSON_BODY_LIMIT");
		expect(within(row).queryByText("Default: 5mb")).toBeNull();
		fireEvent.click(within(row).getByRole("button", { name: /edit/i }));
		expect(within(row).getByText("Default: 5mb")).toBeInTheDocument();
	});
});
