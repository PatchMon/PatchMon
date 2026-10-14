import { describe, expect, it, vi } from "vitest";

vi.mock("guacamole-common-js", () => ({
	default: {},
}));

import {
	classifyGuacUpstreamFailure,
	deriveCodeFromGuacStatus,
	deriveCodeFromHttpError,
	guacErrorFromStatus,
} from "../../components/RdpViewer";

describe("RdpViewer error classification", () => {
	it("classifies security negotiation failures from upstream messages", () => {
		expect(
			classifyGuacUpstreamFailure(
				"Server refused connection (wrong security type?)",
			),
		).toBe("rdp_security_negotiation_failed");
	});

	it("classifies authentication failures from upstream messages", () => {
		expect(classifyGuacUpstreamFailure("Authentication failed for user")).toBe(
			"rdp_auth_failed",
		);
	});

	it("classifies NTSTATUS auth failures from upstream messages", () => {
		expect(
			classifyGuacUpstreamFailure(
				"ERRCONNECT_LOGON_FAILURE [0x00020014] STATUS_LOGON_FAILURE",
			),
		).toBe("rdp_auth_failed");
	});

	it("keeps ambiguous upstream failures generic", () => {
		expect(
			classifyGuacUpstreamFailure(
				"Upstream connection closed during desktop session setup",
			),
		).toBe("rdp_gateway_failed");
	});

	it("maps status 520 to a security-specific guidance code when possible", () => {
		expect(
			deriveCodeFromGuacStatus({
				code: 0x0208,
				message: "Server refused connection (wrong security type?)",
			}),
		).toBe("rdp_security_negotiation_failed");
	});

	it("maps status 520 to auth guidance when the upstream message is explicit", () => {
		expect(
			deriveCodeFromGuacStatus({
				code: 0x0208,
				message: "Authentication failed for user",
			}),
		).toBe("rdp_auth_failed");
	});

	it("maps status 519 to upstream-unreachable guidance, not port 3389", () => {
		expect(
			deriveCodeFromGuacStatus({
				code: 0x0207,
				message: "Server refused connection (wrong security type?)",
			}),
		).toBe("rdp_upstream_unreachable");
	});

	it("maps the ticket rate limit and per-instance session cap to their own guidance", () => {
		expect(
			deriveCodeFromHttpError({
				response: { status: 429, data: { message: "Too many requests." } },
			}),
		).toBe("rate_limited");
		expect(
			deriveCodeFromHttpError({
				response: { status: 503, data: { code: "max_sessions_context" } },
			}),
		).toBe("max_sessions_context");
		expect(
			deriveCodeFromHttpError({
				response: {
					status: 400,
					data: {
						error: "Unsupported keyboard layout",
						code: "invalid_keyboard_layout",
					},
				},
			}),
		).toBe("invalid_keyboard_layout");
	});

	it.each([
		[
			0x0203,
			"agent_disconnected: The PatchMon agent on this host disconnected. Connect again once it is back online.",
			"agent_disconnected",
			"The PatchMon agent on this host disconnected. Connect again once it is back online.",
		],
		[
			0x0203,
			"rdp_session_ended: The RDP session ended before it could start. Connect again.",
			"rdp_session_ended",
			"The RDP session ended before it could start. Connect again.",
		],
		[
			0x0204,
			"rdp_ticket_invalid: This RDP ticket is invalid, expired or already used. Connect again to get a new one.",
			"rdp_ticket_invalid",
			"This RDP ticket is invalid, expired or already used. Connect again to get a new one.",
		],
		[
			0x0200,
			"server_error: The RDP session could not be started on the PatchMon server. Check the server logs.",
			"server_error",
			"The RDP session could not be started on the PatchMon server. Check the server logs.",
		],
	])("uses the server's code prefix for status %i", (code, message, expectedCode, expectedText) => {
		expect(guacErrorFromStatus({ code, message }, "fallback")).toEqual({
			code: expectedCode,
			message: expectedText,
		});
	});

	it("falls back to the status code for unprefixed or unknown-prefix messages", () => {
		expect(
			guacErrorFromStatus(
				{ code: 0x0207, message: "Server refused connection" },
				"fallback",
			),
		).toEqual({
			code: "rdp_upstream_unreachable",
			message: "Server refused connection",
		});
		expect(
			guacErrorFromStatus(
				{ code: 0x0203, message: "not_a_key: something" },
				"fallback",
			),
		).toEqual({ code: "rdp_unknown", message: "not_a_key: something" });
		expect(
			guacErrorFromStatus({ code: 0x0207 }, "Tunnel error (code 519)"),
		).toEqual({
			code: "rdp_upstream_unreachable",
			message: "Tunnel error (code 519)",
		});
	});
});
