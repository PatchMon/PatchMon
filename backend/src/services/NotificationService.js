const { getPrismaClient } = require("../config/prisma");
const GotifyService = require("./GotifyService");
const NotificationRuleService = require("./NotificationRuleService");
const NotificationChannelService = require("./NotificationChannelService");

/**
 * NotificationService
 *
 * Orchestrates notification sending and history tracking.
 * Handles finding matching rules for events, sending notifications to channels,
 * and maintaining an audit trail of all notification delivery attempts.
 */
class NotificationService {
	constructor() {
		this.prisma = getPrismaClient();
		this.gotifyService = new GotifyService();
		this.ruleService = new NotificationRuleService();
		this.channelService = new NotificationChannelService();
	}

	/**
	 * Send notifications for a system event
	 *
	 * Finds all rules matching the event type, applies filters, and sends notifications
	 * to all configured channels for matching rules.
	 *
	 * @param {string} eventType - Type of event (package_update, security_update, host_status_change, agent_update)
	 * @param {Object} eventData - Event data containing context for the notification
	 * @param {string} [eventData.host_id] - ID of the affected host
	 * @param {string} [eventData.host_group_id] - ID of the affected host group
	 * @param {string} [eventData.package_name] - Name of the package (for package events)
	 * @param {string} [eventData.package_version] - Version of the package
	 * @param {string} [eventData.available_version] - Available version of the package
	 * @param {boolean} [eventData.is_security_update] - Whether this is a security update
	 * @param {string} [eventData.host_status] - New host status (for status change events)
	 * @param {string} [eventData.agent_version] - Agent version (for agent update events)
	 * @returns {Promise<Object>} Summary of notifications sent {sent: number, failed: number}
	 */
	async sendNotification(eventType, eventData) {
		try {
			// Get all enabled rules for this event type
			const matchingRules = await this.ruleService.getRulesForEvent(eventType);

			let sentCount = 0;
			let failedCount = 0;

			// Process each matching rule
			for (const rule of matchingRules) {
				// Check if event matches rule filters
				if (!this.ruleService.applyFilters(rule, eventData)) {
					continue;
				}

				// Get channels for this rule
				const channels = rule.channels || [];

				// Send to each channel
				for (const ruleChannel of channels) {
					const channel = ruleChannel.channels;

					// Format the message
					const message = this._formatMessage(rule, eventData);

					// Send to Gotify
					const sendResult = await this.gotifyService.sendMessage(
						channel.server_url,
						channel.token,
						{
							title: message.title,
							message: message.body,
							priority: rule.priority,
						},
					);

					// Log delivery attempt
					if (sendResult.success) {
						await this.logDelivery(
							channel.id,
							rule.id,
							"sent",
							message.body,
							null,
						);
						sentCount++;

						// Update channel status to connected
						await this.channelService.updateChannelStatus(
							channel.id,
							"connected",
						);
					} else {
						await this.logDelivery(
							channel.id,
							rule.id,
							"failed",
							message.body,
							sendResult.error,
						);
						failedCount++;

						// Update channel status to disconnected
						await this.channelService.updateChannelStatus(
							channel.id,
							"disconnected",
							sendResult.error,
						);
					}
				}
			}

			return {
				sent: sentCount,
				failed: failedCount,
			};
		} catch (error) {
			console.error("Error sending notifications:", error);
			throw error;
		}
	}

	/**
	 * Get notification history with optional filtering
	 *
	 * @param {Object} filters - Filter criteria
	 * @param {Date} [filters.start_date] - Start date for filtering
	 * @param {Date} [filters.end_date] - End date for filtering
	 * @param {string} [filters.event_type] - Filter by event type
	 * @param {string} [filters.channel_id] - Filter by channel ID
	 * @param {string} [filters.status] - Filter by delivery status (sent/failed)
	 * @param {number} [filters.limit=100] - Maximum number of results
	 * @param {number} [filters.offset=0] - Offset for pagination
	 * @returns {Promise<Array>} Array of notification history entries
	 */
	async getHistory(filters = {}) {
		try {
			const {
				start_date,
				end_date,
				event_type,
				channel_id,
				status,
				limit = 100,
				offset = 0,
			} = filters;

			// Build where clause
			const where = {};

			if (start_date || end_date) {
				where.sent_at = {};
				if (start_date) {
					where.sent_at.gte = new Date(start_date);
				}
				if (end_date) {
					where.sent_at.lte = new Date(end_date);
				}
			}

			if (event_type) {
				where.event_type = event_type;
			}

			if (channel_id) {
				where.channel_id = channel_id;
			}

			if (status) {
				where.status = status;
			}

			// Query history
			const history = await this.prisma.notification_history.findMany({
				where,
				include: {
					channels: true,
					rules: true,
				},
				orderBy: { sent_at: "desc" },
				take: limit,
				skip: offset,
			});

			return history;
		} catch (error) {
			console.error("Error retrieving notification history:", error);
			throw error;
		}
	}

	/**
	 * Log a notification delivery attempt
	 *
	 * @param {string} channelId - ID of the notification channel
	 * @param {string} ruleId - ID of the notification rule
	 * @param {string} status - Delivery status ('sent' or 'failed')
	 * @param {string} messageContent - The message that was sent
	 * @param {string} [errorMessage] - Error message if delivery failed
	 * @returns {Promise<Object>} Created history entry
	 */
	async logDelivery(
		channelId,
		ruleId,
		status,
		messageContent,
		errorMessage = null,
	) {
		try {
			// Get the rule to extract event type
			const rule = await this.prisma.notification_rules.findUnique({
				where: { id: ruleId },
			});

			if (!rule) {
				throw new Error(`Rule with ID ${ruleId} not found`);
			}

			// Create history entry
			const historyEntry = await this.prisma.notification_history.create({
				data: {
					channel_id: channelId,
					rule_id: ruleId,
					event_type: rule.event_type,
					status,
					message_title: rule.message_title,
					message_content: messageContent,
					error_message: errorMessage,
					sent_at: new Date(),
				},
			});

			return historyEntry;
		} catch (error) {
			console.error("Error logging notification delivery:", error);
			throw error;
		}
	}

	/**
	 * Format a message for a notification rule
	 *
	 * Renders the message template with event data, or uses a default format
	 * if no template is provided.
	 *
	 * @private
	 * @param {Object} rule - Notification rule with template
	 * @param {Object} eventData - Event data to include in message
	 * @returns {Object} Formatted message {title: string, body: string}
	 */
	_formatMessage(rule, eventData) {
		// Use custom title if provided, otherwise generate default
		const title =
			rule.message_title || this._generateDefaultTitle(rule.event_type);

		// Use custom template if provided, otherwise generate default
		const body = rule.message_template
			? this._renderTemplate(rule.message_template, eventData)
			: this._generateDefaultMessage(rule.event_type, eventData);

		return { title, body };
	}

	/**
	 * Generate a default title for an event type
	 *
	 * @private
	 * @param {string} eventType - Type of event
	 * @returns {string} Default title
	 */
	_generateDefaultTitle(eventType) {
		const titles = {
			package_update: "Package Update Available",
			security_update: "Security Update Available",
			host_status_change: "Host Status Changed",
			agent_update: "Agent Update Available",
		};

		return titles[eventType] || "PatchMon Notification";
	}

	/**
	 * Generate a default message for an event type
	 *
	 * @private
	 * @param {string} eventType - Type of event
	 * @param {Object} eventData - Event data
	 * @returns {string} Default message
	 */
	_generateDefaultMessage(eventType, eventData) {
		switch (eventType) {
			case "package_update":
				return `Package update available: ${eventData.package_name} (${eventData.package_version} → ${eventData.available_version})`;

			case "security_update":
				return `Security update available: ${eventData.package_name} (${eventData.package_version} → ${eventData.available_version})`;

			case "host_status_change":
				return `Host status changed to: ${eventData.host_status}`;

			case "agent_update":
				return `Agent update available: ${eventData.agent_version}`;

			default:
				return "A system event has occurred";
		}
	}

	/**
	 * Render a message template with event data
	 *
	 * Simple template rendering that replaces {{key}} placeholders with values from eventData.
	 *
	 * @private
	 * @param {string} template - Template string with {{key}} placeholders
	 * @param {Object} eventData - Data to substitute into template
	 * @returns {string} Rendered message
	 */
	_renderTemplate(template, eventData) {
		let rendered = template;

		// Replace all {{key}} placeholders with values from eventData
		Object.entries(eventData).forEach(([key, value]) => {
			const placeholder = `{{${key}}}`;
			rendered = rendered.replace(
				new RegExp(placeholder, "g"),
				String(value || ""),
			);
		});

		return rendered;
	}

	/**
	 * Get notification history count with optional filtering
	 *
	 * @param {Object} filters - Filter criteria (same as getHistory)
	 * @returns {Promise<number>} Total count of matching history entries
	 */
	async getHistoryCount(filters = {}) {
		try {
			const { start_date, end_date, event_type, channel_id, status } = filters;

			// Build where clause
			const where = {};

			if (start_date || end_date) {
				where.sent_at = {};
				if (start_date) {
					where.sent_at.gte = new Date(start_date);
				}
				if (end_date) {
					where.sent_at.lte = new Date(end_date);
				}
			}

			if (event_type) {
				where.event_type = event_type;
			}

			if (channel_id) {
				where.channel_id = channel_id;
			}

			if (status) {
				where.status = status;
			}

			// Count matching entries
			const count = await this.prisma.notification_history.count({ where });

			return count;
		} catch (error) {
			console.error("Error counting notification history:", error);
			throw error;
		}
	}
}

module.exports = NotificationService;
