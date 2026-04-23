/**
 * RdpViewer - In-browser RDP for Windows hosts via Apache Guacamole (guacd)
 *
 * Uses guacamole-common-js with WebSocket tunnel to the PatchMon server.
 * Backend runs guacd as subprocess and bridges agent RDP proxy to guacd.
 *
 * Error UX: every failure maps to a stable code (server-supplied in the
 * ticket response, or client-derived from Guacamole status codes) and renders
 * a specific guidance block. Before connecting, a requirements checklist is
 * shown so the user knows what RDP needs end-to-end.
 */

import Guacamole from "guacamole-common-js";
import {
	AlertCircle,
	CheckCircle2,
	ChevronDown,
	ChevronRight,
	Info,
	Maximize2,
	Minimize2,
	Monitor,
	Power,
	RefreshCw,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { rdpAPI } from "../utils/api";

// Guacamole Status codes we surface with bespoke guidance. See
// https://guacamole.apache.org/doc/gug/protocol-reference.html#status-codes
const GUAC_STATUS_CLIENT_UNAUTHORIZED = 0x0301; // 769
const GUAC_STATUS_CLIENT_FORBIDDEN = 0x0303; // 771
const GUAC_STATUS_UPSTREAM_NOT_FOUND = 0x0204; // 516 — host unreachable via guacd
const GUAC_STATUS_UPSTREAM_UNAVAILABLE = 0x0207; // 519 — NLA/auth failure (Guacamole remaps)

/**
 * Guidance blocks rendered in the error panel. Keyed by code returned from the
 * backend (server classifies agent errors into these codes) plus client-side
 * codes derived from Guacamole status.
 */
const GUIDANCE = {
	guacd_unavailable: {
		title: "guacd is not running on the PatchMon server",
		body: [
			"The RDP gateway (guacd) is required to render Windows desktops in the browser.",
			"On the PatchMon server, install and start guacd:",
		],
		code: `# Debian / Ubuntu
apt install guacd

# RHEL / CentOS / Rocky (EPEL)
dnf install guacd

# Docker: add the guacamole/guacd sidecar and set GUACD_ADDRESS=guacd:4822`,
		doc: "See Internal documentation: technical/WINDOWS.md",
	},
	agent_disconnected: {
		title: "The PatchMon agent on this host is not connected",
		body: [
			"The agent must be running and maintain a WebSocket to the PatchMon server for RDP to work.",
			"Check the agent service status on the Windows host:",
		],
		code: `# PowerShell (as Administrator)
Get-Service patchmon-agent
Start-Service patchmon-agent

# Or inspect recent logs
Get-Content "C:\\ProgramData\\PatchMon\\agent.log" -Tail 200`,
	},
	agent_timeout: {
		title: "The agent did not respond in time",
		body: [
			"The RDP proxy request was sent but no reply arrived within 5 seconds.",
			"Usually this means the agent is unhealthy or the network link is saturated. Try again, and if it keeps failing, restart the agent service.",
		],
	},
	agent_rdp_disabled: {
		title: "RDP proxy is disabled in the agent config",
		body: [
			"The agent rejected the request because the RDP proxy integration is not enabled. This must be enabled manually on the host — it cannot be pushed from the server.",
			"Edit the agent config and restart the service:",
		],
		code: `# C:\\ProgramData\\PatchMon\\config.yml
integrations:
    rdp-proxy-enabled: true

# Then restart the agent
Restart-Service patchmon-agent`,
	},
	rdp_port_unreachable: {
		title: "The Windows host's RDP service is not reachable on port 3389",
		body: [
			"The agent connected to the server but could not open a TCP connection to localhost:3389 on the host.",
			"On the Windows host, confirm Remote Desktop is enabled and listening:",
		],
		code: `# PowerShell (as Administrator)
# 1. Enable Remote Desktop
Set-ItemProperty -Path 'HKLM:\\System\\CurrentControlSet\\Control\\Terminal Server' -Name fDenyTSConnections -Value 0
Enable-NetFirewallRule -DisplayGroup "Remote Desktop"

# 2. Confirm the service is running and listening
Get-Service TermService
Test-NetConnection -ComputerName localhost -Port 3389`,
	},
	agent_invalid_host: {
		title: "The agent rejected the proxy host",
		body: [
			"The server requested a proxy target that the agent refused to dial. This is typically a version mismatch — update the PatchMon agent on the host to the latest release.",
		],
	},
	agent_error: {
		title: "The agent reported an error",
		body: [
			"The RDP proxy could not be established. The agent's message is shown above — follow its instructions.",
		],
	},
	agent_send_failed: {
		title: "Could not deliver the RDP proxy request to the agent",
		body: [
			"The agent's WebSocket was accepted but the write failed. The agent may have just disconnected. Wait a few seconds and try again.",
		],
	},
	max_sessions: {
		title: "Too many concurrent RDP sessions",
		body: [
			"The server is at its concurrent RDP session limit. Close unused sessions and try again.",
		],
	},
	server_error: {
		title: "An unexpected server error occurred",
		body: [
			"The request failed on the server. Check the PatchMon server logs for details.",
		],
	},
	rdp_auth_failed: {
		title: "Authentication to the Windows host failed",
		body: [
			"The Windows host accepted the connection but refused the credentials.",
			"Confirm the username and password are correct. If the host requires Network Level Authentication (NLA), the credentials must belong to a user permitted by the host's Remote Desktop settings.",
			"If you left credentials blank, try providing them explicitly — some hosts do not allow NLA with empty credentials.",
		],
	},
	rdp_gateway_failed: {
		title: "guacd could not complete the RDP handshake",
		body: [
			"The RDP gateway reached the host but the protocol handshake failed. This is often a certificate/TLS issue on the Windows side or an unsupported RDP security level.",
			"On the host, ensure Remote Desktop security is set to 'Negotiate' or 'SSL (TLS 1.0)'. Verify the host certificate is valid.",
		],
	},
	rdp_unknown: {
		title: "RDP connection failed",
		body: [
			"The connection failed for an unclassified reason. The underlying message is shown above.",
		],
	},
};

/**
 * Pick a guidance code from either an HTTP error response (ticket creation)
 * or a Guacamole Status object (tunnel/client onerror).
 */
const deriveCodeFromHttpError = (err) => {
	const data = err?.response?.data;
	if (data?.code && GUIDANCE[data.code]) return data.code;
	const msg = (data?.error || err?.message || "").toLowerCase();
	if (msg.includes("guacd")) return "guacd_unavailable";
	if (
		msg.includes("agent not connected") ||
		msg.includes("agent is not connected")
	)
		return "agent_disconnected";
	if (msg.includes("rdp-proxy-enabled")) return "agent_rdp_disabled";
	return "rdp_unknown";
};

const deriveCodeFromGuacStatus = (status) => {
	const code = status?.code;
	switch (code) {
		case GUAC_STATUS_CLIENT_UNAUTHORIZED:
		case GUAC_STATUS_CLIENT_FORBIDDEN:
			return "rdp_auth_failed";
		case GUAC_STATUS_UPSTREAM_NOT_FOUND:
			return "rdp_port_unreachable";
		case GUAC_STATUS_UPSTREAM_UNAVAILABLE:
			return "rdp_gateway_failed";
		default:
			return "rdp_unknown";
	}
};

const Requirement = ({ children }) => (
	<li className="flex items-start gap-2 text-sm text-secondary-300">
		<CheckCircle2 className="h-4 w-4 text-primary-500 shrink-0 mt-0.5" />
		<span>{children}</span>
	</li>
);

const ErrorPanel = ({ code, message, onRetry }) => {
	const guidance = GUIDANCE[code] || GUIDANCE.rdp_unknown;
	return (
		<div className="w-full max-w-2xl bg-secondary-800 border border-secondary-700 rounded-lg p-5 shadow-lg">
			<div className="flex items-start gap-3">
				<AlertCircle className="h-6 w-6 text-danger-400 shrink-0 mt-0.5" />
				<div className="flex-1 min-w-0">
					<h3 className="text-base font-semibold text-white">
						{guidance.title}
					</h3>
					{message && (
						<pre className="mt-2 whitespace-pre-wrap text-xs text-danger-300 bg-secondary-900/60 border border-secondary-700 rounded p-2 font-mono">
							{message}
						</pre>
					)}
					<div className="mt-3 space-y-2">
						{guidance.body.map((p) => (
							<p key={p} className="text-sm text-secondary-300">
								{p}
							</p>
						))}
					</div>
					{guidance.code && (
						<pre className="mt-3 text-xs text-secondary-200 bg-black/40 border border-secondary-700 rounded p-3 font-mono overflow-x-auto">
							{guidance.code}
						</pre>
					)}
					{guidance.doc && (
						<p className="mt-2 text-xs text-secondary-500">{guidance.doc}</p>
					)}
					{onRetry && (
						<button
							type="button"
							onClick={onRetry}
							className="mt-4 inline-flex items-center gap-1.5 px-3 py-1.5 rounded-md bg-primary-600 hover:bg-primary-500 text-white text-sm font-medium"
						>
							<RefreshCw className="h-4 w-4" />
							Retry
						</button>
					)}
				</div>
			</div>
		</div>
	);
};

const RequirementsChecklist = ({ open, onToggle }) => (
	<div className="w-full max-w-2xl bg-secondary-800/60 border border-secondary-700 rounded-lg">
		<button
			type="button"
			onClick={onToggle}
			className="w-full flex items-center gap-2 px-4 py-3 text-left"
			aria-expanded={open}
		>
			{open ? (
				<ChevronDown className="h-4 w-4 text-secondary-400" />
			) : (
				<ChevronRight className="h-4 w-4 text-secondary-400" />
			)}
			<Info className="h-4 w-4 text-primary-500" />
			<span className="text-sm font-medium text-white">
				What does RDP need to work?
			</span>
		</button>
		{open && (
			<div className="px-4 pb-4 space-y-3">
				<p className="text-xs text-secondary-400">
					In-browser RDP bridges your browser → PatchMon server (guacd) →
					PatchMon agent on the host → Windows RDP service on port 3389. All
					four pieces must be in place.
				</p>
				<ul className="space-y-2">
					<Requirement>
						<strong className="text-white">On the Windows host:</strong> Remote
						Desktop enabled and listening on <code>3389</code>.
					</Requirement>
					<Requirement>
						<strong className="text-white">PatchMon agent</strong> installed,
						connected, and at a recent version.
					</Requirement>
					<Requirement>
						<strong className="text-white">Agent config:</strong>{" "}
						<code>integrations: rdp-proxy-enabled: true</code> in{" "}
						<code>config.yml</code>, then restart the agent service. This is
						opt-in per host — the server cannot enable it remotely.
					</Requirement>
					<Requirement>
						<strong className="text-white">PatchMon server:</strong>{" "}
						<code>guacd</code> running locally or as a sidecar, with{" "}
						<code>GUACD_ADDRESS</code> pointing to it.
					</Requirement>
				</ul>
				<p className="text-xs text-secondary-500">
					Valid credentials for a user with Remote Desktop access are required.
					Leave blank only if the host allows unauthenticated NLA prompts.
				</p>
			</div>
		)}
	</div>
);

const RdpViewer = ({ host, isOpen }) => {
	const displayRef = useRef(null);
	const clientRef = useRef(null);
	const tunnelRef = useRef(null);
	const mouseRef = useRef(null);
	const keyboardRef = useRef(null);
	const resizeObserverRef = useRef(null);
	const pasteHandlerRef = useRef(null);

	const [isConnecting, setIsConnecting] = useState(false);
	const [isConnected, setIsConnected] = useState(false);
	const [error, setError] = useState(null); // { code, message } | null
	const [isFullscreen, setIsFullscreen] = useState(false);
	const [requirementsOpen, setRequirementsOpen] = useState(false);
	const [credentials, setCredentials] = useState({
		username: "",
		password: "",
	});

	/** Scale the Guacamole display to fit the container */
	const scaleDisplay = useCallback(() => {
		const client = clientRef.current;
		const container = displayRef.current;
		if (!client || !container) return;

		const display = client.getDisplay();
		const displayWidth = display.getWidth();
		const displayHeight = display.getHeight();
		if (!displayWidth || !displayHeight) return;

		const containerWidth = container.offsetWidth;
		const containerHeight = container.offsetHeight;
		const scale = Math.min(
			containerWidth / displayWidth,
			containerHeight / displayHeight,
		);

		display.scale(scale);
	}, []);

	const disconnect = useCallback(() => {
		if (pasteHandlerRef.current && displayRef.current) {
			displayRef.current.removeEventListener("paste", pasteHandlerRef.current);
			pasteHandlerRef.current = null;
		}
		if (resizeObserverRef.current) {
			resizeObserverRef.current.disconnect();
			resizeObserverRef.current = null;
		}
		if (clientRef.current) {
			try {
				clientRef.current.disconnect();
			} catch {
				// ignore
			}
			clientRef.current = null;
		}
		if (tunnelRef.current) {
			try {
				tunnelRef.current.disconnect();
			} catch {
				// ignore
			}
			tunnelRef.current = null;
		}
		if (mouseRef.current) {
			mouseRef.current = null;
		}
		if (keyboardRef.current) {
			keyboardRef.current = null;
		}
		setIsConnected(false);
	}, []);

	// Disconnect when tab closes or host changes
	useEffect(() => {
		if (!isOpen || !host) {
			disconnect();
		}
		return () => disconnect();
	}, [isOpen, host, disconnect]);

	// Reset transient state when the host changes so stale errors and creds
	// from a previous host do not bleed into the new one. host?.id is the
	// intentional trigger; setState callbacks are stable and correctly omitted.
	// biome-ignore lint/correctness/useExhaustiveDependencies: intentional reset trigger
	useEffect(() => {
		setError(null);
		setCredentials({ username: "", password: "" });
		setRequirementsOpen(false);
	}, [host?.id]);

	const connect = useCallback(async () => {
		if (!host?.id || !isOpen) return;

		disconnect();
		setError(null);
		setIsConnecting(true);

		// Measure the display container to request matching dimensions
		const containerWidth = displayRef.current?.offsetWidth || 1024;
		const containerHeight = displayRef.current?.offsetHeight || 768;

		let ticketData;
		try {
			const res = await rdpAPI.createTicket({
				hostId: host.id,
				username: credentials.username || undefined,
				password: credentials.password || undefined,
				width: containerWidth,
				height: containerHeight,
			});
			ticketData = res.data;
		} catch (err) {
			setIsConnecting(false);
			const serverMsg = err.response?.data?.error || err.message || "";
			setError({
				code: deriveCodeFromHttpError(err),
				message: serverMsg,
			});
			return;
		}

		const {
			ticket,
			websocketTunnelUrl,
			width = containerWidth,
			height = containerHeight,
		} = ticketData;
		if (!ticket || !websocketTunnelUrl) {
			setError({ code: "server_error", message: "Invalid ticket response" });
			setIsConnecting(false);
			return;
		}

		const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
		const baseUrl = `${protocol}//${window.location.host}${websocketTunnelUrl}`;
		const connectData = `ticket=${encodeURIComponent(ticket)}&width=${width}&height=${height}`;

		try {
			const tunnel = new Guacamole.WebSocketTunnel(baseUrl);
			tunnelRef.current = tunnel;

			tunnel.onerror = (status) => {
				// disconnect() teardown can fire onerror with no status; ignore
				// that. Don't clobber an error already set by an earlier path.
				if (!status) return;
				setError((prev) =>
					prev
						? prev
						: {
								code: deriveCodeFromGuacStatus(status),
								message:
									status?.message ||
									`Tunnel error (code ${status?.code ?? "?"})`,
							},
				);
				setIsConnecting(false);
				setIsConnected(false);
			};

			tunnel.onstatechange = (state) => {
				if (state === Guacamole.Tunnel.State.CLOSED) {
					setIsConnected(false);
					setIsConnecting(false);
				}
			};

			const client = new Guacamole.Client(tunnel);
			clientRef.current = client;

			client.onstatechange = (state) => {
				if (state === Guacamole.Client.State.CONNECTED) {
					setIsConnecting(false);
					setIsConnected(true);
					setError(null);

					// Auto-focus the display element for keyboard input
					if (displayRef.current) {
						displayRef.current.focus();
					}

					// Scale display to fit after connection
					scaleDisplay();
				} else if (
					state === Guacamole.Client.State.DISCONNECTED ||
					state === Guacamole.Client.State.CONNECTING
				) {
					// CONNECTING is initial; DISCONNECTED is final
					if (state === Guacamole.Client.State.DISCONNECTED) {
						setIsConnected(false);
						setIsConnecting(false);
					}
				}
			};

			client.onerror = (status) => {
				if (!status) return;
				setError((prev) =>
					prev
						? prev
						: {
								code: deriveCodeFromGuacStatus(status),
								message:
									status?.message || `RDP error (code ${status?.code ?? "?"})`,
							},
				);
				setIsConnecting(false);
				setIsConnected(false);
			};

			// Clipboard sync: receive remote clipboard data
			client.onclipboard = (stream, mimetype) => {
				if (mimetype !== "text/plain") return;
				const reader = new Guacamole.StringReader(stream);
				let clipboardData = "";
				reader.ontext = (text) => {
					clipboardData += text;
				};
				reader.onend = () => {
					if (clipboardData && navigator.clipboard?.writeText) {
						navigator.clipboard.writeText(clipboardData).catch(() => {
							// Clipboard write may fail without user gesture
						});
					}
				};
			};

			if (displayRef.current) {
				// Properly clean up existing child nodes
				while (displayRef.current.firstChild) {
					displayRef.current.removeChild(displayRef.current.firstChild);
				}
				displayRef.current.appendChild(client.getDisplay().getElement());

				const display = client.getDisplay();
				const element = display.getElement();

				const mouse = new Guacamole.Mouse(element);
				mouse.onmousedown =
					mouse.onmousemove =
					mouse.onmouseup =
						(mouseState) => {
							display.moveCursor(
								mouseState.x,
								mouseState.y,
								mouseState.left,
								mouseState.middle,
								mouseState.right,
							);
							client.sendMouseState(mouseState);
						};
				mouseRef.current = mouse;

				const keyboard = new Guacamole.Keyboard(element);
				keyboard.onkeydown = (keysym) => {
					client.sendKeyEvent(1, keysym);
				};
				keyboard.onkeyup = (keysym) => {
					client.sendKeyEvent(0, keysym);
				};
				keyboardRef.current = keyboard;

				// Observe container resize and scale display accordingly
				const observer = new ResizeObserver(() => {
					scaleDisplay();
				});
				observer.observe(displayRef.current);
				resizeObserverRef.current = observer;
			}

			// Clipboard sync: send local clipboard on paste
			const handlePaste = (e) => {
				const text = e.clipboardData?.getData("text/plain");
				if (text && clientRef.current) {
					const stream = clientRef.current.createClipboardStream("text/plain");
					const writer = new Guacamole.StringWriter(stream);
					writer.sendText(text);
					writer.sendEnd();
				}
			};
			pasteHandlerRef.current = handlePaste;
			displayRef.current?.addEventListener("paste", handlePaste);

			client.connect(connectData);
		} catch (err) {
			setError({
				code: "rdp_unknown",
				message: err?.message || "Failed to connect",
			});
			setIsConnecting(false);
			disconnect();
		}
	}, [
		host?.id,
		isOpen,
		credentials.username,
		credentials.password,
		disconnect,
		scaleDisplay,
	]);

	const handleCredentialKeyDown = useCallback(
		(e) => {
			if (e.key === "Enter") {
				connect();
			}
		},
		[connect],
	);

	const toggleFullscreen = () => {
		const container = displayRef.current?.closest(".rdp-viewer-container");
		if (!container) return;
		if (!document.fullscreenElement) {
			container.requestFullscreen?.();
		} else {
			document.exitFullscreen?.();
		}
	};

	useEffect(() => {
		const handler = () => {
			setIsFullscreen(!!document.fullscreenElement);
		};
		document.addEventListener("fullscreenchange", handler);
		return () => document.removeEventListener("fullscreenchange", handler);
	}, []);

	// Show the requirements checklist by default if the user has never connected
	// AND has no error yet — keeps the first-run experience guided without
	// shouting at users who already know the drill.
	const showIdleHelp = useMemo(
		() => !isConnected && !isConnecting && !error,
		[isConnected, isConnecting, error],
	);

	if (!host || !isOpen) return null;

	return (
		<div className="rdp-viewer-container flex flex-col h-full min-h-[300px] bg-secondary-800 rounded-lg overflow-hidden">
			<div className="flex items-center justify-between px-4 py-3 border-b border-secondary-600 shrink-0">
				<div className="flex items-center gap-2">
					<Monitor className="h-5 w-5 text-primary-500" />
					<span className="font-medium text-white">
						{host?.friendly_name || host?.hostname || host?.ip}
					</span>
					{isConnected ? (
						<span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-green-900/50 text-green-400 border border-green-700/50">
							<span className="h-1.5 w-1.5 rounded-full bg-green-400" />
							Connected
						</span>
					) : (
						<span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium bg-secondary-700 text-secondary-400 border border-secondary-600">
							<span className="h-1.5 w-1.5 rounded-full bg-secondary-500" />
							Disconnected
						</span>
					)}
				</div>
				<div className="flex items-center gap-2">
					{!isConnected && !isConnecting && (
						<div className="flex gap-2 items-center">
							<input
								type="text"
								placeholder="Username (optional)"
								value={credentials.username}
								onChange={(e) =>
									setCredentials((c) => ({
										...c,
										username: e.target.value,
									}))
								}
								onKeyDown={handleCredentialKeyDown}
								className="px-2 py-1 text-sm rounded bg-secondary-700 text-white border border-secondary-600 w-32"
							/>
							<input
								type="password"
								placeholder="Password (optional)"
								value={credentials.password}
								onChange={(e) =>
									setCredentials((c) => ({
										...c,
										password: e.target.value,
									}))
								}
								onKeyDown={handleCredentialKeyDown}
								className="px-2 py-1 text-sm rounded bg-secondary-700 text-white border border-secondary-600 w-32"
							/>
						</div>
					)}
					{!isConnected && (
						<button
							type="button"
							onClick={connect}
							disabled={isConnecting}
							className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-md bg-primary-600 hover:bg-primary-500 text-white text-sm font-medium disabled:opacity-50 disabled:cursor-not-allowed"
						>
							{isConnecting ? (
								<RefreshCw className="h-4 w-4 animate-spin" />
							) : (
								<RefreshCw className="h-4 w-4" />
							)}
							{isConnecting ? "Connecting..." : "Connect"}
						</button>
					)}
					{isConnected && (
						<>
							<button
								type="button"
								onClick={disconnect}
								className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-md bg-red-700 hover:bg-red-600 text-white text-sm font-medium"
							>
								<Power className="h-4 w-4" />
								Disconnect
							</button>
							<button
								type="button"
								onClick={toggleFullscreen}
								className="p-1.5 rounded hover:bg-secondary-600 text-secondary-300 hover:text-white"
								title={isFullscreen ? "Exit fullscreen" : "Fullscreen"}
							>
								{isFullscreen ? (
									<Minimize2 className="h-4 w-4" />
								) : (
									<Maximize2 className="h-4 w-4" />
								)}
							</button>
						</>
					)}
				</div>
			</div>

			<div className="flex-1 min-h-0 flex flex-col relative">
				{error && (
					<div className="absolute inset-0 flex items-start justify-center bg-secondary-900/95 z-10 p-4 overflow-y-auto">
						<div className="w-full flex justify-center pt-4">
							<ErrorPanel
								code={error.code}
								message={error.message}
								onRetry={connect}
							/>
						</div>
					</div>
				)}

				{isConnecting && !error && (
					<div className="absolute inset-0 flex items-center justify-center bg-secondary-900/80 z-10">
						<div className="flex flex-col items-center gap-2">
							<RefreshCw className="h-8 w-8 text-primary-500 animate-spin" />
							<span className="text-sm text-secondary-400">
								Connecting to RDP...
							</span>
						</div>
					</div>
				)}

				{showIdleHelp && (
					<div className="absolute inset-0 flex items-start justify-center bg-secondary-900/70 z-0 p-4 overflow-y-auto pointer-events-none">
						<div className="w-full flex justify-center pt-8 pointer-events-auto">
							<RequirementsChecklist
								open={requirementsOpen}
								onToggle={() => setRequirementsOpen((o) => !o)}
							/>
						</div>
					</div>
				)}

				<div
					ref={displayRef}
					// biome-ignore lint/a11y/noNoninteractiveTabindex: RDP display container needs focus for keyboard input capture
					tabIndex={0}
					className="flex-1 min-h-[200px] bg-black overflow-hidden outline-none"
					style={{ cursor: isConnected ? "default" : "not-allowed" }}
				/>
			</div>
		</div>
	);
};

export default RdpViewer;
