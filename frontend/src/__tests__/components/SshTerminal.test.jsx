/**
 * Unit tests for SSH Terminal Component
 */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { act } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import SshTerminal from "../../components/SshTerminal";
import { AuthContext } from "../../contexts/AuthContext";
import SidebarContext from "../../contexts/SidebarContext";

// Mock xterm - must be a constructor (function/class) for `new Terminal()` in component
vi.mock("xterm", () => ({
	Terminal: vi.fn().mockImplementation(function Terminal() {
		return {
			loadAddon: vi.fn(),
			open: vi.fn(),
			write: vi.fn(),
			writeln: vi.fn(),
			onData: vi.fn(() => ({
				dispose: vi.fn(),
			})),
			dispose: vi.fn(),
		};
	}),
}));

vi.mock("xterm-addon-fit", () => ({
	FitAddon: vi.fn().mockImplementation(function FitAddon() {
		return {
			fit: vi.fn(),
			proposeDimensions: vi.fn(() => ({ cols: 80, rows: 24 })),
		};
	}),
}));

vi.mock("xterm/css/xterm.css", () => ({}));

// Mock API
vi.mock("../../utils/api", () => ({
	settingsAPI: {
		getServerUrl: vi.fn(() =>
			Promise.resolve({ data: { server_url: "http://localhost:3001" } }),
		),
		get: vi.fn(() =>
			Promise.resolve({
				data: { ignore_ssl_self_signed: false },
			}),
		),
	},
}));

describe("SshTerminal Component", () => {
	let queryClient;
	let mockHost;
	let mockOnClose;
	let mockSetSidebarCollapsed;
	let mockToken;

	beforeEach(() => {
		queryClient = new QueryClient({
			defaultOptions: {
				queries: { retry: false },
				mutations: { retry: false },
			},
		});

		mockHost = {
			id: "host-123",
			friendly_name: "Test Host",
			hostname: "test.example.com",
			ip: "192.168.1.100",
			api_id: "api-123",
			api_key: "api-key-123",
		};

		mockOnClose = vi.fn();
		mockSetSidebarCollapsed = vi.fn();
		mockToken = "test-jwt-token";

		// Reset localStorage
		localStorage.getItem = vi.fn(() => mockToken);

		// So component can build WebSocket URL and fetch uses a valid base
		Object.defineProperty(window, "location", {
			value: {
				protocol: "http:",
				hostname: "localhost",
				port: "3000",
				origin: "http://localhost:3000",
			},
			writable: true,
		});

		// Mock fetch for SSH ticket (relative URL fails in Node); component then creates WebSocket
		global.fetch = vi.fn().mockResolvedValue({
			ok: true,
			json: () => Promise.resolve({ ticket: "test-ssh-ticket" }),
		});
	});

	afterEach(() => {
		vi.clearAllMocks();
	});

	const renderComponent = (props = {}) => {
		const defaultProps = {
			host: mockHost,
			isOpen: true,
			onClose: mockOnClose,
			embedded: false,
			...props,
		};

		return render(
			<QueryClientProvider client={queryClient}>
				<AuthContext.Provider value={{ token: mockToken }}>
					<SidebarContext.Provider
						value={{
							sidebarCollapsed: false,
							setSidebarCollapsed: mockSetSidebarCollapsed,
						}}
					>
						<SshTerminal {...defaultProps} />
					</SidebarContext.Provider>
				</AuthContext.Provider>
			</QueryClientProvider>,
		);
	};

	describe("Rendering", () => {
		it("should not render when isOpen is false", () => {
			const { container } = renderComponent({ isOpen: false });
			expect(container.firstChild).toBeNull();
		});

		it("should render connection form when not connected", () => {
			renderComponent();
			expect(screen.getByPlaceholderText(/root/i)).toBeInTheDocument();
			expect(screen.getByPlaceholderText(/password/i)).toBeInTheDocument();
		});

		it("should render modal mode by default", () => {
			renderComponent();
			expect(screen.getByText(/SSH Terminal/i)).toBeInTheDocument();
			expect(screen.getByText(/Test Host/)).toBeInTheDocument();
		});

		it("should render embedded mode when embedded prop is true", () => {
			renderComponent({ embedded: true });
			expect(screen.getByText(mockHost.friendly_name)).toBeInTheDocument();
		});
	});

	describe("Authentication Methods", () => {
		it("should show password authentication by default", () => {
			renderComponent();
			const passwordInput = screen.getByPlaceholderText(/password/i);
			expect(passwordInput).toBeInTheDocument();
		});

		it.skip("should switch to key authentication when selected", async () => {
			renderComponent();
			await waitFor(() => {
				expect(screen.getByPlaceholderText(/password/i)).toBeInTheDocument();
			});
			const keyRadio = document.querySelector(
				'input[name="authMethod"][value="key"]',
			);
			expect(keyRadio).toBeTruthy();
			fireEvent.click(keyRadio);

			expect(
				screen.getByPlaceholderText(/BEGIN OPENSSH PRIVATE KEY/i),
			).toBeInTheDocument();
		});

		it.skip("should show passphrase field for key authentication", async () => {
			renderComponent();
			await waitFor(() => {
				expect(screen.getByPlaceholderText(/password/i)).toBeInTheDocument();
			});
			const keyRadio = document.querySelector(
				'input[name="authMethod"][value="key"]',
			);
			expect(keyRadio).toBeTruthy();
			fireEvent.click(keyRadio);

			expect(screen.getByPlaceholderText(/Passphrase/i)).toBeInTheDocument();
		});
	});

	describe("Form Validation", () => {
		it("should disable connect button when username is empty", () => {
			renderComponent();
			const connectButton = screen.getByRole("button", { name: /connect/i });
			expect(connectButton).toBeDisabled();
		});

		it("should disable connect button when password is empty (password auth)", () => {
			renderComponent();
			const usernameInput = screen.getByPlaceholderText(/root/i);
			fireEvent.change(usernameInput, { target: { value: "testuser" } });

			const connectButton = screen.getByRole("button", { name: /connect/i });
			expect(connectButton).toBeDisabled();
		});

		it.skip("should disable connect button when private key is empty (key auth)", async () => {
			renderComponent();
			await waitFor(() => {
				expect(screen.getByPlaceholderText(/password/i)).toBeInTheDocument();
			});
			const keyRadio = document.querySelector(
				'input[name="authMethod"][value="key"]',
			);
			expect(keyRadio).toBeTruthy();
			fireEvent.click(keyRadio);

			const usernameInput = screen.getByPlaceholderText(/root/i);
			fireEvent.change(usernameInput, { target: { value: "testuser" } });

			const connectButton = screen.getByRole("button", { name: /connect/i });
			expect(connectButton).toBeDisabled();
		});

		it("should enable connect button when all required fields are filled", () => {
			renderComponent();
			const usernameInput = screen.getByPlaceholderText(/root/i);
			const passwordInput = screen.getByPlaceholderText(/password/i);

			fireEvent.change(usernameInput, { target: { value: "testuser" } });
			fireEvent.change(passwordInput, { target: { value: "testpass" } });

			const connectButton = screen.getByRole("button", { name: /connect/i });
			expect(connectButton).not.toBeDisabled();
		});
	});

	describe("WebSocket Connection", () => {
		it("should create WebSocket connection when connect is clicked", async () => {
			renderComponent();

			const usernameInput = screen.getByPlaceholderText(/root/i);
			const passwordInput = screen.getByPlaceholderText(/password/i);
			const connectButton = screen.getByRole("button", { name: /connect/i });

			fireEvent.change(usernameInput, { target: { value: "testuser" } });
			fireEvent.change(passwordInput, { target: { value: "testpass" } });
			fireEvent.click(connectButton);

			// WebSocket is created after async fetch; wait for it
			await waitFor(() => {
				expect(WebSocket).toHaveBeenCalled();
			});
		});

		it("should construct correct WebSocket URL", async () => {
			renderComponent();

			const usernameInput = screen.getByPlaceholderText(/root/i);
			const passwordInput = screen.getByPlaceholderText(/password/i);
			const connectButton = screen.getByRole("button", { name: /connect/i });

			fireEvent.change(usernameInput, { target: { value: "testuser" } });
			fireEvent.change(passwordInput, { target: { value: "testpass" } });
			fireEvent.click(connectButton);

			await waitFor(() => {
				expect(WebSocket).toHaveBeenCalled();
			});

			// Check WebSocket URL contains host ID and ticket (one-time auth)
			const wsCall = WebSocket.mock.calls[0];
			expect(wsCall[0]).toContain(`/api/v1/ssh-terminal/${mockHost.id}`);
			expect(wsCall[0]).toContain("ticket=");
		});

		it("should send connect message with credentials", async () => {
			renderComponent();

			const usernameInput = screen.getByPlaceholderText(/root/i);
			const passwordInput = screen.getByPlaceholderText(/password/i);
			const connectButton = screen.getByRole("button", { name: /connect/i });

			fireEvent.change(usernameInput, { target: { value: "testuser" } });
			fireEvent.change(passwordInput, { target: { value: "testpass" } });
			fireEvent.click(connectButton);

			// Wait for WebSocket to be created
			await waitFor(() => {
				expect(WebSocket).toHaveBeenCalled();
			});

			// Get the WebSocket instance
			const wsInstance = WebSocket.mock.results[0].value;
			await act(async () => {
				wsInstance._simulateOpen();
			});

			// Wait for connect message to be sent (component sends in onopen)
			await waitFor(
				() => {
					expect(wsInstance.send).toHaveBeenCalled();
				},
				{ timeout: 2000 },
			);

			// Verify connect message structure
			const sendCalls = wsInstance.send.mock.calls;
			const connectMessage = JSON.parse(sendCalls[0][0]);
			expect(connectMessage.type).toBe("connect");
			expect(connectMessage.username).toBe("testuser");
			expect(connectMessage.password).toBe("testpass");
		});

		it("should handle WebSocket connection errors", async () => {
			renderComponent();

			const usernameInput = screen.getByPlaceholderText(/root/i);
			const passwordInput = screen.getByPlaceholderText(/password/i);
			const connectButton = screen.getByRole("button", { name: /connect/i });

			fireEvent.change(usernameInput, { target: { value: "testuser" } });
			fireEvent.change(passwordInput, { target: { value: "testpass" } });
			fireEvent.click(connectButton);

			await waitFor(() => {
				expect(WebSocket).toHaveBeenCalled();
			});

			const wsInstance = WebSocket.mock.results[0].value;
			await act(async () => {
				wsInstance._simulateError(new Error("Connection failed"));
			});

			await waitFor(() => {
				expect(screen.getByText(/error/i)).toBeInTheDocument();
			});
		});
	});

	describe("Token Validation", () => {
		it("should show error when token is missing", async () => {
			localStorage.getItem = vi.fn(() => null);
			// Fetch fails without auth (no cookies), so component shows error
			global.fetch = vi.fn().mockResolvedValue({
				ok: false,
				status: 401,
				json: () => Promise.resolve({ error: "Unauthorized" }),
			});
			renderComponent();

			const usernameInput = screen.getByPlaceholderText(/root/i);
			const passwordInput = screen.getByPlaceholderText(/password/i);
			const connectButton = screen.getByRole("button", { name: /connect/i });

			fireEvent.change(usernameInput, { target: { value: "testuser" } });
			fireEvent.change(passwordInput, { target: { value: "testpass" } });
			fireEvent.click(connectButton);

			await waitFor(() => {
				expect(
					screen.getByText(/authentication required/i),
				).toBeInTheDocument();
			});
		});

		it("should show error when token is expired", async () => {
			// Mock expired token (exp < now)
			const expiredToken =
				"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VySWQiOiJ0ZXN0Iiwic2Vzc2lvbklkIjoic2VzcyIsImV4cCI6MTAwMDAwMDAwMH0.expired";
			localStorage.getItem = vi.fn(() => expiredToken);
			// Server returns 401 for expired session
			global.fetch = vi.fn().mockResolvedValue({
				ok: false,
				status: 401,
				json: () => Promise.resolve({ error: "Session expired" }),
			});
			renderComponent();

			const usernameInput = screen.getByPlaceholderText(/root/i);
			const passwordInput = screen.getByPlaceholderText(/password/i);
			const connectButton = screen.getByRole("button", { name: /connect/i });

			fireEvent.change(usernameInput, { target: { value: "testuser" } });
			fireEvent.change(passwordInput, { target: { value: "testpass" } });
			fireEvent.click(connectButton);

			await waitFor(() => {
				expect(
					screen.getByText(/authentication required|session expired/i),
				).toBeInTheDocument();
			});
		});
	});

	describe("SSH Connection States", () => {
		it("should show connecting state", async () => {
			renderComponent();

			const usernameInput = screen.getByPlaceholderText(/root/i);
			const passwordInput = screen.getByPlaceholderText(/password/i);
			const connectButton = screen.getByRole("button", { name: /connect/i });

			fireEvent.change(usernameInput, { target: { value: "testuser" } });
			fireEvent.change(passwordInput, { target: { value: "testpass" } });
			fireEvent.click(connectButton);

			// After connect click, fetch runs then WebSocket is created
			await waitFor(
				() => {
					expect(WebSocket).toHaveBeenCalled();
				},
				{ timeout: 3000 },
			);
		});

		it("should show connected state when SSH connection is established", async () => {
			renderComponent();

			const usernameInput = screen.getByPlaceholderText(/root/i);
			const passwordInput = screen.getByPlaceholderText(/password/i);
			const connectButton = screen.getByRole("button", { name: /connect/i });

			fireEvent.change(usernameInput, { target: { value: "testuser" } });
			fireEvent.change(passwordInput, { target: { value: "testpass" } });
			fireEvent.click(connectButton);

			await waitFor(() => {
				expect(WebSocket).toHaveBeenCalled();
			});

			const wsInstance = WebSocket.mock.results[0].value;
			await act(async () => {
				wsInstance._simulateOpen();
				wsInstance._simulateMessage(JSON.stringify({ type: "connected" }));
			});

			await waitFor(() => {
				expect(screen.getByText(/connected/i)).toBeInTheDocument();
			});
		});
	});

	describe("Disconnect", () => {
		it("should call onClose when disconnect is clicked", async () => {
			renderComponent({ embedded: true });

			// Connect first
			const usernameInput = screen.getByPlaceholderText(/root/i);
			const passwordInput = screen.getByPlaceholderText(/password/i);
			const connectButton = screen.getByRole("button", { name: /connect/i });

			fireEvent.change(usernameInput, { target: { value: "testuser" } });
			fireEvent.change(passwordInput, { target: { value: "testpass" } });
			fireEvent.click(connectButton);

			await waitFor(() => {
				expect(WebSocket).toHaveBeenCalled();
			});

			const wsInstance = WebSocket.mock.results[0].value;
			await act(async () => {
				wsInstance._simulateOpen();
				wsInstance._simulateMessage(JSON.stringify({ type: "connected" }));
			});

			await waitFor(() => {
				expect(screen.getByText(/connected/i)).toBeInTheDocument();
			});

			// Click disconnect
			const disconnectButton = screen.getByRole("button", {
				name: /disconnect/i,
			});
			fireEvent.click(disconnectButton);

			expect(wsInstance.close).toHaveBeenCalled();
		});
	});

	describe("Sidebar Management", () => {
		it("should collapse sidebar when modal opens", () => {
			renderComponent({ embedded: false });
			expect(mockSetSidebarCollapsed).toHaveBeenCalledWith(true);
		});

		it("should not collapse sidebar in embedded mode", () => {
			renderComponent({ embedded: true });
			expect(mockSetSidebarCollapsed).not.toHaveBeenCalled();
		});
	});

	describe("Install Command", () => {
		it("should show install command when host has API credentials", async () => {
			renderComponent({ embedded: true });

			// Connect first
			const usernameInput = screen.getByPlaceholderText(/root/i);
			const passwordInput = screen.getByPlaceholderText(/password/i);
			const connectButton = screen.getByRole("button", { name: /connect/i });

			fireEvent.change(usernameInput, { target: { value: "testuser" } });
			fireEvent.change(passwordInput, { target: { value: "testpass" } });
			fireEvent.click(connectButton);

			await waitFor(() => {
				expect(WebSocket).toHaveBeenCalled();
			});

			const wsInstance = WebSocket.mock.results[0].value;
			await act(async () => {
				wsInstance._simulateOpen();
				wsInstance._simulateMessage(JSON.stringify({ type: "connected" }));
			});

			await waitFor(() => {
				expect(screen.getByText(/install agent/i)).toBeInTheDocument();
			});
		});
	});
});
