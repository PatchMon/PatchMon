export const isNavChildActive = (location, parent, child) => {
	const [childPath, childQuery = ""] = child.href.split("?");
	if (location.pathname !== childPath) return false;
	const childTab = new URLSearchParams(childQuery).get("tab");
	const currentTab = new URLSearchParams(location.search).get("tab");
	if (!childTab)
		return location.search === (childQuery ? `?${childQuery}` : "");
	if (currentTab) return currentTab === childTab;
	return location.pathname === parent.href && parent.children?.[0] === child;
};
