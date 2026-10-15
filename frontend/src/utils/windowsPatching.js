import {
	isPacmanHost,
	PACMAN_SELECTED_PATCH_UNSUPPORTED,
} from "./patchSupport";

export const LINUX_DRY_RUN_PACKAGE_LIMIT = 5;
export const WINDOWS_DRY_RUN_PACKAGE_LIMIT = 100;

export const UNPATCHABLE_REASON_TEXT = {
	no_winget_id:
		"Not patchable: this app was not found in winget. Update it with its own updater.",
	not_pending: "Up to date",
	winget_pinned:
		"Pinned with winget pin; remove the pin on the host to update it",
	store_app: "Microsoft Store app: updates through the Microsoft Store",
	pacman_partial_upgrade: PACMAN_SELECTED_PATCH_UNSUPPORTED,
};

// Reasons that keep a pending update visible but block patching, each with
// its own neutral badge instead of "Not patchable".
const BLOCKED_UPDATE_BADGES = {
	winget_pinned: { key: "pinned", label: "Pinned in winget" },
	store_app: { key: "store-app", label: "Microsoft Store app" },
};

export const isBlockedUpdateReason = (reason) =>
	Object.hasOwn(BLOCKED_UPDATE_BADGES, reason || "");

const REJECT_REASON_TEXT = {
	...UNPATCHABLE_REASON_TEXT,
	not_found: "No longer in this host's inventory",
	invalid_identifier: "The update identifier is not valid",
};

// Builds a readable message from a trigger error, listing each rejected
// package with its reason when the server returns them.
export const patchErrorMessage = (err, fallback = "Request failed") => {
	const data = err?.response?.data;
	const base = data?.error || err?.message || fallback;
	const rejected = Array.isArray(data?.rejected) ? data.rejected : [];
	if (rejected.length === 0) return base;
	const list = rejected
		.map((r) => `${r.name}: ${REJECT_REASON_TEXT[r.reason] || r.reason}`)
		.join("; ");
	return `${base} (${list})`;
};

export const FEATURE_UPGRADE_WARNING =
	"Feature upgrades download several GB, can take over an hour and need one or more restarts to finish.";

export const RESTART_NOT_AUTOMATIC =
	"The host may need a restart afterwards. PatchMon does not restart it.";

export const RESTART_REQUIRED_BANNER =
	"A restart is required to finish installing updates.";

export const DEFAULT_WINDOWS_SCOPE = Object.freeze({
	updates: true,
	apps: true,
});

export const RESTART_REQUIRED_BADGE = Object.freeze({
	label: "Restart required",
	className: "badge-warning",
	title: "Installed, but the host must restart to finish installing it",
});

export const isPendingRestart = (row) =>
	!!(row?.pendingRestart ?? row?.pending_restart);

export const isWindowsOs = (osType) =>
	String(osType || "")
		.toLowerCase()
		.includes("windows");

export const hostOsType = (host) =>
	host?.os_type || host?.osType || host?.expected_platform || "";

const pick = (row, camel, snake) => {
	if (!row) return undefined;
	if (row[camel] !== undefined && row[camel] !== null) return row[camel];
	return row[snake] ?? undefined;
};

const hasValue = (v) => v !== undefined && v !== null && v !== "";

export const isFeatureUpgradeRow = (row) => {
	const explicit = pick(row, "isFeatureUpgrade", "is_feature_upgrade");
	if (explicit !== undefined) return !!explicit;
	const cats = pick(row, "wuaCategories", "wua_categories");
	const list = Array.isArray(cats)
		? cats
		: typeof cats === "string"
			? cats.split(",")
			: [];
	return list.some((c) => String(c).trim().toLowerCase() === "upgrades");
};

// Mirrors the server's patchable predicate. The server's `patchable` wins
// when present so the UI and trigger validation cannot disagree.
export const windowsPatchInfo = (row) => {
	const needsUpdate = !!pick(row, "needsUpdate", "needs_update");
	const patchSource = pick(row, "patchSource", "patch_source");
	const source =
		patchSource === "wua" || patchSource === "winget"
			? patchSource
			: hasValue(pick(row, "wuaGuid", "wua_guid"))
				? "wua"
				: hasValue(pick(row, "wingetId", "winget_id"))
					? "winget"
					: null;

	const serverPatchable = pick(row, "patchable", "patchable");
	const serverReason = pick(row, "notPatchableReason", "not_patchable_reason");
	let patchable;
	let reason;
	if (serverPatchable !== undefined) {
		patchable = !!serverPatchable;
		reason = patchable ? null : serverReason || null;
	} else if (!needsUpdate) {
		patchable = false;
		reason = "not_pending";
	} else {
		patchable = source !== null;
		reason = patchable ? null : "no_winget_id";
	}

	return {
		patchable,
		reason,
		reasonText: reason ? UNPATCHABLE_REASON_TEXT[reason] || reason : null,
		source,
		security: !!pick(row, "isSecurityUpdate", "is_security_update"),
		optional: !!pick(row, "wuaBrowseOnly", "wua_browse_only"),
		driver:
			String(pick(row, "wuaUpdateType", "wua_update_type") || "")
				.trim()
				.toLowerCase() === "driver",
		featureUpgrade: isFeatureUpgradeRow(row),
	};
};

export const windowsSourceLabel = (info) => {
	if (!info) return null;
	if (info.reason === "no_winget_id") return "Not patchable";
	if (info.source === "wua") return "Windows Update";
	if (info.source === "winget") return "winget";
	if (info.patchable || info.reason === "not_pending") return null;
	return "Not patchable";
};

// Ordered badge list for a Windows host-package row. `.badge-*` classes only.
export const windowsBadges = (info) => {
	if (!info) return [];
	const out = [];
	if (info.reason === "no_winget_id") {
		out.push({
			key: "not-patchable",
			label: "Not patchable",
			className: "badge-secondary",
			title: info.reasonText,
		});
		return out;
	}
	const blocked = BLOCKED_UPDATE_BADGES[info.reason];
	if (blocked) {
		out.push({
			...blocked,
			className: "badge-secondary",
			title: info.reasonText,
		});
	}
	if (info.featureUpgrade) {
		out.push({
			key: "feature-upgrade",
			label: "Feature upgrade",
			className: "badge-warning",
		});
	}
	if (info.security) {
		out.push({ key: "security", label: "Security", className: "badge-danger" });
	}
	if (info.optional) {
		out.push({
			key: "optional",
			label: "Optional",
			className: "badge-secondary",
		});
	}
	if (info.driver) {
		out.push({ key: "driver", label: "Driver", className: "badge-secondary" });
	}
	return out;
};

export const dryRunPackageLimit = (osType) =>
	isWindowsOs(osType)
		? WINDOWS_DRY_RUN_PACKAGE_LIMIT
		: LINUX_DRY_RUN_PACKAGE_LIMIT;

export const isWindowsScopeValid = (scope) => !!(scope?.updates || scope?.apps);

export const windowsPatchAllSummary = (scope) => {
	const s = scope || DEFAULT_WINDOWS_SCOPE;
	if (s.updates && s.apps) {
		return "Installs all pending Windows updates, except optional, driver and feature upgrades, and upgrades the apps winget can update.";
	}
	if (s.updates) {
		return "Installs all pending Windows updates, except optional, driver and feature upgrades.";
	}
	if (s.apps) {
		return "Upgrades the apps winget can update.";
	}
	return "Nothing selected. Choose Windows Updates, Applications (winget), or both.";
};

// Splits package-host pairs into ones the wizard may patch and ones it must
// list as not patchable.
export const partitionPatchableHosts = (hosts) => {
	const patchable = [];
	const unpatchable = [];
	for (const h of hosts || []) {
		if (isPacmanHost(h)) {
			unpatchable.push({ host: h, reason: "pacman_partial_upgrade" });
			continue;
		}
		if (!isWindowsOs(hostOsType(h))) {
			patchable.push(h);
			continue;
		}
		const info = windowsPatchInfo(h);
		if (info.patchable) patchable.push(h);
		else unpatchable.push({ host: h, reason: info.reason, info });
	}
	return { patchable, unpatchable };
};

export const RUN_ITEM_STATUS = {
	pending: { label: "Pending", className: "badge-secondary" },
	would_install: { label: "Would install", className: "badge-primary" },
	not_applicable: { label: "Not applicable", className: "badge-secondary" },
	installed: { label: "Installed", className: "badge-success" },
	already_installed: {
		label: "Already installed",
		className: "badge-secondary",
	},
	installed_pending_reboot: {
		label: "Installed, restart required",
		className: "badge-warning",
	},
	installed_with_errors: {
		label: "Installed with errors",
		className: "badge-warning",
	},
	failed: { label: "Failed", className: "badge-danger" },
	aborted: { label: "Aborted", className: "badge-danger" },
	skipped: { label: "Skipped", className: "badge-secondary" },
};

export const runItemStatus = (status) =>
	RUN_ITEM_STATUS[status] || {
		label: status ? String(status) : "Unknown",
		className: "badge-secondary",
	};

export const runItemSourceLabel = (kind) =>
	kind === "wua" ? "Windows Update" : kind === "winget" ? "winget" : kind || "";

// Formats an HRESULT or exit code as 0x-prefixed upper-case hex. Negative
// numbers are treated as uint32, matching how Windows reports them.
export const formatResultCode = (value) => {
	if (value === undefined || value === null || value === "") return null;
	if (typeof value === "string") {
		const s = value.trim();
		if (/^0x[0-9a-f]+$/i.test(s)) return `0x${s.slice(2).toUpperCase()}`;
		if (!/^-?\d+$/.test(s)) return s;
		return formatResultCode(Number(s));
	}
	if (!Number.isFinite(value)) return null;
	const u = value < 0 ? value >>> 0 : value;
	return `0x${u.toString(16).toUpperCase().padStart(8, "0")}`;
};

// Message and code for the Result column. The code is dropped when the
// message already contains it, so it never shows twice.
export const runItemResultParts = (item) => {
	const message = item?.message ? String(item.message) : "";
	const code = runItemResultCode(item);
	if (code && message.toLowerCase().includes(code.toLowerCase())) {
		return { message, code: null };
	}
	return { message, code };
};

// The hex code to show for a run item: the HRESULT when it reports a failure,
// otherwise a non-zero exit code. A zero HRESULT is success and shows nothing.
export const runItemResultCode = (item) => {
	const hr = formatResultCode(item?.hresult);
	if (hr && hr !== "0x00000000") return hr;
	const exit = item?.exit_code;
	if (exit !== undefined && exit !== null && Number(exit) !== 0) {
		return formatResultCode(Number(exit));
	}
	return null;
};

export const ACTIVE_RUN_STATUSES = new Set([
	"queued",
	"scheduled",
	"running",
	"pending_validation",
]);

export const isRunActive = (status) => ACTIVE_RUN_STATUSES.has(status);
