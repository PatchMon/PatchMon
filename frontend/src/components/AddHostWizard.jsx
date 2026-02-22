import { useQuery, useQueryClient } from "@tanstack/react-query";
import { CheckCircle, Copy, Download, RefreshCw, Wifi, X } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { DiWindows } from "react-icons/di";
import { SiFreebsd, SiLinux } from "react-icons/si";
import { useNavigate } from "react-router-dom";
import {
	adminHostsAPI,
	dashboardAPI,
	hostGroupsAPI,
	settingsAPI,
} from "../utils/api";

const STEPS = [
	{ key: 1, label: "Choose OS" },
	{ key: 2, label: "Host details" },
	{ key: 3, label: "Copy command" },
	{ key: 4, label: "Connection" },
];

const hasInitialReport = (hostData) => {
	if (!hostData) return false;
	return (
		(hostData.os_type && hostData.os_type !== "unknown") ||
		(hostData.hostname &&
			hostData.hostname !== null &&
			hostData.hostname !== "") ||
		(hostData.ip && hostData.ip !== null && hostData.ip !== "") ||
		(hostData.architecture &&
			hostData.architecture !== null &&
			hostData.architecture !== "") ||
		(hostData.machine_id &&
			hostData.machine_id !== null &&
			!hostData.machine_id.startsWith("pending-"))
	);
};

const AddHostWizard = ({ isOpen, onClose, onSuccess }) => {
	const [step, setStep] = useState(1);
	const [platform, setPlatform] = useState("linux"); // linux | freebsd
	const [formData, setFormData] = useState({
		friendly_name: "",
		hostGroupIds: [],
		docker_enabled: false,
		compliance_enabled: false,
	});
	const [isSubmitting, setIsSubmitting] = useState(false);
	const [error, setError] = useState("");
	const [createdHost, setCreatedHost] = useState(null);
	const [plaintextApiKey, setPlaintextApiKey] = useState(null);
	const [connectionStage, setConnectionStage] = useState("waiting");
	const [hasNavigated, setHasNavigated] = useState(false);
	const transitionTimeoutRef = useRef(null);
	const navigate = useNavigate();
	const queryClient = useQueryClient();

	const { data: hostGroups } = useQuery({
		queryKey: ["hostGroups"],
		queryFn: () => hostGroupsAPI.list().then((res) => res.data),
		enabled: isOpen,
	});

	const { data: serverUrlData } = useQuery({
		queryKey: ["serverUrl"],
		queryFn: () => settingsAPI.getServerUrl().then((res) => res.data),
		enabled: isOpen,
	});

	const { data: settings } = useQuery({
		queryKey: ["settings"],
		queryFn: () => settingsAPI.get().then((res) => res.data),
		enabled: isOpen,
	});

	const serverUrl =
		serverUrlData?.server_url ||
		(import.meta.env.PROD
			? `${window.location.protocol}//${window.location.host}`
			: "http://localhost:3001");
	const curlFlags = settings?.ignore_ssl_self_signed ? "-sk" : "-s";

	const buildInstallUrl = (force = false) => {
		const base = `${serverUrl}/api/v1/hosts/install`;
		const params = new URLSearchParams();
		if (platform === "freebsd") params.set("os", "freebsd");
		if (force) params.set("force", "true");
		const qs = params.toString();
		return qs ? `${base}?${qs}` : base;
	};

	const getShellCommand = (force) => {
		const use_sudo = platform !== "freebsd";
		const base = use_sudo ? "sudo sh" : "sh";
		return force ? `${base} -s -- --force` : base;
	};

	// Poll for connection (steps 4–7)
	useEffect(() => {
		if (step < 4 || !createdHost?.api_id || connectionStage === "done") return;
		let isMounted = true;
		let pollInterval;
		const fetchStatus = async () => {
			try {
				const wsResponse = await fetch(
					`/api/v1/ws/status/${createdHost.api_id}`,
					{
						credentials: "include",
					},
				);
				if (!wsResponse.ok || !isMounted) return;
				const wsResult = await wsResponse.json();
				const status = wsResult.data;
				if (status?.connected && connectionStage === "waiting") {
					setConnectionStage("connected");
					queryClient.invalidateQueries(["host", createdHost.id]);
					queryClient.invalidateQueries(["hosts"]);
				}
				if (
					status?.connected &&
					(connectionStage === "connected" || connectionStage === "receiving")
				) {
					try {
						const hostResponse = await dashboardAPI.getHostDetail(
							createdHost.id,
						);
						if (hasInitialReport(hostResponse.data)) {
							if (connectionStage === "connected") {
								setConnectionStage("receiving");
								if (transitionTimeoutRef.current)
									clearTimeout(transitionTimeoutRef.current);
								transitionTimeoutRef.current = setTimeout(() => {
									if (isMounted) {
										setConnectionStage("done");
										transitionTimeoutRef.current = null;
									}
								}, 1500);
							} else if (
								connectionStage === "receiving" &&
								!transitionTimeoutRef.current
							) {
								transitionTimeoutRef.current = setTimeout(() => {
									if (isMounted) {
										setConnectionStage("done");
										transitionTimeoutRef.current = null;
									}
								}, 500);
							}
						}
					} catch (_err) {}
				}
			} catch (_err) {}
		};
		fetchStatus();
		pollInterval = setInterval(fetchStatus, 2000);
		return () => {
			isMounted = false;
			clearInterval(pollInterval);
			if (transitionTimeoutRef.current) {
				clearTimeout(transitionTimeoutRef.current);
				transitionTimeoutRef.current = null;
			}
		};
	}, [
		step,
		createdHost?.api_id,
		createdHost?.id,
		connectionStage,
		queryClient,
	]);

	// When done, close and navigate
	useEffect(() => {
		if (connectionStage !== "done" || hasNavigated || !createdHost) return;
		setHasNavigated(true);
		onClose();
		onSuccess?.();
		setTimeout(() => {
			navigate(`/hosts/${createdHost.id}`, {
				replace: true,
				state: { fromWizard: true },
			});
			setTimeout(() => {
				queryClient.invalidateQueries(["host", createdHost.id]);
				queryClient.invalidateQueries(["hosts"]);
			}, 2000);
		}, 300);
	}, [
		connectionStage,
		hasNavigated,
		createdHost,
		onClose,
		onSuccess,
		navigate,
		queryClient,
	]);

	const handleStep2Next = async (e) => {
		e.preventDefault();
		setIsSubmitting(true);
		setError("");
		try {
			const response = await adminHostsAPI.create({
				...formData,
				expected_platform: platform,
			});
			const data = response.data;
			setCreatedHost({
				id: data.hostId,
				api_id: data.apiId,
				friendly_name: data.friendlyName,
			});
			setPlaintextApiKey(data.apiKey);
			setStep(3);
		} catch (err) {
			let errorMessage = "Failed to create host";
			if (err.response?.data?.errors)
				errorMessage = err.response.data.errors.map((e) => e.msg).join(", ");
			else if (err.response?.data?.error)
				errorMessage = err.response.data.error;
			else if (err.message) errorMessage = err.message;
			setError(errorMessage);
		} finally {
			setIsSubmitting(false);
		}
	};

	const handleCopy = async () => {
		const forceInstall = false;
		const installUrl = buildInstallUrl(forceInstall);
		const command = `curl ${curlFlags} "${installUrl}" -H "X-API-ID: ${createdHost.api_id}" -H "X-API-KEY: ${plaintextApiKey}" | ${getShellCommand(forceInstall)}`;
		try {
			if (navigator.clipboard && window.isSecureContext) {
				await navigator.clipboard.writeText(command);
			} else {
				const ta = document.createElement("textarea");
				ta.value = command;
				ta.style.position = "fixed";
				ta.style.left = "-999999px";
				document.body.appendChild(ta);
				ta.focus();
				ta.select();
				document.execCommand("copy");
				document.body.removeChild(ta);
			}
			setStep(4);
		} catch (_err) {
			prompt("Copy this command:", command);
		}
	};

	const resetWizard = () => {
		setStep(1);
		setPlatform("linux");
		setFormData({
			friendly_name: "",
			hostGroupIds: [],
			docker_enabled: false,
			compliance_enabled: false,
		});
		setCreatedHost(null);
		setPlaintextApiKey(null);
		setConnectionStage("waiting");
		setHasNavigated(false);
		setError("");
	};

	if (!isOpen) return null;

	const currentStepKey = step >= 4 ? 4 : step;
	const stepIndicator = (
		<div className="flex items-center gap-2 mb-6 flex-wrap">
			{STEPS.map((s, _i) => (
				<span
					key={s.key}
					className={`text-xs font-medium px-2 py-1 rounded ${
						currentStepKey >= s.key
							? "bg-primary-100 dark:bg-primary-900/30 text-primary-700 dark:text-primary-300"
							: "bg-secondary-100 dark:bg-secondary-700 text-secondary-500"
					}`}
				>
					{s.key}. {s.label}
				</span>
			))}
		</div>
	);

	return (
		<div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4">
			<div className="bg-white dark:bg-secondary-800 rounded-lg p-4 sm:p-6 w-full max-w-lg max-h-[90vh] overflow-y-auto">
				<div className="flex justify-between items-center mb-2">
					<h3 className="text-lg font-medium text-secondary-900 dark:text-white">
						Add New Host
					</h3>
					<button
						type="button"
						onClick={() => {
							resetWizard();
							onClose();
						}}
						className="text-secondary-400 hover:text-secondary-600 dark:text-secondary-500 dark:hover:text-secondary-300"
					>
						<X className="h-5 w-5" />
					</button>
				</div>
				{stepIndicator}

				{/* Step 1: Choose OS */}
				{step === 1 && (
					<div className="space-y-4">
						<p className="text-sm text-secondary-600 dark:text-secondary-400">
							Select the operating system of the host you want to add. The
							install command will match this choice.
						</p>
						<div className="grid grid-cols-3 gap-4">
							<button
								type="button"
								onClick={() => setPlatform("linux")}
								className={`flex flex-col items-center justify-center p-6 rounded-lg border-2 transition-all ${
									platform === "linux"
										? "border-primary-500 bg-primary-50 dark:bg-primary-900/30"
										: "border-secondary-300 dark:border-secondary-600 hover:border-primary-400"
								}`}
							>
								<SiLinux className="h-12 w-12 text-secondary-700 dark:text-secondary-200 mb-2" />
								<span className="text-sm font-medium">Linux</span>
							</button>
							<button
								type="button"
								onClick={() => setPlatform("freebsd")}
								className={`flex flex-col items-center justify-center p-6 rounded-lg border-2 transition-all ${
									platform === "freebsd"
										? "border-primary-500 bg-primary-50 dark:bg-primary-900/30"
										: "border-secondary-300 dark:border-secondary-600 hover:border-primary-400"
								}`}
							>
								<SiFreebsd className="h-12 w-12 text-secondary-700 dark:text-secondary-200 mb-2" />
								<span className="text-sm font-medium">FreeBSD</span>
							</button>
							<div
								className="flex flex-col items-center justify-center p-6 rounded-lg border-2 border-secondary-200 dark:border-secondary-700 bg-secondary-50 dark:bg-secondary-800/50 opacity-60 cursor-not-allowed"
								title="Coming soon"
							>
								<DiWindows className="h-12 w-12 text-secondary-500 mb-2" />
								<span className="text-sm font-medium">Windows</span>
								<span className="text-xs text-secondary-500 mt-1">
									Coming soon
								</span>
							</div>
						</div>
						<div className="flex justify-end pt-2">
							<button
								type="button"
								onClick={() => setStep(2)}
								className="px-6 py-3 text-sm font-medium text-white bg-primary-600 rounded-lg hover:bg-primary-700 min-h-[44px]"
							>
								Next
							</button>
						</div>
					</div>
				)}

				{/* Step 2: Host details */}
				{step === 2 && (
					<form onSubmit={handleStep2Next} className="space-y-6">
						<div>
							<label className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-2">
								Friendly Name *
							</label>
							<input
								type="text"
								required
								value={formData.friendly_name}
								onChange={(e) =>
									setFormData({ ...formData, friendly_name: e.target.value })
								}
								className="block w-full px-3 py-2.5 border-2 border-secondary-300 dark:border-secondary-600 rounded-lg bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white min-h-[44px]"
								placeholder="server.example.com"
							/>
							<p className="mt-2 text-sm text-secondary-500 dark:text-secondary-400">
								System information will be detected when the agent connects.
							</p>
						</div>
						<div>
							<span className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-3">
								Host Groups
							</span>
							<div className="space-y-2 max-h-48 overflow-y-auto">
								{hostGroups?.map((group) => (
									<label
										key={group.id}
										className={`flex items-center gap-3 p-3 border-2 rounded-lg cursor-pointer ${
											formData.hostGroupIds.includes(group.id)
												? "border-primary-500 bg-primary-50 dark:bg-primary-900/30"
												: "border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-700"
										}`}
									>
										<input
											type="checkbox"
											checked={formData.hostGroupIds.includes(group.id)}
											onChange={(e) => {
												if (e.target.checked)
													setFormData({
														...formData,
														hostGroupIds: [...formData.hostGroupIds, group.id],
													});
												else
													setFormData({
														...formData,
														hostGroupIds: formData.hostGroupIds.filter(
															(id) => id !== group.id,
														),
													});
											}}
											className="w-4 h-4 text-primary-600 rounded"
										/>
										{group.color && (
											<div
												className="w-3 h-3 rounded-full flex-shrink-0"
												style={{ backgroundColor: group.color }}
											/>
										)}
										<span className="text-sm font-medium">{group.name}</span>
									</label>
								))}
							</div>
						</div>
						<div>
							<p className="text-xs text-secondary-500 dark:text-secondary-300 mb-1.5">
								Integrations (Optional)
							</p>
							<ul className="space-y-0 border border-secondary-200 dark:border-secondary-600 rounded-lg divide-y divide-secondary-200 dark:divide-secondary-600 overflow-hidden">
								<li className="flex items-center justify-between gap-3 px-3 py-2.5 bg-white dark:bg-secondary-700/50">
									<span className="text-sm font-medium text-secondary-900 dark:text-white">
										Docker
									</span>
									<button
										type="button"
										role="switch"
										aria-checked={formData.docker_enabled}
										onClick={() =>
											setFormData({
												...formData,
												docker_enabled: !formData.docker_enabled,
											})
										}
										className={`relative inline-flex h-5 w-9 flex-shrink-0 items-center rounded-md transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 ${
											formData.docker_enabled
												? "bg-primary-600 dark:bg-primary-500"
												: "bg-secondary-200 dark:bg-secondary-600"
										}`}
									>
										<span
											className={`inline-block h-3 w-3 transform rounded-full bg-white transition-transform ${
												formData.docker_enabled
													? "translate-x-5"
													: "translate-x-1"
											}`}
										/>
									</button>
								</li>
								<li className="flex items-center justify-between gap-3 px-3 py-2.5 bg-white dark:bg-secondary-700/50">
									<span className="text-sm font-medium text-secondary-900 dark:text-white">
										Compliance
									</span>
									<button
										type="button"
										role="switch"
										aria-checked={formData.compliance_enabled}
										onClick={() =>
											setFormData({
												...formData,
												compliance_enabled: !formData.compliance_enabled,
											})
										}
										className={`relative inline-flex h-5 w-9 flex-shrink-0 items-center rounded-md transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 ${
											formData.compliance_enabled
												? "bg-primary-600 dark:bg-primary-500"
												: "bg-secondary-200 dark:bg-secondary-600"
										}`}
									>
										<span
											className={`inline-block h-3 w-3 transform rounded-full bg-white transition-transform ${
												formData.compliance_enabled
													? "translate-x-5"
													: "translate-x-1"
											}`}
										/>
									</button>
								</li>
							</ul>
						</div>
						{error && (
							<div className="bg-danger-50 dark:bg-danger-900 border border-danger-200 dark:border-danger-700 rounded-md p-3">
								<p className="text-sm text-danger-700 dark:text-danger-300">
									{error}
								</p>
							</div>
						)}
						<div className="flex justify-between pt-2">
							<button
								type="button"
								onClick={() => setStep(1)}
								className="px-6 py-3 text-sm font-medium text-secondary-700 dark:text-secondary-200 bg-white dark:bg-secondary-700 border-2 border-secondary-300 dark:border-secondary-600 rounded-lg min-h-[44px]"
							>
								Back
							</button>
							<button
								type="submit"
								disabled={isSubmitting}
								className="px-6 py-3 text-sm font-medium text-white bg-primary-600 rounded-lg hover:bg-primary-700 disabled:opacity-50 min-h-[44px]"
							>
								{isSubmitting ? "Creating..." : "Next"}
							</button>
						</div>
					</form>
				)}

				{/* Step 3: Copy command */}
				{step === 3 && createdHost && (
					<div className="space-y-4">
						<p className="text-sm text-secondary-600 dark:text-secondary-400">
							Run this command on your{" "}
							{platform === "freebsd" ? "FreeBSD" : "Linux"} host to install the
							agent. After copying, the wizard will wait for the connection.
						</p>
						<div className="flex flex-col gap-2">
							<input
								type="text"
								readOnly
								value={`curl ${curlFlags} "${buildInstallUrl()}" -H "X-API-ID: ${createdHost.api_id}" -H "X-API-KEY: ${plaintextApiKey}" | ${getShellCommand(false)}`}
								className="w-full px-3 py-2 border-2 border-secondary-300 dark:border-secondary-600 rounded-lg bg-secondary-50 dark:bg-secondary-900 text-xs font-mono break-all"
							/>
							<button
								type="button"
								onClick={handleCopy}
								className="btn-primary flex items-center justify-center gap-2"
							>
								<Copy className="h-4 w-4" />
								Copy command
							</button>
						</div>
						<div className="flex justify-between pt-2">
							<button
								type="button"
								onClick={() => setStep(2)}
								className="px-6 py-3 text-sm font-medium text-secondary-700 dark:text-secondary-200 bg-white dark:bg-secondary-700 border-2 border-secondary-300 dark:border-secondary-600 rounded-lg min-h-[44px]"
							>
								Back
							</button>
						</div>
					</div>
				)}

				{/* Steps 4–7: Connection progress */}
				{step >= 4 && (
					<div className="space-y-6">
						{connectionStage === "waiting" && (
							<div className="flex flex-col items-center py-6">
								<div className="w-16 h-16 rounded-full bg-primary-100 dark:bg-primary-900/30 flex items-center justify-center mb-4">
									<Wifi className="h-8 w-8 text-primary-600 dark:text-primary-400 animate-pulse" />
								</div>
								<h4 className="text-lg font-semibold text-secondary-900 dark:text-white mb-2">
									Waiting for connection
								</h4>
								<p className="text-sm text-secondary-600 dark:text-secondary-400 text-center">
									Run the installation command on your host. This will update
									automatically when the agent connects.
								</p>
								<div className="mt-4 flex items-center gap-2 text-xs text-secondary-500">
									<RefreshCw className="h-4 w-4 animate-spin" />
									<span>Checking...</span>
								</div>
							</div>
						)}
						{connectionStage === "connected" && (
							<div className="flex flex-col items-center py-6">
								<div className="w-16 h-16 rounded-full bg-green-100 dark:bg-green-900/30 flex items-center justify-center mb-4">
									<CheckCircle className="h-8 w-8 text-green-600 dark:text-green-400" />
								</div>
								<h4 className="text-lg font-semibold text-green-600 dark:text-green-400 mb-2">
									Connected
								</h4>
								<p className="text-sm text-secondary-600 dark:text-secondary-400 text-center">
									Waiting for initial system report...
								</p>
								<div className="mt-4 flex items-center gap-2 text-xs text-secondary-500">
									<RefreshCw className="h-4 w-4 animate-spin" />
									<span>Waiting for report...</span>
								</div>
							</div>
						)}
						{connectionStage === "receiving" && (
							<div className="flex flex-col items-center py-6">
								<div className="w-16 h-16 rounded-full bg-blue-100 dark:bg-blue-900/30 flex items-center justify-center mb-4">
									<Download className="h-8 w-8 text-blue-600 dark:text-blue-400 animate-pulse" />
								</div>
								<h4 className="text-lg font-semibold text-blue-600 dark:text-blue-400 mb-2">
									Receiving initial report
								</h4>
								<p className="text-sm text-secondary-600 dark:text-secondary-400 text-center">
									Collecting system information...
								</p>
							</div>
						)}
						{connectionStage === "done" && (
							<div className="flex flex-col items-center py-6">
								<div className="w-16 h-16 rounded-full bg-green-100 dark:bg-green-900/30 flex items-center justify-center mb-4">
									<CheckCircle className="h-8 w-8 text-green-600 dark:text-green-400" />
								</div>
								<h4 className="text-lg font-semibold text-green-600 dark:text-green-400 mb-2">
									Done
								</h4>
								<p className="text-sm text-secondary-600 dark:text-secondary-400 text-center">
									Redirecting to host page...
								</p>
							</div>
						)}
						{connectionStage !== "done" && (
							<button
								type="button"
								onClick={() => setStep(3)}
								className="btn-outline w-full flex items-center justify-center gap-2"
							>
								<Copy className="h-4 w-4" />
								View command again
							</button>
						)}
					</div>
				)}
			</div>
		</div>
	);
};

export default AddHostWizard;
