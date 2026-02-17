import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
	AlertCircle,
	BookOpen,
	CheckCircle,
	Code,
	Save,
	Server,
} from "lucide-react";
import { useEffect, useId, useState } from "react";
import { settingsAPI } from "../../utils/api";

const ProtocolUrlTab = () => {
	const protocolId = useId();
	const hostId = useId();
	const portId = useId();
	const [formData, setFormData] = useState({
		serverProtocol: "http",
		serverHost: "localhost",
		serverPort: 3001,
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

	// Fetch environment config
	const { data: envConfig } = useQuery({
		queryKey: ["envConfig"],
		queryFn: () => settingsAPI.getEnvConfig().then((res) => res.data),
		staleTime: 0, // Always fetch fresh data
	});

	// Update form data when settings are loaded
	useEffect(() => {
		if (settings) {
			const newFormData = {
				serverProtocol: settings.server_protocol || "http",
				serverHost: settings.server_host || "localhost",
				serverPort: settings.server_port || 3001,
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
				});
			}
			if (data?.settings) {
				queryClient.setQueryData(["settings"], data.settings);
			}
			queryClient.invalidateQueries(["settings"]);
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
				<div className="flex items-center mb-6">
					<Server className="h-6 w-6 text-primary-600 mr-3" />
					<h2 className="text-xl font-semibold text-secondary-900 dark:text-white">
						Server Configuration
					</h2>
				</div>

				<div className="grid grid-cols-1 md:grid-cols-3 gap-6">
					<div>
						<label
							htmlFor={protocolId}
							className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-2"
						>
							Protocol
						</label>
						<select
							id={protocolId}
							value={formData.serverProtocol}
							onChange={(e) =>
								handleInputChange("serverProtocol", e.target.value)
							}
							className="w-full border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white"
						>
							<option value="http">HTTP</option>
							<option value="https">HTTPS</option>
						</select>
					</div>

					<div>
						<label
							htmlFor={hostId}
							className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-2"
						>
							Host *
						</label>
						<input
							id={hostId}
							type="text"
							value={formData.serverHost}
							onChange={(e) => handleInputChange("serverHost", e.target.value)}
							className={`w-full border rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white ${
								errors.serverHost
									? "border-red-300 dark:border-red-500"
									: "border-secondary-300 dark:border-secondary-600"
							}`}
							placeholder="example.com"
						/>
						{errors.serverHost && (
							<p className="mt-1 text-sm text-red-600 dark:text-red-400">
								{errors.serverHost}
							</p>
						)}
					</div>

					<div>
						<label
							htmlFor={portId}
							className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-2"
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
							className={`w-full border rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white ${
								errors.serverPort
									? "border-red-300 dark:border-red-500"
									: "border-secondary-300 dark:border-secondary-600"
							}`}
							min="1"
							max="65535"
						/>
						{errors.serverPort && (
							<p className="mt-1 text-sm text-red-600 dark:text-red-400">
								{errors.serverPort}
							</p>
						)}
					</div>
				</div>

				<div className="mt-4 p-3 bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-700 rounded-md">
					<p className="text-sm text-blue-800 dark:text-blue-200">
						<strong>Note:</strong> This URL will be used in installation scripts
						and agent communications. Change this in order for the agents to
						communicate with PatchMon, usually the "outside" port and
						CORS_Origin url.
					</p>
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

				{/* Environment Variables Display */}
				<div className="mt-6 pt-6 border-t border-secondary-200 dark:border-secondary-600">
					<div className="flex items-center mb-4">
						<Code className="h-5 w-5 text-primary-600 mr-2" />
						<h3 className="text-lg font-semibold text-secondary-900 dark:text-white">
							Environment Configuration
						</h3>
					</div>
					<p className="text-sm text-secondary-500 dark:text-secondary-300 mb-4">
						Current configuration values and their sources. Values from the
						database take precedence and are edited using the form above.
					</p>

					{envConfig ? (
						<div className="overflow-x-auto">
							<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-700">
								<thead className="bg-secondary-50 dark:bg-secondary-800">
									<tr>
										<th
											scope="col"
											className="px-4 py-3 text-left text-xs font-medium text-secondary-500 dark:text-secondary-400 uppercase tracking-wider"
										>
											Setting Name
										</th>
										<th
											scope="col"
											className="px-4 py-3 text-left text-xs font-medium text-secondary-500 dark:text-secondary-400 uppercase tracking-wider"
										>
											Current Value
										</th>
										<th
											scope="col"
											className="px-4 py-3 text-left text-xs font-medium text-secondary-500 dark:text-secondary-400 uppercase tracking-wider"
										>
											Source
										</th>
									</tr>
								</thead>
								<tbody className="bg-white dark:bg-secondary-900 divide-y divide-secondary-200 dark:divide-secondary-700">
									{/* Active Server URL from Database */}
									<tr className="bg-secondary-50 dark:bg-secondary-800/50">
										<td
											colSpan="3"
											className="px-4 py-2 text-xs font-semibold text-secondary-700 dark:text-secondary-300 uppercase"
										>
											Server URL (Active Configuration)
										</td>
									</tr>
									<tr>
										<td className="px-4 py-3 text-sm font-medium text-secondary-900 dark:text-white">
											Server Protocol
										</td>
										<td className="px-4 py-3 text-sm text-secondary-700 dark:text-secondary-300 font-mono">
											{envConfig.backend.DB_SERVER_PROTOCOL}
										</td>
										<td className="px-4 py-3 text-sm text-secondary-500 dark:text-secondary-400">
											Database
										</td>
									</tr>
									<tr className="bg-secondary-50 dark:bg-secondary-800/30">
										<td className="px-4 py-3 text-sm font-medium text-secondary-900 dark:text-white">
											Server Host
										</td>
										<td className="px-4 py-3 text-sm text-secondary-700 dark:text-secondary-300 font-mono break-all">
											{envConfig.backend.DB_SERVER_HOST}
										</td>
										<td className="px-4 py-3 text-sm text-secondary-500 dark:text-secondary-400">
											Database
										</td>
									</tr>
									<tr>
										<td className="px-4 py-3 text-sm font-medium text-secondary-900 dark:text-white">
											Server Port
										</td>
										<td className="px-4 py-3 text-sm text-secondary-700 dark:text-secondary-300 font-mono">
											{envConfig.backend.DB_SERVER_PORT}
										</td>
										<td className="px-4 py-3 text-sm text-secondary-500 dark:text-secondary-400">
											Database
										</td>
									</tr>
									<tr className="bg-secondary-50 dark:bg-secondary-800/30">
										<td className="px-4 py-3 text-sm font-medium text-secondary-900 dark:text-white">
											Full Server URL
										</td>
										<td className="px-4 py-3 text-sm text-secondary-700 dark:text-secondary-300 font-mono break-all">
											{envConfig.backend.DB_SERVER_URL}
										</td>
										<td className="px-4 py-3 text-sm text-secondary-500 dark:text-secondary-400">
											Database (Computed)
										</td>
									</tr>

									{/* Backend .env Configuration */}
									<tr className="bg-secondary-50 dark:bg-secondary-800/50">
										<td
											colSpan="3"
											className="px-4 py-2 text-xs font-semibold text-secondary-700 dark:text-secondary-300 uppercase"
										>
											Backend Configuration (backend/.env)
										</td>
									</tr>
									<tr>
										<td className="px-4 py-3 text-sm font-medium text-secondary-900 dark:text-white">
											CORS_ORIGIN
										</td>
										<td className="px-4 py-3 text-sm text-secondary-700 dark:text-secondary-300 font-mono break-all">
											{envConfig.backend.CORS_ORIGIN}
										</td>
										<td className="px-4 py-3 text-sm text-secondary-500 dark:text-secondary-400">
											backend/.env
										</td>
									</tr>
									<tr className="bg-secondary-50 dark:bg-secondary-800/30">
										<td className="px-4 py-3 text-sm font-medium text-secondary-900 dark:text-white">
											CORS_ORIGINS
										</td>
										<td className="px-4 py-3 text-sm text-secondary-700 dark:text-secondary-300 font-mono break-all">
											{envConfig.backend.CORS_ORIGINS}
										</td>
										<td className="px-4 py-3 text-sm text-secondary-500 dark:text-secondary-400">
											backend/.env
										</td>
									</tr>
									<tr>
										<td className="px-4 py-3 text-sm font-medium text-secondary-900 dark:text-white">
											PORT
										</td>
										<td className="px-4 py-3 text-sm text-secondary-700 dark:text-secondary-300 font-mono">
											{envConfig.backend.PORT}
										</td>
										<td className="px-4 py-3 text-sm text-secondary-500 dark:text-secondary-400">
											backend/.env
										</td>
									</tr>
									<tr className="bg-secondary-50 dark:bg-secondary-800/30">
										<td className="px-4 py-3 text-sm font-medium text-secondary-900 dark:text-white">
											NODE_ENV
										</td>
										<td className="px-4 py-3 text-sm text-secondary-700 dark:text-secondary-300 font-mono">
											{envConfig.backend.NODE_ENV}
										</td>
										<td className="px-4 py-3 text-sm text-secondary-500 dark:text-secondary-400">
											backend/.env
										</td>
									</tr>

									{/* Frontend .env Configuration */}
									<tr className="bg-secondary-50 dark:bg-secondary-800/50">
										<td
											colSpan="3"
											className="px-4 py-2 text-xs font-semibold text-secondary-700 dark:text-secondary-300 uppercase"
										>
											Frontend Configuration (frontend/.env)
										</td>
									</tr>
									<tr>
										<td className="px-4 py-3 text-sm font-medium text-secondary-900 dark:text-white">
											VITE_API_URL
										</td>
										<td className="px-4 py-3 text-sm text-secondary-700 dark:text-secondary-300 font-mono break-all">
											{envConfig.frontend.VITE_API_URL}
										</td>
										<td className="px-4 py-3 text-sm text-secondary-500 dark:text-secondary-400">
											frontend/.env
										</td>
									</tr>

									{/* Environment Variable Fallbacks (Optional) */}
									<tr className="bg-secondary-50 dark:bg-secondary-800/50">
										<td
											colSpan="3"
											className="px-4 py-2 text-xs font-semibold text-secondary-700 dark:text-secondary-300 uppercase"
										>
											Environment Variable Fallbacks (backend/.env - Only used
											on first DB init)
										</td>
									</tr>
									<tr>
										<td className="px-4 py-3 text-sm font-medium text-secondary-900 dark:text-white">
											SERVER_PROTOCOL
										</td>
										<td className="px-4 py-3 text-sm text-secondary-700 dark:text-secondary-300 font-mono">
											{envConfig.backend.ENV_SERVER_PROTOCOL}
										</td>
										<td className="px-4 py-3 text-sm text-secondary-500 dark:text-secondary-400">
											backend/.env (fallback)
										</td>
									</tr>
									<tr className="bg-secondary-50 dark:bg-secondary-800/30">
										<td className="px-4 py-3 text-sm font-medium text-secondary-900 dark:text-white">
											SERVER_HOST
										</td>
										<td className="px-4 py-3 text-sm text-secondary-700 dark:text-secondary-300 font-mono break-all">
											{envConfig.backend.ENV_SERVER_HOST}
										</td>
										<td className="px-4 py-3 text-sm text-secondary-500 dark:text-secondary-400">
											backend/.env (fallback)
										</td>
									</tr>
									<tr>
										<td className="px-4 py-3 text-sm font-medium text-secondary-900 dark:text-white">
											SERVER_PORT
										</td>
										<td className="px-4 py-3 text-sm text-secondary-700 dark:text-secondary-300 font-mono">
											{envConfig.backend.ENV_SERVER_PORT}
										</td>
										<td className="px-4 py-3 text-sm text-secondary-500 dark:text-secondary-400">
											backend/.env (fallback)
										</td>
									</tr>
								</tbody>
							</table>
						</div>
					) : (
						<div className="bg-secondary-50 dark:bg-secondary-800 rounded-lg p-4 border border-secondary-200 dark:border-secondary-600">
							<p className="text-sm text-secondary-500 dark:text-secondary-400">
								Loading environment configuration...
							</p>
						</div>
					)}

					<div className="mt-4 flex justify-end">
						<a
							href="https://docs.patchmon.net/books/patchmon-application-documentation/page/patchmon-environment-variables-reference"
							target="_blank"
							rel="noopener noreferrer"
							className="inline-flex items-center gap-2 px-3 py-2 bg-primary-600 hover:bg-primary-700 dark:bg-primary-500 dark:hover:bg-primary-600 text-white rounded-lg text-sm transition-colors"
						>
							<BookOpen className="h-4 w-4" />
							Environment Variables Documentation
						</a>
					</div>
				</div>
			</form>
		</div>
	);
};

export default ProtocolUrlTab;
