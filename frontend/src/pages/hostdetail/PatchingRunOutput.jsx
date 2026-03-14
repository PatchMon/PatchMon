import { useQuery } from "@tanstack/react-query";
import { RefreshCw } from "lucide-react";
import { patchingAPI } from "../../utils/patchingApi";

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

	return (
		<div className="py-4">
			{run.error_message && (
				<p className="mb-2 text-sm text-red-600 dark:text-red-400 font-mono bg-red-50 dark:bg-red-900/20 p-2 rounded">
					{run.error_message}
				</p>
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
