import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
	AlertTriangle,
	ArrowLeft,
	CheckCircle2,
	PlayCircle,
	RefreshCw,
	Server,
	Shield,
	Square,
	User,
} from "lucide-react";

import { useEffect, useRef, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import {
	PackageListDisplay,
	PackageNameList,
} from "../../components/PackageListDisplay";
import { PatchRunStatusBadge } from "../../components/PatchRunStatusBadge";
import { formatDate } from "../../utils/api";
import { buildRunStreamURL, patchingAPI } from "../../utils/patchingApi";

// Statuses where we should open the live WebSocket stream and show the Stop
// Run affordance. Everything else is already terminal or still scheduled.
const LIVE_STATUSES = new Set(["running"]);

// Statuses that indicate the run has reached a final state and we should stop
// any open live stream.
const TERMINAL_STATUSES = new Set([
	"completed",
	"failed",
	"cancelled",
	"validated",
	"dry_run_completed",
]);

// PostPatchReportPill renders one of two pills next to the run status:
//
//   - "Awaiting inventory report" while the host is still the one flagged
//     as owing us a post-patch report (server sets this when the run hits
//     "completed" and clears it when the host report arrives).
//   - "New report received" for a short window AFTER the awaiting flag
//     clears, as long as the host's last_update timestamp is newer than
//     this run's completed_at. This gives the user a clear "inventory
//     refreshed" confirmation without leaving it permanently pinned.
const PostPatchReportPill = ({ run }) => {
	const isAwaiting = run.hosts?.awaiting_post_patch_report_run_id === run.id;
	if (isAwaiting) {
		return (
			<span className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs font-medium bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200">
				<RefreshCw className="h-3 w-3 animate-spin" />
				Awaiting inventory report
			</span>
		);
	}

	// Only claim "received" when we can actually prove an inventory report
	// landed after this run completed. Without both timestamps the pill is
	// suppressed — the absence of the awaiting pill is already a soft hint
	// of success.
	const completedAt = run.completed_at ? Date.parse(run.completed_at) : NaN;
	const lastUpdate = run.hosts?.last_update
		? Date.parse(run.hosts.last_update)
		: NaN;
	if (
		Number.isFinite(completedAt) &&
		Number.isFinite(lastUpdate) &&
		lastUpdate >= completedAt
	) {
		return (
			<span className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs font-medium bg-emerald-100 text-emerald-800 dark:bg-emerald-900/40 dark:text-emerald-300">
				<CheckCircle2 className="h-3 w-3" />
				New report received
			</span>
		);
	}
	return null;
};

const RunDetail = () => {
	const { id } = useParams();
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const [approvingId, setApprovingId] = useState(null);
	const [stopConfirmOpen, setStopConfirmOpen] = useState(false);
	const [stopError, setStopError] = useState(null);

	// Live output buffer built from the WebSocket snapshot + chunks. When the
	// run is still active this is the source-of-truth for the terminal view;
	// on terminal stages we fall back to the persisted shell_output returned
	// by the run query (which the agent re-sends as the final authoritative
	// blob).
	const [liveOutput, setLiveOutput] = useState("");
	const [isStreamOpen, setIsStreamOpen] = useState(false);
	const outputRef = useRef(null);
	// Tracks whether the user is currently pinned to the bottom of the
	// terminal pane. We only auto-scroll when true so we don't fight a user
	// who has scrolled up to read earlier output.
	const stickToBottomRef = useRef(true);

	const approveMutation = useMutation({
		mutationFn: (runId) => patchingAPI.approveRun(runId),
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ["patching-run", id] });
			queryClient.invalidateQueries({ queryKey: ["patching-runs"] });
			queryClient.invalidateQueries({ queryKey: ["patching-dashboard"] });
		},
	});
	const retryValidationMutation = useMutation({
		mutationFn: (runId) => patchingAPI.retryValidation(runId),
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ["patching-run", id] });
			queryClient.invalidateQueries({ queryKey: ["patching-runs"] });
			queryClient.invalidateQueries({ queryKey: ["patching-dashboard"] });
		},
	});
	const stopRunMutation = useMutation({
		mutationFn: (runId) => patchingAPI.stopRun(runId),
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ["patching-run", id] });
			queryClient.invalidateQueries({ queryKey: ["patching-runs"] });
		},
	});
	const [retryingId, setRetryingId] = useState(null);

	const handleApprove = async () => {
		setApprovingId(id);
		try {
			// Approving a validation run creates a NEW execution run on the
			// server. If it's going to start immediately, deep-link straight
			// into its detail page so the user can watch the live terminal
			// without hunting for the new row in the runs list.
			const res = await approveMutation.mutateAsync(id);
			const newRunId = res?.patch_run_id;
			const runAt = res?.run_at ? Date.parse(res.run_at) : NaN;
			const isImmediate = Number.isFinite(runAt) && runAt - Date.now() < 5_000;
			if (newRunId && newRunId !== id && isImmediate) {
				navigate(`/patching/runs/${newRunId}`);
			}
		} finally {
			setApprovingId(null);
		}
	};

	const handleRetryValidation = async () => {
		setRetryingId(id);
		try {
			await retryValidationMutation.mutateAsync(id);
		} finally {
			setRetryingId(null);
		}
	};

	const handleConfirmStop = async () => {
		setStopError(null);
		try {
			await stopRunMutation.mutateAsync(id);
			setStopConfirmOpen(false);
		} catch (err) {
			const msg =
				err?.response?.data?.error ||
				err?.message ||
				"Failed to stop patch run";
			setStopError(msg);
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
		// While the WebSocket is open we already have push updates for the
		// output itself, so we only poll at a slow cadence to pick up
		// metadata (packages_affected, completed_at, status). When the WS is
		// closed we fall back to the existing 3s polling for active runs.
		// We also keep polling slowly while the run is completed but the
		// host still owes us a post-patch inventory report, so the
		// "Awaiting inventory report" pill flips to "New report received"
		// without the user having to refresh the page.
		refetchInterval: (query) => {
			const data = query.state.data;
			const status = data?.status;
			if (status === "queued" || status === "pending_validation") {
				return 3000;
			}
			if (status === "running") {
				return isStreamOpen ? 5000 : 3000;
			}
			if (
				status === "completed" &&
				!data?.dry_run &&
				data?.hosts?.awaiting_post_patch_report_run_id === data?.id
			) {
				return 3000;
			}
			return false;
		},
	});

	// Open / close the live output WebSocket based on run status. We
	// intentionally key the effect on the status (not the full run object)
	// so we don't churn the socket on unrelated metadata updates.
	const runStatus = run?.status;
	useEffect(() => {
		if (!id || !runStatus) return undefined;
		if (!LIVE_STATUSES.has(runStatus)) {
			setIsStreamOpen(false);
			return undefined;
		}

		const ws = new WebSocket(buildRunStreamURL(id));
		let closed = false;

		ws.onopen = () => {
			setIsStreamOpen(true);
		};
		ws.onmessage = (event) => {
			let msg;
			try {
				msg = JSON.parse(event.data);
			} catch {
				return;
			}
			if (!msg || typeof msg !== "object") return;
			switch (msg.type) {
				case "snapshot":
					setLiveOutput(msg.shell_output || "");
					break;
				case "chunk":
					if (typeof msg.chunk === "string" && msg.chunk.length > 0) {
						setLiveOutput((prev) => prev + msg.chunk);
					}
					break;
				case "done":
					// The server will immediately persist the final output
					// and the run query will refetch with fresh data; close
					// the socket so we stop showing "Live" in the UI.
					if (!closed) {
						closed = true;
						ws.close();
					}
					// Refetch so the terminal state (completed / failed /
					// cancelled) lands in the UI without waiting for the
					// next poll tick.
					queryClient.invalidateQueries({ queryKey: ["patching-run", id] });
					break;
				default:
					break;
			}
		};
		ws.onerror = () => {
			// Swallow: onclose fires right after and handles state cleanup.
		};
		ws.onclose = () => {
			setIsStreamOpen(false);
		};

		return () => {
			closed = true;
			try {
				ws.close();
			} catch {
				// ignore: socket may already be closed.
			}
		};
	}, [id, runStatus, queryClient]);

	// Reset the live buffer when the run id changes so stale output from a
	// previous page doesn't leak into a freshly-opened run.
	// biome-ignore lint/correctness/useExhaustiveDependencies: this effect is intentionally keyed on id only.
	useEffect(() => {
		setLiveOutput("");
		stickToBottomRef.current = true;
	}, [id]);

	// Track whether the user is pinned to the bottom of the terminal pane so
	// we know whether to auto-scroll on each chunk. A 32px slop lets the
	// user scroll slightly off the bottom without detaching.
	useEffect(() => {
		const el = outputRef.current;
		if (!el) return undefined;
		const onScroll = () => {
			const gap = el.scrollHeight - (el.scrollTop + el.clientHeight);
			stickToBottomRef.current = gap < 32;
		};
		el.addEventListener("scroll", onScroll);
		return () => el.removeEventListener("scroll", onScroll);
	}, []);

	// Auto-scroll to the bottom whenever the live buffer grows, but only
	// while the user is still pinned there. Runs after paint via a
	// requestAnimationFrame so the new content is laid out first.
	// biome-ignore lint/correctness/useExhaustiveDependencies: intentionally re-runs on every liveOutput change.
	useEffect(() => {
		if (!stickToBottomRef.current) return undefined;
		const el = outputRef.current;
		if (!el) return undefined;
		const rafId = requestAnimationFrame(() => {
			el.scrollTop = el.scrollHeight;
		});
		return () => cancelAnimationFrame(rafId);
	}, [liveOutput]);

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

	// Prefer the live WebSocket buffer while the run is active so the user
	// sees stdout/stderr lines as they arrive. Once the run reaches a
	// terminal state the agent re-sends the full authoritative output and
	// we display the persisted value from the run query.
	const rawShell =
		LIVE_STATUSES.has(run.status) && liveOutput
			? liveOutput
			: run.shell_output || "";

	// Normalize shell output: apt/dpkg use \r to overwrite the same line
	// (progress bars); show each overwrite as its own line so the user gets
	// a readable scrollback.
	const shellDisplay = rawShell
		? rawShell.replace(/\r\n/g, "\n").replace(/\r/g, "\n")
		: "(No output yet)";

	const canStop =
		run.status === "running" &&
		!stopRunMutation.isPending &&
		!TERMINAL_STATUSES.has(run.status);

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
						{run.status === "completed" && !run.dry_run && (
							<PostPatchReportPill run={run} />
						)}
					</div>
					<div className="flex items-center gap-2">
						<User className="h-4 w-4 text-secondary-500" />
						<span className="text-secondary-500 dark:text-secondary-400">
							Initiated by{" "}
						</span>
						<span className="text-secondary-900 dark:text-white">
							{run.triggered_by_username || " -"}
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
								? formatDate(run.started_at)
								: run.created_at
									? formatDate(run.created_at)
									: " -"}
						</span>
					</div>
					{run.status === "queued" && run.scheduled_at && (
						<div>
							<span className="text-secondary-500 dark:text-secondary-400">
								Scheduled for{" "}
							</span>
							<span className="text-secondary-900 dark:text-white">
								{formatDate(run.scheduled_at)}
							</span>
						</div>
					)}
					<div>
						<span className="text-secondary-500 dark:text-secondary-400">
							Completed{" "}
						</span>
						<span className="text-secondary-900 dark:text-white">
							{run.completed_at ? formatDate(run.completed_at) : " -"}
						</span>
					</div>
					{/* Link to related run (validation ↔ patch) */}
					{run.validation_run_id && (
						<div className="flex items-center gap-2">
							<Shield className="h-4 w-4 text-secondary-500" />
							<span className="text-secondary-500 dark:text-secondary-400">
								Validation run
							</span>
							<Link
								to={`/patching/runs/${run.validation_run_id}`}
								className="text-primary-600 dark:text-primary-400 hover:underline text-sm"
							>
								View validation output
							</Link>
						</div>
					)}
					{run.status === "pending_validation" && (
						<div className="sm:col-span-2 flex items-start gap-3 p-3 rounded-lg bg-amber-50 dark:bg-amber-900/30 border border-amber-200 dark:border-amber-600">
							<AlertTriangle className="h-5 w-5 text-amber-600 dark:text-amber-400 mt-0.5 shrink-0" />
							<div className="flex-1">
								<p className="text-sm font-medium text-amber-800 dark:text-amber-200">
									Validation pending - host may be offline
								</p>
								<p className="text-xs text-amber-700 dark:text-amber-300 mt-0.5">
									The dry-run has not completed. You can retry when the host is
									back online, or skip validation to patch immediately.
								</p>
							</div>
							<div className="flex items-center gap-2 shrink-0">
								<button
									type="button"
									onClick={handleRetryValidation}
									disabled={retryingId === id}
									className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded border border-secondary-300 dark:border-secondary-600 text-secondary-700 dark:text-secondary-300 text-sm hover:bg-secondary-100 dark:hover:bg-secondary-700 disabled:opacity-50"
								>
									{retryingId === id ? (
										<>
											<RefreshCw className="h-4 w-4 animate-spin" />
											Retrying…
										</>
									) : (
										<>
											<RefreshCw className="h-4 w-4" />
											Retry Validation
										</>
									)}
								</button>
								<button
									type="button"
									onClick={handleApprove}
									disabled={approvingId === id}
									className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded bg-amber-600 text-white text-sm hover:bg-amber-700 disabled:opacity-50"
								>
									{approvingId === id ? (
										<>
											<RefreshCw className="h-4 w-4 animate-spin" />
											Queuing…
										</>
									) : (
										<>
											<PlayCircle className="h-4 w-4" />
											Skip & Patch
										</>
									)}
								</button>
							</div>
						</div>
					)}
					{run.status === "validated" && (
						<div className="sm:col-span-2 flex items-start gap-3 p-3 rounded-lg bg-amber-50 dark:bg-amber-900/30 border border-amber-200 dark:border-amber-600">
							<AlertTriangle className="h-5 w-5 text-amber-600 dark:text-amber-400 mt-0.5 shrink-0" />
							<div className="flex-1">
								<p className="text-sm font-medium text-amber-800 dark:text-amber-200">
									Validation complete - approval required
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
				<div className="flex items-center justify-between mb-3 gap-2">
					<div className="flex items-center gap-2">
						<h3 className="text-md font-medium text-secondary-900 dark:text-white">
							Shell output
						</h3>
						{isStreamOpen && run.status === "running" && (
							<span className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded text-xs font-medium bg-emerald-100 text-emerald-800 dark:bg-emerald-900/40 dark:text-emerald-300">
								<span className="h-1.5 w-1.5 rounded-full bg-emerald-500 animate-pulse" />
								Live
							</span>
						)}
					</div>
					{canStop && (
						<button
							type="button"
							onClick={() => {
								setStopError(null);
								setStopConfirmOpen(true);
							}}
							className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded bg-red-600 text-white text-sm hover:bg-red-700 disabled:opacity-50"
						>
							<Square className="h-4 w-4" />
							Stop Run
						</button>
					)}
				</div>
				<div className="rounded-lg border border-secondary-700 dark:border-secondary-600 bg-[#0d1117] dark:bg-black overflow-hidden shadow-inner">
					<pre
						ref={outputRef}
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

			{stopConfirmOpen && (
				<div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4">
					<div className="w-full max-w-md rounded-lg bg-white dark:bg-secondary-800 shadow-xl border border-secondary-200 dark:border-secondary-700">
						<div className="p-5 border-b border-secondary-200 dark:border-secondary-700 flex items-start gap-3">
							<AlertTriangle className="h-5 w-5 text-red-500 mt-0.5 shrink-0" />
							<div>
								<h4 className="text-base font-semibold text-secondary-900 dark:text-white">
									Stop this patch run?
								</h4>
								<p className="mt-1 text-sm text-secondary-600 dark:text-secondary-300">
									The agent will be asked to interrupt the current command.
									Partially-installed packages may leave the host in an
									intermediate state; the agent will report back its final
									output and an updated inventory when it exits.
								</p>
							</div>
						</div>
						{stopError && (
							<div className="mx-5 mt-3 rounded border border-red-300 bg-red-50 text-red-700 dark:border-red-700 dark:bg-red-900/30 dark:text-red-200 p-2 text-sm">
								{stopError}
							</div>
						)}
						<div className="p-4 flex items-center justify-end gap-2">
							<button
								type="button"
								onClick={() => setStopConfirmOpen(false)}
								disabled={stopRunMutation.isPending}
								className="px-3 py-1.5 rounded border border-secondary-300 dark:border-secondary-600 text-secondary-700 dark:text-secondary-200 text-sm hover:bg-secondary-100 dark:hover:bg-secondary-700 disabled:opacity-50"
							>
								Cancel
							</button>
							<button
								type="button"
								onClick={handleConfirmStop}
								disabled={stopRunMutation.isPending}
								className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded bg-red-600 text-white text-sm hover:bg-red-700 disabled:opacity-50"
							>
								{stopRunMutation.isPending ? (
									<>
										<RefreshCw className="h-4 w-4 animate-spin" />
										Stopping…
									</>
								) : (
									<>
										<Square className="h-4 w-4" />
										Stop Run
									</>
								)}
							</button>
						</div>
					</div>
				</div>
			)}
		</div>
	);
};

export default RunDetail;
