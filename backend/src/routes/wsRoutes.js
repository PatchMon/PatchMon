const express = require("express");
const { authenticateToken } = require("../middleware/auth");
const {
	getConnectionInfo,
	subscribeToConnectionChanges,
	subscribeToComplianceProgress,
} = require("../services/agentWs");
const {
	validate_session,
	update_session_activity,
} = require("../utils/session_manager");

const router = express.Router();

// Get WebSocket connection status for multiple hosts at once (bulk endpoint)
router.get("/status", authenticateToken, async (req, res) => {
	try {
		const { apiIds } = req.query; // Comma-separated list of api_ids
		const idArray = apiIds ? apiIds.split(",").filter((id) => id.trim()) : [];

		const statusMap = {};
		idArray.forEach((apiId) => {
			statusMap[apiId] = getConnectionInfo(apiId);
		});

		res.json({
			success: true,
			data: statusMap,
		});
	} catch (error) {
		console.error("Error fetching bulk WebSocket status:", error);
		res.status(500).json({
			success: false,
			error: "Failed to fetch WebSocket status",
		});
	}
});

// Get WebSocket connection status by api_id (single endpoint)
router.get("/status/:apiId", authenticateToken, async (req, res) => {
	try {
		const { apiId } = req.params;

		// Direct in-memory check - no database query needed
		const connectionInfo = getConnectionInfo(apiId);

		// Minimal response for maximum speed
		res.json({
			success: true,
			data: connectionInfo,
		});
	} catch (error) {
		console.error("Error fetching WebSocket status:", error);
		res.status(500).json({
			success: false,
			error: "Failed to fetch WebSocket status",
		});
	}
});

// Server-Sent Events endpoint for real-time status updates (no polling needed!)
router.get("/status/:apiId/stream", async (req, res) => {
	try {
		const { apiId } = req.params;

		// Manual authentication for SSE (EventSource doesn't support custom headers)
		// Check cookies first (httpOnly cookie from browser), then query param, then Authorization header
		const token =
			req.cookies?.token ||
			req.query.token ||
			req.headers.authorization?.replace("Bearer ", "");
		if (!token) {
			return res.status(401).json({ error: "Authentication required" });
		}

		// Verify token manually with session validation
		const jwt = require("jsonwebtoken");
		try {
			const decoded = jwt.verify(token, process.env.JWT_SECRET);

			// Validate session (same as regular auth middleware)
			const validation = await validate_session(decoded.sessionId, token);
			if (!validation.valid) {
				return res.status(401).json({ error: "Invalid or expired session" });
			}

			// Update session activity to prevent inactivity timeout
			await update_session_activity(decoded.sessionId);

			req.user = validation.user;
		} catch (_err) {
			return res.status(401).json({ error: "Invalid or expired token" });
		}

		console.log("[SSE] Client connected for api_id:", apiId);

		// Set headers for SSE
		res.setHeader("Content-Type", "text/event-stream");
		res.setHeader("Cache-Control", "no-cache");
		res.setHeader("Connection", "keep-alive");
		res.setHeader("X-Accel-Buffering", "no"); // Disable nginx buffering

		// Send initial status immediately
		const initialInfo = getConnectionInfo(apiId);
		res.write(`data: ${JSON.stringify(initialInfo)}\n\n`);
		res.flushHeaders(); // Ensure headers are sent immediately

		// Subscribe to connection changes for this specific api_id
		const unsubscribe = subscribeToConnectionChanges(apiId, (_connected) => {
			try {
				// Push update to client instantly when status changes
				const connectionInfo = getConnectionInfo(apiId);
				console.log(
					`[SSE] Pushing status change for ${apiId}: connected=${connectionInfo.connected} secure=${connectionInfo.secure}`,
				);
				res.write(`data: ${JSON.stringify(connectionInfo)}\n\n`);
			} catch (err) {
				console.error("[SSE] Error writing to stream:", err);
			}
		});

		// Heartbeat to keep connection alive (every 30 seconds)
		const heartbeat = setInterval(() => {
			try {
				res.write(": heartbeat\n\n");
			} catch (err) {
				console.error("[SSE] Error writing heartbeat:", err);
				clearInterval(heartbeat);
			}
		}, 30000);

		// Cleanup on client disconnect
		req.on("close", () => {
			console.log("[SSE] Client disconnected for api_id:", apiId);
			clearInterval(heartbeat);
			unsubscribe();
		});

		// Handle errors - distinguish between different error types
		req.on("error", (err) => {
			// Only log non-connection-reset errors to reduce noise
			if (err.code !== "ECONNRESET" && err.code !== "EPIPE") {
				console.error("[SSE] Request error:", err);
			} else {
				console.log("[SSE] Client connection reset for api_id:", apiId);
			}
			clearInterval(heartbeat);
			unsubscribe();
		});

		// Handle response errors
		res.on("error", (err) => {
			if (err.code !== "ECONNRESET" && err.code !== "EPIPE") {
				console.error("[SSE] Response error:", err);
			}
			clearInterval(heartbeat);
			unsubscribe();
		});
	} catch (error) {
		console.error("[SSE] Unexpected error:", error);
		if (!res.headersSent) {
			res.status(500).json({ error: "Internal server error" });
		}
	}
});

// Server-Sent Events endpoint for real-time compliance scan progress updates
router.get("/compliance-progress/:apiId/stream", async (req, res) => {
	try {
		const { apiId } = req.params;

		// Manual authentication for SSE (EventSource doesn't support custom headers)
		// Check cookies first (httpOnly cookie from browser), then query param, then Authorization header
		const token =
			req.cookies?.token ||
			req.query.token ||
			req.headers.authorization?.replace("Bearer ", "");
		if (!token) {
			return res.status(401).json({ error: "Authentication required" });
		}

		// Verify token manually with session validation
		const jwt = require("jsonwebtoken");
		try {
			const decoded = jwt.verify(token, process.env.JWT_SECRET);

			// Validate session (same as regular auth middleware)
			const validation = await validate_session(decoded.sessionId, token);
			if (!validation.valid) {
				return res.status(401).json({ error: "Invalid or expired session" });
			}

			// Update session activity to prevent inactivity timeout
			await update_session_activity(decoded.sessionId);

			req.user = validation.user;
		} catch (_err) {
			return res.status(401).json({ error: "Invalid or expired token" });
		}

		console.log("[SSE] Client connected for compliance progress, api_id:", apiId);

		// Set headers for SSE
		res.setHeader("Content-Type", "text/event-stream");
		res.setHeader("Cache-Control", "no-cache");
		res.setHeader("Connection", "keep-alive");
		res.setHeader("X-Accel-Buffering", "no"); // Disable nginx buffering

		// Send initial connection confirmation
		res.write(`data: ${JSON.stringify({ phase: "connected", message: "Listening for scan progress..." })}\n\n`);
		res.flushHeaders(); // Ensure headers are sent immediately

		// Subscribe to compliance progress updates for this specific api_id
		const unsubscribe = subscribeToComplianceProgress(apiId, (progressData) => {
			try {
				// Push progress update to client instantly
				console.log(
					`[SSE] Pushing compliance progress for ${apiId}: phase=${progressData.phase} progress=${progressData.progress}%`,
				);
				res.write(`data: ${JSON.stringify(progressData)}\n\n`);
			} catch (err) {
				console.error("[SSE] Error writing compliance progress to stream:", err);
			}
		});

		// Heartbeat to keep connection alive (every 30 seconds)
		const heartbeat = setInterval(() => {
			try {
				res.write(": heartbeat\n\n");
			} catch (err) {
				console.error("[SSE] Error writing heartbeat:", err);
				clearInterval(heartbeat);
			}
		}, 30000);

		// Cleanup on client disconnect
		req.on("close", () => {
			console.log("[SSE] Client disconnected for compliance progress, api_id:", apiId);
			clearInterval(heartbeat);
			unsubscribe();
		});

		// Handle errors
		req.on("error", (err) => {
			if (err.code !== "ECONNRESET" && err.code !== "EPIPE") {
				console.error("[SSE] Compliance progress request error:", err);
			}
			clearInterval(heartbeat);
			unsubscribe();
		});

		res.on("error", (err) => {
			if (err.code !== "ECONNRESET" && err.code !== "EPIPE") {
				console.error("[SSE] Compliance progress response error:", err);
			}
			clearInterval(heartbeat);
			unsubscribe();
		});
	} catch (error) {
		console.error("[SSE] Unexpected error in compliance progress:", error);
		if (!res.headersSent) {
			res.status(500).json({ error: "Internal server error" });
		}
	}
});

module.exports = router;
