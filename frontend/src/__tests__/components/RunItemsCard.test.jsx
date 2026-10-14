import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import RunItemsCard from "../../components/patching/RunItemsCard";

const ITEMS = [
	{
		kind: "wua",
		identifier: "7f1b0c5e-0000-0000-0000-000000000001",
		title: "2026-10 Cumulative Update for Windows 11 (KB5044284)",
		kb: "KB5044284",
		status: "failed",
		hresult: "0x800F0922",
		message: "Installation failed",
		reboot_required: false,
	},
	{
		kind: "winget",
		identifier: "7zip.7zip",
		title: "7-Zip 24.08 (x64)",
		status: "installed",
		exit_code: 0,
		reboot_required: true,
	},
];

describe("RunItemsCard", () => {
	it("renders nothing without items", () => {
		const { container } = render(<RunItemsCard items={[]} />);
		expect(container).toBeEmptyDOMElement();
		const { container: c2 } = render(<RunItemsCard />);
		expect(c2).toBeEmptyDOMElement();
	});

	it("renders every item in both the desktop table and the mobile cards", () => {
		render(<RunItemsCard items={ITEMS} />);
		const rows = screen.getAllByTestId("run-item-row");
		const cards = screen.getAllByTestId("run-item-card");
		expect(rows).toHaveLength(2);
		expect(cards).toHaveLength(2);

		const table = rows[0].closest("table");
		expect(table.parentElement).toHaveClass("hidden", "md:block");
		expect(cards[0].parentElement).toHaveClass("md:hidden");

		for (const view of [rows[0], cards[0]]) {
			const w = within(view);
			expect(w.getByText(/KB5044284\)$/)).toBeInTheDocument();
			expect(w.getByText(/^Windows Update/)).toBeInTheDocument();
			expect(w.getByText("Failed")).toHaveClass("badge-danger");
			expect(w.getByText("0x800F0922")).toBeInTheDocument();
		}
		for (const view of [rows[1], cards[1]]) {
			const w = within(view);
			expect(w.getByText(/^winget/)).toBeInTheDocument();
			expect(w.getByText("Installed")).toHaveClass("badge-success");
			expect(w.getByText("Required")).toBeInTheDocument();
		}
	});

	it("uses left-aligned headers", () => {
		render(<RunItemsCard items={ITEMS} />);
		for (const th of screen.getAllByRole("columnheader")) {
			expect(th).toHaveClass("text-left");
		}
	});

	it("gives the name the flexible width, truncated with a tooltip", () => {
		render(<RunItemsCard items={ITEMS} />);
		const table = screen.getAllByTestId("run-item-row")[0].closest("table");
		expect(table).toHaveClass("table-fixed");
		expect(table.parentElement).toHaveClass("overflow-x-auto");
		const name = within(screen.getAllByTestId("run-item-row")[0]).getByTitle(
			ITEMS[0].title,
		);
		expect(name).toHaveClass("truncate");
	});

	it("shows the result code once when the message already contains it", () => {
		render(
			<RunItemsCard
				items={[
					{
						kind: "winget",
						identifier: "Notepad++.Notepad++",
						title: "Notepad++ (ARM 64-bit)",
						status: "failed",
						exit_code: -2147012889,
						message: "winget exited with 0x80072EE7",
					},
				]}
			/>,
		);
		const row = screen.getByTestId("run-item-row");
		expect(row.textContent.match(/0x80072EE7/gi)).toHaveLength(1);
	});

	it("shows a skipped pinned app as Skipped with its message", () => {
		render(
			<RunItemsCard
				items={[
					{
						kind: "winget",
						identifier: "7zip.7zip",
						title: "7-Zip 26.04 (arm64)",
						status: "skipped",
						exit_code: -1978335166,
						message: "Pinned in winget; not upgraded",
					},
				]}
			/>,
		);
		const row = within(screen.getByTestId("run-item-row"));
		expect(row.getByText("Skipped")).toHaveClass("badge-secondary");
		expect(row.queryByText("Failed")).toBeNull();
		expect(row.getByText(/Pinned in winget/)).toBeInTheDocument();
	});

	it("shows a skipped Store app as Skipped with its message", () => {
		render(
			<RunItemsCard
				items={[
					{
						kind: "winget",
						identifier: "Microsoft.WindowsTerminal",
						title: "Windows Terminal",
						status: "skipped",
						message:
							"Microsoft Store app: it updates through the Store for each user, not as SYSTEM",
					},
				]}
			/>,
		);
		const row = within(screen.getByTestId("run-item-row"));
		expect(row.getByText("Skipped")).toHaveClass("badge-secondary");
		expect(row.getByText(/updates through the Store/)).toBeInTheDocument();
	});
});
