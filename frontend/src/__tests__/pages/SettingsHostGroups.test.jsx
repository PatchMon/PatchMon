import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import SettingsHostGroups from "../../pages/settings/SettingsHostGroups";

vi.mock("../../utils/api", () => ({
	hostGroupsAPI: {
		list: vi.fn(() => Promise.resolve({ data: [] })),
		create: vi.fn(),
		update: vi.fn(),
		delete: vi.fn(),
		getHosts: vi.fn(() => Promise.resolve({ data: [] })),
	},
}));

const renderPage = () => {
	const queryClient = new QueryClient({
		defaultOptions: { queries: { retry: false } },
	});
	return render(
		<QueryClientProvider client={queryClient}>
			<MemoryRouter>
				<SettingsHostGroups />
			</MemoryRouter>
		</QueryClientProvider>,
	);
};

describe("SettingsHostGroups modals", () => {
	it("portals the create modal above the sidebar and removes it on close", () => {
		const { container } = renderPage();
		fireEvent.click(screen.getByTitle("Create host group"));
		const overlay = screen
			.getByRole("heading", { name: "Create Host Group" })
			.closest(".fixed.inset-0");
		expect(container).not.toContainElement(overlay);
		expect(overlay.parentElement).toBe(document.body);
		expect(overlay).toHaveClass("z-[120]");
		fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
		expect(overlay).not.toBeInTheDocument();
	});
});
