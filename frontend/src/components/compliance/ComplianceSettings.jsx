import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronDown, Clock, Info, Save, Shield } from "lucide-react";
import { useEffect, useId, useState } from "react";
import { useToast } from "../../contexts/ToastContext";
import { settingsAPI } from "../../utils/api";
import { complianceAPI } from "../../utils/complianceApi";

const SUPPORTED_PROFILES = [
	{ os: "Ubuntu", profiles: "CIS Level 1 Server, CIS Level 2 Server" },
	{ os: "Debian", profiles: "CIS Level 1 Server, CIS Level 2 Server" },
	{ os: "RHEL", profiles: "CIS Level 1 Server, CIS Level 2 Server" },
	{ os: "CentOS", profiles: "CIS Level 1 Server, CIS Level 2 Server" },
	{ os: "Rocky Linux", profiles: "CIS Level 1 Server, CIS Level 2 Server" },
	{ os: "AlmaLinux", profiles: "CIS Level 1 Server, CIS Level 2 Server" },
	{ os: "Fedora", profiles: "CIS Level 1 Server, CIS Level 2 Server" },
	{ os: "SLES", profiles: "CIS Level 1 Server, CIS Level 2 Server" },
	{ os: "OpenSUSE", profiles: "CIS Level 1 Server, CIS Level 2 Server" },
];

const ComplianceSettings = () => {
	const queryClient = useQueryClient();
	const toast = useToast();
	const defaultComplianceModeId = useId();
	const [formData, setFormData] = useState({
		defaultComplianceMode: "on-demand",
		complianceScanInterval: 1440,
	});
	const [isDirty, setIsDirty] = useState(false);
	const [showProfiles, setShowProfiles] = useState(false);

	const { data: settings } = useQuery({
		queryKey: ["settings"],
		queryFn: () => settingsAPI.get().then((res) => res.data),
	});

	const { data: serverSSGInfo } = useQuery({
		queryKey: ["server-ssg-info"],
		queryFn: () => complianceAPI.getSSGInfo(),
		staleTime: 30 * 60 * 1000,
	});

	useEffect(() => {
		if (settings) {
			setFormData({
				defaultComplianceMode: settings.default_compliance_mode || "on-demand",
				complianceScanInterval: settings.compliance_scan_interval || 1440,
			});
			setIsDirty(false);
		}
	}, [settings]);

	const updateMutation = useMutation({
		mutationFn: (data) => settingsAPI.update(data).then((res) => res.data),
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ["settings"] });
			setIsDirty(false);
			toast.success("Compliance settings saved");
		},
		onError: (error) => {
			toast.error(
				error.response?.data?.error || "Failed to update compliance settings",
			);
		},
	});

	const handleInputChange = (field, value) => {
		setFormData((prev) => ({ ...prev, [field]: value }));
		setIsDirty(true);
	};

	const handleSave = () => {
		updateMutation.mutate({
			default_compliance_mode: formData.defaultComplianceMode,
			compliance_scan_interval: formData.complianceScanInterval,
		});
	};

	const formatInterval = (mins) => {
		if (mins < 1440) return `${Math.round(mins / 60)}h`;
		if (mins < 10080) return `${Math.round(mins / 1440)}d`;
		return "7d";
	};

	return (
		<div className="space-y-6">
			{/* Two-column: Mode + Interval side by side */}
			<div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
				{/* Default Compliance Mode */}
				<div className="card p-4 md:p-5">
					<div className="flex items-center gap-2 mb-1">
						<Shield className="h-4 w-4 text-primary-600 dark:text-primary-400" />
						<h3 className="text-sm font-semibold text-secondary-900 dark:text-white">
							Default Compliance Mode
						</h3>
					</div>
					<p className="text-xs text-secondary-500 dark:text-secondary-300 mb-3">
						Applies to newly registered hosts. Per-host overrides available in
						host detail.
					</p>
					<div className="flex flex-col gap-2">
						{[
							{
								value: "disabled",
								label: "Disabled",
								desc: "Scanning off for new hosts",
							},
							{
								value: "on-demand",
								label: "On-Demand",
								desc: "Manual trigger only",
							},
							{
								value: "enabled",
								label: "Enabled",
								desc: "Runs on schedule automatically",
							},
						].map((opt) => (
							<label
								key={opt.value}
								className={`flex items-center gap-3 p-2.5 border rounded-lg cursor-pointer transition-colors ${
									formData.defaultComplianceMode === opt.value
										? "border-primary-500 bg-primary-50 dark:bg-primary-900/20"
										: "border-secondary-200 dark:border-secondary-600 hover:border-secondary-300 dark:hover:border-secondary-500"
								}`}
							>
								<input
									type="radio"
									name="defaultComplianceMode"
									id={`${defaultComplianceModeId}-${opt.value}`}
									value={opt.value}
									checked={formData.defaultComplianceMode === opt.value}
									onChange={() =>
										handleInputChange("defaultComplianceMode", opt.value)
									}
									className="h-4 w-4 text-primary-600 border-secondary-300 focus:ring-primary-500"
								/>
								<div>
									<span className="text-sm font-medium text-secondary-900 dark:text-white">
										{opt.label}
									</span>
									<span className="ml-2 text-xs text-secondary-500 dark:text-secondary-400">
										{opt.desc}
									</span>
								</div>
							</label>
						))}
					</div>
				</div>

				{/* Scan Interval */}
				<div className="card p-4 md:p-5">
					<div className="flex items-center gap-2 mb-1">
						<Clock className="h-4 w-4 text-primary-600 dark:text-primary-400" />
						<h3 className="text-sm font-semibold text-secondary-900 dark:text-white">
							Scan Interval
						</h3>
					</div>
					<p className="text-xs text-secondary-500 dark:text-secondary-300 mb-3">
						Frequency when mode is "Enabled". Pushed to all agents on save.
					</p>
					<div className="flex flex-wrap items-center gap-2">
						{[
							{ label: "6h", value: 360 },
							{ label: "12h", value: 720 },
							{ label: "24h", value: 1440 },
							{ label: "48h", value: 2880 },
							{ label: "3d", value: 4320 },
							{ label: "7d", value: 10080 },
						].map((preset) => (
							<button
								key={preset.value}
								type="button"
								onClick={() =>
									handleInputChange("complianceScanInterval", preset.value)
								}
								className={`px-3 py-1.5 text-xs font-medium rounded-md border transition-colors ${
									formData.complianceScanInterval === preset.value
										? "bg-primary-100 dark:bg-primary-900/30 border-primary-500 text-primary-700 dark:text-primary-300"
										: "border-secondary-200 dark:border-secondary-600 text-secondary-600 dark:text-secondary-300 hover:border-secondary-400"
								}`}
							>
								{preset.label}
							</button>
						))}
					</div>
					<div className="flex items-center gap-2 mt-3">
						<input
							type="number"
							min="60"
							max="10080"
							step="60"
							value={formData.complianceScanInterval}
							onChange={(e) => {
								const val = Number.parseInt(e.target.value, 10);
								if (!Number.isNaN(val)) {
									handleInputChange(
										"complianceScanInterval",
										Math.max(60, Math.min(10080, val)),
									);
								}
							}}
							className="w-20 rounded-md border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-700 px-2 py-1.5 text-xs text-secondary-900 dark:text-white focus:border-primary-500 focus:ring-1 focus:ring-primary-500"
						/>
						<span className="text-xs text-secondary-400">
							minutes (every {formatInterval(formData.complianceScanInterval)})
						</span>
					</div>
				</div>
			</div>

			{/* OpenSCAP Content + Supported Profiles */}
			<div className="card p-4 md:p-5">
				<div className="flex items-center justify-between">
					<div className="flex items-center gap-2">
						<Info className="h-4 w-4 text-primary-600 dark:text-primary-400" />
						<h3 className="text-sm font-semibold text-secondary-900 dark:text-white">
							OpenSCAP Content
						</h3>
						{serverSSGInfo?.version && (
							<span className="text-xs text-secondary-400 dark:text-secondary-500">
								SSG {serverSSGInfo.version}
								{serverSSGInfo.files?.length > 0 &&
									` · ${serverSSGInfo.files.length} file${serverSSGInfo.files.length !== 1 ? "s" : ""}`}
							</span>
						)}
					</div>
					<button
						type="button"
						onClick={() => setShowProfiles(!showProfiles)}
						className="flex items-center gap-1 text-xs text-primary-600 dark:text-primary-400 hover:text-primary-700 dark:hover:text-primary-300"
					>
						{showProfiles ? "Hide details" : "Supported configurations"}
						<ChevronDown
							className={`h-3.5 w-3.5 transition-transform ${showProfiles ? "rotate-180" : ""}`}
						/>
					</button>
				</div>

				{!serverSSGInfo?.version && serverSSGInfo !== undefined && (
					<p className="text-sm text-secondary-500 dark:text-secondary-400 mt-2">
						No SSG content configured on server
					</p>
				)}

				{showProfiles && (
					<div className="mt-3 border-t border-secondary-200 dark:border-secondary-700 pt-3">
						{/* Content files */}
						{serverSSGInfo?.files?.length > 0 && (
							<div className="mb-3">
								<h4 className="text-xs font-medium text-secondary-500 dark:text-secondary-400 uppercase tracking-wider mb-2">
									Content Files
								</h4>
								<div className="flex flex-wrap gap-1.5">
									{serverSSGInfo.files.map((f) => (
										<span
											key={f}
											className="text-xs px-2 py-1 rounded bg-secondary-100 dark:bg-secondary-700 text-secondary-600 dark:text-secondary-300 font-mono"
										>
											{f}
										</span>
									))}
								</div>
							</div>
						)}

						{/* Supported OS + Profiles */}
						<h4 className="text-xs font-medium text-secondary-500 dark:text-secondary-400 uppercase tracking-wider mb-2">
							Supported OS & Profiles
						</h4>
						<div className="overflow-x-auto">
							<table className="min-w-full text-xs">
								<thead>
									<tr className="border-b border-secondary-200 dark:border-secondary-700">
										<th className="text-left py-1.5 pr-4 font-medium text-secondary-500 dark:text-white">
											Operating System
										</th>
										<th className="text-left py-1.5 font-medium text-secondary-500 dark:text-white">
											Profiles
										</th>
									</tr>
								</thead>
								<tbody className="divide-y divide-secondary-100 dark:divide-secondary-700">
									{SUPPORTED_PROFILES.map((row) => (
										<tr key={row.os}>
											<td className="py-1.5 pr-4 text-secondary-900 dark:text-white font-medium">
												{row.os}
											</td>
											<td className="py-1.5 text-secondary-500 dark:text-secondary-300">
												{row.profiles}
											</td>
										</tr>
									))}
								</tbody>
							</table>
						</div>
					</div>
				)}
			</div>

			{/* Save */}
			<div className="flex justify-end">
				<button
					type="button"
					onClick={handleSave}
					disabled={!isDirty || updateMutation.isPending}
					className={`inline-flex items-center px-4 py-2 text-sm font-medium rounded-md text-white ${
						!isDirty || updateMutation.isPending
							? "bg-secondary-400 cursor-not-allowed"
							: "bg-primary-600 hover:bg-primary-700"
					}`}
				>
					{updateMutation.isPending ? (
						<>
							<div className="animate-spin rounded-full h-4 w-4 border-b-2 border-white mr-2" />
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
		</div>
	);
};

export default ComplianceSettings;
