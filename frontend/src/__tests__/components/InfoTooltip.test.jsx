import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import InfoTooltip from "../../components/ui/InfoTooltip";

const trigger = () => screen.getByRole("button", { name: "About Patch Run" });

afterEach(() => {
	vi.restoreAllMocks();
});

describe("InfoTooltip", () => {
	it("renders nothing without content", () => {
		const { container } = render(<InfoTooltip content="" label="x" />);
		expect(container).toBeEmptyDOMElement();
	});

	it("is a labelled button that keeps the text out of the page until opened", () => {
		render(<InfoTooltip content="Marks stuck runs" label="About Patch Run" />);
		expect(trigger()).toHaveAttribute("type", "button");
		expect(screen.queryByRole("tooltip")).toBeNull();
	});

	it("shows on hover and hides on leave", () => {
		render(<InfoTooltip content="Marks stuck runs" label="About Patch Run" />);
		fireEvent.mouseEnter(trigger());
		expect(screen.getByRole("tooltip")).toHaveTextContent("Marks stuck runs");
		fireEvent.mouseLeave(trigger());
		expect(screen.queryByRole("tooltip")).toBeNull();
	});

	it("shows on keyboard focus, links it with aria-describedby, and closes on Escape", () => {
		render(<InfoTooltip content="Marks stuck runs" label="About Patch Run" />);
		act(() => trigger().focus());
		const tip = screen.getByRole("tooltip");
		expect(trigger()).toHaveAttribute("aria-describedby", tip.id);
		fireEvent.keyDown(document, { key: "Escape" });
		expect(screen.queryByRole("tooltip")).toBeNull();
	});

	it("stays open after a tap, where the browser fires enter, focus and click together", () => {
		render(<InfoTooltip content="Marks stuck runs" label="About Patch Run" />);
		const el = trigger();
		fireEvent.pointerDown(el, { pointerType: "touch" });
		fireEvent.mouseEnter(el);
		fireEvent.focus(el);
		fireEvent.click(el);
		expect(screen.getByRole("tooltip")).toBeInTheDocument();
		fireEvent.pointerDown(document.body, { pointerType: "touch" });
		expect(screen.queryByRole("tooltip")).toBeNull();
	});

	it("toggles closed on a second click", () => {
		const now = vi.spyOn(Date, "now").mockReturnValue(1_000);
		render(<InfoTooltip content="Marks stuck runs" label="About Patch Run" />);
		fireEvent.click(trigger());
		expect(screen.getByRole("tooltip")).toBeInTheDocument();
		now.mockReturnValue(2_000);
		fireEvent.click(trigger());
		expect(screen.queryByRole("tooltip")).toBeNull();
	});
});
