import { useQuery } from "@tanstack/react-query";
import { createContext, useContext, useEffect, useState } from "react";
import { userPreferencesAPI } from "../utils/api";
import { useAuth } from "./AuthContext";

const ThemeContext = createContext();

export const useTheme = () => {
	const context = useContext(ThemeContext);
	if (!context) {
		throw new Error("useTheme must be used within a ThemeProvider");
	}
	return context;
};

export const ThemeProvider = ({ children }) => {
	const [theme, setTheme] = useState(() => {
		// Check localStorage first for immediate render
		const savedTheme = localStorage.getItem("theme");
		if (savedTheme) {
			return savedTheme;
		}
		// Check system preference
		if (window.matchMedia("(prefers-color-scheme: dark)").matches) {
			return "dark";
		}
		return "light";
	});

	// Use reactive authentication state from AuthContext
	// This ensures the query re-enables when user logs in
	const { user } = useAuth();
	const isAuthenticated = !!user;

	// Fetch user preferences from backend (only if authenticated)
	const { data: userPreferences } = useQuery({
		queryKey: ["userPreferences"],
		queryFn: () => userPreferencesAPI.get().then((res) => res.data),
		enabled: isAuthenticated, // Only run query if user is authenticated
		retry: 1,
		staleTime: 5 * 60 * 1000, // 5 minutes
	});

	// Sync with user preferences from backend or user object from login
	useEffect(() => {
		const preferredTheme =
			userPreferences?.theme_preference || user?.theme_preference;
		if (preferredTheme) {
			setTheme(preferredTheme);
			localStorage.setItem("theme", preferredTheme);
		}
	}, [userPreferences, user?.theme_preference]);

	useEffect(() => {
		// Apply theme to document
		if (theme === "dark") {
			document.documentElement.classList.add("dark");
		} else {
			document.documentElement.classList.remove("dark");
		}

		// Save to localStorage
		localStorage.setItem("theme", theme);
	}, [theme]);

	const toggleTheme = async () => {
		const newTheme = theme === "light" ? "dark" : "light";
		setTheme(newTheme);

		// Save to backend
		try {
			await userPreferencesAPI.update({ theme_preference: newTheme });
		} catch (error) {
			console.error("Failed to save theme preference:", error);
			// Theme is already set locally, so user still sees the change
		}
	};

	const value = {
		theme,
		toggleTheme,
		isDark: theme === "dark",
	};

	return (
		<ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
	);
};
