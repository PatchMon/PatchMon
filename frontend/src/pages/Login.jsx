import {
	AlertCircle,
	ArrowLeft,
	BookOpen,
	Eye,
	EyeOff,
	Github,
	Globe,
	Lock,
	Mail,
	Route,
	Star,
	User,
} from "lucide-react";

import { useEffect, useId, useRef, useState } from "react";
import { FaReddit, FaYoutube } from "react-icons/fa";

import { useNavigate } from "react-router-dom";
import DiscordIcon from "../components/DiscordIcon";
import { useAuth } from "../contexts/AuthContext";
import { useColorTheme } from "../contexts/ColorThemeContext";
import { authAPI, isCorsError } from "../utils/api";

const Login = () => {
	const usernameId = useId();
	const firstNameId = useId();
	const lastNameId = useId();
	const emailId = useId();
	const passwordId = useId();
	const tokenId = useId();
	const rememberMeId = useId();
	const { login, setAuthState } = useAuth();
	const [isSignupMode, setIsSignupMode] = useState(false);
	const [formData, setFormData] = useState({
		username: "",
		email: "",
		password: "",
		firstName: "",
		lastName: "",
	});
	const [tfaData, setTfaData] = useState({
		token: "",
		remember_me: false,
	});
	const [showPassword, setShowPassword] = useState(false);
	const [isLoading, setIsLoading] = useState(false);
	const [error, setError] = useState("");
	const [requiresTfa, setRequiresTfa] = useState(false);
	const [tfaUsername, setTfaUsername] = useState("");
	const [signupEnabled, setSignupEnabled] = useState(false);
	const [showGithubVersionOnLogin, setShowGithubVersionOnLogin] =
		useState(true);
	const [latestRelease, setLatestRelease] = useState(null);
	const [githubStars, setGithubStars] = useState(null);
	const [oidcConfig, setOidcConfig] = useState({
		enabled: false,
		buttonText: "Login with SSO",
		disableLocalAuth: false,
	});
	const canvasRef = useRef(null);
	const { themeConfig } = useColorTheme();

	const navigate = useNavigate();

	// Generate clean radial gradient background with subtle triangular accents
	useEffect(() => {
		const generateBackground = () => {
			if (!canvasRef.current || !themeConfig?.login) return;

			const canvas = canvasRef.current;
			canvas.width = canvas.offsetWidth;
			canvas.height = canvas.offsetHeight;
			const ctx = canvas.getContext("2d");

			// Get theme colors - pick first color from each palette
			const xColors = themeConfig.login.xColors || [
				"#667eea",
				"#764ba2",
				"#f093fb",
				"#4facfe",
			];
			const yColors = themeConfig.login.yColors || [
				"#667eea",
				"#764ba2",
				"#f093fb",
				"#4facfe",
			];

			// Use date for daily color rotation
			const today = new Date();
			const seed =
				today.getFullYear() * 10000 + today.getMonth() * 100 + today.getDate();
			const random = (s) => {
				const x = Math.sin(s) * 10000;
				return x - Math.floor(x);
			};

			const color1 = xColors[Math.floor(random(seed) * xColors.length)];
			const color2 = yColors[Math.floor(random(seed + 1000) * yColors.length)];

			// Create clean radial gradient from center to bottom-right corner
			const gradient = ctx.createRadialGradient(
				canvas.width * 0.3, // Center slightly left
				canvas.height * 0.3, // Center slightly up
				0,
				canvas.width * 0.5, // Expand to cover screen
				canvas.height * 0.5,
				Math.max(canvas.width, canvas.height) * 1.2,
			);

			// Subtle gradient with darker corners
			gradient.addColorStop(0, color1);
			gradient.addColorStop(0.6, color2);
			gradient.addColorStop(1, "#0a0a0a"); // Very dark edges

			ctx.fillStyle = gradient;
			ctx.fillRect(0, 0, canvas.width, canvas.height);

			// Add subtle triangular shapes as accents across entire background
			const cellSize = 180;
			const cols = Math.ceil(canvas.width / cellSize) + 1;
			const rows = Math.ceil(canvas.height / cellSize) + 1;

			for (let y = 0; y < rows; y++) {
				for (let x = 0; x < cols; x++) {
					const idx = y * cols + x;
					// Draw more triangles (less sparse)
					if (random(seed + idx + 5000) > 0.4) {
						const baseX =
							x * cellSize + random(seed + idx * 3) * cellSize * 0.8;
						const baseY =
							y * cellSize + random(seed + idx * 3 + 100) * cellSize * 0.8;
						const size = 50 + random(seed + idx * 4) * 100;

						ctx.beginPath();
						ctx.moveTo(baseX, baseY);
						ctx.lineTo(baseX + size, baseY);
						ctx.lineTo(baseX + size / 2, baseY - size * 0.866);
						ctx.closePath();

						// More visible white with slightly higher opacity
						ctx.fillStyle = `rgba(255, 255, 255, ${0.05 + random(seed + idx * 5) * 0.08})`;
						ctx.fill();
					}
				}
			}
		};

		generateBackground();

		// Regenerate on window resize
		const handleResize = () => {
			generateBackground();
		};

		window.addEventListener("resize", handleResize);
		return () => window.removeEventListener("resize", handleResize);
	}, [themeConfig]);

	// Check login settings (signup enabled and show github version)
	useEffect(() => {
		const checkLoginSettings = async () => {
			try {
				const response = await fetch("/api/v1/settings/login-settings");
				if (response.ok) {
					const data = await response.json();
					setSignupEnabled(data.signup_enabled || false);
					setShowGithubVersionOnLogin(
						data.show_github_version_on_login !== false,
					);
				}
			} catch (error) {
				console.error("Failed to check login settings:", error);
				// Default to disabled on error for security
				setSignupEnabled(false);
				setShowGithubVersionOnLogin(true); // Default to showing on error
			}
		};
		checkLoginSettings();
	}, []);

	// Fetch OIDC configuration
	useEffect(() => {
		const fetchOidcConfig = async () => {
			try {
				const response = await fetch("/api/v1/auth/oidc/config");
				if (response.ok) {
					const config = await response.json();
					setOidcConfig(config);
				}
			} catch (error) {
				console.error("Failed to fetch OIDC config:", error);
			}
		};
		fetchOidcConfig();
	}, []);

	// Handle OIDC callback (tokens are now in httpOnly cookies)
	useEffect(() => {
		const urlParams = new URLSearchParams(window.location.search);
		const oidcSuccess = urlParams.get("oidc");
		const oidcError = urlParams.get("error");

		if (oidcError) {
			setError(decodeURIComponent(oidcError));
			// Clean up URL
			window.history.replaceState({}, document.title, "/login");
		} else if (oidcSuccess === "success") {
			// Tokens are in httpOnly cookies, fetch user profile to verify auth
			const fetchUserAndRedirect = async () => {
				try {
					// The token cookie will be sent automatically with credentials
					const response = await fetch("/api/v1/auth/profile", {
						credentials: "include",
					});
					if (response.ok) {
						const data = await response.json();
						// Store minimal info for UI state (token is in httpOnly cookie)
						setAuthState(null, data.user);
						navigate("/");
					} else {
						setError("Failed to complete OIDC login");
					}
				} catch (err) {
					console.error("Error fetching user after OIDC login:", err);
					setError("Failed to complete OIDC login");
				}
				// Clean up URL
				window.history.replaceState({}, document.title, "/login");
			};
			fetchUserAndRedirect();
		}
	}, [navigate, setAuthState]);

	// Fetch latest release and stars from GitHub
	useEffect(() => {
		// Only fetch if the setting allows it
		if (!showGithubVersionOnLogin) {
			return;
		}

		const fetchGitHubData = async () => {
			try {
				// Try to get cached data first
				const cachedRelease = localStorage.getItem("githubLatestRelease");
				const cachedStars = localStorage.getItem("githubStarsCount");
				const cacheTime = localStorage.getItem("githubReleaseCacheTime");
				const now = Date.now();

				// Load cached data immediately
				if (cachedRelease) {
					setLatestRelease(JSON.parse(cachedRelease));
				}
				if (cachedStars) {
					setGithubStars(parseInt(cachedStars, 10));
				}

				// Use cache if less than 1 hour old
				if (cacheTime && now - parseInt(cacheTime, 10) < 3600000) {
					return;
				}

				// Fetch repository info (includes star count)
				const repoResponse = await fetch(
					"https://api.github.com/repos/PatchMon/PatchMon",
					{
						headers: {
							Accept: "application/vnd.github.v3+json",
						},
					},
				);

				if (repoResponse.ok) {
					const repoData = await repoResponse.json();
					setGithubStars(repoData.stargazers_count);
					localStorage.setItem(
						"githubStarsCount",
						repoData.stargazers_count.toString(),
					);
				}

				// Fetch latest release
				const releaseResponse = await fetch(
					"https://api.github.com/repos/PatchMon/PatchMon/releases/latest",
					{
						headers: {
							Accept: "application/vnd.github.v3+json",
						},
					},
				);

				if (releaseResponse.ok) {
					const data = await releaseResponse.json();
					const releaseInfo = {
						version: data.tag_name,
						name: data.name,
						publishedAt: new Date(data.published_at).toLocaleDateString(
							"en-US",
							{
								year: "numeric",
								month: "long",
								day: "numeric",
							},
						),
						body: data.body?.split("\n").slice(0, 3).join("\n") || "", // First 3 lines
					};

					setLatestRelease(releaseInfo);
					localStorage.setItem(
						"githubLatestRelease",
						JSON.stringify(releaseInfo),
					);
				}

				localStorage.setItem("githubReleaseCacheTime", now.toString());
			} catch (error) {
				console.error("Failed to fetch GitHub data:", error);
				// Set fallback data if nothing cached
				const cachedRelease = localStorage.getItem("githubLatestRelease");
				if (!cachedRelease) {
					setLatestRelease({
						version: "v1.3.0",
						name: "Latest Release",
						publishedAt: "Recently",
						body: "Monitor and manage your Linux package updates",
					});
				}
			}
		};

		fetchGitHubData();
	}, [showGithubVersionOnLogin]); // Run once on mount

	const handleSubmit = async (e) => {
		e.preventDefault();
		setIsLoading(true);
		setError("");

		try {
			// Use the AuthContext login function which handles everything
			const result = await login(formData.username, formData.password);

			if (result.requiresTfa) {
				setRequiresTfa(true);
				setTfaUsername(formData.username);
				setError("");
			} else if (result.success) {
				navigate("/");
			} else {
				setError(result.error || "Login failed");
			}
		} catch (err) {
			// Check for CORS/network errors first
			if (isCorsError(err)) {
				setError(
					"CORS_ORIGIN mismatch - please set your URL in your environment variable",
				);
			} else if (
				err.name === "TypeError" &&
				err.message?.includes("Failed to fetch")
			) {
				setError(
					"CORS_ORIGIN mismatch - please set your URL in your environment variable",
				);
			} else {
				setError(err.response?.data?.error || "Login failed");
			}
		} finally {
			setIsLoading(false);
		}
	};

	const handleSignupSubmit = async (e) => {
		e.preventDefault();
		setIsLoading(true);
		setError("");

		try {
			const response = await authAPI.signup(
				formData.username,
				formData.email,
				formData.password,
				formData.firstName,
				formData.lastName,
			);
			if (response.data?.token) {
				// Update AuthContext state and localStorage
				setAuthState(response.data.token, response.data.user);

				// Redirect to dashboard
				navigate("/");
			} else {
				setError("Signup failed - invalid response");
			}
		} catch (err) {
			console.error("Signup error:", err);
			if (isCorsError(err)) {
				setError(
					"CORS_ORIGIN mismatch - please set your URL in your environment variable",
				);
			} else if (
				err.name === "TypeError" &&
				err.message?.includes("Failed to fetch")
			) {
				setError(
					"CORS_ORIGIN mismatch - please set your URL in your environment variable",
				);
			} else {
				const errorMessage =
					err.response?.data?.error ||
					(err.response?.data?.errors && err.response.data.errors.length > 0
						? err.response.data.errors.map((e) => e.msg).join(", ")
						: err.message || "Signup failed");
				setError(errorMessage);
			}
		} finally {
			setIsLoading(false);
		}
	};

	const handleTfaSubmit = async (e) => {
		e.preventDefault();
		setIsLoading(true);
		setError("");

		try {
			const response = await authAPI.verifyTfa(
				tfaUsername,
				tfaData.token,
				tfaData.remember_me,
			);

			if (response.data?.token) {
				// Update AuthContext with the new authentication state
				setAuthState(response.data.token, response.data.user);

				// Redirect to dashboard
				navigate("/");
			} else {
				setError("TFA verification failed - invalid response");
			}
		} catch (err) {
			console.error("TFA verification error:", err);
			if (isCorsError(err)) {
				setError(
					"CORS_ORIGIN mismatch - please set your URL in your environment variable",
				);
			} else if (
				err.name === "TypeError" &&
				err.message?.includes("Failed to fetch")
			) {
				setError(
					"CORS_ORIGIN mismatch - please set your URL in your environment variable",
				);
			} else {
				const errorMessage =
					err.response?.data?.error || err.message || "TFA verification failed";
				setError(errorMessage);
			}
			// Clear the token input for security
			setTfaData({ token: "" });
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

	const handleTfaInputChange = (e) => {
		const { name, value, type, checked } = e.target;
		setTfaData({
			...tfaData,
			[name]:
				type === "checkbox"
					? checked
					: value
							.toUpperCase()
							.replace(/[^A-Z0-9]/g, "")
							.slice(0, 6),
		});
		// Clear error when user starts typing
		if (error) {
			setError("");
		}
	};

	const handleBackToLogin = () => {
		setRequiresTfa(false);
		setTfaData({ token: "", remember_me: false });
		setError("");
	};

	const toggleMode = () => {
		// Only allow signup mode if signup is enabled
		if (!signupEnabled && !isSignupMode) {
			return; // Don't allow switching to signup if disabled
		}
		setIsSignupMode(!isSignupMode);
		setFormData({
			username: "",
			email: "",
			password: "",
			firstName: "",
			lastName: "",
		});
		setError("");
	};

	return (
		<div className="min-h-screen relative flex">
			{/* Full-screen Trianglify Background */}
			<canvas ref={canvasRef} className="absolute inset-0 w-full h-full" />
			<div className="absolute inset-0 bg-gradient-to-br from-black/40 to-black/60" />

			{/* Left side - Info Panel (hidden on mobile or when GitHub version is disabled) */}
			{showGithubVersionOnLogin && (
				<div className="hidden lg:flex lg:w-1/2 xl:w-3/5 relative z-10">
					<div className="flex flex-col justify-between text-white p-12 h-full w-full">
						<div className="flex-1 flex flex-col justify-center items-start max-w-xl mx-auto">
							<div className="space-y-6">
								<div>
									<img
										src="/assets/logo_dark.png"
										alt="PatchMon"
										className="h-16 mb-4"
									/>
									<p className="text-sm text-blue-200 font-medium tracking-wide uppercase">
										Linux Patch Monitoring
									</p>
								</div>

								{showGithubVersionOnLogin && latestRelease ? (
									<div className="space-y-4 bg-black/20 backdrop-blur-sm rounded-lg p-6 border border-white/10">
										<div className="flex items-center gap-3">
											<div className="flex items-center gap-2">
												<div className="w-2 h-2 bg-green-400 rounded-full animate-pulse" />
												<span className="text-green-300 text-sm font-semibold">
													Latest Release
												</span>
											</div>
											<span className="text-2xl font-bold text-white">
												{latestRelease.version}
											</span>
										</div>

										{latestRelease.name && (
											<h3 className="text-lg font-semibold text-white">
												{latestRelease.name}
											</h3>
										)}

										<div className="flex items-center gap-2 text-sm text-gray-300">
											<svg
												className="w-4 h-4"
												fill="none"
												stroke="currentColor"
												viewBox="0 0 24 24"
												aria-label="Release date"
											>
												<title>Release date</title>
												<path
													strokeLinecap="round"
													strokeLinejoin="round"
													strokeWidth={2}
													d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"
												/>
											</svg>
											<span>Released {latestRelease.publishedAt}</span>
										</div>

										{latestRelease.body && (
											<p className="text-sm text-gray-300 leading-relaxed line-clamp-3">
												{latestRelease.body}
											</p>
										)}

										<a
											href="https://github.com/PatchMon/PatchMon/releases/latest"
											target="_blank"
											rel="noopener noreferrer"
											className="inline-flex items-center gap-2 text-sm text-blue-300 hover:text-blue-200 transition-colors font-medium"
										>
											View Release Notes
											<svg
												className="w-4 h-4"
												fill="none"
												stroke="currentColor"
												viewBox="0 0 24 24"
												aria-label="External link"
											>
												<title>External link</title>
												<path
													strokeLinecap="round"
													strokeLinejoin="round"
													strokeWidth={2}
													d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14"
												/>
											</svg>
										</a>
									</div>
								) : showGithubVersionOnLogin ? (
									<div className="space-y-4 bg-black/20 backdrop-blur-sm rounded-lg p-6 border border-white/10">
										<div className="animate-pulse space-y-3">
											<div className="h-6 bg-white/20 rounded w-3/4" />
											<div className="h-4 bg-white/20 rounded w-1/2" />
											<div className="h-4 bg-white/20 rounded w-full" />
										</div>
									</div>
								) : null}
							</div>
						</div>

						{/* Social Links Footer */}
						<div className="max-w-xl mx-auto w-full">
							<div className="border-t border-white/10 pt-6">
								<p className="text-sm text-gray-400 mb-4">Connect with us</p>
								<div className="flex flex-wrap items-center gap-2">
									{/* GitHub */}
									<a
										href="https://github.com/PatchMon/PatchMon"
										target="_blank"
										rel="noopener noreferrer"
										className="flex items-center justify-center gap-1.5 px-3 h-10 bg-white/10 hover:bg-white/20 backdrop-blur-sm rounded-lg transition-colors border border-white/10"
										title="GitHub Repository"
									>
										<Github className="h-5 w-5 text-white" />
										{githubStars !== null && (
											<div className="flex items-center gap-1">
												<Star className="h-3.5 w-3.5 fill-current text-yellow-400" />
												<span className="text-sm font-medium text-white">
													{githubStars}
												</span>
											</div>
										)}
									</a>

									{/* Roadmap */}
									<a
										href="https://github.com/orgs/PatchMon/projects/2/views/1"
										target="_blank"
										rel="noopener noreferrer"
										className="flex items-center justify-center w-10 h-10 bg-white/10 hover:bg-white/20 backdrop-blur-sm rounded-lg transition-colors border border-white/10"
										title="Roadmap"
									>
										<Route className="h-5 w-5 text-white" />
									</a>

									{/* Docs */}
									<a
										href="https://docs.patchmon.net"
										target="_blank"
										rel="noopener noreferrer"
										className="flex items-center justify-center w-10 h-10 bg-white/10 hover:bg-white/20 backdrop-blur-sm rounded-lg transition-colors border border-white/10"
										title="Documentation"
									>
										<BookOpen className="h-5 w-5 text-white" />
									</a>

									{/* Discord */}
									<a
										href="https://patchmon.net/discord"
										target="_blank"
										rel="noopener noreferrer"
										className="flex items-center justify-center w-10 h-10 bg-white/10 hover:bg-white/20 backdrop-blur-sm rounded-lg transition-colors border border-white/10"
										title="Discord Community"
									>
										<DiscordIcon className="h-5 w-5 text-white" />
									</a>

									{/* Email */}
									<a
										href="mailto:support@patchmon.net"
										className="flex items-center justify-center w-10 h-10 bg-white/10 hover:bg-white/20 backdrop-blur-sm rounded-lg transition-colors border border-white/10"
										title="Email Support"
									>
										<Mail className="h-5 w-5 text-white" />
									</a>

									{/* YouTube */}
									<a
										href="https://youtube.com/@patchmonTV"
										target="_blank"
										rel="noopener noreferrer"
										className="flex items-center justify-center w-10 h-10 bg-white/10 hover:bg-white/20 backdrop-blur-sm rounded-lg transition-colors border border-white/10"
										title="YouTube Channel"
									>
										<FaYoutube className="h-5 w-5 text-white" />
									</a>

									{/* Reddit */}
									<a
										href="https://www.reddit.com/r/patchmon"
										target="_blank"
										rel="noopener noreferrer"
										className="flex items-center justify-center w-10 h-10 bg-white/10 hover:bg-white/20 backdrop-blur-sm rounded-lg transition-colors border border-white/10"
										title="Reddit Community"
									>
										<FaReddit className="h-5 w-5 text-white" />
									</a>

									{/* Website */}
									<a
										href="https://patchmon.net"
										target="_blank"
										rel="noopener noreferrer"
										className="flex items-center justify-center w-10 h-10 bg-white/10 hover:bg-white/20 backdrop-blur-sm rounded-lg transition-colors border border-white/10"
										title="Visit patchmon.net"
									>
										<Globe className="h-5 w-5 text-white" />
									</a>
								</div>
							</div>
						</div>
					</div>
				</div>
			)}

			{/* Right side - Login Form */}
			<div
				className={`${showGithubVersionOnLogin ? "flex-1" : "w-full"} flex items-center justify-center py-12 px-4 sm:px-6 lg:px-8 relative z-10`}
			>
				<div className="max-w-md w-full space-y-8 bg-white dark:bg-secondary-900 rounded-2xl shadow-2xl p-8 lg:p-10">
					<div>
						<div className="mx-auto h-16 w-16 flex items-center justify-center">
							<img
								src="/assets/favicon.svg"
								alt="PatchMon Logo"
								className="h-16 w-16"
							/>
						</div>
						<h2 className="mt-6 text-center text-3xl font-extrabold text-secondary-900 dark:text-secondary-100">
							{isSignupMode ? "Create PatchMon Account" : "Sign in to PatchMon"}
						</h2>
						<p className="mt-2 text-center text-sm text-secondary-600 dark:text-secondary-400">
							Monitor and manage your Linux package updates
						</p>
					</div>

					{!requiresTfa ? (
						<form
							className="mt-8 space-y-6"
							onSubmit={isSignupMode ? handleSignupSubmit : handleSubmit}
						>
							{/* Only show form fields if local auth is not disabled */}
							{!oidcConfig.disableLocalAuth && (
							<div className="space-y-4">
								<div>
									<label
										htmlFor={usernameId}
										className="block text-sm font-medium text-secondary-900 dark:text-secondary-100"
									>
										{isSignupMode ? "Username" : "Username or Email"}
									</label>
									<div className="mt-1 relative">
										<input
											id={usernameId}
											name="username"
											type="text"
											required
											value={formData.username}
											onChange={handleInputChange}
											className="appearance-none rounded-md relative block w-full pl-10 pr-3 py-2 border border-secondary-300 placeholder-secondary-500 text-secondary-900 focus:outline-none focus:ring-primary-500 focus:border-primary-500 focus:z-10 sm:text-sm"
											placeholder={
												isSignupMode
													? "Enter your username"
													: "Enter your username or email"
											}
										/>
										<div className="absolute left-3 top-1/2 -translate-y-1/2 pointer-events-none z-20 flex items-center">
											<User size={20} color="#64748b" strokeWidth={2} />
										</div>
									</div>
								</div>

								{isSignupMode && (
									<>
										<div className="grid grid-cols-1 md:grid-cols-2 gap-4">
											<div>
												<label
													htmlFor={firstNameId}
													className="block text-sm font-medium text-secondary-900 dark:text-secondary-100"
												>
													First Name
												</label>
												<div className="mt-1 relative">
													<div className="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
														<User className="h-5 w-5 text-secondary-400" />
													</div>
													<input
														id={firstNameId}
														name="firstName"
														type="text"
														required
														value={formData.firstName}
														onChange={handleInputChange}
														className="appearance-none rounded-md relative block w-full pl-10 pr-3 py-2 border border-secondary-300 placeholder-secondary-500 text-secondary-900 focus:outline-none focus:ring-primary-500 focus:border-primary-500 focus:z-10 sm:text-sm"
														placeholder="Enter your first name"
													/>
												</div>
											</div>
											<div>
												<label
													htmlFor={lastNameId}
													className="block text-sm font-medium text-secondary-900 dark:text-secondary-100"
												>
													Last Name
												</label>
												<div className="mt-1 relative">
													<div className="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
														<User className="h-5 w-5 text-secondary-400" />
													</div>
													<input
														id={lastNameId}
														name="lastName"
														type="text"
														required
														value={formData.lastName}
														onChange={handleInputChange}
														className="appearance-none rounded-md relative block w-full pl-10 pr-3 py-2 border border-secondary-300 placeholder-secondary-500 text-secondary-900 focus:outline-none focus:ring-primary-500 focus:border-primary-500 focus:z-10 sm:text-sm"
														placeholder="Enter your last name"
													/>
												</div>
											</div>
										</div>
										<div>
											<label
												htmlFor={emailId}
												className="block text-sm font-medium text-secondary-900 dark:text-secondary-100"
											>
												Email
											</label>
											<div className="mt-1 relative">
												<input
													id={emailId}
													name="email"
													type="email"
													required
													value={formData.email}
													onChange={handleInputChange}
													className="appearance-none rounded-md relative block w-full pl-10 pr-3 py-2 border border-secondary-300 placeholder-secondary-500 text-secondary-900 focus:outline-none focus:ring-primary-500 focus:border-primary-500 focus:z-10 sm:text-sm"
													placeholder="Enter your email"
												/>
												<div className="absolute left-3 top-1/2 -translate-y-1/2 pointer-events-none z-20 flex items-center">
													<Mail size={20} color="#64748b" strokeWidth={2} />
												</div>
											</div>
										</div>
									</>
								)}

								<div>
									<label
										htmlFor={passwordId}
										className="block text-sm font-medium text-secondary-900 dark:text-secondary-100"
									>
										Password
									</label>
									<div className="mt-1 relative">
										<input
											id={passwordId}
											name="password"
											type={showPassword ? "text" : "password"}
											required
											value={formData.password}
											onChange={handleInputChange}
											className="appearance-none rounded-md relative block w-full pl-10 pr-10 py-2 border border-secondary-300 placeholder-secondary-500 text-secondary-900 focus:outline-none focus:ring-primary-500 focus:border-primary-500 focus:z-10 sm:text-sm"
											placeholder="Enter your password"
										/>
										<div className="absolute left-3 top-1/2 -translate-y-1/2 pointer-events-none z-20 flex items-center">
											<Lock size={20} color="#64748b" strokeWidth={2} />
										</div>
										<div className="absolute right-3 top-1/2 -translate-y-1/2 z-20 flex items-center">
											<button
												type="button"
												onClick={() => setShowPassword(!showPassword)}
												className="bg-transparent border-none cursor-pointer p-1 flex items-center justify-center"
											>
												{showPassword ? (
													<EyeOff size={20} color="#64748b" strokeWidth={2} />
												) : (
													<Eye size={20} color="#64748b" strokeWidth={2} />
												)}
											</button>
										</div>
									</div>
								</div>
							</div>
							)}

							{error && (
								<div className="bg-danger-50 border border-danger-200 rounded-md p-3">
									<div className="flex">
										<AlertCircle size={20} color="#dc2626" strokeWidth={2} />
										<div className="ml-3">
											<p className="text-sm text-danger-700">{error}</p>
										</div>
									</div>
								</div>
							)}

							{/* Only show local auth form if not disabled by OIDC config */}
							{!oidcConfig.disableLocalAuth && (
								<div>
									<button
										type="submit"
										disabled={isLoading}
										className="group relative w-full flex justify-center py-2 px-4 border border-transparent text-sm font-medium rounded-md text-white bg-primary-600 hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500 disabled:opacity-50 disabled:cursor-not-allowed"
									>
										{isLoading ? (
											<div className="flex items-center">
												<div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white mr-2"></div>
												{isSignupMode ? "Creating account..." : "Signing in..."}
											</div>
										) : isSignupMode ? (
											"Create Account"
										) : (
											"Sign in"
										)}
									</button>
								</div>
							)}

							{/* SSO Login Button */}
							{oidcConfig.enabled && (
								<div className={oidcConfig.disableLocalAuth ? "" : "mt-4"}>
									{!oidcConfig.disableLocalAuth && (
										<div className="relative">
											<div className="absolute inset-0 flex items-center">
												<div className="w-full border-t border-secondary-300 dark:border-secondary-600"></div>
											</div>
											<div className="relative flex justify-center text-sm">
												<span className="px-2 bg-white dark:bg-secondary-900 text-secondary-500">
													or
												</span>
											</div>
										</div>
									)}

									<button
										onClick={() =>
											(window.location.href = "/api/v1/auth/oidc/login")
										}
										className={`${oidcConfig.disableLocalAuth ? "" : "mt-4"} w-full flex justify-center py-2 px-4 border border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm text-sm font-medium text-secondary-700 dark:text-secondary-200 bg-white dark:bg-secondary-800 hover:bg-secondary-50 dark:hover:bg-secondary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500`}
										type="button"
									>
										{oidcConfig.buttonText || "Login with SSO"}
									</button>
								</div>
							)}

							{signupEnabled && !oidcConfig.disableLocalAuth && (
								<div className="text-center">
									<p className="text-sm text-secondary-700 dark:text-secondary-300">
										{isSignupMode
											? "Already have an account?"
											: "Don't have an account?"}{" "}
										<button
											type="button"
											onClick={toggleMode}
											className="font-medium text-primary-600 hover:text-primary-500 dark:text-primary-400 dark:hover:text-primary-300 focus:outline-none focus:underline"
										>
											{isSignupMode ? "Sign in" : "Sign up"}
										</button>
									</p>
								</div>
							)}
						</form>
					) : (
						<form className="mt-8 space-y-6" onSubmit={handleTfaSubmit}>
							<div className="text-center">
								<div className="mx-auto h-16 w-16 flex items-center justify-center">
									<img
										src="/assets/favicon.svg"
										alt="PatchMon Logo"
										className="h-16 w-16"
									/>
								</div>
								<h3 className="mt-4 text-lg font-medium text-secondary-900 dark:text-secondary-100">
									Two-Factor Authentication
								</h3>
								<p className="mt-2 text-sm text-secondary-600 dark:text-secondary-400">
									Enter the code from your authenticator app, or use a backup
									code
								</p>
							</div>

							<div>
								<label
									htmlFor={tokenId}
									className="block text-sm font-medium text-secondary-900 dark:text-secondary-100"
								>
									Verification Code
								</label>
								<div className="mt-1">
									<input
										id={tokenId}
										name="token"
										type="text"
										required
										value={tfaData.token}
										onChange={handleTfaInputChange}
										className="appearance-none rounded-md relative block w-full px-3 py-2 border border-secondary-300 placeholder-secondary-500 text-secondary-900 focus:outline-none focus:ring-primary-500 focus:border-primary-500 focus:z-10 sm:text-sm text-center text-lg font-mono tracking-widest uppercase"
										placeholder="Enter code"
										maxLength="6"
										pattern="[A-Z0-9]{6}"
									/>
								</div>
								<p className="mt-1 text-xs text-secondary-500 dark:text-secondary-400">
									Enter a 6-digit TOTP code or a 6-character backup code
								</p>
							</div>

							<div className="flex items-center">
								<input
									id={rememberMeId}
									name="remember_me"
									type="checkbox"
									checked={tfaData.remember_me}
									onChange={handleTfaInputChange}
									className="h-4 w-4 text-primary-600 focus:ring-primary-500 border-secondary-300 rounded"
								/>
								<label
									htmlFor={rememberMeId}
									className="ml-2 block text-sm text-secondary-900 dark:text-secondary-200"
								>
									Remember me on this computer (skip TFA for 30 days)
								</label>
							</div>

							{error && (
								<div className="bg-danger-50 border border-danger-200 rounded-md p-3">
									<div className="flex">
										<AlertCircle size={20} color="#dc2626" strokeWidth={2} />
										<div className="ml-3">
											<p className="text-sm text-danger-700">{error}</p>
										</div>
									</div>
								</div>
							)}

							<div className="space-y-3">
								<button
									type="submit"
									disabled={isLoading || tfaData.token.length !== 6}
									className="group relative w-full flex justify-center py-2 px-4 border border-transparent text-sm font-medium rounded-md text-white bg-primary-600 hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500 disabled:opacity-50 disabled:cursor-not-allowed"
								>
									{isLoading ? (
										<div className="flex items-center">
											<div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white mr-2"></div>
											Verifying...
										</div>
									) : (
										"Verify Code"
									)}
								</button>

								<button
									type="button"
									onClick={handleBackToLogin}
									className="group relative w-full flex justify-center py-2 px-4 border border-secondary-300 dark:border-secondary-600 text-sm font-medium rounded-md text-secondary-700 dark:text-secondary-200 bg-white dark:bg-secondary-800 hover:bg-secondary-50 dark:hover:bg-secondary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500 items-center gap-2"
								>
									<ArrowLeft
										size={16}
										className="text-secondary-700 dark:text-secondary-200"
										strokeWidth={2}
									/>
									Back to Login
								</button>
							</div>
						</form>
					)}
				</div>
			</div>
		</div>
	);
};

export default Login;
