const express = require("express");
const fs = require("node:fs").promises;
const path = require("node:path");
const { authenticateToken } = require("../middleware/auth");

const router = express.Router();

// Helper to get current version
function getCurrentVersion() {
	try {
		const packageJson = require("../../package.json");
		return packageJson?.version || "unknown";
	} catch (error) {
		console.error("Could not read version from package.json:", error);
		return "unknown";
	}
}

// Get release notes for a specific version
router.get("/:version", authenticateToken, async (req, res) => {
	try {
		const { version } = req.params;
		const releaseNotesPath = path.join(
			__dirname,
			"../../release-notes",
			`RELEASE_NOTES_${version}.md`,
		);

		try {
			const content = await fs.readFile(releaseNotesPath, "utf-8");
			res.json({
				version,
				content,
				exists: true,
			});
		} catch (_fileError) {
			// File doesn't exist for this version
			res.json({
				version,
				content: null,
				exists: false,
			});
		}
	} catch (error) {
		console.error("Error fetching release notes:", error);
		res.status(500).json({ error: "Failed to fetch release notes" });
	}
});

// Get release notes for current version
router.get("/current", authenticateToken, async (_req, res) => {
	try {
		const currentVersion = getCurrentVersion();
		const releaseNotesPath = path.join(
			__dirname,
			"../../release-notes",
			`RELEASE_NOTES_${currentVersion}.md`,
		);

		try {
			const content = await fs.readFile(releaseNotesPath, "utf-8");
			res.json({
				version: currentVersion,
				content,
				exists: true,
			});
		} catch (_fileError) {
			// No release notes for current version
			res.json({
				version: currentVersion,
				content: null,
				exists: false,
			});
		}
	} catch (error) {
		console.error("Error fetching current release notes:", error);
		res.status(500).json({ error: "Failed to fetch release notes" });
	}
});

// List all available release notes versions
router.get("/", authenticateToken, async (_req, res) => {
	try {
		const releaseNotesDir = path.join(__dirname, "../../release-notes");
		const files = await fs.readdir(releaseNotesDir);

		const versions = files
			.filter(
				(file) => file.startsWith("RELEASE_NOTES_") && file.endsWith(".md"),
			)
			.map((file) => file.replace("RELEASE_NOTES_", "").replace(".md", ""))
			.sort((a, b) => {
				// Simple version comparison (you might want a more robust one)
				return b.localeCompare(a, undefined, { numeric: true });
			});

		res.json({ versions });
	} catch (error) {
		console.error("Error listing release notes:", error);
		res.status(500).json({ error: "Failed to list release notes" });
	}
});

module.exports = router;
