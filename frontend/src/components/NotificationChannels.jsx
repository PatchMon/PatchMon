import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
	AlertCircle,
	CheckCircle,
	Edit,
	Loader,
	Plus,
	Trash2,
	XCircle,
} from "lucide-react";
import { useEffect, useId, useState } from "react";

// API functions for notification channels
const notificationChannelsAPI = {
	list: () =>
		fetch("/api/v1/notifications/channels", {
			headers: {
				Authorization: `Bearer ${localStorage.getItem("token")}`,
			},
		}).then((res) => res.json()),
	create: (data) =>
		fetch("/api/v1/notifications/channels", {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
				Authorization: `Bearer ${localStorage.getItem("token")}`,
			},
			body: JSON.stringify(data),
		}).then((res) => res.json()),
	update: (id, data) =>
		fetch(`/api/v1/notifications/channels/${id}`, {
			method: "PUT",
			headers: {
				"Content-Type": "application/json",
				Authorization: `Bearer ${localStorage.getItem("token")}`,
			},
			body: JSON.stringify(data),
		}).then((res) => res.json()),
	delete: (id) =>
		fetch(`/api/v1/notifications/channels/${id}`, {
			method: "DELETE",
			headers: {
				Authorization: `Bearer ${localStorage.getItem("token")}`,
			},
		}).then((res) => res.json()),
	test: (id) =>
		fetch(`/api/v1/notifications/channels/${id}/test`, {
			method: "POST",
			headers: {
				Authorization: `Bearer ${localStorage.getItem("token")}`,
			},
		}).then((res) => res.json()),
};

const NotificationChannels = () => {
	const [showAddModal, setShowAddModal] = useState(false);
	const [editingChannel, setEditingChannel] = useState(null);
	const [testingChannelId, setTestingChannelId] = useState(null);
	const queryClient = useQueryClient();

	// Fetch channels
	const {
		data: channels = [],
		isLoading,
		error,
	} = useQuery({
		queryKey: ["notificationChannels"],
		queryFn: notificationChannelsAPI.list,
	});

	// Delete channel mutation
	const deleteChannelMutation = useMutation({
		mutationFn: notificationChannelsAPI.delete,
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ["notificationChannels"] });
		},
	});

	// Update channel mutation
	const updateChannelMutation = useMutation({
		mutationFn: ({ id, data }) => notificationChannelsAPI.update(id, data),
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ["notificationChannels"] });
			setEditingChannel(null);
		},
	});

	// Test channel mutation
	const testChannelMutation = useMutation({
		mutationFn: notificationChannelsAPI.test,
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ["notificationChannels"] });
			setTestingChannelId(null);
		},
	});

	const handleDeleteChannel = async (channelId, channelName) => {
		if (
			globalThis.confirm(
				`Are you sure you want to delete the channel "${channelName}"? This will also delete any associated notification rules.`,
			)
		) {
			try {
				await deleteChannelMutation.mutateAsync(channelId);
			} catch (error) {
				console.error("Failed to delete channel:", error);
			}
		}
	};

	const handleEditChannel = (channel) => {
		setEditingChannel(null);
		setTimeout(() => {
			setEditingChannel(channel);
		}, 0);
	};

	const handleTestChannel = async (channelId) => {
		setTestingChannelId(channelId);
		try {
			await testChannelMutation.mutateAsync(channelId);
		} catch (error) {
			console.error("Failed to test channel:", error);
		}
	};

	const handleChannelCreated = () => {
		queryClient.invalidateQueries({ queryKey: ["notificationChannels"] });
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
							Error loading channels
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
						Notification Channels
					</h2>
					<p className="mt-1 text-sm text-secondary-600 dark:text-secondary-400">
						Configure Gotify servers to receive notifications
					</p>
				</div>
				<button
					type="button"
					onClick={() => setShowAddModal(true)}
					className="inline-flex items-center gap-2 px-4 py-2 bg-primary-600 text-white rounded-md hover:bg-primary-700 transition-colors"
				>
					<Plus className="h-4 w-4" />
					Add Channel
				</button>
			</div>

			{/* Channels List */}
			<div className="bg-white dark:bg-secondary-800 shadow overflow-hidden sm:rounded-lg">
				{channels && Array.isArray(channels) && channels.length > 0 ? (
					<>
						{/* Mobile Card Layout */}
						<div className="md:hidden space-y-3 p-4">
							{channels.map((channel) => (
								<ChannelCard
									key={channel.id}
									channel={channel}
									onEdit={handleEditChannel}
									onDelete={handleDeleteChannel}
									onTest={handleTestChannel}
									isTestingChannelId={testingChannelId}
									isTestLoading={testChannelMutation.isPending}
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
											Server URL
										</th>
										<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
											Status
										</th>
										<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
											Priority
										</th>
										<th className="px-6 py-3 text-right text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
											Actions
										</th>
									</tr>
								</thead>
								<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-600">
									{channels.map((channel) => (
										<tr
											key={channel.id}
											className="hover:bg-secondary-50 dark:hover:bg-secondary-700"
										>
											<td className="px-6 py-4 whitespace-nowrap">
												<div className="text-sm font-medium text-secondary-900 dark:text-white">
													{channel.name}
												</div>
											</td>
											<td className="px-6 py-4 whitespace-nowrap">
												<div className="text-sm text-secondary-500 dark:text-secondary-300 truncate max-w-xs">
													{channel.server_url}
												</div>
											</td>
											<td className="px-6 py-4 whitespace-nowrap">
												<StatusBadge status={channel.status} />
											</td>
											<td className="px-6 py-4 whitespace-nowrap">
												<div className="text-sm text-secondary-900 dark:text-white">
													{channel.priority || 5}
												</div>
											</td>
											<td className="px-6 py-4 whitespace-nowrap text-right text-sm font-medium">
												<div className="flex items-center justify-end space-x-2">
													<button
														type="button"
														onClick={() => handleTestChannel(channel.id)}
														disabled={testingChannelId === channel.id}
														className="text-blue-400 hover:text-blue-600 dark:text-blue-500 dark:hover:text-blue-300 disabled:text-gray-300 disabled:cursor-not-allowed"
														title="Test connection"
													>
														{testingChannelId === channel.id ? (
															<Loader className="h-4 w-4 animate-spin" />
														) : (
															<AlertCircle className="h-4 w-4" />
														)}
													</button>
													<button
														type="button"
														onClick={() => handleEditChannel(channel)}
														className="text-secondary-400 hover:text-secondary-600 dark:text-secondary-500 dark:hover:text-secondary-300"
														title="Edit channel"
													>
														<Edit className="h-4 w-4" />
													</button>
													<button
														type="button"
														onClick={() =>
															handleDeleteChannel(channel.id, channel.name)
														}
														className="text-danger-400 hover:text-danger-600 dark:text-danger-500 dark:hover:text-danger-400"
														title="Delete channel"
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
							No notification channels configured
						</p>
						<p className="text-sm text-secondary-400 dark:text-secondary-400 mt-2">
							Click "Add Channel" to create your first Gotify notification
							channel
						</p>
					</div>
				)}
			</div>

			{/* Add Channel Modal */}
			<AddChannelModal
				isOpen={showAddModal}
				onClose={() => setShowAddModal(false)}
				onChannelCreated={handleChannelCreated}
			/>

			{/* Edit Channel Modal */}
			{editingChannel && (
				<EditChannelModal
					channel={editingChannel}
					isOpen={!!editingChannel}
					onClose={() => setEditingChannel(null)}
					onUpdateChannel={updateChannelMutation.mutate}
					isLoading={updateChannelMutation.isPending}
				/>
			)}
		</div>
	);
};

// Status Badge Component
const StatusBadge = ({ status }) => {
	if (status === "connected") {
		return (
			<span className="inline-flex items-center px-2.5 py-0.5 rounded-md text-xs font-medium bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">
				<CheckCircle className="h-3 w-3 mr-1" />
				Connected
			</span>
		);
	}
	return (
		<span className="inline-flex items-center px-2.5 py-0.5 rounded-md text-xs font-medium bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200">
			<XCircle className="h-3 w-3 mr-1" />
			Disconnected
		</span>
	);
};

// Channel Card Component (Mobile)
const ChannelCard = ({
	channel,
	onEdit,
	onDelete,
	onTest,
	isTestingChannelId,
}) => {
	return (
		<div className="card p-4 space-y-3">
			<div className="flex items-start justify-between">
				<div className="flex-1">
					<div className="text-base font-semibold text-secondary-900 dark:text-white">
						{channel.name}
					</div>
					<div className="text-sm text-secondary-500 dark:text-secondary-400 mt-1 truncate">
						{channel.server_url}
					</div>
				</div>
				<StatusBadge status={channel.status} />
			</div>

			<div className="flex items-center justify-between pt-2 border-t border-secondary-200 dark:border-secondary-600">
				<div className="text-sm">
					<span className="text-secondary-500 dark:text-secondary-400">
						Priority:&nbsp;
					</span>
					<span className="text-secondary-900 dark:text-white font-medium">
						{channel.priority || 5}
					</span>
				</div>
			</div>

			<div className="flex items-center justify-end gap-3 pt-2 border-t border-secondary-200 dark:border-secondary-600">
				<button
					type="button"
					onClick={() => onTest(channel.id)}
					disabled={isTestingChannelId === channel.id}
					className="text-blue-400 hover:text-blue-600 dark:text-blue-500 dark:hover:text-blue-300 disabled:text-gray-300 disabled:cursor-not-allowed inline-flex items-center gap-1 text-sm"
					title="Test connection"
				>
					{isTestingChannelId === channel.id ? (
						<Loader className="h-4 w-4 animate-spin" />
					) : (
						<AlertCircle className="h-4 w-4" />
					)}
					Test
				</button>
				<button
					type="button"
					onClick={() => onEdit(channel)}
					className="text-secondary-400 hover:text-secondary-600 dark:text-secondary-500 dark:hover:text-secondary-300 inline-flex items-center gap-1 text-sm"
					title="Edit channel"
				>
					<Edit className="h-4 w-4" />
					Edit
				</button>
				<button
					type="button"
					onClick={() => onDelete(channel.id, channel.name)}
					className="text-danger-400 hover:text-danger-600 dark:text-danger-500 dark:hover:text-danger-400 inline-flex items-center gap-1 text-sm"
					title="Delete channel"
				>
					<Trash2 className="h-4 w-4" />
					Delete
				</button>
			</div>
		</div>
	);
};

export default NotificationChannels;

// Add Channel Modal Component
const AddChannelModal = ({ isOpen, onClose, onChannelCreated }) => {
	// eslint-disable-next-line react/prop-types
	const nameId = useId();
	const urlId = useId();
	const tokenId = useId();
	const priorityId = useId();

	const [formData, setFormData] = useState({
		name: "",
		server_url: "",
		token: "",
		priority: 5,
	});
	const [isLoading, setIsLoading] = useState(false);
	const [error, setError] = useState("");
	const [success, setSuccess] = useState(false);

	// Reset form when modal is closed
	useEffect(() => {
		if (!isOpen) {
			setFormData({
				name: "",
				server_url: "",
				token: "",
				priority: 5,
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
			const response = await notificationChannelsAPI.create(formData);
			if (response.error) {
				setError(response.error);
			} else {
				setSuccess(true);
				onChannelCreated();
				setTimeout(() => {
					onClose();
				}, 1500);
			}
		} catch (err) {
			setError(err.message || "Failed to create channel");
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

	if (!isOpen) return null;

	return (
		<div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
			<div className="bg-white dark:bg-secondary-800 rounded-lg p-6 w-full max-w-md">
				<h3 className="text-lg font-medium text-secondary-900 dark:text-white mb-4">
					Add Notification Channel
				</h3>

				<form onSubmit={handleSubmit} className="space-y-4">
					<div>
						<label
							htmlFor={nameId}
							className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
						>
							Channel Name
						</label>
						<input
							id={nameId}
							type="text"
							name="name"
							required
							placeholder="e.g., Production Alerts"
							value={formData.name}
							onChange={handleInputChange}
							className="block w-full border border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white px-3 py-2"
						/>
					</div>

					<div>
						<label
							htmlFor={urlId}
							className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
						>
							Gotify Server URL
						</label>
						<input
							id={urlId}
							type="url"
							name="server_url"
							required
							placeholder="https://gotify.example.com"
							value={formData.server_url}
							onChange={handleInputChange}
							className="block w-full border border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white px-3 py-2"
						/>
					</div>

					<div>
						<label
							htmlFor={tokenId}
							className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
						>
							Application Token
						</label>
						<input
							id={tokenId}
							type="password"
							name="token"
							required
							placeholder="Your Gotify application token"
							value={formData.token}
							onChange={handleInputChange}
							className="block w-full border border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white px-3 py-2"
						/>
						<p className="mt-1 text-xs text-secondary-500 dark:text-secondary-400">
							Token will be encrypted and stored securely
						</p>
					</div>

					<div>
						<label
							htmlFor={priorityId}
							className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
						>
							Default Priority (0-10)
						</label>
						<input
							id={priorityId}
							type="number"
							name="priority"
							min="0"
							max="10"
							value={formData.priority}
							onChange={handleInputChange}
							className="block w-full border border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white px-3 py-2"
						/>
						<p className="mt-1 text-xs text-secondary-500 dark:text-secondary-400">
							Higher values indicate more urgent messages
						</p>
					</div>

					{success && (
						<div className="bg-green-50 dark:bg-green-900 border border-green-200 dark:border-green-700 rounded-md p-3">
							<div className="flex items-center">
								<CheckCircle className="h-4 w-4 text-green-600 dark:text-green-400 mr-2" />
								<p className="text-sm text-green-700 dark:text-green-300">
									Channel created successfully!
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

					<div className="flex justify-end space-x-3">
						<button
							type="button"
							onClick={onClose}
							className="px-4 py-2 text-sm font-medium text-secondary-700 dark:text-secondary-200 bg-white dark:bg-secondary-700 border border-secondary-300 dark:border-secondary-600 rounded-md hover:bg-secondary-50 dark:hover:bg-secondary-600"
						>
							Cancel
						</button>
						<button
							type="submit"
							disabled={isLoading}
							className="px-4 py-2 text-sm font-medium text-white bg-primary-600 border border-transparent rounded-md hover:bg-primary-700 disabled:opacity-50"
						>
							{isLoading ? "Creating..." : "Create Channel"}
						</button>
					</div>
				</form>
			</div>
		</div>
	);
};

// Edit Channel Modal Component
const EditChannelModal = ({
	channel,
	isOpen,
	onClose,
	onUpdateChannel,
	isLoading,
}) => {
	// eslint-disable-next-line react/prop-types
	const nameId = useId();
	const urlId = useId();
	const tokenId = useId();
	const priorityId = useId();

	const [formData, setFormData] = useState({
		name: channel?.name || "",
		server_url: channel?.server_url || "",
		token: "",
		priority: channel?.priority || 5,
	});
	const [error, setError] = useState("");
	const [success, setSuccess] = useState(false);

	// Update formData when channel prop changes or modal opens
	useEffect(() => {
		if (channel && isOpen) {
			setFormData({
				name: channel.name || "",
				server_url: channel.server_url || "",
				token: "",
				priority: channel.priority || 5,
			});
		}
	}, [channel, isOpen]);

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
			// Only include token if it was changed
			const updateData = {
				name: formData.name,
				server_url: formData.server_url,
				priority: formData.priority,
			};
			if (formData.token) {
				updateData.token = formData.token;
			}

			await onUpdateChannel({ id: channel.id, data: updateData });
			setSuccess(true);
			setTimeout(() => {
				onClose();
			}, 1500);
		} catch (err) {
			setError(err.message || "Failed to update channel");
		}
	};

	const handleInputChange = (e) => {
		const { name, value } = e.target;
		setFormData({
			...formData,
			[name]: name === "priority" ? Number.parseInt(value, 10) : value,
		});
	};

	if (!isOpen || !channel) return null;

	return (
		<div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
			<div className="bg-white dark:bg-secondary-800 rounded-lg p-6 w-full max-w-md">
				<h3 className="text-lg font-medium text-secondary-900 dark:text-white mb-4">
					Edit Notification Channel
				</h3>

				<form onSubmit={handleSubmit} className="space-y-4">
					<div>
						<label
							htmlFor={nameId}
							className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
						>
							Channel Name
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
							htmlFor={urlId}
							className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
						>
							Gotify Server URL
						</label>
						<input
							id={urlId}
							type="url"
							name="server_url"
							required
							value={formData.server_url}
							onChange={handleInputChange}
							className="block w-full border border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white px-3 py-2"
						/>
					</div>

					<div>
						<label
							htmlFor={tokenId}
							className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
						>
							Application Token (leave blank to keep current)
						</label>
						<input
							id={tokenId}
							type="password"
							name="token"
							placeholder="Leave blank to keep current token"
							value={formData.token}
							onChange={handleInputChange}
							className="block w-full border border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white px-3 py-2"
						/>
					</div>

					<div>
						<label
							htmlFor={priorityId}
							className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
						>
							Default Priority (0-10)
						</label>
						<input
							id={priorityId}
							type="number"
							name="priority"
							min="0"
							max="10"
							value={formData.priority}
							onChange={handleInputChange}
							className="block w-full border border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white px-3 py-2"
						/>
					</div>

					{success && (
						<div className="bg-green-50 dark:bg-green-900 border border-green-200 dark:border-green-700 rounded-md p-3">
							<div className="flex items-center">
								<CheckCircle className="h-4 w-4 text-green-600 dark:text-green-400 mr-2" />
								<p className="text-sm text-green-700 dark:text-green-300">
									Channel updated successfully!
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

					<div className="flex justify-end space-x-3">
						<button
							type="button"
							onClick={onClose}
							className="px-4 py-2 text-sm font-medium text-secondary-700 dark:text-secondary-200 bg-white dark:bg-secondary-700 border border-secondary-300 dark:border-secondary-600 rounded-md hover:bg-secondary-50 dark:hover:bg-secondary-600"
						>
							Cancel
						</button>
						<button
							type="submit"
							disabled={isLoading}
							className="px-4 py-2 text-sm font-medium text-white bg-primary-600 border border-transparent rounded-md hover:bg-primary-700 disabled:opacity-50"
						>
							{isLoading ? "Updating..." : "Update Channel"}
						</button>
					</div>
				</form>
			</div>
		</div>
	);
};
