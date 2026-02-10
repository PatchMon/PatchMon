const axios = require("axios");
const fc = require("fast-check");
const GotifyService = require("../../services/GotifyService");

// Mock axios
jest.mock("axios");

/**
 * Property-Based Test Suite for GotifyService
 *
 * These tests validate the correctness properties of the Gotify API integration
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

describe("GotifyService", () => {
	let service;

	beforeEach(() => {
		service = new GotifyService();
		jest.clearAllMocks();
	});

	/**
	 * Feature: gotify-notifications, Property 1: Channel Configuration Validation
	 * Validates: Requirements 1.2, 1.4
	 *
	 * Property: For any Gotify server URL and application token, attempting to validate
	 * the connection should either succeed with a valid response or fail with a descriptive
	 * error message, never silently fail or return ambiguous results.
	 */
	describe("validateConnection - Property 1: Channel Configuration Validation", () => {
		it("should always return a valid response structure with either valid:true or valid:false", async () => {
			// Mock axios to always reject (simulating network error)
			axios.get.mockRejectedValue(new Error("Network error"));

			await fc.assert(
				fc.asyncProperty(
					gotifyServerUrlArbitrary(),
					tokenArbitrary(),
					async (url, token) => {
						const result = await service.validateConnection(url, token);

						// Result must have valid property
						expect(result).toHaveProperty("valid");
						expect(typeof result.valid).toBe("boolean");

						// If valid is false, must have error message
						if (!result.valid) {
							expect(result).toHaveProperty("error");
							expect(typeof result.error).toBe("string");
							expect(result.error.length).toBeGreaterThan(0);
						}

						// If valid is true, should not have error
						if (result.valid) {
							expect(result.error).toBeUndefined();
						}
					},
				),
				{ numRuns: 20 },
			);
		}, 15000);

		it("should reject invalid URL formats with descriptive error", async () => {
			await fc.assert(
				fc.asyncProperty(
					fc.stringMatching(/^[a-zA-Z0-9]{5,20}$/), // Invalid URL format
					tokenArbitrary(),
					async (invalidUrl, token) => {
						const result = await service.validateConnection(invalidUrl, token);

						expect(result.valid).toBe(false);
						expect(result.error).toBeDefined();
						expect(result.error).toContain("Invalid");
					},
				),
				{ numRuns: 20 },
			);
		});

		it("should reject empty or invalid tokens with descriptive error", async () => {
			await fc.assert(
				fc.asyncProperty(
					gotifyServerUrlArbitrary(),
					fc.oneof(
						fc.constant(""),
						fc.constant(null),
						fc.constant("   "), // whitespace only
					),
					async (url, invalidToken) => {
						const result = await service.validateConnection(url, invalidToken);

						expect(result.valid).toBe(false);
						expect(result.error).toBeDefined();
						expect(result.error).toContain("Invalid");
					},
				),
				{ numRuns: 20 },
			);
		});
	});

	/**
	 * Additional tests for sendMessage and getServerInfo methods
	 */
	describe("sendMessage", () => {
		it("should always return a valid response structure with success property", async () => {
			// Mock axios to always reject (simulating network error)
			axios.post.mockRejectedValue(new Error("Network error"));

			await fc.assert(
				fc.asyncProperty(
					gotifyServerUrlArbitrary(),
					tokenArbitrary(),
					async (url, token) => {
						const result = await service.sendMessage(url, token, {
							title: "Test",
							message: "Test message",
							priority: 5,
						});

						// Result must have success property
						expect(result).toHaveProperty("success");
						expect(typeof result.success).toBe("boolean");

						// If success is false, must have error message
						if (!result.success) {
							expect(result).toHaveProperty("error");
							expect(typeof result.error).toBe("string");
						}
					},
				),
				{ numRuns: 20 },
			);
		});

		it("should reject invalid priority values", async () => {
			await fc.assert(
				fc.asyncProperty(
					gotifyServerUrlArbitrary(),
					tokenArbitrary(),
					fc.integer({ min: 11, max: 100 }), // Invalid priority
					async (url, token, invalidPriority) => {
						const result = await service.sendMessage(url, token, {
							title: "Test",
							message: "Test message",
							priority: invalidPriority,
						});

						expect(result.success).toBe(false);
						expect(result.error).toContain("Priority");
					},
				),
				{ numRuns: 20 },
			);
		});
	});

	describe("getServerInfo", () => {
		it("should always return a valid response structure", async () => {
			// Mock axios to always reject (simulating network error)
			axios.get.mockRejectedValue(new Error("Network error"));

			await fc.assert(
				fc.asyncProperty(
					gotifyServerUrlArbitrary(),
					tokenArbitrary(),
					async (url, token) => {
						const result = await service.getServerInfo(url, token);

						// Result must have either version or error
						expect(
							Object.hasOwn(result, "version") ||
								Object.hasOwn(result, "error"),
						).toBe(true);
					},
				),
				{ numRuns: 20 },
			);
		}, 15000);
	});
});
