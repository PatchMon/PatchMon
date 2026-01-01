import { Component } from "react";

/**
 * React Error Boundary component
 * Catches JavaScript errors anywhere in child component tree,
 * logs errors, and displays a fallback UI.
 */
class ErrorBoundary extends Component {
	constructor(props) {
		super(props);
		this.state = { hasError: false, error: null, errorInfo: null };
	}

	static getDerivedStateFromError(error) {
		// Update state so the next render shows the fallback UI
		return { hasError: true, error };
	}

	componentDidCatch(error, errorInfo) {
		// Log error to console (in production, you'd send to error tracking service)
		console.error("ErrorBoundary caught an error:", error);
		console.error("Error info:", errorInfo);

		this.setState({ errorInfo });

		// TODO: In production, send error to monitoring service like Sentry
		// Example: Sentry.captureException(error, { extra: errorInfo });
	}

	handleReload = () => {
		window.location.reload();
	};

	handleGoHome = () => {
		window.location.href = "/";
	};

	render() {
		if (this.state.hasError) {
			// Custom fallback UI
			return (
				<div className="min-h-screen bg-gradient-to-br from-red-50 to-secondary-50 dark:from-secondary-900 dark:to-secondary-800 flex items-center justify-center p-4">
					<div className="max-w-md w-full bg-white dark:bg-secondary-800 rounded-lg shadow-lg p-8 text-center">
						<div className="mb-6">
							<svg
								className="w-16 h-16 text-red-500 mx-auto"
								fill="none"
								stroke="currentColor"
								viewBox="0 0 24 24"
							>
								<path
									strokeLinecap="round"
									strokeLinejoin="round"
									strokeWidth={2}
									d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"
								/>
							</svg>
						</div>

						<h1 className="text-2xl font-bold text-secondary-900 dark:text-white mb-2">
							Something went wrong
						</h1>

						<p className="text-secondary-600 dark:text-secondary-300 mb-6">
							An unexpected error occurred. Our team has been notified and is working on a fix.
						</p>

						{process.env.NODE_ENV === "development" && this.state.error && (
							<div className="mb-6 text-left">
								<details className="bg-secondary-100 dark:bg-secondary-700 rounded-md p-3">
									<summary className="cursor-pointer text-sm font-medium text-secondary-700 dark:text-secondary-300">
										Error Details (Development Only)
									</summary>
									<pre className="mt-2 text-xs text-red-600 dark:text-red-400 overflow-auto max-h-40">
										{this.state.error.toString()}
										{this.state.errorInfo?.componentStack}
									</pre>
								</details>
							</div>
						)}

						<div className="flex flex-col sm:flex-row gap-3 justify-center">
							<button
								onClick={this.handleReload}
								className="px-4 py-2 bg-primary-600 text-white rounded-md hover:bg-primary-700 transition-colors font-medium"
							>
								Reload Page
							</button>
							<button
								onClick={this.handleGoHome}
								className="px-4 py-2 bg-secondary-200 dark:bg-secondary-600 text-secondary-700 dark:text-secondary-200 rounded-md hover:bg-secondary-300 dark:hover:bg-secondary-500 transition-colors font-medium"
							>
								Go to Dashboard
							</button>
						</div>
					</div>
				</div>
			);
		}

		return this.props.children;
	}
}

export default ErrorBoundary;
