import { useQuery } from "@tanstack/react-query";
import {
	AlertTriangle,
	ArrowLeft,
	BookOpen,
	CheckCircle,
	ExternalLink,
	Info,
	RefreshCw,
	Shield,
	ShieldAlert,
	ShieldCheck,
	ShieldX,
	Users,
	Wrench,
	XCircle,
} from "lucide-react";
import ReactMarkdown from "react-markdown";
import { Link, useParams } from "react-router-dom";
import { complianceAPI } from "../../utils/complianceApi";

const SEVERITY_COLORS = {
	critical:
		"bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300 border-red-200 dark:border-red-800",
	high: "bg-orange-100 text-orange-800 dark:bg-orange-900/30 dark:text-orange-300 border-orange-200 dark:border-orange-800",
	medium:
		"bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-300 border-yellow-200 dark:border-yellow-800",
	low: "bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300 border-blue-200 dark:border-blue-800",
	unknown:
		"bg-secondary-100 text-secondary-600 dark:bg-secondary-700 dark:text-secondary-400 border-secondary-200 dark:border-secondary-600",
};

const STATUS_CONFIG = {
	fail: {
		label: "Fail",
		icon: XCircle,
		color: "text-red-600 dark:text-red-400",
		bg: "bg-red-100 dark:bg-red-900/30",
	},
	warn: {
		label: "Warning",
		icon: AlertTriangle,
		color: "text-yellow-600 dark:text-yellow-400",
		bg: "bg-yellow-100 dark:bg-yellow-900/30",
	},
	pass: {
		label: "Pass",
		icon: CheckCircle,
		color: "text-green-600 dark:text-green-400",
		bg: "bg-green-100 dark:bg-green-900/30",
	},
	error: {
		label: "Error",
		icon: AlertTriangle,
		color: "text-red-600 dark:text-red-400",
		bg: "bg-red-100 dark:bg-red-900/30",
	},
	skip: {
		label: "Skipped",
		icon: Shield,
		color: "text-secondary-500 dark:text-secondary-400",
		bg: "bg-secondary-100 dark:bg-secondary-700/50",
	},
	notapplicable: {
		label: "N/A",
		icon: Shield,
		color: "text-secondary-500 dark:text-secondary-400",
		bg: "bg-secondary-100 dark:bg-secondary-700/50",
	},
};

export default function RuleDetail() {
	const { id: ruleId } = useParams();
	const { data, isLoading, isError, error, refetch } = useQuery({
		queryKey: ["compliance-rule-detail", ruleId],
		queryFn: () => complianceAPI.getRuleDetail(ruleId),
		enabled: !!ruleId,
	});

	if (isLoading) {
		return (
			<div className="flex items-center justify-center py-24">
				<RefreshCw className="h-8 w-8 animate-spin text-primary-500" />
			</div>
		);
	}

	if (isError || !data?.rule) {
		return (
			<div className="card p-8 text-center">
				<ShieldAlert className="h-12 w-12 text-amber-500 mx-auto mb-3" />
				<p className="text-secondary-600 dark:text-secondary-400">
					{error?.message || "Rule not found"}
				</p>
				<Link
					to="/compliance"
					className="inline-flex items-center gap-2 mt-4 text-primary-600 dark:text-primary-400 hover:underline"
				>
					<ArrowLeft className="h-4 w-4" />
					Back to Compliance
				</Link>
			</div>
		);
	}

	const { rule, affected_hosts = [] } = data;
	const counts = affected_hosts.reduce((acc, h) => {
		acc[h.status] = (acc[h.status] || 0) + 1;
		return acc;
	}, {});

	return (
		<div className="space-y-6">
			{/* Back link */}
			<Link
				to="/compliance"
				className="inline-flex items-center gap-2 text-sm text-secondary-600 dark:text-secondary-400 hover:text-primary-600 dark:hover:text-primary-400"
			>
				<ArrowLeft className="h-4 w-4" />
				Back to Compliance
			</Link>

			{/* Header */}
			<div className="flex flex-wrap items-start justify-between gap-4">
				<div>
					<h1 className="text-xl font-semibold text-secondary-900 dark:text-white">
						{rule.title}
					</h1>
					<div className="flex flex-wrap items-center gap-2 mt-2">
						{rule.rule_ref && (
							<span className="text-xs font-mono text-secondary-500 dark:text-secondary-400">
								{rule.rule_ref}
							</span>
						)}
						{rule.section && (
							<span className="text-xs text-secondary-500 dark:text-secondary-400">
								Section {rule.section}
							</span>
						)}
						<span
							className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium border ${
								SEVERITY_COLORS[rule.severity || "unknown"] ||
								SEVERITY_COLORS.unknown
							}`}
						>
							{(rule.severity || "unknown").charAt(0).toUpperCase() +
								(rule.severity || "unknown").slice(1)}
						</span>
						<span
							className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${
								rule.profile_type === "docker-bench"
									? "bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300"
									: "bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300"
							}`}
						>
							{rule.profile_type === "docker-bench"
								? "Docker Bench"
								: "OpenSCAP"}
						</span>
					</div>
				</div>
				<button
					type="button"
					onClick={() => refetch()}
					className="p-2 rounded-lg border border-secondary-200 dark:border-secondary-600 text-secondary-600 dark:text-secondary-400 hover:bg-secondary-50 dark:hover:bg-secondary-700"
					title="Refresh"
				>
					<RefreshCw className="h-4 w-4" />
				</button>
			</div>

			{/* Summary Cards */}
			<div className="grid grid-cols-2 sm:grid-cols-4 gap-3 sm:gap-4">
				<div className="card p-4 text-left w-full">
					<div className="flex items-center">
						<Users className="h-5 w-5 text-primary-600 mr-2" />
						<div>
							<p className="text-sm text-secondary-500 dark:text-white">
								Affected Hosts
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{affected_hosts.length}
							</p>
						</div>
					</div>
				</div>
				<div className="card p-4 text-left w-full">
					<div className="flex items-center">
						<ShieldCheck className="h-5 w-5 text-green-600 mr-2" />
						<div>
							<p className="text-sm text-secondary-500 dark:text-white">
								Passing
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{counts.pass || 0}
							</p>
						</div>
					</div>
				</div>
				<div className="card p-4 text-left w-full">
					<div className="flex items-center">
						<ShieldX className="h-5 w-5 text-red-600 mr-2" />
						<div>
							<p className="text-sm text-secondary-500 dark:text-white">
								Failing
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{counts.fail || 0}
							</p>
						</div>
					</div>
				</div>
				<div className="card p-4 text-left w-full">
					<div className="flex items-center">
						<AlertTriangle className="h-5 w-5 text-yellow-600 mr-2" />
						<div>
							<p className="text-sm text-secondary-500 dark:text-white">
								Warnings
							</p>
							<p className="text-xl font-semibold text-secondary-900 dark:text-white">
								{counts.warn || 0}
							</p>
						</div>
					</div>
				</div>
			</div>

			{/* Rule Details */}
			<div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
				{/* Left column - Details */}
				<div className="lg:col-span-2 space-y-4">
					{/* Description */}
					{rule.description && (
						<div className="card p-5">
							<div className="flex items-center gap-2 mb-3">
								<BookOpen className="h-4 w-4 text-primary-400" />
								<h2 className="text-sm font-semibold text-secondary-900 dark:text-white">
									Description
								</h2>
							</div>
							<p className="text-sm text-secondary-600 dark:text-secondary-300 whitespace-pre-wrap leading-relaxed">
								{rule.description}
							</p>
						</div>
					)}

					{/* Rationale */}
					{rule.rationale && (
						<div className="card p-5">
							<div className="flex items-center gap-2 mb-3">
								<Info className="h-4 w-4 text-blue-400" />
								<h2 className="text-sm font-semibold text-secondary-900 dark:text-white">
									Rationale
								</h2>
							</div>
							<p className="text-sm text-secondary-600 dark:text-secondary-300 whitespace-pre-wrap leading-relaxed">
								{rule.rationale}
							</p>
						</div>
					)}

					{/* Remediation - render as markdown (CIS/OpenSCAP often use headers, lists, code blocks) */}
					{rule.remediation && (
						<div className="card p-5">
							<div className="flex items-center gap-2 mb-3">
								<Wrench className="h-4 w-4 text-amber-400" />
								<h2 className="text-sm font-semibold text-secondary-900 dark:text-white">
									Remediation
								</h2>
							</div>
							<div className="remediation-markdown text-sm text-secondary-600 dark:text-secondary-300 leading-relaxed">
								<ReactMarkdown
									components={{
										p: ({ node, ...props }) => (
											<p className="mb-2 last:mb-0" {...props} />
										),
										ul: ({ node, ...props }) => (
											<ul
												className="list-disc list-inside mb-2 ml-2 space-y-1"
												{...props}
											/>
										),
										ol: ({ node, ...props }) => (
											<ol
												className="list-decimal list-inside mb-2 ml-2 space-y-1"
												{...props}
											/>
										),
										li: ({ node, ...props }) => (
											<li className="ml-1" {...props} />
										),
										strong: ({ node, ...props }) => (
											<strong
												className="font-semibold text-secondary-900 dark:text-white"
												{...props}
											/>
										),
										code: ({ node, className, ...props }) => {
											const is_block = className?.startsWith("language-");
											return is_block ? (
												<code
													className="block bg-secondary-100 dark:bg-secondary-700 rounded-lg p-3 font-mono text-xs overflow-x-auto whitespace-pre"
													{...props}
												/>
											) : (
												<code
													className="bg-secondary-100 dark:bg-secondary-700 px-1 py-0.5 rounded font-mono text-xs"
													{...props}
												/>
											);
										},
										pre: ({ node, ...props }) => (
											<div
												className="my-2 overflow-x-auto rounded-lg"
												{...props}
											/>
										),
										h1: ({ node, ...props }) => (
											<h1
												className="text-base font-semibold text-secondary-900 dark:text-white mt-3 mb-1 first:mt-0"
												{...props}
											/>
										),
										h2: ({ node, ...props }) => (
											<h2
												className="text-sm font-semibold text-secondary-900 dark:text-white mt-3 mb-1 first:mt-0"
												{...props}
											/>
										),
										h3: ({ node, ...props }) => (
											<h3
												className="text-sm font-medium text-secondary-900 dark:text-white mt-2 mb-1"
												{...props}
											/>
										),
									}}
								>
									{rule.remediation}
								</ReactMarkdown>
							</div>
						</div>
					)}
				</div>

				{/* Right column - Affected Hosts */}
				<div>
					<div className="card overflow-hidden">
						<div className="px-4 py-3 border-b border-secondary-200 dark:border-secondary-700">
							<h2 className="text-sm font-semibold text-secondary-900 dark:text-white">
								Affected Hosts ({affected_hosts.length})
							</h2>
						</div>
						{affected_hosts.length === 0 ? (
							<div className="p-6 text-center text-secondary-400 text-sm">
								No hosts have been scanned with this rule yet
							</div>
						) : (
							<div className="divide-y divide-secondary-200 dark:divide-secondary-700">
								{affected_hosts.map((host) => {
									const config =
										STATUS_CONFIG[host.status] || STATUS_CONFIG.skip;
									const StatusIcon = config.icon;
									const is_fail_or_warn =
										host.status === "fail" || host.status === "warn";
									const has_why_info =
										host.finding ||
										host.actual ||
										(rule.description && rule.description.length > 10);
									const show_why = is_fail_or_warn && has_why_info;

									return (
										<div
											key={host.host_id}
											className="px-4 py-3 hover:bg-secondary-50 dark:hover:bg-secondary-700/50 transition-colors"
										>
											<Link
												to={`/compliance/hosts/${host.host_id}`}
												className="flex items-center justify-between gap-2"
											>
												<div className="flex items-center gap-3 min-w-0">
													<StatusIcon
														className={`h-4 w-4 shrink-0 ${config.color}`}
													/>
													<div className="min-w-0">
														<p className="text-sm font-medium text-secondary-900 dark:text-white truncate">
															{host.friendly_name || host.hostname || "Host"}
														</p>
														{host.hostname &&
															host.friendly_name &&
															host.hostname !== host.friendly_name && (
																<p className="text-xs text-secondary-500 dark:text-secondary-400 truncate">
																	{host.hostname}
																</p>
															)}
													</div>
												</div>
												<span
													className={`shrink-0 px-2 py-0.5 rounded text-xs font-medium ${config.bg} ${config.color}`}
												>
													{config.label}
												</span>
												<ExternalLink className="h-3.5 w-3.5 shrink-0 text-secondary-400" />
											</Link>
											{show_why && (
												<div
													className={`mt-2 pt-2 border-t border-secondary-200 dark:border-secondary-600 text-xs ${
														host.status === "warn"
															? "text-yellow-700 dark:text-yellow-300/90"
															: "text-red-700 dark:text-red-300/90"
													}`}
												>
													<p className="font-medium mb-1">
														{host.status === "warn"
															? "Why this warning"
															: "Why this failed"}
													</p>
													<div className="space-y-2">
														{host.finding && (
															<p className="leading-relaxed">{host.finding}</p>
														)}
														{!host.finding && host.actual && (
															<>
																<p>The check found a non-compliant value:</p>
																<div className="mt-1.5 grid grid-cols-1 gap-1.5">
																	<div className="bg-red-100 dark:bg-red-900/30 rounded p-2">
																		<span className="text-red-700 dark:text-red-300 text-xs font-medium">
																			Current:
																		</span>
																		<code className="block mt-0.5 text-red-800 dark:text-red-200 break-all font-mono">
																			{host.actual}
																		</code>
																	</div>
																	{host.expected && (
																		<div className="bg-green-100 dark:bg-green-900/30 rounded p-2">
																			<span className="text-green-700 dark:text-green-300 text-xs font-medium">
																				Required:
																			</span>
																			<code className="block mt-0.5 text-green-800 dark:text-green-200 break-all font-mono">
																				{host.expected}
																			</code>
																		</div>
																	)}
																</div>
															</>
														)}
														{!host.finding &&
															!host.actual &&
															rule.description && (
																<p className="leading-relaxed">
																	{rule.description
																		.replace(/\s+/g, " ")
																		.trim()
																		.substring(0, 300)}
																	{rule.description.length > 300 ? "…" : ""}
																</p>
															)}
													</div>
												</div>
											)}
										</div>
									);
								})}
							</div>
						)}
					</div>
				</div>
			</div>
		</div>
	);
}
