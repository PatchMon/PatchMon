import { GitBranch, Package, Search, Server, User, X } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { searchAPI } from "../utils/api";

const GlobalSearch = () => {
	const [query, setQuery] = useState("");
	const [results, setResults] = useState(null);
	const [isOpen, setIsOpen] = useState(false);
	const [isLoading, setIsLoading] = useState(false);
	const [selectedIndex, setSelectedIndex] = useState(-1);
	const searchRef = useRef(null);
	const inputRef = useRef(null);
	const navigate = useNavigate();

	// Debounce search
	const debounceTimerRef = useRef(null);

	const performSearch = useCallback(async (searchQuery) => {
		if (!searchQuery || searchQuery.trim().length === 0) {
			setResults(null);
			setIsOpen(false);
			return;
		}

		setIsLoading(true);
		try {
			const response = await searchAPI.global(searchQuery);
			setResults(response.data);
			setIsOpen(true);
			setSelectedIndex(-1);
		} catch (error) {
			console.error("Search error:", error);
			setResults(null);
		} finally {
			setIsLoading(false);
		}
	}, []);

	const handleInputChange = (e) => {
		const value = e.target.value;
		setQuery(value);

		// Clear previous timer
		if (debounceTimerRef.current) {
			clearTimeout(debounceTimerRef.current);
		}

		// Set new timer
		debounceTimerRef.current = setTimeout(() => {
			performSearch(value);
		}, 300);
	};

	const handleClear = () => {
		// Clear debounce timer to prevent any pending searches
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
		// Navigate based on result type
		switch (result.type) {
			case "host":
				navigate(`/hosts/${result.id}`);
				break;
			case "package":
				navigate(`/packages/${result.id}`);
				break;
			case "repository":
				navigate(`/repositories/${result.id}`);
				break;
			case "user":
				// Users don't have detail pages, so navigate to settings
				navigate("/settings/users");
				break;
			default:
				break;
		}

		// Close dropdown and clear
		handleClear();
	};

	// Close dropdown when clicking outside
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

	// Keyboard navigation
	const flattenedResults = [];
	if (results) {
		if (results.hosts?.length > 0) {
			flattenedResults.push({ type: "header", label: "Hosts" });
			flattenedResults.push(...results.hosts);
		}
		if (results.packages?.length > 0) {
			flattenedResults.push({ type: "header", label: "Packages" });
			flattenedResults.push(...results.packages);
		}
		if (results.repositories?.length > 0) {
			flattenedResults.push({ type: "header", label: "Repositories" });
			flattenedResults.push(...results.repositories);
		}
		if (results.users?.length > 0) {
			flattenedResults.push({ type: "header", label: "Users" });
			flattenedResults.push(...results.users);
		}
	}

	const navigableResults = flattenedResults.filter((r) => r.type !== "header");

	const handleKeyDown = (e) => {
		if (!isOpen || !results) return;

		switch (e.key) {
			case "ArrowDown":
				e.preventDefault();
				setSelectedIndex((prev) =>
					prev < navigableResults.length - 1 ? prev + 1 : prev,
				);
				break;
			case "ArrowUp":
				e.preventDefault();
				setSelectedIndex((prev) => (prev > 0 ? prev - 1 : -1));
				break;
			case "Enter":
				e.preventDefault();
				if (selectedIndex >= 0 && navigableResults[selectedIndex]) {
					handleResultClick(navigableResults[selectedIndex]);
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

	// Get icon for result type
	const getResultIcon = (type) => {
		switch (type) {
			case "host":
				return <Server className="h-4 w-4 text-blue-500" />;
			case "package":
				return <Package className="h-4 w-4 text-green-500" />;
			case "repository":
				return <GitBranch className="h-4 w-4 text-purple-500" />;
			case "user":
				return <User className="h-4 w-4 text-orange-500" />;
			default:
				return null;
		}
	};

	// Get display text for result
	const getResultDisplay = (result) => {
		switch (result.type) {
			case "host":
				return {
					primary: result.friendly_name || result.hostname,
					secondary: result.ip || result.hostname,
				};
			case "package":
				return {
					primary: result.name,
					secondary: result.description || result.category,
				};
			case "repository":
				return {
					primary: result.name,
					secondary: result.distribution,
				};
			case "user":
				return {
					primary: result.username,
					secondary: result.email,
				};
			default:
				return { primary: "", secondary: "" };
		}
	};

	const hasResults =
		results &&
		(results.hosts?.length > 0 ||
			results.packages?.length > 0 ||
			results.repositories?.length > 0 ||
			results.users?.length > 0);

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
					placeholder="Search hosts, packages, repos, users..."
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

			{/* Dropdown Results */}
			{isOpen && (
				<div className="absolute z-50 mt-2 w-full sm:w-[calc(100vw-2rem)] sm:max-w-md rounded-lg border border-secondary-200 bg-white shadow-lg dark:border-secondary-600 dark:bg-secondary-800 left-0 sm:left-auto right-0 sm:right-auto">
					{isLoading ? (
						<div className="px-4 py-2 text-center text-sm text-secondary-500 dark:text-white/70">
							Searching...
						</div>
					) : hasResults ? (
						<div className="max-h-96 overflow-y-auto">
							{/* Hosts */}
							{results.hosts?.length > 0 && (
								<div>
									<div className="sticky top-0 z-10 bg-secondary-50 px-3 py-1.5 text-xs font-semibold uppercase tracking-wider text-secondary-500 dark:bg-secondary-700 dark:text-white/80">
										Hosts
									</div>
									{results.hosts.map((host, _idx) => {
										const display = getResultDisplay(host);
										const globalIdx = navigableResults.findIndex(
											(r) => r.id === host.id && r.type === "host",
										);
										return (
											<button
												type="button"
												key={host.id}
												onClick={() => handleResultClick(host)}
												className={`flex w-full items-center gap-2 px-3 py-3 sm:py-1.5 text-left transition-colors min-h-[44px] ${
													globalIdx === selectedIndex
														? "bg-primary-50 dark:bg-primary-900/20"
														: "hover:bg-secondary-50 dark:hover:bg-secondary-700"
												}`}
											>
												{getResultIcon("host")}
												<div className="flex-1 min-w-0 flex items-center gap-2">
													<span className="text-sm font-medium text-secondary-900 dark:text-white truncate">
														{display.primary}
													</span>
													<span className="text-xs text-secondary-400 dark:text-white/50">
														•
													</span>
													<span className="text-xs text-secondary-500 dark:text-white/70 truncate">
														{display.secondary}
													</span>
												</div>
												<div className="flex-shrink-0 text-xs text-secondary-400 dark:text-white/60">
													{host.os_type}
												</div>
											</button>
										);
									})}
								</div>
							)}

							{/* Packages */}
							{results.packages?.length > 0 && (
								<div>
									<div className="sticky top-0 z-10 bg-secondary-50 px-3 py-1.5 text-xs font-semibold uppercase tracking-wider text-secondary-500 dark:bg-secondary-700 dark:text-white/80">
										Packages
									</div>
									{results.packages.map((pkg, _idx) => {
										const display = getResultDisplay(pkg);
										const globalIdx = navigableResults.findIndex(
											(r) => r.id === pkg.id && r.type === "package",
										);
										return (
											<button
												type="button"
												key={pkg.id}
												onClick={() => handleResultClick(pkg)}
												className={`flex w-full items-center gap-2 px-3 py-3 sm:py-1.5 text-left transition-colors min-h-[44px] ${
													globalIdx === selectedIndex
														? "bg-primary-50 dark:bg-primary-900/20"
														: "hover:bg-secondary-50 dark:hover:bg-secondary-700"
												}`}
											>
												{getResultIcon("package")}
												<div className="flex-1 min-w-0 flex items-center gap-2">
													<span className="text-sm font-medium text-secondary-900 dark:text-white truncate">
														{display.primary}
													</span>
													{display.secondary && (
														<>
															<span className="text-xs text-secondary-400">
																•
															</span>
															<span className="text-xs text-secondary-500 dark:text-white/70 truncate">
																{display.secondary}
															</span>
														</>
													)}
												</div>
												<div className="flex-shrink-0 text-xs text-secondary-400 dark:text-white/60">
													{pkg.host_count} hosts
												</div>
											</button>
										);
									})}
								</div>
							)}

							{/* Repositories */}
							{results.repositories?.length > 0 && (
								<div>
									<div className="sticky top-0 z-10 bg-secondary-50 px-3 py-1.5 text-xs font-semibold uppercase tracking-wider text-secondary-500 dark:bg-secondary-700 dark:text-white/80">
										Repositories
									</div>
									{results.repositories.map((repo, _idx) => {
										const display = getResultDisplay(repo);
										const globalIdx = navigableResults.findIndex(
											(r) => r.id === repo.id && r.type === "repository",
										);
										return (
											<button
												type="button"
												key={repo.id}
												onClick={() => handleResultClick(repo)}
												className={`flex w-full items-center gap-2 px-3 py-3 sm:py-1.5 text-left transition-colors min-h-[44px] ${
													globalIdx === selectedIndex
														? "bg-primary-50 dark:bg-primary-900/20"
														: "hover:bg-secondary-50 dark:hover:bg-secondary-700"
												}`}
											>
												{getResultIcon("repository")}
												<div className="flex-1 min-w-0 flex items-center gap-2">
													<span className="text-sm font-medium text-secondary-900 dark:text-white truncate">
														{display.primary}
													</span>
													<span className="text-xs text-secondary-400 dark:text-white/50">
														•
													</span>
													<span className="text-xs text-secondary-500 dark:text-white/70 truncate">
														{display.secondary}
													</span>
												</div>
												<div className="flex-shrink-0 text-xs text-secondary-400 dark:text-white/60">
													{repo.host_count} hosts
												</div>
											</button>
										);
									})}
								</div>
							)}

							{/* Users */}
							{results.users?.length > 0 && (
								<div>
									<div className="sticky top-0 z-10 bg-secondary-50 px-3 py-1.5 text-xs font-semibold uppercase tracking-wider text-secondary-500 dark:bg-secondary-700 dark:text-white/80">
										Users
									</div>
									{results.users.map((user, _idx) => {
										const display = getResultDisplay(user);
										const globalIdx = navigableResults.findIndex(
											(r) => r.id === user.id && r.type === "user",
										);
										return (
											<button
												type="button"
												key={user.id}
												onClick={() => handleResultClick(user)}
												className={`flex w-full items-center gap-2 px-3 py-3 sm:py-1.5 text-left transition-colors min-h-[44px] ${
													globalIdx === selectedIndex
														? "bg-primary-50 dark:bg-primary-900/20"
														: "hover:bg-secondary-50 dark:hover:bg-secondary-700"
												}`}
											>
												{getResultIcon("user")}
												<div className="flex-1 min-w-0 flex items-center gap-2">
													<span className="text-sm font-medium text-secondary-900 dark:text-white truncate">
														{display.primary}
													</span>
													<span className="text-xs text-secondary-400 dark:text-white/50">
														•
													</span>
													<span className="text-xs text-secondary-500 dark:text-white/70 truncate">
														{display.secondary}
													</span>
												</div>
												<div className="flex-shrink-0 text-xs text-secondary-400 dark:text-white/60">
													{user.role}
												</div>
											</button>
										);
									})}
								</div>
							)}
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
