import { useLocation } from "react-router-dom";

const PageTransition = ({ children }) => {
	const location = useLocation();
	// Use top-level path segment as key so nested routes (e.g. /settings/users -> /settings/branding)
	// don't remount the layout; only remount when switching sections (e.g. /hosts -> /settings)
	const pathSegments = location.pathname.split("/").filter(Boolean);
	const sectionKey = pathSegments[0] ?? "home";

	return (
		<div key={sectionKey} className="page-transition-enter">
			{children}
		</div>
	);
};

export default PageTransition;
