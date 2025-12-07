// Test setup file
// This runs before all tests

// Set test environment variables
process.env.JWT_SECRET = process.env.JWT_SECRET || "test-secret-key-for-jwt";
process.env.NODE_ENV = "test";

// Suppress console logs during tests unless DEBUG is set
if (!process.env.DEBUG) {
	global.console = {
		...console,
		log: jest.fn(),
		debug: jest.fn(),
		info: jest.fn(),
		warn: jest.fn(),
		error: jest.fn(),
	};
}
