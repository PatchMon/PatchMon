import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import UsersTab from "../../components/settings/UsersTab";
import { adminUsersAPI } from "../../utils/api";

const toast = { success: vi.fn(), error: vi.fn() };
let currentUser = { id: "me", has_password: true };

vi.mock("../../contexts/AuthContext", () => ({
	useAuth: () => ({ user: currentUser }),
}));
vi.mock("../../contexts/ConfirmContext", () => ({
	useConfirm: () => vi.fn(),
}));
vi.mock("../../contexts/SettingsContext", () => ({
	useSettings: () => ({ settings: {} }),
}));
vi.mock("../../contexts/ToastContext", () => ({
	useToast: () => toast,
}));
vi.mock("../../utils/api", () => ({
	adminUsersAPI: {
		list: vi.fn(),
		resetTfa: vi.fn(),
		delete: vi.fn(),
		update: vi.fn(),
		resetPassword: vi.fn(),
	},
	permissionsAPI: { getRoles: vi.fn(() => Promise.resolve({ data: [] })) },
	settingsAPI: { get: vi.fn(() => Promise.resolve({ data: {} })) },
	formatDateOnly: () => "2026-01-01",
}));

const baseUser = {
	role: "user",
	is_active: true,
	created_at: "2026-01-01T00:00:00Z",
};

const setUsers = (meTfa) =>
	adminUsersAPI.list.mockResolvedValue({
		data: {
			data: [
				{
					...baseUser,
					id: "me",
					username: "me",
					email: "me@x",
					role: "admin",
					tfa_enabled: meTfa,
				},
				{
					...baseUser,
					id: "alice",
					username: "alice",
					email: "a@x",
					tfa_enabled: true,
				},
				{
					...baseUser,
					id: "bob",
					username: "bob",
					email: "b@x",
					tfa_enabled: false,
				},
			],
		},
	});

const renderTab = () => {
	const queryClient = new QueryClient({
		defaultOptions: { queries: { retry: false } },
	});
	return render(
		<QueryClientProvider client={queryClient}>
			<UsersTab />
		</QueryClientProvider>,
	);
};

const openFor = async (username) => {
	const [btn] = await screen.findAllByRole("button", {
		name: `Reset two-factor authentication for ${username}`,
	});
	fireEvent.click(btn);
	return screen.getByRole("dialog");
};

describe("UsersTab reset 2FA", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		currentUser = { id: "me", has_password: true };
		global.fetch = vi.fn(() =>
			Promise.resolve({ ok: true, json: () => ({ enabled: false }) }),
		);
	});

	it("offers the action only for other users with 2FA enabled", async () => {
		setUsers(true);
		renderTab();
		const alice = await screen.findAllByRole("button", {
			name: "Reset two-factor authentication for alice",
		});
		expect(alice).toHaveLength(2);
		expect(
			screen.queryByRole("button", {
				name: "Reset two-factor authentication for bob",
			}),
		).toBeNull();
		expect(
			screen.queryByRole("button", {
				name: "Reset two-factor authentication for me",
			}),
		).toBeNull();
	});

	it("requires the actor's password and code before calling the API", async () => {
		setUsers(true);
		renderTab();
		const dialog = await openFor("alice");
		expect(dialog.closest(".fixed.inset-0").parentElement).toBe(document.body);

		fireEvent.click(screen.getByRole("button", { name: "Reset 2FA" }));
		expect(screen.getByRole("alert")).toHaveTextContent(
			"Enter your password to confirm",
		);
		fireEvent.change(screen.getByLabelText("Your password"), {
			target: { value: "pw" },
		});
		fireEvent.click(screen.getByRole("button", { name: "Reset 2FA" }));
		expect(screen.getByRole("alert")).toHaveTextContent(
			"Enter your two-factor code to confirm",
		);
		expect(adminUsersAPI.resetTfa).not.toHaveBeenCalled();
	});

	it("shows the server error and keeps the modal open", async () => {
		setUsers(false);
		adminUsersAPI.resetTfa.mockRejectedValue({
			response: { data: { error: "Your password is incorrect" } },
		});
		renderTab();
		await openFor("alice");
		expect(screen.queryByLabelText(/two-factor code/)).toBeNull();
		fireEvent.change(screen.getByLabelText("Your password"), {
			target: { value: "wrong" },
		});
		fireEvent.click(screen.getByRole("button", { name: "Reset 2FA" }));
		expect(await screen.findByRole("alert")).toHaveTextContent(
			"Your password is incorrect",
		);
		expect(screen.getByRole("dialog")).toBeInTheDocument();
	});

	it("sends password and code, then closes with a toast", async () => {
		setUsers(true);
		adminUsersAPI.resetTfa.mockResolvedValue({ data: {} });
		renderTab();
		await openFor("alice");
		fireEvent.change(screen.getByLabelText("Your password"), {
			target: { value: "pw" },
		});
		fireEvent.change(screen.getByLabelText("Your two-factor code"), {
			target: { value: " 123456 " },
		});
		fireEvent.click(screen.getByRole("button", { name: "Reset 2FA" }));
		await waitFor(() =>
			expect(adminUsersAPI.resetTfa).toHaveBeenCalledWith("alice", {
				password: "pw",
				tfaToken: "123456",
			}),
		);
		await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
		expect(toast.success).toHaveBeenCalled();
	});

	it("explains why an SSO-only admin cannot confirm", async () => {
		currentUser = { id: "me", has_password: false };
		setUsers(false);
		renderTab();
		await openFor("alice");
		expect(screen.queryByLabelText("Your password")).toBeNull();
		expect(screen.getByRole("button", { name: "Reset 2FA" })).toBeDisabled();
	});
});
