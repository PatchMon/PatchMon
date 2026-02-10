import { useQuery } from "@tanstack/react-query";
import {
	AlertCircle,
	ChevronDown,
	ChevronUp,
	Filter,
	XCircle,
} from "lucide-react";
import { useState } from "react";

// API functions for notification history
const notificationHistoryAPI = {
	list: (filters) => {
		const params = new URLSearchParams();

		if (filters.start_date) {
			params.append("start_date", filters.start_date);
		}
		if (filters.end_date) {
			params.append("end_date", filters.end_date);
		}
		if (filters.event_type) {
			params.append("event_type", filters.event_type);
		}
		if (filters.channel_id) {
			params.append("channel_id", filters.channel_id);
		}
		if (filters.status) {
			params.append("status", filters.status);
		}
		if (filters.limit) {
			params.append("limit", filters.limit);
		}
		if (filters.offset) {
			params.append("offset", filters.offset);
		}

		return fetch(`/api/v1/notifications/history?${params.toString()}`, {
			headers: {
				Authorization: `Bearer ${localStorage.getItem("token")}`,
			},
		}).then((res) => res.json());
	},
};

// API functions for notification channels (for filter dropdown)
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

const STATUS_OPTIONS = [
	{ value: "sent", label: "Sent" },
	{ value: "failed", label: "Failed" },
];

const NotificationHistory = () => {
	const [filters, setFilters] = useState({
		start_date: "",
		end_date: "",
		event_type: "",
		channel_id: "",
		status: "",
		limit: 50,
		offset: 0,
	});
	const [expandedRowId, setExpandedRowId] = useState(null);
	const [showFilters, setShowFilters] = useState(false);

	// Fetch history
	const {
		data: historyData = { data: [], pagination: {} },
		isLoading,
		error,
	} = useQuery({
		queryKey: ["notificationHistory", filters],
		queryFn: () => notificationHistoryAPI.list(filters),
	});

	// Fetch channels for filter dropdown
	const { data: channels = [] } = useQuery({
		queryKey: ["notificationChannels"],
		queryFn: notificationChannelsAPI.list,
	});

	const handleFilterChange = (e) => {
		const { name, value } = e.target;
		setFilters({
			...filters,
			[name]: value,
			offset: 0, // Reset pagination when filters change
		});
	};

	const handleDateChange = (e) => {
		const { name, value } = e.target;
		setFilters({
			...filters,
			[name]: value,
			offset: 0,
		});
	};

	const handleClearFilters = () => {
		setFilters({
			start_date: "",
			end_date: "",
			event_type: "",
			channel_id: "",
			status: "",
			limit: 50,
			offset: 0,
		});
	};

	const handlePreviousPage = () => {
		setFilters({
			...filters,
			offset: Math.max(0, filters.offset - filters.limit),
		});
	};

	const handleNextPage = () => {
		if (historyData.pagination?.hasMore) {
			setFilters({
				...filters,
				offset: filters.offset + filters.limit,
			});
		}
	};

	const toggleRowExpansion = (rowId) => {
		setExpandedRowId(expandedRowId === rowId ? null : rowId);
	};

	const getEventTypeLabel = (eventType) => {
		return EVENT_TYPES.find((e) => e.value === eventType)?.label || eventType;
	};

	const getChannelName = (channelId) => {
		const channel = channels.find((c) => c.id === channelId);
		return channel?.name || channelId;
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
							Error loading notification history
						</h3>
						<p className="mt-1 text-sm text-danger-700 dark:text-danger-300">
							{error.message}
						</p>
					</div>
				</div>
			</div>
		);
	}

	const history = historyData.data || [];
	const pagination = historyData.pagination || {};

	return (
		<div className="space-y-6">
			{/* Header */}
			<div className="flex items-center justify-between">
				<div>
					<h2 className="text-xl font-bold text-secondary-900 dark:text-white">
						Notification History
					</h2>
					<p className="mt-1 text-sm text-secondary-600 dark:text-secondary-400">
						View all sent notifications and delivery status
					</p>
				</div>
				<button
					type="button"
					onClick={() => setShowFilters(!showFilters)}
					className="inline-flex items-center gap-2 px-4 py-2 bg-secondary-100 dark:bg-secondary-700 text-secondary-700 dark:text-secondary-200 rounded-md hover:bg-secondary-200 dark:hover:bg-secondary-600 transition-colors"
				>
					<Filter className="h-4 w-4" />
					Filters
				</button>
			</div>

			{/* Filters Section */}
			{showFilters && (
				<div className="bg-white dark:bg-secondary-800 border border-secondary-200 dark:border-secondary-600 rounded-lg p-4 space-y-4">
					<div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
						{/* Date Range */}
						<div>
							<label
								htmlFor="start_date"
								className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
							>
								Start Date
							</label>
							<input
								id="start_date"
								type="date"
								name="start_date"
								value={filters.start_date}
								onChange={handleDateChange}
								className="block w-full border border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white px-3 py-2"
							/>
						</div>

						<div>
							<label
								htmlFor="end_date"
								className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
							>
								End Date
							</label>
							<input
								id="end_date"
								type="date"
								name="end_date"
								value={filters.end_date}
								onChange={handleDateChange}
								className="block w-full border border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white px-3 py-2"
							/>
						</div>

						{/* Event Type */}
						<div>
							<label
								htmlFor="event_type"
								className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
							>
								Event Type
							</label>
							<select
								id="event_type"
								name="event_type"
								value={filters.event_type}
								onChange={handleFilterChange}
								className="block w-full border border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white px-3 py-2"
							>
								<option value="">All Event Types</option>
								{EVENT_TYPES.map((type) => (
									<option key={type.value} value={type.value}>
										{type.label}
									</option>
								))}
							</select>
						</div>

						{/* Channel */}
						<div>
							<label
								htmlFor="channel_id"
								className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
							>
								Channel
							</label>
							<select
								id="channel_id"
								name="channel_id"
								value={filters.channel_id}
								onChange={handleFilterChange}
								className="block w-full border border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white px-3 py-2"
							>
								<option value="">All Channels</option>
								{channels.map((channel) => (
									<option key={channel.id} value={channel.id}>
										{channel.name}
									</option>
								))}
							</select>
						</div>

						{/* Status */}
						<div>
							<label
								htmlFor="status"
								className="block text-sm font-medium text-secondary-700 dark:text-secondary-200 mb-1"
							>
								Status
							</label>
							<select
								id="status"
								name="status"
								value={filters.status}
								onChange={handleFilterChange}
								className="block w-full border border-secondary-300 dark:border-secondary-600 rounded-md shadow-sm focus:ring-primary-500 focus:border-primary-500 bg-white dark:bg-secondary-700 text-secondary-900 dark:text-white px-3 py-2"
							>
								<option value="">All Statuses</option>
								{STATUS_OPTIONS.map((option) => (
									<option key={option.value} value={option.value}>
										{option.label}
									</option>
								))}
							</select>
						</div>
					</div>

					<div className="flex justify-end">
						<button
							type="button"
							onClick={handleClearFilters}
							className="px-4 py-2 text-sm font-medium text-secondary-700 dark:text-secondary-200 bg-white dark:bg-secondary-700 border border-secondary-300 dark:border-secondary-600 rounded-md hover:bg-secondary-50 dark:hover:bg-secondary-600"
						>
							Clear Filters
						</button>
					</div>
				</div>
			)}

			{/* History Table */}
			<div className="bg-white dark:bg-secondary-800 shadow overflow-hidden sm:rounded-lg">
				{history && history.length > 0 ? (
					<>
						{/* Mobile Card Layout */}
						<div className="md:hidden space-y-3 p-4">
							{history.map((entry) => (
								<HistoryCard
									key={entry.id}
									entry={entry}
									isExpanded={expandedRowId === entry.id}
									onToggleExpand={() => toggleRowExpansion(entry.id)}
									getChannelName={getChannelName}
									getEventTypeLabel={getEventTypeLabel}
								/>
							))}
						</div>

						{/* Desktop Table Layout */}
						<div className="hidden md:block overflow-x-auto">
							<table className="min-w-full divide-y divide-secondary-200 dark:divide-secondary-600">
								<thead className="bg-secondary-50 dark:bg-secondary-700">
									<tr>
										<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
											Timestamp
										</th>
										<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
											Event Type
										</th>
										<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
											Channel
										</th>
										<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
											Status
										</th>
										<th className="px-6 py-3 text-left text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
											Message
										</th>
										<th className="px-6 py-3 text-right text-xs font-medium text-secondary-500 dark:text-secondary-300 uppercase tracking-wider">
											Actions
										</th>
									</tr>
								</thead>
								<tbody className="bg-white dark:bg-secondary-800 divide-y divide-secondary-200 dark:divide-secondary-600">
									{history.map((entry) => (
										<tr
											key={entry.id}
											className="hover:bg-secondary-50 dark:hover:bg-secondary-700"
										>
											<td className="px-6 py-4 whitespace-nowrap">
												<div className="text-sm text-secondary-900 dark:text-white">
													{new Date(entry.sent_at).toLocaleString()}
												</div>
											</td>
											<td className="px-6 py-4 whitespace-nowrap">
												<div className="text-sm text-secondary-500 dark:text-secondary-300">
													{getEventTypeLabel(entry.event_type)}
												</div>
											</td>
											<td className="px-6 py-4 whitespace-nowrap">
												<div className="text-sm text-secondary-500 dark:text-secondary-300">
													{getChannelName(entry.channel_id)}
												</div>
											</td>
											<td className="px-6 py-4 whitespace-nowrap">
												<StatusBadge status={entry.status} />
											</td>
											<td className="px-6 py-4">
												<div className="text-sm text-secondary-500 dark:text-secondary-300 truncate max-w-xs">
													{entry.message_title || entry.message_content || "—"}
												</div>
											</td>
											<td className="px-6 py-4 whitespace-nowrap text-right text-sm font-medium">
												<button
													type="button"
													onClick={() => toggleRowExpansion(entry.id)}
													className="text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
													title="View details"
												>
													{expandedRowId === entry.id ? (
														<ChevronUp className="h-4 w-4" />
													) : (
														<ChevronDown className="h-4 w-4" />
													)}
												</button>
											</td>
										</tr>
									))}
								</tbody>
							</table>

							{/* Expanded Row Details */}
							{expandedRowId && (
								<ExpandedRowDetails
									entry={history.find((e) => e.id === expandedRowId)}
									getChannelName={getChannelName}
									getEventTypeLabel={getEventTypeLabel}
								/>
							)}
						</div>

						{/* Pagination */}
						<div className="bg-white dark:bg-secondary-800 px-4 py-3 flex items-center justify-between border-t border-secondary-200 dark:border-secondary-600 sm:px-6">
							<div className="flex-1 flex justify-between sm:hidden">
								<button
									type="button"
									onClick={handlePreviousPage}
									disabled={filters.offset === 0}
									className="relative inline-flex items-center px-4 py-2 border border-secondary-300 dark:border-secondary-600 text-sm font-medium rounded-md text-secondary-700 dark:text-secondary-200 bg-white dark:bg-secondary-700 hover:bg-secondary-50 dark:hover:bg-secondary-600 disabled:opacity-50 disabled:cursor-not-allowed"
								>
									Previous
								</button>
								<button
									type="button"
									onClick={handleNextPage}
									disabled={!pagination.hasMore}
									className="ml-3 relative inline-flex items-center px-4 py-2 border border-secondary-300 dark:border-secondary-600 text-sm font-medium rounded-md text-secondary-700 dark:text-secondary-200 bg-white dark:bg-secondary-700 hover:bg-secondary-50 dark:hover:bg-secondary-600 disabled:opacity-50 disabled:cursor-not-allowed"
								>
									Next
								</button>
							</div>
							<div className="hidden sm:flex-1 sm:flex sm:items-center sm:justify-between">
								<div>
									<p className="text-sm text-secondary-700 dark:text-secondary-300">
										Showing{" "}
										<span className="font-medium">{filters.offset + 1}</span> to{" "}
										<span className="font-medium">
											{Math.min(
												filters.offset + filters.limit,
												pagination.total,
											)}
										</span>{" "}
										of <span className="font-medium">{pagination.total}</span>{" "}
										results
									</p>
								</div>
								<div>
									<nav
										className="relative z-0 inline-flex rounded-md shadow-sm -space-x-px"
										aria-label="Pagination"
									>
										<button
											type="button"
											onClick={handlePreviousPage}
											disabled={filters.offset === 0}
											className="relative inline-flex items-center px-2 py-2 rounded-l-md border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-700 text-sm font-medium text-secondary-500 dark:text-secondary-400 hover:bg-secondary-50 dark:hover:bg-secondary-600 disabled:opacity-50 disabled:cursor-not-allowed"
										>
											<span className="sr-only">Previous</span>
											<ChevronUp className="h-5 w-5 rotate-90" />
										</button>
										<button
											type="button"
											onClick={handleNextPage}
											disabled={!pagination.hasMore}
											className="relative inline-flex items-center px-2 py-2 rounded-r-md border border-secondary-300 dark:border-secondary-600 bg-white dark:bg-secondary-700 text-sm font-medium text-secondary-500 dark:text-secondary-400 hover:bg-secondary-50 dark:hover:bg-secondary-600 disabled:opacity-50 disabled:cursor-not-allowed"
										>
											<span className="sr-only">Next</span>
											<ChevronUp className="h-5 w-5 -rotate-90" />
										</button>
									</nav>
								</div>
							</div>
						</div>
					</>
				) : (
					<div className="p-12 text-center">
						<AlertCircle className="h-12 w-12 text-secondary-400 mx-auto mb-4" />
						<p className="text-secondary-500 dark:text-secondary-300">
							No notification history found
						</p>
						<p className="text-sm text-secondary-400 dark:text-secondary-400 mt-2">
							Notifications will appear here once they are sent
						</p>
					</div>
				)}
			</div>
		</div>
	);
};

// Status Badge Component
const StatusBadge = ({ status }) => {
	if (status === "sent") {
		return (
			<span className="inline-flex items-center px-2.5 py-0.5 rounded-md text-xs font-medium bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">
				✓ Sent
			</span>
		);
	}
	return (
		<span className="inline-flex items-center px-2.5 py-0.5 rounded-md text-xs font-medium bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200">
			✗ Failed
		</span>
	);
};

// History Card Component (Mobile)
const HistoryCard = ({
	entry,
	isExpanded,
	onToggleExpand,
	getChannelName,
	getEventTypeLabel,
}) => {
	return (
		<div className="card p-4 space-y-3">
			<div className="flex items-start justify-between">
				<div className="flex-1">
					<div className="text-sm font-semibold text-secondary-900 dark:text-white">
						{new Date(entry.sent_at).toLocaleString()}
					</div>
					<div className="text-xs text-secondary-500 dark:text-secondary-400 mt-1">
						{getEventTypeLabel(entry.event_type)}
					</div>
				</div>
				<StatusBadge status={entry.status} />
			</div>

			<div className="text-sm text-secondary-600 dark:text-secondary-300 border-t border-secondary-200 dark:border-secondary-600 pt-2">
				<span className="text-secondary-500 dark:text-secondary-400">
					Channel:&nbsp;
				</span>
				{getChannelName(entry.channel_id)}
			</div>

			<div className="text-sm text-secondary-600 dark:text-secondary-300">
				<span className="text-secondary-500 dark:text-secondary-400">
					Message:&nbsp;
				</span>
				<span className="truncate">
					{entry.message_title || entry.message_content || "—"}
				</span>
			</div>

			<button
				type="button"
				onClick={onToggleExpand}
				className="w-full text-left text-sm text-primary-600 dark:text-primary-400 hover:text-primary-700 dark:hover:text-primary-300 font-medium pt-2 border-t border-secondary-200 dark:border-secondary-600"
			>
				{isExpanded ? "Hide Details" : "View Details"}
			</button>

			{isExpanded && (
				<ExpandedRowDetails
					entry={entry}
					getChannelName={getChannelName}
					getEventTypeLabel={getEventTypeLabel}
				/>
			)}
		</div>
	);
};

// Expanded Row Details Component
const ExpandedRowDetails = ({ entry, getChannelName, getEventTypeLabel }) => {
	return (
		<div className="bg-secondary-50 dark:bg-secondary-700 p-4 rounded space-y-3 text-sm">
			<div>
				<div className="font-medium text-secondary-900 dark:text-white mb-1">
					Full Message Content
				</div>
				<div className="bg-white dark:bg-secondary-800 p-3 rounded border border-secondary-200 dark:border-secondary-600 text-secondary-700 dark:text-secondary-300 whitespace-pre-wrap break-words">
					{entry.message_content || "No message content"}
				</div>
			</div>

			{entry.status === "failed" && entry.error_message && (
				<div>
					<div className="font-medium text-danger-900 dark:text-danger-200 mb-1">
						Error Details
					</div>
					<div className="bg-danger-50 dark:bg-danger-900 p-3 rounded border border-danger-200 dark:border-danger-700 text-danger-700 dark:text-danger-300 whitespace-pre-wrap break-words">
						{entry.error_message}
					</div>
				</div>
			)}

			<div className="grid grid-cols-2 gap-3 pt-2 border-t border-secondary-200 dark:border-secondary-600">
				<div>
					<div className="text-secondary-500 dark:text-secondary-400">
						Event Type
					</div>
					<div className="text-secondary-900 dark:text-white font-medium">
						{getEventTypeLabel(entry.event_type)}
					</div>
				</div>
				<div>
					<div className="text-secondary-500 dark:text-secondary-400">
						Channel
					</div>
					<div className="text-secondary-900 dark:text-white font-medium">
						{getChannelName(entry.channel_id)}
					</div>
				</div>
				<div>
					<div className="text-secondary-500 dark:text-secondary-400">
						Status
					</div>
					<div className="text-secondary-900 dark:text-white font-medium">
						{entry.status === "sent" ? "Sent" : "Failed"}
					</div>
				</div>
				<div>
					<div className="text-secondary-500 dark:text-secondary-400">
						Sent At
					</div>
					<div className="text-secondary-900 dark:text-white font-medium">
						{new Date(entry.sent_at).toLocaleString()}
					</div>
				</div>
			</div>
		</div>
	);
};

export default NotificationHistory;
