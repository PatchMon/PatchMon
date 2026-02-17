import { useQuery } from "@tanstack/react-query";
import {
	AlertTriangle,
	ArrowLeft,
	BookOpen,
	Check,
	CheckCircle,
	Copy,
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
import { useState } from "react";
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

function fallback_copy(text, on_success) {
	const textarea = document.createElement("textarea");
	textarea.value = text;
	textarea.style.position = "fixed";
	textarea.style.left = "-9999px";
	textarea.style.top = "0";
	document.body.appendChild(textarea);
	textarea.focus();
	textarea.select();
	try {
		const ok = document.execCommand("copy");
		if (ok) on_success();
	} finally {
		document.body.removeChild(textarea);
	}
}

function get_what_fix_does(remediation, title) {
	if (!remediation?.trim()) return null;
	const r = remediation.trim();
	const t = (title || "").toLowerCase();
	if (r.includes("sysctl") || r.includes("/proc/sys"))
		return "This fix will modify kernel parameters to enable the required security setting. Changes are applied immediately and persist across reboots.";
	if (r.includes("chmod") || r.includes("chown"))
		return "This fix will update file permissions or ownership to meet the required security standard. This restricts unauthorized access to sensitive files.";
	if (r.includes("apt") || r.includes("yum") || r.includes("dnf"))
		return "This fix will install, update, or remove packages as needed to meet the security requirement.";
	if (r.includes("systemctl") || r.includes("service"))
		return "This fix will enable, disable, or configure a system service to meet the security requirement.";
	if (r.includes("/etc/ssh"))
		return "This fix will update SSH daemon configuration to harden remote access security.";
	if (r.includes("audit") || r.includes("auditd"))
		return "This fix will configure audit logging to track security-relevant system events.";
	if (r.includes("pam") || r.includes("/etc/pam"))
		return "This fix will configure authentication modules to enforce stronger access controls.";
	if (t.includes("password"))
		return "This fix will update password policy settings to require stronger passwords or enforce better credential management.";
	if (
		t.includes("firewall") ||
		r.includes("iptables") ||
		r.includes("nftables")
	)
		return "This fix will configure firewall rules to restrict network access and improve security.";
	return "This fix will apply the recommended configuration change to bring your system into compliance with the security benchmark.";
}

export default function RuleDetail() {
	const { id: ruleId } = useParams();
	const [remediation_copied, set_remediation_copied] = useState(false);
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

	const copy_remediation = () => {
		if (!rule.remediation) return;
		const text = rule.remediation;

		const do_feedback = () => {
			set_remediation_copied(true);
			setTimeout(() => set_remediation_copied(false), 2000);
		};

		if (navigator.clipboard?.writeText) {
			navigator.clipboard
				.writeText(text)
				.then(do_feedback)
				.catch(() => {
					fallback_copy(text, do_feedback);
				});
		} else {
			fallback_copy(text, do_feedback);
		}
	};

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

			{/* Rule Details: Description (left 1/2) + Remediation (right 1/2), Why under Description */}
			<div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
				{/* Left 1/2: Description then Why this failed (Rationale) */}
				<div className="space-y-4">
					{/* Description */}
					<div className="card p-5">
						<div className="flex items-center gap-2 mb-3">
							<BookOpen className="h-4 w-4 text-primary-400" />
							<h2 className="text-sm font-semibold text-secondary-900 dark:text-white">
								Description
							</h2>
						</div>
						<p className="text-sm text-secondary-600 dark:text-secondary-300 whitespace-pre-wrap leading-relaxed">
							{rule.description || "—"}
						</p>
					</div>

					{/* Why this failed / Rationale - under Description */}
					<div className="card p-5">
						<div className="flex items-center gap-2 mb-3">
							<Info className="h-4 w-4 text-blue-400" />
							<h2 className="text-sm font-semibold text-secondary-900 dark:text-white">
								Why this failed
							</h2>
						</div>
						<p className="text-sm text-secondary-600 dark:text-secondary-300 whitespace-pre-wrap leading-relaxed">
							{rule.rationale || "No rationale provided."}
						</p>
					</div>
				</div>

				{/* Right 1/2: What the fix does + Remediation */}
				<div className="space-y-4">
					{/* What the fix does - plain-language explanation */}
					{(() => {
						const what_fix = get_what_fix_does(rule.remediation, rule.title);
						return what_fix ? (
							<div className="card p-5 bg-amber-50 dark:bg-amber-900/10 border-amber-200 dark:border-amber-800/50">
								<div className="flex items-center gap-2 mb-2">
									<Wrench className="h-4 w-4 text-amber-500 dark:text-amber-400" />
									<h2 className="text-sm font-semibold text-secondary-900 dark:text-white">
										What the fix does
									</h2>
								</div>
								<p className="text-sm text-secondary-700 dark:text-secondary-300 leading-relaxed">
									{what_fix}
								</p>
							</div>
						) : null;
					})()}
					{/* Remediation script */}
					<div className="card p-5">
						<div className="flex items-center justify-between gap-2 mb-3">
							<div className="flex items-center gap-2">
								<Wrench className="h-4 w-4 text-amber-400" />
								<h2 className="text-sm font-semibold text-secondary-900 dark:text-white">
									Remediation
								</h2>
							</div>
							{rule.remediation && (
								<button
									type="button"
									onClick={copy_remediation}
									className="inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg text-xs font-medium bg-secondary-100 dark:bg-secondary-700 text-secondary-700 dark:text-secondary-300 hover:bg-secondary-200 dark:hover:bg-secondary-600 border border-secondary-200 dark:border-secondary-600 transition-colors"
								>
									{remediation_copied ? (
										<>
											<Check className="h-3.5 w-3.5 text-green-600 dark:text-green-400" />
											Copied
										</>
									) : (
										<>
											<Copy className="h-3.5 w-3.5" />
											Copy
										</>
									)}
								</button>
							)}
						</div>
						{rule.remediation ? (
							<pre className="text-xs font-mono text-secondary-700 dark:text-secondary-300 bg-secondary-100 dark:bg-secondary-700/50 border border-secondary-200 dark:border-secondary-600 rounded-lg p-4 overflow-x-auto whitespace-pre-wrap leading-relaxed">
								{rule.remediation}
							</pre>
						) : (
							<p className="text-sm text-secondary-500 dark:text-secondary-400">
								No remediation steps available.
							</p>
						)}
					</div>
				</div>
			</div>

			{/* Affected Hosts table - same design as Hosts page */}
			<div className="card p-4 md:p-6 overflow-hidden">
				<h2 className="text-lg font-medium text-secondary-900 dark:text-white mb-4">
					Hosts this rule affects ({affected_hosts.length})
				</h2>
				{affected_hosts.length === 0 ? (
					<div className="py-8 text-center text-secondary-500 dark:text-secondary-400 text-sm">
						No hosts have been scanned with this rule yet
					</div>
				) : (
					<div className="overflow-x-auto">
						<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-600">
							<thead className="bg-secondary-50 dark:bg-secondary-700">
								<tr>
									<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider whitespace-nowrap">
										Host
									</th>
									<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider whitespace-nowrap w-24">
										Status
									</th>
									<th className="px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 whitespace-nowrap">
										Why (this host)
									</th>
								</tr>
							</thead>
							<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-600 text-sm">
								{affected_hosts.map((host) => {
									const config =
										STATUS_CONFIG[host.status] || STATUS_CONFIG.skip;
									const StatusIcon = config.icon;
									const why_text =
										host.finding ||
										(host.actual
											? `Current: ${host.actual}${host.expected ? ` → Required: ${host.expected}` : ""}`
											: null);
									return (
										<tr
											key={host.host_id}
											className="hover:bg-secondary-50 dark:hover:bg-secondary-700/50 transition-colors"
										>
											<td className="px-4 py-2 whitespace-nowrap">
												<Link
													to={`/compliance/hosts/${host.host_id}`}
													className="text-secondary-900 dark:text-white hover:text-primary-600 dark:hover:text-primary-400 hover:underline font-medium"
												>
													{host.friendly_name || host.hostname || "Host"}
												</Link>
												{host.hostname &&
													host.friendly_name &&
													host.hostname !== host.friendly_name && (
														<p className="text-xs text-secondary-500 dark:text-secondary-400 truncate">
															{host.hostname}
														</p>
													)}
											</td>
											<td className="px-4 py-2 whitespace-nowrap">
												<span
													className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded text-xs font-medium ${config.bg} ${config.color}`}
												>
													<StatusIcon className="h-3.5 w-3.5" />
													{config.label}
												</span>
											</td>
											<td className="px-4 py-2 text-secondary-600 dark:text-secondary-400 max-w-md">
												{why_text ? (
													<span className="line-clamp-2 text-xs">
														{why_text}
													</span>
												) : (
													<span className="text-secondary-400">—</span>
												)}
											</td>
										</tr>
									);
								})}
							</tbody>
						</table>
					</div>
				)}
			</div>
		</div>
	);
}
