import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
	Clock,
	Edit,
	Plus,
	Server,
	Settings,
	Trash2,
	Users,
	X,
} from "lucide-react";
import { useState } from "react";
import { useSettings } from "../../contexts/SettingsContext";
import { useToast } from "../../contexts/ToastContext";
import { adminHostsAPI, hostGroupsAPI } from "../../utils/api";
import { patchingAPI } from "../../utils/patchingApi";

const delay_type_labels = {
	immediate: "Immediate",
	delayed: "Delayed (e.g. 1 hour)",
	fixed_time: "Fixed time (e.g. 3:00 AM)",
};

const PatchManagement = () => {
	const queryClient = useQueryClient();
	const toast = useToast();
	const { settings } = useSettings();
	const orgTimezone = settings?.timezone || "UTC";
	const [showModal, setShowModal] = useState(false);
	const [editingPolicy, setEditingPolicy] = useState(null);
	const [form, setForm] = useState({
		name: "",
		description: "",
		patch_delay_type: "immediate",
		delay_minutes: 60,
		fixed_time_utc: "03:00",
	});
	const [expandedPolicyId, setExpandedPolicyId] = useState(null);

	const { data: policies = [], isLoading } = useQuery({
		queryKey: ["patching-policies"],
		queryFn: () => patchingAPI.getPolicies(),
	});

	const { data: hostGroups = [] } = useQuery({
		queryKey: ["hostGroups"],
		queryFn: () => hostGroupsAPI.list().then((res) => res.data),
	});

	const { data: hostsData } = useQuery({
		queryKey: ["hosts-list"],
		queryFn: () => adminHostsAPI.list().then((res) => res.data),
	});
	const hosts = hostsData?.data || [];

	const createMutation = useMutation({
		mutationFn: (data) => patchingAPI.createPolicy(data),
		onSuccess: () => {
			queryClient.invalidateQueries(["patching-policies"]);
			setShowModal(false);
			setForm({
				name: "",
				description: "",
				patch_delay_type: "immediate",
				delay_minutes: 60,
				fixed_time_utc: "03:00",
			});
			toast.success("Policy created");
		},
		onError: (err) => toast.error(err.response?.data?.error || err.message),
	});

	const updateMutation = useMutation({
		mutationFn: ({ id, data }) => patchingAPI.updatePolicy(id, data),
		onSuccess: () => {
			queryClient.invalidateQueries(["patching-policies"]);
			setShowModal(false);
			setEditingPolicy(null);
			toast.success("Policy updated");
		},
		onError: (err) => toast.error(err.response?.data?.error || err.message),
	});

	const deleteMutation = useMutation({
		mutationFn: (id) => patchingAPI.deletePolicy(id),
		onSuccess: () => {
			queryClient.invalidateQueries(["patching-policies"]);
			toast.success("Policy deleted");
		},
		onError: (err) => toast.error(err.response?.data?.error || err.message),
	});

	const openCreate = () => {
		setEditingPolicy(null);
		setForm({
			name: "",
			description: "",
			patch_delay_type: "immediate",
			delay_minutes: 60,
			fixed_time_utc: "03:00",
		});
		setShowModal(true);
	};

	const openEdit = (policy) => {
		setEditingPolicy(policy);
		setForm({
			name: policy.name,
			description: policy.description || "",
			patch_delay_type: policy.patch_delay_type || "immediate",
			delay_minutes: policy.delay_minutes ?? 60,
			fixed_time_utc: policy.fixed_time_utc || "03:00",
		});
		setShowModal(true);
	};

	const handleSubmit = (e) => {
		e.preventDefault();
		const payload = {
			name: form.name.trim(),
			description: form.description.trim() || null,
			patch_delay_type: form.patch_delay_type,
			delay_minutes:
				form.patch_delay_type === "delayed" ? Number(form.delay_minutes) : null,
			fixed_time_utc:
				form.patch_delay_type === "fixed_time" ? form.fixed_time_utc : null,
		};
		if (editingPolicy) {
			updateMutation.mutate({ id: editingPolicy.id, data: payload });
		} else {
			createMutation.mutate(payload);
		}
	};

	return (
		<div className="space-y-6">
			<div className="flex justify-between items-center">
				<div>
					<h1 className="text-2xl font-bold text-secondary-900 dark:text-white">
						Patch Management
					</h1>
					<p className="mt-1 text-sm text-secondary-600 dark:text-secondary-400">
						Patch policies control when patches run: immediate, delayed, or at a
						fixed time. Assign policies to hosts or host groups; use exclusions
						to exclude specific hosts from a group policy.
					</p>
				</div>
				<button
					type="button"
					onClick={openCreate}
					className="btn-primary flex items-center gap-2"
				>
					<Plus className="h-4 w-4" />
					Create policy
				</button>
			</div>

			<div className="bg-white dark:bg-secondary-800 border border-secondary-200 dark:border-secondary-600 rounded-lg overflow-hidden">
				<div className="px-4 py-3 border-b border-secondary-200 dark:border-secondary-600 flex items-center gap-2">
					<Settings className="h-5 w-5 text-primary-600" />
					<h3 className="text-lg font-medium text-secondary-900 dark:text-white">
						Patch policies
					</h3>
				</div>
				{isLoading ? (
					<div className="p-8 text-center text-secondary-500">Loading…</div>
				) : policies.length === 0 ? (
					<div className="p-8 text-center text-secondary-500">
						No policies yet. Create one to control when patches run (immediate,
						delayed, or at a fixed time).
					</div>
				) : (
					<ul className="divide-y divide-secondary-200 dark:divide-secondary-600">
						{policies.map((policy) => (
							<li key={policy.id}>
								<div className="px-4 py-3 flex items-center justify-between gap-4">
									<div className="min-w-0">
										<p className="font-medium text-secondary-900 dark:text-white">
											{policy.name}
										</p>
										<p className="text-sm text-secondary-500 dark:text-secondary-400">
											{delay_type_labels[policy.patch_delay_type] ||
												policy.patch_delay_type}
											{policy.patch_delay_type === "delayed" &&
												policy.delay_minutes != null &&
												` (${policy.delay_minutes} min)`}
											{policy.patch_delay_type === "fixed_time" &&
												policy.fixed_time_utc &&
												` at ${policy.fixed_time_utc} (${orgTimezone})`}
										</p>
									</div>
									<div className="flex items-center gap-2 flex-shrink-0">
										<button
											type="button"
											onClick={() =>
												setExpandedPolicyId(
													expandedPolicyId === policy.id ? null : policy.id,
												)
											}
											className="text-sm text-primary-600 dark:text-primary-400 hover:underline"
										>
											{expandedPolicyId === policy.id ? "Hide" : "Assignments"}
										</button>
										<button
											type="button"
											onClick={() => openEdit(policy)}
											className="p-1.5 rounded hover:bg-secondary-100 dark:hover:bg-secondary-700 text-secondary-600 dark:text-secondary-300"
											title="Edit"
										>
											<Edit className="h-4 w-4" />
										</button>
										<button
											type="button"
											onClick={() => {
												if (window.confirm(`Delete policy "${policy.name}"?`))
													deleteMutation.mutate(policy.id);
											}}
											className="p-1.5 rounded hover:bg-red-100 dark:hover:bg-red-900/30 text-red-600"
											title="Delete"
										>
											<Trash2 className="h-4 w-4" />
										</button>
									</div>
								</div>
								{expandedPolicyId === policy.id && (
									<PolicyAssignments
										policy={policy}
										hosts={hosts}
										hostGroups={hostGroups}
										onUpdate={() =>
											queryClient.invalidateQueries(["patching-policies"])
										}
									/>
								)}
							</li>
						))}
					</ul>
				)}
			</div>

			{/* Create/Edit Modal */}
			{showModal && (
				<div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50">
					<div className="bg-white dark:bg-secondary-800 rounded-lg shadow-xl max-w-md w-full max-h-[90vh] overflow-y-auto">
						<div className="flex items-center justify-between p-4 border-b border-secondary-200 dark:border-secondary-600">
							<h3 className="text-lg font-semibold text-secondary-900 dark:text-white">
								{editingPolicy ? "Edit policy" : "Create policy"}
							</h3>
							<button
								type="button"
								onClick={() => setShowModal(false)}
								className="p-1 rounded hover:bg-secondary-100 dark:hover:bg-secondary-700"
							>
								<X className="h-5 w-5" />
							</button>
						</div>
						<form onSubmit={handleSubmit} className="p-4 space-y-4">
							<div>
								<label className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-1">
									Name
								</label>
								<input
									type="text"
									value={form.name}
									onChange={(e) =>
										setForm((f) => ({ ...f, name: e.target.value }))
									}
									className="w-full rounded border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white px-3 py-2"
									required
								/>
							</div>
							<div>
								<label className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-1">
									Description (optional)
								</label>
								<input
									type="text"
									value={form.description}
									onChange={(e) =>
										setForm((f) => ({ ...f, description: e.target.value }))
									}
									className="w-full rounded border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white px-3 py-2"
								/>
							</div>
							<div>
								<label className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-1">
									Patch delay
								</label>
								<select
									value={form.patch_delay_type}
									onChange={(e) =>
										setForm((f) => ({
											...f,
											patch_delay_type: e.target.value,
										}))
									}
									className="w-full rounded border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white px-3 py-2"
								>
									<option value="immediate">Immediate</option>
									<option value="delayed">Delayed (run after N minutes)</option>
									<option value="fixed_time">Fixed time (e.g. 3:00 AM)</option>
								</select>
							</div>
							{form.patch_delay_type === "delayed" && (
								<div>
									<label className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-1">
										Delay (minutes)
									</label>
									<input
										type="number"
										min={1}
										value={form.delay_minutes}
										onChange={(e) =>
											setForm((f) => ({
												...f,
												delay_minutes: e.target.value,
											}))
										}
										className="w-full rounded border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white px-3 py-2"
									/>
								</div>
							)}
							{form.patch_delay_type === "fixed_time" && (
								<div>
									<label className="block text-sm font-medium text-secondary-700 dark:text-secondary-300 mb-1">
										Time (HH:MM)
									</label>
									<input
										type="text"
										placeholder="03:00"
										value={form.fixed_time_utc}
										onChange={(e) =>
											setForm((f) => ({
												...f,
												fixed_time_utc: e.target.value,
											}))
										}
										className="w-full rounded border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white px-3 py-2"
									/>
									<p className="mt-1 text-xs text-secondary-500 dark:text-secondary-400">
										Local time in the organization timezone (
										<span className="font-medium">{orgTimezone}</span>). Change
										it under Settings &rarr; General.
									</p>
								</div>
							)}
							<div className="flex justify-end gap-2 pt-2">
								<button
									type="button"
									onClick={() => setShowModal(false)}
									className="btn-outline"
								>
									Cancel
								</button>
								<button
									type="submit"
									className="btn-primary"
									disabled={
										createMutation.isPending || updateMutation.isPending
									}
								>
									{editingPolicy ? "Update" : "Create"}
								</button>
							</div>
						</form>
					</div>
				</div>
			)}
		</div>
	);
};

function PolicyAssignments({ policy, hosts, hostGroups, onUpdate }) {
	const queryClient = useQueryClient();
	const toast = useToast();
	const [addTargetType, setAddTargetType] = useState("host");
	const [addTargetId, setAddTargetId] = useState("");
	const [addExclusionHostId, setAddExclusionHostId] = useState("");

	const { data: fullPolicy } = useQuery({
		queryKey: ["patching-policy", policy.id],
		queryFn: () => patchingAPI.getPolicyById(policy.id),
		enabled: !!policy.id,
	});

	const assignments = fullPolicy?.assignments || [];
	const exclusions = fullPolicy?.exclusions || [];

	const addAssignmentMutation = useMutation({
		mutationFn: () =>
			patchingAPI.addPolicyAssignment(policy.id, addTargetType, addTargetId),
		onSuccess: () => {
			queryClient.invalidateQueries(["patching-policies"]);
			queryClient.invalidateQueries(["patching-policy", policy.id]);
			setAddTargetId("");
			onUpdate?.();
			toast.success("Assignment added");
		},
		onError: (err) => toast.error(err.response?.data?.error || err.message),
	});

	const removeAssignmentMutation = useMutation({
		mutationFn: (assignmentId) =>
			patchingAPI.removePolicyAssignment(policy.id, assignmentId),
		onSuccess: () => {
			queryClient.invalidateQueries(["patching-policies"]);
			queryClient.invalidateQueries(["patching-policy", policy.id]);
			onUpdate?.();
			toast.success("Assignment removed");
		},
		onError: (err) => toast.error(err.response?.data?.error || err.message),
	});

	const addExclusionMutation = useMutation({
		mutationFn: () =>
			patchingAPI.addPolicyExclusion(policy.id, addExclusionHostId),
		onSuccess: () => {
			queryClient.invalidateQueries(["patching-policy", policy.id]);
			setAddExclusionHostId("");
			onUpdate?.();
			toast.success("Exclusion added");
		},
		onError: (err) => toast.error(err.response?.data?.error || err.message),
	});

	const removeExclusionMutation = useMutation({
		mutationFn: (hostId) =>
			patchingAPI.removePolicyExclusion(policy.id, hostId),
		onSuccess: () => {
			queryClient.invalidateQueries(["patching-policy", policy.id]);
			onUpdate?.();
			toast.success("Exclusion removed");
		},
		onError: (err) => toast.error(err.response?.data?.error || err.message),
	});

	const getTargetLabel = (a) => {
		if (a.target_type === "host") {
			const h = hosts.find((x) => x.id === a.target_id);
			return h?.friendly_name || h?.hostname || a.target_id;
		}
		const g = hostGroups.find((x) => x.id === a.target_id);
		return g?.name || a.target_id;
	};

	return (
		<div className="px-4 pb-4 pt-0 bg-secondary-50 dark:bg-secondary-900/50 border-t border-secondary-200 dark:border-secondary-600">
			<div className="space-y-3 mt-2">
				<div className="flex items-center gap-2 text-sm font-medium text-secondary-700 dark:text-secondary-300">
					<Users className="h-4 w-4" />
					Applied to
				</div>
				{assignments.length === 0 ? (
					<p className="text-sm text-secondary-500">
						No assignments. Add a host or host group.
					</p>
				) : (
					<ul className="flex flex-wrap gap-2">
						{assignments.map((a) => (
							<li
								key={a.id}
								className="inline-flex items-center gap-1 px-2 py-1 rounded bg-secondary-200 dark:bg-secondary-700 text-sm"
							>
								{a.target_type === "host" ? (
									<Server className="h-3 w-3" />
								) : (
									<Users className="h-3 w-3" />
								)}
								{getTargetLabel(a)}
								<button
									type="button"
									onClick={() => removeAssignmentMutation.mutate(a.id)}
									className="ml-1 text-secondary-500 hover:text-red-600"
								>
									<X className="h-3 w-3" />
								</button>
							</li>
						))}
					</ul>
				)}
				<div className="flex flex-wrap items-center gap-2">
					<select
						value={addTargetType}
						onChange={(e) => {
							setAddTargetType(e.target.value);
							setAddTargetId("");
						}}
						className="rounded border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white text-sm px-2 py-1"
					>
						<option value="host">Host</option>
						<option value="host_group">Host group</option>
					</select>
					<select
						value={addTargetId}
						onChange={(e) => setAddTargetId(e.target.value)}
						className="rounded border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white text-sm px-2 py-1 min-w-[160px]"
					>
						<option value="">
							Select {addTargetType === "host" ? "host" : "group"}…
						</option>
						{addTargetType === "host"
							? hosts.map((h) => (
									<option key={h.id} value={h.id}>
										{h.friendly_name || h.hostname || h.id}
									</option>
								))
							: hostGroups.map((g) => (
									<option key={g.id} value={g.id}>
										{g.name}
									</option>
								))}
					</select>
					<button
						type="button"
						onClick={() => addTargetId && addAssignmentMutation.mutate()}
						disabled={!addTargetId || addAssignmentMutation.isPending}
						className="btn-outline text-sm py-1"
					>
						Add
					</button>
				</div>

				<div className="flex items-center gap-2 text-sm font-medium text-secondary-700 dark:text-secondary-300 pt-2 border-t border-secondary-200 dark:border-secondary-600">
					<Clock className="h-4 w-4" />
					Exclusions (hosts excluded from this policy when applied via group)
				</div>
				{exclusions.length === 0 ? (
					<p className="text-sm text-secondary-500">No exclusions.</p>
				) : (
					<ul className="flex flex-wrap gap-2">
						{exclusions.map((exc) => (
							<li
								key={exc.id}
								className="inline-flex items-center gap-1 px-2 py-1 rounded bg-amber-100 dark:bg-amber-900/30 text-sm"
							>
								{exc.hosts?.friendly_name || exc.hosts?.hostname || exc.host_id}
								<button
									type="button"
									onClick={() => removeExclusionMutation.mutate(exc.host_id)}
									className="ml-1 text-amber-700 dark:text-amber-400 hover:text-red-600"
								>
									<X className="h-3 w-3" />
								</button>
							</li>
						))}
					</ul>
				)}
				<div className="flex flex-wrap items-center gap-2">
					<select
						value={addExclusionHostId}
						onChange={(e) => setAddExclusionHostId(e.target.value)}
						className="rounded border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white text-sm px-2 py-1 min-w-[160px]"
					>
						<option value="">Select host to exclude…</option>
						{hosts.map((h) => (
							<option key={h.id} value={h.id}>
								{h.friendly_name || h.hostname || h.id}
							</option>
						))}
					</select>
					<button
						type="button"
						onClick={() => addExclusionHostId && addExclusionMutation.mutate()}
						disabled={!addExclusionHostId || addExclusionMutation.isPending}
						className="btn-outline text-sm py-1"
					>
						Exclude host
					</button>
				</div>
			</div>
		</div>
	);
}

export default PatchManagement;
