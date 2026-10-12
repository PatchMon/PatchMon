import { useEffect, useRef } from "react";
import { useLocation, useSearchParams } from "react-router-dom";

const writeTab = ({ activeTab, setSearchParams, state }) => {
	setSearchParams(
		(prev) => {
			const next = new URLSearchParams(prev);
			next.set("tab", activeTab);
			return next;
		},
		{ replace: true, state },
	);
};

export const useTabParam = (activeTab) => {
	const location = useLocation();
	const [searchParams, setSearchParams] = useSearchParams();
	const urlTab = searchParams.get("tab");
	const latest = useRef(null);
	latest.current = {
		activeTab,
		urlTab,
		setSearchParams,
		state: location.state,
	};

	useEffect(() => {
		if (latest.current.urlTab !== activeTab) writeTab(latest.current);
	}, [activeTab]);

	useEffect(() => {
		if (!urlTab) writeTab(latest.current);
	}, [urlTab]);
};

export const searchWithoutTab = (search) => {
	const params = new URLSearchParams(search);
	params.delete("tab");
	return params.toString();
};
