const IORedis = require("ioredis");

// Build TLS configuration if enabled
function getTlsConfig() {
	if (process.env.REDIS_TLS !== "true") {
		return undefined;
	}
	return {
		rejectUnauthorized: process.env.REDIS_TLS_VERIFY !== "false",
		// Optional: Custom CA certificate
		ca: process.env.REDIS_TLS_CA ? process.env.REDIS_TLS_CA : undefined,
	};
}

// Redis connection configuration with connection pooling
const redisConnection = {
	host: process.env.REDIS_HOST || "localhost",
	port: parseInt(process.env.REDIS_PORT, 10) || 6379,
	password: process.env.REDIS_PASSWORD || undefined,
	username: process.env.REDIS_USER || undefined,
	db: parseInt(process.env.REDIS_DB, 10) || 0,
	// TLS configuration (set REDIS_TLS=true to enable)
	tls: getTlsConfig(),
	// Connection pooling settings
	lazyConnect: true,
	keepAlive: 30000,
	connectTimeout: 30000, // Increased from 10s to 30s
	commandTimeout: 30000, // Increased from 5s to 30s
	enableReadyCheck: false,
	// Reduce connection churn
	family: 4, // Force IPv4
	// Retry settings
	retryDelayOnClusterDown: 300,
	retryDelayOnFailover: 100,
	maxRetriesPerRequest: null, // BullMQ requires this to be null
	// Connection pool settings
	maxLoadingTimeout: 30000,
};

// Create Redis connection with singleton pattern
let redisInstance = null;

function getRedisConnection() {
	if (!redisInstance) {
		redisInstance = new IORedis(redisConnection);

		// Handle graceful shutdown
		process.on("beforeExit", async () => {
			await redisInstance.quit();
		});

		process.on("SIGINT", async () => {
			await redisInstance.quit();
			process.exit(0);
		});

		process.on("SIGTERM", async () => {
			await redisInstance.quit();
			process.exit(0);
		});
	}

	return redisInstance;
}

module.exports = {
	redis: getRedisConnection(),
	redisConnection,
	getRedisConnection,
};
