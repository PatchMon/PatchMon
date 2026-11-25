import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { ToastProvider } from "../../contexts/ToastContext";
import Packages from "../../pages/Packages";
import { packagesAPI } from "../../utils/api";

vi.mock("../../contexts/AuthContext", () => ({
	useAuth: () => ({ canManageHosts: () => true }),
}));

vi.mock("../../utils/api", async (importOriginal) => {
	const actual = await importOriginal();
	const ok = (data) => vi.fn().mockResolvedValue({ data });
	return {
		...actual,
		packagesAPI: {
			...actual.packagesAPI,
			getAll: vi.fn(),
			getCategories: ok([]),
		},
		dashboardAPI: {
			...actual.dashboardAPI,
			getStats: ok({}),
			getHostDetail: ok({}),
			getHostOptions: ok([{ id: "win", os_type: "Windows" }]),
		},
	};
});

const pkg = (name, patchable) => ({
	id: name,
	name,
	packageHosts: [{ hostId: "win", patchable, needsUpdate: true }],
});

const renderPackages = (firstPage, secondPage) => {
	packagesAPI.getAll.mockImplementation(async ({ page, host }) => ({
		data: {
			packages: host === "win" ? (page === 2 ? secondPage : firstPage) : [],
			pagination: {
				total: secondPage ? 26 : firstPage.length,
				pages: secondPage ? 2 : 1,
			},
		},
	}));
	const client = new QueryClient({
		defaultOptions: { queries: { retry: false } },
	});
	return render(
		<QueryClientProvider client={client}>
			<MemoryRouter initialEntries={["/packages?host=win"]}>
				<ToastProvider>
					<Packages />
				</ToastProvider>
			</MemoryRouter>
		</QueryClientProvider>,
	);
};

describe("Packages selection on small screens", () => {
	it("toggles only eligible packages on the visible page and keeps other pages selected", async () => {
		const { container } = renderPackages(
			[pkg("page-one", true), pkg("not-patchable", false)],
			[pkg("page-two", true)],
		);
		fireEvent.click(
			await screen.findByText("Select all on page", { selector: "button" }),
		);
		expect(screen.getByText("1 package selected")).toBeInTheDocument();

		// Pagination uses an icon-only Next button (the right chevron).
		fireEvent.click(
			container.querySelector("button:has(.lucide-chevron-right)"),
		);
		await screen.findAllByText("page-two");
		fireEvent.click(
			screen.getByText("Select all on page", { selector: "button" }),
		);
		expect(screen.getByText("2 packages selected")).toBeInTheDocument();

		fireEvent.click(
			screen.getByText("Deselect all on page", { selector: "button" }),
		);
		expect(screen.getByText("1 package selected")).toBeInTheDocument();
		expect(
			within(screen.getByRole("table")).getByTitle("Select all on page"),
		).toBeEnabled();

		fireEvent.click(
			container.querySelector("button:has(.lucide-chevron-left)"),
		);
		await screen.findAllByText("page-one");
		const desktop = within(screen.getByRole("table"));
		fireEvent.click(desktop.getByTitle("Deselect all on page"));
		expect(screen.queryByText("1 package selected")).not.toBeInTheDocument();
		expect(
			screen.getByText("Select all on page", { selector: "button" }),
		).toBeEnabled();
	});

	it("disables the mobile control when no package on the page is eligible", async () => {
		renderPackages([pkg("not-patchable", false)]);
		expect(
			await screen.findByText("Select all on page", { selector: "button" }),
		).toBeDisabled();
		expect(
			within(screen.getByRole("table")).getByTitle("Select all on page"),
		).toBeDisabled();
	});
});
