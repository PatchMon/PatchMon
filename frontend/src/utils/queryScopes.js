// Adding or removing a host changes more than the host list. The sidebar
// badges, the Hosts page stat cards and the dashboard totals are all derived
// from the same rows, and each lives under its own query key.
//
// Those keys were previously left out of every mutation's invalidation set, and
// because Layout never unmounts and does not refetch on focus, the sidebar count
// stayed wrong until the browser was reloaded. Invalidate the whole scope
// through this helper so the list cannot drift again.
export const HOST_SCOPE_KEYS = [
	["hosts"],
	["hostCounts"],
	["navigationStats"],
	["hostFilterOptions"],
	["dashboardStats"],
];

export const invalidateHostScope = (queryClient) =>
	Promise.all(
		HOST_SCOPE_KEYS.map((queryKey) =>
			queryClient.invalidateQueries({ queryKey }),
		),
	);
