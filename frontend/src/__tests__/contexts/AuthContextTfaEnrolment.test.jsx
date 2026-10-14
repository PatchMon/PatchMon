import { act, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AUTH_PHASES } from "../../constants/authPhases";
import { AuthProvider, useAuth } from "../../contexts/AuthContext";

const json = (status, body) =>
	Promise.resolve({ ok: status < 400, status, json: () => body });

let api;
const Probe = () => {
	api = useAuth();
	return (
		<div>
			<span data-testid="phase">{api.authPhase}</span>
			<span data-testid="auth">{String(api.isAuthenticated())}</span>
		</div>
	);
};

const mountWith = (routes) => {
	global.fetch = vi.fn((url) => {
		const handler = routes[url];
		return handler ? handler() : json(404, {});
	});
	render(
		<AuthProvider>
			<Probe />
		</AuthProvider>,
	);
};

const user = { id: "u1", username: "alice", role: "user" };

describe("AuthContext forced 2FA enrolment", () => {
	afterEach(() => {
		localStorage.clear();
		vi.restoreAllMocks();
	});

	it("enters the enrolment phase on bootstrap and skips permissions", async () => {
		mountWith({
			"/api/v1/auth/profile": () =>
				json(200, { user: { ...user, tfa_enrolment_required: true } }),
		});
		await waitFor(() =>
			expect(screen.getByTestId("phase")).toHaveTextContent(
				AUTH_PHASES.TFA_ENROLMENT_REQUIRED,
			),
		);
		expect(screen.getByTestId("auth")).toHaveTextContent("false");
		expect(global.fetch).not.toHaveBeenCalledWith(
			"/api/v1/permissions/user-permissions",
			expect.anything(),
		);
	});

	it("enters the enrolment phase from a password login, then READY once enrolled", async () => {
		mountWith({
			"/api/v1/auth/profile": () => json(401, {}),
			"/api/v1/settings/login-settings": () =>
				json(200, { hasAdminUsers: true }),
		});
		await waitFor(() =>
			expect(screen.getByTestId("phase")).toHaveTextContent(AUTH_PHASES.READY),
		);

		global.fetch.mockImplementation((url) => {
			if (url === "/api/v1/auth/login")
				return json(200, { tfaEnrolmentRequired: true, token: "t", user });
			if (url === "/api/v1/auth/profile")
				return json(200, { user: { ...user, tfa_enrolment_required: false } });
			if (url === "/api/v1/permissions/user-permissions")
				return json(200, { can_view_dashboard: true });
			if (url === "/api/v1/me/context") return json(200, {});
			return json(404, {});
		});

		let result;
		await act(async () => {
			result = await api.login("alice", "pw");
		});
		expect(result).toEqual({ success: true, tfaEnrolmentRequired: true });
		expect(screen.getByTestId("phase")).toHaveTextContent(
			AUTH_PHASES.TFA_ENROLMENT_REQUIRED,
		);
		expect(screen.getByTestId("auth")).toHaveTextContent("false");

		let done;
		await act(async () => {
			done = await api.completeTfaEnrolment();
		});
		expect(done).toBe(true);
		expect(screen.getByTestId("phase")).toHaveTextContent(AUTH_PHASES.READY);
		expect(screen.getByTestId("auth")).toHaveTextContent("true");
	});

	it("stays restricted if the server still reports enrolment pending", async () => {
		mountWith({
			"/api/v1/auth/profile": () =>
				json(200, { user: { ...user, tfa_enrolment_required: true } }),
		});
		await waitFor(() =>
			expect(screen.getByTestId("phase")).toHaveTextContent(
				AUTH_PHASES.TFA_ENROLMENT_REQUIRED,
			),
		);
		let done;
		await act(async () => {
			done = await api.completeTfaEnrolment();
		});
		expect(done).toBe(false);
		expect(screen.getByTestId("phase")).toHaveTextContent(
			AUTH_PHASES.TFA_ENROLMENT_REQUIRED,
		);
	});

	it("logout leaves the enrolment phase", async () => {
		mountWith({
			"/api/v1/auth/profile": () =>
				json(200, { user: { ...user, tfa_enrolment_required: true } }),
			"/api/v1/auth/logout": () => json(200, {}),
		});
		await waitFor(() =>
			expect(screen.getByTestId("phase")).toHaveTextContent(
				AUTH_PHASES.TFA_ENROLMENT_REQUIRED,
			),
		);
		await act(async () => {
			await api.logout();
		});
		expect(screen.getByTestId("phase")).toHaveTextContent(AUTH_PHASES.READY);
		expect(screen.getByTestId("auth")).toHaveTextContent("false");
	});
});
