import { useAuth } from "../contexts/AuthContext";

/**
 * useModule returns whether a given module key is enabled for the current
 * multi-context (tenant). Drives UI feature flagging so that disabled
 * features are hidden rather than clicking through to a 403 response.
 *
 * The backend exposes `tenant.modules` on GET /api/v1/me/context as either
 * a comma-separated list (e.g. "core,patching,docker") or "*" for wildcard
 * (all modules, Max tier or single-context deployments).
 *
 * Usage:
 *   const patchingEnabled = useModule("patching");
 *   if (!patchingEnabled) return null;
 *
 * Returns true when:
 *   - no moduleKey is provided (defensive default),
 *   - the tenant has a wildcard ("*") module list,
 *   - the moduleKey appears in the comma-separated modules string.
 *
 * Module keys are the canonical strings defined in the server's module
 * catalog (see docs/research/per-host-pricing/10-tiers-v2...md).
 */
export const useModule = (moduleKey) => {
	const { hasModule } = useAuth();
	return hasModule(moduleKey);
};

export default useModule;
