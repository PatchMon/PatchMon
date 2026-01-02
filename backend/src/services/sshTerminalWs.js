// SSH Terminal WebSocket Service
// Allows users to SSH into hosts from the PatchMon UI
// Auth: JWT token via query parameter or Authorization header

const WebSocket = require("ws");
const { Client } = require("ssh2");
const jwt = require("jsonwebtoken");
const { validate_session, update_session_activity } = require("../utils/session_manager");
const { getPrismaClient } = require("../config/prisma");

const prisma = getPrismaClient();

/**
 * Handle SSH terminal WebSocket connection
 * Path: /api/{v}/ssh-terminal/:hostId
 */
async function handleSshTerminalUpgrade(request, socket, head, pathname) {
	try {
		console.log(`[ssh-terminal] Upgrade request received: ${pathname}`);
		
		// Parse path: /api/v1/ssh-terminal/{hostId}
		const parts = pathname.split("/").filter(Boolean);
		if (parts.length !== 4 || parts[2] !== "ssh-terminal") {
			console.log(`[ssh-terminal] Path does not match SSH terminal pattern: ${pathname}`);
			return false; // Not an SSH terminal connection
		}

		const hostId = parts[3];
		console.log(`[ssh-terminal] Processing connection for host ID: ${hostId}`);

		// Authenticate user via JWT token
		const token =
			request.url.match(/[?&]token=([^&]+)/)?.[1] ||
			request.headers.authorization?.replace("Bearer ", "");
		
		if (!token) {
			console.log(`[ssh-terminal] No token provided for host ${hostId}`);
			socket.destroy();
			return true; // Handled but rejected
		}

		// Verify token
		let decoded;
		try {
			decoded = jwt.verify(token, process.env.JWT_SECRET);
		} catch (err) {
			console.log(`[ssh-terminal] Token verification failed for host ${hostId}:`, err.message);
			socket.destroy();
			return true;
		}

		// Check if this is a WebSocket-purpose token (from /api/v1/auth/ws-token)
		// These tokens have a shorter lifetime and are specifically for WS connections
		let validation;
		if (decoded.purpose === "websocket") {
			// For WS tokens, validate the session using the sessionId from the token
			// Pass empty string for access_token since WS tokens are separate from session tokens
			// The JWT signature was already verified above, we just need to check session validity
			validation = await validate_session(decoded.sessionId, "");
			if (!validation.valid) {
				console.log(`[ssh-terminal] WS token session validation failed for host ${hostId}`);
				socket.destroy();
				return true;
			}
		} else {
			// For regular tokens, validate with the token itself
			validation = await validate_session(decoded.sessionId, token);
			if (!validation.valid) {
				console.log(`[ssh-terminal] Session validation failed for host ${hostId}`);
				socket.destroy();
				return true;
			}
		}

		// Update session activity
		await update_session_activity(decoded.sessionId);

		const user = validation.user;
		console.log(`[ssh-terminal] User ${user.username} authenticated for host ${hostId}`);

		// Get host information
		const host = await prisma.hosts.findUnique({
			where: { id: hostId },
		});

		if (!host) {
			console.log(`[ssh-terminal] Host ${hostId} not found`);
			socket.destroy();
			return true;
		}

		console.log(`[ssh-terminal] Host found: ${host.friendly_name || host.hostname} (${host.ip || host.hostname})`);

		// TODO: Check user permissions to access this host
		// For now, allow all authenticated users

		// Create WebSocket connection using noServer mode
		const wss = new WebSocket.Server({ noServer: true });
		
		try {
			wss.handleUpgrade(request, socket, head, (ws) => {
			let sshClient = null;
			let sshStream = null;

			console.log(`[ssh-terminal] User ${user.username} connecting to host ${host.friendly_name} (${host.id})`);

			ws.on("message", async (message) => {
				try {
					const data = JSON.parse(message.toString());

					if (data.type === "connect") {
						// Initialize SSH connection
						if (sshClient) {
							ws.send(JSON.stringify({ type: "error", message: "Already connected" }));
							return;
						}

						sshClient = new Client();

						sshClient.on("ready", () => {
							console.log(`[ssh-terminal] SSH connection established to ${host.friendly_name}`);
							ws.send(JSON.stringify({ type: "connected" }));

							// Open shell session
							sshClient.shell(
								{
									term: data.terminal || "xterm-256color",
									cols: data.cols || 80,
									rows: data.rows || 24,
								},
								(err, stream) => {
									if (err) {
										ws.send(JSON.stringify({ type: "error", message: err.message }));
										return;
									}

									sshStream = stream;

									// Forward SSH output to WebSocket
									stream.on("data", (chunk) => {
										if (ws.readyState === WebSocket.OPEN) {
											ws.send(JSON.stringify({
												type: "data",
												data: chunk.toString(),
											}));
										}
									});

									stream.on("close", () => {
										console.log(`[ssh-terminal] SSH stream closed for ${host.friendly_name}`);
										if (ws.readyState === WebSocket.OPEN) {
											ws.send(JSON.stringify({ type: "closed" }));
										}
										if (sshClient) {
											sshClient.end();
										}
									});

									stream.stderr.on("data", (chunk) => {
										if (ws.readyState === WebSocket.OPEN) {
											ws.send(JSON.stringify({
												type: "error",
												message: chunk.toString(),
											}));
										}
									});
								},
							);
						});

						sshClient.on("error", (err) => {
							console.error(`[ssh-terminal] SSH error for ${host.friendly_name}:`, err);
							console.error(`[ssh-terminal] SSH error details:`, {
								message: err.message,
								code: err.code,
								level: err.level,
								stack: err.stack,
							});
							if (ws.readyState === WebSocket.OPEN) {
								ws.send(JSON.stringify({
									type: "error",
									message: err.message || "SSH connection error",
								}));
							}
							// Clean up on error
							sshClient = null;
							sshStream = null;
						});

						sshClient.on("close", () => {
							console.log(`[ssh-terminal] SSH connection closed for ${host.friendly_name}`);
							sshClient = null;
							sshStream = null;
						});

						// Connect to SSH server
						// Use host's IP address and default SSH port
						// User provides credentials (password or SSH key) via WebSocket message
						const sshConfig = {
							host: host.ip || host.hostname,
							port: data.port || 22,
							username: data.username || "root",
							readyTimeout: 20000,
						};

						// If password provided in connect request
						if (data.password) {
							sshConfig.password = data.password;
						}

						// If private key provided
						if (data.privateKey) {
							sshConfig.privateKey = data.privateKey;
							if (data.passphrase) {
								sshConfig.passphrase = data.passphrase;
							}
							// When using private key, prefer key auth over password
							sshConfig.tryKeyboard = false;
						}

						console.log(`[ssh-terminal] Connecting to ${sshConfig.host}:${sshConfig.port} as ${sshConfig.username} using ${data.privateKey ? 'private key' : 'password'} authentication`);
						sshClient.connect(sshConfig);
					} else if (data.type === "input") {
						// Send input to SSH session
						if (sshStream && sshStream.writable) {
							sshStream.write(data.data);
						}
					} else if (data.type === "resize") {
						// Resize terminal
						if (sshStream && sshStream.setWindow) {
							sshStream.setWindow(data.rows || 24, data.cols || 80);
						}
					} else if (data.type === "disconnect") {
						// Disconnect SSH session
						if (sshClient) {
							try {
								sshClient.end();
							} catch (err) {
								console.error(`[ssh-terminal] Error disconnecting SSH client:`, err);
							}
						}
						sshClient = null;
						sshStream = null;
					}
				} catch (err) {
					console.error("[ssh-terminal] Error handling message:", err);
					if (ws.readyState === WebSocket.OPEN) {
						ws.send(JSON.stringify({
							type: "error",
							message: err.message || "Internal error",
						}));
					}
				}
			});

			ws.on("close", () => {
				console.log(`[ssh-terminal] WebSocket closed for ${host.friendly_name}`);
				if (sshClient) {
					try {
						sshClient.end();
					} catch (err) {
						console.error(`[ssh-terminal] Error closing SSH client:`, err);
					}
				}
				sshClient = null;
				sshStream = null;
			});

			ws.on("error", (err) => {
				console.error(`[ssh-terminal] WebSocket error for ${host.friendly_name}:`, err);
				if (sshClient) {
					try {
						sshClient.end();
					} catch (closeErr) {
						console.error(`[ssh-terminal] Error closing SSH client:`, closeErr);
					}
				}
				sshClient = null;
				sshStream = null;
			});
		});
		
		console.log(`[ssh-terminal] WebSocket upgrade completed for host ${host.friendly_name}`);
		} catch (upgradeErr) {
			console.error(`[ssh-terminal] Failed to handle upgrade for host ${host.friendly_name}:`, upgradeErr);
			socket.destroy();
			return true;
		}

		return true; // Handled
	} catch (err) {
		console.error("[ssh-terminal] Error in upgrade handler:", err);
		console.error("[ssh-terminal] Error stack:", err.stack);
		// Try to complete upgrade before destroying to avoid 1006 error
		try {
			const wss = new WebSocket.Server({ noServer: true });
			wss.handleUpgrade(request, socket, head, (ws) => {
				ws.close(1011, "Internal server error");
			});
		} catch (upgradeErr) {
			console.error("[ssh-terminal] Failed to complete upgrade:", upgradeErr);
			socket.destroy();
		}
		return true;
	}
}

module.exports = { handleSshTerminalUpgrade };

