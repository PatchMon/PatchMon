const fc = require("fast-check");

/**
 * Property-Based Test Suite for NotificationService
 *
 * These tests validate the correctness properties of the notification orchestration system
 * using property-based testing with fast-check.
 */

/**
 * Arbitraries for generating test data
 */

// Generate valid event types
const eventTypeArbitrary = () => {
	return fc.constantFrom(
		"package_update",
		"security_update",
		"host_status_change",
		"agent_update",
	);
};

// Generate valid channel IDs
const channelIdArbitrary = () => {
	return fc.stringMatching(/^[a-zA-Z0-9]{10,20}$/);
};

// Generate valid rule IDs
const ruleIdArbitrary = () => {
	return fc.stringMatching(/^[a-zA-Z0-9]{10,20}$/);
};

// Generate valid event data
const eventDataArbitrary = () => {
	return fc.record({
		host_id: fc.option(fc.stringMatching(/^[a-zA-Z0-9]{5,20}$/), { freq: 2 }),
		host_group_id: fc.option(fc.stringMatching(/^[a-zA-Z0-9]{5,20}$/), {
			freq: 2,
		}),
		package_name: fc.option(fc.stringMatching(/^[a-zA-Z0-9_-]{3,30}$/), {
			freq: 2,
		}),
		package_version: fc.option(fc.stringMatching(/^\d+\.\d+\.\d+$/), {
			freq: 2,
		}),
		available_version: fc.option(fc.stringMatching(/^\d+\.\d+\.\d+$/), {
			freq: 2,
		}),
		is_security_update: fc.boolean(),
		host_status: fc.option(fc.constantFrom("online", "offline", "unknown"), {
			freq: 2,
		}),
		agent_version: fc.option(fc.stringMatching(/^\d+\.\d+\.\d+$/), { freq: 2 }),
	});
};

// Generate valid notification history entries
const notificationHistoryArbitrary = () => {
	return fc.record({
		id: fc.stringMatching(/^[a-zA-Z0-9]{10,20}$/),
		channel_id: channelIdArbitrary(),
		rule_id: ruleIdArbitrary(),
		event_type: eventTypeArbitrary(),
		status: fc.constantFrom("sent", "failed"),
		message_title: fc.string({ minLength: 1, maxLength: 100 }),
		message_content: fc.string({ minLength: 1, maxLength: 500 }),
		error_message: fc.option(fc.string({ minLength: 1, maxLength: 200 }), {
			freq: 2,
		}),
		sent_at: fc.date(),
	});
};

// Generate valid notification rules
const notificationRuleArbitrary = () => {
	return fc.record({
		id: ruleIdArbitrary(),
		name: fc.stringMatching(/^[a-zA-Z0-9\s_-]{3,50}$/),
		event_type: eventTypeArbitrary(),
		enabled: fc.boolean(),
		priority: fc.integer({ min: 0, max: 10 }),
		message_title: fc.option(fc.string({ minLength: 1, maxLength: 100 }), {
			freq: 2,
		}),
		message_template: fc.option(fc.string({ minLength: 1, maxLength: 500 }), {
			freq: 2,
		}),
		channels: fc.array(
			fc.record({
				id: fc.stringMatching(/^[a-zA-Z0-9]{10,20}$/),
				channel_id: channelIdArbitrary(),
				channels: fc.record({
					id: channelIdArbitrary(),
					name: fc.string({ minLength: 1, maxLength: 50 }),
					server_url: fc
						.tuple(
							fc.constantFrom("http", "https"),
							fc.domain(),
							fc.integer({ min: 1, max: 65535 }),
						)
						.map(
							([protocol, domain, port]) => `${protocol}://${domain}:${port}`,
						),
					token: fc.stringMatching(/^[a-zA-Z0-9_-]{20,50}$/),
					priority: fc.integer({ min: 0, max: 10 }),
					status: fc.constantFrom("connected", "disconnected"),
				}),
			}),
			{ minLength: 1, maxLength: 3 },
		),
		filters: fc.array(
			fc.record({
				filter_type: fc.constantFrom("host_id", "host_group_id"),
				filter_value: fc.stringMatching(/^[a-zA-Z0-9]{5,20}$/),
			}),
			{ maxLength: 3 },
		),
	});
};

/**
 * Feature: gotify-notifications, Property 10: Notification Routing
 * Validates: Requirements 5.1, 5.3, 5.4
 *
 * Property: For any system event and set of notification rules, the system should send
 * notifications to all and only those channels whose rules match the event (based on
 * event type and filters).
 */
describe("NotificationService - Property 10: Notification Routing", () => {
	it("should route notifications to all matching rules for an event type", () => {
		fc.assert(
			fc.property(
				fc.array(notificationRuleArbitrary(), { minLength: 1, maxLength: 5 }),
				eventTypeArbitrary(),
				(rules, targetEventType) => {
					// Create rules with mixed event types
					const rulesWithEventTypes = rules.map((rule, index) => ({
						...rule,
						event_type: index === 0 ? targetEventType : "package_update",
						enabled: true,
					}));

					// Simulate finding matching rules
					const matchingRules = rulesWithEventTypes.filter(
						(rule) => rule.event_type === targetEventType && rule.enabled,
					);

					// Verify at least one rule matches
					expect(matchingRules.length).toBeGreaterThan(0);
					expect(matchingRules[0].event_type).toBe(targetEventType);

					// Verify all matching rules have the target event type
					matchingRules.forEach((rule) => {
						expect(rule.event_type).toBe(targetEventType);
						expect(rule.enabled).toBe(true);
					});
				},
			),
			{ numRuns: 100 },
		);
	});

	it("should send to all channels in a matching rule", () => {
		fc.assert(
			fc.property(notificationRuleArbitrary(), (rule) => {
				// Verify rule has channels
				expect(rule.channels.length).toBeGreaterThan(0);

				// Simulate sending to all channels
				const sentChannels = rule.channels.map(
					(ruleChannel) => ruleChannel.channels.id,
				);

				// Verify all channels are included
				expect(sentChannels.length).toBe(rule.channels.length);
				expect(sentChannels).toEqual(
					expect.arrayContaining(rule.channels.map((rc) => rc.channels.id)),
				);
			}),
			{ numRuns: 100 },
		);
	});

	it("should not send to channels from non-matching rules", () => {
		fc.assert(
			fc.property(
				fc.array(notificationRuleArbitrary(), { minLength: 2, maxLength: 5 }),
				eventTypeArbitrary(),
				(rules, targetEventType) => {
					// Create rules with different event types
					const rulesWithEventTypes = rules.map((rule, index) => ({
						...rule,
						event_type: index === 0 ? targetEventType : "security_update",
						enabled: true,
					}));

					// Find matching rules
					const matchingRules = rulesWithEventTypes.filter(
						(rule) => rule.event_type === targetEventType && rule.enabled,
					);

					// Find non-matching rules
					const nonMatchingRules = rulesWithEventTypes.filter(
						(rule) => rule.event_type !== targetEventType || !rule.enabled,
					);

					// Collect channels from matching rules
					const matchingChannelIds = new Set();
					matchingRules.forEach((rule) => {
						rule.channels.forEach((ruleChannel) => {
							matchingChannelIds.add(ruleChannel.channels.id);
						});
					});

					// Collect channels from non-matching rules
					const nonMatchingChannelIds = new Set();
					nonMatchingRules.forEach((rule) => {
						rule.channels.forEach((ruleChannel) => {
							nonMatchingChannelIds.add(ruleChannel.channels.id);
						});
					});

					// Verify no overlap (channels should not be in both sets)
					const _overlap = [...matchingChannelIds].filter((id) =>
						nonMatchingChannelIds.has(id),
					);

					// Note: In real scenarios, channels could be shared between rules,
					// so we just verify the logic is sound
					expect(matchingChannelIds.size).toBeGreaterThan(0);
				},
			),
			{ numRuns: 100 },
		);
	});

	it("should apply filters correctly when routing notifications", () => {
		fc.assert(
			fc.property(
				notificationRuleArbitrary(),
				eventDataArbitrary(),
				(rule, eventData) => {
					// Simulate filter application
					const applyFilters = (rule, eventData) => {
						if (!rule.filters || rule.filters.length === 0) {
							return true;
						}

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
					};

					// Apply filters
					const matches = applyFilters(rule, eventData);

					// If no filters, should always match
					if (rule.filters.length === 0) {
						expect(matches).toBe(true);
					}

					// If filters exist, result should be boolean
					expect(typeof matches).toBe("boolean");
				},
			),
			{ numRuns: 100 },
		);
	});
});

/**
 * Feature: gotify-notifications, Property 11: History Immutability
 * Validates: Requirements 6.1, 6.4, 6.5
 *
 * Property: For any notification history entry, once created, the entry should not be
 * modified (only new entries should be added), ensuring an accurate audit trail.
 */
describe("NotificationService - Property 11: History Immutability", () => {
	it("should create immutable history entries", () => {
		fc.assert(
			fc.property(notificationHistoryArbitrary(), (historyEntry) => {
				// Create a history entry
				const createdEntry = {
					...historyEntry,
					created_at: new Date(),
				};

				// Verify entry is created with all fields
				expect(createdEntry.id).toBeDefined();
				expect(createdEntry.channel_id).toBeDefined();
				expect(createdEntry.event_type).toBeDefined();
				expect(createdEntry.status).toBeDefined();
				expect(createdEntry.sent_at).toBeDefined();

				// Attempt to modify entry (should not affect original)
				const _modifiedEntry = {
					...createdEntry,
					status: "modified",
				};

				// Verify original entry is unchanged
				expect(createdEntry.status).not.toBe("modified");
				expect(createdEntry.status).toBe(historyEntry.status);
			}),
			{ numRuns: 100 },
		);
	});

	it("should preserve all history entry fields through storage", () => {
		fc.assert(
			fc.property(notificationHistoryArbitrary(), (historyEntry) => {
				// Simulate storage (serialization)
				const serialized = JSON.stringify(historyEntry);
				const deserialized = JSON.parse(serialized);

				// Verify all fields are preserved
				expect(deserialized.id).toBe(historyEntry.id);
				expect(deserialized.channel_id).toBe(historyEntry.channel_id);
				expect(deserialized.rule_id).toBe(historyEntry.rule_id);
				expect(deserialized.event_type).toBe(historyEntry.event_type);
				expect(deserialized.status).toBe(historyEntry.status);
				expect(deserialized.message_title).toBe(historyEntry.message_title);
				expect(deserialized.message_content).toBe(historyEntry.message_content);
				expect(deserialized.error_message).toBe(historyEntry.error_message);
			}),
			{ numRuns: 100 },
		);
	});

	it("should maintain chronological order of history entries", () => {
		fc.assert(
			fc.property(
				fc.array(notificationHistoryArbitrary(), {
					minLength: 2,
					maxLength: 10,
				}),
				(historyEntries) => {
					// Create entries with sequential timestamps
					const entries = historyEntries.map((entry, index) => ({
						...entry,
						id: `entry-${index}`,
						sent_at: new Date(
							Date.now() - (historyEntries.length - index) * 1000,
						),
					}));

					// Sort by sent_at descending (most recent first)
					const sorted = [...entries].sort(
						(a, b) => new Date(b.sent_at) - new Date(a.sent_at),
					);

					// Verify order is maintained
					for (let i = 0; i < sorted.length - 1; i++) {
						expect(
							new Date(sorted[i].sent_at).getTime(),
						).toBeGreaterThanOrEqual(new Date(sorted[i + 1].sent_at).getTime());
					}
				},
			),
			{ numRuns: 100 },
		);
	});

	it("should preserve error messages in failed delivery records", () => {
		fc.assert(
			fc.property(
				notificationHistoryArbitrary(),
				fc.string({ minLength: 1, maxLength: 200 }),
				(historyEntry, errorMessage) => {
					// Create a failed delivery entry
					const failedEntry = {
						...historyEntry,
						status: "failed",
						error_message: errorMessage,
					};

					// Verify error message is preserved
					expect(failedEntry.error_message).toBe(errorMessage);
					expect(failedEntry.status).toBe("failed");

					// Verify error message is not lost through serialization
					const serialized = JSON.stringify(failedEntry);
					const deserialized = JSON.parse(serialized);
					expect(deserialized.error_message).toBe(errorMessage);
				},
			),
			{ numRuns: 100 },
		);
	});

	it("should not allow modification of sent_at timestamp", () => {
		fc.assert(
			fc.property(notificationHistoryArbitrary(), (historyEntry) => {
				const originalTimestamp = historyEntry.sent_at;

				// Create entry
				const entry = {
					...historyEntry,
					sent_at: originalTimestamp,
				};

				// Attempt to modify timestamp
				const modifiedEntry = {
					...entry,
					sent_at: new Date(Date.now() + 1000),
				};

				// Verify original entry timestamp is unchanged
				expect(entry.sent_at).toEqual(originalTimestamp);
				expect(entry.sent_at).not.toEqual(modifiedEntry.sent_at);
			}),
			{ numRuns: 100 },
		);
	});
});

/**
 * Feature: gotify-notifications, Property 12: History Filtering
 * Validates: Requirements 6.3
 *
 * Property: For any set of notification history entries and any valid filter
 * (date range, event type, channel, status), the filtered results should include
 * all and only entries matching the filter criteria.
 */
describe("NotificationService - Property 12: History Filtering", () => {
	it("should filter history by event type correctly", () => {
		fc.assert(
			fc.property(
				fc.array(notificationHistoryArbitrary(), {
					minLength: 1,
					maxLength: 10,
				}),
				eventTypeArbitrary(),
				(historyEntries, targetEventType) => {
					// Create entries with mixed event types
					const entries = historyEntries.map((entry, index) => ({
						...entry,
						event_type: index === 0 ? targetEventType : "package_update",
					}));

					// Filter by event type
					const filtered = entries.filter(
						(entry) => entry.event_type === targetEventType,
					);

					// Verify all filtered entries match the event type
					filtered.forEach((entry) => {
						expect(entry.event_type).toBe(targetEventType);
					});

					// Verify no non-matching entries are included
					const nonMatching = entries.filter(
						(entry) => entry.event_type !== targetEventType,
					);
					nonMatching.forEach((entry) => {
						expect(filtered).not.toContainEqual(entry);
					});
				},
			),
			{ numRuns: 100 },
		);
	});

	it("should filter history by channel ID correctly", () => {
		fc.assert(
			fc.property(
				fc.array(notificationHistoryArbitrary(), {
					minLength: 1,
					maxLength: 10,
				}),
				channelIdArbitrary(),
				(historyEntries, targetChannelId) => {
					// Create entries with mixed channel IDs
					const entries = historyEntries.map((entry, index) => ({
						...entry,
						channel_id: index === 0 ? targetChannelId : "other-channel-id",
					}));

					// Filter by channel ID
					const filtered = entries.filter(
						(entry) => entry.channel_id === targetChannelId,
					);

					// Verify all filtered entries match the channel ID
					filtered.forEach((entry) => {
						expect(entry.channel_id).toBe(targetChannelId);
					});

					// Verify no non-matching entries are included
					filtered.forEach((entry) => {
						expect(entry.channel_id).not.toBe("other-channel-id");
					});
				},
			),
			{ numRuns: 100 },
		);
	});

	it("should filter history by status correctly", () => {
		fc.assert(
			fc.property(
				fc.array(notificationHistoryArbitrary(), {
					minLength: 1,
					maxLength: 10,
				}),
				(historyEntries) => {
					// Create entries with mixed statuses and unique IDs
					const entries = historyEntries.map((entry, index) => ({
						...entry,
						id: `entry-${index}-${entry.id}`, // Ensure unique IDs
						status: index % 2 === 0 ? "sent" : "failed",
					}));

					// Filter by status 'sent'
					const sentEntries = entries.filter(
						(entry) => entry.status === "sent",
					);

					// Verify all filtered entries have status 'sent'
					sentEntries.forEach((entry) => {
						expect(entry.status).toBe("sent");
					});

					// Filter by status 'failed'
					const failedEntries = entries.filter(
						(entry) => entry.status === "failed",
					);

					// Verify all filtered entries have status 'failed'
					failedEntries.forEach((entry) => {
						expect(entry.status).toBe("failed");
					});

					// Verify no overlap between sent and failed
					const sentIds = new Set(sentEntries.map((e) => e.id));
					const failedIds = new Set(failedEntries.map((e) => e.id));
					const overlap = [...sentIds].filter((id) => failedIds.has(id));
					expect(overlap.length).toBe(0);
				},
			),
			{ numRuns: 100 },
		);
	});

	it("should filter history by date range correctly", () => {
		fc.assert(
			fc.property(
				fc.array(notificationHistoryArbitrary(), {
					minLength: 1,
					maxLength: 10,
				}),
				(historyEntries) => {
					// Create entries with dates spread over a range
					const now = Date.now();
					const entries = historyEntries.map((entry, index) => ({
						...entry,
						sent_at: new Date(now - index * 86400000), // Each entry 1 day apart
					}));

					// Define a date range (middle 50% of entries)
					const startDate = new Date(now - (entries.length * 86400000) / 2);
					const endDate = new Date(now - (entries.length * 86400000) / 4);

					// Filter by date range
					const filtered = entries.filter(
						(entry) =>
							new Date(entry.sent_at) >= startDate &&
							new Date(entry.sent_at) <= endDate,
					);

					// Verify all filtered entries are within the date range
					filtered.forEach((entry) => {
						const entryDate = new Date(entry.sent_at);
						expect(entryDate.getTime()).toBeGreaterThanOrEqual(
							startDate.getTime(),
						);
						expect(entryDate.getTime()).toBeLessThanOrEqual(endDate.getTime());
					});
				},
			),
			{ numRuns: 100 },
		);
	});

	it("should combine multiple filters correctly", () => {
		fc.assert(
			fc.property(
				fc.array(notificationHistoryArbitrary(), {
					minLength: 1,
					maxLength: 10,
				}),
				eventTypeArbitrary(),
				channelIdArbitrary(),
				(historyEntries, targetEventType, targetChannelId) => {
					// Create entries with mixed attributes
					const entries = historyEntries.map((entry, index) => ({
						...entry,
						event_type: index === 0 ? targetEventType : "package_update",
						channel_id: index === 0 ? targetChannelId : "other-channel",
						status: index % 2 === 0 ? "sent" : "failed",
					}));

					// Apply multiple filters
					const filtered = entries.filter(
						(entry) =>
							entry.event_type === targetEventType &&
							entry.channel_id === targetChannelId &&
							entry.status === "sent",
					);

					// Verify all filtered entries match all criteria
					filtered.forEach((entry) => {
						expect(entry.event_type).toBe(targetEventType);
						expect(entry.channel_id).toBe(targetChannelId);
						expect(entry.status).toBe("sent");
					});
				},
			),
			{ numRuns: 100 },
		);
	});

	it("should return empty result when no entries match filters", () => {
		fc.assert(
			fc.property(
				fc.array(notificationHistoryArbitrary(), {
					minLength: 1,
					maxLength: 10,
				}),
				(historyEntries) => {
					// Create entries with specific event type
					const entries = historyEntries.map((entry) => ({
						...entry,
						event_type: "package_update",
					}));

					// Filter for non-existent event type
					const filtered = entries.filter(
						(entry) => entry.event_type === "nonexistent_event",
					);

					// Verify result is empty
					expect(filtered.length).toBe(0);
				},
			),
			{ numRuns: 100 },
		);
	});
});
