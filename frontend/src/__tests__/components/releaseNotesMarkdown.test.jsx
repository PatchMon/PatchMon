import { render, screen } from "@testing-library/react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { describe, expect, it } from "vitest";

// Release note bodies are written as GitHub Flavoured Markdown, but
// react-markdown only implements CommonMark. Without remark-gfm a table renders
// as literal pipe characters in the What's New modal, which is exactly what
// happened up to v2.1.1. These tests pin the plugin in place.
const TABLE_MARKDOWN = [
	"| Issue | PR | Change |",
	"|---|---|---|",
	"| #1049 | #1051 | Windows installer no longer dies on antivirus |",
].join("\n");

describe("release notes markdown rendering", () => {
	it("renders a GFM table as a real table when remark-gfm is enabled", () => {
		const { container } = render(
			<ReactMarkdown remarkPlugins={[remarkGfm]}>
				{TABLE_MARKDOWN}
			</ReactMarkdown>,
		);

		expect(container.querySelector("table")).not.toBeNull();
		expect(container.querySelectorAll("th")).toHaveLength(3);
		expect(container.querySelectorAll("tbody tr")).toHaveLength(1);
		expect(
			screen.getByText("Windows installer no longer dies on antivirus"),
		).toBeInTheDocument();
	});

	it("renders the same table as literal text without the plugin", () => {
		// Guards the assertion above: proves the test would actually fail if the
		// plugin were dropped, rather than passing for some unrelated reason.
		const { container } = render(
			<ReactMarkdown>{TABLE_MARKDOWN}</ReactMarkdown>,
		);

		expect(container.querySelector("table")).toBeNull();
		expect(container.textContent).toContain("| Issue | PR | Change |");
	});

	it("renders autolinked issue URLs, also a GFM feature", () => {
		const { container } = render(
			<ReactMarkdown remarkPlugins={[remarkGfm]}>
				{"See https://github.com/PatchMon/PatchMon/issues/1049 for detail."}
			</ReactMarkdown>,
		);

		const link = container.querySelector("a");
		expect(link).not.toBeNull();
		expect(link.getAttribute("href")).toBe(
			"https://github.com/PatchMon/PatchMon/issues/1049",
		);
	});
});
