import { useEffect } from "react";
import { useSettings } from "../contexts/SettingsContext";

const LogoProvider = ({ children }) => {
	const { settings } = useSettings();

	useEffect(() => {
		// Use custom favicon or fallback to default
		const faviconUrl = settings?.favicon || "/assets/favicon.svg";

		// Encode the path to handle spaces and special characters
		const encodeLogoPath = (path) => {
			if (!path) return path;
			const parts = path.split("/");
			const filename = parts.pop();
			const directory = parts.join("/");
			return directory
				? `${directory}/${encodeURIComponent(filename)}`
				: encodeURIComponent(filename);
		};
		const encodedFaviconUrl = encodeLogoPath(faviconUrl);

		// Add cache-busting parameter using updated_at timestamp
		const cacheBuster = settings?.updated_at
			? new Date(settings.updated_at).getTime()
			: Date.now();
		const faviconUrlWithCache = `${encodedFaviconUrl}?v=${cacheBuster}`;

		// Update favicon
		const favicon = document.querySelector('link[rel="icon"]');
		if (favicon) {
			favicon.href = faviconUrlWithCache;
		} else {
			// Create favicon link if it doesn't exist
			const link = document.createElement("link");
			link.rel = "icon";
			link.href = faviconUrlWithCache;
			document.head.appendChild(link);
		}
	}, [settings?.favicon, settings?.updated_at]);

	return children;
};

export default LogoProvider;
