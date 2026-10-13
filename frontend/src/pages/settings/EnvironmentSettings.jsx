import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, Edit2, Lock, RefreshCw, Variable } from "lucide-react";
import { Fragment, useState } from "react";
import { TimezoneSelect } from "../../components/TimezoneSelect";
import InfoTooltip from "../../components/ui/InfoTooltip";
import { useToast } from "../../contexts/ToastContext";
import { settingsAPI } from "../../utils/api";

const sourceLabels = {
	env: ".env",
	db: "Database",
	default: "Default",
};

const LOG_LEVEL_OPTIONS = ["debug", "info", "warn", "error"];

const BODY_LIMIT_OPTIONS = ["1mb", "2mb", "5mb", "10mb", "20mb", "32mb"];

const PING_BODY_LIMIT_OPTIONS = ["8kb", "16kb", "32kb", "64kb", "128kb"];

const JWT_EXPIRES_IN_OPTIONS = ["15m", "30m", "1h", "2h", "7d"];

const MAX_TFA_ATTEMPTS_OPTIONS = ["3", "5", "10"];

const TFA_LOCKOUT_DURATION_OPTIONS = ["15", "30", "60"];

const TFA_REMEMBER_ME_OPTIONS = ["7d", "30d", "90d"];

const isBooleanVar = (v) =>
	v.defaultValue === "true" || v.defaultValue === "false";

const isBodyLimitVar = (v) =>
	v.key === "JSON_BODY_LIMIT" ||
	v.key === "AGENT_UPDATE_BODY_LIMIT" ||
	v.key === "COMPLIANCE_BODY_LIMIT" ||
	v.key === "AGENT_PING_BODY_LIMIT";

const getBodyLimitOptions = (v) =>
	v.key === "AGENT_PING_BODY_LIMIT"
		? PING_BODY_LIMIT_OPTIONS
		: BODY_LIMIT_OPTIONS;

const getSelectOptionsForVar = (v) => {
	switch (v.key) {
		case "JWT_EXPIRES_IN":
			return JWT_EXPIRES_IN_OPTIONS;
		case "MAX_TFA_ATTEMPTS":
			return MAX_TFA_ATTEMPTS_OPTIONS;
		case "TFA_LOCKOUT_DURATION_MINUTES":
			return TFA_LOCKOUT_DURATION_OPTIONS;
		case "TFA_REMEMBER_ME_EXPIRES_IN":
			return TFA_REMEMBER_ME_OPTIONS;
		default:
			return null;
	}
};

const SourceBadge = ({ source }) => {
	const colors = {
		env: "bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300",
		db: "bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300",
		default:
			"bg-secondary-100 text-secondary-700 dark:bg-secondary-700 dark:text-secondary-300",
	};
	return (
		<span
			className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${colors[source] || colors.default}`}
		>
			{sourceLabels[source] ?? source}
		</span>
	);
};

const VariableKey = ({ name }) => (
	<span className="text-sm font-mono font-medium text-secondary-900 dark:text-white break-words">
		{name.split("_").map((part, i, parts) => (
			<Fragment key={parts.slice(0, i + 1).join("_")}>
				{part}
				{i < parts.length - 1 && (
					<>
						_<wbr />
					</>
				)}
			</Fragment>
		))}
	</span>
);

const EnvironmentSettings = () => {
	const queryClient = useQueryClient();
	const toast = useToast();
	const [editingKey, setEditingKey] = useState(null);
	const [editValue, setEditValue] = useState("");

	const { data, isLoading, error } = useQuery({
		queryKey: ["environment-config"],
		queryFn: () => settingsAPI.getEnvironmentConfig().then((r) => r.data),
	});

	const updateMutation = useMutation({
		mutationFn: ({ key, value }) =>
			settingsAPI.updateEnvironmentConfig(key, value),
		onSuccess: async (response) => {
			await queryClient.invalidateQueries({ queryKey: ["environment-config"] });
			setEditingKey(null);
			setEditValue("");
			toast.success(response?.data?.message || "Saved.");
		},
		onError: (err) => {
			toast.error(err.response?.data?.error || "Failed to update");
		},
	});

	const variables = data?.variables || [];
	const hasConflict = variables.some((v) => v.conflict);

	// Group variables by category (preserve backend order, matches .env.example)
	const categoryOrder = [
		"Database",
		"Server",
		"Logging",
		"Authentication",
		"Password policy",
		"Server performance",
		"Rate limits",
		"Redis",
		"Encryption",
		"Deployment",
	];
	const byCategory = variables.reduce((acc, v) => {
		const cat = v.category || "Other";
		if (!acc[cat]) acc[cat] = [];
		acc[cat].push(v);
		return acc;
	}, {});
	const orderedCategories = categoryOrder.filter((c) => byCategory[c]?.length);
	for (const cat of Object.keys(byCategory)) {
		if (!categoryOrder.includes(cat)) orderedCategories.push(cat);
	}

	const handleEdit = (v) => {
		setEditingKey(v.key);
		setEditValue(v.effectiveValue);
	};

	const handleSave = () => {
		if (!editingKey) return;
		updateMutation.mutate({ key: editingKey, value: editValue });
	};

	const handleCancel = () => {
		setEditingKey(null);
		setEditValue("");
	};

	const renderEditorControl = (v) => (
		<>
			{v.key === "LOG_LEVEL" ? (
				<select
					value={editValue}
					onChange={(e) => setEditValue(e.target.value)}
					className="block w-28 rounded border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-800 px-2 py-1 text-sm"
				>
					{LOG_LEVEL_OPTIONS.map((opt) => (
						<option key={opt} value={opt}>
							{opt}
						</option>
					))}
				</select>
			) : isBodyLimitVar(v) ? (
				<select
					value={editValue}
					onChange={(e) => setEditValue(e.target.value)}
					className="block w-24 rounded border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-800 px-2 py-1 text-sm"
				>
					{getBodyLimitOptions(v).map((opt) => (
						<option key={opt} value={opt}>
							{opt}
						</option>
					))}
					{editValue &&
						!getBodyLimitOptions(v).includes(editValue.toLowerCase()) && (
							<option value={editValue}>{editValue}</option>
						)}
				</select>
			) : getSelectOptionsForVar(v) ? (
				<select
					value={editValue}
					onChange={(e) => setEditValue(e.target.value)}
					className="block w-24 rounded border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-800 px-2 py-1 text-sm"
				>
					{getSelectOptionsForVar(v).map((opt) => (
						<option key={opt} value={opt}>
							{opt}
						</option>
					))}
					{editValue && !getSelectOptionsForVar(v).includes(editValue) && (
						<option value={editValue}>{editValue}</option>
					)}
				</select>
			) : v.key === "TIMEZONE" ? (
				<TimezoneSelect
					value={editValue}
					onChange={(e) => setEditValue(e.target.value)}
					className="block w-full max-w-[12rem] rounded border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-800 px-2 py-1 text-sm"
				/>
			) : isBooleanVar(v) ? (
				<select
					value={editValue}
					onChange={(e) => setEditValue(e.target.value)}
					className="block w-20 rounded border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-800 px-2 py-1 text-sm"
				>
					<option value="true">true</option>
					<option value="false">false</option>
				</select>
			) : (
				<input
					type="text"
					value={editValue}
					onChange={(e) => setEditValue(e.target.value)}
					className="block min-w-[12rem] max-w-[280px] rounded border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-800 px-2 py-1 text-sm"
				/>
			)}
		</>
	);

	const renderEditor = (v) => (
		<div className="flex flex-wrap items-center gap-2">
			{renderEditorControl(v)}
			<button
				type="button"
				onClick={handleSave}
				disabled={updateMutation.isPending}
				className="text-sm font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 min-h-[44px] md:min-h-0"
			>
				Save
			</button>
			<button
				type="button"
				onClick={handleCancel}
				className="text-sm text-secondary-500 hover:text-secondary-700 dark:text-secondary-400 min-h-[44px] md:min-h-0"
			>
				Cancel
			</button>
		</div>
	);

	const renderValue = (v) => {
		const showDefault =
			(v.defaultValue || "") !== (v.effectiveValue || "") && v.defaultValue;
		if (editingKey === v.key) {
			return (
				<div className="min-w-0">
					{renderEditor(v)}
					{v.defaultValue && (
						<div className="mt-0.5 text-xs text-secondary-500 dark:text-secondary-400 [overflow-wrap:anywhere]">
							Default: {v.defaultValue}
						</div>
					)}
				</div>
			);
		}
		return (
			<div className="min-w-0">
				{v.effectiveValue?.includes(",") ? (
					<div className="flex flex-col gap-0.5 text-sm text-secondary-700 dark:text-secondary-300 [overflow-wrap:anywhere]">
						{v.effectiveValue.split(",").map((part) => {
							const trimmed = part.trim() || "\u00a0";
							return <span key={trimmed}>{trimmed}</span>;
						})}
					</div>
				) : (
					<span className="text-sm text-secondary-700 dark:text-secondary-300 [overflow-wrap:anywhere]">
						{v.effectiveValue || " -"}
					</span>
				)}
				{showDefault && (
					<div className="mt-0.5 text-xs text-secondary-500 dark:text-secondary-400 [overflow-wrap:anywhere]">
						Default: {v.defaultValue}
					</div>
				)}
			</div>
		);
	};

	const renderAction = (v) => {
		if (v.editable && v.effectiveSource === "env") {
			return (
				<div className="flex flex-col items-start gap-0.5">
					<span
						className="inline-flex items-center gap-1 text-xs font-medium text-amber-700 dark:text-amber-300 whitespace-nowrap"
						title={`${v.key} is set in .env, which takes priority over the database. Change it in .env and restart PatchMon.`}
					>
						<Lock className="h-3.5 w-3.5" />
						Set in .env
					</span>
					{v.conflict && (
						<span className="text-xs text-secondary-500 dark:text-secondary-400 [overflow-wrap:anywhere]">
							{v.dbValue
								? `Database value "${v.dbValue}" is ignored`
								: "Database value is ignored"}
						</span>
					)}
				</div>
			);
		}
		if (v.editable) {
			return (
				<button
					type="button"
					onClick={() => handleEdit(v)}
					disabled={editingKey !== null && editingKey !== v.key}
					className="inline-flex items-center gap-1 text-xs text-primary-600 hover:text-primary-700 dark:text-primary-400 disabled:opacity-50 min-h-[44px] md:min-h-0"
				>
					<Edit2 className="h-4 w-4" />
					Edit
				</button>
			);
		}
		return (
			<span
				className="text-xs text-secondary-400 whitespace-nowrap"
				title="Startup/deployment only - configure via .env"
			>
				Configure via .env
			</span>
		);
	};

	if (isLoading) {
		return (
			<div className="flex items-center justify-center h-64">
				<RefreshCw className="h-8 w-8 animate-spin text-primary-600" />
			</div>
		);
	}

	if (error) {
		return (
			<div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-md p-4">
				<div className="flex">
					<AlertTriangle className="h-5 w-5 text-red-400 flex-shrink-0 mt-0.5" />
					<div className="ml-3">
						<h3 className="text-sm font-medium text-red-800 dark:text-red-200">
							Error loading environment config
						</h3>
						<p className="mt-1 text-sm text-red-700 dark:text-red-300">
							{error.message || "Failed to load settings"}
						</p>
					</div>
				</div>
			</div>
		);
	}

	return (
		<div className="space-y-6">
			<div className="flex items-center mb-6">
				<Variable className="h-6 w-6 flex-shrink-0 text-primary-600 mr-3" />
				<div>
					<h2 className="text-lg font-semibold text-secondary-900 dark:text-white">
						Environment Variables
					</h2>
					<p className="text-sm text-secondary-500 dark:text-secondary-400 mt-1">
						<strong>Priority 1</strong> - .env file •{" "}
						<strong>Priority 2</strong> - Database settings (configurable below){" "}
						• <strong>Priority 3</strong> - Coded defaults
					</p>
					<p className="text-sm text-secondary-500 dark:text-secondary-400 mt-1">
						Variables marked &quot;Configure via .env&quot; are
						startup/deployment related and can only be changed in the .env file
						(not in the database).
					</p>
					<p className="text-sm text-secondary-500 dark:text-secondary-400 mt-1">
						Variables marked &quot;Set in .env&quot; already have a value in the
						.env file, which takes priority, so they cannot be changed here.
						Edit or remove them in .env and restart the PatchMon server.
					</p>
					<p className="text-sm text-secondary-500 dark:text-secondary-400 mt-1">
						Most changes apply straight away. After saving you will be told if a
						restart is needed.
					</p>
				</div>
			</div>

			{hasConflict && (
				<div className="bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 rounded-md p-4">
					<div className="flex">
						<AlertTriangle className="h-5 w-5 text-amber-500 flex-shrink-0 mt-0.5" />
						<div className="ml-3">
							<h3 className="text-sm font-medium text-amber-800 dark:text-amber-200">
								Misconfiguration detected
							</h3>
							<p className="mt-1 text-sm text-amber-700 dark:text-amber-300">
								Some variables have both .env and database values. The .env
								value takes precedence and the database value is ignored. Remove
								the variable from .env and restart for the database value to
								take effect.
							</p>
						</div>
					</div>
				</div>
			)}

			<div className="md:hidden space-y-4">
				{orderedCategories.map((category) => (
					<section key={category} className="space-y-2">
						<h3 className="text-sm font-semibold text-secondary-700 dark:text-secondary-200">
							{category}
						</h3>
						{byCategory[category].map((v) => (
							<div
								key={v.key}
								data-env-key={v.key}
								className={`card p-4 space-y-3 ${
									v.conflict ? "ring-1 ring-amber-300 dark:ring-amber-700" : ""
								}`}
							>
								<div className="flex items-start justify-between gap-3">
									<div className="flex items-center gap-1.5 min-w-0">
										<VariableKey name={v.key} />
										<InfoTooltip
											content={v.description}
											label={`About ${v.key}`}
										/>
									</div>
									<SourceBadge source={v.effectiveSource} />
								</div>
								{renderValue(v)}
								<div className="pt-2 border-t border-secondary-200 dark:border-secondary-600">
									{renderAction(v)}
								</div>
							</div>
						))}
					</section>
				))}
			</div>

			<div className="hidden md:block card overflow-x-auto">
				<table className="w-full divide-y divide-secondary-200 dark:divide-secondary-600">
					<thead className="bg-secondary-50 dark:bg-secondary-700">
						<tr>
							<th className="w-1/3 px-3 py-2 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider">
								Variable
							</th>
							<th className="px-3 py-2 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider">
								Value
							</th>
							<th className="w-px px-3 py-2 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider">
								Source
							</th>
							<th className="w-px px-3 py-2 text-left text-xs font-medium text-secondary-500 dark:text-white uppercase tracking-wider">
								Action
							</th>
						</tr>
					</thead>
					<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-600">
						{orderedCategories.map((category) => (
							<Fragment key={category}>
								<tr className="bg-secondary-100 dark:bg-secondary-700/80">
									<td
										colSpan={4}
										className="px-3 py-1.5 text-sm font-semibold text-secondary-700 dark:text-secondary-200"
									>
										{category}
									</td>
								</tr>
								{byCategory[category].map((v) => (
									<tr
										key={v.key}
										className={`align-top hover:bg-secondary-50 dark:hover:bg-secondary-700 transition-colors ${
											v.conflict ? "bg-amber-50/50 dark:bg-amber-900/10" : ""
										}`}
									>
										<td className="px-3 py-2">
											<div className="flex items-center gap-1.5">
												<VariableKey name={v.key} />
												<InfoTooltip
													content={v.description}
													label={`About ${v.key}`}
												/>
											</div>
										</td>
										<td className="px-3 py-2">{renderValue(v)}</td>
										<td className="px-3 py-2 whitespace-nowrap">
											<SourceBadge source={v.effectiveSource} />
										</td>
										<td className="px-3 py-2">{renderAction(v)}</td>
									</tr>
								))}
							</Fragment>
						))}
					</tbody>
				</table>
			</div>
		</div>
	);
};

export default EnvironmentSettings;
