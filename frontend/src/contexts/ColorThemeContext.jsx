import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
	createContext,
	useCallback,
	useContext,
	useEffect,
	useMemo,
	useRef,
	useState,
} from "react";
import { userPreferencesAPI } from "../utils/api";
import { useAuth } from "./AuthContext";

const ColorThemeContext = createContext();

// Theme configurations matching the login backgrounds
export const THEME_PRESETS = {
	default: {
		name: "Normal Dark",
		login: {
			cellSize: 90,
			variance: 0.85,
			xColors: ["#0f172a", "#1e293b", "#334155", "#475569", "#64748b"],
			yColors: ["#0f172a", "#1e293b", "#334155", "#475569", "#64748b"],
		},
		app: {
			bgPrimary: "#1e293b",
			bgSecondary: "#1e293b",
			bgTertiary: "#334155",
			borderColor: "#475569",
			cardBg: "#1e293b",
			cardBorder: "#334155",
			buttonBg: "#334155",
			buttonHover: "#475569",
		},
	},
	cyber_blue: {
		name: "Cyber Blue",
		login: {
			cellSize: 90,
			variance: 0.85,
			xColors: ["#0a0820", "#1a1f3a", "#2d3561", "#4a5584", "#667eaf"],
			yColors: ["#0a0820", "#1a1f3a", "#2d3561", "#4a5584", "#667eaf"],
		},
		app: {
			bgPrimary: "#0a0820",
			bgSecondary: "#1a1f3a",
			bgTertiary: "#2d3561",
			borderColor: "#4a5584",
			cardBg: "#1a1f3a",
			cardBorder: "#2d3561",
			buttonBg: "#2d3561",
			buttonHover: "#4a5584",
		},
	},
	neon_purple: {
		name: "Neon Purple",
		login: {
			cellSize: 80,
			variance: 0.9,
			xColors: ["#0f0a1e", "#1e0f3e", "#4a0082", "#7209b7", "#b5179e"],
			yColors: ["#0f0a1e", "#1e0f3e", "#4a0082", "#7209b7", "#b5179e"],
		},
		app: {
			bgPrimary: "#0f0a1e",
			bgSecondary: "#1e0f3e",
			bgTertiary: "#4a0082",
			borderColor: "#7209b7",
			cardBg: "#1e0f3e",
			cardBorder: "#4a0082",
			buttonBg: "#4a0082",
			buttonHover: "#7209b7",
		},
	},
	matrix_green: {
		name: "Matrix Green",
		login: {
			cellSize: 70,
			variance: 0.7,
			xColors: ["#001a00", "#003300", "#004d00", "#006600", "#00b300"],
			yColors: ["#001a00", "#003300", "#004d00", "#006600", "#00b300"],
		},
		app: {
			bgPrimary: "#001a00",
			bgSecondary: "#003300",
			bgTertiary: "#004d00",
			borderColor: "#006600",
			cardBg: "#003300",
			cardBorder: "#004d00",
			buttonBg: "#004d00",
			buttonHover: "#006600",
		},
	},
	ocean_blue: {
		name: "Ocean Blue",
		login: {
			cellSize: 85,
			variance: 0.8,
			xColors: ["#001845", "#023e7d", "#0077b6", "#0096c7", "#00b4d8"],
			yColors: ["#001845", "#023e7d", "#0077b6", "#0096c7", "#00b4d8"],
		},
		app: {
			bgPrimary: "#001845",
			bgSecondary: "#023e7d",
			bgTertiary: "#0077b6",
			borderColor: "#0096c7",
			cardBg: "#023e7d",
			cardBorder: "#0077b6",
			buttonBg: "#0077b6",
			buttonHover: "#0096c7",
		},
	},
	sunset_gradient: {
		name: "Sunset Gradient",
		login: {
			cellSize: 95,
			variance: 0.75,
			xColors: ["#1a0033", "#330066", "#4d0099", "#6600cc", "#9933ff"],
			yColors: ["#1a0033", "#660033", "#990033", "#cc0066", "#ff0099"],
		},
		app: {
			bgPrimary: "#1a0033",
			bgSecondary: "#330066",
			bgTertiary: "#4d0099",
			borderColor: "#6600cc",
			cardBg: "#330066",
			cardBorder: "#4d0099",
			buttonBg: "#4d0099",
			buttonHover: "#6600cc",
		},
	},
};

export const ColorThemeProvider = ({ children }) => {
	const queryClient = useQueryClient();
	const lastThemeRef = useRef(null);

	// Use reactive authentication state from AuthContext
	// This ensures the query re-enables when user logs in
	const { user } = useAuth();
	const isAuthenticated = !!user;

	// Source of truth: Database (via userPreferences query)
	// localStorage is only used as a temporary cache until DB loads
	// Only fetch if user is authenticated to avoid 401 errors on login page
	const { data: userPreferences, isLoading: preferencesLoading } = useQuery({
		queryKey: ["userPreferences"],
		queryFn: () => userPreferencesAPI.get().then((res) => res.data),
		enabled: isAuthenticated, // Only run query if user is authenticated
		retry: 2,
		staleTime: 5 * 60 * 1000, // 5 minutes
		refetchOnWindowFocus: true, // Refetch when user returns to tab
	});

	// Get theme from database (source of truth), fallback to user object from login, then localStorage cache, then default
	// Memoize to prevent recalculation on every render
	const colorThemeValue = useMemo(() => {
		return (
			userPreferences?.color_theme ||
			user?.color_theme ||
			localStorage.getItem("colorTheme") ||
			"cyber_blue"
		);
	}, [userPreferences?.color_theme, user?.color_theme]);

	// Only update state if the theme value actually changed (prevent loops)
	const [colorTheme, setColorTheme] = useState(() => colorThemeValue);

	useEffect(() => {
		// Only update if the value actually changed from what we last saw (prevent loops)
		if (colorThemeValue !== lastThemeRef.current) {
			setColorTheme(colorThemeValue);
			lastThemeRef.current = colorThemeValue;
		}
	}, [colorThemeValue]);

	const isLoading = preferencesLoading;

	// Sync localStorage cache when DB data is available (for offline/performance)
	useEffect(() => {
		if (userPreferences?.color_theme) {
			localStorage.setItem("colorTheme", userPreferences.color_theme);
		}
	}, [userPreferences?.color_theme]);

	const updateColorTheme = useCallback(
		async (theme) => {
			// Store previous theme for potential revert
			const previousTheme = colorTheme;

			// Immediately update state for instant UI feedback
			setColorTheme(theme);
			lastThemeRef.current = theme;

			// Also update localStorage cache
			localStorage.setItem("colorTheme", theme);

			// Save to backend (source of truth)
			try {
				await userPreferencesAPI.update({ color_theme: theme });

				// Invalidate and refetch user preferences to ensure sync across tabs/browsers
				await queryClient.invalidateQueries({ queryKey: ["userPreferences"] });
			} catch (error) {
				console.error("Failed to save color theme preference:", error);
				// Revert to previous theme if save failed
				setColorTheme(previousTheme);
				lastThemeRef.current = previousTheme;
				localStorage.setItem("colorTheme", previousTheme);

				// Invalidate to refresh from DB
				await queryClient.invalidateQueries({ queryKey: ["userPreferences"] });

				// Show error to user if possible (could add toast notification here)
				throw error; // Re-throw so calling code can handle it
			}
		},
		[colorTheme, queryClient],
	);

	// Memoize themeConfig to prevent unnecessary re-renders
	const themeConfig = useMemo(
		() => THEME_PRESETS[colorTheme] || THEME_PRESETS.default,
		[colorTheme],
	);

	// Memoize the context value to prevent unnecessary re-renders
	const value = useMemo(
		() => ({
			colorTheme,
			setColorTheme: updateColorTheme,
			themeConfig,
			isLoading,
		}),
		[colorTheme, themeConfig, isLoading, updateColorTheme],
	);

	return (
		<ColorThemeContext.Provider value={value}>
			{children}
		</ColorThemeContext.Provider>
	);
};

export const useColorTheme = () => {
	const context = useContext(ColorThemeContext);
	if (!context) {
		throw new Error("useColorTheme must be used within ColorThemeProvider");
	}
	return context;
};
