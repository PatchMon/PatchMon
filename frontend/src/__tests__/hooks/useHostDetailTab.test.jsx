import { act, renderHook } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { useHostDetailTab } from "../../hooks/useHostDetailTab";

const setup = (initialEntry) => {
	const wrapper = ({ children }) => (
		<MemoryRouter initialEntries={[initialEntry]}>{children}</MemoryRouter>
	);
	return renderHook(
		() => {
			const [activeTab, handleTabChange] = useHostDetailTab();
			const location = useLocation();
			return { activeTab, handleTabChange, location };
		},
		{ wrapper },
	);
};

describe("useHostDetailTab", () => {
	it("defaults to the host tab", () => {
		const { result } = setup("/hosts/abc");
		expect(result.current.activeTab).toBe("host");
	});

	it("restores the tab from the URL on load", () => {
		const { result } = setup("/hosts/abc?tab=compliance");
		expect(result.current.activeTab).toBe("compliance");
	});

	it("writes every tab change to ?tab=", () => {
		const { result } = setup("/hosts/abc");
		act(() => result.current.handleTabChange("docker"));
		expect(result.current.activeTab).toBe("docker");
		expect(result.current.location.search).toBe("?tab=docker");
	});

	it("ignores an unknown tab in the URL", () => {
		const { result } = setup("/hosts/abc?tab=bogus");
		expect(result.current.activeTab).toBe("host");
	});

	it("redirects legacy history and queue tabs to activity", () => {
		const { result } = setup("/hosts/abc?tab=queue");
		expect(result.current.activeTab).toBe("activity");
		expect(result.current.location.search).toBe("?tab=activity");
	});

	it("drops activity filters when leaving the activity tab", () => {
		const { result } = setup("/hosts/abc?tab=activity&status=failed&page=2");
		expect(result.current.activeTab).toBe("activity");
		act(() => result.current.handleTabChange("network"));
		expect(result.current.location.search).toBe("?tab=network");
	});

	it("opens the tab requested through navigation state", () => {
		const { result } = setup({
			pathname: "/hosts/abc",
			state: { tab: "compliance" },
		});
		expect(result.current.activeTab).toBe("compliance");
		expect(result.current.location.search).toBe("?tab=compliance");
	});

	it("keeps navigation state when switching tabs", () => {
		const { result } = setup({
			pathname: "/hosts/abc",
			state: { apiKey: "k" },
		});
		act(() => result.current.handleTabChange("notes"));
		expect(result.current.activeTab).toBe("notes");
		expect(result.current.location.state).toEqual({ apiKey: "k" });
	});
});
