import { useMutation } from "@tanstack/react-query";
import { AlertTriangle, Search, ShieldCheck } from "lucide-react";
import { useId, useState } from "react";
import { Link } from "react-router-dom";
import { dashboardAPI } from "../utils/api";

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

			<div className="space-y-6">
				{results.map((r) => (
					<CveResultCard key={r.cve_id} result={r} />
				))}
			</div>
		</div>
	);
};

const CveResultCard = ({ result }) => {
	const c = result.counts || {};
	const hosts = result.hosts || [];
	return (
		<div className="border border-secondary-200 dark:border-secondary-700 rounded-lg overflow-hidden">
			<div className="px-4 py-3 bg-secondary-50 dark:bg-secondary-800 flex flex-wrap items-center justify-between gap-2">
				<div className="font-mono font-semibold text-secondary-900 dark:text-white">
					{result.cve_id}
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
								<th className="px-4 py-2 text-left">Fix</th>
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
									<td className="px-4 py-2 font-mono text-secondary-700 dark:text-secondary-200">
										{h.fixed_version ? (
											<span className="text-green-700 dark:text-green-400">
												{h.fixed_version}
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
