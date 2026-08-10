import { useIsFetching, useQueryClient } from "@tanstack/react-query";
import { useCallback, useMemo } from "react";

// Prefix comparison for the spinner's filter, so ["hosts"] covers the paginated
// ["hosts", params] entry. This is not TanStack's own hashing: it compares
// JSON.stringify output, which is insertion-ordered, whereas TanStack sorts
// object keys. Every prefix declared in the app is strings-only, where the two
// agree. Keep it that way; an object inside a prefix would make the spinner and
// the invalidation disagree.
const matchesPrefix = (queryKey, prefix) => {
	if (!Array.isArray(queryKey) || queryKey.length < prefix.length) return false;
	return prefix.every(
		(part, i) => JSON.stringify(queryKey[i]) === JSON.stringify(part),
	);
};

// Only queries a refresh actually invalidated count towards the spinner.
// Several screens poll on a timer, and counting those made the button spin and
// disable itself every 10s on its own — worst of all in the sidebar, which
// never unmounts. Invalidation sets this flag and the refetch clears it, so it
// marks exactly the window a refresh is responsible for.
const isRefreshFetch = (query) => query.state.isInvalidated;

// A Refresh button has to mean "refresh this screen". Binding it to a single
// query's refetch() left every other panel on the page showing whatever it had
// cached, which is why a browser reload was the only thing that reliably worked.
//
// Pass every key the screen renders from. `isRefreshing` counts all of them, so
// the spinner keeps turning until the slowest panel lands rather than stopping
// when the first one does.
export function usePageRefresh(keys) {
	const queryClient = useQueryClient();

	// Callers pass literal arrays, so identity changes every render. Serialising
	// keeps the callback and the fetch filter stable.
	const signature = JSON.stringify(keys);
	// biome-ignore lint/correctness/useExhaustiveDependencies: signature is the stable form of keys
	const stableKeys = useMemo(() => keys, [signature]);

	const isRefreshing =
		useIsFetching({
			predicate: (query) =>
				isRefreshFetch(query) &&
				stableKeys.some((key) => matchesPrefix(query.queryKey, key)),
		}) > 0;

	const refresh = useCallback(
		() =>
			Promise.all(
				stableKeys.map((key) =>
					queryClient.invalidateQueries({ queryKey: key }),
				),
			),
		[queryClient, stableKeys],
	);

	return { refresh, isRefreshing };
}

// The sidebar control sits outside any page, so it cannot know which keys the
// current route uses. Invalidating everything active is both what its position
// promises and cheap in practice: only mounted queries refetch, the rest are
// merely marked stale for their next mount.
export function useGlobalRefresh() {
	const queryClient = useQueryClient();
	const isRefreshing = useIsFetching({ predicate: isRefreshFetch }) > 0;
	const refresh = useCallback(
		() => queryClient.invalidateQueries(),
		[queryClient],
	);
	return { refresh, isRefreshing };
}

export { matchesPrefix };
