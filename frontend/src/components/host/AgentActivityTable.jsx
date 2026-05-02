import { useState } from "react";
import { formatDate, formatRelativeTime } from "../../utils/api";
import ActivityTypeBadge from "./ActivityTypeBadge";
import SectionChips from "./SectionChips";

const PLACEHOLDER = "-";

const STATUS_STYLE = {
	success:
		"bg-success-100 text-success-800 dark:bg-success-900 dark:text-success-200",
	completed:
		"bg-success-100 text-success-800 dark:bg-success-900 dark:text-success-200",
	error:
		"bg-danger-100 text-danger-800 dark:bg-danger-900 dark:text-danger-200",
	failed:
		"bg-danger-100 text-danger-800 dark:bg-danger-900 dark:text-danger-200",
	waiting:
		"bg-secondary-100 text-secondary-800 dark:bg-secondary-700 dark:text-secondary-200",
	delayed: "bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200",
	active: "bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200",
};

const titleCase = (value) => {
	if (!value) return "";
	const str = value.toString();
	return str.charAt(0).toUpperCase() + str.slice(1);
};

const StatusBadge = ({ status }) => {
	if (!status) {
		return (
			<span className="text-xs text-secondary-500 dark:text-secondary-300">
				{PLACEHOLDER}
			</span>
		);
	}
	const cls =
		STATUS_STYLE[status.toLowerCase()] ||
		"bg-secondary-100 text-secondary-800 dark:bg-secondary-700 dark:text-secondary-200";
	return (
		<span
			className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium whitespace-nowrap ${cls}`}
		>
			{titleCase(status)}
		</span>
	);
};

const formatKb = (kb) => {
	if (kb == null || Number.isNaN(Number(kb))) return PLACEHOLDER;
	const value = Number(kb);
	if (value >= 1024) return `${(value / 1024).toFixed(1)} MB`;
	if (value >= 100) return `${value.toFixed(0)} KB`;
	if (value > 0) return `${value.toFixed(1)} KB`;
	return `${value.toFixed(0)} KB`;
};

const formatDurationMs = (ms) => {
	if (ms == null || Number.isNaN(Number(ms))) return PLACEHOLDER;
	const value = Number(ms);
	if (value < 1) return `<1 ms`;
	if (value < 1000) return `${Math.round(value)} ms`;
	return `${(value / 1000).toFixed(2)} s`;
};

const buildDurationTooltip = (item) => {
	const parts = [];
	if (item.server_processing_ms != null) {
		parts.push(
			`Server processing: ${formatDurationMs(item.server_processing_ms)}`,
		);
	}
	if (item.agent_execution_ms != null) {
		parts.push(`Agent execution: ${formatDurationMs(item.agent_execution_ms)}`);
	}
	return parts.join("\n");
};

const truncate = (text, max = 80) => {
	if (!text) return "";
	const str = text.toString();
	if (str.length <= max) return str;
	return `${str.slice(0, max)}…`;
};

// Output preview — for jobs we show a stringified snippet of the JSON output.
// Click to expand the full payload underneath the row.
const formatOutputPreview = (output) => {
	if (output == null) return "";
	if (typeof output === "string") return output;
	try {
		return JSON.stringify(output);
	} catch {
		return "";
	}
};

const AttemptPill = ({ attempt }) => {
	if (attempt == null) return null;
	return (
		<span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-secondary-100 text-secondary-700 dark:bg-secondary-700 dark:text-secondary-200 whitespace-nowrap">
			Attempt {attempt}
		</span>
	);
};

const ExpandableText = ({ text, expanded, onToggle, tone = "neutral" }) => {
	if (!text) return null;
	const toneClasses =
		tone === "error"
			? "text-danger-600 dark:text-danger-400"
			: "text-secondary-600 dark:text-secondary-200";
	return (
		<button
			type="button"
			onClick={onToggle}
			className={`text-left text-xs ${toneClasses} hover:underline focus:outline-none focus:ring-2 focus:ring-primary-500 rounded`}
			title={expanded ? "Collapse" : "Click to expand"}
		>
			{expanded ? (
				<span className="break-words whitespace-pre-wrap">{text}</span>
			) : (
				<span className="break-words">{truncate(text, 80)}</span>
			)}
		</button>
	);
};

const DetailsCell = ({ item, expanded, onToggleOutput }) => {
	if (item.kind === "report") {
		return (
			<SectionChips
				sectionsSent={item.sections_sent}
				sectionsUnchanged={item.sections_unchanged}
			/>
		);
	}
	const preview = formatOutputPreview(item.output);
	return (
		<div className="flex flex-wrap items-center gap-2 min-w-0">
			<AttemptPill attempt={item.attempt_number} />
			{preview ? (
				<ExpandableText
					text={preview}
					expanded={expanded}
					onToggle={onToggleOutput}
				/>
			) : (
				<span className="text-xs text-secondary-500 dark:text-secondary-300">
					{PLACEHOLDER}
				</span>
			)}
		</div>
	);
};

const ErrorCell = ({ message, expanded, onToggle }) => {
	if (!message) {
		return (
			<span className="text-xs text-secondary-500 dark:text-secondary-300">
				{PLACEHOLDER}
			</span>
		);
	}
	return (
		<ExpandableText
			text={message}
			expanded={expanded}
			onToggle={onToggle}
			tone="error"
		/>
	);
};

const AgentActivityTable = ({ items = [] }) => {
	const [expanded, setExpanded] = useState({});

	const toggle = (key) => {
		setExpanded((prev) => ({ ...prev, [key]: !prev[key] }));
	};

	if (!items || items.length === 0) {
		return null;
	}

	return (
		<>
			{/* Mobile cards */}
			<div className="md:hidden space-y-3">
				{items.map((item) => {
					const rowKey = `${item.kind}-${item.id}`;
					const outputKey = `${rowKey}-output`;
					const errorKey = `${rowKey}-error`;
					return (
						<div key={rowKey} className="card p-4 space-y-3 min-h-[44px]">
							<div className="flex items-center justify-between gap-2 flex-wrap">
								<ActivityTypeBadge
									kind={item.kind}
									type={item.type}
									jobName={item.job_name}
								/>
								<StatusBadge status={item.status} />
							</div>
							<div className="text-xs text-secondary-600 dark:text-secondary-200">
								<span title={formatDate(item.occurred_at)}>
									{formatRelativeTime(item.occurred_at)}
								</span>
							</div>
							<div className="pt-2 border-t border-secondary-200 dark:border-secondary-600">
								<DetailsCell
									item={item}
									expanded={!!expanded[outputKey]}
									onToggleOutput={() => toggle(outputKey)}
								/>
							</div>
							<div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-secondary-600 dark:text-secondary-200">
								<div>
									<span className="text-secondary-500 dark:text-secondary-400">
										Size:
									</span>{" "}
									{formatKb(item.payload_size_kb)}
								</div>
								<div title={buildDurationTooltip(item)}>
									<span className="text-secondary-500 dark:text-secondary-400">
										Duration:
									</span>{" "}
									{formatDurationMs(item.server_processing_ms)}
								</div>
							</div>
							{item.error_message && (
								<div className="pt-2 border-t border-secondary-200 dark:border-secondary-600">
									<ErrorCell
										message={item.error_message}
										expanded={!!expanded[errorKey]}
										onToggle={() => toggle(errorKey)}
									/>
								</div>
							)}
						</div>
					);
				})}
			</div>

			{/* Desktop table */}
			<div className="hidden md:block h-full overflow-auto">
				<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-600">
					<thead className="bg-secondary-50 dark:bg-secondary-700 sticky top-0 z-10">
						<tr>
							<th className="px-3 sm:px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider whitespace-nowrap">
								When
							</th>
							<th className="px-3 sm:px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider whitespace-nowrap">
								Type
							</th>
							<th className="px-3 sm:px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider">
								Details
							</th>
							<th className="px-3 sm:px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider whitespace-nowrap">
								Size
							</th>
							<th className="px-3 sm:px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider whitespace-nowrap">
								Duration
							</th>
							<th className="px-3 sm:px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider whitespace-nowrap">
								Status
							</th>
							<th className="px-3 sm:px-4 py-2 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider">
								Error
							</th>
						</tr>
					</thead>
					<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-600">
						{items.map((item) => {
							const rowKey = `${item.kind}-${item.id}`;
							const outputKey = `${rowKey}-output`;
							const errorKey = `${rowKey}-error`;
							return (
								<tr
									key={rowKey}
									className="hover:bg-secondary-50 dark:hover:bg-secondary-700 transition-colors align-top"
								>
									<td className="px-3 sm:px-4 py-2 whitespace-nowrap text-sm text-secondary-900 dark:text-white">
										<span title={formatDate(item.occurred_at)}>
											{formatRelativeTime(item.occurred_at)}
										</span>
									</td>
									<td className="px-3 sm:px-4 py-2 whitespace-nowrap text-sm">
										<ActivityTypeBadge
											kind={item.kind}
											type={item.type}
											jobName={item.job_name}
										/>
									</td>
									<td className="px-3 sm:px-4 py-2 text-sm text-secondary-900 dark:text-white">
										<DetailsCell
											item={item}
											expanded={!!expanded[outputKey]}
											onToggleOutput={() => toggle(outputKey)}
										/>
									</td>
									<td className="px-3 sm:px-4 py-2 whitespace-nowrap text-sm text-secondary-900 dark:text-white">
										{formatKb(item.payload_size_kb)}
									</td>
									<td
										className="px-3 sm:px-4 py-2 whitespace-nowrap text-sm text-secondary-900 dark:text-white"
										title={buildDurationTooltip(item)}
									>
										{formatDurationMs(item.server_processing_ms)}
									</td>
									<td className="px-3 sm:px-4 py-2 whitespace-nowrap text-sm">
										<StatusBadge status={item.status} />
									</td>
									<td className="px-3 sm:px-4 py-2 text-sm max-w-xs">
										<ErrorCell
											message={item.error_message}
											expanded={!!expanded[errorKey]}
											onToggle={() => toggle(errorKey)}
										/>
									</td>
								</tr>
							);
						})}
					</tbody>
				</table>
			</div>
		</>
	);
};

export default AgentActivityTable;
