/**
 * Unit tests for hydrateTLSMode and hydrateAllowInsecureAuth, which map a
 * stored SMTP destination config back onto the TLS controls when an existing
 * channel is reopened.
 */

import { describe, expect, it } from "vitest";
import {
	hydrateAllowInsecureAuth,
	hydrateTLSMode,
} from "../../pages/settings/AlertChannels";

describe("hydrateTLSMode", () => {
	it("defaults to starttls when there is no config", () => {
		expect(hydrateTLSMode(null)).toBe("starttls");
		expect(hydrateTLSMode(undefined)).toBe("starttls");
		expect(hydrateTLSMode("")).toBe("starttls");
		expect(hydrateTLSMode(0)).toBe("starttls");
		expect(hydrateTLSMode("starttls")).toBe("starttls");
	});

	it("honours an explicit, known tls_mode", () => {
		expect(hydrateTLSMode({ tls_mode: "starttls" })).toBe("starttls");
		expect(hydrateTLSMode({ tls_mode: "tls" })).toBe("tls");
		expect(hydrateTLSMode({ tls_mode: "none" })).toBe("none");
		expect(hydrateTLSMode({ tls_mode: "auto" })).toBe("auto");
	});

	it("ignores an unknown or non-string tls_mode and falls through", () => {
		expect(hydrateTLSMode({ tls_mode: "ssl" })).toBe("auto");
		expect(hydrateTLSMode({ tls_mode: 1 })).toBe("auto");
		expect(hydrateTLSMode({ tls_mode: "tls", use_tls: false })).toBe("tls");
		expect(hydrateTLSMode({ tls_mode: "ssl", use_tls: false })).toBe("none");
	});

	it("maps legacy use_tls:false to none", () => {
		expect(hydrateTLSMode({ use_tls: false })).toBe("none");
	});

	it("maps every other legacy config to auto", () => {
		expect(hydrateTLSMode({})).toBe("auto");
		expect(hydrateTLSMode({ use_tls: true })).toBe("auto");
		// use_tls is only treated as legacy "none" on an exact false, never on
		// a falsy-but-absent value.
		expect(hydrateTLSMode({ use_tls: undefined })).toBe("auto");
		expect(hydrateTLSMode({ smtp_host: "mail.example.com" })).toBe("auto");
	});
});

describe("hydrateAllowInsecureAuth", () => {
	it("returns false when there is no config", () => {
		expect(hydrateAllowInsecureAuth(null)).toBe(false);
		expect(hydrateAllowInsecureAuth(undefined)).toBe(false);
		expect(hydrateAllowInsecureAuth("")).toBe(false);
		expect(hydrateAllowInsecureAuth(0)).toBe(false);
	});

	it("returns false when the field is absent, as on older rows", () => {
		expect(hydrateAllowInsecureAuth({})).toBe(false);
		expect(hydrateAllowInsecureAuth({ tls_mode: "none" })).toBe(false);
	});

	it("returns true only on an exact boolean true", () => {
		expect(hydrateAllowInsecureAuth({ allow_insecure_auth: true })).toBe(true);
		expect(hydrateAllowInsecureAuth({ allow_insecure_auth: false })).toBe(
			false,
		);
		expect(hydrateAllowInsecureAuth({ allow_insecure_auth: "true" })).toBe(
			false,
		);
		expect(hydrateAllowInsecureAuth({ allow_insecure_auth: 1 })).toBe(false);
		expect(hydrateAllowInsecureAuth({ allow_insecure_auth: null })).toBe(false);
	});
});
