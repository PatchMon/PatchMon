import fs from "node:fs";
import path from "node:path";
import {
	QueryClient,
	QueryClientProvider,
	useQuery,
} from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useGlobalRefresh, usePageRefresh } from "../hooks/usePageRefresh";
import { HOST_SCOPE_KEYS, invalidateHostScope } from "../utils/queryScopes";

/**
 * The Refresh buttons used to be bound to a single query's refetch(), so a page
 * built from a dozen queries refreshed one panel and left the rest showing
 * cached values. Users learned to reload the browser instead. These tests pin
 * the two properties that fix has to keep: a refresh reaches every key the
 * screen renders, and the spinner reflects all of them rather than the first.
 */

const makeClient = () =>
	new QueryClient({
		defaultOptions: { queries: { retry: false, gcTime: Infinity } },
	});

const wrapperFor = (queryClient) => {
	const Wrapper = ({ children }) => (
		<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
	);
	return Wrapper;
};

// A promise the test releases by hand, so a fetch can be held mid-flight.
const deferred = () => {
	let resolve;
	const promise = new Promise((r) => {
		resolve = r;
	});
	return { promise, resolve };
};

// Mount real observers alongside the hook. usePageRefresh does not subscribe to
// anything itself, and invalidateQueries only refetches queries that have an
// active observer, so a harness without useQuery would test nothing.
// `specs` is a fixed-length literal per test, so the hook count is stable.
const harness = (specs, keys) => () => {
	const queries = specs.map((spec) =>
		useQuery({ queryKey: spec.key, queryFn: spec.fn }),
	);
	return { queries, ...usePageRefresh(keys) };
};

describe("usePageRefresh", () => {
	it("invalidates every key it is given, not just the first", async () => {
		const queryClient = makeClient();
		const spy = vi.spyOn(queryClient, "invalidateQueries");

		const { result } = renderHook(
			() => usePageRefresh([["hosts"], ["hostCounts"], ["dashboardStats"]]),
			{ wrapper: wrapperFor(queryClient) },
		);

		await act(async () => {
			await result.current.refresh();
		});

		expect(spy.mock.calls.map(([filters]) => filters.queryKey)).toEqual([
			["hosts"],
			["hostCounts"],
			["dashboardStats"],
		]);
	});

	it("matches paginated keys by prefix, the way the pages declare them", async () => {
		const queryClient = makeClient();
		await queryClient.prefetchQuery({
			queryKey: ["hosts", { page: 2, search: "web" }],
			queryFn: () => "page-2",
		});
		await queryClient.prefetchQuery({
			queryKey: ["packages"],
			queryFn: () => "packages",
		});

		const { result } = renderHook(() => usePageRefresh([["hosts"]]), {
			wrapper: wrapperFor(queryClient),
		});

		await act(async () => {
			await result.current.refresh();
		});

		const state = Object.fromEntries(
			queryClient
				.getQueryCache()
				.findAll()
				.map((q) => [JSON.stringify(q.queryKey), q.state.isInvalidated]),
		);
		expect(state['["hosts",{"page":2,"search":"web"}]']).toBe(true);
		expect(state['["packages"]']).toBe(false);
	});

	it("keeps isRefreshing true until the slowest key settles", async () => {
		const queryClient = makeClient();
		// First call resolves immediately so both queries mount; the refresh
		// then holds the slow one open.
		let slowCall = 0;
		const slowRefetch = deferred();

		const { result } = renderHook(
			harness(
				[
					{ key: ["fast"], fn: () => "fast" },
					{
						key: ["slow"],
						fn: () => (slowCall++ === 0 ? "slow" : slowRefetch.promise),
					},
				],
				[["fast"], ["slow"]],
			),
			{ wrapper: wrapperFor(queryClient) },
		);

		await waitFor(() =>
			expect(queryClient.getQueryData(["slow"])).toBe("slow"),
		);
		expect(result.current.isRefreshing).toBe(false);

		act(() => {
			result.current.refresh();
		});
		await waitFor(() => expect(result.current.isRefreshing).toBe(true));

		// The fast query landing must not stop the spinner.
		await waitFor(() =>
			expect(queryClient.isFetching({ queryKey: ["fast"] })).toBe(0),
		);
		expect(result.current.isRefreshing).toBe(true);

		await act(async () => {
			slowRefetch.resolve("slow-again");
			await slowRefetch.promise;
		});
		await waitFor(() => expect(result.current.isRefreshing).toBe(false));
	});

	it("ignores background polls, so the button does not spin on a timer", async () => {
		// Several screens poll on a refetchInterval. Counting those fetches made
		// the button spin and disable itself with no user action, worst of all
		// in the sidebar, which never unmounts. Only fetches a refresh caused
		// should count.
		const queryClient = makeClient();
		let call = 0;
		const poll = deferred();
		const refreshFetch = deferred();

		const { result } = renderHook(
			harness(
				[
					{
						key: ["wsStatusSummary"],
						fn: () => {
							call += 1;
							if (call === 1) return "initial";
							if (call === 2) return poll.promise;
							return refreshFetch.promise;
						},
					},
				],
				[["wsStatusSummary"]],
			),
			{ wrapper: wrapperFor(queryClient) },
		);

		await waitFor(() =>
			expect(queryClient.getQueryData(["wsStatusSummary"])).toBe("initial"),
		);

		// A poll: a plain refetch, exactly what a refetchInterval tick fires.
		act(() => {
			result.current.queries[0].refetch();
		});
		await waitFor(() =>
			expect(queryClient.isFetching({ queryKey: ["wsStatusSummary"] })).toBe(1),
		);
		expect(result.current.isRefreshing).toBe(false);

		await act(async () => {
			poll.resolve("polled");
			await poll.promise;
		});

		// A refresh, by contrast, does count.
		act(() => {
			result.current.refresh();
		});
		await waitFor(() => expect(result.current.isRefreshing).toBe(true));
		await act(async () => {
			refreshFetch.resolve("refreshed");
			await refreshFetch.promise;
		});
		await waitFor(() => expect(result.current.isRefreshing).toBe(false));
	});

	it("ignores queries outside its key set", async () => {
		const queryClient = makeClient();
		queryClient.setQueryDefaults(["unrelated"], {
			queryFn: () => new Promise(() => {}),
		});

		const { result } = renderHook(() => usePageRefresh([["hosts"]]), {
			wrapper: wrapperFor(queryClient),
		});

		queryClient.prefetchQuery({ queryKey: ["unrelated"] });
		await new Promise((r) => setTimeout(r, 10));

		expect(result.current.isRefreshing).toBe(false);
	});
});

describe("useGlobalRefresh", () => {
	it("invalidates the whole cache, which is what the sidebar control promises", async () => {
		const queryClient = makeClient();
		await queryClient.prefetchQuery({
			queryKey: ["navigationStats"],
			queryFn: () => 1,
		});
		await queryClient.prefetchQuery({
			queryKey: ["dashboardStats"],
			queryFn: () => 2,
		});

		const { result } = renderHook(() => useGlobalRefresh(), {
			wrapper: wrapperFor(queryClient),
		});
		await act(async () => {
			await result.current.refresh();
		});

		const invalidated = queryClient
			.getQueryCache()
			.findAll()
			.every((q) => q.state.isInvalidated);
		expect(invalidated).toBe(true);
	});
});

describe("save-then-rehydrate ordering", () => {
	/**
	 * AlertSettings derives its dirty flag by comparing local form state to the
	 * server copy, so it can only drop local state once the cache holds the
	 * saved values. Clearing it while the cache is still pre-save re-hydrates
	 * the form from stale data and then wedges it dirty against what arrives
	 * next, which silently reverts the save. The fix depends on
	 * invalidateQueries resolving only after the refetch has landed.
	 */
	it("invalidateQueries resolves only after the refetch has settled", async () => {
		const queryClient = makeClient();
		let served = "before-save";

		renderHook(
			harness(
				[{ key: ["alert-config"], fn: () => served }],
				[["alert-config"]],
			),
			{ wrapper: wrapperFor(queryClient) },
		);
		await waitFor(() =>
			expect(queryClient.getQueryData(["alert-config"])).toBe("before-save"),
		);

		served = "after-save";
		await act(async () => {
			await queryClient.invalidateQueries({ queryKey: ["alert-config"] });
		});

		// If this were still "before-save", AlertSettings clearing its local
		// state at this point would re-hydrate the form from the pre-save
		// values and then wedge it dirty, silently reverting the save.
		expect(queryClient.getQueryData(["alert-config"])).toBe("after-save");
	});
});

describe("host scope invalidation", () => {
	it("covers the sidebar counters, which no mutation used to touch", async () => {
		const queryClient = makeClient();
		const spy = vi.spyOn(queryClient, "invalidateQueries");

		await invalidateHostScope(queryClient);

		const keys = spy.mock.calls.map(([filters]) =>
			JSON.stringify(filters.queryKey),
		);
		// These three are the ones that were previously missing: adding or
		// deleting a host left the sidebar badge and the dashboard cards stale
		// until the browser was reloaded.
		expect(keys).toContain('["hostCounts"]');
		expect(keys).toContain('["navigationStats"]');
		expect(keys).toContain('["hostFilterOptions"]');
	});
});

describe("source tree", () => {
	const SRC = path.resolve(__dirname, "..");

	const read = (relative) => fs.readFileSync(path.join(SRC, relative), "utf8");

	it("does not disable focus refetching globally", () => {
		// Focus refetch is the mechanism that makes returning to a tab show
		// current data. Turning it off in the client defaults silently reverts
		// the fix for every query in the app at once.
		const main = read("main.jsx");
		expect(main).not.toMatch(/refetchOnWindowFocus:\s*false/);
	});

	it("keeps the default staleTime low enough for focus refetch to fire", () => {
		// Both focus and mount refetching only act on stale queries, so a long
		// default staleTime makes them no-ops. Five minutes is what the bug
		// report was actually about.
		const main = read("main.jsx");
		const match = main.match(/staleTime:\s*([0-9*\s]+),/);
		expect(match).not.toBeNull();
		// biome-ignore lint/security/noGlobalEval: arithmetic literal from our own source
		const staleTime = eval(match[1]);
		expect(staleTime).toBeLessThanOrEqual(60 * 1000);
	});

	it("every host-scope key is a real array key", () => {
		for (const key of HOST_SCOPE_KEYS) {
			expect(Array.isArray(key)).toBe(true);
			expect(key.length).toBeGreaterThan(0);
		}
	});

	it("the Docker refresh no longer falls back to a full browser reload", () => {
		// Its per-tab dispatch ended in window.location.reload(), and refreshed
		// only the active tab's list while leaving the stat cards alone. One
		// ["docker"] prefix covers every tab and the cards.
		// (Profile.jsx reloads on Discord unlink, which is a genuine full-page
		// reset of auth state, not a refresh fallback.)
		const docker = read("pages/Docker.jsx");
		expect(docker).not.toMatch(/window\.location\.reload/);
		expect(docker).toMatch(/usePageRefresh/);
	});
});
