import { AlertTriangle, RefreshCw } from "lucide-react";
import { useAuth } from "../contexts/AuthContext";

/**
 * Shown when the initial setup check fails due to server/DB unreachable, rate limit,
 * or CORS_ORIGIN mismatch (accessing from wrong URL).
 */
const SetupCheckError = () => {
	const { setupCheckError, retrySetupCheck } = useAuth();

	const is_rate_limited = setupCheckError === "rate_limited";
	const is_cors_mismatch = setupCheckError === "cors_mismatch";
	const is_server_or_cors = setupCheckError === "server_or_cors";
	const title = is_rate_limited
		? "Too many requests"
		: is_cors_mismatch
			? "Wrong URL"
			: is_server_or_cors
				? "Cannot connect"
				: "Server not available";
	const message = is_rate_limited
		? "Too many requests from this IP. Please wait a few minutes and try again."
		: is_cors_mismatch
			? "You're accessing from a URL that isn't allowed. Please use the URL configured in CORS_ORIGIN in your .env file, then try again."
			: is_server_or_cors
				? "The server is not reachable. This could mean: (1) The server is not running, or (2) You're accessing from a URL that isn't in CORS_ORIGIN. Ensure CORS_ORIGIN in your server .env includes the URL you're using (e.g. https://patchmon-local.local), then try again."
				: "The database or server is not accessible. Please check that the server is running and try again.";

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
				<p className="text-secondary-600 dark:text-white mb-6">{message}</p>
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
