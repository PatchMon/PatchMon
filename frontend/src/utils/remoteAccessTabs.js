export const getRemoteAccessTabs = ({ isWindowsHost, canUseRemoteAccess }) => ({
	terminal: canUseRemoteAccess,
	rdp: isWindowsHost && canUseRemoteAccess,
});
