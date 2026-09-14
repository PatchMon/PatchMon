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
export const hasExtraDependencies = (run) =>
	run?.status === "validated" && extraDependencies(run).length > 0;
