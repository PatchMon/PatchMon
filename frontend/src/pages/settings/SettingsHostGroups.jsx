import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, Edit, Plus, Server, Trash2 } from "lucide-react";
import { useEffect, useId, useState } from "react";
import { useNavigate } from "react-router-dom";
import SettingsLayout from "../../components/SettingsLayout";
import { hostGroupsAPI } from "../../utils/api";

const SettingsHostGroups = () => {
	const [showCreateModal, setShowCreateModal] = useState(false);
	const [showEditModal, setShowEditModal] = useState(false);
	const [selectedGroup, setSelectedGroup] = useState(null);
	const [showDeleteModal, setShowDeleteModal] = useState(false);
	const [groupToDelete, setGroupToDelete] = useState(null);

	const queryClient = useQueryClient();
	const navigate = useNavigate();

	// Fetch host groups
	const {
		data: hostGroups,
		isLoading,
		error,
	} = useQuery({
		queryKey: ["hostGroups"],
		queryFn: () => hostGroupsAPI.list().then((res) => res.data),
	});

	// Create host group mutation
	const createMutation = useMutation({
		mutationFn: (data) => hostGroupsAPI.create(data),
		onSuccess: () => {
			queryClient.invalidateQueries(["hostGroups"]);
			setShowCreateModal(false);
		},
		onError: (error) => {
			console.error("Failed to create host group:", error);
		},
	});

	// Update host group mutation
	const updateMutation = useMutation({
		mutationFn: ({ id, data }) => hostGroupsAPI.update(id, data),
		onSuccess: () => {
			queryClient.invalidateQueries(["hostGroups"]);
			setShowEditModal(false);
			setSelectedGroup(null);
		},
		onError: (error) => {
			console.error("Failed to update host group:", error);
		},
	});

	// Delete host group mutation
	const deleteMutation = useMutation({
		mutationFn: (id) => hostGroupsAPI.delete(id),
		onSuccess: () => {
			queryClient.invalidateQueries(["hostGroups"]);
			setShowDeleteModal(false);
			setGroupToDelete(null);
		},
		onError: (error) => {
			console.error("Failed to delete host group:", error);
		},
	});

	const handleCreate = (data) => {
		createMutation.mutate(data);
	};

	const handleEdit = (group) => {
		setSelectedGroup(group);
		setShowEditModal(true);
	};

	const handleUpdate = (data) => {
		updateMutation.mutate({ id: selectedGroup.id, data });
	};

	const _handleDeleteClick = (group) => {
		setGroupToDelete(group);
		setShowDeleteModal(true);
	};

	const handleDeleteConfirm = () => {
		deleteMutation.mutate(groupToDelete.id);
	};

	const handleHostsClick = (groupId) => {
		navigate(`/hosts?group=${groupId}`);
	};

	// Listen for delete modal trigger from edit modal
	useEffect(() => {
		const handleOpenDeleteModal = (event) => {
			setGroupToDelete(event.detail);
			setShowDeleteModal(true);
		};

		window.addEventListener("openDeleteHostGroupModal", handleOpenDeleteModal);
		return () =>
			window.removeEventListener(
				"openDeleteHostGroupModal",
				handleOpenDeleteModal,
			);
	}, []);

	return (
		<SettingsLayout>
			<div className="space-y-6">
				{/* Header */}
				<div className="flex justify-end items-center">
					<button
						type="button"
						onClick={() => setShowCreateModal(true)}
						className="btn-primary flex items-center gap-2 w-full sm:w-auto justify-center sm:justify-end"
						title="Create host group"
					>
						<Plus className="h-4 w-4" />
						Create Group
					</button>
				</div>

				{/* Host Groups Table */}
				<div className="bg-white dark:bg-secondary-800 shadow overflow-hidden sm:rounded-lg">
					{hostGroups && hostGroups.length > 0 ? (
						<>
							{/* Mobile Card Layout */}
							<div className="md:hidden space-y-3 p-4">
								{hostGroups.map((group) => (
									<div key={group.id} className="card p-4 space-y-3">
										{/* Group Name and Color */}
										<div className="flex items-center gap-3">
											<div
												className="w-4 h-4 rounded-full flex-shrink-0"
												style={{ backgroundColor: group.color }}
											/>
											<div className="flex-1 min-w-0">
												<div className="text-base font-semibold text-secondary-900 dark:text-white truncate">
													{group.name}
												</div>
											</div>
											<button
												type="button"
												onClick={() => handleEdit(group)}
												className="text-secondary-400 hover:text-secondary-600 dark:text-secondary-500 dark:hover:text-secondary-300 flex-shrink-0"
												title="Edit group"
											>
												<Edit className="h-4 w-4" />
											</button>
										</div>

										{/* Description */}
										{group.description && (
											<div className="text-sm text-secondary-500 dark:text-secondary-300">
												{group.description}
											</div>
										)}

										{/* Color and Hosts */}
										<div className="flex items-center justify-between gap-3 pt-2 border-t border-secondary-200 dark:border-secondary-600">
											<div className="flex items-center gap-2">
												<div
													className="w-6 h-6 rounded border border-secondary-300 dark:border-secondary-600 flex-shrink-0"
													style={{ backgroundColor: group.color }}
												/>
												<span className="text-xs text-secondary-500 dark:text-secondary-400 font-mono">
													{group.color}
												</span>
											</div>
											<button
												type="button"
												onClick={() => handleHostsClick(group.id)}
												className="flex items-center text-sm text-secondary-500 dark:text-secondary-300 hover:text-primary-600 dark:hover:text-primary-400 transition-colors"
												title={`View hosts in ${group.name}`}
											>
												<Server className="h-4 w-4 mr-1" />
												{group._count?.hosts || 0} host
												{group._count?.hosts !== 1 ? "s" : ""}
											</button>
										</div>
									</div>
								))}
							</div>

							{/* Desktop Table Layout */}
							<div className="hidden md:block overflow-x-auto">
								<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-600">
									<thead className="bg-secondary-50 dark:bg-secondary-700">
										<tr>
											<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
												Group
											</th>
											<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
												Description
											</th>
											<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
												Color
											</th>
											<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
												Hosts
											</th>
											<th className="px-6 py-3 text-right text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
												Actions
											</th>
										</tr>
									</thead>
									<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-600">
										{hostGroups.map((group) => (
											<tr
												key={group.id}
												className="hover:bg-secondary-50 dark:hover:bg-secondary-700"
											>
												<td className="px-6 py-4 whitespace-nowrap">
													<div className="flex items-center">
														<div
															className="w-3 h-3 rounded-full mr-3"
															style={{ backgroundColor: group.color }}
														/>
														<div className="text-sm font-medium text-secondary-900 dark:text-white">
															{group.name}
														</div>
													</div>
												</td>
												<td className="px-6 py-4">
													<div className="text-sm text-secondary-500 dark:text-secondary-300">
														{group.description || (
															<span className="text-secondary-400 italic">
																No description
															</span>
														)}
													</div>
												</td>
												<td className="px-6 py-4 whitespace-nowrap">
													<div className="flex items-center">
														<div
															className="w-6 h-6 rounded border border-secondary-300"
															style={{ backgroundColor: group.color }}
														/>
														<span className="ml-2 text-sm text-secondary-500 dark:text-secondary-300">
															{group.color}
														</span>
													</div>
												</td>
												<td className="px-6 py-4 whitespace-nowrap">
													<button
														type="button"
														onClick={() => handleHostsClick(group.id)}
														className="flex items-center text-sm text-secondary-500 dark:text-secondary-300 hover:text-primary-600 dark:hover:text-primary-400 transition-colors"
														title={`View hosts in ${group.name}`}
													>
														<Server className="h-4 w-4 mr-2" />
														{group._count?.hosts || 0} host
														{group._count?.hosts !== 1 ? "s" : ""}
													</button>
												</td>
												<td className="px-6 py-4 whitespace-nowrap text-right text-sm font-medium">
													<button
														type="button"
														onClick={() => handleEdit(group)}
														className="text-secondary-400 hover:text-secondary-600 dark:text-secondary-500 dark:hover:text-secondary-300"
														title="Edit group"
													>
														<Edit className="h-4 w-4" />
													</button>
												</td>
											</tr>
										))}
									</tbody>
								</table>
							</div>
						</>
					) : isLoading ? (
						<div className="p-12 text-center">
							<div className="flex items-center justify-center">
								<div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600"></div>
							</div>
						</div>
					) : error ? (
						<div className="p-6">
							<div className="bg-danger-50 dark:bg-danger-900 border border-danger-200 dark:border-danger-700 rounded-md p-4">
								<div className="flex">
									<AlertTriangle className="h-5 w-5 text-danger-400 dark:text-danger-300" />
									<div className="ml-3">
										<h3 className="text-sm font-medium text-danger-800 dark:text-danger-200">
											Error loading host groups
										</h3>
										<p className="text-sm text-danger-700 dark:text-danger-300 mt-1">
											{error.message || "Failed to load host groups"}
										</p>
									</div>
								</div>
							</div>
						</div>
					) : (
						<div className="p-12 text-center">
							<Server className="h-12 w-12 text-secondary-400 mx-auto mb-4" />
							<p className="text-secondary-500 dark:text-secondary-300">
								No host groups found
							</p>
							<p className="text-sm text-secondary-400 dark:text-secondary-400 mt-2">
								Click "Create Group" to create the first host group
							</p>
						</div>
					)}
				</div>

				{/* Create Modal */}
				{showCreateModal && (
					<CreateHostGroupModal
						onClose={() => setShowCreateModal(false)}
						onSubmit={handleCreate}
						isLoading={createMutation.isPending}
					/>
				)}

				{/* Edit Modal */}
				{showEditModal && selectedGroup && (
					<EditHostGroupModal
						group={selectedGroup}
						onClose={() => {
							setShowEditModal(false);
							setSelectedGroup(null);
						}}
						onSubmit={handleUpdate}
						isLoading={updateMutation.isPending}
					/>
				)}

				{/* Delete Confirmation Modal */}
				{showDeleteModal && groupToDelete && (
					<DeleteHostGroupModal
						group={groupToDelete}
						onClose={() => {
							setShowDeleteModal(false);
							setGroupToDelete(null);
						}}
						onConfirm={handleDeleteConfirm}
						isLoading={deleteMutation.isPending}
					/>
				)}
			</div>
		</SettingsLayout>
	);
};

// Create Host Group Modal
const CreateHostGroupModal = ({ onClose, onSubmit, isLoading }) => {
	const nameId = useId();
	const descriptionId = useId();
	const colorId = useId();
	const [formData, setFormData] = useState({
		name: "",
		description: "",
		color: "#3B82F6",
	});

	const handleSubmit = (e) => {
		e.preventDefault();
		onSubmit(formData);
	};

	const handleChange = (e) => {
		setFormData({
			...formData,
			[e.target.name]: e.target.value,
		});
	};

	return (
		<div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
			<div className="bg-white dark:bg-secondary-800 rounded-lg p-6 w-full max-w-md">
				<h3 className="text-lg font-semibold text-secondary-900 dark:text-white mb-4">
					Create Host Group
				</h3>

				<form onSubmit={handleSubmit} className="space-y-4">
					<div>
						<label
							htmlFor={nameId}
							className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
						>
							Name *
						</label>
						<input
							type="text"
							id={nameId}
							name="name"
							value={formData.name}
							onChange={handleChange}
							required
							className="w-full px-3 py-2 border border-secondary-300 dark:border-secondary-600 rounded-md focus:outline-none focus:ring-2 focus:ring-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white placeholder-secondary-500 dark:placeholder-secondary-400"
							placeholder="e.g., Production Servers"
						/>
					</div>

					<div>
						<label
							htmlFor={descriptionId}
							className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
						>
							Description
						</label>
						<textarea
							id={descriptionId}
							name="description"
							value={formData.description}
							onChange={handleChange}
							rows={3}
							className="w-full px-3 py-2 border border-secondary-300 dark:border-secondary-600 rounded-md focus:outline-none focus:ring-2 focus:ring-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white placeholder-secondary-500 dark:placeholder-secondary-400"
							placeholder="Optional description for this group"
						/>
					</div>

					<div>
						<label
							htmlFor={colorId}
							className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
						>
							Color
						</label>
						<div className="flex items-center gap-3">
							<input
								type="color"
								id={colorId}
								name="color"
								value={formData.color}
								onChange={handleChange}
								className="w-12 h-10 border border-secondary-300 rounded cursor-pointer"
							/>
							<input
								type="text"
								value={formData.color}
								onChange={handleChange}
								className="flex-1 px-3 py-2 border border-secondary-300 dark:border-secondary-600 rounded-md focus:outline-none focus:ring-2 focus:ring-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white placeholder-secondary-500 dark:placeholder-secondary-400"
								placeholder="#3B82F6"
							/>
						</div>
					</div>

					<div className="flex justify-end gap-3 pt-4">
						<button
							type="button"
							onClick={onClose}
							className="btn-outline"
							disabled={isLoading}
						>
							Cancel
						</button>
						<button type="submit" className="btn-primary" disabled={isLoading}>
							{isLoading ? "Creating..." : "Create Group"}
						</button>
					</div>
				</form>
			</div>
		</div>
	);
};

// Edit Host Group Modal
const EditHostGroupModal = ({ group, onClose, onSubmit, isLoading }) => {
	const editNameId = useId();
	const editDescriptionId = useId();
	const editColorId = useId();
	const [formData, setFormData] = useState({
		name: group.name,
		description: group.description || "",
		color: group.color || "#3B82F6",
	});

	const handleSubmit = (e) => {
		e.preventDefault();
		onSubmit(formData);
	};

	const handleChange = (e) => {
		setFormData({
			...formData,
			[e.target.name]: e.target.value,
		});
	};

	return (
		<div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
			<div className="bg-white dark:bg-secondary-800 rounded-lg p-6 w-full max-w-md">
				<h3 className="text-lg font-semibold text-secondary-900 dark:text-white mb-4">
					Edit Host Group
				</h3>

				<form onSubmit={handleSubmit} className="space-y-4">
					<div>
						<label
							htmlFor={editNameId}
							className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
						>
							Name *
						</label>
						<input
							type="text"
							id={editNameId}
							name="name"
							value={formData.name}
							onChange={handleChange}
							required
							className="w-full px-3 py-2 border border-secondary-300 dark:border-secondary-600 rounded-md focus:outline-none focus:ring-2 focus:ring-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white placeholder-secondary-500 dark:placeholder-secondary-400"
							placeholder="e.g., Production Servers"
						/>
					</div>

					<div>
						<label
							htmlFor={editDescriptionId}
							className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
						>
							Description
						</label>
						<textarea
							id={editDescriptionId}
							name="description"
							value={formData.description}
							onChange={handleChange}
							rows={3}
							className="w-full px-3 py-2 border border-secondary-300 dark:border-secondary-600 rounded-md focus:outline-none focus:ring-2 focus:ring-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white placeholder-secondary-500 dark:placeholder-secondary-400"
							placeholder="Optional description for this group"
						/>
					</div>

					<div>
						<label
							htmlFor={editColorId}
							className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
						>
							Color
						</label>
						<div className="flex items-center gap-3">
							<input
								type="color"
								id={editColorId}
								name="color"
								value={formData.color}
								onChange={handleChange}
								className="w-12 h-10 border border-secondary-300 rounded cursor-pointer"
							/>
							<input
								type="text"
								value={formData.color}
								onChange={handleChange}
								className="flex-1 px-3 py-2 border border-secondary-300 dark:border-secondary-600 rounded-md focus:outline-none focus:ring-2 focus:ring-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white placeholder-secondary-500 dark:placeholder-secondary-400"
								placeholder="#3B82F6"
							/>
						</div>
					</div>

					<div className="flex justify-between pt-4">
						<button
							type="button"
							onClick={() => {
								onClose();
								// Trigger delete modal
								window.dispatchEvent(
									new CustomEvent("openDeleteHostGroupModal", {
										detail: group,
									}),
								);
							}}
							className="btn-danger"
							disabled={isLoading}
						>
							<Trash2 className="h-4 w-4 mr-2" />
							Delete Group
						</button>
						<div className="flex gap-3">
							<button
								type="button"
								onClick={onClose}
								className="btn-outline"
								disabled={isLoading}
							>
								Cancel
							</button>
							<button
								type="submit"
								className="btn-primary"
								disabled={isLoading}
							>
								{isLoading ? "Updating..." : "Update Group"}
							</button>
						</div>
					</div>
				</form>
			</div>
		</div>
	);
};

// Delete Confirmation Modal
const DeleteHostGroupModal = ({ group, onClose, onConfirm, isLoading }) => {
	// Fetch hosts for this group
	const { data: hostsData } = useQuery({
		queryKey: ["hostGroupHosts", group?.id],
		queryFn: () => hostGroupsAPI.getHosts(group.id).then((res) => res.data),
		enabled: !!group && group._count?.hosts > 0,
	});

	const hosts = hostsData || [];

	return (
		<div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
			<div className="bg-white dark:bg-secondary-800 rounded-lg p-6 w-full max-w-md max-h-[90vh] overflow-y-auto">
				<div className="flex items-center gap-3 mb-4">
					<div className="w-10 h-10 bg-danger-100 rounded-full flex items-center justify-center">
						<AlertTriangle className="h-5 w-5 text-danger-600" />
					</div>
					<div>
						<h3 className="text-lg font-semibold text-secondary-900 dark:text-white">
							Delete Host Group
						</h3>
						<p className="text-sm text-secondary-600 dark:text-secondary-300">
							This action cannot be undone
						</p>
					</div>
				</div>

				<div className="mb-6">
					<p className="text-secondary-700 dark:text-secondary-200">
						Are you sure you want to delete the host group{" "}
						<span className="font-semibold">"{group.name}"</span>?
					</p>
					{group._count?.hosts > 0 && (
						<div className="mt-3 p-3 bg-blue-50 border border-blue-200 rounded-md">
							<p className="text-sm text-blue-800 mb-2">
								<strong>Note:</strong> This group contains {group._count?.hosts}{" "}
								host
								{group._count?.hosts !== 1 ? "s" : ""}. These hosts will be
								moved to "No group" after deletion.
							</p>
							{hosts.length > 0 && (
								<div className="mt-2">
									<p className="text-xs font-medium text-blue-900 mb-1">
										Hosts in this group:
									</p>
									<div className="max-h-32 overflow-y-auto bg-blue-100 rounded p-2">
										{hosts.map((host) => (
											<div
												key={host.id}
												className="text-xs text-blue-900 flex items-center gap-1"
											>
												<Server className="h-3 w-3" />
												{host.friendly_name || host.hostname}
											</div>
										))}
									</div>
								</div>
							)}
						</div>
					)}
				</div>

				<div className="flex justify-end gap-3">
					<button
						type="button"
						onClick={onClose}
						className="btn-outline"
						disabled={isLoading}
					>
						Cancel
					</button>
					<button
						type="button"
						onClick={onConfirm}
						className="btn-danger"
						disabled={isLoading}
					>
						{isLoading ? "Deleting..." : "Delete Group"}
					</button>
				</div>
			</div>
		</div>
	);
};

export default SettingsHostGroups;
