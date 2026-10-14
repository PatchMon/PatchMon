import { useCallback, useEffect, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";

export const HOSTS_SORT_FIELDS = {
	agent_version: "agent_version",
	config_version: "config_version",
	friendlyName: "friendly_name",
	group: "group",
	hostname: "hostname",
	integrations: "integrations",
	ip: "ip",
	last_update: "last_update",
	needs_reboot: "needs_reboot",
	notes: "notes",
	os: "os_type",
	os_version: "os_version",
	security_updates: "security_updates",
	ssg_version: "ssg_version",
	status: "status",
	updates: "updates",
	uptime: "uptime",
};

const STATUS_FILTERS = ["all", "reporting", "overdue", "stale", "awaiting"];
const CONNECTION_FILTERS = ["all", "connected", "offline"];
const GROUP_BY_OPTIONS = ["none", "group", "status", "os"];
const STATUS_IMPLIED_BY_FILTER = { stale: "stale", upToDate: "reporting" };

export const HOSTS_FILTER_PARAMS = [
	"search",
	"group",
	"status",
	"connection",
	"osFilter",
	"osVersionFilter",
	"groupBy",
	"hideStale",
	"filter",
	"reboot",
	"selected",
];

const DEFAULTS = {
	search: "",
	group: "all",
	connection: "all",
	osFilter: "all",
	osVersionFilter: "all",
	groupBy: "none",
	hideStale: "false",
	sort: "hostname",
	order: "asc",
};

const impliedStatus = (filter) => STATUS_IMPLIED_BY_FILTER[filter] || "all";
const oneOf = (value, allowed, fallback) =>
	allowed.includes(value) ? value : fallback;

export const readHostsFilters = (searchParams) => {
	const status = searchParams.get("status");
	const sort = searchParams.get("sort");
	return {
		search: (searchParams.get("search") || "").trim(),
		groupFilter: searchParams.get("group") || "all",
		statusFilter: STATUS_FILTERS.includes(status)
			? status
			: impliedStatus(searchParams.get("filter") || ""),
		connectionFilter: oneOf(
			searchParams.get("connection"),
			CONNECTION_FILTERS,
			"all",
		),
		osFilter: searchParams.get("osFilter") || "all",
		osVersionFilter: searchParams.get("osVersionFilter") || "all",
		groupBy: oneOf(searchParams.get("groupBy"), GROUP_BY_OPTIONS, "none"),
		hideStale: searchParams.get("hideStale") === "true",
		sortField: Object.hasOwn(HOSTS_SORT_FIELDS, sort) ? sort : "hostname",
		sortDirection: searchParams.get("order") === "desc" ? "desc" : "asc",
	};
};

export const applyHostsFilterChanges = (current, changes, { clear } = {}) => {
	const next = new URLSearchParams(current);
	if (clear) {
		for (const key of HOSTS_FILTER_PARAMS) next.delete(key);
	}
	for (const [key, value] of Object.entries(changes)) {
		const text = value === undefined || value === null ? "" : String(value);
		if (text === "" || text === DEFAULTS[key]) next.delete(key);
		else next.set(key, text);
	}
	const status = next.get("status");
	if (status && status === impliedStatus(next.get("filter") || "")) {
		next.delete("status");
	}
	next.delete("page");
	return next;
};

export const useHostsUrlFilters = () => {
	const [searchParams, setSearchParams] = useSearchParams();
	const latest = useRef(null);
	latest.current = { searchParams, setSearchParams };

	const filters = readHostsFilters(searchParams);
	const urlSearch = filters.search;

	const updateFilters = useCallback((changes, options) => {
		const { searchParams: current, setSearchParams: write } = latest.current;
		write(applyHostsFilterChanges(current, changes, options), {
			replace: true,
		});
	}, []);

	const [searchTerm, setSearchTerm] = useState(urlSearch);
	const writtenSearch = useRef(urlSearch);

	useEffect(() => {
		if (urlSearch === writtenSearch.current) return;
		writtenSearch.current = urlSearch;
		setSearchTerm(urlSearch);
	}, [urlSearch]);

	useEffect(() => {
		const timer = setTimeout(() => {
			const trimmed = searchTerm.trim();
			if (trimmed === writtenSearch.current) return;
			writtenSearch.current = trimmed;
			updateFilters({ search: trimmed });
		}, 400);
		return () => clearTimeout(timer);
	}, [searchTerm, updateFilters]);

	return { ...filters, searchTerm, setSearchTerm, updateFilters };
};
