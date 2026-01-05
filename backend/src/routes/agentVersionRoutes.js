const express = require("express");
const logger = require("../utils/logger");
const router = express.Router();
const agentVersionService = require("../services/agentVersionService");
const { authenticateToken } = require("../middleware/auth");
const { requirePermission } = require("../middleware/permissions");

// Test GitHub API connectivity
router.get(
	"/test-github",
	authenticateToken,
	requirePermission("can_manage_settings"),
	async (_req, res) => {
		try {
			const axios = require("axios");
			const response = await axios.get(
				"https://api.github.com/repos/MacJediWizard/PatchMonEnhanced-agent/releases",
				{
					timeout: 10000,
					headers: {
						"User-Agent": "PatchMon-Server/1.0",
						Accept: "application/vnd.github.v3+json",
					},
				},
			);

			res.json({
				success: true,
				status: response.status,
				releasesFound: response.data.length,
				latestRelease: response.data[0]?.tag_name || "No releases",
				rateLimitRemaining: response.headers["x-ratelimit-remaining"],
				rateLimitLimit: response.headers["x-ratelimit-limit"],
			});
		} catch (error) {
			logger.error("❌ GitHub API test failed:", error.message);
			// SECURITY: Only expose detailed error info in development
			const isDev = process.env.NODE_ENV === "development";
			res.status(500).json({
				success: false,
				error: isDev ? error.message : "GitHub API request failed",
				status: error.response?.status,
				statusText: error.response?.statusText,
				rateLimitRemaining: error.response?.headers["x-ratelimit-remaining"],
				rateLimitLimit: error.response?.headers["x-ratelimit-limit"],
			});
		}
	},
);

// Get current version information
router.get("/version", authenticateToken, async (_req, res) => {
	try {
		const versionInfo = await agentVersionService.getVersionInfo();
		logger.info(
			"📊 Version info response:",
			JSON.stringify(versionInfo, null, 2),
		);
		res.json(versionInfo);
	} catch (error) {
		logger.error("❌ Failed to get version info:", error.message);
		res.status(500).json({
			error: "Failed to get version information",
			details: error.message,
			status: "error",
		});
	}
});

// Refresh current version by executing agent binary
router.post(
	"/version/refresh",
	authenticateToken,
	requirePermission("can_manage_settings"),
	async (_req, res) => {
		try {
			logger.info("🔄 Refreshing current agent version...");
			const currentVersion = await agentVersionService.refreshCurrentVersion();
			logger.info("📊 Refreshed current version:", currentVersion);
			res.json({
				success: true,
				currentVersion: currentVersion,
				message: currentVersion
					? `Current version refreshed: ${currentVersion}`
					: "No agent binary found",
			});
		} catch (error) {
			logger.error("❌ Failed to refresh current version:", error.message);
			res.status(500).json({
				success: false,
				error: "Failed to refresh current version",
				details: error.message,
			});
		}
	},
);

// Download latest update
router.post(
	"/version/download",
	authenticateToken,
	requirePermission("can_manage_settings"),
	async (_req, res) => {
		try {
			logger.info("🔄 Downloading latest agent update...");
			const downloadResult = await agentVersionService.downloadLatestUpdate();
			logger.info(
				"📊 Download result:",
				JSON.stringify(downloadResult, null, 2),
			);
			res.json(downloadResult);
		} catch (error) {
			logger.error("❌ Failed to download latest update:", error.message);
			res.status(500).json({
				success: false,
				error: "Failed to download latest update",
				details: error.message,
			});
		}
	},
);

// Check for updates
router.post(
	"/version/check",
	authenticateToken,
	requirePermission("can_manage_settings"),
	async (_req, res) => {
		try {
			logger.info("🔄 Manual update check triggered");
			const updateInfo = await agentVersionService.checkForUpdates();
			logger.info(
				"📊 Update check result:",
				JSON.stringify(updateInfo, null, 2),
			);
			res.json(updateInfo);
		} catch (error) {
			logger.error("❌ Failed to check for updates:", error.message);
			res.status(500).json({ error: "Failed to check for updates" });
		}
	},
);

// Get available versions
router.get("/versions", authenticateToken, async (_req, res) => {
	try {
		const versions = await agentVersionService.getAvailableVersions();
		logger.info(
			"📦 Available versions response:",
			JSON.stringify(versions, null, 2),
		);
		res.json({ versions });
	} catch (error) {
		logger.error("❌ Failed to get available versions:", error.message);
		res.status(500).json({ error: "Failed to get available versions" });
	}
});

// Get binary information
router.get(
	"/binary/:version/:architecture",
	authenticateToken,
	async (req, res) => {
		try {
			const { version, architecture } = req.params;
			const binaryInfo = await agentVersionService.getBinaryInfo(
				version,
				architecture,
			);
			res.json(binaryInfo);
		} catch (error) {
			logger.error("❌ Failed to get binary info:", error.message);
			// SECURITY: Use generic error message in production
			res.status(404).json({
				error: process.env.NODE_ENV === "development"
					? error.message
					: "Binary not found for specified version and architecture",
			});
		}
	},
);

// Download agent binary
router.get(
	"/download/:version/:architecture",
	authenticateToken,
	async (req, res) => {
		try {
			const { version, architecture } = req.params;

			// Validate architecture
			if (!agentVersionService.supportedArchitectures.includes(architecture)) {
				return res.status(400).json({ error: "Unsupported architecture" });
			}

			await agentVersionService.serveBinary(version, architecture, res);
		} catch (error) {
			logger.error("❌ Failed to serve binary:", error.message);
			res.status(500).json({ error: "Failed to serve binary" });
		}
	},
);

// Get latest binary for architecture (for agents to query)
router.get("/latest/:architecture", async (req, res) => {
	try {
		const { architecture } = req.params;

		// Validate architecture
		if (!agentVersionService.supportedArchitectures.includes(architecture)) {
			return res.status(400).json({ error: "Unsupported architecture" });
		}

		const versionInfo = await agentVersionService.getVersionInfo();

		if (!versionInfo.latestVersion) {
			return res.status(404).json({ error: "No latest version available" });
		}

		const binaryInfo = await agentVersionService.getBinaryInfo(
			versionInfo.latestVersion,
			architecture,
		);

		res.json({
			version: binaryInfo.version,
			architecture: binaryInfo.architecture,
			size: binaryInfo.size,
			hash: binaryInfo.hash,
			downloadUrl: `/api/v1/agent/download/${binaryInfo.version}/${binaryInfo.architecture}`,
		});
	} catch (error) {
		logger.error("❌ Failed to get latest binary info:", error.message);
		res.status(500).json({ error: "Failed to get latest binary information" });
	}
});

// Push update notification to specific agent
router.post(
	"/notify-update/:apiId",
	authenticateToken,
	requirePermission("admin"),
	async (req, res) => {
		try {
			const { apiId } = req.params;
			const { version, force = false } = req.body;

			const versionInfo = await agentVersionService.getVersionInfo();
			const targetVersion = version || versionInfo.latestVersion;

			if (!targetVersion) {
				return res
					.status(400)
					.json({ error: "No version specified or available" });
			}

			// Import WebSocket service
			const { pushUpdateNotification } = require("../services/agentWs");

			// Push update notification via WebSocket
			pushUpdateNotification(apiId, {
				version: targetVersion,
				force,
				downloadUrl: `/api/v1/agent/latest/${req.body.architecture || "linux-amd64"}`,
				message: `Update available: ${targetVersion}`,
			});

			res.json({
				success: true,
				message: `Update notification sent to agent ${apiId}`,
				version: targetVersion,
			});
		} catch (error) {
			logger.error("❌ Failed to notify agent update:", error.message);
			res.status(500).json({ error: "Failed to notify agent update" });
		}
	},
);

// Push update notification to all agents
router.post(
	"/notify-update-all",
	authenticateToken,
	requirePermission("admin"),
	async (req, res) => {
		try {
			const { version, force = false } = req.body;

			const versionInfo = await agentVersionService.getVersionInfo();
			const targetVersion = version || versionInfo.latestVersion;

			if (!targetVersion) {
				return res
					.status(400)
					.json({ error: "No version specified or available" });
			}

			// Import WebSocket service
			const { pushUpdateNotificationToAll } = require("../services/agentWs");

			// Push update notification to all connected agents
			const result = await pushUpdateNotificationToAll({
				version: targetVersion,
				force,
				message: `Update available: ${targetVersion}`,
			});

			res.json({
				success: true,
				message: `Update notification sent to ${result.notifiedCount} agents`,
				version: targetVersion,
				notifiedCount: result.notifiedCount,
				failedCount: result.failedCount,
			});
		} catch (error) {
			logger.error("❌ Failed to notify all agents update:", error.message);
			res.status(500).json({ error: "Failed to notify all agents update" });
		}
	},
);

// Check if specific agent needs update and push notification
router.post(
	"/check-update/:apiId",
	authenticateToken,
	requirePermission("can_manage_settings"),
	async (req, res) => {
		try {
			const { apiId } = req.params;
			const { version, force = false } = req.body;

			if (!version) {
				return res.status(400).json({
					success: false,
					error: "Agent version is required",
				});
			}

			logger.info(
				`🔍 Checking update for agent ${apiId} (version: ${version})`,
			);
			const result = await agentVersionService.checkAndPushAgentUpdate(
				apiId,
				version,
				force,
			);
			logger.info(
				"📊 Agent update check result:",
				JSON.stringify(result, null, 2),
			);

			res.json({
				success: true,
				...result,
			});
		} catch (error) {
			logger.error("❌ Failed to check agent update:", error.message);
			res.status(500).json({
				success: false,
				error: "Failed to check agent update",
				details: error.message,
			});
		}
	},
);

// Push updates to all connected agents
router.post(
	"/push-updates-all",
	authenticateToken,
	requirePermission("can_manage_settings"),
	async (req, res) => {
		try {
			const { force = false } = req.body;

			logger.info(`🔄 Pushing updates to all agents (force: ${force})`);
			const result = await agentVersionService.checkAndPushUpdatesToAll(force);
			logger.info("📊 Bulk update result:", JSON.stringify(result, null, 2));

			res.json(result);
		} catch (error) {
			logger.error("❌ Failed to push updates to all agents:", error.message);
			res.status(500).json({
				success: false,
				error: "Failed to push updates to all agents",
				details: error.message,
			});
		}
	},
);

// Agent reports its version (for automatic update checking)
router.post("/report-version", authenticateToken, async (req, res) => {
	try {
		const { apiId, version } = req.body;

		if (!apiId || !version) {
			return res.status(400).json({
				success: false,
				error: "API ID and version are required",
			});
		}

		logger.info(`📊 Agent ${apiId} reported version: ${version}`);

		// Check if agent needs update and push notification if needed
		const updateResult = await agentVersionService.checkAndPushAgentUpdate(
			apiId,
			version,
		);

		res.json({
			success: true,
			message: "Version reported successfully",
			updateCheck: updateResult,
		});
	} catch (error) {
		logger.error("❌ Failed to process agent version report:", error.message);
		res.status(500).json({
			success: false,
			error: "Failed to process version report",
			details: error.message,
		});
	}
});

module.exports = router;
