/**
 * Unit tests for ComplianceScore Component
 */

import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import ComplianceScore from "../../components/compliance/ComplianceScore";

describe("ComplianceScore Component", () => {
	describe("Score Display", () => {
		it("should display score as percentage", () => {
			render(<ComplianceScore score={85} />);
			expect(screen.getByText("85%")).toBeInTheDocument();
		});

		it("should round score to whole number", () => {
			render(<ComplianceScore score={85.7} />);
			expect(screen.getByText("86%")).toBeInTheDocument();
		});

		it("should display N/A when score is null", () => {
			render(<ComplianceScore score={null} />);
			expect(screen.getByText("N/A")).toBeInTheDocument();
		});

		it("should display N/A when score is undefined", () => {
			render(<ComplianceScore score={undefined} />);
			expect(screen.getByText("N/A")).toBeInTheDocument();
		});

		it("should display 0% for zero score", () => {
			render(<ComplianceScore score={0} />);
			expect(screen.getByText("0%")).toBeInTheDocument();
		});

		it("should display 100% for perfect score", () => {
			render(<ComplianceScore score={100} />);
			expect(screen.getByText("100%")).toBeInTheDocument();
		});
	});

	describe("Color Coding - High Score (Compliant)", () => {
		it("should apply green color for score >= 80", () => {
			const { container } = render(<ComplianceScore score={80} />);
			const scoreElement = screen.getByText("80%");
			expect(scoreElement.className).toContain("text-green-400");
		});

		it("should apply green color for score of 90", () => {
			const { container } = render(<ComplianceScore score={90} />);
			const scoreElement = screen.getByText("90%");
			expect(scoreElement.className).toContain("text-green-400");
		});

		it("should apply green color for score of 100", () => {
			render(<ComplianceScore score={100} />);
			const scoreElement = screen.getByText("100%");
			expect(scoreElement.className).toContain("text-green-400");
		});

		it("should apply green background for high scores", () => {
			const { container } = render(<ComplianceScore score={85} />);
			const wrapper = container.firstChild;
			expect(wrapper.className).toContain("bg-green-900/30");
		});
	});

	describe("Color Coding - Medium Score (Warning)", () => {
		it("should apply yellow color for score >= 60 and < 80", () => {
			render(<ComplianceScore score={70} />);
			const scoreElement = screen.getByText("70%");
			expect(scoreElement.className).toContain("text-yellow-400");
		});

		it("should apply yellow color for score of 60", () => {
			render(<ComplianceScore score={60} />);
			const scoreElement = screen.getByText("60%");
			expect(scoreElement.className).toContain("text-yellow-400");
		});

		it("should apply yellow color for score of 79", () => {
			render(<ComplianceScore score={79} />);
			const scoreElement = screen.getByText("79%");
			expect(scoreElement.className).toContain("text-yellow-400");
		});

		it("should apply yellow background for warning scores", () => {
			const { container } = render(<ComplianceScore score={70} />);
			const wrapper = container.firstChild;
			expect(wrapper.className).toContain("bg-yellow-900/30");
		});
	});

	describe("Color Coding - Low Score (Critical)", () => {
		it("should apply red color for score < 60", () => {
			render(<ComplianceScore score={45} />);
			const scoreElement = screen.getByText("45%");
			expect(scoreElement.className).toContain("text-red-400");
		});

		it("should apply red color for score of 59", () => {
			render(<ComplianceScore score={59} />);
			const scoreElement = screen.getByText("59%");
			expect(scoreElement.className).toContain("text-red-400");
		});

		it("should apply red color for score of 0", () => {
			render(<ComplianceScore score={0} />);
			const scoreElement = screen.getByText("0%");
			expect(scoreElement.className).toContain("text-red-400");
		});

		it("should apply red background for critical scores", () => {
			const { container } = render(<ComplianceScore score={45} />);
			const wrapper = container.firstChild;
			expect(wrapper.className).toContain("bg-red-900/30");
		});
	});

	describe("Color Coding - No Score", () => {
		it("should apply secondary color for null score", () => {
			render(<ComplianceScore score={null} />);
			const scoreElement = screen.getByText("N/A");
			expect(scoreElement.className).toContain("text-secondary-400");
		});

		it("should apply secondary background for null score", () => {
			const { container } = render(<ComplianceScore score={null} />);
			const wrapper = container.firstChild;
			expect(wrapper.className).toContain("bg-secondary-700");
		});
	});

	describe("Size Variants", () => {
		it("should apply small size classes when size is sm", () => {
			const { container } = render(<ComplianceScore score={85} size="sm" />);
			const wrapper = container.firstChild;
			expect(wrapper.className).toContain("text-sm");
			expect(wrapper.className).toContain("px-2");
			expect(wrapper.className).toContain("py-1");
		});

		it("should apply medium size classes by default", () => {
			const { container } = render(<ComplianceScore score={85} />);
			const wrapper = container.firstChild;
			expect(wrapper.className).toContain("text-base");
			expect(wrapper.className).toContain("px-3");
			expect(wrapper.className).toContain("py-1.5");
		});

		it("should apply medium size classes when size is md", () => {
			const { container } = render(<ComplianceScore score={85} size="md" />);
			const wrapper = container.firstChild;
			expect(wrapper.className).toContain("text-base");
			expect(wrapper.className).toContain("px-3");
		});

		it("should apply large size classes when size is lg", () => {
			const { container } = render(<ComplianceScore score={85} size="lg" />);
			const wrapper = container.firstChild;
			expect(wrapper.className).toContain("text-lg");
			expect(wrapper.className).toContain("px-4");
			expect(wrapper.className).toContain("py-2");
		});
	});

	describe("Component Structure", () => {
		it("should render as an inline-flex container", () => {
			const { container } = render(<ComplianceScore score={85} />);
			const wrapper = container.firstChild;
			expect(wrapper.className).toContain("inline-flex");
		});

		it("should have rounded-full class for pill shape", () => {
			const { container } = render(<ComplianceScore score={85} />);
			const wrapper = container.firstChild;
			expect(wrapper.className).toContain("rounded-full");
		});

		it("should render an icon alongside the score", () => {
			const { container } = render(<ComplianceScore score={85} />);
			const icon = container.querySelector("svg");
			expect(icon).toBeInTheDocument();
		});

		it("should have items-center class for vertical alignment", () => {
			const { container } = render(<ComplianceScore score={85} />);
			const wrapper = container.firstChild;
			expect(wrapper.className).toContain("items-center");
		});
	});

	describe("Icon Selection", () => {
		it("should render shield check icon for high score", () => {
			const { container } = render(<ComplianceScore score={85} />);
			const icon = container.querySelector("svg");
			expect(icon).toBeInTheDocument();
			// Lucide icons use class attribute, check via classList
			expect(icon.classList.toString()).toContain("text-green-400");
		});

		it("should render shield alert icon for warning score", () => {
			const { container } = render(<ComplianceScore score={70} />);
			const icon = container.querySelector("svg");
			expect(icon).toBeInTheDocument();
			expect(icon.classList.toString()).toContain("text-yellow-400");
		});

		it("should render shield x icon for critical score", () => {
			const { container } = render(<ComplianceScore score={45} />);
			const icon = container.querySelector("svg");
			expect(icon).toBeInTheDocument();
			expect(icon.classList.toString()).toContain("text-red-400");
		});

		it("should render shield question icon for null score", () => {
			const { container } = render(<ComplianceScore score={null} />);
			const icon = container.querySelector("svg");
			expect(icon).toBeInTheDocument();
			expect(icon.classList.toString()).toContain("text-secondary-400");
		});
	});
});
