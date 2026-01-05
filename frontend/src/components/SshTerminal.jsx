import { useEffect, useRef, useState, useCallback } from "react";
import { Terminal } from "xterm";
import { FitAddon } from "xterm-addon-fit";
import "xterm/css/xterm.css";
import { X, Loader2, TerminalSquare, Download, Copy, ChevronDown, Bot, Send, PanelRightClose, PanelRightOpen, Sparkles } from "lucide-react";
// Note: Auth is handled via httpOnly cookies - no need for useAuth token
import { useSidebar } from "../contexts/SidebarContext";
import { useQuery } from "@tanstack/react-query";
import { settingsAPI, aiAPI } from "../utils/api";

const SshTerminal = ({ host, isOpen, onClose, embedded = false }) => {
	const { setSidebarCollapsed, sidebarCollapsed } = useSidebar();
	const previousSidebarStateRef = useRef(null);
	const terminalRef = useRef(null);
	const terminalInstanceRef = useRef(null);
	const fitAddonRef = useRef(null);
	const wsRef = useRef(null);
	const reconnectTimeoutRef = useRef(null);
	const idleTimeoutRef = useRef(null);
	const idleWarningTimeoutRef = useRef(null);

	const [isConnecting, setIsConnecting] = useState(false);
	const [isConnected, setIsConnected] = useState(false);
	const [error, setError] = useState(null);
	const [idleWarning, setIdleWarning] = useState(false);
	const IDLE_TIMEOUT_MS = 15 * 60 * 1000; // 15 minutes
	const IDLE_WARNING_MS = 1 * 60 * 1000; // 1 minute warning before disconnect
	const [showInstallCommands, setShowInstallCommands] = useState(false);

	// AI Assistant state
	const [aiPanelOpen, setAiPanelOpen] = useState(false);
	const [aiMessages, setAiMessages] = useState([]);
	const [aiInput, setAiInput] = useState("");
	const [aiLoading, setAiLoading] = useState(false);
	const [commandSuggestion, setCommandSuggestion] = useState("");
	const [currentInput, setCurrentInput] = useState("");
	const terminalBufferRef = useRef("");
	const completionTimeoutRef = useRef(null);
	const aiInputRef = useRef(null);

	// Load cached username from localStorage, keyed by host ID for per-host caching
	const getCachedUsername = () => {
		if (!host?.id) return "root";
		try {
			const cached = localStorage.getItem(`ssh_username_${host.id}`);
			return cached || "root";
		} catch {
			return "root";
		}
	};

	const [sshConfig, setSshConfig] = useState({
		username: "root", // Will be updated in useEffect when host is available
		password: "",
		privateKey: "",
		passphrase: "",
		port: 22,
		authMethod: "password", // "password" or "key"
	});

	// Load cached username when host changes
	useEffect(() => {
		if (host?.id) {
			const cachedUsername = getCachedUsername();
			setSshConfig(prev => ({ ...prev, username: cachedUsername }));
		}
	}, [host?.id]);

	// Save username to localStorage when it changes (debounced on blur/connect)
	const saveUsername = (username) => {
		if (host?.id && username) {
			try {
				localStorage.setItem(`ssh_username_${host.id}`, username);
			} catch {
				// localStorage might be full or disabled
			}
		}
	};

	// Fetch server URL and settings for agent install command
	const { data: serverUrlData } = useQuery({
		queryKey: ["serverUrl"],
		queryFn: () => settingsAPI.getServerUrl().then((res) => res.data),
	});

	const { data: settings } = useQuery({
		queryKey: ["settings"],
		queryFn: () => settingsAPI.get().then((res) => res.data),
	});

	const serverUrl = serverUrlData?.server_url || "http://localhost:3001";
	const getCurlFlags = () => {
		return settings?.ignore_ssl_self_signed ? "-sk" : "-s";
	};

	// Fetch AI status (available to all authenticated users)
	const { data: aiStatus } = useQuery({
		queryKey: ["aiStatus"],
		queryFn: () => aiAPI.getStatus().then((res) => res.data),
		staleTime: 60000, // Cache for 1 minute
	});

	const aiEnabled = aiStatus?.ai_enabled && aiStatus?.ai_api_key_set;

	// Get recent terminal output for AI context
	const getTerminalContext = useCallback(() => {
		return terminalBufferRef.current.slice(-3000); // Last ~3000 chars
	}, []);

	// Send message to AI assistant
	const sendAiMessage = async () => {
		if (!aiInput.trim() || aiLoading || !aiEnabled) return;

		const userMessage = aiInput.trim();
		setAiInput("");
		setAiMessages((prev) => [...prev, { role: "user", content: userMessage }]);
		setAiLoading(true);

		try {
			const response = await aiAPI.assist({
				question: userMessage,
				context: getTerminalContext(),
				history: aiMessages.slice(-10), // Last 10 messages for context
			});

			setAiMessages((prev) => [
				...prev,
				{ role: "assistant", content: response.data.response },
			]);
		} catch (err) {
			setAiMessages((prev) => [
				...prev,
				{
					role: "assistant",
					content: `Error: ${err.response?.data?.error || "Failed to get AI response"}`,
					isError: true,
				},
			]);
		} finally {
			setAiLoading(false);
		}
	};

	// Get command completion suggestion (debounced)
	const getCommandCompletion = useCallback(
		async (input) => {
			if (!aiEnabled || input.length < 3) {
				setCommandSuggestion("");
				return;
			}

			try {
				const response = await aiAPI.complete({
					input,
					context: getTerminalContext(),
				});
				if (response.data.completion) {
					setCommandSuggestion(response.data.completion);
				} else {
					setCommandSuggestion("");
				}
			} catch {
				setCommandSuggestion("");
			}
		},
		[aiEnabled, getTerminalContext]
	);

	// Handle current input change for completion
	const handleInputChange = useCallback(
		(input) => {
			setCurrentInput(input);

			// Clear existing timeout
			if (completionTimeoutRef.current) {
				clearTimeout(completionTimeoutRef.current);
			}

			// Debounce completion requests
			completionTimeoutRef.current = setTimeout(() => {
				getCommandCompletion(input);
			}, 300);
		},
		[getCommandCompletion]
	);

	// Accept command suggestion
	const acceptSuggestion = useCallback(() => {
		if (commandSuggestion && wsRef.current?.readyState === WebSocket.OPEN) {
			wsRef.current.send(
				JSON.stringify({
					type: "input",
					data: commandSuggestion,
				})
			);
			setCommandSuggestion("");
			setCurrentInput("");
		}
	}, [commandSuggestion]);

	// Clear suggestion
	const clearSuggestion = useCallback(() => {
		setCommandSuggestion("");
	}, []);

	// Build agent install command
	const getInstallCommand = () => {
		if (!host?.api_id || !host?.api_key) return "";
		const curlFlags = getCurlFlags();
		const installUrl = `${serverUrl}/api/v1/hosts/install`;
		return `curl ${curlFlags} ${installUrl} -H "X-API-ID: ${host.api_id}" -H "X-API-KEY: ${host.api_key}" | sh`;
	};

	// Paste command to terminal
	const pasteToTerminal = (command) => {
		if (wsRef.current?.readyState === WebSocket.OPEN && terminalInstanceRef.current) {
			// Send the command followed by Enter
			wsRef.current.send(
				JSON.stringify({
					type: "input",
					data: command + "\r",
				}),
			);
			setShowInstallCommands(false);
		}
	};

	// Copy command to clipboard
	const copyCommand = async (command) => {
		try {
			if (navigator.clipboard && window.isSecureContext) {
				await navigator.clipboard.writeText(command);
			} else {
				const textArea = document.createElement("textarea");
				textArea.value = command;
				textArea.style.position = "fixed";
				textArea.style.left = "-999999px";
				document.body.appendChild(textArea);
				textArea.select();
				document.execCommand("copy");
				document.body.removeChild(textArea);
			}
		} catch (err) {
			console.error("Failed to copy:", err);
		}
	};

	// Initialize terminal
	useEffect(() => {
		// Only initialize when terminal container is rendered (when connecting or connected)
		// In embedded mode, only when connecting/connected
		// In modal mode, only when isOpen and connecting/connected
		if ((!embedded && !isOpen) || !terminalRef.current || (!isConnected && !isConnecting)) return;

		// Create terminal instance
		const term = new Terminal({
			cursorBlink: false, // Disable cursor blink in preview mode
			cursorStyle: "block",
			fontFamily: '"Courier New", monospace',
			fontSize: 14,
			theme: {
				background: "#1e1e1e",
				foreground: "#d4d4d4",
				cursor: "#aeafad",
				cursorAccent: "#1e1e1e", // Make cursor blend with background when not connected
				selection: "#264f78",
				black: "#000000",
				red: "#cd3131",
				green: "#0dbc79",
				yellow: "#e5e510",
				blue: "#2472c8",
				magenta: "#bc3fbc",
				cyan: "#11a8cd",
				white: "#e5e5e5",
				brightBlack: "#666666",
				brightRed: "#f14c4c",
				brightGreen: "#23d18b",
				brightYellow: "#f5f543",
				brightBlue: "#3b8eea",
				brightMagenta: "#d670d6",
				brightCyan: "#29b8db",
				brightWhite: "#e5e5e5",
			},
		});

		const fitAddon = new FitAddon();
		term.loadAddon(fitAddon);
		term.open(terminalRef.current);

		// Fit terminal to container
		fitAddon.fit();

		// Terminal is ready, will be used when connection is established

		terminalInstanceRef.current = term;
		fitAddonRef.current = fitAddon;

		// Handle window resize
		const handleResize = () => {
			if (fitAddonRef.current) {
				fitAddonRef.current.fit();
				// Send resize event to SSH session if connected
				if (wsRef.current?.readyState === WebSocket.OPEN && isConnected) {
					const dimensions = fitAddonRef.current.proposeDimensions();
					wsRef.current.send(
						JSON.stringify({
							type: "resize",
							cols: dimensions?.cols || 80,
							rows: dimensions?.rows || 24,
						}),
					);
				}
			}
		};

		window.addEventListener("resize", handleResize);

		return () => {
			window.removeEventListener("resize", handleResize);
			term.dispose();
		};
	}, [isOpen, isConnected]);

	// Connect to SSH via WebSocket
	const connectSsh = async () => {
		if (!host) return;

		// Close existing WebSocket connection if any
		if (wsRef.current) {
			console.log("[SSH Terminal] Closing existing WebSocket connection");
			wsRef.current.close();
			wsRef.current = null;
		}

		// Validate credentials based on auth method
		if (sshConfig.authMethod === "password" && !sshConfig.password) {
			setError("Password is required");
			return;
		}
		if (sshConfig.authMethod === "key" && !sshConfig.privateKey) {
			setError("Private key is required");
			return;
		}

		setIsConnecting(true);
		setError(null);

		// SECURITY: Use one-time ticket authentication instead of tokens in URLs
		// Tickets are single-use, short-lived, and host-specific
		// This prevents token exposure in server logs and browser history
		let sshTicket;
		try {
			const response = await fetch("/api/v1/auth/ssh-ticket", {
				method: "POST",
				credentials: "include", // Include httpOnly cookies for auth
				headers: {
					"Content-Type": "application/json",
				},
				body: JSON.stringify({ hostId: host.id }),
			});
			if (!response.ok) {
				const errorData = await response.json().catch(() => ({}));
				throw new Error(errorData.error || "Failed to get SSH ticket");
			}
			const data = await response.json();
			sshTicket = data.ticket;
		} catch (err) {
			console.error("[SSH Terminal] Failed to get SSH ticket:", err);
			setError("Authentication required. Please log in again.");
			if (terminalInstanceRef.current) {
				terminalInstanceRef.current.writeln("\r\n\x1b[31m✗ Error: Authentication required\x1b[0m");
				terminalInstanceRef.current.writeln("\x1b[33mPlease log in again and try connecting.\x1b[0m");
			}
			setIsConnecting(false);
			return;
		}

		// Determine WebSocket URL - using ticket instead of token for security
		const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
		const hostname = window.location.hostname;
		const port = window.location.port ? `:${window.location.port}` : "";
		const wsUrl = `${protocol}//${hostname}${port}/api/v1/ssh-terminal/${host.id}?ticket=${encodeURIComponent(sshTicket)}`;

		try {
			const ws = new WebSocket(wsUrl);

			ws.onopen = () => {
				console.log("[SSH Terminal] WebSocket connected");
				if (terminalInstanceRef.current) {
					terminalInstanceRef.current.writeln("\x1b[32m✓ WebSocket connected\x1b[0m");
					terminalInstanceRef.current.writeln("\x1b[33mEstablishing SSH connection...\x1b[0m");
				}
				
				// Ensure WebSocket is ready before sending (Firefox may need this)
				if (ws.readyState === WebSocket.OPEN) {
					// Send connect message with SSH credentials
					const connectData = {
						type: "connect",
						username: sshConfig.username,
						port: sshConfig.port || 22,
						terminal: "xterm-256color",
						cols: fitAddonRef.current?.proposeDimensions()?.cols || 80,
						rows: fitAddonRef.current?.proposeDimensions()?.rows || 24,
					};

					// Add authentication data based on selected method
					if (sshConfig.authMethod === "password") {
						connectData.password = sshConfig.password;
					} else if (sshConfig.authMethod === "key") {
						connectData.privateKey = sshConfig.privateKey;
						if (sshConfig.passphrase) {
							connectData.passphrase = sshConfig.passphrase;
						}
					}

					try {
						ws.send(JSON.stringify(connectData));
						console.log("[SSH Terminal] Connect message sent");
					} catch (err) {
						console.error("[SSH Terminal] Error sending connect message:", err);
						setError("Failed to send connection request: " + err.message);
					}
				} else {
					console.error("[SSH Terminal] WebSocket not ready, state:", ws.readyState);
					setError("WebSocket connection not ready");
				}
			};

			ws.onmessage = (event) => {
				try {
					const message = JSON.parse(event.data);

					switch (message.type) {
						case "connected":
							console.log("[SSH Terminal] SSH connection established");
							if (terminalInstanceRef.current) {
								terminalInstanceRef.current.write("\x1b[?25h"); // Show cursor when connected
								terminalInstanceRef.current.writeln("\x1b[32m✓ SSH connection established\x1b[0m");
								terminalInstanceRef.current.writeln("");
							}
							setIsConnecting(false);
							setIsConnected(true);
							setError(null);
							// Cache the username for future connections
							saveUsername(sshConfig.username);
							// Resize terminal to fit expanded container
							setTimeout(() => {
								if (fitAddonRef.current) {
									fitAddonRef.current.fit();
								}
							}, 100);
							break;

						case "data":
							// Write data to terminal
							if (terminalInstanceRef.current) {
								terminalInstanceRef.current.write(message.data);
								// Capture terminal output for AI context (keep last 5000 chars)
								terminalBufferRef.current = (terminalBufferRef.current + message.data).slice(-5000);
								// Reset idle timeout on terminal activity
								resetIdleTimeout();
							}
							break;

						case "error":
							setError(message.message || "SSH connection error");
							setIsConnecting(false);
							setIsConnected(false);
							if (terminalInstanceRef.current) {
								terminalInstanceRef.current.writeln(
									`\r\n\x1b[31mError: ${message.message}\x1b[0m`,
								);
							}
							break;

						case "closed":
							setIsConnected(false);
							if (terminalInstanceRef.current) {
								terminalInstanceRef.current.writeln(
									"\r\n\x1b[33mSSH connection closed\x1b[0m",
								);
							}
							break;

						default:
							console.warn("[SSH Terminal] Unknown message type:", message.type);
					}
				} catch (err) {
					console.error("[SSH Terminal] Error parsing message:", err);
				}
			};

			ws.onerror = (err) => {
				console.error("[SSH Terminal] WebSocket error:", err);
				const errorMsg = "Failed to connect to terminal server. Check browser console and ensure backend is running.";
				setError(errorMsg);
				if (terminalInstanceRef.current) {
					terminalInstanceRef.current.writeln(`\r\n\x1b[31m✗ WebSocket Error:\x1b[0m ${errorMsg}`);
					terminalInstanceRef.current.writeln("\x1b[33mCheck browser console (F12) for details.\x1b[0m");
				}
				setIsConnecting(false);
				setIsConnected(false);
			};

			ws.onclose = (event) => {
				console.log("[SSH Terminal] WebSocket closed", event.code, event.reason);
				const wasConnected = isConnected;
				setIsConnected(false);
				setIsConnecting(false);
				wsRef.current = null;

				// Check for authentication errors (1006 = abnormal closure, often means auth failed)
				if (event.code === 1006 && !wasConnected) {
					const errorMsg = "Connection failed: Session may have expired. Please refresh the page or log in again.";
					setError(errorMsg);
					if (terminalInstanceRef.current) {
						terminalInstanceRef.current.writeln(`\r\n\x1b[31m✗ ${errorMsg}\x1b[0m`);
					}
				} else if (terminalInstanceRef.current && wasConnected) {
					terminalInstanceRef.current.writeln(`\r\n\x1b[33m⚠ WebSocket closed (code: ${event.code})\x1b[0m`);
				}

				// Attempt to reconnect if we were connected and not manually closed
				// Don't reconnect on auth errors (1006, 1008) unless we were already connected
				if (wasConnected && isOpen && event.code !== 1000 && event.code !== 1006 && event.code !== 1008) {
					reconnectTimeoutRef.current = setTimeout(() => {
						console.log("[SSH Terminal] Attempting to reconnect...");
						if (terminalInstanceRef.current) {
							terminalInstanceRef.current.writeln("\x1b[33mAttempting to reconnect...\x1b[0m");
						}
						connectSsh();
					}, 3000);
				}
			};

			wsRef.current = ws;
		} catch (err) {
			console.error("[SSH Terminal] Connection error:", err);
			const errorMsg = err.message || "Failed to establish connection";
			setError(errorMsg);
			if (terminalInstanceRef.current) {
				terminalInstanceRef.current.writeln(`\r\n\x1b[31m✗ Connection Error:\x1b[0m ${errorMsg}`);
			}
			setIsConnecting(false);
		}
	};

	// Close install commands dropdown when clicking outside
	useEffect(() => {
		if (!showInstallCommands) return;

		const handleClickOutside = (event) => {
			// Check if click is outside the dropdown and button
			const target = event.target;
			const installCommandsEl = target.closest('[data-install-commands]');
			if (!installCommandsEl) {
				setShowInstallCommands(false);
			}
		};

		document.addEventListener("mousedown", handleClickOutside);
		return () => document.removeEventListener("mousedown", handleClickOutside);
	}, [showInstallCommands]);

	// Reset idle timeout on activity
	const resetIdleTimeout = () => {
		if (!isConnected) return;

		// Clear existing timeouts
		if (idleTimeoutRef.current) {
			clearTimeout(idleTimeoutRef.current);
			idleTimeoutRef.current = null;
		}
		if (idleWarningTimeoutRef.current) {
			clearTimeout(idleWarningTimeoutRef.current);
			idleWarningTimeoutRef.current = null;
		}

		// Clear warning state
		setIdleWarning(false);

		// Set warning timeout (1 minute before disconnect)
		idleWarningTimeoutRef.current = setTimeout(() => {
			setIdleWarning(true);
			if (terminalInstanceRef.current) {
				terminalInstanceRef.current.writeln("\r\n\x1b[33m⚠ Warning: Connection will close in 1 minute due to inactivity.\x1b[0m");
			}
		}, IDLE_TIMEOUT_MS - IDLE_WARNING_MS);

		// Set disconnect timeout
		idleTimeoutRef.current = setTimeout(() => {
			if (terminalInstanceRef.current) {
				terminalInstanceRef.current.writeln("\r\n\x1b[31m✗ Connection closed due to 30 minutes of inactivity.\x1b[0m");
			}
			handleDisconnect();
		}, IDLE_TIMEOUT_MS);
	};

	// Handle terminal input with AI completion support
	useEffect(() => {
		if (!terminalInstanceRef.current || !isConnected) return;

		const term = terminalInstanceRef.current;
		let currentLine = "";

		const handleData = (data) => {
			if (wsRef.current?.readyState === WebSocket.OPEN) {
				// Handle Tab key for AI completion
				if (data === "\t" && commandSuggestion && aiEnabled) {
					// Accept AI suggestion
					wsRef.current.send(
						JSON.stringify({
							type: "input",
							data: commandSuggestion,
						})
					);
					setCommandSuggestion("");
					currentLine = "";
					setCurrentInput("");
					resetIdleTimeout();
					return;
				}

				// Handle Escape key to dismiss suggestion
				if (data === "\x1b" && commandSuggestion) {
					setCommandSuggestion("");
					return;
				}

				// Track current line for completion
				if (data === "\r" || data === "\n") {
					// Enter pressed - clear current line
					currentLine = "";
					setCommandSuggestion("");
					setCurrentInput("");
				} else if (data === "\x7f" || data === "\b") {
					// Backspace - remove last char
					currentLine = currentLine.slice(0, -1);
					handleInputChange(currentLine);
				} else if (data.length === 1 && data.charCodeAt(0) >= 32) {
					// Printable character
					currentLine += data;
					handleInputChange(currentLine);
				}

				wsRef.current.send(
					JSON.stringify({
						type: "input",
						data: data,
					}),
				);
				// Reset idle timeout on input
				resetIdleTimeout();
			}
		};

		// onData returns a disposable that we can dispose to remove the listener
		const disposable = term.onData(handleData);

		return () => {
			if (disposable && typeof disposable.dispose === 'function') {
				disposable.dispose();
			}
		};
	}, [isConnected, commandSuggestion, aiEnabled, handleInputChange]);

	// Set up idle timeout when connected, reset on terminal data
	useEffect(() => {
		if (isConnected) {
			resetIdleTimeout();
		} else {
			// Clear timeouts when disconnected
			if (idleTimeoutRef.current) {
				clearTimeout(idleTimeoutRef.current);
				idleTimeoutRef.current = null;
			}
			if (idleWarningTimeoutRef.current) {
				clearTimeout(idleWarningTimeoutRef.current);
				idleWarningTimeoutRef.current = null;
			}
			setIdleWarning(false);
		}

		return () => {
			if (idleTimeoutRef.current) {
				clearTimeout(idleTimeoutRef.current);
			}
			if (idleWarningTimeoutRef.current) {
				clearTimeout(idleWarningTimeoutRef.current);
			}
		};
	}, [isConnected]);

	// Reset timeout when receiving terminal data
	useEffect(() => {
		if (!isConnected || !terminalInstanceRef.current) return;

		const term = terminalInstanceRef.current;
		// Monitor for any terminal activity to reset timeout
		const handleActivity = () => {
			resetIdleTimeout();
		};

		// Reset timeout when receiving data
		// This is handled via the WebSocket message handler below
		// We'll call resetIdleTimeout from there

		return () => {
			// Cleanup if needed
		};
	}, [isConnected]);

	// Collapse sidebar when SSH terminal opens (only in modal mode), restore when it closes
	useEffect(() => {
		if (!embedded) {
			// Only collapse sidebar for modal mode
			if (isOpen) {
				// Store current sidebar state before collapsing
				previousSidebarStateRef.current = sidebarCollapsed;
				// Collapse sidebar if it's not already collapsed
				if (!sidebarCollapsed) {
					setSidebarCollapsed(true);
				}
			} else {
				// Restore previous sidebar state when terminal closes
				if (previousSidebarStateRef.current !== null) {
					setSidebarCollapsed(previousSidebarStateRef.current);
					previousSidebarStateRef.current = null;
				}
			}
		}
	}, [isOpen, sidebarCollapsed, setSidebarCollapsed, embedded]);

	// Cleanup on close - but preserve state when switching tabs in embedded mode
	useEffect(() => {
		if (!isOpen && !embedded) {
			// Only fully cleanup in modal mode when actually closed
			if (wsRef.current) {
				wsRef.current.close();
				wsRef.current = null;
			}
			if (reconnectTimeoutRef.current) {
				clearTimeout(reconnectTimeoutRef.current);
				reconnectTimeoutRef.current = null;
			}
			setIsConnected(false);
			setIsConnecting(false);
			setError(null);
		}
		// In embedded mode, we never cleanup - connection stays alive when tab is hidden
	}, [isOpen, embedded]);

	// Handle disconnect
	const handleDisconnect = () => {
		// Clear idle timeouts
		if (idleTimeoutRef.current) {
			clearTimeout(idleTimeoutRef.current);
			idleTimeoutRef.current = null;
		}
		if (idleWarningTimeoutRef.current) {
			clearTimeout(idleWarningTimeoutRef.current);
			idleWarningTimeoutRef.current = null;
		}
		setIdleWarning(false);

		if (wsRef.current) {
			wsRef.current.send(JSON.stringify({ type: "disconnect" }));
			wsRef.current.close();
			wsRef.current = null;
		}
		setIsConnected(false);
		setIsConnecting(false);
		// Clear sensitive credentials from memory
		setSshConfig((prev) => ({
			...prev,
			password: "",
			privateKey: "",
			passphrase: "",
		}));
	};

	const handleClose = () => {
		handleDisconnect();
		onClose();
	};

	if (!isOpen) return null;

	// Embedded mode - render inline without modal overlay
	if (embedded) {
		return (
			<div className="bg-secondary-900 rounded-lg w-full flex flex-col" style={{ minHeight: isConnected || isConnecting ? "750px" : "auto", maxHeight: "calc(100vh - 150px)" }}>
				{/* Compact Header */}
				<div className="flex items-center justify-between px-4 py-2 border-b border-secondary-700 flex-shrink-0">
					<div className="flex items-center gap-2">
						<TerminalSquare className="h-4 w-4 text-primary-400" />
						<span className="text-sm font-medium text-white">
							{host?.friendly_name || host?.ip || host?.hostname}
						</span>
						{host?.ip && (
							<span className="text-xs text-secondary-400">
								({host.ip})
							</span>
						)}
					</div>
					<div className="flex items-center gap-2 relative">
						{isConnected && (
							<>
								{idleWarning && (
									<span className="px-2 py-0.5 text-xs font-medium bg-yellow-900 text-yellow-200 rounded animate-pulse">
										Idle - Closing soon
									</span>
								)}
								<span className="px-2 py-0.5 text-xs font-medium bg-green-900 text-green-200 rounded">
									Connected
								</span>
								{host?.api_id && host?.api_key && (
									<div className="relative" data-install-commands>
										<button
											type="button"
											onClick={() => setShowInstallCommands(!showInstallCommands)}
											className="px-2 py-0.5 text-xs font-medium text-secondary-300 hover:text-white hover:bg-secondary-700 rounded transition-colors flex items-center gap-1"
											title={host?.agent_version ? "Reinstall Agent" : "Install Agent"}
										>
											<Download className="h-3 w-3" />
											{host?.agent_version ? "Reinstall Agent" : "Install Agent"}
											<ChevronDown className="h-3 w-3" />
										</button>
										{showInstallCommands && (
											<div className="absolute right-0 top-full mt-1 bg-secondary-800 border border-secondary-600 rounded-lg shadow-lg z-50 w-96 p-3" data-install-commands>
												<div className="text-xs font-medium text-white mb-2">
													{host?.agent_version ? "Reinstall PatchMonEnhanced Agent" : "Install PatchMonEnhanced Agent"}
												</div>
												<div className="flex gap-1">
													<code className="flex-1 px-2 py-1 text-xs bg-secondary-900 text-secondary-200 rounded border border-secondary-700 font-mono break-all">
														{getInstallCommand()}
													</code>
													<button
														type="button"
														onClick={() => {
															copyCommand(getInstallCommand());
															setShowInstallCommands(false);
														}}
														className="px-2 py-1 text-xs bg-primary-600 hover:bg-primary-700 text-white rounded flex items-center gap-1"
														title="Copy"
													>
														<Copy className="h-3 w-3" />
													</button>
													<button
														type="button"
														onClick={() => {
															pasteToTerminal(getInstallCommand());
														}}
														className="px-2 py-1 text-xs bg-secondary-700 hover:bg-secondary-600 text-white rounded"
														title="Paste to Terminal"
													>
														Paste
													</button>
												</div>
											</div>
										)}
									</div>
								)}
								<button
									type="button"
									onClick={handleDisconnect}
									className="px-2 py-0.5 text-xs font-medium text-secondary-300 hover:text-white hover:bg-secondary-700 rounded transition-colors"
									title="Disconnect"
								>
									Disconnect
								</button>
							</>
						)}
						{isConnecting && (
							<span className="px-2 py-0.5 text-xs font-medium bg-yellow-900 text-yellow-200 rounded flex items-center gap-1">
								<Loader2 className="h-3 w-3 animate-spin" />
								Connecting...
							</span>
						)}
						{error && (
							<span className="px-2 py-0.5 text-xs font-medium bg-red-900 text-red-200 rounded">
								Error
							</span>
						)}
					</div>
				</div>

				{/* Connection Form (shown when not connected) */}
				{!isConnected && !isConnecting && (
					<div className="border-b border-secondary-700 bg-secondary-800/50 flex-shrink-0 overflow-y-auto" style={{ maxHeight: "40%" }}>
						<div className="p-5 max-w-2xl mx-auto space-y-4">
							{error && (
								<div className="p-2 bg-red-900/50 border border-red-700 rounded text-red-200 text-xs">
									{error}
								</div>
							)}
							
							{/* Authentication Method Toggle */}
							<div className="flex gap-6 mb-1">
								<label className="flex items-center gap-2 cursor-pointer group">
									<input
										type="radio"
										name="authMethod"
										value="password"
										checked={sshConfig.authMethod === "password"}
										onChange={(e) =>
											setSshConfig({ ...sshConfig, authMethod: e.target.value })
										}
										className="text-primary-600 focus:ring-primary-500"
									/>
									<span className="text-xs font-medium text-secondary-300 group-hover:text-secondary-200 transition-colors">Password</span>
								</label>
								<label className="flex items-center gap-2 cursor-pointer group">
									<input
										type="radio"
										name="authMethod"
										value="key"
										checked={sshConfig.authMethod === "key"}
										onChange={(e) =>
											setSshConfig({ ...sshConfig, authMethod: e.target.value })
										}
										className="text-primary-600 focus:ring-primary-500"
									/>
									<span className="text-xs font-medium text-secondary-300 group-hover:text-secondary-200 transition-colors">SSH Key</span>
								</label>
							</div>

							{/* Credentials Row - Username, Password, Port, Connect */}
							{sshConfig.authMethod === "password" && (
								<div className="flex gap-3 items-end">
									<div className="flex-1">
										<label className="block text-xs font-medium text-secondary-300 mb-1">
											Username
										</label>
										<input
											type="text"
											value={sshConfig.username}
											onChange={(e) =>
												setSshConfig({ ...sshConfig, username: e.target.value })
											}
											onBlur={(e) => saveUsername(e.target.value)}
											autoComplete="username"
											className="w-full px-3 py-2 text-sm bg-secondary-700 border border-secondary-600 rounded text-white focus:outline-none focus:ring-2 focus:ring-primary-500"
											placeholder="root"
											tabIndex={1}
										/>
									</div>
									<div className="flex-1">
										<label className="block text-xs font-medium text-secondary-300 mb-1">
											Password
										</label>
										<input
											type="password"
											value={sshConfig.password}
											onChange={(e) =>
												setSshConfig({ ...sshConfig, password: e.target.value })
											}
											onKeyDown={(e) => {
												if (e.key === "Enter" && sshConfig.password) {
													connectSsh();
												}
											}}
											className="w-full px-3 py-2 text-sm bg-secondary-700 border border-secondary-600 rounded text-white focus:outline-none focus:ring-2 focus:ring-primary-500"
											placeholder="Password"
											tabIndex={2}
										/>
									</div>
									<div className="w-20">
										<label className="block text-xs font-medium text-secondary-300 mb-1">
											Port
										</label>
										<input
											type="number"
											value={sshConfig.port}
											onChange={(e) =>
												setSshConfig({
													...sshConfig,
													port: parseInt(e.target.value, 10) || 22,
												})
											}
											className="w-full px-2 py-2 text-sm bg-secondary-700 border border-secondary-600 rounded text-white focus:outline-none focus:ring-2 focus:ring-primary-500"
											placeholder="22"
											tabIndex={3}
										/>
									</div>
									<button
										type="button"
										onClick={connectSsh}
										disabled={!sshConfig.username || !sshConfig.password}
										className="px-4 py-2 text-sm bg-primary-600 hover:bg-primary-700 text-white font-medium rounded transition-all disabled:opacity-50 disabled:cursor-not-allowed whitespace-nowrap shadow-sm hover:shadow disabled:shadow-none"
										tabIndex={4}
									>
										Connect
									</button>
								</div>
							)}
							
							{/* Username and Port Row for Key Auth */}
							{sshConfig.authMethod === "key" && (
								<div className="flex gap-3">
									<div className="flex-1">
										<label className="block text-xs font-medium text-secondary-300 mb-1">
											Username
										</label>
										<input
											type="text"
											value={sshConfig.username}
											onChange={(e) =>
												setSshConfig({ ...sshConfig, username: e.target.value })
											}
											onBlur={(e) => saveUsername(e.target.value)}
											autoComplete="username"
											className="w-full px-3 py-2 text-sm bg-secondary-700 border border-secondary-600 rounded text-white focus:outline-none focus:ring-2 focus:ring-primary-500"
											placeholder="root"
											tabIndex={1}
										/>
									</div>
									<div className="w-20">
										<label className="block text-xs font-medium text-secondary-300 mb-1">
											Port
										</label>
										<input
											type="number"
											value={sshConfig.port}
											onChange={(e) =>
												setSshConfig({
													...sshConfig,
													port: parseInt(e.target.value, 10) || 22,
												})
											}
											className="w-full px-2 py-2 text-sm bg-secondary-700 border border-secondary-600 rounded text-white focus:outline-none focus:ring-2 focus:ring-primary-500"
											placeholder="22"
											tabIndex={4}
										/>
									</div>
								</div>
							)}

							{/* SSH Key Authentication */}
							{sshConfig.authMethod === "key" && (
								<>
									<div>
										<label className="block text-xs font-medium text-secondary-300 mb-1">
											Private Key
										</label>
										<textarea
											value={sshConfig.privateKey}
											onChange={(e) =>
												setSshConfig({ ...sshConfig, privateKey: e.target.value })
											}
											className="w-full px-3 py-2 text-sm bg-secondary-700 border border-secondary-600 rounded text-white focus:outline-none focus:ring-2 focus:ring-primary-500 font-mono resize-none"
											placeholder="-----BEGIN OPENSSH PRIVATE KEY-----&#10;...&#10;-----END OPENSSH PRIVATE KEY-----"
											rows={4}
											tabIndex={2}
										/>
										<div className="mt-1 space-y-0.5">
											<p className="text-xs text-secondary-400">
												Paste your private SSH key here (supports encrypted keys)
											</p>
											<p className="text-xs text-secondary-500 italic">
												🔒 Security: Your private key is never stored. It's only used in memory for the SSH connection and cleared when you disconnect.
											</p>
										</div>
									</div>
									<div>
										<label className="block text-xs font-medium text-secondary-300 mb-1">
											Passphrase (if key is encrypted)
										</label>
										<input
											type="password"
											value={sshConfig.passphrase}
											onChange={(e) =>
												setSshConfig({ ...sshConfig, passphrase: e.target.value })
											}
											onKeyDown={(e) => {
												if (e.key === "Enter" && sshConfig.privateKey) {
													connectSsh();
												}
											}}
											className="w-full px-3 py-2 text-sm bg-secondary-700 border border-secondary-600 rounded text-white focus:outline-none focus:ring-2 focus:ring-primary-500"
											placeholder="Passphrase (optional)"
											tabIndex={3}
										/>
									</div>
								</>
							)}

							{/* Connect Button for Key Auth */}
							{sshConfig.authMethod === "key" && (
								<button
									type="button"
									onClick={connectSsh}
									disabled={!sshConfig.username || !sshConfig.privateKey}
									className="w-full px-4 py-2 text-sm bg-primary-600 hover:bg-primary-700 text-white font-medium rounded transition-all disabled:opacity-50 disabled:cursor-not-allowed shadow-sm hover:shadow disabled:shadow-none"
									tabIndex={5}
								>
									Connect
								</button>
							)}
						</div>
					</div>
				)}

				{/* Connecting indicator */}
				{isConnecting && (
					<div className="p-3 border-b border-secondary-700 bg-secondary-800 flex-shrink-0">
						<div className="flex items-center gap-2 text-sm">
							<Loader2 className="h-4 w-4 text-primary-400 animate-spin" />
							<span className="text-secondary-300">Connecting to SSH server...</span>
						</div>
					</div>
				)}

				{/* Terminal Container with AI Panel - Shown when connecting or connected */}
				{(isConnected || isConnecting) && (
					<div className="flex-1 overflow-hidden min-h-[500px] flex">
						{/* Main Terminal Area */}
						<div className={`flex-1 p-4 flex flex-col transition-all duration-300 ${aiPanelOpen ? "pr-2" : ""}`}>
							{/* Command Suggestion Overlay */}
							{commandSuggestion && aiEnabled && (
								<div className="mb-2 px-3 py-2 bg-primary-900/40 border border-primary-700/50 rounded-lg flex items-center justify-between">
									<div className="flex items-center gap-2">
										<Sparkles className="h-4 w-4 text-primary-400" />
										<span className="text-sm text-secondary-300">
											Suggestion: <span className="text-primary-300 font-mono">{currentInput}<span className="text-primary-400/60">{commandSuggestion}</span></span>
										</span>
									</div>
									<div className="flex items-center gap-2 text-xs text-secondary-400">
										<kbd className="px-1.5 py-0.5 bg-secondary-700 rounded">Tab</kbd>
										<span>accept</span>
										<kbd className="px-1.5 py-0.5 bg-secondary-700 rounded">Esc</kbd>
										<span>dismiss</span>
									</div>
								</div>
							)}
							<div
								ref={terminalRef}
								className="w-full flex-1 bg-black rounded"
							/>
							{/* AI Toggle Button */}
							{aiEnabled && isConnected && (
								<div className="mt-2 flex items-center justify-end">
									<button
										type="button"
										onClick={() => setAiPanelOpen(!aiPanelOpen)}
										className={`flex items-center gap-2 px-3 py-1.5 text-xs font-medium rounded-lg transition-colors ${
											aiPanelOpen
												? "bg-primary-600 text-white"
												: "bg-secondary-700 text-secondary-300 hover:bg-secondary-600 hover:text-white"
										}`}
									>
										<Bot className="h-4 w-4" />
										AI Assistant
										{aiPanelOpen ? (
											<PanelRightClose className="h-3 w-3" />
										) : (
											<PanelRightOpen className="h-3 w-3" />
										)}
									</button>
								</div>
							)}
						</div>

						{/* AI Assistant Panel */}
						{aiPanelOpen && aiEnabled && (
							<div className="w-80 border-l border-secondary-700 flex flex-col bg-secondary-800/50">
								{/* Panel Header */}
								<div className="p-3 border-b border-secondary-700 flex items-center justify-between">
									<div className="flex items-center gap-2">
										<Bot className="h-4 w-4 text-primary-400" />
										<span className="text-sm font-medium text-white">AI Assistant</span>
									</div>
									<button
										type="button"
										onClick={() => setAiPanelOpen(false)}
										className="text-secondary-400 hover:text-white"
									>
										<X className="h-4 w-4" />
									</button>
								</div>

								{/* Messages Area */}
								<div className="flex-1 overflow-y-auto p-3 space-y-3">
									{aiMessages.length === 0 && (
										<div className="text-center text-secondary-400 text-xs py-4">
											<Bot className="h-8 w-8 mx-auto mb-2 opacity-50" />
											<p>Ask about terminal output,</p>
											<p>errors, or get command help.</p>
										</div>
									)}
									{aiMessages.map((msg, idx) => (
										<div
											key={idx}
											className={`text-sm ${
												msg.role === "user"
													? "bg-primary-900/30 border border-primary-700/30 rounded-lg p-2 ml-4"
													: msg.isError
													? "bg-red-900/30 border border-red-700/30 rounded-lg p-2 mr-4"
													: "bg-secondary-700/50 rounded-lg p-2 mr-4"
											}`}
										>
											<div className="flex items-start gap-2">
												{msg.role === "assistant" && (
													<Bot className={`h-4 w-4 mt-0.5 ${msg.isError ? "text-red-400" : "text-primary-400"}`} />
												)}
												<div className="flex-1 text-secondary-200 whitespace-pre-wrap break-words">
													{msg.content}
												</div>
											</div>
										</div>
									))}
									{aiLoading && (
										<div className="bg-secondary-700/50 rounded-lg p-2 mr-4">
											<div className="flex items-center gap-2">
												<Loader2 className="h-4 w-4 animate-spin text-primary-400" />
												<span className="text-sm text-secondary-400">Thinking...</span>
											</div>
										</div>
									)}
								</div>

								{/* Input Area */}
								<div className="p-3 border-t border-secondary-700">
									<div className="flex gap-2">
										<input
											ref={aiInputRef}
											type="text"
											value={aiInput}
											onChange={(e) => setAiInput(e.target.value)}
											onKeyDown={(e) => {
												if (e.key === "Enter" && !e.shiftKey) {
													e.preventDefault();
													sendAiMessage();
												}
											}}
											placeholder="Ask about the terminal..."
											className="flex-1 px-3 py-2 text-sm bg-secondary-700 border border-secondary-600 rounded-lg text-white placeholder-secondary-400 focus:outline-none focus:ring-2 focus:ring-primary-500"
											disabled={aiLoading}
										/>
										<button
											type="button"
											onClick={sendAiMessage}
											disabled={!aiInput.trim() || aiLoading}
											className="px-3 py-2 bg-primary-600 text-white rounded-lg hover:bg-primary-700 disabled:opacity-50 disabled:cursor-not-allowed"
										>
											<Send className="h-4 w-4" />
										</button>
									</div>
								</div>
							</div>
						)}
					</div>
				)}
				
				{/* Status area when not connected - compact */}
				{!isConnected && !isConnecting && (
					<div className="flex-shrink-0 py-4 px-4 flex items-center justify-center border-t border-secondary-700/50 bg-secondary-900/30">
						<div className="flex items-center gap-2.5">
							<TerminalSquare className="h-5 w-5 text-secondary-500 opacity-60" />
							<p className="text-xs text-secondary-400 font-medium">Enter credentials above to connect</p>
						</div>
					</div>
				)}
			</div>
		);
	}

	// Modal mode - full screen overlay
	return (
		<div className="fixed inset-0 bg-black bg-opacity-75 flex items-center justify-center z-50">
			<div className="bg-secondary-900 rounded-lg w-full h-full max-w-[95vw] max-h-[95vh] flex flex-col m-2">
				{/* Header */}
				<div className="flex items-center justify-between p-4 border-b border-secondary-700">
					<div className="flex items-center gap-3">
						<TerminalSquare className="h-5 w-5 text-primary-400" />
						<div>
							<h3 className="text-lg font-semibold text-white">
								SSH Terminal - {host?.friendly_name}
							</h3>
							<p className="text-sm text-secondary-400">
								{host?.ip_address || host?.hostname}
							</p>
						</div>
					</div>
					<div className="flex items-center gap-2">
						{isConnected && (
							<span className="px-2 py-1 text-xs font-medium bg-green-900 text-green-200 rounded">
								Connected
							</span>
						)}
						{isConnecting && (
							<span className="px-2 py-1 text-xs font-medium bg-yellow-900 text-yellow-200 rounded flex items-center gap-1">
								<Loader2 className="h-3 w-3 animate-spin" />
								Connecting...
							</span>
						)}
						{error && (
							<span className="px-2 py-1 text-xs font-medium bg-red-900 text-red-200 rounded">
								Error
							</span>
						)}
						<button
							type="button"
							onClick={handleClose}
							className="text-secondary-400 hover:text-white transition-colors"
						>
							<X className="h-5 w-5" />
						</button>
					</div>
				</div>

				{/* Connection Form (shown when not connected) */}
				{!isConnected && !isConnecting && (
					<div className="p-6 border-b border-secondary-700 bg-secondary-800 flex-shrink-0">
						<div className="max-w-2xl mx-auto space-y-4">
							{error && (
								<div className="p-3 bg-red-900/50 border border-red-700 rounded text-red-200 text-sm">
									{error}
								</div>
							)}
							<div className="grid grid-cols-2 gap-4">
								<div>
									<label className="block text-sm font-medium text-secondary-300 mb-1">
										Username
									</label>
									<input
										type="text"
										value={sshConfig.username}
										onChange={(e) =>
											setSshConfig({ ...sshConfig, username: e.target.value })
										}
										onBlur={(e) => saveUsername(e.target.value)}
										autoComplete="username"
										className="w-full px-3 py-2 bg-secondary-700 border border-secondary-600 rounded text-white focus:outline-none focus:ring-2 focus:ring-primary-500"
										placeholder="root"
									/>
								</div>
								<div>
									<label className="block text-sm font-medium text-secondary-300 mb-1">
										Port
									</label>
									<input
										type="number"
										value={sshConfig.port}
										onChange={(e) =>
											setSshConfig({
												...sshConfig,
												port: parseInt(e.target.value, 10) || 22,
											})
										}
										className="w-full px-3 py-2 bg-secondary-700 border border-secondary-600 rounded text-white focus:outline-none focus:ring-2 focus:ring-primary-500"
										placeholder="22"
									/>
								</div>
							</div>
							<div>
								<label className="block text-sm font-medium text-secondary-300 mb-1">
									Password
								</label>
								<input
									type="password"
									value={sshConfig.password}
									onChange={(e) =>
										setSshConfig({ ...sshConfig, password: e.target.value })
									}
									onKeyDown={(e) => {
										if (e.key === "Enter" && sshConfig.password) {
											connectSsh();
										}
									}}
									className="w-full px-3 py-2 bg-secondary-700 border border-secondary-600 rounded text-white focus:outline-none focus:ring-2 focus:ring-primary-500"
									placeholder="Enter SSH password"
								/>
							</div>
							<button
								type="button"
								onClick={connectSsh}
								disabled={!sshConfig.username || !sshConfig.password}
								className="w-full px-4 py-2 bg-primary-600 hover:bg-primary-700 text-white font-medium rounded transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
							>
								Connect
							</button>
						</div>
					</div>
				)}

				{/* Connecting indicator */}
				{isConnecting && (
					<div className="p-6 border-b border-secondary-700 bg-secondary-800 flex-shrink-0">
						<div className="max-w-2xl mx-auto flex items-center gap-3">
							<Loader2 className="h-5 w-5 text-primary-400 animate-spin" />
							<span className="text-secondary-300">Connecting to SSH server...</span>
						</div>
					</div>
				)}

				{/* Terminal Container - Always visible, but takes full height when connected */}
				<div
					className={`p-4 overflow-hidden transition-all duration-300 ${
						isConnected || isConnecting ? "flex-1 min-h-[600px]" : "h-96"
					}`}
				>
					<div
						ref={terminalRef}
						className="w-full h-full bg-black rounded"
						style={{ minHeight: isConnected || isConnecting ? "100%" : "350px" }}
					/>
				</div>
			</div>
		</div>
	);
};

export default SshTerminal;

