const { getPrismaClient } = require("../config/prisma");
const GotifyService = require("./GotifyService");

/**
 * NotificationChannelService
 *
 * Manages Gotify notification channel configurations.
 * Handles creation, updating, deletion, and testing of notification channels.
 */
class NotificationChannelService {
	constructor() {
		this.prisma = getPrismaClient();
		this.gotifyService = new GotifyService();
	}

	/**
	 * Create a new notification channel
	 *
	 * @param {Object} data - Channel configuration data
	 * @param {string} data.name - Channel name
	 * @param {string} data.server_url - Gotify server URL
	 * @param {string} data.token - Gotify application token
	 * @param {number} [data.priority=5] - Default message priority (0-10)
	 * @param {string} [data.created_by_user_id] - User ID who created the channel
	 * @returns {Promise<Object>} Created channel object
	 * @throws {Error} If validation fails or connection test fails
	 */
	async createChannel(data) {
		// Validate required fields
		if (
			!data.name ||
			typeof data.name !== "string" ||
			data.name.trim().length === 0
		) {
			throw new Error(
				"Channel name is required and must be a non-empty string",
			);
		}

		if (!data.server_url || typeof data.server_url !== "string") {
			throw new Error("Server URL is required and must be a string");
		}

		if (!data.token || typeof data.token !== "string") {
			throw new Error("Token is required and must be a string");
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

		// Test connection to Gotify server
		const connectionResult = await this.gotifyService.validateConnection(
			data.server_url,
			data.token,
		);

		if (!connectionResult.valid) {
			throw new Error(
				`Gotify connection validation failed: ${connectionResult.error}`,
			);
		}

		// Create channel in database
		const channel = await this.prisma.notification_channels.create({
			data: {
				name: data.name.trim(),
				channel_type: "gotify",
				server_url: data.server_url,
				token: data.token,
				priority: data.priority ?? 5,
				status: "connected",
				last_tested_at: new Date(),
				created_by_user_id: data.created_by_user_id,
			},
		});

		return channel;
	}

	/**
	 * Update an existing notification channel
	 *
	 * @param {string} channelId - Channel ID to update
	 * @param {Object} data - Updated channel data
	 * @param {string} [data.name] - Channel name
	 * @param {string} [data.server_url] - Gotify server URL
	 * @param {string} [data.token] - Gotify application token
	 * @param {number} [data.priority] - Default message priority
	 * @returns {Promise<Object>} Updated channel object
	 * @throws {Error} If channel not found or validation fails
	 */
	async updateChannel(channelId, data) {
		// Verify channel exists
		const existingChannel = await this.prisma.notification_channels.findUnique({
			where: { id: channelId },
		});

		if (!existingChannel) {
			throw new Error(`Channel with ID ${channelId} not found`);
		}

		// Prepare update data
		const updateData = {};

		if (data.name !== undefined) {
			if (typeof data.name !== "string" || data.name.trim().length === 0) {
				throw new Error("Channel name must be a non-empty string");
			}
			updateData.name = data.name.trim();
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

		// If URL or token is being updated, validate connection
		if (data.server_url !== undefined || data.token !== undefined) {
			const urlToTest = data.server_url ?? existingChannel.server_url;
			const tokenToTest = data.token ?? existingChannel.token;

			const connectionResult = await this.gotifyService.validateConnection(
				urlToTest,
				tokenToTest,
			);

			if (!connectionResult.valid) {
				throw new Error(
					`Gotify connection validation failed: ${connectionResult.error}`,
				);
			}

			if (data.server_url !== undefined) {
				updateData.server_url = data.server_url;
			}

			if (data.token !== undefined) {
				updateData.token = data.token;
			}

			updateData.status = "connected";
			updateData.last_tested_at = new Date();
			updateData.last_error = null;
		}

		// Update channel in database
		const updatedChannel = await this.prisma.notification_channels.update({
			where: { id: channelId },
			data: updateData,
		});

		return updatedChannel;
	}

	/**
	 * Delete a notification channel and its associated rules
	 *
	 * @param {string} channelId - Channel ID to delete
	 * @returns {Promise<Object>} Deleted channel object
	 * @throws {Error} If channel not found
	 */
	async deleteChannel(channelId) {
		// Verify channel exists
		const existingChannel = await this.prisma.notification_channels.findUnique({
			where: { id: channelId },
		});

		if (!existingChannel) {
			throw new Error(`Channel with ID ${channelId} not found`);
		}

		// Delete channel (cascading deletion will handle associated rules and history)
		const deletedChannel = await this.prisma.notification_channels.delete({
			where: { id: channelId },
		});

		return deletedChannel;
	}

	/**
	 * Get all notification channels
	 *
	 * @returns {Promise<Array>} Array of channel objects
	 */
	async getChannels() {
		const channels = await this.prisma.notification_channels.findMany({
			orderBy: { created_at: "desc" },
		});

		return channels;
	}

	/**
	 * Test a notification channel by sending a test message
	 *
	 * @param {string} channelId - Channel ID to test
	 * @returns {Promise<Object>} Test result with success status and message
	 * @throws {Error} If channel not found
	 */
	async testChannel(channelId) {
		// Get channel
		const channel = await this.prisma.notification_channels.findUnique({
			where: { id: channelId },
		});

		if (!channel) {
			throw new Error(`Channel with ID ${channelId} not found`);
		}

		// Send test message
		const testMessage = {
			title: "PatchMon Test Notification",
			message: `Test message from PatchMon at ${new Date().toISOString()}. This is a test notification.`,
			priority: channel.priority,
		};

		const sendResult = await this.gotifyService.sendMessage(
			channel.server_url,
			channel.token,
			testMessage,
		);

		// Update channel status based on test result
		if (sendResult.success) {
			await this.prisma.notification_channels.update({
				where: { id: channelId },
				data: {
					status: "connected",
					last_tested_at: new Date(),
					last_error: null,
				},
			});

			return {
				success: true,
				message: "Test message sent successfully",
				messageId: sendResult.messageId,
			};
		} else {
			await this.prisma.notification_channels.update({
				where: { id: channelId },
				data: {
					status: "disconnected",
					last_tested_at: new Date(),
					last_error: sendResult.error,
				},
			});

			return {
				success: false,
				message: `Test message failed: ${sendResult.error}`,
				error: sendResult.error,
			};
		}
	}

	/**
	 * Update channel status
	 *
	 * @param {string} channelId - Channel ID to update
	 * @param {string} status - New status ('connected' or 'disconnected')
	 * @param {string} [errorMessage] - Error message if status is disconnected
	 * @returns {Promise<Object>} Updated channel object
	 * @throws {Error} If channel not found or invalid status
	 */
	async updateChannelStatus(channelId, status, errorMessage = null) {
		// Verify channel exists
		const existingChannel = await this.prisma.notification_channels.findUnique({
			where: { id: channelId },
		});

		if (!existingChannel) {
			throw new Error(`Channel with ID ${channelId} not found`);
		}

		// Validate status
		if (!["connected", "disconnected"].includes(status)) {
			throw new Error('Status must be either "connected" or "disconnected"');
		}

		// Update status
		const updateData = {
			status,
			last_tested_at: new Date(),
		};

		if (errorMessage) {
			updateData.last_error = errorMessage;
		} else if (status === "connected") {
			updateData.last_error = null;
		}

		const updatedChannel = await this.prisma.notification_channels.update({
			where: { id: channelId },
			data: updateData,
		});

		return updatedChannel;
	}
}

module.exports = NotificationChannelService;
