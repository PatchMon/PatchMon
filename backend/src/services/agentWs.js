// Lightweight WebSocket hub for agent connections
const logger = require("../utils/logger");
// Auth: X-API-ID / X-API-KEY headers on the upgrade request

const WebSocket = require("ws");
const url = require("node:url");
const { get_current_time } = require("../utils/timezone");
const { handleSshTerminalUpgrade } = require("./sshTerminalWs");
const { verifyApiKey } = require("../utils/apiKeyUtils");

// Connection registry by api_id
const apiIdToSocket = new Map();

// Connection metadata (secure/insecure)
// Map<api_id, { ws: WebSocket, secure: boolean }>
const connectionMetadata = new Map();

// Subscribers for connection status changes (for SSE)
// Map<api_id, Set<callback>>
const connectionChangeSubscribers = new Map();

// Subscribers for compliance scan progress (for SSE)
// Map<api_id, Set<callback>>
const complianceProgressSubscribers = new Map();

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
							logger.info("[bullboard-ws] connection error:", err.code);
						} else {
							logger.error("[bullboard-ws] error:", err.message || err);
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
				logger.info(`[agent-ws] invalid API key for api_id=${apiId}`);
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

				logger.info(
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
						} else if (message.type === "compliance_scan_progress") {
							// Handle compliance scan progress events
							handleComplianceProgressEvent(apiId, message);
						}
						// Add more message types here as needed
					} catch (err) {
						logger.error(
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
						logger.info(
							`[agent-ws] connection error for ${apiId} (abnormal closure):`,
							err.message || err.code,
						);
					} else if (
						err.code === "ECONNRESET" ||
						err.code === "EPIPE" ||
						err.message?.includes("read ECONNRESET")
					) {
						// Connection reset errors are common and expected
						logger.info(`[agent-ws] connection reset for ${apiId}`);
					} else {
						// Log other errors for debugging
						logger.error(
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
					logger.info(
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

function pushRefreshIntegrationStatus(apiId) {
	const ws = apiIdToSocket.get(apiId);
	if (ws && ws.readyState === WebSocket.OPEN) {
		safeSend(ws, JSON.stringify({ type: "refresh_integration_status" }));
		logger.info(`📤 Pushed refresh integration status to agent ${apiId}`);
		return true;
	} else {
		logger.info(
			`⚠️ Agent ${apiId} not connected, cannot refresh integration status`,
		);
		return false;
	}
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
		logger.info(
			`📤 Pushed integration toggle to agent ${apiId}: ${integrationName} = ${enabled}`,
		);
		return true;
	} else {
		logger.info(
			`⚠️ Agent ${apiId} not connected, cannot push integration toggle, please edit config.yml manually`,
		);
		return false;
	}
}

function getConnectionByApiId(apiId) {
	return apiIdToSocket.get(apiId);
}

function pushComplianceScan(apiId, profileType = "all", options = {}) {
	const ws = apiIdToSocket.get(apiId);
	if (ws && ws.readyState === WebSocket.OPEN) {
		const payload = {
			type: "compliance_scan",
			profile_type: profileType,
			profile_id: options.profileId || null,
			enable_remediation: options.enableRemediation || false,
			fetch_remote_resources: options.fetchRemoteResources || false,
		};
		safeSend(ws, JSON.stringify(payload));
		const remediationStatus = options.enableRemediation ? " (with remediation)" : "";
		const profileInfo = options.profileId ? ` profile=${options.profileId}` : "";
		logger.info(`[agent-ws] Triggered compliance scan for ${apiId}: ${profileType}${profileInfo}${remediationStatus}`);
		return true;
	}
	return false;
}

function pushUpgradeSSG(apiId) {
	logger.info(`[agent-ws] pushUpgradeSSG called for api_id=${apiId}`);
	const ws = apiIdToSocket.get(apiId);
	logger.info(`[agent-ws] WebSocket found: ${!!ws}, readyState: ${ws?.readyState}, OPEN=${WebSocket.OPEN}`);
	if (ws && ws.readyState === WebSocket.OPEN) {
		const payload = JSON.stringify({ type: "upgrade_ssg" });
		logger.info(`[agent-ws] Sending payload: ${payload}`);
		try {
			ws.send(payload);
			logger.info(`[agent-ws] Triggered SSG upgrade for ${apiId}`);
			return true;
		} catch (err) {
			logger.error(`[agent-ws] Failed to send SSG upgrade to ${apiId}:`, err);
			return false;
		}
	}
	logger.info(`[agent-ws] Cannot send SSG upgrade - WebSocket not ready for ${apiId}`);
	return false;
}

function pushDockerImageScan(apiId, options = {}) {
	const ws = apiIdToSocket.get(apiId);
	if (ws && ws.readyState === WebSocket.OPEN) {
		const payload = {
			type: "docker_image_scan",
			image_name: options.imageName || null,
			container_name: options.containerName || null,
			scan_all_images: options.scanAllImages || false,
		};
		safeSend(ws, JSON.stringify(payload));
		const scanTarget = options.scanAllImages
			? "all images"
			: options.imageName
			? `image: ${options.imageName}`
			: options.containerName
			? `container: ${options.containerName}`
			: "unknown target";
		logger.info(`[agent-ws] Triggered Docker image CVE scan for ${apiId}: ${scanTarget}`);
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
		logger.info(
			`📤 Pushed update notification to agent ${apiId}: version ${updateInfo.version}`,
		);
		return true;
	} else {
		logger.info(
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
				logger.info(
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
				logger.info(
					`📤 Pushed update notification to agent ${apiId}: version ${updateInfo.version}`,
				);
			} catch (error) {
				failedCount++;
				logger.error(`❌ Failed to notify agent ${apiId}:`, error.message);
			}
		} else {
			failedCount++;
		}
	}

	const totalAgents = apiIdToSocket.size;
	logger.info(
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
				logger.error(`[agent-ws] error notifying subscriber:`, err);
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

// Handle compliance scan progress events from agent
function handleComplianceProgressEvent(apiId, message) {
	const { phase, profile_name, message: progressMessage, progress, error, timestamp } = message;

	logger.info(
		`[Compliance Progress] ${apiId}: ${phase} - ${progressMessage} (${progress}%)`,
	);

	// Notify all subscribers for this api_id
	const subscribers = complianceProgressSubscribers.get(apiId);
	if (subscribers) {
		const progressData = {
			phase,
			profile_name,
			message: progressMessage,
			progress,
			error,
			timestamp: timestamp || new Date().toISOString(),
		};

		for (const callback of subscribers) {
			try {
				callback(progressData);
			} catch (err) {
				logger.error(`[Compliance Progress] error notifying subscriber:`, err);
			}
		}
	}
}

// Subscribe to compliance progress updates for a specific api_id
function subscribeToComplianceProgress(apiId, callback) {
	if (!complianceProgressSubscribers.has(apiId)) {
		complianceProgressSubscribers.set(apiId, new Set());
	}
	complianceProgressSubscribers.get(apiId).add(callback);

	// Return unsubscribe function
	return () => {
		const subscribers = complianceProgressSubscribers.get(apiId);
		if (subscribers) {
			subscribers.delete(callback);
			if (subscribers.size === 0) {
				complianceProgressSubscribers.delete(apiId);
			}
		}
	};
}

// Handle Docker container status events from agent
async function handleDockerStatusEvent(apiId, message) {
	try {
		const { event: _event, container_id, name, status, timestamp } = message;

		logger.info(
			`[Docker Event] ${apiId}: Container ${name} (${container_id}) - ${status}`,
		);

		// Find the host
		const host = await prisma.hosts.findUnique({
			where: { api_id: apiId },
		});

		if (!host) {
			logger.error(`[Docker Event] Host not found for api_id: ${apiId}`);
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

			logger.info(
				`[Docker Event] Updated container ${name} status to ${status}`,
			);
		} else {
			logger.info(
				`[Docker Event] Container ${name} not found in database (may be new)`,
			);
		}

		// TODO: Broadcast to connected dashboard clients via SSE or WebSocket
		// This would notify the frontend UI in real-time
	} catch (error) {
		logger.error(`[Docker Event] Error handling Docker status event:`, error);
	}
}

function pushRemediateRule(apiId, ruleId) {
	const ws = apiIdToSocket.get(apiId);
	if (ws && ws.readyState === WebSocket.OPEN) {
		const payload = {
			type: "remediate_rule",
			rule_id: ruleId,
		};
		safeSend(ws, JSON.stringify(payload));
		logger.info(`[agent-ws] Triggered single rule remediation for ${apiId}: ${ruleId}`);
		return true;
	}
	return false;
}

module.exports = {
	init,
	broadcastSettingsUpdate,
	pushReportNow,
	pushSettingsUpdate,
	pushUpdateAgent,
	pushRefreshIntegrationStatus,
	pushIntegrationToggle,
	pushUpdateNotification,
	pushUpdateNotificationToAll,
	pushComplianceScan,
	pushUpgradeSSG,
	pushRemediateRule,
	pushDockerImageScan,
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
	// Subscribe to compliance progress updates (for SSE)
	subscribeToComplianceProgress,
};
