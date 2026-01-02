import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
	Shield,
	Play,
	RefreshCw,
	CheckCircle,
	XCircle,
	AlertTriangle,
	MinusCircle,
	ChevronDown,
	ChevronRight,
} from "lucide-react";
import { complianceAPI } from "../../utils/complianceApi";
import ComplianceScore from "./ComplianceScore";

const ComplianceTab = ({ hostId, isConnected }) => {
	const [expandedRules, setExpandedRules] = useState({});
	const [statusFilter, setStatusFilter] = useState("all");
	const queryClient = useQueryClient();

	const { data: latestScan, isLoading } = useQuery({
		queryKey: ["compliance-latest", hostId],
		queryFn: () => complianceAPI.getLatestScan(hostId).then((res) => res.data),
		enabled: !!hostId,
	});

	const { data: scanHistory } = useQuery({
		queryKey: ["compliance-history", hostId],
		queryFn: () => complianceAPI.getHostScans(hostId, { limit: 5 }).then((res) => res.data),
		enabled: !!hostId,
	});

	const triggerScan = useMutation({
		mutationFn: (profileType) => complianceAPI.triggerScan(hostId, profileType),
		onSuccess: () => {
			queryClient.invalidateQueries(["compliance-latest", hostId]);
			queryClient.invalidateQueries(["compliance-history", hostId]);
		},
	});

	const toggleRule = (ruleId) => {
		setExpandedRules((prev) => ({
			...prev,
			[ruleId]: !prev[ruleId],
		}));
	};

	const getStatusIcon = (status) => {
		switch (status) {
			case "pass":
				return <CheckCircle className="h-4 w-4 text-green-400" />;
			case "fail":
				return <XCircle className="h-4 w-4 text-red-400" />;
			case "warn":
				return <AlertTriangle className="h-4 w-4 text-yellow-400" />;
			default:
				return <MinusCircle className="h-4 w-4 text-secondary-400" />;
		}
	};

	const filteredResults = latestScan?.results?.filter((r) =>
		statusFilter === "all" ? true : r.status === statusFilter
	);

	if (isLoading) {
		return (
			<div className="flex items-center justify-center h-64">
				<div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-500"></div>
			</div>
		);
	}

	return (
		<div className="space-y-6">
			{/* Header with Scan Button */}
			<div className="flex items-center justify-between">
				<div className="flex items-center gap-3">
					<Shield className="h-6 w-6 text-primary-400" />
					<h2 className="text-xl font-semibold text-white">Security Compliance</h2>
				</div>
				<button
					onClick={() => triggerScan.mutate("all")}
					disabled={!isConnected || triggerScan.isPending}
					className="flex items-center gap-2 px-4 py-2 bg-primary-600 hover:bg-primary-700 disabled:bg-secondary-700 disabled:cursor-not-allowed text-white rounded-lg transition-colors"
				>
					{triggerScan.isPending ? (
						<RefreshCw className="h-4 w-4 animate-spin" />
					) : (
						<Play className="h-4 w-4" />
					)}
					Run Scan
				</button>
			</div>

			{/* Latest Scan Summary */}
			{latestScan ? (
				<div className="bg-secondary-800 rounded-lg border border-secondary-700 p-4">
					<div className="flex items-center justify-between mb-4">
						<div>
							<p className="text-sm text-secondary-400">Latest Scan</p>
							<p className="text-white font-medium">{latestScan.profile?.name}</p>
							<p className="text-xs text-secondary-500">
								{new Date(latestScan.completed_at).toLocaleString()}
							</p>
						</div>
						<ComplianceScore score={latestScan.score} size="lg" />
					</div>

					{/* Stats */}
					<div className="grid grid-cols-5 gap-2 text-center">
						<div className="bg-secondary-700/50 rounded p-2">
							<p className="text-lg font-bold text-white">{latestScan.total_rules}</p>
							<p className="text-xs text-secondary-400">Total</p>
						</div>
						<div className="bg-green-900/20 rounded p-2">
							<p className="text-lg font-bold text-green-400">{latestScan.passed}</p>
							<p className="text-xs text-secondary-400">Passed</p>
						</div>
						<div className="bg-red-900/20 rounded p-2">
							<p className="text-lg font-bold text-red-400">{latestScan.failed}</p>
							<p className="text-xs text-secondary-400">Failed</p>
						</div>
						<div className="bg-yellow-900/20 rounded p-2">
							<p className="text-lg font-bold text-yellow-400">{latestScan.warnings}</p>
							<p className="text-xs text-secondary-400">Warnings</p>
						</div>
						<div className="bg-secondary-700/50 rounded p-2">
							<p className="text-lg font-bold text-secondary-400">{latestScan.skipped + latestScan.not_applicable}</p>
							<p className="text-xs text-secondary-400">Skipped</p>
						</div>
					</div>
				</div>
			) : (
				<div className="bg-secondary-800 rounded-lg border border-secondary-700 p-8 text-center">
					<Shield className="h-12 w-12 text-secondary-600 mx-auto mb-3" />
					<p className="text-secondary-400">No compliance scans yet</p>
					<p className="text-sm text-secondary-500 mt-1">
						Run a scan to check this host against CIS benchmarks
					</p>
				</div>
			)}

			{/* Results Filter */}
			{latestScan?.results && (
				<div className="flex gap-2">
					{["all", "fail", "warn", "pass"].map((status) => (
						<button
							key={status}
							onClick={() => setStatusFilter(status)}
							className={`px-3 py-1 rounded-full text-sm capitalize transition-colors ${
								statusFilter === status
									? "bg-primary-600 text-white"
									: "bg-secondary-700 text-secondary-300 hover:bg-secondary-600"
							}`}
						>
							{status}
						</button>
					))}
				</div>
			)}

			{/* Results List */}
			{filteredResults && filteredResults.length > 0 && (
				<div className="bg-secondary-800 rounded-lg border border-secondary-700 divide-y divide-secondary-700">
					{filteredResults.map((result) => (
						<div key={result.id} className="p-3">
							<button
								onClick={() => toggleRule(result.id)}
								className="w-full flex items-center gap-3 text-left"
							>
								{expandedRules[result.id] ? (
									<ChevronDown className="h-4 w-4 text-secondary-400 flex-shrink-0" />
								) : (
									<ChevronRight className="h-4 w-4 text-secondary-400 flex-shrink-0" />
								)}
								{getStatusIcon(result.status)}
								<div className="flex-1 min-w-0">
									<p className="text-white font-medium truncate">{result.rule?.title}</p>
									<p className="text-xs text-secondary-400">
										{result.rule?.section && `${result.rule.section} • `}
										{result.rule?.severity && (
											<span className={`capitalize ${
												result.rule.severity === "critical" ? "text-red-400" :
												result.rule.severity === "high" ? "text-orange-400" :
												result.rule.severity === "medium" ? "text-yellow-400" :
												"text-secondary-400"
											}`}>
												{result.rule.severity}
											</span>
										)}
									</p>
								</div>
							</button>

							{expandedRules[result.id] && (
								<div className="mt-3 ml-7 space-y-2 text-sm">
									{result.rule?.description && (
										<div>
											<p className="text-secondary-400 font-medium">Description</p>
											<p className="text-secondary-300">{result.rule.description}</p>
										</div>
									)}
									{result.finding && (
										<div>
											<p className="text-secondary-400 font-medium">Finding</p>
											<p className="text-secondary-300">{result.finding}</p>
										</div>
									)}
									{result.rule?.remediation && (
										<div>
											<p className="text-secondary-400 font-medium">Remediation</p>
											<p className="text-secondary-300">{result.rule.remediation}</p>
										</div>
									)}
								</div>
							)}
						</div>
					))}
				</div>
			)}
		</div>
	);
};

export default ComplianceTab;
