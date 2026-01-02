// Lightweight WebSocket hub for agent connections
// Auth: X-API-ID / X-API-KEY headers on the upgrade request

const WebSocket = require("ws");
const url = require("node:url");
const bcrypt = require("bcryptjs");
const crypto = require("node:crypto");
const { get_current_time } = require("../utils/timezone");
const { handleSshTerminalUpgrade } = require("./sshTerminalWs");

/**
 * Verify API key against stored hash or plaintext (legacy support)
 * @param {string} providedKey - The key provided by the agent
 * @param {string} storedKey - The key stored in the database (may be hashed or plaintext)
 * @returns {Promise<boolean>} True if the key matches
 */
async function verifyApiKey(providedKey, storedKey) {
	if (!providedKey || !storedKey) return false;

	// Check if stored key is a bcrypt hash
	if (storedKey.match(/^\$2[aby]\$/)) {
		return bcrypt.compare(providedKey, storedKey);
	}

	// Legacy: plaintext comparison with timing-safe check
	try {
		const providedBuffer = Buffer.from(providedKey, "utf8");
		const storedBuffer = Buffer.from(storedKey, "utf8");
		if (providedBuffer.length !== storedBuffer.length) return false;
		return crypto.timingSafeEqual(providedBuffer, storedBuffer);
	} catch {
		return false;
	}
}

// Connection registry by api_id
const apiIdToSocket = new Map();

// Connection metadata (secure/insecure)
// Map<api_id, { ws: WebSocket, secure: boolean }>
const connectionMetadata = new Map();

// Subscribers for connection status changes (for SSE)
// Map<api_id, Set<callback>>
const connectionChangeSubscribers = new Map();

let wss;
let prisma;

function init(server, prismaClient) {
	prisma = prismaClient;
	wss = new WebSocket.Server({ noServer: true });

	// Handle HTTP upgrade events and authenticate before accepting WS
	server.on("upgrade", async (request, socket, head) => {
		try {
			const { pathname } = url.parse(request.url);
			if (!pathname) {
				socket.destroy();
				return;
			}

			// Handle Bull Board WebSocket connections
			if (pathname.startsWith("/bullboard")) {
				// For Bull Board, we need to check if the user is authenticated
				// Check for session cookie or authorization header
				const sessionCookie = request.headers.cookie?.match(
					/bull-board-session=([^;]+)/,
				)?.[1];
				const authHeader = request.headers.authorization;

				if (!sessionCookie && !authHeader) {
					socket.destroy();
					return;
				}

				// Accept the WebSocket connection for Bull Board
				wss.handleUpgrade(request, socket, head, (ws) => {
					ws.on("message", (message) => {
						// Echo back for Bull Board WebSocket
						try {
							ws.send(message);
						} catch (_err) {
							// Ignore send errors (connection may be closed)
						}
					});

					ws.on("error", (err) => {
						// Handle WebSocket errors gracefully for Bull Board
						if (
							err.code === "WS_ERR_INVALID_CLOSE_CODE" ||
							err.code === "ECONNRESET" ||
							err.code === "EPIPE"
						) {
							// These are expected errors, just log quietly
							console.log("[bullboard-ws] connection error:", err.code);
						} else {
							console.error("[bullboard-ws] error:", err.message || err);
						}
					});

					ws.on("close", () => {
						// Connection closed, no action needed
					});
				});
				return;
			}

			// Handle SSH terminal WebSocket connections
			if (pathname.startsWith("/api/") && pathname.includes("/ssh-terminal/")) {
				const handled = await handleSshTerminalUpgrade(request, socket, head, pathname);
				if (handled) return;
			}

			// Handle agent WebSocket connections
			if (!pathname.startsWith("/api/")) {
				socket.destroy();
				return;
			}

			// Expected path: /api/{v}/agents/ws
			const parts = pathname.split("/").filter(Boolean); // [api, v1, agents, ws]
			if (parts.length !== 4 || parts[2] !== "agents" || parts[3] !== "ws") {
				socket.destroy();
				return;
			}

			const apiId = request.headers["x-api-id"];
			const apiKey = request.headers["x-api-key"];
			if (!apiId || !apiKey) {
				socket.destroy();
				return;
			}

			// Validate credentials
			const host = await prisma.hosts.findUnique({ where: { api_id: apiId } });
			if (!host) {
				socket.destroy();
				return;
			}

			// Verify API key (supports bcrypt hashed and legacy plaintext keys)
			const isValidKey = await verifyApiKey(apiKey, host.api_key);
			if (!isValidKey) {
				console.log(`[agent-ws] invalid API key for api_id=${apiId}`);
				socket.destroy();
				return;
			}

			wss.handleUpgrade(request, socket, head, (ws) => {
				ws.apiId = apiId;

				// Detect if connection is secure (wss://) or not (ws://)
				const isSecure =
					socket.encrypted || request.headers["x-forwarded-proto"] === "https";

				apiIdToSocket.set(apiId, ws);
				connectionMetadata.set(apiId, { ws, secure: isSecure });

				console.log(
					`[agent-ws] connected api_id=${apiId} protocol=${isSecure ? "wss" : "ws"} total=${apiIdToSocket.size}`,
				);

				// Notify subscribers of connection
				notifyConnectionChange(apiId, true);

				ws.on("message", async (data) => {
					// Handle incoming messages from agent (e.g., Docker status updates)
					try {
						const message = JSON.parse(data.toString());

						if (message.type === "docker_status") {
							// Handle Docker container status events
							await handleDockerStatusEvent(apiId, message);
						}
						// Add more message types here as needed
					} catch (err) {
						console.error(
							`[agent-ws] error parsing message from ${apiId}:`,
							err,
						);
					}
				});

				ws.on("error", (err) => {
					// Handle WebSocket errors gracefully without crashing
					// Common errors: invalid close codes (1006), connection resets, etc.
					if (
						err.code === "WS_ERR_INVALID_CLOSE_CODE" ||
						err.message?.includes("invalid status code 1006") ||
						err.message?.includes("Invalid WebSocket frame")
					) {
						// 1006 is a special close code indicating abnormal closure
						// It cannot be sent in a close frame, but can occur when connection is lost
						console.log(
							`[agent-ws] connection error for ${apiId} (abnormal closure):`,
							err.message || err.code,
						);
					} else if (
						err.code === "ECONNRESET" ||
						err.code === "EPIPE" ||
						err.message?.includes("read ECONNRESET")
					) {
						// Connection reset errors are common and expected
						console.log(`[agent-ws] connection reset for ${apiId}`);
					} else {
						// Log other errors for debugging
						console.error(
							`[agent-ws] error for ${apiId}:`,
							err.message || err.code || err,
						);
					}

					// Clean up connection on error
					const existing = apiIdToSocket.get(apiId);
					if (existing === ws) {
						apiIdToSocket.delete(apiId);
						connectionMetadata.delete(apiId);
						// Notify subscribers of disconnection
						notifyConnectionChange(apiId, false);
					}

					// Try to close the connection gracefully if still open
					if (
						ws.readyState === WebSocket.OPEN ||
						ws.readyState === WebSocket.CONNECTING
					) {
						try {
							ws.close(1000); // Normal closure
						} catch {
							// Ignore errors when closing
						}
					}
				});

				ws.on("close", (code, reason) => {
					const existing = apiIdToSocket.get(apiId);
					if (existing === ws) {
						apiIdToSocket.delete(apiId);
						connectionMetadata.delete(apiId);
						// Notify subscribers of disconnection
						notifyConnectionChange(apiId, false);
					}
					console.log(
						`[agent-ws] disconnected api_id=${apiId} code=${code} reason=${reason || "none"} total=${apiIdToSocket.size}`,
					);
				});

				// Optional: greet/ack
				safeSend(ws, JSON.stringify({ type: "connected" }));
			});
		} catch (_err) {
			try {
				socket.destroy();
			} catch {
				/* ignore */
			}
		}
	});
}

function safeSend(ws, data) {
	if (ws && ws.readyState === WebSocket.OPEN) {
		try {
			ws.send(data);
		} catch {
			/* ignore */
		}
	}
}

function broadcastSettingsUpdate(newInterval) {
	const payload = JSON.stringify({
		type: "settings_update",
		update_interval: newInterval,
	});
	for (const [, ws] of apiIdToSocket) {
		safeSend(ws, payload);
	}
}

function pushReportNow(apiId) {
	const ws = apiIdToSocket.get(apiId);
	safeSend(ws, JSON.stringify({ type: "report_now" }));
}

function pushSettingsUpdate(apiId, newInterval) {
	const ws = apiIdToSocket.get(apiId);
	safeSend(
		ws,
		JSON.stringify({ type: "settings_update", update_interval: newInterval }),
	);
}

function pushUpdateAgent(apiId) {
	const ws = apiIdToSocket.get(apiId);
	safeSend(ws, JSON.stringify({ type: "update_agent" }));
}

function pushIntegrationToggle(apiId, integrationName, enabled) {
	const ws = apiIdToSocket.get(apiId);
	if (ws && ws.readyState === WebSocket.OPEN) {
		safeSend(
			ws,
			JSON.stringify({
				type: "integration_toggle",
				integration: integrationName,
				enabled: enabled,
			}),
		);
		console.log(
			`📤 Pushed integration toggle to agent ${apiId}: ${integrationName} = ${enabled}`,
		);
		return true;
	} else {
		console.log(
			`⚠️ Agent ${apiId} not connected, cannot push integration toggle, please edit config.yml manually`,
		);
		return false;
	}
}

function getConnectionByApiId(apiId) {
	return apiIdToSocket.get(apiId);
}

function pushComplianceScan(apiId, profileType = "all") {
	const ws = apiIdToSocket.get(apiId);
	if (ws && ws.readyState === WebSocket.OPEN) {
		safeSend(ws, JSON.stringify({
			type: "compliance_scan",
			profile_type: profileType,
		}));
		console.log(`[agent-ws] Triggered compliance scan for ${apiId}: ${profileType}`);
		return true;
	}
	return false;
}

function pushUpdateNotification(apiId, updateInfo) {
	const ws = apiIdToSocket.get(apiId);
	if (ws && ws.readyState === WebSocket.OPEN) {
		safeSend(
			ws,
			JSON.stringify({
				type: "update_notification",
				version: updateInfo.version,
				force: updateInfo.force || false,
				downloadUrl: updateInfo.downloadUrl,
				message: updateInfo.message,
			}),
		);
		console.log(
			`📤 Pushed update notification to agent ${apiId}: version ${updateInfo.version}`,
		);
		return true;
	} else {
		console.log(
			`⚠️ Agent ${apiId} not connected, cannot push update notification`,
		);
		return false;
	}
}

async function pushUpdateNotificationToAll(updateInfo) {
	let notifiedCount = 0;
	let failedCount = 0;
	let skippedCount = 0;

	// Get all hosts with their auto_update settings
	const hosts = await prisma.hosts.findMany({
		where: {
			api_id: { in: Array.from(apiIdToSocket.keys()) },
		},
		select: {
			api_id: true,
			auto_update: true,
		},
	});

	// Create a map for quick lookup
	const hostAutoUpdateMap = new Map();
	for (const host of hosts) {
		hostAutoUpdateMap.set(host.api_id, host.auto_update);
	}

	for (const [apiId, ws] of apiIdToSocket) {
		if (ws && ws.readyState === WebSocket.OPEN) {
			// Check per-host auto_update setting
			const hostAutoUpdate = hostAutoUpdateMap.get(apiId);
			if (hostAutoUpdate === false) {
				skippedCount++;
				console.log(
					`⚠️ Skipping update notification for agent ${apiId} (auto-update disabled for host)`,
				);
				continue;
			}

			try {
				safeSend(
					ws,
					JSON.stringify({
						type: "update_notification",
						version: updateInfo.version,
						force: updateInfo.force || false,
						message: updateInfo.message,
					}),
				);
				notifiedCount++;
				console.log(
					`📤 Pushed update notification to agent ${apiId}: version ${updateInfo.version}`,
				);
			} catch (error) {
				failedCount++;
				console.error(`❌ Failed to notify agent ${apiId}:`, error.message);
			}
		} else {
			failedCount++;
		}
	}

	const totalAgents = apiIdToSocket.size;
	console.log(
		`📤 Update notification sent to ${notifiedCount} agents, ${failedCount} failed, ${skippedCount} skipped (auto-update disabled)`,
	);
	return { notifiedCount, failedCount, skippedCount, totalAgents };
}

// Notify all subscribers when connection status changes
function notifyConnectionChange(apiId, connected) {
	const subscribers = connectionChangeSubscribers.get(apiId);
	if (subscribers) {
		for (const callback of subscribers) {
			try {
				callback(connected);
			} catch (err) {
				console.error(`[agent-ws] error notifying subscriber:`, err);
			}
		}
	}
}

// Subscribe to connection status changes for a specific api_id
function subscribeToConnectionChanges(apiId, callback) {
	if (!connectionChangeSubscribers.has(apiId)) {
		connectionChangeSubscribers.set(apiId, new Set());
	}
	connectionChangeSubscribers.get(apiId).add(callback);

	// Return unsubscribe function
	return () => {
		const subscribers = connectionChangeSubscribers.get(apiId);
		if (subscribers) {
			subscribers.delete(callback);
			if (subscribers.size === 0) {
				connectionChangeSubscribers.delete(apiId);
			}
		}
	};
}

// Handle Docker container status events from agent
async function handleDockerStatusEvent(apiId, message) {
	try {
		const { event: _event, container_id, name, status, timestamp } = message;

		console.log(
			`[Docker Event] ${apiId}: Container ${name} (${container_id}) - ${status}`,
		);

		// Find the host
		const host = await prisma.hosts.findUnique({
			where: { api_id: apiId },
		});

		if (!host) {
			console.error(`[Docker Event] Host not found for api_id: ${apiId}`);
			return;
		}

		// Update container status in database
		const container = await prisma.docker_containers.findUnique({
			where: {
				host_id_container_id: {
					host_id: host.id,
					container_id: container_id,
				},
			},
		});

		if (container) {
			await prisma.docker_containers.update({
				where: { id: container.id },
				data: {
					status: status,
					state: status,
					updated_at: new Date(timestamp || Date.now()),
					last_checked: get_current_time(),
				},
			});

			console.log(
				`[Docker Event] Updated container ${name} status to ${status}`,
			);
		} else {
			console.log(
				`[Docker Event] Container ${name} not found in database (may be new)`,
			);
		}

		// TODO: Broadcast to connected dashboard clients via SSE or WebSocket
		// This would notify the frontend UI in real-time
	} catch (error) {
		console.error(`[Docker Event] Error handling Docker status event:`, error);
	}
}

module.exports = {
	init,
	broadcastSettingsUpdate,
	pushReportNow,
	pushSettingsUpdate,
	pushUpdateAgent,
	pushIntegrationToggle,
	pushUpdateNotification,
	pushUpdateNotificationToAll,
	pushComplianceScan,
	// Expose read-only view of connected agents
	getConnectedApiIds: () => Array.from(apiIdToSocket.keys()),
	getConnectionByApiId,
	isConnected: (apiId) => {
		const ws = apiIdToSocket.get(apiId);
		return !!ws && ws.readyState === WebSocket.OPEN;
	},
	// Get connection info including protocol (ws/wss)
	getConnectionInfo: (apiId) => {
		const metadata = connectionMetadata.get(apiId);
		if (!metadata) {
			return { connected: false, secure: false };
		}
		const connected = metadata.ws.readyState === WebSocket.OPEN;
		return { connected, secure: metadata.secure };
	},
	// Subscribe to connection status changes (for SSE)
	subscribeToConnectionChanges,
};
