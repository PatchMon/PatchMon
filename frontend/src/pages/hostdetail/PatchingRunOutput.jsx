import { useQuery } from "@tanstack/react-query";
import { RefreshCw } from "lucide-react";
import { patchingAPI } from "../../utils/patchingApi";
import { runMessageNotice } from "../../utils/patchRun";

const PatchingRunOutput = ({ runId }) => {
	const {
		data: run,
		isLoading,
		error,
	} = useQuery({
		queryKey: ["patching-run", runId],
		queryFn: () => patchingAPI.getRunById(runId),
		enabled: !!runId,
		refetchInterval: (query) => {
			const status = query.state.data?.status;
			if (status === "queued" || status === "running") return 3000;
			if (status === "scheduled") return 30000;
			return false;
		},
	});

	if (isLoading) {
		return (
			<div className="flex items-center justify-center h-24 py-4">
				<RefreshCw className="h-5 w-5 animate-spin text-primary-600" />
			</div>
		);
	}

	if (error || !run) {
		return (
			<div className="py-4 text-sm text-red-600 dark:text-red-400">
				Failed to load run output
			</div>
		);
	}

	const shellDisplay =
		run.shell_output != null && run.shell_output !== ""
			? run.shell_output.replace(/\r\n/g, "\n").replace(/\r/g, "\n")
			: "(No output yet)";

	const notice = runMessageNotice(run);

	return (
		<div className="py-4">
			{notice?.tone === "error" && (
				<p className="mb-2 text-sm text-danger-600 dark:text-danger-400 font-mono bg-danger-50 dark:bg-danger-900/20 p-2 rounded whitespace-pre-wrap break-words">
					{notice.detail}
				</p>
			)}
			{notice?.tone === "info" && (
				<div
					role="status"
					className="mb-2 text-sm text-secondary-700 dark:text-secondary-200 bg-secondary-50 dark:bg-secondary-700/40 border border-secondary-200 dark:border-secondary-600 p-2 rounded"
				>
					<p className="font-medium">{notice.title}</p>
					{notice.detail && (
						<p className="text-xs mt-0.5 break-words">{notice.detail}</p>
					)}
				</div>
			)}
			<div className="rounded-lg border border-secondary-700 dark:border-secondary-600 bg-[#0d1117] dark:bg-black overflow-hidden shadow-inner">
				<pre
					className="block w-full min-h-[120px] max-h-[50vh] overflow-auto p-4 text-[13px] leading-relaxed font-mono text-[#e6edf3] whitespace-pre-wrap break-words"
					style={{
						fontFamily:
							"ui-monospace, SFMono-Regular, 'SF Mono', Menlo, Consolas, 'Liberation Mono', monospace",
					}}
				>
					{shellDisplay}
				</pre>
			</div>
		</div>
	);
};

export default PatchingRunOutput;
