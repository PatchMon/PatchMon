import { useCallback, useEffect } from "react";
import { useLocation, useSearchParams } from "react-router-dom";

export const HOST_DETAIL_TABS = [
	"host",
	"network",
	"system",
	"activity",
	"notes",
	"integrations",
	"reporting",
	"docker",
	"patching",
	"compliance",
	"terminal",
	"rdp",
];

const LEGACY_TABS = { history: "activity", queue: "activity" };

const ACTIVITY_FILTER_PARAMS = [
	"direction",
	"type",
	"status",
	"since",
	"q",
	"page",
];

const normaliseTab = (tab) => {
	if (!tab) return null;
	if (LEGACY_TABS[tab]) return LEGACY_TABS[tab];
	return HOST_DETAIL_TABS.includes(tab) ? tab : null;
};

export const useHostDetailTab = () => {
	const location = useLocation();
	const [searchParams, setSearchParams] = useSearchParams();
	const rawUrlTab = searchParams.get("tab");
	const urlTab = normaliseTab(rawUrlTab);
	const stateTab = normaliseTab(location.state?.tab);
	const activeTab = urlTab || stateTab || "host";

	const handleTabChange = useCallback(
		(tabName) => {
			const tab = normaliseTab(tabName);
			if (!tab) return;
			setSearchParams(
				(prev) => {
					const next = new URLSearchParams(prev);
					next.set("tab", tab);
					if (tab !== "activity") {
						for (const key of ACTIVITY_FILTER_PARAMS) next.delete(key);
					}
					return next;
				},
				{ replace: true, state: location.state },
			);
		},
		[setSearchParams, location.state],
	);

	useEffect(() => {
		const wanted = urlTab ? (rawUrlTab !== urlTab ? urlTab : null) : stateTab;
		if (!wanted) return;
		setSearchParams(
			(prev) => {
				const next = new URLSearchParams(prev);
				next.set("tab", wanted);
				return next;
			},
			{ replace: true, state: location.state },
		);
	}, [rawUrlTab, urlTab, stateTab, setSearchParams, location.state]);

	return [activeTab, handleTabChange];
};
