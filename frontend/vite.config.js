import { Agent as HttpAgent } from "node:http";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// https://vitejs.dev/config/
export default defineConfig({
	plugins: [react()],
	optimizeDeps: {
		include: ["xterm", "xterm-addon-fit"],
		esbuildOptions: {
			target: "es2020",
		},
	},
	server: {
		port: 3000,
		host: "0.0.0.0", // Listen on all interfaces
		strictPort: true, // Exit if port is already in use
		allowedHosts: true, // Allow all hosts in development
		proxy: {
			"/api": {
				target: `http://${process.env.BACKEND_HOST || "localhost"}:${process.env.BACKEND_PORT || "3001"}`,
				changeOrigin: true,
				secure: false,
				ws: true, // Enable WebSocket proxy
				// Configure HTTP agent to support more concurrent connections
				// Fixes 1000ms timeout issue when using HTTP (not HTTPS) with multiple hosts
				agent: new HttpAgent({
					keepAlive: true,
					maxSockets: 50, // Increase from default 6 to handle multiple hosts
					maxFreeSockets: 10,
					timeout: 60000,
					keepAliveMsecs: 1000,
				}),
				configure:
					process.env.VITE_ENABLE_LOGGING === "true"
						? (proxy, _options) => {
								proxy.on("error", (err, _req, _res) => {
									console.log("proxy error", err);
								});
								proxy.on("proxyReq", (_proxyReq, req, _res) => {
									console.log(
										"Sending Request to the Target:",
										req.method,
										req.url,
									);
								});
								proxy.on("proxyRes", (proxyRes, req, _res) => {
									console.log(
										"Received Response from the Target:",
										proxyRes.statusCode,
										req.url,
									);
								});
							}
						: undefined,
			},
			"/admin": {
				target: `http://${process.env.BACKEND_HOST || "localhost"}:${process.env.BACKEND_PORT || "3001"}`,
				changeOrigin: true,
				secure: false,
			},
		},
	},
	build: {
		outDir: "dist",
		sourcemap: process.env.NODE_ENV !== "production",
		target: "es2018",
		rollupOptions: {
			output: {
				manualChunks(id) {
					if (id.includes("node_modules")) {
						// Only split large, standalone libraries that don't have React dependencies
						// chart.js is safe - pure JS library
						if (id.includes("/chart.js/") && !id.includes("react-chartjs")) {
							return "chart-vendor";
						}
						// Icon libraries - pure components, no circular deps
						if (id.includes("/lucide-react/")) {
							return "icons-vendor";
						}
						// DnD libraries
						if (id.includes("/@dnd-kit/")) {
							return "dnd-vendor";
						}
						// Let React, react-router, tanstack, and other interconnected
						// libraries bundle together to avoid initialization order issues
					}
				},
			},
		},
	},
});
