import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
	Calendar,
	CheckCircle,
	Edit,
	Key,
	Mail,
	Plus,
	Shield,
	Trash2,
	User,
	XCircle,
} from "lucide-react";
import { useId, useState } from "react";
import { useAuth } from "../contexts/AuthContext";
import { adminUsersAPI, permissionsAPI } from "../utils/api";

const Users = () => {
	const [showAddModal, setShowAddModal] = useState(false);
	const [editingUser, setEditingUser] = useState(null);
	const [resetPasswordUser, setResetPasswordUser] = useState(null);
	const queryClient = useQueryClient();
	const { user: currentUser } = useAuth();

	// Fetch users
	const {
		data: users,
		isLoading,
		error,
	} = useQuery({
		queryKey: ["users"],
		queryFn: () => adminUsersAPI.list().then((res) => res.data),
	});

	// Fetch available roles
	const { data: roles } = useQuery({
		queryKey: ["rolePermissions"],
		queryFn: () => permissionsAPI.getRoles().then((res) => res.data),
	});

	// Delete user mutation
	const deleteUserMutation = useMutation({
		mutationFn: adminUsersAPI.delete,
		onSuccess: () => {
			queryClient.invalidateQueries(["users"]);
		},
	});

	// Update user mutation
	const updateUserMutation = useMutation({
		mutationFn: ({ id, data }) => adminUsersAPI.update(id, data),
		onSuccess: () => {
			queryClient.invalidateQueries(["users"]);
			setEditingUser(null);
		},
	});

	// Reset password mutation
	const resetPasswordMutation = useMutation({
		mutationFn: ({ userId, newPassword }) =>
			adminUsersAPI.resetPassword(userId, newPassword),
		onSuccess: () => {
			queryClient.invalidateQueries(["users"]);
			setResetPasswordUser(null);
		},
	});

	const handleDeleteUser = async (userId, username) => {
		if (
			window.confirm(
				`Are you sure you want to delete user "${username}"? This action cannot be undone.`,
			)
		) {
			try {
				await deleteUserMutation.mutateAsync(userId);
			} catch (error) {
				console.error("Failed to delete user:", error);
			}
		}
	};

	const handleUserCreated = () => {
		queryClient.invalidateQueries(["users"]);
		setShowAddModal(false);
	};

	const handleEditUser = (user) => {
		setEditingUser(user);
	};

	const handleResetPassword = (user) => {
		setResetPasswordUser(user);
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
			{/* Header */}
			<div className="flex justify-end items-center">
				<button
					type="button"
					onClick={() => setShowAddModal(true)}
					className="inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md text-white bg-primary-600 hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500"
				>
					<Plus className="h-4 w-4 mr-2" />
					Add User
				</button>
			</div>

			{/* Users Table */}
			<div className="bg-white dark:bg-secondary-800 shadow overflow-hidden sm:rounded-md">
				<ul className="divide-y divide-secondary-200 dark:divide-secondary-600">
					{users && Array.isArray(users) && users.length > 0 ? (
						users.map((user) => (
							<li key={user.id}>
								<div className="px-4 py-4 flex items-center justify-between">
									<div className="flex items-center">
										<div className="flex-shrink-0">
											<div className="h-10 w-10 rounded-full bg-primary-100 flex items-center justify-center">
												<User className="h-5 w-5 text-primary-600" />
											</div>
										</div>
										<div className="ml-4">
											<div className="flex items-center">
												<p className="text-sm font-medium text-secondary-900 dark:text-white">
													{user.username}
												</p>
												{user.id === currentUser?.id && (
													<span className="ml-2 inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-blue-100 text-blue-800">
														You
													</span>
												)}
												<span
													className={`ml-2 inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${
														user.role === "admin"
															? "bg-primary-100 text-primary-800"
															: user.role === "host_manager"
																? "bg-green-100 text-green-800"
																: user.role === "readonly"
																	? "bg-yellow-100 text-yellow-800"
																	: "bg-secondary-100 text-secondary-800"
													}`}
												>
													<Shield className="h-3 w-3 mr-1" />
													{user.role.charAt(0).toUpperCase() +
														user.role.slice(1).replace("_", " ")}
												</span>
												{user.is_active ? (
													<CheckCircle className="ml-2 h-4 w-4 text-green-500" />
												) : (
													<XCircle className="ml-2 h-4 w-4 text-red-500" />
												)}
											</div>
											<div className="flex items-center mt-1 text-sm text-secondary-500 dark:text-secondary-300">
												<Mail className="h-4 w-4 mr-1" />
												{user.email}
											</div>
											<div className="flex items-center mt-1 text-sm text-secondary-500 dark:text-secondary-300">
												<Calendar className="h-4 w-4 mr-1" />
												Created:{" "}
												{new Date(user.created_at).toLocaleDateString()}
												{user.last_login && (
													<>
														<span className="mx-2">•</span>
														Last login:{" "}
														{new Date(user.last_login).toLocaleDateString()}
													</>
												)}
											</div>
										</div>
									</div>
									<div className="flex items-center space-x-2">
										<button
											type="button"
											onClick={() => handleEditUser(user)}
											className="text-secondary-400 hover:text-secondary-600 dark:text-secondary-500 dark:hover:text-secondary-300"
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
										<button
											type="button"
											onClick={() => handleDeleteUser(user.id, user.username)}
											className="text-danger-400 hover:text-danger-600 dark:text-danger-500 dark:hover:text-danger-400 disabled:text-gray-300 disabled:cursor-not-allowed"
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
										</button>
									</div>
								</div>
							</li>
						))
					) : (
						<li>
							<div className="px-4 py-8 text-center">
								<User className="h-12 w-12 text-secondary-400 mx-auto mb-4" />
								<p className="text-secondary-500 dark:text-secondary-300">
									No users found
								</p>
								<p className="text-sm text-secondary-400 dark:text-secondary-400 mt-2">
									Click "Add User" to create the first user
								</p>
							</div>
						</li>
					)}
				</ul>
			</div>

			{/* Add User Modal */}
			<AddUserModal
				isOpen={showAddModal}
				onClose={() => setShowAddModal(false)}
				onUserCreated={handleUserCreated}
				roles={roles}
			/>

			{/* Edit User Modal */}
			{editingUser && (
				<EditUserModal
					user={editingUser}
					isOpen={!!editingUser}
					onClose={() => setEditingUser(null)}
					onUserUpdated={() => updateUserMutation.mutate()}
					roles={roles}
				/>
			)}

			{/* Reset Password Modal */}
			{resetPasswordUser && (
				<ResetPasswordModal
					user={resetPasswordUser}
					isOpen={!!resetPasswordUser}
					onClose={() => setResetPasswordUser(null)}
					onPasswordReset={resetPasswordMutation.mutate}
					isLoading={resetPasswordMutation.isPending}
				/>
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

	const handleSubmit = async (e) => {
		e.preventDefault();
		setIsLoading(true);
		setError("");

		try {
			// Only send role if roles are available from API
			const payload = {
				username: formData.username,
				email: formData.email,
				password: formData.password,
			};
			if (roles && Array.isArray(roles) && roles.length > 0) {
				payload.role = formData.role;
			}
			await adminUsersAPI.create(payload);
			onUserCreated();
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

	return (
		<div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
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
						<p className="mt-1 text-xs text-secondary-500 dark:text-secondary-400">
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
};

// Edit User Modal Component
const EditUserModal = ({ user, isOpen, onClose, onUserUpdated, roles }) => {
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
	const [isLoading, setIsLoading] = useState(false);
	const [error, setError] = useState("");

	const handleSubmit = async (e) => {
		e.preventDefault();
		setIsLoading(true);
		setError("");

		try {
			await adminUsersAPI.update(user.id, formData);
			onUserUpdated();
		} catch (err) {
			setError(err.response?.data?.error || "Failed to update user");
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

	if (!isOpen || !user) return null;

	return (
		<div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
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

	return (
		<div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center p-4 z-50">
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
};

export default Users;
