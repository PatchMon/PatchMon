import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
	AlertTriangle,
	Check,
	ChevronDown,
	ChevronUp,
	Eye,
	EyeOff,
	KeyRound,
	Loader2,
	X,
} from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useToast } from "../../contexts/ToastContext";
import { oidcAPI, permissionsAPI } from "../../utils/api";

// Map PatchMon role to OIDC form field (backend supports these 5 + default)
const ROLE_TO_OIDC_FIELD = {
	superadmin: "oidc_superadmin_group",
	admin: "oidc_admin_group",
	host_manager: "oidc_host_manager_group",
	readonly: "oidc_readonly_group",
	user: "oidc_user_group",
};

// Default placeholders for role mapping (common IdP group names)
const ROLE_MAPPING_DEFAULTS = {
	oidc_default_role: "user",
	oidc_superadmin_group: "superadmin",
	oidc_admin_group: "admin",
	oidc_host_manager_group: "host_manager",
	oidc_readonly_group: "readonly",
	oidc_user_group: "user",
};

const ToggleCard = ({ label, description, checked, onChange, disabled }) => (
	<div className="flex items-start justify-between gap-3 p-3 rounded-lg bg-white dark:bg-secondary-800 border border-secondary-200 dark:border-secondary-600">
		<div className="min-w-0">
			<p className="text-sm font-medium text-secondary-900 dark:text-white">
				{label}
			</p>
			<p className="text-xs text-secondary-500 dark:text-secondary-400 mt-0.5">
				{description}
			</p>
		</div>
		<button
			type="button"
			onClick={onChange}
			disabled={disabled}
			className={`relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-md border-2 border-transparent transition-colors ${
				checked ? "bg-primary-600" : "bg-secondary-300 dark:bg-secondary-600"
			} disabled:opacity-50 disabled:cursor-not-allowed`}
		>
			<span
				className={`inline-block h-5 w-5 transform rounded-md bg-white shadow transition ${
					checked ? "translate-x-5" : "translate-x-0"
				}`}
			/>
		</button>
	</div>
);

const OidcSettings = () => {
	const queryClient = useQueryClient();
	const toast = useToast();
	const [showSecret, setShowSecret] = useState(false);
	const [secretInput, setSecretInput] = useState("");
	const [showRoleMapping, setShowRoleMapping] = useState(false);
	const [showSetupGuide, setShowSetupGuide] = useState(false);

	// Local form state - only saved when user clicks Apply
	const [form, setForm] = useState({
		oidc_issuer_url: "",
		oidc_client_id: "",
		oidc_redirect_uri: "",
		oidc_scopes: "openid email profile groups",
		oidc_button_text: "Login with SSO",
		oidc_sync_roles: false,
		oidc_default_role: "user",
		oidc_superadmin_group: "",
		oidc_admin_group: "",
		oidc_host_manager_group: "",
		oidc_readonly_group: "",
		oidc_user_group: "",
		oidc_disable_local_auth: false,
		oidc_auto_create_users: true,
		oidc_enforce_https: true,
	});

	// Fetch OIDC settings
	const { data: settings, isLoading: settingsLoading } = useQuery({
		queryKey: ["oidcSettings"],
		queryFn: () => oidcAPI.getSettings().then((res) => res.data),
	});

	// Fetch roles for dynamic role mapping table
	const { data: rolesData } = useQuery({
		queryKey: ["rolePermissions"],
		queryFn: () => permissionsAPI.getRoles().then((res) => res.data),
	});

	// Sort roles: superadmin > admin > host_manager > user > readonly, then custom alphabetically
	const sortedRoles = useMemo(() => {
		if (!rolesData) return [];
		const order = {
			superadmin: 0,
			admin: 1,
			host_manager: 2,
			user: 3,
			readonly: 4,
		};
		return [...rolesData].sort((a, b) => {
			const aOrder = order[a.role] ?? 999;
			const bOrder = order[b.role] ?? 999;
			if (aOrder !== bOrder) return aOrder - bOrder;
			return a.role.localeCompare(b.role);
		});
	}, [rolesData]);

	// Sync form from server when settings load; pre-fill redirect URI from callback_url when empty
	useEffect(() => {
		if (!settings) return;
		setForm({
			oidc_issuer_url: settings.oidc_issuer_url || "",
			oidc_client_id: settings.oidc_client_id || "",
			oidc_redirect_uri:
				settings.oidc_redirect_uri || settings?.callback_url || "",
			oidc_scopes: settings.oidc_scopes || "openid email profile groups",
			oidc_button_text: settings.oidc_button_text || "Login with SSO",
			oidc_sync_roles: settings.oidc_sync_roles ?? false,
			oidc_default_role: settings.oidc_default_role || "user",
			oidc_superadmin_group: settings.oidc_superadmin_group || "",
			oidc_admin_group: settings.oidc_admin_group || "",
			oidc_host_manager_group: settings.oidc_host_manager_group || "",
			oidc_readonly_group: settings.oidc_readonly_group || "",
			oidc_user_group: settings.oidc_user_group || "",
			oidc_disable_local_auth: settings.oidc_disable_local_auth ?? false,
			oidc_auto_create_users: settings.oidc_auto_create_users ?? true,
			oidc_enforce_https: settings.oidc_enforce_https ?? true,
		});
	}, [settings]);

	// Update settings mutation
	const updateMutation = useMutation({
		mutationFn: (data) => oidcAPI.updateSettings(data),
		onSuccess: () => {
			queryClient.invalidateQueries(["oidcSettings"]);
			setSecretInput("");
			toast.success("OIDC settings saved");
		},
		onError: (err) => {
			toast.error(err.response?.data?.error || "Failed to save OIDC settings");
		},
	});

	// Import from .env mutation
	const importMutation = useMutation({
		mutationFn: () => oidcAPI.importFromEnv(),
		onSuccess: () => {
			queryClient.invalidateQueries(["oidcSettings"]);
			toast.success("OIDC settings imported from .env");
		},
		onError: (err) => {
			toast.error(err.response?.data?.error || "Failed to import from .env");
		},
	});

	const handleToggleEnabled = () => {
		updateMutation.mutate({
			oidc_enabled: !settings?.oidc_enabled,
		});
	};

	const handleFieldChange = (field, value) => {
		setForm((prev) => ({ ...prev, [field]: value }));
	};

	const handleApply = () => {
		const payload = {
			oidc_enabled: settings?.oidc_enabled ?? false,
			oidc_issuer_url: form.oidc_issuer_url || "",
			oidc_client_id: form.oidc_client_id || "",
			oidc_redirect_uri: form.oidc_redirect_uri || settings?.callback_url || "",
			oidc_scopes: form.oidc_scopes || "openid email profile groups",
			oidc_button_text: form.oidc_button_text || "Login with SSO",
			oidc_sync_roles: form.oidc_sync_roles,
			oidc_default_role: form.oidc_default_role || "user",
			oidc_superadmin_group: form.oidc_superadmin_group || "",
			oidc_admin_group: form.oidc_admin_group || "",
			oidc_host_manager_group: form.oidc_host_manager_group || "",
			oidc_readonly_group: form.oidc_readonly_group || "",
			oidc_user_group: form.oidc_user_group || "",
			oidc_disable_local_auth: form.oidc_disable_local_auth,
			oidc_auto_create_users: form.oidc_auto_create_users,
			oidc_enforce_https: form.oidc_enforce_https ?? true,
		};
		if (secretInput.trim()) {
			payload.oidc_client_secret = secretInput.trim();
		}
		updateMutation.mutate(payload);
	};

	const handleSaveSecret = () => {
		if (secretInput.trim()) {
			updateMutation.mutate({ oidc_client_secret: secretInput.trim() });
		}
	};

	const handleClearSecret = () => {
		updateMutation.mutate({ oidc_client_secret: "" });
	};

	if (settingsLoading) {
		return (
			<div className="flex items-center justify-center h-64">
				<Loader2 className="h-8 w-8 animate-spin text-primary-500" />
			</div>
		);
	}

	return (
		<div className="space-y-6">
			{/* Header */}
			<div className="flex items-center gap-3">
				<div className="p-2 rounded-lg bg-primary-600">
					<KeyRound className="h-6 w-6 text-white" />
				</div>
				<div>
					<h1 className="text-xl font-semibold text-secondary-900 dark:text-white">
						OIDC / SSO
					</h1>
					<p className="text-sm text-secondary-500 dark:text-white">
						Allow users to sign in with OpenID Connect (SSO)
					</p>
				</div>
			</div>

			{/* Import banner when configured via .env */}
			{settings?.configured_via_env && (
				<div className="bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 rounded-lg p-4">
					<div className="flex items-start gap-3">
						<AlertTriangle className="h-5 w-5 text-amber-600 dark:text-amber-400 flex-shrink-0 mt-0.5" />
						<div className="flex-1 min-w-0">
							<h3 className="font-medium text-amber-800 dark:text-amber-200">
								OIDC is configured via .env
							</h3>
							<p className="text-sm text-amber-700 dark:text-amber-300 mt-1">
								Import these settings and save to database to manage from the
								UI. After saving, remove OIDC_* from .env.
							</p>
							{settings?.env_preview &&
								Object.keys(settings.env_preview).length > 0 && (
									<dl className="mt-2 text-xs text-amber-600 dark:text-amber-400 space-y-1">
										{Object.entries(settings.env_preview).map(([k, v]) => (
											<div key={k}>
												<span className="font-mono">{k}:</span>{" "}
												<span className="font-mono">{v}</span>
											</div>
										))}
									</dl>
								)}
							<button
								type="button"
								onClick={() => importMutation.mutate()}
								disabled={importMutation.isPending}
								className="mt-3 px-4 py-2 text-sm font-medium text-amber-800 dark:text-amber-200 bg-amber-100 dark:bg-amber-900/40 border border-amber-300 dark:border-amber-700 rounded-md hover:bg-amber-200 dark:hover:bg-amber-900/60 disabled:opacity-50 disabled:cursor-not-allowed"
							>
								{importMutation.isPending ? (
									<>
										<Loader2 className="inline h-4 w-4 animate-spin mr-2" />
										Importing...
									</>
								) : (
									"Load from .env"
								)}
							</button>
						</div>
					</div>
				</div>
			)}

			{/* Toggles - all configurable options at top */}
			<div className="bg-secondary-50 dark:bg-secondary-900/50 rounded-lg p-4 border border-secondary-200 dark:border-secondary-700">
				<h3 className="font-medium text-secondary-900 dark:text-white mb-4">
					Configuration
				</h3>
				<div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-5 gap-4">
					{/* Enable OIDC */}
					<ToggleCard
						label="Enable OIDC / SSO"
						description="Allow users to log in with IdP"
						checked={settings?.oidc_enabled ?? false}
						onChange={handleToggleEnabled}
						disabled={updateMutation.isPending}
					/>
					{/* Enforce HTTPS */}
					<ToggleCard
						label="Enforce HTTPS"
						description="Require HTTPS for OIDC (recommended)"
						checked={form.oidc_enforce_https}
						onChange={() =>
							handleFieldChange("oidc_enforce_https", !form.oidc_enforce_https)
						}
						disabled={updateMutation.isPending}
					/>
					{/* Sync roles */}
					<ToggleCard
						label="Sync roles from IdP"
						description="Map IdP groups to PatchMon roles"
						checked={form.oidc_sync_roles}
						onChange={() =>
							handleFieldChange("oidc_sync_roles", !form.oidc_sync_roles)
						}
						disabled={updateMutation.isPending}
					/>
					{/* Disable local auth */}
					<ToggleCard
						label="Disable local auth"
						description="Hide username/password when OIDC enabled"
						checked={form.oidc_disable_local_auth}
						onChange={() =>
							handleFieldChange(
								"oidc_disable_local_auth",
								!form.oidc_disable_local_auth,
							)
						}
						disabled={updateMutation.isPending}
					/>
					{/* Auto-create users */}
					<ToggleCard
						label="Auto-create users"
						description="Create users on first OIDC login"
						checked={form.oidc_auto_create_users}
						onChange={() =>
							handleFieldChange(
								"oidc_auto_create_users",
								!form.oidc_auto_create_users,
							)
						}
						disabled={updateMutation.isPending}
					/>
				</div>
				{!form.oidc_enforce_https && (
					<div className="mt-4 p-3 rounded-md bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800">
						<p className="text-sm font-medium text-amber-800 dark:text-amber-200">
							Development only - Enforce HTTPS is disabled. Only use for local
							testing.
						</p>
					</div>
				)}
			</div>

			{/* OAuth2 Configuration */}
			<div className="bg-white dark:bg-secondary-800 rounded-lg p-4 border border-secondary-200 dark:border-secondary-700">
				<h3 className="font-medium text-secondary-900 dark:text-white mb-4">
					OAuth2 Configuration
				</h3>

				<div className="space-y-4">
					{/* Issuer URL */}
					<div>
						<label
							htmlFor="oidc-issuer-url"
							className="block text-sm font-medium text-secondary-700 dark:text-white mb-1"
						>
							Issuer URL
						</label>
						<input
							id="oidc-issuer-url"
							type="url"
							value={form.oidc_issuer_url}
							onChange={(e) =>
								handleFieldChange("oidc_issuer_url", e.target.value)
							}
							disabled={updateMutation.isPending}
							placeholder="https://your-idp.com/realms/your-realm"
							className="w-full px-3 py-2 bg-white dark:bg-secondary-900 border border-secondary-300 dark:border-secondary-600 rounded-md text-secondary-900 dark:text-white focus:ring-2 focus:ring-primary-500 focus:border-primary-500 placeholder-secondary-400"
						/>
					</div>

					{/* Client ID */}
					<div>
						<label
							htmlFor="oidc-client-id"
							className="block text-sm font-medium text-secondary-700 dark:text-white mb-1"
						>
							Client ID
						</label>
						<input
							id="oidc-client-id"
							type="text"
							value={form.oidc_client_id}
							onChange={(e) =>
								handleFieldChange("oidc_client_id", e.target.value)
							}
							disabled={updateMutation.isPending}
							placeholder="Enter your OIDC Client ID"
							className="w-full px-3 py-2 bg-white dark:bg-secondary-900 border border-secondary-300 dark:border-secondary-600 rounded-md text-secondary-900 dark:text-white focus:ring-2 focus:ring-primary-500 focus:border-primary-500 placeholder-secondary-400"
						/>
					</div>

					{/* Client Secret */}
					<div>
						<label
							htmlFor="oidc-client-secret"
							className="block text-sm font-medium text-secondary-700 dark:text-white mb-1"
						>
							Client Secret
							{settings?.oidc_client_secret_set ? (
								<span className="ml-2 inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">
									<Check className="h-3 w-3 mr-1" />
									Set
								</span>
							) : (
								<span className="ml-2 inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-secondary-100 text-secondary-600 dark:bg-secondary-700 dark:text-white">
									Not set
								</span>
							)}
						</label>
						<div className="flex gap-2">
							<div className="relative flex-1">
								<input
									id="oidc-client-secret"
									type={showSecret ? "text" : "password"}
									value={secretInput}
									onChange={(e) => setSecretInput(e.target.value)}
									disabled={updateMutation.isPending}
									placeholder={
										settings?.oidc_client_secret_set
											? "Enter new secret to replace"
											: "Enter your OIDC Client Secret"
									}
									className="w-full px-3 py-2 pr-10 bg-white dark:bg-secondary-900 border border-secondary-300 dark:border-secondary-600 rounded-md text-secondary-900 dark:text-white focus:ring-2 focus:ring-primary-500 focus:border-primary-500 placeholder-secondary-400"
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
								disabled={!secretInput.trim() || updateMutation.isPending}
								className="px-4 py-2 text-sm font-medium text-white bg-primary-600 rounded-md hover:opacity-90 disabled:opacity-50 disabled:cursor-not-allowed"
							>
								Save
							</button>
							{settings?.oidc_client_secret_set && (
								<button
									type="button"
									onClick={handleClearSecret}
									disabled={updateMutation.isPending}
									className="px-3 py-2 text-sm font-medium text-red-700 dark:text-red-400 border border-red-300 dark:border-red-700 rounded-md hover:bg-red-50 dark:hover:bg-red-900/20"
								>
									<X className="h-4 w-4" />
								</button>
							)}
						</div>
					</div>

					{/* Callback URL */}
					<div>
						<label className="block text-sm font-medium text-secondary-700 dark:text-white mb-1">
							Callback URL
						</label>
						<div className="px-3 py-2 bg-secondary-50 dark:bg-secondary-900 border border-secondary-200 dark:border-secondary-600 rounded-md">
							<code className="text-sm text-secondary-700 dark:text-white break-all">
								{settings?.callback_url || " -"}
							</code>
						</div>
						<p className="mt-1 text-xs text-secondary-500 dark:text-white">
							Add this URL to your IdP&apos;s allowed redirect URIs
						</p>
					</div>

					{/* Redirect URI (optional override) */}
					<div>
						<label
							htmlFor="oidc-redirect-uri"
							className="block text-sm font-medium text-secondary-700 dark:text-white mb-1"
						>
							Redirect URI (optional override)
						</label>
						<input
							id="oidc-redirect-uri"
							type="url"
							value={form.oidc_redirect_uri}
							onChange={(e) =>
								handleFieldChange("oidc_redirect_uri", e.target.value)
							}
							disabled={updateMutation.isPending}
							placeholder="Leave empty to use callback URL above"
							className="w-full px-3 py-2 bg-white dark:bg-secondary-900 border border-secondary-300 dark:border-secondary-600 rounded-md text-secondary-900 dark:text-white focus:ring-2 focus:ring-primary-500 focus:border-primary-500 placeholder-secondary-400"
						/>
					</div>

					{/* Scopes */}
					<div>
						<label
							htmlFor="oidc-scopes"
							className="block text-sm font-medium text-secondary-700 dark:text-white mb-1"
						>
							Scopes
						</label>
						<input
							id="oidc-scopes"
							type="text"
							value={form.oidc_scopes}
							onChange={(e) => handleFieldChange("oidc_scopes", e.target.value)}
							disabled={updateMutation.isPending}
							placeholder="openid email profile groups"
							className="w-full px-3 py-2 bg-white dark:bg-secondary-900 border border-secondary-300 dark:border-secondary-600 rounded-md text-secondary-900 dark:text-white focus:ring-2 focus:ring-primary-500 focus:border-primary-500 placeholder-secondary-400"
						/>
					</div>

					{/* Button Text */}
					<div>
						<label
							htmlFor="oidc-button-text"
							className="block text-sm font-medium text-secondary-700 dark:text-white mb-1"
						>
							Button Text
						</label>
						<input
							id="oidc-button-text"
							type="text"
							value={form.oidc_button_text}
							onChange={(e) =>
								handleFieldChange("oidc_button_text", e.target.value)
							}
							disabled={updateMutation.isPending}
							placeholder="Login with SSO"
							className="w-full px-3 py-2 bg-white dark:bg-secondary-900 border border-secondary-300 dark:border-secondary-600 rounded-md text-secondary-900 dark:text-white focus:ring-2 focus:ring-primary-500 focus:border-primary-500 placeholder-secondary-400"
						/>
					</div>

					{/* Apply Button */}
					<div className="pt-2">
						<button
							type="button"
							onClick={handleApply}
							disabled={updateMutation.isPending}
							className="px-4 py-2 text-sm font-medium text-white bg-primary-600 rounded-md hover:opacity-90 disabled:opacity-50 disabled:cursor-not-allowed"
						>
							{updateMutation.isPending ? (
								<>
									<Loader2 className="inline h-4 w-4 animate-spin mr-2" />
									Applying...
								</>
							) : (
								"Apply"
							)}
						</button>
					</div>
				</div>
			</div>

			{/* Role Mapping */}
			<div className="bg-white dark:bg-secondary-800 rounded-lg border border-secondary-200 dark:border-secondary-700">
				<button
					type="button"
					onClick={() => setShowRoleMapping(!showRoleMapping)}
					className="w-full flex items-center justify-between p-4 text-left"
				>
					<div className="flex items-center gap-2">
						<KeyRound className="h-5 w-5 text-primary-500" />
						<h3 className="font-medium text-secondary-900 dark:text-white">
							Role Mapping
						</h3>
					</div>
					{showRoleMapping ? (
						<ChevronUp className="h-5 w-5 text-secondary-400" />
					) : (
						<ChevronDown className="h-5 w-5 text-secondary-400" />
					)}
				</button>

				{showRoleMapping && (
					<div className="px-4 pb-4 border-t border-secondary-200 dark:border-secondary-700 pt-4">
						<p className="text-sm text-secondary-500 dark:text-secondary-400 mb-4">
							Map IdP group names to PatchMon roles. Users are assigned the
							highest matching role. Default role is used when no group matches.
						</p>
						<div className="overflow-x-auto rounded-lg border border-secondary-200 dark:border-secondary-600">
							<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-600">
								<thead className="bg-secondary-50 dark:bg-secondary-700">
									<tr>
										<th className="px-4 py-3 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
											PatchMon Role
										</th>
										<th className="px-4 py-3 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
											OIDC Mapped Role (IdP Group Name)
										</th>
									</tr>
								</thead>
								<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-600">
									<tr className="hover:bg-secondary-50 dark:hover:bg-secondary-700/50">
										<td className="px-4 py-3 text-sm font-medium text-secondary-900 dark:text-white">
											Default (fallback)
										</td>
										<td className="px-4 py-3">
											<input
												id="oidc-default-role"
												type="text"
												value={form.oidc_default_role}
												onChange={(e) =>
													handleFieldChange("oidc_default_role", e.target.value)
												}
												disabled={updateMutation.isPending}
												placeholder={ROLE_MAPPING_DEFAULTS.oidc_default_role}
												className="w-full max-w-xs px-3 py-2 text-sm bg-white dark:bg-secondary-900 border border-secondary-300 dark:border-secondary-600 rounded-md text-secondary-900 dark:text-white focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
											/>
											<p className="text-xs text-secondary-500 mt-1">
												Role when no IdP group matches
											</p>
										</td>
									</tr>
									{sortedRoles.map((r) => {
										const fieldName = ROLE_TO_OIDC_FIELD[r.role];
										const isConfigurable = !!fieldName;
										return (
											<tr
												key={r.role}
												className="hover:bg-secondary-50 dark:hover:bg-secondary-700/50"
											>
												<td className="px-4 py-3 text-sm font-medium text-secondary-900 dark:text-white">
													{r.role.replace(/_/g, " ")}
												</td>
												<td className="px-4 py-3">
													{isConfigurable ? (
														<input
															id={`oidc-${r.role}-group`}
															type="text"
															value={form[fieldName] || ""}
															onChange={(e) =>
																handleFieldChange(fieldName, e.target.value)
															}
															disabled={updateMutation.isPending}
															placeholder={
																ROLE_MAPPING_DEFAULTS[fieldName] || r.role
															}
															className="w-full max-w-xs px-3 py-2 text-sm bg-white dark:bg-secondary-900 border border-secondary-300 dark:border-secondary-600 rounded-md text-secondary-900 dark:text-white focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
														/>
													) : (
														<span className="text-sm text-secondary-400 dark:text-secondary-500 italic">
															- Not configurable via OIDC
														</span>
													)}
												</td>
											</tr>
										);
									})}
								</tbody>
							</table>
						</div>
					</div>
				)}
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
						<ol className="space-y-3 text-sm text-secondary-700 dark:text-white">
							<li className="flex gap-3">
								<span className="flex-shrink-0 flex items-center justify-center h-6 w-6 rounded-full text-xs font-medium text-white bg-primary-600">
									1
								</span>
								<span>
									Configure your IdP (Keycloak, Okta, Auth0, etc.) with a new
									OAuth2 client
								</span>
							</li>
							<li className="flex gap-3">
								<span className="flex-shrink-0 flex items-center justify-center h-6 w-6 rounded-full text-xs font-medium text-white bg-primary-600">
									2
								</span>
								<span>
									Add the callback URL to allowed redirect URIs:{" "}
									<code className="bg-secondary-100 dark:bg-secondary-700 px-1 rounded">
										{settings?.callback_url ||
											"{server_url}/api/v1/auth/oidc/callback"}
									</code>
								</span>
							</li>
							<li className="flex gap-3">
								<span className="flex-shrink-0 flex items-center justify-center h-6 w-6 rounded-full text-xs font-medium text-white bg-primary-600">
									3
								</span>
								<span>
									Request scopes:{" "}
									<code className="bg-secondary-100 dark:bg-secondary-700 px-1 rounded">
										openid email profile groups
									</code>{" "}
									(or equivalent for your IdP)
								</span>
							</li>
							<li className="flex gap-3">
								<span className="flex-shrink-0 flex items-center justify-center h-6 w-6 rounded-full text-xs font-medium text-white bg-primary-600">
									4
								</span>
								<span>
									<strong>Authentik users:</strong> Create a Scope Mapping to
									add groups to the token. Go to Customization → Property
									Mappings → Create → Scope Mapping. Use scope
									&quot;profile&quot; and expression:{" "}
									<code className="bg-secondary-100 dark:bg-secondary-700 px-1 rounded block mt-1">
										{`return {"groups": [str(g.name) for g in request.user.ak_groups.all()]}`}
									</code>
									Assign this mapping to your OAuth2 provider.
								</span>
							</li>
							<li className="flex gap-3">
								<span className="flex-shrink-0 flex items-center justify-center h-6 w-6 rounded-full text-xs font-medium text-white bg-primary-600">
									5
								</span>
								<span>
									Copy Issuer URL, Client ID, and Client Secret into the fields
									above
								</span>
							</li>
						</ol>
					</div>
				)}
			</div>
		</div>
	);
};

export default OidcSettings;
