// Extra-dependency derivation shared by the Runs list and the run detail page.
// "Extra" means the affected set contains a name that was not requested, not
// merely that it is larger: requesting ["foo","bar"] and resolving to
// ["foo","baz"] is the same size but still installs "baz".

// `package_name` is the legacy single-package form.
export const requestedPackageNames = (run) => {
	if (Array.isArray(run?.package_names) && run.package_names.length > 0) {
		return run.package_names;
	}
	return run?.package_name ? [run.package_name] : [];
};

export const extraDependencies = (run) => {
	const requested = new Set(
		requestedPackageNames(run).map((name) => String(name).toLowerCase()),
	);
	const affected = Array.isArray(run?.packages_affected)
		? run.packages_affected
		: [];
	return affected.filter(
		(pkg) => pkg && !requested.has(String(pkg).toLowerCase()),
	);
};

// Scoped to `validated` because only there is the list a pre-approval
// prediction. On completed runs it records what was applied, so a badge would
// be both retrospective and near-universal.
// How a run's error_message is presented. A run the user stopped, or one that
// succeeded apart from skipped items, is not an error and gets a neutral notice.
export const runMessageNotice = (run) => {
	const message = run?.error_message ? String(run.error_message).trim() : "";
	if (run?.status === "cancelled") {
		const byUser =
			!message || /(cancel|stop)\w*\s+by\s+(the\s+)?user/i.test(message);
		return {
			tone: "info",
			title: byUser ? "Stopped by user" : "Run cancelled",
			detail: byUser ? null : message,
		};
	}
	if (!message) return null;
	if (run?.status === "completed") {
		return {
			tone: "info",
			title: "Completed with skipped items",
			detail: message,
		};
	}
	if (run?.status === "validated" || run?.status === "dry_run_completed") {
		return {
			tone: "info",
			title: "Validated with skipped items",
			detail: message,
		};
	}
	return { tone: "error", title: "Run reported an error", detail: message };
};

// Summary of the items a successful dry run will leave alone, or null. Prefers
// the server's error_message summary, else builds one from skipped items.
export const skippedItemsSummary = (run) => {
	const message = run?.error_message ? String(run.error_message).trim() : "";
	if (message) return message;
	const skipped = (Array.isArray(run?.items) ? run.items : []).filter(
		(i) => i?.status === "skipped",
	);
	if (skipped.length === 0) return null;
	const names = skipped
		.map((i) => {
			const name = i.title || i.identifier;
			return i.message ? `${name} (${i.message})` : name;
		})
		.join(", ");
	return `${skipped.length} skipped: ${names}`;
};

export const hasExtraDependencies = (run) =>
	run?.status === "validated" &&
	run.patch_type !== "patch_all" &&
	!run.windows_scope &&
	!String(run.hosts?.os_type || "")
		.toLowerCase()
		.includes("windows") &&
	extraDependencies(run).length > 0;
