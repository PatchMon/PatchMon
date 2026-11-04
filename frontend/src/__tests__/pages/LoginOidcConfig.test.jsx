import { act, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import Login from "../../pages/Login";

vi.mock("../../contexts/AuthContext", () => ({
	useAuth: () => ({
		login: vi.fn(),
		setAuthState: vi.fn(),
	}),
}));

const deferred = () => {
	let resolve;
	const promise = new Promise((resolvePromise) => {
		resolve = resolvePromise;
	});
	return { promise, resolve };
};

const jsonResponse = (body) => ({
	ok: true,
	json: vi.fn().mockResolvedValue(body),
});

const loginSettings = {
	signup_enabled: false,
	show_github_version_on_login: false,
	discord: { enabled: false },
};

const renderLogin = (oidcResponse) => {
	vi.stubGlobal(
		"fetch",
		vi.fn((url) => {
			if (url === "/api/v1/settings/login-settings") {
				return Promise.resolve(jsonResponse(loginSettings));
			}
			if (url === "/api/v1/auth/oidc/config") {
				return oidcResponse;
			}
			return Promise.resolve({ ok: false, status: 404 });
		}),
	);

	return render(
		<MemoryRouter>
			<Login />
		</MemoryRouter>,
	);
};

afterEach(() => {
	vi.unstubAllGlobals();
	vi.restoreAllMocks();
});

describe("Login OIDC configuration", () => {
	it("does not render local credentials while the OIDC policy is loading", async () => {
		const oidcConfig = deferred();
		renderLogin(oidcConfig.promise);

		expect(screen.getByRole("status")).toHaveTextContent(
			"Checking sign-in options",
		);
		expect(
			screen.queryByLabelText("Username or Email"),
		).not.toBeInTheDocument();
		expect(screen.queryByLabelText("Password")).not.toBeInTheDocument();
		expect(
			screen.queryByRole("button", { name: "Sign in" }),
		).not.toBeInTheDocument();

		await act(async () => {
			oidcConfig.resolve(
				jsonResponse({
					enabled: true,
					buttonText: "Continue with SSO",
					disableLocalAuth: true,
					passwordFallback: false,
				}),
			);
			await oidcConfig.promise;
		});

		expect(
			screen.getByRole("button", { name: "Continue with SSO" }),
		).toBeInTheDocument();
		expect(
			screen.queryByLabelText("Username or Email"),
		).not.toBeInTheDocument();
		expect(screen.queryByLabelText("Password")).not.toBeInTheDocument();
	});

	it("keeps local sign-in available after a successful policy response that allows it", async () => {
		const oidcConfig = deferred();
		renderLogin(oidcConfig.promise);

		await act(async () => {
			oidcConfig.resolve(
				jsonResponse({
					enabled: true,
					buttonText: "Continue with SSO",
					disableLocalAuth: false,
					passwordFallback: false,
				}),
			);
			await oidcConfig.promise;
		});

		expect(screen.getByLabelText("Username or Email")).toBeInTheDocument();
		expect(screen.getByLabelText("Password")).toBeInTheDocument();
		expect(screen.getByRole("button", { name: "Sign in" })).toBeInTheDocument();
	});

	it("keeps local credentials hidden when the OIDC policy cannot be loaded", async () => {
		const consoleError = vi
			.spyOn(console, "error")
			.mockImplementation(() => {});
		renderLogin(
			Promise.resolve({
				ok: false,
				status: 503,
			}),
		);

		expect(await screen.findByRole("alert")).toHaveTextContent(
			"Unable to load sign-in options",
		);
		expect(
			screen.queryByLabelText("Username or Email"),
		).not.toBeInTheDocument();
		expect(screen.queryByLabelText("Password")).not.toBeInTheDocument();
		expect(
			screen.queryByRole("button", { name: "Sign in" }),
		).not.toBeInTheDocument();
		expect(consoleError).toHaveBeenCalled();
	});
});
