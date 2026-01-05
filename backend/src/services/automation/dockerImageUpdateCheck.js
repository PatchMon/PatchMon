const { prisma } = require("./shared/prisma");
const https = require("node:https");
const { v4: uuidv4 } = require("uuid");

/**
 * Docker Image Update Check Automation
 * Checks for Docker image updates by comparing local digests with remote registry digests
 */
class DockerImageUpdateCheck {
	constructor(queueManager) {
		this.queueManager = queueManager;
		this.queueName = "docker-image-update-check";
		// Cache tokens to avoid requesting new ones for each image
		this.tokenCache = new Map();
		// SECURITY: Allowlist of trusted container registries to prevent SSRF attacks
		// Only these registries will be contacted for update checks
		this.trustedRegistries = new Set([
			"registry-1.docker.io", // Docker Hub
			"docker.io",
			"ghcr.io",             // GitHub Container Registry
			"gcr.io",              // Google Container Registry
			"quay.io",             // Red Hat Quay
			"mcr.microsoft.com",   // Microsoft Container Registry
			"public.ecr.aws",      // AWS Public ECR
			"registry.k8s.io",     // Kubernetes registry
			"docker.elastic.co",   // Elastic
			"nvcr.io",             // NVIDIA
		]);
	}

	/**
	 * Check if a registry hostname is trusted
	 * @param {string} registry - The registry hostname to check
	 * @returns {boolean} True if the registry is trusted
	 */
	isRegistryTrusted(registry) {
		// Normalize registry name
		const normalizedRegistry = registry.toLowerCase().trim();

		// Check exact match
		if (this.trustedRegistries.has(normalizedRegistry)) {
			return true;
		}

		// Check if it's a subdomain of a trusted registry (e.g., us.gcr.io, europe-west1-docker.pkg.dev)
		for (const trusted of this.trustedRegistries) {
			if (normalizedRegistry.endsWith(`.${trusted}`)) {
				return true;
			}
		}

		// Allow Google Artifact Registry regions
		if (/^[a-z0-9-]+-docker\.pkg\.dev$/.test(normalizedRegistry)) {
			return true;
		}

		// Allow AWS ECR private registries (account-id.dkr.ecr.region.amazonaws.com)
		if (/^\d{12}\.dkr\.ecr\.[a-z0-9-]+\.amazonaws\.com$/.test(normalizedRegistry)) {
			return true;
		}

		return false;
	}

	/**
	 * Make an HTTPS request and return a promise
	 */
	httpsRequest(options) {
		return new Promise((resolve, reject) => {
			const client = https;

			const req = client.request(options, (res) => {
				let data = "";
				res.on("data", (chunk) => {
					data += chunk;
				});
				res.on("end", () => {
					resolve({
						statusCode: res.statusCode,
						headers: res.headers,
						body: data,
					});
				});
			});

			req.on("error", reject);
			req.setTimeout(15000, () => {
				req.destroy();
				reject(new Error("Request timeout"));
			});

			req.end();
		});
	}

	/**
	 * Parse WWW-Authenticate header to extract token endpoint details
	 * Format: Bearer realm="https://auth.docker.io/token",service="registry.docker.io",scope="repository:library/nginx:pull"
	 */
	parseWwwAuthenticate(header) {
		if (!header || !header.startsWith("Bearer ")) {
			return null;
		}

		const params = {};
		const regex = /(\w+)="([^"]+)"/g;
		let match;
		while (true) {
			match = regex.exec(header);
			if (match === null) break;
			params[match[1]] = match[2];
		}

		return params;
	}

	/**
	 * Get authentication token for a registry
	 * Supports Docker Hub, GHCR, and other OCI-compliant registries
	 */
	async getAuthToken(registry, repository, wwwAuthHeader) {
		const cacheKey = `${registry}/${repository}`;

		// Check cache first (tokens are typically valid for 5+ minutes)
		const cached = this.tokenCache.get(cacheKey);
		if (cached && cached.expiresAt > Date.now()) {
			return cached.token;
		}

		const authParams = this.parseWwwAuthenticate(wwwAuthHeader);
		if (!authParams || !authParams.realm) {
			throw new Error(`Cannot parse WWW-Authenticate header: ${wwwAuthHeader}`);
		}

		// Build token request URL
		const tokenUrl = new URL(authParams.realm);
		if (authParams.service) {
			tokenUrl.searchParams.set("service", authParams.service);
		}
		if (authParams.scope) {
			tokenUrl.searchParams.set("scope", authParams.scope);
		}

		const options = {
			hostname: tokenUrl.hostname,
			port: tokenUrl.port || 443,
			path: tokenUrl.pathname + tokenUrl.search,
			method: "GET",
			headers: {
				"User-Agent": "PatchMon/1.0",
			},
		};

		const response = await this.httpsRequest(options);

		if (response.statusCode !== 200) {
			throw new Error(
				`Token request failed with status ${response.statusCode}`,
			);
		}

		const tokenData = JSON.parse(response.body);
		const token = tokenData.token || tokenData.access_token;

		if (!token) {
			throw new Error("No token in authentication response");
		}

		// Cache the token (default 5 minute expiry if not specified)
		const expiresIn = tokenData.expires_in || 300;
		this.tokenCache.set(cacheKey, {
			token,
			expiresAt: Date.now() + expiresIn * 1000 - 30000, // 30 second buffer
		});

		return token;
	}

	/**
	 * Get remote digest from Docker registry using HEAD request
	 * Supports Docker Hub, GHCR, and other OCI-compliant registries
	 * Handles authentication automatically via OAuth2 bearer tokens
	 */
	async getRemoteDigest(imageName, tag = "latest") {
		const registryInfo = this.parseImageName(imageName);

		// SECURITY: Validate registry is trusted to prevent SSRF attacks
		if (!this.isRegistryTrusted(registryInfo.registry)) {
			throw new Error(
				`Untrusted registry: ${registryInfo.registry}. Only images from trusted registries can be checked for updates.`
			);
		}

		const manifestPath = `/v2/${registryInfo.repository}/manifests/${tag}`;

		const options = {
			hostname: registryInfo.registry,
			port: 443,
			path: manifestPath,
			method: "HEAD",
			headers: {
				Accept: [
					"application/vnd.docker.distribution.manifest.v2+json",
					"application/vnd.docker.distribution.manifest.list.v2+json",
					"application/vnd.oci.image.manifest.v1+json",
					"application/vnd.oci.image.index.v1+json",
				].join(", "),
				"User-Agent": "PatchMon/1.0",
			},
		};

		// First attempt without auth
		let response = await this.httpsRequest(options);

		// If we get 401, get a token and retry
		if (response.statusCode === 401) {
			const wwwAuth = response.headers["www-authenticate"];
			if (!wwwAuth) {
				throw new Error(
					`401 received but no WWW-Authenticate header for ${imageName}:${tag}`,
				);
			}

			const token = await this.getAuthToken(
				registryInfo.registry,
				registryInfo.repository,
				wwwAuth,
			);

			// Retry with token
			options.headers.Authorization = `Bearer ${token}`;
			response = await this.httpsRequest(options);
		}

		if (response.statusCode === 401 || response.statusCode === 403) {
			throw new Error(
				`Authentication failed for ${imageName}:${tag} (status ${response.statusCode})`,
			);
		}

		if (response.statusCode !== 200) {
			throw new Error(
				`Registry returned status ${response.statusCode} for ${imageName}:${tag}`,
			);
		}

		// Get digest from Docker-Content-Digest header
		const digest = response.headers["docker-content-digest"];
		if (!digest) {
			throw new Error(
				`No Docker-Content-Digest header for ${imageName}:${tag}`,
			);
		}

		// Clean up digest (remove sha256: prefix if present)
		return digest.startsWith("sha256:") ? digest.substring(7) : digest;
	}

	/**
	 * Parse image name to extract registry and repository
	 */
	parseImageName(imageName) {
		// Remove docker.io/ prefix if present (normalize)
		if (imageName.startsWith("docker.io/")) {
			imageName = imageName.substring(10);
		}

		let registry = "registry-1.docker.io";
		let repository = imageName;

		// Handle explicit registries (ghcr.io, quay.io, etc.)
		if (imageName.includes("/")) {
			const parts = imageName.split("/");
			const firstPart = parts[0];

			// Check if first part looks like a registry (contains . or : or is localhost)
			if (
				firstPart.includes(".") ||
				firstPart.includes(":") ||
				firstPart === "localhost"
			) {
				registry = firstPart;
				repository = parts.slice(1).join("/");
			}
		}

		// Docker Hub official images need library/ prefix
		if (registry === "registry-1.docker.io" && !repository.includes("/")) {
			repository = `library/${repository}`;
		}

		return { registry, repository };
	}

	/**
	 * Process Docker image update check job
	 */
	async process(_job) {
		const startTime = Date.now();
		console.log("🐳 Starting Docker image update check...");

		// Clear token cache at start of each run
		this.tokenCache.clear();

		try {
			// Get all Docker images that have a digest
			// Note: repository is required (non-nullable) in schema, so we don't need to check it
			const images = await prisma.docker_images.findMany({
				where: {
					digest: {
						not: null,
					},
				},
				include: {
					docker_image_updates: true,
				},
			});

			console.log(`📦 Found ${images.length} images to check for updates`);

			let checkedCount = 0;
			let updateCount = 0;
			let errorCount = 0;
			const errors = [];

			// Process images in batches to avoid overwhelming the API
			const batchSize = 10;
			for (let i = 0; i < images.length; i += batchSize) {
				const batch = images.slice(i, i + batchSize);

				// Process batch concurrently with Promise.allSettled for error tolerance
				const _results = await Promise.allSettled(
					batch.map(async (image) => {
						try {
							checkedCount++;

							// Skip local images (no digest means they're local)
							if (!image.digest || image.digest.trim() === "") {
								return { image, skipped: true, reason: "No digest" };
							}

							// Get clean digest (remove sha256: prefix if present)
							const localDigest = image.digest.startsWith("sha256:")
								? image.digest.substring(7)
								: image.digest;

							// Get remote digest from registry
							const remoteDigest = await this.getRemoteDigest(
								image.repository,
								image.tag || "latest",
							);

							// Compare digests
							if (localDigest !== remoteDigest) {
								console.log(
									`🔄 Update found: ${image.repository}:${image.tag} (local: ${localDigest.substring(0, 12)}..., remote: ${remoteDigest.substring(0, 12)}...)`,
								);

								// Store digest info in changelog_url field as JSON
								const digestInfo = JSON.stringify({
									method: "digest_comparison",
									current_digest: localDigest,
									available_digest: remoteDigest,
									checked_at: new Date().toISOString(),
								});

								// Upsert the update record
								await prisma.docker_image_updates.upsert({
									where: {
										image_id_available_tag: {
											image_id: image.id,
											available_tag: image.tag || "latest",
										},
									},
									update: {
										updated_at: new Date(),
										changelog_url: digestInfo,
										severity: "digest_changed",
									},
									create: {
										id: uuidv4(),
										image_id: image.id,
										current_tag: image.tag || "latest",
										available_tag: image.tag || "latest",
										severity: "digest_changed",
										changelog_url: digestInfo,
										updated_at: new Date(),
									},
								});

								// Update last_checked timestamp on image
								await prisma.docker_images.update({
									where: { id: image.id },
									data: { last_checked: new Date() },
								});

								updateCount++;
								return { image, updated: true };
							} else {
								// No update - still update last_checked
								await prisma.docker_images.update({
									where: { id: image.id },
									data: { last_checked: new Date() },
								});

								// Remove existing update record if digest matches now
								const existingUpdate = image.docker_image_updates?.find(
									(u) => u.available_tag === (image.tag || "latest"),
								);
								if (existingUpdate) {
									await prisma.docker_image_updates.delete({
										where: { id: existingUpdate.id },
									});
								}

								return { image, updated: false };
							}
						} catch (error) {
							errorCount++;
							const errorMsg = `Error checking ${image.repository}:${image.tag}: ${error.message}`;
							errors.push(errorMsg);
							console.error(`❌ ${errorMsg}`);

							// Still update last_checked even on error
							try {
								await prisma.docker_images.update({
									where: { id: image.id },
									data: { last_checked: new Date() },
								});
							} catch (_updateError) {
								// Ignore update errors
							}

							return { image, error: error.message };
						}
					}),
				);

				// Log batch progress
				if (i + batchSize < images.length) {
					console.log(
						`⏳ Processed ${Math.min(i + batchSize, images.length)}/${images.length} images...`,
					);
				}

				// Small delay between batches to be respectful to registries
				if (i + batchSize < images.length) {
					await new Promise((resolve) => setTimeout(resolve, 500));
				}
			}

			const executionTime = Date.now() - startTime;
			console.log(
				`✅ Docker image update check completed in ${executionTime}ms - Checked: ${checkedCount}, Updates: ${updateCount}, Errors: ${errorCount}`,
			);

			return {
				success: true,
				checked: checkedCount,
				updates: updateCount,
				errors: errorCount,
				executionTime,
				errorDetails: errors,
			};
		} catch (error) {
			const executionTime = Date.now() - startTime;
			console.error(
				`❌ Docker image update check failed after ${executionTime}ms:`,
				error.message,
			);
			throw error;
		}
	}

	/**
	 * Schedule recurring Docker image update check (daily at 2 AM)
	 */
	async schedule() {
		const job = await this.queueManager.queues[this.queueName].add(
			"docker-image-update-check",
			{},
			{
				repeat: { cron: "0 2 * * *" }, // Daily at 2 AM
				jobId: "docker-image-update-check-recurring",
			},
		);
		console.log("✅ Docker image update check scheduled");
		return job;
	}

	/**
	 * Trigger manual Docker image update check
	 */
	async triggerManual() {
		const job = await this.queueManager.queues[this.queueName].add(
			"docker-image-update-check-manual",
			{},
			{ priority: 1 },
		);
		console.log("✅ Manual Docker image update check triggered");
		return job;
	}
}

module.exports = DockerImageUpdateCheck;
