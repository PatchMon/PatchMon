import { useQuery } from "@tanstack/react-query";
import { Check, Eye, RefreshCw, Wrench, X } from "lucide-react";
import { useState } from "react";
import { getGlobalTimezone } from "../utils/api";
import { patchingAPI, pollDryRunUntilDone } from "../utils/patchingApi";

const DRY_RUN_PACKAGE_LIMIT = 5;

/**
 * Modal that shows policy preview (host, policy name, run at) before confirming a patch.
 * Used for Patch all and Patch package actions.
 * For patch_package with few packages, supports dry-run preview to show affected dependencies.
 */
export default function PatchConfirmModal({
	isOpen,
	onClose,
	onConfirm,
	isPending,
	hostId,
	patchType = "patch_all",
	packageName = null,
	packageNames = null,
	hostDisplayName = null,
}) {
	const [validationState, setValidationState] = useState(null);
	const [isValidating, setIsValidating] = useState(false);

	const { data: preview, isLoading } = useQuery({
		queryKey: ["patching-preview-run", hostId],
		queryFn: () => patchingAPI.getPreviewRun(hostId),
		enabled: isOpen && !!hostId,
	});

	const selectedPkgs =
		packageNames?.length > 0 ? packageNames : packageName ? [packageName] : [];
	const canPreview =
		patchType === "patch_package" &&
		selectedPkgs.length > 0 &&
		selectedPkgs.length <= DRY_RUN_PACKAGE_LIMIT;

	const handlePreview = async () => {
		if (!hostId || !canPreview) return;
		setIsValidating(true);
		setValidationState(null);
		try {
			const res = await patchingAPI.trigger(
				hostId,
				"patch_package",
				selectedPkgs.length === 1 ? selectedPkgs[0] : null,
				selectedPkgs.length > 1 ? selectedPkgs : null,
				{ dry_run: true },
			);
			const runId = res?.patch_run_id;
			if (!runId) {
				setValidationState({
					status: "failed",
					packages_affected: [],
					error: "Failed to start dry run",
				});
				return;
			}
			const result = await pollDryRunUntilDone(runId);
			setValidationState(result);
		} catch (err) {
			setValidationState({
				status: "failed",
				packages_affected: [],
				error: err.response?.data?.error || err.message,
			});
		} finally {
			setIsValidating(false);
		}
	};

	if (!isOpen) return null;

	const packagesLabel =
		patchType === "patch_package"
			? packageNames?.length > 0
				? `${packageNames.length} package(s)`
				: packageName
					? "1 package"
					: "packages"
			: "all packages (apt update + upgrade)";

	return (
		<div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
			<button
				type="button"
				onClick={() => !isPending && onClose()}
				className="fixed inset-0 cursor-default"
				aria-label="Close modal"
			/>
			<div className="bg-white dark:bg-secondary-800 rounded-lg shadow-xl max-w-2xl w-full mx-4 relative z-10">
				<div className="px-6 py-4 border-b border-secondary-200 dark:border-secondary-600">
					<div className="flex items-center justify-between">
						<h3 className="text-lg font-medium text-secondary-900 dark:text-white flex items-center gap-2">
							<Wrench className="h-5 w-5 text-primary-600" />
							Confirm patch
						</h3>
						<button
							type="button"
							onClick={() => !isPending && onClose()}
							className="p-1 rounded hover:bg-secondary-100 dark:hover:bg-secondary-700 text-secondary-400 hover:text-secondary-600"
						>
							<X className="h-5 w-5" />
						</button>
					</div>
				</div>
				<div className="px-6 py-4">
					{isLoading ? (
						<div className="flex items-center gap-2 text-secondary-600 dark:text-secondary-400">
							<RefreshCw className="h-4 w-4 animate-spin shrink-0" />
							Loading policy…
						</div>
					) : preview ? (
						<div className="overflow-x-auto -mx-1">
							<table className="w-full divide-y divide-secondary-200 dark:divide-secondary-600 min-w-[320px]">
								<thead className="bg-secondary-50 dark:bg-secondary-700">
									<tr>
										<th className="px-3 py-2 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider">
											Host
										</th>
										<th className="px-3 py-2 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider">
											Policy
										</th>
										<th className="px-3 py-2 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider">
											Run at
										</th>
										<th className="px-3 py-2 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider">
											Scope
										</th>
									</tr>
								</thead>
								<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-600">
									<tr className="hover:bg-secondary-50 dark:hover:bg-secondary-700">
										<td className="px-3 py-2 text-sm text-secondary-900 dark:text-white whitespace-nowrap">
											{preview.host_name || hostDisplayName || hostId}
										</td>
										<td className="px-3 py-2 text-sm text-secondary-700 dark:text-secondary-300 whitespace-nowrap">
											{preview.policy_name}
										</td>
										<td className="px-3 py-2 text-sm text-secondary-700 dark:text-secondary-300 whitespace-nowrap">
											{new Date(preview.run_at_iso).toLocaleString(undefined, {
												dateStyle: "medium",
												timeStyle: "short",
												timeZone: getGlobalTimezone() || undefined,
											})}
										</td>
										<td className="px-3 py-2 text-sm text-secondary-700 dark:text-secondary-300">
											{packagesLabel}
										</td>
									</tr>
								</tbody>
							</table>
							{patchType === "patch_package" &&
								(packageNames?.length > 0 || packageName) && (
									<div className="mt-3 pt-3 border-t border-secondary-200 dark:border-secondary-600">
										<p className="text-xs font-medium text-secondary-500 dark:text-secondary-400 mb-1.5">
											Packages:
										</p>
										<ul className="text-sm text-secondary-600 dark:text-secondary-400 max-h-20 overflow-y-auto list-disc list-inside space-y-0.5">
											{(packageNames?.length
												? packageNames
												: [packageName]
											).map((name) => (
												<li key={name}>{name}</li>
											))}
										</ul>
									</div>
								)}
							{canPreview && (
								<div className="mt-3 pt-3 border-t border-secondary-200 dark:border-secondary-600">
									<div className="flex items-center justify-between gap-2">
										<p className="text-xs font-medium text-secondary-500 dark:text-secondary-400">
											Preview affected packages (including dependencies)
										</p>
										<button
											type="button"
											onClick={handlePreview}
											disabled={isValidating || isPending}
											className="btn-outline text-xs inline-flex items-center gap-1 py-1 px-2"
										>
											{isValidating ? (
												<>
													<RefreshCw className="h-3 w-3 animate-spin" />
													Validating…
												</>
											) : (
												<>
													<Eye className="h-3 w-3" />
													Preview
												</>
											)}
										</button>
									</div>
									{validationState && (
										<div className="mt-2 p-2 rounded bg-secondary-50 dark:bg-secondary-700/50 text-sm">
											{validationState.status === "validated" &&
											validationState.packages_affected?.length > 0 ? (
												<>
													<p className="text-secondary-600 dark:text-secondary-300 mb-1">
														Updating {selectedPkgs.length} package
														{selectedPkgs.length !== 1 ? "s" : ""} will also
														update {validationState.packages_affected.length}{" "}
														package
														{validationState.packages_affected.length !== 1
															? "s"
															: ""}
														:
													</p>
													<ul className="text-secondary-600 dark:text-secondary-400 max-h-24 overflow-y-auto list-disc list-inside space-y-0.5">
														{validationState.packages_affected.map((p) => (
															<li key={p}>{p}</li>
														))}
													</ul>
												</>
											) : validationState.status === "timeout" ||
												validationState.error ? (
												<p className="text-amber-600 dark:text-amber-400">
													{validationState.error}
												</p>
											) : validationState.status === "validated" &&
												(!validationState.packages_affected ||
													validationState.packages_affected.length === 0) ? (
												<p className="text-secondary-600 dark:text-secondary-300">
													No additional packages would be updated.
												</p>
											) : null}
										</div>
									)}
								</div>
							)}
						</div>
					) : (
						<p className="text-sm text-secondary-600 dark:text-secondary-400">
							Could not load policy preview. You can still confirm to queue the
							patch with the default policy (immediate).
						</p>
					)}
				</div>
				<div className="px-6 py-4 border-t border-secondary-200 dark:border-secondary-600 flex justify-end gap-2">
					<button
						type="button"
						onClick={onClose}
						disabled={isPending}
						className="btn-outline"
					>
						Cancel
					</button>
					<button
						type="button"
						onClick={onConfirm}
						disabled={isPending}
						className="btn-primary inline-flex items-center gap-1.5"
					>
						{isPending ? (
							<>
								<RefreshCw className="h-4 w-4 animate-spin" />
								Queuing…
							</>
						) : (
							<>
								<Check className="h-4 w-4" />
								Confirm
							</>
						)}
					</button>
				</div>
			</div>
		</div>
	);
}
