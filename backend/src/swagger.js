const swaggerAutogen = require("swagger-autogen");
const fs = require("node:fs");

const outputFile = "./swagger_output.json";
const endpointsFiles = ["./server.js"];

const doc = {
	info: {
		version: "1.0.0",
		title: "PatchMon REST API",
		description:
			"PatchMon API documentation. For scoped credentials API documentation, see: https://docs.patchmon.net/books/patchmon-application-documentation/page/integration-api-documentation",
	},
	host: "localhost:3000",
	basePath: "/api/v1",
	schemes: ["http", "https"],
	securityDefinitions: {
		basicAuth: {
			type: "basic",
			description:
				"Basic Authentication using API credentials (token_key:token_secret). Used for scoped API endpoints under /api/v1/api/*",
		},
		bearerAuth: {
			type: "apiKey",
			name: "Authorization",
			in: "header",
			description:
				"JWT Bearer token authentication. Format: 'Bearer <token>'. Used for dashboard endpoints.",
		},
	},
};

swaggerAutogen(outputFile, endpointsFiles, doc).then(() => {
	// Post-process: Fix paths to remove /api/ prefix since basePath is /api/v1
	// swagger-autogen generates paths like /api/auth/... and /api/api/hosts/...
	// With basePath /api/v1, we need to remove /api/ from paths so they become /auth/... and /api/hosts/...
	const path = require("node:path");
	const outputPath = path.join(__dirname, outputFile);
	const swaggerDoc = JSON.parse(fs.readFileSync(outputPath, "utf8"));
	const _apiVersion = process.env.API_VERSION || "v1";

	if (swaggerDoc.paths) {
		const newPaths = {};
		for (const [pathKey, methods] of Object.entries(swaggerDoc.paths)) {
			let correctedPath = pathKey;

			// Remove /api/ prefix from all paths since basePath is /api/v1
			// This handles routes like /api/auth/... -> /auth/... (becomes /api/v1/auth/... with basePath)
			if (pathKey.startsWith("/api/")) {
				correctedPath = pathKey.replace("/api/", "/");
			}

			// Special case: /api/api/hosts should become /api/hosts (becomes /api/v1/api/hosts with basePath)
			if (pathKey.startsWith("/api/api/")) {
				correctedPath = pathKey.replace("/api/api/", "/api/");
			}

			newPaths[correctedPath] = methods;
		}
		swaggerDoc.paths = newPaths;
		fs.writeFileSync(outputPath, JSON.stringify(swaggerDoc, null, 2));
		console.log("Fixed API endpoint paths in Swagger documentation");
	}
});
