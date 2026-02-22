/**
 * Unit tests for POST /api/v1/auth/rdp-ticket (RDP/Guacamole ticket endpoint)
 */

const express = require("express");
const request = require("supertest");

jest.mock("../../src/config/prisma");
jest.mock("../../src/middleware/auth");
jest.mock("../../src/middleware/permissions");
jest.mock("axios");

const { getPrismaClient } = require("../../src/config/prisma");
const { authenticateToken } = require("../../src/middleware/auth");
const { requireManageHosts } = require("../../src/middleware/permissions");
const axios = require("axios");

const mockPrisma = {
	hosts: { findUnique: jest.fn() },
};
getPrismaClient.mockReturnValue(mockPrisma);

authenticateToken.mockImplementation((req, _res, next) => {
	req.user = { id: "user-123", role: "admin" };
	next();
});
requireManageHosts.mockImplementation((_req, _res, next) => next());

const VALID_HOST_ID = "a1b2c3d4-e5f6-7890-abcd-ef1234567890";
const VALID_SECRET_KEY = "0123456789abcdef0123456789abcdef"; // 32 hex chars

function createTestApp() {
	const app = express();
	app.use(express.json());
	const authRoutes = require("../../src/routes/authRoutes");
	app.use("/api/v1/auth", authRoutes);
	return app;
}

describe("POST /api/v1/auth/rdp-ticket", () => {
	let app;
	const originalEnv = { ...process.env };

	beforeEach(() => {
		jest.clearAllMocks();
		process.env.GUACAMOLE_JSON_SECRET_KEY = VALID_SECRET_KEY;
		axios.post.mockResolvedValue({ data: { authToken: "mock-guac-token" } });
		app = createTestApp();
	});

	afterAll(() => {
		process.env = originalEnv;
	});

	it("returns 400 when hostId is missing", async () => {
		const res = await request(app)
			.post("/api/v1/auth/rdp-ticket")
			.send({ username: "u", password: "p" });
		expect(res.status).toBe(400);
		expect(res.body.error).toMatch(/validation|required/i);
	});

	it("returns 400 when username is missing", async () => {
		const res = await request(app)
			.post("/api/v1/auth/rdp-ticket")
			.send({ hostId: VALID_HOST_ID, password: "p" });
		expect(res.status).toBe(400);
	});

	it("returns 400 when password is missing", async () => {
		const res = await request(app)
			.post("/api/v1/auth/rdp-ticket")
			.send({ hostId: VALID_HOST_ID, username: "u" });
		expect(res.status).toBe(400);
	});

	it("returns 404 when host is not found", async () => {
		mockPrisma.hosts.findUnique.mockResolvedValue(null);
		const res = await request(app).post("/api/v1/auth/rdp-ticket").send({
			hostId: VALID_HOST_ID,
			username: "u",
			password: "p",
		});
		expect(res.status).toBe(404);
		expect(res.body.error).toBe("Host not found");
	});

	it("returns 400 when host is not Windows", async () => {
		mockPrisma.hosts.findUnique.mockResolvedValue({
			id: VALID_HOST_ID,
			ip: "192.168.1.10",
			hostname: "linux-box",
			os_type: "Linux",
		});
		const res = await request(app).post("/api/v1/auth/rdp-ticket").send({
			hostId: VALID_HOST_ID,
			username: "u",
			password: "p",
		});
		expect(res.status).toBe(400);
		expect(res.body.error).toMatch(/Windows/i);
	});

	it("returns 400 when host has no IP or hostname", async () => {
		mockPrisma.hosts.findUnique.mockResolvedValue({
			id: VALID_HOST_ID,
			ip: null,
			hostname: null,
			os_type: "Windows",
		});
		const res = await request(app).post("/api/v1/auth/rdp-ticket").send({
			hostId: VALID_HOST_ID,
			username: "u",
			password: "p",
		});
		expect(res.status).toBe(400);
		expect(res.body.error).toMatch(/IP|hostname/i);
	});

	it("returns 503 when GUACAMOLE_JSON_SECRET_KEY is missing", async () => {
		delete process.env.GUACAMOLE_JSON_SECRET_KEY;
		app = createTestApp();
		mockPrisma.hosts.findUnique.mockResolvedValue({
			id: VALID_HOST_ID,
			ip: "192.168.1.10",
			hostname: "win-host",
			os_type: "Windows",
		});
		const res = await request(app).post("/api/v1/auth/rdp-ticket").send({
			hostId: VALID_HOST_ID,
			username: "u",
			password: "p",
		});
		expect(res.status).toBe(503);
		expect(res.body.error).toMatch(/RDP|configured|unavailable/i);
		process.env.GUACAMOLE_JSON_SECRET_KEY = VALID_SECRET_KEY;
	});

	it("returns 503 when GUACAMOLE_JSON_SECRET_KEY is not 32 chars", async () => {
		process.env.GUACAMOLE_JSON_SECRET_KEY = "tooshort";
		app = createTestApp();
		mockPrisma.hosts.findUnique.mockResolvedValue({
			id: VALID_HOST_ID,
			ip: "192.168.1.10",
			hostname: "win-host",
			os_type: "Windows",
		});
		const res = await request(app).post("/api/v1/auth/rdp-ticket").send({
			hostId: VALID_HOST_ID,
			username: "u",
			password: "p",
		});
		expect(res.status).toBe(503);
		process.env.GUACAMOLE_JSON_SECRET_KEY = VALID_SECRET_KEY;
	});

	it("returns 200 with authToken, guacamoleBaseUrl, connectionId, dataSource for Windows host", async () => {
		mockPrisma.hosts.findUnique.mockResolvedValue({
			id: VALID_HOST_ID,
			ip: "192.168.1.10",
			hostname: "win-host",
			os_type: "Windows",
		});
		const res = await request(app).post("/api/v1/auth/rdp-ticket").send({
			hostId: VALID_HOST_ID,
			username: "admin",
			password: "secret",
			domain: "CORP",
		});
		expect(res.status).toBe(200);
		expect(res.body).toHaveProperty("authToken", "mock-guac-token");
		expect(res.body).toHaveProperty("guacamoleBaseUrl", "/guacamole");
		expect(res.body).toHaveProperty("connectionId", "PatchMon-RDP");
		expect(res.body).toHaveProperty("dataSource", "json");
		expect(axios.post).toHaveBeenCalledTimes(1);
	});

	it("returns 503 when Guacamole /api/tokens fails", async () => {
		mockPrisma.hosts.findUnique.mockResolvedValue({
			id: VALID_HOST_ID,
			ip: "192.168.1.10",
			hostname: "win-host",
			os_type: "Windows",
		});
		axios.post.mockRejectedValue(new Error("Connection refused"));
		const res = await request(app).post("/api/v1/auth/rdp-ticket").send({
			hostId: VALID_HOST_ID,
			username: "u",
			password: "p",
		});
		expect(res.status).toBe(503);
		expect(res.body.error).toMatch(/RDP|unavailable/i);
	});

	it("returns 503 when Guacamole response has no authToken", async () => {
		mockPrisma.hosts.findUnique.mockResolvedValue({
			id: VALID_HOST_ID,
			ip: "192.168.1.10",
			hostname: "win-host",
			os_type: "Windows",
		});
		axios.post.mockResolvedValue({ data: {} });
		const res = await request(app).post("/api/v1/auth/rdp-ticket").send({
			hostId: VALID_HOST_ID,
			username: "u",
			password: "p",
		});
		expect(res.status).toBe(503);
	});
});
