/**
const logger = require("../utils/logger");
 * Centralized Prisma Client Singleton
 * Prevents multiple Prisma clients from creating connection leaks
 */

const { PrismaClient } = require("@prisma/client");

// Parse DATABASE_URL and add connection pooling parameters
function getOptimizedDatabaseUrl() {
	const originalUrl = process.env.DATABASE_URL;

	if (!originalUrl) {
		throw new Error("DATABASE_URL environment variable is required");
	}

	// Parse the URL
	const url = new URL(originalUrl);

	// Add connection pooling parameters - configurable via environment variables
	const connectionLimit = process.env.DB_CONNECTION_LIMIT || "30";
	const poolTimeout = process.env.DB_POOL_TIMEOUT || "20";
	const connectTimeout = process.env.DB_CONNECT_TIMEOUT || "10";
	const idleTimeout = process.env.DB_IDLE_TIMEOUT || "300";
	const maxLifetime = process.env.DB_MAX_LIFETIME || "1800";

	url.searchParams.set("connection_limit", connectionLimit);
	url.searchParams.set("pool_timeout", poolTimeout);
	url.searchParams.set("connect_timeout", connectTimeout);
	url.searchParams.set("idle_timeout", idleTimeout);
	url.searchParams.set("max_lifetime", maxLifetime);

	// Log connection pool settings in development/debug mode
	if (
		process.env.ENABLE_LOGGING === "true" ||
		process.env.LOG_LEVEL === "debug"
	) {
		logger.info(
			`[Database Pool] connection_limit=${connectionLimit}, pool_timeout=${poolTimeout}s, connect_timeout=${connectTimeout}s`,
		);
	}

	return url.toString();
}

// Singleton Prisma client instance
let prismaInstance = null;

function getPrismaClient() {
	if (!prismaInstance) {
		const optimizedUrl = getOptimizedDatabaseUrl();

		prismaInstance = new PrismaClient({
			datasources: {
				db: {
					url: optimizedUrl,
				},
			},
			log:
				process.env.PRISMA_LOG_QUERIES === "true"
					? ["query", "info", "warn", "error"]
					: ["warn", "error"],
			errorFormat: "pretty",
		});

		// Handle graceful shutdown
		process.on("beforeExit", async () => {
			await prismaInstance.$disconnect();
		});

		process.on("SIGINT", async () => {
			await prismaInstance.$disconnect();
			process.exit(0);
		});

		process.on("SIGTERM", async () => {
			await prismaInstance.$disconnect();
			process.exit(0);
		});
	}

	return prismaInstance;
}

// Connection health check
async function checkDatabaseConnection(prisma) {
	try {
		await prisma.$queryRaw`SELECT 1`;
		return true;
	} catch (error) {
		logger.error("Database connection check failed:", error.message);
		return false;
	}
}

// Wait for database to be available with retry logic
async function waitForDatabase(prisma, options = {}) {
	const maxAttempts =
		options.maxAttempts ||
		parseInt(process.env.PM_DB_CONN_MAX_ATTEMPTS, 10) ||
		30;
	const waitInterval =
		options.waitInterval ||
		parseInt(process.env.PM_DB_CONN_WAIT_INTERVAL, 10) ||
		2;

	if (process.env.ENABLE_LOGGING === "true") {
		logger.info(
			`Waiting for database connection (max ${maxAttempts} attempts, ${waitInterval}s interval)...`,
		);
	}

	for (let attempt = 1; attempt <= maxAttempts; attempt++) {
		try {
			const isConnected = await checkDatabaseConnection(prisma);
			if (isConnected) {
				if (process.env.ENABLE_LOGGING === "true") {
					logger.info(
						`Database connected successfully after ${attempt} attempt(s)`,
					);
				}
				return true;
			}
		} catch {
			// checkDatabaseConnection already logs the error
		}

		if (attempt < maxAttempts) {
			if (process.env.ENABLE_LOGGING === "true") {
				logger.info(
					`⏳ Database not ready (attempt ${attempt}/${maxAttempts}), retrying in ${waitInterval}s...`,
				);
			}
			await new Promise((resolve) => setTimeout(resolve, waitInterval * 1000));
		}
	}

	throw new Error(
		`❌ Database failed to become available after ${maxAttempts} attempts`,
	);
}

// Graceful disconnect with retry
async function disconnectPrisma(prisma, maxRetries = 3) {
	for (let i = 0; i < maxRetries; i++) {
		try {
			await prisma.$disconnect();
			logger.info("Database disconnected successfully");
			return;
		} catch (error) {
			logger.error(`Disconnect attempt ${i + 1} failed:`, error.message);
			if (i === maxRetries - 1) {
				logger.error("Failed to disconnect from database after all retries");
			} else {
				await new Promise((resolve) => setTimeout(resolve, 1000)); // Wait 1 second
			}
		}
	}
}

module.exports = {
	getPrismaClient,
	checkDatabaseConnection,
	waitForDatabase,
	disconnectPrisma,
};
