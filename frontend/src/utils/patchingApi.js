import api from "./api";
import { skippedItemsSummary } from "./patchRun";

const DRY_RUN_POLL_INTERVAL_MS = 2000;
const DRY_RUN_TIMEOUT_MS = 30000;
// Covers the agent's Windows Update search (15 min) and winget list (5 min) limits.
const DRY_RUN_RUNNING_CAP_MS = 25 * 60 * 1000;

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

const DRY_RUN_FAILED_STATUSES = [
	"failed",
	"cancelled",
	"timed_out",
	"agent_disconnected",
];

/**
 * Poll a patch run until it reaches a terminal state or timeout.
 * The 30s offline timeout only applies until the agent picks the run up: a
 * Windows Update search can take minutes.
 * Returns { status, packages_affected, error }, or null once signal aborts.
 */
export async function pollDryRunUntilDone(runId, signal) {
	const start = Date.now();
	for (;;) {
		if (signal?.aborted) return null;
		const run = await api.get(`/patching/runs/${runId}`).then((r) => r.data);
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
		const elapsed = Date.now() - start;
		if (status === "running" && elapsed >= DRY_RUN_RUNNING_CAP_MS) {
			return {
				status: "still_running",
				packages_affected: [],
				shell_output: run.shell_output || "",
				error:
					"Validation is still running on the host. Check Runs & History for the result.",
			};
		}
		if (status !== "running" && elapsed >= DRY_RUN_TIMEOUT_MS) {
			return {
				status: "timeout",
				packages_affected: [],
				shell_output: "",
				error: "Validation skipped (host offline)",
			};
		}
		await new Promise((r) => setTimeout(r, DRY_RUN_POLL_INTERVAL_MS));
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
