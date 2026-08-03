import { useQueries, useQueryClient } from "@tanstack/react-query";
import {
	AlertTriangle,
	ArrowLeft,
	ArrowRight,
	Calendar,
	Check,
	CheckSquare,
	ChevronDown,
	ChevronRight,
	ChevronUp,
	Clock,
	Eye,
	RefreshCw,
	Send,
	Server,
	Shield,
	Square,
	Terminal,
	Wrench,
	X,
	Zap,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useToast } from "../contexts/ToastContext";
import { formatDate, packagesAPI } from "../utils/api";
import { patchingAPI, pollDryRunUntilDone } from "../utils/patchingApi";
import { extraDependencies } from "../utils/patchRun";

const DRY_RUN_PACKAGE_LIMIT = 5;
const VALIDATION_CONCURRENCY = 5;

// Verb used across the outcome UI so approval and patch flows read right.
const submitVerbFor = (mode) => (mode === "approval" ? "submitted" : "queued");

// Approve mode submits one existing run at a time, trigger mode one host.
const pluraliseTarget = (n, isApprove) =>
	isApprove ? (n === 1 ? "run" : "runs") : n === 1 ? "host" : "hosts";

// Run an async worker over items with a bounded concurrency pool so large
// batches don't serialize (one-at-a-time) or fan out uncapped (stampede).
async function runWithConcurrency(items, limit, worker) {
	const queue = [...items];
	const workers = Array.from(
		{ length: Math.min(limit, queue.length) },
		async () => {
			while (queue.length) {
				const item = queue.shift();
				await worker(item);
			}
		},
	);
	await Promise.all(workers);
}

/**
 * Unified patch wizard. Every place in the app that initiates patching funnels
 * through this component for UI consistency.
 *
 * The wizard has up to six steps, rendered conditionally based on what is
 * already known when the wizard opens:
 *
 *   1. hosts      Select hosts (hidden when lockHosts=true, i.e. caller knows
 *                 the target host(s) already).
 *   2. packages   Select packages (only shown when multiple packages came in
 *                 and can be trimmed; hidden when lockPackages=true or when
 *                 patchType="patch_all").
 *   3. validate   Dry-run validation per host (trigger + patch_package only;
 *                 patch_all cannot dry-run server-side and approve mode
 *                 already has a validation run on file).
 *   4. timing     Per-host policy override + scheduled-at preview.
 *   5. approval   Decide whether to approve now or submit for approval later
 *                 (trigger + patch_package only — the only mode where a
 *                 pending-validation run can be left for a second approver).
 *   6. submit     Final per-host summary + the fire button.
 *
 * The visible steps list collapses down to just the steps that actually apply,
 * so e.g. a single-host Patch-all shows only Timing and Submit.
 *
 * Props:
 *   isOpen, onClose, onSuccess - standard modal contract. onSuccess fires
 *                      once per session with every run that was actually
 *                      created: onSuccess(mode, { runs, deferred }). After a
 *                      partial failure the wizard stays open and defers that
 *                      call until the user retries or closes, so the caller
 *                      never sees a run twice and never misses one.
 *                      `deferred` is true when the report was flushed on
 *                      close. Do not navigate away on it: the user asked to
 *                      close the wizard, not to be taken elsewhere.
 *   mode               "trigger" | "approve" (default "trigger")
 *   patchType          "patch_all" | "patch_package" (default "patch_package")
 *   packageNames       string[] | null - required for patch_package
 *   lockHosts          bool - hide the hosts step; hosts come from presetHosts
 *   lockPackages       bool - do not allow editing packages (informational)
 *   presetHosts        [{ id, friendly_name, hostname }] required when lockHosts.
 *                      In approve mode the same host may appear more than once,
 *                      one entry per pending run.
 *   validationRunIds   string[] - required when mode="approve"; each index maps
 *                      to the same index in presetHosts and is the existing
 *                      validation (or pending_validation) run to approve.
 *   restrictHostIds    Set|Array - (trigger/patch_package only) restricts the
 *                      host discovery step to these host IDs.
 *   initialStep        number - override starting step (auto-clamped to
 *                      enabled steps). Rarely needed; steps skip-ahead
 *                      automatically based on what props pin down.
 *   packagesByRun      { [runId]: string[] } - optional, approve-mode bulk.
 *                      When provided, the Submit step renders the per-run
 *                      list from this map instead of the global packageNames.
 *   patchTypeByRun     { [runId]: "patch_all" | "patch_package" } - optional,
 *                      approve-mode bulk. Drives the per-run render in the
 *                      Submit step; patch_all rows show a summary instead of
 *                      a packages table.
 */
export default function PatchWizard({
	isOpen,
	onClose,
	onSuccess,
	mode = "trigger",
	patchType = "patch_package",
	packageNames,
	lockHosts = false,
	lockPackages = false,
	presetHosts,
	validationRunIds,
	restrictHostIds,
	initialStep = 0,
	packagesByRun,
	patchTypeByRun,
}) {
	const queryClient = useQueryClient();
	const { warning: toastWarning } = useToast();
	const isApprove = mode === "approve";
	const isPatchAll = patchType === "patch_all";
	// "Packages are fixed" means the Select Packages step has no useful
	// decision to make: either there's no finite package list at all
	// (patch_all), the server already has one on file (approve mode), the
	// caller explicitly locked the list, or there's only a single package
	// and deselecting it would just empty the wizard. In those cases we
	// skip the step entirely rather than showing a rubber-stamp page.
	const packagesAreFixed =
		isPatchAll || isApprove || lockPackages || (packageNames?.length || 0) <= 1;

	// --- Stable session identity ------------------------------------------
	// Callers very commonly pass inline array/object literals for presetHosts,
	// validationRunIds, packageNames etc. Those are a fresh reference on every
	// parent render, which would make any reset effect keyed on them fire
	// constantly (e.g. every react-query refetch) and wipe in-flight wizard
	// state. We collapse the content into a single string key and only reset
	// when that key changes.
	//
	// We compute the key inline (not via useMemo) because it's a cheap string
	// join and using it as a raw primitive dep is what makes the reset effect
	// actually stable — a memoized object would still be re-created every
	// render due to the same upstream identity thrash.
	const sessionKey = `${mode}|${patchType}|${lockHosts ? "LH" : ""}|${
		lockPackages ? "LP" : ""
	}|H:${(presetHosts || []).map((h) => h?.id || "").join(",")}|R:${(
		validationRunIds || []
	).join(",")}|P:${(packageNames || []).join("\n")}`;

	// Build the list of enabled steps from props. We use string IDs so the
	// render code can branch on them without relying on index math, which
	// would change every time a step is dropped.
	//
	// The "ANYWHERE patching is initiated must go through this wizard" rule
	// means we keep the same step *sequence* everywhere, but hide steps that
	// have no decision to make for a given entry point. That way the user
	// never has to click through a rubber-stamp page, but the mental model
	// stays identical across flows.
	// Canonical 6-step sequence, always computed. Each step carries a
	// `hidden` flag so the indicator shows the full sequence with
	// auto-skipped steps visually muted (UX consistency across flows),
	// while navigation only advances through the visible ones.
	const allSteps = useMemo(
		() => [
			{ id: "hosts", label: "Hosts", hidden: lockHosts },
			{ id: "packages", label: "Packages", hidden: packagesAreFixed },
			{
				id: "validate",
				label: "Validate",
				// Validate only makes sense for a fresh trigger of a package
				// patch. patch_all cannot dry-run (backend rejects it) and
				// approve mode already has a validation result on file.
				hidden: isApprove || isPatchAll,
			},
			{ id: "timing", label: "Timing", hidden: false },
			{
				id: "approval",
				label: "Approval",
				// Approval is a real decision for any fresh trigger: the user
				// can run it now, or submit it for a second approver to
				// release later. In approve mode the user already chose to
				// approve, so we skip the step (still shown muted in the
				// indicator so the 6-step mental model stays consistent).
				hidden: isApprove,
			},
			{ id: "submit", label: "Submit", hidden: false },
		],
		[lockHosts, isApprove, isPatchAll, packagesAreFixed],
	);
	const steps = useMemo(() => allSteps.filter((s) => !s.hidden), [allSteps]);

	const clampedInitialStep = Math.min(
		Math.max(initialStep, 0),
		Math.max(steps.length - 1, 0),
	);

	const [step, setStep] = useState(clampedInitialStep);
	const [selectedTargetIds, setSelectedTargetIds] = useState(new Set());
	// User-editable package selection for the Select Packages step. Starts
	// populated with everything the caller passed in; when the step is hidden
	// (packagesAreFixed) this always equals packageNames and is effectively
	// read-only throughout the wizard.
	const [selectedPackageNames, setSelectedPackageNames] = useState(
		() => packageNames || [],
	);
	// Approval decision captured in the Approval step. "approve_now" runs the
	// triggers (or approves existing validation runs) as normal; "submit" is
	// only meaningful when validation produced pending runs we can leave for
	// a second approver to act on later. Default is approve_now because that
	// matches the existing behaviour of Queue & patch.
	const [approvalDecision, setApprovalDecision] = useState("approve_now");

	// --- Host discovery (only used when !lockHosts) ---
	// Fetch hosts for each package in parallel when we need to discover them.
	// Always filter to hosts that need this update - patching an up-to-date host
	// for a specific package has no effect and just clutters the wizard.
	const discoverHostsEnabled = !lockHosts && !isPatchAll && !!packageNames;
	const hostQueries = useQueries({
		queries: discoverHostsEnabled
			? (packageNames || []).map((pkgName) => ({
					queryKey: ["package-hosts-for-patch", pkgName],
					queryFn: () =>
						packagesAPI
							.getHosts(encodeURIComponent(pkgName), {
								limit: 500,
								needsUpdate: true,
							})
							.then((res) => res.data?.hosts || []),
					enabled: isOpen && !!pkgName,
				}))
			: [],
	});

	// Normalise restrictHostIds to a Set we can consult cheaply in the memo below.
	const restrictSet = useMemo(() => {
		if (!restrictHostIds) return null;
		if (restrictHostIds instanceof Set) {
			return restrictHostIds.size > 0 ? restrictHostIds : null;
		}
		if (Array.isArray(restrictHostIds)) {
			return restrictHostIds.length > 0 ? new Set(restrictHostIds) : null;
		}
		return null;
	}, [restrictHostIds]);

	// The unit the wizard selects, submits and renders. Trigger mode makes one
	// submission per host, so a target is a host. Approve mode approves one
	// existing run at a time and a host can hold several pending runs, so a
	// target is a run. `id` is the selection/result/React key; `hostId` is
	// always the real host and is what the API is called with.
	const targets = useMemo(() => {
		if (lockHosts) {
			const preset = presetHosts || [];
			if (isApprove) {
				return preset.map((h, i) => {
					const runId = validationRunIds?.[i];
					return {
						id: runId || h.id,
						hostId: h.id,
						runId,
						friendly_name: h.friendly_name,
						hostname: h.hostname,
					};
				});
			}
			return preset.map((h) => ({
				id: h.id,
				hostId: h.id,
				friendly_name: h.friendly_name,
				hostname: h.hostname,
			}));
		}
		const byId = new Map();
		for (const q of hostQueries) {
			const list = q.data || [];
			for (const h of list) {
				const osType = (h.os_type || h.osType || "").toLowerCase();
				if (osType.includes("windows")) continue;
				const id = h.hostId || h.host_id || h.id;
				if (!id) continue;
				if (restrictSet && !restrictSet.has(id)) continue;
				if (!byId.has(id)) {
					byId.set(id, {
						id,
						hostId: id,
						friendly_name: h.friendly_name || h.friendlyName,
						hostname: h.hostname,
					});
				}
			}
		}
		return Array.from(byId.values()).sort((a, b) =>
			(a.friendly_name || a.hostname || a.id || "").localeCompare(
				b.friendly_name || b.hostname || b.id || "",
			),
		);
	}, [
		lockHosts,
		isApprove,
		presetHosts,
		validationRunIds,
		hostQueries,
		restrictSet,
	]);

	const isLoading = !lockHosts && hostQueries.some((q) => q.isLoading);
	const hasInitialized = useRef(false);

	// Reset wizard state only when the *content* of the job changes - not on
	// every parent re-render. sessionKey collapses the props the wizard cares
	// about into a single primitive string, so react-query refetches and
	// unrelated parent re-renders no longer wipe in-flight wizard state.
	// biome-ignore lint/correctness/useExhaustiveDependencies: sessionKey is the intentional single identity
	useEffect(() => {
		hasInitialized.current = false;
		setSelectedTargetIds(new Set());
		setSelectedPackageNames(packageNames || []);
		setStep(clampedInitialStep);
		setValidationByTarget({});
		setError(null);
		setExpandedDepsByTarget({});
		setPolicyOverrides({});
		setApprovalDecision("approve_now");
		setSubmitResultByTarget({});
		setSubmitMode(null);
		hasReported.current = false;
	}, [sessionKey, clampedInitialStep]);

	// Pre-select everything we can — but only *once* per session. When hosts
	// are locked, the parent typically re-creates the presetHosts array each
	// render; without the hasInitialized guard we'd overwrite the user's
	// deselections on every parent re-render. The session-key reset effect
	// above clears hasInitialized when the job actually changes.
	useEffect(() => {
		if (hasInitialized.current) return;
		if (lockHosts) {
			if (targets.length > 0) {
				hasInitialized.current = true;
				setSelectedTargetIds(new Set(targets.map((t) => t.id)));
			}
			return;
		}
		if (!isLoading && targets.length > 0) {
			hasInitialized.current = true;
			setSelectedTargetIds(new Set(targets.map((t) => t.id)));
		}
	}, [lockHosts, isLoading, targets]);

	const toggleTarget = (targetId) => {
		setSelectedTargetIds((prev) => {
			const next = new Set(prev);
			if (next.has(targetId)) next.delete(targetId);
			else next.add(targetId);
			return next;
		});
	};

	const selectAll = () =>
		setSelectedTargetIds(new Set(targets.map((t) => t.id)));
	const selectNone = () => setSelectedTargetIds(new Set());

	const [isPatching, setIsPatching] = useState(false);
	const [error, setError] = useState(null);
	const [validationByTarget, setValidationByTarget] = useState({});
	const [isValidating, setIsValidating] = useState(false);
	const [expandedOutput, setExpandedOutput] = useState(null);
	// Per-target policy override: targetId -> "default" | policyId | "immediate"
	const [policyOverrides, setPolicyOverrides] = useState({});
	// Per-target: which rows have dependencies expanded in the Submit step
	const [expandedDepsByTarget, setExpandedDepsByTarget] = useState({});
	// Per-target outcome of the submit attempts:
	//   targetId -> { status: "success" | "failed", hostId, runId, immediate, error }
	// A target that already produced a run is never submitted again, so a retry
	// after a partial failure cannot duplicate runs or hit the backend status
	// guard on an already-approved validation.
	const [submitResultByTarget, setSubmitResultByTarget] = useState({});
	const [submitMode, setSubmitMode] = useState(null);
	const hasReported = useRef(false);

	const submittedRuns = useMemo(
		() =>
			Object.values(submitResultByTarget)
				.filter((r) => r?.status === "success" && r.runId)
				.map((r) => ({
					hostId: r.hostId,
					runId: r.runId,
					immediate: !!r.immediate,
				})),
		[submitResultByTarget],
	);

	const succeededTargetIds = useMemo(
		() =>
			Array.from(selectedTargetIds).filter(
				(id) => submitResultByTarget[id]?.status === "success",
			),
		[selectedTargetIds, submitResultByTarget],
	);
	const failedTargetIds = useMemo(
		() =>
			Array.from(selectedTargetIds).filter(
				(id) => submitResultByTarget[id]?.status === "failed",
			),
		[selectedTargetIds, submitResultByTarget],
	);

	// Report every run that really exists on the backend, then close. Used by
	// the clean-submit path and by every close path after a partial failure so
	// successful runs are never orphaned.
	const finishAndClose = useCallback(
		(resultMap, modeOverride, deferred = true) => {
			if (!hasReported.current) {
				hasReported.current = true;
				const runs = Object.values(resultMap)
					.filter((r) => r?.status === "success" && r.runId)
					.map((r) => ({
						hostId: r.hostId,
						runId: r.runId,
						immediate: !!r.immediate,
					}));
				const reported = modeOverride || submitMode;
				onSuccess?.(reported === "approval" ? "approval" : "patch", {
					runs,
					deferred,
				});
			}
			onClose?.();
		},
		[onSuccess, onClose, submitMode],
	);

	const handleClose = useCallback(() => {
		if (submittedRuns.length > 0) {
			finishAndClose(submitResultByTarget);
			return;
		}
		onClose?.();
	}, [submittedRuns, submitResultByTarget, finishAndClose, onClose]);

	const currentStepId = steps[step]?.id;
	// Find the current interactive step's index within allSteps so the
	// indicator can paint everything before it as "done" and everything
	// after as "pending", while still drawing the auto-skipped steps in
	// their canonical slot.
	const currentAllStepIdx = useMemo(
		() => allSteps.findIndex((s) => s.id === currentStepId),
		[allSteps, currentStepId],
	);

	// We only need per-host policy previews from the Timing step onwards.
	// Gating the query by step keeps early steps snappy on slow networks.
	const needsPolicyData =
		currentStepId === "timing" ||
		currentStepId === "approval" ||
		currentStepId === "submit";

	// Per-host scheduling preview (default policy, next run time) for the
	// Timing and Submit steps.
	const selectedTargets = useMemo(
		() => targets.filter((t) => selectedTargetIds.has(t.id)),
		[targets, selectedTargetIds],
	);

	// How many selected hosts already have a pending validation run we could
	// leave for a second approver. Shown as informational context in the
	// Approval step — "N of M hosts have been validated". Submit-for-approval
	// is viable for every host (patch_all and patch_package without a
	// validation run simply create a new pending run on the backend), so we
	// don't gate the feature on this count anymore.
	const approvableRunCount = useMemo(() => {
		let n = 0;
		for (const id of selectedTargetIds) {
			const v = validationByTarget[id];
			if (
				v?.patch_run_id &&
				(v.status === "validated" || v.status === "pending_validation")
			) {
				n += 1;
			}
		}
		return n;
	}, [selectedTargetIds, validationByTarget]);
	// Submit for approval is always a real choice for any fresh trigger.
	// patch_all and patch_package without a prior dry-run create a new
	// "pending_approval" run; patch_package after validation reuses the
	// existing "pending_validation" or "validated" run. Either way the
	// approver can release them later from Runs & History.
	const canSubmitForApproval = !isApprove && selectedTargetIds.size > 0;

	// If the user previously picked "submit" but that option is no longer
	// viable (e.g. they went back and changed host selection), fold the
	// decision back to "approve_now" so footer labels and confirm logic
	// stay consistent. An effect keeps the render pure.
	useEffect(() => {
		if (approvalDecision === "submit" && !canSubmitForApproval) {
			setApprovalDecision("approve_now");
		}
	}, [approvalDecision, canSubmitForApproval]);

	// Clamp the current step index if the visible steps array shrinks (e.g.
	// the caller switches mode and we lose a step). Prevents rendering
	// a blank panel when steps[step] becomes undefined.
	useEffect(() => {
		if (step > steps.length - 1) {
			setStep(Math.max(steps.length - 1, 0));
		}
	}, [steps.length, step]);

	// Close on Escape, matching the rest of the app's modals. We intentionally
	// ignore ESC while the user is mid-submit or mid-validate so they can't
	// dismiss and miss a half-finished action.
	useEffect(() => {
		if (!isOpen) return;
		const onKeyDown = (e) => {
			if (e.key !== "Escape") return;
			if (isPatching || isValidating) return;
			e.stopPropagation();
			handleClose();
		};
		window.addEventListener("keydown", onKeyDown);
		return () => window.removeEventListener("keydown", onKeyDown);
	}, [isOpen, isPatching, isValidating, handleClose]);

	// --- Navigation helpers ----------------------------------------------
	// goNext/goBack walk the step list, skipping past steps that are
	// currently showing a single option (Approval when submit isn't viable).
	// Keeping skip logic in one place means the forward and back buttons
	// stay in sync.
	const isStepInteractive = useCallback(
		(id) => {
			if (id === "approval" && !canSubmitForApproval) return false;
			return true;
		},
		[canSubmitForApproval],
	);
	const goNext = useCallback(() => {
		setStep((s) => {
			let next = s + 1;
			while (next < steps.length && !isStepInteractive(steps[next]?.id)) {
				next += 1;
			}
			return Math.min(next, steps.length - 1);
		});
	}, [steps, isStepInteractive]);
	const goBack = useCallback(() => {
		setStep((s) => {
			let prev = s - 1;
			while (prev > 0 && !isStepInteractive(steps[prev]?.id)) {
				prev -= 1;
			}
			return Math.max(prev, 0);
		});
	}, [steps, isStepInteractive]);

	// The preview is a property of the host, and two approve targets can share
	// one, so query distinct hosts and fan the result back out per target.
	const previewHostIds = useMemo(
		() => Array.from(new Set(selectedTargets.map((t) => t.hostId))),
		[selectedTargets],
	);
	const previewQueries = useQueries({
		queries: previewHostIds.map((hostId) => ({
			queryKey: ["patching-preview-run", hostId],
			queryFn: () => patchingAPI.getPreviewRun(hostId),
			staleTime: 30 * 1000,
			enabled: needsPolicyData,
		})),
	});
	const previewByTarget = useMemo(() => {
		const byHost = {};
		previewHostIds.forEach((hostId, i) => {
			if (previewQueries[i]?.data) {
				byHost[hostId] = previewQueries[i].data;
			}
		});
		const map = {};
		for (const t of selectedTargets) {
			if (byHost[t.hostId]) map[t.id] = byHost[t.hostId];
		}
		return map;
	}, [previewQueries, previewHostIds, selectedTargets]);

	// selectedPkgs is what the user actually wants to patch (after the Select
	// Packages step has had a chance to trim the list). For callers that
	// don't expose that step (single-package flows, patch_all, approve) it
	// simply mirrors the packageNames prop.
	const selectedPkgs = selectedPackageNames;

	// Host-discovery inversion: hostQueries[i] is the list of hosts where
	// packageNames[i] is installed and outdated. Invert that into a map of
	// hostId -> [pkg names applicable to this host], intersected with the
	// user's current selection. This lets Validate, Submit, and the actual
	// trigger calls target only the packages each host actually needs,
	// instead of blasting the union list at every host. When host discovery
	// isn't active (lockHosts, patch_all, approve) we return null and
	// downstream code falls back to selectedPkgs.
	const discoveredPackagesByHost = useMemo(() => {
		if (!discoverHostsEnabled) return null;
		const selectedSet = new Set(selectedPkgs);
		const map = {};
		(packageNames || []).forEach((pkgName, i) => {
			if (!selectedSet.has(pkgName)) return;
			const list = hostQueries[i]?.data || [];
			for (const h of list) {
				const id = h.hostId || h.host_id || h.id;
				if (!id) continue;
				if (!map[id]) map[id] = [];
				map[id].push(pkgName);
			}
		});
		return map;
	}, [discoverHostsEnabled, packageNames, hostQueries, selectedPkgs]);

	// Effective packages-per-target used for display and requests:
	//   1. Caller-provided packagesByRun wins (approve-mode bulk).
	//   2. Otherwise use the discovered map.
	//   3. Fall back to selectedPkgs for hosts we couldn't resolve
	//      (e.g. lockHosts flows where host discovery didn't run).
	const packagesForTarget = useCallback(
		(target) => {
			const runPkgs = target?.runId ? packagesByRun?.[target.runId] : undefined;
			if (runPkgs !== undefined) return runPkgs;
			const discovered = discoveredPackagesByHost?.[target?.hostId];
			if (discovered) return discovered;
			return selectedPkgs;
		},
		[packagesByRun, discoveredPackagesByHost, selectedPkgs],
	);

	const canValidate =
		!isApprove &&
		!isPatchAll &&
		selectedPkgs.length > 0 &&
		selectedPkgs.length <= DRY_RUN_PACKAGE_LIMIT;

	const handleValidate = async () => {
		if (selectedTargets.length === 0) return;
		setIsValidating(true);
		// Seed every selected target as "pending" so the UI can render a live
		// per-row list immediately, instead of a single batch-wide spinner.
		const seeded = {};
		for (const target of selectedTargets) {
			seeded[target.id] = {
				status: "pending",
				packages_affected: [],
				shell_output: "",
			};
		}
		setValidationByTarget(seeded);

		await runWithConcurrency(
			selectedTargets,
			VALIDATION_CONCURRENCY,
			async (target) => {
				setValidationByTarget((prev) => ({
					...prev,
					[target.id]: {
						...(prev[target.id] || {}),
						status: "validating",
					},
				}));
				try {
					// Send only the packages this host actually has outdated —
					// the wizard already knows from host discovery, so there's
					// no point asking a host to dry-run packages it doesn't have.
					const hostPkgs = packagesForTarget(target);
					const res = await patchingAPI.trigger(
						target.hostId,
						"patch_package",
						null,
						hostPkgs,
						{ dry_run: true },
					);
					const runId = res?.patch_run_id;
					if (!runId) {
						setValidationByTarget((prev) => ({
							...prev,
							[target.id]: {
								status: "failed",
								packages_affected: [],
								shell_output: "",
								error: "No run ID returned",
							},
						}));
						return;
					}
					const result = await pollDryRunUntilDone(runId);
					setValidationByTarget((prev) => ({
						...prev,
						[target.id]: { ...result, patch_run_id: runId },
					}));
				} catch (err) {
					setValidationByTarget((prev) => ({
						...prev,
						[target.id]: {
							status: "failed",
							packages_affected: [],
							shell_output: "",
							error: err.response?.data?.error || err.message,
						},
					}));
				}
			},
		);

		setIsValidating(false);
	};

	const handleConfirm = async () => {
		if (selectedTargets.length === 0) {
			setError("Select at least one host");
			return;
		}
		// "Submit for approval" is available in any fresh-trigger flow. The
		// wizard either reuses an existing pending_validation / validated run
		// (if the user ran validation in step 3) or asks the backend to
		// create a new pending_approval run via trigger({ pending_approval:
		// true }). Approve mode ignores the radio — it's already an approval
		// action.
		const userChoseSubmit = !isApprove && approvalDecision === "submit";
		const nextSubmitMode = userChoseSubmit ? "approval" : "patch";
		const targetHasPendingRun = (id) => {
			const v = validationByTarget[id];
			return (
				!!v?.patch_run_id &&
				(v.status === "validated" || v.status === "pending_validation")
			);
		};

		// Only targets without a run of their own are submitted, so retrying a
		// partial failure never fires a second request at a target we already
		// patched.
		const pending = selectedTargets.filter(
			(t) => submitResultByTarget[t.id]?.status !== "success",
		);
		if (pending.length === 0) {
			finishAndClose(submitResultByTarget);
			return;
		}

		// One target, one request. Throws on failure so the caller records the
		// target as failed and carries on with the rest of the batch.
		const submitTarget = async (target) => {
			if (userChoseSubmit) {
				// Reuse an existing pending validation run when we have one —
				// otherwise we'd leave two pending rows on the same host (one
				// from validation, one from pending_approval).
				if (targetHasPendingRun(target.id)) {
					return {
						runId: validationByTarget[target.id].patch_run_id,
						immediate: false,
					};
				}
				// Fresh pending run. patch_all has no package list;
				// patch_package needs the same package set that would have
				// been sent on a normal trigger — scoped to the packages this
				// specific host actually has outdated.
				const response = await patchingAPI.trigger(
					target.hostId,
					patchType,
					null,
					isPatchAll ? null : packagesForTarget(target),
					{ pending_approval: true },
				);
				if (!response?.patch_run_id) {
					throw new Error("The server did not return a run for this host");
				}
				return { runId: response.patch_run_id, immediate: false };
			}

			const override = policyOverrides[target.id];
			// The backend currently only recognises "immediate" as a valid
			// schedule_override; any other value (including "default" or a
			// policy ID) is silently ignored. Only send the override on the
			// wire when it actually does something — sending "default" was a
			// no-op but looked like an instruction.
			const overrideBody =
				override === "immediate" ? { schedule_override: "immediate" } : {};

			let response;
			if (isApprove) {
				// Approve mode: the target *is* an existing validation run.
				// Approving creates a linked execution run on the backend.
				if (!target.runId) {
					throw new Error("No validation run on file for this host");
				}
				response = await patchingAPI.approveRun(target.runId, overrideBody);
			} else if (isPatchAll) {
				response = await patchingAPI.trigger(
					target.hostId,
					"patch_all",
					null,
					null,
					overrideBody,
				);
			} else {
				// Trigger mode, package patch. If a validation run exists from
				// Step 2 we approve it (this marks the validation as
				// "approved" and links a new execution run). Otherwise we fire
				// a fresh trigger.
				const validation = validationByTarget[target.id];
				const validationRunId = validation?.patch_run_id;
				if (
					validationRunId &&
					(validation.status === "validated" ||
						validation.status === "pending_validation")
				) {
					response = await patchingAPI.approveRun(
						validationRunId,
						overrideBody,
					);
				} else {
					response = await patchingAPI.trigger(
						target.hostId,
						"patch_package",
						null,
						packagesForTarget(target),
						overrideBody,
					);
				}
			}

			if (!response?.patch_run_id) {
				throw new Error("The server did not return a run for this host");
			}

			// Derive "immediate" from the server's run_at: the backend is the
			// source of truth for when the run will fire, regardless of
			// policy/override negotiation on the frontend. If the response
			// carries run_at within ~5s of now we treat it as immediate;
			// otherwise it's scheduled.
			const runAtMs = response.run_at
				? Date.parse(response.run_at)
				: Number.NaN;
			const immediate = Number.isFinite(runAtMs)
				? runAtMs - Date.now() <= 5000
				: override === "immediate";
			return { runId: response.patch_run_id, immediate };
		};

		setError(null);
		setSubmitMode(nextSubmitMode);
		setIsPatching(true);

		const attempt = {};
		for (const target of pending) {
			try {
				const result = await submitTarget(target);
				attempt[target.id] = {
					status: "success",
					hostId: target.hostId,
					...result,
				};
			} catch (err) {
				attempt[target.id] = {
					status: "failed",
					hostId: target.hostId,
					error: err?.response?.data?.error || err?.message || "Request failed",
				};
			}
		}

		const merged = { ...submitResultByTarget, ...attempt };
		setSubmitResultByTarget(merged);
		setIsPatching(false);

		// Runs created before the failure are real, so refresh the shared
		// patching views regardless of how the rest of the batch went.
		const createdCount = Object.values(attempt).filter(
			(r) => r.status === "success",
		).length;
		if (createdCount > 0) {
			queryClient.invalidateQueries({ queryKey: ["patching-runs"] });
			queryClient.invalidateQueries({ queryKey: ["patching-dashboard"] });
		}

		const failedCount = selectedTargets.filter(
			(t) => merged[t.id]?.status === "failed",
		).length;
		if (failedCount === 0) {
			finishAndClose(merged, nextSubmitMode, false);
			return;
		}
		if (createdCount > 0) {
			const verb = submitVerbFor(nextSubmitMode);
			toastWarning(
				`${createdCount} ${pluraliseTarget(createdCount, isApprove)} ${verb}, ${failedCount} failed. Review the wizard to retry.`,
			);
		}
	};

	// Determine if any target has extra dependencies. The requested set is
	// per-target because in multi-host multi-package flows each host only gets
	// sent the subset of packages it actually has outdated; using the global
	// selectedPkgs here would hide real extra-deps and surface false ones for
	// any host that was sent a narrower list.
	const targetsWithExtraDeps = useMemo(() => {
		return targets.filter((t) => {
			const v = validationByTarget[t.id];
			if (!v || v.status !== "validated") return false;
			return (
				extraDependencies({
					package_names: packagesForTarget(t),
					packages_affected: v.packages_affected,
				}).length > 0
			);
		});
	}, [validationByTarget, targets, packagesForTarget]);

	const TERMINAL_VALIDATION_STATUSES = ["validated", "failed", "timeout"];
	// Derived from selectedTargets, the same source handleValidate seeds from.
	// A selection id with no matching target would otherwise never be seeded.
	const validationDone =
		selectedTargets.length > 0 &&
		selectedTargets.every((t) =>
			TERMINAL_VALIDATION_STATUSES.includes(validationByTarget[t.id]?.status),
		);

	// Aggregate counts for the live progress summary in the Validate step.
	const validationProgress = useMemo(() => {
		const counts = {
			total: selectedTargets.length,
			clean: 0,
			extraDeps: 0,
			failed: 0,
			offline: 0,
			pending: 0,
			validating: 0,
			done: 0,
		};
		for (const target of selectedTargets) {
			const v = validationByTarget[target.id];
			const status = v?.status;
			if (status === "validated") {
				counts.done += 1;
				const hasExtra =
					extraDependencies({
						package_names: selectedPkgs,
						packages_affected: v.packages_affected,
					}).length > 0;
				if (hasExtra) counts.extraDeps += 1;
				else counts.clean += 1;
			} else if (status === "failed") {
				counts.done += 1;
				counts.failed += 1;
			} else if (status === "timeout") {
				counts.done += 1;
				counts.offline += 1;
			} else if (status === "validating") {
				counts.validating += 1;
			} else {
				counts.pending += 1;
			}
		}
		return counts;
	}, [selectedTargets, validationByTarget, selectedPkgs]);

	if (!isOpen) return null;

	// Derive header copy from mode + patchType + current selection so every
	// entry point shows something accurate without callers having to pass a
	// title string. When hosts are locked to a single target we surface its
	// friendly name/hostname to give the user a strong "what am I about to
	// do to *which* machine?" anchor.
	const singleLockedHostLabel =
		lockHosts && selectedTargets.length === 1
			? selectedTargets[0].friendly_name || selectedTargets[0].hostname || null
			: null;
	const headerTitle = isApprove
		? selectedTargetIds.size > 1
			? `Approve ${selectedTargetIds.size} patch runs`
			: singleLockedHostLabel
				? `Approve patch run on ${singleLockedHostLabel}`
				: "Approve patch run"
		: isPatchAll
			? singleLockedHostLabel
				? `Patch all packages on ${singleLockedHostLabel}`
				: "Patch all packages"
			: singleLockedHostLabel && selectedPkgs.length > 0
				? `Patch ${selectedPkgs.length} package${
						selectedPkgs.length === 1 ? "" : "s"
					} on ${singleLockedHostLabel}`
				: "Patch packages on hosts";

	const submitVerb = submitVerbFor(submitMode);

	// Submit-step button label varies with mode + approval decision.
	const confirmButtonLabel = () => {
		if (isPatching) return "Working…";
		if (failedTargetIds.length > 0) {
			return `Retry ${failedTargetIds.length} failed ${pluraliseTarget(
				failedTargetIds.length,
				isApprove,
			)}`;
		}
		const n = selectedTargetIds.size;
		if (isApprove) {
			return n === 1 ? "Approve & patch" : `Approve & patch ${n} runs`;
		}
		if (approvalDecision === "submit") {
			return n === 1 ? "Submit for approval" : `Submit ${n} hosts for approval`;
		}
		return `Queue & patch ${n} host${n !== 1 ? "s" : ""}`;
	};

	// Step-indicator visibility. We render the full canonical 6-step sequence
	// for consistency across entry points (the user sees the same mental
	// model regardless of mode), with auto-skipped steps visually muted. The
	// only case we suppress the indicator is when the wizard has a single
	// step to show — then it's effectively a plain confirm dialog.
	const showStepIndicator = steps.length > 1;

	return (
		<div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
			<button
				type="button"
				onClick={() => !isPatching && !isValidating && handleClose()}
				className="fixed inset-0 cursor-default"
				aria-label="Close modal"
			/>
			<div className="bg-white dark:bg-secondary-800 rounded-lg shadow-xl max-w-2xl w-full mx-4 max-h-[92vh] flex flex-col relative z-10">
				{/* Header */}
				<div className="px-6 py-4 border-b border-secondary-200 dark:border-secondary-600 shrink-0">
					<div
						className={`flex items-center justify-between ${
							showStepIndicator ? "mb-3" : ""
						}`}
					>
						<h3 className="text-lg font-medium text-secondary-900 dark:text-white flex items-center gap-2">
							<Wrench className="h-5 w-5 text-primary-600" />
							{headerTitle}
						</h3>
						<button
							type="button"
							onClick={() => !isPatching && !isValidating && handleClose()}
							className="p-1 rounded hover:bg-secondary-100 dark:hover:bg-secondary-700 text-secondary-400 hover:text-secondary-600"
						>
							<X className="h-5 w-5" />
						</button>
					</div>
					{/* Step indicator. We render the full 6-step sequence so users
					    get a consistent mental model across every patching entry
					    point; auto-skipped steps are drawn muted. `flex-wrap`
					    keeps long labels from overflowing on narrow viewports. */}
					{showStepIndicator && (
						<div className="flex flex-wrap items-center gap-y-1 gap-x-1 text-xs">
							{allSteps.map((s, i) => {
								const isHidden = s.hidden;
								const isCurrent = !isHidden && s.id === currentStepId;
								const isDone =
									currentAllStepIdx >= 0 && i < currentAllStepIdx && !isHidden;
								const cls = isHidden
									? "bg-transparent text-secondary-400 dark:text-secondary-500 italic line-through opacity-60"
									: isCurrent
										? "bg-primary-100 text-primary-700 dark:bg-primary-900 dark:text-primary-300"
										: isDone
											? "bg-green-100 text-green-700 dark:bg-green-900 dark:text-green-300"
											: "bg-secondary-100 text-secondary-500 dark:bg-secondary-700 dark:text-secondary-400";
								return (
									<div
										key={s.id}
										className="flex items-center gap-1"
										title={
											isHidden ? `${s.label} — not required here` : s.label
										}
									>
										<div
											className={`flex items-center gap-1 px-2 py-0.5 rounded font-medium ${cls}`}
										>
											{isDone ? (
												<Check className="h-3 w-3" />
											) : (
												<span>{i + 1}</span>
											)}
											{s.label}
										</div>
										{i < allSteps.length - 1 && (
											<ChevronRight className="h-3 w-3 text-secondary-400" />
										)}
									</div>
								);
							})}
						</div>
					)}
				</div>

				{/* Body */}
				<div className="px-6 py-4 overflow-y-auto flex-1 min-h-0">
					{/* Mode banner. A tiny, persistent hint so the user never loses
					    track of what "kind" of wizard they're in — especially in
					    approve mode, which reuses almost all the same chrome as a
					    fresh trigger but has very different consequences. */}
					{isApprove && (
						<div className="mb-3 flex items-start gap-2 rounded-lg bg-primary-50 dark:bg-primary-900/20 border border-primary-200 dark:border-primary-800 px-3 py-2">
							<Check className="h-4 w-4 text-primary-600 dark:text-primary-300 mt-0.5 shrink-0" />
							<p className="text-xs text-primary-700 dark:text-primary-200">
								Approving {selectedTargetIds.size || "existing"} validated run
								{selectedTargetIds.size !== 1 ? "s" : ""}. Packages and hosts
								are fixed from the validation; you can still adjust timing.
							</p>
						</div>
					)}

					{/* Step 1: Select hosts */}
					{currentStepId === "hosts" &&
						(isLoading ? (
							<div className="flex items-center gap-2 text-secondary-600 dark:text-secondary-400 py-8">
								<RefreshCw className="h-4 w-4 animate-spin shrink-0" />
								Loading hosts…
							</div>
						) : targets.length === 0 ? (
							<p className="text-sm text-secondary-600 dark:text-secondary-400 py-4">
								{restrictSet
									? "None of the selected hosts have a pending update for these packages."
									: hostQueries.some((q) => (q.data || []).length > 0)
										? "These packages are only pending on Windows hosts. Patching is not supported for Windows."
										: "No hosts have a pending update for these packages."}
							</p>
						) : (
							<>
								<p className="text-sm text-secondary-600 dark:text-secondary-400 mb-3">
									Select the hosts you want to deploy{" "}
									<strong>{packageNames?.length || 0}</strong> package
									{packageNames?.length !== 1 ? "s" : ""} to.
								</p>
								<div className="flex items-center gap-2 mb-3">
									{/* Select all/none only earn their keep when there's
									    more than one host to toggle. With a single host the
									    row checkbox says everything. */}
									{targets.length > 1 && (
										<>
											<button
												type="button"
												onClick={selectAll}
												className="text-xs text-primary-600 dark:text-primary-400 hover:underline"
											>
												Select all
											</button>
											<span className="text-secondary-400">|</span>
											<button
												type="button"
												onClick={selectNone}
												className="text-xs text-primary-600 dark:text-primary-400 hover:underline"
											>
												Select none
											</button>
										</>
									)}
									<span className="text-sm text-secondary-500 dark:text-secondary-400 ml-auto">
										{selectedTargetIds.size} of {targets.length} selected
									</span>
								</div>
								<div className="border border-secondary-200 dark:border-secondary-600 rounded-lg overflow-hidden max-h-64 overflow-y-auto mb-4">
									<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-600">
										<thead className="bg-secondary-50 dark:bg-secondary-700 sticky top-0">
											<tr>
												<th className="w-10 px-3 py-2" />
												<th className="px-3 py-2 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider">
													Host
												</th>
											</tr>
										</thead>
										<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-600">
											{targets.map((target) => (
												<tr
													key={target.id}
													className="hover:bg-secondary-50 dark:hover:bg-secondary-700"
												>
													<td className="px-3 py-2">
														<button
															type="button"
															onClick={() => toggleTarget(target.id)}
															className="flex items-center justify-center w-full"
														>
															{selectedTargetIds.has(target.id) ? (
																<CheckSquare className="h-4 w-4 text-primary-600" />
															) : (
																<Square className="h-4 w-4 text-secondary-400" />
															)}
														</button>
													</td>
													<td className="px-3 py-2 text-sm text-secondary-900 dark:text-white flex items-center gap-2">
														<Server className="h-4 w-4 text-secondary-400 shrink-0" />
														{target.friendly_name ||
															target.hostname ||
															target.hostId}
													</td>
												</tr>
											))}
										</tbody>
									</table>
								</div>
								{packageNames?.length > 0 && (
									<div className="border-t border-secondary-200 dark:border-secondary-600 pt-3">
										<p className="text-xs font-medium text-secondary-500 dark:text-secondary-400 mb-1.5">
											Packages to patch:
										</p>
										<ul className="text-sm text-secondary-600 dark:text-secondary-400 list-disc list-inside space-y-0.5 max-h-16 overflow-y-auto">
											{packageNames.map((name) => (
												<li key={name}>{name}</li>
											))}
										</ul>
									</div>
								)}
							</>
						))}

					{/* Step 2: Validate */}
					{currentStepId === "validate" && (
						<>
							<p className="text-sm text-secondary-600 dark:text-secondary-400 mb-4">
								Run a dry-run validation to see exactly which packages will be
								updated on each host before committing.
							</p>
							{!isValidating &&
								Object.keys(validationByTarget).length === 0 && (
									<div className="flex items-center justify-center py-8">
										<button
											type="button"
											onClick={handleValidate}
											disabled={selectedTargetIds.size === 0}
											className="btn-primary inline-flex items-center gap-2"
										>
											<Eye className="h-4 w-4" />
											Run validation
										</button>
									</div>
								)}
							{(isValidating || Object.keys(validationByTarget).length > 0) && (
								<div className="space-y-3">
									{/* Live progress summary */}
									<div className="rounded-lg border border-secondary-200 dark:border-secondary-700 bg-secondary-50 dark:bg-secondary-900/40 px-3 py-2 text-sm">
										<div className="flex items-center gap-2 flex-wrap">
											{isValidating ? (
												<RefreshCw className="h-4 w-4 animate-spin text-primary-600 shrink-0" />
											) : (
												<Check className="h-4 w-4 text-green-600 shrink-0" />
											)}
											<span className="font-medium text-secondary-800 dark:text-secondary-200">
												Validated {validationProgress.done} /{" "}
												{validationProgress.total}
											</span>
											<span className="text-secondary-500 dark:text-secondary-400">
												·
											</span>
											<span className="text-green-700 dark:text-green-400">
												{validationProgress.clean} clean
											</span>
											{validationProgress.extraDeps > 0 && (
												<>
													<span className="text-secondary-500 dark:text-secondary-400">
														·
													</span>
													<span className="text-amber-700 dark:text-amber-400">
														{validationProgress.extraDeps} extra deps
													</span>
												</>
											)}
											{validationProgress.failed > 0 && (
												<>
													<span className="text-secondary-500 dark:text-secondary-400">
														·
													</span>
													<span className="text-red-700 dark:text-red-400">
														{validationProgress.failed} failed
													</span>
												</>
											)}
											{validationProgress.offline > 0 && (
												<>
													<span className="text-secondary-500 dark:text-secondary-400">
														·
													</span>
													<span className="text-secondary-600 dark:text-secondary-400">
														{validationProgress.offline} offline
													</span>
												</>
											)}
											{validationProgress.validating > 0 && (
												<>
													<span className="text-secondary-500 dark:text-secondary-400">
														·
													</span>
													<span className="text-primary-700 dark:text-primary-400">
														{validationProgress.validating} validating
													</span>
												</>
											)}
											{validationProgress.pending > 0 && (
												<>
													<span className="text-secondary-500 dark:text-secondary-400">
														·
													</span>
													<span className="text-secondary-600 dark:text-secondary-400">
														{validationProgress.pending} pending
													</span>
												</>
											)}
										</div>
									</div>

									{validationDone && targetsWithExtraDeps.length > 0 && (
										<div className="flex items-start gap-2 p-3 rounded-lg bg-amber-50 dark:bg-amber-900/30 border border-amber-200 dark:border-amber-700">
											<AlertTriangle className="h-4 w-4 text-amber-600 dark:text-amber-400 mt-0.5 shrink-0" />
											<p className="text-sm text-amber-700 dark:text-amber-300">
												<strong>
													{targetsWithExtraDeps.length} host
													{targetsWithExtraDeps.length !== 1 ? "s" : ""}
												</strong>{" "}
												will install additional dependencies beyond the
												requested packages. Review below before proceeding.
											</p>
										</div>
									)}

									{selectedTargets.map((target) => {
										const v = validationByTarget[target.id] || {
											status: "pending",
										};
										const hostLabel =
											target.friendly_name || target.hostname || target.hostId;
										const extraDeps = extraDependencies({
											package_names: selectedPkgs,
											packages_affected: v.packages_affected,
										});
										const extraSet = new Set(
											extraDeps.map((p) => p.toLowerCase()),
										);
										const isExpanded = expandedOutput === target.id;
										const isTerminal = TERMINAL_VALIDATION_STATUSES.includes(
											v.status,
										);

										const borderClass =
											v.status === "pending"
												? "border-secondary-200 dark:border-secondary-700"
												: v.status === "validating"
													? "border-primary-200 dark:border-primary-700"
													: extraDeps.length > 0
														? "border-amber-300 dark:border-amber-600"
														: v.status === "validated"
															? "border-green-200 dark:border-green-700"
															: v.status === "timeout"
																? "border-secondary-200 dark:border-secondary-700"
																: "border-red-200 dark:border-red-700";
										const headerBgClass =
											v.status === "pending"
												? "bg-secondary-50 dark:bg-secondary-900/40"
												: v.status === "validating"
													? "bg-primary-50 dark:bg-primary-900/20"
													: extraDeps.length > 0
														? "bg-amber-50 dark:bg-amber-900/30"
														: v.status === "validated"
															? "bg-green-50 dark:bg-green-900/20"
															: v.status === "timeout"
																? "bg-secondary-100 dark:bg-secondary-800/60"
																: "bg-red-50 dark:bg-red-900/20";

										return (
											<div
												key={target.id}
												className={`rounded-lg border text-sm overflow-hidden ${borderClass}`}
											>
												<div
													className={`px-3 py-2 flex items-center gap-2 ${headerBgClass}`}
												>
													<Server className="h-4 w-4 text-secondary-400 shrink-0" />
													<span className="font-medium text-secondary-800 dark:text-secondary-200 flex-1">
														{hostLabel}
													</span>
													{v.status === "pending" && (
														<span className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded bg-secondary-100 text-secondary-600 dark:bg-secondary-700 dark:text-secondary-300">
															<Clock className="h-3 w-3" />
															Pending
														</span>
													)}
													{v.status === "validating" && (
														<span className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded bg-primary-100 text-primary-700 dark:bg-primary-800 dark:text-primary-200">
															<RefreshCw className="h-3 w-3 animate-spin" />
															Validating…
														</span>
													)}
													{v.status === "validated" && extraDeps.length > 0 && (
														<span className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded bg-amber-100 text-amber-700 dark:bg-amber-800 dark:text-amber-200">
															<AlertTriangle className="h-3 w-3" />
															{extraDeps.length} extra dep
															{extraDeps.length !== 1 ? "s" : ""}
														</span>
													)}
													{v.status === "validated" &&
														extraDeps.length === 0 && (
															<span className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded bg-green-100 text-green-700 dark:bg-green-800 dark:text-green-200">
																<Check className="h-3 w-3" />
																Clean
															</span>
														)}
													{v.status === "failed" && (
														<span className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded bg-red-100 text-red-700 dark:bg-red-800 dark:text-red-200">
															Failed
														</span>
													)}
													{v.status === "timeout" && (
														<span className="text-xs px-2 py-0.5 rounded bg-secondary-100 text-secondary-600 dark:bg-secondary-700 dark:text-secondary-300">
															Offline
														</span>
													)}
												</div>
												{isTerminal && (
													<div className="px-3 py-2 bg-white dark:bg-secondary-800">
														{v.error ? (
															<p className="text-amber-600 dark:text-amber-400 text-xs">
																{v.error}
															</p>
														) : v.packages_affected?.length > 0 ? (
															<>
																<p className="text-secondary-600 dark:text-secondary-300 text-xs mb-1">
																	{v.packages_affected.length} package
																	{v.packages_affected.length !== 1 ? "s" : ""}{" "}
																	will be updated:
																</p>
																<ul className="text-xs text-secondary-600 dark:text-secondary-400 list-disc list-inside space-y-0.5 max-h-20 overflow-y-auto">
																	{v.packages_affected.map((p) => (
																		<li
																			key={p}
																			className={
																				extraSet.has(p.toLowerCase())
																					? "text-amber-600 dark:text-amber-400 font-medium"
																					: ""
																			}
																		>
																			{p}
																			{extraSet.has(p.toLowerCase()) && (
																				<span className="ml-1 text-amber-500 dark:text-amber-400">
																					(dependency)
																				</span>
																			)}
																		</li>
																	))}
																</ul>
															</>
														) : (
															<p className="text-xs text-secondary-500 dark:text-secondary-400">
																No additional packages.
															</p>
														)}
														{v.shell_output && (
															<button
																type="button"
																onClick={() =>
																	setExpandedOutput(
																		isExpanded ? null : target.id,
																	)
																}
																className="mt-2 text-xs text-primary-600 dark:text-primary-400 hover:underline inline-flex items-center gap-1"
															>
																<Terminal className="h-3 w-3" />
																{isExpanded ? "Hide" : "Show"} validation output
															</button>
														)}
														{isExpanded && v.shell_output && (
															<pre className="mt-2 p-2 rounded bg-[#0d1117] text-[#e6edf3] text-[11px] font-mono max-h-48 overflow-auto whitespace-pre-wrap break-words">
																{v.shell_output}
															</pre>
														)}
													</div>
												)}
											</div>
										);
									})}

									{validationDone && !isValidating && (
										<button
											type="button"
											onClick={handleValidate}
											className="text-xs text-primary-600 dark:text-primary-400 hover:underline inline-flex items-center gap-1"
										>
											<RefreshCw className="h-3 w-3" />
											Re-run validation
										</button>
									)}
								</div>
							)}
						</>
					)}

					{/* Select Packages step: only rendered when the caller gave us
					    more than one package and didn't lock them. */}
					{currentStepId === "packages" && (
						<div className="space-y-3">
							<p className="text-sm text-secondary-600 dark:text-secondary-400">
								Choose which packages to include in this patch run.
							</p>
							<div className="flex items-center gap-2 text-xs">
								<button
									type="button"
									onClick={() => setSelectedPackageNames(packageNames || [])}
									className="text-primary-600 dark:text-primary-400 hover:underline"
								>
									Select all
								</button>
								<span className="text-secondary-400">|</span>
								<button
									type="button"
									onClick={() => setSelectedPackageNames([])}
									className="text-primary-600 dark:text-primary-400 hover:underline"
								>
									Select none
								</button>
								<span className="ml-auto text-sm text-secondary-500 dark:text-secondary-400">
									{selectedPackageNames.length} of {packageNames?.length || 0}{" "}
									selected
								</span>
							</div>
							<div className="border border-secondary-200 dark:border-secondary-600 rounded-lg overflow-hidden max-h-64 overflow-y-auto">
								<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-600">
									<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-100 dark:divide-secondary-700">
										{(packageNames || []).map((name) => {
											const checked = selectedPackageNames.includes(name);
											return (
												<tr
													key={name}
													className="hover:bg-secondary-50 dark:hover:bg-secondary-700"
												>
													<td className="w-10 px-3 py-2">
														<button
															type="button"
															onClick={() =>
																setSelectedPackageNames((prev) =>
																	prev.includes(name)
																		? prev.filter((n) => n !== name)
																		: [...prev, name],
																)
															}
															className="flex items-center justify-center w-full"
														>
															{checked ? (
																<CheckSquare className="h-4 w-4 text-primary-600" />
															) : (
																<Square className="h-4 w-4 text-secondary-400" />
															)}
														</button>
													</td>
													<td className="px-3 py-2 text-sm font-mono text-secondary-900 dark:text-white">
														{name}
													</td>
												</tr>
											);
										})}
									</tbody>
								</table>
							</div>
							{selectedPackageNames.length > DRY_RUN_PACKAGE_LIMIT && (
								<p className="text-xs text-amber-700 dark:text-amber-400 flex items-center gap-1">
									<AlertTriangle className="h-3.5 w-3.5" />
									{DRY_RUN_PACKAGE_LIMIT} package limit for dry-run validation.
									The next step will let you skip validation and queue the patch
									directly.
								</p>
							)}
						</div>
					)}

					{/* Timing Policy step: per-host policy override + scheduled-at
					    preview. No package details here - this page is about
					    "when" not "what". */}
					{currentStepId === "timing" && (
						<div className="space-y-4">
							<p className="text-sm text-secondary-600 dark:text-secondary-400">
								Choose when this patch should run on each host. By default each
								host follows its assigned policy; you can override to run
								immediately instead.
							</p>

							{/* Bulk controls — flips every selected host at once. Only
							    shown when there's more than one host (no point otherwise). */}
							{selectedTargets.length > 1 && (
								<div className="flex flex-wrap items-center gap-2 rounded-lg border border-dashed border-secondary-300 dark:border-secondary-600 px-3 py-2 bg-secondary-50/50 dark:bg-secondary-800/40">
									<span className="text-xs font-medium text-secondary-500 dark:text-secondary-400">
										Apply to all hosts:
									</span>
									<button
										type="button"
										onClick={() => {
											const next = {};
											for (const t of selectedTargets) next[t.id] = "default";
											setPolicyOverrides(next);
										}}
										className="text-xs px-2 py-1 rounded border border-secondary-300 dark:border-secondary-600 hover:bg-white dark:hover:bg-secondary-700"
									>
										Use host policy
									</button>
									<button
										type="button"
										onClick={() => {
											const next = {};
											for (const t of selectedTargets) next[t.id] = "immediate";
											setPolicyOverrides(next);
										}}
										className="text-xs px-2 py-1 rounded border border-secondary-300 dark:border-secondary-600 hover:bg-white dark:hover:bg-secondary-700"
									>
										Run immediately
									</button>
								</div>
							)}

							{selectedTargets.map((target) => {
								const preview = previewByTarget[target.id];
								const override = policyOverrides[target.id] || "default";
								const scheduledAt =
									override === "immediate"
										? "Now"
										: preview?.run_at_iso
											? formatDate(preview.run_at_iso)
											: preview
												? "Immediately"
												: "\u2014";
								const hostLabel =
									target.friendly_name || target.hostname || target.hostId;
								return (
									<div
										key={target.id}
										className="border border-secondary-200 dark:border-secondary-600 rounded-lg overflow-hidden"
									>
										<div className="bg-secondary-50 dark:bg-secondary-700 px-4 py-2 flex items-center gap-2">
											<Server className="h-4 w-4 text-secondary-500 shrink-0" />
											<span className="font-medium text-secondary-800 dark:text-white text-sm flex-1">
												{hostLabel}
											</span>
										</div>
										<div className="px-4 py-3 bg-white dark:bg-secondary-800">
											<div className="flex flex-wrap gap-4 items-start">
												<div className="flex-1 min-w-0">
													<p className="text-xs font-medium text-secondary-500 dark:text-secondary-400 mb-1 flex items-center gap-1">
														<Shield className="h-3 w-3" />
														Patch policy
													</p>
													{/*
													 * Only two options are truly honoured by the backend
													 * right now: keep the host's assigned policy, or
													 * override to "immediate". Surfacing arbitrary
													 * policy IDs here would silently no-op and create
													 * a "I picked Policy X but it ran on the default"
													 * bug, so we intentionally keep the choice binary.
													 */}
													<select
														value={override}
														onChange={(e) =>
															setPolicyOverrides((prev) => ({
																...prev,
																[target.id]: e.target.value,
															}))
														}
														className="w-full text-sm rounded border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-800 text-secondary-900 dark:text-white py-1 px-2"
													>
														<option value="default">
															{preview?.policy_name
																? `Use host policy (${preview.policy_name})`
																: "Use host policy (Default)"}
														</option>
														<option value="immediate">
															Override: Run immediately
														</option>
													</select>
													{preview?.policy_name && (
														<p className="mt-1 text-[11px] text-secondary-500 dark:text-secondary-400">
															To use a different policy, change the host's
															assigned policy in Patching → Policies.
														</p>
													)}
												</div>
												<div className="shrink-0">
													<p className="text-xs font-medium text-secondary-500 dark:text-secondary-400 mb-1 flex items-center gap-1">
														<Calendar className="h-3 w-3" />
														Scheduled at
													</p>
													<div className="flex items-center gap-1 text-sm text-secondary-700 dark:text-secondary-300">
														<Clock className="h-3.5 w-3.5 text-secondary-400" />
														{override === "immediate" ? (
															<span className="text-green-600 dark:text-green-400 font-medium">
																Immediately
															</span>
														) : (
															scheduledAt
														)}
													</div>
												</div>
											</div>
										</div>
									</div>
								);
							})}
						</div>
					)}

					{/* Approval step: decide whether to run this now or leave a
					    pending run for someone else to approve later. Available
					    for every fresh-trigger flow (including patch_all — the
					    backend creates a pending_approval row for it without
					    executing anything on the host). Only hidden in approve
					    mode, where the decision is already made. */}
					{currentStepId === "approval" && (
						<div className="space-y-4">
							<p className="text-sm text-secondary-600 dark:text-secondary-400">
								How should this patch be released?
							</p>
							{/* Helpful context for the submit-for-approval choice:
							    tell the user how many of the selected hosts have
							    already been validated, since that changes whether
							    we reuse an existing pending run or create one. */}
							{!isPatchAll && selectedTargetIds.size > 0 && (
								<div className="text-xs text-secondary-500 dark:text-secondary-400">
									{approvableRunCount === selectedTargetIds.size
										? "All selected hosts have a pending validation run — submit will reuse them."
										: approvableRunCount > 0
											? `${approvableRunCount} of ${selectedTargetIds.size} selected hosts already validated; the remaining ${selectedTargetIds.size - approvableRunCount} will be queued as pending approval.`
											: "No validation was run — submit will create pending approval runs directly."}
								</div>
							)}
							{isPatchAll && (
								<div className="text-xs text-secondary-500 dark:text-secondary-400">
									Patch All doesn't use dry-run validation, so submit will
									create a pending approval run per host for a second approver
									to release later.
								</div>
							)}
							<div className="grid sm:grid-cols-2 gap-3">
								<button
									type="button"
									onClick={() => setApprovalDecision("approve_now")}
									className={`text-left border rounded-lg p-4 transition-colors ${
										approvalDecision === "approve_now"
											? "border-primary-500 bg-primary-50 dark:bg-primary-900/20"
											: "border-secondary-200 dark:border-secondary-600 hover:border-secondary-400"
									}`}
								>
									<div className="flex items-center gap-2 mb-1">
										<Zap className="h-4 w-4 text-primary-600" />
										<span className="font-medium text-secondary-900 dark:text-white">
											Approve now
										</span>
									</div>
									<p className="text-xs text-secondary-600 dark:text-secondary-400">
										Queue the patch to run as soon as the selected timing
										allows.
									</p>
								</button>
								<button
									type="button"
									onClick={() =>
										canSubmitForApproval && setApprovalDecision("submit")
									}
									disabled={!canSubmitForApproval}
									aria-describedby={
										!canSubmitForApproval
											? "submit-for-approval-help"
											: undefined
									}
									className={`text-left border rounded-lg p-4 transition-colors ${
										!canSubmitForApproval
											? "border-secondary-200 dark:border-secondary-700 opacity-50 cursor-not-allowed"
											: approvalDecision === "submit"
												? "border-primary-500 bg-primary-50 dark:bg-primary-900/20"
												: "border-secondary-200 dark:border-secondary-600 hover:border-secondary-400"
									}`}
								>
									<div className="flex items-center gap-2 mb-1">
										<Send className="h-4 w-4 text-amber-500" />
										<span className="font-medium text-secondary-900 dark:text-white">
											Submit for approval
										</span>
									</div>
									<p
										id={
											!canSubmitForApproval
												? "submit-for-approval-help"
												: undefined
										}
										className="text-xs text-secondary-600 dark:text-secondary-400"
									>
										{canSubmitForApproval
											? "Create a pending run for a second approver. Review and release later from Runs & History."
											: selectedTargetIds.size === 0
												? "Select at least one host first."
												: "Not available in approve mode."}
									</p>
								</button>
							</div>
						</div>
					)}

					{/* Submit step: final per-host summary + fire button. */}
					{currentStepId === "submit" && (
						<div className="space-y-4">
							{/* Partial-failure outcome. Successful runs already exist on
							    the backend, so we say so explicitly rather than showing a
							    single error banner that implies nothing happened. */}
							{failedTargetIds.length > 0 && (
								<div className="flex items-start gap-2 p-3 rounded-lg bg-danger-50 dark:bg-danger-900/30 border border-danger-200 dark:border-danger-700">
									<AlertTriangle className="h-4 w-4 text-danger-600 dark:text-danger-400 mt-0.5 shrink-0" />
									<div className="text-sm text-danger-700 dark:text-danger-300">
										<p>
											<strong>
												{failedTargetIds.length}{" "}
												{pluraliseTarget(failedTargetIds.length, isApprove)}
											</strong>{" "}
											could not be {submitVerb}.
											{succeededTargetIds.length > 0 &&
												` ${succeededTargetIds.length} ${pluraliseTarget(
													succeededTargetIds.length,
													isApprove,
												)} ${
													succeededTargetIds.length !== 1 ? "were" : "was"
												} ${submitVerb} successfully and now appear${
													succeededTargetIds.length !== 1 ? "" : "s"
												} in Runs & History.`}
										</p>
										<p className="mt-1 text-xs">
											Retrying only re-submits the failed{" "}
											{pluraliseTarget(failedTargetIds.length, isApprove)}.
											{succeededTargetIds.length > 0
												? " Choose Done to keep the successful runs and close."
												: ""}
										</p>
									</div>
								</div>
							)}

							{/* Approve mode has no dry-run in the wizard, so the summary
							    band and the extra-deps callout are only meaningful for
							    trigger flows that passed through Validate. */}
							{!isApprove &&
								validationDone &&
								targetsWithExtraDeps.length > 0 && (
									<div className="flex items-start gap-2 p-3 rounded-lg bg-amber-50 dark:bg-amber-900/30 border border-amber-200 dark:border-amber-700">
										<AlertTriangle className="h-4 w-4 text-amber-600 dark:text-amber-400 mt-0.5 shrink-0" />
										<p className="text-sm text-amber-700 dark:text-amber-300">
											<strong>
												{targetsWithExtraDeps.length} host
												{targetsWithExtraDeps.length !== 1 ? "s" : ""}
											</strong>{" "}
											will install additional dependencies. Review each host
											below before confirming.
										</p>
									</div>
								)}

							{/* Offline-host warning: a host in `timeout` state did not
							    respond during validation, so the patch is likely to
							    fail/queue. Better to surface this once at the top than
							    to let the user discover it from a failed run later. */}
							{!isApprove &&
								(() => {
									const offline = selectedTargets.filter(
										(t) => validationByTarget[t.id]?.status === "timeout",
									);
									if (offline.length === 0) return null;
									return (
										<div className="flex items-start gap-2 p-3 rounded-lg bg-secondary-50 dark:bg-secondary-700/40 border border-secondary-200 dark:border-secondary-600">
											<AlertTriangle className="h-4 w-4 text-secondary-500 dark:text-secondary-300 mt-0.5 shrink-0" />
											<p className="text-sm text-secondary-700 dark:text-secondary-200">
												<strong>
													{offline.length} host
													{offline.length !== 1 ? "s" : ""}
												</strong>{" "}
												did not respond to validation and may be offline. The
												patch will queue but may not run until the host comes
												back online.
											</p>
										</div>
									);
								})()}

							{isApprove && (
								<p className="text-sm text-secondary-600 dark:text-secondary-400">
									You are approving {selectedTargetIds.size} existing run
									{selectedTargetIds.size !== 1 ? "s" : ""}. Review the summary
									for each host below before approving.
								</p>
							)}

							{!isApprove && approvalDecision === "submit" && (
								<div className="flex items-start gap-2 p-3 rounded-lg bg-amber-50 dark:bg-amber-900/30 border border-amber-200 dark:border-amber-700">
									<Send className="h-4 w-4 text-amber-600 dark:text-amber-400 mt-0.5 shrink-0" />
									<p className="text-sm text-amber-700 dark:text-amber-300">
										These runs will be left as <strong>pending approval</strong>
										. Nothing will be patched until a second approver reviews
										them in Runs & History.
									</p>
								</div>
							)}

							{lockPackages && selectedPkgs.length > 0 && !isPatchAll && (
								<p className="text-xs text-secondary-500 dark:text-secondary-400 italic">
									Packages are fixed for this action and cannot be edited here.
								</p>
							)}

							{/* One table per selected target */}
							{selectedTargets.map((target) => {
								const v = validationByTarget[target.id];
								const submitResult = submitResultByTarget[target.id];
								const preview = previewByTarget[target.id];
								// Per-target package list and patch type. Bulk-approve
								// callers pass packagesByRun; multi-host trigger flows
								// derive it from host discovery; single-host / lockHosts
								// flows fall through to the global selection. This is
								// the same source the validation and trigger calls use,
								// so the Submit step stays consistent with what the
								// server will actually be asked to patch.
								const hostPkgs = packagesForTarget(target);
								const hostPatchType =
									(target.runId ? patchTypeByRun?.[target.runId] : null) ||
									patchType;
								const hostIsPatchAll = hostPatchType === "patch_all";

								// Build package rows: requested packages first, then extra deps.
								// For patch_all we show a single informational row; there's no
								// finite list to display.
								const requestedPackages = hostPkgs;
								const extraDeps = extraDependencies({
									package_names: hostPkgs,
									packages_affected: v?.packages_affected,
								});

								// Policy display. With the Timing step trimmed to
								// default/immediate, any other value here is a stale
								// override from a previous session and collapses to
								// "default" for display purposes.
								const override = policyOverrides[target.id] || "default";
								const scheduledAt =
									override === "immediate"
										? "Now"
										: preview?.run_at_iso
											? formatDate(preview.run_at_iso)
											: preview
												? "Immediately"
												: "\u2014";

								const hostLabel =
									target.friendly_name || target.hostname || target.hostId;

								return (
									<div
										key={target.id}
										className="border border-secondary-200 dark:border-secondary-600 rounded-lg overflow-hidden"
									>
										{/* Host header */}
										<div className="bg-secondary-50 dark:bg-secondary-700 px-4 py-2 flex items-center gap-2">
											<Server className="h-4 w-4 text-secondary-500 shrink-0" />
											<span className="font-medium text-secondary-800 dark:text-white text-sm flex-1">
												{hostLabel}
											</span>
											{!isApprove && validationDone && extraDeps.length > 0 && (
												<span className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded bg-amber-100 text-amber-700 dark:bg-amber-800 dark:text-amber-200">
													<AlertTriangle className="h-3 w-3" />
													{extraDeps.length} extra dep
													{extraDeps.length !== 1 ? "s" : ""}
												</span>
											)}
											{!isApprove &&
												validationDone &&
												v?.status === "validated" &&
												extraDeps.length === 0 && (
													<span className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded bg-green-100 text-green-700 dark:bg-green-800 dark:text-green-200">
														<Check className="h-3 w-3" />
														Clean
													</span>
												)}
											{!isApprove && !validationDone && !hostIsPatchAll && (
												<span
													className="text-xs text-secondary-400 dark:text-secondary-300"
													title="Validation was skipped — dependency preview unavailable"
												>
													Validation skipped
												</span>
											)}
											{v?.status === "timeout" && (
												<span className="text-xs px-2 py-0.5 rounded bg-secondary-100 text-secondary-600 dark:bg-secondary-600 dark:text-secondary-300">
													Host offline
												</span>
											)}
											{submitResult?.status === "success" && (
												<span className="inline-flex items-center gap-1 text-xs font-medium px-2 py-0.5 rounded bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">
													<Check className="h-3 w-3" />
													{submitMode === "approval" ? "Submitted" : "Queued"}
												</span>
											)}
											{submitResult?.status === "failed" && (
												<span className="inline-flex items-center gap-1 text-xs font-medium px-2 py-0.5 rounded bg-danger-100 text-danger-800 dark:bg-danger-900 dark:text-danger-200">
													<AlertTriangle className="h-3 w-3" />
													Failed
												</span>
											)}
										</div>

										{/* Packages table or summary */}
										{hostIsPatchAll ? (
											<div className="px-4 py-3 bg-white dark:bg-secondary-800 text-sm text-secondary-700 dark:text-secondary-300">
												All pending system package updates on this host will be
												installed.
											</div>
										) : (
											<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-600">
												<thead className="bg-secondary-50/50 dark:bg-secondary-700/50">
													<tr>
														<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-400 uppercase tracking-wider w-1/2">
															Package
														</th>
														<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-400 uppercase tracking-wider">
															Type
														</th>
													</tr>
												</thead>
												<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-100 dark:divide-secondary-700">
													{/* Requested packages */}
													{requestedPackages.map((pkg) => (
														<tr key={pkg}>
															<td className="px-4 py-1.5 text-sm font-mono text-secondary-800 dark:text-secondary-200">
																{pkg}
															</td>
															<td className="px-4 py-1.5">
																<span className="text-xs text-secondary-500 dark:text-secondary-400">
																	Requested
																</span>
															</td>
														</tr>
													))}
													{/* Dependencies: expandable when many */}
													{extraDeps.length > 0 && (
														<>
															<tr className="bg-amber-50/40 dark:bg-amber-900/10">
																<td colSpan={2} className="px-4 py-0">
																	<button
																		type="button"
																		onClick={() =>
																			setExpandedDepsByTarget((prev) => ({
																				...prev,
																				[target.id]: !prev[target.id],
																			}))
																		}
																		className="w-full flex items-center gap-2 py-2 text-left text-sm text-amber-700 dark:text-amber-300 hover:bg-amber-100/50 dark:hover:bg-amber-900/20 transition-colors"
																	>
																		{expandedDepsByTarget[target.id] ? (
																			<ChevronUp className="h-4 w-4 shrink-0" />
																		) : (
																			<ChevronDown className="h-4 w-4 shrink-0" />
																		)}
																		<AlertTriangle className="h-3.5 w-3.5 shrink-0" />
																		{extraDeps.length} additional dependenc
																		{extraDeps.length !== 1 ? "ies" : "y"}
																	</button>
																</td>
															</tr>
															{expandedDepsByTarget[target.id] &&
																extraDeps.map((pkg) => (
																	<tr
																		key={pkg}
																		className="bg-amber-50/40 dark:bg-amber-900/10"
																	>
																		<td className="px-4 py-1.5 pl-8 text-sm font-mono text-amber-700 dark:text-amber-300">
																			{pkg}
																		</td>
																		<td className="px-4 py-1.5">
																			<span className="inline-flex items-center gap-1 text-xs text-amber-600 dark:text-amber-400">
																				<AlertTriangle className="h-3 w-3" />
																				Dependency
																			</span>
																		</td>
																	</tr>
																))}
														</>
													)}
													{/* No validation: show requested only */}
													{!validationDone &&
														requestedPackages.length === 0 && (
															<tr>
																<td
																	colSpan={2}
																	className="px-4 py-2 text-sm text-secondary-500 dark:text-secondary-400"
																>
																	Validation was skipped — dependencies will be
																	resolved at run time.
																</td>
															</tr>
														)}
												</tbody>
											</table>
										)}

										{/* Policy & schedule row. Read-only in the Submit step -
										    the user already made the choice on the Timing step
										    and Back is available if they want to change it. */}
										<div className="border-t border-secondary-200 dark:border-secondary-600 px-4 py-3 bg-secondary-50/50 dark:bg-secondary-700/30">
											<div className="flex flex-wrap gap-4 items-start">
												<div className="flex-1 min-w-0">
													<p className="text-xs font-medium text-secondary-500 dark:text-secondary-400 mb-1 flex items-center gap-1">
														<Shield className="h-3 w-3" />
														Patch policy
													</p>
													<p className="text-sm text-secondary-800 dark:text-secondary-200">
														{override === "immediate"
															? "Run immediately (override)"
															: preview?.policy_name
																? `Host policy: ${preview.policy_name}`
																: "Host policy: Default"}
													</p>
												</div>
												<div className="shrink-0">
													<p className="text-xs font-medium text-secondary-500 dark:text-secondary-400 mb-1 flex items-center gap-1">
														<Calendar className="h-3 w-3" />
														Scheduled at
													</p>
													<div className="flex items-center gap-1 text-sm text-secondary-700 dark:text-secondary-300">
														<Clock className="h-3.5 w-3.5 text-secondary-400" />
														{override === "immediate" ? (
															<span className="text-green-600 dark:text-green-400 font-medium">
																Immediately
															</span>
														) : (
															scheduledAt
														)}
													</div>
												</div>
											</div>
										</div>
										{submitResult?.status === "failed" && (
											<div className="border-t border-danger-200 dark:border-danger-700 bg-danger-50 dark:bg-danger-900/30 px-4 py-2">
												<p className="text-xs text-danger-700 dark:text-danger-300">
													{submitResult.error}
												</p>
											</div>
										)}
									</div>
								);
							})}

							{error && (
								<p className="text-sm text-danger-600 dark:text-danger-400">
									{error}
								</p>
							)}
						</div>
					)}
				</div>

				{/* Footer */}
				<div className="px-6 py-4 border-t border-secondary-200 dark:border-secondary-600 flex justify-between items-center shrink-0">
					<div className="flex gap-2">
						{step > 0 && (
							<button
								type="button"
								onClick={goBack}
								disabled={isPatching || isValidating}
								className="btn-outline inline-flex items-center gap-1"
							>
								<ArrowLeft className="h-4 w-4" />
								Back
							</button>
						)}
						<button
							type="button"
							onClick={handleClose}
							disabled={isPatching || isValidating}
							className="btn-outline"
						>
							{submittedRuns.length > 0 ? "Done" : "Cancel"}
						</button>
					</div>

					<div>
						{/* Hosts step: advance when a viable selection exists. */}
						{currentStepId === "hosts" && (
							<button
								type="button"
								disabled={selectedTargetIds.size === 0 || targets.length === 0}
								onClick={goNext}
								className="btn-primary inline-flex items-center gap-1.5 disabled:opacity-50 disabled:cursor-not-allowed"
							>
								Next
								<ArrowRight className="h-4 w-4" />
							</button>
						)}

						{/* Packages step. */}
						{currentStepId === "packages" && (
							<button
								type="button"
								disabled={selectedPackageNames.length === 0}
								onClick={goNext}
								className="btn-primary inline-flex items-center gap-1.5 disabled:opacity-50 disabled:cursor-not-allowed"
							>
								Next
								<ArrowRight className="h-4 w-4" />
							</button>
						)}

						{/* Validate step: validation is optional, Next advances
						    regardless of whether the user ran it. */}
						{currentStepId === "validate" && (
							<div className="flex gap-2">
								{!validationDone && !isValidating && canValidate && (
									<button
										type="button"
										onClick={goNext}
										className="btn-outline text-sm"
									>
										Skip validation
									</button>
								)}
								{validationDone && !isValidating && (
									<button
										type="button"
										onClick={goNext}
										className="btn-primary inline-flex items-center gap-1.5"
									>
										Next
										<ArrowRight className="h-4 w-4" />
									</button>
								)}
								{!canValidate && !validationDone && (
									<button
										type="button"
										onClick={goNext}
										className="btn-outline text-sm"
									>
										Skip (too many packages)
									</button>
								)}
							</div>
						)}

						{/* Timing step. */}
						{currentStepId === "timing" && (
							<button
								type="button"
								onClick={goNext}
								className="btn-primary inline-flex items-center gap-1.5"
							>
								Next
								<ArrowRight className="h-4 w-4" />
							</button>
						)}

						{/* Approval step. */}
						{currentStepId === "approval" && (
							<button
								type="button"
								onClick={goNext}
								className="btn-primary inline-flex items-center gap-1.5"
							>
								Next
								<ArrowRight className="h-4 w-4" />
							</button>
						)}

						{/* Submit step: fire. */}
						{currentStepId === "submit" && (
							<button
								type="button"
								onClick={handleConfirm}
								disabled={isPatching || selectedTargetIds.size === 0}
								className="btn-primary inline-flex items-center gap-1.5 disabled:opacity-50 disabled:cursor-not-allowed"
							>
								{isPatching ? (
									<>
										<RefreshCw className="h-4 w-4 animate-spin" />
										{confirmButtonLabel()}
									</>
								) : (
									<>
										{failedTargetIds.length > 0 ? (
											<RefreshCw className="h-4 w-4" />
										) : isApprove ? (
											<Check className="h-4 w-4" />
										) : approvalDecision === "submit" ? (
											<Send className="h-4 w-4" />
										) : (
											<Zap className="h-4 w-4" />
										)}
										{confirmButtonLabel()}
									</>
								)}
							</button>
						)}
					</div>
				</div>
			</div>
		</div>
	);
}
