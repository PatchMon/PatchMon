import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import {
	Shield,
	ShieldCheck,
	ShieldAlert,
	ShieldX,
	TrendingUp,
	TrendingDown,
	Clock,
	Server,
} from "lucide-react";
import { complianceAPI } from "../utils/complianceApi";
import ComplianceScore from "../components/compliance/ComplianceScore";

const Compliance = () => {
	const { data: dashboard, isLoading, error } = useQuery({
		queryKey: ["compliance-dashboard"],
		queryFn: () => complianceAPI.getDashboard().then((res) => res.data),
		refetchInterval: 30000,
	});

	if (isLoading) {
		return (
			<div className="flex items-center justify-center h-64">
				<div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-500"></div>
			</div>
		);
	}

	if (error) {
		return (
			<div className="p-4 bg-red-900/50 border border-red-700 rounded-lg">
				<p className="text-red-200">Failed to load compliance dashboard</p>
			</div>
		);
	}

	const { summary, recent_scans, worst_hosts } = dashboard || {};

	return (
		<div className="space-y-6">
			{/* Header */}
			<div className="flex items-center justify-between">
				<div className="flex items-center gap-3">
					<Shield className="h-8 w-8 text-primary-400" />
					<h1 className="text-2xl font-bold text-white">Security Compliance</h1>
				</div>
			</div>

			{/* Summary Cards */}
			<div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-5 gap-4">
				<div className="bg-secondary-800 rounded-lg p-4 border border-secondary-700">
					<div className="flex items-center gap-2 text-secondary-400 mb-2">
						<Server className="h-4 w-4" />
						<span className="text-sm">Total Hosts</span>
					</div>
					<p className="text-2xl font-bold text-white">{summary?.total_hosts || 0}</p>
				</div>

				<div className="bg-secondary-800 rounded-lg p-4 border border-secondary-700">
					<div className="flex items-center gap-2 text-secondary-400 mb-2">
						<TrendingUp className="h-4 w-4" />
						<span className="text-sm">Average Score</span>
					</div>
					<p className="text-2xl font-bold text-white">{summary?.average_score?.toFixed(1) || 0}%</p>
				</div>

				<div className="bg-secondary-800 rounded-lg p-4 border border-green-700/50">
					<div className="flex items-center gap-2 text-green-400 mb-2">
						<ShieldCheck className="h-4 w-4" />
						<span className="text-sm">Compliant</span>
					</div>
					<p className="text-2xl font-bold text-green-400">{summary?.compliant || 0}</p>
				</div>

				<div className="bg-secondary-800 rounded-lg p-4 border border-yellow-700/50">
					<div className="flex items-center gap-2 text-yellow-400 mb-2">
						<ShieldAlert className="h-4 w-4" />
						<span className="text-sm">Warning</span>
					</div>
					<p className="text-2xl font-bold text-yellow-400">{summary?.warning || 0}</p>
				</div>

				<div className="bg-secondary-800 rounded-lg p-4 border border-red-700/50">
					<div className="flex items-center gap-2 text-red-400 mb-2">
						<ShieldX className="h-4 w-4" />
						<span className="text-sm">Critical</span>
					</div>
					<p className="text-2xl font-bold text-red-400">{summary?.critical || 0}</p>
				</div>
			</div>

			<div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
				{/* Recent Scans */}
				<div className="bg-secondary-800 rounded-lg border border-secondary-700">
					<div className="px-4 py-3 border-b border-secondary-700">
						<h2 className="text-lg font-semibold text-white flex items-center gap-2">
							<Clock className="h-5 w-5 text-secondary-400" />
							Recent Scans
						</h2>
					</div>
					<div className="divide-y divide-secondary-700">
						{recent_scans?.map((scan) => (
							<Link
								key={scan.id}
								to={`/hosts/${scan.host?.id}`}
								className="flex items-center justify-between px-4 py-3 hover:bg-secondary-700/50 transition-colors"
							>
								<div>
									<p className="text-white font-medium">
										{scan.host?.friendly_name || scan.host?.hostname}
									</p>
									<p className="text-sm text-secondary-400">{scan.profile?.name}</p>
								</div>
								<div className="flex items-center gap-3">
									<ComplianceScore score={scan.score} size="sm" />
									<span className="text-xs text-secondary-500">
										{new Date(scan.completed_at).toLocaleDateString()}
									</span>
								</div>
							</Link>
						))}
						{(!recent_scans || recent_scans.length === 0) && (
							<div className="px-4 py-8 text-center text-secondary-400">
								No compliance scans yet
							</div>
						)}
					</div>
				</div>

				{/* Worst Performing Hosts */}
				<div className="bg-secondary-800 rounded-lg border border-secondary-700">
					<div className="px-4 py-3 border-b border-secondary-700">
						<h2 className="text-lg font-semibold text-white flex items-center gap-2">
							<TrendingDown className="h-5 w-5 text-red-400" />
							Needs Attention
						</h2>
					</div>
					<div className="divide-y divide-secondary-700">
						{worst_hosts?.map((scan) => (
							<Link
								key={scan.id}
								to={`/hosts/${scan.host?.id}`}
								className="flex items-center justify-between px-4 py-3 hover:bg-secondary-700/50 transition-colors"
							>
								<div>
									<p className="text-white font-medium">
										{scan.host?.friendly_name || scan.host?.hostname}
									</p>
									<p className="text-sm text-secondary-400">{scan.profile?.name}</p>
								</div>
								<ComplianceScore score={scan.score} size="sm" />
							</Link>
						))}
						{(!worst_hosts || worst_hosts.length === 0) && (
							<div className="px-4 py-8 text-center text-secondary-400">
								No hosts with low compliance scores
							</div>
						)}
					</div>
				</div>
			</div>
		</div>
	);
};

export default Compliance;
