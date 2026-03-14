import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertCircle, CheckCircle, Save, Server } from "lucide-react";
import { useEffect, useId, useState } from "react";
import { settingsAPI } from "../../utils/api";
import { FORM_INPUT_CLASS } from "../FormInput";

const ProtocolUrlTab = () => {
	const protocolId = useId();
	const hostId = useId();
	const portId = useId();
	const ignoreSslId = useId();
	const [formData, setFormData] = useState({
		serverProtocol: "http",
		serverHost: "localhost",
		serverPort: 3001,
		ignoreSslSelfSigned: false,
	});
	const [errors, setErrors] = useState({});
	const [isDirty, setIsDirty] = useState(false);

	const queryClient = useQueryClient();

	// Fetch current settings
	const {
		data: settings,
		isLoading,
		error,
		refetch: _refetchSettings,
	} = useQuery({
		queryKey: ["settings"],
		queryFn: () => settingsAPI.get().then((res) => res.data),
		staleTime: 0, // Always fetch fresh data
	});

	// Update form data when settings are loaded
	useEffect(() => {
		if (settings) {
			const newFormData = {
				serverProtocol: settings.server_protocol || "http",
				serverHost: settings.server_host || "localhost",
				serverPort: settings.server_port || 3001,
				ignoreSslSelfSigned: settings.ignore_ssl_self_signed === true,
			};
			setFormData(newFormData);
			setIsDirty(false);
		}
	}, [settings]);

	// Update settings mutation
	const updateSettingsMutation = useMutation({
		mutationFn: (data) => {
			return settingsAPI.update(data).then((res) => res.data);
		},
		onSuccess: (data) => {
			// Apply saved settings to form immediately so UI shows persisted values
			// (avoids race with refetch and ensures form is not overwritten by stale cache)
			if (data?.settings) {
				const s = data.settings;
				setFormData({
					serverProtocol: s.server_protocol || "http",
					serverHost: s.server_host || "localhost",
					serverPort: s.server_port ?? 3001,
					ignoreSslSelfSigned: s.ignore_ssl_self_signed === true,
				});
			}
			if (data?.settings) {
				queryClient.setQueryData(["settings"], data.settings);
			}
			queryClient.invalidateQueries(["settings"]);
			queryClient.invalidateQueries(["serverUrl"]);
			setIsDirty(false);
			setErrors({});
		},
		onError: (error) => {
			if (error.response?.data?.errors) {
				setErrors(
					error.response.data.errors.reduce((acc, err) => {
						acc[err.path] = err.msg;
						return acc;
					}, {}),
				);
			} else {
				setErrors({
					general: error.response?.data?.error || "Failed to update settings",
				});
			}
		},
	});

	const handleInputChange = (field, value) => {
		setFormData((prev) => ({
			...prev,
			[field]: value,
		}));
		setIsDirty(true);
		if (errors[field]) {
			setErrors((prev) => ({ ...prev, [field]: null }));
		}
	};

	const validateForm = () => {
		const newErrors = {};

		if (!formData.serverHost.trim()) {
			newErrors.serverHost = "Server host is required";
		}

		if (
			!formData.serverPort ||
			formData.serverPort < 1 ||
			formData.serverPort > 65535
		) {
			newErrors.serverPort = "Port must be between 1 and 65535";
		}

		setErrors(newErrors);
		return Object.keys(newErrors).length === 0;
	};

	const handleSave = () => {
		if (validateForm()) {
			updateSettingsMutation.mutate(formData);
		}
	};

	if (isLoading) {
		return (
			<div className="flex items-center justify-center h-64">
				<div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600"></div>
			</div>
		);
	}

	if (error) {
		return (
			<div className="bg-red-50 dark:bg-red-900 border border-red-200 dark:border-red-700 rounded-md p-4">
				<div className="flex">
					<AlertCircle className="h-5 w-5 text-red-400 dark:text-red-300" />
					<div className="ml-3">
						<h3 className="text-sm font-medium text-red-800 dark:text-red-200">
							Error loading settings
						</h3>
						<p className="mt-1 text-sm text-red-700 dark:text-red-300">
							{error.response?.data?.error || "Failed to load settings"}
						</p>
					</div>
				</div>
			</div>
		);
	}

	return (
		<div className="space-y-6">
			{errors.general && (
				<div className="bg-red-50 dark:bg-red-900 border border-red-200 dark:border-red-700 rounded-md p-4">
					<div className="flex">
						<AlertCircle className="h-5 w-5 text-red-400 dark:text-red-300" />
						<div className="ml-3">
							<p className="text-sm text-red-700 dark:text-red-300">
								{errors.general}
							</p>
						</div>
					</div>
				</div>
			)}

			<form className="space-y-6">
				<div className="bg-white dark:bg-secondary-800 rounded-lg p-4 border border-secondary-200 dark:border-secondary-600">
					<div className="flex items-center mb-2">
						<Server className="h-6 w-6 text-primary-600 mr-3" />
						<h2 className="text-xl font-semibold text-secondary-900 dark:text-white">
							Server Configuration
						</h2>
					</div>
					<p className="text-sm text-secondary-600 dark:text-secondary-400 mb-1">
						Agent communication URL
					</p>
					<p className="text-sm text-secondary-500 dark:text-secondary-500 mb-4">
						This is the URL agents use to connect. If you change your access
						URL, update this in Settings &gt; Server URL.
					</p>

					<div className="grid grid-cols-1 md:grid-cols-3 gap-6">
						<div>
							<label
								htmlFor={protocolId}
								className="block text-sm font-medium text-secondary-700 dark:text-white mb-1"
							>
								Protocol
							</label>
							<select
								id={protocolId}
								value={formData.serverProtocol}
								onChange={(e) =>
									handleInputChange("serverProtocol", e.target.value)
								}
								className={FORM_INPUT_CLASS}
							>
								<option value="http">HTTP</option>
								<option value="https">HTTPS</option>
							</select>
						</div>

						<div>
							<label
								htmlFor={hostId}
								className="block text-sm font-medium text-secondary-700 dark:text-white mb-1"
							>
								Host *
							</label>
							<input
								id={hostId}
								type="text"
								value={formData.serverHost}
								onChange={(e) =>
									handleInputChange("serverHost", e.target.value)
								}
								className={`${FORM_INPUT_CLASS} ${
									errors.serverHost
										? "border-danger-500 dark:border-danger-400"
										: ""
								}`}
								placeholder="example.com"
							/>
							{errors.serverHost && (
								<p className="mt-1 text-sm text-danger-600 dark:text-danger-400">
									{errors.serverHost}
								</p>
							)}
						</div>

						<div>
							<label
								htmlFor={portId}
								className="block text-sm font-medium text-secondary-700 dark:text-white mb-1"
							>
								Port *
							</label>
							<input
								id={portId}
								type="number"
								value={formData.serverPort}
								onChange={(e) =>
									handleInputChange("serverPort", parseInt(e.target.value, 10))
								}
								className={`${FORM_INPUT_CLASS} ${
									errors.serverPort
										? "border-danger-500 dark:border-danger-400"
										: ""
								}`}
								min="1"
								max="65535"
							/>
							{errors.serverPort && (
								<p className="mt-1 text-sm text-danger-600 dark:text-danger-400">
									{errors.serverPort}
								</p>
							)}
						</div>
					</div>

					<div className="mt-4 p-3 bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-700 rounded-md">
						<p className="text-sm text-blue-800 dark:text-blue-200">
							<strong>Note:</strong> This URL will be used in installation
							scripts and agent communications. Change this in order for the
							agents to communicate with PatchMon, usually the
							&quot;outside&quot; port and CORS_Origin url.
						</p>
					</div>

					{/* Ignore SSL Self-Signed Certificates */}
					<div className="mt-6 flex items-start justify-between gap-4 p-4 bg-warning-50 dark:bg-warning-900/20 rounded-lg border border-warning-200 dark:border-warning-700">
						<div className="flex-1">
							<div className="flex items-center gap-2">
								<label
									htmlFor={ignoreSslId}
									className="text-sm font-medium text-secondary-900 dark:text-secondary-100"
								>
									Ignore SSL Self-Signed Certificates
								</label>
								<span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-warning-100 text-warning-800 dark:bg-warning-900/50 dark:text-warning-300">
									Security
								</span>
							</div>
							<p className="mt-1 text-sm text-secondary-500 dark:text-white">
								When enabled, curl commands in agent scripts will use the -k
								flag to ignore SSL certificate validation errors. Use with
								caution on production systems as this reduces security.
							</p>
						</div>
						<button
							type="button"
							id={ignoreSslId}
							onClick={() =>
								handleInputChange(
									"ignoreSslSelfSigned",
									!formData.ignoreSslSelfSigned,
								)
							}
							className={`relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-md border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 ${
								formData.ignoreSslSelfSigned
									? "bg-warning-500 dark:bg-warning-600"
									: "bg-secondary-200 dark:bg-secondary-600"
							}`}
						>
							<span
								className={`pointer-events-none inline-block h-5 w-5 transform rounded-md bg-white shadow ring-0 transition duration-200 ease-in-out ${
									formData.ignoreSslSelfSigned
										? "translate-x-5"
										: "translate-x-0"
								}`}
							/>
						</button>
					</div>
				</div>

				{/* Save Button */}
				<div className="flex justify-end mt-4">
					<button
						type="button"
						onClick={handleSave}
						disabled={!isDirty || updateSettingsMutation.isPending}
						className={`inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md shadow-sm text-white ${
							!isDirty || updateSettingsMutation.isPending
								? "bg-secondary-400 cursor-not-allowed"
								: "bg-primary-600 hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500"
						}`}
					>
						{updateSettingsMutation.isPending ? (
							<>
								<div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white mr-2"></div>
								Saving...
							</>
						) : (
							<>
								<Save className="h-4 w-4 mr-2" />
								Save Settings
							</>
						)}
					</button>
				</div>

				{updateSettingsMutation.isSuccess && (
					<div className="mt-4 bg-green-50 dark:bg-green-900 border border-green-200 dark:border-green-700 rounded-md p-4">
						<div className="flex">
							<CheckCircle className="h-5 w-5 text-green-400 dark:text-green-300" />
							<div className="ml-3">
								<p className="text-sm text-green-700 dark:text-green-300">
									Settings saved successfully!
								</p>
							</div>
						</div>
					</div>
				)}
			</form>
		</div>
	);
};

export default ProtocolUrlTab;
