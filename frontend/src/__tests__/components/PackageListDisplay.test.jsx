import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import {
	CompactPackageSummary,
	PackageListDisplay,
	PackageNameList,
} from "../../components/PackageListDisplay";

const KB = "2026-10 Security Update (KB5127648) (26300.9711)";
const FIREFOX = "Mozilla Firefox (x64 en-GB)";
const FIREFOX_ID = "3f2b6c1e-0000-4000-8000-000000000001";
const LONG_PACKAGE =
	"Security Intelligence Update for Microsoft Defender Antivirus - KB2267602 (Version 1.463.33.0) - Current Channel (Broad)";

const renderIn = (ui) => render(<MemoryRouter>{ui}</MemoryRouter>);

describe("PackageNameList", () => {
	it("links inventory packages by ID and leaves the rest as plain text", () => {
		renderIn(
			<PackageNameList
				packages={[KB, FIREFOX]}
				packageIds={{ [FIREFOX]: FIREFOX_ID }}
			/>,
		);
		expect(screen.getByRole("link", { name: FIREFOX })).toHaveAttribute(
			"href",
			`/packages/${FIREFOX_ID}`,
		);
		expect(screen.queryByRole("link", { name: KB })).toBeNull();
		expect(screen.getByText(KB)).toHaveAttribute(
			"title",
			`${KB} (not in the package inventory)`,
		);
	});

	it("links by encoded name when no IDs were resolved", () => {
		renderIn(<PackageNameList packages={["libstdc++6", "openssl"]} />);
		expect(screen.getByRole("link", { name: "libstdc++6" })).toHaveAttribute(
			"href",
			"/packages/libstdc%2B%2B6",
		);
	});
});

describe("PackageListDisplay", () => {
	it("uses the run's package IDs for requested packages and dependencies", () => {
		renderIn(
			<PackageListDisplay
				run={{
					patch_type: "patch_package",
					package_names: ["libstdc++6"],
					packages_affected: ["libstdc++6", "libssl3t64"],
					package_ids: { "libstdc++6": "pkg-1" },
				}}
			/>,
		);
		expect(screen.getByRole("link", { name: "libstdc++6" })).toHaveAttribute(
			"href",
			"/packages/pkg-1",
		);
		expect(screen.queryByRole("link", { name: "libssl3t64" })).toBeNull();
		expect(screen.getByText("libssl3t64")).toBeInTheDocument();
	});
});

describe("CompactPackageSummary", () => {
	it("renders a package missing from the inventory as text", () => {
		renderIn(
			<CompactPackageSummary
				run={{
					patch_type: "patch_package",
					package_names: [KB, FIREFOX],
					package_ids: { [FIREFOX]: FIREFOX_ID },
				}}
			/>,
		);
		expect(screen.queryByRole("link")).toBeNull();
		expect(screen.getByText(KB)).toBeInTheDocument();
	});

	it("truncates long package names while preserving their full hover title", () => {
		renderIn(
			<CompactPackageSummary
				run={{
					patch_type: "patch_package",
					package_names: [LONG_PACKAGE],
				}}
			/>,
		);

		const summary = screen.getByTitle(LONG_PACKAGE);
		const packageName = screen.getByRole("link", { name: LONG_PACKAGE });
		expect(summary).toHaveClass("flex", "min-w-0", "max-w-full");
		expect(packageName).toHaveClass("min-w-0", "flex-1", "truncate");
	});

	it("shows the full name on hover when the package is not in the inventory", () => {
		renderIn(
			<CompactPackageSummary
				run={{
					patch_type: "patch_package",
					package_names: [LONG_PACKAGE],
					package_ids: { [FIREFOX]: FIREFOX_ID },
				}}
			/>,
		);

		const packageName = screen.getByText(LONG_PACKAGE);
		expect(packageName).toHaveClass("truncate");
		expect(packageName).toHaveAttribute(
			"title",
			`${LONG_PACKAGE} (not in the package inventory)`,
		);
	});
});
