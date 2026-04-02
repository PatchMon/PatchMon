import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
	AlertTriangle,
	Check,
	ChevronDown,
	ChevronUp,
	Eye,
	EyeOff,
	Loader2,
	X,
} from "lucide-react";
import { useEffect, useState } from "react";
import DiscordIcon from "../../components/DiscordIcon";
import SettingsLayout from "../../components/SettingsLayout";
import { discordAPI } from "../../utils/api";

const DEFAULT_BUTTON_TEXT = "Login with Discord";

const getFormDataFromSettings = (settings) => ({
	discord_oauth_enabled: settings?.discord_oauth_enabled || false,
	discord_client_id: settings?.discord_client_id || "",
	discord_redirect_uri: settings?.discord_redirect_uri || "",
	discord_button_text: settings?.discord_button_text || DEFAULT_BUTTON_TEXT,
});

const getComparableFieldValue = (source, field) => source?.[field] ?? "";

const DiscordSettings = () => {
	const queryClient = useQueryClient();
	const [showSecret, setShowSecret] = useState(false);
	const [secretInput, setSecretInput] = useState("");
	const [showSetupGuide, setShowSetupGuide] = useState(false);
	const [formData, setFormData] = useState(() => getFormDataFromSettings());

	// Fetch Discord settings
	const { data: settings, isLoading: settingsLoading } = useQuery({
		queryKey: ["discordSettings"],
		queryFn: () => discordAPI.getSettings().then((res) => res.data),
	});

	useEffect(() => {
		if (settings) {
			setFormData(getFormDataFromSettings(settings));
		}
	}, [settings]);

	// Update settings mutation
	const updateMutation = useMutation({
		mutationFn: (data) =>
			discordAPI.updateSettings(data).then((res) => res.data),
		onSuccess: (updatedSettings, variables) => {
			queryClient.setQueryData(["discordSettings"], (currentSettings = {}) => ({
				...currentSettings,
				...updatedSettings,
			}));
			setFormData((currentFormData) => ({
				...currentFormData,
				...getFormDataFromSettings(updatedSettings),
			}));
			if (Object.hasOwn(variables, "discord_client_secret")) {
				setSecretInput("");
			}
		},
		onError: () => {
			queryClient.invalidateQueries(["discordSettings"]);
		},
	});

	const handleToggleEnabled = () => {
		const nextValue = !formData.discord_oauth_enabled;
		setFormData((currentFormData) => ({
			...currentFormData,
			discord_oauth_enabled: nextValue,
		}));
		updateMutation.mutate({ discord_oauth_enabled: nextValue });
	};

	const handleFieldChange = (field, value) => {
		setFormData((currentFormData) => ({
			...currentFormData,
			[field]: value,
		}));
	};

	const handleFieldBlur = (field) => {
		if (
			getComparableFieldValue(formData, field) ===
			getComparableFieldValue(settings, field)
		) {
			return;
		}

		updateMutation.mutate({ [field]: formData[field] });
	};

	const handleSaveSecret = () => {
		if (secretInput.trim()) {
			updateMutation.mutate({ discord_client_secret: secretInput.trim() });
		}
	};

	const handleClearSecret = () => {
		updateMutation.mutate({ discord_client_secret: "" });
	};

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
					<div
						className="p-2 rounded-lg"
						style={{ backgroundColor: "#5865F2" }}
					>
						<DiscordIcon className="h-6 w-6 text-white" />
					</div>
					<div>
						<h1 className="text-xl font-semibold text-secondary-900 dark:text-white">
							Discord Authentication
						</h1>
						<p className="text-sm text-secondary-500 dark:text-secondary-400">
							Allow users to sign in with their Discord account
						</p>
					</div>
				</div>

				{/* Enable/Disable Toggle */}
				<div className="bg-secondary-50 dark:bg-secondary-900/50 rounded-lg p-4 border border-secondary-200 dark:border-secondary-700">
					<div className="flex items-center justify-between">
						<div>
							<h3 className="font-medium text-secondary-900 dark:text-white">
								Enable Discord OAuth
							</h3>
							<p className="text-sm text-secondary-500 dark:text-secondary-400">
								Allow users to log in and link their Discord accounts
							</p>
						</div>
						<button
							type="button"
							onClick={handleToggleEnabled}
							className={`relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-[#5865F2] focus:ring-offset-2 ${
								formData.discord_oauth_enabled
									? "bg-[#5865F2]"
									: "bg-secondary-300 dark:bg-secondary-600"
							}`}
						>
							<span
								className={`inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out ${
									formData.discord_oauth_enabled
										? "translate-x-5"
										: "translate-x-0"
								}`}
							/>
						</button>
					</div>
				</div>

				{/* Configuration */}
				<div className="bg-white dark:bg-secondary-800 rounded-lg p-4 border border-secondary-200 dark:border-secondary-700">
					<h3 className="font-medium text-secondary-900 dark:text-white mb-4">
						OAuth2 Configuration
					</h3>

					<div className="space-y-4">
						{/* Client ID */}
						<div>
							<label
								htmlFor="discord-client-id"
								className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-1"
							>
								Client ID
							</label>
							<input
								id="discord-client-id"
								type="text"
								value={formData.discord_client_id}
								onChange={(e) =>
									handleFieldChange("discord_client_id", e.target.value)
								}
								onBlur={() => handleFieldBlur("discord_client_id")}
								placeholder="Enter your Discord application Client ID"
								className="w-full px-3 py-2 bg-white dark:bg-secondary-900 border border-secondary-300 dark:border-secondary-600 rounded-md text-secondary-900 dark:text-white focus:ring-2 focus:ring-[#5865F2] focus:border-[#5865F2] placeholder-secondary-400"
							/>
						</div>

						{/* Client Secret */}
						<div>
							<label
								htmlFor="discord-client-secret"
								className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-1"
							>
								Client Secret
								{settings?.discord_client_secret_set ? (
									<span className="ml-2 inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">
										<Check className="h-3 w-3 mr-1" />
										Set
									</span>
								) : (
									<span className="ml-2 inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-secondary-100 text-secondary-600 dark:bg-secondary-700 dark:text-secondary-400">
										Not set
									</span>
								)}
							</label>
							<div className="flex gap-2">
								<div className="relative flex-1">
									<input
										id="discord-client-secret"
										type={showSecret ? "text" : "password"}
										value={secretInput}
										onChange={(e) => setSecretInput(e.target.value)}
										placeholder={
											settings?.discord_client_secret_set
												? "Enter new secret to replace"
												: "Enter your Discord Client Secret"
										}
										className="w-full px-3 py-2 pr-10 bg-white dark:bg-secondary-900 border border-secondary-300 dark:border-secondary-600 rounded-md text-secondary-900 dark:text-white focus:ring-2 focus:ring-[#5865F2] focus:border-[#5865F2] placeholder-secondary-400"
									/>
									<button
										type="button"
										onClick={() => setShowSecret(!showSecret)}
										className="absolute right-2 top-1/2 -translate-y-1/2 text-secondary-400 hover:text-secondary-600 dark:hover:text-secondary-300"
									>
										{showSecret ? (
											<EyeOff className="h-4 w-4" />
										) : (
											<Eye className="h-4 w-4" />
										)}
									</button>
								</div>
								<button
									type="button"
									onClick={handleSaveSecret}
									disabled={!secretInput.trim()}
									className="px-4 py-2 text-sm font-medium text-white rounded-md hover:opacity-90 disabled:opacity-50 disabled:cursor-not-allowed"
									style={{ backgroundColor: "#5865F2" }}
								>
									Save
								</button>
								{settings?.discord_client_secret_set && (
									<button
										type="button"
										onClick={handleClearSecret}
										className="px-3 py-2 text-sm font-medium text-red-700 dark:text-red-400 border border-red-300 dark:border-red-700 rounded-md hover:bg-red-50 dark:hover:bg-red-900/20"
									>
										<X className="h-4 w-4" />
									</button>
								)}
							</div>
						</div>

						{/* Redirect URI */}
						<div>
							<label
								htmlFor="discord-redirect-uri"
								className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-1"
							>
								Redirect URI
							</label>
							<input
								id="discord-redirect-uri"
								type="text"
								value={formData.discord_redirect_uri}
								onChange={(e) =>
									handleFieldChange("discord_redirect_uri", e.target.value)
								}
								onBlur={() => handleFieldBlur("discord_redirect_uri")}
								placeholder="https://your-domain.com/api/v1/auth/discord/callback"
								className="w-full px-3 py-2 bg-white dark:bg-secondary-900 border border-secondary-300 dark:border-secondary-600 rounded-md text-secondary-900 dark:text-white focus:ring-2 focus:ring-[#5865F2] focus:border-[#5865F2] placeholder-secondary-400"
							/>
							{settings?.server_url && (
								<p className="mt-1 text-xs text-secondary-500 dark:text-secondary-400">
									Your redirect URI should be:{" "}
									<code className="bg-secondary-100 dark:bg-secondary-700 px-1 rounded">
										{settings.server_url}/api/v1/auth/discord/callback
									</code>
								</p>
							)}
						</div>

						{/* Button Text */}
						<div>
							<label
								htmlFor="discord-button-text"
								className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-1"
							>
								Button Text
							</label>
							<input
								id="discord-button-text"
								type="text"
								value={formData.discord_button_text}
								onChange={(e) =>
									handleFieldChange("discord_button_text", e.target.value)
								}
								onBlur={() => handleFieldBlur("discord_button_text")}
								placeholder="Login with Discord"
								className="w-full px-3 py-2 bg-white dark:bg-secondary-900 border border-secondary-300 dark:border-secondary-600 rounded-md text-secondary-900 dark:text-white focus:ring-2 focus:ring-[#5865F2] focus:border-[#5865F2] placeholder-secondary-400"
							/>
						</div>
					</div>
				</div>

				{/* Setup Instructions */}
				<div className="bg-white dark:bg-secondary-800 rounded-lg border border-secondary-200 dark:border-secondary-700">
					<button
						type="button"
						onClick={() => setShowSetupGuide(!showSetupGuide)}
						className="w-full flex items-center justify-between p-4 text-left"
					>
						<div className="flex items-center gap-2">
							<AlertTriangle className="h-5 w-5 text-amber-500" />
							<h3 className="font-medium text-secondary-900 dark:text-white">
								Setup Instructions
							</h3>
						</div>
						{showSetupGuide ? (
							<ChevronUp className="h-5 w-5 text-secondary-400" />
						) : (
							<ChevronDown className="h-5 w-5 text-secondary-400" />
						)}
					</button>

					{showSetupGuide && (
						<div className="px-4 pb-4 border-t border-secondary-200 dark:border-secondary-700 pt-4">
							<ol className="space-y-3 text-sm text-secondary-700 dark:text-secondary-300">
								<li className="flex gap-3">
									<span
										className="flex-shrink-0 flex items-center justify-center h-6 w-6 rounded-full text-xs font-medium text-white"
										style={{ backgroundColor: "#5865F2" }}
									>
										1
									</span>
									<span>
										Go to the{" "}
										<a
											href="https://discord.com/developers/applications"
											target="_blank"
											rel="noopener noreferrer"
											className="font-medium underline"
											style={{ color: "#5865F2" }}
										>
											Discord Developer Portal
										</a>
									</span>
								</li>
								<li className="flex gap-3">
									<span
										className="flex-shrink-0 flex items-center justify-center h-6 w-6 rounded-full text-xs font-medium text-white"
										style={{ backgroundColor: "#5865F2" }}
									>
										2
									</span>
									<span>
										Click &quot;New Application&quot; and give it a name
									</span>
								</li>
								<li className="flex gap-3">
									<span
										className="flex-shrink-0 flex items-center justify-center h-6 w-6 rounded-full text-xs font-medium text-white"
										style={{ backgroundColor: "#5865F2" }}
									>
										3
									</span>
									<span>
										Navigate to the &quot;OAuth2&quot; section in the sidebar
									</span>
								</li>
								<li className="flex gap-3">
									<span
										className="flex-shrink-0 flex items-center justify-center h-6 w-6 rounded-full text-xs font-medium text-white"
										style={{ backgroundColor: "#5865F2" }}
									>
										4
									</span>
									<span>
										Add your redirect URI:{" "}
										<code className="bg-secondary-100 dark:bg-secondary-700 px-1 rounded">
											{settings?.server_url
												? `${settings.server_url}/api/v1/auth/discord/callback`
												: "https://your-domain.com/api/v1/auth/discord/callback"}
										</code>
									</span>
								</li>
								<li className="flex gap-3">
									<span
										className="flex-shrink-0 flex items-center justify-center h-6 w-6 rounded-full text-xs font-medium text-white"
										style={{ backgroundColor: "#5865F2" }}
									>
										5
									</span>
									<span>
										Copy the Client ID and Client Secret into the fields above
									</span>
								</li>
								<li className="flex gap-3">
									<span
										className="flex-shrink-0 flex items-center justify-center h-6 w-6 rounded-full text-xs font-medium text-white"
										style={{ backgroundColor: "#5865F2" }}
									>
										6
									</span>
									<span>
										Set scopes to:{" "}
										<code className="bg-secondary-100 dark:bg-secondary-700 px-1 rounded">
											identify email
										</code>
									</span>
								</li>
							</ol>
						</div>
					)}
				</div>
			</div>
		</SettingsLayout>
	);
};

export default DiscordSettings;
