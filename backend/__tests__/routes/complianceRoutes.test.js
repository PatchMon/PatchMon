/**
 * Unit tests for Compliance Routes
 */

const express = require("express");
const request = require("supertest");

// Mock dependencies before requiring routes
jest.mock("../../src/config/prisma");
jest.mock("../../src/middleware/auth");
jest.mock("../../src/services/agentWs");
jest.mock("../../src/utils/apiKeyUtils");
// Only mock uuid v4, preserve validate for route validation
jest.mock("uuid", () => ({
	...jest.requireActual("uuid"),
	v4: jest.fn(() => "mock-uuid"),
}));

const { getPrismaClient } = require("../../src/config/prisma");
const { authenticateToken } = require("../../src/middleware/auth");
const agentWs = require("../../src/services/agentWs");
const { verifyApiKey } = require("../../src/utils/apiKeyUtils");
const { v4: _uuidv4 } = require("uuid");

// Mock verifyApiKey to always return true for test API keys
verifyApiKey.mockImplementation((providedKey, storedKey) => {
	return Promise.resolve(providedKey === storedKey);
});

// Setup default mock implementations
const mockPrisma = {
	hosts: {
		findFirst: jest.fn(),
		findUnique: jest.fn(),
		count: jest.fn(),
	},
	compliance_profiles: {
		findFirst: jest.fn(),
		findMany: jest.fn(),
		create: jest.fn(),
	},
	compliance_scans: {
		create: jest.fn(),
		findMany: jest.fn(),
		findFirst: jest.fn(),
		count: jest.fn(),
		deleteMany: jest.fn(),
	},
	compliance_rules: {
		findFirst: jest.fn(),
		create: jest.fn(),
		update: jest.fn(),
	},
	compliance_results: {
		create: jest.fn(),
		createMany: jest.fn(),
		findMany: jest.fn(),
		count: jest.fn(),
		groupBy: jest.fn(),
		upsert: jest.fn(),
	},
	$queryRaw: jest.fn(),
};

getPrismaClient.mockReturnValue(mockPrisma);

// Mock authenticateToken to pass through
authenticateToken.mockImplementation((req, _res, next) => {
	req.user = { id: "user-123", role: "admin" };
	next();
});

// Create test app
function createTestApp() {
	const app = express();
	app.use(express.json());

	const complianceRoutes = require("../../src/routes/complianceRoutes");
	app.use("/api/v1/compliance", complianceRoutes);

	return app;
}

// Valid UUID for tests (routes now validate UUID format)
const VALID_HOST_ID = "a1b2c3d4-e5f6-7890-abcd-ef1234567890";
const VALID_SCAN_ID = "f1e2d3c4-b5a6-7890-abcd-ef0987654321";

describe("Compliance Routes", () => {
	let app;

	beforeEach(() => {
		jest.clearAllMocks();
		app = createTestApp();
	});

	describe("POST /api/v1/compliance/scans", () => {
		const mockHost = {
			id: VALID_HOST_ID,
			api_id: "test-api-id",
			api_key: "test-api-key",
			friendly_name: "Test Host",
			hostname: "test-host",
		};

		const mockProfile = {
			id: "b1c2d3e4-f5a6-7890-abcd-ef1234567891",
			name: "CIS Level 1",
			type: "openscap",
		};

		const mockScan = {
			id: VALID_SCAN_ID,
			host_id: VALID_HOST_ID,
			profile_id: "b1c2d3e4-f5a6-7890-abcd-ef1234567891",
			score: 85.0,
		};

		beforeEach(() => {
			mockPrisma.hosts.findFirst.mockResolvedValue(mockHost);
			mockPrisma.compliance_profiles.findFirst.mockResolvedValue(mockProfile);
			mockPrisma.compliance_scans.deleteMany.mockResolvedValue({ count: 0 });
			mockPrisma.compliance_scans.create.mockResolvedValue(mockScan);
			mockPrisma.compliance_rules.findFirst.mockResolvedValue(null);
			mockPrisma.compliance_rules.create.mockResolvedValue({
				id: "c1d2e3f4-a5b6-7890-abcd-ef1234567892",
				profile_id: "b1c2d3e4-f5a6-7890-abcd-ef1234567891",
				rule_ref: "test-rule",
				title: "Test Rule",
			});
			mockPrisma.compliance_results.create.mockResolvedValue({});
			mockPrisma.compliance_results.createMany.mockResolvedValue({ count: 0 });
			mockPrisma.compliance_results.upsert.mockResolvedValue({});
			mockPrisma.compliance_rules.update.mockResolvedValue({
				id: "c1d2e3f4-a5b6-7890-abcd-ef1234567892",
				profile_id: "b1c2d3e4-f5a6-7890-abcd-ef1234567891",
				rule_ref: "test-rule",
				title: "Test Rule",
			});
		});

		it("should accept scan results with valid API credentials", async () => {
			const response = await request(app)
				.post("/api/v1/compliance/scans")
				.set("X-API-ID", "test-api-id")
				.set("X-API-KEY", "test-api-key")
				.send({
					profile_name: "CIS Level 1",
					profile_type: "openscap",
					results: [
						{ rule_ref: "test-rule", status: "pass", title: "Test Rule" },
					],
				})
				.expect(200);

			expect(response.body.message).toBe("Scan results saved successfully");
			expect(response.body.scans).toBeDefined();
			expect(response.body.scans.length).toBeGreaterThan(0);
			expect(response.body.scans[0].scan_id).toBeDefined();
			expect(mockPrisma.compliance_scans.create).toHaveBeenCalled();
		});

		it("should reject without API credentials", async () => {
			const response = await request(app)
				.post("/api/v1/compliance/scans")
				.send({
					profile_name: "CIS Level 1",
					results: [],
				})
				.expect(401);

			expect(response.body.error).toBe("API credentials required");
		});

		it("should reject with invalid API credentials", async () => {
			mockPrisma.hosts.findFirst.mockResolvedValue(null);

			const response = await request(app)
				.post("/api/v1/compliance/scans")
				.set("X-API-ID", "invalid-id")
				.set("X-API-KEY", "invalid-key")
				.send({
					profile_name: "CIS Level 1",
					results: [],
				})
				.expect(401);

			expect(response.body.error).toBe("Invalid API credentials");
		});

		it("should create a new profile if it does not exist", async () => {
			mockPrisma.compliance_profiles.findFirst.mockResolvedValue(null);
			mockPrisma.compliance_profiles.create.mockResolvedValue(mockProfile);

			await request(app)
				.post("/api/v1/compliance/scans")
				.set("X-API-ID", "test-api-id")
				.set("X-API-KEY", "test-api-key")
				.send({
					profile_name: "New Profile",
					profile_type: "docker-bench",
					results: [],
				})
				.expect(200);

			expect(mockPrisma.compliance_profiles.create).toHaveBeenCalled();
		});

		it("should calculate correct stats from results", async () => {
			const results = [
				{ rule_ref: "rule1", status: "pass", title: "Rule 1" },
				{ rule_ref: "rule2", status: "pass", title: "Rule 2" },
				{ rule_ref: "rule3", status: "fail", title: "Rule 3" },
				{ rule_ref: "rule4", status: "warn", title: "Rule 4" },
				{ rule_ref: "rule5", status: "skip", title: "Rule 5" },
			];

			const response = await request(app)
				.post("/api/v1/compliance/scans")
				.set("X-API-ID", "test-api-id")
				.set("X-API-KEY", "test-api-key")
				.send({
					profile_name: "CIS Level 1",
					results,
				})
				.expect(200);

			expect(response.body.scans).toBeDefined();
			expect(response.body.scans[0].stats.total_rules).toBe(5);
			expect(response.body.scans[0].stats.passed).toBe(2);
			expect(response.body.scans[0].stats.failed).toBe(1);
			expect(response.body.scans[0].stats.warnings).toBe(1);
			expect(response.body.scans[0].stats.skipped).toBe(1);
		});
	});

	describe("GET /api/v1/compliance/profiles", () => {
		it("should return list of profiles", async () => {
			const mockProfiles = [
				{
					id: "1",
					name: "CIS Level 1",
					type: "openscap",
					_count: { compliance_rules: 10, compliance_scans: 5 },
				},
				{
					id: "2",
					name: "Docker Bench",
					type: "docker-bench",
					_count: { compliance_rules: 20, compliance_scans: 3 },
				},
			];

			mockPrisma.compliance_profiles.findMany.mockResolvedValue(mockProfiles);

			const response = await request(app)
				.get("/api/v1/compliance/profiles")
				.expect(200);

			expect(response.body).toHaveLength(2);
			expect(response.body[0].name).toBe("CIS Level 1");
		});
	});

	describe("GET /api/v1/compliance/dashboard", () => {
		it("should return dashboard statistics", async () => {
			mockPrisma.$queryRaw.mockResolvedValue([
				{ host_id: "1", score: 90 },
				{ host_id: "2", score: 75 },
				{ host_id: "3", score: 50 },
			]);
			mockPrisma.hosts.count.mockResolvedValue(2); // unscanned hosts
			mockPrisma.compliance_scans.findMany.mockResolvedValue([]);

			const response = await request(app)
				.get("/api/v1/compliance/dashboard")
				.expect(200);

			expect(response.body.summary).toBeDefined();
			expect(response.body.summary.total_hosts).toBe(3);
			expect(response.body.summary.compliant).toBe(1);
			expect(response.body.summary.warning).toBe(1);
			expect(response.body.summary.critical).toBe(1);
			expect(response.body.summary.unscanned).toBe(2);
		});

		it("should handle empty database", async () => {
			mockPrisma.$queryRaw.mockResolvedValue([]);
			mockPrisma.hosts.count.mockResolvedValue(0);
			mockPrisma.compliance_scans.findMany.mockResolvedValue([]);

			const response = await request(app)
				.get("/api/v1/compliance/dashboard")
				.expect(200);

			expect(response.body.summary.total_hosts).toBe(0);
			expect(response.body.summary.average_score).toBe(0);
		});
	});

	describe("GET /api/v1/compliance/scans/:hostId", () => {
		it("should return scan history for a host", async () => {
			const mockScans = [
				{
					id: "d1e2f3a4-b5c6-7890-abcd-ef1234567893",
					completed_at: new Date(),
					score: 85,
				},
				{
					id: "e1f2a3b4-c5d6-7890-abcd-ef1234567894",
					completed_at: new Date(),
					score: 80,
				},
			];

			mockPrisma.compliance_scans.findMany.mockResolvedValue(mockScans);
			mockPrisma.compliance_scans.count.mockResolvedValue(2);

			const response = await request(app)
				.get(`/api/v1/compliance/scans/${VALID_HOST_ID}`)
				.expect(200);

			expect(response.body.scans).toHaveLength(2);
			expect(response.body.pagination.total).toBe(2);
		});

		it("should support pagination", async () => {
			mockPrisma.compliance_scans.findMany.mockResolvedValue([]);
			mockPrisma.compliance_scans.count.mockResolvedValue(50);

			const response = await request(app)
				.get(`/api/v1/compliance/scans/${VALID_HOST_ID}?limit=10&offset=20`)
				.expect(200);

			expect(response.body.pagination.limit).toBe(10);
			expect(response.body.pagination.offset).toBe(20);
			expect(response.body.pagination.total).toBe(50);
		});
	});

	describe("GET /api/v1/compliance/scans/:hostId/latest", () => {
		it("should return the latest scan for a host", async () => {
			const mockScan = {
				id: VALID_SCAN_ID,
				score: 85,
				compliance_profiles: { name: "CIS Level 1" },
				compliance_results: [],
			};

			mockPrisma.compliance_scans.findFirst.mockResolvedValue(mockScan);

			const response = await request(app)
				.get(`/api/v1/compliance/scans/${VALID_HOST_ID}/latest`)
				.expect(200);

			expect(response.body.id).toBe(VALID_SCAN_ID);
			expect(response.body.score).toBe(85);
		});

		it("should return 404 if no scans found", async () => {
			mockPrisma.compliance_scans.findFirst.mockResolvedValue(null);

			const response = await request(app)
				.get(`/api/v1/compliance/scans/${VALID_HOST_ID}/latest`)
				.expect(404);

			expect(response.body.error).toBe("No scans found for this host");
		});
	});

	describe("GET /api/v1/compliance/results/:scanId", () => {
		beforeEach(() => {
			mockPrisma.compliance_results.count.mockResolvedValue(2);
			mockPrisma.compliance_results.groupBy.mockResolvedValue([]);
			mockPrisma.$queryRaw.mockResolvedValue([]);
		});

		it("should return paginated results for a scan", async () => {
			const mockResults = [
				{
					id: "f1a2b3c4-d5e6-7890-abcd-ef1234567895",
					status: "fail",
					compliance_rules: { title: "Rule 1", severity: "high" },
				},
				{
					id: "a1b2c3d4-e5f6-7890-abcd-ef1234567896",
					status: "pass",
					compliance_rules: { title: "Rule 2", severity: "low" },
				},
			];

			mockPrisma.compliance_results.findMany.mockResolvedValue(mockResults);

			const response = await request(app)
				.get(`/api/v1/compliance/results/${VALID_SCAN_ID}`)
				.expect(200);

			expect(response.body.results).toHaveLength(2);
			expect(response.body.pagination).toEqual(
				expect.objectContaining({
					total: 2,
					limit: 50,
					offset: 0,
				}),
			);
		});

		it("should filter by status", async () => {
			mockPrisma.compliance_results.findMany.mockResolvedValue([]);

			await request(app)
				.get(`/api/v1/compliance/results/${VALID_SCAN_ID}?status=fail`)
				.expect(200);

			expect(mockPrisma.compliance_results.findMany).toHaveBeenCalledWith(
				expect.objectContaining({
					where: { scan_id: VALID_SCAN_ID, status: "fail" },
					take: 50,
					skip: 0,
				}),
			);
		});
	});

	describe("POST /api/v1/compliance/trigger/:hostId", () => {
		const mockHost = {
			id: VALID_HOST_ID,
			api_id: "test-api-id",
		};

		beforeEach(() => {
			mockPrisma.hosts.findUnique.mockResolvedValue(mockHost);
		});

		it("should trigger a compliance scan when host is connected", async () => {
			agentWs.isConnected.mockReturnValue(true);
			agentWs.pushComplianceScan.mockReturnValue(true);

			const response = await request(app)
				.post(`/api/v1/compliance/trigger/${VALID_HOST_ID}`)
				.send({ profile_type: "openscap" })
				.expect(200);

			expect(response.body.message).toBe("Compliance scan triggered");
			expect(agentWs.pushComplianceScan).toHaveBeenCalledWith(
				"test-api-id",
				"openscap",
				{
					enableRemediation: false,
					fetchRemoteResources: false,
					profileId: null,
				},
			);
		});

		it("should return 404 if host not found", async () => {
			mockPrisma.hosts.findUnique.mockResolvedValue(null);

			const response = await request(app)
				.post(`/api/v1/compliance/trigger/${VALID_HOST_ID}`)
				.expect(404);

			expect(response.body.error).toBe("Host not found");
		});

		it("should return 400 if host is not connected", async () => {
			agentWs.isConnected.mockReturnValue(false);

			const response = await request(app)
				.post(`/api/v1/compliance/trigger/${VALID_HOST_ID}`)
				.expect(400);

			expect(response.body.error).toBe("Host is not connected");
		});
	});

	describe("GET /api/v1/compliance/trends/:hostId", () => {
		it("should return compliance trends", async () => {
			const mockScans = [
				{
					completed_at: new Date("2024-01-01"),
					score: 80,
					compliance_profiles: { name: "CIS", type: "openscap" },
				},
				{
					completed_at: new Date("2024-01-15"),
					score: 85,
					compliance_profiles: { name: "CIS", type: "openscap" },
				},
				{
					completed_at: new Date("2024-01-30"),
					score: 90,
					compliance_profiles: { name: "CIS", type: "openscap" },
				},
			];

			mockPrisma.compliance_scans.findMany.mockResolvedValue(mockScans);

			const response = await request(app)
				.get(`/api/v1/compliance/trends/${VALID_HOST_ID}`)
				.expect(200);

			expect(response.body).toHaveLength(3);
		});

		it("should filter by days parameter", async () => {
			mockPrisma.compliance_scans.findMany.mockResolvedValue([]);

			await request(app)
				.get(`/api/v1/compliance/trends/${VALID_HOST_ID}?days=7`)
				.expect(200);

			expect(mockPrisma.compliance_scans.findMany).toHaveBeenCalledWith(
				expect.objectContaining({
					where: expect.objectContaining({
						host_id: VALID_HOST_ID,
						completed_at: expect.any(Object),
					}),
				}),
			);
		});
	});
});
