/**
 * Unit tests for Compliance Dashboard Page
 */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import { BrowserRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
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
				<ToastProvider>
					<BrowserRouter>
						<Compliance />
					</BrowserRouter>
				</ToastProvider>
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
				expect(screen.getByText("10")).toBeInTheDocument();
			});
		});

		it("should display average compliance score", async () => {
			complianceAPI.getDashboard.mockResolvedValue({ data: mockDashboardData });

			renderComponent();

			await waitFor(() => {
				// Component uses Math.round() so 78.5 becomes 79%
				expect(screen.getByText(/79%/)).toBeInTheDocument();
			});
		});

		it("should display compliant hosts count", async () => {
			complianceAPI.getDashboard.mockResolvedValue({ data: mockDashboardData });

			renderComponent();

			await waitFor(() => {
				expect(screen.getByText("6")).toBeInTheDocument();
			});
		});

		it("should display warning hosts count", async () => {
			complianceAPI.getDashboard.mockResolvedValue({ data: mockDashboardData });

			renderComponent();

			await waitFor(() => {
				expect(screen.getByText("2")).toBeInTheDocument();
			});
		});

		it("should display critical hosts count", async () => {
			complianceAPI.getDashboard.mockResolvedValue({ data: mockDashboardData });

			renderComponent();

			await waitFor(() => {
				expect(screen.getByText("3")).toBeInTheDocument();
			});
		});

		it("should display recent scans section", async () => {
			complianceAPI.getDashboard.mockResolvedValue({ data: mockDashboardData });

			renderComponent();

			await waitFor(() => {
				expect(screen.getByText(/Recent Scans/i)).toBeInTheDocument();
			});
		});

		it("should display recent scan entries", async () => {
			complianceAPI.getDashboard.mockResolvedValue({ data: mockDashboardData });

			renderComponent();

			await waitFor(() => {
				expect(screen.getByText("web-server-01")).toBeInTheDocument();
				expect(screen.getByText("db-server-01")).toBeInTheDocument();
			});
		});

		it("should display profile names in recent scans", async () => {
			complianceAPI.getDashboard.mockResolvedValue({ data: mockDashboardData });

			renderComponent();

			await waitFor(() => {
				expect(screen.getByText("CIS Ubuntu 22.04 L1")).toBeInTheDocument();
				expect(screen.getByText("CIS Docker")).toBeInTheDocument();
			});
		});

		it("should display needs attention section", async () => {
			complianceAPI.getDashboard.mockResolvedValue({ data: mockDashboardData });

			renderComponent();

			await waitFor(() => {
				expect(screen.getByText(/Needs Attention/i)).toBeInTheDocument();
			});
		});

		it("should display worst performing hosts", async () => {
			complianceAPI.getDashboard.mockResolvedValue({ data: mockDashboardData });

			renderComponent();

			await waitFor(() => {
				expect(screen.getByText("legacy-server")).toBeInTheDocument();
			});
		});
	});

	describe("Empty State", () => {
		it("should display empty state when no recent scans", async () => {
			complianceAPI.getDashboard.mockResolvedValue({
				data: {
					summary: {
						total_hosts: 0,
						average_score: 0,
						compliant: 0,
						warning: 0,
						critical: 0,
						unscanned: 0,
					},
					recent_scans: [],
					worst_hosts: [],
				},
			});

			renderComponent();

			await waitFor(() => {
				expect(
					screen.getByText(/No.*scan.*available|No.*scans found/i),
				).toBeInTheDocument();
			});
		});

		it("should display empty state when no worst hosts", async () => {
			complianceAPI.getDashboard.mockResolvedValue({
				data: {
					summary: mockDashboardData.summary,
					recent_scans: mockDashboardData.recent_scans,
					worst_hosts: [],
				},
			});

			renderComponent();

			await waitFor(() => {
				expect(
					screen.getByText(/No.*hosts with low scores/i),
				).toBeInTheDocument();
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
		it("should have links to host details in recent scans", async () => {
			complianceAPI.getDashboard.mockResolvedValue({ data: mockDashboardData });

			renderComponent();

			await waitFor(() => {
				const links = screen.getAllByRole("link");
				const hostLinks = links.filter((link) =>
					link.getAttribute("href")?.includes("/hosts/"),
				);
				expect(hostLinks.length).toBeGreaterThan(0);
			});
		});
	});
});
