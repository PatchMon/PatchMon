/**
 * Unit tests for Guacamole auth-json utilities (RDP ticket signing/encryption)
 */

const {
	signAndEncrypt,
	createRdpAuthJson,
	JSON_DATASOURCE,
	RDP_CONNECTION_NAME,
} = require("../../src/utils/guacamoleAuth");

const VALID_KEY_HEX = "0123456789abcdef0123456789abcdef"; // 32 hex chars = 16 bytes

describe("guacamoleAuth", () => {
	describe("signAndEncrypt", () => {
		it("throws when secret key is not 32 hex characters (16 bytes decoded)", () => {
			expect(() => signAndEncrypt({}, "short")).toThrow(
				"GUACAMOLE_JSON_SECRET_KEY must be 32 hex chars",
			);
			expect(() => signAndEncrypt({}, "")).toThrow(
				"GUACAMOLE_JSON_SECRET_KEY must be 32 hex chars",
			);
			// 30 hex chars => 15 bytes decoded (invalid)
			expect(() =>
				signAndEncrypt({}, "0123456789abcdef0123456789abcd"),
			).toThrow("GUACAMOLE_JSON_SECRET_KEY must be 32 hex chars");
		});

		it("returns a base64 string for valid key and json", () => {
			const result = signAndEncrypt({ foo: "bar" }, VALID_KEY_HEX);
			expect(typeof result).toBe("string");
			expect(result).toMatch(/^[A-Za-z0-9+/]+=*$/);
			expect(Buffer.from(result, "base64").length).toBeGreaterThan(0);
		});

		it("produces different ciphertext for different plaintext", () => {
			const a = signAndEncrypt({ a: 1 }, VALID_KEY_HEX);
			const b = signAndEncrypt({ a: 2 }, VALID_KEY_HEX);
			expect(a).not.toBe(b);
		});

		it("produces different ciphertext for different keys", () => {
			const key2 = "fedcba9876543210fedcba9876543210";
			const a = signAndEncrypt({ same: true }, VALID_KEY_HEX);
			const b = signAndEncrypt({ same: true }, key2);
			expect(a).not.toBe(b);
		});
	});

	describe("createRdpAuthJson", () => {
		it("returns object with username, expires, connections", () => {
			const out = createRdpAuthJson({
				hostname: "win.example.com",
				username: "admin",
				password: "secret",
			});
			expect(out).toHaveProperty("username", "patchmon-rdp");
			expect(out).toHaveProperty("expires");
			expect(typeof out.expires).toBe("number");
			expect(out).toHaveProperty("connections");
			expect(typeof out.connections).toBe("object");
		});

		it("uses RDP_CONNECTION_NAME as the single connection key", () => {
			const out = createRdpAuthJson({
				hostname: "h",
				username: "u",
				password: "p",
			});
			expect(out.connections).toHaveProperty(RDP_CONNECTION_NAME);
			expect(Object.keys(out.connections)).toHaveLength(1);
		});

		it("connection has protocol rdp and parameters with security any", () => {
			const out = createRdpAuthJson({
				hostname: "win.local",
				username: "u",
				password: "p",
			});
			const conn = out.connections[RDP_CONNECTION_NAME];
			expect(conn).toHaveProperty("protocol", "rdp");
			expect(conn).toHaveProperty("id", RDP_CONNECTION_NAME);
			expect(conn.parameters).toHaveProperty("security", "any");
		});

		it("parameters include hostname, port, username, password", () => {
			const out = createRdpAuthJson({
				hostname: "192.168.1.10",
				username: "Administrator",
				password: "P@ss",
				port: 3389,
			});
			const p = out.connections[RDP_CONNECTION_NAME].parameters;
			expect(p.hostname).toBe("192.168.1.10");
			expect(p.port).toBe("3389");
			expect(p.username).toBe("Administrator");
			expect(p.password).toBe("P@ss");
		});

		it("includes domain in parameters when provided", () => {
			const out = createRdpAuthJson({
				hostname: "dc.local",
				username: "u",
				password: "p",
				domain: "CORP",
			});
			expect(out.connections[RDP_CONNECTION_NAME].parameters.domain).toBe(
				"CORP",
			);
		});

		it("omits domain from parameters when not provided", () => {
			const out = createRdpAuthJson({
				hostname: "h",
				username: "u",
				password: "p",
			});
			expect(
				out.connections[RDP_CONNECTION_NAME].parameters.domain,
			).toBeUndefined();
		});

		it("expires is in the future when called with default", () => {
			const before = Date.now();
			const out = createRdpAuthJson({
				hostname: "h",
				username: "u",
				password: "p",
			});
			const after = Date.now();
			expect(out.expires).toBeGreaterThanOrEqual(before);
			expect(out.expires).toBeLessThanOrEqual(after + 6 * 60 * 1000);
		});
	});

	describe("constants", () => {
		it("JSON_DATASOURCE is json", () => {
			expect(JSON_DATASOURCE).toBe("json");
		});
		it("RDP_CONNECTION_NAME is PatchMon-RDP", () => {
			expect(RDP_CONNECTION_NAME).toBe("PatchMon-RDP");
		});
	});
});
