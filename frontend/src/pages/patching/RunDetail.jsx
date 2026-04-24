import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
	AlertTriangle,
	ArrowLeft,
	CheckCircle2,
	Copy,
	PlayCircle,
	RefreshCw,
	Server,
	Shield,
	Square,
	User,
	X,
} from "lucide-react";

import { useEffect, useRef, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import {
	PackageListDisplay,
	PackageNameList,
} from "../../components/PackageListDisplay";
import { PatchRunStatusBadge } from "../../components/PatchRunStatusBadge";
import { useToast } from "../../contexts/ToastContext";
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

// Compact label/value stack used inside the Run Summary sidebar. Putting the
// label above the value keeps rows narrow and avoids the "empty gutter"
// problem the old horizontal layout suffered from.
const SummaryRow = ({ label, icon: Icon, children }) => (
	<div>
		<dt className="flex items-center gap-1.5 text-xs font-medium uppercase tracking-wider text-secondary-500 dark:text-white/70">
			{Icon ? <Icon className="h-3.5 w-3.5" /> : null}
			{label}
		</dt>
		<dd className="mt-1 text-sm text-secondary-900 dark:text-white">
			{children}
		</dd>
	</div>
);

const RunDetail = () => {
	const { id } = useParams();
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const toast = useToast();
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
				<RefreshCw className="h-8 w-8 animate-spin text-primary-600" />
			</div>
		);
	}

	if (error || !run) {
		return (
			<div className="bg-danger-50 border border-danger-200 rounded-md p-4 dark:bg-danger-900/30 dark:border-danger-700">
				<div className="flex">
					<AlertTriangle className="h-5 w-5 text-danger-400 flex-shrink-0" />
					<div className="ml-3">
						<h3 className="text-sm font-medium text-danger-800 dark:text-danger-200">
							Patch run not found
						</h3>
						<p className="text-sm text-danger-700 dark:text-danger-300 mt-1">
							The requested patch run either doesn't exist or failed to load.
						</p>
						<Link
							to="/patching?tab=runs"
							className="mt-3 inline-flex items-center gap-1.5 text-sm font-medium text-danger-700 dark:text-danger-300 hover:underline"
						>
							<ArrowLeft className="h-4 w-4" />
							Back to Patching
						</Link>
					</div>
				</div>
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

	const hasExtraDeps =
		run.status === "validated" &&
		run.packages_affected?.length > (run.package_names?.length || 1);

	const hasPolicy = Boolean(run.policy_name || run.policy_snapshot);
	const showDefaultPolicy = !hasPolicy && !run.dry_run;

	const handleCopyOutput = async () => {
		try {
			await navigator.clipboard.writeText(shellDisplay);
			toast.success("Shell output copied to clipboard");
		} catch {
			toast.error("Unable to copy output");
		}
	};

	// Subtitle line shown under the H1: a concise summary of "what this run
	// is" (type) with the live status badge and the post-patch pill inline.
	const headerSubtitle = (
		<div className="flex items-center flex-wrap gap-x-2 gap-y-1 text-sm text-secondary-600 dark:text-white/80 mt-1">
			<span className="inline-flex items-center">
				<PackageListDisplay run={run} />
			</span>
			<span aria-hidden="true" className="text-secondary-400">
				·
			</span>
			<PatchRunStatusBadge run={run} />
			{hasExtraDeps && (
				<span className="inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs font-medium bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200">
					<AlertTriangle className="h-3 w-3" />
					Extra dependencies
				</span>
			)}
			{run.status === "completed" && !run.dry_run && (
				<PostPatchReportPill run={run} />
			)}
		</div>
	);

	// Primary actions that live in the Detail Page Header's right slot.
	// These are the dominant call-to-action for the current run state so
	// users don't have to scroll past summary content to act on them.
	const headerActions = (
		<>
			{run.status === "pending_validation" && (
				<>
					<button
						type="button"
						onClick={handleRetryValidation}
						disabled={retryingId === id}
						className="btn-outline inline-flex items-center gap-1.5 min-h-[44px] disabled:opacity-50 disabled:cursor-not-allowed"
					>
						<RefreshCw
							className={`h-4 w-4 ${retryingId === id ? "animate-spin" : ""}`}
						/>
						{retryingId === id ? "Retrying…" : "Retry Validation"}
					</button>
					<button
						type="button"
						onClick={handleApprove}
						disabled={approvingId === id}
						className="btn-warning inline-flex items-center gap-1.5 min-h-[44px] disabled:opacity-50 disabled:cursor-not-allowed"
					>
						{approvingId === id ? (
							<RefreshCw className="h-4 w-4 animate-spin" />
						) : (
							<PlayCircle className="h-4 w-4" />
						)}
						{approvingId === id ? "Queuing…" : "Skip & Patch"}
					</button>
				</>
			)}
			{(run.status === "pending_approval" || run.status === "validated") && (
				<button
					type="button"
					onClick={handleApprove}
					disabled={approvingId === id}
					className="btn-primary inline-flex items-center gap-1.5 min-h-[44px] disabled:opacity-50 disabled:cursor-not-allowed"
				>
					{approvingId === id ? (
						<RefreshCw className="h-4 w-4 animate-spin" />
					) : (
						<PlayCircle className="h-4 w-4" />
					)}
					{approvingId === id
						? run.status === "validated"
							? "Approving…"
							: "Queuing…"
						: "Approve & Patch"}
				</button>
			)}
			{canStop && (
				<button
					type="button"
					onClick={() => {
						setStopError(null);
						setStopConfirmOpen(true);
					}}
					className="btn-danger inline-flex items-center gap-1.5 min-h-[44px] disabled:opacity-50 disabled:cursor-not-allowed"
				>
					<Square className="h-4 w-4" />
					Stop Run
				</button>
			)}
		</>
	);

	return (
		<div className="space-y-6">
			{/* Detail Page Header (§2.5) */}
			<div className="flex flex-col md:flex-row md:items-start md:justify-between gap-4 mb-4 pb-4 border-b border-secondary-200 dark:border-secondary-600">
				<div className="flex items-start gap-3 min-w-0">
					<Link
						to="/patching?tab=runs"
						className="text-secondary-500 hover:text-secondary-700 dark:text-white dark:hover:text-secondary-200 mt-1 flex-shrink-0"
						aria-label="Back to Patching"
					>
						<ArrowLeft className="h-5 w-5" />
					</Link>
					<div className="min-w-0">
						<h1 className="text-2xl font-semibold text-secondary-900 dark:text-white truncate">
							Run on {hostName}
						</h1>
						{headerSubtitle}
					</div>
				</div>
				<div className="flex items-center flex-wrap gap-3 md:flex-shrink-0">
					{headerActions}
				</div>
			</div>

			{/* Two-pane layout: summary sidebar (left) + primary content (right) */}
			<div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
				{/* Summary sidebar */}
				<aside className="lg:col-span-1 lg:self-start">
					<div className="card p-4 sm:p-6 flex flex-col">
						<h3 className="text-lg font-medium text-secondary-900 dark:text-white mb-4 flex-shrink-0">
							Run summary
						</h3>
						<dl className="space-y-4">
							<SummaryRow label="Host" icon={Server}>
								{run.host_id ? (
									<Link
										to={`/hosts/${run.host_id}`}
										className="text-primary-600 dark:text-primary-400 hover:underline break-words"
									>
										{hostName}
									</Link>
								) : (
									<span className="break-words">{hostName}</span>
								)}
							</SummaryRow>

							<SummaryRow label="Type">
								<div className="text-sm text-secondary-900 dark:text-white">
									<PackageListDisplay run={run} />
								</div>
							</SummaryRow>

							{/* People: Initiated by + Approved by side-by-side. Collapses to
							    a single full-width row when the run hasn't been approved. */}
							<div className="grid grid-cols-2 gap-4">
								<SummaryRow label="Initiated by" icon={User}>
									<span className="break-words">
										{run.triggered_by_username || "—"}
									</span>
								</SummaryRow>
								<SummaryRow label="Approved by" icon={User}>
									<span className="break-words">
										{run.approved_by_username || "—"}
									</span>
								</SummaryRow>
							</div>

							{/* Timing: Started + Completed side-by-side. Scheduled-for
							    remains full width since it's mutually exclusive with a
							    start time in practice. */}
							<div className="grid grid-cols-2 gap-4">
								<SummaryRow label="Started">
									<span className="break-words">
										{run.started_at
											? formatDate(run.started_at)
											: run.created_at
												? formatDate(run.created_at)
												: "—"}
									</span>
								</SummaryRow>
								<SummaryRow label="Completed">
									<span className="break-words">
										{run.completed_at ? formatDate(run.completed_at) : "—"}
									</span>
								</SummaryRow>
							</div>

							{run.status === "queued" && run.scheduled_at && (
								<SummaryRow label="Scheduled for">
									{formatDate(run.scheduled_at)}
								</SummaryRow>
							)}

							{/* Link to related run (validation ↔ patch) */}
							{run.validation_run_id && (
								<SummaryRow label="Validation run" icon={Shield}>
									<Link
										to={`/patching/runs/${run.validation_run_id}`}
										className="text-primary-600 dark:text-primary-400 hover:underline"
									>
										View validation output
									</Link>
								</SummaryRow>
							)}

							{/* Patch policy: keep in the sidebar alongside the other
							    run metadata so the right column stays focused on the
							    live output. */}
							{(hasPolicy || showDefaultPolicy) && (
								<SummaryRow label="Patch policy" icon={Shield}>
									<div className="flex items-start gap-2 p-3 -mx-1 rounded-md bg-secondary-50 dark:bg-secondary-700/40 border border-secondary-200 dark:border-secondary-600">
										<div className="min-w-0 space-y-0.5">
											{hasPolicy ? (
												<>
													<p className="text-sm font-medium text-secondary-900 dark:text-white break-words">
														{run.policy_name ?? "Patch policy"}
													</p>
													{policyDetail && (
														<p className="text-xs text-secondary-500 dark:text-secondary-400">
															{policyDetail}
														</p>
													)}
												</>
											) : (
												<>
													<p className="text-sm font-medium text-secondary-900 dark:text-white">
														Default policy
													</p>
													<p className="text-xs text-secondary-500 dark:text-secondary-400 italic">
														Runs immediately on trigger
													</p>
												</>
											)}
										</div>
									</div>
								</SummaryRow>
							)}

							{/* Packages affected: belongs with the rest of the run
							    metadata. Scrollable when the dependency list is long
							    so the sidebar stays bounded. */}
							{run.packages_affected?.length > 0 && (
								<SummaryRow
									label={`Packages affected (${run.packages_affected.length})`}
								>
									<div className="max-h-48 overflow-auto pr-1 text-sm text-secondary-700 dark:text-secondary-300">
										<PackageNameList
											packages={run.packages_affected}
											showIcon={false}
										/>
									</div>
								</SummaryRow>
							)}
						</dl>
					</div>
				</aside>

				{/* Primary content column */}
				<div className="lg:col-span-2 space-y-6 min-w-0">
					{/* State banners: explain the current run state. Action buttons
					    (Approve & Patch / Skip & Patch / Retry Validation / Stop Run)
					    live in the page header to keep the shell output area roomy. */}
					{run.status === "pending_validation" && (
						<div className="rounded-lg bg-warning-50 dark:bg-warning-900/30 border border-warning-200 dark:border-warning-600 p-4 flex items-start gap-3">
							<AlertTriangle className="h-5 w-5 text-warning-600 dark:text-warning-400 mt-0.5 flex-shrink-0" />
							<div className="min-w-0">
								<p className="text-sm font-medium text-warning-800 dark:text-warning-200">
									Validation pending — host may be offline
								</p>
								<p className="text-xs text-warning-700 dark:text-warning-300 mt-0.5">
									The dry-run has not completed. You can retry when the host is
									back online, or skip validation to patch immediately.
								</p>
							</div>
						</div>
					)}

					{run.status === "pending_approval" && (
						<div className="rounded-lg bg-warning-50 dark:bg-warning-900/30 border border-warning-200 dark:border-warning-600 p-4 flex items-start gap-3">
							<AlertTriangle className="h-5 w-5 text-warning-600 dark:text-warning-400 mt-0.5 flex-shrink-0" />
							<div className="min-w-0">
								<p className="text-sm font-medium text-warning-800 dark:text-warning-200">
									Awaiting approval
								</p>
								<p className="text-xs text-warning-700 dark:text-warning-300 mt-0.5">
									{run.patch_type === "patch_all"
										? "This Patch All run was submitted for approval and hasn't been executed yet. Approve to queue it, or delete it from Runs & History."
										: "This run was submitted for approval without a dry-run. Approve to queue it, or delete it from Runs & History."}
								</p>
							</div>
						</div>
					)}

					{run.status === "validated" && (
						<div className="rounded-lg bg-warning-50 dark:bg-warning-900/30 border border-warning-200 dark:border-warning-600 p-4 flex items-start gap-3">
							<AlertTriangle className="h-5 w-5 text-warning-600 dark:text-warning-400 mt-0.5 flex-shrink-0" />
							<div className="min-w-0">
								<p className="text-sm font-medium text-warning-800 dark:text-warning-200">
									Validation complete — approval required
								</p>
								<p className="text-xs text-warning-700 dark:text-warning-300 mt-0.5">
									{hasExtraDeps
										? `This run will install ${run.packages_affected.length} packages including additional dependencies. Review the output and approve to proceed.`
										: "Review the dry-run output and approve to proceed with patching."}
								</p>
							</div>
						</div>
					)}

					{/* Error panel (§16.1) */}
					{run.error_message && (
						<div className="bg-danger-50 border border-danger-200 rounded-md p-4 dark:bg-danger-900/30 dark:border-danger-700">
							<div className="flex">
								<AlertTriangle className="h-5 w-5 text-danger-400 flex-shrink-0" />
								<div className="ml-3 min-w-0 flex-1">
									<h3 className="text-sm font-medium text-danger-800 dark:text-danger-200">
										Run reported an error
									</h3>
									<pre className="mt-2 text-xs text-danger-700 dark:text-danger-300 font-mono whitespace-pre-wrap break-words">
										{run.error_message}
									</pre>
								</div>
							</div>
						</div>
					)}

					{/* Shell output (§3.4 content card + terminal chrome) */}
					<div className="card p-4 sm:p-6 flex flex-col">
						<div className="flex items-center justify-between gap-2 mb-4 flex-shrink-0 flex-wrap">
							<div className="flex items-center gap-2">
								<h3 className="text-lg font-medium text-secondary-900 dark:text-white">
									Shell output
								</h3>
								{isStreamOpen && run.status === "running" && (
									<span className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded text-xs font-medium bg-emerald-100 text-emerald-800 dark:bg-emerald-900/40 dark:text-emerald-300">
										<span className="h-1.5 w-1.5 rounded-full bg-emerald-500 animate-pulse" />
										Live
									</span>
								)}
							</div>
							<div className="flex items-center gap-2">
								{run.status !== "running" && run.status !== "queued" && (
									<button
										type="button"
										onClick={handleCopyOutput}
										className="btn-outline inline-flex items-center gap-1.5 min-h-[44px]"
										title="Copy shell output"
									>
										<Copy className="h-4 w-4" />
										<span className="hidden sm:inline">Copy output</span>
									</button>
								)}
							</div>
						</div>
						<div className="flex-1 min-h-0 rounded-lg border border-secondary-700 dark:border-secondary-600 bg-[#0d1117] dark:bg-black overflow-hidden shadow-inner">
							<pre
								ref={outputRef}
								className="block w-full h-[420px] max-h-[55vh] overflow-auto p-4 text-[13px] leading-relaxed font-mono text-[#e6edf3] whitespace-pre-wrap break-words"
								style={{
									fontFamily:
										"ui-monospace, SFMono-Regular, 'SF Mono', Menlo, Consolas, 'Liberation Mono', monospace",
								}}
							>
								{shellDisplay}
							</pre>
						</div>
						<p className="mt-2 text-xs text-secondary-500 dark:text-white/60">
							Output streams (stdout/stderr) from the patch run. Scroll to see
							full output.
						</p>
					</div>
				</div>
			</div>

			{/* Stop confirm modal (§9.1 + §9.4 warning badge) */}
			{stopConfirmOpen && (
				<div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4">
					<button
						type="button"
						onClick={() => {
							if (!stopRunMutation.isPending) setStopConfirmOpen(false);
						}}
						className="fixed inset-0 cursor-default"
						aria-label="Close"
					/>
					<div className="bg-white dark:bg-secondary-800 rounded-lg shadow-xl max-w-md w-full mx-4 relative z-10">
						<div className="px-6 py-4 border-b border-secondary-200 dark:border-secondary-600">
							<div className="flex items-center justify-between">
								<h3 className="text-lg font-medium text-secondary-900 dark:text-white flex items-center gap-3">
									<span className="w-10 h-10 bg-danger-100 dark:bg-danger-900 rounded-full flex items-center justify-center flex-shrink-0">
										<AlertTriangle className="h-5 w-5 text-danger-600 dark:text-danger-400" />
									</span>
									Stop this patch run?
								</h3>
								<button
									type="button"
									onClick={() => setStopConfirmOpen(false)}
									disabled={stopRunMutation.isPending}
									className="p-1 rounded hover:bg-secondary-100 dark:hover:bg-secondary-700 text-secondary-400 hover:text-secondary-600 disabled:opacity-50 disabled:cursor-not-allowed"
									aria-label="Close dialog"
								>
									<X className="h-5 w-5" />
								</button>
							</div>
						</div>
						<div className="px-6 py-4 space-y-3">
							<p className="text-sm text-secondary-600 dark:text-white/80">
								The agent will be asked to interrupt the current command.
								Partially-installed packages may leave the host in an
								intermediate state; the agent will report back its final output
								and an updated inventory when it exits.
							</p>
							<div className="p-3 bg-danger-50 dark:bg-danger-900/40 border border-danger-200 dark:border-danger-700 rounded-md">
								<p className="text-sm text-danger-700 dark:text-danger-300">
									This action cannot be undone.
								</p>
							</div>
							{stopError && (
								<div className="p-3 bg-danger-50 dark:bg-danger-900/40 border border-danger-300 dark:border-danger-700 rounded-md text-sm text-danger-700 dark:text-danger-200">
									{stopError}
								</div>
							)}
						</div>
						<div className="px-6 py-4 border-t border-secondary-200 dark:border-secondary-600 flex justify-end gap-3">
							<button
								type="button"
								onClick={() => setStopConfirmOpen(false)}
								disabled={stopRunMutation.isPending}
								className="btn-outline min-h-[44px] disabled:opacity-50 disabled:cursor-not-allowed"
							>
								Cancel
							</button>
							<button
								type="button"
								onClick={handleConfirmStop}
								disabled={stopRunMutation.isPending}
								className="btn-danger inline-flex items-center gap-1.5 min-h-[44px] disabled:opacity-50 disabled:cursor-not-allowed"
							>
								{stopRunMutation.isPending ? (
									<RefreshCw className="h-4 w-4 animate-spin" />
								) : (
									<Square className="h-4 w-4" />
								)}
								{stopRunMutation.isPending ? "Stopping…" : "Stop Run"}
							</button>
						</div>
					</div>
				</div>
			)}
		</div>
	);
};

export default RunDetail;
