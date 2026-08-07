import { describe, expect, it, vi } from "vitest";

vi.mock("guacamole-common-js", () => ({
	default: {},
}));

import {
	classifyGuacUpstreamFailure,
	deriveCodeFromGuacStatus,
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

	it("maps status 519 to port unreachable guidance", () => {
		expect(
			deriveCodeFromGuacStatus({
				code: 0x0207,
				message: "upstream host not found",
			}),
		).toBe("rdp_port_unreachable");
	});
});
