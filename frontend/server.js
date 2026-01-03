import path from "node:path";
import { fileURLToPath } from "node:url";
import cors from "cors";
import express from "express";
import { createProxyMiddleware } from "http-proxy-middleware";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const app = express();
const PORT = process.env.PORT || 3000;
const BACKEND_URL = process.env.BACKEND_URL || "http://backend:3001";

// Add security headers to prevent search engine indexing
app.use((_req, res, next) => {
	res.setHeader("X-Robots-Tag", "noindex, nofollow, noarchive, nosnippet");
	next();
});

// Enable CORS for API calls
app.use(
	cors({
		origin: process.env.CORS_ORIGIN || "*",
		credentials: true,
	}),
);

	// Proxy API requests to backend (including WebSocket)
	app.use(
		"/api",
		createProxyMiddleware({
			target: BACKEND_URL,
			changeOrigin: true,
			ws: true,  // Enable WebSocket proxying
			logLevel: "info",
			onError: (err, _req, res) => {
				console.error("Proxy error:", err.message);
				if (!res.headersSent) {
					res.status(500).json({ error: "Backend service unavailable" });
				}
			},
			onProxyReq: (_proxyReq, req, _res) => {
				console.log(`Proxying ${req.method} ${req.path} to ${BACKEND_URL}`);
			},
		}),
	);

// Serve static files from dist directory
app.use(express.static(path.join(__dirname, "dist")));

// Handle SPA routing - serve index.html for all routes
app.get("*", (_req, res) => {
	res.sendFile(path.join(__dirname, "dist", "index.html"));
});

app.listen(PORT, () => {
	console.log(`Frontend server running on port ${PORT}`);
	console.log(`Serving from: ${path.join(__dirname, "dist")}`);
});
