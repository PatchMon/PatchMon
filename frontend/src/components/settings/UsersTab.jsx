import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
	Calendar,
	CheckCircle,
	CheckSquare,
	Edit,
	Info,
	Key,
	Mail,
	RefreshCw,
	Save,
	Shield,
	ShieldOff,
	Square,
	Trash2,
	User,
	X,
	XCircle,
} from "lucide-react";
import { useEffect, useId, useState } from "react";
import { useAuth } from "../../contexts/AuthContext";
import { useConfirm } from "../../contexts/ConfirmContext";
import { useSettings } from "../../contexts/SettingsContext";
import { useToast } from "../../contexts/ToastContext";
import {
	adminUsersAPI,
	formatDateOnly,
	permissionsAPI,
	settingsAPI,
} from "../../utils/api";
import { isRenderableAvatarSrc } from "../../utils/avatar";
import ModalPortal from "../ui/ModalPortal";

// Mirrors roleCovers and roleOutranks in role_guard.go on the server.
const roleCovers = (roles, callerRole, targetRole) => {
	if (callerRole === "superadmin") return true;
	const caller = roles?.find((r) => r.role === callerRole);
	if (!caller) return false;
	const target = roles.find((r) => r.role === targetRole);
	if (!target) return targetRole !== "admin" && targetRole !== "superadmin";
	return Object.entries(target).every(
		([key, value]) =>
			!key.startsWith("can_") || value !== true || caller[key] === true,
	);
};

const roleOutranks = (roles, callerRole, targetRole) =>
	callerRole === "superadmin" ||
	(roleCovers(roles, callerRole, targetRole) &&
		!roleCovers(roles, targetRole, callerRole));

const canResetTfaOf = (roles, actor, target) =>
	!!actor &&
	actor.id !== target.id &&
	roleOutranks(roles, actor.role, target.role);

const UsersTab = () => {
	const confirm = useConfirm();
	const toast = useToast();
	const [showAddModal, setShowAddModal] = useState(false);
	const [editingUser, setEditingUser] = useState(null);
	const [resetPasswordUser, setResetPasswordUser] = useState(null);
	const [resetTfaUser, setResetTfaUser] = useState(null);
	const queryClient = useQueryClient();
	const { user: currentUser } = useAuth();
	const { settings: publicSettings } = useSettings();
	const isAdminMode = publicSettings?.admin_mode;
	const signupEnabledId = useId();
	const defaultRoleId = useId();
	const [signupFormData, setSignupFormData] = useState({
		signupEnabled: false,
		defaultUserRole: "user",
	});
	const [isSignupDirty, setIsSignupDirty] = useState(false);

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
	const isOIDCSyncRoles = isOIDCEnabled && (oidcConfig?.syncRoles || false);

	// Listen for the header button event to open add modal (only blocked when OIDC sync roles is active)
	useEffect(() => {
		const handleOpenAddModal = () => {
			if (!isOIDCSyncRoles) {
				setShowAddModal(true);
			}
		};
		window.addEventListener("openAddUserModal", handleOpenAddModal);
		return () =>
			window.removeEventListener("openAddUserModal", handleOpenAddModal);
	}, [isOIDCSyncRoles]);

	// Fetch users
	const {
		data: users,
		isLoading,
		error,
	} = useQuery({
		queryKey: ["users"],
		queryFn: () => adminUsersAPI.list().then((res) => res.data.data),
	});

	// Fetch available roles
	const { data: roles, isLoading: rolesLoading } = useQuery({
		queryKey: ["rolePermissions"],
		queryFn: () => permissionsAPI.getRoles().then((res) => res.data),
	});

	// Fetch current settings for user registration
	const { data: settings, isLoading: settingsLoading } = useQuery({
		queryKey: ["settings"],
		queryFn: () => settingsAPI.get().then((res) => res.data),
	});

	// Never hydrate over unsaved edits: a refetch would otherwise discard them.
	useEffect(() => {
		if (!settings || isSignupDirty) return;
		setSignupFormData({
			signupEnabled: settings.signup_enabled === true,
			defaultUserRole: settings.default_user_role || "user",
		});
	}, [settings, isSignupDirty]);

	// Delete user mutation
	const deleteUserMutation = useMutation({
		mutationFn: adminUsersAPI.delete,
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ["users"] });
		},
	});

	// Update user mutation
	// The modal awaits this, so it must be mutateAsync: mutate() swallows
	// rejections and the await would resolve on failure.
	const updateUserMutation = useMutation({
		mutationFn: ({ id, data }) => adminUsersAPI.update(id, data),
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ["users"] });
			setEditingUser(null);
		},
		onError: (error) => {
			console.error("Failed to update user:", error);
		},
	});

	const unlinkOidcMutation = useMutation({
		mutationFn: (userId) => adminUsersAPI.unlinkOidc(userId),
		onSuccess: (_, userId) => {
			queryClient.invalidateQueries({ queryKey: ["users"] });
			setEditingUser((current) =>
				current?.id === userId
					? { ...current, oidc_linked: false, oidc_provider: undefined }
					: current,
			);
		},
	});

	// Reset password mutation
	const resetPasswordMutation = useMutation({
		mutationFn: ({ userId, newPassword }) =>
			adminUsersAPI.resetPassword(userId, newPassword),
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ["users"] });
			setResetPasswordUser(null);
		},
		onError: (error) => {
			console.error("Failed to reset password:", error);
		},
	});

	const resetTfaMutation = useMutation({
		mutationFn: ({ userId, password, tfaToken, newPassword }) =>
			adminUsersAPI.resetTfa(userId, { password, tfaToken, newPassword }),
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ["users"] });
		},
	});

	// Update settings mutation for user registration
	const updateSignupSettingsMutation = useMutation({
		mutationFn: (data) => {
			return settingsAPI.update(data).then((res) => res.data);
		},
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ["settings"] });
			setIsSignupDirty(false);
		},
		onError: (error) => {
			toast.error(
				error.response?.data?.error || "Failed to save registration settings",
			);
		},
	});

	const handleSignupInputChange = (field, value) => {
		setSignupFormData((prev) => ({
			...prev,
			[field]: value,
		}));
		setIsSignupDirty(true);
	};

	const handleSignupSave = () => {
		updateSignupSettingsMutation.mutate({
			signupEnabled: signupFormData.signupEnabled,
			defaultUserRole: signupFormData.defaultUserRole,
		});
	};

	const handleDeleteUser = async (userId, username) => {
		const confirmed = await confirm({
			title: "Delete user",
			message: `Are you sure you want to delete user "${username}"?`,
			confirmLabel: "Delete user",
		});
		if (!confirmed) return;

		try {
			await deleteUserMutation.mutateAsync(userId);
			toast.success(`User "${username}" deleted`);
		} catch (error) {
			console.error("Failed to delete user:", error);
			toast.error(error.response?.data?.error || "Failed to delete user");
		}
	};

	const handleUserCreated = () => {
		queryClient.invalidateQueries({ queryKey: ["users"] });
		setShowAddModal(false);
	};

	const handleEditUser = (user) => {
		// Reset editingUser first to force re-render with fresh data
		setEditingUser(null);
		// Use setTimeout to ensure the modal re-initializes with fresh data
		setTimeout(() => {
			setEditingUser(user);
		}, 0);
	};

	const handleResetPassword = (user) => {
		setResetPasswordUser(user);
	};

	const handleTfaReset = (username, withTempPassword) => {
		setResetTfaUser(null);
		toast.success(
			withTempPassword
				? `Two-factor authentication reset and temporary password set for "${username}"`
				: `Two-factor authentication reset for "${username}"`,
		);
	};

	const currentUserTfaEnabled = Array.isArray(users)
		? users.find((u) => u.id === currentUser?.id)?.tfa_enabled
		: undefined;

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
					<XCircle className="h-5 w-5 text-danger-400" />
					<div className="ml-3">
						<h3 className="text-sm font-medium text-danger-800">
							Error loading users
						</h3>
						<p className="mt-1 text-sm text-danger-700">{error.message}</p>
					</div>
				</div>
			</div>
		);
	}

	return (
		<div className="space-y-6">
			{/* Users Table */}
			<div className="bg-white dark:bg-secondary-800 shadow overflow-hidden sm:rounded-lg">
				{users && Array.isArray(users) && users.length > 0 ? (
					<>
						{/* Mobile Card Layout */}
						<div className="md:hidden space-y-3 p-4">
							{users.map((user) => (
								<div key={user.id} className="card p-4 space-y-3">
									{/* User Name and Avatar */}
									<div className="flex items-center gap-3">
										<div className="flex-shrink-0 h-10 w-10">
											{isRenderableAvatarSrc(user.avatar_url) ? (
												<img
													src={user.avatar_url}
													alt={user.username}
													className="h-10 w-10 rounded-full object-cover"
												/>
											) : (
												<div className="h-10 w-10 rounded-full bg-primary-100 dark:bg-primary-900 flex items-center justify-center">
													<User className="h-5 w-5 text-primary-600 dark:text-primary-400" />
												</div>
											)}
										</div>
										<div className="flex-1 min-w-0">
											<div className="flex items-center gap-2">
												<div className="text-base font-semibold text-secondary-900 dark:text-white truncate">
													{user.username}
												</div>
												{user.id === currentUser?.id && (
													<span className="inline-flex items-center px-2 py-0.5 rounded-md text-xs font-medium bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200 flex-shrink-0">
														You
													</span>
												)}
											</div>
										</div>
									</div>

									{/* Email */}
									<div className="flex items-center gap-2 text-sm">
										<Mail className="h-4 w-4 text-secondary-400 flex-shrink-0" />
										<span className="text-secondary-900 dark:text-white truncate">
											{user.email}
										</span>
									</div>

									{/* Role and Status */}
									<div className="flex items-center justify-between gap-2">
										<span
											className={`inline-flex items-center px-2.5 py-0.5 rounded-md text-xs font-medium ${
												user.role === "superadmin"
													? "bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200"
													: user.role === "admin"
														? "bg-primary-100 text-primary-800 dark:bg-primary-900 dark:text-primary-200"
														: user.role === "host_manager"
															? "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200"
															: user.role === "readonly"
																? "bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200"
																: "bg-secondary-100 text-secondary-800 dark:bg-secondary-700 dark:text-secondary-200"
											}`}
										>
											<Shield className="h-3 w-3 mr-1" />
											{user.role === "superadmin"
												? "Super Admin"
												: user.role.charAt(0).toUpperCase() +
													user.role.slice(1).replace("_", " ")}
										</span>
										{user.is_active ? (
											<div className="flex items-center text-green-600 dark:text-green-400">
												<CheckCircle className="h-4 w-4 mr-1" />
												<span className="text-xs font-medium">Active</span>
											</div>
										) : (
											<div className="flex items-center text-red-600 dark:text-red-400">
												<XCircle className="h-4 w-4 mr-1" />
												<span className="text-xs font-medium">Inactive</span>
											</div>
										)}
									</div>

									{/* Created and Last Login */}
									<div className="space-y-2 pt-2 border-t border-secondary-200 dark:border-secondary-600">
										<div className="flex items-center gap-2 text-sm">
											<Calendar className="h-4 w-4 text-secondary-400 flex-shrink-0" />
											<span className="text-secondary-500 dark:text-white">
												Created:&nbsp;
											</span>
											<span className="text-secondary-900 dark:text-white">
												{formatDateOnly(user.created_at)}
											</span>
										</div>
										<div className="text-sm">
											<span className="text-secondary-500 dark:text-white">
												Last Login:&nbsp;
											</span>
											<span className="text-secondary-900 dark:text-white">
												{user.last_login
													? formatDateOnly(user.last_login)
													: "Never"}
											</span>
										</div>
									</div>

									{/* Actions */}
									<div className="flex items-center justify-end gap-3 pt-2 border-t border-secondary-200 dark:border-secondary-600">
										<button
											type="button"
											onClick={() => handleEditUser(user)}
											className="text-secondary-400 hover:text-secondary-600 dark:text-white dark:hover:text-secondary-300 inline-flex items-center gap-1 text-sm"
											title="Edit user"
										>
											<Edit className="h-4 w-4" />
											Edit
										</button>
										<button
											type="button"
											onClick={() => handleResetPassword(user)}
											className="text-blue-400 hover:text-blue-600 dark:text-blue-500 dark:hover:text-blue-300 disabled:text-gray-300 disabled:cursor-not-allowed inline-flex items-center gap-1 text-sm"
											title={
												!user.is_active
													? "Cannot reset password for inactive user"
													: "Reset password"
											}
											disabled={!user.is_active}
										>
											<Key className="h-4 w-4" />
											Reset
										</button>
										{user.tfa_enabled &&
											canResetTfaOf(roles, currentUser, user) && (
												<button
													type="button"
													onClick={() => setResetTfaUser(user)}
													className="text-warning-500 hover:text-warning-700 dark:text-warning-400 dark:hover:text-warning-300 inline-flex items-center gap-1 text-sm min-h-[44px]"
													title="Reset two-factor authentication"
													aria-label={`Reset two-factor authentication for ${user.username}`}
												>
													<ShieldOff className="h-4 w-4" />
													Reset 2FA
												</button>
											)}
										<button
											type="button"
											onClick={() => handleDeleteUser(user.id, user.username)}
											className="text-danger-400 hover:text-danger-600 dark:text-danger-500 dark:hover:text-danger-400 disabled:text-gray-300 disabled:cursor-not-allowed inline-flex items-center gap-1 text-sm"
											title={
												user.id === currentUser?.id
													? "Cannot delete your own account"
													: user.role === "admin" &&
															users.filter((u) => u.role === "admin").length ===
																1
														? "Cannot delete the last admin user"
														: "Delete user"
											}
											disabled={
												user.id === currentUser?.id ||
												(user.role === "admin" &&
													users.filter((u) => u.role === "admin").length === 1)
											}
										>
											<Trash2 className="h-4 w-4" />
											Delete
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
										<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider">
											User
										</th>
										<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider">
											Email
										</th>
										<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider">
											Role
										</th>
										<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider">
											Status
										</th>
										<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider">
											Created
										</th>
										<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider">
											Last Login
										</th>
										<th className="px-2 py-3 text-right text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider">
											Actions
										</th>
									</tr>
								</thead>
								<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-600">
									{users.map((user) => (
										<tr
											key={user.id}
											className="hover:bg-secondary-50 dark:hover:bg-secondary-700"
										>
											<td className="px-6 py-4 whitespace-nowrap">
												<div className="flex items-center">
													<div className="flex-shrink-0 h-10 w-10">
														{isRenderableAvatarSrc(user.avatar_url) ? (
															<img
																src={user.avatar_url}
																alt={user.username}
																className="h-10 w-10 rounded-full object-cover"
															/>
														) : (
															<div className="h-10 w-10 rounded-full bg-primary-100 flex items-center justify-center">
																<User className="h-5 w-5 text-primary-600" />
															</div>
														)}
													</div>
													<div className="ml-4">
														<div className="flex items-center">
															<div className="text-sm font-medium text-secondary-900 dark:text-white">
																{user.username}
															</div>
															{user.id === currentUser?.id && (
																<span className="ml-2 inline-flex items-center px-2.5 py-0.5 rounded-md text-xs font-medium bg-blue-100 text-blue-800">
																	You
																</span>
															)}
														</div>
													</div>
												</div>
											</td>
											<td className="px-6 py-4 whitespace-nowrap">
												<div className="flex items-center text-sm text-secondary-500 dark:text-white">
													<Mail className="h-4 w-4 mr-2" />
													{user.email}
												</div>
											</td>
											<td className="px-6 py-4 whitespace-nowrap">
												<span
													className={`inline-flex items-center px-2.5 py-0.5 rounded-md text-xs font-medium ${
														user.role === "superadmin"
															? "bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200"
															: user.role === "admin"
																? "bg-primary-100 text-primary-800 dark:bg-primary-900 dark:text-primary-200"
																: user.role === "host_manager"
																	? "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200"
																	: user.role === "readonly"
																		? "bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200"
																		: "bg-secondary-100 text-secondary-800 dark:bg-secondary-700 dark:text-secondary-200"
													}`}
												>
													<Shield className="h-3 w-3 mr-1" />
													{user.role === "superadmin"
														? "Super Admin"
														: user.role.charAt(0).toUpperCase() +
															user.role.slice(1).replace("_", " ")}
												</span>
											</td>
											<td className="px-6 py-4 whitespace-nowrap">
												{user.is_active ? (
													<div className="flex items-center text-green-600">
														<CheckCircle className="h-4 w-4 mr-1" />
														<span className="text-sm">Active</span>
													</div>
												) : (
													<div className="flex items-center text-red-600">
														<XCircle className="h-4 w-4 mr-1" />
														<span className="text-sm">Inactive</span>
													</div>
												)}
											</td>
											<td className="px-6 py-4 whitespace-nowrap">
												<div className="flex items-center text-sm text-secondary-500 dark:text-white">
													<Calendar className="h-4 w-4 mr-2" />
													{formatDateOnly(user.created_at)}
												</div>
											</td>
											<td className="px-6 py-4 whitespace-nowrap text-sm text-secondary-500 dark:text-white">
												{user.last_login ? (
													formatDateOnly(user.last_login)
												) : (
													<span className="text-secondary-400">Never</span>
												)}
											</td>
											<td className="px-2 py-4 whitespace-nowrap text-right text-sm font-medium">
												<div className="grid grid-cols-2 gap-x-3 gap-y-2 justify-items-center w-fit ml-auto">
													<button
														type="button"
														onClick={() => handleEditUser(user)}
														className="text-secondary-400 hover:text-secondary-600 dark:text-white dark:hover:text-secondary-300"
														title="Edit user"
													>
														<Edit className="h-4 w-4" />
													</button>
													<button
														type="button"
														onClick={() => handleResetPassword(user)}
														className="text-blue-400 hover:text-blue-600 dark:text-blue-500 dark:hover:text-blue-300 disabled:text-gray-300 disabled:cursor-not-allowed"
														title={
															!user.is_active
																? "Cannot reset password for inactive user"
																: "Reset password"
														}
														disabled={!user.is_active}
													>
														<Key className="h-4 w-4" />
													</button>
													{user.tfa_enabled &&
														canResetTfaOf(roles, currentUser, user) && (
															<button
																type="button"
																onClick={() => setResetTfaUser(user)}
																className="text-warning-500 hover:text-warning-700 dark:text-warning-400 dark:hover:text-warning-300"
																title="Reset two-factor authentication"
																aria-label={`Reset two-factor authentication for ${user.username}`}
															>
																<ShieldOff className="h-4 w-4" />
															</button>
														)}
													<button
														type="button"
														onClick={() =>
															handleDeleteUser(user.id, user.username)
														}
														className="col-start-2 text-danger-400 hover:text-danger-600 dark:text-danger-500 dark:hover:text-danger-400 disabled:text-gray-300 disabled:cursor-not-allowed"
														title={
															user.id === currentUser?.id
																? "Cannot delete your own account"
																: user.role === "admin" &&
																		users.filter((u) => u.role === "admin")
																			.length === 1
																	? "Cannot delete the last admin user"
																	: "Delete user"
														}
														disabled={
															user.id === currentUser?.id ||
															(user.role === "admin" &&
																users.filter((u) => u.role === "admin")
																	.length === 1)
														}
													>
														<Trash2 className="h-4 w-4" />
													</button>
												</div>
											</td>
										</tr>
									))}
								</tbody>
							</table>
						</div>
					</>
				) : (
					<div className="p-12 text-center">
						<User className="h-12 w-12 text-secondary-400 mx-auto mb-4" />
						<p className="text-secondary-500 dark:text-white">No users found</p>
						<p className="text-sm text-secondary-400 dark:text-white mt-2">
							Click "Add User" to create the first user
						</p>
					</div>
				)}
			</div>

			{/* Add User Modal - only hidden when OIDC sync roles is active */}
			{!isOIDCSyncRoles && (
				<AddUserModal
					isOpen={showAddModal}
					onClose={() => setShowAddModal(false)}
					onUserCreated={handleUserCreated}
					roles={roles}
				/>
			)}

			{/* Edit User Modal */}
			{editingUser && (
				<EditUserModal
					user={editingUser}
					isOpen={!!editingUser}
					onClose={() => setEditingUser(null)}
					onUpdateUser={updateUserMutation.mutateAsync}
					isLoading={updateUserMutation.isPending}
					onUnlinkOidc={unlinkOidcMutation.mutateAsync}
					isUnlinkingOidc={unlinkOidcMutation.isPending}
					roles={roles}
				/>
			)}

			{/* Reset Password Modal */}
			{resetPasswordUser && (
				<ResetPasswordModal
					user={resetPasswordUser}
					isOpen={!!resetPasswordUser}
					onClose={() => setResetPasswordUser(null)}
					onPasswordReset={resetPasswordMutation.mutateAsync}
					isLoading={resetPasswordMutation.isPending}
				/>
			)}

			{resetTfaUser && (
				<ResetTfaModal
					user={resetTfaUser}
					onClose={() => setResetTfaUser(null)}
					onReset={resetTfaMutation.mutateAsync}
					onSuccess={handleTfaReset}
					isLoading={resetTfaMutation.isPending}
					actorHasPassword={currentUser?.has_password !== false}
					actorTfaEnabled={currentUserTfaEnabled}
				/>
			)}

			{/* OIDC Info Banner - only show when OIDC sync roles is active */}
			{isOIDCSyncRoles && (
				<div className="bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-lg p-4">
					<div className="flex">
						<Info className="h-5 w-5 text-blue-500 dark:text-blue-400 flex-shrink-0" />
						<div className="ml-3">
							<h3 className="text-sm font-medium text-blue-800 dark:text-blue-200">
								OIDC Authentication Enabled
							</h3>
							<p className="mt-1 text-sm text-blue-700 dark:text-blue-300">
								User management is handled by your Identity Provider (IdP).
								Users are created automatically when they log in via OIDC. Roles
								are assigned based on IdP group membership configured in your
								environment variables.
							</p>
						</div>
					</div>
				</div>
			)}

			{/* User Registration Settings - hidden in managed/multi-context mode or when OIDC sync roles is active */}
			{!isAdminMode && !isOIDCSyncRoles && (
				<div className="bg-white dark:bg-secondary-800 shadow overflow-hidden sm:rounded-lg">
					<div className="px-6 py-4 border-b border-secondary-200 dark:border-secondary-600">
						<h3 className="text-lg font-medium text-secondary-900 dark:text-white">
							User Registration Settings
						</h3>
					</div>
					<div className="px-6 py-4 space-y-4">
						{/* User Signup Setting */}
						<div>
							<label className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-2">
								<div className="flex items-center gap-2">
									<input
										id={signupEnabledId}
										type="checkbox"
										checked={signupFormData.signupEnabled}
										onChange={(e) =>
											handleSignupInputChange("signupEnabled", e.target.checked)
										}
										className="rounded border-secondary-300 text-primary-600 shadow-sm focus:border-primary-300 focus:ring focus:ring-primary-200 focus:ring-opacity-50"
										disabled={settingsLoading}
									/>
									<label htmlFor={signupEnabledId}>
										Enable User Self-Registration
									</label>
								</div>
							</label>

							{/* Default User Role Dropdown */}
							{signupFormData.signupEnabled && (
								<div className="mt-3 ml-6">
									<label
										htmlFor={defaultRoleId}
										className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-2"
									>
										Default Role for New Users
									</label>
									<select
										id={defaultRoleId}
										value={signupFormData.defaultUserRole}
										onChange={(e) =>
											handleSignupInputChange("defaultUserRole", e.target.value)
										}
										className="w-full max-w-xs border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
										disabled={rolesLoading}
									>
										{rolesLoading ? (
											<option>Loading roles...</option>
										) : roles && Array.isArray(roles) ? (
											roles.map((role) => (
												<option key={role.role} value={role.role}>
													{role.role.charAt(0).toUpperCase() +
														role.role.slice(1)}
												</option>
											))
										) : (
											<option value="user">User</option>
										)}
									</select>
									<p className="mt-1 text-xs text-secondary-500 dark:text-white">
										New users will be assigned this role when they register.
									</p>
								</div>
							)}

							<p className="mt-1 text-sm text-secondary-500 dark:text-white">
								When enabled, users can create their own accounts through the
								signup page. When disabled, only administrators can create user
								accounts.
							</p>
						</div>

						{/* Security Notice */}
						<div className="bg-blue-50 dark:bg-blue-900 border border-blue-200 dark:border-blue-700 rounded-md p-4">
							<div className="flex">
								<Shield className="h-5 w-5 text-blue-400 dark:text-blue-300" />
								<div className="ml-3">
									<h3 className="text-sm font-medium text-blue-800 dark:text-blue-200">
										Security Notice
									</h3>
									<p className="mt-1 text-sm text-blue-700 dark:text-blue-300">
										When enabling user self-registration, exercise caution on
										internal networks. Consider restricting access to trusted
										networks only and ensure proper role assignments to prevent
										unauthorized access to sensitive systems.
									</p>
								</div>
							</div>
						</div>

						{/* Save Button */}
						<div className="flex justify-end">
							<button
								type="button"
								onClick={handleSignupSave}
								disabled={
									!isSignupDirty || updateSignupSettingsMutation.isPending
								}
								className={`inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md shadow-sm text-white ${
									!isSignupDirty || updateSignupSettingsMutation.isPending
										? "bg-secondary-400 cursor-not-allowed"
										: "bg-primary-600 hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500"
								}`}
							>
								{updateSignupSettingsMutation.isPending ? (
									<>
										<div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white mr-2"></div>
										Saving...
									</>
								) : (
									<>
										<Save className="h-4 w-4 mr-2" />
										Save Settings
									</>
								)}
							</button>
						</div>

						{updateSignupSettingsMutation.isSuccess && (
							<div className="bg-green-50 dark:bg-green-900 border border-green-200 dark:border-green-700 rounded-md p-4">
								<div className="flex">
									<CheckCircle className="h-5 w-5 text-green-400 dark:text-green-300" />
									<div className="ml-3">
										<p className="text-sm text-green-700 dark:text-green-300">
											Settings saved successfully!
										</p>
									</div>
								</div>
							</div>
						)}
					</div>
				</div>
			)}
		</div>
	);
};

// Add User Modal Component
const AddUserModal = ({ isOpen, onClose, onUserCreated, roles }) => {
	const usernameId = useId();
	const emailId = useId();
	const firstNameId = useId();
	const lastNameId = useId();
	const passwordId = useId();
	const roleId = useId();

	const [formData, setFormData] = useState({
		username: "",
		email: "",
		password: "",
		first_name: "",
		last_name: "",
		role: "user",
	});
	const [isLoading, setIsLoading] = useState(false);
	const [error, setError] = useState("");
	const [success, setSuccess] = useState(false);

	// Reset form when modal is closed
	useEffect(() => {
		if (!isOpen) {
			setFormData({
				username: "",
				email: "",
				password: "",
				first_name: "",
				last_name: "",
				role: "user",
			});
			setError("");
			setSuccess(false);
		}
	}, [isOpen]);

	const handleSubmit = async (e) => {
		e.preventDefault();
		setIsLoading(true);
		setError("");
		setSuccess(false);

		try {
			// Only send role if roles are available from API
			const payload = {
				username: formData.username,
				email: formData.email,
				password: formData.password,
				first_name: formData.first_name,
				last_name: formData.last_name,
			};
			if (roles && Array.isArray(roles) && roles.length > 0) {
				payload.role = formData.role;
			}
			await adminUsersAPI.create(payload);
			setSuccess(true);
			onUserCreated();
			// Auto-close after 1.5 seconds
			setTimeout(() => {
				onClose();
			}, 1500);
		} catch (err) {
			setError(err.response?.data?.error || "Failed to create user");
		} finally {
			setIsLoading(false);
		}
	};

	const handleInputChange = (e) => {
		setFormData({
			...formData,
			[e.target.name]: e.target.value,
		});
	};

	if (!isOpen) return null;

	const modal = (
		<div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-[120]">
			<div className="bg-white dark:bg-secondary-800 rounded-lg p-6 w-full max-w-md">
				<h3 className="text-lg font-medium text-secondary-900 dark:text-white mb-4">
					Add New User
				</h3>

				<form onSubmit={handleSubmit} className="space-y-4">
					<div>
						<label
							htmlFor={usernameId}
							className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
						>
							Username
						</label>
						<input
							id={usernameId}
							type="text"
							name="username"
							required
							value={formData.username}
							onChange={handleInputChange}
							className="block w-full border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
						/>
					</div>

					<div>
						<label
							htmlFor={emailId}
							className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
						>
							Email
						</label>
						<input
							id={emailId}
							type="email"
							name="email"
							required
							value={formData.email}
							onChange={handleInputChange}
							className="block w-full border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
						/>
					</div>

					<div className="grid grid-cols-2 gap-4">
						<div>
							<label
								htmlFor={firstNameId}
								className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
							>
								First Name
							</label>
							<input
								id={firstNameId}
								type="text"
								name="first_name"
								value={formData.first_name}
								onChange={handleInputChange}
								className="block w-full border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
							/>
						</div>
						<div>
							<label
								htmlFor={lastNameId}
								className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
							>
								Last Name
							</label>
							<input
								id={lastNameId}
								type="text"
								name="last_name"
								value={formData.last_name}
								onChange={handleInputChange}
								className="block w-full border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
							/>
						</div>
					</div>

					<div>
						<label
							htmlFor={passwordId}
							className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
						>
							Password
						</label>
						<input
							id={passwordId}
							type="password"
							name="password"
							required
							minLength={6}
							value={formData.password}
							onChange={handleInputChange}
							className="block w-full border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
						/>
						<p className="mt-1 text-xs text-secondary-500 dark:text-white">
							Minimum 6 characters
						</p>
					</div>

					<div>
						<label
							htmlFor={roleId}
							className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
						>
							Role
						</label>
						<select
							id={roleId}
							name="role"
							value={formData.role}
							onChange={handleInputChange}
							className="block w-full border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
						>
							{roles && Array.isArray(roles) && roles.length > 0 ? (
								roles.map((role) => (
									<option key={role.role} value={role.role}>
										{role.role.charAt(0).toUpperCase() +
											role.role.slice(1).replace("_", " ")}
									</option>
								))
							) : (
								<>
									<option value="user">User</option>
									<option value="admin">Admin</option>
								</>
							)}
						</select>
					</div>

					{success && (
						<div className="bg-green-50 dark:bg-green-900 border border-green-200 dark:border-green-700 rounded-md p-3">
							<div className="flex items-center">
								<CheckCircle className="h-4 w-4 text-green-600 dark:text-green-400 mr-2" />
								<p className="text-sm text-green-700 dark:text-green-300">
									User created successfully!
								</p>
							</div>
						</div>
					)}

					{error && (
						<div className="bg-danger-50 dark:bg-danger-900 border border-danger-200 dark:border-danger-700 rounded-md p-3">
							<p className="text-sm text-danger-700 dark:text-danger-300">
								{error}
							</p>
						</div>
					)}

					<div className="flex justify-end space-x-3">
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
							{isLoading ? "Creating..." : "Create User"}
						</button>
					</div>
				</form>
			</div>
		</div>
	);

	return <ModalPortal>{modal}</ModalPortal>;
};

// Edit User Modal Component
const EditUserModal = ({
	user,
	isOpen,
	onClose,
	onUpdateUser,
	isLoading,
	onUnlinkOidc,
	isUnlinkingOidc,
	roles,
}) => {
	const confirm = useConfirm();
	const toast = useToast();
	const editUsernameId = useId();
	const editEmailId = useId();
	const editFirstNameId = useId();
	const editLastNameId = useId();
	const editRoleId = useId();
	const editActiveId = useId();

	const [formData, setFormData] = useState({
		username: user?.username || "",
		email: user?.email || "",
		first_name: user?.first_name || "",
		last_name: user?.last_name || "",
		role: user?.role || "user",
		is_active: user?.is_active ?? true,
	});
	const [error, setError] = useState("");
	const [success, setSuccess] = useState(false);

	// Update formData when user prop changes or modal opens
	useEffect(() => {
		if (user && isOpen) {
			setFormData({
				username: user.username || "",
				email: user.email || "",
				first_name: user.first_name || "",
				last_name: user.last_name || "",
				role: user.role || "user",
				is_active: user.is_active ?? true,
			});
		}
	}, [user, isOpen]);

	// Reset error and success when modal closes
	useEffect(() => {
		if (!isOpen) {
			setError("");
			setSuccess(false);
		}
	}, [isOpen]);

	const handleSubmit = async (e) => {
		e.preventDefault();
		setError("");
		setSuccess(false);

		try {
			await onUpdateUser({ id: user.id, data: formData });
			setSuccess(true);
			// Auto-close after 1.5 seconds
			setTimeout(() => {
				onClose();
			}, 1500);
		} catch (err) {
			setError(err.response?.data?.error || "Failed to update user");
		}
	};

	const handleUnlinkOidc = async () => {
		const confirmed = await confirm({
			title: "Unlink SSO identity",
			subtitle: null,
			message: `Unlink "${user.username}" from their SSO identity? Their next SSO login links the account again, matched by the verified email address your identity provider sends.`,
			warning:
				"If Trust unverified email is on in Settings > OIDC, whoever next signs in with this email address claims the account, whether or not the provider has verified it. Ask the user to sign in straight after you unlink.",
			confirmLabel: "Unlink SSO identity",
		});
		if (!confirmed) return;
		try {
			await onUnlinkOidc(user.id);
			toast.success(`SSO identity unlinked from "${user.username}"`);
		} catch (err) {
			toast.error(err.response?.data?.error || "Failed to unlink SSO identity");
		}
	};

	const handleInputChange = (e) => {
		const { name, value, type, checked } = e.target;
		setFormData({
			...formData,
			[name]: type === "checkbox" ? checked : value,
		});
	};

	if (!isOpen || !user) return null;

	const modal = (
		<div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-[120]">
			<div className="bg-white dark:bg-secondary-800 rounded-lg p-6 w-full max-w-md">
				<h3 className="text-lg font-medium text-secondary-900 dark:text-white mb-4">
					Edit User
				</h3>

				<form onSubmit={handleSubmit} className="space-y-4">
					<div>
						<label
							htmlFor={editUsernameId}
							className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
						>
							Username
						</label>
						<input
							id={editUsernameId}
							type="text"
							name="username"
							required
							value={formData.username}
							onChange={handleInputChange}
							className="block w-full border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
						/>
					</div>

					<div>
						<label
							htmlFor={editEmailId}
							className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
						>
							Email
						</label>
						<input
							id={editEmailId}
							type="email"
							name="email"
							required
							value={formData.email}
							onChange={handleInputChange}
							className="block w-full border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
						/>
					</div>

					<div className="grid grid-cols-2 gap-4">
						<div>
							<label
								htmlFor={editFirstNameId}
								className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
							>
								First Name
							</label>
							<input
								id={editFirstNameId}
								type="text"
								name="first_name"
								value={formData.first_name}
								onChange={handleInputChange}
								className="block w-full border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
							/>
						</div>
						<div>
							<label
								htmlFor={editLastNameId}
								className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
							>
								Last Name
							</label>
							<input
								id={editLastNameId}
								type="text"
								name="last_name"
								value={formData.last_name}
								onChange={handleInputChange}
								className="block w-full border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
							/>
						</div>
					</div>

					<div>
						<label
							htmlFor={editRoleId}
							className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
						>
							Role
						</label>
						<select
							id={editRoleId}
							name="role"
							value={formData.role}
							onChange={handleInputChange}
							className="block w-full border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
						>
							{roles && Array.isArray(roles) ? (
								roles.map((role) => (
									<option key={role.role} value={role.role}>
										{role.role.charAt(0).toUpperCase() +
											role.role.slice(1).replace("_", " ")}
									</option>
								))
							) : (
								<>
									<option value="user">User</option>
									<option value="admin">Admin</option>
								</>
							)}
						</select>
					</div>

					<div className="flex items-center">
						<input
							id={editActiveId}
							type="checkbox"
							name="is_active"
							checked={formData.is_active}
							onChange={handleInputChange}
							className="h-4 w-4 text-primary-600 focus:ring-primary-500 border-secondary-300 rounded"
						/>
						<label
							htmlFor={editActiveId}
							className="ml-2 block text-sm text-secondary-700 dark:text-secondary-200"
						>
							Active user
						</label>
					</div>

					{user.oidc_linked && (
						<div className="border-t border-secondary-200 dark:border-secondary-600 pt-4">
							<p className="text-sm font-medium text-secondary-700 dark:text-secondary-200">
								SSO identity
							</p>
							<p className="mt-1 text-sm text-secondary-500 dark:text-secondary-400">
								Linked
								{user.oidc_provider ? ` via ${user.oidc_provider}` : ""}. Unlink
								it if the user was re-created at the identity provider, or you
								changed provider, and their SSO login is refused.
							</p>
							<button
								type="button"
								onClick={handleUnlinkOidc}
								disabled={isUnlinkingOidc}
								className="btn-danger mt-3 inline-flex items-center gap-2 min-h-[44px] md:min-h-0"
							>
								{isUnlinkingOidc && (
									<RefreshCw className="h-4 w-4 animate-spin" />
								)}
								{isUnlinkingOidc ? "Unlinking..." : "Unlink SSO identity"}
							</button>
						</div>
					)}

					{success && (
						<div className="bg-green-50 dark:bg-green-900 border border-green-200 dark:border-green-700 rounded-md p-3">
							<div className="flex items-center">
								<CheckCircle className="h-4 w-4 text-green-600 dark:text-green-400 mr-2" />
								<p className="text-sm text-green-700 dark:text-green-300">
									User updated successfully!
								</p>
							</div>
						</div>
					)}

					{error && (
						<div className="bg-danger-50 dark:bg-danger-900 border border-danger-200 dark:border-danger-700 rounded-md p-3">
							<p className="text-sm text-danger-700 dark:text-danger-300">
								{error}
							</p>
						</div>
					)}

					<div className="flex justify-end space-x-3">
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
							{isLoading ? "Updating..." : "Update User"}
						</button>
					</div>
				</form>
			</div>
		</div>
	);

	return <ModalPortal>{modal}</ModalPortal>;
};

// Reset Password Modal Component
const ResetPasswordModal = ({
	user,
	isOpen,
	onClose,
	onPasswordReset,
	isLoading,
}) => {
	const newPasswordId = useId();
	const confirmPasswordId = useId();
	const [newPassword, setNewPassword] = useState("");
	const [confirmPassword, setConfirmPassword] = useState("");
	const [error, setError] = useState("");

	const handleSubmit = async (e) => {
		e.preventDefault();
		setError("");

		// Validate passwords
		if (newPassword.length < 6) {
			setError("Password must be at least 6 characters long");
			return;
		}

		if (newPassword !== confirmPassword) {
			setError("Passwords do not match");
			return;
		}

		try {
			await onPasswordReset({ userId: user.id, newPassword });
			// Reset form on success
			setNewPassword("");
			setConfirmPassword("");
		} catch (err) {
			setError(err.response?.data?.error || "Failed to reset password");
		}
	};

	const handleClose = () => {
		setNewPassword("");
		setConfirmPassword("");
		setError("");
		onClose();
	};

	if (!isOpen) return null;

	const modal = (
		<div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center p-4 z-[120]">
			<div className="bg-white dark:bg-secondary-800 rounded-lg p-6 w-full max-w-md">
				<h3 className="text-lg font-medium text-secondary-900 dark:text-white mb-4">
					Reset Password for {user.username}
				</h3>

				<form onSubmit={handleSubmit} className="space-y-4">
					<div>
						<label
							htmlFor={newPasswordId}
							className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
						>
							New Password
						</label>
						<input
							id={newPasswordId}
							type="password"
							required
							minLength={6}
							value={newPassword}
							onChange={(e) => setNewPassword(e.target.value)}
							className="block w-full border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
							placeholder="Enter new password (min 6 characters)"
						/>
					</div>

					<div>
						<label
							htmlFor={confirmPasswordId}
							className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
						>
							Confirm Password
						</label>
						<input
							id={confirmPasswordId}
							type="password"
							required
							value={confirmPassword}
							onChange={(e) => setConfirmPassword(e.target.value)}
							className="block w-full border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
							placeholder="Confirm new password"
						/>
					</div>

					<div className="bg-yellow-50 dark:bg-yellow-900 border border-yellow-200 dark:border-yellow-700 rounded-md p-3">
						<div className="flex">
							<div className="flex-shrink-0">
								<Key className="h-5 w-5 text-yellow-400" />
							</div>
							<div className="ml-3">
								<h3 className="text-sm font-medium text-yellow-800 dark:text-yellow-200">
									Password Reset Warning
								</h3>
								<div className="mt-2 text-sm text-yellow-700 dark:text-yellow-300">
									<p>
										This will immediately change the user's password. The user
										will need to use the new password to login.
									</p>
								</div>
							</div>
						</div>
					</div>

					{error && (
						<div className="bg-danger-50 dark:bg-danger-900 border border-danger-200 dark:border-danger-700 rounded-md p-3">
							<p className="text-sm text-danger-700 dark:text-danger-300">
								{error}
							</p>
						</div>
					)}

					<div className="flex justify-end space-x-3">
						<button
							type="button"
							onClick={handleClose}
							className="px-4 py-2 text-sm font-medium text-secondary-700 dark:text-secondary-200 bg-white dark:bg-secondary-700 border border-secondary-300 dark:border-secondary-600 rounded-md hover:bg-secondary-50 dark:hover:bg-secondary-600"
						>
							Cancel
						</button>
						<button
							type="submit"
							disabled={isLoading}
							className="px-4 py-2 text-sm font-medium text-white bg-primary-600 border border-transparent rounded-md hover:bg-primary-700 disabled:opacity-50 flex items-center"
						>
							{isLoading && (
								<div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white mr-2"></div>
							)}
							{isLoading ? "Resetting..." : "Reset Password"}
						</button>
					</div>
				</form>
			</div>
		</div>
	);

	return <ModalPortal>{modal}</ModalPortal>;
};

const ResetTfaModal = ({
	user,
	onClose,
	onReset,
	onSuccess,
	isLoading,
	actorHasPassword,
	actorTfaEnabled,
}) => {
	const titleId = useId();
	const passwordId = useId();
	const tokenId = useId();
	const tempId = useId();
	const tempConfirmId = useId();
	const [password, setPassword] = useState("");
	const [tfaToken, setTfaToken] = useState("");
	const [setTemp, setSetTemp] = useState(false);
	const [tempPassword, setTempPassword] = useState("");
	const [tempConfirm, setTempConfirm] = useState("");
	const [error, setError] = useState("");
	const askForCode = actorTfaEnabled !== false;

	const handleSubmit = async (e) => {
		e.preventDefault();
		setError("");
		if (!password) {
			setError("Enter your password to confirm");
			return;
		}
		if (actorTfaEnabled && !tfaToken.trim()) {
			setError("Enter your two-factor code to confirm");
			return;
		}
		if (setTemp && !tempPassword) {
			setError("Enter a temporary password for the user");
			return;
		}
		if (setTemp && tempPassword !== tempConfirm) {
			setError("The temporary passwords do not match");
			return;
		}
		try {
			await onReset({
				userId: user.id,
				password,
				tfaToken: tfaToken.trim(),
				newPassword: setTemp ? tempPassword : undefined,
			});
			onSuccess(user.username, setTemp);
		} catch (err) {
			setError(
				err.response?.data?.error ||
					err.response?.data?.message ||
					"Failed to reset two-factor authentication",
			);
		}
	};

	const modal = (
		<div className="fixed inset-0 bg-black/50 flex items-center justify-center p-4 z-[120]">
			<button
				type="button"
				onClick={onClose}
				className="fixed inset-0 cursor-default"
				aria-label="Close"
			/>
			<div
				role="dialog"
				aria-modal="true"
				aria-labelledby={titleId}
				className="bg-white dark:bg-secondary-800 rounded-lg shadow-xl max-w-md w-full relative z-10 max-h-[85vh] flex flex-col"
			>
				<div className="px-6 py-4 border-b border-secondary-200 dark:border-secondary-600 flex-shrink-0">
					<div className="flex items-center justify-between">
						<h3
							id={titleId}
							className="text-lg font-medium text-secondary-900 dark:text-white flex items-center gap-2"
						>
							<ShieldOff className="h-5 w-5 text-warning-600" />
							Reset 2FA for {user.username}
						</h3>
						<button
							type="button"
							onClick={onClose}
							className="p-1 rounded hover:bg-secondary-100 dark:hover:bg-secondary-700 text-secondary-400 hover:text-secondary-600 min-h-[44px] min-w-[44px] md:min-h-0 md:min-w-0 flex items-center justify-center"
							aria-label="Close"
						>
							<X className="h-5 w-5" />
						</button>
					</div>
				</div>

				<form onSubmit={handleSubmit} className="flex flex-col min-h-0">
					<div className="px-6 py-4 space-y-4 overflow-y-auto">
						<div className="p-3 bg-danger-50 dark:bg-danger-900 border border-danger-200 dark:border-danger-700 rounded-md">
							<p className="text-sm text-danger-700 dark:text-danger-300">
								This removes {user.username}'s authenticator and backup codes,
								forgets their trusted devices and signs them out everywhere. The
								next time they sign in with their password they must set up
								two-factor authentication again before they can use PatchMon.
								Accounts that sign in through single sign-on are not asked to
								set it up again.
							</p>
							<p className="mt-2 text-sm font-medium text-danger-700 dark:text-danger-300">
								If you suspect the account is compromised, also set a temporary
								password below.
							</p>
						</div>

						{!actorHasPassword ? (
							<p className="text-sm text-secondary-700 dark:text-secondary-200">
								Your account signs in through single sign-on and has no local
								password, so it cannot confirm this action. Ask an administrator
								with a local password to reset it.
							</p>
						) : (
							<>
								<div>
									<label
										htmlFor={passwordId}
										className="block text-sm font-medium text-secondary-700 dark:text-white mb-1"
									>
										Your password
									</label>
									<input
										id={passwordId}
										type="password"
										autoComplete="current-password"
										value={password}
										onChange={(e) => setPassword(e.target.value)}
										className="w-full px-3 py-2 bg-white dark:bg-secondary-900 border border-secondary-300 dark:border-secondary-600 rounded-md text-secondary-900 dark:text-white focus:ring-2 focus:ring-primary-500 focus:border-primary-500 placeholder-secondary-400"
									/>
								</div>
								{askForCode && (
									<div>
										<label
											htmlFor={tokenId}
											className="block text-sm font-medium text-secondary-700 dark:text-white mb-1"
										>
											Your two-factor code
											{actorTfaEnabled === undefined && " (if enabled)"}
										</label>
										<input
											id={tokenId}
											type="text"
											inputMode="text"
											autoComplete="one-time-code"
											maxLength={6}
											value={tfaToken}
											onChange={(e) => setTfaToken(e.target.value)}
											placeholder="6-digit or backup code"
											className="w-full px-3 py-2 bg-white dark:bg-secondary-900 border border-secondary-300 dark:border-secondary-600 rounded-md text-secondary-900 dark:text-white focus:ring-2 focus:ring-primary-500 focus:border-primary-500 placeholder-secondary-400 font-mono"
										/>
									</div>
								)}
								<div className="pt-2 border-t border-secondary-200 dark:border-secondary-600">
									<button
										type="button"
										aria-pressed={setTemp}
										onClick={() => setSetTemp((v) => !v)}
										className="flex items-start gap-2 text-left min-h-[44px] md:min-h-0"
									>
										{setTemp ? (
											<CheckSquare className="h-5 w-5 text-primary-600 flex-shrink-0" />
										) : (
											<Square className="h-5 w-5 text-secondary-400 flex-shrink-0" />
										)}
										<span>
											<span className="block text-sm font-medium text-secondary-900 dark:text-white">
												Also set a new temporary password
											</span>
											<span className="block text-xs text-secondary-500 dark:text-secondary-300">
												Their current password stops working. They must replace
												the temporary one when they next sign in, before setting
												up two-factor authentication.
											</span>
										</span>
									</button>
								</div>
								{setTemp && (
									<>
										<div>
											<label
												htmlFor={tempId}
												className="block text-sm font-medium text-secondary-700 dark:text-white mb-1"
											>
												Temporary password
											</label>
											<input
												id={tempId}
												type="password"
												autoComplete="new-password"
												value={tempPassword}
												onChange={(e) => setTempPassword(e.target.value)}
												className="w-full px-3 py-2 bg-white dark:bg-secondary-900 border border-secondary-300 dark:border-secondary-600 rounded-md text-secondary-900 dark:text-white focus:ring-2 focus:ring-primary-500 focus:border-primary-500 placeholder-secondary-400"
											/>
										</div>
										<div>
											<label
												htmlFor={tempConfirmId}
												className="block text-sm font-medium text-secondary-700 dark:text-white mb-1"
											>
												Confirm temporary password
											</label>
											<input
												id={tempConfirmId}
												type="password"
												autoComplete="new-password"
												value={tempConfirm}
												onChange={(e) => setTempConfirm(e.target.value)}
												className="w-full px-3 py-2 bg-white dark:bg-secondary-900 border border-secondary-300 dark:border-secondary-600 rounded-md text-secondary-900 dark:text-white focus:ring-2 focus:ring-primary-500 focus:border-primary-500 placeholder-secondary-400"
											/>
										</div>
										<p className="text-xs text-secondary-500 dark:text-secondary-300">
											Give it to the user over a separate, secure channel.
										</p>
									</>
								)}
							</>
						)}

						{error && (
							<p
								role="alert"
								className="text-sm text-danger-600 dark:text-danger-400"
							>
								{error}
							</p>
						)}
					</div>

					<div className="px-6 py-4 border-t border-secondary-200 dark:border-secondary-600 flex flex-col-reverse sm:flex-row sm:justify-end gap-3 flex-shrink-0">
						<button
							type="button"
							onClick={onClose}
							className="btn-outline min-h-[44px] md:min-h-0"
						>
							Cancel
						</button>
						<button
							type="submit"
							disabled={isLoading || !actorHasPassword}
							className="btn-danger min-h-[44px] md:min-h-0 disabled:opacity-50 disabled:cursor-not-allowed"
						>
							{isLoading && <RefreshCw className="h-4 w-4 mr-2 animate-spin" />}
							{isLoading ? "Resetting..." : "Reset 2FA"}
						</button>
					</div>
				</form>
			</div>
		</div>
	);

	return <ModalPortal>{modal}</ModalPortal>;
};

export default UsersTab;
