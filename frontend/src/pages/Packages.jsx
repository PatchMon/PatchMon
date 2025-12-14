import { useQuery } from "@tanstack/react-query";
import {
	AlertTriangle,
	ArrowDown,
	ArrowUp,
	ArrowUpDown,
	ChevronLeft,
	ChevronRight,
	Columns,
	Eye as EyeIcon,
	EyeOff as EyeOffIcon,
	GripVertical,
	Package,
	RefreshCw,
	Search,
	Server,
	Shield,
	X,
} from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { dashboardAPI, packagesAPI } from "../utils/api";

const Packages = () => {
	const [searchTerm, setSearchTerm] = useState("");
	const [categoryFilter, setCategoryFilter] = useState("all");
	const [updateStatusFilter, setUpdateStatusFilter] = useState("all-packages");
	const [hostFilter, setHostFilter] = useState("all");
	const [sortField, setSortField] = useState("name");
	const [sortDirection, setSortDirection] = useState("asc");
	const [showColumnSettings, setShowColumnSettings] = useState(false);
	const [currentPage, setCurrentPage] = useState(1);
	const [pageSize, setPageSize] = useState(() => {
		const saved = localStorage.getItem("packages-page-size");
		if (saved) {
			const parsedSize = parseInt(saved, 10);
			// Validate that the saved page size is one of the allowed values
			if ([25, 50, 100, 200].includes(parsedSize)) {
				return parsedSize;
			}
		}
		return 25; // Default fallback
	});
	const [searchParams] = useSearchParams();
	const navigate = useNavigate();

	// Handle host filter from URL parameter
	useEffect(() => {
		const hostParam = searchParams.get("host");
		if (hostParam) {
			setHostFilter(hostParam);
		}
	}, [searchParams]);

	// Column configuration
	const [columnConfig, setColumnConfig] = useState(() => {
		const defaultConfig = [
			{ id: "name", label: "Package", visible: true, order: 0 },
			{ id: "packageHosts", label: "Installed On", visible: true, order: 1 },
			{ id: "status", label: "Status", visible: true, order: 2 },
			{ id: "latestVersion", label: "Latest Version", visible: true, order: 3 },
		];

		const saved = localStorage.getItem("packages-column-config");
		if (saved) {
			const savedConfig = JSON.parse(saved);
			// Merge with defaults to handle new columns
			return defaultConfig.map((defaultCol) => {
				const savedCol = savedConfig.find((col) => col.id === defaultCol.id);
				return savedCol ? { ...defaultCol, ...savedCol } : defaultCol;
			});
		}
		return defaultConfig;
	});

	// Update column configuration
	const updateColumnConfig = (newConfig) => {
		setColumnConfig(newConfig);
		localStorage.setItem("packages-column-config", JSON.stringify(newConfig));
	};

	// Handle hosts click (view hosts where package is installed)
	const handlePackageHostsClick = async (pkg) => {
		try {
			// Fetch all hosts for this package (packageHosts may only include hosts needing updates)
			const response = await packagesAPI.getHosts(pkg.id, { limit: 1000 });
			const hosts = response.data?.hosts || [];
			const hostIds = hosts.map((host) => host.hostId).filter(Boolean);

			if (hostIds.length === 0) {
				// No hosts found, navigate without filter
				navigate("/hosts");
				return;
			}

			// Create URL with selected hosts and filter
			const params = new URLSearchParams();
			params.set("selected", hostIds.join(","));
			params.set("filter", "selected");

			// Navigate to hosts page with selected hosts
			navigate(`/hosts?${params.toString()}`);
		} catch (error) {
			console.error("Error fetching package hosts:", error);
			// Fallback: navigate to hosts page without filter
			navigate("/hosts");
		}
	};

	// Handle URL filter parameters
	useEffect(() => {
		const filter = searchParams.get("filter");
		if (filter === "outdated") {
			// For outdated packages, we want to show all packages that need updates
			// This is the default behavior, so we don't need to change filters
			setCategoryFilter("all");
			setUpdateStatusFilter("needs-updates");
		} else if (filter === "security") {
			// For security updates, filter to show only security updates
			setUpdateStatusFilter("security-updates");
			setCategoryFilter("all");
		} else if (filter === "regular") {
			// For regular (non-security) updates
			setUpdateStatusFilter("regular-updates");
			setCategoryFilter("all");
		}
	}, [searchParams]);

	const {
		data: packagesResponse,
		isLoading,
		error,
		refetch,
		isFetching,
	} = useQuery({
		queryKey: ["packages", hostFilter, updateStatusFilter],
		queryFn: () => {
			const params = { limit: 10000 }; // High limit to effectively get all packages
			if (hostFilter && hostFilter !== "all") {
				params.host = hostFilter;
			}
			// Pass update status filter to backend to pre-filter packages
			if (updateStatusFilter === "needs-updates") {
				params.needsUpdate = "true";
			} else if (updateStatusFilter === "security-updates") {
				params.isSecurityUpdate = "true";
			}
			return packagesAPI.getAll(params).then((res) => res.data);
		},
		staleTime: 5 * 60 * 1000, // Data stays fresh for 5 minutes
		refetchOnWindowFocus: false, // Don't refetch when window regains focus
	});

	// Extract packages from the response and normalise the data structure
	const packages = useMemo(() => {
		if (!packagesResponse?.packages) return [];

		return packagesResponse.packages.map((pkg) => ({
			...pkg,
			// Normalise field names to match the frontend expectations
			packageHostsCount: pkg.packageHostsCount || pkg.stats?.totalInstalls || 0,
			latestVersion: pkg.latest_version || pkg.latestVersion || "Unknown",
			isUpdatable: (pkg.stats?.updatesNeeded || 0) > 0,
			isSecurityUpdate: (pkg.stats?.securityUpdates || 0) > 0,
			// Ensure we have hosts array (for packages, this contains all hosts where the package is installed)
			packageHosts: pkg.packageHosts || [],
		}));
	}, [packagesResponse]);

	// Fetch dashboard stats for card counts (consistent with homepage)
	const { data: dashboardStats } = useQuery({
		queryKey: ["dashboardStats"],
		queryFn: () => dashboardAPI.getStats().then((res) => res.data),
		staleTime: 5 * 60 * 1000, // Data stays fresh for 5 minutes
		refetchOnWindowFocus: false, // Don't refetch when window regains focus
	});

	// Fetch hosts data to get total packages count
	const { data: hosts } = useQuery({
		queryKey: ["hosts"],
		queryFn: () => dashboardAPI.getHosts().then((res) => res.data),
		staleTime: 5 * 60 * 1000, // Data stays fresh for 5 minutes
		refetchOnWindowFocus: false, // Don't refetch when window regains focus
	});

	// Filter and sort packages
	const filteredAndSortedPackages = useMemo(() => {
		if (!packages) return [];

		// Filter packages
		const filtered = packages.filter((pkg) => {
			const matchesSearch =
				pkg.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
				pkg.description?.toLowerCase().includes(searchTerm.toLowerCase());

			const matchesCategory =
				categoryFilter === "all" || pkg.category === categoryFilter;

			const matchesUpdateStatus =
				updateStatusFilter === "all-packages" ||
				(updateStatusFilter === "needs-updates" &&
					(pkg.stats?.updatesNeeded || 0) > 0) ||
				(updateStatusFilter === "security-updates" &&
					(pkg.stats?.securityUpdates || 0) > 0) ||
				(updateStatusFilter === "regular-updates" &&
					(pkg.stats?.updatesNeeded || 0) > 0 &&
					(pkg.stats?.securityUpdates || 0) === 0);

			const packageHosts = pkg.packageHosts || [];
			const matchesHost =
				hostFilter === "all" ||
				packageHosts.some((host) => host.hostId === hostFilter);

			return (
				matchesSearch && matchesCategory && matchesUpdateStatus && matchesHost
			);
		});

		// Sorting
		filtered.sort((a, b) => {
			let aValue, bValue;

			switch (sortField) {
				case "name":
					aValue = a.name?.toLowerCase() || "";
					bValue = b.name?.toLowerCase() || "";
					break;
				case "latestVersion":
					aValue = a.latestVersion?.toLowerCase() || "";
					bValue = b.latestVersion?.toLowerCase() || "";
					break;
				case "packageHosts":
					aValue = a.packageHostsCount || a.packageHosts?.length || 0;
					bValue = b.packageHostsCount || b.packageHosts?.length || 0;
					break;
				case "status": {
					// Handle sorting for the three status states: Up to Date, Update Available, Security Update Available
					const aNeedsUpdates = (a.stats?.updatesNeeded || 0) > 0;
					const bNeedsUpdates = (b.stats?.updatesNeeded || 0) > 0;

					// Define priority order: Security Update (0) > Regular Update (1) > Up to Date (2)
					let aPriority, bPriority;

					if (!aNeedsUpdates) {
						aPriority = 2; // Up to Date
					} else if (a.isSecurityUpdate) {
						aPriority = 0; // Security Update
					} else {
						aPriority = 1; // Regular Update
					}

					if (!bNeedsUpdates) {
						bPriority = 2; // Up to Date
					} else if (b.isSecurityUpdate) {
						bPriority = 0; // Security Update
					} else {
						bPriority = 1; // Regular Update
					}

					aValue = aPriority;
					bValue = bPriority;
					break;
				}
				default:
					aValue = a.name?.toLowerCase() || "";
					bValue = b.name?.toLowerCase() || "";
			}

			if (aValue < bValue) return sortDirection === "asc" ? -1 : 1;
			if (aValue > bValue) return sortDirection === "asc" ? 1 : -1;
			return 0;
		});

		return filtered;
	}, [
		packages,
		searchTerm,
		categoryFilter,
		updateStatusFilter,
		sortField,
		sortDirection,
		hostFilter,
	]);

	// Calculate pagination
	const totalPages = Math.ceil(filteredAndSortedPackages.length / pageSize);
	const startIndex = (currentPage - 1) * pageSize;
	const endIndex = startIndex + pageSize;
	const paginatedPackages = filteredAndSortedPackages.slice(
		startIndex,
		endIndex,
	);

	// Reset to first page when filters or page size change
	// biome-ignore lint/correctness/useExhaustiveDependencies: We want this effect to run when filter values or page size change to reset pagination
	useEffect(() => {
		setCurrentPage(1);
	}, [searchTerm, categoryFilter, updateStatusFilter, hostFilter, pageSize]);

	// Function to handle page size change and save to localStorage
	const handlePageSizeChange = (newPageSize) => {
		setPageSize(newPageSize);
		localStorage.setItem("packages-page-size", newPageSize.toString());
	};

	// Get visible columns in order
	const visibleColumns = columnConfig
		.filter((col) => col.visible)
		.sort((a, b) => a.order - b.order);

	// Sorting functions
	const handleSort = (field) => {
		if (sortField === field) {
			setSortDirection(sortDirection === "asc" ? "desc" : "asc");
		} else {
			setSortField(field);
			setSortDirection("asc");
		}
	};

	const getSortIcon = (field) => {
		if (sortField !== field) return <ArrowUpDown className="h-4 w-4" />;
		return sortDirection === "asc" ? (
			<ArrowUp className="h-4 w-4" />
		) : (
			<ArrowDown className="h-4 w-4" />
		);
	};

	// Column management functions
	const toggleColumnVisibility = (columnId) => {
		const newConfig = columnConfig.map((col) =>
			col.id === columnId ? { ...col, visible: !col.visible } : col,
		);
		updateColumnConfig(newConfig);
	};

	const reorderColumns = (fromIndex, toIndex) => {
		const newConfig = [...columnConfig];
		const [movedColumn] = newConfig.splice(fromIndex, 1);
		newConfig.splice(toIndex, 0, movedColumn);

		// Update order values
		const updatedConfig = newConfig.map((col, index) => ({
			...col,
			order: index,
		}));
		updateColumnConfig(updatedConfig);
	};

	const resetColumns = () => {
		const defaultConfig = [
			{ id: "name", label: "Package", visible: true, order: 0 },
			{ id: "packageHosts", label: "Installed On", visible: true, order: 1 },
			{ id: "status", label: "Status", visible: true, order: 2 },
			{ id: "latestVersion", label: "Latest Version", visible: true, order: 3 },
		];
		updateColumnConfig(defaultConfig);
	};

	// Helper function to render table cell content
	const renderCellContent = (column, pkg) => {
		switch (column.id) {
			case "name":
				return (
					<button
						type="button"
						onClick={() => navigate(`/packages/${pkg.id}`)}
						className="flex items-center text-left hover:bg-secondary-100 dark:hover:bg-secondary-700 rounded p-2 -m-2 transition-colors group w-full"
					>
						<Package className="h-5 w-5 text-secondary-400 mr-3 flex-shrink-0" />
						<div className="flex-1">
							<div className="text-sm font-medium text-secondary-900 dark:text-white group-hover:text-primary-600 dark:group-hover:text-primary-400">
								{pkg.name}
							</div>
							{pkg.description && (
								<div className="text-sm text-secondary-500 dark:text-secondary-300 max-w-md truncate">
									{pkg.description}
								</div>
							)}
							{pkg.category && (
								<div className="text-xs text-secondary-400 dark:text-secondary-400">
									Category: {pkg.category}
								</div>
							)}
						</div>
					</button>
				);
			case "packageHosts": {
				// Show total number of hosts where this package is installed
				const installedHostsCount =
					pkg.packageHostsCount ||
					pkg.stats?.totalInstalls ||
					pkg.packageHosts?.length ||
					0;
				// For packages that need updates, show how many need updates
				const hostsNeedingUpdates = pkg.stats?.updatesNeeded || 0;

				const displayText =
					hostsNeedingUpdates > 0 && hostsNeedingUpdates < installedHostsCount
						? `${hostsNeedingUpdates}/${installedHostsCount} hosts`
						: `${installedHostsCount} host${installedHostsCount !== 1 ? "s" : ""}`;

				const titleText =
					hostsNeedingUpdates > 0 && hostsNeedingUpdates < installedHostsCount
						? `${hostsNeedingUpdates} of ${installedHostsCount} hosts need updates`
						: `Installed on ${installedHostsCount} host${installedHostsCount !== 1 ? "s" : ""}`;

				return (
					<button
						type="button"
						onClick={() => handlePackageHostsClick(pkg)}
						className="text-left hover:bg-secondary-100 dark:hover:bg-secondary-700 rounded p-1 -m-1 transition-colors group"
						title={titleText}
					>
						<div className="text-sm text-secondary-900 dark:text-white group-hover:text-primary-600 dark:group-hover:text-primary-400">
							{displayText}
						</div>
					</button>
				);
			}
			case "status": {
				// Check if this package needs updates
				const needsUpdates = (pkg.stats?.updatesNeeded || 0) > 0;

				if (!needsUpdates) {
					return <span className="badge-success">Up to Date</span>;
				}

				return pkg.isSecurityUpdate ? (
					<span className="badge-danger">
						<Shield className="h-3 w-3" />
						Security Update Available
					</span>
				) : (
					<span className="badge-warning">Update Available</span>
				);
			}
			case "latestVersion":
				return (
					<div
						className="text-sm text-secondary-900 dark:text-white max-w-xs truncate"
						title={pkg.latestVersion || "Unknown"}
					>
						{pkg.latestVersion || "Unknown"}
					</div>
				);
			default:
				return null;
		}
	};

	// Get unique categories
	const categories =
		[...new Set(packages?.map((pkg) => pkg.category).filter(Boolean))] || [];

	// Calculate unique package hosts
	const uniquePackageHosts = new Set();
	packages?.forEach((pkg) => {
		// Only count hosts for packages that need updates
		if ((pkg.stats?.updatesNeeded || 0) > 0) {
			const packageHosts = pkg.packageHosts || [];
			packageHosts.forEach((host) => {
				uniquePackageHosts.add(host.hostId);
			});
		}
	});
	const uniquePackageHostsCount = uniquePackageHosts.size;

	// Calculate total packages installed
	// Show unique package count (same as table) for consistency
	const totalPackagesCount = packages?.length || 0;

	// Calculate total installations across all hosts
	const totalInstallationsCount =
		packages?.reduce((sum, pkg) => sum + (pkg.stats?.totalInstalls || 0), 0) ||
		0;

	// Use dashboard stats for outdated packages count (consistent with homepage)
	const outdatedPackagesCount =
		dashboardStats?.cards?.totalOutdatedPackages || 0;

	// Use dashboard stats for security updates count (consistent with homepage)
	const securityUpdatesCount = dashboardStats?.cards?.securityUpdates || 0;

	if (isLoading) {
		return (
			<div className="flex items-center justify-center h-64">
				<RefreshCw className="h-8 w-8 animate-spin text-primary-600" />
			</div>
		);
	}

	if (error) {
		return (
			<div className="space-y-6">
				<div className="bg-danger-50 border border-danger-200 rounded-md p-4">
					<div className="flex">
						<AlertTriangle className="h-5 w-5 text-danger-400" />
						<div className="ml-3">
							<h3 className="text-sm font-medium text-danger-800">
								Error loading packages
							</h3>
							<p className="text-sm text-danger-700 mt-1">
								{error.message || "Failed to load packages"}
							</p>
							<button
								type="button"
								onClick={() => refetch()}
								className="mt-2 btn-danger text-xs"
							>
								Try again
							</button>
						</div>
					</div>
				</div>
			</div>
		);
	}

	return (
		<div className="md:h-[calc(100vh-7rem)] flex flex-col md:overflow-hidden min-h-0">
			{/* Page Header */}
			<div className="flex items-center justify-between mb-6">
				<div>
					<h1 className="text-2xl font-semibold text-secondary-900 dark:text-white">
						Packages
					</h1>
					<p className="text-sm text-secondary-600 dark:text-secondary-400 mt-1">
						Manage package updates and security patches
					</p>
				</div>
				<div className="flex items-center gap-3">
					<button
						type="button"
						onClick={() => refetch()}
						disabled={isFetching}
						className="btn-outline flex items-center gap-2"
						title="Refresh packages data"
					>
						<RefreshCw
							className={`h-4 w-4 ${isFetching ? "animate-spin" : ""}`}
						/>
						{isFetching ? "Refreshing..." : "Refresh"}
					</button>
				</div>
			</div>

			{/* Summary Stats */}
			<div className="grid grid-cols-2 lg:grid-cols-5 gap-4 mb-6 flex-shrink-0">
				<div className="card p-4 cursor-pointer hover:shadow-card-hover dark:hover:shadow-card-hover-dark transition-shadow duration-200">
					<div className="flex items-center">
						<Package className="h-5 w-5 text-primary-600 mr-2" />
						<div>
							<p className="text-sm text-secondary-500 dark:text-white">
								Packages
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{totalPackagesCount}
							</p>
						</div>
					</div>
				</div>

				<div className="card p-4 cursor-pointer hover:shadow-card-hover dark:hover:shadow-card-hover-dark transition-shadow duration-200">
					<div className="flex items-center">
						<Package className="h-5 w-5 text-blue-600 mr-2" />
						<div>
							<p className="text-sm text-secondary-500 dark:text-white">
								Installations
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{totalInstallationsCount}
							</p>
						</div>
					</div>
				</div>

				<button
					type="button"
					onClick={() => {
						setUpdateStatusFilter("needs-updates");
						setCategoryFilter("all");
						setHostFilter("all");
						setSearchTerm("");
					}}
					className="card p-4 cursor-pointer hover:shadow-card-hover dark:hover:shadow-card-hover-dark transition-shadow duration-200 text-left w-full"
					title="Click to filter packages that need updates"
				>
					<div className="flex items-center">
						<Package className="h-5 w-5 text-warning-600 mr-2" />
						<div>
							<p className="text-sm text-secondary-500 dark:text-white">
								Outdated Packages
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{outdatedPackagesCount}
							</p>
						</div>
					</div>
				</button>

				<button
					type="button"
					onClick={() => {
						setUpdateStatusFilter("security-updates");
						setCategoryFilter("all");
						setHostFilter("all");
						setSearchTerm("");
					}}
					className="card p-4 cursor-pointer hover:shadow-card-hover dark:hover:shadow-card-hover-dark transition-shadow duration-200 text-left w-full"
					title="Click to filter packages with security updates"
				>
					<div className="flex items-center">
						<Shield className="h-5 w-5 text-danger-600 mr-2" />
						<div>
							<p className="text-sm text-secondary-500 dark:text-white">
								Security Packages
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{securityUpdatesCount}
							</p>
						</div>
					</div>
				</button>

				<button
					type="button"
					onClick={() => navigate("/hosts?filter=needsUpdates")}
					className="card p-4 cursor-pointer hover:shadow-card-hover dark:hover:shadow-card-hover-dark transition-shadow duration-200 text-left w-full"
					title="Click to view hosts that need updates"
				>
					<div className="flex items-center">
						<Server className="h-5 w-5 text-warning-600 mr-2" />
						<div>
							<p className="text-sm text-secondary-500 dark:text-white">
								Outdated Hosts
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{uniquePackageHostsCount}
							</p>
						</div>
					</div>
				</button>
			</div>

			{/* Packages List */}
			<div className="card md:flex-1 flex flex-col md:overflow-hidden min-h-0">
				<div className="px-4 py-4 sm:p-4 md:flex-1 flex flex-col md:overflow-hidden min-h-0">
					<div className="flex items-center justify-end mb-4">
						{/* Empty selection controls area to match hosts page spacing */}
					</div>

					{/* Table Controls */}
					<div className="mb-4 space-y-4">
						<div className="flex flex-col sm:flex-row gap-4">
							{/* Search */}
							<div className="hidden md:flex flex-1">
								<div className="relative w-full">
									<Search className="absolute left-3 top-1/2 transform -translate-y-1/2 h-4 w-4 text-secondary-400 dark:text-secondary-500" />
									<input
										type="text"
										placeholder="Search packages..."
										value={searchTerm}
										onChange={(e) => setSearchTerm(e.target.value)}
										className="w-full pl-10 pr-4 py-2 border border-secondary-300 dark:border-secondary-600 rounded-md focus:ring-2 focus:ring-primary-500 focus:border-transparent bg-white dark:bg-secondary-800 text-secondary-900 dark:text-white placeholder-secondary-500 dark:placeholder-secondary-400"
									/>
								</div>
							</div>

							{/* Category Filter */}
							<div className="sm:w-48">
								<select
									value={categoryFilter}
									onChange={(e) => setCategoryFilter(e.target.value)}
									className="w-full px-3 py-2 border border-secondary-300 dark:border-secondary-600 rounded-md focus:ring-2 focus:ring-primary-500 focus:border-transparent bg-white dark:bg-secondary-800 text-secondary-900 dark:text-white"
								>
									<option value="all">All Categories</option>
									{categories.map((category) => (
										<option key={category} value={category}>
											{category}
										</option>
									))}
								</select>
							</div>

							{/* Update Status Filter */}
							<div className="sm:w-48">
								<select
									value={updateStatusFilter}
									onChange={(e) => setUpdateStatusFilter(e.target.value)}
									className="w-full px-3 py-2 border border-secondary-300 dark:border-secondary-600 rounded-md focus:ring-2 focus:ring-primary-500 focus:border-transparent bg-white dark:bg-secondary-800 text-secondary-900 dark:text-white"
								>
									<option value="all-packages">All Packages</option>
									<option value="needs-updates">
										Packages Needing Updates
									</option>
									<option value="security-updates">
										Security Updates Only
									</option>
									<option value="regular-updates">Regular Updates Only</option>
								</select>
							</div>

							{/* Host Filter */}
							<div className="sm:w-48">
								<select
									value={hostFilter}
									onChange={(e) => setHostFilter(e.target.value)}
									className="w-full px-3 py-2 border border-secondary-300 dark:border-secondary-600 rounded-md focus:ring-2 focus:ring-primary-500 focus:border-transparent bg-white dark:bg-secondary-800 text-secondary-900 dark:text-white"
								>
									<option value="all">All Hosts</option>
									{hosts?.map((host) => (
										<option key={host.id} value={host.id}>
											{host.friendly_name}
										</option>
									))}
								</select>
							</div>

							{/* Columns Button */}
							<div className="hidden md:flex items-center">
								<button
									type="button"
									onClick={() => setShowColumnSettings(true)}
									className="flex items-center gap-2 px-3 py-2 text-sm text-secondary-700 dark:text-secondary-300 bg-white dark:bg-secondary-700 border border-secondary-300 dark:border-secondary-600 rounded-md hover:bg-secondary-50 dark:hover:bg-secondary-600 transition-colors"
								>
									<Columns className="h-4 w-4" />
									Columns
								</button>
							</div>
						</div>
					</div>

					<div className="md:flex-1 md:overflow-hidden">
						{filteredAndSortedPackages.length === 0 ? (
							<div className="text-center py-8">
								<Package className="h-12 w-12 text-secondary-400 mx-auto mb-4" />
								<p className="text-secondary-500 dark:text-secondary-300">
									{packages?.length === 0
										? "No packages found"
										: "No packages match your filters"}
								</p>
								{packages?.length === 0 && (
									<p className="text-sm text-secondary-400 dark:text-secondary-400 mt-2">
										Packages will appear here once hosts start reporting their
										installed packages
									</p>
								)}
							</div>
						) : (
							<>
								{/* Mobile Card Layout */}
								<div className="md:hidden space-y-3 pb-4">
									{paginatedPackages.map((pkg) => (
										<div key={pkg.id} className="card p-4 space-y-3">
											{/* Package Name */}
											<button
												type="button"
												onClick={() => navigate(`/packages/${pkg.id}`)}
												className="text-left w-full"
											>
												<div className="flex items-center gap-3">
													<Package className="h-5 w-5 text-secondary-400 flex-shrink-0" />
													<div className="text-base font-semibold text-secondary-900 dark:text-white hover:text-primary-600 dark:hover:text-primary-400">
														{pkg.name}
													</div>
												</div>
											</button>

											{/* Status and Hosts on same line */}
											<div className="flex items-center justify-between gap-2">
												<div className="flex items-center gap-1.5">
													{(() => {
														const needsUpdates =
															(pkg.stats?.updatesNeeded || 0) > 0;
														if (!needsUpdates) {
															return (
																<span className="badge-success text-xs">
																	Up to Date
																</span>
															);
														}
														return pkg.isSecurityUpdate ? (
															<span className="badge-danger text-xs flex items-center gap-1">
																<Shield className="h-3 w-3" />
																Security
															</span>
														) : (
															<span className="badge-warning text-xs">
																Update
															</span>
														);
													})()}
												</div>
												<button
													type="button"
													onClick={() => handlePackageHostsClick(pkg)}
													className="text-sm hover:bg-secondary-100 dark:hover:bg-secondary-700 rounded px-2 py-1 -mx-2 transition-colors"
												>
													<span className="text-secondary-500 dark:text-secondary-400">
														On:&nbsp;
													</span>
													<span className="text-secondary-900 dark:text-white font-semibold">
														{(() => {
															const installedHostsCount =
																pkg.packageHostsCount ||
																pkg.stats?.totalInstalls ||
																pkg.packageHosts?.length ||
																0;
															const hostsNeedingUpdates =
																pkg.stats?.updatesNeeded || 0;
															return hostsNeedingUpdates > 0 &&
																hostsNeedingUpdates < installedHostsCount
																? `${hostsNeedingUpdates}/${installedHostsCount}`
																: installedHostsCount;
														})()}
													</span>
													<span className="text-secondary-500 dark:text-secondary-400">
														{(() => {
															const installedHostsCount =
																pkg.packageHostsCount ||
																pkg.stats?.totalInstalls ||
																pkg.packageHosts?.length ||
																0;
															return ` host${installedHostsCount !== 1 ? "s" : ""}`;
														})()}
													</span>
												</button>
											</div>

											{/* Version Info */}
											<div className="pt-2 border-t border-secondary-200 dark:border-secondary-600">
												<div className="text-sm">
													<span className="text-secondary-500 dark:text-secondary-400">
														Latest:&nbsp;
													</span>
													<span className="text-secondary-900 dark:text-white font-mono text-sm">
														{pkg.latestVersion || "Unknown"}
													</span>
												</div>
											</div>
										</div>
									))}
								</div>

								{/* Desktop Table Layout */}
								<div className="hidden md:block h-full overflow-auto">
									<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-600">
										<thead className="bg-secondary-50 dark:bg-secondary-700 sticky top-0 z-10">
											<tr>
												{visibleColumns.map((column) => (
													<th
														key={column.id}
														className="px-4 py-2 text-center text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider"
													>
														<button
															type="button"
															onClick={() => handleSort(column.id)}
															className="flex items-center gap-1 hover:text-secondary-700 dark:hover:text-secondary-200 transition-colors"
														>
															{column.label}
															{getSortIcon(column.id)}
														</button>
													</th>
												))}
											</tr>
										</thead>
										<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-600">
											{paginatedPackages.map((pkg) => (
												<tr
													key={pkg.id}
													className="hover:bg-secondary-50 dark:hover:bg-secondary-700 transition-colors"
												>
													{visibleColumns.map((column) => (
														<td
															key={column.id}
															className="px-4 py-2 whitespace-nowrap text-center"
														>
															{renderCellContent(column, pkg)}
														</td>
													))}
												</tr>
											))}
										</tbody>
									</table>
								</div>
							</>
						)}
					</div>

					{/* Pagination Controls */}
					{filteredAndSortedPackages.length > 0 && (
						<div className="flex items-center justify-between px-6 py-3 bg-white dark:bg-secondary-800 border-t border-secondary-200 dark:border-secondary-600">
							<div className="flex items-center gap-4">
								<div className="flex items-center gap-2">
									<span className="text-sm text-secondary-700 dark:text-secondary-300">
										Rows per page:
									</span>
									<select
										value={pageSize}
										onChange={(e) =>
											handlePageSizeChange(Number(e.target.value))
										}
										className="text-sm border border-secondary-300 dark:border-secondary-600 rounded px-2 py-1 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
									>
										<option value={25}>25</option>
										<option value={50}>50</option>
										<option value={100}>100</option>
										<option value={200}>200</option>
									</select>
								</div>
								<span className="text-sm text-secondary-700 dark:text-secondary-300">
									{startIndex + 1}-
									{Math.min(endIndex, filteredAndSortedPackages.length)} of{" "}
									{filteredAndSortedPackages.length}
								</span>
							</div>
							<div className="flex items-center gap-2">
								<button
									type="button"
									onClick={() => setCurrentPage(currentPage - 1)}
									disabled={currentPage === 1}
									className="p-1 rounded hover:bg-secondary-100 dark:hover:bg-secondary-600 disabled:opacity-50 disabled:cursor-not-allowed"
								>
									<ChevronLeft className="h-4 w-4" />
								</button>
								<span className="text-sm text-secondary-700 dark:text-secondary-300">
									Page {currentPage} of {totalPages}
								</span>
								<button
									type="button"
									onClick={() => setCurrentPage(currentPage + 1)}
									disabled={currentPage === totalPages}
									className="p-1 rounded hover:bg-secondary-100 dark:hover:bg-secondary-600 disabled:opacity-50 disabled:cursor-not-allowed"
								>
									<ChevronRight className="h-4 w-4" />
								</button>
							</div>
						</div>
					)}
				</div>
			</div>

			{/* Column Settings Modal */}
			{showColumnSettings && (
				<ColumnSettingsModal
					columnConfig={columnConfig}
					onClose={() => setShowColumnSettings(false)}
					onToggleVisibility={toggleColumnVisibility}
					onReorder={reorderColumns}
					onReset={resetColumns}
				/>
			)}
		</div>
	);
};

// Column Settings Modal Component
const ColumnSettingsModal = ({
	columnConfig,
	onClose,
	onToggleVisibility,
	onReorder,
	onReset,
}) => {
	const [draggedIndex, setDraggedIndex] = useState(null);

	const handleDragStart = (e, index) => {
		setDraggedIndex(index);
		e.dataTransfer.effectAllowed = "move";
	};

	const handleDragOver = (e) => {
		e.preventDefault();
		e.dataTransfer.dropEffect = "move";
	};

	const handleDrop = (e, dropIndex) => {
		e.preventDefault();
		if (draggedIndex !== null && draggedIndex !== dropIndex) {
			onReorder(draggedIndex, dropIndex);
		}
		setDraggedIndex(null);
	};

	return (
		<div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
			<div className="bg-white dark:bg-secondary-800 rounded-lg p-6 w-full max-w-md">
				<div className="flex justify-between items-center mb-4">
					<h3 className="text-lg font-medium text-secondary-900 dark:text-white">
						Customize Columns
					</h3>
					<button
						type="button"
						onClick={onClose}
						className="text-secondary-400 hover:text-secondary-600 dark:text-secondary-500 dark:hover:text-secondary-300"
					>
						<X className="h-5 w-5" />
					</button>
				</div>

				<div className="space-y-2">
					{columnConfig.map((column, index) => (
						// biome-ignore lint/a11y/useSemanticElements: Draggable element requires div
						<div
							key={column.id}
							role="button"
							tabIndex={0}
							draggable
							onDragStart={(e) => handleDragStart(e, index)}
							onDragOver={handleDragOver}
							onDrop={(e) => handleDrop(e, index)}
							className={`flex items-center justify-between p-3 border rounded-lg cursor-move w-full ${
								draggedIndex === index
									? "opacity-50"
									: "hover:bg-secondary-50 dark:hover:bg-secondary-700"
							} border-secondary-200 dark:border-secondary-600`}
						>
							<div className="flex items-center gap-3">
								<GripVertical className="h-4 w-4 text-secondary-400 dark:text-secondary-500" />
								<span className="text-sm font-medium text-secondary-900 dark:text-white">
									{column.label}
								</span>
							</div>
							<button
								type="button"
								onClick={(e) => {
									e.stopPropagation();
									onToggleVisibility(column.id);
								}}
								className={`p-1 rounded ${
									column.visible
										? "text-primary-600 hover:text-primary-700"
										: "text-secondary-400 hover:text-secondary-600"
								}`}
							>
								{column.visible ? (
									<EyeIcon className="h-4 w-4" />
								) : (
									<EyeOffIcon className="h-4 w-4" />
								)}
							</button>
						</div>
					))}
				</div>

				<div className="flex justify-between mt-6">
					<button
						type="button"
						onClick={onReset}
						className="px-4 py-2 text-sm font-medium text-secondary-700 dark:text-secondary-200 bg-white dark:bg-secondary-700 border border-secondary-300 dark:border-secondary-600 rounded-md hover:bg-secondary-50 dark:hover:bg-secondary-600"
					>
						Reset to Default
					</button>
					<button
						type="button"
						onClick={onClose}
						className="px-4 py-2 text-sm font-medium text-white bg-primary-600 rounded-md hover:bg-primary-700"
					>
						Done
					</button>
				</div>
			</div>
		</div>
	);
};

export default Packages;
