const express = require("express");
const { getPrismaClient } = require("../config/prisma");
const crypto = require("node:crypto");
const bcrypt = require("bcryptjs");
const { body, validationResult } = require("express-validator");
const { authenticateToken } = require("../middleware/auth");
const { requireManageSettings } = require("../middleware/permissions");
const { v4: uuidv4 } = require("uuid");

const router = express.Router();
const prisma = getPrismaClient();

// Generate auto-enrollment token credentials
const generate_auto_enrollment_token = () => {
	const token_key = `patchmon_ae_${crypto.randomBytes(16).toString("hex")}`;
	const token_secret = crypto.randomBytes(48).toString("hex");
	return { token_key, token_secret };
};

// Middleware to validate auto-enrollment token
const validate_auto_enrollment_token = async (req, res, next) => {
	try {
		const token_key = req.headers["x-auto-enrollment-key"];
		const token_secret = req.headers["x-auto-enrollment-secret"];

		if (!token_key || !token_secret) {
			return res
				.status(401)
				.json({ error: "Auto-enrollment credentials required" });
		}

		// Find token
		const token = await prisma.auto_enrollment_tokens.findUnique({
			where: { token_key: token_key },
		});

		if (!token || !token.is_active) {
			return res.status(401).json({ error: "Invalid or inactive token" });
		}

		// Verify secret (hashed)
		const is_valid = await bcrypt.compare(token_secret, token.token_secret);
		if (!is_valid) {
			return res.status(401).json({ error: "Invalid token secret" });
		}

		// Check expiration
		if (token.expires_at && new Date() > new Date(token.expires_at)) {
			return res.status(401).json({ error: "Token expired" });
		}

		// Check IP whitelist if configured
		if (token.allowed_ip_ranges && token.allowed_ip_ranges.length > 0) {
			const client_ip = req.ip || req.connection.remoteAddress;
			// Basic IP check - can be enhanced with CIDR matching
			const ip_allowed = token.allowed_ip_ranges.some((allowed_ip) => {
				return client_ip.includes(allowed_ip);
			});

			if (!ip_allowed) {
				console.warn(
					`Auto-enrollment attempt from unauthorized IP: ${client_ip}`,
				);
				return res
					.status(403)
					.json({ error: "IP address not authorized for this token" });
			}
		}

		// Check rate limit (hosts per day)
		const today = new Date().toISOString().split("T")[0];
		const token_reset_date = token.last_reset_date.toISOString().split("T")[0];

		if (token_reset_date !== today) {
			// Reset daily counter
			await prisma.auto_enrollment_tokens.update({
				where: { id: token.id },
				data: {
					hosts_created_today: 0,
					last_reset_date: new Date(),
					updated_at: new Date(),
				},
			});
			token.hosts_created_today = 0;
		}

		if (token.hosts_created_today >= token.max_hosts_per_day) {
			return res.status(429).json({
				error: "Rate limit exceeded",
				message: `Maximum ${token.max_hosts_per_day} hosts per day allowed for this token`,
			});
		}

		req.auto_enrollment_token = token;
		next();
	} catch (error) {
		console.error("Auto-enrollment token validation error:", error);
		res.status(500).json({ error: "Token validation failed" });
	}
};

// ========== ADMIN ENDPOINTS (Manage Tokens) ==========

// Create auto-enrollment token
router.post(
	"/tokens",
	authenticateToken,
	requireManageSettings,
	[
		body("token_name")
			.isLength({ min: 1, max: 255 })
			.withMessage("Token name is required (max 255 characters)"),
		body("allowed_ip_ranges")
			.optional()
			.isArray()
			.withMessage("Allowed IP ranges must be an array"),
		body("max_hosts_per_day")
			.optional()
			.isInt({ min: 1, max: 1000 })
			.withMessage("Max hosts per day must be between 1 and 1000"),
		body("default_host_group_id")
			.optional({ nullable: true, checkFalsy: true })
			.isString(),
		body("expires_at")
			.optional({ nullable: true, checkFalsy: true })
			.isISO8601()
			.withMessage("Invalid date format"),
		body("scopes")
			.optional()
			.isObject()
			.withMessage("Scopes must be an object"),
	],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const {
				token_name,
				allowed_ip_ranges = [],
				max_hosts_per_day = 100,
				default_host_group_id,
				expires_at,
				metadata = {},
				scopes,
			} = req.body;

			// Validate host group if provided
			if (default_host_group_id) {
				const host_group = await prisma.host_groups.findUnique({
					where: { id: default_host_group_id },
				});

				if (!host_group) {
					return res.status(400).json({ error: "Host group not found" });
				}
			}

			// Validate scopes for API tokens
			if (metadata.integration_type === "api" && scopes) {
				// Validate scopes structure
				if (typeof scopes !== "object" || scopes === null) {
					return res.status(400).json({ error: "Scopes must be an object" });
				}

				// Validate each resource in scopes
				for (const [resource, actions] of Object.entries(scopes)) {
					if (!Array.isArray(actions)) {
						return res.status(400).json({
							error: `Scopes for resource "${resource}" must be an array of actions`,
						});
					}

					// Validate action names
					for (const action of actions) {
						if (typeof action !== "string") {
							return res.status(400).json({
								error: `All actions in scopes must be strings`,
							});
						}
					}
				}
			}

			const { token_key, token_secret } = generate_auto_enrollment_token();
			const hashed_secret = await bcrypt.hash(token_secret, 10);

			const token = await prisma.auto_enrollment_tokens.create({
				data: {
					id: uuidv4(),
					token_name,
					token_key: token_key,
					token_secret: hashed_secret,
					created_by_user_id: req.user.id,
					allowed_ip_ranges,
					max_hosts_per_day,
					default_host_group_id: default_host_group_id || null,
					expires_at: expires_at ? new Date(expires_at) : null,
					metadata: { integration_type: "proxmox-lxc", ...metadata },
					scopes: metadata.integration_type === "api" ? scopes || null : null,
					updated_at: new Date(),
				},
				include: {
					host_groups: {
						select: {
							id: true,
							name: true,
							color: true,
						},
					},
					users: {
						select: {
							id: true,
							username: true,
							first_name: true,
							last_name: true,
						},
					},
				},
			});

			// Return unhashed secret ONLY once (like API keys)
			res.status(201).json({
				message: "Auto-enrollment token created successfully",
				token: {
					id: token.id,
					token_name: token.token_name,
					token_key: token_key,
					token_secret: token_secret, // ONLY returned here!
					max_hosts_per_day: token.max_hosts_per_day,
					default_host_group: token.host_groups,
					created_by: token.users,
					expires_at: token.expires_at,
					scopes: token.scopes,
				},
				warning: "⚠️ Save the token_secret now - it cannot be retrieved later!",
			});
		} catch (error) {
			console.error("Create auto-enrollment token error:", error);
			res.status(500).json({ error: "Failed to create token" });
		}
	},
);

// List auto-enrollment tokens
router.get(
	"/tokens",
	authenticateToken,
	requireManageSettings,
	async (_req, res) => {
		try {
			const tokens = await prisma.auto_enrollment_tokens.findMany({
				select: {
					id: true,
					token_name: true,
					token_key: true,
					is_active: true,
					allowed_ip_ranges: true,
					max_hosts_per_day: true,
					hosts_created_today: true,
					last_used_at: true,
					expires_at: true,
					created_at: true,
					default_host_group_id: true,
					metadata: true,
					scopes: true,
					host_groups: {
						select: {
							id: true,
							name: true,
							color: true,
						},
					},
					users: {
						select: {
							id: true,
							username: true,
							first_name: true,
							last_name: true,
						},
					},
				},
				orderBy: { created_at: "desc" },
			});

			res.json(tokens);
		} catch (error) {
			console.error("List auto-enrollment tokens error:", error);
			res.status(500).json({ error: "Failed to list tokens" });
		}
	},
);

// Get single token details
router.get(
	"/tokens/:tokenId",
	authenticateToken,
	requireManageSettings,
	async (req, res) => {
		try {
			const { tokenId } = req.params;

			const token = await prisma.auto_enrollment_tokens.findUnique({
				where: { id: tokenId },
				include: {
					host_groups: {
						select: {
							id: true,
							name: true,
							color: true,
						},
					},
					users: {
						select: {
							id: true,
							username: true,
							first_name: true,
							last_name: true,
						},
					},
				},
			});

			if (!token) {
				return res.status(404).json({ error: "Token not found" });
			}

			// Don't include the secret in response
			const { token_secret: _secret, ...token_data } = token;

			res.json(token_data);
		} catch (error) {
			console.error("Get token error:", error);
			res.status(500).json({ error: "Failed to get token" });
		}
	},
);

// Update token (toggle active state, update limits, etc.)
router.patch(
	"/tokens/:tokenId",
	authenticateToken,
	requireManageSettings,
	[
		body("is_active").optional().isBoolean(),
		body("max_hosts_per_day").optional().isInt({ min: 1, max: 1000 }),
		body("allowed_ip_ranges").optional().isArray(),
		body("expires_at").optional().isISO8601(),
		body("scopes")
			.optional()
			.isObject()
			.withMessage("Scopes must be an object"),
	],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const { tokenId } = req.params;

			// First, get the existing token to check its integration type
			const existing_token = await prisma.auto_enrollment_tokens.findUnique({
				where: { id: tokenId },
			});

			if (!existing_token) {
				return res.status(404).json({ error: "Token not found" });
			}

			const update_data = { updated_at: new Date() };

			if (req.body.is_active !== undefined)
				update_data.is_active = req.body.is_active;
			if (req.body.max_hosts_per_day !== undefined)
				update_data.max_hosts_per_day = req.body.max_hosts_per_day;
			if (req.body.allowed_ip_ranges !== undefined)
				update_data.allowed_ip_ranges = req.body.allowed_ip_ranges;
			if (req.body.expires_at !== undefined)
				update_data.expires_at = new Date(req.body.expires_at);

			// Handle scopes updates for API tokens only
			if (req.body.scopes !== undefined) {
				if (existing_token.metadata?.integration_type === "api") {
					// Validate scopes structure
					const scopes = req.body.scopes;
					if (typeof scopes !== "object" || scopes === null) {
						return res.status(400).json({ error: "Scopes must be an object" });
					}

					// Validate each resource in scopes
					for (const [resource, actions] of Object.entries(scopes)) {
						if (!Array.isArray(actions)) {
							return res.status(400).json({
								error: `Scopes for resource "${resource}" must be an array of actions`,
							});
						}

						// Validate action names
						for (const action of actions) {
							if (typeof action !== "string") {
								return res.status(400).json({
									error: `All actions in scopes must be strings`,
								});
							}
						}
					}

					update_data.scopes = scopes;
				} else {
					return res.status(400).json({
						error: "Scopes can only be updated for API integration tokens",
					});
				}
			}

			const token = await prisma.auto_enrollment_tokens.update({
				where: { id: tokenId },
				data: update_data,
				include: {
					host_groups: true,
					users: {
						select: {
							username: true,
							first_name: true,
							last_name: true,
						},
					},
				},
			});

			const { token_secret: _secret, ...token_data } = token;

			res.json({
				message: "Token updated successfully",
				token: token_data,
			});
		} catch (error) {
			console.error("Update token error:", error);
			res.status(500).json({ error: "Failed to update token" });
		}
	},
);

// Delete token
router.delete(
	"/tokens/:tokenId",
	authenticateToken,
	requireManageSettings,
	async (req, res) => {
		try {
			const { tokenId } = req.params;

			const token = await prisma.auto_enrollment_tokens.findUnique({
				where: { id: tokenId },
			});

			if (!token) {
				return res.status(404).json({ error: "Token not found" });
			}

			await prisma.auto_enrollment_tokens.delete({
				where: { id: tokenId },
			});

			res.json({
				message: "Auto-enrollment token deleted successfully",
				deleted_token: {
					id: token.id,
					token_name: token.token_name,
				},
			});
		} catch (error) {
			console.error("Delete token error:", error);
			res.status(500).json({ error: "Failed to delete token" });
		}
	},
);

// ========== AUTO-ENROLLMENT ENDPOINTS (Used by Scripts) ==========
// Universal script-serving endpoint with type parameter
// Supported types:
//   - proxmox-lxc     - Proxmox LXC containers
//   - direct-host     - Direct host enrollment
// Future types:
//   - vmware-esxi     - VMware ESXi VMs
//   - docker          - Docker containers
//   - kubernetes      - Kubernetes pods

// Serve auto-enrollment scripts with credentials injected
router.get("/script", async (req, res) => {
	try {
		// Get parameters from query params
		const token_key = req.query.token_key;
		const token_secret = req.query.token_secret;
		const script_type = req.query.type;

		if (!token_key || !token_secret) {
			return res
				.status(401)
				.json({ error: "Token key and secret required as query parameters" });
		}

		if (!script_type) {
			return res.status(400).json({
				error:
					"Script type required as query parameter (e.g., ?type=proxmox-lxc or ?type=direct-host)",
			});
		}

		// Map script types to script file paths
		const scriptMap = {
			"proxmox-lxc": "proxmox_auto_enroll.sh",
			"direct-host": "direct_host_auto_enroll.sh",
		};

		if (!scriptMap[script_type]) {
			return res.status(400).json({
				error: `Invalid script type: ${script_type}. Supported types: ${Object.keys(scriptMap).join(", ")}`,
			});
		}

		// Validate token
		const token = await prisma.auto_enrollment_tokens.findUnique({
			where: { token_key: token_key },
		});

		if (!token || !token.is_active) {
			return res.status(401).json({ error: "Invalid or inactive token" });
		}

		// Verify secret
		const is_valid = await bcrypt.compare(token_secret, token.token_secret);
		if (!is_valid) {
			return res.status(401).json({ error: "Invalid token secret" });
		}

		// Check expiration
		if (token.expires_at && new Date() > new Date(token.expires_at)) {
			return res.status(401).json({ error: "Token expired" });
		}

		const fs = require("node:fs");
		const path = require("node:path");

		const script_path = path.join(
			__dirname,
			`../../../agents/${scriptMap[script_type]}`,
		);

		if (!fs.existsSync(script_path)) {
			return res.status(404).json({
				error: `Enrollment script not found: ${scriptMap[script_type]}`,
			});
		}

		let script = fs.readFileSync(script_path, "utf8");

		// Convert Windows line endings to Unix line endings
		script = script.replace(/\r\n/g, "\n").replace(/\r/g, "\n");

		// Get the configured server URL from settings
		let server_url = "http://localhost:3001";
		try {
			const settings = await prisma.settings.findFirst();
			if (settings?.server_url) {
				server_url = settings.server_url;
			}
		} catch (settings_error) {
			console.warn(
				"Could not fetch settings, using default server URL:",
				settings_error.message,
			);
		}

		// Determine curl flags dynamically from settings
		let curl_flags = "-s";
		try {
			const settings = await prisma.settings.findFirst();
			if (settings && settings.ignore_ssl_self_signed === true) {
				curl_flags = "-sk";
			}
		} catch (_) {}

		// Check for --force parameter
		const force_install = req.query.force === "true" || req.query.force === "1";

		// Inject the token credentials, server URL, curl flags, and force flag into the script
		const env_vars = `#!/bin/sh
# PatchMon Auto-Enrollment Configuration (Auto-generated)
export PATCHMON_URL="${server_url}"
export AUTO_ENROLLMENT_KEY="${token.token_key}"
export AUTO_ENROLLMENT_SECRET="${token_secret}"
export CURL_FLAGS="${curl_flags}"
export FORCE_INSTALL="${force_install ? "true" : "false"}"

`;

		// Remove the shebang and configuration section from the original script
		script = script.replace(/^#!/, "#");

		// Remove the configuration section (between # ===== CONFIGURATION ===== and the next # =====)
		script = script.replace(
			/# ===== CONFIGURATION =====[\s\S]*?(?=# ===== COLOR OUTPUT =====)/,
			"",
		);

		script = env_vars + script;

		res.setHeader("Content-Type", "text/plain");
		res.setHeader(
			"Content-Disposition",
			`inline; filename="${scriptMap[script_type]}"`,
		);
		res.send(script);
	} catch (error) {
		console.error("Script serve error:", error);
		res.status(500).json({ error: "Failed to serve enrollment script" });
	}
});

// Create host via auto-enrollment
router.post(
	"/enroll",
	validate_auto_enrollment_token,
	[
		body("friendly_name")
			.isLength({ min: 1, max: 255 })
			.withMessage("Friendly name is required"),
		body("machine_id")
			.optional()
			.isLength({ min: 1, max: 255 })
			.withMessage(
				"Machine ID must be between 1 and 255 characters if provided",
			),
		body("metadata").optional().isObject(),
	],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const { friendly_name, machine_id } = req.body;

			// Generate host API credentials
			const api_id = `patchmon_${crypto.randomBytes(8).toString("hex")}`;
			const api_key = crypto.randomBytes(32).toString("hex");

			// Create host (no duplicate check - using config.yml checking instead)
			const host = await prisma.hosts.create({
				data: {
					id: uuidv4(),
					machine_id,
					friendly_name,
					os_type: "unknown",
					os_version: "unknown",
					api_id: api_id,
					api_key: api_key,
					status: "pending",
					notes: `Auto-enrolled via ${req.auto_enrollment_token.token_name} on ${new Date().toISOString()}`,
					updated_at: new Date(),
				},
			});

			// Create host group membership if default host group is specified
			let hostGroupMembership = null;
			if (req.auto_enrollment_token.default_host_group_id) {
				hostGroupMembership = await prisma.host_group_memberships.create({
					data: {
						id: uuidv4(),
						host_id: host.id,
						host_group_id: req.auto_enrollment_token.default_host_group_id,
						created_at: new Date(),
					},
				});
			}

			// Update token usage stats
			await prisma.auto_enrollment_tokens.update({
				where: { id: req.auto_enrollment_token.id },
				data: {
					hosts_created_today: { increment: 1 },
					last_used_at: new Date(),
					updated_at: new Date(),
				},
			});

			console.log(
				`Auto-enrolled host: ${friendly_name} (${host.id}) via token: ${req.auto_enrollment_token.token_name}`,
			);

			// Get host group details for response if membership was created
			let hostGroup = null;
			if (hostGroupMembership) {
				hostGroup = await prisma.host_groups.findUnique({
					where: { id: req.auto_enrollment_token.default_host_group_id },
					select: {
						id: true,
						name: true,
						color: true,
					},
				});
			}

			res.status(201).json({
				message: "Host enrolled successfully",
				host: {
					id: host.id,
					friendly_name: host.friendly_name,
					api_id: api_id,
					api_key: api_key,
					host_group: hostGroup,
					status: host.status,
				},
			});
		} catch (error) {
			console.error("Auto-enrollment error:", error);
			res.status(500).json({ error: "Failed to enroll host" });
		}
	},
);

// Bulk enroll multiple hosts at once
router.post(
	"/enroll/bulk",
	validate_auto_enrollment_token,
	[
		body("hosts")
			.isArray({ min: 1, max: 50 })
			.withMessage("Hosts array required (max 50)"),
		body("hosts.*.friendly_name")
			.isLength({ min: 1 })
			.withMessage("Each host needs a friendly_name"),
	],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const { hosts } = req.body;

			// Check rate limit
			const remaining_quota =
				req.auto_enrollment_token.max_hosts_per_day -
				req.auto_enrollment_token.hosts_created_today;

			if (hosts.length > remaining_quota) {
				return res.status(429).json({
					error: "Rate limit exceeded",
					message: `Only ${remaining_quota} hosts remaining in daily quota`,
				});
			}

			const results = {
				success: [],
				failed: [],
				skipped: [],
			};

			for (const host_data of hosts) {
				try {
					const { friendly_name, machine_id } = host_data;

					// Generate credentials (no duplicate check - using config.yml checking instead)
					const api_id = `patchmon_${crypto.randomBytes(8).toString("hex")}`;
					const api_key = crypto.randomBytes(32).toString("hex");

					// Create host
					const host = await prisma.hosts.create({
						data: {
							id: uuidv4(),
							machine_id,
							friendly_name,
							os_type: "unknown",
							os_version: "unknown",
							api_id: api_id,
							api_key: api_key,
							status: "pending",
							notes: `Auto-enrolled via ${req.auto_enrollment_token.token_name} on ${new Date().toISOString()}`,
							updated_at: new Date(),
						},
					});

					// Create host group membership if default host group is specified
					if (req.auto_enrollment_token.default_host_group_id) {
						await prisma.host_group_memberships.create({
							data: {
								id: uuidv4(),
								host_id: host.id,
								host_group_id: req.auto_enrollment_token.default_host_group_id,
								created_at: new Date(),
							},
						});
					}

					results.success.push({
						id: host.id,
						friendly_name: host.friendly_name,
						api_id: api_id,
						api_key: api_key,
					});
				} catch (error) {
					results.failed.push({
						friendly_name: host_data.friendly_name,
						error: error.message,
					});
				}
			}

			// Update token usage stats
			if (results.success.length > 0) {
				await prisma.auto_enrollment_tokens.update({
					where: { id: req.auto_enrollment_token.id },
					data: {
						hosts_created_today: { increment: results.success.length },
						last_used_at: new Date(),
						updated_at: new Date(),
					},
				});
			}

			res.status(201).json({
				message: `Bulk enrollment completed: ${results.success.length} succeeded, ${results.failed.length} failed, ${results.skipped.length} skipped`,
				results,
			});
		} catch (error) {
			console.error("Bulk auto-enrollment error:", error);
			res.status(500).json({ error: "Failed to bulk enroll hosts" });
		}
	},
);

module.exports = router;
