import { AlertCircle, CheckCircle, Shield, UserPlus } from "lucide-react";
import { useId, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useAuth } from "../contexts/AuthContext";
import { isCorsError } from "../utils/api";

// Development-only logging
const isDev = import.meta.env.DEV;
const devLog = (...args) => isDev && console.log(...args);
const devError = (...args) => isDev && console.error(...args);

const FirstTimeAdminSetup = () => {
	const { login, setAuthState } = useAuth();
	const navigate = useNavigate();
	const firstNameId = useId();
	const lastNameId = useId();
	const usernameId = useId();
	const emailId = useId();
	const passwordId = useId();
	const confirmPasswordId = useId();
	const [formData, setFormData] = useState({
		username: "",
		email: "",
		password: "",
		confirmPassword: "",
		firstName: "",
		lastName: "",
	});
	const [isLoading, setIsLoading] = useState(false);
	const [error, setError] = useState("");
	const [success, setSuccess] = useState(false);

	const handleInputChange = (e) => {
		const { name, value } = e.target;
		setFormData((prev) => ({
			...prev,
			[name]: value,
		}));
		// Clear error when user starts typing
		if (error) setError("");
	};

	const validateForm = () => {
		if (!formData.firstName.trim()) {
			setError("First name is required");
			return false;
		}
		if (!formData.lastName.trim()) {
			setError("Last name is required");
			return false;
		}
		if (!formData.username.trim()) {
			setError("Username is required");
			return false;
		}
		if (!formData.email.trim()) {
			setError("Email address is required");
			return false;
		}

		// Enhanced email validation
		const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
		if (!emailRegex.test(formData.email.trim())) {
			setError("Please enter a valid email address (e.g., user@example.com)");
			return false;
		}

		if (formData.password.length < 8) {
			setError("Password must be at least 8 characters for security");
			return false;
		}
		if (formData.password !== formData.confirmPassword) {
			setError("Passwords do not match");
			return false;
		}
		return true;
	};

	const handleSubmit = async (e) => {
		e.preventDefault();

		if (!validateForm()) return;

		setIsLoading(true);
		setError("");

		try {
			const response = await fetch("/api/v1/auth/setup-admin", {
				method: "POST",
				headers: {
					"Content-Type": "application/json",
				},
				body: JSON.stringify({
					username: formData.username.trim(),
					email: formData.email.trim(),
					password: formData.password,
					firstName: formData.firstName.trim(),
					lastName: formData.lastName.trim(),
				}),
			});

			const data = await response.json();

			if (response.ok) {
				setSuccess(true);

				// If the response includes a token, use it to automatically log in
				if (data.token && data.user) {
					// Set the authentication state immediately
					setAuthState(data.token, data.user);
					// Navigate to dashboard after successful setup
					setTimeout(() => {
						navigate("/", { replace: true });
					}, 100); // Small delay to ensure auth state is set
				} else {
					// Fallback to manual login if no token provided
					setTimeout(async () => {
						try {
							await login(formData.username.trim(), formData.password);
						} catch (error) {
							devError("Auto-login failed:", error);
							setError(
								"Account created but auto-login failed. Please login manually.",
							);
							setSuccess(false);
						}
					}, 2000);
				}
			} else {
				// Handle HTTP error responses (like 500 CORS errors)
				devLog("HTTP error response:", response.status, data);

				// Check if this is a CORS error based on the response data
				if (
					data.message?.includes("Not allowed by CORS") ||
					data.message?.includes("CORS") ||
					data.error?.includes("CORS")
				) {
					setError(
						"CORS_ORIGIN mismatch - please set your URL in your environment variable",
					);
				} else {
					setError(data.error || "Failed to create admin user");
				}
			}
		} catch (error) {
			devError("Setup error:", error);
			// Check for CORS/network errors first
			if (isCorsError(error)) {
				setError(
					"CORS_ORIGIN mismatch - please set your URL in your environment variable",
				);
			} else if (
				error.name === "TypeError" &&
				error.message?.includes("Failed to fetch")
			) {
				setError(
					"CORS_ORIGIN mismatch - please set your URL in your environment variable",
				);
			} else {
				setError("Network error. Please try again.");
			}
		} finally {
			setIsLoading(false);
		}
	};

	if (success) {
		return (
			<div className="min-h-screen bg-gradient-to-br from-primary-50 to-secondary-50 dark:from-secondary-900 dark:to-secondary-800 flex items-center justify-center p-4">
				<div className="max-w-md w-full">
					<div className="card p-8 text-center">
						<div className="flex justify-center mb-6">
							<div className="bg-green-100 dark:bg-green-900 p-4 rounded-full">
								<CheckCircle className="h-12 w-12 text-green-600 dark:text-green-400" />
							</div>
						</div>
						<h1 className="text-2xl font-bold text-secondary-900 dark:text-white mb-4">
							Admin Account Created!
						</h1>
						<p className="text-secondary-600 dark:text-secondary-300 mb-6">
							Your admin account has been successfully created and you are now
							logged in. Redirecting to the dashboard...
						</p>
						<div className="flex justify-center">
							<div className="animate-spin rounded-full h-6 w-6 border-b-2 border-primary-600"></div>
						</div>
					</div>
				</div>
			</div>
		);
	}

	return (
		<div className="min-h-screen bg-gradient-to-br from-primary-50 to-secondary-50 dark:from-secondary-900 dark:to-secondary-800 flex items-center justify-center p-4">
			<div className="max-w-md w-full">
				<div className="card p-8">
					<div className="text-center mb-8">
						<div className="flex justify-center mb-4">
							<div className="bg-primary-100 dark:bg-primary-900 p-4 rounded-full">
								<Shield className="h-12 w-12 text-primary-600 dark:text-primary-400" />
							</div>
						</div>
						<h1 className="text-2xl font-bold text-secondary-900 dark:text-white mb-2">
							Welcome to PatchMon
						</h1>
						<p className="text-secondary-600 dark:text-secondary-300">
							Let's set up your admin account to get started
						</p>
					</div>

					{error && (
						<div className="mb-6 p-4 bg-danger-50 dark:bg-danger-900 border border-danger-200 dark:border-danger-700 rounded-lg">
							<div className="flex items-center">
								<AlertCircle className="h-5 w-5 text-danger-600 dark:text-danger-400 mr-2" />
								<span className="text-danger-700 dark:text-danger-300 text-sm">
									{error}
								</span>
							</div>
						</div>
					)}

					<form onSubmit={handleSubmit} className="space-y-6">
						<div className="grid grid-cols-1 md:grid-cols-2 gap-4">
							<div>
								<label
									htmlFor={firstNameId}
									className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-2"
								>
									First Name
								</label>
								<input
									type="text"
									id={firstNameId}
									name="firstName"
									value={formData.firstName}
									onChange={handleInputChange}
									className="input w-full"
									placeholder="Enter your first name"
									required
									disabled={isLoading}
								/>
							</div>
							<div>
								<label
									htmlFor={lastNameId}
									className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-2"
								>
									Last Name
								</label>
								<input
									type="text"
									id={lastNameId}
									name="lastName"
									value={formData.lastName}
									onChange={handleInputChange}
									className="input w-full"
									placeholder="Enter your last name"
									required
									disabled={isLoading}
								/>
							</div>
						</div>

						<div>
							<label
								htmlFor={usernameId}
								className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-2"
							>
								Username
							</label>
							<input
								type="text"
								id={usernameId}
								name="username"
								value={formData.username}
								onChange={handleInputChange}
								className="input w-full"
								placeholder="Enter your username"
								required
								disabled={isLoading}
							/>
						</div>

						<div>
							<label
								htmlFor={emailId}
								className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-2"
							>
								Email Address
							</label>
							<input
								type="email"
								id={emailId}
								name="email"
								value={formData.email}
								onChange={handleInputChange}
								className="input w-full"
								placeholder="Enter your email"
								required
								disabled={isLoading}
							/>
						</div>

						<div>
							<label
								htmlFor={passwordId}
								className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-2"
							>
								Password
							</label>
							<input
								type="password"
								id={passwordId}
								name="password"
								value={formData.password}
								onChange={handleInputChange}
								className="input w-full"
								placeholder="Enter your password (min 8 characters)"
								required
								disabled={isLoading}
							/>
						</div>

						<div>
							<label
								htmlFor={confirmPasswordId}
								className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-2"
							>
								Confirm Password
							</label>
							<input
								type="password"
								id={confirmPasswordId}
								name="confirmPassword"
								value={formData.confirmPassword}
								onChange={handleInputChange}
								className="input w-full"
								placeholder="Confirm your password"
								required
								disabled={isLoading}
							/>
						</div>

						<button
							type="submit"
							disabled={isLoading}
							className="btn-primary w-full flex items-center justify-center gap-2"
						>
							{isLoading ? (
								<>
									<div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white"></div>
									Creating Admin Account...
								</>
							) : (
								<>
									<UserPlus className="h-4 w-4" />
									Create Admin Account
								</>
							)}
						</button>
					</form>

					<div className="mt-8 p-4 bg-blue-50 dark:bg-blue-900 border border-blue-200 dark:border-blue-700 rounded-lg">
						<div className="flex items-start">
							<Shield className="h-5 w-5 text-blue-600 dark:text-blue-400 mr-2 mt-0.5 flex-shrink-0" />
							<div className="text-sm text-blue-700 dark:text-blue-300">
								<p className="font-medium mb-1">Admin Privileges</p>
								<p>
									This account will have full administrative access to manage
									users, hosts, packages, and system settings.
								</p>
							</div>
						</div>
					</div>
				</div>
			</div>
		</div>
	);
};

export default FirstTimeAdminSetup;
