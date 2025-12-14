import {
	AlertCircle,
	BookOpen,
	CheckCircle,
	Container,
	Copy,
	Eye,
	EyeOff,
	Plus,
	Server,
	Trash2,
	X,
} from "lucide-react";
import { useEffect, useId, useState } from "react";
import SettingsLayout from "../../components/SettingsLayout";
import api from "../../utils/api";

const Integrations = () => {
	// Generate unique IDs for form elements
	const token_name_id = useId();
	const token_key_id = useId();
	const token_secret_id = useId();
	const token_base64_id = useId();
	const gethomepage_config_id = useId();

	const [activeTab, setActiveTab] = useState("auto-enrollment");
	const [tokens, setTokens] = useState([]);
	const [host_groups, setHostGroups] = useState([]);
	const [loading, setLoading] = useState(true);
	const [show_create_modal, setShowCreateModal] = useState(false);
	const [show_edit_modal, setShowEditModal] = useState(false);
	const [edit_token, setEditToken] = useState(null);
	const [new_token, setNewToken] = useState(null);
	const [show_secret, setShowSecret] = useState(false);
	const [server_url, setServerUrl] = useState("");
	const [force_proxmox_install, setForceProxmoxInstall] = useState(false);
	const [usage_type, setUsageType] = useState("proxmox-lxc");
	const [selected_script_type, setSelectedScriptType] = useState("proxmox-lxc");
	const [curl_flags, setCurlFlags] = useState("-s");

	// Form state
	const [form_data, setFormData] = useState({
		token_name: "",
		max_hosts_per_day: 100,
		default_host_group_id: "",
		allowed_ip_ranges: "",
		expires_at: "",
		scopes: {
			host: [],
		},
	});

	const [copy_success, setCopySuccess] = useState({});

	// Helper function to build enrollment URL with optional force flag and selected type
	const getEnrollmentUrl = (scriptType = selected_script_type) => {
		const baseUrl = `${server_url}/api/v1/auto-enrollment/script?type=${scriptType}&token_key=${new_token.token_key}&token_secret=${new_token.token_secret}`;
		return force_proxmox_install ? `${baseUrl}&force=true` : baseUrl;
	};

	const handleTabChange = (tabName) => {
		setActiveTab(tabName);
	};

	const toggle_scope_action = (resource, action) => {
		setFormData((prev) => {
			const current_scopes = prev.scopes || { [resource]: [] };
			const resource_scopes = current_scopes[resource] || [];

			const updated_scopes = resource_scopes.includes(action)
				? resource_scopes.filter((a) => a !== action)
				: [...resource_scopes, action];

			return {
				...prev,
				scopes: {
					...current_scopes,
					[resource]: updated_scopes,
				},
			};
		});
	};

	// biome-ignore lint/correctness/useExhaustiveDependencies: Only run on mount
	useEffect(() => {
		load_tokens();
		load_host_groups();
		load_server_url();
	}, []);

	const load_tokens = async () => {
		try {
			setLoading(true);
			const response = await api.get("/auto-enrollment/tokens");
			setTokens(response.data);
		} catch (error) {
			console.error("Failed to load tokens:", error);
		} finally {
			setLoading(false);
		}
	};

	const load_host_groups = async () => {
		try {
			const response = await api.get("/host-groups");
			setHostGroups(response.data);
		} catch (error) {
			console.error("Failed to load host groups:", error);
		}
	};

	const load_server_url = async () => {
		try {
			const response = await api.get("/settings");
			setServerUrl(response.data.server_url || window.location.origin);
			// Set curl flags based on SSL settings
			setCurlFlags(response.data.ignore_ssl_self_signed ? "-sk" : "-s");
		} catch (error) {
			console.error("Failed to load server URL:", error);
			setServerUrl(window.location.origin);
			setCurlFlags("-s");
		}
	};

	const create_token = async (e) => {
		e.preventDefault();

		try {
			// Determine integration type based on active tab or usage_type
			let integration_type = "proxmox-lxc";
			if (activeTab === "gethomepage") {
				integration_type = "gethomepage";
			} else if (activeTab === "auto-enrollment") {
				// Use the usage_type selected in the modal
				integration_type = usage_type;
			}

			const data = {
				token_name: form_data.token_name,
				max_hosts_per_day: Number.parseInt(form_data.max_hosts_per_day, 10),
				allowed_ip_ranges: form_data.allowed_ip_ranges
					? form_data.allowed_ip_ranges.split(",").map((ip) => ip.trim())
					: [],
				metadata: {
					integration_type: integration_type,
				},
			};

			// Only add optional fields if they have values
			if (form_data.default_host_group_id) {
				data.default_host_group_id = form_data.default_host_group_id;
			}
			if (form_data.expires_at) {
				data.expires_at = form_data.expires_at;
			}

			// Add scopes for API credentials
			if (usage_type === "api" && form_data.scopes) {
				data.scopes = form_data.scopes;
			}

			const response = await api.post("/auto-enrollment/tokens", data);
			setNewToken(response.data.token);
			setShowCreateModal(false);
			load_tokens();
			// Keep usage_type so the success modal can use it

			// Reset form
			setFormData({
				token_name: "",
				max_hosts_per_day: 100,
				default_host_group_id: "",
				allowed_ip_ranges: "",
				expires_at: "",
				scopes: {
					host: [],
				},
			});
		} catch (error) {
			console.error("Failed to create token:", error);
			const error_message = error.response?.data?.errors
				? error.response.data.errors.map((e) => e.msg).join(", ")
				: error.response?.data?.error || "Failed to create token";
			alert(error_message);
		}
	};

	const delete_token = async (id, name) => {
		if (
			!confirm(
				`Are you sure you want to delete the token "${name}"? This action cannot be undone.`,
			)
		) {
			return;
		}

		try {
			await api.delete(`/auto-enrollment/tokens/${id}`);
			load_tokens();
		} catch (error) {
			console.error("Failed to delete token:", error);
			alert(error.response?.data?.error || "Failed to delete token");
		}
	};

	const toggle_token_active = async (id, current_status) => {
		try {
			await api.patch(`/auto-enrollment/tokens/${id}`, {
				is_active: !current_status,
			});
			load_tokens();
		} catch (error) {
			console.error("Failed to toggle token:", error);
			alert(error.response?.data?.error || "Failed to toggle token");
		}
	};

	const open_edit_modal = (token) => {
		setEditToken(token);
		setFormData({
			token_name: token.token_name,
			max_hosts_per_day: token.max_hosts_per_day || 100,
			default_host_group_id: token.default_host_group_id || "",
			allowed_ip_ranges: token.allowed_ip_ranges?.join(", ") || "",
			expires_at: token.expires_at
				? new Date(token.expires_at).toISOString().slice(0, 16)
				: "",
			scopes: token.scopes || { host: [] },
		});
		setShowEditModal(true);
	};

	const update_token = async (e) => {
		e.preventDefault();

		try {
			const data = {
				allowed_ip_ranges: form_data.allowed_ip_ranges
					? form_data.allowed_ip_ranges.split(",").map((ip) => ip.trim())
					: [],
			};

			// Add expiration if provided
			if (form_data.expires_at) {
				data.expires_at = form_data.expires_at;
			}

			// Add scopes for API credentials
			if (
				edit_token?.metadata?.integration_type === "api" &&
				form_data.scopes
			) {
				data.scopes = form_data.scopes;
			}

			await api.patch(`/auto-enrollment/tokens/${edit_token.id}`, data);
			setShowEditModal(false);
			setEditToken(null);
			load_tokens();

			// Reset form
			setFormData({
				token_name: "",
				max_hosts_per_day: 100,
				default_host_group_id: "",
				allowed_ip_ranges: "",
				expires_at: "",
				scopes: {
					host: [],
				},
			});
		} catch (error) {
			console.error("Failed to update token:", error);
			const error_message = error.response?.data?.errors
				? error.response.data.errors.map((e) => e.msg).join(", ")
				: error.response?.data?.error || "Failed to update token";
			alert(error_message);
		}
	};

	const copy_to_clipboard = async (text, key) => {
		// Check if Clipboard API is available
		if (navigator.clipboard && window.isSecureContext) {
			try {
				await navigator.clipboard.writeText(text);
				setCopySuccess({ ...copy_success, [key]: true });
				setTimeout(() => {
					setCopySuccess({ ...copy_success, [key]: false });
				}, 2000);
				return;
			} catch (error) {
				console.error("Clipboard API failed:", error);
				// Fall through to fallback method
			}
		}

		// Fallback method for older browsers or non-secure contexts
		try {
			const textArea = document.createElement("textarea");
			textArea.value = text;
			textArea.style.position = "fixed";
			textArea.style.left = "-999999px";
			textArea.style.top = "-999999px";
			document.body.appendChild(textArea);
			textArea.focus();
			textArea.select();

			const successful = document.execCommand("copy");
			document.body.removeChild(textArea);

			if (successful) {
				setCopySuccess({ ...copy_success, [key]: true });
				setTimeout(() => {
					setCopySuccess({ ...copy_success, [key]: false });
				}, 2000);
			} else {
				console.error("Fallback copy failed");
				alert("Failed to copy to clipboard. Please copy manually.");
			}
		} catch (fallbackError) {
			console.error("Fallback copy failed:", fallbackError);
			alert("Failed to copy to clipboard. Please copy manually.");
		}
	};

	const format_date = (date_string) => {
		if (!date_string) return "Never";
		return new Date(date_string).toLocaleString();
	};

	return (
		<SettingsLayout>
			<div className="space-y-6">
				{/* Header */}
				<div>
					<h1 className="text-xl md:text-2xl font-bold text-secondary-900 dark:text-white">
						Integrations
					</h1>
					<p className="mt-1 text-xs md:text-sm text-secondary-600 dark:text-secondary-400">
						Manage auto-enrollment tokens for Proxmox and other integrations
					</p>
				</div>

				{/* Tabs Navigation */}
				<div className="bg-white dark:bg-secondary-800 border border-secondary-200 dark:border-secondary-600 rounded-lg overflow-hidden">
					{/* Mobile Button Navigation */}
					<div className="md:hidden space-y-2 p-4">
						<button
							type="button"
							onClick={() => handleTabChange("auto-enrollment")}
							className={`w-full flex items-center justify-between px-4 py-3 rounded-md font-medium text-sm transition-colors ${
								activeTab === "auto-enrollment"
									? "bg-primary-50 dark:bg-primary-900/20 text-primary-600 dark:text-primary-400 border border-primary-200 dark:border-primary-800"
									: "bg-secondary-50 dark:bg-secondary-700 text-secondary-700 dark:text-secondary-300 border border-secondary-200 dark:border-secondary-600 hover:bg-secondary-100 dark:hover:bg-secondary-600"
							}`}
						>
							<span>Auto-Enrollment & API</span>
							{activeTab === "auto-enrollment" && (
								<CheckCircle className="h-5 w-5 text-primary-600 dark:text-primary-400" />
							)}
						</button>
						<button
							type="button"
							onClick={() => handleTabChange("gethomepage")}
							className={`w-full flex items-center justify-between px-4 py-3 rounded-md font-medium text-sm transition-colors ${
								activeTab === "gethomepage"
									? "bg-primary-50 dark:bg-primary-900/20 text-primary-600 dark:text-primary-400 border border-primary-200 dark:border-primary-800"
									: "bg-secondary-50 dark:bg-secondary-700 text-secondary-700 dark:text-secondary-300 border border-secondary-200 dark:border-secondary-600 hover:bg-secondary-100 dark:hover:bg-secondary-600"
							}`}
						>
							<span>GetHomepage</span>
							{activeTab === "gethomepage" && (
								<CheckCircle className="h-5 w-5 text-primary-600 dark:text-primary-400" />
							)}
						</button>
						<button
							type="button"
							onClick={() => handleTabChange("docker")}
							className={`w-full flex items-center justify-between px-4 py-3 rounded-md font-medium text-sm transition-colors ${
								activeTab === "docker"
									? "bg-primary-50 dark:bg-primary-900/20 text-primary-600 dark:text-primary-400 border border-primary-200 dark:border-primary-800"
									: "bg-secondary-50 dark:bg-secondary-700 text-secondary-700 dark:text-secondary-300 border border-secondary-200 dark:border-secondary-600 hover:bg-secondary-100 dark:hover:bg-secondary-600"
							}`}
						>
							<span>Docker</span>
							{activeTab === "docker" && (
								<CheckCircle className="h-5 w-5 text-primary-600 dark:text-primary-400" />
							)}
						</button>
					</div>

					{/* Desktop Tab Navigation */}
					<div className="hidden md:block border-b border-secondary-200 dark:border-secondary-600">
						<div className="flex">
							<button
								type="button"
								onClick={() => handleTabChange("auto-enrollment")}
								className={`px-6 py-3 text-sm font-medium ${
									activeTab === "auto-enrollment"
										? "text-primary-600 dark:text-primary-400 border-b-2 border-primary-500 bg-primary-50 dark:bg-primary-900/20"
										: "text-secondary-500 dark:text-secondary-400 hover:text-secondary-700 dark:hover:text-secondary-300 hover:bg-secondary-50 dark:hover:bg-secondary-700/50"
								}`}
							>
								Auto-Enrollment & API
							</button>
							<button
								type="button"
								onClick={() => handleTabChange("gethomepage")}
								className={`px-6 py-3 text-sm font-medium ${
									activeTab === "gethomepage"
										? "text-primary-600 dark:text-primary-400 border-b-2 border-primary-500 bg-primary-50 dark:bg-primary-900/20"
										: "text-secondary-500 dark:text-secondary-400 hover:text-secondary-700 dark:hover:text-secondary-300 hover:bg-secondary-50 dark:hover:bg-secondary-700/50"
								}`}
							>
								GetHomepage
							</button>
							<button
								type="button"
								onClick={() => handleTabChange("docker")}
								className={`px-6 py-3 text-sm font-medium ${
									activeTab === "docker"
										? "text-primary-600 dark:text-primary-400 border-b-2 border-primary-500 bg-primary-50 dark:bg-primary-900/20"
										: "text-secondary-500 dark:text-secondary-400 hover:text-secondary-700 dark:hover:text-secondary-300 hover:bg-secondary-50 dark:hover:bg-secondary-700/50"
								}`}
							>
								Docker
							</button>
							{/* Future tabs can be added here */}
						</div>
					</div>

					{/* Tab Content */}
					<div className="p-4 md:p-6">
						{/* Auto-Enrollment & API Tab */}
						{activeTab === "auto-enrollment" && (
							<div className="space-y-6">
								{/* Header with New Token Button */}
								<div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
									<div className="flex items-center gap-3 flex-1 min-w-0">
										<div className="w-10 h-10 bg-primary-100 dark:bg-primary-900 rounded-lg flex items-center justify-center flex-shrink-0">
											<Server className="h-5 w-5 text-primary-600 dark:text-primary-400" />
										</div>
										<div className="min-w-0">
											<h3 className="text-base md:text-lg font-semibold text-secondary-900 dark:text-white">
												Auto-Enrollment & API Credentials
											</h3>
											<p className="text-xs md:text-sm text-secondary-600 dark:text-secondary-400">
												Manage tokens for Proxmox LXC auto-enrollment and API
												access
											</p>
										</div>
									</div>
									<button
										type="button"
										onClick={() => setShowCreateModal(true)}
										className="btn-primary flex items-center gap-2 w-full sm:w-auto justify-center sm:justify-start"
									>
										<Plus className="h-4 w-4" />
										New Token
									</button>
								</div>

								{/* Token List */}
								{loading ? (
									<div className="text-center py-8">
										<div className="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600" />
									</div>
								) : tokens.filter(
										(token) =>
											token.metadata?.integration_type === "proxmox-lxc" ||
											token.metadata?.integration_type === "api",
									).length === 0 ? (
									<div className="text-center py-8 text-secondary-600 dark:text-secondary-400">
										<p>No auto-enrollment or API tokens created yet.</p>
										<p className="text-sm mt-2">
											Create a token to enable Proxmox auto-enrollment or API
											access.
										</p>
									</div>
								) : (
									<div className="space-y-3">
										{tokens
											.filter(
												(token) =>
													token.metadata?.integration_type === "proxmox-lxc" ||
													token.metadata?.integration_type === "api",
											)
											.map((token) => (
												<div
													key={token.id}
													className="border border-secondary-200 dark:border-secondary-600 rounded-lg p-4 hover:border-primary-300 dark:hover:border-primary-700 transition-colors"
												>
													<div className="flex justify-between items-start">
														<div className="flex-1">
															<div className="flex items-center gap-2 flex-wrap">
																<h4 className="font-medium text-secondary-900 dark:text-white">
																	{token.token_name}
																</h4>
																{token.metadata?.integration_type ===
																"proxmox-lxc" ? (
																	<span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200">
																		Proxmox LXC
																	</span>
																) : (
																	<span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">
																		API
																	</span>
																)}
																{token.is_active ? (
																	<span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">
																		Active
																	</span>
																) : (
																	<span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-secondary-100 text-secondary-800 dark:bg-secondary-700 dark:text-secondary-200">
																		Inactive
																	</span>
																)}
															</div>
															<div className="mt-2 space-y-1 text-sm text-secondary-600 dark:text-secondary-400">
																<div className="flex items-center gap-2">
																	<span className="font-mono text-xs bg-secondary-100 dark:bg-secondary-700 px-2 py-1 rounded">
																		{token.token_key}
																	</span>
																	<button
																		type="button"
																		onClick={() =>
																			copy_to_clipboard(
																				token.token_key,
																				`key-${token.id}`,
																			)
																		}
																		className="text-primary-600 hover:text-primary-700 dark:text-primary-400"
																	>
																		{copy_success[`key-${token.id}`] ? (
																			<CheckCircle className="h-4 w-4" />
																		) : (
																			<Copy className="h-4 w-4" />
																		)}
																	</button>
																</div>
																{token.metadata?.integration_type ===
																	"proxmox-lxc" && (
																	<p>
																		Usage: {token.hosts_created_today}/
																		{token.max_hosts_per_day} hosts today
																	</p>
																)}
																{token.metadata?.integration_type ===
																	"proxmox-lxc" &&
																	token.host_groups && (
																		<p>
																			Default Group:{" "}
																			<span
																				className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium"
																				style={{
																					backgroundColor: `${token.host_groups.color}20`,
																					color: token.host_groups.color,
																				}}
																			>
																				{token.host_groups.name}
																			</span>
																		</p>
																	)}
																{token.metadata?.integration_type === "api" &&
																	token.scopes && (
																		<p>
																			Scopes:{" "}
																			{Object.entries(token.scopes)
																				.map(
																					([resource, actions]) =>
																						`${resource}: ${Array.isArray(actions) ? actions.join(", ") : actions}`,
																				)
																				.join(" | ")}
																		</p>
																	)}
																{token.allowed_ip_ranges?.length > 0 && (
																	<p>
																		Allowed IPs:{" "}
																		{token.allowed_ip_ranges.join(", ")}
																	</p>
																)}
																<p>Created: {format_date(token.created_at)}</p>
																{token.last_used_at && (
																	<p>
																		Last Used: {format_date(token.last_used_at)}
																	</p>
																)}
																{token.expires_at && (
																	<p>
																		Expires: {format_date(token.expires_at)}
																		{new Date(token.expires_at) <
																			new Date() && (
																			<span className="ml-2 text-red-600 dark:text-red-400">
																				(Expired)
																			</span>
																		)}
																	</p>
																)}
															</div>
														</div>
														<div className="flex items-center gap-2 flex-wrap w-full sm:w-auto">
															{token.metadata?.integration_type === "api" && (
																<button
																	type="button"
																	onClick={() => open_edit_modal(token)}
																	className="px-3 py-1 text-xs md:text-sm rounded bg-blue-100 text-blue-700 hover:bg-blue-200 dark:bg-blue-900 dark:text-blue-300"
																>
																	Edit
																</button>
															)}
															<button
																type="button"
																onClick={() =>
																	toggle_token_active(token.id, token.is_active)
																}
																className={`px-3 py-1 text-xs md:text-sm rounded ${
																	token.is_active
																		? "bg-secondary-100 text-secondary-700 hover:bg-secondary-200 dark:bg-secondary-700 dark:text-secondary-300"
																		: "bg-green-100 text-green-700 hover:bg-green-200 dark:bg-green-900 dark:text-green-300"
																}`}
															>
																{token.is_active ? "Disable" : "Enable"}
															</button>
															<button
																type="button"
																onClick={() =>
																	delete_token(token.id, token.token_name)
																}
																className="text-red-600 hover:text-red-800 dark:text-red-400 p-2"
															>
																<Trash2 className="h-4 w-4" />
															</button>
														</div>
													</div>
												</div>
											))}
									</div>
								)}

								{/* Documentation Section */}
								<div className="bg-primary-50 dark:bg-primary-900/20 border border-primary-200 dark:border-primary-800 rounded-lg p-4 md:p-6">
									<h3 className="text-base md:text-lg font-semibold text-primary-900 dark:text-primary-200 mb-4">
										Documentation
									</h3>
									<div className="grid grid-cols-1 md:grid-cols-2 gap-4">
										{/* Proxmox Documentation */}
										<div className="border border-primary-200 dark:border-primary-700 rounded-lg p-4 bg-white dark:bg-secondary-800">
											<div className="flex items-center gap-2 mb-3">
												<Server className="h-5 w-5 text-blue-600 dark:text-blue-400" />
												<h4 className="font-semibold text-secondary-900 dark:text-white">
													Auto-enrollment
												</h4>
											</div>
											<p className="text-sm text-secondary-600 dark:text-secondary-400 mb-3">
												Automatically discover and enroll hosts from Proxmox or
												direct enrollment.
											</p>
											<a
												href="https://docs.patchmon.net/books/patchmon-application-documentation/page/proxmox-lxc-auto-enrollment-guide"
												target="_blank"
												rel="noopener noreferrer"
												className="inline-flex items-center gap-2 px-3 py-2 bg-primary-600 hover:bg-primary-700 dark:bg-primary-500 dark:hover:bg-primary-600 text-white rounded-lg text-sm transition-colors"
											>
												<BookOpen className="h-4 w-4" />
												View Guide
											</a>
										</div>

										{/* API Documentation */}
										<div className="border border-primary-200 dark:border-primary-700 rounded-lg p-4 bg-white dark:bg-secondary-800">
											<div className="flex items-center gap-2 mb-3">
												<Server className="h-5 w-5 text-green-600 dark:text-green-400" />
												<h4 className="font-semibold text-secondary-900 dark:text-white">
													Scoped credentials
												</h4>
											</div>
											<p className="text-sm text-secondary-600 dark:text-secondary-400 mb-3">
												Programmatic access to PatchMon data with granular
												scope-based permissions.
											</p>
											<a
												href="https://docs.patchmon.net/books/patchmon-application-documentation/page/integration-api-documentation"
												target="_blank"
												rel="noopener noreferrer"
												className="inline-flex items-center gap-2 px-3 py-2 bg-primary-600 hover:bg-primary-700 dark:bg-primary-500 dark:hover:bg-primary-600 text-white rounded-lg text-sm transition-colors"
											>
												<BookOpen className="h-4 w-4" />
												View Guide
											</a>
										</div>
									</div>
								</div>
							</div>
						)}

						{/* GetHomepage Tab */}
						{activeTab === "gethomepage" && (
							<div className="space-y-6">
								{/* Header with New API Key Button */}
								<div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
									<div className="flex items-center gap-3 flex-1 min-w-0">
										<div className="w-10 h-10 bg-primary-100 dark:bg-primary-900 rounded-lg flex items-center justify-center flex-shrink-0">
											<Server className="h-5 w-5 text-primary-600 dark:text-primary-400" />
										</div>
										<div className="min-w-0">
											<h3 className="text-base md:text-lg font-semibold text-secondary-900 dark:text-white">
												GetHomepage Widget Integration
											</h3>
											<p className="text-xs md:text-sm text-secondary-600 dark:text-secondary-400">
												Create API keys to display PatchMon statistics in your
												GetHomepage dashboard
											</p>
										</div>
									</div>
									<button
										type="button"
										onClick={() => setShowCreateModal(true)}
										className="btn-primary flex items-center gap-2 w-full sm:w-auto justify-center sm:justify-start"
									>
										<Plus className="h-4 w-4" />
										New API Key
									</button>
								</div>

								{/* API Keys List */}
								{loading ? (
									<div className="text-center py-8">
										<div className="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600" />
									</div>
								) : tokens.filter(
										(token) =>
											token.metadata?.integration_type === "gethomepage",
									).length === 0 ? (
									<div className="text-center py-8 text-secondary-600 dark:text-secondary-400">
										<p>No GetHomepage API keys created yet.</p>
										<p className="text-sm mt-2">
											Create an API key to enable GetHomepage widget
											integration.
										</p>
									</div>
								) : (
									<div className="space-y-3">
										{tokens
											.filter(
												(token) =>
													token.metadata?.integration_type === "gethomepage",
											)
											.map((token) => (
												<div
													key={token.id}
													className="border border-secondary-200 dark:border-secondary-600 rounded-lg p-3 md:p-4 hover:border-primary-300 dark:hover:border-primary-700 transition-colors"
												>
													<div className="flex flex-col sm:flex-row justify-between items-start gap-3">
														<div className="flex-1 min-w-0">
															<div className="flex items-center gap-2 flex-wrap">
																<h4 className="text-sm md:text-base font-medium text-secondary-900 dark:text-white truncate">
																	{token.token_name}
																</h4>
																<span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200">
																	GetHomepage
																</span>
																{token.is_active ? (
																	<span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">
																		Active
																	</span>
																) : (
																	<span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-secondary-100 text-secondary-800 dark:bg-secondary-700 dark:text-secondary-200">
																		Inactive
																	</span>
																)}
															</div>
															<div className="mt-2 space-y-1 text-xs md:text-sm text-secondary-600 dark:text-secondary-400">
																<div className="flex items-center gap-2">
																	<span className="font-mono text-xs bg-secondary-100 dark:bg-secondary-700 px-2 py-1 rounded break-all flex-1 min-w-0">
																		{token.token_key}
																	</span>
																	<button
																		type="button"
																		onClick={() =>
																			copy_to_clipboard(
																				token.token_key,
																				`key-${token.id}`,
																			)
																		}
																		className="text-primary-600 hover:text-primary-700 dark:text-primary-400 flex-shrink-0"
																	>
																		{copy_success[`key-${token.id}`] ? (
																			<CheckCircle className="h-4 w-4" />
																		) : (
																			<Copy className="h-4 w-4" />
																		)}
																	</button>
																</div>
																<p>Created: {format_date(token.created_at)}</p>
																{token.last_used_at && (
																	<p>
																		Last Used: {format_date(token.last_used_at)}
																	</p>
																)}
																{token.expires_at && (
																	<p>
																		Expires: {format_date(token.expires_at)}
																		{new Date(token.expires_at) <
																			new Date() && (
																			<span className="ml-2 text-red-600 dark:text-red-400">
																				(Expired)
																			</span>
																		)}
																	</p>
																)}
															</div>
														</div>
														<div className="flex items-center gap-2 flex-wrap w-full sm:w-auto">
															<button
																type="button"
																onClick={() =>
																	toggle_token_active(token.id, token.is_active)
																}
																className={`px-3 py-1 text-xs md:text-sm rounded ${
																	token.is_active
																		? "bg-secondary-100 text-secondary-700 hover:bg-secondary-200 dark:bg-secondary-700 dark:text-secondary-300"
																		: "bg-green-100 text-green-700 hover:bg-green-200 dark:bg-green-900 dark:text-green-300"
																}`}
															>
																{token.is_active ? "Disable" : "Enable"}
															</button>
															<button
																type="button"
																onClick={() =>
																	delete_token(token.id, token.token_name)
																}
																className="text-red-600 hover:text-red-800 dark:text-red-400 p-2"
															>
																<Trash2 className="h-4 w-4" />
															</button>
														</div>
													</div>
												</div>
											))}
									</div>
								)}

								{/* Documentation Section */}
								<div className="bg-primary-50 dark:bg-primary-900/20 border border-primary-200 dark:border-primary-800 rounded-lg p-4 md:p-6">
									<div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4 mb-4">
										<h3 className="text-base md:text-lg font-semibold text-primary-900 dark:text-primary-200">
											How to Use GetHomepage Integration
										</h3>
										<a
											href="https://docs.patchmon.net/books/patchmon-application-documentation/page/gethomepagedev-dashboard-card"
											target="_blank"
											rel="noopener noreferrer"
											className="px-4 py-2 bg-primary-600 hover:bg-primary-700 dark:bg-primary-500 dark:hover:bg-primary-600 text-white rounded-lg flex items-center gap-2 transition-colors w-full sm:w-auto justify-center sm:justify-start"
										>
											<BookOpen className="h-4 w-4" />
											Documentation
										</a>
									</div>
									<ol className="list-decimal list-inside space-y-2 text-sm text-primary-800 dark:text-primary-300">
										<li>Create a new API key using the button above</li>
										<li>Copy the API key and secret from the success dialog</li>
										<li>
											Add the following widget configuration to your GetHomepage{" "}
											<code className="bg-primary-100 dark:bg-primary-900/40 px-1 py-0.5 rounded text-xs">
												services.yml
											</code>{" "}
											file:
										</li>
									</ol>

									<div className="mt-4 p-3 bg-primary-100 dark:bg-primary-900/40 rounded border border-primary-200 dark:border-primary-700">
										<pre className="text-xs text-primary-800 dark:text-primary-300 whitespace-pre-wrap overflow-x-auto font-mono">
											{`- PatchMon:
    href: ${server_url}
    description: PatchMon Statistics
    icon: ${server_url}/assets/favicon.svg
    widget:
      type: customapi
      url: ${server_url}/api/v1/gethomepage/stats
      headers:
        Authorization: Basic BASE64_ENCODED_CREDENTIALS
      mappings:
        - field: total_hosts
          label: Total Hosts
        - field: hosts_needing_updates
          label: Needs Updates
        - field: security_updates
          label: Security Updates`}
										</pre>
									</div>

									<div className="mt-3 p-3 bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded">
										<p className="text-xs text-blue-800 dark:text-blue-300 mb-2">
											<strong>
												How to generate BASE64_ENCODED_CREDENTIALS:
											</strong>
										</p>
										<pre className="text-xs text-blue-800 dark:text-blue-300 font-mono bg-blue-100 dark:bg-blue-900/40 p-2 rounded overflow-x-auto">
											{`echo -n "YOUR_API_KEY:YOUR_API_SECRET" | base64`}
										</pre>
										<p className="text-xs text-blue-800 dark:text-blue-300 mt-2">
											Replace YOUR_API_KEY and YOUR_API_SECRET with your actual
											credentials, then run this command to get the base64
											string.
										</p>
									</div>

									<div className="mt-4 p-3 bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded">
										<h4 className="text-sm font-semibold text-blue-900 dark:text-blue-200 mb-2">
											Additional Widget Examples
										</h4>
										<p className="text-xs text-blue-800 dark:text-blue-300 mb-2">
											You can create multiple widgets to display different
											statistics:
										</p>
										<div className="space-y-2 text-xs text-blue-800 dark:text-blue-300 font-mono">
											<div className="bg-blue-100 dark:bg-blue-900/40 p-2 rounded">
												<strong>Security Updates Widget:</strong>
												<br />
												type: customapi
												<br />
												key: security_updates
												<br />
												value: hosts_with_security_updates
												<br />
												label: Security Updates
											</div>
											<div className="bg-blue-100 dark:bg-blue-900/40 p-2 rounded">
												<strong>Up-to-Date Hosts Widget:</strong>
												<br />
												type: customapi
												<br />
												key: up_to_date_hosts
												<br />
												value: total_hosts
												<br />
												label: Up-to-Date Hosts
											</div>
											<div className="bg-blue-100 dark:bg-blue-900/40 p-2 rounded">
												<strong>Recent Activity Widget:</strong>
												<br />
												type: customapi
												<br />
												key: recent_updates_24h
												<br />
												value: total_hosts
												<br />
												label: Updates (24h)
											</div>
										</div>
									</div>
								</div>
							</div>
						)}

						{/* Docker Tab */}
						{activeTab === "docker" && (
							<div className="space-y-6">
								{/* Header */}
								<div className="flex items-center gap-3">
									<div className="w-10 h-10 bg-primary-100 dark:bg-primary-900 rounded-lg flex items-center justify-center flex-shrink-0">
										<Container className="h-5 w-5 text-primary-600 dark:text-primary-400" />
									</div>
									<div className="min-w-0">
										<h3 className="text-base md:text-lg font-semibold text-secondary-900 dark:text-white">
											Docker Inventory Collection
										</h3>
										<p className="text-xs md:text-sm text-secondary-600 dark:text-secondary-400">
											Docker monitoring is now built into the PatchMon Go agent
										</p>
									</div>
								</div>

								{/* Info Message */}
								<div className="bg-primary-50 dark:bg-primary-900/20 border border-primary-200 dark:border-primary-800 rounded-lg p-4 md:p-6">
									<div className="flex items-start gap-3">
										<CheckCircle className="h-5 w-5 text-primary-600 dark:text-primary-400 flex-shrink-0 mt-0.5" />
										<div className="min-w-0">
											<h4 className="text-sm md:text-base font-semibold text-primary-900 dark:text-primary-200 mb-2">
												Automatic Docker Discovery
											</h4>
											<p className="text-xs md:text-sm text-primary-800 dark:text-primary-300 mb-3">
												The PatchMon Go agent automatically discovers Docker
												when it's available on your host and collects
												comprehensive inventory information:
											</p>
											<ul className="list-disc list-inside space-y-2 text-xs md:text-sm text-primary-800 dark:text-primary-300 ml-2">
												<li>
													<strong>Containers</strong> - Running and stopped
													containers with status, images, ports, and labels
												</li>
												<li>
													<strong>Images</strong> - All Docker images with
													repository, tags, sizes, and sources
												</li>
												<li>
													<strong>Volumes</strong> - Named and anonymous volumes
													with drivers, mountpoints, and usage
												</li>
												<li>
													<strong>Networks</strong> - Docker networks with
													drivers, IPAM configuration, and connected containers
												</li>
												<li>
													<strong>Real-time Updates</strong> - Container status
													changes are pushed instantly via WebSocket
												</li>
											</ul>
										</div>
									</div>
								</div>

								{/* How It Works */}
								<div className="bg-white dark:bg-secondary-900 border border-secondary-200 dark:border-secondary-600 rounded-lg p-4 md:p-6">
									<h4 className="text-sm md:text-base font-semibold text-secondary-900 dark:text-white mb-4">
										How It Works
									</h4>
									<ol className="list-decimal list-inside space-y-3 text-xs md:text-sm text-secondary-700 dark:text-secondary-300">
										<li>
											Install the PatchMon Go agent on your host (see the Hosts
											page for installation instructions)
										</li>
										<li>
											The agent automatically detects if Docker is installed and
											running on the host
										</li>
										<li>
											During each collection cycle, the agent gathers Docker
											inventory data and sends it to the PatchMon server
										</li>
										<li>
											View your complete Docker inventory (containers, images,
											volumes, networks) in the{" "}
											<a
												href="/docker"
												className="text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300 underline"
											>
												Docker page
											</a>
										</li>
										<li>
											Container status changes are pushed to the server in
											real-time via WebSocket connection
										</li>
									</ol>
								</div>

								{/* No Configuration Required */}
								<div className="bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800 rounded-lg p-3 md:p-4">
									<div className="flex items-start gap-2">
										<CheckCircle className="h-5 w-5 text-green-600 dark:text-green-400 flex-shrink-0 mt-0.5" />
										<div className="text-xs md:text-sm text-green-800 dark:text-green-200">
											<p className="font-semibold mb-1">
												No Additional Configuration Required
											</p>
											<p>
												Once the Go agent is installed and Docker is running on
												your host, Docker inventory collection happens
												automatically. No separate Docker agent or cron jobs
												needed.
											</p>
										</div>
									</div>
								</div>

								{/* Requirements */}
								<div className="bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-lg p-3 md:p-4">
									<div className="flex items-start gap-2">
										<AlertCircle className="h-5 w-5 text-blue-600 dark:text-blue-400 flex-shrink-0 mt-0.5" />
										<div className="text-xs md:text-sm text-blue-800 dark:text-blue-200">
											<p className="font-semibold mb-2">Requirements:</p>
											<ul className="list-disc list-inside space-y-1 ml-2">
												<li>PatchMon Go agent must be installed and running</li>
												<li>Docker daemon must be installed and running</li>
												<li>
													Agent must have access to the Docker socket (
													<code className="bg-blue-100 dark:bg-blue-900/40 px-1 py-0.5 rounded text-xs">
														/var/run/docker.sock
													</code>
													)
												</li>
												<li>
													Typically requires running the agent as root or with
													Docker group permissions
												</li>
											</ul>
										</div>
									</div>
								</div>
							</div>
						)}
					</div>
				</div>
			</div>

			{/* Create Token Modal */}
			{show_create_modal && (
				<div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4">
					<div className="bg-white dark:bg-secondary-800 rounded-lg max-w-2xl w-full max-h-[90vh] overflow-y-auto">
						<div className="p-4 md:p-6">
							<div className="flex items-center justify-between mb-4">
								<h2 className="text-lg md:text-xl font-bold text-secondary-900 dark:text-white">
									{activeTab === "gethomepage"
										? "Create GetHomepage API Key"
										: "Create Token"}
								</h2>
								<button
									type="button"
									onClick={() => {
										setShowCreateModal(false);
										setUsageType("proxmox-lxc");
										setSelectedScriptType("proxmox-lxc");
									}}
									className="text-secondary-400 hover:text-secondary-600 dark:hover:text-secondary-200"
								>
									<X className="h-5 w-5 md:h-6 md:w-6" />
								</button>
							</div>

							{/* Tabs for Auto-enrollment modal */}
							{activeTab === "auto-enrollment" && (
								<div className="flex border-b border-secondary-200 dark:border-secondary-700 mb-4 md:mb-6 overflow-x-auto">
									<button
										type="button"
										onClick={() => setUsageType("proxmox-lxc")}
										className={`px-3 md:px-4 py-2 text-xs md:text-sm font-medium border-b-2 transition-colors whitespace-nowrap ${
											usage_type === "proxmox-lxc"
												? "text-primary-600 dark:text-primary-400 border-primary-500"
												: "text-secondary-500 dark:text-secondary-400 border-transparent hover:text-secondary-700 dark:hover:text-secondary-300"
										}`}
									>
										Auto-enrollment
									</button>
									<button
										type="button"
										onClick={() => setUsageType("api")}
										className={`px-3 md:px-4 py-2 text-xs md:text-sm font-medium border-b-2 transition-colors whitespace-nowrap ${
											usage_type === "api"
												? "text-primary-600 dark:text-primary-400 border-primary-500"
												: "text-secondary-500 dark:text-secondary-400 border-transparent hover:text-secondary-700 dark:hover:text-secondary-300"
										}`}
									>
										Scoped credentials
									</button>
								</div>
							)}

							<form onSubmit={create_token} className="space-y-4">
								<label className="block">
									<span className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-1">
										Token Name *
									</span>
									<input
										type="text"
										required
										value={form_data.token_name}
										onChange={(e) =>
											setFormData({ ...form_data, token_name: e.target.value })
										}
										placeholder={
											activeTab === "gethomepage"
												? "e.g., GetHomepage Widget"
												: usage_type === "api"
													? "e.g., Ansible Inventory"
													: "e.g., Proxmox Production"
										}
										className="w-full px-3 py-2 border border-secondary-300 dark:border-secondary-600 rounded-md bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
									/>
								</label>

								{usage_type === "proxmox-lxc" &&
									activeTab === "auto-enrollment" && (
										<>
											<label className="block">
												<span className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-1">
													Max Hosts Per Day
												</span>
												<input
													type="number"
													min="1"
													max="1000"
													value={form_data.max_hosts_per_day}
													onChange={(e) =>
														setFormData({
															...form_data,
															max_hosts_per_day: e.target.value,
														})
													}
													className="w-full px-3 py-2 border border-secondary-300 dark:border-secondary-600 rounded-md bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
												/>
												<p className="mt-1 text-xs text-secondary-500 dark:text-secondary-400">
													Maximum number of hosts that can be enrolled per day
													using this token
												</p>
											</label>

											<label className="block">
												<span className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-1">
													Default Host Group (Optional)
												</span>
												<select
													value={form_data.default_host_group_id}
													onChange={(e) =>
														setFormData({
															...form_data,
															default_host_group_id: e.target.value,
														})
													}
													className="w-full px-3 py-2 border border-secondary-300 dark:border-secondary-600 rounded-md bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
												>
													<option value="">No default group</option>
													{host_groups.map((group) => (
														<option key={group.id} value={group.id}>
															{group.name}
														</option>
													))}
												</select>
												<p className="mt-1 text-xs text-secondary-500 dark:text-secondary-400">
													Auto-enrolled hosts will be assigned to this group
												</p>
											</label>
										</>
									)}

								{usage_type === "api" && activeTab === "auto-enrollment" && (
									<div className="block">
										<span className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-2">
											Scopes *
										</span>
										<div className="border border-secondary-300 dark:border-secondary-600 rounded-md p-4 bg-secondary-50 dark:bg-secondary-900">
											<div className="mb-3">
												<p className="text-xs font-semibold text-secondary-700 dark:text-secondary-300 mb-2">
													Host Permissions
												</p>
												<div className="space-y-2">
													{["get", "put", "patch", "update", "delete"].map(
														(action) => (
															<label
																key={action}
																className="flex items-center gap-2"
															>
																<input
																	type="checkbox"
																	checked={
																		form_data.scopes?.host?.includes(action) ||
																		false
																	}
																	onChange={() =>
																		toggle_scope_action("host", action)
																	}
																	className="rounded border-secondary-300 dark:border-secondary-600 text-primary-600 focus:ring-primary-500 dark:focus:ring-primary-400"
																/>
																<span className="text-sm text-secondary-700 dark:text-secondary-300 uppercase">
																	{action}
																</span>
																<span className="text-xs text-secondary-500 dark:text-secondary-400">
																	{action === "get" && "- Read host data"}
																	{action === "put" && "- Replace host data"}
																	{action === "patch" && "- Update host data"}
																	{action === "update" && "- Modify host data"}
																	{action === "delete" && "- Delete hosts"}
																</span>
															</label>
														),
													)}
												</div>
											</div>
										</div>
										<p className="mt-1 text-xs text-secondary-500 dark:text-secondary-400">
											Select the permissions this API credential should have
										</p>
									</div>
								)}

								<label className="block">
									<span className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-1">
										Allowed IP Addresses (Optional)
									</span>
									<input
										type="text"
										value={form_data.allowed_ip_ranges}
										onChange={(e) =>
											setFormData({
												...form_data,
												allowed_ip_ranges: e.target.value,
											})
										}
										placeholder="e.g., 192.168.1.100, 10.0.0.50"
										className="w-full px-3 py-2 border border-secondary-300 dark:border-secondary-600 rounded-md bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
									/>
									<p className="mt-1 text-xs text-secondary-500 dark:text-secondary-400">
										Comma-separated list of IP addresses allowed to use this
										token
									</p>
								</label>

								<label className="block">
									<span className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-1">
										Expiration Date (Optional)
									</span>
									<input
										type="datetime-local"
										value={form_data.expires_at}
										onChange={(e) =>
											setFormData({ ...form_data, expires_at: e.target.value })
										}
										className="w-full px-3 py-2 border border-secondary-300 dark:border-secondary-600 rounded-md bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
									/>
								</label>

								<div className="flex flex-col sm:flex-row gap-3 pt-4">
									<button
										type="submit"
										className="flex-1 btn-primary py-2 px-4 rounded-md w-full sm:w-auto"
									>
										Create Token
									</button>
									<button
										type="button"
										onClick={() => {
											setShowCreateModal(false);
											setUsageType("proxmox-lxc");
										}}
										className="flex-1 bg-secondary-100 dark:bg-secondary-700 text-secondary-700 dark:text-secondary-300 py-2 px-4 rounded-md hover:bg-secondary-200 dark:hover:bg-secondary-600 w-full sm:w-auto"
									>
										Cancel
									</button>
								</div>
							</form>
						</div>
					</div>
				</div>
			)}

			{/* New Token Display Modal */}
			{new_token && (
				<div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4">
					<div className="bg-white dark:bg-secondary-800 rounded-lg max-w-4xl w-full max-h-[90vh] overflow-y-auto">
						<div className="p-4 md:p-6">
							<div className="flex items-center justify-between mb-4 gap-3">
								<div className="flex items-center gap-2 min-w-0">
									<CheckCircle className="h-5 w-5 md:h-6 md:w-6 text-green-600 dark:text-green-400 flex-shrink-0" />
									<h2 className="text-base md:text-lg font-bold text-secondary-900 dark:text-white truncate">
										{new_token.metadata?.integration_type === "gethomepage" ||
										activeTab === "gethomepage"
											? "API Key Created Successfully"
											: new_token.metadata?.integration_type === "api" ||
													usage_type === "api"
												? "API Credential Created Successfully"
												: "Token Created Successfully"}
									</h2>
								</div>
								<button
									type="button"
									onClick={() => {
										setNewToken(null);
										setShowSecret(false);
										setUsageType("proxmox-lxc");
										setSelectedScriptType("proxmox-lxc");
									}}
									className="text-secondary-400 hover:text-secondary-600 dark:hover:text-secondary-200 flex-shrink-0"
								>
									<X className="h-5 w-5" />
								</button>
							</div>

							<div className="bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800 rounded-lg p-3 mb-4">
								<div className="flex items-center gap-2">
									<AlertCircle className="h-4 w-4 text-yellow-600 dark:text-yellow-400 flex-shrink-0" />
									<p className="text-xs text-yellow-800 dark:text-yellow-200">
										<strong>Important:</strong> Save these credentials - the
										secret won't be shown again.
									</p>
								</div>
							</div>

							<div className="space-y-3">
								<div>
									<label
										htmlFor={token_name_id}
										className="block text-xs font-medium text-secondary-700 dark:text-secondary-300 mb-1"
									>
										Token Name
									</label>
									<input
										id={token_name_id}
										type="text"
										value={new_token.token_name}
										readOnly
										className="w-full px-3 py-2 text-sm border border-secondary-300 dark:border-secondary-600 rounded-md bg-secondary-50 dark:bg-secondary-900 text-secondary-900 dark:text-white font-mono"
									/>
								</div>

								<div className="grid grid-cols-1 md:grid-cols-2 gap-3">
									<div>
										<label
											htmlFor={token_key_id}
											className="block text-xs font-medium text-secondary-700 dark:text-secondary-300 mb-1"
										>
											Token Key
										</label>
										<div className="flex items-center gap-2">
											<input
												id={token_key_id}
												type="text"
												value={new_token.token_key}
												readOnly
												className="flex-1 px-3 py-2 text-xs md:text-sm border border-secondary-300 dark:border-secondary-600 rounded-md bg-secondary-50 dark:bg-secondary-900 text-secondary-900 dark:text-white font-mono break-all"
											/>
											<button
												type="button"
												onClick={() =>
													copy_to_clipboard(new_token.token_key, "new-key")
												}
												className="btn-primary p-2 flex-shrink-0"
												title="Copy Key"
											>
												{copy_success["new-key"] ? (
													<CheckCircle className="h-4 w-4" />
												) : (
													<Copy className="h-4 w-4" />
												)}
											</button>
										</div>
									</div>

									<div>
										<label
											htmlFor={token_secret_id}
											className="block text-xs font-medium text-secondary-700 dark:text-secondary-300 mb-1"
										>
											Token Secret
										</label>
										<div className="flex items-center gap-2">
											<input
												id={token_secret_id}
												type={show_secret ? "text" : "password"}
												value={new_token.token_secret}
												readOnly
												className="flex-1 px-3 py-2 text-xs md:text-sm border border-secondary-300 dark:border-secondary-600 rounded-md bg-secondary-50 dark:bg-secondary-900 text-secondary-900 dark:text-white font-mono break-all"
											/>
											<button
												type="button"
												onClick={() => setShowSecret(!show_secret)}
												className="p-2 text-secondary-600 hover:text-secondary-800 dark:text-secondary-400 flex-shrink-0"
												title="Toggle visibility"
											>
												{show_secret ? (
													<EyeOff className="h-4 w-4" />
												) : (
													<Eye className="h-4 w-4" />
												)}
											</button>
											<button
												type="button"
												onClick={() =>
													copy_to_clipboard(
														new_token.token_secret,
														"new-secret",
													)
												}
												className="btn-primary p-2 flex-shrink-0"
												title="Copy Secret"
											>
												{copy_success["new-secret"] ? (
													<CheckCircle className="h-4 w-4" />
												) : (
													<Copy className="h-4 w-4" />
												)}
											</button>
										</div>
									</div>
								</div>

								{(new_token.metadata?.integration_type === "api" ||
									usage_type === "api") &&
									new_token.scopes && (
										<div className="mt-4">
											<div className="block text-xs font-medium text-secondary-700 dark:text-secondary-300 mb-2">
												Granted Scopes
											</div>
											<div className="bg-secondary-50 dark:bg-secondary-900 border border-secondary-300 dark:border-secondary-600 rounded-md p-3">
												{Object.entries(new_token.scopes).map(
													([resource, actions]) => (
														<div key={resource} className="text-sm">
															<span className="font-semibold text-secondary-800 dark:text-secondary-200 capitalize">
																{resource}:
															</span>{" "}
															<span className="text-secondary-600 dark:text-secondary-400">
																{Array.isArray(actions)
																	? actions.join(", ").toUpperCase()
																	: actions}
															</span>
														</div>
													),
												)}
											</div>
										</div>
									)}

								{(new_token.metadata?.integration_type === "api" ||
									usage_type === "api") && (
									<div className="mt-6">
										<div className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-2">
											Usage Examples
										</div>
										<div className="space-y-3">
											<div>
												<p className="text-xs text-secondary-600 dark:text-secondary-400 mb-2">
													Basic cURL request:
												</p>
												<div className="flex flex-col sm:flex-row items-stretch sm:items-center gap-2">
													<input
														type="text"
														value={`curl -u "${new_token.token_key}:${new_token.token_secret}" ${server_url}/api/v1/api/hosts`}
														readOnly
														className="flex-1 px-3 py-2 text-xs border border-secondary-300 dark:border-secondary-600 rounded-md bg-secondary-50 dark:bg-secondary-900 text-secondary-900 dark:text-white font-mono break-all"
													/>
													<button
														type="button"
														onClick={() =>
															copy_to_clipboard(
																`curl -u "${new_token.token_key}:${new_token.token_secret}" ${server_url}/api/v1/api/hosts`,
																"api-curl-basic",
															)
														}
														className="btn-primary p-2 flex-shrink-0"
														title="Copy cURL command"
													>
														{copy_success["api-curl-basic"] ? (
															<CheckCircle className="h-4 w-4" />
														) : (
															<Copy className="h-4 w-4" />
														)}
													</button>
												</div>
											</div>
											<div>
												<p className="text-xs text-secondary-600 dark:text-secondary-400 mb-2">
													Filter by host group:
												</p>
												<div className="flex flex-col sm:flex-row items-stretch sm:items-center gap-2">
													<input
														type="text"
														value={`curl -u "${new_token.token_key}:${new_token.token_secret}" "${server_url}/api/v1/api/hosts?hostgroup=Production"`}
														readOnly
														className="flex-1 px-3 py-2 text-xs border border-secondary-300 dark:border-secondary-600 rounded-md bg-secondary-50 dark:bg-secondary-900 text-secondary-900 dark:text-white font-mono break-all"
													/>
													<button
														type="button"
														onClick={() =>
															copy_to_clipboard(
																`curl -u "${new_token.token_key}:${new_token.token_secret}" "${server_url}/api/v1/api/hosts?hostgroup=Production"`,
																"api-curl-filter",
															)
														}
														className="btn-primary p-2 flex-shrink-0"
														title="Copy cURL command"
													>
														{copy_success["api-curl-filter"] ? (
															<CheckCircle className="h-4 w-4" />
														) : (
															<Copy className="h-4 w-4" />
														)}
													</button>
												</div>
											</div>
										</div>
										<p className="text-xs text-secondary-500 dark:text-secondary-400 mt-3">
											💡 Replace "Production" with your host group name or UUID
										</p>
									</div>
								)}

								{(new_token.metadata?.integration_type === "proxmox-lxc" ||
									usage_type === "proxmox-lxc") && (
									<div className="mt-6">
										<div className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-2">
											Auto-Enrollment Command
										</div>

										{/* Script Type Toggle Buttons */}
										<div className="flex gap-2 mb-3">
											<button
												type="button"
												onClick={() => setSelectedScriptType("proxmox-lxc")}
												className={`px-3 py-1.5 text-sm font-medium rounded-md transition-colors ${
													selected_script_type === "proxmox-lxc"
														? "bg-primary-600 text-white dark:bg-primary-500"
														: "bg-secondary-200 text-secondary-700 dark:bg-secondary-700 dark:text-secondary-300 hover:bg-secondary-300 dark:hover:bg-secondary-600"
												}`}
											>
												Proxmox
											</button>
											<button
												type="button"
												onClick={() => setSelectedScriptType("direct-host")}
												className={`px-3 py-1.5 text-sm font-medium rounded-md transition-colors ${
													selected_script_type === "direct-host"
														? "bg-primary-600 text-white dark:bg-primary-500"
														: "bg-secondary-200 text-secondary-700 dark:bg-secondary-700 dark:text-secondary-300 hover:bg-secondary-300 dark:hover:bg-secondary-600"
												}`}
											>
												Direct Host
											</button>
										</div>

										<p className="text-xs text-secondary-600 dark:text-secondary-400 mb-2">
											{selected_script_type === "proxmox-lxc"
												? "Run this command on your Proxmox host to automatically discover and enroll all running LXC containers:"
												: "Run this command on individual hosts to enroll them directly:"}
										</p>

										{/* Force Install Toggle */}
										<div className="mb-3">
											<label className="flex items-center gap-2 text-sm">
												<input
													type="checkbox"
													checked={force_proxmox_install}
													onChange={(e) =>
														setForceProxmoxInstall(e.target.checked)
													}
													className="rounded border-secondary-300 dark:border-secondary-600 text-primary-600 focus:ring-primary-500 dark:focus:ring-primary-400 dark:bg-secondary-700"
												/>
												<span className="text-secondary-800 dark:text-secondary-200">
													Force install (bypass broken packages)
												</span>
											</label>
											<p className="text-xs text-secondary-600 dark:text-secondary-400 mt-1">
												Enable this if hosts have broken packages (CloudPanel,
												WHM, etc.) that block apt-get operations
											</p>
										</div>

										<div className="flex flex-col sm:flex-row items-stretch sm:items-center gap-2">
											<input
												type="text"
												value={`curl ${curl_flags} "${getEnrollmentUrl()}" | ${selected_script_type === "proxmox-lxc" ? "bash" : "sh"}`}
												readOnly
												className="flex-1 px-3 py-2 border border-secondary-300 dark:border-secondary-600 rounded-md bg-secondary-50 dark:bg-secondary-900 text-secondary-900 dark:text-white font-mono text-xs break-all"
											/>
											<button
												type="button"
												onClick={() =>
													copy_to_clipboard(
														`curl ${curl_flags} "${getEnrollmentUrl()}" | ${selected_script_type === "proxmox-lxc" ? "bash" : "sh"}`,
														"enrollment-command",
													)
												}
												className="btn-primary flex items-center justify-center gap-1 px-3 py-2 whitespace-nowrap"
											>
												{copy_success["enrollment-command"] ? (
													<>
														<CheckCircle className="h-4 w-4" />
														Copied
													</>
												) : (
													<>
														<Copy className="h-4 w-4" />
														Copy
													</>
												)}
											</button>
										</div>

										{/* Usage hint for direct-host */}
										{selected_script_type === "direct-host" && (
											<p className="text-xs text-secondary-500 dark:text-secondary-400 mt-3 p-2 bg-blue-50 dark:bg-blue-900/20 rounded border border-blue-200 dark:border-blue-800">
												💡 <strong>Tip:</strong> Specify a custom name:{" "}
												<code className="text-xs bg-secondary-200 dark:bg-secondary-700 px-1 py-0.5 rounded">
													FRIENDLY_NAME="My Server" sh
												</code>
											</p>
										)}
									</div>
								)}

								{(new_token.metadata?.integration_type === "gethomepage" ||
									activeTab === "gethomepage") && (
									<div className="mt-3 space-y-3">
										<div>
											<label
												htmlFor={token_base64_id}
												className="block text-xs font-medium text-secondary-700 dark:text-secondary-300 mb-1"
											>
												Base64 Encoded Credentials
											</label>
											<div className="flex flex-col sm:flex-row items-stretch sm:items-center gap-2">
												<input
													id={token_base64_id}
													type="text"
													value={btoa(
														`${new_token.token_key}:${new_token.token_secret}`,
													)}
													readOnly
													className="flex-1 px-3 py-2 text-xs md:text-sm border border-secondary-300 dark:border-secondary-600 rounded-md bg-secondary-50 dark:bg-secondary-900 text-secondary-900 dark:text-white font-mono break-all"
												/>
												<button
													type="button"
													onClick={() =>
														copy_to_clipboard(
															btoa(
																`${new_token.token_key}:${new_token.token_secret}`,
															),
															"base64-creds",
														)
													}
													className="btn-primary p-2 flex-shrink-0"
													title="Copy Base64"
												>
													{copy_success["base64-creds"] ? (
														<CheckCircle className="h-4 w-4" />
													) : (
														<Copy className="h-4 w-4" />
													)}
												</button>
											</div>
										</div>

										<div>
											<div className="flex items-center justify-between mb-1">
												<label
													htmlFor={gethomepage_config_id}
													className="text-xs font-medium text-secondary-700 dark:text-secondary-300"
												>
													GetHomepage Configuration
												</label>
												<button
													type="button"
													onClick={() => {
														const base64Creds = btoa(
															`${new_token.token_key}:${new_token.token_secret}`,
														);
														const config = `- PatchMon:
    href: ${server_url}
    description: PatchMon Statistics
    icon: ${server_url}/assets/favicon.svg
    widget:
      type: customapi
      url: ${server_url}/api/v1/gethomepage/stats
      headers:
        Authorization: Basic ${base64Creds}
      mappings:
        - field: total_hosts
          label: Total Hosts
        - field: hosts_needing_updates
          label: Needs Updates
        - field: security_updates
          label: Security Updates`;
														copy_to_clipboard(config, "gethomepage-config");
													}}
													className="btn-primary flex items-center gap-1 px-2 py-1 text-xs"
												>
													{copy_success["gethomepage-config"] ? (
														<>
															<CheckCircle className="h-3 w-3" />
															Copied
														</>
													) : (
														<>
															<Copy className="h-3 w-3" />
															Copy Config
														</>
													)}
												</button>
											</div>
											<textarea
												id={gethomepage_config_id}
												value={(() => {
													const base64Creds = btoa(
														`${new_token.token_key}:${new_token.token_secret}`,
													);
													return `- PatchMon:
    href: ${server_url}
    description: PatchMon Statistics
    icon: ${server_url}/assets/favicon.svg
    widget:
      type: customapi
      url: ${server_url}/api/v1/gethomepage/stats
      headers:
        Authorization: Basic ${base64Creds}
      mappings:
        - field: total_hosts
          label: Total Hosts
        - field: hosts_needing_updates
          label: Needs Updates
        - field: security_updates
          label: Security Updates`;
												})()}
												readOnly
												rows={12}
												className="w-full px-3 py-2 border border-secondary-300 dark:border-secondary-600 rounded-md bg-secondary-50 dark:bg-secondary-900 text-secondary-900 dark:text-white font-mono text-xs resize-none"
											/>
											<p className="text-xs text-secondary-500 dark:text-secondary-400 mt-1">
												💡 Paste into your GetHomepage{" "}
												<code className="bg-secondary-100 dark:bg-secondary-700 px-1 rounded">
													services.yml
												</code>
											</p>
										</div>
									</div>
								)}
							</div>

							<div className="mt-4 pt-4 border-t border-secondary-200 dark:border-secondary-600">
								<button
									type="button"
									onClick={() => {
										setNewToken(null);
										setShowSecret(false);
										setUsageType("proxmox-lxc");
										setSelectedScriptType("proxmox-lxc");
									}}
									className="w-full btn-primary py-2 px-4 rounded-md text-sm md:text-base"
								>
									I've Saved the Credentials
								</button>
							</div>
						</div>
					</div>
				</div>
			)}

			{/* Edit API Credential Modal */}
			{show_edit_modal && edit_token && (
				<div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4">
					<div className="bg-white dark:bg-secondary-800 rounded-lg max-w-2xl w-full max-h-[90vh] overflow-y-auto">
						<div className="p-4 md:p-6">
							<div className="flex items-center justify-between mb-4 md:mb-6 gap-3">
								<h2 className="text-lg md:text-xl font-bold text-secondary-900 dark:text-white">
									Edit API Credential
								</h2>
								<button
									type="button"
									onClick={() => {
										setShowEditModal(false);
										setEditToken(null);
									}}
									className="text-secondary-400 hover:text-secondary-600 dark:hover:text-secondary-200 flex-shrink-0"
								>
									<X className="h-5 w-5 md:h-6 md:w-6" />
								</button>
							</div>

							<form onSubmit={update_token} className="space-y-4">
								<div className="block">
									<span className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-1">
										Token Name
									</span>
									<input
										type="text"
										value={form_data.token_name}
										readOnly
										disabled
										className="w-full px-3 py-2 border border-secondary-300 dark:border-secondary-600 rounded-md bg-secondary-100 dark:bg-secondary-900 text-secondary-500 dark:text-secondary-400"
									/>
									<p className="mt-1 text-xs text-secondary-500 dark:text-secondary-400">
										Token name cannot be changed
									</p>
								</div>

								{edit_token?.metadata?.integration_type === "api" && (
									<div className="block">
										<span className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-2">
											Scopes
										</span>
										<div className="border border-secondary-300 dark:border-secondary-600 rounded-md p-4 bg-secondary-50 dark:bg-secondary-900">
											<div className="mb-3">
												<p className="text-xs font-semibold text-secondary-700 dark:text-secondary-300 mb-2">
													Host Permissions
												</p>
												<div className="space-y-2">
													{["get", "put", "patch", "update", "delete"].map(
														(action) => (
															<label
																key={action}
																className="flex items-center gap-2"
															>
																<input
																	type="checkbox"
																	checked={
																		form_data.scopes?.host?.includes(action) ||
																		false
																	}
																	onChange={() =>
																		toggle_scope_action("host", action)
																	}
																	className="rounded border-secondary-300 dark:border-secondary-600 text-primary-600 focus:ring-primary-500 dark:focus:ring-primary-400"
																/>
																<span className="text-sm text-secondary-700 dark:text-secondary-300 uppercase">
																	{action}
																</span>
																<span className="text-xs text-secondary-500 dark:text-secondary-400">
																	{action === "get" && "- Read host data"}
																	{action === "put" && "- Replace host data"}
																	{action === "patch" && "- Update host data"}
																	{action === "update" && "- Modify host data"}
																	{action === "delete" && "- Delete hosts"}
																</span>
															</label>
														),
													)}
												</div>
											</div>
										</div>
										<p className="mt-1 text-xs text-secondary-500 dark:text-secondary-400">
											Update the permissions for this API credential
										</p>
									</div>
								)}

								<label className="block">
									<span className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-1">
										Allowed IP Addresses (Optional)
									</span>
									<input
										type="text"
										value={form_data.allowed_ip_ranges}
										onChange={(e) =>
											setFormData({
												...form_data,
												allowed_ip_ranges: e.target.value,
											})
										}
										placeholder="e.g., 192.168.1.100, 10.0.0.50"
										className="w-full px-3 py-2 border border-secondary-300 dark:border-secondary-600 rounded-md bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
									/>
									<p className="mt-1 text-xs text-secondary-500 dark:text-secondary-400">
										Comma-separated list of IP addresses allowed to use this
										token
									</p>
								</label>

								<label className="block">
									<span className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-1">
										Expiration Date (Optional)
									</span>
									<input
										type="datetime-local"
										value={form_data.expires_at}
										onChange={(e) =>
											setFormData({ ...form_data, expires_at: e.target.value })
										}
										className="w-full px-3 py-2 border border-secondary-300 dark:border-secondary-600 rounded-md bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
									/>
								</label>

								<div className="flex flex-col sm:flex-row gap-3 pt-4">
									<button
										type="submit"
										className="flex-1 btn-primary py-2 px-4 rounded-md w-full sm:w-auto"
									>
										Update Credential
									</button>
									<button
										type="button"
										onClick={() => {
											setShowEditModal(false);
											setEditToken(null);
										}}
										className="flex-1 bg-secondary-100 dark:bg-secondary-700 text-secondary-700 dark:text-secondary-300 py-2 px-4 rounded-md hover:bg-secondary-200 dark:hover:bg-secondary-600 w-full sm:w-auto"
									>
										Cancel
									</button>
								</div>
							</form>
						</div>
					</div>
				</div>
			)}
		</SettingsLayout>
	);
};

export default Integrations;
