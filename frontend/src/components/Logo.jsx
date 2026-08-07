import { useQuery } from "@tanstack/react-query";
import { useTheme } from "../contexts/ThemeContext";
import { settingsAPI } from "../utils/api";
import { resolveLogoPath } from "../utils/logoPaths";

const Logo = ({
	className = "h-8 w-auto",
	alt = "PatchMon Logo",
	...props
}) => {
	const { isDark } = useTheme();

	const { data: settings } = useQuery({
		queryKey: ["settings", "public"],
		queryFn: () => settingsAPI.getPublic().then((res) => res.data),
	});

	// Helper function to encode logo path for URLs (handles spaces and special characters)
	const encodeLogoPath = (path) => {
		if (!path) return path;
		// Split path into directory and filename
		const parts = path.split("/");
		const filename = parts.pop();
		const directory = parts.join("/");
		// Encode only the filename part, keep directory structure
		return directory
			? `${directory}/${encodeURIComponent(filename)}`
			: encodeURIComponent(filename);
	};

	// Determine which logo to use based on theme (resolveLogoPath maps legacy defaults)
	const logoSrc = isDark
		? resolveLogoPath(settings?.logo_dark, "logo_dark")
		: resolveLogoPath(settings?.logo_light, "logo_light");

	// Encode the path to handle spaces and special characters
	const encodedLogoSrc = encodeLogoPath(logoSrc);

	// Add cache-busting parameter using updated_at timestamp
	const cacheBuster = settings?.updated_at
		? new Date(settings.updated_at).getTime()
		: Date.now();
	const logoSrcWithCache = `${encodedLogoSrc}?v=${cacheBuster}`;

	return (
		<img
			src={logoSrcWithCache}
			alt={alt}
			className={className}
			onError={(e) => {
				// Fallback to default logo if custom logo fails to load (e.g. 404)
				e.target.src = `${isDark ? "/assets/logo_dark_default.png" : "/assets/logo_light_default.png"}?v=${Date.now()}`;
			}}
			{...props}
		/>
	);
};

export default Logo;
