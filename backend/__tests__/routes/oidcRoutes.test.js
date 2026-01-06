/**
 * Unit tests for OIDC Routes
 */

const express = require("express");
const request = require("supertest");

// Mock dependencies before requiring routes
jest.mock("../../src/auth/oidc");
jest.mock("../../src/config/prisma");
jest.mock("../../src/utils/session_manager");
jest.mock("../../src/routes/dashboardPreferencesRoutes");
jest.mock("../../src/services/automation/shared/redis");
jest.mock("../../src/utils/auditLogger");
jest.mock("uuid");

const {
	isOIDCEnabled,
	getAuthorizationUrl,
	handleCallback,
	getOIDCConfig,
	getLogoutUrl,
} = require("../../src/auth/oidc");
const { getPrismaClient } = require("../../src/config/prisma");
const { create_session } = require("../../src/utils/session_manager");
const { redis } = require("../../src/services/automation/shared/redis");
const { createDefaultDashboardPreferences } = require("../../src/routes/dashboardPreferencesRoutes");
const { v4: uuidv4 } = require("uuid");
const { AUDIT_EVENTS, logAuditEvent } = require("../../src/utils/auditLogger");

// Setup default mock implementations
const mockPrisma = {
	users: {
		findFirst: jest.fn(),
		findUnique: jest.fn(),
		create: jest.fn(),
		update: jest.fn(),
		count: jest.fn().mockResolvedValue(1), // Default: not first user
	},
};

getPrismaClient.mockReturnValue(mockPrisma);
uuidv4.mockReturnValue("mock-uuid");
logAuditEvent.mockResolvedValue();
createDefaultDashboardPreferences.mockResolvedValue();

// Create test app
function createTestApp() {
	const app = express();
	app.use(express.json());

	// Mock cookie-parser
	app.use((req, res, next) => {
		req.cookies = {};
		next();
	});

	const oidcRoutes = require("../../src/routes/oidcRoutes");
	app.use("/api/v1/auth/oidc", oidcRoutes);

	return app;
}

describe("OIDC Routes", () => {
	let app;

	beforeAll(() => {
		// Set up environment
		process.env.NODE_ENV = "development";
		process.env.CORS_ORIGIN = "http://localhost:3000";
	});

	beforeEach(() => {
		jest.clearAllMocks();

		// Default mock implementations
		isOIDCEnabled.mockReturnValue(true);
		getOIDCConfig.mockReturnValue({
			enabled: true,
			buttonText: "Login with SSO",
			disableLocalAuth: false,
		});
		getAuthorizationUrl.mockReturnValue({
			url: "https://idp.example.com/auth?params",
			state: "mock-state",
			codeVerifier: "mock-verifier",
			nonce: "mock-nonce",
		});
		getLogoutUrl.mockReturnValue("https://idp.example.com/logout?params");

		redis.setex.mockResolvedValue();
		redis.get.mockResolvedValue(null);
		redis.del.mockResolvedValue();

		create_session.mockResolvedValue({
			access_token: "mock-access-token",
			refresh_token: "mock-refresh-token",
			expires_at: new Date(Date.now() + 3600000),
		});

		app = createTestApp();
	});

	describe("GET /api/v1/auth/oidc/config", () => {
		it("should return OIDC configuration", async () => {
			const response = await request(app)
				.get("/api/v1/auth/oidc/config")
				.expect(200);

			expect(response.body).toEqual({
				enabled: true,
				buttonText: "Login with SSO",
				disableLocalAuth: false,
			});
		});

		it("should return disabled config when OIDC is off", async () => {
			getOIDCConfig.mockReturnValue({
				enabled: false,
				buttonText: "Login with SSO",
				disableLocalAuth: false,
			});

			const response = await request(app)
				.get("/api/v1/auth/oidc/config")
				.expect(200);

			expect(response.body.enabled).toBe(false);
		});
	});

	describe("GET /api/v1/auth/oidc/login", () => {
		it("should return 400 when OIDC is disabled", async () => {
			isOIDCEnabled.mockReturnValue(false);

			const response = await request(app)
				.get("/api/v1/auth/oidc/login")
				.expect(400);

			expect(response.body.error).toBe("OIDC authentication is not enabled");
		});

		it("should redirect to IdP when OIDC is enabled", async () => {
			const response = await request(app)
				.get("/api/v1/auth/oidc/login")
				.expect(302);

			expect(response.headers.location).toBe(
				"https://idp.example.com/auth?params"
			);
		});

		it("should store session data in Redis", async () => {
			await request(app).get("/api/v1/auth/oidc/login");

			expect(redis.setex).toHaveBeenCalledWith(
				expect.stringContaining("oidc:session:"),
				expect.any(Number),
				expect.stringContaining("codeVerifier")
			);
		});

		it("should set oidc_state cookie", async () => {
			const response = await request(app).get("/api/v1/auth/oidc/login");

			expect(response.headers["set-cookie"]).toBeDefined();
			const cookies = response.headers["set-cookie"];
			expect(cookies.some((c) => c.includes("oidc_state"))).toBe(true);
		});
	});

	describe("GET /api/v1/auth/oidc/callback", () => {
		const mockUserInfo = {
			sub: "oidc-user-123",
			email: "test@example.com",
			name: "Test User",
			emailVerified: true,
			groups: [],
		};

		const mockUser = {
			id: "user-123",
			email: "test@example.com",
			username: "testuser",
			role: "user",
			is_active: true,
			oidc_sub: "oidc-user-123",
		};

		beforeEach(() => {
			// Setup session in Redis
			redis.get.mockResolvedValue(
				JSON.stringify({
					codeVerifier: "mock-verifier",
					nonce: "mock-nonce",
					createdAt: Date.now(),
				})
			);

			handleCallback.mockResolvedValue(mockUserInfo);
			mockPrisma.users.findFirst.mockResolvedValue(mockUser);
			mockPrisma.users.update.mockResolvedValue(mockUser);
		});

		it("should return 400 when OIDC is disabled", async () => {
			isOIDCEnabled.mockReturnValue(false);

			const response = await request(app)
				.get("/api/v1/auth/oidc/callback")
				.query({ code: "auth-code", state: "mock-state" })
				.expect(400);

			expect(response.body.error).toBe("OIDC authentication is not enabled");
		});

		it("should redirect with error when IdP returns error", async () => {
			const response = await request(app)
				.get("/api/v1/auth/oidc/callback")
				.query({ error: "access_denied", error_description: "User denied" })
				.expect(302);

			expect(response.headers.location).toContain("/login?error=");
		});

		it("should redirect with error when state is missing", async () => {
			const response = await request(app)
				.get("/api/v1/auth/oidc/callback")
				.query({ code: "auth-code" })
				.expect(302);

			expect(response.headers.location).toContain("/login?error=");
		});

		it("should redirect with error when code is missing", async () => {
			const response = await request(app)
				.get("/api/v1/auth/oidc/callback")
				.query({ state: "mock-state" })
				.expect(302);

			expect(response.headers.location).toContain("/login?error=");
		});

		it("should redirect with error when session expired", async () => {
			redis.get.mockResolvedValue(null);

			const response = await request(app)
				.get("/api/v1/auth/oidc/callback")
				.query({ code: "auth-code", state: "mock-state" })
				.expect(302);

			expect(response.headers.location).toContain("Session+expired");
		});

		it("should authenticate existing user successfully", async () => {
			const response = await request(app)
				.get("/api/v1/auth/oidc/callback")
				.query({ code: "auth-code", state: "mock-state" })
				.expect(302);

			expect(response.headers.location).toContain("oidc=success");
			expect(create_session).toHaveBeenCalled();
		});

		it("should set auth cookies on successful login", async () => {
			const response = await request(app)
				.get("/api/v1/auth/oidc/callback")
				.query({ code: "auth-code", state: "mock-state" });

			const cookies = response.headers["set-cookie"];
			expect(cookies.some((c) => c.includes("token="))).toBe(true);
			expect(cookies.some((c) => c.includes("refresh_token="))).toBe(true);
		});

		it("should create new user when auto-creation is enabled", async () => {
			process.env.OIDC_AUTO_CREATE_USERS = "true";
			process.env.OIDC_DEFAULT_ROLE = "user";
			process.env.OIDC_ISSUER_URL = "https://idp.example.com";

			mockPrisma.users.findFirst.mockResolvedValue(null);
			mockPrisma.users.findUnique.mockResolvedValue(null);
			mockPrisma.users.create.mockResolvedValue({
				...mockUser,
				id: "mock-uuid",
			});

			const response = await request(app)
				.get("/api/v1/auth/oidc/callback")
				.query({ code: "auth-code", state: "mock-state" })
				.expect(302);

			expect(mockPrisma.users.create).toHaveBeenCalled();
			expect(createDefaultDashboardPreferences).toHaveBeenCalled();
			expect(response.headers.location).toContain("oidc=success");
		});

		it("should redirect with error when user not found and auto-creation disabled", async () => {
			process.env.OIDC_AUTO_CREATE_USERS = "false";
			mockPrisma.users.findFirst.mockResolvedValue(null);

			const response = await request(app)
				.get("/api/v1/auth/oidc/callback")
				.query({ code: "auth-code", state: "mock-state" })
				.expect(302);

			expect(response.headers.location).toContain("User+not+found");
		});

		it("should redirect with error when user is inactive", async () => {
			mockPrisma.users.findFirst.mockResolvedValue({
				...mockUser,
				is_active: false,
			});

			const response = await request(app)
				.get("/api/v1/auth/oidc/callback")
				.query({ code: "auth-code", state: "mock-state" })
				.expect(302);

			expect(response.headers.location).toContain("Account+disabled");
		});

		it("should delete session from Redis after use", async () => {
			await request(app)
				.get("/api/v1/auth/oidc/callback")
				.query({ code: "auth-code", state: "mock-state" });

			expect(redis.del).toHaveBeenCalledWith(
				expect.stringContaining("oidc:session:")
			);
		});
	});

	describe("GET /api/v1/auth/oidc/logout", () => {
		it("should redirect to login when OIDC is disabled", async () => {
			isOIDCEnabled.mockReturnValue(false);

			const response = await request(app)
				.get("/api/v1/auth/oidc/logout")
				.expect(302);

			expect(response.headers.location).toBe("/login");
		});

		it("should redirect to IdP logout when available", async () => {
			const response = await request(app)
				.get("/api/v1/auth/oidc/logout")
				.expect(302);

			expect(response.headers.location).toContain("idp.example.com/logout");
		});

		it("should clear auth cookies on logout", async () => {
			const response = await request(app).get("/api/v1/auth/oidc/logout");

			const cookies = response.headers["set-cookie"];
			expect(cookies).toBeDefined();
			expect(cookies.length).toBeGreaterThan(0);
		});

		it("should redirect to login when no logout URL available", async () => {
			getLogoutUrl.mockReturnValue(null);

			const response = await request(app)
				.get("/api/v1/auth/oidc/logout")
				.expect(302);

			expect(response.headers.location).toBe("/login");
		});
	});

	describe("HTTPS enforcement", () => {
		it("should allow HTTP in non-production", async () => {
			process.env.NODE_ENV = "development";

			const response = await request(app)
				.get("/api/v1/auth/oidc/config")
				.expect(200);

			expect(response.body.enabled).toBe(true);
		});
	});
});
