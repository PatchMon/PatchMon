import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, Bot, Check, Eye, EyeOff, Loader2, RefreshCw, Sparkles, X } from "lucide-react";
import { aiAPI } from "../../utils/api";
import SettingsLayout from "../../components/SettingsLayout";

const AiSettings = () => {
	const queryClient = useQueryClient();
	const [showApiKey, setShowApiKey] = useState(false);
	const [apiKeyInput, setApiKeyInput] = useState("");
	const [testResult, setTestResult] = useState(null);

	// Fetch AI settings
	const { data: settings, isLoading: settingsLoading } = useQuery({
		queryKey: ["aiSettings"],
		queryFn: () => aiAPI.getSettings().then((res) => res.data),
	});

	// Fetch available providers
	const { data: providersData } = useQuery({
		queryKey: ["aiProviders"],
		queryFn: () => aiAPI.getProviders().then((res) => res.data),
	});

	const providers = providersData?.providers || [];

	// Update settings mutation
	const updateMutation = useMutation({
		mutationFn: (data) => aiAPI.updateSettings(data),
		onSuccess: () => {
			queryClient.invalidateQueries(["aiSettings"]);
			setApiKeyInput("");
		},
	});

	// Test connection mutation
	const testMutation = useMutation({
		mutationFn: () => aiAPI.testConnection(),
		onSuccess: (res) => {
			setTestResult({ success: true, message: res.data.message });
		},
		onError: (err) => {
			setTestResult({
				success: false,
				message: err.response?.data?.error || "Connection test failed",
			});
		},
	});

	const handleToggleEnabled = () => {
		updateMutation.mutate({ ai_enabled: !settings?.ai_enabled });
	};

	const handleProviderChange = (e) => {
		const provider = e.target.value;
		const providerConfig = providers.find((p) => p.id === provider);
		updateMutation.mutate({
			ai_provider: provider,
			ai_model: providerConfig?.defaultModel || null,
		});
	};

	const handleModelChange = (e) => {
		updateMutation.mutate({ ai_model: e.target.value });
	};

	const handleSaveApiKey = () => {
		if (apiKeyInput.trim()) {
			updateMutation.mutate({ ai_api_key: apiKeyInput.trim() });
		}
	};

	const selectedProvider = providers.find((p) => p.id === settings?.ai_provider);

	if (settingsLoading) {
		return (
			<SettingsLayout>
				<div className="flex items-center justify-center h-64">
					<Loader2 className="h-8 w-8 animate-spin text-primary-500" />
				</div>
			</SettingsLayout>
		);
	}

	return (
		<SettingsLayout>
			<div className="space-y-6">
				{/* Header */}
				<div className="flex items-center gap-3">
					<div className="p-2 bg-primary-100 dark:bg-primary-900/30 rounded-lg">
						<Bot className="h-6 w-6 text-primary-600 dark:text-primary-400" />
					</div>
					<div>
						<h1 className="text-xl font-semibold text-secondary-900 dark:text-white">
							AI Terminal Assistant
						</h1>
						<p className="text-sm text-secondary-500 dark:text-secondary-400">
							Configure AI-powered terminal assistance and command completion
						</p>
					</div>
				</div>

				{/* API Key Invalid Warning */}
				{settings?.ai_api_key_invalid && (
					<div className="bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 rounded-lg p-4">
						<div className="flex items-start gap-3">
							<AlertTriangle className="h-5 w-5 text-amber-600 dark:text-amber-400 flex-shrink-0 mt-0.5" />
							<div>
								<h3 className="font-medium text-amber-800 dark:text-amber-200">
									API Key Needs to be Re-entered
								</h3>
								<p className="text-sm text-amber-700 dark:text-amber-300 mt-1">
									Your AI API key was encrypted with a different secret and cannot be decrypted.
									This can happen if <code className="bg-amber-100 dark:bg-amber-900 px-1 rounded">SESSION_SECRET</code> was changed or not set consistently.
								</p>
								<p className="text-sm text-amber-600 dark:text-amber-400 mt-2">
									Please re-enter your API key below to restore AI functionality.
								</p>
							</div>
						</div>
					</div>
				)}

				{/* Enable/Disable Toggle */}
				<div className="bg-secondary-50 dark:bg-secondary-900/50 rounded-lg p-4 border border-secondary-200 dark:border-secondary-700">
					<div className="flex items-center justify-between">
						<div>
							<h3 className="font-medium text-secondary-900 dark:text-white">
								Enable AI Assistant
							</h3>
							<p className="text-sm text-secondary-500 dark:text-secondary-400">
								Enable AI-powered terminal assistance in the SSH terminal
							</p>
						</div>
						<button
							type="button"
							onClick={handleToggleEnabled}
							disabled={updateMutation.isPending}
							className={`relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 ${
								settings?.ai_enabled
									? "bg-primary-600"
									: "bg-secondary-300 dark:bg-secondary-600"
							}`}
						>
							<span
								className={`inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out ${
									settings?.ai_enabled ? "translate-x-5" : "translate-x-0"
								}`}
							/>
						</button>
					</div>
				</div>

				{/* Provider Selection */}
				<div className="bg-white dark:bg-secondary-800 rounded-lg p-4 border border-secondary-200 dark:border-secondary-700">
					<h3 className="font-medium text-secondary-900 dark:text-white mb-4">
						AI Provider Configuration
					</h3>

					<div className="space-y-4">
						{/* Provider Dropdown */}
						<div>
							<label
								htmlFor="ai-provider"
								className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-1"
							>
								Provider
							</label>
							<select
								id="ai-provider"
								value={settings?.ai_provider || "openrouter"}
								onChange={handleProviderChange}
								disabled={updateMutation.isPending}
								className="w-full px-3 py-2 bg-white dark:bg-secondary-900 border border-secondary-300 dark:border-secondary-600 rounded-md text-secondary-900 dark:text-white focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
							>
								{providers.map((provider) => (
									<option key={provider.id} value={provider.id}>
										{provider.name}
									</option>
								))}
							</select>
							<p className="mt-1 text-xs text-secondary-500 dark:text-secondary-400">
								{settings?.ai_provider === "openrouter" &&
									"Access multiple AI models through a single API"}
								{settings?.ai_provider === "anthropic" &&
									"Direct access to Anthropic Claude models"}
								{settings?.ai_provider === "openai" &&
									"Direct access to OpenAI GPT models"}
								{settings?.ai_provider === "gemini" &&
									"Direct access to Google Gemini models"}
							</p>
						</div>

						{/* Model Selection */}
						<div>
							<label
								htmlFor="ai-model"
								className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-1"
							>
								Model
							</label>
							<select
								id="ai-model"
								value={settings?.ai_model || selectedProvider?.defaultModel || ""}
								onChange={handleModelChange}
								disabled={updateMutation.isPending}
								className="w-full px-3 py-2 bg-white dark:bg-secondary-900 border border-secondary-300 dark:border-secondary-600 rounded-md text-secondary-900 dark:text-white focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
							>
								{selectedProvider?.models.map((model) => (
									<option key={model.id} value={model.id}>
										{model.name}
									</option>
								))}
							</select>
						</div>

						{/* API Key Input */}
						<div>
							<label
								htmlFor="ai-api-key"
								className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-1"
							>
								API Key
							</label>
							<div className="flex gap-2">
								<div className="relative flex-1">
									<input
										id="ai-api-key"
										type={showApiKey ? "text" : "password"}
										value={apiKeyInput}
										onChange={(e) => setApiKeyInput(e.target.value)}
										placeholder={
											settings?.ai_api_key_set
												? "••••••••••••••••"
												: "Enter your API key"
										}
										className="w-full px-3 py-2 pr-10 bg-white dark:bg-secondary-900 border border-secondary-300 dark:border-secondary-600 rounded-md text-secondary-900 dark:text-white focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
									/>
									<button
										type="button"
										onClick={() => setShowApiKey(!showApiKey)}
										className="absolute right-2 top-1/2 -translate-y-1/2 text-secondary-400 hover:text-secondary-600 dark:hover:text-secondary-300"
									>
										{showApiKey ? (
											<EyeOff className="h-4 w-4" />
										) : (
											<Eye className="h-4 w-4" />
										)}
									</button>
								</div>
								<button
									type="button"
									onClick={handleSaveApiKey}
									disabled={!apiKeyInput.trim() || updateMutation.isPending}
									className="px-4 py-2 bg-primary-600 text-white rounded-md hover:bg-primary-700 disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2"
								>
									{updateMutation.isPending ? (
										<Loader2 className="h-4 w-4 animate-spin" />
									) : (
										<Check className="h-4 w-4" />
									)}
									Save
								</button>
							</div>
							{settings?.ai_api_key_set && !settings?.ai_api_key_invalid && (
								<p className="mt-1 text-xs text-green-600 dark:text-green-400">
									API key is configured
								</p>
							)}
							{settings?.ai_api_key_invalid && (
								<p className="mt-1 text-xs text-amber-600 dark:text-amber-400">
									API key needs to be re-entered (encryption key changed)
								</p>
							)}
							<p className="mt-1 text-xs text-secondary-500 dark:text-secondary-400">
								Get your API key from:{" "}
								{settings?.ai_provider === "openrouter" && (
									<a
										href="https://openrouter.ai/keys"
										target="_blank"
										rel="noopener noreferrer"
										className="text-primary-600 hover:underline"
									>
										openrouter.ai/keys
									</a>
								)}
								{settings?.ai_provider === "anthropic" && (
									<a
										href="https://console.anthropic.com/settings/keys"
										target="_blank"
										rel="noopener noreferrer"
										className="text-primary-600 hover:underline"
									>
										console.anthropic.com
									</a>
								)}
								{settings?.ai_provider === "openai" && (
									<a
										href="https://platform.openai.com/api-keys"
										target="_blank"
										rel="noopener noreferrer"
										className="text-primary-600 hover:underline"
									>
										platform.openai.com
									</a>
								)}
								{settings?.ai_provider === "gemini" && (
									<a
										href="https://aistudio.google.com/apikey"
										target="_blank"
										rel="noopener noreferrer"
										className="text-primary-600 hover:underline"
									>
										aistudio.google.com
									</a>
								)}
							</p>
						</div>

						{/* Test Connection */}
						<div className="pt-2 border-t border-secondary-200 dark:border-secondary-700">
							<button
								type="button"
								onClick={() => {
									setTestResult(null);
									testMutation.mutate();
								}}
								disabled={!settings?.ai_api_key_set || testMutation.isPending}
								className="px-4 py-2 bg-secondary-100 dark:bg-secondary-700 text-secondary-700 dark:text-secondary-200 rounded-md hover:bg-secondary-200 dark:hover:bg-secondary-600 disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2"
							>
								{testMutation.isPending ? (
									<Loader2 className="h-4 w-4 animate-spin" />
								) : (
									<RefreshCw className="h-4 w-4" />
								)}
								Test Connection
							</button>
							{testResult && (
								<div
									className={`mt-2 p-2 rounded-md text-sm ${
										testResult.success
											? "bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-400"
											: "bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-400"
									}`}
								>
									{testResult.success ? (
										<Check className="h-4 w-4 inline mr-1" />
									) : (
										<X className="h-4 w-4 inline mr-1" />
									)}
									{testResult.message}
								</div>
							)}
						</div>
					</div>
				</div>

				{/* Features Info */}
				<div className="bg-gradient-to-r from-primary-50 to-purple-50 dark:from-primary-900/20 dark:to-purple-900/20 rounded-lg p-4 border border-primary-200 dark:border-primary-800">
					<div className="flex items-start gap-3">
						<Sparkles className="h-5 w-5 text-primary-600 dark:text-primary-400 mt-0.5" />
						<div>
							<h3 className="font-medium text-secondary-900 dark:text-white mb-2">
								AI Terminal Features
							</h3>
							<ul className="text-sm text-secondary-600 dark:text-secondary-300 space-y-1">
								<li className="flex items-center gap-2">
									<Check className="h-4 w-4 text-green-500" />
									<span>
										<strong>AI Assistant Panel</strong> - Ask questions about
										terminal output and get help
									</span>
								</li>
								<li className="flex items-center gap-2">
									<Check className="h-4 w-4 text-green-500" />
									<span>
										<strong>Command Completion</strong> - Get AI-powered command
										suggestions as you type
									</span>
								</li>
								<li className="flex items-center gap-2">
									<Check className="h-4 w-4 text-green-500" />
									<span>
										<strong>Error Diagnosis</strong> - Let AI explain errors and
										suggest solutions
									</span>
								</li>
								<li className="flex items-center gap-2">
									<Check className="h-4 w-4 text-green-500" />
									<span>
										<strong>Context Aware</strong> - AI uses terminal history
										for better suggestions
									</span>
								</li>
							</ul>
						</div>
					</div>
				</div>
			</div>
		</SettingsLayout>
	);
};

export default AiSettings;
