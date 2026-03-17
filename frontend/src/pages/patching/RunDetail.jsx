import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
	AlertTriangle,
	ArrowLeft,
	PlayCircle,
	RefreshCw,
	Server,
	Shield,
	User,
} from "lucide-react";
import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import {
	PackageListDisplay,
	PackageNameList,
} from "../../components/PackageListDisplay";
import { PatchRunStatusBadge } from "../../components/PatchRunStatusBadge";
import { patchingAPI } from "../../utils/patchingApi";

const RunDetail = () => {
	const { id } = useParams();
	const queryClient = useQueryClient();
	const [approvingId, setApprovingId] = useState(null);

	const approveMutation = useMutation({
		mutationFn: (runId) => patchingAPI.approveRun(runId),
		onSuccess: () => {
			queryClient.invalidateQueries(["patching-run", id]);
			queryClient.invalidateQueries(["patching-runs"]);
			queryClient.invalidateQueries(["patching-dashboard"]);
		},
	});

	const handleApprove = async () => {
		setApprovingId(id);
		try {
			await approveMutation.mutateAsync(id);
		} finally {
			setApprovingId(null);
		}
	};

	const {
		data: run,
		isLoading,
		error,
	} = useQuery({
		queryKey: ["patching-run", id],
		queryFn: () => patchingAPI.getRunById(id),
		enabled: !!id,
		refetchInterval: (query) => {
			const status = query.state.data?.status;
			if (
				status === "queued" ||
				status === "running" ||
				status === "pending_validation"
			) {
				return 3000;
			}
			return false;
		},
	});

	if (isLoading) {
		return (
			<div className="flex items-center justify-center h-64">
				<div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-500"></div>
			</div>
		);
	}

	if (error || !run) {
		return (
			<div className="p-4 bg-red-900/50 border border-red-700 rounded-lg">
				<p className="text-red-200">Patch run not found or failed to load</p>
				<Link
					to="/patching"
					className="text-primary-400 hover:underline mt-2 inline-block"
				>
					Back to Patching
				</Link>
			</div>
		);
	}

	const hostName =
		run.hosts?.friendly_name || run.hosts?.hostname || run.host_id;

	// Format policy settings into a human-readable description line
	const policyDetail = (() => {
		const snap = run.policy_snapshot;
		if (!snap) return null;
		const type = snap.patch_delay_type;
		if (type === "immediate") return "Runs immediately on trigger";
		if (type === "delayed" && snap.delay_minutes != null)
			return `Delayed by ${snap.delay_minutes} min after trigger`;
		if (type === "fixed_time" && snap.fixed_time_utc) {
			const tz = snap.timezone || "UTC";
			return `Fixed time: ${snap.fixed_time_utc} (${tz})`;
		}
		return type || null;
	})();

	// Normalize shell output: apt/dpkg use \r to overwrite the same line (progress); show each as its own line
	const shellDisplay =
		run.shell_output != null && run.shell_output !== ""
			? run.shell_output.replace(/\r\n/g, "\n").replace(/\r/g, "\n")
			: "(No output yet)";

	return (
		<div className="space-y-6">
			<div className="flex items-center gap-4">
				<Link
					to="/patching"
					className="flex items-center gap-1 text-secondary-600 dark:text-secondary-400 hover:text-primary-600 dark:hover:text-primary-400"
				>
					<ArrowLeft className="h-4 w-4" />
					Back to Patching
				</Link>
			</div>

			<div className="card p-4">
				<h2 className="text-lg font-semibold text-secondary-900 dark:text-white mb-4">
					Patch run details
				</h2>
				<dl className="grid grid-cols-1 sm:grid-cols-2 gap-3 text-sm">
					<div className="flex items-center gap-2">
						<Server className="h-4 w-4 text-secondary-500" />
						<span className="text-secondary-500 dark:text-secondary-400">
							Host
						</span>
						<span className="text-secondary-900 dark:text-white">
							{hostName}
						</span>
					</div>
					<div className="flex items-center gap-2 sm:col-span-2">
						<span className="text-secondary-500 dark:text-secondary-400">
							Type
						</span>
						<span className="text-secondary-900 dark:text-white">
							<PackageListDisplay run={run} />
						</span>
					</div>
					<div className="flex items-center gap-2 flex-wrap">
						<span className="text-secondary-500 dark:text-secondary-400">
							Status{" "}
						</span>
						<PatchRunStatusBadge run={run} />
						{run.status === "validated" &&
							run.packages_affected?.length >
								(run.package_names?.length || 1) && (
								<span className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs font-medium bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200">
									<AlertTriangle className="h-3 w-3" />
									Extra dependencies
								</span>
							)}
					</div>
					<div className="flex items-center gap-2">
						<User className="h-4 w-4 text-secondary-500" />
						<span className="text-secondary-500 dark:text-secondary-400">
							Initiated by{" "}
						</span>
						<span className="text-secondary-900 dark:text-white">
							{run.triggered_by_username || "—"}
						</span>
					</div>
					{run.approved_by_username && (
						<div className="flex items-center gap-2">
							<User className="h-4 w-4 text-secondary-500" />
							<span className="text-secondary-500 dark:text-secondary-400">
								Approved by{" "}
							</span>
							<span className="text-secondary-900 dark:text-white">
								{run.approved_by_username}
							</span>
						</div>
					)}
					{(run.policy_name || run.policy_snapshot) && (
						<div className="sm:col-span-2 flex items-start gap-2 p-3 rounded-lg bg-secondary-50 dark:bg-secondary-700/40 border border-secondary-200 dark:border-secondary-600">
							<Shield className="h-4 w-4 text-primary-500 mt-0.5 shrink-0" />
							<div className="space-y-1 text-sm">
								<p className="font-medium text-secondary-800 dark:text-secondary-200">
									{run.policy_name ?? "Patch policy"}
								</p>
								{policyDetail && (
									<p className="text-secondary-500 dark:text-secondary-400 text-xs">
										{policyDetail}
									</p>
								)}
								{run.policy_snapshot?.patch_delay_type === "immediate" &&
									!policyDetail &&
									null}
							</div>
						</div>
					)}
					{!run.policy_name && !run.policy_snapshot && !run.dry_run && (
						<div className="flex items-center gap-2">
							<Shield className="h-4 w-4 text-secondary-400" />
							<span className="text-secondary-500 dark:text-secondary-400">
								Policy
							</span>
							<span className="text-secondary-500 dark:text-secondary-400 italic text-xs">
								Default (immediate)
							</span>
						</div>
					)}
					<div>
						<span className="text-secondary-500 dark:text-secondary-400">
							Started{" "}
						</span>
						<span className="text-secondary-900 dark:text-white">
							{run.started_at
								? new Date(run.started_at).toLocaleString()
								: run.created_at
									? new Date(run.created_at).toLocaleString()
									: "—"}
						</span>
					</div>
					{run.status === "queued" && run.scheduled_at && (
						<div>
							<span className="text-secondary-500 dark:text-secondary-400">
								Scheduled for{" "}
							</span>
							<span className="text-secondary-900 dark:text-white">
								{new Date(run.scheduled_at).toLocaleString()}
							</span>
						</div>
					)}
					<div>
						<span className="text-secondary-500 dark:text-secondary-400">
							Completed{" "}
						</span>
						<span className="text-secondary-900 dark:text-white">
							{run.completed_at
								? new Date(run.completed_at).toLocaleString()
								: "—"}
						</span>
					</div>
					{run.status === "validated" && (
						<div className="sm:col-span-2 flex items-start gap-3 p-3 rounded-lg bg-amber-50 dark:bg-amber-900/30 border border-amber-200 dark:border-amber-600">
							<AlertTriangle className="h-5 w-5 text-amber-600 dark:text-amber-400 mt-0.5 shrink-0" />
							<div className="flex-1">
								<p className="text-sm font-medium text-amber-800 dark:text-amber-200">
									Validation complete — approval required
								</p>
								<p className="text-xs text-amber-700 dark:text-amber-300 mt-0.5">
									{run.packages_affected?.length >
									(run.package_names?.length || 1)
										? `This run will install ${run.packages_affected.length} packages including additional dependencies. Review the output and approve to proceed.`
										: "Review the dry-run output and approve to proceed with patching."}
								</p>
							</div>
							<button
								type="button"
								onClick={handleApprove}
								disabled={approvingId === id}
								className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded bg-primary-600 text-white text-sm hover:bg-primary-700 disabled:opacity-50 shrink-0"
							>
								{approvingId === id ? (
									<>
										<RefreshCw className="h-4 w-4 animate-spin" />
										Approving…
									</>
								) : (
									<>
										<PlayCircle className="h-4 w-4" />
										Approve & Patch
									</>
								)}
							</button>
						</div>
					)}
					{run.error_message && (
						<div className="sm:col-span-2">
							<span className="text-secondary-500 dark:text-secondary-400">
								Error{" "}
							</span>
							<p className="mt-1 text-red-600 dark:text-red-400 font-mono text-xs bg-red-50 dark:bg-red-900/20 p-2 rounded">
								{run.error_message}
							</p>
						</div>
					)}
					{run.packages_affected?.length > 0 && (
						<div className="sm:col-span-2">
							<span className="text-secondary-500 dark:text-secondary-400">
								Packages affected{" "}
							</span>
							<div className="mt-1 text-sm text-secondary-700 dark:text-secondary-300">
								<PackageNameList
									packages={run.packages_affected}
									showIcon={false}
								/>
							</div>
						</div>
					)}
				</dl>
			</div>

			<div className="card p-4">
				<h3 className="text-md font-medium text-secondary-900 dark:text-white mb-3">
					Shell output
				</h3>
				<div className="rounded-lg border border-secondary-700 dark:border-secondary-600 bg-[#0d1117] dark:bg-black overflow-hidden shadow-inner">
					<pre
						className="block w-full min-h-[240px] max-h-[75vh] overflow-auto p-4 text-[13px] leading-relaxed font-mono text-[#e6edf3] whitespace-pre-wrap break-words"
						style={{
							fontFamily:
								"ui-monospace, SFMono-Regular, 'SF Mono', Menlo, Consolas, 'Liberation Mono', monospace",
						}}
					>
						{shellDisplay}
					</pre>
				</div>
				<p className="mt-2 text-xs text-secondary-500 dark:text-secondary-400">
					Output streams (stdout/stderr) from the patch run. Scroll to see full
					output.
				</p>
			</div>
		</div>
	);
};

export default RunDetail;
