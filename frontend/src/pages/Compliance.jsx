import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import {
	Shield,
	ShieldCheck,
	ShieldAlert,
	ShieldX,
	ShieldOff,
	TrendingUp,
	TrendingDown,
	Clock,
	Server,
	BarChart3,
	PieChart as PieChartIcon,
	CheckCircle,
	XCircle,
	AlertTriangle,
	List,
	RefreshCw,
	Wifi,
	WifiOff,
} from "lucide-react";
import { PieChart, Pie, Cell, BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, Legend } from "recharts";
import { complianceAPI } from "../utils/complianceApi";
import ComplianceScore from "../components/compliance/ComplianceScore";
import { formatDistanceToNow } from "date-fns";

const Compliance = () => {
	const { data: dashboard, isLoading, error } = useQuery({
		queryKey: ["compliance-dashboard"],
		queryFn: () => complianceAPI.getDashboard().then((res) => res.data),
		refetchInterval: 30000,
	});

	// Fetch active/running scans
	const { data: activeScansData } = useQuery({
		queryKey: ["compliance-active-scans"],
		queryFn: () => complianceAPI.getActiveScans().then((res) => res.data),
		refetchInterval: 5000, // Refresh every 5 seconds for active scans
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

	const { summary, recent_scans, worst_hosts, top_failing_rules, profile_distribution, severity_breakdown } = dashboard || {};
	const activeScans = activeScansData?.activeScans || [];

	return (
		<div className="space-y-6">
			{/* Header */}
			<div className="flex items-center justify-between">
				<div className="flex items-center gap-3">
					<Shield className="h-8 w-8 text-primary-400" />
					<h1 className="text-2xl font-bold text-white">Security Compliance</h1>
				</div>
			</div>

			{/* Active Scans Section - Only show if there are running scans */}
			{activeScans.length > 0 && (
				<div className="bg-blue-900/30 border border-blue-700/50 rounded-lg p-4">
					<div className="flex items-center gap-2 mb-3">
						<RefreshCw className="h-5 w-5 text-blue-400 animate-spin" />
						<h2 className="text-lg font-semibold text-blue-300">
							Scans in Progress ({activeScans.length})
						</h2>
					</div>
					<div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
						{activeScans.map((scan) => (
							<Link
								key={scan.id}
								to={`/hosts/${scan.hostId}`}
								className="bg-secondary-800/50 rounded-lg p-3 border border-secondary-700 hover:border-blue-600 transition-colors"
							>
								<div className="flex items-center justify-between mb-2">
									<span className="font-medium text-white truncate">
										{scan.hostName}
									</span>
									{scan.connected ? (
										<Wifi className="h-4 w-4 text-green-400" title="Connected" />
									) : (
										<WifiOff className="h-4 w-4 text-red-400" title="Disconnected" />
									)}
								</div>
								<div className="flex items-center gap-2 text-sm text-secondary-400">
									<span className="px-2 py-0.5 bg-blue-900/50 text-blue-300 rounded text-xs">
										{scan.profileType || "Scanning..."}
									</span>
									<span className="text-xs">
										Started {formatDistanceToNow(new Date(scan.startedAt), { addSuffix: true })}
									</span>
								</div>
							</Link>
						))}
					</div>
				</div>
			)}

			{/* Summary Cards - Row 1: Host Stats */}
			<div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-4">
				<div className="bg-secondary-800 rounded-lg p-4 border border-secondary-700">
					<div className="flex items-center gap-2 text-secondary-400 mb-2">
						<Server className="h-4 w-4" />
						<span className="text-sm">Scanned Hosts</span>
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

				<div className="bg-secondary-800 rounded-lg p-4 border border-secondary-600">
					<div className="flex items-center gap-2 text-secondary-400 mb-2">
						<ShieldOff className="h-4 w-4" />
						<span className="text-sm">Not Scanned</span>
					</div>
					<p className="text-2xl font-bold text-secondary-400">{summary?.unscanned || 0}</p>
				</div>
			</div>

			{/* Summary Cards - Row 2: Rule Stats */}
			{summary?.total_rules > 0 && (
				<div className="grid grid-cols-2 md:grid-cols-4 gap-4">
					<div className="bg-secondary-800 rounded-lg p-4 border border-secondary-700">
						<div className="flex items-center gap-2 text-secondary-400 mb-2">
							<List className="h-4 w-4" />
							<span className="text-sm">Total Rules Evaluated</span>
						</div>
						<p className="text-2xl font-bold text-white">{summary?.total_rules?.toLocaleString() || 0}</p>
					</div>

					<div className="bg-secondary-800 rounded-lg p-4 border border-green-700/50">
						<div className="flex items-center gap-2 text-green-400 mb-2">
							<CheckCircle className="h-4 w-4" />
							<span className="text-sm">Rules Passed</span>
						</div>
						<p className="text-2xl font-bold text-green-400">{summary?.total_passed_rules?.toLocaleString() || 0}</p>
					</div>

					<div className="bg-secondary-800 rounded-lg p-4 border border-red-700/50">
						<div className="flex items-center gap-2 text-red-400 mb-2">
							<XCircle className="h-4 w-4" />
							<span className="text-sm">Rules Failed</span>
						</div>
						<p className="text-2xl font-bold text-red-400">{summary?.total_failed_rules?.toLocaleString() || 0}</p>
					</div>

					<div className="bg-secondary-800 rounded-lg p-4 border border-secondary-700">
						<div className="flex items-center gap-2 text-secondary-400 mb-2">
							<TrendingUp className="h-4 w-4" />
							<span className="text-sm">Pass Rate</span>
						</div>
						<p className="text-2xl font-bold text-white">
							{summary?.total_rules > 0
								? ((summary?.total_passed_rules / summary?.total_rules) * 100).toFixed(1)
								: 0}%
						</p>
					</div>
				</div>
			)}

			{/* Charts Section */}
			{summary && (summary.total_hosts > 0 || summary.unscanned > 0) && (() => {
				const hostDistribution = [
					{ name: "Compliant (≥80%)", value: summary.compliant || 0, color: "#22c55e" },
					{ name: "Warning (60-80%)", value: summary.warning || 0, color: "#eab308" },
					{ name: "Critical (<60%)", value: summary.critical || 0, color: "#ef4444" },
					{ name: "Not Scanned", value: summary.unscanned || 0, color: "#6b7280" },
				].filter(d => d.value > 0);

				// Score ranges for bar chart
				const scoreRanges = [
					{ range: "90-100%", count: recent_scans?.filter(s => s.score >= 90).length || 0, color: "#22c55e" },
					{ range: "80-89%", count: recent_scans?.filter(s => s.score >= 80 && s.score < 90).length || 0, color: "#84cc16" },
					{ range: "70-79%", count: recent_scans?.filter(s => s.score >= 70 && s.score < 80).length || 0, color: "#eab308" },
					{ range: "60-69%", count: recent_scans?.filter(s => s.score >= 60 && s.score < 70).length || 0, color: "#f97316" },
					{ range: "<60%", count: recent_scans?.filter(s => s.score < 60).length || 0, color: "#ef4444" },
				];

				return (
					<div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
						{/* Host Compliance Distribution */}
						<div className="bg-secondary-800 rounded-lg border border-secondary-700 p-4">
							<h3 className="text-white font-medium mb-4 flex items-center gap-2">
								<PieChartIcon className="h-4 w-4 text-primary-400" />
								Host Compliance Status
							</h3>
							<div className="h-48">
								<ResponsiveContainer width="100%" height="100%">
									<PieChart>
										<Pie
											data={hostDistribution}
											cx="50%"
											cy="50%"
											innerRadius={40}
											outerRadius={70}
											dataKey="value"
											label={({ name, value }) => `${value}`}
											labelLine={false}
										>
											{hostDistribution.map((entry, index) => (
												<Cell key={`cell-${index}`} fill={entry.color} />
											))}
										</Pie>
										<Tooltip
											contentStyle={{
												backgroundColor: "#1f2937",
												border: "1px solid #374151",
												borderRadius: "0.5rem",
											}}
											formatter={(value, name) => [value, name]}
										/>
									</PieChart>
								</ResponsiveContainer>
							</div>
							<div className="flex flex-wrap justify-center gap-4 mt-2">
								{hostDistribution.map((entry) => (
									<div key={entry.name} className="flex items-center gap-2 text-sm">
										<div className="w-3 h-3 rounded-full" style={{ backgroundColor: entry.color }} />
										<span className="text-secondary-400">{entry.name}: {entry.value}</span>
									</div>
								))}
							</div>
						</div>

						{/* Recent Scan Score Distribution */}
						{recent_scans && recent_scans.length > 0 && (
							<div className="bg-secondary-800 rounded-lg border border-secondary-700 p-4">
								<h3 className="text-white font-medium mb-4 flex items-center gap-2">
									<BarChart3 className="h-4 w-4 text-primary-400" />
									Recent Scans by Score
								</h3>
								<div className="h-48">
									<ResponsiveContainer width="100%" height="100%">
										<BarChart data={scoreRanges} layout="vertical">
											<XAxis type="number" stroke="#6b7280" fontSize={12} />
											<YAxis type="category" dataKey="range" stroke="#6b7280" fontSize={12} width={60} />
											<Tooltip
												contentStyle={{
													backgroundColor: "#1f2937",
													border: "1px solid #374151",
													borderRadius: "0.5rem",
												}}
												formatter={(value) => [value, "Scans"]}
											/>
											<Bar dataKey="count" radius={[0, 4, 4, 0]}>
												{scoreRanges.map((entry, index) => (
													<Cell key={`cell-${index}`} fill={entry.color} />
												))}
											</Bar>
										</BarChart>
									</ResponsiveContainer>
								</div>
							</div>
						)}
					</div>
				);
			})()}

			{/* Additional Charts - Severity & Profile Distribution */}
			{((severity_breakdown && severity_breakdown.length > 0) || (profile_distribution && profile_distribution.length > 0)) && (
				<div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
					{/* Severity Breakdown */}
					{severity_breakdown && severity_breakdown.length > 0 && (() => {
						const severityColors = {
							critical: "#ef4444",
							high: "#f97316",
							medium: "#eab308",
							low: "#22c55e",
							unknown: "#6b7280",
						};
						const severityData = severity_breakdown.map(s => ({
							name: s.severity.charAt(0).toUpperCase() + s.severity.slice(1),
							value: s.count,
							color: severityColors[s.severity] || severityColors.unknown,
						}));

						return (
							<div className="bg-secondary-800 rounded-lg border border-secondary-700 p-4">
								<h3 className="text-white font-medium mb-4 flex items-center gap-2">
									<AlertTriangle className="h-4 w-4 text-primary-400" />
									Failed Rules by Severity
								</h3>
								<div className="h-48">
									<ResponsiveContainer width="100%" height="100%">
										<PieChart>
											<Pie
												data={severityData}
												cx="50%"
												cy="50%"
												innerRadius={40}
												outerRadius={70}
												dataKey="value"
												label={({ name, value }) => `${value}`}
												labelLine={false}
											>
												{severityData.map((entry, index) => (
													<Cell key={`cell-${index}`} fill={entry.color} />
												))}
											</Pie>
											<Tooltip
												contentStyle={{
													backgroundColor: "#1f2937",
													border: "1px solid #374151",
													borderRadius: "0.5rem",
												}}
												formatter={(value, name) => [value, name]}
											/>
										</PieChart>
									</ResponsiveContainer>
								</div>
								<div className="flex flex-wrap justify-center gap-4 mt-2">
									{severityData.map((entry) => (
										<div key={entry.name} className="flex items-center gap-2 text-sm">
											<div className="w-3 h-3 rounded-full" style={{ backgroundColor: entry.color }} />
											<span className="text-secondary-400">{entry.name}: {entry.value}</span>
										</div>
									))}
								</div>
							</div>
						);
					})()}

					{/* Profile Distribution */}
					{profile_distribution && profile_distribution.length > 0 && (
						<div className="bg-secondary-800 rounded-lg border border-secondary-700 p-4">
							<h3 className="text-white font-medium mb-4 flex items-center gap-2">
								<Shield className="h-4 w-4 text-primary-400" />
								Profiles in Use
							</h3>
							<div className="h-48">
								<ResponsiveContainer width="100%" height="100%">
									<BarChart data={profile_distribution} layout="vertical">
										<XAxis type="number" stroke="#6b7280" fontSize={12} />
										<YAxis
											type="category"
											dataKey="name"
											stroke="#6b7280"
											fontSize={11}
											width={140}
											tickFormatter={(value) => value.length > 20 ? value.slice(0, 20) + "..." : value}
										/>
										<Tooltip
											contentStyle={{
												backgroundColor: "#1f2937",
												border: "1px solid #374151",
												borderRadius: "0.5rem",
											}}
											formatter={(value) => [value, "Hosts"]}
										/>
										<Bar dataKey="host_count" fill="#6366f1" radius={[0, 4, 4, 0]} />
									</BarChart>
								</ResponsiveContainer>
							</div>
						</div>
					)}
				</div>
			)}

			{/* Top Failing Rules */}
			{top_failing_rules && top_failing_rules.length > 0 && (
				<div className="bg-secondary-800 rounded-lg border border-secondary-700">
					<div className="px-4 py-3 border-b border-secondary-700">
						<h2 className="text-lg font-semibold text-white flex items-center gap-2">
							<XCircle className="h-5 w-5 text-red-400" />
							Top Failing Rules
						</h2>
					</div>
					<div className="divide-y divide-secondary-700">
						{top_failing_rules.map((rule) => {
							const severityColors = {
								critical: "bg-red-500/20 text-red-400 border-red-500/30",
								high: "bg-orange-500/20 text-orange-400 border-orange-500/30",
								medium: "bg-yellow-500/20 text-yellow-400 border-yellow-500/30",
								low: "bg-green-500/20 text-green-400 border-green-500/30",
							};
							const severityClass = severityColors[rule.severity] || "bg-secondary-700 text-secondary-400 border-secondary-600";

							return (
								<div key={rule.rule_id} className="flex items-center justify-between px-4 py-3">
									<div className="flex-1 min-w-0">
										<p className="text-white font-medium truncate">{rule.title}</p>
										<p className="text-sm text-secondary-400 truncate">{rule.rule_id}</p>
									</div>
									<div className="flex items-center gap-3 ml-4">
										<span className={`px-2 py-0.5 rounded text-xs font-medium border ${severityClass}`}>
											{rule.severity}
										</span>
										<span className="text-red-400 font-bold whitespace-nowrap">
											{rule.fail_count} {rule.fail_count === 1 ? "host" : "hosts"}
										</span>
									</div>
								</div>
							);
						})}
					</div>
				</div>
			)}

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
