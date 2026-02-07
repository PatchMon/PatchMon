import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
	AlertTriangle,
	Edit,
	Plus,
	Server,
	Settings,
	Trash2,
	Users,
} from "lucide-react";
import { useId, useState } from "react";
import { hostGroupsAPI } from "../utils/api";

const Options = () => {
	const [activeTab, setActiveTab] = useState("hostgroups");
	const [showCreateModal, setShowCreateModal] = useState(false);
	const [showEditModal, setShowEditModal] = useState(false);
	const [selectedGroup, setSelectedGroup] = useState(null);
	const [showDeleteModal, setShowDeleteModal] = useState(false);
	const [groupToDelete, setGroupToDelete] = useState(null);

	const queryClient = useQueryClient();

	// Tab configuration
	const tabs = [
		{ id: "hostgroups", name: "Host Groups", icon: Users },
		{
			id: "notifications",
			name: "Notifications",
			icon: AlertTriangle,
			comingSoon: true,
		},
	];

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

	const handleDeleteClick = (group) => {
		setGroupToDelete(group);
		setShowDeleteModal(true);
	};

	const handleDeleteConfirm = () => {
		deleteMutation.mutate(groupToDelete.id);
	};

	const renderHostGroupsTab = () => {
		if (isLoading) {
			return (
				<div className="flex items-center justify-center h-64">
					<div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600"></div>
				</div>
			);
		}

		if (error) {
			return (
				<div className="bg-danger-50 border border-danger-200 rounded-md p-4">
					<div className="flex">
						<AlertTriangle className="h-5 w-5 text-danger-400" />
						<div className="ml-3">
							<h3 className="text-sm font-medium text-danger-800">
								Error loading host groups
							</h3>
							<p className="text-sm text-danger-700 mt-1">
								{error.message || "Failed to load host groups"}
							</p>
						</div>
					</div>
				</div>
			);
		}

		return (
			<div className="space-y-6">
				{/* Header */}
				<div className="flex items-center justify-between">
					<div>
						<h2 className="text-xl font-semibold text-secondary-900 dark:text-white">
							Host Groups
						</h2>
						<p className="text-secondary-600 dark:text-secondary-300">
							Organize your hosts into logical groups for better management
						</p>
					</div>
					<button
						type="button"
						onClick={() => setShowCreateModal(true)}
						className="btn-primary flex items-center gap-2"
					>
						<Plus className="h-4 w-4" />
						Create Group
					</button>
				</div>

				{/* Host Groups Grid */}
				{hostGroups && hostGroups.length > 0 ? (
					<div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
						{hostGroups.map((group) => (
							<div
								key={group.id}
								className="bg-white dark:bg-secondary-800 border border-secondary-200 dark:border-secondary-600 rounded-lg p-6 hover:shadow-card-hover dark:hover:shadow-card-hover-dark transition-shadow"
							>
								<div className="flex items-start justify-between">
									<div className="flex items-center gap-3">
										<div
											className="w-3 h-3 rounded-full"
											style={{ backgroundColor: group.color }}
										/>
										<div>
											<h3 className="text-lg font-semibold text-secondary-900 dark:text-white">
												{group.name}
											</h3>
											{group.description && (
												<p className="text-sm text-secondary-600 dark:text-secondary-300 mt-1">
													{group.description}
												</p>
											)}
										</div>
									</div>
									<div className="flex items-center gap-2">
										<button
											type="button"
											onClick={() => handleEdit(group)}
											className="p-1 text-secondary-400 hover:text-secondary-600 hover:bg-secondary-100 rounded"
											title="Edit group"
										>
											<Edit className="h-4 w-4" />
										</button>
										<button
											type="button"
											onClick={() => handleDeleteClick(group)}
											className="p-1 text-secondary-400 hover:text-danger-600 hover:bg-danger-50 rounded"
											title="Delete group"
										>
											<Trash2 className="h-4 w-4" />
										</button>
									</div>
								</div>

								<div className="mt-4 flex items-center gap-4 text-sm text-secondary-600 dark:text-secondary-300">
									<div className="flex items-center gap-1">
										<Server className="h-4 w-4" />
										<span>
											{group._count.hosts} host
											{group._count.hosts !== 1 ? "s" : ""}
										</span>
									</div>
								</div>
							</div>
						))}
					</div>
				) : (
					<div className="text-center py-12">
						<Server className="h-12 w-12 text-secondary-400 mx-auto mb-4" />
						<h3 className="text-lg font-medium text-secondary-900 dark:text-white mb-2">
							No host groups yet
						</h3>
						<p className="text-secondary-600 dark:text-secondary-300 mb-6">
							Create your first host group to organize your hosts
						</p>
						<button
							type="button"
							onClick={() => setShowCreateModal(true)}
							className="btn-primary flex items-center gap-2 mx-auto"
						>
							<Plus className="h-4 w-4" />
							Create Group
						</button>
					</div>
				)}
			</div>
		);
	};

	const renderComingSoonTab = (tabName) => (
		<div className="text-center py-12">
			<Settings className="h-12 w-12 text-secondary-400 mx-auto mb-4" />
			<h3 className="text-lg font-medium text-secondary-900 dark:text-white mb-2">
				{tabName} Coming Soon
			</h3>
			<p className="text-secondary-600 dark:text-secondary-300">
				This feature is currently under development and will be available in a
				future update.
			</p>
		</div>
	);

	return (
		<div className="space-y-6">
			{/* Page Header */}
			<div>
				<h1 className="text-2xl font-bold text-secondary-900 dark:text-white">
					Options
				</h1>
				<p className="text-secondary-600 dark:text-secondary-300 mt-1">
					Configure PatchMon parameters and user preferences
				</p>
			</div>

			{/* Tabs */}
			<div className="border-b border-secondary-200 dark:border-secondary-600">
				<nav className="-mb-px flex space-x-8">
					{tabs.map((tab) => {
						const Icon = tab.icon;
						return (
							<button
								type="button"
								key={tab.id}
								onClick={() => setActiveTab(tab.id)}
								className={`py-2 px-1 border-b-2 font-medium text-sm flex items-center gap-2 ${
									activeTab === tab.id
										? "border-primary-500 text-primary-600 dark:text-primary-400"
										: "border-transparent text-secondary-500 hover:text-secondary-700 hover:border-secondary-300 dark:text-secondary-400 dark:hover:text-secondary-300"
								}`}
							>
								<Icon className="h-4 w-4" />
								{tab.name}
								{tab.comingSoon && (
									<span className="text-xs bg-secondary-100 text-secondary-600 px-1.5 py-0.5 rounded">
										Soon
									</span>
								)}
							</button>
						);
					})}
				</nav>
			</div>

			{/* Tab Content */}
			<div className="mt-6">
				{activeTab === "hostgroups" && renderHostGroupsTab()}
				{activeTab === "notifications" && renderComingSoonTab("Notifications")}
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
							{isLoading ? "Updating..." : "Update Group"}
						</button>
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
					{group._count.hosts > 0 && (
						<div className="mt-3 p-3 bg-warning-50 border border-warning-200 rounded-md">
							<p className="text-sm text-warning-800 mb-2">
								<strong>Warning:</strong> This group contains{" "}
								{group._count.hosts} host
								{group._count.hosts !== 1 ? "s" : ""}. You must move or remove
								these hosts before deleting the group.
							</p>
							{hosts.length > 0 && (
								<div className="mt-2">
									<p className="text-xs font-medium text-warning-900 mb-1">
										Hosts in this group:
									</p>
									<div className="max-h-32 overflow-y-auto bg-warning-100 rounded p-2">
										{hosts.map((host) => (
											<div
												key={host.id}
												className="text-xs text-warning-900 flex items-center gap-1"
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
						disabled={isLoading || group._count.hosts > 0}
					>
						{isLoading ? "Deleting..." : "Delete Group"}
					</button>
				</div>
			</div>
		</div>
	);
};

export default Options;
