import { afterEach, describe, expect, it, vi } from "vitest";
import api, { rdpAPI } from "../../utils/api";

const SERVER_AGENT_HANDSHAKE_MS = 12000;

describe("rdpAPI.createTicket", () => {
	afterEach(() => vi.restoreAllMocks());

	it("waits longer than the server's agent handshake budget", () => {
		const post = vi.spyOn(api, "post").mockResolvedValue({ data: {} });
		rdpAPI.createTicket({ hostId: "h1" });
		const config = post.mock.calls[0][2];
		expect(config?.timeout).toBeGreaterThanOrEqual(
			SERVER_AGENT_HANDSHAKE_MS + 5000,
		);
	});
});
