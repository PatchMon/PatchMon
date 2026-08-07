/**
 * Logo path resolution for branding assets.
 * Maps legacy default paths (pre-rename) to the new *_default.* filenames
 * so existing DBs and stale paths don't 404.
 * Only maps the exact old schema default paths - custom uploads keep their paths.
 */
const LEGACY_DEFAULT_PATHS = {
	"/assets/logo_dark.png": "/assets/logo_dark_default.png",
	"/assets/logo_light.png": "/assets/logo_light_default.png",
	"/assets/logo_square.svg": "/assets/logo_square_default.svg",
	"/assets/favicon.svg": "/assets/logo_square_default.svg",
};

const DEFAULT_PATHS = {
	logo_dark: "/assets/logo_dark_default.png",
	logo_light: "/assets/logo_light_default.png",
	favicon: "/assets/logo_square_default.svg",
};

/**
 * Resolves a logo path from settings. Returns the path to use for the img src.
 * - null/undefined: use disk default
 * - /api/...: custom logo from API (return as-is)
 * - legacy paths: map to new *_default.* filenames
 */
export function resolveLogoPath(path, type) {
	if (!path) return DEFAULT_PATHS[type] || path;
	const normalized = path.startsWith("/") ? path : `/${path}`;
	if (normalized.startsWith("/api/")) return path;
	return LEGACY_DEFAULT_PATHS[normalized] || path;
}

/** Returns true if the path is a legacy default (mapped to new default). Custom /api/ paths return false. */
export function isLegacyDefaultPath(path) {
	if (!path) return true;
	const normalized = path.startsWith("/") ? path : `/${path}`;
	if (normalized.startsWith("/api/")) return false;
	return normalized in LEGACY_DEFAULT_PATHS;
}
