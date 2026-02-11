const express = require("express");
const logger = require("../utils/logger");
const { body, validationResult } = require("express-validator");
const { getPrismaClient } = require("../config/prisma");
const { authenticateToken } = require("../middleware/auth");
const { requireManageSettings } = require("../middleware/permissions");
const { getSettings, updateSettings } = require("../services/settingsService");
const { verifyApiKey } = require("../utils/apiKeyUtils");

const router = express.Router();

/**
 * Sanitize SVG content to remove potentially dangerous elements
 * Removes: script tags, event handlers, external references, data URIs
 * @param {string} svgContent - Raw SVG content
 * @returns {string} - Sanitized SVG content
 */
function sanitizeSvg(svgContent) {
	// Remove script tags and their contents
	let sanitized = svgContent.replace(
		/<script\b[^<]*(?:(?!<\/script>)<[^<]*)*<\/script>/gi,
		"",
	);

	// Remove on* event handlers (onclick, onload, onerror, etc.)
	sanitized = sanitized.replace(/\s+on\w+\s*=\s*["'][^"']*["']/gi, "");
	sanitized = sanitized.replace(/\s+on\w+\s*=\s*[^\s>]+/gi, "");

	// Remove javascript: URLs
	sanitized = sanitized.replace(
		/href\s*=\s*["']javascript:[^"']*["']/gi,
		'href=""',
	);
	sanitized = sanitized.replace(
		/xlink:href\s*=\s*["']javascript:[^"']*["']/gi,
		'xlink:href=""',
	);

	// Remove data: URLs (can contain JavaScript)
	sanitized = sanitized.replace(/href\s*=\s*["']data:[^"']*["']/gi, 'href=""');
	sanitized = sanitized.replace(
		/xlink:href\s*=\s*["']data:[^"']*["']/gi,
		'xlink:href=""',
	);

	// Remove foreign object elements (can embed HTML/JS)
	sanitized = sanitized.replace(
		/<foreignObject\b[^<]*(?:(?!<\/foreignObject>)<[^<]*)*<\/foreignObject>/gi,
		"",
	);

	// Remove use elements with external references (security risk)
	sanitized = sanitized.replace(
		/<use\b[^>]*xlink:href\s*=\s*["']https?:\/\/[^"']*["'][^>]*\/?>/gi,
		"",
	);

	// Remove embed, object, iframe elements
	sanitized = sanitized.replace(/<embed\b[^>]*\/?>/gi, "");
	sanitized = sanitized.replace(
		/<object\b[^<]*(?:(?!<\/object>)<[^<]*)*<\/object>/gi,
		"",
	);
	sanitized = sanitized.replace(
		/<iframe\b[^<]*(?:(?!<\/iframe>)<[^<]*)*<\/iframe>/gi,
		"",
	);

	// Remove set and animate elements with dangerous attributes
	sanitized = sanitized.replace(
		/<set\b[^>]*attributeName\s*=\s*["']on\w+["'][^>]*\/?>/gi,
		"",
	);
	sanitized = sanitized.replace(
		/<animate\b[^>]*attributeName\s*=\s*["']on\w+["'][^>]*\/?>/gi,
		"",
	);

	return sanitized;
}
const prisma = getPrismaClient();

const { queueManager, QUEUE_NAMES } = require("../services/automation");

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
// Public settings endpoint - returns read-only settings that all authenticated users can view
// This allows users to see things like auto_update status without requiring can_manage_settings
router.get("/public", authenticateToken, async (_req, res) => {
	try {
		const settings = await getSettings();
		// Return only public/read-only settings
		res.json({
			auto_update: settings.auto_update || false,
		});
	} catch (error) {
		logger.error("Public settings fetch error:", error);
		res.status(500).json({ error: "Failed to fetch public settings" });
	}
});

router.get("/", authenticateToken, requireManageSettings, async (_req, res) => {
	try {
		const settings = await getSettings();
		if (process.env.ENABLE_LOGGING === "true") {
			logger.info("Returning settings");
		}
		res.json(settings);
	} catch (error) {
		logger.error("Settings fetch error:", error);
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
		body("defaultComplianceMode")
			.optional()
			.isIn(["disabled", "on-demand", "enabled"])
			.withMessage(
				"Default compliance mode must be one of: disabled, on-demand, enabled",
			),
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
		body("showGithubVersionOnLogin")
			.optional()
			.isBoolean()
			.withMessage("Show GitHub version on login must be a boolean"),
	],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				logger.info("Validation errors:", errors.array());
				return res.status(400).json({ errors: errors.array() });
			}

			const {
				serverProtocol,
				serverHost,
				serverPort,
				updateInterval,
				autoUpdate,
				defaultComplianceMode,
				ignoreSslSelfSigned,
				signupEnabled,
				defaultUserRole,
				githubRepoUrl,
				repositoryType,
				sshKeyPath,
				logoDark,
				logoLight,
				favicon,
				colorTheme,
				showGithubVersionOnLogin,
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
			if (defaultComplianceMode !== undefined)
				updateData.default_compliance_mode = defaultComplianceMode;
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
			if (colorTheme !== undefined) updateData.color_theme = colorTheme;
			if (showGithubVersionOnLogin !== undefined)
				updateData.show_github_version_on_login = showGithubVersionOnLogin;

			const updatedSettings = await updateSettings(
				currentSettings.id,
				updateData,
			);

			if (process.env.ENABLE_LOGGING === "true") {
				logger.info("Settings updated successfully");
			}

			// If update interval changed, enqueue persistent jobs for agents
			if (
				updateInterval !== undefined &&
				oldUpdateInterval !== updateData.update_interval
			) {
				logger.info(
					`Update interval changed from ${oldUpdateInterval} to ${updateData.update_interval} minutes. Enqueueing agent settings updates...`,
				);

				const hosts = await prisma.hosts.findMany({
					where: { status: "active" },
					select: { api_id: true },
				});

				const queue = queueManager.queues[QUEUE_NAMES.AGENT_COMMANDS];
				const jobs = hosts.map((h) => ({
					name: "settings_update",
					data: {
						api_id: h.api_id,
						type: "settings_update",
						update_interval: updateData.update_interval,
					},
					opts: { attempts: 10, backoff: { type: "exponential", delay: 5000 } },
				}));

				// Bulk add jobs
				await queue.addBulk(jobs);

				// Note: Queue-based delivery handles retries and ensures reliable delivery
				// No need for immediate broadcast as it would cause duplicate messages
			}

			res.json({
				message: "Settings updated successfully",
				settings: updatedSettings,
			});
		} catch (error) {
			logger.error("Settings update error:", error);
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
		logger.error("Server URL fetch error:", error);
		res.status(500).json({ error: "Failed to fetch server URL" });
	}
});

// Get login settings for public use (used by login screen)
router.get("/login-settings", async (_req, res) => {
	try {
		const settings = await getSettings();
		res.json({
			show_github_version_on_login:
				settings.show_github_version_on_login !== false,
			signup_enabled: settings.signup_enabled || false,
		});
	} catch (error) {
		logger.error("Failed to fetch login settings:", error);
		res.status(500).json({ error: "Failed to fetch login settings" });
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

		if (!host) {
			return res.status(401).json({ error: "Invalid API credentials" });
		}

		// Verify API key using bcrypt (or timing-safe comparison for legacy keys)
		const isValidKey = await verifyApiKey(apiKey, host.api_key);
		if (!isValidKey) {
			return res.status(401).json({ error: "Invalid API credentials" });
		}

		const settings = await getSettings();
		const interval = normalizeUpdateInterval(settings.update_interval || 60);
		res.json({
			updateInterval: interval,
			cronExpression: buildCronExpression(interval),
		});
	} catch (error) {
		logger.error("Update interval fetch error:", error);
		res.json({ updateInterval: 60, cronExpression: "0 * * * *" });
	}
});

// Get auto-update policy for agents (requires API authentication)
router.get("/auto-update", async (req, res) => {
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

		if (!host) {
			return res.status(401).json({ error: "Invalid API credentials" });
		}

		// Verify API key using bcrypt (or timing-safe comparison for legacy keys)
		const isValidKey = await verifyApiKey(apiKey, host.api_key);
		if (!isValidKey) {
			return res.status(401).json({ error: "Invalid API credentials" });
		}

		const settings = await getSettings();
		res.json({
			autoUpdate: settings.auto_update || false,
		});
	} catch (error) {
		logger.error("Auto-update fetch error:", error);
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
			const { logoType, fileContent } = req.body;

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

			// Validate file size (max 5MB for logos)
			const MAX_FILE_SIZE = 5 * 1024 * 1024;
			const estimatedSize = (fileContent.length * 3) / 4; // Approximate decoded size
			if (estimatedSize > MAX_FILE_SIZE) {
				return res.status(400).json({
					error: "File size exceeds maximum allowed (5MB)",
				});
			}

			const fs = require("node:fs").promises;
			const path = require("node:path");

			// Create assets directory if it doesn't exist
			// Priority: 1. ASSETS_DIR env var (Docker), 2. Development/public, 3. Production/dist
			let assetsDir;
			if (process.env.ASSETS_DIR) {
				// Docker: Use ASSETS_DIR environment variable (mounted volume)
				assetsDir = process.env.ASSETS_DIR;
			} else {
				// Local development: save to public/assets (served by Vite)
				// Local production: save to dist/assets (served by built app)
				const isDevelopment = process.env.NODE_ENV !== "production";
				assetsDir = isDevelopment
					? path.join(__dirname, "../../../frontend/public/assets")
					: path.join(__dirname, "../../../frontend/dist/assets");
			}
			const resolvedAssetsDir = path.resolve(assetsDir);
			await fs.mkdir(resolvedAssetsDir, { recursive: true });

			// Handle base64 data URLs - decode first to validate magic bytes
			let fileBuffer;
			if (fileContent.startsWith("data:")) {
				const base64Data = fileContent.split(",")[1];
				fileBuffer = Buffer.from(base64Data, "base64");
			} else {
				// Assume it's already base64
				fileBuffer = Buffer.from(fileContent, "base64");
			}

			// Magic byte validation for file type
			const PNG_MAGIC = Buffer.from([0x89, 0x50, 0x4e, 0x47]);
			const JPEG_MAGIC = Buffer.from([0xff, 0xd8, 0xff]);
			const isPng = fileBuffer.slice(0, 4).equals(PNG_MAGIC);
			const isJpeg = fileBuffer.slice(0, 3).equals(JPEG_MAGIC);
			const isSvg = fileBuffer.toString("utf8", 0, 100).includes("<svg");

			// Determine file extension based on actual content (magic bytes)
			let fileExtension;
			let fileName_final;

			if (logoType === "favicon") {
				if (!isSvg) {
					return res.status(400).json({
						error: "Favicon must be an SVG file",
					});
				}
				fileExtension = ".svg";
				fileName_final = "logo_square.svg";
			} else {
				// Validate and determine extension from magic bytes
				if (isPng) {
					fileExtension = ".png";
				} else if (isJpeg) {
					fileExtension = ".jpg";
				} else if (isSvg) {
					fileExtension = ".svg";
				} else {
					return res.status(400).json({
						error: "Invalid file type. Allowed: PNG, JPEG, SVG",
					});
				}
				fileName_final = `logo_${logoType}${fileExtension}`;
			}

			// SECURITY: Sanitize filename to prevent path traversal
			// Only allow alphanumeric, underscore, hyphen, and dot
			const sanitizedFileName = path
				.basename(fileName_final)
				.replace(/[^a-zA-Z0-9_.-]/g, "_");
			if (
				sanitizedFileName !== fileName_final ||
				sanitizedFileName.includes("..")
			) {
				return res.status(400).json({
					error: "Invalid filename",
				});
			}

			const filePath = path.join(resolvedAssetsDir, sanitizedFileName);

			// SECURITY: Verify final path is within assets directory
			if (!filePath.startsWith(resolvedAssetsDir + path.sep)) {
				return res.status(400).json({
					error: "Invalid file path",
				});
			}

			// Create backup of existing file
			try {
				const backupPath = `${filePath}.backup.${Date.now()}`;
				await fs.copyFile(filePath, backupPath);
				logger.info(`Created backup: ${backupPath}`);
			} catch (error) {
				// Ignore if original doesn't exist
				if (error.code !== "ENOENT") {
					logger.warn("Failed to create backup:", error.message);
				}
			}

			// Sanitize SVG content to prevent XSS attacks
			if (isSvg) {
				const svgContent = fileBuffer.toString("utf8");
				const sanitizedSvg = sanitizeSvg(svgContent);
				fileBuffer = Buffer.from(sanitizedSvg, "utf8");
				logger.info("SVG content sanitized for security");
			}

			// Write new logo file
			await fs.writeFile(filePath, fileBuffer);

			// Update settings with new logo path
			const settings = await getSettings();
			const logoPath = `/assets/${sanitizedFileName}`;

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
				fileName: sanitizedFileName,
				path: logoPath,
				size: stats.size,
				sizeFormatted: `${(stats.size / 1024).toFixed(1)} KB`,
			});
		} catch (error) {
			logger.error("Upload logo error:", error);
			// Provide actionable error messages for common Docker issues
			if (error.code === "EACCES" || error.code === "EPERM") {
				logger.error(
					`Permission denied writing to assets directory. ` +
						`Ensure the ASSETS_DIR (${process.env.ASSETS_DIR || "not set"}) is writable by the application user. ` +
						`In Docker, run: docker exec -u root <container> chmod 1777 ${process.env.ASSETS_DIR || "/app/assets"}`,
				);
				res.status(500).json({
					error:
						"Failed to upload logo: permission denied writing to assets directory. Check container volume permissions.",
				});
			} else if (error.code === "ENOSPC") {
				res.status(500).json({
					error: "Failed to upload logo: no disk space available.",
				});
			} else {
				res.status(500).json({ error: "Failed to upload logo" });
			}
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
			logger.error("Reset logo error:", error);
			res.status(500).json({ error: "Failed to reset logo" });
		}
	},
);

module.exports = router;
