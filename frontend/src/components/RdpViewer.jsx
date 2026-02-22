import Guacamole from "guacamole-common-js";
import {
	Loader2,
	Maximize,
	Minimize,
	Monitor,
	MonitorOff,
	Play,
	Unplug,
	X,
} from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { authAPI } from "../utils/api";

const DEFAULT_DOMAIN_PLACEHOLDER = "WORKGROUP or computer name";
const isDev = import.meta.env.DEV;

const RdpViewer = ({ host, isOpen, onClose, embedded = false }) => {
	const [username, setUsername] = useState("");
	const [password, setPassword] = useState("");
	const [domain, setDomain] = useState("");
	const [isConnecting, setIsConnecting] = useState(false);
	const [isConnected, setIsConnected] = useState(false);
	const [error, setError] = useState("");
	const [session, setSession] = useState(null);
	const [isFullscreen, setIsFullscreen] = useState(false);

	const wrapperRef = useRef(null);
	const containerRef = useRef(null);
	const clientRef = useRef(null);
	const tunnelRef = useRef(null);
	const keyboardRef = useRef(null);
	const resizeObserverRef = useRef(null);

	const disconnect = useCallback(() => {
		try {
			if (keyboardRef.current) {
				keyboardRef.current.onkeydown = null;
				keyboardRef.current.onkeyup = null;
			}
			keyboardRef.current = null;

			if (clientRef.current) {
				clientRef.current.disconnect();
			}
			if (tunnelRef.current) {
				tunnelRef.current.disconnect();
			}
		} catch (_err) {
			/* best-effort cleanup */
		}

		if (resizeObserverRef.current) {
			resizeObserverRef.current.disconnect();
			resizeObserverRef.current = null;
		}

		clientRef.current = null;
		tunnelRef.current = null;
		setIsConnected(false);
		setIsConnecting(false);
	}, []);

	useEffect(() => {
		return () => disconnect();
	}, [disconnect]);

	useEffect(() => {
		const onFsChange = () => setIsFullscreen(!!document.fullscreenElement);
		document.addEventListener("fullscreenchange", onFsChange);
		return () => document.removeEventListener("fullscreenchange", onFsChange);
	}, []);

	const toggleFullscreen = useCallback(() => {
		if (!wrapperRef.current) return;
		if (document.fullscreenElement) {
			document.exitFullscreen();
		} else {
			wrapperRef.current.requestFullscreen();
		}
	}, []);

	useEffect(() => {
		if (!session || !containerRef.current) return;

		setError("");
		setIsConnecting(true);

		const baseUrl = session.guacamoleBaseUrl?.startsWith("http")
			? session.guacamoleBaseUrl
			: `${window.location.origin}${session.guacamoleBaseUrl?.startsWith("/") ? "" : "/"}${session.guacamoleBaseUrl || "guacamole"}`;

		const tunnel = new Guacamole.HTTPTunnel(`${baseUrl}/tunnel`);
		const client = new Guacamole.Client(tunnel);

		tunnelRef.current = tunnel;
		clientRef.current = client;

		const display = client.getDisplay();
		const displayElement = display.getElement();

		// Let Guacamole manage the display element's own dimensions.
		// Only set non-layout styles so it receives focus/clicks.
		displayElement.style.outline = "none";
		displayElement.style.cursor = "none";
		displayElement.tabIndex = 0;

		const container = containerRef.current;
		container.innerHTML = "";
		container.appendChild(displayElement);

		// Scale the Guacamole display to fit the container whenever either
		// the server-side resolution or the container size changes.
		const scaleToFit = () => {
			const dw = display.getWidth();
			const dh = display.getHeight();
			if (!dw || !dh || !container) return;
			const scale = Math.min(
				container.clientWidth / dw,
				container.clientHeight / dh,
			);
			display.scale(scale);
		};

		display.onresize = scaleToFit;

		const ro = new ResizeObserver(scaleToFit);
		ro.observe(container);
		resizeObserverRef.current = ro;

		// Input: mouse — use built-in applyDisplayScale for correct coordinate mapping
		const mouse = new Guacamole.Mouse(displayElement);
		const sendMouse = (e) => client.sendMouseState(e.state, true);
		mouse.onEach(["mousedown", "mousemove", "mouseup"], sendMouse);

		// Input: keyboard — attach to displayElement so it only captures when focused
		const keyboard = new Guacamole.Keyboard(displayElement);
		keyboardRef.current = keyboard;
		keyboard.onkeydown = (keysym) => client.sendKeyEvent(1, keysym);
		keyboard.onkeyup = (keysym) => client.sendKeyEvent(0, keysym);

		// Auto-focus the display so keyboard events register immediately
		displayElement.focus();

		// Error handlers
		client.onerror = (status) => {
			if (isDev) console.error("[RDP] client error", status);
			setError(status?.message || "RDP connection error");
			setIsConnecting(false);
			setIsConnected(false);
		};
		tunnel.onerror = (status) => {
			if (isDev) console.error("[RDP] tunnel error", status);
			setError(status?.message || "RDP tunnel error");
			setIsConnecting(false);
			setIsConnected(false);
		};

		client.onstatechange = (state) => {
			if (isDev) console.log("[RDP] client state →", state);
			if (state === 3) {
				setIsConnecting(false);
				setIsConnected(true);
			}
			if (state === 5) {
				setIsConnected(false);
			}
		};

		const connectParams = new URLSearchParams({
			token: session.authToken,
			GUAC_ID: session.connectionId,
			GUAC_TYPE: "c",
			GUAC_DATA_SOURCE: session.dataSource,
		}).toString();

		if (isDev)
			console.log("[RDP] connecting via HTTP tunnel", `${baseUrl}/tunnel`);
		client.connect(connectParams);

		return () => disconnect();
	}, [session, disconnect]);

	const startConnection = async (e) => {
		e.preventDefault();
		if (!host?.id) return;

		setError("");
		setIsConnecting(true);

		try {
			const response = await authAPI.rdpTicket(
				host.id,
				username.trim(),
				password,
				domain.trim(),
			);
			setSession(response.data);
		} catch (err) {
			const message =
				err.response?.data?.error ||
				err.response?.data?.message ||
				"Failed to start RDP session";
			setError(message);
			setIsConnecting(false);
		}
	};

	if (!host || !isOpen) return null;

	return (
		<div
			ref={wrapperRef}
			className="bg-secondary-900 rounded-lg w-full flex flex-col overflow-hidden"
			style={{
				height: isFullscreen
					? "100vh"
					: embedded
						? "calc(100vh - 14rem)"
						: "80vh",
			}}
		>
			<div className="flex items-center justify-between px-4 py-2 border-b border-secondary-700 flex-shrink-0">
				<div className="flex items-center gap-2">
					<Monitor className="h-4 w-4 text-primary-400" />
					<span className="text-sm font-medium text-white">
						{host?.friendly_name || host?.hostname || host?.ip}
					</span>
					{isConnected && (
						<span className="px-2 py-0.5 text-xs font-medium bg-green-900 text-green-200 rounded">
							Connected
						</span>
					)}
				</div>
				<div className="flex items-center gap-2">
					{isConnected && (
						<>
							<button
								type="button"
								onClick={toggleFullscreen}
								className="px-2 py-1 text-xs font-medium text-secondary-300 hover:text-white hover:bg-secondary-700 rounded transition-colors inline-flex items-center gap-1"
								title={isFullscreen ? "Exit fullscreen (Esc)" : "Fullscreen"}
							>
								{isFullscreen ? (
									<Minimize className="h-3.5 w-3.5" />
								) : (
									<Maximize className="h-3.5 w-3.5" />
								)}
							</button>
							<button
								type="button"
								onClick={() => {
									disconnect();
									setSession(null);
								}}
								className="px-2 py-1 text-xs font-medium text-secondary-300 hover:text-white hover:bg-secondary-700 rounded transition-colors inline-flex items-center gap-1"
							>
								<Unplug className="h-3.5 w-3.5" />
								Disconnect
							</button>
						</>
					)}
					{!isFullscreen && (
						<button
							type="button"
							onClick={onClose}
							className="text-secondary-400 hover:text-white transition-colors"
						>
							<X className="h-4 w-4" />
						</button>
					)}
				</div>
			</div>

			{!session && (
				<form
					onSubmit={startConnection}
					className="p-4 border-b border-secondary-700"
				>
					<div className="grid grid-cols-1 md:grid-cols-3 gap-3 mb-3">
						<div>
							<label className="block text-xs text-secondary-300 mb-1">
								Username
							</label>
							<input
								type="text"
								value={username}
								onChange={(e) => setUsername(e.target.value)}
								className="w-full px-3 py-2 text-sm bg-secondary-800 border border-secondary-500 rounded text-white shadow-inner focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
								placeholder="Administrator"
								required
							/>
						</div>
						<div>
							<label className="block text-xs text-secondary-300 mb-1">
								Password
							</label>
							<input
								type="password"
								value={password}
								onChange={(e) => setPassword(e.target.value)}
								className="w-full px-3 py-2 text-sm bg-secondary-800 border border-secondary-500 rounded text-white shadow-inner focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
								required
							/>
						</div>
						<div>
							<label className="block text-xs text-secondary-300 mb-1">
								Domain (optional)
							</label>
							<input
								type="text"
								value={domain}
								onChange={(e) => setDomain(e.target.value)}
								className="w-full px-3 py-2 text-sm bg-secondary-800 border border-secondary-500 rounded text-white shadow-inner focus:outline-none focus:ring-2 focus:ring-primary-500 focus:border-primary-500"
								placeholder={DEFAULT_DOMAIN_PLACEHOLDER}
							/>
						</div>
					</div>

					{error && (
						<div className="mb-3 text-xs text-red-300 bg-red-900/30 border border-red-700 rounded p-2">
							{error}
						</div>
					)}

					<button
						type="submit"
						disabled={isConnecting}
						className="px-4 py-2 text-sm bg-primary-600 hover:bg-primary-700 text-white rounded inline-flex items-center gap-2 disabled:opacity-60"
					>
						{isConnecting ? (
							<>
								<Loader2 className="h-4 w-4 animate-spin" />
								Connecting...
							</>
						) : (
							<>
								<Play className="h-4 w-4" />
								Connect RDP
							</>
						)}
					</button>
				</form>
			)}

			<div className="flex-1 min-h-0 p-3">
				{session ? (
					<div
						ref={containerRef}
						className="w-full h-full bg-black rounded overflow-hidden"
						style={{ position: "relative" }}
					/>
				) : (
					<div className="w-full h-full bg-secondary-950 rounded flex items-center justify-center">
						<div className="text-center text-secondary-400">
							<MonitorOff className="h-8 w-8 mx-auto mb-2 opacity-70" />
							<p className="text-sm">Enter Windows credentials to start RDP.</p>
						</div>
					</div>
				)}
			</div>
		</div>
	);
};

export default RdpViewer;
