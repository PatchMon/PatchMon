/**
 * RdpViewer - In-browser RDP for Windows hosts via Apache Guacamole (guacd)
 *
 * Uses guacamole-common-js with WebSocket tunnel to the PatchMon server.
 * Backend runs guacd as subprocess and bridges agent RDP proxy to guacd.
 *
 * When guacd is not available, shows an informational message.
 */

import Guacamole from "guacamole-common-js";
import { Maximize2, Minimize2, Monitor, RefreshCw } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { rdpAPI } from "../utils/api";

const RDP_GUACD_MESSAGE =
	"RDP requires guacd to be installed on the server. Install with: apt install guacd";

const RdpViewer = ({ host, isOpen }) => {
	const displayRef = useRef(null);
	const clientRef = useRef(null);
	const tunnelRef = useRef(null);
	const mouseRef = useRef(null);
	const keyboardRef = useRef(null);

	const [isConnecting, setIsConnecting] = useState(false);
	const [isConnected, setIsConnected] = useState(false);
	const [error, setError] = useState(null);
	const [isFullscreen, setIsFullscreen] = useState(false);
	const [credentials, setCredentials] = useState({
		username: "",
		password: "",
	});

	const disconnect = useCallback(() => {
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

	const connect = useCallback(async () => {
		if (!host?.id || !isOpen) return;

		disconnect();
		setError(null);
		setIsConnecting(true);

		let ticketData;
		try {
			const res = await rdpAPI.createTicket({
				hostId: host.id,
				username: credentials.username || undefined,
				password: credentials.password || undefined,
			});
			ticketData = res.data;
		} catch (err) {
			setIsConnecting(false);
			const status = err.response?.status;
			const msg = err.response?.data?.error || err.message;
			if (status === 404 || msg?.toLowerCase().includes("guacd")) {
				setError(RDP_GUACD_MESSAGE);
			} else {
				setError(msg || "Failed to create RDP ticket");
			}
			return;
		}

		const {
			ticket,
			websocketTunnelUrl,
			width = 1024,
			height = 768,
		} = ticketData;
		if (!ticket || !websocketTunnelUrl) {
			setError("Invalid ticket response");
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
				const msg = status?.message || status?.code || "Connection failed";
				setError(String(msg));
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
				setError(status?.message || "RDP connection error");
				setIsConnecting(false);
				setIsConnected(false);
			};

			if (displayRef.current) {
				displayRef.current.innerHTML = "";
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
			}

			client.connect(connectData);
		} catch (err) {
			setError(err?.message || "Failed to connect");
			setIsConnecting(false);
			disconnect();
		}
	}, [
		host?.id,
		isOpen,
		credentials.username,
		credentials.password,
		disconnect,
	]);

	const toggleFullscreen = () => {
		const container = displayRef.current?.closest(".rdp-viewer-container");
		if (!container) return;
		if (!document.fullscreenElement) {
			container.requestFullscreen?.();
			setIsFullscreen(true);
		} else {
			document.exitFullscreen?.();
			setIsFullscreen(false);
		}
	};

	useEffect(() => {
		const handler = () => {
			setIsFullscreen(!!document.fullscreenElement);
		};
		document.addEventListener("fullscreenchange", handler);
		return () => document.removeEventListener("fullscreenchange", handler);
	}, []);

	if (!host || !isOpen) return null;

	return (
		<div className="rdp-viewer-container flex flex-col h-full min-h-[300px] bg-secondary-800 rounded-lg overflow-hidden">
			<div className="flex items-center justify-between px-4 py-3 border-b border-secondary-600 shrink-0">
				<div className="flex items-center gap-2">
					<Monitor className="h-5 w-5 text-primary-500" />
					<span className="font-medium text-white">
						{host?.friendly_name || host?.hostname || host?.ip}
					</span>
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
								className="px-2 py-1 text-sm rounded bg-secondary-700 text-white border border-secondary-600 w-32"
							/>
						</div>
					)}
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
						{isConnecting ? "Connecting…" : "Connect"}
					</button>
					{isConnected && (
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
					)}
				</div>
			</div>

			<div className="flex-1 min-h-0 flex flex-col relative">
				{error && (
					<div className="absolute inset-0 flex items-center justify-center bg-secondary-900/90 z-10 p-4">
						<div className="text-center max-w-md">
							<Monitor className="h-12 w-12 text-secondary-500 mx-auto mb-3" />
							<p className="text-sm text-secondary-300">{error}</p>
							{error === RDP_GUACD_MESSAGE && (
								<p className="mt-2 text-xs text-secondary-500">
									See WINDOWS.md for RDP setup instructions.
								</p>
							)}
						</div>
					</div>
				)}

				{isConnecting && !error && (
					<div className="absolute inset-0 flex items-center justify-center bg-secondary-900/80 z-10">
						<div className="flex flex-col items-center gap-2">
							<RefreshCw className="h-8 w-8 text-primary-500 animate-spin" />
							<span className="text-sm text-secondary-400">
								Connecting to RDP…
							</span>
						</div>
					</div>
				)}

				<div
					ref={displayRef}
					className="flex-1 min-h-[200px] bg-black overflow-hidden"
					style={{ cursor: isConnected ? "default" : "not-allowed" }}
				/>
			</div>
		</div>
	);
};

export default RdpViewer;
