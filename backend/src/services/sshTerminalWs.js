// SSH Terminal WebSocket Service
const logger = require("../utils/logger");
// Allows users to SSH into hosts from the PatchMon UI
// Auth: One-time ticket (preferred) or JWT token (legacy) via query parameter

const WebSocket = require("ws");
const { Client } = require("ssh2");
const jwt = require("jsonwebtoken");
const crypto = require("crypto");
const {
	validate_session,
	update_session_activity,
} = require("../utils/session_manager");
const { getPrismaClient } = require("../config/prisma");
const { logAuditEvent } = require("../utils/auditLogger");
const { redis } = require("./automation/shared/redis");

const prisma = getPrismaClient();

// Import agentWs functions - will be set after agentWs is initialized
let agentWsModule = null;

// Set agentWs module reference (called from agentWs.js after init)
function setAgentWsModule(module) {
	agentWsModule = module;
}

// Map to track SSH proxy sessions: sessionId -> { frontendWs, hostId, apiId }
const sshProxySessions = new Map();

// SSH Ticket constants (must match authRoutes.js)
const SSH_TICKET_PREFIX = "ssh:ticket:";

/**
 * Validate and consume an SSH ticket from Redis
 * @param {string} ticket - The ticket to validate
 * @param {string} expectedHostId - The expected host ID
 * @returns {Promise<{valid: boolean, userId?: string, sessionId?: string, reason?: string}>}
 */
async function consumeSshTicket(ticket, expectedHostId) {
	const key = `${SSH_TICKET_PREFIX}${ticket}`;
	const data = await redis.get(key);

	if (!data) {
		return { valid: false, reason: "Invalid or expired ticket" };
	}

	// Delete ticket immediately (one-time use)
	await redis.del(key);

	const ticketData = JSON.parse(data);

	// Verify host ID matches
	if (ticketData.hostId !== expectedHostId) {
		return { valid: false, reason: "Ticket host mismatch" };
	}

	return {
		valid: true,
		userId: ticketData.userId,
		sessionId: ticketData.sessionId,
	};
}

/**
 * Check if user has permission to access SSH terminal for a host
 * Requires can_manage_hosts permission
 * @param {Object} user - The authenticated user
 * @param {Object} host - The target host
 * @returns {Promise<{allowed: boolean, reason: string}>}
 */
async function checkSshAuthorization(user, _host) {
	// Admin role always has access (backward compatibility)
	if (user.role === "admin") {
		return { allowed: true, reason: "admin role" };
	}

	// Check role permissions
	const rolePermissions = await prisma.role_permissions.findUnique({
		where: { role: user.role },
	});

	if (!rolePermissions) {
		return {
			allowed: false,
			reason: `No permissions defined for role: ${user.role}`,
		};
	}

	// SSH terminal requires can_manage_hosts permission
	// This is a privileged operation that allows command execution on hosts
	if (!rolePermissions.can_manage_hosts) {
		return {
			allowed: false,
			reason: "Missing can_manage_hosts permission",
		};
	}

	return { allowed: true, reason: "has can_manage_hosts permission" };
}

/**
 * Handle SSH terminal WebSocket connection
 * Path: /api/{v}/ssh-terminal/:hostId
 */
async function handleSshTerminalUpgrade(request, socket, head, pathname) {
	try {
		logger.info(`[ssh-terminal] Upgrade request received: ${pathname}`);

		// Parse path: /api/v1/ssh-terminal/{hostId}
		const parts = pathname.split("/").filter(Boolean);
		if (parts.length !== 4 || parts[2] !== "ssh-terminal") {
			logger.info(
				`[ssh-terminal] Path does not match SSH terminal pattern: ${pathname}`,
			);
			return false; // Not an SSH terminal connection
		}

		const hostId = parts[3];
		logger.info(`[ssh-terminal] Processing connection for host ID: ${hostId}`);

		// SECURITY: Prefer one-time tickets over tokens in URLs
		// Tickets don't expose sensitive data in server logs or browser history
		const ticket = request.url.match(/[?&]ticket=([^&]+)/)?.[1];
		const token =
			request.url.match(/[?&]token=([^&]+)/)?.[1] ||
			request.headers.authorization?.replace("Bearer ", "");

		let user;
		let sessionId;

		if (ticket) {
			// Preferred: Ticket-based authentication (one-time use, not logged in URLs)
			logger.info(
				`[ssh-terminal] Using ticket authentication for host ${hostId}`,
			);

			const ticketResult = await consumeSshTicket(ticket, hostId);
			if (!ticketResult.valid) {
				logger.info(
					`[ssh-terminal] Ticket validation failed for host ${hostId}: ${ticketResult.reason}`,
				);
				socket.destroy();
				return true;
			}

			// Get user from database using ticket data
			const dbUser = await prisma.users.findUnique({
				where: { id: ticketResult.userId },
				select: {
					id: true,
					username: true,
					email: true,
					role: true,
					is_active: true,
				},
			});

			if (!dbUser || !dbUser.is_active) {
				logger.info(
					`[ssh-terminal] User not found or inactive for host ${hostId}`,
				);
				socket.destroy();
				return true;
			}

			user = dbUser;
			sessionId = ticketResult.sessionId;
			logger.info(
				`[ssh-terminal] Ticket authenticated user ${user.username} for host ${hostId}`,
			);
		} else if (token) {
			// Legacy: Token-based authentication (less secure - token visible in URLs)
			logger.info(
				`[ssh-terminal] Using legacy token authentication for host ${hostId}`,
			);

			// Verify token
			let decoded;
			try {
				decoded = jwt.verify(token, process.env.JWT_SECRET);
			} catch (err) {
				logger.info(
					`[ssh-terminal] Token verification failed for host ${hostId}:`,
					err.message,
				);
				socket.destroy();
				return true;
			}

			// Check if this is a WebSocket-purpose token (from /api/v1/auth/ws-token)
			let validation;
			if (decoded.purpose === "websocket") {
				validation = await validate_session(decoded.sessionId, "");
				if (!validation.valid) {
					logger.info(
						`[ssh-terminal] WS token session validation failed for host ${hostId}`,
					);
					socket.destroy();
					return true;
				}
			} else {
				validation = await validate_session(decoded.sessionId, token);
				if (!validation.valid) {
					logger.info(
						`[ssh-terminal] Session validation failed for host ${hostId}`,
					);
					socket.destroy();
					return true;
				}
			}

			user = validation.user;
			sessionId = decoded.sessionId;
			logger.info(
				`[ssh-terminal] Token authenticated user ${user.username} for host ${hostId}`,
			);
		} else {
			logger.info(
				`[ssh-terminal] No ticket or token provided for host ${hostId}`,
			);
			socket.destroy();
			return true;
		}

		// Update session activity
		await update_session_activity(sessionId);

		// Get host information
		const host = await prisma.hosts.findUnique({
			where: { id: hostId },
		});

		if (!host) {
			logger.info(`[ssh-terminal] Host ${hostId} not found`);
			socket.destroy();
			return true;
		}

		logger.info(
			`[ssh-terminal] Host found: ${host.friendly_name || host.hostname} (${host.ip || host.hostname})`,
		);

		// Check user permissions to access SSH terminal for this host
		const authCheck = await checkSshAuthorization(user, host);

		if (!authCheck.allowed) {
			logger.info(
				`[ssh-terminal] Access denied for user ${user.username} to host ${hostId}: ${authCheck.reason}`,
			);

			// Log the denied access attempt
			await logAuditEvent({
				event: "ssh_terminal_access_denied",
				userId: user.id,
				ipAddress: request.socket.remoteAddress,
				userAgent: request.headers["user-agent"],
				details: JSON.stringify({
					hostId: host.id,
					hostName: host.friendly_name || host.hostname,
					reason: authCheck.reason,
					userRole: user.role,
				}),
				success: false,
			});

			socket.destroy();
			return true;
		}

		logger.info(
			`[ssh-terminal] Access granted for user ${user.username} to host ${hostId}: ${authCheck.reason}`,
		);

		// Log the successful access
		await logAuditEvent({
			event: "ssh_terminal_access_granted",
			userId: user.id,
			ipAddress: request.socket.remoteAddress,
			userAgent: request.headers["user-agent"],
			details: JSON.stringify({
				hostId: host.id,
				hostName: host.friendly_name || host.hostname,
				reason: authCheck.reason,
				userRole: user.role,
			}),
			success: true,
		});

		// Create WebSocket connection using noServer mode
		const wss = new WebSocket.Server({ noServer: true });

		try {
			wss.handleUpgrade(request, socket, head, (ws) => {
				let sshClient = null;
				let sshStream = null;
				let proxySessionId = null;

				logger.info(
					`[ssh-terminal] User ${user.username} connecting to host ${host.friendly_name} (${host.id})`,
				);

				ws.on("message", async (message) => {
					try {
						const data = JSON.parse(message.toString());

						if (data.type === "connect") {
							// Initialize SSH connection
							if (sshClient || proxySessionId) {
								ws.send(
									JSON.stringify({
										type: "error",
										message: "Already connected",
									}),
								);
								return;
							}

							// Check if proxy mode is requested
							const connectionMode = data.connection_mode || "direct";
							if (connectionMode === "proxy") {
								// Proxy mode: route through agent WebSocket
								if (!agentWsModule) {
									ws.send(
										JSON.stringify({
											type: "error",
											message: "Agent WebSocket service not available",
										}),
									);
									return;
								}

								// Check if agent is connected
								if (!agentWsModule.isConnected(host.api_id)) {
									ws.send(
										JSON.stringify({
											type: "error",
											message:
												"Agent not connected. Please ensure the agent is running and connected.",
										}),
									);
									return;
								}

								// Check if SSH proxy is enabled in agent config
								// This will be checked by the agent, but we can provide a better error message
								// by checking integration status if available
								// For now, let the agent handle the check

								// Generate unique session ID
								proxySessionId = crypto.randomBytes(16).toString("hex");

								// Store session mapping
								sshProxySessions.set(proxySessionId, {
									frontendWs: ws,
									hostId: host.id,
									apiId: host.api_id,
								});

								// Forward SSH proxy request to agent
								const agentWs = agentWsModule.getConnectionByApiId(host.api_id);
								if (!agentWs || agentWs.readyState !== WebSocket.OPEN) {
									sshProxySessions.delete(proxySessionId);
									proxySessionId = null;
									ws.send(
										JSON.stringify({
											type: "error",
											message: "Agent WebSocket connection lost",
										}),
									);
									return;
								}

								// Validate proxy host and port
								const proxyHost = data.proxy_host || "localhost";
								const proxyPort = data.proxy_port || 22;

								// Basic validation to prevent injection
								if (
									!proxyHost.match(/^[a-zA-Z0-9._-]+$/) &&
									!proxyHost.match(/^(\d{1,3}\.){3}\d{1,3}$/)
								) {
									sshProxySessions.delete(proxySessionId);
									proxySessionId = null;
									ws.send(
										JSON.stringify({
											type: "error",
											message: "Invalid proxy host format",
										}),
									);
									return;
								}

								if (proxyPort < 1 || proxyPort > 65535) {
									sshProxySessions.delete(proxySessionId);
									proxySessionId = null;
									ws.send(
										JSON.stringify({
											type: "error",
											message: "Invalid proxy port (must be 1-65535)",
										}),
									);
									return;
								}

								// Send SSH proxy request to agent
								const sshProxyRequest = {
									type: "ssh_proxy",
									session_id: proxySessionId,
									host: proxyHost,
									port: proxyPort,
									username: data.username || "root",
									terminal: data.terminal || "xterm-256color",
									cols: data.cols || 80,
									rows: data.rows || 24,
								};

								// Add authentication data
								if (data.password) {
									sshProxyRequest.password = data.password;
								}
								if (data.privateKey) {
									sshProxyRequest.private_key = data.privateKey;
									if (data.passphrase) {
										sshProxyRequest.passphrase = data.passphrase;
									}
								}

								logger.info(
									`[ssh-terminal] Sending SSH proxy request to agent ${host.api_id} for session ${proxySessionId}`,
								);

								try {
									agentWs.send(JSON.stringify(sshProxyRequest));
								} catch (err) {
									logger.error(
										`[ssh-terminal] Failed to send SSH proxy request to agent:`,
										err,
									);
									sshProxySessions.delete(proxySessionId);
									proxySessionId = null;
									ws.send(
										JSON.stringify({
											type: "error",
											message: "Failed to send proxy request to agent",
										}),
									);
								}
								return; // Don't proceed with direct connection
							}

							// Direct mode: existing behavior
							sshClient = new Client();

							sshClient.on("ready", () => {
								logger.info(
									`[ssh-terminal] SSH connection established to ${host.friendly_name}`,
								);
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
											ws.send(
												JSON.stringify({ type: "error", message: err.message }),
											);
											return;
										}

										sshStream = stream;

										// Forward SSH output to WebSocket
										stream.on("data", (chunk) => {
											if (ws.readyState === WebSocket.OPEN) {
												ws.send(
													JSON.stringify({
														type: "data",
														data: chunk.toString(),
													}),
												);
											}
										});

										stream.on("close", () => {
											logger.info(
												`[ssh-terminal] SSH stream closed for ${host.friendly_name}`,
											);
											if (ws.readyState === WebSocket.OPEN) {
												ws.send(JSON.stringify({ type: "closed" }));
											}
											if (sshClient) {
												sshClient.end();
											}
										});

										stream.stderr.on("data", (chunk) => {
											if (ws.readyState === WebSocket.OPEN) {
												ws.send(
													JSON.stringify({
														type: "error",
														message: chunk.toString(),
													}),
												);
											}
										});
									},
								);
							});

							sshClient.on("error", (err) => {
								logger.error(
									`[ssh-terminal] SSH error for ${host.friendly_name}:`,
									err,
								);
								logger.error(`[ssh-terminal] SSH error details:`, {
									message: err.message,
									code: err.code,
									level: err.level,
									stack: err.stack,
								});
								if (ws.readyState === WebSocket.OPEN) {
									ws.send(
										JSON.stringify({
											type: "error",
											message: err.message || "SSH connection error",
										}),
									);
								}
								// Clean up on error
								sshClient = null;
								sshStream = null;
							});

							sshClient.on("close", () => {
								logger.info(
									`[ssh-terminal] SSH connection closed for ${host.friendly_name}`,
								);
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

							logger.info(
								`[ssh-terminal] Connecting to ${sshConfig.host}:${sshConfig.port} as ${sshConfig.username} using ${data.privateKey ? "private key" : "password"} authentication`,
							);
							sshClient.connect(sshConfig);
						} else if (data.type === "input") {
							// Send input to SSH session
							if (proxySessionId) {
								// Proxy mode: forward input to agent
								const session = sshProxySessions.get(proxySessionId);
								if (session) {
									const agentWs = agentWsModule.getConnectionByApiId(
										session.apiId,
									);
									if (agentWs && agentWs.readyState === WebSocket.OPEN) {
										agentWs.send(
											JSON.stringify({
												type: "ssh_proxy_input",
												session_id: proxySessionId,
												data: data.data,
											}),
										);
									}
								}
							} else if (sshStream?.writable) {
								// Direct mode
								sshStream.write(data.data);
							}
						} else if (data.type === "resize") {
							// Resize terminal
							if (proxySessionId) {
								// Proxy mode: forward resize to agent
								const session = sshProxySessions.get(proxySessionId);
								if (session) {
									const agentWs = agentWsModule.getConnectionByApiId(
										session.apiId,
									);
									if (agentWs && agentWs.readyState === WebSocket.OPEN) {
										agentWs.send(
											JSON.stringify({
												type: "ssh_proxy_resize",
												session_id: proxySessionId,
												cols: data.cols || 80,
												rows: data.rows || 24,
											}),
										);
									}
								}
							} else if (sshStream?.setWindow) {
								// Direct mode
								sshStream.setWindow(data.rows || 24, data.cols || 80);
							}
						} else if (data.type === "disconnect") {
							// Disconnect SSH session
							if (proxySessionId) {
								// Proxy mode: forward disconnect to agent
								const session = sshProxySessions.get(proxySessionId);
								if (session) {
									const agentWs = agentWsModule.getConnectionByApiId(
										session.apiId,
									);
									if (agentWs && agentWs.readyState === WebSocket.OPEN) {
										agentWs.send(
											JSON.stringify({
												type: "ssh_proxy_disconnect",
												session_id: proxySessionId,
											}),
										);
									}
									sshProxySessions.delete(proxySessionId);
								}
								proxySessionId = null;
							} else if (sshClient) {
								// Direct mode
								try {
									sshClient.end();
								} catch (err) {
									logger.error(
										`[ssh-terminal] Error disconnecting SSH client:`,
										err,
									);
								}
							}
							sshClient = null;
							sshStream = null;
						}
					} catch (err) {
						logger.error("[ssh-terminal] Error handling message:", err);
						if (ws.readyState === WebSocket.OPEN) {
							ws.send(
								JSON.stringify({
									type: "error",
									message: err.message || "Internal error",
								}),
							);
						}
					}
				});

				ws.on("close", () => {
					logger.info(
						`[ssh-terminal] WebSocket closed for ${host.friendly_name}`,
					);
					// Clean up proxy session if exists
					if (proxySessionId) {
						const session = sshProxySessions.get(proxySessionId);
						if (session) {
							const agentWs = agentWsModule?.getConnectionByApiId(
								session.apiId,
							);
							if (agentWs && agentWs.readyState === WebSocket.OPEN) {
								try {
									agentWs.send(
										JSON.stringify({
											type: "ssh_proxy_disconnect",
											session_id: proxySessionId,
										}),
									);
								} catch (err) {
									logger.error(
										`[ssh-terminal] Error sending disconnect to agent:`,
										err,
									);
								}
							}
							sshProxySessions.delete(proxySessionId);
						}
						proxySessionId = null;
					}
					// Clean up direct SSH connection if exists
					if (sshClient) {
						try {
							sshClient.end();
						} catch (err) {
							logger.error(`[ssh-terminal] Error closing SSH client:`, err);
						}
					}
					sshClient = null;
					sshStream = null;
				});

				ws.on("error", (err) => {
					logger.error(
						`[ssh-terminal] WebSocket error for ${host.friendly_name}:`,
						err,
					);
					// Clean up proxy session if exists
					if (proxySessionId) {
						const session = sshProxySessions.get(proxySessionId);
						if (session) {
							sshProxySessions.delete(proxySessionId);
						}
						proxySessionId = null;
					}
					// Clean up direct SSH connection if exists
					if (sshClient) {
						try {
							sshClient.end();
						} catch (closeErr) {
							logger.error(
								`[ssh-terminal] Error closing SSH client:`,
								closeErr,
							);
						}
					}
					sshClient = null;
					sshStream = null;
				});
			});

			logger.info(
				`[ssh-terminal] WebSocket upgrade completed for host ${host.friendly_name}`,
			);
		} catch (upgradeErr) {
			logger.error(
				`[ssh-terminal] Failed to handle upgrade for host ${host.friendly_name}:`,
				upgradeErr,
			);
			socket.destroy();
			return true;
		}

		return true; // Handled
	} catch (err) {
		logger.error("[ssh-terminal] Error in upgrade handler:", err);
		logger.error("[ssh-terminal] Error stack:", err.stack);
		// Try to complete upgrade before destroying to avoid 1006 error
		try {
			const wss = new WebSocket.Server({ noServer: true });
			wss.handleUpgrade(request, socket, head, (ws) => {
				ws.close(1011, "Internal server error");
			});
		} catch (upgradeErr) {
			logger.error("[ssh-terminal] Failed to complete upgrade:", upgradeErr);
			socket.destroy();
		}
		return true;
	}
}

/**
 * Handle SSH proxy messages from agent
 * Called from agentWs.js when agent sends SSH proxy data
 */
function handleSshProxyMessage(apiId, message) {
	const sessionId = message.session_id;
	const session = sshProxySessions.get(sessionId);

	if (!session) {
		logger.warn(
			`[ssh-terminal] Received SSH proxy message for unknown session ${sessionId}`,
		);
		return;
	}

	// Verify session belongs to this agent
	if (session.apiId !== apiId) {
		logger.warn(
			`[ssh-terminal] Session ${sessionId} API ID mismatch: expected ${session.apiId}, got ${apiId}`,
		);
		return;
	}

	const frontendWs = session.frontendWs;

	if (!frontendWs || frontendWs.readyState !== WebSocket.OPEN) {
		// Frontend disconnected, clean up session
		sshProxySessions.delete(sessionId);
		return;
	}

	try {
		if (message.type === "ssh_proxy_data") {
			// Forward SSH data to frontend
			frontendWs.send(
				JSON.stringify({
					type: "data",
					data: message.data,
				}),
			);
		} else if (message.type === "ssh_proxy_connected") {
			// SSH connection established via proxy
			frontendWs.send(JSON.stringify({ type: "connected" }));
		} else if (message.type === "ssh_proxy_error") {
			// SSH error via proxy
			frontendWs.send(
				JSON.stringify({
					type: "error",
					message: message.message || "SSH connection error",
				}),
			);
		} else if (message.type === "ssh_proxy_closed") {
			// SSH connection closed via proxy
			frontendWs.send(JSON.stringify({ type: "closed" }));
			sshProxySessions.delete(sessionId);
		}
	} catch (err) {
		logger.error(
			`[ssh-terminal] Error forwarding SSH proxy message to frontend:`,
			err,
		);
		sshProxySessions.delete(sessionId);
	}
}

module.exports = {
	handleSshTerminalUpgrade,
	setAgentWsModule,
	handleSshProxyMessage,
};
