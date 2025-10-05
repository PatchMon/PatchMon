const express = require("express");
const { body, validationResult } = require("express-validator");
const { PrismaClient } = require("@prisma/client");
const { authenticateToken } = require("../middleware/auth");
const { requireManageSettings } = require("../middleware/permissions");
const { getSettings, updateSettings } = require("../services/settingsService");

const router = express.Router();
const prisma = new PrismaClient();

// Function to trigger crontab updates on all hosts with auto-update enabled
async function triggerCrontabUpdates() {
	try {
		console.log(
			"Triggering crontab updates on all hosts with auto-update enabled...",
		);

		// Get current settings for server URL
		const settings = await getSettings();
		const serverUrl = settings.server_url;

		// Get all hosts that have auto-update enabled
		const hosts = await prisma.hosts.findMany({
			where: {
				auto_update: true,
				status: "active", // Only update active hosts
			},
			select: {
				id: true,
				friendly_name: true,
				api_id: true,
				api_key: true,
			},
		});

		console.log(`Found ${hosts.length} hosts with auto-update enabled`);

		// For each host, we'll send a special update command that triggers crontab update
		// This is done by sending a ping with a special flag
		for (const host of hosts) {
			try {
				console.log(
					`Triggering crontab update for host: ${host.friendly_name}`,
				);

				// We'll use the existing ping endpoint but add a special parameter
				// The agent will detect this and run update-crontab command
				const http = require("node:http");
				const https = require("node:https");

				const url = new URL(`${serverUrl}/api/v1/hosts/ping`);
				const isHttps = url.protocol === "https:";
				const client = isHttps ? https : http;

				const postData = JSON.stringify({
					triggerCrontabUpdate: true,
					message: "Update interval changed, please update your crontab",
				});

				const options = {
					hostname: url.hostname,
					port: url.port || (isHttps ? 443 : 80),
					path: url.pathname,
					method: "POST",
					headers: {
						"Content-Type": "application/json",
						"Content-Length": Buffer.byteLength(postData),
						"X-API-ID": host.api_id,
						"X-API-KEY": host.api_key,
					},
				};

				const req = client.request(options, (res) => {
					if (res.statusCode === 200) {
						console.log(
							`Successfully triggered crontab update for ${host.friendly_name}`,
						);
					} else {
						console.error(
							`Failed to trigger crontab update for ${host.friendly_name}: ${res.statusCode}`,
						);
					}
				});

				req.on("error", (error) => {
					console.error(
						`Error triggering crontab update for ${host.friendly_name}:`,
						error.message,
					);
				});

				req.write(postData);
				req.end();
			} catch (error) {
				console.error(
					`Error triggering crontab update for ${host.friendly_name}:`,
					error.message,
				);
			}
		}

		console.log("Crontab update trigger completed");
	} catch (error) {
		console.error("Error in triggerCrontabUpdates:", error);
	}
}

// Helpers
function normalizeUpdateInterval(minutes) {
	let m = parseInt(minutes, 10);
	if (Number.isNaN(m)) return 60;
	if (m < 5) m = 5;
	if (m > 1440) m = 1440;
	if (m < 60) {
		// Clamp to 5-59, step 5
		const snapped = Math.round(m / 5) * 5;
		return Math.min(59, Math.max(5, snapped));
	}
	// Allowed hour-based presets
	const allowed = [60, 120, 180, 360, 720, 1440];
	let nearest = allowed[0];
	let bestDiff = Math.abs(m - nearest);
	for (const a of allowed) {
		const d = Math.abs(m - a);
		if (d < bestDiff) {
			bestDiff = d;
			nearest = a;
		}
	}
	return nearest;
}

function buildCronExpression(minutes) {
	const m = normalizeUpdateInterval(minutes);
	if (m < 60) {
		return `*/${m} * * * *`;
	}
	if (m === 60) {
		// Hourly at current minute is chosen by agent; default 0 here
		return `0 * * * *`;
	}
	const hours = Math.floor(m / 60);
	// Every N hours at minute 0
	return `0 */${hours} * * *`;
}

// Get current settings
router.get("/", authenticateToken, requireManageSettings, async (_req, res) => {
	try {
		const settings = await getSettings();
		if (process.env.ENABLE_LOGGING === "true") {
			console.log("Returning settings:", settings);
		}
		res.json(settings);
	} catch (error) {
		console.error("Settings fetch error:", error);
		res.status(500).json({ error: "Failed to fetch settings" });
	}
});

// Update settings
router.put(
	"/",
	authenticateToken,
	requireManageSettings,
	[
		body("serverProtocol")
			.optional()
			.isIn(["http", "https"])
			.withMessage("Protocol must be http or https"),
		body("serverHost")
			.optional()
			.isLength({ min: 1 })
			.withMessage("Server host is required"),
		body("serverPort")
			.optional()
			.isInt({ min: 1, max: 65535 })
			.withMessage("Port must be between 1 and 65535"),
		body("updateInterval")
			.optional()
			.isInt({ min: 5, max: 1440 })
			.withMessage("Update interval must be between 5 and 1440 minutes"),
		body("autoUpdate")
			.optional()
			.isBoolean()
			.withMessage("Auto update must be a boolean"),
		body("ignoreSslSelfSigned")
			.optional()
			.isBoolean()
			.withMessage("Ignore SSL self-signed must be a boolean"),
		body("signupEnabled")
			.optional()
			.isBoolean()
			.withMessage("Signup enabled must be a boolean"),
		body("defaultUserRole")
			.optional()
			.isLength({ min: 1 })
			.withMessage("Default user role must be a non-empty string"),
		body("githubRepoUrl")
			.optional()
			.isLength({ min: 1 })
			.withMessage("GitHub repo URL must be a non-empty string"),
		body("repositoryType")
			.optional()
			.isIn(["public", "private"])
			.withMessage("Repository type must be public or private"),
		body("sshKeyPath")
			.optional()
			.custom((value) => {
				if (value && value.trim().length === 0) {
					return true; // Allow empty string
				}
				if (value && value.trim().length < 1) {
					throw new Error("SSH key path must be a non-empty string");
				}
				return true;
			}),
		body("logoDark")
			.optional()
			.isLength({ min: 1 })
			.withMessage("Logo dark path must be a non-empty string"),
		body("logoLight")
			.optional()
			.isLength({ min: 1 })
			.withMessage("Logo light path must be a non-empty string"),
		body("favicon")
			.optional()
			.isLength({ min: 1 })
			.withMessage("Favicon path must be a non-empty string"),
	],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				console.log("Validation errors:", errors.array());
				return res.status(400).json({ errors: errors.array() });
			}

			const {
				serverProtocol,
				serverHost,
				serverPort,
				updateInterval,
				autoUpdate,
				ignoreSslSelfSigned,
				signupEnabled,
				defaultUserRole,
				githubRepoUrl,
				repositoryType,
				sshKeyPath,
				logoDark,
				logoLight,
				favicon,
			} = req.body;

			// Get current settings to check for update interval changes
			const currentSettings = await getSettings();
			const oldUpdateInterval = currentSettings.update_interval;

			// Build update object with only provided fields
			const updateData = {};

			if (serverProtocol !== undefined)
				updateData.server_protocol = serverProtocol;
			if (serverHost !== undefined) updateData.server_host = serverHost;
			if (serverPort !== undefined) updateData.server_port = serverPort;
			if (updateInterval !== undefined) {
				updateData.update_interval = normalizeUpdateInterval(updateInterval);
			}
			if (autoUpdate !== undefined) updateData.auto_update = autoUpdate;
			if (ignoreSslSelfSigned !== undefined)
				updateData.ignore_ssl_self_signed = ignoreSslSelfSigned;
			if (signupEnabled !== undefined)
				updateData.signup_enabled = signupEnabled;
			if (defaultUserRole !== undefined)
				updateData.default_user_role = defaultUserRole;
			if (githubRepoUrl !== undefined)
				updateData.github_repo_url = githubRepoUrl;
			if (repositoryType !== undefined)
				updateData.repository_type = repositoryType;
			if (sshKeyPath !== undefined) updateData.ssh_key_path = sshKeyPath;
			if (logoDark !== undefined) updateData.logo_dark = logoDark;
			if (logoLight !== undefined) updateData.logo_light = logoLight;
			if (favicon !== undefined) updateData.favicon = favicon;

			const updatedSettings = await updateSettings(
				currentSettings.id,
				updateData,
			);

			console.log("Settings updated successfully:", updatedSettings);

			// If update interval changed, trigger crontab updates on all hosts with auto-update enabled
			if (
				updateInterval !== undefined &&
				oldUpdateInterval !== updateData.update_interval
			) {
				console.log(
					`Update interval changed from ${oldUpdateInterval} to ${updateData.update_interval} minutes. Triggering crontab updates...`,
				);
				await triggerCrontabUpdates();
			}

			res.json({
				message: "Settings updated successfully",
				settings: updatedSettings,
			});
		} catch (error) {
			console.error("Settings update error:", error);
			res.status(500).json({ error: "Failed to update settings" });
		}
	},
);

// Get server URL for public use (used by installation scripts)
router.get("/server-url", async (_req, res) => {
	try {
		const settings = await getSettings();
		const serverUrl = settings.server_url;
		res.json({ server_url: serverUrl });
	} catch (error) {
		console.error("Server URL fetch error:", error);
		res.status(500).json({ error: "Failed to fetch server URL" });
	}
});

// Get update interval policy for agents (requires API authentication)
router.get("/update-interval", async (req, res) => {
	try {
		// Verify API credentials
		const apiId = req.headers["x-api-id"];
		const apiKey = req.headers["x-api-key"];

		if (!apiId || !apiKey) {
			return res.status(401).json({ error: "API credentials required" });
		}

		// Validate API credentials
		const host = await prisma.hosts.findUnique({
			where: { api_id: apiId },
		});

		if (!host || host.api_key !== apiKey) {
			return res.status(401).json({ error: "Invalid API credentials" });
		}

		const settings = await getSettings();
		const interval = normalizeUpdateInterval(settings.update_interval || 60);
		res.json({
			updateInterval: interval,
			cronExpression: buildCronExpression(interval),
		});
	} catch (error) {
		console.error("Update interval fetch error:", error);
		res.json({ updateInterval: 60, cronExpression: "0 * * * *" });
	}
});

// Get auto-update policy for agents (public endpoint)
router.get("/auto-update", async (_req, res) => {
	try {
		const settings = await getSettings();
		res.json({
			autoUpdate: settings.auto_update || false,
		});
	} catch (error) {
		console.error("Auto-update fetch error:", error);
		res.json({ autoUpdate: false });
	}
});

// Upload logo files
router.post(
	"/logos/upload",
	authenticateToken,
	requireManageSettings,
	async (req, res) => {
		try {
			const { logoType, fileContent, fileName } = req.body;

			if (!logoType || !fileContent) {
				return res.status(400).json({
					error: "Logo type and file content are required",
				});
			}

			if (!["dark", "light", "favicon"].includes(logoType)) {
				return res.status(400).json({
					error: "Logo type must be 'dark', 'light', or 'favicon'",
				});
			}

			// Validate file content (basic checks)
			if (typeof fileContent !== "string") {
				return res.status(400).json({
					error: "File content must be a base64 string",
				});
			}

			const fs = require("node:fs").promises;
			const path = require("node:path");
			const _crypto = require("node:crypto");

			// Create assets directory if it doesn't exist
			// In development: save to public/assets (served by Vite)
			// In production: save to dist/assets (served by built app)
			const isDevelopment = process.env.NODE_ENV !== "production";
			const assetsDir = isDevelopment
				? path.join(__dirname, "../../../frontend/public/assets")
				: path.join(__dirname, "../../../frontend/dist/assets");
			await fs.mkdir(assetsDir, { recursive: true });

			// Determine file extension and path
			let fileExtension;
			let fileName_final;

			if (logoType === "favicon") {
				fileExtension = ".svg";
				fileName_final = fileName || "logo_square.svg";
			} else {
				// Determine extension from file content or use default
				if (fileContent.startsWith("data:image/png")) {
					fileExtension = ".png";
				} else if (fileContent.startsWith("data:image/svg")) {
					fileExtension = ".svg";
				} else if (
					fileContent.startsWith("data:image/jpeg") ||
					fileContent.startsWith("data:image/jpg")
				) {
					fileExtension = ".jpg";
				} else {
					fileExtension = ".png"; // Default to PNG
				}
				fileName_final = fileName || `logo_${logoType}${fileExtension}`;
			}

			const filePath = path.join(assetsDir, fileName_final);

			// Handle base64 data URLs
			let fileBuffer;
			if (fileContent.startsWith("data:")) {
				const base64Data = fileContent.split(",")[1];
				fileBuffer = Buffer.from(base64Data, "base64");
			} else {
				// Assume it's already base64
				fileBuffer = Buffer.from(fileContent, "base64");
			}

			// Create backup of existing file
			try {
				const backupPath = `${filePath}.backup.${Date.now()}`;
				await fs.copyFile(filePath, backupPath);
				console.log(`Created backup: ${backupPath}`);
			} catch (error) {
				// Ignore if original doesn't exist
				if (error.code !== "ENOENT") {
					console.warn("Failed to create backup:", error.message);
				}
			}

			// Write new logo file
			await fs.writeFile(filePath, fileBuffer);

			// Update settings with new logo path
			const settings = await getSettings();
			const logoPath = `/assets/${fileName_final}`;

			const updateData = {};
			if (logoType === "dark") {
				updateData.logo_dark = logoPath;
			} else if (logoType === "light") {
				updateData.logo_light = logoPath;
			} else if (logoType === "favicon") {
				updateData.favicon = logoPath;
			}

			await updateSettings(settings.id, updateData);

			// Get file stats
			const stats = await fs.stat(filePath);

			res.json({
				message: `${logoType} logo uploaded successfully`,
				fileName: fileName_final,
				path: logoPath,
				size: stats.size,
				sizeFormatted: `${(stats.size / 1024).toFixed(1)} KB`,
			});
		} catch (error) {
			console.error("Upload logo error:", error);
			res.status(500).json({ error: "Failed to upload logo" });
		}
	},
);

// Reset logo to default
router.post(
	"/logos/reset",
	authenticateToken,
	requireManageSettings,
	async (req, res) => {
		try {
			const { logoType } = req.body;

			if (!logoType) {
				return res.status(400).json({
					error: "Logo type is required",
				});
			}

			if (!["dark", "light", "favicon"].includes(logoType)) {
				return res.status(400).json({
					error: "Logo type must be 'dark', 'light', or 'favicon'",
				});
			}

			// Get current settings
			const settings = await getSettings();

			// Clear the custom logo path to revert to default
			const updateData = {};
			if (logoType === "dark") {
				updateData.logo_dark = null;
			} else if (logoType === "light") {
				updateData.logo_light = null;
			} else if (logoType === "favicon") {
				updateData.favicon = null;
			}

			await updateSettings(settings.id, updateData);

			res.json({
				message: `${logoType} logo reset to default successfully`,
				logoType,
			});
		} catch (error) {
			console.error("Reset logo error:", error);
			res.status(500).json({ error: "Failed to reset logo" });
		}
	},
);

module.exports = router;
