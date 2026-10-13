/**
 * Docker-related utility functions for the frontend
 */

const DOCKER_HUB_HOSTS = new Set([
	"docker.io",
	"index.docker.io",
	"registry-1.docker.io",
]);

const hostMatches = (domain, base) =>
	domain === base || domain.endsWith(`.${base}`);

const isLocalRegistryHost = (domain) => {
	const host = domain.replace(/:\d+$/, "");
	return (
		host === "localhost" ||
		host.startsWith("[") ||
		/^\d{1,3}(\.\d{1,3}){3}$/.test(host) ||
		!host.includes(".")
	);
};

export function splitRepository(repository) {
	const parts = repository.split("/");
	const first = parts[0];
	if (
		parts.length > 1 &&
		(first.includes(".") || first.includes(":") || first === "localhost")
	) {
		return { domain: first.toLowerCase(), path: parts.slice(1).join("/") };
	}
	return { domain: null, path: repository };
}

const dockerHubLink = (path) => {
	const clean = path.startsWith("library/") ? path.slice(8) : path;
	if (!clean) return null;
	return clean.includes("/")
		? `https://hub.docker.com/r/${clean}`
		: `https://hub.docker.com/_/${clean}`;
};

const githubLink = (domain, path) => {
	const pathParts = path.split("/").filter(Boolean);
	if (domain === "ghcr.io") {
		if (pathParts.length === 2) {
			const [owner, packageName] = pathParts;
			return `https://github.com/${owner}/${owner}/pkgs/container/${packageName}`;
		}
		if (pathParts.length >= 3) {
			const [owner, repo, ...rest] = pathParts;
			return `https://github.com/${owner}/${repo}/pkgs/container/${rest.join("/")}`;
		}
		return null;
	}
	if (pathParts.length >= 1) {
		return `https://github.com/${pathParts[0]}/packages`;
	}
	return null;
};

export function generateRegistryLink(repository, source) {
	if (!repository || repository === "<none>" || source === "local") {
		return null;
	}

	const { domain, path } = splitRepository(repository);
	if (!path) return null;

	if (domain === null || DOCKER_HUB_HOSTS.has(domain)) {
		return dockerHubLink(path);
	}

	if (domain === "ghcr.io" || domain === "docker.pkg.github.com") {
		return githubLink(domain, path);
	}

	if (domain === "registry.gitlab.com") {
		return `https://gitlab.com/${path}/container_registry`;
	}

	if (hostMatches(domain, "gcr.io") || hostMatches(domain, "pkg.dev")) {
		return `https://console.cloud.google.com/gcr/images/${path}`;
	}

	if (domain === "quay.io") {
		return `https://quay.io/repository/${path}`;
	}

	if (domain === "registry.access.redhat.com") {
		return `https://access.redhat.com/containers/#/registry.access.redhat.com/${path}`;
	}

	if (hostMatches(domain, "azurecr.io") && domain !== "azurecr.io") {
		const registryName = domain.split(".")[0];
		return `https://portal.azure.com/#view/Microsoft_Azure_ContainerRegistries/RepositoryBlade/registryName/${registryName}/repositoryName/${path}`;
	}

	if (hostMatches(domain, "amazonaws.com")) {
		const region = domain.split(".")[3];
		if (!region) return null;
		return `https://${region}.console.aws.amazon.com/ecr/repositories/private/${path}`;
	}

	if (isLocalRegistryHost(domain)) {
		return null;
	}

	return `https://${domain}`;
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

export function shortImageId(imageId) {
	if (!imageId) return "";
	return imageId.replace(/^sha256:/, "").slice(0, 12);
}

export function shortDigest(digest) {
	if (!digest) return "";
	const hex = digest.replace(/^sha256:/, "");
	return `sha256:${hex.slice(0, 12)}`;
}

export const UPDATE_STATUS_FILTERS = [
	{ value: "all", label: "All statuses" },
	{ value: "update_available", label: "Update available" },
	{ value: "recreate_needed", label: "Recreate needed" },
	{ value: "up_to_date", label: "Up to date" },
	{ value: "unknown", label: "Unknown" },
	{ value: "not_checked", label: "Not checked" },
];

const CHECK_REASONS = {
	rate_limited:
		"The registry rate limited the check. It will be retried on the next run.",
	auth_failed: "The registry rejected the credentials configured on the host.",
	tag_not_found: "The tag no longer exists in the registry.",
	unreachable: "The registry could not be reached from the host.",
	unsupported_auth:
		"The registry asked for an authentication method the agent does not support.",
	insecure_registry_disallowed:
		"The registry is served over plain HTTP and is not marked insecure in the Docker daemon.",
	no_digest: "The registry did not return a digest for this tag.",
};

export function describeCheckReason(reason) {
	return CHECK_REASONS[reason] || "The update check did not complete.";
}

const STATUS_META = {
	update_available: {
		label: "Update available",
		className: "badge-warning",
		tooltip: () =>
			"The registry has a different image for this tag than the one on the host.",
	},
	recreate_needed: {
		label: "Recreate needed",
		className: "badge-primary",
		tooltip: () =>
			"A newer image is already pulled on this host; recreate the container to use it",
	},
	up_to_date: {
		label: "Up to date",
		className: "badge-success",
		tooltip: ({ checkedAt, formatTime }) =>
			checkedAt ? `Checked ${formatTime(checkedAt)}` : "",
	},
	unknown: {
		label: "Unknown",
		className: "badge-secondary",
		tooltip: ({ reason }) => describeCheckReason(reason),
	},
	local_build: {
		label: "Local build",
		className: "badge-secondary",
		tooltip: () =>
			"Built on the host, so there is no registry image to compare against.",
	},
	pinned: {
		label: "Pinned digest",
		className: "badge-secondary",
		tooltip: () =>
			"Referenced by digest, so the image cannot change under this reference.",
	},
	skipped: {
		label: "Untagged",
		className: "badge-secondary",
		tooltip: () => "The image has no tag to check against a registry.",
	},
	not_checked: {
		label: "Not checked",
		className: "badge-secondary",
		tooltip: () => "No update check has been reported for this image yet.",
	},
};

export function getUpdateStatusMeta(status, opts = {}) {
	const key = Object.hasOwn(STATUS_META, status) ? status : "not_checked";
	const meta = STATUS_META[key];
	return {
		status: key,
		label: meta.label,
		className: meta.className,
		tooltip: meta.tooltip({
			checkedAt: opts.checkedAt,
			reason: opts.reason,
			formatTime: opts.formatTime || ((d) => new Date(d).toLocaleString()),
		}),
	};
}
