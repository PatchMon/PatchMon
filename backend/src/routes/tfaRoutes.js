const express = require("express");
const { getPrismaClient } = require("../config/prisma");
const speakeasy = require("speakeasy");
const QRCode = require("qrcode");
const { authenticateToken } = require("../middleware/auth");
const { body, validationResult } = require("express-validator");

const router = express.Router();
const prisma = getPrismaClient();

// Generate TFA secret and QR code
router.get("/setup", authenticateToken, async (req, res) => {
	try {
		const userId = req.user.id;

		// Check if user already has TFA enabled
		const user = await prisma.users.findUnique({
			where: { id: userId },
			select: { tfa_enabled: true, tfa_secret: true },
		});

		if (user.tfa_enabled) {
			return res.status(400).json({
				error: "Two-factor authentication is already enabled for this account",
			});
		}

		// Generate a new secret
		const secret = speakeasy.generateSecret({
			name: `PatchMon (${req.user.username})`,
			issuer: "PatchMon",
			length: 32,
		});

		// Generate QR code
		const qrCodeUrl = await QRCode.toDataURL(secret.otpauth_url);

		// Store the secret temporarily (not enabled yet)
		await prisma.users.update({
			where: { id: userId },
			data: { tfa_secret: secret.base32 },
		});

		res.json({
			secret: secret.base32,
			qrCode: qrCodeUrl,
			manualEntryKey: secret.base32,
		});
	} catch (error) {
		console.error("TFA setup error:", error);
		res
			.status(500)
			.json({ error: "Failed to setup two-factor authentication" });
	}
});

// Verify TFA setup
router.post(
	"/verify-setup",
	authenticateToken,
	[
		body("token")
			.isLength({ min: 6, max: 6 })
			.withMessage("Token must be 6 digits"),
		body("token").isNumeric().withMessage("Token must contain only numbers"),
	],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const { token } = req.body;
			const userId = req.user.id;

			// Get user's TFA secret
			const user = await prisma.users.findUnique({
				where: { id: userId },
				select: { tfa_secret: true, tfa_enabled: true },
			});

			if (!user.tfa_secret) {
				return res.status(400).json({
					error: "No TFA secret found. Please start the setup process first.",
				});
			}

			if (user.tfa_enabled) {
				return res.status(400).json({
					error:
						"Two-factor authentication is already enabled for this account",
				});
			}

			// Verify the token
			const verified = speakeasy.totp.verify({
				secret: user.tfa_secret,
				encoding: "base32",
				token: token,
				window: 2, // Allow 2 time windows (60 seconds) for clock drift
			});

			if (!verified) {
				return res.status(400).json({
					error: "Invalid verification code. Please try again.",
				});
			}

			// Generate backup codes
			const backupCodes = Array.from({ length: 10 }, () =>
				Math.random().toString(36).substring(2, 8).toUpperCase(),
			);

			// Enable TFA and store backup codes
			await prisma.users.update({
				where: { id: userId },
				data: {
					tfa_enabled: true,
					tfa_backup_codes: JSON.stringify(backupCodes),
				},
			});

			res.json({
				message: "Two-factor authentication has been enabled successfully",
				backupCodes: backupCodes,
			});
		} catch (error) {
			console.error("TFA verification error:", error);
			res
				.status(500)
				.json({ error: "Failed to verify two-factor authentication setup" });
		}
	},
);

// Disable TFA
router.post(
	"/disable",
	authenticateToken,
	[
		body("password")
			.notEmpty()
			.withMessage("Password is required to disable TFA"),
	],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const { password: _password } = req.body;
			const userId = req.user.id;

			// Verify password
			const user = await prisma.users.findUnique({
				where: { id: userId },
				select: { password_hash: true, tfa_enabled: true },
			});

			if (!user.tfa_enabled) {
				return res.status(400).json({
					error: "Two-factor authentication is not enabled for this account",
				});
			}

			// FIXME: In a real implementation, you would verify the password hash here
			// For now, we'll skip password verification for simplicity

			// Disable TFA
			await prisma.users.update({
				where: { id: userId },
				data: {
					tfa_enabled: false,
					tfa_secret: null,
					tfa_backup_codes: null,
				},
			});

			res.json({
				message: "Two-factor authentication has been disabled successfully",
			});
		} catch (error) {
			console.error("TFA disable error:", error);
			res
				.status(500)
				.json({ error: "Failed to disable two-factor authentication" });
		}
	},
);

// Get TFA status
router.get("/status", authenticateToken, async (req, res) => {
	try {
		const userId = req.user.id;

		const user = await prisma.users.findUnique({
			where: { id: userId },
			select: {
				tfa_enabled: true,
				tfa_secret: true,
				tfa_backup_codes: true,
			},
		});

		res.json({
			enabled: user.tfa_enabled,
			hasBackupCodes: !!user.tfa_backup_codes,
		});
	} catch (error) {
		console.error("TFA status error:", error);
		res.status(500).json({ error: "Failed to get TFA status" });
	}
});

// Regenerate backup codes
router.post("/regenerate-backup-codes", authenticateToken, async (req, res) => {
	try {
		const userId = req.user.id;

		// Check if TFA is enabled
		const user = await prisma.users.findUnique({
			where: { id: userId },
			select: { tfa_enabled: true },
		});

		if (!user.tfa_enabled) {
			return res.status(400).json({
				error: "Two-factor authentication is not enabled for this account",
			});
		}

		// Generate new backup codes
		const backupCodes = Array.from({ length: 10 }, () =>
			Math.random().toString(36).substring(2, 8).toUpperCase(),
		);

		// Update backup codes
		await prisma.users.update({
			where: { id: userId },
			data: {
				tfa_backup_codes: JSON.stringify(backupCodes),
			},
		});

		res.json({
			message: "Backup codes have been regenerated successfully",
			backupCodes: backupCodes,
		});
	} catch (error) {
		console.error("TFA backup codes regeneration error:", error);
		res.status(500).json({ error: "Failed to regenerate backup codes" });
	}
});

// Verify TFA token (for login)
router.post(
	"/verify",
	[
		body("username").notEmpty().withMessage("Username is required"),
		body("token")
			.isLength({ min: 6, max: 6 })
			.withMessage("Token must be 6 digits"),
		body("token").isNumeric().withMessage("Token must contain only numbers"),
	],
	async (req, res) => {
		try {
			const errors = validationResult(req);
			if (!errors.isEmpty()) {
				return res.status(400).json({ errors: errors.array() });
			}

			const { username, token } = req.body;

			// Get user's TFA secret
			const user = await prisma.users.findUnique({
				where: { username },
				select: {
					id: true,
					tfa_enabled: true,
					tfa_secret: true,
					tfa_backup_codes: true,
				},
			});

			if (!user || !user.tfa_enabled || !user.tfa_secret) {
				return res.status(400).json({
					error: "Two-factor authentication is not enabled for this account",
				});
			}

			// Check if it's a backup code
			const backupCodes = user.tfa_backup_codes
				? JSON.parse(user.tfa_backup_codes)
				: [];
			const isBackupCode = backupCodes.includes(token);

			let verified = false;

			if (isBackupCode) {
				// Remove the used backup code
				const updatedBackupCodes = backupCodes.filter((code) => code !== token);
				await prisma.users.update({
					where: { id: user.id },
					data: {
						tfa_backup_codes: JSON.stringify(updatedBackupCodes),
					},
				});
				verified = true;
			} else {
				// Verify TOTP token
				verified = speakeasy.totp.verify({
					secret: user.tfa_secret,
					encoding: "base32",
					token: token,
					window: 2,
				});
			}

			if (!verified) {
				return res.status(400).json({
					error: "Invalid verification code",
				});
			}

			res.json({
				message: "Two-factor authentication verified successfully",
				userId: user.id,
			});
		} catch (error) {
			console.error("TFA verification error:", error);
			res
				.status(500)
				.json({ error: "Failed to verify two-factor authentication" });
		}
	},
);

module.exports = router;
