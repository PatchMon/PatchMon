import { ArrowUpRight, Check, Lock, Mail } from "lucide-react";
import { Link } from "react-router-dom";
import {
	getModuleLabel,
	getRequiredTier,
	getTier,
	getTierUnlocks,
} from "../constants/tiers";
import { useAuth } from "../contexts/AuthContext";
import { useSettings } from "../contexts/SettingsContext";
import TierBadge from "./TierBadge";

// Shared content block used by both the full-page UpgradeRequired screen and
// the modal/inline variants. Renders no data fetching — pure render from
// static constants + auth/settings context. This keeps the locked surfaces
// free of any API calls that could leak information or trigger 403s.
const UpgradeRequiredContent = ({ module: moduleKey, variant = "page" }) => {
	const { hasPermission } = useAuth();
	const { settings: publicSettings } = useSettings();

	const tierId = getRequiredTier(moduleKey);
	const tierDef = getTier(tierId);
	const featureName = getModuleLabel(moduleKey);
	const unlocks = getTierUnlocks(tierId);

	const canManageBilling =
		publicSettings?.admin_mode === true && hasPermission("can_manage_billing");

	if (!tierDef) {
		return (
			<div className="card p-6 text-center">
				<h2 className="text-lg font-semibold text-secondary-900 dark:text-white mb-2">
					Feature unavailable
				</h2>
				<p className="text-secondary-600 dark:text-secondary-300">
					This feature is not included in your current plan.
				</p>
			</div>
		);
	}

	const containerCls =
		variant === "inline"
			? "py-8 px-4"
			: variant === "modal"
				? ""
				: "max-w-3xl mx-auto py-12 px-4";

	return (
		<div className={containerCls}>
			<div className="card p-6 sm:p-10">
				<div className="flex items-center justify-center mb-6">
					<div
						className={`rounded-full p-4 ${tierDef.badgeClass} bg-opacity-60`}
					>
						<Lock className="h-8 w-8" />
					</div>
				</div>

				<div className="text-center mb-6">
					<div className="flex items-center justify-center gap-2 mb-3">
						<h2 className="text-2xl font-bold text-secondary-900 dark:text-white">
							{featureName}
						</h2>
						<TierBadge tier={tierId} />
					</div>
					<p className="text-secondary-600 dark:text-secondary-300 text-base">
						{tierDef.tagline}
					</p>
				</div>

				{unlocks.length > 0 && (
					<div className="mb-6">
						<p className="text-sm font-semibold text-secondary-700 dark:text-secondary-200 mb-3 text-center">
							Unlock with {tierDef.name}:
						</p>
						<ul className="space-y-2 max-w-md mx-auto">
							{unlocks.map((label) => (
								<li
									key={label}
									className="flex items-start gap-2 text-sm text-secondary-700 dark:text-secondary-200"
								>
									<Check className="h-4 w-4 mt-0.5 flex-shrink-0 text-success-600 dark:text-success-400" />
									<span>{label}</span>
								</li>
							))}
						</ul>
					</div>
				)}

				<div className="flex flex-col sm:flex-row items-center justify-center gap-3 pt-2">
					{canManageBilling ? (
						<Link
							to="/billing"
							className="btn-primary inline-flex items-center gap-2 min-h-[44px]"
						>
							Upgrade to {tierDef.name}
							<ArrowUpRight className="h-4 w-4" />
						</Link>
					) : (
						<div className="inline-flex items-start gap-3 px-4 py-3 rounded-md bg-primary-50 dark:bg-primary-900/20 border border-primary-200 dark:border-primary-800 text-sm text-primary-800 dark:text-primary-200 max-w-md">
							<Mail className="h-4 w-4 mt-0.5 flex-shrink-0" />
							<span>
								Contact your administrator to upgrade to{" "}
								<strong>{tierDef.name}</strong> and unlock this feature.
							</span>
						</div>
					)}
				</div>
			</div>
		</div>
	);
};

export default UpgradeRequiredContent;
