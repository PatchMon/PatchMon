import { AlertTriangle, RefreshCw } from "lucide-react";
import { useAuth } from "../contexts/AuthContext";

/**
 * Shown when the initial setup check fails due to backend/DB unreachable or rate limit.
 * Prevents misleading display of the first-time admin setup page when the real issue
 * is that the backend or database is not available.
 */
const SetupCheckError = () => {
	const { setupCheckError, retrySetupCheck } = useAuth();

	const is_rate_limited = setupCheckError === "rate_limited";
	const title = is_rate_limited ? "Too many requests" : "Backend not available";
	const message = is_rate_limited
		? "Too many requests from this IP. Please wait a few minutes and try again."
		: "The database or backend is not accessible. Please check that the server is running and try again.";

	return (
		<div className="min-h-screen bg-gradient-to-br from-primary-50 to-secondary-50 dark:from-secondary-900 dark:to-secondary-800 flex items-center justify-center p-4">
			<div className="max-w-md w-full bg-white dark:bg-secondary-800 rounded-xl shadow-lg border border-secondary-200 dark:border-secondary-700 p-6 text-center">
				<div className="flex justify-center mb-4">
					<AlertTriangle
						className="w-12 h-12 text-amber-500 dark:text-amber-400"
						aria-hidden
					/>
				</div>
				<h1 className="text-xl font-semibold text-secondary-900 dark:text-secondary-100 mb-2">
					{title}
				</h1>
				<p className="text-secondary-600 dark:text-secondary-300 mb-6">
					{message}
				</p>
				<button
					type="button"
					onClick={retrySetupCheck}
					className="inline-flex items-center gap-2 px-4 py-2 bg-primary-600 hover:bg-primary-700 text-white rounded-lg font-medium transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 dark:focus:ring-offset-secondary-800"
				>
					<RefreshCw className="w-4 h-4" aria-hidden />
					Try again
				</button>
			</div>
		</div>
	);
};

export default SetupCheckError;
