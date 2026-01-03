const axios = require("axios");
const fs = require("node:fs").promises;
const path = require("node:path");
const os = require("node:os");
const { exec, spawn } = require("node:child_process");
const { promisify } = require("node:util");
const _execAsync = promisify(exec);

// Simple semver comparison function
function compareVersions(version1, version2) {
	const v1parts = version1.split(".").map(Number);
	const v2parts = version2.split(".").map(Number);

	// Ensure both arrays have the same length
	while (v1parts.length < 3) v1parts.push(0);
	while (v2parts.length < 3) v2parts.push(0);

	for (let i = 0; i < 3; i++) {
		if (v1parts[i] > v2parts[i]) return 1;
		if (v1parts[i] < v2parts[i]) return -1;
	}
	return 0;
}
const crypto = require("node:crypto");

class AgentVersionService {
	constructor() {
		this.githubApiUrl =
			"https://api.github.com/repos/MacJediWizard/PatchMonEnhanced-agent/releases";
		this.agentsDir = path.resolve(__dirname, "../../../agents");
		this.supportedArchitectures = [
			"linux-amd64",
			"linux-arm64",
			"linux-386",
			"linux-arm",
		];
		this.currentVersion = null;
		this.latestVersion = null;
		this.lastChecked = null;
		this.checkInterval = 30 * 60 * 1000; // 30 minutes
	}

	async initialize() {
		try {
			// Ensure agents directory exists
			await fs.mkdir(this.agentsDir, { recursive: true });

			console.log("🔍 Testing GitHub API connectivity...");
			try {
				const testResponse = await axios.get(
					"https://api.github.com/repos/MacJediWizard/PatchMonEnhanced-agent/releases",
					{
						timeout: 5000,
						headers: {
							"User-Agent": "PatchMon-Server/1.0",
							Accept: "application/vnd.github.v3+json",
						},
					},
				);
				console.log(
					`✅ GitHub API accessible - found ${testResponse.data.length} releases`,
				);
			} catch (testError) {
				console.error("❌ GitHub API not accessible:", testError.message);
				if (testError.response) {
					console.error(
						"❌ Status:",
						testError.response.status,
						testError.response.statusText,
					);
					if (testError.response.status === 403) {
						console.log("⚠️ GitHub API rate limit exceeded - will retry later");
					}
				}
			}

			// Get current agent version by executing the binary
			await this.getCurrentAgentVersion();

			// Try to check for updates, but don't fail initialization if GitHub API is unavailable
			try {
				await this.checkForUpdates();
			} catch (updateError) {
				console.log(
					"⚠️ Failed to check for updates on startup, will retry later:",
					updateError.message,
				);
			}

			// Set up periodic checking
			setInterval(() => {
				this.checkForUpdates().catch((error) => {
					console.log("⚠️ Periodic update check failed:", error.message);
				});
			}, this.checkInterval);

			console.log("✅ Agent Version Service initialized");
		} catch (error) {
			console.error(
				"❌ Failed to initialize Agent Version Service:",
				error.message,
			);
		}
	}

	async getCurrentAgentVersion() {
		try {
			console.log("🔍 Getting current agent version...");

			// Detect server architecture and map to Go architecture names
			const serverArch = os.arch();
			// Map Node.js architecture to Go architecture names
			const archMap = {
				x64: "amd64",
				ia32: "386",
				arm64: "arm64",
				arm: "arm",
			};
			const serverGoArch = archMap[serverArch] || serverArch;

			console.log(
				`🔍 Detected server architecture: ${serverArch} -> ${serverGoArch}`,
			);

			// Try to find the agent binary in agents/ folder based on server architecture
			const possiblePaths = [
				path.join(this.agentsDir, `patchmonenhanced-agent-linux-${serverGoArch}`),
				path.join(this.agentsDir, "patchmonenhanced-agent-linux-amd64"), // Fallback
				path.join(this.agentsDir, "patchmon-agent"), // Legacy fallback
			];

			let agentPath = null;
			for (const testPath of possiblePaths) {
				try {
					await fs.access(testPath);
					agentPath = testPath;
					console.log(`✅ Found agent binary at: ${testPath}`);
					break;
				} catch {
					// Path doesn't exist, continue to next
				}
			}

			if (!agentPath) {
				console.log(
					`⚠️ No agent binary found in agents/ folder for architecture ${serverGoArch}, current version will be unknown`,
				);
				console.log("💡 Use the Download Updates button to get agent binaries");
				this.currentVersion = null;
				return;
			}

			// Execute the agent binary with help flag to get version info
			try {
				const child = spawn(agentPath, ["--help"], {
					timeout: 10000,
				});

				let stdout = "";
				let stderr = "";

				child.stdout.on("data", (data) => {
					stdout += data.toString();
				});

				child.stderr.on("data", (data) => {
					stderr += data.toString();
				});

				const result = await new Promise((resolve, reject) => {
					child.on("close", (code) => {
						resolve({ stdout, stderr, code });
					});
					child.on("error", reject);
				});

				if (result.stderr) {
					console.log("⚠️ Agent help stderr:", result.stderr);
				}

				// Parse version from help output (e.g., "PatchMon Agent v1.3.0")
				const versionMatch = result.stdout.match(
					/PatchMon Agent v([0-9]+\.[0-9]+\.[0-9]+)/i,
				);
				if (versionMatch) {
					this.currentVersion = versionMatch[1];
					console.log(`✅ Current agent version: ${this.currentVersion}`);
				} else {
					console.log(
						"⚠️ Could not parse version from agent help output:",
						result.stdout,
					);
					this.currentVersion = null;
				}
			} catch (execError) {
				console.error("❌ Failed to execute agent binary:", execError.message);
				this.currentVersion = null;
			}
		} catch (error) {
			console.error("❌ Failed to get current agent version:", error.message);
			this.currentVersion = null;
		}
	}

	async checkForUpdates() {
		try {
			console.log("🔍 Checking for agent updates...");

			const response = await axios.get(this.githubApiUrl, {
				timeout: 10000,
				headers: {
					"User-Agent": "PatchMon-Server/1.0",
					Accept: "application/vnd.github.v3+json",
				},
			});

			console.log(`📡 GitHub API response status: ${response.status}`);
			console.log(`📦 Found ${response.data.length} releases`);

			const releases = response.data;
			if (releases.length === 0) {
				console.log("ℹ️ No releases found");
				this.latestVersion = null;
				this.lastChecked = new Date();
				return {
					latestVersion: null,
					currentVersion: this.currentVersion,
					hasUpdate: false,
					lastChecked: this.lastChecked,
				};
			}

			const latestRelease = releases[0];
			this.latestVersion = latestRelease.tag_name.replace("v", ""); // Remove 'v' prefix
			this.lastChecked = new Date();

			console.log(`📦 Latest agent version: ${this.latestVersion}`);

			// Don't download binaries automatically - only when explicitly requested
			console.log(
				"ℹ️ Skipping automatic binary download - binaries will be downloaded on demand",
			);

			return {
				latestVersion: this.latestVersion,
				currentVersion: this.currentVersion,
				hasUpdate: this.currentVersion !== this.latestVersion,
				lastChecked: this.lastChecked,
			};
		} catch (error) {
			console.error("❌ Failed to check for updates:", error.message);
			if (error.response) {
				console.error(
					"❌ GitHub API error:",
					error.response.status,
					error.response.statusText,
				);
				console.error(
					"❌ Rate limit info:",
					error.response.headers["x-ratelimit-remaining"],
					"/",
					error.response.headers["x-ratelimit-limit"],
				);
			}
			throw error;
		}
	}

	async downloadBinariesToAgentsFolder(release) {
		try {
			console.log(
				`⬇️ Downloading binaries for version ${release.tag_name} to agents folder...`,
			);

			for (const arch of this.supportedArchitectures) {
				const assetName = `patchmonenhanced-agent-${arch}`;
				const asset = release.assets.find((a) => a.name === assetName);

				if (!asset) {
					console.warn(`⚠️ Binary not found for architecture: ${arch}`);
					continue;
				}

				const binaryPath = path.join(this.agentsDir, assetName);

				console.log(`⬇️ Downloading ${assetName}...`);

				const response = await axios.get(asset.browser_download_url, {
					responseType: "stream",
					timeout: 60000,
				});

				const writer = require("node:fs").createWriteStream(binaryPath);
				response.data.pipe(writer);

				await new Promise((resolve, reject) => {
					writer.on("finish", resolve);
					writer.on("error", reject);
				});

				// Make executable
				await fs.chmod(binaryPath, "755");

				console.log(`✅ Downloaded: ${assetName} to agents folder`);
			}
		} catch (error) {
			console.error(
				"❌ Failed to download binaries to agents folder:",
				error.message,
			);
			throw error;
		}
	}

	async downloadBinaryForVersion(version, architecture) {
		try {
			console.log(
				`⬇️ Downloading binary for version ${version} architecture ${architecture}...`,
			);

			// Get the release info from GitHub
			const response = await axios.get(this.githubApiUrl, {
				timeout: 10000,
				headers: {
					"User-Agent": "PatchMon-Server/1.0",
					Accept: "application/vnd.github.v3+json",
				},
			});

			const releases = response.data;
			const release = releases.find(
				(r) => r.tag_name.replace("v", "") === version,
			);

			if (!release) {
				throw new Error(`Release ${version} not found`);
			}

			const assetName = `patchmonenhanced-agent-linux-${architecture}`;
			const asset = release.assets.find((a) => a.name === assetName);

			if (!asset) {
				throw new Error(`Binary not found for architecture: ${architecture}`);
			}

			const binaryPath = path.join(
				this.agentBinariesDir,
				`${release.tag_name}-${assetName}`,
			);

			console.log(`⬇️ Downloading ${assetName}...`);

			const downloadResponse = await axios.get(asset.browser_download_url, {
				responseType: "stream",
				timeout: 60000,
			});

			const writer = require("node:fs").createWriteStream(binaryPath);
			downloadResponse.data.pipe(writer);

			await new Promise((resolve, reject) => {
				writer.on("finish", resolve);
				writer.on("error", reject);
			});

			// Make executable
			await fs.chmod(binaryPath, "755");

			console.log(`✅ Downloaded: ${assetName}`);
			return binaryPath;
		} catch (error) {
			console.error(
				`❌ Failed to download binary ${version}-${architecture}:`,
				error.message,
			);
			throw error;
		}
	}

	async getBinaryPath(version, architecture) {
		const binaryName = `patchmonenhanced-agent-${architecture}`;
		const binaryPath = path.join(this.agentsDir, binaryName);

		try {
			await fs.access(binaryPath);
			return binaryPath;
		} catch {
			throw new Error(`Binary not found: ${binaryName} version ${version}`);
		}
	}

	async serveBinary(version, architecture, res) {
		try {
			// Check if binary exists, if not download it
			const binaryPath = await this.getBinaryPath(version, architecture);
			const stats = await fs.stat(binaryPath);

			res.setHeader("Content-Type", "application/octet-stream");
			res.setHeader(
				"Content-Disposition",
				`attachment; filename="patchmonenhanced-agent-linux-${architecture}"`,
			);
			res.setHeader("Content-Length", stats.size);

			// Add cache headers
			res.setHeader("Cache-Control", "public, max-age=3600");
			res.setHeader("ETag", `"${version}-${architecture}"`);

			const stream = require("node:fs").createReadStream(binaryPath);
			stream.pipe(res);
		} catch (_error) {
			// Binary doesn't exist, try to download it
			console.log(
				`⬇️ Binary not found locally, attempting to download ${version}-${architecture}...`,
			);
			try {
				await this.downloadBinaryForVersion(version, architecture);
				// Retry serving the binary
				const binaryPath = await this.getBinaryPath(version, architecture);
				const stats = await fs.stat(binaryPath);

				res.setHeader("Content-Type", "application/octet-stream");
				res.setHeader(
					"Content-Disposition",
					`attachment; filename="patchmonenhanced-agent-linux-${architecture}"`,
				);
				res.setHeader("Content-Length", stats.size);
				res.setHeader("Cache-Control", "public, max-age=3600");
				res.setHeader("ETag", `"${version}-${architecture}"`);

				const stream = require("node:fs").createReadStream(binaryPath);
				stream.pipe(res);
			} catch (downloadError) {
				console.error(
					`❌ Failed to download binary ${version}-${architecture}:`,
					downloadError.message,
				);
				res
					.status(404)
					.json({ error: "Binary not found and could not be downloaded" });
			}
		}
	}

	async getVersionInfo() {
		let hasUpdate = false;
		let updateStatus = "unknown";

		// Latest version should ALWAYS come from GitHub, not from local binaries
		// currentVersion = what's installed locally
		// latestVersion = what's available on GitHub
		if (this.latestVersion) {
			console.log(`📦 Latest version from GitHub: ${this.latestVersion}`);
		} else {
			console.log(
				`⚠️ No GitHub release version available (API may be unavailable)`,
			);
		}

		if (this.currentVersion) {
			console.log(`💾 Current local agent version: ${this.currentVersion}`);
		} else {
			console.log(`⚠️ No local agent binary found`);
		}

		// Determine update status by comparing current vs latest (from GitHub)
		if (this.currentVersion && this.latestVersion) {
			const comparison = compareVersions(
				this.currentVersion,
				this.latestVersion,
			);
			if (comparison < 0) {
				hasUpdate = true;
				updateStatus = "update-available";
			} else if (comparison > 0) {
				hasUpdate = false;
				updateStatus = "newer-version";
			} else {
				hasUpdate = false;
				updateStatus = "up-to-date";
			}
		} else if (this.latestVersion && !this.currentVersion) {
			hasUpdate = true;
			updateStatus = "no-agent";
		} else if (this.currentVersion && !this.latestVersion) {
			// We have a current version but no latest version (GitHub API unavailable)
			hasUpdate = false;
			updateStatus = "github-unavailable";
		} else if (!this.currentVersion && !this.latestVersion) {
			updateStatus = "no-data";
		}

		return {
			currentVersion: this.currentVersion,
			latestVersion: this.latestVersion, // Always return GitHub version, not local
			hasUpdate: hasUpdate,
			updateStatus: updateStatus,
			lastChecked: this.lastChecked,
			supportedArchitectures: this.supportedArchitectures,
			status: this.latestVersion ? "ready" : "no-releases",
		};
	}

	async refreshCurrentVersion() {
		await this.getCurrentAgentVersion();
		return this.currentVersion;
	}

	async downloadLatestUpdate() {
		try {
			console.log("⬇️ Downloading latest agent update...");

			// First check for updates to get the latest release info
			const _updateInfo = await this.checkForUpdates();

			if (!this.latestVersion) {
				throw new Error("No latest version available to download");
			}

			// Get the release info from GitHub
			const response = await axios.get(this.githubApiUrl, {
				timeout: 10000,
				headers: {
					"User-Agent": "PatchMon-Server/1.0",
					Accept: "application/vnd.github.v3+json",
				},
			});

			const releases = response.data;
			const latestRelease = releases[0];

			if (!latestRelease) {
				throw new Error("No releases found");
			}

			console.log(
				`⬇️ Downloading binaries for version ${latestRelease.tag_name}...`,
			);

			// Download binaries for all architectures directly to agents folder
			await this.downloadBinariesToAgentsFolder(latestRelease);

			console.log("✅ Latest update downloaded successfully");

			return {
				success: true,
				version: this.latestVersion,
				downloadedArchitectures: this.supportedArchitectures,
				message: `Successfully downloaded version ${this.latestVersion}`,
			};
		} catch (error) {
			console.error("❌ Failed to download latest update:", error.message);
			throw error;
		}
	}

	async getAvailableVersions() {
		// No local caching - only return latest from GitHub
		if (this.latestVersion) {
			return [this.latestVersion];
		}
		return [];
	}

	async getBinaryInfo(version, architecture) {
		try {
			// Always use local version if it matches the requested version
			if (version === this.currentVersion && this.currentVersion) {
				const binaryPath = await this.getBinaryPath(
					this.currentVersion,
					architecture,
				);
				const stats = await fs.stat(binaryPath);

				// Calculate file hash
				const fileBuffer = await fs.readFile(binaryPath);
				const hash = crypto
					.createHash("sha256")
					.update(fileBuffer)
					.digest("hex");

				return {
					version: this.currentVersion,
					architecture,
					size: stats.size,
					hash,
					lastModified: stats.mtime,
					path: binaryPath,
				};
			}

			// For other versions, try to find them in the agents folder
			const binaryPath = await this.getBinaryPath(version, architecture);
			const stats = await fs.stat(binaryPath);

			// Calculate file hash
			const fileBuffer = await fs.readFile(binaryPath);
			const hash = crypto.createHash("sha256").update(fileBuffer).digest("hex");

			return {
				version,
				architecture,
				size: stats.size,
				hash,
				lastModified: stats.mtime,
				path: binaryPath,
			};
		} catch (error) {
			throw new Error(`Failed to get binary info: ${error.message}`);
		}
	}

	/**
	 * Check if an agent needs an update and push notification if needed
	 * @param {string} agentApiId - The agent's API ID
	 * @param {string} agentVersion - The agent's current version
	 * @param {boolean} force - Force update regardless of version
	 * @returns {Object} Update check result
	 */
	async checkAndPushAgentUpdate(agentApiId, agentVersion, force = false) {
		try {
			console.log(
				`🔍 Checking update for agent ${agentApiId} (version: ${agentVersion})`,
			);

			// Check general server auto_update setting
			const { getPrismaClient } = require("../config/prisma");
			const prisma = getPrismaClient();
			const settings = await prisma.settings.findFirst();
			if (!settings || !settings.auto_update) {
				console.log(
					`⚠️ Auto-update is disabled in server settings, skipping update check for agent ${agentApiId}`,
				);
				return {
					needsUpdate: false,
					reason: "auto-update-disabled-server",
					message: "Auto-update is disabled in server settings",
				};
			}

			// Check per-host auto_update setting
			const host = await prisma.hosts.findUnique({
				where: { api_id: agentApiId },
				select: { auto_update: true },
			});

			if (!host) {
				console.log(
					`⚠️ Host not found for agent ${agentApiId}, skipping update check`,
				);
				return {
					needsUpdate: false,
					reason: "host-not-found",
					message: "Host not found",
				};
			}

			if (!host.auto_update) {
				console.log(
					`⚠️ Auto-update is disabled for host ${agentApiId}, skipping update check`,
				);
				return {
					needsUpdate: false,
					reason: "auto-update-disabled-host",
					message: "Auto-update is disabled for this host",
				};
			}

			// Get current server version info
			const versionInfo = await this.getVersionInfo();

			if (!versionInfo.latestVersion) {
				console.log(`⚠️ No latest version available for agent ${agentApiId}`);
				return {
					needsUpdate: false,
					reason: "no-latest-version",
					message: "No latest version available on server",
				};
			}

			// Compare versions
			const comparison = compareVersions(
				agentVersion,
				versionInfo.latestVersion,
			);
			const needsUpdate = force || comparison < 0;

			if (needsUpdate) {
				console.log(
					`📤 Agent ${agentApiId} needs update: ${agentVersion} → ${versionInfo.latestVersion}`,
				);

				// Import agentWs service to push notification
				const { pushUpdateNotification } = require("./agentWs");

				const updateInfo = {
					version: versionInfo.latestVersion,
					force: force,
					downloadUrl: `/api/v1/agent/binary/${versionInfo.latestVersion}/linux-amd64`,
					message: force
						? "Force update requested"
						: `Update available: ${versionInfo.latestVersion}`,
				};

				const pushed = pushUpdateNotification(agentApiId, updateInfo);

				if (pushed) {
					console.log(`✅ Update notification pushed to agent ${agentApiId}`);
					return {
						needsUpdate: true,
						reason: force ? "force-update" : "version-outdated",
						message: `Update notification sent: ${agentVersion} → ${versionInfo.latestVersion}`,
						targetVersion: versionInfo.latestVersion,
					};
				} else {
					console.log(
						`⚠️ Failed to push update notification to agent ${agentApiId} (not connected)`,
					);
					return {
						needsUpdate: true,
						reason: "agent-offline",
						message: "Agent needs update but is not connected",
						targetVersion: versionInfo.latestVersion,
					};
				}
			} else {
				console.log(`✅ Agent ${agentApiId} is up to date: ${agentVersion}`);
				return {
					needsUpdate: false,
					reason: "up-to-date",
					message: `Agent is up to date: ${agentVersion}`,
				};
			}
		} catch (error) {
			console.error(
				`❌ Failed to check update for agent ${agentApiId}:`,
				error.message,
			);
			return {
				needsUpdate: false,
				reason: "error",
				message: `Error checking update: ${error.message}`,
			};
		}
	}

	/**
	 * Check and push updates to all connected agents
	 * @param {boolean} force - Force update regardless of version
	 * @returns {Object} Bulk update result
	 */
	async checkAndPushUpdatesToAll(force = false) {
		try {
			console.log(
				`🔍 Checking updates for all connected agents (force: ${force})`,
			);

			// Check general server auto_update setting
			const { getPrismaClient } = require("../config/prisma");
			const prisma = getPrismaClient();
			const settings = await prisma.settings.findFirst();
			if (!settings || !settings.auto_update) {
				console.log(
					`⚠️ Auto-update is disabled in server settings, skipping bulk update check`,
				);
				return {
					success: false,
					message: "Auto-update is disabled in server settings",
					updatedAgents: 0,
					totalAgents: 0,
				};
			}

			// Import agentWs service to get connected agents
			const { pushUpdateNotificationToAll } = require("./agentWs");

			const versionInfo = await this.getVersionInfo();

			if (!versionInfo.latestVersion) {
				return {
					success: false,
					message: "No latest version available on server",
					updatedAgents: 0,
					totalAgents: 0,
				};
			}

			const updateInfo = {
				version: versionInfo.latestVersion,
				force: force,
				downloadUrl: `/api/v1/agent/binary/${versionInfo.latestVersion}/linux-amd64`,
				message: force
					? "Force update requested for all agents"
					: `Update available: ${versionInfo.latestVersion}`,
			};

			const result = await pushUpdateNotificationToAll(updateInfo);

			console.log(
				`✅ Bulk update notification sent to ${result.notifiedCount} agents`,
			);

			return {
				success: true,
				message: `Update notifications sent to ${result.notifiedCount} agents`,
				updatedAgents: result.notifiedCount,
				totalAgents: result.totalAgents,
				targetVersion: versionInfo.latestVersion,
			};
		} catch (error) {
			console.error("❌ Failed to push updates to all agents:", error.message);
			return {
				success: false,
				message: `Error pushing updates: ${error.message}`,
				updatedAgents: 0,
				totalAgents: 0,
			};
		}
	}
}

module.exports = new AgentVersionService();
