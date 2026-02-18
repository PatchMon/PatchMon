import { useQuery } from "@tanstack/react-query";
import { createContext, useContext } from "react";
import { isAuthReady } from "../constants/authPhases";
import { settingsAPI } from "../utils/api";
import { useAuth } from "./AuthContext";

const SettingsContext = createContext();

export const useSettings = () => {
	const context = useContext(SettingsContext);
	if (!context) {
		throw new Error("useSettings must be used within a SettingsProvider");
	}
	return context;
};

export const SettingsProvider = ({ children }) => {
	const { authPhase, isAuthenticated } = useAuth();

	const {
		data: settings,
		isLoading,
		error,
		refetch,
	} = useQuery({
		queryKey: ["settings", "public"],
		queryFn: async () => {
			try {
				// Try public endpoint first (available to all authenticated users)
				return await settingsAPI.getPublic().then((res) => res.data);
			} catch (error) {
				// If public endpoint fails, try full settings (requires can_manage_settings)
				if (error.response?.status === 403 || error.response?.status === 401) {
					try {
						return await settingsAPI.get().then((res) => res.data);
					} catch (_e) {
						// If both fail, return minimal default
						return { auto_update: false };
					}
				}
				// For other errors, return minimal default
				return { auto_update: false };
			}
		},
		staleTime: 5 * 60 * 1000, // Settings stay fresh for 5 minutes
		refetchOnWindowFocus: false,
		enabled: isAuthReady(authPhase, isAuthenticated()),
	});

	const value = {
		settings,
		isLoading,
		error,
		refetch,
	};

	return (
		<SettingsContext.Provider value={value}>
			{children}
		</SettingsContext.Provider>
	);
};
