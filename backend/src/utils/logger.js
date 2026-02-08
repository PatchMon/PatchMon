const winston = require("winston");

// Determine if logging is enabled
const isLoggingEnabled = process.env.ENABLE_LOGGING === "true";
const logToConsole = process.env.LOG_TO_CONSOLE === "true";
const logLevel = process.env.LOG_LEVEL || "info";

// Create the logger instance
const logger = winston.createLogger({
	level: logLevel,
	format: winston.format.combine(
		winston.format.timestamp(),
		winston.format.errors({ stack: true }),
		winston.format.json(),
	),
	transports: [],
	// Silent mode when logging is disabled
	silent: !isLoggingEnabled,
});

// Configure transports based on environment
if (isLoggingEnabled) {
	if (logToConsole) {
		// Log to stdout/stderr instead of files
		logger.add(
			new winston.transports.Console({
				format: winston.format.combine(
					winston.format.timestamp(),
					winston.format.errors({ stack: true }),
					winston.format.printf(({ timestamp, level, message, stack }) => {
						return `${timestamp} [${level.toUpperCase()}]: ${stack || message}`;
					}),
				),
			}),
		);
	} else {
		// Log to files (default behavior)
		logger.add(
			new winston.transports.File({
				filename: "logs/error.log",
				level: "error",
			}),
		);
		logger.add(new winston.transports.File({ filename: "logs/combined.log" }));

		// Also add console logging for non-production environments
		if (process.env.NODE_ENV !== "production") {
			logger.add(
				new winston.transports.Console({
					format: winston.format.simple(),
				}),
			);
		}
	}
}

module.exports = logger;
