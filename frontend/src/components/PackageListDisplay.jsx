import { Package } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";

const COLLAPSE_THRESHOLD = 2;

/**
 * Renders a list of package names. When count > COLLAPSE_THRESHOLD, shows
 * "firstPackage + N more" with expandable full list. Each package links to its detail page.
 */
export function PackageNameList({ packages, className = "", showIcon = true }) {
	const [expanded, setExpanded] = useState(false);

	if (!packages || packages.length === 0) return null;
	const showCollapsed = packages.length > COLLAPSE_THRESHOLD && !expanded;
	const first = packages[0];
	const restCount = packages.length - 1;

	return (
		<span className={`flex items-center gap-1 flex-wrap ${className}`}>
			{showIcon && <Package className="h-4 w-4 shrink-0" />}
			{showCollapsed ? (
				<>
					<Link
						to={`/packages/${encodeURIComponent(first)}`}
						className="text-primary-600 dark:text-primary-400 hover:underline"
					>
						{first}
					</Link>
					<button
						type="button"
						onClick={() => setExpanded(true)}
						className="text-primary-600 dark:text-primary-400 hover:underline font-medium"
					>
						+ {restCount} more
					</button>
				</>
			) : (
				packages.map((pkg, i) => (
					<span key={pkg}>
						{i > 0 && ", "}
						<Link
							to={`/packages/${encodeURIComponent(pkg)}`}
							className="text-primary-600 dark:text-primary-400 hover:underline"
						>
							{pkg}
						</Link>
					</span>
				))
			)}
		</span>
	);
}

/**
 * Displays package list for a patch run. Shows requested packages with optional
 * expandable dependencies. Uses "first + N more" when many packages.
 */
export function PackageListDisplay({ run }) {
	if (run.patch_type !== "patch_package") {
		return "Patch all";
	}

	const requested =
		Array.isArray(run.package_names) && run.package_names.length > 0
			? run.package_names
			: run.package_name
				? [run.package_name]
				: [];
	const requestedSet = new Set(requested.map((n) => n.toLowerCase()));

	const extraDeps =
		run.packages_affected?.filter((p) => !requestedSet.has(p.toLowerCase())) ||
		[];

	if (requested.length === 0) {
		return " -";
	}

	return (
		<span className="flex flex-col gap-0.5">
			<PackageNameList packages={requested} />
			{extraDeps.length > 0 && (
				<span className="text-xs text-amber-600 dark:text-amber-400 flex items-center gap-1 flex-wrap">
					(+ {extraDeps.length} dep{extraDeps.length !== 1 ? "s" : ""}:{" "}
					<PackageNameList packages={extraDeps} showIcon={false} />)
				</span>
			)}
		</span>
	);
}
