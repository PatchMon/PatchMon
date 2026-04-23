import { TIERS } from "../constants/tiers";

const TierBadge = ({ tier, className = "" }) => {
	const tierDef = TIERS[tier];
	if (!tierDef) return null;

	return (
		<span
			className={`inline-flex items-center px-1.5 py-px rounded text-[10px] font-semibold uppercase tracking-wide leading-tight ${tierDef.badgeClass} ${className}`}
			title={`Requires ${tierDef.name} plan`}
		>
			{tierDef.name}
		</span>
	);
};

export default TierBadge;
