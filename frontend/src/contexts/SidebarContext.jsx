import { createContext, useContext } from "react";

// Sidebar context for controlling sidebar state from child components
const SidebarContext = createContext(null);

export const useSidebar = () => {
	const context = useContext(SidebarContext);
	if (!context) {
		// Return a no-op if used outside provider (for safety)
		return {
			setSidebarCollapsed: () => {},
			sidebarCollapsed: false,
		};
	}
	return context;
};

export default SidebarContext;

