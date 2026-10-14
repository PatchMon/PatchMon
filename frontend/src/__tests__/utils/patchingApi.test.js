import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../utils/api", () => ({
	default: { get: vi.fn() },
}));

import api from "../../utils/api";
import { pollDryRunUntilDone } from "../../utils/patchingApi";

const respondWith = (statusAt) => {
	api.get.mockImplementation(async () => ({ data: statusAt(Date.now()) }));
};

const settle = async (promise, ms) => {
	let result;
	promise.then((r) => {
		result = r;
	});
	await vi.advanceTimersByTimeAsync(ms);
	return result;
};

describe("pollDryRunUntilDone", () => {
	let t0;
	beforeEach(() => {
		vi.useFakeTimers();
		t0 = Date.now();
		api.get.mockReset();
	});
	afterEach(() => {
		vi.useRealTimers();
	});

	it("keeps waiting while the agent is running a long validation", async () => {
		respondWith((now) =>
			now - t0 < 3 * 60_000
				? { status: "running" }
				: { status: "validated", packages_affected: ["KB5001"] },
		);
		const result = await settle(pollDryRunUntilDone("r1"), 4 * 60_000);
		expect(result?.status).toBe("validated");
		expect(result.packages_affected).toEqual(["KB5001"]);
	});

	it("gives up as still running, not offline, after the safety cap", async () => {
		respondWith(() => ({ status: "running" }));
		const result = await settle(pollDryRunUntilDone("r5"), 26 * 60_000);
		expect(result?.status).toBe("still_running");
		expect(result.error).toMatch(/Runs & History/);
	});

	it("reports offline when the run is never dispatched", async () => {
		respondWith(() => ({ status: "pending_validation" }));
		const result = await settle(pollDryRunUntilDone("r2"), 60_000);
		expect(result?.status).toBe("timeout");
	});

	it.each([
		"timed_out",
		"agent_disconnected",
		"cancelled",
	])("stops on terminal status %s", async (status) => {
		respondWith((now) =>
			now - t0 < 60_000
				? { status: "running" }
				: { status, error_message: `run ${status}` },
		);
		const result = await settle(pollDryRunUntilDone("r3"), 70_000);
		expect(result?.status).toBe("failed");
		expect(result.error).toBe(`run ${status}`);
	});

	it("stops polling once the signal aborts", async () => {
		respondWith(() => ({ status: "running" }));
		const ctl = new AbortController();
		const promise = pollDryRunUntilDone("r4", ctl.signal);
		await vi.advanceTimersByTimeAsync(10_000);
		const calls = api.get.mock.calls.length;
		ctl.abort();
		const result = await settle(promise, 10_000);
		expect(result).toBeNull();
		expect(api.get.mock.calls.length).toBe(calls);
	});
});
