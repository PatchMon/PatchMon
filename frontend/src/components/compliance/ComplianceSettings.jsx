import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertCircle, CheckCircle, Save, Shield, X } from "lucide-react";
import { useEffect, useId, useState } from "react";
import { Link } from "react-router-dom";
import { settingsAPI } from "../../utils/api";
import { complianceAPI } from "../../utils/complianceApi";

const ComplianceSettings = () => {
	const queryClient = useQueryClient();
	const defaultComplianceModeId = useId();
	const [formData, setFormData] = useState({
		defaultComplianceMode: "on-demand",
	});
	const [isDirty, setIsDirty] = useState(false);
	const [toast, setToast] = useState(null);

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
			});
			setIsDirty(false);
		}
	}, [settings]);

	const updateMutation = useMutation({
		mutationFn: (data) => settingsAPI.update(data).then((res) => res.data),
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ["settings"] });
			setIsDirty(false);
			setToast({
				message: "Compliance settings saved successfully!",
				type: "success",
			});
			setTimeout(() => setToast(null), 3000);
		},
		onError: (error) => {
			setToast({
				message:
					error.response?.data?.error || "Failed to update compliance settings",
				type: "error",
			});
			setTimeout(() => setToast(null), 3000);
		},
	});

	const handleInputChange = (field, value) => {
		setFormData((prev) => ({ ...prev, [field]: value }));
		setIsDirty(true);
	};

	const handleSave = () => {
		updateMutation.mutate({
			default_compliance_mode: formData.defaultComplianceMode,
		});
	};

	return (
		<div className="space-y-6">
			{/* Toast */}
			{toast && (
				<div
					className={`rounded-lg shadow-lg border-2 p-4 flex items-start space-x-3 ${
						toast.type === "success"
							? "bg-green-50 dark:bg-green-900/90 border-green-500 dark:border-green-600"
							: "bg-red-50 dark:bg-red-900/90 border-red-500 dark:border-red-600"
					}`}
				>
					<div
						className={`flex-shrink-0 rounded-full p-1 ${
							toast.type === "success"
								? "bg-green-100 dark:bg-green-800"
								: "bg-red-100 dark:bg-red-800"
						}`}
					>
						{toast.type === "success" ? (
							<CheckCircle className="h-5 w-5 text-green-600 dark:text-green-400" />
						) : (
							<AlertCircle className="h-5 w-5 text-red-600 dark:text-red-400" />
						)}
					</div>
					<div className="flex-1">
						<p
							className={`text-sm font-medium ${
								toast.type === "success"
									? "text-green-800 dark:text-green-100"
									: "text-red-800 dark:text-red-100"
							}`}
						>
							{toast.message}
						</p>
					</div>
					<button
						type="button"
						onClick={() => setToast(null)}
						className={`flex-shrink-0 rounded-lg p-1 transition-colors ${
							toast.type === "success"
								? "hover:bg-green-100 dark:hover:bg-green-800 text-green-600 dark:text-green-400"
								: "hover:bg-red-100 dark:hover:bg-red-800 text-red-600 dark:text-red-400"
						}`}
					>
						<X className="h-4 w-4" />
					</button>
				</div>
			)}

			{/* Header */}
			<div className="flex items-center gap-3">
				<div className="w-10 h-10 bg-primary-100 dark:bg-primary-900 rounded-lg flex items-center justify-center flex-shrink-0">
					<Shield className="h-5 w-5 text-primary-600 dark:text-primary-400" />
				</div>
				<div className="min-w-0">
					<h3 className="text-base md:text-lg font-semibold text-secondary-900 dark:text-white">
						Compliance Scanning
					</h3>
					<p className="text-xs md:text-sm text-secondary-600 dark:text-white">
						Security compliance scanning is built into the PatchMon Go agent
					</p>
				</div>
			</div>

			{/* Default Compliance Mode */}
			<div className="p-4 bg-secondary-50 dark:bg-secondary-800/50 rounded-lg border border-secondary-200 dark:border-secondary-700">
				<div className="flex items-center gap-2 mb-3">
					<label
						htmlFor={defaultComplianceModeId}
						className="text-sm font-medium text-secondary-900 dark:text-secondary-100"
					>
						Default Compliance Mode
					</label>
					<span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-primary-100 text-primary-800 dark:bg-primary-900/50 dark:text-primary-300">
						Master
					</span>
				</div>
				<p className="text-sm text-secondary-500 dark:text-white mb-4">
					Default compliance mode for all new hosts. Per-host settings in the
					dashboard can override this default.
				</p>
				<div className="flex flex-col gap-2">
					{["disabled", "on-demand", "enabled"].map((modeOption) => (
						<label
							key={modeOption}
							className={`flex items-center justify-between p-3 border-2 rounded-lg transition-all duration-200 cursor-pointer ${
								formData.defaultComplianceMode === modeOption
									? "border-primary-500 bg-primary-50 dark:bg-primary-900/20"
									: "bg-white dark:bg-secondary-700 hover:border-secondary-400 dark:hover:border-secondary-500 border-secondary-300 dark:border-secondary-600"
							}`}
						>
							<div className="flex items-center gap-3">
								<input
									type="radio"
									name="defaultComplianceMode"
									id={`${defaultComplianceModeId}-${modeOption}`}
									value={modeOption}
									checked={formData.defaultComplianceMode === modeOption}
									onChange={() =>
										handleInputChange("defaultComplianceMode", modeOption)
									}
									className="h-4 w-4 text-primary-600 border-secondary-300 focus:ring-primary-500"
								/>
								<div>
									<div className="text-sm font-medium text-secondary-700 dark:text-secondary-200 capitalize">
										{modeOption === "on-demand" ? "On-Demand Only" : modeOption}
									</div>
									<div className="text-xs text-secondary-500 dark:text-white">
										{modeOption === "disabled" &&
											"Compliance scanning is completely off for new hosts."}
										{modeOption === "on-demand" &&
											"Compliance scans run only when manually triggered from the UI for new hosts."}
										{modeOption === "enabled" &&
											"Compliance scans run automatically during scheduled reports for new hosts."}
									</div>
								</div>
							</div>
						</label>
					))}
				</div>
				<div className="flex justify-end mt-4">
					<button
						type="button"
						onClick={handleSave}
						disabled={!isDirty || updateMutation.isPending}
						className={`inline-flex items-center justify-center px-4 py-2 border border-transparent text-sm font-medium rounded-md shadow-sm text-white w-full sm:w-auto ${
							!isDirty || updateMutation.isPending
								? "bg-secondary-400 cursor-not-allowed"
								: "bg-primary-600 hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-primary-500"
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

			{/* Server OpenSCAP Content */}
			<div className="p-4 bg-secondary-50 dark:bg-secondary-800/50 rounded-lg border border-secondary-200 dark:border-secondary-700">
				<h4 className="text-sm font-medium text-secondary-900 dark:text-white mb-3">
					OpenSCAP Content on PatchMon Server
				</h4>
				{serverSSGInfo === undefined ? (
					<p className="text-sm text-secondary-500 dark:text-white">
						Loading...
					</p>
				) : serverSSGInfo?.version ? (
					<div className="space-y-2 text-sm text-secondary-700 dark:text-white">
						<p>
							<strong>SCAP Security Guide version:</strong>{" "}
							{serverSSGInfo.version}
						</p>
						{serverSSGInfo.files?.length > 0 && (
							<div>
								<p className="font-medium mb-1">Available content files:</p>
								<ul className="list-disc list-inside text-secondary-600 dark:text-secondary-300">
									{serverSSGInfo.files.map((f) => (
										<li key={f}>{f}</li>
									))}
								</ul>
							</div>
						)}
					</div>
				) : (
					<p className="text-sm text-secondary-500 dark:text-white">
						No SSG content configured on server
					</p>
				)}
			</div>

			{/* Info */}
			<div className="bg-primary-50 dark:bg-primary-900/20 border border-primary-200 dark:border-primary-800 rounded-lg p-4 md:p-6">
				<div className="flex items-start gap-3">
					<CheckCircle className="h-5 w-5 text-primary-600 dark:text-primary-400 flex-shrink-0 mt-0.5" />
					<div className="min-w-0">
						<h4 className="text-sm md:text-base font-semibold text-primary-900 dark:text-primary-200 mb-2">
							Automatic Security Compliance Scanning
						</h4>
						<ul className="list-disc list-inside space-y-2 text-xs md:text-sm text-primary-800 dark:text-primary-300 ml-2">
							<li>
								<strong>OpenSCAP</strong> - CIS benchmarks with automatic tool
								installation
							</li>
							<li>
								<strong>Scoring & Trending</strong> - Compliance scores and
								historical tracking
							</li>
							<li>
								<strong>On-Demand Scans</strong> - Trigger scans from the
								dashboard at any time
							</li>
							<li>
								<strong>Auto-Remediation</strong> - Fix failing rules when
								enabled
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
				<ol className="list-decimal list-inside space-y-3 text-xs md:text-sm text-secondary-700 dark:text-white">
					<li>
						Install the PatchMon Go agent on your host (see the Hosts page for
						installation instructions)
					</li>
					<li>Enable the Compliance integration from the dashboard</li>
					<li>
						The agent automatically installs OpenSCAP and SCAP Security Guide
					</li>
					<li>
						View scores, failing rules, and remediation in the{" "}
						<Link
							to="/compliance"
							className="text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300 underline"
						>
							Compliance page
						</Link>
					</li>
					<li>
						Trigger on-demand scans or auto-remediation from the host details
						page
					</li>
				</ol>
			</div>

			{/* Supported Profiles */}
			<div className="bg-white dark:bg-secondary-900 border border-secondary-200 dark:border-secondary-600 rounded-lg p-4 md:p-6">
				<h4 className="text-sm md:text-base font-semibold text-secondary-900 dark:text-white mb-4">
					OpenSCAP
				</h4>
				<div className="flex items-start gap-2">
					<Shield className="h-5 w-5 text-blue-600 dark:text-blue-400 flex-shrink-0 mt-0.5" />
					<p className="text-xs md:text-sm text-secondary-600 dark:text-white">
						CIS benchmarks for Ubuntu, Debian, RHEL, CentOS, Rocky, AlmaLinux,
						Fedora, SLES, and OpenSUSE. Supports Level 1 and Level 2 server
						profiles.
					</p>
				</div>
			</div>

			{/* Requirements */}
			<div className="bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-lg p-3 md:p-4">
				<div className="flex items-start gap-2">
					<AlertCircle className="h-5 w-5 text-blue-600 dark:text-blue-400 flex-shrink-0 mt-0.5" />
					<div className="text-xs md:text-sm text-blue-800 dark:text-blue-200">
						<p className="font-semibold mb-2">Requirements</p>
						<p className="mb-2">
							The agent automatically installs openscap-scanner and
							scap-security-guide when you enable Compliance.
						</p>
						<ul className="list-disc list-inside space-y-1 ml-2">
							<li>PatchMon Go agent must be installed and running</li>
							<li>Agent must run as root for full compliance scanning</li>
						</ul>
					</div>
				</div>
			</div>
		</div>
	);
};

export default ComplianceSettings;
