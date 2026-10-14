import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
	defaultRdpKeyboardLayout,
	loadRdpKeyboardLayout,
	RDP_KEYBOARD_LAYOUTS,
	saveRdpKeyboardLayout,
} from "../../utils/rdpKeyboardLayouts";
import serverLayouts from "../../utils/rdpKeyboardLayouts.json";

describe("defaultRdpKeyboardLayout", () => {
	it.each([
		[["en-GB"], "en-gb-qwerty"],
		[["en-IE"], "en-gb-qwerty"],
		[["en-US"], "en-us-qwerty"],
		[["en-AU"], "en-us-qwerty"],
		[["de-DE"], "de-de-qwertz"],
		[["de-CH"], "de-ch-qwertz"],
		[["fr-FR"], "fr-fr-azerty"],
		[["fr-CA"], "fr-ca-qwerty"],
		[["es-MX"], "es-latam-qwerty"],
		[["es-ES"], "es-es-qwerty"],
		[["pt-BR"], "pt-br-qwerty"],
		[["nb-NO"], "no-no-qwerty"],
		[["zh-CN", "en-GB"], "en-gb-qwerty"],
		[["zh-CN"], "en-us-qwerty"],
		[[], "en-us-qwerty"],
	])("maps %j to %s", (languages, expected) => {
		expect(defaultRdpKeyboardLayout(languages)).toBe(expected);
	});

	it("offers exactly the layouts the server allows", () => {
		expect(RDP_KEYBOARD_LAYOUTS.map((l) => l.value).sort()).toEqual(
			[...serverLayouts].sort(),
		);
	});
});

describe("per-host keyboard layout storage", () => {
	let store;
	beforeEach(() => {
		store = {};
		vi.spyOn(localStorage, "getItem").mockImplementation(
			(k) => store[k] ?? null,
		);
		vi.spyOn(localStorage, "setItem").mockImplementation((k, v) => {
			store[k] = v;
		});
	});
	afterEach(() => vi.restoreAllMocks());

	it("remembers the layout per host", () => {
		saveRdpKeyboardLayout("h1", "de-de-qwertz");
		expect(loadRdpKeyboardLayout("h1", ["en-GB"])).toBe("de-de-qwertz");
		expect(loadRdpKeyboardLayout("h2", ["en-GB"])).toBe("en-gb-qwerty");
	});

	it("ignores a stored value that is not a supported layout", () => {
		store.rdp_keyboard_layout_h1 = "failsafe";
		expect(loadRdpKeyboardLayout("h1", ["fr-FR"])).toBe("fr-fr-azerty");
	});
});
