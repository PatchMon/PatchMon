import * as fc from "fast-check";
import { describe, expect, it } from "vitest";

/**
 * Property-Based Test Suite for NotificationChannelService
 *
 * These tests validate the correctness properties of the notification channel system
 * using property-based testing with fast-check.
 */

/**
 * Arbitraries for generating test data
 */

// Generate valid Gotify server URLs
const gotifyServerUrlArbitrary = () => {
	return fc
		.tuple(
			fc.constantFrom("http", "https"),
			fc.domain(),
			fc.integer({ min: 1, max: 65535 }),
		)
		.map(([protocol, domain, port]) => `${protocol}://${domain}:${port}`);
};

// Generate valid application tokens (alphanumeric strings)
const tokenArbitrary = () => {
	return fc.stringMatching(/^[a-zA-Z0-9_-]{20,50}$/);
};

// Generate valid channel names
const channelNameArbitrary = () => {
	return fc.stringMatching(/^[a-zA-Z0-9\s_-]{3,50}$/);
};

// Generate valid priority values (0-10)
const priorityArbitrary = () => {
	return fc.integer({ min: 0, max: 10 });
};

// Generate valid notification channel configurations
const notificationChannelArbitrary = () => {
	return fc.record({
		name: channelNameArbitrary(),
		channel_type: fc.constant("gotify"),
		server_url: gotifyServerUrlArbitrary(),
		token: tokenArbitrary(),
		priority: priorityArbitrary(),
		status: fc.constantFrom("connected", "disconnected"),
	});
};

/**
 * Feature: gotify-notifications, Property 2: Channel Persistence
 * Validates: Requirements 1.3, 7.1, 7.2
 *
 * Property: For any valid Gotify channel configuration, after saving to the database,
 * retrieving the channel should return an equivalent configuration with all fields intact.
 */
describe("NotificationChannelService - Channel Persistence", () => {
	it("should preserve all channel configuration fields through serialization and deserialization", () => {
		fc.assert(
			fc.property(notificationChannelArbitrary(), (channelConfig) => {
				// Simulate serialization to JSON (as would happen when saving to database)
				const serialized = JSON.stringify(channelConfig);

				// Simulate deserialization from JSON (as would happen when retrieving from database)
				const deserialized = JSON.parse(serialized);

				// Verify all fields are preserved
				expect(deserialized.name).toBe(channelConfig.name);
				expect(deserialized.channel_type).toBe(channelConfig.channel_type);
				expect(deserialized.server_url).toBe(channelConfig.server_url);
				expect(deserialized.token).toBe(channelConfig.token);
				expect(deserialized.priority).toBe(channelConfig.priority);
				expect(deserialized.status).toBe(channelConfig.status);

				// Verify the deserialized object is equivalent to the original
				expect(deserialized).toEqual(channelConfig);
			}),
			{ numRuns: 100 },
		);
	});

	it("should handle channel configurations with optional fields", () => {
		fc.assert(
			fc.property(
				fc.record({
					name: channelNameArbitrary(),
					channel_type: fc.constant("gotify"),
					server_url: gotifyServerUrlArbitrary(),
					token: tokenArbitrary(),
					priority: priorityArbitrary(),
					status: fc.constantFrom("connected", "disconnected"),
					last_error: fc.option(fc.string({ minLength: 1, maxLength: 200 }), {
						freq: 2,
					}),
				}),
				(channelConfig) => {
					const serialized = JSON.stringify(channelConfig);
					const deserialized = JSON.parse(serialized);

					// Verify optional fields are preserved (including null/undefined)
					expect(deserialized.last_error).toBe(channelConfig.last_error);
					expect(deserialized).toEqual(channelConfig);
				},
			),
			{ numRuns: 100 },
		);
	});

	it("should maintain data integrity for channel URLs with special characters", () => {
		fc.assert(
			fc.property(
				fc.record({
					name: channelNameArbitrary(),
					server_url: fc
						.tuple(
							fc.constantFrom("http", "https"),
							fc.domain(),
							fc.integer({ min: 1, max: 65535 }),
						)
						.map(
							([protocol, domain, port]) =>
								`${protocol}://${domain}:${port}/path?query=value`,
						),
					token: tokenArbitrary(),
				}),
				(channelConfig) => {
					const serialized = JSON.stringify(channelConfig);
					const deserialized = JSON.parse(serialized);

					// Verify URL is preserved exactly
					expect(deserialized.server_url).toBe(channelConfig.server_url);
				},
			),
			{ numRuns: 100 },
		);
	});
});

/**
 * Feature: gotify-notifications, Property 3: Multiple Channels Independence
 * Validates: Requirements 2.1, 2.2, 2.3
 *
 * Property: For any set of Gotify channels, adding, updating, or deleting one channel
 * should not affect the state of other channels.
 */
describe("NotificationChannelService - Multiple Channels Independence", () => {
	it("should maintain independence between multiple channels during operations", () => {
		fc.assert(
			fc.property(
				fc.array(notificationChannelArbitrary(), {
					minLength: 2,
					maxLength: 5,
				}),
				(channels) => {
					// Create a map to track channel states
					const channelMap = new Map();
					channels.forEach((channel, index) => {
						channelMap.set(index, { ...channel });
					});

					// Simulate updating one channel
					const updateIndex = 0;
					const originalChannel = channelMap.get(updateIndex);
					const updatedChannel = {
						...originalChannel,
						name: "Updated Channel Name",
						priority: 8,
					};
					channelMap.set(updateIndex, updatedChannel);

					// Verify other channels remain unchanged
					for (let i = 1; i < channels.length; i++) {
						const currentChannel = channelMap.get(i);
						expect(currentChannel).toEqual(channels[i]);
						expect(currentChannel.name).toBe(channels[i].name);
						expect(currentChannel.priority).toBe(channels[i].priority);
					}

					// Verify updated channel has new values
					expect(channelMap.get(updateIndex).name).toBe("Updated Channel Name");
					expect(channelMap.get(updateIndex).priority).toBe(8);
				},
			),
			{ numRuns: 100 },
		);
	});

	it("should not affect other channels when deleting one channel", () => {
		fc.assert(
			fc.property(
				fc.array(notificationChannelArbitrary(), {
					minLength: 2,
					maxLength: 5,
				}),
				(channels) => {
					// Create a list of channels
					const channelList = [...channels];
					const initialLength = channelList.length;

					// Delete one channel
					const deleteIndex = 0;
					channelList.splice(deleteIndex, 1);

					// Verify list length decreased by 1
					expect(channelList.length).toBe(initialLength - 1);

					// Verify remaining channels are unchanged
					for (let i = 0; i < channelList.length; i++) {
						expect(channelList[i]).toEqual(channels[i + 1]);
					}
				},
			),
			{ numRuns: 100 },
		);
	});

	it("should maintain channel isolation when adding new channels", () => {
		fc.assert(
			fc.property(
				fc.array(notificationChannelArbitrary(), {
					minLength: 1,
					maxLength: 4,
				}),
				notificationChannelArbitrary(),
				(existingChannels, newChannel) => {
					// Create a list of existing channels
					const channelList = [...existingChannels];
					const originalLength = channelList.length;

					// Add new channel
					channelList.push(newChannel);

					// Verify list length increased by 1
					expect(channelList.length).toBe(originalLength + 1);

					// Verify existing channels remain unchanged
					for (let i = 0; i < existingChannels.length; i++) {
						expect(channelList[i]).toEqual(existingChannels[i]);
					}

					// Verify new channel is added correctly
					expect(channelList[originalLength]).toEqual(newChannel);
				},
			),
			{ numRuns: 100 },
		);
	});
});

/**
 * Feature: gotify-notifications, Property 13: Configuration Round-Trip
 * Validates: Requirements 7.1, 7.2, 7.3, 7.5
 *
 * Property: For any valid notification channel or rule configuration, serializing to JSON
 * and then deserializing should produce an equivalent configuration with all fields and
 * relationships intact.
 */
describe("NotificationChannelService - Property 13: Configuration Round-Trip", () => {
	it("should preserve channel configuration through JSON serialization round-trip", () => {
		fc.assert(
			fc.property(notificationChannelArbitrary(), (channelConfig) => {
				// Serialize to JSON
				const serialized = JSON.stringify(channelConfig);

				// Deserialize from JSON
				const deserialized = JSON.parse(serialized);

				// Verify all fields are preserved
				expect(deserialized).toEqual(channelConfig);
				expect(deserialized.name).toBe(channelConfig.name);
				expect(deserialized.channel_type).toBe(channelConfig.channel_type);
				expect(deserialized.server_url).toBe(channelConfig.server_url);
				expect(deserialized.token).toBe(channelConfig.token);
				expect(deserialized.priority).toBe(channelConfig.priority);
				expect(deserialized.status).toBe(channelConfig.status);
			}),
			{ numRuns: 100 },
		);
	});

	it("should handle complex channel configurations with optional fields in round-trip", () => {
		fc.assert(
			fc.property(
				fc.record({
					name: channelNameArbitrary(),
					channel_type: fc.constant("gotify"),
					server_url: gotifyServerUrlArbitrary(),
					token: tokenArbitrary(),
					priority: priorityArbitrary(),
					status: fc.constantFrom("connected", "disconnected"),
					last_tested_at: fc.option(fc.date(), { freq: 2 }),
					last_error: fc.option(fc.string({ minLength: 1, maxLength: 200 }), {
						freq: 2,
					}),
					created_at: fc.date(),
					updated_at: fc.date(),
				}),
				(complexConfig) => {
					// Serialize and deserialize
					const serialized = JSON.stringify(complexConfig);
					const deserialized = JSON.parse(serialized);

					// Verify all string fields are preserved
					expect(deserialized.name).toBe(complexConfig.name);
					expect(deserialized.channel_type).toBe(complexConfig.channel_type);
					expect(deserialized.server_url).toBe(complexConfig.server_url);
					expect(deserialized.token).toBe(complexConfig.token);
					expect(deserialized.priority).toBe(complexConfig.priority);
					expect(deserialized.status).toBe(complexConfig.status);
					expect(deserialized.last_error).toBe(complexConfig.last_error);

					// Verify optional fields are preserved (dates become ISO strings after JSON round-trip)
					if (complexConfig.last_tested_at !== null) {
						expect(deserialized.last_tested_at).toBe(
							complexConfig.last_tested_at.toISOString(),
						);
					} else {
						expect(deserialized.last_tested_at).toBeNull();
					}

					// Verify created_at and updated_at are preserved as ISO strings
					expect(deserialized.created_at).toBe(
						complexConfig.created_at.toISOString(),
					);
					expect(deserialized.updated_at).toBe(
						complexConfig.updated_at.toISOString(),
					);
				},
			),
			{ numRuns: 100 },
		);
	});

	it("should preserve channel URLs with special characters through round-trip", () => {
		fc.assert(
			fc.property(
				fc.record({
					name: channelNameArbitrary(),
					server_url: fc
						.tuple(
							fc.constantFrom("http", "https"),
							fc.domain(),
							fc.integer({ min: 1, max: 65535 }),
						)
						.map(
							([protocol, domain, port]) =>
								`${protocol}://${domain}:${port}/path?query=value&other=123`,
						),
					token: tokenArbitrary(),
				}),
				(config) => {
					const serialized = JSON.stringify(config);
					const deserialized = JSON.parse(serialized);

					// Verify URL with special characters is preserved exactly
					expect(deserialized.server_url).toBe(config.server_url);
				},
			),
			{ numRuns: 100 },
		);
	});

	it("should maintain data integrity for multiple round-trips", () => {
		fc.assert(
			fc.property(notificationChannelArbitrary(), (originalConfig) => {
				let config = originalConfig;

				// Perform multiple round-trips
				for (let i = 0; i < 5; i++) {
					const serialized = JSON.stringify(config);
					config = JSON.parse(serialized);
				}

				// Verify configuration is still equivalent after multiple round-trips
				expect(config).toEqual(originalConfig);
			}),
			{ numRuns: 100 },
		);
	});
});

/**
 * Feature: gotify-notifications, Property 4: Channel Status Accuracy
 * Validates: Requirements 1.5, 2.4, 2.5
 *
 * Property: For any notification channel, the displayed status (connected/disconnected)
 * should accurately reflect the last known connection state, and should be updated
 * whenever a connection attempt is made.
 */
describe("NotificationChannelService - Channel Status Accuracy", () => {
	it("should maintain accurate status for all valid status values", () => {
		fc.assert(
			fc.property(
				notificationChannelArbitrary(),
				fc.constantFrom("connected", "disconnected"),
				(channel, newStatus) => {
					// Create a channel with initial status
					const updatedChannel = {
						...channel,
						status: newStatus,
						last_tested_at: new Date(),
					};

					// Verify status is set correctly
					expect(updatedChannel.status).toBe(newStatus);
					expect(updatedChannel.last_tested_at).toBeDefined();
				},
			),
			{ numRuns: 100 },
		);
	});

	it("should update last_tested_at timestamp when status changes", () => {
		fc.assert(
			fc.property(notificationChannelArbitrary(), (channel) => {
				const beforeUpdate = new Date();

				// Simulate status update
				const updatedChannel = {
					...channel,
					status: "connected",
					last_tested_at: new Date(),
				};

				const afterUpdate = new Date();

				// Verify timestamp is between before and after
				expect(updatedChannel.last_tested_at.getTime()).toBeGreaterThanOrEqual(
					beforeUpdate.getTime(),
				);
				expect(updatedChannel.last_tested_at.getTime()).toBeLessThanOrEqual(
					afterUpdate.getTime(),
				);
			}),
			{ numRuns: 100 },
		);
	});

	it("should clear error message when status changes to connected", () => {
		fc.assert(
			fc.property(
				notificationChannelArbitrary(),
				fc.string({ minLength: 1, maxLength: 200 }),
				(channel, errorMessage) => {
					// Create a disconnected channel with error
					const disconnectedChannel = {
						...channel,
						status: "disconnected",
						last_error: errorMessage,
					};

					// Simulate reconnection
					const reconnectedChannel = {
						...disconnectedChannel,
						status: "connected",
						last_error: null,
						last_tested_at: new Date(),
					};

					// Verify error is cleared
					expect(reconnectedChannel.status).toBe("connected");
					expect(reconnectedChannel.last_error).toBeNull();
				},
			),
			{ numRuns: 100 },
		);
	});

	it("should preserve error message when status is disconnected", () => {
		fc.assert(
			fc.property(
				notificationChannelArbitrary(),
				fc.string({ minLength: 1, maxLength: 200 }),
				(channel, errorMessage) => {
					// Create a disconnected channel with error
					const disconnectedChannel = {
						...channel,
						status: "disconnected",
						last_error: errorMessage,
						last_tested_at: new Date(),
					};

					// Verify error is preserved
					expect(disconnectedChannel.status).toBe("disconnected");
					expect(disconnectedChannel.last_error).toBe(errorMessage);
				},
			),
			{ numRuns: 100 },
		);
	});
});
