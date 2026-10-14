import api from "./api";
import { skippedItemsSummary } from "./patchRun";

const DRY_RUN_POLL_INTERVAL_MS = 2000;
const DRY_RUN_MAX_POLL_INTERVAL_MS = 10000;
const DRY_RUN_TIMEOUT_MS = 30000;
// Covers the agent's Windows Update search (15 min) and winget list (5 min) limits.
const DRY_RUN_RUNNING_CAP_MS = 25 * 60 * 1000;
const DRY_RUN_MAX_FETCH_ERRORS = 3;

export const STILL_RUNNING_MESSAGE =
	"Validation is still running on the host. Check Runs & History for the result.";

/**
 * Build the absolute WebSocket URL for the live patch-run output stream.
 * The browser auto-attaches the auth cookie on same-origin upgrades, so no
 * ticket is required — the backend uses the shared JWT middleware.
 */
export function buildRunStreamURL(runId) {
	const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
	const host = window.location.host;
	return `${protocol}//${host}/api/v1/patching/runs/${encodeURIComponent(
		runId,
	)}/stream`;
}

const DRY_RUN_FAILED_STATUSES = ["failed", "cancelled", "timed_out"];
// The server lets a reconnecting agent move agent_disconnected back to running.
const DRY_RUN_ACTIVE_STATUSES = ["running", "agent_disconnected"];

const sleep = (ms, signal) =>
	new Promise((resolve) => {
		const timer = setTimeout(done, ms);
		function done() {
			clearTimeout(timer);
			signal?.removeEventListener("abort", done);
			resolve();
		}
		signal?.addEventListener("abort", done);
	});

/**
 * Poll a patch run until it reaches a terminal state or timeout.
 * The 30s offline timeout only applies until the server has dispatched the run
 * to the agent: a Windows Update search can take minutes.
 * onStatus receives each raw run status while the run is still in progress.
 * Returns { status, packages_affected, error }, or null once signal aborts.
 */
export async function pollDryRunUntilDone(runId, { signal, onStatus } = {}) {
	const start = Date.now();
	let interval = DRY_RUN_POLL_INTERVAL_MS;
	let fetchErrors = 0;
	for (;;) {
		if (signal?.aborted) return null;
		let run;
		try {
			run = await api.get(`/patching/runs/${runId}`).then((r) => r.data);
			fetchErrors = 0;
		} catch (err) {
			if (signal?.aborted) return null;
			fetchErrors += 1;
			if (fetchErrors >= DRY_RUN_MAX_FETCH_ERRORS) throw err;
			await sleep(DRY_RUN_POLL_INTERVAL_MS * 2 ** fetchErrors, signal);
			continue;
		}
		if (signal?.aborted) return null;
		const status = run?.status;
		if (
			status === "validated" ||
			status === "completed" ||
			status === "dry_run_completed"
		) {
			return {
				status: "validated",
				packages_affected: run.packages_affected || [],
				shell_output: run.shell_output || "",
				skip_note: skippedItemsSummary(run),
				error: null,
			};
		}
		if (DRY_RUN_FAILED_STATUSES.includes(status)) {
			return {
				status: "failed",
				packages_affected: [],
				shell_output: run.shell_output || "",
				error: run.error_message || "Dry run failed",
			};
		}
		const active = DRY_RUN_ACTIVE_STATUSES.includes(status);
		const elapsed = Date.now() - start;
		if (active && elapsed >= DRY_RUN_RUNNING_CAP_MS) {
			if (status === "agent_disconnected") {
				return {
					status: "failed",
					packages_affected: [],
					shell_output: run.shell_output || "",
					error: run.error_message || "Agent disconnected during validation",
				};
			}
			return {
				status: "still_running",
				packages_affected: [],
				shell_output: run.shell_output || "",
				error: STILL_RUNNING_MESSAGE,
			};
		}
		if (!active && elapsed >= DRY_RUN_TIMEOUT_MS) {
			return {
				status: "timeout",
				packages_affected: [],
				shell_output: "",
				error: "Validation skipped (host offline)",
			};
		}
		onStatus?.(status);
		if (active) {
			interval = Math.min(
				interval + DRY_RUN_POLL_INTERVAL_MS,
				DRY_RUN_MAX_POLL_INTERVAL_MS,
			);
		}
		await sleep(interval, signal);
	}
}

export const patchingAPI = {
	getDashboard: () => api.get("/patching/dashboard").then((res) => res.data),
	getRuns: (params = {}) =>
		api.get("/patching/runs", { params }).then((res) => res.data),
	getActiveRuns: () => api.get("/patching/runs/active").then((res) => res.data),
	getRunById: (id) => api.get(`/patching/runs/${id}`).then((res) => res.data),
	trigger: (
		host_id,
		patch_type,
		package_name = null,
		package_names = null,
		opts = {},
	) =>
		api
			.post("/patching/trigger", {
				host_id,
				patch_type,
				...(package_name ? { package_name } : {}),
				...(Array.isArray(package_names) && package_names.length > 0
					? { package_names }
					: {}),
				...(opts.dry_run ? { dry_run: true } : {}),
				...(opts.pending_approval ? { pending_approval: true } : {}),
				...(opts.schedule_override
					? { schedule_override: opts.schedule_override }
					: {}),
				...(opts.windows_scope
					? {
							windows_scope: {
								updates: !!opts.windows_scope.updates,
								apps: !!opts.windows_scope.apps,
							},
						}
					: {}),
			})
			.then((res) => res.data),
	approveRun: (id, opts = {}) =>
		api
			.post(`/patching/runs/${id}/approve`, {
				...(opts.schedule_override
					? { schedule_override: opts.schedule_override }
					: {}),
			})
			.then((res) => res.data),
	retryValidation: (id) =>
		api.post(`/patching/runs/${id}/retry-validation`).then((res) => res.data),
	stopRun: (id) =>
		api.post(`/patching/runs/${id}/stop`).then((res) => res.data),
	deleteRun: (id) => api.delete(`/patching/runs/${id}`),
	getPreviewRun: (host_id) =>
		api
			.get("/patching/preview-run", { params: { host_id } })
			.then((res) => res.data),

	// Policies
	getPolicies: () => api.get("/patching/policies").then((res) => res.data),
	getPolicyById: (id) =>
		api.get(`/patching/policies/${id}`).then((res) => res.data),
	createPolicy: (data) =>
		api.post("/patching/policies", data).then((res) => res.data),
	updatePolicy: (id, data) =>
		api.put(`/patching/policies/${id}`, data).then((res) => res.data),
	deletePolicy: (id) => api.delete(`/patching/policies/${id}`),
	getPolicyAssignments: (id) =>
		api.get(`/patching/policies/${id}/assignments`).then((res) => res.data),
	addPolicyAssignment: (id, target_type, target_id) =>
		api
			.post(`/patching/policies/${id}/assignments`, {
				target_type,
				target_id,
			})
			.then((res) => res.data),
	removePolicyAssignment: (id, assignmentId) =>
		api.delete(`/patching/policies/${id}/assignments/${assignmentId}`),
	addPolicyExclusion: (id, host_id) =>
		api
			.post(`/patching/policies/${id}/exclusions`, { host_id })
			.then((res) => res.data),
	removePolicyExclusion: (id, hostId) =>
		api.delete(`/patching/policies/${id}/exclusions/${hostId}`),
};
