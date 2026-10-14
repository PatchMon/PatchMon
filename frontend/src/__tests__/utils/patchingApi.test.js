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
	let error;
	promise.then(
		(r) => {
			result = r;
		},
		(e) => {
			error = e;
		},
	);
	await vi.advanceTimersByTimeAsync(ms);
	if (error) throw error;
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

	it("reports offline when the run is never dispatched", async () => {
		respondWith(() => ({ status: "pending_validation" }));
		const result = await settle(pollDryRunUntilDone("r2"), 60_000);
		expect(result?.status).toBe("timeout");
	});

	it("gives up as still running, not offline, after the safety cap", async () => {
		respondWith(() => ({ status: "running" }));
		const result = await settle(pollDryRunUntilDone("r5"), 26 * 60_000);
		expect(result?.status).toBe("still_running");
		expect(result.error).toMatch(/Runs & History/);
	});

	it.each([
		"timed_out",
		"cancelled",
		"failed",
	])("stops on terminal status %s", async (status) => {
		respondWith((now) =>
			now - t0 < 60_000
				? { status: "running" }
				: { status, error_message: `run ${status}` },
		);
		const result = await settle(pollDryRunUntilDone("r3"), 80_000);
		expect(result?.status).toBe("failed");
		expect(result.error).toBe(`run ${status}`);
	});

	it("waits through an agent reconnect and reports the real result", async () => {
		respondWith((now) => {
			const t = now - t0;
			if (t < 20_000) return { status: "running" };
			if (t < 90_000)
				return {
					status: "agent_disconnected",
					error_message: "Agent disconnected during patch run",
				};
			return { status: "validated", packages_affected: ["KB5002"] };
		});
		const seen = [];
		const result = await settle(
			pollDryRunUntilDone("r6", { onStatus: (s) => seen.push(s) }),
			3 * 60_000,
		);
		expect(result?.status).toBe("validated");
		expect(seen).toContain("agent_disconnected");
	});

	it("fails a run whose agent never comes back once the cap is reached", async () => {
		respondWith(() => ({
			status: "agent_disconnected",
			error_message: "Agent disconnected during patch run",
		}));
		const result = await settle(pollDryRunUntilDone("r7"), 26 * 60_000);
		expect(result?.status).toBe("failed");
		expect(result.error).toBe("Agent disconnected during patch run");
	});

	it("rides out a couple of failed requests", async () => {
		let calls = 0;
		api.get.mockImplementation(async () => {
			calls += 1;
			if (calls <= 2) throw new Error("network");
			return { data: { status: "validated", packages_affected: [] } };
		});
		const result = await settle(pollDryRunUntilDone("r8"), 30_000);
		expect(result?.status).toBe("validated");
	});

	it("fails after three requests in a row fail", async () => {
		api.get.mockRejectedValue(new Error("network"));
		await expect(settle(pollDryRunUntilDone("r9"), 60_000)).rejects.toThrow(
			"network",
		);
		expect(api.get).toHaveBeenCalledTimes(3);
	});

	it("backs off to 10s between requests once the agent is running", async () => {
		respondWith(() => ({ status: "running" }));
		const ctl = new AbortController();
		pollDryRunUntilDone("r10", { signal: ctl.signal });
		await vi.advanceTimersByTimeAsync(60_000);
		const firstMinute = api.get.mock.calls.length;
		await vi.advanceTimersByTimeAsync(60_000);
		const secondMinute = api.get.mock.calls.length - firstMinute;
		ctl.abort();
		await vi.advanceTimersByTimeAsync(0);
		expect(firstMinute).toBeLessThan(15);
		expect(secondMinute).toBe(6);
	});

	it("polls every 2s before the run is dispatched", async () => {
		respondWith(() => ({ status: "pending_validation" }));
		pollDryRunUntilDone("r11");
		await vi.advanceTimersByTimeAsync(10_000);
		expect(api.get.mock.calls.length).toBe(6);
	});

	it("stops polling once the signal aborts", async () => {
		respondWith(() => ({ status: "running" }));
		const ctl = new AbortController();
		const promise = pollDryRunUntilDone("r4", { signal: ctl.signal });
		await vi.advanceTimersByTimeAsync(10_000);
		const calls = api.get.mock.calls.length;
		ctl.abort();
		const result = await settle(promise, 10_000);
		expect(result).toBeNull();
		expect(api.get.mock.calls.length).toBe(calls);
	});
});
