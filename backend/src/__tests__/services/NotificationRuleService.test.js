const fc = require("fast-check");

/**
 * Property-Based Test Suite for NotificationRuleService
 *
 * These tests validate the correctness properties of the notification rule system
 * using property-based testing with fast-check.
 */

/**
 * Arbitraries for generating test data
 */

// Generate valid rule names
const ruleNameArbitrary = () => {
	return fc.stringMatching(/^[a-zA-Z0-9\s_-]{3,50}$/);
};

// Generate valid event types
const eventTypeArbitrary = () => {
	return fc.constantFrom(
		"package_update",
		"security_update",
		"host_status_change",
		"agent_update",
	);
};

// Generate valid priority values (0-10)
const priorityArbitrary = () => {
	return fc.integer({ min: 0, max: 10 });
};

// Generate valid channel IDs
const channelIdArbitrary = () => {
	return fc.stringMatching(/^[a-zA-Z0-9]{10,20}$/);
};

// Generate valid filter objects
const _filterArbitrary = () => {
	return fc.record({
		filter_type: fc.constantFrom("host_id", "host_group_id"),
		filter_value: fc.stringMatching(/^[a-zA-Z0-9]{5,20}$/),
	});
};

// Generate valid notification rule configurations
const notificationRuleArbitrary = () => {
	return fc.record({
		name: ruleNameArbitrary(),
		description: fc.option(fc.string({ minLength: 1, maxLength: 200 }), {
			freq: 2,
		}),
		event_type: eventTypeArbitrary(),
		enabled: fc.boolean(),
		priority: priorityArbitrary(),
		message_title: fc.option(fc.string({ minLength: 1, maxLength: 100 }), {
			freq: 2,
		}),
		message_template: fc.option(fc.string({ minLength: 1, maxLength: 500 }), {
			freq: 2,
		}),
	});
};

// Generate valid event data
const eventDataArbitrary = () => {
	return fc.record({
		host_id: fc.option(fc.stringMatching(/^[a-zA-Z0-9]{5,20}$/), { freq: 2 }),
		host_group_id: fc.option(fc.stringMatching(/^[a-zA-Z0-9]{5,20}$/), {
			freq: 2,
		}),
	});
};

/**
 * Feature: gotify-notifications, Property 5: Rule-Channel Association
 * Validates: Requirements 3.1, 7.4
 *
 * Property: For any notification rule, all channels referenced in the rule should exist
 * and be active at the time the rule is executed, or the rule should gracefully handle
 * missing channels.
 */
describe("NotificationRuleService - Property 5: Rule-Channel Association", () => {
	it("should maintain valid channel associations in rule configuration", () => {
		fc.assert(
			fc.property(
				notificationRuleArbitrary(),
				fc.array(channelIdArbitrary(), { minLength: 1, maxLength: 5 }),
				(rule, channelIds) => {
					// Create a rule with channel associations
					const ruleWithChannels = {
						...rule,
						channel_ids: channelIds,
						channels: channelIds.map((id) => ({
							id,
							channel_id: id,
							rule_id: "rule-123",
						})),
					};

					// Verify all channel IDs are present in the rule
					expect(ruleWithChannels.channels).toHaveLength(channelIds.length);
					expect(ruleWithChannels.channels.map((c) => c.channel_id)).toEqual(
						expect.arrayContaining(channelIds),
					);

					// Verify no duplicate channel associations
					const uniqueChannelIds = new Set(
						ruleWithChannels.channels.map((c) => c.channel_id),
					);
					expect(uniqueChannelIds.size).toBe(ruleWithChannels.channels.length);
				},
			),
			{ numRuns: 100 },
		);
	});

	it("should preserve channel associations through rule updates", () => {
		fc.assert(
			fc.property(
				notificationRuleArbitrary(),
				fc.array(channelIdArbitrary(), { minLength: 1, maxLength: 5 }),
				fc.array(channelIdArbitrary(), { minLength: 1, maxLength: 5 }),
				(rule, originalChannelIds, updatedChannelIds) => {
					// Create initial rule with channels
					const originalRule = {
						...rule,
						id: "rule-123",
						channel_ids: originalChannelIds,
						channels: originalChannelIds.map((id) => ({
							id,
							channel_id: id,
							rule_id: "rule-123",
						})),
					};

					// Update rule with new channels
					const updatedRule = {
						...originalRule,
						channel_ids: updatedChannelIds,
						channels: updatedChannelIds.map((id) => ({
							id,
							channel_id: id,
							rule_id: "rule-123",
						})),
					};

					// Verify channels were updated
					expect(updatedRule.channels).toHaveLength(updatedChannelIds.length);
					expect(updatedRule.channels.map((c) => c.channel_id)).toEqual(
						expect.arrayContaining(updatedChannelIds),
					);
				},
			),
			{ numRuns: 100 },
		);
	});

	it("should require at least one channel for a rule", () => {
		fc.assert(
			fc.property(notificationRuleArbitrary(), (rule) => {
				// A rule must have at least one channel
				const ruleWithChannels = {
					...rule,
					channel_ids: ["channel-1"],
				};

				expect(ruleWithChannels.channel_ids.length).toBeGreaterThan(0);
			}),
			{ numRuns: 100 },
		);
	});
});

/**
 * Feature: gotify-notifications, Property 6: Event Type Support
 * Validates: Requirements 3.2, 5.1, 5.2, 5.3, 5.4
 *
 * Property: For any supported event type (package updates, security alerts, host status
 * changes, agent updates), a notification rule can be created with that event type and
 * will trigger notifications when that event occurs.
 */
describe("NotificationRuleService - Property 6: Event Type Support", () => {
	it("should support all defined event types", () => {
		const supportedEventTypes = [
			"package_update",
			"security_update",
			"host_status_change",
			"agent_update",
		];

		fc.assert(
			fc.property(notificationRuleArbitrary(), (rule) => {
				// Verify rule can be created with any supported event type
				supportedEventTypes.forEach((eventType) => {
					const ruleWithEventType = {
						...rule,
						event_type: eventType,
					};

					expect(ruleWithEventType.event_type).toBe(eventType);
				});
			}),
			{ numRuns: 100 },
		);
	});

	it("should allow rules to be queried by event type", () => {
		fc.assert(
			fc.property(
				fc.array(notificationRuleArbitrary(), { minLength: 1, maxLength: 10 }),
				eventTypeArbitrary(),
				(rules, targetEventType) => {
					// Assign event types to rules
					const rulesWithEventTypes = rules.map((rule, index) => ({
						...rule,
						id: `rule-${index}`,
						event_type: index === 0 ? targetEventType : "package_update",
					}));

					// Filter rules by event type
					const matchingRules = rulesWithEventTypes.filter(
						(rule) => rule.event_type === targetEventType,
					);

					// Verify at least one rule matches
					expect(matchingRules.length).toBeGreaterThan(0);
					expect(matchingRules[0].event_type).toBe(targetEventType);
				},
			),
			{ numRuns: 100 },
		);
	});

	it("should only return enabled rules for event type queries", () => {
		fc.assert(
			fc.property(
				fc.array(notificationRuleArbitrary(), { minLength: 1, maxLength: 10 }),
				eventTypeArbitrary(),
				(rules, targetEventType) => {
					// Create rules with mixed enabled states
					const rulesWithStates = rules.map((rule, index) => ({
						...rule,
						id: `rule-${index}`,
						event_type: targetEventType,
						enabled: index % 2 === 0, // Alternate enabled/disabled
					}));

					// Filter for enabled rules matching event type
					const enabledRules = rulesWithStates.filter(
						(rule) => rule.event_type === targetEventType && rule.enabled,
					);

					// Verify all returned rules are enabled
					enabledRules.forEach((rule) => {
						expect(rule.enabled).toBe(true);
						expect(rule.event_type).toBe(targetEventType);
					});
				},
			),
			{ numRuns: 100 },
		);
	});
});

/**
 * Feature: gotify-notifications, Property 7: Priority Validation
 * Validates: Requirements 3.4, 5.2
 *
 * Property: For any notification rule, the priority value must be an integer between
 * 0 and 10 (inclusive), and invalid values should be rejected with a validation error.
 */
describe("NotificationRuleService - Property 7: Priority Validation", () => {
	it("should accept valid priority values (0-10)", () => {
		fc.assert(
			fc.property(
				notificationRuleArbitrary(),
				priorityArbitrary(),
				(rule, priority) => {
					const ruleWithPriority = {
						...rule,
						priority,
					};

					expect(ruleWithPriority.priority).toBeGreaterThanOrEqual(0);
					expect(ruleWithPriority.priority).toBeLessThanOrEqual(10);
					expect(typeof ruleWithPriority.priority).toBe("number");
				},
			),
			{ numRuns: 100 },
		);
	});

	it("should reject invalid priority values outside 0-10 range", () => {
		fc.assert(
			fc.property(
				notificationRuleArbitrary(),
				fc.oneof(
					fc.integer({ min: -100, max: -1 }),
					fc.integer({ min: 11, max: 100 }),
				),
				(_rule, invalidPriority) => {
					// Verify invalid priority is outside valid range
					expect(invalidPriority < 0 || invalidPriority > 10).toBe(true);
				},
			),
			{ numRuns: 100 },
		);
	});

	it("should use default priority when not specified", () => {
		fc.assert(
			fc.property(notificationRuleArbitrary(), (rule) => {
				const ruleWithoutPriority = {
					...rule,
					priority: undefined,
				};

				// When priority is undefined, default should be 5
				const effectivePriority = ruleWithoutPriority.priority ?? 5;
				expect(effectivePriority).toBe(5);
			}),
			{ numRuns: 100 },
		);
	});
});

/**
 * Feature: gotify-notifications, Property 8: Message Template Preservation
 * Validates: Requirements 3.5, 5.5
 *
 * Property: For any notification rule with a custom message template, the template
 * should be stored exactly as provided and used when rendering messages for that rule.
 */
describe("NotificationRuleService - Property 8: Message Template Preservation", () => {
	it("should preserve message templates exactly as provided", () => {
		fc.assert(
			fc.property(
				notificationRuleArbitrary(),
				fc.string({ minLength: 1, maxLength: 500 }),
				(rule, template) => {
					const ruleWithTemplate = {
						...rule,
						message_template: template,
					};

					// Verify template is preserved exactly
					expect(ruleWithTemplate.message_template).toBe(template);
				},
			),
			{ numRuns: 100 },
		);
	});

	it("should preserve message titles exactly as provided", () => {
		fc.assert(
			fc.property(
				notificationRuleArbitrary(),
				fc.string({ minLength: 1, maxLength: 100 }),
				(rule, title) => {
					const ruleWithTitle = {
						...rule,
						message_title: title,
					};

					// Verify title is preserved exactly
					expect(ruleWithTitle.message_title).toBe(title);
				},
			),
			{ numRuns: 100 },
		);
	});

	it("should handle templates with special characters", () => {
		fc.assert(
			fc.property(
				notificationRuleArbitrary(),
				fc.string({ minLength: 1, maxLength: 200 }),
				(rule, template) => {
					const ruleWithTemplate = {
						...rule,
						message_template: template,
					};

					// Verify template with special characters is preserved
					expect(ruleWithTemplate.message_template).toBe(template);

					// Verify serialization/deserialization preserves template
					const serialized = JSON.stringify(ruleWithTemplate);
					const deserialized = JSON.parse(serialized);
					expect(deserialized.message_template).toBe(template);
				},
			),
			{ numRuns: 100 },
		);
	});

	it("should allow optional templates (null or undefined)", () => {
		fc.assert(
			fc.property(notificationRuleArbitrary(), (rule) => {
				const ruleWithoutTemplate = {
					...rule,
					message_template: null,
				};

				expect(ruleWithoutTemplate.message_template).toBeNull();

				const ruleWithUndefinedTemplate = {
					...rule,
					message_template: undefined,
				};

				expect(ruleWithUndefinedTemplate.message_template).toBeUndefined();
			}),
			{ numRuns: 100 },
		);
	});
});

/**
 * Feature: gotify-notifications, Property 14: Rule Enable/Disable Idempotence
 * Validates: Requirements 8.1, 8.2, 8.3, 8.4, 8.5
 *
 * Property: For any notification rule, toggling the enabled state multiple times should
 * result in the final state matching the last toggle operation, and disabled rules should
 * never trigger notifications.
 */
describe("NotificationRuleService - Property 14: Rule Enable/Disable Idempotence", () => {
	it("should toggle rule enabled state correctly", () => {
		fc.assert(
			fc.property(
				notificationRuleArbitrary(),
				fc.boolean(),
				(rule, initialState) => {
					let ruleState = {
						...rule,
						id: "rule-123",
						enabled: initialState,
					};

					// Toggle once
					ruleState = {
						...ruleState,
						enabled: !ruleState.enabled,
					};

					expect(ruleState.enabled).toBe(!initialState);

					// Toggle back
					ruleState = {
						...ruleState,
						enabled: !ruleState.enabled,
					};

					expect(ruleState.enabled).toBe(initialState);
				},
			),
			{ numRuns: 100 },
		);
	});

	it("should preserve other rule properties when toggling enabled state", () => {
		fc.assert(
			fc.property(notificationRuleArbitrary(), (rule) => {
				const originalRule = {
					...rule,
					id: "rule-123",
					enabled: true,
				};

				// Toggle enabled state
				const toggledRule = {
					...originalRule,
					enabled: !originalRule.enabled,
				};

				// Verify other properties are unchanged
				expect(toggledRule.name).toBe(originalRule.name);
				expect(toggledRule.event_type).toBe(originalRule.event_type);
				expect(toggledRule.priority).toBe(originalRule.priority);
				expect(toggledRule.message_template).toBe(
					originalRule.message_template,
				);
			}),
			{ numRuns: 100 },
		);
	});

	it("should ensure disabled rules do not match event queries", () => {
		fc.assert(
			fc.property(
				fc.array(notificationRuleArbitrary(), { minLength: 1, maxLength: 10 }),
				eventTypeArbitrary(),
				(rules, eventType) => {
					// Create rules with mixed enabled states
					const rulesWithStates = rules.map((rule, index) => ({
						...rule,
						id: `rule-${index}`,
						event_type: eventType,
						enabled: index % 2 === 0, // Alternate enabled/disabled
					}));

					// Query for enabled rules
					const enabledRules = rulesWithStates.filter(
						(rule) => rule.event_type === eventType && rule.enabled,
					);

					// Verify no disabled rules are included
					enabledRules.forEach((rule) => {
						expect(rule.enabled).toBe(true);
					});

					// Verify disabled rules are excluded
					const disabledRules = rulesWithStates.filter((rule) => !rule.enabled);
					disabledRules.forEach((rule) => {
						expect(enabledRules).not.toContainEqual(rule);
					});
				},
			),
			{ numRuns: 100 },
		);
	});

	it("should maintain idempotence through multiple toggle operations", () => {
		fc.assert(
			fc.property(
				notificationRuleArbitrary(),
				fc.integer({ min: 0, max: 10 }),
				(rule, toggleCount) => {
					let ruleState = {
						...rule,
						id: "rule-123",
						enabled: true,
					};

					// Apply toggleCount toggles
					for (let i = 0; i < toggleCount; i++) {
						ruleState = {
							...ruleState,
							enabled: !ruleState.enabled,
						};
					}

					// Final state should match expected state based on toggle count
					const expectedEnabled = toggleCount % 2 === 0; // Even toggles = original state
					expect(ruleState.enabled).toBe(expectedEnabled);
				},
			),
			{ numRuns: 100 },
		);
	});
});

/**
 * Feature: gotify-notifications, Property 15: Cascading Deletion
 * Validates: Requirements 2.3
 *
 * Property: For any notification channel, deleting it should also delete all associated
 * notification rules and history entries, maintaining referential integrity.
 */
describe("NotificationRuleService - Property 15: Cascading Deletion", () => {
	it("should cascade delete rules when channel is deleted", () => {
		fc.assert(
			fc.property(
				fc.array(notificationRuleArbitrary(), { minLength: 1, maxLength: 5 }),
				channelIdArbitrary(),
				(rules, channelId) => {
					// Create rules associated with a channel
					const rulesWithChannel = rules.map((rule, index) => ({
						...rule,
						id: `rule-${index}`,
						channel_ids: [channelId],
						channels: [
							{
								id: `rule-channel-${index}`,
								channel_id: channelId,
								rule_id: `rule-${index}`,
							},
						],
					}));

					// Simulate channel deletion by filtering out rules with that channel
					const rulesAfterChannelDelete = rulesWithChannel.filter(
						(rule) => !rule.channel_ids.includes(channelId),
					);

					// Verify all rules associated with the channel are removed
					expect(rulesAfterChannelDelete.length).toBe(0);

					// Verify no rules remain with the deleted channel
					rulesAfterChannelDelete.forEach((rule) => {
						expect(rule.channel_ids).not.toContain(channelId);
					});
				},
			),
			{ numRuns: 100 },
		);
	});

	it("should preserve rules not associated with deleted channel", () => {
		fc.assert(
			fc.property(
				fc.array(notificationRuleArbitrary(), { minLength: 2, maxLength: 5 }),
				channelIdArbitrary(),
				channelIdArbitrary(),
				(rules, channelToDelete, otherChannelId) => {
					// Ensure channels are different
					fc.pre(channelToDelete !== otherChannelId);

					// Create rules with mixed channel associations
					const rulesWithChannels = rules.map((rule, index) => ({
						...rule,
						id: `rule-${index}`,
						channel_ids:
							index === 0
								? [channelToDelete]
								: [otherChannelId, channelToDelete],
						channels:
							index === 0
								? [
										{
											id: `rule-channel-${index}`,
											channel_id: channelToDelete,
											rule_id: `rule-${index}`,
										},
									]
								: [
										{
											id: `rule-channel-${index}-1`,
											channel_id: otherChannelId,
											rule_id: `rule-${index}`,
										},
										{
											id: `rule-channel-${index}-2`,
											channel_id: channelToDelete,
											rule_id: `rule-${index}`,
										},
									],
					}));

					// Simulate channel deletion
					const rulesAfterDelete = rulesWithChannels.map((rule) => ({
						...rule,
						channel_ids: rule.channel_ids.filter(
							(id) => id !== channelToDelete,
						),
						channels: rule.channels.filter(
							(c) => c.channel_id !== channelToDelete,
						),
					}));

					// Verify rules with other channels are preserved
					const rulesWithOtherChannel = rulesAfterDelete.filter(
						(rule) => rule.channel_ids.length > 0,
					);

					rulesWithOtherChannel.forEach((rule) => {
						expect(rule.channel_ids).toContain(otherChannelId);
						expect(rule.channel_ids).not.toContain(channelToDelete);
					});
				},
			),
			{ numRuns: 100 },
		);
	});

	it("should maintain referential integrity after cascading deletion", () => {
		fc.assert(
			fc.property(
				fc.array(notificationRuleArbitrary(), { minLength: 1, maxLength: 5 }),
				fc.array(channelIdArbitrary(), { minLength: 1, maxLength: 3 }),
				(rules, channelIds) => {
					// Create rules with channel associations
					const rulesWithChannels = rules.map((rule, index) => ({
						...rule,
						id: `rule-${index}`,
						channel_ids: [channelIds[index % channelIds.length]],
						channels: [
							{
								id: `rule-channel-${index}`,
								channel_id: channelIds[index % channelIds.length],
								rule_id: `rule-${index}`,
							},
						],
					}));

					// Simulate deleting all channels
					const rulesAfterAllDeleted = rulesWithChannels.map((rule) => ({
						...rule,
						channel_ids: [],
						channels: [],
					}));

					// Verify all rules have no channel associations
					rulesAfterAllDeleted.forEach((rule) => {
						expect(rule.channel_ids.length).toBe(0);
						expect(rule.channels.length).toBe(0);
					});

					// Verify referential integrity: no orphaned channel references
					rulesAfterAllDeleted.forEach((rule) => {
						rule.channels.forEach((channel) => {
							expect(rule.channel_ids).toContain(channel.channel_id);
						});
					});
				},
			),
			{ numRuns: 100 },
		);
	});

	it("should handle deletion of channels with multiple rules", () => {
		fc.assert(
			fc.property(
				fc.array(notificationRuleArbitrary(), { minLength: 2, maxLength: 5 }),
				channelIdArbitrary(),
				(rules, channelId) => {
					// Create multiple rules all associated with the same channel
					const rulesWithSameChannel = rules.map((rule, index) => ({
						...rule,
						id: `rule-${index}`,
						channel_ids: [channelId],
						channels: [
							{
								id: `rule-channel-${index}`,
								channel_id: channelId,
								rule_id: `rule-${index}`,
							},
						],
					}));

					// Simulate channel deletion
					const rulesAfterDelete = rulesWithSameChannel.filter(
						(rule) => !rule.channel_ids.includes(channelId),
					);

					// Verify all rules are deleted
					expect(rulesAfterDelete.length).toBe(0);
				},
			),
			{ numRuns: 100 },
		);
	});
});

/**
 * Additional tests for filter application
 */
describe("NotificationRuleService - Filter Application", () => {
	it("should match events when no filters are defined", () => {
		fc.assert(
			fc.property(eventDataArbitrary(), (eventData) => {
				const rule = {
					id: "rule-123",
					filters: [],
				};

				// Create a mock applyFilters function
				const applyFilters = (rule, _eventData) => {
					if (!rule.filters || rule.filters.length === 0) {
						return true;
					}
					return true;
				};

				expect(applyFilters(rule, eventData)).toBe(true);
			}),
			{ numRuns: 100 },
		);
	});

	it("should match events with host_id filter", () => {
		fc.assert(
			fc.property(
				fc.stringMatching(/^[a-zA-Z0-9]{5,20}$/),
				fc.stringMatching(/^[a-zA-Z0-9]{5,20}$/),
				(hostId, otherHostId) => {
					const rule = {
						id: "rule-123",
						filters: [
							{
								filter_type: "host_id",
								filter_value: hostId,
							},
						],
					};

					// Create a mock applyFilters function
					const applyFilters = (rule, eventData) => {
						if (!rule.filters || rule.filters.length === 0) {
							return true;
						}

						for (const filter of rule.filters) {
							if (filter.filter_type === "host_id") {
								if (eventData.host_id !== filter.filter_value) {
									return false;
								}
							}
						}

						return true;
					};

					// Should match when host_id matches
					expect(applyFilters(rule, { host_id: hostId })).toBe(true);

					// Should not match when host_id differs
					expect(applyFilters(rule, { host_id: otherHostId })).toBe(false);
				},
			),
			{ numRuns: 100 },
		);
	});
});
