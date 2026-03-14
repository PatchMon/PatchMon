/**
 * Docker-related utility functions for the frontend
 */

/**
 * Generate a registry link for a Docker image based on its repository and source
 * @param {string} repository - The full repository name (e.g., "ghcr.io/owner/repo")
 * @param {string} source - The detected source (github, gitlab, docker-hub, etc.)
 * @returns {string|null} - The URL to the registry page, or null if unknown
 */
export function generateRegistryLink(repository, source) {
	if (!repository) {
		return null;
	}

	// Parse the domain and path from the repository
	const parts = repository.split("/");
	let domain = "";
	let path = "";

	// Check if repository has a domain (contains a dot)
	if (parts[0].includes(".") || parts[0].includes(":")) {
		domain = parts[0];
		path = parts.slice(1).join("/");
	} else {
		// No domain means Docker Hub
		domain = "docker.io";
		path = repository;
	}

	switch (source) {
		case "docker-hub":
		case "docker.io": {
			// Docker Hub: https://hub.docker.com/r/{path} or https://hub.docker.com/_/{path} for official images
			// Official images are those without a namespace (e.g., "postgres" not "user/postgres")
			// or explicitly prefixed with "library/"
			if (path.startsWith("library/")) {
				const cleanPath = path.replace("library/", "");
				return `https://hub.docker.com/_/${cleanPath}`;
			}
			// Check if it's an official image (single part, no slash after removing library/)
			if (!path.includes("/")) {
				return `https://hub.docker.com/_/${path}`;
			}
			// Regular user/org image
			return `https://hub.docker.com/r/${path}`;
		}

		case "github":
		case "ghcr.io": {
			// GitHub Container Registry
			// Format: ghcr.io/{owner}/{package} or ghcr.io/{owner}/{repo}/{package}
			// URL format: https://github.com/{owner}/{repo}/pkgs/container/{package}
			if (domain === "ghcr.io" && path) {
				const pathParts = path.split("/");
				if (pathParts.length === 2) {
					// Simple case: ghcr.io/owner/package -> github.com/owner/owner/pkgs/container/package
					// OR: ghcr.io/owner/repo -> github.com/owner/repo/pkgs/container/{package}
					// Actually, for 2 parts it's owner/package, and repo is same as owner typically
					const owner = pathParts[0];
					const packageName = pathParts[1];
					return `https://github.com/${owner}/${owner}/pkgs/container/${packageName}`;
				} else if (pathParts.length >= 3) {
					// Extended case: ghcr.io/owner/repo/package -> github.com/owner/repo/pkgs/container/package
					const owner = pathParts[0];
					const repo = pathParts[1];
					const packageName = pathParts.slice(2).join("/");
					return `https://github.com/${owner}/${repo}/pkgs/container/${packageName}`;
				}
			}
			// Legacy GitHub Packages
			if (domain === "docker.pkg.github.com" && path) {
				const pathParts = path.split("/");
				if (pathParts.length >= 1) {
					return `https://github.com/${pathParts[0]}/packages`;
				}
			}
			return null;
		}

		case "gitlab":
		case "registry.gitlab.com": {
			// GitLab Container Registry
			if (path) {
				return `https://gitlab.com/${path}/container_registry`;
			}
			return null;
		}

		case "google":
		case "gcr.io": {
			// Google Container Registry
			if (domain.includes("gcr.io") || domain.includes("pkg.dev")) {
				return `https://console.cloud.google.com/gcr/images/${path}`;
			}
			return null;
		}

		case "quay":
		case "quay.io": {
			// Quay.io
			if (path) {
				return `https://quay.io/repository/${path}`;
			}
			return null;
		}

		case "redhat":
		case "registry.access.redhat.com": {
			// Red Hat
			if (path) {
				return `https://access.redhat.com/containers/#/registry.access.redhat.com/${path}`;
			}
			return null;
		}

		case "azure":
		case "azurecr.io": {
			// Azure Container Registry
			if (domain.includes("azurecr.io")) {
				const registryName = domain.split(".")[0];
				return `https://portal.azure.com/#view/Microsoft_Azure_ContainerRegistries/RepositoryBlade/registryName/${registryName}/repositoryName/${path}`;
			}
			return null;
		}

		case "aws":
		case "amazonaws.com": {
			// AWS ECR
			if (domain.includes("amazonaws.com")) {
				const domainParts = domain.split(".");
				const region = domainParts[3]; // Extract region
				return `https://${region}.console.aws.amazon.com/ecr/repositories/private/${path}`;
			}
			return null;
		}

		case "private":
			// For private registries, try to construct a basic URL
			if (domain) {
				return `https://${domain}`;
			}
			return null;

		default:
			return null;
	}
}

/**
 * Get a user-friendly display name for a registry source
 * @param {string} source - The source identifier
 * @returns {string} - Human-readable source name
 */
export function getSourceDisplayName(source) {
	const sourceNames = {
		"docker-hub": "Docker Hub",
		github: "GitHub",
		gitlab: "GitLab",
		google: "Google",
		quay: "Quay.io",
		redhat: "Red Hat",
		azure: "Azure",
		aws: "AWS ECR",
		private: "Private Registry",
		local: "Local",
		unknown: "Unknown",
	};

	return sourceNames[source] || source;
}
