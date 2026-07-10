import { useMutation, useQuery } from "@tanstack/react-query";
import { AlertTriangle, Database, Search, ShieldCheck } from "lucide-react";
import { useId, useState } from "react";
import { Link } from "react-router-dom";
import { dashboardAPI } from "../utils/api";

// CveDataSources shows the freshness of the CVE databases: last update attempt/
// success, whether it succeeded, how many CVEs and the newest CVE known.
const CveDataSources = () => {
	const { data } = useQuery({
		queryKey: ["cveDataSources"],
		queryFn: () => dashboardAPI.getCVEDataSources().then((r) => r.data),
		refetchInterval: 30000,
	});
	const sources = data?.sources || [];
	const fmt = (t) => (t ? new Date(t).toLocaleString() : "—");
	return (
		<details className="mb-6 border border-secondary-200 dark:border-secondary-700 rounded-lg">
			<summary className="px-4 py-2 cursor-pointer text-sm font-medium text-secondary-700 dark:text-secondary-200 flex items-center gap-2">
				<Database className="h-4 w-4" />
				CVE data sources ({sources.length})
			</summary>
			<div className="overflow-x-auto">
				<table className="min-w-full text-xs">
					<thead className="bg-secondary-50 dark:bg-secondary-700 uppercase text-secondary-500 dark:text-secondary-300">
						<tr>
							<th className="px-4 py-2 text-left">Source</th>
							<th className="px-4 py-2 text-left">Status</th>
							<th className="px-4 py-2 text-left">Last success</th>
							<th className="px-4 py-2 text-left">Last attempt</th>
							<th className="px-4 py-2 text-right">CVEs</th>
							<th className="px-4 py-2 text-left">Newest CVE</th>
						</tr>
					</thead>
					<tbody className="divide-y divide-secondary-100 dark:divide-secondary-700">
						{sources.length === 0 && (
							<tr>
								<td
									colSpan={6}
									className="px-4 py-3 text-secondary-500 dark:text-secondary-400"
								>
									No data sources loaded yet.
								</td>
							</tr>
						)}
						{sources.map((s) => (
							<tr key={s.source}>
								<td className="px-4 py-2 font-mono">{s.source}</td>
								<td className="px-4 py-2">
									{s.ok ? (
										<span className="text-green-600 dark:text-green-400">
											ok{s.from_disk ? " (disk)" : ""}
										</span>
									) : s.error ? (
										<span
											className="text-red-600 dark:text-red-400"
											title={s.error}
										>
											failed
										</span>
									) : (
										<span className="text-amber-600 dark:text-amber-400">
											loading…
										</span>
									)}
								</td>
								<td className="px-4 py-2">{fmt(s.last_success)}</td>
								<td className="px-4 py-2">{fmt(s.last_attempt)}</td>
								<td className="px-4 py-2 text-right">{s.count || 0}</td>
								<td className="px-4 py-2 font-mono">{s.newest_cve || "—"}</td>
							</tr>
						))}
					</tbody>
				</table>
			</div>
		</details>
	);
};

// CveReport lets an operator paste a list of CVE identifiers and get, per CVE,
// the hosts their distribution reports vulnerable (installed kernel package
// older than the distro's fixed version, or affected with no fix yet).
const CveReport = () => {
	const inputId = useId();
	const [input, setInput] = useState("");

	const mutation = useMutation({
		mutationFn: (cves) =>
			dashboardAPI.getCVEReport(cves).then((res) => res.data),
	});

	const runReport = () => {
		const cves = input.trim();
		if (cves) mutation.mutate(cves);
	};

	const results = mutation.data?.results || [];

	return (
		<div className="p-4 sm:p-6 max-w-6xl mx-auto">
			<div className="mb-6">
				<h1 className="text-2xl font-bold text-secondary-900 dark:text-white flex items-center gap-2">
					<AlertTriangle className="h-6 w-6 text-amber-500" />
					CVE vulnerability report
				</h1>
				<p className="mt-1 text-sm text-secondary-600 dark:text-secondary-300">
					Paste one or more CVE identifiers (any separator). Each host is
					checked against its distribution's security data (Ubuntu, Debian,
					RHEL, Fedora, Proxmox, CentOS/Alma) by comparing its installed kernel
					package against the distro's fixed version.
				</p>
			</div>

			<div className="mb-6">
				<label
					htmlFor={inputId}
					className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
				>
					CVE identifiers
				</label>
				<textarea
					id={inputId}
					value={input}
					onChange={(e) => setInput(e.target.value)}
					rows={3}
					placeholder="CVE-2026-43499, CVE-2022-0847 CVE-2022-0185 …"
					className="w-full border border-secondary-300 dark:border-secondary-600 rounded-lg px-3 py-2 focus:ring-2 focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-800 text-secondary-900 dark:text-white font-mono text-sm"
				/>
				<div className="mt-2 flex items-center gap-3">
					<button
						type="button"
						onClick={runReport}
						disabled={mutation.isPending || !input.trim()}
						className="btn-primary inline-flex items-center gap-2 disabled:opacity-50"
					>
						<Search className="h-4 w-4" />
						{mutation.isPending ? "Checking…" : "Run report"}
					</button>
					{mutation.data?.cves?.length > 0 && (
						<span className="text-xs text-secondary-500 dark:text-secondary-400">
							Checked {mutation.data.cves.length} CVE(s)
						</span>
					)}
				</div>
			</div>

			{mutation.isError && (
				<div className="mb-4 text-sm text-red-600 dark:text-red-400">
					{mutation.error?.response?.data?.error || "Report failed."}
				</div>
			)}

			<CveDataSources />

			<div className="space-y-6">
				{results.map((r) => (
					<CveResultCard key={r.cve_id} result={r} />
				))}
			</div>
		</div>
	);
};

const severityColor = (sev) => {
	switch ((sev || "").toUpperCase()) {
		case "CRITICAL":
			return "bg-red-600 text-white";
		case "HIGH":
			return "bg-orange-500 text-white";
		case "MEDIUM":
			return "bg-amber-500 text-white";
		case "LOW":
			return "bg-secondary-400 text-white";
		default:
			return "bg-secondary-300 text-secondary-800";
	}
};

const rangeFrom = (r) =>
	r.Exact ? `= ${r.Exact}` : r.Lo ? `${r.LoIncl ? "≥" : ">"} ${r.Lo}` : "(any)";
const rangeTo = (r) =>
	r.Exact ? "" : r.Hi ? `${r.HiIncl ? "≤" : "<"} ${r.Hi}` : "(open)";

// sourceLinks builds the canonical per-CVE advisory URLs (deterministic from the
// id — no network needed).
const sourceLinks = (id) => [
	{ name: "NVD", url: `https://nvd.nist.gov/vuln/detail/${id}` },
	{ name: "MITRE", url: `https://www.cve.org/CVERecord?id=${id}` },
	{ name: "Ubuntu", url: `https://ubuntu.com/security/${id}` },
	{ name: "Debian", url: `https://security-tracker.debian.org/tracker/${id}` },
	{ name: "Red Hat", url: `https://access.redhat.com/security/cve/${id}` },
];

const CveResultCard = ({ result }) => {
	const c = result.counts || {};
	const hosts = result.hosts || [];
	const nvd = result.nvd || {};
	return (
		<div className="border border-secondary-200 dark:border-secondary-700 rounded-lg overflow-hidden">
			<div className="px-4 py-3 bg-secondary-50 dark:bg-secondary-800 flex flex-wrap items-center justify-between gap-2">
				<div className="flex items-center gap-2">
					<a
						href={`https://nvd.nist.gov/vuln/detail/${result.cve_id}`}
						target="_blank"
						rel="noreferrer"
						className="font-mono font-semibold text-secondary-900 dark:text-white hover:underline"
					>
						{result.cve_id}
					</a>
					{nvd.cvss_severity && (
						<span
							className={`px-2 py-0.5 rounded text-xs font-medium ${severityColor(nvd.cvss_severity)}`}
							title={nvd.cvss_vector || ""}
						>
							{nvd.cvss_severity}
							{nvd.cvss_score ? ` ${nvd.cvss_score}` : ""}
						</span>
					)}
				</div>
				<div className="flex items-center gap-3 text-xs">
					<span className="text-red-600 dark:text-red-400 font-medium">
						{c.vulnerable || 0} vulnerable
					</span>
					<span className="text-green-600 dark:text-green-400">
						{c.patched || 0} patched
					</span>
					<span className="text-secondary-500 dark:text-secondary-400">
						{c.not_affected || 0} not affected
					</span>
					<span className="text-amber-600 dark:text-amber-400">
						{c.unknown || 0} unknown
					</span>
				</div>
			</div>

			<div className="px-4 py-2 border-b border-secondary-100 dark:border-secondary-700 text-xs text-secondary-600 dark:text-secondary-300 space-y-2">
				{nvd.description && <p>{nvd.description}</p>}
				<div className="flex flex-wrap gap-x-4 gap-y-1 text-secondary-400">
					{nvd.published && (
						<span>Published: {nvd.published.slice(0, 10)}</span>
					)}
					{nvd.last_modified && (
						<span>Updated: {nvd.last_modified.slice(0, 10)}</span>
					)}
				</div>
				{nvd.ranges && nvd.ranges.length > 0 && (
					<div>
						<div className="font-medium mb-1">
							Upstream affected kernel ranges (NVD)
						</div>
						<table className="text-xs border border-secondary-200 dark:border-secondary-700">
							<thead className="bg-secondary-50 dark:bg-secondary-700">
								<tr>
									<th className="px-3 py-1 text-left font-medium">From</th>
									<th className="px-3 py-1 text-left font-medium">
										Fixed / to
									</th>
								</tr>
							</thead>
							<tbody>
								{nvd.ranges.map((r) => (
									<tr
										key={`${r.Lo}-${r.Hi}-${r.Exact}`}
										className="border-t border-secondary-100 dark:border-secondary-700 font-mono"
									>
										<td className="px-3 py-1">{rangeFrom(r)}</td>
										<td className="px-3 py-1">{rangeTo(r)}</td>
									</tr>
								))}
							</tbody>
						</table>
					</div>
				)}
				<div className="flex flex-wrap items-center gap-x-3 gap-y-1">
					<span className="font-medium">Sources:</span>
					{sourceLinks(result.cve_id).map((l) => (
						<a
							key={l.name}
							href={l.url}
							target="_blank"
							rel="noreferrer"
							className="text-primary-600 dark:text-primary-400 hover:underline"
						>
							{l.name}
						</a>
					))}
					{(nvd.references || []).slice(0, 3).map((u, i) => (
						<a
							key={u}
							href={u}
							target="_blank"
							rel="noreferrer"
							className="text-secondary-500 dark:text-secondary-400 hover:underline"
						>
							ref{i + 1}
						</a>
					))}
				</div>
			</div>

			{hosts.length === 0 ? (
				<div className="px-4 py-4 text-sm text-secondary-500 dark:text-secondary-400 flex items-center gap-2">
					<ShieldCheck className="h-4 w-4 text-green-500" />
					No hosts reported vulnerable to this CVE.
				</div>
			) : (
				<div className="overflow-x-auto">
					<table className="min-w-full text-sm">
						<thead className="bg-secondary-50 dark:bg-secondary-700 text-xs uppercase text-secondary-500 dark:text-secondary-300">
							<tr>
								<th className="px-4 py-2 text-left">Host</th>
								<th className="px-4 py-2 text-left">OS</th>
								<th className="px-4 py-2 text-left">Kernel</th>
								<th className="px-4 py-2 text-left">Remediation</th>
							</tr>
						</thead>
						<tbody className="divide-y divide-secondary-100 dark:divide-secondary-700">
							{hosts.map((h) => (
								<tr key={h.id}>
									<td className="px-4 py-2">
										<Link
											to={`/hosts/${h.id}`}
											className="text-primary-600 dark:text-primary-400 hover:underline"
										>
											{h.friendly_name || h.hostname || h.id}
										</Link>
									</td>
									<td className="px-4 py-2 text-secondary-700 dark:text-secondary-200">
										{h.os_type} {h.os_version}
									</td>
									<td className="px-4 py-2 font-mono text-secondary-700 dark:text-secondary-200">
										{h.kernel_version || "N/A"}
									</td>
									<td className="px-4 py-2 text-xs">
										{h.reboot_required ? (
											<span className="text-orange-600 dark:text-orange-400">
												reboot required (fixed kernel installed)
											</span>
										) : h.fixed_version ? (
											<span className="text-red-600 dark:text-red-400 font-mono">
												update → {h.fixed_version}
											</span>
										) : (
											<span className="text-amber-600 dark:text-amber-400">
												no fix yet
											</span>
										)}
									</td>
								</tr>
							))}
						</tbody>
					</table>
				</div>
			)}
		</div>
	);
};

export default CveReport;
