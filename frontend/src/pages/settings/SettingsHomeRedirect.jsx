import { Navigate } from "react-router-dom";
import { useAuth } from "../../contexts/AuthContext";

/** Picks a sensible default tab under /settings for the current permissions. */
const SettingsHomeRedirect = () => {
	const {
		canManageSettings,
		canManageNotifications,
		canViewNotificationLogs,
		canViewUsers,
	} = useAuth();

	if (canManageSettings()) {
		return <Navigate to="/settings/server-config" replace />;
	}
	if (canManageNotifications() || canViewNotificationLogs()) {
		return <Navigate to="/settings/alert-channels" replace />;
	}
	if (canViewUsers()) {
		return <Navigate to="/settings/users" replace />;
	}
	return <Navigate to="/settings/profile" replace />;
};

export default SettingsHomeRedirect;
