import { describe, expect, it } from "vitest";
import {
	dryRunPackageLimit,
	formatResultCode,
	isBlockedUpdateReason,
	isFeatureUpgradeRow,
	isPendingRestart,
	isWindowsScopeValid,
	partitionPatchableHosts,
	patchErrorMessage,
	runItemResultCode,
	runItemResultParts,
	runItemStatus,
	UNPATCHABLE_REASON_TEXT,
	windowsBadges,
	windowsPatchAllSummary,
	windowsPatchInfo,
	windowsSourceLabel,
} from "../../utils/windowsPatching";

describe("windowsPatchInfo", () => {
	it("trusts the server's patchable flag and reason", () => {
		const info = windowsPatchInfo({
			needsUpdate: true,
			patchable: false,
			notPatchableReason: "no_winget_id",
		});
		expect(info.patchable).toBe(false);
		expect(info.reason).toBe("no_winget_id");
		expect(info.reasonText).toMatch(/not found in winget/);
	});

	it("accepts snake_case fields", () => {
		const info = windowsPatchInfo({
			needs_update: true,
			patchable: true,
			patch_source: "winget",
		});
		expect(info.patchable).toBe(true);
		expect(info.source).toBe("winget");
		const blocked = windowsPatchInfo({
			needs_update: true,
			patchable: false,
			not_patchable_reason: "no_winget_id",
		});
		expect(blocked.reason).toBe("no_winget_id");
	});

	it("reads the server's camelCase package-host shape", () => {
		const info = windowsPatchInfo({
			needsUpdate: true,
			patchable: true,
			patchSource: "wua",
			wuaBrowseOnly: true,
			wuaUpdateType: "Driver",
			isFeatureUpgrade: true,
		});
		expect(info).toMatchObject({
			patchable: true,
			source: "wua",
			optional: true,
			driver: true,
			featureUpgrade: true,
		});
	});

	it.each([
		[{ needsUpdate: true, wuaGuid: "g" }, true, null, "wua"],
		[{ needsUpdate: true, wingetId: "7zip.7zip" }, true, null, "winget"],
		[{ needsUpdate: true }, false, "no_winget_id", null],
		[
			{ needsUpdate: false, wingetId: "7zip.7zip" },
			false,
			"not_pending",
			"winget",
		],
	])("falls back to the server's patchable predicate for %j", (row, patchable, reason, source) => {
		const info = windowsPatchInfo(row);
		expect(info.patchable).toBe(patchable);
		expect(info.reason).toBe(reason);
		expect(info.source).toBe(source);
	});

	it("flags optional, driver and feature upgrade rows", () => {
		const info = windowsPatchInfo({
			needsUpdate: true,
			wuaGuid: "g",
			wuaBrowseOnly: true,
			wuaUpdateType: "Driver",
			wuaCategories: ["Upgrades"],
		});
		expect(info.optional).toBe(true);
		expect(info.driver).toBe(true);
		expect(info.featureUpgrade).toBe(true);
	});
});

describe("isFeatureUpgradeRow", () => {
	it("prefers the explicit flag", () => {
		expect(
			isFeatureUpgradeRow({
				isFeatureUpgrade: false,
				wuaCategories: ["Upgrades"],
			}),
		).toBe(false);
	});

	it("reads a comma-separated category string", () => {
		expect(
			isFeatureUpgradeRow({ wua_categories: "Security Updates, Upgrades" }),
		).toBe(true);
		expect(isFeatureUpgradeRow({ wua_categories: "Security Updates" })).toBe(
			false,
		);
	});
});

describe("Windows wording", () => {
	it.each([
		[{ needsUpdate: true, wuaGuid: "g" }, "Windows Update"],
		[{ needsUpdate: true, wingetId: "Notepad++.Notepad++" }, "winget"],
		[
			{
				needsUpdate: true,
				patchable: false,
				notPatchableReason: "no_winget_id",
			},
			"Not patchable",
		],
		[{ needsUpdate: false }, null],
	])("labels the source of %j", (row, label) => {
		expect(windowsSourceLabel(windowsPatchInfo(row))).toBe(label);
	});

	it("puts the feature upgrade badge first, in the warning style", () => {
		const badges = windowsBadges(
			windowsPatchInfo({
				needsUpdate: true,
				wuaGuid: "g",
				isSecurityUpdate: true,
				isFeatureUpgrade: true,
			}),
		);
		expect(badges.map((b) => b.label)).toEqual(["Feature upgrade", "Security"]);
		expect(badges[0].className).toBe("badge-warning");
	});

	it("shows only Not patchable, with the reason as tooltip, for registry-only apps", () => {
		const badges = windowsBadges(
			windowsPatchInfo({ needsUpdate: true, isSecurityUpdate: true }),
		);
		expect(badges).toHaveLength(1);
		expect(badges[0].label).toBe("Not patchable");
		expect(badges[0].title).toMatch(/own updater/);
	});

	it("drops the half of the Patch all text the user unticked", () => {
		expect(windowsPatchAllSummary({ updates: true, apps: true })).toMatch(
			/Windows updates.*winget/,
		);
		expect(windowsPatchAllSummary({ updates: true, apps: false })).not.toMatch(
			/winget/,
		);
		expect(windowsPatchAllSummary({ updates: false, apps: true })).not.toMatch(
			/Windows updates/,
		);
	});

	it("has no em-dashes in user-facing text", () => {
		const strings = [
			windowsPatchAllSummary({ updates: true, apps: true }),
			windowsPatchAllSummary({ updates: true, apps: false }),
			windowsPatchAllSummary({ updates: false, apps: true }),
			windowsPatchAllSummary({ updates: false, apps: false }),
			windowsPatchInfo({ needsUpdate: true }).reasonText,
		];
		for (const s of strings) expect(s).not.toContain("—");
	});
});

describe("scope and limits", () => {
	it("requires at least one Patch all scope", () => {
		expect(isWindowsScopeValid({ updates: true, apps: false })).toBe(true);
		expect(isWindowsScopeValid({ updates: false, apps: true })).toBe(true);
		expect(isWindowsScopeValid({ updates: false, apps: false })).toBe(false);
		expect(isWindowsScopeValid(undefined)).toBe(false);
	});

	it("uses a per-OS dry-run cap", () => {
		expect(dryRunPackageLimit("Windows 11 Pro")).toBe(100);
		expect(dryRunPackageLimit("Ubuntu")).toBe(5);
		expect(dryRunPackageLimit("")).toBe(5);
	});
});

describe("partitionPatchableHosts", () => {
	it("keeps Linux hosts and splits Windows pairs by patchability", () => {
		const hosts = [
			{ hostId: "l1", osType: "Ubuntu", needsUpdate: true },
			{
				hostId: "w1",
				osType: "Windows 11",
				needsUpdate: true,
				patchable: true,
			},
			{
				hostId: "w2",
				osType: "Windows Server 2025",
				needsUpdate: true,
				patchable: false,
				notPatchableReason: "no_winget_id",
			},
		];
		const { patchable, unpatchable } = partitionPatchableHosts(hosts);
		expect(patchable.map((h) => h.hostId)).toEqual(["l1", "w1"]);
		expect(unpatchable).toHaveLength(1);
		expect(unpatchable[0].host.hostId).toBe("w2");
		expect(unpatchable[0].reason).toBe("no_winget_id");
	});

	it("lists Arch hosts as not patchable, since selected patching is a partial upgrade", () => {
		const hosts = [
			{
				hostId: "a1",
				osType: "Arch Linux",
				packageManager: "pacman",
				needsUpdate: true,
			},
			{
				hostId: "d1",
				osType: "Debian",
				packageManager: "apt",
				needsUpdate: true,
			},
		];
		const { patchable, unpatchable } = partitionPatchableHosts(hosts);
		expect(patchable.map((h) => h.hostId)).toEqual(["d1"]);
		expect(unpatchable).toHaveLength(1);
		expect(unpatchable[0].host.hostId).toBe("a1");
		expect(unpatchable[0].reason).toBe("pacman_partial_upgrade");
		expect(UNPATCHABLE_REASON_TEXT.pacman_partial_upgrade).toMatch(
			/partial upgrades/,
		);
	});

	it("tolerates a missing list", () => {
		expect(partitionPatchableHosts(undefined)).toEqual({
			patchable: [],
			unpatchable: [],
		});
	});
});

describe("run item results", () => {
	it("maps every run item status to a badge", () => {
		for (const s of [
			"pending",
			"would_install",
			"not_applicable",
			"installed",
			"already_installed",
			"installed_pending_reboot",
			"installed_with_errors",
			"failed",
			"aborted",
			"skipped",
		]) {
			expect(runItemStatus(s).className).toMatch(/^badge-/);
		}
		expect(runItemStatus("installed").className).toBe("badge-success");
		expect(runItemStatus("installed_with_errors").className).toBe(
			"badge-warning",
		);
		expect(runItemStatus("already_installed")).toEqual({
			label: "Already installed",
			className: "badge-secondary",
		});
		expect(runItemStatus("installed_pending_reboot")).toEqual({
			label: "Installed, restart required",
			className: "badge-warning",
		});
		expect(runItemStatus("would_install").className).toBe("badge-primary");
		expect(runItemStatus("failed").className).toBe("badge-danger");
		expect(runItemStatus("aborted").className).toBe("badge-danger");
		expect(runItemStatus("skipped").className).toBe("badge-secondary");
	});

	it("formats HRESULTs and negative exit codes as uint32 hex", () => {
		expect(formatResultCode("0x800f0922")).toBe("0x800F0922");
		expect(formatResultCode(-2145124329)).toBe("0x80240017");
		expect(formatResultCode("-1978335189")).toBe("0x8A15002B");
		expect(formatResultCode(0)).toBe("0x00000000");
		expect(formatResultCode(null)).toBeNull();
	});

	it("shows a failing HRESULT, else a non-zero exit code, else nothing", () => {
		expect(runItemResultCode({ hresult: "0x800F0922" })).toBe("0x800F0922");
		expect(
			runItemResultCode({ hresult: "0x00000000", exit_code: null }),
		).toBeNull();
		expect(runItemResultCode({ exit_code: -1978335189 })).toBe("0x8A15002B");
		expect(runItemResultCode({ exit_code: 0 })).toBeNull();
	});
});

describe("patchErrorMessage", () => {
	it("lists each rejected package with its reason", () => {
		const msg = patchErrorMessage({
			response: {
				data: {
					error: "Some packages cannot be patched",
					rejected: [
						{ name: "Legacy App", reason: "no_winget_id" },
						{ name: "Gone", reason: "not_found" },
					],
				},
			},
		});
		expect(msg).toMatch(/^Some packages cannot be patched \(/);
		expect(msg).toMatch(/Legacy App: Not patchable/);
		expect(msg).toMatch(/Gone: No longer in this host's inventory/);
	});

	it("falls back to the plain error or message", () => {
		expect(
			patchErrorMessage({ response: { data: { error: "Update the agent" } } }),
		).toBe("Update the agent");
		expect(patchErrorMessage({ message: "Network Error" })).toBe(
			"Network Error",
		);
		expect(patchErrorMessage({})).toBe("Request failed");
	});
});

describe("pending restart and result parts", () => {
	it("reads pendingRestart in either case", () => {
		expect(isPendingRestart({ pendingRestart: true })).toBe(true);
		expect(isPendingRestart({ pending_restart: true })).toBe(true);
		expect(isPendingRestart({ needsUpdate: false })).toBe(false);
		expect(isPendingRestart(undefined)).toBe(false);
	});

	it("drops a code the message already contains", () => {
		expect(
			runItemResultParts({
				message: "winget exited with 0x80072ee7",
				exit_code: -2147012889,
			}),
		).toEqual({ message: "winget exited with 0x80072ee7", code: null });
		expect(
			runItemResultParts({
				message: "Installation failed",
				hresult: "0x800F0922",
			}),
		).toEqual({ message: "Installation failed", code: "0x800F0922" });
	});
});

describe("winget pinned apps", () => {
	const pinned = {
		needsUpdate: true,
		availableVersion: "26.05",
		patchable: false,
		notPatchableReason: "winget_pinned",
		patchSource: "winget",
	};

	it("is not patchable, keeps its winget source and shows a neutral pin badge", () => {
		const info = windowsPatchInfo(pinned);
		expect(info.patchable).toBe(false);
		expect(info.reasonText).toBe(
			"Pinned with winget pin; remove the pin on the host to update it",
		);
		expect(windowsSourceLabel(info)).toBe("winget");
		expect(windowsBadges(info)).toEqual([
			{
				key: "pinned",
				label: "Pinned in winget",
				className: "badge-secondary",
				title: info.reasonText,
			},
		]);
	});

	it("is skipped by the wizard with its reason", () => {
		const { patchable, unpatchable } = partitionPatchableHosts([
			{ ...pinned, hostId: "w1", osType: "Windows 11" },
		]);
		expect(patchable).toHaveLength(0);
		expect(unpatchable[0].reason).toBe("winget_pinned");
	});

	it("names the pin when the server rejects a trigger", () => {
		expect(
			patchErrorMessage({
				response: {
					data: {
						error: "Some packages cannot be patched",
						rejected: [{ name: "7-Zip", reason: "winget_pinned" }],
					},
				},
			}),
		).toMatch(/7-Zip: Pinned with winget pin/);
	});

	it("renders a skipped pinned item as neutral, not failed", () => {
		expect(runItemStatus("skipped")).toEqual({
			label: "Skipped",
			className: "badge-secondary",
		});
	});
});

describe("Microsoft Store apps", () => {
	const store = {
		needsUpdate: true,
		patchable: false,
		notPatchableReason: "store_app",
		patchSource: "winget",
		wingetStoreApp: true,
	};

	it("shows a neutral Store badge with the Store tooltip", () => {
		const info = windowsPatchInfo(store);
		expect(info.patchable).toBe(false);
		expect(info.reasonText).toBe(
			"Microsoft Store app: updates through the Microsoft Store",
		);
		expect(windowsSourceLabel(info)).toBe("winget");
		expect(windowsBadges(info)).toEqual([
			{
				key: "store-app",
				label: "Microsoft Store app",
				className: "badge-secondary",
				title: info.reasonText,
			},
		]);
		expect(isBlockedUpdateReason("store_app")).toBe(true);
		expect(isBlockedUpdateReason("winget_pinned")).toBe(true);
		expect(isBlockedUpdateReason("no_winget_id")).toBe(false);
		expect(isBlockedUpdateReason(undefined)).toBe(false);
	});

	it("names the Store when the server rejects a trigger", () => {
		expect(
			patchErrorMessage({
				response: {
					data: {
						error: "Some packages cannot be patched",
						rejected: [{ name: "Windows Terminal", reason: "store_app" }],
					},
				},
			}),
		).toMatch(/Windows Terminal: Microsoft Store app: updates through/);
	});
});
