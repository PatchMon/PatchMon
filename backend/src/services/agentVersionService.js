const axios = require("axios");
const logger = require("../utils/logger");
const fs = require("node:fs").promises;
const path = require("node:path");
const os = require("node:os");
const { exec, spawn } = require("node:child_process");
const { promisify } = require("node:util");
const _execAsync = promisify(exec);
const dns = require("node:dns").promises;

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
			"https://api.github.com/repos/PatchMon/PatchMon/releases";
		this.dnsDomain = "agent.vcheck.patchmon.net";
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

			logger.info("🔍 Testing DNS connectivity for agent version...");
			try {
				const testVersion = await this.checkVersionFromDNS(this.dnsDomain);
				logger.info(
					`✅ DNS lookup successful - latest agent version: ${testVersion}`,
				);
			} catch (testError) {
				logger.error("❌ DNS lookup failed:", testError.message);
			}

			// Get current agent version by executing the binary
			await this.getCurrentAgentVersion();

			// Try to check for updates, but don't fail initialization if DNS is unavailable
			try {
				await this.checkForUpdates();
			} catch (updateError) {
				logger.info(
					"⚠️ Failed to check for updates on startup, will retry later:",
					updateError.message,
				);
			}

			// Set up periodic checking
			setInterval(() => {
				this.checkForUpdates().catch((error) => {
					logger.info("⚠️ Periodic update check failed:", error.message);
				});
			}, this.checkInterval);

			logger.info("✅ Agent Version Service initialized");
		} catch (error) {
			logger.error(
				"❌ Failed to initialize Agent Version Service:",
				error.message,
			);
		}
	}

	async getCurrentAgentVersion() {
		try {
			logger.info("🔍 Getting current agent version...");

			// Server runs on Linux: only consider the Linux agent binary for "current version"
			// (Settings/UI, refresh, download flow). FreeBSD binaries are for per-host update
			// checks only in hostRoutes.js /agent/version.
			const serverArch = os.arch();
			const archMap = {
				x64: "amd64",
				ia32: "386",
				arm64: "arm64",
				arm: "arm",
			};
			const serverGoArch = archMap[serverArch] || serverArch;

			logger.info(
				`🔍 Detected server architecture: ${serverArch} -> ${serverGoArch}`,
			);

			// Linux binary only (server arch); do not consider FreeBSD or other OS binaries
			const possiblePaths = [
				path.join(this.agentsDir, `patchmon-agent-linux-${serverGoArch}`),
				path.join(this.agentsDir, "patchmon-agent-linux-amd64"), // Fallback
				path.join(this.agentsDir, "patchmon-agent"), // Legacy fallback
			];

			let agentPath = null;
			for (const testPath of possiblePaths) {
				try {
					await fs.access(testPath);
					agentPath = testPath;
					logger.info(`✅ Found agent binary at: ${testPath}`);
					break;
				} catch {
					// Path doesn't exist, continue to next
				}
			}

			if (!agentPath) {
				logger.info(
					`⚠️ No agent binary found in agents/ folder for architecture ${serverGoArch}, current version will be unknown`,
				);
				logger.info("💡 Use the Download Updates button to get agent binaries");
				this.currentVersion = null;
				return;
			}

			// Execute the agent binary to get version info
			// Try --version first, then --help as fallback
			const versionCommands = ["--version", "version", "--help"];
			let versionFound = false;

			for (const cmd of versionCommands) {
				try {
					logger.info(`🔍 Trying to get version with command: ${cmd}`);
					const child = spawn(agentPath, [cmd], {
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

					const _result = await new Promise((resolve, reject) => {
						child.on("close", (code) => {
							resolve({ stdout, stderr, code });
						});
						child.on("error", reject);
					});

					// Combine stdout and stderr for version parsing
					const output = stdout + stderr;

					if (output) {
						logger.info(`📄 Agent output (${cmd}):`, output.substring(0, 500));
					}

					// Try multiple version patterns
					const versionPatterns = [
						/PatchMon Agent v([0-9]+\.[0-9]+\.[0-9]+)/i,
						/patchmon-agent v([0-9]+\.[0-9]+\.[0-9]+)/i,
						/version ([0-9]+\.[0-9]+\.[0-9]+)/i,
						/v([0-9]+\.[0-9]+\.[0-9]+)/i,
						/([0-9]+\.[0-9]+\.[0-9]+)/,
					];

					for (const pattern of versionPatterns) {
						const versionMatch = output.match(pattern);
						if (versionMatch) {
							this.currentVersion = versionMatch[1];
							logger.info(
								`✅ Current agent version: ${this.currentVersion} (found with ${cmd})`,
							);
							versionFound = true;
							break;
						}
					}

					if (versionFound) {
						break;
					}
				} catch (execError) {
					logger.warn(
						`⚠️ Failed to execute agent binary with ${cmd}:`,
						execError.message,
					);
					// Continue to next command
				}
			}

			if (!versionFound) {
				logger.warn("⚠️ Could not determine agent version from binary output");
				logger.info(
					"💡 The binary was downloaded but version detection failed",
				);
				// Don't set to null - keep previous version if available
				// this.currentVersion = null;
			}
		} catch (error) {
			logger.error("❌ Failed to get current agent version:", error.message);
			this.currentVersion = null;
		}
	}

	async checkVersionFromDNS(domain) {
		try {
			const records = await dns.resolveTxt(domain);
			if (!records || records.length === 0) {
				throw new Error(`No TXT records found for ${domain}`);
			}
			// TXT records are arrays of strings, get first record's first string
			const version = records[0][0].trim().replace(/^["']|["']$/g, "");
			// Validate version format (semantic versioning)
			if (!/^\d+\.\d+\.\d+/.test(version)) {
				throw new Error(`Invalid version format: ${version}`);
			}
			return version;
		} catch (error) {
			logger.error(`DNS lookup failed for ${domain}:`, error.message);
			throw error;
		}
	}

	async checkForUpdates() {
		try {
			logger.info("🔍 Checking for agent updates via DNS...");

			this.latestVersion = await this.checkVersionFromDNS(this.dnsDomain);
			this.lastChecked = new Date();

			logger.info(`📦 Latest agent version: ${this.latestVersion}`);

			// Don't download binaries automatically - only when explicitly requested
			logger.info(
				"ℹ️ Skipping automatic binary download - binaries will be downloaded on demand",
			);

			return {
				latestVersion: this.latestVersion,
				currentVersion: this.currentVersion,
				hasUpdate: this.currentVersion !== this.latestVersion,
				lastChecked: this.lastChecked,
			};
		} catch (error) {
			logger.error("❌ Failed to check for updates:", error.message);
			throw error;
		}
	}

	async downloadBinariesToAgentsFolder(release) {
		try {
			logger.info(
				`⬇️ Downloading binaries for version ${release.tag_name} to agents folder...`,
			);
			logger.info(`📁 Agents directory: ${this.agentsDir}`);

			// Ensure agents directory exists
			await fs.mkdir(this.agentsDir, { recursive: true });
			logger.info(`✅ Agents directory exists: ${this.agentsDir}`);

			const downloadedFiles = [];
			const failedDownloads = [];

			for (const arch of this.supportedArchitectures) {
				// arch already includes "linux-", so just use it directly
				const assetName = `patchmon-agent-${arch}`;
				logger.info(`🔍 Looking for asset: ${assetName}`);

				// List all available asset names for debugging
				if (release.assets && release.assets.length > 0) {
					logger.info(
						`📋 Available assets in release:`,
						release.assets.map((a) => a.name).join(", "),
					);
				} else {
					logger.warn(`⚠️ Release has no assets!`);
				}

				const asset = release.assets?.find((a) => a.name === assetName);

				if (!asset) {
					logger.warn(`⚠️ Binary not found for architecture: ${arch}`);
					logger.warn(`⚠️ Expected: ${assetName}`);
					logger.warn(
						`⚠️ Available assets:`,
						release.assets?.map((a) => a.name).join(", ") || "none",
					);
					failedDownloads.push({
						arch,
						reason: `Asset "${assetName}" not found in release`,
					});
					continue;
				}

				logger.info(`✅ Found asset: ${asset.name} (${asset.size} bytes)`);

				const binaryPath = path.join(this.agentsDir, assetName);
				logger.info(
					`⬇️ Downloading ${assetName} from ${asset.browser_download_url}...`,
				);
				logger.info(`📁 Target path: ${binaryPath}`);

				try {
					logger.info(`🌐 Fetching ${assetName} from GitHub...`);
					const response = await axios.get(asset.browser_download_url, {
						responseType: "stream",
						timeout: 120000, // Increased timeout for large files
						maxContentLength: Infinity,
						maxBodyLength: Infinity,
					});

					logger.info(`📝 Creating write stream for ${binaryPath}...`);
					const writer = require("node:fs").createWriteStream(binaryPath);

					// Track download progress
					let downloadedBytes = 0;

					// Set up error handlers before piping
					response.data.on("error", (err) => {
						logger.error(
							`❌ Download stream error for ${assetName}:`,
							err.message,
						);
						writer.destroy();
					});

					response.data.on("data", (chunk) => {
						downloadedBytes += chunk.length;
					});

					// Pipe the stream
					response.data.pipe(writer);

					// Wait for the stream to finish
					await new Promise((resolve, reject) => {
						let resolved = false;

						writer.on("finish", () => {
							if (!resolved) {
								resolved = true;
								logger.info(
									`📥 Downloaded ${downloadedBytes} bytes for ${assetName}`,
								);
								resolve();
							}
						});

						writer.on("close", () => {
							if (!resolved) {
								resolved = true;
								logger.info(
									`📥 Stream closed, downloaded ${downloadedBytes} bytes for ${assetName}`,
								);
								resolve();
							}
						});

						writer.on("error", (err) => {
							if (!resolved) {
								resolved = true;
								logger.error(`❌ Write error for ${assetName}:`, err.message);
								reject(err);
							}
						});

						response.data.on("error", (err) => {
							if (!resolved) {
								resolved = true;
								logger.error(
									`❌ Download error for ${assetName}:`,
									err.message,
								);
								writer.destroy();
								reject(err);
							}
						});
					});

					// Verify file was written
					try {
						const stats = await fs.stat(binaryPath);
						logger.info(`✅ File verified: ${assetName} (${stats.size} bytes)`);

						if (stats.size === 0) {
							throw new Error(`Downloaded file is empty: ${assetName}`);
						}
					} catch (statError) {
						logger.error(
							`❌ File verification failed for ${assetName}:`,
							statError.message,
						);
						throw new Error(`File verification failed: ${statError.message}`);
					}

					// Make executable
					await fs.chmod(binaryPath, "755");

					// Verify executable permission
					const statsAfter = await fs.stat(binaryPath);
					const isExecutable = (statsAfter.mode & 0o111) !== 0;
					if (!isExecutable) {
						logger.warn(`⚠️ File may not be executable: ${assetName}`);
					}

					downloadedFiles.push({
						arch,
						path: binaryPath,
						size: statsAfter.size,
					});
					logger.info(`✅ Successfully downloaded and verified: ${assetName}`);
				} catch (downloadError) {
					logger.error(
						`❌ Failed to download ${assetName}:`,
						downloadError.message,
					);
					logger.error(`❌ Error stack for ${assetName}:`, downloadError.stack);
					failedDownloads.push({
						arch,
						reason: downloadError.message,
						stack: downloadError.stack,
					});
					// Continue with other architectures
				}
			}

			// Summary
			logger.info(
				`📊 Download summary: ${downloadedFiles.length} successful, ${failedDownloads.length} failed`,
			);

			if (downloadedFiles.length === 0) {
				const errorDetails = failedDownloads
					.map((f) => `${f.arch}: ${f.reason}`)
					.join("; ");
				logger.error(`❌ All downloads failed. Details: ${errorDetails}`);
				throw new Error(
					`No binaries were successfully downloaded. Failures: ${errorDetails}`,
				);
			}

			if (failedDownloads.length > 0) {
				logger.warn(
					`⚠️ Some downloads failed:`,
					JSON.stringify(failedDownloads, null, 2),
				);
			}

			return {
				success: true,
				downloaded: downloadedFiles,
				failed: failedDownloads,
			};
		} catch (error) {
			logger.error(
				"❌ Failed to download binaries to agents folder:",
				error.message,
			);
			logger.error("❌ Error stack:", error.stack);
			throw error;
		}
	}

	async downloadBinaryForVersion(version, architecture) {
		try {
			logger.info(
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

			const assetName = `patchmon-agent-linux-${architecture}`;
			const asset = release.assets.find((a) => a.name === assetName);

			if (!asset) {
				throw new Error(`Binary not found for architecture: ${architecture}`);
			}

			const binaryPath = path.join(
				this.agentBinariesDir,
				`${release.tag_name}-${assetName}`,
			);

			logger.info(`⬇️ Downloading ${assetName}...`);

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

			logger.info(`✅ Downloaded: ${assetName}`);
			return binaryPath;
		} catch (error) {
			logger.error(
				`❌ Failed to download binary ${version}-${architecture}:`,
				error.message,
			);
			throw error;
		}
	}

	async getBinaryPath(version, architecture) {
		const binaryName = `patchmon-agent-linux-${architecture}`;
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
				`attachment; filename="patchmon-agent-linux-${architecture}"`,
			);
			res.setHeader("Content-Length", stats.size);

			// Add cache headers
			res.setHeader("Cache-Control", "public, max-age=3600");
			res.setHeader("ETag", `"${version}-${architecture}"`);

			const stream = require("node:fs").createReadStream(binaryPath);
			stream.pipe(res);
		} catch (_error) {
			// Binary doesn't exist, try to download it
			logger.info(
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
					`attachment; filename="patchmon-agent-linux-${architecture}"`,
				);
				res.setHeader("Content-Length", stats.size);
				res.setHeader("Cache-Control", "public, max-age=3600");
				res.setHeader("ETag", `"${version}-${architecture}"`);

				const stream = require("node:fs").createReadStream(binaryPath);
				stream.pipe(res);
			} catch (downloadError) {
				logger.error(
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

		// Latest version comes from DNS TXT record
		// currentVersion = what's installed locally
		// latestVersion = what's available from DNS
		if (this.latestVersion) {
			logger.info(`📦 Latest version from DNS: ${this.latestVersion}`);
		} else {
			logger.info(`⚠️ No latest version available (DNS lookup may have failed)`);
		}

		if (this.currentVersion) {
			logger.info(`💾 Current local agent version: ${this.currentVersion}`);
		} else {
			logger.info(`⚠️ No local agent binary found`);
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
			// We have a current version but no latest version (DNS unavailable)
			hasUpdate = false;
			updateStatus = "dns-unavailable";
		} else if (!this.currentVersion && !this.latestVersion) {
			updateStatus = "no-data";
		}

		return {
			currentVersion: this.currentVersion,
			latestVersion: this.latestVersion, // Always return DNS version, not local
			hasUpdate: hasUpdate,
			updateStatus: updateStatus,
			lastChecked: this.lastChecked,
			supportedArchitectures: this.supportedArchitectures,
			status: this.latestVersion ? "ready" : "no-version",
		};
	}

	async refreshCurrentVersion() {
		await this.getCurrentAgentVersion();
		return this.currentVersion;
	}

	async downloadLatestUpdate() {
		try {
			logger.info("⬇️ Downloading latest agent update...");

			// First check for updates to get the latest release info
			const _updateInfo = await this.checkForUpdates();

			if (!this.latestVersion) {
				throw new Error("No latest version available to download");
			}

			// Download the specific version from DNS
			return await this.downloadVersion(this.latestVersion);
		} catch (error) {
			logger.error("❌ Failed to download latest update:", error.message);
			throw error;
		}
	}

	async downloadVersion(version) {
		try {
			logger.info(`⬇️ Downloading agent version ${version}...`);
			logger.info(`🌐 GitHub API URL: ${this.githubApiUrl}`);

			// Get the release info from GitHub
			const response = await axios.get(this.githubApiUrl, {
				timeout: 10000,
				headers: {
					"User-Agent": "PatchMon-Server/1.0",
					Accept: "application/vnd.github.v3+json",
				},
			});

			const releases = response.data;
			logger.info(`📦 Found ${releases.length} releases on GitHub`);

			// Log first few releases for debugging
			if (releases.length > 0) {
				logger.info(
					`📋 First 3 releases:`,
					releases.slice(0, 3).map((r) => ({
						tag_name: r.tag_name,
						name: r.name,
						assets_count: r.assets?.length || 0,
						assets: r.assets?.map((a) => a.name) || [],
					})),
				);
			}

			// Normalize version (remove 'v' prefix for comparison)
			const normalizedVersion = version.replace(/^v/, "").trim();
			logger.info(
				`🔍 Looking for version: ${normalizedVersion} (original: ${version})`,
			);

			// Find the release matching the requested version
			// Try multiple matching strategies
			let release = releases.find(
				(r) => r.tag_name.replace(/^v/, "").trim() === normalizedVersion,
			);

			// If not found, try with 'v' prefix
			if (!release) {
				release = releases.find(
					(r) =>
						r.tag_name === `v${normalizedVersion}` ||
						r.tag_name === normalizedVersion,
				);
			}

			// If still not found, try case-insensitive
			if (!release) {
				release = releases.find(
					(r) =>
						r.tag_name.replace(/^v/, "").trim().toLowerCase() ===
						normalizedVersion.toLowerCase(),
				);
			}

			if (!release) {
				const availableTags = releases
					.slice(0, 10)
					.map((r) => r.tag_name)
					.join(", ");
				logger.error(
					`❌ Release ${version} not found. Available tags (first 10): ${availableTags}`,
				);
				throw new Error(
					`Release ${version} not found on GitHub. Available releases: ${availableTags}`,
				);
			}

			logger.info(`✅ Found release: ${release.tag_name}`);
			logger.info(`📦 Release has ${release.assets?.length || 0} assets`);

			// If release doesn't have assets, fetch the individual release
			if (!release.assets || release.assets.length === 0) {
				logger.info(
					`⚠️ Release object doesn't have assets, fetching individual release...`,
				);
				const individualReleaseResponse = await axios.get(
					`https://api.github.com/repos/PatchMon/PatchMon/releases/tags/${release.tag_name}`,
					{
						timeout: 10000,
						headers: {
							"User-Agent": "PatchMon-Server/1.0",
							Accept: "application/vnd.github.v3+json",
						},
					},
				);
				release = individualReleaseResponse.data;
				logger.info(
					`✅ Fetched individual release, now has ${release.assets?.length || 0} assets`,
				);
			}

			if (release.assets && release.assets.length > 0) {
				logger.info(
					`📋 Asset names:`,
					release.assets.map((a) => a.name).join(", "),
				);
			} else {
				throw new Error(`Release ${release.tag_name} has no assets`);
			}

			logger.info(`⬇️ Downloading binaries for version ${release.tag_name}...`);

			// Download binaries for all architectures directly to agents folder
			const downloadResult = await this.downloadBinariesToAgentsFolder(release);

			if (!downloadResult.success || downloadResult.downloaded.length === 0) {
				throw new Error(
					`Failed to download binaries: ${downloadResult.failed?.map((f) => `${f.arch}: ${f.reason}`).join(", ") || "Unknown error"}`,
				);
			}

			logger.info(
				`✅ Downloaded ${downloadResult.downloaded.length} binaries successfully`,
			);

			// Set the current version to the downloaded version
			const downloadedVersion = version.replace(/^v/, "");
			this.currentVersion = downloadedVersion;
			logger.info(`✅ Set current version to: ${downloadedVersion}`);

			// Try to verify by executing the binary (but don't fail if it doesn't work)
			try {
				await this.getCurrentAgentVersion();
				// If version detection worked, use that; otherwise keep the downloaded version
				if (this.currentVersion && this.currentVersion !== downloadedVersion) {
					logger.info(
						`⚠️ Version mismatch: downloaded ${downloadedVersion}, detected ${this.currentVersion}. Using detected version.`,
					);
				} else if (!this.currentVersion) {
					// If detection failed, keep the downloaded version
					this.currentVersion = downloadedVersion;
					logger.info(
						`⚠️ Version detection failed, using downloaded version: ${downloadedVersion}`,
					);
				}
			} catch (verifyError) {
				logger.warn(
					`⚠️ Could not verify downloaded version, using downloaded version: ${downloadedVersion}`,
					verifyError.message,
				);
				// Keep the downloaded version
				this.currentVersion = downloadedVersion;
			}

			logger.info(`✅ Version ${version} downloaded successfully`);

			return {
				success: true,
				version: downloadedVersion,
				currentVersion: this.currentVersion,
				downloadedArchitectures: this.supportedArchitectures,
				message: `Successfully downloaded version ${downloadedVersion}`,
			};
		} catch (error) {
			logger.error(`❌ Failed to download version ${version}:`, error.message);
			throw error;
		}
	}

	async getAvailableVersions() {
		try {
			logger.info(`🌐 Fetching releases from GitHub: ${this.githubApiUrl}`);
			// Fetch all releases from GitHub
			const response = await axios.get(this.githubApiUrl, {
				timeout: 10000,
				headers: {
					"User-Agent": "PatchMon-Server/1.0",
					Accept: "application/vnd.github.v3+json",
				},
			});

			const releases = response.data || [];
			logger.info(`📦 Found ${releases.length} releases from GitHub`);

			if (releases.length === 0) {
				logger.warn("⚠️ No releases found in GitHub response");
				throw new Error("No releases found in GitHub response");
			}

			// Extract version numbers from tag names (remove 'v' prefix if present)
			// Filter out drafts and only include published releases
			const versions = releases
				.filter((release) => !release.draft) // Exclude draft releases
				.map((release) => ({
					version: release.tag_name.replace(/^v/, ""),
					tag_name: release.tag_name,
					published_at: release.published_at,
					prerelease: release.prerelease,
					draft: release.draft,
					html_url: release.html_url,
				}));

			// Sort by version (newest first)
			versions.sort((a, b) => {
				return compareVersions(b.version, a.version);
			});

			logger.info(
				`✅ Returning ${versions.length} available versions from GitHub`,
			);
			return versions;
		} catch (error) {
			logger.error(
				"❌ Failed to get available versions from GitHub:",
				error.message,
			);
			logger.error("❌ Error stack:", error.stack);
			// Fallback to DNS version if GitHub fails - but log this clearly
			if (this.latestVersion) {
				logger.warn(`⚠️ Falling back to DNS version: ${this.latestVersion}`);
				return [
					{
						version: this.latestVersion,
						tag_name: `v${this.latestVersion}`,
						published_at: null,
						prerelease: false,
						draft: false,
						html_url: `https://github.com/PatchMon/PatchMon/releases/tag/v${this.latestVersion}`,
					},
				];
			}
			logger.warn("⚠️ No fallback version available");
			return [];
		}
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
			logger.info(
				`🔍 Checking update for agent ${agentApiId} (version: ${agentVersion})`,
			);

			// Check general server auto_update setting
			const { getPrismaClient } = require("../config/prisma");
			const prisma = getPrismaClient();
			const settings = await prisma.settings.findFirst();
			if (!settings || !settings.auto_update) {
				logger.info(
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
				logger.info(
					`⚠️ Host not found for agent ${agentApiId}, skipping update check`,
				);
				return {
					needsUpdate: false,
					reason: "host-not-found",
					message: "Host not found",
				};
			}

			if (!host.auto_update) {
				logger.info(
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
				logger.info(`⚠️ No latest version available for agent ${agentApiId}`);
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
				logger.info(
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
					logger.info(`✅ Update notification pushed to agent ${agentApiId}`);
					return {
						needsUpdate: true,
						reason: force ? "force-update" : "version-outdated",
						message: `Update notification sent: ${agentVersion} → ${versionInfo.latestVersion}`,
						targetVersion: versionInfo.latestVersion,
					};
				} else {
					logger.info(
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
				logger.info(`✅ Agent ${agentApiId} is up to date: ${agentVersion}`);
				return {
					needsUpdate: false,
					reason: "up-to-date",
					message: `Agent is up to date: ${agentVersion}`,
				};
			}
		} catch (error) {
			logger.error(
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
			logger.info(
				`🔍 Checking updates for all connected agents (force: ${force})`,
			);

			// Check general server auto_update setting
			const { getPrismaClient } = require("../config/prisma");
			const prisma = getPrismaClient();
			const settings = await prisma.settings.findFirst();
			if (!settings || !settings.auto_update) {
				logger.info(
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

			logger.info(
				`✅ Bulk update notification sent to ${result.notifiedCount} agents`,
			);

			// Create or update alert for agent update availability
			try {
				const alertService = require("./alertService");
				const alertConfigService = require("./alertConfigService");
				const { getPrismaClient } = require("../config/prisma");
				const prismaClient = getPrismaClient();

				// Check if alerts system is enabled
				const alertsEnabled = await alertService.isAlertsEnabled();
				if (!alertsEnabled) {
					logger.info(
						"⚠️ Alerts system is disabled, skipping agent update alert",
					);
					return {
						success: true,
						message: `Update notifications sent to ${result.notifiedCount} agents`,
						updatedAgents: result.notifiedCount,
						totalAgents: result.totalAgents,
						targetVersion: versionInfo.latestVersion,
					};
				}

				const alertType = "agent_update";
				const isEnabled =
					await alertConfigService.isAlertTypeEnabled(alertType);

				if (isEnabled && result.notifiedCount > 0) {
					const defaultSeverity =
						await alertConfigService.getDefaultSeverity(alertType);

					// Check if alert already exists for this version
					const existingAlert = await prismaClient.alerts.findFirst({
						where: {
							type: alertType,
							is_active: true,
							metadata: {
								path: ["latest_version"],
								equals: versionInfo.latestVersion,
							},
						},
					});

					if (!existingAlert) {
						// Create new alert
						const alert = await alertService.createAlert(
							alertType,
							defaultSeverity,
							"Agent Update Available",
							`A new agent version (${versionInfo.latestVersion}) is available. ${result.notifiedCount} agent(s) need updates.`,
							{
								latest_version: versionInfo.latestVersion,
								agents_needing_update: result.notifiedCount,
								total_agents: result.totalAgents,
							},
						);

						// Check auto-assignment
						if (
							await alertConfigService.shouldAutoAssign(alertType, {
								severity: defaultSeverity,
							})
						) {
							const assignUserId =
								await alertConfigService.getAutoAssignUser(alertType);
							if (assignUserId) {
								await alertService.assignAlertToUser(
									alert.id,
									assignUserId,
									null,
								);
							}
						}

						logger.info(`✅ Created agent update alert: ${alert.id}`);
					} else {
						// Update existing alert with new count
						await prismaClient.alerts.update({
							where: { id: existingAlert.id },
							data: {
								message: `A new agent version (${versionInfo.latestVersion}) is available. ${result.notifiedCount} agent(s) need updates.`,
								metadata: {
									...existingAlert.metadata,
									agents_needing_update: result.notifiedCount,
									total_agents: result.totalAgents,
								},
								updated_at: new Date(),
							},
						});
					}
				} else if (result.notifiedCount === 0 && result.totalAgents > 0) {
					// All agents are up to date - resolve existing alerts
					const existingAlert = await prismaClient.alerts.findFirst({
						where: {
							type: alertType,
							is_active: true,
						},
					});

					if (existingAlert) {
						await alertService.performAlertAction(
							null,
							existingAlert.id,
							"resolved",
							{
								reason: "all_agents_up_to_date",
							},
						);
						logger.info(`✅ Resolved agent update alert: ${existingAlert.id}`);
					}
				}
			} catch (alertError) {
				// Don't fail the update check if alert creation fails
				logger.error("Failed to create/update agent update alert:", alertError);
			}

			return {
				success: true,
				message: `Update notifications sent to ${result.notifiedCount} agents`,
				updatedAgents: result.notifiedCount,
				totalAgents: result.totalAgents,
				targetVersion: versionInfo.latestVersion,
			};
		} catch (error) {
			logger.error("❌ Failed to push updates to all agents:", error.message);
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
