import { createContext, useContext, useState } from "react";
import { useSettings } from "./SettingsContext";

const UpdateNotificationContext = createContext();

export const useUpdateNotification = () => {
	const context = useContext(UpdateNotificationContext);
	if (!context) {
		throw new Error(
			"useUpdateNotification must be used within an UpdateNotificationProvider",
		);
	}
	return context;
};

export const UpdateNotificationProvider = ({ children }) => {
	const [dismissed, setDismissed] = useState(false);
	const { settings, isLoading: settingsLoading } = useSettings();

	// Read cached update information from settings (no GitHub API calls)
	// The backend scheduler updates this data periodically
	const updateAvailable = settings?.is_update_available && !dismissed;
	const updateInfo = settings
		? {
				isUpdateAvailable: settings.is_update_available,
				latestVersion: settings.latest_version,
				currentVersion: settings.current_version,
				last_update_check: settings.last_update_check,
			}
		: null;

	const isLoading = settingsLoading;
	const error = null;

	const dismissNotification = () => {
		setDismissed(true);
	};

	const value = {
		updateAvailable,
		updateInfo,
		dismissNotification,
		isLoading,
		error,
	};

	return (
		<UpdateNotificationContext.Provider value={value}>
			{children}
		</UpdateNotificationContext.Provider>
	);
};
