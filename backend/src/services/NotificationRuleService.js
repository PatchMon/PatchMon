const { getPrismaClient } = require("../config/prisma");

/**
 * NotificationRuleService
 *
 * Manages notification rules and their execution.
 * Handles creation, updating, deletion, and matching of notification rules.
 */
class NotificationRuleService {
	constructor() {
		this.prisma = getPrismaClient();
	}

	/**
	 * Supported event types for notifications
	 */
	static SUPPORTED_EVENT_TYPES = [
		"package_update",
		"security_update",
		"host_status_change",
		"agent_update",
	];

	/**
	 * Create a new notification rule
	 *
	 * @param {Object} data - Rule configuration data
	 * @param {string} data.name - Rule name
	 * @param {string} [data.description] - Rule description
	 * @param {string} data.event_type - Event type (package_update, security_update, host_status_change, agent_update)
	 * @param {Array<string>} data.channel_ids - Array of channel IDs to send notifications to
	 * @param {Array<Object>} [data.filters] - Array of filter objects {filter_type, filter_value}
	 * @param {number} [data.priority=5] - Message priority (0-10)
	 * @param {string} [data.message_title] - Custom message title
	 * @param {string} [data.message_template] - Custom message template
	 * @param {string} [data.created_by_user_id] - User ID who created the rule
	 * @returns {Promise<Object>} Created rule object with channels and filters
	 * @throws {Error} If validation fails or channels don't exist
	 */
	async createRule(data) {
		// Validate required fields
		if (
			!data.name ||
			typeof data.name !== "string" ||
			data.name.trim().length === 0
		) {
			throw new Error("Rule name is required and must be a non-empty string");
		}

		if (!data.event_type || typeof data.event_type !== "string") {
			throw new Error("Event type is required and must be a string");
		}

		if (
			!NotificationRuleService.SUPPORTED_EVENT_TYPES.includes(data.event_type)
		) {
			throw new Error(
				`Event type must be one of: ${NotificationRuleService.SUPPORTED_EVENT_TYPES.join(", ")}`,
			);
		}

		if (!Array.isArray(data.channel_ids) || data.channel_ids.length === 0) {
			throw new Error("At least one channel ID is required");
		}

		// Validate priority if provided
		if (data.priority !== undefined) {
			if (
				typeof data.priority !== "number" ||
				data.priority < 0 ||
				data.priority > 10
			) {
				throw new Error("Priority must be a number between 0 and 10");
			}
		}

		// Verify all channels exist
		const channels = await this.prisma.notification_channels.findMany({
			where: {
				id: {
					in: data.channel_ids,
				},
			},
		});

		if (channels.length !== data.channel_ids.length) {
			throw new Error("One or more specified channels do not exist");
		}

		// Create rule with channels and filters
		const rule = await this.prisma.notification_rules.create({
			data: {
				name: data.name.trim(),
				description: data.description,
				event_type: data.event_type,
				enabled: true,
				priority: data.priority ?? 5,
				message_title: data.message_title,
				message_template: data.message_template,
				created_by_user_id: data.created_by_user_id,
				channels: {
					create: data.channel_ids.map((channelId) => ({
						channel_id: channelId,
					})),
				},
				filters: {
					create: (data.filters || []).map((filter) => ({
						filter_type: filter.filter_type,
						filter_value: filter.filter_value,
					})),
				},
			},
			include: {
				channels: {
					include: {
						channels: true,
					},
				},
				filters: true,
			},
		});

		return rule;
	}

	/**
	 * Update an existing notification rule
	 *
	 * @param {string} ruleId - Rule ID to update
	 * @param {Object} data - Updated rule data
	 * @param {string} [data.name] - Rule name
	 * @param {string} [data.description] - Rule description
	 * @param {string} [data.event_type] - Event type
	 * @param {Array<string>} [data.channel_ids] - Array of channel IDs
	 * @param {Array<Object>} [data.filters] - Array of filter objects
	 * @param {number} [data.priority] - Message priority
	 * @param {string} [data.message_title] - Custom message title
	 * @param {string} [data.message_template] - Custom message template
	 * @returns {Promise<Object>} Updated rule object
	 * @throws {Error} If rule not found or validation fails
	 */
	async updateRule(ruleId, data) {
		// Verify rule exists
		const existingRule = await this.prisma.notification_rules.findUnique({
			where: { id: ruleId },
			include: {
				channels: true,
				filters: true,
			},
		});

		if (!existingRule) {
			throw new Error(`Rule with ID ${ruleId} not found`);
		}

		// Prepare update data
		const updateData = {};

		if (data.name !== undefined) {
			if (typeof data.name !== "string" || data.name.trim().length === 0) {
				throw new Error("Rule name must be a non-empty string");
			}
			updateData.name = data.name.trim();
		}

		if (data.description !== undefined) {
			updateData.description = data.description;
		}

		if (data.event_type !== undefined) {
			if (
				!NotificationRuleService.SUPPORTED_EVENT_TYPES.includes(data.event_type)
			) {
				throw new Error(
					`Event type must be one of: ${NotificationRuleService.SUPPORTED_EVENT_TYPES.join(", ")}`,
				);
			}
			updateData.event_type = data.event_type;
		}

		if (data.priority !== undefined) {
			if (
				typeof data.priority !== "number" ||
				data.priority < 0 ||
				data.priority > 10
			) {
				throw new Error("Priority must be a number between 0 and 10");
			}
			updateData.priority = data.priority;
		}

		if (data.message_title !== undefined) {
			updateData.message_title = data.message_title;
		}

		if (data.message_template !== undefined) {
			updateData.message_template = data.message_template;
		}

		// Handle channel updates
		if (data.channel_ids !== undefined) {
			if (!Array.isArray(data.channel_ids) || data.channel_ids.length === 0) {
				throw new Error("At least one channel ID is required");
			}

			// Verify all channels exist
			const channels = await this.prisma.notification_channels.findMany({
				where: {
					id: {
						in: data.channel_ids,
					},
				},
			});

			if (channels.length !== data.channel_ids.length) {
				throw new Error("One or more specified channels do not exist");
			}

			// Delete existing channel associations and create new ones
			await this.prisma.notification_rule_channels.deleteMany({
				where: { rule_id: ruleId },
			});

			updateData.channels = {
				create: data.channel_ids.map((channelId) => ({
					channel_id: channelId,
				})),
			};
		}

		// Handle filter updates
		if (data.filters !== undefined) {
			// Delete existing filters and create new ones
			await this.prisma.notification_rule_filters.deleteMany({
				where: { rule_id: ruleId },
			});

			updateData.filters = {
				create: (data.filters || []).map((filter) => ({
					filter_type: filter.filter_type,
					filter_value: filter.filter_value,
				})),
			};
		}

		// Update rule
		const updatedRule = await this.prisma.notification_rules.update({
			where: { id: ruleId },
			data: updateData,
			include: {
				channels: {
					include: {
						channels: true,
					},
				},
				filters: true,
			},
		});

		return updatedRule;
	}

	/**
	 * Delete a notification rule
	 *
	 * @param {string} ruleId - Rule ID to delete
	 * @returns {Promise<Object>} Deleted rule object
	 * @throws {Error} If rule not found
	 */
	async deleteRule(ruleId) {
		// Verify rule exists
		const existingRule = await this.prisma.notification_rules.findUnique({
			where: { id: ruleId },
		});

		if (!existingRule) {
			throw new Error(`Rule with ID ${ruleId} not found`);
		}

		// Delete rule (cascading deletion will handle associated channels and filters)
		const deletedRule = await this.prisma.notification_rules.delete({
			where: { id: ruleId },
		});

		return deletedRule;
	}

	/**
	 * Get all notification rules
	 *
	 * @returns {Promise<Array>} Array of rule objects with channels and filters
	 */
	async getRules() {
		const rules = await this.prisma.notification_rules.findMany({
			include: {
				channels: {
					include: {
						channels: true,
					},
				},
				filters: true,
			},
			orderBy: { created_at: "desc" },
		});

		return rules;
	}

	/**
	 * Toggle a rule's enabled/disabled state
	 *
	 * @param {string} ruleId - Rule ID to toggle
	 * @returns {Promise<Object>} Updated rule object with new enabled state
	 * @throws {Error} If rule not found
	 */
	async toggleRule(ruleId) {
		// Get current rule
		const existingRule = await this.prisma.notification_rules.findUnique({
			where: { id: ruleId },
		});

		if (!existingRule) {
			throw new Error(`Rule with ID ${ruleId} not found`);
		}

		// Toggle enabled state
		const updatedRule = await this.prisma.notification_rules.update({
			where: { id: ruleId },
			data: {
				enabled: !existingRule.enabled,
			},
			include: {
				channels: {
					include: {
						channels: true,
					},
				},
				filters: true,
			},
		});

		return updatedRule;
	}

	/**
	 * Get all rules matching a specific event type
	 *
	 * @param {string} eventType - Event type to match
	 * @returns {Promise<Array>} Array of matching enabled rules
	 */
	async getRulesForEvent(eventType) {
		const rules = await this.prisma.notification_rules.findMany({
			where: {
				event_type: eventType,
				enabled: true,
			},
			include: {
				channels: {
					include: {
						channels: true,
					},
				},
				filters: true,
			},
		});

		return rules;
	}

	/**
	 * Check if an event matches a rule's filters
	 *
	 * @param {Object} rule - Rule object with filters
	 * @param {Object} eventData - Event data to check against filters
	 * @param {string} [eventData.host_id] - Host ID from event
	 * @param {string} [eventData.host_group_id] - Host group ID from event
	 * @returns {boolean} True if event matches all filters (or no filters), false otherwise
	 */
	applyFilters(rule, eventData) {
		// If no filters, rule matches all events
		if (!rule.filters || rule.filters.length === 0) {
			return true;
		}

		// Check each filter
		for (const filter of rule.filters) {
			if (filter.filter_type === "host_id") {
				if (eventData.host_id !== filter.filter_value) {
					return false;
				}
			} else if (filter.filter_type === "host_group_id") {
				if (eventData.host_group_id !== filter.filter_value) {
					return false;
				}
			}
		}

		return true;
	}
}

module.exports = NotificationRuleService;
