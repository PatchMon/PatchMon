/**
 * Unit tests for Compliance Routes
 */

const express = require("express");
const request = require("supertest");

// Mock dependencies before requiring routes
jest.mock("../../src/config/prisma");
jest.mock("../../src/middleware/auth");
jest.mock("../../src/services/agentWs");
jest.mock("uuid");

const { getPrismaClient } = require("../../src/config/prisma");
const { authenticateToken } = require("../../src/middleware/auth");
const agentWs = require("../../src/services/agentWs");
const { v4: uuidv4 } = require("uuid");

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
  },
  compliance_rules: {
    findFirst: jest.fn(),
    create: jest.fn(),
  },
  compliance_results: {
    create: jest.fn(),
    findMany: jest.fn(),
  },
  $queryRaw: jest.fn(),
};

getPrismaClient.mockReturnValue(mockPrisma);
uuidv4.mockReturnValue("mock-uuid");

// Mock authenticateToken to pass through
authenticateToken.mockImplementation((req, res, next) => {
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

describe("Compliance Routes", () => {
  let app;

  beforeEach(() => {
    jest.clearAllMocks();
    app = createTestApp();
  });

  describe("POST /api/v1/compliance/scans", () => {
    const mockHost = {
      id: "host-123",
      api_id: "test-api-id",
      api_key: "test-api-key",
      friendly_name: "Test Host",
      hostname: "test-host",
    };

    const mockProfile = {
      id: "profile-123",
      name: "CIS Level 1",
      type: "openscap",
    };

    const mockScan = {
      id: "scan-123",
      host_id: "host-123",
      profile_id: "profile-123",
      score: 85.0,
    };

    beforeEach(() => {
      mockPrisma.hosts.findFirst.mockResolvedValue(mockHost);
      mockPrisma.compliance_profiles.findFirst.mockResolvedValue(mockProfile);
      mockPrisma.compliance_scans.create.mockResolvedValue(mockScan);
      mockPrisma.compliance_rules.findFirst.mockResolvedValue(null);
      mockPrisma.compliance_rules.create.mockResolvedValue({
        id: "rule-123",
        profile_id: "profile-123",
        rule_ref: "test-rule",
        title: "Test Rule",
      });
      mockPrisma.compliance_results.create.mockResolvedValue({});
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
      expect(response.body.scan_id).toBeDefined();
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

      expect(response.body.stats.total_rules).toBe(5);
      expect(response.body.stats.passed).toBe(2);
      expect(response.body.stats.failed).toBe(1);
      expect(response.body.stats.warnings).toBe(1);
      expect(response.body.stats.skipped).toBe(1);
    });
  });

  describe("GET /api/v1/compliance/profiles", () => {
    it("should return list of profiles", async () => {
      const mockProfiles = [
        { id: "1", name: "CIS Level 1", type: "openscap", _count: { compliance_rules: 10, compliance_scans: 5 } },
        { id: "2", name: "Docker Bench", type: "docker-bench", _count: { compliance_rules: 20, compliance_scans: 3 } },
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
        { id: "1", completed_at: new Date(), score: 85 },
        { id: "2", completed_at: new Date(), score: 80 },
      ];

      mockPrisma.compliance_scans.findMany.mockResolvedValue(mockScans);
      mockPrisma.compliance_scans.count.mockResolvedValue(2);

      const response = await request(app)
        .get("/api/v1/compliance/scans/host-123")
        .expect(200);

      expect(response.body.scans).toHaveLength(2);
      expect(response.body.pagination.total).toBe(2);
    });

    it("should support pagination", async () => {
      mockPrisma.compliance_scans.findMany.mockResolvedValue([]);
      mockPrisma.compliance_scans.count.mockResolvedValue(50);

      const response = await request(app)
        .get("/api/v1/compliance/scans/host-123?limit=10&offset=20")
        .expect(200);

      expect(response.body.pagination.limit).toBe(10);
      expect(response.body.pagination.offset).toBe(20);
      expect(response.body.pagination.total).toBe(50);
    });
  });

  describe("GET /api/v1/compliance/scans/:hostId/latest", () => {
    it("should return the latest scan for a host", async () => {
      const mockScan = {
        id: "scan-123",
        score: 85,
        compliance_profiles: { name: "CIS Level 1" },
        compliance_results: [],
      };

      mockPrisma.compliance_scans.findFirst.mockResolvedValue(mockScan);

      const response = await request(app)
        .get("/api/v1/compliance/scans/host-123/latest")
        .expect(200);

      expect(response.body.id).toBe("scan-123");
      expect(response.body.score).toBe(85);
    });

    it("should return 404 if no scans found", async () => {
      mockPrisma.compliance_scans.findFirst.mockResolvedValue(null);

      const response = await request(app)
        .get("/api/v1/compliance/scans/host-123/latest")
        .expect(404);

      expect(response.body.error).toBe("No scans found for this host");
    });
  });

  describe("GET /api/v1/compliance/results/:scanId", () => {
    it("should return results for a scan", async () => {
      const mockResults = [
        { id: "1", status: "fail", compliance_rules: { title: "Rule 1", severity: "high" } },
        { id: "2", status: "pass", compliance_rules: { title: "Rule 2", severity: "low" } },
      ];

      mockPrisma.compliance_results.findMany.mockResolvedValue(mockResults);

      const response = await request(app)
        .get("/api/v1/compliance/results/scan-123")
        .expect(200);

      expect(response.body).toHaveLength(2);
    });

    it("should filter by status", async () => {
      mockPrisma.compliance_results.findMany.mockResolvedValue([]);

      await request(app)
        .get("/api/v1/compliance/results/scan-123?status=fail")
        .expect(200);

      expect(mockPrisma.compliance_results.findMany).toHaveBeenCalledWith(
        expect.objectContaining({
          where: { scan_id: "scan-123", status: "fail" },
        })
      );
    });
  });

  describe("POST /api/v1/compliance/trigger/:hostId", () => {
    const mockHost = {
      id: "host-123",
      api_id: "test-api-id",
    };

    beforeEach(() => {
      mockPrisma.hosts.findUnique.mockResolvedValue(mockHost);
    });

    it("should trigger a compliance scan when host is connected", async () => {
      const mockWs = {
        readyState: 1, // WebSocket.OPEN
        send: jest.fn(),
      };

      agentWs.isConnected.mockReturnValue(true);
      agentWs.getConnectionByApiId.mockReturnValue(mockWs);

      const response = await request(app)
        .post("/api/v1/compliance/trigger/host-123")
        .send({ profile_type: "openscap" })
        .expect(200);

      expect(response.body.message).toBe("Compliance scan triggered");
      expect(mockWs.send).toHaveBeenCalledWith(
        JSON.stringify({ type: "compliance_scan", profile_type: "openscap" })
      );
    });

    it("should return 404 if host not found", async () => {
      mockPrisma.hosts.findUnique.mockResolvedValue(null);

      const response = await request(app)
        .post("/api/v1/compliance/trigger/invalid-host")
        .expect(404);

      expect(response.body.error).toBe("Host not found");
    });

    it("should return 400 if host is not connected", async () => {
      agentWs.isConnected.mockReturnValue(false);

      const response = await request(app)
        .post("/api/v1/compliance/trigger/host-123")
        .expect(400);

      expect(response.body.error).toBe("Host is not connected");
    });
  });

  describe("GET /api/v1/compliance/trends/:hostId", () => {
    it("should return compliance trends", async () => {
      const mockScans = [
        { completed_at: new Date("2024-01-01"), score: 80, compliance_profiles: { name: "CIS", type: "openscap" } },
        { completed_at: new Date("2024-01-15"), score: 85, compliance_profiles: { name: "CIS", type: "openscap" } },
        { completed_at: new Date("2024-01-30"), score: 90, compliance_profiles: { name: "CIS", type: "openscap" } },
      ];

      mockPrisma.compliance_scans.findMany.mockResolvedValue(mockScans);

      const response = await request(app)
        .get("/api/v1/compliance/trends/host-123")
        .expect(200);

      expect(response.body).toHaveLength(3);
    });

    it("should filter by days parameter", async () => {
      mockPrisma.compliance_scans.findMany.mockResolvedValue([]);

      await request(app)
        .get("/api/v1/compliance/trends/host-123?days=7")
        .expect(200);

      expect(mockPrisma.compliance_scans.findMany).toHaveBeenCalledWith(
        expect.objectContaining({
          where: expect.objectContaining({
            host_id: "host-123",
            completed_at: expect.any(Object),
          }),
        })
      );
    });
  });
});
