import { useQuery } from "@tanstack/react-query";
import {
	AlertTriangle,
	ArrowLeft,
	Calendar,
	ChevronRight,
	Download,
	Package,
	RefreshCw,
	RotateCcw,
	Search,
	Server,
	Shield,
} from "lucide-react";
import { useMemo, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { formatRelativeTime, packagesAPI } from "../utils/api";

const PackageDetail = () => {
	const { packageId } = useParams();
	const decodedPackageId = decodeURIComponent(packageId || "");
	const navigate = useNavigate();
	const [searchTerm, setSearchTerm] = useState("");
	const [currentPage, setCurrentPage] = useState(1);
	const [pageSize, setPageSize] = useState(25);

	// Fetch package details
	const {
		data: packageData,
		isLoading: isLoadingPackage,
		error: packageError,
		refetch: refetchPackage,
	} = useQuery({
		queryKey: ["package", decodedPackageId],
		queryFn: () =>
			packagesAPI.getById(decodedPackageId).then((res) => res.data),
		staleTime: 5 * 60 * 1000,
		refetchOnWindowFocus: false,
		enabled: !!decodedPackageId,
	});

	// Fetch hosts that have this package
	const {
		data: hostsData,
		isLoading: isLoadingHosts,
		error: hostsError,
		refetch: refetchHosts,
	} = useQuery({
		queryKey: ["package-hosts", decodedPackageId, searchTerm],
		queryFn: () =>
			packagesAPI
				.getHosts(decodedPackageId, { search: searchTerm, limit: 1000 })
				.then((res) => res.data),
		staleTime: 5 * 60 * 1000,
		refetchOnWindowFocus: false,
		enabled: !!decodedPackageId,
	});

	const hosts = hostsData?.hosts || [];

	// Filter and paginate hosts
	const filteredAndPaginatedHosts = useMemo(() => {
		let filtered = hosts;

		if (searchTerm) {
			filtered = hosts.filter(
				(host) =>
					host.friendlyName?.toLowerCase().includes(searchTerm.toLowerCase()) ||
					host.hostname?.toLowerCase().includes(searchTerm.toLowerCase()),
			);
		}

		const startIndex = (currentPage - 1) * pageSize;
		const endIndex = startIndex + pageSize;
		return filtered.slice(startIndex, endIndex);
	}, [hosts, searchTerm, currentPage, pageSize]);

	const totalPages = Math.ceil(
		(searchTerm
			? hosts.filter(
					(host) =>
						host.friendlyName
							?.toLowerCase()
							.includes(searchTerm.toLowerCase()) ||
						host.hostname?.toLowerCase().includes(searchTerm.toLowerCase()),
				).length
			: hosts.length) / pageSize,
	);

	const handleHostClick = (hostId) => {
		navigate(`/hosts/${hostId}`);
	};

	const handleRefresh = () => {
		refetchPackage();
		refetchHosts();
	};

	if (isLoadingPackage) {
		return (
			<div className="flex items-center justify-center h-64">
				<RefreshCw className="h-8 w-8 animate-spin text-primary-600" />
			</div>
		);
	}

	if (packageError) {
		return (
			<div className="space-y-6">
				<div className="bg-danger-50 border border-danger-200 rounded-md p-4">
					<div className="flex">
						<AlertTriangle className="h-5 w-5 text-danger-400" />
						<div className="ml-3">
							<h3 className="text-sm font-medium text-danger-800">
								Error loading package
							</h3>
							<p className="text-sm text-danger-700 mt-1">
								{packageError.message || "Failed to load package details"}
							</p>
							<button
								type="button"
								onClick={() => refetchPackage()}
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

	if (!packageData) {
		return (
			<div className="space-y-6">
				<div className="text-center py-8">
					<Package className="h-12 w-12 text-secondary-400 mx-auto mb-4" />
					<p className="text-secondary-500 dark:text-secondary-300">
						Package not found
					</p>
				</div>
			</div>
		);
	}

	const pkg = packageData;
	const stats = packageData.stats || {};

	return (
		<div className="space-y-4 sm:space-y-6">
			{/* Header */}
			<div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 sm:gap-4">
				<div className="flex items-center gap-2 sm:gap-4 flex-wrap">
					<button
						type="button"
						onClick={() => navigate("/packages")}
						className="flex items-center gap-2 text-secondary-600 hover:text-secondary-900 dark:text-secondary-400 dark:hover:text-white transition-colors text-sm sm:text-base"
					>
						<ArrowLeft className="h-4 w-4" />
						<span className="hidden sm:inline">Back to Packages</span>
						<span className="sm:hidden">Back</span>
					</button>
					<ChevronRight className="h-4 w-4 text-secondary-400 hidden sm:block" />
					<h1 className="text-xl sm:text-2xl font-semibold text-secondary-900 dark:text-white truncate">
						{pkg.name}
					</h1>
					{stats.updatesNeeded > 0 ? (
						stats.securityUpdates > 0 ? (
							<span className="badge-danger flex items-center gap-1">
								<Shield className="h-3 w-3" />
								Security Update Available
							</span>
						) : (
							<span className="badge-warning">Update Available</span>
						)
					) : (
						<span className="badge-success">Up to Date</span>
					)}
				</div>
				<button
					type="button"
					onClick={handleRefresh}
					disabled={isLoadingPackage || isLoadingHosts}
					className="btn-outline flex items-center gap-2 text-sm sm:text-base self-start sm:self-auto"
				>
					<RefreshCw
						className={`h-4 w-4 ${
							isLoadingPackage || isLoadingHosts ? "animate-spin" : ""
						}`}
					/>
					Refresh
				</button>
			</div>

			{/* Package Stats Cards */}
			<div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
				{/* Latest Version */}
				<div className="card p-4">
					<div className="flex items-center">
						<Download className="h-5 w-5 text-primary-600 mr-2 flex-shrink-0" />
						<div className="min-w-0 flex-1">
							<p className="text-sm text-secondary-500 dark:text-secondary-400">
								Latest Version
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white truncate">
								{pkg.latest_version || "Unknown"}
							</p>
						</div>
					</div>
				</div>

				{/* Updated Date */}
				<div className="card p-4">
					<div className="flex items-center">
						<Calendar className="h-5 w-5 text-primary-600 mr-2 flex-shrink-0" />
						<div className="min-w-0 flex-1">
							<p className="text-sm text-secondary-500 dark:text-secondary-400">
								Updated
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{pkg.updated_at ? formatRelativeTime(pkg.updated_at) : "Never"}
							</p>
						</div>
					</div>
				</div>

				{/* Hosts with this Package */}
				<div className="card p-4">
					<div className="flex items-center">
						<Server className="h-5 w-5 text-primary-600 mr-2 flex-shrink-0" />
						<div className="min-w-0 flex-1">
							<p className="text-sm text-secondary-500 dark:text-secondary-400">
								Hosts with Package
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{stats.totalInstalls || 0}
							</p>
						</div>
					</div>
				</div>

				{/* Up to Date */}
				<div className="card p-4">
					<div className="flex items-center">
						<Shield className="h-5 w-5 text-success-600 mr-2 flex-shrink-0" />
						<div className="min-w-0 flex-1">
							<p className="text-sm text-secondary-500 dark:text-secondary-400">
								Up to Date
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{(stats.totalInstalls || 0) - (stats.updatesNeeded || 0)}
							</p>
						</div>
					</div>
				</div>
			</div>

			{/* Hosts List */}
			<div className="card">
				<div className="px-4 sm:px-6 py-4 border-b border-secondary-200 dark:border-secondary-600">
					{/* Search */}
					<div className="relative w-full">
						<Search className="absolute left-3 top-1/2 transform -translate-y-1/2 h-4 w-4 text-secondary-400" />
						<input
							type="text"
							placeholder="Search hosts..."
							value={searchTerm}
							onChange={(e) => {
								setSearchTerm(e.target.value);
								setCurrentPage(1);
							}}
							className="w-full pl-10 pr-4 py-2 border border-secondary-300 dark:border-secondary-600 rounded-md focus:ring-2 focus:ring-primary-500 focus:border-transparent bg-white dark:bg-secondary-800 text-secondary-900 dark:text-white placeholder-secondary-500 dark:placeholder-secondary-400 text-sm sm:text-base"
						/>
					</div>
				</div>

				<div className="overflow-x-auto">
					{isLoadingHosts ? (
						<div className="flex items-center justify-center h-32">
							<RefreshCw className="h-6 w-6 animate-spin text-primary-600" />
						</div>
					) : hostsError ? (
						<div className="p-6">
							<div className="bg-danger-50 border border-danger-200 rounded-md p-4">
								<div className="flex">
									<AlertTriangle className="h-5 w-5 text-danger-400" />
									<div className="ml-3">
										<h3 className="text-sm font-medium text-danger-800">
											Error loading hosts
										</h3>
										<p className="text-sm text-danger-700 mt-1">
											{hostsError.message || "Failed to load hosts"}
										</p>
									</div>
								</div>
							</div>
						</div>
					) : filteredAndPaginatedHosts.length === 0 ? (
						<div className="text-center py-8">
							<Server className="h-12 w-12 text-secondary-400 mx-auto mb-4" />
							<p className="text-secondary-500 dark:text-secondary-300">
								{searchTerm
									? "No hosts match your search"
									: "No hosts have this package installed"}
							</p>
						</div>
					) : (
						<>
							{/* Mobile Card Layout */}
							<div className="md:hidden space-y-3 p-4">
								{filteredAndPaginatedHosts.map((host) => (
									// biome-ignore lint/a11y/useSemanticElements: Complex card layout requires div
									<div
										key={host.hostId}
										role="button"
										tabIndex={0}
										onClick={() => handleHostClick(host.hostId)}
										onKeyDown={(e) => {
											if (e.key === "Enter" || e.key === " ") {
												e.preventDefault();
												handleHostClick(host.hostId);
											}
										}}
										className="card p-4 space-y-3 cursor-pointer"
									>
										{/* Host Name */}
										<div className="flex items-center gap-3">
											<Server className="h-5 w-5 text-secondary-400 flex-shrink-0" />
											<div className="flex-1 min-w-0">
												<div className="text-base font-semibold text-secondary-900 dark:text-white truncate">
													{host.friendlyName || host.hostname}
												</div>
											</div>
										</div>

										{/* Status and Version */}
										<div className="flex items-center justify-between gap-3 pt-3 border-t border-secondary-200 dark:border-secondary-600">
											<div className="flex flex-col gap-2 flex-1">
												<div className="flex items-center gap-2">
													<span className="text-xs text-secondary-500 dark:text-secondary-400">
														Version:
													</span>
													<span className="text-sm text-secondary-900 dark:text-white font-mono">
														{host.currentVersion || "Unknown"}
													</span>
												</div>
												<div className="flex items-center gap-2">
													<span className="text-xs text-secondary-500 dark:text-secondary-400">
														Status:
													</span>
													{host.needsUpdate ? (
														host.isSecurityUpdate ? (
															<span className="badge-danger flex items-center gap-1 text-xs">
																<Shield className="h-3 w-3" />
																Security Update
															</span>
														) : (
															<span className="badge-warning text-xs">
																Update Available
															</span>
														)
													) : (
														<span className="badge-success text-xs">
															Up to Date
														</span>
													)}
												</div>
											</div>
											<div className="flex flex-col gap-2 items-end">
												{host.needsReboot && (
													<span className="inline-flex items-center gap-1 px-2 py-1 rounded-md text-xs font-medium bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200">
														<RotateCcw className="h-3 w-3" />
														Reboot Required
													</span>
												)}
												{host.lastUpdate && (
													<span className="text-xs text-secondary-500 dark:text-secondary-400">
														{formatRelativeTime(host.lastUpdate)}
													</span>
												)}
											</div>
										</div>
									</div>
								))}
							</div>

							{/* Desktop Table Layout */}
							<div className="hidden md:block">
								<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-600">
									<thead className="bg-secondary-50 dark:bg-secondary-700">
										<tr>
											<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
												Host
											</th>
											<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
												Current Version
											</th>
											<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
												Status
											</th>
											<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
												Last Updated
											</th>
											<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
												Reboot Required
											</th>
										</tr>
									</thead>
									<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-600">
										{filteredAndPaginatedHosts.map((host) => (
											<tr
												key={host.hostId}
												className="hover:bg-secondary-50 dark:hover:bg-secondary-700 cursor-pointer transition-colors"
												onClick={() => handleHostClick(host.hostId)}
											>
												<td className="px-6 py-4 whitespace-nowrap">
													<div className="flex items-center">
														<Server className="h-5 w-5 text-secondary-400 mr-3" />
														<div className="text-sm font-medium text-secondary-900 dark:text-white">
															{host.friendlyName || host.hostname}
														</div>
													</div>
												</td>
												<td className="px-6 py-4 whitespace-nowrap text-sm text-secondary-900 dark:text-white">
													{host.currentVersion || "Unknown"}
												</td>
												<td className="px-6 py-4 whitespace-nowrap">
													{host.needsUpdate ? (
														host.isSecurityUpdate ? (
															<span className="badge-danger flex items-center gap-1 w-fit">
																<Shield className="h-3 w-3" />
																Security Update
															</span>
														) : (
															<span className="badge-warning w-fit">
																Update Available
															</span>
														)
													) : (
														<span className="badge-success w-fit">
															Up to Date
														</span>
													)}
												</td>
												<td className="px-6 py-4 whitespace-nowrap text-sm text-secondary-500 dark:text-secondary-300">
													{host.lastUpdate
														? formatRelativeTime(host.lastUpdate)
														: "Never"}
												</td>
												<td className="px-6 py-4 whitespace-nowrap">
													{host.needsReboot ? (
														<span className="inline-flex items-center gap-1 px-2 py-1 rounded-md text-xs font-medium bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200">
															<RotateCcw className="h-3 w-3" />
															Required
														</span>
													) : (
														<span className="text-sm text-secondary-500 dark:text-secondary-300">
															No
														</span>
													)}
												</td>
											</tr>
										))}
									</tbody>
								</table>
							</div>

							{/* Pagination */}
							{totalPages > 1 && (
								<div className="px-4 sm:px-6 py-3 bg-white dark:bg-secondary-800 border-t border-secondary-200 dark:border-secondary-600 flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-3 sm:gap-0">
									<div className="flex items-center gap-2">
										<span className="text-xs sm:text-sm text-secondary-700 dark:text-secondary-300">
											Rows per page:
										</span>
										<select
											value={pageSize}
											onChange={(e) => {
												setPageSize(Number(e.target.value));
												setCurrentPage(1);
											}}
											className="text-xs sm:text-sm border border-secondary-300 dark:border-secondary-600 rounded px-2 py-1 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
										>
											<option value={25}>25</option>
											<option value={50}>50</option>
											<option value={100}>100</option>
										</select>
									</div>
									<div className="flex items-center justify-between sm:justify-end gap-2">
										<button
											type="button"
											onClick={() => setCurrentPage(currentPage - 1)}
											disabled={currentPage === 1}
											className="px-3 py-1 text-xs sm:text-sm border border-secondary-300 dark:border-secondary-600 rounded disabled:opacity-50 disabled:cursor-not-allowed hover:bg-secondary-50 dark:hover:bg-secondary-700"
										>
											Previous
										</button>
										<span className="text-xs sm:text-sm text-secondary-700 dark:text-secondary-300">
											Page {currentPage} of {totalPages}
										</span>
										<button
											type="button"
											onClick={() => setCurrentPage(currentPage + 1)}
											disabled={currentPage === totalPages}
											className="px-3 py-1 text-xs sm:text-sm border border-secondary-300 dark:border-secondary-600 rounded disabled:opacity-50 disabled:cursor-not-allowed hover:bg-secondary-50 dark:hover:bg-secondary-700"
										>
											Next
										</button>
									</div>
								</div>
							)}
						</>
					)}
				</div>
			</div>
		</div>
	);
};

export default PackageDetail;
