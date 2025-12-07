module.exports = {
	testEnvironment: "node",
	roots: ["<rootDir>/src", "<rootDir>/__tests__"],
	testMatch: ["**/__tests__/**/*.test.js", "**/?(*.)+(spec|test).js"],
	collectCoverageFrom: [
		"src/**/*.js",
		"!src/server.js",
		"!src/config/**",
	],
	coverageDirectory: "coverage",
	coverageReporters: ["text", "lcov", "html"],
	setupFilesAfterEnv: ["<rootDir>/__tests__/setup.js"],
	testTimeout: 10000,
	verbose: true,
};
