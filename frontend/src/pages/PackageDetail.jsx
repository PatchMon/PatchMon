import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
	AlertTriangle,
	ArrowLeft,
	Calendar,
	ChevronRight,
	Download,
	Info,
	Package,
	RefreshCw,
	RotateCcw,
	Search,
	Server,
	Shield,
	Wrench,
} from "lucide-react";
import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import PatchConfirmModal from "../components/PatchConfirmModal";
import { useAuth } from "../contexts/AuthContext";
import { useToast } from "../contexts/ToastContext";
import { formatRelativeTime, packagesAPI } from "../utils/api";
import { patchingAPI } from "../utils/patchingApi";

const PackageDetail = () => {
	const { packageId } = useParams();
	const decodedPackageId = decodeURIComponent(packageId || "");
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const toast = useToast();
	const { canManageHosts } = useAuth();
	const [searchTerm, setSearchTerm] = useState("");
	const [currentPage, setCurrentPage] = useState(1);
	const [pageSize, setPageSize] = useState(25);
	const [patchConfirmTarget, setPatchConfirmTarget] = useState(null); // { hostId, hostName, packageName }

	const patchPackageMutation = useMutation({
		mutationFn: ({ hostId, packageName }) =>
			patchingAPI.trigger(hostId, "patch_package", packageName),
		onSuccess: (_, { hostName }) => {
			setPatchConfirmTarget(null);
			toast.success(`Patch queued for ${hostName || "host"}`);
			queryClient.invalidateQueries(["package", decodedPackageId]);
			queryClient.invalidateQueries(["package-hosts", decodedPackageId]);
			queryClient.invalidateQueries(["patching-dashboard"]);
		},
		onError: (err, { hostName }) => {
			const msg = err.response?.data?.error || err.message;
			toast.error(`Patch failed for ${hostName || "host"}: ${msg}`);
		},
	});

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

	// Fetch hosts that have this package (backend filters by search, paginates)
	const {
		data: hostsData,
		isLoading: isLoadingHosts,
		error: hostsError,
		refetch: refetchHosts,
	} = useQuery({
		queryKey: [
			"package-hosts",
			decodedPackageId,
			searchTerm,
			currentPage,
			pageSize,
		],
		queryFn: () =>
			packagesAPI
				.getHosts(decodedPackageId, {
					search: searchTerm,
					page: currentPage,
					limit: pageSize,
				})
				.then((res) => res.data),
		staleTime: 5 * 60 * 1000,
		refetchOnWindowFocus: false,
		enabled: !!decodedPackageId,
	});

	const hosts = hostsData?.hosts || [];
	const pagination = hostsData?.pagination || {};
	const _totalFromBackend = pagination.total ?? 0;
	const totalPages = pagination.pages ?? 1;

	// Backend returns paginated, filtered results - use directly (no client-side filter/paginate)
	const filteredAndPaginatedHosts = hosts;

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
					<p className="text-secondary-500 dark:text-white">
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
						className="flex items-center gap-2 text-secondary-600 hover:text-secondary-900 dark:text-white dark:hover:text-white transition-colors text-sm sm:text-base"
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
							<p className="text-sm text-secondary-500 dark:text-white">
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
							<p className="text-sm text-secondary-500 dark:text-white">
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
							<p className="text-sm text-secondary-500 dark:text-white">
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
							<p className="text-sm text-secondary-500 dark:text-white">
								Up to Date
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{(stats.totalInstalls || 0) - (stats.updatesNeeded || 0)}
							</p>
						</div>
					</div>
				</div>
			</div>

			{/* Description */}
			<div className="card p-4">
				<h4 className="text-sm font-medium text-secondary-600 dark:text-white mb-3 flex items-center gap-2">
					Description
					<div className="relative group">
						<Info className="dark:text-white" />
						<div className="absolute left-full top-1/2 -translate-y-1/2 ml-2 hidden group-hover:block w-max max-w-xs px-2 py-1 bg-gray-900 text-white text-xs rounded shadow-lg z-[100]">
							The description was pulled directly from the host package manager.
						</div>
					</div>
				</h4>
				<p className="text-sm text-secondary-600 dark:text-white dark:text-white">
					{pkg.description || "No description available."}
				</p>
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
							<p className="text-secondary-500 dark:text-white">
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
													<span className="text-xs text-secondary-500 dark:text-white">
														Version:
													</span>
													<span className="text-sm text-secondary-900 dark:text-white font-mono">
														{host.currentVersion || "Unknown"}
													</span>
												</div>
												<div className="flex items-center gap-2">
													<span className="text-xs text-secondary-500 dark:text-white">
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
												{host.needsUpdate &&
													canManageHosts() &&
													!(host.osType || host.os_type || "")
														.toLowerCase()
														.includes("windows") && (
														<button
															type="button"
															onClick={(e) => {
																e.stopPropagation();
																setPatchConfirmTarget({
																	hostId: host.hostId,
																	hostName: host.friendlyName || host.hostname,
																	packageName: pkg.name,
																});
															}}
															disabled={patchPackageMutation.isPending}
															className="inline-flex items-center gap-1 px-2 py-1 text-xs font-medium rounded bg-primary-100 text-primary-800 hover:bg-primary-200 dark:bg-primary-900 dark:text-primary-200 dark:hover:bg-primary-800 disabled:opacity-50"
														>
															<Wrench className="h-3 w-3" />
															{patchPackageMutation.isPending
																? "Queuing…"
																: "Patch"}
														</button>
													)}
												{host.needsReboot && (
													<span
														className="inline-flex items-center gap-1 px-2 py-1 rounded-md text-xs font-medium bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200"
														title={host.rebootReason || "Reboot required"}
													>
														<RotateCcw className="h-3 w-3" />
														Reboot Required
													</span>
												)}
												{host.lastUpdate && (
													<span className="text-xs text-secondary-500 dark:text-white">
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
											<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider">
												Host
											</th>
											<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider">
												Current Version
											</th>
											<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider">
												Status
											</th>
											<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider">
												Last Updated
											</th>
											<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider">
												Reboot Required
											</th>
											{canManageHosts() && (
												<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider">
													Actions
												</th>
											)}
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
												<td className="px-6 py-4 whitespace-nowrap text-sm text-secondary-500 dark:text-white">
													{host.lastUpdate
														? formatRelativeTime(host.lastUpdate)
														: "Never"}
												</td>
												<td className="px-6 py-4 whitespace-nowrap">
													{host.needsReboot ? (
														<span
															className="inline-flex items-center gap-1 px-2 py-1 rounded-md text-xs font-medium bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200"
															title={host.rebootReason || "Reboot required"}
														>
															<RotateCcw className="h-3 w-3" />
															Required
														</span>
													) : (
														<span className="text-sm text-secondary-500 dark:text-white">
															No
														</span>
													)}
												</td>
												{canManageHosts() && (
													<td
														className="px-6 py-4 whitespace-nowrap"
														onClick={(e) => e.stopPropagation()}
													>
														{host.needsUpdate &&
														!(host.osType || host.os_type || "")
															.toLowerCase()
															.includes("windows") ? (
															<button
																type="button"
																onClick={() =>
																	setPatchConfirmTarget({
																		hostId: host.hostId,
																		hostName:
																			host.friendlyName || host.hostname,
																		packageName: pkg.name,
																	})
																}
																disabled={patchPackageMutation.isPending}
																className="inline-flex items-center gap-1 px-2 py-1 text-xs font-medium rounded bg-primary-100 text-primary-800 hover:bg-primary-200 dark:bg-primary-900 dark:text-primary-200 dark:hover:bg-primary-800 disabled:opacity-50"
															>
																<Wrench className="h-3 w-3" />
																{patchPackageMutation.isPending
																	? "Queuing…"
																	: "Patch"}
															</button>
														) : (
															<span className="text-sm text-secondary-500 dark:text-white">
																—
															</span>
														)}
													</td>
												)}
											</tr>
										))}
									</tbody>
								</table>
							</div>

							{/* Pagination */}
							{totalPages > 1 && (
								<div className="px-4 sm:px-6 py-3 bg-white dark:bg-secondary-800 border-t border-secondary-200 dark:border-secondary-600 flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-3 sm:gap-0">
									<div className="flex items-center gap-2">
										<span className="text-xs sm:text-sm text-secondary-700 dark:text-white">
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
										<span className="text-xs sm:text-sm text-secondary-700 dark:text-white">
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

			{/* Patch Confirmation Modal */}
			{patchConfirmTarget && (
				<PatchConfirmModal
					isOpen={!!patchConfirmTarget}
					onClose={() => setPatchConfirmTarget(null)}
					onConfirm={() =>
						patchPackageMutation.mutate({
							hostId: patchConfirmTarget.hostId,
							packageName: patchConfirmTarget.packageName,
							hostName: patchConfirmTarget.hostName,
						})
					}
					isPending={patchPackageMutation.isPending}
					hostId={patchConfirmTarget.hostId}
					patchType="patch_package"
					packageName={patchConfirmTarget.packageName}
					hostDisplayName={patchConfirmTarget.hostName}
				/>
			)}
		</div>
	);
};

export default PackageDetail;
