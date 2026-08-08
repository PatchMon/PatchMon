/**
 * Unit tests for ConfirmContext
 *
 * These cover the promise contract that every delete flow in the app now
 * depends on: a confirm must always settle exactly once, and must never leave
 * its caller awaiting forever.
 */

import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { ConfirmProvider, useConfirm } from "../../contexts/ConfirmContext";

const Trigger = ({ onResult, options, label = "Delete thing" }) => {
	const confirm = useConfirm();
	return (
		<button
			type="button"
			onClick={async () => onResult(await confirm(options))}
		>
			{label}
		</button>
	);
};

const renderWithProvider = (ui) =>
	render(
		<MemoryRouter>
			<ConfirmProvider>{ui}</ConfirmProvider>
		</MemoryRouter>,
	);

describe("ConfirmContext", () => {
	it("resolves true when the confirm button is activated", async () => {
		const user = userEvent.setup();
		const onResult = vi.fn();
		renderWithProvider(
			<Trigger onResult={onResult} options={{ confirmLabel: "Delete role" }} />,
		);

		await user.click(screen.getByRole("button", { name: "Delete thing" }));
		await user.click(screen.getByRole("button", { name: "Delete role" }));

		await waitFor(() => expect(onResult).toHaveBeenCalledWith(true));
	});

	it("resolves false when cancelled", async () => {
		const user = userEvent.setup();
		const onResult = vi.fn();
		renderWithProvider(<Trigger onResult={onResult} options={{}} />);

		await user.click(screen.getByRole("button", { name: "Delete thing" }));
		await user.click(screen.getByRole("button", { name: "Cancel" }));

		await waitFor(() => expect(onResult).toHaveBeenCalledWith(false));
	});

	it("resolves false on Escape", async () => {
		const user = userEvent.setup();
		const onResult = vi.fn();
		renderWithProvider(<Trigger onResult={onResult} options={{}} />);

		await user.click(screen.getByRole("button", { name: "Delete thing" }));
		await user.keyboard("{Escape}");

		await waitFor(() => expect(onResult).toHaveBeenCalledWith(false));
	});

	it("resolves false when the close control is used", async () => {
		const user = userEvent.setup();
		const onResult = vi.fn();
		renderWithProvider(<Trigger onResult={onResult} options={{}} />);

		await user.click(screen.getByRole("button", { name: "Delete thing" }));
		await user.click(screen.getByRole("button", { name: "Close" }));

		await waitFor(() => expect(onResult).toHaveBeenCalledWith(false));
	});

	it("dismisses the dialog once settled", async () => {
		const user = userEvent.setup();
		renderWithProvider(<Trigger onResult={vi.fn()} options={{}} />);

		await user.click(screen.getByRole("button", { name: "Delete thing" }));
		expect(screen.getByRole("dialog")).toBeInTheDocument();

		await user.click(screen.getByRole("button", { name: "Cancel" }));
		await waitFor(() =>
			expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
		);
	});

	it("renders the supplied title, message and warning", async () => {
		const user = userEvent.setup();
		renderWithProvider(
			<Trigger
				onResult={vi.fn()}
				options={{
					title: "Delete repository",
					message: 'Are you sure you want to delete "epel"?',
					warning: "This repository is assigned to 3 hosts.",
				}}
			/>,
		);

		await user.click(screen.getByRole("button", { name: "Delete thing" }));

		expect(screen.getByText("Delete repository")).toBeInTheDocument();
		expect(
			screen.getByText('Are you sure you want to delete "epel"?'),
		).toBeInTheDocument();
		expect(
			screen.getByText("This repository is assigned to 3 hosts."),
		).toBeInTheDocument();
	});

	it("moves focus to Cancel so the destructive action is never the default", async () => {
		const user = userEvent.setup();
		renderWithProvider(<Trigger onResult={vi.fn()} options={{}} />);

		await user.click(screen.getByRole("button", { name: "Delete thing" }));

		await waitFor(() =>
			expect(screen.getByRole("button", { name: "Cancel" })).toHaveFocus(),
		);
	});

	it("supersedes an open dialog and cancels the earlier caller", async () => {
		const user = userEvent.setup();
		const first = vi.fn();
		const second = vi.fn();
		renderWithProvider(
			<>
				<Trigger onResult={first} options={{ title: "First" }} label="One" />
				<Trigger onResult={second} options={{ title: "Second" }} label="Two" />
			</>,
		);

		await user.click(screen.getByRole("button", { name: "One" }));
		await user.click(screen.getByRole("button", { name: "Two" }));

		// The superseded caller must not be left awaiting forever.
		await waitFor(() => expect(first).toHaveBeenCalledWith(false));
		expect(screen.getByText("Second")).toBeInTheDocument();

		// The replacement dialog must still be usable and focused.
		await waitFor(() =>
			expect(screen.getByRole("button", { name: "Cancel" })).toHaveFocus(),
		);
	});

	it("throws when used outside a provider", () => {
		const spy = vi.spyOn(console, "error").mockImplementation(() => {});
		// React re-throws through a DOM error event, which jsdom reports to its
		// own console unless the event is cancelled.
		const swallow = (event) => event.preventDefault();
		window.addEventListener("error", swallow);
		const Bare = () => {
			useConfirm();
			return null;
		};
		expect(() => render(<Bare />)).toThrow(
			"useConfirm must be used within a ConfirmProvider",
		);
		window.removeEventListener("error", swallow);
		spy.mockRestore();
	});
});
