import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
	AlertCircle,
	CheckCircle,
	Clock,
	Copy,
	Download,
	Eye,
	EyeOff,
	Key,
	LogOut,
	Mail,
	MapPin,
	Monitor,
	Moon,
	RefreshCw,
	Save,
	Shield,
	Smartphone,
	Sun,
	Trash2,
	User,
} from "lucide-react";

import { useEffect, useId, useState } from "react";

import { useAuth } from "../contexts/AuthContext";
import { THEME_PRESETS, useColorTheme } from "../contexts/ColorThemeContext";
import { useTheme } from "../contexts/ThemeContext";
import { isCorsError, tfaAPI } from "../utils/api";

const Profile = () => {
	const usernameId = useId();
	const emailId = useId();
	const firstNameId = useId();
	const lastNameId = useId();
	const currentPasswordId = useId();
	const newPasswordId = useId();
	const confirmPasswordId = useId();
	const { user, updateProfile, changePassword } = useAuth();
	const { toggleTheme, isDark } = useTheme();
	const { colorTheme, setColorTheme } = useColorTheme();
	const [activeTab, setActiveTab] = useState("profile");
	const [isLoading, setIsLoading] = useState(false);
	const [message, setMessage] = useState({ type: "", text: "" });

	const [profileData, setProfileData] = useState({
		username: user?.username || "",
		email: user?.email || "",
		first_name: user?.first_name || "",
		last_name: user?.last_name || "",
	});

	// Update profileData when user data changes
	useEffect(() => {
		if (user) {
			setProfileData({
				username: user.username || "",
				email: user.email || "",
				first_name: user.first_name || "",
				last_name: user.last_name || "",
			});
		}
	}, [user]);

	const [passwordData, setPasswordData] = useState({
		currentPassword: "",
		newPassword: "",
		confirmPassword: "",
	});

	const [showPasswords, setShowPasswords] = useState({
		current: false,
		new: false,
		confirm: false,
	});

	const handleProfileSubmit = async (e) => {
		e.preventDefault();
		setIsLoading(true);
		setMessage({ type: "", text: "" });

		console.log("Submitting profile data:", profileData);
		try {
			const result = await updateProfile(profileData);
			console.log("Profile update result:", result);
			if (result.success) {
				setMessage({ type: "success", text: "Profile updated successfully!" });
			} else {
				setMessage({
					type: "error",
					text: result.error || "Failed to update profile",
				});
			}
		} catch (error) {
			if (isCorsError(error)) {
				setMessage({
					type: "error",
					text: "CORS_ORIGIN mismatch - please set your URL in your environment variable",
				});
			} else {
				setMessage({ type: "error", text: "Network error occurred" });
			}
		} finally {
			setIsLoading(false);
		}
	};

	const handlePasswordSubmit = async (e) => {
		e.preventDefault();
		setIsLoading(true);
		setMessage({ type: "", text: "" });

		if (passwordData.newPassword !== passwordData.confirmPassword) {
			setMessage({ type: "error", text: "New passwords do not match" });
			setIsLoading(false);
			return;
		}

		if (passwordData.newPassword.length < 6) {
			setMessage({
				type: "error",
				text: "New password must be at least 6 characters",
			});
			setIsLoading(false);
			return;
		}

		try {
			const result = await changePassword(
				passwordData.currentPassword,
				passwordData.newPassword,
			);
			if (result.success) {
				setMessage({ type: "success", text: "Password changed successfully!" });
				setPasswordData({
					currentPassword: "",
					newPassword: "",
					confirmPassword: "",
				});
			} else {
				setMessage({
					type: "error",
					text: result.error || "Failed to change password",
				});
			}
		} catch (error) {
			if (isCorsError(error)) {
				setMessage({
					type: "error",
					text: "CORS_ORIGIN mismatch - please set your URL in your environment variable",
				});
			} else {
				setMessage({ type: "error", text: "Network error occurred" });
			}
		} finally {
			setIsLoading(false);
		}
	};

	const handleInputChange = (e) => {
		const { name, value } = e.target;
		if (activeTab === "profile") {
			setProfileData((prev) => ({ ...prev, [name]: value }));
		} else {
			setPasswordData((prev) => ({ ...prev, [name]: value }));
		}
	};

	const togglePasswordVisibility = (field) => {
		setShowPasswords((prev) => ({ ...prev, [field]: !prev[field] }));
	};

	const tabs = [
		{ id: "profile", name: "Profile Information", icon: User },
		{ id: "password", name: "Change Password", icon: Key },
		{ id: "tfa", name: "Multi-Factor Authentication", icon: Smartphone },
		{ id: "sessions", name: "Active Sessions", icon: Monitor },
	];

	return (
		<div className="space-y-6">
			{/* Header */}
			<div>
				<p className="text-sm text-secondary-600 dark:text-secondary-300">
					Manage your account information and security settings
				</p>
			</div>

			{/* User Info Card */}
			<div className="bg-white dark:bg-secondary-800 shadow rounded-lg p-4 md:p-6">
				<div className="flex items-center space-x-3 md:space-x-4">
					<div className="flex-shrink-0">
						<div className="h-12 w-12 md:h-16 md:w-16 rounded-full bg-primary-100 dark:bg-primary-900 flex items-center justify-center">
							<User className="h-6 w-6 md:h-8 md:w-8 text-primary-600 dark:text-primary-400" />
						</div>
					</div>
					<div className="flex-1 min-w-0">
						<h3 className="text-base md:text-lg font-medium text-secondary-900 dark:text-white truncate">
							{user?.first_name && user?.last_name
								? `${user.first_name} ${user.last_name}`
								: user?.first_name || user?.username}
						</h3>
						<p className="text-sm text-secondary-600 dark:text-secondary-300 truncate">
							{user?.email}
						</p>
						<div className="mt-2">
							<span
								className={`inline-flex items-center px-2.5 py-0.5 rounded-md text-xs font-medium ${
									user?.role === "admin"
										? "bg-primary-100 text-primary-800 dark:bg-primary-900 dark:text-primary-200"
										: user?.role === "host_manager"
											? "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200"
											: user?.role === "readonly"
												? "bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200"
												: "bg-secondary-100 text-secondary-800 dark:bg-secondary-700 dark:text-secondary-200"
								}`}
							>
								<Shield className="h-3 w-3 mr-1" />
								{user?.role?.charAt(0).toUpperCase() +
									user?.role?.slice(1).replace("_", " ")}
							</span>
						</div>
					</div>
				</div>
			</div>

			{/* Tabs */}
			<div className="bg-white dark:bg-secondary-800 shadow rounded-lg">
				{/* Mobile Button Navigation */}
				<div className="md:hidden p-4 space-y-2 border-b border-secondary-200 dark:border-secondary-600">
					{tabs.map((tab) => {
						const Icon = tab.icon;
						return (
							<button
								type="button"
								key={tab.id}
								onClick={() => setActiveTab(tab.id)}
								className={`w-full flex items-center justify-between px-4 py-3 rounded-md font-medium text-sm transition-colors ${
									activeTab === tab.id
										? "bg-primary-50 dark:bg-primary-900/20 text-primary-600 dark:text-primary-400 border border-primary-200 dark:border-primary-800"
										: "bg-secondary-50 dark:bg-secondary-700 text-secondary-700 dark:text-secondary-300 border border-secondary-200 dark:border-secondary-600 hover:bg-secondary-100 dark:hover:bg-secondary-600"
								}`}
							>
								<div className="flex items-center space-x-3">
									<Icon className="h-5 w-5" />
									<span>{tab.name}</span>
								</div>
								{activeTab === tab.id && (
									<CheckCircle className="h-5 w-5 text-primary-600 dark:text-primary-400" />
								)}
							</button>
						);
					})}
				</div>

				{/* Desktop Tab Navigation */}
				<div className="hidden md:block border-b border-secondary-200 dark:border-secondary-600">
					<nav className="-mb-px flex space-x-8 px-6">
						{tabs.map((tab) => {
							const Icon = tab.icon;
							return (
								<button
									type="button"
									key={tab.id}
									onClick={() => setActiveTab(tab.id)}
									className={`py-4 px-1 border-b-2 font-medium text-sm flex items-center ${
										activeTab === tab.id
											? "border-primary-500 text-primary-600 dark:text-primary-400"
											: "border-transparent text-secondary-500 dark:text-secondary-400 hover:text-secondary-700 dark:hover:text-secondary-300 hover:border-secondary-300 dark:hover:border-secondary-500"
									}`}
								>
									<Icon className="h-4 w-4 mr-2" />
									{tab.name}
								</button>
							);
						})}
					</nav>
				</div>

				<div className="p-4 md:p-6">
					{/* Success/Error Message */}
					{message.text && (
						<div
							className={`mb-6 rounded-md p-4 ${
								message.type === "success"
									? "bg-green-50 dark:bg-green-900 border border-green-200 dark:border-green-700"
									: "bg-red-50 dark:bg-red-900 border border-red-200 dark:border-red-700"
							}`}
						>
							<div className="flex">
								{message.type === "success" ? (
									<CheckCircle className="h-5 w-5 text-green-400 dark:text-green-300" />
								) : (
									<AlertCircle className="h-5 w-5 text-red-400 dark:text-red-300" />
								)}
								<div className="ml-3">
									<p
										className={`text-sm font-medium ${
											message.type === "success"
												? "text-green-800 dark:text-green-200"
												: "text-red-800 dark:text-red-200"
										}`}
									>
										{message.text}
									</p>
								</div>
							</div>
						</div>
					)}

					{/* Profile Information Tab */}
					{activeTab === "profile" && (
						<form onSubmit={handleProfileSubmit} className="space-y-6">
							<div>
								<h3 className="text-lg font-medium text-secondary-900 dark:text-white mb-4">
									Profile Information
								</h3>
								<div className="grid grid-cols-1 gap-6 sm:grid-cols-2">
									<div>
										<label
											htmlFor={usernameId}
											className="block text-sm font-medium text-secondary-700 dark:text-secondary-200"
										>
											Username
										</label>
										<div className="mt-1 relative">
											<input
												type="text"
												name="username"
												id={usernameId}
												value={profileData.username}
												onChange={handleInputChange}
												className="block w-full border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 pl-10 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
												required
											/>
											<User className="absolute left-3 top-1/2 transform -translate-y-1/2 h-4 w-4 text-secondary-400 dark:text-secondary-500" />
										</div>
									</div>

									<div>
										<label
											htmlFor={emailId}
											className="block text-sm font-medium text-secondary-700 dark:text-secondary-200"
										>
											Email Address
										</label>
										<div className="mt-1 relative">
											<input
												type="email"
												name="email"
												id={emailId}
												value={profileData.email}
												onChange={handleInputChange}
												className="block w-full border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 pl-10 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
												required
											/>
											<Mail className="absolute left-3 top-1/2 transform -translate-y-1/2 h-4 w-4 text-secondary-400 dark:text-secondary-500" />
										</div>
									</div>

									<div>
										<label
											htmlFor={firstNameId}
											className="block text-sm font-medium text-secondary-700 dark:text-secondary-200"
										>
											First Name
										</label>
										<div className="mt-1">
											<input
												type="text"
												name="first_name"
												id={firstNameId}
												value={profileData.first_name}
												onChange={handleInputChange}
												className="block w-full border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
											/>
										</div>
									</div>

									<div>
										<label
											htmlFor={lastNameId}
											className="block text-sm font-medium text-secondary-700 dark:text-secondary-200"
										>
											Last Name
										</label>
										<div className="mt-1">
											<input
												type="text"
												name="last_name"
												id={lastNameId}
												value={profileData.last_name}
												onChange={handleInputChange}
												className="block w-full border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
											/>
										</div>
									</div>
								</div>
							</div>

							{/* Theme Settings */}
							<div className="border-t border-secondary-200 dark:border-secondary-600 pt-4 md:pt-6">
								<h4 className="text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-3">
									Appearance
								</h4>
								<div className="max-w-md">
									<div className="flex items-center justify-between gap-3">
										<div className="flex items-center space-x-2 md:space-x-3 flex-1 min-w-0">
											<div className="flex-shrink-0">
												{isDark ? (
													<Moon className="h-5 w-5 text-secondary-600 dark:text-secondary-400" />
												) : (
													<Sun className="h-5 w-5 text-secondary-600 dark:text-secondary-400" />
												)}
											</div>
											<div className="min-w-0">
												<p className="text-sm font-medium text-secondary-900 dark:text-white truncate">
													{isDark ? "Dark Mode" : "Light Mode"}
												</p>
												<p className="text-xs text-secondary-500 dark:text-secondary-400 truncate">
													{isDark
														? "Switch to light mode"
														: "Switch to dark mode"}
												</p>
											</div>
										</div>
										<button
											type="button"
											onClick={toggleTheme}
											className={`relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-md border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 ${
												isDark ? "bg-primary-600" : "bg-secondary-200"
											}`}
											role="switch"
											aria-checked={isDark}
										>
											<span
												aria-hidden="true"
												className={`pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out ${
													isDark ? "translate-x-5" : "translate-x-0"
												}`}
											/>
										</button>
									</div>
								</div>

								{/* Color Theme Settings */}
								<div className="mt-4 md:mt-6 pt-4 md:pt-6 border-t border-secondary-200 dark:border-secondary-600">
									<h4 className="text-sm font-medium text-secondary-900 dark:text-white mb-2">
										Color Theme
									</h4>
									<p className="text-xs text-secondary-500 dark:text-secondary-400 mb-4">
										Choose your preferred color scheme for the application
									</p>

									<div className="grid grid-cols-2 md:grid-cols-3 gap-3 md:gap-4">
										{Object.entries(THEME_PRESETS).map(([themeKey, theme]) => {
											const isSelected = colorTheme === themeKey;
											const gradientColors = theme.login.xColors;

											return (
												<button
													key={themeKey}
													type="button"
													onClick={() => setColorTheme(themeKey)}
													className={`relative p-4 rounded-lg border-2 transition-all ${
														isSelected
															? "border-primary-500 ring-2 ring-primary-200 dark:ring-primary-800"
															: "border-secondary-200 dark:border-secondary-600 hover:border-primary-300"
													} cursor-pointer`}
												>
													{/* Theme Preview */}
													<div
														className="h-20 rounded-md mb-3 overflow-hidden"
														style={{
															background: `linear-gradient(135deg, ${gradientColors.join(", ")})`,
														}}
													/>

													{/* Theme Name */}
													<div className="text-sm font-medium text-secondary-900 dark:text-white mb-1">
														{theme.name}
													</div>

													{/* Selected Indicator */}
													{isSelected && (
														<div className="absolute top-2 right-2 bg-primary-500 text-white rounded-full p-1">
															<svg
																className="w-4 h-4"
																fill="currentColor"
																viewBox="0 0 20 20"
																aria-label="Selected theme"
															>
																<title>Selected</title>
																<path
																	fillRule="evenodd"
																	d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z"
																	clipRule="evenodd"
																/>
															</svg>
														</div>
													)}
												</button>
											);
										})}
									</div>
								</div>
							</div>

							<div className="flex justify-end">
								<button
									type="submit"
									disabled={isLoading}
									className="inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md text-white bg-primary-600 hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500 disabled:opacity-50 w-full sm:w-auto justify-center sm:justify-end"
								>
									<Save className="h-4 w-4 mr-2" />
									{isLoading ? "Saving..." : "Save Changes"}
								</button>
							</div>
						</form>
					)}

					{/* Change Password Tab */}
					{activeTab === "password" && (
						<form onSubmit={handlePasswordSubmit} className="space-y-6">
							<div>
								<h3 className="text-lg font-medium text-secondary-900 dark:text-white mb-4">
									Change Password
								</h3>
								<div className="space-y-4">
									<div>
										<label
											htmlFor={currentPasswordId}
											className="block text-sm font-medium text-secondary-700 dark:text-secondary-200"
										>
											Current Password
										</label>
										<div className="mt-1 relative">
											<input
												type={showPasswords.current ? "text" : "password"}
												name="currentPassword"
												id={currentPasswordId}
												value={passwordData.currentPassword}
												onChange={handleInputChange}
												className="block w-full border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 pl-10 pr-10 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
												required
											/>
											<Key className="absolute left-3 top-1/2 transform -translate-y-1/2 h-4 w-4 text-secondary-400 dark:text-secondary-500" />
											<button
												type="button"
												onClick={() => togglePasswordVisibility("current")}
												className="absolute right-3 top-1/2 transform -translate-y-1/2 text-secondary-400 dark:text-secondary-500 hover:text-secondary-600 dark:hover:text-secondary-300"
											>
												{showPasswords.current ? (
													<EyeOff className="h-4 w-4" />
												) : (
													<Eye className="h-4 w-4" />
												)}
											</button>
										</div>
									</div>

									<div>
										<label
											htmlFor={newPasswordId}
											className="block text-sm font-medium text-secondary-700 dark:text-secondary-200"
										>
											New Password
										</label>
										<div className="mt-1 relative">
											<input
												type={showPasswords.new ? "text" : "password"}
												name="newPassword"
												id={newPasswordId}
												value={passwordData.newPassword}
												onChange={handleInputChange}
												className="block w-full border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 pl-10 pr-10 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
												required
												minLength="6"
											/>
											<Key className="absolute left-3 top-1/2 transform -translate-y-1/2 h-4 w-4 text-secondary-400 dark:text-secondary-500" />
											<button
												type="button"
												onClick={() => togglePasswordVisibility("new")}
												className="absolute right-3 top-1/2 transform -translate-y-1/2 text-secondary-400 dark:text-secondary-500 hover:text-secondary-600 dark:hover:text-secondary-300"
											>
												{showPasswords.new ? (
													<EyeOff className="h-4 w-4" />
												) : (
													<Eye className="h-4 w-4" />
												)}
											</button>
										</div>
										<p className="mt-1 text-xs text-secondary-500 dark:text-secondary-400">
											Must be at least 6 characters long
										</p>
									</div>

									<div>
										<label
											htmlFor={confirmPasswordId}
											className="block text-sm font-medium text-secondary-700 dark:text-secondary-200"
										>
											Confirm New Password
										</label>
										<div className="mt-1 relative">
											<input
												type={showPasswords.confirm ? "text" : "password"}
												name="confirmPassword"
												id={confirmPasswordId}
												value={passwordData.confirmPassword}
												onChange={handleInputChange}
												className="block w-full border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 pl-10 pr-10 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
												required
												minLength="6"
											/>
											<Key className="absolute left-3 top-1/2 transform -translate-y-1/2 h-4 w-4 text-secondary-400 dark:text-secondary-500" />
											<button
												type="button"
												onClick={() => togglePasswordVisibility("confirm")}
												className="absolute right-3 top-1/2 transform -translate-y-1/2 text-secondary-400 dark:text-secondary-500 hover:text-secondary-600 dark:hover:text-secondary-300"
											>
												{showPasswords.confirm ? (
													<EyeOff className="h-4 w-4" />
												) : (
													<Eye className="h-4 w-4" />
												)}
											</button>
										</div>
									</div>
								</div>
							</div>

							<div className="flex justify-end">
								<button
									type="submit"
									disabled={isLoading}
									className="inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md text-white bg-primary-600 hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500 disabled:opacity-50 w-full sm:w-auto justify-center sm:justify-end"
								>
									<Key className="h-4 w-4 mr-2" />
									{isLoading ? "Changing..." : "Change Password"}
								</button>
							</div>
						</form>
					)}

					{/* Multi-Factor Authentication Tab */}
					{activeTab === "tfa" && <TfaTab />}

					{/* Sessions Tab */}
					{activeTab === "sessions" && <SessionsTab />}
				</div>
			</div>
		</div>
	);
};

// TFA Tab Component
const TfaTab = () => {
	const verificationTokenId = useId();
	const disablePasswordId = useId();
	const [setupStep, setSetupStep] = useState("status"); // 'status', 'setup', 'verify', 'backup-codes'
	const [verificationToken, setVerificationToken] = useState("");
	const [password, setPassword] = useState("");
	const [backupCodes, setBackupCodes] = useState([]);
	const [message, setMessage] = useState({ type: "", text: "" });
	const queryClient = useQueryClient();

	// Fetch TFA status
	const { data: tfaStatus, isLoading: statusLoading } = useQuery({
		queryKey: ["tfaStatus"],
		queryFn: () => tfaAPI.status().then((res) => res.data),
	});

	// Setup TFA mutation
	const setupMutation = useMutation({
		mutationFn: () => tfaAPI.setup().then((res) => res.data),
		onSuccess: () => {
			setSetupStep("setup");
			setMessage({
				type: "info",
				text: "Scan the QR code with your authenticator app and enter the verification code below.",
			});
		},
		onError: (error) => {
			setMessage({
				type: "error",
				text: error.response?.data?.error || "Failed to setup TFA",
			});
		},
	});

	// Verify setup mutation
	const verifyMutation = useMutation({
		mutationFn: (data) => tfaAPI.verifySetup(data).then((res) => res.data),
		onSuccess: (data) => {
			setBackupCodes(data.backupCodes);
			setSetupStep("backup-codes");
			setMessage({
				type: "success",
				text: "Two-factor authentication has been enabled successfully!",
			});
		},
		onError: (error) => {
			setMessage({
				type: "error",
				text: error.response?.data?.error || "Failed to verify TFA setup",
			});
		},
	});

	// Disable TFA mutation
	const disableMutation = useMutation({
		mutationFn: (data) => tfaAPI.disable(data).then((res) => res.data),
		onSuccess: () => {
			queryClient.invalidateQueries(["tfaStatus"]);
			setSetupStep("status");
			setMessage({
				type: "success",
				text: "Two-factor authentication has been disabled successfully!",
			});
		},
		onError: (error) => {
			setMessage({
				type: "error",
				text: error.response?.data?.error || "Failed to disable TFA",
			});
		},
	});

	// Regenerate backup codes mutation
	const regenerateBackupCodesMutation = useMutation({
		mutationFn: () => tfaAPI.regenerateBackupCodes().then((res) => res.data),
		onSuccess: (data) => {
			setBackupCodes(data.backupCodes);
			setMessage({
				type: "success",
				text: "Backup codes have been regenerated successfully!",
			});
		},
		onError: (error) => {
			setMessage({
				type: "error",
				text:
					error.response?.data?.error || "Failed to regenerate backup codes",
			});
		},
	});

	const handleSetup = () => {
		setupMutation.mutate();
	};

	const handleVerify = (e) => {
		e.preventDefault();
		if (verificationToken.length !== 6) {
			setMessage({
				type: "error",
				text: "Please enter a 6-digit verification code",
			});
			return;
		}
		verifyMutation.mutate({ token: verificationToken });
	};

	const handleDisable = (e) => {
		e.preventDefault();
		if (!password) {
			setMessage({
				type: "error",
				text: "Please enter your password to disable TFA",
			});
			return;
		}
		disableMutation.mutate({ password });
	};

	const handleRegenerateBackupCodes = () => {
		regenerateBackupCodesMutation.mutate();
	};

	const copyToClipboard = async (text) => {
		try {
			// Try modern clipboard API first
			if (navigator.clipboard && window.isSecureContext) {
				await navigator.clipboard.writeText(text);
				setMessage({ type: "success", text: "Copied to clipboard!" });
				return;
			}

			// Fallback for older browsers or non-secure contexts
			const textArea = document.createElement("textarea");
			textArea.value = text;
			textArea.style.position = "fixed";
			textArea.style.left = "-999999px";
			textArea.style.top = "-999999px";
			document.body.appendChild(textArea);
			textArea.focus();
			textArea.select();

			try {
				const successful = document.execCommand("copy");
				if (successful) {
					setMessage({ type: "success", text: "Copied to clipboard!" });
				} else {
					throw new Error("Copy command failed");
				}
			} catch {
				// If all else fails, show the text in a prompt
				prompt("Copy this text:", text);
				setMessage({
					type: "info",
					text: "Text shown in prompt for manual copying",
				});
			} finally {
				document.body.removeChild(textArea);
			}
		} catch (err) {
			console.error("Failed to copy to clipboard:", err);
			// Show the text in a prompt as last resort
			prompt("Copy this text:", text);
			setMessage({
				type: "info",
				text: "Text shown in prompt for manual copying",
			});
		}
	};

	const downloadBackupCodes = () => {
		const content = `PatchMon Backup Codes\n\n${backupCodes.map((code, index) => `${index + 1}. ${code}`).join("\n")}\n\nKeep these codes safe! Each code can only be used once.`;
		const blob = new Blob([content], { type: "text/plain" });
		const url = URL.createObjectURL(blob);
		const a = document.createElement("a");
		a.href = url;
		a.download = "patchmon-backup-codes.txt";
		document.body.appendChild(a);
		a.click();
		document.body.removeChild(a);
		URL.revokeObjectURL(url);
	};

	if (statusLoading) {
		return (
			<div className="flex items-center justify-center h-64">
				<div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600"></div>
			</div>
		);
	}

	return (
		<div className="space-y-6">
			<div>
				<h3 className="text-lg font-medium text-secondary-900 dark:text-white mb-4">
					Multi-Factor Authentication
				</h3>
				<p className="text-sm text-secondary-600 dark:text-secondary-300 mb-6">
					Add an extra layer of security to your account by enabling two-factor
					authentication.
				</p>
			</div>

			{/* Status Message */}
			{message.text && (
				<div
					className={`rounded-md p-4 ${
						message.type === "success"
							? "bg-green-50 dark:bg-green-900 border border-green-200 dark:border-green-700"
							: message.type === "error"
								? "bg-red-50 dark:bg-red-900 border border-red-200 dark:border-red-700"
								: "bg-blue-50 dark:bg-blue-900 border border-blue-200 dark:border-blue-700"
					}`}
				>
					<div className="flex">
						{message.type === "success" ? (
							<CheckCircle className="h-5 w-5 text-green-400 dark:text-green-300" />
						) : message.type === "error" ? (
							<AlertCircle className="h-5 w-5 text-red-400 dark:text-red-300" />
						) : (
							<AlertCircle className="h-5 w-5 text-blue-400 dark:text-blue-300" />
						)}
						<div className="ml-3">
							<p
								className={`text-sm font-medium ${
									message.type === "success"
										? "text-green-800 dark:text-green-200"
										: message.type === "error"
											? "text-red-800 dark:text-red-200"
											: "text-blue-800 dark:text-blue-200"
								}`}
							>
								{message.text}
							</p>
						</div>
					</div>
				</div>
			)}

			{/* TFA Status */}
			{setupStep === "status" && (
				<div className="space-y-4 md:space-y-6">
					<div className="bg-white dark:bg-secondary-800 border border-secondary-200 dark:border-secondary-600 rounded-lg p-4 md:p-6">
						<div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
							<div className="flex items-center space-x-3 flex-1 min-w-0">
								<div
									className={`p-2 rounded-full flex-shrink-0 ${tfaStatus?.enabled ? "bg-green-100 dark:bg-green-900" : "bg-secondary-100 dark:bg-secondary-700"}`}
								>
									<Smartphone
										className={`h-5 w-5 md:h-6 md:w-6 ${tfaStatus?.enabled ? "text-green-600 dark:text-green-400" : "text-secondary-600 dark:text-secondary-400"}`}
									/>
								</div>
								<div className="min-w-0">
									<h4 className="text-base md:text-lg font-medium text-secondary-900 dark:text-white">
										{tfaStatus?.enabled
											? "Two-Factor Authentication Enabled"
											: "Two-Factor Authentication Disabled"}
									</h4>
									<p className="text-sm text-secondary-600 dark:text-secondary-300">
										{tfaStatus?.enabled
											? "Your account is protected with two-factor authentication."
											: "Add an extra layer of security to your account."}
									</p>
								</div>
							</div>
							<div className="flex-shrink-0">
								{tfaStatus?.enabled ? (
									<button
										type="button"
										onClick={() => setSetupStep("disable")}
										className="btn-outline text-danger-600 border-danger-300 hover:bg-danger-50 w-full sm:w-auto"
									>
										<Trash2 className="h-4 w-4 mr-2" />
										Disable TFA
									</button>
								) : (
									<button
										type="button"
										onClick={handleSetup}
										disabled={setupMutation.isPending}
										className="btn-primary w-full sm:w-auto"
									>
										<Smartphone className="h-4 w-4 mr-2" />
										{setupMutation.isPending ? "Setting up..." : "Enable TFA"}
									</button>
								)}
							</div>
						</div>
					</div>

					{tfaStatus?.enabled && (
						<div className="bg-white dark:bg-secondary-800 border border-secondary-200 dark:border-secondary-600 rounded-lg p-4 md:p-6">
							<h4 className="text-base md:text-lg font-medium text-secondary-900 dark:text-white mb-3 md:mb-4">
								Backup Codes
							</h4>
							<p className="text-sm text-secondary-600 dark:text-secondary-300 mb-4">
								Use these backup codes to access your account if you lose your
								authenticator device.
							</p>
							<button
								type="button"
								onClick={handleRegenerateBackupCodes}
								disabled={regenerateBackupCodesMutation.isPending}
								className="btn-outline w-full sm:w-auto"
							>
								<RefreshCw
									className={`h-4 w-4 mr-2 ${regenerateBackupCodesMutation.isPending ? "animate-spin" : ""}`}
								/>
								{regenerateBackupCodesMutation.isPending
									? "Regenerating..."
									: "Regenerate Codes"}
							</button>
						</div>
					)}
				</div>
			)}

			{/* TFA Setup */}
			{setupStep === "setup" && setupMutation.data && (
				<div className="space-y-4 md:space-y-6">
					<div className="bg-white dark:bg-secondary-800 border border-secondary-200 dark:border-secondary-600 rounded-lg p-4 md:p-6">
						<h4 className="text-base md:text-lg font-medium text-secondary-900 dark:text-white mb-4">
							Setup Two-Factor Authentication
						</h4>
						<div className="space-y-4">
							<div className="text-center">
								<img
									src={setupMutation.data.qrCode}
									alt="QR Code"
									className="mx-auto h-40 w-40 md:h-48 md:w-48 border border-secondary-200 dark:border-secondary-600 rounded-lg"
								/>
								<p className="text-sm text-secondary-600 dark:text-secondary-300 mt-2">
									Scan this QR code with your authenticator app
								</p>
							</div>

							<div className="bg-secondary-50 dark:bg-secondary-700 p-3 md:p-4 rounded-lg">
								<p className="text-sm font-medium text-secondary-900 dark:text-white mb-2">
									Manual Entry Key:
								</p>
								<div className="flex items-center gap-2">
									<code className="flex-1 bg-white dark:bg-secondary-800 px-2 md:px-3 py-2 rounded border text-xs md:text-sm font-mono break-all">
										{setupMutation.data.manualEntryKey}
									</code>
									<button
										type="button"
										onClick={() =>
											copyToClipboard(setupMutation.data.manualEntryKey)
										}
										className="p-2 text-secondary-400 hover:text-secondary-600 dark:hover:text-secondary-300 flex-shrink-0"
										title="Copy to clipboard"
									>
										<Copy className="h-4 w-4" />
									</button>
								</div>
							</div>

							<div className="text-center">
								<button
									type="button"
									onClick={() => setSetupStep("verify")}
									className="btn-primary w-full sm:w-auto"
								>
									Continue to Verification
								</button>
							</div>
						</div>
					</div>
				</div>
			)}

			{/* TFA Verification */}
			{setupStep === "verify" && (
				<div className="space-y-4 md:space-y-6">
					<div className="bg-white dark:bg-secondary-800 border border-secondary-200 dark:border-secondary-600 rounded-lg p-4 md:p-6">
						<h4 className="text-base md:text-lg font-medium text-secondary-900 dark:text-white mb-4">
							Verify Setup
						</h4>
						<p className="text-sm text-secondary-600 dark:text-secondary-300 mb-4">
							Enter the 6-digit code from your authenticator app to complete the
							setup.
						</p>
						<form onSubmit={handleVerify} className="space-y-4">
							<div>
								<label
									htmlFor={verificationTokenId}
									className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
								>
									Verification Code
								</label>
								<input
									id={verificationTokenId}
									type="text"
									value={verificationToken}
									onChange={(e) =>
										setVerificationToken(
											e.target.value.replace(/\D/g, "").slice(0, 6),
										)
									}
									placeholder="000000"
									className="w-full px-3 py-2 border border-secondary-300 dark:border-secondary-600 rounded-md focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white text-center text-base md:text-lg font-mono tracking-widest"
									maxLength="6"
									required
								/>
							</div>
							<div className="flex flex-col sm:flex-row gap-3">
								<button
									type="submit"
									disabled={
										verifyMutation.isPending || verificationToken.length !== 6
									}
									className="btn-primary w-full sm:w-auto"
								>
									{verifyMutation.isPending
										? "Verifying..."
										: "Verify & Enable"}
								</button>
								<button
									type="button"
									onClick={() => setSetupStep("status")}
									className="btn-outline w-full sm:w-auto"
								>
									Cancel
								</button>
							</div>
						</form>
					</div>
				</div>
			)}

			{/* Backup Codes */}
			{setupStep === "backup-codes" && backupCodes.length > 0 && (
				<div className="space-y-4 md:space-y-6">
					<div className="bg-white dark:bg-secondary-800 border border-secondary-200 dark:border-secondary-600 rounded-lg p-4 md:p-6">
						<h4 className="text-base md:text-lg font-medium text-secondary-900 dark:text-white mb-4">
							Backup Codes
						</h4>
						<p className="text-sm text-secondary-600 dark:text-secondary-300 mb-4">
							Save these backup codes in a safe place. Each code can only be
							used once.
						</p>
						<div className="bg-secondary-50 dark:bg-secondary-700 p-3 md:p-4 rounded-lg mb-4">
							<div className="grid grid-cols-1 sm:grid-cols-2 gap-2 font-mono text-xs md:text-sm">
								{backupCodes.map((code, index) => (
									<div
										key={code}
										className="flex items-center justify-between py-1"
									>
										<span className="text-secondary-600 dark:text-secondary-400">
											{index + 1}.
										</span>
										<span className="text-secondary-900 dark:text-white break-all ml-2">
											{code}
										</span>
									</div>
								))}
							</div>
						</div>
						<div className="flex flex-col sm:flex-row gap-3">
							<button
								type="button"
								onClick={downloadBackupCodes}
								className="btn-outline w-full sm:w-auto"
							>
								<Download className="h-4 w-4 mr-2" />
								Download Codes
							</button>
							<button
								type="button"
								onClick={() => {
									setSetupStep("status");
									queryClient.invalidateQueries(["tfaStatus"]);
								}}
								className="btn-primary w-full sm:w-auto"
							>
								Done
							</button>
						</div>
					</div>
				</div>
			)}

			{/* Disable TFA */}
			{setupStep === "disable" && (
				<div className="space-y-4 md:space-y-6">
					<div className="bg-white dark:bg-secondary-800 border border-secondary-200 dark:border-secondary-600 rounded-lg p-4 md:p-6">
						<h4 className="text-base md:text-lg font-medium text-secondary-900 dark:text-white mb-4">
							Disable Two-Factor Authentication
						</h4>
						<p className="text-sm text-secondary-600 dark:text-secondary-300 mb-4">
							Enter your password to disable two-factor authentication.
						</p>
						<form onSubmit={handleDisable} className="space-y-4">
							<div>
								<label
									htmlFor={disablePasswordId}
									className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
								>
									Password
								</label>
								<input
									id={disablePasswordId}
									type="password"
									value={password}
									onChange={(e) => setPassword(e.target.value)}
									className="w-full px-3 py-2 border border-secondary-300 dark:border-secondary-600 rounded-md focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
									required
								/>
							</div>
							<div className="flex flex-col sm:flex-row gap-3">
								<button
									type="submit"
									disabled={disableMutation.isPending || !password}
									className="btn-danger w-full sm:w-auto"
								>
									{disableMutation.isPending ? "Disabling..." : "Disable TFA"}
								</button>
								<button
									type="button"
									onClick={() => setSetupStep("status")}
									className="btn-outline w-full sm:w-auto"
								>
									Cancel
								</button>
							</div>
						</form>
					</div>
				</div>
			)}
		</div>
	);
};

// Sessions Tab Component
const SessionsTab = () => {
	const _queryClient = useQueryClient();
	const [_isLoading, _setIsLoading] = useState(false);
	const [message, setMessage] = useState({ type: "", text: "" });

	// Fetch user sessions
	const {
		data: sessionsData,
		isLoading: sessionsLoading,
		refetch,
	} = useQuery({
		queryKey: ["user-sessions"],
		queryFn: async () => {
			const response = await fetch("/api/v1/auth/sessions", {
				headers: {
					Authorization: `Bearer ${localStorage.getItem("token")}`,
				},
			});
			if (!response.ok) throw new Error("Failed to fetch sessions");
			return response.json();
		},
	});

	// Revoke individual session mutation
	const revokeSessionMutation = useMutation({
		mutationFn: async (sessionId) => {
			const response = await fetch(`/api/v1/auth/sessions/${sessionId}`, {
				method: "DELETE",
				headers: {
					Authorization: `Bearer ${localStorage.getItem("token")}`,
				},
			});
			if (!response.ok) throw new Error("Failed to revoke session");
			return response.json();
		},
		onSuccess: () => {
			setMessage({ type: "success", text: "Session revoked successfully" });
			refetch();
		},
		onError: (error) => {
			setMessage({ type: "error", text: error.message });
		},
	});

	// Revoke all sessions mutation
	const revokeAllSessionsMutation = useMutation({
		mutationFn: async () => {
			const response = await fetch("/api/v1/auth/sessions", {
				method: "DELETE",
				headers: {
					Authorization: `Bearer ${localStorage.getItem("token")}`,
				},
			});
			if (!response.ok) throw new Error("Failed to revoke sessions");
			return response.json();
		},
		onSuccess: () => {
			setMessage({
				type: "success",
				text: "All other sessions revoked successfully",
			});
			refetch();
		},
		onError: (error) => {
			setMessage({ type: "error", text: error.message });
		},
	});

	const formatDate = (dateString) => {
		return new Date(dateString).toLocaleString();
	};

	const formatRelativeTime = (dateString) => {
		const now = new Date();
		const date = new Date(dateString);
		const diff = now - date;
		const minutes = Math.floor(diff / 60000);
		const hours = Math.floor(diff / 3600000);
		const days = Math.floor(diff / 86400000);

		if (days > 0) return `${days} day${days > 1 ? "s" : ""} ago`;
		if (hours > 0) return `${hours} hour${hours > 1 ? "s" : ""} ago`;
		if (minutes > 0) return `${minutes} minute${minutes > 1 ? "s" : ""} ago`;
		return "Just now";
	};

	const handleRevokeSession = (sessionId) => {
		if (window.confirm("Are you sure you want to revoke this session?")) {
			revokeSessionMutation.mutate(sessionId);
		}
	};

	const handleRevokeAllSessions = () => {
		if (
			window.confirm(
				"Are you sure you want to revoke all other sessions? This will log you out of all other devices.",
			)
		) {
			revokeAllSessionsMutation.mutate();
		}
	};

	return (
		<div className="space-y-6">
			{/* Header */}
			<div>
				<h3 className="text-lg font-medium text-secondary-900 dark:text-secondary-100">
					Active Sessions
				</h3>
				<p className="text-sm text-secondary-600 dark:text-secondary-300">
					Manage your active sessions and devices. You can see where you're
					logged in and revoke access for any device.
				</p>
			</div>

			{/* Message */}
			{message.text && (
				<div
					className={`rounded-md p-4 ${
						message.type === "success"
							? "bg-success-50 border border-success-200 text-success-700"
							: "bg-danger-50 border border-danger-200 text-danger-700"
					}`}
				>
					<div className="flex">
						{message.type === "success" ? (
							<CheckCircle className="h-5 w-5" />
						) : (
							<AlertCircle className="h-5 w-5" />
						)}
						<div className="ml-3">
							<p className="text-sm">{message.text}</p>
						</div>
					</div>
				</div>
			)}

			{/* Sessions List */}
			{sessionsLoading ? (
				<div className="flex items-center justify-center py-8">
					<div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600"></div>
				</div>
			) : sessionsData?.sessions?.length > 0 ? (
				<div className="space-y-4">
					{/* Revoke All Button */}
					{sessionsData.sessions.filter((s) => !s.is_current_session).length >
						0 && (
						<div className="flex justify-end">
							<button
								type="button"
								onClick={handleRevokeAllSessions}
								disabled={revokeAllSessionsMutation.isPending}
								className="inline-flex items-center px-4 py-2 border border-danger-300 text-sm font-medium rounded-md text-danger-700 bg-white hover:bg-danger-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-danger-500 disabled:opacity-50 w-full sm:w-auto justify-center sm:justify-end"
							>
								<LogOut className="h-4 w-4 mr-2" />
								{revokeAllSessionsMutation.isPending
									? "Revoking..."
									: "Revoke All Other Sessions"}
							</button>
						</div>
					)}

					{/* Sessions */}
					{sessionsData.sessions.map((session) => (
						<div
							key={session.id}
							className={`border rounded-lg p-3 md:p-4 ${
								session.is_current_session
									? "border-primary-200 bg-primary-50 dark:border-primary-800 dark:bg-primary-900/20"
									: "border-secondary-200 bg-white dark:border-secondary-700 dark:bg-secondary-800"
							}`}
						>
							<div className="flex items-start justify-between gap-3">
								<div className="flex-1 min-w-0">
									<div className="flex items-start space-x-2 md:space-x-3">
										<Monitor className="h-4 w-4 md:h-5 md:w-5 text-secondary-500 flex-shrink-0 mt-0.5" />
										<div className="flex-1 min-w-0">
											<div className="flex flex-wrap items-center gap-2">
												<h4 className="text-sm font-medium text-secondary-900 dark:text-secondary-100">
													{session.device_info?.browser} on{" "}
													{session.device_info?.os}
												</h4>
												{session.is_current_session && (
													<span className="inline-flex items-center px-2 py-1 rounded-md text-xs font-medium bg-primary-100 text-primary-800 dark:bg-primary-900 dark:text-primary-200 flex-shrink-0">
														Current Session
													</span>
												)}
												{session.tfa_remember_me && (
													<span className="inline-flex items-center px-2 py-1 rounded-md text-xs font-medium bg-success-100 text-success-800 dark:bg-success-900 dark:text-success-200 flex-shrink-0">
														Remembered
													</span>
												)}
											</div>
											<p className="text-sm text-secondary-600 dark:text-secondary-400 mt-1">
												{session.device_info?.device} • {session.ip_address}
											</p>
										</div>
									</div>

									<div className="mt-3 space-y-2 text-sm text-secondary-600 dark:text-secondary-400">
										<div className="flex items-center space-x-2">
											<MapPin className="h-4 w-4 flex-shrink-0" />
											<span className="truncate">
												{session.location_info?.city},{" "}
												{session.location_info?.country}
											</span>
										</div>
										<div className="flex items-center space-x-2">
											<Clock className="h-4 w-4 flex-shrink-0" />
											<span>
												Last active: {formatRelativeTime(session.last_activity)}
											</span>
										</div>
										<div className="text-xs md:text-sm">
											<span>Created: {formatDate(session.created_at)}</span>
										</div>
										<div className="text-xs md:text-sm">
											<span>Login count: {session.login_count}</span>
										</div>
									</div>
								</div>

								{!session.is_current_session && (
									<button
										type="button"
										onClick={() => handleRevokeSession(session.id)}
										disabled={revokeSessionMutation.isPending}
										className="inline-flex items-center px-3 py-2 border border-danger-300 text-sm font-medium rounded-md text-danger-700 bg-white hover:bg-danger-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-danger-500 disabled:opacity-50 flex-shrink-0"
									>
										<LogOut className="h-4 w-4" />
									</button>
								)}
							</div>
						</div>
					))}
				</div>
			) : (
				<div className="text-center py-8">
					<Monitor className="mx-auto h-12 w-12 text-secondary-400" />
					<h3 className="mt-2 text-sm font-medium text-secondary-900 dark:text-secondary-100">
						No active sessions
					</h3>
					<p className="mt-1 text-sm text-secondary-600 dark:text-secondary-400">
						You don't have any active sessions at the moment.
					</p>
				</div>
			)}
		</div>
	);
};

export default Profile;
