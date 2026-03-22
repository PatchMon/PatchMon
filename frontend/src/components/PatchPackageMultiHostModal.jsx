import { useQueries, useQuery } from "@tanstack/react-query";
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
import { useEffect, useMemo, useRef, useState } from "react";
import { packagesAPI } from "../utils/api";
import { patchingAPI, pollDryRunUntilDone } from "../utils/patchingApi";

const DRY_RUN_PACKAGE_LIMIT = 5;

const STEPS = [
	{ id: "hosts", label: "Select Hosts" },
	{ id: "validate", label: "Validate" },
	{ id: "confirm", label: "Confirm" },
];

/**
 * Wizard modal for patching selected packages across multiple hosts.
 * Step 1: Select hosts
 * Step 2: Validate (dry-run preview)
 * Step 3: Confirm and patch
 */
export default function PatchPackageMultiHostModal({
	isOpen,
	onClose,
	packageNames,
	onSuccess,
}) {
	const [step, setStep] = useState(0);
	const [selectedHostIds, setSelectedHostIds] = useState(new Set());

	// Fetch hosts for each package in parallel
	const hostQueries = useQueries({
		queries: (packageNames || []).map((pkgName) => ({
			queryKey: ["package-hosts-for-patch", pkgName],
			queryFn: () =>
				packagesAPI
					.getHosts(encodeURIComponent(pkgName), { limit: 500 })
					.then((res) => res.data?.hosts || []),
			enabled: isOpen && !!pkgName,
		})),
	});

	// Union of unique hosts across all packages, excluding Windows (patching not supported)
	const hosts = useMemo(() => {
		const byId = new Map();
		for (const q of hostQueries) {
			const list = q.data || [];
			for (const h of list) {
				const osType = (h.os_type || h.osType || "").toLowerCase();
				if (osType.includes("windows")) continue; // Skip Windows hosts
				const id = h.hostId || h.host_id || h.id;
				if (id && !byId.has(id)) {
					byId.set(id, {
						id,
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
	}, [hostQueries]);

	const isLoading = hostQueries.some((q) => q.isLoading);
	const hasInitialized = useRef(false);

	// biome-ignore lint/correctness/useExhaustiveDependencies: intentionally reset when packageNames changes
	useEffect(() => {
		hasInitialized.current = false;
		setSelectedHostIds(new Set());
		setStep(0);
		setValidationByHost({});
		setError(null);
		setExpandedDepsByHost({});
	}, [packageNames]);

	// When hosts first load, select all by default
	useEffect(() => {
		if (!isLoading && hosts.length > 0 && !hasInitialized.current) {
			hasInitialized.current = true;
			setSelectedHostIds(new Set(hosts.map((h) => h.id)));
		}
		if (hosts.length === 0) {
			hasInitialized.current = false;
		}
	}, [isLoading, hosts.length, hosts]);

	const toggleHost = (hostId) => {
		setSelectedHostIds((prev) => {
			const next = new Set(prev);
			if (next.has(hostId)) next.delete(hostId);
			else next.add(hostId);
			return next;
		});
	};

	const selectAll = () => setSelectedHostIds(new Set(hosts.map((h) => h.id)));
	const selectNone = () => setSelectedHostIds(new Set());

	const [isPatching, setIsPatching] = useState(false);
	const [error, setError] = useState(null);
	const [validationByHost, setValidationByHost] = useState({});
	const [isValidating, setIsValidating] = useState(false);
	const [expandedOutput, setExpandedOutput] = useState(null);
	// Per-host policy override: hostId -> "default" | policyId | "immediate"
	const [policyOverrides, setPolicyOverrides] = useState({});
	// Per-host: which hosts have dependencies expanded in Confirm step
	const [expandedDepsByHost, setExpandedDepsByHost] = useState({});
	// Split button dropdown for the validate step action
	const [splitOpen, setSplitOpen] = useState(false);
	const splitRef = useRef(null);

	// Close split dropdown on outside click
	useEffect(() => {
		if (!splitOpen) return;
		const handler = (e) => {
			if (splitRef.current && !splitRef.current.contains(e.target)) {
				setSplitOpen(false);
			}
		};
		document.addEventListener("mousedown", handler);
		return () => document.removeEventListener("mousedown", handler);
	}, [splitOpen]);

	// Fetch all policies for the override selector (only when on confirm step)
	const { data: allPolicies } = useQuery({
		queryKey: ["patching-policies"],
		queryFn: () => patchingAPI.getPolicies(),
		staleTime: 60 * 1000,
		enabled: step === 2,
	});

	// Fetch per-host policy/schedule preview for confirm step
	const selectedHostArr = useMemo(
		() => hosts.filter((h) => selectedHostIds.has(h.id)),
		[hosts, selectedHostIds],
	);
	const previewQueries = useQueries({
		queries: selectedHostArr.map((h) => ({
			queryKey: ["patching-preview-run", h.id],
			queryFn: () => patchingAPI.getPreviewRun(h.id),
			staleTime: 30 * 1000,
			enabled: step === 2,
		})),
	});
	const previewByHost = useMemo(() => {
		const map = {};
		for (let i = 0; i < selectedHostArr.length; i++) {
			if (previewQueries[i]?.data) {
				map[selectedHostArr[i].id] = previewQueries[i].data;
			}
		}
		return map;
	}, [previewQueries, selectedHostArr]);

	const selectedPkgs = packageNames || [];
	const canValidate =
		selectedPkgs.length > 0 && selectedPkgs.length <= DRY_RUN_PACKAGE_LIMIT;

	const handleValidate = async () => {
		const ids = Array.from(selectedHostIds);
		if (ids.length === 0) return;
		setIsValidating(true);
		setValidationByHost({});
		const results = {};
		for (const hostId of ids) {
			try {
				const res = await patchingAPI.trigger(
					hostId,
					"patch_package",
					null,
					selectedPkgs,
					{ dry_run: true },
				);
				const runId = res?.patch_run_id;
				if (!runId) {
					results[hostId] = {
						status: "failed",
						packages_affected: [],
						shell_output: "",
						error: "No run ID returned",
					};
					continue;
				}
				const result = await pollDryRunUntilDone(runId);
				results[hostId] = { ...result, patch_run_id: runId };
			} catch (err) {
				results[hostId] = {
					status: "failed",
					packages_affected: [],
					shell_output: "",
					error: err.response?.data?.error || err.message,
				};
			}
		}
		setValidationByHost(results);
		setIsValidating(false);
	};

	const handleConfirm = async () => {
		const ids = Array.from(selectedHostIds);
		if (ids.length === 0) {
			setError("Select at least one host");
			return;
		}
		setError(null);
		setIsPatching(true);
		try {
			for (const hostId of ids) {
				const override = policyOverrides[hostId];
				const validation = validationByHost[hostId];
				const validationRunId = validation?.patch_run_id;
				// If we have a validation run (validated or pending_validation), approve it
				// instead of creating a brand new run. This marks the validation entry as
				// "approved" and creates a linked execution run on the backend.
				if (
					validationRunId &&
					(validation.status === "validated" ||
						validation.status === "pending_validation")
				) {
					await patchingAPI.approveRun(
						validationRunId,
						override ? { schedule_override: override } : {},
					);
				} else {
					// No usable validation run: trigger a fresh run directly.
					await patchingAPI.trigger(
						hostId,
						"patch_package",
						null,
						packageNames,
						override ? { schedule_override: override } : {},
					);
				}
			}
			onSuccess?.("patch");
			onClose();
		} catch (err) {
			setError(err.response?.data?.error || err.message);
		} finally {
			setIsPatching(false);
		}
	};

	// Determine if any selected host has extra dependencies
	const hostsWithExtraDeps = useMemo(() => {
		const requestedSet = new Set(selectedPkgs.map((n) => n.toLowerCase()));
		return hosts.filter((h) => {
			const v = validationByHost[h.id];
			if (!v || v.status !== "validated") return false;
			return v.packages_affected?.some(
				(p) => !requestedSet.has(p.toLowerCase()),
			);
		});
	}, [validationByHost, hosts, selectedPkgs]);

	const validationDone = Object.keys(validationByHost).length > 0;

	if (!isOpen) return null;

	const currentStepId = STEPS[step]?.id;

	return (
		<div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
			<button
				type="button"
				onClick={() => !isPatching && !isValidating && onClose()}
				className="fixed inset-0 cursor-default"
				aria-label="Close modal"
			/>
			<div className="bg-white dark:bg-secondary-800 rounded-lg shadow-xl max-w-2xl w-full mx-4 max-h-[92vh] flex flex-col relative z-10">
				{/* Header */}
				<div className="px-6 py-4 border-b border-secondary-200 dark:border-secondary-600 shrink-0">
					<div className="flex items-center justify-between mb-3">
						<h3 className="text-lg font-medium text-secondary-900 dark:text-white flex items-center gap-2">
							<Wrench className="h-5 w-5 text-primary-600" />
							Patch packages on hosts
						</h3>
						<button
							type="button"
							onClick={() => !isPatching && !isValidating && onClose()}
							className="p-1 rounded hover:bg-secondary-100 dark:hover:bg-secondary-700 text-secondary-400 hover:text-secondary-600"
						>
							<X className="h-5 w-5" />
						</button>
					</div>
					{/* Step indicator */}
					<div className="flex items-center gap-1 text-xs">
						{STEPS.map((s, i) => (
							<div key={s.id} className="flex items-center gap-1">
								<div
									className={`flex items-center gap-1 px-2 py-0.5 rounded font-medium ${
										i === step
											? "bg-primary-100 text-primary-700 dark:bg-primary-900 dark:text-primary-300"
											: i < step
												? "bg-green-100 text-green-700 dark:bg-green-900 dark:text-green-300"
												: "bg-secondary-100 text-secondary-500 dark:bg-secondary-700 dark:text-secondary-400"
									}`}
								>
									{i < step ? (
										<Check className="h-3 w-3" />
									) : (
										<span>{i + 1}</span>
									)}
									{s.label}
								</div>
								{i < STEPS.length - 1 && (
									<ChevronRight className="h-3 w-3 text-secondary-400" />
								)}
							</div>
						))}
					</div>
				</div>

				{/* Body */}
				<div className="px-6 py-4 overflow-y-auto flex-1 min-h-0">
					{/* Step 1: Select hosts */}
					{currentStepId === "hosts" &&
						(isLoading ? (
							<div className="flex items-center gap-2 text-secondary-600 dark:text-secondary-400 py-8">
								<RefreshCw className="h-4 w-4 animate-spin shrink-0" />
								Loading hosts…
							</div>
						) : hosts.length === 0 ? (
							<p className="text-sm text-secondary-600 dark:text-secondary-400 py-4">
								{hostQueries.some((q) => (q.data || []).length > 0)
									? "These packages are only installed on Windows hosts. Patching is not supported for Windows."
									: "No hosts found with these packages installed."}
							</p>
						) : (
							<>
								<p className="text-sm text-secondary-600 dark:text-secondary-400 mb-3">
									Select the hosts you want to deploy{" "}
									<strong>{packageNames?.length || 0}</strong> package
									{packageNames?.length !== 1 ? "s" : ""} to.
								</p>
								<div className="flex items-center gap-2 mb-3">
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
									<span className="text-sm text-secondary-500 dark:text-secondary-400 ml-auto">
										{selectedHostIds.size} of {hosts.length} selected
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
											{hosts.map((host) => (
												<tr
													key={host.id}
													className="hover:bg-secondary-50 dark:hover:bg-secondary-700"
												>
													<td className="px-3 py-2">
														<button
															type="button"
															onClick={() => toggleHost(host.id)}
															className="flex items-center justify-center w-full"
														>
															{selectedHostIds.has(host.id) ? (
																<CheckSquare className="h-4 w-4 text-primary-600" />
															) : (
																<Square className="h-4 w-4 text-secondary-400" />
															)}
														</button>
													</td>
													<td className="px-3 py-2 text-sm text-secondary-900 dark:text-white flex items-center gap-2">
														<Server className="h-4 w-4 text-secondary-400 shrink-0" />
														{host.friendly_name || host.hostname || host.id}
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
							{!validationDone && !isValidating && (
								<div className="flex items-center justify-center py-8">
									<button
										type="button"
										onClick={handleValidate}
										disabled={selectedHostIds.size === 0}
										className="btn-primary inline-flex items-center gap-2"
									>
										<Eye className="h-4 w-4" />
										Run validation
									</button>
								</div>
							)}
							{isValidating && (
								<div className="flex items-center gap-2 text-secondary-600 dark:text-secondary-400 py-8 justify-center">
									<RefreshCw className="h-4 w-4 animate-spin shrink-0" />
									Validating on {selectedHostIds.size} host
									{selectedHostIds.size !== 1 ? "s" : ""}…
								</div>
							)}
							{validationDone && !isValidating && (
								<div className="space-y-3">
									{hostsWithExtraDeps.length > 0 && (
										<div className="flex items-start gap-2 p-3 rounded-lg bg-amber-50 dark:bg-amber-900/30 border border-amber-200 dark:border-amber-700">
											<AlertTriangle className="h-4 w-4 text-amber-600 dark:text-amber-400 mt-0.5 shrink-0" />
											<p className="text-sm text-amber-700 dark:text-amber-300">
												<strong>
													{hostsWithExtraDeps.length} host
													{hostsWithExtraDeps.length !== 1 ? "s" : ""}
												</strong>{" "}
												will install additional dependencies beyond the
												requested packages. Review below before proceeding.
											</p>
										</div>
									)}
									{hosts
										.filter((h) => selectedHostIds.has(h.id))
										.map((host) => {
											const v = validationByHost[host.id];
											if (!v) return null;
											const hostLabel =
												host.friendly_name || host.hostname || host.id;
											const requestedSet = new Set(
												selectedPkgs.map((n) => n.toLowerCase()),
											);
											const extraDeps =
												v.packages_affected?.filter(
													(p) => !requestedSet.has(p.toLowerCase()),
												) || [];
											const isExpanded = expandedOutput === host.id;

											return (
												<div
													key={host.id}
													className={`rounded-lg border text-sm overflow-hidden ${
														extraDeps.length > 0
															? "border-amber-300 dark:border-amber-600"
															: v.status === "validated"
																? "border-green-200 dark:border-green-700"
																: "border-red-200 dark:border-red-700"
													}`}
												>
													<div
														className={`px-3 py-2 flex items-center gap-2 ${
															extraDeps.length > 0
																? "bg-amber-50 dark:bg-amber-900/30"
																: v.status === "validated"
																	? "bg-green-50 dark:bg-green-900/20"
																	: "bg-red-50 dark:bg-red-900/20"
														}`}
													>
														<Server className="h-4 w-4 text-secondary-400 shrink-0" />
														<span className="font-medium text-secondary-800 dark:text-secondary-200 flex-1">
															{hostLabel}
														</span>
														{extraDeps.length > 0 && (
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
																				!requestedSet.has(p.toLowerCase())
																					? "text-amber-600 dark:text-amber-400 font-medium"
																					: ""
																			}
																		>
																			{p}
																			{!requestedSet.has(p.toLowerCase()) && (
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
																	setExpandedOutput(isExpanded ? null : host.id)
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
												</div>
											);
										})}
									<button
										type="button"
										onClick={handleValidate}
										className="text-xs text-primary-600 dark:text-primary-400 hover:underline inline-flex items-center gap-1"
									>
										<RefreshCw className="h-3 w-3" />
										Re-run validation
									</button>
								</div>
							)}
						</>
					)}

					{/* Step 3: Confirm */}
					{currentStepId === "confirm" && (
						<div className="space-y-4">
							{validationDone && hostsWithExtraDeps.length > 0 && (
								<div className="flex items-start gap-2 p-3 rounded-lg bg-amber-50 dark:bg-amber-900/30 border border-amber-200 dark:border-amber-700">
									<AlertTriangle className="h-4 w-4 text-amber-600 dark:text-amber-400 mt-0.5 shrink-0" />
									<p className="text-sm text-amber-700 dark:text-amber-300">
										<strong>
											{hostsWithExtraDeps.length} host
											{hostsWithExtraDeps.length !== 1 ? "s" : ""}
										</strong>{" "}
										will install additional dependencies. Review each host below
										before confirming.
									</p>
								</div>
							)}

							{/* One table per selected host */}
							{selectedHostArr.map((host) => {
								const v = validationByHost[host.id];
								const preview = previewByHost[host.id];
								const requestedSet = new Set(
									selectedPkgs.map((n) => n.toLowerCase()),
								);

								// Build package rows: requested packages first, then extra deps
								const requestedPackages = selectedPkgs;
								const extraDeps =
									v?.packages_affected?.filter(
										(p) => !requestedSet.has(p.toLowerCase()),
									) || [];

								// Policy display
								const override = policyOverrides[host.id] || "default";
								const _policyLabel =
									override === "immediate"
										? "Run immediately"
										: override !== "default" && allPolicies
											? allPolicies.find((p) => p.id === override)?.name ||
												"Custom policy"
											: preview?.policy_name || "Default (immediate)";
								const scheduledAt =
									override === "immediate"
										? "Now"
										: preview?.run_at_iso
											? new Date(preview.run_at_iso).toLocaleString()
											: " -";

								const hostLabel =
									host.friendly_name || host.hostname || host.id;

								return (
									<div
										key={host.id}
										className="border border-secondary-200 dark:border-secondary-600 rounded-lg overflow-hidden"
									>
										{/* Host header */}
										<div className="bg-secondary-50 dark:bg-secondary-700 px-4 py-2 flex items-center gap-2">
											<Server className="h-4 w-4 text-secondary-500 shrink-0" />
											<span className="font-medium text-secondary-800 dark:text-white text-sm flex-1">
												{hostLabel}
											</span>
											{validationDone && extraDeps.length > 0 && (
												<span className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded bg-amber-100 text-amber-700 dark:bg-amber-800 dark:text-amber-200">
													<AlertTriangle className="h-3 w-3" />
													{extraDeps.length} extra dep
													{extraDeps.length !== 1 ? "s" : ""}
												</span>
											)}
											{validationDone &&
												v?.status === "validated" &&
												extraDeps.length === 0 && (
													<span className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded bg-green-100 text-green-700 dark:bg-green-800 dark:text-green-200">
														<Check className="h-3 w-3" />
														Clean
													</span>
												)}
											{!validationDone && (
												<span className="text-xs text-secondary-400 dark:text-secondary-300">
													No validation
												</span>
											)}
											{v?.status === "timeout" && (
												<span className="text-xs px-2 py-0.5 rounded bg-secondary-100 text-secondary-600 dark:bg-secondary-600 dark:text-secondary-300">
													Host offline
												</span>
											)}
										</div>

										{/* Packages table */}
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
																		setExpandedDepsByHost((prev) => ({
																			...prev,
																			[host.id]: !prev[host.id],
																		}))
																	}
																	className="w-full flex items-center gap-2 py-2 text-left text-sm text-amber-700 dark:text-amber-300 hover:bg-amber-100/50 dark:hover:bg-amber-900/20 transition-colors"
																>
																	{expandedDepsByHost[host.id] ? (
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
														{expandedDepsByHost[host.id] &&
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
												{!validationDone && requestedPackages.length === 0 && (
													<tr>
														<td
															colSpan={2}
															className="px-4 py-2 text-sm text-secondary-500 dark:text-secondary-400"
														>
															No validation run
														</td>
													</tr>
												)}
											</tbody>
										</table>

										{/* Policy & schedule row */}
										<div className="border-t border-secondary-200 dark:border-secondary-600 px-4 py-3 bg-secondary-50/50 dark:bg-secondary-700/30">
											<div className="flex flex-wrap gap-4 items-start">
												<div className="flex-1 min-w-0">
													<p className="text-xs font-medium text-secondary-500 dark:text-secondary-400 mb-1 flex items-center gap-1">
														<Shield className="h-3 w-3" />
														Patch policy
													</p>
													<select
														value={override}
														onChange={(e) =>
															setPolicyOverrides((prev) => ({
																...prev,
																[host.id]: e.target.value,
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
														{(allPolicies || [])
															.filter((p) => p.id !== preview?.policy_id)
															.map((p) => (
																<option key={p.id} value={p.id}>
																	Override: {p.name}
																</option>
															))}
													</select>
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

							{error && (
								<p className="text-sm text-red-600 dark:text-red-400">
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
								onClick={() => setStep((s) => s - 1)}
								disabled={isPatching || isValidating}
								className="btn-outline inline-flex items-center gap-1"
							>
								<ArrowLeft className="h-4 w-4" />
								Back
							</button>
						)}
						<button
							type="button"
							onClick={onClose}
							disabled={isPatching || isValidating}
							className="btn-outline"
						>
							Cancel
						</button>
					</div>

					<div>
						{/* Step 1: Next (select hosts) */}
						{currentStepId === "hosts" && (
							<button
								type="button"
								disabled={selectedHostIds.size === 0 || hosts.length === 0}
								onClick={() => setStep(1)}
								className="btn-primary inline-flex items-center gap-1.5 disabled:opacity-50 disabled:cursor-not-allowed"
							>
								Next
								<ArrowRight className="h-4 w-4" />
							</button>
						)}

						{/* Step 2: validate/skip/next */}
						{currentStepId === "validate" && (
							<div className="flex gap-2">
								{!validationDone && !isValidating && canValidate && (
									<button
										type="button"
										onClick={() => setStep(2)}
										className="btn-outline text-sm"
									>
										Skip validation
									</button>
								)}
								{validationDone && !isValidating && (
									<div ref={splitRef} className="relative flex">
										{/* Primary action: Queue & patch now (go to confirm step) */}
										<button
											type="button"
											onClick={() => setStep(2)}
											className="btn-primary inline-flex items-center gap-1.5 rounded-r-none border-r border-primary-700"
										>
											<Zap className="h-4 w-4" />
											Queue &amp; patch now
										</button>
										{/* Dropdown toggle */}
										<button
											type="button"
											onClick={() => setSplitOpen((o) => !o)}
											className="btn-primary px-2 rounded-l-none"
											aria-label="More options"
										>
											<ChevronDown className="h-4 w-4" />
										</button>
										{/* Dropdown menu */}
										{splitOpen && (
											<div className="absolute right-0 bottom-full mb-1 w-56 rounded-lg border border-secondary-200 dark:border-secondary-600 bg-white dark:bg-secondary-800 shadow-lg z-20 py-1">
												<button
													type="button"
													className="w-full flex items-center gap-2 px-3 py-2 text-sm text-secondary-700 dark:text-secondary-300 hover:bg-secondary-100 dark:hover:bg-secondary-700"
													onClick={() => {
														setSplitOpen(false);
														onSuccess?.("approval");
														onClose();
													}}
												>
													<Send className="h-4 w-4 shrink-0 text-amber-500" />
													<span>
														<span className="font-medium block">
															Submit for approval
														</span>
														<span className="text-xs text-secondary-500 dark:text-secondary-400">
															Leave pending - approve later from Runs &amp;
															History
														</span>
													</span>
												</button>
											</div>
										)}
									</div>
								)}
								{!canValidate && !validationDone && (
									<button
										type="button"
										onClick={() => setStep(2)}
										className="btn-outline text-sm"
									>
										Skip (too many packages)
									</button>
								)}
							</div>
						)}

						{/* Step 3: Confirm and patch */}
						{currentStepId === "confirm" && (
							<button
								type="button"
								onClick={handleConfirm}
								disabled={isPatching || selectedHostIds.size === 0}
								className="btn-primary inline-flex items-center gap-1.5 disabled:opacity-50 disabled:cursor-not-allowed"
							>
								{isPatching ? (
									<>
										<RefreshCw className="h-4 w-4 animate-spin" />
										Queuing…
									</>
								) : (
									<>
										<Zap className="h-4 w-4" />
										Queue &amp; patch {selectedHostIds.size} host
										{selectedHostIds.size !== 1 ? "s" : ""}
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
