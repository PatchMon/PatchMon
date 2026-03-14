import {
	AlertCircle,
	CheckCircle,
	Shield,
	Smartphone,
	UserPlus,
} from "lucide-react";
import { useEffect, useId, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useAuth } from "../contexts/AuthContext";
import { marketingAPI, settingsAPI } from "../utils/api";
import { WizardCommunityLinks } from "./CommunityLinks";
import FormInput, { FORM_INPUT_CLASS } from "./FormInput";
import WizardTfaSetup from "./WizardTfaSetup";

const EMAIL_REGEX = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
const USERNAME_MIN_LENGTH = 2;
const DEFAULT_PASSWORD_POLICY = {
	min_length: 8,
	require_uppercase: true,
	require_lowercase: true,
	require_number: true,
	require_special: true,
};
const SPECIAL_REGEX = /[^A-Za-z0-9]/;

const get_password_checks = (password, policy = DEFAULT_PASSWORD_POLICY) => ({
	length: password.length >= (policy.min_length || 8),
	uppercase: !policy.require_uppercase || /[A-Z]/.test(password),
	lowercase: !policy.require_lowercase || /[a-z]/.test(password),
	number: !policy.require_number || /[0-9]/.test(password),
	special: !policy.require_special || SPECIAL_REGEX.test(password),
});

const password_strength = (password, policy = DEFAULT_PASSWORD_POLICY) => {
	if (!password.length) return 0;
	const c = get_password_checks(password, policy);
	const requirements = [
		c.length,
		policy.require_uppercase && c.uppercase,
		policy.require_lowercase && c.lowercase,
		policy.require_number && c.number,
		policy.require_special && c.special,
	].filter((x) => x === true);
	return requirements.length;
};

const STEP_LABELS = [
	"Create Admin Account",
	"Multi-Factor Authentication",
	"Confirm URL",
	"Stay Updated",
	"Contact & Follow",
];

// Parse URL into protocol, host, port
const parseServerUrl = (url) => {
	try {
		const u = new URL(url || "http://localhost:3001");
		return {
			serverProtocol: u.protocol.replace(":", "") || "http",
			serverHost: u.hostname || "localhost",
			serverPort: u.port
				? parseInt(u.port, 10)
				: u.protocol === "https:"
					? 443
					: 80,
		};
	} catch {
		return {
			serverProtocol: "http",
			serverHost: "localhost",
			serverPort: 3001,
		};
	}
};

const FirstTimeWizard = () => {
	const { setAuthState, completeFirstTimeWizard } = useAuth();
	const navigate = useNavigate();
	const [step, setStep] = useState(1);
	const [settingUp, setSettingUp] = useState(false);
	const [setupStatus, setSetupStatus] = useState("");
	const [setupError, setSetupError] = useState("");
	const [wizardData, setWizardData] = useState({
		firstName: "",
		lastName: "",
		username: "",
		email: "",
		password: "",
		confirmPassword: "",
		serverProtocol: "http",
		serverHost: "localhost",
		serverPort: 3001,
		ignoreSslSelfSigned: false,
		newsletterSubscribed: false,
		newsletterName: "",
		newsletterEmail: "",
		honeypot: "",
	});
	const [_touched, setTouched] = useState({});
	const [fieldErrors, setFieldErrors] = useState({});
	const [passwordPolicy, setPasswordPolicy] = useState(DEFAULT_PASSWORD_POLICY);
	const [mfaChoice, setMfaChoice] = useState(null); // 'setup_now' | 'skip'
	const [userCreatedEarly, setUserCreatedEarly] = useState(false); // true when we created user for MFA path
	const [tfaSetupComplete, setTfaSetupComplete] = useState(false); // true when user finished TFA setup (allows Back from step 3)
	const firstNameId = useId();
	const lastNameId = useId();
	const usernameId = useId();
	const emailId = useId();
	const passwordId = useId();
	const confirmPasswordId = useId();
	const protocolId = useId();
	const hostId = useId();
	const portId = useId();
	const ignoreSslId = useId();
	const stayUpdatedId = useId();

	// Fetch password policy and current URL
	useEffect(() => {
		let cancelled = false;
		fetch("/api/v1/settings/login-settings")
			.then((res) => (res.ok ? res.json() : {}))
			.then((data) => {
				if (cancelled) return;
				if (
					data.password_policy &&
					typeof data.password_policy.min_length === "number"
				) {
					setPasswordPolicy({
						min_length: data.password_policy.min_length,
						require_uppercase: data.password_policy.require_uppercase !== false,
						require_lowercase: data.password_policy.require_lowercase !== false,
						require_number: data.password_policy.require_number !== false,
						require_special: data.password_policy.require_special !== false,
					});
				}
			})
			.catch(() => {});
		return () => {
			cancelled = true;
		};
	}, []);

	useEffect(() => {
		if (step !== 3) return;
		let cancelled = false;
		settingsAPI
			.getCurrentUrl()
			.then((res) => res.data)
			.then((data) => {
				if (cancelled || !data?.url) return;
				const parsed = parseServerUrl(data.url);
				setWizardData((prev) => ({
					...prev,
					...parsed,
				}));
			})
			.catch(() => {
				if (cancelled) return;
				const parsed = parseServerUrl(window.location.origin);
				setWizardData((prev) => ({
					...prev,
					...parsed,
				}));
			});
		return () => {
			cancelled = true;
		};
	}, [step]);

	const updateWizardData = (updates) => {
		setWizardData((prev) => ({ ...prev, ...updates }));
		setFieldErrors((prev) => {
			const next = { ...prev };
			for (const k of Object.keys(updates)) {
				delete next[k];
			}
			return next;
		});
	};

	const runFieldValidation = (fieldName) => {
		const d = wizardData;
		setFieldErrors((prev) => {
			const next = { ...prev };
			switch (fieldName) {
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
					const checks = get_password_checks(d.password, passwordPolicy);
					if (!d.password) next.password = "Password is required";
					else if (!checks.length)
						next.password = `Password must be at least ${passwordPolicy.min_length} characters`;
					else {
						const missing = [];
						if (passwordPolicy.require_uppercase && !checks.uppercase)
							missing.push("one uppercase letter");
						if (passwordPolicy.require_lowercase && !checks.lowercase)
							missing.push("one lowercase letter");
						if (passwordPolicy.require_number && !checks.number)
							missing.push("one number");
						if (passwordPolicy.require_special && !checks.special)
							missing.push("one special character");
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
			if (next[fieldName] === "") delete next[fieldName];
			return next;
		});
	};

	const validateStep1 = () => {
		const d = wizardData;
		const next = {};
		if (!d.firstName.trim()) next.firstName = "First name is required";
		if (!d.lastName.trim()) next.lastName = "Last name is required";
		const u = d.username.trim();
		if (!u) next.username = "Username is required";
		else if (u.length < USERNAME_MIN_LENGTH)
			next.username = `Username must be at least ${USERNAME_MIN_LENGTH} characters`;
		const em = d.email.trim();
		if (!em) next.email = "Email address is required";
		else if (!EMAIL_REGEX.test(em))
			next.email = "Enter a valid email (e.g. you@example.com)";
		const pc = get_password_checks(d.password, passwordPolicy);
		if (!d.password) next.password = "Password is required";
		else if (!pc.length)
			next.password = `Password must be at least ${passwordPolicy.min_length} characters`;
		else {
			const missing = [];
			if (passwordPolicy.require_uppercase && !pc.uppercase)
				missing.push("one uppercase letter");
			if (passwordPolicy.require_lowercase && !pc.lowercase)
				missing.push("one lowercase letter");
			if (passwordPolicy.require_number && !pc.number)
				missing.push("one number");
			if (passwordPolicy.require_special && !pc.special)
				missing.push("one special character");
			if (missing.length) next.password = `Add ${missing.join(", ")}`;
		}
		if (!d.confirmPassword)
			next.confirmPassword = "Please confirm your password";
		else if (d.password !== d.confirmPassword)
			next.confirmPassword = "Passwords do not match";
		setFieldErrors(next);
		return Object.keys(next).length === 0;
	};

	const handleBlur = (e) => {
		const { name } = e.target;
		setTouched((prev) => ({ ...prev, [name]: true }));
		runFieldValidation(name);
	};

	const passwordChecks = get_password_checks(
		wizardData.password,
		passwordPolicy,
	);
	const passwordStrengthLevel = password_strength(
		wizardData.password,
		passwordPolicy,
	);

	const handleNext = () => {
		if (step === 1 && !validateStep1()) return;
		if (step < 5) setStep((s) => s + 1);
	};

	const handleBack = () => {
		if (step > 1) setStep((s) => s - 1);
	};

	const handleMfaSetupNow = async () => {
		setMfaChoice("setup_now");
		setSettingUp(true);
		setSetupError("");
		setSetupStatus("Creating admin account...");
		try {
			const setupRes = await fetch("/api/v1/auth/setup-admin", {
				method: "POST",
				headers: { "Content-Type": "application/json" },
				credentials: "include",
				body: JSON.stringify({
					username: wizardData.username.trim(),
					email: wizardData.email.trim(),
					password: wizardData.password,
					firstName: wizardData.firstName.trim(),
					lastName: wizardData.lastName.trim(),
				}),
			});
			const setupData = await setupRes.json();
			if (!setupRes.ok) {
				throw new Error(
					setupData.error || setupData.message || "Failed to create admin",
				);
			}
			setAuthState(setupData.token, setupData.user, {
				keepWizardVisible: true,
			});
			setUserCreatedEarly(true);
			setSettingUp(false);
			// Stay on step 2 but show TFA setup UI (handled in render)
		} catch (err) {
			setSetupError(err.message || "Something went wrong");
			setSettingUp(false);
		}
	};

	const handleMfaSkip = () => {
		setMfaChoice("skip");
		setStep(3); // Go to URL step
	};

	const handleTfaComplete = () => {
		setTfaSetupComplete(true);
		setStep(3); // Continue to URL step
	};

	const runSetup = async (skipOnError = false) => {
		setSettingUp(true);
		setSetupError("");
		setSetupStatus(
			userCreatedEarly ? "Saving server URL..." : "Creating admin account...",
		);

		let setupData = null;

		try {
			// 1. Create admin (skip if already created for MFA path)
			if (!userCreatedEarly) {
				const setupRes = await fetch("/api/v1/auth/setup-admin", {
					method: "POST",
					headers: { "Content-Type": "application/json" },
					credentials: "include",
					body: JSON.stringify({
						username: wizardData.username.trim(),
						email: wizardData.email.trim(),
						password: wizardData.password,
						firstName: wizardData.firstName.trim(),
						lastName: wizardData.lastName.trim(),
					}),
				});

				setupData = await setupRes.json();

				if (!setupRes.ok) {
					throw new Error(
						setupData.error || setupData.message || "Failed to create admin",
					);
				}
				setAuthState(setupData.token, setupData.user);
			} else {
				setupData = { token: "existing", user: {} }; // Auth already set
			}

			// 2. Save server URL (requires auth - cookies from step 1 or existing session)
			setSetupStatus("Saving server URL...");
			try {
				await settingsAPI.update({
					server_protocol: wizardData.serverProtocol,
					server_host: wizardData.serverHost.trim(),
					server_port: Number(wizardData.serverPort) || 3001,
					ignore_ssl_self_signed: wizardData.ignoreSslSelfSigned,
				});
			} catch (err) {
				if (skipOnError && setupData?.token && setupData?.user) {
					// Admin created; skip URL save and continue
					gotoDashboard(setupData);
					return;
				}
				throw err;
			}

			// 3. Newsletter subscribe (optional, no auth)
			if (wizardData.newsletterSubscribed) {
				setSetupStatus("Subscribing to newsletter...");
				try {
					const name = `${wizardData.firstName} ${wizardData.lastName}`.trim();
					const email = wizardData.email.trim();
					if (name && email) {
						await marketingAPI.subscribe({ name, email });
					}
				} catch {
					// Non-fatal; continue
				}
			}

			gotoDashboard(setupData);
		} catch (err) {
			setSetupError(err.message || "Something went wrong");
		} finally {
			setSettingUp(false);
		}
	};

	const gotoDashboard = (data) => {
		if (data?.token && data?.user && data.token !== "existing") {
			setAuthState(data.token, data.user);
		} else if (data?.token === "existing") {
			// User was created early (MFA path); clear wizard flag
			completeFirstTimeWizard();
		}
		localStorage.setItem("patchmon_first_time_complete", "1");
		navigate("/", { replace: true });
	};

	const handleAccessDashboard = () => {
		if (step === 5) {
			runSetup(false);
		}
	};

	const handleSkipAndContinue = () => {
		runSetup(true);
	};

	// Setting Up Screen
	if (settingUp) {
		return (
			<div className="min-h-screen bg-gradient-to-br from-primary-50 to-secondary-50 dark:from-secondary-900 dark:to-secondary-800 flex items-center justify-center p-4">
				<div className="max-w-2xl w-full text-center">
					<div className="card p-8">
						<div className="flex justify-center mb-6">
							<div className="animate-spin rounded-full h-12 w-12 border-b-2 border-primary-600" />
						</div>
						<h2 className="text-xl font-bold text-secondary-900 dark:text-white mb-2">
							Setting up PatchMon
						</h2>
						<p className="text-secondary-600 dark:text-secondary-300 mb-6">
							{setupStatus}
						</p>
						{setupError && (
							<div className="mb-6 p-4 bg-danger-50 dark:bg-danger-900 border border-danger-200 dark:border-danger-700 rounded-lg text-left">
								<div className="flex items-start gap-2">
									<AlertCircle className="h-5 w-5 text-danger-600 dark:text-danger-400 flex-shrink-0 mt-0.5" />
									<div>
										<p className="text-danger-700 dark:text-danger-300 text-sm font-medium">
											{setupError}
										</p>
										<div className="mt-3 flex gap-2">
											<button
												type="button"
												onClick={runSetup}
												className="btn-primary text-sm py-1.5 px-3"
											>
												Retry
											</button>
											<button
												type="button"
												onClick={handleSkipAndContinue}
												className="btn-secondary text-sm py-1.5 px-3"
											>
												Skip and continue
											</button>
										</div>
									</div>
								</div>
							</div>
						)}
					</div>
				</div>
			</div>
		);
	}

	// Wizard layout
	return (
		<div className="min-h-screen bg-gradient-to-br from-primary-50 to-secondary-50 dark:from-secondary-900 dark:to-secondary-800 flex items-center justify-center p-4">
			<div className="max-w-2xl w-full">
				<div className="card p-8">
					{/* Step indicator */}
					<div className="mb-8">
						<div className="flex justify-between mb-2">
							{[1, 2, 3, 4, 5].map((s) => (
								<div
									key={s}
									className={`flex-1 h-1.5 rounded-full mx-0.5 first:ml-0 last:mr-0 ${
										s <= step
											? "bg-primary-500"
											: "bg-secondary-200 dark:bg-secondary-600"
									}`}
								/>
							))}
						</div>
						<p className="text-sm text-secondary-600 dark:text-secondary-400">
							Step {step} of 5: {STEP_LABELS[step - 1]}
						</p>
					</div>

					{/* Step 1: Create Admin Account */}
					{step === 1 && (
						<>
							<div className="text-center mb-6">
								<div className="flex justify-center mb-4">
									<div className="bg-primary-100 dark:bg-primary-900 p-4 rounded-full">
										<Shield className="h-12 w-12 text-primary-600 dark:text-primary-400" />
									</div>
								</div>
								<h1 className="text-2xl font-bold text-secondary-900 dark:text-white mb-2">
									Create Admin Account
								</h1>
								<p className="text-secondary-600 dark:text-secondary-300">
									Set up your administrator account to manage PatchMon
								</p>
							</div>

							<div className="space-y-4">
								<div className="grid grid-cols-2 gap-4">
									<FormInput
										id={firstNameId}
										label="First Name"
										name="firstName"
										value={wizardData.firstName}
										onChange={(e) =>
											updateWizardData({ firstName: e.target.value })
										}
										onBlur={handleBlur}
										placeholder="First name"
										required
										error={fieldErrors.firstName}
									/>
									<FormInput
										id={lastNameId}
										label="Last Name"
										name="lastName"
										value={wizardData.lastName}
										onChange={(e) =>
											updateWizardData({ lastName: e.target.value })
										}
										onBlur={handleBlur}
										placeholder="Last name"
										required
										error={fieldErrors.lastName}
									/>
								</div>
								<FormInput
									id={usernameId}
									label="Username"
									name="username"
									value={wizardData.username}
									onChange={(e) =>
										updateWizardData({ username: e.target.value })
									}
									onBlur={handleBlur}
									placeholder="At least 2 characters"
									required
									error={fieldErrors.username}
								/>
								<FormInput
									id={emailId}
									label="Email Address"
									type="email"
									name="email"
									value={wizardData.email}
									onChange={(e) => updateWizardData({ email: e.target.value })}
									onBlur={handleBlur}
									placeholder="you@example.com"
									required
									error={fieldErrors.email}
								/>
								<div>
									<FormInput
										id={passwordId}
										label="Password"
										type="password"
										name="password"
										value={wizardData.password}
										onChange={(e) =>
											updateWizardData({ password: e.target.value })
										}
										onBlur={handleBlur}
										placeholder="Enter your password"
										required
										error={fieldErrors.password}
									/>
									<div className="mt-1.5 flex gap-0.5">
										{[1, 2, 3, 4, 5].map((level) => (
											<div
												key={level}
												className="h-1 flex-1 rounded-full transition-colors"
												style={{
													backgroundColor:
														passwordStrengthLevel >= level
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
									<ul className="mt-1.5 space-y-0.5 text-xs text-secondary-600 dark:text-secondary-400">
										<li
											className={
												passwordChecks.length
													? "text-success-600 dark:text-success-400"
													: ""
											}
										>
											{passwordChecks.length ? (
												<CheckCircle className="inline h-3.5 w-3.5 mr-1 align-middle" />
											) : (
												<span className="inline-block w-3.5 h-3.5 mr-1 align-middle rounded-full border border-current" />
											)}
											At least {passwordPolicy.min_length} characters
										</li>
										{passwordPolicy.require_uppercase && (
											<li
												className={
													passwordChecks.uppercase
														? "text-success-600 dark:text-success-400"
														: ""
												}
											>
												{passwordChecks.uppercase ? (
													<CheckCircle className="inline h-3.5 w-3.5 mr-1 align-middle" />
												) : (
													<span className="inline-block w-3.5 h-3.5 mr-1 align-middle rounded-full border border-current" />
												)}
												One uppercase letter
											</li>
										)}
										{passwordPolicy.require_lowercase && (
											<li
												className={
													passwordChecks.lowercase
														? "text-success-600 dark:text-success-400"
														: ""
												}
											>
												{passwordChecks.lowercase ? (
													<CheckCircle className="inline h-3.5 w-3.5 mr-1 align-middle" />
												) : (
													<span className="inline-block w-3.5 h-3.5 mr-1 align-middle rounded-full border border-current" />
												)}
												One lowercase letter
											</li>
										)}
										{passwordPolicy.require_number && (
											<li
												className={
													passwordChecks.number
														? "text-success-600 dark:text-success-400"
														: ""
												}
											>
												{passwordChecks.number ? (
													<CheckCircle className="inline h-3.5 w-3.5 mr-1 align-middle" />
												) : (
													<span className="inline-block w-3.5 h-3.5 mr-1 align-middle rounded-full border border-current" />
												)}
												One number
											</li>
										)}
										{passwordPolicy.require_special && (
											<li
												className={
													passwordChecks.special
														? "text-success-600 dark:text-success-400"
														: ""
												}
											>
												{passwordChecks.special ? (
													<CheckCircle className="inline h-3.5 w-3.5 mr-1 align-middle" />
												) : (
													<span className="inline-block w-3.5 h-3.5 mr-1 align-middle rounded-full border border-current" />
												)}
												One special character
											</li>
										)}
									</ul>
								</div>
								<FormInput
									id={confirmPasswordId}
									label="Confirm Password"
									type="password"
									name="confirmPassword"
									value={wizardData.confirmPassword}
									onChange={(e) =>
										updateWizardData({ confirmPassword: e.target.value })
									}
									onBlur={handleBlur}
									placeholder="Confirm your password"
									required
									error={fieldErrors.confirmPassword}
								/>
							</div>
						</>
					)}

					{/* Step 2: Multi-Factor Authentication */}
					{step === 2 &&
						(mfaChoice === "setup_now" &&
						userCreatedEarly &&
						!tfaSetupComplete ? (
							<WizardTfaSetup onComplete={handleTfaComplete} />
						) : mfaChoice === "setup_now" &&
							userCreatedEarly &&
							tfaSetupComplete ? (
							<div className="text-center mb-6">
								<div className="flex justify-center mb-4">
									<div className="bg-success-100 dark:bg-success-900 p-4 rounded-full">
										<CheckCircle className="h-12 w-12 text-success-600 dark:text-success-400" />
									</div>
								</div>
								<h1 className="text-2xl font-bold text-secondary-900 dark:text-white mb-2">
									MFA configured
								</h1>
								<p className="text-secondary-600 dark:text-secondary-300">
									Multi-factor authentication is set up. Click Next to continue.
								</p>
							</div>
						) : (
							<>
								<div className="text-center mb-6">
									<div className="flex justify-center mb-4">
										<div className="bg-primary-100 dark:bg-primary-900 p-4 rounded-full">
											<Smartphone className="h-12 w-12 text-primary-600 dark:text-primary-400" />
										</div>
									</div>
									<h1 className="text-2xl font-bold text-secondary-900 dark:text-white mb-2">
										Multi-Factor Authentication
									</h1>
									<p className="text-secondary-600 dark:text-secondary-300">
										Add an extra layer of security with TOTP. You can set it up
										now or do it later from your profile.
									</p>
								</div>
								{setupError && (
									<div className="mb-4 p-4 bg-danger-50 dark:bg-danger-900 border border-danger-200 dark:border-danger-700 rounded-lg flex items-start gap-2">
										<AlertCircle className="h-5 w-5 text-danger-600 dark:text-danger-400 flex-shrink-0 mt-0.5" />
										<p className="text-danger-700 dark:text-danger-300 text-sm">
											{setupError}
										</p>
									</div>
								)}
								<div className="flex flex-col gap-3">
									<button
										type="button"
										onClick={handleMfaSetupNow}
										disabled={settingUp}
										className="btn-primary w-full flex items-center justify-center gap-2"
									>
										{settingUp ? (
											<>
												<div className="animate-spin rounded-full h-4 w-4 border-b-2 border-current" />
												Creating account...
											</>
										) : (
											<>
												<Shield className="h-4 w-4" />
												Setup MFA now
											</>
										)}
									</button>
									<button
										type="button"
										onClick={handleMfaSkip}
										disabled={settingUp}
										className="btn-secondary w-full"
									>
										Skip (I&apos;ll do it later)
									</button>
								</div>
							</>
						))}

					{/* Step 3: Confirm URL */}
					{step === 3 && (
						<>
							<div className="text-center mb-6">
								<h1 className="text-2xl font-bold text-secondary-900 dark:text-white mb-2">
									Confirm Server URL
								</h1>
								<p className="text-secondary-600 dark:text-secondary-300">
									This is the URL agents use to connect. If you change your
									access URL later, update it in Settings &gt; Server URL.
								</p>
							</div>
							<div className="grid grid-cols-1 md:grid-cols-[auto_1fr_6rem] gap-4">
								<div className="min-w-0">
									<label
										htmlFor={protocolId}
										className="block text-sm font-medium text-secondary-700 dark:text-white mb-1"
									>
										Protocol
									</label>
									<select
										id={protocolId}
										value={wizardData.serverProtocol}
										onChange={(e) =>
											updateWizardData({ serverProtocol: e.target.value })
										}
										className={FORM_INPUT_CLASS}
									>
										<option value="http">HTTP</option>
										<option value="https">HTTPS</option>
									</select>
								</div>
								<div className="min-w-0">
									<label
										htmlFor={hostId}
										className="block text-sm font-medium text-secondary-700 dark:text-white mb-1"
									>
										Host
									</label>
									<input
										id={hostId}
										type="text"
										value={wizardData.serverHost}
										onChange={(e) =>
											updateWizardData({ serverHost: e.target.value })
										}
										className={FORM_INPUT_CLASS}
										placeholder="example.com"
									/>
								</div>
								<div className="min-w-0">
									<label
										htmlFor={portId}
										className="block text-sm font-medium text-secondary-700 dark:text-white mb-1"
									>
										Port
									</label>
									<input
										id={portId}
										type="number"
										value={wizardData.serverPort}
										onChange={(e) =>
											updateWizardData({
												serverPort: parseInt(e.target.value, 10) || 3001,
											})
										}
										className={FORM_INPUT_CLASS}
										min={1}
										max={65535}
									/>
								</div>
							</div>
							{/* Self-signed SSL toggle */}
							<div className="mt-6 flex items-start justify-between gap-4 p-4 bg-secondary-50 dark:bg-secondary-800/50 rounded-lg border border-secondary-200 dark:border-secondary-600">
								<div className="flex-1">
									<label
										htmlFor={ignoreSslId}
										className="text-sm font-medium text-secondary-900 dark:text-secondary-100"
									>
										Will you be using a self-signed SSL certificate?
									</label>
									<p className="mt-1 text-sm text-secondary-500 dark:text-secondary-400">
										When enabled, curl commands in agent scripts will use the -k
										flag to skip certificate verification. You can change this
										later in Settings.
									</p>
								</div>
								<button
									type="button"
									id={ignoreSslId}
									onClick={() =>
										updateWizardData({
											ignoreSslSelfSigned: !wizardData.ignoreSslSelfSigned,
										})
									}
									className={`relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-md border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 ${
										wizardData.ignoreSslSelfSigned
											? "bg-primary-600 dark:bg-primary-500"
											: "bg-secondary-200 dark:bg-secondary-600"
									}`}
								>
									<span
										className={`pointer-events-none inline-block h-5 w-5 transform rounded-md bg-white shadow ring-0 transition duration-200 ease-in-out ${
											wizardData.ignoreSslSelfSigned
												? "translate-x-5"
												: "translate-x-0"
										}`}
									/>
								</button>
							</div>
						</>
					)}

					{/* Step 4: Stay Updated */}
					{step === 4 && (
						<>
							<div className="text-center mb-6">
								<h1 className="text-2xl font-bold text-secondary-900 dark:text-white mb-2">
									Stay Updated
								</h1>
								<p className="text-secondary-600 dark:text-secondary-300">
									Opt-in to receive security and important information about
									your PatchMon instance (optional).
								</p>
							</div>
							<div className="flex items-start justify-between gap-4 p-4 bg-secondary-50 dark:bg-secondary-800/50 rounded-lg border border-secondary-200 dark:border-secondary-600">
								<div className="flex-1">
									<label
										htmlFor={stayUpdatedId}
										className="text-sm font-medium text-secondary-900 dark:text-secondary-100"
									>
										Opt-in to stay updated with security and important
										information about PatchMon instance
									</label>
									{wizardData.newsletterSubscribed && (
										<div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-secondary-600 dark:text-secondary-400">
											<span>
												Using:{" "}
												{`${wizardData.firstName} ${wizardData.lastName}`.trim() ||
													"—"}
											</span>
											<span className="text-secondary-400 dark:text-secondary-500">
												•
											</span>
											<span>{wizardData.email || "—"}</span>
										</div>
									)}
								</div>
								<button
									type="button"
									id={stayUpdatedId}
									onClick={() =>
										updateWizardData({
											newsletterSubscribed: !wizardData.newsletterSubscribed,
										})
									}
									className={`relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-md border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 ${
										wizardData.newsletterSubscribed
											? "bg-primary-600 dark:bg-primary-500"
											: "bg-secondary-200 dark:bg-secondary-600"
									}`}
								>
									<span
										className={`pointer-events-none inline-block h-5 w-5 transform rounded-md bg-white shadow ring-0 transition duration-200 ease-in-out ${
											wizardData.newsletterSubscribed
												? "translate-x-5"
												: "translate-x-0"
										}`}
									/>
								</button>
							</div>
							{/* Honeypot - hidden */}
							<input
								type="text"
								name="62f7db5ee090a53e362464fbe0341d7154a992bb"
								value={wizardData.honeypot}
								onChange={(e) => updateWizardData({ honeypot: e.target.value })}
								className="hidden"
								tabIndex={-1}
								autoComplete="off"
							/>
						</>
					)}

					{/* Step 5: Contact & Follow */}
					{step === 5 && (
						<>
							<div className="text-center mb-6">
								<h1 className="text-2xl font-bold text-secondary-900 dark:text-white mb-2">
									Get in Touch
								</h1>
								<p className="text-secondary-600 dark:text-secondary-300">
									Join our community and stay connected.
								</p>
							</div>
							<WizardCommunityLinks />
						</>
					)}

					{/* Navigation */}
					{!(
						step === 2 &&
						mfaChoice === "setup_now" &&
						userCreatedEarly &&
						!tfaSetupComplete
					) && (
						<div className="mt-8 flex gap-3">
							{step > 1 && (
								<button
									type="button"
									onClick={handleBack}
									className="btn-secondary flex-1"
								>
									Back
								</button>
							)}
							{step < 5 ? (
								<button
									type="button"
									onClick={handleNext}
									className="btn-primary flex-1 flex items-center justify-center gap-2"
								>
									Next
								</button>
							) : (
								<button
									type="button"
									onClick={handleAccessDashboard}
									className="btn-primary flex-1 flex items-center justify-center gap-2"
								>
									<UserPlus className="h-4 w-4" />
									Access Dashboard
								</button>
							)}
						</div>
					)}
				</div>
			</div>
		</div>
	);
};

export default FirstTimeWizard;
