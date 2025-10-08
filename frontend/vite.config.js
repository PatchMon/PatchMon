import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// https://vitejs.dev/config/
export default defineConfig({
	plugins: [react()],
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
		},
	},
	build: {
		outDir: "dist",
		sourcemap: process.env.NODE_ENV !== "production",
		target: "es2018",
		rollupOptions: {
			output: {
				manualChunks: {
					// React core
					"react-vendor": ["react", "react-dom", "react-router-dom"],
					// Large utility libraries
					"utils-vendor": ["axios", "@tanstack/react-query", "date-fns"],
					// Chart libraries
					"chart-vendor": ["chart.js", "react-chartjs-2"],
					// Icon libraries
					"icons-vendor": ["lucide-react", "react-icons"],
					// DnD libraries
					"dnd-vendor": [
						"@dnd-kit/core",
						"@dnd-kit/sortable",
						"@dnd-kit/utilities",
					],
				},
			},
		},
	},
});
