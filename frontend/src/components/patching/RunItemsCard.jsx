import { RotateCcw } from "lucide-react";
import {
	runItemResultParts,
	runItemSourceLabel,
	runItemStatus,
} from "../../utils/windowsPatching";

const itemKey = (item) => `${item.kind}:${item.identifier || item.id}`;

const itemName = (item) => item.title || item.identifier;

const StatusBadge = ({ status }) => {
	const s = runItemStatus(status);
	return <span className={`${s.className} w-fit`}>{s.label}</span>;
};

const RestartCell = ({ required }) =>
	required ? (
		<span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-md text-xs font-medium whitespace-nowrap bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200">
			<RotateCcw className="h-3 w-3" />
			Required
		</span>
	) : (
		<span className="text-sm text-secondary-500 dark:text-white">No</span>
	);

const ResultText = ({ item }) => {
	const { message, code } = runItemResultParts(item);
	if (!message && !code) {
		return (
			<span className="text-sm text-secondary-400 dark:text-secondary-400">
				-
			</span>
		);
	}
	return (
		<div className="text-sm text-secondary-700 dark:text-secondary-300 [overflow-wrap:anywhere]">
			{message && <span>{message}</span>}
			{code && (
				<span
					className={`font-mono text-xs text-secondary-500 dark:text-secondary-400 ${
						message ? "ml-1" : ""
					}`}
				>
					{code}
				</span>
			)}
		</div>
	);
};

const SourceLine = ({ item }) => (
	<span className="text-xs text-secondary-500 dark:text-secondary-400">
		{runItemSourceLabel(item.kind)}
		{item.kb && <span className="font-mono"> · {item.kb}</span>}
	</span>
);

const RunItemsCard = ({ items }) => {
	if (!Array.isArray(items) || items.length === 0) return null;

	return (
		<div className="card p-4 sm:p-6 flex flex-col min-w-0">
			<h3 className="text-lg font-medium text-secondary-900 dark:text-white mb-4 flex-shrink-0">
				Updates ({items.length})
			</h3>

			<div className="md:hidden space-y-3">
				{items.map((item) => (
					<div
						key={itemKey(item)}
						data-testid="run-item-card"
						className="rounded-lg border border-secondary-200 dark:border-secondary-600 p-4 space-y-3"
					>
						<div className="flex items-start justify-between gap-3">
							<p className="text-base font-semibold text-secondary-900 dark:text-white [overflow-wrap:anywhere] min-w-0">
								{itemName(item)}
							</p>
							<StatusBadge status={item.status} />
						</div>
						<SourceLine item={item} />
						<ResultText item={item} />
						<div className="flex items-center gap-2 pt-2 border-t border-secondary-200 dark:border-secondary-600">
							<span className="text-xs text-secondary-500 dark:text-white">
								Restart:
							</span>
							<RestartCell required={item.reboot_required} />
						</div>
					</div>
				))}
			</div>

			<div className="hidden md:block overflow-x-auto">
				<table className="w-full min-w-[40rem] table-fixed divide-y divide-secondary-200 dark:divide-secondary-600">
					<colgroup>
						<col />
						<col className="w-44" />
						<col className="w-52" />
						<col className="w-24" />
					</colgroup>
					<thead className="bg-secondary-50 dark:bg-secondary-700">
						<tr>
							{["Name", "Status", "Result", "Restart"].map((label) => (
								<th
									key={label}
									className="px-3 sm:px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider whitespace-nowrap"
								>
									{label}
								</th>
							))}
						</tr>
					</thead>
					<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-600">
						{items.map((item) => (
							<tr
								key={itemKey(item)}
								data-testid="run-item-row"
								className="hover:bg-secondary-50 dark:hover:bg-secondary-700 transition-colors align-top"
							>
								<td className="px-3 sm:px-4 py-2">
									<p
										className="text-sm text-secondary-900 dark:text-white truncate"
										title={itemName(item)}
									>
										{itemName(item)}
									</p>
									<SourceLine item={item} />
								</td>
								<td className="px-3 sm:px-4 py-2">
									<StatusBadge status={item.status} />
								</td>
								<td className="px-3 sm:px-4 py-2">
									<ResultText item={item} />
								</td>
								<td className="px-3 sm:px-4 py-2 whitespace-nowrap">
									<RestartCell required={item.reboot_required} />
								</td>
							</tr>
						))}
					</tbody>
				</table>
			</div>
		</div>
	);
};

export default RunItemsCard;
