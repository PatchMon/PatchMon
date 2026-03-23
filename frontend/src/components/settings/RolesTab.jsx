import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
	AlertTriangle,
	Check,
	Edit,
	Info,
	Minus,
	Save,
	Shield,
	Trash2,
	X,
} from "lucide-react";
import { useEffect, useId, useState } from "react";
import {
	countPermissions,
	PERMISSION_GROUPS,
	ROLE_PRESETS,
	riskBadgeClasses,
	riskBorderColor,
	riskGroupBg,
	riskLabel,
} from "../../constants/permissionGroups";
import { useAuth } from "../../contexts/AuthContext";
import { permissionsAPI } from "../../utils/api";

const RolesTab = () => {
	const [editingRole, setEditingRole] = useState(null);
	const [showAddModal, setShowAddModal] = useState(false);
	const queryClient = useQueryClient();
	const { refreshPermissions } = useAuth();

	// Fetch OIDC config to determine if OIDC is enabled
	const { data: oidcConfig } = useQuery({
		queryKey: ["oidcConfig"],
		queryFn: async () => {
			const response = await fetch("/api/v1/auth/oidc/config");
			if (response.ok) {
				return response.json();
			}
			return { enabled: false };
		},
	});

	const isOIDCEnabled = oidcConfig?.enabled || false;

	// Listen for the header button event to open add modal (only if OIDC is not enabled)
	useEffect(() => {
		const handleOpenAddModal = () => {
			if (!isOIDCEnabled) {
				setShowAddModal(true);
			}
		};
		window.addEventListener("openAddRoleModal", handleOpenAddModal);
		return () =>
			window.removeEventListener("openAddRoleModal", handleOpenAddModal);
	}, [isOIDCEnabled]);

	// Fetch all role permissions
	const {
		data: rolesData,
		isLoading,
		error,
	} = useQuery({
		queryKey: ["rolePermissions"],
		queryFn: () => permissionsAPI.getRoles().then((res) => res.data),
	});

	// Sort roles by permission level: superadmin > admin > host_manager > user > readonly
	// Custom roles come after built-in roles, sorted alphabetically
	const roles = rolesData
		? [...rolesData].sort((a, b) => {
				const order = {
					superadmin: 0,
					admin: 1,
					host_manager: 2,
					user: 3,
					readonly: 4,
				};
				const aOrder = order[a.role] ?? 999;
				const bOrder = order[b.role] ?? 999;
				if (aOrder !== bOrder) return aOrder - bOrder;
				return a.role.localeCompare(b.role);
			})
		: null;

	// Update role permissions mutation
	const updateRoleMutation = useMutation({
		mutationFn: ({ role, permissions }) =>
			permissionsAPI.updateRole(role, permissions),
		onSuccess: () => {
			queryClient.invalidateQueries(["rolePermissions"]);
			setEditingRole(null);
			// Refresh user permissions to apply changes immediately
			refreshPermissions();
		},
	});

	// Delete role mutation
	const deleteRoleMutation = useMutation({
		mutationFn: (role) => permissionsAPI.deleteRole(role),
		onSuccess: () => {
			queryClient.invalidateQueries(["rolePermissions"]);
		},
	});

	const handleSavePermissions = async (role, permissions) => {
		try {
			await updateRoleMutation.mutateAsync({ role, permissions });
		} catch (error) {
			console.error("Failed to update permissions:", error);
		}
	};

	const handleDeleteRole = async (role) => {
		if (
			window.confirm(
				`Are you sure you want to delete the "${role}" role? This action cannot be undone.`,
			)
		) {
			try {
				await deleteRoleMutation.mutateAsync(role);
			} catch (error) {
				console.error("Failed to delete role:", error);
			}
		}
	};

	if (isLoading) {
		return (
			<div className="flex items-center justify-center h-64">
				<div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600" />
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
							Error loading permissions
						</h3>
						<p className="mt-1 text-sm text-danger-700">{error.message}</p>
					</div>
				</div>
			</div>
		);
	}

	return (
		<div className="space-y-6">
			{/* OIDC Info Banner */}
			{isOIDCEnabled && (
				<div className="bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-lg p-4">
					<div className="flex">
						<Info className="h-5 w-5 text-blue-500 dark:text-blue-400 flex-shrink-0" />
						<div className="ml-3">
							<h3 className="text-sm font-medium text-blue-800 dark:text-blue-200">
								OIDC Role Mapping Enabled
							</h3>
							<p className="mt-1 text-sm text-blue-700 dark:text-blue-300">
								Roles are managed via your Identity Provider (IdP) groups.
								Configure the following environment variables to map IdP groups
								to roles:
							</p>
							<ul className="mt-2 text-sm text-blue-600 dark:text-blue-400 list-disc list-inside space-y-1">
								<li>
									<code className="bg-blue-100 dark:bg-blue-900 px-1 rounded">
										OIDC_SUPERADMIN_GROUP
									</code>{" "}
									+{" "}
									<code className="bg-blue-100 dark:bg-blue-900 px-1 rounded">
										OIDC_ADMIN_GROUP
									</code>{" "}
									maps to Super Admin (highest)
								</li>
								<li>
									<code className="bg-blue-100 dark:bg-blue-900 px-1 rounded">
										OIDC_ADMIN_GROUP
									</code>{" "}
									maps to Admin
								</li>
								<li>
									<code className="bg-blue-100 dark:bg-blue-900 px-1 rounded">
										OIDC_HOST_MANAGER_GROUP
									</code>{" "}
									maps to Host Manager
								</li>
								<li>
									<code className="bg-blue-100 dark:bg-blue-900 px-1 rounded">
										OIDC_USER_GROUP
									</code>{" "}
									maps to User (can export data)
								</li>
								<li>
									<code className="bg-blue-100 dark:bg-blue-900 px-1 rounded">
										OIDC_READONLY_GROUP
									</code>{" "}
									maps to Readonly (view only, lowest)
								</li>
							</ul>
						</div>
					</div>
				</div>
			)}

			{/* Roles Matrix Table */}
			<div className="bg-white dark:bg-secondary-800 shadow overflow-hidden sm:rounded-lg">
				<div className="overflow-x-auto">
					<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-600">
						<thead className="bg-secondary-50 dark:bg-secondary-700">
							<tr>
								<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider sticky left-0 z-10 bg-secondary-50 dark:bg-secondary-700 border-r border-secondary-200 dark:border-secondary-700">
									Permission
								</th>
								{roles &&
									Array.isArray(roles) &&
									roles.map((r) => {
										const { enabled, total } = countPermissions(r);
										return (
											<th
												key={r.role}
												className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider"
											>
												<div className="flex items-center gap-2">
													<div>
														<span className="capitalize">
															{r.role.replace(/_/g, " ")}
														</span>
														<div className="text-[10px] font-normal normal-case text-secondary-400 dark:text-secondary-300 mt-0.5">
															{enabled}/{total}
														</div>
													</div>
													<button
														type="button"
														onClick={() => setEditingRole(r.role)}
														className="text-secondary-400 hover:text-secondary-600 dark:text-white dark:hover:text-secondary-200"
														title="Edit role permissions"
													>
														<Edit className="h-4 w-4" />
													</button>
												</div>
											</th>
										);
									})}
							</tr>
						</thead>
						<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-600">
							{roles &&
								Array.isArray(roles) &&
								roles.length > 0 &&
								PERMISSION_GROUPS.map((group) => {
									const GroupIcon = group.icon;
									return [
										/* Group header row */
										<tr
											key={`group-${group.id}`}
											className={`border-l-4 ${riskBorderColor(group.riskLevel)} ${riskGroupBg(group.riskLevel)}`}
										>
											<td colSpan={1 + roles.length} className="px-6 py-2.5">
												<div className="flex items-center gap-2">
													<GroupIcon className="h-4 w-4 text-secondary-500 dark:text-secondary-300" />
													<span className="text-sm font-semibold text-secondary-800 dark:text-secondary-100">
														{group.name}
													</span>
													<span className="text-xs text-secondary-500 dark:text-secondary-400 hidden sm:inline">
														{group.description}
													</span>
												</div>
											</td>
										</tr>,
										/* Permission rows within the group */
										...group.permissions.map((perm, permIdx) => {
											const PermIcon = perm.icon;
											return (
												<tr
													key={perm.key}
													className={`hover:bg-secondary-50 dark:hover:bg-secondary-700 ${permIdx % 2 === 0 ? "bg-secondary-50/50 dark:bg-secondary-800/50" : ""}`}
												>
													<td className="px-6 py-3 text-sm text-secondary-700 dark:text-secondary-200 whitespace-nowrap sticky left-0 z-10 bg-white dark:bg-secondary-800 border-r border-secondary-200 dark:border-secondary-700">
														<div className="flex items-center gap-2">
															<PermIcon className="h-4 w-4 text-secondary-400 flex-shrink-0" />
															<div>
																<div className="font-medium">{perm.label}</div>
																<div className="text-xs text-secondary-400 dark:text-secondary-500 hidden lg:block">
																	{perm.description}
																</div>
															</div>
														</div>
													</td>
													{roles.map((r) => (
														<td
															key={`${r.role}-${perm.key}`}
															className="px-6 py-3 whitespace-nowrap"
														>
															{r[perm.key] ? (
																<div className="inline-flex items-center justify-center h-6 w-6 rounded-full bg-green-100 dark:bg-green-900/30">
																	<Check className="h-3.5 w-3.5 text-green-600 dark:text-green-400" />
																</div>
															) : (
																<div className="inline-flex items-center justify-center h-6 w-6 rounded-full bg-secondary-100 dark:bg-secondary-700">
																	<Minus className="h-3.5 w-3.5 text-secondary-400 dark:text-secondary-500" />
																</div>
															)}
														</td>
													))}
												</tr>
											);
										}),
									];
								})}
						</tbody>
					</table>
				</div>
			</div>

			{/* Inline editor for selected role */}
			{editingRole && roles && Array.isArray(roles) && (
				<div className="space-y-4">
					{roles
						.filter((r) => r.role === editingRole)
						.map((r) => (
							<RolePermissionsCard
								key={`editor-${r.role}`}
								role={r}
								isEditing={true}
								isOIDCEnabled={isOIDCEnabled}
								onEdit={() => {}}
								onCancel={() => setEditingRole(null)}
								onSave={handleSavePermissions}
								onDelete={handleDeleteRole}
							/>
						))}
				</div>
			)}

			{/* Add Role Modal - only show when OIDC is not enabled */}
			{!isOIDCEnabled && (
				<AddRoleModal
					isOpen={showAddModal}
					onClose={() => setShowAddModal(false)}
					onSuccess={() => {
						queryClient.invalidateQueries(["rolePermissions"]);
						setShowAddModal(false);
					}}
				/>
			)}
		</div>
	);
};

// Role Permissions Card Component
const RolePermissionsCard = ({
	role,
	isEditing,
	isOIDCEnabled = false,
	onEdit,
	onCancel,
	onSave,
	onDelete,
}) => {
	const [permissions, setPermissions] = useState(role);

	// Sync permissions state with role prop when it changes
	useEffect(() => {
		setPermissions(role);
	}, [role]);

	const handlePermissionChange = (key, value) => {
		setPermissions((prev) => ({
			...prev,
			[key]: value,
		}));
	};

	const handleSave = () => {
		onSave(role.role, permissions);
	};

	// Standard built-in roles (always protected from deletion and permission changes)
	const standardBuiltInRoles = ["superadmin", "admin", "user"];
	// OIDC roles (protected from deletion when OIDC is enabled, but permissions can be edited)
	const oidcRoles = ["superadmin", "admin", "host_manager", "readonly", "user"];

	const isBuiltInRole = standardBuiltInRoles.includes(role.role);
	const isOIDCRole = oidcRoles.includes(role.role);
	// Can't delete OIDC roles or built-in roles
	const cannotDelete = isOIDCEnabled ? isOIDCRole : isBuiltInRole;
	// Can't edit permissions for built-in roles (superadmin, admin, user)
	const cannotEditPermissions = isBuiltInRole;

	return (
		<div className="bg-white dark:bg-secondary-800 shadow rounded-lg">
			<div className="px-6 py-4 border-b border-secondary-200 dark:border-secondary-600">
				<div className="flex items-center justify-between">
					<div className="flex items-center">
						<Shield className="h-5 w-5 text-primary-600 mr-3" />
						<h3 className="text-lg font-medium text-secondary-900 dark:text-white capitalize">
							{role.role.replace(/_/g, " ")}
						</h3>
						{isOIDCEnabled && isOIDCRole && (
							<span className="ml-2 inline-flex items-center px-2.5 py-0.5 rounded-md text-xs font-medium bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200">
								OIDC Role
							</span>
						)}
						{!isOIDCEnabled && isBuiltInRole && (
							<span className="ml-2 inline-flex items-center px-2.5 py-0.5 rounded-md text-xs font-medium bg-primary-100 text-primary-800 dark:bg-primary-900 dark:text-primary-200">
								Built-in Role
							</span>
						)}
					</div>
					<div className="flex items-center space-x-2">
						{isEditing ? (
							<>
								<button
									type="button"
									onClick={handleSave}
									className="inline-flex items-center px-3 py-1 border border-transparent text-sm font-medium rounded-md text-white bg-green-600 hover:bg-green-700"
								>
									<Save className="h-4 w-4 mr-1" />
									Save
								</button>
								<button
									type="button"
									onClick={onCancel}
									className="inline-flex items-center px-3 py-1 border border-secondary-300 dark:border-secondary-600 text-sm font-medium rounded-md text-secondary-700 dark:text-secondary-200 bg-white dark:bg-secondary-700 hover:bg-secondary-50 dark:hover:bg-secondary-600"
								>
									<X className="h-4 w-4 mr-1" />
									Cancel
								</button>
								{!cannotDelete && (
									<button
										type="button"
										onClick={() => onDelete(role.role)}
										className="inline-flex items-center px-3 py-1 border border-transparent text-sm font-medium rounded-md text-white bg-red-600 hover:bg-red-700"
									>
										<Trash2 className="h-4 w-4 mr-1" />
										Delete
									</button>
								)}
							</>
						) : (
							<>
								<button
									type="button"
									onClick={onEdit}
									disabled={cannotEditPermissions}
									className="inline-flex items-center px-3 py-1 border border-transparent text-sm font-medium rounded-md text-white bg-primary-600 hover:bg-primary-700 disabled:opacity-50 disabled:cursor-not-allowed"
								>
									<Edit className="h-4 w-4 mr-1" />
									Edit
								</button>
								{!cannotDelete && (
									<button
										type="button"
										onClick={() => onDelete(role.role)}
										className="inline-flex items-center px-3 py-1 border border-transparent text-sm font-medium rounded-md text-white bg-red-600 hover:bg-red-700"
									>
										<Trash2 className="h-4 w-4 mr-1" />
										Delete
									</button>
								)}
							</>
						)}
					</div>
				</div>
			</div>

			<div className="px-6 py-4 space-y-6">
				{PERMISSION_GROUPS.map((group) => {
					const GroupIcon = group.icon;
					return (
						<div key={group.id}>
							{/* Group header */}
							<div className="flex items-center gap-2 mb-3">
								<GroupIcon className="h-4 w-4 text-secondary-500 dark:text-secondary-300" />
								<h4 className="text-sm font-semibold text-secondary-800 dark:text-secondary-100">
									{group.name}
								</h4>
								<span
									className={`inline-flex items-center px-2 py-0.5 rounded-full text-[10px] font-medium ${riskBadgeClasses(group.riskLevel)}`}
								>
									{riskLabel(group.riskLevel)}
								</span>
							</div>
							{/* Permissions grid */}
							<div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
								{group.permissions.map((field) => {
									const Icon = field.icon;
									const isChecked = permissions[field.key];
									return (
										<div key={field.key} className="flex items-start">
											<div className="flex items-center h-5">
												<input
													id={`${role.role}-${field.key}`}
													type="checkbox"
													checked={isChecked}
													onChange={(e) =>
														handlePermissionChange(field.key, e.target.checked)
													}
													disabled={!isEditing || cannotEditPermissions}
													className="h-4 w-4 text-primary-600 focus:ring-primary-500 border-secondary-300 rounded disabled:opacity-50"
												/>
											</div>
											<div className="ml-3">
												<div className="flex items-center">
													<Icon className="h-4 w-4 text-secondary-400 mr-2" />
													<label
														htmlFor={`${role.role}-${field.key}`}
														className="text-sm font-medium text-secondary-900 dark:text-white"
													>
														{field.label}
													</label>
												</div>
												<p className="text-xs text-secondary-500 mt-1">
													{field.description}
												</p>
											</div>
										</div>
									);
								})}
							</div>
						</div>
					);
				})}
			</div>
		</div>
	);
};

// Add Role Modal Component
const AddRoleModal = ({ isOpen, onClose, onSuccess }) => {
	const roleNameInputId = useId();
	const [formData, setFormData] = useState({
		role: "",
		...ROLE_PRESETS.clear,
	});
	const [isLoading, setIsLoading] = useState(false);
	const [error, setError] = useState("");

	const handleSubmit = async (e) => {
		e.preventDefault();
		setIsLoading(true);
		setError("");

		try {
			await permissionsAPI.updateRole(formData.role, formData);
			onSuccess();
		} catch (err) {
			setError(err.response?.data?.error || "Failed to create role");
		} finally {
			setIsLoading(false);
		}
	};

	const handleInputChange = (e) => {
		const { name, value, type, checked } = e.target;
		setFormData({
			...formData,
			[name]: type === "checkbox" ? checked : value,
		});
	};

	const applyPreset = (presetKey) => {
		setFormData((prev) => ({
			...prev,
			...ROLE_PRESETS[presetKey],
		}));
	};

	const toggleGroup = (group, selectAll) => {
		setFormData((prev) => {
			const updated = { ...prev };
			for (const perm of group.permissions) {
				updated[perm.key] = selectAll;
			}
			return updated;
		});
	};

	const isGroupAllSelected = (group) =>
		group.permissions.every((p) => formData[p.key]);

	const { enabled, total } = countPermissions(formData);

	if (!isOpen) return null;

	return (
		<div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
			<div className="bg-white dark:bg-secondary-800 rounded-lg p-6 w-full max-w-2xl max-h-[90vh] overflow-y-auto">
				<h3 className="text-lg font-medium text-secondary-900 dark:text-white mb-4">
					Add New Role
				</h3>

				<form onSubmit={handleSubmit} className="space-y-4">
					{/* Role name input */}
					<div>
						<label
							htmlFor={roleNameInputId}
							className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
						>
							Role Name
						</label>
						<input
							id={roleNameInputId}
							type="text"
							name="role"
							required
							value={formData.role}
							onChange={handleInputChange}
							className="block w-full border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
							placeholder="e.g., host_manager, readonly"
						/>
						<p className="mt-1 text-xs text-secondary-500 dark:text-white">
							Use lowercase with underscores (e.g., host_manager)
						</p>
					</div>

					{/* Preset buttons */}
					<div className="flex flex-wrap gap-2">
						<button
							type="button"
							onClick={() => applyPreset("readonly")}
							className="px-3 py-1.5 text-xs font-medium rounded-md border border-green-300 dark:border-green-700 text-green-700 dark:text-green-400 bg-green-50 dark:bg-green-900/20 hover:bg-green-100 dark:hover:bg-green-900/40"
						>
							Read Only
						</button>
						<button
							type="button"
							onClick={() => applyPreset("operator")}
							className="px-3 py-1.5 text-xs font-medium rounded-md border border-yellow-300 dark:border-yellow-700 text-yellow-700 dark:text-yellow-400 bg-yellow-50 dark:bg-yellow-900/20 hover:bg-yellow-100 dark:hover:bg-yellow-900/40"
						>
							Operator
						</button>
						<button
							type="button"
							onClick={() => applyPreset("admin")}
							className="px-3 py-1.5 text-xs font-medium rounded-md border border-red-300 dark:border-red-700 text-red-700 dark:text-red-400 bg-red-50 dark:bg-red-900/20 hover:bg-red-100 dark:hover:bg-red-900/40"
						>
							Admin
						</button>
						<button
							type="button"
							onClick={() => applyPreset("clear")}
							className="px-3 py-1.5 text-xs font-medium rounded-md border border-secondary-300 dark:border-secondary-600 text-secondary-700 dark:text-secondary-300 bg-white dark:bg-secondary-700 hover:bg-secondary-50 dark:hover:bg-secondary-600"
						>
							Clear All
						</button>
					</div>

					{/* Grouped permissions */}
					<div className="space-y-5">
						{PERMISSION_GROUPS.map((group) => {
							const GroupIcon = group.icon;
							const allSelected = isGroupAllSelected(group);
							return (
								<div
									key={group.id}
									className={`rounded-lg border border-secondary-200 dark:border-secondary-700 border-l-4 ${riskBorderColor(group.riskLevel)} overflow-hidden`}
								>
									{/* Group header */}
									<div className="flex items-center justify-between px-4 py-2.5 bg-secondary-50 dark:bg-secondary-750">
										<div className="flex items-center gap-2">
											<GroupIcon className="h-4 w-4 text-secondary-500 dark:text-secondary-300" />
											<span className="text-sm font-semibold text-secondary-800 dark:text-secondary-100">
												{group.name}
											</span>
											<span
												className={`inline-flex items-center px-2 py-0.5 rounded-full text-[10px] font-medium ${riskBadgeClasses(group.riskLevel)}`}
											>
												{riskLabel(group.riskLevel)}
											</span>
										</div>
										<button
											type="button"
											onClick={() => toggleGroup(group, !allSelected)}
											className="text-xs text-primary-600 dark:text-primary-400 hover:text-primary-800 dark:hover:text-primary-300 font-medium"
										>
											{allSelected ? "Deselect all" : "Select all"}
										</button>
									</div>
									{/* Permission checkboxes */}
									<div className="grid grid-cols-1 sm:grid-cols-2 gap-3 p-4">
										{group.permissions.map((perm) => {
											const PermIcon = perm.icon;
											return (
												<div key={perm.key} className="flex items-start gap-2">
													<input
														id={`add-role-${perm.key}`}
														type="checkbox"
														name={perm.key}
														checked={formData[perm.key]}
														onChange={handleInputChange}
														className="mt-0.5 h-4 w-4 text-primary-600 focus:ring-primary-500 border-secondary-300 rounded"
													/>
													<label
														htmlFor={`add-role-${perm.key}`}
														className="flex-1 min-w-0"
													>
														<div className="flex items-center gap-1.5">
															<PermIcon className="h-3.5 w-3.5 text-secondary-400 flex-shrink-0" />
															<span className="text-sm font-medium text-secondary-800 dark:text-secondary-100">
																{perm.label}
															</span>
														</div>
														<p className="text-xs text-secondary-500 dark:text-secondary-400 mt-0.5">
															{perm.description}
														</p>
													</label>
												</div>
											);
										})}
									</div>
								</div>
							);
						})}
					</div>

					{error && (
						<div className="bg-danger-50 dark:bg-danger-900 border border-danger-200 dark:border-danger-700 rounded-md p-3">
							<p className="text-sm text-danger-700 dark:text-danger-300">
								{error}
							</p>
						</div>
					)}

					<div className="flex items-center justify-between pt-2">
						<span className="text-sm text-secondary-500 dark:text-secondary-400">
							{enabled}/{total} permissions selected
						</span>
						<div className="flex space-x-3">
							<button
								type="button"
								onClick={onClose}
								className="px-4 py-2 text-sm font-medium text-secondary-700 dark:text-secondary-200 bg-white dark:bg-secondary-700 border border-secondary-300 dark:border-secondary-600 rounded-md hover:bg-secondary-50 dark:hover:bg-secondary-600"
							>
								Cancel
							</button>
							<button
								type="submit"
								disabled={isLoading}
								className="px-4 py-2 text-sm font-medium text-white bg-primary-600 border border-transparent rounded-md hover:bg-primary-700 disabled:opacity-50"
							>
								{isLoading ? "Creating..." : "Create Role"}
							</button>
						</div>
					</div>
				</form>
			</div>
		</div>
	);
};

export default RolesTab;
