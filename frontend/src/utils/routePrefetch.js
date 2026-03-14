/**
 * Prefetch route chunks on hover for lazy-loaded pages.
 * Main nav pages (Dashboard, Hosts, Packages, etc.) are eager-loaded for instant navigation.
 * Add paths here for any remaining lazy-loaded routes that benefit from prefetch.
 */
const routePrefetchers = {};

export const prefetchRoute = (path) => {
	const prefetcher = routePrefetchers[path];
	if (prefetcher) {
		prefetcher().catch(() => {
			// Ignore prefetch errors (e.g. network issues)
		});
	}
};
