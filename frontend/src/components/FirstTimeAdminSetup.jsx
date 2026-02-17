import { AlertCircle, CheckCircle, Shield, UserPlus } from "lucide-react";
import { useId, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useAuth } from "../contexts/AuthContext";
import { isCorsError } from "../utils/api";

// Development-only logging
const isDev = import.meta.env.DEV;
const devLog = (...args) => isDev && console.log(...args);
const devError = (...args) => isDev && console.error(...args);

const EMAIL_REGEX = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
const USERNAME_MIN_LENGTH = 2;

// Password rules (match backend defaults; backend may enforce stricter via env)
const get_password_checks = (password) => ({
	length: password.length >= 8,
	uppercase: /[A-Z]/.test(password),
	lowercase: /[a-z]/.test(password),
	number: /[0-9]/.test(password),
	special: /[!@#$%^&*()_+\-=[\]{};':"\\|,.<>/?]/.test(password),
});
const password_strength = (password) => {
	if (!password.length) return 0;
	const c = get_password_checks(password);
	return [c.length, c.uppercase, c.lowercase, c.number, c.special].filter(
		Boolean,
	).length;
};

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
	const [_touched, setTouched] = useState({});
	const [field_errors, setFieldErrors] = useState({});
	const [isLoading, setIsLoading] = useState(false);
	const [error, setError] = useState("");
	const [success, setSuccess] = useState(false);

	const handleInputChange = (e) => {
		const { name, value } = e.target;
		setFormData((prev) => ({ ...prev, [name]: value }));
		if (error) setError("");
		// Clear this field's error when user types
		setFieldErrors((prev) => {
			const next = { ...prev };
			delete next[name];
			return next;
		});
	};

	const handleBlur = (e) => {
		const { name } = e.target;
		setTouched((prev) => ({ ...prev, [name]: true }));
		// Run inline validation for this field
		run_field_validation(name);
	};

	const run_field_validation = (field_name) => {
		const d = formData;
		setFieldErrors((prev) => {
			const next = { ...prev };
			switch (field_name) {
				case "firstName":
					next.firstName = !d.firstName.trim() ? "First name is required" : "";
					break;
				case "lastName":
					next.lastName = !d.lastName.trim() ? "Last name is required" : "";
					break;
				case "username": {
					const u = d.username.trim();
					if (!u) next.username = "Username is required";
					else if (u.length < USERNAME_MIN_LENGTH)
						next.username = `Username must be at least ${USERNAME_MIN_LENGTH} characters`;
					else next.username = "";
					break;
				}
				case "email": {
					const e = d.email.trim();
					if (!e) next.email = "Email address is required";
					else if (!EMAIL_REGEX.test(e))
						next.email = "Enter a valid email (e.g. you@example.com)";
					else next.email = "";
					break;
				}
				case "password": {
					const checks = get_password_checks(d.password);
					if (!d.password) next.password = "Password is required";
					else if (!checks.length)
						next.password = "Password must be at least 8 characters";
					else {
						const missing = [];
						if (!checks.uppercase) missing.push("one uppercase letter");
						if (!checks.lowercase) missing.push("one lowercase letter");
						if (!checks.number) missing.push("one number");
						if (!checks.special) missing.push("one special character");
						next.password =
							missing.length > 0 ? `Add ${missing.join(", ")}` : "";
					}
					break;
				}
				case "confirmPassword":
					if (!d.confirmPassword)
						next.confirmPassword = "Please confirm your password";
					else if (d.password !== d.confirmPassword)
						next.confirmPassword = "Passwords do not match";
					else next.confirmPassword = "";
					break;
				default:
					break;
			}
			if (next[field_name] === "") delete next[field_name];
			return next;
		});
	};

	// Inline messages for fields (show when touched or after submit)
	const get_field_error = (name) => field_errors[name] ?? "";
	const input_error_class = (name) =>
		get_field_error(name)
			? "input w-full border-danger-500 dark:border-danger-400 focus:ring-danger-500"
			: "input w-full";

	const password_checks = get_password_checks(formData.password);
	const password_strength_level = password_strength(formData.password);
	const passwords_match =
		!formData.confirmPassword || formData.password === formData.confirmPassword;
	const confirm_error = formData.confirmPassword && !passwords_match;

	const validateForm = () => {
		const d = formData;
		const next_errors = {};
		if (!d.firstName.trim()) next_errors.firstName = "First name is required";
		if (!d.lastName.trim()) next_errors.lastName = "Last name is required";
		const u = d.username.trim();
		if (!u) next_errors.username = "Username is required";
		else if (u.length < USERNAME_MIN_LENGTH)
			next_errors.username = `Username must be at least ${USERNAME_MIN_LENGTH} characters`;
		const em = d.email.trim();
		if (!em) next_errors.email = "Email address is required";
		else if (!EMAIL_REGEX.test(em))
			next_errors.email = "Enter a valid email (e.g. you@example.com)";
		const pc = get_password_checks(d.password);
		if (!d.password) next_errors.password = "Password is required";
		else if (!pc.length)
			next_errors.password = "Password must be at least 8 characters";
		else {
			const missing = [];
			if (!pc.uppercase) missing.push("one uppercase letter");
			if (!pc.lowercase) missing.push("one lowercase letter");
			if (!pc.number) missing.push("one number");
			if (!pc.special) missing.push("one special character");
			if (missing.length) next_errors.password = `Add ${missing.join(", ")}`;
		}
		if (!d.confirmPassword)
			next_errors.confirmPassword = "Please confirm your password";
		else if (d.password !== d.confirmPassword)
			next_errors.confirmPassword = "Passwords do not match";
		setFieldErrors(next_errors);
		setTouched(
			Object.fromEntries(
				[
					"firstName",
					"lastName",
					"username",
					"email",
					"password",
					"confirmPassword",
				].map((n) => [n, true]),
			),
		);
		return Object.keys(next_errors).length === 0;
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

				if (
					data.message?.includes("Not allowed by CORS") ||
					data.message?.includes("CORS") ||
					data.error?.includes("CORS")
				) {
					setError(
						"CORS_ORIGIN mismatch - please set your URL in your environment variable",
					);
				} else if (data.details && Array.isArray(data.details)) {
					// Map backend validation details to field errors
					const by_field = {};
					for (const item of data.details) {
						const path = item.path || item.param;
						if (path) by_field[path] = item.msg || item.message;
					}
					setFieldErrors(by_field);
					setError(data.error || "Please fix the errors below.");
				} else if (
					data.error &&
					(data.error.includes("already exists") ||
						data.error.includes("Username or email"))
				) {
					setError(data.error);
					setFieldErrors({
						username: "Username or email may already be in use",
						email: "Username or email may already be in use",
					});
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
									onBlur={handleBlur}
									className={input_error_class("firstName")}
									placeholder="Enter your first name"
									required
									disabled={isLoading}
								/>
								{get_field_error("firstName") && (
									<p className="mt-1 text-sm text-danger-600 dark:text-danger-400">
										{get_field_error("firstName")}
									</p>
								)}
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
									onBlur={handleBlur}
									className={input_error_class("lastName")}
									placeholder="Enter your last name"
									required
									disabled={isLoading}
								/>
								{get_field_error("lastName") && (
									<p className="mt-1 text-sm text-danger-600 dark:text-danger-400">
										{get_field_error("lastName")}
									</p>
								)}
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
								onBlur={handleBlur}
								className={input_error_class("username")}
								placeholder="At least 2 characters"
								required
								disabled={isLoading}
							/>
							{get_field_error("username") && (
								<p className="mt-1 text-sm text-danger-600 dark:text-danger-400">
									{get_field_error("username")}
								</p>
							)}
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
								onBlur={handleBlur}
								className={input_error_class("email")}
								placeholder="you@example.com"
								required
								disabled={isLoading}
							/>
							{get_field_error("email") && (
								<p className="mt-1 text-sm text-danger-600 dark:text-danger-400">
									{get_field_error("email")}
								</p>
							)}
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
								onBlur={handleBlur}
								className={input_error_class("password")}
								placeholder="Enter your password"
								required
								disabled={isLoading}
							/>
							{/* Strength bar: 5 segments for length, upper, lower, number, special */}
							<div className="mt-1.5 flex gap-0.5">
								{[1, 2, 3, 4, 5].map((level) => (
									<div
										key={level}
										className="h-1 flex-1 rounded-full transition-colors"
										style={{
											backgroundColor:
												password_strength_level >= level
													? level <= 2
														? "var(--color-danger-500, #ef4444)"
														: level <= 4
															? "var(--color-warning-500, #eab308)"
															: "var(--color-success-500, #22c55e)"
													: "var(--color-secondary-200, #e5e7eb)",
										}}
									/>
								))}
							</div>
							<p className="mt-1 text-xs text-secondary-500 dark:text-secondary-400">
								Strength:{" "}
								{password_strength_level === 0
									? "none"
									: password_strength_level <= 2
										? "weak"
										: password_strength_level <= 4
											? "good"
											: "strong"}
							</p>
							<ul className="mt-1.5 space-y-0.5 text-xs text-secondary-600 dark:text-secondary-400">
								<li
									className={
										password_checks.length
											? "text-success-600 dark:text-success-400"
											: ""
									}
								>
									{password_checks.length ? (
										<CheckCircle className="inline h-3.5 w-3.5 mr-1 align-middle" />
									) : (
										<span className="inline-block w-3.5 h-3.5 mr-1 align-middle rounded-full border border-current" />
									)}
									At least 8 characters
								</li>
								<li
									className={
										password_checks.uppercase
											? "text-success-600 dark:text-success-400"
											: ""
									}
								>
									{password_checks.uppercase ? (
										<CheckCircle className="inline h-3.5 w-3.5 mr-1 align-middle" />
									) : (
										<span className="inline-block w-3.5 h-3.5 mr-1 align-middle rounded-full border border-current" />
									)}
									One uppercase letter
								</li>
								<li
									className={
										password_checks.lowercase
											? "text-success-600 dark:text-success-400"
											: ""
									}
								>
									{password_checks.lowercase ? (
										<CheckCircle className="inline h-3.5 w-3.5 mr-1 align-middle" />
									) : (
										<span className="inline-block w-3.5 h-3.5 mr-1 align-middle rounded-full border border-current" />
									)}
									One lowercase letter
								</li>
								<li
									className={
										password_checks.number
											? "text-success-600 dark:text-success-400"
											: ""
									}
								>
									{password_checks.number ? (
										<CheckCircle className="inline h-3.5 w-3.5 mr-1 align-middle" />
									) : (
										<span className="inline-block w-3.5 h-3.5 mr-1 align-middle rounded-full border border-current" />
									)}
									One number
								</li>
								<li
									className={
										password_checks.special
											? "text-success-600 dark:text-success-400"
											: ""
									}
								>
									{password_checks.special ? (
										<CheckCircle className="inline h-3.5 w-3.5 mr-1 align-middle" />
									) : (
										<span className="inline-block w-3.5 h-3.5 mr-1 align-middle rounded-full border border-current" />
									)}
									One special character (!@#$%...)
								</li>
							</ul>
							{get_field_error("password") && (
								<p className="mt-1 text-sm text-danger-600 dark:text-danger-400">
									{get_field_error("password")}
								</p>
							)}
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
								onBlur={handleBlur}
								className={
									confirm_error
										? "input w-full border-danger-500 dark:border-danger-400 focus:ring-danger-500"
										: "input w-full"
								}
								placeholder="Confirm your password"
								required
								disabled={isLoading}
							/>
							{confirm_error && (
								<p className="mt-1 text-sm text-danger-600 dark:text-danger-400">
									Passwords do not match
								</p>
							)}
							{get_field_error("confirmPassword") && !confirm_error && (
								<p className="mt-1 text-sm text-danger-600 dark:text-danger-400">
									{get_field_error("confirmPassword")}
								</p>
							)}
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
