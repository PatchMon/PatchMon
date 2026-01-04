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

	// Determine which logo to use based on theme
	const logoSrc = isDark
		? settings?.logo_dark || "/assets/logo_dark.png"
		: settings?.logo_light || "/assets/logo_light.png";

	// Add cache-busting parameter using updated_at timestamp
	const cacheBuster = settings?.updated_at
		? new Date(settings.updated_at).getTime()
		: Date.now();
	const logoSrcWithCache = `${logoSrc}?v=${cacheBuster}`;

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
