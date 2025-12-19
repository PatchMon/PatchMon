import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
	AlertTriangle,
	ArrowLeft,
	Calendar,
	Database,
	Globe,
	Lock,
	RotateCcw,
	Search,
	Server,
	Shield,
	ShieldOff,
	Trash2,
	Unlock,
} from "lucide-react";

import { useId, useMemo, useState } from "react";

import { Link, useNavigate, useParams } from "react-router-dom";
import { formatRelativeTime, repositoryAPI } from "../utils/api";

const RepositoryDetail = () => {
	const isActiveId = useId();
	const repositoryNameId = useId();
	const priorityId = useId();
	const descriptionId = useId();
	const { repositoryId } = useParams();
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const [editMode, setEditMode] = useState(false);
	const [formData, setFormData] = useState({});
	const [searchTerm, setSearchTerm] = useState("");
	const [currentPage, setCurrentPage] = useState(1);
	const [pageSize, setPageSize] = useState(25);
	const [showDeleteModal, setShowDeleteModal] = useState(false);

	// Fetch repository details
	const {
		data: repository,
		isLoading,
		error,
	} = useQuery({
		queryKey: ["repository", repositoryId],
		queryFn: () => repositoryAPI.getById(repositoryId).then((res) => res.data),
		enabled: !!repositoryId,
	});

	const hosts = repository?.host_repositories || [];

	// Filter and paginate hosts
	const filteredAndPaginatedHosts = useMemo(() => {
		let filtered = hosts;

		if (searchTerm) {
			filtered = hosts.filter(
				(hostRepo) =>
					hostRepo.hosts.friendly_name
						?.toLowerCase()
						.includes(searchTerm.toLowerCase()) ||
					hostRepo.hosts.hostname
						?.toLowerCase()
						.includes(searchTerm.toLowerCase()) ||
					hostRepo.hosts.ip?.toLowerCase().includes(searchTerm.toLowerCase()),
			);
		}

		const startIndex = (currentPage - 1) * pageSize;
		const endIndex = startIndex + pageSize;
		return filtered.slice(startIndex, endIndex);
	}, [hosts, searchTerm, currentPage, pageSize]);

	const totalPages = Math.ceil(
		(searchTerm
			? hosts.filter(
					(hostRepo) =>
						hostRepo.hosts.friendly_name
							?.toLowerCase()
							.includes(searchTerm.toLowerCase()) ||
						hostRepo.hosts.hostname
							?.toLowerCase()
							.includes(searchTerm.toLowerCase()) ||
						hostRepo.hosts.ip?.toLowerCase().includes(searchTerm.toLowerCase()),
				).length
			: hosts.length) / pageSize,
	);

	const handleHostClick = (hostId) => {
		navigate(`/hosts/${hostId}`);
	};

	// Update repository mutation
	const updateRepositoryMutation = useMutation({
		mutationFn: (data) => repositoryAPI.update(repositoryId, data),
		onSuccess: () => {
			queryClient.invalidateQueries(["repository", repositoryId]);
			queryClient.invalidateQueries(["repositories"]);
			setEditMode(false);
		},
	});

	// Delete repository mutation
	const deleteRepositoryMutation = useMutation({
		mutationFn: () => repositoryAPI.delete(repositoryId),
		onSuccess: () => {
			queryClient.invalidateQueries(["repositories"]);
			navigate("/repositories");
		},
	});

	const handleEdit = () => {
		setFormData({
			name: repository.name,
			description: repository.description || "",
			is_active: repository.is_active,
			priority: repository.priority || "",
		});
		setEditMode(true);
	};

	const handleSave = () => {
		updateRepositoryMutation.mutate(formData);
	};

	const handleCancel = () => {
		setEditMode(false);
		setFormData({});
	};

	const handleDelete = () => {
		setShowDeleteModal(true);
	};

	const confirmDelete = () => {
		deleteRepositoryMutation.mutate();
		setShowDeleteModal(false);
	};

	const cancelDelete = () => {
		setShowDeleteModal(false);
	};

	if (isLoading) {
		return (
			<div className="flex items-center justify-center h-64">
				<div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600"></div>
			</div>
		);
	}

	if (error) {
		return (
			<div className="space-y-6">
				<div className="flex items-center gap-4">
					<Link
						to="/repositories"
						className="btn-outline flex items-center gap-2"
					>
						<ArrowLeft className="h-4 w-4" />
						Back to Repositories
					</Link>
				</div>
				<div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-4">
					<div className="flex items-center">
						<AlertTriangle className="h-5 w-5 text-red-400 mr-2" />
						<span className="text-red-700 dark:text-red-300">
							Failed to load repository: {error.message}
						</span>
					</div>
				</div>
			</div>
		);
	}

	if (!repository) {
		return (
			<div className="space-y-6">
				<div className="flex items-center gap-4">
					<Link
						to="/repositories"
						className="btn-outline flex items-center gap-2"
					>
						<ArrowLeft className="h-4 w-4" />
						Back to Repositories
					</Link>
				</div>
				<div className="text-center py-12">
					<Database className="mx-auto h-12 w-12 text-secondary-400" />
					<h3 className="mt-2 text-sm font-medium text-secondary-900 dark:text-white">
						Repository not found
					</h3>
					<p className="mt-1 text-sm text-secondary-500 dark:text-secondary-300">
						The repository you're looking for doesn't exist.
					</p>
				</div>
			</div>
		);
	}

	return (
		<div className="space-y-6">
			{/* Delete Confirmation Modal */}
			{showDeleteModal && (
				<div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
					<div className="bg-white dark:bg-secondary-800 rounded-lg p-6 max-w-md w-full mx-4">
						<div className="flex items-center mb-4">
							<AlertTriangle className="h-6 w-6 text-red-500 mr-3" />
							<h3 className="text-lg font-semibold text-secondary-900 dark:text-white">
								Delete Repository
							</h3>
						</div>
						<div className="mb-6">
							<p className="text-secondary-700 dark:text-secondary-300 mb-2">
								Are you sure you want to delete{" "}
								<strong>"{repository?.name}"</strong>?
							</p>
							{repository?.host_repositories?.length > 0 && (
								<p className="text-amber-600 dark:text-amber-400 text-sm">
									⚠️ This repository is currently assigned to{" "}
									{repository.host_repositories.length} host
									{repository.host_repositories.length !== 1 ? "s" : ""}.
								</p>
							)}
							<p className="text-red-600 dark:text-red-400 text-sm mt-2">
								This action cannot be undone.
							</p>
						</div>
						<div className="flex gap-3 justify-end">
							<button
								type="button"
								onClick={cancelDelete}
								className="px-4 py-2 text-secondary-600 dark:text-secondary-400 hover:text-secondary-800 dark:hover:text-secondary-200 transition-colors"
								disabled={deleteRepositoryMutation.isPending}
							>
								Cancel
							</button>
							<button
								type="button"
								onClick={confirmDelete}
								className="px-4 py-2 bg-red-600 text-white rounded-md hover:bg-red-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
								disabled={deleteRepositoryMutation.isPending}
							>
								{deleteRepositoryMutation.isPending
									? "Deleting..."
									: "Delete Repository"}
							</button>
						</div>
					</div>
				</div>
			)}

			{/* Header */}
			<div className="flex items-center justify-between">
				<div className="flex items-center gap-4">
					<Link
						to="/repositories"
						className="btn-outline flex items-center gap-2"
					>
						<ArrowLeft className="h-4 w-4" />
						Back
					</Link>
					<div>
						<div className="flex items-center gap-3">
							{repository.isSecure ? (
								<Lock className="h-6 w-6 text-green-600" />
							) : (
								<Unlock className="h-6 w-6 text-orange-600" />
							)}
							<h1 className="text-2xl font-bold text-secondary-900 dark:text-white">
								{repository.name}
							</h1>
							<span
								className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${
									repository.is_active
										? "bg-green-100 dark:bg-green-900/20 text-green-800 dark:text-green-300"
										: "bg-red-100 dark:bg-red-900/20 text-red-800 dark:text-red-300"
								}`}
							>
								{repository.is_active ? "Active" : "Inactive"}
							</span>
						</div>
					</div>
				</div>
				<div className="flex items-center gap-2">
					{editMode ? (
						<>
							<button
								type="button"
								onClick={handleCancel}
								className="btn-outline"
								disabled={updateRepositoryMutation.isPending}
							>
								Cancel
							</button>
							<button
								type="button"
								onClick={handleSave}
								className="btn-primary"
								disabled={updateRepositoryMutation.isPending}
							>
								{updateRepositoryMutation.isPending
									? "Saving..."
									: "Save Changes"}
							</button>
						</>
					) : (
						<>
							<button
								type="button"
								onClick={handleDelete}
								className="btn-outline border-red-200 text-red-600 hover:bg-red-50 hover:border-red-300 dark:border-red-800 dark:text-red-400 dark:hover:bg-red-900/20 dark:hover:border-red-700 flex items-center gap-2"
								disabled={deleteRepositoryMutation.isPending}
							>
								<Trash2 className="h-4 w-4" />
								{deleteRepositoryMutation.isPending ? "Deleting..." : "Delete"}
							</button>
							<button
								type="button"
								onClick={handleEdit}
								className="btn-primary"
							>
								Edit Repository
							</button>
						</>
					)}
				</div>
			</div>

			{/* Repository Information */}
			<div className="card">
				<div className="px-6 py-4 border-b border-secondary-200 dark:border-secondary-700">
					<h2 className="text-lg font-semibold text-secondary-900 dark:text-white">
						Repository Information
					</h2>
				</div>
				<div className="px-6 py-4 space-y-4">
					{editMode ? (
						<div className="grid grid-cols-1 md:grid-cols-2 gap-6">
							<div>
								<label
									htmlFor={repositoryNameId}
									className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-1"
								>
									Repository Name
								</label>
								<input
									type="text"
									id={repositoryNameId}
									value={formData.name}
									onChange={(e) =>
										setFormData({ ...formData, name: e.target.value })
									}
									className="w-full px-3 py-2 border border-secondary-300 dark:border-secondary-600 rounded-md focus:outline-none focus:ring-2 focus:ring-primary-500 dark:bg-secondary-700 dark:text-white"
								/>
							</div>
							<div>
								<label
									htmlFor={priorityId}
									className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-1"
								>
									Priority
								</label>
								<input
									type="number"
									id={priorityId}
									value={formData.priority}
									onChange={(e) =>
										setFormData({ ...formData, priority: e.target.value })
									}
									className="w-full px-3 py-2 border border-secondary-300 dark:border-secondary-600 rounded-md focus:outline-none focus:ring-2 focus:ring-primary-500 dark:bg-secondary-700 dark:text-white"
									placeholder="Optional priority"
								/>
							</div>
							<div className="md:col-span-2">
								<label
									htmlFor={descriptionId}
									className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-1"
								>
									Description
								</label>
								<textarea
									id={descriptionId}
									value={formData.description}
									onChange={(e) =>
										setFormData({ ...formData, description: e.target.value })
									}
									rows="3"
									className="w-full px-3 py-2 border border-secondary-300 dark:border-secondary-600 rounded-md focus:outline-none focus:ring-2 focus:ring-primary-500 dark:bg-secondary-700 dark:text-white"
									placeholder="Optional description"
								/>
							</div>
							<div className="flex items-center">
								<input
									type="checkbox"
									id={isActiveId}
									checked={formData.is_active}
									onChange={(e) =>
										setFormData({ ...formData, is_active: e.target.checked })
									}
									className="h-4 w-4 text-primary-600 focus:ring-primary-500 border-secondary-300 rounded"
								/>
								<label
									htmlFor={isActiveId}
									className="ml-2 block text-sm text-secondary-900 dark:text-white"
								>
									Repository is active
								</label>
							</div>
						</div>
					) : (
						<div className="grid grid-cols-1 md:grid-cols-2 gap-6">
							<div className="space-y-4">
								<div>
									<span className="text-sm font-medium text-secondary-500 dark:text-secondary-400">
										URL
									</span>
									<div className="flex items-center mt-1">
										<Globe className="h-4 w-4 text-secondary-400 mr-2" />
										<span className="text-secondary-900 dark:text-white">
											{repository.url}
										</span>
									</div>
								</div>
								<div>
									<span className="text-sm font-medium text-secondary-500 dark:text-secondary-400">
										Distribution
									</span>
									<p className="text-secondary-900 dark:text-white mt-1">
										{repository.distribution}
									</p>
								</div>
								<div>
									<span className="text-sm font-medium text-secondary-500 dark:text-secondary-400">
										Components
									</span>
									<p className="text-secondary-900 dark:text-white mt-1">
										{repository.components}
									</p>
								</div>
								<div>
									<span className="text-sm font-medium text-secondary-500 dark:text-secondary-400">
										Repository Type
									</span>
									<p className="text-secondary-900 dark:text-white mt-1">
										{repository.repoType}
									</p>
								</div>
							</div>
							<div className="space-y-4">
								<div>
									<span className="text-sm font-medium text-secondary-500 dark:text-secondary-400">
										Security
									</span>
									<div className="flex items-center mt-1">
										{repository.isSecure ? (
											<>
												<Shield className="h-4 w-4 text-green-600 mr-2" />
												<span className="text-green-600">Secure (HTTPS)</span>
											</>
										) : (
											<>
												<ShieldOff className="h-4 w-4 text-orange-600 mr-2" />
												<span className="text-orange-600">Insecure (HTTP)</span>
											</>
										)}
									</div>
								</div>
								{repository.priority && (
									<div>
										<span className="text-sm font-medium text-secondary-500 dark:text-secondary-400">
											Priority
										</span>
										<p className="text-secondary-900 dark:text-white mt-1">
											{repository.priority}
										</p>
									</div>
								)}
								{repository.description && (
									<div>
										<span className="text-sm font-medium text-secondary-500 dark:text-secondary-400">
											Description
										</span>
										<p className="text-secondary-900 dark:text-white mt-1">
											{repository.description}
										</p>
									</div>
								)}
								<div>
									<span className="text-sm font-medium text-secondary-500 dark:text-secondary-400">
										Created
									</span>
									<div className="flex items-center mt-1">
										<Calendar className="h-4 w-4 text-secondary-400 mr-2" />
										<span className="text-secondary-900 dark:text-white">
											{new Date(repository.created_at).toLocaleDateString()}
										</span>
									</div>
								</div>
							</div>
						</div>
					)}
				</div>
			</div>

			{/* Hosts Using This Repository */}
			<div className="card">
				<div className="px-6 py-4 border-b border-secondary-200 dark:border-secondary-600">
					<div className="flex items-center justify-between mb-4">
						<div className="flex items-center gap-3">
							<Server className="h-5 w-5 text-primary-600" />
							<h3 className="text-lg font-medium text-secondary-900 dark:text-white">
								Hosts Using This Repository ({hosts.length})
							</h3>
						</div>
					</div>

					{/* Search */}
					<div className="relative max-w-sm">
						<Search className="absolute left-3 top-1/2 transform -translate-y-1/2 h-4 w-4 text-secondary-400" />
						<input
							type="text"
							placeholder="Search hosts..."
							value={searchTerm}
							onChange={(e) => {
								setSearchTerm(e.target.value);
								setCurrentPage(1);
							}}
							className="w-full pl-10 pr-4 py-2 border border-secondary-300 dark:border-secondary-600 rounded-md focus:ring-2 focus:ring-primary-500 focus:border-transparent bg-white dark:bg-secondary-800 text-secondary-900 dark:text-white placeholder-secondary-500 dark:placeholder-secondary-400"
						/>
					</div>
				</div>

				<div className="overflow-x-auto">
					{filteredAndPaginatedHosts.length === 0 ? (
						<div className="text-center py-8">
							<Server className="h-12 w-12 text-secondary-400 mx-auto mb-4" />
							<p className="text-secondary-500 dark:text-secondary-300">
								{searchTerm
									? "No hosts match your search"
									: "This repository hasn't been reported by any hosts yet."}
							</p>
						</div>
					) : (
						<>
							<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-600">
								<thead className="bg-secondary-50 dark:bg-secondary-700">
									<tr>
										<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
											Host
										</th>
										<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
											Operating System
										</th>
										<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
											Last Checked
										</th>
										<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
											Last Update
										</th>
										<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
											Reboot Required
										</th>
									</tr>
								</thead>
								<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-600">
									{filteredAndPaginatedHosts.map((hostRepo) => (
										<tr
											key={hostRepo.id}
											className="hover:bg-secondary-50 dark:hover:bg-secondary-700 cursor-pointer transition-colors"
											onClick={() => handleHostClick(hostRepo.hosts.id)}
										>
											<td className="px-6 py-4 whitespace-nowrap">
												<div className="flex items-center">
													<div
														className={`w-2 h-2 rounded-full mr-3 ${
															hostRepo.hosts.status === "active"
																? "bg-success-500"
																: hostRepo.hosts.status === "pending"
																	? "bg-warning-500"
																	: "bg-danger-500"
														}`}
													/>
													<Server className="h-5 w-5 text-secondary-400 mr-3" />
													<div>
														<div className="text-sm font-medium text-secondary-900 dark:text-white">
															{hostRepo.hosts.friendly_name ||
																hostRepo.hosts.hostname}
														</div>
														{hostRepo.hosts.friendly_name &&
															hostRepo.hosts.hostname && (
																<div className="text-sm text-secondary-500 dark:text-secondary-300">
																	{hostRepo.hosts.hostname}
																</div>
															)}
													</div>
												</div>
											</td>
											<td className="px-6 py-4 whitespace-nowrap text-sm text-secondary-900 dark:text-white">
												{hostRepo.hosts.os_type} {hostRepo.hosts.os_version}
											</td>
											<td className="px-6 py-4 whitespace-nowrap text-sm text-secondary-500 dark:text-secondary-300">
												{hostRepo.last_checked
													? formatRelativeTime(hostRepo.last_checked)
													: "Never"}
											</td>
											<td className="px-6 py-4 whitespace-nowrap text-sm text-secondary-500 dark:text-secondary-300">
												{hostRepo.hosts.last_update
													? formatRelativeTime(hostRepo.hosts.last_update)
													: "Never"}
											</td>
											<td className="px-6 py-4 whitespace-nowrap">
												{hostRepo.hosts.needs_reboot ? (
													<span
														className="inline-flex items-center gap-1 px-2 py-1 rounded-md text-xs font-medium bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200"
														title={
															hostRepo.hosts.reboot_reason || "Reboot required"
														}
													>
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

							{/* Pagination */}
							{totalPages > 1 && (
								<div className="px-6 py-3 bg-white dark:bg-secondary-800 border-t border-secondary-200 dark:border-secondary-600 flex items-center justify-between">
									<div className="flex items-center gap-2">
										<span className="text-sm text-secondary-700 dark:text-secondary-300">
											Rows per page:
										</span>
										<select
											value={pageSize}
											onChange={(e) => {
												setPageSize(Number(e.target.value));
												setCurrentPage(1);
											}}
											className="text-sm border border-secondary-300 dark:border-secondary-600 rounded px-2 py-1 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
										>
											<option value={25}>25</option>
											<option value={50}>50</option>
											<option value={100}>100</option>
										</select>
									</div>
									<div className="flex items-center gap-2">
										<button
											type="button"
											onClick={() => setCurrentPage(currentPage - 1)}
											disabled={currentPage === 1}
											className="px-3 py-1 text-sm border border-secondary-300 dark:border-secondary-600 rounded disabled:opacity-50 disabled:cursor-not-allowed hover:bg-secondary-50 dark:hover:bg-secondary-700"
										>
											Previous
										</button>
										<span className="text-sm text-secondary-700 dark:text-secondary-300">
											Page {currentPage} of {totalPages}
										</span>
										<button
											type="button"
											onClick={() => setCurrentPage(currentPage + 1)}
											disabled={currentPage === totalPages}
											className="px-3 py-1 text-sm border border-secondary-300 dark:border-secondary-600 rounded disabled:opacity-50 disabled:cursor-not-allowed hover:bg-secondary-50 dark:hover:bg-secondary-700"
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

export default RepositoryDetail;
