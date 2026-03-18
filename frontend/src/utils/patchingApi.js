import api from "./api";

const DRY_RUN_POLL_INTERVAL_MS = 2000;
const DRY_RUN_TIMEOUT_MS = 30000;

/**
 * Poll a patch run until it reaches a terminal state (validated, completed, failed) or timeout.
 * Returns { status, packages_affected, error }.
 */
export async function pollDryRunUntilDone(runId) {
	const start = Date.now();
	while (Date.now() - start < DRY_RUN_TIMEOUT_MS) {
		const run = await api.get(`/patching/runs/${runId}`).then((r) => r.data);
		const status = run?.status;
		if (status === "validated" || status === "completed") {
			return {
				status: "validated",
				packages_affected: run.packages_affected || [],
				shell_output: run.shell_output || "",
				error: null,
			};
		}
		if (status === "failed") {
			return {
				status: "failed",
				packages_affected: [],
				shell_output: run.shell_output || "",
				error: run.error_message || "Dry run failed",
			};
		}
		await new Promise((r) => setTimeout(r, DRY_RUN_POLL_INTERVAL_MS));
	}
	return {
		status: "timeout",
		packages_affected: [],
		shell_output: "",
		error: "Validation skipped (host offline)",
	};
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
				...(opts.schedule_override
					? { schedule_override: opts.schedule_override }
					: {}),
			})
			.then((res) => res.data),
	approveRun: (id) =>
		api.post(`/patching/runs/${id}/approve`).then((res) => res.data),
	retryValidation: (id) =>
		api.post(`/patching/runs/${id}/retry-validation`).then((res) => res.data),
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
