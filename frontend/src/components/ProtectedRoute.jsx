import { Navigate } from "react-router-dom";
import { useAuth } from "../contexts/AuthContext";

const ProtectedRoute = ({
	children,
	requireAdmin = false,
	requirePermission = null,
}) => {
	const { isAuthenticated, isAdmin, isLoading, hasPermission } = useAuth();

	if (isLoading) {
		return (
			<div className="flex items-center justify-center h-64">
				<div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-600"></div>
			</div>
		);
	}

	if (!isAuthenticated()) {
		return <Navigate to="/login" replace />;
	}

	// Check admin requirement
	if (requireAdmin && !isAdmin()) {
		return (
			<div className="flex items-center justify-center h-64">
				<div className="text-center">
					<h2 className="text-xl font-semibold text-secondary-900 mb-2">
						Access Denied
					</h2>
					<p className="text-secondary-600">
						You don't have permission to access this page.
					</p>
				</div>
			</div>
		);
	}

	// Check specific permission requirement
	if (requirePermission && !hasPermission(requirePermission)) {
		return (
			<div className="flex items-center justify-center h-64">
				<div className="text-center">
					<h2 className="text-xl font-semibold text-secondary-900 mb-2">
						Access Denied
					</h2>
					<p className="text-secondary-600">
						You don't have permission to access this page.
					</p>
				</div>
			</div>
		);
	}

	return children;
};

export default ProtectedRoute;
