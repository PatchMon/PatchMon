import { LogOut, ShieldCheck } from "lucide-react";
import { useState } from "react";
import { useAuth } from "../contexts/AuthContext";
import WizardTfaSetup from "./WizardTfaSetup";

const TfaEnrolmentRequired = () => {
	const { user, logout, completeTfaEnrolment } = useAuth();
	const [error, setError] = useState("");

	const handleComplete = async () => {
		setError("");
		if (!(await completeTfaEnrolment())) {
			setError("Could not finish signing in. Please sign in again.");
		}
	};

	return (
		<div className="min-h-screen bg-gradient-to-br from-primary-50 to-secondary-50 dark:from-secondary-900 dark:to-secondary-800 flex items-center justify-center px-4 py-8">
			<div className="card w-full max-w-md p-6 space-y-6">
				<div className="flex items-start gap-3">
					<div className="w-10 h-10 bg-primary-100 dark:bg-primary-900 rounded-full flex items-center justify-center flex-shrink-0">
						<ShieldCheck className="h-5 w-5 text-primary-600 dark:text-primary-400" />
					</div>
					<div>
						<h1 className="text-lg font-medium text-secondary-900 dark:text-white">
							Set up two-factor authentication
						</h1>
						<p className="mt-1 text-sm text-secondary-600 dark:text-secondary-300">
							An administrator reset two-factor authentication on
							{user?.username ? ` ${user.username}` : " your account"}. Set it
							up again with your authenticator app to continue.
						</p>
					</div>
				</div>

				<WizardTfaSetup onComplete={handleComplete} />

				{error && (
					<p
						role="alert"
						className="text-sm text-danger-600 dark:text-danger-400"
					>
						{error}
					</p>
				)}

				<div className="pt-4 border-t border-secondary-200 dark:border-secondary-600 flex justify-center">
					<button
						type="button"
						onClick={logout}
						className="inline-flex items-center gap-2 text-sm text-secondary-500 hover:text-secondary-700 dark:text-secondary-300 dark:hover:text-white min-h-[44px]"
					>
						<LogOut className="h-4 w-4" />
						Sign out
					</button>
				</div>
			</div>
		</div>
	);
};

export default TfaEnrolmentRequired;
