import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Clock, Info, Save, Shield } from "lucide-react";
import { useEffect, useId, useState } from "react";
import { useToast } from "../../contexts/ToastContext";
import { settingsAPI } from "../../utils/api";
import { complianceAPI } from "../../utils/complianceApi";

const ComplianceSettings = () => {
	const queryClient = useQueryClient();
	const toast = useToast();
	const defaultComplianceModeId = useId();
	const [formData, setFormData] = useState({
		defaultComplianceMode: "on-demand",
		complianceScanInterval: 1440,
	});
	const [isDirty, setIsDirty] = useState(false);

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
		<div className="space-y-6 max-w-3xl">
			{/* Default Compliance Mode */}
			<div className="card p-4 md:p-6">
				<div className="flex items-center gap-2 mb-1">
					<Shield className="h-4 w-4 text-primary-600 dark:text-primary-400" />
					<h3 className="text-sm font-semibold text-secondary-900 dark:text-white">
						Default Compliance Mode
					</h3>
				</div>
				<p className="text-xs text-secondary-500 dark:text-secondary-300 mb-4">
					Applies to newly registered hosts. Per-host overrides are available in
					the host detail page.
				</p>
				<div className="flex flex-col gap-2">
					{[
						{
							value: "disabled",
							label: "Disabled",
							desc: "Compliance scanning off for new hosts",
						},
						{
							value: "on-demand",
							label: "On-Demand",
							desc: "Scans run only when manually triggered",
						},
						{
							value: "enabled",
							label: "Enabled",
							desc: "Scans run automatically on schedule",
						},
					].map((opt) => (
						<label
							key={opt.value}
							className={`flex items-center gap-3 p-3 border rounded-lg cursor-pointer transition-colors ${
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
			<div className="card p-4 md:p-6">
				<div className="flex items-center gap-2 mb-1">
					<Clock className="h-4 w-4 text-primary-600 dark:text-primary-400" />
					<h3 className="text-sm font-semibold text-secondary-900 dark:text-white">
						Scan Interval
					</h3>
				</div>
				<p className="text-xs text-secondary-500 dark:text-secondary-300 mb-4">
					How often scheduled scans run when compliance mode is "Enabled".
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
					<div className="flex items-center gap-2 ml-2">
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
						<span className="text-xs text-secondary-400">min</span>
					</div>
				</div>
				<p className="text-xs text-secondary-400 dark:text-secondary-500 mt-2">
					Currently: every {formatInterval(formData.complianceScanInterval)}
				</p>
			</div>

			{/* OpenSCAP Info */}
			<div className="card p-4 md:p-6">
				<div className="flex items-center gap-2 mb-1">
					<Info className="h-4 w-4 text-primary-600 dark:text-primary-400" />
					<h3 className="text-sm font-semibold text-secondary-900 dark:text-white">
						OpenSCAP Content
					</h3>
				</div>
				{serverSSGInfo?.version ? (
					<div className="text-sm text-secondary-600 dark:text-secondary-300">
						<p>
							SCAP Security Guide <strong>{serverSSGInfo.version}</strong>
							{serverSSGInfo.files?.length > 0 && (
								<span className="ml-1 text-xs text-secondary-400">
									({serverSSGInfo.files.length} content file
									{serverSSGInfo.files.length !== 1 ? "s" : ""})
								</span>
							)}
						</p>
						<p className="text-xs text-secondary-400 dark:text-secondary-500 mt-1">
							CIS benchmarks for Ubuntu, Debian, RHEL, CentOS, Rocky, AlmaLinux,
							Fedora, SLES, and OpenSUSE (Level 1 & 2)
						</p>
					</div>
				) : (
					<p className="text-sm text-secondary-500 dark:text-secondary-400">
						{serverSSGInfo === undefined
							? "Loading..."
							: "No SSG content configured on server"}
					</p>
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
