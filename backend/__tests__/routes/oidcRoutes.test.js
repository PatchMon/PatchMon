/**
 * Unit tests for OIDC Routes
 */

const express = require("express");
const request = require("supertest");

// Mock dependencies
jest.mock("../../src/auth/oidc", () => ({
	isOIDCEnabled: jest.fn(() => true),
	getAuthorizationUrl: jest.fn(() => ({
		url: "https://idp.example.com/auth?params",
		state: "mock-state",
		codeVerifier: "mock-verifier",
		nonce: "mock-nonce",
	})),
	handleCallback: jest.fn(),
	getOIDCConfig: jest.fn(() => ({
		enabled: true,
		buttonText: "Login with SSO",
		disableLocalAuth: false,
	})),
	getLogoutUrl: jest.fn(() => "https://idp.example.com/logout?params"),
}));

jest.mock("../../src/config/prisma", () => ({
	getPrismaClient: jest.fn(() => ({
		users: {
			findFirst: jest.fn(),
			findUnique: jest.fn(),
			create: jest.fn(),
			update: jest.fn(),
		},
	})),
}));

jest.mock("../../src/utils/session_manager", () => ({
	create_session: jest.fn(() =>
		Promise.resolve({
			access_token: "mock-access-token",
			refresh_token: "mock-refresh-token",
			expires_at: new Date(Date.now() + 3600000),
		})
	),
}));

jest.mock("../../src/routes/dashboardPreferencesRoutes", () => ({
	createDefaultDashboardPreferences: jest.fn(() => Promise.resolve()),
}));

jest.mock("../../src/services/automation/shared/redis", () => ({
	redis: {
		setex: jest.fn(() => Promise.resolve()),
		get: jest.fn(() => Promise.resolve(null)),
		del: jest.fn(() => Promise.resolve()),
	},
}));

jest.mock("../../src/utils/auditLogger", () => ({
	AUDIT_EVENTS: {
		OIDC_LOGIN_SUCCESS: "oidc.login.success",
		OIDC_LOGIN_FAILED: "oidc.login.failed",
	},
	logAuditEvent: jest.fn(() => Promise.resolve()),
}));

jest.mock("uuid", () => ({
	v4: jest.fn(() => "mock-uuid"),
}));

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

// Create test app
function createTestApp() {
	const app = express();
	app.use(express.json());

	// Mock cookie-parser
	app.use((req, res, next) => {
		req.cookies = {};
		next();
	});

	// Add the OIDC routes
	const oidcRoutes = require("../../src/routes/oidcRoutes");
	app.use("/api/v1/auth/oidc", oidcRoutes);

	return app;
}

describe("OIDC Routes", () => {
	let app;
	let prismaMock;

	beforeEach(() => {
		jest.clearAllMocks();

		// Reset mocks to default behavior
		isOIDCEnabled.mockReturnValue(true);
		getOIDCConfig.mockReturnValue({
			enabled: true,
			buttonText: "Login with SSO",
			disableLocalAuth: false,
		});

		prismaMock = getPrismaClient();

		// Create fresh app for each test
		jest.resetModules();
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
			handleCallback.mockResolvedValue(mockUserInfo);
			prismaMock.users.findFirst.mockResolvedValue(mockUser);
			prismaMock.users.update.mockResolvedValue(mockUser);

			// Mock Redis session retrieval
			redis.get.mockResolvedValue(
				JSON.stringify({
					codeVerifier: "mock-verifier",
					nonce: "mock-nonce",
					createdAt: Date.now(),
				})
			);
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

			prismaMock.users.findFirst.mockResolvedValue(null); // No existing user
			prismaMock.users.findUnique.mockResolvedValue(null); // Username not taken
			prismaMock.users.create.mockResolvedValue({
				...mockUser,
				id: "mock-uuid",
			});

			const response = await request(app)
				.get("/api/v1/auth/oidc/callback")
				.query({ code: "auth-code", state: "mock-state" })
				.expect(302);

			expect(prismaMock.users.create).toHaveBeenCalled();
			expect(createDefaultDashboardPreferences).toHaveBeenCalled();
			expect(response.headers.location).toContain("oidc=success");
		});

		it("should redirect with error when user not found and auto-creation disabled", async () => {
			process.env.OIDC_AUTO_CREATE_USERS = "false";
			prismaMock.users.findFirst.mockResolvedValue(null);

			const response = await request(app)
				.get("/api/v1/auth/oidc/callback")
				.query({ code: "auth-code", state: "mock-state" })
				.expect(302);

			expect(response.headers.location).toContain("User+not+found");
		});

		it("should redirect with error when user is inactive", async () => {
			prismaMock.users.findFirst.mockResolvedValue({
				...mockUser,
				is_active: false,
			});

			const response = await request(app)
				.get("/api/v1/auth/oidc/callback")
				.query({ code: "auth-code", state: "mock-state" })
				.expect(302);

			expect(response.headers.location).toContain("Account+disabled");
		});

		it("should link OIDC to existing user with verified email", async () => {
			prismaMock.users.findFirst
				.mockResolvedValueOnce({
					...mockUser,
					oidc_sub: null, // Not linked yet
				})
				.mockResolvedValueOnce(null); // No conflict

			handleCallback.mockResolvedValue({
				...mockUserInfo,
				emailVerified: true,
			});

			await request(app)
				.get("/api/v1/auth/oidc/callback")
				.query({ code: "auth-code", state: "mock-state" });

			expect(prismaMock.users.update).toHaveBeenCalledWith(
				expect.objectContaining({
					data: expect.objectContaining({
						oidc_sub: "oidc-user-123",
					}),
				})
			);
		});

		it("should not link OIDC when email is unverified", async () => {
			prismaMock.users.findFirst.mockResolvedValue({
				...mockUser,
				oidc_sub: null,
			});

			handleCallback.mockResolvedValue({
				...mockUserInfo,
				emailVerified: false,
			});

			await request(app)
				.get("/api/v1/auth/oidc/callback")
				.query({ code: "auth-code", state: "mock-state" });

			// Should update last_login but not oidc_sub
			expect(prismaMock.users.update).toHaveBeenCalledWith(
				expect.objectContaining({
					data: expect.not.objectContaining({
						oidc_sub: expect.anything(),
					}),
				})
			);
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
			// Check that cookies are cleared (have past expiration or empty value)
			expect(
				cookies.some(
					(c) =>
						c.includes("token=;") ||
						c.includes("token=") && c.includes("Expires=Thu, 01 Jan 1970")
				)
			).toBe(true);
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

		it("should reject non-HTTPS in production", async () => {
			process.env.NODE_ENV = "production";

			// Recreate app with production env
			jest.resetModules();
			app = createTestApp();

			const response = await request(app)
				.get("/api/v1/auth/oidc/login")
				.expect(403);

			expect(response.body.error).toBe("HTTPS required for authentication");
		});

		it("should accept HTTPS in production via x-forwarded-proto", async () => {
			process.env.NODE_ENV = "production";

			jest.resetModules();
			app = createTestApp();

			const response = await request(app)
				.get("/api/v1/auth/oidc/config")
				.set("x-forwarded-proto", "https")
				.expect(200);

			expect(response.body.enabled).toBe(true);
		});
	});
});
