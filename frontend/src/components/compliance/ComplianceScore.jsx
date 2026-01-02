import { ShieldCheck, ShieldAlert, ShieldX, ShieldQuestion } from "lucide-react";

const ComplianceScore = ({ score, size = "md" }) => {
	const getScoreColor = (score) => {
		if (score === null || score === undefined) return "text-secondary-400";
		if (score >= 80) return "text-green-400";
		if (score >= 60) return "text-yellow-400";
		return "text-red-400";
	};

	const getScoreBg = (score) => {
		if (score === null || score === undefined) return "bg-secondary-700";
		if (score >= 80) return "bg-green-900/30";
		if (score >= 60) return "bg-yellow-900/30";
		return "bg-red-900/30";
	};

	const getIcon = (score) => {
		if (score === null || score === undefined) return ShieldQuestion;
		if (score >= 80) return ShieldCheck;
		if (score >= 60) return ShieldAlert;
		return ShieldX;
	};

	const Icon = getIcon(score);
	const sizeClasses = {
		sm: "text-sm px-2 py-1",
		md: "text-base px-3 py-1.5",
		lg: "text-lg px-4 py-2",
	};

	return (
		<div
			className={`inline-flex items-center gap-1.5 rounded-full ${getScoreBg(score)} ${sizeClasses[size]}`}
		>
			<Icon className={`h-4 w-4 ${getScoreColor(score)}`} />
			<span className={`font-semibold ${getScoreColor(score)}`}>
				{score !== null && score !== undefined ? `${score.toFixed(0)}%` : "N/A"}
			</span>
		</div>
	);
};

export default ComplianceScore;
