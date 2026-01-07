import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
	AlertTriangle,
	CheckCircle,
	Copy,
	Eye,
	EyeOff,
	RotateCcw,
	X,
} from "lucide-react";
import { useId, useState } from "react";
import { adminHostsAPI, settingsAPI } from "../../utils/api";

const CredentialsModal = ({ host, isOpen, onClose, plaintextApiKey }) => {
	const [showApiKey, setShowApiKey] = useState(false);
	const [activeTab, setActiveTab] = useState("quick-install");
	const [forceInstall, setForceInstall] = useState(false);
	const [regeneratedCredentials, setRegeneratedCredentials] = useState(null);
	const [isRegenerating, setIsRegenerating] = useState(false);
	const apiIdInputId = useId();
	const apiKeyInputId = useId();
	const queryClient = useQueryClient();

	// Use plaintext API key if available (from host creation or regeneration), otherwise the stored key is a hash
	// Priority: regenerated > navigation state > stored (which is a hash)
	const effectiveApiKey =
		regeneratedCredentials?.apiKey || plaintextApiKey || host.api_key;
	const effectiveApiId = regeneratedCredentials?.apiId || host.api_id;
	const isApiKeyHash =
		!regeneratedCredentials &&
		!plaintextApiKey &&
		host.api_key?.startsWith("$2");

	const handleRegenerateCredentials = async () => {
		setIsRegenerating(true);
		try {
			const response = await adminHostsAPI.regenerateCredentials(host.id);
			setRegeneratedCredentials({
				apiId: response.data.apiId,
				apiKey: response.data.apiKey,
			});
			queryClient.invalidateQueries(["host", host.id]);
		} catch (error) {
			console.error("Failed to regenerate credentials:", error);
		} finally {
			setIsRegenerating(false);
		}
	};

	const { data: serverUrlData } = useQuery({
		queryKey: ["serverUrl"],
		queryFn: () => settingsAPI.getServerUrl().then((res) => res.data),
	});

	// Use configured server URL, or derive from current page URL in production
	const serverUrl = serverUrlData?.server_url ||
		(import.meta.env.PROD ? `${window.location.protocol}//${window.location.host}` : "http://localhost:3001");

	// Fetch settings for dynamic curl flags (local to modal)
	const { data: settings } = useQuery({
		queryKey: ["settings"],
		queryFn: () => settingsAPI.get().then((res) => res.data),
	});

	// Helper function to get curl flags based on settings
	const getCurlFlags = () => {
		return settings?.ignore_ssl_self_signed ? "-sk" : "-s";
	};

	// Helper function to get the install URL
	const getInstallUrl = () => `${serverUrl}/api/v1/hosts/install`;

	// Helper function to build the shell command suffix (sudo sh with optional --force)
	const getShellCommand = () =>
		forceInstall ? "sudo sh -s -- --force" : "sudo sh";

	const copyToClipboard = async (text) => {
		try {
			// Try modern clipboard API first
			if (navigator.clipboard && window.isSecureContext) {
				await navigator.clipboard.writeText(text);
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
				if (!successful) {
					throw new Error("Copy command failed");
				}
			} catch {
				// If all else fails, show the text in a prompt
				prompt("Copy this command:", text);
			} finally {
				document.body.removeChild(textArea);
			}
		} catch (err) {
			console.error("Failed to copy to clipboard:", err);
			// Show the text in a prompt as last resort
			prompt("Copy this command:", text);
		}
	};

	if (!isOpen || !host) return null;

	return (
		<div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4">
			<div className="bg-white dark:bg-secondary-800 rounded-lg p-4 md:p-6 w-full max-w-4xl max-h-[90vh] overflow-y-auto">
				<div className="flex justify-between items-center mb-4 gap-3">
					<h3 className="text-base md:text-lg font-medium text-secondary-900 dark:text-white truncate">
						Host Setup - {host.friendly_name}
					</h3>
					<button
						type="button"
						onClick={onClose}
						className="text-secondary-400 hover:text-secondary-600 dark:text-secondary-500 dark:hover:text-secondary-300 flex-shrink-0"
					>
						<X className="h-5 w-5" />
					</button>
				</div>

				{/* Mobile Button Navigation */}
				<div className="md:hidden space-y-2 mb-4">
					<button
						type="button"
						onClick={() => setActiveTab("quick-install")}
						className={`w-full flex items-center justify-between px-4 py-3 rounded-md font-medium text-sm transition-colors ${
							activeTab === "quick-install"
								? "bg-primary-50 dark:bg-primary-900/20 text-primary-600 dark:text-primary-400 border border-primary-200 dark:border-primary-800"
								: "bg-secondary-50 dark:bg-secondary-700 text-secondary-700 dark:text-secondary-300 border border-secondary-200 dark:border-secondary-600 hover:bg-secondary-100 dark:hover:bg-secondary-600"
						}`}
					>
						<span>Quick Install</span>
						{activeTab === "quick-install" && (
							<CheckCircle className="h-5 w-5 text-primary-600 dark:text-primary-400" />
						)}
					</button>
					<button
						type="button"
						onClick={() => setActiveTab("credentials")}
						className={`w-full flex items-center justify-between px-4 py-3 rounded-md font-medium text-sm transition-colors ${
							activeTab === "credentials"
								? "bg-primary-50 dark:bg-primary-900/20 text-primary-600 dark:text-primary-400 border border-primary-200 dark:border-primary-800"
								: "bg-secondary-50 dark:bg-secondary-700 text-secondary-700 dark:text-secondary-300 border border-secondary-200 dark:border-secondary-600 hover:bg-secondary-100 dark:hover:bg-secondary-600"
						}`}
					>
						<span>API Credentials</span>
						{activeTab === "credentials" && (
							<CheckCircle className="h-5 w-5 text-primary-600 dark:text-primary-400" />
						)}
					</button>
				</div>

				{/* Desktop Tab Navigation */}
				<div className="hidden md:block border-b border-secondary-200 dark:border-secondary-600 mb-4 md:mb-6">
					<nav className="-mb-px flex space-x-8">
						<button
							type="button"
							onClick={() => setActiveTab("quick-install")}
							className={`py-2 px-1 border-b-2 font-medium text-sm ${
								activeTab === "quick-install"
									? "border-primary-500 text-primary-600 dark:text-primary-400"
									: "border-transparent text-secondary-500 dark:text-secondary-400 hover:text-secondary-700 dark:hover:text-secondary-300 hover:border-secondary-300 dark:hover:border-secondary-500"
							}`}
						>
							Quick Install
						</button>
						<button
							type="button"
							onClick={() => setActiveTab("credentials")}
							className={`py-2 px-1 border-b-2 font-medium text-sm ${
								activeTab === "credentials"
									? "border-primary-500 text-primary-600 dark:text-primary-400"
									: "border-transparent text-secondary-500 dark:text-secondary-400 hover:text-secondary-700 dark:hover:text-secondary-300 hover:border-secondary-300 dark:hover:border-secondary-500"
							}`}
						>
							API Credentials
						</button>
					</nav>
				</div>

				{/* Tab Content */}
				{activeTab === "quick-install" && (
					<div className="space-y-4">
						<div className="bg-primary-50 dark:bg-primary-900 border border-primary-200 dark:border-primary-700 rounded-lg p-3 md:p-4">
							<h4 className="text-xs md:text-sm font-medium text-primary-900 dark:text-primary-200 mb-2">
								One-Line Installation
							</h4>
							<p className="text-xs md:text-sm text-primary-700 dark:text-primary-300 mb-3">
								Copy and run this command on the target host to securely install
								and configure the PatchMonEnhanced agent:
							</p>

							{/* Force Install Toggle */}
							<div className="mb-3">
								<label className="flex items-center gap-2 text-xs md:text-sm">
									<input
										type="checkbox"
										checked={forceInstall}
										onChange={(e) => setForceInstall(e.target.checked)}
										className="rounded border-secondary-300 dark:border-secondary-600 text-primary-600 focus:ring-primary-500 dark:focus:ring-primary-400 dark:bg-secondary-700"
									/>
									<span className="text-primary-800 dark:text-primary-200">
										Force install (bypass broken packages)
									</span>
								</label>
								<p className="text-xs text-primary-600 dark:text-primary-400 mt-1">
									Enable this if the target host has broken packages
									(CloudPanel, WHM, etc.) that block apt-get operations
								</p>
							</div>

							{isApiKeyHash && (
								<div className="mb-3 p-3 bg-warning-50 dark:bg-warning-900/20 border border-warning-200 dark:border-warning-700 rounded-lg">
									<div className="flex items-start justify-between gap-2">
										<div className="flex items-start gap-2">
											<AlertTriangle className="h-4 w-4 text-warning-600 dark:text-warning-400 flex-shrink-0 mt-0.5" />
											<div>
												<p className="text-xs md:text-sm font-medium text-warning-800 dark:text-warning-200">
													API Key Not Available
												</p>
												<p className="text-xs text-warning-700 dark:text-warning-300 mt-1">
													The plaintext API key is only shown once when the host
													is created.
												</p>
											</div>
										</div>
										<button
											type="button"
											onClick={handleRegenerateCredentials}
											disabled={isRegenerating}
											className="btn-outline flex items-center gap-1 text-xs whitespace-nowrap"
										>
											<RotateCcw
												className={`h-3 w-3 ${isRegenerating ? "animate-spin" : ""}`}
											/>
											{isRegenerating ? "Regenerating..." : "Regenerate"}
										</button>
									</div>
								</div>
							)}

							<div className="flex flex-col sm:flex-row items-stretch sm:items-center gap-2">
								<input
									type="text"
									value={
										isApiKeyHash
											? "API key not available - click Regenerate above"
											: `curl ${getCurlFlags()} ${getInstallUrl()} -H "X-API-ID: ${effectiveApiId}" -H "X-API-KEY: ${effectiveApiKey}" | ${getShellCommand()}`
									}
									readOnly
									disabled={isApiKeyHash}
									className={`flex-1 px-3 py-2 border rounded-md text-xs md:text-sm font-mono break-all ${isApiKeyHash ? "border-warning-300 dark:border-warning-600 bg-warning-50 dark:bg-warning-900/20 text-warning-700 dark:text-warning-300" : "border-primary-300 dark:border-primary-600 bg-white dark:bg-secondary-800 text-secondary-900 dark:text-white"}`}
								/>
								<button
									type="button"
									onClick={() =>
										copyToClipboard(
											`curl ${getCurlFlags()} ${getInstallUrl()} -H "X-API-ID: ${effectiveApiId}" -H "X-API-KEY: ${effectiveApiKey}" | ${getShellCommand()}`,
										)
									}
									disabled={isApiKeyHash}
									className="btn-outline flex items-center justify-center gap-1 whitespace-nowrap disabled:opacity-50 disabled:cursor-not-allowed"
								>
									<Copy className="h-4 w-4" />
									Copy
								</button>
							</div>
						</div>
					</div>
				)}

				{activeTab === "credentials" && (
					<div className="space-y-4 md:space-y-6">
						<div className="bg-secondary-50 dark:bg-secondary-700 rounded-lg p-3 md:p-4">
							<h4 className="text-xs md:text-sm font-medium text-secondary-900 dark:text-white mb-3">
								API Credentials
							</h4>
							<div className="space-y-4">
								<div>
									<label
										htmlFor={apiIdInputId}
										className="block text-xs md:text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
									>
										API ID
									</label>
									<div className="flex flex-col sm:flex-row items-stretch sm:items-center gap-2">
										<input
											id={apiIdInputId}
											type="text"
											value={effectiveApiId}
											readOnly
											className="flex-1 px-3 py-2 border border-secondary-300 dark:border-secondary-600 rounded-md bg-secondary-50 dark:bg-secondary-800 text-xs md:text-sm font-mono text-secondary-900 dark:text-white break-all"
										/>
										<button
											type="button"
											onClick={() => copyToClipboard(effectiveApiId)}
											className="btn-outline flex items-center justify-center gap-1 whitespace-nowrap"
										>
											<Copy className="h-4 w-4" />
											Copy
										</button>
									</div>
								</div>

								<div>
									<label
										htmlFor={apiKeyInputId}
										className="block text-xs md:text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
									>
										API Key
									</label>
									{isApiKeyHash && (
										<div className="mb-2 p-2 bg-warning-50 dark:bg-warning-900/20 border border-warning-200 dark:border-warning-700 rounded-lg">
											<p className="text-xs text-warning-700 dark:text-warning-300">
												The stored key is a hash. Regenerate credentials to get
												a new plaintext key.
											</p>
										</div>
									)}
									<div className="flex flex-col sm:flex-row items-stretch sm:items-center gap-2">
										<input
											id={apiKeyInputId}
											type={showApiKey ? "text" : "password"}
											value={
												isApiKeyHash ? "(hashed - not usable)" : effectiveApiKey
											}
											readOnly
											disabled={isApiKeyHash}
											className={`flex-1 px-3 py-2 border rounded-md text-xs md:text-sm font-mono break-all ${isApiKeyHash ? "border-warning-300 dark:border-warning-600 bg-warning-50 dark:bg-warning-900/20 text-warning-700 dark:text-warning-300" : "border-secondary-300 dark:border-secondary-600 bg-secondary-50 dark:bg-secondary-800 text-secondary-900 dark:text-white"}`}
										/>
										<button
											type="button"
											onClick={() => setShowApiKey(!showApiKey)}
											disabled={isApiKeyHash}
											className="btn-outline flex items-center justify-center gap-1 whitespace-nowrap disabled:opacity-50 disabled:cursor-not-allowed"
										>
											{showApiKey ? (
												<EyeOff className="h-4 w-4" />
											) : (
												<Eye className="h-4 w-4" />
											)}
										</button>
										<button
											type="button"
											onClick={() => copyToClipboard(effectiveApiKey)}
											disabled={isApiKeyHash}
											className="btn-outline flex items-center justify-center gap-1 whitespace-nowrap disabled:opacity-50 disabled:cursor-not-allowed"
										>
											<Copy className="h-4 w-4" />
											Copy
										</button>
									</div>
								</div>
							</div>
						</div>

						<div className="bg-warning-50 dark:bg-warning-900 border border-warning-200 dark:border-warning-700 rounded-lg p-3 md:p-4">
							<div className="flex items-start gap-3">
								<AlertTriangle className="h-5 w-5 text-warning-400 dark:text-warning-300 flex-shrink-0 mt-0.5" />
								<div className="min-w-0">
									<h3 className="text-xs md:text-sm font-medium text-warning-800 dark:text-warning-200">
										Security Notice
									</h3>
									<p className="text-xs md:text-sm text-warning-700 dark:text-warning-300 mt-1">
										Keep these credentials secure. They provide full access to
										this host's monitoring data.
									</p>
								</div>
							</div>
						</div>
					</div>
				)}

				<div className="flex justify-end pt-4 md:pt-6">
					<button
						type="button"
						onClick={onClose}
						className="btn-primary w-full sm:w-auto"
					>
						Close
					</button>
				</div>
			</div>
		</div>
	);
};

export default CredentialsModal;
