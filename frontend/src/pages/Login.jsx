import { useQuery } from "@tanstack/react-query";
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
import { FaLinkedin, FaYoutube } from "react-icons/fa";

import { useNavigate } from "react-router-dom";
import DiscordIcon from "../components/DiscordIcon";
import { useAuth } from "../contexts/AuthContext";
import { useColorTheme } from "../contexts/ColorThemeContext";
import { authAPI, isCorsError, settingsAPI } from "../utils/api";

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
	const [_githubStars, setGithubStars] = useState(null);
	const [oidcConfig, setOidcConfig] = useState({
		enabled: false,
		buttonText: "Login with SSO",
		disableLocalAuth: false,
	});
	const [oidcProcessed, setOidcProcessed] = useState(false); // Track if OIDC callback was processed
	const canvasRef = useRef(null);
	const { themeConfig } = useColorTheme();

	const navigate = useNavigate();

	// Fetch settings for favicon
	const { data: settings } = useQuery({
		queryKey: ["settings"],
		queryFn: () => settingsAPI.get().then((res) => res.data),
	});

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

	// Auto-redirect to OIDC if enabled and local auth is disabled
	useEffect(() => {
		// Don't auto-redirect if user explicitly logged out
		const explicitLogout = sessionStorage.getItem("explicit_logout");
		if (explicitLogout) {
			return;
		}

		if (oidcConfig.enabled && oidcConfig.disableLocalAuth) {
			window.location.href = "/api/v1/auth/oidc/login";
		}
	}, [oidcConfig]);

	// Handle OIDC callback (tokens are now in httpOnly cookies)
	useEffect(() => {
		// Prevent processing the same callback multiple times
		if (oidcProcessed) {
			return;
		}

		const urlParams = new URLSearchParams(window.location.search);
		const oidcSuccess = urlParams.get("oidc");
		const oidcError = urlParams.get("error");

		if (oidcError) {
			setError(decodeURIComponent(oidcError));
			window.history.replaceState({}, document.title, "/login");
			setOidcProcessed(true);
			return;
		}

		if (oidcSuccess === "success") {
			setOidcProcessed(true);
			sessionStorage.removeItem("explicit_logout");
			window.location.href = "/";
		}
	}, [oidcProcessed]);

	// Fetch latest release and social media stats
	useEffect(() => {
		// Only fetch if the setting allows it
		if (!showGithubVersionOnLogin) {
			return;
		}

		const abortController = new AbortController();
		let isMounted = true;

		const fetchData = async () => {
			try {
				// Try to get cached release data first
				const cachedRelease = localStorage.getItem("githubLatestRelease");
				const cacheTime = localStorage.getItem("githubReleaseCacheTime");
				const now = Date.now();

				// Load cached data immediately
				if (cachedRelease && isMounted) {
					try {
						setLatestRelease(JSON.parse(cachedRelease));
					} catch (_e) {
						localStorage.removeItem("githubLatestRelease");
					}
				}
				const cachedStars = localStorage.getItem("githubStarsCount");
				if (cachedStars && isMounted) {
					setGithubStars(parseInt(cachedStars, 10));
				}

				// Use cache if less than 1 hour old
				const shouldFetchFresh =
					!cacheTime || now - parseInt(cacheTime, 10) >= 3600000;

				// Fetch repository info (includes star count) - still from GitHub for stars
				try {
					const repoResponse = await fetch(
						"https://api.github.com/repos/PatchMon/PatchMon",
						{
							headers: {
								Accept: "application/vnd.github.v3+json",
							},
							signal: abortController.signal,
						},
					);

					if (repoResponse.ok && isMounted) {
						const repoData = await repoResponse.json();
						setGithubStars(repoData.stargazers_count);
						localStorage.setItem(
							"githubStarsCount",
							repoData.stargazers_count.toString(),
						);
					}
				} catch (_repoError) {
					// Silently fail - stars are optional
				}

				// Fetch latest release from GitHub API (for release notes, published date, etc.)
				if (shouldFetchFresh) {
					try {
						const releaseResponse = await fetch(
							"https://api.github.com/repos/PatchMon/PatchMon/releases/latest",
							{
								headers: {
									Accept: "application/vnd.github.v3+json",
								},
								signal: abortController.signal,
							},
						);

						if (releaseResponse.ok && isMounted) {
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
							localStorage.setItem("githubReleaseCacheTime", now.toString());
						}
					} catch (releaseError) {
						// Ignore abort errors
						if (releaseError.name === "AbortError") return;
						console.error("Failed to fetch release from GitHub:", releaseError);
						// Will use cached data if available
					}
				}
			} catch (error) {
				// Ignore abort errors
				if (error.name === "AbortError") return;

				console.error("Failed to fetch GitHub data:", error);
				// Set fallback data if nothing cached
				const cachedRelease = localStorage.getItem("githubLatestRelease");
				if (!cachedRelease && isMounted) {
					setLatestRelease({
						version: "v1.3.0",
						name: "Latest Release",
						publishedAt: "Recently",
						body: "Monitor and manage your Linux package updates",
					});
				}
			}
		};

		fetchData();

		return () => {
			isMounted = false;
			abortController.abort();
		};
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
			// Clear the token input for security (preserve remember_me preference)
			setTfaData((prev) => ({ ...prev, token: "" }));
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
										<div className="flex items-center gap-1">
											<Star className="h-3.5 w-3.5 fill-current text-yellow-400" />
											<span className="text-sm font-medium text-white">
												2.1K
											</span>
										</div>
									</a>

									{/* Buy Me a Coffee */}
									<a
										href="https://buymeacoffee.com/iby___"
										target="_blank"
										rel="noopener noreferrer"
										className="flex items-center justify-center gap-1.5 px-3 h-10 bg-white/10 hover:bg-white/20 backdrop-blur-sm rounded-lg transition-colors border border-white/10"
										title="Buy Me a Coffee"
									>
										<svg
											className="h-5 w-5 text-yellow-500"
											viewBox="0 0 900 1300"
											fill="currentColor"
										>
											<title>Buy Me a Coffee</title>
											<path d="M879.567 341.849L872.53 306.352C866.215 274.503 851.882 244.409 819.19 232.898C808.711 229.215 796.821 227.633 788.786 220.01C780.751 212.388 778.376 200.55 776.518 189.572C773.076 169.423 769.842 149.257 766.314 129.143C763.269 111.85 760.86 92.4243 752.928 76.56C742.604 55.2584 721.182 42.8009 699.88 34.559C688.965 30.4844 677.826 27.0375 666.517 24.2352C613.297 10.1947 557.342 5.03277 502.591 2.09047C436.875 -1.53577 370.983 -0.443234 305.422 5.35968C256.625 9.79894 205.229 15.1674 158.858 32.0469C141.91 38.224 124.445 45.6399 111.558 58.7341C95.7448 74.8221 90.5829 99.7026 102.128 119.765C110.336 134.012 124.239 144.078 138.985 150.737C158.192 159.317 178.251 165.846 198.829 170.215C256.126 182.879 315.471 187.851 374.007 189.968C438.887 192.586 503.87 190.464 568.44 183.618C584.408 181.863 600.347 179.758 616.257 177.304C634.995 174.43 647.022 149.928 641.499 132.859C634.891 112.453 617.134 104.538 597.055 107.618C594.095 108.082 591.153 108.512 588.193 108.942L586.06 109.252C579.257 110.113 572.455 110.915 565.653 111.661C551.601 113.175 537.515 114.414 523.394 115.378C491.768 117.58 460.057 118.595 428.363 118.647C397.219 118.647 366.058 117.769 334.983 115.722C320.805 114.793 306.661 113.611 292.552 112.177C286.134 111.506 279.733 110.801 273.333 110.009L267.241 109.235L265.917 109.046L259.602 108.134C246.697 106.189 233.792 103.953 221.025 101.251C219.737 100.965 218.584 100.249 217.758 99.2193C216.932 98.1901 216.482 96.9099 216.482 95.5903C216.482 94.2706 216.932 92.9904 217.758 91.9612C218.584 90.9319 219.737 90.2152 221.025 89.9293H221.266C232.33 87.5721 243.479 85.5589 254.663 83.8038C258.392 83.2188 262.131 82.6453 265.882 82.0832H265.985C272.988 81.6186 280.026 80.3625 286.994 79.5366C347.624 73.2301 408.614 71.0801 469.538 73.1014C499.115 73.9618 528.676 75.6996 558.116 78.6935C564.448 79.3474 570.746 80.0357 577.043 80.8099C579.452 81.1025 581.878 81.4465 584.305 81.7391L589.191 82.4445C603.438 84.5667 617.61 87.1419 631.708 90.1703C652.597 94.7128 679.422 96.1925 688.713 119.077C691.673 126.338 693.015 134.408 694.649 142.03L696.732 151.752C696.786 151.926 696.826 152.105 696.852 152.285C701.773 175.227 706.7 198.169 711.632 221.111C711.994 222.806 712.002 224.557 711.657 226.255C711.312 227.954 710.621 229.562 709.626 230.982C708.632 232.401 707.355 233.6 705.877 234.504C704.398 235.408 702.75 235.997 701.033 236.236H700.895L697.884 236.649L694.908 237.044C685.478 238.272 676.038 239.419 666.586 240.486C647.968 242.608 629.322 244.443 610.648 245.992C573.539 249.077 536.356 251.102 499.098 252.066C480.114 252.57 461.135 252.806 442.162 252.771C366.643 252.712 291.189 248.322 216.173 239.625C208.051 238.662 199.93 237.629 191.808 236.58C198.106 237.389 187.231 235.96 185.029 235.651C179.867 234.928 174.705 234.177 169.543 233.397C152.216 230.798 134.993 227.598 117.7 224.793C96.7944 221.352 76.8005 223.073 57.8906 233.397C42.3685 241.891 29.8055 254.916 21.8776 270.735C13.7217 287.597 11.2956 305.956 7.64786 324.075C4.00009 342.193 -1.67805 361.688 0.472751 380.288C5.10128 420.431 33.165 453.054 73.5313 460.35C111.506 467.232 149.687 472.807 187.971 477.556C338.361 495.975 490.294 498.178 641.155 484.129C653.44 482.982 665.708 481.732 677.959 480.378C681.786 479.958 685.658 480.398 689.292 481.668C692.926 482.938 696.23 485.005 698.962 487.717C701.694 490.429 703.784 493.718 705.08 497.342C706.377 500.967 706.846 504.836 706.453 508.665L702.633 545.797C694.936 620.828 687.239 695.854 679.542 770.874C671.513 849.657 663.431 928.434 655.298 1007.2C653.004 1029.39 650.71 1051.57 648.416 1073.74C646.213 1095.58 645.904 1118.1 641.757 1139.68C635.218 1173.61 612.248 1194.45 578.73 1202.07C548.022 1209.06 516.652 1212.73 485.161 1213.01C450.249 1213.2 415.355 1211.65 380.443 1211.84C343.173 1212.05 297.525 1208.61 268.756 1180.87C243.479 1156.51 239.986 1118.36 236.545 1085.37C231.957 1041.7 227.409 998.039 222.9 954.381L197.607 711.615L181.244 554.538C180.968 551.94 180.693 549.376 180.435 546.76C178.473 528.023 165.207 509.681 144.301 510.627C126.407 511.418 106.069 526.629 108.168 546.76L120.298 663.214L145.385 904.104C152.532 972.528 159.661 1040.96 166.773 1109.41C168.15 1122.52 169.44 1135.67 170.885 1148.78C178.749 1220.43 233.465 1259.04 301.224 1269.91C340.799 1276.28 381.337 1277.59 421.497 1278.24C472.979 1279.07 524.977 1281.05 575.615 1271.72C650.653 1257.95 706.952 1207.85 714.987 1130.13C717.282 1107.69 719.576 1085.25 721.87 1062.8C729.498 988.559 737.115 914.313 744.72 840.061L769.601 597.451L781.009 486.263C781.577 480.749 783.905 475.565 787.649 471.478C791.392 467.391 796.352 464.617 801.794 463.567C823.25 459.386 843.761 452.245 859.023 435.916C883.318 409.918 888.153 376.021 879.567 341.849ZM72.4301 365.835C72.757 365.68 72.1548 368.484 71.8967 369.792C71.8451 367.813 71.9483 366.058 72.4301 365.835ZM74.5121 381.94C74.6842 381.819 75.2003 382.508 75.7337 383.334C74.925 382.576 74.4089 382.009 74.4949 381.94H74.5121ZM76.5597 384.641C77.2996 385.897 77.6953 386.689 76.5597 384.641V384.641ZM80.672 387.979H80.7752C80.7752 388.1 80.9645 388.22 81.0333 388.341C80.9192 388.208 80.7925 388.087 80.6548 387.979H80.672ZM800.796 382.989C793.088 390.319 781.473 393.726 769.996 395.43C641.292 414.529 510.713 424.199 380.597 419.932C287.476 416.749 195.336 406.407 103.144 393.382C94.1102 392.109 84.3197 390.457 78.1082 383.798C66.4078 371.237 72.1548 345.944 75.2003 330.768C77.9878 316.865 83.3218 298.334 99.8572 296.355C125.667 293.327 155.64 304.218 181.175 308.09C211.917 312.781 242.774 316.538 273.745 319.36C405.925 331.405 540.325 329.529 671.92 311.91C695.906 308.686 719.805 304.941 743.619 300.674C764.835 296.871 788.356 289.731 801.175 311.703C809.967 326.673 811.137 346.701 809.778 363.615C809.359 370.984 806.139 377.915 800.779 382.989H800.796Z" />
										</svg>
									</a>

									{/* Discord */}
									<a
										href="https://patchmon.net/discord"
										target="_blank"
										rel="noopener noreferrer"
										className="flex items-center justify-center gap-1.5 px-3 h-10 bg-white/10 hover:bg-white/20 backdrop-blur-sm rounded-lg transition-colors border border-white/10"
										title="Discord Community"
									>
										<DiscordIcon className="h-5 w-5 text-white" />
										<span className="text-sm font-medium text-white">500</span>
									</a>

									{/* LinkedIn */}
									<a
										href="https://linkedin.com/company/patchmon"
										target="_blank"
										rel="noopener noreferrer"
										className="flex items-center justify-center gap-1.5 px-3 h-10 bg-white/10 hover:bg-white/20 backdrop-blur-sm rounded-lg transition-colors border border-white/10"
										title="LinkedIn Company Page"
									>
										<FaLinkedin className="h-5 w-5 text-[#0077B5]" />
										<span className="text-sm font-medium text-white">250</span>
									</a>

									{/* YouTube */}
									<a
										href="https://youtube.com/@patchmonTV"
										target="_blank"
										rel="noopener noreferrer"
										className="flex items-center justify-center gap-1.5 px-3 h-10 bg-white/10 hover:bg-white/20 backdrop-blur-sm rounded-lg transition-colors border border-white/10"
										title="YouTube Channel"
									>
										<FaYoutube className="h-5 w-5 text-[#FF0000]" />
										<span className="text-sm font-medium text-white">100</span>
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

									{/* Documentation */}
									<a
										href="https://docs.patchmon.net"
										target="_blank"
										rel="noopener noreferrer"
										className="flex items-center justify-center w-10 h-10 bg-white/10 hover:bg-white/20 backdrop-blur-sm rounded-lg transition-colors border border-white/10"
										title="Documentation"
									>
										<BookOpen className="h-5 w-5 text-white" />
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
								src={
									settings?.favicon
										? `${(() => {
												const parts = settings.favicon.split("/");
												const filename = parts.pop();
												const directory = parts.join("/");
												const encodedPath = directory
													? `${directory}/${encodeURIComponent(filename)}`
													: encodeURIComponent(filename);
												return `${encodedPath}?v=${
													settings?.updated_at
														? new Date(settings.updated_at).getTime()
														: Date.now()
												}`;
											})()}`
										: "/assets/favicon.svg"
								}
								alt="PatchMon Logo"
								className="h-16 w-16"
								onError={(e) => {
									e.target.src = "/assets/favicon.svg";
								}}
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
										onClick={() => {
											sessionStorage.removeItem("explicit_logout");
											window.location.href = "/api/v1/auth/oidc/login";
										}}
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
										src={
											settings?.favicon
												? `${settings.favicon}?v=${
														settings?.updated_at
															? new Date(settings.updated_at).getTime()
															: Date.now()
													}`
												: "/assets/favicon.svg"
										}
										alt="PatchMon Logo"
										className="h-16 w-16"
										onError={(e) => {
											e.target.src = "/assets/favicon.svg";
										}}
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
