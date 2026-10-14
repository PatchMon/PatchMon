import { act, renderHook } from "@testing-library/react";
import { useEffect, useState } from "react";
import {
	MemoryRouter,
	useLocation,
	useNavigate,
	useSearchParams,
} from "react-router-dom";
import { describe, expect, it } from "vitest";
import { searchWithoutTab, useTabParam } from "../../hooks/useTabParam";

const TABS = ["overview", "runs", "policies"];

const usePage = () => {
	const [searchParams] = useSearchParams();
	const urlTab = searchParams.get("tab");
	const [activeTab, setActiveTab] = useState(
		TABS.includes(urlTab) ? urlTab : "overview",
	);
	useTabParam(activeTab);
	useEffect(() => {
		if (urlTab && TABS.includes(urlTab)) setActiveTab(urlTab);
	}, [urlTab]);
	return {
		activeTab,
		setActiveTab,
		location: useLocation(),
		navigate: useNavigate(),
	};
};

const setup = (entry) =>
	renderHook(usePage, {
		wrapper: ({ children }) => (
			<MemoryRouter initialEntries={[entry]}>{children}</MemoryRouter>
		),
	});

describe("useTabParam", () => {
	it("writes the default tab when the URL has none", () => {
		const { result } = setup("/patching?status=failed");
		expect(result.current.location.search).toBe("?status=failed&tab=overview");
	});

	it("writes in-page tab changes to the URL", () => {
		const { result } = setup("/patching?tab=overview");
		act(() => result.current.setActiveTab("runs"));
		expect(result.current.location.search).toBe("?tab=runs");
	});

	it("follows a tab chosen by navigation", () => {
		const { result } = setup("/patching?tab=overview");
		act(() => result.current.navigate("/patching?tab=policies"));
		expect(result.current.activeTab).toBe("policies");
		expect(result.current.location.search).toBe("?tab=policies");
	});

	it("restores the current tab when navigation drops the param", () => {
		const { result } = setup("/patching?tab=runs");
		act(() => result.current.navigate("/patching"));
		expect(result.current.activeTab).toBe("runs");
		expect(result.current.location.search).toBe("?tab=runs");
	});

	it("keeps navigation state", () => {
		const { result } = setup({ pathname: "/patching", state: { x: 1 } });
		act(() => result.current.setActiveTab("runs"));
		expect(result.current.location.state).toEqual({ x: 1 });
	});
});

const useStateTabPage = (counter) => {
	counter.renders += 1;
	if (counter.renders > 200) throw new Error("render loop");
	const location = useLocation();
	const [searchParams, setSearchParams] = useSearchParams();
	const urlTab = searchParams.get("tab");
	const [activeTab, setActiveTab] = useState(
		TABS.includes(urlTab) ? urlTab : "overview",
	);
	useTabParam(activeTab);
	useEffect(() => {
		if (urlTab && TABS.includes(urlTab)) setActiveTab(urlTab);
	}, [urlTab]);
	const requestedTab = location.state?.requestedTab;
	useEffect(() => {
		if (requestedTab && TABS.includes(requestedTab)) {
			setActiveTab(requestedTab);
			setSearchParams(
				(prev) => {
					const next = new URLSearchParams(prev);
					next.set("tab", requestedTab);
					return next;
				},
				{ replace: true },
			);
		}
	}, [requestedTab, setSearchParams]);
	return { activeTab, location, navigate: useNavigate() };
};

describe("useTabParam with a tab requested through navigation state", () => {
	it("settles on the requested tab without looping", () => {
		const counter = { renders: 0 };
		const { result } = renderHook(() => useStateTabPage(counter), {
			wrapper: ({ children }) => (
				<MemoryRouter initialEntries={["/compliance?tab=overview"]}>
					{children}
				</MemoryRouter>
			),
		});
		counter.renders = 0;
		act(() =>
			result.current.navigate("/compliance", {
				state: { requestedTab: "runs" },
			}),
		);
		expect(result.current.activeTab).toBe("runs");
		expect(result.current.location.search).toBe("?tab=runs");
		expect(counter.renders).toBeLessThan(20);
	});
});

describe("searchWithoutTab", () => {
	it("ignores the tab param only", () => {
		expect(searchWithoutTab("?tab=runs&status=failed")).toBe("status=failed");
		expect(searchWithoutTab("?tab=runs")).toBe("");
	});
});
