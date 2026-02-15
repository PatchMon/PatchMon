import { useQueryClient } from "@tanstack/react-query";
import { CheckCircle, Copy, Download, RefreshCw, Wifi, X } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { dashboardAPI } from "../../utils/api";

// Check if host has received initial report (has system info beyond "unknown")
const hasInitialReport = (hostData) => {
	if (!hostData) return false;
	// Check if host has actual system information (not just placeholder values)
	// Check multiple fields to be more robust - if any of these are populated, we have a report
	const hasSystemInfo =
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
			!hostData.machine_id.startsWith("pending-"));

	// If we have system info, consider the report received (status might still be pending briefly)
	return hasSystemInfo;
};

const WaitingForConnection = ({
	host,
	onBack,
	onClose,
	plaintextApiKey,
	_serverUrl,
	curlFlags,
	installUrl,
	shellCommand,
	installOs = "linux",
}) => {
	const [_wsStatus, setWsStatus] = useState(null);
	const [connectionStage, setConnectionStage] = useState("waiting"); // waiting, connected, receiving, done
	const [hasNavigated, setHasNavigated] = useState(false);
	const transitionTimeoutRef = useRef(null);
	const navigate = useNavigate();
	const queryClient = useQueryClient();

	// Poll for websocket connection status and host data
	useEffect(() => {
		if (!host?.api_id || connectionStage === "done") return;

		let isMounted = true;
		let pollInterval;

		const fetchStatus = async () => {
			try {
				// Fetch websocket status
				const wsResponse = await fetch(`/api/v1/ws/status/${host.api_id}`, {
					credentials: "include",
				});

				if (wsResponse.ok && isMounted) {
					const wsResult = await wsResponse.json();
					const status = wsResult.data;
					setWsStatus(status);

					// Stage 1: Check if websocket is connected
					if (status?.connected && connectionStage === "waiting") {
						setConnectionStage("connected");
						// Invalidate queries to start fetching host data
						queryClient.invalidateQueries(["host", host.id]);
						queryClient.invalidateQueries(["hosts"]);
					}

					// Stage 2: If connected, check if initial report has been received
					if (
						status?.connected &&
						(connectionStage === "connected" || connectionStage === "receiving")
					) {
						try {
							// Fetch current host data to check if initial report is received
							const hostResponse = await dashboardAPI.getHostDetail(host.id);
							const hostData = hostResponse.data;

							if (hasInitialReport(hostData)) {
								// Initial report received!
								if (connectionStage === "connected") {
									setConnectionStage("receiving");
									// Clear any existing timeout
									if (transitionTimeoutRef.current) {
										clearTimeout(transitionTimeoutRef.current);
									}
									// Give a moment to show "Receiving initial report" message
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
									// Already showing "receiving" message, transition to done after a brief moment
									// Only set timeout if we haven't already set one
									transitionTimeoutRef.current = setTimeout(() => {
										if (isMounted) {
											setConnectionStage("done");
											transitionTimeoutRef.current = null;
										}
									}, 500);
								}
							}
						} catch (_err) {
							// Silently handle errors fetching host data
						}
					}
				}
			} catch (_err) {
				// Silently handle errors
			}
		};

		// Fetch immediately
		fetchStatus();

		// Poll every 2 seconds for faster response
		pollInterval = setInterval(fetchStatus, 2000);

		return () => {
			isMounted = false;
			if (pollInterval) {
				clearInterval(pollInterval);
			}
			if (transitionTimeoutRef.current) {
				clearTimeout(transitionTimeoutRef.current);
				transitionTimeoutRef.current = null;
			}
		};
	}, [host?.api_id, host?.id, connectionStage, queryClient]);

	// Separate effect to handle navigation when done
	useEffect(() => {
		if (connectionStage === "done" && !hasNavigated) {
			setHasNavigated(true);
			// Close the modal first
			onClose();
			// Small delay to ensure modal closes, then navigate
			setTimeout(() => {
				navigate(`/hosts/${host.id}`, { replace: true });
				// Refresh after 2 seconds to show all data
				setTimeout(() => {
					queryClient.invalidateQueries(["host", host.id]);
					queryClient.invalidateQueries(["hosts"]);
				}, 2000);
			}, 300);
		}
	}, [connectionStage, hasNavigated, navigate, host?.id, queryClient, onClose]);

	const copyCommand = async () => {
		const command =
			installOs === "windows"
				? `$script = Invoke-WebRequest -Uri "${installUrl}" -Headers @{"X-API-ID"="${host.api_id}"; "X-API-KEY"="${plaintextApiKey}"} -UseBasicParsing; $script.Content | Out-File -FilePath "$env:TEMP\\patchmon-install.ps1" -Encoding utf8; powershell.exe -ExecutionPolicy Bypass -File "$env:TEMP\\patchmon-install.ps1"`
				: `curl ${curlFlags} ${installUrl} -H "X-API-ID: ${host.api_id}" -H "X-API-KEY: ${plaintextApiKey}" | ${shellCommand}`;
		try {
			if (navigator.clipboard && window.isSecureContext) {
				await navigator.clipboard.writeText(command);
			} else {
				const textArea = document.createElement("textarea");
				textArea.value = command;
				textArea.style.position = "fixed";
				textArea.style.left = "-999999px";
				textArea.style.top = "-999999px";
				document.body.appendChild(textArea);
				textArea.focus();
				textArea.select();
				document.execCommand("copy");
				document.body.removeChild(textArea);
			}
		} catch (err) {
			console.error("Failed to copy:", err);
		}
	};

	return (
		<div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 p-4">
			<div className="bg-white dark:bg-secondary-800 rounded-lg p-6 w-full max-w-md">
				<div className="flex justify-between items-center mb-6">
					<h3 className="text-lg font-medium text-secondary-900 dark:text-white">
						Waiting for Connection
					</h3>
					<button
						type="button"
						onClick={onClose}
						className="text-secondary-400 hover:text-secondary-600 dark:text-secondary-500 dark:hover:text-secondary-300"
					>
						<X className="h-5 w-5" />
					</button>
				</div>

				<div className="space-y-6">
					{/* Connection Status */}
					<div className="flex flex-col items-center justify-center py-8">
						{connectionStage === "waiting" && (
							<>
								<div className="w-16 h-16 rounded-full bg-primary-100 dark:bg-primary-900/30 flex items-center justify-center mb-4">
									<Wifi className="h-8 w-8 text-primary-600 dark:text-primary-400 animate-pulse" />
								</div>
								<h4 className="text-lg font-semibold text-secondary-900 dark:text-white mb-2">
									Waiting for connection
								</h4>
								<p className="text-sm text-secondary-600 dark:text-secondary-400 text-center">
									Please run the installation command on your host. This page
									will automatically update when the connection is established.
								</p>
								<div className="mt-4 flex items-center gap-2 text-xs text-secondary-500 dark:text-secondary-400">
									<RefreshCw className="h-4 w-4 animate-spin" />
									<span>Checking connection status...</span>
								</div>
							</>
						)}

						{connectionStage === "connected" && (
							<>
								<div className="w-16 h-16 rounded-full bg-green-100 dark:bg-green-900/30 flex items-center justify-center mb-4">
									<CheckCircle className="h-8 w-8 text-green-600 dark:text-green-400" />
								</div>
								<h4 className="text-lg font-semibold text-green-600 dark:text-green-400 mb-2">
									Connected
								</h4>
								<p className="text-sm text-secondary-600 dark:text-secondary-400 text-center">
									Agent is connected. Waiting for initial system report...
								</p>
								<div className="mt-4 flex items-center gap-2 text-xs text-secondary-500 dark:text-secondary-400">
									<RefreshCw className="h-4 w-4 animate-spin" />
									<span>Waiting for initial report...</span>
								</div>
							</>
						)}

						{connectionStage === "receiving" && (
							<>
								<div className="w-16 h-16 rounded-full bg-blue-100 dark:bg-blue-900/30 flex items-center justify-center mb-4">
									<Download className="h-8 w-8 text-blue-600 dark:text-blue-400 animate-pulse" />
								</div>
								<h4 className="text-lg font-semibold text-blue-600 dark:text-blue-400 mb-2">
									Receiving initial report
								</h4>
								<p className="text-sm text-secondary-600 dark:text-secondary-400 text-center">
									Collecting system information from the agent...
								</p>
								<div className="mt-4 flex items-center gap-2 text-xs text-secondary-500 dark:text-secondary-400">
									<RefreshCw className="h-4 w-4 animate-spin" />
									<span>Processing data...</span>
								</div>
							</>
						)}

						{connectionStage === "done" && (
							<>
								<div className="w-16 h-16 rounded-full bg-green-100 dark:bg-green-900/30 flex items-center justify-center mb-4">
									<CheckCircle className="h-8 w-8 text-green-600 dark:text-green-400" />
								</div>
								<h4 className="text-lg font-semibold text-green-600 dark:text-green-400 mb-2">
									Done
								</h4>
								<p className="text-sm text-secondary-600 dark:text-secondary-400 text-center">
									Initial report received. Redirecting to host page...
								</p>
							</>
						)}
					</div>

					{/* Host Info */}
					<div className="bg-secondary-50 dark:bg-secondary-700 rounded-lg p-4">
						<div className="space-y-2">
							<div className="flex justify-between">
								<span className="text-sm font-medium text-secondary-700 dark:text-secondary-300">
									Host:
								</span>
								<span className="text-sm text-secondary-900 dark:text-white">
									{host.friendly_name}
								</span>
							</div>
							<div className="flex justify-between">
								<span className="text-sm font-medium text-secondary-700 dark:text-secondary-300">
									Status:
								</span>
								<span
									className={`text-sm font-medium ${
										connectionStage === "connected" ||
										connectionStage === "receiving" ||
										connectionStage === "done"
											? "text-green-600 dark:text-green-400"
											: "text-secondary-600 dark:text-secondary-400"
									}`}
								>
									{connectionStage === "waiting"
										? "Pending"
										: connectionStage === "connected"
											? "Connected"
											: connectionStage === "receiving"
												? "Receiving"
												: "Complete"}
								</span>
							</div>
						</div>
					</div>

					{/* Action Buttons - Hide when done */}
					{connectionStage !== "done" && (
						<div className="flex flex-col sm:flex-row gap-3">
							<button
								type="button"
								onClick={onBack}
								className="btn-outline flex-1 flex items-center justify-center gap-2"
							>
								<Copy className="h-4 w-4" />
								View Command
							</button>
							<button
								type="button"
								onClick={copyCommand}
								disabled={!plaintextApiKey}
								className="btn-primary flex-1 flex items-center justify-center gap-2 disabled:opacity-50 disabled:cursor-not-allowed"
							>
								<Copy className="h-4 w-4" />
								Copy Command Again
							</button>
						</div>
					)}
				</div>
			</div>
		</div>
	);
};

export default WaitingForConnection;
