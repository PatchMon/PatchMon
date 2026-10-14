// Must match the server and agent message for the same refusal.
export const PACMAN_SELECTED_PATCH_UNSUPPORTED =
	"Arch Linux does not support partial upgrades, so PatchMon cannot patch selected packages on this host. Use Patch all.";

export const isPacmanHost = (host) =>
	(host?.package_manager ?? host?.packageManager ?? "").toLowerCase() ===
	"pacman";
