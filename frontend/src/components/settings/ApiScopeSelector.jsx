const resources = [
	{ resource: "host", label: "Host", actions: ["get", "delete"] },
	{
		resource: "host_group",
		label: "Host Group",
		actions: ["get", "create", "update", "delete"],
	},
	{
		resource: "enrollment_token",
		label: "Enrollment Token",
		actions: ["get", "create", "update", "delete"],
	},
];

export default function ApiScopeSelector({ scopes, onToggle }) {
	return (
		<div className="border border-secondary-300 dark:border-secondary-600 rounded-md p-4 bg-secondary-50 dark:bg-secondary-900 space-y-4">
			{resources.map(({ resource, label, actions }) => (
				<fieldset key={resource}>
					<legend className="text-xs font-semibold text-secondary-700 dark:text-white mb-2">
						{label} Permissions
					</legend>
					<div className="flex flex-wrap gap-x-4 gap-y-2">
						{actions.map((action) => (
							<label key={action} className="flex items-center gap-2">
								<input
									type="checkbox"
									aria-label={`${label}: ${action}`}
									checked={scopes?.[resource]?.includes(action) || false}
									onChange={() => onToggle(resource, action)}
									className="rounded border-secondary-300 dark:border-secondary-600 text-primary-600 focus:ring-primary-500 dark:focus:ring-primary-400"
								/>
								<span className="text-sm text-secondary-700 dark:text-white capitalize">
									{action}
								</span>
							</label>
						))}
					</div>
				</fieldset>
			))}
		</div>
	);
}
