import { useQuery } from "@tanstack/react-query";
import { useTheme } from "../contexts/ThemeContext";
import { settingsAPI } from "../utils/api";

const Logo = ({
	className = "h-8 w-auto",
	alt = "PatchMonEnhanced Logo",
	...props
}) => {
	const { isDark } = useTheme();

	const { data: settings } = useQuery({
		queryKey: ["settings"],
		queryFn: () => settingsAPI.get().then((res) => res.data),
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

	// Determine which logo to use based on theme
	const logoSrc = isDark
		? settings?.logo_dark || "/assets/logo_dark.png"
		: settings?.logo_light || "/assets/logo_light.png";

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
				// Fallback to default logo if custom logo fails to load
				e.target.src = isDark
					? "/assets/logo_dark.png"
					: "/assets/logo_light.png";
			}}
			{...props}
		/>
	);
};

export default Logo;
