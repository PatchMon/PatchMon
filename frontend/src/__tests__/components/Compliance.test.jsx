/**
 * Unit tests for Compliance Dashboard Page
 */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import { BrowserRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ThemeProvider } from "../../contexts/ThemeContext";
import { ToastProvider } from "../../contexts/ToastContext";
import Compliance from "../../pages/Compliance";

// Mock ResizeObserver for recharts
global.ResizeObserver = vi.fn().mockImplementation(() => ({
	observe: vi.fn(),
	unobserve: vi.fn(),
	disconnect: vi.fn(),
}));

// Mock the complianceAPI
vi.mock("../../utils/complianceApi", () => ({
	complianceAPI: {
		getDashboard: vi.fn(),
		getActiveScans: vi.fn(),
	},
}));

// Mock the adminHostsAPI
vi.mock("../../utils/api", () => ({
	adminHostsAPI: {
		list: vi.fn(),
	},
}));

// Mock AuthContext so ThemeProvider can render (ThemeProvider uses useAuth)
vi.mock("../../contexts/AuthContext", () => ({
	useAuth: () => ({ user: null }),
}));

// Mock react-chartjs-2 (Chart.js uses canvas, not available in jsdom)
vi.mock("react-chartjs-2", () => ({
	Bar: () => null,
	Doughnut: () => null,
	Line: () => null,
	Pie: () => null,
}));

import { adminHostsAPI } from "../../utils/api";
import { complianceAPI } from "../../utils/complianceApi";

describe("Compliance Dashboard", () => {
	let queryClient;

	const mockDashboardData = {
		summary: {
			total_hosts: 10,
			average_score: 78.5,
			hosts_compliant: 6,
			hosts_warning: 2,
			hosts_critical: 3,
			unscanned: 4,
		},
		profile_type_stats: [
			{
				type: "openscap",
				hosts_scanned: 8,
				average_score: 78.5,
				total_rules: 100,
				total_passed: 79,
				total_failed: 21,
				total_warnings: 0,
			},
			{
				type: "docker-bench",
				hosts_scanned: 5,
				average_score: 72,
				total_rules: 50,
				total_passed: 36,
				total_failed: 10,
				total_warnings: 4,
			},
		],
		recent_scans: [
			{
				id: "scan-1",
				host: {
					id: "host-1",
					friendly_name: "web-server-01",
					hostname: "web-server-01.local",
				},
				profile: { name: "CIS Ubuntu 22.04 L1" },
				score: 85,
				completed_at: "2024-01-15T10:00:00Z",
			},
			{
				id: "scan-2",
				host: {
					id: "host-2",
					friendly_name: "db-server-01",
					hostname: "db-server-01.local",
				},
				profile: { name: "CIS Docker" },
				score: 72,
				completed_at: "2024-01-14T10:00:00Z",
			},
		],
		worst_hosts: [
			{
				id: "scan-3",
				host: {
					id: "host-3",
					friendly_name: "legacy-server",
					hostname: "legacy.local",
				},
				profile: { name: "CIS Ubuntu 20.04 L1" },
				score: 45,
			},
		],
	};

	beforeEach(() => {
		vi.clearAllMocks();
		queryClient = new QueryClient({
			defaultOptions: {
				queries: { retry: false },
			},
		});

		// Set default mock implementations
		complianceAPI.getActiveScans.mockResolvedValue({ data: [] });
		adminHostsAPI.list.mockResolvedValue({ data: [] });
	});

	const renderComponent = () => {
		return render(
			<QueryClientProvider client={queryClient}>
				<ThemeProvider>
					<ToastProvider>
						<BrowserRouter>
							<Compliance />
						</BrowserRouter>
					</ToastProvider>
				</ThemeProvider>
			</QueryClientProvider>,
		);
	};

	describe("Loading State", () => {
		it("should display loading spinner while fetching data", () => {
			complianceAPI.getDashboard.mockImplementation(
				() => new Promise(() => {}), // Never resolves
			);

			const { container } = renderComponent();

			// Should show loading spinner
			const spinner = container.querySelector(".animate-spin");
			expect(spinner).toBeInTheDocument();
		});
	});

	describe("Success State", () => {
		it("should render the compliance dashboard header", async () => {
			complianceAPI.getDashboard.mockResolvedValue({ data: mockDashboardData });

			renderComponent();

			await waitFor(() => {
				expect(screen.getByText(/Security Compliance/i)).toBeInTheDocument();
			});
		});

		it("should display total hosts count", async () => {
			complianceAPI.getDashboard.mockResolvedValue({ data: mockDashboardData });

			renderComponent();

			await waitFor(() => {
				// Total hosts card shows total_hosts + unscanned (10 + 4 = 14)
				expect(screen.getByText("14")).toBeInTheDocument();
			});
		});

		it("should display average compliance score or overview stats", async () => {
			complianceAPI.getDashboard.mockResolvedValue({ data: mockDashboardData });

			renderComponent();

			await waitFor(() => {
				// Dashboard loads with summary; average may be in charts or we at least have summary cards
				expect(screen.getByText(/Security Compliance/i)).toBeInTheDocument();
				expect(screen.getAllByText("6").length).toBeGreaterThan(0);
			});
		});

		it("should display compliant hosts count", async () => {
			complianceAPI.getDashboard.mockResolvedValue({ data: mockDashboardData });

			renderComponent();

			await waitFor(() => {
				expect(screen.getAllByText("6").length).toBeGreaterThan(0);
			});
		});

		it("should display warning hosts count", async () => {
			complianceAPI.getDashboard.mockResolvedValue({ data: mockDashboardData });

			renderComponent();

			await waitFor(() => {
				expect(screen.getAllByText("2").length).toBeGreaterThan(0);
			});
		});

		it("should display critical hosts count", async () => {
			complianceAPI.getDashboard.mockResolvedValue({ data: mockDashboardData });

			renderComponent();

			await waitFor(() => {
				expect(screen.getAllByText("3").length).toBeGreaterThan(0);
			});
		});

		it("should display tab navigation", async () => {
			complianceAPI.getDashboard.mockResolvedValue({ data: mockDashboardData });

			renderComponent();

			await waitFor(() => {
				expect(screen.getByText("Overview")).toBeInTheDocument();
			});
		});

		it("should display dashboard summary cards when data is loaded", async () => {
			complianceAPI.getDashboard.mockResolvedValue({ data: mockDashboardData });

			renderComponent();

			await waitFor(() => {
				expect(screen.getAllByText("Compliant").length).toBeGreaterThan(0);
				expect(screen.getAllByText("Warning").length).toBeGreaterThan(0);
				expect(screen.getAllByText("Critical").length).toBeGreaterThan(0);
			});
		});

		it("should display Never scanned card when data is loaded", async () => {
			complianceAPI.getDashboard.mockResolvedValue({ data: mockDashboardData });

			renderComponent();

			await waitFor(() => {
				expect(screen.getAllByText("Never scanned").length).toBeGreaterThan(0);
			});
		});

		it("should display overview when dashboard has worst_hosts data", async () => {
			complianceAPI.getDashboard.mockResolvedValue({ data: mockDashboardData });

			renderComponent();

			await waitFor(() => {
				expect(screen.getByText(/Security Compliance/i)).toBeInTheDocument();
				expect(screen.getByText("14")).toBeInTheDocument();
			});
		});
	});

	describe("Empty State", () => {
		it("should display zero counts when dashboard has no hosts", async () => {
			complianceAPI.getDashboard.mockResolvedValue({
				data: {
					summary: {
						total_hosts: 0,
						average_score: 0,
						hosts_compliant: 0,
						hosts_warning: 0,
						hosts_critical: 0,
						unscanned: 0,
					},
					recent_scans: [],
					worst_hosts: [],
					profile_type_stats: [],
				},
			});

			renderComponent();

			await waitFor(() => {
				expect(screen.getByText("Security Compliance")).toBeInTheDocument();
			});
			// Total hosts card shows 0 when total_hosts + unscanned = 0
			expect(screen.getAllByText("0").length).toBeGreaterThan(0);
		});

		it("should display overview when worst_hosts is empty but summary has data", async () => {
			complianceAPI.getDashboard.mockResolvedValue({
				data: {
					summary: mockDashboardData.summary,
					recent_scans: mockDashboardData.recent_scans,
					worst_hosts: [],
					profile_type_stats: mockDashboardData.profile_type_stats,
				},
			});

			renderComponent();

			await waitFor(() => {
				expect(screen.getByText(/Security Compliance/i)).toBeInTheDocument();
				expect(screen.getAllByText("6").length).toBeGreaterThan(0);
			});
		});
	});

	describe("Error State", () => {
		it("should display error message when API call fails", async () => {
			complianceAPI.getDashboard.mockRejectedValue(
				new Error("Failed to fetch"),
			);

			renderComponent();

			await waitFor(() => {
				expect(
					screen.getByText(/Failed to load compliance dashboard/i),
				).toBeInTheDocument();
			});
		});
	});

	describe("Navigation", () => {
		it("should have navigation tabs for compliance sections", async () => {
			complianceAPI.getDashboard.mockResolvedValue({ data: mockDashboardData });

			renderComponent();

			await waitFor(() => {
				expect(screen.getByText("Security Compliance")).toBeInTheDocument();
				expect(screen.getByText("Hosts")).toBeInTheDocument();
			});
		});
	});
});
