import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
	AlertCircle,
	CheckCircle,
	Edit,
	Plus,
	Power,
	Trash2,
	XCircle,
} from "lucide-react";
import { useEffect, useId, useState } from "react";

// API functions for notification rules
const notificationRulesAPI = {
	list: () =>
		fetch("/api/v1/notifications/rules", {
			headers: {
				Authorization: `Bearer ${localStorage.getItem("token")}`,
			},
		}).then((res) => res.json()),
	create: (data) =>
		fetch("/api/v1/notifications/rules", {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
				Authorization: `Bearer ${localStorage.getItem("token")}`,
			},
			body: JSON.stringify(data),
		}).then((res) => res.json()),
	update: (id, data) =>
		fetch(`/api/v1/notifications/rules/${id}`, {
			method: "PUT",
			headers: {
				"Content-Type": "application/json",
				Authorization: `Bearer ${localStorage.getItem("token")}`,
			},
			body: JSON.stringify(data),
		}).then((res) => res.json()),
	delete: (id) =>
		fetch(`/api/v1/notifications/rules/${id}`, {
			method: "DELETE",
			headers: {
				Authorization: `Bearer ${localStorage.getItem("token")}`,
			},
		}).then((res) => res.json()),
	toggle: (id) =>
		fetch(`/api/v1/notifications/rules/${id}/toggle`, {
			method: "PATCH",
			headers: {
				Authorization: `Bearer ${localStorage.getItem("token")}`,
			},
		}).then((res) => res.json()),
};

// API functions for notification channels (for multi-select)
const notificationChannelsAPI = {
	list: () =>
		fetch("/api/v1/notifications/channels", {
			headers: {
				Authorization: `Bearer ${localStorage.getItem("token")}`,
			},
		}).then((res) => res.json()),
};

const EVENT_TYPES = [
	{ value: "package_update", label: "Package Updates" },
	{ value: "security_update", label: "Security Updates" },
	{ value: "host_status_change", label: "Host Status Changes" },
	{ value: "agent_update", label: "Agent Updates" },
];

const NotificationRules = () => {
	const [showAddModal, setShowAddModal] = useState(false);
	const [editingRule, setEditingRule] = useState(null);
	const queryClient = useQueryClient();

	// Fetch rules
	const {
		data: rules = [],
		isLoading,
		error,
	} = useQuery({
		queryKey: ["notificationRules"],
		queryFn: notificationRulesAPI.list,
	});

	// Fetch channels for multi-select
	const { data: channels = [] } = useQuery({
		queryKey: ["notificationChannels"],
		queryFn: notificationChannelsAPI.list,
	});

	// Delete rule mutation
	const deleteRuleMutation = useMutation({
		mutationFn: notificationRulesAPI.delete,
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ["notificationRules"] });
		},
	});

	// Update rule mutation
	const updateRuleMutation = useMutation({
		mutationFn: ({ id, data }) => notificationRulesAPI.update(id, data),
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ["notificationRules"] });
			setEditingRule(null);
		},
	});

	// Toggle rule mutation
	const toggleRuleMutation = useMutation({
		mutationFn: notificationRulesAPI.toggle,
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ["notificationRules"] });
		},
	});

	const handleDeleteRule = async (ruleId, ruleName) => {
		if (
			globalThis.confirm(
				`Are you sure you want to delete the rule "${ruleName}"?`,
			)
		) {
			try {
				await deleteRuleMutation.mutateAsync(ruleId);
			} catch (error) {
				console.error("Failed to delete rule:", error);
			}
		}
	};

	const handleEditRule = (rule) => {
		setEditingRule(null);
		setTimeout(() => {
			setEditingRule(rule);
		}, 0);
	};

	const handleToggleRule = async (ruleId) => {
		try {
			await toggleRuleMutation.mutateAsync(ruleId);
		} catch (error) {
			console.error("Failed to toggle rule:", error);
		}
	};

	const handleRuleCreated = () => {
		queryClient.invalidateQueries({ queryKey: ["notificationRules"] });
		setShowAddModal(false);
	};

	if (isLoading) {
		return (
			<div className="flex items-center justify-center h-64">
				<div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600"></div>
			</div>
		);
	}

	if (error) {
		return (
			<div className="bg-danger-50 dark:bg-danger-900 border border-danger-200 dark:border-danger-700 rounded-md p-4">
				<div className="flex">
					<XCircle className="h-5 w-5 text-danger-400" />
					<div className="ml-3">
						<h3 className="text-sm font-medium text-danger-800 dark:text-danger-200">
							Error loading rules
						</h3>
						<p className="mt-1 text-sm text-danger-700 dark:text-danger-300">
							{error.message}
						</p>
					</div>
				</div>
			</div>
		);
	}

	return (
		<div className="space-y-6">
			{/* Header with Add Button */}
			<div className="flex items-center justify-between">
				<div>
					<h2 className="text-xl font-bold text-secondary-900 dark:text-white">
						Notification Rules
					</h2>
					<p className="mt-1 text-sm text-secondary-600 dark:text-secondary-400">
						Create rules to trigger notifications for specific events
					</p>
				</div>
				<button
					type="button"
					onClick={() => setShowAddModal(true)}
					className="inline-flex items-center gap-2 px-4 py-2 bg-primary-600 text-white rounded-md hover:bg-primary-700 transition-colors"
				>
					<Plus className="h-4 w-4" />
					Add Rule
				</button>
			</div>

			{/* Rules List */}
			<div className="bg-white dark:bg-secondary-800 shadow overflow-hidden sm:rounded-lg">
				{rules && Array.isArray(rules) && rules.length > 0 ? (
					<>
						{/* Mobile Card Layout */}
						<div className="md:hidden space-y-3 p-4">
							{rules.map((rule) => (
								<RuleCard
									key={rule.id}
									rule={rule}
									channels={channels}
									onEdit={handleEditRule}
									onDelete={handleDeleteRule}
									onToggle={handleToggleRule}
									isToggling={toggleRuleMutation.isPending}
								/>
							))}
						</div>

						{/* Desktop Table Layout */}
						<div className="hidden md:block overflow-x-auto">
							<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-600">
								<thead className="bg-secondary-50 dark:bg-secondary-700">
									<tr>
										<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
											Name
										</th>
										<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
											Event Type
										</th>
										<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
											Channels
										</th>
										<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
											Priority
										</th>
										<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
											Status
										</th>
										<th className="px-6 py-3 text-right text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
											Actions
										</th>
									</tr>
								</thead>
								<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-600">
									{rules.map((rule) => (
										<tr
											key={rule.id}
											className="hover:bg-secondary-50 dark:hover:bg-secondary-700"
										>
											<td className="px-6 py-4 whitespace-nowrap">
												<div className="text-sm font-medium text-secondary-900 dark:text-white">
													{rule.name}
												</div>
											</td>
											<td className="px-6 py-4 whitespace-nowrap">
												<div className="text-sm text-secondary-500 dark:text-secondary-300">
													{EVENT_TYPES.find((e) => e.value === rule.event_type)
														?.label || rule.event_type}
												</div>
											</td>
											<td className="px-6 py-4 whitespace-nowrap">
												<div className="text-sm text-secondary-500 dark:text-secondary-300">
													{rule.channels?.length || 0} channel
													{rule.channels?.length !== 1 ? "s" : ""}
												</div>
											</td>
											<td className="px-6 py-4 whitespace-nowrap">
												<div className="text-sm text-secondary-900 dark:text-white">
													{rule.priority || 5}
												</div>
											</td>
											<td className="px-6 py-4 whitespace-nowrap">
												<RuleStatusBadge enabled={rule.enabled} />
											</td>
											<td className="px-6 py-4 whitespace-nowrap text-right text-sm font-medium">
												<div className="flex items-center justify-end space-x-2">
													<button
														type="button"
														onClick={() => handleToggleRule(rule.id)}
														disabled={toggleRuleMutation.isPending}
														className="text-blue-400 hover:text-blue-600 dark:text-blue-500 dark:hover:text-blue-300 disabled:text-gray-300 disabled:cursor-not-allowed"
														title={
															rule.enabled ? "Disable rule" : "Enable rule"
														}
													>
														<Power className="h-4 w-4" />
													</button>
													<button
														type="button"
														onClick={() => handleEditRule(rule)}
														className="text-secondary-400 hover:text-secondary-600 dark:text-secondary-500 dark:hover:text-secondary-300"
														title="Edit rule"
													>
														<Edit className="h-4 w-4" />
													</button>
													<button
														type="button"
														onClick={() => handleDeleteRule(rule.id, rule.name)}
														className="text-danger-400 hover:text-danger-600 dark:text-danger-500 dark:hover:text-danger-400"
														title="Delete rule"
													>
														<Trash2 className="h-4 w-4" />
													</button>
												</div>
											</td>
										</tr>
									))}
								</tbody>
							</table>
						</div>
					</>
				) : (
					<div className="p-12 text-center">
						<AlertCircle className="h-12 w-12 text-secondary-400 mx-auto mb-4" />
						<p className="text-secondary-500 dark:text-secondary-300">
							No notification rules configured
						</p>
						<p className="text-sm text-secondary-400 dark:text-secondary-400 mt-2">
							Click "Add Rule" to create your first notification rule
						</p>
					</div>
				)}
			</div>

			{/* Add Rule Modal */}
			<AddRuleModal
				isOpen={showAddModal}
				onClose={() => setShowAddModal(false)}
				onRuleCreated={handleRuleCreated}
				channels={channels}
			/>

			{/* Edit Rule Modal */}
			{editingRule && (
				<EditRuleModal
					rule={editingRule}
					isOpen={!!editingRule}
					onClose={() => setEditingRule(null)}
					onUpdateRule={updateRuleMutation.mutate}
					isLoading={updateRuleMutation.isPending}
					channels={channels}
				/>
			)}
		</div>
	);
};

// Rule Status Badge Component
const RuleStatusBadge = ({ enabled }) => {
	if (enabled) {
		return (
			<span className="inline-flex items-center px-2.5 py-0.5 rounded-md text-xs font-medium bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">
				<CheckCircle className="h-3 w-3 mr-1" />
				Enabled
			</span>
		);
	}
	return (
		<span className="inline-flex items-center px-2.5 py-0.5 rounded-md text-xs font-medium bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200">
			<XCircle className="h-3 w-3 mr-1" />
			Disabled
		</span>
	);
};

// Rule Card Component (Mobile)
const RuleCard = ({ rule, onEdit, onDelete, onToggle, isToggling }) => {
	const eventTypeLabel = EVENT_TYPES.find(
		(e) => e.value === rule.event_type,
	)?.label;

	return (
		<div className="card p-4 space-y-3">
			<div className="flex items-start justify-between">
				<div className="flex-1">
					<div className="text-base font-semibold text-secondary-900 dark:text-white">
						{rule.name}
					</div>
					<div className="text-sm text-secondary-500 dark:text-secondary-400 mt-1">
						{eventTypeLabel}
					</div>
				</div>
				<RuleStatusBadge enabled={rule.enabled} />
			</div>

			<div className="flex items-center justify-between pt-2 border-t border-secondary-200 dark:border-secondary-600">
				<div className="text-sm">
					<span className="text-secondary-500 dark:text-secondary-400">
						Channels:&nbsp;
					</span>
					<span className="text-secondary-900 dark:text-white font-medium">
						{rule.channels?.length || 0}
					</span>
				</div>
				<div className="text-sm">
					<span className="text-secondary-500 dark:text-secondary-400">
						Priority:&nbsp;
					</span>
					<span className="text-secondary-900 dark:text-white font-medium">
						{rule.priority || 5}
					</span>
				</div>
			</div>

			<div className="flex items-center justify-end gap-3 pt-2 border-t border-secondary-200 dark:border-secondary-600">
				<button
					type="button"
					onClick={() => onToggle(rule.id)}
					disabled={isToggling}
					className="text-blue-400 hover:text-blue-600 dark:text-blue-500 dark:hover:text-blue-300 disabled:text-gray-300 disabled:cursor-not-allowed inline-flex items-center gap-1 text-sm"
					title={rule.enabled ? "Disable rule" : "Enable rule"}
				>
					<Power className="h-4 w-4" />
					{rule.enabled ? "Disable" : "Enable"}
				</button>
				<button
					type="button"
					onClick={() => onEdit(rule)}
					className="text-secondary-400 hover:text-secondary-600 dark:text-secondary-500 dark:hover:text-secondary-300 inline-flex items-center gap-1 text-sm"
					title="Edit rule"
				>
					<Edit className="h-4 w-4" />
					Edit
				</button>
				<button
					type="button"
					onClick={() => onDelete(rule.id, rule.name)}
					className="text-danger-400 hover:text-danger-600 dark:text-danger-500 dark:hover:text-danger-400 inline-flex items-center gap-1 text-sm"
					title="Delete rule"
				>
					<Trash2 className="h-4 w-4" />
					Delete
				</button>
			</div>
		</div>
	);
};

export default NotificationRules;

// Add Rule Modal Component
const AddRuleModal = ({ isOpen, onClose, onRuleCreated, channels }) => {
	const nameId = useId();
	const eventTypeId = useId();
	const priorityId = useId();
	const messageTitleId = useId();
	const messageTemplateId = useId();

	const [formData, setFormData] = useState({
		name: "",
		event_type: "package_update",
		channel_ids: [],
		priority: 5,
		message_title: "",
		message_template: "",
		filters: [],
	});
	const [isLoading, setIsLoading] = useState(false);
	const [error, setError] = useState("");
	const [success, setSuccess] = useState(false);

	// Reset form when modal is closed
	useEffect(() => {
		if (!isOpen) {
			setFormData({
				name: "",
				event_type: "package_update",
				channel_ids: [],
				priority: 5,
				message_title: "",
				message_template: "",
				filters: [],
			});
			setError("");
			setSuccess(false);
		}
	}, [isOpen]);

	const handleSubmit = async (e) => {
		e.preventDefault();
		setIsLoading(true);
		setError("");
		setSuccess(false);

		try {
			const response = await notificationRulesAPI.create(formData);
			if (response.error) {
				setError(response.error);
			} else {
				setSuccess(true);
				onRuleCreated();
				setTimeout(() => {
					onClose();
				}, 1500);
			}
		} catch (err) {
			setError(err.message || "Failed to create rule");
		} finally {
			setIsLoading(false);
		}
	};

	const handleInputChange = (e) => {
		const { name, value } = e.target;
		setFormData({
			...formData,
			[name]: name === "priority" ? Number.parseInt(value, 10) : value,
		});
	};

	const handleChannelToggle = (channelId) => {
		setFormData({
			...formData,
			channel_ids: formData.channel_ids.includes(channelId)
				? formData.channel_ids.filter((id) => id !== channelId)
				: [...formData.channel_ids, channelId],
		});
	};

	if (!isOpen) return null;

	return (
		<div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 overflow-y-auto">
			<div className="bg-white dark:bg-secondary-800 rounded-lg p-6 w-full max-w-2xl my-8">
				<h3 className="text-lg font-medium text-secondary-900 dark:text-white mb-4">
					Create Notification Rule
				</h3>

				<form
					onSubmit={handleSubmit}
					className="space-y-4 max-h-96 overflow-y-auto"
				>
					<div>
						<label
							htmlFor={nameId}
							className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
						>
							Rule Name
						</label>
						<input
							id={nameId}
							type="text"
							name="name"
							required
							placeholder="e.g., Security Updates Alert"
							value={formData.name}
							onChange={handleInputChange}
							className="block w-full border border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white px-3 py-2"
						/>
					</div>

					<div>
						<label
							htmlFor={eventTypeId}
							className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
						>
							Event Type
						</label>
						<select
							id={eventTypeId}
							name="event_type"
							value={formData.event_type}
							onChange={handleInputChange}
							className="block w-full border border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white px-3 py-2"
						>
							{EVENT_TYPES.map((type) => (
								<option key={type.value} value={type.value}>
									{type.label}
								</option>
							))}
						</select>
					</div>

					<div>
						<div className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-2">
							Notification Channels
						</div>
						{channels && channels.length > 0 ? (
							<div className="space-y-2 border border-secondary-300 dark:border-secondary-600 rounded-md p-3 bg-secondary-50 dark:bg-secondary-700">
								{channels.map((channel) => (
									<label
										key={channel.id}
										className="flex items-center cursor-pointer"
									>
										<input
											type="checkbox"
											checked={formData.channel_ids.includes(channel.id)}
											onChange={() => handleChannelToggle(channel.id)}
											className="rounded border-secondary-300 text-primary-600 shadow-sm focus:border-primary-500 focus:ring-primary-500"
										/>
										<span className="ml-2 text-sm text-secondary-900 dark:text-white">
											{channel.name}
										</span>
									</label>
								))}
							</div>
						) : (
							<p className="text-sm text-secondary-500 dark:text-secondary-400">
								No channels available. Create a channel first.
							</p>
						)}
						{formData.channel_ids.length === 0 && channels.length > 0 && (
							<p className="text-xs text-danger-600 dark:text-danger-400 mt-1">
								At least one channel is required
							</p>
						)}
					</div>

					<div>
						<label
							htmlFor={priorityId}
							className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
						>
							Priority (0-10)
						</label>
						<div className="flex items-center gap-3">
							<input
								id={priorityId}
								type="range"
								name="priority"
								min="0"
								max="10"
								value={formData.priority}
								onChange={handleInputChange}
								className="flex-1"
							/>
							<span className="text-sm font-medium text-secondary-900 dark:text-white w-8 text-center">
								{formData.priority}
							</span>
						</div>
						<p className="mt-1 text-xs text-secondary-500 dark:text-secondary-400">
							Higher values indicate more urgent messages
						</p>
					</div>

					<div>
						<label
							htmlFor={messageTitleId}
							className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
						>
							Message Title (Optional)
						</label>
						<input
							id={messageTitleId}
							type="text"
							name="message_title"
							placeholder="e.g., Security Alert"
							value={formData.message_title}
							onChange={handleInputChange}
							className="block w-full border border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white px-3 py-2"
						/>
						<p className="mt-1 text-xs text-secondary-500 dark:text-secondary-400">
							Leave blank for default title
						</p>
					</div>

					<div>
						<label
							htmlFor={messageTemplateId}
							className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
						>
							Message Template (Optional)
						</label>
						<textarea
							id={messageTemplateId}
							name="message_template"
							rows="3"
							placeholder="Use {{key}} for placeholders. E.g., Package {{package_name}} update available"
							value={formData.message_template}
							onChange={handleInputChange}
							className="block w-full border border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white px-3 py-2"
						/>
						<p className="mt-1 text-xs text-secondary-500 dark:text-secondary-400">
							Leave blank for default message
						</p>
					</div>

					{success && (
						<div className="bg-green-50 dark:bg-green-900 border border-green-200 dark:border-green-700 rounded-md p-3">
							<div className="flex items-center">
								<CheckCircle className="h-4 w-4 text-green-600 dark:text-green-400 mr-2" />
								<p className="text-sm text-green-700 dark:text-green-300">
									Rule created successfully!
								</p>
							</div>
						</div>
					)}

					{error && (
						<div className="bg-danger-50 dark:bg-danger-900 border border-danger-200 dark:border-danger-700 rounded-md p-3">
							<p className="text-sm text-danger-700 dark:text-danger-300">
								{error}
							</p>
						</div>
					)}

					<div className="flex justify-end space-x-3 pt-4 border-t border-secondary-200 dark:border-secondary-600">
						<button
							type="button"
							onClick={onClose}
							className="px-4 py-2 text-sm font-medium text-secondary-700 dark:text-secondary-200 bg-white dark:bg-secondary-700 border border-secondary-300 dark:border-secondary-600 rounded-md hover:bg-secondary-50 dark:hover:bg-secondary-600"
						>
							Cancel
						</button>
						<button
							type="submit"
							disabled={isLoading || formData.channel_ids.length === 0}
							className="px-4 py-2 text-sm font-medium text-white bg-primary-600 border border-transparent rounded-md hover:bg-primary-700 disabled:opacity-50"
						>
							{isLoading ? "Creating..." : "Create Rule"}
						</button>
					</div>
				</form>
			</div>
		</div>
	);
};

// Edit Rule Modal Component
const EditRuleModal = ({
	rule,
	isOpen,
	onClose,
	onUpdateRule,
	isLoading,
	channels,
}) => {
	const nameId = useId();
	const eventTypeId = useId();
	const priorityId = useId();
	const messageTitleId = useId();
	const messageTemplateId = useId();

	const [formData, setFormData] = useState({
		name: rule?.name || "",
		event_type: rule?.event_type || "package_update",
		channel_ids: rule?.channels?.map((c) => c.channel_id) || [],
		priority: rule?.priority || 5,
		message_title: rule?.message_title || "",
		message_template: rule?.message_template || "",
		filters: rule?.filters || [],
	});
	const [error, setError] = useState("");
	const [success, setSuccess] = useState(false);

	// Update formData when rule prop changes or modal opens
	useEffect(() => {
		if (rule && isOpen) {
			setFormData({
				name: rule.name || "",
				event_type: rule.event_type || "package_update",
				channel_ids: rule.channels?.map((c) => c.channel_id) || [],
				priority: rule.priority || 5,
				message_title: rule.message_title || "",
				message_template: rule.message_template || "",
				filters: rule.filters || [],
			});
		}
	}, [rule, isOpen]);

	// Reset error and success when modal closes
	useEffect(() => {
		if (!isOpen) {
			setError("");
			setSuccess(false);
		}
	}, [isOpen]);

	const handleSubmit = async (e) => {
		e.preventDefault();
		setError("");
		setSuccess(false);

		try {
			await onUpdateRule({ id: rule.id, data: formData });
			setSuccess(true);
			setTimeout(() => {
				onClose();
			}, 1500);
		} catch (err) {
			setError(err.message || "Failed to update rule");
		}
	};

	const handleInputChange = (e) => {
		const { name, value } = e.target;
		setFormData({
			...formData,
			[name]: name === "priority" ? Number.parseInt(value, 10) : value,
		});
	};

	const handleChannelToggle = (channelId) => {
		setFormData({
			...formData,
			channel_ids: formData.channel_ids.includes(channelId)
				? formData.channel_ids.filter((id) => id !== channelId)
				: [...formData.channel_ids, channelId],
		});
	};

	if (!isOpen || !rule) return null;

	return (
		<div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50 overflow-y-auto">
			<div className="bg-white dark:bg-secondary-800 rounded-lg p-6 w-full max-w-2xl my-8">
				<h3 className="text-lg font-medium text-secondary-900 dark:text-white mb-4">
					Edit Notification Rule
				</h3>

				<form
					onSubmit={handleSubmit}
					className="space-y-4 max-h-96 overflow-y-auto"
				>
					<div>
						<label
							htmlFor={nameId}
							className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
						>
							Rule Name
						</label>
						<input
							id={nameId}
							type="text"
							name="name"
							required
							value={formData.name}
							onChange={handleInputChange}
							className="block w-full border border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white px-3 py-2"
						/>
					</div>

					<div>
						<label
							htmlFor={eventTypeId}
							className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
						>
							Event Type
						</label>
						<select
							id={eventTypeId}
							name="event_type"
							value={formData.event_type}
							onChange={handleInputChange}
							className="block w-full border border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white px-3 py-2"
						>
							{EVENT_TYPES.map((type) => (
								<option key={type.value} value={type.value}>
									{type.label}
								</option>
							))}
						</select>
					</div>

					<div>
						<div className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-2">
							Notification Channels
						</div>
						{channels && channels.length > 0 ? (
							<div className="space-y-2 border border-secondary-300 dark:border-secondary-600 rounded-md p-3 bg-secondary-50 dark:bg-secondary-700">
								{channels.map((channel) => (
									<label
										key={channel.id}
										className="flex items-center cursor-pointer"
									>
										<input
											type="checkbox"
											checked={formData.channel_ids.includes(channel.id)}
											onChange={() => handleChannelToggle(channel.id)}
											className="rounded border-secondary-300 text-primary-600 shadow-sm focus:border-primary-500 focus:ring-primary-500"
										/>
										<span className="ml-2 text-sm text-secondary-900 dark:text-white">
											{channel.name}
										</span>
									</label>
								))}
							</div>
						) : (
							<p className="text-sm text-secondary-500 dark:text-secondary-400">
								No channels available.
							</p>
						)}
						{formData.channel_ids.length === 0 && channels.length > 0 && (
							<p className="text-xs text-danger-600 dark:text-danger-400 mt-1">
								At least one channel is required
							</p>
						)}
					</div>

					<div>
						<label
							htmlFor={priorityId}
							className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
						>
							Priority (0-10)
						</label>
						<div className="flex items-center gap-3">
							<input
								id={priorityId}
								type="range"
								name="priority"
								min="0"
								max="10"
								value={formData.priority}
								onChange={handleInputChange}
								className="flex-1"
							/>
							<span className="text-sm font-medium text-secondary-900 dark:text-white w-8 text-center">
								{formData.priority}
							</span>
						</div>
					</div>

					<div>
						<label
							htmlFor={messageTitleId}
							className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
						>
							Message Title (Optional)
						</label>
						<input
							id={messageTitleId}
							type="text"
							name="message_title"
							value={formData.message_title}
							onChange={handleInputChange}
							className="block w-full border border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white px-3 py-2"
						/>
					</div>

					<div>
						<label
							htmlFor={messageTemplateId}
							className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
						>
							Message Template (Optional)
						</label>
						<textarea
							id={messageTemplateId}
							name="message_template"
							rows="3"
							value={formData.message_template}
							onChange={handleInputChange}
							className="block w-full border border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white px-3 py-2"
						/>
					</div>

					{success && (
						<div className="bg-green-50 dark:bg-green-900 border border-green-200 dark:border-green-700 rounded-md p-3">
							<div className="flex items-center">
								<CheckCircle className="h-4 w-4 text-green-600 dark:text-green-400 mr-2" />
								<p className="text-sm text-green-700 dark:text-green-300">
									Rule updated successfully!
								</p>
							</div>
						</div>
					)}

					{error && (
						<div className="bg-danger-50 dark:bg-danger-900 border border-danger-200 dark:border-danger-700 rounded-md p-3">
							<p className="text-sm text-danger-700 dark:text-danger-300">
								{error}
							</p>
						</div>
					)}

					<div className="flex justify-end space-x-3 pt-4 border-t border-secondary-200 dark:border-secondary-600">
						<button
							type="button"
							onClick={onClose}
							className="px-4 py-2 text-sm font-medium text-secondary-700 dark:text-secondary-200 bg-white dark:bg-secondary-700 border border-secondary-300 dark:border-secondary-600 rounded-md hover:bg-secondary-50 dark:hover:bg-secondary-600"
						>
							Cancel
						</button>
						<button
							type="submit"
							disabled={isLoading || formData.channel_ids.length === 0}
							className="px-4 py-2 text-sm font-medium text-white bg-primary-600 border border-transparent rounded-md hover:bg-primary-700 disabled:opacity-50"
						>
							{isLoading ? "Updating..." : "Update Rule"}
						</button>
					</div>
				</form>
			</div>
		</div>
	);
};
