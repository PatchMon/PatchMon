import {
	Box,
	Container,
	GitBranch,
	Layers,
	Package,
	Search,
	Server,
	User,
	X,
} from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { searchAPI } from "../utils/api";

const typeConfig = {
	host: {
		icon: Server,
		color: "text-blue-500",
		label: "Hosts",
		route: (id) => `/hosts/${id}`,
	},
	package: {
		icon: Package,
		color: "text-green-500",
		label: "Packages",
		route: (id) => `/packages/${id}`,
	},
	repository: {
		icon: GitBranch,
		color: "text-purple-500",
		label: "Repositories",
		route: (id) => `/repositories/${id}`,
	},
	host_group: {
		icon: Layers,
		color: "text-teal-500",
		label: "Host Groups",
		route: (id) => `/host-groups/${id}`,
	},
	user: {
		icon: User,
		color: "text-orange-500",
		label: "Users",
		route: () => "/settings/users",
	},
	docker_container: {
		icon: Container,
		color: "text-cyan-500",
		label: "Docker Containers",
		route: (id) => `/docker/containers/${id}`,
	},
	docker_image: {
		icon: Box,
		color: "text-indigo-500",
		label: "Docker Images",
		route: (id) => `/docker/images/${id}`,
	},
};

const GlobalSearch = () => {
	const [query, setQuery] = useState("");
	const [results, setResults] = useState(null);
	const [isOpen, setIsOpen] = useState(false);
	const [isLoading, setIsLoading] = useState(false);
	const [selectedIndex, setSelectedIndex] = useState(-1);
	const searchRef = useRef(null);
	const inputRef = useRef(null);
	const navigate = useNavigate();

	const debounceTimerRef = useRef(null);
	const abortControllerRef = useRef(null);
	const searchCounterRef = useRef(0);

	const performSearch = useCallback(async (searchQuery) => {
		if (!searchQuery || searchQuery.trim().length === 0) {
			setResults(null);
			setIsOpen(false);
			return;
		}

		// Cancel any in-flight request
		if (abortControllerRef.current) {
			abortControllerRef.current.abort();
		}
		const controller = new AbortController();
		abortControllerRef.current = controller;
		const requestId = ++searchCounterRef.current;

		setIsLoading(true);
		try {
			const response = await searchAPI.global(searchQuery, {
				signal: controller.signal,
			});
			// Ignore stale responses
			if (requestId !== searchCounterRef.current) return;
			// Backend returns flat array [{id, name, type, description}]
			// Group by type for display
			const grouped = {};
			for (const item of response.data || []) {
				if (!grouped[item.type]) grouped[item.type] = [];
				grouped[item.type].push(item);
			}
			setResults(grouped);
			setIsOpen(true);
			setSelectedIndex(-1);
		} catch (error) {
			if (error.name === "AbortError" || error.name === "CanceledError") return;
			console.error("Search error:", error);
			if (requestId === searchCounterRef.current) {
				setResults(null);
			}
		} finally {
			if (requestId === searchCounterRef.current) {
				setIsLoading(false);
			}
		}
	}, []);

	const handleInputChange = (e) => {
		const value = e.target.value;
		setQuery(value);

		if (debounceTimerRef.current) {
			clearTimeout(debounceTimerRef.current);
		}

		debounceTimerRef.current = setTimeout(() => {
			performSearch(value);
		}, 300);
	};

	const handleClear = () => {
		if (debounceTimerRef.current) {
			clearTimeout(debounceTimerRef.current);
		}
		setQuery("");
		setResults(null);
		setIsOpen(false);
		setSelectedIndex(-1);
		inputRef.current?.focus();
	};

	const handleResultClick = (result) => {
		const config = typeConfig[result.type];
		if (config) {
			navigate(config.route(result.id));
		}
		handleClear();
	};

	useEffect(() => {
		const handleClickOutside = (event) => {
			if (searchRef.current && !searchRef.current.contains(event.target)) {
				setIsOpen(false);
			}
		};

		document.addEventListener("mousedown", handleClickOutside);
		return () => {
			document.removeEventListener("mousedown", handleClickOutside);
		};
	}, []);

	// Build flat list for keyboard navigation
	const allResults = [];
	const typeOrder = [
		"host",
		"package",
		"repository",
		"host_group",
		"user",
		"docker_container",
		"docker_image",
	];
	if (results) {
		for (const type of typeOrder) {
			if (results[type]?.length > 0) {
				allResults.push(...results[type]);
			}
		}
	}

	const handleKeyDown = (e) => {
		if (!isOpen || !allResults.length) return;

		switch (e.key) {
			case "ArrowDown":
				e.preventDefault();
				setSelectedIndex((prev) =>
					prev < allResults.length - 1 ? prev + 1 : prev,
				);
				break;
			case "ArrowUp":
				e.preventDefault();
				setSelectedIndex((prev) => (prev > 0 ? prev - 1 : -1));
				break;
			case "Enter":
				e.preventDefault();
				if (selectedIndex >= 0 && allResults[selectedIndex]) {
					handleResultClick(allResults[selectedIndex]);
				}
				break;
			case "Escape":
				e.preventDefault();
				setIsOpen(false);
				setSelectedIndex(-1);
				break;
			default:
				break;
		}
	};

	const hasResults = results && Object.keys(results).length > 0;

	return (
		<div ref={searchRef} className="relative w-full max-w-sm">
			<div className="relative">
				<div className="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3">
					<Search className="h-5 w-5 text-secondary-400" />
				</div>
				<input
					ref={inputRef}
					type="text"
					className="block w-full rounded-lg border border-secondary-200 bg-white py-2.5 sm:py-2 pl-10 pr-10 text-sm text-secondary-900 placeholder-secondary-500 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500 dark:border-secondary-600 dark:bg-secondary-700 dark:text-white dark:placeholder-secondary-400 min-h-[44px]"
					placeholder="Search hosts, packages, repos..."
					value={query}
					onChange={handleInputChange}
					onKeyDown={handleKeyDown}
					onFocus={() => {
						if (query && results) setIsOpen(true);
					}}
				/>
				{query && (
					<button
						type="button"
						onClick={handleClear}
						className="absolute inset-y-0 right-0 flex items-center pr-3 text-secondary-400 hover:text-secondary-600 min-w-[44px] min-h-[44px] justify-center"
						aria-label="Clear search"
					>
						<X className="h-4 w-4" />
					</button>
				)}
			</div>

			{isOpen && (
				<div className="absolute z-50 mt-2 w-full sm:w-[calc(100vw-2rem)] sm:max-w-md rounded-lg border border-secondary-200 bg-white shadow-lg dark:border-secondary-600 dark:bg-secondary-800 left-0 sm:left-auto right-0 sm:right-auto">
					{isLoading ? (
						<div className="px-4 py-2 text-center text-sm text-secondary-500 dark:text-white/70">
							Searching...
						</div>
					) : hasResults ? (
						<div className="max-h-96 overflow-y-auto">
							{typeOrder.map((type) => {
								const items = results[type];
								if (!items?.length) return null;
								const config = typeConfig[type];
								if (!config) return null;
								const Icon = config.icon;

								return (
									<div key={type}>
										<div className="sticky top-0 z-10 bg-secondary-50 px-3 py-1.5 text-xs font-semibold uppercase tracking-wider text-secondary-500 dark:bg-secondary-700 dark:text-white/80">
											{config.label}
										</div>
										{items.map((item) => {
											const globalIdx = allResults.findIndex(
												(r) => r.id === item.id && r.type === item.type,
											);
											return (
												<button
													type="button"
													key={`${item.type}-${item.id}`}
													onClick={() => handleResultClick(item)}
													className={`flex w-full items-center gap-2 px-3 py-3 sm:py-1.5 text-left transition-colors min-h-[44px] ${
														globalIdx === selectedIndex
															? "bg-primary-50 dark:bg-primary-900/20"
															: "hover:bg-secondary-50 dark:hover:bg-secondary-700"
													}`}
												>
													<Icon className={`h-4 w-4 ${config.color}`} />
													<div className="flex-1 min-w-0 flex items-center gap-2">
														<span className="text-sm font-medium text-secondary-900 dark:text-white truncate">
															{item.name}
														</span>
														{item.description && (
															<>
																<span className="text-xs text-secondary-400 dark:text-white/50">
																	•
																</span>
																<span className="text-xs text-secondary-500 dark:text-white/70 truncate">
																	{item.description}
																</span>
															</>
														)}
													</div>
												</button>
											);
										})}
									</div>
								);
							})}
						</div>
					) : query.trim() ? (
						<div className="px-4 py-2 text-center text-sm text-secondary-500 dark:text-white/70">
							No results found for "{query}"
						</div>
					) : null}
				</div>
			)}
		</div>
	);
};

export default GlobalSearch;
